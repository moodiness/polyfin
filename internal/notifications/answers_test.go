package notifications

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// toFake sends every request to the fake targets, whatever its host, as
// Discord's webhooks would be reached.
type toFake struct{ fake *url.URL }

func (f toFake) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme, r.URL.Host, r.Host = f.fake.Scheme, f.fake.Host, f.fake.Host
	return http.DefaultTransport.RoundTrip(r)
}

// discordAnswer answers as Discord does: 204 to a message with its own
// fields, 400 with its error to anything else, such as Polyfin's event.
func discordAnswer(got map[string]any) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		if embeds, _ := got["embeds"].([]any); len(embeds) > 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "{\"message\": \"Cannot send an empty message\", \"code\": 50006}")
	}
}

// A Webhook target whose address is a Discord webhook's is sent Discord's
// message, which Discord takes, as a Discord target is; other webhooks
// still get Polyfin's event.
func TestAWebhookTargetAtADiscordAddressIsSentDiscordsMessage(t *testing.T) {
	h := newHarness(t)
	fake, _ := url.Parse(h.targets.url)
	h.trusted = &http.Client{Transport: toFake{fake}}
	h.confined = h.trusted
	const path = "/api/webhooks/1234567890/discord-Token_abcdefghijklmnop"
	// Discord's answer depends on what it is sent: the fake reads it.
	h.targets.always(path, func(w http.ResponseWriter) {
		got := h.targets.at(path)
		discordAnswer(got[len(got)-1].body)(w)
	})
	for _, address := range []string{"https://discord.com" + path, "https://canary.discordapp.com" + path} {
		target := h.add(t, nil, Draft{Kind: Webhook, Name: new("Pasted " + address[8:15]), Address: new(address), Events: []string{RecordingFinished}})
		result, err := h.Test(t.Context(), nil, target.ID)
		if err != nil || !result.Delivered || result.Answer != "" {
			t.Errorf("%s: %+v, %v", address, result, err)
		}
	}
	sent := h.targets.at(path)
	if len(sent) != 2 || sent[0].header.Get("X-Polyfin-Event") != "" || sent[0].body["version"] != nil {
		t.Errorf("sent %+v", sent)
	}
	// Elsewhere, a webhook is sent the event.
	other := h.add(t, nil, Draft{Kind: Webhook, Name: new("Other"), Address: new("https://hooks.example.org/api/webhooks/1"), Events: []string{RecordingFinished}})
	if result, err := h.Test(t.Context(), nil, other.ID); err != nil || !result.Delivered {
		t.Fatalf("other: %+v, %v", result, err)
	}
	if got := h.targets.at("/api/webhooks/1"); len(got) != 1 || got[0].body["version"] != 1.0 || got[0].header.Get("X-Polyfin-Event") == "" {
		t.Errorf("other webhook: %+v", got)
	}
}

// A target that refuses a message tells why in the start of its answer,
// kept as the test's result and in the log, never with its address or
// tokens.
func TestARefusalKeepsTheStartOfTheAnswer(t *testing.T) {
	h := newHarness(t)
	const path = "/hooks/refusing/Secret-Token-0123456789abcdef"
	h.targets.always(path, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		// An answer naming the address, as some do, and long.
		_, _ = io.WriteString(w, "{\n  \"message\": \"Cannot send an empty message\",\n  \"code\": 50006,\n  \"url\": \""+h.targets.url+path+
			"\",\n  \"detail\": \""+strings.Repeat("x", 400)+"\"\n}")
	})
	target := h.add(t, nil, h.webhook(path, RecordingFinished))
	result, err := h.Test(t.Context(), nil, target.ID)
	if err != nil || result.Delivered || result.Status != http.StatusBadRequest {
		t.Fatalf("test: %+v, %v", result, err)
	}
	host := strings.TrimPrefix(h.targets.url, "http://")
	if !strings.HasPrefix(result.Answer, `{"message":"Cannot send an empty message","code":50006,`) || len([]rune(result.Answer)) > maxExcerpt ||
		strings.Contains(result.Answer, "Secret-Token") || strings.Contains(result.Answer, host) {
		t.Errorf("answer: %q", result.Answer)
	}
	logged := h.log.String()
	if !strings.Contains(logged, "Cannot send an empty message") || strings.Contains(logged, "Secret-Token") {
		t.Errorf("log: %s", logged)
	}
}

func TestDiscordWebhook(t *testing.T) {
	for address, want := range map[string]bool{
		"https://discord.com/api/webhooks/1/token":            true,
		"https://discordapp.com/api/webhooks/1/token":         true,
		"https://ptb.discord.com/api/webhooks/1/token":        true,
		"https://canary.discord.com/api/webhooks/1/token":     true,
		"https://Discord.com/api/webhooks/1/token":            true,
		" https://discord.com/api/webhooks/1/token ":          true,
		"http://discord.com/api/webhooks/1/token":             false,
		"https://discord.com/channels/1/2":                    false,
		"https://evil.example/api/webhooks/1/token":           false,
		"https://discord.com.evil.example/api/webhooks/1/tok": false,
		"https://notdiscord.com/api/webhooks/1/token":         false,
	} {
		if got := DiscordWebhook(address); got != want {
			t.Errorf("%q: %t", address, got)
		}
	}
}

// leaked lists the pieces of secret, 6 characters or more, that text
// still holds.
func leaked(text, secret string) []string {
	var found []string
	for i := 0; i+6 <= len(secret); i++ {
		if piece := secret[i : i+6]; strings.Contains(text, piece) {
			found = append(found, piece)
		}
	}
	return found
}

// An answer echoing the request leaves nothing of a target's secret: not
// a piece of a Discord webhook's token split on - or _, nor of an access
// token, whatever its characters.
func TestARefusalKeepsNothingOfTheSecrets(t *testing.T) {
	h := newHarness(t)
	fake, _ := url.Parse(h.targets.url)
	h.trusted = &http.Client{Transport: toFake{fake}}
	h.confined = h.trusted
	const webhookToken = "refuse_aB3dEf9GhIjK1mNoPqR-sTuVwXyZ0123456789_abcdEFGH-ijklMNOP-qrstUV_wx"
	const webhookPath = "/api/webhooks/112233445566778899/" + webhookToken
	const accessToken = "refuse-SecretTok3n-XYZ"
	// Each answers 400, repeating its path and its authorization.
	echo := func(path string) {
		h.targets.always(path, func(w http.ResponseWriter) {
			got := h.targets.at(path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"Cannot send an empty message","path":"`+path+`","authorization":"`+got[len(got)-1].header.Get("Authorization")+`","code":50006}`)
		})
	}
	echo(webhookPath)
	echo("/ntfy-refuse/")
	webhook := h.add(t, nil, Draft{Kind: Webhook, Name: new("Hook"), Address: new("https://discord.com" + webhookPath), Events: []string{RecordingFinished}})
	ntfy := h.add(t, nil, Draft{Kind: Ntfy, Name: new("Phone"), Address: new(h.targets.url + "/ntfy-refuse"), Topic: new("polyfin-alerts"),
		Token: new(accessToken), Events: []string{RecordingFinished}})
	for _, c := range []struct {
		target  Target
		secrets []string
	}{
		{webhook, []string{webhookToken, "112233445566778899"}},
		{ntfy, []string{accessToken}},
	} {
		result, err := h.Test(t.Context(), nil, c.target.ID)
		if err != nil || result.Delivered || !strings.Contains(result.Answer, `"message":"Cannot send an empty message"`) {
			t.Fatalf("%s: %+v, %v", c.target.Name, result, err)
		}
		for _, secret := range c.secrets {
			if pieces := leaked(result.Answer, secret); len(pieces) > 0 {
				t.Errorf("%s: the answer %q keeps %q", c.target.Name, result.Answer, pieces)
			}
			if pieces := leaked(h.log.String(), secret); len(pieces) > 0 {
				t.Errorf("%s: the log keeps %q", c.target.Name, pieces)
			}
		}
	}
}

// Every kind's secrets are kept out of an answer that repeats them whole
// or in part: a Telegram bot token, Pushover's user key and token, a
// Gotify token, an address's path and query.
func TestExcerptKeepsNothingOfAnyKindsSecrets(t *testing.T) {
	for _, secrets := range [][]string{
		// A made-up bot token, joined at run time so that no literal has
		// the shape of a real one for secret scanners.
		{strings.Join([]string{"123456789", "AAH-x_SecretPart0123456789_abc-DEF"}, ":")},
		{"uQiRzpo4DXghDmr9QzzfQu27cmVRsG\nazGDORePK8gMaC0QOYAMyEEuzJnyUi"},
		{"Ab-C_dEfGh-IjK"},
		{"https://hooks.example.org/in/Abc123_def-456?key=s3cr_et-Value&x=1"},
	} {
		answer := []byte(`{"error":"no","echo":"` + strings.ReplaceAll(strings.Join(secrets, " "), "\n", " ") + `",` +
			`"parts":"` + strings.ReplaceAll(strings.ReplaceAll(secrets[0], "_", " "), "-", " ") + `"}`)
		got := excerpt(answer, secrets...)
		for _, secret := range secrets {
			for line := range strings.SplitSeq(secret, "\n") {
				if pieces := leaked(got, line); len(pieces) > 0 {
					t.Errorf("%q keeps %q", got, pieces)
				}
			}
		}
		if !strings.HasPrefix(got, `{"error":"no",`) {
			t.Errorf("the answer is lost: %q", got)
		}
	}
}
