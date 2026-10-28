package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// infoLog keeps the messages a logger handles at INFO or above.
type infoLog struct {
	mu       sync.Mutex
	messages []string
}

func (l *infoLog) Enabled(_ context.Context, level slog.Level) bool { return level >= slog.LevelInfo }

func (l *infoLog) Handle(_ context.Context, record slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, record.Message)
	return nil
}

func (l *infoLog) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l *infoLog) WithGroup(string) slog.Handler { return l }

func (l *infoLog) logged(message string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Contains(l.messages, message)
}

// leftOut is what the log says of a live stream PlaybackInfo leaves out
// for a recent failure.
const leftOut = "A channel's stream is left out: it failed lately"

// A channel whose stream failed lately is left out of PlaybackInfo until
// its failure is forgotten, and the log says so, where the app only tells
// that nothing plays.
func TestAChannelLeftOutForAFailureIsLogged(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/get.php" {
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Zeb One\n%s/live/one.ts\n", server.URL)
			return
		}
		// The stream is dead: an error that is no refusal.
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	if _, err := s.iptv.Add(t.Context(), addons.Shared(), iptv.NewSource{Name: "Provider",
		Account: iptv.Account{Kind: addons.KindM3U, URL: server.URL + "/get.php"}}, false); err != nil {
		t.Fatal(err)
	}
	var channels QueryResult
	s.get(t, "/LiveTv/Channels", token, &channels)
	if len(channels.Items) != 1 {
		t.Fatalf("channels: %+v", channels.Items)
	}
	log := &infoLog{}
	s.handler.Logger = slog.New(log)
	for range 2 {
		if answer := s.playbackInfo(t, "tv", token, channels.Items[0].Id, map[string]any{"IsPlayback": true}); len(answer.MediaSources) != 0 {
			t.Fatalf("a dead stream played: %+v", answer)
		}
	}
	if !log.logged(leftOut) {
		t.Errorf("a channel left out for a failure is not logged: %q", log.messages)
	}
}

// A video of an addon's other titles found live, whose streams all failed
// lately, plays as a live video still: its PlaybackInfo says in the log
// why its stream is left out, where it answered nothing as a file would.
func TestALiveVideoLeftOutForAFailureIsLogged(t *testing.T) {
	t.Parallel()
	// No live playlist: the match's stream answers 404.
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	addon, err := s.addons.Install(t.Context(), addons.Shared(), sportsAddon(t, t.TempDir(), "/nonexistent.mp4"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{{AddonID: addon.ID, CatalogType: "sport", CatalogID: "live"}}); err != nil {
		t.Fatal(err)
	}
	var views, live QueryResult
	s.get(t, "/UserViews", token, &views)
	if len(views.Items) != 1 {
		t.Fatalf("views: %+v", views.Items)
	}
	s.get(t, "/Items?ParentId="+views.Items[0].Id, token, &live)
	if len(live.Items) != 1 {
		t.Fatalf("the Live Now library: %+v", live.Items)
	}
	match, _ := accounts.ParseID(live.Items[0].Id)
	versions, err := s.library.Versions(t.Context(), member, match)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %v %v", versions, err)
	}
	// It fails, then is known live: here its analysis fails as no
	// ffprobe runs, and a live analysis is seeded.
	if _, err := s.handler.Playback.Analyze(t.Context(), versions[0]); err == nil {
		t.Fatal("analyzed without ffprobe")
	}
	eventually(t, "the failure kept", func() bool { return s.handler.Playback.Failed(versions[0].ID) })
	seedLiveAnalysis(t, s, versions[0].ID)
	log := &infoLog{}
	s.handler.Logger = slog.New(log)
	if answer := s.playbackInfo(t, "tv", token, live.Items[0].Id, map[string]any{"IsPlayback": true}); len(answer.MediaSources) != 0 {
		t.Fatalf("a failed stream played: %+v", answer)
	}
	if !log.logged(leftOut) {
		t.Errorf("a live video left out for a failure is not logged: %q", log.messages)
	}
}
