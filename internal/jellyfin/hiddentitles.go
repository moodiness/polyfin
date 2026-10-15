package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/userdata"
)

// Jellyfin has no way to take a title off Continue Watching but marking it
// played or unplayed, nor to hide a title from one's lists. Polyfin's own
// routes do both, for its web player's script (see
// internal/server/webclient.js), its admin app's My account and any app:
//
//	DELETE /Polyfin/UserItems/{itemId}/Resume takes an item off Continue
//	  Watching (see userdata.Data.RemoveResume);
//	POST /Polyfin/UserItems/{itemId}/Hidden hides a movie or a series from
//	  the user's lists, DELETE shows it again (see library.HiddenTitles);
//	GET /Polyfin/UserItems/Hidden lists the hidden titles.
//
// Both change what the user's apps list, which their sockets are told as
// a change of the user data of the item, as for any other change.

var (
	// errNotHideable reports an item that is neither a title nor in one.
	errNotHideable = errors.New("only movies and series can be hidden")
	// errNoResume reports an item that keeps no resume point, nor holds
	// items that do.
	errNoResume = errors.New("the item has no resume point")
)

func (h *Handler) hiddenTitleRoutes(signedIn func(method, pattern string, handler http.HandlerFunc)) {
	signedIn(http.MethodDelete, "/Polyfin/UserItems/{itemId}/Resume", h.removeResume)
	signedIn(http.MethodPost, "/Polyfin/UserItems/{itemId}/Hidden", h.hideTitle(true))
	signedIn(http.MethodDelete, "/Polyfin/UserItems/{itemId}/Hidden", h.hideTitle(false))
	signedIn(http.MethodGet, "/Polyfin/UserItems/Hidden", h.hiddenItems)
}

// removeResume answers DELETE /Polyfin/UserItems/{itemId}/Resume with what
// the user did with the item afterwards, as Jellyfin's user data routes
// do. A series or a season loses the resume points of its episodes.
func (h *Handler) removeResume(w http.ResponseWriter, r *http.Request) {
	user, item, ok := h.userDataItem(w, r, bindErrors{})
	if !ok {
		return
	}
	if err := h.removeResumeOf(r.Context(), user, item); err != nil {
		if errors.Is(err, errNoResume) {
			badRequestProblem(w)
			return
		}
		h.browseError(w, r, err)
		return
	}
	data, err := h.itemData(r.Context(), user, item)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// RemoveResume takes the item user opens by id off their Continue
// Watching: its resume point goes, and nothing else, a series or a season
// losing those of its episodes. The user's tracking services are told
// nothing. library.ErrNotFound reports an item the user cannot open.
func (h *Handler) RemoveResume(ctx context.Context, user accounts.User, id accounts.ID) error {
	item, err := h.Library.Item(ctx, user, id)
	if errors.Is(err, library.ErrNotFound) {
		item, err = h.title(ctx, user, id)
	}
	if err != nil {
		return err
	}
	return h.removeResumeOf(ctx, user, item)
}

func (h *Handler) removeResumeOf(ctx context.Context, user accounts.User, item library.Item) error {
	var targets []library.Item
	switch item.Kind {
	case library.KindSeries, library.KindSeason:
		var err error
		if targets, err = h.markTargets(ctx, user, item); err != nil {
			return err
		}
	case library.KindMovie, library.KindVideo, library.KindEpisode, library.KindRecording, library.KindReplay, library.KindAudiobook,
		library.KindExtra:
		targets = []library.Item{item}
	default:
		return errNoResume
	}
	// Only the items with a resume point change: no row is made for the
	// others.
	ids := make([]accounts.ID, 0, len(targets))
	for _, target := range targets {
		ids = append(ids, target.ID)
	}
	data, err := h.UserData.Get(ctx, user.ID, ids)
	if err != nil {
		return err
	}
	targets = slices.DeleteFunc(targets, func(target library.Item) bool { return data[target.ID].Position <= 0 })
	if len(targets) == 0 {
		return nil
	}
	refs := make([]userdata.Item, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, stored(target))
	}
	now := time.Now()
	if _, err := h.UserData.ChangeEach(ctx, user.ID, refs, func(_ userdata.Item, d *userdata.Data) { d.RemoveResume(now) }); err != nil {
		return err
	}
	h.userDataChanged(user, append([]library.Item{item}, targets...))
	return nil
}

// hideTitle answers POST and DELETE /Polyfin/UserItems/{itemId}/Hidden
// with 204: hidden, a movie or a series leaves the user's lists; shown,
// it comes back. A season or an episode stands for its series.
func (h *Handler) hideTitle(hidden bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, item, ok := h.userDataItem(w, r, bindErrors{})
		if !ok {
			return
		}
		if user.ID == (accounts.ID{}) {
			// A request made with an API key names no user.
			notFoundProblem(w)
			return
		}
		title, err := h.hiddenTarget(r.Context(), user, item)
		if errors.Is(err, errNotHideable) {
			badRequestProblem(w)
			return
		}
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		if hidden {
			err = h.UserData.Hide(r.Context(), user.ID, title.ID, time.Now())
		} else {
			_, err = h.UserData.Show(r.Context(), user.ID, title.ID)
		}
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		h.userDataChanged(user, []library.Item{title})
		w.WriteHeader(http.StatusNoContent)
	}
}

// hiddenTarget returns the title hiding item hides: the movie, home video
// or series itself, or the series of a season or an episode.
func (h *Handler) hiddenTarget(ctx context.Context, user accounts.User, item library.Item) (library.Item, error) {
	switch {
	case library.Hideable(item.Kind):
		return item, nil
	case (item.Kind == library.KindSeason || item.Kind == library.KindEpisode) && item.SeriesID != accounts.ID{}:
		return h.Library.Item(ctx, user, item.SeriesID)
	}
	return library.Item{}, errNotHideable
}

// ShowTitle shows a title user hid in their lists again, by its identifier
// or by that of one of its seasons or episodes. A title no longer known is
// shown again all the same.
func (h *Handler) ShowTitle(ctx context.Context, user accounts.User, id accounts.ID) error {
	var title library.Item
	if item, err := h.Library.Item(ctx, user, id); err == nil {
		if title, err = h.hiddenTarget(ctx, user, item); err == nil {
			id = title.ID
		}
	}
	shown, err := h.UserData.Show(ctx, user.ID, id)
	if err == nil && shown && title.ID == id {
		h.userDataChanged(user, []library.Item{title})
	}
	return err
}

// HiddenTitle is a title a user hid, and when they hid it.
type HiddenTitle struct {
	Item library.Item
	At   time.Time
}

// HiddenTitles lists the titles user hid that Polyfin can still describe,
// the latest hidden first.
func (h *Handler) HiddenTitles(ctx context.Context, user accounts.User) ([]HiddenTitle, error) {
	hidden, err := h.UserData.Hidden(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]accounts.ID, 0, len(hidden))
	for _, title := range hidden {
		ids = append(ids, title.ID)
	}
	items, err := h.Library.Items(ctx, user, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[accounts.ID]library.Item, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]HiddenTitle, 0, len(items))
	for _, title := range hidden {
		if item, ok := byID[title.ID]; ok {
			result = append(result, HiddenTitle{Item: item, At: title.At})
		}
	}
	return result, nil
}

// hiddenItems answers GET /Polyfin/UserItems/Hidden: the titles the user
// hid, the latest hidden first, as a listing describes them.
func (h *Handler) hiddenItems(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	start, limit := b.paging(r, -1)
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return
	}
	hidden, err := h.HiddenTitles(r.Context(), user)
	if err != nil {
		h.browseError(w, r, err)
		return
	}
	items := make([]library.Item, 0, len(hidden))
	for _, title := range hidden {
		items = append(items, title.Item)
	}
	h.writeItems(w, r, user, window(items, start, limit), start, len(items))
}

// ResumeItem is an item of Continue Watching, with what the user did with
// it.
type ResumeItem struct {
	Item library.Item
	Data userdata.Data
}

// ResumeItems lists user's Continue Watching as /UserItems/Resume does,
// the hidden titles left out, most recently played first.
func (h *Handler) ResumeItems(ctx context.Context, user accounts.User) ([]ResumeItem, error) {
	entries, err := h.UserData.Resumable(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	items, err := h.resumeItems(ctx, user, entries, resumeKinds)
	if err != nil {
		return nil, err
	}
	data := make(map[accounts.ID]userdata.Data, len(entries))
	for _, e := range entries {
		data[e.Item] = e.Data
	}
	result := make([]ResumeItem, 0, len(items))
	for _, item := range items {
		result = append(result, ResumeItem{Item: item, Data: data[item.ID]})
	}
	return result, nil
}
