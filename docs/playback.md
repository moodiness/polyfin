# Playback

This page explains how Polyfin plays a title in Jellyfin apps: versions, chapters, preparing playback in advance, the settings that choose a version, watching together, and scrubbing thumbnails.

## How a title plays

Polyfin always handles sign-in, accounts, browsing, metadata, source selection and playback state. Apps never see the addons' stream URLs: they receive Polyfin's own, signed for the user.

A version reaches the app in one of three ways:

- **Direct play.** The app plays the version as it is. Polyfin sends it to the provider's URL, so the stream does not pass through the server.
- **Relay.** Polyfin reads the stream and passes it on when the app could not reach the source itself: a source that needs request headers, one on a local network address, or a redirect the app could not follow.
- **HLS.** FFmpeg reads the stream through Polyfin's cache and Polyfin serves it as HLS segments, remuxed or transcoded. See [Transcoding](transcoding.md).

Whether a version plays directly or is converted decides how much bandwidth the server uses.

**For app developers:**

- Direct play answers with a `302` redirect to the debrid or provider URL.

## Versions and analysis

A title's details list every stream the addons offer as a version (see [Title pages](#title-pages) for when). The first time a version is played, Polyfin analyzes it with ffprobe, which takes a few seconds, and keeps the result. It then tells the app whether the app can play the version as it is, as a Jellyfin server would.

- Versions always keep the addons' order. Picking or playing one never moves it.
- When the app has not picked a version, a version that cannot be read, or does not play on the app, is skipped in favor of the next one. It is also left out of the versions Polyfin lists, which keep their order starting from the one chosen, for apps that let the user pick at that point.
- A version that would play over HLS but whose keyframe index cannot be read is skipped the same way, as nothing could stream it.
- The first versions are analyzed together, up to 3 at once, and taken in order. Once a later one is ready, Polyfin waits 3 more seconds for those before it, then plays the later one: a source that never answers delays playback by those 3 seconds rather than by the whole analysis time. The analyses still running go on in the background and are kept for the next play.
- The title's page plays the version the app sends. Most apps (the web player, Android TV, Swiftfin) send the title's own identifier, which names the first version: Polyfin then chooses among all the versions as when the app picks none. Only a version picked in the version menu, under its own identifier, is played alone.
- Apps can save the user's playback preferences (audio and subtitle languages, subtitle mode). These choose the default audio and subtitle tracks. See [Subtitles](subtitles.md).
- Apps that keep titles for offline viewing can download the movies and episodes whose versions they were shown. A download is the version's file, under its file name, sent to the source or relayed as playback would be.
- Active playback appears in Jellyfin apps' dashboards.
- Apps can control one another (play, pause, seek, messages) when the controlled app keeps Jellyfin's live connection (WebSocket) open.
- The skip intro, recap and credits buttons come from [TheIntroDB](https://theintrodb.org) and [IntroDB](https://introdb.app), community databases. Polyfin sends them only titles' IDs, episode numbers and runtimes. See [Skip segments](skip-segments.md).

**Compared with Jellyfin:**

- Jellyfin 12.1 lists first the version an app opens as an item. jellyfin-web's version menu does this once a version is picked, so the menu reorders itself there. Polyfin keeps the menu as it was.
- Jellyfin lists all versions in PlaybackInfo, including the ones that cannot be read or played.

**For app developers:**

- Unreadable or unplayable versions are left out of the versions `PlaybackInfo` lists only when the app has not picked a version.
- A `MediaSourceId` equal to the item's own identifier picks no version: `PlaybackInfo` tries the versions in order, as without one, and answers with the version chosen alone, under its own identifier unless it is the first. Any other `MediaSourceId` plays that version or nothing.
- `StartTimeTicks` in the query or the body tells where an HLS play starts, so that Polyfin reads ahead from there.

### Title pages

A title's page opens as soon as its description is ready, without waiting for the stream addons, some of which are slow. It lists the versions Polyfin already knows: those of the addons that answered for the title within **Keep version lists for (minutes)**, and those of IPTV sources. Polyfin asks the other addons in the background meanwhile.

A title opened again once **Keep version lists for (minutes)** has passed shows the versions known before at once, even old ones, while Polyfin asks the addons again. For this, Polyfin keeps an addon's list for 24 hours after that time, also across restarts: the last list of each addon for each title is saved in the database. Listings may show these versions too.

- Play does not wait for the addon's new answer: it plays the old versions, asking the addon again in the background. Expired links are renewed when the source refuses them. Only when none of the old versions plays does Play wait for the new answer, then choose among its versions.
- After a restart, a title shows the versions saved before at once, as old versions, while Polyfin asks the addons again.
- The new answer replaces the old list. The versions it lacks stay listed after its own until Polyfin stops asking the addon again (see below): an addon that gathers other addons' streams often answers first with only part of them. They still play meanwhile.
- Once Polyfin stops asking, the list is the addon's last answer alone: the versions the addon no longer lists disappear.
- [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) drops the old lists too.
- Subtitles from the addons are kept the same way.

Some addons gather other addons' streams. They answer the first request for a title with the streams that came in within their own time limit, and get the others moments later. So Polyfin asks an addon again 10 seconds after its first answer for a title, and once more 30 seconds later if that answer listed more playable streams:

- Polyfin keeps the longer list, from then on for **Keep version lists for (minutes)**. It never replaces a list with a shorter one. Each answer is compared with the addon's previous answer, not with the old versions still listed after it.
- It stops at the first answer that lists no more streams, at an error, or once the list is dropped, by [Refresh metadata](jellyfin-compatibility.md#refresh-metadata), or has expired, at the end of **Keep version lists for (minutes)**. A late answer never brings back a dropped list, nor makes an expired one current again.
- It asks again only when it asked the addon itself. A list it already kept is used as it is. IPTV sources are never asked again: their streams are Polyfin's own.
- Subtitles from the addons are asked again the same way, and the longer list kept.
- Opening a title and playing never wait for these requests.
- Renewing an expired link asks the addon as opening the title does, joining a request already under way, and asks again in the same way. When the answer lacks the version's file, Polyfin asks the addon again at once and waits for that answer, 10 seconds at most, before giving the link up. Meanwhile the title keeps listing every version.

In apps:

- In the web player, the versions appear in the page's version menu as each addon answers, then as an addon asked again lists more. They are added in place: the page is not reloaded, and a version already picked stays picked with its audio and subtitle choices. The page reloads its details only when the version picked is gone or has changed, for instance when the placeholder gives way to the first version. Nothing changes while a video plays or while the version menu has focus, since it may be open: the versions come once it loses focus.
- Other Jellyfin apps cannot be told to refresh the page. They show the versions known when it opened, and get every version when the user presses Play, or when the page is opened again.
- With [Prepare playback in advance](#preparing-playback-in-advance) on, Polyfin lists the versions of the titles of Continue Watching and Next Up as soon as an app asks for these rows. Once they are listed, every app opens those titles with all their versions.
- In the web player, a movie's or an episode's Play, Resume and Play from the start buttons wait for a version. While none is known and addons are still asked, they are greyed and disabled, with a small spinner and the tooltip "Looking for sources…". They come back as soon as the first version is known, without reloading the page. The page asks Polyfin as soon as it opens; until Polyfin answers, the buttons wait already when the page lists only the placeholder, as it does while no version is known.
- When every addon has answered without a version, the buttons stay disabled and a line under them says "No source is available for this title.", with a **Try again** button. It has Polyfin drop the title's version lists for the user's addons and ask them again, and the buttons wait again meanwhile. A user can ask again once every 20 seconds for a title. The words follow the web player's language (English or French, English otherwise). Nothing changes during a video, and leaving the page gives the buttons back as they were.
- In other apps, until a version is known, the title still shows as playable. Play waits for the addons still answering for the first time, then picks among all the versions, as before.
- An addon that fails, or does not answer within 15 seconds, only leaves its versions out.
- Subtitles from the addons follow the same way: those known show at once, and the others with Play or the next opening. Play waits for them one second at most once the versions are in: later ones come with the next play or track change.

**Compared with Jellyfin:**

- Jellyfin knows every version of its titles beforehand, so its pages list them all at once.

**For app developers:**

- Item details (`/Items/{id}`, `/Users/{userId}/Items/{id}`) describe the versions known when asked, an expired list's included. With none known, they describe one placeholder source under the title's own identifier, which plays the first version. `PlaybackInfo` waits for every addon asked for the first time, or asked again because its list expired, joining the requests the details started, but never for an addon asked again after an answer.
- `GET /Polyfin/Items/{id}/Versions` answers `{"Pending": <addons still asked for the title, for the first time or again>, "Count": <media sources the details would list now>, "Known": <versions known now>}`, with the authentication and access checks of item details. An addon to be asked again counts from its first answer until it is no longer asked. `Count` includes the placeholder; `Known` leaves it out, so it is `0` until a version is known, old versions of an expired list included. Both drop when the versions an addon's new answer lacked are no longer listed. Items other than movies and episodes answer `0` for all three. Polyfin also pushes it on the user's live connection (WebSocket) as it changes, for 2 minutes after the details or this endpoint were last asked, in a `PolyfinVersions` message whose `Data` is `{"ItemId": <the identifier the page was opened with>, "Pending": …, "Count": …, "Known": …}`. Polyfin's web player script listens to it, and polls at once when a title's page opens, then every second while `Pending` is above `0`, every 10 seconds once a push came, for at most 90 seconds. The 90 seconds cover a first answer (at most 15 seconds) and both requests that follow it (10 and 30 seconds later, at most 15 seconds each). When `Count` is above what the version menu lists, or differs from it once `Pending` is `0`, the script asks for the item's details (`/Users/{userId}/Items/{id}`) and lists their media sources in the menu. It holds the play buttons while `Known` is `0`, searching while `Pending` is above `0`, with no source once it is `0`. Its first answer may come before the details asked any addon, so it takes no source from it, only from the next. Before any answer, it holds them as searching when the version menu lists a single source whose identifier is the title's own, the placeholder.
- `POST /Polyfin/Items/{id}/Versions/Search` asks the user's stream addons again for a movie or an episode, as [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) does for its streams only: each addon's stream list for the title is dropped, the saved one included, its follow-ups stop, and the addons are asked in the background, as item details ask them. Addons still asked for the title are left to answer. It has the checks of `GET /Polyfin/Items/{id}/Versions` and answers the same progress, `Pending` counting the addons now asked. A user may ask once every 20 seconds for a title; sooner, it answers `429 Too Many Requests` with `Retry-After` in seconds. Items other than movies and episodes ask nothing and answer `0` for all three.
- With **Prepare playback in advance** on, `GET /UserItems/Resume`, `GET /Users/{userId}/Items/Resume` and `GET /Shows/NextUp` queue the first 10 movies and episodes of their answer before answering, and never wait for them. Each queued title is listed waiting for every addon and joining any request already under way. The first 2 then have their first version analyzed.

## Chapters

The analysis also reads the version's chapters, which apps show and let you skip through. Apps get the chapters of the version the title was opened or played as. A version never played has no chapters yet. Chapters named as an intro or as credits also give that version's skip buttons (see [Segments of each version](skip-segments.md#segments-of-each-version)).

Chapters come without chapter images unless you turn those on (see [Scrubbing thumbnails and chapter images](#scrubbing-thumbnails-and-chapter-images)). Chapters are always sent: they are read along with the analysis every first play needs, so they never delay playback.

## Preparing playback in advance

The first play of a version waits for some reads. **Prepare playback in advance** does them before the user presses Play:

- as soon as a title's details open in an app, it analyzes the first 2 versions the title would play, those a play reads at once, and again when an addon answering later puts another version first;
- it then reads each version's keyframe index and where its subtitle tracks sit, which HLS playback needs, and the bytes of its first segment (see [Reading the sources](#reading-the-sources));
- it readies the next episode the same way, with its versions and subtitles, once the episode playing has 9 minutes left;
- as a song or audiobook starts, it resolves the next track of the app's queue (else of the album) the same way: where it streams from, and its analysis when its addon does not describe it;
- when an app asks for Continue Watching or Next Up, it asks the addons, in the background, for the versions of the first 10 movies and episodes of each row. A title opened from these rows then lists its versions at once, instead of a placeholder while the addons answer (see [Title pages](#title-pages)). The first version of the first 2 titles of each row is analyzed too, so that resuming from these rows starts at once.

Playback then starts at once instead of waiting a second or two for these reads.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Prepare playback in advance** | **Settings › Playback** | On | Analyzes and reads ahead as described above. |

It costs a few more requests to the sources, also for titles opened but not played. Limits:

- at most 2 preparations run at once; the others wait, the newest first, so that the title opened last is prepared first. Beyond 4 waiting, the oldest is skipped;
- a title is prepared once per user every 10 minutes, unless its preparation failed;
- the titles of Continue Watching and Next Up are listed 2 at a time over the server, apart from the preparations above, in the order the rows asked for them. A title waiting or being listed is not queued again, and a title whose versions are still kept (**Keep version lists for (minutes)**) asks nothing.

## Choosing a version

Five more settings change how Polyfin picks a version and how much video the server converts. Their defaults keep Polyfin's earlier behavior. A change applies to the next request, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Maximum time to analyze a version** | **Settings › Playback** | 20 seconds (5 to 120) | Bounds every ffprobe analysis, of files and of live streams; a channel is analyzed for 8 seconds at most. A source that does not answer in time is given up for 15 minutes, and Polyfin moves on to the next version sooner. |
| **Versions tried when one does not work** | **Settings › Playback** | 3 (1 to 10) | How many versions Polyfin analyzes when the app picked none, for titles and channels alike, up to 3 at once for titles. Later versions analyzed before are still tried, as they cost nothing. |
| **Prefer versions the app plays without conversion** | **Settings › Playback** | Off | Looks among the versions analyzed at once for one that plays on the app as it is or remuxed with its tracks copied. If none does, falls back to the first that plays at all. |
| **Video conversions at once (0 = no limit)** | **Settings › Conversion** | 0, no limit (up to 32) | Caps the playbacks whose video the server converts. |
| **Maximum quality of converted video** | **Settings › Conversion** | Original (or 480p to 2160p) | Scales converted video down to that height. |

### Prefer versions the app plays without conversion

- The first play of a title may analyze more versions. Nothing changes once they are known.
- A version above the user's **Maximum quality** never counts as one that plays without conversion.

**Compared with Jellyfin:**

- Jellyfin 12.1 keeps the version opened first however it plays, as Polyfin does with this setting off.

### Video conversions at once

- The cap counts conversions of files and Live TV together, burning subtitles in included.
- Live TV keeps its own caps on top: 4 channels per user and 16 for the server. See [Live TV](live-tv.md).
- A playback is what a user plays of a version, from its first converted segment until the app stops it or asks for nothing for 3 minutes (1 minute for Live TV). Its next segments, its seeks, and the new play session apps open to switch tracks never count again.
- At the cap, Polyfin plans no new video conversion, as when conversion is off. It picks the next version that plays without one, or else reports that no version is compatible.

**Compared with Jellyfin:**

- Jellyfin has no such cap. Its nearest, a tuner's stream limit, fails with a `500`, which apps take the same way, as a server error.

**For app developers:**

- At the cap, `PlaybackInfo` answers `NoCompatibleStream` when no version plays without conversion.
- An HLS request that would start one more conversion answers `503`, as when Live TV is at its cap.

### Maximum quality of converted video

- Video keeps its shape and is converted at the bitrate of that height. It fits that height's 16:9 frame: a 2.40:1 picture converted to 1080p is 1920×800, as in a 1080p release.
- A running conversion keeps the size it started with.
- Files played as they are, or with their tracks copied, are untouched.
- A GPU converts up to 4K (2160p), at the source's own size when nothing lower is set: a 3832×1600 picture stays 3832×1600. The processor converts to 1080p at most, so 1440p and 2160p change nothing there.

See [Transcoding](transcoding.md) for how conversion works.

## Watching together

Apps that offer SyncPlay, jellyfin-web first, play the same titles in step across devices and users.

- Anyone in a group can set or change what plays (up to 5,000 entries), play, pause and seek.
- The group starts and resumes on Polyfin's clock, a little ahead so that every app gets there in time.
- The group waits for apps still loading or buffering, for 30 seconds at most.
- Every user may create and join groups. A group is listed to, and can be joined by, only the users who can see everything it plays. Nothing a member cannot see is queued.
- A session leaves its group when its app disconnects or is signed out (by the app, a password change, or an administrator). A group ends once empty.
- The apps need Jellyfin's live connection (WebSocket) open.

**Compared with Jellyfin:**

- Like Jellyfin, Polyfin keeps the groups in memory.

## Reading the sources

FFmpeg, ffprobe and relayed apps read a version's file through Polyfin's source cache (`POLYFIN_CACHE_SIZE`). It fetches the file from the source in blocks and keeps them on disk for seeks.

- A file is read over up to 3 connections when its host serves each one at its own pace: one serves what is being read, the others the stretches read next. A host that asks to slow down (`429`, `503`), refuses one more connection, or serves several no faster than one, is read over one connection per file for an hour.
- Each request of FFmpeg or of an app is served on its own, the newest first. A seek is not slowed down by the request FFmpeg is leaving, and a request that ends stops what it waited for.
- Polyfin reads further ahead of a playback as it goes on, up to 256 MiB (at most an eighth of the cache). While FFmpeg waits for the app, the connection stays open for 30 seconds instead of being opened again for each segment.
- When a stream's address redirects, as addons' resolvers do, Polyfin keeps the address it led to for its own reads of the file. Each connection then skips the resolver, and the file cannot change under a playback. Apps sent to the source still get the addon's address. An address that stops working sends Polyfin back to the addon's, then to a renewed link.
- Once a version's keyframe index is known, Polyfin reads the bytes of its first segment before the app asks for them, 64 MiB at most: from `PlaybackInfo`, and when a title's details open with [Prepare playback in advance](#preparing-playback-in-advance) on. For a resume, it reads the file's head, which FFmpeg probes, and the segment the playback resumes at. The index of a Matroska file is read during its analysis.
- A file relayed to an app is served from the same cache: what was read before takes no request, and a seek reads only what is missing.
- Before sending an app to a source for direct play, Polyfin checks that it answers, unless it answered Polyfin's own reads at that address within 2 minutes, as the analysis just did.

### Sources that fail

- A source that sends no headers within 15 seconds fails.
- A connection that sends nothing for 10 seconds is opened again. A host that then does not answer within a few seconds fails, and the segments waiting for it get an error within seconds instead of two minutes.
- An answer of another size than the file, such as an error page or a link now naming another file, is asked once more, after renewing the link when the file was known, then fails. Its bytes are never served as the file's. A `416` that tells another size counts the same.
- Once an addon's file sizes proved right, the size it announces for a file is checked at the first answer: another file fails at once. Rounded or wrong sizes are never used.
- The analysis of a version whose source fails stops at once, and the version is skipped for 15 minutes: the next one is tried within seconds. A version whose source fails while it plays is skipped the same way. The log tells what the source answered, never its address.
- A host that failed to serve a version's keyframe index is asked again at the next play. A failed renewal of a link never counts as the file's failure.

**For app developers:**

- `PlaybackInfo` for a version played over HLS starts reading the bytes of its first segment; with `StartTimeTicks`, those of the segment the app resumes at.

## Scrubbing thumbnails and chapter images

**Settings › Thumbnails** makes Jellyfin's scrubbing thumbnails (trickplay) and chapter images. Both are off by default, because both read parts of the source.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Thumbnails when moving through a title** | **Settings › Thumbnails** | Off | Makes trickplay thumbnails. |
| **Chapter images** | **Settings › Thumbnails** | Off | Makes chapter images. |
| **One thumbnail every (seconds, at least)** | **Settings › Thumbnails** | 10 seconds (5 to 60) | Shortest time between thumbnails. A version longer than the requests allow at that pace gets longer steps (see [How the images look](#how-the-images-look)). |
| **Thumbnail width** | **Settings › Thumbnails** | 320 (or 240, 480) | Thumbnail width in pixels. |
| **Space for images (GB)** | **Settings › Thumbnails** | 2 GB (1 to 50) | Past this, the versions whose images were used longest ago lose them, and get them again on their next play. |

Turning either setting off hides what was made. It shows again when you turn the setting back on.

### When images are made

Images are made after watching, at a gentle pace:

- once a version has been played, and for the next episode when [playback is prepared in advance](#preparing-playback-in-advance);
- Polyfin waits until that playback has stopped and nobody plays anything from the same host;
- it then reads in the background only keyframes, located through the file's index (Matroska Cues or MP4 sample tables), each with a request for its own bytes, never the whole file;
- one version is made at a time, with a short queue.

### Request limits

- A version takes at most 60 requests, its index included.
- Requests are sent one every 3 seconds, and at most 120 an hour per host, across versions. Past that hour's share, they wait.

### How the images look

- Each thumbnail gets a keyframe of its own: a version whose thumbnails would outnumber the requests left once its index is read gets its thumbnails spread evenly over its runtime, one per request, a whole number of seconds apart. With about 58 requests left, that is every 47 seconds for a 45-minute episode, and every 2 minutes 5 seconds for a 2-hour movie.
- Each thumbnail shows the keyframe nearest its time, a few seconds from it at most, unless the file has keyframes further apart.
- Chapter images show the keyframe nearest each chapter's start: one read for it when the requests left allow one per thumbnail and per chapter, otherwise the nearest of the thumbnails' reads.
- FFmpeg decodes each keyframe on its own as it arrives, tied to its time whatever the order it was read in, on the GPU when there is one, and tone maps HDR.
- Thumbnails are packed into Jellyfin's 10x10 tile JPEGs. Chapter images are at most 640 pixels wide.
- Images made by Polyfin 0.7.0, which could show their keyframes out of order, are dropped on upgrade and made again on the next play.
- Images are kept in the database.

### When a source misbehaves

- A request answered oddly, as some hosts do now and then (the whole file for a range, another range, an answer cut short, or none), is asked once more after a short pause. The retry counts in the budget.
- A link the source says expired is renewed, once per version, then asked again.
- Two odd answers in a row fail the version for 30 minutes. The log tells what the source answered, never its address.
- A host that asks to slow down (`429`) or fails as overloaded (`502`, `503`, `504`) is not asked again: the version stops at once, nothing of it is kept, and the version waits a day. That host gets no image request for 2 hours, while other hosts go on.
- A version that fails otherwise is left alone for 30 minutes, or a day when its file cannot give images (no index, an unsupported codec).

**Compared with Jellyfin:**

- Thumbnails are of the keyframes read, so their timing is approximate (see above).
- Chapters carry no `ImagePath`.
- Turning either setting off hides what was made, which shows again when turned back on.

**For app developers:**

- Apps get images as Jellyfin gives them: the item's `Trickplay` field, by media source and width.
- `Trickplay` is keyed as the item's `MediaSources` name the versions (the first under the title's own identifier), and limited to the versions the user is offered under their quality group.
- Tiles are served at `/Videos/{itemId}/Trickplay/{width}/tiles.m3u8` and `{index}.jpg`, which follow parental control, blocked genres and allowed hours.
- Chapters carry an `ImageTag`, served by `/Items/{itemId}/Images/Chapter/{index}` without credentials, like all images.
