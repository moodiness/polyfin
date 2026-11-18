package database

import (
	"bytes"
	"testing"

	"github.com/moodiness/polyfin/internal/testdb"
)

// A new signing secret replaces the stored one with 32 random bytes,
// others each time, and leaves the identifier apps know the server by as
// it was.
func TestANewSigningSecretReplacesTheStoredOne(t *testing.T) {
	t.Parallel()
	pool := testdb.Empty(t)
	ctx := t.Context()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	id, err := ServerID(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	seen := [][]byte{}
	for range 3 {
		secret, err := Secret(ctx, pool)
		if err != nil || len(secret) != 32 {
			t.Fatalf("the stored secret: %d bytes, %v", len(secret), err)
		}
		for _, earlier := range seen {
			if bytes.Equal(secret, earlier) {
				t.Fatal("a new signing secret is one stored before")
			}
		}
		seen = append(seen, secret)
		if err := ReplaceSecret(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}
	if after, err := ServerID(ctx, pool); err != nil || after != id {
		t.Errorf("the server's identifier after new secrets: %s %v, want %s", after, err, id)
	}
}
