package localfiles

import (
	"bytes"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
)

// sideImagesOf lists side files of one byte each, those named "empty:…"
// empty.
func sideImagesOf(names ...string) sideImages {
	sides := map[string]found{}
	for _, name := range names {
		size := int64(1)
		if rest, ok := strings.CutPrefix(name, "empty:"); ok {
			name, size = rest, 0
		}
		sides[name] = found{size: size}
	}
	return listImages(sides)
}

// A movie alone in its folder, its versions included, takes the folder's
// names and those after its file's name; a movie sharing its folder only
// those after its name. Of each type, the name Jellyfin prefers comes
// first, in any letter case, empty files left out.
func TestMovieArtworkNames(t *testing.T) {
	files := []imagedFile{
		{path: "Alone (2001)/Alone (2001).mkv", unit: "Alone (2001)/Alone (2001).mkv", title: "tt1"},
		{path: "Versions (2002)/Versions (2002) - 1080p.mkv", unit: "Versions (2002)/Versions (2002) - 1080p.mkv", title: "tt2"},
		{path: "Versions (2002)/Versions (2002) - 2160p.mkv", unit: "Versions (2002)/Versions (2002) - 2160p.mkv", title: "tt2"},
		{path: "Shared/First (2003).mkv", unit: "Shared/First (2003).mkv", title: "tt3"},
		{path: "Shared/Second (2004).mkv", unit: "Shared/Second (2004).mkv", title: "tt4"},
		{path: "Mixed/Known (2005).mkv", unit: "Mixed/Known (2005).mkv", title: "tt5"},
		{path: "Mixed/Unknown.mkv", unit: "Mixed/Unknown.mkv"},
		{path: "Disc (2006)/BDMV/PLAYLIST/00800.mpls", unit: "Disc (2006)", title: "tt6"},
		{path: "Disc (2006)/BDMV/PLAYLIST/00002.mpls", unit: "Disc (2006)#00002", title: "tt7"},
	}
	found := sideImagesOf(
		"Alone (2001)/poster.png", "Alone (2001)/POSTER.jpg", "Alone (2001)/folder.jpg", "Alone (2001)/clearlogo.webp",
		"Alone (2001)/Logo.png", "Alone (2001)/backdrop3.jpg", "Alone (2001)/FanArt.jpeg", "Alone (2001)/thumb.jpg",
		"Alone (2001)/landscape.jpg", "Alone (2001)/clearart.png", "Alone (2001)/discart.png", "Alone (2001)/banner.jpg",
		"empty:Versions (2002)/poster.jpg", "Versions (2002)/folder.jpg", "Versions (2002)/Versions (2002) - 2160p-fanart.jpg",
		"Shared/poster.jpg", "Shared/fanart.jpg", "Shared/First (2003)-poster.jpg", "Shared/First (2003)-backdrop2.jpg",
		"Shared/Second (2004).jpg", "Shared/Second (2004)-clearlogo.png",
		"Mixed/poster.jpg", "Mixed/Known (2005)-fanart-1.png",
		"Disc (2006)/poster.jpg", "Disc (2006)/Disc (2006)-disc.png",
	)
	want := map[artworkKey]string{
		{"Alone (2001)/Alone (2001).mkv", -1, "Primary", ""}:  "Alone (2001)/POSTER.jpg",
		{"Alone (2001)/Alone (2001).mkv", -1, "Logo", ""}:     "Alone (2001)/Logo.png",
		{"Alone (2001)/Alone (2001).mkv", -1, "Backdrop", ""}: "Alone (2001)/FanArt.jpeg",
		{"Alone (2001)/Alone (2001).mkv", -1, "Thumb", ""}:    "Alone (2001)/landscape.jpg",
		{"Alone (2001)/Alone (2001).mkv", -1, "Art", ""}:      "Alone (2001)/clearart.png",
		{"Alone (2001)/Alone (2001).mkv", -1, "Disc", ""}:     "Alone (2001)/discart.png",
		{"Alone (2001)/Alone (2001).mkv", -1, "Banner", ""}:   "Alone (2001)/banner.jpg",

		{"Versions (2002)/Versions (2002) - 1080p.mkv", -1, "Primary", ""}:  "Versions (2002)/folder.jpg",
		{"Versions (2002)/Versions (2002) - 2160p.mkv", -1, "Primary", ""}:  "Versions (2002)/folder.jpg",
		{"Versions (2002)/Versions (2002) - 2160p.mkv", -1, "Backdrop", ""}: "Versions (2002)/Versions (2002) - 2160p-fanart.jpg",

		{"Shared/First (2003).mkv", -1, "Primary", ""}:  "Shared/First (2003)-poster.jpg",
		{"Shared/First (2003).mkv", -1, "Backdrop", ""}: "Shared/First (2003)-backdrop2.jpg",
		{"Shared/Second (2004).mkv", -1, "Primary", ""}: "Shared/Second (2004).jpg",
		{"Shared/Second (2004).mkv", -1, "Logo", ""}:    "Shared/Second (2004)-clearlogo.png",

		{"Mixed/Known (2005).mkv", -1, "Backdrop", ""}: "Mixed/Known (2005)-fanart-1.png",

		{"Disc (2006)", -1, "Primary", ""}: "Disc (2006)/poster.jpg",
		{"Disc (2006)", -1, "Disc", ""}:    "Disc (2006)/Disc (2006)-disc.png",
	}
	if got := chooseArtwork(KindMovies, "Movies", files, found); !maps.Equal(got, want) {
		t.Errorf("artwork:\n got %v\nwant %v", got, want)
	}

	// A disc at the folder's root is named after the folder.
	root := []imagedFile{{path: "BDMV/PLAYLIST/00800.mpls", unit: ".", title: "tt8"}}
	if got := chooseArtwork(KindMovies, "Lone Disc", root, sideImagesOf("Lone Disc-poster.jpg", "fanart.jpg")); !maps.Equal(got,
		map[artworkKey]string{{".", -1, "Primary", ""}: "Lone Disc-poster.jpg", {".", -1, "Backdrop", ""}: "fanart.jpg"}) {
		t.Errorf("a disc at the root: %v", got)
	}
}

// A show's images are in its folder; a season's in its show's folder,
// named after the season ("season1", then "season01"; "specials", then
// "season-specials"), even a season without files, then in its season
// folder. Episodes at the folder's top, without a show folder, have none.
func TestShowArtworkNames(t *testing.T) {
	season := func(n int) *int { return &n }
	files := []imagedFile{
		{path: "Show (2001)/Season 01/Show - S01E01.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 01/Show - S01E02.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Specials/Show - S00E01.mkv", unit: "Show (2001)", title: "tt1", season: season(0)},
		{path: "Show (2001)/Show - S02E01.mkv", unit: "Show (2001)", title: "tt1", season: season(2)},
		{path: "Show (2001)/Mixed/Show - S04E01.mkv", unit: "Show (2001)", title: "tt1", season: season(4)},
		{path: "Show (2001)/Mixed/Show - S05E01.mkv", unit: "Show (2001)", title: "tt1", season: season(5)},
		{path: "Loose - S01E01.mkv", unit: "Loose - S01E01.mkv", title: "tt2", season: season(1)},
	}
	found := sideImagesOf(
		"Show (2001)/show.jpg", "Show (2001)/backdrop.jpg", "Show (2001)/Show (2001)-logo.png",
		"Show (2001)/season01-poster.jpg", "Show (2001)/season1-poster.png", "Show (2001)/Season 01/folder.jpg",
		"Show (2001)/Season 01/fanart.jpg", "Show (2001)/season-specials-poster.jpg", "Show (2001)/Specials/poster.jpg",
		"Show (2001)/Specials/banner.jpg", "Show (2001)/Season02-Banner.jpg", "Show (2001)/season03-landscape.jpg",
		"Show (2001)/Mixed/poster.jpg", "Loose - S01E01-poster.jpg", "folder.jpg",
	)
	want := map[artworkKey]string{
		{"Show (2001)", -1, "Primary", ""}:  "Show (2001)/show.jpg",
		{"Show (2001)", -1, "Backdrop", ""}: "Show (2001)/backdrop.jpg",
		{"Show (2001)", -1, "Logo", ""}:     "Show (2001)/Show (2001)-logo.png",
		{"Show (2001)", 1, "Primary", ""}:   "Show (2001)/season1-poster.png",
		{"Show (2001)", 1, "Backdrop", ""}:  "Show (2001)/Season 01/fanart.jpg",
		{"Show (2001)", 0, "Primary", ""}:   "Show (2001)/season-specials-poster.jpg",
		{"Show (2001)", 0, "Banner", ""}:    "Show (2001)/Specials/banner.jpg",
		{"Show (2001)", 2, "Banner", ""}:    "Show (2001)/Season02-Banner.jpg",
		{"Show (2001)", 3, "Thumb", ""}:     "Show (2001)/season03-landscape.jpg",
	}
	if got := chooseArtwork(KindShows, "Shows", files, found); !maps.Equal(got, want) {
		t.Errorf("artwork:\n got %v\nwant %v", got, want)
	}
}

// An episode's image is beside its file, named after it, "<name>-thumb"
// first, then "<name>", then in a "metadata" folder beside it, in any
// letter case, empty files left out; it is that of each episode a file
// holds, an episode at the folder's top included. Discs, disc images and
// files that are no episode have none, and other images beside an episode
// are not its image.
func TestEpisodeArtworkNames(t *testing.T) {
	season := func(n int) *int { return &n }
	files := []imagedFile{
		{path: "Show (2001)/Season 01/Show - S01E01.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 01/Show - S01E02.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 01/Show - S01E03-E04.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 01/Show - S01E05.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 01/Show - S01E06.mkv", unit: "Show (2001)", title: "tt1", season: season(1)},
		{path: "Show (2001)/Season 02/Disc 1/BDMV/PLAYLIST/00001.mpls", unit: "Show (2001)", title: "tt1", season: season(2)},
		{path: "Show (2001)/Season 02/Box.iso#2", unit: "Show (2001)", title: "tt1", season: season(2)},
		{path: "Show (2001)/Season 02/Single.iso", unit: "Show (2001)", title: "tt1", season: season(2)},
		{path: "Show (2001)/Show (2001) Making Of.mkv", unit: "Show (2001)", title: "tt1"},
		{path: "Loose - S01E01.mkv", unit: "Loose - S01E01.mkv", season: season(1)},
	}
	found := sideImagesOf(
		"Show (2001)/Season 01/Show - S01E01.png", "Show (2001)/Season 01/Show - S01E01-thumb.jpg",
		"Show (2001)/Season 01/metadata/Show - S01E02-thumb.jpg", "Show (2001)/Season 01/SHOW - S01E02.webp",
		"Show (2001)/Season 01/metadata/Show - S01E03-E04.jpg",
		"Show (2001)/Season 01/Show - S01E05-poster.jpg", "Show (2001)/Season 01/Show - S01E05-fanart.jpg",
		"Show (2001)/Season 01/Show - S01E0-thumb.jpg", "Show (2001)/Season 01/Show - S01E05 - thumb.jpg",
		"Show (2001)/Season 01/Other/Show - S01E05-thumb.jpg", "Show (2001)/Season 01/Show - S01E05-thumb.gif",
		"empty:Show (2001)/Season 01/Show - S01E06-thumb.jpg", "Show (2001)/Season 01/Show - S01E06.jpeg",
		"Show (2001)/Season 02/Disc 1/BDMV/PLAYLIST/00001-thumb.jpg", "Show (2001)/Season 02/Disc 1/BDMV/PLAYLIST/00001.jpg",
		"Show (2001)/Season 02/Box-thumb.jpg", "Show (2001)/Season 02/Single-thumb.jpg",
		"Show (2001)/Show (2001) Making Of-thumb.jpg",
		"Loose - S01E01-thumb.png",
	)
	episode := func(file string) artworkKey {
		unit := "Show (2001)"
		if !strings.Contains(file, "/") {
			unit = file
		}
		return artworkKey{unit: unit, season: -1, imageType: "Primary", file: file}
	}
	want := map[artworkKey]string{
		episode("Show (2001)/Season 01/Show - S01E01.mkv"):     "Show (2001)/Season 01/Show - S01E01-thumb.jpg",
		episode("Show (2001)/Season 01/Show - S01E02.mkv"):     "Show (2001)/Season 01/SHOW - S01E02.webp",
		episode("Show (2001)/Season 01/Show - S01E03-E04.mkv"): "Show (2001)/Season 01/metadata/Show - S01E03-E04.jpg",
		episode("Show (2001)/Season 01/Show - S01E06.mkv"):     "Show (2001)/Season 01/Show - S01E06.jpeg",
		episode("Loose - S01E01.mkv"):                          "Loose - S01E01-thumb.png",
	}
	got := chooseArtwork(KindShows, "Shows", files, found)
	maps.DeleteFunc(got, func(key artworkKey, _ string) bool { return key.file == "" })
	if !maps.Equal(got, want) {
		t.Errorf("episodes' images:\n got %v\nwant %v", got, want)
	}
}

// artworkOf reads the artwork the service gives an item.
func (e env) artworkOf(item accounts.ID) library.Images {
	e.t.Helper()
	return e.service.Artwork()[item]
}

// pictured reads the image an artwork URL stands for.
func (e env) pictured(url string) string {
	e.t.Helper()
	key, ok := library.LocalArtworkKey(url)
	if !ok {
		e.t.Fatalf("not a local image: %q", url)
	}
	return e.opened(key)
}

// writeImage writes data at path, its folders too, modified at.
func writeImage(t *testing.T, path string, data []byte, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// A scan keeps the artwork beside a movies folder's files, which the
// library shows for their titles: the image's URL changes with its size
// and modification time, and an image removed is forgotten. The folder's
// UseArtwork turns it off and on at once, and a link made by hand gives a
// unit's artwork to its title without a scan.
func TestMovieArtworkIsKeptAndServed(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	write(t, filepath.Join(dir, "The General (1926).mkv"), 10)
	write(t, filepath.Join(dir, "The Gold Rush (1925).mkv"), 10)
	write(t, filepath.Join(dir, "Unknown", "An Unknown Picture (1950).mkv"), 10)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "poster.jpg"), []byte("first poster"), at)
	writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "fanart.png"), []byte("backdrop"), at)
	writeImage(t, filepath.Join(dir, "The General (1926)-poster.jpg"), []byte("general"), at)
	writeImage(t, filepath.Join(dir, "poster.jpg"), []byte("shared folder"), at)
	writeImage(t, filepath.Join(dir, "Unknown", "folder.jpg"), []byte("unknown"), at)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shelf", Path: dir, Kind: KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)

	nosferatu := library.TitleItemID(library.KindMovie, "tt0013442")
	images := e.artworkOf(nosferatu)
	if e.pictured(images.Primary) != "first poster" || e.pictured(images.Backdrop) != "backdrop" || images.Logo != "" {
		t.Fatalf("Nosferatu: %+v", images)
	}
	if general := e.artworkOf(library.TitleItemID(library.KindMovie, "tt0017925")); e.pictured(general.Primary) != "general" ||
		general.Backdrop != "" {
		t.Errorf("The General: %+v", general)
	}
	if rush, ok := e.service.Artwork()[library.TitleItemID(library.KindMovie, "tt0015864")]; ok {
		t.Errorf("The Gold Rush shares its folder: %+v", rush)
	}

	// The poster replaced, by one of another size, then of another time.
	before := images.Primary
	writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "poster.jpg"), []byte("second poster!"), at)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	images = e.artworkOf(nosferatu)
	if images.Primary == before || e.pictured(images.Primary) != "second poster!" {
		t.Errorf("a poster of another size: %q, was %q", images.Primary, before)
	}
	before = images.Primary
	writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "poster.jpg"), []byte("third poster!!"), at.Add(time.Minute))
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if images = e.artworkOf(nosferatu); images.Primary == before || e.pictured(images.Primary) != "third poster!!" {
		t.Errorf("a poster of another time: %q, was %q", images.Primary, before)
	}

	// Off, the folder's artwork shows no more, at once; on, it is back.
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{UseArtwork: new(false)}); err != nil {
		t.Fatal(err)
	}
	if folder, err := e.service.Folder(t.Context(), addon.ID); err != nil || folder.UseArtwork {
		t.Errorf("folder: %+v %v", folder, err)
	}
	if len(e.service.Artwork()) != 0 {
		t.Errorf("artwork with the option off: %v", e.service.Artwork())
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{UseArtwork: new(true)}); err != nil {
		t.Fatal(err)
	}
	if e.artworkOf(nosferatu) != images {
		t.Errorf("artwork with the option back on: %+v", e.artworkOf(nosferatu))
	}

	// A link gives the unit's artwork to its title at once.
	if err := e.service.Link(t.Context(), addon.ID, "Unknown/An Unknown Picture (1950).mkv", "tt0100258"); err != nil {
		t.Fatal(err)
	}
	if linked := e.artworkOf(library.TitleItemID(library.KindMovie, "tt0100258")); e.pictured(linked.Primary) != "unknown" {
		t.Errorf("linked: %+v", linked)
	}

	// An image removed is forgotten, and opens no more.
	if err := os.Remove(filepath.Join(dir, "Nosferatu (1922)", "fanart.png")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if after := e.artworkOf(nosferatu); after.Backdrop != "" || after.Primary != images.Primary {
		t.Errorf("after the backdrop's removal: %+v", after)
	}
	key, _ := library.LocalArtworkKey(images.Backdrop)
	if _, _, err := e.service.Open(t.Context(), key); err != ErrGone {
		t.Errorf("a removed image opens: %v", err)
	}
}

// A shows folder's artwork is its shows' and their seasons'.
func TestShowArtworkIsKept(t *testing.T) {
	e := newEnv(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "The Lone Ranger (1949)", "Season 01", "The Lone Ranger - S01E02.mkv"), 10)
	writeImage(t, filepath.Join(dir, "The Lone Ranger (1949)", "poster.jpg"), []byte("show"), at)
	writeImage(t, filepath.Join(dir, "The Lone Ranger (1949)", "season02-poster.jpg"), []byte("season 2"), at)
	writeImage(t, filepath.Join(dir, "The Lone Ranger (1949)", "Season 01", "folder.jpg"), []byte("season 1"), at)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: dir, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	for item, want := range map[accounts.ID]string{
		library.TitleItemID(library.KindSeries, "tt0041038"): "show",
		library.SeasonItemID("tt0041038", 1):                 "season 1",
		library.SeasonItemID("tt0041038", 2):                 "season 2",
	} {
		if images := e.artworkOf(item); images.Primary == "" || e.pictured(images.Primary) != want {
			t.Errorf("%s: %+v", want, images)
		}
	}
}

// An episode's image beside its file is the image of each episode the
// file holds; a scan notices one added, replaced or removed, the folder's
// UseArtwork turns them off and on, and other images are not theirs.
func TestEpisodeArtworkIsKept(t *testing.T) {
	e := newEnv(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	dir := t.TempDir()
	season := filepath.Join(dir, "The Lone Ranger (1949)", "Season 01")
	write(t, filepath.Join(season, "The Lone Ranger - S01E01.mkv"), 10)
	write(t, filepath.Join(season, "The Lone Ranger - S01E02.mkv"), 10)
	write(t, filepath.Join(season, "The Lone Ranger - S01E03-E04.mkv"), 10)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E01-thumb.jpg"), []byte("still 1"), at)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E03-E04.png"), []byte("still 3"), at)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E02-poster.jpg"), []byte("unrelated"), at)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E2-thumb.jpg"), []byte("unrelated"), at)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: dir, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	pilot, second := library.EpisodeItemID("tt0041038:1:1"), library.EpisodeItemID("tt0041038:1:2")
	for item, want := range map[accounts.ID]string{pilot: "still 1", library.EpisodeItemID("tt0041038:1:3"): "still 3",
		library.EpisodeItemID("tt0041038:1:4"): "still 3"} {
		if images := e.artworkOf(item); images.Primary == "" || e.pictured(images.Primary) != want {
			t.Errorf("%s: %+v", want, images)
		}
	}
	if images, ok := e.service.Artwork()[second]; ok {
		t.Errorf("an episode without an image: %+v", images)
	}

	// Added, replaced, then removed.
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E02.jpg"), []byte("still 2"), at)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E01-thumb.jpg"), []byte("still 1, again"), at)
	before := e.artworkOf(pilot).Primary
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if images := e.artworkOf(second); images.Primary == "" || e.pictured(images.Primary) != "still 2" {
		t.Errorf("an image added: %+v", images)
	}
	replaced := e.artworkOf(pilot).Primary
	if replaced == before || e.pictured(replaced) != "still 1, again" {
		t.Errorf("an image replaced: %q, was %q", replaced, before)
	}
	if err := os.Remove(filepath.Join(season, "The Lone Ranger - S01E01-thumb.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if images, ok := e.service.Artwork()[pilot]; ok {
		t.Errorf("an image removed: %+v", images)
	}
	key, _ := library.LocalArtworkKey(replaced)
	if _, _, err := e.service.Open(t.Context(), key); err != ErrGone {
		t.Errorf("a removed image opens: %v", err)
	}

	// Off, the episodes' images show no more; on, they are back.
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{UseArtwork: new(false)}); err != nil {
		t.Fatal(err)
	}
	if len(e.service.Artwork()) != 0 {
		t.Errorf("artwork with the option off: %v", e.service.Artwork())
	}
	if _, err := e.service.Update(t.Context(), addon.ID, Changes{UseArtwork: new(true)}); err != nil {
		t.Fatal(err)
	}
	if images := e.artworkOf(second); images.Primary == "" || e.pictured(images.Primary) != "still 2" {
		t.Errorf("with the option back on: %+v", images)
	}
}

// An episode's image on a share is kept and read from the share.
func TestShareEpisodeArtwork(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	season := filepath.Join(dir, "shows", "The Lone Ranger (1949)", "Season 01")
	write(t, filepath.Join(season, "The Lone Ranger - S01E01.mkv"), 10)
	still := content(50_000)
	writeImage(t, filepath.Join(season, "The Lone Ranger - S01E01-thumb.webp"), still, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	server := serveWebDAV(t, dir, "reader", "pass")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/shows/", Kind: KindShows, User: "reader",
		Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	images := e.artworkOf(library.EpisodeItemID("tt0041038:1:1"))
	key, ok := library.LocalArtworkKey(images.Primary)
	if !ok || !strings.HasSuffix(key, ".webp") {
		t.Fatalf("artwork: %+v", images)
	}
	if got := e.opened(key); got != string(still) {
		t.Errorf("the still from the share: %d bytes", len(got))
	}
}

// Of the images several folders hold for a title, the folder listed first
// among the sources gives each type; a disabled folder gives none.
func TestArtworkOfSeveralFolders(t *testing.T) {
	e := newEnv(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	add := func(name, poster string, backdrop bool) addons.Addon {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
		writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "poster.jpg"), []byte(poster), at)
		if backdrop {
			writeImage(t, filepath.Join(dir, "Nosferatu (1922)", "backdrop.jpg"), []byte(name+" backdrop"), at)
		}
		addon, err := e.service.Add(t.Context(), NewFolder{Name: name, Path: dir, Kind: KindMovies})
		if err != nil {
			t.Fatal(err)
		}
		e.idle(addon.ID)
		return addon
	}
	first := add("First", "first", false)
	second := add("Second", "second", true)
	nosferatu := library.TitleItemID(library.KindMovie, "tt0013442")
	images := e.artworkOf(nosferatu)
	if e.pictured(images.Primary) != "first" || e.pictured(images.Backdrop) != "Second backdrop" {
		t.Errorf("first folder first: %+v", images)
	}
	listed, err := e.addons.Addons(t.Context(), addons.Shared())
	if err != nil {
		t.Fatal(err)
	}
	var order []accounts.ID
	for _, addon := range listed {
		if addon.ID != first.ID && addon.ID != second.ID {
			order = append(order, addon.ID)
		}
	}
	if err := e.addons.Reorder(t.Context(), addons.Shared(), append(order, second.ID, first.ID)); err != nil {
		t.Fatal(err)
	}
	if images := e.artworkOf(nosferatu); e.pictured(images.Primary) != "second" {
		t.Errorf("second folder first: %+v", images)
	}
	if _, err := e.addons.SetEnabled(t.Context(), addons.Shared(), second.ID, false); err != nil {
		t.Fatal(err)
	}
	if images := e.artworkOf(nosferatu); e.pictured(images.Primary) != "first" || images.Backdrop != "" {
		t.Errorf("second folder disabled: %+v", images)
	}
}

// A share's images are kept as a local folder's, and read from the share
// by ranges.
func TestShareArtwork(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	poster := content(200_000)
	writeImage(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Poster.JPG"), poster, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	server := serveWebDAV(t, dir, "reader", "pass")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	images := e.artworkOf(library.TitleItemID(library.KindMovie, "tt0013442"))
	key, ok := library.LocalArtworkKey(images.Primary)
	if !ok || !strings.HasSuffix(key, ".jpg") {
		t.Fatalf("artwork: %+v", images)
	}
	file, _, err := e.service.Open(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Seek(150_000, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 10)
	if _, err := io.ReadFull(file, got); err != nil || !bytes.Equal(got, poster[150_000:150_010]) {
		t.Errorf("bytes at 150000: %v %v", got, err)
	}
}
