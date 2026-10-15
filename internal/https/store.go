package https

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/moodiness/polyfin/internal/secrets"
)

// Stored is what is kept of Let's Encrypt: the account key, the ACME
// directory the account is registered at, and the certificate, for Hosts,
// with its private key, nil for none; Failure is the last attempt's error,
// empty once one succeeds, at FailedAt.
type Stored struct {
	AccountKey  crypto.Signer
	Directory   string
	Hosts       []string
	Certificate *tls.Certificate
	Failure     string
	FailedAt    time.Time
}

// Store keeps what Let's Encrypt gave.
type Store interface {
	Load(ctx context.Context) (Stored, error)
	Save(ctx context.Context, stored Stored) error
}

// DatabaseStore keeps what Let's Encrypt gave in the database, the account
// key and the private key sealed with the stored secrets' key, so that
// backups hold them.
type DatabaseStore struct {
	db  *pgxpool.Pool
	box *secrets.Box
}

// NewDatabaseStore returns a store in db sealing with box (nil for no key).
func NewDatabaseStore(db *pgxpool.Pool, box *secrets.Box) *DatabaseStore {
	return &DatabaseStore{db: db, box: box}
}

// Load reads what is kept. A key the box cannot open, or a certificate that
// cannot be read, counts as none, and is obtained again; the secrets'
// report tells the keys it cannot open.
func (d *DatabaseStore) Load(ctx context.Context) (Stored, error) {
	var accountKey, certificate, privateKey string
	var failedAt *time.Time
	stored := Stored{}
	err := d.db.QueryRow(ctx, `SELECT account_key, directory, hosts, certificate, private_key, failure, failed_at FROM acme_certificate`).
		Scan(&accountKey, &stored.Directory, &stored.Hosts, &certificate, &privateKey, &stored.Failure, &failedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Stored{}, nil
	}
	if err != nil {
		return Stored{}, err
	}
	if failedAt != nil {
		stored.FailedAt = *failedAt
	}
	if opened, err := d.box.Open(accountKey); err == nil && opened != "" {
		if key, err := decodeKey([]byte(opened)); err == nil {
			stored.AccountKey = key
		}
	}
	if opened, err := d.box.Open(privateKey); err == nil && opened != "" && certificate != "" {
		if pair, err := tls.X509KeyPair([]byte(certificate), []byte(opened)); err == nil && pair.Leaf != nil {
			stored.Certificate = &pair
		}
	}
	return stored, nil
}

// Save keeps stored in place of what was kept.
func (d *DatabaseStore) Save(ctx context.Context, stored Stored) error {
	var accountKey, certificate, privateKey string
	if stored.AccountKey != nil {
		encoded, err := encodeKey(stored.AccountKey)
		if err != nil {
			return err
		}
		accountKey = d.box.Seal(string(encoded))
	}
	if stored.Certificate != nil {
		for _, der := range stored.Certificate.Certificate {
			certificate += string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		}
		signer, ok := stored.Certificate.PrivateKey.(crypto.Signer)
		if !ok {
			return errors.New("the certificate's key cannot be kept")
		}
		encoded, err := encodeKey(signer)
		if err != nil {
			return err
		}
		privateKey = d.box.Seal(string(encoded))
	}
	hosts := stored.Hosts
	if hosts == nil {
		hosts = []string{}
	}
	var failedAt *time.Time
	if !stored.FailedAt.IsZero() {
		failedAt = &stored.FailedAt
	}
	_, err := d.db.Exec(ctx, `INSERT INTO acme_certificate (id, account_key, directory, hosts, certificate, private_key, failure, failed_at, updated_at)
		VALUES (true, $1, $2, $3, $4, $5, $6, $7, now())
		ON CONFLICT (id) DO UPDATE SET account_key = $1, directory = $2, hosts = $3, certificate = $4, private_key = $5, failure = $6,
			failed_at = $7, updated_at = now()`,
		accountKey, stored.Directory, hosts, certificate, privateKey, stored.Failure, failedAt)
	return err
}

// encodeKey writes a private key as PEM, in PKCS #8.
func encodeKey(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

// decodeKey reads the first private key of PEM data.
func decodeKey(data []byte) (crypto.Signer, error) {
	for block, rest := decodePEM(data); block != nil; block, rest = decodePEM(rest) {
		if key, err := parsePrivateKey(block.Bytes); err == nil {
			return key, nil
		}
	}
	return nil, errors.New("no private key")
}

// decodePEM returns the first PEM block of data, nil for none, and what
// follows it.
func decodePEM(data []byte) (*pem.Block, []byte) {
	return pem.Decode(data)
}

// parsePrivateKey reads an RSA or ECDSA private key in PKCS #1, PKCS #8 or
// SEC 1, as tls.X509KeyPair does.
func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey:
			return key, nil
		case *ecdsa.PrivateKey:
			return key, nil
		}
		return nil, errors.New("neither an RSA nor an ECDSA key")
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("no private key")
}
