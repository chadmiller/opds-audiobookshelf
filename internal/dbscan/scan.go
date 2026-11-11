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
	COALESCE(GROUP_CONCAT(a.lastFirst, '; ' ORDER BY ba.createdAt ASC), '') as authorNamesLastFirst,
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
	b.ebookFile,
	COALESCE(s.name, '') as seriesName,
	COALESCE(bs.sequence, '') as seriesSequence,
	li.mediaType = 'book' as correctMediaType,
	li.isFile <> 0 as bareFile,
	COALESCE(li.isMissing, 0) <> 0 as missing,
	COALESCE(li.isInvalid, 0) <> 0 as invalid,
	b.ebookFile IS NOT NULL as bookFileExists
FROM "libraryItems" li
JOIN "libraries" l ON li.libraryId = l.id
JOIN "books" b ON li.mediaId = b.id
LEFT JOIN "bookAuthors" ba ON b.id = ba.bookId
LEFT JOIN "authors" a ON ba.authorId = a.id
LEFT JOIN "bookSeries" bs ON b.id = bs.bookId
LEFT JOIN "series" s ON bs.seriesId = s.id
WHERE l.name IN (%s)
GROUP BY li.id
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
			seriesName           sql.NullString
			seriesSequence       sql.NullString

			correctMediaType bool
			bareFile         bool
			missing          bool
			invalid          bool
			bookFileExists   bool
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
			&seriesName,
			&seriesSequence,
			&correctMediaType,
			&bareFile,
			&missing,
			&invalid,
			&bookFileExists,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		if !ebookFileJSON.Valid {
			fmt.Printf("Skipping book because of invalid JSON: %s\n", id)
			continue
		}

		if ebookFileJSON.String == "" {
			fmt.Printf("Skipping book because missing ebook file: %s\n", id)
			continue
		}
		var ebook model.EbookFile
		if err := json.Unmarshal([]byte(ebookFileJSON.String), &ebook); err != nil {
			fmt.Printf("Skipping book because of unparsable JSON: %s, %v\n", id, err)
			continue
		}
		if ebook.Metadata.Path == "" {
			fmt.Printf("Skipping book because missing ebook file path: %s\n", id)
			continue
		}

		if !correctMediaType {
			fmt.Printf("Skipping book because of incorrect media type: %s\n", id)
			continue
		}
		if bareFile {
			fmt.Printf("Skipping book because bare files have no side-data: %s\n", id)
			continue
		}
		if missing {
			fmt.Printf("Skipping book because of missing file: %s\n", id)
			continue
		}
		if invalid {
			fmt.Printf("Skipping book because of invalid file: %s\n", id)
			continue
		}
		if !bookFileExists {
			fmt.Printf("Skipping book because of book file is missing: %s\n", id)
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
			SeriesName:           seriesName.String,
			SeriesSequence:       seriesSequence.String,
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
