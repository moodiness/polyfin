package https

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// filesStamp is what certificate files were when last read: their paths,
// sizes and times of change. Either changing has them read again.
type filesStamp struct {
	certificate, privateKey fileStamp
}

type fileStamp struct {
	path     string
	size     int64
	modified time.Time
}

// FilesError is why certificate files cannot be served: Problem is one of
// FilesMissing, FilesUnreadable and FilesMismatch, File the path it is
// about.
type FilesError struct {
	Problem string
	File    string
	Err     error
}

func (e *FilesError) Error() string {
	switch e.Problem {
	case FilesMissing:
		return fmt.Sprintf("%s does not exist", e.File)
	case FilesMismatch:
		return "the private key does not belong to the certificate"
	}
	return fmt.Sprintf("%s cannot be read: %v", e.File, e.Err)
}

func (e *FilesError) Unwrap() error { return e.Err }

// LoadFiles reads the certificate, with its chain, and its private key,
// RSA or ECDSA, from PEM files, and checks that they belong together. A
// failure is a *FilesError.
func LoadFiles(certificatePath, privateKeyPath string) (*tls.Certificate, error) {
	certificatePEM, err := readFile(certificatePath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := readFile(privateKeyPath)
	if err != nil {
		return nil, err
	}
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return nil, filesError(certificatePath, privateKeyPath, certificatePEM, keyPEM, err)
	}
	switch certificate.PrivateKey.(type) {
	case *rsa.PrivateKey, *ecdsa.PrivateKey:
	default:
		return nil, &FilesError{Problem: FilesUnreadable, File: privateKeyPath, Err: errors.New("the key is neither RSA nor ECDSA")}
	}
	if certificate.Leaf == nil {
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return nil, &FilesError{Problem: FilesUnreadable, File: certificatePath, Err: err}
		}
		certificate.Leaf = leaf
	}
	return &certificate, nil
}

// readFile reads a certificate or key file.
func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, &FilesError{Problem: FilesMissing, File: path, Err: err}
	case err != nil:
		return nil, &FilesError{Problem: FilesUnreadable, File: path, Err: err}
	}
	return data, nil
}

// filesError tells why tls.X509KeyPair refused certificate and key files
// that could be read: one that holds no certificate or no key x509 reads
// is unreadable; a certificate and a key it reads both are mismatched.
func filesError(certificatePath, privateKeyPath string, certificatePEM, keyPEM []byte, err error) *FilesError {
	switch {
	case !holdsCertificate(certificatePEM):
		return &FilesError{Problem: FilesUnreadable, File: certificatePath, Err: err}
	case !holdsKey(keyPEM):
		return &FilesError{Problem: FilesUnreadable, File: privateKeyPath, Err: err}
	}
	return &FilesError{Problem: FilesMismatch, File: privateKeyPath, Err: err}
}

// holdsCertificate reports whether data holds a PEM certificate x509
// reads.
func holdsCertificate(data []byte) bool {
	for block, rest := decodePEM(data); block != nil; block, rest = decodePEM(rest) {
		if block.Type == "CERTIFICATE" {
			if _, err := x509.ParseCertificate(block.Bytes); err == nil {
				return true
			}
		}
	}
	return false
}

// holdsKey reports whether data holds a PEM private key x509 reads, in
// PKCS #1, PKCS #8 or SEC 1.
func holdsKey(data []byte) bool {
	for block, rest := decodePEM(data); block != nil; block, rest = decodePEM(rest) {
		if _, err := parsePrivateKey(block.Bytes); err == nil {
			return true
		}
	}
	return false
}

// readFiles reads the certificate files again when they changed since
// last read, or could not be read then. While they cannot be read, the
// certificate last read stays served, and Status tells why.
func (s *Service) readFiles(config Config) {
	stamp := filesStamp{certificate: stampOf(config.Certificate), privateKey: stampOf(config.PrivateKey)}
	s.mu.Lock()
	failing := s.status.Files != ""
	s.mu.Unlock()
	if stamp == s.files && !failing && s.certificate.Load() != nil {
		return
	}
	certificate, err := LoadFiles(config.Certificate, config.PrivateKey)
	if err != nil {
		var problem *FilesError
		if !errors.As(err, &problem) {
			problem = &FilesError{Problem: FilesUnreadable, File: config.Certificate, Err: err}
		}
		s.mu.Lock()
		changed := s.status.Files != problem.Problem || s.status.File != problem.File
		s.status.Files, s.status.File, s.status.FilesError = problem.Problem, problem.File, problem.Error()
		s.mu.Unlock()
		if changed {
			s.options.Logger.Warn("The certificate files cannot be served: the certificate last read stays in use", "error", problem.Error())
		}
		return
	}
	s.files = stamp
	previous := s.certificate.Swap(certificate)
	s.mu.Lock()
	s.status.Files, s.status.File, s.status.FilesError = "", "", ""
	s.mu.Unlock()
	if previous == nil || previous.Leaf == nil || !previous.Leaf.Equal(certificate.Leaf) {
		s.options.Logger.Info("HTTPS serves the certificate files", "names", certificate.Leaf.DNSNames,
			"expires", certificate.Leaf.NotAfter.Format(time.RFC3339))
	}
}

// stampOf returns what the file at path is now; a file that cannot be
// looked at has only its path.
func stampOf(path string) fileStamp {
	stamp := fileStamp{path: path}
	if info, err := os.Stat(path); err == nil {
		stamp.size, stamp.modified = info.Size(), info.ModTime()
	}
	return stamp
}
