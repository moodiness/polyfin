package jellyfin

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/remuxdb"
)

// remuxDBAnswer is what a fake RemuxDB knows of the streaming addon's
// movie: the files of both its versions.
const remuxDBAnswer = `[
  {"container": "matroska,webm", "duration": 7210.5, "size": 80000000000,
   "sources": [{"kind": "torrent", "filename": "Movie.2160p/Movie.2160p.mkv", "torrent_info_hash": "aa", "torrent_file_idx": 0}],
   "tracks": [
     {"kind": "video", "idx": 0, "codec": "hevc", "width": 3840, "height": 1600, "color_transfer": "smpte2084", "is_default": true},
     {"kind": "audio", "idx": 1, "codec": "eac3", "language": "eng", "channels": 6, "is_default": true},
     {"kind": "audio", "idx": 2, "codec": "ac3", "language": "fre", "channels": 6},
     {"kind": "subtitle", "idx": 3, "codec": "subrip", "language": "fre", "is_forced": true}]},
  {"container": "matroska,webm", "duration": 7200, "size": 3000000000,
   "sources": [{"kind": "nzb", "filename": "Movie.1080p.mp4", "indexer": "indexer", "indexer_guid": "1e8b"}],
   "tracks": [{"kind": "video", "idx": 0, "codec": "hevc", "width": 1920, "height": 1036}]}
]`

func TestItemDetailsDescribeVersionsFromRemuxDB(t *testing.T) {
	var asked atomic.Int32
	remux := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if r.URL.Path != "/api/media/tt1000/versions" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, remuxDBAnswer)
	}))
	t.Cleanup(remux.Close)
	s := newProbingServer(t, 10, "ffprobe-not-installed", func(o *Options, _ *pgxpool.Pool) {
		o.RemuxDB = remuxdb.New(testServerID, "test", slog.New(slog.DiscardHandler), o.Accounts.Settings)
	})
	settings := s.store.Settings()
	settings.RemuxDB, settings.RemuxDBURL = true, remux.URL
	if _, err := s.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	p := playingOn(t, s)

	for range 2 {
		var movie BaseItemDto
		p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
		if movie.MediaSources == nil || len(*movie.MediaSources) != 2 {
			t.Fatalf("media sources: %+v", movie.MediaSources)
		}
		described, analyzed := (*movie.MediaSources)[0], (*movie.MediaSources)[1]
		// The version never analyzed lists the tracks RemuxDB found in its
		// file, numbered after the addon's subtitle file as its analysis
		// will number them.
		var tracks []string
		for _, stream := range described.MediaStreams {
			tracks = append(tracks, stream.Type+" "+stream.Codec)
		}
		want := []string{"Subtitle subrip", "Video hevc", "Audio eac3", "Audio ac3", "Subtitle subrip"}
		if !slices.Equal(tracks, want) || described.MediaStreams[1].Index != 1 || described.MediaStreams[4].Index != 4 ||
			!described.MediaStreams[4].IsForced || described.MediaStreams[1].VideoRangeType != "HDR10" {
			t.Errorf("described tracks %v, want %v: %+v", tracks, want, described.MediaStreams)
		}
		if described.Container != "mkv" || described.DefaultAudioStreamIndex == nil || *described.DefaultAudioStreamIndex != 2 ||
			described.RunTimeTicks == nil || *described.RunTimeTicks != 72_105_000_000 {
			t.Errorf("described version: container %s, audio %v, runtime %v", described.Container, described.DefaultAudioStreamIndex, described.RunTimeTicks)
		}
		// The item describes the version it opened as.
		if movie.Height == nil || *movie.Height != 1600 || len(*movie.MediaStreams) != 5 {
			t.Errorf("item: height %v, streams %+v", movie.Height, movie.MediaStreams)
		}
		// An analysis wins over RemuxDB's description.
		if analyzed.Container != "mp4" || videoHeightOf(analyzed) == 1036 {
			t.Errorf("analyzed version: %s, %d high", analyzed.Container, videoHeightOf(analyzed))
		}
	}
	// The second details are described from the first's answer.
	if n := asked.Load(); n != 1 {
		t.Errorf("RemuxDB asked %d times", n)
	}
}
