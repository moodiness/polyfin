package secrets_test

import (
	"testing"

	"github.com/moodiness/polyfin/internal/secrets"
	"github.com/moodiness/polyfin/internal/testdb/migrated"
)

// The account key and private key of the certificate of Let's Encrypt are
// stored secrets: kept unsealed by a server without a key, sealed once it
// has one, its certificate staying as it is, and told when the key cannot
// open them, in Health and in a backup about to be restored.
func TestCertificateKeysAreStoredSecrets(t *testing.T) {
	ctx := t.Context()
	pool := migrated.New(t)
	if _, err := pool.Exec(ctx, `INSERT INTO acme_certificate (account_key, certificate, private_key)
		VALUES ('account-plain-key', 'certificate-plain', 'private-plain-key')`); err != nil {
		t.Fatal(err)
	}
	var none *secrets.Box
	if report, err := none.Inspect(ctx, pool); err != nil || report.Plaintext != 2 || len(report.Unreadable) != 0 {
		t.Errorf("without a key: %+v, %v", report, err)
	}
	key := newBox(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if sealed, err := key.SealStored(ctx, pool); err != nil || sealed != 2 {
		t.Fatalf("sealed %d, %v", sealed, err)
	}
	var accountKey, certificate, privateKey string
	if err := pool.QueryRow(ctx, "SELECT account_key, certificate, private_key FROM acme_certificate").Scan(&accountKey, &certificate,
		&privateKey); err != nil {
		t.Fatal(err)
	}
	if !secrets.Sealed(accountKey) || !secrets.Sealed(privateKey) || certificate != "certificate-plain" {
		t.Errorf("stored: %q %q %q", accountKey, certificate, privateKey)
	}
	if opened, err := key.Open(privateKey); err != nil || opened != "private-plain-key" {
		t.Errorf("opened %q, %v", opened, err)
	}
	if report, err := key.Inspect(ctx, pool); err != nil || report.Plaintext != 0 || len(report.Unreadable) != 0 {
		t.Errorf("with the key: %+v, %v", report, err)
	}
	other := newBox(t, "ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA=")
	report, err := other.Inspect(ctx, pool)
	if err != nil || len(report.Unreadable) != 1 || !report.Unreadable[0].Certificate {
		t.Errorf("with another key: %+v, %v", report, err)
	}
	if kind, count := other.UnreadableIn("acme_certificate", map[string]string{"account_key": accountKey, "private_key": privateKey}); kind != secrets.KindCertificate || count != 1 {
		t.Errorf("in a backup: %s %d", kind, count)
	}
}
