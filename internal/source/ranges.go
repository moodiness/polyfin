package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/container"
)

// Fetch reads the n bytes at off with one request for that range alone,
// past the block cache: for small reads scattered over a file, such as its
// subtitle blocks, which the cache's 1 MiB blocks would multiply. It uses
// the source's location and renews it when it expired, as reads do, spaces
// its requests with those of others to the source, and waits out a server
// asking it to slow down, failing (429, 502, 503 or 504) or too slow to
// answer, a bounded number of times. Fewer than n bytes only at the end of
// the file.
func (s *Source) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	return s.fetchRange(ctx, off, n, false)
}

// Once is the source read with one attempt per request: Fetch and
// FetchRanges as the source's, but never sent again, nor after renewing
// an expired link. A source asking to slow down or overloaded fails them
// with ErrSlowDown at once. It is for background reads, which must not add
// to a burst a host refuses, nor make it refuse the playbacks it serves.
func (s *Source) Once() Once {
	return Once{s}
}

// Once reads a source with one attempt per request; see Source.Once.
type Once struct{ s *Source }

// Fetch is Source.Fetch with one attempt.
func (o Once) Fetch(ctx context.Context, off int64, n int) ([]byte, error) {
	return o.s.fetchRange(ctx, off, n, true)
}

// FetchRanges is Source.FetchRanges with one attempt.
func (o Once) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	return o.s.fetchRanges(ctx, ranges, true)
}

// Renew asks for a fresh link to the source, as an expired one is renewed
// for playback; the next requests use it. ErrExpired when the source has
// no way to renew it.
func (o Once) Renew(ctx context.Context) error {
	return o.s.renewLink(ctx)
}

// KnownSize is the source's size, when a request told it.
func (o Once) KnownSize() (int64, bool) {
	return o.s.knownSize()
}

// Release releases the source.
func (o Once) Release() {
	o.s.Release()
}

// fetchRange is Fetch, with one attempt when once is set.
func (s *Source) fetchRange(ctx context.Context, off int64, n int, once bool) ([]byte, error) {
	if off < 0 || n < 0 {
		return nil, fmt.Errorf("fetching %d bytes at %d: invalid range", n, off)
	}
	if size, known := s.knownSize(); known && off >= size {
		return nil, io.EOF
	}
	if n == 0 {
		return nil, nil
	}
	ranges := "bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(n)-1, 10)
	var data []byte
	err := s.exchange(ctx, ranges, int64(n), once, func(response *http.Response) error {
		var err error
		switch response.StatusCode {
		case http.StatusPartialContent:
			data, err = s.fetched(response, off, n)
			return err
		case http.StatusOK:
			// The body starts at the first byte, and may hold the whole
			// file: it is not read.
			return fmt.Errorf("fetching %d bytes at %d: %w", n, off, ErrRangesIgnored)
		default:
			return s.unsatisfiable(response, off)
		}
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// errBroken reports a connection that broke while a body was read: the
// request may be tried again.
var errBroken = errors.New("the connection broke")

// fetched reads the body of a response to Fetch's request for the n bytes
// at off: those of them the file holds.
func (s *Source) fetched(response *http.Response, off int64, n int) ([]byte, error) {
	start, _, total, ok := contentRange(response.Header.Get("Content-Range"))
	if !ok || start != off {
		return nil, fmt.Errorf("%w: %w %q", ErrUnavailable, errWrongRange, response.Header.Get("Content-Range"))
	}
	if err := s.learn(total, response.Header.Get("Content-Type")); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if total >= 0 && off+int64(n) > total {
		n = int(max(0, total-off))
	}
	data := make([]byte, n)
	if _, err := io.ReadFull(response.Body, data); err != nil {
		return nil, fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
	}
	return data, nil
}

// FetchRanges reads several ranges of the source with one request, past
// the block cache, as Fetch reads one: result[i] holds ranges[i], shorter
// only at the end of the file. Servers answer with a multipart/byteranges
// body, whose parts they may merge and reorder, or with one part covering
// every range. A source answering otherwise, as with the whole file, is
// not read, and is remembered: then and since, ErrMultiRangeUnsupported.
func (s *Source) FetchRanges(ctx context.Context, ranges []container.Range) ([][]byte, error) {
	return s.fetchRanges(ctx, ranges, false)
}

// fetchRanges is FetchRanges, with one attempt when once is set.
func (s *Source) fetchRanges(ctx context.Context, ranges []container.Range, once bool) ([][]byte, error) {
	result := make([][]byte, len(ranges))
	size, known := s.knownSize()
	var asked []int
	var length int64
	for i, r := range ranges {
		if r.Off < 0 || r.N < 0 {
			return nil, fmt.Errorf("fetching %d bytes at %d: invalid range", r.N, r.Off)
		}
		if r.N > 0 && (!known || r.Off < size) {
			asked = append(asked, i)
			length += int64(r.N)
		}
	}
	switch {
	case len(asked) == 0:
		return result, nil
	case len(asked) == 1:
		data, err := s.fetchRange(ctx, ranges[asked[0]].Off, ranges[asked[0]].N, once)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		result[asked[0]] = data
		return result, nil
	case s.multiRangeUnsupported():
		return nil, container.ErrMultiRangeUnsupported
	}
	var header strings.Builder
	header.WriteString("bytes=")
	for j, i := range asked {
		if j > 0 {
			header.WriteByte(',')
		}
		r := ranges[i]
		header.WriteString(strconv.FormatInt(r.Off, 10) + "-" + strconv.FormatInt(r.Off+int64(r.N)-1, 10))
	}
	err := s.exchange(ctx, header.String(), length, once, func(response *http.Response) error {
		// Merged parts may hold the bytes between the ranges too: a part
		// much larger than the ranges asked is not read.
		clear(result)
		parts := &multiRange{source: s, ranges: ranges, asked: asked, result: result, budget: 2*length + 1<<20}
		return parts.read(response)
	})
	switch {
	case err == nil:
		s.mu.Lock()
		s.multiRanges = true
		s.mu.Unlock()
		return result, nil
	case errors.Is(err, container.ErrMultiRangeUnsupported):
		s.mu.Lock()
		s.singleRanges = true
		s.mu.Unlock()
		s.cache.logger.Debug("A source does not serve several ranges at once", "source", s.id)
	}
	return nil, err
}

// ServesRanges reports whether the source serves several ranges with one
// request, and whether that is known: from a request for several that it
// answered, either way.
func (s *Source) ServesRanges() (served, known bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.multiRanges, s.multiRanges || s.singleRanges
}

// KnownSize is the source's size, when an answer told it.
func (s *Source) KnownSize() (int64, bool) {
	return s.knownSize()
}

func (s *Source) multiRangeUnsupported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.singleRanges
}

// multiRange is the answer to a request for several ranges being read.
type multiRange struct {
	source *Source
	ranges []container.Range
	// asked are the indexes of the ranges asked, and result where each
	// range read goes.
	asked  []int
	result [][]byte
	// budget is the bytes parts may still take.
	budget int64
}

// read reads the response to a request for several ranges into the
// results, the body only when it is made of them.
func (m *multiRange) read(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusOK:
		if err := m.source.learn(response.ContentLength, response.Header.Get("Content-Type")); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		return fmt.Errorf("%w: HTTP 200", container.ErrMultiRangeUnsupported)
	case http.StatusRequestedRangeNotSatisfiable:
		return m.source.unsatisfiable(response, m.ranges[m.asked[0]].Off)
	}
	mediaType, params, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	switch {
	case err == nil && mediaType == "multipart/byteranges" && params["boundary"] != "":
		parts := multipart.NewReader(response.Body, params["boundary"])
		for {
			part, err := parts.NextRawPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
			}
			if err := m.part(part.Header.Get("Content-Range"), part, false); err != nil {
				return err
			}
		}
	case err == nil && strings.HasPrefix(mediaType, "multipart/"):
		return fmt.Errorf("%w: a body of type %s", container.ErrMultiRangeUnsupported, mediaType)
	default:
		// One part: the server merged the ranges asked.
		if err := m.part(response.Header.Get("Content-Range"), response.Body, true); err != nil {
			return err
		}
	}
	size, known := m.source.knownSize()
	for _, i := range m.asked {
		switch {
		case m.result[i] != nil:
		case known && m.ranges[i].Off >= size:
			// The range starts past the end, which the source told.
			m.result[i] = []byte{}
		default:
			return fmt.Errorf("%w: the range %d-%d is missing", container.ErrMultiRangeUnsupported,
				m.ranges[i].Off, m.ranges[i].Off+int64(m.ranges[i].N)-1)
		}
	}
	return nil
}

// part reads a part of the answer, the bytes value, its Content-Range,
// tells, into the results of the ranges it covers whole. A part larger
// than the budget, or alone not covering them all, is not read; one of a
// multipart body covering none is skipped.
func (m *multiRange) part(value string, body io.Reader, alone bool) error {
	start, end, total, ok := contentRange(value)
	if !ok || end < start {
		return fmt.Errorf("%w: a part of range %q", container.ErrMultiRangeUnsupported, value)
	}
	if err := m.source.learn(total, ""); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	// Ranges past the end of the file, which the part tells, have no part.
	var covered []int
	past := 0
	for _, i := range m.asked {
		r := m.ranges[i]
		n := int64(r.N)
		if total >= 0 {
			n = min(n, total-r.Off)
		}
		switch {
		case n <= 0:
			past++
		case r.Off >= start && r.Off+n-1 <= end:
			covered = append(covered, i)
		}
	}
	length := end - start + 1
	if alone && len(covered)+past != len(m.asked) || length > m.budget {
		return fmt.Errorf("%w: a part of range %q", container.ErrMultiRangeUnsupported, value)
	}
	m.budget -= length
	if len(covered) == 0 {
		return nil
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(body, data); err != nil {
		return fmt.Errorf("%w: %w: %v", ErrUnavailable, errBroken, err)
	}
	for _, i := range covered {
		r := m.ranges[i]
		m.result[i] = data[r.Off-start:][:min(int64(r.N), end+1-r.Off)]
	}
	return nil
}

// exchange sends the request of Fetch or FetchRanges for ranges, the value
// of a Range header asking length bytes, and has read read its answer: a
// 200, a 206 or a 416, closed once read. The request goes where the
// source's reads go (see Source.target), renewed once when it expired,
// spaced from the others. It is sent again, a bounded number of times,
// when the source asks to slow down or fails, when it breaks, or when it
// takes longer than an attempt may: a host that stalls must not hold a
// read for minutes. With once, it is sent once, and a source asking to
// slow down or overloaded fails it with ErrSlowDown. Each request holds a
// slot of its host (see slots.go): read once, it is background work, which
// waits for one.
func (s *Source) exchange(ctx context.Context, ranges string, length int64, once bool, read func(*http.Response) error) error {
	timeout := s.cache.attemptTime + time.Duration(float64(length)/attemptRate*float64(time.Second))
	renewed := once
	tries := attempts
	if once {
		tries = 1
	}
	var held *slot
	defer func() { s.cache.release(held) }()
	cl := claim{state: func() (bool, bool) { return !once, ctx.Err() != nil }, done: ctx.Done(), firm: true}
	for attempt := 1; ; attempt++ {
		s.cache.release(held)
		held = nil
		if err := s.pace(ctx); err != nil {
			return err
		}
		to := s.target()
		if held = s.cache.acquire(s, to.host, cl); held == nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("%w: the source was closed", ErrUnavailable)
		}
		header := to.header
		header.Set("Range", ranges)
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		response, err := s.cache.opener.Open(attemptCtx, http.MethodGet, to.url, header, to.confined)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.unpin(to)
			if attempt >= tries {
				return fmt.Errorf("%w: %v", ErrUnavailable, withoutURL(err))
			}
			s.holdOff(backoff(attempt, ""))
			continue
		}
		s.cache.rehome(held, answerHost(to, response))
		if status := response.StatusCode; status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			// The host's connections are fewer for a while.
			s.slowedDown(answerHost(to, response))
			s.cache.press(held)
		}
		retryAfter := ""
		switch status := response.StatusCode; {
		case status == http.StatusOK || status == http.StatusPartialContent || status == http.StatusRequestedRangeNotSatisfiable:
			err := read(response)
			response.Body.Close()
			cancel()
			switch {
			case errors.Is(err, errOtherFile):
				s.unpin(to)
			case status != http.StatusRequestedRangeNotSatisfiable && err == nil:
				// The file's own answer: where it came from is kept.
				s.answered(to, response)
			}
			switch {
			case !errors.Is(err, errBroken):
				return err
			case ctx.Err() != nil:
				return ctx.Err()
			case attempt >= tries:
				return err
			}
		case expiredStatus(status) && to.pinned != nil:
			// The address the origin redirected to expired: the origin
			// tells where the file is now, before a renewal is asked.
			response.Body.Close()
			cancel()
			s.unpin(to)
			if once {
				return &StatusError{Status: status, Kind: ErrExpired}
			}
			continue
		case expiredStatus(status) && to.renew != nil && !renewed:
			response.Body.Close()
			cancel()
			renewed = true
			if err := s.renewLink(ctx); err != nil {
				return fmt.Errorf("%w: HTTP %d, and the link could not be renewed: %v", ErrUnavailable, status, err)
			}
			attempt = 0
			continue
		case once && expiredStatus(status) && to.renew != nil:
			response.Body.Close()
			cancel()
			return &StatusError{Status: status, Kind: ErrExpired}
		case slowDown(status) && attempt < tries:
			response.Body.Close()
			cancel()
			retryAfter = response.Header.Get("Retry-After")
		case slowDown(status):
			response.Body.Close()
			cancel()
			return &StatusError{Status: status, Kind: ErrSlowDown}
		default:
			response.Body.Close()
			cancel()
			return &StatusError{Status: status, Kind: ErrUnavailable}
		}
		s.holdOff(backoff(attempt, retryAfter))
	}
}

// slowDown reports whether a status asks to slow down, or tells an
// overloaded server.
func slowDown(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

// expiredStatus reports whether a status tells that a link expired.
func expiredStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound || status == http.StatusGone
}

// renewLink asks for a fresh link to the source, which the next requests
// use. ErrExpired when the source has no way to renew it.
func (s *Source) renewLink(ctx context.Context) error {
	s.mu.Lock()
	renew := s.renew
	s.mu.Unlock()
	if renew == nil {
		return ErrExpired
	}
	fresh, err := renew(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.origin, s.pinned = fresh, nil
	s.mu.Unlock()
	s.cache.logger.Debug("A source link was renewed", "source", s.id)
	return nil
}

// unsatisfiable reads a 416 answer to a request for the range at off,
// and closes it: io.EOF when it tells the file ends before off, which it
// records, errOtherFile when the size it tells is not the file's, or
// when it refuses a range the file holds.
func (s *Source) unsatisfiable(response *http.Response, off int64) error {
	response.Body.Close()
	if _, _, total, ok := contentRange(response.Header.Get("Content-Range")); ok && total >= 0 {
		if err := s.learn(total, ""); err != nil {
			return fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		if off < total {
			return fmt.Errorf("%w: %w: HTTP 416 at %d of %d bytes", ErrUnavailable, errOtherFile, off, total)
		}
		return io.EOF
	}
	if size, known := s.knownSize(); known && off < size {
		return fmt.Errorf("%w: %w: HTTP 416 at %d of %d bytes", ErrUnavailable, errOtherFile, off, size)
	}
	return io.EOF
}

// pace waits for the next time the source may be sent a request of Fetch
// or FetchRanges, and takes it.
func (s *Source) pace(ctx context.Context) error {
	s.mu.Lock()
	now := time.Now()
	at := s.nextFetch
	if at.Before(now) {
		at = now
	}
	s.nextFetch = at.Add(s.cache.fetchInterval)
	s.mu.Unlock()
	return pause(ctx, at.Sub(now))
}

// holdOff sends the source no request of Fetch or FetchRanges for d: a
// host asking to slow down is asked nothing meanwhile, by any of them.
func (s *Source) holdOff(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at := time.Now().Add(d); at.After(s.nextFetch) {
		s.nextFetch = at
	}
}

// pause waits for d, or until ctx is done.
func pause(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
