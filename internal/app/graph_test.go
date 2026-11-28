package app_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-core/graph"
	"github.com/standards-lab/go-core/graph/graphtest"
	"github.com/standards-lab/go-core/lifecycle"
	"github.com/standards-lab/go-core/reactor"
	"github.com/standards-lab/go-web-sdk"

	"github.com/JaimeStill/capstone-bunrei/internal/app"
	"github.com/JaimeStill/capstone-bunrei/internal/config/configtest"
)

func TestGraph_NodesNamedByTheirFields(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	graphtest.CheckNames(t, a.Nodes())
}

func runSet(a *app.App, extra ...graph.Ref) []graph.Ref {
	return append(append(a.Roots(), a.Nodes().LifecycleConfig), extra...)
}

func buildAsRun(t *testing.T, a *app.App, extra ...graph.Ref) *graph.System {
	t.Helper()
	sys, err := a.Graph().Build(runSet(a, extra...)...)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return sys
}

func observeRun(t *testing.T, a *app.App) []graph.Ref {
	t.Helper()
	var ran []graph.Ref
	a.Graph().Observe(func(r graph.Ref) { ran = append(ran, r) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := a.Run(ctx, io.Discard); code != 0 {
		t.Fatalf("Run = %d, want 0", code)
	}
	return ran
}

func layerError(sys *graph.System, format string, args ...any) error {
	return fmt.Errorf(format+"\nlayers:\n%s", append(args, sys)...)
}

func wantGuardFails(t *testing.T, err error, sys *graph.System, mention string) {
	t.Helper()
	switch {
	case err == nil:
		t.Fatalf("the guard passed a fixture that breaks it\nlayers:\n%s", sys)
	case !strings.Contains(err.Error(), mention):
		t.Errorf("guard error = %q, want it to name %s", err, mention)
	case !strings.Contains(err.Error(), "\nlayers:\n"+sys.String()):
		t.Errorf("guard error = %q, want it to carry the layer table:\n%s", err, sys)
	}
}

func TestGraph_RootsBuildAsRun(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	ran := observeRun(t, a)

	sys := buildAsRun(t, a)
	for _, r := range ran {
		if !sys.Has(r) {
			t.Errorf("Run built %s, the root set's Build did not", r.Name())
		}
	}
	for _, d := range sys.Dependencies() {
		if !slices.Contains(ran, d.Ref) {
			t.Errorf("the root set's Build built %s, Run did not", d.Name)
		}
	}
	if t.Failed() {
		t.Logf("layers:\n%s", sys)
	}
}

func serverAlone(sys *graph.System, server graph.Ref) error {
	if !sys.Has(server) {
		return layerError(sys, "%s is not in the System", server.Name())
	}
	top := sys.LayerOf(server)
	var beside []string
	for _, d := range sys.Dependencies() {
		if d.Ref != server && sys.LayerOf(d.Ref) >= top {
			beside = append(beside, d.Name)
		}
	}
	if len(beside) > 0 {
		return layerError(sys, "%v at or above %s's layer %d; want %s alone in the top layer",
			beside, server.Name(), top, server.Name())
	}
	return nil
}

func TestGraph_ServerAloneInTopLayer(t *testing.T) {
	t.Run("as defined", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		if err := serverAlone(buildAsRun(t, a), a.Nodes().Server); err != nil {
			t.Error(err)
		}
	})

	t.Run("a node above the server", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		n := a.Nodes()
		above := a.Graph().Define("above", func(s *graph.Scope) (*web.Server, error) {
			return s.Use(n.Server), nil
		})
		sys := buildAsRun(t, a, above)
		wantGuardFails(t, serverAlone(sys, n.Server), sys, "above")
	})
}

func fixtureReactor(a *app.App, name string, opts ...graph.Option) *graph.Node[*reactor.Reactor[time.Time]] {
	opts = append([]graph.Option{graph.After(a.Nodes().Router)}, opts...)
	return a.Graph().Define(name, func(*graph.Scope) (*reactor.Reactor[time.Time], error) {
		return reactor.New(reactor.Every(time.Hour), func(context.Context, time.Time) error {
			return nil
		}, reactor.Options{}), nil
	}, opts...)
}

func TestGraph_ServerAboveReactor(t *testing.T) {
	t.Run("in the reactors group", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		fixture := fixtureReactor(a, "fixture", graph.In(a.Nodes().Reactors))

		if !slices.Contains(observeRun(t, a), graph.Ref(fixture)) {
			t.Fatalf("Run did not build the reactor %s", fixture.Name())
		}
		if err := serverAlone(buildAsRun(t, a), a.Nodes().Server); err != nil {
			t.Error(err)
		}
	})

	t.Run("outside the reactors group", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		stray := fixtureReactor(a, "stray", graph.After(a.Nodes().Sweeper))
		sys := buildAsRun(t, a, stray)
		wantGuardFails(t, serverAlone(sys, a.Nodes().Server), sys, "stray")
	})
}

func TestGraph_ReadinessChecks(t *testing.T) {
	cfg := configtest.Config(t)
	a := app.New(cfg, io.Discard)
	sys := buildAsRun(t, a)

	var got []string
	for _, c := range lifecycle.New(sys, cfg.Config).Checks() {
		got = append(got, c.Name)
	}
	want := []string{"database", "storage", "job journal", "work queue", "schema", "sweeper"}
	if !slices.Equal(got, want) {
		t.Errorf("checks = %v, want %v", got, want)
	}
}

func TestGraph_SchemaBetweenTheDatabaseAndTheServer(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	sys := buildAsRun(t, a)
	n := a.Nodes()

	for _, r := range []graph.Ref{n.Database, n.Schema, n.Server} {
		if !sys.Has(r) {
			t.Fatalf("%s is not in the System\nlayers:\n%s", r.Name(), sys)
		}
	}
	below := func(lower, upper graph.Ref) {
		t.Helper()
		if l, u := sys.LayerOf(lower), sys.LayerOf(upper); l >= u {
			t.Errorf("%s in layer %d, %s in layer %d; want %s below", lower.Name(), l, upper.Name(), u, lower.Name())
		}
	}
	below(n.Database, n.Schema)
	below(n.Schema, n.Server)
	if t.Failed() {
		t.Logf("layers:\n%s", sys)
	}
}

func TestGraph_StreamsBelowTheJobRoot(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	sys := buildAsRun(t, a)
	n := a.Nodes()

	for _, r := range []graph.Ref{n.JobJournal, n.WorkQueue, n.Jobs, n.Server} {
		if !sys.Has(r) {
			t.Fatalf("%s is not in the System\nlayers:\n%s", r.Name(), sys)
		}
	}
	for _, stream := range []graph.Ref{n.JobJournal, n.WorkQueue} {
		if l, u := sys.LayerOf(stream), sys.LayerOf(n.Jobs); l >= u {
			t.Errorf("%s in layer %d, %s in layer %d; want %s below", stream.Name(), l, n.Jobs.Name(), u, stream.Name())
		}
	}
	if t.Failed() {
		t.Logf("layers:\n%s", sys)
	}
}

func domainNodes(n app.Nodes) []graph.Ref {
	var refs []graph.Ref
	v := reflect.ValueOf(n)
	for _, field := range v.Fields() {
		if strings.Contains(field.Type().String(), "/capstone-bunrei/domain/") {
			refs = append(refs, field.Interface().(graph.Ref))
		}
	}
	return refs
}

func TestGraph_DomainAboveTheSchema(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	sys := buildAsRun(t, a)
	n := a.Nodes()

	domain := domainNodes(n)
	if len(domain) == 0 {
		t.Fatal("Nodes holds no domain node")
	}
	for _, r := range domain {
		if !sys.Has(r) {
			t.Errorf("%s is not in the System", r.Name())
			continue
		}
		if l, u := sys.LayerOf(n.Schema), sys.LayerOf(r); l >= u {
			t.Errorf("%s in layer %d, %s in layer %d; want %s below", n.Schema.Name(), l, r.Name(), u, n.Schema.Name())
		}
	}
	if t.Failed() {
		t.Logf("layers:\n%s", sys)
	}
}

func TestGraph_SweeperInTheReactorsAboveWhatItUses(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	n := a.Nodes()

	group, err := a.Graph().Build(n.Reactors)
	if err != nil {
		t.Fatalf("Build of %s: %v", n.Reactors.Name(), err)
	}
	if !group.Has(n.Sweeper) {
		t.Errorf("a Build of %s does not hold %s; want it a member", n.Reactors.Name(), n.Sweeper.Name())
	}

	sys := buildAsRun(t, a)
	for _, lower := range []graph.Ref{n.Database, n.SQL, n.Storage, n.Schema} {
		if l, u := sys.LayerOf(lower), sys.LayerOf(n.Sweeper); l >= u {
			t.Errorf("%s in layer %d, %s in layer %d; want %s below", lower.Name(), l, n.Sweeper.Name(), u, lower.Name())
		}
	}
	if t.Failed() {
		t.Logf("layers:\n%s", sys)
	}
}

func TestGraph_SweeperRegistersTheReclaimers(t *testing.T) {
	a := app.New(configtest.Config(t), io.Discard)
	sweeper := buildAsRun(t, a).Get(a.Nodes().Sweeper)

	nudge := func(name string) (panicked bool) {
		defer func() { panicked = recover() != nil }()
		sweeper.Nudge(name)
		return false
	}
	for _, name := range []string{"index", "outbox", "inbox", "content", "input", "uploads", "upload objects", "submissions"} {
		if nudge(name) {
			t.Errorf("Nudge(%q) panicked; want %s registered", name, name)
		}
	}
	if !nudge("files") {
		t.Error(`Nudge("files") did not panic; want a name the sweeper was not built with refused`)
	}
}

func rootDrops(a *app.App, roots []graph.Ref) error {
	n := a.Nodes()
	members, err := a.Graph().Build(n.Reactors)
	if err != nil {
		return fmt.Errorf("Build of %s: %w", n.Reactors.Name(), err)
	}
	empty := len(members.Dependencies()) == 0

	sys, err := a.Graph().Build(roots...)
	if err != nil {
		return fmt.Errorf("Build: %w", err)
	}
	allowed := graph.Drop{Node: n.Server, After: n.Reactors}
	var drops []string
	sawAllowed := false
	for _, d := range sys.Dropped() {
		if empty && d == allowed {
			sawAllowed = true
			continue
		}
		drops = append(drops, fmt.Sprintf("%s's After of %s", d.Node.Name(), d.After.Name()))
	}
	if len(drops) > 0 {
		return layerError(sys, "dropped %s; want every After to order", strings.Join(drops, ", "))
	}
	if empty && !sawAllowed {
		return layerError(sys, "%s's After of the empty %s group was not dropped; want exactly that Drop",
			n.Server.Name(), n.Reactors.Name())
	}
	return nil
}

func TestGraph_RootsDropOnlyTheEmptyReactors(t *testing.T) {
	t.Run("as defined", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		if err := rootDrops(a, runSet(a)); err != nil {
			t.Error(err)
		}
	})

	t.Run("with a reactor", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		fixtureReactor(a, "fixture", graph.In(a.Nodes().Reactors))
		if err := rootDrops(a, runSet(a)); err != nil {
			t.Error(err)
		}
	})

	t.Run("an After of a node not built", func(t *testing.T) {
		a := app.New(configtest.Config(t), io.Discard)
		unbuilt := a.Graph().Value("unbuilt", 0)
		orderer := a.Graph().Value("orderer", 0, graph.After(unbuilt))
		roots := runSet(a, orderer)
		sys, err := a.Graph().Build(roots...)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		wantGuardFails(t, rootDrops(a, roots), sys, "orderer's After of unbuilt")
	})
}

func TestRun_BuildFailureExitsOne(t *testing.T) {
	var log, stderr syncBuffer
	a := app.New(configtest.Config(t), &log)
	a.Graph().Replace(a.Nodes().Logger, func(*graph.Scope) (*slog.Logger, error) {
		return nil, errors.New("unavailable")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := a.Run(ctx, &stderr); code != 1 {
		t.Errorf("Run = %d, want 1", code)
	}
	failure := regexp.MustCompile(`^service failed: .*\blogger\b.*unavailable`)
	if got := stderr.String(); !failure.MatchString(got) {
		t.Errorf("stderr = %q, want a service failure naming the logger node and its error", got)
	}
	if got := log.String(); got != "" {
		t.Errorf("log = %q, want nothing logged", got)
	}
}
