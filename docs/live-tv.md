# Live TV

This page covers Live TV in Polyfin: the channels that come from addon TV catalogs and IPTV sources, the programme guide, and recordings. To import an M3U playlist or an Xtream Codes account directly, see [IPTV sources](iptv.md).

## Channels from addons

Stremio TV catalogs (the `tv` type) become Jellyfin Live TV. A TV catalog enabled under **Libraries** adds no library. Its channels appear in the Live TV view of Jellyfin apps instead, with their logos, numbered in the order the catalogs list them. See [Addons and libraries](addons-and-libraries.md) for enabling catalogs.

- Polyfin reads at most **Channels read per Live TV catalog** channels from each TV catalog, and as many programmes from each day of its guide.
- Who can watch is set per user with the **Live TV** permission on [Users](users.md).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Channels read per Live TV catalog** | **Settings › Catalogs** | 10,000 | Most channels read from each TV catalog, and most programmes read from each day of its guide. |

### How a channel plays

A channel plays as the addon streams it when the app takes it.

- An HLS stream is relayed through Polyfin, with the headers the source needs, unless the app can reach the source itself. jellyfin-web always gets it relayed, as browsers cannot read other sites' playlists.
- Otherwise FFmpeg remuxes the stream into HLS as it comes, or converts what the app cannot take (see [Transcoding](transcoding.md)). This runs from the moment the app opens the channel until it leaves it. A minute without requests stops it.

## Programme guide

Without a guide, the programme guide is empty and the channels play all the same. A guide can come from the addon itself (Native EPG) or from XMLTV files you attach to the catalog.

**Compared with Jellyfin:**
- An empty guide behaves as Jellyfin's does without guide data.

### Native EPG

Addons that publish a guide (Stremio's Native EPG: a TV catalog taking a date) fill the programme guide. Polyfin reads it a day at a time and keeps it ten minutes.

### XMLTV guides

Some IPTV addons publish no Native EPG guide, while their provider publishes one as an XMLTV file. You can attach such guides to any TV catalog shown under **Libraries**, or under **My sources** for a user's own, once the libraries are saved.

- Each catalog takes up to 10 guides, in order, from its guide page. Its row shows the first one.
- On a Stremio addon's catalog, open the page with **Guides and mapping** on its row under **Libraries**. An IPTV source has the same views on [its source page](iptv.md#the-source-page).
- An address may hold the provider's credentials. Like manifest URLs, it is never shown in full.
- Only administrators can use a guide address on a local network address.
- A channel with Native EPG programmes keeps them: the XMLTV guide only fills the channels without.
- Removing a guide removes its mappings with it.

**Compared with Jellyfin:**
- Jellyfin's XMLTV guides belong to its tuner setup; Polyfin attaches a guide to a TV catalog.

#### When guides are fetched

Polyfin fetches a guide:

- when its address is saved;
- again once **Refresh Live TV lists and guides every** hours have passed (Polyfin looks for the guides due every 30 minutes);
- when you press **Refresh guide**, which fetches every guide of the catalog.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Refresh Live TV lists and guides every** | **Settings › Live TV** | 12 hours (as before the setting existed); 1 to 168 | How often guides, and IPTV lists, are downloaded again. |

The catalog's row then shows when its first guide was last fetched, when it will be fetched next, how many of its channels are mapped to a guide channel, and why the last fetch failed, if it did. The guide page shows the same for each guide, with the channels and programmes it held.

#### Guide files and limits

- The file may be plain XML, compressed with gzip, or a ZIP archive. Polyfin tells which by its content, not its name.
- Up to 300 MB downloaded, and four times as much once uncompressed.
- Plain and gzip guides are read as they arrive, never whole.
- A ZIP archive can only be read once complete. It is first written to a temporary file in Polyfin's cache folder (`POLYFIN_CACHE_DIR`, see [Configuration](configuration.md)), removed as soon as it is read or fails. Its `.xml` file, the largest if it holds several, is then read the same way. An archive without one is reported as not a guide.
- The download must answer within 30 seconds, never stall for a minute, and end within 10 minutes.

#### What a download keeps

- The guide's channels, with their names and icons. A channel its programmes name without declaring it counts as one without a name.
- Its programmes from a day before the fetch to eight days after it, with their title, episode title, description, categories, season and episode numbers and image.
- A failed fetch keeps those of the last one.

#### How channels are matched to the guide

Each channel takes one guide channel. Several channels, such as a channel and its timeshift copy, may share one. Polyfin tries in this order:

1. **Identifier.** A guide channel whose identifier is the channel's Stremio ID, or, for a channel of an IPTV source, the channel's own guide identifier (its list's `tvg-id` or `epg_channel_id`), takes it.
2. **Identifier, loosely.** Lists often write that identifier in another case, or with a feed after an `@` (`Name.fr@SD`). Failing an exact match, a guide channel whose identifier is the same ignoring case takes it; then one equal to the part before the last `@`, exactly and then ignoring case. This comes before any name match, with ties ranked by country and then programmes as below.
3. **Name.** Otherwise the guide channel's display names are compared with the channel's name, once HTML entities left in them are decoded and these are set aside: case, accents, punctuation, separators such as "|", list prefixes such as "FR:", "FR|" or "KIDS|", and marker characters such as ᴴᴰ or ★. "+" reads as "plus", as identifiers write it. An exact match keeps quality tags such as HD, FHD, UHD, 4K or SD; a loose one sets them aside too, and the "+", which lists sometimes leave out ("Zeb Sport" for "Zeb+ Sport").
   - French lists write the public networks "France 2" to "France 5" as "F2" to "F5", followed by a region. On a French server, or under a French prefix, Polyfin reads them in full: "F3 Zebria" matches "France 3 Zebria". "F1" is left as it is.
4. **Identifier as a name.** Failing a display name, a guide channel whose identifier, without its country suffix, writes the channel's name takes it: "ZebPlus1.fr" for "Zeb +1", "ZebAndCo.fr" for "Zeb & Co". Guides keep identifiers when a channel is renamed, so a list still using the old name finds it.

Many guides list the same channel for several countries, so the candidates are ranked:

1. Those of the channel's own country first: the one its prefix names ("FR:"), else the one the server language suggests (France for French; none for English). A guide channel's countries are those of its identifier ("Name.fr") and of its display names' prefixes ("FR|").
2. Exact matches before loose ones, and loose ones before identifiers read as names.
3. The catalog's guide listed first.
4. Within a guide, the guide channel with the most distinct programme titles in the window kept. A placeholder repeating one title ("No Data", or "Name 4K" advertising itself) loses to a real guide.
5. The first one listed.

#### Remapping and mapping by hand

Channels are mapped again from what is kept, without downloading:

- after every download, list refresh and change of an IPTV source's options;
- from the guide page, where **Map unmapped channels** maps only the channels without a mapping and **Map all again** remaps every channel.

The guide page (**Guide mapping**) also lists the catalog's channels with their mapping: all, mapped, unmapped or set by hand. It searches the guides' channels by name or identifier, showing what each airs now. From there you can map a channel by hand, or to no guide at all.

A choice made by hand survives downloads, refreshes and **Map unmapped channels**. Only **Map all again**, or clearing it, returns the channel to automatic mapping.

#### Where XMLTV programmes appear

XMLTV programmes appear wherever Native EPG programmes do, with the same filters: the programme listings, recommended programmes, a programme's details, and the programme each channel airs now. A guide category such as "Movie", "News" or "Sports" marks programmes as a Native EPG genre does.

**For app developers:**
- Programmes from either guide give apps their episode title and their season and episode numbers, as Jellyfin's do.
- `/LiveTv/GuideInfo` still answers the seven days Jellyfin's guide spans.

## Recordings

Polyfin records Live TV programmes, as Jellyfin's DVR does, once `POLYFIN_RECORDINGS_DIR` names a folder it can write to (see [Configuration](configuration.md)). Polyfin does not start with a folder it cannot use.

Without it, recording is off and Polyfin answers as a server that records nothing: the recording, timer and series timer lists stay empty, what they would hold is not found, and new timers are refused (400).

### Who can record

Under **Users**, **Can record Live TV** at the end of **Playback and access** lets a user schedule, change and cancel recordings and delete them. It is on for administrators and off for other users by default. Seeing recordings needs the **Live TV** permission only. See [Users](users.md).

**Compared with Jellyfin:**
- **Can record Live TV** is Jellyfin's `EnableLiveTvManagement`, which apps also read and set in the user's policy.
- Without it, requests are refused with 403, as Jellyfin answers.

### Scheduling

From a programme of the guide, apps make:

- a **timer**, which records that programme;
- a **series timer**, which records every programme of the same title in the guide, on the same channel at the same time of day (within ten minutes), unless it records on any channel or at any time, as Jellyfin's do.

Notes on series timers:

- The guide marks no repeats, so "new episodes only" records every one.
- Like Jellyfin, the days of the week a series timer lists are not looked at.
- **Keep up to** deletes a series' oldest recordings past that many.

Timers, series timers and recordings are kept in the database.

**For app developers:**
- Guide programmes show their timers (`TimerId`, `SeriesTimerId`).

### Recording settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Start recordings early** | **Settings › Recordings** | none (0), as in Jellyfin; 0 to 60 minutes | Starts a recording this many minutes before its programme. Apps can change it per timer. |
| **Keep recording after the end** | **Settings › Recordings** | none (0), as in Jellyfin; 0 to 60 minutes | Goes on recording this many minutes after the programme. Apps can change it per timer. |
| **Keep recordings for** | **Settings › Recordings** | 0 (forever); up to 3,650 days | Deletes older recordings once a day. |

### How a recording is made

- FFmpeg copies the channel's stream, read as live playback reads it, with the headers it needs and within its addon's confinement, from the guide of the user who scheduled it.
- It counts among that user's 4 channels played through FFmpeg at once, and the server's 16. Past the user's 4, it takes the place of their oldest channel watched. A channel watched never takes the place of a recording.
- A stream that stops before the end is read again 30 seconds later.
- While it records, the file is written in MPEG-TS, which apps can follow live.
- Once the programme ends, it becomes one Matroska file in the folder. It plays like a movie's version: as it is, or remuxed into HLS (see [Playback](playback.md)).
- A recording under way when Polyfin stops is kept as a partial recording when Polyfin starts again. The rest of the programme, if it still airs, is recorded as another.

### Watching recordings

Recordings are listed newest first, and in a **Recordings** folder. They are shown to the users who reach the channel they come from and to the user who scheduled them. Parental control and blocked genres apply to recordings as to titles, and to the programmes users schedule.

**Compared with Jellyfin:**
- For parental control, Jellyfin judges recorded videos as "Other" items, and scheduled programmes as "LiveTvProgram".
- A recording plays through PlaybackInfo only once finished.
- Jellyfin's keep-until choices are kept, but only **Keep recordings for** and **Keep up to** delete recordings.
- Jellyfin's `IsPrePaddingRequired` and `IsPostPaddingRequired` are not kept.

**For app developers:**
- A recording under way can be followed at `/LiveTv/LiveRecordings/{id}/stream`. Unlike Jellyfin, it needs a signed-in user's credentials or play session, like Polyfin's other media.
- Recordings are listed under `/LiveTv/Recordings`, and the folder at `/LiveTv/Recordings/Folders`.
- A timer or series timer asked for a programme Polyfin does not know answers 404 (`Timers/Defaults`) or 400 (`SeriesTimers`), where Jellyfin fails with 500.
- `POST /LiveTv/Timers/{id}` changes the timer the route names, not the one in the body.
