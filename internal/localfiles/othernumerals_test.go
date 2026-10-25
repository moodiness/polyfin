package localfiles

import "testing"

// The second search writes a title's numerals the other way, only those
// the looser rules read alike: a title of one word, a single letter
// starting a title or inside it, words written with other Roman letters
// and numbers written in words elsewhere than after "Part" have no other
// form.
func TestOtherNumerals(t *testing.T) {
	for title, want := range map[string]string{
		"Harbor Lights 2":           "Harbor Lights II",
		"Harbor Lights II":          "Harbor Lights 2",
		"Harbor Lights XIV":         "Harbor Lights 14",
		"Apollo 13":                 "Apollo XIII",
		"Harbor Lights 5":           "Harbor Lights V",
		"Harbor Lights V":           "Harbor Lights 5",
		"Tidewater Part 1":          "Tidewater Part One",
		"Tidewater - Part One":      "Tidewater - Part 1",
		"Tidewater Part I":          "Tidewater Part 1",
		"Tidewater Part 2: The End": "Tidewater Part Two: The End",
		"Tidewater Part 25":         "Tidewater Part XXV",
		"Rocky 2 and 3":             "Rocky II and III",
		"I Robot":                   "",
		"V for Vendetta":            "",
		"5 for Vendetta":            "",
		"Harbor 5 Lights":           "",
		"XXX":                       "",
		"2":                         "",
		"One Day":                   "",
		"Harbor Lights Two":         "",
		"The Mix":                   "",
		"Civil War":                 "",
		"Harbor Lights 2001":        "",
		"Harbor Lights 40":          "",
		"Harbor Lights 07":          "",
		"Harbor Lights":             "",
	} {
		if got := otherNumerals(title); got != want {
			t.Errorf("%q: %q, want %q", title, got, want)
		}
	}
}
