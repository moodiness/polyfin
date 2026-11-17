package iptv

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// detailsReads counts the statements sent, and those reading details.
type detailsReads struct{ statements, details atomic.Int64 }

func (c *detailsReads) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.statements.Add(1)
	if strings.Contains(data.SQL, "iptv_details") {
		c.details.Add(1)
	}
	return ctx
}

func (c *detailsReads) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// The streams of a series' episodes come from its description kept while
// its details are fresh. A listing of episodes, which needs each one's
// stream, reads the series' title for each one, its source once in a
// request checking it once, and its details once, where it read and
// decoded them for every episode. The streams are those its stored
// details give, and follow the provider's new episodes once the series'
// details are asked again.
func TestEpisodeStreamsComeFromTheDescriptionKept(t *testing.T) {
	t.Parallel()
	reads := &detailsReads{}
	e := tracedEnv(t, reads)
	ctx := t.Context()
	server, source, ids := describedShows(t, e, 1)
	show := server.data.series[0]
	key := fmt.Sprint(show.ID)
	// Opening the series stores its details, as a play does.
	opened := must(e.service.Meta(Opening(ctx), source, ids[0]))
	if len(opened.Videos) != show.Seasons*show.Episodes {
		t.Fatalf("%d episodes opened", len(opened.Videos))
	}
	// What the stored details give each episode, read as streams were
	// before the descriptions kept them.
	stored := func(id string) []stremio.Stream {
		t.Helper()
		title := must(e.service.title(ctx, source, typeSeries, key))
		d := must(e.service.details(ctx, source, title, false))
		episode := id[strings.LastIndex(id, ":")+1:]
		i := slices.IndexFunc(d.Episodes, func(e episodeDetails) bool { return e.Key == episode })
		if i < 0 {
			t.Fatalf("no details of episode %s", id)
		}
		return []stremio.Stream{title.vodStream(d.Episodes[i].Quality,
			accountOf(title.kind, title.manifestAddress).vodURL("series", episode, d.Episodes[i].Extension), nil)}
	}
	want := make([][]stremio.Stream, len(opened.Videos))
	for i, video := range opened.Videos {
		want[i] = stored(video.ID)
	}
	listing := func(when string, statements, details int64) {
		t.Helper()
		once := CheckOnce(ctx)
		sent, read := reads.statements.Load(), reads.details.Load()
		for i, video := range opened.Videos {
			if got := must(e.service.Streams(once, source, video.ID)); !reflect.DeepEqual(got, want[i]) {
				t.Errorf("%s: the streams of %s: %+v, want %+v", when, video.ID, got, want[i])
			}
		}
		sent, read = reads.statements.Load()-sent, reads.details.Load()-read
		if sent != statements || read != details {
			t.Errorf("%s: %d statements, %d reading details, for %d episodes; want %d and %d", when, sent, read, len(opened.Videos),
				statements, details)
		}
	}
	// The titles, one check of the source, and the description read once.
	episodes := int64(len(opened.Videos))
	listing("first listed", episodes+2, 1)
	listing("listed again", episodes+1, 0)

	// The provider adds an episode: the series, changed, is asked again
	// for its details by the first stream that needs them.
	server.mu.Lock()
	server.data.series[0].Last, server.data.series[0].Modified = show.Episodes+1, show.Modified+1
	server.mu.Unlock()
	if err := e.service.Refresh(ctx, addons.Shared(), source, false); err != nil {
		t.Fatal(err)
	}
	asked := server.count("get_series_info")
	added := episodeID(source, key, fmt.Sprint(show.ID*1_000_000+show.Seasons*1_000+show.Episodes+1))
	got := must(e.service.Streams(ctx, source, added))
	if server.count("get_series_info") != asked+1 || !reflect.DeepEqual(got, stored(added)) {
		t.Errorf("the new episode's streams: %+v, details asked %d more times", got, server.count("get_series_info")-asked)
	}
}
