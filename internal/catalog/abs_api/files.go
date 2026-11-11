package abs_api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

// Headers passed from the reader to Audiobookshelf, and back. Range support
// matters: reader devices resume and probe downloads.
var (
	requestHeaders  = []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"}
	responseHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"}
)

// CoverExists reports whether the API lists a cover for book.
func (c Client) CoverExists(book model.Book) bool {
	return book.CoverPath != ""
}

// ServeEbook proxies the book's ebook file from Audiobookshelf.
func (c Client) ServeEbook(w http.ResponseWriter, r *http.Request, book model.Book) error {
	path := fmt.Sprintf("/api/items/%s/file/%s/download",
		url.PathEscape(book.LibraryItemID), url.PathEscape(book.Ebook.Ino))
	resp, err := c.proxy(r, path)
	if err != nil {
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return err
	}
	defer resp.Body.Close()
	copyResponse(w, resp)
	return nil
}

// ServeCover proxies the book's cover from Audiobookshelf. It returns false,
// having written nothing, if Audiobookshelf has no cover for it.
func (c Client) ServeCover(w http.ResponseWriter, r *http.Request, book model.Book) bool {
	resp, err := c.proxy(r, "/api/items/"+url.PathEscape(book.LibraryItemID)+"/cover")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false
	}
	copyResponse(w, resp)
	return true
}

// proxy issues the same kind of request r is (GET or HEAD, with its range
// and conditional headers) to Audiobookshelf, authenticated. It uses a client
// with no overall timeout, since ebook downloads can be slow; the request is
// cancelled when the reader disconnects.
func (c Client) proxy(r *http.Request, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, c.url(path, nil), nil)
	if err != nil {
		return nil, err
	}
	for _, h := range requestHeaders {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	hc := &http.Client{}
	if c.HTTP != nil {
		hc.Transport = c.HTTP.Transport
	}
	return hc.Do(req)
}

func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for _, h := range responseHeaders {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
