# Music, audiobooks and podcasts

This page is for administrators who add music, from Eclipse music addons or from local music folders. It covers music libraries, local music files, lyrics, playback, the volume between songs, and what users under parental control see.

## Music addons

Eclipse music addons install like Stremio addons (see [Server addons](addons.md#server-addons)):

1. Open **Add a source**: under **Content › Sources** for the server, or under **My sources** for a user's own.
2. Choose the **Eclipse addon** tab.
3. Paste its manifest address, or its base address (`https://addon.example/{token}/`).

- Polyfin tells Stremio and Eclipse addons apart by their manifest.
- Eclipse addon addresses are redacted like manifest addresses (see [Manifest addresses](addons.md#manifest-addresses)).
- The sources list names their kind **Eclipse**, in its filter and on their badge. **System › Health** marks their row with the same badge.

### Music addon settings

Change the settings an addon declares in **Addon settings**, on the addon's page under **Content › Sources** or **My sources**. They can be pickers, switches, texts and numbers.

- Each setting holds one value.
- A per-network setting keeps its Wi-Fi default.
- Settings travel as query parameters with every request to the addon, as Eclipse sends them (see [How Polyfin asks an addon](addons.md#how-polyfin-asks-an-addon)).

## Music libraries

Each catalog row of an addon is a library under **Content › Libraries**: songs, albums, artists or playlists.

| The addon's `contentType` | The library | Its tracks |
|---|---|---|
| Music | A Jellyfin music library | Songs |
| `audiobook` | A books library | Audiobooks, with the chapters their stream gives |
| Podcasts | A music library, as podcasts have no Jellyfin type | Songs |

A [local music folder](#music-folders) is a music library too. Each user can have the songs they play scrobbled to their own Last.fm or ListenBrainz account (see [Music: Last.fm and ListenBrainz](tracking.md#music-last-fm-and-listenbrainz)).

### What a row lists

A row lists its own items. When an app asks for another kind, the library derives it:

- the albums and artists its songs name;
- or the songs of its albums, playlists or artists, reading at most 50 of their pages, 8 at a time.

An album or artist that a song only names is found again through the addon's search when opened. So addons without catalogs still serve search, and album, artist and playlist pages. An artist whose page at the addon lists no albums shows the albums its top songs name.

### What Polyfin keeps

| Answer | Read again after |
|---|---|
| Catalog pages | **Refresh catalogs after (minutes)** |
| Album, artist and playlist pages | 6 hours |
| Searches | 10 minutes (kept, then dropped) |

- A page due to be read again is still served at once, for up to 24 more hours, while Polyfin asks the addon again in the background.
- At most 8 requests for pages go to one addon at a time.

### My music

Every music addon with a search also offers "My music", a catalog Polyfin makes. It is listed with the addon's rows under **Content › Libraries** and **My sources**. It is turned on, named and ordered like any library. Without a name of its own, apps show it as "My music" in the server's language, such as "Ma musique" in French.

Each user sees their own music of the addon in it:

- the songs they played, most recently played first, then the songs they marked favorite;
- then the albums, artists and playlists they marked favorite.

- **Other kinds:** asked for another kind, it lists the albums and artists its songs name (and its albums' artists). Or it lists the songs of its albums, playlists and artists, as a row does.
- **Limit:** each kind lists at most **Titles read per movie and series catalog**.
- **Search:** searching from it searches the addon, as in any music library.
- **No requests:** Polyfin never asks the addon for this catalog. It has no automatic image, since every user sees a different list.

When it is turned on:

- **An addon without rows**, as many music addons that only search, gets "My music" turned on as its library. Music apps that start from a library then find the addon. The message that confirms the addon was added says so.
- **Empty at first:** it starts empty for every user. A user's songs show up once they found them with search, and played them or marked them favorite.
- **An addon with rows** keeps them as its libraries, with "My music" offered but off.
- **Library limit:** rows and "My music" are turned on this way only until the server has 20 libraries, as other addons' catalogs are (see [Server addons](addons.md#server-addons)).
- **Older addons:** addons added before "My music" existed keep their libraries. An administrator turns "My music" on under **Content › Libraries**, and users under **My sources**.

### Instant mixes

| Mix of | Holds |
|---|---|
| A song | That song, then its album's and its artist's other songs |
| An album | Its songs, and its artist's songs |
| An artist or a playlist | Their songs |

Every mix is shuffled.

**Compared with Jellyfin**

- Polyfin builds mixes from album and artist. Jellyfin's instant mixes pick songs that share genres, which addons rarely give.

## Music folders

A folder or share added as holding **Music** is a music library of its own (see [Local folders](local-folders.md)). Its audio files are read with their tags, and grouped into albums and artists. The library answers as an Eclipse music addon's does:

- albums, artists and songs views;
- album and artist pages;
- search, lyrics, scrobbling, statistics and playback.

It differs from an addon's library in two ways:

- It is one row of albums, read from its scans, never kept as addon answers are.
- Its songs view lists every album's songs, not the first 50.

What a folder holds can change, to or from music. Nothing users did follows the change (see [Changing what a folder holds](local-folders.md#changing-what-a-folder-holds)).

```
/media/music/Tone Quartet/Sine Studies/01 First Light.flac
/media/music/Tone Quartet/Sine Studies/CD2/01 Second Wind.flac
/media/music/Tone Quartet/Sine Studies/cover.jpg
```

### Files and tags

A scan lists these files:

| Kind | Extensions |
|---|---|
| Audio | `.mp3`, `.flac`, `.m4a`, `.m4b`, `.alac`, `.aac`, `.ogg`, `.oga`, `.opus`, `.wav`, `.aif`, `.aiff`, `.wma`, `.ape`, `.wv`, `.dsf` |
| Lyrics | `.lrc` |
| Playlists | `.m3u`, `.m3u8` |
| Artwork | Images beside the audio files |

- Hidden folders are left out.
- A music folder's subfolders named `Extras` or `Other` are read, as an album may bear such a name.

Each new or changed file is read with ffprobe. Polyfin keeps its title, artist, artists, album, album artist, and track and disc numbers. It also keeps its year (from its date), its genres (several separated by `;`), whether it is part of a compilation, its lyrics, its length, its audio, and whether it embeds a cover. Tags of MP3 (ID3), FLAC, Ogg and Opus (Vorbis comments), MP4 and M4A, WMA and WAV are read as ffprobe reads them.

A scan also keeps a file's gains, which even the volume between songs (see [Volume between songs](#volume-between-songs)):

- its `REPLAYGAIN_TRACK_GAIN` and `REPLAYGAIN_ALBUM_GAIN` tags, from ID3, Vorbis comments and MP4 alike, such as `-6.54 dB`;
- else Opus's `R128_TRACK_GAIN` and `R128_ALBUM_GAIN`, moved from EBU R128's -23 LUFS to the same -18 LUFS reference.

Files kept before Polyfin read gains have their tags read again once, at the next scan.

A file without some tags takes them from its path, as in `Artist/Album/01 Title.flac`:

- **Title:** its name without the number heading it, which is its track number. `1-07 Title` gives disc 1, track 7.
- **Album:** its folder's name. A folder named only after a disc, such as `CD2` or `Disc 2`, gives the disc number, and the folder above gives the album.
- **Artist:** the name of the folder above the album's.

A file whose tags and path give no title or no album is left out of the library, such as an untagged file at the folder's root. The folder's page lists it under **Tracks left out**, with why.

On a share, ffprobe and FFmpeg read each new or changed file through Polyfin, at most 16 MB of it per scan. That covers the tags, the stream headers and the embedded cover at its head, and the index at its end for MP4 files that keep it there. A file whose tags lie beyond is read by its path.

### Albums and artists

- **Albums:** tracks of the same album name and album artist, in any letter case, are one album. A track's album artist is its album artist tag. Otherwise it is "Various Artists" for a compilation (the compilation tag set to 1), else its artist tag as written.
- **A track's artists:** the names of its artists tag, a tag given several times as Vorbis comments allow. Otherwise they are the names of its artist tag, separated by `;`. `Ann; Ben` gives Ann and Ben, each with their page, and the song is listed under each. The artist tag is kept as written, for search.
- **An album's artists:** its album artist tag's, split the same way.
- **Track order:** disc, then track, then path.
- **Album details:** its title and artist are those of its first track. Its year is the latest of its tracks'. Its genres are all of theirs.
- **Artists:** albums group into artists by album artist. An artist's page lists their albums and songs, including songs on other artists' albums, at most 100.
- **Album artwork:** the cover embedded in its first track that embeds one. Otherwise, `cover`, `folder` or `front` (`.jpg`, `.jpeg`, `.png` or `.webp`, any letter case) beside its first track.
- **Artist pictures:** `artist`, `folder` or `photo` (`.jpg`, `.jpeg`, `.png` or `.webp`, any letter case) in their folder. That is the folder holding the folders of their albums, and of no album of another artist. Without one, it is the artwork of their first album. The artists views show it.
- **Search:** it finds songs by title, artist and album, albums by title and artist, and artists by name. Every word must be found, in any letter case.

The folder's figures count its tracks, albums and album artists. They also count its tracks with a gain (from their tags or measured), those measured, and the tracks left out.

### Playlists

The `.m3u` and `.m3u8` files of the folder are the playlists of its library. Its Playlists view lists them by name, with the user's own playlists.

- **Title:** its `#PLAYLIST:` line, else its file's name.
- **Lines:** they name tracks by paths relative to the playlist's folder, or absolute within the folder's path in the container, as paths or `file:` addresses. They may use slashes or backslashes, in any letter case. The playlist's songs are those, in order.
- **Windows paths:** a line may name its track by a Windows path, `D:\Music\…`, or a `file:///D:/Music/…` address, in any letter case.
- **Left out:** a line that names no track of the folder, or leads out of it. A playlist naming none is not listed.
- **Search** finds playlists by title.

### Rescans

- A rescan reads again the files new or changed by size and modification time, and forgets the files gone.
- It chooses again the artwork of the albums and artists whose first track or image changed.
- Lyrics files and playlists are read again when they change.
- Watching notices audio files, artwork, lyrics files and playlists, as it notices videos (see [Watching for changes](local-folders.md#watching-for-changes)).

The tracks a scan first finds count among the files added in the [weekly summary](notifications.md#weekly-summary), which names them by album.

<a id="limits"></a>

### Music folder limits

- **Playlists are read only:** apps cannot add songs to them, reorder or delete them. On a share, a playlist's absolute paths name nothing; relative paths do.
- **Artist names split at `;` only:** `A feat. B`, `A & B` or `A/B` is one artist, unless an artists tag names them. Without an album artist tag, an album's artist is the artist tag as written, one name.
- **Artists one by one:** apps list a song's artists one by one, as Jellyfin does, rather than its artist tag as written.
- **Artists views:** they list album artists. An artist credited only on songs is reached from those songs, and from search.
- **Lyrics files:** lyrics are read from `.lrc` files and tags, not from `.txt` files.

**Compared with Jellyfin**

- Polyfin groups albums by album artist and name, rather than by folder. Both read music folders with their tags, playlists and lyrics files.
- Polyfin splits artists at `;` only, and reads no `.txt` lyrics.

<a id="playback"></a>

## Music playback

Polyfin decides how to play a track with the app's device profile, as Jellyfin decides for audio.

**The track's format:**

- It comes from the codec, container, sample rate and bit depth of the addon's stream reply.
- When the reply or the track listing gives only a known format (`flac`, `mp3`, `aac`, `m4a`, `opus`, `ogg` or `wav`), Polyfin takes the codec and container from it.
- The stream is analyzed, as an audio file, only when neither gives its codec and container. Tracks listed with a `streamURL` play from it the same way.
- The reply does not give a lossless stream's bitrate. Polyfin takes it as the bitrate of its samples in stereo, so that a bitrate limit converts it.
- **Local songs** play from the folder or the share through Polyfin, as other local files do. Their format, codec, sample rate and bit depth come from the scan, so playback decides without probing them.

**How it plays:**

- **As it is:** through the same source and relay rules as videos (see [Playback](playback.md)). A track whose link expires before the track ends is always relayed, so that Polyfin can renew the link while it plays.
- **Converted:** with FFmpeg, to the codec, bitrate, sample rate and channels the app asks for. It is sent progressively or in 3-second HLS segments (see [Transcoding](transcoding.md)).

**Links:**

- An expired link is asked for again a minute before it expires (`expiresAt`), or once it fails.
- A link given less than 15 seconds ago is used until it expires, so one play asks for it once.
- Plays of the same track at the same time share one request.

**Tracks that cannot play:**

- A track whose addon gives no stream does not play. That is when the addon cannot be reached, answers an HTTP error other than `404`, or gives no stream address. Apps say the track cannot be played, as for a title none of whose versions plays.
- A `404` means the track no longer exists: it is not found.

**Next track:** with **Prepare playback in advance** on, Polyfin resolves the next track as a track starts. That is the next one in the queue the app reports, else the next one on the album (see [Playback](reading-the-sources.md#preparing-playback-in-advance)).

**User limits:** the user's conversion permission, bitrate limit and number of playbacks at once apply (see [Users](users.md)). Quality groups, which are about video heights, do not.

**Plays and resume points:**

- Playback reports count plays and mark songs played, without resume points.
- Audiobooks keep their resume points, with Jellyfin's audiobook thresholds.
- The activity log records audio playback as Jellyfin does.

Eclipse's video renditions are not used: Jellyfin music apps cannot show a video for a song.

**Compared with Jellyfin**

- Songs have no resume points, as in Jellyfin.

API details: [Music playback](jellyfin-api.md#music-playback-for-apps).

## Lyrics

Addons give no lyrics. With **Song lyrics from LRCLIB** on, Polyfin finds them on [LRCLIB](https://lrclib.net), a free and open lyrics database, and apps show them:

- synced line by line when LRCLIB has them, so that apps follow the song;
- else as plain text.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Song lyrics from LRCLIB** | **Settings › Content › Music** | On | Looks songs up on LRCLIB, and gives apps their lyrics. Off, songs have no lyrics and LRCLIB is never asked. Lyrics found before are kept, and shown again once it is back on. |

Instrumentals, songs LRCLIB does not know, and audiobooks have no lyrics.

### Lyrics of local songs

A song of a [local music folder](#music-folders) has its own lyrics: those of the `.lrc` file of the same name beside it, else those of its lyrics tag.

- They are synced line by line when timed, plain otherwise.
- They come before LRCLIB's. Polyfin never looks up a song with lyrics of its own on LRCLIB.
- They are served with **Song lyrics from LRCLIB** off too.
- Lyrics an administrator chooses or uploads replace them, until deleted (see [Choosing lyrics](#choosing-lyrics)). The file and the tag are never changed.

Other local songs have LRCLIB's.

### How Polyfin asks LRCLIB

Polyfin looks a song up when it starts playing, when its details open, or when an app asks for its lyrics. It never looks up a whole album or list at once: listings only show what is already known.

- **What it sends:** the song's artist, title, album and length, and nothing else. Its requests name Polyfin, its version and its repository, as LRCLIB asks.
- **Once a song:** Polyfin keeps what LRCLIB answered, lyrics or not, in its database. A song LRCLIB did not know is asked about again after 30 days, as LRCLIB grows every day.
- **Failures:** a request that fails is not taken for "no lyrics", and the song is asked about again later. That is when LRCLIB cannot be reached, answers an error, or does not answer within 10 seconds. After a failure, Polyfin leaves LRCLIB alone for a minute, or as long as LRCLIB asks, an hour at most.
- **No waiting:** playback and listings never wait for LRCLIB. A song's details wait one second at most, and a request for its lyrics 5 seconds. A later answer serves the next request.
- **Pace:** at most 4 requests go to LRCLIB at a time.

Apps show their lyrics button once a song's lyrics are known. The web player takes it from the list the song was played from. A song looked up as it plays for the first time shows the button the next time it is played from a list, or at once on its own page.

### Choosing lyrics

When the lyrics found are another song's, or LRCLIB found none under the song's name, an administrator can choose others in the web player. **Song lyrics from LRCLIB** must be on:

1. In the song's menu, choose **Edit lyrics**. The song's lyrics, if any, show at the top.
2. Below them, choose the search button. Polyfin searches LRCLIB for the song's title, artist and album. It lists the results under the name LRCLIB: the lyrics of the songs whose length is closest first, synced before plain. Each result says whether it is synced, and its preview button shows all its lines.
3. Choose the download button of the lyrics you want. They are the song's lyrics at once, for every user, synced or plain as listed.

An administrator can also upload a lyrics file, whether **Song lyrics from LRCLIB** is on or not:

1. In the editor, choose the add button beside the search button.
2. Pick an `.lrc` or `.txt` file, and upload it.

Its lyrics are the song's at once, for every user.

- **How it is read:** as a local song's `.lrc` file (see [Lyrics of local songs](#lyrics-of-local-songs)). It is synced line by line when its lines start with a time, plain text otherwise. A `.txt` file is read the same way. The word timing of enhanced LRC is left out.
- **Refused files:** a file over 1 MiB, one that is not UTF-8 text, or one without a line of text. The song keeps its lyrics. 1 MiB holds any song's lyrics many times over.

What Polyfin keeps:

- **Chosen and uploaded lyrics:** they are kept in Polyfin's database, as those found are, and replace them. No automatic look-up replaces them later. Choosing or uploading other lyrics replaces them again.
- **Local songs:** they replace a local song's own lyrics too, from its `.lrc` file or lyrics tag.
- **Deleting:** the editor's delete button, once confirmed, removes the lyrics Polyfin keeps for the song: chosen, uploaded or found. The song then has none. The removal is kept as a choice too: no automatic look-up gives the song lyrics again, until lyrics are chosen or uploaded for it.
- **A local song's own lyrics:** they are never deleted. Its `.lrc` file and tag stay as they are, and its lyrics show again.

With **Song lyrics from LRCLIB** off:

- the search finds nothing, and LRCLIB is not asked;
- lyrics chosen are kept, and shown again once it is back on;
- lyrics uploaded are not LRCLIB's, so they show whatever the setting;
- the delete button removes either.

Other rules:

- Searches and choices are asked of LRCLIB as look-ups are. They name Polyfin in the same way, and count among its 4 requests at a time.
- While LRCLIB is left alone after a failure, or when it fails, the search finds nothing. Uploads never ask LRCLIB.
- Only administrators see **Edit lyrics**. Polyfin refuses others' searches, choices, uploads and removals.

**Compared with Jellyfin**

- Polyfin asks LRCLIB itself, without a plugin, and reads the `.lrc` files and lyrics tags of local music folders. Jellyfin reads lyrics from the `.lrc` and `.txt` files next to the songs, or downloads them with a lyrics plugin.
- Polyfin keeps chosen or uploaded lyrics in its database, and its delete button removes only those. Jellyfin saves them as a file next to the song, and its delete button deletes the song's lyrics files. Polyfin leaves the song's files as they are.
- Polyfin reads an uploaded `.txt` file as an `.lrc` file, synced when its lines start with a time. Jellyfin reads it as plain text.
- Lines are synced, not words: the word timing of enhanced LRC is left out.

API details: [Song lyrics](jellyfin-api.md#song-lyrics-for-apps).

## Volume between songs

Songs from different albums and addons are mastered at very different volumes. Polyfin tells apps how much to raise or lower each song and each album, as Jellyfin's `NormalizationGain` does. Apps that apply it, such as the web player, play them all at about the same volume.

The songs themselves are not changed: an app that ignores the gain plays them as before.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Even the volume between songs** | **Settings › Content › Music** | On | Gives apps the gains known. Off, apps get none and nothing is measured. |
| **Measure the loudness of songs without a gain** | **Settings › Content › Music** | Off | **Off**, **Local songs** or **Local and addon songs**: measures the songs that have no track gain, with FFmpeg's `ebur128` filter (EBU R128). |

**Gains:**

- They are relative to -18 LUFS, the reference of ReplayGain 2.0 and of Jellyfin's gains.
- A song's gain is its ReplayGain track tag (see [Files and tags](#files-and-tags)), else its measured loudness.
- An album's gain is its album tag, when all its songs agree on one. Otherwise it is the loudness of all its songs together, each weighed by its length, once each one's is known.

**Measuring:**

- **Local songs** are measured in the background after a scan.
- **Addon songs:** they are measured after their first play, by reading their stream once more, through the same relay and link renewal as playback. A whole album or catalog is never measured ahead.
- **Once:** a song is measured once, its audio alone decoded. Its loudness is kept in the database with the song, as an analysis is. It is measured again only when a local file changes.
- **Pace:** two songs are measured at a time, over local and addon songs. FFmpeg runs at the lowest priority, as for the audio of local episodes (see [Skip buttons](skip-segments.md)).
- **Never measured:** songs longer than 15 minutes, podcasts' long episodes among them, songs whose length the addon does not give, and audiobooks.
- **Failures:** a song whose stream fails is tried again after a day, three times at most.
- **Silence:** a silent song has no gain.

## Health and parental control

Music addon requests count in **System › Health** like a Stremio addon's: last answer, failures and response time, with the same manifest check (see [Administration](administration.md)). What a failure says, and what to check when an addon refuses Polyfin's requests, is in [Addons that fail](addons.md#addons-that-fail).

- **Explicit music:** explicit songs and albums are hidden from users whose parental control sets a highest rating.
- **Unrated:** music has no rating. So all music is hidden from users who block unrated music, and audiobooks from users who block unrated books.
- **Other limits:** visible libraries and allowed hours apply as elsewhere.

API details: [Music libraries](jellyfin-api.md#music-libraries-for-apps).
