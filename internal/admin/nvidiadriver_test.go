package admin

import (
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

// An NVIDIA driver older than FFmpeg's NVENC needs is a warning of System
// › Health naming the version needed, linked to the transcoder and told
// to notifications; it goes once the GPU's detection no longer finds it.
func TestHealthTellsAnNVIDIADriverTooOld(t *testing.T) {
	t.Parallel()
	var options *Options
	var driver atomic.Value
	driver.Store("610.00")
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{NVIDIADriver: func() string { return driver.Load().(string) }}
		options = o
	})
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/health/problems?language=en", nil)
	found, _ := body["problems"].([]any)
	if len(found) != 1 || !reflect.DeepEqual(found[0], map[string]any{"key": "nvidia_driver", "code": "nvidia_driver", "tone": "warning",
		"to": "/system/health#transcoder", "transient": false, "name": "610.00"}) {
		t.Fatalf("problems: %v", found)
	}
	told, err := HealthProblems(*options)(t.Context())
	if err != nil || len(told) != 1 || told[0].Key != "nvidia_driver" || !strings.Contains(told[0].Text, "610.00") {
		t.Errorf("notifications: %+v %v", told, err)
	}

	driver.Store("")
	if _, body, _ := administrator.call(http.MethodGet, "/health/problems", nil); len(body["problems"].([]any)) != 0 {
		t.Errorf("problems with a driver recent enough: %v", body["problems"])
	}
}
