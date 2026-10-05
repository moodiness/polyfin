package trackers

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTraktCodeConnectsOnceEnteredAndKeepsTheConnectionWhenDenied(t *testing.T) {
	h := newHarness(t)
	alice := h.user(t, "alice")
	h.f.reply("POST", "/trakt/oauth/device/code", http.StatusOK,
		`{"device_code":"device-1","user_code":"ABCD1234","verification_url":"https://trakt.example/activate","expires_in":600,"interval":1}`)
	var polls atomic.Int32
	h.f.on("POST", "/trakt/oauth/device/token", func(r request) answer {
		// Pending twice, then entered.
		if polls.Add(1) <= 2 {
			return answer{status: http.StatusBadRequest, body: `{}`}
		}
		return answer{status: http.StatusOK, body: `{"access_token":"trakt-access-1","token_type":"bearer","expires_in":604800,
			"refresh_token":"trakt-refresh-1","scope":"public","created_at":1791201600}`}
	})
	h.f.reply("GET", "/trakt/users/settings", http.StatusOK, `{"user":{"username":"alice-on-trakt","name":"Alice"}}`)

	status, err := h.StartCode(t.Context(), alice, Trakt)
	if err != nil {
		t.Fatal(err)
	}
	if status.Connected || status.Code == nil || status.Code.UserCode != "ABCD1234" || status.Code.VerificationURL != "https://trakt.example/activate" ||
		!status.Code.ExpiresAt.Equal(clockStart.Add(600e9)) {
		t.Fatalf("waiting: %+v %+v", status, status.Code)
	}
	eventually(t, "the connection", func() bool { return h.status(t, alice, Trakt).Connected })
	status = h.status(t, alice, Trakt)
	if status.Code != nil || status.Account != "alice-on-trakt" || status.Problem != "" || !status.ConnectedAt.Equal(clockStart) {
		t.Errorf("connected: %+v", status)
	}
	poll := h.f.sent("trakt", "/oauth/device/token")[0]
	sameJSON(t, "poll", poll.body, `{"code":"device-1","client_id":"trakt-client","client_secret":"trakt-secret"}`)
	if settings := h.f.sent("trakt", "/users/settings")[0]; settings.header.Get("Authorization") != "Bearer trakt-access-1" ||
		settings.header.Get("trakt-api-key") != "trakt-client" || settings.header.Get("trakt-api-version") != "2" {
		t.Errorf("account asked with %v", settings.header)
	}
	c, _, _ := h.connection(t.Context(), alice, Trakt)
	if c.token != "trakt-access-1" || c.refresh != "trakt-refresh-1" || c.expires == nil || c.expires.Unix() != 1791201600+604800 {
		t.Errorf("saved: %+v", c)
	}

	// A new code the user denies leaves the connection as it was.
	h.f.reply("POST", "/trakt/oauth/device/token", 418, `{}`)
	if status, err := h.StartCode(t.Context(), alice, Trakt); err != nil || status.Code == nil || !status.Connected {
		t.Fatalf("second code: %+v %v", status, err)
	}
	eventually(t, "the denied code going", func() bool { return h.status(t, alice, Trakt).Code == nil })
	if status := h.status(t, alice, Trakt); !status.Connected || status.Account != "alice-on-trakt" {
		t.Errorf("after a denied code: %+v", status)
	}
	// Tokens never reach the log.
	if log := h.log.String(); strings.Contains(log, "trakt-access") || strings.Contains(log, "trakt-refresh") || strings.Contains(log, "device-1") {
		t.Errorf("log: %s", log)
	}
}

func TestSimklCodeExpiresOrConnectsWithWriteAccess(t *testing.T) {
	h := newHarness(t)
	bob := h.user(t, "bob")
	h.f.reply("POST", "/simkl/oauth2/device", http.StatusOK, `{"device_code":"simkl-device","user_code":"WXYZ-2345",
		"verification_uri":"https://simkl.example/pin","verification_uri_complete":"https://simkl.example/pin?user_code=WXYZ-2345",
		"expires_in":900,"interval":1}`)
	var polls atomic.Int32
	h.f.on("POST", "/simkl/oauth2/token", func(r request) answer {
		if polls.Add(1) == 1 {
			return answer{status: http.StatusBadRequest, body: `{"error":"authorization_pending"}`}
		}
		return answer{status: http.StatusBadRequest, body: `{"error":"expired_token"}`}
	})
	status, err := h.StartCode(t.Context(), bob, Simkl)
	if err != nil || status.Code == nil || status.Code.UserCode != "WXYZ-2345" || status.Code.VerificationURL != "https://simkl.example/pin?user_code=WXYZ-2345" {
		t.Fatalf("code: %+v %v", status, err)
	}
	eventually(t, "the expired code going", func() bool { return h.status(t, bob, Simkl).Code == nil })
	if h.status(t, bob, Simkl).Connected {
		t.Error("connected with an expired code")
	}
	device := h.f.sent("simkl", "/oauth2/device")[0]
	if device.form.Get("client_id") != "simkl-client" || device.form.Get("scope") != "media:read media:write" ||
		device.query.Get("app-name") != "polyfin" || device.query.Get("app-version") != "1.2.3" {
		t.Errorf("code asked with %v %v", device.form, device.query)
	}
	if poll := h.f.sent("simkl", "/oauth2/token")[0]; poll.form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" ||
		poll.form.Get("device_code") != "simkl-device" || poll.form.Get("client_id") != "simkl-client" {
		t.Errorf("polled with %v", poll.form)
	}

	// A token that cannot write is no use.
	h.f.reply("POST", "/simkl/oauth2/token", http.StatusOK, `{"access_token":"simkl_at_read","token_type":"Bearer","expires_in":604800,
		"refresh_token":"simkl_rt_read","scope":"media:read"}`)
	if _, err := h.StartCode(t.Context(), bob, Simkl); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the read-only code going", func() bool { return h.status(t, bob, Simkl).Code == nil })
	if h.status(t, bob, Simkl).Connected {
		t.Error("connected read-only")
	}

	h.f.reply("POST", "/simkl/oauth2/token", http.StatusOK, `{"access_token":"simkl_at_write","token_type":"Bearer","expires_in":604800,
		"refresh_token":"simkl_rt_write","scope":"media:read media:write"}`)
	h.f.reply("GET", "/simkl/users/settings", http.StatusOK, `{"user":{"name":"bob-on-simkl"},"account":{"type":"free"}}`)
	if _, err := h.StartCode(t.Context(), bob, Simkl); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the connection", func() bool { return h.status(t, bob, Simkl).Connected })
	if status := h.status(t, bob, Simkl); status.Account != "bob-on-simkl" || status.Code != nil {
		t.Errorf("connected: %+v", status)
	}
}

func TestCodeServicesNeedTheirApp(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "carol")
	settings := h.store.Settings()
	settings.TraktClientSecret, settings.SimklClientID = "", ""
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{Trakt, Simkl} {
		if _, err := h.StartCode(t.Context(), user, service); !errors.Is(err, ErrNotAvailable) {
			t.Errorf("%s: %v", service, err)
		}
		if status := h.status(t, user, service); status.Available {
			t.Errorf("%s available", service)
		}
	}
	if status := h.status(t, user, MDBList); !status.Available || status.ByCode {
		t.Errorf("MDBList: %+v", status)
	}
	// A service that does not answer.
	settings.TraktClientSecret = "trakt-secret"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.f.reply("POST", "/trakt/oauth/device/code", http.StatusServiceUnavailable, `{}`)
	if _, err := h.StartCode(t.Context(), user, Trakt); !errors.Is(err, ErrUnreachable) {
		t.Errorf("unreachable: %v", err)
	}
}

func TestKeysAreCheckedWithTheService(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "dave")
	h.f.on("GET", "/mdblist/user", func(r request) answer {
		switch r.query.Get("apikey") {
		case "mdblist-good":
			return answer{status: http.StatusOK, body: `{"user_id":3,"username":"dave-on-mdblist","rate_limit":1000}`}
		case "mdblist-down":
			return answer{status: http.StatusBadGateway, body: `{}`}
		}
		return answer{status: http.StatusUnauthorized, body: `{"error":"Invalid API key"}`}
	})
	h.f.on("GET", "/publicmetadb/api/external/lists", func(r request) answer {
		switch r.header.Get("Authorization") {
		case "Bearer pm-good":
			return answer{status: http.StatusOK, body: `{"items":[],"total":0,"page":1,"perPage":1,"totalPages":0}`}
		case "Bearer pm-down":
			return answer{status: http.StatusInternalServerError, body: `{}`}
		}
		return answer{status: http.StatusUnauthorized, body: `{"error":"Invalid API key"}`}
	})
	for _, check := range []struct {
		service, key string
		err          error
	}{
		{MDBList, "mdblist-bad", ErrInvalidKey},
		{MDBList, "mdblist-down", ErrUnreachable},
		{PublicMetaDB, "pm-bad", ErrInvalidKey},
		{PublicMetaDB, "pm-down", ErrUnreachable},
		{MDBList, "", ErrInvalidKey},
		{MDBList, "with space", ErrInvalidKey},
		{PublicMetaDB, strings.Repeat("k", 257), ErrInvalidKey},
		{Trakt, "trakt-key", ErrUnknownService},
		{"other", "key", ErrUnknownService},
	} {
		if _, err := h.ConnectKey(t.Context(), user, check.service, check.key); !errors.Is(err, check.err) {
			t.Errorf("%s %q: %v, want %v", check.service, check.key, err, check.err)
		}
	}
	if n := len(h.f.sent("mdblist", "/user")) + len(h.f.sent("publicmetadb", "/api/external/lists")); n != 4 {
		t.Errorf("malformed keys were sent: %d requests", n)
	}
	for _, service := range []string{MDBList, PublicMetaDB} {
		if h.status(t, user, service).Connected {
			t.Errorf("%s connected with a refused key", service)
		}
	}

	status, err := h.ConnectKey(t.Context(), user, MDBList, "  mdblist-good\n")
	if err != nil || !status.Connected || status.Account != "dave-on-mdblist" || status.Problem != "" {
		t.Fatalf("MDBList: %+v %v", status, err)
	}
	status, err = h.ConnectKey(t.Context(), user, PublicMetaDB, "pm-good")
	if err != nil || !status.Connected || status.Account != "" {
		t.Fatalf("PublicMetaDB: %+v %v", status, err)
	}
	if lists := h.f.sent("publicmetadb", "/api/external/lists"); lists[len(lists)-1].query.Get("apikey") != "" {
		t.Error("PublicMetaDB's key was sent in the query")
	}
	if log := h.log.String(); strings.Contains(log, "mdblist-good") || strings.Contains(log, "pm-good") {
		t.Errorf("log: %s", log)
	}
}

func TestDisconnectingRevokesAndForgets(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "erin")
	h.connected(t, user, Trakt, "trakt-erin")
	h.connected(t, user, Simkl, "simkl-erin")
	h.connected(t, user, MDBList, "mdblist-erin")
	// A change waits for Trakt, which is down.
	h.f.reply("POST", "/trakt/sync/history", http.StatusServiceUnavailable, `{}`)
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 1, "trakt", "/sync/history")

	for _, service := range []string{Trakt, Simkl, MDBList} {
		if err := h.Disconnect(t.Context(), user, service); err != nil {
			t.Fatal(err)
		}
		if h.status(t, user, service).Connected {
			t.Errorf("%s still connected", service)
		}
	}
	// Disconnecting what is not connected is no error.
	if err := h.Disconnect(t.Context(), user, PublicMetaDB); err != nil {
		t.Error(err)
	}
	revoke := h.f.wait(t, 1, "trakt", "/oauth/revoke")[0]
	sameJSON(t, "Trakt revocation", revoke.body, `{"token":"trakt-erin","client_id":"trakt-client","client_secret":"trakt-secret"}`)
	if revoke := h.f.wait(t, 1, "simkl", "/oauth2/revoke")[0]; revoke.form.Get("token") != "simkl-erin-refresh" || revoke.form.Get("client_id") != "simkl-client" {
		t.Errorf("Simkl revocation: %v", revoke.form)
	}
	var queued int
	if err := h.db.QueryRow(t.Context(), "SELECT count(*) FROM tracking_events").Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%d changes still queued: %v", queued, err)
	}
	h.idle(t)
}

func TestDeletingAUserDeletesTheirConnections(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "frank")
	h.connected(t, user, MDBList, "mdblist-frank")
	h.f.reply("POST", "/mdblist/sync/watched", http.StatusServiceUnavailable, `{}`)
	h.Mark(user, Mark{Played: true, Titles: titles(movie)})
	h.f.wait(t, 1, "mdblist", "/sync/watched")
	if err := h.store.DeleteUser(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	var connections, queued int
	if err := h.db.QueryRow(t.Context(), "SELECT (SELECT count(*) FROM tracking_connections), (SELECT count(*) FROM tracking_events)").
		Scan(&connections, &queued); err != nil || connections != 0 || queued != 0 {
		t.Errorf("%d connections and %d changes left: %v", connections, queued, err)
	}
	h.idle(t)
}

func TestARefusedAppIsNotAnOutage(t *testing.T) {
	h := newHarness(t)
	user := h.user(t, "zoe")
	// Asking for a code with an app Trakt does not know.
	h.f.reply("POST", "/trakt/oauth/device/code", http.StatusUnauthorized, `{"error":"invalid_client","error_description":"client not found"}`)
	if _, err := h.StartCode(t.Context(), user, Trakt); !errors.Is(err, ErrAppRefused) {
		t.Errorf("Trakt: %v", err)
	}

	// A wrong secret shows only once the user entered the code: the code
	// ends, and the service shows the app refused, connected or not.
	h.connected(t, user, Trakt, "trakt-zoe")
	h.f.reply("POST", "/trakt/oauth/device/code", http.StatusOK,
		`{"device_code":"device-2","user_code":"EFGH5678","verification_url":"https://trakt.example/activate","expires_in":600,"interval":1}`)
	h.f.reply("POST", "/trakt/oauth/device/token", http.StatusUnauthorized, `{"error":"invalid_client","error_description":"client not found"}`)
	if _, err := h.StartCode(t.Context(), user, Trakt); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Trakt refusing the app", func() bool { return h.status(t, user, Trakt).Problem == ProblemAppRefused })
	if status := h.status(t, user, Trakt); status.Code != nil || !status.Connected {
		t.Errorf("Trakt: %+v", status)
	}
	// Fixing the app's settings clears it.
	settings := h.store.Settings()
	settings.TraktClientSecret = "fixed-secret"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	if status := h.status(t, user, Trakt); status.Problem != "" {
		t.Errorf("after the settings changed: %+v", status)
	}

	// Simkl refuses an AUTH V1 client ID on its token endpoint the same
	// way; the next attempt clears it.
	h.f.reply("POST", "/simkl/oauth2/device", http.StatusOK, `{"device_code":"simkl-device","user_code":"WXYZ-2345",
		"verification_uri":"https://simkl.example/pin","expires_in":900,"interval":1}`)
	h.f.reply("POST", "/simkl/oauth2/token", http.StatusUnauthorized, `{"error":"invalid_client","error_description":"Unknown or missing client_id"}`)
	if _, err := h.StartCode(t.Context(), user, Simkl); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Simkl refusing the app", func() bool { return h.status(t, user, Simkl).Problem == ProblemAppRefused })
	if status := h.status(t, user, Simkl); status.Connected || status.Code != nil {
		t.Errorf("Simkl: %+v", status)
	}
	h.f.reply("POST", "/simkl/oauth2/token", http.StatusBadRequest, `{"error":"authorization_pending"}`)
	if status, err := h.StartCode(t.Context(), user, Simkl); err != nil || status.Problem != "" || status.Code == nil {
		t.Errorf("trying again: %+v %v", status, err)
	}
	h.f.reply("POST", "/simkl/oauth2/device", http.StatusUnauthorized, `{"error":"invalid_client","error_description":"Unknown or missing client_id"}`)
	if _, err := h.StartCode(t.Context(), user, Simkl); !errors.Is(err, ErrAppRefused) {
		t.Errorf("Simkl: %v", err)
	}
	if strings.Contains(h.log.String(), "trakt-secret") || strings.Contains(h.log.String(), "simkl-client") {
		t.Errorf("log: %s", h.log.String())
	}
}
