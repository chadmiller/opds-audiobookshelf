package catalog

import (
	"net/http"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// Source produces the current set of ebooks in the named Audiobookshelf
// libraries. Implementations read either Audiobookshelf's database directly
// or its HTTP API.
type Source interface {
	Scan(libraryNames []string) ([]model.Book, error)
}

// Files gives access to the bytes of the books a Source listed. Disk-backed
// implementations read them from a mounted copy of the library; API-backed
// ones proxy to Audiobookshelf.
type Files interface {
	// CoverExists reports whether book has a cover that ServeCover can serve.
	CoverExists(book model.Book) bool
	// ServeEbook writes the book's ebook file as the response to r.
	ServeEbook(w http.ResponseWriter, r *http.Request, book model.Book) error
	// ServeCover writes the book's cover as the response to r. It returns
	// false, having written nothing, if there is no cover to serve.
	ServeCover(w http.ResponseWriter, r *http.Request, book model.Book) (served bool)
}

// Backend is a Source together with the Files for the books it lists.
type Backend interface {
	Source
	Files
}
