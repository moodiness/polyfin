package localfiles

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/userdata"
)

// libraryOf is the library of a folder: its identifier and its catalog's
// type.
func (e env) libraryOf(id accounts.ID) (accounts.ID, string) {
	e.t.Helper()
	libraries, err := e.addons.Libraries(e.t.Context(), addons.Shared())
	if err != nil {
		e.t.Fatal(err)
	}
	for _, l := range libraries {
		if l.AddonID == id && l.Enabled {
			return library.LibraryID(l), l.Catalog.Type
		}
	}
	e.t.Fatalf("no library of %s: %+v", id, libraries)
	return accounts.ID{}, ""
}

// A folder that comes to hold something else keeps its addon and its
// library, with their identifiers, and is scanned as the new kind. Links
// made by hand and episodes given to discs go. What users did with its
// home videos follows them to the movies they are matched to, as an
// import adds it, and back from the movies; a file matched later gets it
// then. A resume point moves with its file; played marks and favorites
// are given and stay. Turned back to home videos, its videos are the same
// titles.
func TestFolderKindChanges(t *testing.T) {
	e := newEnv(t)
	addon, _ := e.folder(KindHomeVideos, map[string]int{"Nosferatu (1922).mkv": 10, "The General (1926).mkv": 11, "Beach.mkv": 12})
	users, err := accounts.Open(t.Context(), e.pool)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}
	data := userdata.New(e.pool)
	set := func(item accounts.ID, change func(*userdata.Data)) {
		t.Helper()
		if _, err := data.Change(t.Context(), user.ID, []userdata.Item{{ID: item}}, change); err != nil {
			t.Fatal(err)
		}
	}
	get := func(item accounts.ID) userdata.Data {
		t.Helper()
		got, err := data.Get(t.Context(), user.ID, []accounts.ID{item})
		if err != nil {
			t.Fatal(err)
		}
		return got[item]
	}
	videos := e.files(addon.ID)
	video := func(rel string) accounts.ID { return library.TitleItemID(library.KindVideo, videos[rel]) }
	movie := func(imdb string) accounts.ID { return library.TitleItemID(library.KindMovie, imdb) }
	older := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	newer := older.Add(48 * time.Hour)
	set(video("Nosferatu (1922).mkv"), func(d *userdata.Data) { d.MarkPlayed(&older, older) })
	set(video("The General (1926).mkv"), func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 10*time.Minute, time.Hour, &older })
	set(movie("tt0017925"), func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 30*time.Minute, time.Hour, &newer })
	set(video("Beach.mkv"), func(d *userdata.Data) { d.Favorite = true })
	libraryID, catalogType := e.libraryOf(addon.ID)
	if catalogType != library.VideoType {
		t.Fatalf("the library's catalog: %s", catalogType)
	}

	changed, err := e.service.Update(t.Context(), addon.ID, Changes{Kind: new(KindMovies)})
	if err != nil || changed.ID != addon.ID {
		t.Fatalf("to movies: %+v %v", changed, err)
	}
	e.idle(addon.ID)
	if id, typ := e.libraryOf(addon.ID); id != libraryID || typ != "movie" {
		t.Errorf("the library as movies: %s %s, was %s", id, typ, libraryID)
	}
	if files := e.files(addon.ID); files["Nosferatu (1922).mkv"] != "tt0013442" || files["The General (1926).mkv"] != "tt0017925" ||
		!strings.HasPrefix(files["Beach.mkv"], "unmatched") {
		t.Fatalf("as movies: %v", files)
	}
	if d := get(movie("tt0013442")); !d.Played {
		t.Errorf("a played home video's movie: %+v", d)
	}
	if d := get(movie("tt0017925")); d.Position != 30*time.Minute {
		t.Errorf("the movie's newer resume point: %+v", d)
	}
	if d := get(video("The General (1926).mkv")); d.Position != 0 {
		t.Errorf("the home video's resume point, moved: %+v", d)
	}
	if d := get(video("Nosferatu (1922).mkv")); !d.Played {
		t.Errorf("the home video's played mark, given: %+v", d)
	}
	if d := get(movie("tt0015864")); d.Favorite {
		t.Errorf("before the link: %+v", d)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Beach.mkv", "tt0015864"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if d := get(movie("tt0015864")); !d.Favorite {
		t.Errorf("a file matched later: %+v", d)
	}
	if d := get(video("Beach.mkv")); !d.Favorite {
		t.Errorf("the home video's favorite, given: %+v", d)
	}

	// The movie is played meanwhile, another one resumed; a disc has
	// episodes.
	set(movie("tt0017925"), func(d *userdata.Data) { d.MarkPlayed(&newer, newer) })
	set(movie("tt0015864"), func(d *userdata.Data) { d.Position, d.Runtime, d.LastPlayed = 20*time.Minute, time.Hour, &newer })
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO local_disc_episodes (addon_id, path, season, episode, last_episode)
		VALUES ($1, 'Show/Disc 1/VIDEO_TS#1', 1, 1, 1)`, addon.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Kind: new(KindHomeVideos)}); err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	if id, typ := e.libraryOf(addon.ID); id != libraryID || typ != library.VideoType {
		t.Errorf("the library as home videos again: %s %s, was %s", id, typ, libraryID)
	}
	if again := e.files(addon.ID); again["Nosferatu (1922).mkv"] != videos["Nosferatu (1922).mkv"] ||
		again["The General (1926).mkv"] != videos["The General (1926).mkv"] || again["Beach.mkv"] != videos["Beach.mkv"] {
		t.Errorf("the videos again: %v, were %v", again, videos)
	}
	if d := get(video("The General (1926).mkv")); !d.Played {
		t.Errorf("a played movie's home video: %+v", d)
	}
	if d := get(video("Beach.mkv")); d.Position != 20*time.Minute || !d.Favorite {
		t.Errorf("a resumed movie's home video: %+v", d)
	}
	if d := get(movie("tt0015864")); d.Position != 0 || !d.Favorite {
		t.Errorf("the movie's resume point, moved: %+v", d)
	}
	if d := get(movie("tt0017925")); !d.Played {
		t.Errorf("the movie's played mark, given: %+v", d)
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || len(folder.Links) != 0 || len(folder.Episodes) != 0 || folder.Kind != KindHomeVideos {
		t.Errorf("links and episodes: %+v %v", folder, err)
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{Kind: new("films")}); err != ErrInvalidKind {
		t.Errorf("another kind: %v", err)
	}
}

// A change of what a folder holds stops its scan under way, and scans it
// again as the new kind.
func TestFolderKindChangeStopsTheScan(t *testing.T) {
	e := newEnv(t)
	sealer, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = sealer
	nas := t.TempDir()
	write(t, filepath.Join(nas, "shelf", "Nosferatu (1922).mkv"), 10)
	share := serveWebDAV(t, nas, "reader", "pass phrase")
	// Once holding, the share answers no listing until the scan asking
	// stops.
	held, stop := make(chan struct{}), make(chan struct{})
	var holding atomic.Bool
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" && holding.CompareAndSwap(true, false) {
			close(held)
			select {
			case <-r.Context().Done():
			case <-stop:
			}
			return
		}
		share.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(front.Close)
	t.Cleanup(func() { close(stop) })
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: front.URL + "/shelf/", Kind: KindMusic, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	holding.Store(true)
	e.service.StartScan(addon.ID)
	<-held
	done := make(chan error, 1)
	go func() {
		_, err := e.service.Update(context.WithoutCancel(t.Context()), addon.ID, Changes{Kind: new(KindMovies)})
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the change waited for the scan under way")
	}
	e.idle(addon.ID)
	if files := e.files(addon.ID); files["Nosferatu (1922).mkv"] != "tt0013442" {
		t.Errorf("scanned again as movies: %v", files)
	}
}
