package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// metadataAddon serves the manifest of a metadata addon named name.
func metadataAddon(t *testing.T, id, name string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": "`+id+`", "name": "`+name+`", "version": "1.0.0", "types": ["movie", "series"],
			"resources": ["catalog", {"name": "meta", "types": ["movie", "series"], "idPrefixes": ["tt"]}],
			"catalogs": [{"type": "series", "id": "top", "name": "Top"}]}`)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/manifest.json"
}

// System › Health warns when every metadata addon of the server has a
// language, linking to the first one's page, until one more is left at
// None. A catalog addon without a language does not count.
func TestHealthWarnsWhenEveryMetadataAddonHasALanguage(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	problems := func() []healthProblemJSON {
		t.Helper()
		var answer struct {
			Problems []healthProblemJSON `json:"problems"`
		}
		if status := administrator.raw(http.MethodGet, "/health/problems?language=en", "", &answer); status != http.StatusOK {
			t.Fatalf("problems: %d", status)
		}
		return answer.Problems
	}
	if status, _, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)}); status != http.StatusCreated {
		t.Fatalf("installing the catalog addon: %d", status)
	}
	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": metadataAddon(t, "meta", "Meta")})
	if status != http.StatusCreated {
		t.Fatalf("installing: %d %v", status, addon)
	}
	if got := problems(); len(got) != 0 {
		t.Errorf("without a language: %+v", got)
	}
	id := addon["id"].(string)
	if status, body, _ := administrator.call(http.MethodPatch, "/scopes/shared/addons/"+id, map[string]any{"language": "fr"}); status != http.StatusOK {
		t.Fatalf("naming the language: %d %v", status, body)
	}
	want := healthProblemJSON{Key: "metadata_languages", Code: problemMetadataLanguages, Tone: "warning", To: "/sources/shared/" + id, Name: "Meta"}
	got := problems()
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("with every metadata addon in a language: %+v", got)
	}
	for language, text := range map[string]string{
		"en": "Meta: every metadata addon of the server has a language, so they describe titles in their language for every user, whatever their metadata language. " +
			"Keep Language of its descriptions at None on this addon, or on one more metadata addon.",
		"fr": "Meta : chaque addon de métadonnées du serveur a une langue : ils décrivent donc les titres dans leur langue pour tous les utilisateurs, quelle que soit leur langue des métadonnées. " +
			"Laissez Langue de ses descriptions sur Aucune pour cet addon, ou pour un addon de métadonnées de plus.",
	} {
		if got := want.text(language); got != text {
			t.Errorf("%s: %q", language, got)
		}
	}

	// One more metadata addon left at None ends the warning.
	if status, _, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": metadataAddon(t, "other", "Other")}); status != http.StatusCreated {
		t.Fatalf("installing another: %d", status)
	}
	if got := problems(); len(got) != 0 {
		t.Errorf("with a metadata addon without a language: %+v", got)
	}
}
