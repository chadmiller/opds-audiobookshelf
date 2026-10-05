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

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// FormatTitle returns the book title with series information prepended if
// available. Series prefix is formatted as "(Series Name #n)" with zero-padded
// numbers dynamically sized to fit the largest sequence number.
func FormatTitle(b model.Book, padding int) string {
	if b.SeriesName == "" || b.SeriesSequence == "" {
		return b.Title
	}
	paddedSeq := padSequence(b.SeriesSequence, padding)
	return fmt.Sprintf("(%s #%s) %s", b.SeriesName, paddedSeq, b.Title)
}

// MaxSequencePadding returns the number of digits needed to zero-pad the
// whole number part of all sequence numbers in the book list. Returns 0 if
// no books have sequences or all sequences are single digits.
func MaxSequencePadding(books []model.Book) int {
	maxWhole := 0
	for _, b := range books {
		if b.SeriesSequence == "" {
			continue
		}
		parts := strings.Split(b.SeriesSequence, ".")
		wholeLen := len(parts[0])
		if wholeLen > maxWhole {
			maxWhole = wholeLen
		}
	}
	return maxWhole
}

// padSequence zero-pads only the whole number part of a sequence to the
// given width. Trailing zeros in the fractional part are stripped, but
// leading zeros are preserved (e.g., "0.05" stays "0.05", not "0.5").
// Examples with width=3: "2" -> "002", "2.05" -> "002.05", "100.50" -> "100.5"
func padSequence(seq string, width int) string {
	if seq == "" || width == 0 {
		return seq
	}
	parts := strings.Split(seq, ".")
	whole := fmt.Sprintf("%0*s", width, parts[0])
	if len(parts) == 1 {
		return whole
	}
	frac := strings.TrimRight(parts[1], "0")
	if frac == "" {
		frac = "0"
	}
	return whole + "." + frac
}

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

// SortBooks orders books by author surname, author given name, formatted title
// (with series prefix), then publishedDate, matching how a physical catalog would be shelved.
func SortBooks(books []model.Book) {
	padding := MaxSequencePadding(books)
	sort.SliceStable(books, func(i, j int) bool { return less(books[i], books[j], padding) })
}

func less(a, b model.Book, padding int) bool {
	aSurname, aGiven := splitSurnameGiven(a.AuthorNamesLastFirst)
	bSurname, bGiven := splitSurnameGiven(b.AuthorNamesLastFirst)

	if al, bl := strings.ToLower(aSurname), strings.ToLower(bSurname); al != bl {
		return al < bl
	}
	if al, bl := strings.ToLower(aGiven), strings.ToLower(bGiven); al != bl {
		return al < bl
	}
	if al, bl := strings.ToLower(titleSortKey(a, padding)), strings.ToLower(titleSortKey(b, padding)); al != bl {
		return al < bl
	}
	return a.PublishedDate < b.PublishedDate
}

func titleSortKey(b model.Book, padding int) string {
	formatted := FormatTitle(b, padding)
	if b.TitleIgnorePrefix != "" && b.SeriesName == "" {
		return b.TitleIgnorePrefix
	}
	return formatted
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

// splitAuthors splits a semicolon-separated author string into individual
// author names, trimming whitespace.
func splitAuthors(authorList string) []string {
	if authorList == "" {
		return []string{unknownAuthor}
	}
	parts := strings.Split(authorList, "; ")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{unknownAuthor}
	}
	return result
}

// GroupByAuthor splits a book list already ordered by SortBooks into
// per-author groups, preserving that order both across and within groups.
// Books with multiple authors appear in each author's group.
func GroupByAuthor(books []model.Book) []AuthorGroup {
	authorToID := make(map[string]string)
	groups := make(map[string]*AuthorGroup)
	var order []string
	usedIDs := map[string]bool{}

	for _, b := range books {
		authorNames := splitAuthors(b.AuthorNamesLastFirst)
		for _, display := range authorNames {
			id, exists := authorToID[display]
			if !exists {
				base := authorSlug(display)
				id = base
				for i := 2; usedIDs[id]; i++ {
					id = fmt.Sprintf("%s-%d", base, i)
				}
				usedIDs[id] = true
				authorToID[display] = id

				groups[id] = &AuthorGroup{ID: id, DisplayName: display, Books: []model.Book{}}
				order = append(order, id)
			}
			groups[id].Books = append(groups[id].Books, b)
		}
	}

	var result []AuthorGroup
	for _, id := range order {
		result = append(result, *groups[id])
	}
	return result
}

// stripArticles removes leading English articles ("a", "an", "the") from a title.
func stripArticles(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			return strings.TrimSpace(title[len(article):])
		}
	}
	return title
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

// TitleGroup represents books grouped by title
type TitleGroup struct {
	Title   string
	SortKey string
	Books   []model.Book
}

// GroupByTitle splits books into groups by their Title (without series prefix),
// sorted by title (ignoring articles).
func GroupByTitle(books []model.Book) []TitleGroup {
	groups := make(map[string]*TitleGroup)

	for _, b := range books {
		title := b.Title
		if _, exists := groups[title]; !exists {
			sortKey := b.TitleIgnorePrefix
			if sortKey == "" {
				sortKey = stripArticles(title)
			}
			groups[title] = &TitleGroup{Title: title, SortKey: sortKey, Books: []model.Book{}}
		}
		groups[title].Books = append(groups[title].Books, b)
	}

	var result []TitleGroup
	for _, group := range groups {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].SortKey) < strings.ToLower(result[j].SortKey)
	})
	return result
}

// SeriesGroup represents books grouped by series
type SeriesGroup struct {
	ID    string
	Name  string
	Books []model.Book
}

// GroupBySeries splits books into groups by series name, sorted alphabetically.
// Books without a series are grouped under "Standalone".
func GroupBySeries(books []model.Book) []SeriesGroup {
	const standalone = "Standalone"
	groups := make(map[string]*SeriesGroup)
	var order []string
	usedIDs := map[string]bool{}

	for _, b := range books {
		seriesName := b.SeriesName
		if seriesName == "" {
			seriesName = standalone
		}

		if _, exists := groups[seriesName]; !exists {
			id := authorSlug(seriesName)
			for i := 2; usedIDs[id]; i++ {
				id = fmt.Sprintf("%s-%d", authorSlug(seriesName), i)
			}
			usedIDs[id] = true
			groups[seriesName] = &SeriesGroup{ID: id, Name: seriesName, Books: []model.Book{}}
			order = append(order, seriesName)
		}
		groups[seriesName].Books = append(groups[seriesName].Books, b)
	}

	sort.Slice(order, func(i, j int) bool {
		return strings.ToLower(order[i]) < strings.ToLower(order[j])
	})

	var result []SeriesGroup
	for _, name := range order {
		result = append(result, *groups[name])
	}
	return result
}

// AlphaRange represents a character range for browsing
type AlphaRange struct {
	ID        string
	Label     string
	LeafCount int
}

// AlphaRanged items can be grouped by alphabetical ranges
type AlphaRanged interface {
	GetSortKey() string
	GetLeafCount() int
}

// AuthorRangeAdapter makes AuthorGroup compatible with alphabetical grouping
type AuthorRangeAdapter struct {
	Group *AuthorGroup
}

func (a *AuthorRangeAdapter) GetSortKey() string { return a.Group.DisplayName }
func (a *AuthorRangeAdapter) GetLeafCount() int  { return len(a.Group.Books) }

// TitleRangeAdapter makes TitleGroup compatible with alphabetical grouping
type TitleRangeAdapter struct {
	Group *TitleGroup
}

func (t *TitleRangeAdapter) GetSortKey() string { return t.Group.SortKey }
func (t *TitleRangeAdapter) GetLeafCount() int  { return len(t.Group.Books) }

// SeriesRangeAdapter makes SeriesGroup compatible with alphabetical grouping
type SeriesRangeAdapter struct {
	Group *SeriesGroup
}

func (s *SeriesRangeAdapter) GetSortKey() string { return s.Group.Name }
func (s *SeriesRangeAdapter) GetLeafCount() int  { return len(s.Group.Books) }

// GroupByAlphaRange splits items into alphabetical ranges (a–c, d–g, etc.)
// if itemCount > threshold. Otherwise returns a single "all" range.
// The groupName (e.g., "authors", "titles", "series") is used to generate context-aware labels.
// The threshold is based on the number of items (authors/titles/series), not the number of books.
func GroupByAlphaRange(items []AlphaRanged, threshold int, groupName string) (ranges []AlphaRange, grouped map[string][]AlphaRanged) {
	grouped = make(map[string][]AlphaRanged)

	if len(items) <= threshold {
		totalLeaves := 0
		for _, item := range items {
			totalLeaves += item.GetLeafCount()
		}
		ranges = []AlphaRange{{ID: "all", Label: "All", LeafCount: totalLeaves}}
		grouped["all"] = items
		return
	}

	rangeConfigs := []struct {
		id      string
		idRange string
		check   func(r rune) bool
	}{
		{"range/a-c", "a–c", func(r rune) bool { return r >= 'a' && r <= 'c' }},
		{"range/d-g", "d–g", func(r rune) bool { return r >= 'd' && r <= 'g' }},
		{"range/h-k", "h–k", func(r rune) bool { return r >= 'h' && r <= 'k' }},
		{"range/l-o", "l–o", func(r rune) bool { return r >= 'l' && r <= 'o' }},
		{"range/p-r", "p–r", func(r rune) bool { return r >= 'p' && r <= 'r' }},
		{"range/s-t", "s–t", func(r rune) bool { return r >= 's' && r <= 't' }},
		{"range/u-z", "u–z", func(r rune) bool { return r >= 't' && r <= 'z' }},
		{"range/0-9", "0–9", func(r rune) bool { return r >= '0' && r <= '9' }},
		{"range/other", "other", func(r rune) bool { return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) }},
	}

	for _, rc := range rangeConfigs {
		var itemsInRange []AlphaRanged
		for _, item := range items {
			key := strings.ToLower(item.GetSortKey())
			if len(key) > 0 && rc.check(rune(key[0])) {
				itemsInRange = append(itemsInRange, item)
			}
		}
		if len(itemsInRange) > 0 {
			sort.Slice(itemsInRange, func(i, j int) bool {
				return strings.ToLower(itemsInRange[i].GetSortKey()) < strings.ToLower(itemsInRange[j].GetSortKey())
			})
			leafCount := 0
			for _, item := range itemsInRange {
				leafCount += item.GetLeafCount()
			}
			label := groupName + " starting " + rc.idRange
			ranges = append(ranges, AlphaRange{ID: rc.id, Label: label, LeafCount: leafCount})
			grouped[rc.id] = itemsInRange
		}
	}

	return
}
