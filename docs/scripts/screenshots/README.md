# Admin app screenshots and sweep

Two tools run against the same demo Polyfin, with a fixed dataset:

- **`take.mjs`** retakes the admin app's screenshots in `docs/images/` in one pass. Run it after a change to the admin app's look, then commit the WebP files that changed.
- **`sweep.mjs`** visits every page of the admin app and fails on what overflows or logs an error (see [The sweep](#the-sweep)). CI runs it on each pull request.

Each starts the demo, works in a local Chrome, then stops everything and drops its database. `stage.mjs` starts and fills the demo for both; `demo.mjs` serves its addons, IPTV list and logos.

## What it needs

- **Node 24** or later.
- **Go and npm,** to build Polyfin with the admin app.
- **FFmpeg and ffprobe** in `PATH`, or `POLYFIN_FFMPEG` and `POLYFIN_FFPROBE`. The tool makes the demo video with FFmpeg, and Polyfin analyzes it with ffprobe.
- **OpenSSL,** which makes the certificate of the demo's logos, served over https.
- **Chrome or Chromium.** The tool looks in the usual install folders. Give another one with `--chrome` or `CHROME_PATH`.
- **A PostgreSQL server** the tool may create and drop a database on, such as the one of `compose.yaml`.

The tool has its own `package.json`, so the documentation's `npm ci` never installs it. It uses `puppeteer-core`, which downloads no browser.

## Run it

From the repository's root:

```sh
make web
CGO_ENABLED=0 go build -tags production \
  -ldflags "-X main.version=$(git describe --tags --abbrev=0 | sed 's/^v//')" \
  -o bin/polyfin ./cmd/polyfin
npm --prefix docs/scripts/screenshots ci
node docs/scripts/screenshots/take.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres
```

The sweep runs the same way, from the same build:

```sh
node docs/scripts/screenshots/sweep.mjs \
  --database-url postgresql://polyfin:password@127.0.0.1:5432/postgres
```

- **Version:** the build takes the latest release's number, so that **Home** shows a version rather than `dev`.
- **Time:** the shots take about a minute and a half, the sweep about three minutes.
- **Disk:** the demo video takes about 280 MB in the system's temporary folder while it runs.
- **Log:** Polyfin's log is left in the temporary folder, as `polyfin-screenshots.log`, or `polyfin-sweep.log` for the sweep. It tells why a step failed.

| Option           | Default                               | What it does                                                                                                                                                                                                              |
| ---------------- | ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--database-url` | `SCREENSHOTS_DATABASE_URL`            | A PostgreSQL address whose user may create databases. The tool connects to it to create the scratch database.                                                                                                             |
| `--database`     | `polyfin_screenshots`                 | The scratch database. The tool drops it first if it exists, and again at the end.                                                                                                                                         |
| `--polyfin`      | `bin/polyfin`, or `POLYFIN_BIN`       | The Polyfin executable.                                                                                                                                                                                                   |
| `--chrome`       | `CHROME_PATH`, else the usual install | The Chrome or Chromium executable.                                                                                                                                                                                        |
| `--out`          | `docs/images`                         | The folder the shots go to.                                                                                                                                                                                               |
| `--only`         | every shot                            | The shots to retake, by name, separated by commas, such as `--only user-page,iptv-source`. The others stay as they are.                                                                                                   |
| `--keep`         | off                                   | Keeps the demo running after the shots, until Ctrl-C or SIGTERM, to look at it in a browser. It prints the address, the user name and the password. Either signal stops everything and drops the database, as at the end. |
| `--rich`         | off                                   | Fills the demo as the sweep does, in a real server's shapes. The shots are then not those of the documentation: use it with `--keep`.                                                                                     |

The sweep takes `--database-url`, `--polyfin` and `--chrome` as well, `--database` with `polyfin_sweep` by default, and:

| Option    | Default | What it does                                                                               |
| --------- | ------- | ------------------------------------------------------------------------------------------ |
| `--jobs`  | 4       | The pages swept at once, each in a browser context of its own.                             |
| `--shots` | none    | A folder for a screenshot of each page with a finding, the elements found outlined in red. |

## The demo

The plain demo is what the shots show. The rich demo, which the sweep runs and `take.mjs --rich` gives, adds what a real server has, in its shapes (counts and lengths of a real one): `enrich()` in `stage.mjs`, marked _Rich_ below.

- **Server:** Polyfin listens on free ports of `127.0.0.1`. Its data folder is temporary, and its secret key is new at each run.
- **Accounts:** Morgan is the administrator. Sam, Robin and Alex are members.
  - _Rich:_ six more members, ten users as on a real server, each member in one of two groups, of five and four members.
- **Devices,** _rich:_ 45 Jellyfin apps are signed in, 23 of them Riley's, with names and apps as long as a real server's.
- **Addon:** `demo.mjs` serves "Demo Catalog", a Stremio addon of the website's fictional titles, with the posters of `docs/public/posters/`. The browser draws Wild Atlas's poster, which the website lacks.
- **More sources,** _rich:_ Demo Catalog's logo, and nine more Stremio addons, enough for the list to scroll:
  - **Logos:** most have one served over https, with transparent parts: round, a wide wordmark, a dark one, a white one, a square one and an SVG. The admin app only shows `https` logos, so the tool serves them over TLS, with a certificate of its own that its Chrome is told to accept; Night Owl Picks has one over http, which the admin app does not try, and Festival Reels none.
  - **Shapes:** Archive Shelf has 161 catalogs, a description of 318 characters and an address of 207, as the largest seen. One name is too long for a row, and Kinoklassikersammlungsarchiv has no space.
  - **Failures:** Harbor Classics' server fails once it is added, a Health problem. Night Owl Picks also has a Live TV catalog.
  - **Own source:** Morgan has Festival Reels as their own. A member's own source may not be on the local network, as the demo's are.
- **Libraries,** _rich:_ six, hidden in Jellyfin apps' menus but those of Live TV, which cannot be.
- **IPTV:** "Demo TV" is an M3U list of five channels with logos and a programme guide: Summit, Deep Blue, Arena, Cinema One and Harbor.
- **Folders,** _rich:_ "Home videos" has two files, and the folder of "Movies on the old disk" is gone since, a Health problem.
- **Lists and channels,** _rich:_ a smart list, an own channel airing it, and a list of Alex's own.
- **Home rows:** four rows show in **Settings › Web player › Home rows**.
- **Notification targets and backups,** _rich:_ a webhook that takes messages, one whose address is refused, and a WebDAV backup destination whose server is full.
- **Tracking services,** _rich:_ five are connected for Alex, each importing, as on a real server.
- **Invites,** _rich:_ five expired, and one open.
- **Activity log,** _rich:_ about 1,200 entries, with names up to 108 characters.
- **Playbacks:** Sam and Robin each play a movie in a Jellyfin app, which the tool imitates. The apps read their streams, so **Home** shows real bandwidth.
- **Written in the database,** _rich:_ the tracking connections (the services are never reached: their last import is recent, and Alex plays nothing), the backup destination's failure (no backup runs without `pg_dump`), the invites' expiry and the activity log. The rest goes through the admin API, as the admin app would.
- **Shown addresses:** the admin app shows the demo's local address as `https://demo.example`. It describes Polyfin as running in a container, as most installs do.
- **Names:** every title, channel, user and device is fictional. Keep it that way: no real addon, site or service names, and no real keys.

## The shots

All are 1440 pixels wide at twice the pixel density, cropped to their subject. Each stays under 150 KB: the WebP quality steps down until it does.

| File                        | Page                      | State                                                          |
| --------------------------- | ------------------------- | -------------------------------------------------------------- |
| `get-started.webp`          | **Home**                  | The Get started card of a fresh server                         |
| `add-a-source-addon.webp`   | **Content › Sources**     | **Add a source**, Stremio addon, with a manifest address typed |
| `home-now-playing.webp`     | **Home**                  | Now playing with two playbacks, one with **Details** open      |
| `top-bar-account-menu.webp` | Any page                  | The top bar with the account panel open, with numbered markers |
| `add-a-source-folder.webp`  | **Content › Sources**     | **Add a source**, Folder, with a name and a path typed         |
| `user-page.webp`            | **Users**                 | A member's page, with its sections                             |
| `home-rows-editor.webp`     | **Settings › Web player** | The Home rows list                                             |
| `iptv-source.webp`          | **Content › Sources**     | An IPTV source's page beside the list, on Summary              |

The markers of `top-bar-account-menu.webp` are numbered in this order: 1, the areas; 2, the search; 3, the account panel. The page that shows the shot explains them.

## Change or add a shot

Each shot is a `shot('<name>', …)` step of `take.mjs`. It opens a page, brings it to the state wanted, and returns the area to keep. Use `clipAround()` around the elements of the subject, with a margin, and `addMarkers()` only where numbers help the text. The admin app lights the control under the mouse or the focus, so end a step with `restMouse()` or `blur()` unless the light is part of the subject. To add data the shots show, extend `seed()` in `stage.mjs` or `demo.mjs`; to add a real server's case for the sweep, extend `enrich()`, which the shots never see.

## The sweep

`sweep.mjs` reads the routes from `web/src/app/router.tsx`, so that a new page is swept without being listed. It visits each as the administrator, as a member (Alex) and signed out, 1440 and 390 pixels wide, in English and German.

- **Addresses:** a route without a parameter is visited as is. Signed in at 1440 pixels in English, the sweep follows each page's links to the routes with parameters, 40 addresses of each at most, then visits those in every other case. Signed out, every address leads to the sign-in page, so one address of each route is enough. A title's versions, which a search opens, and an open invite, which no page links to, are given. An invite that does not exist is visited signed out only: each lookup counts against the address's sign-in attempts, which a dozen exhaust.
- **A route no page links to** fails the sweep: give the demo something that shows it.
- **Findings:**
  - **Overflow:** the page scrolls sideways, or an element goes past its edge, unless a box that scrolls or clips keeps it in.
  - **Text:** text runs out of the nearest box that frames it (a border, a background or its own overflow), or is cut off by it, unless it ends with an ellipsis or a line clamp, or the box scrolls. What only screen readers read is left out.
  - **Console:** any error, but the two expected: the 401 of the session check signed out, and the 404 of an invite that does not exist.
- **Report:** the findings, by page and case, then an exit code of 1. With `--shots`, a screenshot of each page with a finding.
