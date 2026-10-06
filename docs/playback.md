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

### Title pages

A title's page opens as soon as its description is ready, without waiting for the stream addons, some of which are slow. It lists the versions Polyfin already knows: those of the addons that answered for the title within **Keep version lists for (minutes)**, and those of IPTV sources. Polyfin asks the other addons in the background meanwhile.

Some addons gather other addons' streams. They answer the first request for a title with the streams that came in within their own time limit, and get the others moments later. So Polyfin asks an addon again 10 seconds after its first answer for a title, and once more 30 seconds later if that answer listed more playable streams:

- Polyfin keeps the longer list, from then on for **Keep version lists for (minutes)**. It never replaces a list with a shorter one.
- It stops at the first answer that lists no more streams, at an error, or once the list is dropped, by [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) or at the end of **Keep version lists for (minutes)**. A late answer never brings back a dropped list.
- It asks again only when it asked the addon itself. A list it already kept is used as it is. IPTV sources are never asked again: their streams are Polyfin's own.
- Subtitles from the addons are asked again the same way, and the longer list kept.
- Opening a title and playing never wait for these requests.

In apps:

- In the web player, the versions appear in the page's version menu as each addon answers, then as an addon asked again lists more. A version already picked stays picked.
- Other Jellyfin apps cannot be told to refresh the page. They show the versions known when it opened, and get every version when the user presses Play, or when the page is opened again.
- Until a version is known, the title still shows as playable. Play waits for the addons still answering for the first time, then picks among all the versions, as before.
- An addon that fails, or does not answer within 15 seconds, only leaves its versions out.
- Subtitles from the addons follow the same way: those known show at once, and the others with Play or the next opening.

**Compared with Jellyfin:**

- Jellyfin knows every version of its titles beforehand, so its pages list them all at once.

**For app developers:**

- Item details (`/Items/{id}`, `/Users/{userId}/Items/{id}`) describe the versions known when asked. With none known, they describe one placeholder source under the title's own identifier, which plays the first version. `PlaybackInfo` waits for every addon asked for the first time, joining the requests the details started, but never for an addon asked again.
- `GET /Polyfin/Items/{id}/Versions` answers `{"Pending": <addons still asked for the title, for the first time or again>, "Count": <media sources the details would list now>}`, with the authentication and access checks of item details. An addon to be asked again counts from its first answer until it is no longer asked. `Count` includes the placeholder. Items other than movies and episodes answer `0` for both. Polyfin's web player script polls it every second while `Pending` is above `0`, for at most 90 seconds, and reloads the page when `Count` is above what the version menu lists. The 90 seconds cover a first answer (at most 15 seconds) and both requests that follow it (10 and 30 seconds later, at most 15 seconds each).

## Chapters

The analysis also reads the version's chapters, which apps show and let you skip through. Apps get the chapters of the version the title was opened or played as. A version never played has no chapters yet. Chapters named as an intro or as credits also give that version's skip buttons (see [Segments of each version](skip-segments.md#segments-of-each-version)).

Chapters come without chapter images unless you turn those on (see [Scrubbing thumbnails and chapter images](#scrubbing-thumbnails-and-chapter-images)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Show chapters** | **Settings › Playback** | On | Sends apps the chapters of the versions Polyfin analyzed. Chapters are read along with the analysis every first play needs, so they never delay playback. Turning this off only hides them from apps. |

## Preparing playback in advance

The first play of a version waits for some reads. **Prepare playback in advance** does them as soon as the title's details open in an app:

- it analyzes the version the title would play;
- it then reads the version's keyframe index and where its subtitle tracks sit, which HLS playback needs;
- it readies the next episode the same way, with its versions and subtitles, once the episode playing has 9 minutes left.

Playback then starts at once instead of waiting a second or two for these reads.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Prepare playback in advance** | **Settings › Playback** | Off | Analyzes and reads ahead as described above. |

It costs a few more requests to the sources, also for titles opened but not played. Limits:

- at most 2 preparations run at once;
- at most 6 start per user each minute; beyond that they are skipped;
- a title is prepared once per user every 10 minutes.

## Choosing a version

Five more settings change how Polyfin picks a version and how much video the server converts. Their defaults keep Polyfin's earlier behavior. A change applies to the next request, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Maximum time to analyze a version** | **Settings › Playback** | 45 seconds (5 to 120) | Bounds every ffprobe analysis, of files and of live streams. A source that does not answer in time is given up for 15 minutes, and Polyfin moves on to the next version sooner. |
| **Versions tried when one does not work** | **Settings › Playback** | 3 (1 to 10) | How many versions Polyfin analyzes when the app picked none, for titles and channels alike. Later versions analyzed before are still tried, as they cost nothing. |
| **Prefer versions the app plays without conversion** | **Settings › Playback** | Off | Goes on through those versions until one plays on the app as it is or remuxed with its tracks copied. If none does, falls back to the first that plays at all. |
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

## Scrubbing thumbnails and chapter images

**Settings › Thumbnails** makes Jellyfin's scrubbing thumbnails (trickplay) and chapter images. Both are off by default, because both read parts of the source.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Thumbnails when moving through a title** | **Settings › Thumbnails** | Off | Makes trickplay thumbnails. |
| **Chapter images** | **Settings › Thumbnails** | Off | Makes chapter images. |
| **One thumbnail every (seconds)** | **Settings › Thumbnails** | 10 seconds (5 to 60) | Time between thumbnails. |
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

- The keyframes read are spread evenly over the runtime. Each thumbnail shows the keyframe read nearest its time, so long movies get coarser thumbnails, a few minutes apart rather than a few seconds.
- A thumbnail may therefore show a moment a few seconds, or on long movies a few minutes, from its time.
- Chapter images share the same reads: the keyframe read nearest each chapter's start.
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
