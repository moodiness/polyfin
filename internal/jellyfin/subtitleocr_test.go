package jellyfin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/subtitles"
	"github.com/moodiness/polyfin/internal/testsubtitles"
)

// fixedReader reads every track as the same two cues, in English.
type fixedReader struct{}

func (fixedReader) Languages(context.Context) ([]string, error) { return []string{"eng"}, nil }

func (fixedReader) Read(_ context.Context, pictures []ocr.Picture, _ string) ([]subtitles.Cue, error) {
	cues := make([]subtitles.Cue, len(pictures))
	for i, p := range pictures {
		cues[i] = subtitles.Cue{Start: p.Start, End: p.End, Lines: []string{"Read from picture " + strconv.Itoa(i+1)}}
	}
	return cues, nil
}

// localPictureCopies is a test server reading picture subtitles with
// fixedReader, the setting on, with a local folder holding "Movie (2008)",
// linked to the movie, beside a PGS file, "Movie (2008).en.sdh.sup". It
// returns the local version's media source, and whether the video is
// analyzed as it is, by the ffprobe beside POLYFIN_TEST_FFMPEG, which lets
// it be converted; else as a fixture tells.
func localPictureCopies(t *testing.T) (playbackSetup, MediaSourceInfo, bool) {
	t.Helper()
	var folders *localfiles.Service
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		o.Library.UseLocal(folders)
		folders.UseFiles(o.Playback.LocalFiles(folders.Open), o.Playback.LocalInputs(folders))
		o.Playback.UseTesseract("ffmpeg", "", fixedReader{})
		settings := o.Accounts.Settings()
		settings.SubtitleOCR = "played"
		if _, err := o.Accounts.UpdateSettings(t.Context(), settings); err != nil {
			t.Fatal(err)
		}
	})
	p := playingOn(t, s)
	video, err := os.ReadFile(filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	captions := testsubtitles.Captions{CanvasWidth: 1920, CanvasHeight: 1080, Size: 56, Shown: []testsubtitles.Caption{
		{Lines: []string{"The lighthouse keeper waited"}, Start: time.Second, End: 3 * time.Second},
		{Lines: []string{"Seven boats came home"}, Start: 5 * time.Second, End: 7 * time.Second},
	}}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Movie (2008)"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"Movie (2008).mkv": video, "Movie (2008).en.sdh.sup": captions.PGS()} {
		if err := os.WriteFile(filepath.Join(dir, "Movie (2008)", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	addon, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), addon.ID); err != nil {
		t.Fatal(err)
	}
	if err := folders.Link(t.Context(), addon.ID, "Movie (2008)/Movie (2008).mkv", "tt1000"); err != nil {
		t.Fatal(err)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
	if movie.MediaSources == nil || len(*movie.MediaSources) != 3 {
		t.Fatalf("media sources: %+v", movie.MediaSources)
	}
	local := (*movie.MediaSources)[2]
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		p.analyzed(t, localVersion(t, p, local), "h264-aac-mp4")
		return p, local, false
	}
	analysis, err := media.Prober{Path: filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), Timeout: time.Minute}.Probe(t.Context(),
		filepath.Join("..", "container", "testdata", "forced.mkv"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(analysis)
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", localVersion(t, p, local).ID, encoded); err != nil {
		t.Fatal(err)
	}
	return p, local, true
}

// textCopies are the text copies a media source lists.
func textCopies(source MediaSourceInfo) []playback.MediaStream {
	var copies []playback.MediaStream
	for _, stream := range source.MediaStreams {
		if strings.HasSuffix(stream.Title, " (OCR)") {
			copies = append(copies, stream)
		}
	}
	return copies
}

// A picture subtitle chosen on an app that cannot draw it is read into
// text in the background. From the next PlaybackInfo, its copy is listed
// after the version's tracks, a SubRip file with the original's language
// and flags, which reaches Chrome as an External WebVTT file with the
// video copied. The picture file is still listed, and burned in or left
// out when asked. With no track asked, the copy stands for its original
// on an app that cannot draw it, and the original stays on one that can.
func TestPictureSubtitlesHaveTextCopies(t *testing.T) {
	t.Parallel()
	p, local, converts := localPictureCopies(t)
	chrome := p.profile(t, "jellyfin-web-chrome")
	// The addon's French file is 0, the PGS file 1.
	source := pictureInfo(t, p, local, chrome, 1)
	if copies := textCopies(source); len(copies) != 0 {
		t.Fatalf("a copy before it is read: %+v", copies)
	}
	if source.MediaStreams[1].DeliveryMethod == "External" {
		t.Errorf("the PGS file goes out to Chrome as %s", source.MediaStreams[1].DeliveryMethod)
	}
	deadline := time.Now().Add(time.Minute)
	for len(textCopies(source)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("no copy was read")
		}
		time.Sleep(50 * time.Millisecond)
		source = pictureInfo(t, p, local, chrome, 1)
	}
	copied := textCopies(source)[0]
	last := source.MediaStreams[len(source.MediaStreams)-1]
	tracks := slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Title == copied.Title })
	if last.Index != copied.Index || tracks != len(source.MediaStreams)-1 || copied.Index <= source.MediaStreams[tracks-1].Index {
		t.Errorf("the copy is not listed after the tracks: %+v", source.MediaStreams)
	}
	if copied.Codec != "subrip" || copied.Language != "eng" || copied.Title != "English (OCR)" || copied.IsForced || !copied.IsHearingImpaired ||
		!copied.IsExternal || !copied.IsTextSubtitleStream || !copied.SupportsExternalStream ||
		copied.DisplayTitle != "English (OCR) - Hearing Impaired - SUBRIP - External" {
		t.Errorf("the copy: %+v", copied)
	}
	// The picture file is still listed, and still burned into converted
	// video when asked, or left out when the video cannot be converted.
	original := source.MediaStreams[1]
	burned := original.DeliveryMethod == "Encode" && strings.Contains(source.TranscodingUrl, "SubtitleMethod=Encode") &&
		strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false")
	if original.Codec != "PGSSUB" || converts && !burned || !converts && original.DeliveryMethod != "Drop" {
		t.Errorf("the original: %s %s", original.DeliveryMethod, source.TranscodingUrl)
	}
	// Asked, the copy goes out as WebVTT, the video copied.
	source = pictureInfo(t, p, local, chrome, copied.Index)
	sent := source.MediaStreams[slices.IndexFunc(source.MediaStreams, func(s playback.MediaStream) bool { return s.Index == copied.Index })]
	if sent.DeliveryMethod != "External" || !strings.Contains(sent.DeliveryUrl, "/Stream.vtt") ||
		strings.Contains(source.TranscodingUrl, "allowVideoStreamCopy=false") {
		t.Fatalf("the copy asked: %s %s, %s", sent.DeliveryMethod, sent.DeliveryUrl, source.TranscodingUrl)
	}
	response, body := fetchURL(t, p.url+sent.DeliveryUrl, nil)
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(body, "WEBVTT") || !strings.Contains(body, "00:00:01.000 --> 00:00:03.000") ||
		!strings.Contains(body, "Read from picture 2") {
		t.Errorf("the copy's file: %d %q", response.StatusCode, body)
	}
	// With no track asked, the user's English is shown: the copy on
	// Chrome, the PGS file on an app that draws PGS.
	if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"SubtitleLanguagePreference": "eng", "SubtitleMode": "Always"}); status != http.StatusNoContent {
		t.Fatalf("configuration: %d", status)
	}
	if source := pictureInfo(t, p, local, chrome, -1); source.DefaultSubtitleStreamIndex == nil || *source.DefaultSubtitleStreamIndex != copied.Index {
		t.Errorf("shown on Chrome: %v", deref(source.DefaultSubtitleStreamIndex))
	}
	if source := pictureInfo(t, p, local, pictureProfile(t, p), -1); source.DefaultSubtitleStreamIndex == nil ||
		*source.DefaultSubtitleStreamIndex != 1 || source.MediaStreams[1].DeliveryMethod != "External" {
		t.Errorf("shown on an app that draws PGS: %v", deref(source.DefaultSubtitleStreamIndex))
	}
	// With the setting off, no copy is listed.
	settings := p.store.Settings()
	settings.SubtitleOCR = "off"
	if _, err := p.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if copies := textCopies(pictureInfo(t, p, local, chrome, 1)); len(copies) != 0 {
		t.Errorf("copies with the setting off: %+v", copies)
	}
}
