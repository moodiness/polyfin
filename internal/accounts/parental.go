package accounts

import (
	"errors"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/localization"
)

// ErrInvalidParentalControl reports a kind of item that is not one of
// UnratedKinds.
var ErrInvalidParentalControl = errors.New("invalid parental control")

// UnratedKinds are the kinds of items Jellyfin's user policy can hide when
// they have no rating (its UnratedItem names), in its order. Polyfin's
// movies are Movie, and its series, with their seasons and episodes,
// Series; the others are kept as apps set them.
var UnratedKinds = []string{"Movie", "Trailer", "Series", "Music", "Book", "LiveTvChannel", "LiveTvProgram", "ChannelContent", "Other"}

// ParentalControl limits the titles a user reaches by their rating, as
// Jellyfin's user policy does. The zero value limits nothing.
type ParentalControl struct {
	// MaxRating is the highest rating score the user may reach (see
	// localization.Score); nil allows every rating.
	MaxRating *int
	// MaxSubRating is the highest subscore allowed at MaxRating; nil
	// allows every subscore.
	MaxSubRating *int
	// BlockUnrated lists the kinds of items (UnratedKinds) hidden from the
	// user when they have no rating.
	BlockUnrated []string
}

// Restricted reports whether the control hides anything.
func (p ParentalControl) Restricted() bool {
	return p.MaxRating != nil || len(p.BlockUnrated) > 0
}

// Allows reports whether the user may reach an item of kind, one of
// UnratedKinds, rated rating, as Jellyfin decides: a title without a known
// rating is hidden only when its kind is blocked; a rated one is allowed
// below the highest score, and at that score up to the highest subscore.
func (p ParentalControl) Allows(kind, rating string) bool {
	score, rated := localization.RatingScore(rating)
	if !rated {
		return !slices.Contains(p.BlockUnrated, kind)
	}
	if p.MaxRating == nil {
		return true
	}
	if score.Score != *p.MaxRating {
		return score.Score < *p.MaxRating
	}
	if p.MaxSubRating == nil {
		return true
	}
	sub := 0
	if score.SubScore != nil {
		sub = *score.SubScore
	}
	return sub <= *p.MaxSubRating
}

// normalized names the blocked kinds as UnratedKinds does, in any case on
// input, each once and in its order. A subscore without a highest score
// limits nothing and is dropped.
func (p ParentalControl) normalized() (ParentalControl, error) {
	result := ParentalControl{MaxRating: p.MaxRating, BlockUnrated: []string{}}
	if p.MaxRating != nil {
		result.MaxSubRating = p.MaxSubRating
	}
	for _, kind := range p.BlockUnrated {
		i := slices.IndexFunc(UnratedKinds, func(known string) bool { return strings.EqualFold(known, kind) })
		if i < 0 {
			return ParentalControl{}, ErrInvalidParentalControl
		}
		if !slices.Contains(result.BlockUnrated, UnratedKinds[i]) {
			result.BlockUnrated = append(result.BlockUnrated, UnratedKinds[i])
		}
	}
	slices.SortFunc(result.BlockUnrated, func(a, b string) int {
		return slices.Index(UnratedKinds, a) - slices.Index(UnratedKinds, b)
	})
	return result, nil
}
