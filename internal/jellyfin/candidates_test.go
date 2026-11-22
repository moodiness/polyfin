package jellyfin

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
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

// waitForEveryVersion has the server's plays wait for every version they
// read before they pass one over, however long a busy machine takes to
// read it (see Handler.patienceEnd).
func (s testServer) waitForEveryVersion() {
	s.handler.patienceEnd = func(time.Duration) <-chan time.Time { return nil }
}

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
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: stremio.Number64(1_000_000 * i)}})
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
	t.Parallel()
	probe, letGo := hungProbe(t)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	p.setting(t, func(settings *accounts.Settings) { settings.AnalysisTimeout = int(hungWait / time.Second) })
	chrome := p.profile(t, "jellyfin-web-chrome")
	started := time.Now()
	answer := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie})
	// An answer that waited for the first version would wait hungWait.
	if elapsed := time.Since(started); elapsed > hungWait/2 {
		t.Errorf("answered after %s", elapsed)
	}
	if len(answer.MediaSources) != 1 || answer.MediaSources[0].Id != p.versions[1].ID.String() || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("the title asked for: %+v", answer)
	}
	// Its source let go, the first version's analysis fails: asked for by
	// its identifier, it is refused at once.
	letGo()
	eventually(t, "the first analysis to fail", func() bool { return p.handler.Playback.Failed(p.versions[0].ID) })
	if asked := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[0].ID.String()}); !asked.refused() {
		t.Errorf("the first version asked for: %+v", asked)
	}
}

// A version whose keyframe index cannot be read cannot be streamed over
// HLS: the next version plays instead, rather than one offered with
// nothing to play.
func TestAVersionThatCannotBeStreamedGivesWay(t *testing.T) {
	t.Parallel()
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
// for, once the first plays converted, among the versions analyzed before
// alone: when they all need a conversion, the first plays, and no other is
// analyzed for it.
func TestPreferDirectPlayAnalyzesNoVersionToSaveAConversion(t *testing.T) {
	t.Parallel()
	probe := newFakeProbe(t, true)
	p := manyOn(t, newProbingServer(t, 10, probe.path), manyVersionsAddon(t, 5, nil))
	for _, version := range p.versions[:3] {
		p.remuxableVersion(t, version, true)
	}
	p.setting(t, func(settings *accounts.Settings) { settings.PreferDirectPlay, settings.VersionAttempts = true, 5 })
	source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "minimal"), nil))
	if source.Id != p.movie || !strings.Contains(source.TranscodingUrl, "&allowAudioStreamCopy=false") || probe.runs() != 0 {
		t.Errorf("chosen after %d analyses: %+v", probe.runs(), source)
	}
}

// triesOf returns the versions the last play of a title reached, and what
// came of each.
func (p playbackSetup) triesOf(t *testing.T, item string) []string {
	t.Helper()
	id, _ := accounts.ParseID(item)
	decisions := p.handler.PlayDecisions(id)
	if len(decisions) == 0 {
		t.Fatalf("no play of %s recorded", item)
	}
	var tries []string
	for _, try := range decisions[0].Tries {
		tries = append(tries, try.Version.String()+" "+try.Outcome)
	}
	return tries
}

// sizedVersionsAddon serves one movie, without a runtime, with n streams
// named Version 1 to Version n, files of their own, each announced at the
// size it is served: a source that fails or stays silent leaves the
// others readable, where a host answering one small size for files
// announced larger is told answering an error file (see
// source.ErrorFiles).
func sizedVersionsAddon(t *testing.T, n int) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt4100", Type: "movie", Name: "Movie", Year: "2008"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "sized", Name: "Sized", Version: "1", Types: []string{"movie"},
				IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs: []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt4100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt4100.json":
			var streams []stremio.Stream
			for i := 1; i <= n; i++ {
				file := fmt.Sprintf("Movie.%d.mkv", i)
				streams = append(streams, stremio.Stream{Name: fmt.Sprintf("Version %d", i), URL: server.URL + "/files/" + file,
					BehaviorHints: stremio.StreamBehavior{Filename: file, VideoSize: stremio.Number64(len(mediaBytes) + i)}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/Movie."):
			i, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path, "/files/Movie."), ".mkv"))
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(mediaBytes+strings.Repeat(" ", i)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// A play analyzes the versions one after another: when the first plays,
// it alone is read; when it cannot be read, the second is read after it,
// and the third never.
func TestAPlayAnalyzesTheVersionsInTurn(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		script   string
		chosen   int
		outcomes []string
	}{
		{"the first plays", "exec cat \"$0.json\"\n", 0, []string{TryPlays}},
		{"the first cannot be read", unreadableFirst, 1, []string{TryUnreadable, TryPlays}},
	} {
		probe := scriptedProbe(t, tc.script)
		p := manyOn(t, newProbingServer(t, 10, probe.path), sizedVersionsAddon(t, 3))
		p.waitForEveryVersion()
		source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil))
		var want []string
		for n, outcome := range tc.outcomes {
			want = append(want, p.versions[n].ID.String()+" "+outcome)
		}
		if got := p.triesOf(t, p.movie); source.ETag != p.versions[tc.chosen].ID.String() || !slices.Equal(got, want) {
			t.Errorf("%s: chose %s after %v, want %v", tc.name, source.ETag, got, want)
		}
	}
}

// A version that stays silent for versionPatience has the next start
// alongside it; once that one plays, the play takes it at once, without
// waiting for the silent one again: a fallback takes no longer than when
// both were read at once.
func TestAVersionAfterASilentOneIsNotHeldUp(t *testing.T) {
	t.Parallel()
	probe, _ := hungProbe(t)
	p := manyOn(t, newProbingServer(t, 10, probe.path), sizedVersionsAddon(t, 3))
	p.setting(t, func(settings *accounts.Settings) { settings.AnalysisTimeout = int(hungWait / time.Second) })
	// The first wait ends as the first version's silence would, once its
	// analysis hangs in the probe's first run, so that the second version's
	// is never that run; the others never end.
	var waits atomic.Int32
	p.handler.patienceEnd = func(time.Duration) <-chan time.Time {
		ended := make(chan time.Time, 1)
		if waits.Add(1) == 1 {
			go func() {
				for probe.runs() < 1 {
					time.Sleep(5 * time.Millisecond)
				}
				ended <- time.Time{}
			}()
		}
		return ended
	}
	source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil))
	want := []string{p.versions[0].ID.String() + " " + TryPassed, p.versions[1].ID.String() + " " + TryPlays}
	if got := p.triesOf(t, p.movie); source.ETag != p.versions[1].ID.String() || !slices.Equal(got, want) {
		t.Errorf("chose %s after %v, want %v", source.ETag, got, want)
	}
	// The first version's silence and the second's: no wait once it plays.
	if got := waits.Load(); got != 2 {
		t.Errorf("%d waits", got)
	}
}

// PlaybackInfo waits for the addons' subtitles a second at most once the
// versions are in: those that come later reach the next PlaybackInfo.
func TestPlaybackInfoWaitsForSubtitlesASecondAtMost(t *testing.T) {
	t.Parallel()
	subtitles := make(chan struct{})
	p := manyOn(t, newTestServer(t, 10), manyVersionsAddon(t, 1, subtitles))
	p.analyzed(t, p.versions[0], "h264-aac-mp4")
	chrome := p.profile(t, "jellyfin-web-chrome")
	hasSubtitle := func(source MediaSourceInfo) bool {
		return slices.ContainsFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Type == "Subtitle" && s.IsExternal })
	}
	started := time.Now()
	source := firstSource(t, p.ask(t, p.token, p.movie, chrome, nil))
	// The subtitles answer only once let go, below: a play that waited for
	// them would wait for their request's 15-second timeout.
	if elapsed := time.Since(started); elapsed < subtitleGrace || elapsed > 7500*time.Millisecond {
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

// A title page is told of its versions on the user's jellyfin-web sockets as
// the addons answer, without asking.
func TestVersionsArePushedToTheTitlePage(t *testing.T) {
	t.Parallel()
	addon := newHeldAddon(t, "Alpha", false)
	h := heldOn(t, newTestServer(t, 10), addon.url)
	socket := h.openWebSocket(t, "member")
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
			if pushed.Pending == 0 && pushed.Count == 2 && pushed.Known == 2 {
				if got := h.progress(t); got != (VersionProgress{Pending: 0, Count: 2, Known: 2}) {
					t.Errorf("asked once pushed: %+v", got)
				}
				return
			}
		case <-deadline:
			t.Fatal("no push of the versions")
		}
	}
}

// The versions are pushed to jellyfin-web alone: the Jellyfin SDKs other
// apps are built on fail on a message type they do not know, and some of
// these apps crash on it. The user's data, changed once jellyfin-web had
// the versions, then reaches both: the other app got none before it.
func TestVersionsArePushedToTheWebPlayerOnly(t *testing.T) {
	t.Parallel()
	addon := newHeldAddon(t, "Alpha", false)
	h := heldOn(t, newTestServer(t, 10), addon.url)
	other, _, err := h.openSocket(t, h.token)
	if err != nil {
		t.Fatal(err)
	}
	web := h.openWebSocket(t, "member")
	// The other app opens the title's page.
	h.sources(t)
	addon.answer()
	for deadline, pushed := time.After(5*time.Second), false; !pushed; {
		select {
		case message := <-web.messages:
			data, _ := message.Data.(map[string]any)
			pushed = message.MessageType == versionsMessage && data["Count"] == float64(2)
		case <-deadline:
			t.Fatal("no push of the versions to jellyfin-web")
		}
	}
	if status, body := h.call(http.MethodPost, "/UserPlayedItems/"+h.movie, app("tv", h.token), nil); status != http.StatusOK {
		t.Fatalf("marked played: %d %s", status, body)
	}
	for kind := ""; kind != "UserDataChanged"; {
		message := other.next(t)
		if kind = message.MessageType; kind == versionsMessage {
			t.Fatalf("the other app was told %+v", message.Data)
		}
	}
}

// A title's progress, asked, asked again or pushed, tells whether the web
// player adds the versions that come to an open page, as the setting says.
func TestVersionsTellWhetherTheWebPlayerAddsThem(t *testing.T) {
	t.Parallel()
	addon := newHeldAddon(t, "Alpha", false)
	h := heldOn(t, newTestServer(t, 10), addon.url)
	socket := h.openWebSocket(t, "member")
	adds := func() any {
		var answer map[string]any
		if status := h.get(t, "/Polyfin/Items/"+h.movie+"/Versions", h.token, &answer); status != http.StatusOK {
			t.Fatalf("progress: %d", status)
		}
		return answer["AddVersions"]
	}
	h.sources(t)
	if got := adds(); got != true {
		t.Errorf("by default: %v", got)
	}
	h.setting(t, func(s *accounts.Settings) { s.AddVersionsToOpenPage = false })
	if got := adds(); got != false {
		t.Errorf("turned off: %v", got)
	}
	addon.answer()
	deadline := time.After(5 * time.Second)
	for pushed := false; !pushed; {
		select {
		case message := <-socket.messages:
			if message.MessageType != versionsMessage {
				continue
			}
			data, _ := json.Marshal(message.Data)
			var push map[string]any
			if err := json.Unmarshal(data, &push); err != nil {
				t.Fatal(err)
			}
			if push["Count"] == float64(2) {
				if push["AddVersions"] != false {
					t.Errorf("pushed: %v", push)
				}
				pushed = true
			}
		case <-deadline:
			t.Fatal("no push of the versions")
		}
	}
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+"/Polyfin/Items/"+h.movie+"/Versions/Search", nil)
	request.Header.Set("Authorization", app("tv", h.token))
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var searched map[string]any
	if err := json.NewDecoder(response.Body).Decode(&searched); err != nil || response.StatusCode != http.StatusOK || searched["AddVersions"] != false {
		t.Errorf("asked again: %d %v %v", response.StatusCode, searched, err)
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
			// The size announced is the one served: the same small file for
			// versions announced far larger is a host's error file.
			for i, name := range names {
				streams[i] = stremio.Stream{Name: name, URL: server.URL + "/files/" + name + ".mkv",
					BehaviorHints: stremio.StreamBehavior{Filename: name + ".mkv", VideoSize: stremio.Number64(len(mediaBytes))}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(mediaBytes))
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

// Opening a title prepares the version a play would read first; an addon
// asked again that puts another version first has that one prepared too.
func TestPreparationFollowsTheVersionThatComesFirst(t *testing.T) {
	t.Parallel()
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
	eventually(t, "the first version to be prepared", func() bool { return analyzed("B") })
	h.settle(t)
	if analyzed("C") || analyzed("D") || probe.runs() != 1 {
		t.Fatalf("%d analyses, the next analyzed: %v %v", probe.runs(), analyzed("C"), analyzed("D"))
	}
	// The follow-up lists A first.
	addon.replies <- []string{"A", "B", "C", "D"}
	eventually(t, "the version now first to be prepared", func() bool { return analyzed("A") })
	addon.replies <- []string{"A", "B", "C", "D"}
	h.reaches(t, VersionProgress{Pending: 0, Count: 4, Known: 4}, "A", "B", "C", "D")
	h.settle(t)
	if probe.runs() != 2 || analyzed("C") || analyzed("D") {
		t.Errorf("%d analyses, the next analyzed: %v %v", probe.runs(), analyzed("C"), analyzed("D"))
	}
}

// A title whose versions are all new to Polyfin has the first analyzed as
// its page opens, and the next left for a play to read should the first
// not play.
func TestOpeningATitlePreparesItsFirstVersionAlone(t *testing.T) {
	t.Parallel()
	probe := newFakeProbe(t, true)
	p := playingOn(t, newProbingServer(t, 10, probe.path))
	t.Cleanup(func() { p.settle(t) })
	p.setting(t, func(settings *accounts.Settings) { settings.PrepareAhead = true })
	versions := p.versionsOf(t, p.remote)
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.remote, p.token, nil)
	p.settle(t)
	for i, version := range versions {
		if _, ok := p.handler.Playback.Analyzed(t.Context(), version.ID); ok != (i == 0) {
			t.Errorf("version %d analyzed: %v", i+1, ok)
		}
	}
	if probe.runs() != 1 {
		t.Errorf("%d analyses", probe.runs())
	}
}
