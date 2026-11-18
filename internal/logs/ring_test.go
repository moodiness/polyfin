package logs

import (
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"
)

func TestALongLineIsCutAndKeepsTheLinesBeforeIt(t *testing.T) {
	t.Parallel()
	// The default ring bounds a line to 160 KiB.
	byDefault := NewRing(Capacity)
	_, _ = byDefault.Write([]byte(strings.Repeat("x", 1<<20) + "\n"))
	if _, _, size := byDefault.Stat(); size != 160<<10 || !strings.HasSuffix(string(byDefault.Contents()), "x"+cutMark+"\n") {
		t.Errorf("the default ring keeps %d bytes of a long line", size)
	}
	// A smaller ring, for the many long lines written here to be quick.
	const capacity, limit = 8 * minLine * lineFraction, 8 * minLine
	ring := NewRing(capacity)
	// A line that fills the bound, its line end counted, is kept whole.
	whole := strings.Repeat("w", limit-1)
	_, _ = ring.Write([]byte("first line\n" + whole + "\n"))
	_, _ = ring.Write([]byte("start of the long line " + strings.Repeat("x", 1<<20) + "\n"))
	_, _ = ring.Write([]byte("last line\n"))
	lines, written := ring.Since(0, 0)
	if len(lines) != 4 || written != 4 || lines[0] != "first line" || lines[1] != whole || lines[3] != "last line" {
		t.Fatalf("%d lines of %d written: %.40q", len(lines), written, lines)
	}
	long := lines[2]
	if !strings.HasPrefix(long, "start of the long line xxx") || !strings.HasSuffix(long, "x"+cutMark) || len(long)+1 != limit {
		t.Errorf("the long line is %d bytes for a bound of %d: %.40q … %q", len(long)+1, limit, long, long[len(long)-20:])
	}
	if _, _, size := ring.Stat(); size != len("first line\n")+2*limit+len("last line\n") {
		t.Errorf("the ring keeps %d bytes", size)
	}
	// Many long lines push out no more than their bound each.
	for range lineFraction / 2 {
		_, _ = ring.Write([]byte(strings.Repeat("y", 1<<20)))
	}
	if lines, _ := ring.Since(0, 0); lines[0] != "first line" {
		t.Errorf("the first line was dropped: %.40q", lines[0])
	}
}

func TestALineThatGrowsPastTheBoundWhenRedactedIsCut(t *testing.T) {
	t.Parallel()
	const capacity, limit = 4 * minLine * lineFraction, 4 * minLine
	ring := NewRing(capacity)
	// Under the bound as written, over it once each value is replaced.
	line := strings.Repeat("pw=1 ", limit/5-1)
	_, _ = ring.Write([]byte(line + "\n"))
	lines, _ := ring.Since(0, 0)
	if len(lines) != 1 || len(lines[0])+1 > limit || !strings.HasPrefix(lines[0], "pw=<redacted> pw=<redacted> ") || !strings.HasSuffix(lines[0], cutMark) {
		t.Fatalf("%d lines, the first of %d bytes: %.40q", len(lines), len(lines[0]), lines[0])
	}
	if strings.Contains(lines[0], "pw=1") {
		t.Errorf("a value is kept")
	}
}

func TestASecretTheCutSplitsStaysHidden(t *testing.T) {
	t.Parallel()
	// A small ring, for the many lines written here to be short ones.
	const capacity, limit = minLine * lineFraction, minLine
	for name, secret := range map[string]string{
		// What is left of each at the cut is not, by itself, what Redact
		// hides: a quoted value without its closing quote, a name cut
		// before its value.
		"quoted value": `"token":"SECRET-0123456789 SECRET-abcdefghij"`,
		"pair":         `password=SECRET-0123456789`,
		"address":      `https://addon.example/SECRET-0123456789/manifest.json`,
	} {
		// The cut falls at every place of the secret in turn.
		for inside := range len(secret) + 1 {
			ring := NewRing(capacity)
			line := strings.Repeat("a", limit-inside-1) + " " + secret + " " + strings.Repeat("b", 2*lookAhead)
			_, _ = ring.Write([]byte(line + "\n"))
			lines, _ := ring.Since(0, 0)
			if len(lines) != 1 || len(lines[0])+1 > limit || !strings.HasSuffix(lines[0], cutMark) {
				t.Fatalf("%s cut after %d bytes: %d lines, the first of %d bytes", name, inside, len(lines), len(lines[0]))
			}
			if strings.Contains(lines[0], "SECRET") {
				t.Errorf("%s cut after %d bytes: the line ends with %q", name, inside, lines[0][len(lines[0])-100:])
			}
		}
	}
}

func TestACutLineEndsOnAWholeCharacter(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{1 << 20, 64, 9, 4} {
		for shift := range 4 {
			ring := NewRing(capacity)
			_, _ = ring.Write([]byte(strings.Repeat("a", shift) + strings.Repeat("é€", 1<<16) + "\n"))
			got := ring.Contents()
			limit := min(capacity, max(capacity/lineFraction, minLine))
			if len(got) == 0 || len(got) > limit || got[len(got)-1] != '\n' || !utf8.Valid(got) {
				t.Errorf("capacity %d, shift %d: %d bytes, ending with %q", capacity, shift, len(got), got[max(len(got)-12, 0):])
			}
			if limit > 2*len(cutMark) && !strings.HasSuffix(string(got), cutMark+"\n") {
				t.Errorf("capacity %d, shift %d: the cut is not marked: %q", capacity, shift, got[max(len(got)-12, 0):])
			}
		}
	}
}

// The ring redacts a line before it takes its lock: while one long line is
// being redacted, a line of another goroutine is written and read.
func TestALineBeingRedactedHoldsNoOtherLineBack(t *testing.T) {
	t.Parallel()
	ring := NewRing(Capacity)
	redacting, release := make(chan struct{}), make(chan struct{})
	var locked, calls atomic.Int32
	ring.redact = func(line string) string {
		if strings.HasPrefix(line, "long") && calls.Add(1) == 1 {
			// The lock is free while the line is redacted; were it held,
			// the test says so rather than wait for a line that cannot
			// be written.
			if ring.mu.TryLock() {
				ring.mu.Unlock()
			} else {
				locked.Store(1)
				close(redacting)
				return Redact(line)
			}
			close(redacting)
			<-release
		}
		return Redact(line)
	}
	written := make(chan struct{})
	go func() {
		defer close(written)
		_, _ = ring.Write([]byte("long " + strings.Repeat("pw=1 ", 1<<12) + "\n"))
	}()
	<-redacting
	if locked.Load() != 0 {
		<-written
		t.Fatal("the line is redacted under the ring's lock")
	}
	_, _ = ring.Write([]byte("short line\n"))
	if got := string(ring.Contents()); got != "short line\n" {
		t.Errorf("while the long line is redacted, the ring holds %.60q", got)
	}
	if lines, next := ring.Since(0, 0); len(lines) != 1 || next != 1 {
		t.Errorf("while the long line is redacted: %d lines, %d written", len(lines), next)
	}
	close(release)
	<-written
	lines, _ := ring.Since(0, 0)
	if len(lines) != 2 || lines[0] != "short line" || !strings.HasPrefix(lines[1], "long pw=<redacted> ") {
		t.Errorf("%d lines: %.40q", len(lines), lines)
	}
}

// The text log escapes line ends and control characters: whatever a record
// carries, it is one line of the ring.
func TestARecordIsOneLineOfTheRing(t *testing.T) {
	t.Parallel()
	ring := NewRing(Capacity)
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(io.Discard, ring), nil))
	logger.Info("An addon answered", "answer", "one\ntime=2026-01-01T00:00:00Z level=ERROR msg=\"another\"\r\n\x1b[2Jtwo")
	lines, written := ring.Since(0, 0)
	if len(lines) != 1 || written != 1 || strings.Contains(lines[0], "\x1b") || !strings.Contains(lines[0], "An addon answered") {
		t.Errorf("lines: %q", lines)
	}
}
