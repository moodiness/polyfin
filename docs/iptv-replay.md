# Replay

This page is for administrators and users of IPTV sources whose provider keeps past programmes, which Polyfin plays from a Replay view. To add the source, see [IPTV sources](iptv.md).

Many providers keep their channels' past programmes for a few days. This archive is also called catch-up or timeshift. Polyfin lists these programmes in apps in a Replay view, apart from Live TV and from recordings, and plays them from the provider's archive.

## The Replay view

Apps show a Replay view after Live TV, to the users who reach at least one channel with an archive.

- The view's name follows the server language.
- It holds one folder per channel with an archive, in channel order, named and pictured as the channel.
- A folder holds the guide's programmes that have ended and started within the channel's archive, the latest first.
- A programme in progress or to come is not listed.
- A programme is a video named after its guide title. It has its episode title, description, image, air time and length.
- Without a guide image, the programme shows the channel's logo.
- A programme plays, resumes from Continue Watching, and is marked played as a movie is.
- A programme no longer in the archive is no longer listed nor played. A resume point on it goes unused.
- Replay follows the **Live TV** permission and the user's sources, as channels do.
- Parental control and blocked genres judge its programmes as guide programmes, which have no rating.

## What lists tell

Archives are read with the channels, on every download of the list.

| List | What it gives |
|---|---|
| Xtream Codes | A live stream's `tv_archive` (1 for an archive) and `tv_archive_duration` (its days). The login's `server_info.timezone` gives the server's time zone. |
| M3U | An entry's `catchup` (or `catchup-type`), `catchup-days` (or `tvg-rec`) and `catchup-source` attributes. On the `#EXTM3U` line, they apply to every channel. A channel's own attributes take over from them. |

- An archive whose days are not given reaches a day back.
- Lists may give up to 365 days. Replay lists at most the last 30.
- A merged channel's archive is that of its first turned-on stream that has one, in the channel's stream order.
- A kind of archive Polyfin does not know is no archive.

| `catchup` | Address of a programme |
|---|---|
| `default` | `catchup-source`, its placeholders filled in. Without a `catchup-source`, no archive. |
| `append` | The stream's address followed by `catchup-source`, filled in. A source starting with `?` starts with `&` after an address that already has a query. Without a `catchup-source`, no archive. |
| `shift` | The stream's address with `utc` and `lutc` added: `?utc={start}&lutc={now}`, or `&` after an address that already has a query. |
| `flussonic` (also `flussonic-hls`, `flussonic-ts`, `fs`) | `archive-{start}-{duration}.ts` beside the stream's playlist or `mpegts` address, or under its address: one MPEG-TS file. |
| `xc` | The Xtream Codes timeshift: `{server}/timeshift/{username}/{password}/{minutes}/{YYYY-MM-DD:HH-MM}/{stream}.ts`, read from the stream's address, its length in minutes rounded up. An Xtream Codes account's channels use it. |

Placeholders of `catchup-source` are written `{name}` or `${name}`:

| Placeholder | Value |
|---|---|
| `utc`, `start` | The programme's start, in seconds since 1970 |
| `utcend`, `end` | Its end |
| `lutc`, `now`, `timestamp` | The time of the request |
| `duration` | Its length in seconds. `{duration:60}` gives minutes, and any unit of seconds may follow the colon, rounded up. |
| `offset` | How long ago it started, in seconds. `{offset:60}` gives minutes, and so on. |
| `Y`, `m`, `d`, `H`, `M`, `S` | The start's year, month, day, hour, minute and second |
| `{utc:Y-m-d H:M:S}` and the like | One of the times above, written with `Y`, `m`, `d`, `H`, `M` and `S`, as `{start:YmdHMS}` or `${end:H-M}` |

Other text, and placeholders of other names, stay as they are.

## Time zones

Seconds since 1970 have no time zone. Dates are written in the provider's time zone: the `xc` start, and the `Y`, `m`, `d`, `H`, `M` and `S` fields.

| Source | Time zone of its dates |
|---|---|
| Xtream Codes account | The one its login names, daylight saving time included. UTC when it names none. |
| M3U playlist | UTC, as a playlist names none. This includes its `xc` entries. |

## Guides and the archive window

A guide download keeps a day of past programmes (see [What a download keeps](live-tv.md#what-a-download-keeps)). Guide channels that channels with an archive take keep more:

- They keep past programmes as far back as the longest of those archives, at most 30 days.
- A new download carries over the past programmes it no longer gives, while that archive still holds them.
- On a guide's first download, before its channels are mapped, a channel's own guide identifier (`tvg-id` or `epg_channel_id`) stands for its mapping.

Each folder then lists its channel's programmes within that channel's archive, however far the guide reaches.

## Playing a programme

A programme plays as a movie's file does: analyzed, then played as it is, repackaged or converted (see [Playback](playback.md) and [Transcoding](transcoding.md)).

- The archive answers an MPEG-TS stream of unknown length, so the programme's length stands for it.
- A file under a tenth of the programme's length is refused, as a short clip standing in for a title is.
- Its times count from its start, though its packets give the time of the broadcast. A seek lands where asked (see [HLS segments](transcoding.md#hls-segments)).
- A converted programme's keyframes may be far apart, so a seek can make FFmpeg start again further back. Polyfin keeps that distance for the programme's channel.
- Seeks in that channel's other programmes then start as far back, by any user and after a restart too. Each seek opens the archive once rather than two or three times. A channel's programmes usually come from the same encoder.
- Polyfin always relays a programme. Its address holds the account's credentials, and is never shown nor logged.
- An archive that answers an HLS playlist is refused, and the app is told no stream suits. A `default` or `append` template ending in `.m3u8` may do this.
- Programmes play as files, and Polyfin's HLS path for sources is the live one. It keeps a sliding window of an endless stream, and cannot serve a finite playlist to seek in. `xc` and `flussonic` archives are MPEG-TS files.
- Programmes get no scrubbing thumbnails nor chapter images, which would read the whole file through a provider connection.

## Connection limits

A programme takes one of the account's connections while it plays.

- It holds it from `PlaybackInfo`, through the analysis and the stream or FFmpeg's reads, until 20 seconds after its last request.
- The requests of the same play share it.
- It counts with the channels played, under the account's [connection limit](live-tv.md#connection-limits).
- A programme playing is never closed for another stream.
- When the limit refuses it, the programme does not start. `PlaybackInfo` says no stream suits, and a request for its file is refused (503).

**Compared with Jellyfin**

- Polyfin plays the provider's catch-up archive. Jellyfin plays none: its past programmes are only those it recorded.
- Replay is apart from Polyfin's recordings, which stay under Live TV.

API details: [Replay view for apps](jellyfin-api.md#replay-view-for-apps).

