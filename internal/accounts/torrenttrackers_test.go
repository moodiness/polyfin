package accounts

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The trackers added to every torrent are saved trimmed, each once, empty
// lines left out, read again by a new start, and refused when one is not
// an HTTP, HTTPS or UDP announce URL, or when there are too many.
func TestTorrentTrackersAreSaved(t *testing.T) {
	store := newStore(t)
	ctx := t.Context()
	changed := store.Settings()
	changed.TorrentTrackers = []string{" udp://tracker.example:1337/announce ", "", "https://tracker.example/announce",
		"udp://tracker.example:1337/announce"}
	want := []string{"udp://tracker.example:1337/announce", "https://tracker.example/announce"}
	saved, err := store.UpdateSettings(ctx, changed)
	if err != nil || !slices.Equal(saved.TorrentTrackers, want) {
		t.Fatalf("saved: %q %v", saved.TorrentTrackers, err)
	}
	reopened, err := Open(ctx, store.db)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Settings().TorrentTrackers; !slices.Equal(got, want) {
		t.Errorf("read again: %q", got)
	}
	for _, wrong := range []string{"udp://tracker.example/announce", "ftp://tracker.example/announce", "tracker.example:1337",
		"http://user:secret@tracker.example/announce", "http://tracker.example/" + strings.Repeat("a", MaxTorrentTrackerBytes)} {
		changed.TorrentTrackers = []string{wrong}
		if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidTorrentTrackers) {
			t.Errorf("%q: %v", wrong, err)
		}
	}
	changed.TorrentTrackers = nil
	for i := range MaxTorrentTrackers + 1 {
		changed.TorrentTrackers = append(changed.TorrentTrackers, "udp://tracker.example:"+strconv.Itoa(1000+i)+"/announce")
	}
	if _, err := store.UpdateSettings(ctx, changed); !errors.Is(err, ErrInvalidTorrentTrackers) {
		t.Errorf("%d trackers: %v", len(changed.TorrentTrackers), err)
	}
	changed.TorrentTrackers = nil
	if saved, err := store.UpdateSettings(ctx, changed); err != nil || saved.TorrentTrackers == nil || len(saved.TorrentTrackers) != 0 {
		t.Errorf("emptied: %#v %v", saved.TorrentTrackers, err)
	}
}
