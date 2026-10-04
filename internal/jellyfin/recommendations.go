package jellyfin

import (
	"net/http"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// RecommendationDto is a category of Jellyfin's movie recommendations.
type RecommendationDto struct {
	Items              []BaseItemDto
	RecommendationType string
	BaselineItemName   string
	CategoryId         string
}

const (
	// recentMovies is how many of the movies a user played last Jellyfin
	// recommends from.
	recentMovies = 7
	// recommendationLookups bounds the lists of similar titles read at once.
	recommendationLookups = 4
)

// movieRecommendations answers Jellyfin's movie recommendations with the
// categories jellyfin-web titles "Because you watched": for each of the
// movies the user played last, the titles similar to it, through the
// similar titles of title pages (see similarItems), without those the user
// played. Jellyfin adds categories for the titles a user likes and for the
// people of recent movies, which would each cost a search of the addons;
// Polyfin leaves them out. With similar titles turned off in the settings,
// there are no categories, and no addon is asked. parentId, which narrows
// the movies played to a library's, is read and left unused: a title
// belongs to whichever library lists it.
func (h *Handler) movieRecommendations(w http.ResponseWriter, r *http.Request) {
	b := bindErrors{}
	b.guid(r, "parentId")
	categoryLimit, categorySet := b.int32(r, "categoryLimit")
	itemLimit, itemSet := b.int32(r, "itemLimit")
	user, ok := h.viewer(w, r, b, unknownListingUser)
	if !ok {
		return
	}
	if !categorySet {
		categoryLimit = 5
	}
	if !itemSet {
		itemLimit = 8
	}
	result := []RecommendationDto{}
	if !h.Accounts.Settings().SimilarTitles || categoryLimit <= 0 || itemLimit <= 0 {
		writeJSON(w, http.StatusOK, result)
		return
	}
	baselines, err := h.recentlyPlayedMovies(r, user, min(categoryLimit, recentMovies))
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	similar := make([][]library.Item, len(baselines))
	var group errgroup.Group
	group.SetLimit(recommendationLookups)
	for i, movie := range baselines {
		group.Go(func() error {
			// Twice the limit leaves room for the played titles left out.
			found, err := h.Library.Similar(r.Context(), user, movie.ID, 2*itemLimit)
			if err == nil {
				similar[i] = found
			}
			return nil
		})
	}
	_ = group.Wait()
	if r.Context().Err() != nil {
		return
	}
	var all []library.Item
	for _, items := range similar {
		all = append(all, items...)
	}
	state, err := h.userState(r.Context(), user, all)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	fields := requestedFields(r)
	for i, movie := range baselines {
		items := slices.DeleteFunc(similar[i], func(item library.Item) bool { return state.of(item).Played })
		if len(items) == 0 {
			continue
		}
		result = append(result, RecommendationDto{
			Items:              h.listDtos(r, user, items[:min(itemLimit, len(items))], fields, state),
			RecommendationType: "SimilarToRecentlyPlayed",
			BaselineItemName:   movie.Name,
			CategoryId:         movie.ID.String(),
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// recentlyPlayedMovies lists up to count of the movies the user played
// last, most recent first, among those they still reach.
func (h *Handler) recentlyPlayedMovies(r *http.Request, user accounts.User, count int) ([]library.Item, error) {
	played, err := h.UserData.Played(r.Context(), user.ID)
	if err != nil {
		return nil, err
	}
	var ids []accounts.ID
	for _, entry := range played {
		// Episodes name their series; movies have none.
		if entry.Series == (accounts.ID{}) {
			ids = append(ids, entry.Item)
		}
	}
	var movies []library.Item
	// Items of other kinds, or no longer reached, are skipped: a few more
	// than needed are described at a time.
	for len(ids) > 0 && len(movies) < count {
		batch := ids[:min(len(ids), 2*count)]
		ids = ids[len(batch):]
		items, err := h.Library.Items(r.Context(), user, batch)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Kind == library.KindMovie && len(movies) < count {
				movies = append(movies, item)
			}
		}
	}
	return movies, nil
}
