<p align="center"><img src="assets/polyfin-social.png" alt="Polyfin"></p>

Polyfin is a self-hosted server that speaks the Jellyfin API. Its content comes from:
- **Stremio addons:** catalogs, metadata, streams and subtitles;
- **Eclipse music addons:** music, audiobooks and podcasts;
- **IPTV sources:** M3U playlists and Xtream Codes accounts, with XMLTV guides.

Any Jellyfin app connects to it like a regular Jellyfin server, with real user accounts and transcoding, and without Jellyfin installed.

> [!NOTE]
> Polyfin is in early development. Expect changes between releases, and report problems in the [issues](https://github.com/moodiness/polyfin/issues).

## Features

- **Works with Jellyfin apps:** sign in with a password or Quick Connect, then browse, search and play. Polyfin targets the Jellyfin 12.1 API.
- **Stremio addons as libraries:** catalogs become libraries and collections, streams become versions of a title, and addon subtitles become subtitle tracks.
- **Live TV and IPTV:** TV catalogs, M3U playlists and Xtream Codes accounts become Live TV, with a programme guide and recordings. IPTV movies and series become libraries.
- **Playback and transcoding:** direct play when the app supports the file. Otherwise a remux or a conversion to HLS, on an NVIDIA, AMD or Intel GPU when there is one, with HDR tone mapping.
- **Subtitles:** addon subtitles, text tracks inside files, and ASS styles with their fonts. Image subtitles are burned in when an app cannot show them.
- **Multiple users:** each user has their own watched state, resume points, favorites and Next Up, with parental control and per-user limits.
- **Skip buttons:** intros, recaps, credits and previews to skip, from three community databases.
- **Tracking:** each user can send what they watch to Trakt, Simkl, MDBList and PublicMetaDB, and import what they watched there.
- **Built in:** Jellyfin's own web client at `/web/`, and an admin app at `/admin/`.

## How it works

```text
Jellyfin client ──Jellyfin API──> Polyfin ──Stremio protocol──> AIOMetadata / AIOStreams / other addons
      │                              │
      │ direct play: 302 ────────────┼──────────────> debrid / provider URL
      │                              │
      └── transcoding: HLS <── FFmpeg (reads the stream through Polyfin's cache, serves the segments)
```

Polyfin always handles authentication, accounts, browsing, metadata, source selection and playback state.

It reads the remote stream itself only in these cases:
- when it transcodes;
- to relay a source the app could not reach (one that needs request headers, or is on a local network address);
- to relay a redirect the app could not follow.

So whether a title plays directly or is transcoded decides how much bandwidth the server uses. Apps never see the addons' stream URLs: they receive Polyfin's own, signed for the user.

## Compatible clients

Polyfin targets the apps that connect to a Jellyfin server, including Infuse, Swiftfin, Findroid, Streamyfin, Nuvio, Strand, Odin, the official Jellyfin apps and Kodi. See [Jellyfin compatibility](docs/jellyfin-compatibility.md).

## Quick start (Docker)

Requirements: Docker with Compose v2. The image, `ghcr.io/moodiness/polyfin`, is published for linux/amd64 and linux/arm64.

```sh
git clone https://github.com/moodiness/polyfin.git
cd polyfin
cp .env.example .env    # then set POSTGRES_PASSWORD, e.g. openssl rand -hex 24
docker compose up -d
```

1. Open `http://<server>:8096/admin/`.
2. Enter the one-time setup code from the log (`docker compose logs polyfin`) to create the administrator.
3. Continue with [Getting started](docs/getting-started.md).

[Installation](docs/installation.md) covers:
- pinning a version;
- building the image from source;
- the Unraid template;
- giving the container a GPU.

## Documentation

The full documentation is in [`docs/`](docs/README.md):

- **Set up:** [Installation](docs/installation.md), [Getting started](docs/getting-started.md), [Configuration](docs/configuration.md)
- **Content:** [Addons and libraries](docs/addons-and-libraries.md), [Live TV](docs/live-tv.md), [IPTV](docs/iptv.md)
- **Watching:** [Playback](docs/playback.md), [Transcoding](docs/transcoding.md), [Subtitles](docs/subtitles.md), [Skip segments](docs/skip-segments.md), [Tracking](docs/tracking.md)
- **Users and administration:** [Users](docs/users.md), [Administration](docs/administration.md), [Web client](docs/web-client.md), [Jellyfin compatibility](docs/jellyfin-compatibility.md)
- **Contributing:** [Development](docs/development.md)

## Configuration

Polyfin reads a few environment variables; with the Compose file, the only one you must set is `POSTGRES_PASSWORD` in `.env`. Everything else has a default, and most options live in the admin app under **Settings**. See [Configuration](docs/configuration.md).

## Legal disclaimer

Polyfin does not host, store, or distribute any content. It only relays what the addons and services configured by its operator provide. You are solely responsible for the addons and services you configure and for complying with the laws that apply to you.

Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.

## License

Polyfin is released under the [MIT License](LICENSE). To report a vulnerability, see the [security policy](SECURITY.md).
