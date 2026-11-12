<a id="scrubbing-thumbnails-and-chapter-images"></a>

# Scrubbing thumbnails

For administrators: how Polyfin makes the scrubbing thumbnails and chapter images Jellyfin apps show, when it reads sources, and its limits.

## Thumbnail settings

**Settings › Thumbnails (trickplay)** makes Jellyfin's scrubbing thumbnails and chapter images. Both are off by default, because both read parts of the source.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Thumbnails when moving through a title** | **Settings › Thumbnails (trickplay)** | Off | Trickplay thumbnails. |
| **Chapter images** | **Settings › Thumbnails (trickplay)** | Off | Chapter images. |
| **One thumbnail every (seconds, at least)** | **Settings › Thumbnails (trickplay)** | 10 seconds (5 to 60) | Shortest time between thumbnails. A version longer than its requests can read at that pace gets longer steps ([How the images look](#how-the-images-look)). |
| **Thumbnail width** | **Settings › Thumbnails (trickplay)** | 320 (or 240, 480) | Thumbnail width in pixels. |
| **Space for images (GB)** | **Settings › Thumbnails (trickplay)** | 2 GB (1 to 50) | Past this, the least recently used versions lose images, remade on their next play. |

Turning a setting off hides its images until it is turned back on.

### When images are made

Polyfin makes thumbnails during a play from what it reads, and the rest after watching, gently. The files of local folders also get theirs from a task, before any play ([Files of local folders](#files-of-local-folders)).

**During the play:**

- Every 30 seconds, Polyfin makes thumbnails from keyframes already in the [source cache](reading-the-sources.md#reading-the-sources), with no request. The file's index must be cached too, as a first play leaves it.
- They run from the version's start as far as cached keyframes are contiguous. Apps show them from the title's next opening.
- FFmpeg decodes one keyframe at a time, at lowest priority, never while the processor converts or tone maps a video.
- Chapter images wait for the end of the play.

**After the play:**

- For a version once played, and the next episode when [playback is prepared in advance](reading-the-sources.md#preparing-playback-in-advance).
- Polyfin waits until playback stops and nobody plays from the same host. A play of a version Polyfin does not know holds back no host.
- It adds what the source cache still holds, then reads only missing keyframes, located through the index (Matroska Cues or MP4 sample tables), never the whole file: one a request, or several when the host serves several ranges in one answer.
- One version at a time, with a short queue.

DVDs, recordings and Replay get no images. A torrent's file gets thumbnails from what its plays read alone ([Torrents' files](#torrents-files)). The log tells why a version gets none, with the title, once a day for each version.

### Files of local folders

A file of a local folder or a network share is read from the server's storage, not from a provider:

- It gets a thumbnail at every **One thumbnail every (seconds, at least)**: a 2-hour movie, 720 at 10 seconds.
- It takes none of the request limits below.
- It waits only while the same file plays, not for the plays of other local files.
- A file whose container has no index Polyfin reads, such as AVI, MPEG-TS, M2TS or MPG, is read whole by FFmpeg, which decodes its keyframes alone, as Jellyfin does when it extracts key frames only. It gets a thumbnail at every interval too, each showing the keyframe at its time.
- That read goes through the whole file once, from the disk or the share, FFmpeg decoding on the processor at lowest priority. If the same file starts playing, it stops, and starts again once the play is over.

The task Make thumbnails, in **System › Schedule**, makes the images of local files without waiting for a play:

- It runs every day at 3:00 by default, as Jellyfin's, and when started by hand.
- It makes the thumbnails and chapter images each local file lacks, one file at a time. A file not played yet is analyzed first.
- While **Thumbnails when moving through a title** and **Chapter images** are both off, it does nothing.
- It passes over discs, and the files whose last result says they cannot have images. Those that failed are tried again. A file found without an index by an earlier version has its result dropped at the upgrade, so the task makes its images.
- Addons' links and torrents still get theirs from their plays.

**Thumbnails**, on **Content › Versions**, tells where each local file stands:

- "15 made", "Waiting" or "Being made";
- "Cannot be made: the file cannot be read" or "Cannot be made: no video";
- "Failed, tried again at the next play";
- "None yet", before its first result.

**Make again** drops the file's thumbnails and chapter images, forgets that it failed or could not have them, and makes them again at once, without waiting for a play. It is refused while both settings are off, and offered for local files only.

jellyfin-web's "Refresh metadata" does the same with "Replace existing trickplay images" checked and a full refresh ("Search for missing metadata" or "Replace all metadata"), as Jellyfin asks: for the local files of a title, or of every title of a series, a season or a local folder's library.

The **Thumbnails (trickplay)** card of **System › Health** counts the local files by their last result (see [Health](health.md)).

### Torrents' files

A torrent's file is read only by its plays, from its swarm (see [Torrents](torrents.md)). Its thumbnails come from what those plays read:

- During a play, every 30 seconds, from the keyframes the play brought into the source cache, from the start of the file as far as they follow one another.
- After the play, once, from what the cache still holds.
- Nothing more: no request to the swarm, no read ahead, and no chapter images.
- The set is kept for the file, the same version for the same torrent and file, and each later play extends it. A seek leaves a gap, where the set stops until a play covers it.

### Request limits

These limits apply to addons' links.

| Limit | Value |
|---|---|
| Requests per version, index included | 60 at most |
| Keyframes per request | 1; up to 16, about 16 MB, from a host that serves several ranges in one answer |
| Pace | 1 request every 3 seconds |
| Per host, across versions | 120 an hour; past that, requests wait |
| Thumbnails from the source cache | No request |

### How the images look

- Each thumbnail gets its own keyframe. When thumbnails would outnumber the keyframes the requests left can read, they spread evenly over the runtime, a whole number of seconds apart.
- From a host that answers one range a request, with about 58 requests left, that is every 47 seconds for a 45-minute episode, and every 2 minutes 5 seconds for a 2-hour movie.
- From a host that serves several ranges in one answer, Polyfin first reads one keyframe for each request left. With the requests still left, it then reads keyframes for thumbnails closer together, as many as the requests before read each, down to **One thumbnail every (seconds, at least)**. A 2-hour movie can thus get a thumbnail every 10 seconds, 720, within its 60 requests. If the requests run out first, the thumbnails keep the last step whose keyframes were all read.
- Each shows the keyframe nearest its time, a few seconds off at most unless keyframes are further apart.
- During a play, thumbnails follow **One thumbnail every (seconds, at least)**. After it, Polyfin keeps them and reads only later ones; if the interval grows, kept ones move to its nearest time.
- A chapter image shows the keyframe nearest the chapter's start: its own read if requests allow one per thumbnail and chapter, else the nearest thumbnail read.
- FFmpeg decodes each keyframe as it arrives, tied to its time whatever the read order, on the GPU if any, tone mapping HDR.
- Thumbnails are packed into Jellyfin's 10x10 tile JPEGs. Chapter images are at most 640 pixels wide.
- Images are kept in the database.

### When a source misbehaves

| Case | What Polyfin does | Wait |
|---|---|---|
| Odd answer (whole file for a range, another range, a cut answer, or none) | Asks once more; the retry counts in the budget | Short pause |
| Two odd answers in a row | Fails the version; the log shows the answer, never the address | 30 minutes |
| Link the source says expired | Renews it, once per version, then asks again | None |
| Host asks to slow down (`429`) or is overloaded (`502`, `503`, `504`) | Stops the version, discarding that read; thumbnails from the play stay. Other hosts go on | Version: a day. Host: no image request for 2 hours |
| File cannot give images (an addon's file without an index, an unsupported codec) | Leaves the version alone | A day |
| Any other failure | Leaves the version alone | 30 minutes |

**Compared with Jellyfin**

- Jellyfin makes trickplay images in a scheduled task, every day at 3 a.m. ([TrickplayImagesTask.cs, lines 60 to 68](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Trickplay/TrickplayImagesTask.cs#L60-L68)), or during a library scan when the library asks ([TrickplayProvider.cs, line 101](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Trickplay/TrickplayProvider.cs#L101)), from the whole file. Polyfin makes them of the versions played, the first minutes of a first play during that play, and of the files of local folders in its own task, at 3:00 too.
- Thumbnail timing is approximate, from keyframes read.
- Chapters carry no `ImagePath`.
- Turning either setting off hides what was made, which shows again when turned back on.

API details: [Scrubbing thumbnails for apps](jellyfin-api.md#scrubbing-thumbnails-for-apps).
