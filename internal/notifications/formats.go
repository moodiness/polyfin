package notifications

import "time"

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
	Test:              {"bell"},
}

// ntfyMessage formats ev for topic: its title, its message, tags by its
// type, a high priority for failures and errors, and its link as the
// address a click on it opens.
func ntfyMessage(topic string, ev Event) ntfyPublish {
	message := ntfyPublish{Topic: topic, Title: ev.Title, Message: ev.Message, Tags: append([]string{}, ntfyTags[ev.Type]...)}
	if ev.Recording != nil && ev.Recording.Partial && ev.Type == RecordingFinished {
		message.Tags = append(message.Tags, "warning")
	}
	if ev.Type == RecordingFailed || ev.Type == HealthProblem && ev.Problem != nil && ev.Problem.Severity == SeverityError {
		message.Priority = 4
	}
	if ev.URL != nil {
		message.Click = *ev.URL
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
