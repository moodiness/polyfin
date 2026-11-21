package jellyfin

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// localMovie is the streaming addon's movie with a third version, a file
// of a local folder with a French subtitle file beside it, whose bytes it
// returns; the movie's other versions have none. The addon lists a French
// file for the title.
func localMovie(t *testing.T) (playbackSetup, library.Version, []byte) {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	p := playingOn(t, s)
	video, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	// Past the 64 KiB at each end the hash of a file reads.
	video = append(video, make([]byte, 150_000)...)
	dir := t.TempDir()
	for name, data := range map[string]string{
		"Movie (2008) 1080p.mkv":    string(video),
		"Movie (2008) 1080p.fr.srt": "1\n00:00:01,000 --> 00:00:02,500\nMon fichier\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := p.library.Versions(t.Context(), p.user, movie)
	i := slices.IndexFunc(versions, func(v library.Version) bool { return v.LocalFolder })
	if err != nil || i < 0 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	return p, versions[i], video
}

// A subtitle file beside the user's own video is shown before an addon's
// for the title in the same language; a version with none of its own
// shows the addon's, as before.
func TestAUsersOwnSubtitleFileComesBeforeAnAddons(t *testing.T) {
	t.Parallel()
	p, local, _ := localMovie(t)
	if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"SubtitleLanguagePreference": "fre", "SubtitleMode": "Always"}); status != http.StatusNoContent {
		t.Fatalf("configuration: %d", status)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	for _, source := range *movie.MediaSources {
		var files []string
		for _, stream := range source.MediaStreams {
			if stream.IsExternal {
				files = append(files, stream.Language)
			}
		}
		want, shown := []string{"fra"}, 0
		if source.Id == local.ID.String() {
			// The addon's file first, then the local file's own.
			want, shown = []string{"fra", "fra"}, 1
		}
		if !slices.Equal(files, want) {
			t.Fatalf("%s: subtitle files %q, want %q", source.Name, files, want)
		}
		if got := source.DefaultSubtitleStreamIndex; got == nil || *got != shown {
			t.Errorf("%s: subtitle shown %v, want %d", source.Name, deref(got), shown)
		}
	}
}

// videoHashOf is the video hash subtitle addons take of data: its size
// plus every 8-byte little-endian word of its first and last 64 KiB.
func videoHashOf(data []byte) string {
	const chunk = 64 << 10
	sum := uint64(len(data))
	for _, part := range [][]byte{data[:chunk], data[len(data)-chunk:]} {
		for i := 0; i < chunk; i += 8 {
			sum += binary.LittleEndian.Uint64(part[i:])
		}
	}
	return fmt.Sprintf("%016x", sum)
}

// subtitleAddon serves subtitles alone: an English file for the movie,
// "Hello". It records what follows the movie's identifier in each request,
// and answers none while block is set, until release is closed.
type subtitleAddon struct {
	server  *httptest.Server
	block   atomic.Bool
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	asked   []string
}

func newSubtitleAddon(t *testing.T) *subtitleAddon {
	t.Helper()
	a := &subtitleAddon{release: make(chan struct{})}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "subtitles", Name: "Subtitles", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "subtitles"}}})
		case strings.HasPrefix(path, "/subtitles/movie/tt1000"):
			a.mu.Lock()
			a.asked = append(a.asked, strings.TrimSuffix(strings.TrimPrefix(path, "/subtitles/movie/tt1000"), ".json"))
			a.mu.Unlock()
			if a.block.Load() {
				<-a.release
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{{ID: "en-1", URL: a.server.URL + "/hello.srt", Lang: "eng"}}})
		case path == "/hello.srt":
			_, _ = io.WriteString(w, "1\r\n00:00:01,000 --> 00:00:04,000\r\nHello\r\n")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(a.server.Close)
	t.Cleanup(a.unblock)
	return a
}

// unblock answers the requests waiting, and those to come.
func (a *subtitleAddon) unblock() {
	a.block.Store(false)
	a.once.Do(func() { close(a.release) })
}

func (a *subtitleAddon) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.asked)
}

// install installs the addon on p, first of the addons when first is set.
func (a *subtitleAddon) install(t *testing.T, p playbackSetup, first bool) {
	t.Helper()
	installed, err := p.addons.Install(t.Context(), addons.Shared(), a.server.URL+"/manifest.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		return
	}
	all, err := p.addons.Addons(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	order := []accounts.ID{installed.ID}
	for _, addon := range all {
		if addon.ID != installed.ID {
			order = append(order, addon.ID)
		}
	}
	if err := p.addons.Reorder(t.Context(), addons.Shared(), order); err != nil {
		t.Fatal(err)
	}
}

// playbackInfo asks PlaybackInfo for the movie's version, and returns its
// source.
func (p playbackSetup) versionPlayback(t *testing.T, token string, user accounts.User, version accounts.ID) MediaSourceInfo {
	t.Helper()
	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", token), map[string]any{
		"UserId": user.ID.String(), "MediaSourceId": version.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	return info.MediaSources[0]
}

// Subtitle addons are asked for the version played with its file name,
// size and video hash, from the bytes of the file a play read.
func TestSubtitleAddonsAreToldTheVersionPlayed(t *testing.T) {
	t.Parallel()
	p, local, video := localMovie(t)
	subtitles := newSubtitleAddon(t)
	subtitles.install(t, p, false)
	p.analyzed(t, local, "h264-aac-mp4")
	src := p.handler.Playback.OpenSource(local)
	if _, err := src.ReadAt(t.Context(), make([]byte, len(video)), 0); err != nil {
		t.Fatal(err)
	}
	src.Release()

	source := p.versionPlayback(t, p.token, p.user, local.ID)
	want := fmt.Sprintf("/videoHash=%s&videoSize=%d&filename=Movie%%20(2008)%%201080p.mkv", videoHashOf(video), len(video))
	if got := subtitles.requests(); !slices.Contains(got, want) {
		t.Errorf("the addon was asked %q, want %q", got, want)
	}
	// Its file, given for the version, is listed with the title's.
	var languages []string
	for _, stream := range source.MediaStreams {
		if stream.IsExternal {
			languages = append(languages, stream.Language)
		}
	}
	if want := []string{"eng", "fra", "fra"}; !slices.Equal(languages, want) {
		t.Errorf("subtitle files %q, want %q", languages, want)
	}
}

// subtitleAt fetches the index-th subtitle of the movie's version as
// SubRip.
func (p playbackSetup) subtitleAt(t *testing.T, version accounts.ID, index int) string {
	t.Helper()
	response, body := fetchURL(t, fmt.Sprintf("%s/Videos/%s/%s/Subtitles/%d/0/Stream.srt?ApiKey=%s", p.url, p.movie, version, index, p.token), nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("subtitle %d: %d %s", index, response.StatusCode, body)
	}
	return body
}

// A subtitle file keeps the index PlaybackInfo gave it: an addon listing
// files before it once PlaybackInfo answered, and other users' plays,
// move it neither for the app's subtitle requests nor in the next
// PlaybackInfo, where they come after it.
func TestSubtitleIndexesHoldOnceDescribed(t *testing.T) {
	t.Parallel()
	p := playing(t)
	version := p.versions[1].ID
	// An addon listed first answers after PlaybackInfo stopped waiting.
	late := newSubtitleAddon(t)
	late.install(t, p, true)
	late.block.Store(true)
	source := p.versionPlayback(t, p.token, p.user, version)
	var languages []string
	for _, stream := range source.MediaStreams {
		if stream.IsExternal {
			languages = append(languages, stream.Language)
		}
	}
	if !slices.Equal(languages, []string{"fra"}) {
		t.Fatalf("subtitle files %q, want the title's French one alone", languages)
	}
	late.unblock()

	// Another user plays the movie meanwhile, every file listed.
	other := p.testServer.user("other", nil)
	otherToken := p.signIn("other", "tv")
	if got := p.versionPlayback(t, otherToken, other, version).MediaStreams[0].Language; got != "eng" {
		t.Errorf("the other user's first file: %q", got)
	}
	if got := p.subtitleAt(t, version, 0); !strings.Contains(got, "Bonjour") {
		t.Errorf("subtitle 0 once the addon answered: %q", got)
	}
	// The next PlaybackInfo lists the new file after it.
	source = p.versionPlayback(t, p.token, p.user, version)
	languages = nil
	for _, stream := range source.MediaStreams {
		if stream.IsExternal {
			languages = append(languages, stream.Language)
		}
	}
	if !slices.Equal(languages, []string{"fra", "eng"}) {
		t.Errorf("subtitle files of the next PlaybackInfo %q, want French then English", languages)
	}
	if got := p.subtitleAt(t, version, 1); !strings.Contains(got, "Hello") {
		t.Errorf("subtitle 1: %q", got)
	}
}
