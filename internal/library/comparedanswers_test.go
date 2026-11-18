package library

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// A first answer that replaces a list still kept is compared with it as
// the list is replaced (see keepAnswer), at the same cost: each link is
// parsed once, and each stream compared with a few others. Its streams
// keep the identifiers of those they were.
func TestAnAnswerIsComparedWithTheStaleListItReplacesAtTheSameCost(t *testing.T) {
	t.Parallel()
	streams := func(folder string, count int) []stremio.Stream {
		streams := make([]stremio.Stream, count)
		for n := range streams {
			streams[n] = stremio.Stream{URL: fmt.Sprintf("https://f.example/%s/%d", folder, n)}
		}
		return streams
	}
	for _, count := range []int{500, 1000} {
		t.Run(fmt.Sprintf("%d streams", count), func(t *testing.T) {
			t.Parallel()
			answers := make(chan []stremio.Stream, 2)
			e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
				if resource != "stream" {
					_ = json.NewEncoder(w).Encode(map[string]any{"subtitles": []stremio.Subtitle{}})
					return
				}
				select {
				case streams := <-answers:
					_ = json.NewEncoder(w).Encode(map[string]any{"streams": streams})
				case <-r.Context().Done():
				}
			})
			var mu sync.Mutex
			var costs []carryCost
			e.service.followUps.carry = func(answer, before []stremio.Stream) []stremio.Stream {
				carried, cost := carriedIdentities(answer, before)
				mu.Lock()
				defer mu.Unlock()
				costs = append(costs, cost)
				return carried
			}
			answers <- streams("a", count)
			first, err := e.service.Versions(t.Context(), e.member, e.movie)
			if err != nil || len(first) != count {
				t.Fatalf("first answer: %d versions, %v", len(first), err)
			}
			// The list expires, and is kept as a stale list: opened again,
			// the title asks the addon, whose answer replaces it.
			e.wait(e.service.listLife() + time.Minute)
			answers <- streams("b", count+1)
			versions, err := e.service.Versions(t.Context(), e.member, e.movie)
			if err != nil || len(versions) != count+1 {
				t.Fatalf("the answer that replaces the stale list: %d versions, %v", len(versions), err)
			}
			for i := range first {
				if versions[i].ID != first[i].ID {
					t.Fatalf("version %d is %s, was %s", i, versions[i].ID, first[i].ID)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if len(costs) != 1 {
				t.Fatalf("the answer was compared %d times with the list it replaces, want once", len(costs))
			}
			if links := 2*count + 1; costs[0].parsed != links || costs[0].compared > mostCompared*links {
				t.Errorf("%d streams replaced by %d: %d links parsed and %d pairs compared, want each of the %d links parsed once, and %d pairs at most",
					count, count+1, costs[0].parsed, costs[0].compared, links, mostCompared*links)
			}
		})
	}
}

// While a follow-up's answer is compared with the list it may replace, the
// locks every title's lists share are free: another title's versions are
// listed, and the title's own are those of the list not replaced yet.
func TestAFollowUpComparesItsAnswerWithNoLockOfEveryList(t *testing.T) {
	t.Parallel()
	other := titles("movie", 2)[1]
	replies := make(chan int, 2)
	scripted := scriptedReplies(replies)
	e := repliedEnv(t, func(w http.ResponseWriter, r *http.Request, resource string) {
		// The other title has no stream from this addon.
		if strings.Contains(r.URL.Path, other.ID) {
			answerNumbered(w, resource, nil)
			return
		}
		scripted(w, r, resource)
	}, time.Hour, time.Hour)
	e.install(addons.Shared(), &fakeAddon{
		manifest: stremio.Manifest{ID: "more", Name: "More", Version: "1", Types: []string{"movie"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "more", Name: "More"}}},
		catalogs: map[string][]stremio.Meta{"movie/more": {other}},
		metas:    map[string]stremio.Meta{"movie/" + other.ID: other},
		streams:  map[string][]stremio.Stream{"movie/" + other.ID: {{Name: "More", URL: "https://cdn.example/more.mkv"}}},
	})
	page, err := e.service.Children(t.Context(), e.member, e.library(e.member, "More").ID, 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("listing the other title: %+v %v", page, err)
	}
	// The comparison is held until the test releases it.
	comparing, compared := make(chan []stremio.Stream, 1), make(chan struct{})
	release := sync.OnceFunc(func() { close(compared) })
	t.Cleanup(release)
	e.service.followUps.carry = func(answer, before []stremio.Stream) []stremio.Stream {
		select {
		case comparing <- before:
		case <-compared:
		}
		<-compared
		return carryIdentities(answer, before)
	}
	replies <- 1
	if versions, err := e.service.Versions(t.Context(), e.member, e.movie); err != nil || len(versions) != 1 {
		t.Fatalf("first answer: %+v %v", versions, err)
	}
	replies <- 3
	e.hurry()
	if before := next(t, comparing, "the follow-up's answer to be compared"); len(before) != 1 {
		t.Fatalf("the answer is compared with %d streams, want the one of the first answer", len(before))
	}
	listed := make(chan int, 1)
	go func() {
		versions, _ := e.service.Versions(t.Context(), e.member, page.Items[0].ID)
		listed <- len(versions)
	}()
	if n := next(t, listed, "another title's versions while the answer is compared"); n != 1 {
		t.Errorf("another title lists %d versions meanwhile, want 1", n)
	}
	if got := e.known(); got != 1 {
		t.Errorf("%d versions while the answer is compared, want 1: the list is not replaced yet", got)
	}
	release()
	e.eventually("the follow-up's versions", func() bool { return e.known() == 3 })
}

// A list that changed while a follow-up's answer was compared with it is
// not replaced by what the comparison made of the list before: the answer
// is compared again with the list as it is, a few times at most.
func TestAFollowUpComparesItsAnswerAgainWithAListThatChanged(t *testing.T) {
	t.Parallel()
	others := func(numbers ...int) []stremio.Stream {
		streams := make([]stremio.Stream, len(numbers))
		for i, n := range numbers {
			streams[i] = stremio.Stream{Name: fmt.Sprintf("Other %d", n), URL: fmt.Sprintf("https://cdn.example/other/x%d.mkv", n)}
		}
		return streams
	}
	for _, tc := range []struct {
		name string
		// change is the list stored while the answer is compared for the
		// nth time, its first answer items; nil for none.
		change func(n int) (items []stremio.Stream, answer int)
		// compared is how many times the answer is compared, want the
		// versions listed after, and ends whether the schedule ends
		// there.
		compared int
		want     []string
		ends     bool
	}{
		{"for a longer list", func(n int) ([]stremio.Stream, int) { return others(1, 2, 3, 4, 5), 5 },
			1, []string{"Other 1", "Other 2", "Other 3", "Other 4", "Other 5"}, true},
		{"for a list that keeps a stream of the list before it", func(n int) ([]stremio.Stream, int) {
			if n > 1 {
				return nil, 0
			}
			return others(1, 2), 1
		}, 2, []string{"Source 1", "Source 2", "Source 3", "Other 2"}, false},
		{"each time", func(n int) ([]stremio.Stream, int) { return others(n), 1 },
			followUpTries, []string{fmt.Sprintf("Other %d", followUpTries)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newFollowEnv(t, time.Hour, time.Hour)
			var key streamKey
			var compared atomic.Int32
			e.service.followUps.carry = func(answer, before []stremio.Stream) []stremio.Stream {
				if items, first := tc.change(int(compared.Add(1))); items != nil {
					e.service.streamLists.Put(key, list[stremio.Stream]{items: items, answer: first, at: e.service.now()})
				}
				return carryIdentities(answer, before)
			}
			e.replies <- 1
			versions, err := e.service.Versions(t.Context(), e.member, e.movie)
			if err != nil || len(versions) != 1 {
				t.Fatalf("first answer: %+v %v", versions, err)
			}
			key = streamKey{versions[0].Origin.Addon, versions[0].Origin.Type, versions[0].Origin.ID}
			e.replies <- 3
			due := e.schedule()
			due.hurry <- struct{}{}
			next(t, due.asked, "the follow-up")
			listed, err := e.service.KnownVersions(t.Context(), e.member, e.movie)
			if err != nil {
				t.Fatal(err)
			}
			if got := versionNames(listed); !slices.Equal(got, tc.want) || int(compared.Load()) != tc.compared {
				t.Errorf("after %d comparisons, the title lists %q, want %q after %d", compared.Load(), got, tc.want, tc.compared)
			}
			if ended := e.scheduled() == 0; ended != tc.ends {
				t.Errorf("the schedule ended: %v, want %v", ended, tc.ends)
			}
		})
	}
}
