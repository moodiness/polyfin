package silo

import (
	"context"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/playback"
)

// Text subtitles as renditions of a plan's HLS stream.
//
// An app that opts in gets the embedded text tracks of a converted or
// repackaged version as WebVTT SUBTITLES renditions of the HLS stream
// itself, which its player selects like any other track, rather than as
// files it fetches beside the video. Polyfin's conversions already extract
// every text track of the file they read, as WebVTT cut like the video
// (the Jellyfin API's hls1/subtitles{index} playlists): the renditions are
// those playlists, under the names Silo's contract gives them.
//
// The master playlist is then a multivariant playlist naming the video
// playlist and each subtitle playlist as master.m3u8 itself, with
// silo_rendition=video, or silo_rendition=subtitles and the track's
// container number in silo_track, and every segment as segment/{name}: the
// only addresses the Apple app's media credentials admit
// (PlaybackMediaAuthorization.Scope). A plan without renditions answers
// master.m3u8 with the video playlist itself.

// The features of the protocol for subtitle renditions (Silo's
// docs/architecture/playback-protocol-v3.md, section 8): a converted
// stream's selected text track as its rendition; a repackaged one's too;
// every embedded text track as a rendition, each named on its inventory
// row, which the player switches among on the same stream.
const (
	subtitleRenditions    = "hls_subtitle_renditions_v1"
	repackagedRenditions  = "hls_remux_subtitle_renditions_v1"
	allSubtitleRenditions = "hls_subtitle_renditions_all_v1"
)

// The classifications of the failures an app reports of renditions: its
// player could not offer one, which turns them off for the rest of the
// playback; it switched to another rendition itself.
const (
	renditionFailed = "subtitle_hls_rendition_failed"
	trackChanged    = "subtitle_track_changed"
)

const (
	// renditionGroup is the GROUP-ID of the renditions.
	renditionGroup = "subs"
	// maxRenditions bounds the renditions of a stream.
	maxRenditions = 64
	// maxRenditionName bounds a rendition's name, in bytes, leaving room
	// for the suffix that sets it apart within the 256 Silo's apps take.
	maxRenditionName = 232
	// numbersWait bounds the wait for a version's track numbers: one
	// read of a Matroska file's head, done once.
	numbersWait = 3 * time.Second
)

// renditionModes are the renditions an app negotiated at the start of a
// playback, which its replans keep.
type renditionModes struct {
	// single carries the selected text track; repackaged also on a
	// repackaged stream; all, every embedded text track.
	single, repackaged, all bool
}

// negotiated reads the renditions an app's start asks: the others need
// the first.
func negotiated(features []string) renditionModes {
	single := slices.Contains(features, subtitleRenditions)
	return renditionModes{single: single, repackaged: single && slices.Contains(features, repackagedRenditions),
		all: single && slices.Contains(features, allSubtitleRenditions)}
}

// rendition is a subtitle rendition of a stream: track is the container
// number silo_track names it by, 0 for a stream's single rendition;
// stream its Jellyfin index.
type rendition struct {
	track          uint64
	stream         int
	name, language string
	forced         bool
}

// hlsRendition is the selected track as a rendition, as the plan's
// subtitle decision names it (HLSSubtitleRenditionV3); inventoryRendition
// a row's (SubtitleInventoryHLSRenditionV3).
type (
	hlsRendition struct {
		GroupID  string `json:"group_id"`
		Name     string `json:"name"`
		Format   string `json:"format"`
		MimeType string `json:"mime_type"`
		Index    int    `json:"index,omitempty"`
	}
	inventoryRendition struct {
		GroupID string `json:"group_id"`
		Name    string `json:"name"`
		Index   int    `json:"index"`
	}
)

// containerTracks are the container numbers of the tracks of the version
// a Jellyfin play session plays, by FFmpeg index, as long as numbersWait
// allows.
func (s *Server) containerTracks(ctx context.Context, playSession string) map[int]uint64 {
	if s.options.ContainerTracks == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, numbersWait)
	defer cancel()
	return s.options.ContainerTracks(ctx, playSession)
}

// keepsStream reports whether a track change, from the choice before,
// keeps the session's stream, as Silo keeps a stream serving every
// embedded text track (playback-protocol-v3.md, section 8): the stream
// lists them all, the app keeps its quality and audio, and asks one of
// them or none. The app's player then switches on the stream it plays.
func (found *session) keepsStream(before choice) bool {
	c := found.choice
	if !found.allRenditions || found.method == "DirectPlay" || c.quality != before.quality || c.original != before.original ||
		c.renditions != before.renditions {
		return false
	}
	asked := tracksAt(found.source, c)
	if asked.audio != nil && (found.audio == nil || *asked.audio != *found.audio) {
		return false
	}
	return asked.subtitle < 0 || slices.ContainsFunc(found.renditions, func(r rendition) bool { return r.stream == asked.subtitle })
}

// renditionPlan makes the text subtitles of a plan's HLS stream its
// renditions, as the app negotiated them: under all, every embedded text
// track whose container number is known, the first maxRenditions in
// inventory order, the selected one taking the last place when it comes
// later; otherwise, or when the selected track is none of them, the
// selected embedded text track alone. A rendition replaces the selected
// track's file, which its row keeps as the fallback; picture subtitles, and
// the files beside the video, keep their files or burn-in. numbers are the
// version's container track numbers, streams its subtitles in inventory
// order.
func renditionPlan(found *session, p *plan, streams []playback.MediaStream, numbers map[int]uint64) {
	found.renditions, found.allRenditions = nil, false
	modes := found.choice.renditions
	if !modes.single || p.Subtitle.Mode == "burn_in" || p.Delivery != converted && !(p.Delivery == repackaged && modes.repackaged) {
		return
	}
	selected := -1
	if p.SelectedTracks.Subtitle != nil && p.SelectedTracks.Subtitle.Index != nil {
		selected = *p.SelectedTracks.Subtitle.Index
	}
	carried := func(n int) bool {
		row := p.Subtitle.Inventory[n]
		return row.Source == "embedded" && row.Delivery == "sidecar" && !row.Unplayable && strings.TrimSpace(row.Codec) != "" &&
			streams[n].IsTextSubtitleStream
	}
	var listed []int
	if modes.all {
		for n := range p.Subtitle.Inventory {
			if row := p.Subtitle.Inventory[n]; carried(n) && row.StreamIndex != nil && numbers[*row.StreamIndex] > 0 {
				listed = append(listed, n)
			}
		}
		if len(listed) > maxRenditions {
			last := listed[maxRenditions-1]
			if slices.Contains(listed[maxRenditions:], selected) {
				last = selected
			}
			listed = append(listed[:maxRenditions-1], last)
		}
	}
	if selected >= 0 && carried(selected) && !slices.Contains(listed, selected) {
		// The selected track is not among them: it is the stream's one
		// rendition.
		row := p.Subtitle.Inventory[selected]
		found.renditions = []rendition{{stream: streams[selected].Index, name: "Subtitles", language: row.Language, forced: row.Forced}}
		p.Subtitle.HLSRendition = &hlsRendition{GroupID: renditionGroup, Name: "Subtitles", Format: "vtt", MimeType: "text/vtt"}
		p.Subtitle.Artifact = nil
		return
	}
	if len(listed) == 0 {
		return
	}
	found.allRenditions = true
	names := map[string]bool{}
	for i, n := range listed {
		row := &p.Subtitle.Inventory[n]
		name := renditionName(row.Label, row.Language, names)
		found.renditions = append(found.renditions, rendition{track: numbers[*row.StreamIndex], stream: streams[n].Index, name: name,
			language: row.Language, forced: row.Forced})
		row.HLSRendition = &inventoryRendition{GroupID: renditionGroup, Name: name, Index: i + 1}
		if n == selected {
			p.Subtitle.HLSRendition = &hlsRendition{GroupID: renditionGroup, Name: name, Format: "vtt", MimeType: "text/vtt", Index: i + 1}
			p.Subtitle.Artifact = nil
		}
	}
}

// renditionName names a rendition as Silo does: the row's label, else its
// language, else Subtitles, with quotes made apostrophes and control
// characters dropped, cut to maxRenditionName bytes, and numbered " (2)",
// " (3)"... when another of the group has its name. taken are the names
// given, which it adds to.
func renditionName(label, language string, taken map[string]bool) string {
	base := strings.Map(func(r rune) rune {
		switch {
		case r == '"':
			return '\''
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, strings.TrimSpace(label))
	if base == "" {
		base = language
	}
	if base == "" {
		base = "Subtitles"
	}
	for len(base) > maxRenditionName {
		_, size := utf8.DecodeLastRuneInString(base)
		base = base[:len(base)-size]
	}
	name := base
	for n := 2; taken[name]; n++ {
		name = base + " (" + strconv.Itoa(n) + ")"
	}
	taken[name] = true
	return name
}

// languageAttribute is a rendition's LANGUAGE, none for an undetermined
// language or one that is not a tag.
func languageAttribute(tag string) string {
	if tag == "" || strings.EqualFold(tag, "und") || strings.ContainsFunc(tag, func(r rune) bool {
		return !(r == '-' || r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	}) {
		return ""
	}
	return tag
}

func yes(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

// multivariant writes the master playlist of a stream with renditions:
// the Jellyfin API's master playlist's header and its first variant,
// whose playlist is master.m3u8 with silo_rendition=video, with the
// renditions in one group; the selected one is shown by default. Every
// rendition is selected automatically, so a player following the
// system's caption preferences may show one: the apps turn them off
// themselves when the plan shows none.
func multivariant(jellyfin string, renditions []rendition, selected *int) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	var variant string
	for line := range strings.SplitSeq(jellyfin, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "#EXT-X-VERSION:"), line == "#EXT-X-INDEPENDENT-SEGMENTS":
			b.WriteString(line + "\n")
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:") && variant == "":
			variant = line
		}
	}
	for _, r := range renditions {
		attributes := []string{"TYPE=SUBTITLES", `GROUP-ID="` + renditionGroup + `"`, `NAME="` + r.name + `"`}
		if language := languageAttribute(r.language); language != "" {
			attributes = append(attributes, `LANGUAGE="`+language+`"`)
		}
		uri := "master.m3u8?silo_rendition=subtitles"
		if r.track > 0 {
			uri += "&silo_track=" + strconv.FormatUint(r.track, 10)
		}
		attributes = append(attributes, "DEFAULT="+yes(selected != nil && *selected == r.stream), "AUTOSELECT=YES", "FORCED="+yes(r.forced),
			`URI="`+uri+`"`)
		b.WriteString("#EXT-X-MEDIA:" + strings.Join(attributes, ",") + "\n")
	}
	if variant == "" {
		variant = "#EXT-X-STREAM-INF:BANDWIDTH=1000000"
	}
	b.WriteString(withSubtitles(variant) + "\nmaster.m3u8?silo_rendition=video\n")
	return b.String()
}

// withSubtitles is a variant's EXT-X-STREAM-INF naming the renditions'
// group, in place of any other.
func withSubtitles(variant string) string {
	name, list, _ := strings.Cut(variant, ":")
	var kept []string
	for _, attribute := range splitAttributes(list) {
		if !strings.HasPrefix(attribute, "SUBTITLES=") {
			kept = append(kept, attribute)
		}
	}
	return name + ":" + strings.Join(append(kept, `SUBTITLES="`+renditionGroup+`"`), ",")
}

// splitAttributes splits an attribute list at the commas outside quotes.
func splitAttributes(list string) []string {
	var attributes []string
	start, quoted := 0, false
	for i, r := range list {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			attributes = append(attributes, list[start:i])
			start = i + 1
		}
	}
	if start < len(list) {
		attributes = append(attributes, list[start:])
	}
	return attributes
}

// segmentURI matches the URI of a segment of the Jellyfin API's
// playlists, once its folder and query are taken off: its number and
// extension, the initialization segments being negative.
var segmentURI = regexp.MustCompile(`^-?[0-9]+\.[A-Za-z0-9]+$`)

// segmentsUnder rewrites a media playlist of the Jellyfin API so that it
// names each segment segment/{name}, name being what named gives the
// segment's file: the segment's number and extension.
func segmentsUnder(playlist string, named func(file string) string) string {
	rename := func(uri string) string {
		file := path.Base(strings.SplitN(uri, "?", 2)[0])
		if !segmentURI.MatchString(file) {
			return uri
		}
		return "segment/" + named(file)
	}
	lines := strings.Split(playlist, "\n")
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			if start := strings.Index(line, `URI="`); start >= 0 {
				start += len(`URI="`)
				if end := strings.IndexByte(line[start:], '"'); end >= 0 {
					line = line[:start] + rename(line[start:start+end]) + line[start+end:]
				}
			}
		case line != "" && !strings.HasPrefix(line, "#"):
			line = rename(line)
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// subtitleSegment reads a subtitle rendition's segment name: sub_{n}.vtt
// for a stream's single rendition, sub_{track}_{n}.vtt for the rendition
// of a container track (track 0 for the first form).
func subtitleSegment(name string) (track uint64, n int, ok bool) {
	rest, isSubtitle := strings.CutPrefix(name, "sub_")
	rest, isVTT := strings.CutSuffix(rest, ".vtt")
	if !isSubtitle || !isVTT {
		return 0, 0, false
	}
	if before, after, two := strings.Cut(rest, "_"); two {
		var err error
		if track, err = strconv.ParseUint(before, 10, 64); err != nil || track == 0 || strconv.FormatUint(track, 10) != before {
			return 0, 0, false
		}
		rest = after
	}
	n, err := strconv.Atoi(rest)
	return track, n, err == nil && n >= 0 && strconv.Itoa(n) == rest
}

// renditionOf is the rendition of a stream silo_track names, or its single
// rendition when it names none.
func renditionOf(renditions []rendition, track uint64) (rendition, bool) {
	i := slices.IndexFunc(renditions, func(r rendition) bool { return r.track == track })
	if i < 0 {
		return rendition{}, false
	}
	return renditions[i], true
}

// servePlaylist answers a playlist of a stream: the Jellyfin API's
// playlist at target, rewritten by rewrite.
func (s *Server) servePlaylist(w http.ResponseWriter, r *http.Request, found mediaView, target string, rewrite func(string) string) {
	a, err := s.callAs(r, found.device, found.token, http.MethodGet, target, nil, nil)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	w = private(w)
	if !a.ok() {
		w.WriteHeader(a.status)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(rewrite(string(a.body))))
	}
}
