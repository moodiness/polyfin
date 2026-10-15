package admin

import (
	"errors"
	"strings"
	"time"

	"github.com/moodiness/polyfin/internal/accounts"
	"github.com/moodiness/polyfin/internal/https"
	"github.com/moodiness/polyfin/internal/notifications"
)

// httpsProblems are the problems of HTTPS System › Health shows, as status
// tells them at now: a port it cannot listen on, certificate files it
// cannot read, a failed attempt with Let's Encrypt, and a certificate that
// ends within https.ExpiresSoon, an error once it ended. HTTP keeps
// working through all of them.
func httpsProblems(status https.Status, now time.Time) []healthProblemJSON {
	if status.Mode == https.ModeOff || status.Mode == "" {
		return nil
	}
	var problems []healthProblemJSON
	warning := func(key, code, anchor string) healthProblemJSON {
		return healthProblemJSON{Key: "https:" + key, Code: code, Tone: notifications.SeverityWarning, To: "/settings/security#" + anchor}
	}
	if status.PortFailed {
		p := warning("port", problemHTTPSPort, "https-port")
		p.Port = status.Port
		problems = append(problems, p)
	}
	if status.Files != "" {
		p := warning("files", problemHTTPSFiles, "https-certificate")
		p.Failure, p.File = status.Files, status.File
		problems = append(problems, p)
	}
	if status.Renewal != "" {
		p := warning("renewal", problemHTTPSRenewal, "https")
		p.Error = status.Renewal
		problems = append(problems, p)
	}
	if status.ExpiresSoon(now) {
		p := warning("expiry", problemHTTPSExpiry, "https")
		p.Expires = new(status.NotAfter)
		if !now.Before(status.NotAfter) {
			p.Tone = notifications.SeverityError
		}
		problems = append(problems, p)
	}
	return problems
}

// unreadableCertificate checks the certificate files next serves HTTPS
// with, against the current settings: files chosen anew, or other paths,
// must be read, and the key must be the certificate's. Saving the same
// files again checks nothing, so that a renewal under way elsewhere never
// stops other settings from being saved: Polyfin reads them again when
// they change, and System › Health tells what fails. It returns the error
// code of what fails, empty when nothing does; paths that are not
// absolute are left to UpdateSettings to refuse.
func (h *handler) unreadableCertificate(next, current accounts.Settings) string {
	if next.HTTPSMode != accounts.HTTPSFiles {
		return ""
	}
	certificate, okCertificate := accounts.CleanFolder(strings.TrimSpace(next.HTTPSCertificate))
	key, okKey := accounts.CleanFolder(strings.TrimSpace(next.HTTPSPrivateKey))
	if !okCertificate || !okKey || certificate == "" || key == "" {
		return ""
	}
	if current.HTTPSMode == accounts.HTTPSFiles && current.HTTPSCertificate == certificate && current.HTTPSPrivateKey == key {
		return ""
	}
	_, err := https.LoadFiles(certificate, key)
	if err == nil {
		return ""
	}
	var problem *https.FilesError
	if !errors.As(err, &problem) {
		return "https_certificate_unreadable"
	}
	if problem.Problem == https.FilesMismatch {
		return "https_files_mismatch"
	}
	file := "https_certificate"
	if problem.File == key && problem.File != certificate {
		file = "https_private_key"
	}
	if problem.Problem == https.FilesMissing {
		return file + "_missing"
	}
	return file + "_unreadable"
}
