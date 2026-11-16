//go:build !linux && !windows && !darwin

package hls

// systemGPU names no GPU nor driver on systems Polyfin offers no GPU
// method on.
func (m *Manager) systemGPU(*Hardware) (gpu, driver string) {
	return "", ""
}
