// Package tunertest runs a fake HDHomeRun tuner for tests: its documents
// (discover.json, lineup.json, lineup_status.json), its channel scans
// (lineup.post), its MPEG-TS streams, which take its tuners, and its
// answer to discovery on a UDP port of the loopback interface, which
// iptv.Service.UseTunerDiscovery asks directly: nothing is broadcast.
package tunertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/iptv"
)

// Channel is a channel of the tuner's line-up.
type Channel struct {
	Number, Name string
	// DRM marks a protected channel, which the tuner does not play.
	DRM      bool
	Favorite bool
}

// Options describe a tuner.
type Options struct {
	DeviceID    uint32
	Name, Model string
	// Tuners is how many channels it plays at once.
	Tuners   int
	Channels []Channel
	// Scanned is the line-up a channel scan finds.
	Scanned []Channel
}

// Tuner is a running fake tuner.
type Tuner struct {
	// URL is its address, http://127.0.0.1:port.
	URL string
	// Discovery is where it answers discovery, 127.0.0.1:port.
	Discovery string

	options Options
	stream  []byte

	mu       sync.Mutex
	channels []Channel
	scans    int
	playing  int
	most     int
}

// Start runs a tuner until the test ends.
func Start(t testing.TB, options Options) *Tuner {
	t.Helper()
	tuner := &Tuner{options: options, channels: options.Channels, stream: Stream(t)}
	server := httptest.NewServer(tuner)
	t.Cleanup(server.Close)
	tuner.URL = server.URL
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	tuner.Discovery = conn.LocalAddr().String()
	go func() {
		buffer := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if iptv.IsDiscoveryRequest(buffer[:n]) {
				_, _ = conn.WriteToUDP(iptv.DiscoveryReply(options.DeviceID, options.Tuners, tuner.URL), from)
			}
		}
	}()
	return tuner
}

// Stream is an MPEG-TS stream of a few seconds: encoded by the FFmpeg of
// POLYFIN_TEST_FFMPEG, else packets of nothing, which tests that only
// count streams relay as well.
func Stream(t testing.TB) []byte {
	t.Helper()
	if ffmpeg := os.Getenv("POLYFIN_TEST_FFMPEG"); ffmpeg != "" {
		out := filepath.Join(t.TempDir(), "channel.ts")
		command := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25:duration=2",
			"-f", "lavfi", "-i", "sine=duration=2", "-c:v", "mpeg2video", "-g", "12", "-c:a", "mp2", "-f", "mpegts", out)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("encoding a channel: %v %s", err, output)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	packet := make([]byte, 188)
	packet[0], packet[1], packet[2] = 0x47, 0x1f, 0xff
	return bytes.Repeat(packet, 2000)
}

// SetChannels changes the tuner's line-up.
func (t *Tuner) SetChannels(channels []Channel) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.channels = channels
}

// Scans is how many channel scans the tuner was asked for.
func (t *Tuner) Scans() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scans
}

// Streams are how many streams play now, and the most that played at once.
func (t *Tuner) Streams() (playing, most int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.playing, t.most
}

func (t *Tuner) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/discover.json":
		writeJSON(w, map[string]any{"FriendlyName": t.options.Name, "ModelNumber": t.options.Model,
			"DeviceID": fmt.Sprintf("%08X", t.options.DeviceID), "TunerCount": t.options.Tuners, "BaseURL": t.URL,
			"LineupURL": t.URL + "/lineup.json"})
	case r.URL.Path == "/lineup.json":
		t.mu.Lock()
		lineup := make([]map[string]any, 0, len(t.channels))
		for _, c := range t.channels {
			entry := map[string]any{"GuideNumber": c.Number, "GuideName": c.Name, "URL": t.URL + "/auto/v" + c.Number}
			if c.DRM {
				entry["DRM"] = 1
			}
			if c.Favorite {
				entry["Favorite"] = 1
			}
			lineup = append(lineup, entry)
		}
		t.mu.Unlock()
		writeJSON(w, lineup)
	case r.URL.Path == "/lineup_status.json":
		// A scan ends at once.
		writeJSON(w, map[string]any{"ScanInProgress": 0, "ScanPossible": 1, "Source": "Antenna", "SourceList": []string{"Antenna"}})
	case r.URL.Path == "/lineup.post" && r.Method == http.MethodPost:
		if r.URL.Query().Get("scan") != "start" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		t.mu.Lock()
		t.scans++
		if t.options.Scanned != nil {
			t.channels = t.options.Scanned
		}
		t.mu.Unlock()
	case strings.HasPrefix(r.URL.Path, "/auto/v"):
		t.play(w, r, strings.TrimPrefix(r.URL.Path, "/auto/v"))
	default:
		http.NotFound(w, r)
	}
}

// play streams a channel, in a loop, until its request ends: while every
// tuner plays, it answers as HDHomeRun tuners do, 503 with error 805.
func (t *Tuner) play(w http.ResponseWriter, r *http.Request, number string) {
	t.mu.Lock()
	found := false
	for _, c := range t.channels {
		found = found || c.Number == number && !c.DRM
	}
	switch {
	case !found:
		t.mu.Unlock()
		http.NotFound(w, r)
		return
	case t.playing >= t.options.Tuners:
		t.mu.Unlock()
		w.Header().Set("X-HDHomeRun-Error", "805")
		http.Error(w, "All Tuners In Use", http.StatusServiceUnavailable)
		return
	}
	t.playing++
	t.most = max(t.most, t.playing)
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.playing--
		t.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "video/mp2t")
	flusher, _ := w.(http.Flusher)
	const chunk = 188 * 64
	for {
		for at := 0; at < len(t.stream); at += chunk {
			if _, err := w.Write(t.stream[at:min(at+chunk, len(t.stream))]); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}
