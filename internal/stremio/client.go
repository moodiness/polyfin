package stremio

import (
	"context"
	"errors"
	"fmt"
	"io"
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
}

// NewClient returns a client identifying itself with the Polyfin version.
func NewClient(version string) *Client {
	return &Client{
		userAgent: "Polyfin/" + version,
		trusted:   newHTTPClient(false),
		confined:  newHTTPClient(true),
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

// get downloads a resource. Errors never contain the URL, which usually
// carries credentials.
func (c *Client) get(ctx context.Context, target string, confined bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL", ErrUnreachable)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.userAgent)
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
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUnreachable, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("%w: response larger than %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	return body, nil
}

// Manifest downloads and validates an addon manifest. confined restricts
// the request to public addresses.
func (c *Client) Manifest(ctx context.Context, manifestURL string, confined bool) (Manifest, error) {
	body, err := c.get(ctx, manifestURL, confined)
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
			return nil, "", ErrPrivateNetwork
		}
		return nil, "", fmt.Errorf("%w: %v", ErrUnreachable, err)
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
