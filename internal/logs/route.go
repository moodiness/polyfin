package logs

import (
	"net/http"
	"strings"
)

// Route returns what a log line says of where a request went: the pattern
// of the route that serves it, without its method, as
// /admin/api/invite/{token}. Never the path itself: the paths of invites,
// share links and API keys hold them, and administrators share the log.
// A request that no router has matched gives nothing.
func Route(r *http.Request) string {
	return r.Pattern[strings.IndexByte(r.Pattern, ' ')+1:]
}
