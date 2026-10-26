package silotest

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// libraryTypes are the library types both apps show: the Apple app's
// SiloMediaType.isSupportedLibrary (Networking/Models.swift), audiobooks
// aside, and the video types of the Android app's mobileMediaModeForLibraryType
// (model/navigation/MediaMode.kt). The apps leave a library of another
// type out of their lists, saying none is available.
var libraryTypes = []string{"movie", "movies", "series", "show", "shows", "tv", "mixed"}

// sectionItemTypes are the card types the Apple app keeps in a section
// (SiloMediaType.isSupportedSectionItem), audiobooks aside: it strips the
// others, and drops a section left empty.
var sectionItemTypes = []string{"movie", "movies", "series", "show", "shows", "tv", "tvshows", "episode", "episodes"}

// complete checks a listing the apps read whole: without a page, or one
// that says nothing follows (Apple APIv2CatalogReadCollection.completeItems,
// Android UserLibrariesV2.project).
func complete(c *checker, o object, where string) {
	page, ok := o["page"].(object)
	if !ok {
		return
	}
	if page["has_more"] == true || page["next_cursor"] != nil && page["next_cursor"] != "" {
		c.fail("%s says more follows: %v", where, page)
	}
}

// CheckLibraries checks the user's libraries as the apps list them
// (GET /api/v2/user/libraries): Apple APIv2UserLibrary and Library(v2:),
// Android UserLibrariesV2.project.
func CheckLibraries(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "libraries", map[string]string{"items": "a"})
	complete(c, doc, "libraries")
	items, _ := doc["items"].([]any)
	seen := map[string]bool{}
	for n, entry := range items {
		library, _ := entry.(object)
		where := fmt.Sprintf("libraries[%d]", n)
		c.has(library, where, map[string]string{"id": "s", "name": "s", "type": "s", "sort_order": "i"})
		id, _ := library["id"].(string)
		if !positiveID(id) {
			c.fail("%s id %q is not a positive integer", where, id)
		}
		if seen[id] {
			c.fail("%s id %q twice", where, id)
		}
		seen[id] = true
		if kind, _ := library["type"].(string); !slices.Contains(libraryTypes, strings.ToLower(strings.TrimSpace(kind))) {
			c.fail("%s type %q, which the apps leave out", where, kind)
		}
		if poster, ok := library["poster_url"]; ok && poster != nil {
			if _, text := poster.(string); !text {
				c.fail("%s poster_url %v", where, poster)
			}
		}
	}
	return c.err()
}

// checkSection checks a section as the apps read one: Apple
// ResolvedSection and SectionItem, with the cards it keeps; Android
// decodeSectionV2, which wants its items.
func checkSection(c *checker, section object, where string) {
	c.has(section, where, map[string]string{"id": "s", "section_type": "s", "title": "s", "items": "a"})
	if id, _ := section["id"].(string); strings.TrimSpace(id) == "" {
		c.fail("%s has no id", where)
	}
	for _, key := range []string{"item_limit", "total_count"} {
		if value, ok := section[key]; ok {
			if n, number := value.(float64); !number || n != float64(int(n)) {
				c.fail("%s.%s %v", where, key, value)
			}
		}
	}
	items, _ := section["items"].([]any)
	for n, entry := range items {
		card, _ := entry.(object)
		at := fmt.Sprintf("%s.items[%d]", where, n)
		checkCard(c, card, at)
		if kind, _ := card["type"].(string); !slices.Contains(sectionItemTypes, strings.ToLower(strings.TrimSpace(kind))) {
			c.fail("%s type %q, which the Apple app strips from a section", at, kind)
		}
	}
}

// checkCard checks a card's keys the apps require: Apple SectionItem and
// BrowseItem, Android SectionItem and BrowseItem.
func checkCard(c *checker, card object, where string) {
	c.has(card, where, map[string]string{"content_id": "s", "type": "s", "title": "s"})
	if id, _ := card["content_id"].(string); strings.TrimSpace(id) == "" {
		c.fail("%s has no content_id", where)
	}
}

// CheckSections checks sections as the apps show Home and a library's
// landing (GET /api/v2/home/sections, GET /api/v2/library/{id}/sections):
// Apple decodeSections, whose section identifiers are unique and not
// empty, and SectionsResponse, which strips the cards of other types;
// Android decodeSectionsV2.
func CheckSections(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "sections", map[string]string{"sections": "a"})
	sections, _ := doc["sections"].([]any)
	seen := map[string]bool{}
	for n, entry := range sections {
		section, _ := entry.(object)
		where := fmt.Sprintf("sections[%d]", n)
		checkSection(c, section, where)
		id, _ := section["id"].(string)
		if seen[id] {
			c.fail("%s id %q twice", where, id)
		}
		seen[id] = true
	}
	return c.err()
}

// CheckSection checks one section, the answer of its items' read
// (GET /api/v2/home/sections/{id}/items, GET /api/v2/library/{id}/sections/{section_id}/items):
// Android wants the section asked.
func CheckSection(data []byte, id string) error {
	var section object
	if err := json.Unmarshal(data, &section); err != nil {
		return err
	}
	c := &checker{}
	checkSection(c, section, "section")
	if section["id"] != id {
		c.fail("section %v, asked %q", section["id"], id)
	}
	return c.err()
}

// CheckCatalogPage checks a page of the catalog as the apps page through
// one (GET /api/v2/catalog): Apple APIv2CatalogPage and its continuation,
// which follows only a new cursor and stops only without one; Android
// CatalogPageWireV2.
func CheckCatalogPage(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "page", map[string]string{"items": "a", "page": "o", "total": "i", "total_exact": "b", "window_cursor": "s"})
	if page, ok := doc["page"].(object); ok {
		c.has(page, "page.page", map[string]string{"has_more": "b"})
		next, _ := page["next_cursor"].(string)
		switch more, _ := page["has_more"].(bool); {
		case more && next == "":
			c.fail("more follows without a cursor")
		case !more && next != "":
			c.fail("a cursor %q though nothing follows", next)
		}
	}
	items, _ := doc["items"].([]any)
	for n, entry := range items {
		card, _ := entry.(object)
		checkCard(c, card, fmt.Sprintf("items[%d]", n))
	}
	return c.err()
}

// CheckCollectionTab checks a library's Collections tab as the apps show
// it (GET /api/v2/library/{id}/collections): Apple
// APIv2LibraryCollectionTab, which wants the library asked, and
// APIv2CuratedCollection; Android LibraryCollectionTabV2 and
// CuratedCollectionV2.
func CheckCollectionTab(data []byte, library string) error {
	var tab object
	if err := json.Unmarshal(data, &tab); err != nil {
		return err
	}
	c := &checker{}
	c.has(tab, "tab", map[string]string{"library_id": "s", "collections": "a", "groups": "a"})
	if tab["library_id"] != library {
		c.fail("tab of library %v, asked %q", tab["library_id"], library)
	}
	collections, _ := tab["collections"].([]any)
	for n, entry := range collections {
		collection, _ := entry.(object)
		c.has(collection, fmt.Sprintf("collections[%d]", n), map[string]string{"id": "s", "title": "s", "collection_type": "s",
			"poster_url": "s", "item_count": "i", "library_id": "s", "library_ids": "a", "sort_order": "i"})
	}
	return c.err()
}
