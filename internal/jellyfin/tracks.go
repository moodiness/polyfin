package jellyfin

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/playback"
)

// trackPreferences are the settings of a user's configuration that choose
// the tracks a version plays unless the app asks for others. Languages are
// compared whichever ISO 639 code names them ("fre", "fra" and "fr" are
// the same language).
type trackPreferences struct {
	audioLanguage string
	// playDefaultAudio plays the audio track flagged default whatever its
	// language.
	playDefaultAudio bool
	subtitleLanguage string
	// subtitleMode is one of subtitleModes.
	subtitleMode string
}

func newTrackPreferences(configuration UserConfiguration) trackPreferences {
	preferences := trackPreferences{
		playDefaultAudio: configuration.PlayDefaultAudioTrack,
		subtitleLanguage: configuration.SubtitleLanguagePreference,
		subtitleMode:     configuration.SubtitleMode,
	}
	if configuration.AudioLanguagePreference != nil {
		preferences.audioLanguage = *configuration.AudioLanguagePreference
	}
	return preferences
}

// trackPreferences returns the track preferences of user. When they cannot
// be read, titles are still described and played, with a new user's
// default tracks.
func (h *Handler) trackPreferences(ctx context.Context, user accounts.User) trackPreferences {
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The configuration of a user could not be read", "error", err)
		}
		configuration = defaultUserConfiguration()
	}
	return newTrackPreferences(configuration)
}

// defaultAudio is the audio track a decision plays when none is asked:
// the one flagged default, else the first. nil when there is none.
func defaultAudio(streams []playback.MediaStream) *int {
	return trackPreferences{playDefaultAudio: true}.audio(streams)
}

// audio is the audio track played unless the app asks for another, as
// Jellyfin chooses it: one in the preferred language, then the one flagged
// default, then the first; a user who plays the default track whatever its
// language gets the one flagged default first. nil when there is none.
func (t trackPreferences) audio(streams []playback.MediaStream) *int {
	rank := func(stream playback.MediaStream) int {
		preferred, flagged := 0, 0
		if localization.SameLanguage(stream.Language, t.audioLanguage) {
			preferred = 1
		}
		if stream.IsDefault {
			flagged = 1
		}
		if t.playDefaultAudio {
			return flagged<<1 | preferred
		}
		return preferred<<1 | flagged
	}
	best, bestRank := -1, -1
	for _, stream := range streams {
		if stream.Type != "Audio" {
			continue
		}
		if r := rank(stream); r > bestRank {
			best, bestRank = stream.Index, r
		}
	}
	if best < 0 {
		return nil
	}
	return new(best)
}

// subtitle is the subtitle shown unless the app asks for another, -1 for
// none, following the subtitle mode as Jellyfin does:
//
//   - Default shows a track flagged forced or default, a forced one in the
//     language of the audio first.
//   - Always shows a full (not forced) track in the preferred language, else
//     what Default shows.
//   - OnlyForced shows a forced track in the preferred language or of no
//     stated language; Polyfin also takes any forced track when no
//     language is preferred, where Jellyfin would take none.
//   - Smart shows a track in the preferred language when the audio is in
//     another one, else what Default shows; when the audio is in the
//     preferred language, only a forced track.
//   - None shows none.
//
// Polyfin differs from Jellyfin with subtitle files: Jellyfin shows the
// files next to a video before any embedded track, as somebody put them
// there for it. Addons find files for the title, not for the version, so
// Polyfin offers them but shows one only when it is in the preferred
// language (Always, Smart) and no embedded track is. Among equal tracks, a
// text one in the same language is preferred to an image one, as an app
// may only take an image one burned into converted video. audio is the
// audio track that plays. The subtitle is absent when there are no
// subtitles at all, as Jellyfin reports it.
func (t trackPreferences) subtitle(streams []playback.MediaStream, audio *int) *int {
	audioLanguage := ""
	var subtitles []playback.MediaStream
	for _, stream := range streams {
		switch {
		case stream.Type == "Subtitle":
			subtitles = append(subtitles, stream)
		case stream.Type == "Audio" && audio != nil && stream.Index == *audio:
			audioLanguage = stream.Language
		}
	}
	if len(subtitles) == 0 {
		return nil
	}
	preferred := func(s playback.MediaStream) bool { return localization.SameLanguage(s.Language, t.subtitleLanguage) }
	flagged := func(s playback.MediaStream) bool { return !s.IsExternal && (s.IsForced || s.IsDefault) }
	forced := func(s playback.MediaStream) bool { return !s.IsExternal && s.IsForced }
	best := func(keep func(playback.MediaStream) bool) *playback.MediaStream {
		return t.bestSubtitle(subtitles, audioLanguage, keep)
	}
	var choice *playback.MediaStream
	switch t.subtitleMode {
	case "None":
	case "Always":
		if choice = best(func(s playback.MediaStream) bool { return !s.IsForced && preferred(s) }); choice == nil {
			choice = best(flagged)
		}
	case "OnlyForced":
		choice = best(func(s playback.MediaStream) bool {
			return forced(s) && (t.subtitleLanguage == "" || preferred(s) || undeterminedLanguage(s.Language))
		})
	case "Smart":
		if localization.SameLanguage(audioLanguage, t.subtitleLanguage) {
			choice = best(forced)
		} else if choice = best(preferred); choice == nil {
			choice = best(flagged)
		}
	default:
		choice = best(flagged)
	}
	if choice == nil {
		return new(-1)
	}
	return new(choice.Index)
}

// bestSubtitle returns the first of the subtitles keep accepts, ranked:
// embedded before files, then forced in the audio's language, forced,
// flagged default, and in the preferred language; nil when keep accepts
// none. An image track gives way to a text track in the same language,
// equally forced, that keep accepts too.
func (t trackPreferences) bestSubtitle(subtitles []playback.MediaStream, audioLanguage string, keep func(playback.MediaStream) bool) *playback.MediaStream {
	rank := func(s playback.MediaStream) int {
		r := 0
		for _, criterion := range [...]bool{
			!s.IsExternal,
			s.IsForced && localization.SameLanguage(s.Language, audioLanguage),
			s.IsForced,
			s.IsDefault,
			localization.SameLanguage(s.Language, t.subtitleLanguage),
		} {
			r <<= 1
			if criterion {
				r |= 1
			}
		}
		return r
	}
	var best *playback.MediaStream
	bestRank := -1
	for i, s := range subtitles {
		if !keep(s) {
			continue
		}
		if r := rank(s); r > bestRank {
			best, bestRank = &subtitles[i], r
		}
	}
	if best == nil || best.IsTextSubtitleStream {
		return best
	}
	for i, s := range subtitles {
		if s.IsTextSubtitleStream && s.IsForced == best.IsForced && keep(s) &&
			(s.Language == best.Language || localization.SameLanguage(s.Language, best.Language)) {
			return &subtitles[i]
		}
	}
	return best
}

// undeterminedLanguage reports whether a track states no language.
func undeterminedLanguage(language string) bool {
	return language == "" || localization.LanguageCode(language) == "und"
}
