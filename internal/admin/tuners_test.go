package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/iptv/tunertest"
)

// A tuner is the server's: only an administrator finds and adds one, to
// the server's sources, and has it scan for channels; its page shows its
// model and tuner count.
func TestOnlyAdministratorsAddTuners(t *testing.T) {
	t.Parallel()
	tuner := tunertest.Start(t, tunertest.Options{DeviceID: 0x1234ABCD, Name: "Attic Tuner", Model: "HDTC-2US", Tuners: 2,
		Channels: []tunertest.Channel{{Number: "5.1", Name: "KSTL"}, {Number: "9.1", Name: "Locked", DRM: true}},
		Scanned:  []tunertest.Channel{{Number: "5.1", Name: "KSTL"}, {Number: "6.1", Name: "Found Later"}}})
	// Discovery asks the tuner, never the local network.
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) { o.IPTV.UseTunerDiscovery(0, tuner.Discovery) })
	member := api.signedIn("member", false)
	administrator := api.signedIn("administrator", true)
	body := map[string]any{"kind": "hdhomerun", "url": tuner.URL}
	for _, scope := range []string{"shared", "me"} {
		if status, answer, _ := member.call(http.MethodPost, "/scopes/"+scope+"/iptv", body); status != http.StatusForbidden || answer["error"] != "forbidden" {
			t.Errorf("a member's tuner in %s: %d %v", scope, status, answer)
		}
	}
	if status, answer, _ := administrator.call(http.MethodPost, "/scopes/me/iptv", body); status != http.StatusForbidden || answer["error"] != "forbidden" {
		t.Errorf("an administrator's own tuner: %d %v", status, answer)
	}
	if status, _, _ := member.call(http.MethodGet, "/tuners", nil); status != http.StatusForbidden {
		t.Errorf("tuners found by a member: %d", status)
	}

	status, added, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", body)
	if status != http.StatusCreated {
		t.Fatalf("an administrator's tuner: %d %v", status, added)
	}
	id := added["id"].(string)
	source, _ := added["source"].(map[string]any)
	state, _ := source["tuner"].(map[string]any)
	if state["device"] != "1234ABCD" || state["model"] != "HDTC-2US" || state["tuners"] != float64(2) || source["channels"] != float64(1) {
		t.Errorf("the tuner's source: %v", added)
	}
	if status, answer, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv", body); status != http.StatusConflict || answer["error"] != "tuner_added" {
		t.Errorf("the same tuner added twice: %d %v", status, answer)
	}
	status, found, _ := administrator.call(http.MethodGet, "/tuners", nil)
	items, _ := found["items"].([]any)
	if status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["source"] != id || items[0].(map[string]any)["name"] != "Attic Tuner" {
		t.Errorf("tuners found: %d %v", status, found)
	}

	if status, _, _ := member.call(http.MethodPost, "/scopes/shared/iptv/"+id+"/scan", nil); status != http.StatusForbidden {
		t.Errorf("a scan by a member: %d", status)
	}
	if status, answer, _ := administrator.call(http.MethodPost, "/scopes/shared/iptv/"+id+"/scan", nil); status != http.StatusAccepted || tuner.Scans() != 1 {
		t.Fatalf("a scan: %d %v, %d scans", status, answer, tuner.Scans())
	}
	// The line-up is read again once the tuner's scan ended, a poll later.
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		_, answer, _ := administrator.call(http.MethodGet, "/scopes/shared/iptv/"+id+"/channels", nil)
		if answer["total"] == float64(2) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the line-up after the scan: %v", answer)
		}
	}
}
