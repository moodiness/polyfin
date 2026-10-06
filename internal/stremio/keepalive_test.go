package stremio

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A wave of requests to one addon reuses the connections of the wave
// before instead of opening new ones.
func TestWavesOfRequestsReuseConnections(t *testing.T) {
	var opened atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"metas":[]}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	client := NewClient("test")
	wave := func() {
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				if _, err := client.Catalog(t.Context(), server.URL+"/manifest.json", "movie", "top", nil, false); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
	}
	wave()
	first := opened.Load()
	wave()
	if got := opened.Load() - first; got > 0 {
		t.Errorf("the second wave opened %d connections, the first %d", got, first)
	}
}
