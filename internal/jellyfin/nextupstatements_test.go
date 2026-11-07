package jellyfin

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/userdata"
)

// statementCounter counts the statements a pool sends, those of batches
// too, and those that write or lock items, and each statement's sends.
type statementCounter struct {
	statements, itemWrites atomic.Int64

	mu   sync.Mutex
	sent map[string]int
}

// writesItems matches the statements that write or lock rows of items.
var writesItems = regexp.MustCompile(`(?s)INSERT INTO items|UPDATE items|FROM items\b.*FOR UPDATE`)

func (c *statementCounter) count(sql string) {
	c.statements.Add(1)
	if writesItems.MatchString(sql) {
		c.itemWrites.Add(1)
	}
	c.mu.Lock()
	if c.sent == nil {
		c.sent = map[string]int{}
	}
	c.sent[strings.Join(strings.Fields(sql), " ")]++
	c.mu.Unlock()
}

// reset forgets the statements counted so far.
func (c *statementCounter) reset() {
	c.statements.Store(0)
	c.itemWrites.Store(0)
	c.mu.Lock()
	c.sent = nil
	c.mu.Unlock()
}

// mostSent lists the most sent statements since the last reset, at most
// count of them, each with its sends and its first 160 characters.
func (c *statementCounter) mostSent(count int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	statements := slices.Collect(maps.Keys(c.sent))
	slices.SortFunc(statements, func(a, b string) int { return cmp.Or(c.sent[b]-c.sent[a], strings.Compare(a, b)) })
	var b strings.Builder
	for _, sql := range statements[:min(count, len(statements))] {
		fmt.Fprintf(&b, "%4d  %.160s\n", c.sent[sql], sql)
	}
	return b.String()
}

func (c *statementCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	c.count(data.SQL)
	return ctx
}

func (c *statementCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *statementCounter) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return ctx
}

func (c *statementCounter) TraceBatchQuery(_ context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	c.count(data.SQL)
}

func (c *statementCounter) TraceBatchEnd(context.Context, *pgx.Conn, pgx.TraceBatchEndData) {}

// watchedSeriesAddon serves a catalog of count series, of episodes episodes
// each.
func watchedSeriesAddon(t *testing.T, count, episodes int) string {
	t.Helper()
	show := func(i int, videos bool) stremio.Meta {
		meta := stremio.Meta{ID: fmt.Sprintf("tt9%03d", i), Type: "series", Name: fmt.Sprintf("Show %d", i)}
		if videos {
			for e := range episodes {
				meta.Videos = append(meta.Videos, stremio.Video{ID: fmt.Sprintf("%s:1:%d", meta.ID, e+1), Title: fmt.Sprintf("Episode %d", e+1),
					Season: 1, Episode: stremio.Number(e + 1), Released: fmt.Sprintf("2020-01-%02dT00:00:00Z", e+1)})
			}
		}
		return meta
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch path := r.URL.EscapedPath(); {
		case path == "/manifest.json":
			_ = json.NewEncoder(w).Encode(stremio.Manifest{ID: "series", Name: "Series", Version: "1", Types: []string{"series"},
				Resources: []stremio.Resource{{Name: "catalog"}, {Name: "meta"}},
				Catalogs:  []stremio.Catalog{{Type: "series", ID: "shows", Name: "Shows"}}})
		case strings.HasPrefix(path, "/catalog/series/shows"):
			metas := make([]stremio.Meta, count)
			for i := range metas {
				metas[i] = show(i, false)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metas": metas})
		case strings.HasPrefix(path, "/meta/series/tt9"):
			i, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(path, "/meta/series/tt9"), ".json"))
			_ = json.NewEncoder(w).Encode(map[string]any{"meta": show(i, true)})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// Next Up reads the episodes of every series the user watches, and
// records them: once they are stored as it records them, it writes and
// locks no row, and its statements stay within one look-up of all the
// series and one of all their episodes' versions, however many series
// and episodes.
func TestNextUpStatementsStayWithinABudget(t *testing.T) {
	t.Parallel()
	const series = 6
	s := newTestServer(t, 10)
	s.user("member", nil)
	addon, err := s.addons.Install(t.Context(), addons.Shared(), watchedSeriesAddon(t, series, 12), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.addons.SetLibraries(t.Context(), addons.Shared(), []addons.LibraryChoice{
		{AddonID: addon.ID, CatalogType: "series", CatalogID: "shows"}}); err != nil {
		t.Fatal(err)
	}
	token := s.signIn("member", "tv")
	views := s.viewIDs(t, token)
	var shows QueryResult
	s.get(t, "/Items?ParentId="+views["Shows"], token, &shows)
	if len(shows.Items) != series {
		t.Fatalf("%d series", len(shows.Items))
	}
	for _, show := range shows.Items {
		var episodes QueryResult
		s.get(t, "/Shows/"+show.Id+"/Episodes", token, &episodes)
		if status, body := s.call(http.MethodPost, "/UserPlayedItems/"+episodes.Items[0].Id, app("tv", token), nil); status != http.StatusOK {
			t.Fatalf("played: %d %s", status, body)
		}
	}

	// A restart, whose library and users' data count their statements.
	counter := &statementCounter{}
	config := s.pool.Config()
	config.ConnConfig.Tracer = counter
	counted, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(counted.Close)
	options := s.handler.Options
	options.Library = library.New(counted, s.addons, options.Stremio, options.Logger, s.store.Settings)
	options.Library.SetFollowUps()
	options.UserData = userdata.New(counted)
	server := httptest.NewServer(New(options))
	t.Cleanup(server.Close)
	restarted := s
	restarted.url = server.URL
	nextUp := func() {
		t.Helper()
		counter.reset()
		var page QueryResult
		if status := restarted.get(t, "/Shows/NextUp", token, &page); status != http.StatusOK || len(page.Items) != series {
			t.Fatalf("Next Up: %d, %d items", status, len(page.Items))
		}
	}
	nextUp()
	nextUp()
	t.Logf("Next Up: %d statements, %d writing or locking items", counter.statements.Load(), counter.itemWrites.Load())
	if n := counter.statements.Load(); n > 10 {
		t.Errorf("%d statements for %d series", n, series)
	}
	if n := counter.itemWrites.Load(); n != 0 {
		t.Errorf("%d statements wrote or locked items", n)
	}
}
