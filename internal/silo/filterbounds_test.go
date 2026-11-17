package silo

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A filter of more rule groups, or more rules in all, than the bounds is
// refused as a validation problem of its groups.
func TestFilterBoundsRefuseOversizedFilters(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	h.server.options.Genres = func(context.Context, accounts.User, accounts.ID) ([]string, error) { return []string{"Action"}, nil }
	rule := map[string]any{"field": "genre", "op": "is", "value": "Action"}
	group := func(rules int) map[string]any {
		list := make([]any, rules)
		for i := range list {
			list[i] = rule
		}
		return map[string]any{"match": "any", "rules": list}
	}
	groups := func(count, rules int) []any {
		list := make([]any, count)
		for i := range list {
			list[i] = group(rules)
		}
		return list
	}
	for name, body := range map[string]map[string]any{
		"33 groups":            {"source": "query", "groups": groups(maxGroups+1, 1)},
		"513 rules in a group": {"source": "query", "groups": groups(1, maxRules+1)},
		"513 rules in all":     {"source": "query", "groups": groups(3, maxRules/3+1)},
	} {
		w := h.do(http.MethodPost, "/api/v2/catalog/query", body)
		var p struct {
			Errors []struct {
				Location string `json:"location"`
				Code     string `json:"code"`
			} `json:"errors"`
		}
		if w.Code != http.StatusUnprocessableEntity || problemCode(w) != "validation_failed" || json.Unmarshal(w.Body.Bytes(), &p) != nil ||
			len(p.Errors) != 1 || p.Errors[0].Location != "body.groups" || p.Errors[0].Code != "out_of_range" {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if w := h.do(http.MethodPost, "/api/v2/catalog/query", map[string]any{"source": "query", "groups": groups(maxGroups, maxRules/maxGroups)}); w.Code == http.StatusUnprocessableEntity {
		t.Errorf("a filter at the bounds: %d %s", w.Code, w.Body)
	}
}

// Of an any group of genres, only those a library's catalog offers are
// read, each as a listing of its own: genres it does not offer are read
// as none.
func TestFilterBoundsReadOnlyOfferedGenres(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	h.server.options.Genres = func(context.Context, accounts.User, accounts.ID) ([]string, error) { return []string{"Action"}, nil }
	var libraries struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if w := h.do(http.MethodGet, "/api/v2/user/libraries", nil); w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &libraries) != nil ||
		len(libraries.Items) != 1 {
		t.Fatalf("libraries: %d %s", w.Code, w.Body)
	}
	rules := []any{}
	for _, genre := range []string{"Nope 1", "action", "Nope 2", "Nope 3"} {
		rules = append(rules, map[string]any{"field": "genre", "op": "is", "value": genre})
	}
	body := map[string]any{"source": "query", "library_id": libraries.Items[0].ID, "limit": 10,
		"groups": []any{map[string]any{"match": "any", "rules": rules}}}
	if w := h.do(http.MethodPost, "/api/v2/catalog/query", body); w.Code != http.StatusOK {
		t.Fatalf("the query: %d %s", w.Code, w.Body)
	}
	var genres []string
	for _, c := range h.jellyfin.called(http.MethodGet, "/Items") {
		if genre := c.query.Get("Genres"); genre != "" {
			genres = append(genres, genre)
		}
	}
	if len(genres) == 0 || genres[0] != "action" {
		t.Errorf("genres read: %q", genres)
	}
	for _, genre := range genres {
		if genre != "action" {
			t.Errorf("genres read: %q", genres)
			break
		}
	}
}
