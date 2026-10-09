package iptv

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	// Providers name their time zone; the server may run without the
	// system's zone files.
	_ "time/tzdata"
)

// The kinds of catch-up addresses (see Catchup.Type): an M3U playlist's
// catchup attribute, or an Xtream Codes timeshift.
const (
	// CatchupDefault takes catchup-source as the whole address.
	CatchupDefault = "default"
	// CatchupAppend appends catchup-source to the stream's address.
	CatchupAppend = "append"
	// CatchupShift adds utc and lutc to the stream's address.
	CatchupShift = "shift"
	// CatchupFlussonic asks a Flussonic server's archive of the stream.
	CatchupFlussonic = "flussonic"
	// CatchupXtream asks an Xtream Codes server's timeshift.
	CatchupXtream = "xc"
)

const (
	// defaultCatchupDays is how many days back a channel whose list names
	// a catch-up kind, but no number of days, reaches.
	defaultCatchupDays = 1
	// maxCatchupDays bounds the days a list may say.
	maxCatchupDays = 365
)

// Catchup is how a provider's archive of a channel's past programmes is
// reached: the kind of address (one of the Catchup kinds), how many days
// back it reaches, and the address template an M3U playlist gives. The
// zero value is no archive. Its addresses hold the account's credentials,
// as stream addresses do: they are never logged nor shown.
type Catchup struct {
	Type   string
	Days   int
	Source string
}

// Archived reports whether the channel has an archive.
func (c Catchup) Archived() bool { return c.Type != "" && c.Days > 0 }

// catchupType reads the kind an M3U playlist names in its catchup (or
// catchup-type) attribute; an unknown kind is none.
func catchupType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "default":
		return CatchupDefault
	case "append":
		return CatchupAppend
	case "shift":
		return CatchupShift
	case "flussonic", "flussonic-hls", "flussonic-ts", "fs":
		return CatchupFlussonic
	case "xc":
		return CatchupXtream
	}
	return ""
}

// catchupOf reads the catch-up attributes of an M3U line over what the
// playlist's header gives every channel: catchup (or catchup-type),
// catchup-days (or tvg-rec) and catchup-source. A kind without days
// reaches defaultCatchupDays back; default and append need a source.
func catchupOf(attributes map[string]string, inherited Catchup) Catchup {
	c := inherited
	if value, ok := attributes["catchup"]; ok {
		c.Type = catchupType(value)
	} else if value, ok := attributes["catchup-type"]; ok {
		c.Type = catchupType(value)
	}
	for _, name := range []string{"catchup-days", "tvg-rec"} {
		if value, ok := attributes[name]; ok && strings.TrimSpace(value) != "" {
			days, _ := strconv.Atoi(strings.TrimSpace(value))
			c.Days = days
			break
		}
	}
	if value, ok := attributes["catchup-source"]; ok {
		c.Source = strings.TrimSpace(value)
	}
	return c
}

// normalized returns the catch-up of an entry as it is stored: none for an
// unknown kind, or a kind that needs a source and has none; days within
// bounds, defaultCatchupDays when none is said.
func (c Catchup) normalized() Catchup {
	if c.Type == "" || (c.Type == CatchupDefault || c.Type == CatchupAppend) && c.Source == "" || len(c.Source) > maxAddress {
		return Catchup{}
	}
	switch {
	case c.Days <= 0:
		c.Days = defaultCatchupDays
	case c.Days > maxCatchupDays:
		c.Days = maxCatchupDays
	}
	if c.Type != CatchupDefault && c.Type != CatchupAppend {
		c.Source = ""
	}
	return c
}

// catchupAddress is the address of a channel's programme from start to
// end in its provider's archive, asked at now, the stream's address being
// stream. Dates of the template are written in zone, the provider's. ok
// is false when no address can be made: a stream address of no known
// form, or a template that does not make a web address.
func catchupAddress(c Catchup, stream string, start, end, now time.Time, zone *time.Location) (string, bool) {
	var address string
	switch c.Type {
	case CatchupDefault:
		address = expandCatchup(c.Source, start, end, now, zone)
	case CatchupAppend:
		source := c.Source
		if strings.HasPrefix(source, "?") && strings.Contains(stream, "?") {
			source = "&" + source[1:]
		}
		address = stream + expandCatchup(source, start, end, now, zone)
	case CatchupShift:
		separator := "?"
		if strings.Contains(stream, "?") {
			separator = "&"
		}
		address = stream + separator + expandCatchup("utc={utc}&lutc={lutc}", start, end, now, zone)
	case CatchupFlussonic:
		parsed, err := url.Parse(stream)
		if err != nil || parsed.Host == "" {
			return "", false
		}
		// The archive of the stream as one MPEG-TS file: archive-FROM-DURATION.ts
		// beside its playlist or its mpegts address, or under its name.
		path := strings.TrimSuffix(parsed.Path, "/")
		last := path[strings.LastIndexByte(path, '/')+1:]
		if last == "mpegts" || strings.HasSuffix(last, ".m3u8") || strings.HasSuffix(last, ".ts") {
			path = path[:len(path)-len(last)-1]
		}
		if path == "" {
			return "", false
		}
		parsed.Path, parsed.RawPath = path+"/"+expandCatchup("archive-{utc}-{duration}.ts", start, end, now, zone), ""
		address = parsed.String()
	case CatchupXtream:
		match := xtreamStream.FindStringSubmatch(stream)
		if match == nil {
			return "", false
		}
		address = timeshiftAddress(match[1], match[2], match[3], match[4], start, end, zone)
	default:
		return "", false
	}
	if !webAddress(address) {
		return "", false
	}
	return address, true
}

// xtreamStream reads an Xtream Codes stream address: its server, then
// optionally live, the username, the password and the stream's id, with
// its extension.
var xtreamStream = regexp.MustCompile(`^(https?://.+?)/(?:live/)?([^/?#]+)/([^/?#]+)/([^/?#.]+)(?:\.(?:ts|m3u8?))?$`)

// timeshiftAddress is an Xtream Codes server's address of a stream's
// programme from start to end: its duration in minutes, rounded up, and
// its start in the server's zone, as YYYY-MM-DD:HH-MM.
func timeshiftAddress(server, username, password, stream string, start, end time.Time, zone *time.Location) string {
	minutes := int((end.Sub(start) + time.Minute - 1) / time.Minute)
	return server + "/timeshift/" + username + "/" + password + "/" + strconv.Itoa(max(minutes, 1)) + "/" +
		start.In(zone).Format("2006-01-02:15-04") + "/" + stream + ".ts"
}

// providerZone is the time zone a provider names, UTC when it names none
// or one unknown.
func providerZone(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return zone
}

// expandCatchup fills the placeholders of a catch-up template for the
// programme from start to end, asked at now:
//
//   - {utc}, {start} and ${start}: the start, in seconds since 1970;
//   - {utcend}, {end} and ${end}: the end;
//   - {lutc}, {now}, ${now} and ${timestamp}: now;
//   - {duration} and ${duration}: the programme's length in seconds, and
//     {duration:N} in units of N seconds, rounded up;
//   - {offset} and ${offset}: how long ago it started, in seconds, and
//     {offset:N} in units of N seconds;
//   - {Y}, {m}, {d}, {H}, {M} and {S}: the start's year, month, day, hour,
//     minute and second in zone;
//   - {utc:F}, {start:F} and ${start:F}, {utcend:F}, {end:F} and ${end:F},
//     {lutc:F}, ${now:F} and ${timestamp:F}: those times in zone, F
//     writing them with Y, m, d, H, M and S.
//
// Other text, and placeholders of no known name, stay as they are.
func expandCatchup(template string, start, end, now time.Time, zone *time.Location) string {
	var b strings.Builder
	for i := 0; i < len(template); {
		open := i
		if template[i] == '$' && i+1 < len(template) && template[i+1] == '{' {
			open = i + 1
		} else if template[i] != '{' {
			b.WriteByte(template[i])
			i++
			continue
		}
		closing := strings.IndexByte(template[open:], '}')
		if closing < 0 {
			b.WriteString(template[i:])
			break
		}
		name, argument, _ := strings.Cut(template[open+1:open+closing], ":")
		if value, ok := placeholder(name, argument, start, end, now, zone); ok {
			b.WriteString(value)
		} else {
			b.WriteString(template[i : open+closing+1])
		}
		i = open + closing + 1
	}
	return b.String()
}

// placeholder is the value of one placeholder of a catch-up template.
func placeholder(name, argument string, start, end, now time.Time, zone *time.Location) (string, bool) {
	unix := func(t time.Time) string { return strconv.FormatInt(t.Unix(), 10) }
	var at *time.Time
	switch name {
	case "utc", "start":
		at = &start
	case "utcend", "end":
		at = &end
	case "lutc", "now", "timestamp":
		at = &now
	case "duration", "offset":
		seconds := int64(end.Sub(start) / time.Second)
		if name == "offset" {
			seconds = int64(now.Sub(start) / time.Second)
		}
		if argument == "" {
			return strconv.FormatInt(seconds, 10), true
		}
		unit, err := strconv.ParseInt(argument, 10, 64)
		if err != nil || unit <= 0 {
			return "", false
		}
		return strconv.FormatInt((seconds+unit-1)/unit, 10), true
	case "Y", "m", "d", "H", "M", "S":
		if argument != "" {
			return "", false
		}
		return dateFields(name, start.In(zone)), true
	default:
		return "", false
	}
	if argument == "" {
		return unix(*at), true
	}
	var b strings.Builder
	local := at.In(zone)
	for _, r := range argument {
		if field := string(r); strings.ContainsRune("YmdHMS", r) {
			b.WriteString(dateFields(field, local))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String(), true
}

// dateFields writes one field of a date: Y the year, m the month, d the
// day, H the hour, M the minute, S the second, all but the year on two
// digits.
func dateFields(field string, t time.Time) string {
	two := func(n int) string {
		if n < 10 {
			return "0" + strconv.Itoa(n)
		}
		return strconv.Itoa(n)
	}
	switch field {
	case "Y":
		return strconv.Itoa(t.Year())
	case "m":
		return two(int(t.Month()))
	case "d":
		return two(t.Day())
	case "H":
		return two(t.Hour())
	case "M":
		return two(t.Minute())
	default:
		return two(t.Second())
	}
}
