package jellyfin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// A movie waiting for its digital release answers PlaybackInfo with
// Jellyfin's NotAllowed and no source, until it is out at home.
func TestAMovieWaitingForItsDigitalReleaseDoesNotPlay(t *testing.T) {
	t.Parallel()
	// The streaming addon's Movie came out in cinemas on 10 April 2008; TMDB
	// tells it is out at home on 1 August.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/3/find/tt1000":
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{map[string]any{"id": 1000}}, "tv_results": []any{}})
		case "/3/movie/1000/release_dates":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{"iso_3166_1": "US", "release_dates": []any{
				map[string]any{"release_date": "2008-04-10T00:00:00.000Z", "type": 3},
				map[string]any{"release_date": "2008-08-01T00:00:00.000Z", "type": 4},
			}}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"movie_results": []any{}, "tv_results": []any{}})
		}
	}))
	t.Cleanup(fake.Close)
	s := newTestServer(t, 10)
	var today atomic.Int64
	today.Store(time.Date(2008, 5, 1, 12, 0, 0, 0, time.UTC).Unix())
	s.library.SetClock(func() time.Time { return time.Unix(today.Load(), 0).UTC() })
	s.library.UseTMDB(tmdb.New(fake.URL+"/3", fake.URL+"/img/", "test", s.store.Settings))
	p := playingOn(t, s)
	if answer := p.playbackInfo(t, "tv", p.token, p.movie, nil); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("without the wait: %+v", answer)
	}
	p.setting(t, func(settings *accounts.Settings) {
		settings.TMDBKey, settings.WaitDigitalRelease = "k3y", true
	})
	if answer := p.playbackInfo(t, "tv", p.token, p.movie, nil); answer.ErrorCode != "NotAllowed" || len(answer.MediaSources) != 0 {
		t.Errorf("before 1 August: %+v", answer)
	}
	today.Store(time.Date(2008, 8, 1, 12, 0, 0, 0, time.UTC).Unix())
	if answer := p.playbackInfo(t, "tv", p.token, p.movie, nil); answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Errorf("on 1 August: %+v", answer)
	}
}
