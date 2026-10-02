package bookmeta

import (
	"testing"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

func TestGroupByAuthorSplitsMultipleAuthors(t *testing.T) {
	books := []model.Book{
		{
			LibraryItemID:        "book1",
			Title:                "The Martian",
			AuthorNamesLastFirst: "Weir, Andy",
		},
		{
			LibraryItemID:        "book2",
			Title:                "Hail to the Chief",
			AuthorNamesLastFirst: "Smith, Jane; Weir, Andy",
		},
		{
			LibraryItemID:        "book3",
			Title:                "Another Book",
			AuthorNamesLastFirst: "Smith, Jane",
		},
	}

	SortBooks(books)
	groups := GroupByAuthor(books)

	if len(groups) != 2 {
		t.Errorf("expected 2 author groups, got %d", len(groups))
	}

	for _, g := range groups {
		if g.DisplayName == "Smith, Jane" {
			if len(g.Books) != 2 {
				t.Errorf("Smith, Jane should have 2 books, got %d", len(g.Books))
			}
		} else if g.DisplayName == "Weir, Andy" {
			if len(g.Books) != 2 {
				t.Errorf("Weir, Andy should have 2 books, got %d", len(g.Books))
			}
		} else {
			t.Errorf("unexpected author: %s", g.DisplayName)
		}
	}
}

func TestSplitAuthors(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"Weir, Andy", []string{"Weir, Andy"}},
		{"Smith, Jane; Weir, Andy", []string{"Smith, Jane", "Weir, Andy"}},
		{"Smith, Jane; Weir, Andy; Johnson, Bob", []string{"Smith, Jane", "Weir, Andy", "Johnson, Bob"}},
		{"", []string{"Unknown Author"}},
		{"   ", []string{"Unknown Author"}},
	}

	for _, test := range tests {
		got := splitAuthors(test.input)
		if len(got) != len(test.want) {
			t.Errorf("splitAuthors(%q): got %d authors, want %d", test.input, len(got), len(test.want))
			continue
		}
		for i, want := range test.want {
			if got[i] != want {
				t.Errorf("splitAuthors(%q): got %q at position %d, want %q", test.input, got[i], i, want)
			}
		}
	}
}

func TestGroupByAuthorMultiAuthorBooks(t *testing.T) {
	books := []model.Book{
		{
			LibraryItemID:        "book1",
			Title:                "Collaborative Work",
			AuthorNamesLastFirst: "Smith, Jane; Jones, Bob",
		},
		{
			LibraryItemID:        "book2",
			Title:                "Solo Work",
			AuthorNamesLastFirst: "Smith, Jane",
		},
		{
			LibraryItemID:        "book3",
			Title:                "Another Collaboration",
			AuthorNamesLastFirst: "Jones, Bob; Brown, Carol",
		},
	}

	SortBooks(books)
	groups := GroupByAuthor(books)

	if len(groups) != 3 {
		t.Fatalf("expected 3 author groups, got %d", len(groups))
	}

	expectedAuthors := map[string]int{
		"Smith, Jane":   2,
		"Jones, Bob":    2,
		"Brown, Carol":  1,
	}

	for _, g := range groups {
		expected, ok := expectedAuthors[g.DisplayName]
		if !ok {
			t.Errorf("unexpected author: %s", g.DisplayName)
			continue
		}
		if len(g.Books) != expected {
			t.Errorf("%s: expected %d books, got %d", g.DisplayName, expected, len(g.Books))
		}
	}
}
