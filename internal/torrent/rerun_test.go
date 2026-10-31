package torrent

import (
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// stopped stops a playback of spec and waits for its torrent to stop, as
// it does once its linger passes.
func stopped(t *testing.T, e *Engine, spec Spec, stop func()) {
	t.Helper()
	stop()
	waitFor(t, "the torrent to stop", func() bool { return !e.Running(spec.Hash) })
}

// A torrent whose sources name no tracker, which the DHT lists with peers
// that never answer, is looked up again while none of its peers answers,
// rather than 5 minutes later: a later lookup reaches a node whose first
// answer was lost, and the seeder it lists, within the start.
func TestADHTLookupIsMadeAgainWhileNoPeerAnswers(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	near := torrenttest.StartDHTNode(t, seeder.Address())
	near.Drop(1)
	first := torrenttest.StartDHTNode(t, torrenttest.Silent(t, "127.0.0.1"), torrenttest.Silent(t, "127.0.0.1"),
		torrenttest.Silent(t, "127.0.0.1"))
	first.TellNodes(netip.MustParseAddrPort(near.Address()))
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) {
		c.Bootstrap = []string{first.Address()}
		c.StartTimeout = 30 * time.Second
	})
	e.dhtAgainWait = 200 * time.Millisecond
	spec, err := NewSpec(tor.HashHex(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
}

// A torrent played again once it stopped dials first the peers its last
// run connected to: it starts though its tracker now lists none.
func TestARunStartsFromThePeersItsLastRunReached(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) { c.StartTimeout = 10 * time.Second })
	first, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(first)
	readAll(t, e, first)
	stopped(t, e, first, stop)

	again, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + empty.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(again)()
	if got := readAll(t, e, again); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
}

// A torrent played again the moment its last run stops, as Running tells
// it stopped and before that run has closed its peers, starts from what
// that run learned: its peers and its info dictionary.
func TestARunPlayedAgainAsTheLastStopsStartsFromIt(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) { c.StartTimeout = 10 * time.Second })
	// The first run that stops holds there until the next has started.
	left, again := make(chan struct{}), make(chan struct{})
	var once sync.Once
	e.left = func(Hash) {
		once.Do(func() {
			close(left)
			<-again
		})
	}
	first, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(first)
	readAll(t, e, first)
	stop()
	<-left
	if e.Running(first.Hash) {
		t.Fatal("the torrent that stops still runs")
	}

	next, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + empty.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(next)()
	tr := torrentOf(e, tor.Hash)
	tr.mu.Lock()
	info, known := tr.info != nil, tr.knows(seeder.Address())
	tr.mu.Unlock()
	close(again)
	if !info || !known {
		t.Fatalf("the next run starts with info %v, its last peer known %v", info, known)
	}
	if got := readAll(t, e, next); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
}

// A run that stops leaves the pieces of a run of the same torrent that
// started once Running told it stopped, before its own pieces were
// removed: they are the new run's.
func TestARunThatStopsLeavesTheNextRunsPieces(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) { c.StartTimeout = 10 * time.Second })
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	// pieces counts the torrent's pieces the store keeps.
	pieces := func() int {
		e.store.mu.Lock()
		defer e.store.mu.Unlock()
		n := 0
		for key := range e.store.pieces {
			if key.hash == tor.Hash {
				n++
			}
		}
		return n
	}
	// As the first run leaves the engine, the next starts and reads the
	// file, before the first has closed its peers and removed its pieces.
	var once sync.Once
	var stopNext func()
	read := 0
	e.left = func(Hash) {
		once.Do(func() {
			stopNext = e.Play(spec)
			if got := readAll(t, e, spec); len(got) != len(large) {
				t.Errorf("the next run read %d bytes", len(got))
			}
			read = pieces()
		})
	}
	stop := e.Play(spec)
	readAll(t, e, spec)
	stop()
	// The first run stops at once, here, as when another title's playback
	// takes its place.
	e.End(spec.Hash)
	if stopNext == nil {
		t.Fatal("the first run never left the engine")
	}
	defer stopNext()
	if read == 0 {
		t.Fatal("the next run kept no piece")
	}
	if got := pieces(); got != read {
		t.Fatalf("the store keeps %d of the next run's %d pieces once the first run stopped", got, read)
	}
}

// A torrent played again once it stopped knows its info dictionary from
// its last run: it starts from a peer that sends pieces but no info.
func TestARunStartsFromTheInfoItsLastRunGot(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) { c.StartTimeout = 10 * time.Second })
	first, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(first)
	readAll(t, e, first)
	stopped(t, e, first, stop)
	seeder.Close()

	quiet := torrenttest.Seed(t, tor, "127.0.0.1")
	quiet.HideMetadata()
	other := torrenttest.HTTPTracker(t, "127.0.0.1", quiet.Address())
	again, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + other.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(again)()
	if got := readAll(t, e, again); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}
}

// A torrent played again once it stopped starts its DHT lookups from the
// nodes nearest it that answered its last run, not from the bootstrap
// nodes alone: they are asked though the bootstrap node no longer answers.
func TestALookupStartsFromTheNodesItsLastRunReached(t *testing.T) {
	t.Parallel()
	tor, _, _ := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	near := torrenttest.StartDHTNode(t, seeder.Address())
	bootstrap := torrenttest.StartDHTNode(t)
	bootstrap.TellNodes(netip.MustParseAddrPort(near.Address()))
	e := newEngineWith(t, proxied(torrenttest.StartProxy(t, true, "", "")), func(c *Config) {
		c.Bootstrap = []string{bootstrap.Address()}
	})
	spec, err := NewSpec(tor.HashHex(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stop := e.Play(spec)
	readAll(t, e, spec)
	stopped(t, e, spec, stop)

	bootstrap.Drop(1 << 20)
	asked := near.Queries()
	defer e.Play(spec)()
	waitFor(t, "the node nearest the torrent to be asked", func() bool { return near.Queries() > asked })
}

// The trackers of the settings are announced to for a torrent whose
// sources name none, which they start; the DHT still looks such a torrent
// up, as the only way its sources leave.
func TestTheSettingsTrackersAreAskedForEveryTorrent(t *testing.T) {
	t.Parallel()
	tor, _, large := twoFiles()
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	settings := proxied(torrenttest.StartProxy(t, true, "", ""))
	settings.set(func(s *Settings) { s.Trackers = []string{tracker.URL()} })
	e := newEngineWith(t, settings, func(c *Config) { c.StartTimeout = 10 * time.Second })
	spec, err := NewSpec(tor.HashHex(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	if got := readAll(t, e, spec); len(got) != len(large) {
		t.Fatalf("read %d bytes", len(got))
	}

	other, _, _ := twoFiles()
	dhtSeeder := torrenttest.Seed(t, other, "127.0.0.1")
	node := torrenttest.StartDHTNode(t, dhtSeeder.Address())
	empty := torrenttest.HTTPTracker(t, "127.0.0.1")
	settings = proxied(torrenttest.StartProxy(t, true, "", ""))
	settings.set(func(s *Settings) { s.Trackers = []string{empty.URL()} })
	e = newEngineWith(t, settings, func(c *Config) {
		c.Bootstrap = []string{node.Address()}
		c.StartTimeout = 10 * time.Second
	})
	spec, err = NewSpec(other.HashHex(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	readAll(t, e, spec)
	waitFor(t, "the settings' tracker to be asked", func() bool { return len(empty.Announces()) > 0 })
}

// A private torrent's trackers are its own (BEP 27): once its info tells
// it is private, the trackers of the settings, announced to while it was
// not known, are told it stopped.
func TestAPrivateTorrentLeavesTheSettingsTrackers(t *testing.T) {
	t.Parallel()
	tor := torrenttest.BuildPrivate("A Movie", 64<<10, torrenttest.File{Name: "a.mkv", Data: torrenttest.Bytes(300 << 10)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	none := torrenttest.HTTPTracker(t, "127.0.0.1")
	own := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	added := torrenttest.HTTPTracker(t, "127.0.0.1")
	settings := proxied(torrenttest.StartProxy(t, true, "", ""))
	settings.set(func(s *Settings) { s.Trackers = []string{added.URL()} })
	e := newEngineWith(t, settings, func(c *Config) { c.StartTimeout = 10 * time.Second })
	// Its first listing finds no peer: the info is not known yet when the
	// settings' tracker is announced to.
	unknown, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + none.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(unknown)()
	waitFor(t, "the settings' tracker to be told the torrent started", func() bool {
		return slices.Contains(added.Announces(), "started")
	})
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + own.URL()})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Play(spec)()
	readAll(t, e, spec)
	waitFor(t, "the settings' tracker to be told the torrent stopped", func() bool {
		return slices.Contains(added.Announces(), "stopped")
	})
}
