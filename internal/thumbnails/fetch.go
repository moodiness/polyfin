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
)

// Source is a version's file as thumbnails read it: *source.Source is one.
// It may also serve several ranges with one request, as a
// container.RangeFetcher.
type Source interface {
	container.Fetcher
	Size(ctx context.Context) (int64, error)
	Release()
}

// errTooManyRequests reports a version whose thumbnails would take its
// host more requests than a generation may make.
var errTooManyRequests = errors.New("making the images would take the source too many requests")

// pacer spaces the requests made to each host: providers answer bursts
// with 429, and slow the video they serve meanwhile.
type pacer struct {
	interval time.Duration

	mu   sync.Mutex
	next map[string]time.Time
}

func newPacer(interval time.Duration) *pacer {
	return &pacer{interval: interval, next: map[string]time.Time{}}
}

// wait waits for host's next turn.
func (p *pacer) wait(ctx context.Context, host string) error {
	p.mu.Lock()
	now := time.Now()
	at := now
	if next := p.next[host]; next.After(now) {
		at = next
	}
	p.next[host] = at.Add(p.interval)
	if len(p.next) > 1000 {
		for other, next := range p.next {
			if next.Before(now) {
				delete(p.next, other)
			}
		}
	}
	p.mu.Unlock()
	if wait := time.Until(at); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// A turn taken late pushes the next one back: requests never come
	// closer than the interval.
	p.mu.Lock()
	if next := time.Now().Add(p.interval); next.After(p.next[host]) {
		p.next[host] = next
	}
	p.mu.Unlock()
	return ctx.Err()
}

// paced reads a version's source for one generation: each request waits
// for its host's turn, and those past the generation's share fail.
type paced struct {
	src   Source
	host  string
	pacer *pacer
	left  int
	// requests counts the requests made.
	requests int
}

func (p *paced) take(ctx context.Context) error {
	if p.left <= 0 {
		return errTooManyRequests
	}
	p.left--
	p.requests++
	return p.pacer.wait(ctx, p.host)
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
	if err := p.take(ctx); err != nil {
		return nil, err
	}
	return p.src.Fetch(ctx, off, n)
}

func (p *paced) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	rf, ok := p.src.(container.RangeFetcher)
	if !ok {
		return nil, container.ErrMultiRangeUnsupported
	}
	if err := p.take(ctx); err != nil {
		return nil, err
	}
	return rf.FetchRanges(ctx, ranges)
}

func (p *paced) Size(ctx context.Context) (int64, error) {
	if err := p.take(ctx); err != nil {
		return 0, err
	}
	return p.src.Size(ctx)
}

// hostOf is the host a version's URL names.
func hostOf(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
