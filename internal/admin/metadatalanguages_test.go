package admin

import (
	"net/http"
	"slices"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// languages reads the languages an answer lists.
func languages(body map[string]any) []string {
	list, _ := body["languages"].([]any)
	result := []string{}
	for _, language := range list {
		text, _ := language.(string)
		result = append(result, text)
	}
	return result
}

func TestServerAddonsTakeALanguageUsersChooseFrom(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	reader, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "reader", Password: "correct horse"})
	if err != nil {
		t.Fatal(err)
	}

	status, addon, _ := administrator.call(http.MethodPost, "/scopes/shared/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated || addon["language"] != "" {
		t.Fatalf("installing: %d %v", status, addon)
	}
	path := "/scopes/shared/addons/" + addon["id"].(string)
	for _, bad := range []string{"French", "FR", "f", "fr-fr", "fr_FR", "frenc"} {
		if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"language": bad}); status != http.StatusBadRequest || body["error"] != "invalid_language" {
			t.Errorf("language %q: %d %v", bad, status, body)
		}
	}
	if status, body, _ := member.call(http.MethodPatch, path, map[string]any{"language": "fr"}); status != http.StatusForbidden {
		t.Errorf("a member naming the language: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"language": "fr"}); status != http.StatusOK || body["language"] != "fr" {
		t.Fatalf("naming the language: %d %v", status, body)
	}

	// A user's own addon takes none.
	status, own, _ := administrator.call(http.MethodPost, "/scopes/me/addons", map[string]string{"manifestUrl": localAddon(t)})
	if status != http.StatusCreated {
		t.Fatalf("installing an own addon: %d %v", status, own)
	}
	if status, body, _ := administrator.call(http.MethodPatch, "/scopes/me/addons/"+own["id"].(string), map[string]any{"language": "de"}); status != http.StatusBadRequest || body["error"] != "invalid_language" {
		t.Errorf("an own addon's language: %d %v", status, body)
	}

	// Every signed-in user reads the languages.
	if status, body, _ := member.call(http.MethodGet, "/metadata-languages", nil); status != http.StatusOK || !slices.Equal(languages(body), []string{"fr"}) {
		t.Errorf("languages: %d %v", status, body)
	}

	// A user's metadata language is one of them, or the server's.
	users := "/users/" + reader.ID.String()
	if status, body, _ := administrator.call(http.MethodPatch, users, map[string]any{"metadataLanguage": "de"}); status != http.StatusBadRequest || body["error"] != "invalid_language" {
		t.Errorf("a language no addon has: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPatch, users, map[string]any{"metadataLanguage": "fr"}); status != http.StatusOK || body["metadataLanguage"] != "fr" {
		t.Errorf("choosing French for a user: %d %v", status, body)
	}

	// Users choose their own.
	if status, body, _ := member.call(http.MethodGet, "/account/metadata-language", nil); status != http.StatusOK || body["language"] != "" ||
		!slices.Equal(languages(body), []string{"fr"}) {
		t.Errorf("own language: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/metadata-language", map[string]any{"language": "es"}); status != http.StatusBadRequest || body["error"] != "invalid_language" {
		t.Errorf("an own language no addon has: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/metadata-language", map[string]any{"language": "fr"}); status != http.StatusOK || body["language"] != "fr" {
		t.Errorf("choosing French: %d %v", status, body)
	}
	if status, body, _ := member.call(http.MethodPut, "/account/metadata-language", map[string]any{"language": ""}); status != http.StatusOK || body["language"] != "" {
		t.Errorf("back to the server's: %d %v", status, body)
	}

	// An addon's language cleared leaves the list.
	if status, body, _ := administrator.call(http.MethodPatch, path, map[string]any{"language": ""}); status != http.StatusOK || body["language"] != "" {
		t.Errorf("clearing the language: %d %v", status, body)
	}
	if _, body, _ := member.call(http.MethodGet, "/metadata-languages", nil); len(languages(body)) != 0 {
		t.Errorf("languages once cleared: %v", body)
	}
}
