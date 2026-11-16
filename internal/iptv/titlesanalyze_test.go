package iptv

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/moodiness/polyfin/internal/addons"
)

// titlesCounted is how many titles the planner counts, and how many are
// stored.
func (e env) titlesCounted() (counted, stored float64) {
	e.t.Helper()
	if err := e.service.db.QueryRow(e.t.Context(), `SELECT (SELECT reltuples FROM pg_class WHERE oid = 'iptv_titles'::regclass),
		(SELECT count(*) FROM iptv_titles)`).Scan(&counted, &stored); err != nil {
		e.t.Fatal(err)
	}
	return counted, stored
}

// Once a catalogue download is committed, the planner counts its titles:
// until autovacuum's first visit it would count none, and plan the lookup
// of a title on the index of its category. So after an add, a change of
// options and a refresh, and the catalogue served is the one stored.
func TestCatalogueDownloadsRefreshThePlannersStatistics(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx, shared := t.Context(), addons.Shared()
	_, account := syntheticVOD(t, 12, 4)
	if counted, _ := e.titlesCounted(); counted > 0 {
		t.Fatalf("%v titles counted before the first download", counted)
	}
	on := true
	addon := must(e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &on, Series: &on}}, false))
	if counted, stored := e.titlesCounted(); stored != 16 || counted != stored {
		t.Errorf("after the add, the planner counts %v titles, %v stored", counted, stored)
	}
	if page := must(e.service.Catalog(ctx, addon.ID, "movie", "movies", 0, "", "")); len(page) != 12 {
		t.Errorf("movies served after the add: %d", len(page))
	}

	off := false
	if err := e.service.Update(ctx, shared, addon.ID, Changes{Options: &OptionsPatch{Series: &off}}, false); err != nil {
		t.Fatal(err)
	}
	if counted, stored := e.titlesCounted(); stored != 12 || counted != stored {
		t.Errorf("after the series were turned off, the planner counts %v titles, %v stored", counted, stored)
	}

	// As an automatic ANALYZE while the titles were away would leave them.
	if _, err := e.service.db.Exec(ctx, "DELETE FROM iptv_titles"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, "ANALYZE iptv_titles"); err != nil {
		t.Fatal(err)
	}
	if err := e.service.Refresh(ctx, shared, addon.ID, false); err != nil {
		t.Fatal(err)
	}
	if counted, stored := e.titlesCounted(); stored != 12 || counted != stored {
		t.Errorf("after a refresh, the planner counts %v titles, %v stored", counted, stored)
	}
}

// failAnalyze fails the statistics' refresh, as a canceled statement.
type failAnalyze struct{}

func (failAnalyze) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "ANALYZE iptv_titles") {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		return canceled
	}
	return ctx
}

func (failAnalyze) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// lockedBuffer is a log written from several goroutines.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// A statistics' refresh that fails is logged once, at Debug, and the
// download stays stored and served.
func TestCatalogueDownloadsKeepAFailedStatisticsRefresh(t *testing.T) {
	t.Parallel()
	e := tracedEnv(t, failAnalyze{})
	var log lockedBuffer
	e.service.logger = slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, shared := t.Context(), addons.Shared()
	_, account := syntheticVOD(t, 12, 4)
	on := true
	addon, err := e.service.Add(ctx, shared, NewSource{Name: "Box", Account: account, Options: &OptionsPatch{Movies: &on, Series: &on}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, stored := e.titlesCounted(); stored != 16 {
		t.Errorf("%v titles stored", stored)
	}
	if page := must(e.service.Catalog(ctx, addon.ID, "series", "series", 0, "", "")); len(page) != 4 {
		t.Errorf("series served: %d", len(page))
	}
	lines := strings.Split(strings.TrimSpace(log.String()), "\n")
	var failed []string
	for _, line := range lines {
		if strings.Contains(line, "statistics could not be refreshed") {
			failed = append(failed, line)
		}
	}
	if len(failed) != 1 || !strings.Contains(failed[0], "level=DEBUG") {
		t.Errorf("failure logged as %q", failed)
	}
}
