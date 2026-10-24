//go:build !windows

package jellyfin

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// An artwork server that resets the connection in the middle of an
// image's body is spared, as one that resets it before answering.
func TestArtworkServerResettingMidImageIsSpared(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
					return
				}
				// The headers and a quarter of the image, then a reset:
				// closing with no linger sends RST rather than FIN.
				_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: image/jpeg\r\nContent-Length: 4096\r\n\r\n")
				_, _ = conn.Write(make([]byte, 1024))
				_ = conn.(*net.TCPConn).SetLinger(0)
			}()
		}
	}()
	h := imageHandler(t.TempDir())
	if _, err := h.fetchArtwork(t.Context(), "http://"+listener.Addr().String()+"/poster.jpg", false, false); err == nil ||
		!strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("an image cut by a reset: %v", err)
	}
	h.images.mu.Lock()
	fetches, ok := h.images.hosts[listener.Addr().String()]
	spared := ok && time.Now().Before(fetches.slowUntil)
	h.images.mu.Unlock()
	if !spared {
		t.Error("a host that reset an image's download is not spared")
	}
}
