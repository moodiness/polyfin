package jellyfin

import (
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A movie of a local folder that no addon describes shows how long its
// file plays, as the folder's scan probed it, with the time it would end.
func TestALocalMovieShowsItsProbedLength(t *testing.T) {
	t.Parallel()
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	ffprobe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	dir := t.TempDir()
	target := filepath.Join(dir, "Quiet Valley (2019)", "Quiet Valley (2019).mkv")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "sine=duration=7",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=1:duration=7", "-map", "1:v", "-map", "0:a", "-c:v", "libx264",
		"-preset", "veryfast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", target).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	var folders *localfiles.Service
	s := newProbingServer(t, 10, ffprobe, func(o *Options, pool *pgxpool.Pool) {
		folders = localfiles.New(pool, addons.New(pool, stremio.NewClient("test")), o.Library, slog.New(slog.NewTextHandler(io.Discard, nil)),
			o.Accounts.Settings, nil)
		t.Cleanup(folders.Close)
		folders.UseTools(ffprobe, ffmpeg)
		o.Library.UseLocal(folders)
	})
	p := playingOn(t, s)
	folder, err := folders.Add(t.Context(), localfiles.NewFolder{Name: "Shelf", Path: dir, Kind: localfiles.KindMovies})
	if err != nil {
		t.Fatal(err)
	}
	if err := folders.Scan(t.Context(), folder.ID); err != nil {
		t.Fatal(err)
	}
	user := p.user.ID.String()
	var views QueryResult
	p.get(t, "/Users/"+user+"/Views", p.token, &views)
	shelf := ""
	for _, view := range views.Items {
		if view.Name == "Shelf" {
			shelf = view.Id
		}
	}
	var listed QueryResult
	p.get(t, "/Users/"+user+"/Items?ParentId="+shelf, p.token, &listed)
	if len(listed.Items) != 1 {
		t.Fatalf("the folder's titles: %+v", listed.Items)
	}
	var movie BaseItemDto
	p.get(t, "/Users/"+user+"/Items/"+listed.Items[0].Id, p.token, &movie)
	if movie.Type != "Movie" || movie.RunTimeTicks == nil || *movie.RunTimeTicks < 6e7 || *movie.RunTimeTicks > 8e7 {
		t.Errorf("the local movie: %s %s, length %v ticks, want about 7 s", movie.Name, movie.Type, movie.RunTimeTicks)
	}
}
