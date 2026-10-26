package silotest

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// decimalID reports whether text is a positive decimal number, as the
// Android app reads a person's identifier (CatalogDetailReadV2Models
// checkedLongId, AppNavigation's onPersonClick).
func decimalID(text string) bool {
	n, err := strconv.ParseInt(text, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == text
}

func decode(data []byte) (object, error) {
	var doc object
	return doc, json.Unmarshal(data, &doc)
}

// CheckMembershipEntry checks the entry of a favorite or watchlist read
// (GET /api/v2/favorites/{id}, /api/v2/watchlist/{id}): Apple
// personalMembership wants the item asked and a non-empty added_at;
// Android MembershipV2Api the item asked.
func CheckMembershipEntry(data []byte, id string) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "entry", map[string]string{"item_id": "s", "added_at": "s"})
	if doc["item_id"] != id || doc["added_at"] == "" {
		c.fail("entry %v for %q", doc, id)
	}
	return c.err()
}

// CheckPersonalList checks a page of favorites or of the watchlist
// (GET /api/v2/favorites, /api/v2/watchlist): Apple
// APIv2PersonalListPage wants a page, and personalListItems a cursor
// exactly when more follows.
func CheckPersonalList(data []byte) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "list", map[string]string{"items": "a", "page": "o"})
	page, _ := doc["page"].(object)
	c.has(page, "list.page", map[string]string{"has_more": "b"})
	next, _ := page["next_cursor"].(string)
	if more, _ := page["has_more"].(bool); more == (next == "") {
		c.fail("has_more %v with cursor %q", more, next)
	}
	items, _ := doc["items"].([]any)
	for n, entry := range items {
		card, _ := entry.(object)
		checkCard(c, card, fmt.Sprintf("items[%d]", n))
	}
	return c.err()
}

// CheckPerson checks a person's page (GET /api/v2/catalog/people/{id}):
// Apple catalogPerson wants the person asked and a name; Android
// PersonReadV2 a positive decimal id.
func CheckPerson(data []byte, id string) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "person", map[string]string{"id": "s", "name": "s"})
	if doc["id"] != id || !decimalID(id) {
		c.fail("person %v, asked %q", doc["id"], id)
	}
	return c.err()
}

// CheckItemDetail checks the people of a title's page
// (GET /api/v2/catalog/items/{id}): Apple catalogItem wants the title
// asked; the Android app opens a person's page only by a positive decimal
// person_id.
func CheckItemDetail(data []byte, id string) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	if doc["content_id"] != id {
		c.fail("detail of %v, asked %q", doc["content_id"], id)
	}
	for _, list := range []string{"cast", "crew"} {
		people, _ := doc[list].([]any)
		for n, entry := range people {
			credit, _ := entry.(object)
			if person, ok := credit["person_id"].(string); ok && !decimalID(person) {
				c.fail("%s[%d].person_id %q", list, n, person)
			}
		}
	}
	return c.err()
}

// CheckCards checks a list read whole of at most limit cards
// (GET /api/v2/recommendations/similar/{id}): Apple similarCards and
// Android SimilarCardsV2Api want no continuation, unique non-empty
// content_ids, at most limit.
func CheckCards(data []byte, limit int) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "cards", map[string]string{"items": "a"})
	complete(c, doc, "cards")
	items, _ := doc["items"].([]any)
	if len(items) > limit {
		c.fail("%d cards, limit %d", len(items), limit)
	}
	uniqueCards(c, items, "items")
	return c.err()
}

func uniqueCards(c *checker, items []any, where string) {
	seen := map[any]bool{}
	for n, entry := range items {
		card, _ := entry.(object)
		checkCard(c, card, fmt.Sprintf("%s[%d]", where, n))
		if seen[card["content_id"]] {
			c.fail("%s[%d] twice", where, n)
		}
		seen[card["content_id"]] = true
	}
}

// CheckDiscover checks For You (GET /api/v2/recommendations/discover):
// Apple discover and Android DiscoverV2Api want rows with a type, a title
// and cards, unique in each row, and no continuation.
func CheckDiscover(data []byte) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "discover", map[string]string{"items": "a"})
	complete(c, doc, "discover")
	rows, _ := doc["items"].([]any)
	for n, entry := range rows {
		row, _ := entry.(object)
		where := fmt.Sprintf("rows[%d]", n)
		c.has(row, where, map[string]string{"type": "s", "title": "s", "items": "a"})
		items, _ := row["items"].([]any)
		uniqueCards(c, items, where)
	}
	return c.err()
}

// CheckCalendar checks the calendar (GET /api/v2/calendar): Apple
// CalendarResponse wants days with a date and events with a content_id,
// a type and a title, an event once a day; Android DefaultCalendarApi
// non-blank content_ids.
func CheckCalendar(data []byte) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "calendar", map[string]string{"events": "a"})
	days, _ := doc["events"].([]any)
	for n, entry := range days {
		day, _ := entry.(object)
		where := fmt.Sprintf("events[%d]", n)
		c.has(day, where, map[string]string{"date": "s", "items": "a"})
		events, _ := day["items"].([]any)
		seen := map[any]bool{}
		for m, e := range events {
			event, _ := e.(object)
			c.has(event, fmt.Sprintf("%s.items[%d]", where, m), map[string]string{"content_id": "s", "type": "s", "title": "s"})
			if event["content_id"] == "" || seen[event["content_id"]] {
				c.fail("%s.items[%d] content_id %v", where, m, event["content_id"])
			}
			seen[event["content_id"]] = true
		}
	}
	return c.err()
}

// CheckImageCapabilities checks the artwork sizes
// (GET /api/v2/images/capabilities): Apple ImageSizeCapabilityResponse
// wants param, sizes, widths, original_max_width_px and state, and asks
// slot widths only of an available document naming their parameters.
func CheckImageCapabilities(data []byte) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "images", map[string]string{"param": "s", "sizes": "a", "widths": "o", "original_max_width_px": "i", "state": "s",
		"slot_width_params": "o"})
	if doc["state"] != "available" {
		c.fail("images state %v", doc["state"])
	}
	return c.err()
}

// appRequired are the keys each app decodes, as required, of the
// discovery documents Polyfin answers as unsupported (Apple
// APIv2SubtitleProviderStatus, APIv2SubtitleAIStatus,
// APIv2DiagnosticsCapabilities, RequestsFeatureStatus,
// APIv2ApplePushCapability, APIv2SettingsContractCapabilities,
// APIv2OAuthCapabilities, APIv2ExternalSignInCapabilities).
var appRequired = map[string]map[string]string{
	"/api/v2/subtitles/providers/status": {"enabled": "b", "revision": "s", "state": "s", "allowed": "b"},
	"/api/v2/subtitles/ai/status":        {"enabled": "b", "transcribe_enabled": "b", "revision": "s", "state": "s", "allowed": "b"},
	"/api/v2/diagnostics/capabilities": {"allowed": "b", "revision": "s", "state": "s", "server_instance_id": "s",
		"accepted_schema_versions": "a", "max_bundle_bytes": "i", "max_manifest_bytes": "i", "retention_days": "i",
		"consent_notice_version": "i", "upload_chunk_bytes": "i"},
	"/api/v2/requests/status":                 {"requests_enabled": "b", "state": "s"},
	"/api/v2/devices/push/apple/capabilities": {"allowed": "b", "registration_available": "b", "revision": "s", "state": "s"},
	"/api/v2/settings/contract/capabilities": {"revision": "s", "state": "s", "allowed": "b", "manifest_revision": "i",
		"client_families": "a", "supports_batched_effective": "b", "supports_atomic_shortcuts": "b"},
	"/api/v2/auth/oauth/capabilities":            {"state": "s"},
	"/api/v2/auth/external-sign-in/capabilities": {"state": "s"},
}

// CheckUnsupported checks the discovery document of a feature Polyfin does
// not serve: the apps hide a feature whose state is not available (the
// contract's capability head), and decode the keys appRequired names.
func CheckUnsupported(data []byte, path string) error {
	doc, err := decode(data)
	if err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, path, map[string]string{"state": "s", "revision": "s"})
	if doc["state"] == "available" || doc["allowed"] == true || doc["revision"] == "" {
		c.fail("%s advertises the feature: %v", path, doc)
	}
	if keys, ok := appRequired[path]; ok {
		c.has(doc, path, keys)
	}
	return c.err()
}
