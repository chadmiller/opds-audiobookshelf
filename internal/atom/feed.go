// Package atom builds an OPDS 1.x Atom/XML catalog feed out of the scanned
// book catalog, for clients (e.g. e-readers using Expat-based XML parsers)
// that don't understand OPDS 2.0 JSON.
package atom

import (
	"encoding/xml"
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
	entry := Entry{
		Title:   bookmeta.FormatTitle(b, padding),
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
