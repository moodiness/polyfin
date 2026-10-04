// Package logs keeps the most recent lines of the server log in memory, so
// that administrators can read them from Jellyfin apps, with the addresses
// and secrets they could carry redacted.
package logs

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Capacity is how many bytes of log lines a Ring keeps by default.
const Capacity = 5 << 20

// Ring is an io.Writer that keeps the last lines written, redacted, up to
// a number of bytes; older lines are dropped first.
type Ring struct {
	mu       sync.Mutex
	capacity int
	lines    [][]byte
	// head is the index of the oldest line in lines.
	head     int
	size     int
	created  time.Time
	modified time.Time
	now      func() time.Time
}

// NewRing returns an empty ring keeping up to capacity bytes.
func NewRing(capacity int) *Ring {
	now := time.Now()
	return &Ring{capacity: capacity, created: now, modified: now, now: time.Now}
}

// Write keeps p's lines, redacted. It never fails.
func (r *Ring) Write(p []byte) (int, error) {
	text := strings.TrimSuffix(string(p), "\n")
	r.mu.Lock()
	defer r.mu.Unlock()
	for line := range strings.SplitSeq(text, "\n") {
		redacted := []byte(Redact(line) + "\n")
		if len(redacted) > r.capacity {
			redacted = append(redacted[:r.capacity-1:r.capacity-1], '\n')
		}
		r.lines = append(r.lines, redacted)
		r.size += len(redacted)
		for r.size > r.capacity {
			r.size -= len(r.lines[r.head])
			r.lines[r.head] = nil
			r.head++
		}
		// The dropped lines' slots are reclaimed once they are half of
		// the slice.
		if r.head > len(r.lines)/2 {
			r.lines = append(r.lines[:0:0], r.lines[r.head:]...)
			r.head = 0
		}
	}
	r.modified = r.now()
	return len(p), nil
}

// Contents returns the lines kept, oldest first.
func (r *Ring) Contents() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bytes.Join(r.lines[r.head:], nil)
}

// Stat tells when the ring was made and last written, and the bytes it
// keeps.
func (r *Ring) Stat() (created, modified time.Time, size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created, r.modified, r.size
}

var (
	// addresses matches URLs of any scheme, up to a blank, a quote or a
	// backslash, as the text log escapes quotes.
	addresses = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.\-]*://[^\s"'<>\x60\\]+`)
	// secrets matches the values of parameters, headers and attributes
	// that carry credentials, as key=value, key: value or "key":"value".
	secrets = regexp.MustCompile(`(?i)\b(api_?key|access_?token|token|x-emby-token|x-mediabrowser-token|password|pw|secret|authorization)("?\s*[=:]\s*)("[^"]*"|[^\s&,;"]+)`)
)

// Redact hides what a log line could carry that must not be read: URLs
// keep only their scheme and host, as Polyfin shows the addresses of
// addons, and the values of tokens, keys and passwords are replaced.
func Redact(line string) string {
	line = addresses.ReplaceAllStringFunc(line, func(address string) string {
		parsed, err := url.Parse(address)
		if err != nil || parsed.Host == "" {
			return "…"
		}
		if parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" {
			return parsed.Scheme + "://" + parsed.Host
		}
		return parsed.Scheme + "://" + parsed.Host + "/…"
	})
	return secrets.ReplaceAllString(line, "$1$2<redacted>")
}
