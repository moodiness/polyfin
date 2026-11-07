# Reading the sources

For administrators: preparing playback, reading and keeping addon and remote storage files, and failing sources.

## Preparing playback in advance

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Prepare playback in advance** | **Settings › Playback** | On | Does the reads a first play waits for before the user presses Play. |

Playback then starts at once, not after a second or two. Polyfin prepares:

- **A title opened in an app:** it analyzes the first 2 versions a play would read at once, again if a later addon answer puts another first.
- **Each prepared version:** its keyframe index and subtitle track positions, which HLS playback needs, and its first segment's bytes (see [Reading the sources](#reading-the-sources)).
- **The next episode:** with its versions and subtitles, once the episode playing has 9 minutes left.
- **The next track:** as a song or audiobook starts, the next in the app's queue (else the album): its stream, and its analysis if the addon does not describe it.
- **Continue Watching and Next Up:** in the background, the versions of each row's first 10 movies and episodes. Titles opened there list versions without a placeholder (see [Title pages](versions.md#title-pages)). The first version of each row's first 2 titles is analyzed, for instant resumes.

This costs a few more source requests, even for titles never played.

| Limit | Value |
|---|---|
| Preparations at once | 2; others wait newest first, so the title opened last goes first |
| Preparations waiting | 4; beyond that, the oldest is skipped |
| Same title and user | Once per 10 minutes, unless it failed |
| Row titles listed at once | 2 over the server, apart from preparations, in the order rows asked |

- A title waiting or being listed is not queued again. One whose versions are still kept (**Keep version lists for (minutes)**) asks nothing.
- All is background work: paced per addon, behind what users open, pausing after a rate refusal (see [How Polyfin paces addons](addons.md#how-polyfin-paces-addons)).
- Torrent versions are left out while no playback runs their torrent. Only a playback joins a swarm, and its first play analyzes it (see [How a torrent starts](torrents.md#how-a-torrent-starts)). Opening the page neither reads nor logs it.

## The source cache

FFmpeg, ffprobe and relayed apps read files through the source cache, which keeps fetched blocks on disk for seeks.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Disk space for files being read (GB)** | **Settings › Playback** | 10 GB | Cache size. A lower value applies at once. |
| **Folder for files being read** | **Settings › Playback** | The data folder's `cache` | Where blocks go, from the start after it is set (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)). |

| Files of | How Polyfin reads them |
|---|---|
| Addons, local folders | Through the cache |
| S3 buckets, rclone remotes | Straight from storage in blocks, over HTTP with ranges |
| SFTP folders | Straight from storage in blocks, over the folder's SSH connection |
| Folders in the container, SMB shares, WebDAV folders | By Polyfin at each request, through its loopback interface (see [network shares](network-shares.md#performance)) |

S3, SFTP and rclone files count against the same disk space, and the per-host limits apply to their storage.

### Connections per host

| Limit | Value |
|---|---|
| Per file | Up to 3 when the host serves each at its own pace: one for the current read, others for what follows |
| Per host, all files | 4 |
| Per host after `429` or `503` | 2, for 10 minutes |
| Per file after a slow-down (`429`, `503`), a refused connection, or no gain from several | 1, for an hour |

Playback reads never wait: repackaging or relay of the version playing, seeks, the chosen version's analysis and first segment, and subtitles read through a file's index.

Background reads wait for a free connection and yield theirs to playback: read-ahead for opened titles, keyframe indexes during analysis, versions analyzed beside the first, scrubbing images, and a file's 2nd and 3rd connections (not opened while the host has none free).

### How reads are served

- Each FFmpeg or app request is served on its own, newest first. A seek is not slowed by the request FFmpeg leaves; an ended request stops what it waited for.
- Read-ahead grows as playback goes on, up to 256 MiB (at most an eighth of the cache).
- While FFmpeg waits for the app, the connection stays open 30 seconds instead of reopening per segment.
- Torrent files are read ahead only during playback. When the last ends, connections close at once, except those serving a waiting read, so a stop leaves the swarm (see [Torrents](torrents.md#only-while-playing)).
- When a stream redirects, as addon resolvers do, Polyfin reads from the final address: connections skip the resolver, and the file cannot change mid-play.
- Apps still get the addon's address. A dead final address sends Polyfin back to the addon's, then to a renewed link.
- Once the keyframe index is known, Polyfin reads the first segment's bytes (64 MiB at most) before the app asks: from `PlaybackInfo`, and when details open with [Prepare playback in advance](#preparing-playback-in-advance) on.
- For a resume, it reads the file's head, which FFmpeg probes, and the resume segment. A Matroska index is read during analysis.
- Relayed files use the same cache: earlier reads cost no request; seeks fetch only what is missing.
- Before direct play, Polyfin checks the source answers, unless it answered Polyfin at that address within 2 minutes, as after an analysis.

### Keeping the files read across restarts

A title played again, or resumed after a restart or update, reads its kept parts from disk without asking its source.

**Disk space and retention**, under **Settings › Playback**, picks a preset for the fields below, including **Disk space for files being read (GB)**. Changing one makes it **Custom**. Its help shows the free space on the disk of the folder for files being read.

| Preset | Disk space | Kept for | Extra days for each play again | Next episodes preloaded |
|---|---|---|---|---|
| **Off** | 2 GB | Emptied at each start | 0 | Opening and end |
| **Small** (the default) | 10 GB | 3 days | 1 | Opening and end |
| **Everyday** | 50 GB | 14 days | 3 | Opening and end |
| **Large** | 250 GB | 60 days | 7 | Whole files |

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Keep parts read for (days)** | **Settings › Playback** | 3 (0 to 365) | Days a part is kept after its last read, restarts included. 0 empties the cache at each start. |
| **Extra days for each play again** | **Settings › Playback** | 1 (0 to 30) | Added for each play of a title after the first. |

- **Kept:** addon, S3, SFTP and rclone files, by version, whatever link the addon gives next.
- **Not kept:** torrent files, whose pieces are removed on stop by design (see [Torrents](torrents.md)), and files of unknown size, such as live video.
- **Changed links:** a file at the same link reads from disk at once after a restart. For a new link, Polyfin asks the size once: the same size reuses the parts; another size is another file, whose parts are removed (`A source's link now names a file of another size: the blocks kept of it were removed`, at Info). An answer that agrees with neither the parts kept nor the size the addon announced, nor the analysis, is refused instead, and the parts stay (see [Hosts answering an error file](#hosts-answering-an-error-file)).
- **Plays again:** a playback 6 hours or more after the last counts. Each, 8 at most, adds the extra days.
- **Removal order:** titles played once go before titles played again, whether the disk is full or days run out. When full, the oldest-read parts go first, extra days counted; parts read in the last 30 seconds stay. Expired parts go at start and hourly.
- **Crashes:** each file's record of parts is saved 30 seconds after a change, and at stop once parts reach disk. After a crash, the last 30 seconds of reads are fetched again; an unclaimed part is never served.
- **Health:** **System › Health › Source cache** shows space used, retention, files open and kept, and **Read from the disk**: the share of bytes FFmpeg, ffprobe and apps read since start that were read before or kept from before. Fetched bytes count as not from disk on first read, even if read ahead or preloaded, so a first play shows close to 0%.

API details: [Source cache in the admin API](jellyfin-api.md#source-cache-in-the-admin-api).

### Preloading the next episodes

Preloading reads ahead each user's next episodes, so they start from disk, even after a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Preload the next episodes** | **Settings › Playback** | Off | Turns preloading on. |
| **Episodes per series** | **Settings › Playback** | 1 (3 at most) | Episodes per series, from the one Next Up offers. |
| **Preload whole files** | **Settings › Playback** | Off (on with **Large**) | Whole files instead of opening and end. |
| **Speed limit (Mbit/s)** | **Settings › Playback** | 20 | Average speed. |
| **Free space to keep (GB)** | **Settings › Playback** | 20 | Runs only while the disk of the folder for files being read keeps this free. |

- **When:** every 15 minutes, from 2 minutes after start, while nothing plays in **Home › Now playing**.
- **Whose:** the first 5 Next Up series, latest first, of users who played or started an episode in 14 days.
- **Which version:** the one Play would pick, under the user's parental control, hidden titles, [version order](versions.md#version-order) and quality group. Never a torrent, or a version that failed lately.
- **How much:** the first 128 MiB and last 4 MiB (where many files keep their index), or the whole file. Per pass, at most a quarter of **Disk space for files being read (GB)**, so played parts stay.

A starting playback stops it within 8 MiB. Its reads are background reads, and are kept like any part, so repeating asks nothing.

**Compared with Jellyfin**

- Polyfin keeps and preloads addon and remote storage files, which a restart would otherwise download again. Jellyfin reads files in place, with no source cache (see [Configuration](configuration.md#folders-for-conversion-files-being-read-and-the-cache)).

### Sources that fail

| Case | What Polyfin does |
|---|---|
| No headers within 15 seconds | Fails. |
| Nothing sent for 10 seconds | Reopens; a host silent a few more seconds fails, so waiting segments error in seconds, not two minutes. |
| Slow-down (`429`, `503`), at open or mid-file | Retries up to 4 times, after `Retry-After` (5 seconds at most), else 0.5, 1, then 2 seconds. Reads wait. Then reads fail, and the file is retried 5 seconds later. |
| Wrong-size answer (error page, link to another file, `416` with another size) | Asks once more, renewing the link if the file was known, then fails. Its bytes are never served. An error file a host answers for every link is told apart (see [Hosts answering an error file](#hosts-answering-an-error-file)). |
| Range answered with the whole file (`200`) | Treats ranges as ignored; reads from the start to the part asked. |
| Same, from a host that served a range before | Asks again up to 3 times (4 requests in all), 0.5, 1, then 2 seconds apart: such hosts do this now and then, and reading through would hold a seek for minutes. |
| Announced size off by over 1%, once the addon's sizes proved right | Another file: fails at once. Within 1%, the real size is used. Addons wrong more often than right are not checked. |
| Source fails during analysis or playback | Skips the version for 15 minutes; analysis stops at once and the next version is tried within seconds. |
| Answers but not with video | Fails analysis the same way. An empty answer fails at once. |
| Slow-down during analysis, keyframe index, playback or direct-play check, or an analysis timing out on it | Never skips; retried next play. |
| Keyframe index not served | Retried at the next play. |
| Link renewal fails | Not the file's failure. |

- The log tells what the source answered, never its address.
- A torrent that did not start in time stays listed; only plays that pick no version skip it (see [When torrents do not start](torrents.md#when-torrents-do-not-start)).

`A version could not be analyzed` logs the status, header type and size, and what the first bytes are: a web page, JSON, XML, text, a playlist, a playlist not starting with `#EXTM3U`, or an empty body. Example: `not a playable video: the source answered a web page (HTTP 200, text/html, 2.1 kB)`. With an error status: `source unavailable: HTTP 403, a web page (text/html, 2.1 kB)`. Neither shows the address or bytes.

A page or error usually means the site wants a header the addon does not declare (`proxyHeaders`), such as a player's user agent. Without one, Polyfin sends `Polyfin/<version>`.

API details: [First segment reads for apps](jellyfin-api.md#first-segment-reads-for-apps).

### Hosts answering an error file

A host whose service is down may answer every link with the same short video, an error file, in place of the files asked. Polyfin tells it apart from a link that names another file.

- **Found:** one host answers the same size, 50 MB at most, for different files, each more than ten times that size, as their analysis, addon or kept parts tell. From two such files, that size is the host's error file: it is refused at once, whatever the addon's sizes are worth. A real episode is never that small: versions that all answer the same episode under a pack's announced size are no error file.
- **Told:** from three files within 10 minutes, `A host answers an error file instead of the files asked` is logged at Warn, **System › Health** names the host, never the link, and since when, and **Content › Versions** shows it on the versions from that host (see [Host answering an error file](health.md#host-answering-an-error-file)). A host asked nothing for 6 hours is no longer told: nothing tells whether it recovered.
- **Analyzed files:** a version analyzed before reads its file with the size its analysis found, when its addon announced a size within 1% of it. An answer more than 1% off is refused at its first byte, before the host is found answering an error file.
- **What stays:** an error file never removes the parts kept of a file, nor makes the sizes an addon announces untrusted.
- **Recovery:** as soon as the host answers a file of its own size, the problem goes, and the versions refused meanwhile are tried again at once, not after their 15 minutes (`A host answers its files again`, at Info). A file that took the error file for its own, before the host was found answering one, is read anew.
- **Analysis of another file:** an analysis the addon's size does not side with, such as one made of an error page, never refuses the file: the file is read as before, and the analysis's size is corrected from it.

### Carrying a play on

A source failing for good mid-play, past the retries above, need not end it.

#### Mirrors

The cache reads on from the version's next mirror, the same file another addon lists (see [Which streams become versions](versions.md#which-streams-become-versions), step 5), from the byte where it stopped.

- Blocks already read stay. A wrong-size answer is no mirror and fails in turn.
- Relayed apps and HLS plays go on without error. The log says `A source failed: reading on from another copy of the file`, at Info.
- The version is marked failed only when its last mirror fails.
- A relayed file with mirrors always goes through the cache, which alone can switch.

#### Another version over HLS

When a version and its mirrors fail, a repackaged or converted HLS play continues from the next version, at the segment the app awaits.

- **Order:** later versions as the user is offered them ([version order](versions.md#version-order) and quality group included), then earlier ones.
- **Playlist:** segments keep numbers and times, with one `#EXT-X-DISCONTINUITY` before the new version's first segment.
- **Video:** converted to the codec, size and frame rate the app received.
- **Audio:** the first track in the same language, copied when alike, else converted to the codec, channels and sample rate the app received.
- **Timing:** shifted by the speech offset when both versions' speech is known (see [Subtitles](subtitles.md)).
- **Signs:** the log says `A play went on from another version: its source failed`; Home's playback **Details** show **Switched version**.

In fragmented MP4 segments, the app keeps its first initialization segment and never reloads the playlist:

- The new video uses that segment's timescale and track order.
- If the first video was copied, it is H.264 High, Main or Baseline at the same size and frame rate. The conversion repeats parameter sets before each keyframe, replacing the copy's in the decoder.
- Converted audio is AAC-LC only, the encoder's sole profile.

#### Limits of the switch

| Limit | Value |
|---|---|
| Search for another version | 40 seconds at most; app requests wait |
| App requests waiting over 20 seconds | Told the server is busy; apps ask again |
| Switches per play | 3 |
| Video conversion | Counts toward [Video conversions at once](transcoding.md#video-conversions-at-once); at the cap, no switch |

Versions skipped:

- torrents, whose swarm starts slower than players wait;
- versions that failed lately;
- MPEG-TS, MPEG program streams, AVI and discs;
- versions over 1% longer or shorter, or of another frame shape (2%): another cut;
- versions whose tracks cannot be converted to the app's.

Plays that never switch, and end when the source fails: copied HDR or 10-bit video; fragmented MP4 with copied HEVC, or H.264 of another profile or odd size; MPEG-TS, MPEG program streams, AVI and discs; Live TV; and **Keep the converted video when the audio track changes** on.

At a switch, the first file's picture subtitles are not burned in, and its embedded text subtitles stop.

#### Plays that cannot be carried on

- Direct play: the app reads the provider's link itself and never asks Polyfin again.
- A relayed file without mirrors.

**Compared with Jellyfin**

- Polyfin carries a play on through mirrors and other versions; in Jellyfin a failed source ends it. Apps need no change: they keep requesting the same session's segments.
- Polyfin keeps VOD media playlists, never reloaded, like Jellyfin ([DynamicHlsPlaylistGenerator.cs, line 58](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs#L58)). An `EVENT` playlist, reloaded to find a new initialization segment, would make every play live in apps, with another start, duration and seeking.
