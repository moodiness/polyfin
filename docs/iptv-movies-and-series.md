# IPTV movies and series

This page is for administrators who import an IPTV source's movies and series as libraries. To add the source and choose **What to import**, see [IPTV sources](iptv.md).

## Movies and series libraries

### Libraries

Choose how movies and series become libraries:

| Choice | Libraries |
|---|---|
| **One per type: Movies, Series** (default) | A Movies and a Series library named after the source. Each title's category is its genre, by which the library can be narrowed. |
| **One per provider category** | One library per category |

The libraries are ordinary library rows of the source in **Content › Libraries**. Each links to the source's import options: **Its import options**.

- A catalog that appears is added as an enabled library, at the end of the scope's libraries.
- A catalog that goes away loses its library.
- A library turned off in **Content › Libraries** stays off.
- A category the provider adds later is imported.
- With **Live TV channels** off, the source has no Live TV catalog, and its guides are removed. Its line-up keeps its edits for when the channels come back.

### How titles are read

| Source | How titles are found |
|---|---|
| Xtream Codes | The VOD and series lists, through its player API (`get_vod_categories`, `get_vod_streams`, `get_series_categories`, `get_series`), with its live list. Only the types turned on are read. Turning a type on reads its list at once. |
| M3U | Entries are movies or episodes by their address (a `/movie/` or `/series/` path), or by their video file's extension (`mp4`, `mkv`, `avi` and the like). `ts` and `m3u8` stay live. |

- An episode's name gives its series, season and episode: `S01E02`, `S01 E02`, `1x02` or `Season 1 Episode 2`.
- Episodes whose names give no numbers are numbered by their place.
- Episodes group into series by name, whatever its case.
- Titles are named without the list's dressing: a language or country prefix such as `EN -` or `|FR|`, and quality tags.
- A year written after a name becomes the title's year.
- Titles keep the provider's identifier. Their item identifiers survive refreshes, changes of order and new credentials.
- So do users' played state, resume points, favorites and next up.
- When movies or series are imported, an M3U list may reach 400 MB and 400,000 titles.

### In apps

Movies and series are ordinary titles in apps: details, search, latest, genres, continue watching, next up and collections.

- They are newest first in their libraries, which are paged and counted whole.
- **Titles read per movie and series catalog** does not bound them: they are stored, not asked of an addon page by page.
- Listings use only the lists' data.
- An Xtream title's details are asked only when an app opens the title or plays it: `get_vod_info` and `get_series_info`.
- `get_vod_info` gives the overview, cast, director, genres, release date, duration, backdrop, trailer and TMDB id. `get_series_info` gives the seasons and episodes.
- Details are kept until the series changes, or for seven days.
- Details are asked of a provider host at most once a second, as providers ban accounts that send bursts.

### Playback

Each movie or episode has one version: the provider's file. For Xtream, it is `/movie/…` or `/series/…` with its container.

- The version is labelled with the quality its name gives (4K, FHD, HD, SD), for quality groups.
- It plays, converts, downloads, and takes thumbnails and subtitles as any title's (see [Playback](playback.md) and [Transcoding](transcoding.md)).

An MPEG-TS file has no index to cut it by its keyframes. This also holds for addons' MPEG-TS files.

- It plays as it is on apps that take MPEG-TS. Elsewhere it plays only converted.
- Converted, it is cut into a 2-second segment, then one every 4 seconds over its duration. The encoder places a keyframe at each segment's start.
- After a seek, FFmpeg reads from before the time asked, as it lands on the first keyframe after the time it is given. What comes before the segment is left out.
- How far before is kept with the file, for its next playbacks (see [HLS segments](transcoding.md#hls-segments)).
- A user who may not have video converted is told no stream suits.

**Compared with Jellyfin**

- Polyfin plays MPEG-TS files as Jellyfin plays such files.

### Ratings and enrichment

Providers give no content rating, so titles are unrated unless enrichment brings one. A user who blocks unrated movies or series does not see them.

Enrichment is an import option, on by default. It brings the posters, cast and age ratings providers lack.

- It asks only the administrator's own addons, once for each title opened.
- A title the provider gives a TMDB or IMDb identifier is described by the first of the user's addons that describes that identifier.
- The provider's description fills what the addon lacks.
- The title's name, category and episodes stay the provider's.

