package iptv

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
)

// limitedXtream is an Xtream server that answers at most limit requests
// a window, else 429 with Retry-After, and tells the account's
// connections; at records when each request came.
type limitedXtream struct {
	url    string
	limit  int
	window time.Duration

	mu    sync.Mutex
	at    []time.Time
	codes []int
}

func newLimitedXtream(t *testing.T, limit int, window time.Duration) *limitedXtream {
	t.Helper()
	x := &limitedXtream{limit: limit, window: window}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		x.mu.Lock()
		now := time.Now()
		recent := 0
		for i, at := range x.at {
			if now.Sub(at) < x.window && x.codes[i] == http.StatusOK {
				recent++
			}
		}
		code := http.StatusOK
		if x.limit > 0 && recent >= x.limit {
			code = http.StatusTooManyRequests
		}
		x.at, x.codes = append(x.at, now), append(x.codes, code)
		x.mu.Unlock()
		if code != http.StatusOK {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(code)
			return
		}
		switch r.URL.Query().Get("action") {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_info": map[string]any{"auth": 1, "max_connections": "1", "active_cons": "0",
				"allowed_output_formats": []string{"ts"}}})
		case "get_live_categories", "get_vod_categories", "get_series_categories":
			_, _ = w.Write([]byte(`[{"category_id": "1", "category_name": "News"}]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[{"num": 1, "name": "Zeb One", "stream_id": 101, "category_id": "1"}]`))
		case "get_vod_streams":
			_, _ = w.Write([]byte(`[{"name": "Mirfen", "stream_id": 7, "category_id": "1", "container_extension": "mkv"}]`))
		case "get_series":
			_, _ = w.Write([]byte(`[{"name": "Quill", "series_id": 8, "category_id": "1"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	x.url = server.URL
	return x
}

func countCodes(codes []int, code int) int {
	n := 0
	for _, c := range codes {
		if c == code {
			n++
		}
	}
	return n
}

// Requests to one host are spaced by the pacer's gap, and a host that
// asked to wait is not asked before.
func TestPacerSpacesRequestsToAHost(t *testing.T) {
	p := NewPacer(100 * time.Millisecond)
	start := time.Now()
	for range 3 {
		if err := p.Wait(t.Context(), "http://provider.example:8080/player_api.php"); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took < 190*time.Millisecond {
		t.Errorf("three requests in %s", took)
	}
	// Another host has its own turns.
	start = time.Now()
	if err := p.Wait(t.Context(), "http://other.example/xmltv.php"); err != nil || time.Since(start) > 50*time.Millisecond {
		t.Errorf("another host waited %s", time.Since(start))
	}
	p.Delay("http://other.example/a", 300*time.Millisecond)
	start = time.Now()
	if err := p.Wait(t.Context(), "http://other.example/b"); err != nil || time.Since(start) < 250*time.Millisecond {
		t.Errorf("a host that asked to wait was asked after %s", time.Since(start))
	}
}

// Adding an account with its movies and series sends its requests one at
// a time: a provider allowing 3 requests every 2 seconds refuses none, and
// the add succeeds; the account's connections are kept.
func TestAddingAnAccountStaysWithinItsRateLimit(t *testing.T) {
	e := newEnv(t)
	e.service.pacer = NewPacer(700 * time.Millisecond)
	x := newLimitedXtream(t, 3, 2*time.Second)
	on := true
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Box",
		Account: Account{Kind: addons.KindXtream, Server: x.url, Username: "user", Password: "secret"},
		Options: &OptionsPatch{Movies: &on, Series: &on}}, false)
	if err != nil {
		t.Fatalf("adding the account: %v", err)
	}
	x.mu.Lock()
	requests, refused := len(x.at), countCodes(x.codes, http.StatusTooManyRequests)
	x.mu.Unlock()
	if requests != 7 || refused != 0 {
		t.Errorf("%d requests, %d refused", requests, refused)
	}
	source, err := e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if err != nil || source.MaxConnections == nil || *source.MaxConnections != 1 {
		t.Errorf("connections: %v %v", source.MaxConnections, err)
	}
	if limit, err := e.service.Connections(t.Context(), addon.ID); err != nil || limit != 1 {
		t.Errorf("Connections: %d %v", limit, err)
	}
}

// A provider that asks to slow down is asked again once it said, and the
// request succeeds; one that keeps refusing fails as rate limited.
func TestRateLimitedRequestsAreTriedAgain(t *testing.T) {
	client := requester{client: newEnv(t).service.client, pacer: NewPacer(0)}
	x := newLimitedXtream(t, 1, 1500*time.Millisecond)
	account := Account{Kind: addons.KindXtream, Server: x.url, Username: "user", Password: "secret"}
	address, _ := account.address()
	if _, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), false, livePart); err != nil {
		t.Fatalf("a provider asking to wait a second: %v", err)
	}
	x.mu.Lock()
	refused := countCodes(x.codes, http.StatusTooManyRequests)
	x.mu.Unlock()
	if refused == 0 {
		t.Fatalf("the provider never refused: the test proves nothing")
	}
	x.mu.Lock()
	x.limit, x.window = 1, time.Hour
	x.mu.Unlock()
	_, err := fetch(t.Context(), client, accountOf(addons.KindXtream, address), false, livePart)
	var limited *RateLimitError
	if !errors.Is(err, ErrRateLimited) || !errors.As(err, &limited) || limited.RetryAfter != time.Second {
		t.Errorf("a provider refusing for good: %v", err)
	}
}

// A failed download is tried again after 5 minutes, 15, then every hour,
// never later than the refresh interval; a success clears it.
func TestFailedListsAreTriedAgainSooner(t *testing.T) {
	for failures, want := range map[int]time.Duration{1: 5 * time.Minute, 2: 15 * time.Minute, 3: time.Hour, 9: time.Hour} {
		if got := Backoff(failures, 12*time.Hour, 0); got != want {
			t.Errorf("after %d failures: %s, want %s", failures, got, want)
		}
	}
	if got := Backoff(3, 30*time.Minute, 0); got != 30*time.Minute {
		t.Errorf("capped by the interval: %s", got)
	}
	if got := Backoff(1, 12*time.Hour, 10*time.Minute); got != 10*time.Minute {
		t.Errorf("after a Retry-After of 10 minutes: %s", got)
	}

	e := newEnv(t)
	list := newPlaylistServer(t, "#EXTM3U\n#EXTINF:-1,Zeb One\nhttps://live.example/1.ts\n")
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false))
	list.set("", http.StatusTooManyRequests)
	if err := e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	source := must(e.service.Source(t.Context(), addons.Shared(), addon.ID))
	if source.Error != errorRateLimited || source.NextAt == nil || !source.NextAt.Equal(e.now.Load().Add(5*time.Minute)) {
		t.Fatalf("after a 429: %q, next %v", source.Error, source.NextAt)
	}
	downloads := list.downloads.Load()
	e.later(4 * time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil || list.downloads.Load() != downloads {
		t.Errorf("tried again within its backoff: %v", err)
	}
	e.later(time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil || list.downloads.Load() == downloads {
		t.Errorf("not tried again after its backoff: %v", err)
	}
	if source = must(e.service.Source(t.Context(), addons.Shared(), addon.ID)); !source.NextAt.Equal(e.now.Load().Add(15 * time.Minute)) {
		t.Errorf("after a second failure: next %v", source.NextAt)
	}
	list.set("#EXTM3U\n#EXTINF:-1,Zeb One\nhttps://live.example/1.ts\n", 0)
	e.later(15 * time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	interval := time.Duration(e.users.Settings().LiveTvRefreshHours) * time.Hour
	if source = must(e.service.Source(t.Context(), addons.Shared(), addon.ID)); source.Error != "" || !source.NextAt.Equal(e.now.Load().Add(interval)) {
		t.Errorf("after a success: %q, next %v", source.Error, source.NextAt)
	}
}

// How a stream answered orders its channel's streams: those that failed
// come last, dead and silent ones are left out for their backoff, refused
// ones are not; a new address forgets it.
func TestStreamHealthOrdersAndHidesStreams(t *testing.T) {
	e := newEnv(t)
	merged := ChannelsMerged
	list := newPlaylistServer(t, "#EXTM3U\n"+entry("News", "Zeb One FHD", "fhd.ts")+entry("News", "Zeb One HD", "hd.ts")+entry("News", "Zeb One SD", "sd.ts"))
	addon := must(e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url},
		Options: &OptionsPatch{Channels: &merged}}, false))
	channel := must(e.service.Channels(t.Context(), addon.ID))[0].ID
	addresses := func() []string {
		var result []string
		for _, s := range must(e.service.Streams(t.Context(), addon.ID, channel)) {
			result = append(result, s.URL[strings.LastIndexByte(s.URL, '/')+1:])
		}
		return result
	}
	base := "https://live.example/"
	if err := e.service.ReportStream(t.Context(), addon.ID, channel, base+"fhd.ts", FailureDead); err != nil {
		t.Fatal(err)
	}
	if err := e.service.ReportStream(t.Context(), addon.ID, channel, base+"hd.ts", FailureRefused); err != nil {
		t.Fatal(err)
	}
	if got := addresses(); !slices.Equal(got, []string{"sd.ts", "hd.ts"}) {
		t.Errorf("after a dead and a refused stream: %q", got)
	}
	e.later(61 * time.Minute)
	if got := addresses(); !slices.Equal(got, []string{"sd.ts", "fhd.ts", "hd.ts"}) {
		t.Errorf("after the dead stream's backoff: %q", got)
	}
	// Dead again: six hours this time.
	_ = e.service.ReportStream(t.Context(), addon.ID, channel, base+"fhd.ts", FailureDead)
	e.later(5 * time.Hour)
	if got := addresses(); slices.Contains(got, "fhd.ts") {
		t.Errorf("a stream dead twice came back within 6 hours: %q", got)
	}
	// It played: first again.
	_ = e.service.ReportStream(t.Context(), addon.ID, channel, base+"fhd.ts", "")
	_ = e.service.ReportStream(t.Context(), addon.ID, channel, base+"hd.ts", "")
	if got := addresses(); !slices.Equal(got, []string{"fhd.ts", "hd.ts", "sd.ts"}) {
		t.Errorf("after they played: %q", got)
	}
	channels := e.lineup(addon.ID)
	if h := channels[0].Streams[0].Health; h.OKAt == nil || h.Failure != "" {
		t.Errorf("the line-up's health: %+v", h)
	}
	// A list that gives a stream another address forgets what was known.
	_ = e.service.ReportStream(t.Context(), addon.ID, channel, base+"sd.ts", FailureTimeout)
	list.set("#EXTM3U\n"+entry("News", "Zeb One FHD", "fhd.ts")+entry("News", "Zeb One HD", "hd.ts")+entry("News", "Zeb One SD", "sd2.ts"), 0)
	if err := e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if got := addresses(); !slices.Contains(got, "sd2.ts") {
		t.Errorf("a stream at a new address is still left out: %q", got)
	}
	// The administrator's "try again" clears it all.
	_ = e.service.ReportStream(t.Context(), addon.ID, channel, base+"fhd.ts", FailureDead)
	if err := e.service.ForgetStreamHealth(t.Context(), addons.Shared(), addon.ID, channels[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := addresses(); !slices.Equal(got, []string{"fhd.ts", "hd.ts", "sd2.ts"}) {
		t.Errorf("after trying again: %q", got)
	}
}
