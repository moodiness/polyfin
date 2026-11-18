package admin

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/moodiness/polyfin/internal/notifications"
)

// mailSink is a plain SMTP server on the loopback that keeps the
// recipients of the messages it accepted.
type mailSink struct {
	port int
	mu   sync.Mutex
	to   []string
}

func newMailSink(t *testing.T) *mailSink {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	s := &mailSink{port: listener.Addr().(*net.TCPAddr).Port}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *mailSink) serve(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	reply := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	reply("220 sink ESMTP")
	recipient := ""
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(command, "RCPT TO"):
			recipient = strings.Trim(strings.TrimSpace(line)[len("RCPT TO:"):], "<> ")
			reply("250 OK")
		case command == "DATA":
			reply("354 go on")
			for {
				data, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if data == ".\r\n" {
					break
				}
			}
			s.mu.Lock()
			s.to = append(s.to, recipient)
			s.mu.Unlock()
			reply("250 queued")
		case command == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 OK")
		}
	}
}

// recipients lists who the messages accepted were for, in order.
func (s *mailSink) recipients() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.to...)
}

// "Send a test" is spaced, for members, administrators and the server's
// targets alike: a target tested a moment ago answers 429 test_too_soon,
// an owner's eleventh test of the hour 429 too_many_tests, both with
// Retry-After, and neither sends a message. One user's tests leave
// another's theirs.
func TestTestMessagesAreSpaced(t *testing.T) {
	t.Parallel()
	sink := newMailSink(t)
	api := newTestAPI(t, 10, func(o *Options, deps testDeps) {
		o.Notifications = notifications.New(notifications.Options{DB: deps.pool, Accounts: o.Accounts, Version: "1.2.3",
			Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		t.Cleanup(o.Notifications.Close)
	})
	settings := api.store.Settings()
	settings.SMTPHost, settings.SMTPPort, settings.SMTPSecurity, settings.SMTPFrom = "127.0.0.1", sink.port, "none", "polyfin@example.org"
	if _, err := api.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	administrator := api.signedIn("admin", true)
	member := api.signedIn("member", false)
	other := api.signedIn("other", false)
	// add adds an email target under path, and returns the path of its
	// test.
	add := func(b browser, path, address string, perMinute int) string {
		t.Helper()
		status, target, _ := b.call(http.MethodPost, path+"/notifications/targets",
			map[string]any{"kind": "email", "name": address, "address": address, "perMinute": perMinute})
		if status != http.StatusCreated {
			t.Fatalf("adding a target for %s: %d %v", address, status, target)
		}
		return path + "/notifications/targets/" + target["id"].(string) + "/test"
	}
	deliver := func(b browser, test string) {
		t.Helper()
		if status, body, _ := b.call(http.MethodPost, test, nil); status != http.StatusOK || body["delivered"] != true {
			t.Fatalf("%s: %d %v", test, status, body)
		}
	}
	refused := func(b browser, test, code string, longest int) {
		t.Helper()
		status, body, response := b.call(http.MethodPost, test, nil)
		wait, err := strconv.Atoi(response.Header.Get("Retry-After"))
		if status != http.StatusTooManyRequests || body["error"] != code || len(body) != 1 || err != nil || wait < 1 || wait > longest {
			t.Fatalf("%s: %d %v, Retry-After %q, want 429 %s and 1 to %d seconds", test, status, body, response.Header.Get("Retry-After"), code,
				longest)
		}
	}
	sunk := func(want int) {
		t.Helper()
		if to := sink.recipients(); len(to) != want {
			t.Fatalf("the SMTP server accepted %d messages, want %d: %v", len(to), want, to)
		}
	}

	// A member's target, with the strictest cap a target takes, asked
	// twenty times in a row: one message.
	first := add(member, "/account", "someone@elsewhere.example", 1)
	deliver(member, first)
	for range 19 {
		refused(member, first, "test_too_soon", 30)
	}
	sunk(1)
	// Another member's test is their own.
	deliver(other, add(other, "/account", "other@example.org", 0))
	// The member's other targets, until their tests of the hour are ten.
	for i := range notifications.MaxTestsPerHour - 1 {
		deliver(member, add(member, "/account", fmt.Sprintf("other%d@elsewhere.example", i), 0))
	}
	refused(member, add(member, "/account", "eleventh@elsewhere.example", 0), "too_many_tests", 3600)
	sunk(notifications.MaxTestsPerHour + 1)

	// An administrator's own target and the server's are spaced the same.
	for _, path := range []string{"/account", ""} {
		test := add(administrator, path, "admin@example.org", 0)
		deliver(administrator, test)
		refused(administrator, test, "test_too_soon", 30)
	}
	sunk(notifications.MaxTestsPerHour + 3)
	if to := sink.recipients(); to[0] != "someone@elsewhere.example" {
		t.Errorf("the first message was for %s", to[0])
	}
}
