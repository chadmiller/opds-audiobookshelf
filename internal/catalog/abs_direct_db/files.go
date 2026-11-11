package abs_direct_db

import (
	"net/http"
	"os"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/pathmap"
)

func (c Client) CoverExists(book model.Book) bool {
	if book.CoverPath == "" {
		return false
	}
	info, err := os.Stat(pathmap.HostPath(c.FilesRoot, book, book.CoverPath))
	return err == nil && !info.IsDir()
}

func (c Client) ServeEbook(w http.ResponseWriter, r *http.Request, book model.Book) error {
	http.ServeFile(w, r, pathmap.HostPath(c.FilesRoot, book, book.Ebook.Metadata.Path))
	return nil
}

func (c Client) ServeCover(w http.ResponseWriter, r *http.Request, book model.Book) bool {
	if !c.CoverExists(book) {
		return false
	}
	http.ServeFile(w, r, pathmap.HostPath(c.FilesRoot, book, book.CoverPath))
	return true
}
