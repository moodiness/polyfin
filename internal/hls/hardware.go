package hls

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Hardware is a GPU FFmpeg decodes and encodes video on. Decoded frames
// either stay in the GPU's memory until they are encoded, scaled there in
// SDR (see Resident) and tone mapped there from Vulkan frames (see
// VulkanDecoding) or by the GPU's own video processing (see
// deviceToneMapping), or come back to memory for the filters and go to
// the GPU again to be encoded: Polyfin times both at startup and keeps the
// faster for each kind of conversion. Burned subtitles, tone mapping on
// the processor and decoding left to the processor go through memory. A
// GPU that cannot decode a codec leaves it to FFmpeg's own decoder, and
// the conversion looks the same either way.
type Hardware struct {
	// Method is the GPU method: cuda for NVIDIA GPUs, on Linux and
	// Windows; vaapi for AMD and Intel ones on Linux; videotoolbox on
	// macOS; qsv for Intel GPUs and amf for AMD ones on Windows, which
	// both decode through D3D11VA.
	Method string
	// Device is the render node VAAPI opens.
	Device string
	// Encoders are those that encoded on it at startup, such as h264_nvenc.
	Encoders []string
	// ToneMapping is set when the GPU also scales video and tone maps HDR
	// to SDR, which a short HDR10 sample proved at startup. NVIDIA's do it
	// through libplacebo on their Vulkan driver, which applies Dolby
	// Vision's metadata. Intel's do it through their video processing,
	// tonemap_vaapi on Linux and vpp_qsv on Windows, for HDR10 whose
	// frames carry their mastering display. VideoToolbox does it through
	// scale_vt, for HDR10 and HLG. AMD's never do: on Linux, the frames
	// libplacebo imports from memory set off a fault in the driver,
	// mapping VAAPI frames into Vulkan fails, and Vulkan's HEVC decoder
	// hung the GPU; on Windows, FFmpeg's AMF filter converts colors but
	// does not tone map.
	ToneMapping bool
	// QVBR is set when a VAAPI driver takes a quality factor within a
	// maximum rate, which a quality set in the settings needs there: AMD's
	// drivers often lack it.
	QVBR bool
	// Resident is set when SDR frames the GPU decoded stay in its memory to
	// be scaled, by scale_cuda, scale_vaapi, scale_vt, scale_qsv or
	// vpp_amf, and encoded, which worked and was faster than going through
	// memory at startup. Deinterlacers are the deinterlacing filters that
	// deinterlaced such frames at startup, and those a GPU tone maps:
	// yadif_cuda and bwdif_cuda, deinterlace_vaapi, yadif_videotoolbox,
	// or vpp_qsv.
	Resident      bool
	Deinterlacers []string
	// VulkanDecoding is set when the GPU of ToneMapping also decodes HEVC
	// in 10 bits into Vulkan frames, which libplacebo tone maps as they
	// are, rather than into memory it uploads them from again, and that
	// was faster at startup. FFmpeg maps no CUDA frame to Vulkan, nor back:
	// libplacebo's output, at the size converted to, goes to NVENC through
	// memory.
	VulkanDecoding bool
}

// gpuMethod is a GPU method, named as POLYFIN_HWACCEL names it, with the
// systems offering it, the encoders Polyfin uses, the filters frames kept
// in the GPU's memory need, and the deinterlacers taking them.
type gpuMethod struct {
	name, method  string
	systems       []string
	encoders      []string
	filters       []string
	deinterlacers []string
}

// hardwareMethods are the GPU methods by preference. Each system offers
// its own, as accounts.HardwareAccelerationsOn lists them: NVIDIA and
// VAAPI on Linux, VideoToolbox on macOS, NVIDIA, Intel's QSV and AMD's
// AMF on Windows.
var hardwareMethods = []gpuMethod{
	{"nvenc", "cuda", []string{"linux", "windows"}, []string{"h264_nvenc", "hevc_nvenc"}, []string{"hwupload", "scale_cuda"},
		[]string{"yadif_cuda", "bwdif_cuda"}},
	{"vaapi", "vaapi", []string{"linux"}, []string{"h264_vaapi", "hevc_vaapi"}, []string{"hwupload", "scale_vaapi"}, []string{"deinterlace_vaapi"}},
	{"videotoolbox", "videotoolbox", []string{"darwin"}, []string{"h264_videotoolbox", "hevc_videotoolbox"}, []string{"hwupload", "scale_vt"},
		[]string{"yadif_videotoolbox"}},
	{"qsv", "qsv", []string{"windows"}, []string{"h264_qsv", "hevc_qsv"}, []string{"hwupload", "hwmap", "scale_qsv"}, []string{"vpp_qsv"}},
	{"amf", "amf", []string{"windows"}, []string{"h264_amf", "hevc_amf"}, []string{"hwupload", "vpp_amf"}, nil},
}

// methods are the GPU methods detection tries: those of m.system, every
// one when it is empty, as tests leave it.
func (m *Manager) methods() []gpuMethod {
	if m.system == "" {
		return hardwareMethods
	}
	var methods []gpuMethod
	for _, method := range hardwareMethods {
		if slices.Contains(method.systems, m.system) {
			methods = append(methods, method)
		}
	}
	return methods
}

// DetectHardware chooses the GPU video is converted on, by encoding a few
// frames with each encoder of the methods this system offers: with want
// auto, the first that encodes, by preference (NVIDIA then VAAPI on
// Linux, NVIDIA then QSV then AMF on Windows, VideoToolbox on macOS),
// VAAPI on device or else on each render node in turn; with a method's
// name, that one only; with none, none. Each want is detected once:
// choosing it again switches to what was found, encodings already running
// going on as they started. It reports false when no GPU encodes, and
// conversions then run on the processor. The GPU's conversion chains are
// then timed in the background (see measure): conversions starting before
// that ends go through memory.
func (m *Manager) DetectHardware(want, device string) (Hardware, bool) {
	m.detecting.Lock()
	defer m.detecting.Unlock()
	hw, done := m.detected[want]
	if !done {
		var failures []gpuFailure
		hw, failures = m.detect(want, device)
		m.detected[want] = hw
		if m.failed == nil {
			m.failed = map[string][]gpuFailure{}
		}
		m.failed[want] = failures
	}
	m.hardware.Store(hw)
	failures := m.failed[want]
	m.failures.Store(&failures)
	if !done {
		m.measureLater(want, hw)
	}
	if hw == nil {
		return Hardware{}, false
	}
	return *hw, true
}

// SelectHardware chooses the GPU as DetectHardware does, and logs the
// outcome, after why each GPU method tried before or instead of the one
// chosen failed, at the outcome's level. The first choice also times HDR
// tone mapped on the processor in the background, once the GPU's chains
// are (see timeToneMapping).
func (m *Manager) SelectHardware(want, device string) {
	hw, ok := m.DetectHardware(want, device)
	level := slog.LevelInfo
	if !ok && want != "auto" {
		level = slog.LevelWarn
	}
	for _, failure := range *m.failures.Load() {
		attrs := []any{"hwaccel", failure.method, "encoder", failure.encoder, "error", failure.err}
		if failure.device != "" {
			attrs = append(attrs, "device", failure.device)
		}
		m.logger.Log(context.Background(), level, "A GPU method does not encode", attrs...)
	}
	switch {
	case ok:
		m.logger.Info("Video is converted on the GPU", "method", hw.Method, "device", hw.Device, "encoders", hw.Encoders,
			"tone_mapping", hw.ToneMapping, "qvbr", hw.QVBR)
	case want == "auto":
		m.logger.Info("Video is converted in software: no GPU encodes")
	case want != "none":
		m.logger.Warn("Video is converted in software: the GPU asked for does not encode", "hwaccel", want)
	}
	m.toneMappingTimed.Do(m.timeToneMappingLater)
}

// gpuFailure is why a GPU method, on device for VAAPI, encoded nothing:
// FFmpeg's error with the first of its encoders that failed. driver is
// the NVIDIA driver version NVENC needs, when FFmpeg found the GPU's
// older, empty otherwise.
type gpuFailure struct {
	method, device, encoder string
	err                     error
	driver                  string
}

// olderDriver matches what FFmpeg writes when NVENC needs a newer NVIDIA
// driver than the GPU's, with the version it needs. FFmpeg writes that
// version after a missing NVENC library too, a failure of another kind.
var olderDriver = regexp.MustCompile(`(?s)Driver does not support the required nvenc API version.*The minimum required Nvidia driver for nvenc is ([0-9][0-9.]*) or newer`)

// NVIDIADriver is the NVIDIA driver version FFmpeg's NVENC needs, when the
// detection of the GPU chosen last found the GPU's driver older, empty
// otherwise.
func (m *Manager) NVIDIADriver() string {
	if failures := m.failures.Load(); failures != nil {
		for _, failure := range *failures {
			if failure.driver != "" {
				return failure.driver
			}
		}
	}
	return ""
}

// detect finds the GPU want asks for (see DetectHardware), nil for none,
// and why each method tried before it, or instead of it, failed.
func (m *Manager) detect(want, device string) (*Hardware, []gpuFailure) {
	var failures []gpuFailure
	for _, candidate := range m.methods() {
		if want != "auto" && want != candidate.name {
			continue
		}
		devices := []string{""}
		if candidate.method == "vaapi" {
			devices = renderNodes(device)
		}
		for _, node := range devices {
			hw := Hardware{Method: candidate.method, Device: node}
			var failure *gpuFailure
			for _, encoder := range candidate.encoders {
				if !slices.Contains(m.can.encoders, encoder) {
					continue
				}
				err := m.encodes(hw, encoder)
				if err == nil {
					hw.Encoders = append(hw.Encoders, encoder)
				} else if failure == nil {
					failure = &gpuFailure{method: candidate.name, device: node, encoder: encoder, err: err}
				}
			}
			if len(hw.Encoders) > 0 {
				hw.ToneMapping = m.toneMaps(hw)
				hw.QVBR = hw.Method == "vaapi" && m.encodes(hw, hw.Encoders[0], "-rc_mode", "QVBR", "-global_quality", "25", "-b:v", "1000000", "-maxrate", "1500000") == nil
				return &hw, failures
			}
			if failure != nil {
				if found := olderDriver.FindStringSubmatch(failure.err.Error()); found != nil && candidate.name == "nvenc" {
					failure.driver = found[1]
				}
				failures = append(failures, *failure)
			}
		}
	}
	return nil, failures
}

// toneMaps reports whether hw tone maps HDR to SDR, as conversions would:
// NVIDIA's GPU with libplacebo, Intel's with tonemap_vaapi or vpp_qsv,
// and VideoToolbox with scale_vt. AMD's are never tried (see
// Hardware.ToneMapping).
func (m *Manager) toneMaps(hw Hardware) bool {
	x265 := slices.Contains(m.can.encoders, "libx265")
	switch {
	case hw.Method == "cuda":
		return m.HasFilters("libplacebo") && m.placeboToneMaps(hw)
	case hw.Method == "vaapi" && m.vendorOf(hw.Device) == intelVendor:
		return m.HasFilters("hwupload", "scale_vaapi", "tonemap_vaapi") && x265 && m.deviceToneMaps(hw)
	case hw.Method == "videotoolbox":
		return m.HasFilters("hwupload", "scale_vt") && x265 && m.deviceToneMaps(hw)
	case hw.Method == "qsv":
		return m.HasFilters("hwupload", "hwmap", "vpp_qsv") && x265 && m.deviceToneMaps(hw)
	}
	return false
}

// placeboToneMaps reports whether hw tone maps a quarter of a second of
// test pattern, tagged as HDR10, on its Vulkan device: NVIDIA's Vulkan
// driver may be missing from the container, or fail.
func (m *Manager) placeboToneMaps(hw Hardware) bool {
	hw.ToneMapping = true
	v := VideoEncoding{Width: 160, Height: 90, ToneMap: true, Hardware: &hw}
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, toneMappingDevices...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25",
		"-vf", "format=yuv420p10le,"+hdrTags+","+v.filters(),
		"-c:v", hw.Encoders[0], "-f", "null", "-")
	return m.succeeds(args...)
}

// deviceToneMaps reports whether hw, Intel's GPU or VideoToolbox, tone
// maps a quarter of a second of HDR10 through the chain conversions use,
// decoded on the GPU (see deviceToneMapping). tonemap_vaapi and vpp_qsv
// take no frame without the mastering display of HDR10, which no filter
// adds: the sample is HEVC from x265, which writes it, and FFmpeg's
// decoder passes it on to the frames.
func (m *Manager) deviceToneMaps(hw Hardware) bool {
	dir, err := os.MkdirTemp(m.dir, "tone-mapping-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(dir)
	sample := filepath.Join(dir, "hdr10.mkv")
	if !m.succeeds(slices.Concat(probeStart, []string{"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25",
		"-vf", "format=yuv420p10le," + hdrTags, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", hdr10Params, sample})...) {
		return false
	}
	hw.ToneMapping = true
	v := VideoEncoding{Width: 160, Height: 90, ToneMap: true, Hardware: &hw}
	return m.succeeds(slices.Concat(probeStart, v.inputs(), []string{"-i", sample, "-vf", v.filters(), "-c:v", hw.Encoders[0], "-f", "null", "-"})...)
}

// hdr10Params are x265's options writing the static metadata of HDR10: a
// BT.2020 display mastered at 1000 nits, and content up to 1000 nits.
const hdr10Params = "log-level=error:hdr10=1:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400"

// The PCI vendor IDs of Intel and AMD, as Linux names a render node's and
// Direct3D 11 an adapter's.
const (
	intelVendor = "0x8086"
	amdVendor   = "0x1002"
)

// vendorOf is the PCI vendor ID of the GPU behind a render node, such as
// 0x8086 for Intel and 0x1002 for AMD, empty when unknown, through
// m.vendor when tests set it.
func (m *Manager) vendorOf(node string) string {
	if m.vendor != nil {
		return m.vendor(node)
	}
	data, err := os.ReadFile(filepath.Join("/sys/class/drm", filepath.Base(node), "device", "vendor"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// hdrTags tags test pattern as HDR10.
const hdrTags = "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc"

// sampleLength is how long the samples timed at startup last: 4K at 24
// frames a second, about half a second to convert each way on a GPU.
const sampleLength = 2 * time.Second

// measureLater times hw's conversion chains in the background (see
// measure), then puts what it found in hw's place, as the choice of want
// and as the GPU chosen, unless another was chosen meanwhile. Close stops
// it.
func (m *Manager) measureLater(want string, hw *Hardware) {
	if hw == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.measuring.Add(1)
	m.timingGPU.Add(1)
	go func() {
		defer m.measuring.Done()
		defer m.timingGPU.Done()
		defer cancel()
		go func() {
			select {
			case <-m.done:
				cancel()
			case <-ctx.Done():
			}
		}()
		measured := m.measure(ctx, *hw)
		m.detecting.Lock()
		if m.detected[want] == hw {
			m.detected[want] = &measured
		}
		m.detecting.Unlock()
		m.hardware.CompareAndSwap(hw, &measured)
	}()
}

// timing is how long a sample took to convert through memory and kept on
// the GPU, with how each failed, if it did.
type timing struct {
	memory, gpu       time.Duration
	memoryErr, gpuErr error
}

// gpuWins reports whether frames kept on the GPU convert faster: they
// converted, and through memory failed or took longer.
func (t timing) gpuWins() bool {
	return t.gpuErr == nil && (t.memoryErr != nil || t.gpu < t.memory)
}

// speed is how many times real time a sample converted in d, or failed.
func speed(d time.Duration, err error) string {
	if err != nil {
		return "failed"
	}
	return strconv.FormatFloat(sampleLength.Seconds()/max(d, time.Millisecond).Seconds(), 'f', 1, 64) + "x"
}

// measure converts a sample through memory and on the GPU, for each kind
// of conversion hw may keep there, and keeps the faster: SDR H.264 scaled
// on the GPU (Resident), and on a GPU that tone maps, HDR10 HEVC in 10
// bits decoded into Vulkan frames for libplacebo (VulkanDecoding), once
// Vulkan proved to decode it. Every failure leaves frames going through
// memory. The deinterlacers of frames kept on the GPU are those that
// deinterlaced a quarter of a second there: a filter FFmpeg lists may
// still fail, or crash, on a GPU. It logs what it timed.
func (m *Manager) measure(ctx context.Context, hw Hardware) Hardware {
	i := slices.IndexFunc(hardwareMethods, func(c gpuMethod) bool { return c.method == hw.Method })
	if i < 0 || len(hw.Encoders) == 0 {
		return hw
	}
	method := hardwareMethods[i]
	dir, err := os.MkdirTemp(m.dir, "measure-")
	if err != nil {
		m.logger.Warn("The GPU's conversions could not be timed", "error", err)
		return hw
	}
	defer os.RemoveAll(dir)
	pattern := []string{"-f", "lavfi", "-i", "testsrc2=size=3840x2160:rate=24", "-t", strconv.FormatFloat(sampleLength.Seconds(), 'f', -1, 64)}
	// The encoder converting the samples: H.264 on the GPU, as the
	// conversions most apps get.
	encoder := hw.Encoders[0]
	for _, e := range hw.Encoders {
		if strings.HasPrefix(e, "h264_") {
			encoder = e
		}
	}
	if m.HasFilters(method.filters...) {
		sdr := filepath.Join(dir, "sdr.mkv")
		var encode []string
		switch {
		case strings.HasPrefix(encoder, "h264_") && hw.Method == "vaapi":
			encode = slices.Concat(hw.devices(), pattern, []string{"-vf", "format=nv12,hwupload", "-c:v", encoder})
		case strings.HasPrefix(encoder, "h264_"):
			encode = slices.Concat(pattern, []string{"-c:v", encoder})
		case slices.Contains(m.can.encoders, "libx264"):
			encode = slices.Concat(pattern, []string{"-c:v", "libx264", "-preset", "ultrafast"})
		}
		if encode != nil {
			if _, err := m.timed(ctx, slices.Concat(probeStart, encode, []string{sdr})); err == nil {
				memory, gpu := hw, hw
				gpu.Resident = true
				t := m.timeChains(ctx, sdr, encoder, VideoEncoding{Hardware: &memory}, VideoEncoding{Hardware: &gpu})
				hw.Resident = t.gpuWins()
				m.logChoice("SDR", hw.Resident, t)
			}
		}
	}
	if hw.Resident || hw.ToneMapping && hw.Method != "cuda" {
		for _, name := range method.deinterlacers {
			if m.HasFilters(name) && m.deinterlaces(ctx, hw, name) {
				hw.Deinterlacers = append(hw.Deinterlacers, name)
			}
		}
	}
	// Only libplacebo, on NVIDIA's GPUs, takes Vulkan frames.
	if !hw.ToneMapping || hw.Method != "cuda" || ctx.Err() != nil {
		return hw
	}
	// The HDR sample is HEVC in 10 bits, as HDR files are, from the GPU's
	// encoder or else x265's.
	hdr := filepath.Join(dir, "hdr.mkv")
	var encode []string
	switch {
	case slices.Contains(hw.Encoders, "hevc_nvenc"):
		encode = []string{"-vf", "format=p010le," + hdrTags, "-c:v", "hevc_nvenc", "-profile:v", "main10"}
	case slices.Contains(m.can.encoders, "libx265"):
		encode = []string{"-vf", "format=yuv420p10le," + hdrTags, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error"}
	default:
		return hw
	}
	if _, err := m.timed(ctx, slices.Concat(probeStart, pattern, encode, []string{hdr})); err != nil {
		return hw
	}
	memory, gpu := hw, hw
	gpu.VulkanDecoding = true
	// The frames must come from the GPU's decoder: FFmpeg would decode in
	// software, unseen, a codec Vulkan does not, and libplacebo take the
	// frames from memory. Only Vulkan frames can be downloaded.
	check := VideoEncoding{ToneMap: true, Hardware: &gpu}
	if _, err := m.timed(ctx, slices.Concat(probeStart, check.inputs(), []string{"-i", hdr, "-vf", "hwdownload,format=p010le", "-f", "null", "-"})); err != nil {
		m.logChoice("HDR", false, timing{gpuErr: err})
		return hw
	}
	t := m.timeChains(ctx, hdr, encoder, VideoEncoding{ToneMap: true, Hardware: &memory}, VideoEncoding{ToneMap: true, Hardware: &gpu})
	hw.VulkanDecoding = t.gpuWins()
	m.logChoice("HDR", hw.VulkanDecoding, t)
	return hw
}

// timeChains times the conversion of sample to 1080p by encoder, through
// memory then on the GPU, as the two encodings say.
func (m *Manager) timeChains(ctx context.Context, sample, encoder string, memory, gpu VideoEncoding) timing {
	var t timing
	for _, run := range []struct {
		v    VideoEncoding
		took *time.Duration
		err  *error
	}{{memory, &t.memory, &t.memoryErr}, {gpu, &t.gpu, &t.gpuErr}} {
		run.v.Encoder, run.v.Width, run.v.Height = encoder, 1920, 1080
		*run.took, *run.err = m.timed(ctx, slices.Concat(probeStart, run.v.inputs(),
			[]string{"-i", sample, "-vf", run.v.filters(), "-c:v", encoder, "-f", "null", "-"}))
	}
	return t
}

// logChoice logs the chain chosen for a kind of conversion, and the speed
// of each.
func (m *Manager) logChoice(kind string, gpu bool, t timing) {
	chain := "memory"
	if gpu {
		chain = "gpu"
	}
	m.logger.Info("Timed the GPU's conversion chains", "kind", kind, "chain", chain,
		"memory_speed", speed(t.memory, t.memoryErr), "gpu_speed", speed(t.gpu, t.gpuErr))
}

// probeStart opens FFmpeg's command lines testing the GPU.
var probeStart = []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}

// timed runs FFmpeg with args, within 20 s, and reports how long it took,
// or why it failed with the end of what FFmpeg wrote, through m.run when
// tests set it.
func (m *Manager) timed(ctx context.Context, args []string) (time.Duration, error) {
	if m.run != nil {
		return m.run(ctx, args)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	stderr := &tail{limit: 4096}
	cmd.Stderr = stderr
	started := time.Now()
	err := cmd.Run()
	if err != nil {
		err = fmt.Errorf("FFmpeg failed: %w: %s", err, bytes.TrimSpace(stderr.bytes()))
	}
	return time.Since(started), err
}

// succeeds reports whether FFmpeg ends well with args within 20 s. A GPU
// or driver that fails makes FFmpeg fail, or abort.
func (m *Manager) succeeds(args ...string) bool {
	_, err := m.timed(context.Background(), args)
	return err == nil
}

// toneMappingDevices open the NVIDIA GPU for CUDA and its Vulkan device
// derived from it, for filters: libplacebo runs on the GPU that decodes and
// encodes, whichever other GPUs Vulkan sees.
var toneMappingDevices = []string{"-init_hw_device", "cuda=cu", "-init_hw_device", "vulkan=vk@cu", "-filter_hw_device", "vk"}

// Hardware is the GPU DetectHardware chose, nil for none.
func (m *Manager) Hardware() *Hardware {
	return m.hardware.Load()
}

// renderNodes are the render nodes VAAPI may open: device if set, else
// those the system has, in order.
func renderNodes(device string) []string {
	if device != "" {
		return []string{device}
	}
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	slices.Sort(nodes)
	return nodes
}

// encodes encodes a quarter of a second of test pattern with encoder on
// hw, with options, and tells why it failed, nil when it did not.
func (m *Manager) encodes(hw Hardware, encoder string, options ...string) error {
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, hw.devices()...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25")
	if hw.Method == "vaapi" {
		args = append(args, "-vf", "format=nv12,hwupload")
	}
	args = append(args, "-c:v", encoder)
	args = append(args, options...)
	args = append(args, "-f", "null", "-")
	_, err := m.timed(context.Background(), args)
	return err
}

// deinterlaces reports whether the deinterlacer name deinterlaces a
// quarter of a second of test pattern uploaded to hw, as frames kept on
// the GPU are.
func (m *Manager) deinterlaces(ctx context.Context, hw Hardware, name string) bool {
	hw.Deinterlacers = []string{name}
	// yadif_cuda is yadif's counterpart, and so on (see gpuDeinterlacer).
	deinterlacer, _, _ := strings.Cut(name, "_")
	v := VideoEncoding{Deinterlace: true, Deinterlacer: deinterlacer, Hardware: &hw}
	filters := slices.Concat([]string{"format=nv12", "hwupload"}, hw.mapping(), []string{v.gpuDeinterlacer(), "hwdownload", "format=nv12"})
	_, err := m.timed(ctx, slices.Concat(probeStart, hw.residentDevices(), []string{"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25",
		"-vf", strings.Join(filters, ","), "-f", "null", "-"}))
	return err == nil
}

// devices are FFmpeg's options opening the GPU for filters: VAAPI's
// render node, which the upload before the encoder needs, VideoToolbox,
// or the Direct3D 11 adapter of Intel or AMD, which QSV and AMF derive
// their devices from. NVENC opens the NVIDIA GPU itself.
func (hw Hardware) devices() []string {
	switch hw.Method {
	case "vaapi":
		return []string{"-init_hw_device", "vaapi=va:" + hw.Device, "-filter_hw_device", "va"}
	case "videotoolbox":
		return []string{"-init_hw_device", "videotoolbox=vt", "-filter_hw_device", "vt"}
	case "qsv":
		return []string{"-init_hw_device", "d3d11va=dx:,vendor_id=" + intelVendor, "-filter_hw_device", "dx"}
	case "amf":
		return []string{"-init_hw_device", "d3d11va=dx:,vendor_id=" + amdVendor, "-filter_hw_device", "dx"}
	}
	return nil
}

// residentDevices are FFmpeg's options opening the GPU for frames kept in
// its memory: the decoder and the upload of frames it left in memory use
// it.
func (hw Hardware) residentDevices() []string {
	if hw.Method == "cuda" {
		return []string{"-init_hw_device", "cuda=cu", "-filter_hw_device", "cu"}
	}
	return hw.devices()
}

// decoder is how FFmpeg names the GPU's decoding, the device it decodes
// on (see residentDevices) and the frames it decodes into: Intel's and
// AMD's GPUs decode through Direct3D 11 on Windows.
func (hw *Hardware) decoder() (hwaccel, device, frames string) {
	switch hw.Method {
	case "vaapi":
		return "vaapi", "va", "vaapi"
	case "videotoolbox":
		return "videotoolbox", "vt", "videotoolbox_vld"
	case "qsv", "amf":
		return "d3d11va", "dx", "d3d11"
	}
	return "cuda", "cu", "cuda"
}

// residentInputs are FFmpeg's input options decoding into the GPU's
// memory, for the filters there.
func (hw *Hardware) residentInputs() []string {
	hwaccel, device, frames := hw.decoder()
	return append(hw.residentDevices(), "-hwaccel", hwaccel, "-hwaccel_device", device, "-hwaccel_output_format", frames)
}

// mapping are the filters taking frames uploaded to Direct3D 11 to QSV,
// whose filters take only its own: none for other methods.
func (hw *Hardware) mapping() []string {
	if hw.Method == "qsv" {
		return []string{"hwmap=derive_device=qsv", "format=qsv"}
	}
	return nil
}

// pipeline is the way a conversion's frames go from the decoder to the
// encoder.
type pipeline int

const (
	// throughMemory decodes into memory, or on the processor, and filters
	// there, the encoder uploading the frames again.
	throughMemory pipeline = iota
	// onGPU decodes into the GPU's memory and scales and deinterlaces
	// there, for the encoder to take as they are.
	onGPU
	// throughVulkan decodes into Vulkan frames, which libplacebo scales
	// and tone maps as they are, into memory for the encoder.
	throughVulkan
	// throughDevice keeps HDR frames in the memory of Intel's GPU or of
	// VideoToolbox, which deinterlaces, scales and tone maps them there
	// (see deviceToneMapping).
	throughDevice
)

// pipeline is the way the conversion's frames go: kept on the GPU when it
// can do every filter the conversion needs, through memory for burned
// subtitles, for decoding left to the processor, and for tone mapping on
// the processor. The tone mapping of Intel's GPUs and of VideoToolbox
// keeps them on the GPU whatever else the conversion needs.
func (v *VideoEncoding) pipeline() pipeline {
	hw := v.Hardware
	switch {
	case hw == nil:
		return throughMemory
	case v.toneMapsOnGPU() && hw.Method != "cuda":
		return throughDevice
	case v.DecodeOnCPU || v.Burns():
		return throughMemory
	case v.toneMapsOnGPU():
		if hw.VulkanDecoding && !v.Deinterlace {
			return throughVulkan
		}
		return throughMemory
	case v.ToneMap || !hw.Resident || v.Deinterlace && v.gpuDeinterlacer() == "":
		return throughMemory
	}
	return onGPU
}

// inputs are FFmpeg's input options of a conversion on a GPU, decoding as
// its pipeline says: into the GPU's memory, into Vulkan frames, or into
// memory unless DecodeOnCPU, and, to tone map on the GPU, the devices
// libplacebo needs.
func (v *VideoEncoding) inputs() []string {
	hw := v.Hardware
	switch v.pipeline() {
	case onGPU:
		return hw.residentInputs()
	case throughVulkan:
		return append(slices.Clone(toneMappingDevices), "-hwaccel", "vulkan", "-hwaccel_device", "vk", "-hwaccel_output_format", "vulkan")
	case throughDevice:
		switch {
		case v.DecodeOnCPU:
			return hw.devices()
		case v.deviceFramesOnGPU():
			return hw.residentInputs()
		}
		hwaccel, device, _ := hw.decoder()
		return append(hw.devices(), "-hwaccel", hwaccel, "-hwaccel_device", device)
	}
	if hw == nil {
		return nil
	}
	switch {
	case hw.Method == "cuda" && v.toneMapsOnGPU() && v.DecodeOnCPU:
		return slices.Clone(toneMappingDevices)
	case hw.Method == "cuda" && v.toneMapsOnGPU():
		return append(slices.Clone(toneMappingDevices), "-hwaccel", "cuda", "-hwaccel_device", "cu")
	case hw.Method == "vaapi" && v.DecodeOnCPU:
		// The upload before the encoder opens the render node.
		return hw.devices()
	case v.DecodeOnCPU:
		return nil
	}
	return hw.DecodeInputs()
}

// deviceToneMapping is the filter chain of HDR tone mapped by the video
// processing of Intel's GPU or of VideoToolbox, as Jellyfin's on Intel's:
// frames the GPU decoded go through hwupload as they are, as would those
// FFmpeg's own decoder takes over for a codec the GPU does not decode,
// while frames decoded on the processor, or deinterlaced there when FFmpeg
// has no deinterlacer on the GPU, are uploaded in 10 bits. They are then
// deinterlaced, scaled to the size asked, and tone mapped to BT.709 there,
// by scale_vaapi and tonemap_vaapi, vpp_qsv, or scale_vt, for the encoder
// to take as they are. tonemap_vaapi and vpp_qsv read the HDR10 mastering
// display the frames carry: tonemap_vaapi fails without it, and vpp_qsv
// would not tone map.
func (v *VideoEncoding) deviceToneMapping() string {
	var filters []string
	if v.deviceFramesOnGPU() {
		filters = append(filters, "hwupload")
		filters = append(filters, v.Hardware.mapping()...)
		if v.Deinterlace {
			filters = append(filters, v.gpuDeinterlacer())
		}
	} else {
		if v.Deinterlace {
			filters = append(filters, v.deinterlacer())
		}
		filters = append(filters, "format=p010le", "hwupload")
		filters = append(filters, v.Hardware.mapping()...)
	}
	size := "w=" + strconv.Itoa(v.Width) + ":h=" + strconv.Itoa(v.Height)
	switch v.Hardware.Method {
	case "videotoolbox":
		filters = append(filters, "scale_vt="+size+":color_matrix=bt709:color_primaries=bt709:color_transfer=bt709")
	case "qsv":
		filters = append(filters, "vpp_qsv="+size+":format=nv12:tonemap=1:out_color_matrix=bt709:out_color_primaries=bt709:out_color_transfer=bt709")
	default:
		filters = append(filters, "scale_vaapi="+size+":format=p010", "tonemap_vaapi=format=nv12:p=bt709:t=bt709:m=bt709")
	}
	return strings.Join(filters, ",")
}

// deviceFramesOnGPU reports whether the frames the GPU tone maps (see
// deviceToneMapping) come from its decoder in its memory: not when the
// processor decodes, nor when it deinterlaces for want of a deinterlacer
// on the GPU.
func (v *VideoEncoding) deviceFramesOnGPU() bool {
	return !v.DecodeOnCPU && (!v.Deinterlace || v.gpuDeinterlacer() != "")
}

// gpuFilters is the filter chain of video kept in the GPU's memory: frames
// the decoder left in memory, for a codec the GPU does not decode, are
// uploaded, those in the GPU's memory go through as they are, and with
// QSV are mapped to its frames; then deinterlaced and scaled to 8-bit
// 4:2:0 there. VideoToolbox's encoders take frames in 10 bits as well.
func (v *VideoEncoding) gpuFilters() string {
	filters := append([]string{"hwupload"}, v.Hardware.mapping()...)
	if v.Deinterlace {
		filters = append(filters, v.gpuDeinterlacer())
	}
	size := "w=" + strconv.Itoa(v.Width) + ":h=" + strconv.Itoa(v.Height)
	switch v.Hardware.Method {
	case "vaapi":
		filters = append(filters, "scale_vaapi="+size+":format=nv12")
	case "videotoolbox":
		filters = append(filters, "scale_vt="+size)
	case "qsv":
		filters = append(filters, "scale_qsv="+size+":format=nv12")
	case "amf":
		filters = append(filters, "vpp_amf="+size+":format=nv12")
	default:
		// Bicubic, as FFmpeg's scale filter in memory.
		filters = append(filters, "scale_cuda="+size+":interp_algo=bicubic:format=nv12")
	}
	return strings.Join(filters, ",")
}

// qsvDeinterlacer is QSV's motion adaptive deinterlacer, making a frame of
// each frame; with rate=field, one of each field.
const qsvDeinterlacer = "vpp_qsv=deinterlace=advanced"

// gpuDeinterlacer is the deinterlacing filter of frames kept in the GPU's
// memory, empty when FFmpeg has none: the CUDA or VideoToolbox counterpart
// of the chosen one, or VAAPI's or QSV's own, which picks its best method.
// AMF has none.
func (v *VideoEncoding) gpuDeinterlacer() string {
	hw := v.Hardware
	switch hw.Method {
	case "vaapi", "qsv":
		name, filter := "deinterlace_vaapi", "deinterlace_vaapi"
		if hw.Method == "qsv" {
			name, filter = "vpp_qsv", qsvDeinterlacer
		}
		switch {
		case !slices.Contains(hw.Deinterlacers, name):
			return ""
		case v.DoubleRate && hw.Method == "qsv":
			return filter + ":rate=field"
		case v.DoubleRate:
			return filter + "=rate=field"
		}
		return filter
	case "amf":
		return ""
	}
	suffix := "_cuda"
	if hw.Method == "videotoolbox" {
		suffix = "_videotoolbox"
	}
	name, options, _ := strings.Cut(v.deinterlacer(), "=")
	if !slices.Contains(hw.Deinterlacers, name+suffix) {
		return ""
	}
	if options != "" {
		return name + suffix + "=" + options
	}
	return name + suffix
}

// toneMapsOnGPU reports whether the conversion tone maps HDR on its GPU.
func (v *VideoEncoding) toneMapsOnGPU() bool {
	return v.ToneMap && !v.ToneMapOnCPU && v.Hardware != nil && v.Hardware.ToneMapping
}

// output ends a filter chain in memory: frames as the encoder takes them,
// uploaded to the GPU for VAAPI, in NV12 for QSV and AMF, which upload
// them themselves; NVENC, VideoToolbox and the software encoders take
// them from memory in 4:2:0.
func (hw *Hardware) output() string {
	switch {
	case hw == nil:
	case hw.Method == "vaapi":
		return "format=nv12,hwupload"
	case hw.Method == "qsv" || hw.Method == "amf":
		return "format=nv12"
	}
	return "format=yuv420p"
}

// DecodeInputs are FFmpeg's input options decoding on hw, nil for none,
// into memory, for work that only decodes, such as thumbnails: a codec the
// GPU cannot decode is left to FFmpeg's own decoder. VAAPI and Direct3D
// 11 decode on the device devices opens.
func (hw *Hardware) DecodeInputs() []string {
	if hw == nil {
		return nil
	}
	hwaccel, device, _ := hw.decoder()
	switch hw.Method {
	case "vaapi", "qsv", "amf":
		return append(hw.devices(), "-hwaccel", hwaccel, "-hwaccel_device", device)
	}
	return []string{"-hwaccel", hwaccel}
}

// NeedsMasteringDisplay reports whether hw tone maps only HDR10 whose
// frames carry their mastering display: Intel's GPUs (see
// deviceToneMapping).
func (hw *Hardware) NeedsMasteringDisplay() bool {
	return hw != nil && (hw.Method == "vaapi" || hw.Method == "qsv")
}
