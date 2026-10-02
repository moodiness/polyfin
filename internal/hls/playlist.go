package hls

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Variant describes the stream a master playlist offers.
type Variant struct {
	// Bandwidth is in bits per second.
	Bandwidth int64
	// Codecs is the RFC 6381 CODECS attribute; empty leaves it out, and
	// players read the codecs from the media.
	Codecs        string
	Width, Height int
	FrameRate     float64
	// Range is SDR, PQ or HLG.
	Range string
	// Subtitles are the subtitle tracks offered with the variant.
	Subtitles []Rendition
}

// WriteMaster writes a master playlist with one variant, whose media
// playlist is at uri.
func WriteMaster(w io.Writer, v Variant, uri string) error {
	attributes := []string{"BANDWIDTH=" + strconv.FormatInt(v.Bandwidth, 10), "AVERAGE-BANDWIDTH=" + strconv.FormatInt(v.Bandwidth, 10)}
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
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	if len(v.Subtitles) > 0 {
		writeRenditions(&b, v.Subtitles)
		attributes = append(attributes, `SUBTITLES="subs"`)
	}
	fmt.Fprintf(&b, "#EXT-X-STREAM-INF:%s\n%s\n", strings.Join(attributes, ","), uri)
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
