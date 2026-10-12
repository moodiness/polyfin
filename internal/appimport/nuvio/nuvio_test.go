package nuvio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/appimport"
	"github.com/moodiness/polyfin/internal/appimport/nuvio/nuviotest"
	"github.com/moodiness/polyfin/internal/library"
)

const (
	email    = "someone@example.com"
	password = "correct horse"
)

// backend is a fake Nuvio backend with the sample account (see
// nuviotest.Sample).
func backend(t *testing.T) *nuviotest.Backend {
	t.Helper()
	b := nuviotest.New(t)
	b.Set(func(b *nuviotest.Backend) {
		b.Email, b.Password = email, password
		b.Backup = nuviotest.Backup(1, nuviotest.Sample("https://first.example/u/secret-token/manifest.json",
			"https://second.example/u/other-secret/"))
	})
	return b
}

func at(text string) time.Time {
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		panic(err)
	}
	return t
}

func TestAnAccountIsReadSignedInThenSignedOut(t *testing.T) {
	t.Parallel()
	b := backend(t)
	account, err := New(Options{Version: "1.2.3"}).Read(t.Context(), b.URL+"/", " "+email+" ", password)
	if err != nil {
		t.Fatal(err)
	}
	if account.App != App || len(account.Profiles) != 2 || account.Profiles[0].ID != "1" || account.Profiles[0].Name != "Main" ||
		account.Profiles[1].ID != "2" || account.Profiles[1].Name != "Kids" {
		t.Fatalf("profiles: %+v", account.Profiles)
	}
	main := account.Profiles[0]
	byKey := map[string]appimport.Entry{}
	for _, e := range main.Entries {
		key := e.Label.ID
		if e.Label.Kind == "episode" {
			key += ":" + string(rune('0'+e.Label.Season)) + ":" + string(rune('0'+e.Label.Episode))
		}
		byKey[key] = e
	}
	// The series watched whole, and the titles of a server Nuvio connects
	// to, are left out; a title watched and in the library is one entry.
	if len(main.Entries) != 8 || len(byKey) != 8 {
		t.Errorf("entries: %+v", main.Entries)
	}
	want := map[string]func(appimport.Entry) bool{
		"tt0000001": func(e appimport.Entry) bool {
			return e.Played && e.PlayedAt.Equal(at("2026-09-01T20:00:00Z")) && e.Favorite && e.Resume == nil &&
				e.Title == library.TitleRef{IMDb: "tt0000001"} && e.Label.Name == "First movie"
		},
		// Dated as older apps dated it.
		"tt0100:1:2": func(e appimport.Entry) bool {
			return e.Played && e.PlayedAt.Equal(at("2026-09-04T20:00:00Z")) && e.Resume == nil &&
				e.Title == library.TitleRef{Episode: true, IMDb: "tt0100", Season: 1, Number: 2}
		},
		"tt0000002": func(e appimport.Entry) bool {
			return !e.Played && e.Resume != nil && e.Resume.Position == 30*time.Minute && e.Resume.Runtime == 100*time.Minute &&
				e.Resume.At.Equal(at("2026-10-01T20:00:00Z"))
		},
		"tt0100:1:3": func(e appimport.Entry) bool { return e.Resume != nil && e.Resume.Position == 20*time.Minute },
		// Placed by its video's identifier.
		"tt0100:2:1": func(e appimport.Entry) bool {
			return e.Resume != nil && e.Title == library.TitleRef{Episode: true, IMDb: "tt0100", Season: 2, Number: 1}
		},
		"tt0100": func(e appimport.Entry) bool {
			return e.Favorite && !e.Played && e.Title == library.TitleRef{Series: true, IMDb: "tt0100"} && e.Label.Kind == "series"
		},
		"tmdb:999999": func(e appimport.Entry) bool { return e.Played && e.Title == library.TitleRef{TMDB: 999999} },
		"custom:1": func(e appimport.Entry) bool {
			return e.Played && !appimport.Identified(e.Title) && e.Label.Name == "Home video" && e.Label.Kind == "movie"
		},
	}
	for key, check := range want {
		if e, found := byKey[key]; !found || !check(e) {
			t.Errorf("%s: %+v", key, e)
		}
	}
	// The address of an addon's resources names its manifest; the addon
	// turned off is not offered; Kids uses Main's.
	addons := []appimport.Addon{{Name: "First", ManifestURL: "https://first.example/u/secret-token/manifest.json"},
		{Name: "Second", ManifestURL: "https://second.example/u/other-secret/manifest.json"}}
	for _, p := range account.Profiles {
		if len(p.Addons) != 2 || p.Addons[0] != addons[0] || p.Addons[1] != addons[1] {
			t.Errorf("%s's addons: %+v", p.Name, p.Addons)
		}
	}
	if kids := account.Profiles[1]; len(kids.Entries) != 1 || kids.Entries[0].Title.IMDb != "tt0000009" {
		t.Errorf("Kids: %+v", kids.Entries)
	}

	// The key and the token go in headers; the password in the sign-in's
	// body only; the session ends.
	var paths []string
	for _, r := range b.Requests() {
		paths = append(paths, r.Method+" "+r.Path)
		if strings.Contains(r.Query, password) || strings.Contains(r.Query, "access-token") || strings.Contains(r.Query, nuviotest.Key) ||
			r.Path != "/auth/v1/token" && strings.Contains(r.Body, password) {
			t.Errorf("%s %s?%s sends a secret", r.Method, r.Path, r.Query)
		}
		if r.Path == "/rest/v1/rpc/sync_export_account_backup" && (r.APIKey != nuviotest.Key || r.Bearer == "") {
			t.Errorf("the backup is asked without the key or the token: %+v", r)
		}
	}
	if strings.Join(paths, ", ") != "GET /.well-known/nuvio, POST /auth/v1/token, POST /rest/v1/rpc/sync_export_account_backup, POST /auth/v1/logout" {
		t.Errorf("requests: %v", paths)
	}
	if b.Open() != 0 {
		t.Error("the session is still open")
	}
}

func TestReadingFailsWithWhatWentWrong(t *testing.T) {
	t.Parallel()
	b := backend(t)
	r := New(Options{Version: "1.2.3"})
	if _, err := r.Read(t.Context(), b.URL, email, "wrong"); !errors.Is(err, ErrSignInRefused) {
		t.Errorf("a wrong password: %v", err)
	}
	for _, request := range b.Requests() {
		if request.Path == "/rest/v1/rpc/sync_export_account_backup" {
			t.Error("the backup is asked without signing in")
		}
	}

	// Another version of the backup: refused, signed out still.
	b.Set(func(b *nuviotest.Backend) { b.Backup = nuviotest.Backup(2, map[string]any{}) })
	_, err := r.Read(t.Context(), b.URL, email, password)
	var unsupported *BackupError
	if !errors.As(err, &unsupported) || unsupported.Version != 2 || !errors.Is(err, ErrUnsupportedBackup) ||
		!strings.Contains(err.Error(), "version 2") {
		t.Errorf("version 2: %v", err)
	}
	if b.Open() != 0 {
		t.Error("the session is still open")
	}

	web := httptest.NewServer(http.NotFoundHandler())
	defer web.Close()
	if _, err := r.Read(t.Context(), web.URL, email, password); !errors.Is(err, ErrNotNuvio) {
		t.Errorf("not a Nuvio backend: %v", err)
	}
	web.Close()
	if _, err := r.Read(t.Context(), web.URL, email, password); !errors.Is(err, ErrUnreachable) {
		t.Errorf("nothing answers: %v", err)
	}
	for _, address := range []string{"ftp://nuvio.example", "https://user:secret@nuvio.example", "https://nuvio.example/?a=b"} {
		if _, err := r.Read(t.Context(), address, email, password); !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%q: %v", address, err)
		}
	}
	for typed, want := range map[string]string{"": DefaultBackend, " nuvio.example/ ": "https://nuvio.example",
		"http://192.168.1.10:8000": "http://192.168.1.10:8000"} {
		if got, err := ParseBackend(typed); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", typed, got, err, want)
		}
	}
}

func TestABackendAskingToWaitIsWaitedFor(t *testing.T) {
	t.Parallel()
	b := backend(t)
	b.Set(func(b *nuviotest.Backend) { b.Limited, b.RetryAfter = 1, "1" })
	r := New(Options{Version: "1.2.3"})
	started := time.Now()
	if _, err := r.Read(t.Context(), b.URL, email, password); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(started); waited < time.Second {
		t.Errorf("waited %v", waited)
	}
	// Longer than a sign-in waits: it says so at once.
	b.Set(func(b *nuviotest.Backend) { b.Limited, b.RetryAfter = 1, "120" })
	started = time.Now()
	if _, err := r.Read(t.Context(), b.URL, email, password); !errors.Is(err, ErrRateLimited) || time.Since(started) > 10*time.Second {
		t.Errorf("a long wait: %v after %v", err, time.Since(started))
	}
}

func TestOlderAppsCompactDatesAreRead(t *testing.T) {
	t.Parallel()
	for value, want := range map[int64]int64{
		20260904200000:                         at("2026-09-04T20:00:00Z").UnixMilli(),
		at("2026-09-04T20:00:00Z").UnixMilli(): at("2026-09-04T20:00:00Z").UnixMilli(),
		// Not a date: left as it is.
		20261399000000: 20261399000000,
	} {
		if got := watchedAt(value); got != want {
			t.Errorf("%d: %d, want %d", value, got, want)
		}
	}
}
