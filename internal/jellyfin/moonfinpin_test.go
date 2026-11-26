package jellyfin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/moodiness/polyfin/internal/accounts"
)

// pinAnswer is the answer of a PIN's route.
type pinAnswer struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

// pinCall calls a PIN's route of s with body, sent as it is when a
// string, and keeps every answer's text in seen.
func pinCall(t *testing.T, s testServer, seen *strings.Builder, method, path, auth string, body any) pinAnswer {
	t.Helper()
	var reader io.Reader
	switch body := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(body)
	default:
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request, _ := http.NewRequestWithContext(t.Context(), method, s.url+path, reader)
	request.Header.Set("Authorization", auth)
	response, err := testClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	seen.Write(data)
	answer := pinAnswer{status: response.StatusCode, header: response.Header, raw: string(data)}
	_ = json.Unmarshal(data, &answer.body)
	return answer
}

// A PIN set on one device holds on the user's others: it is checked, then
// changed and removed with the current one, which a wrong one refuses. No
// answer or log line holds the PIN or its hash; another user's PIN is
// apart, an API key has no PIN, and a user the companion is not served
// to finds no PIN routes.
func TestMoonfinPinsFollowTheUserAcrossDevices(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	s.warnings.all.Store(true)
	phone, tv := app("phone", s.signIn("alice", "phone")), app("tv", s.signIn("alice", "tv"))
	bob := app("bob-tv", s.signIn("bob", "bob-tv"))
	var seen strings.Builder
	call := func(method, path, auth string, body any) pinAnswer {
		t.Helper()
		return pinCall(t, s, &seen, method, path, auth, body)
	}
	if _, ping := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Ping", phone, nil); ping["pinSupported"] != true {
		t.Errorf("pinSupported: %v", ping["pinSupported"])
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", phone, nil); a.status != http.StatusOK || a.body["enabled"] != false {
		t.Errorf("before any PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPost, "/Moonfin/Pin/Verify", phone, map[string]string{"pin": "4821"}); a.status != http.StatusNotFound ||
		a.body["error"] != "No PIN is set" {
		t.Errorf("a check without a PIN: %d %s", a.status, a.raw)
	}
	for _, pin := range []any{"482", "48210", "48a1", 4821} {
		if a := call(http.MethodPut, "/Moonfin/Pin", phone, map[string]any{"pin": pin}); a.status != http.StatusBadRequest ||
			a.body["error"] != "A PIN is exactly four digits" {
			t.Errorf("PIN %v: %d %s", pin, a.status, a.raw)
		}
	}
	if a := call(http.MethodPut, "/Moonfin/Pin", phone, "not json"); a.status != http.StatusBadRequest {
		t.Errorf("a body that is no JSON: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPut, "/Moonfin/Pin", phone, map[string]string{"pin": "4821"}); a.status != http.StatusOK || a.body["enabled"] != true {
		t.Fatalf("set: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", tv, nil); a.body["enabled"] != true {
		t.Errorf("on the TV: %s", a.raw)
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", bob, nil); a.body["enabled"] != false {
		t.Errorf("bob's: %s", a.raw)
	}
	// Names are matched in any case, as the companion's model binding does.
	if a := call(http.MethodPost, "/Moonfin/Pin/Verify", tv, map[string]string{"Pin": "4821"}); a.status != http.StatusOK || a.body["valid"] != true {
		t.Errorf("the right PIN on the TV: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPost, "/Moonfin/Pin/Verify", tv, map[string]string{"pin": "1111"}); a.status != http.StatusOK || a.body["valid"] != false {
		t.Errorf("a wrong PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPost, "/Moonfin/Pin/Verify", bob, map[string]string{"pin": "4821"}); a.status != http.StatusNotFound {
		t.Errorf("alice's PIN for bob: %d %s", a.status, a.raw)
	}
	for _, body := range []map[string]string{{"pin": "7777"}, {"pin": "7777", "currentPin": "1111"}} {
		if a := call(http.MethodPut, "/Moonfin/Pin", tv, body); a.status != http.StatusForbidden || a.body["error"] != "The current PIN is wrong" {
			t.Errorf("a change with %v: %d %s", body, a.status, a.raw)
		}
	}
	if a := call(http.MethodPut, "/Moonfin/Pin", tv, map[string]string{"pin": "7777", "currentPin": "4821"}); a.status != http.StatusOK {
		t.Fatalf("a change with the current PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPost, "/Moonfin/Pin/Verify", phone, map[string]string{"pin": "7777"}); a.body["valid"] != true {
		t.Errorf("the new PIN on the phone: %s", a.raw)
	}
	for _, body := range []any{nil, map[string]string{"currentPin": "4821"}} {
		if a := call(http.MethodDelete, "/Moonfin/Pin", phone, body); a.status != http.StatusForbidden || a.body["error"] != "The current PIN is wrong" {
			t.Errorf("a removal with %v: %d %s", body, a.status, a.raw)
		}
	}
	if a := call(http.MethodDelete, "/Moonfin/Pin", phone, map[string]string{"currentPin": "7777"}); a.status != http.StatusOK || a.body["enabled"] != false {
		t.Fatalf("a removal with the current PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", tv, nil); a.body["enabled"] != false {
		t.Errorf("on the TV after the removal: %s", a.raw)
	}
	// What an app sent last is in the store, a hash of it alone.
	if a := call(http.MethodPut, "/Moonfin/Pin", phone, map[string]string{"pin": "2468"}); a.status != http.StatusOK {
		t.Fatalf("set again: %d %s", a.status, a.raw)
	}
	var hash string
	if err := s.pool.QueryRow(t.Context(), "SELECT hash FROM moonfin_pins WHERE user_id = $1", users["alice"].ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}

	_, key, err := s.store.CreateAPIKey(t.Context(), "Script")
	if err != nil {
		t.Fatal(err)
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", `MediaBrowser Token="`+key+`"`, nil); a.status != http.StatusBadRequest {
		t.Errorf("an API key: %d %s", a.status, a.raw)
	}
	off := accounts.MoonfinOff
	if _, err := s.store.UpdateUser(t.Context(), users["alice"].ID, accounts.UserChanges{Moonfin: &off}, nil); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/Moonfin/Pin", nil},
		{http.MethodPut, "/Moonfin/Pin", map[string]string{"pin": "1357", "currentPin": "2468"}},
		{http.MethodDelete, "/Moonfin/Pin", map[string]string{"currentPin": "2468"}},
		{http.MethodPost, "/Moonfin/Pin/Verify", map[string]string{"pin": "2468"}},
	} {
		if a := call(route.method, route.path, phone, route.body); a.status != http.StatusNotFound {
			t.Errorf("%s %s while the companion is not served to alice: %d %s", route.method, route.path, a.status, a.raw)
		}
	}

	var logged strings.Builder
	s.warnings.mu.Lock()
	for i, message := range s.warnings.messages {
		logged.WriteString(message)
		for name, value := range s.warnings.attrs[i] {
			logged.WriteString(" " + name + "=" + value)
		}
		logged.WriteString("\n")
	}
	s.warnings.mu.Unlock()
	for _, secret := range []string{"4821", "7777", "2468", hash, strings.Split(hash, "$")[5]} {
		if strings.Contains(seen.String(), secret) {
			t.Errorf("an answer holds %q", secret)
		}
		// A PIN as a word of its own: an address's port may end with its
		// digits.
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(secret) + `\b`).MatchString(logged.String()) {
			t.Errorf("the log holds %q", secret)
		}
	}
}

// Past the free wrong guesses, each makes the next wait longer, and while
// it waits every guess, the right one or a current PIN included, answers
// 429 with Retry-After. A wrong current PIN counts as a guess. A right PIN
// once the wait is over, or a PIN an administrator sets, starts the count
// again; only an administrator may set another user's PIN.
func TestMoonfinPinGuessesWaitLongerEachTime(t *testing.T) {
	t.Parallel()
	s, users := moonfinServer(t)
	root, alice := app("dashboard", s.signIn("root", "dashboard")), app("phone", s.signIn("alice", "phone"))
	var seen strings.Builder
	call := func(method, path, auth string, body any) pinAnswer {
		t.Helper()
		return pinCall(t, s, &seen, method, path, auth, body)
	}
	verify := func(pin string) pinAnswer {
		t.Helper()
		return call(http.MethodPost, "/Moonfin/Pin/Verify", alice, map[string]string{"pin": pin})
	}
	waited := func(a pinAnswer, most int) {
		t.Helper()
		retry, err := strconv.Atoi(a.header.Get("Retry-After"))
		seconds, _ := a.body["retryAfterSeconds"].(float64)
		if a.status != http.StatusTooManyRequests || a.body["error"] != "Too many attempts" || err != nil ||
			retry != int(seconds) || retry <= 0 || retry > most {
			t.Errorf("not a wait of %d s at most: %d %q %s", most, a.status, a.header.Get("Retry-After"), a.raw)
		}
	}
	// The wait ends now, as if its time went by.
	endWait := func() {
		t.Helper()
		if _, err := s.pool.Exec(t.Context(), "UPDATE moonfin_pins SET wait_until = now() - interval '1 second' WHERE wait_until IS NOT NULL"); err != nil {
			t.Fatal(err)
		}
	}
	if a := call(http.MethodPut, "/Moonfin/Pin", alice, map[string]string{"pin": "4821"}); a.status != http.StatusOK {
		t.Fatalf("set: %d %s", a.status, a.raw)
	}
	for i := range 3 {
		if a := verify("0000"); a.status != http.StatusOK || a.body["valid"] != false {
			t.Fatalf("free guess %d: %d %s", i+1, a.status, a.raw)
		}
	}
	for i := range 2 {
		if a := call(http.MethodPut, "/Moonfin/Pin", alice, map[string]string{"pin": "1111", "currentPin": "0000"}); a.status != http.StatusForbidden {
			t.Fatalf("free guess %d as a current PIN: %d %s", i+4, a.status, a.raw)
		}
	}
	first := verify("0000")
	waited(first, 30)
	if first.header.Get("Retry-After") != "30" {
		t.Errorf("first wait: %q", first.header.Get("Retry-After"))
	}
	waited(verify("4821"), 30)
	waited(call(http.MethodPut, "/Moonfin/Pin", alice, map[string]string{"pin": "1111", "currentPin": "4821"}), 30)
	waited(call(http.MethodDelete, "/Moonfin/Pin", alice, map[string]string{"currentPin": "4821"}), 30)
	endWait()
	second := verify("0000")
	waited(second, 60)
	if second.header.Get("Retry-After") != "60" {
		t.Errorf("second wait: %q", second.header.Get("Retry-After"))
	}
	endWait()
	if a := verify("4821"); a.status != http.StatusOK || a.body["valid"] != true {
		t.Fatalf("the right PIN after the wait: %d %s", a.status, a.raw)
	}
	// The count started again: the free guesses are free again.
	for i := range 5 {
		if a := verify("0000"); a.status != http.StatusOK {
			t.Fatalf("free guess %d after a right PIN: %d %s", i+1, a.status, a.raw)
		}
	}
	waited(verify("0000"), 30)

	path := "/Moonfin/Users/" + users["alice"].ID.String() + "/Pin"
	if a := call(http.MethodPut, path, app("tv", s.signIn("bob", "tv")), map[string]string{"pin": "1357"}); a.status != http.StatusForbidden {
		t.Errorf("a member setting alice's PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPut, path, root, map[string]string{"pin": "13579"}); a.status != http.StatusBadRequest {
		t.Errorf("an administrator's PIN of five digits: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPut, "/Moonfin/Users/0123456789abcdef0123456789abcdef/Pin", root, map[string]string{"pin": "1357"}); a.status != http.StatusNotFound {
		t.Errorf("an unknown user: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodPut, path, root, map[string]string{"pin": "1357"}); a.status != http.StatusOK || a.body["enabled"] != true {
		t.Fatalf("an administrator setting alice's PIN: %d %s", a.status, a.raw)
	}
	if a := verify("1357"); a.status != http.StatusOK || a.body["valid"] != true {
		t.Errorf("the PIN the administrator set, at once: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodDelete, path, root, nil); a.status != http.StatusOK || a.body["enabled"] != false {
		t.Errorf("an administrator removing alice's PIN: %d %s", a.status, a.raw)
	}
	if a := call(http.MethodGet, "/Moonfin/Pin", alice, nil); a.body["enabled"] != false {
		t.Errorf("after the administrator removed it: %s", a.raw)
	}
	if strings.Contains(seen.String(), "4821") || strings.Contains(seen.String(), "1357") {
		t.Error("an answer holds a PIN")
	}
}

// A Moonfin app signed in through Silo's API reaches the same PIN under the
// plugin's content path.
func TestSiloAppsReachMoonfinPins(t *testing.T) {
	t.Parallel()
	s, _ := moonfinServer(t)
	allowSilo(t, s)
	a := servingSilo(t, s)
	token, profile := a.signIn(t, "alice")
	base := "/api/v2/plugin-content/plugins/moonfin"
	if status, data := a.do(t, http.MethodPut, base+"/Moonfin/Pin", token, profile, map[string]string{"pin": "4821"}); status != http.StatusOK {
		t.Fatalf("set through Silo: %d %s", status, data)
	}
	var verdict map[string]any
	if status, data := a.do(t, http.MethodPost, base+"/Moonfin/Pin/Verify", token, profile, map[string]string{"pin": "4821"}); status != http.StatusOK ||
		json.Unmarshal(data, &verdict) != nil || verdict["valid"] != true {
		t.Errorf("verified through Silo: %d %s", status, data)
	}
	jellyfin := app("phone", s.signIn("alice", "phone"))
	if _, answer := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Pin", jellyfin, nil); answer["enabled"] != true {
		t.Errorf("through the Jellyfin API: %v", answer)
	}
	if status, data := a.do(t, http.MethodDelete, base+"/Moonfin/Pin", token, profile, map[string]string{"currentPin": "4821"}); status != http.StatusOK {
		t.Errorf("removed through Silo: %d %s", status, data)
	}
	if _, answer := moonfinJSON(t, s, http.MethodGet, "/Moonfin/Pin", jellyfin, nil); answer["enabled"] != false {
		t.Errorf("through the Jellyfin API after the removal: %v", answer)
	}
}
