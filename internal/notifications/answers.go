package notifications

import (
	"bytes"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// discordHosts are the hosts of Discord's webhooks: its own, its former
// one, and their test builds'.
var discordHosts = []string{"discord.com", "discordapp.com", "ptb.discord.com", "canary.discord.com", "ptb.discordapp.com",
	"canary.discordapp.com"}

// DiscordWebhook reports whether address is a Discord webhook's: https,
// on one of Discord's hosts, under /api/webhooks/. A Webhook target at
// such an address is sent Discord's message, which Discord alone takes.
func DiscordWebhook(address string) bool {
	parsed, err := url.Parse(strings.TrimSpace(address))
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") {
		return false
	}
	return slices.Contains(discordHosts, strings.ToLower(parsed.Hostname())) && strings.HasPrefix(parsed.EscapedPath(), "/api/webhooks/")
}

// maxExcerpt bounds the excerpt of an answer kept as the error of a
// message a target did not accept, in characters.
const maxExcerpt = 200

// excerpt is the start of a target's answer, the error of a message it did
// not accept: on one line, compacted when it is JSON, at most maxExcerpt
// characters, with hidden, the target's address and tokens, blanked out of
// it, and the parts of them that are long or hold digits, as tokens and
// identifiers do.
func excerpt(body []byte, hidden ...string) string {
	var compact bytes.Buffer
	if json.Compact(&compact, body) == nil {
		body = compact.Bytes()
	}
	text := strings.Join(strings.Fields(strings.ToValidUTF8(string(body), "\uFFFD")), " ")
	for _, secret := range hidden {
		for _, part := range secretParts(secret) {
			text = strings.ReplaceAll(text, part, "…")
		}
	}
	if utf8.RuneCountInString(text) > maxExcerpt {
		text = string([]rune(text)[:maxExcerpt-1]) + "…"
	}
	return text
}

// secretParts are what of secret an answer must not repeat: secret
// itself, its host when it is an address, and its parts between other
// characters than letters and digits that are 16 characters or more, or 6
// or more with a digit.
func secretParts(secret string) []string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	parts := []string{secret}
	if parsed, err := url.Parse(secret); err == nil && parsed.Hostname() != "" {
		parts = append(parts, parsed.Hostname())
	}
	for _, field := range strings.FieldsFunc(secret, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(field) >= 16 || len(field) >= 6 && strings.ContainsFunc(field, unicode.IsDigit) {
			parts = append(parts, field)
		}
	}
	// The longest first, so that a part never leaves pieces of a longer one.
	slices.SortFunc(parts, func(a, b string) int { return len(b) - len(a) })
	return parts
}
