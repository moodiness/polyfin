package jellyfin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/iptv"
)

// channelLog keeps the channel attribute of the INFO records a logger
// handles, by message.
type channelLog struct {
	mu       sync.Mutex
	channels map[string]string
}

func (l *channelLog) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (l *channelLog) Handle(_ context.Context, record slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	record.Attrs(func(a slog.Attr) bool {
		if a.Key == "channel" {
			l.channels[record.Message] = a.Value.String()
		}
		return true
	})
	return nil
}

func (l *channelLog) WithAttrs([]slog.Attr) slog.Handler { return l }

func (l *channelLog) WithGroup(string) slog.Handler { return l }

// A channel whose stream cannot be analyzed is named in the log, as lists
// hold hundreds of channels of one provider.
func TestAChannelThatCannotBeAnalyzedIsNamedInTheLog(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/get.php" {
			_, _ = fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1 tvg-id=\"\",Zeb One\n%s/live/one/index.m3u8\n", server.URL)
			return
		}
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
	log := &channelLog{channels: map[string]string{}}
	s.handler.Logger = slog.New(log)
	if answer := s.playbackInfo(t, "tv", token, channels.Items[0].Id, map[string]any{"IsPlayback": true}); len(answer.MediaSources) != 0 {
		t.Fatalf("a dead stream played: %+v", answer)
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if got := log.channels["A channel's stream could not be analyzed"]; got != "Zeb One" {
		t.Errorf("the log names the channel %q", got)
	}
}
