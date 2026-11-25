package silo

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// The realtime events socket is off, as the contract says a server
// without it answers: the apps read its document, and leave the socket.
func TestEventsCapabilitiesSayTheSocketIsOff(t *testing.T) {
	t.Parallel()
	h := newHarness(t, browsing)
	w := h.do(http.MethodGet, "/api/v2/events/capabilities", nil)
	var doc map[string]any
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &doc) != nil {
		t.Fatalf("events capabilities: %d %s", w.Code, w.Body)
	}
	for _, key := range []string{"schema_version", "subscribe_frame", "declared_channels", "subscribe_grace_period_seconds",
		"max_requested_channels", "channels", "revision", "state", "allowed"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("%s is missing: %s", key, w.Body)
		}
	}
	if doc["state"] != "unsupported" || doc["allowed"] != false || doc["subscribe_frame"] != false {
		t.Errorf("events capabilities: %s", w.Body)
	}
	if err := silotest.CheckUnsupported(w.Body.Bytes(), "/api/v2/events/capabilities"); err != nil {
		t.Error(err)
	}
	if strings.Contains(h.log.String(), "does not serve") {
		t.Errorf("logged as not served: %s", h.log)
	}
}

// rangedPlayable is playable, its file served by ranges as a Matroska
// file.
func rangedPlayable(w http.ResponseWriter, c call) {
	if strings.HasPrefix(c.path, "/Videos/") && strings.HasSuffix(c.path, "/stream") {
		w.Header().Set("Content-Type", "video/x-matroska")
		r, _ := http.NewRequest(http.MethodGet, "/", nil)
		if ranged := c.header.Get("Range"); ranged != "" {
			r.Header.Set("Range", ranged)
		}
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader("media bytes"))
		return
	}
	playable(w, c)
}

// A playback that starts at its resume point, reads its file by ranges, and
// is replanned unchanged, as an app reopening its player asks, plays on
// from the same file at the same address: the file is read again by
// ranges, and the replan neither converts it nor moves it elsewhere.
func TestAnUnchangedReplanKeepsTheFileAsItIs(t *testing.T) {
	t.Parallel()
	h := newHarness(t, rangedPlayable)
	session, d := h.start(map[string]any{"start_position": 1234.5})
	plan, _ := d["playback_plan"].(map[string]any)
	stream, _ := plan["stream"].(map[string]any)
	address, _ := stream["url"].(string)
	if plan["delivery"] != "original_http" || address != "/api/v2/stream/"+session || stream["mime_type"] != "video/mp4" {
		t.Fatalf("plan: %v", plan)
	}
	read := func(when, ranged, want string) {
		t.Helper()
		w := h.do(http.MethodGet, address, nil, "Range", ranged)
		if w.Code != http.StatusPartialContent || w.Header().Get("Content-Range") != want {
			t.Errorf("%s, %s: %d %q", when, ranged, w.Code, w.Header().Get("Content-Range"))
		}
	}
	read("before the replan", "bytes=0-", "bytes 0-10/11")
	read("before the replan", "bytes=6-", "bytes 6-10/11")
	tracks, _ := plan["selected_tracks"].(map[string]any)
	w := h.replanWith(session, "reopen-1", map[string]any{"operation": "track_change", "position_seconds": 1234.5,
		"selected_tracks": tracks})
	var replanned struct {
		Plan struct {
			Delivery string `json:"delivery"`
			Stream   struct {
				URL string `json:"url"`
			} `json:"stream"`
		} `json:"playback_plan"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &replanned) != nil || replanned.Plan.Delivery != "original_http" ||
		replanned.Plan.Stream.URL != address {
		t.Fatalf("replan: %d %s", w.Code, w.Body)
	}
	read("after the replan", "bytes=0-", "bytes 0-10/11")
	read("after the replan", "bytes=6-", "bytes 6-10/11")
}
