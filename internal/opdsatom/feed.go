// Package atom builds an OPDS 1.x Atom/XML catalog feed out of the scanned
// book catalog, for clients (e.g. e-readers using Expat-based XML parsers)
// that don't understand OPDS 2.0 JSON.
package opdsatom

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/pagination"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

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

type AlphaRangeWrapper struct {
	Range *bookmeta.AlphaRange
}

func (w *AlphaRangeWrapper) GetSortKey() string {
	return w.Range.ID
}

func (w *AlphaRangeWrapper) GetLeafCount() int {
	return w.Range.LeafCount
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

// BuildNavigationFeed renders the catalog root as an OPDS 1.x navigation
// feed: one entry per author, each linking to the acquisition feed built by
// BuildFeed for that author's books. baseURL must not have a trailing
// slash, e.g. "https://example.com".
func BuildNavigationFeed(baseURL, title string, groups []bookmeta.AuthorGroup, updatedAt time.Time) Feed {
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
func BuildFeed(baseURL, selfPath, title string, books []model.Book, page int, updatedAt time.Time) Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, title, updatedAt)

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + selfPath, Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + selfPath + pageQueryString(page-1), Type: MediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + selfPath + pageQueryString(page+1), Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + selfPath + pageQueryString(totalPages), Type: MediaType})
		}
	}

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
func BuildRootFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	feed := newFeed(baseURL, "/opds", NavigationMediaType, title, updatedAt)
	feed.Entries = make([]Entry, 3)
	feed.Entries[0] = Entry{
		Title:   fmt.Sprintf("Find books by author (%d)", len(bookmeta.GroupByAuthor(books))),
		ID:      "urn:opds:browse:authors",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/authors", Type: NavigationMediaType},
		},
	}
	feed.Entries[1] = Entry{
		Title:   fmt.Sprintf("Find books by title (%d)", len(bookmeta.GroupByTitle(books))),
		ID:      "urn:opds:browse:titles",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/titles", Type: NavigationMediaType},
		},
	}
	feed.Entries[2] = Entry{
		Title:   fmt.Sprintf("Find books by series (%d)", len(bookmeta.GroupBySeries(books))),
		ID:      "urn:opds:browse:series",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/series", Type: NavigationMediaType},
		},
	}
	return feed
}

// BuildAuthorNavigationFeed renders a navigation feed of authors (or ranges).
// page is 1-indexed.
func BuildAuthorNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "authors")

	var items []bookmeta.AlphaRanged
	if len(ranges) == 1 && ranges[0].ID == "all" {
		items = make([]bookmeta.AlphaRanged, len(groups))
		for i := range groups {
			items[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
		}
	} else {
		items = make([]bookmeta.AlphaRanged, len(ranges))
		for i := range ranges {
			items[i] = &AlphaRangeWrapper{Range: &ranges[i]}
		}
	}

	pageItems, _, totalPages := pagination.Paginate(items, page)

	feed := newFeed(baseURL, "/opds/authors"+pageQueryString(page), NavigationMediaType, "Authors", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/authors", Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/authors" + pageQueryString(page-1), Type: NavigationMediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/authors" + pageQueryString(page+1), Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/authors" + pageQueryString(totalPages), Type: NavigationMediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageItems))
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, item := range pageItems {
			group := item.(*bookmeta.AuthorRangeAdapter).Group
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", group.DisplayName, len(group.Books)),
				ID:      "urn:opds:authors:" + group.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/authors/" + group.ID, Type: MediaType},
				},
			})
		}
	} else {
		for _, item := range pageItems {
			wrapper := item.(*AlphaRangeWrapper)
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", wrapper.Range.Label, wrapper.Range.LeafCount),
				ID:      "urn:opds:authors:range:" + wrapper.Range.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/authors/" + wrapper.Range.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildAuthorRangeFeed renders authors in a specific alphabetical range.
// page is 1-indexed.
func BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "authors")
	rangeItems := grouped[rangeID]

	pageItems, _, totalPages := pagination.Paginate(rangeItems, page)

	feed := newFeed(baseURL, "/opds/authors/"+rangeID+pageQueryString(page), NavigationMediaType, "Authors", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/authors", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/authors/" + rangeID, Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/authors/" + rangeID + pageQueryString(page-1), Type: NavigationMediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/authors/" + rangeID + pageQueryString(page+1), Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/authors/" + rangeID + pageQueryString(totalPages), Type: NavigationMediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageItems))
	for _, adapter := range pageItems {
		group := adapter.(*bookmeta.AuthorRangeAdapter).Group
		feed.Entries = append(feed.Entries, Entry{
			Title:   fmt.Sprintf("%s (%d)", group.DisplayName, len(group.Books)),
			ID:      "urn:opds:authors:" + group.ID,
			Updated: feed.Updated,
			Links: []Link{
				{Rel: "subsection", Href: baseURL + "/opds/authors/" + group.ID, Type: MediaType},
			},
		})
	}
	return feed
}

// BuildTitleNavigationFeed renders a navigation feed of titles (or ranges).
// page is 1-indexed.
func BuildTitleNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByTitle(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "titles")

	var items []bookmeta.AlphaRanged
	if len(ranges) == 1 && ranges[0].ID == "all" {
		items = make([]bookmeta.AlphaRanged, len(groups))
		for i := range groups {
			items[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
		}
	} else {
		items = make([]bookmeta.AlphaRanged, len(ranges))
		for i := range ranges {
			items[i] = &AlphaRangeWrapper{Range: &ranges[i]}
		}
	}

	pageItems, _, totalPages := pagination.Paginate(items, page)

	feed := newFeed(baseURL, "/opds/titles"+pageQueryString(page), NavigationMediaType, "Titles", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/titles", Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/titles" + pageQueryString(page-1), Type: NavigationMediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/titles" + pageQueryString(page+1), Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/titles" + pageQueryString(totalPages), Type: NavigationMediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageItems))
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, item := range pageItems {
			group := item.(*bookmeta.TitleRangeAdapter).Group
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", group.Title, len(group.Books)),
				ID:      "urn:opds:titles:" + fmt.Sprintf("%x", group.Title),
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/titles/" + fmt.Sprintf("%x", group.Title), Type: MediaType},
				},
			})
		}
	} else {
		for _, item := range pageItems {
			wrapper := item.(*AlphaRangeWrapper)
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", wrapper.Range.Label, wrapper.Range.LeafCount),
				ID:      "urn:opds:titles:range:" + wrapper.Range.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/titles/" + wrapper.Range.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildTitleRangeFeed renders all books in a specific alphabetical range as an acquisition feed.
// page is 1-indexed.
func BuildTitleRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByTitle(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "titles")
	rangeItems := grouped[rangeID]

	var booksInRange []model.Book
	for _, adapter := range rangeItems {
		group := adapter.(*bookmeta.TitleRangeAdapter).Group
		booksInRange = append(booksInRange, group.Books...)
	}

	pageBooks, _, totalPages := pagination.Paginate(booksInRange, page)

	feed := newFeed(baseURL, "/opds/titles/"+rangeID+pageQueryString(page), MediaType, "Titles", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/titles/" + rangeID, Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/titles/" + rangeID + pageQueryString(page-1), Type: MediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/titles/" + rangeID + pageQueryString(page+1), Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/titles/" + rangeID + pageQueryString(totalPages), Type: MediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildTitleFeed renders all books with a specific title as an acquisition feed.
// page is 1-indexed.
func BuildTitleFeed(baseURL, titleName string, books []model.Book, page int, updatedAt time.Time) Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/titles/" + fmt.Sprintf("%x", titleName)

	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, titleName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + selfPath, Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + selfPath + pageQueryString(page-1), Type: MediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + selfPath + pageQueryString(page+1), Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + selfPath + pageQueryString(totalPages), Type: MediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildSeriesNavigationFeed renders a navigation feed of series (or ranges).
// page is 1-indexed.
func BuildSeriesNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "series")

	var items []bookmeta.AlphaRanged
	if len(ranges) == 1 && ranges[0].ID == "all" {
		items = make([]bookmeta.AlphaRanged, len(groups))
		for i := range groups {
			items[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
		}
	} else {
		items = make([]bookmeta.AlphaRanged, len(ranges))
		for i := range ranges {
			items[i] = &AlphaRangeWrapper{Range: &ranges[i]}
		}
	}

	pageItems, _, totalPages := pagination.Paginate(items, page)

	feed := newFeed(baseURL, "/opds/series"+pageQueryString(page), NavigationMediaType, "Series", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/series", Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/series" + pageQueryString(page-1), Type: NavigationMediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/series" + pageQueryString(page+1), Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/series" + pageQueryString(totalPages), Type: NavigationMediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageItems))
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, item := range pageItems {
			group := item.(*bookmeta.SeriesRangeAdapter).Group
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", group.Name, len(group.Books)),
				ID:      "urn:opds:series:" + group.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/series/" + group.ID, Type: MediaType},
				},
			})
		}
	} else {
		for _, item := range pageItems {
			wrapper := item.(*AlphaRangeWrapper)
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", wrapper.Range.Label, wrapper.Range.LeafCount),
				ID:      "urn:opds:series:range:" + wrapper.Range.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/series/" + wrapper.Range.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildSeriesRangeFeed renders series in a specific alphabetical range.
// page is 1-indexed.
func BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "series")
	rangeItems := grouped[rangeID]

	pageItems, _, totalPages := pagination.Paginate(rangeItems, page)

	feed := newFeed(baseURL, "/opds/series/"+rangeID+pageQueryString(page), NavigationMediaType, "Series", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/series", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + "/opds/series/" + rangeID, Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + "/opds/series/" + rangeID + pageQueryString(page-1), Type: NavigationMediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + "/opds/series/" + rangeID + pageQueryString(page+1), Type: NavigationMediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + "/opds/series/" + rangeID + pageQueryString(totalPages), Type: NavigationMediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageItems))
	for _, adapter := range pageItems {
		group := adapter.(*bookmeta.SeriesRangeAdapter).Group
		feed.Entries = append(feed.Entries, Entry{
			Title:   fmt.Sprintf("%s (%d)", group.Name, len(group.Books)),
			ID:      "urn:opds:series:" + group.ID,
			Updated: feed.Updated,
			Links: []Link{
				{Rel: "subsection", Href: baseURL + "/opds/series/" + group.ID, Type: MediaType},
			},
		})
	}
	return feed
}

// BuildSeriesFeed renders all books in a specific series as an acquisition feed.
// page is 1-indexed.
func BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, page int, updatedAt time.Time) Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/series/" + seriesID

	feed := newFeed(baseURL, selfPath+pageQueryString(page), MediaType, seriesName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/series", Type: NavigationMediaType})

	if totalPages > 1 {
		if page > 1 {
			feed.Links = append(feed.Links, Link{Rel: "first", Href: baseURL + selfPath, Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "previous", Href: baseURL + selfPath + pageQueryString(page-1), Type: MediaType})
		}
		if page < totalPages {
			feed.Links = append(feed.Links, Link{Rel: "next", Href: baseURL + selfPath + pageQueryString(page+1), Type: MediaType})
			feed.Links = append(feed.Links, Link{Rel: "last", Href: baseURL + selfPath + pageQueryString(totalPages), Type: MediaType})
		}
	}

	feed.Entries = make([]Entry, 0, len(pageBooks))
	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Entries = append(feed.Entries, buildEntry(baseURL, b, feed.Updated, padding))
	}
	return feed
}
