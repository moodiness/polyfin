package library

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// Once a guide download is current, the planner counts its programmes:
// a statistic taken while the download was written would count none of
// them, and plan every guide read as on an empty table.
func TestGuideDownloadsRefreshThePlannersStatistics(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	now := time.Now().UTC().Truncate(time.Minute)
	e.service.now = func() time.Time { return now }
	key := e.tvCatalog(addons.Shared(), stremio.Meta{ID: "tv:one", Type: "tv", Name: "One"})
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv><channel id="one"><display-name>One</display-name></channel>`)
	for i := range 300 {
		body.WriteString(programme("one", now.Add(time.Duration(i)*10*time.Minute), now.Add(time.Duration(i+1)*10*time.Minute), fmt.Sprint("P", i)))
	}
	body.WriteString(`</tv>`)
	guide := newGuideServer(t, body.String())
	if _, err := e.addons.SetGuide(t.Context(), addons.Shared(), key, guide.url); err != nil {
		t.Fatal(err)
	}
	counted := func() (float64, float64) {
		t.Helper()
		var programmes, channels float64
		if err := e.service.db.QueryRow(t.Context(), `SELECT (SELECT reltuples FROM pg_class WHERE oid = 'live_guide_programmes'::regclass),
			(SELECT reltuples FROM pg_class WHERE oid = 'live_guide_channels'::regclass)`).Scan(&programmes, &channels); err != nil {
			t.Fatal(err)
		}
		return programmes, channels
	}
	// As an automatic ANALYZE during the download leaves them: none.
	if _, err := e.service.db.Exec(t.Context(), "ANALYZE live_guide_programmes, live_guide_channels"); err != nil {
		t.Fatal(err)
	}
	if programmes, _ := counted(); programmes > 0 {
		t.Fatalf("%v programmes counted before the download", programmes)
	}
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	var stored float64
	if err := e.service.db.QueryRow(t.Context(), "SELECT count(*) FROM live_guide_programmes").Scan(&stored); err != nil || stored == 0 {
		t.Fatal(stored, err)
	}
	if programmes, channels := counted(); programmes != stored || channels != 1 {
		t.Errorf("the planner counts %v programmes and %v channels, %v programmes stored", programmes, channels, stored)
	}
	// So after every download, not only the first.
	guide.set(strings.Replace(body.String(), `</tv>`, programme("one", now.Add(-2*time.Hour), now.Add(-time.Hour), "Earlier")+`</tv>`, 1), 0)
	if err := e.service.RefreshGuide(t.Context(), addons.Shared(), key); err != nil {
		t.Fatal(err)
	}
	if programmes, _ := counted(); programmes != stored+1 {
		t.Errorf("after a second download, the planner counts %v programmes, want %v", programmes, stored+1)
	}
}
