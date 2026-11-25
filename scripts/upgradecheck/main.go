// Command upgradecheck tests updating to a candidate image of Polyfin from
// older releases, as their users would: for each release, and for each
// kind of database, it starts the release's published image on an empty
// database, has it make representative data through its own APIs (see
// seed.go), stops it, starts the candidate on the same database, and
// checks that the candidate starts healthy with every migration applied
// with its checksum, and that the data, the settings and a few routes
// answer as they did before (see checks.go).
//
//	go run ./scripts/upgradecheck [-candidate image] [-from versions] [-databases kinds]
//
// It needs Docker, and runs from the repository's root, whose migrations
// the candidate must ship. It prints, for each release and database, each
// step it takes, then a report; a failure names the release, the database,
// the step, the check and what differed. It exits with 0 when every
// upgrade passed, 1 when one failed, and 2 on an error before any started.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// defaultFrom are the releases upgraded from, spread across the migration
// history since Polyfin could run its own database (0.71.0):
//   - 0.71.0, the first release with an embedded database, 238 migrations:
//     the oldest database a single-container install can hold, run by its
//     aio image (see releaseImage);
//   - 0.80.0, 250 migrations;
//   - 0.85.0, 266 migrations;
//   - latest, the newest release, whose users the candidate updates next.
const defaultFrom = "0.71.0,0.80.0,0.85.0,latest"

// postgresImage is the PostgreSQL the external database runs: the image
// compose.yaml and CI's services run, pinned by the same digest, through
// Google's mirror of Docker Hub, as CI pulls it. Its psql also reads the
// embedded database's migrations.
const postgresImage = "mirror.gcr.io/library/postgres:18.6@sha256:5a5a84b19854a9ffaa54082c166ff4ec27473a361e496e5ea167f298f2da9722"

// releaseVersion is a release's version: prereleases are left out of
// latest.
var releaseVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)$`)

// releaseImage is the image a release runs on database with. 0.71 and
// 0.72 ran their own database in a second image, tagged <version>-aio,
// from which their users move to the main image (docs/installation.md,
// "Moving from the aio tags"); every other release runs both databases in
// its one image.
func releaseImage(repository, version, database string) string {
	if database == embedded && (strings.HasPrefix(version, "0.71.") || strings.HasPrefix(version, "0.72.")) {
		return repository + ":" + version + "-aio"
	}
	return repository + ":" + version
}

type options struct {
	candidate  string
	releases   string
	postgres   string
	migrations string
	keep       bool
}

func main() {
	var opts options
	var from, databases string
	var parallel int
	flag.StringVar(&opts.candidate, "candidate", "polyfin-local:check", "the `image` upgraded to, as make image-check builds it")
	flag.StringVar(&from, "from", defaultFrom, "the `versions` upgraded from, comma-separated; latest is the newest release tag of the remote origin")
	flag.StringVar(&databases, "databases", "embedded,external", "the `kinds` of database, comma-separated: embedded, external")
	flag.IntVar(&parallel, "parallel", 4, "the number of upgrades run at once")
	flag.StringVar(&opts.releases, "releases", "ghcr.io/moodiness/polyfin", "the `repository` of the releases' images")
	flag.StringVar(&opts.postgres, "postgres", postgresImage, "the PostgreSQL `image` of the external database")
	flag.StringVar(&opts.migrations, "migrations", "internal/database/migrations", "the `folder` of the migrations the candidate ships")
	flag.BoolVar(&opts.keep, "keep", false, "keep the containers, volumes and networks of each upgrade, for a look after")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, opts, from, databases, parallel))
}

func run(ctx context.Context, opts options, from, databases string, parallel int) int {
	fail := func(format string, args ...any) int {
		fmt.Fprintf(os.Stderr, "upgradecheck: "+format+"\n", args...)
		return 2
	}
	kinds, err := databaseKinds(databases)
	if err != nil {
		return fail("%v", err)
	}
	if parallel < 1 {
		return fail("-parallel must be at least 1")
	}
	shipped, err := shippedMigrations(opts.migrations)
	if err != nil {
		return fail("read the candidate's migrations: %v", err)
	}
	candidateVersion, err := docker(ctx, "run", "--rm", "--network", "none", opts.candidate, "version")
	if err != nil {
		return fail("the candidate image %s: %v", opts.candidate, err)
	}
	versions, err := resolveVersions(ctx, from, candidateVersion)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Printf("Upgrading to %s (version %s, %d migrations) from %s, with the %s database\n",
		opts.candidate, candidateVersion, len(shipped), strings.Join(versions, ", "), strings.Join(kinds, " and the "))

	id := make([]byte, 3)
	_, _ = rand.Read(id)
	prefix := "pfupgrade-" + hex.EncodeToString(id)
	media := prefix + "-media"
	var upgrades []*upgrade
	for _, version := range versions {
		for _, kind := range kinds {
			upgrades = append(upgrades, &upgrade{
				options: opts, from: version, oldImage: releaseImage(opts.releases, version, kind), database: kind, media: media,
				name: prefix + "-" + strings.ReplaceAll(version, ".", "-") + "-" + kind, shipped: shipped,
			})
		}
	}
	pulled := map[string]bool{}
	for _, u := range upgrades {
		if pulled[u.oldImage] {
			continue
		}
		pulled[u.oldImage] = true
		fmt.Printf("Pulling %s\n", u.oldImage)
		if _, err := docker(ctx, "pull", "--quiet", u.oldImage); err != nil {
			return fail("pull the image of %s: %v", u.from, err)
		}
	}
	if slices.Contains(kinds, external) {
		fmt.Printf("Pulling %s\n", opts.postgres)
		if _, err := docker(ctx, "pull", "--quiet", opts.postgres); err != nil {
			return fail("pull PostgreSQL's image: %v", err)
		}
	}
	if !opts.keep {
		defer func() { _, _ = docker(context.WithoutCancel(ctx), "volume", "rm", media) }()
	}
	if err := makeMedia(ctx, opts.candidate, media); err != nil {
		return fail("make the media files: %v", err)
	}

	slots := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	for _, u := range upgrades {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			u.run(ctx)
		})
	}
	wg.Wait()
	return report(os.Stdout, upgrades)
}

const (
	embedded = "embedded"
	external = "external"
)

func databaseKinds(list string) ([]string, error) {
	var kinds []string
	for kind := range strings.SplitSeq(list, ",") {
		kind = strings.TrimSpace(kind)
		if kind != embedded && kind != external {
			return nil, fmt.Errorf("unknown database %q: want embedded or external", kind)
		}
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	return kinds, nil
}

// resolveVersions reads the list of -from, with latest as the newest
// release tag of the remote origin other than the candidate's own
// version, each version once: the release about to be published may be
// tagged before its image is.
func resolveVersions(ctx context.Context, list, candidate string) ([]string, error) {
	var versions []string
	for version := range strings.SplitSeq(list, ",") {
		version = strings.TrimPrefix(strings.TrimSpace(version), "v")
		if version == "latest" {
			var stderr strings.Builder
			cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--refs", "origin", "v*")
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				return nil, fmt.Errorf("find the latest release among the tags of the remote origin (or name it in -from): %w: %s",
					err, strings.TrimSpace(stderr.String()))
			}
			if version, err = latestRelease(string(out), candidate); err != nil {
				return nil, err
			}
		}
		if version == "" {
			return nil, errors.New("-from names an empty version")
		}
		if !slices.Contains(versions, version) {
			versions = append(versions, version)
		}
	}
	return versions, nil
}

// latestRelease is the newest release of git ls-remote's list of tags,
// prereleases and the version except left out.
func latestRelease(tags, except string) (string, error) {
	var latest []int
	var name string
	for line := range strings.Lines(tags) {
		_, ref, _ := strings.Cut(strings.TrimSpace(line), "\t")
		match := releaseVersion.FindStringSubmatch(strings.TrimPrefix(ref, "refs/tags/v"))
		if match == nil || match[0] == except {
			continue
		}
		numbers := make([]int, 3)
		for i := range numbers {
			numbers[i], _ = strconv.Atoi(match[i+1])
		}
		if latest == nil || slices.Compare(numbers, latest) > 0 {
			latest, name = numbers, match[0]
		}
	}
	if latest == nil {
		return "", errors.New("the remote origin has no release tag")
	}
	return name, nil
}

// report prints how each upgrade went, then the failures in full, and
// annotates each failure in GitHub Actions. It returns the exit code.
func report(w io.Writer, upgrades []*upgrade) int {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Upgrade check:")
	failed := 0
	for _, u := range upgrades {
		if u.err == nil {
			fmt.Fprintf(w, "  passed  %s (%s)\n", u.title(), u.took.Round(time.Second))
			continue
		}
		failed++
		fmt.Fprintf(w, "  FAILED  %s (%s)\n", u.title(), u.took.Round(time.Second))
	}
	for _, u := range upgrades {
		if u.err == nil {
			continue
		}
		fmt.Fprintf(w, "\n%s failed:\n%s\n", u.title(), indent(u.err.Error()))
		if os.Getenv("GITHUB_ACTIONS") == "true" {
			// One line, escaped as the workflow commands want: the title,
			// a property, escapes its commas and colons too.
			message := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(u.err.Error())
			title := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ",", "%2C", ":", "%3A").Replace("Update " + u.title())
			fmt.Fprintf(w, "::error title=%s::%s\n", title, message)
		}
	}
	if failed > 0 {
		fmt.Fprintf(w, "\n%d of %d upgrades failed\n", failed, len(upgrades))
		return 1
	}
	fmt.Fprintf(w, "\nAll %d upgrades passed\n", len(upgrades))
	return 0
}

func indent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n")
}
