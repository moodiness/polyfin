package admin

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/thumbnails"
)

// thumbnailVersions is a title with a local folder's file and an addon's
// link, whose thumbnails are made again unless the settings turn them off.
type thumbnailVersions struct {
	Versions
	title, local, link accounts.ID
	off                atomic.Bool
	at                 time.Time
}

func (v *thumbnailVersions) ExplainVersions(_ context.Context, _ accounts.User, id accounts.ID) (jellyfin.VersionsExplanation, error) {
	if id != v.title {
		return jellyfin.VersionsExplanation{}, library.ErrNotFound
	}
	return jellyfin.VersionsExplanation{Sources: []jellyfin.SourceExplanation{{Name: "Shelf", State: "answered", Streams: []jellyfin.StreamExplanation{
		{Name: "Shelf · 1080p", Version: v.local, Step: jellyfin.StepOffered, Place: 1,
			Thumbnails: &thumbnails.Result{State: thumbnails.StateCannot, Reason: thumbnails.ReasonNoIndex, At: &v.at}},
		{Name: "Addon · 720p", Version: v.link, Step: jellyfin.StepOffered, Place: 2},
	}}}}, nil
}

func (v *thumbnailVersions) MakeThumbnailsAgain(_ context.Context, _ accounts.User, item, id accounts.ID) (thumbnails.Result, error) {
	switch {
	case item != v.title || id != v.local && id != v.link:
		return thumbnails.Result{}, library.ErrNotFound
	case id == v.link:
		return thumbnails.Result{}, thumbnails.ErrNotLocal
	case v.off.Load():
		return thumbnails.Result{}, thumbnails.ErrTurnedOff
	}
	return thumbnails.Result{State: thumbnails.StateWaiting}, nil
}

// noTitles finds no title: the Versions page's routes need one.
type noTitles struct{ Titles }

// The Versions page tells where the thumbnails of a local folder's file
// stand, and none for a link; Make again queues a local file, and refuses
// a link, any while the settings turn the images off, and a version not
// known.
func TestVersionsPageMakesLocalThumbnailsAgain(t *testing.T) {
	t.Parallel()
	versions := &thumbnailVersions{title: accounts.ID{1}, local: accounts.ID{2}, link: accounts.ID{3}, at: time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.Versions, o.Titles = versions, noTitles{} })
	admin := api.signedIn("root", true)
	var page versionsJSON
	if status := admin.raw(http.MethodGet, "/titles/"+versions.title.String()+"/versions", "", &page); status != http.StatusOK || len(page.Sources) != 1 {
		t.Fatalf("versions: %d %+v", status, page)
	}
	streams := page.Sources[0].Streams
	if got := streams[0].Thumbnails; got == nil || got.State != "cannot" || got.Reason != "no_index" || got.Count != 0 || got.At == nil || !got.At.Equal(versions.at) {
		t.Errorf("a local file's thumbnails: %+v", got)
	}
	if streams[1].Thumbnails != nil {
		t.Errorf("a link's thumbnails: %+v", streams[1].Thumbnails)
	}

	again := func(version accounts.ID) (int, map[string]any) {
		t.Helper()
		status, body, _ := admin.call(http.MethodPost, "/titles/"+versions.title.String()+"/versions/"+version.String()+"/thumbnails", nil)
		return status, body
	}
	if status, body := again(versions.local); status != http.StatusAccepted || body["state"] != "waiting" || body["reason"] != "" ||
		body["count"] != float64(0) || body["at"] != nil {
		t.Errorf("make again: %d %v", status, body)
	}
	if status, body := again(versions.link); status != http.StatusBadRequest || body["error"] != "not_local" {
		t.Errorf("a link: %d %v", status, body)
	}
	if status, _ := again(accounts.ID{9}); status != http.StatusNotFound {
		t.Errorf("a version not known: %d", status)
	}
	versions.off.Store(true)
	if status, body := again(versions.local); status != http.StatusConflict || body["error"] != "thumbnails_off" {
		t.Errorf("turned off: %d %v", status, body)
	}
	if status, _, _ := api.signedIn("member", false).call(http.MethodPost, "/titles/"+versions.title.String()+"/versions/"+versions.local.String()+"/thumbnails", nil); status != http.StatusForbidden {
		t.Errorf("a member: %d", status)
	}
}

// Health counts the files of local folders by the last result of their
// images, those of links and torrents aside.
func TestHealthCountsLocalThumbnails(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		images := thumbnails.New(thumbnails.Options{DB: deps.pool, Settings: o.Accounts.Settings, Logger: slog.New(slog.DiscardHandler)})
		t.Cleanup(images.Close)
		o.Health = HealthSources{Thumbnails: images}
		for i, row := range [][2]string{{"local", "made"}, {"local", "made"}, {"local", "cannot"}, {"local", "failed"}, {"link", "made"},
			{"torrent", "failed"}} {
			reason := map[string]string{"made": "", "cannot": "no_index", "failed": "error"}[row[1]]
			if _, err := deps.pool.Exec(t.Context(), "INSERT INTO thumbnail_results (version_id, item_id, kind, state, reason) VALUES ($1, $2, $3, $4, $5)",
				accounts.ID{byte(i + 1)}, accounts.ID{99}, row[0], row[1], reason); err != nil {
				t.Fatal(err)
			}
		}
	})
	var health healthJSON
	if status := api.signedIn("root", true).raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || health.Thumbnails == nil ||
		health.Thumbnails.Local != (localThumbnailHealthJSON{Made: 2, Cannot: 1, Failed: 1}) {
		t.Fatalf("health: %d %+v", status, health.Thumbnails)
	}
}
