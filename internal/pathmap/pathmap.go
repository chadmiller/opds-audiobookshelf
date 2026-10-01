// Package pathmap re-roots the container-side absolute paths recorded in
// Audiobookshelf's database onto this process's view of the filesystem.
package pathmap

import (
	"path/filepath"
	"strings"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// HostPath re-roots containerPath (an absolute path as Audiobookshelf
// recorded it, e.g. book.CoverPath or book.Ebook.Metadata.Path) onto
// filesRoot. Audiobookshelf records libraryItems.path as the library
// folder's container-side path joined with libraryItems.relPath; stripping
// that same relPath suffix off libraryItems.path yields the library
// folder's container-side root, which filesRoot stands in for on the host.
func HostPath(filesRoot string, book model.Book, containerPath string) string {
	libraryRoot := strings.TrimSuffix(book.ItemPath, book.ItemRelPath)
	rel := strings.TrimPrefix(containerPath, libraryRoot)
	return filepath.Join(filesRoot, rel)
}
