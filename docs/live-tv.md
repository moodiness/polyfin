# Live TV

This page covers Live TV in Polyfin: the channels that come from addon TV catalogs and IPTV sources, the channels Polyfin airs from your own titles, the programme guide, and recordings. To import an M3U playlist or an Xtream Codes account directly, see [IPTV sources](iptv.md). The past programmes an IPTV provider keeps play from a Replay view, described there under [Replay](iptv.md#replay).

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

- An HLS stream is relayed through Polyfin, with the headers the source needs, unless the app can reach the source itself. jellyfin-web always gets it relayed, as browsers cannot read other sites' playlists. The channels of IPTV sources are requested with the user agent and headers their list or source gives, else a web browser's user agent (see [Headers and user agent](iptv.md#headers-and-user-agent)).
- Otherwise FFmpeg repackages the stream into HLS as it comes, or converts what the app cannot take (see [Transcoding](transcoding.md)). This runs from the moment the app opens the channel until it leaves it. A minute without requests stops it.
  - Converted video has a keyframe every 3 seconds, which cut its segments. Video copied as it is can only be cut at the source's keyframes.
  - The app gets the first playlist once it lists the segments the app asks for (`MinSegments`: jellyfin-web asks 1, and 2 on Apple's systems), and at least two segments, of 4 seconds or more in all. Players start a live stream at the first segment its playlist lists: with this lead they stay a few seconds behind FFmpeg, so that a source that pauses a moment, or a conversion that falls behind briefly, does not stall the picture.
  - Playback then starts about 4 seconds after FFmpeg does, up to two keyframe intervals when video copied has keyframes far apart: a few seconds more than with one segment, for a picture that holds through the source's short pauses.
  - When the stream's connection is open already, as after its analysis, a play prepared ahead from the title's page, or a return to the channel within 20 seconds, FFmpeg starts from what the connection kept: the lead is there at once, and playback starts within a second or two for video copied, a few seconds sooner for video converted. The lead itself is no shorter.
  - Once FFmpeg was started again (see below), a playlist is answered as soon as it lists a segment.
- An MPEG-TS stream played as it is (apps such as Android TV) is relayed from the same connection, unless the app can reach the source itself and the source has no connection limit, in which case the app is sent to it.
- A DASH stream (an MPD manifest) of an IPTV source or an addon always plays converted: FFmpeg reads the manifest and its segments through Polyfin and repackages it into HLS, decrypting a ClearKey stream with the keys its list gives. An app that sends no device profile is offered no DASH stream; Widevine and PlayReady streams are not played (see [DASH channels and encryption](iptv.md#dash-channels-and-encryption)).

The files of an HLS stream reach FFmpeg, and apps relayed, through Polyfin, which takes care of what some providers do to pass their segments for something else:

- Segments named oddly, such as `1234.png`, `1234.jpg`, `seg.php?n=1234` or `1234` without an extension, are named as video segments, `.ts` added: FFmpeg (since 7.0.3 and 7.1.1) refuses a segment whose name does not fit what it holds, with "Invalid data found when processing input". FFmpeg still checks what each segment holds: a picture or a web page is not read as video.
- A picture in front of a segment, a PNG, JPEG, GIF or WebP before its MPEG-TS, is skipped, as other players do, when the MPEG-TS starts within the segment's first 64 kB. FFmpeg would read the picture instead.

Each channel's stream is read through one connection to its source, whoever reads it: the check at its start, FFmpeg, apps relayed, a recording, several users watching it. The connection stays open 20 seconds after the last of them left, so a quick return to the channel starts at once. It keeps the last 6 MB it received. Whoever starts reading an MPEG-TS stream, first or joining, starts at a keyframe, after the tables that describe it, so that it decodes from its first bytes: the oldest keyframe kept within 6 seconds of the latest one, by the stream's own times, else, for a stream whose times are not told, the first one within the last 3 MB, whichever comes first, and always with 1 MB of the 6 left before the connection overwrites it. A reader that falls more than a few seconds behind is dropped rather than holding the others back.

An Xtream account tells, at its login, how many streams it plays at once. Polyfin keeps within it: opening a channel closes first a stream no one watches any more, then the user's own oldest one; when other users' channels take every connection, the new channel is refused at once (the app tells it could not play). A stream being closed, as when its 20 seconds end, still counts until its connection is closed: a channel opening meanwhile waits for it, a second at most. Right after closing a stream, a refusal from the provider is tried again for a few seconds, as providers count a connection a little while after it closed.

An HDHomeRun tuner plays as many channels at once as it has tuners, which its `discover.json` tells: each channel watched or recorded takes one, whoever reads it, and Polyfin keeps within them the same way (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)). A tuner whose tuners another app took answers that all are in use: the channel is refused, its stream is refused by the provider for this start only, and the tuner shows no problem under **System › Health**.

#### Starting a channel

Before anything reads a channel's stream, Polyfin checks its first bytes. A source that answers with an error, a web page, a JSON or text answer, an empty or short body, or bytes of no video container fails within a second, and the channel's next stream is tried at once. A source that sends nothing within 3 seconds (10 for its answer, as providers that redirect to the stream can take several seconds) fails as silent. What the stream holds is then analyzed from about a second of it, within **Maximum time to analyze a version** but never more than 8 seconds, and kept: later starts of the channel, even after a restart, skip the analysis for a week, and analyze it again on the way, from the stream already read, once it is an hour old. FFmpeg then reads it with a second and a half of probing at most, enough for audio that comes a second after the video, as in streams FFmpeg itself muxed, and keeps what it probed as the start of what it writes; if FFmpeg fails within 10 seconds, the stream is analyzed again in full.

Interlaced channels, such as most TV sports in 1080i50, are converted for the apps that do not take interlaced video, jellyfin-web among them. They are deinterlaced to 25 frames a second by default, which looks jerky in sports: **Double the frame rate** under **Settings › Conversion (transcoding)** makes them 50 frames a second, for about 15% more processor time on a 1080i50 stream (see [Transcoding](transcoding.md)).

A live stream has no end: when its source closes the connection, or sends nothing for 10 seconds, FFmpeg reads it again after 1, 2, then 4 seconds, and the playlist goes on with a discontinuity instead of ending. So does a stream FFmpeg made no segment of for 20 seconds while the app plays it, as when its source answers each segment with an error or a page: FFmpeg skips the segments it cannot read and would otherwise wait for good. Past 5 restarts in 2 minutes it gives up, until the app asks again, and the log says so (`A live encoding gave up: its stream failed too often`), but for a source that refused the stream, as when a recording took the tuner: the app is told the stream is refused, and the log says so at Info level.

How each stream of an IPTV source answered is kept (see [IPTV sources](iptv.md#stream-health)): a stream found dead is left out of its channel for a while, so the next start goes to a stream that works. A stream that failed to play in the last 15 minutes, of an IPTV source, an addon's channel or a live video, is left out of PlaybackInfo meanwhile, and the log says so (`A channel's stream is left out: it failed lately`), with why it failed.

**Compared with Jellyfin:** Jellyfin waits for the segments the app asks for before it answers the first live playlist (`DynamicHlsController.GetLiveHlsStream`, which calls `HlsHelpers.WaitForMinimumSegmentCount`, [lines 325-329](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/DynamicHlsController.cs#L325-L329)). Polyfin waits for them too, and also for its own lead of two segments of 4 seconds, as jellyfin-web asks only one on most systems, which leaves the player right behind FFmpeg.

When a file of an HLS stream, a playlist, a key or a segment, answers no media while the stream is analyzed (an error, a web page, JSON, text or nothing), the failure says which file, by its name only (a name without an extension, which may be a token, is not shown), and what it answered, as in `nothing came from the source in time: …; 1234.ts answered text (HTTP 404, text/plain, 4 bytes)`. When FFmpeg refuses a segment by its name, its own line is kept, the address cut to the file name. The log lines `A channel's stream failed` and `A channel's stream could not be analyzed` name the stream and the channel (`stream`, `channel`).

**Compared with Jellyfin:** Jellyfin 12.4 repackages a channel's HLS stream with FFmpeg's defaults ([M3UTunerHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/M3UTunerHost.cs#L161-L167)), so FFmpeg refuses its segments named oddly, or starting with a picture. Polyfin names the segments it relays, and skips the picture.

Once a channel plays, converted or repackaged by FFmpeg, the same holds for what its source answers. When FFmpeg fails (`A live encoding failed`), stalls (`A live encoding stalled`), or the stream ends and is read again (`A live stream is read again`), the failure and the log line add the last file of the stream that answered no media since FFmpeg last made a segment, by its name only, as in `FFmpeg made no segment of the live stream for 20s; source unavailable: 1234.ts answered text (HTTP 404, text/plain, 4 bytes)`, and name the channel (`channel`). A file that answered no media before the last segment is not told: FFmpeg read on past it. What FFmpeg writes keeps the names of the files it read, never their addresses, which hold the source's, nor the host and port of Polyfin's relay it reads them through. The lines `A segment could not be served in time` and `A live stream was refused by its source` name the channel too.

**Compared with Jellyfin:** Jellyfin 12.4 writes what FFmpeg writes for each conversion or repackaging to a file of its own in its log folder, such as `FFmpeg.Transcode-…log`, after the version's media source and FFmpeg's command line, addresses included ([TranscodeManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.MediaEncoding/Transcoding/TranscodeManager.cs#L451-L486)), and does not tell what the source answered. Polyfin keeps FFmpeg's last lines in the server's log, without addresses, with what the source answered.

### Searching

An app's search finds the user's channels by name, those of TV catalogs and IPTV sources alike, as their apps list them. A search that asks for programmes also finds the programmes of their guides that have not ended, airing now or in the guide's days ahead. Case and accents do not count: "tele" finds "Télé Orbe".

- Searched from the home page, jellyfin-web 12.4 lists the channels found under "Channels" and the programmes under "Programs". It asks for the channels in the same request as movies, shows, music and the other types, then lists each type found under its own heading. A user without the **Live TV** permission finds no channel or programme.
- Searched from the Live TV page, it lists the programmes under "Movies", "Episodes", "Sports", "Kids", "News" and "Programs", by their guide categories, and the channels under "Channels".
- Selecting a channel found plays it, from either search.
- Polyfin searches the channels it lists, not the addons' own searches: a channel no TV catalog lists, or one turned off in an IPTV line-up, is not found.

Polyfin adds nothing to the web client for this: jellyfin-web already asks for channels from the home page, and labels them itself. Its search also asks for every video but movies, episodes and channels, which it lists under "Videos". Polyfin leaves programmes out of that answer, so that each programme shows once, under "Programs".

**For app developers:**
- `/Items` with `searchTerm` finds channels asked as `TvChannel` or `LiveTvChannel`, and programmes asked as `LiveTvProgram`, `Program` or `TvProgram`, filtered by `isMovie`, `isSeries`, `isNews`, `isKids` and `isSports`. It finds at most `limit` of each.
- A search that names no type finds channels, and programmes only when it sets one of those five filters. A search that names types finds programmes only when one of them is a programme type.
- Search hints leave channels and programmes out.

## Channels of your titles

Polyfin can air channels of its own, around the clock, from the movies and episodes Jellyfin apps list: **Content › Live TV**, under **Channels of your titles**, with **Add a channel**. Under **Built from**, a channel airs one of:

- **Collection**: one made in a Jellyfin app, or one of an addon, its titles listed as a collection row lists them;
- **Smart list**: one of the server's (see [Smart lists](addons-and-libraries.md#smart-lists));
- **Series**: its episodes, season after season, specials and episodes not aired yet left out;
- **Titles**: movies and series you pick by searching them; a series airs its episodes.

Its titles are read when the channel is saved, at most 5,000 movies and episodes, and kept as they were: titles added to the collection later are not aired until you save the channel with **Read the titles again**. The schedule is then a pure function of the channel's start, the midnight of the day it was saved, in the server's time zone, its seed and its titles. It survives restarts, gives the same guide whenever it is computed, and is stored nowhere. Each title airs for its runtime as its description gives it; a title without one is given 90 minutes for a movie, 30 for an episode.

- Titles air one after the other, in their order, round after round. **Shuffle** airs them in a new order each round, the same on every computation: changing the sources or the shuffle starts the schedule again today, shuffled anew.
- **Time blocks** air their own titles each day, from their start to their end, in the server's time zone (at most 12, apart from each other), and the channel's titles fill the rest of the day. Each block, and the channel's own titles, go on each day from where they stopped. A programme the end of its time cuts airs in full at the start of the next, unless it started that time.
- **Number** gives the channel its number; empty, it follows the other channels. **Shown in apps** off hides it from every user.

The channel is an ordinary channel for apps: listed after the channels of TV catalogs, to the users with the **Live TV** permission, with its programmes in the guide, in searches, in recommended programmes and as what airs now. Programmes are movies or series as their category, with their episode title, season and number, and their title's image. Recordings, timers, series timers and [reminders](#programme-reminders) work on them as on any channel.

- A user sees the channel only when their parental control and blocked genres let them reach every movie and episode it airs, by the ratings and genres those had when it was saved: a stream other users watch cannot leave a programme out.
- The programmes of a title a user hid are left out of their guide.

### How a channel of your titles plays

The programme airing when a player tunes in plays from where it is: tuned 90 seconds after a programme started, the channel starts 90 seconds into it. The next programmes follow as one live HLS stream, converted by FFmpeg in real time to H.264, at most 1080p, or what **Maximum quality of converted video** allows, and stereo AAC, with a discontinuity at each programme. The stream plays as an IPTV channel's HLS stream does (see [How a channel plays](#how-a-channel-plays)): relayed to apps, converted again only for an app that cannot take it, and recorded from the same playlist.

- One FFmpeg airs a channel, whoever watches or records it, from the first player on, and stops a minute after the last left. It counts among the server's 16 live encodings and among **Video conversions at once**.
- A user whose quality group is below the shared stream, such as 720p for a 1080p programme, gets it converted down to their group by a live encoding of their own, as for any channel taller than their group (see [Quality groups](users.md#quality-groups)); without the permission to convert video, the channel is refused to them.
- Each programme plays from its title's versions: those of local folders first, then the others in the order of the [version rules](playback.md), at most **Versions tried when one does not work** of them. A version that fails, at its start or later, passes the programme to the next one from where it stopped.
- When none plays, a black and silent slate airs for a minute at most, and the versions are tried again. A programme shorter than its runtime is followed by the slate until its time ends.
- The log tells it: `A version of a programme could not air`, `No version of a programme plays: a slate airs instead`.

**Compared with Jellyfin:** Jellyfin has no channels of its own titles. Apps see these channels and their programmes as Jellyfin's `TvChannel` and `Program` items, and play them as live streams.

**For app developers:** the admin app's routes, for administrators: `GET` and `POST /admin/api/live-tv/channels`, `GET`, `PUT` and `DELETE /admin/api/live-tv/channels/{id}`, `GET /admin/api/live-tv/channels/choices` for the collections and smart lists, and `GET /admin/api/live-tv/channels/titles?q=` for movies and series. A `PUT` that changes the sources, the blocks or the shuffle, or sets `rebuild`, reads the titles again and starts the schedule again today.

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
- Only administrators can use a guide address on a local network address, or a guide that is a file of the server, by its path (**Guide address or file**; see [Lists and guides that are files of the server](iptv.md#lists-and-guides-that-are-files-of-the-server)). Its path is shown whole.
- A channel with Native EPG programmes keeps them: the XMLTV guide only fills the channels without.
- Removing a guide removes its mappings with it.

**Compared with Jellyfin:**
- Jellyfin's XMLTV guides belong to its tuner setup; Polyfin attaches a guide to a TV catalog.

#### When guides are fetched

Polyfin fetches a guide:

- when its address is saved;
- again once **Refresh Live TV lists and guides every** hours have passed (Polyfin looks for the guides due every 5 minutes);
- after a failed download, again after 5 minutes, then 15 minutes, then every hour (never later than the setting), or later when the server asked to wait;
- for a guide that is a file of the server, within 5 minutes once the file changed, or became unreadable;
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
- Its programmes from a day before the fetch to eight days after it, with their title, episode title, description, categories, season and episode numbers, image, and whether the guide marks them new (`<new/>`) or previously shown (`<previously-shown>`); a programme marked both is new. The guide channels of IPTV channels whose provider keeps past programmes keep theirs as far back as that archive, at most 30 days (see [Replay](iptv.md#guides-and-the-archive-window)).
- A failed fetch keeps those of the last one.

#### How channels are matched to the guide

Each channel takes one guide channel. Several channels, such as a channel and its timeshift copy, may share one. Polyfin tries in this order:

1. **Identifier.** A guide channel whose identifier is the channel's Stremio ID, or, for a channel of an IPTV source, the channel's own guide identifier (its list's `tvg-id` or `epg_channel_id`), takes it.
2. **Identifier, loosely.** Lists often write that identifier in another case, or with a feed after an `@` (`Name.fr@SD`). Failing an exact match, a guide channel whose identifier is the same ignoring case takes it; then one equal to the part before the last `@`, exactly and then ignoring case. This comes before any name match, with ties ranked by country and then programmes as below.
3. **Number.** For a channel of an HDHomeRun tuner, a guide channel whose display name is the channel's number ("5.1"), alone or before its name ("5.1 Name"), takes it. "5.1" is neither "5.10" nor "15.1". Guides of terrestrial channels often name them so. A channel given a fixed number under **Channels** is matched by its name.
4. **Name.** Otherwise the guide channel's display names are compared with the channel's name, once HTML entities left in them are decoded and these are set aside: case, accents, punctuation, separators such as "|", list prefixes such as "FR:", "FR|" or "KIDS|", and marker characters such as ᴴᴰ or ★. "+" reads as "plus", as identifiers write it. An exact match keeps quality tags such as HD, FHD, UHD, 4K or SD; a loose one sets them aside too, and the "+", which lists sometimes leave out ("Zeb Sport" for "Zeb+ Sport").
   - French lists write the public networks "France 2" to "France 5" as "F2" to "F5", followed by a region. On a French server, or under a French prefix, Polyfin reads them in full: "F3 Zebria" matches "France 3 Zebria". "F1" is left as it is.
5. **Identifier as a name.** Failing a display name, a guide channel whose identifier, without its country suffix, writes the channel's name takes it: "ZebPlus1.fr" for "Zeb +1", "ZebAndCo.fr" for "Zeb & Co". Guides keep identifiers when a channel is renamed, so a list still using the old name finds it.

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

XMLTV programmes appear wherever Native EPG programmes do, with the same filters: the programme listings, recommended programmes, searches, a programme's details, and the programme each channel airs now. A guide category such as "Movie", "News" or "Sports" marks programmes as a Native EPG genre does.

XMLTV programmes are read from the guide when asked: a guide page reads only its channels' programmes, and a listing by start time (such as the upcoming rows of the Live TV page) reads no further than it shows. They are not stored apart from the guide; a programme opened by its identifier is found again in the guide. A programme's image, its guide's `icon`, is relayed for the programmes listed in the last day, as images are served without signing in. Native EPG programmes listed are kept, and deleted two days after they end, unless a recording or a timer names them.

Recommended programmes airing now are listed by channel number; those to come, by start time.

**For app developers:**
- Programmes from either guide give apps their episode title and their season and episode numbers, as Jellyfin's do.
- XMLTV programmes the guide marks previously shown, and not new, are repeats (`IsRepeat`), as Jellyfin reads XMLTV guides.
- `/LiveTv/GuideInfo` still answers the seven days Jellyfin's guide spans.

## Programme reminders

A user with the **Live TV** permission can be told before a programme starts, without recording it. On the page of a programme that has not started, the web client shows **Remind me** after the record buttons, and its menu **Remind me of every airing**: the programmes of the same title on the same channel at the same time of day, within ten minutes, as a series timer finds them, looked for in the guide when the reminder is set and every half hour. A programme with a reminder shows **Reminder set**, which removes it, or the series reminder that made it. Reminders follow parental control and blocked genres, as scheduled programmes do; a user who loses the permission, is disabled or whose account ends gets none.

As long before the programme as **Remind me before** says, under **My account › Notifications**, the user's own targets receive **Programme starting** (see [Programme reminders](notifications.md#programme-reminders)), and their apps too when they chose it. **My account › Reminders** lists the user's reminders and series reminders, with each programme's channel and start, and **Delete**.

A reminder is checked against the guide when it is due. A programme the guide moved within the hour keeps its reminder at its new start, also when it moved earlier: the guide is looked at every few minutes during the hour before a reminder is due. A programme gone from the guide sends nothing. Reminders are kept in the database: a restart neither sends one twice nor forgets one, and one due while Polyfin was off is sent on start only before its programme begins.

**For app developers:** Jellyfin's API has nothing for reminders, and Jellyfin apps show no button for them; reminders set in the web client reach the user wherever their targets are. Polyfin's own routes, under the Live TV permission: `GET /Polyfin/Reminders` lists the user's reminders and series reminders; `GET /Polyfin/Reminders/Programs/{programId}` tells how a programme stands (`Started`, `ReminderId`, `SeriesReminderId`, `Reaches`, `BeforeMinutes`), and answers 204 with nothing, without the permission too, for an item that is no programme of the user's guide or for a user without Live TV, as the web client's script asks it of every page; `POST /Polyfin/Reminders` with `ProgramId`, `Series` and `Language` sets one; `DELETE /Polyfin/Reminders/{id}` deletes a reminder or a series reminder.

## Recordings

Polyfin records Live TV programmes, as Jellyfin's DVR does, once **Record Live TV** is turned on, under **Settings › Recordings**. Recordings are written to **Recordings folder**: when it is empty, to `recordings` in Polyfin's data folder, `/data/recordings` in the Docker image's volume, which Polyfin creates.

- To keep them on another disk, mount a folder of the server at `/data/recordings`: with Compose, add `- ./recordings:/data/recordings` under `volumes` in the `polyfin` service of `compose.yaml`; on Unraid, set the template's **Recordings folder**. A folder mounted elsewhere in the container works too: enter its path in **Recordings folder**.
- Polyfin must be able to write to the folder, as the container's user, UID 65532: it checks it when the setting is saved, and refuses a folder it cannot write to. **System › Health** lists a folder it can no longer write to.
- Changing the folder does not move the recordings already made: they are found again once moved there.

While recording is off, Polyfin answers as a server that records nothing: the recording, timer and series timer lists stay empty, what they would hold is not found, and new timers are refused (400).

A recording that finished or failed can be sent to the targets of the user who scheduled it and to the server's: see [Notifications](notifications.md).

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

- A series timer records only on the days of the week it lists, in the server's time zone, where Jellyfin does not look at them. Listing none, or all of them, records every day.
- "New episodes only" leaves out the programmes the guide marks previously shown, and records those it marks new. A programme without either mark is left out when the series timer already recorded the same episode in full: told by its season and episode numbers, else its episode title, else its description, the first of these both have. A programme with none of them is recorded, and so is an episode recorded only in part, such as one whose stream failed for a while.
- The episodes a series timer recorded in full are kept with it, even once their recordings are deleted, and go with it. Once one is recorded, the timers the series timer made for the same episode's other airings go.
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
| **Find commercials in recordings** | **Settings › Recordings** | Off | Reads each recording once it ends, partial ones included, for its commercial breaks, which apps then offer to skip. See [Commercials of recordings](skip-segments.md#commercials-of-recordings). |
| **Convert finished recordings to HEVC** | **Settings › Recordings** | off | Converts finished recordings to HEVC at night, so that they take less space (see [Smaller recordings](#smaller-recordings)). |
| **Convert between** | **Settings › Recordings** | 1:00 to 6:00, in the server's time zone | The hours conversions run. A conversion still running at the end stops, and starts over the next night. |
| **Use the processor when there is no graphics card** | **Settings › Recordings** | off | Converts on the processor, with x265, when no graphics card encodes HEVC. Without a graphics card and with it off, nothing is converted. |

### How a recording is made

- FFmpeg copies the channel's stream, read as live playback reads it, with the headers it needs and within its addon's confinement, from the guide of the user who scheduled it.
- It counts among that user's 4 channels played through FFmpeg at once, and the server's 16. Past the user's 4, it takes the place of their oldest channel watched. A channel watched never takes the place of a recording.
- A stream that stops before the end is read again 30 seconds later.
- While it records, the file is written in MPEG-TS, which apps can follow live.
- Once the programme ends, it becomes one Matroska file in the folder. It plays like a movie's version: as it is, or remuxed into HLS (see [Playback](playback.md)).
- A recording under way when Polyfin stops is kept as a partial recording when Polyfin starts again. The rest of the programme, if it still airs, is recorded as another.

### Recordings and tuners

A recording reads its channel's stream from before FFmpeg starts until it ends, so the connection or tuner it takes stays its own between its analysis and FFmpeg's start.

- On an HDHomeRun tuner, a recording due takes, after the tuners no one uses any more, the tuner of the oldest channel only watched, whoever watches it: that channel stops for its viewers. It never takes another recording's tuner. A viewer's app that asks for the channel again gets `503` while the tuners are in use, as for a channel an IPTV account's connections refuse, and the log tells it at INFO, saying the tuners are in use.
- One that finds every tuner recording another channel is tried again every 30 seconds, as a stream that fails. If it recorded nothing by its programme's end, it fails, and the **Recording failed** notification says every tuner was recording another channel.
- On an IPTV account, recordings keep the account's rule: a recording takes a connection no one uses any more, never another user's channel, and is refused when they all play.
- The admin app's lists of timers, under **Content › Live TV**, mark with **Not enough tuners** the timers of a tuner's channel that would need more tuners than it has, counting the other timers recording at the same time.

### Commercials of recordings

With **Find commercials in recordings** on, a recording is read once it ends, in the background, one recording at a time and with FFmpeg at the lowest priority, for the moments that are both black and silent. The short pieces between them are its commercial breaks, which apps offer to skip while **Skip intro and credits buttons** is on, as each user chose in their app's playback settings.

- A recording longer than 4 hours is not read, and one whose reading takes longer than it lasts gets none.
- **Find commercials in recordings**, a task under **System › Schedule**, reads the recordings not read yet, such as those made before the setting was turned on. It runs every day, and when started by hand.
- What was found stays with the recording when its file is replaced, and is deleted with it.

How breaks are told apart from the programme, and how apps get them: see [Commercials of recordings](skip-segments.md#commercials-of-recordings).

### Smaller recordings

A recording copies the channel's stream at the channel's own bitrate. With **Convert finished recordings to HEVC** on, Polyfin converts finished recordings, partial ones included, to HEVC, which takes about half the space of H.264 and less still of MPEG-2:

- One recording at a time, the one that finished first, between the hours of **Convert between**.
- On the card **Graphics card used to convert** chooses, with the **HEVC quality** and the **Deinterlacing method** of the conversion settings (see [Transcoding](transcoding.md#conversion-settings)). With **HEVC quality** at 0, recordings use 28, Jellyfin's. Without a graphics card that encodes HEVC, recordings are converted on the processor with x265, at its lowest priority, only if **Use the processor when there is no graphics card** is on.
- The picture keeps its size, deinterlaced when it is interlaced, in 8 bits. The audio and subtitles are copied as they are. The copy's bitrate never goes above the recording's.
- A conversion counts as one of the **Video conversions at once**, and waits while playbacks fill them. Playbacks started meanwhile count it too.
- A conversion starts only with free space on the recordings' disk for a copy as large as the recording, and waits for it otherwise.
- A conversion still running at the end of the hours stops, its copy is deleted, and the recording starts over the next night, as it does when Polyfin stops.

Before it replaces the recording, the copy is checked: its length is within a second of the recording's, it holds the same tracks, in the same order, its video decodes to its end, and it is smaller. Otherwise the copy is deleted, the recording kept as it was, and it is not tried again. Recordings in HEVC, AV1 or VP9 already, those in HDR or in more than 8 bits, and those ffprobe cannot read, are kept as they are. A recording that cannot be read for a reason that says nothing of it is never kept, and is tried again:

- With the recordings' folder missing or unreadable, as when their disk is not mounted yet, or with ffprobe missing or out of time, conversions stop, and try again a minute later.
- A recording whose file alone cannot be opened is passed over, and the next recordings converted. Once none is left, conversions try it again a minute later.

A recording someone is playing is replaced once they stop. The recording keeps its identifier, so its played state, resume points and commercials stay. Its file becomes `<identifier>.mkv`, and what playback learned of the old file is learned again.

**Content › Live TV**, under **Recordings**, shows how many recordings were converted and the space saved.

Apps that do not decode HEVC get a converted recording converted when they play it, as any HEVC video (see [Transcoding](transcoding.md#when-polyfin-converts)). With **Conversion (transcoding)** off, they cannot play it.

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
