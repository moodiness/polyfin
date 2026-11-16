package hls

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// hold opens path as an antivirus or an indexer may, sharing reads only:
// no rename in its place, no write; release closes it.
func hold(t *testing.T, path string) (release func()) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	return func() { _ = windows.CloseHandle(handle) }
}

// The timings file held a moment by another program is still written in
// place once it lets go; held past sharedWait, the timing is not kept and
// the log says so.
func TestTheTimingsFileWaitsForAProgramHoldingIt(t *testing.T) {
	t.Parallel()
	log := &syncBuffer{}
	m := &Manager{logger: slog.New(slog.NewTextHandler(log, nil))}
	path := filepath.Join(t.TempDir(), "gpu-timings.json")
	keep(t, path, kept(nvidiaKey, time.Hour, usualSpeeds))
	m.UseTimings(path)
	m.timings.identify = testKey("580.95")
	release := hold(t, path)
	time.AfterFunc(50*time.Millisecond, release)
	m.timings.add(m.logger, nvidiaKey, chainTimings{Speeds: map[string]float64{chainHDRGPU: 1.2}}, time.Now())
	if got := keptIn(t, path); len(got) != 2 || strings.Contains(log.String(), "could not be kept") {
		t.Errorf("held a moment: kept %+v, logged %s", got, log)
	}

	release = hold(t, path)
	m.timings.add(m.logger, nvidiaKey, chainTimings{Speeds: map[string]float64{chainHDRGPU: 1.1}}, time.Now())
	release()
	if !strings.Contains(log.String(), `msg="The timings could not be kept"`) || len(keptIn(t, path)) != 2 {
		t.Errorf("held throughout: kept %+v, logged %s", keptIn(t, path), log)
	}
}
