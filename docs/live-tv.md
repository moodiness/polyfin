# Live TV

This page covers Live TV in Polyfin: the channels that come from addon TV catalogs and IPTV sources, the programme guide, and recordings. To import an M3U playlist or an Xtream Codes account directly, see [IPTV sources](iptv.md).

## Channels from addons

Stremio TV catalogs (the `tv` type) become Jellyfin Live TV. A TV catalog enabled under **Content › Libraries** adds no library. Its channels appear in the Live TV view of Jellyfin apps instead, with their logos, numbered in the order the catalogs list them. See [Addons and libraries](addons-and-libraries.md) for enabling catalogs.

- Polyfin reads at most **Channels read per Live TV catalog** channels from each TV catalog, and as many programmes from each day of its guide.
- Who can watch is set per user with the **Live TV** permission on [Users](users.md).

### The Live TV page

**Content › Live TV** gathers Live TV in the admin app:

- **TV catalogs**: each addon TV catalog with its guide state (such as **Guide complete** or **Guide download failed**) and how many channels have a guide, with **Guides and mapping** to open its guides. A catalog not shown as a library is marked **Not in Live TV**, with **Open Libraries**.
- **IPTV channels**: each IPTV source's live entries, channels on, shown in apps and with a guide, with **Line-up and guides** to open [its source page](iptv.md#the-source-page).
- **Recordings**: the recordings under way and scheduled.

**Live TV settings** and **Recording settings** open those settings sections.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Channels read per Live TV catalog** | **Settings › Catalogs** | 10,000 | Most channels read from each TV catalog, and most programmes read from each day of its guide. |

### How a channel plays

A channel plays as the addon streams it when the app takes it.

- An HLS stream is relayed through Polyfin, with the headers the source needs, unless the app can reach the source itself. jellyfin-web always gets it relayed, as browsers cannot read other sites' playlists.
- Otherwise FFmpeg remuxes the stream into HLS as it comes, or converts what the app cannot take (see [Transcoding](transcoding.md)). This runs from the moment the app opens the channel until it leaves it. A minute without requests stops it. Its first segment lasts one second, so playback starts about a second after FFmpeg does.
- An MPEG-TS stream played as it is (apps such as Android TV) is relayed from the same connection, unless the app can reach the source itself and the source has no connection limit, in which case the app is sent to it.

Each channel's stream is read through one connection to its source, whoever reads it: the check at its start, FFmpeg, apps relayed, a recording, several users watching it. The connection stays open 20 seconds after the last of them left, so a quick return to the channel starts at once. Whoever starts reading an MPEG-TS stream, first or joining, starts at its latest keyframe, after the tables that describe it, so that it decodes from its first bytes. A reader that falls more than a few seconds behind is dropped rather than holding the others back.

An Xtream account tells, at its login, how many streams it plays at once. Polyfin keeps within it: opening a channel closes first a stream no one watches any more, then the user's own oldest one; when other users' channels take every connection, the new channel is refused at once (the app tells it could not play). A stream being closed, as when its 20 seconds end, still counts until its connection is closed: a channel opening meanwhile waits for it, a second at most. Right after closing a stream, a refusal from the provider is tried again for a few seconds, as providers count a connection a little while after it closed.

#### Starting a channel

Before anything reads a channel's stream, Polyfin checks its first bytes. A source that answers with an error, a web page, a JSON or text answer, an empty or short body, or bytes of no video container fails within a second, and the channel's next stream is tried at once. A source that sends nothing within 3 seconds (10 for its answer, as providers that redirect to the stream can take several seconds) fails as silent. What the stream holds is then analyzed from about a second of it, within **Maximum time to analyze a version** but never more than 8 seconds, and kept: later starts of the channel, even after a restart, skip the analysis for a week, and analyze it again on the way, from the stream already read, once it is an hour old. FFmpeg then reads it with half a second of probing; if FFmpeg fails within 10 seconds, the stream is analyzed again in full.

A live stream has no end: when its source closes the connection, or sends nothing for 10 seconds, FFmpeg reads it again after 1, 2, then 4 seconds, and the playlist goes on with a discontinuity instead of ending. Past 5 restarts in 2 minutes it gives up, until the app asks again.

How each stream of an IPTV source answered is kept (see [IPTV sources](iptv.md#stream-health)): a stream found dead is left out of its channel for a while, so the next start goes to a stream that works.

## Programme guide

Without a guide, the programme guide is empty and the channels play all the same. A guide can come from the addon itself (Native EPG) or from XMLTV files you attach to the catalog.

**Compared with Jellyfin:**
- An empty guide behaves as Jellyfin's does without guide data.

### Native EPG

Addons that publish a guide (Stremio's Native EPG: a TV catalog taking a date) fill the programme guide. Polyfin reads it a day at a time and keeps it ten minutes.

### XMLTV guides

Some IPTV addons publish no Native EPG guide, while their provider publishes one as an XMLTV file. You can attach such guides to any TV catalog shown under **Content › Live TV**, or under **My sources** for a user's own, once the libraries are saved.

- Each catalog takes up to 10 guides, in order, from its **Guides** page, saved with **Save and download**. Its row shows the first one.
- On a Stremio addon's catalog, open the page with **Guides and mapping** on its row under **Content › Live TV**. An IPTV source has the same views on [its source page](iptv.md#the-source-page).
- An address may hold the provider's credentials. Like manifest addresses, it is never shown in full.
- Only administrators can use a guide address on a local network address.
- A channel with Native EPG programmes keeps them: the XMLTV guide only fills the channels without.
- Removing a guide removes its mappings with it.

**Compared with Jellyfin:**
- Jellyfin's XMLTV guides belong to its tuner setup; Polyfin attaches a guide to a TV catalog.

#### When guides are fetched

Polyfin fetches a guide:

- when its address is saved;
- again once **Refresh Live TV lists and guides every** hours have passed (Polyfin looks for the guides due every 5 minutes);
- after a failed download, again after 5 minutes, then 15 minutes, then every hour (never later than the setting), or later when the server asked to wait;
- when you press **Download again** on the **Guides** page, which fetches every guide of the catalog.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Refresh Live TV lists and guides every (hours)** | **Settings › Live TV** | 12 hours (as before the setting existed); 1 to 168 | How often guides, and IPTV lists, are downloaded again. Failed downloads are tried again sooner. |

The catalog's row then shows when its first guide was last fetched, when it will be fetched next, how many of its channels are mapped to a guide channel, and why the last fetch failed, if it did. The guide page shows the same for each guide, with the channels and programmes it held.

#### Guide files and limits

- The file may be plain XML, compressed with gzip, or a ZIP archive. Polyfin tells which by its content, not its name.
- Up to 300 MB downloaded, and four times as much once uncompressed.
- Plain and gzip guides are read as they arrive, never whole.
- A ZIP archive can only be read once complete. It is first written to a temporary file in Polyfin's cache folder (`cache` in `POLYFIN_DATA_DIR`, see [Configuration](configuration.md)), removed as soon as it is read or fails. Its `.xml` file, the largest if it holds several, is then read the same way. An archive without one is reported as not a guide.
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
- from the **Guides** page's **Automatic mapping**, where **Map channels without a guide** maps only the channels without a mapping and **Remap every channel** remaps every channel, after asking.

The **Guide mapping** page also lists the catalog's channels with their mapping: all, with a guide, without a guide or set by hand. It searches the guides' channels by name or identifier, showing what each airs now. From there you can map a channel by hand (**Choose a guide channel**), or to no guide at all.

A choice made by hand survives downloads, refreshes and **Map channels without a guide**. Only **Remap every channel**, or **Back to automatic**, returns the channel to automatic mapping.

#### Where XMLTV programmes appear

XMLTV programmes appear wherever Native EPG programmes do, with the same filters: the programme listings, recommended programmes, a programme's details, and the programme each channel airs now. A guide category such as "Movie", "News" or "Sports" marks programmes as a Native EPG genre does.

XMLTV programmes are read from the guide when asked: a guide page reads only its channels' programmes, and a listing by start time (such as the upcoming rows of the Live TV page) reads no further than it shows. They are not stored apart from the guide; a programme opened by its identifier is found again in the guide. A programme's image, its guide's `icon`, is relayed for the programmes listed in the last day, as images are served without signing in. Native EPG programmes listed are kept, and deleted two days after they end, unless a recording or a timer names them.

Recommended programmes airing now are listed by channel number; those to come, by start time.

**For app developers:**
- Programmes from either guide give apps their episode title and their season and episode numbers, as Jellyfin's do.
- `/LiveTv/GuideInfo` still answers the seven days Jellyfin's guide spans.

## Recordings

Polyfin records Live TV programmes, as Jellyfin's DVR does, once **Record Live TV** is turned on, under **Settings › Recordings**. Recordings are written to **Recordings folder**: when it is empty, to `recordings` in Polyfin's data folder, `/data/recordings` in the Docker image's volume, which Polyfin creates.

- To keep them on another disk, mount a folder of the server at `/data/recordings`: with Compose, add `- ./recordings:/data/recordings` under `volumes` in the `polyfin` service of `compose.yaml`; on Unraid, set the template's **Recordings folder**. A folder mounted elsewhere in the container works too: enter its path in **Recordings folder**.
- Polyfin must be able to write to the folder, as the container's user, UID 65532: it checks it when the setting is saved, and refuses a folder it cannot write to. **System › Health** lists a folder it can no longer write to.
- Changing the folder does not move the recordings already made: they are found again once moved there.

While recording is off, Polyfin answers as a server that records nothing: the recording, timer and series timer lists stay empty, what they would hold is not found, and new timers are refused (400).

### Who can record

On a user's page under **Users**, **Can record Live TV** at the end of **Playback and access** lets a user schedule, change and cancel recordings and delete them. It is on for administrators and off for other users by default. Seeing recordings needs the **Live TV** permission only. See [Users](users.md).

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
| **Start recordings early (minutes)** | **Settings › Recordings** | none (0), as in Jellyfin; 0 to 60 minutes | Starts a recording this many minutes before its programme. Apps can change it per timer. |
| **Keep recording after the end (minutes)** | **Settings › Recordings** | none (0), as in Jellyfin; 0 to 60 minutes | Goes on recording this many minutes after the programme. Apps can change it per timer. |
| **Keep recordings for (days, 0 = forever)** | **Settings › Recordings** | 0 (forever); up to 3,650 days | Deletes older recordings once a day. |

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
