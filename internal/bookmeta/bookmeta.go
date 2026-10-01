// Package bookmeta holds catalog-format-agnostic derivations from
// model.Book (identifiers, MIME types, normalized dates) shared by the
// OPDS 2.0 JSON renderer and the OPDS 1.x Atom/XML renderer.
package bookmeta

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chadmiller/audiobookshelf-opds-server/internal/model"
)

// Identifier returns a stable URN for the book: its ISBN or ASIN if known,
// else a URN built from its library item ID.
func Identifier(b model.Book) string {
	switch {
	case b.ISBN != "":
		return "urn:isbn:" + b.ISBN
	case b.ASIN != "":
		return "urn:asin:" + b.ASIN
	default:
		return "urn:uuid:" + b.LibraryItemID
	}
}

// EbookMimeType returns the MIME type of the book's ebook file, derived
// from its Audiobookshelf-recorded format (falling back to its extension).
func EbookMimeType(b model.Book) string {
	f := strings.ToLower(strings.TrimPrefix(b.Ebook.EbookFormat, "."))
	if f == "" {
		f = strings.ToLower(strings.TrimPrefix(b.Ebook.Metadata.Ext, "."))
	}
	switch f {
	case "epub":
		return "application/epub+zip"
	case "pdf":
		return "application/pdf"
	case "mobi":
		return "application/x-mobipocket-ebook"
	case "azw3", "azw":
		return "application/vnd.amazon.ebook"
	case "cbz":
		return "application/vnd.comicbook+zip"
	case "cbr":
		return "application/vnd.comicbook-rar"
	default:
		return "application/octet-stream"
	}
}

// CoverMimeType returns the MIME type of a book's resolvable cover image,
// guessed from its file extension.
func CoverMimeType(coverPath string) string {
	switch strings.ToLower(filepath.Ext(coverPath)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

// Published turns Audiobookshelf's free-form publishedDate/publishedYear
// strings into an RFC 3339 full-date ("YYYY-MM-DD"), or "" if nothing
// usable is available.
func Published(b model.Book) string {
	if b.PublishedDate != "" {
		if t, err := time.Parse("2006-01-02", b.PublishedDate); err == nil {
			return t.Format("2006-01-02")
		}
		if t, err := time.Parse(time.RFC3339, b.PublishedDate); err == nil {
			return t.Format(time.RFC3339)
		}
		if year, err := strconv.Atoi(b.PublishedDate); err == nil {
			return fmt.Sprintf("%04d-01-01", year)
		}
	}
	if year, err := strconv.Atoi(b.PublishedYear); err == nil {
		return fmt.Sprintf("%04d-01-01", year)
	}
	return ""
}

// SortBooks orders books by author surname, author given name, title, then
// publishedDate, matching how a physical catalog would be shelved.
func SortBooks(books []model.Book) {
	sort.SliceStable(books, func(i, j int) bool { return less(books[i], books[j]) })
}

func less(a, b model.Book) bool {
	aSurname, aGiven := splitSurnameGiven(a.AuthorNamesLastFirst)
	bSurname, bGiven := splitSurnameGiven(b.AuthorNamesLastFirst)

	if al, bl := strings.ToLower(aSurname), strings.ToLower(bSurname); al != bl {
		return al < bl
	}
	if al, bl := strings.ToLower(aGiven), strings.ToLower(bGiven); al != bl {
		return al < bl
	}
	if al, bl := strings.ToLower(titleSortKey(a)), strings.ToLower(titleSortKey(b)); al != bl {
		return al < bl
	}
	return a.PublishedDate < b.PublishedDate
}

func titleSortKey(b model.Book) string {
	if b.TitleIgnorePrefix != "" {
		return b.TitleIgnorePrefix
	}
	return b.Title
}

// splitSurnameGiven splits Audiobookshelf's "Surname, Given Name" format
// (libraryItems.authorNamesLastFirst) into its two parts.
func splitSurnameGiven(lastFirst string) (surname, given string) {
	surname, given, found := strings.Cut(lastFirst, ",")
	if !found {
		return strings.TrimSpace(lastFirst), ""
	}
	return strings.TrimSpace(surname), strings.TrimSpace(given)
}

const unknownAuthor = "Unknown Author"

// AuthorGroup is one author's books, used to render the two-level
// author -> works catalog hierarchy.
type AuthorGroup struct {
	// ID is a URL-safe slug identifying this author, stable for the
	// lifetime of one scan (used to build /opds/authors/{ID}).
	ID string
	// DisplayName is Audiobookshelf's "Surname, Given Name" string, or
	// "Unknown Author" if the book(s) have no author on file.
	DisplayName string
	Books       []model.Book
}

// GroupByAuthor splits a book list already ordered by SortBooks into
// per-author groups, preserving that order both across and within groups.
func GroupByAuthor(books []model.Book) []AuthorGroup {
	var groups []AuthorGroup
	usedIDs := map[string]bool{}

	for _, b := range books {
		display := b.AuthorNamesLastFirst
		if display == "" {
			display = unknownAuthor
		}
		if n := len(groups); n > 0 && groups[n-1].DisplayName == display {
			groups[n-1].Books = append(groups[n-1].Books, b)
			continue
		}

		base := authorSlug(display)
		id := base
		for i := 2; usedIDs[id]; i++ {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		usedIDs[id] = true

		groups = append(groups, AuthorGroup{ID: id, DisplayName: display, Books: []model.Book{b}})
	}
	return groups
}

// authorSlug lowercases name and collapses every run of non-alphanumeric
// characters into a single hyphen, for use as a URL path segment.
func authorSlug(name string) string {
	var b strings.Builder
	needDash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if needDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			needDash = false
		} else {
			needDash = true
		}
	}
	if b.Len() == 0 {
		return "author"
	}
	return b.String()
}
