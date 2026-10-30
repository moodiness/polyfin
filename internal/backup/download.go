package backup

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotEncrypted reports a backup asked for decrypted that is not
// encrypted.
var ErrNotEncrypted = errors.New("the backup is not encrypted")

// OpenBackup opens the backup of the folder named name, to download it as
// it is, and tells what it is.
func (s *Service) OpenBackup(name string) (*os.File, File, error) {
	dir := s.Dir()
	if dir == "" {
		return nil, File{}, ErrOff
	}
	if !restorableName.MatchString(name) {
		return nil, File{}, ErrBackupNotFound
	}
	path := filepath.Join(dir, name)
	// A link named like a backup is not one.
	if info, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return nil, File{}, ErrBackupNotFound
	} else if err != nil {
		return nil, File{}, err
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, File{}, ErrBackupNotFound
	}
	if err != nil {
		return nil, File{}, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, File{}, err
	}
	file, _ := restorable(name, info.Size())
	return f, file, nil
}

// Decrypted is the dump an encrypted backup holds, read as it is
// decrypted: a read fails with ErrDamagedBackup at the first part of the
// backup changed or missing.
type Decrypted struct {
	io.Reader
	file *os.File
	// Name is the dump's name, the backup's without .enc, and Size its
	// size.
	Name string
	Size int64
}

// Close closes the backup.
func (d *Decrypted) Close() error {
	return d.file.Close()
}

// OpenDecrypted opens the encrypted backup of the folder named name with
// passphrase, to download the dump it holds: ErrWrongPassphrase when the
// passphrase does not open it, ErrNotEncrypted when it is not encrypted.
func (s *Service) OpenDecrypted(name, passphrase string) (*Decrypted, error) {
	f, file, err := s.OpenBackup(name)
	if err != nil {
		return nil, err
	}
	if !file.Encrypted {
		_ = f.Close()
		return nil, ErrNotEncrypted
	}
	// One key is derived at a time: scrypt takes 128 MiB.
	s.unlocks.Lock()
	dump, err := newDecrypter(f, passphrase)
	s.unlocks.Unlock()
	if errors.Is(err, ErrNotABackup) {
		err = ErrDamagedBackup
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Decrypted{Reader: dump, file: f, Name: strings.TrimSuffix(name, encryptedExtension), Size: decryptedSize(file.Size)}, nil
}
