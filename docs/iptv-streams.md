# IPTV stream headers and encryption

This page is for administrators whose IPTV lists need request headers, or carry DASH and ClearKey channels. It covers how Polyfin requests a source's streams, and how it plays and decrypts DASH channels. To add the source itself, see [IPTV sources](iptv.md).

## Headers and user agent

Polyfin sends a source's headers on every request for its channels, replays, movies and episodes. That covers:

- the check of a channel's first bytes, and its analysis;
- FFmpeg;
- the playlists, keys and segments of the HLS streams Polyfin relays;
- the streams Polyfin relays to apps.

A playlist gives a channel's headers in four ways:

1. `http-user-agent`, `http-referrer` and `http-origin` attributes on its `#EXTINF` line, such as `#EXTINF:-1 http-user-agent="…" http-referrer="…",Name`.
2. `#EXTVLCOPT:http-user-agent=`, `#EXTVLCOPT:http-referrer=` and `#EXTVLCOPT:http-origin=` lines.
3. An `#EXTHTTP:` line holding a JSON object of headers, such as `#EXTHTTP:{"User-Agent":"…","Referer":"…"}`.
4. `Name=value` pairs after a `|` at the end of its address, joined by `&` and percent-encoded or not, such as `https://…/1.m3u8|User-Agent=…&Referer=…`. The pairs are not part of the address.

- `#EXTVLCOPT` and `#EXTHTTP` lines replace a header of the same name from the `#EXTINF` attributes.
- Among those lines, a later line replaces an earlier one.
- The `|` pairs replace a header from any of them.
- Headers that shape the request itself are skipped: `Host`, `Connection`, `Content-Length`, `Transfer-Encoding`, `Range` and `If-Range`.

Set **User agent (optional)** when you add the source, or later with **Edit the account**.

| The source has | Streams are requested with | The list is downloaded with |
|---|---|---|
| A user agent in the list | The list's user agent | The source's **User agent (optional)**, else `Polyfin/<version>` |
| A **User agent (optional)** only | The source's user agent | The source's user agent |
| Neither | A web browser's user agent, as some providers serve only players and browsers | `Polyfin/<version>` |

- Changing **User agent (optional)** downloads the list again with it.
- The change is refused when the list cannot be read with the new user agent.
- A header the list gives, or the source's user agent, is one the stream needs. That stream is always relayed to apps.
- The browser's user agent is Polyfin's own choice. An app that can reach the source itself is still sent there, with its own user agent (see [How a channel plays](live-tv.md#how-a-channel-plays)).

**Compared with Jellyfin**

- Both send a browser user agent (Chrome 122) with the streams when no user agent is set. Jellyfin's M3U tuners send their **User agent** setting otherwise ([M3UTunerHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L169-L177)), and download the list with it when set ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L57-L61)).
- Polyfin sends the headers a playlist gives, as other players do. Jellyfin reads none of a playlist's header lines, nor the header attributes of `#EXTINF`: it reads only the `tvg-`, `logo`, `group-title` and `channel-id` attributes, and skips every line but `#EXTINF` and the addresses ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L72-L114)).

## DASH channels and encryption

A channel whose address is a DASH manifest (MPD), live or static, plays as other channels do. FFmpeg's DASH demuxer reads the manifest and its segments through Polyfin, with the channel's headers and within its source's rules. FFmpeg repackages the stream into HLS for apps, or converts what they cannot take (see [How a channel plays](live-tv.md#how-a-channel-plays)). Apps never get the manifest itself.

- Every address the manifest names, its `BaseURL`s, segment templates and lists, is rewritten to go through Polyfin.
- A manifest that names an address that is not HTTP is refused.
- A live manifest of several periods plays its last one, the one airing. FFmpeg plays one period of a manifest.
- Segment templates, timelines and lists play as FFmpeg reads them.
- The analysis, the dead-stream rules, the [stream health](iptv.md#stream-health) and the log lines are those of HLS channels.

FFmpeg has two limits with live manifests:

- Pace: FFmpeg reads a DASH stream at its own pace after its first 6 seconds, live or static. It would reach a segment the source does not have yet within a second. So it starts a live manifest up to a minute behind its end. When the source answers 404, the stream ends, and starts again a minute behind.
- Short segments: FFmpeg reads a live manifest again before each segment, and finds its place to the whole second. A live stream whose segments start less than a second apart may read the same segment again and again, and not play. Providers' segments last 2 seconds or more.

A playlist's `#KODIPROP` lines, before or after the channel's `#EXTINF` line, are read as such lists write them:

| Property | What it gives |
|---|---|
| `inputstream.adaptive.manifest_type` | `mpd` or `hls` |
| `inputstream.adaptive.license_type` | `org.w3.clearkey` or `clearkey` for ClearKey. Any other type, such as `com.widevine.alpha` or `com.microsoft.playready`, is not supported (see below). |
| `inputstream.adaptive.license_key` | ClearKey keys as `kid:key` pairs, in hexadecimal or base64url, several separated by commas, or a JSON Web Key set. Or the address of a ClearKey license server, optionally followed by `\|` and its `Name=value&Name=value` headers. |
| `inputstream.adaptive.manifest_headers` and `inputstream.adaptive.stream_headers` | `Name=value` pairs separated by `&`. Both become the channel's headers, as Polyfin sends one set with every request of a stream. They come after its `#EXTVLCOPT` and `#EXTHTTP` lines, and before the `\|` pairs of its address. |

FFmpeg decrypts a ClearKey channel with the keys its list gives, by key ID.

- Polyfin passes them as the DASH demuxer's `cenc_decryption_keys` option, which its MP4 demuxer applies as `decryption_keys`.
- FFmpeg decrypts the `cenc`, `cens`, `cbc1` and `cbcs` schemes.
- With a license server, Polyfin posts the W3C ClearKey license request for the key IDs the manifest names (`cenc:default_KID`). It sends the channel's headers and the license's own.
- Polyfin reads the JSON Web Key set the server answers ([Encrypted Media Extensions, Clear Key](https://www.w3.org/TR/encrypted-media-2/#clear-key-request-format), [License Format](https://www.w3.org/TR/encrypted-media-2/#clear-key-license-format)). It asks at each start, and keeps the keys nowhere.

| Case | What happens |
|---|---|
| Keys given by a list | Kept with the list's other data, and nowhere else. Never logged nor shown in the admin app. |
| A license server's address | Never shown |
| Keys while the channel plays | FFmpeg takes them on its command line, the only way it reads them. The server's other local users can see them. |
| Keys that do not decrypt the stream | The stream fails as **The list's keys do not decrypt it** |
| Widevine, PlayReady or another license type | The stream fails as **Needs a DRM Polyfin does not support** |
| An encrypted stream whose list gives no ClearKey keys | The stream fails as **Needs a DRM Polyfin does not support** |
| An FFmpeg built without libxml2 | It has no DASH demuxer. Its DASH channels fail, saying so. |

- Polyfin checks the keys by decoding the stream's first 8 pictures. A wrong key's garbage sometimes decodes for one picture, never for several in a row.
- Widevine, PlayReady and other license types need the provider's content decryption module. It is licensed to browsers and devices, not to servers.
- A channel whose list names such a license fails before its source is asked.
- These failures are not tried again as a network failure would be (see [Stream health](iptv.md#stream-health)).
- The keys apply to DASH channels. The FFmpeg builds Polyfin ships read DASH.

Polyfin decrypts a stream only with the keys its list gives, or that its list's license server answers. You remain responsible for having the right to watch what you decrypt with them.

**Compared with Jellyfin**

- Polyfin plays DASH through FFmpeg, as Jellyfin 12.4 does, but through its relay. It tells a DASH stream by its content, whatever its address, and decrypts ClearKey. Jellyfin plays a channel whose address ends in `.mpd` only through FFmpeg, which reads the manifest and its segments itself ([M3UTunerHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L35), [lines 161-167](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L161-L167)).
- Polyfin decrypts ClearKey channels. Jellyfin reads no `#KODIPROP` line, so it decrypts nothing ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L72-L114)).

