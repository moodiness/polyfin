package playback

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
)

// gatedSource serves a file as fileSource does, each request held until
// gate closes, and tells the files asked for so far by host.
type gatedSource struct {
	data []byte
	gate chan struct{}
	mu   sync.Mutex
	// asked are the files asked for, by host.
	asked map[string]map[string]bool
}

func (g *gatedSource) Open(ctx context.Context, method, target string, header http.Header, _ bool) (*http.Response, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	if g.asked[u.Host] == nil {
		g.asked[u.Host] = map[string]bool{}
	}
	g.asked[u.Host][u.Path] = true
	g.mu.Unlock()
	select {
	case <-g.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	request.Header = header.Clone()
	recorder := httptest.NewRecorder()
	http.ServeContent(recorder, request, "", time.Time{}, bytes.NewReader(g.data))
	response := recorder.Result()
	response.Request = request
	return response, nil
}

// reading returns the files asked for so far, by host, and in all.
func (g *gatedSource) reading() (byHost map[string]int, total int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	byHost = map[string]int{}
	for host, files := range g.asked {
		byHost[host] = len(files)
		total += len(files)
	}
	return byHost, total
}

// waitingForIndexSlots returns how many of the goroutines labeled
// indexread with one of names wait for a slot to read an index.
func waitingForIndexSlots(t *testing.T, names ...string) int {
	t.Helper()
	var profile strings.Builder
	if err := pprof.Lookup("goroutine").WriteTo(&profile, 1); err != nil {
		t.Fatal(err)
	}
	waiting := 0
	for _, name := range names {
		label := fmt.Sprintf("%q:%q", "indexread", name)
		for record := range strings.SplitSeq(profile.String(), "\n\n") {
			if strings.Contains(record, label) && strings.Contains(record, "(*Service).indexSlot") {
				waiting++
				break
			}
		}
	}
	return waiting
}

// More container indexes than their slots are never read at once: two
// from a host and four over every host. The reads past them wait, and a
// read waiting ends with its context without asking its host anything.
func TestIndexReadsWaitForTheirSlots(t *testing.T) {
	t.Parallel()
	data := allSyncMP4(10)
	opener := &gatedSource{data: data, gate: make(chan struct{}), asked: map[string]map[string]bool{}}
	s := newService(t, opener, "ffprobe", nil)
	// Reads still held when the test ends are served, so that the service
	// closes.
	serve := sync.OnceFunc(func() { close(opener.gate) })
	t.Cleanup(serve)
	ctx := t.Context()
	analysis := media.Analysis{Size: int64(len(data))}
	// Three versions on one host, two on another, one on a third: six
	// reads, past both bounds.
	const busy = "93.184.216.34"
	hosts := []string{busy, busy, busy, "93.184.216.35", "93.184.216.35", "93.184.216.36"}
	names := make([]string, len(hosts))
	errs := make(chan error, len(hosts))
	for i, host := range hosts {
		names[i] = fmt.Sprintf("v%d", i)
		version := library.Version{ID: accounts.ID{0xB0, byte(i)}, URL: fmt.Sprintf("https://%s/%s.mp4", host, names[i])}
		go pprof.Do(ctx, pprof.Labels("indexread", names[i]), func(ctx context.Context) {
			times, err := s.keyframes(ctx, version, analysis)
			if err == nil && len(times) != 10 {
				err = fmt.Errorf("%d keyframes, want 10", len(times))
			}
			errs <- err
		})
	}
	// Held at their hosts, the reads under way fill the slots; the rest
	// wait for one.
	for {
		byHost, total := opener.reading()
		for host, n := range byHost {
			if n > hostTrackReads {
				t.Fatalf("%d indexes read at once from %s, want %d at most", n, host, hostTrackReads)
			}
		}
		if total > maxTrackReads {
			t.Fatalf("%d indexes read at once, want %d at most", total, maxTrackReads)
		}
		if total == maxTrackReads && waitingForIndexSlots(t, names...) == len(hosts)-maxTrackReads {
			break
		}
		time.Sleep(time.Millisecond)
	}
	// One more read from the first host, as a warm reads one, waits too,
	// and ends with its context.
	version := library.Version{ID: accounts.ID{0xB1}, URL: "https://" + busy + "/warmed.mp4"}
	src := s.open(version)
	defer src.Release()
	waiting, cancel := context.WithCancel(ctx)
	defer cancel()
	warmed := make(chan error, 1)
	go pprof.Do(waiting, pprof.Labels("indexread", "warmed"), func(ctx context.Context) {
		_, err := s.keyframeOffsets(ctx, version, src, int64(len(data)))
		warmed <- err
	})
	askedWarmed := func() bool {
		opener.mu.Lock()
		defer opener.mu.Unlock()
		return opener.asked[busy]["/warmed.mp4"]
	}
	for waitingForIndexSlots(t, "warmed") == 0 {
		if askedWarmed() {
			t.Fatal("a read past its host's slots asked the host")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-warmed; !errors.Is(err, context.Canceled) {
		t.Fatalf("the waiting read ended with %v, want its context's end", err)
	}
	if askedWarmed() {
		t.Error("the read ended waiting asked its host")
	}
	// Served, the reads end.
	serve()
	for range hosts {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}
