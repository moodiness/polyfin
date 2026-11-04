package playback

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/source"
	"github.com/moodiness/polyfin/internal/torrent"
)

// routeDown fails every request as a torrent's file read over a SOCKS5
// proxy that does not answer: the dial was cut by its deadline, as
// Windows reports it.
type routeDown struct{}

func (routeDown) Open(context.Context, string, string, http.Header, bool) (*http.Response, error) {
	return nil, &torrent.RouteError{Kind: torrent.ProxyUnreachable, Route: "192.0.2.1:1080",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}}
}

// A version's source failing while it plays, as the route of torrents does
// not answer, is told as the route's failure, without the addon, which was
// never asked: not as an addon that gave no answer in time.
func TestARouteFailureWhilePlayingTellsTheRoute(t *testing.T) {
	t.Parallel()
	s := newService(t, routeDown{}, "", nil)
	log := &records{}
	s.logger = slog.New(log)
	version := library.Version{ID: accounts.ID{8, 9, 9}, URL: "https://93.184.216.34/movie.mkv", Addon: "Torrents addon"}
	src := s.OpenSource(version)
	defer src.Release()
	if _, err := src.ReadAt(t.Context(), make([]byte, 10), 0); !errors.Is(err, source.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	s.watchSource(version, src, make(chan struct{}))
	const said = "A version's source failed while it played"
	if answer, _ := log.attr(said, "answer"); answer != "the route for torrents does not answer" {
		t.Errorf("answer %q", answer)
	}
	if addon, found := log.attr(said, "addon"); found {
		t.Errorf("the addon %q is named", addon)
	}
	if s.Failed(version.ID) {
		t.Error("the version is kept as failed")
	}
}
