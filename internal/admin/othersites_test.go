package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
)

// sent sends a GET as b's browser does with the Sec-Fetch headers given,
// site and destination, and returns the status; "" sends none, as an
// older browser and other clients do.
func (b browser) sent(path, site, destination string) int {
	b.api.t.Helper()
	request, _ := http.NewRequestWithContext(b.api.t.Context(), http.MethodGet, b.api.url+path, nil)
	if site != "" {
		request.Header.Set("Sec-Fetch-Site", site)
		request.Header.Set("Sec-Fetch-Dest", destination)
	}
	client := *b.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		b.api.t.Fatal(err)
	}
	response.Body.Close()
	return response.StatusCode
}

// withFlow gives b the cookie of a provider flow whose state is state.
func (b browser) withFlow(state string) {
	served, _ := url.Parse(b.api.url)
	b.client.Jar.SetCookies(served, []*http.Cookie{{Name: providerCookieName, Value: providerAdminApp + "." + state, Path: providerPath}})
}

// What a page of another site has a browser ask for, as an image, and a
// return from the provider to a browser that began no flow, are no
// attempts of the person: after as many as the limit, the right password
// still signs in.
func TestRequestsOfOtherSitesCountAsNoAttempt(t *testing.T) {
	t.Parallel()
	const limit = 3
	for name, ask := range map[string]func(b browser, n int) int{
		"a return from the provider without the flow's cookie": func(b browser, n int) int {
			return b.sent(fmt.Sprintf("%s?state=%d&code=x", providerCallbackPath, n), "", "")
		},
		"a return from the provider as an image": func(b browser, n int) int {
			b.withFlow("another")
			return b.sent(fmt.Sprintf("%s?state=%d&code=x", providerCallbackPath, n), "cross-site", "image")
		},
		"an invite as an image": func(b browser, n int) int {
			return b.sent(fmt.Sprintf("/admin/api/invite/not-an-invite-%d", n), "cross-site", "image")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := newTestAPI(t, limit)
			api.createUser("alice", false)
			b := api.browser()
			for n := range 2 * limit {
				if status := ask(b, n); status != http.StatusSeeOther && status != http.StatusNotFound {
					t.Fatalf("request %d: %d", n+1, status)
				}
			}
			if _, status, body := api.password("alice"); status != http.StatusOK {
				t.Errorf("the right password afterwards: %d %v, want 200", status, body)
			}
		})
	}
}

// The limit still bounds what a person's own browser asks: a return from
// the provider with the flow's cookie and another state, and an invite's
// token that is no invite's, asked by the guest's page, by a link opened
// from another site or by a client that tells nothing.
func TestOwnRequestsStillCountAsAttempts(t *testing.T) {
	t.Parallel()
	const limit = 3
	for name, ask := range map[string]func(b browser, n int) int{
		"a return from the provider with another state": func(b browser, n int) int {
			b.withFlow("another")
			// The provider sends the browser back from its own site.
			return b.sent(fmt.Sprintf("%s?state=%d&code=x", providerCallbackPath, n), "cross-site", "document")
		},
		"an invite asked by the guest's page": func(b browser, n int) int {
			return b.sent(fmt.Sprintf("/admin/api/invite/not-an-invite-%d", n), "same-origin", "empty")
		},
		"an invite opened from another site": func(b browser, n int) int {
			return b.sent(fmt.Sprintf("/admin/api/invite/not-an-invite-%d", n), "cross-site", "document")
		},
		"an invite asked by a client that tells nothing": func(b browser, n int) int {
			return b.sent(fmt.Sprintf("/admin/api/invite/not-an-invite-%d", n), "", "")
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := newTestAPI(t, limit)
			api.createUser("alice", false)
			b := api.browser()
			for n := range limit {
				if status := ask(b, n); status != http.StatusSeeOther && status != http.StatusNotFound {
					t.Fatalf("request %d: %d", n+1, status)
				}
			}
			if _, status, body := api.password("alice"); status != http.StatusTooManyRequests || body["error"] != "too_many_attempts" {
				t.Errorf("the right password afterwards: %d %v, want 429 too_many_attempts", status, body)
			}
		})
	}
}
