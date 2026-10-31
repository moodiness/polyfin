package backup

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"slices"
	"testing"
)

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

// A dump encrypted reads back whole: empty, of whole chunks, or ending
// with a short one; its size is told from the encrypted size, and nothing
// of it is in the clear.
func TestAnEncryptedBackupReadsBack(t *testing.T) {
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
