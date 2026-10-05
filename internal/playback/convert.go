package playback

import (
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/hls"
)

// Capabilities returns what the installed FFmpeg converts with, tuned as
// the settings say.
func (s *Service) Capabilities() Capabilities {
	return Capabilities{Encoders: s.segments.Encoders(), ToneMapping: s.segments.HasFilters("zscale", "tonemap"), Hardware: s.segments.Hardware(),
		Tuning: TuningOf(s.settings())}
}

// DecodingHardware is the GPU that decodes video of codec in bitDepth
// bits, as the settings allow, nil for none.
func (s *Service) DecodingHardware(codec string, bitDepth int) *hls.Hardware {
	if hw := s.segments.Hardware(); hw != nil && TuningOf(s.settings()).DecodesOnGPU(codec, bitDepth) {
		return hw
	}
	return nil
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
	// Tuning is how the settings tune conversions.
	Tuning Tuning
}

// AudioConversion is what the audio that plays is converted to when the
// app cannot take it as it is.
type AudioConversion struct {
	// Codec is the FFmpeg encoder: aac, ac3, eac3 or flac.
	Codec    string
	Channels int
	// Bitrate is in bits per second, zero for lossless codecs.
	Bitrate int64
	// Filter is the filter graph the audio goes through, such as a downmix
	// to stereo, empty for none.
	Filter string
}

// audioEncoders are the audio encoders every FFmpeg build has, by
// preference when a profile takes several.
var audioEncoders = []string{"aac", "ac3", "eac3", "flac"}

// ConvertAudio is the conversion of audio of channels in layout, as
// ffprobe names it, for a transcoding profile taking codecs, as a
// comma-separated list, and at most maxChannels channels when it is a
// number: the first codec of the list FFmpeg encodes, with the audio's
// channels up to the limit, to the tuning's and to what the codec carries,
// mixed down to stereo as the tuning says. It is nil when the profile takes
// no codec Polyfin encodes.
func ConvertAudio(codecs, maxChannels string, channels int, layout string, t Tuning) *AudioConversion {
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
	source := channels
	if channels <= 0 {
		channels = 2
	}
	if limit, err := strconv.Atoi(maxChannels); err == nil && limit > 0 {
		channels = min(channels, limit)
	}
	if t.MaxAudioChannels > 0 {
		channels = min(channels, t.MaxAudioChannels)
	}
	// AAC and Dolby Digital go up to 5.1 in the players that take them.
	if codec != "flac" {
		channels = min(channels, 6)
	}
	conversion := &AudioConversion{Codec: codec, Channels: channels}
	switch {
	case codec == "flac":
	case t.AudioBitrate > 0:
		conversion.Bitrate = int64(channels) * t.AudioBitrate
	case channels == 2:
		conversion.Bitrate = 192_000
	default:
		// About 64 kb/s a channel: 192 kb/s in stereo, 384 kb/s in 5.1.
		conversion.Bitrate = max(int64(channels)*64_000, 128_000)
	}
	if channels == 2 && source > 2 {
		conversion.Filter = t.downmix(source, layout)
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
// preference unless the tuning prefers HEVC: H.264 encodes several times
// faster than HEVC, and more apps take it. A GPU's encoder of the same
// codec comes first.
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

// toneMappedHeight caps video converted from HDR to SDR on the processor:
// the conversion costs about four times the encoding at 1080p, and keeps up
// with playback at 720p. A GPU that tone maps has no such cap.
const toneMappedHeight = 720

// ConvertVideo is the conversion of video for a transcoding profile taking
// codecs, as a comma-separated list, within limit bits per second when it
// is positive: the first of H.264 and HEVC the profile takes and FFmpeg
// encodes, H.264 first unless the tuning prefers HEVC, which then follows
// the profile's order, on the GPU when it encodes that codec, at the
// height the limit allows, never larger than the source nor, when it is
// positive, than maxHeight, the bitrate being that height's, converted to
// SDR and deinterlaced as needed. HDR is tone mapped on that GPU when it
// can, Dolby Vision with no base layer other players read (profile 5)
// included, else on the processor, unless the tuning turns tone mapping
// off. It is nil when the profile takes neither codec, and for HDR that
// cannot be converted: on the processor, without FFmpeg's filters, or
// Dolby Vision with no base layer other players read, which without tone
// mapping would show wrong colors.
func ConvertVideo(codecs string, limit int64, maxHeight int, video MediaStream, can Capabilities) *VideoConversion {
	candidates := videoEncoders
	if can.Tuning.PreferHEVC {
		// Jellyfin's AllowHevcEncoding keeps the profile's order, and
		// otherwise moves HEVC last.
		candidates = slices.Clone(videoEncoders)
		slices.SortStableFunc(candidates, func(a, b struct{ codec, encoder string }) int {
			return listIndex(codecs, a.codec) - listIndex(codecs, b.codec)
		})
	}
	conversion := &VideoConversion{}
	for _, candidate := range candidates {
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
	switch {
	case video.VideoRange != "HDR":
	case can.Tuning.NoToneMapping:
		if video.VideoRangeType == "DOVI" {
			return nil
		}
	default:
		if gpu := conversion.Hardware; gpu == nil || !gpu.ToneMapping {
			if video.VideoRangeType == "DOVI" || !can.ToneMapping {
				return nil
			}
			tallest = toneMappedHeight
		}
		conversion.ToneMap = true
	}
	// A source no taller than maxHeight converts as without it.
	if maxHeight > 0 && (video.Height == nil || *video.Height > maxHeight) {
		tallest = min(tallest, maxHeight)
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
