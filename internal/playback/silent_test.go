package playback

import (
	"log/slog"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// An analysis that finds video and no audio is said at Info once per
// version, with its track count; one with audio, or of a file with only a
// cover picture as video, is not.
func TestAnAnalysisWithoutAudioIsLoggedOnce(t *testing.T) {
	t.Parallel()
	s := newService(t, nil, "", nil)
	log := &records{}
	s.logger = slog.New(log)
	silent := library.Version{ID: accounts.ID{8, 5, 6}, Name: "The.Movie.1080p.mkv"}
	videoOnly := media.Analysis{Streams: []media.Stream{{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "subtitle", Codec: "subrip"}}}
	for range 2 {
		s.keepAnalysis(t.Context(), silent, videoOnly, time.Now())
	}
	s.keepAnalysis(t.Context(), library.Version{ID: accounts.ID{8, 5, 7}}, media.Analysis{Streams: []media.Stream{
		{Index: 0, Type: "video", Codec: "h264"}, {Index: 1, Type: "audio", Codec: "ac3"}}}, time.Now())
	s.keepAnalysis(t.Context(), library.Version{ID: accounts.ID{8, 5, 8}}, media.Analysis{Streams: []media.Stream{
		{Index: 0, Type: "video", Codec: "mjpeg", AttachedPicture: true}}}, time.Now())

	log.mu.Lock()
	defer log.mu.Unlock()
	said := 0
	for _, record := range log.kept {
		if record.Message != "A version has no audio track" {
			continue
		}
		said++
		if record.Level != slog.LevelInfo {
			t.Errorf("level %s", record.Level)
		}
		record.Attrs(func(a slog.Attr) bool {
			if a.Key == "version" && a.Value.String() != silent.Name || a.Key == "tracks" && a.Value.Int64() != 2 {
				t.Errorf("%s = %s", a.Key, a.Value)
			}
			return true
		})
	}
	if said != 1 {
		t.Errorf("said %d times", said)
	}
}
