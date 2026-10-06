package jellyfin

import (
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/cache"
	"github.com/moodiness/polyfin/internal/subtitles"
)

// subtitleText is a subtitle track to serve in the format an app asks: its
// cues, and its script when it is ASS, whose styles, positions and fonts no
// other format keeps.
type subtitleText struct {
	cues   []subtitles.Cue
	script *subtitles.Script
}

// readSubtitleText reads a subtitle file: SubRip, WebVTT or an ASS script.
func readSubtitleText(data []byte) (subtitleText, error) {
	if script, err := subtitles.ParseScript(data); err == nil {
		return subtitleText{cues: script.Cues(), script: script}, nil
	}
	cues, err := subtitles.Parse(data)
	return subtitleText{cues: cues}, err
}

// size is about what t takes in memory, as the cache of subtitle texts
// counts it: the length of its cues and script in JSON.
func (t subtitleText) size() int {
	return cache.JSONSize(t.cues) + cache.JSONSize(t.script)
}

// write gives the text from start in format, with its content type: an
// ASS script as it is when the app asks for ASS. Other formats get an ASS
// track's text without its override tags, which apps would show as
// written: Jellyfin keeps them in SubRip and JSON, and drops them only in
// WebVTT. ok is false for a format Polyfin does not write.
func (t subtitleText) write(format string, start time.Duration) (data []byte, contentType string, ok bool) {
	cues := subtitles.From(t.cues, start)
	switch strings.ToLower(format) {
	case "vtt", "webvtt":
		return subtitles.WebVTT(cues), "text/vtt", true
	case "srt", "subrip":
		return subtitles.SubRip(cues), "application/x-subrip", true
	case "js", "json":
		return subtitles.TrackEvents(cues), "application/json", true
	case "ass", "ssa":
		if t.script != nil {
			return t.script.From(start).Bytes(), "text/x-ssa", true
		}
		return subtitles.ASS(cues), "text/x-ssa", true
	}
	return nil, "", false
}
