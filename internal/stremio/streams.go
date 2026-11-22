package stremio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/moodiness/polyfin/internal/drm"
)

// Stream is one way to play a title, as an addon's stream resource lists it.
type Stream struct {
	Name string `json:"name,omitempty"`
	// Description, or Title for older addons, details the stream on several
	// lines.
	Description string `json:"description,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	// ExternalURL and YouTubeID are streams Polyfin does not play: a web
	// page and a YouTube video. InfoHash is a torrent's, played when the
	// server plays torrents (see Torrent): FileIdx is its file to play,
	// the largest without, and Sources its trackers and DHT nodes.
	ExternalURL   string         `json:"externalUrl,omitempty"`
	YouTubeID     string         `json:"ytId,omitempty"`
	InfoHash      string         `json:"infoHash,omitempty"`
	FileIdx       *int           `json:"fileIdx,omitempty"`
	Sources       []string       `json:"sources,omitempty"`
	Subtitles     []Subtitle     `json:"subtitles,omitempty"`
	BehaviorHints StreamBehavior `json:"behaviorHints,omitzero"`
	// Data is what some addons add of their own about the stream, read
	// leniently (see StreamData).
	Data StreamData `json:"streamData,omitzero"`
	// Usenet is a Usenet download's NZB link, and RARs to Tars archives'
	// files: streams Polyfin does not play. They are decoded only to count
	// them in a log line, never asked for.
	Usenet   string            `json:"nzbUrl,omitempty"`
	RARs     []json.RawMessage `json:"rarUrls,omitempty"`
	Zips     []json.RawMessage `json:"zipUrls,omitempty"`
	SevenZip []json.RawMessage `json:"7zipUrls,omitempty"`
	TGZs     []json.RawMessage `json:"tgzUrls,omitempty"`
	Tars     []json.RawMessage `json:"tarUrls,omitempty"`
	// Identity is Polyfin's, never an addon's: the file an earlier answer
	// of the addon listed the stream as, when the stream's link or hints
	// changed since, empty otherwise.
	Identity string `json:"-"`
	// UserAgent is Polyfin's, never an addon's: the User-Agent Polyfin's
	// own requests for the stream send when its ProxyHeaders name none.
	// Unlike those, it is no header the stream needs: an app that can
	// reach the source itself may still be sent there.
	UserAgent string `json:"-"`
	// Protection is Polyfin's, never an addon's: how an IPTV list says the
	// stream is encrypted, with its ClearKey keys, which never leave
	// Polyfin; nil when it says nothing.
	Protection *drm.Protection `json:"-"`
	// Confined is Polyfin's, never an addon's: set for a stream whose
	// address the source's owner gave themselves, as an IPTV channel's
	// custom stream, it tells whether the stream is fetched from public
	// addresses only, as that owner may, rather than as what the source's
	// answers name is; nil for a stream the source named.
	Confined *bool `json:"-"`
	// Remote is Polyfin's, never an addon's: set for a stream of one of
	// Polyfin's local folders that names an address to play, as a .strm
	// file does, rather than a file the folder holds; it plays as an
	// addon's link does.
	Remote bool `json:"-"`
}

// StreamBehavior holds hints about how a stream is played.
type StreamBehavior struct {
	// BingeGroup names streams of the same kind across episodes, so that
	// the next episode can be played from the same source.
	BingeGroup string   `json:"bingeGroup,omitempty"`
	Filename   string   `json:"filename,omitempty"`
	VideoSize  Number64 `json:"videoSize,omitempty"`
	VideoHash  string   `json:"videoHash,omitempty"`
	// ProxyHeaders are the headers the stream must be requested with.
	ProxyHeaders *ProxyHeaders `json:"proxyHeaders,omitempty"`
	NotWebReady  bool          `json:"notWebReady,omitempty"`
}

// ProxyHeaders are headers a player must send with its requests, or would
// receive from a proxy.
type ProxyHeaders struct {
	Request  map[string]string `json:"request,omitempty"`
	Response map[string]string `json:"response,omitempty"`
}

// StreamData is the streamData some addons add to a stream: the debrid
// service holding its file and whether the file is ready there (cached),
// and the seeders of its torrent. It is read leniently: a value of
// another type than expected reads as unknown, and never fails the
// stream; cached may be a boolean, a string or a number. Cached counts
// only with a service named.
type StreamData struct {
	Service StreamService `json:"service,omitzero"`
	Torrent StreamTorrent `json:"torrent,omitzero"`
}

// StreamService is the debrid service a stream's file is on: its
// identifier, and whether it has the file ready, nil when unknown.
type StreamService struct {
	ID     string `json:"id,omitempty"`
	Cached *bool  `json:"cached,omitempty"`
}

// StreamTorrent is what an addon tells of a stream's torrent: its
// seeders, 0 when unknown.
type StreamTorrent struct {
	Seeders Number `json:"seeders,omitempty"`
}

func (d *StreamData) UnmarshalJSON(data []byte) error {
	*d = StreamData{}
	var fields struct {
		Service json.RawMessage `json:"service"`
		Torrent json.RawMessage `json:"torrent"`
	}
	if json.Unmarshal(data, &fields) != nil {
		return nil
	}
	var service struct {
		ID     Text            `json:"id"`
		Cached json.RawMessage `json:"cached"`
	}
	if json.Unmarshal(fields.Service, &service) == nil {
		d.Service.ID = strings.TrimSpace(string(service.ID))
		if d.Service.ID != "" {
			d.Service.Cached = looseBool(service.Cached)
		}
	}
	var torrent struct {
		Seeders Number `json:"seeders"`
	}
	if json.Unmarshal(fields.Torrent, &torrent) == nil {
		d.Torrent.Seeders = max(torrent.Seeders, 0)
	}
	return nil
}

// looseBool reads a JSON boolean, a string saying true or false, yes or
// no, in any letter case, or a number, 0 being false; nil for anything
// else.
func looseBool(data json.RawMessage) *bool {
	var value bool
	if json.Unmarshal(data, &value) == nil {
		return &value
	}
	var text Text
	_ = text.UnmarshalJSON(data)
	switch written := strings.ToLower(strings.TrimSpace(string(text))); written {
	case "true", "yes":
		value = true
	case "false", "no":
	default:
		number, err := strconv.ParseFloat(written, 64)
		if err != nil || math.IsNaN(number) {
			return nil
		}
		value = number != 0
	}
	return &value
}

// Playable reports whether the stream is a direct link to a video.
func (s Stream) Playable() bool { return webURL(s.URL) }

// Torrent reports whether the stream is a torrent's file, with no link: an
// info hash of 40 hexadecimal digits, and a file index, when given, that
// is not negative.
func (s Stream) Torrent() bool {
	raw, err := hex.DecodeString(s.InfoHash)
	return s.URL == "" && err == nil && len(raw) == 20 && (s.FileIdx == nil || *s.FileIdx >= 0)
}

// Archive reports whether the stream is an archive's file, which Polyfin
// does not play.
func (s Stream) Archive() bool {
	return len(s.RARs)+len(s.Zips)+len(s.SevenZip)+len(s.TGZs)+len(s.Tars) > 0
}

// Details returns the description, from whichever field the addon used.
func (s Stream) Details() string {
	if s.Description != "" {
		return s.Description
	}
	return s.Title
}

// RequestHeaders returns the headers the stream must be requested with.
func (s Stream) RequestHeaders() map[string]string {
	if s.BehaviorHints.ProxyHeaders == nil {
		return nil
	}
	return s.BehaviorHints.ProxyHeaders.Request
}

// Subtitle is a subtitle file an addon offers for a title or a stream.
type Subtitle struct {
	ID  Text   `json:"id"`
	URL string `json:"url"`
	// Lang is usually an ISO 639-2 code, sometimes a language name.
	Lang string `json:"lang"`
	// Polyfin's own local folders also tell a file's format ("srt",
	// "ass", "ssa" or "vtt"), the title and flags its name gives; addons
	// never do.
	Format                           string `json:"-"`
	Title                            string `json:"-"`
	Forced, HearingImpaired, Default bool   `json:"-"`
}

// Valid reports whether the subtitle can be downloaded.
func (s Subtitle) Valid() bool { return webURL(s.URL) }

func webURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// Serves reports whether the addon serves a resource for content of this
// type and identifier.
func (m Manifest) Serves(resource, contentType, id string) bool {
	for _, r := range m.Resources {
		if r.Name != resource {
			continue
		}
		types, prefixes := r.Types, r.IDPrefixes
		if len(types) == 0 {
			types = m.Types
		}
		if len(prefixes) == 0 {
			prefixes = m.IDPrefixes
		}
		if !slices.Contains(types, contentType) {
			continue
		}
		if len(prefixes) == 0 || slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(id, prefix) }) {
			return true
		}
	}
	return false
}

// ServesType reports whether the addon serves a resource for content of
// this type, whichever identifiers it serves it for.
func (m Manifest) ServesType(resource, contentType string) bool {
	return slices.ContainsFunc(m.Resources, func(r Resource) bool {
		types := r.Types
		if len(types) == 0 {
			types = m.Types
		}
		return r.Name == resource && slices.Contains(types, contentType)
	})
}

// Streams lists the streams an addon offers for a title, or for an episode
// by its video identifier, waiting StreamTimeout for its answer. Of its
// list, the first MaxListed elements are read, and those that tell
// nothing, as an empty object, are left out (see tells). The notices it
// lists, with streams or without, are recorded for Health.
func (c *Client) Streams(ctx context.Context, manifestURL, streamType, id string, confined bool) ([]Stream, error) {
	body, err := c.get(ctx, manifestURL, ResourceURL(manifestURL, "/stream/"+encodeComponent(streamType)+"/"+encodeComponent(id)+".json"), confined, c.streamTimeout)
	if err != nil {
		return nil, err
	}
	var response struct {
		Streams json.RawMessage `json:"streams"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: streams: %v", ErrInvalidResponse, err)
	}
	streams, err := decodeList(response.Streams, MaxListed, &room{left: listedRoom}, decodeStream, tells)
	if err != nil {
		return nil, fmt.Errorf("%w: streams: %v", ErrInvalidResponse, err)
	}
	c.health.noticed(ctx, manifestURL, noticesOf(streams))
	return streams, nil
}

// Subtitles lists the subtitles an addon offers for a title, or for an
// episode by its video identifier. extra describes the video they should
// match (filename, videoSize, videoHash), when known. Of its list, the
// first MaxListed elements are read, and those that cannot be downloaded
// are left out.
func (c *Client) Subtitles(ctx context.Context, manifestURL, subtitlesType, id string, extra []ExtraValue, confined bool) ([]Subtitle, error) {
	target := ResourceURL(manifestURL, "/subtitles/"+encodeComponent(subtitlesType)+"/"+encodeComponent(id)+extraPath(extra)+".json")
	body, err := c.get(ctx, manifestURL, target, confined, c.timeout)
	if err != nil {
		return nil, err
	}
	var response struct {
		Subtitles json.RawMessage `json:"subtitles"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: subtitles: %v", ErrInvalidResponse, err)
	}
	subtitles, err := decodeList(response.Subtitles, MaxListed, &room{left: listedRoom}, whole[Subtitle], downloadable)
	if err != nil {
		return nil, fmt.Errorf("%w: subtitles: %v", ErrInvalidResponse, err)
	}
	return subtitles, nil
}
