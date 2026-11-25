package jellyfin

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
)

// parityRecord is what scripts/jellyfin-fixtures-values.sh keeps of an
// item's answer: the values of parityFields that were sent, whether the
// fields of parityPresence were sent, and the keys of UserData and
// ImageTags.
type parityRecord struct {
	Values       map[string]any  `json:"values"`
	Present      map[string]bool `json:"present"`
	UserDataKeys []string        `json:"userDataKeys"`
	ImageTags    []string        `json:"imageTags"`
}

// parityFields are the fields whose values are compared: they depend on
// the item's type, the user's permissions or the request, and apps branch
// on them (see the script for why each).
var parityFields = []string{"Type", "MediaType", "IsFolder", "CollectionType", "LocationType", "PlayAccess", "IsPlaceHolder",
	"CanDownload", "CanDelete", "EnableMediaSourceDisplay", "VideoType", "ChannelType", "IsSeries", "IsMovie", "IsNews"}

// parityPresence are the fields whose presence alone is compared.
var parityPresence = []string{"MediaSources", "Chapters", "RunTimeTicks", "UserData", "ParentId", "SeriesId", "SeasonId", "AlbumId",
	"ChildCount", "RecursiveItemCount", "MediaSourceCount"}

// recordedParity reads the values Jellyfin gave, by kind, user ("allowed"
// to download or "denied") and answer ("detail" or "listing"), and the
// fields the listings asked for.
func recordedParity(t *testing.T) (map[string]map[string]map[string]parityRecord, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(jellyfinFixtures, "values", "items.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		ListingFields []string                                      `json:"listingFields"`
		Items         map[string]map[string]map[string]parityRecord `json:"items"`
	}
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	return recorded.Items, strings.Join(recorded.ListingFields, ",")
}

// project keeps of an answer what the script keeps of Jellyfin's.
func project(t *testing.T, body []byte) parityRecord {
	t.Helper()
	var item map[string]any
	if err := json.Unmarshal(body, &item); err != nil {
		t.Fatalf("%v in %s", err, body)
	}
	record := parityRecord{Values: map[string]any{}, Present: map[string]bool{}, UserDataKeys: []string{}, ImageTags: []string{}}
	// A null field counts as not sent, as apps take it.
	for _, field := range parityFields {
		if value, ok := item[field]; ok && value != nil {
			record.Values[field] = value
		}
	}
	for _, field := range parityPresence {
		record.Present[field] = item[field] != nil
	}
	for _, key := range []string{"UserData", "ImageTags"} {
		object, _ := item[key].(map[string]any)
		for name := range object {
			if key == "UserData" {
				record.UserDataKeys = append(record.UserDataKeys, name)
			} else {
				record.ImageTags = append(record.ImageTags, name)
			}
		}
	}
	return record
}

// flatten gives each compared field of a record as text: a value, "absent"
// when it was not sent, or whether a field or key was sent.
func (r parityRecord) flatten(userDataKeys, imageTags []string) map[string]string {
	flat := map[string]string{}
	for _, field := range parityFields {
		flat[field] = "absent"
		if value, ok := r.Values[field]; ok {
			encoded, _ := json.Marshal(value)
			flat[field] = string(encoded)
		}
	}
	for _, field := range parityPresence {
		flat["has "+field] = fmt.Sprint(r.Present[field])
	}
	for _, key := range userDataKeys {
		flat["UserData."+key] = fmt.Sprint(slices.Contains(r.UserDataKeys, key))
	}
	for _, key := range imageTags {
		flat["ImageTags."+key] = fmt.Sprint(slices.Contains(r.ImageTags, key))
	}
	return flat
}

// parityGap is a difference with Jellyfin's values: the fields (as
// flatten names them) of the kinds listed, in item details, listings or
// both (where empty), for both users.
type parityGap struct {
	kinds, fields []string
	where         string
	reason        string
}

// parityGaps are the differences of TestItemValuesMatchJellyfin that
// Polyfin keeps on purpose, and why; docs/jellyfin-compatibility.md tells
// those apps notice. Any difference neither here nor in parityOpen fails
// the test, and so does an entry that matches none.
var parityGaps = []parityGap{
	{kinds: []string{"artist", "audiobook", "boxset", "folder", "movie", "recording-episode", "recording-kids", "recording-news",
		"recording-other", "recording-sports", "season", "series", "track", "video",
		"view-boxsets", "view-homevideos", "view-livetv", "view-recordings"},
		fields: []string{"ImageTags.Primary", "ImageTags.Logo", "ImageTags.Thumb"},
		reason: "an item's pictures are those its source gives: an addon's artwork, a collection's drawn from its titles' " +
			"posters, Polyfin's emblems for its own views. Jellyfin's came from TMDB, from frames of its files for videos " +
			"and recordings, and from collages for folders and libraries"},
	{kinds: []string{"video"}, fields: []string{"has RunTimeTicks"},
		reason: "an addon's video has a length once the addon gives one or a play analyzed it; Jellyfin read its file"},
	{kinds: []string{"audiobook"}, fields: []string{"has AlbumId"},
		reason: "the addon gives the book an album, linked as a track's is; Jellyfin's book stands alone"},
	{kinds: []string{"album"}, fields: []string{"has ParentId"},
		reason: "an addon's album sits in no folder of the library; Jellyfin's sits in its artist's folder"},
	{kinds: []string{"view-movies", "view-tvshows", "view-music", "view-books", "view-homevideos"}, fields: []string{"has ChildCount"},
		reason: "an addon's catalog has no known size until it is read whole"},
	{kinds: []string{"playlist"}, fields: []string{"CanDelete"}, where: "listing",
		reason: "a playlist tells CanDelete to its owner and administrators, who alone may delete it; Jellyfin's " +
			"listing tells it to a user it is shared with, while its details do not"},
	{kinds: []string{"program"}, fields: []string{"IsFolder", "LocationType"},
		reason: "Polyfin sends them on every item, false and FileSystem, which is what apps take their absence for"},
	{kinds: []string{"program"}, fields: []string{"MediaType"}, where: "listing",
		reason: "a programme is Video in listings as in its details; Jellyfin's listing by identifier says Unknown"},
	{kinds: []string{"recording-movie", "recording-episode", "recording-news", "recording-sports", "recording-kids", "recording-other"},
		fields: []string{"Type"},
		reason: "Jellyfin 12.4 turns its recordings into library titles (Movie, or Episode within a series), identified with " +
			"pictures and similar titles; Polyfin's recordings have neither, so jellyfin-web would show them worse as " +
			"Movie: they stay Video"},
	{kinds: []string{"recording-episode"}, fields: []string{"has SeriesId", "has SeasonId"},
		reason: "Jellyfin 12.4 files a series' recording under a Series and a Season of its Recordings library; Polyfin's " +
			"recordings folder holds the recordings alone"},
	{kinds: []string{"recording-movie", "recording-news", "recording-episode"}, fields: []string{"IsMovie", "IsNews", "IsSeries"},
		reason: "a recording keeps its programme's flags, which the recordings listing filters by; Jellyfin's recordings " +
			"are items of its library, without them"},
}

// parityOpen are the differences left for a decision, each with what it
// would change; Polyfin answers as before meanwhile.
var parityOpen = []parityGap{}

// parityUser is a user of a parityArea, signed in.
type parityUser struct{ id, token string }

// parityArea is a test server holding some of the kinds compared, by the
// names the script records them under, seen by two members: "allowed" to
// download and "denied".
type parityArea struct {
	testServer
	users map[string]parityUser
	items map[string]string
}

func newParityArea(t *testing.T, s testServer) parityArea {
	t.Helper()
	a := parityArea{testServer: s, users: map[string]parityUser{}, items: map[string]string{}}
	for who, allowed := range map[string]bool{"allowed": true, "denied": false} {
		name := map[bool]string{true: "downloader", false: "keeper"}[allowed]
		user := s.user(name, func(c *accounts.UserChanges) { c.ContentDownloading = new(allowed) })
		a.users[who] = parityUser{id: user.ID.String(), token: s.signIn(name, "web")}
	}
	return a
}

// install installs an addon for everyone.
func (a parityArea) install(t *testing.T, manifest string) addons.Addon {
	t.Helper()
	addon, err := a.addons.Install(t.Context(), addons.Shared(), manifest, false)
	if err != nil {
		t.Fatal(err)
	}
	return addon
}

// views keeps the allowed user's views of the collection types given, as
// the script does, under "view-" and their type.
func (a parityArea) views(t *testing.T, types ...string) {
	t.Helper()
	var views QueryResult
	a.get(t, "/UserViews", a.users["allowed"].token, &views)
	for _, view := range views.Items {
		if key := "view-" + view.CollectionType; slices.Contains(types, view.CollectionType) && a.items[key] == "" {
			a.items[key] = view.Id
		}
	}
}

// first is the identifier of the first item a listing gives the allowed
// user.
func (a parityArea) first(t *testing.T, path string) string {
	t.Helper()
	var page QueryResult
	if status := a.get(t, path, a.users["allowed"].token, &page); status != http.StatusOK || len(page.Items) == 0 {
		t.Fatalf("%s: %d %+v", path, status, page.Items)
	}
	return page.Items[0].Id
}

// answers projects the details and the listing by identifier of each kind,
// by user. The requests go out together, each a goroutine, as an app's
// do; their answers are projected once all are in.
func (a parityArea) answers(t *testing.T, fields string) map[string]map[string]map[string]parityRecord {
	t.Helper()
	type asked struct {
		kind, who, where, path, token string
		status                        int
		body                          []byte
		err                           error
	}
	var requests []*asked
	for kind, id := range a.items {
		for who, user := range a.users {
			requests = append(requests,
				&asked{kind: kind, who: who, where: "detail", path: "/Users/" + user.id + "/Items/" + id, token: user.token},
				&asked{kind: kind, who: who, where: "listing", path: "/Items?userId=" + user.id + "&ids=" + id + "&fields=" + fields, token: user.token})
		}
	}
	var wg sync.WaitGroup
	for _, r := range requests {
		wg.Go(func() {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, a.url+r.path, nil)
			if err != nil {
				r.err = err
				return
			}
			request.Header.Set("Authorization", app("web", r.token))
			response, err := testClient.Do(request)
			if err != nil {
				r.err = err
				return
			}
			defer response.Body.Close()
			r.status = response.StatusCode
			r.body, r.err = io.ReadAll(response.Body)
		})
	}
	wg.Wait()
	answers := map[string]map[string]map[string]parityRecord{}
	for _, r := range requests {
		body := r.body
		if r.where == "listing" && r.err == nil && r.status == http.StatusOK {
			var page struct{ Items []json.RawMessage }
			if json.Unmarshal(r.body, &page) != nil || len(page.Items) != 1 {
				r.status = 0
			} else {
				body = page.Items[0]
			}
		}
		if r.err != nil || r.status != http.StatusOK {
			t.Errorf("%s %s for the %s user: %d %v %s", r.kind, r.where, r.who, r.status, r.err, r.body)
			continue
		}
		if answers[r.kind] == nil {
			answers[r.kind] = map[string]map[string]parityRecord{}
		}
		if answers[r.kind][r.who] == nil {
			answers[r.kind][r.who] = map[string]parityRecord{}
		}
		answers[r.kind][r.who][r.where] = project(t, body)
	}
	return answers
}

// videoParity holds a movie and an episode with their versions, the
// episode's season and series, a collection, a playlist, and their views.
func videoParity(t *testing.T) parityArea {
	t.Helper()
	a := newParityArea(t, newTestServer(t, 10))
	if _, err := a.store.CreateUser(t.Context(), accounts.NewUser{Name: "owner", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	owner := a.signIn("owner", "web")
	titles := a.install(t, catalogAddon(t))
	a.install(t, streamingAddon(t))
	a.install(t, episodeStreams(t))
	if _, err := a.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: titles.ID, CatalogType: "movie", CatalogID: "top"},
		{AddonID: titles.ID, CatalogType: "series", CatalogID: "shows"},
	}); err != nil {
		t.Fatal(err)
	}
	a.views(t, "movies", "tvshows")
	a.items["movie"] = a.first(t, "/Items?ParentId="+a.items["view-movies"]+"&SortBy=SortName")
	a.items["series"] = a.first(t, "/Items?ParentId="+a.items["view-tvshows"])
	a.items["season"] = a.first(t, "/Shows/"+a.items["series"]+"/Seasons")
	a.items["episode"] = a.first(t, "/Shows/"+a.items["series"]+"/Episodes?seasonId="+a.items["season"])
	// Jellyfin knows its files; Polyfin knows the versions once listed.
	for _, user := range a.users {
		id, _ := accounts.ParseID(user.id)
		u, err := a.store.User(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		a.listed(t, u, a.items["movie"], a.items["episode"])
	}

	// The administrator's collection and playlist, the latter shared with
	// both members, as the script makes them.
	a.items["boxset"] = a.createCollection(t, owner, "name=Group&ids="+a.items["movie"])
	a.items["playlist"] = a.createPlaylist(t, owner,
		map[string]any{"Name": "Fixture playlist", "Ids": []string{a.items["movie"], a.items["episode"]}, "MediaType": "Video"})
	for _, user := range a.users {
		a.expectStatus(t, http.StatusNoContent, http.MethodPost, "/Playlists/"+a.items["playlist"]+"/Users/"+user.id, owner,
			map[string]any{"CanEdit": false})
	}
	a.views(t, "boxsets", "playlists")
	return a
}

// homeVideoParity holds a folder of videos, one of its videos, and their
// library's view, as Jellyfin's home videos.
func homeVideoParity(t *testing.T) parityArea {
	t.Helper()
	clip := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(clip, []byte(mediaBytes), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newParityArea(t, newTestServer(t, 10))
	store := a.install(t, newStoreAddon(t, clip).url)
	catalog := store.Manifest.Catalogs[0]
	if _, err := a.addons.SetLibraries(t.Context(), addons.Shared(),
		[]addons.LibraryChoice{{AddonID: store.ID, CatalogType: catalog.Type, CatalogID: catalog.ID}}); err != nil {
		t.Fatal(err)
	}
	var views QueryResult
	a.get(t, "/UserViews", a.users["allowed"].token, &views)
	views.Items = libraryViews(views.Items)
	if len(views.Items) != 1 {
		t.Fatalf("the views: %+v", views.Items)
	}
	a.items["view-homevideos"] = views.Items[0].Id
	a.items["folder"] = a.first(t, "/Items?ParentId="+views.Items[0].Id+"&IncludeItemTypes=Folder")
	a.items["video"] = a.first(t, "/Items?ParentId="+a.items["folder"])
	return a
}

// musicParity holds a song, its album and artist, and the music view;
// with audiobook set, an audiobook and the books view instead.
func musicParity(t *testing.T, audiobook bool) parityArea {
	t.Helper()
	// Binary, as media is: text is taken for a playlist.
	flac := filepath.Join(t.TempDir(), "tone.flac")
	if err := os.WriteFile(flac, []byte("fLaC\x00\x00\x00\x22\x10\x00\x10\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newParityArea(t, newTestServer(t, 10))
	addon := newFakeEclipse(t, flac)
	if audiobook {
		addon.content = "audiobook"
	}
	a.install(t, addon.url)
	var views QueryResult
	a.get(t, "/UserViews", a.users["allowed"].token, &views)
	for _, view := range views.Items {
		switch {
		case audiobook && view.Name == "Top Songs":
			a.items["view-books"] = view.Id
			a.items["audiobook"] = a.first(t, "/Items?ParentId="+view.Id)
		case !audiobook && view.Name == "New Releases":
			a.items["view-music"] = view.Id
			a.items["album"] = a.first(t, "/Items?ParentId="+view.Id+"&IncludeItemTypes=MusicAlbum&Recursive=true&SearchTerm=Sine")
			a.items["track"] = a.first(t, "/Items?ParentId="+a.items["album"])
			a.items["artist"] = a.first(t, "/Artists?ParentId="+view.Id)
		}
	}
	return a
}

// liveParity holds a channel, the programme airing on it, a finished
// recording of each kind of programme the script records, and the Live TV
// view.
func liveParity(t *testing.T) parityArea {
	t.Helper()
	folder := t.TempDir()
	s, _ := recordingServer(t, "ffprobe-not-installed", folder)
	a := newParityArea(t, s)
	a.install(t, newTVAddon(t, true, "").url)
	a.views(t, "livetv")
	a.items["channel"] = a.first(t, "/LiveTv/Channels")
	a.items["program"] = guidePrograms(t, a.testServer, a.users["allowed"].token)["Now"].Id
	// The recordings, their files in the folder, of the guide categories
	// and episode the script's programmes have.
	channel, _ := parseGUID(a.items["channel"])
	for _, kind := range []struct {
		name, genres    string
		season, episode int
		episodeTitle    string
	}{
		{"movie", "{Movie}", 0, 0, ""},
		{"episode", "{Series}", 2, 5, "The Second One"},
		{"news", "{News}", 0, 0, ""},
		{"sports", "{Sports}", 0, 0, ""},
		{"kids", "{Kids}", 0, 0, ""},
		{"other", "{}", 0, 0, ""},
	} {
		var recording accounts.ID
		if err := a.pool.QueryRow(t.Context(), `INSERT INTO live_recordings (channel_id, name, genres, season, episode, episode_title,
			start_at, end_at, started_at, ended_at, status, file)
			VALUES ($1, $2, $3, $4, $5, $6, now() - interval '1 minute', now(), now() - interval '1 minute', now(), 'Completed', $2 || '.mkv')
			RETURNING id`, channel, kind.name, kind.genres, kind.season, kind.episode, kind.episodeTitle).Scan(&recording); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, kind.name+".mkv"), []byte(mediaBytes), 0o600); err != nil {
			t.Fatal(err)
		}
		a.items["recording-"+kind.name] = strings.ReplaceAll(recording.String(), "-", "")
	}
	// Jellyfin's Recordings library, which has no type, is Polyfin's
	// recordings folder.
	a.items["view-recordings"] = a.first(t, "/LiveTv/Recordings/Folders")
	return a
}

// TestItemValuesMatchJellyfin compares, field by field, the values apps
// branch on that Jellyfin gave for each kind of item, in its details
// and in a listing, to a user allowed to download and one who is not
// (recorded by scripts/jellyfin-fixtures-values.sh), with Polyfin's for
// the same kinds. A difference fails unless parityGaps or parityOpen
// lists it.
func TestItemValuesMatchJellyfin(t *testing.T) {
	t.Parallel()
	recorded, fields := recordedParity(t)
	areas := []func(*testing.T) parityArea{
		videoParity, homeVideoParity, liveParity,
		func(t *testing.T) parityArea { return musicParity(t, false) },
		func(t *testing.T) parityArea { return musicParity(t, true) },
	}
	// The areas answer one after the other, on servers of their own, in
	// this test: subtests run in parallel would each wait for a free slot
	// behind the package's other parallel tests, which held this test far
	// longer under load than its own work takes.
	answers := make([]map[string]map[string]map[string]parityRecord, len(areas))
	for i, area := range areas {
		answers[i] = area(t).answers(t, fields)
	}
	got := map[string]map[string]map[string]parityRecord{}
	for _, area := range answers {
		for kind, answer := range area {
			got[kind] = answer
		}
	}

	gaps := slices.Concat(parityGaps, parityOpen)
	used := make([]bool, len(gaps))
	for _, kind := range slices.Sorted(maps.Keys(recorded)) {
		if _, ok := got[kind]; !ok {
			t.Errorf("%s was recorded but is not compared", kind)
			continue
		}
		for _, who := range []string{"allowed", "denied"} {
			for _, where := range []string{"detail", "listing"} {
				want, have := recorded[kind][who][where], got[kind][who][where]
				if have.Values == nil {
					// Polyfin's answer failed, as reported above.
					continue
				}
				userData := slices.Concat(want.UserDataKeys, have.UserDataKeys)
				images := slices.Concat(want.ImageTags, have.ImageTags)
				wanted, had := want.flatten(userData, images), have.flatten(userData, images)
				for _, field := range slices.Sorted(maps.Keys(wanted)) {
					if wanted[field] == had[field] {
						continue
					}
					gap := slices.IndexFunc(gaps, func(g parityGap) bool {
						return slices.Contains(g.kinds, kind) && slices.Contains(g.fields, field) && (g.where == "" || g.where == where)
					})
					if gap < 0 {
						t.Errorf("%s %s for the %s user: %s is %s, Jellyfin's %s", kind, where, who, field, had[field], wanted[field])
						continue
					}
					used[gap] = true
				}
			}
		}
	}
	for i, gap := range gaps {
		if !used[i] {
			t.Errorf("no difference in %v of %v, listed as one: %s", gap.fields, gap.kinds, gap.reason)
		}
	}
}
