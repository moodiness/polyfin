# IPTV sources

This page explains how to import an M3U playlist or an Xtream Codes account directly, without a Stremio addon, and how to shape its channels, movies and series. For the programme guide, recordings and how channels play, see [Live TV](live-tv.md).

## Adding a source

Use **Add a source** and its **IPTV source** tab, under **Content › Sources** for the server or **My sources** for a user's own. Sources follow the same rules as addons (see [Addons and libraries](addons-and-libraries.md)):

- A user's own sources follow **Allow users' own addons** and the user's **Can add their own addons** (see [Users](users.md)).
- Only an administrator's source may reach a local network address.

Adding a source takes two steps in the admin app: the **Account**, then **What to import** and its categories.

### M3U playlists

An **M3U playlist** source takes a **Name**, the **Playlist address** and, optionally, a **Programme guide address (XMLTV, optional)**.

- A playlist's `#EXTINF` lines give each channel its `tvg-id`, `tvg-name`, `tvg-logo`, `tvg-chno` and `group-title` (quoted with double or single quotes, or not), and its name after the comma.
- Its `#EXTVLCOPT` `http-user-agent` and `http-referrer` lines give the headers its stream is requested with.
- A byte order mark, CRLF line ends and `#EXTGRP` lines are accepted.

### Xtream Codes accounts

An **Xtream Codes account** source takes a **Name**, the **Server address**, a **Username** and a **Password** and, optionally, the address of an XMLTV guide. By default the guide is the one the server publishes for the account (**Use the provider's programme guide**).

- The account is read through its player API (its live categories and streams).
- Its channels stream in MPEG-TS, or HLS when the account only allows HLS.

### What is read

- Lists are read as they arrive, up to 100 MB and 100,000 channels.
- A list that cannot be read is not added.
- Headings that lists put between channels are skipped: names without a letter or digit, or drawn with a run of three or more decoration characters, such as `##### NAME #####` or `=== NAME ===`.
- Addresses and logins are stored as manifest addresses are: never shown in full (only the server's scheme and host) and never logged.
- Catch-up and connection limits are not handled.

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

Counts (including movies and series), and the last and next download.

### Import options

The same choices as when adding the source, including **What to import**, the libraries choice and enrichment, applied to the list already downloaded.

### Categories

Add, rename, reorder (drag and drop or arrows), turn on or off, and delete your own added categories. Deleting an added category sends its channels back to their own.

### Channels

A list read a page of 100 at a time from the server, filtered by category, state, and visibility in apps and guide.

- On/off switches, and a selection to turn on or off.
- **Change by keyword**, after a count of what would change.
- Moves within a category, or to another category or place.
- An editor for the name, logo, description, category, fixed number, streams (reorder, turn off or add) and guide.

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
- **Edit the account** renames it and changes its address or login. The new address is fetched first; a password left empty keeps the current one.

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

An MPEG-TS file has no index to cut it by its keyframes. It plays as it is on apps that take MPEG-TS, and otherwise only converted: cut every six seconds over its duration, FFmpeg reading it from the time an app seeks to, the encoder placing a keyframe at each segment's start. A user who may not have video converted is told no stream suits instead. This holds for addons' MPEG-TS files too.

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
- when you press **Refresh** on the source's row, or **Download again** in its page's header.

A download that fails keeps the previous channels and tells why. After every list refresh and change of options, channels are mapped to the guides again.
