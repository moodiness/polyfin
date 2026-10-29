// Package netproxy reads the proxy Polyfin fetches addons' streams
// through, and finds a WARP proxy running on the same Docker network.
package netproxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrInvalidProxy reports a proxy that is not a URL Parse reads.
var ErrInvalidProxy = errors.New("invalid proxy")

// DefaultCandidates are the names a WARP container usually has on a Docker
// network, tried in this order.
var DefaultCandidates = []string{"warp", "warp-svc", "cloudflare-warp", "host.docker.internal"}

// DefaultPort is the port a WARP container's SOCKS5 proxy listens on.
const DefaultPort = "1080"

// Parse reads a proxy URL: socks5, socks5h, http or https, with a host and
// a port, and no path, query or fragment.
func Parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ErrInvalidProxy
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, ErrInvalidProxy
	}
	switch strings.ToLower(u.Scheme) {
	case "socks5", "socks5h", "http", "https":
	default:
		return nil, ErrInvalidProxy
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return nil, ErrInvalidProxy
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" || strings.ContainsAny(host, " \t\r\n") {
		return nil, ErrInvalidProxy
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, ErrInvalidProxy
	}
	return u, nil
}

// Validate reports whether raw is empty or a proxy URL Parse reads.
func Validate(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	_, err := Parse(raw)
	return err == nil
}

// Detector finds a proxy by trying candidates on a port.
type Detector struct {
	// Candidates are the host names tried in order, DefaultCandidates when
	// nil.
	Candidates []string
	// Port is the port tried on each candidate, DefaultPort when empty.
	Port string
	// Dial opens the connection to a candidate, a TCP connection with a
	// short timeout when nil.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
}

// Detect returns the URL of the first candidate answering on the proxy
// port, and false when none does.
func (d Detector) Detect(ctx context.Context) (string, bool) {
	candidates := d.Candidates
	if candidates == nil {
		candidates = DefaultCandidates
	}
	port := d.Port
	if port == "" {
		port = DefaultPort
	}
	dial := d.Dial
	if dial == nil {
		dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext(ctx, network, address)
		}
	}
	for _, host := range candidates {
		conn, err := dial(ctx, "tcp", net.JoinHostPort(host, port))
		if err != nil {
			continue
		}
		_ = conn.Close()
		return "socks5h://" + net.JoinHostPort(host, port), true
	}
	return "", false
}

// Detect finds a WARP proxy with the default candidates.
func Detect(ctx context.Context) (string, bool) {
	return Detector{}.Detect(ctx)
}
