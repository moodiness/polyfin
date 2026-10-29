package stremio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/moodiness/polyfin/internal/netproxy"
)

const (
	requestTimeout = 15 * time.Second
	// Large configurations list hundreds of catalogs with their options.
	maxResponseBytes = 8 << 20
	maxRedirects     = 5
)

var (
	// ErrUnreachable reports an addon that could not be reached or answered
	// with an HTTP error.
	ErrUnreachable = errors.New("addon unreachable")
	// ErrPrivateNetwork reports a request to a local network address from a
	// client that may only reach public addresses.
	ErrPrivateNetwork = errors.New("addresses on local networks are not allowed")
	// ErrInvalidResponse reports a resource response that is not valid JSON
	// of the expected form.
	ErrInvalidResponse = errors.New("invalid addon response")
	// ErrNotFound reports a resource the addon does not have.
	ErrNotFound = errors.New("not found by the addon")
)

// Client fetches addon resources. Requests made on behalf of untrusted users
// are confined to public internet addresses, checked when connecting so that
// DNS answers and redirects cannot point them at the local network.
type Client struct {
	userAgent string
	trusted   *http.Client
	confined  *http.Client
	health    healthRecords

	proxyMu sync.Mutex
	proxies map[string]*http.Client
}

// NewClient returns a client identifying itself with the Polyfin version.
func NewClient(version string) *Client {
	return &Client{
		userAgent: "Polyfin/" + version,
		trusted:   newHTTPClient(false, nil),
		confined:  newHTTPClient(true, nil),
		health:    healthRecords{byAddon: map[string]*Health{}},
		proxies:   map[string]*http.Client{},
	}
}

// idleConnsPerHost is how many idle connections are kept to each host:
// listings ask an addon for up to 16 catalogs or 8 pages at once, and
// home screens a burst of posters from one artwork server, which would
// otherwise open new connections, with TLS handshakes, at each wave.
const idleConnsPerHost = 32

// HTTPClient returns an HTTP client following the rules of addon requests:
// confined, it reaches public addresses only, checked when connecting so
// that DNS answers and redirects cannot point it at the local network.
// Other features sending requests on behalf of users use it.
func HTTPClient(confined bool) *http.Client {
	return newHTTPClient(confined, nil)
}

func newHTTPClient(confined bool, proxy *url.URL) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = idleConnsPerHost
	if confined {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !Public(ip) {
				return ErrPrivateNetwork
			}
			return nil
		}
		// A proxy would be the address checked instead of the addon.
		transport.Proxy = nil
	} else if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	transport.DialContext = dialer.DialContext
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// cgnat is the shared address space of carrier-grade NAT, not reachable
// from the internet.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// Public reports whether ip is an address on the internet, which confined
// requests may reach: not loopback, link-local, private, nor in the shared
// address space of carrier-grade NAT.
func Public(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !cgnat.Contains(ip)
}

// CheckPublic reports ErrPrivateNetwork when host is, or resolves to, an
// address on a local network, which confined requests may not reach. A
// host that does not resolve is not refused: confined requests check the
// address they connect to anyway.
func CheckPublic(ctx context.Context, host string) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !Public(ip) {
			return ErrPrivateNetwork
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if !Public(ip) {
			return ErrPrivateNetwork
		}
	}
	return nil
}

// do sends a request with the client allowed for it. Errors never contain
// the URL, which usually carries credentials.
func (c *Client) do(request *http.Request, confined bool) (*http.Response, error) {
	client := c.trusted
	if confined {
		client = c.confined
	}
	return c.doWith(client, request)
}

// doWith sends request with client. Errors never contain the URL, which
// usually carries credentials.
func (c *Client) doWith(client *http.Client, request *http.Request) (*http.Response, error) {
	response, err := client.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		if errors.Is(err, ErrPrivateNetwork) {
			return nil, ErrPrivateNetwork
		}
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	return response, nil
}

// get downloads a resource of the addon manifestURL installs, and records
// how its answer went (see Health).
func (c *Client) get(ctx context.Context, manifestURL, target string, confined bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL", ErrUnreachable)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
	started := time.Now()
	response, err := c.do(request, confined)
	if err != nil {
		c.health.record(ctx, manifestURL, target, started, failureOf(ctx, err))
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// An addon asks to wait with a 429, or with a 503 that says how
		// long (Retry-After), as IPTV providers do (see iptv.RateLimited).
		wait, waits := parseRetryAfter(response.Header.Get("Retry-After"), time.Now())
		waits = waits && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable)
		failure := failureOfStatus(response.StatusCode)
		if waits {
			failure = FailureRateLimited
		}
		c.health.record(ctx, manifestURL, target, started, failure)
		return nil, answered{status: httpStatus(response.StatusCode), wait: wait, waits: waits}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		c.health.record(ctx, manifestURL, target, started, failureOf(ctx, err))
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(body) > maxResponseBytes {
		c.health.record(ctx, manifestURL, target, started, FailureInvalidResponse)
		return nil, fmt.Errorf("%w: response larger than %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	c.health.record(ctx, manifestURL, target, started, "")
	return body, nil
}

// httpStatus is the status of an answer that is not a success.
type httpStatus int

func (s httpStatus) Error() string { return fmt.Sprintf("HTTP %d", int(s)) }

// answered is the error of an addon's answer that is not a success. It is
// ErrUnreachable to its callers, as no resource came of it, with its status
// for StatusOf and, when the addon asked to wait, the wait for RetryAfter.
// Its text tells what the addon answered, as Health names it, rather than
// calling an addon that answered unreachable.
type answered struct {
	status httpStatus
	wait   retryAfter
	waits  bool
}

func (a answered) Error() string {
	status := int(a.status)
	switch {
	case a.waits:
		return fmt.Sprintf("addon asked to wait: %v, %v", a.status, a.wait)
	case status == http.StatusTooManyRequests:
		return fmt.Sprintf("addon asked to slow down: %v", a.status)
	case status >= 500:
		return fmt.Sprintf("addon failed: %v", a.status)
	case status >= 400 && status != http.StatusNotFound:
		return fmt.Sprintf("addon refused the request: %v", a.status)
	}
	return fmt.Sprintf("addon answered %v", a.status)
}

func (a answered) Unwrap() []error {
	if a.waits {
		return []error{ErrUnreachable, a.status, a.wait}
	}
	return []error{ErrUnreachable, a.status}
}

// StatusOf returns the HTTP status of an answer that was not a success,
// 0 when err is not one.
func StatusOf(err error) int {
	status, _ := errors.AsType[httpStatus](err)
	return int(status)
}

// retryAfter is how long an addon that answered 429, or 503 with
// Retry-After, asked to wait before it is asked again.
type retryAfter time.Duration

func (r retryAfter) Error() string { return "retry after " + time.Duration(r).String() }

// RetryAfter returns how long the addon whose answer err is asked to wait,
// by the Retry-After of its 429 or 503, or as long as is left of a wait
// it asked for earlier (see AskedToWait); false when it did not say.
func RetryAfter(err error) (time.Duration, bool) {
	wait, ok := errors.AsType[retryAfter](err)
	return time.Duration(wait), ok
}

// ErrAskedToWait reports a request that was not sent: the addon asked to
// wait, and the wait has not ended.
var ErrAskedToWait = errors.New("not asked: the addon asked to wait")

// AskedToWait is the error of a request not sent to an addon that asked
// to wait, with left of the wait to go: ErrAskedToWait, and left for
// RetryAfter.
func AskedToWait(left time.Duration) error {
	return fmt.Errorf("%w, %w", ErrAskedToWait, retryAfter(left))
}

// parseRetryAfter reads a Retry-After header, in seconds or as a date, as
// of now: a date past is no wait.
func parseRetryAfter(value string, now time.Time) (retryAfter, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return retryAfter(time.Duration(max(seconds, 0)) * time.Second), true
	}
	if date, err := http.ParseTime(value); err == nil {
		return retryAfter(max(date.Sub(now), 0)), true
	}
	return 0, false
}

// Fetch downloads a resource of an addon of another protocol than
// Stremio's, through the same rules: bounded, confined when asked, its URL
// kept out of errors, its outcome counted in the health of the addon of
// manifestURL. An answer 404 is ErrNotFound.
func (c *Client) Fetch(ctx context.Context, manifestURL, target string, confined bool) ([]byte, error) {
	body, err := c.get(ctx, manifestURL, target, confined)
	if status, ok := errors.AsType[httpStatus](err); ok && status == http.StatusNotFound {
		return nil, fmt.Errorf("%w: HTTP 404", ErrNotFound)
	}
	return body, err
}

// Open requests a file an addon points to, a stream or a subtitle, to relay
// it: header is sent as given (byte ranges, the stream's own headers) and
// the response is returned whatever its status, its body to be closed by
// the caller.
func (c *Client) Open(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	request, err := c.openRequest(ctx, method, target, header)
	if err != nil {
		return nil, err
	}
	return c.do(request, confined)
}

// OpenViaProxy requests a file an addon points to through proxy, a SOCKS5
// or HTTP proxy URL, as Open does. A confined request stays direct: the
// proxy would reach private addresses its confinement refuses.
func (c *Client) OpenViaProxy(ctx context.Context, proxy, method, target string, header http.Header, confined bool) (*http.Response, error) {
	if confined {
		return c.Open(ctx, method, target, header, confined)
	}
	u, err := netproxy.Parse(proxy)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	request, err := c.openRequest(ctx, method, target, header)
	if err != nil {
		return nil, err
	}
	return c.doWith(c.proxyClient(u), request)
}

// openRequest makes a request for a file an addon points to, with its
// headers and Polyfin's User-Agent when the addon names none.
func (c *Client) openRequest(ctx context.Context, method, target string, header http.Header) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil || (request.URL.Scheme != "https" && request.URL.Scheme != "http") {
		return nil, fmt.Errorf("%w: invalid URL", ErrUnreachable)
	}
	maps.Copy(request.Header, header)
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", c.userAgent)
	}
	// Byte ranges and lengths must reach the player unchanged.
	request.Header.Set("Accept-Encoding", "identity")
	return request, nil
}

// proxyClient returns the client reading files through proxy, made the
// first time and kept.
func (c *Client) proxyClient(proxy *url.URL) *http.Client {
	key := proxy.String()
	c.proxyMu.Lock()
	defer c.proxyMu.Unlock()
	if client := c.proxies[key]; client != nil {
		return client
	}
	client := newHTTPClient(false, proxy)
	c.proxies[key] = client
	return client
}

// Manifest downloads and validates an addon manifest. confined restricts
// the request to public addresses.
func (c *Client) Manifest(ctx context.Context, manifestURL string, confined bool) (Manifest, error) {
	body, err := c.get(ctx, manifestURL, manifestURL, confined)
	if errors.Is(err, ErrInvalidResponse) {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err != nil {
		return Manifest{}, err
	}
	return ParseManifest(body)
}

// maxImageBytes bounds artwork downloads.
const maxImageBytes = 15 << 20

// Image downloads artwork referenced by an addon. Only image responses are
// accepted, so an addon cannot make the server relay arbitrary content.
func (c *Client) Image(ctx context.Context, target string, confined bool) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil || (request.URL.Scheme != "https" && request.URL.Scheme != "http") {
		return nil, "", fmt.Errorf("%w: invalid image URL", ErrUnreachable)
	}
	request.Header.Set("Accept", "image/*")
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.do(request, confined)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	contentType := response.Header.Get("Content-Type")
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("%w: %w %s", ErrUnreachable, httpStatus(response.StatusCode), contentType)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(body) > maxImageBytes {
		return nil, "", fmt.Errorf("%w: image larger than %d bytes", ErrInvalidResponse, maxImageBytes)
	}
	return body, contentType, nil
}
