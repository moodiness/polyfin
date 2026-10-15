package admin

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

// A picture subtitle track left unread for want of its language's
// Tesseract data is a warning of System › Health, naming the language
// and the file to add, linked to the setting and told to notifications.
func TestHealthNamesPictureSubtitleLanguagesWithoutData(t *testing.T) {
	t.Parallel()
	var options *Options
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{MissingLanguages: func(context.Context) []string { return []string{"jpn"} }}
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
