# Development

This page explains how to run Polyfin from source, run its tests, and refresh the Jellyfin fixtures.

## Requirements

- Go 1.27.
- Node.js 24.
- A PostgreSQL 18 server.
- FFmpeg 9.0 or later (`ffmpeg` and `ffprobe`), to play titles.

With `POSTGRES_PASSWORD` set in `.env`, `docker compose up -d postgres` starts a PostgreSQL server on `127.0.0.1:5432`.

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

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.1 server, recorded in `internal/jellyfin/testdata/`.

`scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container. It requires Docker, curl and jq.
