package jellyfin

import (
	"context"
	"fmt"
	"slices"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/i18n"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/taste"
)

// Taste profiles.
//
// Each user's taste profile (see package taste) ranks the titles of a Top
// picks for you home row, and reorders the lists Jellyfin derives from a
// user's titles: similar titles, and the Because you watched rows and
// movie recommendations made of them, half by how close they are and half
// by the user's taste; suggestions by the user's taste alone. Below the
// profile's confidence gate, the lists keep their own order, and a Top
// picks row lists the first titles of the user's libraries as their
// catalogs give them, the most popular first. Titles of a genre the user
// dislikes come last, and leave Top picks for you.
//
// A movie's or series' details also start their Taglines with why the
// title is for the user, above the gate and unless they turned it off:
// "87% for you · because you like Thriller". Saving the title's metadata
// leaves it out (see dropTasteTagline).

// tasteProfile returns the user's profile: the zero Profile, below the
// confidence gate, without one, for a request without a user, or when it
// cannot be read, which is logged.
func (h *Handler) tasteProfile(ctx context.Context, user accounts.User) taste.Profile {
	if h.Taste == nil || user.ID == (accounts.ID{}) {
		return taste.Profile{}
	}
	profile, err := h.Taste.Profile(ctx, user.ID)
	if err != nil {
		h.Logger.Warn("A taste profile could not be read", "user", user.ID.String(), "error", err)
		return taste.Profile{}
	}
	return profile
}

// similarTitles lists up to count titles similar to id (see
// library.Service.Similar), blended with the user's taste.
func (h *Handler) similarTitles(ctx context.Context, user accounts.User, id accounts.ID, count int) ([]library.Item, error) {
	similar, err := h.Library.Similar(ctx, user, id, count)
	if err != nil {
		return nil, err
	}
	return taste.Blend(h.tasteProfile(ctx, user), similar, taste.TitleOf), nil
}

// topPicksRow lists, from start, the movies and series of the first page of
// each of the user's libraries, smart lists seen as libraries among them,
// that they did not play, each once: above the confidence gate in the
// order of their taste, else the libraries' first titles first, in turn.
// The titles of a genre they dislike are left out.
func (h *Handler) topPicksRow(ctx context.Context, user accounts.User, start, count int) (library.Page, error) {
	libraries, err := h.suggestionCandidates(ctx, user, func(item library.Item) bool {
		return item.Kind == library.KindMovie || item.Kind == library.KindSeries
	})
	if err != nil {
		return library.Page{}, err
	}
	var pool []library.Item
	for rank := 0; ; rank++ {
		added := false
		for _, titles := range libraries {
			if rank < len(titles) {
				pool, added = append(pool, titles[rank]), true
			}
		}
		if !added {
			break
		}
	}
	state, err := h.userState(ctx, user, pool)
	if err != nil {
		return library.Page{}, err
	}
	pool = slices.DeleteFunc(pool, func(item library.Item) bool { return state.of(item).Played })
	return pageOfItems(taste.Pick(h.tasteProfile(ctx, user), pool, taste.TitleOf), start, count), nil
}

// tasteTagline is the text for a matched title.
var tasteTagline = i18n.Text{
	"%d%% for you · because you like %s",
	"%d\u00a0%% pour vous · parce que vous aimez %s",
	"%d\u00a0%% für dich · weil du %s magst",
	"%d\u00a0%% para usted · porque le gusta %s",
	"%d%% per te · perché ti piace %s",
	"%d%% para você · porque você gosta de %s",
}

// tasteMatch is the text for a matched title the user likes nothing of
// in particular.
var tasteMatch = i18n.Text{
	"%d%% for you",
	"%d\u00a0%% pour vous",
	"%d\u00a0%% für dich",
	"%d\u00a0%% para usted",
	"%d%% per te",
	"%d%% para você",
}

// tasteTaglineOf is why item is for the user, in their language, "" when
// it is not a movie or series, below the confidence gate, when it does
// not match, and when the user turned taglines off.
func (h *Handler) tasteTaglineOf(ctx context.Context, user accounts.User, item library.Item) string {
	if h.Taste == nil || user.ID == (accounts.ID{}) || item.Kind != library.KindMovie && item.Kind != library.KindSeries {
		return ""
	}
	match, ok := h.tasteProfile(ctx, user).Match(taste.TitleOf(item))
	if !ok {
		return ""
	}
	on, err := h.Taste.Taglines(ctx, user.ID)
	if err != nil || !on {
		return ""
	}
	language := i18n.Match(user.MetadataLanguage)
	if language == "" {
		language = h.Accounts.Settings().Language
	}
	if match.Reason == "" {
		return fmt.Sprintf(tasteMatch.In(language), match.Percent)
	}
	return fmt.Sprintf(tasteTagline.In(language), match.Percent, match.Reason)
}

// addTasteTagline puts why item is for the user before the taglines of
// its details, when they ask for them.
func (h *Handler) addTasteTagline(ctx context.Context, user accounts.User, dto *BaseItemDto, item library.Item) {
	if dto.Taglines == nil {
		return
	}
	if tagline := h.tasteTaglineOf(ctx, user, item); tagline != "" {
		dto.Taglines = new(append([]string{tagline}, *dto.Taglines...))
	}
}

// dropTasteTagline leaves out of the taglines saved for the item id the
// one its details show the user, which the metadata editor sends back
// with the others.
func (h *Handler) dropTasteTagline(ctx context.Context, user accounts.User, id accounts.ID, taglines []string) []string {
	if h.Taste == nil || len(taglines) == 0 {
		return taglines
	}
	item, err := h.Library.Item(ctx, user, id)
	if err != nil {
		return taglines
	}
	tagline := h.tasteTaglineOf(ctx, user, item)
	if tagline == "" {
		return taglines
	}
	return slices.DeleteFunc(slices.Clone(taglines), func(t string) bool { return t == tagline })
}
