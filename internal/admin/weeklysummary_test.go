package admin

import (
	"maps"
	"net/http"
	"testing"
)

// The weekly summary's day and hour default to Monday at 9:00, with their
// bounds; a PUT leaving them out keeps them, and one out of bounds is
// refused with its code.
func TestSettingsWeeklySummary(t *testing.T) {
	api := newTestAPI(t, 10)
	administrator := api.signedIn("administrator", true)
	_, body, _ := administrator.call(http.MethodGet, "/settings", nil)
	bounds, _ := body["bounds"].(map[string]any)
	day, _ := bounds["weeklySummaryDay"].(map[string]any)
	hour, _ := bounds["weeklySummaryHour"].(map[string]any)
	if body["weeklySummaryDay"] != 1.0 || body["weeklySummaryHour"] != 9.0 || day["min"] != 0.0 || day["max"] != 6.0 ||
		hour["min"] != 0.0 || hour["max"] != 23.0 {
		t.Errorf("defaults: %v %v, bounds %v %v", body["weeklySummaryDay"], body["weeklySummaryHour"], day, hour)
	}
	base := map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en"}
	settings := maps.Clone(base)
	settings["weeklySummaryDay"], settings["weeklySummaryHour"] = 0, 23
	if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusOK ||
		body["weeklySummaryDay"] != 0.0 || body["weeklySummaryHour"] != 23.0 {
		t.Fatalf("saving Sunday at 23:00: %d %v", status, body)
	}
	if status, body, _ := administrator.call(http.MethodPut, "/settings", base); status != http.StatusOK ||
		body["weeklySummaryDay"] != 0.0 || body["weeklySummaryHour"] != 23.0 {
		t.Errorf("left out: %d %v", status, body)
	}
	for name, value := range map[string]int{"weeklySummaryDay": 7, "weeklySummaryHour": 24} {
		for _, v := range []int{-1, value} {
			settings := maps.Clone(base)
			settings[name] = v
			code := map[string]string{"weeklySummaryDay": "invalid_weekly_summary_day", "weeklySummaryHour": "invalid_weekly_summary_hour"}[name]
			if status, body, _ := administrator.call(http.MethodPut, "/settings", settings); status != http.StatusBadRequest || body["error"] != code {
				t.Errorf("%s %d: %d %v", name, v, status, body)
			}
		}
	}
	if got := api.store.Settings(); got.WeeklySummaryDay != 0 || got.WeeklySummaryHour != 23 {
		t.Errorf("saved: day %d at %d", got.WeeklySummaryDay, got.WeeklySummaryHour)
	}
}
