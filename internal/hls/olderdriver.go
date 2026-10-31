package hls

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

// NVIDIA's drivers older than 610, such as the R580 drivers that are the
// last of Maxwell, Pascal and Volta cards, offer NVENC's API 13.0, and the
// FFmpeg Polyfin ships is built for 13.1: its NVENC refuses them. FFmpeg
// built for 13.0 takes drivers from 570, and API 13.1 adds nothing Polyfin
// uses. When NVENC's test encode finds such a driver, the Manager takes
// that FFmpeg, built from the same release (scripts/ffmpeg-nvenc13.sh),
// downloads it in the background when it is not there yet, converting on
// the processor meanwhile, and runs it in place of its own while the GPU
// it found with it is the one chosen.

// OlderDriverFFmpeg is the FFmpeg built for NVENC's API 13.0.
type OlderDriverFFmpeg interface {
	// FFmpegReady returns its executable when it is downloaded and checked.
	FFmpegReady() (string, bool)
	// FetchFFmpeg downloads it, checks it against its pinned checksums,
	// unpacks it and returns its executable.
	FetchFFmpeg(ctx context.Context) (string, error)
}

// OlderDriverMinimum is the NVIDIA driver the FFmpeg for older drivers
// needs, as its NVENC tells it.
const OlderDriverMinimum = "570.0"

// olderDriverAPI is NVENC's API that FFmpeg is built for, 13.0, as FFmpeg
// counts versions: major << 4 | minor.
const olderDriverAPI = 13<<4 | 0

// nvencAPI matches the NVENC API versions FFmpeg's NVENC was built for and
// the driver offers, when it refuses the driver.
var nvencAPI = regexp.MustCompile(`Required: ([0-9]+)\.([0-9]+) Found: ([0-9]+)\.([0-9]+)`)

// takesOlderDriverFFmpeg reports whether the driver NVENC refused with
// err offers API 13.0 or later: the FFmpeg for older drivers then works.
func takesOlderDriverFFmpeg(err error) bool {
	found := nvencAPI.FindStringSubmatch(err.Error())
	if found == nil {
		return false
	}
	major, _ := strconv.Atoi(found[3])
	minor, _ := strconv.Atoi(found[4])
	return major<<4|minor >= olderDriverAPI
}

// UseOlderDriverFFmpeg gives the Manager the FFmpeg for NVIDIA drivers
// older than 610, for the GPU's detections from now on.
func (m *Manager) UseOlderDriverFFmpeg(f OlderDriverFFmpeg) {
	m.detecting.Lock()
	defer m.detecting.Unlock()
	m.older = f
}

// OlderDriverStatus tells how the FFmpeg for older NVIDIA drivers stands.
type OlderDriverStatus struct {
	// State is "" when NVENC needs no such FFmpeg, as without NVIDIA's GPU
	// or with a recent enough driver; "used" while the GPU chosen converts
	// with it; "downloading" while it is downloaded; "failed" when its
	// download failed; and "unusable" when NVENC did not encode with it
	// either. Error tells why, for the last two.
	State string
	Error string
}

// OlderDriverFFmpeg tells how the FFmpeg for older NVIDIA drivers stands
// for the GPU chosen last.
func (m *Manager) OlderDriverFFmpeg() OlderDriverStatus {
	if hw := m.hardware.Load(); hw != nil && hw.FFmpeg != "" {
		return OlderDriverStatus{State: "used"}
	}
	failures := m.failures.Load()
	if failures == nil {
		return OlderDriverStatus{}
	}
	for _, failure := range *failures {
		switch {
		case failure.pending:
			m.olderMu.Lock()
			defer m.olderMu.Unlock()
			if !m.fetching && m.fetchErr != nil {
				return OlderDriverStatus{State: "failed", Error: m.fetchErr.Error()}
			}
			return OlderDriverStatus{State: "downloading"}
		case failure.unusable:
			// FFmpeg's reason, without the log's mention of this FFmpeg.
			err := failure.err
			if inner := errors.Unwrap(err); inner != nil {
				err = inner
			}
			return OlderDriverStatus{State: "unusable", Error: err.Error()}
		}
	}
	return OlderDriverStatus{}
}

// olderDriverHardware tries NVENC on hw again with the FFmpeg for older
// drivers, after the Manager's own refused the GPU's driver, as failure
// tells. It returns the GPU when NVENC encodes with it; otherwise the
// failure to tell: the driver version needed when that FFmpeg cannot help
// (FFmpeg's own, or that FFmpeg's when the Manager has it), pending while
// it is not downloaded yet, or why NVENC failed with it too.
func (m *Manager) olderDriverHardware(hw Hardware, candidate gpuMethod, failure gpuFailure, needed string) (*Hardware, gpuFailure) {
	if m.older == nil {
		failure.driver = needed
		return nil, failure
	}
	if !takesOlderDriverFFmpeg(failure.err) {
		failure.driver = OlderDriverMinimum
		return nil, failure
	}
	path, ok := m.older.FFmpegReady()
	if !ok {
		failure.pending = true
		return nil, failure
	}
	hw.FFmpeg = path
	var older *gpuFailure
	for _, encoder := range candidate.encoders {
		if !slices.Contains(m.can.encoders, encoder) {
			continue
		}
		if err := m.encodes(hw, encoder); err == nil {
			hw.Encoders = append(hw.Encoders, encoder)
		} else if older == nil {
			older = &gpuFailure{method: candidate.name, device: hw.Device, encoder: encoder, unusable: true,
				err: fmt.Errorf("with the FFmpeg for NVIDIA drivers older than 610: %w", err)}
		}
	}
	if len(hw.Encoders) > 0 {
		return &hw, gpuFailure{}
	}
	return nil, *older
}

// fetchOlderDriverFFmpeg downloads the FFmpeg for older drivers in the
// background, unless a download runs, and once it is there detects the
// GPU again for the choice made last, which then takes it. Close stops it.
func (m *Manager) fetchOlderDriverFFmpeg() {
	m.olderMu.Lock()
	if m.fetching || m.older == nil {
		m.olderMu.Unlock()
		return
	}
	m.fetching = true
	m.olderMu.Unlock()
	older := m.older
	m.logger.Info("NVENC needs the FFmpeg for NVIDIA drivers older than 610: it is downloaded, and video is converted on the processor meanwhile")
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
		_, err := older.FetchFFmpeg(ctx)
		m.olderMu.Lock()
		m.fetching, m.fetchErr = false, err
		m.olderMu.Unlock()
		if err != nil {
			if ctx.Err() == nil {
				m.logger.Warn("The FFmpeg for NVIDIA drivers older than 610 could not be downloaded: NVENC converts nothing until it is", "error", err)
			}
			return
		}
		m.logger.Info("The FFmpeg for NVIDIA drivers older than 610 is downloaded")
		select {
		case <-m.done:
			return
		default:
		}
		// The choices that waited for it are detected again.
		m.detecting.Lock()
		for want, failures := range m.failed {
			if slices.ContainsFunc(failures, func(f gpuFailure) bool { return f.pending }) {
				delete(m.detected, want)
				delete(m.failed, want)
			}
		}
		want, device := m.wanted, m.wantedDevice
		m.detecting.Unlock()
		m.SelectHardware(want, device)
	}()
}

// ffmpegKey carries, in a context, the FFmpeg a test run uses.
type ffmpegKey struct{}

// withFFmpeg is ctx running FFmpeg from path, unless path is empty.
func withFFmpeg(ctx context.Context, path string) context.Context {
	if path == "" {
		return ctx
	}
	return context.WithValue(ctx, ffmpegKey{}, path)
}

// ffmpegPath is the FFmpeg the Manager runs: the one the GPU chosen
// converts with, the FFmpeg for older NVIDIA drivers, else its own.
func (m *Manager) ffmpegPath() string {
	if hw := m.hardware.Load(); hw != nil && hw.FFmpeg != "" {
		return hw.FFmpeg
	}
	return m.ffmpeg
}
