package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// vodFiles encodes four seconds of H.264 and AAC into a finished HLS
// playlist of MPEG-TS segments of a second, index.m3u8 naming s0.ts to
// s3.ts, and returns its files by name.
func vodFiles(t *testing.T, ffmpeg string) map[string][]byte {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24:duration=4",
		"-f", "lavfi", "-i", "sine=duration=4", "-c:v", "libx264", "-preset", "ultrafast", "-g", "24", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-f", "hls", "-hls_time", "1", "-hls_playlist_type", "vod", "-hls_segment_filename", filepath.Join(dir, "s%d.ts"),
		filepath.Join(dir, "index.m3u8")).CombinedOutput(); err != nil {
		t.Fatalf("encoding: %v: %s", err, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name()] = data
	}
	return files
}

// vodAddon serves one movie whose one stream is a finished HLS playlist,
// stream, among files served under /hls/, each by its path. With a
// header, as the stream's proxyHeaders declare it, every file of the
// stream answers 403 to a request without it, counted in refused. Each
// file's text may name the server's address as {origin}.
func vodAddon(t *testing.T, stream string, files map[string][]byte, header [2]string, refused *atomic.Int32) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt4100", Type: "movie", Name: "Pattern", Year: "2024"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "vod", Name: "VOD", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt4100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			s := stremio.Stream{Name: "VOD", URL: server.URL + "/hls/" + stream}
			if header[0] != "" {
				s.BehaviorHints.ProxyHeaders = &stremio.ProxyHeaders{Request: map[string]string{header[0]: header[1]}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{s}})
		case strings.HasPrefix(path, "/hls/"):
			if header[0] != "" && r.Header.Get(header[0]) != header[1] {
				refused.Add(1)
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			data, ok := files[strings.TrimPrefix(path, "/hls/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			if strings.HasSuffix(path, ".m3u8") {
				w.Header().Set("Content-Type", "application/x-mpegurl")
				data = []byte(strings.ReplaceAll(string(data), "{origin}", server.URL))
			}
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(string(data)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// An addon's stream that is a finished HLS playlist (#1197) plays as a
// file of four seconds: analyzed, relayed as it is to an app that plays
// HLS, and converted by FFmpeg for one that does not. Every address the
// playlist names reaches the source through Polyfin, with the headers the
// addon declares: segments named relative to the playlist, segments named
// by their full address that the source refuses without its header, and
// the variants of a master playlist with their own segments. The playlist
// stays finished, as #EXT-X-ENDLIST tells. The movie's media source tells
// no size, which ffprobe reads as the playlist's bytes, and the bitrate of
// the variant Polyfin plays, the first, from its AVERAGE-BANDWIDTH when
// given, else its BANDWIDTH; a conversion of it then asks no more than
// that bitrate leads to.
func TestFinishedHLSStreamsPlayThroughPolyfin(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	encoded := vodFiles(t, ffmpeg)
	playlist := string(encoded["index.m3u8"])
	absolute := strings.ReplaceAll(playlist, "\ns", "\n{origin}/hls/s")
	relative := map[string][]byte{}
	for name, data := range encoded {
		relative[name] = data
	}
	absoluteFiles := map[string][]byte{"index.m3u8": []byte(absolute)}
	variants := map[string][]byte{"master.m3u8": []byte("#EXTM3U\n#EXT-X-VERSION:3\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=400000,RESOLUTION=320x180\nlow/index.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=320x180\nhigh/index.m3u8\n"),
		"averaged.m3u8": []byte("#EXTM3U\n#EXT-X-VERSION:3\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=400000,AVERAGE-BANDWIDTH=350000,RESOLUTION=320x180\nlow/index.m3u8\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=800000,AVERAGE-BANDWIDTH=700000,RESOLUTION=320x180\nhigh/index.m3u8\n")}
	for name, data := range encoded {
		if strings.HasSuffix(name, ".ts") {
			absoluteFiles[name] = data
		}
		variants["low/"+name], variants["high/"+name] = data, data
	}
	for _, tc := range []struct {
		name   string
		stream string
		files  map[string][]byte
		header [2]string
		// bitrate is the media source's, 0 for none; converted, the most
		// the AVERAGE-BANDWIDTH of a conversion of it may be, 0 for any.
		bitrate, converted int64
	}{
		{"relative segments", "index.m3u8", relative, [2]string{}, 0, 0},
		{"absolute segments needing a header", "index.m3u8", absoluteFiles, [2]string{"Referer", "https://site.example/"}, 0, 0},
		{"a master playlist of two variants", "master.m3u8", variants, [2]string{}, 400_000, 2_000_000},
		{"a master playlist of averaged variants", "averaged.m3u8", variants, [2]string{}, 350_000, 2_000_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var refused atomic.Int32
			s := newProbingServer(t, 10, ffprobe)
			member := s.user("member", nil)
			if _, err := s.addons.Install(t.Context(), addons.Shared(), vodAddon(t, tc.stream, tc.files, tc.header, &refused), false); err != nil {
				t.Fatal(err)
			}
			token := s.signIn("member", "tv")
			var views, page QueryResult
			s.get(t, "/UserViews", token, &views)
			s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
			if len(page.Items) != 1 {
				t.Fatalf("items: %+v", page.Items)
			}
			movie := page.Items[0].Id
			profile := func(name string) json.RawMessage {
				data, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", name+".json"))
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			get := func(address string) (int, string) {
				t.Helper()
				if strings.HasPrefix(address, "/") {
					address = s.url + address
				}
				response, body := fetchURL(t, address, nil)
				return response.StatusCode, body
			}

			// Analyzed, it plays as it is in jellyfin-web, which plays HLS:
			// a file of four seconds.
			answer := s.ask(t, token, movie, profile("jellyfin-web-chrome"), map[string]any{"IsPlayback": true})
			if len(answer.MediaSources) != 1 {
				id, _ := accounts.ParseID(movie)
				versions, _ := s.library.KnownVersions(t.Context(), member, id)
				for _, version := range versions {
					t.Logf("version %s: %v", version.ID, s.handler.Playback.Failure(version.ID))
				}
				t.Fatalf("PlaybackInfo: %+v", answer)
			}
			source := answer.MediaSources[0]
			if source.IsInfiniteStream || source.RunTimeTicks == nil || *source.RunTimeTicks < 35_000_000 || !source.SupportsDirectPlay {
				t.Fatalf("the movie's media source: %+v", source)
			}
			if source.Size != nil {
				t.Errorf("the movie's media source tells a size of %d bytes", *source.Size)
			}
			switch {
			case tc.bitrate == 0 && source.Bitrate != nil:
				t.Errorf("the movie's media source tells a bitrate of %d", *source.Bitrate)
			case tc.bitrate != 0 && source.Bitrate == nil:
				t.Errorf("the movie's media source tells no bitrate, want %d", tc.bitrate)
			case tc.bitrate != 0 && *source.Bitrate != tc.bitrate:
				t.Errorf("the movie's media source's bitrate: %d, want %d", *source.Bitrate, tc.bitrate)
			}

			// Relayed, every address it names, a variant's segments
			// included, is Polyfin's, and answers.
			status, top := get("/Videos/" + movie + "/stream?static=true&mediaSourceId=" + source.Id + "&ApiKey=" + token)
			if status != http.StatusOK || !strings.HasPrefix(top, "#EXTM3U") {
				t.Fatalf("the relayed playlist: %d %s", status, top)
			}
			segments := 0
			var follow func(playlist string)
			follow = func(playlist string) {
				t.Helper()
				if strings.Contains(playlist, "#EXTINF") && !strings.Contains(playlist, "#EXT-X-ENDLIST") {
					t.Errorf("a media playlist that no longer ends: %s", playlist)
				}
				for line := range strings.Lines(playlist) {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					if !strings.HasPrefix(line, s.url+"/") {
						t.Fatalf("an address that is not Polyfin's: %q", line)
					}
					status, body := get(line)
					switch {
					case status != http.StatusOK:
						t.Fatalf("%s: %d %s", line, status, body)
					case strings.HasPrefix(body, "#EXTM3U"):
						follow(body)
					case len(body) == 0 || body[0] != 0x47:
						t.Fatalf("%s: not MPEG-TS", line)
					default:
						segments++
					}
				}
			}
			follow(top)
			if want := strings.Count(top, "#EXT-X-STREAM-INF") * 4; segments != max(want, 4) {
				t.Errorf("%d segments relayed, want %d", segments, max(want, 4))
			}

			// Converted by FFmpeg, which reads it through Polyfin, for an
			// app whose profile does not play HLS as it is.
			answer = s.ask(t, token, movie, profile("swiftfin-native"), map[string]any{"IsPlayback": true})
			if len(answer.MediaSources) != 1 || answer.MediaSources[0].TranscodingUrl == "" {
				t.Fatalf("PlaybackInfo without HLS played as it is: %+v", answer)
			}
			transcoding := answer.MediaSources[0].TranscodingUrl
			status, master := get(transcoding)
			if status != http.StatusOK {
				t.Fatalf("the master playlist: %d %s", status, master)
			}
			if tc.converted > 0 {
				_, attributes, _ := strings.Cut(master, "AVERAGE-BANDWIDTH=")
				average, _, _ := strings.Cut(attributes, ",")
				if n, err := strconv.ParseInt(average, 10, 64); err != nil || n > tc.converted {
					t.Errorf("the conversion's average bandwidth: %q, want at most %d", average, tc.converted)
				}
			}
			base := transcoding[:strings.Index(transcoding, "master.m3u8")]
			var media string
			for line := range strings.Lines(master) {
				if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
					media = line
				}
			}
			status, list := get(base + media)
			if status != http.StatusOK || !strings.Contains(list, "#EXT-X-ENDLIST") {
				t.Fatalf("the media playlist: %d %s", status, list)
			}
			var first string
			for line := range strings.Lines(list) {
				if line = strings.TrimSpace(line); first == "" && line != "" && !strings.HasPrefix(line, "#") {
					first = line
				}
			}
			address, err := url.Parse(base + media)
			if err != nil {
				t.Fatal(err)
			}
			segment, err := address.Parse(first)
			if err != nil {
				t.Fatal(err)
			}
			if status, body := get(segment.String()); status != http.StatusOK || len(body) == 0 {
				t.Errorf("the first segment FFmpeg made: %d %d bytes", status, len(body))
			}
			if n := refused.Load(); n != 0 {
				t.Errorf("%d requests without the declared header", n)
			}
		})
	}
}
