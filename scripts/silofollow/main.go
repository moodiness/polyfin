// Command silofollow reports what changed in Silo since the commits
// Polyfin follows, which internal/silo/followed.json records: in Silo's
// contract, the operations Polyfin serves (silo.Operations), their
// parameters, request bodies and answers, through the schemas they name;
// in Silo's apps, the files the rules of internal/silo/silotest cite.
//
// It prints a Markdown report, and exits with 0 when nothing changed, 1
// when something did, and 2 on an error. It reads GitHub's API with the
// token of GITHUB_TOKEN or GH_TOKEN when one is set.
//
//	go run ./scripts/silofollow [-ref repository=ref]...
//
// It compares each repository's main branch, or the reference -ref gives
// it ("-ref silo-apple=2c8f2e1b").
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/moodiness/polyfin/internal/silo"
)

func main() {
	refs := map[string]string{}
	flag.Func("ref", "compare `repository=ref` instead of its main branch (repeatable)", func(value string) error {
		repository, ref, ok := strings.Cut(value, "=")
		if !ok || ref == "" {
			return errors.New("want repository=ref")
		}
		refs[repository] = ref
		return nil
	})
	flag.Parse()
	report, err := follow(context.Background(), newGitHub(), silo.Followed, silo.Operations(), refs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "silofollow:", err)
		os.Exit(2)
	}
	report.write(os.Stdout)
	if report.changed() {
		os.Exit(1)
	}
}

// report is what changed since the commits followed.
type report struct {
	contract contractReport
	apps     []appReport
}

type contractReport struct {
	pin      silo.ContractPin
	current  string
	compared ContractChanges
}

type appReport struct {
	pin     silo.AppPin
	current string
	link    string
	commits int
	files   []fileChange
}

type fileChange struct {
	path, change, link string
}

func (r report) changed() bool {
	return r.contract.compared.changed() || slices.ContainsFunc(r.apps, func(a appReport) bool { return len(a.files) > 0 })
}

// follow compares the repositories followed with their current commits.
func follow(ctx context.Context, gh *github, pins silo.Pins, operations []string, refs map[string]string) (report, error) {
	// A repository is named in full or by its name alone.
	names := func(name, repository string) bool {
		return name == repository || name == repository[strings.IndexByte(repository, '/')+1:]
	}
	ref := func(repository string) string {
		for name, ref := range refs {
			if names(name, repository) {
				return ref
			}
		}
		return "main"
	}
	for name := range refs {
		if !names(name, pins.Contract.Repository) && !slices.ContainsFunc(pins.Apps, func(a silo.AppPin) bool { return names(name, a.Repository) }) {
			return report{}, fmt.Errorf("-ref %s: not a repository followed", name)
		}
	}
	var r report
	var err error
	if r.contract, err = followContract(ctx, gh, pins.Contract, operations, ref(pins.Contract.Repository)); err != nil {
		return report{}, err
	}
	for _, pin := range pins.Apps {
		app, err := followApp(ctx, gh, pin, ref(pin.Repository))
		if err != nil {
			return report{}, err
		}
		r.apps = append(r.apps, app)
	}
	return r, nil
}

func followContract(ctx context.Context, gh *github, pin silo.ContractPin, operations []string, ref string) (contractReport, error) {
	current, err := gh.commit(ctx, pin.Repository, ref)
	if err != nil {
		return contractReport{}, err
	}
	pinned, err := gh.file(ctx, pin.Repository, pin.Path, pin.Commit)
	if err != nil {
		return contractReport{}, err
	}
	if digest := sha256.Sum256(pinned); hex.EncodeToString(digest[:]) != pin.SHA256 {
		return contractReport{}, fmt.Errorf("followed.json gives the contract's SHA-256 as %s, but it is %x at %s", pin.SHA256, digest, pin.Commit)
	}
	now := pinned
	if current != pin.Commit {
		if now, err = gh.file(ctx, pin.Repository, pin.Path, current); err != nil {
			return contractReport{}, err
		}
	}
	compared, err := compareContracts(pinned, now, operations)
	return contractReport{pin: pin, current: current, compared: compared}, err
}

func followApp(ctx context.Context, gh *github, pin silo.AppPin, ref string) (appReport, error) {
	current, err := gh.commit(ctx, pin.Repository, ref)
	if err != nil {
		return appReport{}, err
	}
	before, err := gh.tree(ctx, pin.Repository, pin.Commit)
	if err != nil {
		return appReport{}, err
	}
	for _, path := range pin.Files {
		if _, ok := before[path]; !ok {
			return appReport{}, fmt.Errorf("followed.json lists %s, which %s does not have at %s", path, pin.Repository, pin.Commit)
		}
	}
	app := appReport{pin: pin, current: current}
	if current == pin.Commit {
		return app, nil
	}
	c, err := gh.compare(ctx, pin.Repository, pin.Commit, current)
	if err != nil {
		return appReport{}, err
	}
	app.link, app.commits = c.HTMLURL, c.TotalCommits
	var after map[string]string
	for _, path := range pin.Files {
		i := slices.IndexFunc(c.Files, func(f comparedFile) bool {
			return f.Filename == path || f.Status == "renamed" && f.PreviousFilename == path
		})
		if i >= 0 {
			f := c.Files[i]
			change := fmt.Sprintf("%s, +%d -%d", f.Status, f.Additions, f.Deletions)
			if f.Status == "renamed" {
				change = fmt.Sprintf("renamed to %s, +%d -%d", f.Filename, f.Additions, f.Deletions)
			}
			digest := sha256.Sum256([]byte(f.Filename))
			app.files = append(app.files, fileChange{path: path, change: change, link: c.HTMLURL + "#diff-" + hex.EncodeToString(digest[:])})
			continue
		}
		if len(c.Files) < compareLimit {
			continue
		}
		// The comparison lists only some of the files changed: their blobs
		// tell the others.
		if after == nil {
			if after, err = gh.tree(ctx, pin.Repository, current); err != nil {
				return appReport{}, err
			}
		}
		switch blob, ok := after[path]; {
		case !ok:
			app.files = append(app.files, fileChange{path: path, change: "removed or renamed"})
		case blob != before[path]:
			app.files = append(app.files, fileChange{path: path, change: "modified"})
		}
	}
	return app, nil
}

func short(commit string) string { return commit[:min(len(commit), 8)] }

// write writes the report in Markdown.
func (r report) write(w io.Writer) {
	fmt.Fprintf(w, "# What changed in Silo since the commits Polyfin follows\n\n")
	fmt.Fprintf(w, "Polyfin follows Silo at the commits `internal/silo/followed.json` records. This report lists what changed since in the operations of Silo's contract that Polyfin serves, and in the files of Silo's apps the rules of `internal/silo/silotest` cite. Read those rules and Polyfin's answers again, change them where needed, then update the record, as `docs/development.md` says.\n\n")

	c := r.contract
	fmt.Fprintf(w, "## Contract\n\n")
	fmt.Fprintf(w, "`%s` of %s, followed at `%s`, now at `%s`", c.pin.Path, c.pin.Repository, short(c.pin.Commit), short(c.current))
	if c.current != c.pin.Commit {
		fmt.Fprintf(w, " ([commits](https://github.com/%s/compare/%s...%s))", c.pin.Repository, c.pin.Commit, c.current)
	}
	fmt.Fprintf(w, ". Of the %d operations Polyfin serves that the contract has, ", c.compared.Served)
	if !c.compared.changed() {
		fmt.Fprintf(w, "none changed.\n\n")
	} else {
		fmt.Fprintf(w, "%d changed.\n\n", len(c.compared.Operations))
	}
	for _, op := range c.compared.Operations {
		fmt.Fprintf(w, "### `%s`\n\n", op.Operation)
		writeChanges(w, op.Changes)
		if len(op.Schemas) > 0 {
			fmt.Fprintf(w, "- Schemas changed: `%s`\n", strings.Join(op.Schemas, "`, `"))
		}
		fmt.Fprintln(w)
	}
	if len(c.compared.Schemas) > 0 {
		fmt.Fprintf(w, "### Schemas\n\n")
		for _, s := range c.compared.Schemas {
			fmt.Fprintf(w, "#### `%s`\n\n", s.Name)
			writeChanges(w, s.Changes)
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "## Apps\n")
	for _, app := range r.apps {
		fmt.Fprintf(w, "\n### %s\n\n", app.pin.Repository)
		fmt.Fprintf(w, "Followed at `%s`, now at `%s`", short(app.pin.Commit), short(app.current))
		if app.link != "" {
			fmt.Fprintf(w, ", %d %s later ([compare](%s))", app.commits, plural(app.commits, "commit", "commits"), app.link)
		}
		fmt.Fprintf(w, ". Of the %d files the rules cite, ", len(app.pin.Files))
		if len(app.files) == 0 {
			fmt.Fprintf(w, "none changed.\n")
			continue
		}
		fmt.Fprintf(w, "%d changed:\n\n", len(app.files))
		for _, f := range app.files {
			if f.link != "" {
				fmt.Fprintf(w, "- [`%s`](%s): %s\n", f.path, f.link, f.change)
			} else {
				fmt.Fprintf(w, "- `%s`: %s\n", f.path, f.change)
			}
		}
	}
}

func writeChanges(w io.Writer, changes []Change) {
	for _, change := range changes {
		if change.Where == "" {
			fmt.Fprintf(w, "- %s\n", change.What)
		} else {
			fmt.Fprintf(w, "- `%s`: %s\n", change.Where, change.What)
		}
	}
}

func plural(n int, one, more string) string {
	if n == 1 {
		return one
	}
	return more
}
