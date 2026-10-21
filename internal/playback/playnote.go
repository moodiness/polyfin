package playback

import (
	"fmt"
	"slices"

	"github.com/moodiness/polyfin/internal/i18n"
)

// noteWords are the endings of version names telling how a version plays,
// in a language: as it is, copied into HLS, converted for a reason or for
// none named, or not at all.
type noteWords struct {
	directPlay, directStream, converted, convertedFor, cannotPlay string
	hdr, quality, subtitles, video, audio                         string
}

// playNotes are the notes of each server language. The French set the
// colon off with a no-break space, which keeps it on the line of the word
// before; the Spanish, Italian and Portuguese agree with "version", which
// is feminine there.
var playNotes = map[string]noteWords{
	"en": {"(direct play)", "(direct stream)", "(converted)", "(converted: %s)", "(cannot play here)",
		"HDR", "quality limit", "subtitles", "video", "audio"},
	"fr": {"(lecture directe)", "(flux direct)", "(converti)", "(converti\u00a0: %s)", "(illisible ici)",
		"HDR", "limite de qualité", "sous-titres", "vidéo", "audio"},
	"de": {"(direktes Abspielen)", "(direkter Stream)", "(konvertiert)", "(konvertiert: %s)", "(hier nicht abspielbar)",
		"HDR", "Qualitätslimit", "Untertitel", "Video", "Audio"},
	"es": {"(reproducción directa)", "(transmisión directa)", "(convertida)", "(convertida: %s)", "(no se puede reproducir aquí)",
		"HDR", "calidad máxima", "subtítulos", "vídeo", "audio"},
	"it": {"(riproduzione diretta)", "(stream diretto)", "(convertita)", "(convertita: %s)", "(non riproducibile qui)",
		"HDR", "limite qualità", "sottotitoli", "video", "audio"},
	"pt-BR": {"(reprodução direta)", "(stream direto)", "(convertida)", "(convertida: %s)", "(não reproduzível aqui)",
		"HDR", "qualidade", "legendas", "vídeo", "áudio"},
}

// videoReasons are the TranscodeReasons about the video.
var videoReasons = []string{"VideoCodecNotSupported", "VideoProfileNotSupported", "VideoLevelNotSupported", "VideoResolutionNotSupported",
	"VideoBitDepthNotSupported", "VideoFramerateNotSupported", "RefFramesNotSupported", "AnamorphicVideoNotSupported",
	"InterlacedVideoNotSupported", "VideoCodecTagNotSupported", "VideoBitrateNotSupported", "UnknownVideoStreamInfo",
	"VideoRangeTypeNotSupported"}

// PlayNote is the ending of a version's name telling how a decision plays
// it, in the server language: "(direct play)" as it is; "(direct stream)"
// copied into HLS; "(converted: reason)" with the first reason that
// applies of HDR (VideoRangeTypeNotSupported, the video converted), the
// quality limit (VideoResolutionNotSupported, VideoBitrateNotSupported or
// ContainerBitrateExceedsLimit, the video converted), subtitles (a
// subtitle burned into the video), the video (another reason about it),
// or the audio (the audio converted, the video copied); "(converted)" for
// any other reason; and "(cannot play here)" when playable is false: the
// decision needs a conversion the server or the user may not have, or
// none plays the version. It is 30 characters at most.
func PlayNote(decision Decision, playable bool, language string) string {
	w, ok := playNotes[i18n.Match(language)]
	if !ok {
		w = playNotes["en"]
	}
	has := func(reasons ...string) bool {
		return slices.ContainsFunc(decision.Reasons, func(reason string) bool { return slices.Contains(reasons, reason) })
	}
	burned := decision.SubtitleStreamIndex >= 0 && decision.Subtitles[decision.SubtitleStreamIndex].Method == "Encode"
	var reason string
	switch {
	case !playable || !decision.DirectPlay && !decision.Streamed():
		return w.cannotPlay
	case decision.DirectPlay:
		return w.directPlay
	case decision.Video == nil && decision.Audio == nil:
		return w.directStream
	case decision.Video != nil && has("VideoRangeTypeNotSupported"):
		reason = w.hdr
	case decision.Video != nil && has("VideoResolutionNotSupported", "VideoBitrateNotSupported", "ContainerBitrateExceedsLimit"):
		reason = w.quality
	case decision.Video != nil && burned:
		reason = w.subtitles
	case decision.Video != nil && has(videoReasons...):
		reason = w.video
	case decision.Video == nil:
		reason = w.audio
	default:
		return w.converted
	}
	return fmt.Sprintf(w.convertedFor, reason)
}
