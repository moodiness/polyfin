# Large-library bench

`bench.mjs` measures what a user of a large library waits for in the web player: its home page, its live TV guide and its search. It replays the requests jellyfin-web sends for those screens against Polyfin built from this repository, filled with a large IPTV library, prints their times, and exits with 1 when a screen's warm p95 passes its budget.

Run it before each minor release. CI does not run it: it takes several minutes and needs a quiet machine.

It sits next to the screenshot tools because it is the same kind of tool: Node scripts with a `package.json` of their own, so that the documentation's `npm ci` never installs them, run against a local build and a PostgreSQL server, and formatted with the documentation's Prettier (`npm --prefix docs run format:check` covers `docs/scripts`).

## What it needs

- **Node 24** or later.
- **Go,** to build Polyfin and the synthetic provider of `internal/iptv`, a test binary.
- **A PostgreSQL server** the tool may create a database on, such as the one of `compose.yaml`. The bench database is kept between runs (about 130 MB).
- **For `capture.mjs` only:** Chrome or Chromium, and a built jellyfin-web, such as the one in the image (`/usr/share/polyfin/jellyfin-web`).

## Run it

From the repository's root:

```sh
make build
npm --prefix docs/scripts/bench ci
node docs/scripts/bench/bench.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres
```

- **Time:** the first run seeds the bench database in about 6 minutes: the import itself takes 10 seconds, the users' history the rest (each series a user watches is opened once, and Polyfin asks a provider for one title a second). The next runs reuse it. Replaying takes about 4 minutes: 20 warm runs of each screen, about 40 seconds, then 20 cold ones, each right after a restart of Polyfin, about 3 minutes.
- **Quiet:** run it on a machine doing nothing else. Other work moves the times more than the budgets allow for.
- **Output:** a table of the screens, then, with `--requests`, one line per request. Polyfin's processor and memory use follow.
- **Exit code:** 1 when a screen's warm p95 passes its budget, or on an error.
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

They were set from Polyfin 0.78.0 on an Apple M1 Max, with PostgreSQL 18 in a container on the same machine, about a third above the warm p95, Home's more as it varies most (a p95 of 757 ms in an earlier run):

| Screen              | Warm p50 | Warm p95 | Budget |
| ------------------- | -------: | -------: | -----: |
| Home                |      526 |      578 |   1000 |
| Guide               |      184 |      197 |    250 |
| Guide, scrolled     |      171 |      176 |    250 |
| Search, common word |      230 |      263 |    350 |
| Search, rare word   |      470 |      498 |    650 |

## Capture the screens again

When jellyfin-web, Polyfin's script or the screens change, record the requests again. The budgets are kept.

```sh
node docs/scripts/bench/capture.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres \
  --web /path/to/jellyfin-web
```

`--chrome` (or `CHROME_PATH`) gives another browser, and `--shots <folder>` keeps a screenshot of each screen as recorded, to check that it shows what it should. The capture takes half a minute on a seeded database.
