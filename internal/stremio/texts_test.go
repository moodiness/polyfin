package stremio

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCutKeepsWholeCharacters(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		text string
		most int
		want string
	}{
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
		{"aé", 2, "a"},
		{"a€b", 3, "a"},
		{"a€b", 4, "a€"},
		{"€", 0, ""},
	} {
		if got := Cut(c.text, c.most); got != c.want {
			t.Errorf("Cut(%q, %d) = %q, want %q", c.text, c.most, got, c.want)
		}
	}
}

// Every text a description gives is bounded as it is read: names at
// MaxName, overviews at MaxOverview, on a character.
func TestTextsOfADescriptionAreBounded(t *testing.T) {
	t.Parallel()
	long := "x" + strings.Repeat("é", 1<<20)
	text, _ := json.Marshal(long)
	s := string(text)
	raw := `{"id":"tt1","type":"series","name":` + s + `,"description":` + s + `,"genres":[` + s + `],"director":` + s +
		`,"releaseInfo":` + s + `,"status":` + s + `,"language":` + s +
		`,"links":[{"name":` + s + `,"category":"Writers"}],"trailers":[{"source":"a","name":` + s + `}],` +
		`"app_extras":{"cast":[{"name":` + s + `,"character":` + s + `}],"certification":` + s + `},` +
		`"videos":[{"id":"tt1:1:1","season":1,"episode":1,"title":` + s + `,"name":` + s + `,"overview":` + s + `}]}`
	var meta Meta
	if err := decodeMeta([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	check := func(where, got string, most int) {
		t.Helper()
		if got == "" || len(got) > most || !utf8.ValidString(got) || !strings.HasPrefix(long, got) {
			t.Errorf("%s: %d bytes, valid %v", where, len(got), utf8.ValidString(got))
		}
	}
	check("name", meta.Name, MaxName)
	check("description", meta.Description, MaxOverview)
	check("release", string(meta.ReleaseInfo), MaxName)
	check("status", meta.Status, MaxName)
	check("language", meta.Language, MaxName)
	check("genre", meta.Genres[0], MaxName)
	check("director", meta.Director[0], MaxName)
	check("writer", meta.Writer[0], MaxName)
	check("trailer", meta.Trailers[0].Name, MaxName)
	check("cast", meta.Extras.Cast[0].Name, MaxName)
	check("character", meta.Extras.Cast[0].Character, MaxName)
	check("certification", meta.Extras.Certification, MaxName)
	check("episode title", meta.Videos[0].Title, MaxName)
	check("episode name", meta.Videos[0].Name, MaxName)
	check("episode overview", meta.Videos[0].Overview, MaxOverview)

	// A list of names given as one text is read up to as many as a list.
	var names Names
	if err := json.Unmarshal([]byte(`"`+strings.Repeat("a,", 3*maxNested)+`"`), &names); err != nil || len(names) != maxNested {
		t.Errorf("%d names, %v", len(names), err)
	}

	manifest, err := ParseManifest([]byte(`{"id":"a","name":` + s + `,"version":"1","resources":["catalog"],` +
		`"catalogs":[{"type":"movie","id":"top","name":` + s + `}]}`))
	if err != nil {
		t.Fatal(err)
	}
	check("addon", manifest.Name, MaxName)
	check("catalog", manifest.Catalogs[0].Name, MaxName)
}

// An addon's description is cut at MaxOverview, on a character, and one
// within it is kept whole.
func TestTheDescriptionOfAnAddonIsBounded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ description, want string }{
		{"An addon.", "An addon."},
		{"x" + strings.Repeat("é", MaxOverview), "x" + strings.Repeat("é", MaxOverview/2-1)},
	} {
		text, _ := json.Marshal(c.description)
		manifest, err := ParseManifest([]byte(`{"id":"a","name":"A","description":` + string(text) + `,"version":"1","resources":["meta"]}`))
		if err != nil {
			t.Fatal(err)
		}
		if manifest.Description != c.want {
			t.Errorf("description of %d bytes, want %d", len(manifest.Description), len(c.want))
		}
	}
}

// A stream's names are cut at MaxName and its description at MaxOverview;
// an address past MaxURL is left out, never cut, and a stream left with
// nothing to tell is left out with it. A stream a description's video
// gives is bounded as one the stream resource lists.
func TestTextsAndAddressesOfStreamsAreBounded(t *testing.T) {
	t.Parallel()
	long := "x" + strings.Repeat("é", MaxOverview)
	text, _ := json.Marshal(long)
	s := string(text)
	tooLong := `"https://f.example/` + strings.Repeat("a", MaxURL) + `"`
	atBound := "https://f.example/" + strings.Repeat("a", MaxURL-len("https://f.example/"))
	subtitles := `[{"id":"1","url":` + tooLong + `,"lang":"en"},{"id":"2","url":"https://f.example/a.srt","lang":"en"}]`
	streams := `[{"name":` + s + `,"title":` + s + `,"description":` + s + `,"url":"` + atBound + `",` +
		`"behaviorHints":{"bingeGroup":` + s + `,"filename":` + s + `},"sources":[` + tooLong + `,"dht:a"],"subtitles":` + subtitles + `},` +
		`{"url":` + tooLong + `},{"externalUrl":` + tooLong + `,"ytId":` + tooLong + `,"nzbUrl":` + tooLong + `,"infoHash":` + tooLong + `}]`
	cut := func(where, got string, most int) {
		t.Helper()
		if got == "" || len(got) > most || !utf8.ValidString(got) || !strings.HasPrefix(long, got) {
			t.Errorf("%s: %d bytes, valid %v", where, len(got), utf8.ValidString(got))
		}
	}
	check := func(where string, got []Stream) {
		t.Helper()
		if len(got) != 1 {
			t.Fatalf("%s: %d streams, want 1", where, len(got))
		}
		stream := got[0]
		cut(where+" name", stream.Name, MaxName)
		cut(where+" title", stream.Title, MaxName)
		cut(where+" description", stream.Description, MaxOverview)
		cut(where+" binge group", stream.BehaviorHints.BingeGroup, MaxName)
		cut(where+" file name", stream.BehaviorHints.Filename, MaxName)
		if stream.URL != atBound {
			t.Errorf("%s: address of %d bytes, want the one of %d", where, len(stream.URL), len(atBound))
		}
		if len(stream.Sources) != 1 || stream.Sources[0] != "dht:a" {
			t.Errorf("%s: sources %d", where, len(stream.Sources))
		}
		if len(stream.Subtitles) != 1 || stream.Subtitles[0].URL != "https://f.example/a.srt" {
			t.Errorf("%s: %d subtitles", where, len(stream.Subtitles))
		}
	}

	listed, err := decodeList([]byte(streams), MaxListed, &room{left: listedRoom}, decodeStream, tells)
	if err != nil {
		t.Fatal(err)
	}
	check("listed", listed)

	var meta Meta
	if err := decodeMeta([]byte(`{"id":"tt1","type":"series","name":"A","videos":[{"id":"tt1:1:1","season":1,"episode":1,"streams":`+streams+`}]}`), &meta); err != nil {
		t.Fatal(err)
	}
	check("embedded", meta.Videos[0].Streams)

	listedSubtitles, err := decodeList([]byte(subtitles), MaxListed, &room{left: listedRoom}, whole[Subtitle], downloadable)
	if err != nil || len(listedSubtitles) != 1 || listedSubtitles[0].URL != "https://f.example/a.srt" {
		t.Errorf("%d subtitles, %v", len(listedSubtitles), err)
	}
}

// The images, thumbnails and trailers of a description, and an addon's
// logo, are left out past MaxURL and kept whole within it.
func TestAddressesOfADescriptionPastTheirBoundAreLeftOut(t *testing.T) {
	t.Parallel()
	describe := func(link string) (Meta, Manifest) {
		t.Helper()
		l := `"` + link + `"`
		var meta Meta
		if err := decodeMeta([]byte(`{"id":"tt1","type":"series","name":"A","poster":`+l+`,"background":`+l+`,"logo":`+l+
			`,"landscapePoster":`+l+`,"trailers":[{"source":`+l+`}],"videos":[{"id":"tt1:1:1","season":1,"episode":1,"thumbnail":`+l+`}],`+
			`"app_extras":{"cast":[{"name":"B","photo":`+l+`}],"seasonPosters":[`+l+`,"https://images.example/2.jpg"],"seasonPosterByNumber":{"3":`+l+`}}}`), &meta); err != nil {
			t.Fatal(err)
		}
		manifest, err := ParseManifest([]byte(`{"id":"a","name":"A","version":"1","resources":["meta"],"logo":` + l + `}`))
		if err != nil {
			t.Fatal(err)
		}
		return meta, manifest
	}
	links := func(meta Meta, manifest Manifest) []string {
		return []string{meta.Poster, meta.Background, meta.Logo, meta.LandscapePoster, meta.Trailers[0].Source,
			meta.Videos[0].Thumbnail, meta.Extras.Cast[0].Photo, meta.Extras.SeasonPosters[0], meta.Extras.SeasonPosterByNumber["3"], manifest.Logo}
	}

	ordinary := "https://images.example/" + strings.Repeat("a", MaxURL-len("https://images.example/"))
	for i, got := range links(describe(ordinary)) {
		if got != ordinary {
			t.Errorf("address %d: %d bytes, want %d", i, len(got), len(ordinary))
		}
	}
	meta, manifest := describe(ordinary + "a")
	for i, got := range links(meta, manifest) {
		if got != "" {
			t.Errorf("address %d: %d bytes, want none", i, len(got))
		}
	}
	if _, kept := meta.Extras.SeasonPosterByNumber["3"]; kept || len(meta.Extras.SeasonPosters) != 2 || meta.Extras.SeasonPosters[1] != "https://images.example/2.jpg" {
		t.Errorf("%d season posters, %d by number", len(meta.Extras.SeasonPosters), len(meta.Extras.SeasonPosterByNumber))
	}
}
