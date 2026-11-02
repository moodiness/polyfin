package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/ocr"
	"github.com/moodiness/polyfin/internal/playback"
)

// A picture subtitle track left unread for want of its language's
// Tesseract data is a warning of System › Health, naming the language
// and the file to add, linked to the setting and told to notifications.
func TestHealthNamesPictureSubtitleLanguagesWithoutData(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{MissingLanguages: func(context.Context) []playback.MissingLanguage {
			return []playback.MissingLanguage{{Language: "jpn"}}
		}}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	if len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "subtitle_ocr_language:jpn", "code": "subtitle_ocr_language",
		"tone": "warning", "to": "/settings/conversion#subtitle-ocr", "transient": false, "name": "Japanese", "file": "jpn.traineddata"}) {
		t.Fatalf("problems: %v", found)
	}
	for language, want := range map[string]string{
		"en": "Picture subtitles in Japanese are not read into text: Tesseract has no data for this language. " +
			"Add the file jpn.traineddata to the folder POLYFIN_TESSDATA_DIR names.",
		"fr": "Les sous-titres en images en Japanese ne sont pas lus en texte : Tesseract n’a pas de données pour cette langue. " +
			"Ajoutez le fichier jpn.traineddata au dossier que nomme POLYFIN_TESSDATA_DIR.",
	} {
		if p, _ := found[0].(map[string]any); p != nil {
			problem := healthProblemJSON{Code: p["code"].(string), Name: p["name"].(string), File: p["file"].(string)}
			if got := problem.text(language); got != want {
				t.Errorf("%s: %q", language, got)
			}
		}
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || told[0].Key != "subtitle_ocr_language:jpn" {
		t.Errorf("notifications: %+v %v", told, err)
	}
}

// The Download button of a language's warning has Polyfin download its
// data, checked, into POLYFIN_TESSDATA_DIR: the warning then goes. The
// answer names what kept it from being kept, and only administrators
// may ask.
func TestHealthDownloadsTheDataOfMissingLanguages(t *testing.T) {
	t.Parallel()
	data := []byte("Finnish model")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fin.traineddata":
			_, _ = w.Write(data)
		case "/est.traineddata":
			_, _ = w.Write([]byte("Estonian model, altered"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	listed := map[string]ocr.DataFile{}
	for language, content := range map[string]string{"fin": string(data), "est": "Estonian model", "lav": "Latvian model"} {
		sum := sha256.Sum256([]byte(content))
		listed[language] = ocr.DataFile{Language: language, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}
	}
	tessdata := filepath.Join(t.TempDir(), "tessdata")
	downloader := &ocr.Downloader{Dir: tessdata, URL: server.URL, Files: listed}
	// A file standing where the folder of another downloader would be
	// keeps it from being made.
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	unwritable := &ocr.Downloader{Dir: filepath.Join(blocked, "tessdata"), URL: server.URL, Files: listed}
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{
			// Finnish is missing until its file is in the folder, as the
			// reader lists the folder's languages.
			MissingLanguages: func(context.Context) []playback.MissingLanguage {
				if _, err := os.Stat(filepath.Join(tessdata, "fin.traineddata")); err == nil {
					return nil
				}
				return []playback.MissingLanguage{{Language: "fin", Downloadable: downloader.Downloadable("fin")}}
			},
			DownloadLanguage: func(ctx context.Context, language string) error {
				if language == "lav" {
					return unwritable.Download(ctx, language)
				}
				return downloader.Download(ctx, language)
			},
		}
	})
	user := api.signedIn("user", false)
	if status, _, _ := user.call(http.MethodPost, "/health/languages/fin/download", nil); status != http.StatusForbidden {
		t.Errorf("a user's download: %d", status)
	}
	if _, err := os.Stat(tessdata); !os.IsNotExist(err) {
		t.Fatalf("a user's download made the folder: %v", err)
	}
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	if found, _ := body["problems"].([]any); len(found) != 1 || found[0].(map[string]any)["download"] != true {
		t.Fatalf("problems: %v", found)
	}
	for _, refused := range []struct {
		language string
		status   int
		code     string
		detail   string
	}{
		{"xyz", http.StatusNotFound, "tessdata_unknown_language", ""},
		{"est", http.StatusBadGateway, "tessdata_checksum", ""},
		{"lav", http.StatusInternalServerError, "tessdata_folder", blocked},
	} {
		status, body, _ := administrator.call(http.MethodPost, "/health/languages/"+refused.language+"/download", nil)
		detail, _ := body["detail"].(string)
		if status != refused.status || body["error"] != refused.code || !strings.Contains(detail, refused.detail) {
			t.Errorf("%s: %d %v", refused.language, status, body)
		}
	}
	if entries, _ := os.ReadDir(tessdata); len(entries) != 0 {
		t.Errorf("kept %v", entries)
	}
	if status, body, _ := administrator.call(http.MethodPost, "/health/languages/fin/download", nil); status != http.StatusNoContent {
		t.Fatalf("download: %d %v", status, body)
	}
	if kept, err := os.ReadFile(filepath.Join(tessdata, "fin.traineddata")); err != nil || string(kept) != string(data) {
		t.Errorf("kept %q, %v", kept, err)
	}
	_, body, _ = administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	if found, _ := body["problems"].([]any); len(found) != 0 {
		t.Errorf("problems once downloaded: %v", found)
	}
}
