package admin

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
)

// System › Health tells how artwork downloads fared in the last hour, by
// kind and by host name, ten hosts at most, and the hosts waited for; and
// how people's lookups on TMDB stand, with TMDB's last 429.
func TestHealthTellsArtworkAndPeople(t *testing.T) {
	t.Parallel()
	until := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	limited := time.Date(2026, 11, 4, 8, 30, 0, 0, time.UTC)
	artwork := jellyfin.ArtworkStatus{Failed: 14, Kinds: map[string]int{jellyfin.ArtworkNotFound: 2, jellyfin.ArtworkRateLimited: 12},
		Waiting: []jellyfin.ArtworkHostWait{{Host: "art.example", Until: until}}, Spared: []jellyfin.ArtworkHostWait{}}
	for i := range 12 {
		artwork.Hosts = append(artwork.Hosts, jellyfin.ArtworkHostFailures{Host: fmt.Sprintf("host%d.example", i), Failed: 12 - i})
	}
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Health = HealthSources{
			Artwork: func() jellyfin.ArtworkStatus { return artwork },
			People: func(context.Context) (library.PeopleStatus, error) {
				return library.PeopleStatus{Enabled: true, Found: 5, NotFound: 2, Failing: 1, Waiting: 3, LimitedAt: limited, LimitWait: 7 * time.Second}, nil
			},
		}
	})
	admin := api.signedIn("root", true)
	var health healthJSON
	if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || health.Artwork == nil || health.People == nil {
		t.Fatalf("health: %d %+v %+v", status, health.Artwork, health.People)
	}
	if a := health.Artwork; a.Failed != 14 || a.Kinds[jellyfin.ArtworkRateLimited] != 12 || len(a.Hosts) != 10 || a.Hosts[0] != (artworkHostJSON{Host: "host0.example", Failed: 12}) ||
		len(a.Waiting) != 1 || a.Waiting[0].Host != "art.example" || !a.Waiting[0].Until.Equal(until) {
		t.Errorf("artwork: %+v", a)
	}
	if p := health.People; !p.KeySaved || p.Found != 5 || p.NotFound != 2 || p.Failing != 1 || p.Waiting != 3 ||
		p.LimitedAt == nil || !p.LimitedAt.Equal(limited) || p.LimitWait != 7 {
		t.Errorf("people: %+v", p)
	}
}
