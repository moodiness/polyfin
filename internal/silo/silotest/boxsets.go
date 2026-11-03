package silotest

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// checkBoxSet checks a box set as the apps decode one: Apple CatalogBoxSet
// (Networking/BoxSetModels.swift), whose movie_count and played_count are
// not optional and whose appLibraryId opens it only for a positive
// integer library_id; Android CatalogBoxSet (model/catalog/BoxSetModels.kt).
// Its played movies are some of its movies: both apps draw
// played_count / movie_count as the share watched (Apple watchedProgress,
// Android watchedFraction).
func checkBoxSet(c *checker, set object, where string) {
	c.has(set, where, map[string]string{"id": "s", "library_id": "s", "title": "s", "movie_count": "i", "played_count": "i"})
	if id, _ := set["id"].(string); id == "" {
		c.fail("%s has no id", where)
	}
	if !positiveID(set["library_id"]) {
		c.fail("%s.library_id %v, which the Apple app cannot open", where, set["library_id"])
	}
	movies, _ := set["movie_count"].(float64)
	played, _ := set["played_count"].(float64)
	if movies < 1 || played < 0 || played > movies {
		c.fail("%s has %v of %v movies played", where, played, movies)
	}
	for _, key := range []string{"poster_url", "backdrop_url", "overview"} {
		if value, ok := set[key]; ok {
			if text, _ := value.(string); text == "" {
				c.fail("%s.%s %v", where, key, value)
			}
		}
	}
	for _, key := range []string{"first_year", "last_year"} {
		if value, ok := set[key]; ok {
			if n, number := value.(float64); !number || n != float64(int(n)) {
				c.fail("%s.%s %v", where, key, value)
			}
		}
	}
}

// CheckBrowseUngrouped checks a library's page asked as the apps at the
// commits followed ask it of a library with box_sets_enabled, with
// group=box_set (Apple CatalogQueryBuilder.build's groupsBoxSets and
// Library.browsesBoxSets in Networking/Models.swift; Android
// LibrariesViewModel.browseGroup in ui/screens/libraries/LibrariesScreen.kt),
// against the same page asked without it. Silo no longer groups a
// library's grid (silo-server a5325887, contract 527e4043, which drops
// group=box_set and CatalogItem.box_set), and its apps since no longer
// ask it (silo-apple 9723df00 CatalogQueryBuilder.swift, silo-android
// 6f3e3682 LibrariesScreen.kt): grouped is the page without the group,
// as it is, and holds no box set card, which the apps followed open as a
// box set only with its box set (Apple BrowseItem.isBoxSet in
// Networking/BoxSetModels.swift, Android BrowseItem.boxSetCard in
// model/catalog/BoxSetModels.kt).
func CheckBrowseUngrouped(grouped, plain []byte) error {
	var g, p object
	if err := json.Unmarshal(grouped, &g); err != nil {
		return err
	}
	if err := json.Unmarshal(plain, &p); err != nil {
		return err
	}
	c := &checker{}
	if !reflect.DeepEqual(g, p) {
		c.fail("the page asked with group=box_set is not the page without it")
	}
	items, _ := g["items"].([]any)
	for n, entry := range items {
		card, _ := entry.(object)
		if _, ok := card["box_set"]; ok || card["type"] == "box_set" {
			c.fail("items[%d] is a box set card", n)
		}
	}
	return c.err()
}

// CheckBoxSet checks a box set read with its movies (GET
// /api/v2/catalog/box-sets/{id}): Apple CatalogBoxSetDetail, read by
// catalogBoxSet, which refuses another box set than the one asked; Android
// CatalogBoxSetDetailWireV2. Its movies are cards of the movies counted.
func CheckBoxSet(data []byte, id, library string) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	checkBoxSetDetail(c, doc, "box set", id, library)
	return c.err()
}

func checkBoxSetDetail(c *checker, doc object, where, id, library string) {
	checkBoxSet(c, doc, where)
	c.has(doc, where, map[string]string{"items": "a"})
	if doc["id"] != id {
		c.fail("%s %v, asked %q", where, doc["id"], id)
	}
	if library != "" && doc["library_id"] != library {
		c.fail("%s of library %v, asked %q", where, doc["library_id"], library)
	}
	items, _ := doc["items"].([]any)
	if movies, _ := doc["movie_count"].(float64); int(movies) != len(items) {
		c.fail("%s counts %v movies and lists %d", where, doc["movie_count"], len(items))
	}
	uniqueCards(c, items, where)
	for n, entry := range items {
		card, _ := entry.(object)
		at := fmt.Sprintf("%s.items[%d]", where, n)
		checkCard(c, card, at)
		if card["type"] != "movie" {
			c.fail("%s type %v in a box set", at, card["type"])
		}
	}
}

// CheckItemBoxSet checks the box set a movie's page reads (GET
// /api/v2/catalog/items/{id}/box-set): {} for none, else box_set, as
// CheckBoxSet: Apple CatalogItemBoxSet, Android CatalogItemBoxSetWireV2.
// It holds the movie asked.
func CheckItemBoxSet(data []byte, movie string) (bool, error) {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return false, err
	}
	set, ok := doc["box_set"]
	if !ok {
		return false, nil
	}
	o, _ := set.(object)
	c := &checker{}
	if o == nil {
		c.fail("box_set %v", set)
		return true, c.err()
	}
	id, _ := o["id"].(string)
	checkBoxSetDetail(c, o, "box_set", id, "")
	items, _ := o["items"].([]any)
	found := false
	for _, entry := range items {
		if card, _ := entry.(object); card["content_id"] == movie {
			found = true
		}
	}
	if !found {
		c.fail("the box set of %q does not hold it", movie)
	}
	return true, c.err()
}

// CheckBoxSetSearch checks the box sets a search finds (GET
// /api/v2/catalog/box-sets): Apple CatalogBoxSetList, Android
// CatalogBoxSetListWireV2, at most limit of them, each once: the Android
// app makes a card of each, box_set:<library_id>:<id>
// (CatalogBoxSet.toSearchCard).
func CheckBoxSetSearch(data []byte, limit int) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "search", map[string]string{"items": "a"})
	items, _ := doc["items"].([]any)
	if len(items) > limit {
		c.fail("%d box sets, asked at most %d", len(items), limit)
	}
	seen := map[string]bool{}
	for n, entry := range items {
		set, _ := entry.(object)
		where := fmt.Sprintf("items[%d]", n)
		checkBoxSet(c, set, where)
		key := fmt.Sprintf("%v:%v", set["library_id"], set["id"])
		if seen[key] {
			c.fail("%s %s twice", where, key)
		}
		seen[key] = true
	}
	return c.err()
}
