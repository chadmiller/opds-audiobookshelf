package abs_api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const goodItem = `{
  "id": "li_1", "path": "/ebooks/Weir/Hail Mary", "relPath": "Weir/Hail Mary",
  "mediaType": "book",
  "media": {
    "coverPath": "/ebooks/Weir/Hail Mary/cover.jpg", "tags": ["scifi"],
    "metadata": {
      "title": "Project Hail Mary", "titleIgnorePrefix": "Project Hail Mary",
      "authorName": "Andy Weir", "authorNameLF": "Weir, Andy",
      "series": [{"name": "Solo", "sequence": "1"}], "genres": ["SF"]
    },
    "ebookFile": {"ino": "1", "ebookFormat": "epub",
      "metadata": {"filename": "phm.epub", "ext": ".epub", "path": "/ebooks/Weir/Hail Mary/phm.epub", "size": 10}}
  }
}`

func fakeServer(t *testing.T) *httptest.Server {
	t.Helper()
	items := map[string]string{
		"li_1":    goodItem,
		"li_miss": `{"id":"li_miss","mediaType":"book","isMissing":true,"media":{}}`,
		"li_noeb": `{"id":"li_noeb","mediaType":"book","media":{"metadata":{}}}`,
	}
	mux := http.NewServeMux()
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "nope", http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/libraries", auth(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"libraries":[{"id":"lib1","name":"ebooks"}]}`)
	}))
	// Two pages of one-and-two results, to exercise pagination.
	mux.HandleFunc("/api/libraries/lib1/items", auth(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "0":
			fmt.Fprint(w, `{"total":3,"results":[{"id":"li_1"},{"id":"li_miss"}]}`)
		case "1":
			fmt.Fprint(w, `{"total":3,"results":[{"id":"li_noeb"}]}`)
		default:
			fmt.Fprint(w, `{"total":3,"results":[]}`)
		}
	}))
	mux.HandleFunc("/api/items/", auth(func(w http.ResponseWriter, r *http.Request) {
		body, ok := items[strings.TrimPrefix(r.URL.Path, "/api/items/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestScan(t *testing.T) {
	srv := fakeServer(t)
	books, err := Client{Address: srv.URL, APIKey: "secret"}.Scan([]string{"ebooks"})
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 {
		t.Fatalf("got %d books, want 1 (skipped items must be dropped): %+v", len(books), books)
	}
	b := books[0]
	if b.LibraryItemID != "li_1" || b.Title != "Project Hail Mary" ||
		b.AuthorName != "Andy Weir" || b.AuthorNamesLastFirst != "Weir, Andy" ||
		b.SeriesName != "Solo" || b.SeriesSequence != "1" ||
		b.Ebook.Metadata.Path != "/ebooks/Weir/Hail Mary/phm.epub" ||
		b.CoverPath != "/ebooks/Weir/Hail Mary/cover.jpg" ||
		b.ItemRelPath != "Weir/Hail Mary" || len(b.Tags) != 1 {
		t.Errorf("unexpected book: %+v", b)
	}
}

func TestScanErrors(t *testing.T) {
	srv := fakeServer(t)
	if _, err := (Client{Address: srv.URL, APIKey: "secret"}).Scan([]string{"nonesuch"}); err == nil {
		t.Error("unknown library: want error")
	}
	if _, err := (Client{Address: srv.URL, APIKey: "wrong"}).Scan([]string{"ebooks"}); err == nil {
		t.Error("bad key: want error")
	}
	if _, err := (Client{Address: srv.URL, APIKey: "secret"}).Scan(nil); err == nil {
		t.Error("no libraries: want error")
	}
}
