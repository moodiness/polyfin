package playback

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/iptv"
)

// tunerService is a service whose sources are all a tuner of limit.
func tunerService(t *testing.T, limit iptv.Limit) (*Service, *liveSource) {
	t.Helper()
	src := &liveSource{interval: 5 * time.Millisecond}
	s := liveService(t, src, "ffprobe-not-installed")
	s.LiveSources(func(context.Context, accounts.ID) (iptv.Limit, error) { return limit, nil }, nil)
	return s, src
}

// reads reports whether a reader still reads its feed.
func reads(r *feedReader) error {
	_, err := r.Read(make([]byte, 10))
	return err
}

// A channel's stream is read once for every viewer, so two users on one
// channel take one tuner; with every tuner taken by other users' channels,
// another channel is refused at once.
func TestTwoUsersOnOneChannelTakeOneTuner(t *testing.T) {
	source := accounts.ID{7}
	s, src := tunerService(t, iptv.Limit{Connections: 1, Tuners: true})
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	first, err := s.openFeed(alice, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := s.openFeed(bob, channelVersion(1, "1.ts", source))
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
	if _, err := s.openFeed(bob, channelVersion(2, "2.ts", source)); !errors.Is(err, ErrTunersInUse) || !errors.Is(err, ErrSlotsInUse) || LiveFailure(err) != LiveRefused {
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
	source := accounts.ID{7}
	s, _ := tunerService(t, iptv.Limit{Connections: 2, Tuners: true})
	alice, bob := ForUser(t.Context(), accounts.ID{1}), ForUser(t.Context(), accounts.ID{2})
	watched, err := s.openFeed(alice, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer watched.Close()
	other, err := s.openFeed(bob, channelVersion(2, "2.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	recorded, err := s.openFeed(forRecording(t.Context()), channelVersion(3, "3.ts", source))
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
	second, err := s.openFeed(forRecording(t.Context()), channelVersion(4, "4.ts", source))
	if err != nil {
		t.Fatalf("a second recording: %v", err)
	}
	defer second.Close()
	// Every tuner records: a recording due fails, through Record as the
	// recordings service calls it, and a viewer is refused.
	err = s.Record(t.Context(), accounts.ID{9}, channelVersion(5, "5.ts", source), "third", filepath.Join(t.TempDir(), "third.ts"))
	if !errors.Is(err, iptv.ErrTunersRecording) || !errors.Is(err, ErrSlotsInUse) {
		t.Errorf("a recording with every tuner recording: %v", err)
	}
	if _, err := s.openFeed(alice, channelVersion(6, "6.ts", source)); !errors.Is(err, ErrSlotsInUse) || errors.Is(err, iptv.ErrTunersRecording) {
		t.Errorf("a channel watched while every tuner records: %v", err)
	}
	for _, r := range []*feedReader{recorded, second} {
		if err := reads(r); err != nil {
			t.Errorf("a recording lost its tuner: %v", err)
		}
	}
}

// An IPTV account's connections are kept for recordings as before tuners:
// a recording takes no stream another user watches, and fails as refused,
// not for want of a tuner; one of a channel watched shares its stream.
func TestIPTVRecordingsKeepTheirConnectionRules(t *testing.T) {
	source := accounts.ID{7}
	s, src := tunerService(t, iptv.Limit{Connections: 1})
	alice := ForUser(t.Context(), accounts.ID{1})
	watched, err := s.openFeed(alice, channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatal(err)
	}
	defer watched.Close()
	err = s.Record(t.Context(), accounts.ID{9}, channelVersion(2, "2.ts", source), "other", filepath.Join(t.TempDir(), "other.ts"))
	if !errors.Is(err, ErrSlotsInUse) || errors.Is(err, iptv.ErrTunersRecording) {
		t.Errorf("a recording of another channel: %v, want refused", err)
	}
	if err := reads(watched); err != nil {
		t.Errorf("the channel watched: %v", err)
	}
	recorded, err := s.openFeed(forRecording(t.Context()), channelVersion(1, "1.ts", source))
	if err != nil {
		t.Fatalf("a recording of the channel watched: %v", err)
	}
	defer recorded.Close()
	if _, opened, most := src.counts(); opened != 1 || most != 1 {
		t.Errorf("%d connections opened, %d at once, want 1", opened, most)
	}
}
