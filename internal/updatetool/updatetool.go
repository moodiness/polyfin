// Package updatetool asks an update tool the administrator runs beside
// Polyfin to update now, through the tool's HTTP API: Update now, in the
// admin app, for Polyfin in a container, which cannot pull its own image
// without the Docker socket. The request is a POST to the address the
// settings give, path and query included, with no body, and the token in
// an Authorization: Bearer header when the settings give one. What the
// tool answers is shown as it is: Polyfin knows no tool's API beyond that.
package updatetool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// ConnectTimeout bounds the connection to the tool, and AnswerTimeout
	// the whole request, until its answer has been read: a tool may answer
	// only once it has pulled the new images.
	ConnectTimeout = 10 * time.Second
	AnswerTimeout  = 50 * time.Second
	// MaxAnswer is how much of the answer is kept, in bytes.
	MaxAnswer = 4 << 10
)

// ErrTimeout is a tool that did not answer within the client's timeout.
var ErrTimeout = errors.New("the update tool did not answer in time")

// Answer is what the tool answered: its status and the start of its body.
type Answer struct {
	Status int
	Body   string
}

// Client sends the requests.
type Client struct {
	// UserAgent names Polyfin and its version.
	UserAgent string
	// Timeout bounds a request until its answer has been read; 0 is
	// AnswerTimeout.
	Timeout time.Duration
	http    *http.Client
}

// New returns a client sending as userAgent.
func New(userAgent string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = ConnectTimeout
	return &Client{UserAgent: userAgent, http: &http.Client{
		Transport: transport,
		// A redirect is the tool's answer: following it would change the
		// method, and send the request where the settings do not say.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Ask asks the tool at address to update now, with token as a bearer
// token unless it is empty, and returns its answer, whatever its status.
// An error is a request that got no answer: ErrTimeout when the tool did
// not answer in time.
func (c *Client) Ask(ctx context.Context, address, token string) (Answer, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = AnswerTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, http.NoBody)
	if err != nil {
		return Answer{}, err
	}
	request.Header.Set("User-Agent", c.UserAgent)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return Answer{}, failure(ctx, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxAnswer))
	if err != nil {
		return Answer{}, failure(ctx, err)
	}
	return Answer{Status: response.StatusCode, Body: text(body)}, nil
}

// failure tells a request that ran out of time from one that failed
// otherwise, without the address, which may hold a secret in its query.
func failure(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ErrTimeout
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return fmt.Errorf("the update tool could not be reached: %w", err)
}

// text is the answer's body as text: trimmed, with what is not valid UTF-8
// replaced.
func text(body []byte) string {
	return strings.TrimSpace(strings.ToValidUTF8(string(body), string(utf8.RuneError)))
}
