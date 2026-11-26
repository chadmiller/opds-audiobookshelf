package bookmeta

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// CollectionGroup is one Audiobookshelf collection and its books.
type CollectionGroup struct {
	// ID is a URL-safe slug (used to build /opds/collections/{ID}).
	ID    string
	Name  string
	Books []model.Book
}

// GroupByCollection groups books by the collections they belong to, sorted
// by name. Books keep their input order within each group. Collections with
// no books in the catalog are omitted.
func GroupByCollection(books []model.Book) []CollectionGroup {
	byName := map[string]*CollectionGroup{}
	var names []string
	for _, b := range books {
		for _, name := range b.Collections {
			g, ok := byName[name]
			if !ok {
				g = &CollectionGroup{Name: name}
				byName[name] = g
				names = append(names, name)
			}
			g.Books = append(g.Books, b)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := strings.ToLower(stripArticles(names[i])), strings.ToLower(stripArticles(names[j]))
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})

	usedIDs := map[string]bool{}
	result := make([]CollectionGroup, 0, len(names))
	for _, name := range names {
		g := byName[name]
		base := authorSlug(name)
		g.ID = base
		for i := 2; usedIDs[g.ID]; i++ {
			g.ID = fmt.Sprintf("%s-%d", base, i)
		}
		usedIDs[g.ID] = true
		result = append(result, *g)
	}
	return result
}
