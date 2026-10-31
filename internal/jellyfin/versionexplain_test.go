package jellyfin

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/library"
)

// A version an administrator pins comes first, for every user, in item
// details and PlaybackInfo, before the order the criteria give; the
// others keep theirs. A version the app picks still plays. A pin brings
// back no version a rule hides.
func TestAPinnedVersionComesFirst(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	movie, _ := accounts.ParseID(p.movie)
	french := p.versions[3]
	if err := p.handler.PinVersion(t.Context(), p.user, p.user, movie, french.ID); err != nil {
		t.Fatal(err)
	}
	sources := p.detailed(t, p.user, p.token)
	if got := firstWords(sources); !slices.Equal(got, []string{"FRENCH", "English", "Source", "MULTI", "Downloading", "ITA"}) {
		t.Errorf("pinned: %v", got)
	}
	if sources[0].Id != p.movie || sources[0].ETag != french.ID.String() {
		t.Errorf("the pinned version's identifiers: %s %s", sources[0].Id, sources[0].ETag)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = []string{"resolution-high"} })
	sorted, sortedToken := p.another(t, "sorted", "")
	if got := firstWords(p.detailed(t, sorted, sortedToken)); !slices.Equal(got, []string{"FRENCH", "MULTI", "English", "Downloading", "ITA", "Source"}) {
		t.Errorf("pinned, with criteria: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.VersionOrder = nil })

	// Every version plays: a play picking none plays the pinned one, a
	// play picking another plays that one.
	for _, version := range p.versions {
		p.analyzed(t, version, "h264-aac-mp4")
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	if chosen := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.movie})); chosen.ETag != french.ID.String() {
		t.Errorf("a play picking no version: %s, want the pinned %s", chosen.ETag, french.ID)
	}
	if chosen := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": p.versions[0].ID.String()})); chosen.Id != p.versions[0].ID.String() {
		t.Errorf("a play picking the first version: %s", chosen.Id)
	}
	// The plays are kept, the latest first.
	decisions := p.handler.PlayDecisions(movie)
	if len(decisions) != 2 || !decisions[0].Picked || decisions[0].Chosen != p.versions[0].ID || decisions[1].Picked ||
		decisions[1].Chosen != french.ID || len(decisions[1].Tries) == 0 || decisions[1].Tries[0].Outcome != TryPlays ||
		decisions[1].User != "member" || decisions[1].Delivery == "" {
		t.Errorf("decisions: %+v", decisions)
	}

	// A rule hiding the pinned version hides it all the same.
	p.setting(t, func(s *accounts.Settings) { s.HiddenVersionWords = []string{"french"} })
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"English", "Source", "MULTI", "Downloading", "ITA"}) {
		t.Errorf("pinned and hidden: %v", got)
	}
	p.setting(t, func(s *accounts.Settings) { s.HiddenVersionWords = nil })
	if err := p.handler.UnpinVersion(t.Context(), movie); err != nil {
		t.Fatal(err)
	}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"English", "Source", "MULTI", "FRENCH", "Downloading", "ITA"}) {
		t.Errorf("unpinned: %v", got)
	}
}

// A file blocked is in no version list, and plays by no identifier; its
// block lifted, it is back.
func TestABlockedVersionPlaysNowhere(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	movie, _ := accounts.ParseID(p.movie)
	english := p.versions[0]
	p.analyzed(t, english, "h264-aac-mp4")
	block, err := p.handler.BlockVersion(t.Context(), p.user, p.user, movie, english.ID, false, "a fake")
	if err != nil {
		t.Fatal(err)
	}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"Source", "MULTI", "FRENCH", "Downloading", "ITA"}) {
		t.Errorf("blocked: %v", got)
	}
	chrome := p.profile(t, "jellyfin-web-chrome")
	if asked := p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": english.ID.String()}); !asked.refused() {
		t.Errorf("the blocked version asked for: %+v", asked)
	}
	if response, _ := fetchURL(t, p.url+"/Items/"+english.ID.String()+"/Download?ApiKey="+p.token, nil); response.StatusCode == http.StatusOK {
		t.Errorf("the blocked version downloaded")
	}
	if err := p.handler.UnblockVersion(t.Context(), block.ID); err != nil {
		t.Fatal(err)
	}
	if chosen := firstSource(t, p.ask(t, p.token, p.movie, chrome, map[string]any{"MediaSourceId": english.ID.String()})); chosen.ETag != english.ID.String() {
		t.Errorf("unblocked: %s", chosen.ETag)
	}
}

// explainedVersions names each stream of an explanation by the first word
// of its name, with its step, place, rule and pin.
type explainedVersion struct {
	word, step, rule, detail string
	place                    int
	pinned                   bool
}

func explainedVersions(explanation VersionsExplanation) []explainedVersion {
	var result []explainedVersion
	for _, source := range explanation.Sources {
		for _, stream := range source.Streams {
			word, _, _ := strings.Cut(stream.Name, " ")
			result = append(result, explainedVersion{word: word, step: stream.Step, rule: stream.Rule, detail: stream.Detail,
				place: stream.Place, pinned: stream.Pinned})
		}
	}
	return result
}

// The explanation tells, for a user, why each version is left out: it
// failed lately, a hide rule hides it, the user's quality group leaves it
// out; and the place of the others, the pinned one first. Hide rules that
// would hide every version hide none, which it tells too.
func TestTheExplanationTellsWhyEachVersionIsLeftOut(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	movie, _ := accounts.ParseID(p.movie)
	grouped := p.testServer.user("grouped", func(c *accounts.UserChanges) { c.QualityGroup = new(1080) })
	p.setting(t, func(s *accounts.Settings) { s.HiddenVersionWords = []string{"downloading"} })
	// ffprobe is missing: the analysis of ITA fails, and is kept.
	if _, err := p.handler.Playback.Analyze(t.Context(), p.versions[5]); err == nil || !p.handler.Playback.Failed(p.versions[5].ID) {
		t.Fatalf("the analysis of ITA: %v", err)
	}
	if err := p.handler.PinVersion(t.Context(), p.user, grouped, movie, p.versions[3].ID); err != nil {
		t.Fatal(err)
	}
	explanation, err := p.handler.ExplainVersions(t.Context(), grouped, movie)
	if err != nil {
		t.Fatal(err)
	}
	got := explainedVersions(explanation)
	want := []explainedVersion{
		{word: "English", step: StepOffered, place: 2},
		{word: "Source", step: StepOffered, place: 3},
		{word: "MULTI", step: StepGroup},
		{word: "FRENCH", step: StepOffered, place: 1, pinned: true},
		{word: "Downloading", step: StepHidden, rule: "words", detail: "downloading"},
		{word: "ITA", step: StepFailed},
	}
	if len(got) != len(want) {
		t.Fatalf("explained: %+v", got)
	}
	for i := range want {
		if want[i].step == StepFailed {
			if got[i].step != StepFailed || got[i].detail == "" {
				t.Errorf("%s: %+v, want failed with why", want[i].word, got[i])
			}
			continue
		}
		if got[i] != want[i] {
			t.Errorf("%s: %+v, want %+v", want[i].word, got[i], want[i])
		}
	}
	if explanation.Group != 1080 || explanation.KeptAll || explanation.Closest || explanation.Pin == nil || explanation.Pin.Missing {
		t.Errorf("explanation: group %d, kept all %v, closest %v, pin %+v", explanation.Group, explanation.KeptAll, explanation.Closest,
			explanation.Pin)
	}
	// Its labels as read come with each stream.
	if multi := explanation.Sources[0].Streams[2]; multi.Height != 2160 || multi.Labels.Codec != "hevc" || multi.Labels.Range != library.HighRange ||
		multi.Facts.Size != 60e9 {
		t.Errorf("MULTI as read: %+v", multi)
	}

	// Hiding every version, the rules hide none.
	p.setting(t, func(s *accounts.Settings) { s.HiddenVersionWords = []string{"movie"} })
	explanation, err = p.handler.ExplainVersions(t.Context(), p.user, movie)
	if err != nil {
		t.Fatal(err)
	}
	if !explanation.KeptAll || slices.ContainsFunc(explainedVersions(explanation), func(v explainedVersion) bool { return v.step == StepHidden }) {
		t.Errorf("every version hidden: %+v", explainedVersions(explanation))
	}

	// A pinned file no addon lists any more is missing: the order stays.
	if err := p.library.PinVersion(t.Context(), p.user.ID, library.Version{ID: accounts.ID{1}, Item: movie, Name: "Gone"}); err != nil {
		t.Fatal(err)
	}
	explanation, err = p.handler.ExplainVersions(t.Context(), p.user, movie)
	if err != nil {
		t.Fatal(err)
	}
	if explanation.Pin == nil || !explanation.Pin.Missing || explanation.Pin.Name != "Gone" {
		t.Errorf("the pin of a file gone: %+v", explanation.Pin)
	}
	if got := firstWords(p.detailed(t, p.user, p.token)); !slices.Equal(got, []string{"English", "Source", "MULTI", "FRENCH", "Downloading"}) {
		t.Errorf("a pin missing: %v", got)
	}
}

// The explanation tells, for each version, the label rules matching it
// and its score under them, and names the label rule hiding a version.
func TestTheExplanationTellsTheLabelRules(t *testing.T) {
	t.Parallel()
	p := versionRulesOn(t, newTestServer(t, 10))
	movie, _ := accounts.ParseID(p.movie)
	p.setting(t, func(s *accounts.Settings) {
		s.VersionLabelRules = []accounts.VersionLabelRule{{Text: "x264", Score: 5}, {Text: "1080p", Score: 2}, {Text: "AV1", Hide: true}}
	})
	explanation, err := p.handler.ExplainVersions(t.Context(), p.user, movie)
	if err != nil {
		t.Fatal(err)
	}
	streams := explanation.Sources[0].Streams
	if len(streams) != 6 {
		t.Fatalf("streams: %+v", streams)
	}
	english, source, french, ita := streams[0], streams[1], streams[3], streams[5]
	if english.Facts.Score != 7 || !slices.Equal(english.Facts.LabelRules, []string{"x264", "1080p"}) {
		t.Errorf("English: score %d, rules %q", english.Facts.Score, english.Facts.LabelRules)
	}
	if french.Facts.Score != 5 || !slices.Equal(french.Facts.LabelRules, []string{"x264"}) {
		t.Errorf("FRENCH: score %d, rules %q", french.Facts.Score, french.Facts.LabelRules)
	}
	if source.Facts.Score != 0 || len(source.Facts.LabelRules) != 0 {
		t.Errorf("Source: score %d, rules %q", source.Facts.Score, source.Facts.LabelRules)
	}
	if ita.Step != StepHidden || ita.Rule != "label" || ita.Detail != "AV1" || !slices.Equal(ita.Facts.LabelRules, []string{"1080p", "AV1"}) {
		t.Errorf("ITA: %s %s %q, rules %q", ita.Step, ita.Rule, ita.Detail, ita.Facts.LabelRules)
	}
}
