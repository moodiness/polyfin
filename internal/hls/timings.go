package hls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

// The chains a timing times, as the timings kept name them: SDR through
// memory and kept on the GPU, HDR through memory and on the GPU (Vulkan
// frames on NVIDIA's, the device's own tone mapping on Intel's and with
// VideoToolbox), the GPU's HDR to LowToneMappedHeight, and the
// processor's tone mapping.
const (
	chainSDRMemory = "sdr_memory"
	chainSDRGPU    = "sdr_gpu"
	chainHDRMemory = "hdr_memory"
	chainHDRGPU    = "hdr_gpu"
	chainHDRLow    = "hdr_low"
	chainProcessor = "processor"
)

// timingWindow is how long a timing is kept, and counts towards the best
// recent one.
const timingWindow = 7 * 24 * time.Hour

// busyShare is the share of its best recent timing under which a chain
// tells a busy server: the whole timing is then left out.
const busyShare = 0.6

// timingsVersion is the version of the timings file, to bump when the
// chains timed change: a file of another version is ignored.
const timingsVersion = 1

// keptTimings bounds how many timings the file keeps, of every GPU.
const keptTimings = 100

// chainTimings are what a timing found: how many times real time each
// chain it timed converted, by chain, 0 when it failed; and the
// deinterlacers that deinterlaced frames kept on the GPU, nil when it did
// not try them.
type chainTimings struct {
	Speeds        map[string]float64 `json:"speeds"`
	Deinterlacers []string           `json:"deinterlacers"`
}

// set keeps how fast chain converted the sample in d, or that it failed.
func (t *chainTimings) set(chain string, d time.Duration, err error) {
	if t.Speeds == nil {
		t.Speeds = map[string]float64{}
	}
	t.Speeds[chain] = speedOf(d, err)
}

// gpuTimed reports whether a chain of the GPU's converted the sample in
// t: the processor's tone mapping and the chains that failed do not
// count.
func (t chainTimings) gpuTimed() bool {
	for chain, speed := range t.Speeds {
		if chain != chainProcessor && speed > 0 {
			return true
		}
	}
	return false
}

// keptGPUTiming reports whether a timing of hw's conversion chains in
// which one converted is kept from the last 7 days (see UseTimings).
func (m *Manager) keptGPUTiming(hw *Hardware) bool {
	if !m.timings.on() {
		return false
	}
	best, _ := m.timings.best(m.logger, m.timingKey(hw), m.retime.now())
	return best.gpuTimed()
}

// below lists the chains t timed under busyShare of their best in best,
// as "hdr_gpu 0.4x (best 1.3x)", failures included: none when the timing
// does not tell a busy server.
func (t chainTimings) below(best chainTimings) []string {
	var below []string
	for _, chain := range slices.Sorted(maps.Keys(t.Speeds)) {
		if speed, most := t.Speeds[chain], best.Speeds[chain]; most > 0 && speed < busyShare*most {
			timed := "failed"
			if speed > 0 {
				timed = speedText(speed)
			}
			below = append(below, chain+" "+timed+" (best "+speedText(most)+")")
		}
	}
	return below
}

// apply is hw, as detection found it, converting as the timings t say
// (see measure): SDR kept on the GPU when that chain converted faster
// than through memory, with the deinterlacers t found; HDR on NVIDIA's
// GPU decoded into Vulkan frames on the same terms; and how fast the
// HDR chain taken converted, then to LowToneMappedHeight when under real
// time.
func (t chainTimings) apply(hw Hardware) Hardware {
	speed := t.Speeds
	hw.Resident = speed[chainSDRGPU] > 0 && speed[chainSDRGPU] > speed[chainSDRMemory]
	hw.Deinterlacers = nil
	if hw.Resident || hw.ToneMapping && hw.Method != "cuda" {
		hw.Deinterlacers = slices.Clone(t.Deinterlacers)
	}
	hw.VulkanDecoding, hw.HDRSpeed, hw.LowHDRSpeed = false, speed[chainHDRGPU], 0
	if hw.Method == "cuda" {
		hw.VulkanDecoding = speed[chainHDRGPU] > 0 && speed[chainHDRGPU] > speed[chainHDRMemory]
		if !hw.VulkanDecoding {
			hw.HDRSpeed = speed[chainHDRMemory]
		}
	}
	if hw.HDRSpeed > 0 && hw.HDRSpeed < realTime {
		hw.LowHDRSpeed = speed[chainHDRLow]
	}
	return hw
}

// untimed is hw as detection found it, before any timing.
func (hw Hardware) untimed() Hardware {
	hw.Resident, hw.Deinterlacers, hw.VulkanDecoding, hw.HDRSpeed, hw.LowHDRSpeed = false, nil, false, 0, 0
	return hw
}

// timingKey is what timings hold for: the GPU method, its device and
// encoders, the GPU and its driver as the system names them, empty when
// it does not, the FFmpeg build, the processor's model, empty when the
// system does not name it, and the number of processors. Timings of
// another key do not count.
type timingKey struct {
	Method     string `json:"method"`
	Device     string `json:"device,omitempty"`
	Encoders   string `json:"encoders,omitempty"`
	GPU        string `json:"gpu,omitempty"`
	Driver     string `json:"driver,omitempty"`
	FFmpeg     string `json:"ffmpeg"`
	CPU        string `json:"cpu,omitempty"`
	Processors int    `json:"processors"`
}

// keptTiming is a timing in the file, with what it holds for and when it
// ended.
type keptTiming struct {
	timingKey
	At time.Time `json:"at"`
	chainTimings
}

// timingsFile is the file of the timings kept.
type timingsFile struct {
	Version int          `json:"version"`
	Timings []keptTiming `json:"timings"`
}

// timingStore keeps the timings of the last timingWindow in a file of the
// data folder, so that a start on a busy server follows the best recent
// ones (see UseTimings). Its zero value keeps none.
type timingStore struct {
	path string
	// identify tells what a timing of a GPU, nil for none, holds for, in
	// place of the system, and wrote hears each write of the file and how
	// it ended, for tests.
	identify func(hw *Hardware) timingKey
	wrote    func(error)
	mu       sync.Mutex
	loaded   bool
	kept     []keptTiming
	// builds and gpus are the FFmpeg builds by path, and the GPUs and
	// drivers by method and device, asked once.
	builds map[string]string
	gpus   map[string][2]string
}

// UseTimings keeps the timings in the file at path, in the data folder:
// Polyfin then follows the best of the last 7 days, and leaves out a
// timing made while the server was busy (see Retime). It is called
// before SelectHardware.
func (m *Manager) UseTimings(path string) {
	m.timings.mu.Lock()
	defer m.timings.mu.Unlock()
	m.timings.path, m.timings.loaded, m.timings.kept = path, false, nil
}

// on reports whether timings are kept.
func (s *timingStore) on() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path != ""
}

// load reads the file once; a missing, unreadable or foreign one counts
// as empty. s.mu is held.
func (s *timingStore) load(logger *slog.Logger) {
	if s.loaded {
		return
	}
	s.loaded = true
	var data []byte
	err := retryShared(func() (err error) {
		data, err = os.ReadFile(s.path)
		return err
	})
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	var file timingsFile
	if err == nil {
		err = json.Unmarshal(data, &file)
	}
	if err == nil && file.Version != timingsVersion {
		err = fmt.Errorf("version %d, not %d", file.Version, timingsVersion)
	}
	if err != nil {
		logger.Info("The timings kept are ignored", "file", s.path, "error", err)
		return
	}
	s.kept = file.Timings
}

// best is the best timing of each chain timed for key within timingWindow
// of now, with the deinterlacers of the last that tried them, and how
// many timings it comes from.
func (s *timingStore) best(logger *slog.Logger, key timingKey, now time.Time) (chainTimings, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(logger)
	best, n, tried := chainTimings{Speeds: map[string]float64{}}, 0, time.Time{}
	for _, kept := range s.kept {
		if kept.timingKey != key || now.Sub(kept.At) > timingWindow {
			continue
		}
		n++
		for chain, speed := range kept.Speeds {
			best.Speeds[chain] = max(best.Speeds[chain], speed)
		}
		if kept.Deinterlacers != nil && !kept.At.Before(tried) {
			best.Deinterlacers, tried = slices.Clone(kept.Deinterlacers), kept.At
		}
	}
	return best, n
}

// add keeps found, a timing for key that ended at now, and writes the
// file again without the timings past timingWindow, through a file
// renamed in its place. A timing of the GPU's chains in which none
// converted is not kept, so that it never stands for a good one (see
// gpuTimed); the processor's is, failed or not.
func (s *timingStore) add(logger *slog.Logger, key timingKey, found chainTimings, now time.Time) {
	if _, processor := found.Speeds[chainProcessor]; !processor && !found.gpuTimed() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load(logger)
	s.kept = slices.DeleteFunc(append(s.kept, keptTiming{timingKey: key, At: now.UTC(), chainTimings: found}), func(kept keptTiming) bool {
		return now.Sub(kept.At) > timingWindow
	})
	if extra := len(s.kept) - keptTimings; extra > 0 {
		s.kept = slices.Delete(s.kept, 0, extra)
	}
	err := s.write()
	if s.wrote != nil {
		s.wrote(err)
	}
	if err != nil {
		logger.Warn("The timings could not be kept", "file", s.path, "error", err)
	}
}

// write writes the timings kept into a file renamed in place of the
// file, so that a crash leaves either. s.mu is held.
func (s *timingStore) write() error {
	data, err := json.MarshalIndent(timingsFile{Version: timingsVersion, Timings: s.kept}, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), "."+filepath.Base(s.path)+"-*")
	if err != nil {
		return err
	}
	if _, err = temp.Write(append(data, '\n')); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = retryShared(func() error { return os.Rename(temp.Name(), s.path) })
	}
	if err != nil {
		_ = os.Remove(temp.Name())
	}
	return err
}

// sharedWait bounds how long retryShared tries again.
const sharedWait = 500 * time.Millisecond

// retryShared runs op, again for up to sharedWait while it fails because
// another program holds the file, as Windows tells: an antivirus or an
// indexer reading it, which a rename in its place or a read meets.
func retryShared(op func() error) error {
	delay := 2 * time.Millisecond
	for deadline := time.Now().Add(sharedWait); ; delay = min(2*delay, 50*time.Millisecond) {
		err := op()
		if err == nil || !heldElsewhere(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
	}
}

// removeAllShared removes a folder of segments, trying again a moment
// while another program holds a file in it, as Windows tells: a player
// still reading a segment, or an antivirus.
func removeAllShared(dir string) error {
	return retryShared(func() error { return os.RemoveAll(dir) })
}

// timingKey is what timings of hw, the GPU conversions run on, nil for
// none, hold for (see timingKey).
func (m *Manager) timingKey(hw *Hardware) timingKey {
	s := &m.timings
	if s.identify != nil {
		return s.identify(hw)
	}
	key, ffmpeg := timingKey{Method: "none", CPU: processorModel(), Processors: runtime.NumCPU()}, m.ffmpeg
	if hw != nil {
		key.Method, key.Device, key.Encoders = hw.Method, hw.Device, strings.Join(hw.Encoders, ",")
		if hw.FFmpeg != "" {
			ffmpeg = hw.FFmpeg
		}
		s.mu.Lock()
		gpu, ok := s.gpus[hw.Method+" "+hw.Device]
		s.mu.Unlock()
		if !ok {
			gpu[0], gpu[1] = m.systemGPU(hw)
			s.mu.Lock()
			if s.gpus == nil {
				s.gpus = map[string][2]string{}
			}
			s.gpus[hw.Method+" "+hw.Device] = gpu
			s.mu.Unlock()
		}
		key.GPU, key.Driver = gpu[0], gpu[1]
	}
	s.mu.Lock()
	build, ok := s.builds[ffmpeg]
	s.mu.Unlock()
	if !ok {
		build = ffmpegBuild(ffmpeg)
		s.mu.Lock()
		if s.builds == nil {
			s.builds = map[string]string{}
		}
		s.builds[ffmpeg] = build
		s.mu.Unlock()
	}
	key.FFmpeg = build
	return key
}

// ffmpegBuild names the build of the FFmpeg at path: the version it
// writes, and a hash of all that -version writes, its configuration
// included; empty when it does not run.
func ffmpegBuild(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "-version").Output()
	if err != nil {
		return ""
	}
	// "ffmpeg version n8.0-… Copyright …": the third word is the version.
	first, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(first)
	sum := sha256.Sum256(out)
	if len(fields) < 3 {
		return hex.EncodeToString(sum[:4])
	}
	return fields[2] + " " + hex.EncodeToString(sum[:4])
}

// processorModel names the processor as the system does (see
// systemProcessor), asked once.
var processorModel = sync.OnceValue(systemProcessor)

// cpuinfoModel names the processor that /proc/cpuinfo describes in
// info: its "model name", as x86 and some ARM kernels write it, else
// the board's "Model", as on a Raspberry Pi, else the "CPU implementer"
// and "CPU part" codes other ARM kernels write; empty without any.
func cpuinfoModel(info string) string {
	fields := map[string]string{}
	for line := range strings.Lines(info) {
		name, value, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if _, seen := fields[name]; ok && !seen {
			fields[name] = strings.TrimSpace(value)
		}
	}
	switch {
	case fields["model name"] != "":
		return fields["model name"]
	case fields["Model"] != "":
		return fields["Model"]
	case fields["CPU implementer"] != "" || fields["CPU part"] != "":
		return fields["CPU implementer"] + ":" + fields["CPU part"]
	}
	return ""
}
