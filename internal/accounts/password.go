package accounts

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters from the OWASP password storage recommendations:
// 19 MiB of memory, two passes, one lane.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// passwordCost is the cost of the hashes made: OWASP's, except in the
// test binaries that lower it (see LowerPasswordCostForTests). A hash
// records its own, which verifyPassword reads.
var passwordCost = argonCost{memory: argonMemory, time: argonTime, threads: argonThreads}

// argonCost is the cost of an Argon2id hash: its memory in KiB, its
// passes and its lanes.
type argonCost struct {
	memory, time uint32
	threads      uint8
}

// LowerPasswordCostForTests hashes the passwords of this test binary at a
// small fraction of OWASP's cost: every test user and every sign-in hashes
// one, and a hash at the server's cost takes a quarter of a second under
// the race detector. Called from a TestMain, before any test runs. The
// hashes made then still verify as the server's do, since each records
// its cost. It panics outside a test binary: the server always hashes at
// OWASP's cost.
func LowerPasswordCostForTests() {
	if !testing.Testing() {
		panic("accounts: LowerPasswordCostForTests called outside a test binary")
	}
	passwordCost = argonCost{memory: 64, time: 1, threads: 1}
	unknownUserHash = hashPassword("polyfin-unknown-user")
}

var errMalformedHash = errors.New("malformed password hash")

// hashPassword returns a PHC-formatted Argon2id hash, at passwordCost.
func hashPassword(password string) string {
	return hashAt(passwordCost, password)
}

// hashAt returns a PHC-formatted Argon2id hash of password at cost.
func hashAt(cost argonCost, password string) string {
	salt := make([]byte, argonSaltLen)
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, cost.time, cost.memory, cost.threads, argonKeyLen)
	encode := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, cost.memory, cost.time, cost.threads, encode(salt), encode(key))
}

// verifyPassword checks password against a hash produced by hashPassword,
// honoring the parameters recorded in the hash.
func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errMalformedHash
	}
	var version int
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errMalformedHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// unknownUserHash is verified when a name matches no account, so a failed
// sign-in takes as long whether or not the account exists.
var unknownUserHash = hashPassword("polyfin-unknown-user")
