package admin

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/moonfin"
)

// messageIDs lists the identifiers of the messages of an answer.
func messageIDs(body map[string]any) []string {
	messages, _ := body["messages"].([]any)
	ids := make([]string, 0, len(messages))
	for _, m := range messages {
		id, _ := m.(map[string]any)["id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// An administrator writes messages, newest first, edits them in place,
// reorders and deletes them; what Valid refuses is answered 400, and each
// message tells whether it shows now.
func TestMoonfinMessagesAreWrittenOrderedAndDeleted(t *testing.T) {
	t.Parallel()
	api, store := moonfinAPI(t)
	administrator := api.signedIn("administrator", true)
	api.signedIn("member", false)
	memberID := api.userID("member").String()
	base := map[string]any{"title": "Maintenance", "body": "Back **soon**.", "color": "yellow", "delivery": "popup", "audience": "all",
		"users": []string{}, "start": "", "end": "", "linkLabel": "Status", "linkUrl": "https://status.example.com"}
	with := func(changes map[string]any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for k, v := range changes {
			m[k] = v
		}
		return m
	}
	for name, changes := range map[string]map[string]any{
		"no title or body":         {"title": "", "body": "  "},
		"a title too long":         {"title": strings.Repeat("a", moonfin.MaxTitleLength+1)},
		"a body too long":          {"body": strings.Repeat("a", moonfin.MaxBodyLength+1)},
		"an unknown color":         {"color": "purple"},
		"an unknown delivery":      {"delivery": "push"},
		"an unknown audience":      {"audience": "guests"},
		"chosen users, none given": {"audience": "users"},
		"a user that is not one":   {"audience": "users", "users": []string{"alice"}},
		"a date that is not one":   {"start": "tomorrow"},
		"a date without its zone":  {"start": "2026-12-01T09:00:00"},
		"an end before the start":  {"start": "2026-12-02T09:00:00Z", "end": "2026-12-01T09:00:00Z"},
		"an ftp link":              {"linkUrl": "ftp://files.example.com"},
		"a javascript link":        {"linkUrl": "javascript:alert(1)"},
		"a link without a label":   {"linkLabel": ""},
		"a label without a link":   {"linkUrl": ""},
	} {
		if status, body, _ := administrator.call(http.MethodPost, "/moonfin/messages", with(changes)); status != http.StatusBadRequest ||
			body["error"] != "invalid_moonfin_message" {
			t.Errorf("%s: %d %v", name, status, body)
		}
	}

	status, first, _ := administrator.call(http.MethodPost, "/moonfin/messages", base)
	if status != http.StatusCreated || first["title"] != "Maintenance" || first["linkUrl"] != "https://status.example.com" ||
		first["state"] != "shown" || first["createdAt"] == "" {
		t.Fatalf("create: %d %v", status, first)
	}
	status, second, _ := administrator.call(http.MethodPost, "/moonfin/messages", with(map[string]any{"title": "For the member",
		"audience": "users", "users": []string{memberID}, "start": "2099-01-01T08:00:00+01:00", "end": "2099-01-02T08:00:00+01:00",
		"linkLabel": "", "linkUrl": ""}))
	if status != http.StatusCreated || second["state"] != "scheduled" || second["start"] != "2099-01-01T07:00:00Z" ||
		!slices.Equal(second["users"].([]any), []any{memberID}) {
		t.Fatalf("create for the member: %d %v", status, second)
	}
	status, third, _ := administrator.call(http.MethodPost, "/moonfin/messages", with(map[string]any{"title": "Over",
		"start": "2020-01-01T00:00:00Z", "end": "2020-01-02T00:00:00Z"}))
	if status != http.StatusCreated || third["state"] != "ended" {
		t.Fatalf("create an ended one: %d %v", status, third)
	}
	ids := []string{first["id"].(string), second["id"].(string), third["id"].(string)}
	if _, list, _ := administrator.call(http.MethodGet, "/moonfin/messages", nil); !slices.Equal(messageIDs(list), []string{ids[2], ids[1], ids[0]}) {
		t.Errorf("newest first: %v", list)
	}
	if status, list, _ := administrator.call(http.MethodPut, "/moonfin/messages/order", map[string]any{"ids": []string{ids[0], ids[1]}}); status != http.StatusOK ||
		!slices.Equal(messageIDs(list), []string{ids[0], ids[1], ids[2]}) {
		t.Errorf("ordered: %d %v", status, list)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/moonfin/messages/order", map[string]any{"ids": []string{"first"}}); status != http.StatusBadRequest {
		t.Errorf("ordered by what is no identifier: %d %v", status, body)
	}
	status, edited, _ := administrator.call(http.MethodPatch, "/moonfin/messages/"+ids[1], with(map[string]any{"title": "For everyone"}))
	if status != http.StatusOK || edited["title"] != "For everyone" || edited["audience"] != "all" || len(edited["users"].([]any)) != 0 ||
		edited["createdAt"] != second["createdAt"] {
		t.Errorf("edited: %d %v", status, edited)
	}
	if _, list, _ := administrator.call(http.MethodGet, "/moonfin/messages", nil); !slices.Equal(messageIDs(list), []string{ids[0], ids[1], ids[2]}) {
		t.Errorf("an edit keeps its place: %v", list)
	}
	if status, body, _ := administrator.call(http.MethodPatch, "/moonfin/messages/"+ids[1], with(map[string]any{"color": "pink"})); status != http.StatusBadRequest ||
		body["error"] != "invalid_moonfin_message" {
		t.Errorf("an invalid edit: %d %v", status, body)
	}
	unknown := accounts.ID{1}.String()
	if status, _, _ := administrator.call(http.MethodPatch, "/moonfin/messages/"+unknown, base); status != http.StatusNotFound {
		t.Errorf("editing an unknown message: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/moonfin/messages/"+ids[2], nil); status != http.StatusNoContent {
		t.Errorf("delete: %d", status)
	}
	if status, _, _ := administrator.call(http.MethodDelete, "/moonfin/messages/"+ids[2], nil); status != http.StatusNotFound {
		t.Errorf("delete again: %d", status)
	}
	if messages, err := store.Messages(t.Context()); err != nil || len(messages) != 2 {
		t.Errorf("kept: %d %v", len(messages), err)
	}
	for range moonfin.MaxMessages - 2 {
		if status, body, _ := administrator.call(http.MethodPost, "/moonfin/messages", base); status != http.StatusCreated {
			t.Fatalf("create: %d %v", status, body)
		}
	}
	if status, body, _ := administrator.call(http.MethodPost, "/moonfin/messages", base); status != http.StatusConflict ||
		body["error"] != "too_many_moonfin_messages" {
		t.Errorf("past the most: %d %v", status, body)
	}
}

// Messages are an administrator's alone, and only while Polyfin serves
// Moonfin's companion.
func TestMoonfinMessagesAreForAdministrators(t *testing.T) {
	t.Parallel()
	api, store := moonfinAPI(t)
	member := api.signedIn("member", false)
	m, err := store.CreateMessage(t.Context(), moonfin.Message{Title: "Hello", Color: moonfin.ColorWhite, Delivery: moonfin.DeliveryInbox,
		Audience: moonfin.AudienceAll, Start: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	message := map[string]any{"title": "Mine", "color": "white", "delivery": "inbox", "audience": "all"}
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/moonfin/messages", nil},
		{http.MethodPost, "/moonfin/messages", message},
		{http.MethodPut, "/moonfin/messages/order", map[string]any{"ids": []string{}}},
		{http.MethodPatch, "/moonfin/messages/" + m.ID.String(), message},
		{http.MethodDelete, "/moonfin/messages/" + m.ID.String(), nil},
	} {
		if status, _, _ := member.call(c.method, c.path, c.body); status != http.StatusForbidden {
			t.Errorf("a member's %s %s: %d", c.method, c.path, status)
		}
		if status, _, _ := api.browser().call(c.method, c.path, c.body); status != http.StatusUnauthorized {
			t.Errorf("signed out %s %s: %d", c.method, c.path, status)
		}
	}
	if messages, err := store.Messages(t.Context()); err != nil || len(messages) != 1 || messages[0].Title != "Hello" {
		t.Errorf("messages: %+v %v", messages, err)
	}

	without := newTestAPI(t, 10)
	administrator := without.signedIn("administrator", true)
	if status, _, _ := administrator.call(http.MethodGet, "/moonfin/messages", nil); status != http.StatusNotFound {
		t.Errorf("without the companion: %d", status)
	}
}
