package logs

import (
	"bytes"
	"strings"
	"testing"
)

// An echoing ring writes each line to its echo as it keeps it: redacted,
// cut alike, and in order, a record of several lines included.
func TestAnEchoingRingWritesWhatItKeeps(t *testing.T) {
	t.Parallel()
	const capacity, limit = minLine * lineFraction, minLine
	echo := &bytes.Buffer{}
	ring := NewEchoingRing(capacity, echo)
	_, _ = ring.Write([]byte("first token=TOKEN-1 url=https://addon.example/KEY-2/manifest.json\n"))
	_, _ = ring.Write([]byte("second password=PASS-3\nthird " + strings.Repeat("x", 2*limit) + " api_key=KEY-4\n"))
	if got := echo.String(); got != string(ring.Contents()) {
		t.Errorf("echoed:\n%q\nkept:\n%q", got, ring.Contents())
	}
	lines := strings.Split(strings.TrimSuffix(echo.String(), "\n"), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "first token=<redacted> url=https://addon.example/…") ||
		lines[1] != "second password=<redacted>" || !strings.HasSuffix(lines[2], cutMark) || len(lines[2])+1 > limit {
		t.Errorf("echoed lines: %.80q", lines)
	}
	for _, secret := range []string{"TOKEN-1", "KEY-2", "PASS-3", "KEY-4"} {
		if strings.Contains(echo.String(), secret) {
			t.Errorf("the echo holds %q", secret)
		}
	}
}
