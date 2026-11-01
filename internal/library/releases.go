package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/stremio"
	"github.com/moodiness/polyfin/internal/tmdb"
)

// Movies wait for their digital release.
//
// Addons' catalogs list movies still in cinemas, which no stream addon
// has a version of yet. With Settings.WaitDigitalRelease on and the
// server's TMDB key saved, a movie listed by an addon (not by an IPTV
// source or a local folder, which hold its file) is hidden until it is
// out at home: its first digital or physical release in any country, as
// TMDB's release dates tell, else releaseGrace after it was first shown in
// cinemas, else anywhere. A movie TMDB does not know, or knows no date
// of, and one without an IMDb or TMDB identifier, stays listed: nothing
// tells it is not out. A movie whose listing tells it came out more than
// releaseRecent ago is out, and TMDB is not asked about it.
//
// TMDB is asked in the background, a few movies at a time, by their TMDB
// identifier, else their IMDb one, and its answers are kept in the
// database for every user (see lookUpRelease); a listing waits for them
// until the request's deadline, as it does for ratings (see visible), and
// leaves out the movies still unknown then. The day a movie is out stays
// once passed, so that a movie listed is never hidden again.
//
// A movie held back is not listed, as a title the user hid is not, by
// libraries, home rows, searches, similar titles or person pages. Smart
// lists whose rules look for titles not out yet (see
// smartlists.Rule.Upcoming) list it, with the day it is out as its
// premiere date and in its overview. It still opens by its identifier,
// with that day in its overview, but has no version, and does not play
// (see Upcoming).

const (
	// releaseGrace is how long after it was first shown a movie TMDB
	// knows no home release of counts as out.
	releaseGrace = 90 * 24 * time.Hour
	// releaseRecent is how long after the day its listing gives a movie
	// may still wait.
	releaseRecent = 365 * 24 * time.Hour
	// releasesUpcoming is how long a movie not out yet is believed so
	// before TMDB is asked again, as its dates change; releasesMissing,
	// how long one TMDB does not know, or knows no date of.
	releasesUpcoming = 3 * 24 * time.Hour
	releasesMissing  = 30 * 24 * time.Hour
	// releaseLookupTime bounds a lookup of a movie's release dates.
	releaseLookupTime = time.Minute
)

// releaseKey identifies a movie as TMDB is asked about its release dates,
// and as they are kept: by the identifier it is asked by.
type releaseKey struct {
	provider string
	id       string
}

// releaseKeys returns the keys of a movie by its identifiers, under
// Jellyfin's names: its TMDB identifier, then its IMDb one. It is asked
// about by the first; what is kept under any of them tells of it.
func releaseKeys(ids map[string]string) []releaseKey {
	var keys []releaseKey
	if id := ids["Tmdb"]; id != "" {
		keys = append(keys, releaseKey{"tmdb", id})
	}
	if id := ids["Imdb"]; id != "" {
		keys = append(keys, releaseKey{"imdb", id})
	}
	return keys
}

// release is what is kept of a movie's release: whether TMDB knew it,
// the day it is out, zero when TMDB knows no date, and when TMDB was
// asked.
type release struct {
	found     bool
	available time.Time
	asked     time.Time
}

// waiting reports whether the movie is not out on the day of now.
func (r release) waiting(now time.Time) bool {
	return !r.available.IsZero() && r.available.After(now)
}

// due reports whether TMDB is asked again about the movie: never once it
// is out, else after releasesUpcoming while it is to come, releasesMissing
// for one TMDB does not know or knows no date of.
func (r release) due(now time.Time) bool {
	switch {
	case !r.available.IsZero() && !r.waiting(now):
		return false
	case r.waiting(now):
		return now.Sub(r.asked) >= releasesUpcoming
	}
	return now.Sub(r.asked) >= releasesMissing
}

// availableDay is the day a movie counts as out: its first home release,
// else releaseGrace after it was first shown in cinemas, else anywhere;
// zero when TMDB knows no date of it.
func availableDay(releases tmdb.Releases) time.Time {
	switch {
	case !releases.Home.IsZero():
		return releases.Home
	case !releases.Theatrical.IsZero():
		return releases.Theatrical.Add(releaseGrace)
	case !releases.First.IsZero():
		return releases.First.Add(releaseGrace)
	}
	return time.Time{}
}

// recentMovie reports whether a movie's listing leaves it possibly not
// out at home: it tells a day less than releaseRecent ago, or later, or a
// year no earlier than last year's, or nothing.
func recentMovie(meta stremio.Meta, now time.Time) bool {
	if day := exactDay(meta.Released); day != nil {
		return day.After(now.Add(-releaseRecent))
	}
	if year := metaYear(meta); year > 0 {
		return year >= now.Year()-1
	}
	return true
}

// waits reports whether the wait for the digital release may hold back
// r: it is on, and r is a movie not listed by an IPTV source or a local
// folder, whose listing leaves it possibly not out.
func (s *Service) waits(v view, r record) bool {
	if !v.waitRelease || r.Kind != KindMovie || r.Meta == nil {
		return false
	}
	if r.Addon != nil {
		if entry, ok := v.addon(*r.Addon); ok && own(entry.addon) {
			return false
		}
	}
	return recentMovie(*r.Meta, s.now())
}

// keptReleases returns what is kept of the movies of keys.
func (s *Service) keptReleases(ctx context.Context, keys []releaseKey) (map[releaseKey]release, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	providers, ids := make([]string, len(keys)), make([]string, len(keys))
	for i, key := range keys {
		providers[i], ids[i] = key.provider, key.id
	}
	rows, err := s.db.Query(ctx, `SELECT r.provider, r.id, r.found, r.available, r.asked_at
		FROM movie_releases r JOIN unnest($1::text[], $2::text[]) AS asked (provider, id) USING (provider, id)`, providers, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	kept := make(map[releaseKey]release, len(keys))
	for rows.Next() {
		var key releaseKey
		var r release
		var available *time.Time
		if err := rows.Scan(&key.provider, &key.id, &r.found, &available, &r.asked); err != nil {
			return nil, err
		}
		if available != nil {
			r.available = available.UTC()
		}
		kept[key] = r
	}
	return kept, rows.Err()
}

// keptRelease returns what is kept of the movie of keys, under the first
// of them kept.
func keptRelease(kept map[releaseKey]release, keys []releaseKey) (release, bool) {
	for _, key := range keys {
		if r, ok := kept[key]; ok {
			return r, true
		}
	}
	return release{}, false
}

// releasesLater asks TMDB in the background about the movies of keys, at
// most limit of them, each once at a time, and returns what each lookup
// started answers on; the requests to TMDB are bounded across the server
// (see tmdb.Client.Hold).
func (s *Service) releasesLater(ctx context.Context, keys []releaseKey, limit int) []<-chan error {
	if keptOnly(ctx) {
		return nil
	}
	detached := context.WithoutCancel(ctx)
	var waits []<-chan error
	for _, key := range keys[:min(len(keys), limit)] {
		answered := make(chan error, 1)
		results := s.flight.DoChan(fmt.Sprintf("release %s %s", key.provider, key.id), func() (any, error) {
			lookup, cancel := context.WithTimeout(detached, releaseLookupTime)
			defer cancel()
			return nil, s.lookUpRelease(lookup, key)
		})
		go func() { answered <- (<-results).Err }()
		waits = append(waits, answered)
	}
	return waits
}

// lookUpRelease asks TMDB for a movie's release dates and keeps the day
// it is out, a day already passed staying. A movie TMDB does not know is
// kept as such; a lookup that fails keeps nothing, and the movie is asked
// about again when next listed.
func (s *Service) lookUpRelease(ctx context.Context, key releaseKey) error {
	id := key.id
	var err error
	if key.provider == "imdb" {
		id, err = s.tmdb.FindByIMDb(ctx, tmdb.Movie, key.id)
	}
	var releases tmdb.Releases
	if err == nil {
		releases, err = s.tmdb.ReleaseDates(ctx, id)
	}
	found := err == nil
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return err
	}
	var available *time.Time
	if day := availableDay(releases); !day.IsZero() {
		available = &day
	}
	now := s.now()
	_, err = s.db.Exec(ctx, `INSERT INTO movie_releases (provider, id, found, available, asked_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (provider, id) DO UPDATE SET found = excluded.found, asked_at = excluded.asked_at,
			available = CASE WHEN movie_releases.available <= $6::date THEN movie_releases.available ELSE excluded.available END`,
		key.provider, key.id, found, available, now, now.UTC().Format(time.DateOnly))
	return err
}

// held tells which of records the wait for the digital release holds back
// for the user of v: the movies it may hold (see waits) not out yet, by
// their item, with the day they are out, zero while not known. TMDB is
// asked about those not known, or known for too long, within the
// request's lookups, and those not known are waited for until the
// request's deadline; pending reports some still unknown then.
func (s *Service) held(ctx context.Context, v view, records []record) (held map[accounts.ID]time.Time, pending bool, err error) {
	var waiting []record
	var keys []releaseKey
	for _, r := range records {
		if s.waits(v, r) {
			if k := releaseKeys(providerIDs(*r.Meta)); len(k) > 0 {
				waiting, keys = append(waiting, r), append(keys, k...)
			}
		}
	}
	if len(waiting) == 0 {
		return nil, false, nil
	}
	kept, err := s.keptReleases(ctx, keys)
	if err != nil {
		return nil, false, err
	}
	now := s.now()
	var unknown, stale []releaseKey
	for _, r := range waiting {
		k := releaseKeys(providerIDs(*r.Meta))
		switch known, ok := keptRelease(kept, k); {
		case !ok:
			unknown = append(unknown, k[0])
		case known.due(now):
			stale = append(stale, k[0])
		}
	}
	// The lookups a request starts are bounded with those of ratings.
	started := 0
	for started < len(unknown) && v.lookups.Add(1) <= maxRatingLookups {
		started++
	}
	waits := s.releasesLater(ctx, unknown, started)
	s.releasesLater(ctx, stale, smartLookups)
	if len(waits) > 0 && !v.deadline.IsZero() {
		deadline := time.NewTimer(time.Until(v.deadline))
		defer deadline.Stop()
	wait:
		for _, answered := range waits {
			select {
			case <-answered:
			case <-deadline.C:
				break wait
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		if kept, err = s.keptReleases(ctx, keys); err != nil {
			return nil, false, err
		}
	}
	held = map[accounts.ID]time.Time{}
	for _, r := range waiting {
		known, ok := keptRelease(kept, releaseKeys(providerIDs(*r.Meta)))
		switch {
		case !ok:
			held[r.ID], pending = time.Time{}, true
		case known.found && known.waiting(now):
			held[r.ID] = known.available
		}
	}
	return held, pending, nil
}

// released leaves out of records those the wait for the digital release
// holds back for the user of v (see held), but in a view that lists them
// (see view.upcoming). pending reports some left out only because their
// release is not known yet. Should what is kept not be read, none is left
// out.
func (s *Service) released(ctx context.Context, v view, records []record) (kept []record, pending bool) {
	if !v.waitRelease || v.upcoming || len(records) == 0 {
		return records, false
	}
	held, pending, err := s.held(ctx, v, records)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("The release dates of movies could not be read", "error", err)
		}
		return records, false
	}
	if len(held) == 0 {
		return records, pending
	}
	return slices.DeleteFunc(slices.Clone(records), func(r record) bool {
		_, ok := held[r.ID]
		return ok
	}), pending
}

// releasedMetas leaves out of a catalog page of src the movies the wait
// for the digital release holds back for the user of v (see released).
func (s *Service) releasedMetas(ctx context.Context, v view, src source, metas []stremio.Meta) ([]stremio.Meta, bool) {
	if !v.waitRelease || v.upcoming || own(src.addon.addon) || len(metas) == 0 {
		return metas, false
	}
	var movies []record
	for _, meta := range metas {
		if _, r, err := titleItem(src.addon, src.catalog, meta, accounts.ID{}); err == nil && r.Kind == KindMovie {
			movies = append(movies, r)
		}
	}
	kept, pending := s.released(ctx, v, movies)
	if len(kept) == len(movies) {
		return metas, pending
	}
	out := make(map[string]bool, len(movies)-len(kept))
	for _, r := range movies {
		out[r.Meta.ID] = true
	}
	for _, r := range kept {
		delete(out, r.Meta.ID)
	}
	return slices.DeleteFunc(slices.Clone(metas), func(meta stremio.Meta) bool { return out[meta.ID] }), pending
}

// Upcoming reports whether the movie identified by id waits for its
// digital release for user, and the day it is out, zero when TMDB could
// not tell yet: it has no version then, and does not play.
func (s *Service) Upcoming(ctx context.Context, user accounts.User, id accounts.ID) (time.Time, bool, error) {
	v, err := s.view(ctx, user)
	if err != nil {
		return time.Time{}, false, err
	}
	return s.upcoming(ctx, v, id)
}

// upcoming is Upcoming in v.
func (s *Service) upcoming(ctx context.Context, v view, id accounts.ID) (time.Time, bool, error) {
	if !v.waitRelease {
		return time.Time{}, false, nil
	}
	r, err := s.load(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	held, _, err := s.held(ctx, v, []record{r})
	if err != nil {
		return time.Time{}, false, err
	}
	day, ok := held[r.ID]
	return day, ok, nil
}

// homeReleaseNote puts the day a movie is out at home first in its
// overview, in the user's words.
func homeReleaseNote(item *Item, w words, day time.Time) {
	note := fmt.Sprintf(w.homeRelease, day.Format(time.DateOnly))
	if item.Overview != "" {
		note += "\n\n" + item.Overview
	}
	item.Overview = note
}
