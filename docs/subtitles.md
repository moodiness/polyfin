# Subtitles

This page explains where Polyfin's subtitles come from and how they reach each Jellyfin app: addon subtitles and how they are synced to the version played, subtitle files listed with a stream, subtitle files beside local files, subtitle files inside torrents, text tracks inside files, ASS styles and fonts, and image subtitles, which Polyfin can read into text.

## Where subtitles come from

Apps get six kinds of subtitles:

- subtitles from the addons;
- the subtitle files a stream addon lists with a stream, for that stream's version;
- the subtitle files beside a local file, for that file's version;
- the subtitle files inside a torrent that go with the video a torrent version plays, for that version;
- the text tracks inside a version's file;
- the text copies of picture subtitles, with **Turn picture subtitles into text** on (see [Picture subtitles read into text](#picture-subtitles-read-into-text)).

All reach each app in the format it takes. Apps can save the user's subtitle language and subtitle mode, which choose the default subtitle track (see [Playback](playback.md#versions-and-analysis)); a user who chose no language gets the server's **Default subtitle language**, under their own mode (see [Default audio and subtitle languages](playback.md#default-audio-and-subtitle-languages)).

## Addon subtitles synced to the version

Addons find subtitles for a title, written for one release of it, not always the version played: another cut, another frame rate, a longer opening. They then run early or late, by a few seconds, or by more and more as the version plays. Polyfin syncs them to the version played.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Sync addon subtitles** | **Settings › Playback** | On | Finds when each version played has speech, and syncs the addons' subtitles to it. Torrent versions are left out. Off, subtitles come with the times the addons give, and no speech is looked for. |

### How Polyfin measures

Polyfin compares a subtitle's lines with when the version itself has speech:

- A text track inside the file, in any language, when there is one: its lines are timed for that file. A forced track comes after the others, and a track needs at least 20 lines. It must be read whole: through the file's index (see [Text tracks inside files](#text-tracks-inside-files)), or by a remux that extracted it.
- Else the version's audio: FFmpeg decodes its first audio track, and Polyfin measures the energy of the voice band, 300 to 3000 Hz, as subtitle aligners do. Speech is where it is loud compared with the quiet of the stretch and rises and falls with syllables, a few times a second, where music and effects hold; it is joined over short pauses. Polyfin listens to three stretches of up to 3 minutes, around a quarter, half and three quarters of the version, joined when the version is too short for them to be apart.

Polyfin finds this once per version, in the background, at the version's first play (when the app reports that a movie or an episode starts), and keeps it in the database:

- One version is looked at at a time, and up to 8 wait. A version whose source fails is tried again at a play 15 minutes later.
- An addon's file is read for at most 300 MB. Its three stretches last as long as that reads at the file's bitrate, from its size and length, up to 3 minutes each: a 12 Mbit/s file gets about 45 seconds each. Each stretch counts a tenth more, 10 seconds before it, where FFmpeg starts at a keyframe, and 8 MB that FFmpeg is sent beyond what it decodes. A file whose stretches would last under 30 seconds each, above about 16 Mbit/s, is not read, and a read that goes past 300 MB stops: the version then has no speech reference, and its subtitles come as they are. Shorter stretches hold fewer lines: a subtitle with less than 20 seconds of them in the stretches is left as it is.
- The files of local folders get stretches of 3 minutes, without a limit.
- A version that is not one file, such as a DVD, and a replay, which would hold one of its provider's connections, are not looked at.
- Nor is a torrent version: looking would download its stretches from the swarm as the playback starts, beside it (see [Torrents](torrents.md#only-while-playing)). Its addon subtitles come as they are.

### What it corrects

- A constant shift, of up to a minute either way, such as a longer or shorter opening.
- A speed difference between the usual frame rates, 23.976, 24 and 25 fps, either way: a subtitle timed for 25 fps runs 4% fast on a 23.976 fps version, and comes earlier and earlier.

Both are found together, to the hundredth of a second. Of two speeds that fit as well, such as 23.976 and 24 fps, which subtitles timed to the half second do not tell apart, Polyfin keeps the subtitle's own speed, or the one for the version's frame rate. A subtitle is changed only when it fits clearly better changed: more of its lines fall on speech, by at least 15% of their time, and that timing stands well above the other shifts, above any timing more than 2 seconds from it, and above the subtitle's own. A subtitle already in time, and one of another title, are left as they are.

It does not correct:

- cuts within the version, a scene added or removed: a subtitle gets one timing for all its lines, and stays wrong on one side of the cut;
- shifts of more than a minute, and other speeds;
- the subtitle files beside a local file, which are that file's own, and the subtitles users add.

### Served synced

Once the version's speech is known, each addon subtitle chosen for it is timed once, and that timing is kept:

- Apps get it synced in every format, WebVTT, SubRip, ASS or JSON, and in HLS.
- Its name ends with "(synced)" in the server language, « (recalé) » in French: in PlaybackInfo once it was timed, which happens for the subtitle the app shows and for those read lately.
- A subtitle chosen before its version's speech was known comes as it is, and is synced from the next time it is chosen.

## Subtitle files listed with a stream

A stream addon may list subtitle files with each stream, written for that file. Polyfin reads them for every stream addon, for links and torrents alike:

- They are subtitles of that stream's version only, listed after the title's subtitles (those users added, then the addons'), with the language the addon gives and the addon as their source. The item's details and PlaybackInfo list them as soon as the version is listed, a torrent version's too, before it was ever played.
- Each app gets them from Polyfin, in the format it asks for, as it gets the addons' subtitles: the addon's address never reaches apps. Polyfin downloads them only from public addresses for a user's own addons, as it does their streams.
- With **Sync addon subtitles** on, they are synced to their version as the addons' subtitles are; a torrent version's come as they are (see [Torrents](torrents.md#only-while-playing)).
- Their links may expire with the stream's. They last as long as the version list (**Keep version lists for (minutes)**, see [Addons and libraries](addons-and-libraries.md#catalog-limits-and-refresh)). A file asked for after that is read from the link the addon gives now: Polyfin uses the addon's answer if a newer one came within that time, else asks the addon again for the title's streams, or for the description that embeds the stream. A file the addon no longer lists, or one of a stream it no longer lists, is not served.
- They reach only the users the version reaches: parental control, blocked genres and the torrent permission apply to them as to their version.

**Compared with Jellyfin:** Jellyfin has no addons. Polyfin offers these files as Jellyfin offers the subtitle files beside a video: external subtitles of that version alone.

## Subtitle files beside local files

A video of a [local folder](local-folders.md) offers the SubRip, ASS, SSA and WebVTT files beside it, named after it, such as `Movie (2001).fr.srt` or `Movie (2001).en.sdh.ass`, and its picture subtitle files: PGS files (`.sup`) and VobSub pairs (`.idx` and `.sub`). [Naming files](local-folders.md#subtitle-files) tells how their names give their language and flags.

- They are subtitles of that file's version only, listed after the addons' subtitles, with their language, title, and whether they are forced, for the hearing impaired or shown by default.
- Each app gets them in the format it asks for, as it gets the addons' subtitles: WebVTT, SubRip, ASS or JSON. An ASS file asked as ASS keeps its styles and positions.
- Files in UTF-8 (with or without a byte order mark) and UTF-16 are read as they are. A file that is not valid UTF-8 is read as Windows-1252, the usual encoding of older subtitles, so that accents survive.
- Polyfin reads them from the folder or the network share when an app asks for them, and keeps their text for a while. A file that changes is read again after the next scan.
- Picture subtitle files are listed with the codec Jellyfin gives them, `PGSSUB` or `DVDSUB`. An app that draws PGS, such as jellyfin-web, gets the `.sup` file as it is, read by byte ranges. A VobSub pair is not one file: like Jellyfin 12.4, which gives VobSub as a file only extracted as Matroska, Polyfin burns it into the video, and so it does a PGS file for an app that does not draw PGS (see [Image subtitles](#image-subtitles)). With **Turn picture subtitles into text** on, both are also read into text copies, which play without conversion (see [Picture subtitles read into text](#picture-subtitles-read-into-text)).

## Subtitle files inside torrents

Many torrents carry their subtitles as files beside the video, such as `.srt` files in a `Subs` folder. A [torrent version](torrents.md) offers the text subtitle files of its torrent that go with the video it plays: SubRip (`.srt`), ASS (`.ass`), SSA (`.ssa`) and WebVTT (`.vtt`) files of up to 8 MB.

Which files go with the video:

- In the video's folder, those whose name begins with the video's, followed by a dot, as for the [subtitle files beside local files](#subtitle-files-beside-local-files): `Movie.2001.1080p.fr.srt` beside `Movie.2001.1080p.mkv`. A file whose name begins with another video's longer name is that video's.
- In a subtitles folder beside the video (`Subs`, `Sub`, `Subtitles` or `Subtitle`, in any letter case): those whose name begins with the video's; those whose name tells its episode, such as `Show.S01E02.English.srt`; all those of a folder in it named after the video or telling its episode, such as `Subs/Show.S01E02.1080p/2_English.srt`; and, when the video is the only one of its folder, those whose name tells no episode, such as `Subs/English.srt`.
- In a season's pack, those of the episode played alone: a file whose name or folder tells another episode is left out, and so is a file of a subtitles folder that tells none, beside several videos.

What a name tells past the video's name, past the episode it tells, or the whole name in a subtitles folder, gives the file's language, title and flags, read as the names of the files beside local files are (see [Naming files](local-folders.md#subtitle-files)): `Movie.2001.1080p.fr.srt` is French, `Subs/English.srt` English, `Movie.2001.1080p.en.forced.srt` English and forced. A number before a name, as in `2_English.srt`, is left out.

How they are listed, fetched and kept:

- They are subtitles of that version only, listed after its other subtitle files (those listed with its stream), with the version's addon as their source, and the language, title and flags their names tell.
- They are listed from the version's first play, once its torrent tells its files: in PlaybackInfo, then on the title's page. Before its first play, a torrent version's files are unknown. A torrent version played before Polyfin 0.46 lists them at its next play, which then waits for its torrent's files as its stream would, so that the indexes of its tracks hold for the whole play.
- The play that lists them chooses its tracks by the user's preferences, whatever tracks the app names: the app named them from the title's page, where the files were not listed yet, and their indexes have moved. jellyfin-web, for one, names no subtitle then ("Off", the page's only choice), and the file in the preferred language is shown all the same. From the next play on, the tracks the app names count, as for any version.
- jellyfin-web keeps a title's page it showed, with the item it read then: going back to the page of a torrent version played for the first time since it opened still shows no subtitle file, and its **Play** names none. A new load of the page shows them, with the preferred one chosen. The player's subtitle menu lists them in every play.
- The file in the user's preferred subtitle language is fetched as the play starts: the one the app shows by default when it is one of them, else the first of them that is not forced. Without a preferred language, only the one the app shows by default is fetched. The others are fetched when a user picks them, and served once fetched. Each costs a whole piece of the torrent (see [Torrents](torrents.md#subtitle-files-inside-the-torrent)).
- Once fetched, a file is kept in the database with its version, as the tracks read from files are: it stays listed and is served once the torrent stopped, in the player's subtitle menu and on the title's page, and across restarts.
- A file not fetched yet, picked once the torrent stopped, as after a pause of more than 30 seconds, starts the torrent again to fetch it. With torrents off, or their route down, it is not served: Jellyfin apps get the error Jellyfin gives for a subtitle it cannot serve, and the log tells why (`A subtitle could not be read`).
- Each app gets them in the format it asks for, as it gets the files beside local files: WebVTT, SubRip, ASS or JSON, and in HLS. An ASS file asked as ASS keeps its styles and positions, and their encodings are read as those of the files beside local files. They come as they are, never synced (see [Torrents](torrents.md#only-while-playing)).
- They reach only the users the version reaches: parental control, blocked genres and the torrent permission apply to them as to their version, and fetching them follows the torrents' route, as their playback does.

**Compared with Jellyfin:** Jellyfin has no torrents. It reads the subtitle files of a video's own folder whose name begins with the video's, never those of a subtitles folder (`GetExternalFiles` in `MediaBrowser.Providers/MediaInfo/MediaInfoResolver.cs`). Inside torrents, Polyfin also reads the subtitles folder beside the video, where many torrents put their subtitles; its local folders read the video's folder alone, as Jellyfin does.

## Text tracks inside files

Some apps take subtitles only as files, jellyfin-web first. These apps get the tracks inside a file from the start of playback.

For a Matroska file, Polyfin reads the track whole through the file's index, which tells where every subtitle line sits:

- Polyfin fetches little more than those lines, many in one request when the host allows it.
- It keeps the track, so it takes seconds the first time and nothing after.
- With [Prepare playback in advance](playback.md#preparing-playback-in-advance) on, Polyfin finds where the subtitle tracks sit before playback starts.

### Hosts that answer one range at a time

A host that answers one byte range at a time would take a request for nearly every line, and providers refuse requests past a rate.

- A track that would take such a host more than 64 requests (the dialogue of a movie, for one) is not read this way, nor offered as a file.
- Before it offers such a track, Polyfin learns whether the host serves several ranges at once, from what it already knows of the host or with one request for two ranges.
- Polyfin logs the tracks it leaves out.

### Tracks offered after a remux

Tracks left out for that reason, and tracks the index does not locate (as in MP4 files), are offered as files once a remux has read them whole. See [Transcoding](transcoding.md).

## ASS styles and fonts

ASS tracks keep their styles and positions. The fonts the file carries are listed for apps that render ASS.

Fallback fonts, which apps load when a subtitle's own fonts are missing, come from the folder set by `POLYFIN_FONTS_DIR`. See [Configuration](configuration.md).

## Image subtitles

When an app cannot show image subtitles, Polyfin burns them into the video: an image track inside the file, or a picture subtitle file beside a local file, which FFmpeg reads beside the video. This is a video conversion: it counts toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)) and is described in [Transcoding](transcoding.md). A subtitle file burned in is read from its start; after a seek, a picture shown more than 10 seconds before the seek point and still on screen is not drawn until the next one.

With **Turn picture subtitles into text** on, a text copy of the track spares that conversion once it is ready.

### Picture subtitles read into text

PGS and DVD (VobSub) subtitles are pictures, and many apps take only text: jellyfin-web in Chrome (unless set to draw PGS) and Findroid list text formats alone, and Swiftfin's native player asks PGS and VobSub burned in. Polyfin can read these pictures into text with Tesseract, so that these apps get a text track instead of a conversion.

Under **Settings › Conversion (transcoding)**, in **Picture subtitles**:

| Setting | Default | What it does |
|---|---|---|
| **Turn picture subtitles into text** | **Off** | **When played** reads a version's picture tracks the first time an app that cannot draw them plays it, the chosen track first. **Also after library scans** also reads the tracks of the new or changed videos that scans of local folders find. |
| **Picture subtitle tracks read at once** | 1 (range 1 to 4) | How many tracks Tesseract reads at the same time, each in one process using one processor thread. |

What is read:

- PGS and VobSub tracks inside an addon's Matroska file, read through the file's index as text tracks are, within the same request limits (see [Text tracks inside files](#text-tracks-inside-files)).
- The tracks of local files and discs, which FFmpeg copies out without conversion.
- PGS files (`.sup`) and VobSub pairs (`.idx` and `.sub`) beside local files. Of a `.idx` file listing several tracks, the first is read.
- DVB subtitles are not read.

Polyfin decodes the pictures itself. Each one is cropped to its text, made dark text on a white background, as Tesseract reads best, and enlarged when its lines are small, as those of DVDs are. The times of the lines are those of the pictures; for an MPEG-TS file, such as a Blu-ray's `.m2ts` file, an MPEG program stream, such as a `.vob` file, an AVI file, and a disc, they count from the start of the video, as their playbacks do. A picture is drawn only within the video the track names, a DVD's at least for VobSub, and at most 3840×2160: what a track places past it is left out. Tesseract reads a track's pictures by batches of at most 64 MB of pictures, so that a long track never holds all of them in memory at once.

How apps see the copy:

- It is one more subtitle track, listed after the version's tracks so that no track's index moves, with the original's language and its forced and hearing-impaired flags. Its title is the original's title or language followed by "(OCR)", such as `English (OCR) - SUBRIP - External`.
- It reaches apps as other text subtitles do, in the format the app asks for (WebVTT, SubRip, ASS or JSON) or in the HLS playlist. It needs no video conversion, and plays with **Conversion (transcoding)** off and for users without **Can use conversion (transcoding)**.
- The picture track stays: it is listed and served as before, and burned in when an app names it, other than as the default track (see below).
- When the app asks for no track, or names the default track it was given for the version, as jellyfin-web always does, the default choice treats the copy like its original, and picks the copy only for an app that would need the original burned in. A track the app names that is not the default stays as asked.

When it runs:

- Reading runs in the background and never delays a playback. Until a copy is ready, the track is burned in as before. The copy is listed from the next time the app asks how to play the version, which most apps do as each playback starts.
- Each track is read once. Its text is kept in the database, not in the `cache` folder that is emptied at startup, and survives restarts. A track inside a local file is known by the file's folder, path, size and modification time, a track of an addon's file by its version and stream, and a subtitle file beside a local file by its path, size and modification time. A file that changes is read again.
- A track that could not be read is tried again after 6 hours; a track of a torrent version whose pieces did not come in time, at the next play (see [Torrents](torrents.md#deadlines)). A track in which Tesseract finds no text gets no copy.

Languages:

- Tesseract reads each track with its language's data: `fre` and `fra` both use `fra`. A track with no stated language uses the server's language.
- The Docker image comes with the data of English, French, German, Spanish, Italian, Portuguese and Dutch. Data for other languages, `.traineddata` files, goes in the folder `POLYFIN_TESSDATA_DIR` names, `/data/tessdata` in the image (see [Configuration](configuration.md)).
- A track whose language has no data is not read, and **System › Health** names the language and the file to add (see [Health](administration.md#health)).

Known limits: italics, colors and positions are lost, and the copy shows plain lines at the bottom. Stylized fonts and small DVD pictures can give misread letters. This is why the original stays, and the copy is marked.

**Compared with Jellyfin:** Jellyfin never reads picture subtitles into text.
