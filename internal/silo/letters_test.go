package silo

import (
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/silo/silotest"
)

// The letters rule takes letters that hold every title once, in list
// order, and runs that cover the list from its start; a seek out of the
// contract's range, and letters in an order that is not one, are refused.
func TestLettersAsTheAppsReadThem(t *testing.T) {
	t.Parallel()
	good := `{"total":5,"letters":[{"letter":"#","count":2,"position":0},{"letter":"A","count":2,"position":1},` +
		`{"letter":"Z","count":1,"position":3}],"runs":[{"letter":"#","position":0},{"letter":"A","position":1},` +
		`{"letter":"Z","position":3},{"letter":"#","position":4}]}`
	if err := silotest.CheckLetters([]byte(good)); err != nil {
		t.Errorf("good letters: %v", err)
	}
	if err := silotest.CheckLetters([]byte(`{"total":0,"letters":[],"runs":[]}`)); err != nil {
		t.Errorf("no titles: %v", err)
	}
	for name, data := range map[string]string{
		"counts short of total":  `{"total":3,"letters":[{"letter":"A","count":2,"position":0}],"runs":[{"letter":"A","position":0}]}`,
		"a run not from 0":       `{"total":2,"letters":[{"letter":"A","count":2,"position":1}],"runs":[{"letter":"A","position":1}]}`,
		"a letter of two":        `{"total":1,"letters":[{"letter":"AB","count":1,"position":0}],"runs":[{"letter":"AB","position":0}]}`,
		"runs of one letter met": `{"total":2,"letters":[{"letter":"A","count":2,"position":0}],"runs":[{"letter":"A","position":0},{"letter":"A","position":1}]}`,
		"no runs":                `{"total":1,"letters":[{"letter":"A","count":1,"position":0}]}`,
	} {
		if silotest.CheckLetters([]byte(data)) == nil {
			t.Errorf("%s passes", name)
		}
	}
	h := newHarness(t, browsing)
	if w := h.do(http.MethodGet, "/api/v2/catalog?source=query&sort=title&seek=10000001", nil); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("seek past the contract's range: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodPost, "/api/v2/catalog/query", map[string]any{"source": "query", "seek": -1}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("a negative seek in a body: %d %s", w.Code, w.Body)
	}
	if w := h.do(http.MethodPost, "/api/v2/catalog/letters", map[string]any{"order": "sideways"}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("letters in no order: %d %s", w.Code, w.Body)
	}
}
