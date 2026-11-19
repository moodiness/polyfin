package stremio

import "github.com/moodiness/polyfin/internal/redirect"

// followRedirects is the CheckRedirect of the package's clients: they
// send maxRedirects requests at most for one, the first included, and
// tell no host the address a redirect comes from. The addresses of
// addons, providers and notification targets hold keys and accounts, and
// the host an answer redirects to is not the one they were given to. A
// Referer the caller set, as a stream's own headers may, goes to every
// address unchanged. The headers and bodies a request marks secret, as
// notifications and imports do, go to no other host. A stream's own
// headers are not marked: its source names them for the hosts it serves
// the stream from, which its redirects lead to, and Go's client keeps
// Authorization and Cookie among them from another host already.
var followRedirects = redirect.Follow(maxRedirects)
