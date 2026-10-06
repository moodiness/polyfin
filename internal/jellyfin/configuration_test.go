package jellyfin

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// me returns the caller's DTO as /Users/Me answers it, and its raw
// configuration.
func (s testServer) me(t *testing.T, token string) (UserDto, map[string]any) {
	t.Helper()
	status, body := s.call(http.MethodGet, "/Users/Me", app("tv", token), nil)
	if status != http.StatusOK {
		t.Fatalf("/Users/Me: %d %s", status, body)
	}
	var dto UserDto
	var raw struct{ Configuration map[string]any }
	_ = json.Unmarshal(body, &dto)
	_ = json.Unmarshal(body, &raw)
	return dto, raw.Configuration
}

// configure posts a configuration and returns the status.
func (s testServer) configure(t *testing.T, path, token string, configuration any) int {
	t.Helper()
	status, _ := s.call(http.MethodPost, path, app("tv", token), configuration)
	return status
}

func TestUsersWhoNeverSavedHaveANewUsersConfiguration(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	dto, raw := s.me(t, s.signIn("member", "tv"))
	if _, present := raw["AudioLanguagePreference"]; present {
		t.Errorf("AudioLanguagePreference of a new user: %v", raw["AudioLanguagePreference"])
	}
	got := dto.Configuration
	if !got.PlayDefaultAudioTrack || got.SubtitleMode != "Default" || got.SubtitleLanguagePreference != "" || !got.HidePlayedInLatest ||
		!got.RememberAudioSelections || !got.EnableNextEpisodeAutoPlay || got.CastReceiverId != "F007D354" || got.OrderedViews == nil {
		t.Errorf("configuration of a new user: %+v", got)
	}
}

func TestUsersSaveTheirConfiguration(t *testing.T) {
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	token := s.signIn("member", "tv")
	view := "0123456789abcdef0123456789abcdef"
	if status := s.configure(t, "/Users/Configuration", token, map[string]any{
		"AudioLanguagePreference": "fre", "SubtitleLanguagePreference": "eng", "SubtitleMode": "Always",
		"PlayDefaultAudioTrack": false, "RememberSubtitleSelections": false, "HidePlayedInLatest": true,
		"OrderedViews": []string{"01234567-89ab-cdef-0123-456789abcdef"}, "CastReceiverId": "6F511C87",
	}); status != http.StatusNoContent {
		t.Fatalf("saving the caller's configuration: %d", status)
	}
	check := func(source string, got UserConfiguration) {
		t.Helper()
		if got.AudioLanguagePreference == nil || *got.AudioLanguagePreference != "fre" || got.SubtitleLanguagePreference != "eng" ||
			got.SubtitleMode != "Always" || got.PlayDefaultAudioTrack || got.RememberSubtitleSelections ||
			!slices.Equal(got.OrderedViews, []string{view}) || got.CastReceiverId != "6F511C87" {
			t.Errorf("%s: %+v", source, got)
		}
		// Settings left out take a new user's values.
		if !got.RememberAudioSelections || !got.EnableNextEpisodeAutoPlay || got.MyMediaExcludes == nil {
			t.Errorf("%s, settings left out: %+v", source, got)
		}
	}
	dto, _ := s.me(t, token)
	check("/Users/Me", dto.Configuration)
	var byID UserDto
	s.get(t, "/Users/"+member.ID.String(), token, &byID)
	check("/Users/{userId}", byID.Configuration)
	var all []UserDto
	s.get(t, "/Users", token, &all)
	if len(all) != 1 {
		t.Fatalf("/Users: %+v", all)
	}
	check("/Users", all[0].Configuration)
	status, body := s.call(http.MethodPost, "/Users/AuthenticateByName", app("phone", ""),
		map[string]string{"Username": "member", "Pw": "correct horse"})
	var result AuthenticationResult
	if err := json.Unmarshal(body, &result); status != http.StatusOK || err != nil {
		t.Fatalf("sign-in: %d %s", status, body)
	}
	check("AuthenticationResult", result.User.Configuration)

	// Saving again replaces the whole configuration.
	if status := s.configure(t, "/Users/Configuration?userId="+member.ID.String(), token, map[string]any{"subtitleMode": 4}); status != http.StatusNoContent {
		t.Fatalf("saving again: %d", status)
	}
	dto, raw := s.me(t, token)
	if _, present := raw["AudioLanguagePreference"]; present || dto.Configuration.SubtitleMode != "Smart" ||
		!dto.Configuration.PlayDefaultAudioTrack || len(dto.Configuration.OrderedViews) != 0 || dto.Configuration.CastReceiverId != "F007D354" {
		t.Errorf("after saving again: %+v", dto.Configuration)
	}
}

func TestOnlyAdministratorsChangeAnotherUsersConfiguration(t *testing.T) {
	s := newTestServer(t, 10)
	member := s.user("member", nil)
	admin := s.user("admin", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	memberToken, adminToken := s.signIn("member", "tv"), s.signIn("admin", "tv")
	never := map[string]any{"SubtitleMode": "None"}

	for _, path := range []string{"/Users/Configuration?userId=" + admin.ID.String(), "/Users/" + admin.ID.String() + "/Configuration"} {
		if status := s.configure(t, path, memberToken, never); status != http.StatusForbidden {
			t.Errorf("member changing an administrator's configuration with %s: %d", path, status)
		}
	}
	if dto, _ := s.me(t, adminToken); dto.Configuration.SubtitleMode != "Default" {
		t.Errorf("refused change applied: %+v", dto.Configuration)
	}

	if status := s.configure(t, "/Users/"+member.ID.String()+"/Configuration", adminToken, never); status != http.StatusNoContent {
		t.Fatalf("administrator changing a member's configuration: %d", status)
	}
	if dto, _ := s.me(t, memberToken); dto.Configuration.SubtitleMode != "None" {
		t.Errorf("member's configuration after the administrator's change: %+v", dto.Configuration)
	}
	if dto, _ := s.me(t, adminToken); dto.Configuration.SubtitleMode != "Default" {
		t.Errorf("administrator's own configuration changed: %+v", dto.Configuration)
	}
	if status := s.configure(t, "/Users/Configuration?userId="+strings.Repeat("ab", 16), adminToken, never); status != http.StatusNotFound {
		t.Errorf("unknown user: %d", status)
	}
	if status, _ := s.call(http.MethodPost, "/Users/Configuration", app("tv", ""), never); status != http.StatusUnauthorized {
		t.Errorf("without credentials: %d", status)
	}
}

func TestInvalidConfigurationsAreRefused(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("member", nil)
	token := s.signIn("member", "tv")
	post := func(contentType, body string) (int, problemDetails) {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/Users/Configuration", strings.NewReader(body))
		request.Header.Set("Authorization", app("tv", token))
		request.Header.Set("Content-Type", contentType)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var problem problemDetails
		_ = json.NewDecoder(response.Body).Decode(&problem)
		return response.StatusCode, problem
	}
	for name, test := range map[string]struct {
		contentType, body string
		status            int
		key               string
	}{
		"an unknown subtitle mode":  {"application/json", `{"SubtitleMode":"Sometimes"}`, http.StatusBadRequest, "$.SubtitleMode"},
		"a mode out of range":       {"application/json", `{"SubtitleMode":9}`, http.StatusBadRequest, "$.SubtitleMode"},
		"a boolean as a string":     {"application/json", `{"PlayDefaultAudioTrack":"yes"}`, http.StatusBadRequest, "$.PlayDefaultAudioTrack"},
		"an invalid library":        {"application/json", `{"OrderedViews":["library"]}`, http.StatusBadRequest, "$.OrderedViews"},
		"malformed JSON":            {"application/json", `{"SubtitleMode":`, http.StatusBadRequest, "$"},
		"no body":                   {"application/json", ``, http.StatusBadRequest, "userConfig"},
		"a body that is not JSON":   {"text/plain", `{}`, http.StatusUnsupportedMediaType, ""},
		"a valid one, as text/json": {"text/json", `{"subtitlemode":"onlyforced"}`, http.StatusNoContent, ""},
	} {
		status, problem := post(test.contentType, test.body)
		if status != test.status {
			t.Errorf("%s: %d, want %d", name, status, test.status)
			continue
		}
		if _, ok := problem.Errors[test.key]; test.key != "" && !ok {
			t.Errorf("%s: errors %v, want %s", name, problem.Errors, test.key)
		}
	}
	if dto, _ := s.me(t, token); dto.Configuration.SubtitleMode != "OnlyForced" {
		t.Errorf("only the valid configuration is saved: %+v", dto.Configuration)
	}
}

func TestSavedConfigurationMatchesJellyfin(t *testing.T) {
	s := newTestServer(t, 10)
	s.user("viewer", nil)
	token := s.signIn("viewer", "tv")
	// The change scripts/jellyfin-fixtures.sh makes to the viewer's
	// configuration before recording it.
	_, configuration := s.me(t, token)
	configuration["AudioLanguagePreference"] = "fre"
	configuration["SubtitleLanguagePreference"] = "eng"
	configuration["SubtitleMode"] = "Always"
	configuration["PlayDefaultAudioTrack"] = false
	configuration["RememberSubtitleSelections"] = false
	if status := s.configure(t, "/Users/Configuration", token, configuration); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	_, body := s.call(http.MethodGet, "/Users/Me", app("tv", token), nil)
	raw, err := os.ReadFile(filepath.Join("testdata", "jellyfin-12.2", "user-configured.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal(raw, &want)
	_ = json.Unmarshal(body, &got)
	for _, difference := range compareShapes("user-configured", want, got, shapeRules{}) {
		t.Error(difference)
	}
}

func TestLibrariesFollowTheHomeScreenSettings(t *testing.T) {
	s, token, views := browsing(t)
	list := func(path string) []string {
		t.Helper()
		var page QueryResult
		s.get(t, path, token, &page)
		return itemNames(page.Items)
	}
	if got := list("/UserViews"); !slices.Equal(got, []string{"Top", "Shows", "Groups"}) {
		t.Fatalf("libraries before any setting: %v", got)
	}
	id, _ := parseGUID(views["Groups"])
	if status := s.configure(t, "/Users/Configuration", token, map[string]any{
		"OrderedViews":    []string{hyphenated(id), views["Top"]},
		"MyMediaExcludes": []string{views["Shows"]},
	}); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	if got := list("/UserViews"); !slices.Equal(got, []string{"Groups", "Top"}) {
		t.Errorf("ordered libraries without the excluded one: %v", got)
	}
	// The settings screen lists every library.
	if got := list("/UserViews?includeHidden=true"); !slices.Equal(got, []string{"Groups", "Top", "Shows"}) {
		t.Errorf("libraries with the hidden ones: %v", got)
	}
}

func TestPlayedTitlesAmongTheLatestFollowTheSetting(t *testing.T) {
	tr := newTracking(t)
	tr.mark(t, http.MethodPost, "/UserPlayedItems/"+tr.movie)
	latest := func(query string) []string {
		t.Helper()
		var items []BaseItemDto
		tr.get(t, "/Items/Latest?parentId="+tr.views["Top"]+query, tr.token, &items)
		return itemIDs(items)
	}
	if slices.Contains(latest(""), tr.movie) {
		t.Error("a played title among the latest of a new user")
	}
	if status := tr.configure(t, "/Users/Configuration", tr.token, map[string]any{"HidePlayedInLatest": false}); status != http.StatusNoContent {
		t.Fatalf("saving: %d", status)
	}
	if !slices.Contains(latest(""), tr.movie) {
		t.Error("a played title hidden among the latest of a user who shows them")
	}
	if slices.Contains(latest("&isPlayed=false"), tr.movie) {
		t.Error("a played title among the latest unplayed")
	}
}
