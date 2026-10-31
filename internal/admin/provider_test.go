package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/oidc/oidctest"
)

// withProvider sets a provider up for the server, its public address the
// test server's, and returns it; configure, if any, completes the
// settings.
func (api testAPI) withProvider(configure func(*accounts.Settings)) *oidctest.Provider {
	api.t.Helper()
	provider := oidctest.New(api.t, "polyfin", "client-secret")
	settings := api.store.Settings()
	settings.PublicAddress = api.url
	settings.ProviderIssuer = provider.URL
	settings.ProviderName = "Example ID"
	settings.ProviderClientID = "polyfin"
	settings.ProviderClientSecret = "client-secret"
	if configure != nil {
		configure(&settings)
	}
	if _, err := api.store.UpdateSettings(api.t.Context(), settings); err != nil {
		api.t.Fatal(err)
	}
	return provider
}

// providerSignIn follows the browser from the sign-in button of app
// through the provider, which signs in at once, to the callback, and
// returns where the callback sends it, and the callback's address.
func (b browser) providerSignIn(app string) (*url.URL, *url.URL) {
	b.api.t.Helper()
	var callback *url.URL
	client := *b.client
	client.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if request.URL.Path == "/authorize" {
			return nil
		}
		if request.URL.Path == providerCallbackPath {
			callback = request.URL
			return nil
		}
		return http.ErrUseLastResponse
	}
	response, err := client.Get(b.api.url + providerPath + "start?app=" + app)
	if err != nil {
		b.api.t.Fatal(err)
	}
	response.Body.Close()
	return b.end(response), callback
}

// visit opens address as the browser does, without following where it
// is sent, and returns where it is sent.
func (b browser) visit(address string) *url.URL {
	b.api.t.Helper()
	client := *b.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(address)
	if err != nil {
		b.api.t.Fatal(err)
	}
	response.Body.Close()
	return b.end(response)
}

func (b browser) end(response *http.Response) *url.URL {
	b.api.t.Helper()
	if response.StatusCode != http.StatusSeeOther {
		b.api.t.Fatalf("answered %d, not sent on", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		b.api.t.Fatal(err)
	}
	return location
}

// outcome is what went wrong for the admin app, "" when it was sent to
// the admin app signed in.
func outcome(location *url.URL) string {
	if location.Path == "/admin/" && location.RawQuery == "" {
		return ""
	}
	if location.Path == "/web/" {
		return location.Query().Get("polyfinProvider")
	}
	return location.Query().Get("provider")
}

func (b browser) sessionUser() map[string]any {
	b.api.t.Helper()
	status, body, _ := b.call(http.MethodGet, "/session", nil)
	if status != http.StatusOK {
		return nil
	}
	user, _ := body["user"].(map[string]any)
	return user
}

// The sign-in pages offer the provider once it and the public address
// are set, and the admin app signs in through it: the person the provider
// signs in gets the account their claim names, without regard to case,
// and is linked to it, so that a later sign-in finds it whatever the
// claim then says.
func TestProviderSignsInToTheAdminAppByTheClaim(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	alice := api.createUser("Alice", false)
	if _, body, _ := api.browser().call(http.MethodGet, "/status", nil); body["provider"] != nil {
		t.Fatalf("a provider offered before one is set: %v", body["provider"])
	}
	provider := api.withProvider(nil)
	_, body, _ := api.browser().call(http.MethodGet, "/status", nil)
	offered, _ := body["provider"].(map[string]any)
	if offered["name"] != "Example ID" || offered["start"] != api.url+"/admin/api/provider/start" {
		t.Fatalf("provider offered: %v", body["provider"])
	}
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "alice"})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" {
		t.Fatalf("sent to %s", location)
	}
	if user := b.sessionUser(); user == nil || user["id"] != alice.ID.String() || user["isAdministrator"] != false {
		t.Fatalf("session: %v", user)
	}
	// The link holds whatever the claim says later.
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "renamed"})
	other := api.browser()
	if location, _ := other.providerSignIn("admin"); outcome(location) != "" || other.sessionUser()["id"] != alice.ID.String() {
		t.Errorf("a later sign-in: sent to %s", location)
	}
}

// Without the claim, the verified email names the account; an email the
// provider says is not verified names none.
func TestProviderMatchesByEmail(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	carol := api.createUser("carol@example.org", false)
	provider := api.withProvider(nil)
	// An email the provider says is not verified, or says nothing of,
	// names no account.
	for name, claims := range map[string]map[string]any{
		"unverified":          {"email": "Carol@example.org", "email_verified": false},
		"verification unsaid": {"email": "Carol@example.org"},
	} {
		provider.SignInAs("carol-"+name, claims)
		if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "provider_no_account" {
			t.Errorf("an email %s: sent to %s", name, location)
		}
	}
	provider.SignInAs("carol-at-provider", map[string]any{"preferred_username": "nobody", "email": "Carol@example.org", "email_verified": true})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" || b.sessionUser()["id"] != carol.ID.String() {
		t.Errorf("by email: sent to %s", location)
	}
}

// Someone no account matches gets one while the provider creates them: a
// member's, named after the claim, signing in through the provider only.
// Otherwise they are refused, and no account is made.
func TestProviderCreatesAccountsWhenAllowed(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	provider := api.withProvider(nil)
	provider.SignInAs("dave-at-provider", map[string]any{"preferred_username": "dave"})
	if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "provider_no_account" {
		t.Errorf("creation off: sent to %s", location)
	}
	if users, _ := api.store.Users(t.Context()); len(users) != 0 {
		t.Fatalf("accounts with creation off: %v", users)
	}
	settings := api.store.Settings()
	settings.ProviderCreateAccounts = true
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" {
		t.Fatalf("creation on: sent to %s", location)
	}
	users, _ := api.store.Users(t.Context())
	if len(users) != 1 || users[0].Name != "dave" || users[0].IsAdministrator || !users[0].ProviderOnly || b.sessionUser()["name"] != "dave" {
		t.Errorf("created: %+v", users)
	}
}

// The group claim holding the administrators' value makes the account an
// administrator; another value leaves it a member.
func TestProviderGroupMakesAdministrators(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	api.createUser("root", true)
	erin := api.createUser("erin", false)
	frank := api.createUser("frank", false)
	provider := api.withProvider(func(s *accounts.Settings) { s.ProviderAdminClaim, s.ProviderAdminValue = "groups", "media-admins" })
	provider.SignInAs("frank-at-provider", map[string]any{"preferred_username": "frank", "groups": []string{"family"}})
	if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "" {
		t.Fatalf("frank: sent to %s", location)
	}
	provider.SignInAs("erin-at-provider", map[string]any{"preferred_username": "erin", "groups": []string{"family", "media-admins"}})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" || b.sessionUser()["isAdministrator"] != true {
		t.Fatalf("erin: sent to %s, %v", location, b.sessionUser())
	}
	for _, user := range []accounts.User{erin, frank} {
		got, _ := api.store.User(t.Context(), user.ID)
		if got.IsAdministrator != (user.ID == erin.ID) {
			t.Errorf("%s is an administrator: %v", user.Name, got.IsAdministrator)
		}
	}
}

// An account set to sign in through the provider only refuses its right
// password, as a wrong one, and still signs in through the provider; an
// account made so from the start needs no password.
func TestProviderOnlyAccountsRefuseTheirPassword(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	grace := api.createUser("grace", false)
	signIn := func() int {
		status, _, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "grace", "password": "correct horse"})
		return status
	}
	if status := signIn(); status != http.StatusOK {
		t.Fatalf("before: %d", status)
	}
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+grace.ID.String(), map[string]any{"providerOnly": true}); status != http.StatusOK || body["providerOnly"] != true {
		t.Fatalf("set provider only: %d %v", status, body)
	}
	if status := signIn(); status != http.StatusUnauthorized {
		t.Errorf("the right password of a provider-only account: %d", status)
	}
	provider := api.withProvider(nil)
	provider.SignInAs("grace-at-provider", map[string]any{"preferred_username": "grace"})
	if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "" {
		t.Errorf("through the provider: sent to %s", location)
	}
	if status, body, _ := admin.call(http.MethodPost, "/users", map[string]any{"name": "heidi", "providerOnly": true}); status != http.StatusCreated || body["providerOnly"] != true {
		t.Errorf("created without a password: %d %v", status, body)
	}
}

// An ID token failing a check opens no session, counts as a wrong
// password, and says nothing of the token: a forged signature, an expired
// token, another audience, another issuer, another nonce.
func TestProviderRefusesBadIDTokens(t *testing.T) {
	t.Parallel()
	for name, spoil := range map[string]func(*oidctest.Provider){
		"forged signature": func(p *oidctest.Provider) { p.Forge() },
		"expired": func(p *oidctest.Provider) {
			p.Edit(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() })
		},
		"wrong audience": func(p *oidctest.Provider) { p.Edit(func(c map[string]any) { c["aud"] = "another-client" }) },
		"wrong issuer":   func(p *oidctest.Provider) { p.Edit(func(c map[string]any) { c["iss"] = "https://elsewhere.example" }) },
		"wrong nonce":    func(p *oidctest.Provider) { p.Edit(func(c map[string]any) { c["nonce"] = "made up" }) },
	} {
		api := newTestAPI(t, 1)
		api.createUser("alice", false)
		provider := api.withProvider(nil)
		provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "alice"})
		spoil(provider)
		b := api.browser()
		location, _ := b.providerSignIn("admin")
		if outcome(location) != "provider_failed" || b.sessionUser() != nil || strings.Contains(location.String(), "ey") {
			t.Errorf("%s: sent to %s", name, location)
		}
		// The failed attempt counted: the next is refused.
		if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "too_many_attempts" {
			t.Errorf("%s: then sent to %s", name, location)
		}
	}
}

// A state is taken once, by the browser it was given to, before it
// expires: the callback refuses a state replayed, one made up, one of
// another browser and one expired, and opens no session.
func TestProviderStatesAreUsedOnce(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 100)
	api.createUser("alice", false)
	provider := api.withProvider(nil)
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "alice"})
	b := api.browser()
	location, callback := b.providerSignIn("admin")
	if outcome(location) != "" || callback == nil {
		t.Fatalf("sent to %s", location)
	}
	state := callback.Query().Get("state")
	served, _ := url.Parse(api.url)
	given := func(b browser, value string) {
		b.client.Jar.SetCookies(served, []*http.Cookie{{Name: providerCookieName, Value: value, Path: providerPath}})
	}
	// Replayed with its cookie, by the browser it was given to.
	replay := api.browser()
	given(replay, "admin."+state)
	if location := replay.visit(callback.String()); outcome(location) != "provider_state" || replay.sessionUser() != nil {
		t.Errorf("replayed: sent to %s", location)
	}
	// Made up.
	madeUp := api.browser()
	given(madeUp, "admin.made-up")
	query := callback.Query()
	query.Set("state", "made-up")
	if location := madeUp.visit(api.url + providerCallbackPath + "?" + query.Encode()); outcome(location) != "provider_state" || madeUp.sessionUser() != nil {
		t.Errorf("made up: sent to %s", location)
	}
	// Another browser's: the provider's answer opened elsewhere.
	var answer *url.URL
	starter := api.browser()
	client := *starter.client
	client.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if request.URL.Path == providerCallbackPath {
			answer = request.URL
			return http.ErrUseLastResponse
		}
		return nil
	}
	if response, err := client.Get(api.url + providerPath + "start?app=admin"); err == nil {
		response.Body.Close()
	}
	stranger := api.browser()
	if answer == nil {
		t.Fatal("the provider did not answer")
	}
	if location := stranger.visit(answer.String()); outcome(location) != "provider_state" || stranger.sessionUser() != nil {
		t.Errorf("another browser: sent to %s", location)
	}
	// Expired: the browser comes back after 10 minutes.
	late := api.browser()
	client = *late.client
	answer = nil
	client.CheckRedirect = func(request *http.Request, _ []*http.Request) error {
		if request.URL.Path == providerCallbackPath {
			answer = request.URL
			return http.ErrUseLastResponse
		}
		return nil
	}
	if response, err := client.Get(api.url + providerPath + "start?app=admin"); err == nil {
		response.Body.Close()
	}
	later := time.Now().Add(providerFlowLifetime + time.Second)
	api.clock.Store(&later)
	if location := late.visit(answer.String()); outcome(location) != "provider_state" {
		t.Errorf("expired: sent to %s", location)
	}
}

// A disabled account, or one that ended, is refused through the provider
// as with a password.
func TestProviderRefusesDisabledAccounts(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	ivan := api.createUser("ivan", false)
	if status, _, _ := admin.call(http.MethodPatch, "/users/"+ivan.ID.String(), map[string]any{"isDisabled": true}); status != http.StatusOK {
		t.Fatal(status)
	}
	provider := api.withProvider(nil)
	provider.SignInAs("ivan-at-provider", map[string]any{"preferred_username": "ivan"})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "account_disabled" || b.sessionUser() != nil {
		t.Errorf("disabled: sent to %s", location)
	}
	if location, _ := b.providerSignIn("web"); outcome(location) != "account_disabled" {
		t.Errorf("disabled, web client: sent to %s", location)
	}
}

// The web client's sign-in: the callback sends the browser to /web/ with
// a ticket, which the web client trades once, as jellyfin-web's device,
// for a Jellyfin sign-in of that device. The account's two-step sign-in
// is not asked again: the device opens the admin app, from which Quick
// Connect approves a TV for the account.
func TestProviderSignsInTheWebClient(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	judy := api.createUser("judy", false)
	setup, err := api.store.StartTwoStep(t.Context(), judy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.store.ConfirmTwoStep(t.Context(), judy.ID, code(t, setup.Secret, 0)); err != nil {
		t.Fatal(err)
	}
	provider := api.withProvider(nil)
	provider.SignInAs("judy-at-provider", map[string]any{"preferred_username": "judy"})
	b := api.browser()
	location, _ := b.providerSignIn("web")
	if location.Path != "/web/" || outcome(location) != "signed-in" {
		t.Fatalf("sent to %s", location)
	}
	device := `MediaBrowser Client="Jellyfin Web", Device="Firefox", DeviceId="judy-browser", Version="12.4.0"`
	status, body, _ := b.call(http.MethodPost, "/provider/web", nil, "Authorization", device)
	token, _ := body["accessToken"].(string)
	if status != http.StatusOK || token == "" || body["userId"] != judy.ID.String() || body["serverId"] != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("trade: %d %v", status, body)
	}
	signedIn, user, err := api.store.DeviceByToken(t.Context(), token, "")
	if err != nil || user.ID != judy.ID || signedIn.DeviceID != "judy-browser" || signedIn.Client != "Jellyfin Web" || !signedIn.Provider {
		t.Fatalf("device: %+v %v", signedIn, err)
	}
	if status, _, _ := b.call(http.MethodPost, "/provider/web", nil, "Authorization", device); status != http.StatusUnauthorized {
		t.Errorf("a ticket traded twice: %d", status)
	}
	// The admin app takes the web client's sign-in, its second step done.
	admin := api.browser()
	if status, body := admin.exchange(token); status != http.StatusOK {
		t.Fatalf("admin app from the web client: %d %v", status, body)
	}
	// A password device of the same account is still refused.
	if status, _ := api.browser().exchange(api.jellyfinToken(judy)); status != http.StatusUnauthorized {
		t.Errorf("a password device of a two-step account: %d", status)
	}
	request, err := api.quickConnect.Initiate("tv-id", "Living room TV", "Swiftfin", "1.3")
	if err != nil {
		t.Fatal(err)
	}
	if status, _, _ := admin.call(http.MethodPost, "/quick-connect", map[string]string{"code": request.Code}); status != http.StatusNoContent {
		t.Fatalf("approval: %d", status)
	}
	if approved, _ := api.quickConnect.BySecret(request.Secret); approved.User == nil || *approved.User != judy.ID {
		t.Errorf("approved for %v", approved.User)
	}
}

// A two-step account signs in to the admin app through the provider
// without a code, and approves Quick Connect from that session.
func TestProviderSessionApprovesQuickConnect(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	root := api.createUser("root", true)
	settings := api.store.Settings()
	settings.TwoStepAdministrators = true
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	provider := api.withProvider(nil)
	provider.SignInAs("root-at-provider", map[string]any{"preferred_username": "root"})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" || b.sessionUser()["id"] != root.ID.String() {
		t.Fatalf("sent to %s", location)
	}
	request, err := api.quickConnect.Initiate("tv-id", "Living room TV", "Swiftfin", "1.3")
	if err != nil {
		t.Fatal(err)
	}
	if status, _, _ := b.call(http.MethodPost, "/quick-connect", map[string]string{"code": request.Code}); status != http.StatusNoContent {
		t.Fatalf("approval: %d", status)
	}
	if approved, _ := api.quickConnect.BySecret(request.Secret); approved.User == nil || *approved.User != root.ID {
		t.Errorf("approved for %v", approved.User)
	}
}

// Settings › Security keeps the provider, its client secret never sent
// back, and shows the redirect URI to register; an issuer that is no web
// address is refused.
func TestProviderSettings(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	status, body, _ := admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"publicAddress":  "https://media.example.org",
		"providerIssuer": "https://id.example.org", "providerClientId": "polyfin", "providerClientSecret": "s3cret",
		"providerUserClaim": "preferred_username", "providerCreateAccounts": true})
	if status != http.StatusOK || body["providerClientSecretSet"] != true || body["providerClientSecret"] != nil ||
		body["providerRedirectUri"] != "https://media.example.org/admin/api/provider/callback" {
		t.Fatalf("saved: %d %v", status, body)
	}
	if api.store.Settings().ProviderClientSecret != "s3cret" {
		t.Error("the secret was not kept")
	}
	if status, body, _ := admin.call(http.MethodPut, "/settings", map[string]any{"serverName": "Polyfin", "quickConnectEnabled": true, "language": "en",
		"providerIssuer": "id.example.org"}); status != http.StatusBadRequest || body["error"] != "invalid_provider" {
		t.Errorf("an issuer without a scheme: %d %v", status, body)
	}
}
