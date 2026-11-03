package silotest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The rules of text subtitles carried as renditions of a plan's HLS
// stream (hls_subtitle_renditions_v1 and its extensions), and of the
// playlists the apps' players load.

// containerTrack is a container track number as both apps take one:
// decimal, no leading zero, a positive 32-bit number for Android
// (PlaybackProtocolV3.kt, isCanonicalContainerTrackId), at most 20 digits
// for Apple's silo_track (PlaybackMediaAuthorization.isContainerTrackNumber).
var containerTrack = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

// checkRenditions checks a plan's renditions as the apps play them.
//
//   - subtitle.hls_rendition: Apple ApplePlaybackV3PlanAdapter.validate
//     wants a converted or repackaged HLS plan, a mode the app renders,
//     neither artifact nor embedded, format vtt, a group, and a selected
//     embedded row of the decision's track whose own hls_rendition has the
//     same name and, when the decision gives one, index; Android
//     validateForMedia3 a group and a name too, and hasValidSubtitleInventory
//     a selected track.
//   - An inventory row's hls_rendition: decoded with group_id, name and
//     index required (Apple PlaybackV3InventoryHLSRendition, Android
//     PlaybackSubtitleInventoryHlsRenditionV3); Apple takes it on embedded
//     rows of an HLS plan only (hlsSubtitleRenditionSlots); Android
//     switchableHlsRendition ignores a row whose group and name another
//     row has, or whose index is under 1. The contract numbers them from 1
//     in inventory order.
//   - container_track_id: embedded rows only, written as Android's
//     isCanonicalContainerTrackId takes it, used once
//     (nativeSwitchTrackId).
//   - native_selectable: sent only when true (Android nativeSwitchTrackId
//     reads a row with stream_index and without it as false), on an
//     embedded row with a container_track_id of a plan playing the file as
//     it is.
func checkRenditions(c *checker, p, subtitle, selected object) {
	delivery, _ := p["delivery"].(string)
	stream, _ := p["stream"].(object)
	hls := (delivery == "server_transcode_hls" || delivery == "server_remux_hls") && stream["protocol"] == "hls"
	inventory, _ := subtitle["inventory"].([]any)
	var selectedRow object
	if track, ok := selected["subtitle"].(object); ok {
		for _, entry := range inventory {
			if row, _ := entry.(object); row["track_id"] == track["id"] {
				selectedRow = row
			}
		}
	}
	names := map[string]bool{}
	containers := map[string]bool{}
	index := 0
	for n, entry := range inventory {
		row, _ := entry.(object)
		where := fmt.Sprintf("subtitle.inventory[%d]", n)
		if id, ok := row["container_track_id"]; ok {
			text, _ := id.(string)
			if row["source"] != "embedded" || !containerTrack.MatchString(text) || containers[text] {
				c.fail("%s container_track_id %v of a %v track", where, id, row["source"])
			}
			containers[text] = true
		}
		if native, ok := row["native_selectable"]; ok {
			if native != true || row["source"] != "embedded" || row["container_track_id"] == nil || delivery != "original_http" {
				c.fail("%s native_selectable %v on a %s plan", where, native, delivery)
			}
		}
		rendition, ok := row["hls_rendition"].(object)
		if !ok {
			if row["hls_rendition"] != nil {
				c.fail("%s hls_rendition %v", where, row["hls_rendition"])
			}
			continue
		}
		c.has(rendition, where+".hls_rendition", map[string]string{"group_id": "s", "name": "s", "index": "i"})
		group, _ := rendition["group_id"].(string)
		name, _ := rendition["name"].(string)
		index++
		switch {
		case !hls:
			c.fail("%s hls_rendition on a %s plan", where, delivery)
		case row["source"] != "embedded":
			c.fail("%s hls_rendition of a %v track", where, row["source"])
		case strings.TrimSpace(group) == "" || strings.TrimSpace(name) == "":
			c.fail("%s hls_rendition without a group or a name", where)
		case names[group+"\x00"+name]:
			c.fail("%s hls_rendition name %q twice in group %q", where, name, group)
		case rendition["index"] != float64(index):
			c.fail("%s hls_rendition index %v, the rendition %d of the group", where, rendition["index"], index)
		}
		names[group+"\x00"+name] = true
	}
	chosen, ok := subtitle["hls_rendition"].(object)
	if !ok {
		if subtitle["hls_rendition"] != nil {
			c.fail("subtitle.hls_rendition %v", subtitle["hls_rendition"])
		}
		return
	}
	c.has(chosen, "subtitle.hls_rendition", map[string]string{"group_id": "s", "name": "s", "format": "s", "mime_type": "s"})
	mode, _ := subtitle["mode"].(string)
	track, _ := selected["subtitle"].(object)
	switch {
	case !hls:
		c.fail("subtitle.hls_rendition on a %s plan", delivery)
	case mode != "render" && mode != "convert":
		c.fail("subtitle.hls_rendition in mode %s", mode)
	case subtitle["artifact"] != nil || subtitle["embedded"] != nil:
		c.fail("subtitle.hls_rendition with an artifact or embedded")
	case chosen["format"] != "vtt":
		c.fail("subtitle.hls_rendition format %v", chosen["format"])
	case strings.TrimSpace(fmt.Sprint(chosen["group_id"])) == "" || strings.TrimSpace(fmt.Sprint(chosen["name"])) == "":
		c.fail("subtitle.hls_rendition without a group or a name")
	case selectedRow == nil || subtitle["track_id"] != track["id"]:
		c.fail("subtitle.hls_rendition of no selected track")
	case selectedRow["source"] != "embedded":
		c.fail("subtitle.hls_rendition of a %v track", selectedRow["source"])
	default:
		if own, listed := selectedRow["hls_rendition"].(object); listed &&
			(own["name"] != chosen["name"] || chosen["index"] != nil && own["index"] != chosen["index"]) {
			c.fail("subtitle.hls_rendition %v, its row's %v", chosen, own)
		}
	}
}

// MediaAddressAllowed reports whether the Apple app's player fetches
// address, relative to a plan's stream URL streamURL, with the app's
// credentials (PlaybackMediaAuthorization.Scope.allows): the stream URL
// itself, without a query but seek; for an HLS stream, master.m3u8 with
// silo_rendition=video or silo_rendition=subtitles alone, or
// silo_rendition=subtitles with a silo_track; or segment/{name} beside it,
// name of letters, digits, '-', '.', '_' and '~', without a query but seek.
// Anything else fails to load.
func MediaAddressAllowed(streamURL, address string) bool {
	base, err := url.Parse(streamURL)
	if err != nil {
		return false
	}
	target, err := base.Parse(address)
	if err != nil || target.Host != base.Host || target.Fragment != "" {
		return false
	}
	if lower := strings.ToLower(target.EscapedPath()); strings.Contains(lower, "%2e") || strings.Contains(lower, "%2f") ||
		strings.Contains(lower, "%5c") || strings.Contains(lower, "%25") {
		return false
	}
	query := target.Query()
	if target.Path == base.Path && path.Base(base.Path) == "master.m3u8" && query.Has("silo_rendition") {
		switch rendition := query.Get("silo_rendition"); len(query) {
		case 1:
			return len(query["silo_rendition"]) == 1 && (rendition == "video" || rendition == "subtitles")
		case 2:
			track := query["silo_track"]
			return rendition == "subtitles" && len(track) == 1 && len(query["silo_rendition"]) == 1 &&
				regexp.MustCompile(`^[1-9][0-9]{0,19}$`).MatchString(track[0])
		}
		return false
	}
	for name, values := range query {
		if name != "seek" || len(values) != 1 {
			return false
		}
		if seconds, err := strconv.ParseFloat(values[0], 64); err != nil || seconds < 0 {
			return false
		}
	}
	if target.Path == base.Path {
		return true
	}
	folder := path.Dir(base.Path)
	segment, ok := strings.CutPrefix(target.Path, folder+"/segment/")
	return path.Base(base.Path) == "master.m3u8" && ok && safeName.MatchString(segment) && segment != "." && segment != ".."
}

// safeName is a path component Apple's scope takes (isSafeComponent).
var safeName = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// playlistURIs are the URIs a playlist names: its URI lines and the URI
// attributes of its tags.
func playlistURIs(playlist string) []string {
	var uris []string
	attribute := regexp.MustCompile(`URI="([^"]*)"`)
	for line := range strings.SplitSeq(playlist, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "#"):
			for _, match := range attribute.FindAllStringSubmatch(line, -1) {
				uris = append(uris, match[1])
			}
		default:
			uris = append(uris, line)
		}
	}
	return uris
}

// CheckMediaPlaylist checks a media playlist the player loads, of the plan
// whose stream URL is streamURL: a VOD playlist (the Jellyfin API's
// conversions list every segment), every URI one the Apple app fetches
// (see MediaAddressAllowed).
func CheckMediaPlaylist(data []byte, streamURL string) error {
	c := &checker{}
	playlist := string(data)
	if !strings.HasPrefix(playlist, "#EXTM3U") || !strings.Contains(playlist, "#EXTINF:") || !strings.Contains(playlist, "#EXT-X-ENDLIST") {
		c.fail("not a complete media playlist")
	}
	for _, uri := range playlistURIs(playlist) {
		if !MediaAddressAllowed(streamURL, uri) {
			c.fail("the Apple app does not fetch %q", uri)
		}
	}
	return c.err()
}

// CheckMultivariant checks the master playlist of a decision whose plan
// carries subtitle renditions (decision, the start's or replan's answer).
// Every URI is one the Apple app fetches (MediaAddressAllowed). The one
// variant names the group of the renditions, which hold, in order, the
// rows' hls_rendition names (Android matches the group and the name,
// Apple the name among its player's options, Roku the 1-based index); a
// rendition's silo_track is its row's container_track_id (the contract's
// getPlaybackManifest). Only the selected rendition is DEFAULT, every one
// AUTOSELECT (playback-protocol-v3.md, section 8), as players following
// the system's caption preference expect. A single rendition, for a plan
// whose rows name none, is the decision's.
func CheckMultivariant(data []byte, decision []byte) error {
	var d struct {
		Plan struct {
			Stream struct {
				URL string `json:"url"`
			} `json:"stream"`
			Subtitle struct {
				HLSRendition *struct {
					GroupID string `json:"group_id"`
					Name    string `json:"name"`
				} `json:"hls_rendition"`
				Inventory []struct {
					ContainerTrackID string `json:"container_track_id"`
					HLSRendition     *struct {
						GroupID string `json:"group_id"`
						Name    string `json:"name"`
						Index   int    `json:"index"`
					} `json:"hls_rendition"`
				} `json:"inventory"`
			} `json:"subtitle"`
		} `json:"playback_plan"`
	}
	c := &checker{}
	if err := json.Unmarshal(decision, &d); err != nil {
		return err
	}
	playlist := string(data)
	for _, uri := range playlistURIs(playlist) {
		if !MediaAddressAllowed(d.Plan.Stream.URL, uri) {
			c.fail("the Apple app does not fetch %q", uri)
		}
	}
	type media struct{ group, name, track, isDefault, autoselect string }
	var renditions []media
	var variants []string
	attribute := func(line, name string) string {
		match := regexp.MustCompile(`(?:^|[:,])` + name + `=("[^"]*"|[^,]*)`).FindStringSubmatch(line)
		if match == nil {
			return ""
		}
		return strings.Trim(match[1], `"`)
	}
	for line := range strings.SplitSeq(playlist, "\n") {
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			if attribute(line, "TYPE") != "SUBTITLES" {
				continue
			}
			uri, _ := url.Parse(attribute(line, "URI"))
			track := ""
			if uri != nil {
				track = uri.Query().Get("silo_track")
			}
			renditions = append(renditions, media{attribute(line, "GROUP-ID"), attribute(line, "NAME"), track, attribute(line, "DEFAULT"),
				attribute(line, "AUTOSELECT")})
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			variants = append(variants, attribute(line, "SUBTITLES"))
		}
	}
	type row struct{ group, name, track string }
	var rows []row
	for _, item := range d.Plan.Subtitle.Inventory {
		if item.HLSRendition != nil {
			rows = append(rows, row{item.HLSRendition.GroupID, item.HLSRendition.Name, item.ContainerTrackID})
		}
	}
	if len(rows) == 0 && d.Plan.Subtitle.HLSRendition != nil {
		rows = []row{{d.Plan.Subtitle.HLSRendition.GroupID, d.Plan.Subtitle.HLSRendition.Name, ""}}
	}
	if len(variants) != 1 {
		c.fail("%d variants", len(variants))
	}
	if len(renditions) != len(rows) {
		c.fail("%d renditions, the plan names %d", len(renditions), len(rows))
		return c.err()
	}
	defaults := 0
	for i, r := range renditions {
		want := rows[i]
		switch {
		case r.group != want.group || r.name != want.name:
			c.fail("rendition %d is %s/%s, the plan's %s/%s", i+1, r.group, r.name, want.group, want.name)
		case len(variants) == 1 && variants[0] != r.group:
			c.fail("rendition %d of group %s, the variant's %s", i+1, r.group, variants[0])
		case r.track != want.track:
			c.fail("rendition %d silo_track %q, its row's container_track_id %q", i+1, r.track, want.track)
		case r.autoselect != "YES":
			c.fail("rendition %d not AUTOSELECT", i+1)
		}
		if r.isDefault == "YES" {
			defaults++
			if d.Plan.Subtitle.HLSRendition == nil || d.Plan.Subtitle.HLSRendition.Name != r.name {
				c.fail("rendition %d is DEFAULT, not the plan's", i+1)
			}
		}
	}
	if defaults > 1 {
		c.fail("%d renditions are DEFAULT", defaults)
	}
	if slices.ContainsFunc(renditions, func(r media) bool { return r.group == "" || r.name == "" }) {
		c.fail("a rendition without a group or a name")
	}
	return c.err()
}

// CheckSubtitleSegment checks a segment of a subtitle rendition: a WebVTT
// document with the X-TIMESTAMP-MAP that places its cues on the video's
// timeline, which HLS requires of WebVTT segments (RFC 8216, section
// 3.5) and AVPlayer and Media3 read.
func CheckSubtitleSegment(data []byte) error {
	c := &checker{}
	text := strings.TrimPrefix(string(data), "\ufeff")
	header, _, _ := strings.Cut(text, "\n\n")
	if !strings.HasPrefix(text, "WEBVTT") {
		c.fail("not WebVTT: %.40q", text)
	}
	if !strings.Contains(header, "X-TIMESTAMP-MAP=") {
		c.fail("no X-TIMESTAMP-MAP in its header")
	}
	return c.err()
}
