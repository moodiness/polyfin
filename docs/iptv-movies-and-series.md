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
- Titles are named without the list's dressing: a language or country prefix such as `EN -` or `|FR|`, and quality tags. With **Name titles after the metadata addons**, a title takes its metadata addon's name once opened (see [Names from the metadata addons](#names-from-the-metadata-addons)).
- A year written after a name becomes the title's year.
- Titles keep the provider's identifier. Their item identifiers survive refreshes, changes of order and new credentials.
- So do users' played state, resume points, favorites and next up. Grouping copies of a title changes this for the entries it groups (see [Versions of the same title](#versions-of-the-same-title)).
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

Each movie or episode has one version: the provider's file. For Xtream, it is `/movie/…` or `/series/…` with its container. With **Group copies of a title as versions**, a title has one version per entry of the provider's list (see [Versions of the same title](#versions-of-the-same-title)).

- The version is labelled with the quality its name gives (4K, FHD, HD, SD), for quality groups.
- It plays, converts, downloads, and takes thumbnails and subtitles as any title's (see [Playback](playback.md) and [Transcoding](transcoding.md)).
- It reaches apps through Polyfin, never from the provider's address, which holds the account (see [Who is sent to the source](playback.md#who-is-sent-to-the-source)).

An MPEG-TS file has no index to cut it by its keyframes. This also holds for addons' MPEG-TS files.

- It plays as it is on apps that take MPEG-TS. Elsewhere it plays only converted.
- Converted, it is cut into a 2-second segment, then one every 4 seconds over its duration. The encoder places a keyframe at each segment's start.
- After a seek, FFmpeg reads from before the time asked, as it lands on the first keyframe after the time it is given. What comes before the segment is left out.
- How far before is kept with the file, for its next playbacks (see [HLS segments](transcoding.md#hls-segments)).
- A user who may not have video converted is told no stream suits.

**Compared with Jellyfin**

- Polyfin plays MPEG-TS files as Jellyfin plays such files.

### Versions of the same title

**Group copies of a title as versions** is an import option of Xtream accounts, off by default. Providers often list a movie or a series more than once, in another quality or category, each entry with the same TMDB id. With the option on, a source's entries of one type that share a TMDB id are one title, whose versions are each entry's file.

- **Which entries:** those the source imports, their type on and their category not left out, that the provider's list gives a TMDB id (`tmdb` or `tmdb_id` in `get_vod_streams` and `get_series`). A TMDB id only in a title's details groups nothing. Entries of two sources are never grouped.
- **The title:** it takes the identifiers of its first entry, the first in the provider's order when the title formed. Libraries, counts, search, latest and genres list it once, with the name, poster and category of its first entry while the provider lists it, else of the entry that joined next. With one library per category, it is in that entry's category's library. Search finds it by that entry's name. With **Name titles after the metadata addons**, it shows the name it learned, and search finds it by that name too (see [Names from the metadata addons](#names-from-the-metadata-addons)).
- **Its versions:** one per entry the provider lists, in the order the entries joined, labelled by quality and category, such as `4K · 4K Movies`, then by name, or by the provider's identifier, when two are alike. The version choice settings order them as any title's (see [Version order](versions.md#version-order)).
- **A series' episodes:** matched by season and episode number. An episode another entry also has plays either file. An episode only one entry has is listed too, and plays that entry's file. An episode without a number, or a second episode of the same numbers in one entry, is listed apart, with its own file.
- **Details:** opening the title asks the provider for the details of each entry not known yet or old (`get_vod_info` or `get_series_info`), at most once a second per host: a title of three entries takes about two seconds more to open the first time. Playing an episode, the watchlist checks and the versions listed ahead ask for them too.
- **When the first entry leaves the list:** the title keeps its identifiers, and what users did with it, while the provider lists two or more of its entries. A title left with one entry the provider lists ends (see **Splitting a title** below), and forms again as the others come back. When the source shows none of them any more, not listed or left out with their category, the title waits for them under the same identifiers, and takes them back as they come.
- **Former identifiers:** an app that opens an entry's former movie, series, season or episode is answered the title, or its season or episode of the same numbers. Once a title splits, an app that opens one of its former episodes is answered the episode of the same numbers that took what users did with it (see **Splitting a title** below), and the title's episode again once the same first entry forms the title again. Nothing else answers them, as for a title the provider no longer lists.

**What users did** with an entry that joins a title goes to the title, its seasons and its episodes of the same numbers, by an import's rules (see [How the data merges with Polyfin's](moving-to-polyfin.md#how-the-data-merges-with-polyfin-data)):

- a played mark is added, the newer resume point is kept, a favorite is set, and a rating is given to a title the user did not rate;
- the entry keeps its played marks, favorites and ratings; its resume points move to the title;
- the first entry's movie or series is the title itself and keeps what users did with it; its episodes give theirs to the title's episodes, which are new items.

**Splitting a title.** Turning the option off makes each entry a title of its own again, under its own identifiers. So does a title whose first entry shows under another TMDB id or none, or that is left with one entry that may be grouped, the others kept apart, of another TMDB id, not listed or left out with their category: a title of two entries ends as soon as one of them does.

- What users did with the title, before and while it was grouped, stays with the first entry's movie or series. Each episode of the title gives what users did with it to the first entry's episode of the same numbers, else to the episode of the first entry, in the order they joined, that has one.
- When the provider no longer lists the first entry, the first entry still listed takes the title's data in its place.
- The other entries find what users did with them before they joined: played marks, favorites and ratings. Their resume points stay with the title.
- Turning the option on again groups them again, as above. An entry that joins a title again gives it again the played marks, favorites and ratings it kept.

**Keep apart.** A provider's TMDB id can be wrong. In **Content › Versions**, a grouped movie, or an episode of a grouped series, lists the title's entries under **Provider entries**, with **Keep apart** on each, and **Group again** on those kept apart (see [Versions of a title](versions.md#versions-of-a-title)).

- An entry kept apart is a title of its own again at once, and stays apart through refreshes and while the option is off. What users did with the title stays with the title.
- Keeping the first entry apart ends the title: that entry keeps the title's identifiers and what users did with the title. The other entries, when two or more, form a new title, under the identifiers of the first of them in the provider's order, and give it what users did with them.
- Keeping apart one of two entries ends the title too: the first entry keeps its identifiers and what users did with the title, and an episode only the other entry has gives what users did with it to that entry's episode.

**What grouping changes for a file**

- Versions are known by their title and their file: an entry's file that becomes a version of the title is a new version, whose analysis, thumbnails, pins and blocks start over. The first entry's movie file stays the same version; its episodes' files do not.
- A title keeps one resume point, whatever version plays (see [Watch state](watch-state.md)). Copies can differ in length, by an intro or a cut.
- Subtitle files users add apply to every version of the title (see [Subtitles](subtitles.md)).

**Compared with Jellyfin**

- Jellyfin groups the files of one movie folder as versions, and **Merge versions** joins items by hand. Polyfin has no **Merge versions**: it groups an IPTV source's entries by their TMDB id.

### Ratings and enrichment

Providers give no content rating, so titles are unrated unless enrichment brings one. A user who blocks unrated movies or series does not see them.

Enrichment is an import option, on by default. It brings the posters, cast and age ratings providers lack.

- It asks only the administrator's own addons, once for each title opened.
- A title the provider gives a TMDB or IMDb identifier is described by the first of the user's addons that describes that identifier.
- The provider's description fills what the addon lacks.
- The title's category and episodes stay the provider's.
- So do its name and its episodes' titles, unless **Name titles after the metadata addons** is on (see below).

### Names from the metadata addons

**Name titles after the metadata addons** is an import option, off by default, beside enrichment, which it needs: while enrichment is off, the option is unavailable. Provider lists often name titles with a brand or a number Polyfin cannot tell from the name, such as `D+ - …`. With the option on, a movie or series takes the name of the metadata addon that describes it.

- **When:** once the title is described with the addon's description, which an app opening it asks for (see [Ratings and enrichment](#ratings-and-enrichment)). It takes no request beyond enrichment's.
- **Where:** the title's details, its libraries, latest, search, sorting by name and the item Polyfin stores. Search finds it by either name, the addon's or the provider's, the titles whose name shown starts with the words first. A listing sorted by name catches up with a new name within a few minutes.
- **Episodes:** each takes the addon's title of the same season and episode number, where the addon has one. The others keep the provider's. Episode titles are not stored: they follow the addon's description Polyfin keeps, and show the provider's again while it has none.
- **Lists mix both names** until their titles are opened: a title not opened yet keeps the provider's name, cleaned as above. Once opened, it can move in a listing sorted by name.
- **Which addon:** the one enrichment uses, without a language. A source the server's users share takes names only from the server's own addons that everyone uses: never from an addon whose **Used by** is limited, nor from a user's own addon. A user's own source takes names from that user's addons too.
- **A group of copies** (see [Versions of the same title](#versions-of-the-same-title)) takes the name learned under its first entry's identifiers, and keeps it while the group lasts, even once the provider no longer lists that entry.
- **What stays the provider's:** item identifiers, what users did with the titles, groups and the labels of their versions, the entries **Content › Versions** lists under **Provider entries**, and M3U series, which group their episodes by the provider's names.
- **An administrator's name wins:** a name set with [Edit metadata](artwork-and-editing.md#edit-metadata) shows over the addon's. It also fixes the wrong name a wrong TMDB or IMDb id in the provider's list gives.
- **Off:** turning the option off, or enrichment off, shows the provider's names again. The names learned are kept, and show again when the option is turned back on, without opening the titles again.
- **A name goes** with its title: once the provider no longer lists it, unless it leads a group of copies, or lists it under another TMDB or IMDb id. Turning **Movies** or **Series** off forgets that type's names, but for the first entries of groups of copies, as their titles are no longer imported. Turning **Group copies of a title as versions** off, or keeping a first entry apart, forgets the name of a first entry the provider no longer lists.

