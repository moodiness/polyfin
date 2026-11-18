package playback

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent"
)

// The subtitle files inside a torrent listed for the video played are
// those named after it beside it, and those of a subtitles folder beside
// it that are its own: named after it, telling its episode, in a folder
// named after it or telling its episode, or telling nothing beside the
// only video of the folder. In a season's pack, another episode's are left
// out. Their names tell their language, title and flags as those beside
// local files do.
func TestTorrentSubtitlesGoWithTheVideo(t *testing.T) {
	t.Parallel()
	files := func(paths ...string) []torrent.File {
		list := make([]torrent.File, len(paths))
		for i, p := range paths {
			list[i] = torrent.File{Path: p, Length: 40 << 10}
		}
		return list
	}
	type want struct {
		path string
		tags SubtitleTags
	}
	for _, c := range []struct {
		name   string
		played string
		files  []torrent.File
		want   []want
	}{
		{"a movie", "Movie.2001/Movie.2001.mkv", files(
			"Movie.2001/Movie.2001.mkv",
			"Movie.2001/Movie.2001.fr.srt",
			"Movie.2001/Movie.2001.en.forced.ass",
			"Movie.2001/movie.2001.SDH.ENG.vtt",
			"Movie.2001/Movie.2001.nfo",
			"Movie.2001/Movie.2001.en.idx",
			"Movie.2001/Movie.2001.en.sub",
			"Movie.2001/Other.srt",
			"Movie.2001/Movie.2001-extras.en.srt",
			"Movie.2001/Subs/English.srt",
			"Movie.2001/Subs/2_Spanish.srt",
			"Movie.2001/Subs/Movie.2001.it.srt",
			"Movie.2001/Subs/Deeper/German.srt",
			"Movie.2001/Extras/Movie.2001.de.srt",
		), []want{
			{"Movie.2001/Movie.2001.fr.srt", SubtitleTags{Language: "fr"}},
			{"Movie.2001/Movie.2001.en.forced.ass", SubtitleTags{Language: "en", Forced: true}},
			{"Movie.2001/movie.2001.SDH.ENG.vtt", SubtitleTags{Language: "ENG", HearingImpaired: true}},
			{"Movie.2001/Subs/English.srt", SubtitleTags{Language: "English"}},
			{"Movie.2001/Subs/2_Spanish.srt", SubtitleTags{Language: "Spanish"}},
			{"Movie.2001/Subs/Movie.2001.it.srt", SubtitleTags{Language: "it"}},
		}},
		{"a season's pack", "Show.S01.1080p/Show.S01E02.1080p.mkv", files(
			"Show.S01.1080p/Show.S01E01.1080p.mkv",
			"Show.S01.1080p/Show.S01E02.1080p.mkv",
			"Show.S01.1080p/Show.S01E01.1080p.en.srt",
			"Show.S01.1080p/Show.S01E02.1080p.en.srt",
			"Show.S01.1080p/Subs/Show.S01E01.1080p/2_English.srt",
			"Show.S01.1080p/Subs/Show.S01E02.1080p/2_English.srt",
			"Show.S01.1080p/Subs/Show.S01E02.1080p/3_French.Commentary.srt",
			"Show.S01.1080p/Subs/Show.S01E03.1080p/2_English.srt",
			"Show.S01.1080p/Subs/English.srt",
			"Show.S01.1080p/Subs/Show.S01E02.Spanish.srt",
			"Show.S01.1080p/Subs/Show.S01E01.Spanish.srt",
		), []want{
			{"Show.S01.1080p/Show.S01E02.1080p.en.srt", SubtitleTags{Language: "en"}},
			{"Show.S01.1080p/Subs/Show.S01E02.1080p/2_English.srt", SubtitleTags{Language: "English"}},
			{"Show.S01.1080p/Subs/Show.S01E02.1080p/3_French.Commentary.srt", SubtitleTags{Language: "French", Title: "Commentary"}},
			{"Show.S01.1080p/Subs/Show.S01E02.Spanish.srt", SubtitleTags{Language: "Spanish"}},
		}},
		{"a longer name", "Movie/Movie.mkv", files(
			"Movie/Movie.mkv",
			"Movie/Movie.Extended.mkv",
			"Movie/Movie.en.srt",
			"Movie/Movie.Extended.en.srt",
		), []want{
			{"Movie/Movie.en.srt", SubtitleTags{Language: "en"}},
		}},
		{"at the torrent's root", "Movie.mkv", files(
			"Movie.mkv",
			"Movie.pt-BR.srt",
			"Subs/French.srt",
		), []want{
			{"Movie.pt-BR.srt", SubtitleTags{Language: "pt-BR"}},
			{"Subs/French.srt", SubtitleTags{Language: "French"}},
		}},
	} {
		got, _, err := torrentSubtitles(t.Context(), torrent.File{Path: c.played}, c.files)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: listed %+v, want %+v", c.name, got, c.want)
			continue
		}
		for i, w := range c.want {
			if f := got[i]; f.path != w.path || c.files[f.index].Path != f.path || f.SubtitleTags != w.tags {
				t.Errorf("%s: file %d is %+v, want %+v", c.name, i, f, w)
			}
		}
	}

	// A file larger than a subtitle is left out.
	large := files("Movie/Movie.mkv", "Movie/Movie.en.srt")
	large[1].Length = maxTorrentSubtitle + 1
	if got, _, err := torrentSubtitles(t.Context(), large[0], large); err != nil || len(got) != 0 {
		t.Errorf("a file of %d bytes was listed", large[1].Length)
	}
}

// The subtitle files that go with a video are found by looking each name
// up among the videos of longer names beside it, not by comparing it with
// each of them: four times the files ask four times the lookups, however
// many such videos there are.
func TestTorrentSubtitlesAreMatchedAsTheFilesGrow(t *testing.T) {
	t.Parallel()
	// A video and its subtitle file, then n videos beside it whose longer
	// names begin with its own, each with a subtitle file of its own.
	pack := func(n int) (torrent.File, []torrent.File) {
		played := torrent.File{Path: "p/a.mkv", Length: 1}
		files := []torrent.File{played, {Path: "p/a.en.srt", Length: 1}}
		for i := range n {
			files = append(files, torrent.File{Path: fmt.Sprintf("p/a.%06d.mkv", i), Length: 1},
				torrent.File{Path: fmt.Sprintf("p/a.%06d.en.srt", i), Length: 1})
		}
		return played, files
	}
	for _, n := range []int{1500, 6000} {
		played, files := pack(n)
		listed, lookups, err := torrentSubtitles(t.Context(), played, files)
		if err != nil || len(listed) != 1 || listed[0].path != "p/a.en.srt" {
			t.Fatalf("%d files: listed %+v, error %v, want the video's own file alone", len(files), listed, err)
		}
		// Each subtitle file's name is looked up once: whole for the
		// video's own, up to its number for another's.
		if lookups != n+1 {
			t.Fatalf("%d files took %d lookups, want %d, one for each subtitle file", len(files), lookups, n+1)
		}
	}
}

// A name looked up is told another video's in any letter case exactly as
// when it was compared with that video's, for the letters of more than
// two forms and the bytes that are no letter too: the files listed are
// those that comparing each name with each longer video's lists.
func TestTorrentSubtitlesLookedUpAreThoseCompared(t *testing.T) {
	t.Parallel()
	letters := []string{"a", "A", "k", "K", "\u212a", "s", "\u017f", "é", "É", "\xff", "\xfe"}
	// Videos whose names are equal to others' in any letter case with the
	// same bytes in another order, or with other bytes, as the Kelvin sign
	// is a "k" of three bytes and the long s an "s" of two.
	others := []string{"a.\u212ak", "a.s\u017f", "s.\u212a"}
	var names []string
	for _, x := range letters {
		for _, y := range letters[2:7] {
			for _, z := range letters[2:7] {
				names = append(names, x+"."+y+z)
			}
		}
	}
	for i, x := range letters {
		names = append(names, x)
		for j, y := range letters {
			names = append(names, x+y, x+"."+y)
			if (i+j)%3 == 0 {
				others = append(others, x+y, x+"."+y)
			}
			for k, z := range letters {
				names = append(names, x+"."+y+"."+z, x+y+"."+z)
				if (i+j+k)%7 == 0 {
					others = append(others, x+"."+y+"."+z)
				}
			}
		}
	}
	for _, stem := range append(slices.Clone(letters), "a.k", "K.s", "\xff.A") {
		played := torrent.File{Path: "d/" + stem + ".mkv"}
		files := []torrent.File{played}
		for _, other := range others {
			files = append(files, torrent.File{Path: "d/" + other + ".mkv"})
		}
		var want []string
		for _, name := range names {
			files = append(files, torrent.File{Path: "d/" + name + ".srt"})
			_, ours := SubtitleNamed(stem, name)
			if ours && !slices.ContainsFunc(others, func(other string) bool {
				_, theirs := SubtitleNamed(other, name)
				return theirs && len(other) > len(stem)
			}) {
				want = append(want, "d/"+name+".srt")
			}
		}
		if len(want) == 0 || len(want) >= maxTorrentSubtitles {
			t.Fatalf("%q: %d files to list, which tells nothing", stem, len(want))
		}
		listed, _, err := torrentSubtitles(t.Context(), played, files)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range listed {
			got = append(got, f.path)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%q: listed %q, want %q", stem, got, want)
		}
	}
}

// countedContext is a context that ends once its error was asked ends
// times.
type countedContext struct {
	context.Context
	asked, ends int
}

func (c *countedContext) Err() error {
	if c.asked++; c.asked > c.ends {
		return context.Canceled
	}
	return nil
}

// The listing ends with its context: one that ended lists nothing, and
// one that ends while the files are gone through ends it there. No more
// than maxTorrentSubtitles files are listed for a video, the first ones.
func TestTorrentSubtitlesEndWithTheirContextAndAreBounded(t *testing.T) {
	t.Parallel()
	played := torrent.File{Path: "Movie/Movie.mkv", Length: 1}
	files := []torrent.File{played}
	for i := range 5000 {
		files = append(files, torrent.File{Path: fmt.Sprintf("Movie/Movie.%04d.srt", i), Length: 1})
	}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	if listed, _, err := torrentSubtitles(ended, played, files); !errors.Is(err, context.Canceled) || listed != nil {
		t.Fatalf("under a context that ended: %d files listed, error %v", len(listed), err)
	}
	ending := &countedContext{Context: t.Context(), ends: 3}
	if listed, _, err := torrentSubtitles(ending, played, files); !errors.Is(err, context.Canceled) || listed != nil || ending.asked != 4 {
		t.Fatalf("under a context that ends at the fourth time it is asked: %d files listed, error %v, asked %d times",
			len(listed), err, ending.asked)
	}

	listed, _, err := torrentSubtitles(t.Context(), played, files)
	if err != nil || len(listed) != maxTorrentSubtitles {
		t.Fatalf("%d files listed, error %v, want %d", len(listed), err, maxTorrentSubtitles)
	}
	for i, f := range listed {
		if f.index != i+1 {
			t.Fatalf("file %d listed is the torrent's file %d, want the first ones", i, f.index)
		}
	}
}
