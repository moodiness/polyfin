package playback

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/hls"
	"github.com/moodiness/polyfin/internal/media"
)

// Capabilities returns what the installed FFmpeg converts with, tuned as
// the settings say.
func (s *Service) Capabilities() Capabilities {
	settings := s.settings()
	hw := s.segments.Hardware()
	return Capabilities{Encoders: s.segments.Encoders(), ToneMapping: s.segments.HasFilters("zscale", "tonemap"), Hardware: hw,
		ToneMappedHeight: cmp.Or(settings.ProcessorToneMappingHeight, s.segments.ToneMappedHeight()), HDR: s.segments.HDRPath(hw),
		Tuning: TuningOf(settings)}
}

// DecodingHardware is the GPU that decodes video of codec in bitDepth
// bits, as the settings allow, nil for none.
func (s *Service) DecodingHardware(codec string, bitDepth int) *hls.Hardware {
	if hw := s.segments.Hardware(); hw != nil && TuningOf(s.settings()).DecodesOnGPU(codec, bitDepth) {
		return hw
	}
	return nil
}

// ConvertsOnProcessor reports whether the video of a playback, a channel
// or a download is converted on the processor now, or tone mapped there.
func (s *Service) ConvertsOnProcessor() bool {
	return slices.ContainsFunc(s.segments.Running(), func(r hls.Running) bool {
		return r.Video != nil && (r.Video.Hardware == nil || r.Video.ToneMapOnCPU)
	})
}

// Capabilities are what the installed FFmpeg converts with.
type Capabilities struct {
	// Encoders are the FFmpeg encoders present, such as libx264.
	Encoders []string
	// ToneMapping is set when FFmpeg has the filters converting HDR to SDR:
	// zscale and tonemap.
	ToneMapping bool
	// ToneMappedHeight caps video the processor tone maps: the settings'
	// ProcessorToneMappingHeight, or with Automatic the height a timing at
	// startup chose (see hls.Manager.ToneMappedHeight). 0 is 720p.
	ToneMappedHeight int
	// Hardware is the GPU video is converted on, nil for none.
	Hardware *hls.Hardware
	// HDR is where HDR Hardware would tone map is tone mapped, as the
	// timings at startup chose it (see hls.HDRPath).
	HDR hls.HDRPath
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
	// Decoder are the options of the source's decoder, as names and values
	// (see hls.Remux.AudioDecoder).
	Decoder []string
}

// audioEncoders are the audio encoders every FFmpeg build has, by
// preference when a profile takes several.
var audioEncoders = []string{"aac", "ac3", "eac3", "flac"}

// ConvertAudio is the conversion of a video's audio track, source, for a
// transcoding profile taking codecs, as a comma-separated list, and at
// most maxChannels channels when it is a number, under a bitrate limit, 0
// for none: the first codec of the list FFmpeg encodes, a lossy one first
// under a limit, which bounds only those, with the audio's channels up to
// the limit, to the tuning's and to what the codec carries, through the
// tuning's filters (see Tuning.videoAudio), at the tuning's bitrate a
// channel, else at Jellyfin's (see audioBitrateFor). It is nil when the
// profile takes no codec Polyfin encodes.
func ConvertAudio(codecs, maxChannels string, source AudioSource, limit int64, t Tuning) *AudioConversion {
	codec := ""
	for name := range strings.SplitSeq(codecs, ",") {
		if name = strings.ToLower(strings.TrimSpace(name)); slices.Contains(audioEncoders, name) && (codec == "" || codec == "flac") {
			codec = name
			if limit <= 0 || name != "flac" {
				break
			}
		}
	}
	if codec == "" {
		return nil
	}
	channels := source.Channels
	if channels <= 0 {
		channels = 2
	}
	if most, err := strconv.Atoi(maxChannels); err == nil && most > 0 {
		channels = min(channels, most)
	}
	if t.MaxAudioChannels > 0 {
		channels = min(channels, t.MaxAudioChannels)
	}
	// AAC and Dolby Digital go up to 5.1 in the players that take them.
	if codec != "flac" {
		channels = min(channels, 6)
	}
	conversion := &AudioConversion{Codec: codec, Channels: channels, Filter: t.videoAudio(source, channels, 0), Decoder: t.decoder(source.Codec)}
	switch {
	case codec == "flac":
	case t.AudioBitrate > 0:
		conversion.Bitrate = int64(channels) * t.AudioBitrate
	default:
		conversion.Bitrate = audioBitrateFor(channels, limit)
	}
	return conversion
}

// AudioTrack is what ConvertAudio reads of an analyzed audio track.
func AudioTrack(s media.Stream) AudioSource {
	return AudioSource{Codec: s.Codec, Channels: s.Channels, ChannelLayout: s.ChannelLayout, SampleRate: s.SampleRate}
}

// audioBitrateFor is the bitrate Jellyfin's StreamBuilder gives audio
// converted to a lossy codec with channels: 128 kb/s in mono, 384 kb/s up
// to five channels, 640 kb/s from six; under a bitrate limit, at most its
// share of the limit (see audioShare).
func audioBitrateFor(channels int, limit int64) int64 {
	bitrate := int64(384_000)
	switch {
	case channels < 2:
		bitrate = 128_000
	case channels >= 6:
		bitrate = 640_000
	}
	if limit > 0 {
		bitrate = min(bitrate, audioShare(limit))
	}
	return bitrate
}

// audioShare is the most audio gets under a bitrate limit, as Jellyfin's
// StreamBuilder gives it: 128 kb/s up to 640 kb/s, 384 kb/s up to 2 Mb/s,
// 640 kb/s up to 4 Mb/s, and more above, beyond what converted audio gets.
func audioShare(limit int64) int64 {
	for _, share := range []struct{ limit, most int64 }{
		{640_000, 128_000}, {2_000_000, 384_000}, {3_000_000, 448_000}, {4_000_000, 640_000}, {5_000_000, 768_000},
		{10_000_000, 1_536_000}, {15_000_000, 2_304_000}, {20_000_000, 3_584_000},
	} {
		if limit <= share.limit {
			return share.most
		}
	}
	return 7_168_000
}

// VideoConversion is what the video is converted to when the app cannot
// take it as it is: 8-bit SDR, progressive.
type VideoConversion struct {
	// Codec is h264 or hevc; Encoder, the FFmpeg encoder writing it.
	Codec, Encoder string
	Width, Height  int
	// Bitrate is the average the encoder aims for, in bits per second, and
	// Peak the most the video may reach, when a bitrate limit allows less
	// than one and a half times Bitrate, 0 otherwise (see ConvertVideo).
	Bitrate, Peak int64
	// ToneMap converts HDR to SDR; Deinterlace, interlaced video to
	// progressive.
	ToneMap, Deinterlace bool
	// ToneMapOnCPU tone maps HDR on the processor though Hardware could
	// (see toneMapsOnGPU). ToneMapReason tells why, or why Hardware tone
	// maps below the height it would convert at: one of hls's ToneMap
	// reasons, empty when neither.
	ToneMapOnCPU  bool
	ToneMapReason string
	// Hardware is the GPU Encoder belongs to, nil for software.
	Hardware *hls.Hardware
	// Size is what set Width and Height.
	Size ConversionSize
	// Profile is the H.264 profile written when the app's allows no High:
	// main or baseline (see AppCaps.h264Profile). Empty writes High, and
	// HEVC is always Main.
	Profile string
	// MaxLevel is the highest level the app decodes, in tenths, 0 when
	// its profile does not say; FrameRate, the frame rate the video is
	// brought down to, 0 to keep the source's.
	MaxLevel  int
	FrameRate float64
}

// AppCaps are what an app's profile allows video converted for it, read
// as Jellyfin's StreamBuilder reads them off the conditions of the codec
// profiles of the codecs it converts to, and of its transcoding profile
// (see decider.appCaps): the largest frame, the most bitrate, the most
// frames a second, the highest level of each codec, and the H.264
// profiles allowed. Zero values and nil allow anything.
type AppCaps struct {
	Width, Height int
	Bitrate       int64
	FrameRate     float64
	// Levels are by codec, h264 and hevc, in Jellyfin's numbers: H.264's
	// in tenths (41 is 4.1), HEVC's in thirtieths (153 is 5.1).
	Levels map[string]int
	// H264Profiles are lowercased without spaces, as "constrainedbaseline".
	H264Profiles []string
}

// framed reports whether caps limit the frame.
func (c AppCaps) framed() bool {
	return c.Width > 0 || c.Height > 0
}

// fit is the size of video of width by height brought within the caps'
// frame keeping its shape, as Jellyfin's EncodingHelper.GetFixedOutputSize
// does, in even numbers; the size itself when it fits.
func (c AppCaps) fit(width, height int) (int, int) {
	scale := 1.0
	if c.Width > 0 && width > c.Width {
		scale = float64(c.Width) / float64(width)
	}
	if c.Height > 0 && height > c.Height {
		scale = min(scale, float64(c.Height)/float64(height))
	}
	if scale >= 1 {
		return width / 2 * 2, height / 2 * 2
	}
	w, h := int(math.Round(float64(width)*scale)), int(math.Round(float64(height)*scale))
	if c.Width > 0 {
		w = min(w, c.Width)
	}
	if c.Height > 0 {
		h = min(h, c.Height)
	}
	return w / 2 * 2, h / 2 * 2
}

// level is the highest level the caps allow codec, in tenths, 0 for no
// cap.
func (c AppCaps) level(codec string) int {
	n := c.Levels[codec]
	if codec == "hevc" {
		n /= 3
	}
	return n
}

// h264Profile is the H.264 profile a conversion writes: the highest of
// high, main and baseline the caps allow, baseline standing for
// constrained baseline too; high when they set no condition, or allow none
// of the three.
func (c AppCaps) h264Profile() string {
	for _, profile := range []string{"high", "main", "baseline"} {
		for _, allowed := range c.H264Profiles {
			if allowed == profile || profile == "baseline" && allowed == "constrainedbaseline" {
				return profile
			}
		}
	}
	return "high"
}

// Limits are what converted video is scaled down within, beside what the
// server converts at most.
type Limits struct {
	// Bitrate is the bits per second the playback is limited to, 0 for no
	// limit, and Video what it leaves the video (see VideoLimit). ByUser
	// tells that Bitrate is the user's MaxBitrate rather than the app's
	// MaxStreamingBitrate.
	Bitrate, Video int64
	ByUser         bool
	// Group is the user's quality group and Server the settings'
	// MaxConversionHeight, 0 when it does not limit.
	Group, Server int
	// App is what the app's profile allows.
	App AppCaps
}

// What sets the size of converted video, as ConversionSize.Reason tells
// it.
const (
	// SizeSource: no cap is below the source's size.
	SizeSource = "source"
	// SizeGPU: a GPU converts up to 4K.
	SizeGPU = "gpu"
	// SizeProcessor: the processor converts up to 1080p.
	SizeProcessor = "processor"
	// SizeToneMapping: the processor converts HDR to SDR up to
	// Capabilities.ToneMappedHeight.
	SizeToneMapping = "toneMapping"
	// SizeGPUToneMapping: the GPU converts HDR to SDR up to the height
	// where it keeps up (see hls.HDRPath).
	SizeGPUToneMapping = "gpuToneMapping"
	// SizeQualityGroup: the user's quality group.
	SizeQualityGroup = "qualityGroup"
	// SizeServerMax: the settings' MaxConversionHeight.
	SizeServerMax = "serverMax"
	// SizeBitrate: the bitrate limit allows no taller rung.
	SizeBitrate = "bitrate"
	// SizeApp: the app's profile allows no larger frame, or no higher
	// codec level.
	SizeApp = "app"
)

// ConversionSize is what set the size of converted video: the lowest of
// the caps that made it smaller than the source. On a tie, the user's
// quality group comes first, then the settings' MaxConversionHeight, then
// the app's profile, then the server's own caps: the settings would hold
// on a stronger server. The bitrate is the reason only when it allows
// less than all of them.
type ConversionSize struct {
	// Reason is one of the Size constants; MaxHeight, the height it caps
	// video at, 0 for SizeSource.
	Reason    string
	MaxHeight int
	// Bitrate is, for SizeBitrate, the limit in bits per second, and
	// ByUser tells it is the user's MaxBitrate rather than the app's
	// MaxStreamingBitrate.
	Bitrate int64
	ByUser  bool
}

// videoEncoders are the software encoders Polyfin converts video with, by
// preference unless the tuning prefers HEVC: H.264 encodes several times
// faster than HEVC, and more apps take it. A GPU's encoder of the same
// codec comes first.
var videoEncoders = []struct{ codec, encoder string }{{"h264", "libx264"}, {"hevc", "libx265"}}

// rungs are the heights video is converted to, with the least video bitrate
// each needs and its usual one, which a conversion gets when its source's
// bitrate is not known (see videoBitrate), and the lower qualities offered
// to step down to (see Steps). A GPU converts up to 4K; the processor
// stops at softwareHeight.
var rungs = []struct {
	height       int
	least, usual int64
}{
	{2160, 25_000_000, 40_000_000},
	{1440, 12_000_000, 20_000_000},
	{1080, 6_000_000, 10_000_000},
	{720, 3_000_000, 5_000_000},
	{540, 1_500_000, 3_000_000},
	{480, 900_000, 2_000_000},
	{360, 0, 1_000_000},
}

// softwareHeight caps video converted by the processor: software encoding
// keeps up with playback up to 1080p.
const softwareHeight = 1080

// sizeClass is the height of the smallest rung whose 16:9 frame holds the
// whole source (2160 for a 3832×1600 picture, 1080 for 1920×800), the top
// rung for anything larger, and 1080p when the size is unknown.
func sizeClass(video MediaStream) int {
	if video.Width == nil || video.Height == nil {
		return softwareHeight
	}
	class := rungs[0].height
	for _, r := range rungs {
		if *video.Width <= (r.height*16+8)/9 && *video.Height <= r.height {
			class = r.height
		}
	}
	return class
}

// toneMapsOnGPU reports whether gpu, the GPU encoding the video, tone maps
// it as the tuning allows. NVIDIA's tone maps any HDR, Dolby Vision with
// no base layer other players read (profile 5) included. VideoToolbox
// tone maps HDR10 and HLG, whose colors scale_vt converts, but not Dolby
// Vision profile 5. Intel's, through VAAPI or QSV, takes only PQ (HDR10)
// frames carrying their mastering display, without which FFmpeg's
// tonemap_vaapi fails and vpp_qsv does not tone map: HLG, Dolby Vision
// profile 5 and a video whose mastering display is not known go to the
// processor.
func toneMapsOnGPU(gpu *hls.Hardware, video MediaStream, t Tuning) bool {
	switch {
	case gpu == nil || !gpu.ToneMapping || t.CPUToneMapping:
		return false
	case gpu.Method == "cuda":
		return true
	case video.VideoRangeType == "DOVI":
		return false
	case gpu.Method == "videotoolbox":
		return video.ColorTransfer == "smpte2084" || video.ColorTransfer == "arib-std-b67"
	}
	return video.ColorTransfer == "smpte2084" && video.MasteringDisplay != nil && *video.MasteringDisplay
}

// appFrame is the size of video of width by height, 0 when not known,
// converted to a rung's height within the app's frame: with exact, at the
// app's cap itself, the source brought within it as Jellyfin does it, else
// framed at the rung (see frame) then brought within it.
func appFrame(app AppCaps, rung, width, height int, exact bool) (int, int) {
	if exact && width > 0 && height > 0 {
		return app.fit(width, height)
	}
	w, h := frame(rung, width, height)
	if app.framed() {
		return app.fit(w, h)
	}
	return w, h
}

// mostRate is the highest frame rate taken for a video's own: ffprobe
// gives 1/time base for a rate it does not find, 1000 in Matroska and
// 90000 in MPEG-TS, which no level holds.
const mostRate = 240

// unknownRate is the frame rate video whose own is not known is
// converted at, which holds the usual 23.976 to 30 without dropping a
// frame.
const unknownRate = 30

// knownRate is the frame rate a conversion takes video of average and
// real frame rates at, as ffprobe gives them: the average, else the real
// one, leaving out 0, NaN and rates above mostRate; else unknownRate.
func knownRate(average, real float64) float64 {
	for _, rate := range []float64{average, real} {
		if rate > 0 && rate <= mostRate {
			return rate
		}
	}
	return unknownRate
}

// sourceRate is the frame rate of video, as its conversion takes it (see
// knownRate).
func sourceRate(video MediaStream) float64 {
	var average, real float64
	if video.AverageFrameRate != nil {
		average = *video.AverageFrameRate
	}
	if video.RealFrameRate != nil {
		real = *video.RealFrameRate
	}
	return knownRate(average, real)
}

// encodedRate is the frame rate a conversion encodes video of rate at,
// tuned by t, and whether it doubles the frames: like Jellyfin, when
// deinterlacing makes a frame of each field, up to 30 frames a second
// and within the frame rate the app's profile allows, to which the rate
// is brought down, dropping the frames beyond it.
func (v *VideoConversion) encodedRate(rate float64, t Tuning) (float64, bool) {
	double := v.Deinterlace && t.DoubleRate && rate <= 30 && (v.FrameRate <= 0 || rate*2 <= v.FrameRate)
	if double {
		rate *= 2
	}
	if v.FrameRate > 0 && rate > v.FrameRate {
		rate = v.FrameRate
	}
	return rate, double
}

// fits reports whether the 16:9 frame of a height holds the whole source,
// which a cap of that height then leaves as it is: never when its size is
// unknown.
func fits(video MediaStream, height int) bool {
	return video.Width != nil && video.Height != nil && *video.Width > 0 && *video.Height > 0 &&
		*video.Width <= (height*16+8)/9 && *video.Height <= height
}

// ConvertVideo is the conversion of video for a transcoding profile taking
// codecs, as a comma-separated list, within limits: the first of H.264 and
// HEVC the profile takes and FFmpeg encodes, H.264 first unless the tuning
// prefers HEVC, which then follows the profile's order, on the GPU when it
// encodes that codec, at the height the bitrate limit allows, up to 4K on a
// GPU and 1080p on the processor, never larger than the source nor, when
// they are positive, than the user's quality group and the settings' cap,
// at about the source's bitrate (see videoBitrate) up to the limit's,
// converted to SDR and deinterlaced as needed. HDR is tone mapped on that
// GPU when it can (see toneMapsOnGPU) and keeps up (see hls.HDRPath),
// else on the processor, up to the capabilities' ToneMappedHeight, unless
// the tuning turns tone mapping off; a GPU keeping up only at a lower
// height tone maps up to it. It is nil when the profile takes neither
// codec, and for HDR that cannot be converted: on the processor, without
// FFmpeg's filters, or Dolby Vision with no base layer other players
// read, which only NVIDIA's GPUs tone map and which without tone mapping
// would show wrong colors.
func ConvertVideo(codecs string, limits Limits, video MediaStream, can Capabilities) *VideoConversion {
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
	// size is the lowest cap so far, the tallest the video may be.
	size := ConversionSize{Reason: SizeProcessor, MaxHeight: softwareHeight}
	if conversion.Hardware != nil {
		// A GPU converts up to 4K, at the rung of the source's own size: a
		// source no larger than 1080p converts as on the processor.
		size = ConversionSize{Reason: SizeGPU, MaxHeight: max(softwareHeight, sizeClass(video))}
	}
	switch {
	case video.VideoRange != "HDR":
	case can.Tuning.NoToneMapping:
		if video.VideoRangeType == "DOVI" {
			return nil
		}
	default:
		onGPU := toneMapsOnGPU(conversion.Hardware, video, can.Tuning)
		// The processor takes the HDR it tone maps from a GPU too slow for
		// it; the GPU keeps Dolby Vision profile 5, which only it tone maps.
		if onGPU && can.HDR.Processor && can.ToneMapping && video.VideoRangeType != "DOVI" {
			onGPU, conversion.ToneMapReason = false, hls.ToneMapSlowGPU
		}
		if !onGPU {
			if video.VideoRangeType == "DOVI" || !can.ToneMapping {
				return nil
			}
			// The processor's cap counts where it is the lowest so far: the
			// software encoder stops at softwareHeight anyway.
			if height := cmp.Or(can.ToneMappedHeight, hls.LowToneMappedHeight); height < size.MaxHeight {
				size = ConversionSize{Reason: SizeToneMapping, MaxHeight: height}
			}
			if conversion.ToneMapOnCPU = conversion.Hardware != nil && conversion.Hardware.ToneMapping; conversion.ToneMapOnCPU && conversion.ToneMapReason == "" {
				conversion.ToneMapReason = hls.ToneMapUnsupported
				if can.Tuning.CPUToneMapping {
					conversion.ToneMapReason = hls.ToneMapTurnedOff
				}
			}
		} else if height := can.HDR.Height; height > 0 && height < size.MaxHeight {
			size, conversion.ToneMapReason = ConversionSize{Reason: SizeGPUToneMapping, MaxHeight: height}, hls.ToneMapSlowGPU
		}
		conversion.ToneMap = true
	}
	width, height := 0, 0
	if video.Height != nil && video.Width != nil {
		width, height = *video.Width, *video.Height
	}
	// The app's frame caps the video at the height the source keeps once
	// brought within it, which need not be a rung's.
	if app := limits.App; app.framed() {
		capped := app.Height
		if width > 0 && height > 0 {
			if _, capped = app.fit(width, height); capped >= height {
				capped = 0
			}
		} else if capped == 0 {
			capped = app.Width * 9 / 16
		}
		if capped > 0 && capped <= size.MaxHeight {
			size = ConversionSize{Reason: SizeApp, MaxHeight: capped}
		}
	}
	// A source no taller than a cap converts as without it. The group comes
	// last, so that it wins a tie with the settings' cap, and both win a
	// tie with the app's and the server's own caps.
	for _, setting := range []ConversionSize{{Reason: SizeServerMax, MaxHeight: limits.Server}, {Reason: SizeQualityGroup, MaxHeight: limits.Group}} {
		if setting.MaxHeight > 0 && setting.MaxHeight <= size.MaxHeight && (video.Height == nil || *video.Height > setting.MaxHeight) {
			size = setting
		}
	}
	conversion.Deinterlace = video.IsInterlaced
	rung, first := rungs[len(rungs)-1], true
	for _, r := range rungs {
		if r.height > size.MaxHeight {
			continue
		}
		if limits.Video <= 0 || limits.Video >= r.least {
			rung = r
			break
		}
		first = false
	}
	// The app's cap, between two rungs or below the last, is the height
	// itself, at the bitrates of the rung below.
	if size.Reason == SizeApp && (first || rung.height > size.MaxHeight) {
		rung.height = size.MaxHeight
	}
	if rung.height < size.MaxHeight {
		size = ConversionSize{Reason: SizeBitrate, MaxHeight: rung.height, Bitrate: limits.Bitrate, ByUser: limits.ByUser}
	}
	if fits(video, size.MaxHeight) {
		size = ConversionSize{Reason: SizeSource}
	}
	conversion.Width, conversion.Height = appFrame(limits.App, rung.height, width, height, size.Reason == SizeApp && rung.height == size.MaxHeight)
	if profile := limits.App.h264Profile(); conversion.Codec == "h264" && profile != "high" {
		conversion.Profile = profile
	}
	rate := sourceRate(video)
	if most := limits.App.FrameRate; most > 0 && rate > most+0.01 {
		conversion.FrameRate = most
	}
	rate, _ = conversion.encodedRate(rate, can.Tuning)
	// A frame larger than the level the app decodes holds goes down the
	// ladder until it fits.
	if conversion.MaxLevel = limits.App.level(conversion.Codec); conversion.MaxLevel > 0 {
		for _, r := range rungs {
			if conversion.codecLevel(rate).tenths <= conversion.MaxLevel {
				break
			}
			if w, h := appFrame(limits.App, r.height, width, height, false); h < conversion.Height {
				rung, conversion.Width, conversion.Height = r, w, h
				size = ConversionSize{Reason: SizeApp, MaxHeight: r.height}
			}
		}
	}
	conversion.Size = size
	// A lower cap than the GPU's own leaves it nothing to say.
	if !conversion.ToneMapOnCPU && size.Reason != SizeGPUToneMapping {
		conversion.ToneMapReason = ""
	}
	conversion.Bitrate = videoBitrate(video, conversion.Codec, conversion.Width, conversion.Height, rung.usual)
	if limits.Video > 0 {
		conversion.Bitrate = min(conversion.Bitrate, limits.Video)
	}
	conversion.Bitrate = max(conversion.Bitrate, 300_000)
	// Under a limit, the video peaks within what the limit leaves it, so
	// that with the audio sent it stays within the limit: at its bitrate,
	// as in Jellyfin, where the limit sets the bitrate.
	if limits.Video > 0 && limits.Video < conversion.Bitrate*3/2 {
		conversion.Peak = max(limits.Video, conversion.Bitrate)
	}
	return conversion
}

// peak is the most a conversion's video reaches, in bits per second: its
// Peak under a limit, else one and a half times its bitrate.
func (v *VideoConversion) peak() int64 {
	if v.Peak > 0 {
		return v.Peak
	}
	return v.Bitrate * 3 / 2
}

// videoBitrate is the bitrate of video converted to codec at width by
// height, before any limit, as Jellyfin's encoder gives it: the source's,
// for the share of its picture a smaller conversion keeps, which Jellyfin
// does not take; up to two and a half times that below 3 Mb/s; more for
// 10-bit video, converted to 8 bits; and more for a codec needing more
// bits than the source's, H.264 taking 1/0.6 times HEVC's and VP9's, and
// twice AV1's, below 30 Mb/s. usual is the bitrate when the source's is
// not known.
func videoBitrate(video MediaStream, codec string, width, height int, usual int64) int64 {
	if video.BitRate == nil || *video.BitRate <= 0 {
		return usual
	}
	source := *video.BitRate
	bitrate := float64(source)
	if video.Width != nil && video.Height != nil && *video.Width > 0 && *video.Height > 0 {
		bitrate *= min(float64(width*height)/float64(*video.Width**video.Height), 1)
	}
	// Jellyfin's GetMinBitrate, "chosen from testing to improve low
	// bitrate streams".
	switch {
	case bitrate <= 2_000_000:
		bitrate = math.RoundToEven(bitrate * 2.5)
	case bitrate <= 3_000_000:
		bitrate *= 2
	}
	// GetBitDepthCompensatedBitrate, by the source's bitrate, for video
	// whose depth is not known too.
	if video.BitDepth == nil || *video.BitDepth > 8 {
		factor := 1.05
		switch {
		case source <= 4_000_000:
			factor = 1.5
		case source <= 8_000_000:
			factor = 1.2
		case source <= 12_000_000:
			factor = 1.1
		}
		bitrate = math.RoundToEven(bitrate * factor)
	}
	// ScaleBitrate: never less than the source's.
	factor := max(codecEfficiency(codec)/codecEfficiency(video.Codec), 1)
	switch {
	case bitrate <= 500_000:
		factor = max(factor, 4)
	case bitrate <= 1_000_000:
		factor = max(factor, 3)
	case bitrate <= 2_000_000:
		factor = max(factor, 2.5)
	case bitrate <= 3_000_000:
		factor = max(factor, 2)
	case bitrate >= 30_000_000:
		factor = 1
	}
	return int64(math.RoundToEven(bitrate * factor))
}

// codecEfficiency is the share of H.264's bitrate a codec needs for the
// same picture, as Jellyfin counts it: 0.6 for HEVC and VP9, 0.5 for AV1.
func codecEfficiency(codec string) float64 {
	switch strings.ToLower(codec) {
	case "hevc", "h265", "vp9":
		return 0.6
	case "av1":
		return 0.5
	}
	return 1
}

// frame is the size of video of width by height, 0 when not known,
// converted to a rung's height. The rung's frame is 16:9, as 1080p's
// 1920×1080, its width rounded up so that a 16:9 source fills it. A wider
// source fits its width, as in Jellyfin: a 2.40:1 picture becomes
// 1920×800 rather than 2586×1080, which no 1080p level holds and an
// NVIDIA GPU refuses to encode. Encoders take even sizes.
func frame(rung, width, height int) (int, int) {
	h, w := rung, (rung*16+8)/9
	if height > 0 && width > 0 {
		if width*h > w*height {
			w = min(w, width)
			h = height * w / width
		} else {
			h = min(h, height)
			w = width * h / height
		}
	}
	return w / 2 * 2, h / 2 * 2
}

// steps is how many lower qualities a conversion offers at most when
// apps may step down (see Steps).
const steps = 2

// Steps are the lower qualities a conversion of video of width by height,
// 0 when not known, offers apps to step down to, tallest first: up to two
// rungs of the ladder below its own, each the tallest whose usual bitrate
// is at most half that of the quality above, at that bitrate, down to
// 360p. 1080p at 26 Mb/s or at 10 Mb/s offers 720p at 5 Mb/s and 480p at
// 2 Mb/s. Each keeps the conversion's codec, encoder and tone mapping, at
// its rung's size, framed as a conversion to that rung would be; Size
// tells what set the first quality's.
func (v *VideoConversion) Steps(width, height int) []VideoConversion {
	// The rung the conversion is at: the smallest whose frame holds it.
	above := rungs[0].height
	for _, r := range rungs {
		if v.Width <= (r.height*16+8)/9 && v.Height <= r.height {
			above = r.height
		}
	}
	bitrate := v.Bitrate
	var lower []VideoConversion
	for _, r := range rungs {
		if len(lower) == steps {
			break
		}
		if r.height >= above || r.usual > bitrate/2 {
			continue
		}
		step := *v
		step.Width, step.Height = frame(r.height, width, height)
		// Half the quality above or less, it peaks at one and a half
		// times its bitrate, below the first's peak.
		step.Bitrate, step.Peak = r.usual, 0
		lower = append(lower, step)
		above, bitrate = r.height, r.usual
	}
	return lower
}

// levels are the codec levels a conversion may declare, lowest first, in
// tenths (41 is 4.1), with the largest frame and the most samples a second
// each holds, macroblocks for H.264 (ITU-T H.264, table A-1) and luma
// samples for HEVC (ITU-T H.265, tables A.8 and A.9), then the most bits a
// second and bits buffered of its Main tier, in thousands, which H.264's
// High profile takes 1.25 times (table A-2). HEVC is always declared at
// the Main tier: many TVs decode no other, and apps state a level, never
// a tier. Conversions are 4K at most, which 5.2 holds up to 60 frames a
// second and 6.1 above. 6.0 is left out: NVENC's H.264 calls it 6.0 and
// VAAPI 6, while every encoder Polyfin uses takes the other levels'
// names. Those below 4.1 serve only apps that decode no higher (see
// codecLevel).
var levels = map[string][]struct {
	tenths        int
	frame, second int64
	rate, buffer  int64
}{
	"h264": {
		{30, 1_620, 40_500, 10_000, 10_000}, {31, 3_600, 108_000, 14_000, 14_000}, {32, 5_120, 216_000, 20_000, 20_000},
		{40, 8_192, 245_760, 20_000, 25_000}, {41, 8_192, 245_760, 50_000, 62_500}, {42, 8_704, 522_240, 50_000, 62_500},
		{50, 22_080, 589_824, 135_000, 135_000}, {51, 36_864, 983_040, 240_000, 240_000}, {52, 36_864, 2_073_600, 240_000, 240_000},
		{61, 139_264, 8_355_840, 480_000, 480_000},
	},
	"hevc": {
		{30, 552_960, 16_588_800, 6_000, 6_000}, {31, 983_040, 33_177_600, 10_000, 10_000}, {40, 2_228_224, 66_846_720, 12_000, 12_000},
		{41, 2_228_224, 133_693_440, 20_000, 20_000}, {50, 8_912_896, 267_386_880, 25_000, 25_000},
		{51, 8_912_896, 534_773_760, 40_000, 40_000}, {52, 8_912_896, 1_069_547_520, 60_000, 60_000},
		{61, 35_651_584, 2_139_095_040, 120_000, 120_000},
	},
}

// codecLevel is the level a conversion declares at a frame rate, and
// what its encoder is told within it (see VideoConversion.codecLevel).
type codecLevel struct {
	// tenths is the level, 41 for 4.1.
	tenths int
	// bitrate, peak and buffer are the encoder's bitrate, maximum rate
	// and buffer, in bits.
	bitrate, peak, buffer int64
}

// name is the level as "4.1" or "5". internal/hls spells it as each
// encoder takes it.
func (l codecLevel) name() string {
	if l.tenths%10 == 0 {
		return strconv.Itoa(l.tenths / 10)
	}
	return strconv.Itoa(l.tenths/10) + "." + strconv.Itoa(l.tenths%10)
}

// codecLevel is the level a conversion declares for video of a frame
// rate, at the Main tier, and the bitrates its encoder is told, all of
// which the level holds: NVENC refuses to encode otherwise, as x265 does
// at a rate above the level's. It is the lowest level holding the frame,
// its samples a second and the peak (see peak), never below H.264 4.1
// (4.2 above 30 frames a second) or HEVC 4.1, unless the app decodes no
// higher (MaxLevel), and up to the app's level. The buffer, twice the
// bitrate, is brought within that level's, which holds at least a second
// of the peak. A peak no such level holds is brought within the highest
// one, the bitrate with it, as within 6.1 when no level holds the frame.
func (v *VideoConversion) codecLevel(rate float64) codecLevel {
	frame, factor, least := int64(v.Width)*int64(v.Height), int64(1000), 41
	if v.Codec == "h264" {
		frame = int64((v.Width+15)/16) * int64((v.Height+15)/16)
		if rate > 30 {
			least = 42
		}
		if v.Profile == "" {
			factor = 1250
		}
	}
	if v.MaxLevel > 0 && v.MaxLevel < least {
		least = 0
	}
	c := codecLevel{bitrate: v.Bitrate, peak: v.peak(), buffer: v.Bitrate * 2}
	table := levels[v.Codec]
	top, found := len(table)-1, false
	for i, l := range table {
		if l.tenths < least || frame > l.frame || float64(frame)*rate > float64(l.second) {
			continue
		}
		// Above the lowest level holding the frame and its samples, the
		// app's level is the highest.
		if found && v.MaxLevel > 0 && l.tenths > v.MaxLevel {
			break
		}
		top, found = i, true
		if c.peak <= l.rate*factor {
			break
		}
	}
	l := table[top]
	c.tenths = l.tenths
	c.peak, c.buffer = min(c.peak, l.rate*factor), min(c.buffer, l.buffer*factor)
	c.bitrate = min(c.bitrate, c.peak)
	return c
}

// codecString is the RFC 6381 codec string of a conversion's video at l:
// H.264 at its profile, High unless the app's allows only Main or
// Baseline, or HEVC Main, at the Main tier. The constraint flags are
// those x264 writes: Baseline as constrained (set 0 and 1), Main with set
// 1.
func (v *VideoConversion) codecString(l codecLevel) string {
	if v.Codec == "hevc" {
		// HEVC levels count in thirtieths: 4.1 is 123.
		return "hvc1.1.6.L" + strconv.Itoa(l.tenths*3) + ".B0"
	}
	level := strings.ToUpper(strconv.FormatInt(int64(l.tenths), 16))
	switch v.Profile {
	case "main":
		return "avc1.4D40" + level
	case "baseline":
		return "avc1.42C0" + level
	}
	return "avc1.6400" + level
}
