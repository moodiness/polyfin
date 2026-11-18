package jellyfin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/throttle"
)

// A client's own sign-ins between its wrong passwords for another account
// leave them counted: the eleventh is refused, and then the client is,
// whoever it signs in as.
func TestASignInKeepsTheFailuresForOtherAccountsCounted(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	s.user("root", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	s.user("mallory", nil)
	signIn := func(address, name, password string) int {
		t.Helper()
		status, _ := connected(t, s, address)(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""),
			map[string]string{"Username": name, "Pw": password})
		return status
	}
	const client = "203.0.113.9:50000"
	for i := range 10 {
		if status := signIn(client, "root", fmt.Sprintf("guess %d", i)); status != http.StatusUnauthorized {
			t.Fatalf("wrong password %d for root: %d, want 401", i+1, status)
		}
		if i == 9 {
			break
		}
		if status := signIn(client, "mallory", "correct horse"); status != http.StatusOK {
			t.Fatalf("the client's own sign-in after %d wrong passwords for root: %d, want 200", i+1, status)
		}
	}
	if status := signIn(client, "root", "guess 10"); status != http.StatusTooManyRequests {
		t.Errorf("the eleventh wrong password for root, with own sign-ins between: %d, want 429", status)
	}
	if status := signIn(client, "root", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the right password for root past the limit: %d, want 429", status)
	}
	if status := signIn(client, "mallory", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the client's own sign-in past the limit: %d, want 429", status)
	}
	// Another client has its own count.
	if status := signIn("203.0.113.10:50000", "root", "correct horse"); status != http.StatusOK {
		t.Errorf("root from another address: %d, want 200", status)
	}
}

// The addresses of one IPv6 /64 share their failed sign-ins. Another /64
// has its own, as each IPv4 address and each IPv4-mapped one has.
func TestAddressesOfOneIPv6PrefixShareTheirFailedSignIns(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 2)
	s.user("alice", nil)
	signIn := func(address, password string) int {
		t.Helper()
		status, _ := connected(t, s, address)(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""),
			map[string]string{"Username": "alice", "Pw": password})
		return status
	}
	for i := range 2 {
		address := fmt.Sprintf("[2001:db8:aaaa:bbbb::%x]:50000", i+1)
		if status := signIn(address, "wrong horse"); status != http.StatusUnauthorized {
			t.Fatalf("wrong password %d, from %s: %d, want 401", i+1, address, status)
		}
	}
	if status := signIn("[2001:db8:aaaa:bbbb::3]:50000", "wrong horse"); status != http.StatusTooManyRequests {
		t.Errorf("the third wrong password, from a third address of the /64: %d, want 429", status)
	}
	if status := signIn("[2001:db8:aaaa:bbbb:1:2:3:4]:50000", "correct horse"); status != http.StatusTooManyRequests {
		t.Errorf("the right password from a fourth address of the /64: %d, want 429", status)
	}
	if status := signIn("[2001:db8:aaaa:bbbc::1]:50000", "correct horse"); status != http.StatusOK {
		t.Errorf("the right password from another /64: %d, want 200", status)
	}
	for i := range 3 {
		for _, address := range []string{fmt.Sprintf("192.0.2.%d:50000", i+1), fmt.Sprintf("[::ffff:198.51.100.%d]:50000", i+1)} {
			if status := signIn(address, "wrong horse"); status != http.StatusUnauthorized {
				t.Errorf("a first wrong password from %s: %d, want 401", address, status)
			}
		}
	}
}

// Wrong reset PINs count for the whole server, whichever address sends
// them: past the limit a PIN is refused unchecked, the right one too, and
// stays valid. Under the limit a right PIN resets its password, and takes
// no wrong PIN back.
func TestWrongResetPINsCountForTheWholeServer(t *testing.T) {
	t.Parallel()
	s := newTestServer(t, 10)
	boss := s.user("boss", func(c *accounts.UserChanges) { c.IsAdministrator = new(true) })
	alice := s.user("alice", nil)
	post := func(address, path string, body map[string]string) (int, []byte) {
		t.Helper()
		return connected(t, s, address)(http.MethodPost, path, "", body)
	}
	// Each asks for a PIN from the local network; the test reads them in
	// place of the administrator.
	pin := func(user accounts.User) string {
		t.Helper()
		if status, body := post("[fd00::1]:40000", "/Users/ForgotPassword", map[string]string{"EnteredUsername": user.Name}); status != http.StatusOK {
			t.Fatalf("forgot password of %s: %d %s", user.Name, status, body)
		}
		pins, err := s.store.PasswordResetPINs(t.Context())
		if err != nil || pins[user.ID].PIN == "" {
			t.Fatalf("the PIN of %s: %v %v", user.Name, pins, err)
		}
		return strings.ReplaceAll(pins[user.ID].PIN, "-", "")
	}
	bossPIN, alicePIN := pin(boss), pin(alice)
	guesses := 0
	wrong := func(address string) int {
		t.Helper()
		guess := fmt.Sprintf("%08X", guesses)
		for guess == bossPIN || guess == alicePIN {
			guesses++
			guess = fmt.Sprintf("%08X", guesses)
		}
		guesses++
		status, _ := post(address, "/Users/ForgotPassword/Pin", map[string]string{"Pin": guess})
		return status
	}
	redeem := func(address, pin string) (int, PinRedeemResult) {
		t.Helper()
		status, body := post(address, "/Users/ForgotPassword/Pin", map[string]string{"Pin": pin})
		var result PinRedeemResult
		_ = json.Unmarshal(body, &result)
		return status, result
	}

	// Five wrong PINs, each from another /64.
	for i := range 5 {
		address := fmt.Sprintf("[2001:db8:0:%x::1]:40000", i+1)
		if status := wrong(address); status != http.StatusNotFound {
			t.Fatalf("wrong PIN %d, from %s: %d, want 404", i+1, address, status)
		}
	}
	// Under the limit, a right PIN works.
	if status, result := redeem("198.51.100.1:40000", alicePIN); status != http.StatusOK || !result.Success ||
		len(result.UsersReset) != 1 || result.UsersReset[0] != "alice" {
		t.Fatalf("a right PIN after 5 wrong ones: %d %+v", status, result)
	}
	// Five more, each from another IPv4 address: the right PIN took none
	// of the first five back.
	for i := range 5 {
		address := fmt.Sprintf("203.0.113.%d:40000", i+1)
		if status := wrong(address); status != http.StatusNotFound {
			t.Fatalf("wrong PIN %d, from %s: %d, want 404", i+6, address, status)
		}
	}
	if status := wrong("192.0.2.1:40000"); status != http.StatusTooManyRequests {
		t.Fatalf("the eleventh wrong PIN, from an eleventh address: %d, want 429", status)
	}
	if status, result := redeem("192.0.2.2:40000", bossPIN); status != http.StatusTooManyRequests || result.Success {
		t.Fatalf("the right PIN past the limit: %d %+v, want 429", status, result)
	}
	// Refused unchecked, the PIN is kept and the password is the same.
	if pins, err := s.store.PasswordResetPINs(t.Context()); err != nil || strings.ReplaceAll(pins[boss.ID].PIN, "-", "") != bossPIN {
		t.Errorf("the PIN refused past the limit: %v %v, want it kept", pins, err)
	}
	if _, err := s.store.Authenticate(t.Context(), "boss", "correct horse"); err != nil {
		t.Errorf("the password after a PIN refused past the limit: %v", err)
	}
	// The limit of sign-ins is each client's own still.
	if status, _ := connected(t, s, "192.0.2.3:40000")(http.MethodPost, "/Users/AuthenticateByName", app("laptop", ""),
		map[string]string{"Username": "alice", "Pw": alicePIN}); status != http.StatusOK {
		t.Errorf("a sign-in while PINs are refused: %d, want 200", status)
	}

	// Once the wrong PINs have left the window, the same PIN works. The
	// test cannot move the clock of the handler's failures: new ones, with
	// nothing counted, stand for the time passed.
	s.handler.SignIns = throttle.New(10, time.Minute)
	if status, result := redeem("192.0.2.2:40000", bossPIN); status != http.StatusOK || !result.Success ||
		len(result.UsersReset) != 1 || result.UsersReset[0] != "boss" {
		t.Fatalf("the right PIN once the limit lifted: %d %+v", status, result)
	}
	if _, err := s.store.Authenticate(t.Context(), "boss", bossPIN); err != nil {
		t.Errorf("the PIN as password: %v", err)
	}
}
