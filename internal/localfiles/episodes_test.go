package localfiles

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/secrets"
)

// A show's disc lists the titles of at least minEpisode that play parts of
// their own: a "play all" title chaining two or more others is left out,
// whatever its order or the short parts of its own, and so are a title
// replaying another's parts and the extras; the episodes come in the order
// of their numbers, at most maxDiscEpisodes.
func TestChooseEpisodes(t *testing.T) {
	part := func(source string, from, to uint64, minutes int) titlePart {
		return titlePart{key: source, length: time.Duration(minutes) * time.Minute, source: source, from: from, to: to}
	}
	title := func(number int, parts ...titlePart) titleCandidate {
		c := titleCandidate{number: number, parts: parts}
		for _, p := range parts {
			c.length += p.length
		}
		return c
	}
	numbers := func(candidates []titleCandidate) []int {
		var got []int
		for _, i := range chooseEpisodes(candidates) {
			got = append(got, candidates[i].number)
		}
		return got
	}
	a, b, c := part("1", 1000, 2000, 22), part("1", 2000, 3000, 22), part("1", 3000, 4000, 22)
	var many []titleCandidate
	for i := range 45 {
		many = append(many, title(45-i, part("1", uint64(i*100), uint64(i*100+100), 4)))
	}
	for _, tc := range []struct {
		name       string
		candidates []titleCandidate
		want       []int
	}{
		{"a DVD's play all, extra and repeat", []titleCandidate{
			title(1, a, b, c), title(2, a), title(3, b), title(4, c), title(5, part("1", 5000, 5100, 2)), title(6, b),
		}, []int{2, 3, 4}},
		{"a play all in another order, with an intro of its own", []titleCandidate{
			title(7, b), title(3, part("1", 0, 100, 1), b, a), title(5, a),
		}, []int{5, 7}},
		{"episodes in one clip", []titleCandidate{
			title(0, part("00001", 0, 300, 66)), title(1, part("00001", 0, 100, 22)), title(2, part("00001", 100, 200, 22)),
			title(3, part("00001", 200, 300, 22)), title(4, part("00002", 0, 10, 2)),
		}, []int{1, 2, 3}},
		{"a title holding one other", []titleCandidate{title(1, a), title(2, a, b)}, []int{1, 2}},
		{"two titles mostly new", []titleCandidate{title(1, a, b, part("1", 9000, 99000, 200)), title(2, a), title(3, b)}, []int{1, 2, 3}},
		{"extras only", []titleCandidate{title(1, part("1", 0, 10, 2)), title(2, part("1", 10, 20, 2))}, nil},
		{"the cap", many, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28,
			29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40}},
	} {
		if got := numbers(tc.candidates); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v", tc.name, got)
		}
	}
}

// A show's disc's season is the one its name gives with its episodes, else
// its season folder's, else the one its name gives, else 1; its number
// among the season's discs is the one its name gives.
func TestPlaceOfDiscs(t *testing.T) {
	for disc, want := range map[string]discPlace{
		"Show/Season 2/Disc 3":             {season: 2, number: 3},
		"Show/Season 2/Box/DISC04":         {season: 2, number: 4},
		"Show/Saison 4 DVD 1":              {season: 4, number: 1},
		"Show/Disc 1":                      {season: 1, number: 1},
		"Show/Specials/Disc 1":             {season: 0, number: 1},
		"Show/Season 05":                   {season: 5},
		"Show/Season 01/Show S02E05-E08":   {season: 2, named: name{title: "Show", season: 2, episode: 5, lastEpisode: 8, episodic: true}},
		"Show/Season 01/Show Vol. 2.iso":   {season: 1, number: 2},
		"Show/Season 01/Show (2004) Box 1": {season: 1},
	} {
		if got := placeOf(disc); got != want {
			t.Errorf("%s: %+v", disc, got)
		}
	}
	for text, want := range map[string]string{"S01E05": "S01E05", " s1e5-e6 ": "S01E05-E06", "S01E05E06": "S01E05-E06",
		"S02E10-11": "S02E10-E11", "Show S01E05": "", "1x05": "", "E05": "", "S01E05 extra": "", "": ""} {
		n, ok := readEpisode(text)
		if got := formatEpisode(n); got != want || ok != (want != "") {
			t.Errorf("%q: %q %v", text, got, ok)
		}
	}
}

// showDisc writes in dir a shows folder holding The Lone Ranger's discs:
// Season 01's Disc 1, a DVD whose play all title 1 chains episodes 2, 3
// and 4, beside a short extra, title 5, and Disc 2, a Blu-ray disc whose
// playlist 00000 chains episodes 00001 and 00002, beside a short extra; and
// Season 02's image, named after its two episodes, a Blu-ray disc's with a
// play all too. An image at the folder's top belongs to no show.
func writeShowDiscs(t *testing.T, dir string) {
	t.Helper()
	cells := func(first uint32) []ifoCell { return []ifoCell{{first: first, last: first + 999, seconds: 1320}} }
	dvd := filepath.Join(dir, "The Lone Ranger", "Season 01", "Disc 1", "VIDEO_TS")
	writeIFOs(t, dvd,
		ifoTitle{set: 1, seconds: 3960, cells: slices.Concat(cells(1000), cells(2000), cells(3000))},
		ifoTitle{set: 1, seconds: 1320, cells: cells(1000)},
		ifoTitle{set: 1, seconds: 1320, cells: cells(2000)},
		ifoTitle{set: 1, seconds: 1320, cells: cells(3000)},
		ifoTitle{set: 1, seconds: 90, cells: []ifoCell{{first: 5000, last: 5099, seconds: 90}}})
	write(t, filepath.Join(dvd, "VTS_01_1.VOB"), 100)
	bluray := t.TempDir()
	playlists := filepath.Join(bluray, "BDMV", "PLAYLIST")
	writeMPLS(t, filepath.Join(playlists, "00000.mpls"), mplsItem{"00001", 1320}, mplsItem{"00002", 1320})
	writeMPLS(t, filepath.Join(playlists, "00001.mpls"), mplsItem{"00001", 1320})
	writeMPLS(t, filepath.Join(playlists, "00002.mpls"), mplsItem{"00002", 1320})
	writeMPLS(t, filepath.Join(playlists, "00003.mpls"), mplsItem{"00003", 60})
	for clip, size := range map[string]int{"00001": 300, "00002": 400, "00003": 10} {
		write(t, filepath.Join(bluray, "BDMV", "STREAM", clip+".m2ts"), size)
	}
	files := map[string][]byte{}
	if err := filepath.WalkDir(bluray, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(bluray, p)
		data, err := os.ReadFile(p)
		files[filepath.ToSlash(rel)] = data
		return err
	}); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Join(dir, "The Lone Ranger", "Season 01", "Disc 2"), files)
	writeUDF(t, filepath.Join(dir, "The Lone Ranger", "Season 02", "The Lone Ranger S02E01-E02.iso"), files, udfLayout{metadata: true})
	writeUDF(t, filepath.Join(dir, "Loose.iso"), files, udfLayout{metadata: true})
}

// The titles of a show's discs, a DVD, a Blu-ray disc and an image, are
// its episodes, each an entry of the show matched with it: those whose
// episodes neither a name nor an administrator gives are unmatched,
// listed with their disc, number and length, and the episodes Polyfin
// suggests, after those the season's discs before them give. The episodes
// given stay through scans, play as movie discs' titles do, and suggest
// the next disc's; a disc named after its episodes is numbered in order.
func TestShowDiscsAreEpisodes(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	writeShowDiscs(t, dir)
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Shows", Path: dir, Kind: KindShows})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	const (
		dvd    = "The Lone Ranger/Season 01/Disc 1"
		bluray = "The Lone Ranger/Season 01/Disc 2"
		image  = "The Lone Ranger/Season 02/The Lone Ranger S02E01-E02.iso"
	)
	want := map[string]string{
		dvd + "/VIDEO_TS/VTS_01_1.VOB#2":     "unmatched: disc_episode",
		dvd + "/VIDEO_TS/VTS_01_1.VOB#3":     "unmatched: disc_episode",
		dvd + "/VIDEO_TS/VTS_01_1.VOB#4":     "unmatched: disc_episode",
		bluray + "/BDMV/PLAYLIST/00001.mpls": "unmatched: disc_episode",
		bluray + "/BDMV/PLAYLIST/00002.mpls": "unmatched: disc_episode",
		image + "#1":                         "tt0041038",
		image + "#2":                         "tt0041038",
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Fatalf("files: %v", got)
	}
	for rel, r := range e.reads(addon.ID) {
		if r != (read{unit: "The Lone Ranger", title: "The Lone Ranger"}) {
			t.Errorf("%s: %+v", rel, r)
		}
	}
	type row struct {
		path, unit, disc string
		size             int64
		title            DiscTitle
	}
	unmatched := func() []row {
		t.Helper()
		list, total, err := e.service.Unmatched(t.Context(), addon.ID)
		if err != nil || total != len(list) {
			t.Fatalf("unmatched: %d %v", total, err)
		}
		var rows []row
		for _, u := range list {
			if u.DiscTitle == nil || u.Reason != reasonDiscEpisode {
				t.Fatalf("%s: %+v", u.Path, u)
			}
			rows = append(rows, row{u.Path, u.Unit, u.Disc, u.Size, *u.DiscTitle})
		}
		return rows
	}
	episode := 22 * time.Minute
	if got, want := unmatched(), []row{
		{dvd + "/VIDEO_TS/VTS_01_1.VOB#2", "The Lone Ranger", DiscDVD, 1000 * ifoSector, DiscTitle{dvd, 2, episode, "", "S01E01"}},
		{dvd + "/VIDEO_TS/VTS_01_1.VOB#3", "The Lone Ranger", DiscDVD, 1000 * ifoSector, DiscTitle{dvd, 3, episode, "", "S01E02"}},
		{dvd + "/VIDEO_TS/VTS_01_1.VOB#4", "The Lone Ranger", DiscDVD, 1000 * ifoSector, DiscTitle{dvd, 4, episode, "", "S01E03"}},
		{bluray + "/BDMV/PLAYLIST/00001.mpls", "The Lone Ranger", DiscBluRay, 300, DiscTitle{bluray, 1, episode, "", "S01E04"}},
		{bluray + "/BDMV/PLAYLIST/00002.mpls", "The Lone Ranger", DiscBluRay, 400, DiscTitle{bluray, 2, episode, "", "S01E05"}},
	}; !slices.Equal(got, want) {
		t.Errorf("unmatched:\n%+v\nwant\n%+v", got, want)
	}

	// Disc 1 numbered, its last title holding two episodes: Disc 2's
	// suggestions follow.
	if err := e.service.SetEpisodes(t.Context(), addon.ID, []EpisodeChange{{dvd + "/VIDEO_TS/VTS_01_1.VOB#2", "S01E01"},
		{dvd + "/VIDEO_TS/VTS_01_1.VOB#3", "s1e2"}, {dvd + "/VIDEO_TS/VTS_01_1.VOB#4", "S01E03-E04"}}); err != nil {
		t.Fatal(err)
	}
	if got := unmatched(); len(got) != 2 || got[0].title.Suggested != "S01E05" || got[1].title.Suggested != "S01E06" {
		t.Errorf("Disc 2's suggestions: %+v", got)
	}
	for _, bad := range []EpisodeChange{{dvd + "/VIDEO_TS/VTS_01_1.VOB#2", "Episode 1"}, {dvd + "/VIDEO_TS/VTS_01_1.VOB#2", "S01"}} {
		if err := e.service.SetEpisodes(t.Context(), addon.ID, []EpisodeChange{bad}); !errors.Is(err, ErrInvalidEpisode) {
			t.Errorf("%q: %v", bad.Episode, err)
		}
	}
	for _, other := range []string{image, "The Lone Ranger", "nowhere.VOB#2"} {
		if err := e.service.SetEpisodes(t.Context(), addon.ID, []EpisodeChange{{other, "S01E09"}}); !errors.Is(err, addons.ErrNotFound) {
			t.Errorf("%s: %v", other, err)
		}
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || !slices.Equal(folder.Episodes, []GivenEpisode{{dvd + "/VIDEO_TS/VTS_01_1.VOB#2", DiscDVD, dvd, 2, "S01E01"},
		{dvd + "/VIDEO_TS/VTS_01_1.VOB#3", DiscDVD, dvd, 3, "S01E02"}, {dvd + "/VIDEO_TS/VTS_01_1.VOB#4", DiscDVD, dvd, 4, "S01E03-E04"}}) {
		t.Errorf("the folder's episodes: %+v %v", folder.Episodes, err)
	}

	// The episodes given stay through scans, the disc changed or not.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(dir, filepath.FromSlash(dvd), "VIDEO_TS", "VIDEO_TS.IFO"), later, later); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := e.service.Scan(t.Context(), addon.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"2", "3", "4"} {
		want[dvd+"/VIDEO_TS/VTS_01_1.VOB#"+n] = "tt0041038"
	}
	if got := e.files(addon.ID); !maps.Equal(got, want) {
		t.Errorf("files once numbered: %v", got)
	}

	// Each episode is a version of the show's, played from its disc.
	play := func(id string, filename string, size int64, args []string, at string) {
		t.Helper()
		streams, err := e.service.Streams(t.Context(), addon.ID, id)
		if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != filename || int64(streams[0].BehaviorHints.VideoSize) != size ||
			!strings.HasPrefix(streams[0].URL, "http://inputs.example/") {
			t.Fatalf("%s: %+v %v", id, streams, err)
		}
		in, err := e.service.Input(t.Context(), strings.TrimPrefix(streams[0].URL, "http://inputs.example/"))
		real, _ := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(at)))
		if err != nil || !slices.Equal(in.Args(), args) || strings.TrimPrefix(in.Path, "bluray:") != real {
			t.Errorf("%s's input: %+v %v", id, in, err)
		}
	}
	play("tt0041038:1:4", "Season 01 - Disc 1 - title 4.VOB", 1000*ifoSector, []string{"-f", "dvdvideo", "-title", "4"}, dvd+"/VIDEO_TS")
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(image)))
	if err != nil {
		t.Fatal(err)
	}
	play("tt0041038:2:2", "The Lone Ranger S02E01-E02 - title 2.iso", info.Size(), []string{"-playlist", "2"}, image)
	if err := e.service.SetEpisodes(t.Context(), addon.ID, []EpisodeChange{{bluray + "/BDMV/PLAYLIST/00001.mpls", "S01E05"}}); err != nil {
		t.Fatal(err)
	}
	play("tt0041038:1:5", "Season 01 - Disc 2 - title 1.m2ts", 300, []string{"-playlist", "1"}, bluray)
	meta, err := e.service.Meta(t.Context(), addon.ID, "tt0041038")
	var videos []string
	for _, v := range meta.Videos {
		videos = append(videos, v.ID)
	}
	if err != nil || !slices.Equal(videos, []string{"tt0041038:1:1", "tt0041038:1:2", "tt0041038:1:3", "tt0041038:1:4", "tt0041038:1:5",
		"tt0041038:2:1", "tt0041038:2:2"}) {
		t.Errorf("the show's episodes: %v %v", videos, err)
	}

	// Removed, a title's episodes are suggested again.
	if err := e.service.SetEpisodes(t.Context(), addon.ID, []EpisodeChange{{dvd + "/VIDEO_TS/VTS_01_1.VOB#4", ""}}); err != nil {
		t.Fatal(err)
	}
	if got := unmatched(); len(got) != 2 || got[0].title.Number != 4 || got[0].title.Suggested != "S01E03" ||
		got[1].title != (DiscTitle{bluray, 2, episode, "", "S01E06"}) {
		t.Errorf("once removed: %+v", got)
	}
}

// A show's discs on a share are read as on a local folder: the same
// episodes, numbered by an image's name.
func TestShareShowDiscsAreEpisodes(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	writeShowDiscs(t, dir)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	share, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/", Kind: KindShows, User: "reader", Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(share.ID)
	image := "The Lone Ranger/Season 02/The Lone Ranger S02E01-E02.iso"
	if got := e.files(share.ID); len(got) != 7 || got[image+"#1"] != "tt0041038" || got[image+"#2"] != "tt0041038" ||
		got["The Lone Ranger/Season 01/Disc 1/VIDEO_TS/VTS_01_1.VOB#3"] != "unmatched: disc_episode" ||
		got["The Lone Ranger/Season 01/Disc 2/BDMV/PLAYLIST/00002.mpls"] != "unmatched: disc_episode" {
		t.Errorf("files: %v", got)
	}
}
