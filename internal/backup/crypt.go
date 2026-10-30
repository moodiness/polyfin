package backup

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// An encrypted backup is pg_dump's output sealed with a key derived from a
// passphrase, in chunks, so that it is written and read as a stream:
//
//   - a header: encryptedMagic, which also tells the format's version, a
//     random salt, and a check, the AES-GCM tag of nothing under the key,
//     which tells a wrong passphrase from a damaged file;
//   - chunks of chunkSize bytes of the dump each, the last one shorter or
//     empty, each sealed with AES-256-GCM, the header as associated data.
//
// The key is scrypt's of the passphrase and the salt, which is new for
// every file: a key never seals two files, and a chunk's nonce is its
// number, with its last byte telling the last chunk from the others. A
// chunk changed, moved, dropped or added, a file cut short or lengthened,
// fails to open: nothing of a damaged backup is restored.
const (
	// encryptedExtension ends the name of an encrypted backup, after
	// .dump.
	encryptedExtension = ".enc"
	saltSize           = 32
	checkSize          = 16
	headerSize         = len(encryptedMagic) + saltSize + checkSize
	chunkSize          = 64 << 10
	// The cost of scrypt: N = 2^17, 128 MiB of memory and a fraction of
	// a second for each file written or opened.
	scryptN = 1 << 17
	scryptR = 8
	scryptP = 1
)

// encryptedMagic starts an encrypted backup; its last byte is the
// format's version.
const encryptedMagic = "PFBKENC\x01"

// The kinds of nonces: a chunk's, the last chunk's, and the check's.
const (
	nonceChunk byte = iota
	nonceLast
	nonceCheck
)

// The errors of an encrypted backup.
var (
	// ErrWrongPassphrase reports a passphrase that does not open an
	// encrypted backup.
	ErrWrongPassphrase = errors.New("the passphrase does not open this backup")
	// ErrDamagedBackup reports an encrypted backup whose content was
	// changed or cut short.
	ErrDamagedBackup = errors.New("the encrypted backup is damaged: part of it was changed or is missing")
)

// newAEAD derives the key of passphrase and salt.
func newAEAD(passphrase string, salt []byte) (cipher.AEAD, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// nonce is the nonce of the chunk numbered counter, of kind kind.
func nonce(counter uint64, kind byte) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n, counter)
	n[11] = kind
	return n
}

// encrypter seals what is written to it into w (see newEncrypter).
type encrypter struct {
	w       io.Writer
	aead    cipher.AEAD
	header  []byte
	buf     []byte
	out     []byte
	counter uint64
	closed  bool
	err     error
}

// newEncrypter writes the header of a backup encrypted with passphrase to
// w, and returns what seals the dump written to it: Close seals its last
// chunk, without closing w.
func newEncrypter(w io.Writer, passphrase string) (io.WriteCloser, error) {
	header := make([]byte, 0, headerSize)
	header = append(header, encryptedMagic...)
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	header = append(header, salt...)
	aead, err := newAEAD(passphrase, salt)
	if err != nil {
		return nil, err
	}
	header = aead.Seal(header, nonce(0, nonceCheck), nil, header)
	if _, err := w.Write(header); err != nil {
		return nil, err
	}
	return &encrypter{w: w, aead: aead, header: header, buf: make([]byte, 0, chunkSize),
		out: make([]byte, 0, chunkSize+aead.Overhead())}, nil
}

func (e *encrypter) Write(p []byte) (int, error) {
	if e.closed {
		return 0, errors.New("write to a closed encrypted backup")
	}
	if e.err != nil {
		return 0, e.err
	}
	written := 0
	for len(p) > 0 {
		// A full chunk is sealed once more follows: the last one, full or
		// not, is sealed by Close.
		if len(e.buf) == chunkSize {
			if e.err = e.seal(nonceChunk); e.err != nil {
				return written, e.err
			}
		}
		n := copy(e.buf[len(e.buf):chunkSize], p)
		e.buf = e.buf[:len(e.buf)+n]
		p = p[n:]
		written += n
	}
	return written, nil
}

func (e *encrypter) seal(kind byte) error {
	e.out = e.aead.Seal(e.out[:0], nonce(e.counter, kind), e.buf, e.header)
	e.counter++
	e.buf = e.buf[:0]
	_, err := e.w.Write(e.out)
	return err
}

// Close seals the last chunk.
func (e *encrypter) Close() error {
	if e.closed || e.err != nil {
		return e.err
	}
	e.closed = true
	e.err = e.seal(nonceLast)
	return e.err
}

// decrypter opens the chunks of an encrypted backup (see newDecrypter).
type decrypter struct {
	r       *bufio.Reader
	aead    cipher.AEAD
	header  []byte
	chunk   []byte
	plain   []byte
	counter uint64
	done    bool
	err     error
}

// encryptedHeader reports whether prefix, the start of a file, is the start
// of an encrypted backup.
func encryptedHeader(prefix []byte) bool {
	return bytes.HasPrefix(prefix, []byte(encryptedMagic))
}

// encryptedName reports whether name is that of an encrypted backup,
// which ends with .enc.
func encryptedName(name string) bool {
	return strings.HasSuffix(name, encryptedExtension)
}

// isEncrypted reports whether the file at path is an encrypted backup.
func isEncrypted(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	prefix := make([]byte, len(encryptedMagic))
	if _, err := io.ReadFull(f, prefix); err != nil {
		// A file shorter than the magic is not one.
		return false, nil
	}
	return encryptedHeader(prefix), nil
}

// newDecrypter reads the header of the encrypted backup r and returns what
// reads the dump: ErrWrongPassphrase when passphrase does not open it, and
// ErrNotABackup when it is not an encrypted backup. Reading fails with
// ErrDamagedBackup at the first chunk that does not open, or once the
// last one is missing; it gives io.EOF only after the last one opened.
func newDecrypter(r io.Reader, passphrase string) (io.Reader, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrNotABackup
		}
		return nil, err
	}
	if !encryptedHeader(header) {
		return nil, ErrNotABackup
	}
	unchecked := len(encryptedMagic) + saltSize
	aead, err := newAEAD(passphrase, header[len(encryptedMagic):unchecked])
	if err != nil {
		return nil, err
	}
	check := aead.Seal(nil, nonce(0, nonceCheck), nil, header[:unchecked])
	if subtle.ConstantTimeCompare(check[len(check)-checkSize:], header[unchecked:]) != 1 {
		return nil, ErrWrongPassphrase
	}
	return &decrypter{r: bufio.NewReaderSize(r, chunkSize+aead.Overhead()+1), aead: aead, header: header,
		chunk: make([]byte, chunkSize+aead.Overhead()), plain: make([]byte, 0, chunkSize)}, nil
}

func (d *decrypter) Read(p []byte) (int, error) {
	for len(d.plain) == 0 {
		switch {
		case d.err != nil:
			return 0, d.err
		case d.done:
			return 0, io.EOF
		}
		d.err = d.open()
	}
	n := copy(p, d.plain)
	d.plain = d.plain[n:]
	return n, nil
}

// open reads and opens the next chunk: the last one is the one the file
// ends with.
func (d *decrypter) open() error {
	n, err := io.ReadFull(d.r, d.chunk)
	last := false
	switch {
	case errors.Is(err, io.EOF):
		// The file ends without its last chunk.
		return ErrDamagedBackup
	case errors.Is(err, io.ErrUnexpectedEOF):
		last = true
	case err != nil:
		return err
	default:
		if _, err := d.r.Peek(1); errors.Is(err, io.EOF) {
			last = true
		} else if err != nil {
			return err
		}
	}
	kind := nonceChunk
	if last {
		kind = nonceLast
	}
	plain, err := d.aead.Open(d.plain[:0], nonce(d.counter, kind), d.chunk[:n], d.header)
	if err != nil {
		return ErrDamagedBackup
	}
	d.counter++
	d.plain = plain
	d.done = last
	return nil
}

// decryptedSize is the size of the dump an encrypted backup of size bytes
// holds, or -1 when no encrypted backup has that size.
func decryptedSize(size int64) int64 {
	body := size - int64(headerSize)
	sealed := int64(chunkSize + 16)
	if body < 16 {
		return -1
	}
	// A body of whole sealed chunks ends with a full last chunk.
	chunks := (body + sealed - 1) / sealed
	if body-(chunks-1)*sealed < 16 {
		return -1
	}
	return body - chunks*16
}
