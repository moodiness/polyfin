package jellyfin

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// A debrid service's error clip, a short video for a release it does not
// hold, is no version of the title: PlaybackInfo goes on to the next
// version, which plays, rather than the clip. The title's runtime is not
// known; the clip's file is far under the release its addon announced.
func TestAnErrorClipGivesWayToTheNextVersion(t *testing.T) {
	t.Parallel()
	probe := newFakeProbe(t, true)
	data, err := os.ReadFile(probe.path + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	// 5 seconds and 50 kB, where the addon announces 1 MB.
	format := output["format"].(map[string]any)
	format["duration"], format["size"] = "5.000000", "50000"
	data, _ = json.Marshal(output)
	if err := os.WriteFile(probe.path+".json", data, 0o600); err != nil {
		t.Fatal(err)
	}
	p := manyOn(t, newProbingServer(t, 10, probe.path), manyVersionsAddon(t, 2, nil))
	p.waitForEveryVersion()
	p.analyzed(t, p.versions[1], "h264-aac-mp4")

	source := firstSource(t, p.ask(t, p.token, p.movie, p.profile(t, "jellyfin-web-chrome"), nil))
	var outcomes []string
	if decisions := p.handler.PlayDecisions(p.versions[0].Item); len(decisions) > 0 {
		for _, try := range decisions[0].Tries {
			outcomes = append(outcomes, try.Outcome)
		}
	}
	if want := []string{TryUnreadable, TryPlays}; source.ETag != p.versions[1].ID.String() || !slices.Equal(outcomes, want) {
		t.Errorf("chose %s after %v, want %v", source.ETag, outcomes, want)
	}
	if !p.handler.Playback.Failed(p.versions[0].ID) {
		t.Error("the error clip's version is not left out")
	}
}
