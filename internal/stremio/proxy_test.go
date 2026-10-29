package stremio

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// A file an addon points to is read through the proxy, and a confined
// request ignores it.
func TestOpenViaProxy(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "through the proxy")
	}))
	defer server.Close()
	proxy := torrenttest.StartProxy(t, false, "", "")
	defer proxy.Stop()
	client := NewClient("test")
	response, err := client.OpenViaProxy(t.Context(), "socks5h://"+proxy.Address(), http.MethodGet, server.URL, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "through the proxy" {
		t.Fatalf("body: %q, %v", body, err)
	}
	target := "tcp " + strings.TrimPrefix(server.URL, "http://")
	if !slices.Contains(proxy.Targets(), target) {
		t.Errorf("targets: %v, want %s", proxy.Targets(), target)
	}
}

func TestOpenViaProxyRefusesAnInvalidProxy(t *testing.T) {
	t.Parallel()
	client := NewClient("test")
	_, err := client.OpenViaProxy(t.Context(), "warp:1080", http.MethodGet, "https://example.org/file", nil, false)
	if !errors.Is(err, ErrUnreachable) {
		t.Errorf("invalid proxy: %v", err)
	}
}

func TestOpenViaProxyLeavesConfinedRequestsDirect(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := NewClient("test")
	// A private address is refused by the confined client, before the
	// proxy's URL is read: an invalid one changes nothing.
	_, err := client.OpenViaProxy(t.Context(), "warp:1080", http.MethodGet, server.URL, nil, true)
	if !errors.Is(err, ErrPrivateNetwork) {
		t.Errorf("confined request: %v", err)
	}
}
