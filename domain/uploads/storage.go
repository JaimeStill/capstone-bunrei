package uploads

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"strconv"

	"github.com/standards-lab/go-storage"
)

type blobs struct {
	client storage.Client
	prefix string
}

func (b *blobs) key(id string, offset int64) string {
	return b.prefix + id + "/" + strconv.FormatInt(offset, 10) + "/" + rand.Text()
}

func (b *blobs) put(ctx context.Context, id string, offset int64, chunk []byte) (string, error) {
	key := b.key(id, offset)
	opts := storage.PutOptions{ContentType: "application/octet-stream", Size: int64(len(chunk))}
	if _, err := b.client.Put(ctx, key, bytes.NewReader(chunk), opts); err != nil {
		return "", err
	}
	return key, nil
}

func (b *blobs) remove(ctx context.Context, key string) {
	_ = b.client.Delete(ctx, key)
}

func (b *blobs) open(ctx context.Context, id string, keys []string) io.ReadCloser {
	return &reader{ctx: ctx, client: b.client, id: id, keys: keys}
}

type reader struct {
	ctx    context.Context
	client storage.Client
	id     string
	keys   []string
	body   io.ReadCloser
}

func (r *reader) Read(p []byte) (int, error) {
	for {
		if r.body == nil {
			if len(r.keys) == 0 {
				return 0, io.EOF
			}
			blob, err := r.client.Get(r.ctx, r.keys[0], storage.GetOptions{})
			if errors.Is(err, storage.ErrNotFound) {
				return 0, &fs.PathError{Op: "read", Path: r.id, Err: fs.ErrNotExist}
			}
			if err != nil {
				return 0, err
			}
			r.keys = r.keys[1:]
			r.body = blob.Body
		}
		n, err := r.body.Read(p)
		if errors.Is(err, io.EOF) {
			err = r.body.Close()
			r.body = nil
			if n > 0 || err != nil {
				return n, err
			}
			continue
		}
		return n, err
	}
}

func (r *reader) Close() error {
	if r.body == nil {
		return nil
	}
	err := r.body.Close()
	r.body = nil
	r.keys = nil
	return err
}
