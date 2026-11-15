package playback

import (
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// The source cache's record of whose announced sizes prove right follows
// the addon itself: two installs of one addon, named alike, keep a record
// each, and a rename keeps the addon's. A version no addon listed has
// none.
func TestSizesAreTrustedByAddon(t *testing.T) {
	t.Parallel()
	version := func(addon accounts.ID, name string) library.Version {
		return library.Version{URL: "https://files.example/movie.mkv", Size: 1000, Addon: name, Origin: library.Origin{Addon: addon}}
	}
	first, second := accounts.ID{1}, accounts.ID{2}
	if a, b := locationOf(version(first, "Streams"), 0).Announcer, locationOf(version(second, "Streams"), 0).Announcer; a == b {
		t.Errorf("two installs named alike share the record %q", a)
	}
	if a, b := locationOf(version(first, "Streams"), 0).Announcer, locationOf(version(first, "Mine"), 0).Announcer; a != b || a == "" {
		t.Errorf("a rename moved the record from %q to %q", a, b)
	}
	if got := locationOf(version(accounts.ID{}, "Recording"), 0).Announcer; got != "" {
		t.Errorf("a version no addon listed: %q", got)
	}
}
