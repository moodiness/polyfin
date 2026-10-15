package totp

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

// rfcSecret is the SHA-1 seed of RFC 6238's test vectors,
// "12345678901234567890", in base32.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// The test vectors of RFC 6238, appendix B, for SHA-1, cut to 6 digits as
// RFC 4226 truncates them.
func TestCodesFollowRFC6238(t *testing.T) {
	for unix, want := range map[int64]string{
		59:          "287082",
		1111111109:  "081804",
		1111111111:  "050471",
		1234567890:  "005924",
		2000000000:  "279037",
		20000000000: "353130",
	} {
		if got, err := Code(rfcSecret, time.Unix(unix, 0)); err != nil || got != want {
			t.Errorf("at %d: %q %v, want %q", unix, got, err, want)
		}
	}
}

// A code matches within one step either way, not two, and only after the
// last step accepted.
func TestMatchAllowsOneStepOfDriftAndNoReplay(t *testing.T) {
	now := time.Unix(1111111111, 0)
	step := StepAt(now)
	for offset, want := range map[int64]bool{-2: false, -1: true, 0: true, 1: true, 2: false} {
		code, _ := Code(rfcSecret, now.Add(time.Duration(offset)*Step))
		matched, ok := Match(rfcSecret, code, now, 0)
		if ok != want || (ok && matched != step+offset) {
			t.Errorf("%d steps away: %d %v, want %v", offset, matched, ok, want)
		}
	}
	code, _ := Code(rfcSecret, now)
	if _, ok := Match(rfcSecret, code[:3]+" "+code[3:], now, 0); !ok {
		t.Error("a code written with a space is refused")
	}
	if _, ok := Match(rfcSecret, code, now, step); ok {
		t.Error("a code of the step already accepted matches again")
	}
	earlier, _ := Code(rfcSecret, now.Add(-Step))
	if _, ok := Match(rfcSecret, earlier, now, step); ok {
		t.Error("a code older than the step accepted matches")
	}
	later, _ := Code(rfcSecret, now.Add(Step))
	if matched, ok := Match(rfcSecret, later, now, step); !ok || matched != step+1 {
		t.Error("the next step's code is refused after this step's")
	}
	for _, wrong := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := Match(rfcSecret, wrong, now, 0); ok {
			t.Errorf("%q matches", wrong)
		}
	}
	if _, ok := Match("not base32!", code, now, 0); ok {
		t.Error("a broken secret matches")
	}
}

func TestNewSecretsHold160RandomBits(t *testing.T) {
	first, second := NewSecret(), NewSecret()
	key, err := decode(first)
	if err != nil || len(key) != 20 || first == second {
		t.Errorf("secrets %q %q: %d bytes, %v", first, second, len(key), err)
	}
}

func TestURI(t *testing.T) {
	uri := URI("Polyfin", "Ana María", rfcSecret)
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "otpauth" || parsed.Host != "totp" {
		t.Fatalf("%s: %v", uri, err)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/Polyfin:Ana%20Mar%C3%ADa?") {
		t.Errorf("label: %s", uri)
	}
	query := parsed.Query()
	if query.Get("secret") != rfcSecret || query.Get("issuer") != "Polyfin" || query.Get("algorithm") != "SHA1" ||
		query.Get("digits") != "6" || query.Get("period") != "30" {
		t.Errorf("parameters: %v", query)
	}
}
