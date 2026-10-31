package hls

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
	"github.com/moodiness/polyfin/internal/process"
	"github.com/moodiness/polyfin/internal/subtitles"
)

const (
	// defaultAhead is how far past the end of the last segment asked for
	// FFmpeg makes segments before it waits, unless LimitAhead says
	// otherwise: a player buffers a minute or two ahead, and FFmpeg working
	// further keeps the source's connection busy.
	defaultAhead = 120 * time.Second
	// reach is how far past the segment being made a request waits for it,
	// rather than starting FFmpeg again from the segment asked for: about
	// 12 s of picture, which a remux makes in about the time a new run
	// takes to start, and room for players asking a few segments at once.
	reach = 3
	// behind is how many segments are kept before the last one asked for,
	// for a player that asks again for the segment it plays. Further back,
	// FFmpeg makes them again from the source cache: a remuxed 4K segment
	// weighs 10 to 30 MB.
	behind = 3
	// stallTimeout is how long a request waits for a job that makes no
	// progress, reading or writing, before it is answered that the server
	// is busy (ErrStalled): a player asks again, or moves on, rather than
	// showing a frozen picture for minutes.
	stallTimeout = 20 * time.Second
	// idleTimeout stops an encoding no player asked anything of for that
	// long, such as a paused one; it starts again on the next request.
	idleTimeout = 3 * time.Minute
	// variantLinger is how long the FFmpeg of a variant of a playback runs
	// on once the player asked for another variant (see playback): an app
	// measuring the qualities it switches between, as hls.js does, comes
	// back to the one it left within seconds.
	variantLinger = 10 * time.Second
	// tolerance absorbs the rounding between the keyframe index and the
	// timestamps in FFmpeg's output.
	tolerance = 2 * time.Millisecond
	// settle is how long after FFmpeg wrote a part of the video the
	// subtitle cues shown there count as extracted. FFmpeg converts them in
	// threads of its own, which may lag behind the video it copies, most
	// when they start; nothing tells a cue still on its way from no cue.
	settle = 500 * time.Millisecond
)

var (
	// ErrNotFound reports a segment the version does not have.
	ErrNotFound = errors.New("no such segment")
	// ErrStopped reports an encoding stopped while a player waited for it.
	ErrStopped = errors.New("the encoding was stopped")
	// ErrStalled reports a segment FFmpeg made no progress towards for
	// stallTimeout: its source is slow to answer, or stopped answering.
	ErrStalled = errors.New("the encoding made no progress")
	errStale   = errors.New("the job was replaced")
)

// Remux is what an encoding reads and produces.
type Remux struct {
	// Input is what FFmpeg reads the version from: its URL, or a disc's
	// path (see media.Input).
	Input string
	// InputOptions are FFmpeg's options for reading Input, such as those of
	// its HLS demuxer for a live Input, or of a disc's demuxer.
	InputOptions []string
	// Video and Audio are FFmpeg stream indexes; Audio is -1 for none.
	Video, Audio int
	// ADTS marks AAC audio framed as MPEG-TS carries it, live or in a file,
	// which MP4
	// stores otherwise when it is copied.
	ADTS bool
	// AudioCodec is the encoder the audio is converted with, empty to copy
	// it; AudioChannels and AudioBitrate, what it is converted to, the
	// bitrate zero for lossless codecs.
	AudioCodec    string
	AudioChannels int
	AudioBitrate  int64
	// AudioFilter is the filter graph converted audio goes through, such
	// as a downmix to stereo, empty for none.
	AudioFilter string
	// Threads is how many threads FFmpeg converts with, 0 letting it
	// choose.
	Threads int
	// VideoTag is the sample entry of the video in MP4, such as hvc1 for
	// HEVC, which Apple players require; empty keeps FFmpeg's.
	VideoTag string
	// Encode converts the video; nil copies it.
	Encode *VideoEncoding
	// Fast, when set, converts the video of the plan's fast segments (see
	// Plan.FastFrom), quickly and smaller, and FastTag is its sample entry
	// in MP4, as VideoTag is Encode's. Nil converts them as Encode does,
	// which a plan with fast segments then needs: copied video starts its
	// segments on the version's keyframes only.
	Fast    *VideoEncoding
	FastTag string
	Format  Format
	Plan    Plan
	// Subtitles are the FFmpeg indexes of the text subtitle streams
	// extracted into Extracted while remuxing, from the bytes FFmpeg reads
	// anyway.
	Subtitles []int
	Extracted Extracted
	// Failed, when set, is closed once the input failed for good: its host
	// stopped answering, refused it, or answers another file. Failure then
	// tells why. The encoding ends at once, nothing it cut short is served,
	// and the requests waiting for it get Failure's error. Nil never fails.
	Failed  <-chan struct{}
	Failure func() error
	// Refused, when set, tells why the source of a live Input refused it
	// lately, as when its connections or tuners are all in use; nil for no
	// refusal. A live run that failed while it was refused failed for that
	// reason, which is no fault of the encoding.
	Refused func() error
	// Channel names the channel a live Input plays, in the log lines of
	// its encoding. Answered, when set, tells the last file of a live
	// Input that answered no media since a time, an error or a page, by
	// its file name; nil for none. A live run that fails or stalls says
	// what was answered since it last made a segment (see live.start).
	Channel  string
	Answered func(since time.Time) error
	// Lookback is how far before a segment's start FFmpeg starts reading
	// a searched version from the encoding's first seek on, as an earlier
	// encoding of the same file learned it (see encoding.lookback); zero
	// for none. Learned, when set, is told of each longer distance this
	// encoding learns, on a goroutine of its own, so as to keep it.
	Lookback time.Duration
	Learned  func(time.Duration)
	// Switch, when set, carries the encoding on from another input once
	// this one failed for good (see Failed), from segment n, the first a
	// player still waits for: it returns that input with the same Plan
	// and Format, and what releases it, within ctx's time. Its segments
	// follow the plan's times, Shift telling where they are in the input.
	// An error ends the encoding, as without it.
	Switch func(ctx context.Context, n int) (Remux, func(), error)
	// Shift is how far the input's times are past the plan's: zero but for
	// an input switched to, whose speech comes that much later than the
	// first one's.
	Shift time.Duration
	// AudioRate is the sample rate converted audio is brought to, 0 to
	// keep the input's: that of the audio the player had before a switch.
	AudioRate int
}

// VideoEncoding is what a job converts the video to: 8-bit, progressive,
// with a keyframe at the start of every segment, so that the segments of
// a conversion follow those of a remux of the same version. The fields
// after Hardware come from the server's settings; their zero values are
// what Polyfin did before they were settings.
type VideoEncoding struct {
	// Encoder is libx264, libx265 or one of Hardware's; Level, the codec
	// level it declares.
	Encoder, Level string
	Width, Height  int
	// Bitrate is the average the encoder aims for, in bits per second, and
	// MaxRate the most it lets the video peak at, 0 for one and a half
	// times Bitrate.
	Bitrate, MaxRate int64
	// FrameRate is the converted video's, frames a second: the source's,
	// twice it with DoubleRate, or less with DropFrames, which brings the
	// video down to it, as an app's profile asks.
	FrameRate  float64
	DropFrames bool
	// Profile is the H.264 profile written, high, main or baseline; empty
	// writes High, and HEVC Main whatever it says.
	Profile string
	// ToneMap converts HDR to SDR; Deinterlace, interlaced video to
	// progressive.
	ToneMap, Deinterlace bool
	// ToneMapOnCPU tone maps HDR on the processor even on a GPU that tone
	// maps.
	ToneMapOnCPU bool
	// Burn is the FFmpeg index of an image subtitle stream burned into the
	// video, nil for none; BurnFile, the address of an image subtitle file
	// burned in instead, a PGS file or a VobSub pair's .idx file, read
	// beside the source (see burnGraph).
	Burn     *int
	BurnFile string
	// Colors are the source's, as its analysis states them, which video
	// converted without tone mapping keeps (see tags).
	Colors Colors
	// Hardware is the GPU decoding and encoding the video, nil for none.
	Hardware *Hardware
	// Preset trades the encoder's speed for quality: one of x264's preset
	// names, from veryslow to ultrafast, which the other encoders' follow
	// (see nvencPresets, vaapiCompression, qsvPresets, amfQualities and
	// videoToolboxSpeed). Empty keeps Polyfin's: veryfast in software, p4
	// on NVIDIA GPUs, the encoder's own on the others.
	Preset string
	// Quality is the encoder's quality factor, Bitrate then being a cap:
	// CRF in software, CQ on NVIDIA GPUs, QVBR's on VAAPI drivers that
	// have it (see Hardware.QVBR), and VideoToolbox's (see
	// videoToolboxQuality); QSV and AMF aim for Bitrate. 0 aims for
	// Bitrate.
	Quality int
	// ToneMapCurve is the curve HDR is tone mapped with: bt2390, hable,
	// reinhard, mobius, clip or linear. Empty is BT.2390 on the GPU and
	// Hable on the processor, whose filter has no BT.2390. ToneMapPeak, in
	// nits, overrides the video's peak, 0 keeping it, and ToneMapDesat
	// desaturates highlights; libplacebo, on the GPU, takes neither.
	ToneMapCurve string
	ToneMapPeak  int
	ToneMapDesat float64
	// Deinterlacer is yadif, if empty, or bwdif; DoubleRate makes a frame
	// of each field rather than of each frame.
	Deinterlacer string
	DoubleRate   bool
	// DecodeOnCPU leaves decoding to FFmpeg's own decoder when converting
	// on a GPU.
	DecodeOnCPU bool
}

// Colors are the color range, matrix, transfer and primaries of a video,
// as FFmpeg names them, each empty when not known.
type Colors struct {
	Range, Matrix, Transfer, Primaries string
}

// taggedColors are the colors tags writes, by setparams option, with the
// names FFmpeg gives them: those of the videos apps play, which scaling
// in YUV keeps. An RGB matrix is left out, as the frames go out in YUV.
var taggedColors = []struct {
	option string
	names  []string
}{
	{"range", []string{"tv", "pc"}},
	{"colorspace", []string{"bt709", "fcc", "bt470bg", "smpte170m", "smpte240m", "bt2020nc", "bt2020c"}},
	{"color_trc", []string{"bt709", "bt470m", "bt470bg", "smpte170m", "smpte240m", "linear", "iec61966-2-1", "bt2020-10", "bt2020-12", "smpte2084", "arib-std-b67"}},
	{"color_primaries", []string{"bt709", "bt470m", "bt470bg", "smpte170m", "smpte240m", "film", "bt2020"}},
}

// tags is the filter tagging the converted frames with the source's
// colors, which the encoders write into the bitstream, empty when none is
// known, and when tone mapping, whose filters tag the frames BT.709. The
// encoders take the colors the frames carry, and FFmpeg's HEVC decoder
// gives frames only those of the bitstream: without it, an HEVC source
// stating its colors in its container alone goes out without any. A color
// not known is left as the frames have it.
func (v *VideoEncoding) tags() string {
	if v.ToneMap {
		return ""
	}
	var options []string
	for i, value := range []string{v.Colors.Range, v.Colors.Matrix, v.Colors.Transfer, v.Colors.Primaries} {
		if slices.Contains(taggedColors[i].names, value) {
			options = append(options, taggedColors[i].option+"="+value)
		}
	}
	if len(options) == 0 {
		return ""
	}
	return ",setparams=" + strings.Join(options, ":")
}

// filters is the filter chain of the video: 8-bit, at the size asked, in
// SDR, as the encoder takes it, on the GPU when its frames stay there,
// tagged with its colors.
func (v *VideoEncoding) filters() string {
	switch v.pipeline() {
	case onGPU:
		return v.gpuFilters() + v.tags()
	case throughDevice:
		return v.deviceToneMapping() + v.tags()
	}
	return v.convert() + "," + v.Hardware.output() + v.tags()
}

// convert is the filter chain bringing the video to the size asked, in
// SDR, before the pixel format the encoder takes, in memory. On NVIDIA's
// GPU, libplacebo scales and tone maps in one pass, applying the Dolby
// Vision metadata FFmpeg's decoder exports, and with the BT.2390 curve,
// which keeps midtones brighter than the processor's Hable. The tone
// mapping of Intel's GPUs and of VideoToolbox keeps the frames on the GPU
// (see deviceToneMapping).
func (v *VideoEncoding) convert() string {
	var filters []string
	if v.Deinterlace {
		filters = append(filters, v.deinterlacer())
	}
	size := "w=" + strconv.Itoa(v.Width) + ":h=" + strconv.Itoa(v.Height)
	switch {
	case v.toneMapsOnGPU():
		filters = append(filters, "libplacebo="+size+":format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping="+v.gpuCurve())
	case v.ToneMap:
		// To linear light in floating point, to BT.709 primaries, tone
		// mapped, then to the BT.709 transfer and matrix in limited range.
		tonemap := "tonemap=tonemap=" + v.cpuCurve() + ":desat=" + strconv.FormatFloat(v.ToneMapDesat, 'f', -1, 64)
		if v.ToneMapPeak > 0 {
			tonemap += ":peak=" + strconv.Itoa(v.ToneMapPeak)
		}
		filters = append(filters, "scale="+size, "zscale=t=linear:npl=100", "format=gbrpf32le", "zscale=p=bt709",
			tonemap, "zscale=t=bt709:m=bt709:r=tv")
	default:
		filters = append(filters, "scale="+size)
	}
	return strings.Join(filters, ",")
}

// deinterlacer is the deinterlacing filter. yadif makes a frame of each
// frame by default, bwdif one of each field.
func (v *VideoEncoding) deinterlacer() string {
	name := cmp.Or(v.Deinterlacer, "yadif")
	switch {
	case v.DoubleRate:
		return name + "=1"
	case name == "yadif":
		return name
	}
	return name + "=0"
}

// gpuCurve is the tone mapping curve as libplacebo names it.
func (v *VideoEncoding) gpuCurve() string {
	if v.ToneMapCurve == "" || v.ToneMapCurve == "bt2390" {
		return "bt.2390"
	}
	return v.ToneMapCurve
}

// cpuCurve is the tone mapping curve as FFmpeg's tonemap filter names it:
// it has no BT.2390, and uses Hable instead.
func (v *VideoEncoding) cpuCurve() string {
	if v.ToneMapCurve == "" || v.ToneMapCurve == "bt2390" {
		return "hable"
	}
	return v.ToneMapCurve
}

// Burns reports whether subtitles are burned into the video.
func (v *VideoEncoding) Burns() bool {
	return v.Burn != nil || v.BurnFile != ""
}

// burnLead is how long before a job's start the pictures of a subtitle
// file burned in are kept, so that one still shown at its start is.
const burnLead = 10 * time.Second

// burnGraph is the filter graph burning the subtitles into video stream
// video, as output [video], for a job starting at from, its frames going
// through skip first (see VideoEncoding.skip). The subtitle's canvas, the
// size of the video it was made for, is scaled to the width of the
// converted video keeping its shape, and laid at the bottom: a
// video cropped since keeps the subtitles near its bottom edge. It is laid
// after the conversion to SDR, which would dim its colors, and in memory,
// before a GPU encoder takes the frames: the frames Intel's GPUs and
// VideoToolbox tone mapped come down to memory for it.
//
// FFmpeg repeats the canvas of a stream of the source for every packet
// read from the source, which is over a thousand a second with TrueHD
// audio: the canvas is first brought to the video's frame rate, or scaling
// the repeats makes the conversion several times slower. A subtitle file,
// the second input, repeats none, but is read from its start, as FFmpeg
// does not seek in PGS files reliably: the pictures shown before the job's
// start but those of its last burnLead are left out before scaling.
func (v *VideoEncoding) burnGraph(video int, from time.Duration, skip string) string {
	converted := v.convert() + ",format=yuv420p"
	if v.pipeline() == throughDevice {
		converted = v.deviceToneMapping() + ",hwdownload,format=nv12"
	}
	var subtitle string
	switch {
	case v.BurnFile == "":
		subtitle = "[0:" + strconv.Itoa(*v.Burn) + "]fps=" + strconv.FormatFloat(v.FrameRate, 'f', -1, 64) + ","
	case from > 0:
		subtitle = "[1:0]select=gte(t\\," + strconv.FormatFloat((from-burnLead).Seconds(), 'f', 3, 64) + "),"
	default:
		subtitle = "[1:0]"
	}
	return "[0:" + strconv.Itoa(video) + "]" + skip + converted + "[converted];" +
		subtitle + "scale=" + strconv.Itoa(v.Width) + ":-2[subtitle];" +
		"[converted][subtitle]overlay=x=0:y=main_h-overlay_h:eof_action=pass," + v.Hardware.output() + v.tags() + "[video]"
}

// args are FFmpeg's encoder options for a job starting at segment n.
func (v *VideoEncoding) args(plan Plan, n int) []string {
	return v.shiftedArgs(plan, n, 0)
}

// shiftedArgs are args for an input whose times are shift past the
// plan's (see Remux.Shift): its keyframes are asked at the plan's times
// moved so, which FFmpeg compares with the input's own.
func (v *VideoEncoding) shiftedArgs(plan Plan, n int, shift time.Duration) []string {
	lead := v.keyframeLead().Seconds()
	times := make([]string, 0, plan.Len()-n)
	for k := n; k < plan.Len(); k++ {
		times = append(times, strconv.FormatFloat(max((plan.Start(k)+shift).Seconds()-lead, 0), 'f', 4, 64))
	}
	return v.encoderArgs(strings.Join(times, ","), v.skip(plan, n))
}

// skip is the filter leaving out, before they are converted, the frames a
// job starting at segment n of a searched version decodes before the
// frame ahead of the segment's keyframe, but the first, empty for a job of
// another version. FFmpeg lands on a keyframe before the segment (see
// Plan.seekTime): the frames up to the segment would be converted only for
// take to leave them out. The first, where FFmpeg landed, tells take
// whether it landed past the segment's start (see
// encoding.searchFurther). The frame ahead is kept as the encoder rounds
// times to its frame rate after the filters: the frame its keyframe is
// asked of may come just before the time asked, as the source's frames
// fall.
func (v *VideoEncoding) skip(plan Plan, n int) string {
	if n == 0 || !plan.searched {
		return ""
	}
	from := plan.Start(n) - v.keyframeLead() - v.frameDuration()
	return "select=eq(n\\,0)+gte(t\\," + strconv.FormatFloat(max(from.Seconds(), 0), 'f', 4, 64) + "),"
}

// frameDuration is how long a frame of the converted video lasts, at 24
// frames a second when its frame rate is not known.
func (v *VideoEncoding) frameDuration() time.Duration {
	rate := v.FrameRate
	if rate <= 0 {
		rate = 24
	}
	return time.Duration(float64(time.Second) / rate)
}

// keyframeLead is how early each keyframe is asked of the encoder: it
// compares times in its frame rate's time base, where a keyframe's exact
// time can fall just before the time asked, so each is asked a fraction
// of a frame early. The encoder makes it on the first frame at or after
// that time: one a little before the segment's start when the source's
// frames fall so.
func (v *VideoEncoding) keyframeLead() time.Duration {
	return time.Duration(0.4 * float64(v.frameDuration()))
}

// cutTolerance is how far before a segment's start a keyframe of FFmpeg's
// output may be and still start it: the rounding between the keyframe
// index and the output's timestamps, and for converted video, the lead its
// keyframes are asked with (see keyframeLead), and the tenth of a
// millisecond its times are written to.
func (r Remux) cutTolerance() time.Duration {
	if r.Encode == nil {
		return tolerance
	}
	return tolerance + r.Encode.keyframeLead()
}

// nvencPresets are NVENC's presets for x264's names, as Jellyfin maps
// them.
var nvencPresets = map[string]string{"veryslow": "p7", "slower": "p6", "slow": "p5", "medium": "p4", "fast": "p3", "faster": "p2",
	"veryfast": "p1", "superfast": "p1", "ultrafast": "p1"}

// vaapiCompression are VAAPI's compression levels for x264's preset
// names, as Jellyfin maps them: 1 is the slowest. Jellyfin sets them on
// Intel drivers only, AMD's following them unreliably; Polyfin does not
// tell drivers apart, and sets them on any.
var vaapiCompression = map[string]string{"veryslow": "1", "slower": "2", "slow": "3", "medium": "4", "fast": "5", "faster": "6",
	"veryfast": "7", "superfast": "7", "ultrafast": "7"}

// qsvPresets are QSV's presets for x264's names: QSV's go from veryslow
// to veryfast.
var qsvPresets = map[string]string{"veryslow": "veryslow", "slower": "slower", "slow": "slow", "medium": "medium", "fast": "fast",
	"faster": "faster", "veryfast": "veryfast", "superfast": "veryfast", "ultrafast": "veryfast"}

// amfQualities are AMF's quality presets for x264's names.
var amfQualities = map[string]string{"veryslow": "quality", "slower": "quality", "slow": "quality", "medium": "balanced", "fast": "balanced",
	"faster": "speed", "veryfast": "speed", "superfast": "speed", "ultrafast": "speed"}

// videoToolboxSpeed tells VideoToolbox whether to put speed before quality
// for x264's names: medium and auto leave it to VideoToolbox.
var videoToolboxSpeed = map[string]string{"veryslow": "0", "slower": "0", "slow": "0", "fast": "1", "faster": "1", "veryfast": "1",
	"superfast": "1", "ultrafast": "1"}

// The families of FFmpeg's encoders Polyfin converts video with, each
// taking its own options.
const (
	familySoftware     = "software"
	familyNVENC        = "nvenc"
	familyVAAPI        = "vaapi"
	familyVideoToolbox = "videotoolbox"
	familyQSV          = "qsv"
	familyAMF          = "amf"
)

// encoderFamily is the family of an encoder, by its name: libx264 and
// libx265, or a GPU's, such as h264_qsv.
func encoderFamily(encoder string) string {
	switch {
	case encoder == "libx264" || encoder == "libx265":
		return familySoftware
	case strings.HasSuffix(encoder, "_nvenc"):
		return familyNVENC
	case strings.HasSuffix(encoder, "_videotoolbox"):
		return familyVideoToolbox
	case strings.HasSuffix(encoder, "_qsv"):
		return familyQSV
	case strings.HasSuffix(encoder, "_amf"):
		return familyAMF
	}
	return familyVAAPI
}

// videoToolboxQuality is VideoToolbox's quality, from 1 to 100, the best,
// for a quality factor on x264's scale, where 0 is the best and 51 the
// worst: 28 is 44.
func videoToolboxQuality(factor int) int {
	return min(max(100-2*factor, 1), 100)
}

// amfLevel is a codec level as AMF's encoders name it: "5.0" for "5".
func amfLevel(level string) string {
	if level != "" && !strings.Contains(level, ".") {
		return level + ".0"
	}
	return level
}

// encoderArgs are FFmpeg's encoder options, with keyframes forced as
// -force_key_frames takes them, and the frames going through skip before
// the video's filters (see skip). A quality factor is taken in software,
// by NVENC, by VAAPI drivers with QVBR and by VideoToolbox, within the
// maximum rate; QSV's and AMF's encoders aim for the bitrate, as their
// quality modes leave the maximum rate out.
func (v *VideoEncoding) encoderArgs(keyframes, skip string) []string {
	family := encoderFamily(v.Encoder)
	software, nvenc := family == familySoftware, family == familyNVENC
	qvbr := v.Quality > 0 && family == familyVAAPI && v.Hardware != nil && v.Hardware.QVBR
	quality := v.Quality > 0 && (software || nvenc || family == familyVideoToolbox)
	args := []string{"-c:v", v.Encoder}
	switch {
	case quality && software:
		// The quality factor rules, within the maximum rate.
	case quality && nvenc:
		args = append(args, "-b:v", "0")
	case quality:
		// VideoToolbox's quality replaces its average bitrate.
	default:
		args = append(args, "-b:v", strconv.FormatInt(v.Bitrate, 10))
	}
	maxRate := v.MaxRate
	if maxRate <= 0 {
		maxRate = v.Bitrate * 3 / 2
	}
	args = append(args, "-maxrate", strconv.FormatInt(maxRate, 10), "-bufsize", strconv.FormatInt(v.Bitrate*2, 10),
		"-force_key_frames", keyframes)
	if !v.Burns() {
		args = append(args, "-vf", skip+v.filters())
	}
	// Every H.264 encoder, x264 as the GPUs', encodes the profile the
	// master playlist announces, High unless an app's profile allows only
	// Main or Baseline; HEVC, Main. Baseline is constrained, under the
	// name each encoder gives it, as in Jellyfin's EncodingHelper.
	h264 := v.Encoder == "libx264" || strings.HasPrefix(v.Encoder, "h264")
	profile := "main"
	if h264 {
		profile = cmp.Or(v.Profile, "high")
		if profile == "baseline" && (family == familyVAAPI || family == familyAMF || family == familyVideoToolbox) {
			profile = "constrained_baseline"
		}
	}
	if v.DropFrames && v.FrameRate > 0 {
		args = append(args, "-r", strconv.FormatFloat(v.FrameRate, 'f', -1, 64))
	}
	switch {
	case v.Encoder == "libx264":
		args = append(args, "-preset", cmp.Or(v.Preset, "veryfast"), "-sc_threshold", "0", "-profile:v", profile, "-level:v", v.Level)
	case v.Encoder == "libx265":
		args = append(args, "-preset", cmp.Or(v.Preset, "veryfast"), "-sc_threshold", "0", "-profile:v", profile, "-x265-params", "log-level=error:level-idc="+v.Level)
	case nvenc:
		// A forced keyframe is an IDR frame, as a segment's first must be.
		args = append(args, "-preset", cmp.Or(nvencPresets[v.Preset], "p4"), "-rc", "vbr", "-forced-idr", "1", "-profile:v", profile, "-level:v", v.Level)
	case family == familyVideoToolbox:
		// VideoToolbox makes a forced keyframe an IDR frame. Its H.264
		// encoder names levels up to 5.2, and picks one itself above;
		// its HEVC encoder takes none.
		args = append(args, "-profile:v", profile)
		if h264 && v.Level <= "5.2" {
			args = append(args, "-level:v", v.Level)
		}
		if speed := videoToolboxSpeed[v.Preset]; speed != "" {
			args = append(args, "-prio_speed", speed)
		}
	case family == familyQSV:
		// QSV takes levels as numbers of its own, and picks one itself.
		args = append(args, "-forced_idr", "1", "-profile:v", profile)
		if preset := qsvPresets[v.Preset]; preset != "" {
			args = append(args, "-preset", preset)
		}
	case family == familyAMF:
		args = append(args, "-rc", "vbr_peak", "-forced_idr", "1", "-profile:v", profile, "-level:v", amfLevel(v.Level))
		if preset := amfQualities[v.Preset]; preset != "" {
			args = append(args, "-quality", preset)
		}
	case qvbr:
		args = append(args, "-rc_mode", "QVBR", "-profile:v", profile, "-level:v", v.Level)
	default:
		args = append(args, "-rc_mode", "VBR", "-profile:v", profile, "-level:v", v.Level)
	}
	switch {
	case quality && software:
		args = append(args, "-crf", strconv.Itoa(v.Quality))
	case quality && nvenc:
		args = append(args, "-cq", strconv.Itoa(v.Quality))
	case quality:
		args = append(args, "-global_quality", strconv.Itoa(videoToolboxQuality(v.Quality)))
	case qvbr:
		args = append(args, "-global_quality", strconv.Itoa(v.Quality))
	}
	if level := vaapiCompression[v.Preset]; level != "" && family == familyVAAPI {
		args = append(args, "-compression_level", level)
	}
	return args
}

// Opener prepares an encoding: what it remuxes, and a function releasing
// its input once the encoding stops.
type Opener func(ctx context.Context) (Remux, func(), error)

// Key identifies an encoding: what a play session plays.
type Key struct {
	Session string
	Audio   int
	Format  Format
	// User is who plays, which bounds the live encodings each user runs.
	User string
	// Device is the app's device, which with the user, the version, the
	// format, Video and Variant tells apart a playback's converted video
	// when it is kept (see KeepVideo).
	Device string
	// Version is the version played, and Converts marks an encoding that
	// converts the video: the playbacks doing so may be bounded, a playback
	// being what a user plays of a version (see LimitConversions).
	Version  string
	Converts bool
	// Video describes what the video is converted to: codec, size,
	// bitrate, tone mapping and burned subtitle, empty when it is copied or
	// not told. Only video it describes is kept (see KeepVideo).
	Video string
	// Variant is the quality of converted video a master playlist offers:
	// 0 for the first, then each lower one. Each variant of a playback
	// runs its own FFmpeg (see playback).
	Variant int
	// FastStart marks the encoding of a play that starts with fast
	// segments (see Plan.FastFrom): its video is never kept, the plan of
	// a session taking it over having none, or others.
	FastStart bool
}

func (k Key) name() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{k.Session, strconv.Itoa(k.Audio), k.Format.Extension(), strconv.Itoa(k.Variant),
		k.User, k.Device, k.Version, k.Video}, "\x00")))
	return hex.EncodeToString(sum[:12])
}

// playback is the key the variants of a playback share.
func (k Key) playback() Key {
	k.Variant = 0
	return k
}

// playback is what the encodings of the variants of a playback share.
// Each variant runs its own FFmpeg, started once its segments are asked
// for, and a request for one never stops another's. The variant asked for
// last plays: its FFmpeg works ahead and runs until the playback is idle,
// as with a single variant. Another variant's FFmpeg stops once it was
// not asked for in variantLinger (see encoding.lingered), so an app
// switching back and forth finds the qualities it measures still running,
// and the extra cost lasts only that long. The segments a variant made
// stay until the playback ends. All the variants count as one playback
// toward the conversions at once (see admits).
//
// Locks are taken in the order Manager.mu, encoding.mu, playback.mu, each
// optional: playback.mu is taken last, and nothing else is taken while it
// is held.
type playback struct {
	mu sync.Mutex
	// last is the encoding asked for last, and requested the last segment
	// asked of any variant, where a variant opened then starts; used is
	// when a player last asked for anything.
	last      *encoding
	requested int
	used      time.Time
	// video is the segment the variant asked for last began last, and
	// track the audio of a kept video, told of it (see track.advance).
	video int
	track *track
}

// began records that e began segment n, which the audio of a kept video
// may then make when e is the variant asked for last.
func (p *playback) began(e *encoding, n int) {
	p.mu.Lock()
	if p.last != nil && p.last != e {
		p.mu.Unlock()
		return
	}
	p.video = n
	t := p.track
	p.mu.Unlock()
	if t != nil {
		t.advance(n)
	}
}

// follow has t told of the segments the video begins, from the last one,
// and of the last segment a player asked for, where its first job starts
// when the initialization segment is asked for first.
func (p *playback) follow(t *track) {
	p.mu.Lock()
	p.track = t
	n, requested := p.video, p.requested
	p.mu.Unlock()
	if t != nil {
		t.follow(n, requested)
	}
}

// asked records a request for e, for segment n when it is not negative,
// and returns the variant e takes over from, nil when e was asked for last
// already. The caller holds e.mu.
func (p *playback) asked(e *encoding, n int, at time.Time) *encoding {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.used = at
	if n >= 0 {
		p.requested = n
	}
	left := p.last
	p.last = e
	if left == e {
		return nil
	}
	return left
}

// playing reports whether e is the variant of its playback asked for last.
func (p *playback) playing(e *encoding) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last == e
}

// Manager runs the encodings players ask for.
type Manager struct {
	ffmpeg string
	dir    string
	logger *slog.Logger
	done   chan struct{}
	can    capabilities
	// hardware is the GPU DetectHardware chose last, nil for none; detected
	// holds what it found for each choice, and detecting serializes it.
	// failed holds why the GPU methods tried for each choice failed, and
	// failures those of the choice made last.
	hardware  atomic.Pointer[Hardware]
	detecting sync.Mutex
	detected  map[string]*Hardware
	failed    map[string][]gpuFailure
	failures  atomic.Pointer[[]gpuFailure]
	// wanted and wantedDevice are the choice detected last, which the
	// download of the FFmpeg for older NVIDIA drivers detects again; older
	// is that FFmpeg, nil for none, and olderMu guards whether it is being
	// downloaded and why its last download failed (see
	// fetchOlderDriverFFmpeg).
	wanted, wantedDevice string
	older                OlderDriverFFmpeg
	olderMu              sync.Mutex
	fetching             bool
	fetchErr             error
	// measuring counts the timings under way (see measureLater and
	// timeToneMappingLater), and timingGPU those of the GPU's chains; run,
	// when set, runs FFmpeg in their place, for tests.
	measuring sync.WaitGroup
	timingGPU sync.WaitGroup
	run       func(ctx context.Context, args []string) (time.Duration, error)
	// toneMappingTimed times HDR tone mapped on the processor once, and
	// toneMappedHeight keeps the height it chose, 0 until then (see
	// ToneMappedHeight).
	toneMappingTimed sync.Once
	toneMappedHeight atomic.Int32
	// vendor, when set, tells the PCI vendor of a render node in place of
	// Linux, for tests.
	vendor func(node string) string
	// system is the GOOS whose GPU methods detection tries (see methods):
	// runtime.GOOS; arch, its GOARCH.
	system, arch string
	// libraryDirs are the folders detection looks for libva in on arm64
	// (see vaapiOffered); nil, those the dynamic linker reads.
	libraryDirs []string
	// conversions returns how many playbacks may have their video
	// converted at once, 0 or less for no limit; nil sets no limit.
	conversions func() int
	// ahead returns how far past the end of the last segment asked for
	// FFmpeg makes segments before it waits; nil, or a value not above
	// zero, keeps defaultAhead.
	ahead atomic.Pointer[func() time.Duration]
	// stall is stallTimeout, and liveStall liveStall, which tests shorten.
	stall, liveStall time.Duration
	// grace is recordGrace, which tests lengthen.
	grace time.Duration
	// linger is variantLinger, in nanoseconds, which tests change.
	linger atomic.Int64
	// keepVideo returns whether converted video is kept apart from its
	// audio (see KeepVideo); keepFor is keepFor, in nanoseconds, which
	// tests change.
	keepVideo atomic.Pointer[func() bool]
	keepFor   atomic.Int64

	mu        sync.Mutex
	encodings map[Key]*encoding
	lives     map[Key]*live
	// audios are the conversions of tracks into segments (see audio.go).
	audios map[AudioKey]*audioEncoding
	// kept are the playbacks whose video is kept, by the key of their
	// first variant (see KeepVideo).
	kept map[Key]*keeping
	// progressives are the conversions streamed as one file (see
	// Progressive).
	progressives map[*progressive]struct{}
	// files counts the file conversions running (see ConvertFile), and
	// slotWait is slotWait, which tests shorten.
	files    int
	slotWait time.Duration
	// recordings are the recordings running, by key, with the function
	// stopping each.
	recordings map[Key]context.CancelFunc
	closed     bool
}

// NewManager returns a manager running FFmpeg from ffmpegPath and keeping
// segments under dir, which it empties first: segments are not reused
// across runs.
func NewManager(ffmpegPath, dir string, logger *slog.Logger) (*Manager, error) {
	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{ffmpeg: ffmpegPath, dir: dir, logger: logger, done: make(chan struct{}), can: probe(ffmpegPath), system: runtime.GOOS, arch: runtime.GOARCH,
		stall: stallTimeout, liveStall: liveStall,
		grace:    recordGrace,
		detected: map[string]*Hardware{}, encodings: map[Key]*encoding{}, lives: map[Key]*live{}, recordings: map[Key]context.CancelFunc{},
		kept: map[Key]*keeping{}}
	m.linger.Store(int64(variantLinger))
	m.keepFor.Store(int64(keepFor))
	go m.stopIdle()
	return m, nil
}

// Close stops every encoding.
func (m *Manager) Close() {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	close(m.done)
	for _, cancel := range m.recordings {
		cancel()
	}
	m.stopProgressives(func(Key) bool { return true })
	m.mu.Unlock()
	m.measuring.Wait()
	m.stopWhere(func(Key, *encoding) bool { return true })
	m.stopLives(func(Key, *live) bool { return true })
	m.stopAudios(func(AudioKey, *audioEncoding) bool { return true })
}

// Stop stops the encodings of a play session. Its kept video is kept for
// another session to take over (see KeepVideo).
func (m *Manager) Stop(session string) {
	m.mu.Lock()
	m.stopProgressives(func(key Key) bool { return key.Session == session })
	m.mu.Unlock()
	m.stopWhere(func(key Key, _ *encoding) bool { return key.Session == session })
	m.stopLives(func(key Key, _ *live) bool { return key.Session == session })
	m.stopAudios(func(key AudioKey, _ *audioEncoding) bool { return key.Session == session })
	m.release(session)
}

func (m *Manager) stopWhere(match func(Key, *encoding) bool) {
	m.mu.Lock()
	var stopping []*encoding
	for key, e := range m.encodings {
		if match(key, e) {
			stopping = append(stopping, e)
			delete(m.encodings, key)
		}
	}
	m.mu.Unlock()
	for _, e := range stopping {
		e.stop()
	}
	m.sweepKept()
}

func (m *Manager) stopIdle() {
	ticker := time.NewTicker(idleTimeout / 6)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.stopWhere(func(_ Key, e *encoding) bool { return e.idle() })
			m.stopLives(func(_ Key, l *live) bool { return l.idle() })
			m.stopAudios(func(_ AudioKey, e *audioEncoding) bool { return e.idle() })
		}
	}
}

// LimitConversions bounds the playbacks whose video is converted at once to
// what limit returns, 0 for no limit, files and live alike. It is read
// whenever an encoding would convert the video of one more, so a change
// applies at once; encodings already running go on.
func (m *Manager) LimitConversions(limit func() int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversions = limit
}

// LimitAhead bounds how far past the end of the last segment a player
// asked for FFmpeg makes segments, as a length of picture, to what ahead
// returns, read whenever FFmpeg finishes a segment: FFmpeg makes those
// starting within it, and at least the next, then waits for the player.
// Counting time rather than segments keeps the same picture ready, and
// the source read as far, whatever the length of the version's segments.
func (m *Manager) LimitAhead(ahead func() time.Duration) {
	m.ahead.Store(&ahead)
}

// aheadDuration is how far FFmpeg makes segments past the end of the last
// one asked for (see LimitAhead).
func (m *Manager) aheadDuration() time.Duration {
	if ahead := m.ahead.Load(); ahead != nil {
		if d := (*ahead)(); d > 0 {
			return d
		}
	}
	return defaultAhead
}

// aheadLimit is the last segment of plan FFmpeg makes while the player
// last asked for segment n: the last starting within the ahead limit past
// n's end, and at least the next.
func (m *Manager) aheadLimit(plan Plan, n int) int {
	return max(plan.Segment(plan.End(n)+m.aheadDuration()-time.Nanosecond), n+1)
}

// MayConvert reports whether an encoding converting the video of version
// for user may start now: see admits.
func (m *Manager) MayConvert(user, version string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.admits(Key{User: user, Version: version, Converts: true}, nil)
}

// MayConvertAlone reports whether a conversion streamed as one file may
// start now: see Progressive.
func (m *Manager) MayConvertAlone() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.admitsAlone(Key{Converts: true})
}

// admits reports whether an encoding of key may start: it copies the
// video, or the playbacks whose video is converted are fewer than the
// limit. A playback is what a user plays of a version, whatever the play
// session: apps start a new one to switch audio track or quality, and stop
// the old one once the new one plays. The playback key belongs to, which
// counts already, and the live encoding leaving, about to be replaced, are
// left out. The caller holds m.mu.
func (m *Manager) admits(key Key, leaving *live) bool {
	if !key.Converts {
		return true
	}
	limit := m.limit()
	return limit <= 0 || m.converting(&userVersion{key.User, key.Version}, leaving) < limit
}

// admitsAlone reports whether a conversion streamed as one file, of key,
// may start: it copies the video, or the conversions under way are fewer
// than the limit. It shares no playback: it runs an FFmpeg of its own,
// next to the user's other plays of the version. The caller holds m.mu.
func (m *Manager) admitsAlone(key Key) bool {
	if !key.Converts {
		return true
	}
	limit := m.limit()
	return limit <= 0 || m.converting(nil, nil) < limit
}

// limit is the limit on conversions at once, 0 or less for none.
func (m *Manager) limit() int {
	if m.conversions == nil {
		return 0
	}
	return m.conversions()
}

// userVersion is what a user plays of a version, as the limit on
// conversions counts it: every variant of a playback, and every play
// session of it, counts once.
type userVersion struct{ user, version string }

// converting counts the conversions of video under way: the playbacks
// of HLS encodings, files' and channels', each conversion streamed as
// one file on its own, and each file converted whole (see ConvertFile).
// The playback shared, which counts already, and the live encoding leaving
// are left out. The caller holds m.mu.
func (m *Manager) converting(shared *userVersion, leaving *live) int {
	playbacks := map[userVersion]bool{}
	count := func(k Key) {
		if p := (userVersion{k.User, k.Version}); k.Converts && (shared == nil || p != *shared) {
			playbacks[p] = true
		}
	}
	for k, e := range m.encodings {
		// An encoding whose input switched to one converting its video
		// counts as converting (see carryOn).
		k.Converts = k.Converts || e.converts
		count(k)
	}
	for k, l := range m.lives {
		if l != leaving {
			count(k)
		}
	}
	n := len(playbacks) + m.files
	for p := range m.progressives {
		if p.key.Converts {
			n++
		}
	}
	return n
}

// Init opens the initialization segment of a fragmented MP4 encoding: that
// of its segments other than the fast ones (see FastInit).
func (m *Manager) Init(ctx context.Context, key Key, open Opener) (*os.File, error) {
	if m.keeps(key) {
		return m.keptFile(ctx, key, open, -1)
	}
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return nil, err
	}
	if plan := e.remux.Plan; e.remux.Format != FMP4 || plan.fastFrom == 0 && plan.fastTo == plan.Len() {
		return nil, ErrNotFound
	}
	return e.await(ctx, -1)
}

// FastInit opens the initialization segment of the fast segments of a
// fragmented MP4 encoding (see Plan.FastFrom).
func (m *Manager) FastInit(ctx context.Context, key Key, open Opener) (*os.File, error) {
	if m.keeps(key) {
		return nil, ErrNotFound
	}
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return nil, err
	}
	if e.remux.Format != FMP4 || e.remux.Plan.fastTo == 0 {
		return nil, ErrNotFound
	}
	return e.await(ctx, FastInit)
}

// Segment opens segment n of an encoding, starting or moving FFmpeg to it
// when needed.
func (m *Manager) Segment(ctx context.Context, key Key, open Opener, n int) (*os.File, error) {
	if n < 0 {
		return nil, ErrNotFound
	}
	if m.keeps(key) {
		return m.keptFile(ctx, key, open, n)
	}
	e, err := m.encoding(ctx, key, open)
	if err != nil {
		return nil, err
	}
	if n >= e.remux.Plan.Len() {
		return nil, ErrNotFound
	}
	return e.await(ctx, n)
}

// Subtitles waits until the extracted subtitles of an encoding cover
// segment n, starting or moving FFmpeg to it when needed. They come from
// the variant of key's playback asked for last, whichever key names: every
// variant extracts them. Kept video extracts them, not its audio.
func (m *Manager) Subtitles(ctx context.Context, key Key, open Opener, n int) error {
	if m.keeps(key) {
		key, open = key.keptVideo(), videoAlone(open)
	}
	e := m.playing(key)
	if e == nil {
		var err error
		if e, err = m.encoding(ctx, key, open); err != nil {
			return err
		}
	} else if err := e.waitOpen(ctx); err != nil {
		return err
	}
	if n < 0 || n >= e.remux.Plan.Len() {
		return ErrNotFound
	}
	if len(e.remux.Subtitles) == 0 {
		return nil
	}
	return e.awaitCovered(ctx, n)
}

// playing returns the encoding of key's playback asked for last, nil when
// none was or it was stopped since.
func (m *Manager) playing(key Key) *encoding {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.encodings {
		if k.playback() != key.playback() {
			continue
		}
		e.playback.mu.Lock()
		last := e.playback.last
		e.playback.mu.Unlock()
		if last != nil && m.encodings[last.key] == last {
			return last
		}
		return nil
	}
	return nil
}

// encoding returns the encoding of key, opening it on first use. The
// variants of a playback share its playback, and one opened later starts
// where the player last asked.
func (m *Manager) encoding(ctx context.Context, key Key, open Opener) (*encoding, error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, ErrStopped
	}
	e := m.encodings[key]
	if e == nil {
		if !m.admits(key, nil) {
			m.mu.Unlock()
			return nil, ErrBusy
		}
		var p *playback
		for k, other := range m.encodings {
			if k.playback() == key.playback() {
				p = other.playback
				break
			}
		}
		if p == nil {
			p = &playback{}
		}
		p.mu.Lock()
		p.used = time.Now()
		requested := p.requested
		p.mu.Unlock()
		e = &encoding{m: m, key: key, dir: filepath.Join(m.dir, key.name()), opened: make(chan struct{}),
			changed: make(chan struct{}), quit: make(chan struct{}), playback: p, requested: requested}
		m.encodings[key] = e
		stale := m.keepPlayback(key)
		m.mu.Unlock()
		for _, k := range stale {
			m.expire(k)
		}
		e.open(ctx, open)
	} else {
		m.mu.Unlock()
	}
	if err := e.waitOpen(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

// waitOpen waits until the encoding is open, and forgets one that failed to
// open, which the next request opens again.
func (e *encoding) waitOpen(ctx context.Context) error {
	select {
	case <-e.opened:
	case <-ctx.Done():
		return ctx.Err()
	}
	if e.err != nil {
		e.m.mu.Lock()
		if e.m.encodings[e.key] == e {
			delete(e.m.encodings, e.key)
		}
		e.m.mu.Unlock()
		e.m.sweepKept()
		return e.err
	}
	return nil
}

// encoding is one remux of a version for a play session.
type encoding struct {
	m   *Manager
	key Key
	dir string
	// opened is closed once remux, release and err are set.
	opened  chan struct{}
	remux   Remux
	release func()
	err     error

	mu sync.Mutex
	// changed is closed, then replaced, whenever a segment is ready, a job
	// ends, or a player asks for another segment. init and fastInit are
	// set once the initialization segments of the segments other than the
	// fast ones, and of those, are made.
	changed   chan struct{}
	ready     []bool
	init      bool
	fastInit  bool
	job       *job
	requested int
	// playback is shared with the other variants of the playback; used is
	// when a player last asked for this variant, and linger stops its
	// FFmpeg once the player left it (see lingered).
	playback *playback
	used     time.Time
	linger   *time.Timer
	// runs counts the FFmpeg processes started, which tests read.
	runs    int
	stopped bool
	// quit is closed once the encoding is stopped.
	quit chan struct{}
	// failure is why the input failed for good, nil while it did not (see
	// Remux.Failed).
	failure error
	// lookback is how far before a segment's start FFmpeg starts reading a
	// searched version: Remux.Lookback until a job landed past its first
	// segment's start (see Plan.seekTime and searchFurther).
	lookback time.Duration
	// input is what the jobs read: remux, until its input failed and
	// Remux.Switch carried the encoding on from another one (see carryOn);
	// switching is set while Switch looks for it, and switches counts the
	// inputs switched to, which tests read. converts marks an encoding
	// copying its video whose input switched to converts it, under the
	// manager's lock: it counts toward the conversions at once.
	input     *Remux
	switching bool
	switches  int
	converts  bool
}

func (e *encoding) open(ctx context.Context, open Opener) {
	defer close(e.opened)
	remux, release, err := open(context.WithoutCancel(ctx))
	if err != nil {
		e.err = err
		return
	}
	if err := os.MkdirAll(e.dir, 0o700); err != nil {
		release()
		e.err = err
		return
	}
	input := remux
	e.remux, e.release, e.lookback, e.input = remux, release, remux.Lookback, &input
	e.ready = make([]bool, remux.Plan.Len())
	if remux.Failed != nil {
		go e.watchInput()
	}
}

// errInputFailed is the failure of an input that tells none.
var errInputFailed = errors.New("the input failed")

// switchBudget bounds the search for another input once one failed (see
// Remux.Switch): a player waits on its buffer meanwhile, and its requests
// past the stall timeout are answered that the server is busy, which it
// asks again.
const switchBudget = 40 * time.Second

// watchInput ends the encoding once its input failed for good: the
// requests waiting get the failure, and the next one opens the encoding
// again, through an opener that knows the input failed. An input that
// can be switched for another carries the encoding on instead (see
// carryOn), and that one is watched in turn.
func (e *encoding) watchInput() {
	for {
		e.mu.Lock()
		input := e.input
		e.mu.Unlock()
		if input.Failed == nil {
			return
		}
		select {
		case <-input.Failed:
		case <-e.quit:
			return
		}
		err := errInputFailed
		if input.Failure != nil {
			err = cmp.Or(input.Failure(), errInputFailed)
		}
		if input.Switch != nil && e.carryOn(input, err) {
			continue
		}
		e.mu.Lock()
		e.failure = err
		e.mu.Unlock()
		e.m.logger.Info("A remux was stopped: its source failed", "error", err)
		e.m.mu.Lock()
		if e.m.encodings[e.key] == e {
			delete(e.m.encodings, e.key)
		}
		e.m.mu.Unlock()
		e.stop()
		e.m.sweepKept()
		return
	}
}

// carryOn switches the encoding to another input once input failed for
// good with err, from the first segment the player still waits for, and
// reports whether it did, or whether the encoding stopped meanwhile. The
// segments made stay, and so does their numbering: the next ones come
// from the other input, at the same times, which the player plays on from
// without a new playlist. Requests wait meanwhile, rather than get the
// failure. An input converting video where the encoding copied it counts
// toward the conversions at once: at the limit, there is no switch.
func (e *encoding) carryOn(input *Remux, err error) bool {
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return true
	}
	n := min(e.requested, len(e.ready))
	for n < len(e.ready) && e.ready[n] {
		n++
	}
	if n >= len(e.ready) {
		n = len(e.ready) - 1
	}
	e.switching = true
	if e.job != nil {
		e.job.cancel()
	}
	e.signal()
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), switchBudget)
	defer cancel()
	go func() {
		select {
		case <-e.quit:
			cancel()
		case <-ctx.Done():
		}
	}()
	next, release, switchErr := input.Switch(ctx, n)
	if switchErr == nil && next.Encode != nil && !e.key.Converts {
		e.m.mu.Lock()
		if e.converts = e.converts || e.m.admits(Key{User: e.key.User, Version: e.key.Version, Converts: true}, nil); !e.converts {
			switchErr = ErrBusy
		}
		e.m.mu.Unlock()
	}
	e.mu.Lock()
	e.switching = false
	if switchErr != nil || e.stopped {
		stopped := e.stopped
		e.signal()
		e.mu.Unlock()
		if switchErr == nil {
			release()
		} else {
			e.m.logger.Info("A playback could not go on from another version", "failure", err, "error", switchErr)
		}
		return stopped
	}
	left := e.release
	e.input, e.release, e.lookback = &next, release, next.Lookback
	e.switches++
	e.start(n)
	e.signal()
	e.mu.Unlock()
	left()
	e.m.logger.Info("A playback went on from another version: its source failed", "segment", n, "error", err)
	return true
}

// inputFailed reports whether the input of job j failed for good.
func (j *job) inputFailed() bool {
	select {
	case <-j.input.Failed:
		return true
	default:
		return false
	}
}

// signal wakes whoever waits on the encoding. The caller holds e.mu.
func (e *encoding) signal() {
	close(e.changed)
	e.changed = make(chan struct{})
}

// wait waits for a change, holding e.mu again when it returns.
func (e *encoding) wait(ctx context.Context) error {
	changed := e.changed
	e.mu.Unlock()
	defer e.mu.Lock()
	select {
	case <-changed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// waitProgress waits for a change as wait does, or reports ErrStalled once
// the running job made no progress for the stall timeout, counted from
// since at the earliest: when the request began waiting.
func (e *encoding) waitProgress(ctx context.Context, since time.Time) error {
	last := since
	if j := e.job; j != nil && j.progressed.After(last) {
		last = j.progressed
	}
	left := time.Until(last.Add(e.m.stall))
	if left <= 0 {
		return ErrStalled
	}
	timer := time.NewTimer(left)
	defer timer.Stop()
	changed := e.changed
	e.mu.Unlock()
	defer e.mu.Lock()
	select {
	case <-changed:
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// idle reports whether no player asked anything of the encoding's
// playback, of any variant, for idleTimeout.
func (e *encoding) idle() bool {
	e.playback.mu.Lock()
	defer e.playback.mu.Unlock()
	return time.Since(e.playback.used) > idleTimeout
}

// ask records that a player asked for segment n of the encoding, or for
// something else when n is negative, at at. The variant it takes over
// from is left: its FFmpeg stops once it lingered (see lingered). The
// caller holds e.mu.
func (e *encoding) ask(n int, at time.Time) {
	e.used = at
	if left := e.playback.asked(e, n, at); left != nil {
		// Taking left.mu under e.mu could deadlock with a request for left
		// doing the same with e.
		go left.lingered()
	}
}

// lingered stops the FFmpeg of a variant the player left for another of
// its playback, once it was not asked for in m.linger, and waits for that
// otherwise. The variant asked for last runs on, and the segments made
// stay: a request for this variant serves them, and starts FFmpeg again
// for the others.
func (e *encoding) lingered() {
	e.mu.Lock()
	defer e.mu.Unlock()
	j := e.job
	if e.stopped || j == nil || j.done || e.playback.playing(e) {
		return
	}
	if left := time.Until(e.used.Add(time.Duration(e.m.linger.Load()))); left > 0 {
		if e.linger == nil {
			e.linger = time.AfterFunc(left, e.lingered)
		} else {
			e.linger.Reset(left)
		}
		return
	}
	e.m.logger.Debug("Stopped the remux of a quality the player left", "variant", e.key.Variant, "from", j.start)
	j.cancel()
}

func (e *encoding) path(n int) string {
	switch {
	case n == FastInit:
		return filepath.Join(e.dir, "init-fast.mp4")
	case n < 0:
		return filepath.Join(e.dir, "init.mp4")
	}
	return filepath.Join(e.dir, strconv.Itoa(n)+"."+e.remux.Format.Extension())
}

// made reports whether segment n is made, or for n = -1 and FastInit,
// that initialization segment. The caller holds e.mu.
func (e *encoding) made(n int) bool {
	switch {
	case n == FastInit:
		return e.fastInit
	case n < 0:
		return e.init
	}
	return e.ready[n]
}

// initStart is the segment a job making initialization segment n starts
// at, the initialization segment coming with any segment of its kind:
// where the player last asked, if it did, or the first fast segment when
// the player last asked for another. A job starting at a fast segment
// goes on with the others once the fast ones are made, if others follow;
// else the others' starts at the one before them. The caller holds e.mu.
func (e *encoding) initStart(n int) int {
	plan := e.remux.Plan
	switch {
	case n == FastInit && !plan.Fast(e.requested):
		return plan.fastFrom
	case n != FastInit && plan.Fast(e.requested) && plan.fastTo == plan.Len():
		return plan.fastFrom - 1
	}
	return e.requested
}

// makesInit reports whether job j makes initialization segment n, of its
// own segments or of those it goes on with (see stretchEnd). The caller
// holds e.mu.
func (e *encoding) makesInit(j *job, n int) bool {
	plan := e.remux.Plan
	if n == FastInit {
		return j.fast || plan.fastTo > 0 && j.end == plan.fastFrom
	}
	return !j.fast || j.end < plan.Len()
}

// await opens segment n, or the initialization segment for n = -1 or
// FastInit, once it is made. A job that makes no progress for the stall
// timeout is not waited for longer: ErrStalled.
func (e *encoding) await(ctx context.Context, n int) (*os.File, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	since := time.Now()
	e.ask(n, since)
	if n >= 0 {
		e.requested = n
		e.prune(n)
		e.signal()
	}
	for {
		if e.failure != nil {
			return nil, e.failure
		}
		if e.stopped {
			return nil, ErrStopped
		}
		if e.made(n) {
			return os.Open(e.path(n))
		}
		j := e.job
		start := n
		if n < 0 {
			start = e.initStart(n)
		}
		switch {
		case e.switching:
			// Another input is being found: the job that failed is no
			// answer, and none starts until it is there.
		case j != nil && j.done && j.err != nil && !errors.Is(j.err, errStale) && j.start == start:
			// Reported once: the next request tries again.
			e.job = nil
			return nil, j.err
		case j != nil && j.done && j.err == nil && n >= j.start && n >= j.next:
			// FFmpeg reached the end of the version before this segment.
			return nil, ErrNotFound
		case j == nil || j.done || (n >= 0 && (n < j.next || n > j.next+reach)) || (n < 0 && !e.makesInit(j, n)):
			e.start(start)
		}
		if err := e.waitProgress(ctx, since); err != nil {
			return nil, err
		}
	}
}

// awaitCovered waits until the extracted subtitles cover segment n. The
// job making it, or about to, is waited for; another starts from n.
func (e *encoding) awaitCovered(ctx context.Context, n int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	since := time.Now()
	e.ask(-1, since)
	plan := e.remux.Plan
	for {
		if e.failure != nil {
			return e.failure
		}
		if e.stopped {
			return ErrStopped
		}
		if e.remux.Extracted.Covers(plan.Start(n), plan.End(n)) {
			return nil
		}
		j := e.job
		switch {
		case e.switching:
		case j != nil && j.done && j.err != nil && !errors.Is(j.err, errStale) && j.start == n:
			e.job = nil
			return j.err
		case j == nil || j.done || n < j.start || n > j.next+reach:
			e.start(n)
		}
		// A player may fetch subtitles ahead of the video: FFmpeg goes on
		// until it reaches them.
		if n > e.requested {
			e.requested = n
			e.signal()
		}
		if err := e.waitProgress(ctx, since); err != nil {
			return err
		}
	}
}

// prune removes the segments far from segment n: more than behind before
// it, or past what FFmpeg makes ahead of it and reach. The caller holds
// e.mu.
func (e *encoding) prune(n int) {
	far := e.m.aheadLimit(e.remux.Plan, n) + reach
	for i, ready := range e.ready {
		if ready && (i < n-behind || i > far) {
			e.ready[i] = false
			if err := os.Remove(e.path(i)); err != nil && !errors.Is(err, os.ErrNotExist) {
				e.m.logger.Warn("A segment could not be removed", "error", err)
			}
		}
	}
}

// stop ends the encoding and removes its segments.
func (e *encoding) stop() {
	<-e.opened
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return
	}
	e.stopped = true
	close(e.quit)
	if e.job != nil {
		e.job.cancel()
	}
	if e.linger != nil {
		e.linger.Stop()
	}
	e.signal()
	release := e.release
	e.mu.Unlock()
	if e.err != nil {
		return
	}
	release()
	if err := os.RemoveAll(e.dir); err != nil {
		e.m.logger.Warn("The segments of an encoding could not be removed", "error", err)
	}
}

// job is one run of FFmpeg, from segment start up to end excluded.
type job struct {
	start int
	// next is the segment being made, and end the first one the job does
	// not make: the first made otherwise (see Plan.stretchEnd), which
	// another job makes, or the plan's length.
	next, end int
	// fast marks a job making fast segments (see Plan.FastFrom).
	fast   bool
	cancel context.CancelFunc
	done   bool
	err    error
	// file receives the segment being made, once its first keyframe came.
	file *os.File
	// progressed is when FFmpeg last read or wrote, as far as it tells,
	// or else when the job started.
	progressed time.Time
	// lookback is the encoding's when the job started (see
	// Plan.seekTime), and landed is set once FFmpeg's first keyframe, where
	// it landed, came (see searchFurther).
	lookback time.Duration
	landed   bool
	// input is what the job reads: the encoding's when it started (see
	// carryOn), converting the video as Remux.Fast says for fast segments.
	input *Remux
}

// start replaces the running job with one starting at segment n, which
// makes the segments up to the first made otherwise. The caller holds
// e.mu.
func (e *encoding) start(n int) {
	if e.job != nil {
		e.job.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	plan, input := e.remux.Plan, e.input
	fast := plan.Fast(n)
	if fast && input.Fast != nil {
		faster := *input
		faster.Encode, faster.VideoTag = input.Fast, input.FastTag
		input = &faster
	}
	j := &job{start: n, next: n, end: plan.stretchEnd(n), fast: fast, cancel: cancel, progressed: time.Now(), lookback: e.lookback, input: input}
	e.job = j
	go e.run(ctx, j)
}

// progress records that a job made progress.
func (e *encoding) progress(j *job) {
	e.mu.Lock()
	defer e.mu.Unlock()
	j.progressed = time.Now()
}

// run runs FFmpeg for a job and files what it writes into segments, and
// the cues of the subtitles it extracts.
func (e *encoding) run(ctx context.Context, j *job) {
	cmd := exec.CommandContext(ctx, e.m.ffmpegPath(), j.input.args(j.start, j.lookback)...)
	cmd.WaitDelay = 5 * time.Second
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	output, err := cmd.StdoutPipe()
	// Each extracted subtitle stream comes on an output of its own, pipe:3
	// on, then FFmpeg's progress (see Remux.args).
	var outputs *process.Outputs
	if err == nil {
		outputs, err = process.NewOutputs(cmd, len(j.input.Subtitles)+1)
	}
	if err == nil {
		err = cmd.Start()
		outputs.Started(cmd, err)
	}
	if err != nil {
		e.finish(j, fmt.Errorf("start FFmpeg: %w", err))
		return
	}
	readers := outputs.Readers
	e.mu.Lock()
	e.runs++
	e.mu.Unlock()
	var extracting sync.WaitGroup
	progress := readers[len(readers)-1]
	extracting.Go(func() {
		defer progress.Close()
		watchProgress(progress, func() { e.progress(j) })
	})
	for i, r := range readers[:len(readers)-1] {
		stream := j.input.Subtitles[i]
		extracting.Go(func() {
			defer r.Close()
			if err := subtitles.Scan(r, func(cue subtitles.Cue) { j.input.Extracted.Add(stream, cue) }); err != nil {
				e.m.logger.Warn("A subtitle could not be extracted", "stream", stream, "error", err)
				// FFmpeg would wait for the pipe to be read.
				_, _ = io.Copy(io.Discard, r)
			}
		})
	}
	started := time.Now()
	e.m.logger.Debug("Started a remux", "from", j.start, "variant", e.key.Variant, "fast", j.fast)
	take := func(p piece) error { return e.take(ctx, j, p) }
	if e.remux.Format == TS {
		err = splitTS(output, take)
	} else {
		err = splitMP4(output, take)
	}
	// A job replaced or stopped from outside is stale; one whose output
	// could not be filed failed, and stops FFmpeg itself.
	replaced := errors.Is(err, errStale) || ctx.Err() != nil
	if err != nil && !replaced {
		j.cancel()
	}
	// FFmpeg is waited for once its output is drained or abandoned.
	_, _ = io.Copy(io.Discard, output)
	waitErr := cmd.Wait()
	// The cues FFmpeg wrote before it ended are added before the job
	// finishes and covers them.
	extracting.Wait()
	switch {
	case replaced:
		err = errStale
	case err == nil && waitErr != nil:
		err = fmt.Errorf("FFmpeg failed: %w: %s", waitErr, bytes.TrimSpace(stderr.bytes()))
	case err == nil && j.inputFailed():
		// FFmpeg takes an input cut short for its end: what it wrote last
		// is not the version's.
		err = errInputFailed
	}
	switch {
	case errors.Is(err, errStale):
		e.m.logger.Debug("A remux was stopped", "from", j.start, "duration", time.Since(started))
	case err != nil:
		e.m.logger.Warn("A remux failed", "from", j.start, "error", err)
		metrics.EncodingFailed(j.input.Encode.method())
	default:
		e.m.logger.Debug("A remux reached the end", "from", j.start, "duration", time.Since(started))
	}
	e.finish(j, err)
}

// take files a piece of FFmpeg's output.
func (e *encoding) take(ctx context.Context, j *job, p piece) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.job != j || e.stopped {
		return errStale
	}
	j.progressed = time.Now()
	if p.init {
		which := -1
		if j.fast {
			which = FastInit
		}
		if e.made(which) {
			return nil
		}
		if err := writeFile(e.path(which), p.data); err != nil {
			return err
		}
		if j.fast {
			e.fastInit = true
		} else {
			e.init = true
		}
		e.signal()
		return nil
	}
	plan := e.remux.Plan
	if p.keyframe && !j.landed {
		j.landed = true
		if e.searchFurther(j, p.at) {
			return errStale
		}
	}
	if j.file == nil {
		// FFmpeg may start on a keyframe before the segment: what comes
		// before it is not part of the job.
		if !p.keyframe || p.at < plan.Start(j.next)-j.input.cutTolerance() {
			return nil
		}
		return e.begin(j, p.data)
	}
	if p.keyframe && j.next+1 < plan.Len() && p.at >= plan.Start(j.next+1)-j.input.cutTolerance() {
		if err := e.close(j); err != nil {
			return err
		}
		// Make segments as far ahead of the last one asked for as allowed.
		for j.next > e.m.aheadLimit(plan, e.requested) {
			if err := e.wait(ctx); err != nil {
				return errStale
			}
			if e.job != j || e.stopped {
				return errStale
			}
		}
		// The segments from the job's end are made otherwise, by another
		// job, from that keyframe on.
		if j.next == j.end {
			e.m.logger.Debug("A remux goes on otherwise", "segment", j.end, "fast", plan.Fast(j.end))
			e.start(j.end)
			e.signal()
			return errStale
		}
		return e.begin(j, p.data)
	}
	_, err := j.file.Write(p.data)
	return err
}

// begin starts writing segment j.next. The caller holds e.mu.
func (e *encoding) begin(j *job, data []byte) error {
	file, err := os.CreateTemp(e.dir, "partial-*")
	if err != nil {
		return err
	}
	j.file = file
	e.playback.began(e, j.next)
	_, err = file.Write(data)
	return err
}

// close completes segment j.next and moves the job to the next. The caller
// holds e.mu.
func (e *encoding) close(j *job) error {
	file := j.file
	if file == nil {
		return nil
	}
	j.file = nil
	err := file.Close()
	if err == nil {
		err = os.Rename(file.Name(), e.path(j.next))
	}
	if err != nil {
		_ = os.Remove(file.Name())
		return err
	}
	e.ready[j.next] = true
	metrics.SegmentMade(j.input.Encode.method())
	// The cues starting before the segment just made are counted as
	// extracted once they had time to come, leaving that segment's length
	// as a margin too.
	if j.next > j.start {
		from, to := e.remux.Plan.Start(j.start), e.remux.Plan.Start(j.next)
		time.AfterFunc(settle, func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			e.cover(from, to)
			e.signal()
		})
	}
	j.next++
	e.signal()
	return nil
}

// cover records that the subtitles a job extracts cover [from, to).
func (e *encoding) cover(from, to time.Duration) {
	if len(e.remux.Subtitles) > 0 {
		e.remux.Extracted.Cover(from, to)
	}
}

// searchFurther starts again further back a job of a searched version
// whose FFmpeg landed past the start of its first segment, on a keyframe
// at at, and reports whether it did. FFmpeg found no keyframe between the
// time it was asked and the segment's start: it is asked again as much
// further back as it went on past that time, at least twice as far back,
// until it lands on or before the segment or reads the version from its
// start. The encoding's later jobs start as far back, and Remux.Learned
// is told. The caller holds e.mu.
func (e *encoding) searchFurther(j *job, at time.Duration) bool {
	plan := e.remux.Plan
	if !plan.searched || j.start == 0 || at <= plan.Start(j.start)+tolerance {
		return false
	}
	from := plan.seekTime(j.start, j.lookback)
	if from == 0 {
		return false
	}
	e.lookback = plan.Start(j.start) - from + at - from
	e.m.logger.Debug("FFmpeg landed past a segment's start: searching further back", "segment", j.start, "asked", from, "landed", at,
		"lookback", e.lookback)
	if learned := e.remux.Learned; learned != nil {
		go learned(e.lookback)
	}
	e.start(j.start)
	return true
}

// finish records the end of a job; one that reached the end of its output
// completes its last segment, and has extracted every cue from its start.
// One that reached it without starting its first segment, FFmpeg finding
// no keyframe it could start it on past the time asked in a searched
// version, starts again further back.
func (e *encoding) finish(j *job, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil && e.job == j && !e.stopped && !j.inputFailed() {
		plan := e.remux.Plan
		if j.next == j.start && j.file == nil && e.searchFurther(j, plan.End(plan.Len()-1)) {
			err = errStale
		} else if err = e.close(j); err == nil {
			e.cover(plan.Start(j.start), plan.End(plan.Len()-1))
		}
	}
	if j.file != nil {
		_ = j.file.Close()
		_ = os.Remove(j.file.Name())
		j.file = nil
	}
	j.done, j.err = true, err
	if e.job == j {
		e.signal()
	}
}

func writeFile(path string, data []byte) error {
	partial := path + ".partial"
	if err := os.WriteFile(partial, data, 0o600); err != nil {
		return err
	}
	return os.Rename(partial, path)
}

// args is FFmpeg's command line for a job starting at segment n, lookback
// before it in a searched version (see Plan.seekTime). Segments keep the
// source's timestamps, shifted by timestampOffset, so that those of
// different jobs follow each other; an input switched to has its own moved
// back by its Shift, so that they follow those of the input before it.
// Extracted subtitles keep the source's own. FFmpeg reports its progress
// on the pipe after the subtitles'. A subtitle file burned in is the
// second input, read whole (see burnGraph).
func (r Remux) args(n int, lookback time.Duration) []string {
	args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-progress", "pipe:" + strconv.Itoa(3+len(r.Subtitles))}
	seek, from := r.seekArgs(n, lookback)
	args = append(args, seek...)
	if r.Encode != nil {
		args = append(args, r.Encode.inputs()...)
	}
	args = append(args, r.InputOptions...)
	args = append(args, "-copyts", "-i", r.Input)
	video := "0:" + strconv.Itoa(r.Video)
	if r.Encode != nil && r.Encode.Burns() {
		if r.Encode.BurnFile != "" {
			args = append(args, "-i", r.Encode.BurnFile)
		}
		args = append(args, "-filter_complex", r.Encode.burnGraph(r.Video, from, r.Encode.skip(r.Plan, n)))
		video = "[video]"
	}
	args = append(args, "-map", video)
	if r.Audio >= 0 {
		args = append(args, "-map", "0:"+strconv.Itoa(r.Audio))
	}
	args = append(args, "-map_metadata", "-1", "-map_chapters", "-1", "-c", "copy")
	if r.Encode != nil {
		args = append(args, r.Encode.shiftedArgs(r.Plan, n, r.Shift)...)
	}
	if r.VideoTag != "" {
		args = append(args, "-tag:v", r.VideoTag)
	}
	args = append(args, r.audioArgs()...)
	if r.Audio >= 0 {
		args = append(args, "-bsf:a", r.audioFilters())
	}
	args = append(args, r.threadArgs()...)
	args = append(args, "-avoid_negative_ts", "disabled", "-output_ts_offset", strconv.FormatFloat((timestampOffset-r.Shift).Seconds(), 'f', -1, 64))
	if r.Format == TS {
		args = append(args, "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1")
	} else {
		// A fragment per keyframe, with the real decode times: without an
		// edit list, presentation times are those of the source, for video
		// as for audio. The movie header waits for the first fragment, as
		// some codecs, such as E-AC-3, describe themselves in their first
		// packet.
		args = append(args, "-use_editlist", "0", "-f", "mp4", "-movflags", "+frag_keyframe+empty_moov+delay_moov+default_base_moof+frag_discont", "pipe:1")
	}
	// Each subtitle stream is written as WebVTT, cue by cue, on its pipe.
	for i, stream := range r.Subtitles {
		args = append(args, "-map", "0:"+strconv.Itoa(stream), "-c:s", "webvtt", "-flush_packets", "1", "-f", "webvtt", "pipe:"+strconv.Itoa(3+i))
	}
	return args
}

// seekArgs are FFmpeg's options reading the input from segment n, lookback
// before it in a searched version, and the time it reads from, zero for
// the start: in the input's times, the plan's moved by Shift.
func (r Remux) seekArgs(n int, lookback time.Duration) ([]string, time.Duration) {
	var from time.Duration
	if n > 0 {
		from = r.Plan.seekTime(n, lookback)
	}
	from += r.Shift
	if from <= 0 {
		return nil, 0
	}
	// Converted audio starts where the demuxer does, on the keyframe, as
	// copied streams do, instead of at the time asked.
	args := []string{"-noaccurate_seek", "-ss", strconv.FormatFloat(from.Seconds(), 'f', 6, 64)}
	// The version was analyzed already: FFmpeg probes the head of the file
	// again before it seeks, and 2 MB of it, which the analysis left in the
	// source cache, describe the streams it copies or decodes. 0 would be
	// FFmpeg's 5 s, not none. A burned image track may need more to know
	// its size, and an MPEG-TS file or disc to find every stream: both
	// probe as usual.
	if (r.Encode == nil || r.Encode.Burn == nil) && !r.Plan.Grid() && !r.Plan.searched {
		args = append(args, "-probesize", "2M", "-analyzeduration", "1M")
	}
	return args, from
}

// audioFilters are the bitstream filters of the audio: audio before zero,
// such as encoder priming, is not played, and AAC copied from MPEG-TS is
// reframed as MP4 stores it.
func (r Remux) audioFilters() string {
	filters := `noise=drop=lt(pts\,0)`
	if r.ADTS && r.AudioCodec == "" && r.Format == FMP4 {
		filters = "aac_adtstoasc," + filters
	}
	return filters
}

// tail keeps the end of what a process writes.
type tail struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.data = append(t.data, p...)
	if extra := len(t.data) - t.limit; extra > 0 {
		t.data = t.data[extra:]
	}
	return len(p), nil
}

func (t *tail) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return bytes.Clone(t.data)
}

// watchProgress reads what FFmpeg's -progress writes, a block of key=value
// lines every half second ending with progress=, and calls progressed
// after each block where FFmpeg wrote more, or moved its frames or its
// output time on: blocks come on even while FFmpeg waits for its input.
func watchProgress(r io.Reader, progressed func()) {
	scanner := bufio.NewScanner(r)
	var last, current string
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		switch key {
		case "frame", "total_size", "out_time_us":
			current += key + "=" + value + ";"
		case "progress":
			if current != last {
				progressed()
			}
			last, current = current, ""
		}
	}
	// FFmpeg would wait for the pipe to be read.
	_, _ = io.Copy(io.Discard, r)
}
