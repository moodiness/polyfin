package hls

import "golang.org/x/sys/unix"

// systemGPU names the GPU of hw and its driver as macOS tells them: the
// Mac's model, whose GPU is its own, and the release of the system, which
// ships the GPU's driver. Empty for what it cannot read.
func (m *Manager) systemGPU(*Hardware) (gpu, driver string) {
	gpu, _ = unix.Sysctl("hw.model")
	var name unix.Utsname
	if unix.Uname(&name) == nil {
		driver = unix.ByteSliceToString(name.Release[:])
	}
	return gpu, driver
}
