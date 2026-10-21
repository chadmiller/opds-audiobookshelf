// Package opds builds an OPDS 2.0 (https://specs.opds.io/opds-2.0.html)
// JSON catalog feed out of the scanned book catalog.
package opdsjson

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/feeds"
	"github.com/chadmiller/opds-audiobookshelf/internal/pagination"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

const MediaType = "application/opds+json"

// JSONFeed builds OPDS 2.0 JSON feeds. It is stateless; negotiation hands
// back a JSONFeed value once per request, and callers call its methods
// directly without needing to know they're talking to this package.
type JSONFeed struct{}

// ContentType satisfies feeds.Feed.
func (f Feed) ContentType() string { return MediaType }

// Encode writes f to w as indented JSON, satisfying feeds.Feed.
func (f Feed) Encode(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

func pageQueryString(page int) string {
	if page <= 1 {
		return ""
	}
	return fmt.Sprintf("?page=%d", page)
}

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

// buildCategoryItemLink renders the listing navigation link for one author/
// title/series group, pointing at that group's own acquisition feed. Shared
// by buildCategoryNavigationFeed's "all" case and buildCategoryRangeFeed.
func buildCategoryItemLink(baseURL string, cat bookmeta.Category, item bookmeta.CategoryItem) Link {
	return Link{
		Href:  baseURL + "/opds/" + cat.PathSegment + "/" + item.ItemID(),
		Type:  MediaType,
		Title: fmt.Sprintf("%s (%d)", item.ItemLabel(), item.GetLeafCount()),
	}
}

// buildRangeLink renders the listing navigation link for one alphabetical
// range bucket of a category's groups (e.g. "authors starting a-c"). Shared
// by buildCategoryNavigationFeed.
func buildRangeLink(baseURL string, cat bookmeta.Category, r bookmeta.AlphaRange) Link {
	return Link{
		Href:  baseURL + "/opds/" + cat.PathSegment + "/" + r.ID,
		Type:  MediaType,
		Title: fmt.Sprintf("%s (%d)", r.Label, r.LeafCount),
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
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        cat.Title,
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + basePath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}

	if len(ranges) == 1 && ranges[0].ID == "all" {
		pageItems, _, totalPages := pagination.Paginate(items, page)
		feed.Links = append(feed.Links, paginationLinks(baseURL, basePath, page, totalPages, MediaType)...)
		for _, item := range pageItems {
			feed.Navigation = append(feed.Navigation, buildCategoryItemLink(baseURL, cat, item))
		}
		return feed
	}

	pageRanges, _, totalPages := pagination.Paginate(ranges, page)
	feed.Links = append(feed.Links, paginationLinks(baseURL, basePath, page, totalPages, MediaType)...)
	for _, r := range pageRanges {
		feed.Navigation = append(feed.Navigation, buildRangeLink(baseURL, cat, r))
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
	feed := Feed{
		Metadata: FeedMetadata{
			Title:        cat.Title,
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + basePath, Type: MediaType},
		},
		Navigation: []Link{},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	for _, item := range pageItems {
		feed.Navigation = append(feed.Navigation, buildCategoryItemLink(baseURL, cat, item.(bookmeta.CategoryItem)))
	}
	return feed
}

// BuildNavigationFeed renders the catalog root as an OPDS 2.0 navigation
// feed: one navigation entry per author, each pointing at the acquisition
// feed built by BuildAuthorFeed for that author's books. baseURL must not have a
// trailing slash, e.g. "https://example.com".
func (JSONFeed) BuildNavigationFeed(baseURL, title string, groups []bookmeta.AuthorGroup, updatedAt time.Time) feeds.Feed {
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

// BuildFAuthoreed renders one author's books as a single-page OPDS 2.0 acquisition
// catalog. baseURL must not have a trailing slash, e.g. "https://example.com".
// selfPath is this feed's own path, e.g. "/opds/authors/weir-andy".
// page is 1-indexed.
func (JSONFeed) BuildAuthorFeed(baseURL, selfPath, title string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        title,
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(pageBooks)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
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
// options: by author, by title, by series. Each option shows the count of groups
// (distinct authors, titles, or series).
func (JSONFeed) BuildRootFeed(baseURL, title string, books []model.Book, updatedAt time.Time) feeds.Feed {

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
				Href:  baseURL + "/opds/authors/",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by author (%d)", len(bookmeta.GroupByAuthor(books))),
			},
			{
				Href:  baseURL + "/opds/titles/",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by title (%d)", len(bookmeta.GroupByTitle(books))),
			},
			{
				Href:  baseURL + "/opds/series/",
				Type:  MediaType,
				Title: fmt.Sprintf("Find books by series (%d)", len(bookmeta.GroupBySeries(books))),
			},
		},
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	return feed
}

// BuildAuthorNavigationFeed renders a navigation feed of authors (or alphabetical
// ranges of authors if there are more than RangeGroupThreshold items). Each author links to their books.
// page is 1-indexed.
func (JSONFeed) BuildAuthorNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.AuthorCategory, bookmeta.AuthorCategoryItems(books), page, updatedAt)
}

// BuildAuthorRangeFeed renders a navigation feed of authors in a specific
// alphabetical range (e.g., a-c). Each author links to their books.
// page is 1-indexed.
func (JSONFeed) BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryRangeFeed(baseURL, bookmeta.AuthorCategory, bookmeta.AuthorCategoryItems(books), rangeID, page, updatedAt)
}

// BuildTitleNavigationFeed renders a navigation feed of titles (or alphabetical
// ranges of titles if there are more than RangeGroupThreshold items). Each title links to all books with that title.
// page is 1-indexed.
func (JSONFeed) BuildTitleNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.TitleCategory, bookmeta.TitleCategoryItems(books), page, updatedAt)
}

// BuildTitleRangeFeed renders all books in a specific alphabetical range as an acquisition feed.
// page is 1-indexed.
func (JSONFeed) BuildTitleRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
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

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        "Titles",
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/titles/", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(pageBooks)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Publications = append(feed.Publications, buildPublicationWithSeriesPrefix(baseURL, b, padding, false))
	}

	return feed
}

// BuildTitleFeed renders all books with a specific title as an acquisition feed.
// page is 1-indexed.
func (JSONFeed) BuildTitleFeed(baseURL, titleName string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/titles/" + fmt.Sprintf("%x", titleName)

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        titleName,
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/titles/", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(pageBooks)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Publications = append(feed.Publications, buildPublicationWithSeriesPrefix(baseURL, b, padding, false))
	}
	return feed
}

// BuildSeriesNavigationFeed renders a navigation feed of series (or alphabetical
// ranges if there are more than RangeGroupThreshold items). Each series links to books in that series.
// page is 1-indexed.
func (JSONFeed) BuildSeriesNavigationFeed(baseURL string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryNavigationFeed(baseURL, bookmeta.SeriesCategory, bookmeta.SeriesCategoryItems(books), page, updatedAt)
}

// BuildSeriesRangeFeed renders a navigation feed of series in a specific
// alphabetical range. Each series links to books in that series.
// page is 1-indexed.
func (JSONFeed) BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) feeds.Feed {
	return buildCategoryRangeFeed(baseURL, bookmeta.SeriesCategory, bookmeta.SeriesCategoryItems(books), rangeID, page, updatedAt)
}

// BuildSeriesFeed renders all books in a specific series as an acquisition feed.
// page is 1-indexed.
func (JSONFeed) BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, page int, updatedAt time.Time) feeds.Feed {
	pageBooks, _, totalPages := pagination.Paginate(books, page)
	selfPath := "/opds/series/" + seriesID

	feed := Feed{
		Metadata: FeedMetadata{
			Title:        seriesName,
			ItemsPerPage: pagination.PageSize,
		},
		Links: []Link{
			{Rel: "self", Href: baseURL + selfPath + pageQueryString(page), Type: MediaType},
			{Rel: "up", Href: baseURL + "/opds/series/", Type: MediaType},
		},
		Publications: make([]Publication, 0, len(pageBooks)),
	}
	if !updatedAt.IsZero() {
		feed.Metadata.Modified = updatedAt.UTC().Format(time.RFC3339)
	}
	feed.Links = append(feed.Links, paginationLinks(baseURL, selfPath, page, totalPages, MediaType)...)

	padding := bookmeta.MaxSequencePadding(pageBooks)
	for _, b := range pageBooks {
		feed.Publications = append(feed.Publications, buildPublication(baseURL, b, padding))
	}
	return feed
}
