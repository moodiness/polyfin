package silo

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"net/http"
	"slices"
)

// followedJSON records what Polyfin follows of Silo: the commit of
// Silo-Server/silo-server whose API contract Polyfin answers, with that
// file's SHA-256, and the commits of the apps whose rules silotest
// encodes, with the app files those rules cite. scripts/silofollow reports
// what changed in them since; docs/development.md says how to update it.
//
//go:embed followed.json
var followedJSON []byte

// Pins is followed.json.
type Pins struct {
	Contract ContractPin `json:"contract"`
	Apps     []AppPin    `json:"apps"`
}

// ContractPin is the contract followed: the file of a repository at a
// commit, and its SHA-256, which system/info tells the apps.
type ContractPin struct {
	Repository string `json:"repository"`
	Path       string `json:"path"`
	Commit     string `json:"commit"`
	SHA256     string `json:"sha256"`
}

// AppPin is an app whose rules silotest encodes: its repository, the
// commit they were read at, and the files they come from.
type AppPin struct {
	Repository string   `json:"repository"`
	Commit     string   `json:"commit"`
	Files      []string `json:"files"`
}

// Followed is what Polyfin follows of Silo.
var Followed = readPins()

func readPins() Pins {
	var pins Pins
	decoder := json.NewDecoder(bytes.NewReader(followedJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&pins); err != nil {
		panic("silo: followed.json: " + err.Error())
	}
	return pins
}

// routes is the API's mux, which keeps the pattern of each route as it is
// added.
type routes struct {
	*http.ServeMux
	patterns []string
}

func (r *routes) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
	r.ServeMux.HandleFunc(pattern, handler)
}

// Operations are the operations the API serves, its routes' patterns
// ("GET /api/v2/system/info"), those it answers as unsupported included:
// scripts/silofollow compares them in the contract followed and the
// current one.
func Operations() []string {
	return slices.Clone(New(Options{}).mux.patterns)
}
