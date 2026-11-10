package jellyfin

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/tasks"
)

// A task tells apps its triggers as Jellyfin's, which read back as they
// were: a day of the week by name, midnight and the time limit included.
func TestTaskTriggersRoundTripAsJellyfins(t *testing.T) {
	t.Parallel()
	triggers := []tasks.Trigger{tasks.Daily(0), tasks.Weekly(time.Sunday, 4*time.Hour+15*time.Minute), tasks.AtStartup(),
		{Type: tasks.IntervalTrigger, Interval: 6 * time.Hour, MaxRuntime: 90 * time.Minute}}
	info := newTaskInfo(tasks.Info{ID: "id", Key: "BackUpDatabase", Name: "Back up the database", Triggers: triggers})
	raw, err := json.Marshal(info.Triggers)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"Type":"DailyTrigger","TimeOfDayTicks":0},{"Type":"WeeklyTrigger","TimeOfDayTicks":153000000000,"DayOfWeek":"Sunday"},` +
		`{"Type":"StartupTrigger"},{"Type":"IntervalTrigger","IntervalTicks":216000000000,"MaxRuntimeTicks":54000000000}]`
	if string(raw) != want {
		t.Errorf("triggers: %s", raw)
	}
	var read []TaskTriggerInfo
	if err := json.Unmarshal(raw, &read); err != nil {
		t.Fatal(err)
	}
	var back []tasks.Trigger
	for _, r := range read {
		trigger, ok := r.trigger()
		if !ok {
			t.Fatalf("unknown: %+v", r)
		}
		back = append(back, trigger)
	}
	if !slices.Equal(back, triggers) {
		t.Errorf("read back: %+v", back)
	}
}

// A day of the week reads as a name in any case or as 0 to 6; others are
// out of range.
func TestDayOfWeekReadsNamesAndNumbers(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]DayOfWeek{`"Monday"`: 1, `"saturday"`: 6, `0`: 0, `6`: 6, `7`: -1, `"Someday"`: -1} {
		var day DayOfWeek
		if err := json.Unmarshal([]byte(raw), &day); err != nil || day != want {
			t.Errorf("%s: %v %v", raw, day, err)
		}
	}
}
