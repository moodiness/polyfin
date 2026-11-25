// Package moonfin keeps what Polyfin serves Moonfin's apps as their
// companion: the routes they call on a server for their extra features (see
// the jellyfin package's moonfin.go), each user's settings synced across
// their devices first.
//
// Moonfin's apps sync their settings as one JSON object per user, the
// envelope: the settings every device shares, the global profile, and those
// one kind of device overrides, the desktop, mobile and tv profiles, each
// with only what it changed. Polyfin keeps them as the apps send them,
// whatever their names, so a setting a newer app adds keeps syncing.
//
// An app asks for a profile resolved: for each setting, the first value of
// the device's own profile, the user's global profile, the administrator's
// defaults for that kind of device, and the administrator's defaults for
// every device (see Resolve). The home layout, homeSections and
// homeRowOrder, is taken whole from the first of them that has one, never
// mixed.
//
// A user's own API keys and the titles they hid from Continue Watching and
// Next Up are theirs alone (PersonalKeys): the administrator's defaults
// never hold them, and the server's own keys are never given to users. The
// hidden titles live on the global profile, whichever device hid them; a
// device that sends a title no longer hidden, having been told it was,
// brings it back for every device (see the baselines of Store).
package moonfin

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"
)

// Settings are one profile's settings, or the administrator's defaults for
// one kind of device, as JSON decodes them.
type Settings = map[string]any

// Envelope is a user's settings: their profiles under their names, and
// what the apps keep beside them (schemaVersion, lastUpdated,
// lastUpdatedBy, syncEnabled).
type Envelope = map[string]any

// The profiles of an envelope: Global, which every device inherits, and
// those of one kind of device.
const (
	Global  = "global"
	Desktop = "desktop"
	Mobile  = "mobile"
	TV      = "tv"
)

// Profiles are the names of an envelope's profiles, Global first.
var Profiles = []string{Global, Desktop, Mobile, TV}

// schemaVersion is the version of the envelope Moonfin's apps speak: its
// profiles, as opposed to one flat object of settings.
const schemaVersion = 2

// ProfileName is the profile a route names, in any letter case, and
// whether it is one of Profiles.
func ProfileName(raw string) (string, bool) {
	name := strings.ToLower(raw)
	return name, slices.Contains(Profiles, name)
}

// The settings a user's own: their API keys and the titles they hid.
const (
	hiddenContinueWatching = "hiddenContinueWatchingItems"
	hiddenNextUp           = "hiddenNextUpSeries"
)

// PersonalKeys are the settings that are a user's own rather than
// preferences: their own API keys and the titles they hid. Defaults never
// hold them, and a reset to the defaults keeps them.
var PersonalKeys = []string{"seerrApiKey", "mdblistApiKey", "tmdbApiKey", hiddenContinueWatching, hiddenNextUp}

// hiddenKeys are the PersonalKeys of hidden titles, kept on the global
// profile only.
var hiddenKeys = []string{hiddenContinueWatching, hiddenNextUp}

// layoutKeys are the home layout, taken whole from one layer.
var layoutKeys = []string{"homeSections", "homeRowOrder"}

// Clean is a profile an app sent, as it is kept: names starting with a
// lower-case letter, as the apps write them and as Jellyfin's model
// binding reads others, and without null values, which an app sends for
// what it does not set.
func Clean(in Settings) Settings {
	out := make(Settings, len(in))
	for name, value := range in {
		if name == "" || value == nil {
			continue
		}
		out[strings.ToLower(name[:1])+name[1:]] = value
	}
	return out
}

// withoutPersonal is settings without PersonalKeys.
func withoutPersonal(settings Settings) Settings {
	out := maps.Clone(settings)
	if out == nil {
		out = Settings{}
	}
	for _, name := range PersonalKeys {
		delete(out, name)
	}
	return out
}

// profileOf is the profile name of env, nil when it has none.
func profileOf(env Envelope, name string) Settings {
	profile, _ := env[name].(map[string]any)
	return profile
}

// mergeInto sets the settings of incoming over those of profile. A home
// order sent without home sections makes the order the layout: the
// sections kept are dropped. An empty API key keeps the one kept, as an
// app that lost its copy sends one.
func mergeInto(profile, incoming Settings) {
	_, sections := incoming["homeSections"]
	if _, order := incoming["homeRowOrder"]; order && !sections {
		delete(profile, "homeSections")
	}
	for name, value := range incoming {
		if text, ok := value.(string); ok && strings.HasSuffix(name, "ApiKey") && strings.TrimSpace(text) == "" {
			continue
		}
		profile[name] = value
	}
}

// hiddenTitles reads a setting of hidden titles: a JSON object of item
// identifiers and when each was hidden, as a string. ok is false for
// anything else.
func hiddenTitles(value any) (titles map[string]string, ok bool) {
	text, isText := value.(string)
	if !isText {
		return nil, false
	}
	if err := json.Unmarshal([]byte(text), &titles); err != nil {
		return nil, false
	}
	if titles == nil {
		titles = map[string]string{}
	}
	return titles, true
}

func hiddenText(titles map[string]string) string {
	text, _ := json.Marshal(titles)
	return string(text)
}

// later reports whether the time a title was hidden at, a, is after b.
// Times that do not read keep the one kept.
func later(a, b string) bool {
	at, errA := time.Parse(time.RFC3339Nano, a)
	bt, errB := time.Parse(time.RFC3339Nano, b)
	return errA == nil && errB == nil && at.After(bt)
}

// Baselines are, for each profile, the hidden titles of each setting of
// hiddenKeys the profile's devices were last told of.
type Baselines = map[string]map[string]map[string]string

// mergeHidden moves the hidden titles a profile was sent with to the
// global profile of env: a title is hidden when the device hid it, or
// another did unless the device was told of it and dropped it since, then
// hidden later, unless hidden again elsewhere after. baselines record what
// the device holds now.
func mergeHidden(env Envelope, name string, incoming Settings, baselines Baselines) {
	for _, key := range hiddenKeys {
		value, sent := incoming[key]
		if !sent {
			continue
		}
		delete(incoming, key)
		sentTitles, ok := hiddenTitles(value)
		if !ok {
			continue
		}
		global := profileOf(env, Global)
		if global == nil {
			global = Settings{}
			env[Global] = global
		}
		kept, _ := hiddenTitles(global[key])
		if kept == nil {
			kept = map[string]string{}
		}
		told := baselines[name][key]
		for id, at := range told {
			if _, still := sentTitles[id]; !still && !later(kept[id], at) {
				delete(kept, id)
			}
		}
		for id, at := range sentTitles {
			if current, has := kept[id]; !has || later(at, current) {
				kept[id] = at
			}
		}
		global[key] = hiddenText(kept)
		remember(baselines, name, key, sentTitles)
	}
}

func remember(baselines Baselines, name, key string, titles map[string]string) {
	if baselines[name] == nil {
		baselines[name] = map[string]map[string]string{}
	}
	baselines[name][key] = maps.Clone(titles)
}

// stamp marks env as saved by clientID at now.
func stamp(env Envelope, clientID string, now time.Time) {
	if clientID == "" {
		clientID = "unknown"
	}
	env["schemaVersion"] = schemaVersion
	env["lastUpdated"] = now.UnixMilli()
	env["lastUpdatedBy"] = clientID
	if _, ok := env["syncEnabled"].(bool); !ok {
		env["syncEnabled"] = true
	}
}

// SaveProfile merges incoming, a profile Clean made, into the profile
// name of env, nil for a user with no settings yet, and returns the
// envelope to keep.
func SaveProfile(env Envelope, baselines Baselines, name string, incoming Settings, clientID string, now time.Time) Envelope {
	if env == nil {
		env = Envelope{}
	}
	mergeHidden(env, name, incoming, baselines)
	if profile := profileOf(env, name); profile != nil {
		mergeInto(profile, incoming)
	} else if len(incoming) > 0 || name == Global {
		env[name] = incoming
	}
	stamp(env, clientID, now)
	return env
}

// SaveEnvelope keeps incoming, a whole envelope an app sent: merged into
// env profile by profile, or replacing it when replace is set.
func SaveEnvelope(env, incoming Envelope, baselines Baselines, replace bool, clientID string, now time.Time) Envelope {
	if env == nil || replace {
		env = Envelope{}
		for name := range baselines {
			delete(baselines, name)
		}
	}
	for name, value := range incoming {
		profile, isProfile := value.(map[string]any)
		switch {
		case slices.Contains(Profiles, name) && isProfile:
			env = SaveProfile(env, baselines, name, Clean(profile), clientID, now)
		case slices.Contains(Profiles, name), value == nil:
		default:
			env[name] = value
		}
	}
	stamp(env, clientID, now)
	return env
}

// LayerDefaults are the administrator's defaults for profile name: those
// of every device, under those of name's kind of device, whose home
// layout replaces theirs whole.
func LayerDefaults(defaults map[string]Settings, name string) Settings {
	out := maps.Clone(defaults[Global])
	if out == nil {
		out = Settings{}
	}
	device := defaults[name]
	if name == Global || len(device) == 0 {
		return out
	}
	if slices.ContainsFunc(layoutKeys, func(key string) bool { return device[key] != nil }) {
		for _, key := range layoutKeys {
			delete(out, key)
		}
	}
	maps.Copy(out, device)
	return out
}

// Resolve is the profile name of env as a device of its kind takes it: its
// own settings, over the user's global ones, over defaults (see
// LayerDefaults). The home layout comes whole from the first of these that
// has one.
func Resolve(env Envelope, name string, defaults Settings) Settings {
	var device Settings
	if name != Global {
		device = profileOf(env, name)
	}
	layers := []Settings{device, profileOf(env, Global), defaults}
	out := Settings{}
	for i := len(layers) - 1; i >= 0; i-- {
		for key, value := range layers[i] {
			if value != nil && !slices.Contains(layoutKeys, key) {
				out[key] = value
			}
		}
	}
	for _, layer := range layers {
		if !slices.ContainsFunc(layoutKeys, func(key string) bool { return layer[key] != nil }) {
			continue
		}
		for _, key := range layoutKeys {
			if value := layer[key]; value != nil {
				out[key] = value
			}
		}
		break
	}
	return out
}

// told records in baselines the hidden titles resolved gives a device of
// profile name, and reports whether that changed them.
func told(baselines Baselines, name string, resolved Settings) bool {
	changed := false
	for _, key := range hiddenKeys {
		titles, ok := hiddenTitles(resolved[key])
		if !ok {
			titles = map[string]string{}
		}
		if maps.Equal(baselines[name][key], titles) && baselines[name] != nil && baselines[name][key] != nil {
			continue
		}
		remember(baselines, name, key, titles)
		changed = true
	}
	return changed
}

// ApplyDefaults writes defaults over the settings of env: each profile's
// defaults into the same profile, and those of every device into each
// device profile env has as well, but for the settings that kind of
// device has defaults of its own. With reset, env starts over from the
// defaults, keeping only the user's PersonalKeys.
func ApplyDefaults(env Envelope, defaults map[string]Settings, reset bool, now time.Time) Envelope {
	if reset {
		fresh := Envelope{}
		for _, key := range []string{"schemaVersion", "syncEnabled"} {
			if value, ok := env[key]; ok {
				fresh[key] = value
			}
		}
		for _, name := range Profiles {
			kept := Settings{}
			for _, key := range PersonalKeys {
				if value, ok := profileOf(env, name)[key]; ok {
					kept[key] = value
				}
			}
			if len(defaults[name]) > 0 || len(kept) > 0 || name == Global {
				maps.Copy(kept, withoutPersonal(defaults[name]))
				fresh[name] = kept
			}
		}
		env = fresh
	} else {
		for _, name := range Profiles {
			values := withoutPersonal(defaults[name])
			if name != Global {
				if len(values) == 0 {
					continue
				}
				profile := profileOf(env, name)
				if profile == nil {
					profile = Settings{}
					env[name] = profile
				}
				mergeInto(profile, values)
				continue
			}
			global := profileOf(env, Global)
			if global == nil {
				global = Settings{}
				env[Global] = global
			}
			mergeInto(global, values)
			for _, device := range Profiles[1:] {
				profile := profileOf(env, device)
				if profile == nil {
					continue
				}
				own := defaults[device]
				for key, value := range values {
					if _, has := own[key]; !has {
						profile[key] = value
					}
				}
			}
		}
	}
	stamp(env, "polyfin", now)
	return env
}

// sameSettings reports whether a and b hold the same profiles, whatever
// they tell of their saving.
func sameSettings(a, b Envelope) bool {
	for _, name := range Profiles {
		left, _ := json.Marshal(profileOf(a, name))
		right, _ := json.Marshal(profileOf(b, name))
		if string(left) != string(right) {
			return false
		}
	}
	return true
}

// Count is how many settings env sets, over its profiles, and the names of
// its profiles that set any, in Profiles' order.
func Count(env Envelope) (int, []string) {
	count, names := 0, []string{}
	for _, name := range Profiles {
		if n := len(profileOf(env, name)); n > 0 {
			count += n
			names = append(names, name)
		}
	}
	return count, names
}
