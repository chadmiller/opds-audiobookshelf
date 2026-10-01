// Package dbscan reads the Audiobookshelf sqlite database and extracts the
// rows belonging to the "ebooks" library.
package dbscan

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/chadmiller/opds-audiobookshelf/internal/model"
)

const queryTemplate = `
SELECT
	li.id,
	li.path,
	li.relPath,
	li.authorNamesFirstLast,
	li.authorNamesLastFirst,
	b.title,
	b.titleIgnorePrefix,
	b.subtitle,
	b.publishedYear,
	b.publishedDate,
	b.publisher,
	b.description,
	b.isbn,
	b.asin,
	b.language,
	b.narrators,
	b.tags,
	b.genres,
	b.coverPath,
	b.ebookFile
FROM "libraryItems" li
JOIN "libraries" l ON li.libraryId = l.id
JOIN "books" b ON li.mediaId = b.id
WHERE l.name IN (%s)
	AND li.mediaType = 'book'
	AND li.isFile = 0
	AND COALESCE(li.isMissing, 0) = 0
	AND COALESCE(li.isInvalid, 0) = 0
	AND b.ebookFile IS NOT NULL
`

// Scan opens dbPath read-only, runs the library query restricted to
// libraryNames, and returns the resulting catalog. The database connection
// is closed before returning.
func Scan(dbPath string, libraryNames []string) ([]model.Book, error) {
	if len(libraryNames) == 0 {
		return nil, fmt.Errorf("no library names given")
	}

	placeholders := make([]string, len(libraryNames))
	args := make([]any, len(libraryNames))
	for i, name := range libraryNames {
		placeholders[i] = "?"
		args[i] = name
	}
	query := fmt.Sprintf(queryTemplate, strings.Join(placeholders, ", "))

	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=0", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ebooks: %w", err)
	}
	defer rows.Close()

	var books []model.Book
	for rows.Next() {
		var (
			id                   string
			itemPath             sql.NullString
			itemRelPath          sql.NullString
			authorNamesFirstLast sql.NullString
			authorNamesLastFirst sql.NullString
			title                sql.NullString
			titleIgnorePrefix    sql.NullString
			subtitle             sql.NullString
			publishedYear        sql.NullString
			publishedDate        sql.NullString
			publisher            sql.NullString
			description          sql.NullString
			isbn                 sql.NullString
			asin                 sql.NullString
			language             sql.NullString
			narratorsJSON        sql.NullString
			tagsJSON             sql.NullString
			genresJSON           sql.NullString
			coverPath            sql.NullString
			ebookFileJSON        sql.NullString
		)
		if err := rows.Scan(
			&id,
			&itemPath,
			&itemRelPath,
			&authorNamesFirstLast,
			&authorNamesLastFirst,
			&title,
			&titleIgnorePrefix,
			&subtitle,
			&publishedYear,
			&publishedDate,
			&publisher,
			&description,
			&isbn,
			&asin,
			&language,
			&narratorsJSON,
			&tagsJSON,
			&genresJSON,
			&coverPath,
			&ebookFileJSON,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if !ebookFileJSON.Valid || ebookFileJSON.String == "" {
			continue
		}
		var ebook model.EbookFile
		if err := json.Unmarshal([]byte(ebookFileJSON.String), &ebook); err != nil {
			continue
		}
		if ebook.Metadata.Path == "" {
			continue
		}

		book := model.Book{
			LibraryItemID:        id,
			ItemPath:             itemPath.String,
			ItemRelPath:          itemRelPath.String,
			AuthorName:           authorNamesFirstLast.String,
			AuthorNamesLastFirst: authorNamesLastFirst.String,
			Title:                title.String,
			TitleIgnorePrefix:    titleIgnorePrefix.String,
			Subtitle:             subtitle.String,
			PublishedYear:        publishedYear.String,
			PublishedDate:        publishedDate.String,
			Publisher:            publisher.String,
			Description:          description.String,
			ISBN:                 isbn.String,
			ASIN:                 asin.String,
			Language:             language.String,
			CoverPath:            coverPath.String,
			Narrators:            stringSlice(narratorsJSON),
			Tags:                 stringSlice(tagsJSON),
			Genres:               stringSlice(genresJSON),
			Ebook:                ebook,
		}
		books = append(books, book)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return books, nil
}

func stringSlice(ns sql.NullString) []string {
	if !ns.Valid || ns.String == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(ns.String), &out); err != nil {
		return nil
	}
	return out
}
