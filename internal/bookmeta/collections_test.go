package bookmeta

import (
	"testing"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

func TestGroupByCollection(t *testing.T) {
	books := []model.Book{
		{LibraryItemID: "1", Collections: []string{"The Zoo", "Favorites"}},
		{LibraryItemID: "2", Collections: []string{"Favorites"}},
		{LibraryItemID: "3"},
	}
	got := GroupByCollection(books)
	if len(got) != 2 || got[0].Name != "Favorites" || got[1].Name != "The Zoo" {
		t.Fatalf("unexpected groups: %+v", got)
	}
	if len(got[0].Books) != 2 || got[0].ID != "favorites" || got[1].ID != "the-zoo" {
		t.Fatalf("unexpected contents: %+v", got)
	}
}
