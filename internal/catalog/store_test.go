package catalog

import (
	"testing"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

func TestStoreEqual(t *testing.T) {
	tests := []struct {
		name     string
		initial  []model.Book
		candidate []model.Book
		want     bool
	}{
		{
			name:      "empty store equals nil slice",
			initial:   nil,
			candidate: nil,
			want:      true,
		},
		{
			name:      "empty store equals empty slice",
			initial:   nil,
			candidate: []model.Book{},
			want:      false, // nil != empty slice for reflect.DeepEqual
		},
		{
			name:    "single book equal to itself",
			initial: []model.Book{{LibraryItemID: "item1", Title: "Book1"}},
			candidate: []model.Book{{LibraryItemID: "item1", Title: "Book1"}},
			want:    true,
		},
		{
			name:    "single book differs in title",
			initial: []model.Book{{LibraryItemID: "item1", Title: "Book1"}},
			candidate: []model.Book{{LibraryItemID: "item1", Title: "Book2"}},
			want:    false,
		},
		{
			name:    "single book differs in LibraryItemID",
			initial: []model.Book{{LibraryItemID: "item1", Title: "Book1"}},
			candidate: []model.Book{{LibraryItemID: "item2", Title: "Book1"}},
			want:    false,
		},
		{
			name: "multiple books equal",
			initial: []model.Book{
				{LibraryItemID: "item1", Title: "Book1"},
				{LibraryItemID: "item2", Title: "Book2"},
			},
			candidate: []model.Book{
				{LibraryItemID: "item1", Title: "Book1"},
				{LibraryItemID: "item2", Title: "Book2"},
			},
			want: true,
		},
		{
			name: "multiple books differ in order",
			initial: []model.Book{
				{LibraryItemID: "item1", Title: "Book1"},
				{LibraryItemID: "item2", Title: "Book2"},
			},
			candidate: []model.Book{
				{LibraryItemID: "item2", Title: "Book2"},
				{LibraryItemID: "item1", Title: "Book1"},
			},
			want: false,
		},
		{
			name: "book with string slices equal",
			initial: []model.Book{
				{
					LibraryItemID: "item1",
					Title:         "Book1",
					Narrators:     []string{"Alice", "Bob"},
					Tags:          []string{"Fiction"},
				},
			},
			candidate: []model.Book{
				{
					LibraryItemID: "item1",
					Title:         "Book1",
					Narrators:     []string{"Alice", "Bob"},
					Tags:          []string{"Fiction"},
				},
			},
			want: true,
		},
		{
			name: "book with string slices differ in order",
			initial: []model.Book{
				{
					LibraryItemID: "item1",
					Title:         "Book1",
					Narrators:     []string{"Alice", "Bob"},
				},
			},
			candidate: []model.Book{
				{
					LibraryItemID: "item1",
					Title:         "Book1",
					Narrators:     []string{"Bob", "Alice"},
				},
			},
			want: false,
		},
		{
			name: "book with CoverResolvable differs",
			initial: []model.Book{
				{LibraryItemID: "item1", Title: "Book1", CoverResolvable: true},
			},
			candidate: []model.Book{
				{LibraryItemID: "item1", Title: "Book1", CoverResolvable: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore()
			if tt.initial != nil {
				store.Set(tt.initial, time.Now())
			}
			got := store.Equal(tt.candidate)
			if got != tt.want {
				t.Errorf("Equal(%v) = %v, want %v", tt.candidate, got, tt.want)
			}
		})
	}
}
