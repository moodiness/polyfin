package library

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/addons"
	"github.com/moodiness/polyfin/internal/plainsearch"
	"github.com/moodiness/polyfin/internal/smartlists"
)

// A search in plain words (see package plainsearch), such as "80s sci-fi
// under 2 hours", stands for a collection: a smart list of the user's own,
// not saved, drawn from the catalogs of their libraries of movies and
// series, or of those of the type it names, with the rules it stands for.
// It is listed as any smart list shown as a collection, within the user's
// limits, but not in the Collections view. Each user's last
// plainSearchesKept are remembered, in memory, for plainSearchLife after
// they last made them; the same words, in any letter case, make the same
// collection again. Saving it (see SavePlainSearch) makes it one of the
// user's own smart lists.

const (
	plainSearchLife   = 24 * time.Hour
	plainSearchesKept = 20
)

// ErrOwnListsOff reports a user whose own smart lists their apps do not
// show: one under parental control or blocking genres, or whose own addons
// the server or their own permission turned off.
var ErrOwnListsOff = errors.New("the user's own smart lists are not shown")

// plainSearches are the searches in plain words users made lately, by user,
// the latest last.
type plainSearches struct {
	mu   sync.Mutex
	made map[accounts.ID][]plainSearch
}

// plainSearch is a search remembered: the list it stands for, and when it
// was last made.
type plainSearch struct {
	list smartlists.List
	made time.Time
}

// remember keeps list as the latest of user's searches and returns it as
// kept: one made again keeps when it was first made, which tells apart the
// titles read for it (see smartKey), unless what it draws from or its
// rules changed. Searches past their life are forgotten, every user's.
func (p *plainSearches) remember(user accounts.ID, list smartlists.List, now time.Time) smartlists.List {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.made == nil {
		p.made = map[accounts.ID][]plainSearch{}
	}
	for id, searches := range p.made {
		if searches = slices.DeleteFunc(searches, func(s plainSearch) bool { return now.Sub(s.made) > plainSearchLife }); len(searches) == 0 {
			delete(p.made, id)
		} else {
			p.made[id] = searches
		}
	}
	searches := p.made[user]
	if i := slices.IndexFunc(searches, func(s plainSearch) bool { return s.list.ID == list.ID }); i >= 0 {
		if kept := searches[i].list; sameJSON(kept.Catalogs, list.Catalogs) && sameJSON(kept.Rules, list.Rules) {
			list.UpdatedAt = kept.UpdatedAt
		}
		searches = slices.Delete(searches, i, i+1)
	}
	searches = append(searches, plainSearch{list: list, made: now})
	if len(searches) > plainSearchesKept {
		searches = searches[len(searches)-plainSearchesKept:]
	}
	p.made[user] = searches
	return list
}

// of returns the lists of user's searches still alive, in order.
func (p *plainSearches) of(user accounts.ID, now time.Time) []smartlists.List {
	p.mu.Lock()
	defer p.mu.Unlock()
	var lists []smartlists.List
	for _, s := range p.made[user] {
		if now.Sub(s.made) <= plainSearchLife {
			lists = append(lists, s.list)
		}
	}
	return lists
}

// find returns the list of user's search whose collection is item id.
func (p *plainSearches) find(user, id accounts.ID, now time.Time) (smartlists.List, bool) {
	for _, l := range p.of(user, now) {
		if SmartListItemID(l.ID) == id {
			return l, true
		}
	}
	return smartlists.List{}, false
}

// sameJSON reports whether a and b encode alike.
func sameJSON(a, b any) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && string(x) == string(y)
}

// plainSearchCatalog reports whether a search in plain words draws from a
// library's catalog: one of movies or series a smart list may draw from.
func plainSearchCatalog(l library) bool {
	return (l.catalog.Type == "movie" || l.catalog.Type == "series") && !l.addon.addon.HasMusic() && smartCatalog(l.catalog)
}

// PlainSearch returns the collection user's search stands for when it is
// in plain words (see package plainsearch), its genres being those of the
// user's libraries of movies and series, and remembers it; ok is false for
// any other search, and for one of a user who has no such library.
func (s *Service) PlainSearch(ctx context.Context, user accounts.User, term string) (item Item, ok bool, err error) {
	if user.ID == (accounts.ID{}) {
		return Item{}, false, nil
	}
	v, err := s.view(ctx, user)
	if err != nil {
		return Item{}, false, err
	}
	var genres []string
	for _, l := range v.libraries {
		if !plainSearchCatalog(l) {
			continue
		}
		if l.genre != "" {
			genres = append(genres, l.genre)
		} else {
			genres = append(genres, addons.GenreChoices(l.catalog)...)
		}
	}
	name := strings.Join(strings.Fields(term), " ")
	query, ok := plainsearch.Parse(name, genres)
	if !ok {
		return Item{}, false, nil
	}
	var catalogs []smartlists.Catalog
	for _, l := range v.libraries {
		c := smartlists.Catalog{AddonID: l.addon.addon.ID, Type: l.catalog.Type, ID: l.catalog.ID}
		if plainSearchCatalog(l) && (query.Type == "" || query.Type == l.catalog.Type) && !slices.Contains(catalogs, c) &&
			len(catalogs) < smartlists.MaxCatalogs {
			catalogs = append(catalogs, c)
		}
	}
	if len(catalogs) == 0 {
		return Item{}, false, nil
	}
	now := s.now()
	list := s.plain.remember(user.ID, smartlists.List{
		ID:      itemID("plainsearch|" + user.ID.String() + "|" + strings.ToLower(name)),
		Owner:   &user.ID,
		Name:    name,
		ShownAs: smartlists.ShownAsCollection, Catalogs: catalogs, Rules: query.Rules, Exclude: smartlists.NoExclusions(),
		Sort: smartlists.SortCatalog, UpdatedAt: now,
	}, now)
	return smartItem(list), true, nil
}

// plainSearchLists adds to v the collections of user's searches in plain
// words.
func (s *Service) plainSearchLists(v *view, user accounts.ID) {
	if user == (accounts.ID{}) {
		return
	}
	for _, l := range s.plain.of(user, s.now()) {
		v.smart = append(v.smart, smartList{item: smartItem(l), list: l, search: true})
	}
}

// IsPlainSearch reports whether id is the collection of one of user's
// searches in plain words.
func (s *Service) IsPlainSearch(user accounts.User, id accounts.ID) bool {
	_, ok := s.plain.find(user.ID, id, s.now())
	return ok
}

// savedFrom reports whether l is the list saved from search: one of the
// user's own, shown as a collection, of the same name and rules.
func savedFrom(l, search smartlists.List) bool {
	return l.Name == search.Name && l.ShownAs == smartlists.ShownAsCollection && sameJSON(l.Rules, search.Rules)
}

// SavePlainSearch saves the collection id of one of user's searches in
// plain words as one of their own smart lists, unless one was saved from
// it already; with save unset, it deletes those saved from it. found is
// false when id is no such collection. ErrOwnListsOff refuses a user whose
// apps would not show the list, and smartlists.ErrTooMany one who has as
// many as they may.
func (s *Service) SavePlainSearch(ctx context.Context, user accounts.User, id accounts.ID, save bool) (found bool, err error) {
	search, ok := s.plain.find(user.ID, id, s.now())
	if !ok {
		return false, nil
	}
	if !browsesOwnAddons(user, s.settings()) {
		return true, ErrOwnListsOff
	}
	lists, err := s.smart.Lists(ctx, &user.ID)
	if err != nil {
		return true, err
	}
	saved := false
	for _, l := range lists {
		if !savedFrom(l, search) {
			continue
		}
		saved = true
		if !save {
			if err := s.smart.Delete(ctx, &user.ID, l.ID); err != nil && !errors.Is(err, smartlists.ErrNotFound) {
				return true, err
			}
		}
	}
	if save && !saved {
		own := search
		own.ID, own.UpdatedAt = accounts.ID{}, time.Time{}
		_, err = s.smart.Create(ctx, own)
	}
	return true, err
}

// SavedPlainSearches tells which of ids are collections of user's searches
// in plain words saved as their own smart lists (see SavePlainSearch).
func (s *Service) SavedPlainSearches(ctx context.Context, user accounts.User, ids []accounts.ID) (map[accounts.ID]bool, error) {
	if user.ID == (accounts.ID{}) {
		return nil, nil
	}
	now := s.now()
	var searches []smartlists.List
	for _, id := range ids {
		if l, ok := s.plain.find(user.ID, id, now); ok {
			searches = append(searches, l)
		}
	}
	if len(searches) == 0 {
		return nil, nil
	}
	lists, err := s.smart.Lists(ctx, &user.ID)
	if err != nil {
		return nil, err
	}
	saved := map[accounts.ID]bool{}
	for _, search := range searches {
		if slices.ContainsFunc(lists, func(l smartlists.List) bool { return savedFrom(l, search) }) {
			saved[SmartListItemID(search.ID)] = true
		}
	}
	return saved, nil
}
