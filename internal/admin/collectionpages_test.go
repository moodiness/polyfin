package admin

import (
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/collections"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

// collectionTitlesAddon serves three movies an addon's search finds by
// name: Harbour, Harbour Lights and Lighthouse.
func collectionTitlesAddon(t *testing.T) string {
	t.Helper()
	movies := []stremio.Meta{
		{ID: "tt0101", Type: "movie", Name: "Harbour", ReleaseInfo: "2001"},
		{ID: "tt0102", Type: "movie", Name: "Harbour Lights", ReleaseInfo: "2004"},
		{ID: "tt0103", Type: "movie", Name: "Lighthouse", ReleaseInfo: "1999"},
	}
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "movies", Name: "Movies", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "search", Name: "Search",
					Extra: []stremio.Extra{{Name: "search", IsRequired: true}}}}})
		case strings.HasPrefix(path, "/catalog/movie/search/"):
			extra, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/catalog/movie/search/"), ".json"))
			term := strings.ToLower(strings.TrimPrefix(extra, "search="))
			found := []stremio.Meta{}
			for _, m := range movies {
				if strings.Contains(strings.ToLower(m.Name), term) {
					found = append(found, m)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": found})
		case strings.HasPrefix(path, "/meta/movie/"):
			id := strings.TrimSuffix(strings.TrimPrefix(path, "/meta/movie/"), ".json")
			for _, m := range movies {
				if m.ID == id {
					_ = json.NewEncoder(w).Encode(map[string]any{"meta": m})
					return
				}
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(addon.Close)
	return addon.URL + "/manifest.json"
}

type collectionPageAnswer struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DisplayOrder string `json:"displayOrder"`
	Entries      []struct {
		ID       string  `json:"id"`
		Kind     string  `json:"kind"`
		Name     string  `json:"name"`
		Year     *int    `json:"year"`
		PosterID *string `json:"posterId"`
		Entries  *int    `json:"entries"`
	} `json:"entries"`
	Holders []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"holders"`
}

func (c collectionPageAnswer) names() []string {
	var names []string
	for _, entry := range c.Entries {
		names = append(names, entry.Name)
	}
	return names
}

func (c collectionPageAnswer) ids() []string {
	var ids []string
	for _, entry := range c.Entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

type collectionChoicesAnswer struct {
	Titles []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Held bool   `json:"held"`
	} `json:"titles"`
	Collections []struct {
		ID      string  `json:"id"`
		Name    string  `json:"name"`
		Held    bool    `json:"held"`
		Refusal *string `json:"refusal"`
	} `json:"collections"`
}

func TestAdministratorsManageCollectionsFromContentLibraries(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		if _, err := deps.addons.Install(t.Context(), addons.Shared(), collectionTitlesAddon(t), false); err != nil {
			t.Fatal(err)
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		lib := library.New(deps.pool, deps.addons, deps.client, logger, o.Accounts.Settings)
		o.Collections = collections.New(deps.pool)
		o.CollectionTitles = lib
		o.CollectionSearch = jellyfin.New(jellyfin.Options{Accounts: o.Accounts, Library: lib, Stremio: deps.client, Collections: o.Collections,
			Logger: logger, CacheDir: t.TempDir()})
	})
	admin := api.signedIn("root", true)
	member := api.signedIn("alice", false)
	if status := member.raw(http.MethodGet, "/collections", "", nil); status != http.StatusForbidden {
		t.Errorf("a member lists collections: %d", status)
	}

	create := func(name string) collectionPageAnswer {
		t.Helper()
		var created collectionPageAnswer
		body, _ := json.Marshal(map[string]string{"name": name})
		if status := admin.raw(http.MethodPost, "/collections", string(body), &created); status != http.StatusCreated || created.Name != name ||
			created.DisplayOrder != "premiere" || len(created.Entries) != 0 {
			t.Fatalf("creating %s: %d %+v", name, status, created)
		}
		return created
	}
	if status, body, _ := admin.call(http.MethodPost, "/collections", map[string]string{"name": "  "}); status != http.StatusBadRequest ||
		body["error"] != "invalid_name" {
		t.Errorf("an empty name: %d %v", status, body)
	}
	sea := create("Sea")
	coast := create("Coast")
	trips := create("Trips")

	// The picker finds titles by name, and the collections whose name
	// holds the search.
	titles := map[string]string{}
	var choices collectionChoicesAnswer
	for _, term := range []string{"harbour", "lighthouse"} {
		if status := admin.raw(http.MethodGet, "/collections/"+sea.ID+"/choices?q="+term, "", &choices); status != http.StatusOK || len(choices.Titles) == 0 ||
			len(choices.Collections) != 0 {
			t.Fatalf("choices for %s: %d %+v", term, status, choices)
		}
		for _, title := range choices.Titles {
			titles[title.Name] = title.ID
		}
	}
	if len(titles) != 3 {
		t.Fatalf("titles found: %v", titles)
	}

	change := func(method, path string, body any) (int, collectionPageAnswer, string) {
		t.Helper()
		encoded, _ := json.Marshal(body)
		var answer collectionPageAnswer
		var failure struct {
			Error string `json:"error"`
		}
		status := admin.raw(method, path, string(encoded), &answer)
		if status >= 400 {
			admin.raw(method, path, string(encoded), &failure)
		}
		return status, answer, failure.Error
	}
	add := func(into collectionPageAnswer, ids ...string) (int, collectionPageAnswer, string) {
		t.Helper()
		return change(http.MethodPost, "/collections/"+into.ID+"/entries", map[string][]string{"ids": ids})
	}

	// Titles and collections mixed, in the order added.
	if status, page, _ := add(sea, titles["Harbour"], coast.ID, titles["Lighthouse"]); status != http.StatusOK ||
		!slices.Equal(page.names(), []string{"Harbour", "Coast", "Lighthouse"}) || page.Entries[1].Kind != "collection" ||
		page.Entries[1].Entries == nil || page.Entries[0].Kind != "movie" || page.Entries[0].PosterID == nil || page.Entries[0].Year == nil {
		t.Fatalf("adding: %d %+v", status, page)
	}
	if status, _, code := add(sea, "0123456789abcdef0123456789abcdef"); status != http.StatusBadRequest || code != "unknown_entry" {
		t.Errorf("an unknown entry: %d %s", status, code)
	}
	// Sea holds Coast: Coast cannot hold Sea, nor Sea itself.
	if status, _, code := add(coast, sea.ID); status != http.StatusConflict || code != "collection_cycle" {
		t.Errorf("a cycle: %d %s", status, code)
	}
	if status, _, code := add(sea, sea.ID); status != http.StatusConflict || code != "collection_cycle" {
		t.Errorf("itself: %d %s", status, code)
	}
	// Trips > Sea > Coast > Deep: a fifth level is refused.
	if status, _, _ := add(trips, sea.ID); status != http.StatusOK {
		t.Fatalf("Sea in Trips: %d", status)
	}
	deep := create("Deep")
	if status, _, _ := add(coast, deep.ID); status != http.StatusOK {
		t.Fatalf("Deep in Coast: %d", status)
	}
	deeper := create("Deeper")
	if status, _, code := add(deep, deeper.ID); status != http.StatusConflict || code != "collection_too_deep" {
		t.Errorf("a fifth level: %d %s", status, code)
	}

	// The picker greys out those that would make a cycle or nest too deep.
	if status := admin.raw(http.MethodGet, "/collections/"+coast.ID+"/choices", "", &choices); status != http.StatusOK {
		t.Fatalf("Coast's choices: %d", status)
	}
	refusals := map[string]string{}
	for _, c := range choices.Collections {
		switch {
		case c.Held:
			refusals[c.Name] = "held"
		case c.Refusal != nil:
			refusals[c.Name] = *c.Refusal
		default:
			refusals[c.Name] = ""
		}
	}
	if want := map[string]string{"Sea": "holds", "Trips": "holds", "Deep": "held", "Deeper": ""}; !maps.Equal(refusals, want) {
		t.Errorf("Coast's choices: %v", refusals)
	}
	if status := admin.raw(http.MethodGet, "/collections/"+deep.ID+"/choices", "", &choices); status != http.StatusOK {
		t.Fatalf("Deep's choices: %d", status)
	}
	for _, c := range choices.Collections {
		if c.Name == "Deeper" && (c.Refusal == nil || *c.Refusal != "tooDeep") {
			t.Errorf("Deeper in Deep: %+v", c)
		}
	}

	// Its holders, and the list's counts.
	var page collectionPageAnswer
	if admin.raw(http.MethodGet, "/collections/"+sea.ID, "", &page); len(page.Holders) != 1 || page.Holders[0].Name != "Trips" {
		t.Errorf("Sea's holders: %+v", page.Holders)
	}
	var list []map[string]any
	admin.raw(http.MethodGet, "/collections", "", &list)
	for _, c := range list {
		if c["name"] == "Sea" && (c["entries"] != 3.0 || c["titles"] != 2.0 || c["collections"] != 1.0) {
			t.Errorf("Sea in the list: %v", c)
		}
	}

	// Reordering names every entry, else it is refused.
	order := []string{page.ids()[2], page.ids()[0], page.ids()[1]}
	if status, reordered, _ := change(http.MethodPut, "/collections/"+sea.ID+"/entries/order", map[string][]string{"ids": order}); status != http.StatusOK ||
		!slices.Equal(reordered.names(), []string{"Lighthouse", "Harbour", "Coast"}) {
		t.Errorf("reordering: %d %v", status, reordered.names())
	}
	if status, _, code := change(http.MethodPut, "/collections/"+sea.ID+"/entries/order", map[string][]string{"ids": order[:2]}); status != http.StatusConflict ||
		code != "entries_changed" {
		t.Errorf("an order of some entries: %d %s", status, code)
	}
	// An entry added later comes last.
	if _, added, _ := add(sea, titles["Harbour Lights"]); !slices.Equal(added.names(), []string{"Lighthouse", "Harbour", "Coast", "Harbour Lights"}) {
		t.Errorf("added after reordering: %v", added.names())
	}

	// Renaming, the display order, removing and deleting.
	if status, renamed, _ := change(http.MethodPatch, "/collections/"+sea.ID, map[string]string{"name": "Open sea", "displayOrder": "chosen"}); status != http.StatusOK ||
		renamed.Name != "Open sea" || renamed.DisplayOrder != "chosen" {
		t.Errorf("renaming: %d %+v", status, renamed)
	}
	if status, _, _ := change(http.MethodPatch, "/collections/"+sea.ID, map[string]string{"displayOrder": "random"}); status != http.StatusBadRequest {
		t.Errorf("an unknown display order: %d", status)
	}
	if status, removed, _ := change(http.MethodDelete, "/collections/"+sea.ID+"/entries", map[string][]string{"ids": {coast.ID}}); status != http.StatusOK ||
		!slices.Equal(removed.names(), []string{"Lighthouse", "Harbour", "Harbour Lights"}) {
		t.Errorf("removing Coast: %d %v", status, removed.names())
	}
	if status := admin.raw(http.MethodDelete, "/collections/"+sea.ID, "", nil); status != http.StatusNoContent {
		t.Errorf("deleting: %d", status)
	}
	if status := admin.raw(http.MethodGet, "/collections/"+sea.ID, "", nil); status != http.StatusNotFound {
		t.Errorf("a deleted collection: %d", status)
	}
	if admin.raw(http.MethodGet, "/collections/"+trips.ID, "", &page); len(page.Entries) != 0 {
		t.Errorf("Trips still holds the deleted collection: %+v", page.Entries)
	}
}
