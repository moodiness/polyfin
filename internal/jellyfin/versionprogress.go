package jellyfin

import (
	"context"
	"errors"
	"net/http"
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
// details would list now, the placeholder included. Polyfin's jellyfin-web
// script follows it to add versions to the title page as they come: it is
// pushed on the user's sockets as it changes (see versionsChanged), and
// answered when asked.
type VersionProgress struct {
	Pending int
	Count   int
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
)

// versionsPush is the data of a PolyfinVersions message.
type versionsPush struct {
	ItemId  string
	Pending int
	Count   int
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
	prepare := h.Accounts.Settings().PrepareAhead
	for _, p := range pages {
		pending, versions, ok := h.Library.FollowedProgress(ctx, p.user, title)
		if !ok {
			continue
		}
		offered := h.offered(ctx, p.user, versions)
		conns := h.userSockets(p.user.ID)
		for _, opened := range p.opened {
			payload := socketPayload(versionsMessage, versionsPush{ItemId: opened.String(), Pending: pending, Count: max(len(offered), 1)})
			for _, conn := range conns {
				_ = conn.Write(ctx, websocket.MessageText, payload)
			}
			if prepare {
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
// checks of its details; other items answer zero for both. A title whose
// details the user opened lately is answered from what is kept in memory
// (see library.Service.FollowedProgress). The page asking is told of the
// changes from then on (see versionsChanged).
func (h *Handler) versionProgress(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	if pending, versions, ok := h.Library.FollowedProgress(r.Context(), user, id); ok {
		title := id
		if owner, ok := h.Library.VersionOwner(id); ok {
			title = owner
		}
		h.watch(user, title, id)
		writeJSON(w, http.StatusOK, VersionProgress{Pending: pending, Count: max(len(h.offered(r.Context(), user, versions)), 1)})
		return
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		if owner, ok := h.Library.VersionOwner(id); ok {
			item, err = h.Library.Item(r.Context(), user, owner)
		}
	}
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	var progress VersionProgress
	if item.Kind == library.KindMovie || item.Kind == library.KindEpisode {
		// Pending first: an addon no longer asked has its streams kept, so
		// the count that follows has its versions.
		pending, err := h.Library.Pending(r.Context(), user, item.ID)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		progress.Pending = pending
		versions, err := h.Library.KnownVersions(r.Context(), user, item.ID)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		// As details count them (see addMediaSources).
		progress.Count = max(len(h.offered(r.Context(), user, versions)), 1)
		h.watch(user, item.ID, id)
	}
	writeJSON(w, http.StatusOK, progress)
}
