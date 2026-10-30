package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/library"
	"github.com/moodiness/polyfin/internal/media"
	"github.com/moodiness/polyfin/internal/playback"
	"github.com/moodiness/polyfin/internal/preferences"
)

// tracksOf stores the analysis of a version: H.264 video, then audio tracks
// in languages, the first flagged default, then full subtitles in
// subtitles.
func (p skipSetup) tracksOf(t *testing.T, version library.Version, audio, subtitles []string) {
	t.Helper()
	streams := []media.Stream{{Index: 0, Type: "video", Codec: "h264", Profile: "High", Level: 40, Width: 1920, Height: 1080,
		FrameRate: 24, AverageRate: 24, PixelFormat: "yuv420p", BitDepth: 8}}
	for i, language := range audio {
		streams = append(streams, media.Stream{Index: len(streams), Type: "audio", Codec: "aac", Language: language, Default: i == 0,
			Channels: 2, SampleRate: 48000, ChannelLayout: "stereo"})
	}
	for _, language := range subtitles {
		streams = append(streams, media.Stream{Index: len(streams), Type: "subtitle", Codec: "subrip", Language: language})
	}
	data, _ := json.Marshal(media.Analysis{Format: "matroska,webm", Duration: 45 * time.Minute, Remote: true, Streams: streams})
	if _, err := p.pool.Exec(t.Context(), "INSERT INTO media_analyses (version_id, analysis) VALUES ($1, $2)", version.ID, data); err != nil {
		t.Fatal(err)
	}
}

// A track picked on an episode is chosen by default on the next ones, in
// whichever version, by its language rather than its index; a version
// without that language plays as the user's preferences have it. Turning
// the configuration's settings off gives the defaults back, and so does
// forgetting the choices.
func TestTracksChosenOnAnEpisodeAreTheDefaultOnTheNext(t *testing.T) {
	t.Parallel()
	p := skipping(t)
	var episodes QueryResult
	p.get(t, "/Shows/"+p.series+"/Episodes", p.token, &episodes)
	pilot := ""
	for _, item := range episodes.Items {
		if item.Name == "Pilot" {
			pilot = item.Id
		}
	}
	first, second := p.versions(t, pilot), p.versions(t, p.episode)
	// S01E01's first cut: English (flagged) and Japanese audio, English
	// subtitles. S01E02's second cut numbers its tracks otherwise, and its
	// third has no Japanese.
	p.tracksOf(t, first[0], []string{"eng", "jpn"}, []string{"eng"})
	p.tracksOf(t, second[1], []string{"eng", "fre", "jpn"}, []string{"eng"})
	p.tracksOf(t, second[2], []string{"eng", "fre"}, []string{"eng"})
	const japanese, englishSubtitles = 3, 4

	defaults := func(when string, version library.Version, audio, subtitle int) {
		t.Helper()
		var episode BaseItemDto
		p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.episode, p.token, &episode)
		for _, source := range *episode.MediaSources {
			if source.ETag != version.ID.String() {
				continue
			}
			if !sameIndex(source.DefaultAudioStreamIndex, new(audio)) || !sameIndex(source.DefaultSubtitleStreamIndex, new(subtitle)) {
				t.Errorf("%s: audio %v subtitle %v, want %d and %d", when, deref(source.DefaultAudioStreamIndex),
					deref(source.DefaultSubtitleStreamIndex), audio, subtitle)
			}
			return
		}
		t.Fatalf("%s: version %s not described", when, version.Name)
	}
	report := func(path, item string, version library.Version, audio, subtitle int) {
		t.Helper()
		status, data := p.call(http.MethodPost, path, app("tv", p.token), map[string]any{"ItemId": item, "MediaSourceId": version.ID.String(),
			"PositionTicks": 600_000_000, "AudioStreamIndex": audio, "SubtitleStreamIndex": subtitle})
		if status != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, status, data)
		}
	}
	defaults("before any choice", second[1], 1, -1)

	// The pilot starts with its default tracks, then the user picks the
	// Japanese audio and the English subtitles.
	report("/Sessions/Playing", pilot, first[0], 1, -1)
	defaults("after default tracks", second[1], 1, -1)
	report("/Sessions/Playing/Progress", pilot, first[0], 2, 3)
	defaults("after picking Japanese", second[1], japanese, englishSubtitles)
	defaults("a version without Japanese", second[2], 1, 3)
	// PlaybackInfo, as jellyfin-web asks it, plays the same tracks.
	status, data := p.call(http.MethodPost, "/Items/"+p.episode+"/PlaybackInfo", app("tv", p.token),
		map[string]any{"UserId": p.user.ID.String(), "MediaSourceId": second[1].ID.String()})
	var response playbackInfoResponse
	if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) != 1 {
		t.Fatalf("PlaybackInfo: %d %s", status, data)
	}
	if source := response.MediaSources[0]; !sameIndex(source.DefaultAudioStreamIndex, new(japanese)) ||
		!sameIndex(source.DefaultSubtitleStreamIndex, new(englishSubtitles)) {
		t.Errorf("PlaybackInfo: audio %v subtitle %v", deref(source.DefaultAudioStreamIndex), deref(source.DefaultSubtitleStreamIndex))
	}

	configure := func(remember bool) {
		t.Helper()
		if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"RememberAudioSelections": remember,
			"RememberSubtitleSelections": remember}); status != http.StatusNoContent {
			t.Fatalf("saving: %d", status)
		}
	}
	configure(false)
	defaults("settings off", second[1], 1, -1)
	configure(true)
	defaults("settings on again", second[1], japanese, englishSubtitles)

	if err := p.handler.ForgetTrackChoices(t.Context(), p.user.ID); err != nil {
		t.Fatal(err)
	}
	defaults("forgotten", second[1], 1, -1)
	if titles, err := p.handler.CountTrackChoices(t.Context(), p.user.ID); err != nil || titles != 0 {
		t.Errorf("titles after forgetting: %d %v", titles, err)
	}

	// Picking the default track back forgets the choice of that kind.
	report("/Sessions/Playing/Progress", pilot, first[0], 2, -1)
	defaults("Japanese again", second[1], japanese, -1)
	report("/Sessions/Playing/Progress", pilot, first[0], 1, -1)
	defaults("English picked back", second[1], 1, -1)
	if titles, err := p.handler.CountTrackChoices(t.Context(), p.user.ID); err != nil || titles != 0 {
		t.Errorf("titles once the defaults were picked back: %d %v", titles, err)
	}

	// With the settings off, a report forgets what was chosen, as in
	// Jellyfin.
	report("/Sessions/Playing/Progress", pilot, first[0], 2, 3)
	configure(false)
	report("/Sessions/Playing", pilot, first[0], 2, 3)
	configure(true)
	defaults("after a report with the settings off", second[1], 1, -1)
}

// What a report counts as the user's choice, and how a choice is played.
func TestTrackChoicesByLanguage(t *testing.T) {
	t.Parallel()
	video := playback.MediaStream{Index: 0, Type: "Video"}
	audio := func(index int, language string, flagged bool) playback.MediaStream {
		return playback.MediaStream{Index: index, Type: "Audio", Language: language, IsDefault: flagged}
	}
	subtitle := func(index int, language string, forced, hearingImpaired bool) playback.MediaStream {
		return playback.MediaStream{Index: index, Type: "Subtitle", Language: language, IsForced: forced, IsHearingImpaired: hearingImpaired,
			IsTextSubtitleStream: true}
	}
	streams := []playback.MediaStream{video, audio(1, "eng", true), audio(2, "jpn", false), audio(3, "und", false),
		subtitle(4, "eng", false, false), subtitle(5, "eng", false, true), subtitle(6, "eng", true, false), subtitle(7, "fre", false, false)}
	copied := subtitle(8, "fre", false, false)
	copied.CopyOf = new(7)
	streams = append(streams, copied)
	remembering := trackPreferences{subtitleMode: "Default", rememberAudio: true, rememberSubtitles: true}
	japanese := preferences.TrackChoice{AudioLanguage: "jpn"}
	for name, test := range map[string]struct {
		preferences     trackPreferences
		audio, subtitle *int
		want            preferences.TrackChoice
	}{
		"the default tracks":               {remembering, new(1), new(6), preferences.TrackChoice{}},
		"another language":                 {remembering, new(2), nil, japanese},
		"an undetermined language":         {remembering, new(3), nil, preferences.TrackChoice{}},
		"a subtitle with its flags":        {remembering, nil, new(5), preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "eng", HearingImpaired: true}}},
		"a text copy as its original":      {remembering, nil, new(8), preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "fre"}}},
		"the chosen track again":           {withChoice(remembering, japanese), new(2), nil, japanese},
		"the default back":                 {withChoice(remembering, japanese), new(1), nil, preferences.TrackChoice{}},
		"audio not remembered":             {trackPreferences{rememberSubtitles: true}, new(2), nil, preferences.TrackChoice{}},
		"subtitles off":                    {trackPreferences{subtitleMode: "Always", subtitleLanguage: "eng", rememberSubtitles: true}, nil, new(-1), preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Off: true}}},
		"an unknown track changes nothing": {withChoice(remembering, japanese), new(9), new(12), japanese},
	} {
		if got := test.preferences.chosen(streams, test.audio, test.subtitle); got != test.want {
			t.Errorf("%s: %+v, want %+v", name, got, test.want)
		}
	}

	for name, test := range map[string]struct {
		preferences trackPreferences
		want        int
	}{
		"hearing impaired like the choice": {withChoice(trackPreferences{subtitleMode: "Default"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "en", HearingImpaired: true}}), 5},
		"equally forced": {withChoice(trackPreferences{subtitleMode: "Always", subtitleLanguage: "fre"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "eng", Forced: true}}), 6},
		"off": {withChoice(trackPreferences{subtitleMode: "Always", subtitleLanguage: "eng"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Off: true}}), -1},
		"a language no track has": {withChoice(trackPreferences{subtitleMode: "Always", subtitleLanguage: "eng"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "ger"}}), 4},
		"OnlyForced shows no full track": {withChoice(trackPreferences{subtitleMode: "OnlyForced", subtitleLanguage: "eng"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "fre"}}), 6},
		"None shows none": {withChoice(trackPreferences{subtitleMode: "None"},
			preferences.TrackChoice{Subtitle: preferences.SubtitleChoice{Language: "fre"}}), -1},
	} {
		if got := test.preferences.subtitle(streams, new(1)); !sameIndex(got, new(test.want)) {
			t.Errorf("%s: %v, want %d", name, deref(got), test.want)
		}
	}
}

// withChoice is t with choice remembered.
func withChoice(t trackPreferences, choice preferences.TrackChoice) trackPreferences {
	t.remembered = choice
	return t
}
