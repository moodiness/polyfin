// Package stremio speaks the Stremio addon protocol.
package stremio

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Manifest describes an addon: what it serves, for which types, and its
// catalogs.
type Manifest struct {
	ID          string     `json:"id"`
	Version     string     `json:"version"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Logo        string     `json:"logo"`
	Types       []string   `json:"types"`
	Resources   []Resource `json:"resources"`
	Catalogs    []Catalog  `json:"catalogs"`
	IDPrefixes  []string   `json:"idPrefixes"`
}

// Resource is a resource the addon serves. Manifests list resources either
// by name or as objects narrowing the types and ID prefixes they apply to.
type Resource struct {
	Name       string   `json:"name"`
	Types      []string `json:"types,omitempty"`
	IDPrefixes []string `json:"idPrefixes,omitempty"`
}

func (r *Resource) UnmarshalJSON(data []byte) error {
	var name string
	if json.Unmarshal(data, &name) == nil {
		*r = Resource{Name: name}
		return nil
	}
	type plain Resource
	return json.Unmarshal(data, (*plain)(r))
}

// Catalog is a list of items an addon can browse.
type Catalog struct {
	Type  string  `json:"type"`
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Extra []Extra `json:"extra,omitempty"`
}

// Extra is a property a catalog request can carry, such as a genre, a
// search query or a page offset.
type Extra struct {
	Name       string   `json:"name"`
	IsRequired bool     `json:"isRequired,omitempty"`
	Options    []string `json:"options,omitempty"`
}

// UnmarshalJSON also reads the legacy extraSupported and extraRequired
// lists of older manifests.
func (c *Catalog) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type           string   `json:"type"`
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		Extra          []Extra  `json:"extra"`
		ExtraSupported []string `json:"extraSupported"`
		ExtraRequired  []string `json:"extraRequired"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = Catalog{Type: raw.Type, ID: raw.ID, Name: raw.Name, Extra: raw.Extra}
	if len(c.Extra) == 0 {
		for _, name := range raw.ExtraSupported {
			c.Extra = append(c.Extra, Extra{Name: name, IsRequired: slices.Contains(raw.ExtraRequired, name)})
		}
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	return nil
}

// Browsable reports whether the catalog can be listed without user input:
// every required property must offer predefined options (the first one is
// used). Search catalogs, which need a query, are not browsable.
func (c Catalog) Browsable() bool {
	for _, extra := range c.Extra {
		if extra.IsRequired && len(extra.Options) == 0 {
			return false
		}
	}
	return true
}

// HasResource reports whether the addon serves a resource.
func (m Manifest) HasResource(name string) bool {
	for _, resource := range m.Resources {
		if resource.Name == name {
			return true
		}
	}
	return false
}

// ResourceNames lists the resources the addon serves.
func (m Manifest) ResourceNames() []string {
	names := make([]string, 0, len(m.Resources))
	for _, resource := range m.Resources {
		if !slices.Contains(names, resource.Name) {
			names = append(names, resource.Name)
		}
	}
	return names
}

// Catalog finds a catalog by type and ID.
func (m Manifest) Catalog(catalogType, id string) (Catalog, bool) {
	for _, catalog := range m.Catalogs {
		if catalog.Type == catalogType && catalog.ID == id {
			return catalog, true
		}
	}
	return Catalog{}, false
}

// ErrInvalidManifest reports a response that is not a usable manifest.
var ErrInvalidManifest = errors.New("not a Stremio addon manifest")

// ParseManifest decodes and validates a manifest.
func ParseManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if manifest.ID == "" || manifest.Name == "" || manifest.Version == "" || len(manifest.Resources) == 0 {
		return Manifest{}, fmt.Errorf("%w: id, name, version and resources are required", ErrInvalidManifest)
	}
	if manifest.Logo != "" && !strings.HasPrefix(manifest.Logo, "https://") {
		manifest.Logo = ""
	}
	return manifest, nil
}

// ErrInvalidManifestURL reports a URL that cannot be an addon manifest.
var ErrInvalidManifestURL = errors.New("manifest URLs are http(s) URLs ending in /manifest.json")

// NormalizeManifestURL validates a manifest URL. Stremio install links
// (stremio://host/…/manifest.json) are converted to https.
func NormalizeManifestURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", ErrInvalidManifestURL
	}
	switch strings.ToLower(parsed.Scheme) {
	case "stremio":
		parsed.Scheme = "https"
	case "http", "https":
		parsed.Scheme = strings.ToLower(parsed.Scheme)
	default:
		return "", ErrInvalidManifestURL
	}
	if !strings.HasSuffix(parsed.Path, "/manifest.json") {
		return "", ErrInvalidManifestURL
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

// RedactManifestURL keeps only what identifies an addon's host. The rest of
// a manifest URL usually carries the user's credentials.
func RedactManifestURL(manifestURL string) string {
	parsed, err := url.Parse(manifestURL)
	if err != nil {
		return "…/manifest.json"
	}
	if parsed.Path == "/manifest.json" {
		return parsed.Scheme + "://" + parsed.Host + "/manifest.json"
	}
	return parsed.Scheme + "://" + parsed.Host + "/…/manifest.json"
}

// BaseURL is the address resources are requested under.
func BaseURL(manifestURL string) string {
	return strings.TrimSuffix(manifestURL, "/manifest.json")
}
