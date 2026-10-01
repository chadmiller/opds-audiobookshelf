// Package catalog provides a thread-safe holder for the most recently
// scanned set of books, so the HTTP server can read it concurrently with the
// periodic scanner replacing it.
package catalog

import (
	"sync/atomic"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// Store holds the latest scan result.
type Store struct {
	v atomic.Value // snapshot
}

type snapshot struct {
	books     []model.Book
	updatedAt time.Time
}

// NewStore returns an empty, ready-to-use Store.
func NewStore() *Store {
	s := &Store{}
	s.v.Store(snapshot{})
	return s
}

// Set replaces the current catalog contents.
func (s *Store) Set(books []model.Book, updatedAt time.Time) {
	s.v.Store(snapshot{books: books, updatedAt: updatedAt})
}

// Get returns the current catalog and when it was last updated. UpdatedAt is
// the zero time if no scan has completed yet.
func (s *Store) Get() ([]model.Book, time.Time) {
	snap := s.v.Load().(snapshot)
	return snap.books, snap.updatedAt
}
