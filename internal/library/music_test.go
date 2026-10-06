package library

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
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
	cache := newMusicCache[string](10, func() time.Duration { return time.Hour }, staleMusic, s.now)
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
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got, _ := remember(t.Context(), s, cache, key, fetch); got == "second" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the page was not read again")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if asked.Load() != 2 {
		t.Errorf("%d requests, want 2", asked.Load())
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
	cache := newMusicCache[string](10, func() time.Duration { return time.Hour }, staleMusic, s.now)
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
	cache := newMusicCache[int](100, func() time.Duration { return time.Hour }, staleMusic, s.now)
	var mu sync.Mutex
	running, most := map[accounts.ID]int{}, map[accounts.ID]int{}
	fetch := func(addon accounts.ID) func(context.Context) (int, error) {
		return func(context.Context) (int, error) {
			mu.Lock()
			running[addon]++
			most[addon] = max(most[addon], running[addon])
			mu.Unlock()
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			running[addon]--
			mu.Unlock()
			return 1, nil
		}
	}
	// Two listings of 3 × addonFetches pages each from one addon, and one
	// from another, at once.
	busy, other := accounts.ID{1}, accounts.ID{2}
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
