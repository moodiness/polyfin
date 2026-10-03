package subtitles

import (
	"bytes"
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Script is an Advanced SubStation Alpha (v4+) or SubStation Alpha (v4)
// script, kept as it is written so that players rendering ASS draw it as
// its authors styled it.
type Script struct {
	// Header holds everything up to and including the [Events] Format
	// line, verbatim: script info, styles and whatever sections precede the
	// events.
	Header []byte
	// Events are the Dialogue lines, in file order.
	Events []Event
	// Footer holds what follows the events ([Fonts], [Graphics], ...),
	// verbatim.
	Footer []byte
}

// Event is a Dialogue line of a script.
type Event struct {
	Start, End time.Duration
	// Layer is the first field verbatim: Layer (ASS) or Marked (SSA).
	Layer string
	// Rest holds the fields after End verbatim: Style, Name, MarginL,
	// MarginR, MarginV, Effect and Text.
	Rest string
}

// eventFields are the fields of Dialogue lines in the order scripts
// almost always give them, and the one Matroska blocks follow; SSA scripts
// name the first one Marked.
var eventFields = []string{"Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text"}

// eventDefaults are the values a field missing from a script's Format
// takes, in the order of eventFields.
var eventDefaults = []string{"0", "", "", "Default", "", "0", "0", "0", "", ""}

func formatLine(ssa bool) string {
	if ssa {
		return "Format: Marked, " + strings.Join(eventFields[1:], ", ") + "\n"
	}
	return "Format: " + strings.Join(eventFields, ", ") + "\n"
}

// eventFormat tells how to read the Dialogue lines of a script.
type eventFormat struct {
	// count is the number of fields of a Dialogue line, the last one, the
	// text, holding commas of its own.
	count int
	// index gives, for each of eventFields, its position in Dialogue lines
	// or -1 when they lack it; nil when they follow the order of
	// eventFields.
	index []int
	ssa   bool
}

var standardFormat = eventFormat{count: len(eventFields)}

// parseFormat reads the fields an [Events] Format line names. Dialogue
// lines are written back as Layer, Start, End and the fields after End,
// so a format placing Start and End elsewhere is read field by field
// (rewrite reports that its Format line must then become the standard
// one); a format lacking Start, End or Text, which libass could not read
// either, is taken for the standard one.
func parseFormat(line string, ssa bool) (format eventFormat, rewrite bool) {
	_, list, _ := strings.Cut(line, ":")
	names := strings.Split(list, ",")
	for i, name := range names {
		names[i] = strings.ToLower(strings.TrimSpace(name))
	}
	last := len(names) - 1
	if len(names) >= 4 && names[1] == "start" && names[2] == "end" && names[last] == "text" {
		return eventFormat{count: len(names), ssa: ssa}, false
	}
	format = eventFormat{count: len(names), index: make([]int, len(eventFields)), ssa: ssa}
	for i, field := range eventFields {
		format.index[i] = -1
		for j, name := range names {
			if name == strings.ToLower(field) || i == 0 && name == "marked" {
				format.index[i] = j
				break
			}
		}
	}
	if format.index[1] < 0 || format.index[2] < 0 || format.index[len(eventFields)-1] != last {
		format = standardFormat
		format.ssa = ssa
	}
	return format, true
}

// parseDialogue reads the fields after "Dialogue:" of an event line.
func (f eventFormat) parseDialogue(fields string) (Event, bool) {
	fields = strings.TrimLeft(fields, " \t")
	var layer, start, end, rest string
	if f.index == nil {
		parts := strings.SplitN(fields, ",", 4)
		if len(parts) < 4 || strings.Count(parts[3], ",") < f.count-4 {
			return Event{}, false
		}
		layer, start, end, rest = parts[0], parts[1], parts[2], parts[3]
	} else {
		parts := strings.SplitN(fields, ",", f.count)
		if len(parts) < f.count {
			return Event{}, false
		}
		values := make([]string, len(eventFields))
		for i, j := range f.index {
			if j >= 0 {
				values[i] = parts[j]
			} else {
				values[i] = eventDefaults[i]
			}
		}
		if f.index[0] < 0 && f.ssa {
			values[0] = "Marked=0"
		}
		layer, start, end, rest = values[0], values[1], values[2], strings.Join(values[3:], ",")
	}
	startTime, ok := assTimestamp(start)
	if !ok {
		return Event{}, false
	}
	endTime, ok := assTimestamp(end)
	if !ok || endTime < startTime {
		return Event{}, false
	}
	return Event{Start: startTime, End: endTime, Layer: layer, Rest: rest}, true
}

// assTimestamp reads an H:MM:SS.cc time. Like libass, it reads the digits
// after the dot as hundredths whatever their number; the bounds on digits
// keep hostile values from overflowing.
func assTimestamp(value string) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	hours, rest, ok1 := strings.Cut(value, ":")
	minutes, rest, ok2 := strings.Cut(rest, ":")
	seconds, hundredths, _ := strings.Cut(rest, ".")
	if !ok1 || !ok2 {
		return 0, false
	}
	var total time.Duration
	for i, part := range []string{hours, minutes, seconds, hundredths} {
		limit := 2
		if i == 0 {
			limit = 6
		} else if i == 3 {
			limit = 3
		}
		if len(part) > limit || part == "" && i < 3 {
			return 0, false
		}
		n := 0
		for _, c := range []byte(part) {
			if c < '0' || c > '9' {
				return 0, false
			}
			n = n*10 + int(c-'0')
		}
		switch i {
		case 0:
			total = time.Duration(n) * time.Hour
		case 1:
			total += time.Duration(n) * time.Minute
		case 2:
			total += time.Duration(n) * time.Second
		case 3:
			total += time.Duration(n) * 10 * time.Millisecond
		}
	}
	return total, true
}

// decode turns subtitle bytes into text with "\n" line endings, without
// byte order mark, reading text that is not valid UTF-8 as Windows-1252.
func decode(data []byte) string {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		data = fromWindows1252(data)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
}

// isScript reports text whose first line is the [Script Info] section
// every ASS and SSA script opens with.
func isScript(text string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return strings.EqualFold(line, "[Script Info]")
		}
	}
	return false
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func withNewline(s string) string {
	if s != "" && !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

// ParseScript reads an ASS or SSA script; ErrUnsupported when data is not
// one. Like Parse, it reads text that is not valid UTF-8 as Windows-1252.
// Lines of the [Events] section other than Dialogue lines (Comment,
// Picture, ...) are dropped: players show none of them. A script whose
// [Events] section has no Format line gets the standard one.
func ParseScript(data []byte) (*Script, error) {
	text := decode(data)
	if !isScript(text) {
		return nil, ErrUnsupported
	}
	script, _ := parseScript(text)
	return script, nil
}

// parseScript reads the script text holds, reporting whether it is an
// SSA (v4) script.
func parseScript(text string) (*Script, bool) {
	var (
		script   Script
		ssa      bool
		inEvents bool
		header   = -1 // where the header ends, once the Format line is read
		format   eventFormat
		blank    = -1 // where the blank lines before the current line start
	)
	// standard ends the header before the line at offset at, giving it the
	// standard Format line.
	standard := func(at int) {
		script.Header = []byte(withNewline(text[:at]) + formatLine(ssa))
		format, format.ssa = standardFormat, ssa
		header = at
	}
	for pos := 0; pos < len(text); {
		start, end, next := pos, len(text), len(text)
		if i := strings.IndexByte(text[pos:], '\n'); i >= 0 {
			end, next = pos+i, pos+i+1
		}
		pos = next
		line := strings.TrimSpace(text[start:end])
		switch {
		case line == "":
			if blank < 0 {
				blank = start
			}
			continue
		case line[0] == '[' && line[len(line)-1] == ']':
			if inEvents {
				// The blank lines between the events and the next section
				// belong with it, so that writing the script back keeps them.
				footer := start
				if blank >= 0 {
					footer = blank
				}
				if header < 0 {
					standard(footer)
				}
				script.Footer = []byte(text[footer:])
				return &script, ssa
			}
			switch strings.ToLower(line) {
			case "[events]":
				inEvents = true
			case "[v4 styles]":
				ssa = true
			}
		case !inEvents:
		case header < 0 && hasPrefixFold(line, "format:"):
			var rewrite bool
			format, rewrite = parseFormat(line, ssa)
			if rewrite {
				standard(start)
				format, _ = parseFormat(line, ssa)
			} else {
				script.Header = []byte(withNewline(text[:next]))
				header = next
			}
		case strings.HasPrefix(line, "Dialogue:"):
			if header < 0 {
				standard(start)
			}
			if event, ok := format.parseDialogue(line[len("Dialogue:"):]); ok {
				script.Events = append(script.Events, event)
			}
		}
		blank = -1
	}
	switch {
	case header >= 0:
	case inEvents:
		standard(len(text))
	default:
		script.Header = []byte(withNewline(text) + "\n[Events]\n" + formatLine(ssa))
	}
	return &script, ssa
}

// Bytes writes the script: its header, its events as Dialogue lines and
// its footer.
func (s *Script) Bytes() []byte {
	var out bytes.Buffer
	out.Grow(len(s.Header) + len(s.Footer) + len(s.Events)*96)
	out.Write(s.Header)
	for _, event := range s.Events {
		out.WriteString("Dialogue: ")
		out.WriteString(event.Layer)
		out.WriteByte(',')
		out.WriteString(assTime(event.Start))
		out.WriteByte(',')
		out.WriteString(assTime(event.End))
		out.WriteByte(',')
		out.WriteString(event.Rest)
		out.WriteByte('\n')
	}
	out.Write(s.Footer)
	return out.Bytes()
}

// From keeps, like From for cues, the events that end after start,
// shifted so that start becomes zero. The new script shares the header
// and footer of s.
func (s *Script) From(start time.Duration) *Script {
	if start <= 0 {
		return s
	}
	events := make([]Event, 0, len(s.Events))
	for _, event := range s.Events {
		if event.End <= start {
			continue
		}
		event.Start, event.End = max(event.Start-start, 0), event.End-start
		events = append(events, event)
	}
	return &Script{Header: s.Header, Events: events, Footer: s.Footer}
}

// Cues returns the text the script shows, for apps that cannot render
// ASS, as Jellyfin writes an ASS track as WebVTT: override blocks
// removed, \N line breaks as lines, \h as a space, the rest of the text
// as it is. \n breaks lines only where the script wraps with style 2, as
// ASS defines it, and is a space elsewhere. Drawings are dropped, and
// events showing the same text at the same times (a sign drawn in several
// layers) give one cue.
func (s *Script) Cues() []Cue {
	fields, wrap := scriptLayout(s.Header)
	cues := make([]Cue, 0, len(s.Events))
	for _, event := range s.Events {
		if event.End <= event.Start {
			continue
		}
		text := event.Rest
		for range fields - 4 {
			if _, text, _ = strings.Cut(text, ","); text == "" {
				break
			}
		}
		if lines := plainText(text, wrap); len(lines) > 0 {
			cues = append(cues, Cue{Start: event.Start, End: event.End, Lines: lines})
		}
	}
	slices.SortStableFunc(cues, func(a, b Cue) int { return cmp.Compare(a.Start, b.Start) })
	unique := cues[:0]
	for _, cue := range cues {
		duplicate := false
		for i := len(unique) - 1; i >= 0 && unique[i].Start == cue.Start; i-- {
			if unique[i].End == cue.End && slices.Equal(unique[i].Lines, cue.Lines) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			unique = append(unique, cue)
		}
	}
	return unique
}

// scriptLayout reads from a script header the number of fields of its
// Dialogue lines and whether it wraps text with style 2, where \n breaks
// lines.
func scriptLayout(header []byte) (fields int, wrap bool) {
	fields = len(eventFields)
	text := strings.TrimRight(string(header), "\n")
	if line := text[strings.LastIndexByte(text, '\n')+1:]; hasPrefixFold(line, "format:") {
		if n := strings.Count(line, ",") + 1; n >= 4 {
			fields = n
		}
	}
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); hasPrefixFold(line, "wrapstyle:") {
			wrap = strings.TrimSpace(line[len("wrapstyle:"):]) == "2"
		}
	}
	return fields, wrap
}

// plainText turns the text of an event into the lines it shows.
func plainText(text string, wrap bool) []string {
	var out strings.Builder
	drawing := false
	show := func(s string) {
		if !drawing {
			out.WriteString(s)
		}
	}
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '{':
			end := strings.IndexByte(text[i:], '}')
			if end < 0 {
				// libass shows an override block left open as text.
				show(text[i:])
				i = len(text)
				continue
			}
			// What precedes the first backslash is a comment.
			tags := strings.Split(text[i+1:i+end], `\`)
			for _, tag := range tags[1:] {
				switch name, value := overrideTag(tag); name {
				case "p":
					drawing = value > 0
				case "q":
					wrap = value == 2
				}
			}
			i += end + 1
		case c == '\\' && i+1 < len(text) && strings.IndexByte("Nnh", text[i+1]) >= 0:
			if text[i+1] == 'N' || text[i+1] == 'n' && wrap {
				show("\n")
			} else {
				show(" ")
			}
			i += 2
		default:
			end := i + 1
			for end < len(text) && text[end] != '{' && text[end] != '\\' {
				end++
			}
			show(text[i:end])
			i = end
		}
	}
	var lines []string
	for line := range strings.SplitSeq(out.String(), "\n") {
		if line = strings.Trim(line, " \t"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// overrideTag splits an override tag into its name, the letters it starts
// with, and the number following them, or -1 when none does.
func overrideTag(tag string) (string, int) {
	tag = strings.TrimSpace(tag)
	n := 0
	for n < len(tag) && (tag[n] >= 'a' && tag[n] <= 'z' || tag[n] >= 'A' && tag[n] <= 'Z') {
		n++
	}
	digits := n
	for digits < len(tag) && digits-n < 6 && tag[digits] >= '0' && tag[digits] <= '9' {
		digits++
	}
	value, err := strconv.Atoi(tag[n:digits])
	if err != nil {
		value = -1
	}
	return tag[:n], value
}

// MatroskaEvent is a block of a Matroska ASS/SSA track: its Data is
// "ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text".
type MatroskaEvent struct {
	Start, Duration time.Duration
	Data            []byte
}

// MatroskaScript rebuilds the script a Matroska ASS/SSA track holds: its
// CodecPrivate is the header, its blocks the events, in ReadOrder (ties by
// Start). The header ends with the standard Format line, the order of the
// fields of blocks, whatever the CodecPrivate gives. Blocks not shaped as
// the format says are dropped.
func MatroskaScript(codecPrivate []byte, events []MatroskaEvent) (*Script, error) {
	text := decode(bytes.TrimRight(codecPrivate, "\x00"))
	if !isScript(text) {
		return nil, ErrUnsupported
	}
	private, ssa := parseScript(text)
	header := strings.TrimSuffix(string(private.Header), "\n")
	at := strings.LastIndexByte(header, '\n') + 1
	if format, rewrite := parseFormat(header[at:], ssa); rewrite || format.count != len(eventFields) ||
		!strings.EqualFold(strings.Join(strings.Fields(header[at:]), " "), strings.TrimSuffix(formatLine(ssa), "\n")) {
		header = header[:at] + formatLine(ssa)
	} else {
		header += "\n"
	}
	type ordered struct {
		readOrder int64
		event     Event
	}
	list := make([]ordered, 0, len(events))
	for _, block := range events {
		data := strings.TrimRight(decode(bytes.TrimRight(block.Data, "\x00")), "\n")
		// A Dialogue line holds no line ending: a break the block carries
		// raw becomes the one ASS writes.
		data = strings.ReplaceAll(data, "\n", `\N`)
		readOrder, rest, ok1 := strings.Cut(data, ",")
		layer, rest, ok2 := strings.Cut(rest, ",")
		order, err := strconv.ParseInt(strings.TrimSpace(readOrder), 10, 64)
		if !ok1 || !ok2 || err != nil || strings.Count(rest, ",") < len(eventFields)-4 {
			continue
		}
		start := max(block.Start, 0)
		list = append(list, ordered{order, Event{Start: start, End: start + max(block.Duration, 0), Layer: layer, Rest: rest}})
	}
	slices.SortStableFunc(list, func(a, b ordered) int {
		return cmp.Or(cmp.Compare(a.readOrder, b.readOrder), cmp.Compare(a.event.Start, b.event.Start))
	})
	script := &Script{Header: []byte(header), Events: make([]Event, len(list)), Footer: private.Footer}
	for i, item := range list {
		script.Events[i] = item.event
	}
	return script, nil
}
