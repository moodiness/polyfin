package jellyfin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/testdb"
)

// Wrong passwords sent at once count before they are checked: past the
// limit, they are refused without being checked. The test holds the user's
// row, which a wrong password writes while accounts block after wrong
// passwords, so that every password that passed the throttle waits in the
// database while the others are answered.
func TestWrongPasswordsAtOnceAreCountedBeforeTheyAreChecked(t *testing.T) {
	t.Parallel()
	// The test pool's 4 connections bound how many may wait at once.
	const limit, sent = 2, 3
	s := newTestServer(t, limit)
	s.user("alice", nil)
	s.setting(t, func(settings *accounts.Settings) { settings.LoginAttempts = accounts.MaxLoginAttempts })
	hold := testdb.HoldRows(t, s.pool, "SELECT FROM users WHERE name = $1 FOR UPDATE", "alice")
	encoded, _ := json.Marshal(map[string]string{"Username": "alice", "Pw": "wrong horse"})
	answers := make(chan int, sent)
	for range sent {
		go func() {
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+"/Users/AuthenticateByName", bytes.NewReader(encoded))
			request.Header.Set("Authorization", app("tv", ""))
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
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
				return answered+waiting >= sent
			}
		}
	})
	hold.Release()
	for ; answered < sent; answered++ {
		counts[<-answers]++
	}
	if counts[http.StatusUnauthorized] != limit || counts[http.StatusTooManyRequests] != sent-limit {
		t.Errorf("%d wrong passwords at once with a limit of %d: %v", sent, limit, counts)
	}
}
