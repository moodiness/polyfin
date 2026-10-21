// Package i18n holds the languages Polyfin speaks, those of the texts it
// writes itself (notifications, task names, System › Health, the names it
// gives seasons and versions, the tray of Polyfin Desktop), and those texts
// in each language.
//
// A text is an i18n.Text literal holding it in each language, in the order
// of Languages, where it is used:
//
//	say(i18n.Text{"%s: the tuner does not answer.", "%s : le tuner ne répond pas.", …}, name)
//
// TestTexts reads every such literal in the module, and fails when one
// misses a language or a language misses a placeholder of the English.
package i18n

import "strings"

// Languages are the languages Polyfin speaks, as BCP 47 tags, English first:
// the default.
var Languages = []string{"en", "fr", "de", "es", "it", "pt-BR"}

// Text is one text in each of Languages, in their order.
type Text [6]string

// In is the text in language, one of Languages, or in English for any
// other.
func (t Text) In(language string) string {
	for i, code := range Languages {
		if code == language {
			return t[i]
		}
	}
	return t[0]
}

// Match is the one of Languages a language tag or locale is written in, ""
// when Polyfin does not speak it: its language, region aside ("de-AT" and
// "de_CH.UTF-8" give "de"), except Portuguese, which Brazilian Portuguese
// covers ("pt", "pt-PT" and "pt_BR" give "pt-BR").
func Match(tag string) string {
	base := strings.ToLower(tag)
	if i := strings.IndexAny(base, "-_.@"); i >= 0 {
		base = base[:i]
	}
	if base == "pt" {
		return "pt-BR"
	}
	for _, code := range Languages {
		if code == base {
			return code
		}
	}
	return ""
}

// Valid reports whether language is one of Languages, as written there.
func Valid(language string) bool {
	for _, code := range Languages {
		if code == language {
			return true
		}
	}
	return false
}
