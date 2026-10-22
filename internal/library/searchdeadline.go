package library

import (
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A search answers with what its search catalogs found by its deadline,
// not after the slowest of them: an addon that never answers would hold
// every search for its request's 15-second timeout (see stremio.Client).
// Pages that come after the deadline are kept for the next search of the
// same term (see page), and a search catalog that missed the deadline is
// left out of the searches for a while (see lateSearches).
//
// Folder matching and the metadata editor (RemoteSearch) still wait for
// every addon: a title left out there would leave a file unmatched, or be
// missing from what an administrator chooses from.
const (
	// searchGrace is how long a search waits for its other title
	// searches once one of them found its page. A page is usually read
	// in well under a second, so an addon three seconds slower than the
	// fastest one is late: the search answers without it.
	searchGrace = 3 * time.Second
	// searchBound is how long a search waits for its title searches at
	// most, when none found its page: a third less than an addon
	// request's timeout, which an addon that never answers takes whole.
	searchBound = 10 * time.Second
	// lateSearchLife is how long a search catalog that missed a search's
	// deadline is left out of the searches, unless one of its pages comes
	// in meanwhile.
	lateSearchLife = 5 * time.Minute
	// emptySearchLife is how long a search's empty page is kept, where
	// pages that list titles are kept searchTTL: an addon may answer
	// nothing while what it reads from fails.
	emptySearchLife = time.Minute
)

// searchCatalog is an addon's search catalog, whatever the term.
type searchCatalog struct {
	addon       accounts.ID
	catalogType string
	catalogID   string
}

// searchCatalogOf is the search catalog of key.
func searchCatalogOf(key pageKey) searchCatalog {
	return searchCatalog{key.addon, key.catalogType, key.catalogID}
}

// lateSearches are the search catalogs left out of the searches, with
// when they are asked again.
type lateSearches struct {
	mu    sync.Mutex
	until map[searchCatalog]time.Time
}

// remember leaves c out of the searches until until, and reports whether
// it was not left out already.
func (l *lateSearches) remember(c searchCatalog, now, until time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.until == nil {
		l.until = map[searchCatalog]time.Time{}
	}
	for kept, end := range l.until {
		if !end.After(now) {
			delete(l.until, kept)
		}
	}
	_, already := l.until[c]
	l.until[c] = until
	return !already
}

// forget asks c again in the searches.
func (l *lateSearches) forget(c searchCatalog) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.until, c)
}

// late reports whether c is left out of the searches at now.
func (l *lateSearches) late(c searchCatalog, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	end, ok := l.until[c]
	return ok && end.After(now)
}

// addon reports whether a search catalog of addon is left out of the
// searches at now.
func (l *lateSearches) addon(addon accounts.ID, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for c, end := range l.until {
		if c.addon == addon && end.After(now) {
			return true
		}
	}
	return false
}

// SearchesLeaveOut reports whether the searches leave out a search
// catalog of the addon id for now, as it did not answer a search in time
// (see searched): System › Health tells it.
func (s *Service) SearchesLeaveOut(id accounts.ID) bool {
	return s.lateSearches.addon(id, s.now())
}

// skipsSearch reports whether a search leaves src out, as it was late:
// unless it found a page for that term already, kept.
func (s *Service) skipsSearch(src source) bool {
	key := src.key(0)
	if !s.lateSearches.late(searchCatalogOf(key), s.now()) {
		return false
	}
	kept, ok := s.searchPages.Get(key)
	return !ok || len(kept.value) == 0
}

// lateSearch leaves src out of the searches for lateSearchLife, as it
// missed a search's deadline.
func (s *Service) lateSearch(src source) {
	now := s.now()
	if s.lateSearches.remember(searchCatalogOf(src.key(0)), now, now.Add(lateSearchLife)) {
		s.logger.Warn("A search catalog did not answer in time: the searches leave it out for a while",
			"addon", src.addon.addon.Manifest.Name, "catalog", src.catalog.ID, "for", lateSearchLife)
	}
}
