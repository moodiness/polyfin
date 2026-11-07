package importlinks

import (
	"bytes"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// Link links the entry of key that user's list of source holds to the
// title input names (see Parse), by the user by: themselves, or an
// administrator. The title must be of the entry's kind: a movie for a
// movie, a series for a series or an episode. An IMDb identifier always
// names a title, described or not by the server's metadata addons; a TMDB
// one must name one they describe, or one Polyfin finds by it (see
// library.Service.LookUpIDs and Resolve). The link replaces the one the
// entry had, and applies at once what was kept of the entry, and of the
// other episodes of its series, as the import would have: played marks,
// resume points, the watchlist and the rating (see Apply). It fails with
// ErrUnknownEntry, ErrInvalidTarget, ErrKindMismatch or ErrTargetNotFound.
func (s *Store) Link(ctx context.Context, by, user accounts.ID, source, key, input string) (Link, error) {
	entries, err := s.entries(ctx, user, source, key)
	if err != nil {
		return Link{}, err
	}
	if len(entries) == 0 {
		return Link{}, ErrUnknownEntry
	}
	target, err := Parse(input)
	if err != nil {
		return Link{}, err
	}
	kind := KindSeries
	if entries[0].Kind == KindMovie {
		kind = KindMovie
	}
	if target.Kind != "" && target.Kind != kind {
		return Link{}, ErrKindMismatch
	}
	link, err := s.find(ctx, kind, target)
	if err != nil {
		return Link{}, err
	}
	var imdb, year, tmdb any
	if link.IMDb != "" {
		imdb = link.IMDb
	}
	if link.TMDB > 0 {
		tmdb = link.TMDB
	}
	if link.Year > 0 {
		year = link.Year
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO import_title_links (user_id, source, entry_key, kind, imdb_id, tmdb_id, name, year, linked_by,
			linked_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, source, entry_key) DO UPDATE SET kind = excluded.kind, imdb_id = excluded.imdb_id, tmdb_id = excluded.tmdb_id,
			name = excluded.name, year = excluded.year, linked_by = excluded.linked_by, linked_at = excluded.linked_at`,
		user, source, key, link.Kind, imdb, tmdb, link.Name, year, by, s.now().UTC()); err != nil {
		return Link{}, err
	}
	if err := s.Apply(ctx, user, entries, link); err != nil {
		return Link{}, err
	}
	linked, err := s.Linked(ctx, user, source)
	if err != nil {
		return Link{}, err
	}
	s.logger.Info("A title an import did not find was linked by hand", "user_id", user.String(), "source", source)
	return linked[key], nil
}

// Unlink removes the link of the entry of key in user's list of source:
// later imports no longer apply it. What it applied stays. It fails with
// ErrUnknownEntry when the entry has no link.
func (s *Store) Unlink(ctx context.Context, user accounts.ID, source, key string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM import_title_links WHERE user_id = $1 AND source = $2 AND entry_key = $3`, user, source, key)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrUnknownEntry
	}
	return nil
}

// entries returns what user's list of source kept of the entry of key: the
// title, or its series' episodes.
func (s *Store) entries(ctx context.Context, user accounts.ID, source, key string) ([]Entry, error) {
	rows, err := s.db.Query(ctx, `SELECT `+entryColumns+` FROM import_unfound u WHERE user_id = $1 AND source = $2 AND entry_key = $3
		ORDER BY season, episode`, user, source, key)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) { return scanEntry(row) })
}

// find finds the title of kind target names, and describes it in a link:
// an IMDb identifier is looked up as it is in the server's metadata
// addons, and is of the other kind when they describe it as one; a TMDB
// identifier is looked up as Identify looks it up, else resolved.
func (s *Store) find(ctx context.Context, kind string, target Target) (Link, error) {
	link := Link{Kind: kind, IMDb: target.IMDb, TMDB: target.TMDB}
	asked, other := library.KindMovie, library.KindSeries
	if kind == KindSeries {
		asked, other = other, asked
	}
	var found *library.RemoteResult
	if target.IMDb != "" {
		result, ok, err := s.titles.RemoteLookup(ctx, asked, target.IMDb)
		if err != nil {
			return Link{}, err
		}
		if ok {
			found = &result
		} else if _, ok, err := s.titles.RemoteLookup(ctx, other, target.IMDb); err != nil {
			return Link{}, err
		} else if ok {
			return Link{}, ErrKindMismatch
		}
	} else {
		var err error
		if found, err = s.titles.LookUpIDs(ctx, asked, map[string]string{"Tmdb": strconv.Itoa(target.TMDB)}); err != nil {
			return Link{}, err
		}
		if found == nil {
			targets, err := s.titles.Resolve(ctx, []library.TitleRef{{Series: kind == KindSeries, TMDB: target.TMDB}})
			if err != nil {
				return Link{}, err
			}
			if len(targets) == 0 || len(targets[0]) == 0 {
				return Link{}, ErrTargetNotFound
			}
		}
	}
	if found == nil {
		return link, nil
	}
	link.Name, link.Year = found.Item.Name, found.Item.ProductionYear
	if imdb := strings.ToLower(provider(found.Item.ProviderIDs, "Imdb")); link.IMDb == "" && imdbID.MatchString(imdb) {
		link.IMDb = imdb
	}
	if number, ok := tmdbNumber(provider(found.Item.ProviderIDs, "Tmdb")); link.TMDB == 0 && ok {
		link.TMDB = number
	}
	return link, nil
}

// provider is the identifier of providers named name, whatever its case.
func provider(providers map[string]string, name string) string {
	for key, value := range providers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// itemChange is what applying entries changes of an item.
type itemChange struct {
	item   userdata.Item
	played bool
	at     *time.Time
	plays  int
	rating *float64
	resume *userdata.ImportedResume
}

// Apply applies to user's data what entries, kept of the titles an import
// did not find, would have imported, as the title of link: played marks at
// the later date with the larger play count, resume points under the
// thresholds the settings give, of the titles still unplayed, the rating
// where the user has none, and the watchlist, a movie or a series, never
// an episode. Nothing the user's data holds is taken back.
func (s *Store) Apply(ctx context.Context, user accounts.ID, entries []Entry, link Link) error {
	if len(entries) == 0 {
		return nil
	}
	refs := make([]library.TitleRef, len(entries))
	for i, e := range entries {
		refs[i] = link.ref(e.Label)
	}
	targets, err := s.titles.Resolve(ctx, refs)
	if err != nil {
		return err
	}
	changes := map[accounts.ID]*itemChange{}
	var order []accounts.ID
	lists := map[string][]watchlist.Title{}
	for i, e := range entries {
		for _, t := range targets[i] {
			c := changes[t.ID]
			if c == nil {
				c = &itemChange{item: userdata.Item{ID: t.ID, Series: t.Series, Season: t.Season}}
				changes[t.ID] = c
				order = append(order, t.ID)
			}
			if e.Played {
				c.played = true
				c.at = later(c.at, e.PlayedAt)
				c.plays = max(c.plays, e.Plays)
			}
			if e.Rating != nil && c.rating == nil {
				c.rating = e.Rating
			}
			if r := e.Resume; r != nil && (c.resume == nil || r.At.After(c.resume.At)) {
				point := *r
				if point.Runtime <= 0 {
					point.Runtime = t.Runtime
				}
				c.resume = &point
			}
			if e.Watchlist != "" {
				id := t.ID
				if t.Series != (accounts.ID{}) {
					id = t.Series
				}
				lists[e.Watchlist] = append(lists[e.Watchlist], watchlist.Title{ID: id, AddedAt: e.AddedAt})
			}
		}
	}
	// Rows are written in the order of their keys, so that writers running
	// at once never deadlock.
	slices.SortFunc(order, func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })
	items := make([]userdata.Item, 0, len(order))
	for _, id := range order {
		items = append(items, changes[id].item)
	}
	settings := s.settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	if _, err := s.userData.ChangeEach(ctx, user, items, func(item userdata.Item, d *userdata.Data) {
		c := changes[item.ID]
		if c.played {
			d.ImportPlayed(c.at)
			// Polyfin counted its own plays: the larger count stands.
			d.PlayCount = max(d.PlayCount, c.plays)
		}
		if c.rating != nil && d.Rating == nil {
			d.Rating = new(*c.rating)
		}
		if c.resume != nil {
			d.ImportResume(*c.resume, thresholds)
		}
	}); err != nil {
		return err
	}
	if s.watchlist == nil {
		return nil
	}
	names := make([]string, 0, len(lists))
	for name := range lists {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if _, err := s.watchlist.Save(ctx, user, name, lists[name], false); err != nil {
			return err
		}
	}
	return nil
}
