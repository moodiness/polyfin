package stremio

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/moodiness/polyfin/internal/metrics"
)

// The failures Health reports, as codes the admin app translates.
const (
	// FailureUnreachable: no answer, the connection failed or broke.
	FailureUnreachable = "unreachable"
	// FailureTimeout: no answer within the time a request is given.
	FailureTimeout = "timeout"
	// FailurePrivateNetwork: the addon is on a local network, which the
	// request was not allowed to reach.
	FailurePrivateNetwork = "private_network"
	// FailureRateLimited: the addon asked to slow down (HTTP 429).
	FailureRateLimited = "rate_limited"
	// FailureServerError: the addon failed (HTTP 5xx).
	FailureServerError = "server_error"
	// FailureHTTPError: the addon refused the request (another HTTP error).
	FailureHTTPError = "http_error"
	// FailureInvalidResponse: the answer was too large to be read.
	FailureInvalidResponse = "invalid_response"
)

// healthLimit bounds the addons whose health is kept: past it, the one
// asked least recently is forgotten.
const healthLimit = 1000

// Health is how the requests Polyfin made to an addon for apps went since
// the server started: nothing is requested to learn it. An answer the
// addon gave, even "not found", is a success.
type Health struct {
	// LastSuccess and LastFailure are when a request last succeeded and
	// failed, zero when none did.
	LastSuccess time.Time
	LastFailure time.Time
	// Failure is the code of the last failure, one of the Failure
	// constants; empty when the last request succeeded.
	Failure string
	// ResponseTime is how long the last answer took to arrive whole.
	ResponseTime time.Duration
	// Requests and Failures count the requests and those that failed.
	Requests int
	Failures int
}

// healthRecords keeps the Health of each addon, by base URL: the address
// is a key only, never shown.
type healthRecords struct {
	mu      sync.Mutex
	byAddon map[string]*Health
}

// record accounts for a request for target to the addon manifestURL
// installs, started at started, which failed with the code failure, or
// succeeded when it is empty, and counts it in the metrics. A request the
// caller gave up on tells nothing of the addon.
func (h *healthRecords) record(ctx context.Context, manifestURL, target string, started time.Time, failure string) {
	if errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	now := time.Now()
	key := BaseURL(manifestURL)
	metrics.AddonRequest(key, resourceOf(manifestURL, target), now.Sub(started), failure == "")
	h.mu.Lock()
	defer h.mu.Unlock()
	health := h.byAddon[key]
	if health == nil {
		if len(h.byAddon) >= healthLimit {
			h.forgetOldest()
		}
		health = &Health{}
		h.byAddon[key] = health
	}
	health.Requests++
	health.Failure = failure
	if failure != "" {
		health.Failures++
		health.LastFailure = now
		return
	}
	health.LastSuccess = now
	health.ResponseTime = now.Sub(started)
}

// resourceOf names the resource target asks of the addon manifestURL
// installs, as the metrics count it: manifest, catalog, meta, stream,
// subtitles, or other, for the addons of other protocols.
func resourceOf(manifestURL, target string) string {
	if target == manifestURL {
		return "manifest"
	}
	rest, ok := strings.CutPrefix(target, BaseURL(manifestURL)+"/")
	resource, _, _ := strings.Cut(rest, "/")
	switch {
	case !ok:
		return "other"
	case resource == "catalog", resource == "meta", resource == "stream", resource == "subtitles":
		return resource
	}
	return "other"
}

// forgetOldest drops the addon asked least recently; h.mu is held.
func (h *healthRecords) forgetOldest() {
	var oldest string
	var at time.Time
	for key, health := range h.byAddon {
		last := health.LastSuccess
		if health.LastFailure.After(last) {
			last = health.LastFailure
		}
		if oldest == "" || last.Before(at) {
			oldest, at = key, last
		}
	}
	delete(h.byAddon, oldest)
}

// Health tells how the requests to the addon manifestURL installs went;
// false when none was made since the server started.
func (c *Client) Health(manifestURL string) (Health, bool) {
	c.health.mu.Lock()
	defer c.health.mu.Unlock()
	health, ok := c.health.byAddon[BaseURL(manifestURL)]
	if !ok {
		return Health{}, false
	}
	return *health, true
}

// failureOf names the failure of a request that got no answer.
func failureOf(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrPrivateNetwork):
		return FailurePrivateNetwork
	case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
		return FailureTimeout
	default:
		return FailureUnreachable
	}
}

// failureOfStatus names the failure an HTTP status tells, none for an
// answer the addon chose to give, such as "not found".
func failureOfStatus(status int) string {
	switch {
	case status == http.StatusTooManyRequests:
		return FailureRateLimited
	case status >= 500:
		return FailureServerError
	case status == http.StatusNotFound:
		return ""
	case status >= 400:
		return FailureHTTPError
	default:
		return ""
	}
}
