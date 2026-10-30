package hls

import (
	"math"
	"strconv"
)

// realTime is the speed, in times real time, a conversion keeps up with
// playback at.
const realTime = 1.0

// Why HDR is tone mapped where HDRPath says, as HDRPath.Reason tells it.
const (
	// HDRNotTimed: the GPU's HDR tone mapping was not timed, not yet or
	// not successfully. The GPU tone maps HDR, as before the timings.
	HDRNotTimed = "notTimed"
	// HDRKeepsUp: the GPU tone maps HDR at real time or faster.
	HDRKeepsUp = "keepsUp"
	// HDRProcessorFaster: the GPU tone maps HDR slower than real time,
	// and the processor keeps up: the processor tone maps.
	HDRProcessorFaster = "processorFaster"
	// HDRLowerHeight: the GPU tone maps HDR slower than real time, keeps
	// up at LowToneMappedHeight, and the processor does not keep up: the
	// GPU tone maps up to that height.
	HDRLowerHeight = "lowerHeight"
	// HDRNoneKeepsUp: neither the GPU, even at LowToneMappedHeight, nor
	// the processor keeps up: the faster of the two tone maps.
	HDRNoneKeepsUp = "noneKeepsUp"
)

// HDRPath is where HDR a GPU tone maps is tone mapped instead, as the
// timings at startup chose it: the GPU's HDR chain to 1080p (see
// Hardware.HDRSpeed), then to LowToneMappedHeight when that is under real
// time, and the processor's tone mapping (see timeToneMapping). A GPU
// keeping up tone maps HDR at any height. One slower than real time hands
// HDR to the processor when the processor keeps up, else tone maps it up
// to LowToneMappedHeight when it keeps up there, else leaves it to the
// faster of the two. The video is encoded on the GPU either way.
type HDRPath struct {
	// Processor tone maps on the processor the HDR the GPU would, the
	// processor's own cap applying (see Capabilities.ToneMappedHeight).
	Processor bool
	// Height caps the HDR the GPU tone maps, 0 for none:
	// LowToneMappedHeight when its HDR chain is slower than real time, all
	// of it or, with Processor, what the processor cannot tone map.
	Height int
	// Reason is one of the HDR reasons, empty for a GPU that does not tone
	// map.
	Reason string
	// GPUSpeed, LowSpeed and ProcessorSpeed are how many times real time
	// the GPU tone mapped to 1080p, then to LowToneMappedHeight, and the
	// processor to 1080p, at startup; 0 when not timed, or failed.
	GPUSpeed, LowSpeed, ProcessorSpeed float64
}

// HDRPath is where HDR is tone mapped on hw, the GPU conversions run on,
// with the processor's timing: the zero HDRPath for none, and for a GPU
// that does not tone map.
func (m *Manager) HDRPath(hw *Hardware) HDRPath {
	if hw == nil || !hw.ToneMapping {
		return HDRPath{}
	}
	return chooseHDRPath(hw.HDRSpeed, hw.LowHDRSpeed, math.Float64frombits(m.toneMappingSpeed.Load()))
}

// chooseHDRPath chooses where HDR is tone mapped from the speeds of the
// GPU to 1080p and lower, and of the processor (see HDRPath).
func chooseHDRPath(gpu, low, processor float64) HDRPath {
	path := HDRPath{GPUSpeed: gpu, LowSpeed: low, ProcessorSpeed: processor}
	switch {
	case gpu <= 0:
		path.Reason = HDRNotTimed
		return path
	case gpu >= realTime:
		path.Reason = HDRKeepsUp
		return path
	}
	path.Height = LowToneMappedHeight
	switch {
	case processor >= realTime:
		path.Processor, path.Reason = true, HDRProcessorFaster
	case low >= realTime:
		path.Reason = HDRLowerHeight
	default:
		// The processor was timed to 1080p, and converts at its own cap,
		// 720p when that timing is under 1.5x: faster than it measured.
		path.Processor, path.Reason = processor > max(gpu, low), HDRNoneKeepsUp
	}
	return path
}

// logHDRPath logs where HDR is tone mapped on the GPU chosen, and why,
// nothing for a GPU that does not tone map.
func (m *Manager) logHDRPath() {
	hw := m.Hardware()
	if hw == nil || !hw.ToneMapping {
		return
	}
	path := m.HDRPath(hw)
	where := "gpu"
	if path.Processor {
		where = "processor"
	}
	m.logger.Info("Chose where HDR is tone mapped", "tone_mapping", where, "height", path.Height, "reason", path.Reason,
		"gpu_speed", speedText(path.GPUSpeed), "low_speed", speedText(path.LowSpeed), "processor_speed", speedText(path.ProcessorSpeed))
}

// speedText writes a speed timed at startup as the log shows it: "0.5x",
// or "none" when not timed.
func speedText(speed float64) string {
	if speed <= 0 {
		return "none"
	}
	return strconv.FormatFloat(speed, 'f', 1, 64) + "x"
}

// Why a conversion tone maps HDR on the processor though its GPU tone
// maps, or on its GPU below the height it would convert at, as
// VideoEncoding.ToneMapReason tells it.
const (
	// ToneMapTurnedOff: the settings turned tone mapping on the GPU off.
	ToneMapTurnedOff = "turned_off"
	// ToneMapUnsupported: the GPU does not tone map this video's kind of
	// HDR.
	ToneMapUnsupported = "unsupported"
	// ToneMapSlowGPU: the GPU's HDR tone mapping was timed slower than
	// real time (see HDRPath).
	ToneMapSlowGPU = "slow_gpu"
)

// toneMapAttrs are the attributes of an encoding's log line telling where
// v tone maps HDR and why: none when it copies the video, or converts
// SDR. A reason is given when the GPU does not tone map at full height,
// with the timings for ToneMapSlowGPU.
func (m *Manager) toneMapAttrs(v *VideoEncoding) []any {
	if v == nil || !v.ToneMap {
		return nil
	}
	where := "processor"
	if v.toneMapsOnGPU() {
		where = "gpu"
	}
	attrs := []any{"tone_mapping", where}
	if v.ToneMapReason == "" {
		return attrs
	}
	attrs = append(attrs, "tone_mapping_reason", v.ToneMapReason)
	if v.ToneMapReason == ToneMapSlowGPU {
		path := m.HDRPath(v.Hardware)
		attrs = append(attrs, "gpu_speed", speedText(path.GPUSpeed), "low_speed", speedText(path.LowSpeed),
			"processor_speed", speedText(path.ProcessorSpeed))
	}
	return attrs
}
