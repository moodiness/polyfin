package hls

import (
	"bytes"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// everyEncoder lists the encoders of every GPU method, and x264's and
// x265's, as an FFmpeg built with all of them would.
var everyEncoder = []string{"libx264", "libx265", "h264_nvenc", "hevc_nvenc", "h264_vaapi", "hevc_vaapi", "h264_videotoolbox", "hevc_videotoolbox",
	"h264_qsv", "hevc_qsv", "h264_amf", "hevc_amf"}

// everyFilter lists the filters of every GPU method.
var everyFilter = []string{"hwupload", "hwdownload", "hwmap", "scale_cuda", "yadif_cuda", "bwdif_cuda", "libplacebo", "scale_vaapi", "tonemap_vaapi",
	"deinterlace_vaapi", "scale_vt", "yadif_videotoolbox", "scale_qsv", "vpp_qsv", "vpp_amf", "zscale", "tonemap"}

// encodersTried are the encoders the command lines a runner saw encoded
// with, in order, each once.
func encodersTried(r *recordingRunner) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var tried []string
	for _, args := range r.runs {
		if i := slices.Index(args, "-c:v"); i >= 0 && !slices.Contains(tried, args[i+1]) {
			tried = append(tried, args[i+1])
		}
	}
	return tried
}

// Each system tries only its own GPU methods, by preference, those the
// settings offer there: a system whose GPUs all fail, such as a Windows
// computer without a GPU, converts on the processor. A method of another
// system finds nothing and runs nothing.
func TestEachSystemDetectsOnlyItsMethods(t *testing.T) {
	for _, tc := range []struct {
		system string
		tried  []string
	}{
		{"linux", []string{"h264_nvenc", "hevc_nvenc", "h264_vaapi", "hevc_vaapi"}},
		{"darwin", []string{"h264_videotoolbox", "hevc_videotoolbox"}},
		{"windows", []string{"h264_nvenc", "hevc_nvenc", "h264_qsv", "hevc_qsv", "h264_amf", "hevc_amf"}},
	} {
		var names []string
		m := &Manager{system: tc.system}
		for _, method := range m.methods() {
			names = append(names, method.name)
		}
		offered := accounts.HardwareAccelerationsOn(tc.system)
		if !slices.Equal(names, offered[1:len(offered)-1]) {
			t.Errorf("%s: methods %v, the settings offer %v", tc.system, names, offered)
		}

		r := &recordingRunner{answer: func([]string) (time.Duration, error) { return 0, errors.New("no GPU") }}
		var log bytes.Buffer
		m = &Manager{system: tc.system, dir: t.TempDir(), logger: slog.New(slog.NewTextHandler(&log, nil)), detected: map[string]*Hardware{},
			done: make(chan struct{}), run: r.run, can: capabilities{encoders: everyEncoder, filters: everyFilter}}
		hw, ok := m.DetectHardware("auto", "/dev/dri/renderD128")
		if ok || m.Hardware() != nil {
			t.Errorf("%s: found %+v without a GPU", tc.system, hw)
		}
		if got := encodersTried(r); !slices.Equal(got, tc.tried) {
			t.Errorf("%s: tried %v, want %v", tc.system, got, tc.tried)
		}
		m.SelectHardware("auto", "")
		// Close waits for the timings SelectHardware started in the
		// background, which log too: the log is read once they ended.
		m.Close()
		if !strings.Contains(log.String(), "Video is converted in software: no GPU encodes") {
			t.Errorf("%s: logged %q", tc.system, log.String())
		}
	}
	for system, foreign := range map[string]string{"linux": "videotoolbox", "darwin": "nvenc", "windows": "vaapi"} {
		r := &recordingRunner{answer: func([]string) (time.Duration, error) { return time.Second, nil }}
		m := &Manager{system: system, dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
			can: capabilities{encoders: everyEncoder, filters: everyFilter}}
		if hw, _ := m.detect(foreign, "/dev/dri/renderD128"); hw != nil || r.count() != 0 {
			t.Errorf("%s found %s: %+v after %q", system, foreign, hw, r.runs)
		}
	}
}

// Each method is detected by encoding a quarter of a second with each of
// its encoders, on the device conversions open: VideoToolbox, or the
// Direct3D 11 adapter of Intel or AMD; auto on Windows takes the first
// that encodes. VideoToolbox's and QSV's tone mapping is proved on an x265
// HDR10 sample through the chain conversions use; AMF's is never tried.
func TestEachMethodIsDetectedOnItsDevice(t *testing.T) {
	for _, tc := range []struct {
		name, system, want string
		failing            []string
		method             string
		device             []string
		toneMapping        string
		inputs             []string
	}{
		{"VideoToolbox", "darwin", "auto", nil, "videotoolbox",
			[]string{"-init_hw_device", "videotoolbox=vt", "-filter_hw_device", "vt"},
			"hwupload,scale_vt=w=160:h=90:color_matrix=bt709:color_primaries=bt709:color_transfer=bt709",
			[]string{"-hwaccel", "videotoolbox", "-hwaccel_device", "vt", "-hwaccel_output_format", "videotoolbox_vld"}},
		{"QSV", "windows", "qsv", nil, "qsv",
			[]string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x8086", "-filter_hw_device", "dx"},
			"hwupload,hwmap=derive_device=qsv,format=qsv,vpp_qsv=w=160:h=90:format=nv12:tonemap=1:out_color_matrix=bt709:out_color_primaries=bt709:out_color_transfer=bt709",
			[]string{"-hwaccel", "d3d11va", "-hwaccel_device", "dx", "-hwaccel_output_format", "d3d11"}},
		{"QSV, by auto without NVIDIA", "windows", "auto", []string{"h264_nvenc", "hevc_nvenc"}, "qsv",
			[]string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x8086", "-filter_hw_device", "dx"}, "vpp_qsv=", nil},
		{"AMF, by auto without NVIDIA nor Intel", "windows", "auto", []string{"h264_nvenc", "hevc_nvenc", "h264_qsv", "hevc_qsv"}, "amf",
			[]string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x1002", "-filter_hw_device", "dx"}, "", nil},
		{"NVIDIA on Windows", "windows", "auto", nil, "cuda", nil, "libplacebo", nil},
	} {
		r := &recordingRunner{answer: func(args []string) (time.Duration, error) {
			if i := slices.Index(args, "-c:v"); i >= 0 && slices.Contains(tc.failing, args[i+1]) {
				return 0, errors.New("failed")
			}
			return time.Second, nil
		}}
		m := &Manager{system: tc.system, dir: t.TempDir(), logger: slog.New(slog.DiscardHandler), detected: map[string]*Hardware{}, run: r.run,
			can: capabilities{encoders: everyEncoder, filters: everyFilter}}
		hw, _ := m.detect(tc.want, "")
		if hw == nil || hw.Method != tc.method || len(hw.Encoders) != 2 || hw.ToneMapping != (tc.toneMapping != "") || hw.QVBR {
			t.Errorf("%s: %+v", tc.name, hw)
			continue
		}
		// Each encoder of the method encoded on the method's device.
		for _, encoder := range hw.Encoders {
			i := slices.IndexFunc(r.runs, func(args []string) bool { return contains(args, "-c:v", encoder) })
			if i < 0 || (tc.device != nil && !contains(r.runs[i], tc.device...)) || (tc.device == nil && slices.Contains(r.runs[i], "-init_hw_device")) {
				t.Errorf("%s: %s encoded with %q", tc.name, encoder, r.runs)
			}
		}
		if tc.toneMapping == "" {
			if r.ran("tonemap") || r.ran("libplacebo") || r.ran("scale_vt") {
				t.Errorf("%s: tone mapping tried: %q", tc.name, r.runs)
			}
			continue
		}
		i := slices.IndexFunc(r.runs, func(args []string) bool {
			return slices.ContainsFunc(args, func(arg string) bool { return strings.Contains(arg, tc.toneMapping) })
		})
		if i < 0 {
			t.Errorf("%s: no tone mapping check in %q", tc.name, r.runs)
			continue
		}
		if tc.inputs != nil && (!contains(r.runs[i], tc.inputs...) || !strings.HasSuffix(r.runs[i][slices.Index(r.runs[i], "-i")+1], "hdr10.mkv") ||
			!r.ran("master-display=")) {
			t.Errorf("%s: the check %q", tc.name, r.runs[i])
		}
	}
}

// The chains of VideoToolbox, QSV and AMF, as TestGPUPipelines checks
// NVIDIA's and VAAPI's: frames stay on the GPU when it does every filter,
// VideoToolbox and Intel's QSV tone map there (uploaded in 10 bits when
// the processor decodes, brought down to memory to burn subtitles), and
// everything else goes through memory, where QSV and AMF take NV12.
func TestDesktopGPUPipelines(t *testing.T) {
	plan := NewPlan(seconds(0, 6, 12), 18*time.Second)
	vt := &Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox", "hevc_videotoolbox"}, ToneMapping: true, Resident: true,
		Deinterlacers: []string{"yadif_videotoolbox"}}
	vtInMemory := &Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox"}}
	qsv := &Hardware{Method: "qsv", Encoders: []string{"h264_qsv", "hevc_qsv"}, ToneMapping: true, Resident: true, Deinterlacers: []string{"vpp_qsv"}}
	qsvNoDeinterlacer := &Hardware{Method: "qsv", Encoders: []string{"h264_qsv"}, ToneMapping: true, Resident: true}
	amf := &Hardware{Method: "amf", Encoders: []string{"h264_amf", "hevc_amf"}, Resident: true}
	amfInMemory := &Hardware{Method: "amf", Encoders: []string{"h264_amf"}}

	vtDevices := []string{"-init_hw_device", "videotoolbox=vt", "-filter_hw_device", "vt"}
	vtResident := append(slices.Clone(vtDevices), "-hwaccel", "videotoolbox", "-hwaccel_device", "vt", "-hwaccel_output_format", "videotoolbox_vld")
	vtDecoding := append(slices.Clone(vtDevices), "-hwaccel", "videotoolbox", "-hwaccel_device", "vt")
	intelDevices := []string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x8086", "-filter_hw_device", "dx"}
	intelResident := append(slices.Clone(intelDevices), "-hwaccel", "d3d11va", "-hwaccel_device", "dx", "-hwaccel_output_format", "d3d11")
	intelInMemory := append(slices.Clone(intelDevices), "-hwaccel", "d3d11va", "-hwaccel_device", "dx")
	amdDevices := []string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x1002", "-filter_hw_device", "dx"}
	amdResident := append(slices.Clone(amdDevices), "-hwaccel", "d3d11va", "-hwaccel_device", "dx", "-hwaccel_output_format", "d3d11")
	amdInMemory := append(slices.Clone(amdDevices), "-hwaccel", "d3d11va", "-hwaccel_device", "dx")

	const (
		scaleVT    = "scale_vt=w=1920:h=1080"
		toneMapVT  = "scale_vt=w=1920:h=1080:color_matrix=bt709:color_primaries=bt709:color_transfer=bt709"
		toQSV      = "hwmap=derive_device=qsv,format=qsv"
		scaleQSV   = "scale_qsv=w=1920:h=1080:format=nv12"
		toneMapQSV = "vpp_qsv=w=1920:h=1080:format=nv12:tonemap=1:out_color_matrix=bt709:out_color_primaries=bt709:out_color_transfer=bt709"
		scaleAMF   = "vpp_amf=w=1920:h=1080:format=nv12"
		cpuToneMap = "scale=w=1920:h=1080,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv"
		burn       = "[0:3]fps=24,scale=1920:-2[subtitle];[converted][subtitle]overlay=x=0:y=main_h-overlay_h:eof_action=pass,"
	)
	burned := 3
	encoding := func(hw *Hardware, change func(*VideoEncoding)) *VideoEncoding {
		e := &VideoEncoding{Encoder: hw.Encoders[0], Level: "4.1", Width: 1920, Height: 1080, Bitrate: 8_000_000, FrameRate: 24, Hardware: hw}
		if change != nil {
			change(e)
		}
		return e
	}
	hdr := func(e *VideoEncoding) { e.ToneMap = true }
	cpuDecoding := func(e *VideoEncoding) { e.DecodeOnCPU = true }
	hdrCPUDecoding := func(e *VideoEncoding) { e.ToneMap, e.DecodeOnCPU = true, true }
	yadif := func(e *VideoEncoding) { e.Deinterlace = true }
	yadifDoubled := func(e *VideoEncoding) { e.Deinterlace, e.DoubleRate = true, true }
	bwdif := func(e *VideoEncoding) { e.Deinterlace, e.Deinterlacer = true, "bwdif" }
	hdrInterlaced := func(e *VideoEncoding) { e.ToneMap, e.Deinterlace = true, true }
	burnIn := func(e *VideoEncoding) { e.Burn = &burned }
	hdrBurnIn := func(e *VideoEncoding) { e.ToneMap, e.Burn = true, &burned }
	cpuToneMapping := func(e *VideoEncoding) { e.ToneMap, e.ToneMapOnCPU = true, true }

	for _, tc := range []struct {
		name    string
		e       *VideoEncoding
		inputs  []string
		filters string
	}{
		// VideoToolbox: SDR scaled by scale_vt, HDR tone mapped by it.
		{"VideoToolbox SDR", encoding(vt, nil), vtResident, "hwupload," + scaleVT},
		{"VideoToolbox SDR, yadif", encoding(vt, yadif), vtResident, "hwupload,yadif_videotoolbox," + scaleVT},
		{"VideoToolbox SDR, yadif doubled", encoding(vt, yadifDoubled), vtResident, "hwupload,yadif_videotoolbox=1," + scaleVT},
		{"VideoToolbox SDR, bwdif, which it lacks", encoding(vt, bwdif), []string{"-hwaccel", "videotoolbox"},
			"bwdif=0,scale=w=1920:h=1080,format=yuv420p"},
		{"VideoToolbox SDR, decoded on the processor", encoding(vt, cpuDecoding), nil, "scale=w=1920:h=1080,format=yuv420p"},
		{"VideoToolbox SDR, frames not kept", encoding(vtInMemory, nil), []string{"-hwaccel", "videotoolbox"}, "scale=w=1920:h=1080,format=yuv420p"},
		{"VideoToolbox HDR", encoding(vt, hdr), vtResident, "hwupload," + toneMapVT},
		{"VideoToolbox HDR, interlaced", encoding(vt, hdrInterlaced), vtResident, "hwupload,yadif_videotoolbox," + toneMapVT},
		{"VideoToolbox HDR, bwdif on the processor", encoding(vt, func(e *VideoEncoding) { e.ToneMap, e.Deinterlace, e.Deinterlacer = true, true, "bwdif" }),
			vtDecoding, "bwdif=0,format=p010le,hwupload," + toneMapVT},
		{"VideoToolbox HDR, decoded on the processor", encoding(vt, hdrCPUDecoding), vtDevices, "format=p010le,hwupload," + toneMapVT},
		{"VideoToolbox HDR, burned", encoding(vt, hdrBurnIn), vtResident,
			"[0:0]hwupload," + toneMapVT + ",hwdownload,format=nv12[converted];" + burn + "format=yuv420p[video]"},
		{"VideoToolbox SDR, burned", encoding(vt, burnIn), []string{"-hwaccel", "videotoolbox"},
			"[0:0]scale=w=1920:h=1080,format=yuv420p[converted];" + burn + "format=yuv420p[video]"},
		{"VideoToolbox HDR, tone mapped on the processor", encoding(vt, cpuToneMapping), []string{"-hwaccel", "videotoolbox"},
			cpuToneMap + ",format=yuv420p"},
		// QSV: decoded by D3D11VA, mapped to QSV frames, scaled by
		// scale_qsv, deinterlaced and tone mapped by vpp_qsv.
		{"QSV SDR", encoding(qsv, nil), intelResident, "hwupload," + toQSV + "," + scaleQSV},
		{"QSV SDR, deinterlaced", encoding(qsv, yadif), intelResident, "hwupload," + toQSV + ",vpp_qsv=deinterlace=advanced," + scaleQSV},
		{"QSV SDR, deinterlaced doubled", encoding(qsv, yadifDoubled), intelResident,
			"hwupload," + toQSV + ",vpp_qsv=deinterlace=advanced:rate=field," + scaleQSV},
		{"QSV SDR, decoded on the processor", encoding(qsv, cpuDecoding), nil, "scale=w=1920:h=1080,format=nv12"},
		{"QSV HDR", encoding(qsv, hdr), intelResident, "hwupload," + toQSV + "," + toneMapQSV},
		{"QSV HDR, interlaced", encoding(qsv, hdrInterlaced), intelResident, "hwupload," + toQSV + ",vpp_qsv=deinterlace=advanced," + toneMapQSV},
		{"QSV HDR, interlaced, no GPU deinterlacer", encoding(qsvNoDeinterlacer, hdrInterlaced), intelInMemory,
			"yadif,format=p010le,hwupload," + toQSV + "," + toneMapQSV},
		{"QSV HDR, decoded on the processor", encoding(qsv, hdrCPUDecoding), intelDevices, "format=p010le,hwupload," + toQSV + "," + toneMapQSV},
		{"QSV HDR, burned", encoding(qsv, hdrBurnIn), intelResident,
			"[0:0]hwupload," + toQSV + "," + toneMapQSV + ",hwdownload,format=nv12[converted];" + burn + "format=nv12[video]"},
		{"QSV HDR, tone mapped on the processor", encoding(qsv, cpuToneMapping), intelInMemory, cpuToneMap + ",format=nv12"},
		// AMF: decoded by D3D11VA, scaled by vpp_amf; no deinterlacer nor
		// tone mapping on the GPU.
		{"AMF SDR", encoding(amf, nil), amdResident, "hwupload," + scaleAMF},
		{"AMF SDR, deinterlaced on the processor", encoding(amf, yadif), amdInMemory, "yadif,scale=w=1920:h=1080,format=nv12"},
		{"AMF SDR, frames not kept", encoding(amfInMemory, nil), amdInMemory, "scale=w=1920:h=1080,format=nv12"},
		{"AMF HDR", encoding(amf, hdr), amdInMemory, cpuToneMap + ",format=nv12"},
		{"AMF SDR, burned", encoding(amf, burnIn), amdInMemory,
			"[0:0]scale=w=1920:h=1080,format=yuv420p[converted];" + burn + "format=nv12[video]"},
	} {
		args := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, Encode: tc.e}.args(0, 0)
		inputs, filters := inputsAndFilters(t, args)
		if !slices.Equal(inputs, tc.inputs) {
			t.Errorf("%s: inputs\n got %q\nwant %q", tc.name, inputs, tc.inputs)
		}
		if filters != tc.filters {
			t.Errorf("%s: filters\n got %q\nwant %q", tc.name, filters, tc.filters)
		}
	}
}

// Every encoder writes the colors the frames carry: converted frames are
// tagged last with the source's colors, those known, on the processor
// and on each GPU, burned subtitles or not; tone mapped frames are left
// to the tone mapping, which tags them BT.709.
func TestConvertedFramesAreTaggedWithTheSourcesColors(t *testing.T) {
	plan := NewPlan(seconds(0, 6, 12), 18*time.Second)
	gpu := func(method string, encoder string) *Hardware {
		return &Hardware{Method: method, Encoders: []string{encoder}, Resident: true, ToneMapping: true}
	}
	const all = ",setparams=range=tv:colorspace=bt709:color_trc=bt709:color_primaries=bt709"
	bt709 := Colors{Range: "tv", Matrix: "bt709", Transfer: "bt709", Primaries: "bt709"}
	burned := 2
	for _, tc := range []struct {
		name   string
		e      VideoEncoding
		suffix string
	}{
		{"x264", VideoEncoding{Encoder: "libx264", Colors: bt709}, all},
		{"NVENC", VideoEncoding{Encoder: "h264_nvenc", Hardware: gpu("cuda", "h264_nvenc"), Colors: bt709}, all},
		{"VAAPI", VideoEncoding{Encoder: "h264_vaapi", Hardware: gpu("vaapi", "h264_vaapi"), Colors: bt709}, all},
		{"QSV", VideoEncoding{Encoder: "h264_qsv", Hardware: gpu("qsv", "h264_qsv"), Colors: bt709}, all},
		{"AMF", VideoEncoding{Encoder: "h264_amf", Hardware: gpu("amf", "h264_amf"), Colors: bt709}, all},
		{"VideoToolbox", VideoEncoding{Encoder: "h264_videotoolbox", Hardware: gpu("videotoolbox", "h264_videotoolbox"), Colors: bt709}, all},
		{"burned in", VideoEncoding{Encoder: "libx264", Burn: &burned, Colors: bt709}, all + "[video]"},
		// Only what the source states: an HEVC source without a VUI
		// still has limited range.
		{"range alone", VideoEncoding{Encoder: "libx264", Colors: Colors{Range: "tv", Matrix: "unknown"}}, "format=yuv420p,setparams=range=tv"},
		{"nothing known", VideoEncoding{Encoder: "libx264", Colors: Colors{Matrix: "unknown", Transfer: "reserved"}}, "format=yuv420p"},
		{"an RGB source", VideoEncoding{Encoder: "libx264", Colors: Colors{Matrix: "gbr"}}, "format=yuv420p"},
		{"tone mapped", VideoEncoding{Encoder: "libx264", ToneMap: true, Colors: Colors{Range: "tv", Matrix: "bt2020nc", Transfer: "smpte2084", Primaries: "bt2020"}},
			"zscale=t=bt709:m=bt709:r=tv,format=yuv420p"},
		{"tone mapped by VideoToolbox", VideoEncoding{Encoder: "h264_videotoolbox", Hardware: gpu("videotoolbox", "h264_videotoolbox"), ToneMap: true,
			Colors: Colors{Transfer: "smpte2084", Primaries: "bt2020"}}, "color_transfer=bt709"},
	} {
		tc.e.Level, tc.e.Width, tc.e.Height, tc.e.Bitrate, tc.e.FrameRate = "4.1", 1920, 1080, 8_000_000, 24
		args := Remux{Input: "http://127.0.0.1:1/a.mkv", Video: 0, Audio: 1, Format: FMP4, Plan: plan, Encode: &tc.e}.args(0, 0)
		i := slices.IndexFunc(args, func(arg string) bool { return arg == "-vf" || arg == "-filter_complex" })
		if i < 0 || !strings.HasSuffix(args[i+1], tc.suffix) {
			t.Errorf("%s: %q, want it to end with %q", tc.name, args, tc.suffix)
		}
	}
}

// Each encoder family takes its own options, from FFmpeg's documentation
// of each: VideoToolbox's H.264 encoder names levels up to 5.2 and its
// HEVC encoder none, and its quality runs from 1 to 100 within the
// maximum rate; QSV and AMF force IDR frames, QSV picks its level, and AMF
// names levels with a tenth; QSV and AMF aim for the bitrate, a quality
// set or not. Presets follow x264's names. Every H.264 encoder, x264 as
// the GPUs', encodes the High profile the master playlist announces.
func TestEachEncoderFamilyTakesItsOptions(t *testing.T) {
	plan := NewPlan(seconds(0, 6, 12), 18*time.Second)
	for _, tc := range []struct {
		name       string
		e          VideoEncoding
		want, none [][]string
	}{
		{"x264", VideoEncoding{Encoder: "libx264", Level: "4.1", Bitrate: 8_000_000},
			[][]string{{"-b:v", "8000000", "-maxrate", "12000000"}, {"-preset", "veryfast", "-sc_threshold", "0", "-profile:v", "high", "-level:v", "4.1"}}, nil},
		{"x265", VideoEncoding{Encoder: "libx265", Level: "4.1", Bitrate: 8_000_000},
			[][]string{{"-preset", "veryfast", "-sc_threshold", "0", "-profile:v", "main"}}, nil},
		{"VideoToolbox H.264", VideoEncoding{Encoder: "h264_videotoolbox", Level: "5.1", Bitrate: 8_000_000, Preset: "faster"},
			[][]string{{"-b:v", "8000000", "-maxrate", "12000000", "-bufsize", "16000000"}, {"-profile:v", "high", "-level:v", "5.1", "-prio_speed", "1"}},
			[][]string{{"-forced_idr"}, {"-rc_mode"}, {"-compression_level"}, {"-global_quality"}}},
		{"VideoToolbox H.264 at 6.1", VideoEncoding{Encoder: "h264_videotoolbox", Level: "6.1", Bitrate: 8_000_000, Preset: "slow"},
			[][]string{{"-profile:v", "high", "-prio_speed", "0"}}, [][]string{{"-level:v"}}},
		{"VideoToolbox HEVC with a quality", VideoEncoding{Encoder: "hevc_videotoolbox", Level: "5", Bitrate: 8_000_000, Quality: 23},
			[][]string{{"-maxrate", "12000000"}, {"-profile:v", "main"}, {"-global_quality", "54"}}, [][]string{{"-level:v"}, {"-b:v"}, {"-prio_speed"}}},
		{"QSV H.264", VideoEncoding{Encoder: "h264_qsv", Level: "4.1", Bitrate: 8_000_000, Quality: 23, Preset: "ultrafast"},
			[][]string{{"-b:v", "8000000", "-maxrate", "12000000"}, {"-forced_idr", "1", "-profile:v", "high", "-preset", "veryfast"}},
			[][]string{{"-level:v"}, {"-global_quality"}, {"-rc_mode"}, {"-compression_level"}}},
		{"QSV HEVC, auto preset", VideoEncoding{Encoder: "hevc_qsv", Level: "5", Bitrate: 8_000_000},
			[][]string{{"-forced_idr", "1", "-profile:v", "main"}}, [][]string{{"-preset"}}},
		{"AMF H.264", VideoEncoding{Encoder: "h264_amf", Level: "5", Bitrate: 8_000_000, Quality: 23, Preset: "medium"},
			[][]string{{"-b:v", "8000000"}, {"-rc", "vbr_peak", "-forced_idr", "1", "-profile:v", "high", "-level:v", "5.0", "-quality", "balanced"}},
			[][]string{{"-qvbr_quality_level"}, {"-rc_mode"}, {"-compression_level"}}},
		{"AMF HEVC", VideoEncoding{Encoder: "hevc_amf", Level: "4.1", Bitrate: 8_000_000, Preset: "veryslow"},
			[][]string{{"-profile:v", "main", "-level:v", "4.1", "-quality", "quality"}}, nil},
	} {
		tc.e.Width, tc.e.Height, tc.e.FrameRate = 1920, 1080, 24
		args := tc.e.args(plan, 0)
		if !contains(args, "-c:v", tc.e.Encoder) {
			t.Errorf("%s: %q", tc.name, args)
		}
		for _, want := range tc.want {
			if !contains(args, want...) {
				t.Errorf("%s: %q lacks %q", tc.name, args, want)
			}
		}
		for _, none := range tc.none {
			if contains(args, none...) {
				t.Errorf("%s: %q has %q", tc.name, args, none)
			}
		}
	}
}

// Recordings are converted to HEVC by each method at the quality factor of
// x265, VideoToolbox's on its own scale, decoded as conversions are.
func TestFileConversionsOnEachMethod(t *testing.T) {
	vt := &Hardware{Method: "videotoolbox", Encoders: []string{"hevc_videotoolbox"}, Resident: true}
	qsv := &Hardware{Method: "qsv", Encoders: []string{"hevc_qsv"}}
	amf := &Hardware{Method: "amf", Encoders: []string{"hevc_amf"}}
	for _, tc := range []struct {
		name string
		v    VideoEncoding
		want [][]string
	}{
		{"VideoToolbox", VideoEncoding{Encoder: "hevc_videotoolbox", Hardware: vt, Preset: "fast"},
			[][]string{{"-hwaccel", "videotoolbox", "-hwaccel_device", "vt", "-hwaccel_output_format", "videotoolbox_vld"},
				{"-vf", "hwupload,scale_vt=w=1920:h=1080"}, {"-global_quality", "44", "-prio_speed", "1"}}},
		{"QSV", VideoEncoding{Encoder: "hevc_qsv", Hardware: qsv, Quality: 25, Preset: "slower"},
			[][]string{{"-hwaccel", "d3d11va", "-hwaccel_device", "dx"}, {"-vf", "scale=w=1920:h=1080,format=nv12"},
				{"-global_quality", "25", "-preset", "slower"}}},
		{"AMF", VideoEncoding{Encoder: "hevc_amf", Hardware: amf, Preset: "superfast"},
			[][]string{{"-init_hw_device", "d3d11va=dx:,vendor_id=0x1002"}, {"-rc", "cqp", "-qp_i", "28", "-qp_p", "28", "-quality", "speed"}}},
	} {
		tc.v.Width, tc.v.Height, tc.v.FrameRate, tc.v.Bitrate = 1920, 1080, 25, 6_000_000
		args := FileConversion{Input: "in.ts", Output: "out.mkv", Video: tc.v}.args()
		for _, want := range append(tc.want, []string{"-c:v", tc.v.Encoder}, []string{"-maxrate", "6000000"},
			[]string{"-profile:v", "main", "-f", "matroska", "out.mkv"}) {
			if !contains(args, want...) {
				t.Errorf("%s: %q lacks %q", tc.name, args, want)
			}
		}
		if slices.Contains(args, "-rc_mode") || slices.Contains(args, "-compression_level") {
			t.Errorf("%s: VAAPI's options in %q", tc.name, args)
		}
	}
}

// Work that only decodes, such as thumbnails, decodes on each method's
// GPU into memory, and the metrics name each method.
func TestEachMethodDecodesAndIsCounted(t *testing.T) {
	for _, tc := range []struct {
		hw     *Hardware
		inputs []string
		method string
	}{
		{&Hardware{Method: "cuda"}, []string{"-hwaccel", "cuda"}, MethodNVIDIA},
		{&Hardware{Method: "vaapi", Device: "/dev/dri/renderD128"},
			[]string{"-init_hw_device", "vaapi=va:/dev/dri/renderD128", "-filter_hw_device", "va", "-hwaccel", "vaapi", "-hwaccel_device", "va"}, MethodVAAPI},
		{&Hardware{Method: "videotoolbox"}, []string{"-hwaccel", "videotoolbox"}, MethodVideoToolbox},
		{&Hardware{Method: "qsv"}, []string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x8086", "-filter_hw_device", "dx", "-hwaccel", "d3d11va",
			"-hwaccel_device", "dx"}, MethodQSV},
		{&Hardware{Method: "amf"}, []string{"-init_hw_device", "d3d11va=dx:,vendor_id=0x1002", "-filter_hw_device", "dx", "-hwaccel", "d3d11va",
			"-hwaccel_device", "dx"}, MethodAMF},
		{nil, nil, MethodProcessor},
	} {
		if got := tc.hw.DecodeInputs(); !slices.Equal(got, tc.inputs) {
			t.Errorf("%+v: %q", tc.hw, got)
		}
		if got := (&VideoEncoding{Hardware: tc.hw}).method(); got != tc.method || !slices.Contains(conversionMethods, got) {
			t.Errorf("%+v: counted as %q", tc.hw, got)
		}
	}
	if (&Hardware{Method: "qsv"}).NeedsMasteringDisplay() != true || (&Hardware{Method: "videotoolbox"}).NeedsMasteringDisplay() {
		t.Error("only Intel's GPUs need the mastering display")
	}
}

// VideoToolbox's chains are timed as NVIDIA's and VAAPI's are, and a
// deinterlacer FFmpeg lists that fails on the GPU, as yadif_videotoolbox
// may, is left out: interlaced video then goes through memory.
func TestVideoToolboxChainsAreTimed(t *testing.T) {
	m, f := measuringManager(t, chainTimes{sdrMemory: time.Second, sdrGPU: time.Second / 2, brokenDeinterlacers: []string{"yadif_videotoolbox"}})
	m.can = capabilities{encoders: everyEncoder, filters: everyFilter}
	hw := m.measure(t.Context(), Hardware{Method: "videotoolbox", Encoders: []string{"h264_videotoolbox", "hevc_videotoolbox"}})
	if !hw.Resident || len(hw.Deinterlacers) != 0 {
		t.Errorf("%+v", hw)
	}
	if !contains(f.runs["sdr sample"], "-c:v", "h264_videotoolbox") ||
		!contains(f.runs["sdr gpu"], "-hwaccel_output_format", "videotoolbox_vld") || !contains(f.runs["sdr gpu"], "-vf", "hwupload,scale_vt=w=1920:h=1080") ||
		!contains(f.runs["sdr memory"], "-hwaccel", "videotoolbox") || !contains(f.runs["sdr memory"], "-vf", "scale=w=1920:h=1080,format=yuv420p") {
		t.Errorf("chains: %q", f.runs)
	}
	if check := f.runs["deinterlacer yadif_videotoolbox"]; !contains(check, "-init_hw_device", "videotoolbox=vt") ||
		!contains(check, "-vf", "format=nv12,hwupload,yadif_videotoolbox,hwdownload,format=nv12") {
		t.Errorf("deinterlacer check %q", check)
	}
	// QSV's deinterlacer is checked on QSV frames.
	m, f = measuringManager(t, chainTimes{sdrMemory: time.Second, sdrGPU: time.Second / 2})
	m.can = capabilities{encoders: everyEncoder, filters: everyFilter}
	if hw := m.measure(t.Context(), Hardware{Method: "qsv", Encoders: []string{"h264_qsv"}}); !hw.Resident || !slices.Equal(hw.Deinterlacers, []string{"vpp_qsv"}) {
		t.Errorf("QSV: %+v", hw)
	}
	if check := f.runs["deinterlacer vpp_qsv"]; !contains(check, "-vf", "format=nv12,hwupload,hwmap=derive_device=qsv,format=qsv,vpp_qsv=deinterlace=advanced,hwdownload,format=nv12") {
		t.Errorf("QSV's deinterlacer check %q", check)
	}
}
