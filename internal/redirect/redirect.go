// Package redirect holds the rule by which Polyfin's HTTP clients follow
// redirects when the addresses they request hold a key, a token or an
// account: a provider's stream and list addresses, an addon's configured
// path, a service's key in a query. Go's client tells each next host the
// address it comes from, whole, in Referer; these clients do not.
//
// A request also names, in its context, the headers that hold a
// credential (SecretHeaders) and whether its body holds one (SecretBody).
// Go's client sends every header but Authorization and Cookie on to
// another host, and sends a body again on a 307 or 308: once a redirect
// leaves the first host, these clients send none of the headers named,
// and follow no redirect that would send such a body there.
package redirect

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// Default is the limit of Go's client when a client has no rule of its
// own: the bound of the clients that had none before.
const Default = 10

var (
	// ErrTooMany reports a request redirected more times than its client
	// follows.
	ErrTooMany = errors.New("too many redirects")
	// ErrSecretBody reports a redirect to another host that would send
	// again a body SecretBody marks.
	ErrSecretBody = errors.New("a redirect to another host would send the request's body again")
)

// secrets are what a request's context says it holds for its first host.
type secrets struct {
	headers []string
	body    bool
}

type secretsKey struct{}

// SecretHeaders returns ctx for requests whose headers named hold a
// credential of the host they are first sent to: once a redirect leaves
// that host, Follow sends none of them, as Go's client does with
// Authorization and Cookie. The names add to those ctx already holds.
func SecretHeaders(ctx context.Context, names ...string) context.Context {
	s, _ := ctx.Value(secretsKey{}).(secrets)
	s.headers = append(slices.Clip(s.headers), names...)
	return context.WithValue(ctx, secretsKey{}, s)
}

// SecretBody returns ctx for requests whose body holds a credential of the
// host they are first sent to, a password, a token or a client secret:
// Follow refuses, with ErrSecretBody, a 307 or 308 that would send the
// body again to another host. Other redirects send no body and are
// followed.
func SecretBody(ctx context.Context) context.Context {
	s, _ := ctx.Value(secretsKey{}).(secrets)
	s.body = true
	return context.WithValue(ctx, secretsKey{}, s)
}

// Follow returns a client's CheckRedirect that sends limit requests at
// most for one, the first included, as Go's client counts its own ten,
// and sends, to every address a redirect names, on the same host or
// another, only the Referer the caller set on the first request, as the
// caller set it, and none when the caller set none. The address of the
// previous request, which Go's client would send there, never is.
//
// Once a redirect leaves the first request's host, by the rule Go's
// client drops Authorization by, it sends none of the headers
// SecretHeaders names, and refuses to send again a body SecretBody marks.
func Follow(limit int) func(request *http.Request, via []*http.Request) error {
	return func(request *http.Request, via []*http.Request) error {
		if len(via) >= limit {
			return ErrTooMany
		}
		if own, set := via[0].Header["Referer"]; set {
			request.Header["Referer"] = slices.Clone(own)
		} else {
			request.Header.Del("Referer")
		}
		s, marked := via[0].Context().Value(secretsKey{}).(secrets)
		if !marked || !left(request, via) {
			return nil
		}
		// Go's client gives a redirect a body, and GetBody, only when
		// it sends the first request's body again.
		if s.body && request.GetBody != nil && request.ContentLength != 0 {
			return ErrSecretBody
		}
		for _, name := range s.headers {
			request.Header.Del(name)
		}
		return nil
	}
}

// left reports whether a request or one before it, since the first, went
// to another host than the first's. Go's client copies the first
// request's headers to each, and drops Authorization from the first one
// that leaves on, even when a later one comes back.
func left(request *http.Request, via []*http.Request) bool {
	first := via[0].URL
	for _, r := range via[1:] {
		if !within(first, r.URL) {
			return true
		}
	}
	return !within(first, request.URL)
}

// within reports whether next is on first's host or a subdomain of it,
// whatever the port: the rule by which Go's client keeps Authorization
// (shouldCopyHeaderOnRedirect in net/http).
func within(first, next *url.URL) bool {
	parent, err := httpguts.PunycodeHostPort(first.Hostname())
	if err != nil {
		return false
	}
	sub, err := httpguts.PunycodeHostPort(next.Hostname())
	if err != nil {
		return false
	}
	parent, sub = strings.ToLower(parent), strings.ToLower(sub)
	if sub == parent {
		return true
	}
	// An IPv6 address, or one with a zone, is no subdomain.
	if strings.ContainsAny(sub, ":%") {
		return false
	}
	return strings.HasSuffix(sub, "."+parent)
}
