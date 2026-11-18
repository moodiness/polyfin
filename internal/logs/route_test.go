package logs

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A request is told by the pattern of its route, without the method:
// what the path holds where the pattern has a name is not in it.
func TestRouteIsThePatternOfTheRequestsRoute(t *testing.T) {
	t.Parallel()
	var got string
	told := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = Route(r) })
	mux := http.NewServeMux()
	mux.Handle("GET /admin/api/invite/{token}", told)
	mux.Handle("/share/{token}/play", told)
	mux.Handle("/web/", told)
	for target, want := range map[string]string{
		"/admin/api/invite/INVITE-123?next=x": "/admin/api/invite/{token}",
		"/share/SHARE-456/play":               "/share/{token}/play",
		"/web/index.html":                     "/web/",
	} {
		got = "not served"
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, nil))
		if got != want {
			t.Errorf("Route of %s: %q, want %q", target, got, want)
		}
	}
	// No router has matched this one.
	if got := Route(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/share/SHARE-456/play", nil)); got != "" {
		t.Errorf("Route of a request without a route: %q", got)
	}
}
