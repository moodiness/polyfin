package jellyfin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/stremio"
)

// heldFile serves a file, holding every request for its bytes while held:
// asked is told of the first request held, and release lets them all go.
type heldFile struct {
	path string
	mu   sync.Mutex
	hold chan struct{}
	// asked is closed at the first request held.
	asked chan struct{}
}

// holdNext holds the requests from now on, until the function it returns
// is called; the channel it returns is closed at the first one.
func (f *heldFile) holdNext() (<-chan struct{}, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hold, f.asked = make(chan struct{}), make(chan struct{})
	hold, asked := f.hold, f.asked
	var once sync.Once
	return asked, func() {
		once.Do(func() {
			f.mu.Lock()
			f.hold = nil
			f.mu.Unlock()
			close(hold)
		})
	}
}

func (f *heldFile) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	hold, asked := f.hold, f.asked
	if hold != nil && asked != nil {
		close(asked)
		f.asked = nil
	}
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	http.ServeFile(w, r, f.path)
}

// downloadAddon serves one movie whose one stream is file.
func downloadAddon(t *testing.T, file http.Handler) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt4000", Type: "movie", Name: "Pattern", Runtime: "3s", Year: "2024"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "downloads", Name: "Downloads", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt4000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case strings.HasPrefix(path, "/stream/movie/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Source", URL: server.URL + "/files/movie.mkv", BehaviorHints: stremio.StreamBehavior{Filename: "Movie.mkv"}}}})
		case path == "/files/movie.mkv":
			file.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// probed is what ffprobe tells of a file.
type probed struct {
	Streams []struct {
		Type   string `json:"codec_type"`
		Codec  string `json:"codec_name"`
		Height int
	}
}

func probeFile(t *testing.T, ffprobe, path string) probed {
	t.Helper()
	out, err := exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_type,codec_name,height", "-of", "json", path).Output()
	var p probed
	if err != nil || json.Unmarshal(out, &p) != nil {
		t.Fatalf("probe %s: %v %s", filepath.Base(path), err, out)
	}
	return p
}

// TestConvertedDownloads saves a title as Streamyfin's download sheet
// does: PlaybackInfo with its download profile and the quality chosen as
// the bitrate, then the TranscodingUrl, else /Items/{id}/Download.
func TestConvertedDownloads(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set: Polyfin converts video with the encoders FFmpeg has")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	if _, err := os.Stat(ffprobe); err != nil {
		t.Skip("no ffprobe next to POLYFIN_TEST_FFMPEG")
	}
	// Three seconds of H.264 at a high bitrate, AAC, and a PGS track.
	dir := t.TempDir()
	source := filepath.Join(dir, "movie.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-i", filepath.Join("..", "container", "testdata", "pgs.sup"),
		"-map", "0:v", "-map", "1:a", "-map", "2:s", "-t", "3", "-c:v", "libx264", "-preset", "ultrafast", "-qp", "8", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-c:s", "copy", source).CombinedOutput(); err != nil {
		t.Fatalf("make the source: %v: %s", err, out)
	}
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := exec.Command(ffprobe, "-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", source).Output()
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := media.Parse(probe)
	if err != nil {
		t.Fatal(err)
	}

	file := &heldFile{path: source}
	s := newTestServer(t, 10)
	s.user("member", func(c *accounts.UserChanges) { c.MaxPlaybacks = new(1) })
	s.user("guest", nil)
	s.user("saver", func(c *accounts.UserChanges) { c.ContentDownloading = new(false) })
	s.user("viewer", func(c *accounts.UserChanges) { c.VideoTranscoding = new(false) })
	groupedUser := s.user("grouped", func(c *accounts.UserChanges) { c.QualityGroup = new(1080) })
	if _, err := s.addons.Install(t.Context(), addons.Shared(), downloadAddon(t, file), false); err != nil {
		t.Fatal(err)
	}
	phone, tablet := s.signIn("member", "phone"), s.signIn("member", "tablet")
	guest, saver, viewer, grouped := s.signIn("guest", "phone"), s.signIn("saver", "phone"), s.signIn("viewer", "phone"), s.signIn("grouped", "phone")
	var views, page QueryResult
	s.get(t, "/UserViews", phone, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, phone, &page)
	if len(page.Items) != 1 {
		t.Fatalf("items: %+v", page.Items)
	}
	movie := page.Items[0].Id
	id, _ := accounts.ParseID(movie)
	versions, err := s.library.Versions(t.Context(), groupedUser, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	// store keeps analysis as the version's, forgetting the one Polyfin
	// keeps in memory.
	store := func(analysis media.Analysis) {
		t.Helper()
		data, _ := json.Marshal(analysis)
		if _, err := s.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2) "+
			"ON CONFLICT (version_id) DO UPDATE SET analysis = EXCLUDED.analysis", versions[0].ID, data); err != nil {
			t.Fatal(err)
		}
		s.handler.Playback.ForgetFile(versions[0].ID)
	}
	store(analysis)

	profile, err := os.ReadFile(filepath.Join(playbackFixtures, "profiles", "streamyfin-download.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The quality chosen: 300 kb/s.
	low := map[string]any{"MaxStreamingBitrate": 300_000}
	transcodingURL := func(token string, more map[string]any) string {
		t.Helper()
		answer := s.ask(t, token, movie, profile, more)
		if len(answer.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo: %+v", answer)
		}
		return answer.MediaSources[0].TranscodingUrl
	}
	saved := func(target string, header http.Header) (*http.Response, string) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+target, nil)
		for name, values := range header {
			request.Header[name] = values
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		path := filepath.Join(t.TempDir(), "saved")
		data, _ := io.ReadAll(response.Body)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return response, path
	}
	plays := func(path string) bool {
		t.Helper()
		out, err := exec.Command(ffmpeg, "-v", "error", "-xerror", "-i", path, "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Logf("decode: %v: %s", err, out)
		}
		return err == nil
	}

	// Off, the default: no TranscodingUrl.
	if target := transcodingURL(phone, low); target != "" {
		t.Errorf("off: %s", target)
	}

	s.setting(t, func(settings *accounts.Settings) { settings.ConvertedDownloads, settings.MaxConversions = true, 1 })
	// Without the permission to download, or to convert, as before.
	if target := transcodingURL(saver, low); target != "" {
		t.Errorf("without Can download: %s", target)
	}
	if target := transcodingURL(viewer, low); target != "" {
		t.Errorf("without Can use conversion: %s", target)
	}
	target := transcodingURL(phone, low)
	kept := target
	if !strings.HasPrefix(target, "/videos/"+hyphenated(id)+"/stream.mp4?") || !strings.Contains(target, "allowVideoStreamCopy=false") ||
		strings.Contains(target, "allowAudioStreamCopy") {
		t.Fatalf("at 300 kb/s: %s", target)
	}
	// The URL works for the user it was signed for only: another user's
	// key does not make it theirs.
	if !strings.Contains(target, "ApiKey="+phone) {
		t.Fatalf("the URL carries no key: %s", target)
	}
	if response, _ := saved(strings.Replace(target, "ApiKey="+phone, "ApiKey="+guest, 1), nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("with another user's key: %d", response.StatusCode)
	}

	// While the download runs, held at its source, which nothing read
	// before, it counts toward the video conversions at once, and as the
	// member's playback on the phone: none more is planned.
	asked, release := file.holdNext()
	defer release()
	type download struct {
		response *http.Response
		data     []byte
		err      error
	}
	done := make(chan download, 1)
	go func() {
		// An app resuming asks for a range; it gets the whole file.
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, s.url+target, nil)
		request.Header.Set("Range", "bytes=100-")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- download{err: err}
			return
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		done <- download{response, data, err}
	}()
	select {
	case <-asked:
	case <-time.After(time.Minute):
		t.Fatal("the download never read its source")
	}
	if target := transcodingURL(guest, low); target != "" {
		t.Errorf("another user's conversion at the limit: %s", target)
	}
	if answer := s.ask(t, tablet, movie, profile, low); answer.ErrorCode != "RateLimitExceeded" {
		t.Errorf("the member's other device: %+v", answer)
	}
	sessions, err := s.handler.LiveSessions(t.Context())
	if err != nil || len(sessions) != 1 || !sessions[0].Download || sessions[0].Item.ID != id || sessions[0].Device.DeviceName != "phone" ||
		len(sessions[0].Encodings) != 1 || !sessions[0].Encodings[0].Progressive {
		t.Errorf("live sessions: %+v %v", sessions, err)
	}
	release()
	var got download
	select {
	case got = <-done:
	case <-time.After(2 * time.Minute):
		t.Fatal("the download did not end")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	savedPath := filepath.Join(t.TempDir(), "saved.mp4")
	if err := os.WriteFile(savedPath, got.data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got.response.StatusCode != http.StatusOK || got.response.ContentLength != -1 || got.response.Header.Get("Accept-Ranges") != "" ||
		got.response.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("the download: %d, length %d, %v", got.response.StatusCode, got.response.ContentLength, got.response.Header)
	}
	info, _ := os.Stat(savedPath)
	streams := probeFile(t, ffprobe, savedPath).Streams
	if info.Size() >= int64(len(original)) || len(streams) != 2 || streams[0].Codec != "h264" || streams[1].Codec != "aac" || !plays(savedPath) {
		t.Errorf("the download: %d bytes of %d, %+v", info.Size(), len(original), streams)
	}
	// Done, it counts no more.
	if target := transcodingURL(guest, low); target == "" {
		t.Error("another user's conversion once the download is done")
	}
	if answer := s.ask(t, tablet, movie, profile, low); len(answer.MediaSources) != 1 {
		t.Errorf("the member's other device once the download is done: %+v", answer)
	}

	// A 4K version, under a 1080p quality group: downloaded converted down
	// to 1080p, at the app's "Max" quality; the file stays refused.
	tall := analysis
	tall.Streams = append([]media.Stream(nil), analysis.Streams...)
	for i := range tall.Streams {
		if tall.Streams[i].Type == "video" {
			tall.Streams[i].Width, tall.Streams[i].Height = 3840, 2160
		}
	}
	store(tall)
	target = transcodingURL(grouped, nil)
	if !strings.Contains(target, "/stream.mp4?") || !strings.Contains(target, "VideoResolutionNotSupported") {
		t.Fatalf("4K under a 1080p group: %s", target)
	}
	response, path := saved(target, nil)
	if streams := probeFile(t, ffprobe, path).Streams; response.StatusCode != http.StatusOK || len(streams) != 2 || streams[0].Height != 1080 || !plays(path) {
		t.Errorf("4K under a 1080p group: %d %+v", response.StatusCode, streams)
	}
	if response, _ := saved("/Items/"+movie+"/Download?ApiKey="+grouped, nil); response.StatusCode != http.StatusForbidden {
		t.Errorf("the 4K file under a 1080p group: %d", response.StatusCode)
	}
	// Off again, the app saves the file as it is, and so does the URL kept.
	s.setting(t, func(settings *accounts.Settings) { settings.ConvertedDownloads = false })
	store(analysis)
	for _, target := range []string{"/Items/" + movie + "/Download?ApiKey=" + phone, kept} {
		if response, path := saved(target, nil); response.StatusCode != http.StatusOK {
			t.Errorf("off again, %s: %d", target, response.StatusCode)
		} else if data, _ := os.ReadFile(path); string(data) != string(original) {
			t.Errorf("off again, %s: %d bytes of %d", target, len(data), len(original))
		}
	}
}
