package uploads_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/standards-lab/go-storage"
	"github.com/standards-lab/go-storage/storagetest"
	"github.com/standards-lab/sqlate"
	"github.com/standards-lab/sqlate/sqltest"

	"github.com/JaimeStill/capstone-bunrei/domain/uploads"
)

const prefix = "upload/"

func service(t *testing.T, responses ...sqltest.Response) (*uploads.Service, *sqltest.Recorder, *storagetest.Fake) {
	t.Helper()
	pool, rec := sqltest.Open(t, responses...)
	fake := storagetest.NewFake(storagetest.Options{})
	return uploads.New(sqlate.Wrap(pool, sqltest.Dialect{}), fake, prefix), rec, fake
}

func progressAt(offset int64) sqltest.Response {
	return sqltest.Response{
		Columns: []string{"upload_offset", "upload_length", "hash_state", "expires_at"},
		Rows:    [][]driver.Value{{offset, nil, []byte("state"), time.Now().Add(time.Hour)}},
	}
}

func keys(t *testing.T, f *storagetest.Fake) []string {
	t.Helper()
	page, err := f.List(context.Background(), storage.ListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got []string
	for _, obj := range page.Objects {
		got = append(got, obj.Key)
	}
	return got
}

func put(t *testing.T, f *storagetest.Fake, key, body string) {
	t.Helper()
	if _, err := f.Put(context.Background(), key, strings.NewReader(body), storage.PutOptions{}); err != nil {
		t.Fatalf("Put %s: %v", key, err)
	}
}

func TestAppend_AtAnOffsetTheUploadLeftWritesNothing(t *testing.T) {
	s, rec, fake := service(t, progressAt(3))

	cur, ok, err := s.Append(t.Context(), "u", 0, []byte("abc"), []byte("s"))
	if err != nil || ok || cur != 3 {
		t.Fatalf("Append = %d, %v, %v; want 3, false, nil", cur, ok, err)
	}
	if got := keys(t, fake); len(got) != 0 {
		t.Errorf("objects = %v, want none written for a refused append", got)
	}
	if ops := rec.Ops(); !slices.Equal(ops, []sqltest.Op{sqltest.OpQuery}) {
		t.Errorf("ops = %v, want the progress read alone", ops)
	}
}

func TestAppend_CommitsTheObjectItsChunkRowNames(t *testing.T) {
	s, rec, fake := service(t,
		progressAt(4),
		sqltest.Response{Affected: 1},
		progressAt(7),
		sqltest.Response{Affected: 1},
	)

	cur, ok, err := s.Append(t.Context(), "u", 4, []byte("abc"), []byte("s"))
	if err != nil || !ok || cur != 7 {
		t.Fatalf("Append = %d, %v, %v; want 7, true, nil", cur, ok, err)
	}
	objects := keys(t, fake)
	if len(objects) != 1 || !regexp.MustCompile(`^upload/u/4/[A-Z2-7]+$`).MatchString(objects[0]) {
		t.Fatalf("objects = %v, want one at upload/u/4/<append id>", objects)
	}
	var recorded []any
	for _, c := range rec.Calls() {
		if strings.Contains(c.SQL, "INSERT INTO upload_chunk") {
			recorded = c.Args
		}
	}
	if !slices.Contains(recorded, any(objects[0])) {
		t.Errorf("chunk row args = %v, want the object's key %s", recorded, objects[0])
	}
}

func TestAppend_ALostCommitRemovesItsObject(t *testing.T) {
	s, rec, fake := service(t,
		progressAt(0),
		sqltest.Response{Affected: 0},
		progressAt(5),
	)

	cur, ok, err := s.Append(t.Context(), "u", 0, []byte("abc"), []byte("s"))
	if err != nil || ok || cur != 5 {
		t.Fatalf("Append = %d, %v, %v; want 5, false, nil: the winner's offset", cur, ok, err)
	}
	if got := keys(t, fake); len(got) != 0 {
		t.Errorf("objects = %v, want the loser's object removed", got)
	}
	for _, c := range rec.Calls() {
		if strings.Contains(c.SQL, "INSERT INTO upload_chunk") {
			t.Errorf("the lost append recorded a chunk row: %s", c.SQL)
		}
	}
}

func TestAppend_MissingUpload(t *testing.T) {
	s, _, fake := service(t, sqltest.Response{Columns: []string{"upload_offset", "upload_length", "hash_state", "expires_at"}})

	if _, _, err := s.Append(t.Context(), "u", 0, []byte("abc"), nil); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Append = %v, want fs.ErrNotExist", err)
	}
	if got := keys(t, fake); len(got) != 0 {
		t.Errorf("objects = %v, want none written for a missing upload", got)
	}
}

func TestOpen_ReadsTheRecordedObjectsInOrder(t *testing.T) {
	s, _, fake := service(t,
		progressAt(9),
		sqltest.Response{Columns: []string{"object_key"}, Rows: [][]driver.Value{
			{"upload/u/0/B"}, {"upload/u/3/A"}, {"upload/u/6/C"},
		}},
	)
	put(t, fake, "upload/u/0/B", "abc")
	put(t, fake, "upload/u/0/Z", "lost")
	put(t, fake, "upload/u/3/A", "def")
	put(t, fake, "upload/u/6/C", "ghi")

	r, err := s.Open(t.Context(), "u")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = r.Close() }()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "abcdefghi" {
		t.Errorf("Open read %q, want %q: the recorded objects alone, in offset order", got, "abcdefghi")
	}
}

func TestOpen_AnObjectGoneIsMissing(t *testing.T) {
	s, _, _ := service(t,
		progressAt(3),
		sqltest.Response{Columns: []string{"object_key"}, Rows: [][]driver.Value{{"upload/u/0/A"}}},
	)

	r, err := s.Open(t.Context(), "u")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = r.Close() }()
	if _, err := io.ReadAll(r); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("read = %v, want fs.ErrNotExist", err)
	}
}

func TestDelete_RemovesTheRowsThenTheObjects(t *testing.T) {
	s, _, fake := service(t,
		sqltest.Response{Columns: []string{"object_key"}, Rows: [][]driver.Value{{"upload/u/0/A"}, {"upload/u/3/B"}}},
		sqltest.Response{Affected: 1},
	)
	put(t, fake, "upload/u/0/A", "abc")
	put(t, fake, "upload/u/3/B", "def")
	put(t, fake, "upload/v/0/C", "other")

	if err := s.Delete(t.Context(), "u"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := keys(t, fake); !slices.Equal(got, []string{"upload/v/0/C"}) {
		t.Errorf("objects = %v, want the other upload's alone", got)
	}
}

func TestDelete_MissingUploadKeepsItsObjects(t *testing.T) {
	s, _, fake := service(t,
		sqltest.Response{Columns: []string{"object_key"}},
		sqltest.Response{Affected: 0},
	)
	put(t, fake, "upload/u/0/A", "abc")

	if err := s.Delete(t.Context(), "u"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Delete = %v, want fs.ErrNotExist", err)
	}
	if got := keys(t, fake); len(got) != 1 {
		t.Errorf("objects = %v, want them kept", got)
	}
}

func TestReclaimer_PassReportsMoreAfterDeletingTheBudget(t *testing.T) {
	for _, c := range []struct {
		name     string
		affected int64
		more     bool
	}{
		{"a full batch", 25, true},
		{"a short batch", 7, false},
		{"nothing expired", 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, rec, _ := service(t, sqltest.Response{Affected: c.affected})
			more, err := s.Reclaimer().Pass(t.Context(), 25)
			if err != nil {
				t.Fatal(err)
			}
			if more != c.more {
				t.Errorf("more = %t after deleting %d of 25, want %t", more, c.affected, c.more)
			}
			calls := rec.Calls()
			if len(calls) != 1 || calls[0].Op != sqltest.OpExec {
				t.Fatalf("calls = %v, want one delete", calls)
			}
			if !slices.Equal(calls[0].Args, []any{25}) {
				t.Errorf("delete args = %v, want the budget", calls[0].Args)
			}
		})
	}
}

func TestReclaimer_PassRefusesABudgetBelowOne(t *testing.T) {
	s, rec, _ := service(t)
	if _, err := s.Reclaimer().Pass(t.Context(), 0); err == nil {
		t.Error("Pass with budget 0 = nil error, want refused")
	}
	if ops := rec.Ops(); len(ops) != 0 {
		t.Errorf("ops = %v, want none", ops)
	}
}
