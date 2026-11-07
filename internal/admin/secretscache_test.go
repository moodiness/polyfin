package admin

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/secrets"
)

// Health and its problems, polled every few seconds, decrypt the stored
// secrets once: again once a secret is sealed to be stored, after an
// administrator's write, which may have removed one, or after
// secretsKeptFor; a read alone does not. Not parallel: secrets.Changes
// counts the secrets other tests seal.
func TestHealthChecksTheSecretsAgainOnlyAfterAChange(t *testing.T) {
	var inspected atomic.Int64
	api := newTestAPI(t, 10, func(o *Options, _ testDeps) {
		o.Health = HealthSources{Secrets: func(context.Context) (secrets.Report, error) {
			inspected.Add(1)
			return secrets.Report{Unreadable: []secrets.Unreadable{{Target: "Team pager"}}}, nil
		}}
	})
	admin := api.signedIn("root", true)
	now := time.Now()
	api.clock.Store(&now)
	polled := func(when string, want int64) {
		t.Helper()
		before := inspected.Load()
		var health healthJSON
		if status := admin.raw(http.MethodGet, "/health", "", &health); status != http.StatusOK || health.Secrets == nil ||
			len(health.Secrets.Unreadable) != 1 {
			t.Fatalf("%s: health %d %+v", when, status, health.Secrets)
		}
		if status := admin.raw(http.MethodGet, "/health/problems", "", nil); status != http.StatusOK {
			t.Fatalf("%s: problems %d", when, status)
		}
		if got := inspected.Load() - before; got != want {
			t.Errorf("%s: the secrets were checked %d times, want %d", when, got, want)
		}
	}

	polled("the first poll", 1)
	polled("a poll with no change", 0)
	box, err := secrets.New(make([]byte, secrets.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	box.Seal("new token")
	polled("after a secret sealed", 1)
	polled("after a secret sealed, again", 0)
	admin.raw(http.MethodDelete, "/account/devices/00000000-0000-0000-0000-000000000000", "", nil)
	polled("after an administrator's write", 1)
	admin.raw(http.MethodGet, "/account/devices", "", nil)
	polled("after a read", 0)
	later := now.Add(secretsKeptFor)
	api.clock.Store(&later)
	polled("after secretsKeptFor", 1)
}
