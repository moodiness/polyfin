// Package silotest checks answers of Silo's API as Silo's apps accept
// them: the rules their code applies to a playback's capabilities and
// decisions, at the commits AppleCommit and AndroidCommit of the apps'
// repositories (silo-apple and silo-android), each rule noted with the
// code it comes from. An answer that breaks one fails in the app, with a
// message that asks to update the server or that the title cannot play.
package silotest

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// The commits of the apps whose rules these are.
const (
	AppleCommit   = "97fe5aacf661b92cf25bccdfb1d51c76839d54d3"
	AndroidCommit = "88db96c280a4ce06877e2402c7c662ac3919c58e"
)

// RequiredFeatures are the features the capabilities document names
// before the apps start a playback: the Apple app's requireAvailable
// (Networking/APIv2/APIv2PlaybackModels.swift) asks all four, the Android
// app's sequenced playback (playback/SequencedPlayback.kt) the last.
var RequiredFeatures = []string{"playback_plan_v3", "neutral_playback_v3_contract_v1", "header_authenticated_media_v1",
	"sequenced_progress_v1"}

type object = map[string]any

// checker gathers the rules an answer breaks.
type checker struct{ broken []string }

func (c *checker) fail(format string, args ...any) {
	c.broken = append(c.broken, fmt.Sprintf(format, args...))
}

func (c *checker) err() error {
	if len(c.broken) == 0 {
		return nil
	}
	return errors.New(strings.Join(c.broken, "; "))
}

// has requires keys of an object, of the kinds the apps decode them as:
// "s" a string, "b" a boolean, "n" a finite number, "i" an integer, "a" an
// array, "o" an object. The apps' non-optional properties are required
// keys: one missing fails the whole answer.
func (c *checker) has(o object, where string, kinds map[string]string) {
	for key, kind := range kinds {
		value, ok := o[key]
		if !ok || value == nil {
			c.fail("%s.%s is missing", where, key)
			continue
		}
		var fits bool
		switch kind {
		case "s":
			_, fits = value.(string)
		case "b":
			_, fits = value.(bool)
		case "n":
			n, number := value.(float64)
			fits = number && !math.IsInf(n, 0) && !math.IsNaN(n)
		case "i":
			n, number := value.(float64)
			fits = number && n == math.Trunc(n)
		case "a":
			_, fits = value.([]any)
		case "o":
			_, fits = value.(object)
		}
		if !fits {
			c.fail("%s.%s is not of kind %s: %v", where, key, kind, value)
		}
	}
}

func texts(values []any) []string {
	var result []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// CheckCapabilities checks a capabilities document as the apps read it
// before they start a playback.
func CheckCapabilities(data []byte) error {
	var doc object
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	c := &checker{}
	c.has(doc, "capabilities", map[string]string{"state": "s", "allowed": "b", "revision": "s", "protocol_versions": "a", "features": "a"})
	if doc["state"] != "available" || doc["allowed"] != true {
		c.fail("playback is not available: %v, allowed %v", doc["state"], doc["allowed"])
	}
	if id, _ := doc["installation_id"].(string); strings.TrimSpace(id) == "" {
		c.fail("no installation_id")
	}
	versions, _ := doc["protocol_versions"].([]any)
	if !slices.Contains(versions, any(float64(3))) {
		c.fail("protocol_versions %v lack 3", versions)
	}
	features, _ := doc["features"].([]any)
	for _, feature := range RequiredFeatures {
		if !slices.Contains(texts(features), feature) {
			c.fail("features lack %s", feature)
		}
	}
	return c.err()
}

// sessionID is a session identifier as the Apple app takes one: a UUID
// in lower case (PlaybackSessionBridge.playbackSessionSuffix).
var sessionID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// The deliveries the Apple app plays (PlaybackProtocolV3Models.swift,
// validatedForApple), by the protocol its plan adapter expects of each
// (ApplePlaybackV3PlanAdapter.validate).
var deliveries = map[string]string{"original_http": "http_progressive", "server_remux_progressive": "http_progressive",
	"server_remux_hls": "hls", "server_transcode_hls": "hls"}

// mediaPath reports whether a media address is one the Apple app fetches
// with its bearer token (StreamRequest.swift): a path of the API's
// streams, without a query but the seek, nor a step out of it.
func mediaPath(address string) bool {
	path, query, _ := strings.Cut(address, "?")
	lower := strings.ToLower(path)
	if !strings.HasPrefix(path, "/api/v2/stream/") && !strings.HasPrefix(path, "/api/v2/playback/transcode/") ||
		strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(path, "\\") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	for _, pair := range strings.Split(query, "&") {
		if name, _, _ := strings.Cut(pair, "="); pair != "" && name != "seek" {
			return false
		}
	}
	return true
}

// fileID reports whether a file identifier is one the apps take: a
// positive integer, written plainly (APIv2PlaybackModels.swift,
// APIv2PlaybackID; PlaybackDecisionV2.kt).
func fileID(value any) bool {
	text, _ := value.(string)
	n, err := strconv.Atoi(text)
	return err == nil && n > 0 && strconv.Itoa(n) == text
}

// CheckDecision checks a decision as the apps play it, the answer of a
// start or of a replan. files are the versions' file identifiers the
// title's watch detail lists, one of which the plan plays.
func CheckDecision(data []byte, files []string) error {
	var d object
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}
	c := &checker{}
	// Apple: APIv2PlaybackDecision decodes server_features as required,
	// and validatedForApple wants protocol 3 and the plan feature.
	c.has(d, "decision", map[string]string{"server_features": "a"})
	features, _ := d["server_features"].([]any)
	if d["protocol_version"] != float64(3) {
		c.fail("protocol_version %v", d["protocol_version"])
	}
	for _, feature := range []string{"playback_plan_v3", "neutral_playback_v3_contract_v1", "header_authenticated_media_v1"} {
		if !slices.Contains(texts(features), feature) {
			c.fail("server_features lack %s", feature)
		}
	}
	switch d["outcome"] {
	case "adaptation_unavailable":
		terminal, _ := d["terminal"].(object)
		if terminal == nil {
			c.fail("a terminal decision without terminal")
			return c.err()
		}
		c.has(terminal, "terminal", map[string]string{"reason": "s", "message": "s", "retryable": "b"})
		return c.err()
	case "playable":
	default:
		c.fail("outcome %v", d["outcome"])
		return c.err()
	}
	session, _ := d["session_id"].(string)
	if !sessionID.MatchString(session) {
		c.fail("session_id %q is not a UUID in lower case", session)
	}
	p, _ := d["playback_plan"].(object)
	if p == nil {
		c.fail("a playable decision without playback_plan")
		return c.err()
	}
	checkPlan(c, p, session, files)
	return c.err()
}

func checkPlan(c *checker, p object, session string, files []string) {
	// The plan's required keys (APIv2PlaybackPlan).
	c.has(p, "plan", map[string]string{"protocol_version": "i", "plan_id": "s", "delivery": "s", "plan_attempt_key": "s", "stream": "o",
		"timeline": "o", "selected_tracks": "o", "effective_recipe": "o", "claims": "o", "subtitle": "o", "transformations": "a",
		"applied_quirks": "a", "runtime_corrections": "a", "degradation_warnings": "a", "decision_reason": "s",
		"requested_media_file_id": "s", "effective_media_file_id": "s", "source": "o", "subtitle_fidelity_policy": "s",
		"available_qualities": "a"})
	if p["protocol_version"] != float64(3) {
		c.fail("plan protocol_version %v", p["protocol_version"])
	}
	if id, _ := p["plan_id"].(string); id == "" {
		c.fail("no plan_id")
	}
	// Android: validateForMedia3 wants a plan attempt key.
	if key, _ := p["plan_attempt_key"].(string); strings.TrimSpace(key) == "" {
		c.fail("no plan_attempt_key")
	}
	if id, ok := p["session_id"].(string); ok && id != session {
		c.fail("plan session_id %q is not the decision's %q", id, session)
	}
	for _, key := range []string{"requested_media_file_id", "effective_media_file_id"} {
		if !fileID(p[key]) {
			c.fail("%s %v is not a positive integer", key, p[key])
		}
	}
	// Apple: the plan plays one of the title's versions (PlaybackSessionBridge).
	if files != nil && !slices.Contains(files, fmt.Sprint(p["effective_media_file_id"])) {
		c.fail("effective_media_file_id %v is none of the versions %v", p["effective_media_file_id"], files)
	}
	delivery, _ := p["delivery"].(string)
	protocol, known := deliveries[delivery]
	if !known {
		c.fail("delivery %q", delivery)
	}
	// Apple: runtime corrections are refused, and transformations the app
	// would apply itself are its own (ApplePlaybackV3PlanAdapter.validate).
	if corrections, _ := p["runtime_corrections"].([]any); len(corrections) > 0 {
		c.fail("runtime corrections %v", corrections)
	}
	transformations, _ := p["transformations"].([]any)
	for _, t := range transformations {
		if step, _ := t.(object); step["executor"] == "client" {
			c.fail("a transformation the app would apply: %v", step)
		}
	}
	if stream, ok := p["stream"].(object); ok {
		c.has(stream, "stream", map[string]string{"url": "s", "protocol": "s", "headers": "o", "header_refresh": "s"})
		if known && stream["protocol"] != protocol {
			c.fail("delivery %s with protocol %v", delivery, stream["protocol"])
		}
		if refresh := stream["header_refresh"]; refresh != "none" && refresh != "session" {
			c.fail("header_refresh %v", refresh)
		}
		if url, _ := stream["url"].(string); !mediaPath(url) {
			c.fail("stream url %q is not a media address the apps fetch with their token", url)
		}
	}
	if timeline, ok := p["timeline"].(object); ok {
		c.has(timeline, "timeline", map[string]string{"source_start_seconds": "n", "stream_origin_seconds": "n",
			"player_start_seconds": "n", "timeline_offset_seconds": "n", "can_seek_anywhere": "b", "seek_restoration": "s"})
		for _, key := range []string{"source_start_seconds", "stream_origin_seconds", "player_start_seconds", "timeline_offset_seconds"} {
			if n, _ := timeline[key].(float64); n < 0 {
				c.fail("timeline %s %v", key, n)
			}
		}
		if restoration := timeline["seek_restoration"]; restoration != "player_position" && restoration != "source_position" {
			c.fail("seek_restoration %v", restoration)
		}
	}
	if claims, ok := p["claims"].(object); ok {
		c.has(claims, "claims", map[string]string{"video": "o", "audio": "o", "subtitles": "o"})
		if video, ok := claims["video"].(object); ok {
			c.has(video, "claims.video", map[string]string{"hdr10": "b", "hdr10_plus": "b", "hlg": "b", "dolby_vision": "b"})
		}
		if audio, ok := claims["audio"].(object); ok {
			c.has(audio, "claims.audio", map[string]string{"passthrough": "b", "atmos_preserved": "b"})
		}
		if subtitles, ok := claims["subtitles"].(object); ok {
			c.has(subtitles, "claims.subtitles", map[string]string{"ass_styling_preserved": "b", "bitmap_overlay": "b", "bitmap_sidecar": "b"})
		}
	}
	if source, ok := p["source"].(object); ok {
		c.has(source, "source", map[string]string{"media_file_id": "s", "hdr10_plus": "b", "dv_enhancement_layer": "s"})
		if !fileID(source["media_file_id"]) {
			c.fail("source media_file_id %v", source["media_file_id"])
		}
	}
	qualities, _ := p["available_qualities"].([]any)
	for n, q := range qualities {
		rung, _ := q.(object)
		c.has(rung, fmt.Sprintf("available_qualities[%d]", n), map[string]string{"label": "s", "bitrate_kbps": "i", "preserves_source": "b"})
	}
	selected, _ := p["selected_tracks"].(object)
	for _, kind := range []string{"audio", "subtitle"} {
		if track, ok := selected[kind].(object); ok {
			c.has(track, "selected_tracks."+kind, map[string]string{"id": "s"})
		}
	}
	if subtitle, ok := p["subtitle"].(object); ok {
		checkSubtitles(c, subtitle, selected, session)
	}
}

// checkSubtitles checks a plan's subtitles as the apps show them: Apple's
// subtitle decision decoding and plan adapter, Android's validateForMedia3
// (PlaybackPlanV3.kt).
func checkSubtitles(c *checker, subtitle, selected object, session string) {
	c.has(subtitle, "subtitle", map[string]string{"mode": "s", "inventory": "a"})
	mode, _ := subtitle["mode"].(string)
	if !slices.Contains([]string{"off", "render", "convert", "burn_in"}, mode) {
		c.fail("subtitle mode %q", mode)
	}
	sidecars := "/api/v2/stream/" + session + "/subtitles/"
	if artifact, ok := subtitle["artifact"].(object); ok {
		c.has(artifact, "subtitle.artifact", map[string]string{"url": "s", "mime_type": "s", "format": "s", "timing_origin_seconds": "n"})
		if origin, _ := artifact["timing_origin_seconds"].(float64); math.Abs(origin) > 0.001 {
			c.fail("subtitle artifact timing origin %v", origin)
		}
		if url, _ := artifact["url"].(string); !strings.HasPrefix(url, sidecars) || !mediaPath(url) {
			c.fail("subtitle artifact url %q", url)
		}
	} else if mode == "render" || mode == "convert" {
		c.fail("subtitle mode %s without artifact", mode)
	}
	inventory, _ := subtitle["inventory"].([]any)
	ids := map[string]int{}
	for n, entry := range inventory {
		row, _ := entry.(object)
		where := fmt.Sprintf("subtitle.inventory[%d]", n)
		c.has(row, where, map[string]string{"track_id": "s", "combined_index": "i", "source": "s", "forced": "b", "default": "b",
			"hearing_impaired": "b", "delivery": "s"})
		// Android: the combined indices are dense, from 0, and the track
		// identifiers unique.
		if row["combined_index"] != float64(n) {
			c.fail("%s combined_index %v", where, row["combined_index"])
		}
		id, _ := row["track_id"].(string)
		if _, twice := ids[id]; twice {
			c.fail("%s track_id %q twice", where, id)
		}
		ids[id] = n
		url, hasURL := row["url"].(string)
		switch row["delivery"] {
		case "sidecar":
			if !hasURL || !strings.HasPrefix(url, sidecars) || !mediaPath(url) {
				c.fail("%s sidecar url %q", where, url)
			}
		case "burn_in_only":
			if hasURL {
				c.fail("%s burned in, with url %q", where, url)
			}
		}
	}
	if track, ok := selected["subtitle"].(object); ok {
		id, _ := track["id"].(string)
		n, listed := ids[id]
		switch {
		case !listed:
			c.fail("the selected subtitle %q is not in the inventory", id)
		case track["index"] != nil && track["index"] != float64(n):
			c.fail("the selected subtitle %q has index %v, listed %d", id, track["index"], n)
		}
	}
}
