package admin

import (
	"net/http"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
)

// account reads the account signed in on b from the store.
func (b browser) account() accounts.User {
	b.api.t.Helper()
	raw, _ := b.sessionUser()["id"].(string)
	id, err := accounts.ParseID(raw)
	if err != nil {
		b.api.t.Fatalf("no session: %v", err)
	}
	user, err := b.api.store.User(b.api.t.Context(), id)
	if err != nil {
		b.api.t.Fatal(err)
	}
	return user
}

// A name says nothing of whose an account is. Someone the provider signs
// in whose claim, or whose verified email, is the name of a local
// administrator gets no session, in the admin app or the web client, and
// the administrator's account stays as it was: not linked, its two-step
// sign-in on, its session its own. With accounts created, none is made
// under that name either.
func TestProviderRefusesSomeoneNamedAsALocalAdministrator(t *testing.T) {
	t.Parallel()
	for name, claims := range map[string]map[string]any{
		"by the claim":          {"preferred_username": "Root@Example.org"},
		"by the verified email": {"preferred_username": "nobody", "email": "Root@Example.org", "email_verified": true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			api := newTestAPI(t, 10)
			root := api.signedIn("root@example.org", true)
			turnOnTwoStep(t, root)
			if _, status, body := api.password("root@example.org"); status != http.StatusOK || body["twoStep"] != "code" {
				t.Fatalf("the administrator's password with two-step sign-in on: %d %v", status, body)
			}
			// The provider as Settings › Security sets it by default.
			provider := api.withProvider(nil)
			if settings := api.store.Settings(); settings.ProviderUserClaim != "preferred_username" || settings.ProviderCreateAccounts {
				t.Fatalf("provider settings: claim %q, create %v", settings.ProviderUserClaim, settings.ProviderCreateAccounts)
			}
			provider.SignInAs("someone-else-7421", claims)
			refused := func(when string) {
				t.Helper()
				for _, app := range []string{"admin", "web"} {
					stranger := api.browser()
					if location, _ := stranger.providerSignIn(app); outcome(location) != "provider_no_account" {
						t.Fatalf("%s, %s: sent to %s, want provider_no_account", when, app, location)
					}
					if user := stranger.sessionUser(); user != nil {
						t.Fatalf("%s, %s: a session of %v", when, app, user["name"])
					}
					if status, _, _ := stranger.call(http.MethodPost, "/provider/web", nil, "Authorization",
						`MediaBrowser Client="Jellyfin Web", Device="Firefox", DeviceId="stranger", Version="12.4.0"`); status != http.StatusUnauthorized {
						t.Fatalf("%s, %s: a ticket traded: %d", when, app, status)
					}
				}
				if account := root.account(); account.ProviderIssuer != "" || !account.TwoStep || !account.IsAdministrator {
					t.Fatalf("%s: the administrator's account: linked at %q, two-step %v", when, account.ProviderIssuer, account.TwoStep)
				}
				if status, _, _ := root.call(http.MethodGet, "/users", nil); status != http.StatusOK || root.accountLinked() {
					t.Fatalf("%s: the administrator's session: %d, linked %v", when, status, root.accountLinked())
				}
			}
			refused("accounts not created")
			settings := api.store.Settings()
			settings.ProviderCreateAccounts = true
			if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
				t.Fatal(err)
			}
			refused("accounts created")
			if users, _ := api.store.Users(t.Context()); len(users) != 1 {
				t.Errorf("accounts: %d, want the administrator's alone", len(users))
			}
		})
	}
}

// An account that bears someone's name before they ever signed in, here a
// guest's who chose it joining by an invite, is not theirs: when that
// person, one of the provider's administrators, signs in for the first
// time, they are refused, and the guest's account is neither linked nor
// made an administrator. Someone whose name no account bears still gets a
// new account, and the group's rights.
func TestProviderDoesNotLinkAnAccountThatTookSomeonesName(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	root := api.signedIn("root", true)
	provider := api.withProvider(func(s *accounts.Settings) {
		s.ProviderAdminClaim, s.ProviderAdminValue = "groups", "polyfin-admins"
		s.ProviderCreateAccounts = true
	})
	status, invite, _ := root.call(http.MethodPost, "/invites", map[string]any{})
	token, _ := invite["token"].(string)
	if status != http.StatusCreated || token == "" {
		t.Fatalf("invite: %d %v", status, invite)
	}
	guest := api.browser()
	if status, body, _ := guest.call(http.MethodPost, "/invite/"+token, map[string]string{"name": "jdoe", "password": "the guest's own"}); status != http.StatusCreated {
		t.Fatalf("joining as jdoe: %d %v", status, body)
	}
	if status, _, _ := guest.call(http.MethodGet, "/users", nil); status != http.StatusForbidden {
		t.Fatalf("an administrators' route for the guest: %d, want 403", status)
	}

	provider.SignInAs("jdoe-subject-0042", map[string]any{"preferred_username": "jdoe", "groups": []string{"staff", "polyfin-admins"}})
	person := api.browser()
	if location, _ := person.providerSignIn("admin"); outcome(location) != "provider_no_account" || person.sessionUser() != nil {
		t.Fatalf("jdoe of the provider: sent to %s, session %v", location, person.sessionUser())
	}
	if account := guest.account(); account.IsAdministrator || account.ProviderIssuer != "" {
		t.Fatalf("the guest's account: administrator %v, linked at %q", account.IsAdministrator, account.ProviderIssuer)
	}
	if status, _, _ := guest.call(http.MethodGet, "/users", nil); status != http.StatusForbidden {
		t.Errorf("an administrators' route for the guest afterwards: %d, want 403", status)
	}
	status, body, _ := api.browser().call(http.MethodPost, "/session", map[string]string{"name": "jdoe", "password": "the guest's own"})
	if user, _ := body["user"].(map[string]any); status != http.StatusOK || user["isAdministrator"] != false {
		t.Errorf("the guest's password afterwards: %d %v", status, body)
	}

	provider.SignInAs("kim-subject-0043", map[string]any{"preferred_username": "kim", "groups": []string{"polyfin-admins"}})
	kim := api.browser()
	if location, _ := kim.providerSignIn("admin"); outcome(location) != "" {
		t.Fatalf("kim, whose name is free: sent to %s", location)
	}
	if user := kim.sessionUser(); user["name"] != "kim" || user["isAdministrator"] != true || user["id"] == guest.sessionUser()["id"] {
		t.Errorf("kim's session: %v", user)
	}
}

// An account an administrator allowed, on its page, is linked to the
// person the provider signs in next under its name, and the permission is
// used up: someone else under that name later is refused, and the person
// linked signs in whatever their claim says.
func TestAnAdministratorAllowsAnAccountToBeLinkedOnce(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	alice := api.createUser("alice", false)
	provider := api.withProvider(nil)
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "Alice"})
	if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "provider_no_account" {
		t.Fatalf("before the permission: sent to %s", location)
	}
	if user := api.user(t, admin, alice.ID.String()); user["providerLinked"] != false || user["providerLinkAllowed"] != false {
		t.Fatalf("before the permission: %v %v", user["providerLinked"], user["providerLinkAllowed"])
	}
	status, body, _ := admin.call(http.MethodPatch, "/users/"+alice.ID.String(), map[string]any{"providerLinkAllowed": true})
	if status != http.StatusOK || body["providerLinkAllowed"] != true || body["providerLinked"] != false {
		t.Fatalf("the permission: %d %v", status, body)
	}
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" || b.sessionUser()["id"] != alice.ID.String() {
		t.Fatalf("with the permission: sent to %s, session %v", location, b.sessionUser())
	}
	if user := api.user(t, admin, alice.ID.String()); user["providerLinked"] != true || user["providerLinkAllowed"] != false {
		t.Errorf("once linked: linked %v, allowed %v", user["providerLinked"], user["providerLinkAllowed"])
	}
	provider.SignInAs("another-alice", map[string]any{"preferred_username": "alice"})
	if location, _ := api.browser().providerSignIn("admin"); outcome(location) != "provider_no_account" {
		t.Errorf("someone else under the name: sent to %s", location)
	}
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "renamed"})
	again := api.browser()
	if location, _ := again.providerSignIn("admin"); outcome(location) != "" || again.sessionUser()["id"] != alice.ID.String() {
		t.Errorf("the person linked, later: sent to %s", location)
	}
}

// The permission is not given for an administrator, nor for an account
// with two-step sign-in, alone or with other changes, which are then not
// made either; given to a member who became one since, it links nobody.
func TestThePermissionToLinkIsNotForAdministratorsNorTwoStepAccounts(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	admin := api.signedIn("root", true)
	allow := func(user accounts.User, more map[string]any) (int, map[string]any) {
		changes := map[string]any{"providerLinkAllowed": true}
		for name, value := range more {
			changes[name] = value
		}
		status, body, _ := admin.call(http.MethodPatch, "/users/"+user.ID.String(), changes)
		return status, body
	}
	if status, body := allow(admin.account(), nil); status != http.StatusConflict || body["error"] != "provider_link_refused" {
		t.Errorf("for an administrator: %d %v", status, body)
	}
	bob := api.signedIn("bob", false)
	turnOnTwoStep(t, bob)
	if status, body := allow(bob.account(), nil); status != http.StatusConflict || body["error"] != "provider_link_refused" {
		t.Errorf("for an account with two-step sign-in: %d %v", status, body)
	}
	carl := api.createUser("carl", false)
	if status, body := allow(carl, map[string]any{"isAdministrator": true, "isHidden": true}); status != http.StatusConflict || body["error"] != "provider_link_refused" {
		t.Errorf("for an account made an administrator at once: %d %v", status, body)
	}
	if got, _ := api.store.User(t.Context(), carl.ID); got.IsAdministrator || got.IsHidden || got.ProviderLinkAllowed {
		t.Errorf("a refused change was made in part: %+v", got)
	}
	// Taking it away is always possible.
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+admin.account().ID.String(), map[string]any{"providerLinkAllowed": false}); status != http.StatusOK {
		t.Errorf("taken away from an administrator: %d %v", status, body)
	}

	// Given to members, who then become an administrator, and an account
	// with two-step sign-in.
	dora := api.signedIn("dora", false)
	for _, user := range []accounts.User{carl, dora.account()} {
		if status, body := allow(user, nil); status != http.StatusOK || body["providerLinkAllowed"] != true {
			t.Fatalf("for the member %s: %d %v", user.Name, status, body)
		}
	}
	if status, body, _ := admin.call(http.MethodPatch, "/users/"+carl.ID.String(), map[string]any{"isAdministrator": true}); status != http.StatusOK {
		t.Fatalf("carl made an administrator: %d %v", status, body)
	}
	turnOnTwoStep(t, dora)
	provider := api.withProvider(nil)
	for _, user := range []accounts.User{carl, dora.account()} {
		provider.SignInAs(user.Name+"-at-provider", map[string]any{"preferred_username": user.Name})
		b := api.browser()
		if location, _ := b.providerSignIn("admin"); outcome(location) != "provider_no_account" || b.sessionUser() != nil {
			t.Errorf("%s, with a permission given before: sent to %s", user.Name, location)
		}
		if got, _ := api.store.User(t.Context(), user.ID); got.ProviderIssuer != "" {
			t.Errorf("%s was linked at %q", user.Name, got.ProviderIssuer)
		}
	}
}

// An account links itself from My account: the person the provider signs
// in, in the browser of the session that asked, is linked to that
// session's account. An administrator with two-step sign-in does so too,
// their session having taken their password and code. The link opens no
// session and gives no rights; the person then signs in through the
// provider, where the administrators' group grants as before.
func TestAnAccountLinksItselfFromItsSession(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 10)
	root := api.signedIn("root", true)
	turnOnTwoStep(t, root)
	provider := api.withProvider(func(s *accounts.Settings) { s.ProviderAdminClaim, s.ProviderAdminValue = "groups", "media-admins" })
	link := func(b browser, who, subject string, claims map[string]any) {
		t.Helper()
		if b.accountLinked() {
			t.Fatalf("%s is linked before asking", who)
		}
		provider.SignInAs(subject, claims)
		got, answer := b.endLink(b.startLink())
		if got != "linked" {
			t.Fatalf("%s: %s", who, got)
		}
		for _, cookie := range answer.Cookies() {
			if cookie.Name == cookieName {
				t.Errorf("%s: the link opened a session", who)
			}
		}
		if !b.accountLinked() {
			t.Fatalf("%s is not linked afterwards", who)
		}
	}
	link(root, "the administrator", "root-at-provider", map[string]any{"preferred_username": "someone"})
	b := api.browser()
	if location, _ := b.providerSignIn("admin"); outcome(location) != "" || b.sessionUser()["id"] != root.sessionUser()["id"] {
		t.Errorf("the administrator, through the provider: sent to %s", location)
	}
	// Linked, an account asks no second link.
	if status, body, _ := root.call(http.MethodPost, "/account/provider/link", nil); status != http.StatusConflict || body["error"] != "provider_link_taken" {
		t.Errorf("a second link: %d %v", status, body)
	}

	erin := api.signedIn("erin", false)
	inGroup := map[string]any{"preferred_username": "whoever", "groups": []string{"media-admins"}}
	link(erin, "the member", "erin-at-provider", inGroup)
	if account := erin.account(); account.IsAdministrator {
		t.Error("the link made the member an administrator")
	}
	later := api.browser()
	if location, _ := later.providerSignIn("admin"); outcome(location) != "" || later.sessionUser()["id"] != erin.sessionUser()["id"] ||
		later.sessionUser()["isAdministrator"] != true {
		t.Errorf("the member, through the provider: sent to %s, %v", location, later.sessionUser())
	}
}

// A link is asked from a session, and made for it alone: not without a
// session, not through the sign-in's own start, not when the provider's
// answer comes back to another browser, not once the session that asked
// was signed out, not after the time a sign-in has, and not to a person
// another account is linked to.
func TestALinkNeedsTheSessionAndTheBrowserThatAskedForIt(t *testing.T) {
	t.Parallel()
	api := newTestAPI(t, 100)
	provider := api.withProvider(nil)
	if status, body, _ := api.browser().call(http.MethodPost, "/account/provider/link", nil); status != http.StatusUnauthorized || body["error"] != "unauthenticated" {
		t.Fatalf("without a session: %d %v", status, body)
	}
	unlinked := func(b browser, when string) {
		t.Helper()
		if account := b.account(); account.ProviderIssuer != "" || b.accountLinked() {
			t.Fatalf("%s: %s was linked", when, account.Name)
		}
	}

	alice := api.signedIn("alice", false)
	provider.SignInAs("alice-at-provider", map[string]any{"preferred_username": "nobody"})
	if location, _ := alice.providerSignIn("link"); outcome(location) != "provider_no_account" {
		t.Errorf("the sign-in's start, asked to link: sent to %s", location)
	}
	unlinked(alice, "the sign-in's start")

	address := alice.startLink()
	if got, _ := api.browser().endLink(address); got != "sent to /admin/?provider=provider_state" {
		t.Errorf("another browser: %s", got)
	}
	unlinked(alice, "another browser")

	bob := api.signedIn("bob", false)
	address = bob.startLink()
	if status, _, _ := bob.call(http.MethodDelete, "/session", nil); status != http.StatusNoContent {
		t.Fatalf("signing bob out: %d", status)
	}
	if got, _ := bob.endLink(address); got != "provider_state" {
		t.Errorf("the session signed out meanwhile: %s", got)
	}
	if got, _ := api.store.User(t.Context(), api.userID("bob")); got.ProviderIssuer != "" {
		t.Errorf("bob was linked at %q after his session ended", got.ProviderIssuer)
	}

	carol := api.createUser("carol", false)
	api.linked(carol, provider, "carol-at-provider")
	dave := api.signedIn("dave", false)
	provider.SignInAs("carol-at-provider", map[string]any{"preferred_username": "dave"})
	if got, _ := dave.endLink(dave.startLink()); got != "provider_link_taken" {
		t.Errorf("a person linked to another account: %s", got)
	}
	unlinked(dave, "a person linked to another account")
	still := api.browser()
	if location, _ := still.providerSignIn("admin"); outcome(location) != "" || still.sessionUser()["id"] != carol.ID.String() {
		t.Errorf("the person, through the provider: sent to %s, %v", location, still.sessionUser())
	}

	provider.SignInAs("dave-at-provider", map[string]any{"preferred_username": "dave"})
	address = dave.startLink()
	late := time.Now().Add(providerFlowLifetime + time.Second)
	api.clock.Store(&late)
	if got, _ := dave.endLink(address); got != "provider_state" {
		t.Errorf("after the sign-in's time: %s", got)
	}
	unlinked(dave, "after the sign-in's time")
}
