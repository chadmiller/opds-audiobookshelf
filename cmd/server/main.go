// Command server runs an unauthenticated OPDS catalog for the configured
// libraries of an Audiobookshelf instance, reading either from its HTTP API
// (--api-key) or directly from its sqlite database (--db).
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chadmiller/opds-audiobookshelf/internal/bookmeta"
	"github.com/chadmiller/opds-audiobookshelf/internal/catalog"
	"github.com/chadmiller/opds-audiobookshelf/internal/catalog/abs_api"
	"github.com/chadmiller/opds-audiobookshelf/internal/catalog/abs_direct_db"
	"github.com/chadmiller/opds-audiobookshelf/internal/httpserver"
)

func main() {
	dbPath := flag.String("db", "", "path to Audiobookshelf's absdatabase.sqlite (use this or --api-key)")
	apiKey := flag.String("api-key", "", "Audiobookshelf API key (use this or --db)")
	apiAddress := flag.String("api-address", "", "base URL of the Audiobookshelf instance, with --api-key (default "+defaultAPIAddress+")")
	filesRoot := flag.String("files-root", "", "with --db: filesystem directory that stands in for the root of the filesystem as seen inside the Audiobookshelf container (required with --db, not allowed with --api-key)")
	addr := flag.String("addr", ":8080", "address to listen on")
	title := flag.String("title", "Audiobookshelf Ebooks", "title of the OPDS catalog")
	libraries := flag.String("libraries", "ebooks", "comma-separated list of Audiobookshelf library names to expose")
	scanInterval := flag.Duration("scan-interval", 60*time.Minute, "how often to rescan the database")
	flag.Parse()

	backend, err := chooseBackend(*dbPath, *apiKey, *apiAddress, *filesRoot)
	if err != nil {
		flag.Usage()
		log.Fatal(err)
	}
	libraryNames := splitLibraryNames(*libraries)
	if len(libraryNames) == 0 {
		flag.Usage()
		log.Fatal("-libraries must name at least one library")
	}

	store := catalog.NewStore()
	runScan(store, backend, libraryNames)

	go func() {
		ticker := time.NewTicker(*scanInterval)
		defer ticker.Stop()
		for range ticker.C {
			runScan(store, backend, libraryNames)
		}
	}()

	srv := httpserver.New(store, backend, *title)
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

const defaultAPIAddress = "http://audiobookshelf:80"

// chooseBackend builds the scan source and file server: the API if apiKey is
// given, the database if dbPath is given. Exactly one of them must be given.
// filesRoot is required with the database, and not allowed with the API,
// which serves files itself.
func chooseBackend(dbPath, apiKey, apiAddress, filesRoot string) (catalog.Backend, error) {
	switch {
	case dbPath != "" && apiKey != "":
		return nil, errors.New("-db and -api-key are mutually exclusive")
	case dbPath == "" && apiKey == "":
		return nil, errors.New("one of -db or -api-key is required")
	case dbPath != "":
		if apiAddress != "" {
			return nil, errors.New("-api-address only applies with -api-key")
		}
		if filesRoot == "" {
			return nil, errors.New("-files-root is required with -db")
		}
		return abs_direct_db.Client{Path: dbPath, FilesRoot: filesRoot}, nil
	}
	if filesRoot != "" {
		return nil, errors.New("-files-root only applies with -db; with -api-key files are fetched through the API")
	}
	if apiAddress == "" {
		apiAddress = defaultAPIAddress
	}
	return abs_api.Client{Address: apiAddress, APIKey: apiKey}, nil
}

func runScan(store *catalog.Store, backend catalog.Backend, libraryNames []string) {
	start := time.Now()
	books, err := backend.Scan(libraryNames)
	if err != nil {
		log.Printf("scan failed: %v", err)
		return
	}
	for i := range books {
		books[i].CoverResolvable = backend.CoverExists(books[i])
	}
	bookmeta.MarkSoleInSeries(books)
	bookmeta.SortBooks(books)
	if store.Equal(books) {
		return
	}
	store.Set(books, start)
	log.Printf("scan updates catalog to %d ebooks after taking %s", len(books), time.Since(start))
}
