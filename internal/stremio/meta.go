package stremio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

// Posters decodes a list of image URLs whose places matter: an element
// that is not a string, such as null for an image missing, is kept as an
// empty string. Any other value is left out rather than failing the title.
type Posters []string

func (p *Posters) UnmarshalJSON(data []byte) error {
	*p = nil
	var elements []json.RawMessage
	if json.Unmarshal(data, &elements) != nil {
		return nil
	}
	*p = make(Posters, len(elements))
	for i, element := range elements {
		_ = json.Unmarshal(element, &(*p)[i])
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
	PosterShape     string        `json:"posterShape,omitempty"` // see Shape
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
	// ChannelNumber and GuideID describe a channel of Polyfin's own IPTV
	// sources: the number its provider gives it, 0 for none, and its
	// channel in XMLTV guides; GuideNumber is a tuner's number of the
	// channel ("5.1"), which it shows and which guide channels may be
	// named by; ChannelGroup is the key of the line-up category it shows
	// in, which refreshes keep. Addons never send them.
	ChannelNumber int    `json:"-"`
	GuideID       string `json:"-"`
	GuideNumber   string `json:"-"`
	ChannelGroup  string `json:"-"`
}

// The shapes of a poster an addon declares (Meta.PosterShape) other than
// the usual 2:3.
const (
	ShapeLandscape = "landscape"
	ShapeSquare    = "square"
)

// Shape is the shape of the meta's poster: ShapeLandscape, ShapeSquare,
// or empty for a poster of 2:3, as the addon protocol takes a missing or
// unknown shape.
func (m Meta) Shape() string {
	switch m.PosterShape {
	case ShapeLandscape, ShapeSquare:
		return m.PosterShape
	}
	return ""
}

// Trailer points to a YouTube video.
type Trailer struct {
	Source string `json:"source"`
	Type   string `json:"type,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Video is an episode of a series, a video of another title, or a
// programme of a live TV channel.
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
	// StartTime and EndTime, ISO 8601, make a video a programme of a
	// channel's guide; Genres are its categories.
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	Genres    Names  `json:"genres,omitempty"`
	// Streams are the video's own streams, which Stremio apps play
	// without asking the stream resource (see library's embedded
	// streams).
	Streams []Stream `json:"streams,omitempty"`
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

// Extras are details some metadata addons add. SeasonPosters holds a
// series' season posters in the order of its seasons, without their
// numbers (see library.seasonPosters); SeasonPosterByNumber holds them by
// season number.
type Extras struct {
	Cast                 []CastMember      `json:"cast,omitempty"`
	SeasonPosterByNumber map[string]string `json:"seasonPosterByNumber,omitempty"`
	SeasonPosters        Posters           `json:"seasonPosters,omitempty"`
	// Certification is a title's age rating, often the US one;
	// CertificationLocal is that of the country of the addon's language,
	// or the US one again when that country has none.
	Certification      string `json:"certification,omitempty"`
	CertificationLocal string `json:"certificationLocal,omitempty"`
}

// UnmarshalJSON reads Extras, whose seasonPosters some addons send as a
// list in season order and others as an object keyed by season number,
// such as {"0": …, "1": …}, depending on the database a series came from.
// The object form names its seasons, so it joins SeasonPosterByNumber,
// where seasonPosterByNumber itself wins.
func (e *Extras) UnmarshalJSON(data []byte) error {
	type plain Extras
	var decoded struct {
		plain
		// Shallower than plain's field of the same name, so it takes the
		// value whatever its form.
		SeasonPosters json.RawMessage `json:"seasonPosters"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = Extras(decoded.plain)
	if len(decoded.SeasonPosters) == 0 {
		return nil
	}
	_ = json.Unmarshal(decoded.SeasonPosters, &e.SeasonPosters)
	var byNumber map[string]json.RawMessage
	if json.Unmarshal(decoded.SeasonPosters, &byNumber) != nil {
		return nil
	}
	for number, raw := range byNumber {
		var poster string
		if _, err := strconv.Atoi(number); err != nil || json.Unmarshal(raw, &poster) != nil || poster == "" {
			continue
		}
		if e.SeasonPosterByNumber == nil {
			e.SeasonPosterByNumber = map[string]string{}
		}
		if _, set := e.SeasonPosterByNumber[number]; !set {
			e.SeasonPosterByNumber[number] = poster
		}
	}
	return nil
}

// CastMember is an actor with their role. Some addons identify the actor:
// by their TMDB identifier, as tmdb_id or as a numeric id (see
// PersonTMDB).
type CastMember struct {
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Photo     string `json:"photo,omitempty"`
	ID        Text   `json:"id,omitempty"`
	TmdbID    Text   `json:"tmdb_id,omitempty"`
}

// PersonTMDB is the actor's TMDB identifier, a number, empty when the
// addon gives none.
func (m CastMember) PersonTMDB() string {
	for _, id := range []Text{m.TmdbID, m.ID} {
		text := strings.TrimSpace(string(id))
		if value, err := strconv.Atoi(text); err == nil && value > 0 {
			return strconv.Itoa(value)
		}
	}
	return ""
}

// described is a meta as an addon writes it, with its links, which the
// Stremio protocol gives in place of the cast, director and writer lists
// it deprecates. Polyfin reads the people of links into those lists (see
// decodeMeta) and keeps no other link.
type described struct {
	Meta
	Links links `json:"links"`
}

// link is one of a meta's links, which Stremio apps list by category.
type link struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// links decodes a meta's links. An element that is not a link, such as
// one whose name is not a string, is left out rather than failing the
// title, as is any other value.
type links []link

func (l *links) UnmarshalJSON(data []byte) error {
	*l = nil
	var elements []json.RawMessage
	if json.Unmarshal(data, &elements) != nil {
		return nil
	}
	for _, element := range elements {
		var entry link
		if json.Unmarshal(element, &entry) == nil {
			*l = append(*l, entry)
		}
	}
	return nil
}

// The categories of links that credit people, in lower case: the plural
// ones Stremio apps show, such as "Cast" and "Directors", and the singular
// ones the addon SDK's documentation recommends, such as "actor".
var (
	castCategories     = []string{"cast", "actor", "actors"}
	directorCategories = []string{"director", "directors"}
	writerCategories   = []string{"writer", "writers"}
)

// credit fills each of meta's cast, director and writer lists that is
// empty with the names of the links of its categories, in any letter case,
// in their order.
func (l links) credit(meta *Meta) {
	for _, list := range []struct {
		names      *Names
		categories []string
	}{{&meta.Cast, castCategories}, {&meta.Director, directorCategories}, {&meta.Writer, writerCategories}} {
		if len(*list.names) > 0 {
			continue
		}
		for _, entry := range l {
			name := strings.TrimSpace(entry.Name)
			if name != "" && slices.Contains(list.categories, strings.ToLower(strings.TrimSpace(entry.Category))) {
				*list.names = append(*list.names, name)
			}
		}
	}
}

// decodeMeta decodes a meta an addon sends, as unmarshal does, its cast,
// directors and writers read from its links when it gives no list of them.
func decodeMeta(data []byte, meta *Meta) error {
	var decoded described
	err := unmarshal(data, &decoded)
	*meta = decoded.Meta
	decoded.Links.credit(meta)
	return err
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
	body, err := c.get(ctx, manifestURL, target+".json", confined)
	if err != nil {
		return nil, err
	}
	// Guide pages, those of live TV catalogs asked for a date, list their
	// channels with their programmes as metasDetailed.
	var response struct {
		Metas         []json.RawMessage `json:"metas"`
		MetasDetailed []json.RawMessage `json:"metasDetailed"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: catalog: %v", ErrInvalidResponse, err)
	}
	if response.Metas == nil {
		response.Metas = response.MetasDetailed
	}
	// Every title keeps its place in the page, whose length tells where the
	// next page starts.
	metas := make([]Meta, len(response.Metas))
	for i, raw := range response.Metas {
		if err := decodeMeta(raw, &metas[i]); err != nil {
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
	body, err := c.get(ctx, manifestURL, BaseURL(manifestURL)+"/meta/"+encodeComponent(metaType)+"/"+encodeComponent(id)+".json", confined)
	if err != nil {
		return Meta{}, err
	}
	var response struct {
		Meta *described `json:"meta"`
	}
	if err := unmarshal(body, &response); err != nil {
		return Meta{}, fmt.Errorf("%w: meta: %v", ErrInvalidResponse, err)
	}
	if response.Meta == nil || response.Meta.ID == "" {
		return Meta{}, ErrNotFound
	}
	meta := response.Meta.Meta
	response.Meta.Links.credit(&meta)
	return meta, nil
}
