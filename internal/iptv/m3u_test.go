package iptv

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/drm"
)

func parse(t *testing.T, playlist string) ([]Entry, error) {
	t.Helper()
	var entries []Entry
	err := ParseM3U(strings.NewReader(playlist), func(e Entry) error {
		entries = append(entries, e)
		return nil
	})
	return entries, err
}

// A playlist as providers write them: a byte order mark, CRLF line ends,
// attributes quoted every way or not at all, a comma inside a quoted
// attribute, options for the stream's requests, and headings between the
// channels.
const playlist = "\xef\xbb\xbf#EXTM3U url-tvg=\"https://guide.example/epg.xml\"\r\n" +
	"#EXTINF:-1 tvg-id=\"zeb.zz\" tvg-name=\"Zeb One\" tvg-logo=\"https://img.example/zeb.png\" tvg-chno=\"7\" group-title=\"News, Weather\",ZZ| Zeb One HD\r\n" +
	"#EXTVLCOPT:http-user-agent=Player/1.0\r\n" +
	"#EXTVLCOPT:http-referrer=https://portal.example/\r\n" +
	"https://live.example/zeb/one.m3u8\r\n" +
	"#EXTINF:-1 group-title=\"Kids\",##### KIDS #####\r\n" +
	"https://live.example/heading.ts\r\n" +
	"#EXTINF:0 tvg-id='orbe.zz' tvg-logo=https://img.example/orbe.png group-title=Kids,Orbe Junior\r\n" +
	"https://live.example/orbe.ts\r\n" +
	"\r\n" +
	"#EXTINF:-1,=== ---- ===\r\n" +
	"https://live.example/rule.ts\r\n" +
	"#EXTINF:-1 tvg-name=\"Named Only\",\r\n" +
	"#EXTGRP:Music\r\n" +
	"http://live.example/named.ts\r\n" +
	"#EXTINF:-1,No Address\r\n" +
	"#EXTINF:-1,Not Web\r\n" +
	"rtmp://live.example/stream\r\n" +
	"#EXTINF:-1 tvg-chno=\"x\",Plain\r\n" +
	"https://live.example/plain.ts\r\n"

func TestM3UPlaylistsAreRead(t *testing.T) {
	t.Parallel()
	entries, err := parse(t, playlist)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	if strings.Join(names, ";") != "ZZ| Zeb One HD;Orbe Junior;Named Only;Plain" {
		t.Fatalf("channels: %q", names)
	}
	zeb := entries[0]
	if zeb.GuideID != "zeb.zz" || zeb.Logo != "https://img.example/zeb.png" || zeb.Number != 7 || zeb.Group != "News, Weather" ||
		zeb.URL != "https://live.example/zeb/one.m3u8" {
		t.Errorf("attributes: %+v", zeb)
	}
	if zeb.Headers["User-Agent"] != "Player/1.0" || zeb.Headers["Referer"] != "https://portal.example/" || len(zeb.Headers) != 2 {
		t.Errorf("request headers: %v", zeb.Headers)
	}
	orbe := entries[1]
	if orbe.GuideID != "orbe.zz" || orbe.Logo != "https://img.example/orbe.png" || orbe.Group != "Kids" || orbe.Headers != nil {
		t.Errorf("single quotes and bare values: %+v", orbe)
	}
	if named := entries[2]; named.Group != "Music" || named.Number != 0 {
		t.Errorf("tvg-name and #EXTGRP: %+v", named)
	}
	if plain := entries[3]; plain.Number != 0 || plain.GuideID != "" {
		t.Errorf("a number that is not one: %+v", plain)
	}
}

// The headers other players read from a playlist: an #EXTHTTP line's JSON
// object, #EXTVLCOPT:http-origin, and Name=value pairs after a | ending
// the address, which leave the address.
func TestM3UHeaderLinesAreRead(t *testing.T) {
	t.Parallel()
	entries, err := parse(t, "#EXTM3U\n"+
		"#EXTINF:-1,Json\n"+
		`#EXTHTTP:{"user-agent":"Player/2","referer":"https://portal.example/","Cookie":"k=v","Range":"bytes=5-","number":3}`+"\n"+
		"#EXTVLCOPT:http-origin=https://portal.example\n"+
		"https://live.example/json.m3u8\n"+
		"#EXTINF:-1,Piped\n"+
		"#EXTVLCOPT:http-user-agent=Player/1.0\n"+
		"https://live.example/piped.m3u8|User-Agent=Player%2F3%20(TV)&Referer=https://portal.example/\n"+
		"#EXTINF:-1,Bar In The Address\n"+
		"https://live.example/a|b.ts\n")
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	if json := entries[0]; json.Headers["User-Agent"] != "Player/2" || json.Headers["Referer"] != "https://portal.example/" ||
		json.Headers["Cookie"] != "k=v" || json.Headers["Origin"] != "https://portal.example" || len(json.Headers) != 4 {
		t.Errorf("#EXTHTTP and http-origin: %v", json.Headers)
	}
	if piped := entries[1]; piped.URL != "https://live.example/piped.m3u8" || piped.Headers["User-Agent"] != "Player/3 (TV)" ||
		piped.Headers["Referer"] != "https://portal.example/" || len(piped.Headers) != 2 {
		t.Errorf("headers after a |: %q %v", piped.URL, piped.Headers)
	}
	if bar := entries[2]; bar.URL != "https://live.example/a|b.ts" || bar.Headers != nil {
		t.Errorf("an address with a | and no headers: %q %v", bar.URL, bar.Headers)
	}
}

// Some lists give a channel's headers as #EXTINF attributes only; the
// lines that follow, more explicit, replace them.
func TestM3UHeaderAttributesAreRead(t *testing.T) {
	t.Parallel()
	entries, err := parse(t, "#EXTM3U\n"+
		`#EXTINF:-1 tvg-id="" http-referrer="https://portal.example/" http-user-agent="Player/1.0 (TV)" http-origin='https://portal.example',Attributes`+"\n"+
		"https://live.example/1.m3u8\n"+
		`#EXTINF:-1 http-user-agent="Player/1.0" http-referrer="https://portal.example/",Replaced`+"\n"+
		"#EXTVLCOPT:http-user-agent=Player/2.0\n"+
		"https://live.example/2.m3u8|Referer=https://other.example/\n")
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	if got := entries[0].Headers; got["User-Agent"] != "Player/1.0 (TV)" || got["Referer"] != "https://portal.example/" ||
		got["Origin"] != "https://portal.example" || len(got) != 3 {
		t.Errorf("headers of #EXTINF attributes: %v", got)
	}
	if got := entries[1].Headers; got["User-Agent"] != "Player/2.0" || got["Referer"] != "https://other.example/" || len(got) != 2 {
		t.Errorf("attributes replaced by the lines after them: %v", got)
	}
}

func TestOtherDocumentsAreNotPlaylists(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]string{
		"empty":     "",
		"html":      "<html><body>Sign in</body></html>",
		"json":      `{"user_info": {"auth": 0}}`,
		"long line": "#EXTM3U\n#EXTINF:-1," + strings.Repeat("x", maxLine+1) + "\n",
	} {
		if _, err := parse(t, document); !errors.Is(err, ErrInvalidList) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if entries, err := parse(t, "#EXTM3U\n"); err != nil || len(entries) != 0 {
		t.Errorf("an empty playlist: %v %v", entries, err)
	}
}

// Headings drawn with decoration, or without a letter or digit, are not
// channels; names that merely contain symbols are.
func TestHeadingsAreNotChannels(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"##### ZEBRA #####", "### 24/7 ZEB ###", "===== Kids =====", "--- Sports ---", "★★★ VIP ★★★", "|==|", "", "  ---  ",
		"ZZ| ----- NEWS -----"} {
		if !heading(name) {
			t.Errorf("%q is a channel", name)
		}
	}
	for _, name := range []string{"Zeb One", "ZZ| Zeb+ ᴿᵂ", "Zeb 24/7", "Orbe & Co.", "A-B-C", "Zeb (HD)", "#1 Hits", "Zeb: Live", "Q7", "+1"} {
		if heading(name) {
			t.Errorf("%q is a heading", name)
		}
	}
}

// #KODIPROP lines say how a stream is encrypted, before the #EXTINF line
// or after it: the manifest and licence types, ClearKey keys or the
// licence server that gives them, with its headers, and the headers of
// the manifest and of the segments, which are the stream's.
func TestKodiPropertiesAreRead(t *testing.T) {
	t.Parallel()
	entries, err := parse(t, "#EXTM3U\n"+
		"#EXTINF:-1 tvg-id=\"zeb.zz\",Zeb Keys\n"+
		"#KODIPROP:inputstream=inputstream.adaptive\n"+
		"#KODIPROP:inputstream.adaptive.manifest_type=mpd\n"+
		"#KODIPROP:inputstream.adaptive.license_type=org.w3.clearkey\n"+
		"#KODIPROP:inputstream.adaptive.license_key=0123456789abcdef0123456789abcdef:00112233445566778899aabbccddeeff\n"+
		"#KODIPROP:inputstream.adaptive.manifest_headers=User-Agent=Player%2F3&Referer=https://portal.example/\n"+
		"https://live.example/zeb/manifest.mpd\n"+
		"#KODIPROP:inputstream.adaptive.license_type=clearkey\n"+
		"#KODIPROP:inputstream.adaptive.license_key=https://licence.example/ck?t=1|X-Token=abc&Origin=https://portal.example|R{SSM}|\n"+
		"#KODIPROP:inputstream.adaptive.stream_headers=Cookie=k%3Dv\n"+
		"#EXTINF:-1,Orbe Server\n"+
		"https://live.example/orbe/manifest.mpd\n"+
		"#EXTINF:-1,Plume Widevine\n"+
		"#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha\n"+
		"#KODIPROP:inputstream.adaptive.license_key=https://licence.example/wv\n"+
		"https://live.example/plume/manifest.mpd\n"+
		"#EXTINF:-1,Kite Untyped\n"+
		"#KODIPROP:inputstream.adaptive.license_key=ASNFZ4mrze8BI0VniavN7w:ABEiM0RVZneImaq7zN3u_w\n"+
		"https://live.example/kite.mpd\n"+
		"#EXTINF:-1,Plain\n"+
		"https://live.example/plain.ts\n")
	if err != nil || len(entries) != 5 {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	keys := []drm.Key{{ID: "0123456789abcdef0123456789abcdef", Value: "00112233445566778899aabbccddeeff"}}
	zeb := entries[0]
	if p := zeb.Protection; p == nil || p.Manifest != "mpd" || p.License != drm.ClearKey || !slices.Equal(p.Keys, keys) || p.LicenseURL != "" {
		t.Errorf("keys in the list: %+v", p)
	}
	if zeb.Headers["User-Agent"] != "Player/3" || zeb.Headers["Referer"] != "https://portal.example/" || len(zeb.Headers) != 2 {
		t.Errorf("manifest headers: %v", zeb.Headers)
	}
	orbe := entries[1]
	if p := orbe.Protection; p == nil || p.License != drm.ClearKey || p.Keys != nil || p.LicenseURL != "https://licence.example/ck?t=1" ||
		p.LicenseHeaders["X-Token"] != "abc" || p.LicenseHeaders["Origin"] != "https://portal.example" || len(p.LicenseHeaders) != 2 {
		t.Errorf("a licence server, before the #EXTINF line: %+v", p)
	}
	if orbe.Headers["Cookie"] != "k=v" || len(orbe.Headers) != 1 {
		t.Errorf("stream headers: %v", orbe.Headers)
	}
	if p := entries[2].Protection; p == nil || !p.Unsupported() || p.Name() != "Widevine" {
		t.Errorf("a Widevine licence: %+v", p)
	}
	if p := entries[3].Protection; p == nil || p.License != drm.ClearKey || !slices.Equal(p.Keys, keys) {
		t.Errorf("keys without a licence type: %+v", p)
	}
	if entries[4].Protection != nil {
		t.Errorf("a stream the list says nothing of: %+v", entries[4].Protection)
	}
}
