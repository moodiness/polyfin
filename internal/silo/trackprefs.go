package silo

import (
	"cmp"
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/localization"
	"github.com/moodiness/polyfin/internal/preferences"
)

// A series' remembered audio and subtitle choice, which Silo's apps read
// and write by the series, are the track choices Polyfin remembers on a
// series or a movie for Jellyfin apps (see docs/audio-and-subtitle-tracks.md):
// a language, with a subtitle's flags, or subtitles off. Polyfin keeps no
// track's place, which differs between episodes and versions: it answers
// the first track's, 0, beside the language that tells the track; the
// apps' choice of a place alone keeps nothing.

// TrackChoices reads and keeps the tracks users chose on titles (see
// jellyfin.Handler.TrackChoice).
type TrackChoices interface {
	// TrackChoice returns the choice of user on title, with when it last
	// changed; the zero choice for none.
	TrackChoice(ctx context.Context, user, title accounts.ID) (preferences.TrackChoice, time.Time, error)
	// PutTrackChoice keeps the choice of user on title, forgetting it when
	// empty.
	PutTrackChoice(ctx context.Context, user, title accounts.ID, choice preferences.TrackChoice) error
}

func (s *Server) trackPreferenceRoutes() {
	s.mux.HandleFunc("GET /api/v2/audio-prefs/{series_id}", s.profiled(s.audioPreference))
	s.mux.HandleFunc("PUT /api/v2/audio-prefs/{series_id}", s.profiled(s.putAudioPreference))
	s.mux.HandleFunc("DELETE /api/v2/audio-prefs/{series_id}", s.profiled(s.forgetTracks(false)))
	s.mux.HandleFunc("GET /api/v2/subtitle-prefs/{series_id}", s.profiled(s.subtitlePreference))
	s.mux.HandleFunc("PUT /api/v2/subtitle-prefs/{series_id}", s.profiled(s.putSubtitlePreference))
	s.mux.HandleFunc("DELETE /api/v2/subtitle-prefs/{series_id}", s.profiled(s.forgetTracks(true)))
}

// titleChoice is the track choice of the user on the series or movie a
// request names, and what their configuration remembers, audio and
// subtitles: a kind it does not remember is neither followed nor kept, as
// for Jellyfin apps.
type titleChoice struct {
	title            accounts.ID
	choice           preferences.TrackChoice
	at               time.Time
	audio, subtitles bool
}

// choiceOf reads the track choice of the user on the series or movie the
// request names, answering the request when the user may not see it, or
// it is neither.
func (s *Server) choiceOf(w http.ResponseWriter, r *http.Request, v viewer) (titleChoice, bool) {
	if s.options.TrackChoices == nil {
		problem(w, http.StatusServiceUnavailable, "unavailable", "This server keeps no track choices.")
		return titleChoice{}, false
	}
	i, ok := s.itemOf(w, r, v, r.PathValue("series_id"))
	if !ok {
		return titleChoice{}, false
	}
	title, err := accounts.ParseID(r.PathValue("series_id"))
	if err != nil || i.Type != "Series" && i.Type != "Movie" {
		problem(w, http.StatusNotFound, "not_found", "Track choices are kept for series and movies.")
		return titleChoice{}, false
	}
	var user struct {
		Configuration struct {
			RememberAudioSelections    bool
			RememberSubtitleSelections bool
		}
	}
	if _, err := s.call(r, v, http.MethodGet, "/Users/Me", nil, &user); err != nil {
		s.internalError(w, r, err)
		return titleChoice{}, false
	}
	choice, at, err := s.options.TrackChoices.TrackChoice(r.Context(), v.user.ID, title)
	if err != nil {
		s.internalError(w, r, err)
		return titleChoice{}, false
	}
	return titleChoice{title: title, choice: choice, at: at, audio: user.Configuration.RememberAudioSelections,
		subtitles: user.Configuration.RememberSubtitleSelections}, true
}

// put keeps a changed choice.
func (s *Server) put(w http.ResponseWriter, r *http.Request, v viewer, c titleChoice, choice preferences.TrackChoice) {
	if choice != c.choice {
		if err := s.options.TrackChoices.PutTrackChoice(r.Context(), v.user.ID, c.title, choice); err != nil {
			s.internalError(w, r, err)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

// forgetTracks forgets the audio choice of a series or movie, or its
// subtitle choice: none is still forgotten.
func (s *Server) forgetTracks(subtitles bool) func(http.ResponseWriter, *http.Request, viewer) {
	return func(w http.ResponseWriter, r *http.Request, v viewer) {
		c, ok := s.choiceOf(w, r, v)
		if !ok {
			return
		}
		choice := c.choice
		if subtitles {
			choice.Subtitle = preferences.SubtitleChoice{}
		} else {
			choice.AudioLanguage, choice.AudioVariant = "", ""
		}
		s.put(w, r, v, c, choice)
	}
}

// preferenceTag names a language Polyfin keeps as the contract does, by
// its BCP 47 tag: its shortest code, then the rest of a regional tag
// ("fre-CA" is "fr-CA").
func preferenceTag(code string) string {
	base, rest, regional := strings.Cut(strings.ReplaceAll(code, "_", "-"), "-")
	tag := languageTag(base)
	if regional {
		tag += "-" + rest
	}
	return tag
}

// sentLanguage matches a language the apps send: a BCP 47 tag or an ISO
// 639 code, in any case, its subtags joined by dashes or underscores.
var sentLanguage = regexp.MustCompile(`^[A-Za-z]{2,3}(?:[-_][0-9A-Za-z]{1,8})*$`)

// maxLanguage bounds a language kept (see track_choices).
const maxLanguage = 35

// sentTag reads a language the apps send, falling back on the one its
// signature names: "" for none, or for the original language, which
// Polyfin does not keep; false, answering the request, for one that is
// not a language.
func sentTag(w http.ResponseWriter, location string, language *string, signature string) (string, bool) {
	sent := strings.TrimSpace(cmp.Or(deref(language), signature))
	switch {
	case sent == "", strings.EqualFold(sent, "x-silo-original"):
		return "", true
	case len(sent) > maxLanguage || !sentLanguage.MatchString(sent):
		validationProblem(w, location, "invalid", "A language is a BCP 47 tag or an ISO 639 code.")
		return "", false
	}
	return preferenceTag(sent), true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// sentIndex reads the place of a track the apps send, -1 for none.
func sentIndex(w http.ResponseWriter, location string, index *int64) (int64, bool) {
	switch {
	case index == nil:
		validationProblem(w, location, "required", "The place of the track chosen is required.")
		return 0, false
	case *index < -1:
		validationProblem(w, location, "invalid", "A track's place is -1 or more.")
		return 0, false
	}
	return *index, true
}

type audioSignature struct {
	Language string `json:"language,omitempty"`
}

type audioPreference struct {
	ProfileID       string          `json:"profile_id"`
	SeriesID        string          `json:"series_id"`
	AudioTrackIndex int             `json:"audio_track_index"`
	AudioLanguage   string          `json:"audio_language"`
	TrackSignature  *audioSignature `json:"track_signature,omitempty"`
	UpdatedAt       string          `json:"updated_at"`
}

// audioPreference answers the audio language the user chose on a series
// or a movie, in its regional variant when they chose one, while their
// configuration remembers audio selections; none is not found, as Silo
// answers a series without one.
func (s *Server) audioPreference(w http.ResponseWriter, r *http.Request, v viewer) {
	c, ok := s.choiceOf(w, r, v)
	if !ok {
		return
	}
	if !c.audio || c.choice.AudioLanguage == "" {
		problem(w, http.StatusNotFound, "not_found", "No audio track is remembered for this title.")
		return
	}
	language := preferenceTag(cmp.Or(c.choice.AudioVariant, c.choice.AudioLanguage))
	writeJSON(w, http.StatusOK, audioPreference{ProfileID: v.user.ID.String(), SeriesID: r.PathValue("series_id"), AudioLanguage: language,
		TrackSignature: &audioSignature{Language: language}, UpdatedAt: instant(c.at)})
}

// putAudioPreference keeps the audio language a Silo app chose on a
// series or a movie, with its regional variant, while the user's
// configuration remembers audio selections: none, or no track, forgets
// it.
func (s *Server) putAudioPreference(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		AudioLanguage   *string `json:"audio_language"`
		AudioTrackIndex *int64  `json:"audio_track_index"`
		TrackSignature  *struct {
			Language string `json:"language"`
		} `json:"track_signature"`
	}
	if !decode(w, r, &body) {
		return
	}
	index, ok := sentIndex(w, "body.audio_track_index", body.AudioTrackIndex)
	if !ok {
		return
	}
	var signature string
	if body.TrackSignature != nil {
		signature = body.TrackSignature.Language
	}
	language, ok := sentTag(w, "body.audio_language", body.AudioLanguage, signature)
	if !ok {
		return
	}
	c, ok := s.choiceOf(w, r, v)
	if !ok {
		return
	}
	choice := c.choice
	if c.audio {
		choice.AudioLanguage, choice.AudioVariant = "", ""
		if index >= 0 && language != "" {
			choice.AudioLanguage, choice.AudioVariant = language, localization.TagVariant(language)
		}
	}
	s.put(w, r, v, c, choice)
}

type subtitleSignature struct {
	Language        string `json:"language,omitempty"`
	Forced          bool   `json:"forced"`
	HearingImpaired bool   `json:"hearing_impaired"`
}

type subtitlePreference struct {
	ProfileID          string             `json:"profile_id"`
	SeriesID           string             `json:"series_id"`
	SubtitleTrackIndex int                `json:"subtitle_track_index"`
	SubtitleLanguage   string             `json:"subtitle_language"`
	SubtitleMode       string             `json:"subtitle_mode"`
	TrackSignature     *subtitleSignature `json:"track_signature,omitempty"`
	UpdatedAt          string             `json:"updated_at"`
}

// subtitlePreference answers the subtitles the user chose on a series or
// a movie, while their configuration remembers subtitle selections: off,
// or a language, forced and hearing-impaired or not, shown always; none
// is not found, as Silo answers a series without one.
func (s *Server) subtitlePreference(w http.ResponseWriter, r *http.Request, v viewer) {
	c, ok := s.choiceOf(w, r, v)
	if !ok {
		return
	}
	chosen := c.choice.Subtitle
	if !c.subtitles || !chosen.Chosen() {
		problem(w, http.StatusNotFound, "not_found", "No subtitle is remembered for this title.")
		return
	}
	answer := subtitlePreference{ProfileID: v.user.ID.String(), SeriesID: r.PathValue("series_id"), SubtitleTrackIndex: -1,
		SubtitleMode: "off", UpdatedAt: instant(c.at)}
	if !chosen.Off {
		language := preferenceTag(chosen.Language)
		answer.SubtitleTrackIndex, answer.SubtitleLanguage, answer.SubtitleMode = 0, language, "always"
		answer.TrackSignature = &subtitleSignature{Language: language, Forced: chosen.Forced, HearingImpaired: chosen.HearingImpaired}
	}
	writeJSON(w, http.StatusOK, answer)
}

// putSubtitlePreference keeps the subtitles a Silo app chose on a series
// or a movie, while the user's configuration remembers subtitle
// selections: off, for no track or the mode off, else the track's
// language with its forced and hearing-impaired flags; none forgets the
// choice.
func (s *Server) putSubtitlePreference(w http.ResponseWriter, r *http.Request, v viewer) {
	var body struct {
		SubtitleLanguage   *string `json:"subtitle_language"`
		SubtitleTrackIndex *int64  `json:"subtitle_track_index"`
		SubtitleMode       string  `json:"subtitle_mode"`
		TrackSignature     *struct {
			Language        string `json:"language"`
			Forced          bool   `json:"forced"`
			HearingImpaired bool   `json:"hearing_impaired"`
		} `json:"track_signature"`
	}
	if !decode(w, r, &body) {
		return
	}
	index, ok := sentIndex(w, "body.subtitle_track_index", body.SubtitleTrackIndex)
	if !ok {
		return
	}
	var signature string
	var forced, hearingImpaired bool
	if body.TrackSignature != nil {
		signature, forced, hearingImpaired = body.TrackSignature.Language, body.TrackSignature.Forced, body.TrackSignature.HearingImpaired
	}
	language, ok := sentTag(w, "body.subtitle_language", body.SubtitleLanguage, signature)
	if !ok {
		return
	}
	c, ok := s.choiceOf(w, r, v)
	if !ok {
		return
	}
	choice := c.choice
	if c.subtitles {
		switch {
		case index < 0, strings.EqualFold(strings.TrimSpace(body.SubtitleMode), "off"):
			choice.Subtitle = preferences.SubtitleChoice{Off: true}
		case language != "":
			choice.Subtitle = preferences.SubtitleChoice{Language: language, Forced: forced, HearingImpaired: hearingImpaired}
		default:
			choice.Subtitle = preferences.SubtitleChoice{}
		}
	}
	s.put(w, r, v, c, choice)
}
