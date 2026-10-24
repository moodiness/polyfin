package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/torrent"
	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// torrentMovieAddon serves one movie, "Harbor Lights", whose only stream
// is a torrent, with no file index.
func torrentMovieAddon(t *testing.T, tor *torrenttest.Torrent, tracker string) string {
	t.Helper()
	movie := stremio.Meta{ID: "tt7100", Type: "movie", Name: "Harbor Lights", Year: "2019"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.Path; {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "subbed", Name: "Subbed", Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
				Catalogs:  []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}})
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt7100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt7100.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": []stremio.Stream{
				{Name: "Torrent", InfoHash: tor.HashHex(), Sources: []string{"tracker:" + tracker}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// torrentServer is a test server playing torrents through a SOCKS5 proxy,
// with a user who may play them and one who may not.
func torrentServer(t *testing.T) (s testServer, allowed accounts.User) {
	t.Helper()
	proxy := torrenttest.StartProxy(t, true, "", "")
	s = newProbingServer(t, 10, "ffprobe-not-installed", func(options *Options, _ *pgxpool.Pool) {
		engine, err := torrent.New(torrent.Config{Settings: func() torrent.Settings { return playback.TorrentSettings(options.Accounts.Settings()) },
			Dir: t.TempDir(), Budget: func() int64 { return 64 << 20 }, Logger: slog.New(slog.DiscardHandler)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(engine.Close)
		options.Playback.Torrents(engine)
	})
	s.setting(t, func(settings *accounts.Settings) {
		settings.TorrentStreams = true
		settings.TorrentProxy = proxy.Address()
	})
	allowed = s.user("allowed", func(c *accounts.UserChanges) { c.TorrentStreams = new(true) })
	s.user("other", nil)
	return s, allowed
}

// externalSubtitles lists the subtitle files a media source offers, by
// their language.
func externalSubtitles(source MediaSourceInfo) map[string]playback.MediaStream {
	files := map[string]playback.MediaStream{}
	for _, stream := range source.MediaStreams {
		if stream.Type == "Subtitle" && stream.IsExternal {
			files[stream.Language] = stream
		}
	}
	return files
}

// fetched reports whether the subtitle file of a torrent at path was
// fetched and kept.
func fetched(t *testing.T, s testServer, path string) bool {
	t.Helper()
	var kept bool
	if err := s.pool.QueryRow(t.Context(), "SELECT data IS NOT NULL FROM torrent_subtitles WHERE path = $1", path).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	return kept
}

// within waits up to a minute for done.
func within(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: timed out", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The subtitle files inside a torrent that go with its video are listed
// for the version from its first play, with the language their names
// tell. The file in the user's preferred language is fetched as the play
// starts, the others only when picked, even once the torrent stopped,
// which starts it again. Once fetched, they are kept: listed on the title's
// page and in the next play, and served, after the torrent stopped and its
// seeder left. A user who may not play torrents gets none of them.
func TestSubtitleFilesInsideATorrent(t *testing.T) {
	t.Parallel()
	const piece = 32 << 10
	srt := func(line string) []byte { return []byte("1\r\n00:00:01,000 --> 00:00:04,000\r\n" + line + "\r\n") }
	english, french, spanish := srt("Hello from the swarm"), srt("Bonjour de l'essaim"), srt("Hola del enjambre")
	files := []torrenttest.File{
		{Name: "Harbor.Lights.2019.1080p.en.srt", Data: english},
		{Name: "Harbor.Lights.2019.1080p.nfo", Data: torrenttest.Bytes(3 * piece)},
		{Name: "Subs/French.srt", Data: french},
		{Name: "Harbor.Lights.2019.1080p.txt", Data: torrenttest.Bytes(3 * piece)},
		{Name: "Harbor.Lights.2019.1080p.es.srt", Data: spanish},
		{Name: "Harbor.Lights.2019.1080p.sample.txt", Data: torrenttest.Bytes(3 * piece)},
		{Name: "Harbor.Lights.2019.1080p.mkv", Data: torrenttest.Bytes(12 * piece)},
	}
	offset := func(name string) (int, int) {
		at := 0
		for _, f := range files {
			if f.Name == name {
				return at, len(f.Data)
			}
			at += len(f.Data)
		}
		t.Fatalf("no file %s", name)
		return 0, 0
	}
	tor := torrenttest.Build("Harbor.Lights.2019.1080p", piece, files...)
	seeder := torrenttest.Seed(t, tor, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	s, user := torrentServer(t)
	if _, err := s.addons.Install(t.Context(), addons.Shared(), torrentMovieAddon(t, tor, tracker.URL()), false); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("allowed", "tv")
	if status := s.configure(t, "/Users/Configuration", token, map[string]any{"SubtitleLanguagePreference": "eng", "SubtitleMode": "Always"}); status != http.StatusNoContent {
		t.Fatalf("configuration: %d", status)
	}
	var views, page QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	movie := page.Items[0].Id
	version := versionNamed(t, s, user, movie, "Torrent")
	p := playbackSetup{testServer: s, token: token, user: user}
	p.analyzed(t, version, "h264-aac-mp4")
	details := func() map[string]playback.MediaStream {
		t.Helper()
		var item BaseItemDto
		if status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+movie, token, &item); status != http.StatusOK || item.MediaSources == nil ||
			len(*item.MediaSources) != 1 {
			t.Fatalf("details: %d", status)
		}
		return externalSubtitles((*item.MediaSources)[0])
	}
	if listed := details(); len(listed) != 0 {
		t.Fatalf("subtitle files listed before the first play: %v", listed)
	}
	play := func(tracks map[string]any) MediaSourceInfo {
		t.Helper()
		body := map[string]any{"UserId": user.ID.String(), "MediaSourceId": version.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")}
		for name, value := range tracks {
			body[name] = value
		}
		status, data := s.call(http.MethodPost, "/Items/"+movie+"/PlaybackInfo", app("tv", token), body)
		var info playbackInfoResponse
		if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		return info.MediaSources[0]
	}

	// The first play lists the three files, with their names' languages,
	// shows the English one, and fetches it alone. jellyfin-web names the
	// tracks its page showed, the audio and no subtitle, as the version
	// had no file yet: they are not the version's tracks now.
	source := play(map[string]any{"AudioStreamIndex": 1, "SubtitleStreamIndex": -1})
	listed := externalSubtitles(source)
	if len(listed) != 3 || listed["eng"].Codec != "subrip" || listed["fra"].Codec != "subrip" || listed["spa"].Codec != "subrip" {
		t.Fatalf("listed in PlaybackInfo: %+v", listed)
	}
	if source.DefaultSubtitleStreamIndex == nil || *source.DefaultSubtitleStreamIndex != listed["eng"].Index {
		t.Errorf("shown: %v, not the English file %d", deref(source.DefaultSubtitleStreamIndex), listed["eng"].Index)
	}
	if audio := source.DefaultAudioStreamIndex; audio == nil || !slices.ContainsFunc(source.MediaStreams, func(s playback.MediaStream) bool {
		return s.Index == *audio && s.Type == "Audio"
	}) {
		t.Errorf("the audio played: %v, not the version's audio track", deref(audio))
	}
	within(t, "the English file to be fetched", func() bool { return fetched(t, s, "Harbor.Lights.2019.1080p/Harbor.Lights.2019.1080p.en.srt") })
	for _, name := range []string{"Subs/French.srt", "Harbor.Lights.2019.1080p.es.srt"} {
		if at, length := offset(name); seeder.Asked(at, length) {
			t.Errorf("%s was fetched unpicked", name)
		}
	}
	text := func(stream playback.MediaStream, token string) (int, string) {
		t.Helper()
		address := strings.Replace(stream.DeliveryUrl, "ApiKey="+p.token, "ApiKey="+token, 1)
		return s.subtitleText(t, address)
	}
	if status, body := text(listed["eng"], token); status != http.StatusOK || !strings.Contains(body, "Hello from the swarm") {
		t.Errorf("the English file: %d %q", status, body)
	}

	// Picked, the French file is fetched and served.
	if status, body := text(listed["fra"], token); status != http.StatusOK || !strings.Contains(body, "Bonjour de l'essaim") {
		t.Errorf("the French file: %d %q", status, body)
	}
	// A user who may not play torrents gets none.
	if status, body := text(listed["fra"], s.signIn("other", "tv")); status == http.StatusOK {
		t.Errorf("the French file was served to a user who may not play torrents: %q", body)
	}

	// Picked once the torrent stopped, the Spanish file starts it again.
	s.handler.Playback.EndTorrent(tor.HashHex())
	within(t, "the torrent to stop", func() bool { return !s.handler.Playback.TorrentRunning(tor.HashHex()) })
	if status, body := text(listed["spa"], token); status != http.StatusOK || !strings.Contains(body, "Hola del enjambre") {
		t.Errorf("the Spanish file: %d %q", status, body)
	}

	// The torrent stopped, its seeder gone, the files stay listed and are
	// served as kept.
	seeder.Close()
	s.handler.Playback.EndTorrent(tor.HashHex())
	within(t, "the torrent to stop", func() bool { return !s.handler.Playback.TorrentRunning(tor.HashHex()) })
	if listed := details(); len(listed) != 3 {
		t.Errorf("listed on the title's page: %+v", listed)
	}
	// From then on, the tracks the app names are the version's: the French
	// file picked on its page stays.
	next := play(map[string]any{"SubtitleStreamIndex": listed["fra"].Index})
	if again := externalSubtitles(next); len(again) != 3 || again["fra"].Index != listed["fra"].Index ||
		next.DefaultSubtitleStreamIndex == nil || *next.DefaultSubtitleStreamIndex != listed["fra"].Index {
		t.Errorf("the next play: %+v, shown %v", again, deref(next.DefaultSubtitleStreamIndex))
	}
	kept := s.handler.Playback.TorrentSubtitles(t.Context(), version)
	if len(kept) != 3 {
		t.Fatalf("kept: %+v", kept)
	}
	for _, file := range kept {
		if data, err := s.handler.Playback.TorrentSubtitle(t.Context(), file); err != nil || !slices.ContainsFunc([][]byte{english, french, spanish},
			func(b []byte) bool { return string(b) == string(data) }) {
			t.Errorf("the %s file once the torrent stopped: %q %v", file.Language, data, err)
		}
	}
	if s.handler.Playback.TorrentRunning(tor.HashHex()) {
		t.Error("the files kept started the torrent")
	}
}

// In a season's pack, the subtitle files listed are those of the episode
// played alone.
func TestSubtitleFilesOfTheEpisodeInASeasonPack(t *testing.T) {
	t.Parallel()
	srt := []byte("1\r\n00:00:01,000 --> 00:00:04,000\r\nLine\r\n")
	pack := torrenttest.Build("Harbor.Tales.S01.1080p", 32<<10,
		torrenttest.File{Name: "Harbor.Tales.S01E01.1080p.mkv", Data: torrenttest.Bytes(90 << 10)},
		torrenttest.File{Name: "Harbor.Tales.S01E02.1080p.mkv", Data: torrenttest.Bytes(110 << 10)},
		torrenttest.File{Name: "Harbor.Tales.S01E01.1080p.en.srt", Data: srt},
		torrenttest.File{Name: "Harbor.Tales.S01E02.1080p.en.srt", Data: srt},
		torrenttest.File{Name: "Subs/Harbor.Tales.S01E01.1080p/2_French.srt", Data: srt},
		torrenttest.File{Name: "Subs/Harbor.Tales.S01E02.1080p/2_French.srt", Data: srt},
		torrenttest.File{Name: "Subs/Harbor.Tales.S01E02.1080p/3_German.srt", Data: srt},
		torrenttest.File{Name: "Subs/Spanish.srt", Data: srt})
	seeder := torrenttest.Seed(t, pack, "127.0.0.1")
	tracker := torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address())
	s, user := torrentServer(t)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), packAddon(t, pack, tracker.URL()), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("allowed", "tv")
	var views, page, listed QueryResult
	s.get(t, "/UserViews", token, &views)
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &page)
	s.get(t, "/Shows/"+page.Items[0].Id+"/Episodes", token, &listed)
	i := slices.IndexFunc(listed.Items, func(item BaseItemDto) bool { return item.Name == "Second" })
	if i < 0 {
		t.Fatalf("episodes: %v", listed.Items)
	}
	episode := listed.Items[i].Id
	id, _ := accounts.ParseID(episode)
	versions, err := s.library.Versions(t.Context(), user, id)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions %+v %v", versions, err)
	}
	p := playbackSetup{testServer: s, token: token, user: user}
	p.analyzed(t, versions[0], "h264-aac-mp4")
	status, data := s.call(http.MethodPost, "/Items/"+episode+"/PlaybackInfo", app("tv", token), map[string]any{"UserId": user.ID.String(),
		"MediaSourceId": versions[0].ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil || len(info.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	var languages []string
	for _, stream := range info.MediaSources[0].MediaStreams {
		if stream.Type == "Subtitle" && stream.IsExternal {
			languages = append(languages, stream.Language)
		}
	}
	if strings.Join(languages, ",") != "eng,fra,deu" {
		t.Errorf("listed for the second episode: %v", languages)
	}
	files := s.handler.Playback.TorrentSubtitles(t.Context(), versions[0])
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Torrent.Torrent.InfoHash+" "+strings.TrimPrefix(fileNamed(t, s, f), "Harbor.Tales.S01.1080p/"))
	}
	want := []string{pack.HashHex() + " Harbor.Tales.S01E02.1080p.en.srt", pack.HashHex() + " Subs/Harbor.Tales.S01E02.1080p/2_French.srt",
		pack.HashHex() + " Subs/Harbor.Tales.S01E02.1080p/3_German.srt"}
	if !slices.Equal(paths, want) {
		t.Errorf("files of the second episode: %q", paths)
	}
}

// fileNamed is the path in its torrent of a subtitle file inside one.
func fileNamed(t *testing.T, s testServer, file library.ExternalSubtitle) string {
	t.Helper()
	var path string
	if err := s.pool.QueryRow(t.Context(), "SELECT path FROM torrent_subtitles WHERE version_id = $1 AND file_index = $2",
		file.Torrent.Version, file.Torrent.Index).Scan(&path); err != nil {
		t.Fatal(err)
	}
	return path
}
