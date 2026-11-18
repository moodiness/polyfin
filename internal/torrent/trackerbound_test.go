package torrent

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// listingOf is listing n of a torrent: trackers of its own, each at a
// name of the tests' public address in names, and nodes of its own.
func listingOf(t *testing.T, names map[string]netip.Addr, n, trackers, nodes int) Spec {
	t.Helper()
	sources := make([]string, 0, trackers+nodes)
	for i := range trackers {
		name := fmt.Sprintf("tracker-%d-%d.test", n, i)
		names[name] = torrenttest.Public
		sources = append(sources, "tracker:http://"+name+"/announce")
	}
	for i := range nodes {
		sources = append(sources, fmt.Sprintf("dht:node-%d-%d.test:6881", n, i))
	}
	spec, err := NewSpec(strings.Repeat("cd", 20), nil, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Trackers) != trackers || len(spec.Nodes) != nodes {
		t.Fatalf("listing %d names %d trackers and %d nodes, want %d and %d", n, len(spec.Trackers), len(spec.Nodes), trackers, nodes)
	}
	return spec
}

// announcedTo are the names of the listings' trackers the route was asked
// to look up, each once: those the torrent announces to.
func announcedTo(route *lookupRoute) []string {
	route.mu.Lock()
	defer route.mu.Unlock()
	names := slices.Clone(route.looked)
	slices.Sort(names)
	return slices.Compact(names)
}

// A source lists as many listings of one torrent as it likes, and each
// play of another listing of a torrent that runs adds its trackers. All
// of them together are no more than one stream may name, the trackers of
// the settings aside, and no announce runs for one that was not kept.
func TestTheListingsOfATorrentNameNoMoreTrackersThanOneStream(t *testing.T) {
	t.Parallel()
	const listings = 20
	const ofSettings = "http://settings.test/announce"
	names := map[string]netip.Addr{}
	specs := make([]Spec, listings)
	for n := range specs {
		specs[n] = listingOf(t, names, n, MaxTrackers, maxNodes)
	}
	route := &lookupRoute{names: names}
	e := publicEngine(t, &testSettings{s: Settings{Enabled: true}})
	tr := newTorrent(e, specs[0], route, false)
	defer tr.cancel()
	for _, tracker := range specs[0].Trackers {
		tr.goAnnounce(tracker)
	}
	for _, spec := range specs[1:] {
		// What Engine.join does for a play of another listing of a torrent
		// that runs.
		tr.merge(spec, []string{ofSettings})
	}
	tr.mu.Lock()
	trackers, named, nodes := slices.Clone(tr.trackers), tr.named(), len(tr.nodes)
	tr.mu.Unlock()
	if named != MaxTrackers || len(trackers) != MaxTrackers+1 {
		t.Fatalf("%d trackers, %d of them named by the listings, want %d and the one of the settings", len(trackers), named, MaxTrackers)
	}
	if want := append(slices.Clone(specs[0].Trackers), ofSettings); !slices.Equal(trackers, want) {
		t.Fatalf("the trackers kept are %q, want the first named and the one of the settings", trackers)
	}
	if nodes != maxNodesLearned {
		t.Fatalf("%d nodes, want the %d a torrent learns at most", nodes, maxNodesLearned)
	}
	// Every tracker kept is announced to, the one of the settings by its
	// name as the route carries it, and no other.
	waitFor(t, "every tracker kept to be announced to", func() bool {
		route.mu.Lock()
		settings := slices.Contains(route.reached, "tcp settings.test:80")
		route.mu.Unlock()
		return settings && len(announcedTo(route)) >= MaxTrackers
	})
	want := make([]string, 0, MaxTrackers)
	for i := range MaxTrackers {
		want = append(want, fmt.Sprintf("tracker-0-%d.test", i))
	}
	slices.Sort(want)
	if got := announcedTo(route); !slices.Equal(got, want) {
		t.Fatalf("announced to %d trackers of the listings, want the %d first named: %q", len(got), MaxTrackers, got)
	}
}

// While the listings name fewer trackers than one stream may, another
// listing still adds its own, the first it names, up to the bound; a
// tracker of the settings that a listing names too is kept either way.
func TestAnotherListingAddsItsTrackersWhileThereIsRoom(t *testing.T) {
	t.Parallel()
	const ofSettings = "http://settings.test/announce"
	names := map[string]netip.Addr{}
	first := listingOf(t, names, 0, 10, 0)
	second := listingOf(t, names, 1, MaxTrackers-1, 0)
	second.Trackers = append(second.Trackers, ofSettings)
	route := &lookupRoute{names: names}
	e := publicEngine(t, &testSettings{s: Settings{Enabled: true}})
	tr := newTorrent(e, first, route, false)
	defer tr.cancel()
	tr.merge(Spec{Hash: first.Hash}, []string{ofSettings})
	tr.merge(second, []string{ofSettings})
	tr.mu.Lock()
	trackers, named := slices.Clone(tr.trackers), tr.named()
	tr.mu.Unlock()
	want := append(slices.Clone(first.Trackers), ofSettings)
	want = append(want, second.Trackers[:MaxTrackers-len(first.Trackers)]...)
	if named != MaxTrackers || !slices.Equal(trackers, want) {
		t.Fatalf("%d trackers named by the listings, kept %q, want %d: the first listing's, the settings' and the first %d of the second",
			named, trackers, MaxTrackers, MaxTrackers-len(first.Trackers))
	}
}
