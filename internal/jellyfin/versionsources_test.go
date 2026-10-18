package jellyfin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// relayAddon serves the movie tt1000 with streams named after the words
// given, each labeled by its description, and a subtitle file named after
// the addon; with a catalog listing the movie when catalog is set.
func relayAddon(t *testing.T, name string, catalog bool, streams ...stremio.Stream) string {
	t.Helper()
	var server *httptest.Server
	movie := stremio.Meta{ID: "tt1000", Type: "movie", Name: "Movie", Runtime: "2h", Year: "2008"}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			manifest := stremio.Manifest{ID: "relay." + strings.ToLower(strings.ReplaceAll(name, " ", ".")), Name: name, Version: "1",
				Types: []string{"movie"}, IDPrefixes: []string{"tt"}, Resources: []stremio.Resource{{Name: "stream"}, {Name: "subtitles"}}}
			if catalog {
				manifest.Resources = append(manifest.Resources, stremio.Resource{Name: "catalog"}, stremio.Resource{Name: "meta"})
				manifest.Catalogs = []stremio.Catalog{{Type: "movie", ID: "top", Name: "Top"}}
			}
			_ = json.NewEncoder(w).Encode(manifest)
		case strings.HasPrefix(path, "/catalog/movie/top"):
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": []stremio.Meta{movie}})
		case path == "/meta/movie/tt1000.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": movie})
		case path == "/stream/movie/tt1000.json":
			listed := slices.Clone(streams)
			for i := range listed {
				listed[i].URL = server.URL + "/files/" + listed[i].BehaviorHints.Filename
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"streams": listed})
		case strings.HasPrefix(path, "/subtitles/movie/tt1000"):
			_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{
				{ID: stremio.Text(name), Lang: "fre", URL: server.URL + "/subtitles/" + strings.ReplaceAll(name, " ", "") + ".srt"}}})
		case strings.HasPrefix(path, "/files/"):
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader("\x1a\x45\xdf\xa3 media bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// relayStream is a stream named word, labeled by description.
func relayStream(word, description string) stremio.Stream {
	return stremio.Stream{Name: word, Description: description, BehaviorHints: stremio.StreamBehavior{Filename: word + ".mkv", VideoSize: 4e9}}
}

// sourcesSetup is the movie tt1000 with versions from Relay One, Relay
// Two and the folder Shelf, installed in that order, as members see it.
type sourcesSetup struct {
	rulesSetup
	one, two, shelf addons.Addon
}

func versionSourcesOn(t *testing.T) sourcesSetup {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.DiscardHandler), o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
	})
	user := s.user("member", nil)
	install := func(manifestURL string) addons.Addon {
		t.Helper()
		addon, err := s.addons.Install(t.Context(), addons.Shared(), manifestURL, false)
		if err != nil {
			t.Fatal(err)
		}
		return addon
	}
	p := sourcesSetup{
		one: install(relayAddon(t, "Relay One", true, relayStream("OneEnglish", "English 1080p"), relayStream("OneFrench", "FRENCH 1080p"))),
		two: install(relayAddon(t, "Relay Two", false, relayStream("TwoFrench", "FRENCH 720p"), relayStream("TwoEnglish", "English 720p"))),
	}
	file, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Movie (2008) 1080p.mkv"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	if p.shelf, err = folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies}); err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), p.shelf.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), p.shelf.ID, "Movie (2008) 1080p.mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	var views QueryResult
	s.get(t, "/UserViews", token, &views)
	for _, view := range views.Items {
		var page QueryResult
		if s.get(t, "/Items?ParentId="+view.Id, token, &page); view.Name != "Shelf" && len(page.Items) == 1 {
			p.rulesSetup = rulesSetup{playbackSetup: playbackSetup{testServer: s, token: token, user: user, movie: page.Items[0].Id}}
		}
	}
	if p.movie == "" {
		t.Fatalf("no movie in %+v", views.Items)
	}
	movie, _ := accounts.ParseID(p.movie)
	if p.versions, err = s.library.Versions(t.Context(), user, movie); err != nil || len(p.versions) != 5 {
		t.Fatalf("versions: %+v %v", p.versions, err)
	}
	return p
}

// The source criterion groups versions by their source: the sources of
// Order of sources first, in its order, then the others in the order of
// Content › Sources, the next criteria sorting each source's versions.
// Without it, versions keep the order of Content › Sources whatever Order
// of sources says. Placed after the language, it orders the versions the
// language ties on: a local file of unknown language comes after the
// versions labeled with the user's language even with its folder placed
// first.
func TestVersionSourceOrder(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	french, frenchToken := p.another(t, "french", "fre")
	check := func(name string, user accounts.User, token string, want ...string) {
		t.Helper()
		if got := firstWords(p.detailed(t, user, token)); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	addonOrder := []string{"OneEnglish", "OneFrench", "TwoFrench", "TwoEnglish", "Shelf"}
	check("without rules", french, frenchToken, addonOrder...)
	p.setting(t, func(s *accounts.Settings) { s.VersionSourceOrder = []accounts.ID{p.shelf.ID, p.two.ID, p.one.ID} })
	check("an order of sources without the criterion", french, frenchToken, addonOrder...)
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"language"} })
	check("the language without the criterion", french, frenchToken, "OneFrench", "TwoFrench", "Shelf", "OneEnglish", "TwoEnglish")

	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"source", "language"}
		s.VersionSourceOrder = []accounts.ID{p.two.ID, p.one.ID}
	})
	check("Relay Two placed first", french, frenchToken, "TwoFrench", "TwoEnglish", "OneFrench", "OneEnglish", "Shelf")
	// The first source carries the title's identifier, as without rules.
	if sources := p.detailed(t, french, frenchToken); sources[0].Id != p.movie || sources[0].ETag != p.versions[2].ID.String() {
		t.Errorf("identifiers: %s %s", sources[0].Id, sources[0].ETag)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionSourceOrder = []accounts.ID{p.shelf.ID} })
	check("the folder placed first", french, frenchToken, "Shelf", "OneFrench", "OneEnglish", "TwoFrench", "TwoEnglish")
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"language", "source"}
		s.VersionSourceOrder = []accounts.ID{p.two.ID, p.shelf.ID}
	})
	check("the criterion after the language", french, frenchToken, "TwoFrench", "OneFrench", "Shelf", "TwoEnglish", "OneEnglish")

	// Descriptions aside, the addons' subtitles keep the order of
	// Content › Sources.
	movie, _ := accounts.ParseID(p.movie)
	subtitles, err := p.library.Subtitles(t.Context(), french, movie)
	if err != nil {
		t.Fatal(err)
	}
	var from []string
	for _, subtitle := range subtitles {
		from = append(from, subtitle.Addon)
	}
	if !slices.Equal(from, []string{"Relay One", "Relay Two"}) {
		t.Errorf("subtitles: %v", from)
	}
}

// A source installed after Order of sources was saved comes after the
// sources it places, as does a user's own addon, after the server's; a
// source removed leaves it, the others keeping their places.
func TestVersionSourceOrderLaterSources(t *testing.T) {
	t.Parallel()
	p := versionSourcesOn(t)
	// An administrator's own addons may reach the local network, where the
	// fake addon answers.
	member := p.testServer.user("own", func(c *accounts.UserChanges) { c.PersonalAddons, c.IsAdministrator = new(true), new(true) })
	if _, err := p.addons.Install(t.Context(), addons.Personal(member.ID), relayAddon(t, "Own Relay", false, relayStream("OwnFrench", "FRENCH 480p")),
		false); err != nil {
		t.Fatal(err)
	}
	token := p.signIn("own", "tv")
	if status := p.configure(t, "/Users/Configuration", token, map[string]any{"AudioLanguagePreference": "fre"}); status != http.StatusNoContent {
		t.Fatalf("configuration: %d", status)
	}
	p.setting(t, func(s *accounts.Settings) {
		s.VersionOrder = []string{"source", "language"}
		s.VersionSourceOrder = []accounts.ID{p.two.ID, p.one.ID}
	})
	// Item details list the versions known: each addon is asked first.
	movie, _ := accounts.ParseID(p.movie)
	listed := func(user accounts.User) {
		t.Helper()
		if _, err := p.library.Versions(t.Context(), user, movie); err != nil {
			t.Fatal(err)
		}
	}
	listed(member)
	if got := firstWords(p.detailed(t, member, token)); !slices.Equal(got,
		[]string{"TwoFrench", "TwoEnglish", "OneFrench", "OneEnglish", "Shelf", "OwnFrench"}) {
		t.Errorf("a user's own addon: %v", got)
	}
	if _, err := p.addons.Install(t.Context(), addons.Shared(), relayAddon(t, "Relay Three", false, relayStream("ThreeFrench", "FRENCH 2160p")),
		false); err != nil {
		t.Fatal(err)
	}
	if err := p.addons.Remove(t.Context(), addons.Shared(), p.two.ID); err != nil {
		t.Fatal(err)
	}
	french, frenchToken := p.another(t, "french", "fre")
	listed(french)
	if got := firstWords(p.detailed(t, french, frenchToken)); !slices.Equal(got, []string{"OneFrench", "OneEnglish", "Shelf", "ThreeFrench"}) {
		t.Errorf("a source installed later, another removed: %v", got)
	}
}

// The rules' key holds Order of sources: a page open keeps the versions
// it lists in place only while the order stays the same.
func TestVersionRulesKeyHoldsTheSources(t *testing.T) {
	t.Parallel()
	one, two := accounts.ID{1}, accounts.ID{2}
	settings := accounts.DefaultSettings()
	settings.VersionOrder = []string{"source"}
	settings.VersionSourceOrder = []accounts.ID{one, two}
	before := newVersionRules(settings).key()
	settings.VersionSourceOrder = []accounts.ID{two, one}
	if after := newVersionRules(settings).key(); after == before {
		t.Errorf("the same key for two orders of sources: %s", after)
	}
}

// sourceRanks ranks the sources listed first, then the others by the
// first place of their versions.
func TestSourceRanks(t *testing.T) {
	t.Parallel()
	listed, gone, first, second := accounts.ID{1}, accounts.ID{2}, accounts.ID{3}, accounts.ID{4}
	rules := versionRules{sources: []accounts.ID{listed, gone}}
	versions := []library.Version{{Origin: library.Origin{Addon: first}}, {Origin: library.Origin{Addon: second}},
		{Origin: library.Origin{Addon: listed}}, {Origin: library.Origin{Addon: first}}}
	ranks := rules.sourceRanks(versions)
	if ranks[listed] != 0 || ranks[first] != 2 || ranks[second] != 3 {
		t.Errorf("ranks: %v", ranks)
	}
}
