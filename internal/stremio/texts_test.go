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
