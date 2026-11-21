package library

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/database"
)

// A listing that shows its titles' versions (see CachedVersions) reads,
// for each title, its record and its series' or folder's, the
// descriptions kept of it and the stream lists saved before a restart:
// one by one, a statement each, page after page. ListingAhead reads them
// for the whole page first, a statement for each kind.

// aheadAtOnce bounds the titles whose stream lists ListingAhead finds at
// once: as many as a listing describes at once.
const aheadAtOnce = 4

// ListingAhead reads ahead what the versions of items, a page of a
// listing, read from the database (see CachedVersions): the records of
// the movies, videos and episodes and of their series and folders, the
// descriptions kept of them, and the stream lists saved for them. It
// returns the context to ask for their versions under, which holds the
// records read and the descriptions found missing; the stream lists and
// the descriptions found are kept as each title's own read keeps them.
// Nothing is read twice: a failure leaves each title to read its own.
func (s *Service) ListingAhead(ctx context.Context, user accounts.User, items []Item) context.Context {
	if s.db == nil {
		return ctx
	}
	titles := make([]accounts.ID, 0, len(items))
	ids := make([]accounts.ID, 0, 2*len(items))
	for _, item := range items {
		switch item.Kind {
		case KindMovie:
		case KindVideo:
			ids = append(ids, item.FolderID)
		case KindEpisode:
			ids = append(ids, item.SeriesID)
		default:
			continue
		}
		titles = append(titles, item.ID)
		ids = append(ids, item.ID)
	}
	ids = slices.DeleteFunc(ids, func(id accounts.ID) bool { return id == (accounts.ID{}) })
	slices.SortFunc(ids, func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)
	if len(titles) == 0 {
		return ctx
	}
	ahead, err := s.withPreloaded(ctx, ids)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("A listing's records could not be read ahead", "error", err)
		}
		return ctx
	}
	v, err := s.view(ahead, user)
	if err != nil {
		return ahead
	}
	ahead = s.metasAhead(ahead, v, ids)
	var (
		g    errgroup.Group
		mu   sync.Mutex
		keys []streamKey
	)
	g.SetLimit(aheadAtOnce)
	for _, id := range titles {
		g.Go(func() error {
			t, v, err := s.target(ahead, user, id)
			if err != nil {
				return nil
			}
			serving := s.streamServing(ahead, v, t)
			mu.Lock()
			defer mu.Unlock()
			for _, source := range serving {
				if !own(source.addon) {
					keys = append(keys, source.key())
				}
			}
			return nil
		})
	}
	_ = g.Wait()
	s.restoreStreams(ahead, keys)
	return ahead
}

// missingAheadKey holds the descriptions a listing found missing from the
// database (see metasAhead).
type missingAheadKey struct{}

// missingAhead reports whether metasAhead found the description of key
// missing from the database, under ctx.
func missingAhead(ctx context.Context, key metaKey) bool {
	missing, _ := ctx.Value(missingAheadKey{}).(map[metaKey]bool)
	return missing[key]
}

// keptMetasOf reads the descriptions kept of the titles of keys, by
// their place among them, kept since $5 (see metasAhead).
var keptMetasOf = database.Listing(`SELECT k.n, m.meta, m.fetched_at
	FROM unnest($1::uuid[], $2::text[], $3::text[], $4::text[]) WITH ORDINALITY AS k(addon_id, meta_type, meta_id, config, n)
	JOIN metas m ON m.addon_id = k.addon_id AND m.meta_type = k.meta_type AND m.meta_id = k.meta_id
	WHERE m.config = k.config AND m.fetched_at > $5`)

// metasAhead reads, in one statement, the descriptions the addons of v
// keep in the database of the titles of ids that ctx holds the records of
// (see withPreloaded), as describedMeta would read each of them when none
// is kept in memory: those found are kept in memory, as loadMeta keeps
// them; the context returned holds those missing, which loadMeta then
// does not look for again.
func (s *Service) metasAhead(ctx context.Context, v view, ids []accounts.ID) context.Context {
	type wanted struct {
		addon installed
		key   metaKey
	}
	var want []wanted
	seen := map[metaKey]bool{}
	for _, id := range ids {
		r, ok, err := preloaded(ctx, id)
		if !ok || err != nil || r.Meta == nil {
			continue
		}
		for _, candidate := range describers(v, r, false) {
			if own(candidate.addon) {
				if r.Addon != nil && candidate.addon.ID == *r.Addon {
					break
				}
				continue
			}
			key := metaKey{candidate.addon.ID, r.Meta.Type, r.Meta.ID}
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, kept := s.metas.Get(key); !kept {
				want = append(want, wanted{candidate, key})
			}
		}
	}
	if len(want) == 0 {
		return ctx
	}
	addonIDs := make([]accounts.ID, len(want))
	types := make([]string, len(want))
	metaIDs := make([]string, len(want))
	configs := make([]string, len(want))
	for i, w := range want {
		addonIDs[i], types[i], metaIDs[i], configs[i] = w.key.addon, w.key.metaType, w.key.id, configOf(w.addon.addon)
	}
	rows, err := s.db.Query(ctx, keptMetasOf, addonIDs, types, metaIDs, configs, s.now().Add(-metaKept))
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("A listing's kept descriptions could not be read ahead", "error", err)
		}
		return ctx
	}
	found := make([]bool, len(want))
	var (
		n    int
		data []byte
		at   time.Time
	)
	if _, err := pgx.ForEachRow(rows, []any{&n, &data, &at}, func() error {
		w := want[n-1]
		_, found[n-1] = s.loadedMeta(ctx, w.addon, w.key, data, at)
		return nil
	}); err != nil {
		if ctx.Err() == nil {
			s.logger.Debug("A listing's kept descriptions could not be read ahead", "error", err)
		}
		return ctx
	}
	missing := make(map[metaKey]bool, len(want))
	for i, w := range want {
		if !found[i] {
			missing[w.key] = true
		}
	}
	return context.WithValue(ctx, missingAheadKey{}, missing)
}
