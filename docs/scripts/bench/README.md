# Large-library bench

`bench.mjs` measures what a user of a large library waits for in the web player: its home page, its live TV guide and its search. It replays the requests jellyfin-web sends for those screens against Polyfin built from this repository, filled with a large IPTV library, prints their times, and exits with 1 when a screen's warm p95 passes its budget. `fluidity.mjs` measures how fast a title opens and plays, in jellyfin-web and for a native app (see [Fluidity](#fluidity)).

Run it before each minor release. CI does not run it: it takes several minutes and needs a quiet machine.

It sits next to the screenshot tools because it is the same kind of tool: Node scripts with a `package.json` of their own, so that the documentation's `npm ci` never installs them, run against a local build and a PostgreSQL server, and formatted with the documentation's Prettier (`npm --prefix docs run format:check` covers `docs/scripts`).

## What it needs

- **Node 24** or later.
- **Go,** to build Polyfin and the synthetic provider of `internal/iptv`, a test binary.
- **A PostgreSQL server** the tool may create a database on, such as the one of `compose.yaml`. The bench database is kept between runs (about 130 MB).
- **For `capture.mjs` and `fluidity.mjs`:** Chrome or Chromium, and a built jellyfin-web, such as the one in the image (`/usr/share/polyfin/jellyfin-web`).
- **For `fluidity.mjs`:** FFmpeg in `PATH`, or `POLYFIN_FFMPEG`, which makes the versions' files once.

## Run it

From the repository's root:

```sh
make build
npm --prefix docs/scripts/bench ci
node docs/scripts/bench/bench.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres
```

- **Time:** the first run seeds the bench database in about 6 minutes: the import itself takes 10 seconds, the users' history the rest (each series a user watches is opened once, and Polyfin asks a provider for one title a second). The next runs reuse it. Replaying takes about 4 minutes: 20 warm runs of each screen, about 40 seconds, then 20 cold ones, each right after a restart of Polyfin, about 3 minutes. A run that seeds replays the warm runs once more before autovacuum's first visit, about 40 seconds more.
- **Quiet:** run it on a machine doing nothing else. Other work moves the times more than the budgets allow for.
- **Polyfin's start:** at each start Polyfin times the GPU's conversions, a few seconds of FFmpeg that slow every request meanwhile. The warm runs wait until it has run no FFmpeg for a second, a minute at most; the cold runs, which measure a start, do not.
- **Output:** a table of the screens, then, with `--requests`, one line per request. A run that seeds prints the same before autovacuum's first visit first (see below), and with `--during-import` while the first import runs. Polyfin's processor and memory use follow.
- **Memory:** the bench samples Polyfin's resident memory (`rssPeakMb`, `rssLastMb` in the JSON) and its Go heap (`heapInusePeakMb`, `heapInuseLastMb`, `heapSysPeakMb`) every half second. On macOS the resident memory follows the most the heap held since Polyfin started, the seed's included, so the warm figure of a run that seeds counts the seed's memory too. The heap in use rises and falls with what Polyfin holds during the phase: compare it between versions. The bench reads the heap from `/metrics` (see [Metrics](../../metrics-and-api-keys.md#metrics)), which it turns on as the bench's administrator; the bench database keeps the setting. A Polyfin whose metrics cannot be turned on gives no heap figures.
- **Exit code:** 1 when a screen's warm p95 passes its budget, settled or before autovacuum's first visit, when a screen is slower before autovacuum's first visit than settled (see below), or on an error.
- **Log:** Polyfin's log is kept in the bench's folder, `polyfin-bench-<database>` in the system's temporary folder.

| Option            | Default                         | What it does                                                                                                                       |
| ----------------- | ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `--database-url`  | `BENCH_DATABASE_URL`            | A PostgreSQL address whose user may create databases.                                                                              |
| `--database`      | `polyfin_bench`                 | The bench database, kept between runs.                                                                                             |
| `--fresh`         | off                             | Seeds the bench database again, as when it is missing or was seeded by another version of the tool.                                |
| `--drop`          | off                             | Drops the bench database and folder at the end.                                                                                    |
| `--polyfin`       | `bin/polyfin`, or `POLYFIN_BIN` | The Polyfin executable.                                                                                                            |
| `--runs`          | 20                              | The warm runs of each screen, after one that warms it up.                                                                          |
| `--cold-runs`     | 20                              | The cold runs of each screen, each right after a restart of Polyfin; 0 for none.                                                   |
| `--during-import` | off                             | Seeds afresh and replays the screens while the first import runs, then measures as usual. The import is short: few runs fit in it. |
| `--requests`      | off                             | Prints each request's times too.                                                                                                   |
| `--json`          | none                            | A file to write every time measured, with the seed's counts.                                                                       |
| `--provider-port` | 47391                           | The port of the synthetic provider. The bench database keeps its address: keep the same port.                                      |

### Before autovacuum's first visit

PostgreSQL's autovacuum counts a table's rows a minute or more after they are written, three minutes on a busy server. Until then the planner has only the statistics Polyfin takes itself, and a table it never analyzed is planned as small: a read that is fast on the settled server may read thousands of rows there. Polyfin analyzes what it writes in bulk at once, the guides' programmes and the IPTV titles.

So each run that seeds, the first one, `--fresh` or `--during-import`, keeps autovacuum away from every table of the bench database, from Polyfin's first start until the users' history is filled. It then replays the screens, warm only, and prints the tables whose statistics count less than half their rows (`Statistics behind`). Autovacuum is let back and the database analyzed before the settled runs. A run that reuses the bench database skips it: its window has passed.

The run fails when a screen passes its budget there, or when its warm p50 there is more than a quarter, and at least 20 ms, above the settled one: the statistics Polyfin takes itself should plan its reads as autovacuum's do. Before a release, run it with `--fresh --during-import`, which covers both. The window the IPTV titles had before Polyfin analyzed them fails it: each title lookup of Next Up read every title of its type, and Home's warm p50 was 293 to 304 ms against 188 to 212 settled.

## The library

`POLYFIN_SYNTHETIC_XTREAM=library` makes the synthetic Xtream Codes provider of `internal/iptv/synthetic_test.go` serve the size of a large library a user reported, the same at each run (seed 1):

- **Live TV:** 2,360 channels in 60 groups, seven in ten in one quality, two in two and one in three. The guide covers 85 % of the channels, from yesterday for four days, with programmes of 15 minutes to 2 hours: 143,873 programmes, one in four named as a show.
- **Movies:** 8,111, in 40 categories.
- **Series:** 8,267, in 82 categories, with 1,151,201 episodes. Their lengths follow a log-normal law, as providers' do: half have fewer than 94 episodes, a tenth more than 300, and 29 more than 1,000, up to 2,023.

The tool adds it as a user would: the administrator's account, then the provider as an IPTV source, with **Movies** and **Series** on, after the preview of its lists. Polyfin then reads its lists and guide. It stores a title as an item once a page lists it, and a series' episodes once it is opened: the users' history lists the libraries whole and opens the series they watch.

## The users

- **Riley** played 300 movies and started 12 more, has 200 series under way, 8 of them among the 30 longest, with the seasons before the current one played, part of the current one, and an episode started in one in ten. Riley has 60 movies, 15 series and 10 channels as favorites, and 40 titles on the Watchlist. The replays are Riley's.
- **Sam, Robin, Alex and Jordan** each played 25 movies, started 3, have 8 series under way, 8 favorites and 6 titles on the Watchlist.
- **Morgan** is the administrator.

## The screens

`screens.json` holds the requests jellyfin-web sent for each screen, in the waves it sent them in, and each screen's budget. A request is in the first wave when it left before any answer of the screen came, else one wave after the latest wave whose answer came before it left. The replay sends a wave's requests at once, and the next wave once they have all answered: a screen's time is the time until its last wave ends.

- **Home:** the home page right after signing in, the web player having kept nothing yet: its settings, the libraries, Continue Watching, Next Up, the latest titles of each library, the programmes on now, and the requests of Polyfin's script and theme. Opened again later, the web player shows what it kept and asks for little.
- **Guide:** the live TV guide's first screen, 500 channels and their programmes for the day; then **Guide, scrolled,** its next page of channels.
- **Search, common word:** `show`, which every series' name has, and one programme in four; **Search, rare word:** `kepnor`, in about one title or programme in 250.

Pictures are not replayed: Polyfin would fetch them from the provider's addresses, which the synthetic provider makes up. The bitrate test the web player runs on a timer is left out too.

An identifier in a request is kept as where it came from: the signed-in user, `{user}`, or a value of an earlier answer of the same screen, such as `{18:Items.0.Id}`, the first item's identifier in the answer of the screen's request 18. Times are kept relative to the request, such as `{time:86398800}`. So the replay asks for the same things against a database seeded again.

## Budgets

Each screen's `budget.warmP95`, in milliseconds, in `screens.json`, is its warm p95 measured with some room, so that noise does not fail the run. Lower a budget when a change makes a screen faster, so that the bench keeps it; raise one only on purpose, in the change that needs it.

They are set about a third above the highest warm p95 measured, rounded up, on an Apple M1 Max, with PostgreSQL 18 in a container on the same machine. Home, Search, common word and Search, rare word were lowered on 2026-11-16, from Polyfin 0.79.0's release run (20 runs, below) and the highest warm p95 measured on its code in other runs (Home 299, common word 88, rare word 72). The guide's budgets were set from 0.78.0 (warm p95 197 and 176) and are kept.

| Screen              | Warm p50 | Warm p95 | Budget |
| ------------------- | -------: | -------: | -----: |
| Home                |      200 |      243 |    400 |
| Guide               |      179 |      186 |    250 |
| Guide, scrolled     |      173 |      178 |    250 |
| Search, common word |       79 |       83 |    120 |
| Search, rare word   |       55 |       60 |    100 |

## Capture the screens again

When jellyfin-web, Polyfin's script or the screens change, record the requests again. The budgets are kept.

```sh
node docs/scripts/bench/capture.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres \
  --web /path/to/jellyfin-web
```

`--chrome` (or `CHROME_PATH`) gives another browser, and `--shots <folder>` keeps a screenshot of each screen as recorded, to check that it shows what it should. The capture takes half a minute on a seeded database.

## Fluidity

`fluidity.mjs` measures what a user waits for from a title's card to its video playing: in jellyfin-web, with Polyfin's theme and with jellyfin-web's own look, and in a native app, which calls the API itself. It tells where the time goes: Polyfin's answers, the addon's, the source file, pictures, jellyfin-web's files, and the page's own work. It has no budgets: it is for finding what to make faster, before and after a change.

```sh
make build
npm --prefix docs/scripts/bench ci
node docs/scripts/bench/fluidity.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres \
  --web /path/to/jellyfin-web --report fluidity.md
```

- **Stage:** a scratch database, `polyfin_fluidity`, dropped at the end; Polyfin on a temporary data folder; and a fixture addon on `127.0.0.1`, "Fluidity bench", with a catalog, a library, for each case.
- **Files:** the fixture's versions are 3 minutes of 1080p at 24 pictures a second: H.264 at 8 Mbps and AAC in MP4, the same in Matroska, and HEVC at 6 Mbps in Matroska. FFmpeg makes them at the first run, in about 2 minutes, and they stay in the system's temporary folder, `polyfin-fluidity-media` (about 500 MB). Titles say they last 20 minutes, so that Polyfin takes the files for the title rather than for a clip.
- **Addon:** every answer of the fixture waits `--latency`, as over the internet; stream lists wait `--stream-delay`, as an addon that gathers others does; files are sent at `--source-rate`.
- **Browser:** Chrome plays Matroska and, on macOS, HEVC, which most browsers do not. The tool makes it say it plays neither, so that jellyfin-web asks for the Matroska version repackaged and the HEVC one converted. `--mkv` and `--hevc` let it play them.
- **Time:** about 10 minutes a case with 10 runs, most of it in the cold runs' restarts. A run that fails is told, with a screenshot in `--shots`, and left out; the tool then exits with 1.

| Option            | Default                         | What it does                                                                                 |
| ----------------- | ------------------------------- | -------------------------------------------------------------------------------------------- |
| `--database-url`  | `BENCH_DATABASE_URL`            | A PostgreSQL address whose user may create databases.                                        |
| `--web`           | `POLYFIN_WEB_DIR`               | A folder of jellyfin-web, built.                                                             |
| `--polyfin`       | `bin/polyfin`, or `POLYFIN_BIN` | The Polyfin executable.                                                                      |
| `--chrome`        | `CHROME_PATH`, else the usual   | The Chrome or Chromium executable.                                                           |
| `--runs`          | 10                              | The runs of each case, warm and cold.                                                        |
| `--modes`         | `warm,cold`                     | The runs made: warm, cold or both.                                                           |
| `--only`          | every case                      | Cases, or scenarios (`open`, `play`, `native`, `home`), separated by commas.                 |
| `--stream-delay`  | 1500                            | Milliseconds the fixture's stream lists take.                                                |
| `--latency`       | 50                              | Milliseconds every answer of the fixture waits first, files and pictures included.           |
| `--source-rate`   | 12                              | MB a second the fixture sends files at.                                                      |
| `--mkv`           | off                             | Lets jellyfin-web play Matroska as it is.                                                    |
| `--hevc`          | off                             | Lets Chrome play HEVC where it can.                                                          |
| `--json`          | none                            | A file for every run, with its requests and the fixture's answers.                           |
| `--report`        | none                            | A Markdown file with the table, the slowest requests and a waterfall for each case.          |
| `--log`           | none                            | A file to keep Polyfin's log in.                                                             |
| `--shots`         | none                            | A folder for a screenshot of each run that fails.                                            |
| `--keep`          | off                             | Keeps the stage running, to look at it in a browser, until Ctrl-C; measures nothing.         |
| `--from`          | none                            | `--json` files of earlier runs, separated by commas: writes their report, measuring nothing. |
| `--fresh`         | off                             | Starts each cold run on an empty database, where no title was ever listed, at `--place`.     |
| `--place`         | 0                               | With `--fresh`, the title the cold runs open: its place in its row, 0 for the first.         |
| `--home-wait`     | 0                               | Milliseconds a run stays on the home page once it settled, before it clicks the card.        |
| `--revisit`       | 0                               | For the home case, milliseconds between two loads of the home page while it stays open.      |
| `--stream-status` | 200                             | The status of the fixture's stream answers: 429 refuses each one for its rate.               |
| `--titles`        | 0                               | The titles of each catalog at least, beyond one for the warm runs and one per cold run.      |

### The cases

| Case                                         | From                                                          | To                                                                                                                         |
| -------------------------------------------- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- |
| **Open,** with LumaaGlaass or without        | A click on the title's card on the home page                  | Its page shows its name, its backdrop drawn (with LumaaGlaass, the poster it shows in its place) and a Play button enabled |
| **Play at once,** MP4, MKV or HEVC           | Play, pressed in the frame it is enabled                      | The video's `playing` event                                                                                                |
| **Play 5 s after opening,** MP4, MKV or HEVC | Play, pressed 5 seconds after the card's click                | The same                                                                                                                   |
| **Native app,** at once or 5 s after opening | The title's details, then its `PlaybackInfo`, 5 seconds later | The first MiB of its stream, as a direct play                                                                              |
| **Home page,** addon requests                | Signing in, which shows the home page                         | `--home-wait` later, the page loaded again every `--revisit`: the fixture's requests by kind                               |

- **Play held:** Polyfin's script holds Play, disabled, until the title has a version, which a title never opened waits for its addons to list. The table's other column tells when the page was first shown, Play held or not (`shown`), and, for a play, the time from the card's click (`from card`).
- **Warm:** a first run that is not counted, then `--runs` runs of the same title, in the same browser.
- **Cold:** each run after a restart of Polyfin, in a browser that kept nothing, signed in afresh, on a title never opened.
- **Native app:** it plays MP4 and Matroska, H.264 and HEVC, as players built on FFmpeg do, so every version plays as it is.

### The report

For each case and mode: p50 and p95, the requests and kilobytes until the end, not counting those served from the browser's cache, and how the version played. The split gives each millisecond of the runs to the first of these under way, then takes the p50 of each:

- `api`: Polyfin answering an API request;
- `media`: Polyfin's video, playlist or segment, until its first byte;
- `addon`: Polyfin waiting for the fixture's metadata or stream lists;
- `source`: Polyfin reading the fixture's file, to analyze it or to send it on;
- `image`, `asset`, `theme`: pictures, jellyfin-web's files, and the theme's and Polyfin's script;
- `client`: nothing under way, the page's own work.

The report then lists each case's slowest requests, with the time Polyfin took before its first byte, and the waterfall of the median run, the fixture's answers among them.
