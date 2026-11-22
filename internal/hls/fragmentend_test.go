package hls

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A segment of copied video is made once FFmpeg read the next segment's
// keyframe: its last fragment tells where it ends, and the keyframe after,
// a segment later, is not waited for. The source here sends nothing past
// halfway between the second and the third keyframes.
func TestASegmentIsMadeOnceItsLastFragmentEnds(t *testing.T) {
	t.Parallel()
	ffmpeg, ffprobe := tools(t)
	path := filepath.Join(t.TempDir(), "source.mkv")
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "30", "-c:v", "libx264", "-preset", "veryfast", "-bf", "3", "-g", "1000", "-sc_threshold", "0",
		"-force_key_frames", "0,10,20", "-c:a", "aac", "-b:a", "96k", path).CombinedOutput()
	if err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	out, err = exec.Command(ffprobe, "-v", "error", "-select_streams", "v", "-show_entries", "packet=pts_time,pos,flags", "-of", "json", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var probed struct {
		Packets []struct {
			PTS   string `json:"pts_time"`
			Pos   string `json:"pos"`
			Flags string `json:"flags"`
		}
	}
	if err := json.Unmarshal(out, &probed); err != nil {
		t.Fatal(err)
	}
	var keyframes []time.Duration
	var offsets []int64
	for _, p := range probed.Packets {
		if strings.Contains(p.Flags, "K") {
			at, _ := strconv.ParseFloat(p.PTS, 64)
			offset, _ := strconv.ParseInt(p.Pos, 10, 64)
			keyframes, offsets = append(keyframes, seconds(at)...), append(offsets, offset)
		}
	}
	if len(keyframes) != 3 {
		t.Fatalf("the source has keyframes at %v", keyframes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	limit := (offsets[1] + offsets[2]) / 2
	held := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := int64(0)
		if value, ok := strings.CutPrefix(r.Header.Get("Range"), "bytes="); ok {
			first, _, _ := strings.Cut(value, "-")
			start, _ = strconv.ParseInt(first, 10, 64)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.FormatInt(int64(len(data))-start, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.Itoa(len(data)-1)+"/"+strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusPartialContent)
		if start < limit {
			_, _ = w.Write(data[start:limit])
			w.(http.Flusher).Flush()
		}
		select {
		case <-held:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(held) })

	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	plan := NewPlan(keyframes, 30*time.Second)
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: server.URL + "/source.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan}, func() {}, nil
	}
	key := Key{Session: "session", Audio: 1, Format: FMP4, User: "alice", Device: "tv", Version: "v1"}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	segment, err := m.Segment(ctx, key, open, 0)
	if err != nil {
		t.Fatalf("the first segment, the source sent up to byte %d of %d: %v", limit, len(data), err)
	}
	defer segment.Close()
	if made, err := io.ReadAll(segment); err != nil || len(made) == 0 {
		t.Errorf("the first segment: %d bytes, %v", len(made), err)
	}
}
