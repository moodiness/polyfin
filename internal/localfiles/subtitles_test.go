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
	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A subtitle file belongs to the video whose name its own begins with, in
// the same folder, the longest such name winning; its name then tells its
// language, title and flags in any order and case. A VobSub pair is its
// .idx file, with the .sub file of the same name.
func TestSubtitleFilesBelongToTheirVideo(t *testing.T) {
	read := func(kind string, videos, subtitles []string) map[string]sidecar {
		entries := map[string]entry{}
		for _, rel := range videos {
			entries[rel] = readEntry(kind, "Shelf", rel, found{})
		}
		sides := map[string]found{}
		for _, rel := range subtitles {
			sides[rel] = found{size: 1}
		}
		return sidecars(kind, "Shelf", entries, sides)
	}
	type want struct {
		video  string
		format string
		pair   string
		subtitleTags
	}
	check := func(got map[string]sidecar, wanted map[string]want) {
		t.Helper()
		simple := map[string]want{}
		for rel, s := range got {
			simple[rel] = want{video: s.video, format: s.format, pair: s.pair, subtitleTags: s.subtitleTags}
		}
		if !maps.Equal(simple, wanted) {
			t.Errorf("got %+v", simple)
		}
	}
	movie, version := "Movie (2001).mkv", "Movie (2001) - 1080p.mkv"
	check(read(KindMovies, []string{movie, version, "Disc (1999)/BDMV/STREAM/00001.m2ts", "BDMV/STREAM/00000.m2ts"}, []string{
		"Movie (2001).srt",
		"Movie (2001).fr.srt",
		"Movie (2001).fre.srt",
		"movie (2001).French.SRT",
		"Movie (2001).en.forced.srt",
		"Movie (2001).en.sdh.ass",
		"Movie (2001).Default.HI.EN.ssa",
		"Movie (2001).foreign.de.cc.vtt",
		"Movie (2001).pt-BR.Director.Commentary.srt",
		"Movie (2001) - 1080p.it.srt",
		"Disc (1999)/Disc (1999).es.srt",
		"Shelf.ja.srt",
		"Movie (2001).en.forced.sup",
		"Movie (2001).fr.idx",
		"Movie (2001).fr.sub",
		"Movie (2001).SDH.IDX",
		"Movie (2001).SDH.sub",
		"Disc (1999)/Disc (1999).de.sup",
		// Belonging to no video: another name, a name followed by no dot,
		// another folder, an NFO file, an .idx file without its .sub file
		// and a .sub file alone, which is text (MicroDVD).
		"Other Movie.fr.srt",
		"Movie (2001)x.srt",
		"Extras/Movie (2001).fr.srt",
		"Disc (1999)/BDMV/Disc (1999).srt",
		"Movie (2001).nfo",
		"Movie (2001).de.idx",
		"Movie (2001).it.sub",
	}), map[string]want{
		"Movie (2001).srt":                           {video: movie, format: "srt"},
		"Movie (2001).fr.srt":                        {video: movie, format: "srt", subtitleTags: subtitleTags{language: "fr"}},
		"Movie (2001).fre.srt":                       {video: movie, format: "srt", subtitleTags: subtitleTags{language: "fre"}},
		"movie (2001).French.SRT":                    {video: movie, format: "srt", subtitleTags: subtitleTags{language: "French"}},
		"Movie (2001).en.forced.srt":                 {video: movie, format: "srt", subtitleTags: subtitleTags{language: "en", forced: true}},
		"Movie (2001).en.sdh.ass":                    {video: movie, format: "ass", subtitleTags: subtitleTags{language: "en", hearingImpaired: true}},
		"Movie (2001).Default.HI.EN.ssa":             {video: movie, format: "ssa", subtitleTags: subtitleTags{language: "EN", hearingImpaired: true, isDefault: true}},
		"Movie (2001).foreign.de.cc.vtt":             {video: movie, format: "vtt", subtitleTags: subtitleTags{language: "de", forced: true, hearingImpaired: true}},
		"Movie (2001).pt-BR.Director.Commentary.srt": {video: movie, format: "srt", subtitleTags: subtitleTags{language: "pt-BR", title: "Director Commentary"}},
		"Movie (2001) - 1080p.it.srt":                {video: version, format: "srt", subtitleTags: subtitleTags{language: "it"}},
		"Disc (1999)/Disc (1999).es.srt":             {video: "Disc (1999)/BDMV/STREAM/00001.m2ts", format: "srt", subtitleTags: subtitleTags{language: "es"}},
		"Shelf.ja.srt":                               {video: "BDMV/STREAM/00000.m2ts", format: "srt", subtitleTags: subtitleTags{language: "ja"}},
		"Movie (2001).en.forced.sup":                 {video: movie, format: "sup", subtitleTags: subtitleTags{language: "en", forced: true}},
		"Movie (2001).fr.idx":                        {video: movie, format: "idx", pair: "Movie (2001).fr.sub", subtitleTags: subtitleTags{language: "fr"}},
		"Movie (2001).SDH.IDX":                       {video: movie, format: "idx", pair: "Movie (2001).SDH.sub", subtitleTags: subtitleTags{hearingImpaired: true}},
		"Disc (1999)/Disc (1999).de.sup":             {video: "Disc (1999)/BDMV/STREAM/00001.m2ts", format: "sup", subtitleTags: subtitleTags{language: "de"}},
	})
	episode := "Ranger Tales/Season 01/Ranger Tales - S01E01.mkv"
	check(read(KindShows, []string{episode}, []string{
		"Ranger Tales/Season 01/Ranger Tales - S01E01.fr.srt",
		"Ranger Tales/Season 01/Ranger Tales - S01E02.fr.srt",
	}), map[string]want{
		"Ranger Tales/Season 01/Ranger Tales - S01E01.fr.srt": {video: episode, format: "srt", subtitleTags: subtitleTags{language: "fr"}},
	})
}

// subtitles lists what Streams offers of the subtitle files of a folder's
// file: their language, title, flags and format.
func (e env) subtitles(id accounts.ID, title string) []stremio.Subtitle {
	e.t.Helper()
	streams, err := e.service.Streams(e.t.Context(), id, title)
	if err != nil || len(streams) != 1 {
		e.t.Fatalf("streams: %+v %v", streams, err)
	}
	return streams[0].Subtitles
}

// text reads a subtitle file as the loopback interface serves it.
func (e env) text(subtitle stremio.Subtitle) string {
	e.t.Helper()
	return e.opened(filepath.Base(subtitle.URL))
}

// opened reads the file a key names as the loopback interface serves it.
func (e env) opened(key string) string {
	e.t.Helper()
	file, _, err := e.service.Open(e.t.Context(), key)
	if err != nil {
		e.t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(data)
}

// The subtitle files beside a folder's videos are stored with them,
// offered with their streams and read as their videos are; a rescan finds
// those added, changed and removed, and a video's go with it.
func TestSubtitleFilesAreOfferedWithTheirVideo(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	at := time.Now()
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).fr.srt"), "1\n00:00:01,000 --> 00:00:02,000\nBonjour\n", at)
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).en.forced.ass"), "[Script Info]\n", at)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	subtitles := e.subtitles(addon.ID, "tt0063350")
	if len(subtitles) != 2 || subtitles[0].Lang != "en" || !subtitles[0].Forced || subtitles[0].Format != "ass" ||
		subtitles[1].Lang != "fr" || subtitles[1].Forced || subtitles[1].Format != "srt" || !subtitles[1].Valid() {
		t.Fatalf("subtitles: %+v", subtitles)
	}
	if got := e.text(subtitles[1]); got != "1\n00:00:01,000 --> 00:00:02,000\nBonjour\n" {
		t.Errorf("text: %q", got)
	}
	// A file changed gets another identifier, so that its text is read
	// again; one added is offered; one removed is not.
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).fr.srt"), "1\n00:00:01,000 --> 00:00:02,000\nSalut\n", at.Add(time.Minute))
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).de.sdh.vtt"), "WEBVTT\n", at)
	if err := os.Remove(filepath.Join(dir, "Night of the Living Dead (1968).en.forced.ass")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	changed := e.subtitles(addon.ID, "tt0063350")
	if len(changed) != 2 || changed[0].Lang != "de" || !changed[0].HearingImpaired || changed[0].Format != "vtt" ||
		changed[1].Lang != "fr" || changed[1].ID == subtitles[1].ID || changed[1].URL != subtitles[1].URL {
		t.Fatalf("after a rescan: %+v", changed)
	}
	if got := e.text(changed[1]); got != "1\n00:00:01,000 --> 00:00:02,000\nSalut\n" {
		t.Errorf("changed text: %q", got)
	}
	if err := os.Remove(filepath.Join(dir, "Night of the Living Dead (1968).mkv")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM local_subtitles WHERE addon_id = $1", addon.ID).Scan(&left); err != nil || left != 0 {
		t.Errorf("subtitle files of a video gone: %d %v", left, err)
	}
	if _, _, err := e.service.Open(t.Context(), filepath.Base(changed[1].URL)); err != ErrGone {
		t.Errorf("a subtitle file forgotten: %v", err)
	}
}

// A PGS file and a VobSub pair are offered as their video's subtitle
// files; a pair whose name tells no language has its .idx file's, and is
// read through its .idx file's key, its .sub file through the same key
// with the .sub extension. A changed .sub file changes the pair; one
// removed takes its pair with it.
func TestPictureSubtitleFilesAreOffered(t *testing.T) {
	e := newEnv(t)
	addon, dir := e.folder(KindMovies, map[string]int{"Night of the Living Dead (1968).mkv": 10})
	at := time.Now().Truncate(time.Second)
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).en.forced.sup"), "PG pictures", at)
	idx := "# VobSub index file, v7 (do not modify this line!)\nsize: 720x480\nid: de, index: 0\ntimestamp: 00:00:01:000, filepos: 000000000\n"
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).idx"), idx, at)
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).SUB"), "packs", at)
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	subtitles := e.subtitles(addon.ID, "tt0063350")
	if len(subtitles) != 2 || subtitles[0].Format != "sup" || subtitles[0].Lang != "en" || !subtitles[0].Forced ||
		!strings.HasSuffix(subtitles[0].URL, ".sup") || subtitles[1].Format != "idx" || subtitles[1].Lang != "de" ||
		!strings.HasSuffix(subtitles[1].URL, ".idx") {
		t.Fatalf("subtitles: %+v", subtitles)
	}
	pair := strings.TrimSuffix(filepath.Base(subtitles[1].URL), ".idx")
	if e.text(subtitles[0]) != "PG pictures" || e.opened(pair+".idx") != idx || e.opened(pair+".sub") != "packs" {
		t.Error("the files are not read as they are")
	}
	writeText(t, filepath.Join(dir, "Night of the Living Dead (1968).SUB"), "more packs", at.Add(time.Minute))
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if changed := e.subtitles(addon.ID, "tt0063350"); len(changed) != 2 || changed[1].ID == subtitles[1].ID || changed[1].Lang != "de" ||
		e.opened(pair+".sub") != "more packs" {
		t.Fatalf("after the .sub file changed: %+v", changed)
	}
	if err := os.Remove(filepath.Join(dir, "Night of the Living Dead (1968).SUB")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if left := e.subtitles(addon.ID, "tt0063350"); len(left) != 1 || left[0].Format != "sup" {
		t.Errorf("after the .sub file was removed: %+v", left)
	}
}

// A WebDAV share's subtitle files are listed with its folders and read
// from the share, a VobSub pair's language from its .idx file there.
func TestShareSubtitleFiles(t *testing.T) {
	e := newEnv(t)
	box, err := secrets.New(bytes.Repeat([]byte{7}, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e.service.box = box
	dir := t.TempDir()
	at := time.Now().Add(-time.Hour).Truncate(time.Second)
	write(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).mkv"), 10)
	writeText(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).fr.srt"), "1\n00:00:01,000 --> 00:00:02,000\nD\xe9j\xe0\n", at)
	writeText(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).sdh.idx"), "size: 720x576\nid: it, index: 0\n", at)
	writeText(t, filepath.Join(dir, "movies", "Nosferatu (1922)", "Nosferatu (1922).sdh.sub"), "packs", at)
	server := serveWebDAV(t, dir, "reader", "pass phrase")
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "NAS", Path: server.URL + "/movies/", Kind: KindMovies, User: "reader",
		Password: "pass phrase"})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)
	subtitles := e.subtitles(addon.ID, "tt0013442")
	if len(subtitles) != 2 || subtitles[0].Lang != "fr" || subtitles[1].Format != "idx" || subtitles[1].Lang != "it" || !subtitles[1].HearingImpaired {
		t.Fatalf("subtitles: %+v", subtitles)
	}
	if got := e.text(subtitles[0]); got != "1\n00:00:01,000 --> 00:00:02,000\nD\xe9j\xe0\n" {
		t.Errorf("text: %q", got)
	}
	if got := e.opened(strings.TrimSuffix(filepath.Base(subtitles[1].URL), ".idx") + ".sub"); got != "packs" {
		t.Errorf("the .sub file: %q", got)
	}
	for _, name := range []string{"Nosferatu (1922).fr.srt", "Nosferatu (1922).sdh.idx"} {
		if err := os.Remove(filepath.Join(dir, "movies", "Nosferatu (1922)", name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if got := e.subtitles(addon.ID, "tt0013442"); len(got) != 0 {
		t.Errorf("a removed file: %+v", got)
	}
}
