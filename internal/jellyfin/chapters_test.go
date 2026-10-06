package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/media"
)

// recordedChapters are the chapters Jellyfin 12.2 returned for the clip
// scripts/jellyfin-fixtures.sh made, where they were asked.
type recordedChapters struct {
	Detail, Listing, NowPlaying []any
	ListingWithoutTheField      bool
}

func TestChaptersAreThoseOfTheVersionOpened(t *testing.T) {
	p := playing(t)
	fixtures := filepath.Join("testdata", "jellyfin-12.2", "chapters")
	data, err := os.ReadFile(filepath.Join(fixtures, "chapters.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded recordedChapters
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	// The first version is the recorded clip; the second, analyzed by
	// playing, has no chapters.
	probe, err := os.ReadFile(filepath.Join(fixtures, "probe.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}
	analysis.Remote = true
	stored, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, stored); err != nil {
		t.Fatal(err)
	}
	chapters := func(what string, body []byte) (any, bool) {
		t.Helper()
		var item map[string]any
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatalf("%s: %v in %s", what, err, body)
		}
		value, ok := item["Chapters"]
		return value, ok
	}
	expect := func(what string, got any, want []any) {
		t.Helper()
		if want == nil {
			want = []any{}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: chapters\n got %v\nwant %v", what, got, want)
		}
	}
	userItem := "/Users/" + p.user.ID.String() + "/Items/"

	_, body := p.call(http.MethodGet, userItem+p.movie, app("tv", p.token), nil)
	got, _ := chapters("title", body)
	expect("title", got, recorded.Detail)
	// jellyfin-web fetches the version the user picked as an item, for its
	// chapters.
	_, body = p.call(http.MethodGet, userItem+p.versions[1].ID.String(), app("tv", p.token), nil)
	got, _ = chapters("second version", body)
	expect("second version", got, nil)
	_, body = p.call(http.MethodGet, userItem+p.versions[0].ID.String(), app("tv", p.token), nil)
	got, _ = chapters("first version", body)
	expect("first version", got, recorded.Detail)

	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	listing := func(query string) map[string]json.RawMessage {
		t.Helper()
		_, body := p.call(http.MethodGet, "/Items?ParentId="+views.Items[0].Id+query, app("tv", p.token), nil)
		var page struct{ Items []json.RawMessage }
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		byID := map[string]json.RawMessage{}
		for _, raw := range page.Items {
			var item struct{ Id string }
			_ = json.Unmarshal(raw, &item)
			byID[item.Id] = raw
		}
		return byID
	}
	listed := listing("&fields=Chapters")
	got, _ = chapters("listed title", listed[p.movie])
	expect("listed title", got, recorded.Listing)
	// The other title's versions were never listed: nothing is known.
	got, _ = chapters("listed title never opened", listed[p.remote])
	expect("listed title never opened", got, nil)
	if _, ok := chapters("listing without the field", listing("")[p.movie]); ok != recorded.ListingWithoutTheField {
		t.Errorf("listing without the field: chapters sent %v, Jellyfin %v", ok, recorded.ListingWithoutTheField)
	}

	nowPlaying := func(source string) any {
		t.Helper()
		if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token),
			map[string]any{"ItemId": p.movie, "MediaSourceId": source, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
			t.Fatalf("report: %d %s", status, data)
		}
		var sessions []map[string]json.RawMessage
		p.get(t, "/Sessions", p.token, &sessions)
		if len(sessions) != 1 {
			t.Fatalf("sessions: %v", sessions)
		}
		got, _ := chapters("now playing", sessions[0]["NowPlayingItem"])
		return got
	}
	expect("playing the first version", nowPlaying(p.movie), recorded.NowPlaying)
	expect("playing the second version", nowPlaying(p.versions[1].ID.String()), nil)
}

func TestChaptersAreAlwaysSent(t *testing.T) {
	p := playing(t)
	probe, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "chapters", "probe.json"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil || len(analysis.Chapters) == 0 {
		t.Fatalf("recorded analysis: %d chapters, %v", len(analysis.Chapters), err)
	}
	analysis.Remote = true
	stored, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", p.versions[0].ID, stored); err != nil {
		t.Fatal(err)
	}
	analyzedAt := func() time.Time {
		t.Helper()
		var at time.Time
		if err := p.pool.QueryRow(t.Context(), "SELECT analyzed_at FROM media_analyses WHERE version_id = $1", p.versions[0].ID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	before := analyzedAt()
	chapters := func(what string, body []byte) []any {
		t.Helper()
		var item map[string]any
		if err := json.Unmarshal(body, &item); err != nil {
			t.Fatalf("%s: %v in %s", what, err, body)
		}
		list, ok := item["Chapters"].([]any)
		if !ok {
			t.Fatalf("%s: chapters %v", what, item["Chapters"])
		}
		return list
	}
	userItem := "/Users/" + p.user.ID.String() + "/Items/"
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	seen := func() map[string]int {
		t.Helper()
		counts := map[string]int{}
		_, body := p.call(http.MethodGet, userItem+p.movie, app("tv", p.token), nil)
		counts["details"] = len(chapters("details", body))
		_, body = p.call(http.MethodGet, userItem+p.versions[0].ID.String(), app("tv", p.token), nil)
		counts["version"] = len(chapters("version opened as an item", body))
		_, body = p.call(http.MethodGet, "/Items?ParentId="+views.Items[0].Id+"&fields=Chapters", app("tv", p.token), nil)
		var page struct{ Items []json.RawMessage }
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		for _, raw := range page.Items {
			var item struct{ Id string }
			_ = json.Unmarshal(raw, &item)
			if item.Id == p.movie {
				counts["listing"] = len(chapters("listing", raw))
			}
		}
		if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token),
			map[string]any{"ItemId": p.movie, "MediaSourceId": p.movie, "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
			t.Fatalf("report: %d %s", status, data)
		}
		var sessions []map[string]json.RawMessage
		p.get(t, "/Sessions", p.token, &sessions)
		if len(sessions) != 1 {
			t.Fatalf("sessions: %v", sessions)
		}
		counts["now playing"] = len(chapters("now playing", sessions[0]["NowPlayingItem"]))
		return counts
	}

	// Every place an app reads an item shows the chapters the analysis
	// kept, with no setting to hide them.
	counts := seen()
	for where, count := range counts {
		if count != len(analysis.Chapters) {
			t.Errorf("%d chapters in %s, want %d", count, where, len(analysis.Chapters))
		}
	}
	if len(counts) != 4 {
		t.Errorf("places checked: %v", counts)
	}
	if after := analyzedAt(); !after.Equal(before) {
		t.Errorf("the version was analyzed again: %v, then %v", before, after)
	}
}

func TestChapterNames(t *testing.T) {
	infos := chapterInfos([]media.Chapter{
		{Title: "Opening"}, {Start: 1_234_567_890}, {Title: "  "}, {Title: "Chapter 04"}, {Title: "00:05:00.000"},
		{Title: "1.02:03:04.1234567"}, {Title: "1:02:03:04"}, {Title: "12:30"}, {Title: "-7"},
		{Title: "24:00"}, {Title: "00:60"}, {Title: "00:05:00.12345678"}, {Title: "1.5"}, {Title: "Part 2: 12:30"},
	}, "en")
	want := []string{"Opening", "Chapter 2", "Chapter 3", "Chapter 04", "Chapter 5",
		"Chapter 6", "Chapter 7", "Chapter 8", "Chapter 9",
		"24:00", "00:60", "00:05:00.12345678", "1.5", "Part 2: 12:30"}
	for i, info := range infos {
		if info.Name != want[i] {
			t.Errorf("chapter %d: %q, want %q", i+1, info.Name, want[i])
		}
	}
	// Starts are kept to the millisecond, as Jellyfin keeps them.
	if infos[1].StartPositionTicks != 12_350_000 {
		t.Errorf("start: %d ticks", infos[1].StartPositionTicks)
	}
}
