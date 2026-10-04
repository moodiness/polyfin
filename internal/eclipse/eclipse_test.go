package eclipse

import (
	"errors"
	"testing"
)

func TestManifestsAreToldApartFromStremioOnes(t *testing.T) {
	for manifest, want := range map[string]bool{
		`{"id":"a","name":"A","version":"1","resources":["search","stream"]}`:                                    true,
		`{"id":"a","name":"A","version":"1","resources":["stream"],"types":["track","album"]}`:                   true,
		`{"id":"a","name":"A","version":"1","resources":["stream"],"contentType":"podcast"}`:                     true,
		`{"id":"a","name":"A","version":"1","resources":["catalog","meta","stream"],"types":["movie","series"]}`: false,
		`{"id":"a","name":"A","version":"1","resources":[{"name":"stream","types":["movie"]}],"catalogs":[]}`:    false,
		`{"id":"a","name":"A","version":"1","resources":["catalog"],"catalogs":[{"type":"movie","id":"top"}]}`:   false,
	} {
		if got := Detect([]byte(manifest)); got != want {
			t.Errorf("Detect(%s) = %v", manifest, got)
		}
	}
}

func TestSettingsReduceToOneValueAndTravelAsQuery(t *testing.T) {
	m, err := ParseManifest([]byte(`{"id":"a","name":"A","version":"1.0.0","resources":["search","stream","settings"],
		"settings":[
			{"key":"quality","type":"select","label":"Quality","perNetwork":true,"default":{"wifi":"high","cellular":"low"},
				"options":[{"value":"high","label":"High"},{"value":"low","label":"Low"}]},
			{"key":"preferOpus","type":"toggle","label":"Opus","default":true},
			{"key":"region","type":"text","label":"Region","default":"US","maxLength":2},
			{"key":"volume","type":"number","label":"Volume","default":5,"min":0,"max":10,"step":1},
			{"key":"broken","type":"select","label":"No options"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Settings) != 4 || m.Settings[0].Default != "high" || m.ContentType != ContentMusic {
		t.Fatalf("settings: %+v", m)
	}
	if got := m.Query(nil).Encode(); got != "preferOpus=true&quality=high&region=US&volume=5" {
		t.Errorf("defaults: %s", got)
	}
	chosen, err := m.CheckSettings(map[string]string{"quality": "low", "preferOpus": "FALSE", "volume": "7"})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Query(chosen).Encode(); got != "preferOpus=false&quality=low&region=US&volume=7" {
		t.Errorf("chosen: %s", got)
	}
	for _, bad := range []map[string]string{{"quality": "medium"}, {"region": "USA"}, {"volume": "11"}, {"volume": "2.5"},
		{"preferOpus": "maybe"}, {"unknown": "x"}} {
		if _, err := m.CheckSettings(bad); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%v accepted", bad)
		}
	}
}
