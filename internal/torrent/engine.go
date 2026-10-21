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
	"sync"
	"sync/atomic"
	"time"
)

// ErrDisabled reports torrents played while the server does not play them.
var ErrDisabled = errors.New("torrent streams are turned off")

// ErrNotPlaying reports a read of a torrent no playback started: only a
// playback joins a torrent's swarm.
var ErrNotPlaying = errors.New("the torrent is read only for a playback")

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

	mu       sync.Mutex
	torrents map[Hash]*torrent
	// routeKey and route are the route of the settings last read.
	routeKey Settings
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
	e := &Engine{config: config, store: s, torrents: map[Hash]*torrent{}, checkEvery: checkEvery}
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
	key := Settings{Interface: settings.Interface, Proxy: settings.Proxy, ProxyUser: settings.ProxyUser, ProxyPassword: settings.ProxyPassword,
		WireGuard: settings.WireGuard}
	e.mu.Lock()
	if settings.Enabled && e.route != nil && e.routeKey == key {
		route := e.route
		e.mu.Unlock()
		return route, nil
	}
	old := e.route
	e.route, e.routeKey = nil, Settings{}
	route, err := Route(nil), ErrDisabled
	if settings.Enabled {
		route, err = RouteOf(key, e.config.Logger)
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
			t.refs++
			if t.linger != nil {
				t.linger.Stop()
				t.linger = nil
			}
			t.mu.Unlock()
			e.mu.Unlock()
			t.merge(spec)
			return t
		}
		t.mu.Unlock()
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
			old.refs++
			if old.linger != nil {
				old.linger.Stop()
				old.linger = nil
			}
		}
		old.mu.Unlock()
		if running {
			e.mu.Unlock()
			old.merge(spec)
			return old
		}
	}
	e.torrents[spec.Hash] = t
	e.mu.Unlock()
	t.start()
	return t
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

// release ends a playback of t: once none is left, t stops after the
// linger.
func (e *Engine) release(t *torrent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.refs--
	if t.refs > 0 || t.err != nil {
		return
	}
	t.linger = time.AfterFunc(e.config.Linger, func() { e.stop(t) })
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
func (e *Engine) remove(t *torrent, err error) {
	e.mu.Lock()
	if e.torrents[t.hash] == t {
		delete(e.torrents, t.hash)
	}
	e.mu.Unlock()
	t.fail(err)
	e.store.drop(t.hash)
}

// Open returns a reader of the file of spec, once a playback joined its
// torrent and its peers told what files it holds, waiting at most until
// ctx is done or the config's StartTimeout passes, which is a
// *StartError. A torrent no playback joined is ErrNotPlaying; one whose
// route fails, why.
func (e *Engine) Open(ctx context.Context, spec Spec) (*Reader, error) {
	route, err := e.currentRoute()
	if err != nil {
		return nil, err
	}
	if err := route.Check(ctx); err != nil {
		return nil, err
	}
	e.mu.Lock()
	t := e.torrents[spec.Hash]
	e.mu.Unlock()
	if t == nil {
		return nil, ErrNotPlaying
	}
	return t.open(ctx, spec.File)
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

// Status is what Health tells of torrents.
type Status struct {
	Enabled bool
	// Route describes the route, empty without one; RouteError is why it
	// cannot carry torrents now, nil when it can; NoUDP tells a proxy
	// without UDP ASSOCIATE, over which UDP trackers and the DHT are off.
	Route      string
	RouteError error
	NoUDP      bool
	// Tunnel is the WireGuard tunnel's state, nil over another route.
	Tunnel *TunnelStatus
	// Playing counts the torrents running, and Peers their peers.
	Playing, Peers int
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

// Status checks the route and tells what runs.
func (e *Engine) Status(ctx context.Context) Status {
	status := Status{Enabled: e.config.Settings().Enabled, Used: e.store.usage()}
	if status.Enabled {
		route, err := e.currentRoute()
		if err == nil {
			status.Route = route.String()
			err = route.Check(ctx)
			if tunnel, ok := route.(*wireGuardRoute); ok {
				state := tunnel.tunnel()
				status.Tunnel = &state
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
		status.Playing++
		status.Peers += t.peerCount()
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
	e.route, e.routeKey = nil, Settings{}
	e.mu.Unlock()
	if route != nil {
		route.Close()
	}
	for _, t := range torrents {
		t.fail(errStopped)
	}
	e.store.close()
}
