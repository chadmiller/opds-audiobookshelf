// Package opds builds an OPDS 2.0 (https://specs.opds.io/opds-2.0.html)
// JSON catalog feed out of the scanned book catalog.
package opds

import (
	"fmt"
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
	return buildPublicationWithSeriesPrefix(baseURL, b, padding, true)
}

func buildPublicationWithSeriesPrefix(baseURL string, b model.Book, padding int, includeSeriesPrefix bool) Publication {
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

	title := b.Title
	if includeSeriesPrefix {
		title = bookmeta.FormatTitle(b, padding)
	}

	pub := Publication{
		Metadata: PublicationMetadata{
			Type:        "http://schema.org/Book",
			Title:       title,
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

// BrowseOption represents one browse option at the root level
type BrowseOption struct {
	Title     string
	Path      string
	LeafCount int
}

// BuildRootFeed renders the root catalog as a selection menu with three browse
// options: by author, by title, by series. Each option shows the count of leaf
// nodes (books) reachable through that route.
func BuildRootFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        title,
			ItemsPerPage: 3,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: []Link{
			{
				Href:  baseURL + "/opds/authors",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by author (%d)", len(books)),
			},
			{
				Href:  baseURL + "/opds/titles",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by title (%d)", len(books)),
			},
			{
				Href:  baseURL + "/opds/series",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by series (%d)", len(books)),
			},
		},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	return feed
}

// BuildAuthorNavigationFeed renders a navigation feed of authors (or alphabetical
// ranges of authors if there are > 100 books). Each author links to their books.
func BuildAuthorNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)

	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}

	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "authors")

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Authors",
			ItemsPerPage: len(ranges),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/authors", Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/authors/" + group.ID,
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", group.DisplayName, len(group.Books)),
			})
		}
	} else {
		for _, r := range ranges {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/authors/" + r.ID,
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
			})
		}
	}

	return feed
}

// BuildAuthorRangeFeed renders a navigation feed of authors in a specific
// alphabetical range (e.g., a-c). Each author links to their books.
func BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByAuthor(books)

	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.AuthorRangeAdapter{Group: &groups[i]}
	}

	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "authors")
	rangeItems := grouped[rangeID]

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Authors",
			ItemsPerPage: len(rangeItems),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/authors/" + rangeID, Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/authors", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	for _, adapter := range rangeItems {
		group := adapter.(*bookmeta.AuthorRangeAdapter).Group
		feed.Navigation = append(feed.Navigation, Link{
			Href:  baseURL + "/opds/authors/" + group.ID,
			Type:  MediaType,
			Title: fmt.Sprintf("%s (%d)", group.DisplayName, len(group.Books)),
		})
	}

	return feed
}

// BuildTitleNavigationFeed renders a navigation feed of titles (or alphabetical
// ranges of titles if there are > 100 books). Each title links to all books with that title.
func BuildTitleNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupByTitle(books)

	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.TitleRangeAdapter{Group: &groups[i]}
	}

	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "titles")

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Titles",
			ItemsPerPage: len(ranges),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/titles", Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/titles/" + fmt.Sprintf("%x", group.Title),
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", group.Title, len(group.Books)),
			})
		}
	} else {
		for _, r := range ranges {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/titles/" + r.ID,
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
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

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Titles",
			ItemsPerPage: len(booksInRange),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/titles/" + rangeID, Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/titles", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(booksInRange)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	padding := bookmeta.MaxSequencePadding(booksInRange)
	for _, b := range booksInRange {
		feed.Publications = append(feed.Publications, buildPublicationWithSeriesPrefix(baseURL, b, padding, false))
	}

	return feed
}

// BuildTitleFeed renders all books with a specific title as an acquisition feed.
func BuildTitleFeed(baseURL, titleName string, books []model.Book, updatedAt time.Time) Feed {
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        titleName,
			ItemsPerPage: len(books),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/titles/" + fmt.Sprintf("%x", titleName), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/titles", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(books)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	padding := bookmeta.MaxSequencePadding(books)
	for _, b := range books {
		feed.Publications = append(feed.Publications, buildPublicationWithSeriesPrefix(baseURL, b, padding, false))
	}
	return feed
}

// BuildSeriesNavigationFeed renders a navigation feed of series (or alphabetical
// ranges if there are > 100 books). Each series links to books in that series.
func BuildSeriesNavigationFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)

	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}

	ranges, _ := bookmeta.GroupByAlphaRange(adapters, 100, "series")

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Series",
			ItemsPerPage: len(ranges),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/series", Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	if len(ranges) == 1 && ranges[0].ID == "all" {
		for _, group := range groups {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/series/" + group.ID,
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", group.Name, len(group.Books)),
			})
		}
	} else {
		for _, r := range ranges {
			feed.Navigation = append(feed.Navigation, Link{
				Href:  baseURL + "/opds/series/" + r.ID,
				Type:  MediaType,
				Title: fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
			})
		}
	}

	return feed
}

// BuildSeriesRangeFeed renders a navigation feed of series in a specific
// alphabetical range. Each series links to books in that series.
func BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, updatedAt time.Time) Feed {
	groups := bookmeta.GroupBySeries(books)

	adapters := make([]bookmeta.AlphaRanged, len(groups))
	for i := range groups {
		adapters[i] = &bookmeta.SeriesRangeAdapter{Group: &groups[i]}
	}

	_, grouped := bookmeta.GroupByAlphaRange(adapters, 100, "series")
	rangeItems := grouped[rangeID]

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Series",
			ItemsPerPage: len(rangeItems),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/series/" + rangeID, Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/series", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	for _, adapter := range rangeItems {
		group := adapter.(*bookmeta.SeriesRangeAdapter).Group
		feed.Navigation = append(feed.Navigation, Link{
			Href:  baseURL + "/opds/series/" + group.ID,
			Type:  MediaType,
			Title: fmt.Sprintf("%s (%d)", group.Name, len(group.Books)),
		})
	}

	return feed
}

// BuildSeriesFeed renders all books in a specific series as an acquisition feed.
func BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, updatedAt time.Time) Feed {
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        seriesName,
			ItemsPerPage: len(books),
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + "/opds/series/" + seriesID, Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/series", Type: MediaType},
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
