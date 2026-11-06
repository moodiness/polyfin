# Admin app screenshots

This tool retakes the admin app's screenshots in `docs/images/` in one pass. Run it after a change to the admin app's look, then commit the WebP files that changed.

It starts a demo Polyfin with a fixed dataset, takes each shot in a local Chrome, then stops everything and drops its database.

## What it needs

- **Node 24** or later.
- **Go and npm,** to build Polyfin with the admin app.
- **FFmpeg and ffprobe** in `PATH`, or `POLYFIN_FFMPEG` and `POLYFIN_FFPROBE`. The tool makes the demo video with FFmpeg, and Polyfin analyzes it with ffprobe.
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

- **Version:** the build takes the latest release's number, so that **Home** shows a version rather than `dev`.
- **Time:** a run takes about a minute.
- **Disk:** the demo video takes about 280 MB in the system's temporary folder while it runs.
- **Log:** Polyfin's log is left in the temporary folder, as `polyfin-screenshots.log`. It tells why a step failed.

| Option           | Default                               | What it does                                                                                                                             |
| ---------------- | ------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `--database-url` | `SCREENSHOTS_DATABASE_URL`            | A PostgreSQL address whose user may create databases. The tool connects to it to create the scratch database.                            |
| `--database`     | `polyfin_screenshots`                 | The scratch database. The tool drops it first if it exists, and again at the end.                                                        |
| `--polyfin`      | `bin/polyfin`, or `POLYFIN_BIN`       | The Polyfin executable.                                                                                                                  |
| `--chrome`       | `CHROME_PATH`, else the usual install | The Chrome or Chromium executable.                                                                                                       |
| `--out`          | `docs/images`                         | The folder the shots go to.                                                                                                              |
| `--only`         | every shot                            | The shots to retake, by name, separated by commas, such as `--only user-page,iptv-source`. The others stay as they are.                  |
| `--keep`         | off                                   | Keeps the demo running after the shots, until Ctrl-C, to look at it in a browser. It prints the address, the user name and the password. |

## The demo

- **Server:** Polyfin listens on free ports of `127.0.0.1`. Its data folder is temporary, and its secret key is new at each run.
- **Accounts:** Morgan is the administrator. Sam, Robin and Alex are members.
- **Addon:** `demo.mjs` serves "Demo Catalog", a Stremio addon of the website's fictional titles, with the posters of `docs/public/posters/`. The browser draws Wild Atlas's poster, which the website lacks.
- **IPTV:** "Demo TV" is an M3U list of five channels with a programme guide: Summit, Deep Blue, Arena, Cinema One and Harbor.
- **Home rows:** four rows show in **Settings › Web player › Home rows**.
- **Playbacks:** Sam and Robin each play a movie in a Jellyfin app, which the tool imitates. The apps read their streams, so **Home** shows real bandwidth.
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

Each shot is a `shot('<name>', …)` step of `take.mjs`. It opens a page, brings it to the state wanted, and returns the area to keep. Use `clipAround()` around the elements of the subject, with a margin, and `addMarkers()` only where numbers help the text. The admin app lights the control under the mouse or the focus, so end a step with `restMouse()` or `blur()` unless the light is part of the subject. To add data, extend `seed()` or `demo.mjs`.
