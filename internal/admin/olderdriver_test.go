package admin

import (
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moodiness/polyfin/internal/hls"
)

// The FFmpeg for NVIDIA drivers older than 610 that could not be
// downloaded, or with which NVENC failed too, is a warning of System ›
// Health with why, linked to the transcoder and told to notifications;
// it goes while that FFmpeg downloads, and once it is used.
func TestHealthTellsTheFFmpegForOlderDriversFailing(t *testing.T) {
	t.Parallel()
	var options *Options
	var status atomic.Value
	status.Store(hls.OlderDriverStatus{State: "failed", Error: "the download does not match its pinned checksum"})
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{OlderDriverFFmpeg: func() hls.OlderDriverStatus { return status.Load().(hls.OlderDriverStatus) }}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	problems := func() []any {
		t.Helper()
		_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
		found, _ := body["problems"].([]any)
		return found
	}
	if found := problems(); len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "nvidia_older_ffmpeg", "code": "nvidia_older_ffmpeg",
		"tone": "warning", "to": "/system/health#transcoder", "transient": false, "failure": "download",
		"error": "the download does not match its pinned checksum"}) {
		t.Fatalf("problems: %v", found)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || !strings.Contains(told[0].Text, "could not be downloaded: the download does not match its pinned checksum") {
		t.Errorf("notifications: %+v %v", told, err)
	}

	status.Store(hls.OlderDriverStatus{State: "unusable", Error: "OpenEncodeSessionEx failed"})
	if found := problems(); len(found) != 1 || found[0].(map[string]any)["failure"] != "encode" {
		t.Errorf("NVENC failing with it: %v", found)
	}
	for _, state := range []string{"downloading", "used", ""} {
		status.Store(hls.OlderDriverStatus{State: state})
		if found := problems(); len(found) != 0 {
			t.Errorf("%q: %v", state, found)
		}
	}
}
