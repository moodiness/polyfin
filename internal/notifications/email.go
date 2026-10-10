package notifications

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/moodiness/polyfin/internal/accounts"
)

// sendEmail sends ev to the email target t through the SMTP server of the
// settings, once, before ctx ends. The server's answers are read as a web
// target's: 4xx asks to try again later; a refused sign-in, sender or
// recipient refuses Polyfin; another 5xx refuses the message.
func (s *Service) sendEmail(ctx context.Context, t target, ev Event) result {
	settings := s.accounts.Settings()
	if !settings.SMTPAvailable() {
		return result{outcome: retry, err: "no SMTP server"}
	}
	message, err := s.emailMessage(settings, t, ev)
	if err != nil {
		return result{outcome: rejected, err: "malformed message"}
	}
	conn, err := s.dialSMTP(ctx, settings)
	if err != nil {
		return smtpResult(err)
	}
	defer func() { _ = conn.Close() }()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	client, err := smtp.NewClient(conn, settings.SMTPHost)
	if err != nil {
		return smtpResult(err)
	}
	if settings.SMTPSecurity == "starttls" {
		// Without STARTTLS, nothing is sent: neither the password nor the
		// message go out in the clear when encryption was asked.
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return result{outcome: retry, err: "STARTTLS not offered"}
		}
		if err := client.StartTLS(s.smtpTLS(settings.SMTPHost)); err != nil {
			return smtpResult(err)
		}
	}
	if settings.SMTPUser != "" {
		if err := client.Auth(smtpAuth(client, settings)); err != nil {
			return smtpResult(err)
		}
	}
	if err := client.Mail(settings.SMTPFrom); err != nil {
		return smtpResult(err)
	}
	if err := client.Rcpt(t.address); err != nil {
		return smtpResult(err)
	}
	data, err := client.Data()
	if err != nil {
		return smtpResult(err)
	}
	if _, err := data.Write(message); err != nil {
		return smtpResult(err)
	}
	if err := data.Close(); err != nil {
		return smtpResult(err)
	}
	// The message is accepted: a failed goodbye changes nothing.
	_ = client.Quit()
	return result{outcome: delivered}
}

// dialSMTP connects to the SMTP server of settings, in TLS from the start
// when its security asks.
func (s *Service) dialSMTP(ctx context.Context, settings accounts.Settings) (net.Conn, error) {
	address := net.JoinHostPort(settings.SMTPHost, strconv.Itoa(settings.SMTPPort))
	dialer := &net.Dialer{}
	if settings.SMTPSecurity == "tls" {
		return (&tls.Dialer{NetDialer: dialer, Config: s.smtpTLS(settings.SMTPHost)}).DialContext(ctx, "tcp", address)
	}
	return dialer.DialContext(ctx, "tcp", address)
}

// smtpTLS is how the SMTP server host is checked: its certificate must
// name it.
func (s *Service) smtpTLS(host string) *tls.Config {
	return &tls.Config{ServerName: host, RootCAs: s.smtpRoots, MinVersion: tls.VersionTLS12}
}

// smtpAuth signs in with PLAIN, or with LOGIN when the server offers only
// that. Both refuse to send the password over a connection that is not
// encrypted, unless to this machine.
func smtpAuth(client *smtp.Client, settings accounts.Settings) smtp.Auth {
	_, mechanisms := client.Extension("AUTH")
	offered := strings.Fields(strings.ToUpper(mechanisms))
	if !slices.Contains(offered, "PLAIN") && slices.Contains(offered, "LOGIN") {
		return &loginAuth{user: settings.SMTPUser, password: settings.SMTPPassword, host: settings.SMTPHost}
	}
	return smtp.PlainAuth("", settings.SMTPUser, settings.SMTPPassword, settings.SMTPHost)
}

// loginAuth is the LOGIN mechanism: the user, then the password, each
// when the server asks.
type loginAuth struct {
	user, password, host string
	step                 int
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !loopback(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	a.step = 0
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(_ []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.user), nil
	case 2:
		return []byte(a.password), nil
	}
	return nil, errors.New("unexpected LOGIN challenge")
}

// loopback reports whether host names this machine.
func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// smtpResult reads what an SMTP server answered, or why it could not be
// reached, never with the password or the message.
func smtpResult(err error) result {
	var answer *textproto.Error
	if !errors.As(err, &answer) {
		var certificate *tls.CertificateVerificationError
		var unknown x509.UnknownAuthorityError
		var hostname x509.HostnameError
		if errors.As(err, &certificate) || errors.As(err, &unknown) || errors.As(err, &hostname) {
			return result{outcome: retry, err: "certificate not trusted"}
		}
		return result{outcome: retry, err: describeError(err)}
	}
	res := result{status: answer.Code, err: "SMTP " + strconv.Itoa(answer.Code)}
	switch {
	case answer.Code >= 400 && answer.Code < 500:
		// Busy, or sending too much: tried again later.
		res.outcome = retry
	case slices.Contains([]int{530, 534, 535, 538, 550, 551, 553}, answer.Code):
		// The sign-in, the sender or the recipient is refused.
		res.outcome = refused
	default:
		res.outcome = rejected
	}
	return res
}

// emailHTML is the HTML part of an email message.
var emailHTML = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html lang="{{.Language}}">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body style="font-family: system-ui, sans-serif; color: #1f2328;">
<h1 style="font-size: 18px;">{{.Title}}</h1>
<p>{{.Message}}</p>
{{if .URL}}<p><a href="{{.URL}}">{{.Open}}</a></p>
{{end}}<p style="color: #6b7280; font-size: 12px;">{{.Footer}}</p>
</body>
</html>
`))

// emailMessage writes ev as an email to the target t, in the server
// language: its title as the subject, its message and link in plain text
// and in HTML.
func (s *Service) emailMessage(settings accounts.Settings, t target, ev Event) ([]byte, error) {
	from := mail.Address{Name: settings.SMTPFromName, Address: settings.SMTPFrom}
	if from.Name == "" {
		from.Name = settings.ServerName
	}
	domain := settings.SMTPFrom[strings.LastIndexByte(settings.SMTPFrom, '@')+1:]
	url, open := "", s.phrase("Open", "Ouvrir")
	if ev.URL != nil {
		url = *ev.URL
	}
	footer := s.phrase("Sent by Polyfin from %s.", "Envoyé par Polyfin depuis %s.", settings.ServerName)

	var message bytes.Buffer
	body := multipart.NewWriter(&message)
	header := func(name, value string) { fmt.Fprintf(&message, "%s: %s\r\n", name, value) }
	header("From", from.String())
	header("To", (&mail.Address{Address: t.address}).String())
	header("Subject", mime.QEncoding.Encode("utf-8", oneLine(ev.Title)))
	header("Date", ev.At.Format(time.RFC1123Z))
	header("Message-ID", "<"+ev.ID+"."+t.id.String()+"@"+domain+">")
	header("Auto-Submitted", "auto-generated")
	header("X-Polyfin-Event", ev.Type)
	header("MIME-Version", "1.0")
	header("Content-Type", `multipart/alternative; boundary="`+body.Boundary()+`"`)
	message.WriteString("\r\n")

	text := ev.Message + "\n"
	if url != "" {
		text += "\n" + url + "\n"
	}
	text += "\n-- \n" + footer + "\n"
	var page bytes.Buffer
	if err := emailHTML.Execute(&page, map[string]string{"Language": settings.Language, "Title": ev.Title, "Message": ev.Message,
		"URL": url, "Open": open, "Footer": footer}); err != nil {
		return nil, err
	}
	for _, part := range []struct{ kind, content string }{{"text/plain", text}, {"text/html", page.String()}} {
		writer, err := body.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.kind + "; charset=utf-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		encoder := quotedprintable.NewWriter(writer)
		if _, err := encoder.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
	}
	if err := body.Close(); err != nil {
		return nil, err
	}
	return message.Bytes(), nil
}

// oneLine replaces the line breaks and other control characters of s with
// spaces, for a header.
func oneLine(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
