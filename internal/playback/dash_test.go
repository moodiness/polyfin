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
