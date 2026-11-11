package abs_api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

func proxyServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	check := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "nope", http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/items/li_1/file/ino1/download", check(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/epub+zip")
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Header.Get("Range") == "bytes=0-3" {
			w.Header().Set("Content-Range", "bytes 0-3/10")
			w.WriteHeader(http.StatusPartialContent)
			io.WriteString(w, "0123")
			return
		}
		io.WriteString(w, "0123456789")
	}))
	mux.HandleFunc("/api/items/li_1/cover", check(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		io.WriteString(w, "JPEG")
	}))
	// li_nocover has no cover route, so Audiobookshelf answers 404.
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestServeEbook(t *testing.T) {
	srv := proxyServer(t)
	c := Client{Address: srv.URL, APIKey: "secret"}
	book := model.Book{LibraryItemID: "li_1", Ebook: model.EbookFile{Ino: "ino1"}}

	rec := httptest.NewRecorder()
	if err := c.ServeEbook(rec, httptest.NewRequest("GET", "/x", nil), book); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || rec.Body.String() != "0123456789" || rec.Header().Get("Content-Type") != "application/epub+zip" {
		t.Errorf("full: %d %q %v", rec.Code, rec.Body, rec.Header())
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Range", "bytes=0-3")
	if err := c.ServeEbook(rec, req, book); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 206 || rec.Body.String() != "0123" || rec.Header().Get("Content-Range") != "bytes 0-3/10" {
		t.Errorf("range: %d %q %v", rec.Code, rec.Body, rec.Header())
	}
}

func TestServeEbookUpstreamDown(t *testing.T) {
	srv := proxyServer(t)
	addr := srv.URL
	srv.Close()
	rec := httptest.NewRecorder()
	err := Client{Address: addr, APIKey: "secret"}.ServeEbook(rec, httptest.NewRequest("GET", "/x", nil), model.Book{LibraryItemID: "li_1"})
	if err == nil || rec.Code != http.StatusBadGateway {
		t.Errorf("err = %v, code = %d; want error and 502", err, rec.Code)
	}
}

func TestServeCover(t *testing.T) {
	srv := proxyServer(t)
	c := Client{Address: srv.URL, APIKey: "secret"}

	rec := httptest.NewRecorder()
	if !c.ServeCover(rec, httptest.NewRequest("GET", "/x", nil), model.Book{LibraryItemID: "li_1"}) {
		t.Fatal("cover not served")
	}
	if rec.Body.String() != "JPEG" || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("cover: %q %v", rec.Body, rec.Header())
	}

	rec = httptest.NewRecorder()
	if c.ServeCover(rec, httptest.NewRequest("GET", "/x", nil), model.Book{LibraryItemID: "li_nocover"}) {
		t.Error("missing cover reported as served")
	}
	if rec.Body.Len() != 0 {
		t.Errorf("wrote %q for a missing cover", rec.Body)
	}
}
