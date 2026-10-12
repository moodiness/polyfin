package jellyfin

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/trackers"
)

// A home videos folder is a homevideos library of Jellyfin apps, of Video
// items as Jellyfin's: listed as jellyfin-web lists such a library,
// searched, played from the file with byte ranges, with their watch state
// and resume point, in collections; hidden by the user's visible libraries
// and by parental control as Other unrated items; never told to tracking
// services.
func TestHomeVideosInJellyfinApps(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	fakes := newTrackerFakes(t)
	var folders *localfiles.Service
	var tracker *trackers.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, o.Logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		folders.UseTools(ffprobe, ffmpeg)
		tracker = trackers.New(trackers.Options{DB: pool, Settings: o.Accounts.Settings, Version: "test", Logger: o.Logger,
			URLs: map[string]string{trackers.MDBList: fakes.url + "/mdblist"}})
		o.Trackers = tracker
	})
	t.Cleanup(tracker.Close)
	tr := trackingOn(t, s)
	member := mustID(t, tr.user)
	if _, err := tracker.ConnectKey(t.Context(), member, trackers.MDBList, "mdblist-member"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	create := func(rel string, args ...string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		args = append([]string{"-v", "error", "-y"}, append(args, path)...)
		if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	// Six minutes, past Jellyfin's shortest resumable runtime.
	create("School play/play.mp4", "-f", "lavfi", "-i", "testsrc=duration=360:size=64x48:rate=1", "-c:v", "mpeg4",
		"-metadata", "creation_time=2018-12-14T18:00:00Z")
	create("Beach.mp4", "-f", "lavfi", "-i", "testsrc=duration=2:size=64x48:rate=5", "-c:v", "mpeg4")
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Family", Path: dir, Kind: localfiles.KindHomeVideos})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}

	var views QueryResult
	s.get(t, "/UserViews", tr.token, &views)
	library := ""
	for _, view := range views.Items {
		if view.Name == "Family" {
			library = view.Id
			if view.CollectionType != "homevideos" {
				t.Errorf("the library's type: %q", view.CollectionType)
			}
		}
	}
	// jellyfin-web's Folders and Videos views.
	var folderView, videoView, movies QueryResult
	s.get(t, "/Items?ParentId="+library+"&Recursive=false&IncludeItemTypes=Folder,Photo,PhotoAlbum,Video", tr.token, &folderView)
	s.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes=Video&SortBy=SortName&SortOrder=Ascending", tr.token, &videoView)
	s.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes=Movie", tr.token, &movies)
	if len(folderView.Items) != 2 || len(videoView.Items) != 2 || videoView.Items[0].Name != "Beach" ||
		videoView.Items[1].Name != "School play" || len(movies.Items) != 0 {
		t.Fatalf("listings: %+v %+v %+v", folderView.Items, videoView.Items, movies.Items)
	}
	for _, item := range videoView.Items {
		if item.Type != "Video" || item.MediaType != "Video" || item.IsFolder || item.ImageTags["Primary"] == "" {
			t.Errorf("a home video: %+v", item)
		}
	}
	play := videoView.Items[1].Id
	var found, foundMovies QueryResult
	s.get(t, "/Items?searchTerm=school&Recursive=true&IncludeItemTypes=Video", tr.token, &found)
	s.get(t, "/Items?searchTerm=school&Recursive=true&IncludeItemTypes=Movie", tr.token, &foundMovies)
	if len(found.Items) != 1 || found.Items[0].Id != play || len(foundMovies.Items) != 0 {
		t.Errorf("search: %+v %+v", found.Items, foundMovies.Items)
	}
	var details BaseItemDto
	s.get(t, "/Users/"+tr.user+"/Items/"+play, tr.token, &details)
	if details.Type != "Video" || details.PremiereDate == nil || details.ProductionYear == nil || *details.ProductionYear != 2018 {
		t.Errorf("details: %+v", details)
	}

	// Played from the file, a range of it as a seek asks.
	answer := s.playbackInfo(t, "tv", tr.token, play, nil)
	if len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
		t.Fatalf("PlaybackInfo: %+v", answer)
	}
	source := answer.MediaSources[0]
	file, err := os.ReadFile(filepath.Join(dir, "School play/play.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	response, body := fetchURL(t, s.url+"/Videos/"+play+"/stream?static=true&MediaSourceId="+source.Id+"&ApiKey="+tr.token,
		http.Header{"Range": {"bytes=2000-2099"}})
	if response.StatusCode != http.StatusPartialContent || body != string(file[2000:2100]) {
		t.Errorf("a range of the file: %d", response.StatusCode)
	}

	// Stopped halfway, it resumes; then marked played. A movie marked
	// played after them is all the tracking service hears of.
	tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": play, "MediaSourceId": source.Id, "PositionTicks": 0})
	tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": play, "MediaSourceId": source.Id, "PositionTicks": 1_800_000_000})
	var resume QueryResult
	s.get(t, "/UserItems/Resume?MediaTypes=Video", tr.token, &resume)
	if len(resume.Items) != 1 || resume.Items[0].Id != play || resume.Items[0].UserData.PlaybackPositionTicks != 1_800_000_000 {
		t.Errorf("resume: %+v", resume.Items)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+videoView.Items[0].Id)
	if data := tr.userData(t, videoView.Items[0].Id); !data.Played {
		t.Errorf("played: %+v", data)
	}
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie)
	if calls := fakes.wait(t, 1); len(calls) != 1 || calls[0].body["movies"] == nil {
		t.Errorf("the tracking service heard: %+v", calls)
	}

	// In a collection, as other titles.
	if _, err := s.store.CreateUser(t.Context(), accounts.NewUser{Name: "admin", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	collection := s.createCollection(t, s.signIn("admin", "tv"), "Name=Holidays&Ids="+play)
	var inside QueryResult
	s.get(t, "/Items?ParentId="+collection, tr.token, &inside)
	if len(inside.Items) != 1 || inside.Items[0].Id != play {
		t.Errorf("the collection: %+v", inside.Items)
	}

	// Unrated, they are Other to parental control; a hidden library hides
	// them too.
	for _, tc := range []struct {
		name    string
		blocked []string
		max     *int
		want    bool
	}{
		{"movies", []string{"Movie"}, nil, true},
		{"rated", nil, new(10), true},
		{"others", []string{"Other"}, nil, false},
	} {
		user := s.user("child-"+tc.name, func(c *accounts.UserChanges) {
			c.Parental = &accounts.ParentalControl{MaxRating: tc.max, BlockUnrated: tc.blocked}
		})
		token := s.signIn("child-"+tc.name, "tv")
		var item BaseItemDto
		status := s.get(t, "/Users/"+user.ID.String()+"/Items/"+play, token, &item)
		var listed QueryResult
		s.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes=Video", token, &listed)
		if (status == http.StatusOK) != tc.want || slices.ContainsFunc(listed.Items, func(i BaseItemDto) bool { return i.Id == play }) != tc.want {
			t.Errorf("blocking %s: %d %+v", tc.name, status, listed.Items)
		}
	}
	s.user("hidden", func(c *accounts.UserChanges) { c.HiddenLibraries = &[]accounts.ID{mustID(t, library)} })
	token := s.signIn("hidden", "tv")
	var none QueryResult
	if status := s.get(t, "/Items?ParentId="+library, token, &none); status != http.StatusUnauthorized {
		t.Errorf("a hidden library: %d %+v", status, none.Items)
	}
}

// A home video renamed, moved into a subfolder, whose folder is renamed,
// or moved into another home videos folder, scanned before or after the
// folder it left, is the same item of Jellyfin apps, with its watch state
// and resume point; a copy is another item. A video found first in the
// folder it went to, and made a favorite there before the folder it left
// is scanned, stays the new item.
func TestHomeVideosFollowedInJellyfinApps(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, o.Logger, o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		folders.UseTools(ffprobe, ffmpeg)
	})
	tr := trackingOn(t, s)
	family, concerts := t.TempDir(), t.TempDir()
	// Videos of their own contents, those past six minutes resumable.
	for rel, seconds := range map[string]int{"Beach.mp4": 2, "Party.mp4": 360, "Trip/day 1.mp4": 3, "Trip/day 2.mp4": 361, "Concert.mp4": 4} {
		path := filepath.Join(family, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration="+strconv.Itoa(seconds)+":size=64x48:rate=1",
			"-c:v", "mpeg4", path).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	// Scanned in this order: the folder a video left first, then the one it
	// went to.
	var scanned []accounts.ID
	for _, folder := range []localfiles.NewFolder{{Name: "Family", Path: family}, {Name: "Concerts", Path: concerts}} {
		folder.Kind = localfiles.KindHomeVideos
		addon, err := folders.Add(t.Context(), folder)
		if err != nil {
			t.Fatal(err)
		}
		scanned = append(scanned, addon.ID)
	}
	scan := func() {
		t.Helper()
		for _, id := range scanned {
			if err := folders.Scan(t.Context(), id); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The videos of both libraries, by name.
	videos := func() map[string]string {
		t.Helper()
		var views QueryResult
		s.get(t, "/UserViews", tr.token, &views)
		ids := map[string]string{}
		for _, view := range views.Items {
			if view.CollectionType != "homevideos" {
				continue
			}
			var page QueryResult
			s.get(t, "/Items?ParentId="+view.Id+"&Recursive=true&IncludeItemTypes=Video", tr.token, &page)
			for _, item := range page.Items {
				ids[item.Name] = item.Id
			}
		}
		return ids
	}
	scan()
	before := videos()
	for _, name := range []string{"Beach", "day 1", "Concert"} {
		tr.mark(t, http.MethodPost, "/UserPlayedItems/"+before[name])
	}
	for _, name := range []string{"Party", "day 2"} {
		source := s.playbackInfo(t, "tv", tr.token, before[name], nil).MediaSources[0].Id
		tr.report(t, "/Sessions/Playing", map[string]any{"ItemId": before[name], "MediaSourceId": source, "PositionTicks": 0})
		tr.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": before[name], "MediaSourceId": source, "PositionTicks": 1_800_000_000})
	}

	for from, to := range map[string]string{"Beach.mp4": "Beach day.mp4", "Party.mp4": "2019/Party.mp4", "Trip": "Summer trip",
		"Concert.mp4": filepath.Join(concerts, "Concert.mp4")} {
		if !filepath.IsAbs(to) {
			to = filepath.Join(family, to)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(family, from), to); err != nil {
			t.Fatal(err)
		}
	}
	scan()
	after := videos()
	// A video alone in its folder is named after it.
	for now, was := range map[string]string{"Beach day": "Beach", "2019": "Party", "day 1": "day 1", "day 2": "day 2", "Concert": "Concert"} {
		if after[now] != before[was] {
			t.Errorf("%s, now %s: %s, was %s", was, now, after[now], before[was])
		}
	}
	for _, name := range []string{"Beach day", "day 1", "Concert"} {
		if data := tr.userData(t, after[name]); !data.Played {
			t.Errorf("%s: %+v", name, data)
		}
	}
	var resume QueryResult
	s.get(t, "/UserItems/Resume?MediaTypes=Video", tr.token, &resume)
	var resumed []string
	for _, item := range resume.Items {
		if item.UserData.PlaybackPositionTicks == 1_800_000_000 {
			resumed = append(resumed, item.Name)
		}
	}
	slices.Sort(resumed)
	if !slices.Equal(resumed, []string{"2019", "day 2"}) {
		t.Errorf("resume: %+v", resume.Items)
	}

	// A copy, its original still there.
	data, err := os.ReadFile(filepath.Join(family, "Beach day.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(family, "Beach copy.mp4"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	scan()
	copied := videos()
	if copied["Beach day"] != before["Beach"] || copied["Beach copy"] == "" || copied["Beach copy"] == before["Beach"] ||
		tr.userData(t, copied["Beach copy"]).Played {
		t.Errorf("a copy: %+v", copied)
	}

	// Moved between the libraries, the folder they went to scanned first.
	for from, to := range map[string]string{filepath.Join(family, "2019/Party.mp4"): filepath.Join(concerts, "Party.mp4"),
		filepath.Join(family, "Summer trip/day 1.mp4"): filepath.Join(concerts, "day 1.mp4"),
		filepath.Join(family, "Summer trip/day 2.mp4"): filepath.Join(concerts, "day 2.mp4"),
		filepath.Join(concerts, "Concert.mp4"):         filepath.Join(family, "Concert.mp4")} {
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	if err := folders.Scan(t.Context(), scanned[1]); err != nil {
		t.Fatal(err)
	}
	early := videos()
	tr.mark(t, http.MethodPost, "/UserFavoriteItems/"+early["day 2"])
	if err := folders.Scan(t.Context(), scanned[0]); err != nil {
		t.Fatal(err)
	}
	late := videos()
	for _, name := range []string{"Party", "day 1", "Concert"} {
		if late[name] != before[name] {
			t.Errorf("%s, scanned first where it went: %s, was %s", name, late[name], before[name])
		}
	}
	if !tr.userData(t, late["day 1"]).Played || !tr.userData(t, late["Concert"]).Played ||
		tr.userData(t, late["Party"]).PlaybackPositionTicks != 1_800_000_000 {
		t.Errorf("their state: %+v %+v %+v", tr.userData(t, late["day 1"]), tr.userData(t, late["Concert"]), tr.userData(t, late["Party"]))
	}
	if data := tr.userData(t, late["day 2"]); late["day 2"] != early["day 2"] || late["day 2"] == before["day 2"] || !data.IsFavorite {
		t.Errorf("a favorite meanwhile: %s %+v, was %s", late["day 2"], data, before["day 2"])
	}
}
