package localfiles

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
)

func TestHomeVideoNames(t *testing.T) {
	entries := map[string]entry{}
	for _, rel := range []string{"Beach.mp4", "Birthday 2019/clip.mp4", "Birthday 2019/clip.en.srt", "Trip/day 1.mov", "Trip/day 2.mov",
		"Trip/Evening/fire.mkv"} {
		if videoFile(rel) {
			entries[rel] = entry{path: rel, unit: rel}
		}
	}
	own := videoFolders(entries)
	for rel, want := range map[string]string{
		// Alone at the top, it is not the folder's own.
		"Beach.mp4": "Beach",
		// Alone in a folder of its own, with its side files.
		"Birthday 2019/clip.mp4": "Birthday 2019",
		// Several in a folder are titles of their own.
		"Trip/day 1.mov":        "day 1",
		"Trip/day 2.mov":        "day 2",
		"Trip/Evening/fire.mkv": "Evening",
	} {
		if got := homeName(rel, own); got != want {
			t.Errorf("%s: %q, want %q", rel, got, want)
		}
	}
}

func TestHomeVideoDates(t *testing.T) {
	modified := time.Date(2024, time.March, 3, 9, 0, 0, 0, time.UTC)
	created := time.Date(2019, time.June, 2, 15, 30, 0, 0, time.UTC)
	premiered := time.Date(2019, time.June, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		nfo     nfoInfo
		created *time.Time
		want    time.Time
		by      string
	}{
		{"the NFO file's first", nfoInfo{released: &premiered, year: 2018}, &created, premiered, datedByNFO},
		{"then the recording's", nfoInfo{}, &created, created, datedByCreated},
		{"then the file's", nfoInfo{}, nil, modified, datedByFile},
		{"a year alone keeps a date of that year", nfoInfo{year: 2019}, &created, created, datedByCreated},
		{"a year alone, of another year", nfoInfo{year: 2010}, &created, time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC), datedByNFO},
	} {
		if got, by := homeDate(tc.nfo, tc.created, modified); !got.Equal(tc.want) || by != tc.by {
			t.Errorf("%s: %v %s", tc.name, got, by)
		}
	}
}

func TestReadVideoProbe(t *testing.T) {
	for _, tc := range []struct {
		name, json string
		created    string
		duration   float64
		hdr        bool
	}{
		{"the container's", `{"format": {"duration": "12.5", "tags": {"creation_time": "2015-07-04T10:00:00.000000Z"}},
			"streams": [{"codec_type": "video", "color_transfer": "bt709"}]}`, "2015-07-04T10:00:00Z", 12.5, false},
		{"Apple's first, with its time zone", `{"format": {"tags": {"creation_time": "2015-07-04T10:00:00.000000Z",
			"com.apple.quicktime.creationdate": "2015-07-04T12:00:05+0200"}}, "streams": [{"codec_type": "video", "color_transfer": "arib-std-b67"}]}`,
			"2015-07-04T10:00:05Z", 0, true},
		{"the video stream's", `{"format": {}, "streams": [{"codec_type": "audio", "tags": {"creation_time": "2001-01-01T00:00:00Z"}},
			{"codec_type": "video", "tags": {"CREATION_TIME": "2016-05-06 07:08:09"}}]}`, "2016-05-06T07:08:09Z", 0, false},
		{"a clock never set", `{"format": {"tags": {"creation_time": "1970-01-01T00:00:00.000000Z"}}, "streams": []}`, "", 0, false},
		{"none", `{"format": {"duration": "N/A"}, "streams": [{"codec_type": "video", "color_transfer": "smpte2084"}]}`, "", 0, true},
	} {
		info, err := readVideoProbe([]byte(tc.json))
		created := ""
		if info.created != nil {
			created = info.created.UTC().Format(time.RFC3339)
		}
		if err != nil || created != tc.created || info.duration != tc.duration || info.hdr != tc.hdr {
			t.Errorf("%s: %+v %s %v", tc.name, info, created, err)
		}
	}
}

// clip makes a short video at path with ffmpeg, with the tags given.
func clip(t *testing.T, ffmpeg, path string, tags map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"-v", "error", "-y", "-f", "lavfi", "-i", "testsrc=duration=2:size=160x120:rate=10", "-c:v", "mpeg4"}
	for key, value := range tags {
		args = append(args, "-metadata", key+"="+value)
	}
	if out, err := exec.Command(ffmpeg, append(args, path)...).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

// homeTitles lists a home videos folder's catalog, by name.
func (e env) homeTitles(id accounts.ID) map[string]stremio.Meta {
	e.t.Helper()
	metas, err := e.service.Catalog(e.t.Context(), id, library.VideoType, catalogID, 0, "")
	if err != nil {
		e.t.Fatal(err)
	}
	titles := map[string]stremio.Meta{}
	for _, meta := range metas {
		titles[meta.Name] = meta
	}
	return titles
}

// A home videos folder lists each video as a title of its own, named,
// dated and described by its NFO file, its recording and its file, with
// the poster beside it or a frame of it; nothing is asked of the metadata
// addons, nothing is unmatched, and identifiers stay through scans until a
// file is renamed.
func TestHomeVideosFolders(t *testing.T) {
	ffmpeg, ffprobe := musicTools(t)
	e := newEnv(t)
	e.service.UseTools(ffprobe, ffmpeg)
	dir := t.TempDir()
	mtime := time.Date(2023, time.September, 9, 9, 0, 0, 0, time.UTC)
	clip(t, ffmpeg, filepath.Join(dir, "Birthday/cake.mp4"), nil)
	writeText(t, filepath.Join(dir, "Birthday/cake.nfo"), `<?xml version="1.0"?><movie><title>Lucy turns five</title>`+
		`<plot>Candles, and a cake.</plot><premiered>2021-04-10</premiered></movie>`, mtime)
	clip(t, ffmpeg, filepath.Join(dir, "Beach.mp4"), map[string]string{"creation_time": "2015-07-04T10:00:00Z"})
	clip(t, ffmpeg, filepath.Join(dir, "Trip/day 1.mp4"), nil)
	clip(t, ffmpeg, filepath.Join(dir, "Trip/day 2.mp4"), nil)
	image(t, ffmpeg, filepath.Join(dir, "Trip/day 2-poster.jpg"), "blue")
	for _, rel := range []string{"Birthday/cake.mp4", "Trip/day 1.mp4", "Trip/day 2.mp4"} {
		if err := os.Chtimes(filepath.Join(dir, rel), mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	addon, err := e.service.Add(t.Context(), NewFolder{Name: "Family", Path: dir, Kind: KindHomeVideos})
	if err != nil {
		t.Fatal(err)
	}
	e.idle(addon.ID)

	titles := e.homeTitles(addon.ID)
	want := map[string]struct {
		path, released, plot string
	}{
		"Lucy turns five": {"Birthday/cake.mp4", "2021-04-10T00:00:00Z", "Candles, and a cake."},
		"Beach":           {"Beach.mp4", "2015-07-04T10:00:00Z", ""},
		"day 1":           {"Trip/day 1.mp4", "2023-09-09T09:00:00Z", ""},
		"day 2":           {"Trip/day 2.mp4", "2023-09-09T09:00:00Z", ""},
	}
	if len(titles) != len(want) {
		t.Fatalf("titles: %+v", titles)
	}
	for name, w := range want {
		meta := titles[name]
		if meta.ID != videoID(addon.ID, w.path) || meta.Type != library.VideoType || meta.Released != w.released || meta.Description != w.plot {
			t.Errorf("%s: %+v", name, meta)
		}
	}
	if e.titles.searches.Load() != 0 {
		t.Errorf("the metadata addons were searched %d times", e.titles.searches.Load())
	}
	folder, err := e.service.Folder(t.Context(), addon.ID)
	if err != nil || folder.Files != 4 || folder.Matched != 4 || folder.Unmatched != 0 || folder.Dated != (Dates{NFO: 1, Created: 1, File: 2}) {
		t.Fatalf("figures: %+v %v", folder, err)
	}
	if unmatched, total, err := e.service.Unmatched(t.Context(), addon.ID); err != nil || total != 0 || len(unmatched) != 0 {
		t.Errorf("unmatched: %v %d %v", unmatched, total, err)
	}
	if err := e.service.Link(t.Context(), addon.ID, "Beach.mp4", "tt0063350"); err == nil {
		t.Error("a home video was linked")
	}

	// A search finds a video by the name it shows; its description and
	// streams are the folder's.
	found, err := e.service.Catalog(t.Context(), addon.ID, library.VideoType, catalogID, 0, "lucy")
	if err != nil || len(found) != 1 || found[0].Name != "Lucy turns five" {
		t.Errorf("search: %+v %v", found, err)
	}
	cake := titles["Lucy turns five"].ID
	meta, err := e.service.Meta(t.Context(), addon.ID, cake)
	if err != nil || meta.Name != "Lucy turns five" || meta.Description != "Candles, and a cake." || meta.Year != "2021" {
		t.Errorf("meta: %+v %v", meta, err)
	}
	streams, err := e.service.Streams(t.Context(), addon.ID, cake)
	if err != nil || len(streams) != 1 || streams[0].BehaviorHints.Filename != "cake.mp4" {
		t.Errorf("streams: %+v %v", streams, err)
	}

	// The poster beside a video is its artwork, a frame of it otherwise.
	artwork := e.service.Artwork()
	poster := artwork[library.TitleItemID(library.KindVideo, titles["day 2"].ID)].Primary
	frame := artwork[library.TitleItemID(library.KindVideo, titles["day 1"].ID)].Primary
	for _, url := range []string{poster, frame, artwork[library.TitleItemID(library.KindVideo, titles["Beach"].ID)].Primary,
		artwork[library.TitleItemID(library.KindVideo, cake)].Primary} {
		if url == "" {
			t.Fatalf("artwork: %v", artwork)
		}
	}
	read := func(url string) []byte {
		t.Helper()
		key, _ := library.LocalArtworkKey(url)
		file, _, err := e.service.Open(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	posterFile, err := os.ReadFile(filepath.Join(dir, "Trip/day 2-poster.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read(poster), posterFile) {
		t.Error("the poster is not the file beside the video")
	}
	if data := read(frame); !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		t.Errorf("the frame is no JPEG: %q", data[:min(len(data), 8)])
	}

	// A scan again keeps every identifier and frame; an NFO file changed
	// renames its video, which is not probed again.
	writeText(t, filepath.Join(dir, "Birthday/cake.nfo"), `<movie><title>Lucy is five</title><premiered>2021-04-10</premiered></movie>`,
		mtime.Add(time.Hour))
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	again := e.homeTitles(addon.ID)
	if again["Lucy is five"].ID != cake || again["Beach"].ID != titles["Beach"].ID || again["Lucy is five"].Description != "" {
		t.Errorf("after a scan: %+v", again)
	}
	if e.service.Artwork()[library.TitleItemID(library.KindVideo, titles["day 1"].ID)].Primary != frame {
		t.Error("the frame was made again")
	}

	// Renamed, a video is another title.
	if err := os.Rename(filepath.Join(dir, "Beach.mp4"), filepath.Join(dir, "Beach day.mp4")); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	renamed := e.homeTitles(addon.ID)
	if _, ok := renamed["Beach"]; ok || renamed["Beach day"].ID != videoID(addon.ID, "Beach day.mp4") ||
		renamed["Beach day"].Released != "2015-07-04T10:00:00Z" {
		t.Errorf("renamed: %+v", renamed)
	}
}
