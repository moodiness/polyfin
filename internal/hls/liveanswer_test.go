package hls

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer is a buffer a logger writes to from several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A live encoding whose FFmpeg fails tells what its source last answered
// that was no media, and names its channel; what FFmpeg wrote keeps the
// names of the files it read, never their addresses on the loopback
// interface, which hold the source's.
func TestALiveEncodingThatFailsTellsWhatItsSourceAnswered(t *testing.T) {
	ffmpeg, _ := tools(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := listener.Addr().String()
	listener.Close()
	// The source's address, http://10.0.0.1/live/0123-token/index.m3u8,
	// in base64, as the loopback interface names a file.
	const target = "aHR0cDovLzEwLjAuMC4xL2xpdmUvMDEyMy10b2tlbi9pbmRleC5tM3U4"
	input := "http://" + host + "/live/k3y/" + target + "/index.m3u8?token=0123-token"
	log := &syncBuffer{}
	m, err := NewManager(ffmpeg, t.TempDir(), slog.New(slog.NewTextHandler(log, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	answer := errors.New("s3.ts answered text (HTTP 404, text/plain, 4 bytes)")
	open := func(context.Context) (Remux, func(), error) {
		return Remux{Input: input, Audio: -1, Format: TS, Channel: "Zeb Sport", Answered: func(time.Time) error { return answer }}, func() {}, nil
	}
	_, err = m.LivePlaylist(t.Context(), Key{Session: "live", Audio: -1, Format: TS}, open, func(name string) string { return name }, 1)
	if err == nil {
		t.Fatal("the encoding played")
	}
	t.Log(err)
	if !strings.Contains(err.Error(), "FFmpeg failed") || !strings.Contains(err.Error(), "index.m3u8") ||
		!strings.HasSuffix(err.Error(), "; "+answer.Error()) {
		t.Errorf("the failure does not tell what FFmpeg read and the source answered: %v", err)
	}
	failed := ""
	for line := range strings.Lines(log.String()) {
		if strings.Contains(line, `msg="A live encoding failed"`) {
			failed = line
		}
	}
	if !strings.Contains(failed, `channel="Zeb Sport"`) || !strings.Contains(failed, answer.Error()) {
		t.Errorf("the log line: %s", failed)
	}
	for _, secret := range []string{host, "k3y", target, "0123-token"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(log.String(), secret) {
			t.Errorf("%q is told:\n%s", secret, log)
		}
	}
}
