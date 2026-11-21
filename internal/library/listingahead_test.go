package library

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/stremio"
)

// statementLog records the statements sent on a pool.
type statementLog struct {
	mu   sync.Mutex
	sent []string
}

func (l *statementLog) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, strings.Join(strings.Fields(data.SQL), " "))
	return ctx
}

func (*statementLog) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// sending counts the statements sent that contain part.
func (l *statementLog) sending(part string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, sql := range l.sent {
		if strings.Contains(sql, part) {
			n++
		}
	}
	return n
}

// restartedLogging is e after a restart, on a pool that logs the
// statements its service sends.
func (e env) restartedLogging() (env, *statementLog) {
	e.t.Helper()
	log := &statementLog{}
	config := e.service.db.Config()
	config.ConnConfig.Tracer = log
	pool, err := pgxpool.NewWithConfig(e.t.Context(), config)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(pool.Close)
	service := New(pool, e.addons, stremio.NewClient("test"), slog.New(slog.NewTextHandler(io.Discard, nil)), e.users.Settings)
	service.followUpDelays = nil
	service.readAhead = false
	service.now = e.service.now
	e.service = service
	return e, log
}

// After a restart, a page that lists its movies' and episodes' versions
// reads their records and their series', the descriptions kept of them
// and the stream lists saved for them in one statement each, before the
// titles list their versions, which then read none of them one by one.
func TestAListingReadsItsTitlesAheadInAStatementEach(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	addon := &fakeAddon{
		manifest: stremio.Manifest{ID: "ahead", Name: "Ahead", Version: "1", Types: []string{"movie", "series"}, IDPrefixes: []string{"tt"},
			Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}, {Name: "stream"}},
			Catalogs:  []stremio.Catalog{{Type: "movie", ID: "popular", Name: "Popular"}, {Type: "series", ID: "shows", Name: "Shows"}}},
		catalogs: map[string][]stremio.Meta{},
		metas:    map[string]stremio.Meta{},
		streams:  map[string][]stremio.Stream{},
	}
	for i := range 6 {
		movie := stremio.Meta{ID: fmt.Sprintf("tt3%d", i), Type: "movie", Name: fmt.Sprintf("Movie %d", i)}
		addon.catalogs["movie/popular"] = append(addon.catalogs["movie/popular"], movie)
		addon.metas["movie/"+movie.ID] = movie
	}
	show := stremio.Meta{ID: "tt40", Type: "series", Name: "Show"}
	addon.catalogs["series/shows"] = []stremio.Meta{show}
	for episode := 1; episode <= 3; episode++ {
		show.Videos = append(show.Videos, stremio.Video{ID: fmt.Sprintf("tt40:1:%d", episode), Title: fmt.Sprintf("Episode %d", episode), Season: 1,
			Episode: stremio.Number(episode)})
	}
	addon.metas["series/tt40"] = show
	e.install(addons.Shared(), addon)
	ctx := t.Context()
	movies, err := e.service.Children(ctx, e.member, e.library(e.member, "Popular").ID, 0, 100, "")
	if err != nil || len(movies.Items) != 6 {
		t.Fatalf("movies: %v %v", movies.Items, err)
	}
	shows, err := e.service.Children(ctx, e.member, e.library(e.member, "Shows").ID, 0, 100, "")
	if err != nil || len(shows.Items) != 1 {
		t.Fatalf("shows: %v %v", shows.Items, err)
	}
	episodes, err := e.service.Episodes(ctx, e.member, shows.Items[0].ID, nil)
	if err != nil || len(episodes) != 3 {
		t.Fatalf("episodes: %v %v", episodes, err)
	}
	// Half the movies were opened: their descriptions are kept.
	for _, movie := range movies.Items[:3] {
		if _, err := e.service.Item(ctx, e.member, movie.ID); err != nil {
			t.Fatal(err)
		}
	}
	// Every title but the last movie has a stream list saved.
	items := append(movies.Items, episodes...)
	list, err := e.addons.Addons(ctx, addons.Shared())
	if err != nil || len(list) != 1 {
		t.Fatalf("addons: %v %v", list, err)
	}
	for _, item := range items {
		if item.ID == movies.Items[5].ID {
			continue
		}
		contentType := "movie"
		if item.Kind == KindEpisode {
			contentType = "series"
		}
		if _, err := e.service.db.Exec(ctx, `INSERT INTO stream_lists (addon_id, content_type, stremio_id, streams, answer, fetched_at)
			VALUES ($1, $2, $3, $4, 1, now())`, list[0].ID, contentType, item.StremioID,
			fmt.Sprintf(`{"streams": [{"name": "Source", "url": "https://streams.example/%s"}]}`, item.StremioID)); err != nil {
			t.Fatal(err)
		}
	}

	restarted, log := e.restartedLogging()
	s := restarted.service
	ahead := s.ListingAhead(PerRequest(ctx), e.member, items)
	for _, item := range items {
		versions, complete := s.CachedVersions(ahead, e.member, item.ID)
		want := 1
		if item.ID == movies.Items[5].ID {
			want = 0
		}
		if !complete && want == 1 || len(versions) != want {
			t.Errorf("%s: %d versions, complete %v", item.Name, len(versions), complete)
		}
	}
	for i, movie := range movies.Items {
		if _, kept := s.metas.Get(metaKey{list[0].ID, "movie", movie.StremioID}); kept != (i < 3) {
			t.Errorf("%s: description kept in memory %v", movie.Name, kept)
		}
	}
	for _, read := range []struct {
		what, part string
		want       int
	}{
		{"records read ahead", "SELECT id, key, data FROM items WHERE id = ANY", 1},
		{"records read one by one", "SELECT key, data FROM items WHERE id = $1", 0},
		{"descriptions read ahead", "JOIN metas", 1},
		{"descriptions read one by one", "FROM metas", 0},
		{"stream lists read ahead", "JOIN stream_lists", 1},
		{"stream lists read one by one", "FROM stream_lists", 0},
	} {
		if got := log.sending(read.part); got != read.want {
			t.Errorf("%s: %d statements, want %d", read.what, got, read.want)
		}
	}
}
