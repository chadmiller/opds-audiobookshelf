// Command server runs an unauthenticated OPDS catalog for the configured
// libraries of an Audiobookshelf instance, reading directly from its sqlite
// database.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
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
	libraries := flag.String("libraries", "ebooks", "comma-separated list of Audiobookshelf library names to expose")
	scanInterval := flag.Duration("scan-interval", 60*time.Minute, "how often to rescan the database")
	flag.Parse()

	if *dbPath == "" || *filesRoot == "" {
		flag.Usage()
		log.Fatal("-db and -files-root are required")
	}
	libraryNames := splitLibraryNames(*libraries)
	if len(libraryNames) == 0 {
		flag.Usage()
		log.Fatal("-libraries must name at least one library")
	}

	store := catalog.NewStore()
	runScan(store, *dbPath, *filesRoot, libraryNames)

	go func() {
		ticker := time.NewTicker(*scanInterval)
		defer ticker.Stop()
		for range ticker.C {
			runScan(store, *dbPath, *filesRoot, libraryNames)
		}
	}()

	srv := httpserver.New(store, *filesRoot, *title)
	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv); err != nil {
		log.Fatal(err)
	}
}

func splitLibraryNames(s string) []string {
	var names []string
	for _, name := range strings.Split(s, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func runScan(store *catalog.Store, dbPath, filesRoot string, libraryNames []string) {
	books, err := dbscan.Scan(dbPath, libraryNames)
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
