package playback

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// hlsSegments encodes four MPEG-TS segments of a second, H.264 and AAC,
// with FFmpeg.
func hlsSegments(t *testing.T) (ffprobe string, segments [][]byte) {
	t.Helper()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	dir := t.TempDir()
	command := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=4",
		"-f", "lavfi", "-i", "sine=duration=4", "-c:v", "libx264", "-g", "25", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "1", "-hls_list_size", "0", "-hls_segment_filename", filepath.Join(dir, "s%d.ts"), filepath.Join(dir, "all.m3u8"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v %s", err, output)
	}
	for n := range 4 {
		data, err := os.ReadFile(filepath.Join(dir, "s"+strconv.Itoa(n)+".ts"))
		if err != nil {
			t.Fatal(err)
		}
		segments = append(segments, data)
	}
	return filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), segments
}

// noise is a picture of random pixels, so that it compresses badly: a
// picture of some size, as providers put in front of their segments.
func noise(side int) image.Image {
	random := rand.New(rand.NewPCG(1, 2))
	picture := image.NewNRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			picture.Set(x, y, color.NRGBA{R: uint8(random.UintN(256)), G: uint8(random.UintN(256)), B: uint8(random.UintN(256)), A: 255})
		}
	}
	return picture
}

func pngOf(t *testing.T, side int) []byte {
	var out bytes.Buffer
	if err := png.Encode(&out, noise(side)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func jpegOf(t *testing.T, side int) []byte {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, noise(side), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// hlsProvider answers like an IPTV provider's live HLS channel: its
// playlist, index.m3u8, then each file it names by the file's name and
// query, nothing else.
type hlsProvider struct {
	playlist string
	files    map[string]func() *http.Response
}

func (p *hlsProvider) Open(_ context.Context, _, target string, _ http.Header, _ bool) (*http.Response, error) {
	address, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	name := path.Base(address.Path)
	if address.RawQuery != "" {
		name += "?" + address.RawQuery
	}
	if name == "index.m3u8" {
		return sized(http.StatusOK, "application/vnd.apple.mpegurl", []byte(p.playlist)), nil
	}
	if file, ok := p.files[name]; ok {
		return file(), nil
	}
	return sized(http.StatusNotFound, "text/plain", []byte("no such file")), nil
}

// sized answers body with its length, as a server does.
func sized(status int, contentType string, body []byte) *http.Response {
	response := answer(status, contentType, bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return response
}

// liveChannel is a provider's live channel whose playlist names the
// segments as name gives them, each answered by segment.
func liveChannel(segments [][]byte, header string, name func(n int) string, segment func(n int) *http.Response) *hlsProvider {
	p := &hlsProvider{files: map[string]func() *http.Response{}}
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:0\n" + header)
	for n := range segments {
		fmt.Fprintf(&playlist, "#EXTINF:1.000,\n%s\n", name(n))
		p.files[name(n)[strings.LastIndexByte(name(n), '/')+1:]] = func() *http.Response { return segment(n) }
	}
	p.playlist = playlist.String()
	return p
}

// providerVersion is a channel of the provider at a private address, as
// IPTV lists name them.
func providerVersion() library.Version {
	return library.Version{ID: accounts.ID{7}, Name: "Zeb Sport · Live", URL: "http://10.0.0.1/live/0123-token/index.m3u8"}
}

// FFmpeg reads a segment by what it holds, whatever the provider names
// it: Polyfin names the segments it relays by a name FFmpeg takes media
// by, as FFmpeg refuses a segment named .png, .jpg, .php or nothing that
// holds MPEG-TS ("Invalid data found when processing input").
func TestSegmentsAreReadWhateverTheirName(t *testing.T) {
	ffprobe, segments := hlsSegments(t)
	for _, tc := range []struct {
		name   string
		naming func(n int) string
	}{
		{"named .png", func(n int) string { return "s" + strconv.Itoa(n) + ".png" }},
		{"named .jpg", func(n int) string { return "s" + strconv.Itoa(n) + ".jpg" }},
		{"named .php with a query", func(n int) string { return "seg.php?n=" + strconv.Itoa(n) }},
		{"named without an extension", func(n int) string { return "seg/" + strconv.Itoa(n) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			channel := liveChannel(segments, "", tc.naming, func(n int) *http.Response {
				return sized(http.StatusOK, "image/png", segments[n])
			})
			s := newService(t, channel, ffprobe, nil)
			analysis, err := s.AnalyzeLive(t.Context(), providerVersion())
			if err != nil {
				t.Fatal(err)
			}
			if video, ok := LiveVideo(analysis); !ok || video.Codec != "h264" {
				t.Errorf("analysis: %+v", analysis.Streams)
			}
		})
	}
}

// A picture in front of each segment's MPEG-TS, as some providers put
// one, is skipped: FFmpeg would find the picture instead.
func TestPicturesInFrontOfSegmentsAreSkipped(t *testing.T) {
	ffprobe, segments := hlsSegments(t)
	for _, tc := range []struct {
		name    string
		picture []byte
		naming  func(n int) string
	}{
		{"a PNG, named .ts", pngOf(t, 20), func(n int) string { return "s" + strconv.Itoa(n) + ".ts" }},
		{"a JPEG, named .jpg", jpegOf(t, 96), func(n int) string { return "s" + strconv.Itoa(n) + ".jpg" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("a picture of %d bytes", len(tc.picture))
			channel := liveChannel(segments, "", tc.naming, func(n int) *http.Response {
				return sized(http.StatusOK, "image/jpeg", append(bytes.Clone(tc.picture), segments[n]...))
			})
			s := newService(t, channel, ffprobe, nil)
			analysis, err := s.AnalyzeLive(t.Context(), providerVersion())
			if err != nil {
				t.Fatal(err)
			}
			if video, ok := LiveVideo(analysis); !ok || video.Codec != "h264" {
				t.Errorf("analysis: %+v", analysis.Streams)
			}
		})
	}
}

// The segments relayed to apps lose the picture in front of them too, of
// any of the kinds providers use, and keep their length right; a picture
// that holds no MPEG-TS after it, and a segment without a picture, are
// relayed as they are.
func TestRelayedSegmentsLoseThePictureInFront(t *testing.T) {
	ts := bytes.Repeat(tsChunk, 4)
	webp := []byte("RIFF\x24\x00\x00\x00WEBPVP8 \x18\x00\x00\x00" + strings.Repeat("\x10", 24))
	for _, tc := range []struct {
		name string
		body []byte
		want []byte
	}{
		{"PNG", append(pngOf(t, 20), ts...), ts},
		{"JPEG", append(jpegOf(t, 32), ts...), ts},
		{"GIF", append([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"), ts...), ts},
		{"WebP", append(webp, ts...), ts},
		{"no picture", ts, ts},
		{"a picture alone", pngOf(t, 20), pngOf(t, 20)},
	} {
		channel := &hlsProvider{files: map[string]func() *http.Response{
			"s1.ts": func() *http.Response { return sized(http.StatusOK, "video/mp2t", tc.body) },
		}}
		s := newService(t, channel, "ffprobe-not-installed", nil)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/live/s1.ts", nil)
		err := s.ServeLive(recorder, request, providerVersion(), "http://10.0.0.1/live/0123-token/s1.ts", func(target, _ string) string { return target })
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
		}
		if got := recorder.Body.Bytes(); !bytes.Equal(got, tc.want) {
			t.Errorf("%s: relayed %d bytes starting % x, want %d starting % x", tc.name, len(got), got[:min(len(got), 8)], len(tc.want), tc.want[:8])
		}
		if length := recorder.Header().Get("Content-Length"); length != "" && length != strconv.Itoa(len(tc.want)) {
			t.Errorf("%s: Content-Length %s, want %d", tc.name, length, len(tc.want))
		}
	}
}

// records keeps the records a logger handles at INFO or above.
type records struct {
	mu   sync.Mutex
	kept []slog.Record
}

func (r *records) Enabled(_ context.Context, level slog.Level) bool { return level >= slog.LevelInfo }

func (r *records) Handle(_ context.Context, record slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kept = append(r.kept, record.Clone())
	return nil
}

func (r *records) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *records) WithGroup(string) slog.Handler { return r }

// attr is the value of the attribute key of the first record with
// message, and whether there is one.
func (r *records) attr(message, key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.kept {
		if record.Message != message {
			continue
		}
		value, found := "", false
		record.Attrs(func(a slog.Attr) bool {
			if a.Key == key {
				value, found = a.Value.String(), true
			}
			return !found
		})
		return value, found
	}
	return "", false
}

// A channel whose segments or key answer no media fails its analysis
// saying what they answered, by their file name, never their address
// nor their body, where ffprobe only says it found invalid data or
// nothing in time; FFmpeg's line refusing a segment by its name is kept,
// with the file name only. The log names the stream.
func TestHLSFilesThatAnswerNoMediaAreTold(t *testing.T) {
	ffprobe, segments := hlsSegments(t)
	page := []byte("<!DOCTYPE html><html><body>Access denied for secret-token</body></html>")
	segmentName := func(n int) string { return "s" + strconv.Itoa(n) + ".ts" }
	for _, tc := range []struct {
		name    string
		channel *hlsProvider
		notes   []string
	}{
		{"a segment gone", liveChannel(segments, "", segmentName, func(int) *http.Response {
			return sized(http.StatusNotFound, "text/plain", []byte("gone"))
		}), []string{".ts answered text (HTTP 404, text/plain, 4 bytes)"}},
		{"a page for a segment", liveChannel(segments, "", segmentName, func(int) *http.Response {
			return sized(http.StatusOK, "text/html", page)
		}), []string{".ts answered a web page (HTTP 200, text/html"}},
		{"a page for the key", func() *hlsProvider {
			p := liveChannel(segments, "#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin?token=secret\",IV=0x1\n", segmentName, func(n int) *http.Response {
				return sized(http.StatusOK, "video/mp2t", segments[n])
			})
			p.files["key.bin?token=secret"] = func() *http.Response { return sized(http.StatusOK, "text/html", page) }
			return p
		}(), []string{"key.bin answered a web page (HTTP 200, text/html"}},
		{"a picture for a segment", liveChannel(segments, "", func(n int) string { return "s" + strconv.Itoa(n) + ".png" }, func(int) *http.Response {
			return sized(http.StatusOK, "image/png", pngOf(t, 20))
		}), []string{"Invalid data found when processing input", "mismatches allowed extensions in url s", ".png.ts)"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newService(t, tc.channel, ffprobe, nil)
			log := &records{}
			s.logger = slog.New(log)
			_, err := s.AnalyzeLive(t.Context(), providerVersion())
			if err == nil {
				t.Fatal("analyzed")
			}
			t.Log(err)
			for _, note := range tc.notes {
				if !strings.Contains(err.Error(), note) {
					t.Errorf("%q does not tell %q", err, note)
				}
			}
			for _, secret := range []string{"secret", "10.0.0.1", "0123-token", "127.0.0.1", "Access denied"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("%q tells %q", err, secret)
				}
			}
			if stream, ok := log.attr("A channel's stream failed", "stream"); !ok || stream != providerVersion().Name {
				t.Errorf("the log names the stream %q (%v)", stream, ok)
			}
		})
	}
}
