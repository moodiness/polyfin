package playback

import (
	"encoding/xml"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// folderOf names an origin's root as the loopback interface would, its
// origin readable.
func folderOf(origin string) string { return "http://lo/live/k/~" + origin + "/" }

// Every address of a DASH manifest is named on the loopback interface, so
// that FFmpeg fetches its files through Polyfin: BaseURLs resolved as DASH
// resolves them, the manifest's own folder when it names none, and the
// absolute addresses of templates and segment lists. Relative ones stay,
// templates whole, and so does what the manifest says of its encryption.
func TestDASHManifestsAreRelayed(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://cdn.example/live/zeb/manifest.mpd?token=t")
	manifest := `<?xml version="1.0" encoding="utf-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:cenc="urn:mpeg:cenc:2013" type="static">
	<Period id="0">
		<AdaptationSet contentType="video">
			<ContentProtection schemeIdUri="urn:mpeg:dash:mp4protection:2011" value="cenc" cenc:default_KID="01234567-89ab-cdef-0123-456789abcdef"/>
			<Representation id="v1" bandwidth="1000">
				<SegmentTemplate media="chunk-$RepresentationID$-$Number%05d$.m4s" initialization="init-$RepresentationID$.m4s" startNumber="1"/>
			</Representation>
		</AdaptationSet>
		<AdaptationSet contentType="audio">
			<BaseURL>../audio/</BaseURL>
			<Representation id="a1" bandwidth="100">
				<BaseURL>https://other.example:8443/a1/</BaseURL>
				<BaseURL>https://spare.example/a1/</BaseURL>
				<SegmentTemplate media="/abs/$Number$.m4s?x=1&amp;y=2" initialization="https://init.example/i/$RepresentationID$.mp4"/>
			</Representation>
			<Representation id="a2" bandwidth="100">
				<SegmentList>
					<Initialization sourceURL="//third.example/a2/init.mp4"/>
					<SegmentURL media="s1.m4s"/>
				</SegmentList>
			</Representation>
		</AdaptationSet>
	</Period>
</MPD>`
	rewritten, err := rewriteManifest([]byte(manifest), base, folderOf)
	if err != nil {
		t.Fatal(err)
	}
	out := string(rewritten)
	for _, want := range []string{
		// The manifest names no BaseURL: its own folder is its.
		`type="static"><BaseURL>http://lo/live/k/~https://cdn.example/live/zeb/</BaseURL>`,
		`<SegmentTemplate media="chunk-$RepresentationID$-$Number%05d$.m4s" initialization="init-$RepresentationID$.m4s" startNumber="1"/>`,
		`cenc:default_KID="01234567-89ab-cdef-0123-456789abcdef"`,
		`<BaseURL>http://lo/live/k/~https://cdn.example/live/audio/</BaseURL>`,
		`<BaseURL>http://lo/live/k/~https://other.example:8443/a1/</BaseURL>`,
		`<BaseURL>http://lo/live/k/~https://spare.example/a1/</BaseURL>`,
		// An absolute path is of the representation's own host.
		`media="http://lo/live/k/~https://other.example:8443/abs/$Number$.m4s?x=1&amp;y=2"`,
		`initialization="http://lo/live/k/~https://init.example/i/$RepresentationID$.mp4"`,
		`<Initialization sourceURL="http://lo/live/k/~https://third.example/a2/init.mp4"/>`,
		`<SegmentURL media="s1.m4s"/>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no %s in\n%s", want, out)
		}
	}
	if err := xml.Unmarshal(rewritten, new(struct{})); err != nil {
		t.Errorf("the manifest is no XML document: %v", err)
	}
	info, err := readManifest([]byte(manifest))
	if err != nil || len(info.kids) != 1 || info.kids[0] != "0123456789abcdef0123456789abcdef" || info.named() != "" {
		t.Errorf("encryption: %+v %v", info, err)
	}

	// An address FFmpeg would read itself refuses the manifest.
	for _, unsafe := range []string{
		`<MPD><BaseURL>file:///etc/</BaseURL></MPD>`,
		`<MPD><Period><AdaptationSet><SegmentTemplate media="file:///etc/$Number$"/></AdaptationSet></Period></MPD>`,
	} {
		if _, err := rewriteManifest([]byte(unsafe), base, folderOf); !errors.Is(err, ErrUnsafePlaylist) {
			t.Errorf("%s: %v", unsafe, err)
		}
	}
}

// A live manifest of several periods keeps its last one, which plays now:
// FFmpeg plays one period, the longest, where the current one has no
// duration yet. A static one keeps them all.
func TestDASHLiveManifestsKeepTheirLastPeriod(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("http://cdn.example/m.mpd")
	periods := `<Period id="advert" duration="PT30S"><AdaptationSet/></Period>
	<Period id="now"><AdaptationSet/></Period>
</MPD>`
	live, err := rewriteManifest([]byte(`<MPD type="dynamic"><BaseURL>http://cdn.example/</BaseURL>
	`+periods), base, folderOf)
	if err != nil || strings.Contains(string(live), `id="advert"`) || !strings.Contains(string(live), `<Period id="now">`) {
		t.Errorf("a live manifest: %s %v", live, err)
	}
	static, err := rewriteManifest([]byte(`<MPD type="static">
	`+periods), base, folderOf)
	if err != nil || !strings.Contains(string(static), `id="advert"`) || !strings.Contains(string(static), `<Period id="now">`) {
		t.Errorf("a static manifest: %s %v", static, err)
	}
}

// namedItself reports whether a rewritten manifest still names origin
// itself somewhere, rather than under the loopback interface's folder for
// it (see folderOf).
func namedItself(rewritten []byte, origin string) bool {
	return strings.Count(string(rewritten), origin) != strings.Count(string(rewritten), "~"+origin)
}

// FFmpeg's dash demuxer knows the elements of a manifest whatever the case
// of their names, and whatever their namespace's prefix: so does the
// rewriting, which names their addresses on the loopback interface.
func TestManifestElementsAreKnownWhateverTheirCase(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/manifest.mpd")
	const local = "http://192.168.1.10"
	for _, tc := range []struct {
		name string
		as   func(element string) string
	}{
		{"as the standard writes them", func(element string) string { return element }},
		{"in lower case", strings.ToLower},
		{"in upper case", strings.ToUpper},
		{"with a prefix", func(element string) string { return "d:" + element }},
	} {
		manifest := `<?xml version="1.0" encoding="utf-8"?>
<` + tc.as("MPD") + ` xmlns="urn:mpeg:dash:schema:mpd:2011" xmlns:d="urn:mpeg:dash:schema:mpd:2011" type="dynamic">
 <` + tc.as("BaseURL") + `>` + local + `/mpd/</` + tc.as("BaseURL") + `>
 <` + tc.as("Period") + ` id="then" duration="PT30S"><AdaptationSet><` + tc.as("BaseURL") + `>` + local + `/then/</` + tc.as("BaseURL") + `></AdaptationSet></` + tc.as("Period") + `>
 <` + tc.as("Period") + ` id="now">
  <AdaptationSet contentType="video">
   <` + tc.as("SegmentList") + ` media="` + local + `/list/$Number$.m4s" initialization="` + local + `/list/init.m4s"/>
   <Representation id="0" bandwidth="100000">
    <` + tc.as("BaseURL") + `>` + local + `/nvr/</` + tc.as("BaseURL") + `>
    <` + tc.as("SegmentTemplate") + ` initialization="` + local + `/t/init.m4s" media="` + local + `/t/chunk-$Number$.m4s" startNumber="1"/>
   </Representation>
   <Representation id="1" bandwidth="100000">
    <SegmentList>
     <` + tc.as("Initialization") + ` sourceURL="` + local + `/l/init.m4s"/>
     <` + tc.as("SegmentURL") + ` media="` + local + `/l/1.m4s"/>
    </SegmentList>
   </Representation>
  </AdaptationSet>
 </` + tc.as("Period") + `>
</` + tc.as("MPD") + `>`
		rewritten, err := rewriteManifest([]byte(manifest), base, folderOf)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if namedItself(rewritten, local) {
			t.Errorf("%s: an address is left as it is in\n%s", tc.name, rewritten)
		}
		for _, want := range []string{">" + folderOf(local) + "nvr/<", `"` + folderOf(local) + `list/$Number$.m4s"`, `"` + folderOf(local) + `l/1.m4s"`} {
			if !strings.Contains(string(rewritten), want) {
				t.Errorf("%s: no %s in\n%s", tc.name, want, rewritten)
			}
		}
		// The live manifest keeps its last period alone, whatever its
		// elements are called.
		if strings.Contains(string(rewritten), `id="then"`) {
			t.Errorf("%s: the period that ended is kept in\n%s", tc.name, rewritten)
		}
		// The manifest names a BaseURL of its own: it is given no other.
		if strings.Contains(string(rewritten), folderOf("https://tv.example")) {
			t.Errorf("%s: the manifest is given its own folder in\n%s", tc.name, rewritten)
		}
	}
}

// A manifest FFmpeg would read otherwise than the rewriting does is
// refused: one whose template spells a scheme once filled in, which
// FFmpeg resolves first, one in an encoding whose bytes are not ASCII's,
// one with a document type declaration, whose entities and default
// attributes are written nowhere else, a BaseURL holding an element, and
// a document that is no manifest.
func TestManifestsFFmpegWouldReadOtherwiseAreRefused(t *testing.T) {
	t.Parallel()
	base, _ := url.Parse("https://tv.example/live/manifest.mpd")
	template := func(attribute string) string {
		return `<MPD type="static"><Period><AdaptationSet><Representation id="http" bandwidth="http"><SegmentTemplate ` + attribute + `/></Representation></AdaptationSet></Period></MPD>`
	}
	for name, manifest := range map[string]string{
		"a scheme spelled by the representation's identifier": template(`media="$RepresentationID$://192.168.1.10/chunk-$Number$.m4s"`),
		"a scheme spelled by the representation's bandwidth":  template(`initialization="$Bandwidth$://192.168.1.10/init.m4s"`),
		"a scheme an empty identifier joins":                  template(`media="ht$RepresentationID$tp://192.168.1.10/chunk-$Number$.m4s"`),
		"a scheme FFmpeg alone reads":                         template(`media="1http://192.168.1.10/chunk-$Number$.m4s"`),
		"a scheme without a host":                             template(`media="http:192.168.1.10/chunk-$Number$.m4s"`),
		"UTF-7":                                               `<?xml version="1.0" encoding="UTF-7"?><MPD type="static"><BaseURL>+AGgAdAB0AHAAOgAvAC8-192.168.1.10/</BaseURL></MPD>`,
		"UTF-16 declared":                                     `<?xml version="1.0" encoding="UTF-16"?><MPD type="static"/>`,
		"an encoding by another name":                         `<?xml version="1.0" encoding="EBCDIC-CP-US"?><MPD type="static"/>`,
		"a declaration read no other way":                     `<?xml version="1.0" encoding="utf-8" encoding="UTF-7"?><MPD type="static"/>`,
		"an entity":                                           `<!DOCTYPE MPD [<!ENTITY local "http://192.168.1.10/">]><MPD type="static"><BaseURL>&local;</BaseURL></MPD>`,
		"a default attribute":                                 `<!DOCTYPE MPD [<!ATTLIST SegmentTemplate media CDATA "http://192.168.1.10/$Number$.m4s">]><MPD type="static"/>`,
		"a BaseURL holding an element":                        `<MPD type="static"><BaseURL>http://192.168.1.10/<b/>nvr/</BaseURL></MPD>`,
		"another document":                                    `<html><MPD type="static"><BaseURL>http://192.168.1.10/</BaseURL></MPD></html>`,
		"no document":                                         `http://192.168.1.10/`,
	} {
		if rewritten, err := rewriteManifest([]byte(manifest), base, folderOf); !errors.Is(err, ErrUnsafePlaylist) {
			t.Errorf("%s: got %v, want the manifest refused:\n%s", name, err, rewritten)
		}
	}
	// UTF-16 itself is no document to the rewriting.
	utf16 := []byte{0xff, 0xfe}
	for _, c := range []byte(`<MPD type="static"><BaseURL>http://192.168.1.10/</BaseURL></MPD>`) {
		utf16 = append(utf16, c, 0)
	}
	if rewritten, err := rewriteManifest(utf16, base, folderOf); err == nil {
		t.Errorf("UTF-16 by its byte order mark is rewritten:\n%s", rewritten)
	}
	for name, declaration := range map[string]string{
		"none":          ``,
		"UTF-8":         `<?xml version="1.0" encoding="UTF-8"?>`,
		"no encoding":   `<?xml version="1.0"?>`,
		"standalone":    `<?xml version='1.0' encoding='utf-8' standalone='yes'?>`,
		"ISO 8859-1":    `<?xml version="1.0" encoding="ISO-8859-1"?>`,
		"ASCII":         `<?xml version="1.0" encoding="us-ascii" ?>`,
		"a style sheet": `<?xml version="1.0"?><?xml-stylesheet href="http://192.168.1.10/style.xsl"?>`,
	} {
		rewritten, err := rewriteManifest([]byte(declaration+`<MPD type="static"><BaseURL>http://192.168.1.10/nvr/</BaseURL></MPD>`), base, folderOf)
		if err != nil || !strings.Contains(string(rewritten), "<BaseURL>"+folderOf("http://192.168.1.10")+"nvr/</BaseURL>") {
			t.Errorf("a declaration of %s: %s, %v", name, rewritten, err)
		}
	}
}

// An answer is a manifest wherever FFmpeg's probe would tell one: the
// start of an MPD element anywhere in its first bytes, in any case, up to
// a NUL.
func TestAManifestIsToldWhereFFmpegTellsOne(t *testing.T) {
	t.Parallel()
	for head, want := range map[string]bool{
		`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011">`:                     true,
		`<mpd xmlns="urn:mpeg:dash:schema:mpd:2011">`:                     true,
		"\xef\xbb\xbf<?xml version=\"1.0\"?>\n<!-- live -->\n<Mpd ":       true,
		strings.Repeat(" ", 3000) + "<MPD ":                               true,
		"<!DOCTYPE x>\n<MPD ":                                             true,
		"dash:profile:isoff-live:2011 <MPD":                               true,
		"#EXTM3U\n#EXT-X-VERSION:3\n":                                     false,
		"<?xml version=\"1.0\"?><tt xmlns=\"http://www.w3.org/ns/ttml\">": false,
		"\x00\x00\x00\x1cftypiso5 <MPD":                                   false,
		"G\x40\x00\x10\x00<MPD":                                           false,
	} {
		if got := dashStart([]byte(head)); got != want {
			t.Errorf("dashStart(%q) = %t, want %t", head, got, want)
		}
	}
}
