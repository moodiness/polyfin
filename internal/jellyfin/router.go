package jellyfin

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/moodiness/polyfin/internal/library"
)

// router matches Jellyfin routes the way Jellyfin does: path segments
// without regard to case, an optional trailing slash, and {name} segments
// captured with their original case as request path values.
type router struct {
	routes []route
	// unmatched logs the requests no route serves; nil logs none.
	unmatched *unmatchedRequests
}

type route struct {
	method   string
	segments []string
	handler  http.Handler
}

func (rt *router) handle(method, pattern string, handler http.Handler) {
	rt.routes = append(rt.routes, route{method: method, segments: split(pattern), handler: handler})
}

func split(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	segments := split(r.URL.Path)
	method := r.Method
	if method == http.MethodHead {
		method = http.MethodGet
	}
	// Like ASP.NET, a literal segment beats a parameter: /Users/Me never
	// reaches /Users/{userId}, whatever the method.
	best, bestLiterals := -1, -1
	var bestValues map[string]string
	pathMatched := false
	for i, candidate := range rt.routes {
		values, literals, ok := candidate.match(segments)
		if !ok {
			continue
		}
		if literals > bestLiterals {
			pathMatched, best, bestLiterals = true, -1, literals
		}
		if literals == bestLiterals && best < 0 && candidate.method == method {
			best, bestValues = i, values
		}
	}
	if best < 0 {
		status := http.StatusNotFound
		if pathMatched {
			status = http.StatusMethodNotAllowed
		}
		rt.unmatched.report(r, status)
		w.WriteHeader(status)
		return
	}
	// A request builds each user's view of the library once, however many
	// items it describes.
	r = r.WithContext(library.PerRequest(r.Context()))
	for name, value := range bestValues {
		r.SetPathValue(name, value)
	}
	rt.routes[best].handler.ServeHTTP(w, r)
}

func (candidate route) match(segments []string) (map[string]string, int, bool) {
	if len(segments) != len(candidate.segments) {
		return nil, 0, false
	}
	var values map[string]string
	literals := 0
	for i, want := range candidate.segments {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			if values == nil {
				values = map[string]string{}
			}
			values[want[1:len(want)-1]] = segments[i]
			continue
		}
		if !strings.EqualFold(want, segments[i]) {
			return nil, 0, false
		}
		literals++
	}
	return values, literals, true
}

// unmatchedRequests tells which endpoints apps call that Polyfin does not
// serve. Each method and path shape is logged at most once an hour, so an
// app polling a missing endpoint does not flood the log.
type unmatchedRequests struct {
	logger *slog.Logger
	now    func() time.Time

	mu     sync.Mutex
	logged map[string]time.Time
}

const (
	unmatchedInterval = time.Hour
	// maxUnmatched bounds the shapes remembered. Once that many were logged
	// within the hour, others go unlogged until some expire: past that
	// many, the requests come from a scanner, not from apps.
	maxUnmatched = 1000
	// maxLogged bounds each value kept and logged. Go accepts request lines
	// and headers of about a megabyte; no endpoint, method or app name is
	// that long, and a thousand such paths would hold a gigabyte.
	maxLogged = 256
)

func newUnmatchedRequests(logger *slog.Logger) *unmatchedRequests {
	return &unmatchedRequests{logger: logger, now: time.Now, logged: map[string]time.Time{}}
}

func (u *unmatchedRequests) report(r *http.Request, status int) {
	if u == nil {
		return
	}
	// The path alone: query strings carry tokens and search terms.
	path := truncate(pathShape(r.URL.Path))
	method := truncate(r.Method)
	// Jellyfin matches paths without regard to case, so apps spelling the
	// same endpoint differently are one shape.
	key := method + " " + strings.ToLower(path)
	now := u.now()
	u.mu.Lock()
	if at, ok := u.logged[key]; ok && now.Sub(at) < unmatchedInterval {
		u.mu.Unlock()
		return
	}
	if len(u.logged) >= maxUnmatched {
		for shape, at := range u.logged {
			if now.Sub(at) >= unmatchedInterval {
				delete(u.logged, shape)
			}
		}
	}
	if len(u.logged) >= maxUnmatched {
		u.mu.Unlock()
		return
	}
	u.logged[key] = now
	u.mu.Unlock()
	app := readCredentials(r, true)
	u.logger.Info("An app asked for something Polyfin does not serve",
		"method", method, "path", path, "status", status, "client", truncate(app.Client), "version", truncate(app.Version))
}

// truncate keeps the first maxLogged bytes of s, without splitting a
// character, marking what it cut.
func truncate(s string) string {
	if len(s) <= maxLogged {
		return s
	}
	end := maxLogged
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "…"
}

// pathShape replaces the identifiers in a path with {id}, so that requests
// for different items are one endpoint, and no item or user shows in the
// log. Identifiers are 32-digit hexadecimal ids, dashed GUIDs and numbers,
// alone or before an extension, as in segment 12.ts.
func pathShape(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		name, extension := segment, ""
		if dot := strings.IndexByte(segment, '.'); dot > 0 {
			name, extension = segment[:dot], segment[dot:]
		}
		if isIdentifier(name) {
			segments[i] = "{id}" + extension
		}
	}
	return strings.Join(segments, "/")
}

func isIdentifier(segment string) bool {
	switch {
	case segment == "":
		return false
	case strings.Trim(segment, "0123456789") == "":
		return true
	case len(segment) == 32:
		return isHex(segment)
	case len(segment) == 36:
		for _, dash := range []int{8, 13, 18, 23} {
			if segment[dash] != '-' {
				return false
			}
		}
		digits := strings.ReplaceAll(segment, "-", "")
		return len(digits) == 32 && isHex(digits)
	}
	return false
}

func isHex(s string) bool {
	return strings.Trim(s, "0123456789abcdefABCDEF") == ""
}

// cors lets browser-based Jellyfin apps served from another origin call the
// API, as Jellyfin allows by default. Credentials travel in the
// Authorization header, never in cookies, so any origin is allowed.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == "" {
			next.ServeHTTP(w, r)
			return
		}
		header := w.Header()
		header.Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			header.Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
			if requested := r.Header.Get("Access-Control-Request-Headers"); requested != "" {
				header.Set("Access-Control-Allow-Headers", requested)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// query returns a query parameter matched without regard to case, as
// ASP.NET binds them.
func query(r *http.Request, name string) string {
	value, _ := queryParam(r, name)
	return value
}
