package jellyfin

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/smartlists"
	"github.com/moodiness/polyfin/internal/stremio"
)

// plainSetup is a test server whose member is signed in (see trackingOn),
// with a movies folder and a shows folder whose files match no title, no
// addon of the server searching titles; video is the bytes of their
// videos, six minutes each, past Jellyfin's shortest resumable runtime.
type plainSetup struct {
	tracking
	folders       *localfiles.Service
	movies, shows accounts.ID
	dir           string
	video         []byte
	moviesLibrary string
	showsLibrary  string
}

func newPlainSetup(t *testing.T) plainSetup {
	t.Helper()
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
	})
	p := plainSetup{tracking: trackingOn(t, s), folders: folders, dir: t.TempDir()}
	scratch := filepath.Join(t.TempDir(), "video.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration=360:size=64x48:rate=1", "-c:v", "mpeg4",
		scratch).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	var err error
	if p.video, err = os.ReadFile(scratch); err != nil {
		t.Fatal(err)
	}
	poster := filepath.Join(t.TempDir(), "poster.jpg")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=32x48", "-frames:v", "1",
		poster).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	image, err := os.ReadFile(poster)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"Movies/Harbor.Lights.2.2011.1080p.mp4":        p.video,
		"Movies/Harbor.Lights.2.2011.1080p-poster.jpg": image,
		"Movies/Unknown clip.mp4":                      p.video,
		// An extra without its title, and a file whose name gives no
		// episode, are no plain videos.
		"Movies/Lost-trailer.mp4":                                   p.video,
		"Shows/Harbor Show (2011)/Season 01/Harbor Show S01E01.mp4": p.video,
		"Shows/Harbor Show (2011)/Season 01/Harbor Show S01E02.mp4": p.video,
		"Shows/Harbor Show (2011)/Season 01/Behind it all.mp4":      p.video,
		"Shows/Harbor Show (2011)/Season 02/Harbor Show S02E01.mp4": p.video,
	}
	for rel, data := range files {
		p.write(t, rel, data)
	}
	add := func(name, rel, kind string) accounts.ID {
		t.Helper()
		addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: name, Path: filepath.Join(p.dir, rel), Kind: kind})
		if err != nil {
			t.Fatal(err)
		}
		return addon.ID
	}
	p.movies, p.shows = add("Local movies", "Movies", localfiles.KindMovies), add("Local shows", "Shows", localfiles.KindShows)
	p.scan(t)
	var views QueryResult
	p.get(t, "/UserViews", p.token, &views)
	for _, view := range views.Items {
		switch view.Name {
		case "Local movies":
			p.moviesLibrary = view.Id
		case "Local shows":
			p.showsLibrary = view.Id
		}
	}
	return p
}

func (p plainSetup) write(t *testing.T, rel string, data []byte) {
	t.Helper()
	path := filepath.Join(p.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// titles lists the plain movies, the latter's first, and the plain series
// with its episodes, failing unless they are listed.
func (p plainSetup) titles(t *testing.T) (movies []BaseItemDto, series string, episodes []BaseItemDto) {
	t.Helper()
	movies = p.listed(t, p.token, p.moviesLibrary, "Movie")
	shows := p.listed(t, p.token, p.showsLibrary, "Series")
	if len(movies) != 2 || len(shows) != 1 {
		t.Fatalf("the plain videos: %v %v", itemNames(movies), itemNames(shows))
	}
	var page QueryResult
	p.get(t, "/Shows/"+shows[0].Id+"/Episodes?UserId="+p.user, p.token, &page)
	if len(page.Items) != 3 {
		t.Fatalf("the plain episodes: %v", itemNames(page.Items))
	}
	return movies, shows[0].Id, page.Items
}

func (p plainSetup) scan(t *testing.T) {
	t.Helper()
	for _, id := range []accounts.ID{p.movies, p.shows} {
		if err := p.folders.Scan(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
}

// listed lists a library's items of a type as token's user sees them, by
// name.
func (p plainSetup) listed(t *testing.T, token, library, types string) []BaseItemDto {
	t.Helper()
	var page QueryResult
	p.get(t, "/Items?ParentId="+library+"&Recursive=true&IncludeItemTypes="+types+"&SortBy=SortName&SortOrder=Ascending", token, &page)
	return page.Items
}

// The files a movies folder leaves unmatched are Movie items of its
// library, named as their names read, with the year they give, without
// a description or provider identifiers, with the poster beside them, and
// searchable; a shows folder's are a Series named after its folder, whose
// seasons and episodes are those the files' names give, the episodes named
// after their files. Extras without their titles and files whose names
// give no episode are not listed. Smart lists drawing from the folder keep
// them by their rules. Their identifiers survive a scan; a file
// renamed is another item.
func TestPlainVideosListed(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	movies := p.listed(t, p.token, p.moviesLibrary, "Movie")
	if !slices.Equal(itemNames(movies), []string{"Harbor Lights 2", "Unknown clip"}) {
		t.Fatalf("the movies: %v", itemNames(movies))
	}
	harbor := movies[0]
	var details BaseItemDto
	p.get(t, "/Users/"+p.user+"/Items/"+harbor.Id, p.token, &details)
	if details.Type != "Movie" || details.ProductionYear == nil || *details.ProductionYear != 2011 ||
		details.Overview != nil && *details.Overview != "" || details.ProviderIds != nil && len(*details.ProviderIds) > 0 ||
		details.ImageTags["Primary"] == "" {
		t.Errorf("a plain movie: %+v", details)
	}
	var found QueryResult
	p.get(t, "/Items?searchTerm=harbor%20lights&Recursive=true&IncludeItemTypes=Movie", p.token, &found)
	if len(found.Items) != 1 || found.Items[0].Id != harbor.Id {
		t.Errorf("search: %+v", found.Items)
	}

	// A smart list drawing from the folder keeps those its rules keep.
	list, err := smartlists.New(p.pool).Create(t.Context(), smartlists.List{Name: "Recent", ShownAs: smartlists.ShownAsLibrary,
		Sort: smartlists.SortName, Catalogs: []smartlists.Catalog{{AddonID: p.movies, Type: "movie", ID: "local"}},
		Rules: smartlists.Rule{Field: smartlists.FieldGroup, Match: smartlists.MatchAll,
			Rules: []smartlists.Rule{{Field: smartlists.FieldYear, Min: new(2010.0)}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.listingNames(t, p.token, "/Items?ParentId="+library.SmartListItemID(list.ID).String()); !slices.Equal(got, []string{"Harbor Lights 2"}) {
		t.Errorf("the smart list: %v", got)
	}

	series := p.listed(t, p.token, p.showsLibrary, "Series")
	if len(series) != 1 || series[0].Name != "Harbor Show" || series[0].ProductionYear == nil || *series[0].ProductionYear != 2011 {
		t.Fatalf("the series: %+v", series)
	}
	var seasons, episodes QueryResult
	p.get(t, "/Shows/"+series[0].Id+"/Seasons?UserId="+p.user, p.token, &seasons)
	p.get(t, "/Shows/"+series[0].Id+"/Episodes?UserId="+p.user, p.token, &episodes)
	if len(seasons.Items) != 2 || *seasons.Items[0].IndexNumber != 1 || *seasons.Items[1].IndexNumber != 2 {
		t.Errorf("the seasons: %+v", seasons.Items)
	}
	type numbered struct {
		name            string
		season, episode int
	}
	var got []numbered
	for _, episode := range episodes.Items {
		if episode.ParentIndexNumber == nil || episode.IndexNumber == nil {
			t.Fatalf("an episode: %+v", episode)
		}
		got = append(got, numbered{episode.Name, *episode.ParentIndexNumber, *episode.IndexNumber})
	}
	if want := []numbered{{"Harbor Show S01E01", 1, 1}, {"Harbor Show S01E02", 1, 2}, {"Harbor Show S02E01", 2, 1}}; !slices.Equal(got, want) {
		t.Errorf("the episodes: %+v", got)
	}

	// The same items after a scan; a file renamed is another.
	p.scan(t)
	if again := p.listed(t, p.token, p.moviesLibrary, "Movie"); len(again) != 2 || again[0].Id != harbor.Id || again[1].Id != movies[1].Id {
		t.Errorf("after a scan: %+v", again)
	}
	if err := os.Rename(filepath.Join(p.dir, "Movies", "Unknown clip.mp4"), filepath.Join(p.dir, "Movies", "Unknown clip 2.mp4")); err != nil {
		t.Fatal(err)
	}
	p.scan(t)
	if renamed := p.listed(t, p.token, p.moviesLibrary, "Movie"); len(renamed) != 2 || renamed[1].Name != "Unknown clip 2" ||
		renamed[1].Id == movies[1].Id {
		t.Errorf("renamed: %+v", renamed)
	}
}

// A plain movie and a plain episode play from their files, with byte
// ranges, as the folders' other files do.
func TestPlainVideosPlay(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	movies, _, episodes := p.titles(t)
	for _, item := range []string{movies[0].Id, episodes[0].Id} {
		answer := p.playbackInfo(t, "tv", p.token, item, nil)
		if len(answer.MediaSources) != 1 || !answer.MediaSources[0].SupportsDirectPlay {
			t.Fatalf("PlaybackInfo of %s: %+v", item, answer)
		}
		response, body := fetchURL(t, p.url+"/Videos/"+item+"/stream?static=true&MediaSourceId="+answer.MediaSources[0].Id+"&ApiKey="+p.token,
			http.Header{"Range": {"bytes=2000-2099"}})
		if response.StatusCode != http.StatusPartialContent || body != string(p.video[2000:2100]) {
			t.Errorf("a range of %s: %d", item, response.StatusCode)
		}
	}
}

// Parental control judges plain movies as unrated movies and plain series
// as unrated series, as Jellyfin judges the items it cannot identify; a
// user's hidden titles and libraries hide them as other titles.
func TestPlainVideosRestricted(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	movies, series, _ := p.titles(t)
	movie := movies[0].Id
	for _, tc := range []struct {
		name          string
		blocked       []string
		max           *int
		movie, series bool
	}{
		{"movies", []string{"Movie"}, nil, false, true},
		{"series", []string{"Series"}, nil, true, false},
		{"others", []string{"Other"}, nil, true, true},
		{"rated", nil, new(10), true, true},
	} {
		user := p.testServer.user("child-"+tc.name, func(c *accounts.UserChanges) {
			c.Parental = &accounts.ParentalControl{MaxRating: tc.max, BlockUnrated: tc.blocked}
		})
		token := p.signIn("child-"+tc.name, "tv")
		for _, title := range []struct {
			id, library, types string
			want               bool
		}{{movie, p.moviesLibrary, "Movie", tc.movie}, {series, p.showsLibrary, "Series", tc.series}} {
			var item BaseItemDto
			status := p.get(t, "/Users/"+user.ID.String()+"/Items/"+title.id, token, &item)
			listed := slices.ContainsFunc(p.listed(t, token, title.library, title.types), func(i BaseItemDto) bool { return i.Id == title.id })
			if (status == http.StatusOK) != title.want || listed != title.want {
				t.Errorf("blocking %s, %s: %d %v", tc.name, title.types, status, listed)
			}
		}
	}

	if status, data := p.call(http.MethodPost, "/Polyfin/UserItems/"+movie+"/Hidden", app("tv", p.token), nil); status/100 != 2 {
		t.Fatalf("hiding: %d %s", status, data)
	}
	if slices.ContainsFunc(p.listed(t, p.token, p.moviesLibrary, "Movie"), func(i BaseItemDto) bool { return i.Id == movie }) {
		t.Error("a hidden plain movie is listed")
	}
	p.testServer.user("hidden", func(c *accounts.UserChanges) { c.HiddenLibraries = &[]accounts.ID{mustID(t, p.showsLibrary)} })
	token := p.signIn("hidden", "tv")
	var none QueryResult
	if status := p.get(t, "/Items?ParentId="+p.showsLibrary, token, &none); status != http.StatusUnauthorized {
		t.Errorf("a hidden library: %d %+v", status, none.Items)
	}
}

// A plain video leaves its library once matched, by hand or by a scan, and
// what its user did with it goes to its title: a movie's resume point and
// favorite, an episode's resume point to the matched show's episode.
// Continue Watching and the favorites then list the title alone, and the
// plain videos' items answer 404.
func TestPlainVideosLeaveOnceMatched(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	movies, plainSeries, plainEpisodes := p.titles(t)
	harbor, clip := movies[0].Id, movies[1].Id
	second := plainEpisodes[1].Id
	play := func(id string, position int64) {
		t.Helper()
		source := p.playbackInfo(t, "tv", p.token, id, nil).MediaSources[0].Id
		p.report(t, "/Sessions/Playing", map[string]any{"ItemId": id, "MediaSourceId": source, "PositionTicks": 0})
		p.report(t, "/Sessions/Playing/Stopped", map[string]any{"ItemId": id, "MediaSourceId": source, "PositionTicks": position})
	}
	play(harbor, 1_800_000_000)
	p.mark(t, http.MethodPost, "/UserFavoriteItems/"+harbor)
	play(second, 1_200_000_000)

	// Linked by hand, to a movie and a show an addon lists.
	imdb := func(item string) string {
		t.Helper()
		var details BaseItemDto
		status := p.get(t, "/Users/"+p.user+"/Items/"+item, p.token, &details)
		if details.ProviderIds == nil || (*details.ProviderIds)["Imdb"] == "" {
			t.Fatalf("no IMDb identifier for %s: %d %v", item, status, details.ProviderIds)
		}
		return (*details.ProviderIds)["Imdb"]
	}
	if err := p.folders.Link(t.Context(), p.movies, "Harbor.Lights.2.2011.1080p.mp4", imdb(p.movie)); err != nil {
		t.Fatal(err)
	}
	if err := p.folders.Link(t.Context(), p.shows, "Harbor Show (2011)", imdb(p.series)); err != nil {
		t.Fatal(err)
	}
	listed := p.listed(t, p.token, p.moviesLibrary, "Movie")
	if len(listed) != 2 || !slices.ContainsFunc(listed, func(i BaseItemDto) bool { return i.Id == p.movie }) ||
		!slices.ContainsFunc(listed, func(i BaseItemDto) bool { return i.Id == clip }) {
		t.Fatalf("the movies once one is linked: %v", itemNames(listed))
	}
	if data := p.userData(t, p.movie); data.PlaybackPositionTicks != 1_800_000_000 || !data.IsFavorite {
		t.Errorf("the linked movie's data: %+v", data)
	}
	shows := p.listed(t, p.token, p.showsLibrary, "Series")
	if len(shows) != 1 || shows[0].Id != p.series {
		t.Fatalf("the series once linked: %v", itemNames(shows))
	}
	var episodes QueryResult
	p.get(t, "/Shows/"+shows[0].Id+"/Episodes?UserId="+p.user, p.token, &episodes)
	var matched string
	for _, episode := range episodes.Items {
		if *episode.ParentIndexNumber == 1 && *episode.IndexNumber == 2 {
			matched = episode.Id
		}
	}
	if matched == "" || matched == second {
		t.Fatalf("the linked show's episodes: %+v", episodes.Items)
	}
	if data := p.userData(t, matched); data.PlaybackPositionTicks != 1_200_000_000 {
		t.Errorf("the linked episode's data: %+v", data)
	}
	var resume QueryResult
	p.get(t, "/UserItems/Resume?MediaTypes=Video", p.token, &resume)
	var resumed []string
	for _, item := range resume.Items {
		resumed = append(resumed, item.Id)
	}
	slices.Sort(resumed)
	want := []string{p.movie, matched}
	slices.Sort(want)
	if !slices.Equal(resumed, want) {
		t.Errorf("Continue Watching: %v, want %v", resumed, want)
	}
	// The plain videos matched are gone, as Jellyfin answers for an item
	// that is.
	for _, gone := range []string{harbor, plainSeries, second} {
		var item BaseItemDto
		if status := p.get(t, "/Users/"+p.user+"/Items/"+gone, p.token, &item); status != http.StatusNotFound {
			t.Errorf("a plain video matched: %d %s", status, item.Name)
		}
	}
	var favorites QueryResult
	p.get(t, "/Items?Filters=IsFavorite&Recursive=true", p.token, &favorites)
	if len(favorites.Items) != 1 || favorites.Items[0].Id != p.movie {
		t.Errorf("the favorites: %v", itemNames(favorites.Items))
	}

	// Matched by a scan, through the NFO file added beside it.
	p.write(t, "Movies/Unknown clip.nfo", []byte(`<movie><uniqueid type="imdb">tt2000000</uniqueid></movie>`))
	p.scan(t)
	if listed := p.listed(t, p.token, p.moviesLibrary, "Movie"); slices.ContainsFunc(listed, func(i BaseItemDto) bool { return i.Id == clip }) ||
		len(listed) != 2 {
		t.Errorf("the movies once the other is matched: %+v", listed)
	}
}
