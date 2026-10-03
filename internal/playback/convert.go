package playback

import (
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/hls"
)

// Capabilities returns what the installed FFmpeg converts with.
func (s *Service) Capabilities() Capabilities {
	return Capabilities{Encoders: s.segments.Encoders(), ToneMapping: s.segments.HasFilters("zscale", "tonemap"), Hardware: s.segments.Hardware()}
}

// Capabilities are what the installed FFmpeg converts with.
type Capabilities struct {
	// Encoders are the FFmpeg encoders present, such as libx264.
	Encoders []string
	// ToneMapping is set when FFmpeg has the filters converting HDR to SDR:
	// zscale and tonemap.
	ToneMapping bool
	// Hardware is the GPU video is converted on, nil for none.
	Hardware *hls.Hardware
}

// AudioConversion is what the audio that plays is converted to when the
// app cannot take it as it is.
type AudioConversion struct {
	// Codec is the FFmpeg encoder: aac, ac3, eac3 or flac.
	Codec    string
	Channels int
	// Bitrate is in bits per second, zero for lossless codecs.
	Bitrate int64
}

// audioEncoders are the audio encoders every FFmpeg build has, by
// preference when a profile takes several.
var audioEncoders = []string{"aac", "ac3", "eac3", "flac"}

// ConvertAudio is the conversion of audio for a transcoding profile taking
// codecs, as a comma-separated list, and at most maxChannels channels when
// it is a number: the first codec of the list FFmpeg encodes, with the
// audio's channels up to the limit and to what the codec carries. It is
// nil when the profile takes no codec Polyfin encodes.
func ConvertAudio(codecs, maxChannels string, channels int) *AudioConversion {
	codec := ""
	for name := range strings.SplitSeq(codecs, ",") {
		if name = strings.ToLower(strings.TrimSpace(name)); slices.Contains(audioEncoders, name) {
			codec = name
			break
		}
	}
	if codec == "" {
		return nil
	}
	if channels <= 0 {
		channels = 2
	}
	if limit, err := strconv.Atoi(maxChannels); err == nil && limit > 0 {
		channels = min(channels, limit)
	}
	// AAC and Dolby Digital go up to 5.1 in the players that take them.
	if codec != "flac" {
		channels = min(channels, 6)
	}
	conversion := &AudioConversion{Codec: codec, Channels: channels}
	if codec != "flac" {
		// About 64 kb/s a channel: 192 kb/s in stereo, 384 kb/s in 5.1.
		conversion.Bitrate = max(int64(channels)*64_000, 128_000)
		if channels == 2 {
			conversion.Bitrate = 192_000
		}
	}
	return conversion
}

// VideoConversion is what the video is converted to when the app cannot
// take it as it is: 8-bit SDR, progressive.
type VideoConversion struct {
	// Codec is h264 or hevc; Encoder, the FFmpeg encoder writing it.
	Codec, Encoder string
	Width, Height  int
	// Bitrate is the average the encoder aims for, in bits per second.
	Bitrate int64
	// ToneMap converts HDR to SDR; Deinterlace, interlaced video to
	// progressive.
	ToneMap, Deinterlace bool
	// Hardware is the GPU Encoder belongs to, nil for software.
	Hardware *hls.Hardware
}

// videoEncoders are the software encoders Polyfin converts video with, by
// preference: H.264 encodes several times faster than HEVC, and more apps
// take it. A GPU's encoder of the same codec comes first.
var videoEncoders = []struct{ codec, encoder string }{{"h264", "libx264"}, {"hevc", "libx265"}}

// rungs are the heights video is converted to by software encoders, with
// the least video bitrate each needs and the most it uses. Software
// encoding stops at 1080p to keep up with playback.
var rungs = []struct {
	height         int
	least, ceiling int64
}{
	{1080, 6_000_000, 10_000_000},
	{720, 3_000_000, 5_000_000},
	{540, 1_500_000, 3_000_000},
	{480, 900_000, 2_000_000},
	{360, 0, 1_000_000},
}

// toneMappedHeight caps video converted from HDR to SDR in software: the
// conversion costs about four times the encoding at 1080p, and keeps up
// with playback at 720p.
const toneMappedHeight = 720

// ConvertVideo is the conversion of video for a transcoding profile taking
// codecs, as a comma-separated list, within limit bits per second when it
// is positive: the first of H.264 and HEVC the profile takes and FFmpeg
// encodes, on the GPU when it encodes that codec, at the height the limit
// allows, never larger than the source, converted to SDR and deinterlaced
// as needed. It is nil when the profile takes neither, and for HDR the
// installed FFmpeg cannot convert: without its filters, or Dolby Vision
// with no base layer other players read.
func ConvertVideo(codecs string, limit int64, video MediaStream, can Capabilities) *VideoConversion {
	conversion := &VideoConversion{}
	for _, candidate := range videoEncoders {
		if !listHas(codecs, candidate.codec) {
			continue
		}
		if hw := can.Hardware; hw != nil {
			if i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, candidate.codec+"_") }); i >= 0 {
				conversion.Codec, conversion.Encoder, conversion.Hardware = candidate.codec, hw.Encoders[i], hw
				break
			}
		}
		if slices.Contains(can.Encoders, candidate.encoder) {
			conversion.Codec, conversion.Encoder = candidate.codec, candidate.encoder
			break
		}
	}
	if conversion.Codec == "" {
		return nil
	}
	tallest := rungs[0].height
	if video.VideoRange == "HDR" {
		if video.VideoRangeType == "DOVI" || !can.ToneMapping {
			return nil
		}
		conversion.ToneMap = true
		tallest = toneMappedHeight
	}
	conversion.Deinterlace = video.IsInterlaced
	rung := rungs[len(rungs)-1]
	for _, r := range rungs {
		if r.height <= tallest && (limit <= 0 || limit >= r.least) {
			rung = r
			break
		}
	}
	height, width := rung.height, 0
	if video.Height != nil && video.Width != nil && *video.Height > 0 {
		height = min(height, *video.Height)
		width = *video.Width * height / *video.Height
	} else {
		width = height * 16 / 9
	}
	// Encoders take even sizes.
	conversion.Width, conversion.Height = width/2*2, height/2*2
	conversion.Bitrate = rung.ceiling
	if limit > 0 {
		conversion.Bitrate = min(conversion.Bitrate, limit)
	}
	if video.BitRate != nil && *video.BitRate > 0 {
		conversion.Bitrate = min(conversion.Bitrate, *video.BitRate)
	}
	conversion.Bitrate = max(conversion.Bitrate, 300_000)
	return conversion
}

// Level is the codec level a conversion declares, for video of a frame
// rate: H.264 4.1, or 4.2 above 30 frames a second, which cover 1080p;
// HEVC 4.1.
func (v *VideoConversion) Level(rate float64) string {
	if v.Codec == "h264" && rate > 30 {
		return "4.2"
	}
	return "4.1"
}

// CodecString is the RFC 6381 codec string of a conversion's video: H.264
// High or HEVC Main at its level.
func (v *VideoConversion) CodecString(rate float64) string {
	level := v.Level(rate)
	if v.Codec == "hevc" {
		// HEVC levels count in thirtieths: 4.1 is 123.
		return "hvc1.1.6.L" + strconv.Itoa(int(level[0]-'0')*30+int(level[2]-'0')*3) + ".B0"
	}
	return "avc1.6400" + strings.ToUpper(strconv.FormatInt(int64((level[0]-'0')*10+(level[2]-'0')), 16))
}
