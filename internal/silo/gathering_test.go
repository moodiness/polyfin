package silo

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// collectionsView answers Polyfin's Collections view as the Jellyfin API
// does: a library of collections it lists no title of, whose collections
// it lists a page at a time, 100 unless asked otherwise, telling how many
// titles the user sees in them when asked, and each list theirs. It counts
// the pages of collections listed and the collections read.
type collectionsView struct {
	collections []map[string]any
	titles      map[string][]any
	// read, when set, is told each collection as it is read.
	read func()

	mu    sync.Mutex
	pages int
	reads map[string]int
}

// add adds a collection of the titles named.
func (c *collectionsView) add(name string, titles ...string) string {
	id := fmt.Sprintf("%032x", 0xc0000+len(c.collections))
	c.collections = append(c.collections, map[string]any{"Id": id, "Name": name, "Type": "BoxSet", "ChildCount": len(titles)})
	if c.titles == nil {
		c.titles = map[string][]any{}
	}
	for _, title := range titles {
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(title))
		c.titles[id] = append(c.titles[id], map[string]any{"Id": fmt.Sprintf("%032x", hash.Sum64()), "Name": title, "Type": "Movie"})
	}
	return id
}

func (c *collectionsView) answer(w http.ResponseWriter, asked call) {
	switch asked.path {
	case "/UserViews":
		jsonAnswer(w, map[string]any{"Items": []any{map[string]any{"Id": testLibrary, "Name": "Collections", "CollectionType": boxsets}}})
	case "/Polyfin/HomeRows", "/Shows/NextUp", "/UserItems/Resume":
		jsonAnswer(w, map[string]any{"Items": []any{}})
	case "/Users/Me":
		jsonAnswer(w, map[string]any{"Configuration": map[string]any{}})
	case "/Items":
		parent := asked.query.Get("ParentId")
		switch {
		case parent == testLibrary && asked.query.Get("Recursive") == "true":
			jsonAnswer(w, map[string]any{"Items": []any{}, "TotalRecordCount": 0})
		case parent == testLibrary:
			c.mu.Lock()
			c.pages++
			c.mu.Unlock()
			start, _ := strconv.Atoi(asked.query.Get("StartIndex"))
			limit, err := strconv.Atoi(asked.query.Get("Limit"))
			if err != nil {
				limit = 100
			}
			from, to := min(start, len(c.collections)), min(start+limit, len(c.collections))
			listed := make([]map[string]any, 0, to-from)
			for _, collection := range c.collections[from:to] {
				shown := map[string]any{}
				for key, value := range collection {
					if key != "ChildCount" || strings.Contains(asked.query.Get("Fields"), "ChildCount") {
						shown[key] = value
					}
				}
				listed = append(listed, shown)
			}
			jsonAnswer(w, map[string]any{"Items": listed, "TotalRecordCount": len(c.collections)})
		default:
			if c.read != nil {
				c.read()
			}
			c.mu.Lock()
			if c.reads == nil {
				c.reads = map[string]int{}
			}
			c.reads[parent]++
			c.mu.Unlock()
			jsonAnswer(w, map[string]any{"Items": c.titles[parent], "TotalRecordCount": len(c.titles[parent])})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// readTimes is how many times the collections named were read, all of them
// when none is named.
func (c *collectionsView) readTimes(ids ...string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	times := 0
	for id, count := range c.reads {
		if len(ids) == 0 || slices.Contains(ids, id) {
			times += count
		}
	}
	return times
}

// pagesListed is how many pages of collections were listed.
func (c *collectionsView) pagesListed() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pages
}

// collectionsRow is the row of the Collections view on Home: its titles.
func (h *harness) collectionsRow() []string {
	h.t.Helper()
	w := h.do(http.MethodGet, "/api/v2/home/sections", nil)
	var home struct {
		Sections []struct {
			Type  string        `json:"section_type"`
			Title string        `json:"title"`
			Items []catalogCard `json:"items"`
		} `json:"sections"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &home) != nil {
		h.t.Fatalf("Home: %d %s", w.Code, w.Body)
	}
	var titles []string
	for _, section := range home.Sections {
		if section.Type == collectionsSection && section.Title == "Collections" {
			for _, card := range section.Items {
				titles = append(titles, card.Title)
			}
		}
	}
	return titles
}

// Home's row of the Collections view reads the collections that hold a
// title for the user, in their order, and none of the others: its titles
// are the same, and its reads as many, however many empty collections the
// view holds.
func TestHomeReadsOnlyCollectionsWithTitles(t *testing.T) {
	t.Parallel()
	for _, empty := range []int{3, 60} {
		view := &collectionsView{}
		var without []string
		for n := range empty {
			without = append(without, view.add("Empty "+strconv.Itoa(n)))
		}
		view.add("Picks", "One", "Two")
		view.add("Sequels", "Two", "Three")
		h := newHarness(t, view.answer)
		if titles, want := h.collectionsRow(), []string{"One", "Two", "Three"}; !slices.Equal(titles, want) {
			t.Errorf("%d empty collections: the row lists %q, want %q", empty, titles, want)
		}
		if read := view.readTimes(without...); read != 0 {
			t.Errorf("%d empty collections: read %d times", empty, read)
		}
		if read := view.readTimes(); read != 2 {
			t.Errorf("%d empty collections: %d collections read, want 2", empty, read)
		}
	}
}

// The titles of a library of collections are gathered with at most
// maxListings listings, of its collections and of their titles: those that
// add no title, as they hold one already listed, are not read without end.
func TestALibraryOfCollectionsReadsABoundedNumberOfThem(t *testing.T) {
	t.Parallel()
	view := &collectionsView{}
	for n := range maxListings + 50 {
		view.add("Again "+strconv.Itoa(n), "One")
	}
	h := newHarness(t, view.answer)
	if titles := h.collectionsRow(); !slices.Equal(titles, []string{"One"}) {
		t.Errorf("the row lists %q", titles)
	}
	if listed := view.pagesListed() + view.readTimes(); listed != maxListings {
		t.Errorf("%d listings asked, want %d", listed, maxListings)
	}
}

// The collections of a library of collections are listed past a first
// page that holds no title for the user: the titles of those that follow
// are gathered.
func TestALibraryOfCollectionsListsPastEmptyCollections(t *testing.T) {
	t.Parallel()
	view := &collectionsView{}
	var without []string
	for n := range 2*scanChunk + 10 {
		without = append(without, view.add("Empty "+strconv.Itoa(n)))
	}
	view.add("Picks", "One", "Two")
	h := newHarness(t, view.answer)
	if titles, want := h.collectionsRow(), []string{"One", "Two"}; !slices.Equal(titles, want) {
		t.Errorf("the row lists %q, want %q", titles, want)
	}
	if read := view.readTimes(without...); read != 0 {
		t.Errorf("empty collections read %d times", read)
	}
	if pages := view.pagesListed(); pages != 3 {
		t.Errorf("%d pages of collections listed, want 3", pages)
	}
}

// Once the request's context is done, no further collection is read for
// the titles of a library of collections.
func TestGatheringCollectionsStopsWithItsRequest(t *testing.T) {
	t.Parallel()
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	view := &collectionsView{read: leave}
	for n := range 5 {
		view.add("Picks "+strconv.Itoa(n), "Title "+strconv.Itoa(n))
	}
	h := newHarness(t, view.answer)
	h.server.ServeHTTP(httptest.NewRecorder(), h.request(ctx, http.MethodGet, "/api/v2/home/sections", nil))
	if read := view.readTimes(); read != 1 {
		t.Errorf("%d collections read, want the one the request ended at", read)
	}
}

// Once the request's context is done, the titles of no further collection
// are counted for a Collections tab: only the counts under way end.
func TestCountingCollectionsStopsWithItsRequest(t *testing.T) {
	t.Parallel()
	ctx, leave := context.WithCancel(t.Context())
	defer leave()
	view := &collectionsView{read: leave}
	var uncounted []item
	for n := range 40 {
		uncounted = append(uncounted, item{Id: view.add("Picks "+strconv.Itoa(n), "Title "+strconv.Itoa(n)), Type: "BoxSet"})
	}
	h := newHarness(t, view.answer)
	if _, err := h.server.collectionCounts(h.request(ctx, http.MethodGet, "/api/v2/library/1/collections", nil), viewer{}, uncounted); err == nil {
		t.Error("the counts of a request that ended were answered")
	}
	if read := view.readTimes(); read > collectionReads {
		t.Errorf("%d collections counted, want at most the %d under way", read, collectionReads)
	}
}
