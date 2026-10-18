package database

import (
	"bytes"
	"io/fs"
	"testing"
)

// The migrations end their lines with LF alone on every system, as
// .gitattributes checks them out: their checksums, kept in
// schema_migrations, are then the same in a Polyfin built on Windows as in
// the Docker image, and a backup restores from one to the other.
func TestMigrationsEndTheirLinesWithLF(t *testing.T) {
	err := fs.WalkDir(embedded, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(embedded, path)
		if err == nil && bytes.Contains(data, []byte("\r")) {
			t.Errorf("%s holds a carriage return", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
