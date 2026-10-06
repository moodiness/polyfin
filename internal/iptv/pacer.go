package iptv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/stremio"
)

// RequestGap is the least time between two requests to one provider host
// for its lists, guides and details: providers refuse, then ban, accounts
// that send bursts. Streams are not paced.
const RequestGap = time.Second

// maxRetryAfter bounds how long a request waits for a provider that asked
// to slow down before it is tried again; longer, it fails as rate limited.
const maxRetryAfter = 30 * time.Second

// ErrRateLimited reports a provider that asked to slow down: HTTP 429, or
// 503 with Retry-After.
var ErrRateLimited = errors.New("the IPTV server asked to slow down")

// RateLimitError is ErrRateLimited with the delay the provider asked for,
// zero when it gave none.
type RateLimitError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%v: HTTP %d, retry after %s", ErrRateLimited, e.Status, e.RetryAfter)
}

func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }

// Pacer spaces the requests to each host: one every gap, later when a host
// asked to wait (Retry-After).
type Pacer struct {
	gap      time.Duration
	mu       sync.Mutex
	next     map[string]time.Time
	requests atomic.Int64
}

// NewPacer returns a pacer letting one request a host through every gap.
func NewPacer(gap time.Duration) *Pacer {
	return &Pacer{gap: gap, next: map[string]time.Time{}}
}

// Hosts paces the requests of the whole server to IPTV providers: the
// lists and details the IPTV service asks, and the guides of IPTV sources
// the library downloads.
var Hosts = NewPacer(RequestGap)

// hostOf is the host a request goes to, its port included.
func hostOf(address string) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

// Wait waits for the turn of address's host, and takes it.
func (p *Pacer) Wait(ctx context.Context, address string) error {
	host := hostOf(address)
	p.mu.Lock()
	now := time.Now()
	turn := now
	if next := p.next[host]; next.After(now) {
		turn = next
	}
	p.next[host] = turn.Add(p.gap)
	p.mu.Unlock()
	p.requests.Add(1)
	if wait := turn.Sub(now); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Delay moves the next turn of address's host at least d from now, as a
// Retry-After asks.
func (p *Pacer) Delay(address string, d time.Duration) {
	host := hostOf(address)
	p.mu.Lock()
	defer p.mu.Unlock()
	if at := time.Now().Add(d); at.After(p.next[host]) {
		p.next[host] = at
	}
}

// Requests counts the turns taken.
func (p *Pacer) Requests() int64 { return p.requests.Load() }

// RateLimited reads a refusal to serve now: 429, or 503 with Retry-After
// (seconds or an HTTP date). It returns nil for any other answer.
func RateLimited(response *http.Response) *RateLimitError {
	header := strings.TrimSpace(response.Header.Get("Retry-After"))
	if response.StatusCode != http.StatusTooManyRequests && (response.StatusCode != http.StatusServiceUnavailable || header == "") {
		return nil
	}
	e := &RateLimitError{Status: response.StatusCode}
	if seconds, err := strconv.Atoi(header); err == nil && seconds > 0 {
		e.RetryAfter = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(header); err == nil {
		e.RetryAfter = max(time.Until(at), 0)
	}
	return e
}

// requester asks providers for lists and details through the client,
// paced per host.
type requester struct {
	client *stremio.Client
	pacer  *Pacer
}

func (s *Service) requester() requester { return requester{client: s.client, pacer: s.pacer} }
