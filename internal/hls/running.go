package hls

import (
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
)

// Running is an encoding under way, as the admin dashboard shows it.
type Running struct {
	Key Key
	// Live is set for the encoding of a channel, read as it comes.
	Live bool
	// Opened is set once its input is open: what follows is known then.
	Opened bool
	// Video is what the video is converted to, nil when it is copied.
	Video *VideoEncoding
	// AudioCodec is the encoder the audio is converted with, empty when
	// it is copied; AudioChannels and AudioBitrate, what it is converted
	// to.
	AudioCodec    string
	AudioChannels int
	AudioBitrate  int64
	// Progressive is set for a conversion streamed as one file (see
	// Manager.Progressive), Position telling how far into the version it
	// has written.
	Progressive bool
	Position    time.Duration
}

// Running lists the encodings under way, of files and channels, and the
// conversions streamed as one file; the recordings are left out. It never
// waits for an input being opened.
func (m *Manager) Running() []Running {
	m.mu.Lock()
	encodings := make([]*encoding, 0, len(m.encodings))
	for _, e := range m.encodings {
		encodings = append(encodings, e)
	}
	lives := make([]*live, 0, len(m.lives))
	for _, l := range m.lives {
		lives = append(lives, l)
	}
	running := make([]Running, 0, len(encodings)+len(lives)+len(m.progressives))
	for p := range m.progressives {
		r := Running{Key: p.key, Progressive: true, Position: time.Duration(p.position.Load())}
		if p.opened {
			r.describe(p.remux)
		}
		running = append(running, r)
	}
	m.mu.Unlock()

	for _, e := range encodings {
		r := Running{Key: e.key}
		select {
		case <-e.opened:
			if e.err == nil {
				r.describe(e.remux)
			}
		default:
		}
		running = append(running, r)
	}
	for _, l := range lives {
		r := Running{Key: l.key, Live: true}
		// A live input being opened holds its lock: it is shown as such.
		if l.mu.TryLock() {
			if l.opened {
				r.describe(l.remux)
			}
			l.mu.Unlock()
		}
		running = append(running, r)
	}
	return running
}

func (r *Running) describe(remux Remux) {
	r.Opened = true
	if remux.Encode != nil {
		video := *remux.Encode
		r.Video = &video
	}
	r.AudioCodec, r.AudioChannels, r.AudioBitrate = remux.AudioCodec, remux.AudioChannels, remux.AudioBitrate
}

// Conversions counts the conversions of video under way, as the limit
// counts them (see converting), and tells the limit, 0 for none.
func (m *Manager) Conversions() (running, limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.converting(nil, nil), max(m.limit(), 0)
}

// The methods an encoding makes its video by, as the metrics count them:
// converted on an NVIDIA GPU, on a GPU through VAAPI, or on the
// processor, or copied.
const (
	MethodNVIDIA    = "nvidia"
	MethodVAAPI     = "vaapi"
	MethodProcessor = "processor"
	MethodCopy      = "copy"
)

// method is how an encoding converting its video to v makes it, copy for
// nil.
func (v *VideoEncoding) method() string {
	switch {
	case v == nil:
		return MethodCopy
	case v.Hardware == nil:
		return MethodProcessor
	case v.Hardware.Method == "cuda":
		return MethodNVIDIA
	}
	// The only other GPU method.
	return MethodVAAPI
}

// ConversionsByMethod counts the encodings under way that convert their
// video, of files and channels, by method, each method told.
func (m *Manager) ConversionsByMethod() []metrics.Sample {
	counts := map[string]float64{}
	for _, r := range m.Running() {
		if r.Video != nil {
			counts[r.Video.method()]++
		}
	}
	samples := make([]metrics.Sample, 0, 3)
	for _, method := range []string{MethodNVIDIA, MethodVAAPI, MethodProcessor} {
		samples = append(samples, metrics.Sample{Labels: []string{method}, Value: counts[method]})
	}
	return samples
}
