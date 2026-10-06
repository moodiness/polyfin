// Package media analyzes video sources with ffprobe: their container,
// duration, bitrate, tracks and chapters.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ErrNotMedia reports a source ffprobe could not read as a video.
var ErrNotMedia = errors.New("not a playable video")

// Analysis is what ffprobe found in a source.
type Analysis struct {
	// Format is ffprobe's container name list, such as "matroska,webm" or
	// "mov,mp4,m4a,3gp,3g2,mj2".
	Format   string        `json:"format"`
	Duration time.Duration `json:"duration"`
	Size     int64         `json:"size,omitempty"`
	// Bitrate is the overall bitrate in bits per second.
	Bitrate int64    `json:"bitrate,omitempty"`
	Streams []Stream `json:"streams"`
	// Chapters are in the order of the file, which is by start.
	Chapters []Chapter `json:"chapters,omitempty"`
	// Remote marks a source ffprobe read over the network rather than from
	// a file: Jellyfin describes the tracks of such sources a little
	// differently.
	Remote bool `json:"remote,omitempty"`
}

// Chapter is a chapter of a source, as its container marks it.
type Chapter struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
	// Title is the chapter's name in the file, empty when it has none.
	Title string `json:"title,omitempty"`
}

// Stream is a track of a source.
type Stream struct {
	Index int `json:"index"`
	// Type is "video", "audio", "subtitle", "attachment" or "data".
	Type     string `json:"type"`
	Codec    string `json:"codec,omitempty"`
	CodecTag string `json:"codecTag,omitempty"`
	Profile  string `json:"profile,omitempty"`
	Bitrate  int64  `json:"bitrate,omitempty"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Default  bool   `json:"default,omitempty"`
	Forced   bool   `json:"forced,omitempty"`
	// HearingImpaired marks subtitles for the deaf and hard of hearing.
	HearingImpaired bool `json:"hearingImpaired,omitempty"`
	// Original marks a track in the work's original language.
	Original bool `json:"original,omitempty"`
	// AttachedPicture marks cover art stored as a video track.
	AttachedPicture bool `json:"attachedPicture,omitempty"`
	// TimeBase is the unit of the track's timestamps, such as "1/1000".
	TimeBase string `json:"timeBase,omitempty"`
	// FileName, MimeType and Comment describe an attached file, such as a
	// font an ASS track uses, or cover art.
	FileName string `json:"fileName,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Comment  string `json:"comment,omitempty"`

	// Video.
	Level          int     `json:"level,omitempty"`
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	PixelFormat    string  `json:"pixelFormat,omitempty"`
	BitDepth       int     `json:"bitDepth,omitempty"`
	FrameRate      float64 `json:"frameRate,omitempty"`
	AverageRate    float64 `json:"averageRate,omitempty"`
	AspectRatio    string  `json:"aspectRatio,omitempty"`
	SampleAspect   string  `json:"sampleAspect,omitempty"`
	ColorRange     string  `json:"colorRange,omitempty"`
	ColorSpace     string  `json:"colorSpace,omitempty"`
	ColorTransfer  string  `json:"colorTransfer,omitempty"`
	ColorPrimaries string  `json:"colorPrimaries,omitempty"`
	FieldOrder     string  `json:"fieldOrder,omitempty"`
	RefFrames      int     `json:"refFrames,omitempty"`
	IsAVC          bool    `json:"isAvc,omitempty"`
	// NalLengthSize is the size of H.264 and HEVC NAL unit lengths, such
	// as "4", when the stream stores them so.
	NalLengthSize string `json:"nalLengthSize,omitempty"`
	// DolbyVision is the Dolby Vision configuration, when present.
	DolbyVision *DolbyVision `json:"dolbyVision,omitempty"`
	// HDR10Plus marks dynamic HDR10+ metadata.
	HDR10Plus bool `json:"hdr10Plus,omitempty"`

	// Audio.
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channelLayout,omitempty"`
	SampleRate    int    `json:"sampleRate,omitempty"`
	SampleBits    int    `json:"sampleBits,omitempty"`
}

// DolbyVision is a Dolby Vision configuration record.
type DolbyVision struct {
	VersionMajor int `json:"versionMajor"`
	VersionMinor int `json:"versionMinor"`
	Profile      int `json:"profile"`
	Level        int `json:"level"`
	// Compatibility is the base layer signal compatibility: 1 HDR10, 2 SDR,
	// 4 HLG, 6 Blu-ray HDR10, 0 none.
	Compatibility int  `json:"compatibility"`
	RPU           bool `json:"rpu"`
	EL            bool `json:"el"`
	BL            bool `json:"bl"`
}

// Prober runs ffprobe.
type Prober struct {
	// Path is the ffprobe executable.
	Path    string
	Timeout time.Duration
}

// Probe analyzes the source at url, which ffprobe reads over HTTP.
func (p Prober) Probe(ctx context.Context, url string) (Analysis, error) {
	// Enough to find every track of a remote file without reading far.
	return p.probe(ctx, url, "-probesize", "20M", "-analyzeduration", "10M")
}

// LiveOptions are the options FFmpeg and ffprobe read a live HLS stream
// with: segments of any name, as some live sources name them oddly. The
// demuxer still checks that each segment's content matches its name.
var LiveOptions = []string{"-allowed_extensions", "ALL", "-allowed_segment_extensions", "ALL"}

// ProbeLive analyzes a live stream at url, an HLS playlist or an endless
// stream, reading about a second of it. It never reconnects: a live
// stream that ends while it is read is a failure, told at once.
func (p Prober) ProbeLive(ctx context.Context, url string) (Analysis, error) {
	data, err := p.run(ctx, url, false, append([]string{"-probesize", "1M", "-analyzeduration", "1M"}, LiveOptions...)...)
	if err != nil {
		return Analysis{}, err
	}
	return Parse(data)
}

// probe runs ffprobe on url with options, reconnecting when the
// connection drops, as remote files need.
func (p Prober) probe(ctx context.Context, url string, options ...string) (Analysis, error) {
	data, err := p.run(ctx, url, true, options...)
	if err != nil {
		return Analysis{}, err
	}
	return Parse(data)
}

// run runs ffprobe on url and returns its JSON output, reconnecting when
// asked.
func (p Prober) run(ctx context.Context, url string, reconnect bool, options ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	args := append([]string{"-v", "error", "-print_format", "json", "-show_format", "-show_streams", "-show_chapters"}, options...)
	if reconnect {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1")
	}
	command := exec.CommandContext(ctx, p.Path, append(args, "-i", url)...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffprobe: %w", ctx.Err())
		}
		// ffprobe's message may quote the URL: keep only its last line, and
		// only once the URL is removed.
		message := strings.TrimSpace(stderr.String())
		if i := strings.LastIndexByte(message, '\n'); i >= 0 {
			message = message[i+1:]
		}
		message = strings.ReplaceAll(message, url, "<source>")
		return nil, fmt.Errorf("%w: ffprobe: %v: %s", ErrNotMedia, err, message)
	}
	return stdout.Bytes(), nil
}

// Parse reads ffprobe's JSON output of a video: one without a video track
// is not one.
func Parse(data []byte) (Analysis, error) {
	analysis, err := parse(data)
	if err != nil {
		return Analysis{}, err
	}
	if !slices.ContainsFunc(analysis.Streams, func(s Stream) bool { return s.Type == "video" && !s.AttachedPicture }) {
		return Analysis{}, fmt.Errorf("%w: no video track", ErrNotMedia)
	}
	return analysis, nil
}

// parse reads ffprobe's JSON output, whatever its tracks.
func parse(data []byte) (Analysis, error) {
	var raw probeOutput
	if err := json.Unmarshal(data, &raw); err != nil {
		return Analysis{}, fmt.Errorf("%w: unreadable ffprobe output: %v", ErrNotMedia, err)
	}
	analysis := Analysis{
		Format:   raw.Format.FormatName,
		Remote:   strings.Contains(raw.Format.Filename, "://"),
		Duration: seconds(raw.Format.Duration),
		Size:     integer(raw.Format.Size),
		Bitrate:  integer(raw.Format.BitRate),
	}
	for _, s := range raw.Streams {
		stream := Stream{
			Index:           s.Index,
			Type:            s.CodecType,
			Codec:           s.CodecName,
			CodecTag:        s.CodecTagString,
			Profile:         s.Profile,
			Bitrate:         integer(s.BitRate),
			Language:        tag(s.Tags, "language"),
			Title:           tag(s.Tags, "title"),
			Default:         s.Disposition["default"] == 1,
			Forced:          s.Disposition["forced"] == 1,
			HearingImpaired: s.Disposition["hearing_impaired"] == 1,
			Original:        s.Disposition["original"] == 1,
			AttachedPicture: s.Disposition["attached_pic"] == 1,
			TimeBase:        s.TimeBase,
			FileName:        tag(s.Tags, "filename"),
			MimeType:        tag(s.Tags, "mimetype"),
			Comment:         tag(s.Tags, "comment"),
			Level:           s.Level,
			Width:           s.Width,
			Height:          s.Height,
			PixelFormat:     s.PixFmt,
			BitDepth:        int(integer(s.BitsPerRawSample)),
			FrameRate:       rate(s.RFrameRate),
			AverageRate:     rate(s.AvgFrameRate),
			AspectRatio:     s.DisplayAspectRatio,
			SampleAspect:    s.SampleAspectRatio,
			ColorRange:      s.ColorRange,
			ColorSpace:      s.ColorSpace,
			ColorTransfer:   s.ColorTransfer,
			ColorPrimaries:  s.ColorPrimaries,
			FieldOrder:      s.FieldOrder,
			RefFrames:       s.Refs,
			IsAVC:           s.IsAVC == "true",
			NalLengthSize:   s.NalLengthSize,
			Channels:        s.Channels,
			ChannelLayout:   s.ChannelLayout,
			SampleRate:      int(integer(s.SampleRate)),
			SampleBits:      s.BitsPerSample,
		}
		if stream.Bitrate == 0 {
			// Matroska keeps track statistics in tags written by mkvmerge.
			stream.Bitrate = integer(tag(s.Tags, "BPS"))
		}
		if stream.Type == "video" && stream.BitDepth == 0 {
			stream.BitDepth = pixelBitDepth(stream.PixelFormat)
		}
		for _, side := range s.SideDataList {
			switch side.Type {
			case "DOVI configuration record":
				stream.DolbyVision = &DolbyVision{
					VersionMajor:  side.DVVersionMajor,
					VersionMinor:  side.DVVersionMinor,
					Profile:       side.DVProfile,
					Level:         side.DVLevel,
					Compatibility: side.DVBLSignalCompatibilityID,
					RPU:           side.RPUPresentFlag == 1,
					EL:            side.ELPresentFlag == 1,
					BL:            side.BLPresentFlag == 1,
				}
			case "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)":
				stream.HDR10Plus = true
			}
		}
		analysis.Streams = append(analysis.Streams, stream)
	}
	for _, c := range raw.Chapters {
		analysis.Chapters = append(analysis.Chapters, Chapter{
			Start: seconds(c.StartTime),
			End:   seconds(c.EndTime),
			Title: tag(c.Tags, "title"),
		})
	}
	return analysis, nil
}

type probeOutput struct {
	Format struct {
		FormatName string `json:"format_name"`
		Filename   string `json:"filename"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index              int               `json:"index"`
		CodecName          string            `json:"codec_name"`
		CodecType          string            `json:"codec_type"`
		CodecTagString     string            `json:"codec_tag_string"`
		Profile            string            `json:"profile"`
		Level              int               `json:"level"`
		Width              int               `json:"width"`
		Height             int               `json:"height"`
		PixFmt             string            `json:"pix_fmt"`
		BitsPerRawSample   string            `json:"bits_per_raw_sample"`
		RFrameRate         string            `json:"r_frame_rate"`
		AvgFrameRate       string            `json:"avg_frame_rate"`
		SampleAspectRatio  string            `json:"sample_aspect_ratio"`
		DisplayAspectRatio string            `json:"display_aspect_ratio"`
		ColorRange         string            `json:"color_range"`
		ColorSpace         string            `json:"color_space"`
		ColorTransfer      string            `json:"color_transfer"`
		ColorPrimaries     string            `json:"color_primaries"`
		FieldOrder         string            `json:"field_order"`
		Refs               int               `json:"refs"`
		IsAVC              string            `json:"is_avc"`
		NalLengthSize      string            `json:"nal_length_size"`
		TimeBase           string            `json:"time_base"`
		BitRate            string            `json:"bit_rate"`
		Channels           int               `json:"channels"`
		ChannelLayout      string            `json:"channel_layout"`
		SampleRate         string            `json:"sample_rate"`
		BitsPerSample      int               `json:"bits_per_sample"`
		Disposition        map[string]int    `json:"disposition"`
		Tags               map[string]string `json:"tags"`
		SideDataList       []struct {
			Type                      string `json:"side_data_type"`
			DVProfile                 int    `json:"dv_profile"`
			DVVersionMajor            int    `json:"dv_version_major"`
			DVVersionMinor            int    `json:"dv_version_minor"`
			DVLevel                   int    `json:"dv_level"`
			RPUPresentFlag            int    `json:"rpu_present_flag"`
			ELPresentFlag             int    `json:"el_present_flag"`
			BLPresentFlag             int    `json:"bl_present_flag"`
			DVBLSignalCompatibilityID int    `json:"dv_bl_signal_compatibility_id"`
		} `json:"side_data_list"`
	} `json:"streams"`
	Chapters []struct {
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

// tag reads a tag whatever its letter case, as containers differ: Matroska
// statistics tags may also carry a language suffix ("BPS-eng").
func tag(tags map[string]string, name string) string {
	for key, value := range tags {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	for key, value := range tags {
		if prefix, _, ok := strings.Cut(key, "-"); ok && strings.EqualFold(prefix, name) {
			return value
		}
	}
	return ""
}

func integer(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

func seconds(value string) time.Duration {
	f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || f < 0 {
		return 0
	}
	return time.Duration(f * float64(time.Second))
}

// rate reads a frame rate written as a fraction.
func rate(value string) float64 {
	numerator, denominator, ok := strings.Cut(value, "/")
	n, err := strconv.ParseFloat(numerator, 64)
	if err != nil {
		return 0
	}
	if !ok {
		return n
	}
	d, err := strconv.ParseFloat(denominator, 64)
	if err != nil || d == 0 {
		return 0
	}
	return n / d
}

// pixelBitDepth infers the bit depth of a pixel format when ffprobe does
// not report it, from names such as "yuv420p10le".
func pixelBitDepth(format string) int {
	for _, depth := range []int{16, 14, 12, 10, 9} {
		if strings.Contains(format, "p"+strconv.Itoa(depth)) {
			return depth
		}
	}
	if format != "" {
		return 8
	}
	return 0
}
