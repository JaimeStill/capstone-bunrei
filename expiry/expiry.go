package expiry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/standards-lab/go-storage"
)

// Options configures a [Reclaimer]; its zero value keeps the defaults.
type Options struct {
	// Now is the clock an object's age is measured against. Nil means
	// time.Now.
	Now func() time.Time
}

// Reclaimer deletes the objects under its prefix that have gone unmodified
// for longer than its age, a budget at a time. It keeps the listing's
// continuation between passes, so passes made one after another walk the
// prefix once; it is safe for concurrent use, though concurrent passes
// share that one walk.
type Reclaimer struct {
	client storage.Client
	prefix string
	age    time.Duration
	now    func() time.Time

	mu    sync.Mutex
	token string
}

// New returns the reclaimer of the objects under prefix in client older
// than age. An empty prefix, which would sweep the whole container, and an
// age that isn't positive are refused with an error. A nil client is a
// wiring defect and panics.
func New(client storage.Client, prefix string, age time.Duration, opts Options) (*Reclaimer, error) {
	if client == nil {
		panic("expiry: nil client")
	}
	if prefix == "" {
		return nil, errors.New("expiry: empty prefix")
	}
	if age <= 0 {
		return nil, fmt.Errorf("expiry: %s: age must be positive, got %s", prefix, age)
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Reclaimer{client: client, prefix: prefix, age: age, now: now}, nil
}

// Pass lists the next page of at most budget objects under the prefix,
// from where the previous pass stopped, and deletes those modified more
// than the age before now. It reports more while the listing has objects
// left past the page, and false once the page ends it, when the next pass
// starts the prefix again. A failed list or delete returns its error and
// starts the next pass at the prefix's beginning; the objects a failed
// pass leaves are found again by a later walk. A budget below 1 is an
// error.
func (r *Reclaimer) Pass(ctx context.Context, budget int) (more bool, err error) {
	if budget < 1 {
		return false, fmt.Errorf("expiry: %s: budget must be at least 1, got %d", r.prefix, budget)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	page, err := r.client.List(ctx, storage.ListOptions{Prefix: r.prefix, Token: r.token, Limit: budget})
	if err != nil {
		r.token = ""
		return false, fmt.Errorf("expiry: %s: list: %w", r.prefix, err)
	}
	cutoff := r.now().Add(-r.age)
	for _, obj := range page.Objects {
		if !obj.ModifiedAt.Before(cutoff) {
			continue
		}
		if err := r.client.Delete(ctx, obj.Key); err != nil {
			r.token = ""
			return false, fmt.Errorf("expiry: %s: delete %s: %w", r.prefix, obj.Key, err)
		}
	}
	r.token = page.Next
	return page.Next != "", nil
}
