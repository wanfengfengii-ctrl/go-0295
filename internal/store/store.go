// Package store provides the embedded transactional persistence layer for the
// rock wool facade handover service. It wraps go.etcd.io/bbolt so every entity
// is durably persisted in named buckets, and exposes JSON helpers so component
// records survive restart unchanged.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	bolt "go.etcd.io/bbolt"
)

// Bucket names for each persisted entity kind.
var (
	BucketTasks       = []byte("tasks")
	BucketSnapshots   = []byte("snapshots")
	BucketLayouts     = []byte("layouts")
	BucketBoards      = []byte("boards")
	BucketMortar      = []byte("mortar")
	BucketLedger      = []byte("ledger")
	BucketLeases      = []byte("leases")
	BucketStages      = []byte("stages")
	BucketAnchors     = []byte("anchors")
	BucketCuring      = []byte("curing")
	BucketInspections = []byte("inspections")
	BucketRetests     = []byte("retests")
	BucketReviews     = []byte("reviews")
	BucketTerminal    = []byte("terminal")
	BucketIdempotency = []byte("idempotency")
	BucketEvents      = []byte("events")
)

var allBuckets = [][]byte{
	BucketTasks, BucketSnapshots, BucketLayouts, BucketBoards, BucketMortar,
	BucketLedger, BucketLeases, BucketStages, BucketAnchors, BucketCuring,
	BucketInspections, BucketRetests, BucketReviews, BucketTerminal,
	BucketIdempotency, BucketEvents,
}

// DB is the embedded transactional store. It must be created with Open and
// closed with Close.
type DB struct {
	db       *bolt.DB
	path     string
	tmp      bool // when true, the backing file is removed on Close
	readonly bool
}

// Open opens (or creates) the embedded database at path. If path is empty an
// in-memory database is used (useful for tests). Buckets are created on first
// use so a fresh database is immediately usable.
func Open(path string) (*DB, error) {
	opts := &bolt.Options{Timeout: 0}
	tmp := false
	if path == "" {
		f, err := os.CreateTemp("", "rockwool-*.db")
		if err != nil {
			return nil, fmt.Errorf("store: temp file: %w", err)
		}
		path = f.Name()
		f.Close()
		tmp = true
	}
	db, err := bolt.Open(path, 0o600, opts)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range allBuckets {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init buckets: %w", err)
	}
	return &DB{db: db, path: path, tmp: tmp}, nil
}

// Close closes the underlying database, removing a temporary backing file when
// one was created.
func (d *DB) Close() error {
	err := d.db.Close()
	if d.tmp {
		_ = os.Remove(d.path)
	}
	return err
}

// setReadOnly places the store into read-only isolation. It is only called by
// recovery verification when a persistent invariant is violated.
func (d *DB) setReadOnly() { d.readonly = true }

// Update runs fn inside a read-write transaction and commits it. When the
// store is in read-only isolation it refuses writes with ErrReadOnly. Any error
// from fn — including every domain rejection such as a prefix violation — rolls
// the transaction back, so a rejected command never leaves a partial material
// withdrawal, lease, coverage change or evidence write behind.
func (d *DB) Update(fn func(*Tx) error) error {
	if d.readonly {
		return ErrReadOnly
	}
	var callbackErr error
	err := d.db.Update(func(tx *bolt.Tx) error {
		callbackErr = fn(&Tx{tx: tx})
		return callbackErr
	})
	if err != nil {
		return err
	}
	return callbackErr
}

// View runs fn inside a read-only transaction.
func (d *DB) View(fn func(*Tx) error) error {
	return d.db.View(func(tx *bolt.Tx) error {
		return fn(&Tx{tx: tx})
	})
}

// ErrReadOnly is returned when a write is attempted against a store that
// entered read-only isolation after recovery detected a contradiction.
var ErrReadOnly = errors.New("store: read-only isolation active")

// Tx wraps a bolt transaction with typed JSON helpers.
type Tx struct {
	tx *bolt.Tx
}

func (t *Tx) bucket(name []byte) *bolt.Bucket { return t.tx.Bucket(name) }

// PutJSON serializes v as JSON and stores it under key in bucket name.
func (t *Tx) PutJSON(name []byte, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return t.bucket(name).Put([]byte(key), raw)
}

// GetJSON reads the value at key from bucket name and decodes it into out.
// It reports false when the key is absent.
func (t *Tx) GetJSON(name []byte, key string, out any) (bool, error) {
	raw := t.bucket(name).Get([]byte(key))
	if raw == nil {
		return false, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false, err
	}
	return true, nil
}

// Delete removes key from bucket name.
func (t *Tx) Delete(name []byte, key string) error {
	return t.bucket(name).Delete([]byte(key))
}

// ForEach iterates every key/value in bucket name in ascending key order,
// decoding each value into a fresh instance supplied by newFn and passing it to
// fn together with its key. Iteration stops on the first error.
func (t *Tx) ForEach(name []byte, newFn func() any, fn func(key string, v any) error) error {
	b := t.bucket(name)
	if b == nil {
		return nil
	}
	return b.ForEach(func(k, v []byte) error {
		out := newFn()
		if err := json.Unmarshal(v, out); err != nil {
			return err
		}
		return fn(string(k), out)
	})
}

// AppendEvent appends a raw event line to the append-only event log. The log
// is used by recovery verification and as an audit trail.
func (t *Tx) AppendEvent(event []byte) error {
	b := t.bucket(BucketEvents)
	seq, _ := b.NextSequence()
	key := fmt.Sprintf("%020d", seq)
	return b.Put([]byte(key), event)
}

// compareAndSwapJSON atomically writes v under key only when the current value
// is absent. It returns false (with no write) when the key already exists.
// This is the single-writer primitive backing the terminal competition.
func (t *Tx) compareAndSwapJSON(name []byte, key string, v any) (bool, error) {
	b := t.bucket(name)
	if b.Get([]byte(key)) != nil {
		return false, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	if err := b.Put([]byte(key), raw); err != nil {
		return false, err
	}
	return true, nil
}

// keyJoin builds a deterministic composite key from ordered segments.
func keyJoin(parts ...string) string {
	var buf bytes.Buffer
	for i, p := range parts {
		if i > 0 {
			buf.WriteByte('/')
		}
		buf.WriteString(p)
	}
	return buf.String()
}
