package jellyfin

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
)

// manyVersionsAddon serves one movie, without a runtime, with n streams
// named Version 1 to Version n, whose files are not media. With subtitles
// held, it lists one subtitle file of the movie once subtitles is closed.
func manyVersionsAddon(t *testing.T, n int, subtitles chan struct{}) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt4000", Type: "movie", Name: "Movie", Year: "2008"}
	closing := make(chan struct{})
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			resources := []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}}
			if subtitles != nil {
				resources = append(resources, stremio.Resource{Name: "subtitles"})
			}
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "many", Name: "Many", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: resources,
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt4000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt4000.json":
			var streams []stremio.Stream
			for i := 1; i <= n; i++ {
				file := fmt.Sprintf("Movie.%d.mkv", i)
				streams = append(streams, stremio.Stream{Name: fmt.Sprintf("Version %d", i), URL: server.URL + "/files/" + file,
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: stremio.Number(1_000_000 * i)}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/subtitles/movie/tt4000"):
			select {
			case <-subtitles:
			case <-r.Context().Done():
				return
			case <-closing:
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{{ID: "fr-1", URL: server.URL + "/files/movie.fr.srt", Lang: "fre"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		close(closing)
		server.Close()
	})
	return server.URL + "/manifest.json"
}

// manyOn installs an addon of manyVersionsAddon on s, signs a member in,
// and lists the movie's versions.
func manyOn(t *testing.T, s testServer, url string) playbackSetup {
	t.Helper()
	user := s.user("member", nil)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), url, false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	if len(page.Items) != 1 {
		t.Fatalf("titles: %+v", page.Items)
	}
	p := playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}
	movie, _ := accounts.ParseID(p.movie)
	versions, err := s.library.Versions(t.Context(), user, movie)
	if err != nil || len(versions) == 0 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	p.versions = versions
	return p
}

// remuxableVersion stores for a version the analysis of a Matroska file
// whose audio, Opus, the minimal profile takes only converted, with, if
// indexed, its keyframe index, which a remux needs.
func (p playbackSetup) remuxableVersion(t *testing.T, version library.Version, indexed bool) {
	t.Helper()
	analysis := media.Analysis{Format: "matroska,webm", Duration: 15 * time.Second, Size: 21, Bitrate: 25_000, Remote: true,
		Streams: []media.Stream{
			{Index: 0, Type: "audio", Codec: "opus", Default: true, Channels: 1, SampleRate: 8000, ChannelLayout: "mono"},
			{Index: 1, Type: "video", Codec: "h264", Profile: "High", Level: 10, Width: 64, Height: 64, FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8},
		}}
	data, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
	if indexed {
		p.keyframed(t, version)
	}
}

// keyframed stores a keyframe index for a version, which streaming it
// over HLS needs.
func (p playbackSetup) keyframed(t *testing.T, version library.Version) {
	t.Helper()
	index := binary.AppendVarint(binary.AppendVarint(nil, 0), 6_000_000)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_keyframes (version_id, keyframes) VALUES ($1, $2)", version.ID, index); err != nil {
		t.Fatal(err)
	}
}

// The title's own identifier, which jellyfin-web, Android TV and Swiftfin
// send from a title's page, names no version: a version that does not
// answer gives way to the next, as when the app names none, and the answer
// lists the version chosen alone. A version named by its own identifier is
// tried alone.
func TestTheTitlesIdentifierFallsBackToTheNextVersion(t *testing.T) {
	probe := scriptedProbe(t, "[ $n -gt 1 ] || exec sleep 60\nexec cat \"$0.json\"\n")
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	p.setting(t, func(settings *accounts.Settings) { settings.AnalysisTimeout = accounts.MinAnalysisTimeout })
	chrome := p.profile(t, "jellyfin-web-chrome")
	started := time.Now()
	answer := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie})
	if elapsed := time.Since(started); elapsed > versionPatience+time.Second {
		t.Errorf("answered after %s", elapsed)
	}
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].Id != p.versions[1].ID.String() || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("the title asked for: %+v", answer)
	}
	// The first version's analysis failed meanwhile, as its source never
	// answered: asked for by its identifier, it is refused at once.
	eventually(t, "the first analysis to fail", func() bool { return p.handler.Playback.Failed(p.versions[0].ID) })
	if asked := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[0].ID.String()}); !asked.refused() {
		t.Errorf("the first version asked for: %+v", asked)
	}
}

// A version whose keyframe index cannot be read cannot be streamed over
// HLS: the next version plays instead, rather than one offered with
// nothing to play.
func TestAVersionThatCannotBeStreamedGivesWay(t *testing.T) {
	p := manyOn(t, newTestServer(t, 10), manyVersionsAddon(t, 2, nil))
	p.remuxableVersion(t, p.versions[0], false)
	p.remuxableVersion(t, p.versions[1], true)
	answer := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), map[string]any{"EnableDirectPlay": false})
	source := firstSource(t, answer)
	if source.Id != p.versions[1].ID.String() || !source.SupportsTranscoding || source.TranscodingUrl == "" {
		t.Errorf("chosen: %+v", source)
	}
	// Asked for alone, the first is refused.
	if asked := p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"),
		map[string]any{"EnableDirectPlay": false, "MediaSourceId": p.versions[0].ID.String()}); !asked.refused() {
		t.Errorf("the version without an index asked for: %+v", asked)
	}
}

// With PreferDirectPlay, a version that plays without conversion is looked
// for among the versions read at once, not beyond: when they all need a
// conversion, the first plays, and no other is analyzed for it.
func TestPreferDirectPlayLooksNoFurtherThanTheVersionsReadAtOnce(t *testing.T) {
	probe := newFakeProbe(t, true)
	p := manyOn(t, newProbingServer(t, 10, probe.path), manyVersionsAddon(t, 5, nil))
	for _, version := range p.versions[:parallelAttempts] {
		p.remuxableVersion(t, version, true)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay, settings.VersionAttempts = true, 5 })
	source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "minimal"), nil))
	if source.Id != p.movie || !strings.HasSuffix(source.TranscodingUrl, "&allowAudioStreamCopy=false") || probe.runs() != 0 {
		t.Errorf("chosen after %d analyses: %+v", probe.runs(), source)
	}
}

// PlaybackInfo waits for the addons' subtitles a second at most once the
// versions are in: those that come later reach the next PlaybackInfo.
func TestPlaybackInfoWaitsForSubtitlesASecondAtMost(t *testing.T) {
	subtitles := make(chan struct{})
	p := manyOn(t, newTestServer(t, 10), manyVersionsAddon(t, 1, subtitles))
	p.analyzed(t, p.versions[0], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	hasSubtitle := func(source MediaSourceInfo) bool {
		return slices.ContainsFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Type == "Subtitle" && s.IsExternal })
	}
	started := time.Now()
	source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil))
	if elapsed := time.Since(started); elapsed < subtitleGrace || elapsed > subtitleGrace+time.Second {
		t.Errorf("answered after %s while the subtitles were held", elapsed)
	}
	if hasSubtitle(source) {
		t.Errorf("a subtitle listed before its addon answered: %+v", source.MediaStreams)
	}
	close(subtitles)
	eventually(t, "the subtitles to be listed", func() bool {
		return hasSubtitle(firstSource(t, p.ask(t, p.token, p.movie, chrome, nil)))
	})
}

// A title page is told of its versions on the user's sockets as the addons
// answer, without asking.
func TestVersionsArePushedToTheTitlePage(t *testing.T) {
	addon := newHeldAddon(t, "Alpha", false)
	h := heldOn(t, newTestServer(t, 10), addon.url)
	socket, _, err := h.openSocket(t, h.token)
	if err != nil {
		t.Fatal(err)
	}
	h.sources(t)
	addon.answer()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case message := <-socket.messages:
			if message.MessageType != versionsMessage {
				continue
			}
			data, _ := json.Marshal(message.Data)
			var pushed versionsPush
			if err := json.Unmarshal(data, &pushed); err != nil || message.MessageId == "" {
				t.Fatalf("message %+v: %v", message, err)
			}
			if pushed.ItemId != h.movie {
				t.Fatalf("pushed for %s", pushed.ItemId)
			}
			if pushed.Pending == 0 && pushed.Count == 2 {
				if got := h.progress(t); got != (VersionProgress{Pending: 0, Count: 2}) {
					t.Errorf("asked once pushed: %+v", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("no push of the versions")
		}
	}
}

// listsAddon lists, for every movie, the streams named in the next list
// sent on replies, holding each request until it comes.
type listsAddon struct {
	url     string
	asked   atomic.Int32
	replies chan []string
}

func newListsAddon(t *testing.T) *listsAddon {
	t.Helper()
	a := &listsAddon{replies: make(chan []string, 10)}
	closing := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "lists", Name: "Lists", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}}})
		case strings.HasPrefix(path, "/stream/movie/"):
			a.asked.Add(1)
			var names []string
			select {
			case names = <-a.replies:
			case <-r.Context().Done():
				return
			case <-closing:
				return
			}
			streams := make([]stremio.Stream, len(names))
			for i, name := range names {
				streams[i] = stremio.Stream{Name: name, URL: server.URL + "/files/" + name + ".mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: name + ".mkv", VideoSize: 1_000_000}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		close(closing)
		server.Close()
	})
	a.url = server.URL + "/manifest.json"
	return a
}

// Opening a title prepares the first two versions a play would read; an
// addon asked again that puts another version first has that one prepared
// too.
func TestPreparationFollowsTheVersionThatComesFirst(t *testing.T) {
	probe := newFakeProbe(t, true)
	addon := newListsAddon(t)
	s := newProbingServer(t, 10, probe.path)
	s.library.SetFollowUps(300*time.Millisecond, 300*time.Millisecond)
	h := heldOn(t, s, addon.url)
	t.Cleanup(func() { h.settle(t) })
	h.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	movie, _ := accounts.ParseID(h.movie)
	analyzed := func(name string) bool {
		versions, _ := h.library.KnownVersions(t.Context(), h.user, movie)
		i := slices.IndexFunc(versions, func(v library.Version) bool { return v.Name == name })
		if i < 0 {
			return false
		}
		_, ok := h.handler.Playback.Analyzed(t.Context(), versions[i].ID)
		return ok
	}

	h.sources(t)
	addon.replies <- []string{"B", "C", "D"}
	eventually(t, "the first two versions to be prepared", func() bool { return analyzed("B") && analyzed("C") })
	h.settle(t)
	if analyzed("D") || probe.runs() != 2 {
		t.Fatalf("%d analyses, the third analyzed: %v", probe.runs(), analyzed("D"))
	}
	// The follow-up lists A first.
	addon.replies <- []string{"A", "B", "C", "D"}
	eventually(t, "the version now first to be prepared", func() bool { return analyzed("A") })
	addon.replies <- []string{"A", "B", "C", "D"}
	h.reaches(t, VersionProgress{Pending: 0, Count: 4}, "A", "B", "C", "D")
	h.settle(t)
	if probe.runs() != 3 || analyzed("D") {
		t.Errorf("%d analyses, the fourth analyzed: %v", probe.runs(), analyzed("D"))
	}
}

// A title whose versions are all new to Polyfin has its first two versions
// analyzed as its page opens.
func TestOpeningATitlePreparesItsFirstTwoVersions(t *testing.T) {
	probe := newFakeProbe(t, true)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { p.settle(t) })
	p.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	versions := p.versionsOf(t, p.remote)
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.remote, p.token, nil)
	p.settle(t)
	for i, version := range versions {
		if _, ok := p.handler.Playback.Analyzed(t.Context(), version.ID); !ok {
			t.Errorf("version %d not analyzed", i+1)
		}
	}
	if probe.runs() != 2 {
		t.Errorf("%d analyses", probe.runs())
	}
}
