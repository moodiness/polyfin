package server

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The words Polyfin's script adds to jellyfin-web are in every language
// Polyfin speaks, keyed as the document's lang starts ("pt" for Brazilian
// Portuguese), each with every word of the English.
func TestWebClientWordsInEveryLanguage(t *testing.T) {
	t.Parallel()
	script := string(webScriptBody)
	declarations := regexp.MustCompile(`var (\w*[wW]ords) = \{`).FindAllStringSubmatchIndex(script, -1)
	if len(declarations) < 4 {
		t.Fatalf("found %d dictionaries of words, want at least 4", len(declarations))
	}
	want := []string{"de", "en", "es", "fr", "it", "pt"}
	for _, at := range declarations {
		name := script[at[2]:at[3]]
		body := balanced(script[at[1]-1:])
		languages := properties(body)
		if got := slices.Sorted(maps.Keys(languages)); !slices.Equal(got, want) {
			t.Errorf("%s has the languages %v, want %v", name, got, want)
			continue
		}
		english := languages["en"]
		for language, value := range languages {
			if strings.HasPrefix(english, "{") != strings.HasPrefix(value, "{") {
				t.Errorf("%s.%s is not shaped as the English", name, language)
				continue
			}
			if !strings.HasPrefix(english, "{") {
				if strings.Trim(value, `'" `) == "" {
					t.Errorf("%s.%s is empty", name, language)
				}
				continue
			}
			wantKeys := slices.Sorted(maps.Keys(properties(english)))
			words := properties(value)
			if got := slices.Sorted(maps.Keys(words)); !slices.Equal(got, wantKeys) {
				t.Errorf("%s.%s has %v, English %v", name, language, got, wantKeys)
			}
			for key, word := range words {
				if strings.Trim(word, `'" `) == "" {
					t.Errorf("%s.%s.%s is empty", name, language, key)
				}
			}
		}
	}
}

// balanced is the object literal text starts with, braces included.
func balanced(text string) string {
	depth, quote := 0, byte(0)
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return text[:i+1]
			}
		}
	}
	return text
}

// properties are the properties of an object literal, by name, each with
// the text of its value.
func properties(object string) map[string]string {
	result := map[string]string{}
	inner := strings.TrimSpace(object[1 : len(object)-1])
	depth, quote, start := 0, byte(0), 0
	add := func(part string) {
		name, value, ok := strings.Cut(part, ":")
		if ok {
			result[strings.TrimSpace(name)] = strings.TrimSpace(value)
		}
	}
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			depth--
		case c == ',' && depth == 0:
			add(inner[start:i])
			start = i + 1
		}
	}
	if rest := strings.TrimSpace(inner[start:]); rest != "" {
		add(rest)
	}
	return result
}
