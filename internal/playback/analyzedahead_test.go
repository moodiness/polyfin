package playback

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// analysisReads counts the reads of media_analyses sent on a pool.
type analysisReads struct{ n atomic.Int32 }

func (r *analysisReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.HasPrefix(data.SQL, "SELECT") && strings.Contains(data.SQL, "FROM media_analyses") {
		r.n.Add(1)
	}
	return ctx
}

func (*analysisReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// A listing reads the analyses of the versions it shows in one statement,
// none it keeps in memory, and does not look again for those the database
// has none of. Outside the listing, a version is looked for again: an
// analysis saved since, as by another Polyfin on the same database, is
// found at once.
func TestAListingReadsItsVersionsAnalysesAtOnce(t *testing.T) {
	t.Parallel()
	reads := &analysisReads{}
	config := migrated.New(t).Config()
	config.ConnConfig.Tracer = reads
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := t.Context()
	analyzed := library.Version{ID: accounts.ID{7}, Name: "Analyzed"}
	unanalyzed := library.Version{ID: accounts.ID{8}, Name: "Unanalyzed"}
	other := newServiceOn(t, pool, &fakeSource{}, "ffprobe-not-installed", nil)
	other.keepAnalysis(ctx, analyzed, media.Analysis{Format: "matroska"}, time.Now())

	s := newServiceOn(t, pool, &fakeSource{}, "ffprobe-not-installed", nil)
	reads.n.Store(0)
	listing := s.AnalyzedAhead(ctx, []accounts.ID{analyzed.ID, unanalyzed.ID})
	for range 3 {
		if analysis, ok := s.Analyzed(listing, analyzed.ID); !ok || analysis.Format != "matroska" {
			t.Errorf("the analyzed version: %+v, %v", analysis, ok)
		}
		if analysis, ok := s.Analyzed(listing, unanalyzed.ID); ok {
			t.Errorf("the unanalyzed version: %+v", analysis)
		}
	}
	// A listing of a version kept in memory reads nothing.
	s.AnalyzedAhead(ctx, []accounts.ID{analyzed.ID})
	if n := reads.n.Load(); n != 1 {
		t.Errorf("%d reads of the analyses, want 1", n)
	}

	other.keepAnalysis(ctx, unanalyzed, media.Analysis{Format: "mp4"}, time.Now())
	if analysis, ok := s.Analyzed(ctx, unanalyzed.ID); !ok || analysis.Format != "mp4" {
		t.Errorf("once another analyzed it: %+v, %v", analysis, ok)
	}
}
