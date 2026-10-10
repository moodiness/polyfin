package hls

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The heights Automatic caps HDR tone mapped on the processor at: 1080p
// when a sample converted fast enough at startup (see timeToneMapping),
// else 720p, which a processor keeps up with: the conversion to linear
// light and back costs about four times the encoding at 1080p.
const (
	LowToneMappedHeight  = 720
	HighToneMappedHeight = 1080
)

// fastToneMapping is how many times real time the sample must convert at
// for HighToneMappedHeight: room for the source's own decoding, and for
// a busier server than at startup.
const fastToneMapping = 1.5

// ToneMappedHeight is the height Automatic caps HDR tone mapped on the
// processor at: HighToneMappedHeight once the timing at startup found the
// processor fast enough, LowToneMappedHeight before it ends, when it
// fails, and when the processor is too slow.
func (m *Manager) ToneMappedHeight() int {
	if height := m.toneMappedHeight.Load(); height > 0 {
		return int(height)
	}
	return LowToneMappedHeight
}

// timeToneMappingLater times HDR tone mapped on the processor in the
// background, once the GPU's chains are timed (see measureLater), and
// keeps the height it chose. Close stops it.
func (m *Manager) timeToneMappingLater() {
	if !m.HasFilters("zscale", "tonemap") {
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
		m.timingGPU.Wait()
		if ctx.Err() != nil {
			return
		}
		m.toneMappedHeight.Store(int32(m.timeToneMapping(ctx, m.Hardware())))
	}()
}

// timeToneMapping converts a 4K HDR10 sample to 1080p with the processor's
// tone mapping, as conversions run on this server: decoded on hw when
// there is one, else in software, and encoded by hw's H.264 encoder, else
// by x264 at Polyfin's preset. It reports HighToneMappedHeight when the
// sample converted at fastToneMapping times real time or faster, else
// LowToneMappedHeight, and logs what it timed.
func (m *Manager) timeToneMapping(ctx context.Context, hw *Hardware) int {
	encoding := VideoEncoding{Encoder: "libx264", Width: 1920, Height: 1080, ToneMap: true, ToneMapOnCPU: true}
	encoder := []string{"-c:v", "libx264", "-preset", "veryfast"}
	if hw != nil {
		if i := slices.IndexFunc(hw.Encoders, func(e string) bool { return strings.HasPrefix(e, "h264_") }); i >= 0 {
			encoding.Encoder, encoding.Hardware = hw.Encoders[i], hw
			encoder = []string{"-c:v", hw.Encoders[i]}
		}
	}
	if encoding.Hardware == nil && !slices.Contains(m.can.encoders, "libx264") {
		return LowToneMappedHeight
	}
	dir, err := os.MkdirTemp(m.dir, "tone-mapping-")
	if err != nil {
		m.logger.Warn("The processor's HDR tone mapping could not be timed", "error", err)
		return LowToneMappedHeight
	}
	defer os.RemoveAll(dir)
	sample := filepath.Join(dir, "hdr.mkv")
	took, err := time.Duration(0), m.makeHDRSample(ctx, hw, sample)
	if err == nil {
		took, err = m.timed(ctx, slices.Concat(probeStart, encoding.inputs(), []string{"-i", sample, "-vf", encoding.filters()}, encoder,
			[]string{"-f", "null", "-"}))
	}
	height := LowToneMappedHeight
	if err == nil && sampleLength.Seconds()/max(took, time.Millisecond).Seconds() >= fastToneMapping {
		height = HighToneMappedHeight
	}
	m.logger.Info("Timed the processor's HDR tone mapping", "encoder", encoding.Encoder, "speed", speed(took, err), "height", height)
	return height
}

// errNoHDREncoder reports that neither the GPU nor FFmpeg encodes HEVC in
// 10 bits.
var errNoHDREncoder = errors.New("no encoder writes HEVC in 10 bits")

// makeHDRSample writes sampleLength of 4K HDR10 test pattern to path, HEVC
// in 10 bits as HDR files are, from hw's encoder, else from x265: a GPU
// that encodes HEVC may still refuse 10 bits.
func (m *Manager) makeHDRSample(ctx context.Context, hw *Hardware, path string) error {
	pattern := []string{"-f", "lavfi", "-i", "testsrc2=size=3840x2160:rate=24", "-t", strconv.FormatFloat(sampleLength.Seconds(), 'f', -1, 64)}
	var encodes [][]string
	if hw != nil && slices.Contains(hw.Encoders, "hevc_nvenc") {
		encodes = append(encodes, slices.Concat(pattern, []string{"-vf", "format=p010le," + hdrTags, "-c:v", "hevc_nvenc", "-profile:v", "main10"}))
	}
	if hw != nil && slices.Contains(hw.Encoders, "hevc_vaapi") {
		encodes = append(encodes, slices.Concat(hw.devices(), pattern, []string{"-vf", "format=p010le," + hdrTags + ",hwupload", "-c:v", "hevc_vaapi", "-profile:v", "main10"}))
	}
	if slices.Contains(m.can.encoders, "libx265") {
		encodes = append(encodes, slices.Concat(pattern, []string{"-vf", "format=yuv420p10le," + hdrTags, "-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error"}))
	}
	err := errNoHDREncoder
	for _, encode := range encodes {
		if _, err = m.timed(ctx, slices.Concat(probeStart, encode, []string{path})); err == nil || ctx.Err() != nil {
			break
		}
	}
	return err
}
