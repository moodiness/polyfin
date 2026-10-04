package iptv

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode"
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
}

// maxLine bounds a playlist line: a line longer than that is not M3U.
const maxLine = 1 << 20

// ParseM3U reads an M3U playlist, handing each channel to entry in order:
// the #EXTINF line's tvg-id, tvg-name, tvg-logo, tvg-chno and group-title
// attributes (quoted with double or single quotes, or not at all) and the
// name after its comma, then the #EXTVLCOPT lines giving the stream's user
// agent and referrer, then the stream's address. A byte order mark and CRLF
// line ends are accepted. Headings that are not channels are skipped (see
// heading), as are entries without an address. A playlist that does not
// start with #EXTM3U, or whose lines are too long, is not M3U.
func ParseM3U(r io.Reader, entry func(Entry) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxLine)
	first := true
	var current *Entry
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
			first = false
			continue
		}
		switch {
		case text == "":
		case hasPrefixFold(text, "#EXTINF:"):
			e := parseExtinf(text[len("#EXTINF:"):])
			current = &e
		case hasPrefixFold(text, "#EXTVLCOPT:"):
			if current == nil {
				continue
			}
			option, value, _ := strings.Cut(text[len("#EXTVLCOPT:"):], "=")
			header := map[string]string{"http-user-agent": "User-Agent", "http-referrer": "Referer", "http-referer": "Referer"}[strings.ToLower(strings.TrimSpace(option))]
			if value = strings.TrimSpace(value); header != "" && value != "" {
				if current.Headers == nil {
					current.Headers = map[string]string{}
				}
				current.Headers[header] = value
			}
		case hasPrefixFold(text, "#EXTGRP:"):
			if current != nil && current.Group == "" {
				current.Group = strings.TrimSpace(text[len("#EXTGRP:"):])
			}
		case strings.HasPrefix(text, "#"):
		default:
			if current == nil {
				continue
			}
			e := *current
			current = nil
			e.URL = text
			if !streamAddress(e.URL) || heading(e.Name) {
				continue
			}
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

// streamAddress reports whether a playlist line is an address Polyfin can
// play from.
func streamAddress(address string) bool {
	scheme, _, ok := strings.Cut(address, "://")
	return ok && (strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https"))
}

// parseExtinf reads what follows "#EXTINF:": a duration, attributes, then
// the name after the first comma outside quotes.
func parseExtinf(text string) Entry {
	attributes := map[string]string{}
	i := 0
	// The duration, such as -1 or 0.
	for i < len(text) && text[i] != ' ' && text[i] != '\t' && text[i] != ',' {
		i++
	}
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
	name := ""
	if i < len(text) {
		name = strings.TrimSpace(text[i+1:])
	}
	number, _ := strconv.Atoi(attributes["tvg-chno"])
	return Entry{
		Name:    cmpOr(name, attributes["tvg-name"]),
		Number:  max(number, 0),
		Logo:    attributes["tvg-logo"],
		Group:   attributes["group-title"],
		GuideID: attributes["tvg-id"],
	}
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
