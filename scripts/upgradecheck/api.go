package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// api calls a server's APIs: the admin app's with an admin session's
// cookie, the Jellyfin API with a device's token.
type api struct {
	base   string
	client *http.Client
}

func newAPI(base string) *api {
	return &api{base: base, client: &http.Client{
		Timeout: 30 * time.Second,
		// Routes are checked for the status they answer, redirects too.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// caller is who calls: an admin session, a Jellyfin device, or nobody.
type caller struct {
	// cookie is the admin session's cookie, name=value.
	cookie string
	// device and token are a Jellyfin device's identifier and access
	// token; a device signing in has no token yet.
	device string
	token  string
}

var nobody caller

// authorization is the Jellyfin header of a device, as apps send it.
func (c caller) authorization() string {
	header := fmt.Sprintf(`MediaBrowser Client="Polyfin upgrade check", Device="upgradecheck", DeviceId=%q, Version="1"`, c.device)
	if c.token != "" {
		header += fmt.Sprintf(", Token=%q", c.token)
	}
	return header
}

// call sends a request with body as JSON, decodes the answer's JSON into
// out when out is not nil and the request succeeded, and returns its
// status. When want lists statuses, any other is an error naming the
// request and the answer.
func (a *api) call(ctx context.Context, who caller, method, path string, body, out any, want ...int) (int, error) {
	answer, status, err := a.send(ctx, who, method, path, body)
	if err != nil {
		return 0, err
	}
	if len(want) > 0 && !slices.Contains(want, status) {
		return status, fmt.Errorf("%s %s answered %d, want %d: %s", method, path, status, want[0], excerpt(answer))
	}
	if out != nil && status < 300 {
		if err := json.Unmarshal(answer, out); err != nil {
			return status, fmt.Errorf("%s %s answered %d with JSON that does not decode: %w: %s", method, path, status, err, excerpt(answer))
		}
	}
	return status, nil
}

// signIn opens an admin session, and returns its caller.
func (a *api) signIn(ctx context.Context, path string, body any) (caller, error) {
	answer, status, cookies, err := a.exchange(ctx, nobody, "POST", path, body)
	if err != nil {
		return nobody, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nobody, fmt.Errorf("POST %s answered %d: %s", path, status, excerpt(answer))
	}
	for _, cookie := range cookies {
		if cookie.Value != "" {
			return caller{cookie: cookie.Name + "=" + cookie.Value}, nil
		}
	}
	return nobody, fmt.Errorf("POST %s set no session cookie", path)
}

// authenticate signs a Jellyfin device in as name.
func (a *api) authenticate(ctx context.Context, device, name, password string) (caller, string, error) {
	who := caller{device: device}
	var answer struct {
		AccessToken string
		User        struct{ Id string }
	}
	if _, err := a.call(ctx, who, "POST", "/Users/AuthenticateByName", map[string]string{"Username": name, "Pw": password},
		&answer, http.StatusOK); err != nil {
		return nobody, "", err
	}
	who.token = answer.AccessToken
	return who, answer.User.Id, nil
}

func (a *api) send(ctx context.Context, who caller, method, path string, body any) ([]byte, int, error) {
	answer, status, _, err := a.exchange(ctx, who, method, path, body)
	return answer, status, err
}

func (a *api) exchange(ctx context.Context, who caller, method, path string, body any) ([]byte, int, []*http.Cookie, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, reader)
	if err != nil {
		return nil, 0, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if who.cookie != "" {
		req.Header.Set("Cookie", who.cookie)
	}
	if who.device != "" {
		req.Header.Set("Authorization", who.authorization())
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	return answer, resp.StatusCode, resp.Cookies(), nil
}

// excerpt is the start of an answer, for an error to quote.
func excerpt(answer []byte) string {
	text := strings.TrimSpace(string(answer))
	if len(text) > 300 {
		text = text[:300] + "…"
	}
	if text == "" {
		return "(empty)"
	}
	return text
}
