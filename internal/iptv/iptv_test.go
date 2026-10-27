package iptv

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

type env struct {
	t       *testing.T
	service *Service
	addons  *addons.Store
	users   *accounts.Store
	now     *atomic.Pointer[time.Time]
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := migrated.New(t)
	users, err := accounts.Open(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	client := stremio.NewClient("test")
	store := addons.New(pool, client)
	service := New(pool, store, client, slog.New(slog.NewTextHandler(io.Discard, nil)), users.Settings)
	// Requests are paced by the tests that check pacing only.
	service.pacer = NewPacer(0)
	now := new(atomic.Pointer[time.Time])
	start := time.Now().Truncate(time.Second)
	now.Store(&start)
	service.now = func() time.Time { return *now.Load() }
	return env{t: t, service: service, addons: store, users: users, now: now}
}

func (e env) later(d time.Duration) {
	next := e.now.Load().Add(d)
	e.now.Store(&next)
}

// playlistServer serves an M3U playlist that can change, counting
// downloads; status replaces it while set.
type playlistServer struct {
	mu        sync.Mutex
	body      string
	status    int
	downloads atomic.Int32
	url       string
}

func newPlaylistServer(t *testing.T, body string) *playlistServer {
	t.Helper()
	p := &playlistServer{body: body}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		p.downloads.Add(1)
		p.mu.Lock()
		body, status := p.body, p.status
		p.mu.Unlock()
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	p.url = server.URL + "/get.php?username=user&password=secret&type=m3u_plus"
	return p
}

func (p *playlistServer) set(body string, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.body, p.status = body, status
}

const groupsPlaylist = "#EXTM3U\n" +
	"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-chno=\"5\" group-title=\"News\",Zeb One\nhttps://live.example/1.ts\n" +
	"#EXTINF:-1 group-title=\"Kids\",Orbe Junior\nhttps://live.example/2.ts\n" +
	"#EXTINF:-1 group-title=\"Kids\",Quill Toons\n#EXTVLCOPT:http-user-agent=Player/1.0\nhttps://live.example/3.ts\n" +
	"#EXTINF:-1,Lone Channel\nhttps://live.example/4.ts\n"

func names(metas []stremio.Meta) []string {
	result := make([]string, 0, len(metas))
	for _, meta := range metas {
		result = append(result, meta.Name)
	}
	return result
}

// A source answers as an addon with one live TV catalog: its channels,
// with their numbers, groups and guide identifiers, then each channel's
// stream with the headers its list gives; the groups shown are chosen.
func TestSourcesListTheirChannels(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	list := newPlaylistServer(t, groupsPlaylist)
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: " My TV ", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if addon.Kind != addons.KindM3U || addon.Manifest.Name != "My TV" || len(addon.Manifest.Catalogs) != 1 || addon.Manifest.Catalogs[0].Type != "tv" {
		t.Fatalf("addon: %+v", addon)
	}
	libraries, err := e.addons.Libraries(t.Context(), addons.Shared())
	if err != nil || len(libraries) != 1 || !libraries[0].Enabled || libraries[0].Guides == nil {
		t.Fatalf("its catalog is enabled: %+v %v", libraries, err)
	}
	channels, err := e.service.Channels(t.Context(), addon.ID)
	if err != nil || !slices.Equal(names(channels), []string{"Zeb One", "Orbe Junior", "Quill Toons", "Lone Channel"}) {
		t.Fatalf("channels: %q %v", names(channels), err)
	}
	zeb := channels[0]
	if zeb.Type != "tv" || zeb.ChannelNumber != 5 || zeb.GuideID != "zeb.zz" || !slices.Equal(zeb.Genres, []string{"News"}) ||
		!strings.HasPrefix(zeb.ID, IDPrefix+addon.ID.String()+":") || !addon.Manifest.Serves("stream", "tv", zeb.ID) {
		t.Errorf("channel: %+v", zeb)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, channels[2].ID)
	if err != nil || len(streams) != 1 || streams[0].URL != "https://live.example/3.ts" || streams[0].RequestHeaders()["User-Agent"] != "Player/1.0" {
		t.Errorf("streams: %+v %v", streams, err)
	}

	source, err := e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if err != nil || source.Channels != 4 || source.Error != "" || source.FetchedAt == nil || source.Options.Categories != CategoriesOriginal ||
		source.Lineup != (LineupCounts{Categories: 3, EnabledCategories: 3, Channels: 4, EnabledChannels: 4, ShownChannels: 4, Unmapped: 4}) {
		t.Fatalf("source: %+v %v", source, err)
	}
	excluded := []string{"g:News", "g:"}
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &OptionsPatch{Excluded: &excluded}}, false); err != nil {
		t.Fatal(err)
	}
	if channels, _ := e.service.Channels(t.Context(), addon.ID); !slices.Equal(names(channels), []string{"Orbe Junior", "Quill Toons"}) {
		t.Errorf("channels of the groups kept: %q", names(channels))
	}
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Options: &OptionsPatch{Excluded: &[]string{}}, Name: new("Renamed")}, false); err != nil {
		t.Fatal(err)
	}
	if channels, _ := e.service.Channels(t.Context(), addon.ID); len(channels) != 4 {
		t.Errorf("every group again: %q", names(channels))
	}
	if found, _ := e.addons.Find(t.Context(), addon.ID); found.Manifest.Name != "Renamed" || found.Manifest.Catalogs[0].Name != "Renamed" {
		t.Errorf("renamed: %+v", found.Manifest)
	}

	// The same list cannot be added twice to a scope; a broken one is not
	// added at all.
	if _, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Again", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false); !errors.Is(err, addons.ErrExists) {
		t.Errorf("added twice: %v", err)
	}
	broken := newPlaylistServer(t, "<html>Sign in</html>")
	if _, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Broken", Account: Account{Kind: addons.KindM3U, URL: broken.url}}, false); !errors.Is(err, ErrInvalidList) {
		t.Errorf("a page that is not a playlist: %v", err)
	}
	if _, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "", Account: Account{Kind: addons.KindM3U, URL: list.url + "&x"}}, false); !errors.Is(err, ErrInvalidName) {
		t.Errorf("no name: %v", err)
	}
	if installed, _ := e.addons.Addons(t.Context(), addons.Shared()); len(installed) != 1 {
		t.Errorf("addons: %d", len(installed))
	}
	// A user's own source stays on public addresses unless they are an
	// administrator.
	member, _ := e.users.CreateUser(t.Context(), accounts.NewUser{Name: "member", Password: "correct horse"})
	if _, err := e.service.Add(t.Context(), addons.Personal(member.ID), NewSource{Name: "Mine", Account: Account{Kind: addons.KindM3U, URL: list.url}}, true); !errors.Is(err, stremio.ErrPrivateNetwork) {
		t.Errorf("a member's local source: %v", err)
	}
}

// A source's streams are requested with the user agent its list gives
// them, else the one its administrator set, else a browser's, as Jellyfin
// requests its M3U tuners' streams. Only the first two are headers the
// stream needs. The set one also goes with the list's downloads: setting
// it downloads the list again, and is refused when the list cannot be
// read with it.
func TestSourcesRequestTheirStreamsWithAUserAgent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	var mu sync.Mutex
	var listAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		listAgent = r.Header.Get("User-Agent")
		mu.Unlock()
		if r.Header.Get("User-Agent") == "Refused/1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:-1,Plain\nhttps://live.example/plain.ts\n"+
			"#EXTINF:-1,Own\n#EXTVLCOPT:http-user-agent=Player/1.0\nhttps://live.example/own.ts\n")
	}))
	t.Cleanup(server.Close)
	downloadedWith := func() string {
		mu.Lock()
		defer mu.Unlock()
		return listAgent
	}
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "Agents", Account: Account{Kind: addons.KindM3U, URL: server.URL + "/get.php"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := downloadedWith(); got != "Polyfin/test" {
		t.Errorf("the list downloaded with %q", got)
	}
	channels, err := e.service.Channels(t.Context(), addon.ID)
	if err != nil || len(channels) != 2 {
		t.Fatalf("channels: %q %v", names(channels), err)
	}
	stream := func(channel int) stremio.Stream {
		t.Helper()
		streams, err := e.service.Streams(t.Context(), addon.ID, channels[channel].ID)
		if err != nil || len(streams) != 1 {
			t.Fatalf("streams of %s: %+v %v", channels[channel].Name, streams, err)
		}
		return streams[0]
	}
	if plain := stream(0); plain.UserAgent != BrowserUserAgent || plain.RequestHeaders() != nil {
		t.Errorf("a stream its list names no user agent for: %q %v", plain.UserAgent, plain.RequestHeaders())
	}
	if own := stream(1); own.UserAgent != "" || own.RequestHeaders()["User-Agent"] != "Player/1.0" {
		t.Errorf("a stream with its list's user agent: %q %v", own.UserAgent, own.RequestHeaders())
	}

	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{UserAgent: new(" Panel/1 ")}, false); err != nil {
		t.Fatal(err)
	}
	if source, err := e.service.Source(t.Context(), addons.Shared(), addon.ID); err != nil || source.UserAgent != "Panel/1" || downloadedWith() != "Panel/1" {
		t.Errorf("the source's user agent: %q, the list downloaded with %q, %v", source.UserAgent, downloadedWith(), err)
	}
	if plain := stream(0); plain.UserAgent != "" || plain.RequestHeaders()["User-Agent"] != "Panel/1" {
		t.Errorf("a stream with its source's user agent: %q %v", plain.UserAgent, plain.RequestHeaders())
	}
	if own := stream(1); own.RequestHeaders()["User-Agent"] != "Player/1.0" {
		t.Errorf("the list's own user agent comes first: %v", own.RequestHeaders())
	}
	if err := e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil || downloadedWith() != "Panel/1" {
		t.Errorf("refreshed with %q: %v", downloadedWith(), err)
	}

	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{UserAgent: new("Refused/1")}, false); !errors.Is(err, stremio.ErrUnreachable) {
		t.Errorf("a user agent the list is refused with: %v", err)
	}
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{UserAgent: new("Tab\tbed")}, false); !errors.Is(err, ErrInvalidUserAgent) {
		t.Errorf("a user agent with a control character: %v", err)
	}
	if source, _ := e.service.Source(t.Context(), addons.Shared(), addon.ID); source.UserAgent != "Panel/1" {
		t.Errorf("a refused user agent replaced the source's: %q", source.UserAgent)
	}
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{UserAgent: new("")}, false); err != nil {
		t.Fatal(err)
	}
	if plain := stream(0); plain.UserAgent != BrowserUserAgent || downloadedWith() != "Polyfin/test" {
		t.Errorf("back to the default: %q, the list downloaded with %q", plain.UserAgent, downloadedWith())
	}
}

// Lists are fetched again once due under the settings' refresh hours, and
// by hand; a failed fetch keeps the channels and tells why.
func TestListsAreRefreshedOnSchedule(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	list := newPlaylistServer(t, "#EXTM3U\n#EXTINF:-1,Zeb One\nhttps://live.example/1.ts\n")
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: Account{Kind: addons.KindM3U, URL: list.url}}, false)
	if err != nil {
		t.Fatal(err)
	}
	settings := e.users.Settings()
	settings.LiveTvRefreshHours = 3
	if _, err := e.users.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	list.set("#EXTM3U\n#EXTINF:-1,Zeb Two\nhttps://live.example/2.ts\n", 0)
	e.later(2*time.Hour + 59*time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if list.downloads.Load() != 1 {
		t.Errorf("a list fetched under 3 hours ago was fetched again")
	}
	e.later(time.Minute)
	if err := e.service.RefreshDue(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if channels, _ := e.service.Channels(t.Context(), addon.ID); list.downloads.Load() != 2 || !slices.Equal(names(channels), []string{"Zeb Two"}) {
		t.Errorf("after 3 hours: %q after %d downloads", names(channels), list.downloads.Load())
	}
	source, _ := e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if source.NextAt == nil || !source.NextAt.Equal(e.now.Load().Add(3*time.Hour)) {
		t.Errorf("next refresh: %v", source.NextAt)
	}
	// By hand, every list is fetched, due or not.
	list.set("", http.StatusForbidden)
	if err := e.service.RefreshDue(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	source, _ = e.service.Source(t.Context(), addons.Shared(), addon.ID)
	if channels, _ := e.service.Channels(t.Context(), addon.ID); source.Error != errorUnreachable || source.Channels != 1 || len(channels) != 1 ||
		list.downloads.Load() != 3 {
		t.Errorf("a failed fetch: %+v %q", source, names(channels))
	}
	list.set("#EXTM3U\n", 0)
	if err := e.service.Refresh(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if source, _ = e.service.Source(t.Context(), addons.Shared(), addon.ID); source.Error != "" || source.Channels != 0 {
		t.Errorf("an empty list: %+v", source)
	}
	// A turned-off source is not fetched on schedule.
	if _, err := e.addons.SetEnabled(t.Context(), addons.Shared(), addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := e.service.RefreshDue(t.Context(), true); err != nil || list.downloads.Load() != 4 {
		t.Errorf("a turned-off source: %v, %d downloads", err, list.downloads.Load())
	}
}

// A new password replaces the stored one, keeping the channels'
// identities; one left out keeps the current one.
func TestXtreamAccountsChange(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	x := newXtreamServer(t, "ts")
	if _, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: x.account("wrong")}, false); !errors.Is(err, ErrLoginRefused) {
		t.Fatalf("a refused account: %v", err)
	}
	addon, err := e.service.Add(t.Context(), addons.Shared(), NewSource{Name: "TV", Account: x.account("p@ss word")}, false)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := e.service.Channels(t.Context(), addon.ID)
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Account: &Account{Server: x.url, Username: "user", Password: "wrong"}}, false); !errors.Is(err, ErrLoginRefused) {
		t.Errorf("a wrong password: %v", err)
	}
	if err := e.service.Update(t.Context(), addons.Shared(), addon.ID, Changes{Account: &Account{Server: x.url, Username: "user"}}, false); err != nil {
		t.Errorf("a password left out: %v", err)
	}
	after, _ := e.service.Channels(t.Context(), addon.ID)
	if len(before) != 2 || !slices.Equal(names(before), names(after)) || before[0].ID != after[0].ID {
		t.Errorf("channels: %q, then %q", names(before), names(after))
	}
	if found, _ := e.addons.Find(t.Context(), addon.ID); accountOf(found.Kind, found.ManifestURL).Password != "p@ss word" {
		t.Errorf("the password changed")
	}
}
