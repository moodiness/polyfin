# IPTV sources

This page explains how to import an M3U playlist or an Xtream Codes account directly, without a Stremio addon, how to watch and record the channels of an HDHomeRun tuner on the local network, and how to shape their channels, movies and series. For the programme guide, recordings and how channels play, see [Live TV](live-tv.md).

## Adding a source

Use **Add a source** and its **IPTV source** tab, under **Content › Sources** for the server or **My sources** for a user's own. Sources follow the same rules as addons (see [Addons and libraries](addons-and-libraries.md)):

- A user's own sources follow **Allow users' own addons** and the user's **Can add their own addons** (see [Users](users.md)).
- Only an administrator's source may reach a local network address, or read a file of the server (see [Lists and guides that are files of the server](#lists-and-guides-that-are-files-of-the-server)).

Adding a source takes two steps in the admin app: the **Account**, then **What to import** and its categories.

### M3U playlists

An **M3U playlist** source takes a **Name**, the **Playlist address** and, optionally, a **User agent** (see [Headers and user agent](#headers-and-user-agent)) and a **Programme guide address (XMLTV, optional)**. For an administrator, they are **Playlist address or file** and **Programme guide address or file (XMLTV, optional)**: each takes a file of the server too (see below).

- A playlist's `#EXTINF` lines give each channel its `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno` and `group-title` (quoted with double or single quotes, or not), and its name after the comma.
- The header attributes of its `#EXTINF` lines, its `#EXTVLCOPT` and `#EXTHTTP` lines, and the `|` ending an address, give the headers its stream is requested with (see [Headers and user agent](#headers-and-user-agent)).
- A byte order mark, CRLF line ends and `#EXTGRP` lines are accepted.

### Lists and guides that are files of the server

An administrator may give an M3U list, and an XMLTV guide, as a file of the server instead of an address: by its absolute path, such as `/iptv/custom.m3u`, or `C:\IPTV\custom.m3u` with Polyfin Desktop on Windows, or by a `file://` address, such as `file:///iptv/custom.m3u`.

- The file is read as a list or a guide from an address is: the same parsing, categories, headers and guide matching.
- A path where there is no file, a folder, or a file Polyfin may not read, is refused when the source or the guide is added, with why. A relative path is refused too: give the whole path.
- The file is read again at each refresh (see [Refreshing](#refreshing)), and within 5 minutes once it changed: Polyfin looks at its modification time every 5 minutes. A file modified with an older date than its last read waits for the next refresh.
- A file that is gone, or that Polyfin can no longer read, is told within 5 minutes on the source's row, its page and **System › Health**, its channels or programmes kept. It is read again after 5 minutes, then 15 minutes, then every hour, as a download that failed.
- Its path is shown whole: a path holds no credential.
- Only administrators may give one, for the server's sources or their own. A member's source, and a member's guide, are refused any file of the server, whether it exists or not, and a source whose owner is no longer an administrator stops reading its file.

With Docker, Polyfin only sees the files of the volumes mounted in its container, and the path to give is the one in the container. Mount the folder that holds the file, read-only, in the `polyfin` service of `compose.yaml`:

```yaml
services:
  polyfin:
    volumes:
      - ./iptv:/iptv:ro
```

Run `docker compose up -d` for the container to get it, then give `/iptv/custom.m3u`. User 65532 must be able to read the file.

**Compared with Jellyfin:**
- Jellyfin's M3U tuners and XMLTV guides take a file or an address too: anything that does not start with `http` is opened as a file ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L52-L55), [XmlTvListingsProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/Listings/XmlTvListingsProvider.cs#L118-L124)), and a missing file is refused when the tuner or the guide is saved ([M3UTunerHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L133-L138), [XmlTvListingsProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/Listings/XmlTvListingsProvider.cs#L342-L346)).
- Jellyfin also takes a path relative to its working folder; Polyfin takes absolute paths only, whose meaning does not depend on how it was started.
- Jellyfin reads a guide file again once its copy is an hour old ([XmlTvListingsProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/Listings/XmlTvListingsProvider.cs#L30)); Polyfin reads a list or a guide file again once it changed, and at each refresh.

### Xtream Codes accounts

An **Xtream Codes account** source takes a **Name**, the **Server address**, a **Username** and a **Password** and, optionally, a **User agent** (see [Headers and user agent](#headers-and-user-agent)) and the address of an XMLTV guide. By default the guide is the one the server publishes for the account (**Use the provider's programme guide**).

- The account is read through its player API (its live categories and streams).
- Its channels stream in MPEG-TS, or HLS when the account only allows HLS.

### Headers and user agent

The channels, replays, movies and episodes of an M3U playlist or an Xtream Codes account are requested with the headers below on every request Polyfin makes for them: the check of a channel's first bytes, its analysis, FFmpeg, the playlists, keys and segments of the HLS streams it relays, and the streams it relays to apps.

- A playlist gives a channel's headers with:
  - `http-user-agent`, `http-referrer` and `http-origin` attributes on its `#EXTINF` line, such as `#EXTINF:-1 http-user-agent="…" http-referrer="…",Name`;
  - `#EXTVLCOPT:http-user-agent=`, `#EXTVLCOPT:http-referrer=` and `#EXTVLCOPT:http-origin=` lines;
  - an `#EXTHTTP:` line holding a JSON object of headers, such as `#EXTHTTP:{"User-Agent":"…","Referer":"…"}`;
  - a `|` after its address, then `Name=value` pairs separated by `&`, percent-encoded or not, such as `https://…/1.m3u8|User-Agent=…&Referer=…`. The pairs are not part of the address.
- The more explicit replace the attributes: `#EXTVLCOPT` and `#EXTHTTP` lines replace a header of the same name the `#EXTINF` line gave, a later line one an earlier line gave, and the `|` pairs any of them. Headers that shape the request itself (`Host`, `Connection`, `Content-Length`, `Transfer-Encoding`, `Range`, `If-Range`) are skipped.
- **User agent**, set when adding the source or with **Edit the account**, is the user agent of the streams whose list names none, and the one the list is downloaded with. Changing it downloads the list again with it, and is refused when the list cannot be read so.
- Without either, the streams are requested with a web browser's user agent, as some providers serve only players and browsers. The list is downloaded with Polyfin's own, `Polyfin/<version>`.
- A header that the list gives, or the user agent set on the source, is one the stream needs: the stream is then always relayed to apps. The browser's user agent is Polyfin's own: an app that can reach the source itself is still sent there, with its own (see [How a channel plays](live-tv.md#how-a-channel-plays)).
- An HDHomeRun tuner's channels are requested with Polyfin's own user agent.

**Compared with Jellyfin:**
- Jellyfin's M3U tuners send their **User agent** setting, else the same browser user agent (Chrome 122), with their channels' streams ([M3UTunerHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L169-L177)), and download the list with that setting when it is set ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L57-L61)).
- Jellyfin reads none of a playlist's header lines, nor the header attributes of its `#EXTINF` lines, of which it reads the `tvg-`, `logo`, `group-title` and `channel-id` ones: it skips every line but `#EXTINF` and the addresses ([M3uParser.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs#L72-L114)). Polyfin sends the headers they give, as other players do.

### What is read

- Lists are read as they arrive, up to 100 MB and 100,000 channels.
- A list that cannot be read is not added.
- Headings that lists put between channels are skipped: names without a letter or digit, or drawn with a run of three or more decoration characters, such as `##### NAME #####` or `=== NAME ===`.
- Addresses and logins are stored as manifest addresses are: never shown in full (only the server's scheme and host) and never logged.
- Requests to a provider's server (its lists, guide, and the details of a title) go one at a time, a second apart; a server that answers "too many requests" is asked again when it says. A list or guide that still fails says the server asked Polyfin to slow down, and is tried again after a few minutes (see [Refreshing](#refreshing)).
- An Xtream account's connection limit, which its login tells, bounds the channels played at once (see [How a channel plays](live-tv.md#how-a-channel-plays)).
- The archive of past programmes a list tells for a channel (catch-up) is read on every download, and its programmes play from Replay (see [Replay](#replay)).

## HDHomeRun tuners

An HDHomeRun tuner on the local network, with an antenna or a cable, is a source too: its free channels play and record as an IPTV source's. Only administrators add tuners, to the server's sources, as only their sources may reach the local network, and its tuners are the server's to share.

**Add a source** under **Content › Sources** has an **HDHomeRun tuner** tab:

- **Find tuners** asks the tuners of the local network to answer (HDHomeRun's discovery, a UDP broadcast on port 65001) and lists those that do, with their name, model and tuner count. A tuner already added says so.
- **Add by address** takes the tuner's address, `192.168.1.20` or `http://192.168.1.20`, and reads its `discover.json`.

In Docker, the broadcast reaches the local network only with host networking (see [Installation](installation.md#find-hdhomerun-tuners)): the tab says so when no tuner answers, and an address always works. `POLYFIN_HDHOMERUN_ADDRESSES` has **Find tuners** ask given addresses instead of broadcasting (see [Configuration](configuration.md)).

A tuner is added under the name it gives itself, with its Live TV catalog only, and once: the same tuner (its device ID) cannot be added twice, as its tuners would be counted twice.

### A tuner's channels

- Channels are read from the tuner's `lineup.json`, with their number (`GuideNumber`, such as "5.1") and name. Channels the tuner marks protected (`DRM`) are left out, as it does not play them.
- Apps show each channel's number as the tuner gives it, and order them by it: 5, 5.1, 5.2, then 6. A fixed number set under **Channels** replaces it.
- **Only the tuner's favorite channels**, off by default, keeps the channels marked favorite on the tuner. It is in the **Tuner** panel of the source page's **Summary**, with the tuner's model and tuner count.
- **Scan for channels** asks the tuner to scan for channels again (on its first source, such as its antenna), shows its progress, then reads its line-up.
- The line-up is read again with **Refresh Live TV lists and guides every**, and kept as an IPTV line-up is, edits included (see [How the line-up is kept](#how-the-line-up-is-kept)).
- XMLTV guides attach to its catalog as to any catalog. A guide channel named by the channel's number takes it (see [How channels are matched to the guide](live-tv.md#how-channels-are-matched-to-the-guide)).

### Tuners in use

Each channel watched or recorded takes one of the tuner's tuners, whoever reads it, as a channel's stream is read once for every viewer. With every tuner taken, opening a channel first frees one no one watches any more, then the user's own oldest, as for an account's connection limit; when other users' channels take them all, it is refused at once. A recording takes the tuner of a channel only watched, never another recording's (see [Recordings and tuners](live-tv.md#recordings-and-tuners)).

Streams play as IPTV channels do: as they come when the app takes them, else converted. Terrestrial video is often MPEG-2 and interlaced: a conversion deinterlaces it with **Deinterlacing method** (see [Transcoding](transcoding.md)). The user's **Live TV** permission, quality group and limits apply.

A tuner that does not answer, or refuses a stream for another reason than its tuners being in use, shows under **System › Health** until one of its streams plays. A stream refused because every tuner was in use, by Polyfin or another app, is **Refused by the provider** (see [Stream health](#stream-health)), and is no fault of the tuner's.

## What to import

The add flow's second step starts with **What to import**:

| Type | Default | What it imports |
|---|---|---|
| **Live TV channels** | on | The provider's live channels, as a Live TV catalog. |
| **Movies** | off | The provider's movies, as a library. |
| **Series** | off | The provider's series, as a library. |

With **Movies** and **Series** off, a source stays exactly as before they existed: an M3U playlist's movies and episodes then stay channels.

The step then previews the **Categories to import**, to include or leave out:

- Channel categories **By group** or **By country**, with their channel counts. The preview downloads the list once and reuses it for the next few minutes.
- With movies or series on, their categories per type, with title counts and search (**Include all**, **Exclude all**).

It also shows the [import options](#import-options), and for movies and series the libraries choice and whether titles are described by the metadata addons (see [Movies and series libraries](#movies-and-series-libraries)).

## Import options

**Import options** are chosen when the source is added and can be changed later on the source page, where they apply to the list already downloaded. They turn the list into the source's line-up. On the source page, **Save and rebuild** applies them and **Discard changes** drops them.

| Option | Default | What it does |
|---|---|---|
| **Categories** | **As the provider groups them** | Categories from the provider's groups, or **One per country**. |
| **Channels** | **One per entry** | One channel per entry, or **Merge quality variants**. |
| **Categories to import** | all included | The groups and countries left out. |
| **Turn on new channels** | on | Whether new channels found by a refresh arrive turned on or off. |
| **Numbering** | **The provider's numbers** | The list's number (`tvg-chno`, or Xtream's `num`), or **In line-up order**, apps then numbering channels by their place. |

### Categories by country

A channel's country is told by the group's name first and then the channel's: a flag, or a prefix such as `FR|`, `[FR]`, `(FR)`, `FR:` or `FR -`. `UK` counts as GB. The rest go under Other, last.

### Merged channels

When merged, the entries of a category with the same name, once country prefixes and quality tags are set aside, become one channel with a stream per entry, best quality first: 4K, UHD, FHD, HD, SD, then the others. The channel's name keeps the brackets that belong to it, such as "Zeb (Prime)".

- A merged channel's details offer one version per turned-on stream, best first.
- Playing it tries them in order, as many as **Versions tried when one does not work**, past those that fail, among those that fit the user's quality group (see [Playback](playback.md)).

**Compared with Jellyfin:**
- Jellyfin's tuner channels each have one source, so apps offer the streams as a version choice.

## The source page

Each source has its own page, titled with the source's name and its kind (**M3U playlist** or **Xtream Codes account**), opened from **Open the source** on its row under **Content › Sources** or **My sources**. Its header holds **Download again**, on every section. It has six sections: **Summary**, **Import options**, **Categories**, **Channels**, **Guides** and **Guide mapping**. While **Live TV channels** is off, the four Live TV sections (**Categories**, **Channels**, **Guides**, **Guide mapping**) are hidden.

### Summary

Counts (including movies and series), and the last and next download. When channels keep their past programmes, a notice counts those apps show, with **See them** to list them under **Channels**.

### Import options

The same choices as when adding the source, including **What to import**, the libraries choice and enrichment, applied to the list already downloaded.

### Categories

Add, rename, reorder (drag and drop or arrows), turn on or off, and delete your own added categories. Deleting an added category sends its channels back to their own.

### Channels

A list read a page of 100 at a time from the server, filtered by category, state, visibility in apps, guide and archive. A channel whose provider keeps its past programmes shows for how many days, such as **Replay, 7 days**.

- On/off switches, and a selection to turn on or off.
- **Change by keyword**, after a count of what would change.
- Moves within a category, or to another category or place.
- An editor for the name, logo, description, category, fixed number, streams (reorder, turn off or add) and guide.

#### Stream health

Each stream shows how it last answered when its channel was opened:

- **No live stream**: an error, a web page, an empty answer or bytes of no video. It is left out of its channel for an hour, then 6 hours, then a day while it keeps failing.
- **Nothing came**: left out for 10 minutes when it never played at its address, or after nothing came twice in a row. A stream that played and was silent once, as when its provider was slow, is tried again at the next start, after the channel's other streams.
- **Refused by the provider**: the provider would not serve it then (connections in use, too many requests). It is tried again at the next start.

Streams that failed come after the others in their channel. A stream that plays, or that the list gives a new address, is healthy again. **Try every stream again** forgets it all for the channel: its next start tries every stream at once, those that failed within the last 15 minutes included.

Edits work one by one or in bulk, by selection, category or keyword, with a count of what would change first. Lists are paged and searched by the server, so line-ups of tens of thousands of channels stay quick. An added stream's address follows the source's rule for local network addresses and is never shown in full.

### Guides

The catalog's XMLTV guides in order, with their status and the automatic mapping. See [XMLTV guides](live-tv.md#xmltv-guides).

### Guide mapping

The guide channel each channel takes its programmes from, set by hand from a search across the guides showing what airs now. See [Remapping and mapping by hand](live-tv.md#remapping-and-mapping-by-hand).

A Stremio addon's Live TV catalog has the same **Guides** and **Guide mapping** views, from **Guides and mapping** on its row under **Content › Live TV** (see [Live TV](live-tv.md)). The **Content › Live TV** page also lists each IPTV source's channel counts, with **Line-up and guides** to open its page.

## How the line-up is kept

Polyfin keeps the line-up and reconciles it with the list on every download and option change, by stable keys: a category by its group or country, a channel by its entry, or by its category and name when merged, and by any of its former streams when the mode changes.

- The administrator's edits stay.
- The provider's names, logos, numbers and guide identifiers update underneath.
- What the list dropped, or the options now leave out, goes away with its edits.
- A source added before line-ups keeps what it showed: the groups it did not show become turned-off categories.

### What apps show

Apps list the turned-on channels of turned-on categories that have a turned-on stream, in category then channel order. A channel's category is its genre. **Channels read per Live TV catalog** counts those only. A channel no longer in the line-up, or turned off, is no longer listed nor played.

### A source behaves as an addon

A source behaves as an installed addon with one TV catalog:

- It is listed, ordered, turned off and removed with the addons.
- Its catalog is enabled under **Content › Libraries**, where its guide panel is, whatever the number of libraries.
- Its channels play, record and follow the **Live TV** permission, quality groups, parental control and allowed hours as any channel (see [Users](users.md) and [Live TV](live-tv.md)).
- **Edit the account** renames it and changes its address, login or user agent. A new address or user agent is fetched first; a password left empty keeps the current one.

## Replay

Many providers keep their channels' past programmes for a few days, an archive also called catch-up or timeshift. Polyfin lists them in apps in a Replay view, apart from Live TV and from recordings, and plays them from the provider's archive.

### The Replay view

- Apps show a Replay view after Live TV to the users who reach at least one channel with an archive. Its name follows the server language.
- It holds one folder per channel with an archive, in channel order, named and pictured as the channel.
- A folder holds the programmes of the channel's guide that have ended and started within the channel's archive, the latest first. A programme in progress or to come is not listed.
- A programme is a video named after its guide's title, with its episode title, description and image (the channel's logo when the guide gives none), its air time and its length. It plays, resumes from Continue Watching, and is marked played as a movie is.
- A programme no longer in the archive is no longer listed nor played; a resume point on it then goes unused.
- Replay follows the **Live TV** permission and the user's sources, as channels do. Parental control and blocked genres judge its programmes as guide programmes, which have no rating.

### What lists tell

Archives are read on every download of the list, with the channels:

- **Xtream Codes:** a live stream's `tv_archive` (1 for an archive) and `tv_archive_duration` (its days). The login's `server_info.timezone` gives the server's time zone.
- **M3U:** an entry's `catchup` (or `catchup-type`), `catchup-days` (or `tvg-rec`) and `catchup-source` attributes. Given on the `#EXTM3U` line, they apply to every channel; a channel's own attributes take over from them.
- An archive whose days are not given reaches a day back. Lists may say up to 365 days; Replay lists at most the last 30.
- A merged channel's archive is that of its first turned-on stream with one, in the channel's stream order.
- A kind of archive Polyfin does not know is no archive.

| `catchup` | Address of a programme |
|---|---|
| `default` | `catchup-source`, its placeholders filled in. Without a `catchup-source`, no archive. |
| `append` | The stream's address followed by `catchup-source`, filled in. A source starting with `?` starts with `&` after an address that already has a query. Without a `catchup-source`, no archive. |
| `shift` | The stream's address with `utc` and `lutc` added: `?utc={start}&lutc={now}`, or `&` after an address that already has a query. |
| `flussonic` (also `flussonic-hls`, `flussonic-ts`, `fs`) | `archive-{start}-{duration}.ts` beside the stream's playlist or `mpegts` address, or under its address: one MPEG-TS file. |
| `xc` | The Xtream Codes timeshift: `{server}/timeshift/{username}/{password}/{minutes}/{YYYY-MM-DD:HH-MM}/{stream}.ts`, read from the stream's address, its length in minutes rounded up. An Xtream Codes account's channels use it. |

Placeholders of `catchup-source`, written `{name}` or `${name}`:

| Placeholder | Value |
|---|---|
| `utc`, `start` | The programme's start, in seconds since 1970. |
| `utcend`, `end` | Its end. |
| `lutc`, `now`, `timestamp` | The time of the request. |
| `duration` | Its length in seconds; `{duration:60}` in minutes, and any unit of seconds after the colon, rounded up. |
| `offset` | How long ago it started, in seconds; `{offset:60}` in minutes, and so on. |
| `Y`, `m`, `d`, `H`, `M`, `S` | The start's year, month, day, hour, minute and second. |
| `{utc:Y-m-d H:M:S}` and the like | One of the times above written with `Y`, `m`, `d`, `H`, `M` and `S`, as `{start:YmdHMS}` or `${end:H-M}`. |

Other text, and placeholders of other names, stay as they are.

### Time zones

Seconds since 1970 have no time zone. Dates, as the `xc` start and the `Y`, `m`, `d`, `H`, `M`, `S` fields, are written in the provider's time zone: for an Xtream Codes account, the one its login names, daylight saving time included; else UTC. An M3U playlist names none, so its dates, those of its `xc` entries included, are in UTC.

### Guides and the archive window

A guide download keeps a day of past programmes (see [What a download keeps](live-tv.md#what-a-download-keeps)). For the guide channels that channels with an archive take, it keeps them as far back as the longest of those archives, at most 30 days, and carries over from the last download the past programmes a new one no longer gives while that archive still holds them. On a guide's first download, before its channels are mapped, a channel's own guide identifier (`tvg-id` or `epg_channel_id`) stands for its mapping.

Each folder then lists its channel's programmes within that channel's archive, however far the guide reaches.

### Playing a programme

- A programme plays as a movie's file does: analyzed, played as it is, remuxed or converted (see [Playback](playback.md) and [Transcoding](transcoding.md)). As the archive answers an MPEG-TS stream of unknown length, the programme's length stands for it. A file under a tenth of the programme's length is refused, as a short clip standing in for a title is. Its times count from its start, though its packets tell the time of the broadcast, and a seek lands where asked (see [HLS segments](transcoding.md#hls-segments)).
- When a seek in a converted programme has FFmpeg start again further back, as its keyframes are far apart, Polyfin keeps that distance for the programme's channel, whose programmes usually come from the same encoder: seeks in its other programmes, by any user and after a restart too, start as far back, each opening the archive once rather than two or three times.
- Polyfin always relays it: the address holds the account's credentials, and is never shown nor logged.
- An archive that answers an HLS playlist, as a `default` or `append` template ending in `.m3u8` may, is refused: the app is told no stream suits. Programmes play as files, and Polyfin's HLS path for sources is the live one, which keeps a sliding window of an endless stream and cannot serve a finite playlist to seek in. `xc` and `flussonic` archives are MPEG-TS files.
- Programmes get no scrubbing thumbnails nor chapter images, which would read the whole file through a provider connection.

### Connection limits

A programme takes one of the account's connections while it plays, counted with the channels played (see [How a channel plays](live-tv.md#how-a-channel-plays)): from `PlaybackInfo`, through the analysis, the stream or FFmpeg's reads, until 20 seconds after its last request. The requests of the same play share it.

- When every connection is taken, a stream no one uses any more is closed first, then the user's own oldest channel. A programme playing is never closed for another stream.
- Otherwise the programme does not start: `PlaybackInfo` tells no stream suits, and a request for its file is refused (503).

**Compared with Jellyfin:**
- Jellyfin plays no catch-up archive: its past programmes are only those it recorded. Replay is apart from Polyfin's recordings, which stay under Live TV.

**For app developers:**
- The Replay view is a `CollectionFolder` of `CollectionType` `folders`, with the same identifier on every server, listed in `/UserViews` after Live TV. Apps open it as a folder list; it has no Latest row.
- Its folders are `Folder` items. Its programmes are `Video` items with `ChannelId`, `StartDate`, `EndDate`, `RunTimeTicks`, and `DateCreated` set to the air time. They play through `PlaybackInfo` and the video stream and HLS routes, as movies do.
- `/UserItems/Resume` lists them with movies and episodes, as Jellyfin lists its other videos there.

## Movies and series libraries

### Libraries

Movies and series libraries are chosen as:

- **One per type: Movies, Series** (default): a Movies and a Series library named after the source, each title's category being its genre, by which the library can be narrowed;
- **One per provider category**.

The libraries are ordinary library rows of the source under **Content › Libraries**, with a link to the source's import options (**Its import options**).

- A catalog that appears is added as an enabled library at the end of the scope's libraries.
- One that goes away loses its library.
- A library turned off under **Content › Libraries** stays off.
- A category added later by the provider is imported.
- With **Live TV channels** turned off, the source has no Live TV catalog (its guides are removed), while its line-up keeps its edits for when they come back.

### How titles are read

- **Xtream Codes:** the VOD and series lists are read through its player API (`get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`) with its live list, only for the types turned on. Turning a type on reads its list at once.
- **M3U:** entries are movies or episodes by their address (a `/movie/` or `/series/` path) or their video file's extension (`mp4`, `mkv`, `avi` and the like; `ts` and `m3u8` stay live).
- An episode's name gives its series, season and episode (`S01E02`, `S01 E02`, `1x02`, `Season 1 Episode 2`). Episodes whose names give no numbers are numbered by their place. Episodes group into series by name whatever its case.
- Titles are named without the list's dressing (a language or country prefix such as `EN -` or `|FR|`, quality tags). A year written after a name becomes the title's year.
- Titles keep the provider's identifier, so their item identifiers, and with them users' played state, resume points, favorites and next up, survive refreshes, changes of order and new credentials.

### In apps

Movies and series are ordinary titles in apps: details, search, latest, genres, continue watching, next up, collections. They are newest first in their libraries, which are paged and counted whole. **Titles read per movie and series catalog** does not bound them: they are stored, not asked of an addon page by page.

- Listings use only the lists' data.
- An Xtream title's details are asked only when an app opens the title or plays it: `get_vod_info` (overview, cast, director, genres, release date, duration, backdrop, trailer, TMDB id) and `get_series_info` (seasons and episodes). They are kept until the series changes or for seven days.
- Details are asked of a provider host at most once a second, as providers ban accounts that send bursts.

### Playback

Each movie or episode has one version, the provider's file (`/movie/…` or `/series/…` with its container for Xtream). It is labelled with the quality its name gives (4K, FHD, HD, SD) for quality groups. It plays, converts, downloads and takes thumbnails and subtitles as any title's (see [Playback](playback.md) and [Transcoding](transcoding.md)).

An MPEG-TS file has no index to cut it by its keyframes. It plays as it is on apps that take MPEG-TS, and otherwise only converted: cut into a 2-second segment, then one every four seconds over its duration, the encoder placing a keyframe at each segment's start. After a seek, FFmpeg reads it from before the time asked, as it lands on the first keyframe after the time it is given, and what comes before the segment is left out; how far before is kept with the file, for its next playbacks (see [HLS segments](transcoding.md#hls-segments)). A user who may not have video converted is told no stream suits instead. This holds for addons' MPEG-TS files too.

**Compared with Jellyfin:**
- MPEG-TS files play as Jellyfin plays such files.

### Ratings and enrichment

Providers give no content rating: titles are unrated unless enrichment brings one. A user blocking unrated movies or series does not see them.

Enrichment is an import option, on by default. It only asks the administrator's own addons, once per title opened, and brings the posters, cast and age ratings the providers lack.

- A title the provider gives a TMDB or IMDb identifier is described by the first of the user's addons that describes that identifier.
- The provider's description fills what the addon lacks.
- The title's name, category and episodes stay the provider's.

## Refreshing

The list is downloaded:

- when the source is added;
- again, with the guides, once **Refresh Live TV lists and guides every** hours have passed (under **Settings › Live TV**, 12 by default, 1 to 168; see [When guides are fetched](live-tv.md#when-guides-are-fetched));
- when you press **Refresh** on the source's row, or **Download again** in its page's header;
- for a list that is a file of the server, within 5 minutes once the file changed, or became unreadable (see [Lists and guides that are files of the server](#lists-and-guides-that-are-files-of-the-server)).

A download that fails keeps the previous channels and tells why, and is tried again after 5 minutes, then 15 minutes, then every hour (never later than the setting). After every list refresh and change of options, channels are mapped to the guides again.
