package jellyfin

import (
	"net/http"
	"strings"
)

// router matches Jellyfin routes the way Jellyfin does: path segments
// without regard to case, an optional trailing slash, and {name} segments
// captured with their original case as request path values.
type router struct {
	routes []route
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
		if pathMatched {
			w.WriteHeader(http.StatusMethodNotAllowed)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
		return
	}
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
