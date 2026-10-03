// Package opds builds an OPDS 2.0 (https://specs.opds.io/opds-2.0.html)
// JSON catalog feed out of the scanned book catalog.
package opds

import (
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

const MediaType = "application/opds+json"

type Feed struct {
	Metadata     FeedMetadata  `json:"metadata"`
	Links        []Link        `json:"links"`
	Navigation   []Link        `json:"navigation,omitempty"`
	Publications []Publication `json:"publications,omitempty"`
}

type FeedMetadata struct {
	Title        string `json:"title"`
	ItemsPerPage int    `json:"itemsPerPage,omitempty"`
	Modified     string `json:"modified,omitempty"`
}

type Link struct {
	Rel     string `json:"rel,omitempty"`
	Href    string `json:"href"`
	Type    string `json:"type,omitempty"`
	Title   string `json:"title,omitempty"`
	Bitrate int    `json:"bitrate,omitempty"`
}

type Author struct {
	Name string `json:"name"`
}

type PublicationMetadata struct {
	Type        string   `json:"@type,omitempty"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Author      []Author `json:"author,omitempty"`
	Narrator    []Author `json:"narrator,omitempty"`
	Identifier  string   `json:"identifier,omitempty"`
	Language    string   `json:"language,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Published   string   `json:"published,omitempty"`
	Description string   `json:"description,omitempty"`
	Subject     []string `json:"subject,omitempty"`
}

type Publication struct {
	Metadata PublicationMetadata `json:"metadata"`
	Links    []Link              `json:"links"`
	Images   []Link              `json:"images,omitempty"`
}

// BuildNavigationFeed renders the catalog root as an OPDS 2.0 navigation
// feed: one navigation entry per author, each pointing at the acquisition
// feed built by BuildFeed for that author's books. baseURL must not have a
// trailing slash, e.g. "https://example.com".
func BuildNavigationFeed(baseURL, title string, groups []bookmeta.AuthorGroup, updatedAt time.Time) Feed {
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        title,
			ItemsPerPage: len(groups),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: make([]Link, 0, len(groups)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	for _, g := range groups {
		feed.Navigation = append(feed.Navigation, Link{
			Href:  baseURL + "/opds/authors/" + g.ID,
			Type:  MediaType,
			Title: g.DisplayName,
		})
	}
	return feed
}

// BuildFeed renders one author's books as a single-page OPDS 2.0 acquisition
// catalog. baseURL must not have a trailing slash, e.g. "https://example.com".
// selfPath is this feed's own path, e.g. "/opds/authors/weir-andy".
func BuildFeed(baseURL, selfPath, title string, books []model.Book, updatedAt time.Time) Feed {
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        title,
			ItemsPerPage: len(books),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath, Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(books)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	padding := bookmeta.MaxSequencePadding(books)
	for _, b := range books {
		feed.Publications = append(feed.Publications, buildPublication(baseURL, b, padding))
	}
	return feed
}

func buildPublication(baseURL string, b model.Book, padding int) Publication {
	var authors []Author
	if b.AuthorName != "" {
		authors = append(authors, Author{Name: b.AuthorName})
	}
	var narrators []Author
	for _, n := range b.Narrators {
		narrators = append(narrators, Author{Name: n})
	}

	var subjects []string
	subjects = append(subjects, b.Genres...)
	subjects = append(subjects, b.Tags...)

	pub := Publication{
		Metadata: PublicationMetadata{
			Type:        "http://schema.org/Book",
			Title:       bookmeta.FormatTitle(b, padding),
			Subtitle:    b.Subtitle,
			Author:      authors,
			Narrator:    narrators,
			Identifier:  bookmeta.Identifier(b),
			Language:    b.Language,
			Publisher:   b.Publisher,
			Published:   bookmeta.Published(b),
			Description: b.Description,
			Subject:     subjects,
		},
		Links: []Link{
			{
				Rel:  "http://opds-spec.org/acquisition",
				Href: baseURL + "/download/" + b.LibraryItemID,
				Type: bookmeta.EbookMimeType(b),
			},
		},
	}

	coverType := placeholder.MimeType
	if b.CoverResolvable {
		coverType = bookmeta.CoverMimeType(b.CoverPath)
	}
	pub.Images = []Link{
		{
			Rel:  "http://opds-spec.org/image",
			Href: baseURL + "/covers/" + b.LibraryItemID,
			Type: coverType,
		},
	}

	return pub
}
