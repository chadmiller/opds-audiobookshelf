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
	"strings"
	"time"

	"github.com/chadmiller/audiobookshelf-opds-server/internal/atom"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/bookmeta"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/catalog"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/model"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/opds"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/pathmap"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/placeholder"
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
	s.mux.HandleFunc("/opds", s.handleFeed)
	s.mux.HandleFunc("/", s.handleFeed)
	s.mux.HandleFunc("/opds/authors/", s.handleAuthorFeed)
	s.mux.HandleFunc("/download/", s.handleDownload)
	s.mux.HandleFunc("/covers/", s.handleCover)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleFeed serves the catalog root as a navigation feed: one entry per
// author, each linking to that author's acquisition feed (handleAuthorFeed).
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/opds" {
		http.NotFound(w, r)
		return
	}
	books, updatedAt := s.store.Get()
	groups := bookmeta.GroupByAuthor(books)
	base := baseURL(r)

	s.writeNegotiatedFeed(w, r,
		func() opds.Feed { return opds.BuildNavigationFeed(base, s.title, groups, updatedAt) },
		func() atom.Feed { return atom.BuildNavigationFeed(base, s.title, groups, updatedAt) },
		atom.NavigationMediaType,
	)
}

// handleAuthorFeed serves one author's books as an acquisition feed.
func (s *Server) handleAuthorFeed(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/opds/authors/")
	books, updatedAt := s.store.Get()
	group, ok := findAuthorGroup(bookmeta.GroupByAuthor(books), id)
	if !ok {
		http.NotFound(w, r)
		fmt.Println("Failed to find author group", id)
		return
	}
	base := baseURL(r)
	selfPath := "/opds/authors/" + group.ID

	s.writeNegotiatedFeed(w, r,
		func() opds.Feed { return opds.BuildFeed(base, selfPath, group.DisplayName, group.Books, updatedAt) },
		func() atom.Feed { return atom.BuildFeed(base, selfPath, group.DisplayName, group.Books, updatedAt) },
		atom.MediaType,
	)
}

// writeNegotiatedFeed renders buildJSON or buildAtom (whichever the
// request's Accept header selects) and writes it with an explicit
// Content-Length rather than letting net/http fall back to chunked
// transfer-encoding, which some minimal OPDS client HTTP stacks don't
// decode correctly.
func (s *Server) writeNegotiatedFeed(w http.ResponseWriter, r *http.Request, buildJSON func() opds.Feed, buildAtom func() atom.Feed, atomContentType string) {
	var buf bytes.Buffer
	var contentType string
	if negotiateFeedFormat(r.Header.Get("Accept")) {
		contentType = opds.MediaType
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
