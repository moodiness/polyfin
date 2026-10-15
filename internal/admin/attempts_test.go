package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb"
)

// The attempts below are sent at once while the test holds the user's row,
// which checking them locks or writes: every attempt that passed the
// throttle then waits in the database, and the others are answered. With a
// limit of 2, no more than 2 may be checked; the test pool's 4 connections
// bound how many attempts may wait at once.
const (
	attemptsLimit = 2
	attemptsSent  = 3
)

// newHeldAPI serves the admin API with a limit of attemptsLimit failed
// attempts, and returns the pool of its database.
func newHeldAPI(t *testing.T) (testAPI, *pgxpool.Pool) {
	t.Helper()
	var pool *pgxpool.Pool
	api := newTestAPI(t, attemptsLimit, func(_ *Options, deps testDeps) { pool = deps.pool })
	return api, pool
}

// atOnce sends attemptsSent requests of b at once while the row of the user
// name is held, waits until each one is answered or waits on the row, then
// releases it, and counts the answers by status.
func atOnce(t *testing.T, pool *pgxpool.Pool, name string, b browser, path string, body any) map[int]int {
	t.Helper()
	hold := testdb.HoldRows(t, pool, "SELECT FROM users WHERE name = $1 FOR UPDATE", name)
	encoded, _ := json.Marshal(body)
	answers := make(chan int, attemptsSent)
	for range attemptsSent {
		go func() {
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, b.api.url+"/admin/api"+path, bytes.NewReader(encoded))
			request.Header.Set("Content-Type", "application/json")
			response, err := b.client.Do(request)
			if err != nil {
				t.Error(err)
				answers <- 0
				return
			}
			response.Body.Close()
			answers <- response.StatusCode
		}()
	}
	counts, answered := map[int]int{}, 0
	hold.AwaitWaiting(func(waiting int) bool {
		for {
			select {
			case status := <-answers:
				counts[status]++
				answered++
			default:
				return answered+waiting >= attemptsSent
			}
		}
	})
	hold.Release()
	for ; answered < attemptsSent; answered++ {
		counts[<-answers]++
	}
	return counts
}

// Wrong codes sent at once with the same step count before they are
// checked: past the limit, they are refused without being checked.
func TestWrongCodesAtOnceAreCountedBeforeTheyAreChecked(t *testing.T) {
	t.Parallel()
	api, pool := newHeldAPI(t)
	turnOnTwoStep(t, api.signedIn("alice", false))
	b, status, body := api.password("alice")
	if status != http.StatusOK || body["twoStep"] != "code" {
		t.Fatalf("password: %d %v", status, body)
	}
	counts := atOnce(t, pool, "alice", b, "/session/two-step", map[string]string{"code": "000000"})
	if counts[http.StatusBadRequest] != attemptsLimit || counts[http.StatusTooManyRequests] != attemptsSent-attemptsLimit {
		t.Errorf("%d wrong codes at once with a limit of %d: %v", attemptsSent, attemptsLimit, counts)
	}
}

// The same for the codes that confirm a change of two-step sign-in.
func TestWrongConfirmationsAtOnceAreCountedBeforeTheyAreChecked(t *testing.T) {
	t.Parallel()
	api, pool := newHeldAPI(t)
	alice := api.signedIn("alice", false)
	turnOnTwoStep(t, alice)
	counts := atOnce(t, pool, "alice", alice, "/account/two-step/recovery-codes", map[string]string{"code": "000000"})
	if counts[http.StatusBadRequest] != attemptsLimit || counts[http.StatusTooManyRequests] != attemptsSent-attemptsLimit {
		t.Errorf("%d wrong codes at once with a limit of %d: %v", attemptsSent, attemptsLimit, counts)
	}
}

// The same for wrong passwords. Accounts block after wrong passwords here,
// so that a wrong one writes the user's row, which the test holds.
func TestWrongPasswordsAtOnceAreCountedBeforeTheyAreChecked(t *testing.T) {
	t.Parallel()
	api, pool := newHeldAPI(t)
	if _, err := api.store.CreateUser(t.Context(), accounts.NewUser{Name: "alice", Password: "correct horse"}); err != nil {
		t.Fatal(err)
	}
	settings := api.store.Settings()
	settings.LoginAttempts = accounts.MaxLoginAttempts
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	counts := atOnce(t, pool, "alice", api.browser(), "/session", map[string]string{"name": "alice", "password": "wrong horse"})
	if counts[http.StatusUnauthorized] != attemptsLimit || counts[http.StatusTooManyRequests] != attemptsSent-attemptsLimit {
		t.Errorf("%d wrong passwords at once with a limit of %d: %v", attemptsSent, attemptsLimit, counts)
	}
}
