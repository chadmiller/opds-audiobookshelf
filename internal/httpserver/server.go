// Package httpserver exposes the catalog as an unauthenticated OPDS 2.0 feed
// plus the supporting download/cover endpoints it links to.
package httpserver

import (
	"bytes"
	"fmt"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/catalog"
	"github.com/chadmiller/opds-audiobookshelf/internal/feeds"
	opdsatom "github.com/chadmiller/opds-audiobookshelf/internal/feeds/atom"
	opdsjson "github.com/chadmiller/opds-audiobookshelf/internal/feeds/json"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/pathmap"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

// negotiateBuilder is the one place in this package that knows the feed
// dialects exist: it wires feeds.Negotiate up to the two concrete Builders,
// so every handler below can just ask for "the" builder for this request.
func negotiateBuilder(r *http.Request) feeds.Builder {
	return feeds.Negotiate(r.Header.Get("Accept"), opdsjson.JSONFeed{}, opdsatom.AtomFeed{})
}

// Server serves the OPDS catalog and the files it references.
type Server struct {
	store     *catalog.Store
	filesRoot string
	title     string
	mux       *http.ServeMux
}

// New builds a Server. filesRoot is the on-disk directory that stands in for
// the filesystem root as seen inside the Audiobookshelf container: paths
// read from the database are resolved as filepath.Join(filesRoot, dbPath).
func New(store *catalog.Store, filesRoot, title string) *Server {
	s := &Server{
		store:     store,
		filesRoot: filesRoot,
		title:     title,
		mux:       http.NewServeMux(),
	}
	s.mux.HandleFunc("/opds", s.handleRoot)
	s.mux.HandleFunc("/", s.handleRoot)
	s.mux.HandleFunc("/opds/authors/", s.handleAuthorOrRange)
	s.mux.HandleFunc("/opds/titles/", s.handleTitleOrRange)
	s.mux.HandleFunc("/opds/series/", s.handleSeriesOrRange)
	s.mux.HandleFunc("/download/", s.handleDownload)
	s.mux.HandleFunc("/covers/", s.handleCover)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func getPage(r *http.Request) int {
	pageStr := r.URL.Query().Get("page")
	if pageStr == "" {
		return 1
	}
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		return 1
	}
	return page
}

// handleRoot serves the catalog root as a selection menu with three browse options.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/opds" {
		http.NotFound(w, r)
		return
	}
	books, updatedAt := s.store.Get()
	base := ""
	builder := negotiateBuilder(r)

	writeFeed(w, r, builder.BuildRootFeed(base, s.title, books, updatedAt))
}

// handleAuthorOrRange serves either a navigation feed of authors/ranges (if no ID or
// if ID is a range like "a-c") or an individual author's books (if ID is a valid author).
func (s *Server) handleAuthorOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/authors/")
	books, updatedAt := s.store.Get()
	base := ""
	page := getPage(r)
	builder := negotiateBuilder(r)

	if id == "" {
		writeFeed(w, r, builder.BuildAuthorNavigationFeed(base, books, page, updatedAt))
		return
	} else if isRange := isAlphaRange(id); isRange {
		writeFeed(w, r, builder.BuildAuthorRangeFeed(base, books, id, page, updatedAt))
		return
	}

	group, ok := findAuthorGroup(bookmeta.GroupByAuthor(books), id)
	if !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find author group", id)
		return
	}
	selfPath := "/opds/authors/" + group.ID

	writeFeed(w, r, builder.BuildAuthorFeed(base, selfPath, group.DisplayName, group.Books, page, updatedAt))
}

// handleTitleOrRange serves either a navigation feed of titles/ranges or all books with a specific title.
func (s *Server) handleTitleOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/titles/")
	books, updatedAt := s.store.Get()
	base := ""
	page := getPage(r)
	builder := negotiateBuilder(r)

	if id == "" {
		writeFeed(w, r, builder.BuildTitleNavigationFeed(base, books, page, updatedAt))
		return
	} else if isRange := isAlphaRange(id); isRange {
		writeFeed(w, r, builder.BuildTitleRangeFeed(base, books, id, page, updatedAt))
		return
	} else if titleName, ok := decodeTitle(id); !ok {
		http.NotFound(w, r)
		return
	} else {
		groups := bookmeta.GroupByTitle(books)
		var titleBooks []model.Book
		for _, g := range groups {
			if g.Title == titleName {
				titleBooks = g.Books
				break
			}
		}
		if len(titleBooks) == 0 {
			http.NotFound(w, r)
			return
		}

		writeFeed(w, r, builder.BuildTitleFeed(base, titleName, titleBooks, page, updatedAt))
	}
}

// handleSeriesOrRange serves either a navigation feed of series/ranges or all books in a specific series.
func (s *Server) handleSeriesOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/series/")
	books, updatedAt := s.store.Get()
	base := ""
	page := getPage(r)
	builder := negotiateBuilder(r)

	if id == "" {
		writeFeed(w, r, builder.BuildSeriesNavigationFeed(base, books, page, updatedAt))
		return
	} else if isRange := isAlphaRange(id); isRange {
		writeFeed(w, r, builder.BuildSeriesRangeFeed(base, books, id, page, updatedAt))
		return
	} else if group, ok := findSeriesGroup(bookmeta.GroupBySeries(books), id); !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find series group", id)
		return
	} else {
		writeFeed(w, r, builder.BuildSeriesFeed(base, group.ID, group.Name, group.Books, page, updatedAt))
	}
}

// writeFeed encodes feed and writes it with an explicit Content-Length
// rather than letting net/http fall back to chunked transfer-encoding,
// which some minimal OPDS client HTTP stacks don't decode correctly.
func writeFeed(w http.ResponseWriter, r *http.Request, feed feeds.Feed) {
	var buf bytes.Buffer
	if err := feed.Encode(&buf); err != nil {
		log.Printf("encode feed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	contentType := feed.ContentType()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", buf.Len()))
	w.Write(buf.Bytes())
	fmt.Println("successfully answered feed", r.URL, "for length", buf.Len(), "content-type", contentType)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/download/")
	book, ok := s.findBook(id)
	if !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find download object", id)
		return
	}
	hostPath := pathmap.HostPath(s.filesRoot, book, book.Ebook.Metadata.Path)
	name := book.Ebook.Metadata.Filename
	if name == "" {
		name = filepath.Base(hostPath)
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	http.ServeFile(w, r, hostPath)
	fmt.Println("successfully answered download", r.URL)
}

func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/covers/")
	book, ok := s.findBook(id)
	if !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find book cover", id)
		return
	}
	if !book.CoverResolvable {
		w.Header().Set("Content-Type", placeholder.MimeType)
		http.ServeContent(w, r, "cover.png", time.Time{}, bytes.NewReader(placeholder.PNG))
		fmt.Println("successfully answered cover (placeholder)", r.URL)
		return
	}
	http.ServeFile(w, r, pathmap.HostPath(s.filesRoot, book, book.CoverPath))
	fmt.Println("successfully answered cover", r.URL)
}

func (s *Server) findBook(id string) (model.Book, bool) {
	books, _ := s.store.Get()
	for _, b := range books {
		if b.LibraryItemID == id {
			return b, true
		}
	}
	return model.Book{}, false
}

func findAuthorGroup(groups []bookmeta.AuthorGroup, id string) (bookmeta.AuthorGroup, bool) {
	for _, g := range groups {
		if g.ID == id {
			return g, true
		}
	}
	return bookmeta.AuthorGroup{}, false
}

func findSeriesGroup(groups []bookmeta.SeriesGroup, id string) (bookmeta.SeriesGroup, bool) {
	for _, g := range groups {
		if g.ID == id {
			return g, true
		}
	}
	return bookmeta.SeriesGroup{}, false
}

func isAlphaRange(id string) bool {
	return strings.HasPrefix(id, "range/")
}

func decodeTitle(encoded string) (string, bool) {
	var result []rune
	for i := 0; i < len(encoded); i += 2 {
		if i+1 >= len(encoded) {
			return "", false
		}
		var b [1]byte
		_, err := fmt.Sscanf(encoded[i:i+2], "%x", &b[0])
		if err != nil {
			return "", false
		}
		result = append(result, rune(b[0]))
	}
	return string(result), true
}
