package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/userdata"
)

func (h *Handler) userDataRoutes(rt *router) {
	signedIn := func(method, pattern string, handler http.HandlerFunc) {
		rt.handle(method, pattern, h.authenticated(handler))
	}
	for _, played := range []string{"/UserPlayedItems/{itemId}", "/Users/{userId}/PlayedItems/{itemId}"} {
		signedIn(http.MethodPost, played, h.markPlayed(true))
		signedIn(http.MethodDelete, played, h.markPlayed(false))
	}
	for _, favorite := range []string{"/UserFavoriteItems/{itemId}", "/Users/{userId}/FavoriteItems/{itemId}"} {
		signedIn(http.MethodPost, favorite, h.markFavorite(true))
		signedIn(http.MethodDelete, favorite, h.markFavorite(false))
	}
	for _, item := range []string{"/UserItems/{itemId}", "/Users/{userId}/Items/{itemId}"} {
		signedIn(http.MethodPost, item+"/Rating", h.rate(true))
		signedIn(http.MethodDelete, item+"/Rating", h.rate(false))
		signedIn(http.MethodGet, item+"/UserData", h.userData)
		signedIn(http.MethodPost, item+"/UserData", h.updateUserData)
	}
	signedIn(http.MethodGet, "/UserItems/Resume", h.resume)
	signedIn(http.MethodGet, "/Users/{userId}/Items/Resume", h.resume)
	signedIn(http.MethodGet, "/Shows/NextUp", h.nextUp)
	signedIn(http.MethodGet, "/Shows/Upcoming", h.upcoming)
}

// userState is what a user did with the items of one response.
type userState struct {
	data map[accounts.ID]userdata.Data
	// counts are the played and resumable episodes under series and
	// seasons.
	counts map[accounts.ID]userdata.Counts
	// contents describes the episodes of series listed without them that
	// the user started, so that their progress shows.
	contents map[accounts.ID]*library.Contents
}

// userState loads what user did with items.
func (h *Handler) userState(ctx context.Context, user accounts.User, items []library.Item) (userState, error) {
	ids := make([]accounts.ID, 0, len(items))
	var parents []accounts.ID
	for _, item := range items {
		ids = append(ids, item.ID)
		if item.Kind == library.KindSeries || item.Kind == library.KindSeason {
			parents = append(parents, item.ID)
		}
	}
	data, err := h.UserData.Get(ctx, user.ID, ids)
	if err != nil {
		return userState{}, err
	}
	counts, err := h.UserData.EpisodeCounts(ctx, user.ID, parents)
	if err != nil {
		return userState{}, err
	}
	state := userState{data: data, counts: counts, contents: map[accounts.ID]*library.Contents{}}
	var incomplete []accounts.ID
	for _, item := range items {
		if item.Kind == library.KindSeries && item.Contents == nil && counts[item.ID].Played > 0 {
			incomplete = append(incomplete, item.ID)
		}
	}
	if len(incomplete) > 0 {
		complete, err := h.Library.Items(ctx, user, incomplete)
		if err != nil {
			return userState{}, err
		}
		for _, series := range complete {
			state.contents[series.ID] = series.Contents
		}
	}
	return state, nil
}

// of describes what the user did with item. Series and seasons count
// their played episodes; a partly watched episode counts as unplayed.
// Other folders only show that they are not played.
func (s userState) of(item library.Item) UserItemData {
	result := UserItemData{Key: hyphenated(item.ID), ItemId: item.ID.String()}
	data := s.data[item.ID]
	result.IsFavorite, result.Rating, result.Likes = data.Favorite, data.Rating, data.Likes()
	switch item.Kind {
	case library.KindMovie, library.KindEpisode, library.KindRecording:
		result.PlaybackPositionTicks = int64(data.Position / 100)
		result.PlayCount = data.PlayCount
		result.Played = data.Played
		if data.LastPlayed != nil {
			result.LastPlayedDate = new(Time(*data.LastPlayed))
		}
		if percent, ok := data.PlayedPercentage(); ok {
			result.PlayedPercentage = new(percent)
		}
	case library.KindSeries, library.KindSeason, library.KindCollection:
		result.PlayedPercentage = new(0.0)
		contents := item.Contents
		if contents == nil {
			contents = s.contents[item.ID]
		}
		if contents == nil {
			break
		}
		played := min(s.counts[item.ID].Played, contents.Released)
		result.UnplayedItemCount = new(contents.Released - played)
		if contents.Released > 0 {
			result.PlayedPercentage = new(float64(played) / float64(contents.Released) * 100)
			result.Played = played == contents.Released
		}
	}
	return result
}

// resumable reports whether the user can resume item: a movie or episode
// with a resume point, or a series or season with such an episode.
func (s userState) resumable(item library.Item) bool {
	if item.Kind == library.KindSeries || item.Kind == library.KindSeason {
		return s.counts[item.ID].Resumable > 0
	}
	return s.data[item.ID].Position > 0
}

// itemData is what user did with one item.
func (h *Handler) itemData(ctx context.Context, user accounts.User, item library.Item) (UserItemData, error) {
	state, err := h.userState(ctx, user, []library.Item{item})
	if err != nil {
		return UserItemData{}, err
	}
	return state.of(item), nil
}

// stored names an item for the user data store: episodes name their series
// and season.
func stored(item library.Item) userdata.Item {
	if item.Kind == library.KindEpisode {
		return userdata.Item{ID: item.ID, Series: item.SeriesID, Season: item.SeasonID}
	}
	return userdata.Item{ID: item.ID}
}

// userDataItem binds the item a user data endpoint acts on and the user it
// acts for. A version stands for its title. ok is false once w was
// answered.
func (h *Handler) userDataItem(w http.ResponseWriter, r *http.Request, b bindErrors) (accounts.User, library.Item, bool) {
	id := b.pathID(r, "itemId")
	user, ok := h.viewer(w, r, b, notFoundProblem)
	if !ok {
		return accounts.User{}, library.Item{}, false
	}
	item, err := h.Library.Item(r.Context(), user, id)
	if errors.Is(err, library.ErrNotFound) {
		item, err = h.title(r.Context(), user, id)
	}
	if err != nil {
		h.browseError(w, r, err)
		return accounts.User{}, library.Item{}, false
	}
	return user, item, true
}

// change applies change to the data of items and answers what the user did
// with item afterwards. The user's apps that keep a socket open get the new
// data of item and items.
func (h *Handler) change(w http.ResponseWriter, r *http.Request, user accounts.User, item library.Item, items []library.Item, change func(*userdata.Data)) {
	refs := make([]userdata.Item, 0, len(items))
	for _, target := range items {
		refs = append(refs, stored(target))
	}
	if _, err := h.UserData.Change(r.Context(), user.ID, refs, change); err != nil {
		h.internalError(w, r, err)
		return
	}
	h.userDataChanged(user, append([]library.Item{item}, items...))
	data, err := h.itemData(r.Context(), user, item)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// markPlayed marks an item played or unplayed: a movie or an episode
// itself, or every released episode of a season or series.
func (h *Handler) markPlayed(played bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := bindErrors{}
		var date *time.Time
		if raw := strings.TrimSpace(query(r, "datePlayed")); raw != "" {
			if parsed, ok := parseTime(raw); ok {
				date = &parsed
			} else {
				b.add("datePlayed", notValid(raw))
			}
		}
		user, item, ok := h.userDataItem(w, r, b)
		if !ok {
			return
		}
		targets, err := h.markTargets(r.Context(), user, item)
		if err != nil {
			h.browseError(w, r, err)
			return
		}
		now := time.Now().UTC()
		h.change(w, r, user, item, targets, func(d *userdata.Data) {
			if played {
				d.MarkPlayed(date, now)
			} else {
				d.MarkUnplayed()
			}
		})
	}
}

// markTargets lists the items a played mark applies to.
func (h *Handler) markTargets(ctx context.Context, user accounts.User, item library.Item) ([]library.Item, error) {
	var episodes []library.Item
	var err error
	switch item.Kind {
	case library.KindMovie, library.KindEpisode, library.KindRecording:
		return []library.Item{item}, nil
	case library.KindSeries:
		episodes, err = h.Library.Episodes(ctx, user, item.ID, nil)
	case library.KindSeason:
		episodes, err = h.Library.Episodes(ctx, user, item.SeriesID, &item.ID)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	released := episodes[:0]
	for _, episode := range episodes {
		if episode.Available {
			released = append(released, episode)
		}
	}
	return released, nil
}

// markFavorite marks an item, of any kind, as a favorite or not. Favorites
// do not spread to what the item contains.
func (h *Handler) markFavorite(favorite bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, item, ok := h.userDataItem(w, r, bindErrors{})
		if !ok {
			return
		}
		h.change(w, r, user, item, []library.Item{item}, func(d *userdata.Data) { d.Favorite = favorite })
	}
}

// rate records that the user likes or dislikes an item. Posting without
// likes, like deleting, removes the rating.
func (h *Handler) rate(post bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b := bindErrors{}
		likes, set := b.bool(r, "likes")
		user, item, ok := h.userDataItem(w, r, b)
		if !ok {
			return
		}
		h.change(w, r, user, item, []library.Item{item}, func(d *userdata.Data) {
			d.Rating = nil
			if post && set {
				d.Like(likes)
			}
		})
	}
}

func (h *Handler) userData(w http.ResponseWriter, r *http.Request) {
	user, item, ok := h.userDataItem(w, r, bindErrors{})
	if !ok {
		return
	}
	data, err := h.itemData(r.Context(), user, item)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

// userDataUpdate is Jellyfin's UpdateUserItemDataDto: the fields sent, and
// not null, replace the stored ones as they are.
type userDataUpdate struct {
	Rating                *float64
	PlaybackPositionTicks *int64
	PlayCount             *int
	IsFavorite            *bool
	Likes                 *bool
	LastPlayedDate        *Time
	Played                *bool
}

// updateUserData replaces what an app sends, without the rules playback
// follows: apps use it to upload what was played offline.
func (h *Handler) updateUserData(w http.ResponseWriter, r *http.Request) {
	if !jsonContent(r.Header.Get("Content-Type")) {
		unsupportedMediaTypeProblem(w)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		processingError(w, http.StatusRequestEntityTooLarge)
		return
	}
	if len(bytes.TrimSpace(body)) == 0 {
		validationProblem(w, map[string][]string{"": {"A non-empty request body is required."}, "userDataDto": {"The userDataDto field is required."}})
		return
	}
	var update userDataUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		validationProblem(w, map[string][]string{"$": {"The JSON value could not be converted."}, "userDataDto": {"The userDataDto field is required."}})
		return
	}
	user, item, ok := h.userDataItem(w, r, bindErrors{})
	if !ok {
		return
	}
	h.change(w, r, user, item, []library.Item{item}, func(d *userdata.Data) {
		if update.PlaybackPositionTicks != nil {
			d.Position = time.Duration(max(*update.PlaybackPositionTicks, 0)) * 100
			if d.Runtime <= 0 {
				// The resume point is measured against the title's runtime
				// until a version of it is played.
				d.Runtime = item.Runtime
			}
		}
		if update.PlayCount != nil {
			d.PlayCount = max(*update.PlayCount, 0)
		}
		if update.IsFavorite != nil {
			d.Favorite = *update.IsFavorite
		}
		if update.Likes != nil {
			d.Like(*update.Likes)
		}
		if update.Rating != nil {
			d.Rating = new(min(max(*update.Rating, 0), 10))
		}
		if update.LastPlayedDate != nil {
			d.LastPlayed = new(time.Time(*update.LastPlayedDate).UTC())
		}
		if update.Played != nil {
			d.Played = *update.Played
		}
	})
}

// playbackEvent is the kind of a playback report.
type playbackEvent int

const (
	playbackStarted playbackEvent = iota
	playbackProgressed
	playbackStopped
)

// track applies a playback report to the user's data of the title played,
// as Jellyfin does: a start counts a play, positions move the resume point
// or mark the title played, and a stop without a position plays it
// through. playing is what the device was known to play before the report.
func (h *Handler) track(ctx context.Context, user accounts.User, device string, deviceID accounts.ID, event playbackEvent, state playback.PlayState, positionKnown bool, playing playback.NowPlaying) {
	id, mediaSource := state.Item, state.MediaSourceID
	if id == (accounts.ID{}) {
		id = playing.Item
	}
	if mediaSource == "" && playing.Item == id {
		mediaSource = playing.MediaSourceID
	}
	if id == (accounts.ID{}) {
		return
	}
	item, err := h.title(ctx, user, id)
	if err != nil {
		if !errors.Is(err, library.ErrNotFound) {
			h.Logger.Warn("A playback report could not be recorded", "error", err)
		}
		return
	}
	h.recordPlayback(ctx, user, device, event, item)
	runtime := h.playedRuntime(ctx, user, item, mediaSource)
	if event != playbackStopped && (positionKnown || event == playbackStarted) {
		h.prepareNearTheEnd(user, deviceID, item, runtime, state.Position)
	}
	if event == playbackStarted {
		h.queueImages(ctx, user, deviceID, item, mediaSource)
	}
	now := time.Now().UTC()
	settings := h.Accounts.Settings()
	thresholds := userdata.Thresholds{Resume: settings.ResumePercent, Played: settings.PlayedPercent}
	_, err = h.UserData.Change(ctx, user.ID, []userdata.Item{stored(item)}, func(d *userdata.Data) {
		switch {
		case event == playbackStarted:
			d.Start(now)
		case positionKnown:
			d.Reach(state.Position, runtime, thresholds)
		case event == playbackStopped:
			d.Finish()
		}
	})
	if err != nil {
		h.Logger.Warn("A playback report could not be recorded", "error", err)
		return
	}
	// Like Jellyfin, apps are not told of every position a player reports:
	// starts and stops are enough for what they show.
	if event != playbackProgressed {
		h.userDataChanged(user, []library.Item{item})
	}
}

// playedRuntime is the runtime of the version a report names, as analyzed,
// or the title's when the version is unknown.
func (h *Handler) playedRuntime(ctx context.Context, user accounts.User, item library.Item, mediaSource string) time.Duration {
	key := item.ID.String() + "|" + mediaSource
	if runtime, ok := h.runtimes.Get(key); ok {
		return runtime
	}
	runtime := item.Runtime
	if id, ok := parseGUID(mediaSource); ok {
		if version, err := h.version(ctx, user, item, id); err == nil {
			if analysis, ok := h.Playback.Analyzed(ctx, version.ID); ok && analysis.Duration > 0 {
				runtime = analysis.Duration
			}
		}
	}
	h.runtimes.Put(key, runtime)
	return runtime
}
