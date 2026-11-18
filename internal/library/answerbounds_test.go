package library

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/stremio"
)

// An addon's answer of one stream to play among a megabyte of empty
// objects is kept as that stream: the list in memory, counted for what it
// holds, and the list saved.
func TestAStreamListOfEmptyObjectsIsNotKept(t *testing.T) {
	t.Parallel()
	answer := `{"streams":[{"name":"Source","url":"https://f.example/a.mkv"},` + strings.Repeat("{},", 1<<20/3) + `{}]}`
	e := repliedEnv(t, func(w http.ResponseWriter, _ *http.Request, resource string) {
		if resource != "stream" {
			_, _ = w.Write([]byte(`{"subtitles":[]}`))
			return
		}
		_, _ = w.Write([]byte(answer))
	})
	versions, err := e.service.Versions(t.Context(), e.member, e.movie)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %d, %v", len(versions), err)
	}
	kept := 0
	e.service.streamLists.DeleteFunc(func(_ streamKey, l list[stremio.Stream]) bool {
		kept += len(l.items)
		return false
	})
	if kept != 1 {
		t.Errorf("%d streams are kept of an answer of one, want 1", kept)
	}
	if counted := e.service.streamLists.Bytes(); counted < int(unsafe.Sizeof(stremio.Stream{})) || counted > streamListBytes {
		t.Errorf("the stream lists kept count %d bytes for %d streams", counted, kept)
	}
	saved := 0
	e.eventually("the list to be saved", func() bool {
		return e.service.db.QueryRow(t.Context(), "SELECT jsonb_array_length(streams->'streams') FROM stream_lists").Scan(&saved) == nil
	})
	if saved != 1 {
		t.Errorf("%d streams are saved of an answer of one, want 1", saved)
	}
}

// The lists kept of the addons' answers count what they hold in memory,
// and stay within their bytes: the least recently used give way.
func TestTheListsKeptCountTheirMemory(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	// Each list is nearly a third of what its cache may take.
	const stream, subtitle = int(unsafe.Sizeof(stremio.Stream{})), int(unsafe.Sizeof(stremio.Subtitle{}))
	streams := make([]stremio.Stream, streamListBytes/3/stream-1)
	subtitles := make([]stremio.Subtitle, subtitleListBytes/3/subtitle-1)
	notices := slices.Clip(streams[:subtitleListBytes/3/stream-1])
	keepsWithin(t, "stream lists", e.service.streamLists, list[stremio.Stream]{items: streams, answer: len(streams)}, len(streams)*stream, streamListBytes)
	keepsWithin(t, "subtitle lists", e.service.subtitleLists, list[stremio.Subtitle]{items: subtitles, answer: len(subtitles)},
		len(subtitles)*subtitle, subtitleListBytes)
	keepsWithin(t, "answers of notices", e.service.noticed, notices, len(notices)*stream, subtitleListBytes)
}

// keepsWithin checks that lists counts value, whose elements take holds
// bytes, for what it holds, and keeps three of it within most bytes when
// given six. The same value is given under each key: the cache counts it
// once a key, and the test holds it once.
func keepsWithin[V any](t *testing.T, name string, lists *cache.Cache[streamKey, V], value V, holds, most int) {
	t.Helper()
	for i := range 6 {
		lists.Put(streamKey{addon: accounts.ID{1}, contentType: "movie", id: fmt.Sprint("tt", i)}, value)
		if counted := lists.Bytes(); i == 0 && (counted < holds || counted > holds+64) {
			t.Errorf("%s: a list of %d bytes of elements counts %d", name, holds, counted)
		}
	}
	if kept, counted := lists.Len(), lists.Bytes(); kept != 3 || counted > most {
		t.Errorf("%s: %d lists kept counting %d bytes, want 3 within %d", name, kept, counted, most)
	}
}

// A catalog page and a description count what they hold in memory, not
// the few bytes their JSON writes an empty title or an empty video in.
func TestCatalogPagesCountWhatTheyHold(t *testing.T) {
	t.Parallel()
	page := fetched[[]stremio.Meta]{value: make([]stremio.Meta, 100_000)}
	if counted, holds := fetchedSize(page), len(page.value)*int(unsafe.Sizeof(stremio.Meta{})); counted < holds {
		t.Errorf("a page of %d empty titles counts %d bytes, and holds %d", len(page.value), counted, holds)
	}
	described := fetched[stremio.Meta]{value: stremio.Meta{Videos: make([]stremio.Video, 100_000)}}
	if counted, holds := fetchedSize(described), len(described.value.Videos)*int(unsafe.Sizeof(stremio.Video{})); counted < holds {
		t.Errorf("a description of %d empty videos counts %d bytes, and holds %d", len(described.value.Videos), counted, holds)
	}
	// What a title holds beyond itself counts too: its texts and its lists.
	title := stremio.Meta{ID: "tt1", Name: "Title", Description: strings.Repeat("d", 500), Genres: []string{"Drama", "Crime"},
		Videos: []stremio.Video{{ID: "tt1:1:1", Streams: []stremio.Stream{{URL: "https://f.example/a.mkv"}}}}}
	holds := int(unsafe.Sizeof(title)) + 508 + int(unsafe.Sizeof(stremio.Video{})) + int(unsafe.Sizeof(stremio.Stream{}))
	if counted := fetchedSize(fetched[stremio.Meta]{value: title}); counted < holds {
		t.Errorf("a title counts %d bytes, and holds over %d", counted, holds)
	}
}

// A list kept with the items of the list it replaces stops at
// stremio.MaxListed items in all: the answer whole, then the items kept.
func TestAListKeptStopsAtItsBound(t *testing.T) {
	t.Parallel()
	numbered := func(folder string, count int) []stremio.Stream {
		streams := make([]stremio.Stream, count)
		for i := range streams {
			streams[i] = stremio.Stream{URL: fmt.Sprintf("https://f.example/%s/%d.mkv", folder, i)}
		}
		return streams
	}
	answer, replaced := numbered("new", stremio.MaxListed-3), numbered("old", 10)
	merged := streamKind.merge(answer, replaced, time.Time{})
	if len(merged.items) != stremio.MaxListed || merged.answer != len(answer) || merged.items[stremio.MaxListed-1].URL != replaced[2].URL {
		t.Errorf("%d items kept, %d of them the answer's; want %d and %d", len(merged.items), merged.answer, stremio.MaxListed, len(answer))
	}
	// A list within the bound keeps every item of the one it replaces.
	if merged := streamKind.merge(numbered("new", 5), replaced, time.Time{}); len(merged.items) != 15 {
		t.Errorf("a short list keeps %d items, want 15", len(merged.items))
	}
}

// A page or a description kept in the database that is too large for
// memory, as one kept before the lists of an answer were bounded may be,
// is not read at every request: the addon is asked again, and its answer
// replaces it.
func TestKeptAnswersTooLargeForMemoryAreAskedAgain(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := pagedAddon(10, 10)
	e.install(addons.Shared(), addon)
	page := e.children(e.member, "Top", 0, 10)
	if _, err := e.service.Item(t.Context(), e.member, page.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	// The rows grow as an answer of empty objects made them.
	for _, grow := range []string{
		"UPDATE catalog_pages SET metas = metas || (SELECT jsonb_agg('{}'::jsonb) FROM generate_series(1, 2000)) WHERE skip = 0",
		"UPDATE metas SET meta = jsonb_set(meta, '{videos}', (SELECT jsonb_agg('{}'::jsonb) FROM generate_series(1, 5000)))",
	} {
		if tag, err := e.service.db.Exec(t.Context(), grow); err != nil || tag.RowsAffected() != 1 {
			t.Fatalf("%s: %v, %v", grow, tag, err)
		}
	}
	e = e.restarted()
	// Memory keeps less than those rows hold, and more than the addon's
	// answers do.
	const room = 64 << 10
	clock := func() time.Time { return e.service.now() }
	e.service.pages = cache.NewLasting[pageKey, fetched[[]stremio.Meta]](4000, e.service.keptCatalogLife, clock).Sized(room, fetchedSize[[]stremio.Meta])
	e.service.metas = cache.NewLasting[metaKey, fetched[stremio.Meta]](4000, func() time.Duration { return metaKept }, clock).
		Sized(room, fetchedSize[stremio.Meta])
	catalogs, metas := len(addon.catalogRequests()), addon.metaRequests()
	after := e.children(e.member, "Top", 0, 10)
	if !slices.Equal(names(after.Items), names(page.Items)) {
		t.Errorf("the listing: %v, want %v", names(after.Items), names(page.Items))
	}
	item, err := e.service.Item(t.Context(), e.member, page.Items[0].ID)
	if err != nil || item.Overview != "Full description of movie 0" {
		t.Errorf("the item: %+v, %v", item, err)
	}
	if asked := len(addon.catalogRequests()) - catalogs; asked != 1 {
		t.Errorf("the addon was asked for %d pages, want 1", asked)
	}
	if asked := addon.metaRequests() - metas; asked != 1 {
		t.Errorf("the addon was asked for %d descriptions, want 1", asked)
	}
	var titles, videos int
	if err := e.service.db.QueryRow(t.Context(), `SELECT (SELECT jsonb_array_length(metas) FROM catalog_pages WHERE skip = 0),
		(SELECT coalesce(jsonb_array_length(meta->'videos'), 0) FROM metas)`).Scan(&titles, &videos); err != nil || titles != 10 || videos != 0 {
		t.Errorf("the rows hold %d titles and %d videos, %v; want the addon's 10 and none", titles, videos, err)
	}
	for kept, counted := range map[string]int{"pages": e.service.pages.Bytes(), "descriptions": e.service.metas.Bytes()} {
		if counted == 0 || counted > room {
			t.Errorf("memory keeps %d bytes of %s, want the addon's answers within %d", counted, kept, room)
		}
	}
}
