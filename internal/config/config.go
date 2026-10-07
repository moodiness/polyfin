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

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/secrets"
)

// DefaultListen is the port Jellyfin clients try first when no port is given.
const DefaultListen = ":8096"

// Config holds the settings needed to start the server.
type Config struct {
	// DatabaseURL is a PostgreSQL connection string. It contains credentials:
	// never log it.
	DatabaseURL string
	// Listen is the TCP address of the HTTP server, as host:port.
	Listen string
	// FFprobe and FFmpeg are the executables, paths or names looked up in
	// PATH.
	FFprobe string
	FFmpeg  string
	// DataDir is the folder Polyfin keeps its files in, an absolute path:
	// the cache, and the default recordings and backups folders.
	DataDir string
	// CacheDir, <data>/cache, keeps the blocks of the sources being played,
	// their remuxes and the guides being read; it is emptied at start.
	CacheDir string
	// FontsDir holds the fallback fonts apps load to render subtitles, read
	// with the folders within.
	FontsDir string
	// WebDir is the folder of jellyfin-web, the web client served at
	// /web/. A folder without its index.html leaves the web client off.
	WebDir string
	// SecretKey is the key the keys, secrets and tokens stored in the
	// database are sealed with, nil for none. It is a secret: never log it.
	SecretKey []byte
	// Environment is what the variables that were options before they were
	// settings give, copied into the settings once (see
	// accounts.Store.AdoptEnvironment) and never read after that. A value
	// that is not valid is left out, and Warnings tell why.
	Environment accounts.Environment
	Warnings    []error
	// Retired are the variables set that the settings hold now, which
	// Polyfin no longer reads.
	Retired []Retired
}

// Retired is a variable Polyfin no longer reads, and the section of the
// settings that holds its value now.
type Retired struct {
	Name    string
	Section string
}

// retiredVariables are the variables copied into the settings once since
// POLYFIN_DATA_DIR, with the section of the settings holding each.
var retiredVariables = []Retired{
	{"POLYFIN_CACHE_SIZE", "Settings › Playback"},
	{"POLYFIN_LOG_LEVEL", "Settings › Diagnostics"},
	{"POLYFIN_VAAPI_DEVICE", "Settings › Conversion"},
	{"POLYFIN_RECORDINGS_DIR", "Settings › Recordings"},
	{"POLYFIN_BACKUP_DIR", "Settings › Backups"},
}

// defaultFontsDir is the system font folder, which the Docker image fills
// with DejaVu.
const defaultFontsDir = "/usr/share/fonts"

// DefaultWebDir is the folder the Docker image puts jellyfin-web in.
const DefaultWebDir = "/usr/share/polyfin/jellyfin-web"

// DefaultDataDir is the folder Polyfin keeps its files in when
// POLYFIN_DATA_DIR is not set: a polyfin folder in the system's temporary
// folder.
func DefaultDataDir() string {
	return filepath.Join(os.TempDir(), "polyfin")
}

// Load reads the configuration through getenv, normally os.Getenv. Only
// the variables still read can make it fail; those copied into the
// settings give warnings instead (see Config.Warnings).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(getenv("POLYFIN_DATABASE_URL")),
		Listen:      strings.TrimSpace(getenv("POLYFIN_LISTEN")),
		FFprobe:     strings.TrimSpace(getenv("POLYFIN_FFPROBE")),
		FFmpeg:      strings.TrimSpace(getenv("POLYFIN_FFMPEG")),
		DataDir:     strings.TrimSpace(getenv("POLYFIN_DATA_DIR")),
		FontsDir:    strings.TrimSpace(getenv("POLYFIN_FONTS_DIR")),
		WebDir:      strings.TrimSpace(getenv("POLYFIN_WEB_DIR")),
	}
	if cfg.WebDir == "" {
		cfg.WebDir = DefaultWebDir
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
	var errs []error
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("POLYFIN_DATABASE_URL is required"))
	}
	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	} else if err := validateListen(cfg.Listen); err != nil {
		errs = append(errs, fmt.Errorf("POLYFIN_LISTEN: %w", err))
	}
	switch {
	case cfg.DataDir == "":
		cfg.DataDir = DefaultDataDir()
	case !filepath.IsAbs(cfg.DataDir):
		errs = append(errs, fmt.Errorf("POLYFIN_DATA_DIR: %q is not an absolute path", cfg.DataDir))
	default:
		cfg.DataDir = filepath.Clean(cfg.DataDir)
	}
	cfg.CacheDir = filepath.Join(cfg.DataDir, "cache")
	if raw := strings.TrimSpace(getenv("POLYFIN_SECRET_KEY")); raw != "" {
		key, err := secrets.ParseKey(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("POLYFIN_SECRET_KEY: %w", err))
		}
		cfg.SecretKey = key
	}
	cfg.readEnvironment(getenv)
	return cfg, errors.Join(errs...)
}

// readEnvironment reads the variables copied into the settings once into
// cfg.Environment, leaving out, with a warning, the values that are not
// valid, and lists those of them Polyfin no longer reads that are set.
func (cfg *Config) readEnvironment(getenv func(string) string) {
	warn := func(name string, err error) { cfg.Warnings = append(cfg.Warnings, fmt.Errorf("%s: %w", name, err)) }
	env := &cfg.Environment
	switch hardware := strings.ToLower(strings.TrimSpace(getenv("POLYFIN_HWACCEL"))); {
	case hardware == "":
	case slices.Contains(accounts.HardwareAccelerations, hardware):
		env.Hardware = hardware
	default:
		warn("POLYFIN_HWACCEL", fmt.Errorf("%q is not one of auto, nvenc, vaapi, none", hardware))
	}
	if segments, err := parseSegments(getenv("POLYFIN_SEGMENTS")); err != nil {
		warn("POLYFIN_SEGMENTS", err)
	} else {
		env.Segments = segments
	}
	if raw := strings.TrimSpace(getenv("POLYFIN_CACHE_SIZE")); raw != "" {
		if size, err := parseSize(raw); err != nil {
			warn("POLYFIN_CACHE_SIZE", err)
		} else if gigabytes := math.Round(float64(size) / 1e9); gigabytes < accounts.MinCacheSizeGB || gigabytes > accounts.MaxCacheSizeGB {
			warn("POLYFIN_CACHE_SIZE", fmt.Errorf("%q is not between %d and %d GB", raw, accounts.MinCacheSizeGB, accounts.MaxCacheSizeGB))
		} else {
			env.CacheSizeGB = int(gigabytes)
		}
	}
	if raw := strings.TrimSpace(getenv("POLYFIN_LOG_LEVEL")); raw != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(raw)); err != nil {
			warn("POLYFIN_LOG_LEVEL", fmt.Errorf("%q is not one of debug, info, warn, error", raw))
		} else {
			env.DetailedLog = level <= slog.LevelDebug
		}
	}
	if device := strings.TrimSpace(getenv("POLYFIN_VAAPI_DEVICE")); accounts.ValidVAAPIDevice(device) {
		env.VAAPIDevice = device
	} else {
		warn("POLYFIN_VAAPI_DEVICE", fmt.Errorf("%q is not a render node such as /dev/dri/renderD128", device))
	}
	for _, folder := range []struct {
		name string
		into *string
	}{{"POLYFIN_RECORDINGS_DIR", &env.RecordingsFolder}, {"POLYFIN_BACKUP_DIR", &env.BackupFolder}} {
		raw := strings.TrimSpace(getenv(folder.name))
		if cleaned, ok := accounts.CleanFolder(raw); ok {
			*folder.into = cleaned
		} else {
			warn(folder.name, fmt.Errorf("%q is not an absolute path of at most %d bytes", raw, accounts.MaxFolderBytes))
		}
	}
	for _, retired := range retiredVariables {
		if strings.TrimSpace(getenv(retired.Name)) != "" {
			cfg.Retired = append(cfg.Retired, retired)
		}
	}
}

// PrepareFolder reports why dir is not a folder Polyfin can write files
// into, by writing one; create makes it first, with its parents, as for a
// default folder under the data folder.
func PrepareFolder(dir string, create bool) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%q is not an absolute path", dir)
	}
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
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

// segmentDatabases are the segment databases POLYFIN_SEGMENTS names, in the
// default order of preference.
var segmentDatabases = []string{"theintrodb", "introdb", "publicmetadb"}

// parseSegments reads the segment databases, in order of preference: nil
// when text is empty, an empty list for none.
func parseSegments(text string) ([]string, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	switch text {
	case "":
		return nil, nil
	case "none":
		return []string{}, nil
	}
	var names []string
	for name := range strings.SplitSeq(text, ",") {
		name = strings.TrimSpace(name)
		if !slices.Contains(segmentDatabases, name) {
			return nil, fmt.Errorf("%q is not a list of theintrodb, introdb and publicmetadb, or none", text)
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
