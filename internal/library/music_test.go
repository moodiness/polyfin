package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/eclipse"
	"github.com/moodiness/polyfin/internal/stremio"
)

// musicService is a service with only what music caching needs, on a
// clock the test moves.
func musicService(t *testing.T) (*Service, *atomic.Pointer[time.Time]) {
	t.Helper()
	var clock atomic.Pointer[time.Time]
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock.Store(&start)
	s := &Service{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), now: func() time.Time { return *clock.Load() },
		settings: func() accounts.Settings { return accounts.Settings{CatalogRefreshMinutes: 60} }}
	s.musicCache = newMusicCaches(s)
	return s, &clock
}

func advance(clock *atomic.Pointer[time.Time], d time.Duration) {
	next := clock.Load().Add(d)
	clock.Store(&next)
}

func TestStaleMusicPagesAreServedWhileTheAddonIsAskedAgain(t *testing.T) {
	s, clock := musicService(t)
	key := musicKey{addon: accounts.ID{1}, resource: "album", id: "a"}
	var asked atomic.Int32
	answers := make(chan string, 1)
	fetch := func(context.Context) (string, error) {
		asked.Add(1)
		return <-answers, nil
	}
	cache := newMusicCache(10, 1<<20, func(string) int { return 1 }, func() time.Duration { return time.Hour }, staleMusic, s.now)
	answers <- "first"
	if got, err := remember(t.Context(), s, cache, key, fetch); got != "first" || err != nil {
		t.Fatalf("first read: %q %v", got, err)
	}
	// Fresh: the addon is not asked.
	advance(clock, 59*time.Minute)
	if got, _ := remember(t.Context(), s, cache, key, fetch); got != "first" || asked.Load() != 1 {
		t.Fatalf("fresh read: %q, %d requests", got, asked.Load())
	}
	// Due again: the page kept is served at once, while the addon, which
	// has not answered yet, is asked again in the background, once for
	// every read meanwhile.
	advance(clock, 2*time.Minute)
	for range 3 {
		if got, _ := remember(t.Context(), s, cache, key, fetch); got != "first" {
			t.Fatalf("stale read: %q", got)
		}
	}
	answers <- "second"
	// The refresh ends with its flight, after it keeps its answer: joining
	// the flight waits for its end.
	s.flight.Do(musicFlight(key), func() (any, error) { return nil, nil })
	if got, _ := remember(t.Context(), s, cache, key, fetch); got != "second" || asked.Load() != 2 {
		t.Fatalf("read after the refresh: %q, %d requests, want 2", got, asked.Load())
	}
	// Once kept staleMusic past its age, a page is forgotten: a read waits
	// for the addon.
	advance(clock, time.Hour+staleMusic+time.Minute)
	answers <- "third"
	if got, _ := remember(t.Context(), s, cache, key, fetch); got != "third" {
		t.Errorf("read after the stale time: %q", got)
	}
}

func TestAStalePageStaysWhenTheAddonFails(t *testing.T) {
	s, clock := musicService(t)
	key := musicKey{addon: accounts.ID{1}, resource: "album", id: "a"}
	cache := newMusicCache(10, 1<<20, func(string) int { return 1 }, func() time.Duration { return time.Hour }, staleMusic, s.now)
	if _, err := remember(t.Context(), s, cache, key, func(context.Context) (string, error) { return "kept", nil }); err != nil {
		t.Fatal(err)
	}
	advance(clock, 2*time.Hour)
	failed := make(chan struct{})
	if got, _ := remember(t.Context(), s, cache, key, func(context.Context) (string, error) {
		defer close(failed)
		return "", errors.New("addon down")
	}); got != "kept" {
		t.Fatalf("stale read: %q", got)
	}
	<-failed
	if got, err := remember(t.Context(), s, cache, key, func(context.Context) (string, error) { return "", errors.New("addon down") }); got != "kept" || err != nil {
		t.Errorf("after a failed refresh: %q %v", got, err)
	}
}

func TestRequestsToOneAddonAreBounded(t *testing.T) {
	s, _ := musicService(t)
	cache := newMusicCache(100, 1<<20, func(int) int { return 1 }, func() time.Duration { return time.Hour }, staleMusic, s.now)
	busy, other := accounts.ID{1}, accounts.ID{2}
	var mu sync.Mutex
	running, most := map[accounts.ID]int{}, map[accounts.ID]int{}
	// Requests to an addon are held until addonFetches of them run at
	// once, whatever the machine's speed, or until the test gives up; then
	// a little longer, so that a request past the bound would show.
	full := map[accounts.ID]chan struct{}{busy: make(chan struct{}), other: make(chan struct{})}
	held, giveUp := context.WithTimeout(t.Context(), 10*time.Second)
	defer giveUp()
	fetch := func(addon accounts.ID) func(context.Context) (int, error) {
		return func(context.Context) (int, error) {
			mu.Lock()
			running[addon]++
			most[addon] = max(most[addon], running[addon])
			if running[addon] == addonFetches {
				select {
				case <-full[addon]:
				default:
					close(full[addon])
				}
			}
			mu.Unlock()
			select {
			case <-full[addon]:
			case <-held.Done():
			}
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running[addon]--
			mu.Unlock()
			return 1, nil
		}
	}
	// Two listings of 3 × addonFetches pages each from one addon, and one
	// from another, at once.
	var wg sync.WaitGroup
	for i := range 3 * addonFetches {
		for _, key := range []musicKey{{addon: busy, resource: "album", id: string(rune('a' + i))},
			{addon: busy, resource: "playlist", id: string(rune('a' + i))}, {addon: other, resource: "album", id: string(rune('a' + i))}} {
			wg.Go(func() {
				if _, err := remember(t.Context(), s, cache, key, fetch(key.addon)); err != nil {
					t.Error(err)
				}
			})
		}
	}
	wg.Wait()
	if most[busy] != addonFetches || most[other] != addonFetches {
		t.Errorf("most requests at once: %d to one addon, %d to the other; want %d", most[busy], most[other], addonFetches)
	}
}

func TestKnownFormatsDescribeTracks(t *testing.T) {
	for _, c := range []struct {
		given AudioSource
		codec string
		box   string
	}{
		{AudioSource{Format: "flac"}, "flac", "flac"},
		{AudioSource{Format: "mp3"}, "mp3", "mp3"},
		{AudioSource{Format: "m4a"}, "aac", "m4a"},
		{AudioSource{Format: "opus"}, "opus", "ogg"},
		// What the addon says is kept, and completed only where the format
		// agrees with it.
		{AudioSource{Format: "flac", Codec: "flac"}, "flac", "flac"},
		{AudioSource{Format: "m4a", Codec: "alac"}, "alac", ""},
		{AudioSource{Format: "mp3", Container: "mp4"}, "", "mp4"},
		// A manifest is no file of that format; an unknown format says
		// nothing.
		{AudioSource{Format: "aac", Manifest: "hls"}, "", ""},
		{AudioSource{Format: "lossless"}, "", ""},
	} {
		got := c.given
		got.completeFromFormat()
		if got.Codec != c.codec || got.Container != c.box {
			t.Errorf("%+v: codec %q, container %q", c.given, got.Codec, got.Container)
		}
	}
}

func TestALinkJustGivenIsUsedUntilItExpires(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	short := Version{Expires: now.Add(30 * time.Second), Audio: &AudioSource{Resolved: now}}
	if !short.fresh(now.Add(10 * time.Second)) {
		t.Error("a link given 10 s ago was renewed")
	}
	if short.fresh(now.Add(20 * time.Second)) {
		t.Error("a link given 20 s ago and expiring in 10 s was used")
	}
	expired := Version{Expires: now.Add(-time.Second), Audio: &AudioSource{Resolved: now}}
	if expired.fresh(now) {
		t.Error("a link given expired was used")
	}
	if !(Version{Expires: now.Add(2 * time.Minute)}).fresh(now) || (Version{Expires: now.Add(30 * time.Second)}).fresh(now) {
		t.Error("the margin before expiry changed")
	}
}

func TestMusicAnswersAreBoundedInBytes(t *testing.T) {
	s, _ := musicService(t)
	playlists := newMusicCaches(s).playlists
	// Playlists of about 1 MB each: past 16 MB, the oldest go.
	long := eclipse.Playlist{ID: "p", Title: string(make([]byte, 1<<20))}
	for i := range 40 {
		playlists.answers.Put(musicKey{addon: accounts.ID{1}, resource: "playlist", id: string(rune('a' + i))}, answered[eclipse.Playlist]{value: long})
	}
	if bytes := playlists.answers.Bytes(); bytes > 16<<20 || bytes < 8<<20 {
		t.Errorf("playlists kept: %d bytes", bytes)
	}
	if _, ok := playlists.answers.Get(musicKey{addon: accounts.ID{1}, resource: "playlist", id: "a"}); ok {
		t.Error("the first playlist is still kept")
	}
	if _, ok := playlists.answers.Get(musicKey{addon: accounts.ID{1}, resource: "playlist", id: string(rune('a' + 39))}); !ok {
		t.Error("the last playlist is not kept")
	}
}

func TestPlaysOfOneTrackShareItsResolution(t *testing.T) {
	s, _ := musicService(t)
	s.music = eclipse.NewClient(stremio.NewClient("test"))
	s.versions = cache.New[accounts.ID, Version](10, time.Hour)
	// The addon holds its answer until the four plays wait on the same
	// resolution.
	var asked atomic.Int32
	hold := make(chan struct{})
	addon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		<-hold
		_ = json.NewEncoder(w).Encode(map[string]any{"url": "http://tracks.example/t1.mp3", "codec": "mp3", "container": "mp3"})
	}))
	t.Cleanup(addon.Close)
	release := sync.OnceFunc(func() { close(hold) })
	var wg sync.WaitGroup
	t.Cleanup(func() {
		release()
		wg.Wait()
	})
	entry := installed{addon: addons.Addon{ID: accounts.ID{1}, Kind: addons.KindEclipse, ManifestURL: addon.URL + "/manifest.json",
		Manifest: stremio.Manifest{Name: "Rig"}, Music: &eclipse.Manifest{}}}
	track := record{ID: accounts.ID{2}, Music: &musicEntry{Type: "track", ID: "t1", Title: "Song", Duration: 10}}
	waiting := make(chan string, 4)
	s.joined = func(key string) { waiting <- key }
	for range 4 {
		wg.Go(func() {
			if version, err := s.resolveTrack(t.Context(), entry, track, false); err != nil || version.URL != "http://tracks.example/t1.mp3" {
				t.Errorf("a play: %+v %v", version, err)
			}
		})
	}
	giveUp := time.After(10 * time.Second)
	for range 4 {
		select {
		case key := <-waiting:
			if key != "track "+track.ID.String() {
				t.Errorf("a play waits on %q", key)
			}
		case <-giveUp:
			t.Fatal("the four plays never waited on a resolution")
		}
	}
	release()
	wg.Wait()
	if n := asked.Load(); n != 1 {
		t.Errorf("4 plays at once asked the addon %d times", n)
	}
}
