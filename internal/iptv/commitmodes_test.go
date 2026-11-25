package iptv

import (
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// The details an Xtream server gave for a title are kept in a cache whose
// writes do not wait for the disk; the source added still waits.
func TestDetailsAreKeptWithoutWaitingForTheDisk(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	modes := testdb.WatchCommitModes(t, e.service.db, "iptv_details", "addons")
	_, source, ids := describedShows(t, e, 1)
	must(e.service.Meta(Opening(t.Context()), source, ids[0]))
	for table, want := range map[string]string{"iptv_details": "off", "addons": "on"} {
		if got := modes(table); len(got) == 0 || slices.ContainsFunc(got, func(mode string) bool { return mode != want }) {
			t.Errorf("%s written with synchronous_commit %v, want %s", table, got, want)
		}
	}
}
