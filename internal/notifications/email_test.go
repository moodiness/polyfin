package notifications

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/recordings"
)

// sunkMail is a message the SMTP sink received: who signed in, whether
// over TLS, its envelope and its data.
type sunkMail struct {
	user, from, to string
	tls            bool
	data           string
	at             time.Time
}

// smtpSink is an SMTP server that keeps what it receives. It offers
// STARTTLS on a plain connection, or speaks TLS from the start, and offers
// to sign in only once encrypted, with mechanisms, checking the user and
// password. Unless told otherwise, it accepts every message.
type smtpSink struct {
	t                    *testing.T
	listener             net.Listener
	certificate          tls.Certificate
	roots                *x509.CertPool
	implicit             bool
	mechanisms           string
	user, password, port string

	mu sync.Mutex
	// answers are the answers to the next messages' data, in order.
	answers []string
	mails   []sunkMail
	// tries counts the messages whose data was received, accepted or not,
	// and signIns the sign-ins tried.
	tries, signIns int
}

// newSMTPSink starts a sink, speaking TLS from the start when implicit,
// with httptest's certificate, which names 127.0.0.1.
func newSMTPSink(t *testing.T, implicit bool, mechanisms string) *smtpSink {
	t.Helper()
	certified := httptest.NewUnstartedServer(nil)
	certified.StartTLS()
	sink := &smtpSink{t: t, certificate: certified.TLS.Certificates[0], roots: x509.NewCertPool(), implicit: implicit,
		mechanisms: mechanisms, user: "polyfin", password: "smtp-very-secret-password"}
	sink.roots.AddCert(certified.Certificate())
	certified.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if implicit {
		listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{sink.certificate}})
	}
	sink.listener = listener
	_, sink.port, _ = net.SplitHostPort(listener.Addr().String())
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go sink.serve(conn)
		}
	}()
	return sink
}

// then has the sink answer the next messages' data with answers.
func (s *smtpSink) then(answers ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers = append(s.answers, answers...)
}

func (s *smtpSink) received() ([]sunkMail, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sunkMail{}, s.mails...), s.tries, s.signIns
}

// wait waits until n messages were accepted, and returns them.
func (s *smtpSink) wait(n int) []sunkMail {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		mails, _, _ := s.received()
		if len(mails) >= n {
			return mails
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the SMTP server received %d messages, want %d", len(mails), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *smtpSink) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_, encrypted := conn.(*tls.Conn)
	text := textproto.NewConn(conn)
	reply := func(lines ...string) {
		for _, line := range lines {
			_ = text.PrintfLine("%s", line)
		}
	}
	reply("220 sink ESMTP")
	var mail sunkMail
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		verb, argument, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			lines := []string{"250-sink"}
			if !encrypted {
				lines = append(lines, "250-STARTTLS")
			} else {
				lines = append(lines, "250-AUTH "+s.mechanisms)
			}
			reply(append(lines, "250 HELP")...)
		case "STARTTLS":
			reply("220 Ready to start TLS")
			secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.certificate}})
			if secured.Handshake() != nil {
				return
			}
			conn, encrypted, text = secured, true, textproto.NewConn(secured)
			mail = sunkMail{}
		case "AUTH":
			if !encrypted {
				reply("530 Must issue a STARTTLS command first")
				continue
			}
			mechanism, initial, _ := strings.Cut(argument, " ")
			var user, password string
			switch mechanism {
			case "PLAIN":
				decoded, _ := base64.StdEncoding.DecodeString(initial)
				parts := strings.Split(string(decoded), "\x00")
				if len(parts) == 3 {
					user, password = parts[1], parts[2]
				}
			case "LOGIN":
				for _, prompt := range []string{"VXNlcm5hbWU6", "UGFzc3dvcmQ6"} {
					reply("334 " + prompt)
					answer, err := text.ReadLine()
					if err != nil {
						return
					}
					decoded, _ := base64.StdEncoding.DecodeString(answer)
					if prompt == "VXNlcm5hbWU6" {
						user = string(decoded)
					} else {
						password = string(decoded)
					}
				}
			}
			s.mu.Lock()
			s.signIns++
			s.mu.Unlock()
			if !strings.Contains(" "+s.mechanisms+" ", " "+mechanism+" ") || user != s.user || password != s.password {
				reply("535 5.7.8 Authentication credentials invalid")
				continue
			}
			mail.user = user
			reply("235 2.7.0 Authentication successful")
		case "MAIL":
			mail.from = strings.Trim(strings.TrimPrefix(strings.Fields(argument)[0], "FROM:"), "<>")
			reply("250 OK")
		case "RCPT":
			mail.to = strings.Trim(strings.TrimPrefix(argument, "TO:"), "<>")
			reply("250 OK")
		case "DATA":
			reply("354 End data with <CR><LF>.<CR><LF>")
			data, err := io.ReadAll(text.DotReader())
			if err != nil {
				return
			}
			s.mu.Lock()
			s.tries++
			answer := "250 2.0.0 Queued"
			if len(s.answers) > 0 {
				answer, s.answers = s.answers[0], s.answers[1:]
			}
			if strings.HasPrefix(answer, "250") {
				mail.tls, mail.data, mail.at = encrypted, string(data), time.Now()
				s.mails = append(s.mails, mail)
			}
			s.mu.Unlock()
			reply(answer)
		case "QUIT":
			reply("221 Bye")
			return
		default:
			reply("250 OK")
		}
	}
}

// useSMTP sets the sink as the SMTP server of the settings, signing in with
// password.
func (h harness) useSMTP(t *testing.T, sink *smtpSink, password string) {
	t.Helper()
	settings := h.store.Settings()
	port, _ := strconv.Atoi(sink.port)
	settings.SMTPHost, settings.SMTPPort, settings.SMTPUser, settings.SMTPPassword = "127.0.0.1", port, sink.user, password
	settings.SMTPSecurity = "starttls"
	if sink.implicit {
		settings.SMTPSecurity = "tls"
	}
	settings.SMTPFrom, settings.SMTPFromName = "polyfin@example.org", "Home server"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	h.Service.smtpRoots = sink.roots
}

// An email target receives each message in the server language, in plain
// text and in HTML, through the SMTP server of the settings: signed in
// only once encrypted, with STARTTLS or TLS from the start, by PLAIN or by
// LOGIN. None can be added without an SMTP server.
func TestEmailTargetsReceiveMessagesInPlainTextAndHTML(t *testing.T) {
	h := newHarness(t)
	inbox := Draft{Kind: Email, Name: new("Inbox"), Address: new("sam@example.org"), Events: []string{RecordingFinished}}
	if _, err := h.Create(t.Context(), nil, inbox); err != ErrEmailUnavailable {
		t.Errorf("an email target without an SMTP server: %v", err)
	}
	sink := newSMTPSink(t, false, "PLAIN LOGIN")
	h.useSMTP(t, sink, sink.password)
	settings := h.store.Settings()
	settings.PublicAddress, settings.ServerName, settings.Language = "https://media.example.org", "Home", "fr"
	if _, err := h.store.UpdateSettings(t.Context(), settings); err != nil {
		t.Fatal(err)
	}
	target := h.add(t, nil, inbox)

	recording := accounts.ID{0x42}
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: recording, User: &h.member.ID, Channel: accounts.ID{9}, Name: "Late <Show>"})
	link := "https://media.example.org/web/#/details?id=" + recording.String() + "&serverId=fedcba9876543210fedcba9876543210"
	got := sink.wait(1)[0]
	if !got.tls || got.user != "polyfin" || got.from != "polyfin@example.org" || got.to != "sam@example.org" {
		t.Errorf("envelope: TLS %v, signed in as %q, from %q to %q", got.tls, got.user, got.from, got.to)
	}
	message, err := mail.ReadMessage(strings.NewReader(got.data))
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	from, _ := mail.ParseAddress(message.Header.Get("From"))
	date, dateErr := mail.ParseDate(message.Header.Get("Date"))
	if subject != "Enregistrement terminé : Late <Show>" || from == nil || from.Name != "Home server" || from.Address != "polyfin@example.org" ||
		dateErr != nil || time.Since(date) > time.Minute || !strings.HasSuffix(message.Header.Get("Message-Id"), "@example.org>") ||
		message.Header.Get("Auto-Submitted") != "auto-generated" || message.Header.Get("To") != "<sam@example.org>" {
		t.Errorf("headers: %v (subject %q)", message.Header, subject)
	}
	kind, params, _ := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if kind != "multipart/alternative" {
		t.Fatalf("content type: %s", message.Header.Get("Content-Type"))
	}
	parts := map[string]string{}
	reader := multipart.NewReader(message.Body, params["boundary"])
	for {
		part, err := reader.NextRawPart()
		if err != nil {
			break
		}
		kind, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		var body io.Reader = part
		if part.Header.Get("Content-Transfer-Encoding") == "quoted-printable" {
			body = quotedprintable.NewReader(part)
		}
		decoded, _ := io.ReadAll(body)
		parts[kind] = string(decoded)
	}
	if text := parts["text/plain"]; !strings.Contains(text, "Enregistré sur Channel Nine, pour member.") || !strings.Contains(text, link) {
		t.Errorf("plain text: %q", text)
	}
	page := parts["text/html"]
	if !strings.Contains(page, `<html lang="fr">`) || !strings.Contains(page, "Late &lt;Show&gt;") ||
		!strings.Contains(page, `href="`+strings.ReplaceAll(link, "&", "&amp;")+`"`) || strings.Contains(page, "<Show>") {
		t.Errorf("HTML: %q", page)
	}
	// The sink keeps the message as soon as it has it, before Polyfin records
	// the delivery.
	h.idle(t)
	targets, _ := h.Targets(t.Context(), nil)
	if targets[0].Problem != "" || targets[0].LastSentAt == nil || targets[0].Address != "sam@example.org" {
		t.Errorf("after delivering: %+v", targets[0])
	}

	// TLS from the start, signing in by LOGIN, the only mechanism offered.
	login := newSMTPSink(t, true, "LOGIN")
	h.useSMTP(t, login, login.password)
	if result, err := h.Test(t.Context(), nil, target.ID); err != nil || !result.Delivered {
		t.Fatalf("a test through TLS: %+v, %v", result, err)
	}
	if got := login.wait(1)[0]; !got.tls || got.user != "polyfin" || !strings.Contains(got.data, "Message de test") {
		t.Errorf("through TLS: TLS %v, signed in as %q", got.tls, got.user)
	}
}

// An SMTP server refusing the password refuses Polyfin: the target shows
// Refused with its code, and the message is not tried again. One asking
// to wait, as when too many messages are sent, gets the message again
// later. The password is never logged.
func TestEmailRefusedSignInAndBusyServer(t *testing.T) {
	h := newHarness(t)
	sink := newSMTPSink(t, false, "PLAIN")
	h.useSMTP(t, sink, "smtp-wrong-password")
	target := h.add(t, nil, Draft{Kind: Email, Name: new("Inbox"), Address: new("sam@example.org"), Events: []string{RecordingFinished}})

	result, err := h.Test(t.Context(), nil, target.ID)
	if err != nil || result.Delivered || result.Status != 535 || result.Target.Problem != ProblemRefused {
		t.Errorf("a refused password: %+v, %v", result, err)
	}
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{1}, User: &h.member.ID, Name: "Show"})
	time.Sleep(100 * time.Millisecond)
	h.idle(t)
	if mails, tries, signIns := sink.received(); len(mails) != 0 || tries != 0 || signIns != 2 {
		t.Errorf("with a refused password: %d messages, %d tries, %d sign-ins, want 2", len(mails), tries, signIns)
	}
	targets, _ := h.Targets(t.Context(), nil)
	if targets[0].Problem != ProblemRefused || targets[0].ProblemStatus == nil || *targets[0].ProblemStatus != 535 {
		t.Errorf("after a message with a refused password: %+v", targets[0])
	}

	h.useSMTP(t, sink, sink.password)
	sink.then("451 4.7.1 Too many messages, slow down")
	h.RecordingEnded(t.Context(), recordings.Ended{Recording: accounts.ID{2}, User: &h.member.ID, Name: "Show"})
	sink.wait(1)
	h.idle(t)
	if _, tries, _ := sink.received(); tries != 2 {
		t.Errorf("%d tries, want 2", tries)
	}
	targets, _ = h.Targets(t.Context(), nil)
	if targets[0].Problem != "" || targets[0].LastSentAt == nil {
		t.Errorf("after the server took it: %+v", targets[0])
	}
	if log := h.log.String(); strings.Contains(log, "smtp-wrong-password") || strings.Contains(log, sink.password) {
		t.Errorf("the log holds the password: %s", log)
	}
}
