package abs_api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const goodItem = `{
  "id": "li_1", "path": "/ebooks/Weir/Hail Mary", "relPath": "Weir/Hail Mary",
  "mediaType": "book", "addedAt": 1700000000000,
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
		len(b.Authors) != 1 || b.Authors[0] != "Weir, Andy" ||
		b.SeriesName != "Solo" || b.SeriesSequence != "1" ||
		b.Ebook.Metadata.Path != "/ebooks/Weir/Hail Mary/phm.epub" ||
		b.CoverPath != "/ebooks/Weir/Hail Mary/cover.jpg" ||
		b.ItemRelPath != "Weir/Hail Mary" || len(b.Tags) != 1 ||
		!b.AddedAt.Equal(time.UnixMilli(1700000000000)) {
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

func TestItemToBookAuthors(t *testing.T) {
	mk := func(md string) libraryItem {
		var it libraryItem
		js := `{"id":"x","mediaType":"book","media":{"metadata":` + md + `,"ebookFile":{"metadata":{"path":"/p"}}}}`
		if err := json.Unmarshal([]byte(js), &it); err != nil {
			t.Fatal(err)
		}
		return it
	}
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{"authors array converted", `{"authors":[{"name":"Andy Weir"},{"name":"Jane Q. Public"}],"authorNameLF":"ignored, Me"}`, []string{"Weir, Andy", "Public, Jane Q."}},
		{"LF fallback", `{"authorNameLF":"Weir, Andy; Smith, Jane"}`, []string{"Weir, Andy", "Smith, Jane"}},
		{"none", `{"authorName":"Someone"}`, nil},
	}
	for _, tt := range tests {
		book, reason := itemToBook(mk(tt.md))
		if tt.want == nil {
			if reason == "" {
				t.Errorf("%s: want skip, got %+v", tt.name, book)
			}
			continue
		}
		if reason != "" || strings.Join(book.Authors, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%s: got %v (%q), want %v", tt.name, book.Authors, reason, tt.want)
		}
	}
}
