package silotest

import "encoding/json"

// CheckLibraryCount checks a library's count as the Android app shows it
// in its phone library dock: CatalogRepository.libraryCount (shared
// repository/CatalogRepository.kt) reads a page of one title of the
// library (GET /api/v2/catalog with limit 1) and keeps its total unless
// total_exact is false, which LibraryDock (androidApp
// ui/screens/libraries/LibraryDock.kt) shows as the library's number of
// titles. page is that page and titles the number of titles the library
// lists when paged through to its end: a total marked exact is that
// number, and one that is not leaves the dock without a count.
func CheckLibraryCount(page []byte, titles int) error {
	var doc object
	if err := json.Unmarshal(page, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "page", map[string]string{"total": "i", "total_exact": "b"})
	total, _ := doc["total"].(float64)
	if doc["total_exact"] == true && int(total) != titles {
		c.fail("total %v marked exact, for %d titles", total, titles)
	}
	return c.err()
}
