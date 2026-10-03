// Package atom builds an OPDS 1.x Atom/XML catalog feed out of the scanned
// book catalog, for clients (e.g. e-readers using Expat-based XML parsers)
// that don't understand OPDS 2.0 JSON.
package atom

import (
	"encoding/xml"
	"fmt"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

// MediaType is the media type of an OPDS 1.x acquisition feed.
const MediaType = "application/atom+xml;profile=opds-catalog;kind=acquisition"

// NavigationMediaType is the media type of an OPDS 1.x navigation feed.
const NavigationMediaType = "application/atom+xml;profile=opds-catalog;kind=navigation"

const atomNS = "http://www.w3.org/2005/Atom"
const dctermsNS = "http://purl.org/dc/terms/"

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
// "/opds/authors/weir-andy".
func BuildFeed(baseURL, selfPath, title string, books []model.Book, updatedAt time.Time) Feed {
	feed := newFeed(baseURL, selfPath, MediaType, title, updatedAt)
	feed.Entries = make([]Entry, 0, len(books))
	padding := bookmeta.MaxSequencePadding(books)
	for _, b := range books {
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
		Title:   fmt.Sprintf("Find books by author (%d)", len(books)),
		ID:      "urn:opds:browse:authors",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/authors", Type: NavigationMediaType},
		},
	}
	feed.Entries[1] = Entry{
		Title:   fmt.Sprintf("Find books by title (%d)", len(books)),
		ID:      "urn:opds:browse:titles",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/titles", Type: NavigationMediaType},
		},
	}
	feed.Entries[2] = Entry{
		Title:   fmt.Sprintf("Find books by series (%d)", len(books)),
		ID:      "urn:opds:browse:series",
		Updated: feed.Updated,
		Links: []Link{
			{Rel: "subsection", Href: baseURL + "/opds/series", Type: NavigationMediaType},
		},
	}
	return feed
}

// BuildAuthorNavigationFeed renders a navigation feed of authors (or ranges).
func BuildAuthorNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "authors")

	feed := newFeed(baseURL, "/opds/authors", NavigationMediaType, "Authors", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0)
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
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
		for _, r := range ranges {
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
				ID:      "urn:opds:authors:range:" + r.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/authors/" + r.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildAuthorRangeFeed renders authors in a specific alphabetical range.
func BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "authors")
	rangeItems := grouped[rangeID]

	feed := newFeed(baseURL, "/opds/authors/"+rangeID, NavigationMediaType, "Authors", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/authors", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0, len(rangeItems))
	for _, adapter := range rangeItems {
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
func BuildTitleNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByTitle(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "titles")

	feed := newFeed(baseURL, "/opds/titles", NavigationMediaType, "Titles", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0)
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
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
		for _, r := range ranges {
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
				ID:      "urn:opds:titles:range:" + r.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/titles/" + r.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildTitleRangeFeed renders all books in a specific alphabetical range as an acquisition feed.
func BuildTitleRangeFeed(baseURL string, books []model.Book, rangeID string, updatedAt time.Time) Feed {
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

	feed := newFeed(baseURL, "/opds/titles/"+rangeID, MediaType, "Titles", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0, len(booksInRange))
	padding := bookmeta.MaxSequencePadding(booksInRange)
	for _, b := range booksInRange {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildTitleFeed renders all books with a specific title as an acquisition feed.
func BuildTitleFeed(baseURL, titleName string, books []model.Book, updatedAt time.Time) Feed {
	feed := newFeed(baseURL, "/opds/titles/"+fmt.Sprintf("%x", titleName), MediaType, titleName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/titles", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0, len(books))
	padding := bookmeta.MaxSequencePadding(books)
	for _, b := range books {
		feed.Entries = append(feed.Entries, buildEntryWithSeriesPrefix(baseURL, b, feed.Updated, padding, false))
	}
	return feed
}

// BuildSeriesNavigationFeed renders a navigation feed of series (or ranges).
func BuildSeriesNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}
	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "series")

	feed := newFeed(baseURL, "/opds/series", NavigationMediaType, "Series", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0)
	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
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
		for _, r := range ranges {
			feed.Entries = append(feed.Entries, Entry{
				Title:   fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
				ID:      "urn:opds:series:range:" + r.ID,
				Updated: feed.Updated,
				Links: []Link{
					{Rel: "subsection", Href: baseURL + "/opds/series/" + r.ID, Type: NavigationMediaType},
				},
			})
		}
	}
	return feed
}

// BuildSeriesRangeFeed renders series in a specific alphabetical range.
func BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)
	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}
	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "series")
	rangeItems := grouped[rangeID]

	feed := newFeed(baseURL, "/opds/series/"+rangeID, NavigationMediaType, "Series", updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/series", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0, len(rangeItems))
	for _, adapter := range rangeItems {
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
func BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, updatedAt time.Time) Feed {
	feed := newFeed(baseURL, "/opds/series/"+seriesID, MediaType, seriesName, updatedAt)
	feed.Links = append(feed.Links, Link{Rel: "up", Href: baseURL + "/opds/series", Type: NavigationMediaType})

	feed.Entries = make([]Entry, 0, len(books))
	padding := bookmeta.MaxSequencePadding(books)
	for _, b := range books {
		feed.Entries = append(feed.Entries, buildEntry(baseURL, b, feed.Updated, padding))
	}
	return feed
}
