package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// VersionProgress tells how far the listing of a title's versions has
// come since its details opened: Pending is how many of the user's addons
// are still asked for its streams in the background, for the first time
// or again (see library.Service.Pending), Count how many media sources its
// details would list now, the placeholder included, and Known how many of
// them are versions, the placeholder left out. Polyfin's jellyfin-web
// script follows it to add versions to the title page as they come, and to
// hold its play buttons until one is known: it is pushed on the user's
// sockets as it changes (see versionsChanged), and answered when asked.
type VersionProgress struct {
	Pending int
	Count   int
	Known   int
}

// newProgress is the progress of a title with pending addons still asked
// and the versions offered its details would list.
func newProgress(pending int, offered []library.Version) VersionProgress {
	// As details count them (see addMediaSources).
	return VersionProgress{Pending: pending, Count: max(len(offered), 1), Known: len(offered)}
}

const (
	// versionsMessage is the type of the socket message that tells a
	// title page's VersionProgress, with the identifier the page was
	// opened with as ItemId.
	versionsMessage = "PolyfinVersions"
	// watchedFor is how long a title page is told of its versions after
	// its details or its script last asked: the script follows them for 90
	// seconds at most.
	watchedFor = 2 * time.Minute
	// pushDelay is how long the changes to a title's versions gather
	// before they are pushed: an addon's answer comes with the end of its
	// request.
	pushDelay = 250 * time.Millisecond
	// maxWatched is how many titles are watched before those no longer
	// watched are forgotten.
	maxWatched = 1000
	// searchInterval is how often a user may have a title's addons asked
	// again (see searchVersions).
	searchInterval = 20 * time.Second
)

// versionsPush is the data of a PolyfinVersions message.
type versionsPush struct {
	ItemId      string
	Pending     int
	Count       int
	Known       int
	AddVersions bool
}

// versionsAnswer answers /Polyfin/Items/{itemId}/Versions and its search:
// the title's progress, and whether the web player adds the versions that
// come to a title's page while it is open (Settings.AddVersionsToOpenPage);
// when it does not, the script only replaces the placeholder.
type versionsAnswer struct {
	VersionProgress
	AddVersions bool
}

// writeProgress answers progress, with the web player's setting.
func (h *Handler) writeProgress(w http.ResponseWriter, progress VersionProgress) {
	writeJSON(w, http.StatusOK, versionsAnswer{VersionProgress: progress, AddVersions: h.Accounts.Settings().AddVersionsToOpenPage})
}

// watchedPages are the title pages users opened lately, by title, then by
// user, each with the identifiers it was opened with and until when it is
// watched; scheduled holds the titles whose push is due.
type watchedPages struct {
	mu        sync.Mutex
	titles    map[accounts.ID]map[accounts.ID]*pageWatch
	scheduled map[accounts.ID]bool
}

type pageWatch struct {
	user   accounts.User
	opened map[accounts.ID]bool
	until  time.Time
}

func newWatchedPages() *watchedPages {
	return &watchedPages{titles: map[accounts.ID]map[accounts.ID]*pageWatch{}, scheduled: map[accounts.ID]bool{}}
}

// watch has a user's page of a title, opened as opened, told of the
// title's versions for watchedFor.
func (h *Handler) watch(user accounts.User, title, opened accounts.ID) {
	w := h.watched
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.titles) >= maxWatched {
		for title, users := range w.titles {
			for user, page := range users {
				if now.After(page.until) {
					delete(users, user)
				}
			}
			if len(users) == 0 {
				delete(w.titles, title)
			}
		}
	}
	users := w.titles[title]
	if users == nil {
		users = map[accounts.ID]*pageWatch{}
		w.titles[title] = users
	}
	page := users[user.ID]
	if page == nil {
		page = &pageWatch{opened: map[accounts.ID]bool{}}
		users[user.ID] = page
	}
	page.user, page.opened[opened], page.until = user, true, now.Add(watchedFor)
}

// versionsChanged is told by the library that a title's versions may have
// changed. The pages that watch the title are told their progress a moment
// later (see pushVersions).
func (h *Handler) versionsChanged(title accounts.ID) {
	w := h.watched
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.titles[title]) == 0 || w.scheduled[title] {
		return
	}
	w.scheduled[title] = true
	time.AfterFunc(pushDelay, func() { h.pushVersions(title) })
}

// pushVersions tells the pages that watch a title their progress, on their
// users' sockets, and, with playback prepared ahead, prepares the versions
// a play would now read first (see prepareOpened): an addon that answered
// after the details may have put another version first.
func (h *Handler) pushVersions(title accounts.ID) {
	w := h.watched
	now := time.Now()
	w.mu.Lock()
	delete(w.scheduled, title)
	type page struct {
		user   accounts.User
		opened []accounts.ID
	}
	var pages []page
	for id, watch := range w.titles[title] {
		if now.After(watch.until) {
			delete(w.titles[title], id)
			continue
		}
		p := page{user: watch.user}
		for opened := range watch.opened {
			p.opened = append(p.opened, opened)
		}
		pages = append(pages, p)
	}
	if len(w.titles[title]) == 0 {
		delete(w.titles, title)
	}
	w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), socketWrite)
	defer cancel()
	settings := h.Accounts.Settings()
	for _, p := range pages {
		pending, versions, ok := h.Library.FollowedProgress(ctx, p.user, title)
		if !ok {
			continue
		}
		offered := h.offered(ctx, p.user, versions)
		conns := h.userSockets(p.user.ID)
		for _, opened := range p.opened {
			progress := newProgress(pending, offered)
			payload := socketPayload(versionsMessage, versionsPush{ItemId: opened.String(), Pending: progress.Pending, Count: progress.Count, Known: progress.Known,
				AddVersions: settings.AddVersionsToOpenPage})
			for _, conn := range conns {
				_ = conn.Write(ctx, websocket.MessageText, payload)
			}
			if settings.PrepareAhead {
				h.prepareOpened(ctx, p.user, library.Item{ID: title}, offered, opened)
			}
		}
	}
}

// userSockets lists the sockets a user's apps keep open.
func (h *Handler) userSockets(user accounts.ID) []*websocket.Conn {
	s := h.sockets
	s.mu.Lock()
	defer s.mu.Unlock()
	conns := make([]*websocket.Conn, 0, len(s.open[user]))
	for conn := range s.open[user] {
		conns = append(conns, conn)
	}
	return conns
}

// versionProgress answers /Polyfin/Items/{itemId}/Versions for a movie or
// an episode, by its own identifier or one of its versions', with the
// checks of its details; other items answer zero for all three. A title
// whose details the user opened lately is answered from what is kept in
// memory (see library.Service.FollowedProgress). The page asking is told of
// the changes from then on (see versionsChanged).
func (h *Handler) versionProgress(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if progress, ok := h.followedProgress(r.Context(), user, id); ok {
		h.writeProgress(w, progress)
		return
	}
	item, ok := h.versionedItem(w, r, user, id)
	if !ok {
		return
	}
	var progress VersionProgress
	if item.Kind == library.KindMovie || item.Kind == library.KindEpisode {
		if progress, ok = h.titleProgress(w, r, user, item.ID, id); !ok {
			return
		}
	}
	h.writeProgress(w, progress)
}

// searchVersions answers POST /Polyfin/Items/{itemId}/Versions/Search for
// a movie or an episode, with the checks of versionProgress: it asks the
// user's addons again for the title's streams (see
// library.Service.AskAgain), at most once every searchInterval for a user's
// title, and answers the progress then. Other items ask nothing and answer
// zero for all three.
func (h *Handler) searchVersions(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	item, ok := h.versionedItem(w, r, user, id)
	if !ok {
		return
	}
	if item.Kind != library.KindMovie && item.Kind != library.KindEpisode {
		h.writeProgress(w, VersionProgress{})
		return
	}
	if allowed, wait := h.searches.allow(user.ID, item.ID, h.now()); !allowed {
		// Whole seconds, rounded up: asking at once after waiting them is allowed.
		w.Header().Set("Retry-After", strconv.Itoa(int((wait+time.Second-1)/time.Second)))
		processingError(w, http.StatusTooManyRequests)
		return
	}
	if err := h.Library.AskAgain(r.Context(), user, item.ID); err != nil {
		h.browseError(w, r, err)
		return
	}
	if progress, ok := h.followedProgress(r.Context(), user, id); ok {
		h.writeProgress(w, progress)
		return
	}
	if progress, ok := h.titleProgress(w, r, user, item.ID, id); ok {
		h.writeProgress(w, progress)
	}
}

// followedProgress answers the progress of a title whose details the user
// opened lately, by its identifier or one of its versions', from what is
// kept in memory, and has the page opened as id told of its changes.
func (h *Handler) followedProgress(ctx context.Context, user accounts.User, id accounts.ID) (VersionProgress, bool) {
	pending, versions, ok := h.Library.FollowedProgress(ctx, user, id)
	if !ok {
		return VersionProgress{}, false
	}
	title := id
	if owner, ok := h.Library.VersionOwner(id); ok {
		title = owner
	}
	h.watch(user, title, id)
	return newProgress(pending, h.offered(ctx, user, versions)), true
}

// versionedItem finds the item id names, or the title of the version it
// names, as the user may see it, answering the error otherwise.
func (h *Handler) versionedItem(w http.ResponseWriter, r *http.Request, user accounts.User, id accounts.ID) (library.Item, bool) {
	item, err := h.Library.Item(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		if owner, ok := h.Library.VersionOwner(id); ok {
			item, err = h.Library.Item(r.Context(), user, owner)
		}
	}
	if err != nil {
		h.browseError(w, r, err)
		return library.Item{}, false
	}
	return item, true
}

// titleProgress answers the progress of a movie or an episode, asking the
// library, answering the error otherwise, and has the page opened as
// opened told of its changes.
func (h *Handler) titleProgress(w http.ResponseWriter, r *http.Request, user accounts.User, title, opened accounts.ID) (VersionProgress, bool) {
	// Pending first: an addon no longer asked has its streams kept, so the
	// versions that follow have them.
	pending, err := h.Library.Pending(r.Context(), user, title)
	if err != nil {
		h.browseError(w, r, err)
		return VersionProgress{}, false
	}
	versions, err := h.Library.KnownVersions(r.Context(), user, title)
	if err != nil {
		h.browseError(w, r, err)
		return VersionProgress{}, false
	}
	h.watch(user, title, opened)
	return newProgress(pending, h.offered(r.Context(), user, versions)), true
}

// versionSearches remembers when each user last had each title's addons
// asked again. Its zero value is ready.
type versionSearches struct {
	mu   sync.Mutex
	last map[searchKey]time.Time
}

type searchKey struct {
	user, title accounts.ID
}

// allow reports whether user may have title's addons asked again at now,
// and records it, or how long until they may.
func (s *versionSearches) allow(user, title accounts.ID, now time.Time) (bool, time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		s.last = map[searchKey]time.Time{}
	}
	for key, at := range s.last {
		if now.Sub(at) >= searchInterval {
			delete(s.last, key)
		}
	}
	key := searchKey{user, title}
	if at, ok := s.last[key]; ok {
		return false, searchInterval - now.Sub(at)
	}
	s.last[key] = now
	return true, 0
}
