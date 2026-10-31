package jellyfin

import (
	"context"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/preferences"
)

// trackPreferences are the settings of a user's configuration that choose
// the tracks a version plays unless the app asks for others. Languages are
// compared whichever ISO 639 code names them ("fre", "fra" and "fr" are
// the same language).
type trackPreferences struct {
	audioLanguage string
	// audioRegion is the regional variant of audioLanguage preferred, of
	// localization.AudioVariants, empty for none (see
	// accounts.User.AudioRegion).
	audioRegion string
	// region is the user's audio region whatever their audio language,
	// which ranks the tracks in a language they chose on a title (see
	// rememberedAudio).
	region string
	// playDefaultAudio plays the audio track flagged default whatever its
	// language.
	playDefaultAudio bool
	subtitleLanguage string
	// subtitleMode is one of subtitleModes.
	subtitleMode string
	// remembered is what the user chose of the title's tracks, which comes
	// before their languages, for the kinds of selection their
	// configuration remembers (see titleTracks).
	remembered                       preferences.TrackChoice
	rememberAudio, rememberSubtitles bool
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
// default tracks. Where the user chose no audio or subtitle language,
// the server's default one applies (Settings › Playback), with the user's
// own subtitle mode; their configuration itself keeps it empty, so that an
// app saving it back never makes the default their own choice.
//
// The server's audio language ranks above the track flagged default: the
// administrator chose it for users who chose none, and
// PlayDefaultAudioTrack, on for every new user, is an app default rather
// than their choice. A user's own language keeps Jellyfin's rule, the
// flagged track first while PlayDefaultAudioTrack is on.
//
// The user's audio region, a Polyfin setting as Jellyfin's preference has
// no region, refines the audio language when it is the region's
// language; for a user who chose no audio language, it names theirs,
// ranking above the flagged track as the server's does.
func (h *Handler) trackPreferences(ctx context.Context, user accounts.User) trackPreferences {
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The configuration of a user could not be read", "error", err)
		}
		configuration = defaultUserConfiguration()
	}
	preferences := newTrackPreferences(configuration)
	preferences.region = user.AudioRegion
	if user.AudioRegion != "" {
		language := localization.VariantLanguage(user.AudioRegion)
		if preferences.audioLanguage == "" {
			preferences.audioLanguage = language
			preferences.playDefaultAudio = false
		}
		if localization.SameLanguage(preferences.audioLanguage, language) {
			preferences.audioRegion = user.AudioRegion
		}
	}
	settings := h.Accounts.Settings()
	if preferences.audioLanguage == "" && settings.DefaultAudioLanguage != "" {
		preferences.audioLanguage = settings.DefaultAudioLanguage
		preferences.playDefaultAudio = false
	}
	if preferences.subtitleLanguage == "" {
		preferences.subtitleLanguage = settings.DefaultSubtitleLanguage
	}
	return preferences
}

// defaultAudio is the audio track a decision plays when none is asked:
// the one flagged default, else the first. nil when there is none.
func defaultAudio(streams []playback.MediaStream) *int {
	return trackPreferences{playDefaultAudio: true}.audio(streams)
}

// audio is the audio track played unless the app asks for another, as
// Jellyfin chooses it: one in the preferred language, then the one flagged
// default, then the first; a user who plays the default track whatever its
// language gets the one flagged default first. Among the tracks in the
// preferred language, those of the preferred region come first, then
// those of no known region, then those of another. nil when there is
// none. A language the user chose on the title comes before all of these
// (see rememberedAudio), as their subtitle choice does in subtitle (see
// rememberedSubtitle).
func (t trackPreferences) audio(streams []playback.MediaStream) *int {
	if remembered := t.rememberedAudio(streams); remembered != nil {
		return remembered
	}
	rank := func(stream playback.MediaStream) int {
		preferred, flagged := t.audioRank(stream), 0
		if stream.IsDefault {
			flagged = 1
		}
		if t.playDefaultAudio {
			return flagged<<2 | preferred
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

// audioRank ranks an audio track for the preferred language and region,
// higher first, in 2 bits: 0 in another language; in the language, 3 in
// the region or without a region preferred, 2 of no known region (see
// localization.AudioVariant), 1 of another region.
func (t trackPreferences) audioRank(stream playback.MediaStream) int {
	if !localization.SameLanguage(stream.Language, t.audioLanguage) {
		return 0
	}
	if t.audioRegion == "" {
		return 3
	}
	switch localization.AudioVariant(stream.Language, stream.Title) {
	case t.audioRegion:
		return 3
	case "":
		return 2
	}
	return 1
}

// subtitle is the subtitle shown unless the app asks for another, -1 for
// none, following the subtitle mode as Jellyfin 12.2 does, with tracks
// ranked as subtitleRank ranks them:
//
//   - Default shows the first track flagged default or forced.
//   - Always shows the first full (not forced) track in the preferred
//     language, else what OnlyForced shows.
//   - OnlyForced shows a forced track in the preferred language, else one
//     of no stated language.
//   - Smart shows the first track in the preferred language when the audio
//     is in another one, and none when no track is; when the audio is in
//     the preferred language, what OnlyForced shows.
//   - None shows none.
//
// When no language is preferred, every track counts as in the preferred
// language, as in Jellyfin.
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
//
// The text copy of a picture subtitle (see playback.TextCopy) counts as
// its original: the original is chosen, and gives way to its copy only on
// an app that would need it burned in (see Handler.trackDecision).
func (t trackPreferences) subtitle(streams []playback.MediaStream, audio *int) *int {
	audioLanguage := ""
	var subtitles []playback.MediaStream
	for _, stream := range streams {
		switch {
		case stream.Type == "Subtitle" && stream.CopyOf == nil:
			subtitles = append(subtitles, stream)
		case stream.Type == "Audio" && audio != nil && stream.Index == *audio:
			audioLanguage = stream.Language
		}
	}
	if len(subtitles) == 0 {
		return nil
	}
	if remembered := t.rememberedSubtitle(subtitles, audioLanguage); remembered != nil {
		return remembered
	}
	ranked := func(keep func(playback.MediaStream) bool) *playback.MediaStream {
		return bestSubtitle(subtitles, func(s playback.MediaStream) int {
			if !keep(s) {
				return -1
			}
			return t.subtitleRank(s, audioLanguage)
		})
	}
	// onlyForced ranks forced tracks in the preferred language first, then
	// those of no stated language.
	onlyForced := func() *playback.MediaStream {
		return bestSubtitle(subtitles, func(s playback.MediaStream) int {
			matches, undetermined := t.matches(s), undeterminedLanguage(s.Language)
			if s.IsExternal || !s.IsForced || !matches && !undetermined {
				return -1
			}
			r := t.subtitleRank(s, audioLanguage)
			if undetermined {
				r |= 1 << 7
			}
			if matches {
				r |= 1 << 8
			}
			return r
		})
	}
	// shown leaves out the files in another language than the preferred one.
	shown := func(s playback.MediaStream) bool {
		return !s.IsExternal || localization.SameLanguage(s.Language, t.subtitleLanguage)
	}
	var choice *playback.MediaStream
	switch t.subtitleMode {
	case "None":
	case "Always":
		if choice = ranked(func(s playback.MediaStream) bool { return !s.IsForced && t.matches(s) && shown(s) }); choice == nil {
			choice = onlyForced()
		}
	case "OnlyForced":
		choice = onlyForced()
	case "Smart":
		if localization.SameLanguage(audioLanguage, t.subtitleLanguage) {
			choice = onlyForced()
		} else {
			choice = ranked(func(s playback.MediaStream) bool { return t.matches(s) && shown(s) })
		}
	default:
		choice = ranked(func(s playback.MediaStream) bool { return !s.IsExternal && (s.IsForced || s.IsDefault) })
	}
	if choice == nil {
		return new(-1)
	}
	return new(choice.Index)
}

// matches reports whether a subtitle counts as in the preferred language:
// every one does when no language is preferred.
func (t trackPreferences) matches(s playback.MediaStream) bool {
	return t.subtitleLanguage == "" || localization.SameLanguage(s.Language, t.subtitleLanguage)
}

// subtitleRank places a subtitle in Jellyfin 12.2's order, higher first:
// embedded before files (Polyfin's, see subtitle), flagged default, full
// in the preferred language, forced in the preferred language, forced of
// no stated language, forced. Last comes a forced track in the audio's
// language, where Jellyfin keeps the tracks' order. It fits in 7 bits.
func (t trackPreferences) subtitleRank(s playback.MediaStream, audioLanguage string) int {
	matches := t.matches(s)
	r := 0
	for _, criterion := range [...]bool{
		!s.IsExternal,
		s.IsDefault,
		!s.IsForced && matches,
		s.IsForced && matches,
		s.IsForced && undeterminedLanguage(s.Language),
		s.IsForced,
		s.IsForced && localization.SameLanguage(s.Language, audioLanguage),
	} {
		r <<= 1
		if criterion {
			r |= 1
		}
	}
	return r
}

// bestSubtitle returns the subtitle score places highest, the first of
// equals; a negative score leaves a track out. nil when every track is
// left out. An image track gives way to the text track score places
// highest among those in the same language, equally forced, that are not
// left out: an embedded one before a file.
func bestSubtitle(subtitles []playback.MediaStream, score func(playback.MediaStream) int) *playback.MediaStream {
	var best *playback.MediaStream
	bestScore := -1
	for i, s := range subtitles {
		if r := score(s); r > bestScore {
			best, bestScore = &subtitles[i], r
		}
	}
	if best == nil || best.IsTextSubtitleStream {
		return best
	}
	var text *playback.MediaStream
	textScore := -1
	for i, s := range subtitles {
		if !s.IsTextSubtitleStream || s.IsForced != best.IsForced ||
			s.Language != best.Language && !localization.SameLanguage(s.Language, best.Language) {
			continue
		}
		if r := score(s); r > textScore {
			text, textScore = &subtitles[i], r
		}
	}
	if text == nil {
		return best
	}
	return text
}

// undeterminedLanguage reports whether a track states no language, as
// Jellyfin 12.2 counts them: none, undetermined, multiple languages, or no
// linguistic content.
func undeterminedLanguage(language string) bool {
	switch localization.LanguageCode(language) {
	case "", "und", "undetermined", "unknown", "mul", "zxx":
		return true
	}
	return false
}
