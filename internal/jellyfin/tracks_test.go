package jellyfin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/playback"
)

func TestTheDefaultAudioFollowsTheLanguagePreference(t *testing.T) {
	t.Parallel()
	audio := func(index int, language string, flagged bool) playback.MediaStream {
		return playback.MediaStream{Type: "Audio", Index: index, Language: language, IsDefault: flagged}
	}
	video := playback.MediaStream{Type: "Video", Index: 0}
	english, french, german := audio(1, "eng", true), audio(2, "fra", false), audio(3, "deu", false)
	for name, test := range map[string]struct {
		preferences trackPreferences
		streams     []playback.MediaStream
		want        *int
	}{
		"no audio":                           {trackPreferences{playDefaultAudio: true}, []playback.MediaStream{video}, nil},
		"no preference: the flagged one":     {trackPreferences{playDefaultAudio: true}, []playback.MediaStream{video, french, english}, new(1)},
		"no preference, none flagged":        {trackPreferences{}, []playback.MediaStream{video, french, german}, new(2)},
		"bibliographic preference":           {trackPreferences{audioLanguage: "fre"}, []playback.MediaStream{video, english, french}, new(2)},
		"two-letter preference":              {trackPreferences{audioLanguage: "de"}, []playback.MediaStream{video, english, french, german}, new(3)},
		"tagged with the bibliographic code": {trackPreferences{audioLanguage: "deu"}, []playback.MediaStream{video, english, audio(4, "GER", false)}, new(4)},
		"preferred language missing":         {trackPreferences{audioLanguage: "jpn"}, []playback.MediaStream{video, french, english}, new(1)},
		"default track whatever the language": {trackPreferences{audioLanguage: "fre", playDefaultAudio: true},
			[]playback.MediaStream{video, english, french}, new(1)},
		"default track in the preferred language first": {trackPreferences{audioLanguage: "fre", playDefaultAudio: true},
			[]playback.MediaStream{video, english, french, audio(4, "fra", true)}, new(4)},
		"flagged first among the preferred": {trackPreferences{audioLanguage: "fre"},
			[]playback.MediaStream{video, english, french, audio(4, "fra", true)}, new(4)},
	} {
		if got := test.preferences.audio(test.streams); !sameIndex(got, test.want) {
			t.Errorf("%s: %v, want %v", name, deref(got), deref(test.want))
		}
	}
}

func TestTheDefaultSubtitleFollowsTheMode(t *testing.T) {
	t.Parallel()
	type flags struct{ text, forced, flagged, file bool }
	subtitle := func(index int, language string, f flags) playback.MediaStream {
		return playback.MediaStream{Type: "Subtitle", Index: index, Language: language, IsTextSubtitleStream: f.text,
			IsForced: f.forced, IsDefault: f.flagged, IsExternal: f.file}
	}
	english := playback.MediaStream{Type: "Audio", Index: 2, Language: "eng"}
	french := playback.MediaStream{Type: "Audio", Index: 3, Language: "fra"}
	video := playback.MediaStream{Type: "Video", Index: 1}
	// A French file from an addon, then an English video with full and
	// forced English tracks and a full French one.
	streams := []playback.MediaStream{
		subtitle(0, "fre", flags{text: true, file: true}), video, english, french,
		subtitle(4, "eng", flags{text: true}), subtitle(5, "eng", flags{text: true, forced: true}), subtitle(6, "fra", flags{text: true}),
	}
	withoutFrench := append(streams[:6:6], subtitle(7, "spa", flags{text: true, flagged: true}))
	onlyTheFile := []playback.MediaStream{streams[0], video, english}
	for name, test := range map[string]struct {
		preferences trackPreferences
		streams     []playback.MediaStream
		audio       int
		want        int
	}{
		"Default: the forced track":                  {trackPreferences{subtitleMode: "Default"}, streams, 2, 5},
		"Default: a track flagged default":           {trackPreferences{subtitleMode: "Default"}, append(streams[:4:4], subtitle(4, "eng", flags{text: true, flagged: true})), 2, 4},
		"Default: no file imposed":                   {trackPreferences{subtitleMode: "Default", subtitleLanguage: "fre"}, onlyTheFile, 2, -1},
		"Always: full track in the language":         {trackPreferences{subtitleMode: "Always", subtitleLanguage: "fr"}, streams, 2, 6},
		"Always: embedded before the file":           {trackPreferences{subtitleMode: "Always", subtitleLanguage: "fra"}, streams, 2, 6},
		"Always: the file when nothing else matches": {trackPreferences{subtitleMode: "Always", subtitleLanguage: "fre"}, withoutFrench, 2, 0},
		"Always: full rather than forced":            {trackPreferences{subtitleMode: "Always", subtitleLanguage: "eng"}, streams, 2, 4},
		"Always: else forced in the language or of none": {trackPreferences{subtitleMode: "Always", subtitleLanguage: "jpn"},
			[]playback.MediaStream{video, english, subtitle(4, "eng", flags{text: true, forced: true}), subtitle(5, "und", flags{text: true, forced: true})}, 2, 5},
		"OnlyForced: forced in the language":     {trackPreferences{subtitleMode: "OnlyForced", subtitleLanguage: "en"}, streams, 3, 5},
		"OnlyForced: forced in another language": {trackPreferences{subtitleMode: "OnlyForced", subtitleLanguage: "fre"}, streams, 2, -1},
		"OnlyForced: no preferred language":      {trackPreferences{subtitleMode: "OnlyForced"}, streams, 2, 5},
		"OnlyForced: undetermined language": {trackPreferences{subtitleMode: "OnlyForced", subtitleLanguage: "fre"},
			[]playback.MediaStream{video, english, subtitle(4, "und", flags{text: true, forced: true})}, 2, 4},
		"OnlyForced: never a full track": {trackPreferences{subtitleMode: "OnlyForced", subtitleLanguage: "fre"},
			[]playback.MediaStream{video, english, subtitle(4, "fra", flags{text: true, flagged: true})}, 2, -1},
		"Smart: foreign audio":                       {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "fre"}, streams, 2, 6},
		"Smart: foreign audio, none in the language": {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "ita"}, withoutFrench, 2, -1},
		"Smart: foreign audio, full rather than forced": {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "fre"},
			append(streams[:4:4], subtitle(4, "fra", flags{text: true, forced: true}), subtitle(5, "fra", flags{text: true})), 2, 5},
		"Smart: no preferred language, the first track": {trackPreferences{subtitleMode: "Smart"}, streams, 2, 4},
		"Smart: audio in the language, forced only":     {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "eng"}, streams, 2, 5},
		"Smart: audio in the language, forced in the language": {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "fre"},
			append(streams[:4:4], subtitle(4, "eng", flags{text: true, forced: true}), subtitle(5, "fra", flags{text: true, forced: true})), 3, 5},
		"None": {trackPreferences{subtitleMode: "None", subtitleLanguage: "fre"}, streams, 2, -1},
		"Default: forced in the audio's language first": {trackPreferences{subtitleMode: "Default"},
			append(streams[:4:4], subtitle(4, "eng", flags{text: true, forced: true}), subtitle(5, "fra", flags{text: true, forced: true})), 3, 5},
		"Default: flagged default before forced": {trackPreferences{subtitleMode: "Default"},
			[]playback.MediaStream{video, english, subtitle(4, "eng", flags{text: true, forced: true}), subtitle(5, "spa", flags{text: true, flagged: true})}, 2, 5},
		"Always: text rather than image": {trackPreferences{subtitleMode: "Always", subtitleLanguage: "fre"},
			[]playback.MediaStream{streams[0], video, english, subtitle(3, "fra", flags{})}, 2, 0},
		"Smart: an image track gives way to embedded text before a file": {trackPreferences{subtitleMode: "Smart", subtitleLanguage: "fre"},
			[]playback.MediaStream{streams[0], video, english, subtitle(3, "fra", flags{flagged: true}), subtitle(4, "fra", flags{text: true})}, 2, 4},
	} {
		got := test.preferences.subtitle(test.streams, new(test.audio))
		if got == nil || *got != test.want {
			t.Errorf("%s: %v, want %d", name, deref(got), test.want)
		}
	}
	if got := (trackPreferences{subtitleMode: "None"}).subtitle([]playback.MediaStream{video, english}, new(2)); got != nil {
		t.Errorf("without subtitles: %d, want none", *got)
	}
}

func sameIndex(a, b *int) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func TestDefaultTracksFollowTheUsersPreferences(t *testing.T) {
	t.Parallel()
	p := playing(t)
	// The first version has English (flagged) and French audio, and full
	// and forced English subtitles, after the addon's French file.
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	const fileFrench, audioEnglish, audioFrench, fullEnglish, forcedEnglish = 0, 2, 3, 4, 5
	detail := func() MediaSourceInfo {
		t.Helper()
		var movie BaseItemDto
		p.get(t, "/Users/"+p.user.ID.String()+"/Items/"+p.movie, p.token, &movie)
		if movie.MediaSources == nil || len(*movie.MediaSources) == 0 {
			t.Fatalf("media sources: %+v", movie.MediaSources)
		}
		return (*movie.MediaSources)[0]
	}
	playbackInfo := func() MediaSourceInfo {
		t.Helper()
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
			map[string]any{"UserId": p.user.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		return response.MediaSources[0]
	}
	check := func(when string, source MediaSourceInfo, audio, subtitle int) {
		t.Helper()
		if !sameIndex(source.DefaultAudioStreamIndex, new(audio)) || !sameIndex(source.DefaultSubtitleStreamIndex, new(subtitle)) {
			t.Errorf("%s: audio %v subtitle %v, want %d and %d", when, deref(source.DefaultAudioStreamIndex),
				deref(source.DefaultSubtitleStreamIndex), audio, subtitle)
		}
	}
	check("details for a new user", detail(), audioEnglish, forcedEnglish)
	check("PlaybackInfo for a new user", playbackInfo(), audioEnglish, forcedEnglish)

	if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"AudioLanguagePreference": "fre",
		"PlayDefaultAudioTrack": false, "SubtitleLanguagePreference": "en", "SubtitleMode": "Always"}); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	check("details in French with English subtitles", detail(), audioFrench, fullEnglish)
	check("PlaybackInfo in French with English subtitles", playbackInfo(), audioFrench, fullEnglish)

	if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"AudioLanguagePreference": "fra",
		"SubtitleLanguagePreference": "fr", "SubtitleMode": "Smart"}); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	check("details playing the default English audio, French subtitles", detail(), audioEnglish, fileFrench)

	if status := p.configure(t, "/Users/Configuration", p.token, map[string]any{"SubtitleMode": "None"}); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	check("PlaybackInfo without subtitles", playbackInfo(), audioEnglish, -1)
}

// The server's default audio and subtitle languages apply to the users who
// chose none, with their own subtitle mode, and never to a user's own
// choice; their configuration keeps showing no language, so that an app
// saving it back does not make the default theirs.
func TestServerDefaultLanguagesApplyWhereUsersChoseNone(t *testing.T) {
	t.Parallel()
	p := playing(t)
	// English (flagged) and French audio, full and forced English
	// subtitles, after the addon's French file.
	p.analyzed(t, p.versions[0], "h264-ac3-srt-mkv")
	p.keyframed(t, p.versions[0])
	const fileFrench, audioEnglish, audioFrench, fullEnglish = 0, 2, 3, 4
	check := func(when string, audio, subtitle int) {
		t.Helper()
		status, data := p.call(http.MethodPost, "/Items/"+p.movie+"/PlaybackInfo", app("tv", p.token),
			map[string]any{"UserId": p.user.ID.String(), "DeviceProfile": p.profile(t, "jellyfin-web-chrome")})
		var response playbackInfoResponse
		if err := json.Unmarshal(data, &response); status != http.StatusOK || err != nil || len(response.MediaSources) == 0 {
			t.Fatalf("PlaybackInfo: %d %s", status, data)
		}
		source := response.MediaSources[0]
		if !sameIndex(source.DefaultAudioStreamIndex, new(audio)) || !sameIndex(source.DefaultSubtitleStreamIndex, new(subtitle)) {
			t.Errorf("%s: audio %v subtitle %v, want %d and %d", when, deref(source.DefaultAudioStreamIndex),
				deref(source.DefaultSubtitleStreamIndex), audio, subtitle)
		}
	}
	configure := func(configuration map[string]any) {
		t.Helper()
		if status := p.configure(t, "/Users/Configuration", p.token, configuration); status != http.StatusNoContent {
			t.Fatalf("saving %v: %d", configuration, status)
		}
	}
	p.setting(t, func(s *accounts.Settings) { s.DefaultAudioLanguage, s.DefaultSubtitleLanguage = "fre", "fre" })

	// A new user plays the default track whatever its language
	// (PlayDefaultAudioTrack), an app default: the server's language still
	// ranks above the English track flagged default.
	configure(map[string]any{"PlayDefaultAudioTrack": true, "SubtitleMode": "Always"})
	check("Always, no language of their own", audioFrench, fileFrench)
	// Smart shows subtitles in the default language when the audio is in
	// another one, and only forced ones when it is in that language.
	configure(map[string]any{"PlayDefaultAudioTrack": true, "AudioLanguagePreference": "eng", "SubtitleMode": "Smart"})
	check("Smart, their own English audio", audioEnglish, fileFrench)
	configure(map[string]any{"PlayDefaultAudioTrack": true, "SubtitleMode": "Smart"})
	check("Smart, French audio", audioFrench, -1)
	// OnlyForced shows no forced English track to a French default.
	configure(map[string]any{"PlayDefaultAudioTrack": true, "SubtitleMode": "OnlyForced"})
	check("OnlyForced", audioFrench, -1)

	// The user's own languages win, under Jellyfin's rule: their own
	// French with PlayDefaultAudioTrack plays the flagged English track.
	configure(map[string]any{"PlayDefaultAudioTrack": true, "AudioLanguagePreference": "fre",
		"SubtitleLanguagePreference": "eng", "SubtitleMode": "Always"})
	check("their own languages", audioEnglish, fullEnglish)

	// A server language the file lacks leaves the flagged track.
	p.setting(t, func(s *accounts.Settings) { s.DefaultAudioLanguage = "ger" })
	configure(map[string]any{"PlayDefaultAudioTrack": true, "SubtitleMode": "Always"})
	check("a language the file lacks", audioEnglish, fileFrench)

	// The configuration of a user who chose none shows none, and an app
	// saving it back leaves it so: a changed default still reaches them.
	p.setting(t, func(s *accounts.Settings) { s.DefaultAudioLanguage, s.DefaultSubtitleLanguage = "eng", "eng" })
	configure(map[string]any{"PlayDefaultAudioTrack": false, "SubtitleMode": "Always"})
	dto, raw := p.me(t, p.token)
	if _, present := raw["AudioLanguagePreference"]; present || dto.Configuration.SubtitleLanguagePreference != "" {
		t.Errorf("configuration shown: %v", raw)
	}
	configure(raw)
	p.setting(t, func(s *accounts.Settings) { s.DefaultAudioLanguage, s.DefaultSubtitleLanguage = "fre", "fre" })
	check("after the defaults changed", audioFrench, fileFrench)
	if dto, raw = p.me(t, p.token); dto.Configuration.AudioLanguagePreference != nil || dto.Configuration.SubtitleLanguagePreference != "" {
		t.Errorf("configuration saved back: %v", raw)
	}
}
