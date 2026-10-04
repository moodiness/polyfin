// Package xmltv reads guides in the XMLTV format, which IPTV providers
// publish beside their channel lists, and matches their channels with the
// names channels are listed under.
package xmltv

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/htmlindex"
)

// MaxSize is how many bytes of a guide are downloaded at most. A
// compressed guide may expand to maxExpansion times as much XML, which
// bounds the work a small file can cause.
const (
	MaxSize      = 300 << 20
	maxExpansion = 4
)

var (
	// ErrTooLarge reports a guide over the size limit.
	ErrTooLarge = errors.New("the guide is too large")
	// ErrMalformed reports a document that is not an XMLTV guide.
	ErrMalformed = errors.New("not an XMLTV guide")
)

// Channel is a channel of a guide, with the names it goes by.
type Channel struct {
	ID    string
	Names []string
}

// Programme is a programme of a guide's channel. Season and Episode count
// from 1; 0 when the guide does not number it.
type Programme struct {
	Channel     string
	Start, Stop time.Time
	Title       string
	SubTitle    string
	Description string
	Categories  []string
	Season      int
	Episode     int
	Icon        string
}

// Options tune Read. Limit is MaxSize when zero; Language is the language
// titles and descriptions are preferred in, when the guide has several.
type Options struct {
	Limit    int64
	Language string
}

// Read decodes a guide from r, plain or gzip-compressed (told by its
// content, not its name), as a stream: each channel and programme is
// decoded on its own and handed to channel or programme, never the whole
// document at once. A callback's error stops reading and is returned.
// Programmes without a channel, a title, a start or a later stop are
// skipped.
func Read(r io.Reader, options Options, channel func(Channel) error, programme func(Programme) error) error {
	limit := options.Limit
	if limit <= 0 {
		limit = MaxSize
	}
	input := bufio.NewReader(&capped{r: r, left: limit})
	var document io.Reader = input
	if magic, _ := input.Peek(2); bytes.Equal(magic, []byte{0x1f, 0x8b}) {
		unzipped, err := gzip.NewReader(input)
		if err != nil {
			return wrapped(err)
		}
		defer unzipped.Close()
		document = &capped{r: unzipped, left: limit * maxExpansion}
	}
	decoder := xml.NewDecoder(document)
	decoder.Entity = xml.HTMLEntity
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		encoding, err := htmlindex.Get(label)
		if err != nil {
			return nil, fmt.Errorf("unknown encoding %q", label)
		}
		return encoding.NewDecoder().Reader(input), nil
	}
	root := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if !root {
				return ErrMalformed
			}
			return nil
		}
		if err != nil {
			return wrapped(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch {
		case !root:
			if start.Name.Local != "tv" {
				return ErrMalformed
			}
			root = true
		case start.Name.Local == "channel":
			var element channelElement
			if err := decoder.DecodeElement(&element, &start); err != nil {
				return wrapped(err)
			}
			if c, ok := element.channel(); ok {
				if err := channel(c); err != nil {
					return err
				}
			}
		case start.Name.Local == "programme":
			var element programmeElement
			if err := decoder.DecodeElement(&element, &start); err != nil {
				return wrapped(err)
			}
			if p, ok := element.programme(options.Language); ok {
				if err := programme(p); err != nil {
					return err
				}
			}
		default:
			if err := decoder.Skip(); err != nil {
				return wrapped(err)
			}
		}
	}
}

// wrapped reports a reading error: the size limit, or a malformed guide.
func wrapped(err error) error {
	if errors.Is(err, ErrTooLarge) {
		return ErrTooLarge
	}
	var syntax *xml.SyntaxError
	if errors.As(err, &syntax) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, gzip.ErrHeader) || errors.Is(err, gzip.ErrChecksum) {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return err
}

// capped reads at most left bytes of r, then fails with ErrTooLarge.
type capped struct {
	r    io.Reader
	left int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// Only a reader with more to give is over the limit.
		var probe [1]byte
		if n, err := c.r.Read(probe[:]); n > 0 {
			return 0, ErrTooLarge
		} else if err != nil {
			return 0, err
		}
		return 0, nil
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

type text struct {
	Lang  string `xml:"lang,attr"`
	Value string `xml:",chardata"`
}

// pick returns the text in language, else the first one.
func pick(texts []text, language string) string {
	for _, t := range texts {
		if language != "" && strings.EqualFold(primary(t.Lang), primary(language)) && strings.TrimSpace(t.Value) != "" {
			return strings.TrimSpace(t.Value)
		}
	}
	for _, t := range texts {
		if value := strings.TrimSpace(t.Value); value != "" {
			return value
		}
	}
	return ""
}

// primary is the language of a tag without its region: "fr" for "fr-CA".
func primary(tag string) string {
	tag, _, _ = strings.Cut(tag, "-")
	tag, _, _ = strings.Cut(tag, "_")
	return tag
}

type channelElement struct {
	ID    string `xml:"id,attr"`
	Names []text `xml:"display-name"`
}

func (e channelElement) channel() (Channel, bool) {
	c := Channel{ID: strings.TrimSpace(e.ID)}
	for _, name := range e.Names {
		if value := strings.TrimSpace(name.Value); value != "" {
			c.Names = append(c.Names, value)
		}
	}
	return c, c.ID != ""
}

type programmeElement struct {
	Channel     string `xml:"channel,attr"`
	Start       string `xml:"start,attr"`
	Stop        string `xml:"stop,attr"`
	Titles      []text `xml:"title"`
	SubTitles   []text `xml:"sub-title"`
	Description []text `xml:"desc"`
	Categories  []text `xml:"category"`
	Episodes    []struct {
		System string `xml:"system,attr"`
		Value  string `xml:",chardata"`
	} `xml:"episode-num"`
	Icons []struct {
		Src string `xml:"src,attr"`
	} `xml:"icon"`
}

func (e programmeElement) programme(language string) (Programme, bool) {
	start, okStart := parseTime(e.Start)
	stop, okStop := parseTime(e.Stop)
	p := Programme{
		Channel:     strings.TrimSpace(e.Channel),
		Start:       start,
		Stop:        stop,
		Title:       pick(e.Titles, language),
		SubTitle:    pick(e.SubTitles, language),
		Description: pick(e.Description, language),
	}
	if p.Channel == "" || p.Title == "" || !okStart || !okStop || !stop.After(start) {
		return Programme{}, false
	}
	seen := map[string]bool{}
	for _, category := range e.Categories {
		value := strings.TrimSpace(category.Value)
		if value != "" && !seen[strings.ToLower(value)] {
			seen[strings.ToLower(value)] = true
			p.Categories = append(p.Categories, value)
		}
	}
	// The xmltv_ns numbering, counted from 0, is the one meant for
	// programs; the others are read as shown on screen.
	for _, episode := range e.Episodes {
		if strings.EqualFold(strings.TrimSpace(episode.System), "xmltv_ns") {
			p.Season, p.Episode = xmltvNS(episode.Value)
			break
		}
	}
	if p.Season == 0 && p.Episode == 0 {
		for _, episode := range e.Episodes {
			if p.Season, p.Episode = onScreen(episode.Value); p.Season != 0 || p.Episode != 0 {
				break
			}
		}
	}
	for _, icon := range e.Icons {
		if src := strings.TrimSpace(icon.Src); strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
			p.Icon = src
			break
		}
	}
	return p, true
}

// parseTime reads an XMLTV date: YYYYMMDDhhmmss, its trailing parts
// optional, then an optional offset such as +0100; a date without one is
// in UTC.
func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	digits := 0
	for digits < len(value) && digits < 14 && value[digits] >= '0' && value[digits] <= '9' {
		digits++
	}
	if digits < 8 || digits%2 != 0 {
		return time.Time{}, false
	}
	stamp := value[:digits] + "000000"[:14-digits]
	rest := strings.TrimSpace(value[digits:])
	location := time.UTC
	switch {
	case rest == "" || rest == "Z" || strings.EqualFold(rest, "UTC") || strings.EqualFold(rest, "GMT"):
	case len(rest) == 5 && (rest[0] == '+' || rest[0] == '-'):
		hours, errH := strconv.Atoi(rest[1:3])
		minutes, errM := strconv.Atoi(rest[3:5])
		if errH != nil || errM != nil || hours > 14 || minutes > 59 {
			return time.Time{}, false
		}
		offset := hours*3600 + minutes*60
		if rest[0] == '-' {
			offset = -offset
		}
		location = time.FixedZone("", offset)
	default:
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102150405", stamp, location)
	return t, err == nil
}

// xmltvNS reads "season.episode.part", each counted from 0 and possibly
// "n/total" or empty.
func xmltvNS(value string) (season, episode int) {
	parts := strings.Split(strings.ReplaceAll(value, " ", ""), ".")
	number := func(i int) int {
		if i >= len(parts) {
			return 0
		}
		head, _, _ := strings.Cut(parts[i], "/")
		n, err := strconv.Atoi(head)
		if err != nil || n < 0 || n > 100000 {
			return 0
		}
		return n + 1
	}
	return number(0), number(1)
}

var onScreenPattern = regexp.MustCompile(`(?i)(?:S\s*(\d{1,5}))?\s*E[pP]?\s*(\d{1,5})`)

// onScreen reads numbering such as "S02E05" or "E5".
func onScreen(value string) (season, episode int) {
	match := onScreenPattern.FindStringSubmatch(value)
	if match == nil {
		return 0, 0
	}
	season, _ = strconv.Atoi(match[1])
	episode, _ = strconv.Atoi(match[2])
	return season, episode
}
