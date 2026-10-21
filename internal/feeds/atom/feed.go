// Package atom builds an OPDS 1.x Atom/XML catalog feed out of the scanned
// book catalog, for clients (e.g. e-readers using Expat-based XML parsers)
// that don't understand OPDS 2.0 JSON.
package opdsatom

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/feeds"
	"github.com/chadmiller/opds-audiobookshelf/internal/pagination"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

// AtomFeed builds OPDS 1.x Atom/XML feeds. It is stateless; negotiation
// hands back an AtomFeed value once per request, and callers call its
// methods directly without needing to know they're talking to this package.
type AtomFeed struct{}

// MediaType is the media type of an OPDS 1.x acquisition feed.
const MediaType = "application/atom+xml;profile=opds-catalog;kind=acquisition"

// NavigationMediaType is the media type of an OPDS 1.x navigation feed.
const NavigationMediaType = "application/atom+xml;profile=opds-catalog;kind=navigation"

const atomNS = "http://www.w3.org/2005/Atom"
const dctermsNS = "http://purl.org/dc/terms/"

func pageQueryString(page int) string {
	if page <= 1 {
		return ""
	}
	return fmt.Sprintf("?page=%d", page)
}

type Feed struct {
	XMLName xml.Name `xml:"feed"`
	Xmlns   string   `xml:"xmlns,attr"`
	XmlnsDC string   `xml:"xmlns:dcterms,attr"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Links   []Link   `xml:"link"`
	Entries []Entry  `xml:"entry"`
}

type Link struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr,omitempty"`
}

type Author struct {
	Name string `xml:"name"`
}

type Summary struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type Entry struct {
	Title   string   `xml:"title"`
	ID      string   `xml:"id"`
	Updated string   `xml:"updated"`
	Authors []Author `xml:"author"`
	Summary *Summary `xml:"summary,omitempty"`
	Issued  string   `xml:"dcterms:issued,omitempty"`
	Links   []Link   `xml:"link"`
}

// ContentType reports this feed's own media type, taken from its self link
// (set by newFeed), satisfying feeds.Feed.
func (f Feed) ContentType() string {
	for _, l := range f.Links {
		if l.Rel == "self" {
			return l.Type
		}
	}
	return MediaType
}

// Encode writes f to w as indented XML with a standard XML header,
// satisfying feeds.Feed.
func (f Feed) Encode(w io.Writer) error {
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	return enc.Encode(f)
}

// formatUpdated renders updatedAt as the RFC 3339 timestamp Atom's
// <updated> element requires, falling back to the Unix epoch if the
// catalog hasn't completed a scan yet.
func formatUpdated(updatedAt time.Time) string {
	if updatedAt.IsZero() {
		return time.Unix(0, 0).UTC().Format(time.RFC3339)
	}
	return updatedAt.UTC().Format(time.RFC3339)
}

// newFeed builds the common feed header (id/title/updated, self + start
// links) shared by both the navigation and acquisition feeds. selfPath is
// this feed's own path, e.g. "/opds" or "/opds/authors/weir-andy"; selfType
// declares whether this feed itself is a navigation or acquisition feed.
func newFeed(baseURL, selfPath, selfType, title string, updatedAt time.Time) Feed {
	return Feed{
		Xmlns:   atomNS,
		XmlnsDC: dctermsNS,
		ID:      baseURL + selfPath,
		Title:   title,
		Updated: formatUpdated(updatedAt),
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath, Type: selfType},
			{Rel: "start", Href: baseURL + "/opds", Type: NavigationMediaType},
		},
	}
}

// paginationLinks returns the first/previous/next/last links for a feed with
// totalPages pages, currently showing page (1-indexed) at selfPath. Returns
// nil if there's only one page.
func paginationLinks(baseURL, selfPath string, page, totalPages int, mediaType string) []Link {
	var links []Link
	if totalPages <= 1 {
		return links
	}
	if page > 1 {
		links = append(links, Link{Rel: "first", Href: baseURL + selfPath, Type: mediaType})
		links = append(links, Link{Rel: "previous", Href: baseURL + selfPath + pageQueryString(page-1), Type: mediaType})
	}
	if page < totalPages {
		links = append(links, Link{Rel: "next", Href: baseURL + selfPath + pageQueryString(page+1), Type: mediaType})
		links = append(links, Link{Rel: "last", Href: baseURL + selfPath + pageQueryString(totalPages), Type: mediaType})
	}
	return links
}

// buildCategoryItemEntry renders the listing entry for one author/title/
// series group: its own subsection link into that group's acquisition feed.
// Shared by buildCategoryNavigationFeed's "all" case and
// buildCategoryRangeFeed.
func buildCategoryItemEntry(baseURL string, cat bookmeta.Category, item bookmeta.CategoryItem, updatedStr string) Entry {
	return Entry{
		Title:   fmt.Sprintf("%s (%d)", item.ItemLabel(), item.GetLeafCount()),
		ID:      cat.URNPrefix + item.ItemID(),
		Updated: updatedStr,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/" + cat.PathSegment + "/" + item.ItemID(), Type: MediaType},
		},
	}
}

// buildRangeEntry renders the listing entry for one alphabetical range
// bucket of a category's groups (e.g. "authors starting a-c"). Shared by
// buildCategoryNavigationFeed.
func buildRangeEntry(baseURL string, cat bookmeta.Category, r bookmeta.AlphaRange, updatedStr string) Entry {
	return Entry{
		Title:   fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
		ID:      cat.URNPrefix + "range:" + r.ID,
		Updated: updatedStr,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/" + cat.PathSegment + "/" + r.ID, Type: NavigationMediaType},
		},
	}
}

// buildCategoryNavigationFeed renders a navigation feed listing every group
// in a category (author, title, or series) — or, once there are more than
// bookmeta.RangeGroupThreshold of them, alphabetical ranges of those groups
// instead. This is the single implementation shared by
// BuildAuthorNavigationFeed, BuildTitleNavigationFeed, and
// BuildSeriesNavigationFeed, which differ only in which groups they list.
// page is 1-indexed.
func buildCategoryNavigationFeed(baseURL string, cat bookmeta.Category, items []bookmeta.CategoryItem, page int, updatedAt time.Time) feeds.Feed {
	adapters := make([]bookmeta.AlphaRanged, len(items))
	for i, it := range items {
		adapters[i] = it
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, bookmeta.RangeGroupThreshold, cat.PathSegment)

	basePath := "/opds/" + cat.PathSegment + "/"
	feed := newFeed(baseURL, basePath+pageQueryString(page), NavigationMediaType, cat.Title, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	if len(ranges) == 1 && ranges[0].ID == "all" {
		pageItems, _, totalPages := pagination.Paginate(items, page)
		feed.Links = append(feed.Links, paginationLinks(baseURL, basePath, page, totalPages, NavigationMediaType)...)
		feed.Entries = make([]Entry, 0, len(pageItems))
		for _, item := range pageItems {
			feed.Entries = append(feed.Entries, buildCategoryItemEntry(baseURL, cat, item, feed.Updated))
		}
		return feed
	}

	pageRanges, _, totalPages := pagination.Paginate(ranges, page)
	feed.Links = append(feed.Links, paginationLinks(baseURL, basePath, page, totalPages, NavigationMediaType)...)
	feed.Entries = make([]Entry, 0, len(pageRanges))
	for _, r := range pageRanges {
		feed.Entries = append(feed.Entries, buildRangeEntry(baseURL, cat, r, feed.Updated))
	}
	return feed
}

// buildCategoryRangeFeed renders the groups (authors or series) within one
// alphabetical range bucket, e.g. "/opds/authors/range/a-c". This is the
// single implementation shared by BuildAuthorRangeFeed and
// BuildSeriesRangeFeed. (Title's range feed lists books directly instead of
// title-groups, since GroupByTitle's groups are exact-title buckets rather
// than meaningfully-sized collections worth clicking into — see
// BuildTitleRangeFeed.) page is 1-indexed.
func buildCategoryRangeFeed(baseURL string, cat bookmeta.Category, items []bookmeta.CategoryItem, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	adapters := make([]bookmeta.AlphaRanged, len(items))
	for i, it := range items {
		adapters[i] = it
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, bookmeta.RangeGroupThreshold, cat.PathSegment)
	rangeItems := grouped[rangeID]

	pageItems, _, totalPages := pagination.Paginate(rangeItems, page)

	basePath := "/opds/" + cat.PathSegment + "/"
	selfPath := basePath + rangeID
	feed := newFeed(baseURL, selfPath+pageQueryString(page), NavigationMediaType, cat.Title, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + basePath, Type: NavigationMediaType})
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, NavigationMediaType)...)

	feed.Entries = make([]Entry, 0, len(pageItems))
	for _, item := range pageItems {
		feed.Entries = append(feed.Entries, buildCategoryItemEntry(baseURL, cat, item.(bookmeta.CategoryItem), feed.Updated))
	}
	return feed
}

// BuildNavigationFeed renders the catalog root as an OPDS 1.x navigation
// feed: one entry per author, each linking to the acquisition feed built by
// BuildAuthorFeed for that author's books. baseURL must not have a trailing
// slash, e.g. "https://example.com".
func (AtomFeed) BuildNavigationFeed(baseURL, title string, groups []bookmeta.AuthorGroup, updatedAt time.Time) feeds.Feed {
	feed := newFeed(baseURL, "/opds", NavigationMediaType, title, updatedAt)
	feed.Entries = make([]Entry, 0, len(groups))
	for _, g := range groups {
		feed.Entries = append(feed.Entries, Entry{
			Title:   g.DisplayName,
			ID:      "urn:opds:authors:" + g.ID,
			Updated: feed.Updated,
			Links: []Link{
				{Rel: "subsection", Href: baseURL + "/opds/authors/" + g.ID, Type: NavigationMediaType},
			},
		})
	}
	return feed
}

// BuildFeed renders one author's books as a single-page OPDS 1.x Atom
// acquisition catalog. baseURL must not have a trailing slash, e.g.
// "https://example.com". selfPath is this feed's own path, e.g.
// "/opds/authors/weir-andy". page is 1-indexed.
func (AtomFeed) BuildAuthorFeed(baseURL, selfPath, title string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, title, updatedAt)
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntry(baseURL, b, feed.Updated, padding))
	}
	return feed
}

func buildEntry(baseURL string, b model.Book, updatedStr string, padding int) Entry {
	return buildEntryWithSeriesPrefix(baseURL, b, updatedStr, padding, true)
}

func buildEntryWithSeriesPrefix(baseURL string, b model.Book, updatedStr string, padding int, includeSeriesPrefix bool) Entry {
	title := b.Title
	if includeSeriesPrefix {
		title = bookmeta.FormatTitle(b, padding)
	}
	entry := Entry{
		Title:   title,
		ID:      bookmeta.Identifier(b),
		Updated: updatedStr,
		Issued:  bookmeta.Published(b),
		Links: []Link{
			{
				Rel:  "http://opds-spec.org/acquisition",
				Href: baseURL + "/download/" + b.LibraryItemID,
				Type: bookmeta.EbookMimeType(b),
			},
		},
	}
	if b.AuthorName != "" {
		entry.Authors = []Author{{Name: b.AuthorName}}
	}
	if b.Description != "" {
		entry.Summary = &Summary{Type: "text", Text: b.Description}
	}

	coverType := placeholder.MimeType
	if b.CoverResolvable {
		coverType = bookmeta.CoverMimeType(b.CoverPath)
	}
	entry.Links = append(entry.Links, Link{
		Rel:  "http://opds-spec.org/image",
		Href: baseURL + "/covers/" + b.LibraryItemID,
		Type: coverType,
	})

	return entry
}

// BuildRootFeed renders the root catalog as a selection menu with three browse options.
func (AtomFeed) BuildRootFeed(baseURL, title string, books []model.Book, updatedAt time.Time) feeds.Feed {
	feed := newFeed(baseURL, "/opds", NavigationMediaType, title, updatedAt)
	feed.Entries = make([]Entry, 3)
	feed.Entries[0] = Entry{
		Title:   fmt.Sprintf("Find books by author (%d)", len(bookmeta.GroupByAuthor(books))),
		ID:      "urn:opds:browse:authors",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/authors/", Type: NavigationMediaType},
		},
	}
	feed.Entries[1] = Entry{
		Title:   fmt.Sprintf("Find books by title (%d)", len(bookmeta.GroupByTitle(books))),
		ID:      "urn:opds:browse:titles",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/titles/", Type: NavigationMediaType},
		},
	}
	feed.Entries[2] = Entry{
		Title:   fmt.Sprintf("Find books by series (%d)", len(bookmeta.GroupBySeries(books))),
		ID:      "urn:opds:browse:series",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/series/", Type: NavigationMediaType},
		},
	}
	return feed
}

// BuildAuthorNavigationFeed renders a navigation feed of authors (or ranges).
// page is 1-indexed.
func (AtomFeed) BuildAuthorNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.AuthorCategory, bookmeta.AuthorCategoryItems(books), page, updatedAt)
}

// BuildAuthorRangeFeed renders authors in a specific alphabetical range.
// page is 1-indexed.
func (AtomFeed) BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryRangeFeed(baseURL, bookmeta.AuthorCategory, bookmeta.AuthorCategoryItems(books), rangeID, page, updatedAt)
}

// BuildTitleNavigationFeed renders a navigation feed of titles (or ranges).
// page is 1-indexed.
func (AtomFeed) BuildTitleNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.TitleCategory, bookmeta.TitleCategoryItems(books), page, updatedAt)
}

// BuildTitleRangeFeed renders all books in a specific alphabetical range as an acquisition feed.
// page is 1-indexed.
func (AtomFeed) BuildTitleRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	groups := bookmeta.GroupByTitle(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, bookmeta.RangeGroupThreshold, "titles")
	rangeItems := grouped[rangeID]

	var booksInRange []model.Book
	for _, adapter := range rangeItems {
		group := adapter.(*bookmeta.TitleRangeAdapter).Group
		booksInRange = append(booksInRange, group.Books...)
	}

	pageBooks, _, totalPages := pagination.Paginate(booksInRange, page)

	selfPath := "/opds/titles/" + rangeID
	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, "Titles", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles/", Type: NavigationMediaType})
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildTitleFeed renders all books with a specific title as an acquisition feed.
// page is 1-indexed.
func (AtomFeed) BuildTitleFeed(baseURL, titleName string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/titles/" + fmt.Sprintf("%x", titleName)

	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, titleName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles/", Type: NavigationMediaType})
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildSeriesNavigationFeed renders a navigation feed of series (or ranges).
// page is 1-indexed.
func (AtomFeed) BuildSeriesNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.SeriesCategory, bookmeta.SeriesCategoryItems(books), page, updatedAt)
}

// BuildSeriesRangeFeed renders series in a specific alphabetical range.
// page is 1-indexed.
func (AtomFeed) BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryRangeFeed(baseURL, bookmeta.SeriesCategory, bookmeta.SeriesCategoryItems(books), rangeID, page, updatedAt)
}

// BuildSeriesFeed renders all books in a specific series as an acquisition feed.
// page is 1-indexed.
func (AtomFeed) BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/series/" + seriesID

	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, seriesName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/series/", Type: NavigationMediaType})
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntry(baseURL, b, feed.Updated, padding))
	}
	return feed
}
