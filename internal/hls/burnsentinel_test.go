package hls

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// sentinelRectangle is the rectangle the sentinel tests write, on a
// canvas of 320 × 180, shown from 2 to 5 s and from 15 to 19 s.
var sentinelRectangle = testsubtitles.Rectangle{CanvasWidth: 320, CanvasHeight: 180, X: 60, Y: 120, Width: 200, Height: 40,
	Shown: [][2]time.Duration{{2 * time.Second, 5 * time.Second}, {15 * time.Second, 19 * time.Second}}}

// The sentinel comes an hour past the version's end, or at 12 hours when
// its duration is not known.
func TestSentinelTimeFollowsTheDuration(t *testing.T) {
	t.Parallel()
	for duration, want := range map[time.Duration]time.Duration{0: 12 * time.Hour, 30 * time.Second: time.Hour + 30*time.Second,
		2 * time.Hour: 3 * time.Hour, 20 * time.Hour: 21 * time.Hour} {
		if got := sentinelTime(duration); got != want {
			t.Errorf("a version of %v: sentinel at %v, want %v", duration, got, want)
		}
	}
}

// pgsSegmentRead is a segment of a PGS file: its type, its time in 90 kHz
// ticks and its payload.
type pgsSegmentRead struct {
	kind    byte
	ticks   uint32
	payload []byte
}

// pgsSegments splits a PGS file into its segments.
func pgsSegments(t *testing.T, data []byte) []pgsSegmentRead {
	t.Helper()
	var segments []pgsSegmentRead
	for len(data) > 0 {
		if len(data) < pgsSegment || data[0] != 'P' || data[1] != 'G' {
			t.Fatalf("a segment does not start where the last ended")
		}
		size := int(binary.BigEndian.Uint16(data[11:]))
		segments = append(segments, pgsSegmentRead{data[10], binary.BigEndian.Uint32(data[2:]), data[pgsSegment : pgsSegment+size]})
		data = data[pgsSegment+size:]
	}
	return segments
}

// A PGS file gets a presentation of no objects at the time asked, with
// the canvas and frame rate of its last one and the next composition
// number, then its end; a second past its last segment when that is
// later, and within the 32-bit clock.
func TestPGSSentinel(t *testing.T) {
	t.Parallel()
	original := sentinelRectangle.PGS()
	for _, c := range []struct {
		name string
		at   time.Duration
		want uint32
	}{
		{"from the duration", sentinelTime(30 * time.Second), uint32((time.Hour + 30*time.Second) / time.Millisecond * 90)},
		{"after the last picture", 10 * time.Second, 20 * 90000},
		{"capped", 20 * time.Hour, 0xFFFFFFFF},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if err := copyPGS(&out, bytes.NewReader(original), c.at); err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(out.Bytes(), original) {
				t.Fatal("the file is not copied as it is")
			}
			added := pgsSegments(t, out.Bytes()[len(original):])
			if len(added) != 2 || added[0].kind != 0x16 || added[1].kind != 0x80 || len(added[1].payload) != 0 {
				t.Fatalf("added %+v, want a presentation and its end", added)
			}
			for _, s := range added {
				if s.ticks != c.want {
					t.Errorf("segment %#x at %d ticks, want %d", s.kind, s.ticks, c.want)
				}
			}
			// 320 × 180, frame rate 0x10, composition 4 after 3, normal, no
			// palette update, palette 0, no objects.
			if want := []byte{1, 64, 0, 180, 0x10, 0, 4, 0, 0, 0, 0}; !bytes.Equal(added[0].payload, want) {
				t.Errorf("presentation %v, want %v", added[0].payload, want)
			}
		})
	}
}

// A PGS file whose segments do not follow each other to its end, without
// a presentation, or already at the end of the clock gets no sentinel.
func TestPGSSentinelRefusesMalformedFiles(t *testing.T) {
	t.Parallel()
	original := sentinelRectangle.PGS()
	late := bytes.Clone(original)
	binary.BigEndian.PutUint32(late[2:], 0xFFFFFFFF)
	for name, data := range map[string][]byte{
		"empty":     nil,
		"truncated": original[:len(original)-1],
		"trailing":  append(bytes.Clone(original), 'P'),
		"not PGS":   append([]byte("XX"), original[2:]...),
		// The end segments alone.
		"no presentation": original[len(original)-pgsSegment:],
		"late":            late,
	} {
		if err := copyPGS(io.Discard, bytes.NewReader(data), 12*time.Hour); !errors.Is(err, errNoSentinel) {
			t.Errorf("%s: %v, want errNoSentinel", name, err)
		}
	}
}

// twoTrackVobSub is the rectangle as a VobSub pair of two tracks, the
// first shown as sentinelRectangle is, the second from 8 to 12 s at the
// same place, then an index of a third without pictures, its lines
// ending in CR LF.
func twoTrackVobSub() (idx string, sub []byte) {
	idx, sub = sentinelRectangle.VobSub()
	second := sentinelRectangle
	second.Shown = [][2]time.Duration{{8 * time.Second, 12 * time.Second}}
	secondIdx, secondSub := second.VobSub()
	// The packs of the second track carry its index in their substream.
	for pack := 0; pack < len(secondSub); pack += 2048 {
		secondSub[pack+28] = 0x21
	}
	_, listed, _ := strings.Cut(secondIdx, "id: en, index: 0\n")
	listed = strings.ReplaceAll(listed, "filepos: 000000000", "filepos: 000001000")
	idx += "\nid: fr, index: 1\n" + listed + "\nid: de, index: 2\n"
	return strings.ReplaceAll(idx, "\n", "\r\n"), append(sub, secondSub...)
}

// Each track with pictures of a VobSub pair gets two packs that stop the
// display, at the time asked and a second later, listed after its last
// picture with the .sub file's size as their place, their substream
// carrying the track's index; a track without pictures gets none, as
// FFmpeg would then make a stream of it.
func TestVobSubSentinelMarksEveryTrack(t *testing.T) {
	t.Parallel()
	idx, sub := twoTrackVobSub()
	at := sentinelTime(30 * time.Second)
	marked, packs, err := vobSubSentinel(idx, int64(len(sub)), at)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(idx, "filepos: 000000800\r\n", "filepos: 000000800\r\n"+
		"timestamp: 01:00:30:000, filepos: 000001800\r\ntimestamp: 01:00:31:000, filepos: 000002000\r\n", 1)
	want = strings.Replace(want, "filepos: 000001000\r\n", "filepos: 000001000\r\n"+
		"timestamp: 01:00:30:000, filepos: 000002800\r\ntimestamp: 01:00:31:000, filepos: 000003000\r\n", 1)
	if marked != want {
		t.Errorf("index\n%q\nwant\n%q", marked, want)
	}
	if len(packs) != 4*2048 {
		t.Fatalf("%d bytes of packs, want 4 of 2048", len(packs))
	}
	for i, wantStream := range []byte{0x20, 0x20, 0x21, 0x21} {
		pack := packs[i*2048 : (i+1)*2048]
		if !bytes.HasPrefix(pack, []byte{0, 0, 1, 0xBA}) || pack[28] != wantStream {
			t.Errorf("pack %d: substream %#x, want %#x", i, pack[28], wantStream)
		}
		// Size 10, the control sequence at 4: stop the display, the last.
		if spu := pack[29:39]; !bytes.Equal(spu, []byte{0, 10, 0, 4, 0, 0, 0, 4, 0x02, 0xFF}) {
			t.Errorf("pack %d: subpicture %v", i, spu)
		}
	}
}

// The sentinel of a VobSub pair is never before a second past its last
// picture, counts the delays FFmpeg adds, and stays within the two-digit
// hours of the .idx file.
func TestVobSubSentinelTimes(t *testing.T) {
	t.Parallel()
	idx, sub := sentinelRectangle.VobSub()
	delayed := strings.Replace(idx, "id: en, index: 0\n", "id: en, index: 0\ndelay: 00:00:30:000\n", 1)
	for _, c := range []struct {
		name, idx string
		at        time.Duration
		want      string
	}{
		{"after the last picture", idx, 10 * time.Second, "00:00:16:000"},
		{"delayed", delayed, 10 * time.Second, "00:00:16:000"},
		{"delayed, from the duration", delayed, 2 * time.Hour, "01:59:30:000"},
		{"capped", idx, 120 * time.Hour, "99:59:58:999"},
	} {
		marked, _, err := vobSubSentinel(c.idx, int64(len(sub)), c.at)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(marked, "timestamp: "+c.want+", filepos: 000001000\n") {
			t.Errorf("%s: index\n%s\nwant a sentinel at %s", c.name, marked, c.want)
		}
	}
}

// An .idx file FFmpeg would refuse, or without pictures, gets no sentinel.
func TestVobSubSentinelRefusesMalformedFiles(t *testing.T) {
	t.Parallel()
	idx, _ := sentinelRectangle.VobSub()
	for name, malformed := range map[string]string{
		"empty":                        "",
		"no picture":                   strings.Split(idx, "timestamp:")[0],
		"a picture before any track":   strings.Replace(idx, "id: en, index: 0\n", "", 1),
		"a time it cannot read":        strings.Replace(idx, "timestamp: 00:00:02:000", "timestamp: 0:0:2.0", 1),
		"a track index past its limit": strings.Replace(idx, "index: 0", "index: 32", 1),
		"a delay it cannot read":       strings.Replace(idx, "id: en, index: 0\n", "id: en, index: 0\ndelay: soon\n", 1),
	} {
		if _, _, err := vobSubSentinel(malformed, 4096, 12*time.Hour); !errors.Is(err, errNoSentinel) {
			t.Errorf("%s: %v, want errNoSentinel", name, err)
		}
	}
}

// writeBurnCopy reads the files over HTTP as by path, the .sub file named
// as FFmpeg names it; a file it cannot add a sentinel to leaves nothing
// behind, and a Manager then burns the file itself in.
func TestBurnCopy(t *testing.T) {
	t.Parallel()
	served := t.TempDir()
	idx, sub := sentinelRectangle.VobSub()
	for name, data := range map[string][]byte{"a.sup": sentinelRectangle.PGS(), "b.IDX": []byte(idx), "b.SUB": sub, "c.idx": []byte(idx),
		"c.sub": sub, "bad.sup": []byte("not PGS")} {
		if err := os.WriteFile(filepath.Join(served, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer(http.FileServer(http.Dir(served)))
	defer server.Close()
	for _, original := range []string{filepath.Join(served, "a.sup"), server.URL + "/a.sup", server.URL + "/b.IDX", filepath.Join(served, "c.idx")} {
		dir := t.TempDir()
		at := sentinelTime(30 * time.Second)
		copied, err := writeBurnCopy(t.Context(), original, filepath.Join(dir, "burn"), at)
		if err != nil {
			t.Fatalf("%s: %v", original, err)
		}
		if strings.HasSuffix(original, ".sup") {
			want := bytes.Buffer{}
			if err := copyPGS(&want, bytes.NewReader(sentinelRectangle.PGS()), at); err != nil {
				t.Fatal(err)
			}
			if got, _ := os.ReadFile(copied); copied != filepath.Join(dir, "burn.sup") || !bytes.Equal(got, want.Bytes()) {
				t.Errorf("%s: copied to %s, not as copyPGS writes it", original, copied)
			}
			continue
		}
		wantIdx, packs, err := vobSubSentinel(idx, int64(len(sub)), at)
		if err != nil {
			t.Fatal(err)
		}
		gotIdx, _ := os.ReadFile(copied)
		gotSub, _ := os.ReadFile(filepath.Join(dir, "burn.sub"))
		if copied != filepath.Join(dir, "burn.idx") || string(gotIdx) != wantIdx || !bytes.Equal(gotSub, append(bytes.Clone(sub), packs...)) {
			t.Errorf("%s: copied to %s, not as vobSubSentinel writes it", original, copied)
		}
	}
	m, err := NewManager("ffmpeg-not-installed", t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, original := range []string{filepath.Join(served, "bad.sup"), server.URL + "/missing.sup", filepath.Join(served, "a.ass")} {
		dir := t.TempDir()
		r := &Remux{Encode: &VideoEncoding{BurnFile: original}}
		if file, ok := m.sentinelCopy(t.Context(), r, filepath.Join(dir, "burn")); ok || file != original {
			t.Errorf("%s: burned %s, want the file itself", original, file)
		}
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			t.Errorf("%s: left %v behind", original, entries)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(served, "bad.sup")); string(got) != "not PGS" {
		t.Error("the malformed file changed")
	}
}

// The jobs of an encoding burn in one copy of the subtitle file, made
// once in its folder and removed with it; a stream as one file burns in
// a copy of its own, removed once it ends.
func TestBurnCopyPerEncoding(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("FFmpeg's stand-in is a shell script")
	}
	dir := t.TempDir()
	original := filepath.Join(dir, "rectangle.sup")
	if err := os.WriteFile(original, sentinelRectangle.PGS(), 0o600); err != nil {
		t.Fatal(err)
	}
	// FFmpeg's stand-in writes down the subtitle file it is given, if it
	// is there, and fails.
	runs, ffmpeg := filepath.Join(dir, "runs"), filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in *.sup) [ -f \"$a\" ] && echo \"$a\" >> '" + runs + "';; esac; done\nexit 1\n"
	if err := process.WriteExecutable(ffmpeg, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	m, err := NewManager(ffmpeg, folder, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	given := func() []string {
		data, _ := os.ReadFile(runs)
		return strings.Fields(string(data))
	}
	plan := NewGridPlan(30 * time.Second)
	remux := Remux{Input: "source.mkv", Video: 0, Audio: -1, Format: FMP4, Plan: plan,
		Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 160, Height: 90, Bitrate: 1_000_000, FrameRate: 24, BurnFile: original}}
	open := func(context.Context) (Remux, func(), error) { return remux, func() {}, nil }
	key := Key{Session: "session", Audio: -1, Format: FMP4}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	// A first job, then another after a seek.
	for _, n := range []int{0, 3} {
		if _, err := m.Segment(ctx, key, open, n); err == nil {
			t.Fatalf("segment %d made by a failing FFmpeg", n)
		}
	}
	files := given()
	if len(files) != 2 || files[0] != files[1] || files[0] == original || !strings.HasPrefix(files[0], folder+string(os.PathSeparator)) {
		t.Fatalf("FFmpeg burned %v, want one copy in %s twice", files, folder)
	}
	m.Stop("session")
	if _, err := os.Stat(files[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the copy is left once the encoding stopped: %v", err)
	}
	if err := m.Progressive(ctx, Key{Session: "download", Audio: -1, Format: FMP4}, open, 0, io.Discard); err == nil {
		t.Fatal("a stream made by a failing FFmpeg")
	}
	files = given()
	if len(files) != 3 || slices.Contains(files[:2], files[2]) || !strings.HasPrefix(files[2], folder+string(os.PathSeparator)) {
		t.Fatalf("the stream burned %v, want a copy of its own in %s", files[2:], folder)
	}
	if _, err := os.Stat(filepath.Dir(files[2])); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the stream's copy is left once it ended: %v", err)
	}
}
