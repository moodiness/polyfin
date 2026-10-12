package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// listens returns the forms of POLYFIN_LISTEN that name no host, or every
// address, for port: the check reaches them all on the loopback.
func listens(port int) []string {
	text := strconv.Itoa(port)
	return []string{":" + text, "0.0.0.0:" + text, "[::]:" + text}
}

// serverPort starts a loopback server answering /healthz with handler,
// and every other path with 404, and returns its port.
func serverPort(t *testing.T, handler http.HandlerFunc) int {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server.Listener.Addr().(*net.TCPAddr).Port
}

func TestHealthcheck(t *testing.T) {
	t.Parallel()
	healthy := serverPort(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	failing := serverPort(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	unblock := make(chan struct{})
	slow := serverPort(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-unblock:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(unblock) })
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	none := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	cases := []struct {
		name    string
		port    int
		healthy bool
	}{
		{"answering 200", healthy, true},
		{"answering 500", failing, false},
		{"answering after the timeout", slow, false},
		{"no server", none, false},
	}
	for _, c := range cases {
		for _, listen := range listens(c.port) {
			getenv := func(name string) string {
				if name == "POLYFIN_LISTEN" {
					return listen
				}
				return ""
			}
			err := healthcheck(getenv, 500*time.Millisecond)
			if c.healthy && err != nil {
				t.Errorf("%s, POLYFIN_LISTEN=%s: %v", c.name, listen, err)
			}
			if !c.healthy && err == nil {
				t.Errorf("%s, POLYFIN_LISTEN=%s: healthy", c.name, listen)
			}
		}
	}
}
