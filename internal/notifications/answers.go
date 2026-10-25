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
// characters, with nothing of hidden, the target's address, tokens and
// topic, left in it (see redact).
func excerpt(body []byte, hidden ...string) string {
	var compact bytes.Buffer
	if json.Compact(&compact, body) == nil {
		body = compact.Bytes()
	}
	text := strings.Join(strings.Fields(strings.ToValidUTF8(string(body), "\uFFFD")), " ")
	text = redact(text, secretParts(hidden...))
	if utf8.RuneCountInString(text) > maxExcerpt {
		text = string([]rune(text)[:maxExcerpt-1]) + "…"
	}
	return text
}

// minPart is the length from which a part of a secret is blanked wherever
// it is in an answer, a shorter one only when a run of the answer is that
// part; and minRun the length from which a run of an answer found in a
// part of a secret is blanked.
const (
	minPart = 6
	minRun  = 4
)

// redact blanks out of text every part of the secrets parts lists: each
// part of minPart characters or more wherever it is, and each run of
// token characters (see tokenRune) that is a shorter part, or that is
// found in a part and has minRun characters or more, as a piece of a
// token split on - or _ is.
func redact(text string, parts []string) string {
	short := map[string]bool{}
	for _, part := range parts {
		if utf8.RuneCountInString(part) >= minPart {
			text = strings.ReplaceAll(text, part, "…")
		} else {
			short[part] = true
		}
	}
	var out strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); {
		if !tokenRune(runes[i]) {
			out.WriteRune(runes[i])
			i++
			continue
		}
		end := i
		for end < len(runes) && tokenRune(runes[end]) {
			end++
		}
		run := string(runes[i:end])
		if short[run] || end-i >= minRun && slices.ContainsFunc(parts, func(part string) bool { return strings.Contains(part, run) }) {
			out.WriteString("…")
		} else {
			out.WriteString(run)
		}
		i = end
	}
	return out.String()
}

// tokenRune reports whether r may be in a token, an identifier or an
// address's path: letters, digits and - _ . ~ % + / = :.
func tokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.~%+/=:", r)
}

// secretParts are what of secrets an answer must not repeat, longest
// first: each secret, each of its lines, as Pushover's user key and token;
// for an address, its host, its path, escaped and not, each of its
// segments, its query and each of its values, and its user and password;
// for a token, as Telegram's bot number and key, each part between colons.
func secretParts(secrets ...string) []string {
	var parts []string
	add := func(part string) {
		if part = strings.TrimSpace(part); part != "" && !slices.Contains(parts, part) {
			parts = append(parts, part)
		}
	}
	for _, secret := range secrets {
		lines := append([]string{secret}, strings.Split(secret, "\n")...)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			add(line)
			parsed, err := url.Parse(line)
			if err != nil || parsed.Hostname() == "" {
				for part := range strings.SplitSeq(line, ":") {
					add(part)
				}
				continue
			}
			add(parsed.Hostname())
			for _, path := range []string{parsed.EscapedPath(), parsed.Path} {
				add(strings.Trim(path, "/"))
				for segment := range strings.SplitSeq(path, "/") {
					add(segment)
				}
			}
			add(parsed.RawQuery)
			for _, values := range parsed.Query() {
				for _, value := range values {
					add(value)
				}
			}
			if parsed.User != nil {
				add(parsed.User.Username())
				password, _ := parsed.User.Password()
				add(password)
			}
		}
	}
	// The longest first, so that a part never leaves pieces of a longer one.
	slices.SortFunc(parts, func(a, b string) int { return len(b) - len(a) })
	return parts
}
