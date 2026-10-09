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

func TestFormatTitleWithSeries(t *testing.T) {
	tests := []struct {
		name        string
		book        model.Book
		padding     int
		expected    string
	}{
		{
			name: "no series",
			book: model.Book{Title: "Dune Messiah"},
			padding: 2,
			expected: "Dune Messiah",
		},
		{
			name: "series with single digit, padding 2",
			book: model.Book{
				Title:          "Dune Messiah",
				SeriesName:     "Dune",
				SeriesSequence: "2",
			},
			padding: 2,
			expected: "(Dune #02) Dune Messiah",
		},
		{
			name: "series with single digit, padding 3",
			book: model.Book{
				Title:          "Dune Messiah",
				SeriesName:     "Dune",
				SeriesSequence: "2",
			},
			padding: 3,
			expected: "(Dune #002) Dune Messiah",
		},
		{
			name: "series with two digits, padding 2",
			book: model.Book{
				Title:          "God Emperor of Dune",
				SeriesName:     "Dune",
				SeriesSequence: "10",
			},
			padding: 2,
			expected: "(Dune #10) God Emperor of Dune",
		},
		{
			name: "series with decimal, padding 3",
			book: model.Book{
				Title:          "Some Book",
				SeriesName:     "Series",
				SeriesSequence: "2.5",
			},
			padding: 3,
			expected: "(Series #002.5) Some Book",
		},
		{
			name: "series with leading zeros in fractional part (preserved)",
			book: model.Book{
				Title:          "Book",
				SeriesName:     "Name",
				SeriesSequence: "8.005",
			},
			padding: 1,
			expected: "(Name #8.005) Book",
		},
		{
			name: "series with trailing zeros in fractional part (dropped)",
			book: model.Book{
				Title:          "Book",
				SeriesName:     "Name",
				SeriesSequence: "100.50",
			},
			padding: 3,
			expected: "(Name #100.5) Book",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := FormatTitle(test.book, test.padding)
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestMaxSequencePadding(t *testing.T) {
	tests := []struct {
		name     string
		books    []model.Book
		expected int
	}{
		{
			name:     "no books",
			books:    []model.Book{},
			expected: 0,
		},
		{
			name:     "no series",
			books:    []model.Book{{Title: "Book1"}, {Title: "Book2"}},
			expected: 0,
		},
		{
			name: "single digit series",
			books: []model.Book{
				{SeriesSequence: "1"},
				{SeriesSequence: "5"},
			},
			expected: 1,
		},
		{
			name: "double digit series",
			books: []model.Book{
				{SeriesSequence: "1"},
				{SeriesSequence: "10"},
			},
			expected: 2,
		},
		{
			name: "triple digit series",
			books: []model.Book{
				{SeriesSequence: "5"},
				{SeriesSequence: "100"},
			},
			expected: 3,
		},
		{
			name: "mixed with decimal",
			books: []model.Book{
				{SeriesSequence: "1"},
				{SeriesSequence: "2.5"},
			},
			expected: 1,
		},
		{
			name: "decimal with larger whole part",
			books: []model.Book{
				{SeriesSequence: "1.5"},
				{SeriesSequence: "100.15"},
			},
			expected: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := MaxSequencePadding(test.books)
			if got != test.expected {
				t.Errorf("got %d, want %d", got, test.expected)
			}
		})
	}
}

func TestGroupBySeriesStripsArticles(t *testing.T) {
	books := []model.Book{
		{
			LibraryItemID: "book1",
			Title:         "The Expanse",
			SeriesName:    "The Expanse",
		},
		{
			LibraryItemID: "book2",
			Title:         "A Series of Events",
			SeriesName:    "A Series of Events",
		},
		{
			LibraryItemID: "book3",
			Title:         "An Example",
			SeriesName:    "An Example",
		},
		{
			LibraryItemID: "book4",
			Title:         "Chronicles",
			SeriesName:    "Chronicles",
		},
	}

	groups := GroupBySeries(books)

	if len(groups) != 4 {
		t.Fatalf("expected 4 series groups, got %d", len(groups))
	}

	// Expected order: Chronicles, Example, Expanse, Series (sorted by stripped sort key)
	expectedOrder := []struct {
		name    string
		sortKey string
	}{
		{"Chronicles", "Chronicles"},
		{"An Example", "Example"},
		{"The Expanse", "Expanse"},
		{"A Series of Events", "Series of Events"},
	}

	for i, expected := range expectedOrder {
		if groups[i].Name != expected.name {
			t.Errorf("group %d: expected name %q, got %q", i, expected.name, groups[i].Name)
		}
		if groups[i].SortKey != expected.sortKey {
			t.Errorf("group %d: expected SortKey %q, got %q", i, expected.sortKey, groups[i].SortKey)
		}
	}
}
