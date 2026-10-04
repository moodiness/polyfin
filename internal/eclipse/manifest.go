// Package eclipse speaks the protocol of Eclipse music addons: small HTTP
// servers answering a manifest, searches, streams, album, artist and
// playlist pages, and catalog rows.
package eclipse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/stremio"
)

// Content types of an addon: what every track it streams is.
const (
	ContentMusic     = "music"
	ContentAudiobook = "audiobook"
	ContentPodcast   = "podcast"
)

// Catalog types: every item of a catalog row is of its type.
const (
	TypeTrack    = "track"
	TypeAlbum    = "album"
	TypeArtist   = "artist"
	TypePlaylist = "playlist"
)

// CatalogType reports whether a catalog type is one of Eclipse's rows.
func CatalogType(catalogType string) bool {
	switch catalogType {
	case TypeTrack, TypeAlbum, TypeArtist, TypePlaylist:
		return true
	}
	return false
}

// Manifest describes an Eclipse addon.
type Manifest struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Resources   []string  `json:"resources"`
	Types       []string  `json:"types,omitempty"`
	ContentType string    `json:"contentType,omitempty"`
	Settings    []Setting `json:"settings,omitempty"`
	Catalogs    []Catalog `json:"catalogs,omitempty"`
}

// Catalog is a row of items of one type the addon lists.
type Catalog struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// Setting is a field of the addon's settings: a picker, a switch, a text or
// a number, whose value is sent with every request under Key.
type Setting struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Help        string   `json:"help,omitempty"`
	Default     string   `json:"default"`
	PerNetwork  bool     `json:"perNetwork,omitempty"`
	Options     []Option `json:"options,omitempty"`
	MaxLength   int      `json:"maxLength,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
}

// Option is a choice of a picker.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Setting types.
const (
	SettingSelect = "select"
	SettingToggle = "toggle"
	SettingText   = "text"
	SettingNumber = "number"
)

// ErrInvalidManifest reports a response that is not a usable Eclipse
// manifest.
var ErrInvalidManifest = errors.New("not an Eclipse addon manifest")

// eclipseResources are resources only Eclipse addons declare.
var eclipseResources = []string{"search", "isrc", "resolve", "settings"}

// Detect reports whether a manifest is an Eclipse addon's rather than a
// Stremio addon's: it declares a resource only Eclipse has (search is
// required of Eclipse addons), a content type, or only Eclipse's types.
func Detect(data []byte) bool {
	var raw struct {
		Resources   []json.RawMessage `json:"resources"`
		Types       []string          `json:"types"`
		ContentType string            `json:"contentType"`
		Catalogs    []struct {
			Type string `json:"type"`
		} `json:"catalogs"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	for _, resource := range resourceNames(raw.Resources) {
		if slices.Contains(eclipseResources, resource) {
			return true
		}
	}
	if raw.ContentType != "" {
		return true
	}
	eclipseType := func(t string) bool { return CatalogType(t) || t == "file" }
	if len(raw.Types) > 0 && !slices.ContainsFunc(raw.Types, func(t string) bool { return !eclipseType(t) }) {
		return true
	}
	return len(raw.Catalogs) > 0 && !slices.ContainsFunc(raw.Catalogs, func(c struct {
		Type string `json:"type"`
	}) bool {
		return !CatalogType(c.Type)
	})
}

// resourceNames reads resources listed by name or, leniently, as objects
// with a name.
func resourceNames(raw []json.RawMessage) []string {
	var names []string
	for _, entry := range raw {
		var name string
		if json.Unmarshal(entry, &name) != nil {
			var object struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(entry, &object) != nil {
				continue
			}
			name = object.Name
		}
		if name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// ParseManifest decodes and validates an Eclipse manifest. Catalogs of an
// unknown type, settings without a key or with a key given twice, and
// pickers without options are left out, as they could not work.
func ParseManifest(data []byte) (Manifest, error) {
	var raw struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Version     stremio.Text      `json:"version"`
		Description string            `json:"description"`
		Icon        string            `json:"icon"`
		Resources   []json.RawMessage `json:"resources"`
		Types       []string          `json:"types"`
		ContentType string            `json:"contentType"`
		Settings    []json.RawMessage `json:"settings"`
		Catalogs    []Catalog         `json:"catalogs"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	m := Manifest{ID: strings.TrimSpace(raw.ID), Name: strings.TrimSpace(raw.Name), Version: strings.TrimSpace(string(raw.Version)),
		Description: raw.Description, Resources: resourceNames(raw.Resources), Types: raw.Types}
	if m.ID == "" || m.Name == "" || m.Version == "" || len(m.Resources) == 0 {
		return Manifest{}, fmt.Errorf("%w: id, name, version and resources are required", ErrInvalidManifest)
	}
	// Artwork is fetched by apps through Polyfin, and only from https.
	if strings.HasPrefix(raw.Icon, "https://") {
		m.Icon = raw.Icon
	}
	switch raw.ContentType {
	case ContentAudiobook, ContentPodcast:
		m.ContentType = raw.ContentType
	default:
		m.ContentType = ContentMusic
	}
	seen := map[string]bool{}
	for _, catalog := range raw.Catalogs {
		catalog.ID, catalog.Name = strings.TrimSpace(catalog.ID), strings.TrimSpace(catalog.Name)
		if catalog.ID == "" || !CatalogType(catalog.Type) || seen[catalog.ID] {
			continue
		}
		seen[catalog.ID] = true
		if catalog.Name == "" {
			catalog.Name = catalog.ID
		}
		m.Catalogs = append(m.Catalogs, catalog)
	}
	keys := map[string]bool{}
	for _, entry := range raw.Settings {
		setting, ok := parseSetting(entry)
		if !ok || keys[setting.Key] {
			continue
		}
		keys[setting.Key] = true
		m.Settings = append(m.Settings, setting)
	}
	return m, nil
}

// parseSetting reads a settings field. A per-network default, given for
// Wi-Fi and cellular, reduces to one value, the Wi-Fi one: Polyfin is a
// server and keeps a single value for each field.
func parseSetting(data []byte) (Setting, bool) {
	var raw struct {
		Key        string          `json:"key"`
		Type       string          `json:"type"`
		Label      string          `json:"label"`
		Help       string          `json:"help"`
		Default    json.RawMessage `json:"default"`
		PerNetwork bool            `json:"perNetwork"`
		Options    []struct {
			Value json.RawMessage `json:"value"`
			Label string          `json:"label"`
		} `json:"options"`
		MaxLength   stremio.Number `json:"maxLength"`
		Placeholder string         `json:"placeholder"`
		Min         *float64       `json:"min"`
		Max         *float64       `json:"max"`
		Step        *float64       `json:"step"`
	}
	if json.Unmarshal(data, &raw) != nil || strings.TrimSpace(raw.Key) == "" {
		return Setting{}, false
	}
	s := Setting{Key: strings.TrimSpace(raw.Key), Type: raw.Type, Label: strings.TrimSpace(raw.Label), Help: raw.Help,
		PerNetwork: raw.PerNetwork, Placeholder: raw.Placeholder, MaxLength: max(int(raw.MaxLength), 0), Min: raw.Min, Max: raw.Max, Step: raw.Step}
	if s.Label == "" {
		s.Label = s.Key
	}
	for _, option := range raw.Options {
		value := scalar(option.Value)
		label := strings.TrimSpace(option.Label)
		if label == "" {
			label = value
		}
		s.Options = append(s.Options, Option{Value: value, Label: label})
	}
	s.Default = scalar(raw.Default)
	if s.PerNetwork {
		var networks map[string]json.RawMessage
		if json.Unmarshal(raw.Default, &networks) == nil {
			s.Default = scalar(networks["wifi"])
			if s.Default == "" {
				s.Default = scalar(networks["cellular"])
			}
		}
	}
	switch s.Type {
	case SettingSelect:
		if len(s.Options) == 0 {
			return Setting{}, false
		}
		if !slices.ContainsFunc(s.Options, func(o Option) bool { return o.Value == s.Default }) {
			s.Default = s.Options[0].Value
		}
	case SettingToggle:
		s.Default = strconv.FormatBool(s.Default == "true")
	case SettingNumber:
		if _, err := strconv.ParseFloat(s.Default, 64); err != nil {
			s.Default = ""
		}
	case SettingText:
	default:
		return Setting{}, false
	}
	return s, true
}

// scalar writes a JSON string, number or boolean as the text sent in a
// query; anything else is empty.
func scalar(data json.RawMessage) string {
	data = bytes.TrimSpace(data)
	var text string
	if json.Unmarshal(data, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(data, &number) == nil {
		return number.String()
	}
	var flag bool
	if json.Unmarshal(data, &flag) == nil {
		return strconv.FormatBool(flag)
	}
	return ""
}

// Has reports whether the addon declares a resource.
func (m Manifest) Has(resource string) bool {
	return slices.Contains(m.Resources, resource)
}

// Catalog finds a catalog row by identifier.
func (m Manifest) Catalog(id string) (Catalog, bool) {
	i := slices.IndexFunc(m.Catalogs, func(c Catalog) bool { return c.ID == id })
	if i < 0 {
		return Catalog{}, false
	}
	return m.Catalogs[i], true
}

// Stremio describes the addon as the libraries see an addon: its name, icon
// and catalog rows, each a catalog of its Eclipse type. It declares no
// Stremio resource, so that nothing asks it for one.
func (m Manifest) Stremio() stremio.Manifest {
	catalogs := make([]stremio.Catalog, 0, len(m.Catalogs))
	for _, c := range m.Catalogs {
		catalogs = append(catalogs, stremio.Catalog{Type: c.Type, ID: c.ID, Name: c.Name})
	}
	return stremio.Manifest{ID: m.ID, Version: m.Version, Name: m.Name, Description: m.Description, Logo: m.Icon, Catalogs: catalogs}
}

// ErrInvalidSettings reports settings values the manifest does not allow.
var ErrInvalidSettings = errors.New("invalid addon settings")

// maxTextSetting bounds a text value without a maxLength of its own.
const maxTextSetting = 1000

// CheckSettings validates values for the declared settings and returns
// them normalized: a toggle as true or false, a number as written by Go.
// Keys the manifest does not declare are refused.
func (m Manifest) CheckSettings(values map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(values))
	for key, value := range values {
		i := slices.IndexFunc(m.Settings, func(s Setting) bool { return s.Key == key })
		if i < 0 {
			return nil, fmt.Errorf("%w: unknown setting", ErrInvalidSettings)
		}
		s := m.Settings[i]
		switch s.Type {
		case SettingSelect:
			if !slices.ContainsFunc(s.Options, func(o Option) bool { return o.Value == value }) {
				return nil, fmt.Errorf("%w: %s is not one of its options", ErrInvalidSettings, key)
			}
		case SettingToggle:
			flag, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("%w: %s is a toggle", ErrInvalidSettings, key)
			}
			value = strconv.FormatBool(flag)
		case SettingText:
			limit := maxTextSetting
			if s.MaxLength > 0 {
				limit = s.MaxLength
			}
			if utf8.RuneCountInString(value) > limit || strings.ContainsFunc(value, func(r rune) bool { return r < ' ' }) {
				return nil, fmt.Errorf("%w: %s is too long", ErrInvalidSettings, key)
			}
		case SettingNumber:
			number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil || math.IsInf(number, 0) || math.IsNaN(number) ||
				s.Min != nil && number < *s.Min || s.Max != nil && number > *s.Max {
				return nil, fmt.Errorf("%w: %s is out of range", ErrInvalidSettings, key)
			}
			if s.Step != nil && *s.Step > 0 {
				base := 0.0
				if s.Min != nil {
					base = *s.Min
				}
				steps := (number - base) / *s.Step
				if math.Abs(steps-math.Round(steps)) > 1e-9 {
					return nil, fmt.Errorf("%w: %s is not a step", ErrInvalidSettings, key)
				}
			}
			value = strconv.FormatFloat(number, 'f', -1, 64)
		}
		result[key] = value
	}
	return result, nil
}

// Query is what every request to the addon carries: the value of each
// declared setting, the one chosen or else its default, under its key.
// Values chosen for settings the addon no longer declares are not sent.
func (m Manifest) Query(chosen map[string]string) url.Values {
	query := url.Values{}
	for _, s := range m.Settings {
		value, ok := chosen[s.Key]
		if !ok {
			value = s.Default
		}
		query.Set(s.Key, value)
	}
	return query
}
