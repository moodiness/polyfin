# Development

This page explains how to run Polyfin from source, run its tests, refresh the Jellyfin fixtures, what CI runs and when, how to follow Silo's changes, and how to work on the website.

## Requirements

- Go 1.27.
- Node.js 24.
- A PostgreSQL 18 server.
- FFmpeg 9.0 or later (`ffmpeg` and `ffprobe`), to play titles.

With `POSTGRES_PASSWORD` set in `.env`, `docker compose -f compose.yaml -f compose.build.yaml up -d postgres` starts a PostgreSQL server on `127.0.0.1:5432`.

## Commands

```sh
export POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin
make dev                      # builds the admin app, then runs the server on :8096
npm --prefix web run dev      # optional: admin app with hot reload, proxied to :8096
make check                    # formatting, vet and tests
```

## Tests

- Database tests run when `POLYFIN_TEST_DATABASE_URL` points to a disposable PostgreSQL database whose role may create databases. Most database tests work in a database of their own, copied from a template database migrated once and kept for later runs, `polyfin_template_` followed by a key of the migrations; the others work in a schema of their own.
- Tests that run FFmpeg run when `POLYFIN_TEST_FFMPEG` names an `ffmpeg` executable with `ffprobe` beside it.
- Backup and restore tests run when `POLYFIN_TEST_PG_DUMP` and `POLYFIN_TEST_PG_RESTORE` name PostgreSQL 18's `pg_dump` and `pg_restore`.
- OCR tests run when `POLYFIN_TEST_TESSERACT` names a Tesseract executable, with the data of `POLYFIN_TEST_TESSDATA` when set.
- Polyfin Desktop's tests that run PostgreSQL (`internal/desktop/cluster` and `internal/desktop/app`) need `POLYFIN_TEST_POSTGRES_BIN`, the folder of PostgreSQL 18's server programs. `POLYFIN_TEST_POSTGRES_PORT` gives them a port, else they take a free one; both packages read it, so a fixed port needs them tested apart. `POLYFIN_TEST_DESKTOP_PORT` gives the app's tests their listening port, else they take a free one.
- The test of a torrent route's interface going down runs when `POLYFIN_TEST_TORRENT_INTERFACE` names a dummy interface with an address, with the right to set it down and up (`NET_ADMIN`).
- CI provides the database, FFmpeg, `pg_dump` and `pg_restore`, and Tesseract to the test jobs, PostgreSQL's server programs to the Polyfin Desktop jobs, and a dummy interface to the torrent route test.
- `make check` runs every package's tests under the race detector (`make test`), the slowest packages first, then, without it, the time bounds of a large IPTV line-up (`make bounds`): the race detector slows that work tenfold, and gets a tenth of the line-up. CI runs the tests in four jobs at once: those of `internal/jellyfin`, by far the longest, in two halves by the first letter after `Test` of their names (`make test-jellyfin-1` and `make test-jellyfin-2`), and the other packages in two shares balanced by their times on CI (`make test-race-1` and `make test-race-2`). A new package goes to the second share, and a new test of `internal/jellyfin` to the half its name falls in.
- The tests of the slowest packages run in parallel. A test that changes state other tests read, such as a package variable, or that times waits of a few milliseconds, stays sequential and says why.
- A test that counts goroutines labels its own with `runtime/pprof` labels and counts those alone, so it runs in parallel too (see `TestPlaysThatGaveUpLeaveNoGoroutineBehind`).
- The tests of `internal/accounts`, `internal/admin`, `internal/backup` and `internal/jellyfin` hash passwords and derive backup keys at a small fraction of the server's cost, set in their `TestMain` by `accounts.LowerPasswordCostForTests` and `backup.LowerKeyCostForTests`, which panic outside a test binary. Under the race detector, the server's cost takes about a quarter of a second for each password and two seconds for each backup key. A password hash records its cost, so it verifies the same way at either cost; a backup does not, so a backup sealed in such a binary opens only there. Another package's tests can call them the same way.
- The SMB share tests run when `POLYFIN_TEST_SMB` names a share holding video files, its user and password in the address: `smb://user:password@host:port/share/folder`. The user must be allowed to write there: the change notification test adds, renames and removes files in a folder of its own, removed after. CI does not run them; WebDAV folders are tested against a server in the tests themselves.
- `make image-check` builds the image and checks the version it prints; `make image-health` then starts it against the PostgreSQL of `IMAGE_DATABASE_URL`, by default the host's on port 5432 with the user, password and database `polyfin`, and waits for its health check to report it healthy, as CI does.
- `POLYFIN_SYNTHETIC_XTREAM=1 go test -run TestServeSyntheticXtream -v -timeout 0 ./internal/iptv` serves a synthetic Xtream Codes provider until interrupted, to try Polyfin by hand with a large line-up. `POLYFIN_SYNTHETIC_XTREAM` sets its number of channels, 54,000 for `1`, with a third as many movies and a twentieth as many series; `POLYFIN_SYNTHETIC_XTREAM_ADDR` sets the address it listens on, `127.0.0.1:0` by default. It is skipped otherwise.

## Jellyfin fixtures

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.2 server, recorded in `internal/jellyfin/testdata/`. Jellyfin 12.3 answers the same: recorded again from it with the same scripts, its answers differ only in identifiers, dates, timings and the encoded clips' bitrates, so the 12.2 recordings stand for both. Jellyfin 12.4 changes only its .NET packages and its version, so they stand for it too.

`scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container. It requires Docker, curl and jq.

## Continuous integration

`.github/workflows/ci.yml` runs on pull requests, on pushes to `main` and release tags, each night, and when run by hand. Its first job, "Decide what this run tests", chooses the others:

- A pull request is opened as a draft, whose runs skip every job: CI runs once it is marked ready for review, and on each push after. A push cancels the run of the push before.
- A pull request that changes only documentation (`docs/`, the Markdown files at the top of the repository, the issue and pull request templates, the website's workflow) runs nothing more. The website's workflow checks and builds the changes to `docs/`, `README.md` and `CONTRIBUTING.md`.
- Pull requests skip the macOS jobs: GitHub has few macOS runners, and a job that waits 15 minutes for one fails. They run on pushes to `main`, on release tags and each night.
- A push to `main` runs nothing more when its pull request's last run tested its tree, and passed: the commit `main` was set to by the merge has the tree of the pull request's head, and that head descends from `main`'s commit before the push, so the pull request's run, which tested the head merged into an earlier `main`, tested this very tree. Any doubt, an error of GitHub's API among them, makes a full run.
- Each night at 03:17 UTC, a full run tests `main`, the macOS jobs included.

Go's build and module caches are restored from `main`'s latest run, and only runs on `main` save them, each under a key of its own, so that the next runs start from the latest build. The tests' PostgreSQL keeps its data in memory, and their `pg_dump` and `pg_restore` are the image's, from the Dockerfile's `postgresql-client` stage, as their FFmpeg comes from its `ffmpeg` stage. `.github/workflows/desktop-package.yml` builds each package of Polyfin Desktop, then installs and smokes it: the CI workflow calls it for each package, so that each test waits for its own package only.

## Following Silo

Polyfin answers Silo's apps from Silo's contract, and from the rules its apps apply, at the commits [`internal/silo/followed.json`](../internal/silo/followed.json) records ([Silo's apps](silo-apps.md#the-contract-followed)). Each Monday, and when run by hand, the workflow `.github/workflows/silo-follow.yml` runs `scripts/silofollow`, which compares that record with the main branches of Silo-Server/silo-server, silo-apple and silo-android, keeping none of their code:

- in the contract, the operations Polyfin's routes serve (`silo.Operations`), with their parameters, request bodies and answers, through the schemas they name, leaving out descriptions, summaries and examples;
- in each app, the files the record lists, through GitHub's compare API.

When something changed, the workflow opens the issue "Silo changed since the commits Polyfin follows", labelled `clients`, or updates the open one, with the report; when nothing changed, it does nothing, and on an error it fails. Its token may only read the repository and write issues.

```sh
go run ./scripts/silofollow                                 # the report for today's main branches
go run ./scripts/silofollow -ref silo-apple=<commit>        # another commit of a repository
DRY_RUN=1 GH_REPO=moodiness/polyfin scripts/silofollow/issue.sh report.md   # what the workflow would open or update
```

The tool exits with 0 when nothing changed, 1 when something did, and 2 on an error; `go run` turns every failure into 1, so the workflow builds it first. It reads `GITHUB_TOKEN` or `GH_TOKEN` when set, and waits out GitHub's rate limit; without a token, GitHub's 60 requests an hour are enough for a run.

To follow new commits, once the issue is read:

1. Read the app files that changed, and the rules of `internal/silo/silotest` that cite them. Change the rules and Polyfin's answers where the apps changed, citing the code; a rule citing a new file adds its path to the app's `files` in the record.
2. Read the contract's changes, and change Polyfin's answers where they need it.
3. In `followed.json`, set each `commit` to the one read, the report's "now at", and the contract's `sha256` to the new file's, which system info gives the apps as `contract_digest`: `curl -sL https://raw.githubusercontent.com/Silo-Server/silo-server/<commit>/contracts/api/v2/openapi.json | shasum -a 256`.
4. Run the tool again: it fails when a listed file is not in its app at the commit recorded, or when the SHA-256 is not the contract's. Close the issue; the next change opens a new one.

## Website

The [website](https://moodiness.github.io/polyfin/) is built with [VitePress](https://vitepress.dev/) from `docs/`:

- Each page of this documentation is one of its pages, under `/docs/`, with the sidebar following the sections of the [documentation's index](README.md). Links that leave `docs/` open the file on GitHub.
- The landing page is in `docs/.vitepress/theme/`, and its videos in `docs/public/videos/`.
- VitePress 1 asks for Vite 5, whose development server has security advisories fixed only in Vite 6.4.3. `docs/package.json` overrides it with Vite 6; the override can go once VitePress asks for a fixed Vite.
- Pages follow the [documentation style guide](../CONTRIBUTING.md#writing-the-documentation).

```sh
npm --prefix docs ci
npm --prefix docs run dev       # the website on http://127.0.0.1:5173/polyfin/, with hot reload
npm --prefix docs run build     # fails on a link to a page that does not exist
npm --prefix docs run check     # anchors (GitHub and site slugs), wording, Settings paths
npm --prefix docs run format    # formats the landing page's code
```

CI checks and builds the website for each pull request that changes `docs/`, `README.md`, `CONTRIBUTING.md` or `web/src/i18n/en/nav.ts`. Each push of such a change to `main` publishes it on GitHub Pages (`.github/workflows/pages.yml`).
