package expiry_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"

	"github.com/JaimeStill/capstone-bunrei/expiry"
)

const day = 24 * time.Hour

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newStore(t *testing.T) (*storagetest.Fake, *clock) {
	t.Helper()
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	return storagetest.NewFake(storagetest.Options{Clock: c.now}), c
}

func put(t *testing.T, f *storagetest.Fake, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, err := f.Put(context.Background(), key, strings.NewReader(key), storage.PutOptions{}); err != nil {
			t.Fatalf("Put %s: %v", key, err)
		}
	}
}

func keys(t *testing.T, f *storagetest.Fake) []string {
	t.Helper()
	var got []string
	opts := storage.ListOptions{}
	for {
		page, err := f.List(context.Background(), opts)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, obj := range page.Objects {
			got = append(got, obj.Key)
		}
		if page.Next == "" {
			return got
		}
		opts.Token = page.Next
	}
}

func reclaimer(t *testing.T, f *storagetest.Fake, c *clock, prefix string, age time.Duration) *expiry.Reclaimer {
	t.Helper()
	r, err := expiry.New(f, prefix, age, expiry.Options{Now: c.now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func TestReclaimer_DeletesOnlyAgedObjectsUnderItsPrefix(t *testing.T) {
	f, c := newStore(t)
	put(t, f, "content/aa/old", "input/aa/old", "contents/old")
	c.t = c.t.Add(10 * day)
	put(t, f, "content/bb/fresh")
	c.t = c.t.Add(25 * day)

	r := reclaimer(t, f, c, "content/", 30*day)
	more, err := r.Pass(context.Background(), 10)
	if err != nil || more {
		t.Fatalf("Pass = %v, %v; want false, nil", more, err)
	}

	want := []string{"content/bb/fresh", "contents/old", "input/aa/old"}
	if got := keys(t, f); !slices.Equal(got, want) {
		t.Errorf("objects = %v, want %v: the aged object under content/ deleted, the rest kept", got, want)
	}
}

func TestReclaimer_KeepsAnObjectExactlyItsAge(t *testing.T) {
	f, c := newStore(t)
	put(t, f, "input/edge")
	c.t = c.t.Add(30 * day)

	if _, err := reclaimer(t, f, c, "input/", 30*day).Pass(context.Background(), 10); err != nil {
		t.Fatalf("Pass: %v", err)
	}
	if got := keys(t, f); !slices.Equal(got, []string{"input/edge"}) {
		t.Errorf("objects = %v, want input/edge kept at exactly its age", got)
	}
}

func TestReclaimer_HonoursTheBudgetThenDrains(t *testing.T) {
	f, c := newStore(t)
	put(t, f, "content/1", "content/2", "content/3", "content/4", "content/5")
	c.t = c.t.Add(time.Hour)
	put(t, f, "content/6")
	c.t = c.t.Add(time.Hour)
	fake := &counting{Fake: f}

	r, err := expiry.New(fake, "content/", 90*time.Minute, expiry.Options{Now: c.now})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var passes []bool
	for range 10 {
		before := fake.deletes
		more, err := r.Pass(context.Background(), 2)
		if err != nil {
			t.Fatalf("Pass: %v", err)
		}
		if n := fake.deletes - before; n > 2 {
			t.Errorf("a pass deleted %d objects, want at most its budget of 2", n)
		}
		if fake.lastLimit != 2 {
			t.Errorf("a pass listed with limit %d, want its budget of 2", fake.lastLimit)
		}
		passes = append(passes, more)
		if !more {
			break
		}
	}

	if want := []bool{true, true, false}; !slices.Equal(passes, want) {
		t.Errorf("passes reported more = %v, want %v", passes, want)
	}
	if got := keys(t, f); !slices.Equal(got, []string{"content/6"}) {
		t.Errorf("objects = %v, want content/6 alone, the fresh one", got)
	}

	put(t, f, "content/7")
	c.t = c.t.Add(2 * time.Hour)
	more, err := r.Pass(context.Background(), 2)
	if err != nil || more {
		t.Fatalf("the next walk's Pass = %v, %v; want false, nil", more, err)
	}
	if got := keys(t, f); len(got) != 0 {
		t.Errorf("objects = %v, want none: the next walk starts the prefix again", got)
	}
}

type counting struct {
	*storagetest.Fake
	deletes   int
	lastLimit int
}

func (c *counting) Delete(ctx context.Context, key string) error {
	c.deletes++
	return c.Fake.Delete(ctx, key)
}

func (c *counting) List(ctx context.Context, opts storage.ListOptions) (storage.Page, error) {
	c.lastLimit = opts.Limit
	return c.Fake.List(ctx, opts)
}

func TestReclaimer_AFailedListStartsTheWalkAgain(t *testing.T) {
	f, c := newStore(t)
	put(t, f, "input/1", "input/2", "input/3")
	r := reclaimer(t, f, c, "input/", time.Hour)

	if more, err := r.Pass(context.Background(), 1); err != nil || !more {
		t.Fatalf("Pass = %v, %v; want true, nil", more, err)
	}
	f.SetDown(true)
	if _, err := r.Pass(context.Background(), 1); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("Pass while down = %v, want storage.ErrUnavailable", err)
	}
	f.SetDown(false)

	c.t = c.t.Add(2 * time.Hour)
	for more := true; more; {
		var err error
		if more, err = r.Pass(context.Background(), 1); err != nil {
			t.Fatalf("Pass: %v", err)
		}
	}
	if got := keys(t, f); len(got) != 0 {
		t.Errorf("objects = %v, want none: the walk after the failure starts at input/1", got)
	}
}

func TestNew_RefusesAnInvalidReclaimer(t *testing.T) {
	f, _ := newStore(t)
	for name, tc := range map[string]struct {
		prefix string
		age    time.Duration
	}{
		"empty prefix": {"", time.Hour},
		"zero age":     {"content/", 0},
		"negative age": {"content/", -time.Hour},
	} {
		if _, err := expiry.New(f, tc.prefix, tc.age, expiry.Options{}); err == nil {
			t.Errorf("New with %s = nil error, want refused", name)
		}
	}

	r, err := expiry.New(f, "content/", time.Hour, expiry.Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.Pass(context.Background(), 0); err == nil {
		t.Error("Pass with budget 0 = nil error, want refused")
	}
}
