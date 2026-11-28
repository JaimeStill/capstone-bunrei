//go:build integration

package integration_test

import (
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-web-sdk"
	"github.com/standards-lab/go-web-sdk/webtest"

	"github.com/JaimeStill/capstone-bunrei/integration"
)

type check struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

type readiness struct {
	Status string  `json:"status"`
	Checks []check `json:"checks"`
}

var readyChecks = []check{
	{Name: "lifecycle", Ready: true},
	{Name: "database", Ready: true},
	{Name: "storage", Ready: true},
	{Name: "schema", Ready: true},
}

func assertProbeHeaders(t *testing.T, probe string, r *webtest.Response) {
	t.Helper()
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("%s Content-Type = %q, want application/json", probe, ct)
	}
	if cc := r.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("%s Cache-Control = %q, want no-store", probe, cc)
	}
	if id := r.Header.Get("X-Request-Id"); id == "" {
		t.Errorf("%s carries no X-Request-Id; RequestID is not wired", probe)
	}
}

func assertReady(t *testing.T, c *webtest.Client) {
	t.Helper()
	r := c.Get(t, web.ReadyPath).Expect(t, http.StatusOK)
	assertProbeHeaders(t, "readyz", r)
	var ready readiness
	r.JSON(t, &ready)
	if ready.Status != "ready" {
		t.Errorf("readyz status = %q, want ready", ready.Status)
	}
	if !slices.Equal(ready.Checks, readyChecks) {
		t.Errorf("readyz checks = %+v, want %+v", ready.Checks, readyChecks)
	}
}

func TestLifecycle_BootProbeDrain(t *testing.T) {
	s := integration.Start(t, integration.Options{})
	c := s.Client()

	r := c.Get(t, web.HealthPath).Expect(t, http.StatusOK)
	assertProbeHeaders(t, "healthz", r)
	var live map[string]string
	r.JSON(t, &live)
	if len(live) != 1 || live["status"] != "ok" {
		t.Errorf("healthz body = %v, want {status: ok}", live)
	}
	assertReady(t, c)

	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
	out := s.Output()
	if !strings.Contains(out, "server stopped") {
		t.Errorf("drain not logged:\n%s", out)
	}
	if !strings.Contains(out, "url.path="+web.HealthPath) {
		t.Errorf("no probe request record; the middleware stack is not wired:\n%s", out)
	}
}

func TestLifecycle_TwoInstances(t *testing.T) {
	a := integration.Launch(t, integration.Options{})
	b := integration.Launch(t, integration.Options{})
	a.Ready(t)
	b.Ready(t)
	assertReady(t, a.Client())
	assertReady(t, b.Client())
	if code := a.Stop(t); code != 0 {
		t.Errorf("a exit = %d:\n%s", code, a.Output())
	}
	if code := b.Stop(t); code != 0 {
		t.Errorf("b exit = %d:\n%s", code, b.Output())
	}
}

func TestLifecycle_LocalOverlayLoads(t *testing.T) {
	s := integration.Start(t, integration.Options{Env: []string{"BUNREI_ENV=local"}})
	if code := s.Stop(t); code != 0 {
		t.Fatalf("exit = %d, want 0:\n%s", code, s.Output())
	}
}

var readyRecord = regexp.MustCompile(`msg="server ready" addr=(\S+)`)

func TestLifecycle_ReadyRecordNamesTheBoundAddressOnce(t *testing.T) {
	s := integration.Launch(t, integration.Options{Env: []string{"BUNREI_SERVER_PORT=0"}})

	var addr string
	s.Await(t, "the server ready record", func() bool {
		m := readyRecord.FindStringSubmatch(s.Output())
		if m == nil {
			return false
		}
		addr = m[1]
		return true
	})
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" || port == "0" || port == "" {
		t.Fatalf("server ready addr = %q (%v), want 127.0.0.1 and the bound port", addr, err)
	}
	assertReady(t, webtest.NewClient("http://"+addr))
	if n := len(readyRecord.FindAllString(s.Output(), -1)); n != 1 {
		t.Errorf("server ready logged %d times, want once", n)
	}
}

func TestLifecycle_PortInUseExitsOneNamingTheServer(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold a port: %v", err)
	}
	t.Cleanup(func() { _ = held.Close() })
	_, port, _ := net.SplitHostPort(held.Addr().String())

	s := integration.Launch(t, integration.Options{Env: []string{"BUNREI_SERVER_PORT=" + port}})
	if code := s.Wait(t); code != 1 {
		t.Fatalf("exit = %d, want 1:\n%s", code, s.Output())
	}
	failure := regexp.MustCompile(`msg="service failed" error="[^"]*startup:[^"]*server`)
	if !failure.MatchString(s.Output()) {
		t.Errorf("no startup error naming server:\n%s", s.Output())
	}
	if strings.Contains(s.Output(), "server ready") {
		t.Errorf("server ready logged on a failed startup:\n%s", s.Output())
	}
}

func TestLifecycle_ShutdownBoundedByTheTimeout(t *testing.T) {
	const timeout = time.Second
	const margin = 3 * time.Second

	s := integration.Start(t, integration.Options{Env: []string{"BUNREI_SHUTDOWN_TIMEOUT=" + timeout.String()}})

	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("dial %s: %v", s.Addr(), err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	unterminatedHeaders := "GET " + web.HealthPath + " HTTP/1.1\r\nHost: stall\r\n"
	if _, err := io.WriteString(conn, unterminatedHeaders); err != nil {
		t.Fatalf("write partial request: %v", err)
	}
	const settleBeforeDrain = 50 * time.Millisecond
	time.Sleep(settleBeforeDrain)

	began := time.Now()
	code := s.Stop(t)
	elapsed := time.Since(began)
	if code != 1 {
		t.Errorf("exit = %d, want 1 when the drain outlives the timeout:\n%s", code, s.Output())
	}
	if elapsed > timeout+margin {
		t.Errorf("exited %s after the interrupt, want within %s of the %s timeout", elapsed, margin, timeout)
	}
}
