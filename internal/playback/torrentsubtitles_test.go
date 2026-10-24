package playback

import (
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
		got := torrentSubtitles(torrent.File{Path: c.played}, c.files)
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
	if got := torrentSubtitles(large[0], large); len(got) != 0 {
		t.Errorf("a file of %d bytes was listed", large[1].Length)
	}
}
