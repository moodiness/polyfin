// Package nuviotest serves a fake Nuvio backend for tests: its public
// configuration, the password sign-in, the account backup and the
// sign-out, as the official backend answers them.
package nuviotest

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Key is the publishable key the fake serves.
const Key = "sb_publishable_test"

// Request is a request the fake received.
type Request struct {
	Method, Path, Query string
	// APIKey and Bearer are the publishable key and the session's token
	// sent; Body is what was posted.
	APIKey, Bearer, Body string
}

// Backend is a fake Nuvio backend, with one account.
type Backend struct {
	URL string

	mu sync.Mutex
	// Email and Password sign in to the account, whose backup is Backup.
	Email, Password string
	Backup          any
	// Limited answers the sign-ins 429 that many times, asking to wait
	// RetryAfter.
	Limited    int
	RetryAfter string
	requests   []Request
	sessions   map[string]bool
	signIns    int
}

// New serves a fake backend until the test ends.
func New(t *testing.T) *Backend {
	t.Helper()
	b := &Backend{sessions: map[string]bool{}}
	server := httptest.NewServer(http.HandlerFunc(b.serve))
	t.Cleanup(server.Close)
	b.URL = server.URL
	return b
}

// Set changes the fake under its lock.
func (b *Backend) Set(change func(*Backend)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	change(b)
}

// Requests lists the requests the fake received.
func (b *Backend) Requests() []Request {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]Request(nil), b.requests...)
}

// Open counts the sessions signed in and not out.
func (b *Backend) Open() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.sessions)
}

func (b *Backend) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	b.mu.Lock()
	defer b.mu.Unlock()
	bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	b.requests = append(b.requests, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, APIKey: r.Header.Get("apikey"),
		Bearer: bearer, Body: string(body)})
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/.well-known/nuvio" && r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "service": "nuvio", "backend_url": b.URL, "publishable_key": Key,
			"capabilities": map[string]bool{"email_password_auth": true}})
		return
	}
	if r.Header.Get("apikey") != Key {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Invalid API key"}`)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/token" && r.URL.Query().Get("grant_type") == "password":
		if b.Limited > 0 {
			b.Limited--
			w.Header().Set("Retry-After", b.RetryAfter)
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		var credentials struct{ Email, Password string }
		if json.Unmarshal(body, &credentials) != nil || credentials.Email != b.Email || credentials.Password != b.Password {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":400,"error_code":"invalid_credentials","msg":"Invalid login credentials"}`)
			return
		}
		b.signIns++
		token := "access-token-" + strconv.Itoa(b.signIns) + "-0123456789abcdef"
		b.sessions[token] = true
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "bearer", "refresh_token": "refresh",
			"user": map[string]string{"email": b.Email}})
	case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/sync_export_account_backup":
		if !b.sessions[bearer] {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"code":"28000","message":"Unauthorized: valid session required for sync."}`)
			return
		}
		_ = json.NewEncoder(w).Encode(b.Backup)
	case r.Method == http.MethodPost && r.URL.Path == "/auth/v1/logout":
		delete(b.sessions, bearer)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Backup is a backup of version, holding data.
func Backup(version int, data map[string]any) map[string]any {
	counts := map[string]int{}
	for name, rows := range data {
		if list, ok := rows.([]map[string]any); ok {
			counts[name] = len(list)
		}
	}
	return map[string]any{"format": "nuvio_account_backup", "version": version, "scope": "account", "exported_at": "2026-10-12T10:00:00Z",
		"sensitive_data_excluded": []string{"auth_sessions", "tracker_tokens"}, "counts": counts, "data": data}
}

// ms is the epoch milliseconds of an RFC 3339 date.
func ms(date string) int64 {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil {
		panic(err)
	}
	return t.UnixMilli()
}

// Sample is the data of an account's backup with two profiles, Main and
// Kids, which uses Main's addons. Main lists the addons whose manifests
// are at first and under second (named by the address of their
// resources), and one turned off. It watched:
//   - the movie tt0000001, on 2026-09-01 at 20:00, which is in its library;
//   - the episode S1E2 of the series tt0100, on 2026-09-04 at 20:00, as
//     older apps dated it (20260904200000);
//   - the series tt0200 as a whole, without its episodes;
//   - the movies tmdb:999999 ("Unknown"), which no catalog lists, and
//     custom:1 ("Home video"), of an addon of its own.
//
// It stopped:
//   - the movie tt0000002 at 30 of 100 minutes, on 2026-10-01 at 20:00;
//   - S1E3 of tt0100 at 20 of 50 minutes, on 2026-10-02 at 20:00;
//   - S2E1 of tt0100, named by its video only, at 5 of 50 minutes, on
//     2026-10-03 at 20:00;
//   - S1E2 of tt0100, watched since, and srv1:abcd, a title of a server
//     Nuvio connects to.
//
// Its library holds tt0000001 and the series tt0100. Kids watched the
// movie tt0000009.
func Sample(first, second string) map[string]any {
	return map[string]any{
		"profiles": []map[string]any{
			{"profile_index": 1, "profile_id": 1, "name": "Main", "uses_primary_addons": false},
			{"profile_index": 2, "profile_id": 2, "name": "Kids", "uses_primary_addons": true},
		},
		"addons": []map[string]any{
			{"profile_id": 1, "url": first, "name": "First", "enabled": true, "sort_order": 0},
			{"profile_id": 1, "url": second, "name": "Second", "enabled": true, "sort_order": 1},
			{"profile_id": 1, "url": "https://off.example/manifest.json", "name": "Off", "enabled": false, "sort_order": 2},
		},
		"plugins": []map[string]any{},
		"library_items": []map[string]any{
			{"profile_id": 1, "content_id": "tt0100", "content_type": "series", "name": "Series", "added_at": ms("2026-08-01T20:00:00Z")},
			{"profile_id": 1, "content_id": "tt0000001", "content_type": "movie", "name": "First movie", "added_at": ms("2026-08-02T20:00:00Z")},
		},
		"watch_progress": []map[string]any{
			{"profile_id": 1, "content_id": "tt0000002", "content_type": "movie", "video_id": "tt0000002", "season": nil, "episode": nil,
				"position": 30 * 60_000, "duration": 100 * 60_000, "last_watched": ms("2026-10-01T20:00:00Z"), "progress_key": "tt0000002"},
			{"profile_id": 1, "content_id": "tt0100", "content_type": "series", "video_id": "tt0100:1:3", "season": 1, "episode": 3,
				"position": 20 * 60_000, "duration": 50 * 60_000, "last_watched": ms("2026-10-02T20:00:00Z"), "progress_key": "tt0100_s1e3"},
			{"profile_id": 1, "content_id": "tt0100", "content_type": "series", "video_id": "tt0100:2:1", "season": nil, "episode": nil,
				"position": 5 * 60_000, "duration": 50 * 60_000, "last_watched": ms("2026-10-03T20:00:00Z"), "progress_key": "tt0100_s2e1"},
			{"profile_id": 1, "content_id": "tt0100", "content_type": "series", "video_id": "tt0100:1:2", "season": 1, "episode": 2,
				"position": 40 * 60_000, "duration": 50 * 60_000, "last_watched": ms("2026-09-03T20:00:00Z"), "progress_key": "tt0100_s1e2"},
			{"profile_id": 1, "content_id": "srv1:abcd", "content_type": "movie", "video_id": "srv1:abcd", "season": nil, "episode": nil,
				"position": 10 * 60_000, "duration": 100 * 60_000, "last_watched": ms("2026-10-04T20:00:00Z"), "progress_key": "srv1:abcd"},
		},
		"watched_items": []map[string]any{
			{"profile_id": 1, "content_id": "tt0000001", "content_type": "movie", "title": "First movie", "season": nil, "episode": nil,
				"watched_at": ms("2026-09-01T20:00:00Z")},
			{"profile_id": 1, "content_id": "tt0100", "content_type": "series", "title": "Series", "season": 1, "episode": 2,
				"watched_at": int64(20260904200000)},
			{"profile_id": 1, "content_id": "tt0200", "content_type": "series", "title": "Whole", "season": nil, "episode": nil,
				"watched_at": ms("2026-09-05T20:00:00Z")},
			{"profile_id": 1, "content_id": "tmdb:999999", "content_type": "movie", "title": "Unknown", "season": nil, "episode": nil,
				"watched_at": ms("2026-09-06T20:00:00Z")},
			{"profile_id": 1, "content_id": "custom:1", "content_type": "movie", "title": "Home video", "season": nil, "episode": nil,
				"watched_at": ms("2026-09-07T20:00:00Z")},
			{"profile_id": 2, "content_id": "tt0000009", "content_type": "movie", "title": "Kids' movie", "season": nil, "episode": nil,
				"watched_at": ms("2026-09-08T20:00:00Z")},
		},
	}
}
