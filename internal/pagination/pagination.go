package pagination

const PageSize = 60

// Paginate returns a slice of items for the requested page (1-indexed).
// It returns the slice, total count, and total pages.
func Paginate[T any](items []T, page int) ([]T, int, int) {
	if page < 1 {
		page = 1
	}

	total := len(items)
	totalPages := (total + PageSize - 1) / PageSize
	if totalPages == 0 {
		totalPages = 1
	}

	if page > totalPages {
		page = totalPages
	}

	start := (page - 1) * PageSize
	end := start + PageSize
	if end > total {
		end = total
	}

	return items[start:end], total, totalPages
}
