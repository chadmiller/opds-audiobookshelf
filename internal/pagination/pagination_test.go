package pagination

import (
	"testing"
)

func TestPaginate(t *testing.T) {
	// Create test data
	items := make([]int, 150)
	for i := 0; i < 150; i++ {
		items[i] = i + 1
	}

	tests := []struct {
		page          int
		expectedCount int
		expectedFirst int
		expectedLast  int
		expectedTotal int
		expectedPages int
	}{
		{page: 1, expectedCount: 60, expectedFirst: 1, expectedLast: 60, expectedTotal: 150, expectedPages: 3},
		{page: 2, expectedCount: 60, expectedFirst: 61, expectedLast: 120, expectedTotal: 150, expectedPages: 3},
		{page: 3, expectedCount: 30, expectedFirst: 121, expectedLast: 150, expectedTotal: 150, expectedPages: 3},
		{page: 0, expectedCount: 60, expectedFirst: 1, expectedLast: 60, expectedTotal: 150, expectedPages: 3},  // invalid page should default to 1
		{page: 4, expectedCount: 30, expectedFirst: 121, expectedLast: 150, expectedTotal: 150, expectedPages: 3}, // out of range should clamp to last page
	}

	for _, tt := range tests {
		pageItems, total, totalPages := Paginate(items, tt.page)

		if len(pageItems) != tt.expectedCount {
			t.Errorf("Page %d: expected %d items, got %d", tt.page, tt.expectedCount, len(pageItems))
		}
		if total != tt.expectedTotal {
			t.Errorf("Page %d: expected total %d, got %d", tt.page, tt.expectedTotal, total)
		}
		if totalPages != tt.expectedPages {
			t.Errorf("Page %d: expected %d total pages, got %d", tt.page, tt.expectedPages, totalPages)
		}
		if len(pageItems) > 0 {
			if pageItems[0] != tt.expectedFirst {
				t.Errorf("Page %d: expected first item %d, got %d", tt.page, tt.expectedFirst, pageItems[0])
			}
			if pageItems[len(pageItems)-1] != tt.expectedLast {
				t.Errorf("Page %d: expected last item %d, got %d", tt.page, tt.expectedLast, pageItems[len(pageItems)-1])
			}
		}
	}
}

func TestPaginateEmptyList(t *testing.T) {
	items := []int{}
	pageItems, total, totalPages := Paginate(items, 1)

	if len(pageItems) != 0 {
		t.Errorf("Expected 0 items, got %d", len(pageItems))
	}
	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
	if totalPages != 1 {
		t.Errorf("Expected 1 total page for empty list, got %d", totalPages)
	}
}

func TestPaginateSmallList(t *testing.T) {
	items := make([]int, 10)
	for i := 0; i < 10; i++ {
		items[i] = i + 1
	}

	pageItems, total, totalPages := Paginate(items, 1)

	if len(pageItems) != 10 {
		t.Errorf("Expected 10 items, got %d", len(pageItems))
	}
	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
	if totalPages != 1 {
		t.Errorf("Expected 1 total page, got %d", totalPages)
	}
}
