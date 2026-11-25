package moonfin

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

var saved = time.Date(2026, time.November, 25, 10, 0, 0, 0, time.UTC)

// settingsOf decodes a JSON object, as the store and the routes do.
func settingsOf(t *testing.T, text string) Settings {
	t.Helper()
	var s Settings
	if err := json.Unmarshal([]byte(text), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// A device takes, for each setting, its own value, else the user's global
// one, else the administrator's default for its kind of device, else the
// default for every device.
func TestResolvedSettingsTakeTheFirstValueSet(t *testing.T) {
	t.Parallel()
	env := Envelope{
		Global: settingsOf(t, `{"a": "user global", "b": "user global"}`),
		TV:     settingsOf(t, `{"a": "user tv"}`),
	}
	defaults := map[string]Settings{
		Global: settingsOf(t, `{"a": "defaults", "b": "defaults", "c": "defaults", "d": "defaults"}`),
		TV:     settingsOf(t, `{"c": "tv defaults"}`),
	}
	want := map[string]Settings{
		TV:      {"a": "user tv", "b": "user global", "c": "tv defaults", "d": "defaults"},
		Desktop: {"a": "user global", "b": "user global", "c": "defaults", "d": "defaults"},
		Global:  {"a": "user global", "b": "user global", "c": "defaults", "d": "defaults"},
	}
	for profile, w := range want {
		if got := Resolve(env, profile, LayerDefaults(defaults, profile)); !reflect.DeepEqual(got, w) {
			t.Errorf("%s: %v, want %v", profile, got, w)
		}
	}
	if got := Resolve(nil, Mobile, LayerDefaults(defaults, Mobile)); !reflect.DeepEqual(got, defaults[Global]) {
		t.Errorf("a user without settings: %v", got)
	}
}

// The home layout comes whole from the first layer that has one: a
// device's sections never mix with the global order.
func TestTheHomeLayoutComesWholeFromOneLayer(t *testing.T) {
	t.Parallel()
	env := Envelope{
		Global: settingsOf(t, `{"homeSections": [{"type": "resume"}], "homeRowOrder": ["resume"]}`),
		Mobile: settingsOf(t, `{"homeRowOrder": ["latest"]}`),
	}
	defaults := map[string]Settings{
		Global: settingsOf(t, `{"homeSections": [{"type": "nextup"}], "homeRowOrder": ["nextup"]}`),
		TV:     settingsOf(t, `{"homeRowOrder": ["livetv"]}`),
	}
	got := Resolve(env, Mobile, LayerDefaults(defaults, Mobile))
	if _, has := got["homeSections"]; has || !reflect.DeepEqual(got["homeRowOrder"], []any{"latest"}) {
		t.Errorf("the mobile layout: %v", got)
	}
	got = Resolve(env, Desktop, LayerDefaults(defaults, Desktop))
	if !reflect.DeepEqual(got["homeRowOrder"], []any{"resume"}) || got["homeSections"] == nil {
		t.Errorf("the global layout: %v", got)
	}
	got = Resolve(nil, TV, LayerDefaults(defaults, TV))
	if _, has := got["homeSections"]; has || !reflect.DeepEqual(got["homeRowOrder"], []any{"livetv"}) {
		t.Errorf("the TV defaults' layout: %v", got)
	}
}

// A profile saved is merged into the one kept: an empty API key keeps the
// one kept, and an order sent without sections makes the order the
// layout.
func TestASavedProfileIsMerged(t *testing.T) {
	t.Parallel()
	env := Envelope{Global: settingsOf(t, `{"keep": 1, "mdblistApiKey": "mine", "homeSections": [{"type": "resume"}]}`)}
	env = SaveProfile(env, Baselines{}, Global, Clean(settingsOf(t, `{"Theme": "dark", "mdblistApiKey": " ", "homeRowOrder": ["latest"], "gone": null}`)),
		"moonfin-flutter", saved)
	want := settingsOf(t, `{"keep": 1, "mdblistApiKey": "mine", "theme": "dark", "homeRowOrder": ["latest"]}`)
	if got := profileOf(env, Global); !reflect.DeepEqual(got, want) {
		t.Errorf("global: %v, want %v", got, want)
	}
	if env["lastUpdatedBy"] != "moonfin-flutter" || env["lastUpdated"] != saved.UnixMilli() || env["schemaVersion"] != schemaVersion || env["syncEnabled"] != true {
		t.Errorf("the envelope's own fields: %v", env)
	}
	env = SaveProfile(env, Baselines{}, TV, Settings{}, "tv", saved)
	if _, has := env[TV]; has {
		t.Errorf("an empty device profile is kept: %v", env)
	}
}

// Hidden titles live on the global profile, whichever device hid them. A
// device brings back a title it was told was hidden by sending it no more;
// a title it never heard of stays hidden.
func TestHiddenTitlesFollowEveryDevice(t *testing.T) {
	t.Parallel()
	baselines := Baselines{}
	env := SaveProfile(nil, baselines, TV, Clean(Settings{hiddenContinueWatching: `{"a":"2026-11-01T00:00:00Z"}`}), "tv", saved)
	if _, has := profileOf(env, TV)[hiddenContinueWatching]; has {
		t.Fatalf("kept on the TV profile: %v", env)
	}
	// The phone hides b without having heard of a.
	env = SaveProfile(env, baselines, Mobile, Clean(Settings{hiddenContinueWatching: `{"b":"2026-11-02T00:00:00Z"}`}), "phone", saved)
	hidden, _ := hiddenTitles(profileOf(env, Global)[hiddenContinueWatching])
	if len(hidden) != 2 {
		t.Fatalf("hidden after the phone: %v", hidden)
	}
	// The TV is told of both, then brings a back.
	if !told(baselines, TV, Resolve(env, TV, nil)) {
		t.Fatal("telling the TV changed nothing")
	}
	env = SaveProfile(env, baselines, TV, Clean(Settings{hiddenContinueWatching: `{"b":"2026-11-02T00:00:00Z"}`}), "tv", saved)
	hidden, _ = hiddenTitles(profileOf(env, Global)[hiddenContinueWatching])
	if _, has := hidden["a"]; has || len(hidden) != 1 {
		t.Errorf("after the TV brought a back: %v", hidden)
	}
	// The phone, which still holds b only, sends it again: a stays back.
	env = SaveProfile(env, baselines, Mobile, Clean(Settings{hiddenContinueWatching: `{"b":"2026-11-02T00:00:00Z"}`}), "phone", saved)
	hidden, _ = hiddenTitles(profileOf(env, Global)[hiddenContinueWatching])
	if len(hidden) != 1 || hidden["b"] == "" {
		t.Errorf("after the phone saved again: %v", hidden)
	}
}

// A whole envelope is merged profile by profile, or replaces the one kept.
func TestAWholeEnvelopeMergesOrReplaces(t *testing.T) {
	t.Parallel()
	kept := func() Envelope {
		return Envelope{Global: Settings{"a": 1.0}, TV: Settings{"b": 2.0}}
	}
	incoming := func() Envelope {
		return Envelope{Global: map[string]any{"c": 3.0}, "syncEnabled": false}
	}
	merged := SaveEnvelope(kept(), incoming(), Baselines{}, false, "web", saved)
	if !reflect.DeepEqual(profileOf(merged, Global), Settings{"a": 1.0, "c": 3.0}) || profileOf(merged, TV) == nil || merged["syncEnabled"] != false {
		t.Errorf("merged: %v", merged)
	}
	replaced := SaveEnvelope(kept(), incoming(), Baselines{}, true, "web", saved)
	if !reflect.DeepEqual(profileOf(replaced, Global), Settings{"c": 3.0}) || profileOf(replaced, TV) != nil {
		t.Errorf("replaced: %v", replaced)
	}
}

// Applying the defaults writes each profile's into the same profile, and
// those of every device into the device profiles a user has, but where the
// device has its own; a reset keeps only the user's own keys and hidden
// titles.
func TestDefaultsAppliedToAUser(t *testing.T) {
	t.Parallel()
	defaults := map[string]Settings{
		Global: {"a": "default", "b": "default"},
		TV:     {"b": "tv default"},
	}
	user := func() Envelope {
		return Envelope{
			Global: Settings{"a": "mine", "z": "mine", "tmdbApiKey": "key", hiddenNextUp: `{"x":"2026-11-01T00:00:00Z"}`},
			TV:     Settings{"a": "my tv", "b": "my tv"},
			Mobile: Settings{"a": "my phone"},
		}
	}
	merged := ApplyDefaults(user(), defaults, false, saved)
	if got := profileOf(merged, Global); got["a"] != "default" || got["b"] != "default" || got["z"] != "mine" || got["tmdbApiKey"] != "key" {
		t.Errorf("global: %v", got)
	}
	if got := profileOf(merged, TV); got["a"] != "default" || got["b"] != "tv default" {
		t.Errorf("tv: %v", got)
	}
	if got := profileOf(merged, Mobile); got["a"] != "default" || got["b"] != "default" {
		t.Errorf("mobile: %v", got)
	}
	reset := ApplyDefaults(user(), defaults, true, saved)
	want := Settings{"a": "default", "b": "default", "tmdbApiKey": "key", hiddenNextUp: `{"x":"2026-11-01T00:00:00Z"}`}
	if got := profileOf(reset, Global); !reflect.DeepEqual(got, want) {
		t.Errorf("reset global: %v, want %v", got, want)
	}
	if got := profileOf(reset, TV); !reflect.DeepEqual(got, Settings{"b": "tv default"}) {
		t.Errorf("reset tv: %v", got)
	}
	if profileOf(reset, Mobile) != nil {
		t.Errorf("reset mobile: %v", reset)
	}
}
