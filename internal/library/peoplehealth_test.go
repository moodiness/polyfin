package library

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/stremio"
)

// counted has the service send its statements through a pool that counts
// them.
func counted(t *testing.T, s *Service) *statements {
	t.Helper()
	counter := &statements{}
	config := s.db.Config()
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	s.db = pool
	return counter
}

// Health counts the people looked up on TMDB without reading the titles:
// with sequential scans off, the count reads items through an index, and
// filters out no row.
func TestPeopleCountsReadNoTitles(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()
	if _, err := e.service.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT gen_random_uuid(), 'movie|' || i, 'movie', jsonb_build_object('meta', jsonb_build_object('name', 'Movie ' || i))
		FROM generate_series(1, 3000) AS i`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, `INSERT INTO items (id, key, kind, data)
		SELECT gen_random_uuid(), 'person|' || i, 'person', jsonb_build_object('person', jsonb_build_object('name', 'Person ' || i))
			|| CASE WHEN i % 100 = 0 THEN jsonb_build_object('tmdb', jsonb_build_object('id', i, 'foundAt', now())) ELSE '{}'::jsonb END
		FROM generate_series(1, 2000) AS i`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.service.db.Exec(ctx, "ANALYZE items"); err != nil {
		t.Fatal(err)
	}
	var plan []byte
	err := pgx.BeginFunc(ctx, e.service.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+peopleCountsQuery).Scan(&plan)
	})
	if err != nil {
		t.Fatal(err)
	}
	var explained []struct{ Plan planNode }
	if err := json.Unmarshal(plan, &explained); err != nil {
		t.Fatal(err)
	}
	read := 0
	var walk func(planNode)
	walk = func(n planNode) {
		if n.Relation == "items" {
			read++
			if n.NodeType == "Seq Scan" || n.Removed > 0 {
				t.Errorf("items read by %s, %d rows filtered out: %s", n.NodeType, n.Removed, plan)
			}
		}
		for _, child := range n.Plans {
			walk(child)
		}
	}
	walk(explained[0].Plan)
	if read == 0 {
		t.Errorf("items not read: %s", plan)
	}
	status, err := e.service.PeopleStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Found != 20 {
		t.Errorf("found %d, want 20", status.Found)
	}
}

// Health's poll does not count the people again: their counts are kept,
// and counted again once a lookup, Refresh metadata or a merge wrote to
// them.
func TestPeopleCountsFollowTheLookups(t *testing.T) {
	t.Parallel()
	e := newPeopleEnv(t, "k3y", []stremio.CastMember{{Name: "Ivy Actor"}, {Name: "Jon Actor"}, {Name: "Kai Actor"}})
	e.tmdb.searches["Ivy Actor"] = []map[string]any{{"id": 41, "name": "Ivy Actor", "popularity": 3, "profile_path": "/ivy.jpg"}}
	e.tmdb.addPerson("41", "Ivy Actor", "/ivy.jpg", "Ivy's life.", nil)
	e.tmdb.failing["Kai Actor"] = true
	queries := counted(t, e.service)
	counts := func(when string, found, notFound, failing int) {
		t.Helper()
		status, err := e.service.PeopleStatus(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if status.Found != found || status.NotFound != notFound || status.Failing != failing {
			t.Errorf("%s: found %d, not found %d, failing %d; want %d, %d, %d", when, status.Found, status.NotFound, status.Failing,
				found, notFound, failing)
		}
	}
	polled := func(when string) {
		t.Helper()
		queries.take()
		status, err := e.service.PeopleStatus(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, sent := queries.take(); sent != 0 {
			t.Errorf("%s: a poll sent %d statements (%+v)", when, sent, status)
		}
	}

	counts("before any lookup", 0, 0, 0)
	polled("before any lookup")
	e.cast(e.admin)
	counts("after the lookups", 1, 1, 1)
	polled("after the lookups")

	e.mu(func() {
		delete(e.tmdb.failing, "Kai Actor")
		e.tmdb.searches["Kai Actor"] = []map[string]any{{"id": 43, "name": "Kai Actor", "popularity": 3, "profile_path": "/kai.jpg"}}
	})
	e.tmdb.addPerson("43", "Kai Actor", "/kai.jpg", "Kai's life.", nil)
	if err := e.service.Refresh(t.Context(), e.admin, PersonID("Kai Actor")); err != nil {
		t.Fatal(err)
	}
	e.service.personLookups.wg.Wait()
	counts("after Refresh metadata", 2, 1, 0)
	polled("after Refresh metadata")

	if err := e.service.saveCredits(t.Context(), e.heist, []Person{{Name: "Ivy Actor", Type: "Actor", TMDB: "41"}}, true); err != nil {
		t.Fatal(err)
	}
	var merged int
	if err := e.service.db.QueryRow(t.Context(), `SELECT count(*) FROM items WHERE kind = 'person' AND data ? 'sameAs'`).Scan(&merged); err != nil {
		t.Fatal(err)
	}
	if merged == 0 {
		t.Fatal("no person was merged")
	}
	var want [3]int
	if err := e.service.db.QueryRow(t.Context(), peopleCountsQuery).Scan(&want[0], &want[1], &want[2]); err != nil {
		t.Fatal(err)
	}
	counts("after a merge", want[0], want[1], want[2])
}

// A photo found dead is marked once an hour at most, however often apps
// ask for it meanwhile.
func TestADeadPhotoIsMarkedOnceAnHour(t *testing.T) {
	t.Parallel()
	photo := "https://images.example/lea.jpg"
	e := newPeopleEnv(t, "", []stremio.CastMember{{Name: "Lea Actor", Photo: photo}})
	e.cast(e.admin)
	lea := PersonID("Lea Actor")
	marked := func() time.Time {
		t.Helper()
		r, err := e.service.load(t.Context(), lea)
		if err != nil {
			t.Fatal(err)
		}
		return r.DeadPhotos[photo]
	}
	fail := func() {
		t.Helper()
		if err := e.service.PhotoFailed(t.Context(), lea, photo); err != nil {
			t.Fatal(err)
		}
	}

	fail()
	first := marked()
	if first.IsZero() {
		t.Fatal("the photo is not marked dead")
	}
	e.clock.advance(30 * time.Minute)
	fail()
	if got := marked(); !got.Equal(first) {
		t.Errorf("marked again after 30 minutes: %v, then %v", first, got)
	}
	e.clock.advance(31 * time.Minute)
	fail()
	if got := marked(); !got.After(first) {
		t.Errorf("not marked again after an hour: %v, then %v", first, got)
	}
}
