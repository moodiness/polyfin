package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// migration is a migration the candidate ships, or one a database
// applied: its version, name and checksum, the SHA-256 of its file, as
// internal/database records it in schema_migrations.
type migration struct {
	version  int
	name     string
	checksum string
}

func (m migration) String() string { return fmt.Sprintf("%04d_%s", m.version, m.name) }

var migrationFile = regexp.MustCompile(`^([0-9]{4})_([a-z0-9_]+)\.sql$`)

// shippedMigrations reads the migrations of folder, by version.
func shippedMigrations(folder string) (map[int]migration, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	shipped := map[int]migration{}
	for _, entry := range entries {
		match := migrationFile.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		body, err := os.ReadFile(filepath.Join(folder, entry.Name()))
		if err != nil {
			return nil, err
		}
		version, _ := strconv.Atoi(match[1])
		sum := sha256.Sum256(body)
		shipped[version] = migration{version: version, name: match[2], checksum: hex.EncodeToString(sum[:])}
	}
	if len(shipped) == 0 {
		return nil, fmt.Errorf("%s holds no migration", folder)
	}
	return shipped, nil
}

// parseApplied reads schema_migrations as psql prints it: version, name
// and checksum, separated by |, a row a line.
func parseApplied(rows string) map[int]migration {
	applied := map[int]migration{}
	for line := range strings.Lines(rows) {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 3 {
			continue
		}
		version, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		applied[version] = migration{version: version, name: fields[1], checksum: fields[2]}
	}
	return applied
}

// compareMigrations fails with each migration the candidate ships that
// the database did not apply, applied under another name or checksum, and
// each it applied that the candidate does not ship.
func compareMigrations(applied, shipped map[int]migration) error {
	var lines []string
	last := 0
	for version := range applied {
		last = max(last, version)
	}
	for version := range shipped {
		last = max(last, version)
	}
	for version := 1; version <= last; version++ {
		a, wasApplied := applied[version]
		s, isShipped := shipped[version]
		switch {
		case isShipped && !wasApplied:
			lines = append(lines, fmt.Sprintf("%s: shipped, not applied", s))
		case wasApplied && !isShipped:
			lines = append(lines, fmt.Sprintf("%s: applied, not shipped", a))
		case wasApplied && a.name != s.name:
			lines = append(lines, fmt.Sprintf("%s: applied as %s", s, a))
		case wasApplied && a.checksum != s.checksum:
			lines = append(lines, fmt.Sprintf("%s: applied with checksum %s, shipped with %s", s, a.checksum, s.checksum))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	return &stepError{check: "schema_migrations", err: errors.New(strings.Join(lines, "\n"))}
}
