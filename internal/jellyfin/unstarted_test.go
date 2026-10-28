package jellyfin

import (
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/torrent/torrenttest"
)

// detailsNames lists the names of the versions a title's details give,
// in their order, as jellyfin-web's version menu shows them.
func (p torrentPlay) detailsNames(t *testing.T) []string {
	t.Helper()
	var details BaseItemDto
	if status := p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &details); status != http.StatusOK || details.MediaSources == nil {
		t.Fatalf("details: %d %+v", status, details.MediaSources)
	}
	var names []string
	for _, source := range *details.MediaSources {
		// A version's name may end with how the browser plays it.
		name, _, _ := strings.Cut(source.Name, " (")
		names = append(names, name)
	}
	return names
}

// Torrents that did not start in time stay in the title's versions, in
// the addon's order: a Play that picks no version skips them and tries the
// next ones, which play; a version the user picks by its own identifier is
// tried again, its torrent started once more.
func TestTorrentsThatDidNotStartStayListedAndArePassedOverByPlays(t *testing.T) {
	t.Parallel()
	ffmpeg, live := torrentMovie(t, "A Movie")
	seeder := torrenttest.Seed(t, live, "127.0.0.1")
	var tors []*torrenttest.Torrent
	var trackers []string
	// Four torrents whose trackers know no peer, then one that plays.
	for _, name := range []string{"Dead One", "Dead Two", "Dead Three", "Dead Four"} {
		tors = append(tors, torrenttest.Build(name, 64<<10, torrenttest.File{Name: "dead.mkv", Data: torrenttest.Bytes(300 << 10)}))
		trackers = append(trackers, torrenttest.HTTPTracker(t, "127.0.0.1").URL())
	}
	tors = append(tors, live)
	trackers = append(trackers, torrenttest.HTTPTracker(t, "127.0.0.1", seeder.Address()).URL())
	var logged lockedLog
	p := newTorrentPlay(t, filepath.Join(filepath.Dir(ffmpeg), "ffprobe"), 2*time.Second, tors, trackers, func(o *Options) {
		o.Logger = slog.New(slog.NewTextHandler(&logged, nil))
	})
	tried := func() int { return strings.Count(logged.String(), `msg="A version could not be analyzed"`) }
	all := []string{"Torrent 1", "Torrent 2", "Torrent 3", "Torrent 4", "Torrent 5"}
	if got := p.detailsNames(t); !slices.Equal(got, all) {
		t.Fatalf("versions before Play: %q", got)
	}

	// Play, as jellyfin-web sends it, under the title's own identifier:
	// the first three versions are tried, and none starts.
	if _, code := p.play(t, p.details(t)); code != "RateLimitExceeded" {
		t.Fatalf("the first Play answered %q", code)
	}
	for i := range 3 {
		if !p.handler.Playback.Unstarted(p.versions[i].ID) {
			t.Fatalf("version %d was not kept as a torrent that did not start", i+1)
		}
	}
	if got := p.detailsNames(t); !slices.Equal(got, all) {
		t.Errorf("versions after a Play whose torrents did not start: %q", got)
	}

	// Play again: the three are passed over, the fourth does not start
	// either, and the fifth plays.
	before := tried()
	info, code := p.play(t, p.details(t))
	if code != "" {
		t.Fatalf("the second Play answered %q", code)
	}
	p.converted(t, info, p.versions[4].ID)
	if n := tried() - before; n != 1 {
		t.Errorf("the second Play failed %d versions; want the fourth alone", n)
	}

	// The user picks the second version in the menu: its torrent starts
	// again, and does not start in time again.
	unstarted := p.engine.Status(t.Context()).UnstartedAt
	if _, code := p.play(t, p.versions[1].ID.String()); code != "RateLimitExceeded" {
		t.Errorf("the version picked answered %q", code)
	}
	if again := p.engine.Status(t.Context()).UnstartedAt; !again.After(unstarted) {
		t.Errorf("the version picked was not tried again: its torrent last failed at %v, before at %v", again, unstarted)
	}
	if got := p.detailsNames(t); !slices.Equal(got, all) {
		t.Errorf("versions at the end: %q", got)
	}
}
