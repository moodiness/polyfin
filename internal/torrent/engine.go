// Package torrent plays the torrents addons list: it joins a torrent's
// swarm while a playback reads it, fetches the pieces at the read position
// first, and keeps every socket on one route, a network interface, a
// SOCKS5 proxy or a WireGuard tunnel run inside Polyfin, with nothing sent
// another way.
package torrent

import (
	"context"
	"errors"
	"log/slog"
	"net/netip"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// ErrDisabled reports torrents played while the server does not play them.
var ErrDisabled = errors.New("torrent streams are turned off")

// ErrNotPlaying reports a read of a torrent no playback started: only a
// playback joins a torrent's swarm.
var ErrNotPlaying = errors.New("the torrent is read only for a playback")

// ErrAtCap reports a torrent that does not start, as many torrents
// running, or lingering after their playbacks, as Settings.AtOnce allows.
// It may start once one of them stops.
var ErrAtCap = errors.New("as many torrents run as the server allows")

// Settings are the server's settings for torrents.
type Settings struct {
	Enabled bool
	// Interface is the network interface every socket is bound to; else
	// Proxy is the SOCKS5 proxy, host:port, with ProxyUser and
	// ProxyPassword when it asks for them; else WireGuard is the
	// configuration of a WireGuard tunnel (see ParseWireGuardConfig).
	Interface                       string
	Proxy, ProxyUser, ProxyPassword string
	WireGuard                       string
	DownloadMbps, UploadMbps        int
	// AtOnce bounds the torrents running at once, those lingering after
	// their playbacks included, 0 for no limit: a torrent that would be
	// one more does not start (see ErrAtCap).
	AtOnce int
	// Trackers are announced to for every torrent besides those its
	// sources name, MaxTrackers at most (see ValidTracker), but for a
	// private one, whose trackers are its own (BEP 27). They leave the
	// DHT as it is: a torrent whose sources name no tracker is still
	// looked up there.
	Trackers []string
}

// routeSettings are the settings that make the route.
type routeSettings struct {
	Interface, Proxy, ProxyUser, ProxyPassword, WireGuard string
}

// keptFor is how long what a torrent learned is kept once it stopped,
// for its next run (see Engine.keepLocked): a replay after a refresh, or
// the next play of the title within the 15 minutes a failed version is
// left out. maxKept bounds the torrents kept, and maxKeptInfo the bytes
// their info dictionaries take.
const (
	keptFor     = 15 * time.Minute
	maxKept     = 64
	maxKeptInfo = 64 << 20
)

// keptRun is what a stopped torrent learned, for its next run until
// until: the peers it connected to, the latest first, its info
// dictionary, nil when none came, and the DHT nodes nearest it that
// answered its last lookup.
type keptRun struct {
	peers    []netip.AddrPort
	info     *Info
	infoData []byte
	nodes    []string
	until    time.Time
}

// RouteOf returns the route settings describe, ErrNoRoute for none;
// logger takes what the route logs.
func RouteOf(s Settings, logger *slog.Logger) (Route, error) {
	switch {
	case s.Interface != "":
		return InterfaceRoute(s.Interface), nil
	case s.Proxy != "":
		return ProxyRoute(s.Proxy, s.ProxyUser, s.ProxyPassword), nil
	case s.WireGuard != "":
		config, err := ParseWireGuardConfig(s.WireGuard)
		if err != nil {
			return nil, err
		}
		return WireGuardRoute(config, logger), nil
	}
	return nil, ErrNoRoute
}

// Config is what an engine needs.
type Config struct {
	// Settings returns the settings, read as they apply.
	Settings func() Settings
	// Dir holds the pieces, within Budget's bytes.
	Dir    string
	Budget func() int64
	// Linger is how long a torrent runs once its playbacks ended, as a
	// player pauses between requests; 0 for 30 seconds.
	Linger time.Duration
	// StartTimeout bounds the wait for a torrent's start (see Open); 0 for
	// StartTimeout.
	StartTimeout time.Duration
	// Bootstrap lists the DHT nodes, host:port, a lookup starts from
	// besides those the addon's sources name (see BootstrapNodes); none
	// when empty, as in tests.
	Bootstrap []string
	Logger    *slog.Logger
}

// Engine runs the torrents being played.
type Engine struct {
	config Config
	store  *store
	peerID [20]byte
	up     *limiter
	down   *limiter
	// checkEvery is how often a running torrent's route is checked.
	checkEvery time.Duration
	// tunnelFresh and tunnelWait, when set, replace a WireGuard tunnel's
	// times (see tunnelFresh).
	tunnelFresh, tunnelWait time.Duration
	// noEndgame turns the endgame off, to measure what it brings.
	noEndgame bool
	// dropWait is how long a request a peer skipped waits before it is
	// taken as dropped, and idleTimeout how long a peer may send nothing,
	// or no block while blocks are asked of it (see torrent.reask); tests
	// shorten them.
	dropWait, idleTimeout time.Duration
	// redialWait, when set, replaces how long a peer waits before it is
	// dialed again (see torrent.redial), and dhtAgainWait how soon the
	// DHT is looked up again while a torrent has no peer (see
	// torrent.lookup); tests shorten them.
	redialWait, dhtAgainWait time.Duration
	// utpOff, set by tests, reaches peers over TCP alone.
	utpOff bool
	// left, set by tests, is called once a torrent that stops has left
	// the engine, before its peers are closed (see remove).
	left func(Hash)

	mu       sync.Mutex
	torrents map[Hash]*torrent
	// routeKey and route are the route of the settings last read.
	routeKey routeSettings
	route    Route
	// failure is why torrents last could not play, and when.
	failure   error
	failedAt  time.Time
	noUDP     bool
	udpRoute  Route
	sharedOut atomic.Int64
	// unstarted is the last torrent that did not start in time, and when;
	// nil once another started since.
	unstarted   *StartError
	unstartedAt time.Time
	// removed is closed, and replaced, as a torrent stops (see
	// AdmitsWithin).
	removed chan struct{}
	// kept is what stopped torrents learned, by info hash (see keepLocked).
	kept map[Hash]keptRun
}

// New returns an engine keeping pieces in config.Dir.
func New(config Config) (*Engine, error) {
	if config.Linger == 0 {
		config.Linger = 30 * time.Second
	}
	if config.StartTimeout == 0 {
		config.StartTimeout = StartTimeout
	}
	s, err := newStore(config.Dir, config.Budget, config.Logger)
	if err != nil {
		return nil, err
	}
	e := &Engine{config: config, store: s, torrents: map[Hash]*torrent{}, removed: make(chan struct{}), checkEvery: checkEvery, dropWait: dropWait,
		idleTimeout: idleTimeout, kept: map[Hash]keptRun{}}
	e.peerID = newPeerID()
	mbps := func(n int) int64 { return int64(n) * 1_000_000 / 8 }
	e.up = newLimiter(func() int64 { return mbps(config.Settings().UploadMbps) }, true)
	e.down = newLimiter(func() int64 { return mbps(config.Settings().DownloadMbps) }, false)
	s.evicted = func(key pieceKey) {
		e.mu.Lock()
		t := e.torrents[key.hash]
		e.mu.Unlock()
		if t != nil {
			t.evicted(key.index)
		}
	}
	return e, nil
}

// currentRoute is the route the settings set now, an error when torrents
// are off or none is set. A route the settings no longer set is closed: a
// WireGuard tunnel stops.
func (e *Engine) currentRoute() (Route, error) {
	settings := e.config.Settings()
	key := routeSettings{Interface: settings.Interface, Proxy: settings.Proxy, ProxyUser: settings.ProxyUser, ProxyPassword: settings.ProxyPassword,
		WireGuard: settings.WireGuard}
	e.mu.Lock()
	if settings.Enabled && e.route != nil && e.routeKey == key {
		route := e.route
		e.mu.Unlock()
		return route, nil
	}
	old := e.route
	e.route, e.routeKey = nil, routeSettings{}
	route, err := Route(nil), ErrDisabled
	if settings.Enabled {
		route, err = RouteOf(settings, e.config.Logger)
	}
	if err == nil {
		if tunnel, ok := route.(*wireGuardRoute); ok && e.tunnelFresh != 0 {
			tunnel.fresh, tunnel.wait = e.tunnelFresh, e.tunnelWait
		}
		e.route, e.routeKey = route, key
	}
	e.mu.Unlock()
	if old != nil {
		old.Close()
	}
	return route, err
}

// failed records why torrents could not play, for Health.
func (e *Engine) failed(err error) {
	e.mu.Lock()
	e.failure, e.failedAt = err, time.Now()
	e.mu.Unlock()
	e.config.Logger.Warn("A torrent could not play over its route", "error", err)
}

// Play joins the torrent of spec, for a playback, until stop is called and
// the linger after it passes. Over a route that cannot carry it, nothing
// starts: its reads fail with the reason (see Open).
func (e *Engine) Play(spec Spec) (stop func()) {
	t := e.start(spec)
	if t == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(func() { e.release(t) }) }
}

func (e *Engine) start(spec Spec) *torrent {
	settings := e.config.Settings()
	limit, extra := settings.AtOnce, settingsTrackers(settings.Trackers)
	route, err := e.currentRoute()
	if err != nil {
		if !errors.Is(err, ErrDisabled) {
			e.failed(err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), proxyTimeout)
	defer cancel()
	if err := route.Check(ctx); err != nil {
		e.failed(err)
		return nil
	}
	e.mu.Lock()
	t := e.torrents[spec.Hash]
	if t != nil {
		t.mu.Lock()
		if t.err == nil {
			t.joined()
			t.mu.Unlock()
			e.mu.Unlock()
			t.merge(spec, extra)
			return t
		}
		t.mu.Unlock()
	}
	if e.fullLocked(spec.Hash, limit) {
		e.mu.Unlock()
		e.capped(limit)
		return nil
	}
	e.mu.Unlock()
	udp := e.carriesUDP(ctx, route)
	t = newTorrent(e, spec, route, udp)
	t.refs = 1
	e.mu.Lock()
	if old := e.torrents[spec.Hash]; old != nil && old != t {
		old.mu.Lock()
		running := old.err == nil
		if running {
			old.joined()
		}
		old.mu.Unlock()
		if running {
			e.mu.Unlock()
			old.merge(spec, extra)
			return old
		}
	}
	// Of two torrents starting at once for the last place, the first to
	// get here takes it.
	if e.fullLocked(spec.Hash, limit) {
		e.mu.Unlock()
		t.cancel()
		e.capped(limit)
		return nil
	}
	e.torrents[spec.Hash] = t
	// No disk work: a number, so that an earlier run that stops leaves
	// this one's pieces.
	t.run = e.store.started(spec.Hash)
	kept, seeded := e.keptLocked(spec.Hash)
	e.mu.Unlock()
	if seeded {
		t.seed(kept)
	}
	t.start(extra)
	return t
}

// settingsTrackers are the trackers of the settings that are valid, each
// once, MaxTrackers at most.
func settingsTrackers(trackers []string) []string {
	valid := make([]string, 0, min(len(trackers), MaxTrackers))
	for _, tracker := range trackers {
		if len(valid) < MaxTrackers && ValidTracker(tracker) && !slices.Contains(valid, tracker) {
			valid = append(valid, tracker)
		}
	}
	return valid
}

// keptLocked returns what the last run of the torrent of hash learned,
// while it is kept. e.mu is held.
func (e *Engine) keptLocked(hash Hash) (keptRun, bool) {
	kept, ok := e.kept[hash]
	if !ok || time.Now().After(kept.until) {
		delete(e.kept, hash)
		return keptRun{}, false
	}
	return kept, true
}

// keepLocked keeps, for keptFor, what a torrent that stops learned, for
// its next run (see torrent.seed): the peers it connected to, its info
// dictionary, and the DHT nodes nearest it. A run that connected to no
// peer and got no info learned nothing new: what an earlier run kept
// stays as it was. The kept torrents stay within maxKept and maxKeptInfo,
// those kept longest dropped first. e's lock is held.
func (e *Engine) keepLocked(t *torrent) {
	t.mu.Lock()
	run := keptRun{peers: slices.Clone(t.reached), info: t.info, infoData: t.infoData, nodes: slices.Clone(t.closest)}
	private := t.private()
	t.mu.Unlock()
	if len(run.peers) == 0 && run.info == nil {
		return
	}
	slices.Reverse(run.peers)
	if private {
		// Its peers come from its trackers alone.
		run.nodes = nil
	}
	now := time.Now()
	run.until = now.Add(keptFor)
	if before, ok := e.keptLocked(t.hash); ok {
		for _, address := range before.peers {
			if len(run.peers) < keptPeers && !slices.Contains(run.peers, address) {
				run.peers = append(run.peers, address)
			}
		}
		if run.info == nil {
			run.info, run.infoData = before.info, before.infoData
		}
		if len(run.nodes) == 0 && !private {
			run.nodes = before.nodes
		}
	}
	e.kept[t.hash] = run
	size := 0
	for hash, kept := range e.kept {
		if now.After(kept.until) {
			delete(e.kept, hash)
			continue
		}
		size += len(kept.infoData)
	}
	for len(e.kept) > maxKept || size > maxKeptInfo {
		var oldest Hash
		first := true
		for hash, kept := range e.kept {
			if first || kept.until.Before(e.kept[oldest].until) {
				oldest, first = hash, false
			}
		}
		size -= len(e.kept[oldest].infoData)
		delete(e.kept, oldest)
	}
}

// lookupAgain is how soon the DHT is looked up again while a torrent has
// no peer (see torrent.lookup).
func (e *Engine) lookupAgain() time.Duration {
	if e != nil && e.dhtAgainWait > 0 {
		return e.dhtAgainWait
	}
	return dhtAgain
}

// fullLocked reports whether the torrent of hash may not start, as many
// others run as limit allows, 0 for no limit. e.mu is held.
func (e *Engine) fullLocked(hash Hash, limit int) bool {
	if limit <= 0 {
		return false
	}
	others := len(e.torrents)
	if e.torrents[hash] != nil {
		others--
	}
	return others >= limit
}

// capped logs a torrent that did not start, as many ran as limit allows.
func (e *Engine) capped(limit int) {
	e.config.Logger.Info("A torrent did not start: as many run as Torrents at once allows", "limit", limit)
}

// Admits reports why the torrent of hash cannot start now: ErrAtCap when
// it does not run and as many others run as the settings allow; nil
// otherwise.
func (e *Engine) Admits(hash Hash) error {
	limit := e.config.Settings().AtOnce
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.torrents[hash] == nil && e.fullLocked(hash, limit) {
		return ErrAtCap
	}
	return nil
}

// AdmitsWithin is Admits, waiting at most wait, or until ctx is done, for
// torrents to stop and make room: as one asked to end does once its last
// read ends (see End).
func (e *Engine) AdmitsWithin(ctx context.Context, hash Hash, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	for {
		e.mu.Lock()
		removed := e.removed
		e.mu.Unlock()
		err := e.Admits(hash)
		if err == nil {
			return nil
		}
		select {
		case <-removed:
		case <-timer.C:
			return err
		case <-ctx.Done():
			return err
		}
	}
}

// carriesUDP tells whether a route carries UDP, which a SOCKS5 proxy does
// only with UDP ASSOCIATE: UDP trackers and the DHT stay off otherwise.
func (e *Engine) carriesUDP(ctx context.Context, route Route) bool {
	conn, err := route.ListenPacket(ctx)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.udpRoute, e.noUDP = route, errors.Is(err, ErrNoUDP)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// joined counts one playback more of t, which runs: its linger, or the
// end asked for it, is called off. t's lock is held.
func (t *torrent) joined() {
	t.refs++
	t.ending = false
	if t.linger != nil {
		t.linger.Stop()
		t.linger = nil
	}
}

// release ends a playback of t: once none is left, t stops after the
// linger, or at once when its end was asked for (see End).
func (e *Engine) release(t *torrent) {
	t.mu.Lock()
	t.refs--
	if t.refs > 0 || t.err != nil {
		t.mu.Unlock()
		return
	}
	if t.ending {
		t.mu.Unlock()
		e.stop(t)
		return
	}
	t.linger = time.AfterFunc(e.config.Linger, func() { e.stop(t) })
	t.mu.Unlock()
}

// End ends the linger of the torrent of hash, as a playback of another
// title took the place of the one that read it: with no playback reading
// it, it stops at once, as the linger would; otherwise it stops as the
// last of them ends, unless another joins it meanwhile.
func (e *Engine) End(hash Hash) {
	e.mu.Lock()
	t := e.torrents[hash]
	e.mu.Unlock()
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.err != nil || t.refs > 0 {
		t.ending = t.err == nil
		t.mu.Unlock()
		return
	}
	if t.linger != nil {
		t.linger.Stop()
		t.linger = nil
	}
	t.mu.Unlock()
	e.stop(t)
}

// stop stops a torrent no playback reads: no more peers, no more sharing.
func (e *Engine) stop(t *torrent) {
	t.mu.Lock()
	if t.refs > 0 {
		t.mu.Unlock()
		return
	}
	t.mu.Unlock()
	e.remove(t, errStopped)
}

// remove stops a torrent, telling its readers why, and removes its pieces.
// What it learned is kept for its next run (see keepLocked) as it leaves
// the engine: a run started once Running tells it stopped starts from it.
func (e *Engine) remove(t *torrent, err error) {
	e.mu.Lock()
	if e.torrents[t.hash] == t {
		delete(e.torrents, t.hash)
		close(e.removed)
		e.removed = make(chan struct{})
	}
	e.keepLocked(t)
	e.mu.Unlock()
	if e.left != nil {
		e.left(t.hash)
	}
	t.fail(err)
	e.store.drop(t.hash, t.run)
}

// Open returns a reader of the file of spec, once a playback joined its
// torrent and its peers told what files it holds, waiting at most until
// ctx is done or the config's StartTimeout passes, which is a
// *StartError. A torrent no playback joined is ErrNotPlaying; one whose
// route fails, why.
func (e *Engine) Open(ctx context.Context, spec Spec) (*Reader, error) {
	t, err := e.playing(ctx, spec.Hash)
	if err != nil {
		return nil, err
	}
	return t.open(ctx, spec)
}

// Files returns the files of the torrent of spec, as its info dictionary
// lists them, and the one spec plays (see Info.File), waiting for them as
// Open does.
func (e *Engine) Files(ctx context.Context, spec Spec) (played File, files []File, err error) {
	t, err := e.playing(ctx, spec.Hash)
	if err != nil {
		return File{}, nil, err
	}
	info, err := t.waitInfo(ctx)
	if err != nil {
		return File{}, nil, err
	}
	played, err = info.File(spec)
	return played, slices.Clone(info.Files), err
}

// ReadFile reads whole the file at index among the files of the torrent
// of spec, waiting for its info as Open does. Only that file's pieces are
// fetched: none ahead of it, nor the start and end Open's reads fetch for
// a video's index. A file longer than limit is not read.
func (e *Engine) ReadFile(ctx context.Context, spec Spec, index int, limit int64) ([]byte, error) {
	t, err := e.playing(ctx, spec.Hash)
	if err != nil {
		return nil, err
	}
	return t.readFile(ctx, index, limit)
}

// playing returns the torrent of hash once its route is checked: a
// torrent no playback joined is ErrNotPlaying; one whose route fails,
// why.
func (e *Engine) playing(ctx context.Context, hash Hash) (*torrent, error) {
	route, err := e.currentRoute()
	if err != nil {
		return nil, err
	}
	if err := route.Check(ctx); err != nil {
		return nil, err
	}
	if err := e.Admits(hash); err != nil {
		return nil, err
	}
	e.mu.Lock()
	t := e.torrents[hash]
	e.mu.Unlock()
	if t == nil {
		return nil, ErrNotPlaying
	}
	return t, nil
}

// StartTimeout is how long a torrent is waited for to start: its peers
// found and its info dictionary received (see Open).
func (e *Engine) StartTimeout() time.Duration {
	return e.config.StartTimeout
}

// Ready reports why torrents cannot play now: off, without a route, or
// over a route that cannot carry them; nil when they can.
func (e *Engine) Ready(ctx context.Context) error {
	route, err := e.currentRoute()
	if err != nil {
		return err
	}
	return route.Check(ctx)
}

// RouteFailure reports whether err is a route's failure, or torrents being
// off or without a route: the torrent's file may play once that changes,
// so it tells nothing of the file.
func RouteFailure(err error) bool {
	var route *RouteError
	return errors.As(err, &route) || errors.Is(err, ErrNoRoute) || errors.Is(err, ErrDisabled)
}

// Running reports whether the torrent of hash runs: a playback joined it,
// and it did not stop.
func (e *Engine) Running(hash Hash) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.torrents[hash] != nil
}

// Received returns when the torrent of hash last received a byte of its
// pieces from a peer, zero when it does not run or received none.
func (e *Engine) Received(hash Hash) time.Time {
	e.mu.Lock()
	t := e.torrents[hash]
	e.mu.Unlock()
	if t == nil {
		return time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var last time.Time
	for _, p := range t.peers {
		if p.heard.After(last) {
			last = p.heard
		}
		if at := time.Unix(0, p.received.Load()); at.After(last) {
			last = at
		}
	}
	return last
}

// Status is what Health tells of torrents.
type Status struct {
	Enabled bool
	// Route describes the route, empty without one; RouteError is why it
	// cannot carry torrents now, nil when it can; NoUDP tells a proxy
	// without UDP ASSOCIATE, over which UDP trackers, the DHT and uTP are
	// off.
	Route      string
	RouteError error
	NoUDP      bool
	// Tunnel is the WireGuard tunnel's state, nil over another route.
	Tunnel *TunnelStatus
	// Playing counts the torrents running, Peers their peers, and
	// UTPPeers those of them reached over uTP.
	Playing, Peers, UTPPeers int
	// AtOnce is the settings' bound on Playing, 0 for none.
	AtOnce int
	// Used is the bytes the pieces take on disk.
	Used int64
	// Failure is why a torrent last could not play, at FailedAt.
	Failure  error
	FailedAt time.Time
	// Unstarted is the last torrent that did not start in time, at
	// UnstartedAt; nil once another started since.
	Unstarted   *StartError
	UnstartedAt time.Time
}

// Status checks the route and tells what runs. A WireGuard tunnel tells
// its last check at once (see lastResult).
func (e *Engine) Status(ctx context.Context) Status {
	settings := e.config.Settings()
	status := Status{Enabled: settings.Enabled, AtOnce: settings.AtOnce, Used: e.store.usage()}
	if status.Enabled {
		route, err := e.currentRoute()
		if err == nil {
			status.Route = route.String()
			if tunnel, ok := route.(*wireGuardRoute); ok {
				err = tunnel.lastResult(ctx)
				state := tunnel.tunnel()
				status.Tunnel = &state
			} else {
				err = route.Check(ctx)
			}
		}
		status.RouteError = err
		e.mu.Lock()
		probed := e.udpRoute == route
		e.mu.Unlock()
		if err == nil && !probed {
			// Whether a proxy offers UDP ASSOCIATE is known once asked.
			e.carriesUDP(ctx, route)
		}
		e.mu.Lock()
		status.NoUDP = e.noUDP && e.udpRoute == route
		e.mu.Unlock()
	}
	e.mu.Lock()
	status.Failure, status.FailedAt = e.failure, e.failedAt
	status.Unstarted, status.UnstartedAt = e.unstarted, e.unstartedAt
	torrents := make([]*torrent, 0, len(e.torrents))
	for _, t := range e.torrents {
		torrents = append(torrents, t)
	}
	e.mu.Unlock()
	for _, t := range torrents {
		all, viaUTP := t.peerCount()
		status.Playing++
		status.Peers += all
		status.UTPPeers += viaUTP
	}
	return status
}

// Usage is the bytes the pieces take on disk.
func (e *Engine) Usage() int64 {
	return e.store.usage()
}

// Uploaded is the bytes shared since the engine started.
func (e *Engine) Uploaded() int64 {
	return e.sharedOut.Load()
}

func (e *Engine) countUpload(n int64) {
	e.sharedOut.Add(n)
}

// Close stops every torrent, and the route: a WireGuard tunnel stops.
func (e *Engine) Close() {
	e.mu.Lock()
	torrents := make([]*torrent, 0, len(e.torrents))
	for _, t := range e.torrents {
		torrents = append(torrents, t)
	}
	clear(e.torrents)
	route := e.route
	e.route, e.routeKey = nil, routeSettings{}
	e.mu.Unlock()
	// The torrents stop over their route still up, then the route closes.
	for _, t := range torrents {
		t.fail(errStopped)
	}
	if route != nil {
		route.Close()
	}
	e.store.close()
}
