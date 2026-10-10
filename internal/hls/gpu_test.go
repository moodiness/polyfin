package hls

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// inputsAndFilters splits a conversion's command line into its options
// before -copyts or -i, past the logging and progress ones, and its video
// filters, from -vf or -filter_complex.
func inputsAndFilters(t *testing.T, args []string) ([]string, string) {
	t.Helper()
	from := slices.Index(args, "-progress") + 2
	to := slices.IndexFunc(args, func(arg string) bool { return arg == "-copyts" || arg == "-i" })
	if from < 2 || to < from {
		t.Fatalf("no inputs in %q", args)
	}
	inputs := slices.Clone(args[from:to])
	if i := slices.Index(inputs, "-re"); i >= 0 {
		inputs = slices.Delete(inputs, i, i+1)
	}
	for _, option := range []string{"-vf", "-filter_complex"} {
		if i := slices.Index(args, option); i >= 0 {
			return inputs, args[i+1]
		}
	}
	return inputs, ""
}

// Frames stay on the GPU from the decoder to the encoder whenever every
// filter the conversion needs runs there; burned subtitles, decoding on
// the processor, tone mapping on the processor, and a GPU without the
// filters keep the frames going through memory. Every GPU, tone mapping
// and burning combination, files and live alike.
func TestGPUPipelines(t *testing.T) {
	plan := NewPlan(seconds(0, 6, 12), 18*time.Second)
	nvidiaInMemory := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}}
	nvidiaResident := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, Resident: true,
		Deinterlacers: []string{"yadif_cuda", "bwdif_cuda"}}
	nvidiaNoDeinterlacer := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, Resident: true}
	nvidiaToneMapping := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc"}, ToneMapping: true, Resident: true,
		Deinterlacers: []string{"yadif_cuda", "bwdif_cuda"}}
	nvidiaFull := &Hardware{Method: "cuda", Encoders: []string{"h264_nvenc", "hevc_nvenc"}, ToneMapping: true, Resident: true,
		Deinterlacers: []string{"yadif_cuda", "bwdif_cuda"}, VulkanDecoding: true}
	vaapiInMemory := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}}
	vaapiResident := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, Resident: true,
		Deinterlacers: []string{"deinterlace_vaapi"}}
	intel := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi", "hevc_vaapi"}, ToneMapping: true,
		Deinterlacers: []string{"deinterlace_vaapi"}}
	intelNoDeinterlacer := &Hardware{Method: "vaapi", Device: "/dev/dri/renderD128", Encoders: []string{"h264_vaapi"}, ToneMapping: true}

	cudaResident := []string{"-init_hw_device", "cuda=cu", "-filter_hw_device", "cu", "-hwaccel", "cuda", "-hwaccel_device", "cu", "-hwaccel_output_format", "cuda"}
	cudaInMemory := []string{"-hwaccel", "cuda"}
	vulkanDevices := []string{"-init_hw_device", "cuda=cu", "-init_hw_device", "vulkan=vk@cu", "-filter_hw_device", "vk"}
	vulkanDecoding := append(slices.Clone(vulkanDevices), "-hwaccel", "vulkan", "-hwaccel_device", "vk", "-hwaccel_output_format", "vulkan")
	cudaToneMapping := append(slices.Clone(vulkanDevices), "-hwaccel", "cuda", "-hwaccel_device", "cu")
	vaapiDevices := []string{"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va"}
	vaapiResidentInputs := append(slices.Clone(vaapiDevices), "-hwaccel", "vaapi", "-hwaccel_device", "va", "-hwaccel_output_format", "vaapi")
	vaapiInMemoryInputs := append(slices.Clone(vaapiDevices), "-hwaccel", "vaapi", "-hwaccel_device", "va")

	const (
		scaleCUDA    = "scale_cuda=w=1920:h=1080:interp_algo=bicubic:format=nv12"
		scaleVAAPI   = "scale_vaapi=w=1920:h=1080:format=nv12"
		placebo      = "libplacebo=w=1920:h=1080:format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping=bt.2390"
		cpuToneMap   = "scale=w=1920:h=1080,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv"
		burn         = "[0:3]fps=24,scale=1920:-2[subtitle];[converted][subtitle]overlay=x=0:y=main_h-overlay_h:eof_action=pass,"
		scale10      = "scale_vaapi=w=1920:h=1080:format=p010"
		tonemapVAAPI = "tonemap_vaapi=format=nv12:p=bt709:t=bt709:m=bt709"
	)
	burned := 3
	encoding := func(hw *Hardware, change func(*VideoEncoding)) *VideoEncoding {
		e := &VideoEncoding{Encoder: "h264_nvenc", Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 24, Hardware: hw}
		if hw.Method == "vaapi" {
			e.Encoder = "h264_vaapi"
		}
		if change != nil {
			change(e)
		}
		return e
	}
	sdr := func(*VideoEncoding) {}
	hdr := func(e *VideoEncoding) { e.ToneMap = true }
	burnIn := func(e *VideoEncoding) { e.Burn = &burned }
	hdrBurnIn := func(e *VideoEncoding) { e.ToneMap, e.Burn = true, &burned }
	cpuDecoding := func(e *VideoEncoding) { e.DecodeOnCPU = true }
	hdrCPUDecoding := func(e *VideoEncoding) { e.ToneMap, e.DecodeOnCPU = true, true }
	yadif := func(e *VideoEncoding) { e.Deinterlace = true }
	yadifDoubled := func(e *VideoEncoding) { e.Deinterlace, e.DoubleRate = true, true }
	bwdif := func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer = true, "bwdif" }
	bwdifDoubled := func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer, e.DoubleRate = true, "bwdif", true }
	hdrInterlaced := func(e *VideoEncoding) { e.ToneMap, e.Deinterlace = true, true }
	cpuToneMapping := func(e *VideoEncoding) { e.ToneMap, e.ToneMapOnCPU = true, true }

	for _, tc := range []struct {
		name    string
		e       *VideoEncoding
		inputs  []string
		filters string
	}{
		// NVIDIA, SDR: decoded into CUDA frames, scaled by scale_cuda.
		{"NVIDIA SDR", encoding(nvidiaFull, sdr), cudaResident, "hwupload," + scaleCUDA},
		{"NVIDIA SDR, yadif", encoding(nvidiaFull, yadif), cudaResident, "hwupload,yadif_cuda," + scaleCUDA},
		{"NVIDIA SDR, yadif doubled", encoding(nvidiaFull, yadifDoubled), cudaResident, "hwupload,yadif_cuda=1," + scaleCUDA},
		{"NVIDIA SDR, bwdif", encoding(nvidiaFull, bwdif), cudaResident, "hwupload,bwdif_cuda=0," + scaleCUDA},
		{"NVIDIA SDR, bwdif doubled", encoding(nvidiaFull, bwdifDoubled), cudaResident, "hwupload,bwdif_cuda=1," + scaleCUDA},
		{"NVIDIA SDR without tone mapping", encoding(nvidiaResident, sdr), cudaResident, "hwupload," + scaleCUDA},
		// NVIDIA, HDR: decoded into Vulkan frames for libplacebo, or into
		// memory when Vulkan does not decode, or for the processor's
		// deinterlacer.
		{"NVIDIA HDR", encoding(nvidiaFull, hdr), vulkanDecoding, placebo + ",format=yuv420p"},
		{"NVIDIA HDR without Vulkan decoding", encoding(nvidiaToneMapping, hdr), cudaToneMapping, placebo + ",format=yuv420p"},
		{"NVIDIA HDR, interlaced", encoding(nvidiaFull, hdrInterlaced), cudaToneMapping, "yadif," + placebo + ",format=yuv420p"},
		// NVIDIA, HDR tone mapped on the processor: through memory.
		{"NVIDIA HDR without GPU tone mapping", encoding(nvidiaResident, hdr), cudaInMemory, cpuToneMap + ",format=yuv420p"},
		// Burned subtitles: through memory, tone mapped where it can be.
		{"NVIDIA SDR, burned", encoding(nvidiaFull, burnIn), cudaInMemory,
			"[0:0]scale=w=1920:h=1080,format=yuv420p[converted];" + burn + "format=yuv420p[video]"},
		{"NVIDIA HDR, burned", encoding(nvidiaFull, hdrBurnIn), cudaToneMapping,
			"[0:0]" + placebo + ",format=yuv420p[converted];" + burn + "format=yuv420p[video]"},
		{"NVIDIA HDR without GPU tone mapping, burned", encoding(nvidiaResident, hdrBurnIn), cudaInMemory,
			"[0:0]" + cpuToneMap + ",format=yuv420p[converted];" + burn + "format=yuv420p[video]"},
		// Decoding on the processor: frames in memory from the start.
		{"NVIDIA SDR, decoded on the processor", encoding(nvidiaFull, cpuDecoding), nil, "scale=w=1920:h=1080,format=yuv420p"},
		{"NVIDIA HDR, decoded on the processor", encoding(nvidiaFull, hdrCPUDecoding), vulkanDevices, placebo + ",format=yuv420p"},
		// A GPU whose frames did not stay at startup, or without the
		// deinterlacer: through memory.
		{"NVIDIA SDR, frames not kept", encoding(nvidiaInMemory, sdr), cudaInMemory, "scale=w=1920:h=1080,format=yuv420p"},
		{"NVIDIA SDR, no GPU deinterlacer", encoding(nvidiaNoDeinterlacer, yadif), cudaInMemory, "yadif,scale=w=1920:h=1080,format=yuv420p"},
		// VAAPI: SDR decoded into VAAPI surfaces and scaled by scale_vaapi.
		{"VAAPI SDR", encoding(vaapiResident, sdr), vaapiResidentInputs, "hwupload," + scaleVAAPI},
		{"VAAPI SDR, deinterlaced", encoding(vaapiResident, yadif), vaapiResidentInputs, "hwupload,deinterlace_vaapi," + scaleVAAPI},
		{"VAAPI SDR, deinterlaced doubled", encoding(vaapiResident, bwdifDoubled), vaapiResidentInputs, "hwupload,deinterlace_vaapi=rate=field," + scaleVAAPI},
		// VAAPI tone maps on the processor: through memory.
		{"VAAPI HDR", encoding(vaapiResident, hdr), vaapiInMemoryInputs, cpuToneMap + ",format=nv12,hwupload"},
		{"VAAPI SDR, burned", encoding(vaapiResident, burnIn), vaapiInMemoryInputs,
			"[0:0]scale=w=1920:h=1080,format=yuv420p[converted];" + burn + "format=nv12,hwupload[video]"},
		{"VAAPI HDR, burned", encoding(vaapiResident, hdrBurnIn), vaapiInMemoryInputs,
			"[0:0]" + cpuToneMap + ",format=yuv420p[converted];" + burn + "format=nv12,hwupload[video]"},
		{"VAAPI SDR, decoded on the processor", encoding(vaapiResident, cpuDecoding), vaapiDevices, "scale=w=1920:h=1080,format=nv12,hwupload"},
		{"VAAPI SDR, frames not kept", encoding(vaapiInMemory, sdr), vaapiInMemoryInputs, "scale=w=1920:h=1080,format=nv12,hwupload"},
		// Intel's GPU tone maps: frames stay on the GPU, decoded by VAAPI or
		// uploaded in 10 bits, deinterlaced, scaled in 10 bits, tone mapped
		// to 8-bit BT.709 there; burned subtitles come down to memory after
		// the tone mapping.
		{"Intel HDR", encoding(intel, hdr), vaapiResidentInputs, "hwupload," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR in HEVC", encoding(intel, func(e *VideoEncoding) { e.ToneMap, e.Encoder = true, "hevc_vaapi" }), vaapiResidentInputs,
			"hwupload," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR, decoded on the processor", encoding(intel, hdrCPUDecoding), vaapiDevices, "format=p010le,hwupload," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR, interlaced", encoding(intel, hdrInterlaced), vaapiResidentInputs, "hwupload,deinterlace_vaapi," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR, interlaced doubled", encoding(intel, func(e *VideoEncoding) { e.ToneMap, e.Deinterlace, e.DoubleRate = true, true, true }),
			vaapiResidentInputs, "hwupload,deinterlace_vaapi=rate=field," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR, interlaced, no GPU deinterlacer", encoding(intelNoDeinterlacer, hdrInterlaced), vaapiInMemoryInputs,
			"yadif,format=p010le,hwupload," + scale10 + "," + tonemapVAAPI},
		{"Intel HDR, burned", encoding(intel, hdrBurnIn), vaapiResidentInputs,
			"[0:0]hwupload," + scale10 + "," + tonemapVAAPI + ",hwdownload,format=nv12[converted];" + burn + "format=nv12,hwupload[video]"},
		{"Intel SDR, burned", encoding(intel, burnIn), vaapiInMemoryInputs,
			"[0:0]scale=w=1920:h=1080,format=yuv420p[converted];" + burn + "format=nv12,hwupload[video]"},
		// Tone mapping on the GPU turned off: the processor's, through
		// memory, on Intel's and NVIDIA's alike.
		{"Intel HDR, tone mapped on the processor", encoding(intel, cpuToneMapping), vaapiInMemoryInputs, cpuToneMap + ",format=nv12,hwupload"},
		{"NVIDIA HDR, tone mapped on the processor", encoding(nvidiaFull, cpuToneMapping), cudaInMemory, cpuToneMap + ",format=yuv420p"},
	} {
		args := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, Encode: tc.e}.args(0)
		inputs, filters := inputsAndFilters(t, args)
		if !slices.Equal(inputs, tc.inputs) {
			t.Errorf("%s: inputs\n got %q\nwant %q", tc.name, inputs, tc.inputs)
		}
		if filters != tc.filters {
			t.Errorf("%s: filters\n got %q\nwant %q", tc.name, filters, tc.filters)
		}
		if !contains(args, "-c:v", tc.e.Encoder) {
			t.Errorf("%s: no %s in %q", tc.name, tc.e.Encoder, args)
		}
		// Live channels convert through the same chains; they never burn.
		if tc.e.Burn != nil {
			continue
		}
		live := Remux{Input: "http://127.0.0.1:1/live.m3u8", InputOptions: []string{"-re"}, Video: 0, Audio: 1, Format: TS, Encode: tc.e}.liveArgs("/tmp/live", 0)
		live = slices.Insert(live, 4, "-progress", "pipe:3")
		if inputs, filters := inputsAndFilters(t, live); !slices.Equal(inputs, tc.inputs) || filters != tc.filters {
			t.Errorf("%s, live: inputs %q, filters %q", tc.name, inputs, filters)
		}
	}
}

// The GPU frames go through: NVENC takes CUDA frames as they are, with
// no format filter in memory between.
func TestResidentChainsLeaveNoMemoryFilter(t *testing.T) {
	for _, hw := range []*Hardware{
		{Method: "cuda", Encoders: []string{"hevc_nvenc"}, Resident: true},
		{Method: "vaapi", Device: "/dev/dri/renderD129", Encoders: []string{"hevc_vaapi"}, Resident: true},
	} {
		v := VideoEncoding{Encoder: hw.Encoders[0], Width: 3840, Height: 1600, Hardware: hw}
		if filters := v.filters(); strings.Contains(filters, "format=yuv420p") || strings.Contains(filters, "scale=") || !strings.HasPrefix(filters, "hwupload,scale_") {
			t.Errorf("%s: %q", hw.Method, filters)
		}
	}
}
