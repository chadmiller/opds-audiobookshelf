package feeds_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/feeds"
	atom "github.com/chadmiller/opds-audiobookshelf/internal/feeds/atom"
	opdsjson "github.com/chadmiller/opds-audiobookshelf/internal/feeds/json"
	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

func TestBuildRecentlyAdded(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var books []model.Book
	for i := 0; i < 45; i++ {
		books = append(books, model.Book{
			LibraryItemID:  fmt.Sprintf("li_%02d", i),
			Title:          fmt.Sprintf("Book%02d", i),
			SeriesName:     "Saga",
			SeriesSequence: "1",
			AddedAt:        base.Add(time.Duration(i) * time.Hour),
		})
	}
	for name, b := range map[string]feeds.Builder{"json": opdsjson.JSONFeed{}, "atom": atom.AtomFeed{}} {
		var buf bytes.Buffer
		if err := b.BuildRecentlyAdded("", books, base).Encode(&buf); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if n := strings.Count(out, "/opds/-/download/"); n != 40 {
			t.Errorf("%s: %d items, want 40", name, n)
		}
		if !strings.Contains(out, "(Saga #") || !strings.Contains(out, "Book44") || strings.Contains(out, "Book04") {
			t.Errorf("%s: wrong contents: %.300s", name, out)
		}
		if strings.Index(out, "Book44") > strings.Index(out, "Book43") {
			t.Errorf("%s: not newest first", name)
		}
		if !strings.Contains(out, "/opds/recent/") {
			t.Errorf("%s: missing self link", name)
		}

		buf.Reset()
		if err := b.BuildRootFeed("", "T", books, base).Encode(&buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "/opds/recent/") {
			t.Errorf("%s: root lacks recent entry", name)
		}
	}
}
