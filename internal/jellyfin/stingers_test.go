package jellyfin

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/mediasegments"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// ms is a time in milliseconds.
func ms(n int64) time.Duration { return time.Duration(n) * time.Millisecond }

// line is a cue from and to times in milliseconds.
func line(from, to int64, text ...string) subtitles.Cue {
	return subtitles.Cue{Start: ms(from), End: ms(to), Lines: text}
}

// talk is a line of dialogue every 4 seconds from from, for n lines.
func talk(from int64, n int) []subtitles.Cue {
	var cues []subtitles.Cue
	for i := range n {
		at := from + int64(i)*4_000
		cues = append(cues, line(at, at+3_000, fmt.Sprintf("Line %d.", i+1)))
	}
	return cues
}

// lyricsFrom is a lyric every 5 seconds from from, for n lyrics, marked as
// subtitles mark them.
func lyricsFrom(from int64, n int, mark func(string) string) []subtitles.Cue {
	var cues []subtitles.Cue
	for i := range n {
		at := from + int64(i)*5_000
		cues = append(cues, line(at, at+4_500, mark(fmt.Sprintf("Lyric number %d", i+1))))
	}
	return cues
}

func TestStingerChapterNames(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{
		"Post-Credits Scene": true, "Post Credit Scene": true, "POSTCREDITS": true, "Mid-credits scene": true,
		"Midcredit": true, "After the Credits": true, "After-credits": true, "Stinger": true,
		"Scène post-générique": true, "Scene mi-generique": true, "Après le générique": true,
		"Credits": false, "End Credits": false, "Postman": false, "Stingers of the Sea": false, "Generique": false,
		"Chapter 12": false,
	} {
		_, text := chapterWords(name)
		if got := stingerText(text); got != want {
			t.Errorf("%q: got %v, want %v", name, got, want)
		}
		if want && chapterKind(name) != "" {
			t.Errorf("%q is a segment of type %q", name, chapterKind(name))
		}
	}
}

// Chapters named as a scene during or after the credits are not credits,
// and split the credits; a short chapter right after credits chapters
// already ends them.
func TestChapterLayouts(t *testing.T) {
	t.Parallel()
	databases := []mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_200_000)}}
	for _, test := range []struct {
		name     string
		analysis media.Analysis
		want     []mediasegments.Segment
	}{
		{"credits then a post-credits scene", lasting(7_200_000, chapter("Movie", 0, 6_900_000),
			chapter("Credits", 6_900_000, 7_100_000), chapter("Post-Credits Scene", 7_100_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_100_000)}}},
		{"a mid-credits scene between two credits", lasting(7_200_000, chapter("Movie", 0, 6_900_000),
			chapter("End Credits", 6_900_000, 7_000_000), chapter("Mid-credits scene", 7_000_000, 7_060_000),
			chapter("Credits", 7_060_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_000_000)},
				{Type: mediasegments.Outro, Start: ms(7_060_000), End: ms(7_199_000)}}},
		{"a scene named in generic chapters splits the databases' credits", lasting(7_200_000,
			chapter("Chapter 1", 0, 3_600_000), chapter("Chapter 2", 3_600_000, 7_000_000),
			chapter("Post-credits", 7_000_000, 7_060_000), chapter("Chapter 4", 7_060_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_000_000)},
				{Type: mediasegments.Outro, Start: ms(7_060_000), End: ms(7_199_000)}}},
		{"a scene at the end", lasting(7_200_000, chapter("Chapter 1", 0, 7_150_000), chapter("Stinger", 7_150_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_150_000)}}},
		{"a short chapter after credits", lasting(7_200_000, chapter("Chapter 1", 0, 6_900_000),
			chapter("End Credits", 6_900_000, 7_080_000), chapter("Chapter 9", 7_080_000, 7_140_000),
			chapter("Chapter 10", 7_140_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_080_000)}}},
		{"a scene ending 2 s before the credits' end leaves no credits after it", lasting(7_200_000,
			chapter("Chapter 1", 0, 7_100_000), chapter("After Credits", 7_100_000, 7_197_000), chapter("Logo", 7_197_000, 7_200_000)),
			[]mediasegments.Segment{{Type: mediasegments.Outro, Start: ms(6_900_000), End: ms(7_100_000)}}},
	} {
		segments := versionSegments(slices.Clone(databases), test.analysis, ms(7_200_000))
		segments = splitCredits(segments, chapterStingers(test.analysis))
		if got := fitSegments(segments, test.analysis.Duration, ms(7_200_000)); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

// Dialogue resuming in the credits after 20 seconds without any, for
// several lines, is a stinger; lyrics, descriptions and a line or two are
// not.
func TestDialogueStingers(t *testing.T) {
	t.Parallel()
	credits := mediasegments.Segment{Type: mediasegments.Outro, Start: ms(6_000_000), End: ms(6_600_000)}
	notes := func(s string) string { return "♪ " + s + " ♪" }
	hashes := func(s string) string { return "# " + s + " #" }
	italic := func(s string) string { return "<i>♪ " + s + "</i>" }
	join := func(parts ...[]subtitles.Cue) []subtitles.Cue { return slices.Concat(parts...) }
	for _, test := range []struct {
		name string
		cues []subtitles.Cue
		want []stinger
	}{
		{"dialogue resuming", join(talk(5_900_000, 20), talk(6_300_000, 5)),
			[]stinger{{ms(6_295_000), ms(6_319_000)}}},
		{"dialogue right after the credits start goes on from before", join(talk(5_960_000, 30)), nil},
		{"dialogue starting under the credits", join(talk(6_005_000, 10)), nil},
		{"credits with only lyrics", join(talk(5_900_000, 20), lyricsFrom(6_000_000, 100, notes)), nil},
		{"lyrics marked with hashes", join(lyricsFrom(6_030_000, 50, hashes)), nil},
		{"lyrics in italics", join(lyricsFrom(6_030_000, 50, italic)), nil},
		{"lyrics, then dialogue", join(lyricsFrom(6_000_000, 60, notes), talk(6_400_000, 4)),
			[]stinger{{ms(6_395_000), ms(6_415_000)}}},
		{"descriptions in brackets", join(talk(5_900_000, 20), []subtitles.Cue{
			line(6_100_000, 6_104_000, "[music playing]"), line(6_110_000, 6_114_000, "(SINGING)"),
			line(6_120_000, 6_124_000, "[door closes]"), line(6_130_000, 6_134_000, "<i>[LAUGHTER]</i>")}), nil},
		{"a credit line or two", join(talk(5_900_000, 20), []subtitles.Cue{
			line(6_500_000, 6_504_000, "Subtitles by a fan"), line(6_505_000, 6_509_000, "Synced and corrected")}), nil},
		{"a mid-credits and a post-credits scene", join(talk(6_100_000, 3), talk(6_500_000, 6)),
			[]stinger{{ms(6_095_000), ms(6_111_000)}, {ms(6_495_000), ms(6_523_000)}}},
		{"dialogue after the credits", join(talk(6_600_000, 10)), nil},
		{"dialogue lasting more than 10 minutes", join(talk(6_100_000, 160)), nil},
		{"a speaker's name and a line in a description", join(talk(6_100_000, 2),
			[]subtitles.Cue{line(6_108_000, 6_110_000, "[Man] Hello.")}),
			[]stinger{{ms(6_095_000), ms(6_110_000)}}},
	} {
		if got := dialogueStingers(test.cues, credits); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestSplitCredits(t *testing.T) {
	t.Parallel()
	intro := mediasegments.Segment{Type: mediasegments.Intro, Start: 0, End: ms(60_000)}
	credits := mediasegments.Segment{Type: mediasegments.Outro, Start: ms(100_000), End: ms(200_000)}
	outro := func(from, to int64) mediasegments.Segment {
		return mediasegments.Segment{Type: mediasegments.Outro, Start: ms(from), End: ms(to)}
	}
	for _, test := range []struct {
		name     string
		stingers []stinger
		want     []mediasegments.Segment
	}{
		{"none", nil, []mediasegments.Segment{intro, credits}},
		{"within", []stinger{{ms(150_000), ms(160_000)}}, []mediasegments.Segment{intro, outro(100_000, 150_000), outro(160_000, 200_000)}},
		{"to the end", []stinger{{ms(170_000), ms(200_000)}}, []mediasegments.Segment{intro, outro(100_000, 170_000)}},
		{"overlapping ones", []stinger{{ms(150_000), ms(160_000)}, {ms(155_000), ms(170_000)}},
			[]mediasegments.Segment{intro, outro(100_000, 150_000), outro(170_000, 200_000)}},
		{"leaving under 3 s", []stinger{{ms(102_000), ms(198_000)}}, []mediasegments.Segment{intro}},
		{"outside", []stinger{{ms(10_000), ms(20_000)}, {ms(200_000), ms(210_000)}}, []mediasegments.Segment{intro, credits}},
	} {
		if got := splitCredits([]mediasegments.Segment{intro, credits}, test.stingers); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

// A file whose chapters name credits, a post-credits scene and credits
// again gives two credits, which jellyfin-web offers in turn, skipping
// the first landing on the scene. The last ends a second before the end.
func TestPostCreditsChapterOfAVideo(t *testing.T) {
	t.Parallel()
	p := skipping(t)
	versions := p.versions(t, p.third)
	p.storeAnalysis(t, versions[0], chapteredVideo(t, 60_000,
		chapter("Episode", 0, 30_000), chapter("Credits", 30_000, 45_000),
		chapter("Post-credits scene", 45_000, 52_000), chapter("Credits", 52_000, 60_000)))
	want := []span{spanMS("Outro", 30_000, 45_000), spanMS("Outro", 52_000, 59_000)}
	for _, path := range []string{p.third, versions[0].ID.String() + "?includeSegmentTypes=Outro"} {
		if got := spans(p.segments(t, "/MediaSegments/"+path)); !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", path, got, want)
		}
	}
}

// srt writes cues as a SubRip file.
func srt(cues []subtitles.Cue) []byte {
	stamp := func(d time.Duration) string {
		return fmt.Sprintf("%02d:%02d:%02d,%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, d.Milliseconds()%1000)
	}
	var b strings.Builder
	for i, cue := range cues {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, stamp(cue.Start), stamp(cue.End), strings.Join(cue.Lines, "\n"))
	}
	return []byte(b.String())
}

// The databases time the third episode's credits from 51:12 to 55:04. Its
// versions' dialogue, already at hand, splits them where it resumes: a
// text track inside the file read before, or an addon subtitle synced to
// the version, timed as synced. Lyrics do not split them, and a version
// not analyzed keeps the databases' credits.
func TestDialogueSplitsTheCredits(t *testing.T) {
	t.Parallel()
	p := skipping(t, mediasegments.TheIntroDB)
	versions := p.versions(t, p.third)
	withText := lasting(3_300_000)
	withText.Streams = []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "subtitle", Codec: "subrip", Forced: true},
		{Index: 2, Type: "subtitle", Codec: "subrip"}}
	notes := func(s string) string { return "♪ " + s + " ♪" }
	dialogue := slices.Concat(talk(2_900_000, 30), lyricsFrom(3_080_000, 20, notes), talk(3_200_000, 4))
	lyrics := slices.Concat(talk(2_900_000, 30), lyricsFrom(3_080_000, 40, notes))
	for i, cues := range [][]subtitles.Cue{dialogue, lyrics} {
		p.storeAnalysis(t, versions[i], withText)
		if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_subtitle_tracks (version_id, stream, format, data) VALUES ($1, 2, 'srt', $2)",
			versions[i].ID, srt(cues)); err != nil {
			t.Fatal(err)
		}
	}
	// The forced track holds only the scene's lines, which would show
	// dialogue resuming anywhere.
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_subtitle_tracks (version_id, stream, format, data) VALUES ($1, 1, 'srt', $2)",
		versions[1].ID, srt(slices.Concat(talk(3_100_000, 20), talk(3_250_000, 5)))); err != nil {
		t.Fatal(err)
	}
	// The addon's subtitle runs 2 s early; synced, it is shifted 2 s later.
	p.storeAnalysis(t, versions[2], lasting(3_300_000))
	subtitle := accounts.ID{7, 2, 0}
	if _, err := p.pool.Exec(t.Context(), `INSERT INTO version_speech (version_id, reference, windows, speech) VALUES ($1, 'audio', '[]', '[]')`,
		versions[2].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO subtitle_syncs (version_id, subtitle_id, ratio, offset_ms) VALUES ($1, $2, 1, 2000)",
		versions[2].ID, subtitle); err != nil {
		t.Fatal(err)
	}
	early := slices.Clone(dialogue)
	for i := range early {
		early[i].Start, early[i].End = early[i].Start-2*time.Second, early[i].End-2*time.Second
	}
	p.handler.subtitleCache.Put(subtitle, subtitleText{cues: early})

	intro := spanMS("Intro", 321_400, 400_400)
	split := []span{intro, spanMS("Outro", 3_072_000, 3_195_000), spanMS("Outro", 3_215_000, 3_300_000)}
	for _, test := range []struct {
		name, path string
		want       []span
	}{
		{"a text track", versions[0].ID.String(), split},
		{"lyrics only", versions[1].ID.String(), []span{intro, spanMS("Outro", 3_072_000, 3_300_000)}},
		{"a synced addon subtitle", versions[2].ID.String(), split},
		{"not analyzed", versions[3].ID.String(), []span{intro, spanMS("Outro", 3_072_000, 3_304_000)}},
	} {
		if got := spans(p.segments(t, "/MediaSegments/"+test.path)); !slices.Equal(got, test.want) {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
	if reads := p.reads.Load(); reads != 0 {
		t.Errorf("the files were read %d times", reads)
	}
}
