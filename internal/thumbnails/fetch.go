package thumbnails

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/container"
	"github.com/moodiness/polyfin/internal/source"
)

// Source is a version's file as thumbnails read it, with one attempt per
// request, never sent again: source.Once is one. It may also serve
// several ranges with one request, as a container.RangeFetcher.
type Source interface {
	container.Fetcher
	// Renew asks for a fresh link, after an answer telling the link
	// expired (source.ErrExpired).
	Renew(ctx context.Context) error
	// KnownSize is the file's size, once a request told it.
	KnownSize() (int64, bool)
	Release()
}

// errBudget reports a version whose images took every request allowed: the
// keyframes read so far are used.
var errBudget = errors.New("the requests allowed for the images of a version were all made")

// errHostPaused reports a host whose thumbnail work is paused, after it
// asked to slow down.
var errHostPaused = errors.New("the source's host asked to slow down: its images are paused")

// gate spaces the requests made to each host for images, bounds those of
// each rolling hour, and pauses a host that asked to slow down. Providers
// answer bursts with 429 and then refuse every file of the account for
// minutes, the playbacks they serve included.
type gate struct {
	// interval spaces two requests to a host; perHour bounds those of an
	// hour, window, to a host.
	interval time.Duration
	perHour  int
	window   time.Duration
	// poll is how often a host busy with a playback is checked again.
	poll time.Duration

	mu    sync.Mutex
	hosts map[string]*hostState
}

type hostState struct {
	next   time.Time
	recent []time.Time
	paused time.Time
}

func newGate(interval time.Duration, perHour int, window, poll time.Duration) *gate {
	return &gate{interval: interval, perHour: perHour, window: window, poll: poll, hosts: map[string]*hostState{}}
}

// state returns host's state, dropping the requests out of the window.
// The caller holds g.mu.
func (g *gate) state(host string, now time.Time) *hostState {
	h, ok := g.hosts[host]
	if !ok {
		h = &hostState{}
		g.hosts[host] = h
	}
	for len(h.recent) > 0 && now.Sub(h.recent[0]) >= g.window {
		h.recent = h.recent[1:]
	}
	return h
}

// ready reports whether a version of host may start: the host is not
// paused, and has room left in its hour.
func (g *gate) ready(host string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	h := g.state(host, now)
	return !now.Before(h.paused) && len(h.recent) < g.perHour
}

// paused reports whether host's images are paused.
func (g *gate) paused(host string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return time.Now().Before(g.state(host, time.Now()).paused)
}

// pause pauses host's images for d.
func (g *gate) pause(host string, d time.Duration) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state(host, time.Now()).paused = time.Now().Add(d)
}

// wait waits for host's next turn, past the interval since its last
// request and within its hour, and takes it. A paused host fails it.
func (g *gate) wait(ctx context.Context, host string) error {
	for {
		g.mu.Lock()
		now := time.Now()
		h := g.state(host, now)
		if now.Before(h.paused) {
			g.mu.Unlock()
			return errHostPaused
		}
		at := h.next
		if len(h.recent) >= g.perHour {
			at = later(at, h.recent[0].Add(g.window))
		}
		if !at.After(now) {
			h.recent = append(h.recent, now)
			h.next = now.Add(g.interval)
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		if err := sleep(ctx, at.Sub(now)); err != nil {
			return err
		}
	}
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// sleep waits for d, or until ctx is done.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// paced reads a version's source for one generation: each request waits
// while a playback reads the same host, then for the host's turn, and
// those past the generation's budget fail with errBudget.
type paced struct {
	src  Source
	host string
	gate *gate
	// busy reports whether a playback reads host.
	busy func(host string) bool
	left int
	// requests counts the requests made, and renewed is set once the
	// link was renewed: once a generation.
	requests int
	renewed  bool
}

func (p *paced) take(ctx context.Context) error {
	if p.left <= 0 {
		return errBudget
	}
	for p.busy != nil && p.busy(p.host) {
		if err := sleep(ctx, p.gate.poll); err != nil {
			return err
		}
	}
	if err := p.gate.wait(ctx, p.host); err != nil {
		return err
	}
	p.left--
	p.requests++
	return nil
}

// ReadAt reads the index, as the keyframes, with requests for the spans
// it needs alone: through the source's cache, each read would also fetch
// the megabytes after it, which playback reads ahead.
func (p *paced) ReadAt(ctx context.Context, b []byte, off int64) (int, error) {
	data, err := p.Fetch(ctx, off, len(b))
	n := copy(b, data)
	if err == nil && n < len(b) {
		err = io.EOF
	}
	return n, err
}

func (p *paced) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	var data []byte
	err := p.request(ctx, func() error {
		var err error
		data, err = p.src.Fetch(ctx, off, n)
		return err
	})
	return data, err
}

func (p *paced) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	rf, ok := p.src.(container.RangeFetcher)
	if !ok {
		return nil, container.ErrMultiRangeUnsupported
	}
	var data [][]byte
	err := p.request(ctx, func() error {
		var err error
		data, err = rf.FetchRanges(ctx, ranges)
		return err
	})
	return data, err
}

// request sends a request in the host's turn. A source answering oddly,
// as some do now and then, is asked once more after a pause, a request
// counted like the others: an answer ignoring the range, for another
// range, cut short, or none. One telling the link expired has it renewed
// first, once a generation. One asking to slow down or overloaded is
// never asked again.
func (p *paced) request(ctx context.Context, send func() error) error {
	if err := p.take(ctx); err != nil {
		return err
	}
	err := send()
	if !odd(err) || ctx.Err() != nil {
		return err
	}
	if errors.Is(err, source.ErrExpired) && !p.renewed {
		p.renewed = true
		if renewErr := p.src.Renew(ctx); renewErr != nil {
			return err
		}
	}
	if pauseErr := sleep(ctx, p.gate.interval); pauseErr != nil {
		return pauseErr
	}
	if takeErr := p.take(ctx); takeErr != nil {
		return err
	}
	return send()
}

// odd reports whether a request failed for an odd answer of the source,
// or none: not one asking to slow down, which is never asked again.
func odd(err error) bool {
	return (errors.Is(err, source.ErrUnavailable) || errors.Is(err, source.ErrRangesIgnored)) && !errors.Is(err, source.ErrSlowDown)
}

// hostOf is the host a version's URL names.
func hostOf(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
