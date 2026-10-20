// Package feeds defines the interfaces shared by every OPDS feed dialect
// (internal/feeds/json, internal/feeds/atom) and negotiates between them.
// Feed is the rendered output of one request; Builder is the per-dialect
// factory that produces Feeds. Negotiate inspects the request's Accept
// header once and returns whichever Builder it's handed that matches;
// callers then call methods on that Builder directly without any further
// branching on dialect.
//
// This package deliberately does not import the dialect packages (they
// import it, for the Feed/Builder types their factories must return) —
// callers pass the dialect Builders into Negotiate explicitly.
package feeds

import (
	"io"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// Feed is the rendered output of a Builder method: something that knows its
// own media type and how to serialize itself.
type Feed interface {
	ContentType() string
	Encode(w io.Writer) error
}

// Builder is implemented once per feed dialect (e.g. internal/feeds/json's
// JSONFeed, internal/feeds/atom's AtomFeed). Negotiate picks a Builder once
// per request; callers then call its methods directly without branching on
// dialect.
type Builder interface {
	BuildRootFeed(baseURL, title string, books []model.Book, updatedAt time.Time) Feed
	BuildAuthorNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed
	BuildAuthorRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed
	BuildFeed(baseURL, selfPath, title string, books []model.Book, page int, updatedAt time.Time) Feed
	BuildTitleNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed
	BuildTitleRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed
	BuildTitleFeed(baseURL, titleName string, books []model.Book, page int, updatedAt time.Time) Feed
	BuildSeriesNavigationFeed(baseURL, title string, books []model.Book, page int, updatedAt time.Time) Feed
	BuildSeriesRangeFeed(baseURL string, books []model.Book, rangeID string, page int, updatedAt time.Time) Feed
	BuildSeriesFeed(baseURL, seriesID, seriesName string, books []model.Book, page int, updatedAt time.Time) Feed
}
