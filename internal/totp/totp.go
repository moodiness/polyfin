// Package totp makes and checks the codes of authenticator apps: RFC 6238
// time-based one-time passwords with HMAC-SHA-1, a 30-second step and 6
// digits, the parameters every authenticator app reads.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// Step is how long a code lasts.
	Step = 30 * time.Second
	// Digits is the length of a code.
	Digits = 6
	// Drift is how many steps a code may be away from the current one,
	// either way, for a clock off by up to that much.
	Drift = 1
	// secretBytes is the length of a secret: 160 bits, the length of an
	// HMAC-SHA-1 output, as RFC 4226 recommends.
	secretBytes = 20
)

// ErrInvalidSecret reports a secret that is not base32.
var ErrInvalidSecret = errors.New("invalid TOTP secret")

// encoding writes secrets as authenticator apps read them: base32 without
// padding.
var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a new random secret, in base32.
func NewSecret() string {
	raw := make([]byte, secretBytes)
	_, _ = rand.Read(raw)
	return encoding.EncodeToString(raw)
}

// decode reads a secret in base32, letter case and spaces aside.
func decode(secret string) ([]byte, error) {
	key, err := encoding.DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil || len(key) == 0 {
		return nil, ErrInvalidSecret
	}
	return key, nil
}

// StepAt is the step a time falls in, counted from the Unix epoch.
func StepAt(at time.Time) int64 {
	return at.Unix() / int64(Step/time.Second)
}

// code is the code of a step, as RFC 4226 truncates it.
func code(key []byte, step int64) string {
	mac := hmac.New(sha1.New, key)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", Digits, value%1_000_000)
}

// Code is the code of secret at a time.
func Code(secret string, at time.Time) (string, error) {
	key, err := decode(secret)
	if err != nil {
		return "", err
	}
	return code(key, StepAt(at)), nil
}

// Match finds the step whose code is entered, within Drift steps of the
// one at, later than after, the last step a code was accepted for: a code
// is accepted once. Spaces in entered are ignored. It returns the step
// matched, and false when none matches.
func Match(secret, entered string, at time.Time, after int64) (int64, bool) {
	key, err := decode(secret)
	if err != nil {
		return 0, false
	}
	entered = strings.ReplaceAll(entered, " ", "")
	if len(entered) != Digits {
		return 0, false
	}
	now := StepAt(at)
	for step := now - Drift; step <= now+Drift; step++ {
		if step <= after {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(code(key, step)), []byte(entered)) == 1 {
			return step, true
		}
	}
	return 0, false
}

// URI is the otpauth:// address an authenticator app reads, from a QR code
// or pasted, for the account of user on the server issuer.
func URI(issuer, user, secret string) string {
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprint(Digits))
	query.Set("period", fmt.Sprint(int(Step/time.Second)))
	label := url.PathEscape(issuer) + ":" + url.PathEscape(user)
	return "otpauth://totp/" + label + "?" + query.Encode()
}
