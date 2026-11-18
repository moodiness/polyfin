// Package redirect holds the rule by which Polyfin's HTTP clients follow
// redirects when the addresses they request hold a key, a token or an
// account: a provider's stream and list addresses, an addon's configured
// path, a service's key in a query. Go's client tells each next host the
// address it comes from, whole, in Referer; these clients do not.
package redirect

import (
	"errors"
	"net/http"
	"slices"
)

// Default is the limit of Go's client when a client has no rule of its
// own: the bound of the clients that had none before.
const Default = 10

// ErrTooMany reports a request redirected more times than its client
// follows.
var ErrTooMany = errors.New("too many redirects")

// Follow returns a client's CheckRedirect that sends limit requests at
// most for one, the first included, as Go's client counts its own ten,
// and sends, to every address a redirect names, on the same host or
// another, only the Referer the caller set on the first request, as the
// caller set it, and none when the caller set none. The address of the
// previous request, which Go's client would send there, never is.
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
		return nil
	}
}
