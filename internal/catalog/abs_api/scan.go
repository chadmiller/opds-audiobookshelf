// Package audiobookshelf extracts the ebook catalog from a running
// Audiobookshelf instance using its public HTTP API.
package abs_api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

const pageSize = 100

// Client is a catalog.Source that talks to the Audiobookshelf API, and a
// catalog.Files that proxies book files and covers from it.
type Client struct {
	Address string // base URL, e.g. http://audiobookshelf:80
	APIKey  string
	HTTP    *http.Client // optional
}

type librariesResponse struct {
	Libraries []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"libraries"`
}

type listResponse struct {
	Results []struct {
		ID string `json:"id"`
	} `json:"results"`
	Total int `json:"total"`
}

// libraryItem is the expanded item shape from GET /api/items/{id}.
type libraryItem struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	RelPath   string `json:"relPath"`
	IsFile    bool   `json:"isFile"`
	IsMissing bool   `json:"isMissing"`
	IsInvalid bool   `json:"isInvalid"`
	AddedAt   int64  `json:"addedAt"` // Unix milliseconds
	MediaType string `json:"mediaType"`
	Media     struct {
		CoverPath string           `json:"coverPath"`
		Tags      []string         `json:"tags"`
		Metadata  metadata         `json:"metadata"`
		EbookFile *model.EbookFile `json:"ebookFile"`
	} `json:"media"`
}

type metadata struct {
	Title             string   `json:"title"`
	TitleIgnorePrefix string   `json:"titleIgnorePrefix"`
	Subtitle          string   `json:"subtitle"`
	PublishedYear     string   `json:"publishedYear"`
	PublishedDate     string   `json:"publishedDate"`
	Publisher         string   `json:"publisher"`
	Description       string   `json:"description"`
	ISBN              string   `json:"isbn"`
	ASIN              string   `json:"asin"`
	Language          string   `json:"language"`
	Narrators         []string `json:"narrators"`
	Genres            []string `json:"genres"`
	AuthorName        string   `json:"authorName"`
	AuthorNameLF      string   `json:"authorNameLF"`
	Authors           []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Series []struct {
		Name     string `json:"name"`
		Sequence string `json:"sequence"`
	} `json:"series"`
}

// Scan returns every usable ebook in the named libraries. A library that
// cannot be found or listed is an error; an individual item that cannot be
// fetched or is unusable is logged and skipped.
func (c Client) Scan(libraryNames []string) ([]model.Book, error) {
	if len(libraryNames) == 0 {
		return nil, fmt.Errorf("no library names given")
	}

	var libs librariesResponse
	if err := c.get("/api/libraries", nil, &libs); err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}
	ids := map[string]string{}
	for _, l := range libs.Libraries {
		ids[l.Name] = l.ID
	}

	var books []model.Book
	for _, name := range libraryNames {
		id, ok := ids[name]
		if !ok {
			return nil, fmt.Errorf("library %q not found", name)
		}
		itemIDs, err := c.listItemIDs(id)
		if err != nil {
			return nil, fmt.Errorf("list items of library %q: %w", name, err)
		}
		for _, itemID := range itemIDs {
			var item libraryItem
			if err := c.get("/api/items/"+url.PathEscape(itemID), nil, &item); err != nil {
				log.Printf("Skipping book because item fetch failed: %s, %v", itemID, err)
				continue
			}
			if book, reason := itemToBook(item); reason != "" {
				log.Printf("Skipping book because %s: %s", reason, itemID)
			} else {
				books = append(books, book)
			}
		}
	}
	return books, nil
}

func (c Client) listItemIDs(libraryID string) ([]string, error) {
	var ids []string
	for page := 0; ; page++ {
		q := url.Values{"limit": {fmt.Sprint(pageSize)}, "page": {fmt.Sprint(page)}}
		var resp listResponse
		if err := c.get("/api/libraries/"+url.PathEscape(libraryID)+"/items", q, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Results {
			ids = append(ids, r.ID)
		}
		if len(resp.Results) == 0 || len(ids) >= resp.Total {
			return ids, nil
		}
	}
}

// itemToBook converts an API item to a Book. A non-empty reason means the
// item is unusable and should be skipped.
func itemToBook(item libraryItem) (book model.Book, skipReason string) {
	switch {
	case item.MediaType != "book":
		return book, "of incorrect media type"
	case item.IsFile:
		return book, "bare files have no side-data"
	case item.IsMissing:
		return book, "of missing file"
	case item.IsInvalid:
		return book, "of invalid file"
	case item.Media.EbookFile == nil:
		return book, "book file is missing"
	case item.Media.EbookFile.Metadata.Path == "":
		return book, "missing ebook file path"
	}

	m := item.Media.Metadata
	authorName := m.AuthorName
	if authorName == "" {
		names := make([]string, len(m.Authors))
		for i, a := range m.Authors {
			names[i] = a.Name
		}
		authorName = strings.Join(names, ", ")
	}
	book = model.Book{
		LibraryItemID:        item.ID,
		ItemPath:             item.Path,
		ItemRelPath:          item.RelPath,
		AuthorName:           authorName,
		AuthorNamesLastFirst: m.AuthorNameLF,
		Title:                m.Title,
		TitleIgnorePrefix:    m.TitleIgnorePrefix,
		Subtitle:             m.Subtitle,
		PublishedYear:        m.PublishedYear,
		PublishedDate:        m.PublishedDate,
		Publisher:            m.Publisher,
		Description:          m.Description,
		ISBN:                 m.ISBN,
		ASIN:                 m.ASIN,
		Language:             m.Language,
		Narrators:            m.Narrators,
		Tags:                 item.Media.Tags,
		Genres:               m.Genres,
		CoverPath:            item.Media.CoverPath,
		Ebook:                *item.Media.EbookFile,
	}
	if item.AddedAt > 0 {
		book.AddedAt = time.UnixMilli(item.AddedAt)
	}
	if len(m.Series) > 0 {
		book.SeriesName = m.Series[0].Name
		book.SeriesSequence = m.Series[0].Sequence
	}
	return book, ""
}

func (c Client) url(path string, query url.Values) string {
	u := strings.TrimRight(c.Address, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c Client) get(path string, query url.Values, out any) error {
	req, err := http.NewRequest(http.MethodGet, c.url(path, query), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("fetch %s: status %d: %s", path, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
