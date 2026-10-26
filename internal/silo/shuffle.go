package silo

import (
	"cmp"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// Shuffles.
//
// A shuffle plays a library, a series, a season or a collection in random
// order, as Jellyfin apps' Shuffle plays them, with no title twice until
// all have played. Its titles are listed once, in random order, as the
// Jellyfin API lists them to the user; a series among them plays one of
// its episodes, picked when it comes up. Each title is described again
// when it comes up: one the user may no longer see is left out.

func (s *Server) shuffleRoutes() {
	s.mux.HandleFunc("GET /api/v2/shuffles/capabilities", s.signedIn(s.shuffleCapabilities))
	s.mux.HandleFunc("POST /api/v2/shuffles", s.profiled(s.createShuffle))
	s.mux.HandleFunc("GET /api/v2/shuffles/{id}", s.profiled(s.shuffleStep(nil)))
	s.mux.HandleFunc("POST /api/v2/shuffles/{id}/advance", s.profiled(s.shuffleStep((*shuffle).advance)))
	s.mux.HandleFunc("POST /api/v2/shuffles/{id}/skip", s.profiled(s.shuffleStep((*shuffle).skip)))
	s.mux.HandleFunc("DELETE /api/v2/shuffles/{id}", s.profiled(s.deleteShuffle))
}

// shuffleKinds are the scopes a shuffle draws from: Polyfin keeps no
// personal collections.
var shuffleKinds = []string{"library", "series", "season", "library_collection"}

func (s *Server) shuffleCapabilities(w http.ResponseWriter, _ *http.Request, _ viewer) {
	writeJSON(w, http.StatusOK, struct {
		State      string   `json:"state"`
		Allowed    bool     `json:"allowed"`
		Revision   string   `json:"revision"`
		ScopeKinds []string `json:"scope_kinds"`
	}{"available", true, revision, shuffleKinds})
}

const (
	// shuffleTitles bounds the titles a shuffle draws from.
	shuffleTitles = 500
	// shufflesKept bounds the shuffles each user keeps; the oldest goes.
	shufflesKept = 8
)

type shuffleScope struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	ParentTitle string `json:"parent_title,omitempty"`
}

// shuffle is a running shuffle: deck holds the titles of the cycle, in the
// order they play, deck[0] playing now and deck[1] next.
type shuffle struct {
	mu      sync.Mutex
	id      string
	user    accounts.ID
	scope   shuffleScope
	titles  []string
	deck    []string
	created time.Time
	updated time.Time
	seen    time.Time
}

// advance moves past from, which must be playing: the next title plays.
// Asked again, when from has moved already, it changes nothing.
func (sh *shuffle) advance(from string) {
	if len(sh.deck) > 0 && sh.deck[0] == from {
		sh.deck = sh.deck[1:]
	}
}

// skip replaces next, which must be the next title, with another of the
// cycle, putting it back later in it.
func (sh *shuffle) skip(next string) {
	if len(sh.deck) > 2 && sh.deck[1] == next {
		other := 2 + rand.IntN(len(sh.deck)-2)
		sh.deck[1], sh.deck[other] = sh.deck[other], sh.deck[1]
	}
}

// refill starts a new cycle once the deck holds fewer than two titles,
// without the one playing first.
func (sh *shuffle) refill() {
	if len(sh.deck) >= 2 || len(sh.titles) < 2 {
		if len(sh.deck) == 0 {
			sh.deck = slices.Clone(sh.titles)
		}
		return
	}
	cycle := slices.Clone(sh.titles)
	rand.Shuffle(len(cycle), func(i, j int) { cycle[i], cycle[j] = cycle[j], cycle[i] })
	if len(sh.deck) == 1 {
		cycle = slices.DeleteFunc(cycle, func(id string) bool { return id == sh.deck[0] })
	}
	sh.deck = append(sh.deck, cycle...)
}

type shuffles struct {
	mu   sync.Mutex
	byID map[string]*shuffle
}

func newShuffles() *shuffles { return &shuffles{byID: map[string]*shuffle{}} }

// add keeps a shuffle, forgetting those unused for a day and the user's
// oldest past shufflesKept.
func (s *shuffles) add(sh *shuffle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var mine []*shuffle
	for id, other := range s.byID {
		switch {
		case sh.seen.Sub(other.seen) > sessionLifetime:
			delete(s.byID, id)
		case other.user == sh.user:
			mine = append(mine, other)
		}
	}
	slices.SortFunc(mine, func(a, b *shuffle) int { return a.seen.Compare(b.seen) })
	for len(mine) >= shufflesKept {
		delete(s.byID, mine[0].id)
		mine = mine[1:]
	}
	s.byID[sh.id] = sh
}

// get is a shuffle of user, locked: done unlocks it.
func (s *shuffles) get(id string, user accounts.ID, now time.Time) (*shuffle, func(), bool) {
	s.mu.Lock()
	found, ok := s.byID[id]
	if ok && found.user == user && now.Sub(found.seen) <= sessionLifetime {
		found.seen = now
	} else {
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return nil, nil, false
	}
	found.mu.Lock()
	return found, found.mu.Unlock, true
}

func (s *shuffles) remove(id string, user accounts.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if found, ok := s.byID[id]; ok && found.user == user {
		delete(s.byID, id)
	}
}

// createShuffle starts a shuffle of a scope the user may see.
func (s *Server) createShuffle(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		Scope struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"scope"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains(shuffleKinds, body.Scope.Kind) {
		validationProblem(w, "body.scope.kind", "unsupported", "This server shuffles libraries, series, seasons and collections.")
		return
	}
	scope := shuffleScope{Kind: body.Scope.Kind, ID: body.Scope.ID}
	query := url.Values{"IncludeItemTypes": {titleTypes + ",Episode"}, "SortBy": {"Random"}, "Limit": {strconv.Itoa(shuffleTitles)}}
	if scope.Kind == "library" {
		view, ok := s.library(w, r, v, scope.ID)
		if !ok {
			return
		}
		scope.Title = view.Name
		query.Set("ParentId", view.Id)
		query.Set("Recursive", "true")
		query.Set("IncludeItemTypes", titleTypes)
	} else {
		folder, ok := s.shuffleFolder(w, r, v, scope.ID)
		if !ok {
			return
		}
		want := map[string]string{"series": "Series", "season": "Season", "library_collection": "BoxSet"}[scope.Kind]
		if folder.Type != want {
			problem(w, http.StatusNotFound, "not_found", "Nothing to shuffle here.")
			return
		}
		scope.Title = folder.Name
		if scope.Kind == "season" {
			scope.ParentTitle = folder.SeriesName
		}
		query.Set("ParentId", folder.Id)
		if scope.Kind != "library_collection" {
			query.Set("Recursive", "true")
			query.Set("IncludeItemTypes", "Episode")
		}
	}
	var result queryResult
	a, err := s.call(r, v, http.MethodGet, "/Items?"+query.Encode(), nil, &result)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return
	}
	now := s.options.now()
	sh := &shuffle{id: newSessionID(), user: v.user.ID, scope: scope, created: now, updated: now, seen: now}
	for _, i := range result.Items {
		if slices.Contains([]string{"Movie", "Series", "Episode", "Video"}, i.Type) && !slices.Contains(sh.titles, i.Id) {
			sh.titles = append(sh.titles, i.Id)
		}
	}
	sh.deck = slices.Clone(sh.titles)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	answer, ok := s.shuffleAnswer(w, r, v, sh)
	if !ok {
		return
	}
	s.shuffles.add(sh)
	writeJSON(w, http.StatusCreated, answer)
}

// shuffleFolder describes the series, season or collection a shuffle draws
// from, answering the request when the user may not see it.
func (s *Server) shuffleFolder(w http.ResponseWriter, r *http.Request, v viewer, id string) (item, bool) {
	if _, err := accounts.ParseID(id); err != nil {
		problem(w, http.StatusNotFound, "not_found", "Nothing to shuffle here.")
		return item{}, false
	}
	var folder item
	a, err := s.call(r, v, http.MethodGet, "/Items/"+id, nil, &folder)
	if err != nil {
		s.internalError(w, r, err)
		return item{}, false
	}
	if !a.ok() {
		s.jellyfinRefusal(w, r, a)
		return item{}, false
	}
	return folder, true
}

// shuffleStep answers a shuffle, once step, when not nil, moved it on as
// the body asks.
func (s *Server) shuffleStep(step func(*shuffle, string)) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		var body struct {
			FromContentID string `json:"from_content_id"`
			NextContentID string `json:"next_content_id"`
		}
		if step != nil && !decode(w, r, &body) {
			return
		}
		sh, done, ok := s.shuffles.get(r.PathValue("id"), v.user.ID, s.options.now())
		if !ok {
			problem(w, http.StatusNotFound, "not_found", "No such shuffle.")
			return
		}
		defer done()
		if step != nil {
			before := slices.Clone(sh.deck)
			step(sh, cmp.Or(body.FromContentID, body.NextContentID))
			if !slices.Equal(before, sh.deck) {
				sh.updated = s.options.now()
			}
		}
		if answer, ok := s.shuffleAnswer(w, r, v, sh); ok {
			writeJSON(w, http.StatusOK, answer)
		}
	}
}

func (s *Server) deleteShuffle(w http.ResponseWriter, r *http.Request, v viewer) {
	s.shuffles.remove(r.PathValue("id"), v.user.ID)
	w.WriteHeader(http.StatusNoContent)
}

type shuffleJSON struct {
	ID        string       `json:"id"`
	Scope     shuffleScope `json:"scope"`
	Current   catalogItem  `json:"current"`
	Next      catalogItem  `json:"next"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
}

// shuffleAnswer describes what plays now and next, the user may see both:
// a title they may no longer see leaves the shuffle, and a series plays one
// of its episodes. With nothing left to play, it answers 409.
func (s *Server) shuffleAnswer(w http.ResponseWriter, r *http.Request, v viewer, sh *shuffle) (shuffleJSON, bool) {
	var playing []catalogItem
	for len(playing) < 2 {
		sh.refill()
		place := len(playing)
		if place >= len(sh.deck) {
			break
		}
		card, ok, err := s.shufflePick(r, v, sh.deck[place])
		if err != nil {
			s.internalError(w, r, err)
			return shuffleJSON{}, false
		}
		if !ok {
			// Gone from the user's sight: out of this shuffle.
			gone := sh.deck[place]
			sh.deck = slices.Delete(sh.deck, place, place+1)
			sh.titles = slices.DeleteFunc(sh.titles, func(id string) bool { return id == gone })
			continue
		}
		sh.deck[place] = card.ContentID
		if place == 1 && card.ContentID == playing[0].ContentID {
			if len(sh.titles) < 2 {
				break
			}
			// A series' episode came up again: another title follows.
			sh.deck = slices.Delete(sh.deck, 1, 2)
			continue
		}
		playing = append(playing, card)
	}
	switch len(playing) {
	case 0:
		problem(w, http.StatusConflict, "conflict", "Nothing in this shuffle can play.")
		return shuffleJSON{}, false
	case 1:
		// One title alone plays next too.
		playing = append(playing, playing[0])
	}
	return shuffleJSON{ID: sh.id, Scope: sh.scope, Current: playing[0], Next: playing[1], CreatedAt: instant(sh.created),
		UpdatedAt: instant(sh.updated)}, true
}

// shufflePick describes the title of a shuffle that comes up, a series
// one of its episodes, picked at random; ok is false for one the user may
// not see or that has no episode.
func (s *Server) shufflePick(r *http.Request, v viewer, id string) (catalogItem, bool, error) {
	var i item
	a, err := s.call(r, v, http.MethodGet, "/Items/"+id+"?Fields="+listFields, nil, &i)
	if err != nil || !a.ok() {
		return catalogItem{}, false, err
	}
	if i.Type == "Series" {
		var episodes queryResult
		a, err = s.call(r, v, http.MethodGet, "/Shows/"+i.Id+"/Episodes?Fields="+listFields, nil, &episodes)
		if err != nil || !a.ok() || len(episodes.Items) == 0 {
			return catalogItem{}, false, err
		}
		i = episodes.Items[rand.IntN(len(episodes.Items))]
	}
	if i.Type != "Movie" && i.Type != "Episode" && i.Type != "Video" {
		return catalogItem{}, false, nil
	}
	return catalogItemOf(r, i), true, nil
}
