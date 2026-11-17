package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
	"github.com/moodiness/polyfin/internal/library"
)

// tunerService is a service whose sources are all a tuner of limit. The
// tests' streams are "paced" ones: each sends at once what its readers
// start from, then nothing.
func tunerService(t *testing.T, limit iptv.Limit) (*Service, *liveSource) {
	t.Helper()
	src := &liveSource{}
	s := liveService(t, src, "ffprobe-not-installed")
	// The streams pause as long as the test takes: none may end as
	// stalled, which would free its tuner.
	s.feeds.times.stall = time.Minute
	s.LiveSources(func(context.Context, accounts.ID) (iptv.Limit, error) { return limit, nil }, nil)
	return s, src
}

// reads reports whether a reader still reads its feed.
func reads(r *feedReader) error {
	_, err := r.Read(make([]byte, 10))
	return err
}

// A tuner that answers a plain 503, without HDHomeRun's error 805, as a
// proxy acting as a tuner does once its connections are all in use, is
// busy: no fault of the tuner's. Another source's 503 stays a refusal.
func TestATunersPlainUnavailableIsBusy(t *testing.T) {
	t.Parallel()
	path, _ := fakeProbe(t, "{}", false)
	s := liveService(t, &liveSource{}, path)
	tuner, provider := accounts.ID{7}, accounts.ID{8}
	var mu sync.Mutex
	reported := map[accounts.ID]string{}
	s.LiveSources(func(_ context.Context, source accounts.ID) (iptv.Limit, error) {
		return iptv.Limit{Connections: 2, Tuners: source == tuner}, nil
	}, func(_ context.Context, version library.Version, failure string) {
		mu.Lock()
		reported[version.ID] = failure
		mu.Unlock()
	})
	for n, tc := range []struct {
		source accounts.ID
		want   string
	}{{tuner, iptv.TunerFailureBusy}, {provider, LiveRefused}} {
		version := channelVersion(byte(n+1), "unavailable.ts", tc.source)
		if _, err := s.AnalyzeLive(t.Context(), version); LiveFailure(err) != LiveRefused {
			t.Errorf("source %v: %v, want a refusal", tc.source, err)
		}
		mu.Lock()
		if reported[version.ID] != tc.want {
			t.Errorf("source %v: reported %q, want %q", tc.source, reported[version.ID], tc.want)
		}
		mu.Unlock()
	}
}

// A channel's stream is read once for every viewer, so two users on one
// channel take one tuner; with every tuner taken by other users' channels,
// another channel is refused at once.
func TestTwoUsersOnOneChannelTakeOneTuner(t *testing.T) {
	t.Parallel()
	source := accounts.ID{7}
	s, src := tunerService(t, iptv.Limit{Connections: 1, Tuners: true})
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	first, err := s.openFeed(alice, channelVersion(1, "paced-1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := s.openFeed(bob, channelVersion(1, "paced-1.ts", source))
	if err != nil {
		t.Fatalf("the same channel for another user: %v", err)
	}
	defer second.Close()
	if err := reads(first); err != nil {
		t.Errorf("the first viewer: %v", err)
	}
	if err := reads(second); err != nil {
		t.Errorf("the second viewer: %v", err)
	}
	if _, err := s.openFeed(bob, channelVersion(2, "paced-2.ts", source)); !errors.Is(err, ErrTunersInUse) || !errors.Is(err, ErrSlotsInUse) || LiveFailure(err) != LiveRefused {
		t.Errorf("a channel past the tuner count: %v, want refused", err)
	}
	if _, _, most := src.counts(); most != 1 {
		t.Errorf("%d tuners at once, want 1", most)
	}
}

// A recording due takes the tuner of a channel only watched, whoever
// watches it, and never another recording's: one that finds every tuner
// recording fails, and so does a viewer's channel.
func TestARecordingTakesAWatchedChannelsTuner(t *testing.T) {
	t.Parallel()
	source := accounts.ID{7}
	s, _ := tunerService(t, iptv.Limit{Connections: 2, Tuners: true})
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	watched, err := s.openFeed(alice, channelVersion(1, "paced-1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer watched.Close()
	other, err := s.openFeed(bob, channelVersion(2, "paced-2.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	recorded, err := s.openFeed(forRecording(t.Context()), channelVersion(3, "paced-3.ts", source))
	if err != nil {
		t.Fatalf("a recording with every tuner watched: %v", err)
	}
	defer recorded.Close()
	if err := reads(watched); !errors.Is(err, ErrTunersInUse) || !strings.Contains(err.Error(), "tuners are in use") {
		t.Errorf("the oldest channel watched: %v, want its tuner taken", err)
	}
	if err := reads(other); err != nil {
		t.Errorf("the other channel watched: %v", err)
	}
	second, err := s.openFeed(forRecording(t.Context()), channelVersion(4, "paced-4.ts", source))
	if err != nil {
		t.Fatalf("a second recording: %v", err)
	}
	defer second.Close()
	// Every tuner records: a recording due fails, through Record as the
	// recordings service calls it, and a viewer is refused.
	err = s.Record(t.Context(), accounts.ID{9}, channelVersion(5, "paced-5.ts", source), "third", filepath.Join(t.TempDir(), "third.ts"))
	if !errors.Is(err, iptv.ErrTunersRecording) || !errors.Is(err, ErrSlotsInUse) {
		t.Errorf("a recording with every tuner recording: %v", err)
	}
	if _, err := s.openFeed(alice, channelVersion(6, "paced-6.ts", source)); !errors.Is(err, ErrSlotsInUse) || errors.Is(err, iptv.ErrTunersRecording) {
		t.Errorf("a channel watched while every tuner records: %v", err)
	}
	for _, r := range []*feedReader{recorded, second} {
		if err := reads(r); err != nil {
			t.Errorf("a recording lost its tuner: %v", err)
		}
	}
}

// A channel whose source ended the stream, a viewer still on it, holds
// its tuner until its connection is closed: a channel opened meanwhile
// waits for that, and the tuner is never taken twice.
func TestAnEndedChannelHoldsItsTunerUntilClosed(t *testing.T) {
	t.Parallel()
	source := accounts.ID{7}
	s, src := tunerService(t, iptv.Limit{Connections: 1, Tuners: true})
	// The source holds each connection whose request ended until the
	// test lets it close (see liveSource.closing).
	src.closing = make(chan struct{})
	var release sync.Once
	closeConnections := func() { release.Do(func() { close(src.closing) }) }
	defer closeConnections()
	waited := make(chan struct{})
	s.feeds.awaiting = func(closing []*feed) {
		closeConnections()
		for _, f := range closing {
			<-f.closed
		}
		close(waited)
	}
	ended, err := s.openFeed(ForUser(t.Context(), accounts.ID{1}), channelVersion(1, "ending-1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer ended.Close()
	// The viewer reads the channel to its end: the feed ended, and its
	// connection stays open until the source lets it close.
	if _, err := io.Copy(io.Discard, ended); err != nil {
		t.Fatal(err)
	}
	next, err := s.openFeed(ForUser(t.Context(), accounts.ID{2}), channelVersion(2, "paced-2.ts", source))
	if err != nil {
		t.Fatalf("a channel once the ended one's tuner was free: %v", err)
	}
	defer next.Close()
	select {
	case <-waited:
	default:
		t.Error("a channel opened without waiting for the ended one's connection to close")
	}
	closeConnections()
	if _, _, most := src.counts(); most != 1 {
		t.Errorf("%d tuners at once, want 1", most)
	}
}

// Channels opened within one tick of the clock, as Windows' is, have the
// same time by it: the first opened is still the oldest. Each recording
// due takes the tuner of the first channel opened that none records.
// Every round has a tuner of its own, its channels the same time.
func TestRecordingsTakeTheFirstOpenedChannelsTuners(t *testing.T) {
	t.Parallel()
	const tuners = 6
	s, _ := tunerService(t, iptv.Limit{Connections: tuners, Tuners: true})
	round := func(source accounts.ID) {
		t.Helper()
		channel := func(n int) library.Version {
			return channelVersion(source[0]*16+byte(n), fmt.Sprintf("paced-%d.ts", n), source)
		}
		var watched []*feedReader
		for n := 1; n <= tuners; n++ {
			r, err := s.openFeed(ForUser(t.Context(), accounts.ID{byte(n)}), channel(n))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			watched = append(watched, r)
		}
		at := time.Now()
		s.feeds.mu.Lock()
		for _, f := range s.feeds.feeds {
			if f.source == source {
				f.opened.at = at
			}
		}
		s.feeds.mu.Unlock()
		for n := 1; n <= 3; n++ {
			recorded, err := s.openFeed(forRecording(t.Context()), channel(tuners+n))
			if err != nil {
				t.Fatalf("tuner %d, recording %d: %v", source[0], n, err)
			}
			defer recorded.Close()
			for i, r := range watched {
				if err := reads(r); errors.Is(err, ErrTunersInUse) != (i < n) {
					t.Errorf("tuner %d, after recording %d, channel %d watched: %v", source[0], n, i+1, err)
				}
			}
		}
	}
	for n := range byte(8) {
		round(accounts.ID{n + 1})
	}
}

// An IPTV account's connections are kept for recordings as before tuners:
// a recording takes no stream another user watches, and fails as refused,
// not for want of a tuner; one of a channel watched shares its stream.
func TestIPTVRecordingsKeepTheirConnectionRules(t *testing.T) {
	t.Parallel()
	source := accounts.ID{7}
	s, src := tunerService(t, iptv.Limit{Connections: 1})
	alice := ForUser(t.Context(), accounts.ID{1})
	watched, err := s.openFeed(alice, channelVersion(1, "paced-1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer watched.Close()
	err = s.Record(t.Context(), accounts.ID{9}, channelVersion(2, "paced-2.ts", source), "other", filepath.Join(t.TempDir(), "other.ts"))
	if !errors.Is(err, ErrSlotsInUse) || errors.Is(err, iptv.ErrTunersRecording) {
		t.Errorf("a recording of another channel: %v, want refused", err)
	}
	if err := reads(watched); err != nil {
		t.Errorf("the channel watched: %v", err)
	}
	recorded, err := s.openFeed(forRecording(t.Context()), channelVersion(1, "paced-1.ts", source))
	if err != nil {
		t.Fatalf("a recording of the channel watched: %v", err)
	}
	defer recorded.Close()
	if _, opened, most := src.counts(); opened != 1 || most != 1 {
		t.Errorf("%d connections opened, %d at once, want 1", opened, most)
	}
}
