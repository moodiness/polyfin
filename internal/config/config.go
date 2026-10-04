// Package config reads Polyfin's runtime settings from the environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// DefaultListen is the port Jellyfin clients try first when no port is given.
const DefaultListen = ":8096"

// Config holds the settings needed to start the server.
type Config struct {
	// DatabaseURL is a PostgreSQL connection string. It contains credentials:
	// never log it.
	DatabaseURL string
	// Listen is the TCP address of the HTTP server, as host:port.
	Listen   string
	LogLevel slog.Level
	// FFprobe and FFmpeg are the executables, paths or names looked up in
	// PATH.
	FFprobe string
	FFmpeg  string
	// CacheDir keeps the blocks of the sources being played, and CacheSize
	// bounds the space they take, in bytes.
	CacheDir  string
	CacheSize int64
	// Acceleration is the GPU video is converted on: auto, the first of
	// NVIDIA and VAAPI that works, nvenc, vaapi, or none.
	Acceleration string
	// VAAPIDevice is the render node VAAPI opens, such as
	// /dev/dri/renderD128; empty tries each in turn.
	VAAPIDevice string
	// Segments are the databases asked where titles' intros and credits
	// are, theintrodb and introdb, the preferred first; empty asks none.
	Segments []string
	// FontsDir holds the fallback fonts apps load to render subtitles, read
	// with the folders within.
	FontsDir string
	// RecordingsDir is the folder Live TV recordings are written to; empty
	// leaves recording off.
	RecordingsDir string
}

// defaultFontsDir is the system font folder, which the Docker image fills
// with DejaVu.
const defaultFontsDir = "/usr/share/fonts"

// defaultCacheSize is 10 GB: a few movies, read again for seeks and
// restarts without downloading them again.
const defaultCacheSize = 10_000_000_000

// minCacheSize leaves room for the blocks of a few sources read at once.
const minCacheSize = 256 << 20

// Load reads the configuration through getenv, normally os.Getenv.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL:  strings.TrimSpace(getenv("POLYFIN_DATABASE_URL")),
		Listen:       strings.TrimSpace(getenv("POLYFIN_LISTEN")),
		LogLevel:     slog.LevelInfo,
		FFprobe:      strings.TrimSpace(getenv("POLYFIN_FFPROBE")),
		FFmpeg:       strings.TrimSpace(getenv("POLYFIN_FFMPEG")),
		CacheDir:     strings.TrimSpace(getenv("POLYFIN_CACHE_DIR")),
		CacheSize:    defaultCacheSize,
		Acceleration: strings.ToLower(strings.TrimSpace(getenv("POLYFIN_HWACCEL"))),
		VAAPIDevice:  strings.TrimSpace(getenv("POLYFIN_VAAPI_DEVICE")),
		FontsDir:     strings.TrimSpace(getenv("POLYFIN_FONTS_DIR")),
	}
	if cfg.FontsDir == "" {
		cfg.FontsDir = defaultFontsDir
	}
	if cfg.FFprobe == "" {
		cfg.FFprobe = "ffprobe"
	}
	if cfg.FFmpeg == "" {
		cfg.FFmpeg = "ffmpeg"
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = filepath.Join(os.TempDir(), "polyfin")
	}
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("POLYFIN_DATABASE_URL is required"))
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	} else if err := validateListen(cfg.Listen); err != nil {
		errs = append(errs, fmt.Errorf("POLYFIN_LISTEN: %w", err))
	}
	if level := strings.TrimSpace(getenv("POLYFIN_LOG_LEVEL")); level != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(level)); err != nil {
			errs = append(errs, fmt.Errorf("POLYFIN_LOG_LEVEL: %q is not one of debug, info, warn, error", level))
		}
	}
	if raw := strings.TrimSpace(getenv("POLYFIN_CACHE_SIZE")); raw != "" {
		size, err := parseSize(raw)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("POLYFIN_CACHE_SIZE: %w", err))
		case size < minCacheSize:
			errs = append(errs, fmt.Errorf("POLYFIN_CACHE_SIZE: %q is below the minimum of 256 MiB", raw))
		default:
			cfg.CacheSize = size
		}
	}
	switch cfg.Acceleration {
	case "":
		cfg.Acceleration = "auto"
	case "auto", "nvenc", "vaapi", "none":
	default:
		errs = append(errs, fmt.Errorf("POLYFIN_HWACCEL: %q is not one of auto, nvenc, vaapi, none", cfg.Acceleration))
	}
	segments, err := parseSegments(getenv("POLYFIN_SEGMENTS"))
	if err != nil {
		errs = append(errs, fmt.Errorf("POLYFIN_SEGMENTS: %w", err))
	}
	cfg.Segments = segments
	if dir := strings.TrimSpace(getenv("POLYFIN_RECORDINGS_DIR")); dir != "" {
		if err := checkWritableDir(dir); err != nil {
			errs = append(errs, fmt.Errorf("POLYFIN_RECORDINGS_DIR: %w", err))
		} else {
			cfg.RecordingsDir = filepath.Clean(dir)
		}
	}
	return cfg, errors.Join(errs...)
}

// checkWritableDir reports why dir is not a folder Polyfin can write files
// into, by writing one.
func checkWritableDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not an absolute path", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a folder", dir)
	}
	file, err := os.CreateTemp(dir, ".polyfin-check-*")
	if err != nil {
		return fmt.Errorf("%q is not writable: %w", dir, err)
	}
	_ = file.Close()
	return os.Remove(file.Name())
}

// parseSegments reads the segment databases, in order of preference: by
// default both, TheIntroDB first; none for neither.
func parseSegments(text string) ([]string, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
	case "":
		return []string{"theintrodb", "introdb"}, nil
	case "none":
		return nil, nil
	}
	var names []string
	for name := range strings.SplitSeq(text, ",") {
		name = strings.TrimSpace(name)
		if name != "theintrodb" && name != "introdb" {
			return nil, fmt.Errorf("%q is not a list of theintrodb and introdb, or none", text)
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names, nil
}

// units are the size suffixes POLYFIN_CACHE_SIZE accepts, decimal and
// binary.
var units = map[string]float64{
	"": 1, "b": 1,
	"kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12,
	"k": 1 << 10, "m": 1 << 20, "g": 1 << 30, "t": 1 << 40,
	"kib": 1 << 10, "mib": 1 << 20, "gib": 1 << 30, "tib": 1 << 40,
}

// parseSize reads a size such as "10GB", "500 MiB" or "20000000000".
func parseSize(text string) (int64, error) {
	number := strings.TrimRightFunc(strings.ToLower(text), func(r rune) bool { return r >= 'a' && r <= 'z' })
	unit := strings.ToLower(text)[len(number):]
	factor, known := units[unit]
	value, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
	if !known || err != nil || value < 0 || value*factor > math.MaxInt64 {
		return 0, fmt.Errorf("%q is not a size such as 10GB", text)
	}
	return int64(value * factor), nil
}

func validateListen(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%q is not a host:port address", address)
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 {
		return fmt.Errorf("%q has no valid port (1-65535)", address)
	}
	return nil
}
