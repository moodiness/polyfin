<a id="scrubbing-thumbnails-and-chapter-images"></a>

# Scrubbing thumbnails

For administrators: how Polyfin makes the scrubbing thumbnails and chapter images Jellyfin apps show, when it reads sources, and its limits.

## Thumbnail settings

**Settings › Thumbnails (trickplay)** makes Jellyfin's scrubbing thumbnails and chapter images. Both are off by default, because both read parts of the source.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Thumbnails when moving through a title** | **Settings › Thumbnails (trickplay)** | Off | Trickplay thumbnails. |
| **Chapter images** | **Settings › Thumbnails (trickplay)** | Off | Chapter images. |
| **One thumbnail every (seconds, at least)** | **Settings › Thumbnails (trickplay)** | 10 seconds (5 to 60) | Shortest time between thumbnails. A version longer than the requests allow at that pace gets longer steps ([How the images look](#how-the-images-look)). |
| **Thumbnail width** | **Settings › Thumbnails (trickplay)** | 320 (or 240, 480) | Thumbnail width in pixels. |
| **Space for images (GB)** | **Settings › Thumbnails (trickplay)** | 2 GB (1 to 50) | Past this, the least recently used versions lose images, remade on their next play. |

Turning a setting off hides its images until it is turned back on.

### When images are made

Polyfin makes thumbnails during a play from what it reads, and the rest after watching, gently.

**During the play:**

- Every 30 seconds, Polyfin makes thumbnails from keyframes already in the [source cache](reading-the-sources.md#reading-the-sources), with no request. The file's index must be cached too, as a first play leaves it.
- They run from the version's start as far as cached keyframes are contiguous. Apps show them from the title's next opening.
- FFmpeg decodes one keyframe at a time, at lowest priority, never while the processor converts or tone maps a video.
- Chapter images wait for the end of the play.

**After the play:**

- For a version once played, and the next episode when [playback is prepared in advance](reading-the-sources.md#preparing-playback-in-advance).
- Polyfin waits until playback stops and nobody plays from the same host.
- It adds what the source cache still holds, then reads only missing keyframes, located through the index (Matroska Cues or MP4 sample tables), one request each, never the whole file.
- One version at a time, with a short queue.

DVDs, recordings, Replay and torrents get no images.

### Request limits

| Limit | Value |
|---|---|
| Requests per version, index included | 60 at most |
| Pace | 1 request every 3 seconds |
| Per host, across versions | 120 an hour; past that, requests wait |
| Thumbnails from the source cache | No request |

### How the images look

- Each thumbnail gets its own keyframe. When thumbnails would outnumber requests left after the index, they spread evenly over the runtime, a whole number of seconds apart.
- With about 58 requests left, that is every 47 seconds for a 45-minute episode, and every 2 minutes 5 seconds for a 2-hour movie.
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
| File cannot give images (no index, an unsupported codec) | Leaves the version alone | A day |
| Any other failure | Leaves the version alone | 30 minutes |

**Compared with Jellyfin**

- Jellyfin makes trickplay images in a scheduled task, every day at 3 a.m. ([TrickplayImagesTask.cs, lines 60 to 68](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Trickplay/TrickplayImagesTask.cs#L60-L68)), or during a library scan when the library asks ([TrickplayProvider.cs, line 101](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Trickplay/TrickplayProvider.cs#L101)), from the whole file. Polyfin makes them of the versions played, the first minutes of a first play during that play.
- Thumbnail timing is approximate, from keyframes read.
- Chapters carry no `ImagePath`.
- Turning either setting off hides what was made, which shows again when turned back on.

API details: [Scrubbing thumbnails for apps](jellyfin-api.md#scrubbing-thumbnails-for-apps).
