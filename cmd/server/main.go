// Command server runs an unauthenticated OPDS 2.0 catalog for the "ebooks"
// library of an Audiobookshelf instance, reading directly from its sqlite
// database.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/chadmiller/audiobookshelf-opds-server/internal/bookmeta"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/catalog"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/dbscan"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/httpserver"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/model"
	"github.com/chadmiller/audiobookshelf-opds-server/internal/pathmap"
)

func main() {
	dbPath := flag.String("db", "", "path to Audiobookshelf's absdatabase.sqlite (required)")
	filesRoot := flag.String("files-root", "", "filesystem directory that stands in for the root of the filesystem as seen inside the Audiobookshelf container (required)")
	addr := flag.String("addr", ":8080", "address to listen on")
	title := flag.String("title", "Audiobookshelf Ebooks", "title of the OPDS catalog")
	scanInterval := flag.Duration("scan-interval", 60*time.Minute, "how often to rescan the database")
	flag.Parse()

	if *dbPath == "" || *filesRoot == "" {
		flag.Usage()
		log.Fatal("-db and -files-root are required")
	}

	store := catalog.NewStore()
	runScan(store, *dbPath, *filesRoot)

	go func() {
		ticker := time.NewTicker(*scanInterval)
		defer ticker.Stop()
		for range ticker.C {
			runScan(store, *dbPath, *filesRoot)
		}
	}()

	srv := httpserver.New(store, *filesRoot, *title)
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatal(err)
	}
}

func runScan(store *catalog.Store, dbPath, filesRoot string) {
	books, err := dbscan.Scan(dbPath)
	if err != nil {
		log.Printf("scan failed: %v", err)
		return
	}
	for i := range books {
		books[i].CoverResolvable = coverExists(filesRoot, books[i])
	}
	bookmeta.SortBooks(books)
	store.Set(books, time.Now())
	log.Printf("scan complete: %d ebooks", len(books))
}

func coverExists(filesRoot string, book model.Book) bool {
	if book.CoverPath == "" {
		return false
	}
	info, err := os.Stat(pathmap.HostPath(filesRoot, book, book.CoverPath))
	return err == nil && !info.IsDir()
}
