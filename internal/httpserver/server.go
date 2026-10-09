// Package httpserver exposes the catalog as an unauthenticated OPDS 2.0 feed
// plus the supporting download/cover endpoints it links to.
package httpserver

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
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
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
	"github.com/chadmiller/opds-audiobookshelf/internal/opdsatom"
	"github.com/chadmiller/opds-audiobookshelf/internal/opdsjson"
	"github.com/chadmiller/opds-audiobookshelf/internal/pathmap"
	"github.com/chadmiller/opds-audiobookshelf/internal/placeholder"
)

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
	base := baseURL(r)

	s.writeNegotiatedFeed(w, r,
		func() opdsjson.Feed { return opdsjson.BuildRootFeed(base, s.title, books, updatedAt) },
		func() opdsatom.Feed { return opdsatom.BuildRootFeed(base, s.title, books, updatedAt) },
		opdsatom.NavigationMediaType,
	)
}

// handleAuthorOrRange serves either a navigation feed of authors/ranges (if no ID or
// if ID is a range like "a-c") or an individual author's books (if ID is a valid author).
func (s *Server) handleAuthorOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/authors/")
	books, updatedAt := s.store.Get()
	base := baseURL(r)
	page := getPage(r)

	if id == "" {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildAuthorNavigationFeed(base, s.title, books, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildAuthorNavigationFeed(base, s.title, books, page, updatedAt) },
			opdsatom.NavigationMediaType,
		)
		return
	} else if isRange := isAlphaRange(id); isRange {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildAuthorRangeFeed(base, books, id, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildAuthorRangeFeed(base, books, id, page, updatedAt) },
			opdsatom.NavigationMediaType,
		)
		return
	}

	group, ok := findAuthorGroup(bookmeta.GroupByAuthor(books), id)
	if !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find author group", id)
		return
	}
	selfPath := "/opds/authors/" + group.ID

	s.writeNegotiatedFeed(w, r,
		func() opdsjson.Feed {
			return opdsjson.BuildFeed(base, selfPath, group.DisplayName, group.Books, page, updatedAt)
		},
		func() opdsatom.Feed {
			return opdsatom.BuildFeed(base, selfPath, group.DisplayName, group.Books, page, updatedAt)
		},
		opdsatom.MediaType,
	)
}

// handleTitleOrRange serves either a navigation feed of titles/ranges or all books with a specific title.
func (s *Server) handleTitleOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/titles/")
	books, updatedAt := s.store.Get()
	base := baseURL(r)
	page := getPage(r)

	if id == "" {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildTitleNavigationFeed(base, s.title, books, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildTitleNavigationFeed(base, s.title, books, page, updatedAt) },
			opdsatom.NavigationMediaType,
		)
		return
	} else if isRange := isAlphaRange(id); isRange {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildTitleRangeFeed(base, books, id, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildTitleRangeFeed(base, books, id, page, updatedAt) },
			opdsatom.MediaType,
		)
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

		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildTitleFeed(base, titleName, titleBooks, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildTitleFeed(base, titleName, titleBooks, page, updatedAt) },
			opdsatom.MediaType,
		)
	}
}

// handleSeriesOrRange serves either a navigation feed of series/ranges or all books in a specific series.
func (s *Server) handleSeriesOrRange(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/series/")
	books, updatedAt := s.store.Get()
	base := baseURL(r)
	page := getPage(r)

	if id == "" {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildSeriesNavigationFeed(base, s.title, books, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildSeriesNavigationFeed(base, s.title, books, page, updatedAt) },
			opdsatom.NavigationMediaType,
		)
		return
	} else if isRange := isAlphaRange(id); isRange {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed { return opdsjson.BuildSeriesRangeFeed(base, books, id, page, updatedAt) },
			func() opdsatom.Feed { return opdsatom.BuildSeriesRangeFeed(base, books, id, page, updatedAt) },
			opdsatom.NavigationMediaType,
		)
		return
	} else if group, ok := findSeriesGroup(bookmeta.GroupBySeries(books), id); !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find series group", id)
		return
	} else {
		s.writeNegotiatedFeed(w, r,
			func() opdsjson.Feed {
				return opdsjson.BuildSeriesFeed(base, group.ID, group.Name, group.Books, page, updatedAt)
			},
			func() opdsatom.Feed {
				return opdsatom.BuildSeriesFeed(base, group.ID, group.Name, group.Books, page, updatedAt)
			},
			opdsatom.MediaType,
		)
	}
}

// writeNegotiatedFeed renders buildJSON or buildAtom (whichever the
// request's Accept header selects) and writes it with an explicit
// Content-Length rather than letting net/http fall back to chunked
// transfer-encoding, which some minimal OPDS client HTTP stacks don't
// decode correctly.
func (s *Server) writeNegotiatedFeed(w http.ResponseWriter, r *http.Request, buildJSON func() opdsjson.Feed, buildAtom func() opdsatom.Feed, atomContentType string) {
	var buf bytes.Buffer
	var contentType string
	if negotiateFeedFormat(r.Header.Get("Accept")) {
		contentType = opdsjson.MediaType
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		if err := enc.Encode(buildJSON()); err != nil {
			log.Printf("encode feed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		contentType = atomContentType
		buf.WriteString(xml.Header)
		enc := xml.NewEncoder(&buf)
		enc.Indent("", "  ")
		if err := enc.Encode(buildAtom()); err != nil {
			log.Printf("encode feed: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

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

func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}
