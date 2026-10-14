package letterboxd

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localfiles"
	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/userdata"
	"github.com/moodiness/polyfin/internal/watchlist"
)

// batch is how many titles an import resolves and writes at a time.
const batch = 500

// imdbPattern is an IMDb identifier, which titles are found by.
var imdbPattern = regexp.MustCompile(`^tt\d{1,12}$`)

// run is an import running.
type run struct {
	s *Service
	// status is the import's, which s.mu guards.
	status *Status
	user   accounts.ID
	movies []Movie
	choice Choice
}

// update changes the status of the import.
func (r *run) update(change func(*Status)) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	change(r.status)
}

// run finds the movies, merges them into the user's data, and ends the
// import. An import stopped, as its user is deleted or Polyfin stops, did
// not fail.
func (r *run) run(ctx context.Context) {
	err := r.merge(ctx)
	if err != nil && ctx.Err() == nil {
		r.s.logger.Warn("A Letterboxd export could not be imported", "user_id", r.user.String(), "error", err)
	}
	if ctx.Err() == nil {
		metrics.Imported("letterboxd", err == nil)
	}
	r.update(func(s *Status) {
		s.State = StateDone
		if err != nil && ctx.Err() == nil {
			s.State, s.Problem = StateFailed, ProblemInternal
		}
		s.EndedAt = new(r.s.now().UTC())
	})
	r.s.mu.Lock()
	if r.s.imports[r.user] == r.status {
		delete(r.s.stops, r.user)
	}
	r.s.mu.Unlock()
}

// found is a movie matched: the title the search found, named by its
// identifiers, or the item it was recorded as when it has none.
type found struct {
	ref  library.TitleRef
	item accounts.ID
}

// match searches each movie's name, one search at a time, each name once,
// and keeps the one movie of that name and year. It returns the movies
// found, by their index, and lists the others with why.
func (r *run) match(ctx context.Context) (map[int]found, []Unmatched, error) {
	type search struct {
		results []library.RemoteResult
		err     error
	}
	searches := map[string]search{}
	matched := map[int]found{}
	var unmatched []Unmatched
	for i, m := range r.movies {
		key := strings.ToLower(m.Name)
		done, ok := searches[key]
		if !ok {
			done.results, done.err = r.s.titles.RemoteSearch(ctx, library.KindMovie, m.Name, localfiles.SearchLimit)
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if done.err != nil {
				r.s.logger.Debug("The metadata addons could not search a Letterboxd movie", "name", m.Name, "error", done.err)
			}
			searches[key] = done
		}
		reason := ""
		if done.err != nil {
			reason = ReasonSearchFailed
		} else if item, ok, ambiguous := localfiles.ChooseTitle(library.KindMovie, m.Name, m.Year, done.results); ambiguous {
			reason = ReasonAmbiguous
		} else if !ok {
			reason = ReasonNotFound
		} else {
			matched[i] = found{ref: titleRef(item), item: item.ID}
		}
		if reason != "" {
			unmatched = append(unmatched, Unmatched{Name: m.Name, Year: m.Year, Link: m.Link, Reason: reason})
		}
		r.update(func(s *Status) { s.Searched++ })
	}
	return matched, unmatched, nil
}

// titleRef names the movie item as the other imports' titles are named:
// by its IMDb and TMDB identifiers, either of which may be missing.
func titleRef(item library.Item) library.TitleRef {
	ref := library.TitleRef{Name: item.Name}
	if imdb := item.ProviderIDs["Imdb"]; imdbPattern.MatchString(imdb) {
		ref.IMDb = imdb
	}
	if tmdb, err := strconv.Atoi(item.ProviderIDs["Tmdb"]); err == nil && tmdb > 0 {
		ref.TMDB = tmdb
	}
	return ref
}

// change is what an import adds to the data of an item: a played mark at
// the latest date, with the most plays, a rating, a favorite. counts tells
// whether it counts in the import's result: an item of a movie's first
// title, as a movie several catalogs list gives several items.
type change struct {
	item     userdata.Item
	played   bool
	at       *time.Time
	plays    int
	rating   *float64
	favorite bool
	counts   bool
}

// merge finds the movies and adds them to the user's data: played marks,
// ratings and favorites, then the watchlist.
func (r *run) merge(ctx context.Context) error {
	matched, unmatched, err := r.match(ctx)
	if err != nil {
		return err
	}
	r.update(func(s *Status) {
		s.UnmatchedCount = len(unmatched)
		s.Unmatched = unmatched[:min(len(unmatched), maxUnmatched)]
	})

	// Movies named by identifiers are resolved, as other imports' titles
	// are; one without is the item its search recorded.
	var order []int
	var refs []library.TitleRef
	for i := range r.movies {
		if f, ok := matched[i]; ok && (f.ref.IMDb != "" || f.ref.TMDB > 0) {
			order = append(order, i)
			refs = append(refs, f.ref)
		}
	}
	targets := map[int][]accounts.ID{}
	for start := 0; start < len(refs); start += batch {
		resolved, err := r.s.titles.Resolve(ctx, refs[start:min(start+batch, len(refs))])
		if err != nil {
			return err
		}
		for j, found := range resolved {
			for _, t := range found {
				targets[order[start+j]] = append(targets[order[start+j]], t.ID)
			}
		}
	}
	for i, f := range matched {
		if len(targets[i]) == 0 {
			targets[i] = []accounts.ID{f.item}
		}
	}

	changes := map[accounts.ID]*change{}
	var changeOrder []accounts.ID
	var titles []watchlist.Title
	listed := map[accounts.ID]bool{}
	for i, m := range r.movies {
		for n, id := range targets[i] {
			c := changes[id]
			if c == nil {
				c = &change{item: userdata.Item{ID: id}}
				changes[id] = c
				changeOrder = append(changeOrder, id)
			}
			c.counts = c.counts || n == 0
			if m.Watched {
				c.played = true
				if m.PlayedAt != nil && (c.at == nil || m.PlayedAt.After(*c.at)) {
					c.at = m.PlayedAt
				}
				c.plays = max(c.plays, m.Plays)
			}
			if r.choice.Ratings && m.Rating != nil && c.rating == nil {
				c.rating = m.Rating
			}
			c.favorite = c.favorite || r.choice.LikesAsFavorites && m.Liked
			// The movie goes on the watchlist once, as its first title.
			if r.choice.Watchlist && m.Watchlist && n == 0 && !listed[id] {
				listed[id] = true
				titles = append(titles, watchlist.Title{ID: id, AddedAt: m.AddedAt})
			}
		}
	}

	// Rows are written in the order of their keys, so that writers running
	// at once never deadlock.
	slices.SortFunc(changeOrder, func(a, b accounts.ID) int { return bytes.Compare(a[:], b[:]) })

	for start := 0; start < len(changeOrder); start += batch {
		chunk := changeOrder[start:min(start+batch, len(changeOrder))]
		items := make([]userdata.Item, 0, len(chunk))
		for _, id := range chunk {
			items = append(items, changes[id].item)
		}
		var counted Status
		if _, err := r.s.userData.ChangeEach(ctx, r.user, items, func(item userdata.Item, d *userdata.Data) {
			c := changes[item.ID]
			if c.played {
				if d.ImportPlayed(c.at) && c.counts {
					counted.Played++
				}
				// Polyfin counted its own plays: the larger count stands.
				d.PlayCount = max(d.PlayCount, c.plays)
			}
			if c.rating != nil && d.Rating == nil {
				d.Rating = new(*c.rating)
				if c.counts {
					counted.Ratings++
				}
			}
			if c.favorite && !d.Favorite {
				d.Favorite = true
				if c.counts {
					counted.Favorites++
				}
			}
		}); err != nil {
			return err
		}
		r.update(func(s *Status) {
			s.Played += counted.Played
			s.Ratings += counted.Ratings
			s.Favorites += counted.Favorites
		})
	}
	if r.s.watchlist == nil || len(titles) == 0 {
		return nil
	}
	added, err := r.s.watchlist.Save(ctx, r.user, watchlist.Import, titles, false)
	if err != nil {
		return err
	}
	r.update(func(s *Status) { s.Watchlist += added })
	return nil
}
