package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

// database is the one variable every configuration needs.
const database = "postgresql://polyfin@db/polyfin"

// with is the variables of values, with the database URL.
func with(values map[string]string) func(string) string {
	all := map[string]string{"POLYFIN_DATABASE_URL": database}
	for name, value := range values {
		all[name] = value
	}
	return env(all)
}

func TestLoadAcceptsExplicitSettings(t *testing.T) {
	cfg, err := Load(with(map[string]string{
		"POLYFIN_LISTEN":       "[::1]:9000",
		"POLYFIN_LOG_LEVEL":    "DEBUG",
		"POLYFIN_HWACCEL":      " VAAPI ",
		"POLYFIN_VAAPI_DEVICE": "/dev/dri/renderD129",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "[::1]:9000" || !cfg.Environment.DetailedLog || cfg.Environment.Hardware != "vaapi" ||
		cfg.Environment.VAAPIDevice != "/dev/dri/renderD129" || len(cfg.Warnings) != 0 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	// Without a choice, the settings keep theirs.
	if cfg, err := Load(with(nil)); err != nil || cfg.Environment.Hardware != "" || cfg.Environment.DetailedLog {
		t.Errorf("without variables: %+v, %v", cfg.Environment, err)
	}
	// Another level than debug leaves the detailed log as it is.
	for _, level := range []string{"info", "warn", "error"} {
		if cfg, err := Load(with(map[string]string{"POLYFIN_LOG_LEVEL": level})); err != nil || cfg.Environment.DetailedLog || len(cfg.Warnings) != 0 {
			t.Errorf("%s: %+v, %v", level, cfg.Environment, err)
		}
	}
}

func TestFontsComeFromTheSystemFolderUnlessSet(t *testing.T) {
	for value, want := range map[string]string{"": "/usr/share/fonts", " /fonts ": "/fonts"} {
		cfg, err := Load(with(map[string]string{"POLYFIN_FONTS_DIR": value}))
		if err != nil || cfg.FontsDir != want {
			t.Errorf("POLYFIN_FONTS_DIR %q: %q, %v", value, cfg.FontsDir, err)
		}
	}
}

func TestWebClientComesFromTheImageFolderUnlessSet(t *testing.T) {
	for value, want := range map[string]string{"": DefaultWebDir, " /srv/jellyfin-web ": "/srv/jellyfin-web"} {
		cfg, err := Load(with(map[string]string{"POLYFIN_WEB_DIR": value}))
		if err != nil || cfg.WebDir != want {
			t.Errorf("POLYFIN_WEB_DIR %q: %q, %v", value, cfg.WebDir, err)
		}
	}
}

func TestLoadReportsEveryInvalidSetting(t *testing.T) {
	_, err := Load(env(map[string]string{
		"POLYFIN_LISTEN":   "8096",
		"POLYFIN_DATA_DIR": "data",
	}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"POLYFIN_DATABASE_URL", "POLYFIN_LISTEN", "POLYFIN_DATA_DIR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

// The variables copied into the settings never stop the start: a value
// that is not valid is left out, with a warning.
func TestCopiedVariablesOnlyWarn(t *testing.T) {
	invalid := map[string]string{
		"POLYFIN_LOG_LEVEL":      "verbose",
		"POLYFIN_CACHE_SIZE":     "lots",
		"POLYFIN_HWACCEL":        "qsv",
		"POLYFIN_SEGMENTS":       "theintrodb,other",
		"POLYFIN_VAAPI_DEVICE":   "/dev/dri/card0",
		"POLYFIN_RECORDINGS_DIR": "recordings",
		"POLYFIN_BACKUP_DIR":     "/backups\x01",
	}
	cfg, err := Load(with(invalid))
	if err != nil {
		t.Fatal(err)
	}
	if e := cfg.Environment; e.Hardware != "" || e.Segments != nil || e.CacheSizeGB != 0 || e.DetailedLog || e.VAAPIDevice != "" ||
		e.RecordingsFolder != "" || e.BackupFolder != "" {
		t.Errorf("invalid values copied: %+v", e)
	}
	for name := range invalid {
		if !slices.ContainsFunc(cfg.Warnings, func(err error) bool { return strings.HasPrefix(err.Error(), name+":") }) {
			t.Errorf("no warning for %s: %v", name, cfg.Warnings)
		}
	}
	if len(cfg.Warnings) != len(invalid) {
		t.Errorf("warnings: %v", cfg.Warnings)
	}
}

// The variables Polyfin no longer reads are listed when they are set, with
// the section of the settings that holds them now.
func TestRetiredVariablesAreListed(t *testing.T) {
	cfg, err := Load(with(map[string]string{"POLYFIN_CACHE_SIZE": "20GB", "POLYFIN_BACKUP_DIR": "/backups", "POLYFIN_HWACCEL": "nvenc"}))
	if err != nil {
		t.Fatal(err)
	}
	want := []Retired{{"POLYFIN_CACHE_SIZE", "Settings › Playback"}, {"POLYFIN_BACKUP_DIR", "Settings › Backups"}}
	if !slices.Equal(cfg.Retired, want) {
		t.Errorf("retired: %v, want %v", cfg.Retired, want)
	}
	if cfg, _ := Load(with(nil)); len(cfg.Retired) != 0 {
		t.Errorf("retired without variables: %v", cfg.Retired)
	}
}

func TestSegmentDatabasesKeepTheOrderOfPreference(t *testing.T) {
	for text, want := range map[string][]string{
		" IntroDB , theintrodb ":            {"introdb", "theintrodb"},
		"PublicMetaDB,introdb,publicmetadb": {"publicmetadb", "introdb"},
		"introdb,introdb":                   {"introdb"},
	} {
		cfg, err := Load(with(map[string]string{"POLYFIN_SEGMENTS": text}))
		if err != nil || !slices.Equal(cfg.Environment.Segments, want) {
			t.Errorf("%q: %v, %v; want %v", text, cfg.Environment.Segments, err, want)
		}
	}
	// Not set is nil, none an empty list.
	if cfg, _ := Load(with(nil)); cfg.Environment.Segments != nil {
		t.Errorf("not set: %#v", cfg.Environment.Segments)
	}
	if cfg, _ := Load(with(map[string]string{"POLYFIN_SEGMENTS": "none"})); cfg.Environment.Segments == nil || len(cfg.Environment.Segments) != 0 {
		t.Errorf("none: %#v", cfg.Environment.Segments)
	}
	// Turning the databases off cannot be mixed with naming one.
	if cfg, _ := Load(with(map[string]string{"POLYFIN_SEGMENTS": "none,introdb"})); cfg.Environment.Segments != nil || len(cfg.Warnings) != 1 {
		t.Error("none with a database was accepted")
	}
}

func TestCacheSizesReadAsPeopleWriteThem(t *testing.T) {
	for text, want := range map[string]int{
		"":            0,
		"20GB":        20,
		"500 mb":      1,
		"1.5TB":       1500,
		"8GiB":        9,
		"10g":         11,
		"21474836480": 21,
	} {
		cfg, err := Load(with(map[string]string{"POLYFIN_CACHE_SIZE": text}))
		if err != nil || cfg.Environment.CacheSizeGB != want || len(cfg.Warnings) != 0 {
			t.Errorf("%q: %d %v %v, want %d", text, cfg.Environment.CacheSizeGB, err, cfg.Warnings, want)
		}
	}
	for _, text := range []string{"10XB", "-1GB", "100MB", "GB", "3TB"} {
		if cfg, err := Load(with(map[string]string{"POLYFIN_CACHE_SIZE": text})); err != nil || cfg.Environment.CacheSizeGB != 0 || len(cfg.Warnings) != 1 {
			t.Errorf("%q: %d %v %v", text, cfg.Environment.CacheSizeGB, err, cfg.Warnings)
		}
	}
}

func TestLoadRejectsUnusablePorts(t *testing.T) {
	for _, listen := range []string{":0", ":65536", ":http", "host:"} {
		if _, err := Load(with(map[string]string{"POLYFIN_LISTEN": listen})); err == nil {
			t.Errorf("POLYFIN_LISTEN=%q was accepted", listen)
		}
	}
}

// Polyfin keeps its files in POLYFIN_DATA_DIR, by default a folder of the
// system's temporary folder, and its cache in a folder within.
func TestDataFolder(t *testing.T) {
	cfg, err := Load(with(nil))
	if want := filepath.Join(os.TempDir(), "polyfin"); err != nil || cfg.DataDir != want || cfg.CacheDir != filepath.Join(want, "cache") {
		t.Errorf("default: %q %q %v", cfg.DataDir, cfg.CacheDir, err)
	}
	cfg, err = Load(with(map[string]string{"POLYFIN_DATA_DIR": " /data/ "}))
	if err != nil || cfg.DataDir != "/data" || cfg.CacheDir != "/data/cache" {
		t.Errorf("set: %q %q %v", cfg.DataDir, cfg.CacheDir, err)
	}
	for _, relative := range []string{"data", "./data"} {
		if _, err := Load(with(map[string]string{"POLYFIN_DATA_DIR": relative})); err == nil || !strings.Contains(err.Error(), "POLYFIN_DATA_DIR") {
			t.Errorf("%q: %v", relative, err)
		}
	}
}
