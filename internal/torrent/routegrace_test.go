package torrent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// graceEngine is an engine playing through proxy, keeping budget bytes of
// pieces, whose route has grace. A handshake with the proxy waits 2
// seconds rather than 10, and a check's result is kept 100 ms rather than
// 5 seconds, so that a stall outlasts a handshake within seconds.
func graceEngine(t *testing.T, proxy *torrenttest.Proxy, grace time.Duration, budget int64) *Engine {
	t.Helper()
	e := newEngine(t, proxied(proxy), budget)
	e.proxyWait, e.proxyFresh = 2*time.Second, 100*time.Millisecond
	e.routeGrace = grace
	return e
}

// graceSwarm is a 4 MiB movie in pieces of 64 KiB, a seeder of it and a
// tracker listing the seeder.
func graceSwarm(t *testing.T) (*torrenttest.Torrent, *torrenttest.Peer, Spec) {
	t.Helper()
	tor := torrenttest.Build("A Movie", 64<<10, torrenttest.File{Name: "a-movie.mkv", Data: torrenttest.Bytes(4 << 20)})
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	spec, err := NewSpec(tor.HashHex(), nil, []string{"tracker:" + tracker.URL()})
	if err != nil {
		t.Fatal(err)
	}
	return tor, seeder, spec
}

// routeFound waits until the torrent of spec found its route down, or
// stopped.
func routeFound(t *testing.T, e *Engine, spec Spec) {
	t.Helper()
	waitFor(t, "the torrent to find its route down", func() bool {
		return !e.Status(context.Background()).RouteDownSince.IsZero() || !e.Running(spec.Hash)
	})
}

// A proxy that stops answering new connections for longer than its
// handshake's timeout leaves the torrent playing: the connection open to
// its peer goes on through the proxy, and the read goes on to the file's
// end, the pieces past the first megabyte coming during the stall. Once
// the proxy answers again, the outage the play went through is kept.
func TestAPlayReadsOnThroughAProxyStall(t *testing.T) {
	t.Parallel()
	tor, seeder, spec := graceSwarm(t)
	proxy := torrenttest.StartProxy(t, false, "", "")
	// With 2 MiB of pieces kept, a megabyte past the read is fetched.
	e := graceEngine(t, proxy, RouteGrace, 2<<20)
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	start := make([]byte, 64<<10)
	if _, err := r.ReadAt(start, 0); err != nil {
		t.Fatal(err)
	}

	proxy.Stall()
	sent := seeder.Sent()
	routeFound(t, e, spec)
	if !e.Running(spec.Hash) {
		t.Fatal("the torrent stopped as its proxy stalled")
	}
	rest, err := io.ReadAll(io.NewSectionReader(r, int64(len(start)), r.Size()-int64(len(start))))
	if err != nil {
		t.Fatalf("the read during the stall: %v", err)
	}
	if !bytes.Equal(append(start, rest...), tor.Data) {
		t.Fatal("the file read during the stall is not the file")
	}
	if during := seeder.Sent() - sent; during < 2<<20 {
		t.Fatalf("the seeder sent %d bytes during the stall", during)
	}
	if proxy.Held() == 0 || seeder.Open() != 1 {
		t.Fatalf("the proxy held %d connections; the seeder has %d open", proxy.Held(), seeder.Open())
	}

	proxy.Answer()
	waitFor(t, "the outage to be kept", func() bool {
		status := e.Status(context.Background())
		return len(status.Outages) == 1 && status.RouteDownSince.IsZero()
	})
	status := e.Status(context.Background())
	if outage := status.Outages[0]; outage.Route != status.Route || outage.At.IsZero() || outage.For <= 0 || status.Stopped != nil {
		t.Fatalf("Health tells %+v", status)
	}
}

// A proxy that freezes, as when its host sleeps, stops the torrent once
// the route gave no answer, and the peer no byte, for the grace, and not
// before: the read is told the route's failure, and Health that the
// torrent stopped for its route. Meanwhile, the read waiting for its
// pieces is told to wait on, as the source cache's watchdog needs.
func TestAFrozenProxyStopsThePlayOnceItsGraceIsOver(t *testing.T) {
	t.Parallel()
	_, _, spec := graceSwarm(t)
	proxy := torrenttest.StartProxy(t, false, "", "")
	grace := 6 * time.Second
	e := graceEngine(t, proxy, grace, 2<<20)
	defer e.Play(spec)()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := e.Open(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, err := r.ReadAt(make([]byte, 64<<10), 0); err != nil {
		t.Fatal(err)
	}

	frozen := time.Now()
	proxy.Freeze()
	// told is when the read was last told that bytes come, or to wait on.
	var told atomic.Int64
	read := make(chan error, 1)
	go func() {
		buffer := make([]byte, 256<<10)
		for off := int64(0); ; {
			n, err := r.ReadAtProgress(buffer, off, func() { told.Store(time.Now().UnixNano()) })
			if off += int64(n); err != nil {
				if errors.Is(err, io.EOF) && off >= r.Size() {
					err = nil
				}
				read <- err
				return
			}
		}
	}()
	select {
	case err = <-read:
	case <-time.After(time.Minute):
		t.Fatal("the read waits on with the proxy frozen")
	}
	if waited := time.Since(frozen); err == nil || !RouteFailure(err) || waited < grace {
		t.Fatalf("the read ended after %v, within the grace of %v, or not for the route: %v", waited, grace, err)
	}
	if at := told.Load(); at == 0 || time.Unix(0, at).Before(frozen.Add(grace/2)) {
		t.Fatal("the read was not told to wait on during the grace")
	}
	if e.Running(spec.Hash) {
		t.Fatal("the torrent runs on")
	}
	status := e.Status(context.Background())
	if !RouteFailure(status.Stopped) || status.StoppedAt.Before(frozen.Add(grace)) || status.Failure == nil || len(status.Outages) != 0 {
		t.Fatalf("Health tells %+v", status)
	}
}

// A playback that joins a torrent while its proxy does not answer joins it
// without the proxy, and reads it: the torrent watches its route itself.
// A torrent that does not run does not start meanwhile, and nothing is
// dialed another way.
func TestAPlayJoinsATorrentWhoseProxyStalls(t *testing.T) {
	t.Parallel()
	tor, _, spec := graceSwarm(t)
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := graceEngine(t, proxy, RouteGrace, 64<<20)
	defer e.Play(spec)()
	readAll(t, e, spec)

	proxy.Stall()
	routeFound(t, e, spec)
	defer e.Play(spec)()
	tr := torrentOf(e, spec.Hash)
	if tr == nil {
		t.Fatal("the torrent stopped as its proxy stalled")
	}
	tr.mu.Lock()
	refs := tr.refs
	tr.mu.Unlock()
	if refs != 2 {
		t.Fatalf("the torrent has %d playbacks, not 2", refs)
	}
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the playback that joined did not read the file")
	}

	_, otherSeeder, other := graceSwarm(t)
	e.Play(other)()
	if e.Running(other.Hash) {
		t.Fatal("a torrent started with its proxy stalled")
	}
	if _, err := e.Open(context.Background(), other); err == nil {
		t.Fatal("a torrent opened with its proxy stalled")
	}
	if len(otherSeeder.Remotes()) != 0 {
		t.Fatal("the other torrent's seeder was reached")
	}
}

// A torrent stopped for its route keeps its pieces: its next run, once the
// route is back, reads them rather than fetching them again.
func TestATorrentStoppedForItsRouteKeepsItsPieces(t *testing.T) {
	t.Parallel()
	tor, seeder, spec := graceSwarm(t)
	proxy := torrenttest.StartProxy(t, false, "", "")
	e := graceEngine(t, proxy, time.Second, 64<<20)
	stop := e.Play(spec)
	defer stop()
	readAll(t, e, spec)

	proxy.Stop()
	waitFor(t, "the torrent to stop for its route", func() bool { return !e.Running(spec.Hash) })
	if used := e.Usage(); used < int64(len(tor.Data)) {
		t.Fatalf("the pieces kept take %d bytes, not the file's %d", used, len(tor.Data))
	}
	proxy.Resume(t)
	waitFor(t, "the route to answer", func() bool { return e.Ready(context.Background()) == nil })
	sent := seeder.Sent()
	defer e.Play(spec)()
	if got := readAll(t, e, spec); !bytes.Equal(got, tor.Data) {
		t.Fatal("the next run did not read the file")
	}
	if again := seeder.Sent() - sent; again != 0 {
		t.Fatalf("the seeder sent %d bytes again", again)
	}
}
