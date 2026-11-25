package main

import (
	"errors"
	"strings"
	"testing"
)

func TestLatestReleaseLeavesOutPrereleasesAndTheCandidate(t *testing.T) {
	t.Parallel()
	tags := "a1\trefs/tags/v0.9.1\nb2\trefs/tags/v0.90.0\nc3\trefs/tags/v0.89.2\nd4\trefs/tags/v0.91.0-rc.1\ne5\trefs/tags/v0.91.0\n"
	for except, want := range map[string]string{"dev": "0.91.0", "0.91.0": "0.90.0"} {
		if got, err := latestRelease(tags, except); err != nil || got != want {
			t.Errorf("latestRelease(tags, %q) = %q, %v; want %q", except, got, err, want)
		}
	}
	if _, err := latestRelease("d4\trefs/tags/v0.91.0-rc.1\n", "dev"); err == nil {
		t.Error("a list of prereleases has a latest release")
	}
}

func TestReleaseImageRunsTheEmbeddedDatabaseOfEarlyReleasesFromTheirAioTags(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ version, database, want string }{
		{"0.71.0", embedded, "r:0.71.0-aio"},
		{"0.72.3", embedded, "r:0.72.3-aio"},
		{"0.71.0", external, "r:0.71.0"},
		{"0.73.0", embedded, "r:0.73.0"},
		{"0.90.0", embedded, "r:0.90.0"},
	} {
		if got := releaseImage("r", c.version, c.database); got != c.want {
			t.Errorf("releaseImage(%s, %s) = %s, want %s", c.version, c.database, got, c.want)
		}
	}
}

func TestCompareMigrationsNamesEachDifference(t *testing.T) {
	t.Parallel()
	shipped := map[int]migration{
		1: {1, "accounts", "aa"}, 2: {2, "items", "bb"}, 3: {3, "playlists", "cc"}, 4: {4, "rows", "dd"},
	}
	if err := compareMigrations(parseApplied("1|accounts|aa\n2|items|bb\n3|playlists|cc\n4|rows|dd\n"), shipped); err != nil {
		t.Fatalf("the same migrations differ: %v", err)
	}
	err := compareMigrations(parseApplied("1|accounts|aa\n2|items|b0\n3|lists|cc\n5|newer|ee\n"), shipped)
	var failed *stepError
	if !errors.As(err, &failed) || failed.check != "schema_migrations" {
		t.Fatalf("compareMigrations = %v, want a failed check of schema_migrations", err)
	}
	for _, want := range []string{
		"0002_items: applied with checksum b0, shipped with bb",
		"0003_playlists: applied as 0003_lists",
		"0004_rows: shipped, not applied",
		"0005_newer: applied, not shipped",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure lacks %q:\n%v", want, err)
		}
	}
}

func TestCompareObservationsNamesWhatDifferedAndLeavesOutWhatIsNew(t *testing.T) {
	t.Parallel()
	before := observations{"settings": {"serverName": "Maison", "playedPercent": "85"}, "user data": {"alice: Beach Day": "played"}}
	after := observations{"settings": {"serverName": "Polyfin", "playedPercent": "85", "added": "yes"}, "user data": {}}
	err := compareObservations(before, after)
	var failed *stepError
	if !errors.As(err, &failed) || failed.check != "settings, user data" {
		t.Fatalf("compareObservations = %v, want failed checks of settings and user data", err)
	}
	for _, want := range []string{`serverName: was "Maison", now "Polyfin"`, `alice: Beach Day: was "played", now absent`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the failure lacks %q:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "playedPercent") || strings.Contains(err.Error(), "added") {
		t.Errorf("the failure names a fact that did not change:\n%v", err)
	}
	if err := compareObservations(before, before); err != nil {
		t.Errorf("the same facts differ: %v", err)
	}
}
