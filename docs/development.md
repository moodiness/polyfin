# Development

This page explains how to run Polyfin from source, run its tests, refresh the Jellyfin fixtures, and work on the website.

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

- Database tests run when `POLYFIN_TEST_DATABASE_URL` points to a disposable PostgreSQL database. Each database test works in its own schema.
- Remux tests run when `POLYFIN_TEST_FFMPEG` names an `ffmpeg` executable with `ffprobe` beside it.
- CI always provides both.

## Jellyfin fixtures

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.2 server, recorded in `internal/jellyfin/testdata/`.

`scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container. It requires Docker, curl and jq.

## Website

The [website](https://moodiness.github.io/polyfin/) is built with [VitePress](https://vitepress.dev/) from `docs/`:

- Each page of this documentation is one of its pages, under `/docs/`, with the sidebar following the sections of the [documentation's index](README.md). Links that leave `docs/` open the file on GitHub.
- The landing page is in `docs/.vitepress/theme/`, and its videos in `docs/public/videos/`.
- VitePress 1 asks for Vite 5, whose development server has security advisories fixed only in Vite 6.4.3. `docs/package.json` overrides it with Vite 6; the override can go once VitePress asks for a fixed Vite.

```sh
npm --prefix docs ci
npm --prefix docs run dev       # the website on http://127.0.0.1:5173/polyfin/, with hot reload
npm --prefix docs run build     # fails on a link to a page that does not exist
npm --prefix docs run format    # formats the landing page's code
```

CI builds the website for each pull request that changes `docs/`. Each push of such a change to `main` publishes it on GitHub Pages (`.github/workflows/pages.yml`).
