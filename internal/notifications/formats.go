package notifications

import (
	"html"
	"time"
)

// Discord bounds an embed's title to 256 characters and its description to
// 4,096, and a footer to 2,048.
const (
	discordTitle       = 256
	discordDescription = 4096
	discordFooterText  = 2048
)

// discordColors are the colors of the embeds, by event type.
var discordColors = map[string]int{
	NewEpisode:        0x3b82f6,
	RecordingFinished: 0x22c55e,
	RecordingFailed:   0xef4444,
	HealthSolved:      0x22c55e,
	UserJoined:        0x14b8a6,
	Test:              0x8b5cf6,
}

// discordWebhook is what a Discord webhook takes: one embed, and no
// mention, whatever the names in it.
type discordWebhook struct {
	Username        string          `json:"username"`
	Embeds          []discordEmbed  `json:"embeds"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type discordEmbed struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url,omitempty"`
	Color       int           `json:"color"`
	Timestamp   time.Time     `json:"timestamp"`
	Footer      discordFooter `json:"footer"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

// discordMessage formats ev for a Discord webhook: an embed titled as the
// event, linking to what it is about, colored by its type, with the server
// name in its footer.
func discordMessage(ev Event) discordWebhook {
	color, ok := discordColors[ev.Type]
	if !ok {
		// Health problems: red for errors, amber for warnings.
		color = 0xf59e0b
		if ev.Problem != nil && ev.Problem.Severity == SeverityError {
			color = 0xef4444
		}
	}
	embed := discordEmbed{Title: clip(ev.Title, discordTitle), Description: clip(ev.Message, discordDescription), Color: color,
		Timestamp: ev.At, Footer: discordFooter{Text: clip(ev.Server.Name, discordFooterText)}}
	if ev.URL != nil {
		embed.URL = *ev.URL
	}
	return discordWebhook{Username: "Polyfin", Embeds: []discordEmbed{embed}, AllowedMentions: allowedMentions{Parse: []string{}}}
}

// ntfyPublish is a message published to ntfy as JSON, to its server's
// root address.
type ntfyPublish struct {
	Topic    string   `json:"topic"`
	Title    string   `json:"title"`
	Message  string   `json:"message"`
	Tags     []string `json:"tags"`
	Priority int      `json:"priority,omitempty"`
	Click    string   `json:"click,omitempty"`
}

// ntfyTags are the tags of the messages, by event type: ntfy shows those
// naming an emoji as the emoji.
var ntfyTags = map[string][]string{
	NewEpisode:        {"tv"},
	RecordingFinished: {"red_circle"},
	RecordingFailed:   {"x"},
	HealthProblem:     {"warning"},
	HealthSolved:      {"white_check_mark"},
	UserJoined:        {"wave"},
	Test:              {"bell"},
}

// ntfyMessage formats ev for topic: its title, its message, tags by its
// type, a high priority for urgent events, and its link as the address a
// click on it opens.
func ntfyMessage(topic string, ev Event) ntfyPublish {
	message := ntfyPublish{Topic: topic, Title: ev.Title, Message: ev.Message, Tags: append([]string{}, ntfyTags[ev.Type]...)}
	if ev.Recording != nil && ev.Recording.Partial && ev.Type == RecordingFinished {
		message.Tags = append(message.Tags, "warning")
	}
	if urgent(ev) {
		message.Priority = 4
	}
	if ev.URL != nil {
		message.Click = *ev.URL
	}
	return message
}

// urgent reports whether ev tells of a failure: a failed recording, or a
// health problem that is an error.
func urgent(ev Event) bool {
	return ev.Type == RecordingFailed || ev.Type == HealthProblem && ev.Problem != nil && ev.Problem.Severity == SeverityError
}

// Telegram bounds a message's text to 4,096 characters once its markup is
// read: the event's message is clipped well under it.
const telegramText = 3500

// telegramSend is what the Bot API's sendMessage takes.
type telegramSend struct {
	ChatID             string              `json:"chat_id"`
	Text               string              `json:"text"`
	ParseMode          string              `json:"parse_mode"`
	LinkPreviewOptions telegramLinkPreview `json:"link_preview_options"`
}

type telegramLinkPreview struct {
	IsDisabled bool `json:"is_disabled"`
}

// telegramMessage formats ev for chat: its title in bold, its message, and
// its link, named open, in HTML, without a preview of the page it opens.
func telegramMessage(chat string, ev Event, open string) telegramSend {
	text := "<b>" + html.EscapeString(clip(ev.Title, 256)) + "</b>\n" + html.EscapeString(clip(ev.Message, telegramText))
	if ev.URL != nil {
		text += "\n\n<a href=\"" + html.EscapeString(*ev.URL) + "\">" + html.EscapeString(open) + "</a>"
	}
	return telegramSend{ChatID: chat, Text: text, ParseMode: "HTML", LinkPreviewOptions: telegramLinkPreview{IsDisabled: true}}
}

// gotifyPost is a message posted to a Gotify server's /message: the
// address a click on it opens is in its extras.
type gotifyPost struct {
	Title    string         `json:"title"`
	Message  string         `json:"message"`
	Priority int            `json:"priority"`
	Extras   map[string]any `json:"extras,omitempty"`
}

// gotifyMessage formats ev for Gotify: its title, its message, a high
// priority (8) for urgent events, a normal one (5) for the others, and its
// link as the address a click on it opens.
func gotifyMessage(ev Event) gotifyPost {
	message := gotifyPost{Title: ev.Title, Message: ev.Message, Priority: 5}
	if urgent(ev) {
		message.Priority = 8
	}
	if ev.URL != nil {
		message.Extras = map[string]any{"client::notification": map[string]any{"click": map[string]string{"url": *ev.URL}}}
	}
	return message
}

// Pushover bounds a title to 250 characters, a message to 1,024 and an
// address to 512.
const (
	pushoverTitle   = 250
	pushoverText    = 1024
	pushoverAddress = 512
)

// pushoverPost is a message posted to Pushover's messages.json.
type pushoverPost struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	URL       string `json:"url,omitempty"`
	Priority  int    `json:"priority"`
	Timestamp int64  `json:"timestamp"`
}

// pushoverMessage formats ev for the Pushover user, through the
// application token: its title, its message, its link, unless too long,
// a high priority (1) for urgent events, a normal one (0) for the others,
// and when it happened.
func pushoverMessage(user, token string, ev Event) pushoverPost {
	message := pushoverPost{Token: token, User: user, Title: clip(ev.Title, pushoverTitle), Message: clip(ev.Message, pushoverText),
		Timestamp: ev.At.Unix()}
	if urgent(ev) {
		message.Priority = 1
	}
	if ev.URL != nil && len(*ev.URL) <= pushoverAddress {
		message.URL = *ev.URL
	}
	return message
}

// clip shortens s to at most n characters.
func clip(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
