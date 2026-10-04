package logs

import (
	"strings"
	"testing"
)

func TestRedactHidesAddressesAndSecrets(t *testing.T) {
	for line, want := range map[string]string{
		`url=https://user:pw@addon.example/secret/manifest.json?x=1 status=500`: `url=https://addon.example/… status=500`,
		`msg="Get \"http://debrid.example/stream/abc\": EOF"`:                   `msg="Get \"http://debrid.example/…\": EOF"`,
		`Authorization: MediaBrowser Token="0123abcd", Client="x"`:              `Authorization: <redacted> Token=<redacted>, Client="x"`,
		`path=/Videos/1/stream?ApiKey=deadbeef&api_key=cafe`:                    `path=/Videos/1/stream?ApiKey=<redacted>&api_key=<redacted>`,
		`{"AccessToken":"secret","Pw":"hunter2"}`:                               `{"AccessToken":<redacted>,"Pw":<redacted>}`,
		`level=INFO msg="Polyfin started" address=[::]:8096`:                    `level=INFO msg="Polyfin started" address=[::]:8096`,
	} {
		if got := Redact(line); got != want {
			t.Errorf("Redact(%s)\n got %s\nwant %s", line, got, want)
		}
	}
}

func TestRingKeepsTheLastLinesWithinItsCapacity(t *testing.T) {
	ring := NewRing(45)
	for _, line := range []string{"first line\n", "second line\n", "third line token=x\n"} {
		_, _ = ring.Write([]byte(line))
	}
	got := string(ring.Contents())
	if got != "second line\nthird line token=<redacted>\n" {
		t.Errorf("contents: %q", got)
	}
	if _, _, size := ring.Stat(); size != len(got) || size > 45 {
		t.Errorf("size %d for %q", size, got)
	}
	_, _ = ring.Write([]byte(strings.Repeat("x", 100)))
	if got := ring.Contents(); len(got) != 45 || got[44] != '\n' {
		t.Errorf("long line: %q", got)
	}
}

func TestSinceFollowsTheNewLinesRedacted(t *testing.T) {
	ring := NewRing(1 << 10)
	_, _ = ring.Write([]byte("one\ntwo password=hunter2\n"))
	lines, next := ring.Since(0, 0)
	if strings.Join(lines, "|") != "one|two password=<redacted>" || next != 2 {
		t.Fatalf("first read: %q %d", lines, next)
	}
	if lines, again := ring.Since(next, 0); len(lines) != 0 || again != 2 {
		t.Errorf("nothing new: %q %d", lines, again)
	}
	_, _ = ring.Write([]byte("three\nfour\nfive\n"))
	if lines, next := ring.Since(next, 2); strings.Join(lines, "|") != "four|five" || next != 5 {
		t.Errorf("newest within the limit: %q %d", lines, next)
	}
	// A reader from before a restart reads everything kept.
	if lines, _ := ring.Since(99, 0); len(lines) != 5 {
		t.Errorf("after a restart: %q", lines)
	}
	// Lines dropped from the ring are skipped.
	small := NewRing(12)
	_, _ = small.Write([]byte("aaaa\nbbbb\ncccc\n"))
	if lines, next := small.Since(1, 0); strings.Join(lines, "|") != "bbbb|cccc" || next != 3 {
		t.Errorf("dropped lines: %q %d", lines, next)
	}
}
