package iptv

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/http/httpguts"

	"github.com/moodiness/polyfin/internal/drm"
)

// Entry is a channel of a provider's list, as read from an M3U playlist or
// an Xtream Codes account. Number is 0 when the list gives none.
type Entry struct {
	// ID identifies the channel within the provider's list, when it has
	// its own identifiers (an Xtream stream); empty otherwise.
	ID      string
	Name    string
	Number  int
	Logo    string
	Group   string
	GuideID string
	URL     string
	Headers map[string]string
	// Kind is what an M3U entry is, told by its address or name: a live
	// channel (""), KindMovie or KindEpisode, an episode of the series
	// Series with its Season and Episode numbers, 0 when its name gives
	// none.
	Kind            string
	Series          string
	Season, Episode int
	// Catchup is how the provider's archive of the channel's past
	// programmes is reached; the zero value for none.
	Catchup Catchup
	// GuideNumber is a tuner's number of the channel ("5.1"), and
	// Favorite tells a channel marked favorite on the tuner.
	GuideNumber string
	Favorite    bool
	// Protection is how the list says the stream is encrypted, by its
	// #KODIPROP lines (see kodiProperty); nil when it says nothing. Its
	// ClearKey keys are kept with the list's other data, and nowhere else.
	Protection *drm.Protection
}

// maxLine bounds a playlist line: a line longer than that is not M3U.
const maxLine = 1 << 20

// ParseM3U reads an M3U playlist, handing each channel to entry in order:
// the #EXTINF line's tvg-id, tvg-name, tvg-logo, tvg-chno and group-title
// attributes (quoted with double or single quotes, or not at all) and the
// name after its comma, its catch-up attributes over those the #EXTM3U
// line gives every channel (see catchupOf), then the headers its stream is
// requested with, then the stream's address. The headers are those the
// players that read such lists take, in this order, a later one replacing
// an earlier one of the same name: the #EXTINF line's http-user-agent,
// http-referrer and http-origin attributes, then #EXTVLCOPT lines' options
// of the same names and #EXTHTTP lines' JSON object, as the lines come,
// then the manifest and stream headers of #KODIPROP lines, then the
// Name=value pairs after a | ending the address (see cutHeaders). The
// #KODIPROP lines since the previous address are the entry's, before its
// #EXTINF line or after it (see kodiProperty). A byte order mark and CRLF
// line ends are accepted. Headings that are not channels are skipped (see
// heading), as are entries without an address. A playlist that does not
// start with #EXTM3U, or whose lines are too long, is not M3U.
func ParseM3U(r io.Reader, entry func(Entry) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxLine)
	first := true
	var current *Entry
	var header Catchup
	// properties are the #KODIPROP lines since the previous address.
	var properties [][2]string
	for scanner.Scan() {
		line := scanner.Bytes()
		if first {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
		}
		text := strings.TrimSpace(string(line))
		if first {
			if !strings.HasPrefix(text, "#EXTM3U") {
				return ErrInvalidList
			}
			attributes, _ := readAttributes(text[len("#EXTM3U"):])
			header = catchupOf(attributes, Catchup{})
			first = false
			continue
		}
		switch {
		case text == "":
		case hasPrefixFold(text, "#EXTINF:"):
			e := parseExtinf(text[len("#EXTINF:"):], header)
			current = &e
		case hasPrefixFold(text, "#EXTVLCOPT:"):
			if current == nil {
				continue
			}
			option, value, _ := strings.Cut(text[len("#EXTVLCOPT:"):], "=")
			current.setHeader(vlcOptions[strings.ToLower(strings.TrimSpace(option))], value)
		case hasPrefixFold(text, "#EXTHTTP:"):
			var fields map[string]any
			if current == nil || json.Unmarshal([]byte(text[len("#EXTHTTP:"):]), &fields) != nil {
				continue
			}
			for _, name := range slices.Sorted(maps.Keys(fields)) {
				if value, ok := fields[name].(string); ok {
					current.setHeader(name, value)
				}
			}
		case hasPrefixFold(text, "#EXTGRP:"):
			if current != nil && current.Group == "" {
				current.Group = strings.TrimSpace(text[len("#EXTGRP:"):])
			}
		case hasPrefixFold(text, "#KODIPROP:"):
			name, value, _ := strings.Cut(text[len("#KODIPROP:"):], "=")
			properties = append(properties, [2]string{name, value})
		case strings.HasPrefix(text, "#"):
		default:
			kept := properties
			properties = nil
			if current == nil {
				continue
			}
			e := *current
			current = nil
			for _, property := range kept {
				e.kodiProperty(property[0], property[1])
			}
			address, headers := cutHeaders(text)
			e.URL = address
			for _, name := range slices.Sorted(maps.Keys(headers)) {
				e.setHeader(name, headers[name])
			}
			if !streamAddress(e.URL) || heading(e.Name) {
				continue
			}
			e.classify()
			e.Catchup = e.Catchup.normalized()
			if err := entry(e); err != nil {
				return err
			}
		}
	}
	if errors.Is(scanner.Err(), bufio.ErrTooLong) {
		return ErrInvalidList
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if first {
		return ErrInvalidList
	}
	return nil
}

func hasPrefixFold(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

// vlcOptions are the #EXTVLCOPT options that name a request header.
var vlcOptions = map[string]string{"http-user-agent": "User-Agent", "http-referrer": "Referer", "http-referer": "Referer", "http-origin": "Origin"}

// unsetHeaders are the headers a list may not set: they shape the request
// itself, or the part of the stream it reads.
var unsetHeaders = []string{"Connection", "Content-Length", "Host", "If-Range", "Range", "Transfer-Encoding"}

// setHeader sets a request header of the entry's stream, named as HTTP
// writes it. An empty value, a name or value HTTP cannot carry, or a
// header of unsetHeaders, is skipped.
func (e *Entry) setHeader(name, value string) {
	name, value = http.CanonicalHeaderKey(strings.TrimSpace(name)), strings.TrimSpace(value)
	if value == "" || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) || slices.Contains(unsetHeaders, name) {
		return
	}
	if e.Headers == nil {
		e.Headers = map[string]string{}
	}
	e.Headers[name] = value
}

// cutHeaders splits a playlist's address line of the form
// address|Name=value&Name=value, as several players read the headers of
// a stream, into the address and those headers, each value unescaped
// when it is percent-encoded. A line whose part after the first | is not
// such pairs is an address whole.
func cutHeaders(line string) (string, map[string]string) {
	address, rest, found := strings.Cut(line, "|")
	if !found {
		return line, nil
	}
	headers, ok := headerPairs(rest)
	if !ok {
		return line, nil
	}
	return strings.TrimSpace(address), headers
}

// headerPairs reads headers written Name=value&Name=value, each value
// unescaped when it is percent-encoded; false when text is not such pairs.
func headerPairs(text string) (map[string]string, bool) {
	headers := map[string]string{}
	for pair := range strings.SplitSeq(text, "&") {
		name, value, ok := strings.Cut(pair, "=")
		if name = strings.TrimSpace(name); !ok || !httpguts.ValidHeaderFieldName(name) {
			return nil, false
		}
		if unescaped, err := url.PathUnescape(value); err == nil {
			value = unescaped
		}
		headers[name] = value
	}
	return headers, true
}

// kodiProperty reads a #KODIPROP line's property, as lists write those of
// adaptive streams: inputstream.adaptive.manifest_type (mpd, hls),
// license_type (org.w3.clearkey, com.widevine.alpha...), license_key
// (ClearKey keys, or a licence server's address, see licenseKey), and
// manifest_headers and stream_headers, Name=value pairs apart by &.
// Polyfin sends one set of headers with every request of a stream: both
// are its headers. Other properties are not read.
func (e *Entry) kodiProperty(name, value string) {
	name, value = strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(value)
	property, ok := strings.CutPrefix(name, "inputstream.adaptive.")
	if !ok || value == "" {
		return
	}
	protection := func() *drm.Protection {
		if e.Protection == nil {
			e.Protection = &drm.Protection{}
		}
		return e.Protection
	}
	switch property {
	case "manifest_type":
		protection().Manifest = strings.ToLower(value)
	case "license_type":
		protection().License = drm.LicenseType(value)
	case "license_key":
		licenseKey(protection(), value)
	case "manifest_headers", "stream_headers":
		headers, _ := headerPairs(value)
		for _, name := range slices.Sorted(maps.Keys(headers)) {
			e.setHeader(name, headers[name])
		}
	}
}

// licenseKey reads a license_key property: ClearKey keys (see
// drm.ParseKeys), which make the licence ClearKey when the list names no
// type; or a licence server's address, written address|headers|... as
// such lists write it, headers being Name=value pairs apart by &.
func licenseKey(p *drm.Protection, value string) {
	if streamAddress(value) {
		address, rest, _ := strings.Cut(value, "|")
		p.LicenseURL = strings.TrimSpace(address)
		fields, _, _ := strings.Cut(rest, "|")
		if headers, ok := headerPairs(fields); ok && strings.TrimSpace(fields) != "" {
			p.LicenseHeaders = headers
		}
		return
	}
	if keys, ok := drm.ParseKeys(value); ok {
		p.Keys = keys
		if p.License == "" {
			p.License = drm.ClearKey
		}
	}
}

// streamAddress reports whether a playlist line is an address Polyfin can
// play from.
func streamAddress(address string) bool {
	scheme, _, ok := strings.Cut(address, "://")
	return ok && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}

// parseExtinf reads what follows "#EXTINF:": a duration, attributes, then
// the name after the first comma outside quotes. header is the catch-up
// the playlist's #EXTM3U line gives every channel. The http-user-agent,
// http-referrer and http-origin attributes give headers, as #EXTVLCOPT
// lines do: some lists give them on the #EXTINF line only. The lines that
// follow replace them.
func parseExtinf(text string, header Catchup) Entry {
	i := 0
	// The duration, such as -1 or 0.
	for i < len(text) && text[i] != ' ' && text[i] != '\t' && text[i] != ',' {
		i++
	}
	attributes, end := readAttributes(text[i:])
	i += end
	name := ""
	if i < len(text) {
		name = strings.TrimSpace(text[i+1:])
	}
	number, _ := strconv.Atoi(attributes["tvg-chno"])
	e := Entry{
		Name:    cmpOr(name, attributes["tvg-name"]),
		Number:  max(number, 0),
		Logo:    attributes["tvg-logo"],
		Group:   attributes["group-title"],
		GuideID: attributes["tvg-id"],
		Catchup: catchupOf(attributes, header),
	}
	for _, option := range slices.Sorted(maps.Keys(vlcOptions)) {
		if value, ok := attributes[option]; ok {
			e.setHeader(vlcOptions[option], value)
		}
	}
	return e
}

// readAttributes reads attributes, key=value pairs whose value is quoted
// with double or single quotes or not at all, up to the first comma
// outside quotes, whose index it returns (the text's length without one).
// Keys are lower-cased.
func readAttributes(text string) (map[string]string, int) {
	attributes := map[string]string{}
	i := 0
	for i < len(text) && text[i] != ',' {
		if text[i] == ' ' || text[i] == '\t' {
			i++
			continue
		}
		start := i
		for i < len(text) && text[i] != '=' && text[i] != ' ' && text[i] != ',' {
			i++
		}
		key := strings.ToLower(text[start:i])
		if i >= len(text) || text[i] != '=' {
			continue
		}
		i++
		var value string
		if i < len(text) && (text[i] == '"' || text[i] == '\'') {
			quote := text[i]
			end := strings.IndexByte(text[i+1:], quote)
			if end < 0 {
				value, i = text[i+1:], len(text)
			} else {
				value, i = text[i+1:i+1+end], i+2+end
			}
		} else {
			start := i
			for i < len(text) && text[i] != ' ' && text[i] != ',' {
				i++
			}
			value = text[start:i]
		}
		attributes[key] = strings.TrimSpace(value)
	}
	return attributes, i
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// decoration are the characters lists draw headings with.
const decoration = "#=-*~_:|/\\<>[](){}"

// heading reports whether a list entry is a heading rather than a channel:
// a name without letters or digits, or drawn with a run of three or more
// decoration characters, such as "##### NAME #####" or "=== NAME ===".
// Channel names do not carry such runs; headings do, to stand out.
func heading(name string) bool {
	run, alphanumeric := 0, false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			alphanumeric, run = true, 0
		case strings.ContainsRune(decoration, r) || unicode.Is(unicode.So, r) || unicode.Is(unicode.Pd, r):
			if run++; run >= 3 {
				return true
			}
		default:
			run = 0
		}
	}
	return !alphanumeric
}
