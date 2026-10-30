package playback

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/virtualtv"
)

// playoutChannels airs one channel, its programme played from version.
type playoutChannels struct {
	channel virtualtv.Channel
	version library.Version
}

func (p playoutChannels) Channel(_ context.Context, id accounts.ID) (virtualtv.Channel, error) {
	if id != p.channel.ID {
		return virtualtv.Channel{}, virtualtv.ErrNotFound
	}
	return p.channel, nil
}

func (p playoutChannels) Versions(context.Context, accounts.ID) ([]library.Version, error) {
	return []library.Version{p.version}, nil
}

// A channel tuned 90 seconds into its programme airs it from 90 seconds
// in: the movie is black for 90 seconds then white, and the first segment
// of its playout is white.
func TestPlayoutTunedLateStartsThatFarIn(t *testing.T) {
	ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG")
	if ffmpeg == "" {
		t.Skip("POLYFIN_TEST_FFMPEG is not set")
	}
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=320x180:r=25:d=90", "-f", "lavfi", "-i", "color=c=white:s=320x180:r=25:d=60",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=150",
		"-filter_complex", "[0:v][1:v]concat=n=2:v=1:a=0[v]", "-map", "[v]", "-map", "2:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "50", "-c:a", "aac", path).CombinedOutput(); err != nil {
		t.Fatalf("make the movie: %v: %s", err, out)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s := speechService(t, ffmpeg, stremio.NewClient("test"))
	version := s.FileVersion(accounts.ID{9}, "Movie", path, info.Size())
	channel := virtualtv.Channel{ID: accounts.ID{7}, Name: "Ninety", Enabled: true, Epoch: time.Now().Add(-90 * time.Second),
		Entries: []virtualtv.Entry{{Item: accounts.ID{8}, Title: accounts.ID{8}, Kind: virtualtv.EntryMovie, Name: "Movie", Runtime: 150 * time.Second}}}
	s.UsePlayouts(playoutChannels{channel: channel, version: version})

	playlistURL := s.PlayoutVersion(accounts.ID{6}, channel).URL
	playlist := fetch(t, playlistURL)
	var first string
	for line := range strings.Lines(string(playlist)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			first = line
			break
		}
	}
	if first == "" {
		t.Fatalf("the playlist lists no segment:\n%s", playlist)
	}
	base, _ := url.Parse(playlistURL)
	segment, _ := base.Parse(first)
	file := filepath.Join(t.TempDir(), "first.ts")
	if err := os.WriteFile(file, fetch(t, segment.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-i", file, "-frames:v", "1",
		"-vf", "signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("measure the first segment: %v: %s", err, out)
	}
	_, value, ok := strings.Cut(string(out), "lavfi.signalstats.YAVG=")
	brightness, err := strconv.ParseFloat(strings.TrimSpace(strings.SplitN(value, "\n", 2)[0]), 64)
	if !ok || err != nil {
		t.Fatalf("no brightness in %s", out)
	}
	// Black is 16, white 235.
	if brightness < 200 {
		t.Fatalf("the first segment's brightness is %.0f: the programme was not tuned 90 seconds in", brightness)
	}
	s.StopPlayout(channel.ID)
}

func fetch(t *testing.T, target string) []byte {
	t.Helper()
	request, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d %s", target, response.StatusCode, body)
	}
	return body
}
