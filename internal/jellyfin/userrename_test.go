package jellyfin

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// A member renames their own account, as jellyfin-web's profile page
// does, to the name or the verified email the identity provider gives for
// someone who never signed in through it. The name does not make the
// account that person's: their first sign-in through the provider is
// refused, with accounts created or not, and the member's account is
// neither linked nor given the rights of the provider's administrators.
func TestAMemberRenamedAsSomeoneIsNotLinkedToThemByTheProvider(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		rename   string
		identity accounts.ProviderIdentity
	}{
		{"by the name claim", "dana.admin", accounts.ProviderIdentity{Issuer: "https://idp.example", Subject: "sub-dana",
			Name: "dana.admin", Email: "dana@corp.example", Administrator: true}},
		// The name claim names no account: the verified email is tried.
		{"by the verified email", "dana@corp.example", accounts.ProviderIdentity{Issuer: "https://idp.example", Subject: "sub-dana",
			Name: `CORP\dana`, Email: "dana@corp.example", Administrator: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newTestServer(t, 10)
			s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
			member := s.user("morgan", nil)
			header := app("laptop", s.signIn("morgan", "laptop"))

			status, body := s.call(http.MethodGet, "/Users/Me", header, nil)
			var me UserDto
			if status != http.StatusOK || json.Unmarshal(body, &me) != nil {
				t.Fatalf("me: %d %s", status, body)
			}
			me.Name = tc.rename
			if status, body := s.call(http.MethodPost, "/Users/"+member.ID.String(), header, me); status != http.StatusNoContent {
				t.Fatalf("renaming oneself %q: %d %s", tc.rename, status, body)
			}

			for _, create := range []bool{false, true} {
				user, created, err := s.store.ProviderUser(t.Context(), tc.identity, create)
				if !errors.Is(err, accounts.ErrNoProviderAccount) {
					t.Fatalf("the person's sign-in through the provider, create %v: account %q, created %v, %v; want ErrNoProviderAccount",
						create, user.Name, created, err)
				}
			}
			got, err := s.store.User(t.Context(), member.ID)
			if err != nil || got.Name != tc.rename || got.IsAdministrator || got.ProviderIssuer != "" {
				t.Fatalf("the member's account afterwards: %q, administrator %v, linked at %q, %v", got.Name, got.IsAdministrator, got.ProviderIssuer, err)
			}
			if status, _ := s.call(http.MethodGet, "/Auth/Keys", header, nil); status != http.StatusForbidden {
				t.Errorf("the member lists API keys: %d, want 403", status)
			}
			status, body = s.call(http.MethodPost, "/Users/AuthenticateByName", app("phone", ""),
				map[string]string{"Username": tc.rename, "Pw": "correct horse"})
			var result AuthenticationResult
			if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.User.Policy.IsAdministrator {
				t.Errorf("the member's password afterwards: %d %s", status, body)
			}
		})
	}
}
