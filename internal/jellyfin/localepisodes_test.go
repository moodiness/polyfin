package jellyfin

import (
	"bytes"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/process"
)

// A show matched to a series whose metadata addon lists only some of its
// episodes lists every episode its files hold, as Jellyfin makes an
// episode of every file: those the addon does not list are named after
// their files and play them, and the folder counts them. An episode
// without an image shows a frame of its file, as Jellyfin's Screen Grabber
// makes one; one with the addon's still keeps it.
func TestLocalEpisodesTheAddonDoesNotList(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	// The addon lists S01E01, S01E02 and S02E01 of its show, a still for
	// the first only; the folder also holds S01E03.
	p.write(t, "Shows/Harbor Show (2011)/Season 01/Harbor Show S01E03.mp4", p.video)
	p.scan(t)
	if err := p.folders.Link(t.Context(), p.shows, "Harbor Show (2011)", "tt0100"); err != nil {
		t.Fatal(err)
	}
	var page QueryResult
	p.get(t, "/Shows/"+p.series+"/Episodes?UserId="+p.user, p.token, &page)
	episodes := map[[2]int]BaseItemDto{}
	for _, episode := range page.Items {
		episodes[[2]int{*episode.ParentIndexNumber, *episode.IndexNumber}] = episode
	}
	third, ok := episodes[[2]int{1, 3}]
	if len(episodes) != 4 || !ok || third.Name != "Harbor Show S01E03" {
		t.Fatalf("the show's episodes: %v", itemNames(page.Items))
	}
	answer := p.playbackInfo(t, "tv", p.token, third.Id, nil)
	if len(answer.MediaSources) != 1 || !strings.HasPrefix(answer.MediaSources[0].Name, "Local shows") {
		t.Errorf("the versions of the episode the addon does not list: %+v", answer.MediaSources)
	}
	folder, err := p.folders.Folder(t.Context(), p.shows)
	if err != nil || folder.Unlisted != 1 {
		t.Errorf("the episodes the addon does not list: %d %v", folder.Unlisted, err)
	}

	// The pilot keeps the addon's still; the second episode, without one,
	// shows a frame of its file.
	pilot, err := p.library.Artwork(t.Context(), mustID(t, episodes[[2]int{1, 1}].Id), "Primary")
	if err != nil || !strings.HasSuffix(pilot.URL, "/still.jpg") {
		t.Errorf("the pilot's image: %q %v", pilot.URL, err)
	}
	second := episodes[[2]int{1, 2}]
	if second.ImageTags["Primary"] == "" {
		t.Errorf("the second episode has no image: %v", second.ImageTags)
	}
	frame, err := p.library.Artwork(t.Context(), mustID(t, second.Id), "Primary")
	if key, local := library.LocalArtworkKey(frame.URL); err != nil || !local || !strings.HasSuffix(key, ".frame") {
		t.Fatalf("the second episode's image: %q %v", frame.URL, err)
	}
	response, body := p.fetch(t, p.url+"/Items/"+second.Id+"/Images/Primary")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("the second episode's frame: %d", response.StatusCode)
	}
	if _, err := jpeg.Decode(bytes.NewReader(body)); err != nil {
		t.Errorf("the second episode's frame is no JPEG image: %v", err)
	}
}

// heldFFmpeg writes an FFmpeg that records its runs, then waits for the
// test to release it and runs the real one, or, when failing, fails.
func heldFFmpeg(t *testing.T, failing bool) (path string, runs func() int, release func()) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "ffmpeg")
	end := "exec '" + os.Getenv("POLYFIN_TEST_FFMPEG") + "' \"$@\"\n"
	if failing {
		end = "exit 1\n"
	}
	script := "#!/bin/sh\necho run >> \"$0.runs\"\nwhile [ ! -e \"$0.released\" ]; do sleep 0.01; done\n" + end
	if err := process.WriteExecutable(path, strings.NewReader(script), 0o700); err != nil {
		t.Fatal(err)
	}
	runs = func() int {
		data, _ := os.ReadFile(path + ".runs")
		return bytes.Count(data, []byte("\n"))
	}
	release = func() { _ = os.WriteFile(path+".released", nil, 0o600) }
	// A run still waiting ends with the test.
	t.Cleanup(release)
	return path, runs, release
}

// frameOf returns the episode of the addon's show without a still, whose
// image is a frame of its file.
func (p plainSetup) frameOf(t *testing.T) string {
	t.Helper()
	p.scan(t)
	if err := p.folders.Link(t.Context(), p.shows, "Harbor Show (2011)", "tt0100"); err != nil {
		t.Fatal(err)
	}
	var page QueryResult
	p.get(t, "/Shows/"+p.series+"/Episodes?UserId="+p.user, p.token, &page)
	for _, episode := range page.Items {
		if *episode.ParentIndexNumber == 1 && *episode.IndexNumber == 2 {
			return p.url + "/Items/" + episode.Id + "/Images/Primary"
		}
	}
	t.Fatalf("the show's episodes: %v", itemNames(page.Items))
	return ""
}

// An episode's frame still being made is waited for, by the request that
// asked for it and by those asking meanwhile, which share it: none is
// answered that the image is missing.
func TestEpisodeFramesBeingMadeAreWaitedFor(t *testing.T) {
	t.Parallel()
	ffmpeg, runs, release := heldFFmpeg(t, false)
	p := newPlainSetup(t, ffmpeg)
	frame := p.frameOf(t)
	type answer struct {
		status int
		body   []byte
		err    error
	}
	ask := func() <-chan answer {
		answered := make(chan answer, 1)
		go func() {
			response, err := testClient.Get(frame)
			if err != nil {
				answered <- answer{err: err}
				return
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			answered <- answer{response.StatusCode, body, err}
		}()
		return answered
	}
	first := ask()
	eventually(t, "FFmpeg to make the frame", func() bool { return runs() == 1 })
	second := ask()
	select {
	case got := <-first:
		t.Fatalf("answered before the frame was made: %d %v", got.status, got.err)
	default:
	}
	release()
	for _, answered := range []<-chan answer{first, second} {
		got := <-answered
		if got.err != nil || got.status != http.StatusOK {
			t.Fatalf("the frame: %d %v", got.status, got.err)
		}
		if _, err := jpeg.Decode(bytes.NewReader(got.body)); err != nil {
			t.Errorf("the frame is no JPEG image: %v", err)
		}
	}
	if runs() != 1 {
		t.Errorf("the frame was made %d times", runs())
	}
}

// A frame FFmpeg fails to make is missing, and is not tried again at each
// request for it.
func TestEpisodeFramesFailedAreNotTriedAtEachRequest(t *testing.T) {
	t.Parallel()
	ffmpeg, runs, release := heldFFmpeg(t, true)
	release()
	p := newPlainSetup(t, ffmpeg)
	frame := p.frameOf(t)
	if response, _ := p.fetch(t, frame); response.StatusCode != http.StatusNotFound {
		t.Errorf("the frame FFmpeg failed: %d", response.StatusCode)
	}
	tried := runs()
	if response, _ := p.fetch(t, frame); response.StatusCode != http.StatusNotFound || runs() != tried || tried == 0 {
		t.Errorf("asked again: %d, FFmpeg ran %d times, then %d", response.StatusCode, tried, runs())
	}
}

// A show's file named by an absolute number is a version of the catalog's
// episode of that place, specials left out, in a season folder or at the
// top of its show's folder; one whose number the catalog does not reach is
// an episode of its own, named after it.
func TestLocalEpisodesNamedByAbsoluteNumber(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	// The addon lists S01E01, S01E02 and S02E01: the third is S02E01.
	p.write(t, "Shows/Harbor Show (2011)/Harbor Show - 3.mp4", p.video)
	p.write(t, "Shows/Harbor Show (2011)/Season 02/Harbor Show 2.mp4", p.video)
	p.write(t, "Shows/Harbor Show (2011)/Harbor Show - 7.mp4", p.video)
	p.scan(t)
	if err := p.folders.Link(t.Context(), p.shows, "Harbor Show (2011)", "tt0100"); err != nil {
		t.Fatal(err)
	}
	var page QueryResult
	p.get(t, "/Shows/"+p.series+"/Episodes?UserId="+p.user, p.token, &page)
	episodes := map[[2]int]BaseItemDto{}
	for _, episode := range page.Items {
		episodes[[2]int{*episode.ParentIndexNumber, *episode.IndexNumber}] = episode
	}
	if len(episodes) != 4 || episodes[[2]int{2, 1}].Name != "Return" || episodes[[2]int{1, 7}].Name != "Harbor Show - 7" {
		t.Fatalf("the show's episodes: %v", itemNames(page.Items))
	}
	for number, want := range map[[2]int]int{{2, 1}: 2, {1, 2}: 2, {1, 7}: 1} {
		answer := p.playbackInfo(t, "tv", p.token, episodes[number].Id, nil)
		if len(answer.MediaSources) != want {
			t.Errorf("the versions of S%02dE%02d: %+v", number[0], number[1], answer.MediaSources)
		}
		for _, source := range answer.MediaSources {
			if !strings.HasPrefix(source.Name, "Local shows") {
				t.Errorf("a version of S%02dE%02d: %+v", number[0], number[1], source)
			}
		}
	}
	folder, err := p.folders.Folder(t.Context(), p.shows)
	if err != nil || folder.Unlisted != 1 {
		t.Errorf("the episodes the addon does not list: %d %v", folder.Unlisted, err)
	}
}

// A show whose files number on across seasons plays them as the catalog's
// episodes once an administrator says they are numbered by absolute
// place: the file S01E03, after the catalog's two episodes of season 1, is
// a version of S02E01, listed once and offered though its name tells
// another episode, beside the file named by its absolute number 3; the
// played mark of the episode it was listed as follows it.
func TestLocalEpisodesNumberedAsTheAdministratorSays(t *testing.T) {
	t.Parallel()
	p := newPlainSetup(t)
	if err := os.Remove(filepath.Join(p.dir, "Shows", "Harbor Show (2011)", "Season 02", "Harbor Show S02E01.mp4")); err != nil {
		t.Fatal(err)
	}
	p.write(t, "Shows/Harbor Show (2011)/Season 01/Harbor Show S01E03.mp4", p.video)
	p.write(t, "Shows/Harbor Show (2011)/Harbor Show - 3.mp4", p.video)
	p.scan(t)
	if err := p.folders.Link(t.Context(), p.shows, "Harbor Show (2011)", "tt0100"); err != nil {
		t.Fatal(err)
	}
	episodes := func() map[[2]int]BaseItemDto {
		t.Helper()
		var page QueryResult
		p.get(t, "/Shows/"+p.series+"/Episodes?UserId="+p.user, p.token, &page)
		episodes := map[[2]int]BaseItemDto{}
		for _, episode := range page.Items {
			episodes[[2]int{*episode.ParentIndexNumber, *episode.IndexNumber}] = episode
		}
		return episodes
	}
	before := episodes()
	if len(before) != 4 || before[[2]int{1, 3}].Name != "Harbor Show S01E03" {
		t.Fatalf("the show's episodes as the catalog: %v", before)
	}
	if status, data := p.call(http.MethodPost, "/UserPlayedItems/"+before[[2]int{1, 3}].Id, app("tv", p.token), nil); status != http.StatusOK {
		t.Fatalf("marking the file's episode played: %d %s", status, data)
	}

	if err := p.folders.SetNumbering(t.Context(), p.shows, "tt0100", localfiles.ShowNumbering{Numbering: localfiles.NumberedAbsolute}); err != nil {
		t.Fatal(err)
	}
	after := episodes()
	if len(after) != 3 || after[[2]int{2, 1}].Name != "Return" {
		t.Fatalf("the show's episodes, absolute: %v", after)
	}
	answer := p.playbackInfo(t, "tv", p.token, after[[2]int{2, 1}].Id, nil)
	if len(answer.MediaSources) != 2 {
		t.Errorf("the versions of S02E01: %+v", answer.MediaSources)
	}
	for _, source := range answer.MediaSources {
		if !strings.HasPrefix(source.Name, "Local shows") {
			t.Errorf("a version of S02E01: %+v", source)
		}
	}
	if !after[[2]int{2, 1}].UserData.Played {
		t.Errorf("the played mark, with its file: %+v", after[[2]int{2, 1}].UserData)
	}
}
