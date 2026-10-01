// Package model holds the in-memory representation of a catalog entry
// scanned out of the Audiobookshelf sqlite database.
package model

// EbookFile mirrors the JSON shape Audiobookshelf stores in books.ebookFile.
type EbookFile struct {
	Ino      string `json:"ino"`
	Metadata struct {
		Filename string `json:"filename"`
		Ext      string `json:"ext"`
		Path     string `json:"path"`
		RelPath  string `json:"relPath"`
		Size     int64  `json:"size"`
	} `json:"metadata"`
	EbookFormat string `json:"ebookFormat"`
}

// Book is a single ebook entry ready to be rendered into an OPDS publication.
type Book struct {
	LibraryItemID string
	Title         string
	Subtitle      string
	PublishedYear string
	PublishedDate string
	Publisher     string
	Description   string
	ISBN          string
	ASIN          string
	Language      string
	AuthorName    string
	// AuthorNamesLastFirst is libraryItems.authorNamesLastFirst, e.g.
	// "Weir, Andy" — Audiobookshelf's own "surname, given name" ordering,
	// used as the primary sort key.
	AuthorNamesLastFirst string
	// TitleIgnorePrefix is books.titleIgnorePrefix, the title with any
	// leading article ("The", "A", ...) stripped for sorting purposes.
	// Empty if Audiobookshelf didn't record one, in which case Title is
	// used for sorting instead.
	TitleIgnorePrefix string
	Narrators         []string
	Tags              []string
	Genres            []string

	// CoverPath is the container-rooted absolute path to the cover image,
	// or empty if the book has no cover.
	CoverPath string

	// CoverResolvable reports whether CoverPath actually exists on disk once
	// re-rooted onto -files-root. When false (no CoverPath, or the file is
	// missing), the cover endpoint serves the placeholder image instead.
	CoverResolvable bool

	// Ebook is the container-rooted file the reader should download.
	Ebook EbookFile

	// ItemPath and ItemRelPath are libraryItems.path and libraryItems.relPath.
	// ItemPath always ends with ItemRelPath; the difference is the
	// container-side path of the library folder this item lives under,
	// which is what -files-root stands in for on the host.
	ItemPath    string
	ItemRelPath string
}
