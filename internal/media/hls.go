package media

import (
	"bytes"
	"math"
	"slices"
	"strconv"
	"strings"
)

// hlsFormat reports whether ffprobe's container name list is that of an
// HLS playlist.
func hlsFormat(format string) bool {
	return slices.Contains(strings.Split(format, ","), "hls")
}

// firstTrack is the track a play of a source takes: its first video, not a
// cover, else its first audio track.
func firstTrack(streams []Stream) (Stream, bool) {
	for _, s := range streams {
		if s.Type == "video" && !s.AttachedPicture {
			return s, true
		}
	}
	for _, s := range streams {
		if s.Type == "audio" {
			return s, true
		}
	}
	return Stream{}, false
}

// VariantBitrate is the bitrate of an HLS playlist analyzed as analysis
// when played, one of its tracks, plays: the BANDWIDTH of the variant of
// a master playlist played belongs to; else the sum of the bitrates
// ffprobe tells of the tracks of no variant, as those of a media
// playlist, 0 when it tells none.
func VariantBitrate(analysis Analysis, played Stream) int64 {
	if played.Variant > 0 {
		return played.Variant
	}
	var bitrate int64
	for _, s := range analysis.Streams {
		if s.Variant == 0 {
			bitrate += s.Bitrate
		}
	}
	return bitrate
}

// PlaylistBitrate is what an HLS playlist tells of its bitrate beyond what
// FFmpeg reads of it, 0 for nothing. Of a master playlist, it is the
// AVERAGE-BANDWIDTH of the first variant whose BANDWIDTH is bandwidth;
// of a media playlist, whose segments all give their length in bytes
// (EXT-X-BYTERANGE), those bytes over the duration of the segments.
func PlaylistBitrate(playlist []byte, bandwidth int64) int64 {
	var (
		bytesRead, length int64
		seconds           float64
		ranged            = true
		segment           bool
	)
	for read := range bytes.FieldsFuncSeq(playlist, func(r rune) bool { return r == '\r' || r == '\n' || r == 0 }) {
		line := strings.TrimSpace(string(read))
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			attributes := playlistAttributes(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			if bandwidth > 0 && integer(attributes["BANDWIDTH"]) == bandwidth {
				return max(integer(attributes["AVERAGE-BANDWIDTH"]), 0)
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			duration, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			value, err := strconv.ParseFloat(strings.TrimSpace(duration), 64)
			if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
				return 0
			}
			seconds += value
			segment = true
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			size, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXT-X-BYTERANGE:"), "@")
			length = integer(size)
		case line == "" || strings.HasPrefix(line, "#"):
		case segment:
			// A segment's address ends it.
			if length <= 0 {
				ranged = false
			}
			bytesRead += length
			length, segment = 0, false
		}
	}
	if bandwidth > 0 || !ranged || bytesRead <= 0 || seconds <= 0 {
		return 0
	}
	return int64(math.Round(float64(bytesRead*8) / seconds))
}

// playlistAttributes reads the attribute list of an HLS tag, by name:
// NAME=VALUE pairs apart by commas, a value in double quotes holding
// commas too.
func playlistAttributes(list string) map[string]string {
	attributes := map[string]string{}
	for list != "" {
		name, rest, ok := strings.Cut(list, "=")
		if !ok {
			break
		}
		var value string
		if quoted, ok := strings.CutPrefix(rest, `"`); ok {
			value, rest, _ = strings.Cut(quoted, `"`)
			_, rest, _ = strings.Cut(rest, ",")
		} else {
			value, rest, _ = strings.Cut(rest, ",")
		}
		attributes[strings.ToUpper(strings.TrimSpace(name))] = strings.TrimSpace(value)
		list = rest
	}
	return attributes
}
