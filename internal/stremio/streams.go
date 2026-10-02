package stremio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Stream is one way to play a title, as an addon's stream resource lists it.
type Stream struct {
	Name string `json:"name,omitempty"`
	// Description, or Title for older addons, details the stream on several
	// lines.
	Description string `json:"description,omitempty"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	// ExternalURL, YouTubeID and InfoHash are streams Polyfin does not play:
	// a web page, a YouTube video and a torrent.
	ExternalURL   string         `json:"externalUrl,omitempty"`
	YouTubeID     string         `json:"ytId,omitempty"`
	InfoHash      string         `json:"infoHash,omitempty"`
	Subtitles     []Subtitle     `json:"subtitles,omitempty"`
	BehaviorHints StreamBehavior `json:"behaviorHints,omitzero"`
}

// StreamBehavior holds hints about how a stream is played.
type StreamBehavior struct {
	// BingeGroup names streams of the same kind across episodes, so that
	// the next episode can be played from the same source.
	BingeGroup string `json:"bingeGroup,omitempty"`
	Filename   string `json:"filename,omitempty"`
	VideoSize  Number `json:"videoSize,omitempty"`
	VideoHash  string `json:"videoHash,omitempty"`
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

// Playable reports whether the stream is a direct link to a video, the only
// kind Polyfin plays.
func (s Stream) Playable() bool { return webURL(s.URL) }

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

// Streams lists the streams an addon offers for a title, or for an episode
// by its video identifier.
func (c *Client) Streams(ctx context.Context, manifestURL, streamType, id string, confined bool) ([]Stream, error) {
	body, err := c.get(ctx, BaseURL(manifestURL)+"/stream/"+encodeComponent(streamType)+"/"+encodeComponent(id)+".json", confined)
	if err != nil {
		return nil, err
	}
	var response struct {
		Streams []Stream `json:"streams"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: streams: %v", ErrInvalidResponse, err)
	}
	return response.Streams, nil
}

// Subtitles lists the subtitles an addon offers for a title, or for an
// episode by its video identifier. extra describes the video they should
// match (filename, videoSize, videoHash), when known.
func (c *Client) Subtitles(ctx context.Context, manifestURL, subtitlesType, id string, extra []ExtraValue, confined bool) ([]Subtitle, error) {
	target := BaseURL(manifestURL) + "/subtitles/" + encodeComponent(subtitlesType) + "/" + encodeComponent(id) + extraPath(extra)
	body, err := c.get(ctx, target+".json", confined)
	if err != nil {
		return nil, err
	}
	var response struct {
		Subtitles []Subtitle `json:"subtitles"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("%w: subtitles: %v", ErrInvalidResponse, err)
	}
	return response.Subtitles, nil
}
