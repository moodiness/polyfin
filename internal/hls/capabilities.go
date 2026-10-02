package hls

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// capabilities are the encoders and filters of the installed FFmpeg.
type capabilities struct {
	encoders, filters []string
}

// probe asks FFmpeg which encoders and filters it has. An FFmpeg that
// cannot be run has none.
func probe(ffmpeg string) capabilities {
	return capabilities{encoders: list(ffmpeg, "-encoders"), filters: list(ffmpeg, "-filters")}
}

// list runs FFmpeg with an option listing components, one a line after
// their flags, and returns their names.
func list(ffmpeg, option string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, ffmpeg, "-hide_banner", option).Output()
	if err != nil {
		return nil
	}
	var names []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		// " V....D libx264 …" or " ... zscale V->V …": flags, then the name.
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && strings.Trim(fields[0], ".|ABCDEFGHIJKLMNOPQRSTUVWXYZ") == "" {
			names = append(names, fields[1])
		}
	}
	return names
}

// Encoders lists the encoders of the installed FFmpeg.
func (m *Manager) Encoders() []string {
	return m.can.encoders
}

// HasFilters reports whether the installed FFmpeg has every filter named.
func (m *Manager) HasFilters(names ...string) bool {
	for _, name := range names {
		if !slices.Contains(m.can.filters, name) {
			return false
		}
	}
	return true
}
