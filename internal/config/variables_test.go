package config

import (
	"slices"
	"strings"
	"testing"
)

func TestVariablesShowTheConfigurationWithoutSecrets(t *testing.T) {
	t.Parallel()
	environ := []string{
		"POLYFIN_DATABASE_URL=postgresql://polyfin:s3cret-pw@db.internal:5432/media?sslmode=disable",
		"POLYFIN_CACHE_SIZE=20GB",
		"POLYFIN_API_TOKEN=tok-123",
		"POLYFIN_ADMIN_PASSWORD=hunter2",
		"POLYFIN_SECRET_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"POLYFIN_EXTRA_FEED=https://user:pw@feeds.example/list?key=abc",
		"POLYFIN_NVENC13_TOOLS=/tmp/nvenc13.json",
		"HOME=/root",
	}
	getenv := func(name string) string {
		for _, entry := range environ {
			if key, value, _ := strings.Cut(entry, "="); key == name {
				return value
			}
		}
		return ""
	}
	cfg, err := Load(getenv)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Variable{}
	for _, v := range Variables(environ, cfg) {
		byName[v.Name] = v
		for _, secret := range []string{"s3cret-pw", "polyfin:", "tok-123", "hunter2", "pw@", "abc", "sslmode", "MDEy"} {
			if strings.Contains(v.Value, secret) {
				t.Errorf("%s shows %q: %q", v.Name, secret, v.Value)
			}
		}
	}
	if _, listed := byName["HOME"]; listed {
		t.Error("a variable of another program is listed")
	}
	for name, want := range map[string]Variable{
		"POLYFIN_DATABASE_URL": {Value: "db.internal:5432/media", Set: true, Known: true},
		// Copied into the settings once, it is no longer read.
		"POLYFIN_CACHE_SIZE":     {Value: "20GB", Set: true},
		"POLYFIN_LISTEN":         {Value: ":8096", Known: true},
		"POLYFIN_DATABASE":       {Known: true},
		"POLYFIN_DATA_DIR":       {Value: cfg.DataDir, Known: true},
		"POLYFIN_API_TOKEN":      {Set: true, Hidden: true},
		"POLYFIN_ADMIN_PASSWORD": {Set: true, Hidden: true},
		"POLYFIN_SECRET_KEY":     {Set: true, Hidden: true, Known: true},
		"POLYFIN_EXTRA_FEED":     {Value: "https://feeds.example/…", Set: true},
		// The FFmpeg for older NVIDIA drivers of a test: read when set.
		"POLYFIN_NVENC13_TOOLS": {Value: "/tmp/nvenc13.json", Set: true, Known: true},
	} {
		got := byName[name]
		want.Name = name
		if got != want {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
	for _, retired := range []string{"POLYFIN_HWACCEL", "POLYFIN_SEGMENTS", "POLYFIN_LOG_LEVEL", "POLYFIN_CACHE_DIR"} {
		if _, listed := byName[retired]; listed {
			t.Errorf("%s is listed while not set", retired)
		}
	}
	if location := databaseLocation("host=db.internal port=5433 user=polyfin password=s3cret dbname=media"); location != "db.internal:5433/media" {
		t.Errorf("keyword form: %q", location)
	}
}

// The variables Polyfin takes out of its environment are the database URL
// and those of its own named after a password, a token, a key or a
// secret, which are the values the admin app hides; what the programs
// Polyfin runs need, and another program's variables, are not among them.
func TestCredentialsNameTheVariablesThatHoldOne(t *testing.T) {
	t.Parallel()
	environ := []string{
		"PATH=/usr/local/bin:/usr/bin",
		"HOME=/root",
		"TMPDIR=/tmp",
		"LANG=C.UTF-8",
		"LD_LIBRARY_PATH=/usr/local/nvidia/lib64",
		"LIBVA_DRIVER_NAME=iHD",
		"NVIDIA_VISIBLE_DEVICES=all",
		"CUDA_VISIBLE_DEVICES=0",
		"HTTP_PROXY=http://proxy.example:3128",
		"https_proxy=http://proxy.example:3128",
		"no_proxy=localhost,127.0.0.1",
		"POLYFIN_DATABASE_URL=postgresql://polyfin:s3cret-pw@db.internal:5432/media",
		"POLYFIN_DATABASE=",
		"POLYFIN_SECRET_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"POLYFIN_DATA_DIR=" + systemPath("/var/lib/polyfin"),
		"POLYFIN_LISTEN=:8096",
		"POLYFIN_FFMPEG=/usr/local/bin/ffmpeg",
		"POLYFIN_DESKTOP=1",
		"POLYFIN_NVENC13_TOOLS=/tmp/nvenc13.json",
		"POLYFIN_API_TOKEN=tok-123",
		"Polyfin_Admin_Password=hunter2",
		"POLYFIN_EXTRA_FEED=https://feeds.example/list",
		"SECRET_KEY=of another program",
		"API_TOKEN=of another program",
		`=C:=C:\polyfin`,
	}
	want := []string{"POLYFIN_DATABASE_URL", "POLYFIN_SECRET_KEY", "POLYFIN_API_TOKEN", "Polyfin_Admin_Password"}
	got := Credentials(environ)
	if !slices.Equal(got, want) {
		t.Errorf("Credentials = %q, want %q", got, want)
	}
	cfg, err := Load(func(name string) string {
		for _, entry := range environ {
			if key, value, _ := strings.Cut(entry, "="); key == name {
				return value
			}
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range Variables(environ, cfg) {
		if variable.Set && variable.Hidden && !slices.Contains(got, variable.Name) {
			t.Errorf("%s, whose value the admin app hides, stays in the environment", variable.Name)
		}
	}
	if Credentials(nil) != nil {
		t.Error("an empty environment has credentials")
	}
}
