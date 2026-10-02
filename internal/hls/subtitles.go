package hls

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/subtitles"
)

// Extracted keeps what remuxes extract of a version's text subtitles: the
// cues of each stream, and the spans of time over which every cue starting
// there was extracted. The remuxes of a version share it.
type Extracted interface {
	// Add records a cue of an extracted stream.
	Add(stream int, cue subtitles.Cue)
	// Cover records that the cues of every extracted stream starting in
	// [from, to) were added.
	Cover(from, to time.Duration)
	// Covers reports whether [from, to) is covered.
	Covers(from, to time.Duration) bool
}

// Rendition is a subtitle track a master playlist offers.
type Rendition struct {
	Name     string
	Language string
	// Default marks the track shown unless the viewer chooses another;
	// Forced, a track for foreign-language passages only.
	Default, Forced bool
	URI             string
}

// writeRenditions writes the subtitle renditions of a master playlist, in
// the group the variant names. Names must differ within a group: tracks
// named alike, such as several files in one language, are numbered.
func writeRenditions(b *strings.Builder, renditions []Rendition) {
	seen := make(map[string]int, len(renditions))
	for _, r := range renditions {
		name := r.Name
		if seen[r.Name]++; seen[r.Name] > 1 {
			name += " " + strconv.Itoa(seen[r.Name])
		}
		attributes := []string{`TYPE=SUBTITLES`, `GROUP-ID="subs"`, `NAME=` + quoted(name)}
		if r.Language != "" {
			attributes = append(attributes, `LANGUAGE=`+quoted(r.Language))
		}
		// A default track is selected automatically; a forced one is shown
		// with the audio of its language.
		attributes = append(attributes, "DEFAULT="+yes(r.Default), "AUTOSELECT="+yes(r.Default || r.Forced), "FORCED="+yes(r.Forced),
			`URI=`+quoted(r.URI))
		b.WriteString("#EXT-X-MEDIA:" + strings.Join(attributes, ",") + "\n")
	}
}

// quoted writes a quoted-string attribute: HLS has no escapes, so quotes
// and line breaks become spaces.
func quoted(s string) string {
	return `"` + strings.NewReplacer(`"`, "'", "\n", " ", "\r", " ").Replace(s) + `"`
}

func yes(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// WriteSubtitlePlaylist writes the media playlist of a subtitle track, cut
// like the video: uri gives the address of segment n.
func WriteSubtitlePlaylist(w io.Writer, plan Plan, uri func(n int) string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", plan.TargetDuration())
	for n := range plan.Len() {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", (plan.End(n) - plan.Start(n)).Seconds(), uri(n))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// SubtitleSegment writes segment n of a subtitle track: the cues shown
// during it, in the source's time. Its timestamp map places them on the
// timeline of the remux, whose timestamps are the source's shifted by
// timestampOffset.
func SubtitleSegment(cues []subtitles.Cue, plan Plan, n int) []byte {
	from, to := plan.Start(n), plan.End(n)
	shown := make([]subtitles.Cue, 0, 8)
	for _, cue := range cues {
		if cue.End > from && cue.Start < to {
			shown = append(shown, cue)
		}
	}
	ticks := int64(timestampOffset / time.Second * 90000)
	return subtitles.WebVTT(shown, "X-TIMESTAMP-MAP=MPEGTS:"+strconv.FormatInt(ticks, 10)+",LOCAL:00:00:00.000")
}
