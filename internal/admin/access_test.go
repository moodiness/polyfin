package admin

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

func TestAdministratorsSetPlaybackAndAccessLimits(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	member := api.signedIn("member", false)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "child", "password": "correct horse"})
	if created["maxPlaybacks"] != 0.0 || created["maxBitrate"] != 0.0 || created["liveTv"] != true ||
		created["syncPlay"] != "CreateAndJoinGroups" || created["remoteControl"] != false {
		t.Fatalf("new user's limits: %v", created)
	}
	// An administrator controls other users' apps unless that is taken away.
	_, admin, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "second", "password": "correct horse", "isAdministrator": true})
	if admin["remoteControl"] != true {
		t.Errorf("new administrator: %v", admin)
	}
	path := "/users/" + created["id"].(string)
	id, _ := accounts.ParseID(created["id"].(string))

	status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{
		"maxPlaybacks": 2, "maxBitrate": 10_000_000, "liveTv": false, "syncPlay": "JoinGroups", "remoteControl": true})
	if status != http.StatusOK || updated["maxPlaybacks"] != 2.0 || updated["maxBitrate"] != 10_000_000.0 || updated["liveTv"] != false ||
		updated["syncPlay"] != "JoinGroups" || updated["remoteControl"] != true {
		t.Fatalf("setting the limits: %d %v", status, updated)
	}
	stored, err := api.store.User(t.Context(), id)
	if err != nil || stored.MaxPlaybacks != 2 || stored.MaxBitrate != 10_000_000 || stored.LiveTv || stored.SyncPlay != accounts.SyncPlayJoin || !stored.RemoteControl {
		t.Errorf("stored: %+v %v", stored, err)
	}
	// A change that leaves them out keeps them.
	status, updated, _ = administrator.call(http.MethodPatch, path, map[string]any{"isHidden": true, "syncPlay": "None"})
	if status != http.StatusOK || updated["maxPlaybacks"] != 2.0 || updated["maxBitrate"] != 10_000_000.0 || updated["liveTv"] != false ||
		updated["syncPlay"] != "None" || updated["remoteControl"] != true {
		t.Errorf("a change without them: %d %v", status, updated)
	}

	for _, refused := range []struct {
		body map[string]any
		code string
	}{
		{map[string]any{"maxPlaybacks": -1}, "invalid_max_playbacks"},
		{map[string]any{"maxPlaybacks": 21}, "invalid_max_playbacks"},
		{map[string]any{"maxBitrate": -1}, "invalid_max_bitrate"},
		{map[string]any{"maxBitrate": 2_147_483_648}, "invalid_max_bitrate"},
		{map[string]any{"syncPlay": "Sometimes"}, "invalid_sync_play"},
		{map[string]any{"syncPlay": "none"}, "invalid_sync_play"},
	} {
		if status, body, _ := administrator.call(http.MethodPatch, path, refused.body); status != http.StatusBadRequest || body["error"] != refused.code {
			t.Errorf("%v: %d %v", refused.body, status, body)
		}
	}
	if user, _ := api.store.User(t.Context(), id); user.MaxPlaybacks != 2 || user.MaxBitrate != 10_000_000 || user.SyncPlay != accounts.SyncPlayNone {
		t.Errorf("stored after refused changes: %+v", user)
	}
	// The bounds themselves are taken.
	if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"maxPlaybacks": 20, "maxBitrate": 2_147_483_647}); status != http.StatusOK ||
		updated["maxPlaybacks"] != 20.0 || updated["maxBitrate"] != 2_147_483_647.0 {
		t.Errorf("the highest limits: %d %v", status, updated)
	}
	if status, _, _ := member.call(http.MethodPatch, path, map[string]any{"maxPlaybacks": 0}); status != http.StatusForbidden {
		t.Errorf("member changing limits: %d", status)
	}
}

func TestAdministratorsMadeLaterKeepTheirRemoteControl(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, created, _ := administrator.call(http.MethodPost, "/users", map[string]any{"name": "helper", "password": "correct horse"})
	path := "/users/" + created["id"].(string)
	if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"isAdministrator": true}); status != http.StatusOK ||
		updated["isAdministrator"] != true || updated["remoteControl"] != false {
		t.Errorf("made an administrator: %d %v", status, updated)
	}
	if status, updated, _ := administrator.call(http.MethodPatch, path, map[string]any{"remoteControl": true, "isAdministrator": false}); status != http.StatusOK ||
		updated["isAdministrator"] != false || updated["remoteControl"] != true {
		t.Errorf("a member allowed to control others: %d %v", status, updated)
	}
}
