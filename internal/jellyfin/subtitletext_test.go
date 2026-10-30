package jellyfin

import (
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/speech"
)

// An ASS subtitle synced across a cut keeps its script, cut as its cues
// are: the events of the scene the version lacks are left out, and those
// after it are shifted by the cut's offset.
func TestASyncedScriptIsCutAsItsCues(t *testing.T) {
	t.Parallel()
	text, err := readSubtitleText([]byte("[Script Info]\nScriptType: v4.00+\n\n[Events]\n" +
		"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:10.00,0:00:12.00,Default,,0,0,0,,Before\n" +
		"Dialogue: 0,0:01:05.00,0:01:07.00,Default,,0,0,0,,Lacking\n" +
		"Dialogue: 0,0:02:10.00,0:02:12.00,Default,,0,0,0,,After\n"))
	if err != nil || text.script == nil {
		t.Fatalf("read %+v: %v", text, err)
	}
	timing := speech.Timing{Ratio: 1, Offset: time.Second, Cuts: []speech.Cut{{Start: time.Minute, End: 2 * time.Minute, Offset: -time.Minute}}}
	retimed := text.retimed(timing)
	want := []struct {
		start time.Duration
		text  string
	}{{11 * time.Second, "Before"}, {70 * time.Second, "After"}}
	if len(retimed.script.Events) != len(want) || len(retimed.cues) != len(want) {
		t.Fatalf("events %+v, cues %+v", retimed.script.Events, retimed.cues)
	}
	for i, w := range want {
		event, cue := retimed.script.Events[i], retimed.cues[i]
		if event.Start != w.start || !strings.HasSuffix(event.Rest, w.text) || cue.Start != w.start {
			t.Errorf("event %d starts at %s (%s), its cue at %s, want %s (%s)", i, event.Start, event.Rest, cue.Start, w.start, w.text)
		}
	}
}
