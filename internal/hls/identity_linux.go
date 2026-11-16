package hls

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// nvidiaDriver matches the version of NVIDIA's kernel module in
// /proc/driver/nvidia/version.
var nvidiaDriver = regexp.MustCompile(`Kernel Module\s+([0-9][0-9.]*)`)

// vaapiDriver matches the driver FFmpeg names as it opens a VAAPI device,
// such as "Intel iHD driver for Intel(R) Gen Graphics - 24.1.0 ()".
var vaapiDriver = regexp.MustCompile(`VAAPI driver: (.+?)\.?\s*$`)

// systemGPU names the GPU of hw and its driver as Linux tells them:
// NVIDIA's models and kernel module version from /proc/driver/nvidia; a
// render node's PCI vendor and device, and the driver FFmpeg's VAAPI
// opens it with. Empty for what it cannot read.
func (m *Manager) systemGPU(hw *Hardware) (gpu, driver string) {
	switch hw.Method {
	case "cuda":
		var models []string
		paths, _ := filepath.Glob("/proc/driver/nvidia/gpus/*/information")
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			for line := range strings.Lines(string(data)) {
				if model, ok := strings.CutPrefix(line, "Model:"); ok {
					models = append(models, strings.TrimSpace(model))
				}
			}
		}
		slices.Sort(models)
		if data, err := os.ReadFile("/proc/driver/nvidia/version"); err == nil {
			if match := nvidiaDriver.FindSubmatch(data); match != nil {
				driver = string(match[1])
			}
		}
		return strings.Join(models, ", "), driver
	case "vaapi":
		node := filepath.Join("/sys/class/drm", filepath.Base(hw.Device), "device")
		vendor, _ := os.ReadFile(filepath.Join(node, "vendor"))
		device, _ := os.ReadFile(filepath.Join(node, "device"))
		if len(vendor) > 0 || len(device) > 0 {
			gpu = strings.TrimSpace(string(vendor)) + ":" + strings.TrimSpace(string(device))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-nostdin", "-v", "verbose", "-init_hw_device", "vaapi=va:"+hw.Device,
			"-f", "lavfi", "-i", "nullsrc=s=64x64", "-frames:v", "1", "-f", "null", "-").CombinedOutput()
		for line := range strings.Lines(string(out)) {
			if match := vaapiDriver.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
				driver = match[1]
			}
		}
		return gpu, driver
	}
	return "", ""
}
