package jellyfin

import (
	"context"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/preferences"
)

// The tracks users choose on a title, a series for all its episodes or a
// movie, are remembered while their configuration remembers selections
// (RememberAudioSelections, RememberSubtitleSelections): its next plays
// show them by default, in whichever version, before the user's language
// preferences, and after a track the app names. A choice is a language,
// with an audio track's regional variant when it differs from the user's
// audio region, a subtitle's forced and hearing-impaired flags, or no
// subtitles; never a track's index, as versions number their tracks
// differently.
// Jellyfin 12.4 remembers the index per item (MediaSourceManager.cs
// L493-L543, SessionManager.cs L1016-L1055).

// trackChoices remembers the track choices of each user, by title, which
// item listings read for every title: they are read once, and read again
// after a change.
type trackChoices struct {
	cache *cache.Cache[accounts.ID, map[accounts.ID]preferences.TrackChoice]
	// mu orders the reads of the store with its changes, so that a read
	// made before a change is never kept after it.
	mu sync.Mutex
}

func newTrackChoices() *trackChoices {
	return &trackChoices{cache: cache.New[accounts.ID, map[accounts.ID]preferences.TrackChoice](1000, 12*time.Hour)}
}

// choiceTitle is the title whose tracks an item's plays remember: its
// series for an episode, the movie itself. Other items remember none.
func choiceTitle(item library.Item) (accounts.ID, bool) {
	switch {
	case item.Kind == library.KindEpisode && item.SeriesID != (accounts.ID{}):
		return item.SeriesID, true
	case item.Kind == library.KindMovie:
		return item.ID, true
	}
	return accounts.ID{}, false
}

// userTrackChoices returns the track choices of user, by title.
func (h *Handler) userTrackChoices(ctx context.Context, user accounts.ID) (map[accounts.ID]preferences.TrackChoice, error) {
	if choices, ok := h.trackChoices.cache.Get(user); ok {
		return choices, nil
	}
	h.trackChoices.mu.Lock()
	defer h.trackChoices.mu.Unlock()
	if choices, ok := h.trackChoices.cache.Get(user); ok {
		return choices, nil
	}
	choices, err := h.Preferences.TrackChoices(ctx, user)
	if err != nil {
		return nil, err
	}
	h.trackChoices.cache.Put(user, choices)
	return choices, nil
}

// putTrackChoice keeps the track choice of user on title, forgetting it
// when empty.
func (h *Handler) putTrackChoice(ctx context.Context, user, title accounts.ID, choice preferences.TrackChoice) error {
	h.trackChoices.mu.Lock()
	defer h.trackChoices.mu.Unlock()
	err := h.Preferences.PutTrackChoice(ctx, user, title, choice)
	h.trackChoices.cache.Delete(user)
	return err
}

// CountTrackChoices counts the titles whose tracks user chose (My account
// › Track choices).
func (h *Handler) CountTrackChoices(ctx context.Context, user accounts.ID) (int, error) {
	choices, err := h.userTrackChoices(ctx, user)
	return len(choices), err
}

// ForgetTrackChoices forgets the tracks user chose on every title: their
// next plays follow their preferences again (My account › Forget my track
// choices).
func (h *Handler) ForgetTrackChoices(ctx context.Context, user accounts.ID) error {
	h.trackChoices.mu.Lock()
	defer h.trackChoices.mu.Unlock()
	err := h.Preferences.ForgetTrackChoices(ctx, user)
	h.trackChoices.cache.Delete(user)
	return err
}

// titleTracks returns the track preferences of user for an item: theirs
// (see trackPreferences), with the tracks they chose on its title while
// their configuration remembers each kind of selection. When the choices
// cannot be read, their preferences alone apply.
func (h *Handler) titleTracks(ctx context.Context, user accounts.User, item library.Item) trackPreferences {
	t := h.trackPreferences(ctx, user)
	title, ok := choiceTitle(item)
	if !ok {
		return t
	}
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		return t
	}
	t.rememberAudio, t.rememberSubtitles = configuration.RememberAudioSelections, configuration.RememberSubtitleSelections
	if !t.rememberAudio && !t.rememberSubtitles {
		return t
	}
	choices, err := h.userTrackChoices(ctx, user.ID)
	if err != nil {
		if ctx.Err() == nil {
			h.Logger.Warn("The track choices of a user could not be read", "error", err)
		}
		return t
	}
	choice := choices[title]
	if t.rememberAudio {
		t.remembered.AudioLanguage, t.remembered.AudioVariant = choice.AudioLanguage, choice.AudioVariant
	}
	if t.rememberSubtitles {
		t.remembered.Subtitle = choice.Subtitle
	}
	return t
}

// rememberTracks follows the tracks a playback report says play: one the
// user chose that the title would not play by default becomes their
// choice on the title; one it plays without their choice forgets it. A
// kind of selection the configuration does not remember is forgotten, as
// Jellyfin forgets it. mediaSource is the version that plays.
func (h *Handler) rememberTracks(ctx context.Context, user accounts.User, item library.Item, state playback.PlayState, mediaSource string) {
	title, ok := choiceTitle(item)
	if !ok || state.AudioStreamIndex == nil && state.SubtitleStreamIndex == nil {
		return
	}
	configuration, err := h.userConfiguration(ctx, user.ID)
	if err != nil {
		return
	}
	choices, err := h.userTrackChoices(ctx, user.ID)
	if err != nil {
		return
	}
	stored := choices[title]
	choice := stored
	if !configuration.RememberAudioSelections {
		choice.AudioLanguage, choice.AudioVariant = "", ""
	}
	if !configuration.RememberSubtitleSelections {
		choice.Subtitle = preferences.SubtitleChoice{}
	}
	if configuration.RememberAudioSelections || configuration.RememberSubtitleSelections {
		if streams, ok := h.reportedStreams(ctx, user, item, mediaSource); ok {
			choice = h.titleTracks(ctx, user, item).chosen(streams, state.AudioStreamIndex, state.SubtitleStreamIndex)
		}
	}
	if choice == stored {
		return
	}
	if err := h.putTrackChoice(ctx, user.ID, title, choice); err != nil {
		h.Logger.Warn("The tracks a user chose could not be remembered", "error", err)
	}
}

// reportedStreams lists the streams of the version a report names, as
// item details describe them, the title's own identifier standing for
// its first working version.
func (h *Handler) reportedStreams(ctx context.Context, user accounts.User, item library.Item, mediaSource string) ([]playback.MediaStream, bool) {
	id, ok := parseGUID(mediaSource)
	if !ok {
		return nil, false
	}
	if id == item.ID {
		id = h.firstWorkingVersion(ctx, user, item)
	}
	version, err := h.version(ctx, user, item, id)
	if err != nil {
		return nil, false
	}
	p := playable{item: item}
	p.subtitles, _ = h.keptSubtitles(user, item.ID)
	return h.versionStreams(ctx, p, version), true
}

// chosen returns the track choice t remembers once the user plays audio
// and subtitle among streams, either nil when not reported. A track
// counts as chosen when the title would not play it by default: it
// replaces the choice, unless its language is not stated; one the user's
// preferences play by themselves forgets it. An audio track's regional
// variant is kept only when it differs from the user's region in its
// language (see chosenVariant). A subtitle's text copy counts as its
// original. Only the kinds of selection t remembers change.
func (t trackPreferences) chosen(streams []playback.MediaStream, audio, subtitle *int) preferences.TrackChoice {
	choice := t.remembered
	plain := t
	plain.remembered = preferences.TrackChoice{}
	if t.rememberAudio && audio != nil {
		if stream, ok := streamAt(streams, "Audio", *audio); ok && !equalIndex(t.audio(streams), audio) {
			switch {
			case equalIndex(plain.audio(streams), audio):
				choice.AudioLanguage, choice.AudioVariant = "", ""
			case !undeterminedLanguage(stream.Language):
				choice.AudioLanguage, choice.AudioVariant = stream.Language, t.chosenVariant(stream)
			}
		}
	}
	if !t.rememberSubtitles || subtitle == nil {
		return choice
	}
	playing := audio
	if playing == nil {
		playing = t.audio(streams)
	}
	index := *subtitle
	var stream playback.MediaStream
	if index >= 0 {
		var ok bool
		if stream, ok = streamAt(streams, "Subtitle", index); !ok {
			return choice
		}
		if stream.CopyOf != nil {
			index = *stream.CopyOf
		}
	}
	if shownSubtitle(t.subtitle(streams, playing)) == index {
		return choice
	}
	switch {
	case shownSubtitle(plain.subtitle(streams, playing)) == index:
		choice.Subtitle = preferences.SubtitleChoice{}
	case index < 0:
		choice.Subtitle = preferences.SubtitleChoice{Off: true}
	case !undeterminedLanguage(stream.Language):
		choice.Subtitle = preferences.SubtitleChoice{Language: stream.Language, Forced: stream.IsForced, HearingImpaired: stream.IsHearingImpaired}
	}
	return choice
}

// rememberedAudio is the audio track in the language the user chose on
// the title. Its tracks rank as audio ranks those of the preferred
// language (see audioRank), for the variant the user chose against their
// region, else for their region when it is of that language: the
// variant's first, then those of no known region, then those of another;
// the one flagged default first among equals. With neither, the one
// flagged default comes first. nil when nothing was chosen, or no track
// is in that language.
func (t trackPreferences) rememberedAudio(streams []playback.MediaStream) *int {
	if t.remembered.AudioLanguage == "" {
		return nil
	}
	language := trackPreferences{audioLanguage: t.remembered.AudioLanguage, audioRegion: t.remembered.AudioVariant}
	if language.audioRegion == "" {
		language.audioRegion = t.regionOf(t.remembered.AudioLanguage)
	}
	best, bestRank := -1, 0
	for _, stream := range streams {
		rank := language.audioRank(stream)
		if stream.Type != "Audio" || rank == 0 {
			continue
		}
		r := rank << 1
		if stream.IsDefault {
			r |= 1
		}
		if r > bestRank {
			best, bestRank = stream.Index, r
		}
	}
	if best < 0 {
		return nil
	}
	return new(best)
}

// regionOf is the user's audio region when it is of language, else "".
func (t trackPreferences) regionOf(language string) string {
	if t.region != "" && localization.SameLanguage(localization.VariantLanguage(t.region), language) {
		return t.region
	}
	return ""
}

// chosenVariant is the regional variant of an audio track the user
// picked that their next plays remember: the track's own when they have
// a region in its language and the track is of another. Otherwise
// nothing, as their region ranks the language's tracks by itself, and
// a user with no region there keeps the track flagged default first.
func (t trackPreferences) chosenVariant(stream playback.MediaStream) string {
	region := t.regionOf(stream.Language)
	if region == "" {
		return ""
	}
	if variant := localization.AudioVariant(stream.Language, stream.Title); variant != region {
		return variant
	}
	return ""
}

// rememberedSubtitle is the subtitle the user chose on the title, among
// subtitles: -1 for none, else the best track in its language, equally
// forced, hearing-impaired like it first, ranked as subtitleRank ranks
// them. nil when nothing was chosen or no track matches. As in Jellyfin,
// the mode None shows none whatever was chosen, and OnlyForced no full
// track.
func (t trackPreferences) rememberedSubtitle(subtitles []playback.MediaStream, audioLanguage string) *int {
	chosen := t.remembered.Subtitle
	if !chosen.Chosen() || t.subtitleMode == "None" {
		return nil
	}
	if chosen.Off {
		return new(-1)
	}
	if t.subtitleMode == "OnlyForced" && !chosen.Forced {
		return nil
	}
	best := bestSubtitle(subtitles, func(s playback.MediaStream) int {
		if s.IsForced != chosen.Forced || !localization.SameLanguage(s.Language, chosen.Language) {
			return -1
		}
		r := t.subtitleRank(s, audioLanguage)
		if s.IsHearingImpaired == chosen.HearingImpaired {
			r |= 1 << 7
		}
		return r
	})
	if best == nil {
		return nil
	}
	return new(best.Index)
}

// streamAt returns the stream of a type at index among streams.
func streamAt(streams []playback.MediaStream, kind string, index int) (playback.MediaStream, bool) {
	for _, stream := range streams {
		if stream.Type == kind && stream.Index == index {
			return stream, true
		}
	}
	return playback.MediaStream{}, false
}

// shownSubtitle is the subtitle a default subtitle shows, -1 for none, as
// when a version has no subtitles.
func shownSubtitle(subtitle *int) int {
	if subtitle == nil {
		return -1
	}
	return *subtitle
}

// equalIndex reports whether two track indices are the same, or both
// absent.
func equalIndex(a, b *int) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}
