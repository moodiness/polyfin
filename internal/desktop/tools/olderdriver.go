package tools

import (
	"context"
	"os"
	"path/filepath"
)

// OlderDriverVariable names a manifest file, JSON as Manifest, that
// replaces the pinned downloads of the FFmpeg for NVIDIA drivers older
// than 610: for development and tests against a local fake.
const OlderDriverVariable = "POLYFIN_NVENC13_TOOLS"

// pinnedOlderDriver are the downloads of the FFmpeg for NVIDIA drivers
// older than 610 of each system, GOOS/GOARCH, where NVIDIA's driver runs
// NVENC: the Docker image's FFmpeg, built for NVENC's API 13.0 by the
// workflow ffmpeg-nvenc13 (scripts/ffmpeg-nvenc13.sh) and published, with
// its sources, as this repository's release ffmpeg-nvenc13.0-<the
// Dockerfile's build tag>. Its license, LICENSE.txt, is kept beside it.
var pinnedOlderDriver = map[string]Manifest{
	"linux/amd64": {Version: "9.0.2-nvenc13.0", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{{
		URL: nvenc13 + "linux64-gpl-shared-9.0-nvenc13.0.tar.xz", Size: 63833132, Format: "tar.xz", Strip: 1,
		SHA256: "6d19ebd3843fd3fff4fcbc1a0e98ba0cb1604ac67a7bedbfe83953075608f35b",
		Keep:   []string{"bin/ffmpeg", "bin/ffprobe", "lib/", "LICENSE.txt"},
	}}},
	"linux/arm64": {Version: "9.0.2-nvenc13.0", FFmpeg: "bin/ffmpeg", FFprobe: "bin/ffprobe", Files: []File{{
		URL: nvenc13 + "linuxarm64-gpl-shared-9.0-nvenc13.0.tar.xz", Size: 50479544, Format: "tar.xz", Strip: 1,
		SHA256: "9e2a8027cf642fc820a3f0b5ef1b1579133121c446b5f12ad312a3a4275ddb2a",
		Keep:   []string{"bin/ffmpeg", "bin/ffprobe", "lib/", "LICENSE.txt"},
	}}},
	"windows/amd64": {Version: "9.0.2-nvenc13.0", FFmpeg: "bin/ffmpeg.exe", FFprobe: "bin/ffprobe.exe", Files: []File{{
		URL: nvenc13 + "win64-gpl-shared-9.0-nvenc13.0.zip", Size: 86266769, Format: "zip", Strip: 1,
		SHA256: "71ae114fa98a55533f8b530a3d15c652c7b884a222d261db191b9685cfb0d98e",
		Keep:   []string{"bin/", "LICENSE.txt"},
	}}},
}

// nvenc13 is where the release of the builds for older NVIDIA drivers of
// the Dockerfile's FFmpeg keeps them.
const nvenc13 = "https://github.com/moodiness/polyfin/releases/download/ffmpeg-nvenc13.0-autobuild-2026-09-30-13-08/ffmpeg-n9.0.2-17-g2a571b6068-"

// OlderDriverManifest is what to download of the FFmpeg for NVIDIA
// drivers older than 610 on goos/goarch: the manifest file the variable
// OlderDriverVariable names, when set, else the pinned one. It reports
// false when there is none, as on systems without NVIDIA's driver.
func OlderDriverManifest(goos, goarch string) (Manifest, bool, error) {
	if file := os.Getenv(OlderDriverVariable); file != "" {
		m, err := Load(file)
		return m, err == nil, err
	}
	m, ok := pinnedOlderDriver[goos+"/"+goarch]
	return m, ok, nil
}

// OlderDriver is the FFmpeg for NVIDIA drivers older than 610, unpacked
// into its own folder of the tools folder dir: nvenc13, which the
// downloads of the other FFmpeg leave alone, as it leaves them.
func OlderDriver(dir string, m Manifest) Tools {
	return Tools{Dir: filepath.Join(dir, "nvenc13"), Manifest: m}
}

// FFmpegReady returns the executable of ffmpeg when it is downloaded
// already.
func (t Tools) FFmpegReady() (string, bool) {
	paths, ok := t.Ready()
	return paths.FFmpeg, ok
}

// FetchFFmpeg downloads, checks and unpacks the tools, and returns the
// executable of ffmpeg.
func (t Tools) FetchFFmpeg(ctx context.Context) (string, error) {
	paths, err := t.Ensure(ctx, nil)
	return paths.FFmpeg, err
}
