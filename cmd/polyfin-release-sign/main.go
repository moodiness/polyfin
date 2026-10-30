// Command polyfin-release-sign writes and renews the signed manifest of a
// release, which Polyfin Desktop checks before it installs an update (see
// internal/desktop/update and docs/desktop.md). The release workflow runs
// it with the private key in POLYFIN_RELEASE_SIGNING_KEY.
//
//	polyfin-release-sign keygen -out FILE
//	    makes a key pair: the private key goes to FILE, readable by its
//	    owner only, and the public key is printed, for the keys the app
//	    carries. Nothing else ever prints the private key.
//	polyfin-release-sign manifest -version VERSION -out FILE PACKAGE...
//	    lists the size and SHA-256 of each package, issued now and valid
//	    for -valid, signed.
//	polyfin-release-sign renew -in FILE -out FILE
//	    checks the manifest's signature against the keys the app carries,
//	    whatever its expiry, then issues it again, valid for -valid.
//	polyfin-release-sign verify -in FILE [PACKAGE...]
//	    checks the manifest as the app does, and that each package given
//	    is listed with its size and SHA-256.
//	polyfin-release-sign key
//	    checks that the private key is one whose public key the app
//	    carries, and prints the public key.
//
// manifest and renew refuse a private key whose public key the app does
// not carry: the packages they vouch for would never install.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/moodiness/polyfin/internal/desktop/update"
)

// KeyVariable holds the private key: the 32-byte seed in base64, as
// keygen writes it.
const KeyVariable = "POLYFIN_RELEASE_SIGNING_KEY"

// defaultValidity is how long a manifest is valid: the scheduled renewal
// runs every week, so that three failed runs in a row still leave time.
const defaultValidity = 30 * 24 * time.Hour

func main() {
	keys, err := update.ReleaseKeys()
	if err == nil {
		err = run(os.Args[1:], os.Getenv, keys, time.Now().UTC().Truncate(time.Second), os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "polyfin-release-sign:", err)
		os.Exit(1)
	}
}

// usage is told for a missing or unknown command.
var usage = errors.New("usage: polyfin-release-sign keygen|manifest|renew|verify|key [flags], see -h of each")

// run runs the command of args, keys being those the app carries.
func run(args []string, env func(string) string, keys []ed25519.PublicKey, now time.Time, stdout io.Writer) error {
	if len(args) == 0 {
		return usage
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	out := flags.String("out", "", "the file to write")
	in := flags.String("in", "", "the manifest to read")
	version := flags.String("version", "", "the release's version, without the v")
	valid := flags.Duration("valid", defaultValidity, "how long the manifest is valid")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "keygen":
		return keygen(*out, stdout)
	case "key":
		key, err := signingKey(env, keys)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Polyfin Desktop carries the public key of %s: %s\n", KeyVariable, update.EncodeKey(key.Public().(ed25519.PublicKey)))
		return nil
	case "manifest":
		key, err := signingKey(env, keys)
		if err != nil {
			return err
		}
		if *version == "" || *out == "" || flags.NArg() == 0 {
			return errors.New("manifest needs -version, -out and the packages")
		}
		manifest := update.Manifest{Version: *version, Issued: now, Expires: now.Add(*valid)}
		for _, path := range flags.Args() {
			file, err := describe(path)
			if err != nil {
				return err
			}
			manifest.Files = append(manifest.Files, file)
		}
		return write(*out, manifest, key)
	case "renew":
		key, err := signingKey(env, keys)
		if err != nil {
			return err
		}
		if *in == "" || *out == "" {
			return errors.New("renew needs -in and -out")
		}
		data, err := os.ReadFile(*in)
		if err != nil {
			return err
		}
		manifest, err := update.Open(data, keys)
		if err != nil {
			return fmt.Errorf("%s: %w", *in, err)
		}
		manifest.Issued, manifest.Expires = now, now.Add(*valid)
		return write(*out, manifest, key)
	case "verify":
		if *in == "" {
			return errors.New("verify needs -in")
		}
		data, err := os.ReadFile(*in)
		if err != nil {
			return err
		}
		manifest, err := update.Verify(data, keys, now)
		if err != nil {
			return fmt.Errorf("%s: %w", *in, err)
		}
		for _, path := range flags.Args() {
			file, err := describe(path)
			if err != nil {
				return err
			}
			if listed, ok := manifest.File(file.Name); !ok || listed != file {
				return fmt.Errorf("%s is not listed as it is", file.Name)
			}
		}
		fmt.Fprintf(stdout, "The manifest of %s lists %d files, and expires on %s.\n", manifest.Version, len(manifest.Files),
			manifest.Expires.Format(time.RFC3339))
		return nil
	}
	return usage
}

// keygen writes a new private key to path, which must not exist, and
// prints its public key.
func keygen(path string, stdout io.Writer) error {
	if path == "" {
		return errors.New("keygen needs -out")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(file, base64.StdEncoding.EncodeToString(private.Seed()))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "The private key is in %s. Its public key:\n%s\n", path, update.EncodeKey(public))
	return nil
}

// signingKey reads the private key of KeyVariable, which must be one of
// keys'.
func signingKey(env func(string) string, keys []ed25519.PublicKey) (ed25519.PrivateKey, error) {
	text := env(KeyVariable)
	if text == "" {
		return nil, fmt.Errorf("%s is not set", KeyVariable)
	}
	key, err := update.ParsePrivateKey(text)
	if err != nil {
		return nil, err
	}
	public := key.Public().(ed25519.PublicKey)
	if !slices.ContainsFunc(keys, func(k ed25519.PublicKey) bool { return k.Equal(public) }) {
		return nil, fmt.Errorf("the public key of %s, %s, is not one of those Polyfin Desktop carries (internal/desktop/update/manifest.go)",
			KeyVariable, update.EncodeKey(public))
	}
	return key, nil
}

// describe is the manifest's entry of the file at path.
func describe(path string) (update.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return update.File{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return update.File{}, err
	}
	return update.File{Name: filepath.Base(path), Size: size, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

// write signs manifest with key into path.
func write(path string, manifest update.Manifest, key ed25519.PrivateKey) error {
	data, err := update.Sign(manifest, key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
