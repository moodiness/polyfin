package stremio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Text decodes a JSON string or number, as addons send years and ratings
// either way.
type Text string

func (t *Text) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*t = Text(text)
		return nil
	}
	var number json.Number
	if json.Unmarshal(data, &number) == nil {
		*t = Text(number.String())
		return nil
	}
	*t = ""
	return nil
}

// Number decodes a JSON number or numeric string.
type Number int

func (n *Number) UnmarshalJSON(data []byte) error {
	var text Text
	if err := text.UnmarshalJSON(data); err != nil {
		return err
	}
	value, _ := strconv.ParseFloat(strings.TrimSpace(string(text)), 64)
	*n = Number(value)
	return nil
}

// Names decodes a list of names, such as genres or directors: a JSON
// array, or a string some addons send instead, its names separated by
// commas. Elements that are not strings, and any other value, are left
// out rather than failing the title.
type Names []string

func (n *Names) UnmarshalJSON(data []byte) error {
	*n = nil
	var elements []json.RawMessage
	if json.Unmarshal(data, &elements) == nil {
		for _, element := range elements {
			var name string
			if json.Unmarshal(element, &name) == nil {
				*n = append(*n, name)
			}
		}
		return nil
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		for name := range strings.SplitSeq(text, ",") {
			if name = strings.TrimSpace(name); name != "" {
				*n = append(*n, name)
			}
		}
	}
	return nil
}

// Meta describes a title, as returned by catalogs (a preview) and by the
// meta resource (complete).
type Meta struct {
	ID              string        `json:"id"`
	Type            string        `json:"type"`
	Name            string        `json:"name"`
	Poster          string        `json:"poster,omitempty"`
	Background      string        `json:"background,omitempty"`
	Logo            string        `json:"logo,omitempty"`
	LandscapePoster string        `json:"landscapePoster,omitempty"`
	Description     string        `json:"description,omitempty"`
	ReleaseInfo     Text          `json:"releaseInfo,omitempty"`
	Year            Text          `json:"year,omitempty"`
	Released        string        `json:"released,omitempty"`
	Runtime         Text          `json:"runtime,omitempty"`
	Genres          Names         `json:"genres,omitempty"`
	Director        Names         `json:"director,omitempty"`
	Writer          Names         `json:"writer,omitempty"`
	Cast            Names         `json:"cast,omitempty"`
	ImdbRating      Text          `json:"imdbRating,omitempty"`
	Status          string        `json:"status,omitempty"`
	Language        string        `json:"language,omitempty"`
	ImdbID          string        `json:"imdb_id,omitempty"`
	TmdbID          Text          `json:"_tmdbId,omitempty"`
	TvdbID          Text          `json:"_tvdbId,omitempty"`
	Trailers        []Trailer     `json:"trailers,omitempty"`
	Videos          []Video       `json:"videos,omitempty"`
	Collection      *Collection   `json:"collection,omitempty"`
	Extras          *Extras       `json:"app_extras,omitempty"`
	BehaviorHints   *MetaBehavior `json:"behaviorHints,omitempty"`
}

// Trailer points to a YouTube video.
type Trailer struct {
	Source string `json:"source"`
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Video is an episode of a series.
type Video struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	Name      string `json:"name,omitempty"`
	Season    Number `json:"season"`
	Episode   Number `json:"episode"`
	Number    Number `json:"number,omitempty"`
	Released  string `json:"released,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
	Overview  string `json:"overview,omitempty"`
	Available *bool  `json:"available,omitempty"`
	Runtime   Text   `json:"runtime,omitempty"`
}

// EpisodeNumber reads the episode number from either field addons use.
func (v Video) EpisodeNumber() int {
	if v.Episode != 0 {
		return int(v.Episode)
	}
	return int(v.Number)
}

// DisplayName is the episode's name from either field addons use.
func (v Video) DisplayName() string {
	if v.Title != "" {
		return v.Title
	}
	return v.Name
}

// Collection is an entry of a "collection" catalog: it groups catalogs,
// each optionally narrowed to a genre, or other collections.
type Collection struct {
	// Items are collections this one groups, as previews.
	Items []Meta `json:"items,omitempty"`
	// Sources are the catalogs this one groups.
	Sources []CollectionSource `json:"sources,omitempty"`
}

// CollectionSource is one catalog of a collection.
type CollectionSource struct {
	Type      string `json:"type"`
	CatalogID string `json:"catalogId"`
	Genre     string `json:"genre,omitempty"`
}

// Extras are details some metadata addons add.
type Extras struct {
	Cast                 []CastMember      `json:"cast,omitempty"`
	SeasonPosterByNumber map[string]string `json:"seasonPosterByNumber,omitempty"`
	Certification        string            `json:"certification,omitempty"`
}

// CastMember is an actor with their role.
type CastMember struct {
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Photo     string `json:"photo,omitempty"`
}

// MetaBehavior holds hints about how a title is played.
type MetaBehavior struct {
	DefaultVideoID string `json:"defaultVideoId,omitempty"`
}

// encodeComponent escapes like JavaScript's encodeURIComponent, which
// addons use to decode path segments.
func encodeComponent(value string) string {
	var out strings.Builder
	for _, b := range []byte(value) {
		if 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' || strings.IndexByte("-_.!~*'()", b) >= 0 {
			out.WriteByte(b)
			continue
		}
		fmt.Fprintf(&out, "%%%02X", b)
	}
	return out.String()
}

// ExtraValue is a resource request property, sent in order.
type ExtraValue struct {
	Name  string
	Value string
}

// extraPath encodes request properties as the path segment addons expect,
// with its leading slash, or nothing when there are none.
func extraPath(extra []ExtraValue) string {
	if len(extra) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(extra))
	for _, value := range extra {
		pairs = append(pairs, encodeComponent(value.Name)+"="+encodeComponent(value.Value))
	}
	return "/" + strings.Join(pairs, "&")
}

// Catalog lists one page of a catalog.
func (c *Client) Catalog(ctx context.Context, manifestURL, catalogType, catalogID string, extra []ExtraValue, confined bool) ([]Meta, error) {
	target := BaseURL(manifestURL) + "/catalog/" + encodeComponent(catalogType) + "/" + encodeComponent(catalogID) + extraPath(extra)
	body, err := c.get(ctx, target+".json", confined)
	if err != nil {
		return nil, err
	}
	var response struct {
		Metas []json.RawMessage `json:"metas"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: catalog: %v", ErrInvalidResponse, err)
	}
	// Every title keeps its place in the page, whose length tells where the
	// next page starts.
	metas := make([]Meta, len(response.Metas))
	for i, raw := range response.Metas {
		if err := unmarshal(raw, &metas[i]); err != nil {
			return nil, fmt.Errorf("%w: catalog: %v", ErrInvalidResponse, err)
		}
	}
	return metas, nil
}

// unmarshal decodes what an addon describes. A field of another type than
// Polyfin reads, such as an object where it reads a list, is left empty
// and the rest decoded, as encoding/json goes on past such a field: one
// odd field must not lose a title, or a catalog page.
func unmarshal(data []byte, v any) error {
	err := json.Unmarshal(data, v)
	if _, mismatched := errors.AsType[*json.UnmarshalTypeError](err); mismatched {
		return nil
	}
	return err
}

// Meta returns the complete description of a title.
func (c *Client) Meta(ctx context.Context, manifestURL, metaType, id string, confined bool) (Meta, error) {
	body, err := c.get(ctx, BaseURL(manifestURL)+"/meta/"+encodeComponent(metaType)+"/"+encodeComponent(id)+".json", confined)
	if err != nil {
		return Meta{}, err
	}
	var response struct {
		Meta *Meta `json:"meta"`
	}
	if err := unmarshal(body, &response); err != nil {
		return Meta{}, fmt.Errorf("%w: meta: %v", ErrInvalidResponse, err)
	}
	if response.Meta == nil || response.Meta.ID == "" {
		return Meta{}, ErrNotFound
	}
	return *response.Meta, nil
}
