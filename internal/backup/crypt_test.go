package backup

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"testing"
)

// serverKeyCost is the key's cost the package starts with, the server's,
// before TestMain lowers it.
var serverKeyCost int

// The tests derive their keys at a small cost (see LowerKeyCostForTests).
func TestMain(m *testing.M) {
	serverKeyCost = keyCost
	LowerKeyCostForTests()
	m.Run()
}

const testPassphrase = "correct horse battery"

// encryptBytes encrypts data with passphrase as a backup is.
func encryptBytes(t *testing.T, data []byte, passphrase string) []byte {
	t.Helper()
	var sealed bytes.Buffer
	w, err := newEncrypter(&sealed, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	// Written in pieces that straddle the chunks, as pg_dump writes.
	for piece := range slices.Chunk(data, 7000) {
		if _, err := w.Write(piece); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return sealed.Bytes()
}

// decryptBytes reads the whole dump of the encrypted backup sealed.
func decryptBytes(sealed []byte, passphrase string) ([]byte, error) {
	r, err := newDecrypter(bytes.NewReader(sealed), passphrase)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	return data
}

// sealedAtTheServersCost is a backup of "PGDMP sealed before the test
// cost\n" that Polyfin sealed with testPassphrase, at its server's cost,
// before the tests could lower it.
const sealedAtTheServersCost = "5046424b454e43013af78a2c19670390433ca1688434e1e3171b3f8278f39437" +
	"aab4329cfe5ade1480e9f804a25817a8316341ec85ff2f40abe6025b352dbe2c" +
	"b6da7bc58edc47b6750b3c58b54b356ea8026409786a57d197249176e45259dd" +
	"a7909403014fa64457aa"

// The server derives its keys at the cost the backups it sealed before
// were sealed at, which the tests' lower cost does not change: a backup
// sealed then still opens with the cost the package starts with, and
// not with the tests' one.
func TestTheServerKeepsItsKeyCost(t *testing.T) {
	t.Parallel()
	sealed, err := hex.DecodeString(sealedAtTheServersCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decryptBytes(sealed, testPassphrase); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("opened at the tests' cost: %v", err)
	}
	r, err := newDecrypterAt(bytes.NewReader(sealed), testPassphrase, serverKeyCost)
	if err != nil {
		t.Fatalf("opened at the server's cost: %v", err)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != "PGDMP sealed before the test cost\n" {
		t.Fatalf("opened at the server's cost: %q, %v", got, err)
	}
}

// A dump encrypted reads back whole: empty, of whole chunks, or ending
// with a short one; its size is told from the encrypted size, and nothing
// of it is in the clear.
func TestAnEncryptedBackupReadsBack(t *testing.T) {
	t.Parallel()
	for _, size := range []int{0, chunkSize, 2*chunkSize + 7} {
		data := randomBytes(t, size)
		sealed := encryptBytes(t, data, testPassphrase)
		if !encryptedHeader(sealed) {
			t.Errorf("%d bytes: no header", size)
		}
		if got := decryptedSize(int64(len(sealed))); got != int64(size) {
			t.Errorf("%d bytes: the size told is %d", size, got)
		}
		if size > 0 && bytes.Contains(sealed, data[:min(size, 64)]) {
			t.Errorf("%d bytes: the dump is in the clear", size)
		}
		got, err := decryptBytes(sealed, testPassphrase)
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("%d bytes: read back %d bytes, %v", size, len(got), err)
		}
	}
	if got := decryptedSize(int64(headerSize + 15)); got != -1 {
		t.Errorf("a size no encrypted backup has: %d", got)
	}
}

// A wrong passphrase is told apart from a damaged backup; any change to
// a chunk, their order, or the end of the file is refused, never read as
// a shorter or different dump.
func TestATamperedBackupIsRefused(t *testing.T) {
	t.Parallel()
	data := randomBytes(t, 2*chunkSize+100)
	sealed := encryptBytes(t, data, testPassphrase)
	if _, err := newDecrypter(bytes.NewReader(sealed), "another passphrase"); !errors.Is(err, ErrWrongPassphrase) {
		t.Errorf("a wrong passphrase: %v", err)
	}
	if _, err := newDecrypter(bytes.NewReader([]byte("PGDMP")), testPassphrase); !errors.Is(err, ErrNotABackup) {
		t.Errorf("a plain dump: %v", err)
	}

	sealedChunk := chunkSize + 16
	chunk := func(i int) []byte {
		return sealed[headerSize+i*sealedChunk : min(headerSize+(i+1)*sealedChunk, len(sealed))]
	}
	flip := func(at int) []byte {
		changed := slices.Clone(sealed)
		changed[at] ^= 1
		return changed
	}
	for name, test := range map[string]struct {
		file []byte
		want error
	}{
		"a byte of the salt":         {flip(len(encryptedMagic)), ErrWrongPassphrase},
		"a byte of the check":        {flip(headerSize - 1), ErrWrongPassphrase},
		"a byte of the first chunk":  {flip(headerSize + 10), ErrDamagedBackup},
		"a byte of the second chunk": {flip(headerSize + sealedChunk + 10), ErrDamagedBackup},
		"a byte of the last tag":     {flip(len(sealed) - 1), ErrDamagedBackup},
		"two chunks swapped":         {slices.Concat(sealed[:headerSize], chunk(1), chunk(0), chunk(2)), ErrDamagedBackup},
		"the last chunk dropped":     {sealed[:headerSize+2*sealedChunk], ErrDamagedBackup},
		"the end cut":                {sealed[:len(sealed)-1], ErrDamagedBackup},
		"no chunk at all":            {sealed[:headerSize], ErrDamagedBackup},
		"a byte added":               {append(slices.Clone(sealed), 0), ErrDamagedBackup},
		"a chunk added":              {slices.Concat(sealed[:headerSize], chunk(0), chunk(0), chunk(1), chunk(2)), ErrDamagedBackup},
		"a chunk of another backup": {slices.Concat(sealed[:headerSize], encryptBytes(t, data, testPassphrase)[headerSize:headerSize+sealedChunk],
			chunk(1), chunk(2)), ErrDamagedBackup},
	} {
		if got, err := decryptBytes(test.file, testPassphrase); !errors.Is(err, test.want) {
			t.Errorf("%s: %d bytes read, %v", name, len(got), err)
		}
	}
}
