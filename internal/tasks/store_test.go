package tasks

import (
	"slices"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// The database keeps triggers and last runs, which a registry loads at its
// next start; an empty list stays empty, a deleted one goes.
func TestDBStoreKeepsTriggersAndRuns(t *testing.T) {
	t.Parallel()
	store := NewStore(migrated.New(t))
	ctx := t.Context()
	triggers := []Trigger{Weekly(time.Friday, 23*time.Hour+45*time.Minute), {Type: IntervalTrigger, Interval: 6 * time.Hour, MaxRuntime: time.Hour}}
	for key, saved := range map[string][]Trigger{"Back": triggers, "Never": {}, "Gone": {Daily(0)}} {
		if err := store.SaveTriggers(ctx, key, saved); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SaveTriggers(ctx, "Back", triggers[1:]); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteTriggers(ctx, "Gone"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadTriggers(ctx)
	if _, gone := loaded["Gone"]; err != nil || gone || !slices.Equal(loaded["Back"], triggers[1:]) || loaded["Never"] == nil || len(loaded["Never"]) != 0 {
		t.Errorf("triggers: %+v %v", loaded, err)
	}
	start := time.Date(2026, 10, 5, 4, 0, 0, 0, time.UTC)
	for _, run := range []Result{{Start: start, End: start.Add(time.Minute), Status: Completed},
		{Start: start.Add(time.Hour), End: start.Add(2 * time.Hour), Status: Failed, Error: "disk full"}} {
		if err := store.SaveRun(ctx, "Back", run); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := store.LoadRuns(ctx)
	if run := runs["Back"]; err != nil || len(runs) != 1 || !run.Start.Equal(start.Add(time.Hour)) || !run.End.Equal(start.Add(2*time.Hour)) ||
		run.Status != Failed || run.Error != "disk full" {
		t.Errorf("runs: %+v %v", runs, err)
	}
	if _, err := store.db.Exec(ctx, `INSERT INTO task_triggers (task_key, triggers) VALUES ('Object', '{}')`); err == nil {
		t.Error("the database took triggers that are not a list")
	}
}
