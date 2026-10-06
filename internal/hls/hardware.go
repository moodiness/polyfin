package hls

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Hardware is a GPU FFmpeg decodes and encodes video on. Decoded frames
// either stay in the GPU's memory until they are encoded, scaled there in
// SDR (see Resident) and tone mapped there from Vulkan frames (see
// VulkanDecoding), or come back to memory for the filters and go to the
// GPU again to be encoded: Polyfin times both at startup and keeps the
// faster for each kind of conversion. Burned subtitles, tone mapping on
// the processor and decoding left to the processor always go through
// memory. A GPU that cannot decode a codec leaves it to FFmpeg's own
// decoder, and the conversion looks the same either way.
type Hardware struct {
	// Method is cuda, for NVIDIA GPUs, or vaapi, for AMD and Intel ones.
	Method string
	// Device is the render node VAAPI opens.
	Device string
	// Encoders are those that encoded on it at startup, such as h264_nvenc.
	Encoders []string
	// ToneMapping is set when the GPU also scales video and tone maps HDR
	// to SDR, through libplacebo on its Vulkan driver, which applies Dolby
	// Vision's metadata: NVIDIA GPUs only. On AMD GPUs, the frames libplacebo
	// imports from memory set off a fault in the Linux driver.
	ToneMapping bool
	// QVBR is set when a VAAPI driver takes a quality factor within a
	// maximum rate, which a quality set in the settings needs there: AMD's
	// drivers often lack it.
	QVBR bool
	// Resident is set when SDR frames the GPU decoded stay in its memory to
	// be scaled, by scale_cuda or scale_vaapi, and encoded, which worked and
	// was faster than going through memory at startup. Deinterlacers are
	// the deinterlacing filters FFmpeg has for such frames: yadif_cuda and
	// bwdif_cuda, or deinterlace_vaapi.
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
// encoders Polyfin uses, the filter scaling frames in the GPU's memory and
// the deinterlacers taking them.
type gpuMethod struct {
	name, method  string
	encoders      []string
	scaler        string
	deinterlacers []string
}

// hardwareMethods are the GPU methods by preference.
var hardwareMethods = []gpuMethod{
	{"nvenc", "cuda", []string{"h264_nvenc", "hevc_nvenc"}, "scale_cuda", []string{"yadif_cuda", "bwdif_cuda"}},
	{"vaapi", "vaapi", []string{"h264_vaapi", "hevc_vaapi"}, "scale_vaapi", []string{"deinterlace_vaapi"}},
}

// DetectHardware chooses the GPU video is converted on, by encoding a few
// frames with each encoder: with want auto, the first of NVIDIA and VAAPI
// that encodes, VAAPI on device or else on each render node in turn; with
// nvenc or vaapi, that one only; with none, none. Each want is detected
// once: choosing it again switches to what was found, encodings already
// running going on as they started. It reports false when no GPU encodes.
// The GPU's conversion chains are then timed in the background (see
// measure): conversions starting before that ends go through memory.
func (m *Manager) DetectHardware(want, device string) (Hardware, bool) {
	m.detecting.Lock()
	defer m.detecting.Unlock()
	hw, done := m.detected[want]
	if !done {
		hw = m.detect(want, device)
		m.detected[want] = hw
	}
	m.hardware.Store(hw)
	if !done {
		m.measureLater(want, hw)
	}
	if hw == nil {
		return Hardware{}, false
	}
	return *hw, true
}

// SelectHardware chooses the GPU as DetectHardware does, and logs the
// outcome.
func (m *Manager) SelectHardware(want, device string) {
	switch hw, ok := m.DetectHardware(want, device); {
	case ok:
		m.logger.Info("Video is converted on the GPU", "method", hw.Method, "device", hw.Device, "encoders", hw.Encoders,
			"tone_mapping", hw.ToneMapping, "qvbr", hw.QVBR)
	case want == "auto":
		m.logger.Info("Video is converted in software: no GPU encodes")
	case want != "none":
		m.logger.Warn("Video is converted in software: the GPU asked for does not encode", "hwaccel", want)
	}
}

// detect finds the GPU want asks for (see DetectHardware), nil for none.
func (m *Manager) detect(want, device string) *Hardware {
	for _, candidate := range hardwareMethods {
		if want != "auto" && want != candidate.name {
			continue
		}
		devices := []string{""}
		if candidate.method == "vaapi" {
			devices = renderNodes(device)
		}
		for _, node := range devices {
			hw := Hardware{Method: candidate.method, Device: node}
			for _, encoder := range candidate.encoders {
				if slices.Contains(m.can.encoders, encoder) && m.encodes(hw, encoder) {
					hw.Encoders = append(hw.Encoders, encoder)
				}
			}
			if len(hw.Encoders) > 0 {
				hw.ToneMapping = hw.Method == "cuda" && slices.Contains(m.can.filters, "libplacebo") && m.toneMaps(hw)
				hw.QVBR = hw.Method == "vaapi" && m.encodes(hw, hw.Encoders[0], "-rc_mode", "QVBR", "-global_quality", "25", "-b:v", "1000000", "-maxrate", "1500000")
				return &hw
			}
		}
	}
	return nil
}

// toneMaps reports whether hw tone maps a quarter of a second of test
// pattern, tagged as HDR10, on its Vulkan device: NVIDIA's Vulkan driver
// may be missing from the container, or fail.
func (m *Manager) toneMaps(hw Hardware) bool {
	hw.ToneMapping = true
	v := VideoEncoding{Width: 160, Height: 90, ToneMap: true, Hardware: &hw}
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, toneMappingDevices...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25",
		"-vf", "format=yuv420p10le,"+hdrTags+","+v.filters(),
		"-c:v", hw.Encoders[0], "-f", "null", "-")
	return m.succeeds(args...)
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
	go func() {
		defer m.measuring.Done()
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
// by scale_cuda or scale_vaapi (Resident), and on a GPU that tone maps,
// HDR10 HEVC in 10 bits decoded into Vulkan frames for libplacebo
// (VulkanDecoding), once Vulkan proved to decode it. Every failure leaves
// frames going through memory. It logs what it timed.
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
	if m.HasFilters(method.scaler, "hwupload") {
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
	if hw.Resident {
		for _, name := range method.deinterlacers {
			if m.HasFilters(name) {
				hw.Deinterlacers = append(hw.Deinterlacers, name)
			}
		}
	}
	if !hw.ToneMapping || ctx.Err() != nil {
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
// through m.run when tests set it.
func (m *Manager) timed(ctx context.Context, args []string) (time.Duration, error) {
	if m.run != nil {
		return m.run(ctx, args)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	started := time.Now()
	err := exec.CommandContext(ctx, m.ffmpeg, args...).Run()
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

// encodes reports whether encoder encodes a quarter of a second of test
// pattern on hw, with options.
func (m *Manager) encodes(hw Hardware, encoder string, options ...string) bool {
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error"}, hw.devices()...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24", "-t", "0.25")
	if hw.Method == "vaapi" {
		args = append(args, "-vf", "format=nv12,hwupload")
	}
	args = append(args, "-c:v", encoder)
	args = append(args, options...)
	args = append(args, "-f", "null", "-")
	return m.succeeds(args...)
}

// devices are FFmpeg's options opening the GPU for filters: VAAPI's
// render node, which the upload before the encoder needs. NVENC opens
// the NVIDIA GPU itself.
func (hw Hardware) devices() []string {
	if hw.Method == "vaapi" {
		return []string{"-init_hw_device", "vaapi=va:" + hw.Device, "-filter_hw_device", "va"}
	}
	return nil
}

// residentDevices are FFmpeg's options opening the GPU for frames kept in
// its memory: the decoder and the upload of frames it left in memory use
// it.
func (hw Hardware) residentDevices() []string {
	if hw.Method == "vaapi" {
		return hw.devices()
	}
	return []string{"-init_hw_device", "cuda=cu", "-filter_hw_device", "cu"}
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
)

// pipeline is the way the conversion's frames go: kept on the GPU when it
// can do every filter the conversion needs, through memory for burned
// subtitles, for decoding left to the processor, and for tone mapping on
// the processor.
func (v *VideoEncoding) pipeline() pipeline {
	hw := v.Hardware
	switch {
	case hw == nil || v.DecodeOnCPU || v.Burn != nil:
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
		name := map[string]string{"cuda": "cu", "vaapi": "va"}[hw.Method]
		return append(hw.residentDevices(), "-hwaccel", hw.Method, "-hwaccel_device", name, "-hwaccel_output_format", hw.Method)
	case throughVulkan:
		return append(slices.Clone(toneMappingDevices), "-hwaccel", "vulkan", "-hwaccel_device", "vk", "-hwaccel_output_format", "vulkan")
	}
	var devices []string
	switch {
	case hw == nil:
		return nil
	case hw.Method == "vaapi":
		devices = hw.devices()
	case v.toneMapsOnGPU():
		devices = slices.Clone(toneMappingDevices)
	}
	if v.DecodeOnCPU {
		return devices
	}
	switch {
	case hw.Method == "vaapi":
		return append(devices, "-hwaccel", "vaapi", "-hwaccel_device", "va")
	case v.toneMapsOnGPU():
		return append(devices, "-hwaccel", "cuda", "-hwaccel_device", "cu")
	}
	return []string{"-hwaccel", "cuda"}
}

// gpuFilters is the filter chain of video kept in the GPU's memory: frames
// the decoder left in memory, for a codec the GPU does not decode, are
// uploaded, those in the GPU's memory go through as they are; then
// deinterlaced and scaled to 8-bit 4:2:0 there.
func (v *VideoEncoding) gpuFilters() string {
	filters := []string{"hwupload"}
	if v.Deinterlace {
		filters = append(filters, v.gpuDeinterlacer())
	}
	size := "w=" + strconv.Itoa(v.Width) + ":h=" + strconv.Itoa(v.Height)
	if v.Hardware.Method == "vaapi" {
		filters = append(filters, "scale_vaapi="+size+":format=nv12")
	} else {
		// Bicubic, as FFmpeg's scale filter in memory.
		filters = append(filters, "scale_cuda="+size+":interp_algo=bicubic:format=nv12")
	}
	return strings.Join(filters, ",")
}

// gpuDeinterlacer is the deinterlacing filter of frames kept in the GPU's
// memory, empty when FFmpeg has none: the CUDA counterpart of the chosen
// one, or VAAPI's own, which picks its best method.
func (v *VideoEncoding) gpuDeinterlacer() string {
	hw := v.Hardware
	if hw.Method == "vaapi" {
		if !slices.Contains(hw.Deinterlacers, "deinterlace_vaapi") {
			return ""
		}
		if v.DoubleRate {
			return "deinterlace_vaapi=rate=field"
		}
		return "deinterlace_vaapi"
	}
	name, options, _ := strings.Cut(v.deinterlacer(), "=")
	if !slices.Contains(hw.Deinterlacers, name+"_cuda") {
		return ""
	}
	if options != "" {
		return name + "_cuda=" + options
	}
	return name + "_cuda"
}

// toneMapsOnGPU reports whether the conversion tone maps HDR on its GPU.
func (v *VideoEncoding) toneMapsOnGPU() bool {
	return v.ToneMap && v.Hardware != nil && v.Hardware.ToneMapping
}

// output ends a filter chain in memory: frames as the encoder takes them,
// uploaded to the GPU for VAAPI; NVENC and the software encoders take
// them from memory.
func (hw *Hardware) output() string {
	if hw != nil && hw.Method == "vaapi" {
		return "format=nv12,hwupload"
	}
	return "format=yuv420p"
}

// DecodeInputs are FFmpeg's input options decoding on hw, nil for none,
// into memory, for work that only decodes, such as thumbnails: a codec the
// GPU cannot decode is left to FFmpeg's own decoder.
func (hw *Hardware) DecodeInputs() []string {
	switch {
	case hw == nil:
		return nil
	case hw.Method == "vaapi":
		return append(hw.devices(), "-hwaccel", "vaapi", "-hwaccel_device", "va")
	}
	return []string{"-hwaccel", "cuda"}
}
