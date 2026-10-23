package hls

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Variant describes a stream a master playlist offers.
type Variant struct {
	// Bandwidth is the most the variant reaches, AverageBandwidth what it
	// sends on average, in bits per second; 0 for AverageBandwidth is
	// Bandwidth.
	Bandwidth, AverageBandwidth int64
	// Codecs is the RFC 6381 CODECS attribute; empty leaves it out, and
	// players read the codecs from the media.
	Codecs        string
	Width, Height int
	FrameRate     float64
	// Range is SDR, PQ or HLG.
	Range string
	// URI is the address of its media playlist.
	URI string
}

// WriteMaster writes a master playlist offering variants, in order, with
// the subtitle tracks they share. Players that do not choose
// play the first. Several variants are cut on the same plan, each segment
// starting on a keyframe, so that a player switches between them at any
// segment: the playlist says so with EXT-X-INDEPENDENT-SEGMENTS.
func WriteMaster(w io.Writer, variants []Variant, subtitles []Rendition) error {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	if len(variants) > 1 {
		b.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	}
	if len(subtitles) > 0 {
		writeRenditions(&b, subtitles)
	}
	for _, v := range variants {
		average := v.AverageBandwidth
		if average <= 0 {
			average = v.Bandwidth
		}
		attributes := []string{"BANDWIDTH=" + strconv.FormatInt(v.Bandwidth, 10), "AVERAGE-BANDWIDTH=" + strconv.FormatInt(average, 10)}
		if v.Range != "" {
			attributes = append(attributes, "VIDEO-RANGE="+v.Range)
		}
		if v.Codecs != "" {
			attributes = append(attributes, `CODECS="`+v.Codecs+`"`)
		}
		if v.Width > 0 && v.Height > 0 {
			attributes = append(attributes, fmt.Sprintf("RESOLUTION=%dx%d", v.Width, v.Height))
		}
		if v.FrameRate > 0 {
			attributes = append(attributes, "FRAME-RATE="+strconv.FormatFloat(v.FrameRate, 'f', -1, 64))
		}
		if len(subtitles) > 0 {
			attributes = append(attributes, `SUBTITLES="subs"`)
		}
		fmt.Fprintf(&b, "#EXT-X-STREAM-INF:%s\n%s\n", strings.Join(attributes, ","), v.URI)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// WriteMedia writes the media playlist of a plan: every segment is listed
// from the start, as players seek in it before FFmpeg reaches them. uri
// gives the address of segment n, and of the initialization segment for
// n = -1 when the segments are fragmented MP4.
func WriteMedia(w io.Writer, plan Plan, format Format, uri func(n int) string) error {
	var b strings.Builder
	version := 3
	if format == FMP4 {
		version = 7
	}
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-VERSION:%d\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n", version, plan.TargetDuration())
	if format == FMP4 {
		fmt.Fprintf(&b, "#EXT-X-MAP:URI=\"%s\"\n", uri(-1))
	}
	for n := range plan.Len() {
		fmt.Fprintf(&b, "#EXTINF:%.6f, nodesc\n%s\n", (plan.End(n) - plan.Start(n)).Seconds(), uri(n))
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// Format is the container of the segments.
type Format int

const (
	// FMP4 segments are fragmented MP4, after an initialization segment.
	FMP4 Format = iota
	// TS segments are MPEG transport streams.
	TS
)

// ParseFormat reads a TranscodingProfile container: "mp4" is fragmented
// MP4, "ts" MPEG-TS.
func ParseFormat(container string) (Format, bool) {
	switch strings.ToLower(container) {
	case "mp4":
		return FMP4, true
	case "ts":
		return TS, true
	}
	return 0, false
}

// Extension is the file extension of the segments.
func (f Format) Extension() string {
	if f == TS {
		return "ts"
	}
	return "mp4"
}

// ContentType is the media type of the segments.
func (f Format) ContentType() string {
	if f == TS {
		return "video/mp2t"
	}
	return "video/mp4"
}
