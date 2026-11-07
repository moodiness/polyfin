package jellyfin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
)

// accountReads counts the statements of the accounts store that read the
// users table and the devices table.
type accountReads struct{ users, devices atomic.Int64 }

var (
	readsUsers   = regexp.MustCompile(`(?i)\b(from|join)\s+users\b`)
	readsDevices = regexp.MustCompile(`(?i)\b(from|join)\s+devices\b`)
)

func (c *accountReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if readsUsers.MatchString(data.SQL) {
		c.users.Add(1)
	}
	if readsDevices.MatchString(data.SQL) {
		c.devices.Add(1)
	}
	return ctx
}

func (c *accountReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// take returns the reads counted since the last take.
func (c *accountReads) take() (users, devices int64) {
	return c.users.Swap(0), c.devices.Swap(0)
}

// Once the user is read, the requests a playback repeats read no users row
// and no devices row: the stream's range requests, under a play session or
// a token, the HLS playlists and segments, and the live sessions the admin
// app polls.
func TestWarmPlaybackRequestsReadNoUserNorDevice(t *testing.T) {
	t.Parallel()
	reads := &accountReads{}
	s := newServerReaching(t, 10, "ffprobe-not-installed", nil, func(o *Options, pool *pgxpool.Pool) {
		config := pool.Config()
		config.ConnConfig.Tracer = reads
		counted, err := pgxpool.NewWithConfig(t.Context(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(counted.Close)
		if o.Accounts, err = accounts.Open(t.Context(), counted); err != nil {
			t.Fatal(err)
		}
	})
	p := playingOn(t, s)
	p.remuxable(t)

	status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"MediaSourceId": p.versions[1].ID.String()})
	var info playbackInfoResponse
	if err := json.Unmarshal(data, &info); status != http.StatusOK || err != nil {
		t.Fatalf("playback info: %d %s", status, data)
	}
	stream := p.url + "/Videos/" + p.movie + "/stream?static=true&mediaSourceId=" + info.MediaSources[0].Id + "&tag=" + info.MediaSources[0].ETag
	ranged := func(target string) int {
		t.Helper()
		request, _ := http.NewRequest(http.MethodGet, target, nil)
		request.Header.Set("Range", "bytes=0-3")
		response, err := testClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}

	status, data = p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token), map[string]any{
		"UserId": p.user.ID.String(), "MediaSourceId": p.movie, "MaxStreamingBitrate": 120_000_000, "EnableDirectPlay": false,
		"SubtitleStreamIndex": 0, "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
	var repackaged playbackInfoResponse
	if err := json.Unmarshal(data, &repackaged); status != http.StatusOK || err != nil || len(repackaged.MediaSources) != 1 {
		t.Fatalf("HLS playback info: %d %s", status, data)
	}
	target := repackaged.MediaSources[0].TranscodingUrl
	query := strings.SplitN(target, "?", 2)[1]
	base := p.url + strings.Split(target, "master.m3u8")[0]

	if status, data := p.call(http.MethodPost, "/Sessions/Playing", app("tv", p.token), map[string]any{
		"ItemId": p.movie, "MediaSourceId": p.versions[1].ID.String(), "PositionTicks": 0, "PlayMethod": "DirectPlay"}); status != http.StatusNoContent {
		t.Fatalf("playing: %d %s", status, data)
	}

	for _, request := range []struct {
		name string
		send func() int
		want int
	}{
		{"stream under a play session", func() int { return ranged(stream + "&playSessionId=" + info.PlaySessionId) }, http.StatusPartialContent},
		{"stream under a token", func() int { return ranged(stream + "&ApiKey=" + p.token) }, http.StatusPartialContent},
		{"HLS playlist", func() int { status, _, _ := fetchText(t, base+"main.m3u8?"+query); return status }, http.StatusOK},
		{"HLS segment", func() int { status, _, _ := fetchText(t, base+"hls1/subtitles0/0.vtt?"+query); return status }, http.StatusOK},
		{"live sessions", func() int {
			sessions, err := p.handler.LiveSessions(t.Context())
			if err != nil || len(sessions) != 1 {
				t.Errorf("live sessions: %+v %v", sessions, err)
			}
			return 0
		}, 0},
	} {
		// The first request may read what is not remembered yet.
		if status := request.send(); status != request.want {
			t.Fatalf("%s: %d", request.name, status)
		}
		reads.take()
		for range 3 {
			request.send()
		}
		if users, devices := reads.take(); users != 0 || devices != 0 {
			t.Errorf("3 warm requests of a %s read users %d times and devices %d times", request.name, users, devices)
		}
	}
}
