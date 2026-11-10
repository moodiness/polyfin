# Development

This page is for contributors who change Polyfin's code or its documentation. It covers running Polyfin from source, its tests, the Jellyfin fixtures, what CI runs and when, following Silo's changes, working on the website, and building and signing Polyfin Desktop's packages.

## Requirements

- Go 1.27.1.
- Node.js 24.
- A PostgreSQL 18 server.
- FFmpeg 9.0 (`ffmpeg` and `ffprobe`), to play titles: the version the image ships and the tests run with.

With `POSTGRES_PASSWORD` set in `.env`, this starts a PostgreSQL server on `127.0.0.1:5432`:

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d postgres
```

## Commands

```sh
export POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin
make dev                      # builds the admin app, then runs the server on :8096
npm --prefix web run dev      # optional: admin app with hot reload, proxied to :8096
make check                    # formatting, vet and tests
```

`make check` builds the admin app first, as its formatting check needs it.

### The web player's script

`internal/server/webclient.js` is the script Polyfin adds to jellyfin-web's page, written in ES5. The page loads `webclient.min.js` beside it, the same script minified, which the server embeds. After a change to `webclient.js`, run `go generate ./internal/server` to write it again: `TestTheScriptIsServedMinified` fails while it is not the source minified.

### French punctuation

French texts set `:`, `;`, `?`, `!` and the inside of `« »` apart with a narrow no-break space (U+202F), written `\u202f`, so that a line never breaks before the sign. After a change of French texts, run from `web/`:

```sh
npx --yes --package=typescript@5.9.3 -- node scripts/french-spacing.mjs
```

- **What it rewrites:** a plain or no-break space before these signs, or after `«`, in the strings of the admin app's French messages (`web/src/i18n/fr`), of the French words of `internal/server/webclient.js`, and of the French of each `i18n.Text` and each Go map by language, never code. It then regenerates `webclient.min.js` when the script changed.
- **What it keeps:** a sign with no space before it (`10:30`, `http://`), and a sign a letter or a digit follows, which is syntax rather than punctuation (`Ajoutez :port`).
- **Again:** a second run changes nothing.
- **The checks:** the i18n check (`scripts/i18n-check.mjs`) fails on a French message or a word of the script with a plain or no-break space there, and `TestFrenchSpacing` (in `internal/i18n`) on a Go text; `go generate ./internal/i18n` rewrites the Go texts alone.

## Tests

Some tests need a program or a service, and run only when a variable names it. Without it, they are skipped.

| Tests | Run when |
|---|---|
| Database tests | `POLYFIN_TEST_DATABASE_URL` points to a disposable PostgreSQL database whose role may create databases. |
| Tests that run FFmpeg | `POLYFIN_TEST_FFMPEG` names an `ffmpeg` executable with `ffprobe` beside it. |
| Backup and restore tests | `POLYFIN_TEST_PG_DUMP` and `POLYFIN_TEST_PG_RESTORE` name PostgreSQL 18's `pg_dump` and `pg_restore`. |
| OCR tests | `POLYFIN_TEST_TESSERACT` names a Tesseract executable. They use the data of `POLYFIN_TEST_TESSDATA` when set. |
| The tests that run PostgreSQL's server (`internal/cluster`, `internal/desktop/app`, and the embedded database's in `cmd/polyfin`) | `POLYFIN_TEST_POSTGRES_BIN` names the folder of PostgreSQL 18's server programs. |
| The test of a torrent route's interface going down | `POLYFIN_TEST_TORRENT_INTERFACE` names a dummy interface with an address, with the right to set it down and up (`NET_ADMIN`). |
| SMB share tests | `POLYFIN_TEST_SMB` names a share holding video files (see [SMB shares](#smb-shares)). |

- **Databases:** most database tests work in a database of their own, copied from a template database. The template is migrated once and kept for later runs, named `polyfin_template_` followed by a key of the migrations. The migrations' own tests start from an empty database of their own, copied from `polyfin_template_empty`. A few other tests work in a schema of their own.
- **Polyfin Desktop's ports:** `POLYFIN_TEST_POSTGRES_PORT` gives PostgreSQL a port, else the tests take a free one. Both packages read it, so a fixed port needs them tested apart.
- **The desktop app's port:** `POLYFIN_TEST_DESKTOP_PORT` gives the app's tests their listening port, else they take a free one.
- **On CI:** the workflows provide:
  - the database, FFmpeg, `pg_dump` and `pg_restore`, and Tesseract to the test jobs;
  - PostgreSQL's server programs to the Polyfin Desktop jobs;
  - a dummy interface to the torrent route test.

### How the tests run

`make check` runs every package's tests under the race detector (`make test`), the slowest packages first. Then it runs, without the race detector, the time bounds of a large IPTV line-up (`make bounds`).

- The race detector slows that work tenfold.
- Under the race detector, the bounds test gets a tenth of the line-up.

CI runs the tests in four jobs at once:

| Job | Tests |
|---|---|
| `make test-jellyfin-1` | The first half of `internal/jellyfin`, by far the longest package: tests whose name has A to K after `Test`. |
| `make test-jellyfin-2` | The other half of `internal/jellyfin`. |
| `make test-race-1` | The first share of the other packages, balanced with the second by their times on CI. |
| `make test-race-2` | The second share, and every package neither share lists. |

A new package goes to the second share, and a new test of `internal/jellyfin` to the half its name falls in.

### Test times

These four jobs run the tests through `go test -json`, by [gotestsum](https://github.com/gotestyourself/gotestsum), and check each test's time with `scripts/test-times.py`:

- **Over 30 seconds:** a test a pull request adds fails its job, unless `.github/slow-tests.txt` lists it with the reason. Any other test over 30 seconds gets a warning, as every test does outside pull requests: an existing test that a busier runner pushes over the limit fails nothing. A subtest's time counts in its test's.
- **The job's summary:** the tests over 30 seconds, the slowest tests, and the packages whose tests run one after the other. An allowlisted test now under 30 seconds is listed too, so that its line can go.
- **The run's artifacts:** each job keeps `test-times-<job>-<part>`, with go test's events and `durations.tsv`, every test's time. `over-limit.txt` holds the lines the allowlist lacks.

The same check runs on any `go test -json` output. `--base` names the commit whose diff to `HEAD` holds the new tests, which may fail; without it, nothing fails:

```sh
go test -json -race ./internal/lyrics > times.json
python3 scripts/test-times.py --allowlist .github/slow-tests.txt --base origin/main times.json
```

### Tests in parallel

A test calls `t.Parallel()` first, and so does each of its subtests: a package's tests run at once.

- A test that changes state other tests read, such as a package variable, stays sequential and says why.
- So does a test that times waits of a few milliseconds.
- A test that counts goroutines labels its own with `runtime/pprof` labels, and counts those alone. So it runs in parallel too (see `TestPlaysThatGaveUpLeaveNoGoroutineBehind`).
- A test that restores a whole database calls `testdb.LockRestores(t)` after `t.Parallel()`, and runs in parallel: a restore holds thousands of entries of the test server's shared lock table until it commits, and two at once, from any package, overflow CI's (`out of shared memory`). The lock lets one restore run at a time across every test binary.
- A test calling `t.Setenv` or `t.Chdir` cannot run in parallel, and needs no reason.

On a pull request, CI checks the lines it changes in test files with the `paralleltest` and `tparallel` linters: a new test runs in parallel or says why not, and a change inside an older test asks nothing of the rest of its file. A test that stays sequential gives its reason on the line above it, in the comment golangci-lint reads:

```go
// What the test checks.
//
//nolint:paralleltest // swaps the package's clock, which every test reads
func TestSomething(t *testing.T) {
```

`//nolint:paralleltest,tparallel // reason` lets a test's subtests run in turn too. The same check, on the packages of the test files a branch changes:

```sh
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run --enable-only paralleltest,tparallel \
  --new-from-rev origin/main ./internal/lyrics/
```

### Building for 32-bit targets

No image or desktop package is built for 32-bit targets. Polyfin is kept building there, and safe on its playback path, by a CI job that runs beside the others, "32-bit build and playback tests, and 64-bit atomics":

- **Build:** it builds the server for 32-bit ARM (`GOARCH=arm GOARM=7`), and vets every package for 386, tests included. A test's constant past 2^31 where an `int` goes is written `int64(...)`.
- **Tests:** it runs the tests of the playback path as 386 binaries, which the runner runs natively, without FFmpeg: `internal/container`, `source`, `media`, `hls`, `subtitles`, `stremio`, `localfiles`, `remuxdb` and `playback`.
- **Atomics:** it checks every package with one rule of golangci-lint's forbidigo, in `.github/golangci-atomics.yml`: no raw 64-bit function of `sync/atomic`, such as `atomic.AddInt64` or `atomic.LoadUint64`. On a 32-bit target, such a call panics when its value is not 8-byte aligned, as a field after a pointer is not. `atomic.Int64` and `atomic.Uint64` take their place: the compiler aligns them.

A size or offset past 2 GiB is an `int64` on every target, never an `int`; an addon's is decoded as a `stremio.Number64`. The same checks:

```sh
GOOS=linux GOARCH=arm GOARM=7 go build ./...
GOOS=linux GOARCH=386 go vet ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 run --config .github/golangci-atomics.yml ./...
```

`GOARCH=386 go test ./internal/playback` runs the tests on Linux on amd64; elsewhere, build them with `GOOS=linux GOARCH=386 go test -c` and run them in a `linux/386` container.

### Password and key costs in tests

The tests of `internal/accounts`, `internal/admin`, `internal/backup` and `internal/jellyfin` hash passwords and derive backup keys at a small fraction of the server's cost.

- Their `TestMain` sets it with `accounts.LowerPasswordCostForTests` and `backup.LowerKeyCostForTests`, which panic outside a test binary.
- Another package's tests can call them the same way.
- Under the race detector, the server's cost takes about a quarter of a second for each password, and two seconds for each backup key.
- A password hash records its cost, so it verifies the same way at either cost.
- A backup does not, so a backup sealed in such a binary opens only there.

### SMB shares

The SMB share tests run when `POLYFIN_TEST_SMB` names a share holding video files, with its user and password in the address: `smb://user:password@host:port/share/folder`.

- The user must be allowed to write there: the change notification test adds, renames and removes files in a folder of its own, removed after.
- CI does not run them.
- WebDAV folders are tested against a server in the tests themselves.

### The image

`make image-check` builds the image and checks the version it prints.

`make image-health` then starts it against the PostgreSQL of `IMAGE_DATABASE_URL`, and waits for its health check to report it healthy, as CI does. By default, `IMAGE_DATABASE_URL` is the host's PostgreSQL on port 5432, with the user, password and database `polyfin`.

The image's FFmpeg comes from an archive the Dockerfile downloads for each platform, and checks against its SHA-256. CI keeps those archives in its cache, so that an outage of the download fails no run, and builds with them; `make image-check FFMPEG_ARCHIVES=<folder>` does the same with a folder `scripts/ffmpeg-archives.sh fetch <folder>` filled.

### A synthetic Xtream Codes provider

This serves a synthetic Xtream Codes provider until interrupted, to try Polyfin by hand with a large line-up:

```sh
POLYFIN_SYNTHETIC_XTREAM=1 go test -run TestServeSyntheticXtream -v -timeout 0 ./internal/iptv
```

| Variable | What it sets | Default |
|---|---|---|
| `POLYFIN_SYNTHETIC_XTREAM` | The number of channels, 54,000 for `1`, with a third as many movies and a twentieth as many series. | Unset: the test is skipped. |
| `POLYFIN_SYNTHETIC_XTREAM_ADDR` | The address the provider listens on. | `127.0.0.1:0` |

## Jellyfin fixtures

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.2 server, recorded in `internal/jellyfin/testdata/`.

- **Jellyfin 12.3:** it answers the same. Recorded again from it with the same scripts, its answers differ only in identifiers, dates, timings and the encoded clips' bitrates. So the 12.2 recordings stand for both.
- **Jellyfin 12.4:** it changes only its .NET packages and its version, so the recordings stand for it too.

`scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container. It requires Docker, curl, jq, `ffmpeg` and `ffprobe`.

## Continuous integration

`.github/workflows/ci.yml` runs on pull requests, on pushes to `main` and release tags, each night, and when run by hand. Its first job, "Decide what this run tests", chooses the others:

- **Drafts:** a pull request is opened as a draft, whose runs skip every job. CI runs once it is marked ready for review, and on each push after.
- **Pushes to a pull request:** a push cancels the run of the push before.
- **Documentation only:** a pull request that changes only documentation runs nothing more. Documentation means `docs/`, the Markdown files at the top of the repository, the issue, discussion and pull request templates, and the website's workflow. The website's workflow checks and builds the changes to `docs/`, `README.md` and `CONTRIBUTING.md` (see [Website](#website)).
- **macOS:** pull requests skip the macOS jobs. GitHub has few macOS runners, and a job that waits 15 minutes for one fails. They run on pushes to `main`, on release tags, each night and when CI is run by hand.
- **Nightly:** each night at 03:17 UTC, a full run tests `main`, the macOS jobs included. GitHub may start a scheduled run hours late, or drop it when Actions is busy. Running CI by hand on `main` runs the same jobs.

A push to `main` runs nothing more when its pull request's last run tested its tree, and passed.

- The merge set `main` to a commit with the tree of the pull request's head.
- That head descends from `main`'s commit before the push.
- So the pull request's run, which tested the head merged into an earlier `main`, tested this very tree.
- Any doubt, an error of GitHub's API among them, makes a full run.

How the jobs are set up:

- **Go caches:** Go's build and module caches are restored from `main`'s latest run. Only runs on `main` save them, each under a key of its own, so that the next runs start from the latest build. A job's leg with no cache of its own, such as a new leg of a split job, starts from another leg's on the same system.
- **PostgreSQL:** in the Linux test jobs, the tests' PostgreSQL keeps its data in memory.
- **Image tools:** in the Linux test jobs, `pg_dump` and `pg_restore` are the image's, from the Dockerfile's `postgresql-client` stage. Their FFmpeg comes from its `ffmpeg` stage.
- **Tesseract:** the race jobs keep the Tesseract packages apt fetches in a cache, keyed by their checksums, as Ubuntu's mirror sometimes sends them at a few kB/s. A cached package without the checksum apt expects is fetched again, and a cached set that fails to install is fetched in full.
- **Windows:** the Windows tests run in two jobs, balanced by the packages' times on the Windows runner. `internal/playback` and the smaller packages run in one, `internal/localfiles` and `internal/hls` in the other.
- **Polyfin Desktop's packages:** `.github/workflows/desktop-package.yml` builds each package, then installs and smokes it. The CI workflow calls it for each package, so that each test waits for its own package only. Every package embeds the admin app one Linux job builds, through the scripts' `SKIP_WEB=1`: `npm ci` alone takes about a minute and a half on Windows on Arm.
- **The admin app's bundle:** after the build, `npm --prefix web run check:bundle` fails when the JavaScript the first page loads, the entry file with the files it imports, passes its gzip budget, or when one file holds the messages of more than one language. The budget is in `web/scripts/check-bundle.mjs`; raise it on purpose, in the change that needs it.

## Following Silo

Polyfin answers Silo's apps from Silo's contract, and from the rules its apps apply, at the commits [`internal/silo/followed.json`](../internal/silo/followed.json) records (see [Silo's apps](silo-apps.md#the-contract-followed)).

Each Monday, and when run by hand, the workflow `.github/workflows/silo-follow.yml` runs `scripts/silofollow`. It compares that record with the main branches of Silo-Server/silo-server, silo-apple and silo-android, keeping none of their code:

- **In the contract:** the operations Polyfin's routes serve (`silo.Operations`), with their parameters, request bodies and answers, through the schemas they name. It leaves out descriptions, summaries and examples.
- **In each app:** the files the record lists, through GitHub's compare API.

What the workflow does:

- When something changed, it opens the issue "Silo changed since the commits Polyfin follows", labelled `clients`, with the report. If that issue is open already, it updates it.
- When nothing changed, it does nothing.
- On an error, it fails.
- Its token may only read the repository and write issues.

```sh
go run ./scripts/silofollow                                 # the report for today's main branches
go run ./scripts/silofollow -ref silo-apple=<commit>        # another commit of a repository
DRY_RUN=1 GH_REPO=moodiness/polyfin scripts/silofollow/issue.sh report.md   # what the workflow would open or update
```

| Exit code | Meaning |
|---|---|
| 0 | Nothing changed. |
| 1 | Something changed. |
| 2 | An error. |

- `go run` turns every failure into 1, so the workflow builds the tool first.
- The tool reads `GITHUB_TOKEN` or `GH_TOKEN` when set.
- It waits out GitHub's rate limit, for up to 15 minutes. Without a token, GitHub's 60 requests an hour are enough for a run.

To follow new commits, once the issue is read:

1. Read the app files that changed, and the rules of `internal/silo/silotest` that cite them. Change the rules and Polyfin's answers where the apps changed, citing the code. A rule citing a new file adds its path to the app's `files` in the record.
2. Read the contract's changes, and change Polyfin's answers where they need it.
3. In `followed.json`, set each `commit` to the one read, the report's "now at". Set the contract's `sha256` to the new file's, which system info gives the apps as `contract_digest`: `curl -sL https://raw.githubusercontent.com/Silo-Server/silo-server/<commit>/contracts/api/v2/openapi.json | shasum -a 256`.
4. Run the tool again. It fails when a listed file is not in its app at the commit recorded, or when the SHA-256 is not the contract's.
5. Close the issue. The next change opens a new one.

## Website

The [website](https://moodiness.github.io/polyfin/) is built with [VitePress](https://vitepress.dev/) from `docs/`.

- Each page of this documentation is one of its pages, under `/docs/`. The sidebar follows the sections of the [documentation's index](README.md).
- Links that leave `docs/` open the file on GitHub.
- The landing page is `docs/index.md`, drawn by the theme in `docs/.vitepress/theme/`. Its videos are in `docs/public/videos/`.
- VitePress 1 asks for Vite 5, whose development server has security advisories fixed only in Vite 6.4.3. `docs/package.json` overrides it with Vite 6. The override can go once VitePress asks for a fixed Vite.
- Pages follow the [documentation style guide](../CONTRIBUTING.md#writing-the-documentation).

```sh
npm --prefix docs ci
npm --prefix docs run dev       # the website on http://127.0.0.1:5173/polyfin/, with hot reload
npm --prefix docs run build     # fails on a link to a page that does not exist
npm --prefix docs run check     # anchors (GitHub and site slugs), old anchors and redirects, wording, Settings paths
npm --prefix docs run format    # formats the website's configuration, theme and scripts, and the landing page
```

CI checks and builds the website for each pull request that changes `docs/`, `README.md`, `CONTRIBUTING.md`, `web/src/i18n/en/nav.ts` or the website's workflow. Each push of such a change to `main` publishes it on GitHub Pages (`.github/workflows/pages.yml`).

## Polyfin Desktop packages

This section is for contributors who run Polyfin Desktop from source, build its packages or sign its releases. For using the app, see [Polyfin Desktop](desktop.md).

For development and tests, these variables change where the app runs from. The server never reads them.

| Variable | What it does |
| --- | --- |
| `POLYFIN_DESKTOP_INSTALL_DIR` | Install folder to run the programs from, instead of the app's own. It holds `polyfin` (`polyfin.exe`), `postgresql/bin`, `postgresql/lib`, `postgresql/share`, `jellyfin-web` and `fonts`, as the packages lay them out |
| `POLYFIN_DESKTOP_DATA_DIR` | Data folder, as an absolute path, instead of the system's usual place |
| `POLYFIN_DESKTOP_DATABASE_PORT` | Port of the database, instead of a free one |
| `POLYFIN_DESKTOP_TOOLS` | JSON file of the FFmpeg downloads, instead of the pinned ones: a local server's, for tests |

The other variables, such as `POLYFIN_LISTEN`, reach the server as they are.

`polyfin-desktop -headless` runs the app without the icon, the browser and the login item, and logs to the terminal too. It runs until it is interrupted or `-quit` quits it.

`scripts/desktop-smoke.py` checks that quitting leaves no process. CI runs it on Linux, Windows and macOS. It starts the app twice, with:

- a data folder of its own;
- a local server for FFmpeg;
- a proxy that refuses every request for outside.

It quits the app with `-quit`, except the first start on Linux and macOS, which it interrupts. On Windows, it runs the build without a console, as the packages do:

- the second start runs with the icon, which must show when explorer.exe runs in the session;
- otherwise the app must log that it runs without the icon;
- `DESKTOP_SMOKE_NO_TRAY=1` expects that log even with explorer.exe. CI sets it on Windows on Arm, whose runners' sessions refuse every notification icon, x64 programs' included.

See [Commands](#commands) to build the server and the admin app. The app itself builds with `go build ./cmd/polyfin-desktop`, which needs cgo on macOS, for the menu bar.

### Building the packages

The scripts of `scripts/packaging` build the packages into `dist/packages`, as CI does. They use the version in `VERSION`, or `dev` without one. Each needs Go, Node.js for the admin app, Python 3 and Docker:

| Script | Builds | Where |
| --- | --- | --- |
| `macos.sh arm64`, `macos.sh x64` | The `.dmg` for Apple silicon or Intel | On a Mac |
| `windows.sh x64` | The Windows x64 installer, with NSIS | On Linux or macOS |
| `windows.sh arm64` | The Windows arm64 installer, with NSIS | On Windows on Arm, in Git Bash |
| `linux.sh amd64`, `linux.sh arm64` | The `.deb` and the AppImage | On Linux or macOS |

Each takes PostgreSQL 18 from a pinned build:

| System | PostgreSQL build |
| --- | --- |
| Windows x64, macOS | EnterpriseDB's binaries, which postgresql.org links |
| Linux | A build from PostgreSQL's source made by `postgresql-linux.Dockerfile`, on Ubuntu 22.04 with ICU, which runs from any folder |
| Windows on Arm | A build from the same source by `postgresql-windows.sh`, as EnterpriseDB makes none |

For Windows on Arm, `postgresql-windows.sh`:

- applies PostgreSQL's upstream change for Visual Studio on Arm (`postgresql-msvc-arm64.patch`);
- builds with Meson and Visual Studio 2022's ARM64 build tools, with ICU's own ARM64 build and zlib;
- puts Visual Studio's runtime beside the programs;
- then makes a cluster with ICU, and restores a backup made with `pg_dump`.

`windows.sh arm64` runs it, and keeps the build in `dist/packaging/postgresql-windows-arm64` while its inputs stay the same. It needs:

- Visual Studio 2022 with the ARM64 build tools;
- a native Perl, such as Strawberry Perl;
- Ninja and NSIS, as GitHub's `windows-11-arm` runners have them;
- Docker, only when `POLYFIN_DESKTOP_FILES` names no folder made by `files.sh`.

jellyfin-web and the fonts come from the Docker image's stages, through `files.sh`. On a Mac without Docker, give `POLYFIN_DESKTOP_FILES` the folder that `files.sh` made elsewhere. `POLYFIN_DESKTOP_FILES_CACHE` names a buildx cache its build takes its layers from and keeps them in, as CI's `type=gha,scope=desktop-files`. Every download is checked against its checksum, and kept in `~/.cache/polyfin-packaging`.

`scripts/packaging/test.sh <package>` tests a package on the system it is for:

1. it installs or opens the package, and checks what it holds;
2. it runs `desktop-smoke.py --installed` on the installed app, which then finds its programs itself;
3. it removes the package.

- **CI:** it runs on Windows, macOS and Linux runners for every package, before a release gets them.
- **On Windows:** it also checks that the version info of `polyfin.exe` and `polyfin-desktop.exe` is the package's. `windows.sh` adds that info, with Polyfin's icon, using go-winres.
- **It changes the system it runs on:** run it on a throwaway machine or container.

### The release signing key

The release workflow signs each release's manifest with the private key in the repository secret `POLYFIN_RELEASE_SIGNING_KEY`.

- **Renewal:** `.github/workflows/desktop-manifests.yml` renews the manifests of the 10 latest releases every Monday with that key, each only once its signature checks.
- **Tool:** both workflows use `cmd/polyfin-release-sign`.
- **Public keys:** the app carries the two public keys of `releaseKeys`, in `internal/desktop/update/manifest.go`.

A release stops at its step `Check the release signing key` while the secret is missing, or while its public key is not one the app carries. The packages it would publish could never be installed by an update.

Once, on a computer you trust, before the first release with signed manifests:

1. Make two key pairs: `go run ./cmd/polyfin-release-sign keygen -out release.key`, then `go run ./cmd/polyfin-release-sign keygen -out spare.key`. Each writes its private key to the file, readable by you only, and prints its public key.
2. Put both public keys in `releaseKeys`, separated by a comma, and commit them.
3. Store the signing key as the secret: `gh secret set POLYFIN_RELEASE_SIGNING_KEY < release.key`, or paste the file's line under the repository's Settings › Secrets and variables › Actions. Then remove `release.key`, or keep it offline.
4. Keep `spare.key` offline, never in the repository's secrets.

`polyfin-release-sign` also checks keys and manifests:

- `go run ./cmd/polyfin-release-sign key`, with the secret's value in `POLYFIN_RELEASE_SIGNING_KEY`, checks a key and prints its public key;
- `verify -in <manifest> [packages]` checks a manifest as the app does.

To change the signing key, such as when it may have leaked:

1. Store `spare.key` as the secret.
2. Make a new spare with `keygen`.
3. Put its public key in `releaseKeys`, in place of the old signing key's.
4. Release.

Apps of earlier releases accept the spare's signatures, as they carry its key. The new release no longer accepts the old key; apps that do not update meanwhile still accept it.

GitHub stops the scheduled workflows of a repository without activity for 60 days. Run the `Polyfin Desktop manifests` workflow by hand then, before the manifests expire.
