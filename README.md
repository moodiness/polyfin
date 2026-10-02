<p align="center"><img src="assets/polyfin-social.png" alt="Polyfin"></p>

Polyfin is a self-hosted, Jellyfin-compatible server that sources its content from Stremio addons: catalogs, metadata, streams, and subtitles. It provides real user accounts and transcoding, so any Jellyfin client can connect to it like a regular Jellyfin server, without Jellyfin installed.

> [!NOTE]
> Polyfin is in early development: Jellyfin apps can sign in with a password or Quick Connect, and addons and libraries can be configured, but there is nothing to browse or play in the apps yet. No image is published before the first release.

## Features

- **Jellyfin-compatible API**: standard Jellyfin clients sign in, browse, search, and play. Polyfin targets the Jellyfin 12.1 API.
- **Stremio addons as the content source**: AIOMetadata for catalogs and metadata, AIOStreams for streams and subtitles, and any other addon that speaks the standard Stremio protocol.
  - Stremio catalogs become Jellyfin libraries.
  - Stremio streams become versions (media sources) of the same item.
- **Multiple users**: separate accounts with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points, and Next Up are tracked per user.
- **Transcoding**: direct play when the client supports the file, with Polyfin redirecting the client to the stream and staying out of the video path; otherwise Polyfin's own on-demand HLS transcoder, built on FFmpeg for remote sources. See the [transcoding design](docs/transcoding.md).

## How it works

```text
Jellyfin client ──Jellyfin API──> Polyfin ──Stremio protocol──> AIOMetadata / AIOStreams / other addons
      │                              │
      │ direct play: 302 ────────────┼──────────────> debrid / provider URL
      │                              │
      └── transcoding: HLS <── FFmpeg (reads the stream through Polyfin's cache, serves the segments)
```

Polyfin always handles authentication, accounts, browsing, metadata, source selection, and playback state. It only reads the remote stream itself when transcoding, so the direct play or transcoding decision determines how much bandwidth the server uses.

## Compatible clients

Polyfin targets the clients that connect to a Jellyfin server, including:

- Infuse
- Swiftfin
- Findroid
- Streamyfin
- Nuvio
- Strand
- Official Jellyfin apps
- Kodi

## Quick start (Docker)

Until the first release, build the image from source. Requirements: Docker with Compose v2.

```sh
git clone https://github.com/moodiness/polyfin.git
cd polyfin
cp .env.example .env    # then set POSTGRES_PASSWORD, e.g. openssl rand -hex 24
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

Open `http://<server>:8096/admin/`. Polyfin waits for PostgreSQL and creates its tables on startup.

**First run:** until an administrator exists, Polyfin prints a one-time setup code in its log (`docker compose logs polyfin`). Enter it on the setup page to create the administrator, then create the other accounts under **Users**. Jellyfin apps sign in with these accounts, by password or with **Quick Connect**: the app shows a 6-digit code that a signed-in user approves on the Quick Connect page.

**Addons:** paste an addon's manifest URL (from its configure page) under **Addons** to share it with every user, then pick under **Libraries** which of its catalogs become libraries in Jellyfin apps. Each user can also add their own addons and libraries under **My addons**, and turn the server's addons off for themselves. Manifest URLs usually contain your addon settings or keys: Polyfin never shows them in full. Only administrators can install addons hosted on a local network address.

**Unraid:** the template lives in [`templates/unraid/polyfin.xml`](templates/unraid/polyfin.xml). It needs a PostgreSQL 18 container and becomes usable once an image is published.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, e.g. `postgresql://polyfin:password@postgres:5432/polyfin` |
| `POLYFIN_LISTEN` | `:8096` | HTTP address. 8096 is the port Jellyfin clients try by default. |
| `POLYFIN_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

The Compose files read their own settings (passwords, ports, image version) from `.env`; see [`.env.example`](.env.example).

## Development

Requirements: Go 1.27, Node.js 24, and a PostgreSQL 18 server. With `POSTGRES_PASSWORD` set in `.env`, `docker compose up -d postgres` starts one on `127.0.0.1:5432`.

```sh
export POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin
make dev                      # builds the admin app, then runs the server on :8096
npm --prefix web run dev      # optional: admin app with hot reload, proxied to :8096
make check                    # formatting, vet and tests
```

Database tests run when `POLYFIN_TEST_DATABASE_URL` points to a disposable PostgreSQL database; CI always provides one. Each test works in its own schema.

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.1 server, recorded in `internal/jellyfin/testdata/`. `scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container (requires Docker, curl and jq).

## Legal disclaimer

Polyfin does not host, store, or distribute any content. It only relays what the addons and services configured by its operator provide. You are solely responsible for the addons and services you configure and for complying with the laws that apply to you.

Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.

## License

Polyfin is released under the [MIT License](LICENSE). To report a vulnerability, see the [security policy](SECURITY.md).
