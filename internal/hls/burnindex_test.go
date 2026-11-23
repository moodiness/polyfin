package hls

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/process"
)

// burnRuns plays original burned in, a segment of an encoding then a
// stream as one file, with a stand-in for FFmpeg that fails, and returns
// the arguments of each run reading the source, the folder of the
// encodings, and the last .idx file given that was there, as it was.
func burnRuns(t *testing.T, original string) ([][]string, string, string) {
	t.Helper()
	dir := t.TempDir()
	runs, ffmpeg := filepath.Join(dir, "runs"), filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\" >> '" + runs + "'; case \"$a\" in *.idx) [ -f \"$a\" ] && cat \"$a\" > '" + runs +
		".idx';; esac; done\necho -- >> '" + runs + "'\nexit 1\n"
	if err := process.WriteExecutable(ffmpeg, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	m, err := NewManager(ffmpeg, folder, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	remux := Remux{Input: "source.mkv", Video: 0, Audio: -1, Format: FMP4, Plan: NewGridPlan(30 * time.Second),
		Encode: &VideoEncoding{Encoder: "libx264", Level: "4.1", Width: 160, Height: 90, Bitrate: 1_000_000, FrameRate: 24, BurnFile: original}}
	open := func(context.Context) (Remux, func(), error) { return remux, func() {}, nil }
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if _, err := m.Segment(ctx, Key{Session: "session", Audio: -1, Format: FMP4}, open, 0); err == nil {
		t.Fatal("a segment made by a failing FFmpeg")
	}
	if err := m.Progressive(ctx, Key{Session: "download", Audio: -1, Format: FMP4}, open, 0, io.Discard); err == nil {
		t.Fatal("a stream made by a failing FFmpeg")
	}
	data, _ := os.ReadFile(runs)
	var args [][]string
	for run := range strings.SplitSeq(strings.TrimSuffix(string(data), "--\n"), "--\n") {
		if run := strings.Split(strings.TrimSuffix(run, "\n"), "\n"); slices.Contains(run, remux.Input) {
			args = append(args, run)
		}
	}
	if len(args) != 2 {
		t.Fatalf("FFmpeg ran %d times, want twice", len(args))
	}
	saved, _ := os.ReadFile(runs + ".idx")
	return args, folder, string(saved)
}

// inputs is the files a run of FFmpeg reads, each after the options given
// to it.
func inputs(args []string) [][]string {
	var files [][]string
	from := 0
	for i, a := range args {
		if a == "-i" && i+1 < len(args) {
			files = append(files, args[from:i+2])
			from = i + 2
		}
	}
	return files
}

// FFmpeg never opens a VobSub pair's .idx file itself: when no sentinel
// can be added, as when its .sub file cannot be copied or its layout is
// not known, it reads a copy of the .idx file as read, in the encoding's
// folder, and the pair's .sub file itself, named as it would name it.
func TestBurnIndexIsAlwaysACopy(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("FFmpeg's stand-in is a shell script")
	}
	idx, sub := sentinelRectangle.VobSub()
	unknown := strings.Split(idx, "timestamp:")[0]
	files := map[string][]byte{"Movie.IDX": []byte(idx), "Odd.idx": []byte(unknown), "Odd.sub": sub}
	share := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(share.Close)
	for _, c := range []struct{ name, idx, sub, read string }{
		{"a .sub file not copied", "Movie.IDX", "Movie.SUB", idx},
		{"a layout not known", "Odd.idx", "Odd.sub", unknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			original := share.URL + "/" + c.idx
			runs, folder, saved := burnRuns(t, original)
			for _, args := range runs {
				in := inputs(args)
				if len(in) != 2 {
					t.Fatalf("FFmpeg reads %v, want the source and the subtitle", in)
				}
				burned := in[1]
				file := burned[len(burned)-1]
				if file == original || !strings.HasPrefix(file, folder+string(os.PathSeparator)) || !strings.HasSuffix(file, ".idx") {
					t.Errorf("FFmpeg reads %s, want a copy of the .idx file in %s", file, folder)
				}
				if !slices.Equal(burned, []string{"-protocol_whitelist", "file,http,tcp", "-sub_name", share.URL + "/" + c.sub, "-i", file}) {
					t.Errorf("FFmpeg reads the subtitle with %v, want the .sub file %s named", burned, c.sub)
				}
			}
			if saved != c.read {
				t.Errorf("FFmpeg was given an .idx file of\n%q\nwant the file as read\n%q", saved, c.read)
			}
		})
	}
}

// FFmpeg burns in a copy of an .idx file with the pair's .sub file read
// where it is, by path or http URL. The pair has its sentinel, as the copy
// of an encoding has (see writeBurnCopy): FFmpeg 9.0.2 drops the end of a
// subtitle file that reaches its filter graph before the first frame of
// the video, and overlay then waits forever for what ends the last
// picture, which the frame at 16 s needs.
func TestBurnIndexCopyReadsItsSubFile(t *testing.T) {
	t.Parallel()
	ffmpeg, _ := tools(t)
	idx, sub := sentinelRectangle.VobSub()
	idx, packs, err := vobSubSentinel(idx, int64(len(sub)), sentinelTime(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	served, copied := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(served, "Movie.sub"), append(sub, packs...), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(copied, "burn-0.idx")
	if err := os.WriteFile(file, []byte(idx), 0o600); err != nil {
		t.Fatal(err)
	}
	share := httptest.NewServer(http.FileServer(http.Dir(served)))
	defer share.Close()
	for _, sub := range []string{filepath.Join(served, "Movie.sub"), share.URL + "/Movie.sub"} {
		encoding := &VideoEncoding{BurnFile: file, BurnSub: sub}
		// The rectangle is shown from 2 to 5 s and from 15 to 19 s, over
		// 160, 140 of the 320×180 canvas.
		for _, c := range []shown{{3, true}, {10, false}, {16, true}} {
			args := slices.Concat([]string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=gray:size=320x180:rate=24:duration=20"},
				encoding.burnFileArgs(), []string{"-filter_complex",
					"[0:v][1:0]overlay=eof_action=pass,select=gte(t\\," + strconv.FormatFloat(c.at, 'f', 3, 64) + "),format=rgb24,crop=1:1:160:140",
					"-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-"})
			// An FFmpeg that never ends fails this test, not the package.
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
			stuck := ctx.Err() != nil
			cancel()
			if stuck {
				t.Fatalf("%s: pixel at %.1f s: FFmpeg did not end within a minute: %s", sub, c.at, out)
			}
			if err != nil || len(out) != 3 {
				t.Fatalf("%s: pixel at %.1f s: %v: %s", sub, c.at, err, out)
			}
			if red := out[0] > 160 && out[1] < 90 && out[2] < 90; red != c.red {
				t.Errorf("%s: at %.1f s: color %v, subtitle shown %v", sub, c.at, out, c.red)
			}
		}
	}
}

// A VobSub pair whose .idx file cannot be read is not burned in, as one
// past its bound: FFmpeg reads the source alone.
func TestBurnIndexUnreadIsNotBurned(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("FFmpeg's stand-in is a shell script")
	}
	_, sub := sentinelRectangle.VobSub()
	share := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Movie.sub" {
			_, _ = w.Write(sub)
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer share.Close()
	runs, _, _ := burnRuns(t, share.URL+"/Movie.idx")
	for _, args := range runs {
		if in := inputs(args); len(in) != 1 || slices.Contains(args, "-sub_name") || slices.Contains(args, "-filter_complex") {
			t.Errorf("FFmpeg reads %v, want the source alone and nothing burned in", in)
		}
	}
}
