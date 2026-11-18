package stremio

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrPublicNetwork reports a request to an address on the internet from
// the client of the sources on a local network, which reaches local
// addresses only.
var ErrPublicNetwork = errors.New("addresses on the internet are not reached for a source on a local network")

// publicOnly is the rule of the confined client for the address it
// connects to, and localOnly that of the local one: nil for an address
// the client may reach, else the error it refuses it with.
func publicOnly(ip netip.Addr) error {
	if !Public(ip) {
		return ErrPrivateNetwork
	}
	return nil
}

func localOnly(ip netip.Addr) error {
	if Public(ip) {
		return ErrPublicNetwork
	}
	return nil
}

// newLocalClient returns the HTTP client of the requests made to a source
// on a local network itself: it reaches local addresses only, those
// Public refuses, checked when connecting, so that neither a DNS answer
// nor a redirect leads a source's own requests from the local network to
// the internet. Its transport is its own: it shares no connection with
// the confined client, nor with the one that reaches any address.
func newLocalClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialer.Control = func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return ErrPublicNetwork
		}
		return localOnly(ip)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = idleConnsPerHost
	// A proxy would be the address checked instead of the source.
	transport.Proxy = nil
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

// sourceKey marks, in a request's context, a request made to a source
// itself rather than to an address its answers name; locatingKey, a read
// that tells where a source is (see Locating).
type (
	sourceKey   struct{}
	locatingKey struct{}
)

// toSource marks ctx as that of a request to a source itself: its
// manifest and resources, a list, an account's API. Such a request is
// confined to public addresses, or else reaches local addresses only: a
// source is asked where it is, never anywhere (see Client.do).
func toSource(ctx context.Context) context.Context {
	return context.WithValue(ctx, sourceKey{}, true)
}

func isToSource(ctx context.Context) bool {
	marked, _ := ctx.Value(sourceKey{}).(bool)
	return marked
}

// Locating marks ctx as that of a read made to tell where a source is: it
// is tried on the local network, then on the internet, and the client of
// the network the source is not on refuses it at once. That refusal says
// nothing of the source: Health does not count it.
func Locating(ctx context.Context) context.Context {
	return context.WithValue(ctx, locatingKey{}, true)
}

// offNetwork reports whether err is the refusal, for a read marked by
// Locating, of the client of the network the source is not on.
func offNetwork(ctx context.Context, err error) bool {
	marked, _ := ctx.Value(locatingKey{}).(bool)
	return marked && (errors.Is(err, ErrPublicNetwork) || errors.Is(err, ErrPrivateNetwork))
}

// OpenSource is Open for a request to a source itself that is no Stremio
// resource, as an IPTV list, an account's API or a tuner, rather than to
// an address its answers name: confined, it reaches public addresses
// only, as Open does; otherwise local addresses only, the source being
// on a local network then.
func (c *Client) OpenSource(ctx context.Context, method, target string, header http.Header, confined bool) (*http.Response, error) {
	return c.Open(toSource(ctx), method, target, header, confined)
}

// Dial opens a connection, as (*net.Dialer).DialContext does.
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// WrapDials replaces, before the client is used, how each of its HTTP
// clients opens its connections, by what wrap makes of its dial, and has
// none go through a proxy. allowed is the client's rule for an address,
// which its own dial applies to the address it connects to: nil for one
// it may reach, else the error it refuses it with. Tests stand for the
// hosts of other networks and for a resolver with it (see package
// reachtest); the server never calls it.
func (c *Client) WrapDials(wrap func(allowed func(netip.Addr) error, dial Dial) Dial) {
	for _, client := range []struct {
		http    *http.Client
		allowed func(netip.Addr) error
	}{
		{c.trusted, func(netip.Addr) error { return nil }},
		{c.confined, publicOnly},
		{c.local, localOnly},
	} {
		transport := client.http.Transport.(*http.Transport)
		transport.Proxy = nil
		transport.DialContext = wrap(client.allowed, transport.DialContext)
	}
}
