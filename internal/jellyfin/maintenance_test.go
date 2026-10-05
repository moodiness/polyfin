package jellyfin

import (
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/tasks"
)

// A daily task tells apps Jellyfin's DailyTrigger, at its hour.
func TestADailyTaskHasADailyTrigger(t *testing.T) {
	info := newTaskInfo(tasks.Info{ID: "id", Key: "BackUpDatabase", Name: "Back up the database", Daily: new(4)})
	if len(info.Triggers) != 1 || info.Triggers[0].Type != "DailyTrigger" || info.Triggers[0].TimeOfDayTicks != int64(4*time.Hour/100) {
		t.Errorf("triggers: %+v", info.Triggers)
	}
}
