package playback

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

func replayVersion(n byte, source accounts.ID) library.Version {
	return library.Version{ID: accounts.ID{n}, URL: "http://10.0.0.1/timeshift/" + strconv.Itoa(int(n)) + ".ts", HoldsConnection: true,
		Origin: library.Origin{Addon: source}}
}

// A replay's MPEG-TS file, whose archive tells not its length, lasts as
// long as its programme; another title's such file keeps no length, which
// is not the title's to guess.
func TestReplaysLastAsLongAsTheirProgramme(t *testing.T) {
	s := newService(t, &fileSource{data: bytes.Repeat(tsChunk, 4)}, "ffprobe-not-installed", nil)
	probe := func(media.Prober, context.Context, string) (media.Analysis, error) {
		return media.Analysis{Format: "mpegts"}, nil
	}
	replay := replayVersion(30, accounts.ID{})
	replay.Runtime = 45 * time.Minute
	movie := library.Version{ID: accounts.ID{31}, URL: "https://93.184.216.34/movie.ts", Runtime: 45 * time.Minute}
	for _, tc := range []struct {
		version library.Version
		want    time.Duration
	}{{replay, 45 * time.Minute}, {movie, 0}} {
		analysis, err := s.analyze(t.Context(), tc.version, probe)
		if err != nil || analysis.Duration != tc.want {
			t.Errorf("replay %v: %s %v, want %s", tc.version.HoldsConnection, analysis.Duration, err, tc.want)
		}
	}
}

// A replay whose archive answers an HLS playlist is refused, though
// ffprobe would read it: neither the title path nor the live one plays a
// finite playlist, and the refusal says why.
func TestReplaysAnsweredByAPlaylistAreRefused(t *testing.T) {
	s := newService(t, &fileSource{data: []byte("#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXTINF:4,\nseg0.ts\n#EXT-X-ENDLIST\n")}, "ffprobe-not-installed", nil)
	probe := func(media.Prober, context.Context, string) (media.Analysis, error) {
		return media.Analysis{Format: "hls", Duration: 4 * time.Second}, nil
	}
	replay := replayVersion(30, accounts.ID{})
	if _, err := s.analyze(t.Context(), replay, probe); !errors.Is(err, ErrPlaylistReplay) {
		t.Errorf("a replay answered by a playlist: %v, want refused", err)
	}
}

// A replay takes one of its source's connections as a channel does: on a
// source that plays one stream at a time, it is refused while another
// user watches a channel, takes the place of a channel no one watches
// any more, is shared by the requests of its play, and keeps its place
// from channels until its play ends.
func TestReplaysHoldTheirSourceConnections(t *testing.T) {
	source := accounts.ID{7}
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	s.LiveSources(func(context.Context, accounts.ID) (int, error) { return 1, nil }, nil)
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	channel, err := s.openFeed(alice, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	replay := replayVersion(20, source)
	if _, err := s.Hold(bob, replay); !errors.Is(err, ErrSlotsInUse) {
		t.Fatalf("a replay while another user watches a channel: %v, want refused", err)
	}
	// Once alice left, her channel waiting out its grace makes room.
	_ = channel.Close()
	first, err := s.Hold(bob, replay)
	if err != nil {
		t.Fatalf("a replay once the channel was left: %v", err)
	}
	second, err := s.Hold(bob, replay)
	if err != nil {
		t.Fatalf("another request of the same replay: %v", err)
	}
	if _, err := s.openFeed(alice, channelVersion(2, "2.ts", source)); !errors.Is(err, ErrSlotsInUse) {
		t.Errorf("a channel while a replay plays: %v, want refused", err)
	}
	if _, err := s.Hold(alice, replayVersion(21, source)); !errors.Is(err, ErrSlotsInUse) {
		t.Errorf("another replay while one plays: %v, want refused", err)
	}
	first()
	second()
	// The replay left waits out its grace, then makes room.
	next, err := s.openFeed(alice, channelVersion(2, "2.ts", source))
	if err != nil {
		t.Fatalf("a channel once the replay was left: %v", err)
	}
	_ = next.Close()
	if _, _, most := src.counts(); most != 1 {
		t.Errorf("%d channel connections at once, want 1", most)
	}
}

// Two requests of a replay that come together, while no room can be
// made, both fail: the second waits for the first to know.
func TestRequestsOfAReplayThatComeTogetherShareItsRefusal(t *testing.T) {
	source := accounts.ID{7}
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	entered, gate := make(chan struct{}), make(chan struct{})
	var held atomic.Bool
	s.LiveSources(func(context.Context, accounts.ID) (int, error) {
		if held.Load() {
			entered <- struct{}{}
			<-gate
		}
		return 1, nil
	}, nil)
	channel, err := s.openFeed(ForUser(t.Context(), accounts.ID{1}), channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	held.Store(true)
	bob := ForUser(t.Context(), accounts.ID{2})
	replay := replayVersion(20, source)
	results := make(chan error, 2)
	go func() {
		_, err := s.Hold(bob, replay)
		results <- err
	}()
	<-entered
	held.Store(false)
	go func() {
		_, err := s.Hold(bob, replay)
		results <- err
	}()
	eventually(t, "the second request to join the first", func() bool {
		s.feeds.mu.Lock()
		defer s.feeds.mu.Unlock()
		h := s.feeds.holds[replay.ID]
		return h != nil && h.users == 2
	})
	close(gate)
	for range 2 {
		if err := <-results; !errors.Is(err, ErrSlotsInUse) {
			t.Errorf("a request of a replay with no room: %v, want refused", err)
		}
	}
}
