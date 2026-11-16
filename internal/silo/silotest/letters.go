package silotest

import (
	"encoding/json"
	"fmt"
)

// The letters of a list in title order (POST /api/v2/catalog/letters), as
// the apps' alphabetical bars read them:
//
//   - Apple APIv2CatalogLetters (Networking/APIv2/APIv2CatalogModels.swift)
//     requires total, letters and runs, each letter's letter, count and
//     position and each run's letter and position; SiloAPI.catalogLetters
//     (Networking/SiloAPI.swift) takes 404, 405 and 501 as no index. Its
//     BrowseViewModel (Screens/Browse/BrowseViewModel.swift) reads the
//     letters only in title order, shows the bar of the letters listed,
//     jumps to a letter's position with seek, a few rows above it, and pages
//     the whole list with seek up to total (CatalogPositionPager).
//   - Android CatalogLettersV2 (shared network/apiv2/CatalogV2Models.kt)
//     requires total; CatalogV2Api.letters (shared
//     network/apiv2/CatalogV2Api.kt) takes 404, 405 and 501 as no index, and
//     its CatalogLetterJump (androidApp ui/screens/browse/CatalogLetterJump.kt),
//     which the Libraries screen's Browse tab shows (LibrariesScreen.kt),
//     drops a bar of total 0, reads the letter at a position from the run
//     that starts at or before it, and pages the list with seek, sending
//     the first page's window_cursor beside it as the cursor.
//
// So the letters hold every title once: their counts add up to total, each
// letter is A to Z or #, listed once, at the position of its first title,
// in list order. The runs cover the list from 0, in list order, each a
// letter listed, two of one letter never next to each other, and each
// letter's position is that of its first run.
func CheckLetters(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "letters", map[string]string{"total": "i", "letters": "a", "runs": "a"})
	total, _ := doc["total"].(float64)
	letters, _ := doc["letters"].([]any)
	runs, _ := doc["runs"].([]any)
	listed := map[string]int{}
	sum, last := 0, -1
	for n, entry := range letters {
		letter, _ := entry.(object)
		where := fmt.Sprintf("letters[%d]", n)
		c.has(letter, where, map[string]string{"letter": "s", "count": "i", "position": "i"})
		name, _ := letter["letter"].(string)
		count, _ := letter["count"].(float64)
		position, _ := letter["position"].(float64)
		if !knownLetter(name) {
			c.fail("%s.letter %q is not A to Z or #", where, name)
		}
		if _, twice := listed[name]; twice {
			c.fail("%s.letter %q listed twice", where, name)
		}
		listed[name] = int(position)
		if count < 1 || int(position) <= last || position >= total {
			c.fail("%s: %v titles from %v, after %d, of %v", where, count, position, last, total)
		}
		last = int(position)
		sum += int(count)
	}
	if sum != int(total) {
		c.fail("the letters count %d titles, total %v", sum, total)
	}
	first := map[string]int{}
	previous, at := "", -1
	for n, entry := range runs {
		run, _ := entry.(object)
		where := fmt.Sprintf("runs[%d]", n)
		c.has(run, where, map[string]string{"letter": "s", "position": "i"})
		name, _ := run["letter"].(string)
		position, _ := run["position"].(float64)
		if _, ok := listed[name]; !ok {
			c.fail("%s.letter %q is not listed", where, name)
		}
		if n == 0 && position != 0 || int(position) <= at || position >= total || name == previous {
			c.fail("%s: %q from %v, after %q from %d, of %v", where, name, position, previous, at, total)
		}
		if _, ok := first[name]; !ok {
			first[name] = int(position)
		}
		previous, at = name, int(position)
	}
	if total > 0 && len(runs) == 0 {
		c.fail("%v titles and no runs", total)
	}
	for name, position := range listed {
		if p, ok := first[name]; ok && p != position {
			c.fail("letter %q at %d, its first run at %d", name, position, p)
		}
	}
	return c.err()
}

func knownLetter(name string) bool {
	return name == "#" || len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z'
}
