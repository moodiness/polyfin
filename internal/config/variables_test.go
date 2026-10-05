package config

import (
	"strings"
	"testing"
)

func TestVariablesShowTheConfigurationWithoutSecrets(t *testing.T) {
	environ := []string{
		"POLYFIN_DATABASE_URL=postgresql://polyfin:s3cret-pw@db.internal:5432/media?sslmode=disable",
		"POLYFIN_CACHE_SIZE=20GB",
		"POLYFIN_API_TOKEN=tok-123",
		"POLYFIN_ADMIN_PASSWORD=hunter2",
		"POLYFIN_SECRET_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"POLYFIN_EXTRA_FEED=https://user:pw@feeds.example/list?key=abc",
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
		"POLYFIN_DATABASE_URL":   {Value: "db.internal:5432/media", Set: true, Known: true},
		"POLYFIN_CACHE_SIZE":     {Value: "20GB", Set: true, Known: true},
		"POLYFIN_LISTEN":         {Value: ":8096", Known: true},
		"POLYFIN_HWACCEL":        {Value: "auto", Known: true},
		"POLYFIN_SEGMENTS":       {Value: "theintrodb,introdb,publicmetadb", Known: true},
		"POLYFIN_API_TOKEN":      {Set: true, Hidden: true},
		"POLYFIN_ADMIN_PASSWORD": {Set: true, Hidden: true},
		"POLYFIN_SECRET_KEY":     {Set: true, Hidden: true, Known: true},
		"POLYFIN_EXTRA_FEED":     {Value: "https://feeds.example/…", Set: true},
	} {
		got := byName[name]
		want.Name = name
		if got != want {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
	if location := databaseLocation("host=db.internal port=5433 user=polyfin password=s3cret dbname=media"); location != "db.internal:5433/media" {
		t.Errorf("keyword form: %q", location)
	}
}
