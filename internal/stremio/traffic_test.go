package stremio

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Each request to an addon counts under the work it was made for: a
// request no work marked is one users wait for, background work keeps
// the kind its caller named, and other background work counts as other.
// Failures count by origin too.
func TestTrafficCountsTheRequestsByOrigin(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/fail/") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"streams":[]}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/manifest.json"
	client := NewClient("test")
	ask := func(ctx context.Context, id string) {
		t.Helper()
		_, _ = client.Streams(ctx, manifest, "movie", id, false)
	}

	ask(t.Context(), "tt1")
	ask(t.Context(), "tt2")
	preload := WithOrigin(t.Context(), OriginPreload)
	for range 3 {
		ask(preload, "tt3")
	}
	// Background work started inside a kind of background work keeps it.
	ask(InBackground(preload), "tt4")
	ask(InBackground(t.Context()), "tt5")
	ask(WithOrigin(t.Context(), OriginWatchlists), "fail/tt6")

	health, _ := client.Health(manifest)
	want := Counts{}
	want.Requests[OriginApps], want.Requests[OriginPreload], want.Requests[OriginOther] = 2, 4, 1
	want.Requests[OriginWatchlists], want.Failures[OriginWatchlists] = 1, 1
	if health.Traffic.Hour != want || health.Traffic.Day != want {
		t.Errorf("traffic: %+v, want %+v in both", health.Traffic, want)
	}
	if health.Requests != 8 || health.Failures != 1 {
		t.Errorf("health: %+v", health)
	}
	if OriginApps.String() != "apps" || OriginPreload.String() != "preload" || OriginOther.String() != "other" {
		t.Errorf("names: %s %s %s", OriginApps, OriginPreload, OriginOther)
	}
}

// Requests counted at once, across a new minute that empties a bucket,
// are all counted, with no lock taken but the bucket's emptying.
func TestTrafficCountsRequestsCountedAtOnce(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	var tr traffic
	// The buckets of start, a day before, hold a request: the first
	// request of start empties them.
	tr.count(start.Add(-24*time.Hour), OriginApps, false)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			for range 500 {
				tr.count(start, Origin(i%int(origins)), i%2 == 0)
			}
		})
	}
	wg.Wait()
	got := tr.totals(start)
	total, failed := 0, 0
	for origin := range got.Hour.Requests {
		total += got.Hour.Requests[origin]
		failed += got.Hour.Failures[origin]
	}
	if total != 8*500 || failed != 4*500 || got.Day != got.Hour {
		t.Errorf("%d requests, %d failed, %+v", total, failed, got)
	}
}

// The last hour counts the current minute and the 59 before, the last day
// the current hour and the 23 before: a bucket reached again a period
// later starts from zero, and a request counted after its bucket moved on
// is left out.
func TestTrafficRollsItsWindows(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	var tr traffic
	tr.count(start, OriginApps, false)
	tr.count(start.Add(30*time.Second), OriginPreload, true)
	tr.count(start.Add(30*time.Minute), OriginPreload, false)

	at := func(after time.Duration) Traffic { return tr.totals(start.Add(after)) }
	if got := at(59 * time.Minute); got.Hour.Requests[OriginApps] != 1 || got.Hour.Requests[OriginPreload] != 2 ||
		got.Hour.Failures[OriginPreload] != 1 || got.Day.Requests[OriginPreload] != 2 {
		t.Errorf("within the hour: %+v", got)
	}
	// An hour later the first minute left the hour, not the day.
	if got := at(60 * time.Minute); got.Hour.Requests[OriginApps] != 0 || got.Hour.Requests[OriginPreload] != 1 ||
		got.Hour.Failures[OriginPreload] != 0 || got.Day.Requests[OriginApps] != 1 || got.Day.Requests[OriginPreload] != 2 {
		t.Errorf("an hour later: %+v", got)
	}

	// The first minute's bucket is reached again: it counts anew.
	tr.count(start.Add(60*time.Minute), OriginRatings, false)
	if got := at(60 * time.Minute); got.Hour.Requests[OriginRatings] != 1 || got.Hour.Requests[OriginApps] != 0 ||
		got.Day.Requests[OriginRatings] != 1 || got.Day.Requests[OriginApps] != 1 {
		t.Errorf("the bucket reused: %+v", got)
	}
	// A request of the minute before it, counted late, is left out.
	tr.count(start, OriginImports, false)
	if got := at(60 * time.Minute); got.Hour.Requests[OriginImports] != 0 {
		t.Errorf("a late request: %+v", got)
	}

	// A day later the first hour left the day, and its bucket counts anew.
	if got := at(24 * time.Hour); got.Day.Requests[OriginApps] != 0 || got.Day.Requests[OriginPreload] != 0 ||
		got.Day.Requests[OriginRatings] != 1 || got.Hour != (Counts{}) {
		t.Errorf("a day later: %+v", got)
	}
	tr.count(start.Add(24*time.Hour), OriginCatalogs, false)
	if got := at(24 * time.Hour); got.Day.Requests[OriginCatalogs] != 1 || got.Day.Requests[OriginRatings] != 1 ||
		got.Day.Requests[OriginImports] != 0 {
		t.Errorf("the hour's bucket reused: %+v", got)
	}
}

// The notices an answer carries are recorded, those of an answer with
// streams to play too: each distinct text once per answer, cut short and
// redacted, rate-limit ones marked, with the answers to background work.
func TestStreamsRecordTheNoticesOfTheirAnswers(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"streams":[
			{"name":"[❌] Sub","description":"Failed to fetch manifest for Sub: Too Many Requests (retry after 1m)"},
			{"name":"Movie 1080p","url":"https://cdn.example/movie.mkv"},
			{"name":"[❌] Sub","description":"Failed to fetch manifest for Sub: Too Many Requests (retry after 1m)"},
			{"name":"ℹ️ Other","description":"see https://other.example/configure?apikey=abc123 for details"},
			{"name":"A page","externalUrl":"https://page.example/title"}
		]}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/manifest.json"
	client := NewClient("test")

	if streams, err := client.Streams(WithOrigin(t.Context(), OriginPreload), manifest, "movie", "tt1", false); err != nil || len(streams) != 5 {
		t.Fatalf("streams: %v %d", err, len(streams))
	}
	if _, err := client.Streams(t.Context(), manifest, "movie", "tt1", false); err != nil {
		t.Fatal(err)
	}
	health, _ := client.Health(manifest)
	if len(health.Notices) != 2 {
		t.Fatalf("notices: %+v", health.Notices)
	}
	byText := map[string]Notice{}
	for _, n := range health.Notices {
		byText[n.Text] = n
	}
	limited := byText["[❌] Sub: Failed to fetch manifest for Sub: Too Many Requests (retry after 1m)"]
	if !limited.RateLimited || limited.Answers != 2 || limited.Background != 1 || limited.First.IsZero() || limited.Last.Before(limited.First) {
		t.Errorf("the rate limit: %+v", limited)
	}
	other := byText["ℹ️ Other: see <link> for details"]
	if other.RateLimited || other.Answers != 2 {
		t.Errorf("the other notice: %+v, all %+v", other, health.Notices)
	}
}

// A description the addon gives of another title, refusing the request,
// is a notice too.
func TestADescriptionOfANoticeIsRecorded(t *testing.T) {
	t.Parallel()
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"meta":{"id":"error","type":"other","name":"Rate limit reached","description":"Try again in a minute"}}`))
	}))
	defer addon.Close()
	manifest := addon.URL + "/manifest.json"
	client := NewClient("test")
	if _, err := client.Meta(t.Context(), manifest, "movie", "tt1", false); err == nil {
		t.Fatal("a notice described as a title was taken")
	}
	health, _ := client.Health(manifest)
	if len(health.Notices) != 1 || health.Notices[0].Text != "Rate limit reached: Try again in a minute" || !health.Notices[0].RateLimited {
		t.Errorf("notices: %+v", health.Notices)
	}
}

// At most noticesKept distinct notices are kept, the one seen least
// lately forgotten first, and none past noticeLife; their texts are cut
// to noticeBytes.
func TestNoticesAreBounded(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	named := func(prefix string, i int) string { return fmt.Sprintf("%s %c", prefix, 'a'+i) }
	var notices []Notice
	for i := range noticesKept {
		notices = note(notices, []string{named("notice", i)}, OriginApps, start.Add(time.Duration(i)*time.Minute))
	}
	// The first notice is seen again, by background work, before 4 new
	// ones come: the 4 seen least lately since are forgotten.
	notices = note(notices, []string{"notice a"}, OriginOther, start.Add(30*time.Minute))
	for i := range 4 {
		notices = note(notices, []string{named("new", i)}, OriginApps, start.Add(time.Duration(31+i)*time.Minute))
	}
	if len(notices) != noticesKept {
		t.Fatalf("%d notices kept, want %d", len(notices), noticesKept)
	}
	kept := recent(notices, start.Add(time.Hour))
	if kept[0].Text != "new d" || kept[4].Text != "notice a" || kept[4].Answers != 2 || kept[4].Background != 1 ||
		!kept[4].First.Equal(start) {
		t.Errorf("kept: %+v", kept)
	}
	for _, n := range kept {
		if slices.Contains([]string{"notice b", "notice c", "notice d", "notice e"}, n.Text) {
			t.Errorf("%q kept, seen least lately", n.Text)
		}
	}

	// A day after their last answer, notices are no longer told, and a
	// new one forgets them.
	later := start.Add(time.Hour + noticeLife)
	if left := recent(notices, later); len(left) != 0 {
		t.Errorf("a day later: %+v", left)
	}
	if notices = note(notices, []string{"fresh"}, OriginApps, later); len(notices) != 1 || notices[0].Text != "fresh" {
		t.Errorf("after a new notice: %+v", notices)
	}

	long := keptNotice(strings.Repeat("é", noticeBytes))
	if len(long) > noticeBytes || !strings.HasSuffix(long, "…") || !strings.HasPrefix(long, "éé") {
		t.Errorf("a long text: %d bytes %q", len(long), long)
	}
	streams := make([]Stream, 0, 2*noticesKept)
	for i := range 2 * noticesKept {
		streams = append(streams, Stream{Name: named("Sub", i), Description: "failed"})
	}
	if texts := noticesOf(streams); len(texts) != noticesKept {
		t.Errorf("an answer of %d notices: %d kept", len(streams), len(texts))
	}
}

// Notices alike but for their digits, such as the summaries of an addon
// that gathers other addons whose timings change on every answer, are one
// notice: it tells the latest text, counts every answer that carried one,
// and leaves the other places to other notices. A refusal for the rate
// stays a notice of its own, even beside a summary that counts 429 of
// something.
func TestNoticesAlikeButForTheirDigitsAreOne(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	var notices []Notice
	for i, text := range []string{
		"Scrape summary: 10 streams, 10 removed, 1.2s",
		"Rate limit exceeded",
		"Scrape summary: 12 streams, 12 removed, 0.8s",
		"Scrape summary: 429 streams, 429 removed, 3s",
	} {
		notices = note(notices, []string{text}, OriginPreload, start.Add(time.Duration(i)*time.Minute))
	}
	kept := recent(notices, start.Add(time.Hour))
	if len(kept) != 3 {
		t.Fatalf("%d notices, want 3: %+v", len(kept), kept)
	}
	if n := kept[0]; n.Text != "Scrape summary: 429 streams, 429 removed, 3s" || !n.RateLimited || n.Answers != 1 {
		t.Errorf("the summary that counts 429: %+v", n)
	}
	if n := kept[1]; n.Text != "Scrape summary: 12 streams, 12 removed, 0.8s" || n.RateLimited || n.Answers != 2 || n.Background != 2 ||
		!n.First.Equal(start) || !n.Last.Equal(start.Add(2*time.Minute)) {
		t.Errorf("the summaries: %+v", n)
	}
	if n := kept[2]; n.Text != "Rate limit exceeded" || !n.RateLimited || n.Answers != 1 {
		t.Errorf("the refusal: %+v", n)
	}
	answer := []Stream{{Name: "Sub 1", Description: "failed"}, {Name: "Sub 2", Description: "failed"}, {Name: "Other", Description: "failed"}}
	if texts := noticesOf(answer); !slices.Equal(texts, []string{"Sub 1: failed", "Other: failed"}) {
		t.Errorf("an answer's notices: %q", texts)
	}
}

// What a notice may carry of an address or a key is hidden before it is
// kept, and what says a rate limit is told apart.
func TestNoticeTextsAreRedacted(t *testing.T) {
	t.Parallel()
	for text, want := range map[string]string{
		"Failed: https://sub.example/AbCdEf0123456789xyz/manifest.json answered 429": "Failed: <link> answered 429",
		"Refused, apikey=Zx81Qm, retry later":                                        "Refused, apikey=<redacted>, retry later",
		"Token 9f86d081884c7d659a2feaa0c55ad015 refused":                             "Token <redacted> refused",
		"Too Many Requests (retry after 1m)":                                         "Too Many Requests (retry after 1m)",
	} {
		if got := keptNotice(text); got != want {
			t.Errorf("%q kept as %q, want %q", text, got, want)
		}
	}
	for text, limited := range map[string]bool{
		"Too Many Requests":            true,
		"HTTP 429 from the sub-addon":  true,
		"Retry-After: 60":              true,
		"rate limited, slow down":      true,
		"No streams found for 4290 Mb": false,
		"23 streams hidden by filters": false,
	} {
		if NoticeRateLimited(text) != limited {
			t.Errorf("%q: rate limited %v, want %v", text, !limited, limited)
		}
	}
}
