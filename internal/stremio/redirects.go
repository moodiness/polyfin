package stremio

import "github.com/moodiness/polyfin/internal/redirect"

// followRedirects is the CheckRedirect of the package's clients: they
// send maxRedirects requests at most for one, the first included, and
// tell no host the address a redirect comes from. The addresses of
// addons, providers and notification targets hold keys and accounts, and
// the host an answer redirects to is not the one they were given to. A
// Referer the caller set, as a stream's own headers may, goes to every
// address unchanged.
var followRedirects = redirect.Follow(maxRedirects)
