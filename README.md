<p align="center"><img src="assets/polyfin-social.png" alt="Polyfin"></p>

Polyfin is a self-hosted, Jellyfin-compatible server that sources its content from Stremio addons: catalogs, metadata, streams, and subtitles. It provides real user accounts and transcoding, so any Jellyfin client can connect to it like a regular Jellyfin server, without Jellyfin installed.

> [!NOTE]
> Polyfin is in early development. There is no release to install yet.

## Features

- **Jellyfin-compatible API**: standard Jellyfin clients sign in, browse, search, and play. Polyfin targets the Jellyfin 12.1 API.
- **Stremio addons as the content source**: AIOMetadata for catalogs and metadata, AIOStreams for streams and subtitles, and any other addon that speaks the standard Stremio protocol.
  - Stremio catalogs become Jellyfin libraries.
  - Stremio streams become versions (media sources) of the same item.
- **Multiple users**: separate accounts with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points, and Next Up are tracked per user.
- **Transcoding**: direct play when the client supports the file, with Polyfin redirecting the client to the stream and staying out of the video path; otherwise on-the-fly ffmpeg transcoding to HLS.

## How it works

```text
Jellyfin client ──Jellyfin API──> Polyfin ──Stremio protocol──> AIOMetadata / AIOStreams / other addons
      │                              │
      │ direct play: 302 ────────────┼──────────────> debrid / provider URL
      │                              │
      └── transcoding: HLS <── ffmpeg (reads the debrid URL, serves the segments)
```

Polyfin always handles authentication, accounts, browsing, metadata, source selection, and playback state. It only reads the remote stream itself when transcoding, so the direct play or transcoding decision determines how much bandwidth the server uses.

## Compatible clients

Polyfin targets the clients that connect to a Jellyfin server, including:

- Infuse
- Swiftfin
- Findroid
- Streamyfin
- Official Jellyfin apps
- Kodi

## Legal disclaimer

Polyfin does not host, store, or distribute any content. It only relays what the addons and services configured by its operator provide. You are solely responsible for the addons and services you configure and for complying with the laws that apply to you.

Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.

## License

Polyfin is released under the [MIT License](LICENSE). To report a vulnerability, see the [security policy](SECURITY.md).
