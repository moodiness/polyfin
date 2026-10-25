package jellyfin

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

// reportAsWeb sends a playback report as jellyfin-web does when it plays a
// title's first version as it is: the title's own identifier as its media
// source, and no play session.
func (p capTitles) reportAsWeb(t *testing.T, device, token, path, name string) {
	t.Helper()
	body := map[string]any{"ItemId": p.movies[name], "MediaSourceId": p.movies[name], "PositionTicks": 0, "PlayMethod": "DirectPlay"}
	if status, data := p.call(http.MethodPost, path, app(device, token), body); status != http.StatusNoContent {
		t.Fatalf("%s of %s: %d %s", path, name, status, data)
	}
}

// streamOpen opens the stream of a title's torrent version, as a player
// does, and keeps the request open until the function it returns is
// called.
func (p capTitles) streamOpen(t *testing.T, token, name string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		p.url+"/Videos/"+p.movies[name]+"/stream.mkv?static=true&mediaSourceId="+p.versions[name].ID.String()+"&ApiKey="+token, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if response, err := http.DefaultClient.Do(request); err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// At Torrents at once, the torrent a device left in the web player gives
// its place to the device's next title, though the player reports the
// title it plays by its own identifier, with no play session, and its
// read of the file is still open when the next title's PlaybackInfo comes,
// to end a moment later.
func TestATorrentTheWebPlayerLeftMakesRoomAtTheCap(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	p.cap(t, 1)
	token := p.signIn("member", "web")
	if answer := p.ask(t, "web", token, "Iron Coast"); answer.ErrorCode != "" {
		t.Fatalf("the first title answered %q", answer.ErrorCode)
	}
	// The peer holds the pieces past those read: the player's read stays
	// open, as a player's does while it plays.
	p.seeders["Iron Coast"].Hold()
	defer p.seeders["Iron Coast"].Release()
	stop := p.streamOpen(t, token, "Iron Coast")
	defer stop()
	p.reportAsWeb(t, "web", token, "/Sessions/Playing", "Iron Coast")
	p.reportAsWeb(t, "web", token, "/Sessions/Playing/Stopped", "Iron Coast")

	answered := make(chan playbackAnswer, 1)
	go func() { answered <- p.ask(t, "web", token, "Signal Fire") }()
	// The player's read ends a moment after the next title's PlaybackInfo
	// came.
	time.Sleep(time.Second)
	select {
	case answer := <-answered:
		t.Fatalf("the next title answered %q while the player still read the torrent it left", answer.ErrorCode)
	default:
	}
	stop()
	if answer := <-answered; answer.ErrorCode != "" || len(answer.MediaSources) == 0 {
		t.Fatalf("the next title answered %q", answer.ErrorCode)
	}
	if p.running(t, "Iron Coast") || !p.running(t, "Signal Fire") {
		t.Fatal("the torrent the player left did not give its place")
	}
}

// A torrent another device plays in the web player, reported by its
// title's own identifier, is never ended for the cap: the user's other
// device moving on to another title is told it cannot play now.
func TestATorrentAnotherDevicePlaysInTheWebPlayerKeepsItsPlace(t *testing.T) {
	t.Parallel()
	p := newCapTitles(t, 0)
	p.cap(t, 1)
	web, tv := p.signIn("member", "web"), p.signIn("member", "tv")
	p.play(t, "web", web, "Iron Coast")
	p.report(t, "web", web, "/Sessions/Playing/Stopped", "Iron Coast", nil)
	// The tv plays the same title, as jellyfin-web reports it.
	if answer := p.ask(t, "tv", tv, "Iron Coast"); answer.ErrorCode != "" {
		t.Fatalf("the tv's play answered %q", answer.ErrorCode)
	}
	p.reportAsWeb(t, "tv", tv, "/Sessions/Playing", "Iron Coast")

	if answer := p.ask(t, "web", web, "Signal Fire"); answer.ErrorCode != "RateLimitExceeded" {
		t.Fatalf("the web device's next title answered %q, though the tv plays the torrent", answer.ErrorCode)
	}
	if !p.running(t, "Iron Coast") {
		t.Fatal("the torrent the tv plays was ended")
	}
}
