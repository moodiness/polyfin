package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/jellyfin"
	"github.com/moodiness/polyfin/internal/throttle"
)

// Wrong passwords and PINs that a page of another site had the browser
// send to the Jellyfin API, as many as the limit the API shares with the
// admin app, leave the admin app's sign-in from the same address as it
// was, as in main.
func TestOtherSitesFailedSignInsLeaveTheAdminAppsSignIn(t *testing.T) {
	t.Parallel()
	const limit = 3
	var api *httptest.Server
	admin := newTestAPI(t, limit, func(o *Options, _ testDeps) {
		api = httptest.NewServer(jellyfin.New(jellyfin.Options{Accounts: o.Accounts, SignIns: o.SignIns,
			CrossSiteSignIns: throttle.New(limit, time.Minute), Activity: o.Activity, Logger: o.Logger}))
		t.Cleanup(api.Close)
	})
	if _, err := admin.store.CreateUser(t.Context(), accounts.NewUser{Name: "root", Password: "correct horse", IsAdministrator: true}); err != nil {
		t.Fatal(err)
	}
	fromAnotherSite := func(path, body string) int {
		t.Helper()
		request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, api.URL+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", `MediaBrowser Client="Page", Device="browser", DeviceId="browser-id", Version="1.0.0"`)
		request.Header.Set("Origin", "https://elsewhere.example")
		request.Header.Set("Sec-Fetch-Site", "cross-site")
		response, err := testTransport.RoundTrip(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		return response.StatusCode
	}
	for i := range limit - 1 {
		if status := fromAnotherSite("/Users/AuthenticateByName", `{"Username":"root","Pw":"wrong horse"}`); status != http.StatusUnauthorized {
			t.Fatalf("wrong password %d from another site: %d, want 401", i+1, status)
		}
	}
	if status := fromAnotherSite("/Users/ForgotPassword/Pin", `{"Pin":"00000000"}`); status != http.StatusNotFound {
		t.Fatalf("wrong PIN from another site: %d, want 404", status)
	}
	if status := fromAnotherSite("/Users/AuthenticateByName", `{"Username":"root","Pw":"correct horse"}`); status != http.StatusTooManyRequests {
		t.Errorf("the right password from another site past its limit: %d, want 429", status)
	}
	b := admin.browser()
	if status, body, _ := b.call(http.MethodPost, "/session", map[string]string{"name": "root", "password": "correct horse"}); status != http.StatusOK {
		t.Errorf("the admin app's sign-in after %d failures from another site: %d %v, want 200", limit, status, body)
	}
}
