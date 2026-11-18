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
	"unicode/utf8"
)

// Capacity is how many bytes of log lines a Ring keeps by default.
const Capacity = 5 << 20

const (
	// lineFraction is the part of a ring's capacity one line may take: a
	// thirty-second, 160 KiB of the default, which holds the longest
	// lines the server writes, an error with a stack among them, many
	// times over, while no single record displaces more than a
	// thirty-second of what the ring keeps.
	lineFraction = 32
	// minLine is the least a line may take in a ring too small for the
	// fraction to hold an ordinary line, up to the ring's capacity.
	minLine = 1 << 10
	// lookAhead is how far past the place a long line is cut the ring
	// reads before it redacts, so that a secret the cut would split is
	// still hidden whole.
	lookAhead = 4 << 10
	// cutMark ends a line that was cut.
	cutMark = "… (cut)"
)

// Ring is an io.Writer that keeps the last lines written, redacted, up to
// a number of bytes; older lines are dropped first.
type Ring struct {
	mu       sync.Mutex
	capacity int
	lines    [][]byte
	// head is the index of the oldest line in lines.
	head int
	size int
	// written counts the lines ever written, which numbers them.
	written  uint64
	created  time.Time
	modified time.Time
	now      func() time.Time
	// redact is Redact, which tests replace.
	redact func(string) string
}

// NewRing returns an empty ring keeping up to capacity bytes.
func NewRing(capacity int) *Ring {
	now := time.Now()
	return &Ring{capacity: capacity, created: now, modified: now, now: time.Now, redact: Redact}
}

// Write keeps p's lines, redacted, each cut to a fraction of the ring's
// capacity (see lineFraction). It never fails.
func (r *Ring) Write(p []byte) (int, error) {
	text := strings.TrimSuffix(string(p), "\n")
	// The lines are redacted before the lock is taken: readers and other
	// writers do not wait for it.
	limit := min(r.capacity, max(r.capacity/lineFraction, minLine))
	var lines [][]byte
	for line := range strings.SplitSeq(text, "\n") {
		lines = append(lines, r.bounded(line, limit))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, redacted := range lines {
		r.lines = append(r.lines, redacted)
		r.size += len(redacted)
		r.written++
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

// bounded returns line redacted, with its line end, in limit bytes at
// most: a longer one keeps its start and ends with cutMark, when limit
// has room for it.
func (r *Ring) bounded(line string, limit int) []byte {
	var redacted string
	cut := len(line) > limit
	if !cut {
		redacted = r.redact(line)
	} else {
		// A long line is cut before it is redacted, which bounds the
		// work. What remains of a secret the cut splits may no longer
		// look like one: the line's start is redacted twice, alone and
		// with lookAhead bytes more, and only what both give is kept.
		alone, further := r.redact(line[:limit]), r.redact(line[:min(len(line), limit+lookAhead)])
		same := 0
		for same < len(alone) && same < len(further) && alone[same] == further[same] {
			same++
		}
		redacted = alone[:same]
	}
	if !cut && len(redacted) < limit {
		return []byte(redacted + "\n")
	}
	keep, mark := limit-1-len(cutMark), cutMark
	if keep < 0 {
		keep, mark = limit-1, ""
	}
	keep = min(keep, len(redacted))
	// A character the cut split is dropped.
	for range utf8.UTFMax - 1 {
		if last, size := utf8.DecodeLastRuneInString(redacted[:keep]); last != utf8.RuneError || size != 1 {
			break
		}
		keep--
	}
	return []byte(redacted[:keep] + mark + "\n")
}

// Contents returns the lines kept, oldest first.
func (r *Ring) Contents() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bytes.Join(r.lines[r.head:], nil)
}

// Since returns the lines kept among those written after the first after
// lines, oldest first and without their line ends, keeping the newest when
// there are more than limit, with the number of lines written so far: the
// after of the next call, which then returns only newer lines. An after
// past that number, from before the server started again, counts from 0.
func (r *Ring) Since(after uint64, limit int) (lines []string, next uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.lines[r.head:]
	if after > r.written {
		after = 0
	}
	first := uint64(0)
	if r.written > uint64(len(kept)) {
		first = r.written - uint64(len(kept))
	}
	from := 0
	if after > first {
		from = int(after - first)
	}
	if limit > 0 && len(kept)-from > limit {
		from = len(kept) - limit
	}
	lines = make([]string, 0, len(kept)-from)
	for _, line := range kept[from:] {
		lines = append(lines, string(line[:len(line)-1]))
	}
	return lines, r.written
}

// Stat tells when the ring was made and last written, and the bytes it
// keeps.
func (r *Ring) Stat() (created, modified time.Time, size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created, r.modified, r.size
}

// The pieces of the patterns below.
const (
	// inAddress is a character of an address: a blank, a quote, an angle
	// bracket or a backslash ends one, as the text log escapes quotes.
	inAddress = `[^\s"'<>\x60\\]`
	// hostName is a host of several labels, or an IPv4 address.
	hostName = `(?:[a-z0-9\-]+\.)+[a-z0-9\-]+`
	// userInfo is the user and password some addresses start with.
	userInfo = `(?:[^\s"'<>\x60\\/@=:]+(?::[^\s"'<>\x60\\/@]*)?@)?`
	// quoted is a value in double quotes, whatever it holds: as it is
	// written, with its escapes; as the text log writes it inside a quoted
	// attribute, where the quotes that end it follow one backslash, or
	// five, and those it holds follow three, or seven; and, quoted more
	// times than that, up to the next quote.
	quoted = `(?:"(?:[^"\\]|\\.)*"|\\"(?:[^"\\]|\\+[^"\\]|(?:\\{4})*\\{3}")*(?:\\{4})*\\"|\\{2,}"[^"]*")`
	// separator is what stands between a name and its value, as in
	// name=value, name: value, "name":"value" and name%3Dvalue.
	separator = `\\*["']?\s*(?:[=:]|%3d)\s*`
	// value is what a name is given: the token of an Authorization header
	// with its scheme, a quoted value, one whose quotes never close up to
	// the next blank, or everything up to a blank or the next parameter.
	value = `(?:\[?(?:bearer|basic)\s+)?(?:` + quoted + `|\\*"[^\s"]*|'[^']*'|[^\s&,;"]+)`
	// cookie is one name=value of a Cookie or Set-Cookie header.
	cookie = `[^\s;"\\=]+(?:=(?:` + quoted + `|[^\s;"\\]*))?`
)

var (
	// addresses matches, each kind in a group of its own: a URL of any
	// scheme, whose query runs up to a blank, a double quote or a
	// backslash; a URL written inside another's query, its :// encoded;
	// host:port/path without a scheme; and //host/path where a value
	// starts.
	addresses = regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+.\-]*://(?:[^\s"'<>\x60\\?#]+(?:[?#][^\s"\\]*)?|[?#][^\s"\\]*))` +
		`|(\b[a-z][a-z0-9+.\-]*%3a%2f%2f[^\s"'<>\x60\\&]+)` +
		`|(\b` + userInfo + hostName + `:[0-9]{2,5}/` + inAddress + `+)` +
		`|(?:^|[\s="'(\[,])(//` + userInfo + hostName + `(?::[0-9]{1,5})?/` + inAddress + `+)`)
	// secrets matches, each kind in a group of its own, the values that
	// carry credentials: those of parameters, headers and attributes
	// whose name ends in token, secret, key, password or passphrase,
	// whatever starts it, and pass in a query or in quotes, as an
	// attribute of the log tells a pass by that name; those of the short
	// names for one, as whole words; the value of code; the cookies of a
	// Cookie header; and a token after Bearer, which has a digit, as no
	// word has.
	secrets = regexp.MustCompile(`(?i)(?:token|secret|key|pass(?:word|wd|phrase)|["'?&]pass)` + separator + `(` + value + `)` +
		`|\b(?:(?:pw|sig|signature|auth|authorization)` + separator + `(` + value + `)` +
		`|code` + separator + `(` + value + `)` +
		`|cookie` + separator + `(` + cookie + `(?:;\s*` + cookie + `)*)` +
		`|bearer\s+([a-z0-9._~+/\-]*[0-9][a-z0-9._~+/\-]*=*))`)
)

// Redact hides what a log line could carry that must not be read: URLs
// keep only their scheme and host, as Polyfin shows the addresses of
// addons, and the values of tokens, keys, passwords and cookies are
// replaced. Two values stay, which administrators read in the log: the
// PIN of a password reset and the setup code, logged as pin and
// setup_code. Both patterns are matched in a time that grows with the
// line's length alone, whatever the line holds.
func Redact(line string) string {
	// Every address has a slash, as it is or encoded.
	if strings.ContainsAny(line, "/%") {
		line = rewrite(addresses, line, func(group int, address string) string {
			switch group {
			case 2:
				decoded, err := url.PathUnescape(address)
				if err != nil {
					return "…"
				}
				return origin(decoded)
			case 3:
				return strings.TrimPrefix(origin("//"+address), "//")
			}
			return origin(address)
		})
	}
	return rewrite(secrets, line, func(group int, value string) string {
		// A status or an exit code is no secret: code=404 stays.
		if group == 3 && len(value) <= 3 && strings.Trim(value, "0123456789") == "" {
			return value
		}
		return "<redacted>"
	})
}

// origin returns address with its scheme and host alone.
func origin(address string) string {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return "…"
	}
	host := "//" + parsed.Host
	if parsed.Scheme != "" {
		host = parsed.Scheme + ":" + host
	}
	if parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" {
		return host
	}
	return host + "/…"
}

// rewrite returns line with, of each match of pattern, the group that
// matched replaced by what replace returns for it.
func rewrite(pattern *regexp.Regexp, line string, replace func(group int, text string) string) string {
	matches := pattern.FindAllStringSubmatchIndex(line, -1)
	if matches == nil {
		return line
	}
	var rewritten strings.Builder
	rewritten.Grow(len(line))
	last := 0
	for _, match := range matches {
		for group := 1; 2*group < len(match); group++ {
			if start, end := match[2*group], match[2*group+1]; start >= 0 {
				rewritten.WriteString(line[last:start])
				rewritten.WriteString(replace(group, line[start:end]))
				last = end
				break
			}
		}
	}
	rewritten.WriteString(line[last:])
	return rewritten.String()
}
