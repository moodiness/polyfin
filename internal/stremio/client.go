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
	"strings"
	"syscall"
	"time"
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
}

// NewClient returns a client identifying itself with the Polyfin version.
func NewClient(version string) *Client {
	return &Client{
		userAgent: "Polyfin/" + version,
		trusted:   newHTTPClient(false),
		confined:  newHTTPClient(true),
		health:    healthRecords{byAddon: map[string]*Health{}},
	}
}

func newHTTPClient(confined bool) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if confined {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || !public(ip) {
				return ErrPrivateNetwork
			}
			return nil
		}
		// A proxy would be the address checked instead of the addon.
		transport.Proxy = nil
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

func public(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsGlobalUnicast() && !ip.IsPrivate() && !cgnat.Contains(ip)
}

// do sends a request with the client allowed for it. Errors never contain
// the URL, which usually carries credentials.
func (c *Client) do(request *http.Request, confined bool) (*http.Response, error) {
	client := c.trusted
	if confined {
		client = c.confined
	}
	response, err := client.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		if errors.Is(err, ErrPrivateNetwork) {
			return nil, ErrPrivateNetwork
		}
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
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
		c.health.record(ctx, manifestURL, started, failureOf(ctx, err))
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		c.health.record(ctx, manifestURL, started, failureOfStatus(response.StatusCode))
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, httpStatus(response.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		c.health.record(ctx, manifestURL, started, failureOf(ctx, err))
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if len(body) > maxResponseBytes {
		c.health.record(ctx, manifestURL, started, FailureInvalidResponse)
		return nil, fmt.Errorf("%w: response larger than %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	c.health.record(ctx, manifestURL, started, "")
	return body, nil
}

// httpStatus is the status of an answer that is not a success.
type httpStatus int

func (s httpStatus) Error() string { return fmt.Sprintf("HTTP %d", int(s)) }

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
	return c.do(request, confined)
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
		return nil, "", fmt.Errorf("%w: HTTP %d %s", ErrUnreachable, response.StatusCode, contentType)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if len(body) > maxImageBytes {
		return nil, "", fmt.Errorf("%w: image larger than %d bytes", ErrInvalidResponse, maxImageBytes)
	}
	return body, contentType, nil
}
