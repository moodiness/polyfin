# Subtitles

This page is for administrators who want to know where each app's subtitles come from. It covers addon subtitles and their sync, subtitle files from streams, local folders and torrents, text tracks inside files, ASS fonts, and picture subtitles read into text.

## Where subtitles come from

Apps get seven kinds of subtitles:

| Kind | For which versions |
|---|---|
| Subtitle files users add from their app (see [Subtitle files](jellyfin-compatibility.md#subtitle-files)) | Every version of the movie or episode |
| Subtitles from the addons | Every version |
| Subtitle files a stream addon lists with a stream | That stream's version |
| Subtitle files beside a local file | That file's version |
| Subtitle files inside a torrent that go with the video it plays | That torrent version |
| Text tracks inside a version's file | That version |
| Text copies of picture subtitles, with **Turn picture subtitles into text** on (see [Picture subtitles read into text](#picture-subtitles-read-into-text)) | The version of the picture track |

- Each app gets every kind in the format it takes.
- Apps can save the user's subtitle language and subtitle mode, which choose the default track (see [Playback](playback.md#versions-and-analysis)).
- A user who chose no language gets the server's **Default subtitle language**, under their own mode (see [Default audio and subtitle languages](audio-and-subtitle-tracks.md#default-audio-and-subtitle-languages)).

## Addon subtitles synced to the version

Polyfin syncs addon subtitles to the version played. Addons find subtitles written for one release of a title, which is not always the version played. Another cut, another frame rate or a longer opening makes them run early or late. They may be off by a few seconds, or drift more and more as the version plays.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Sync addon subtitles** | **Settings › Playback** | On | Finds when each version played has speech, and syncs the addons' subtitles to it. Torrent versions are left out. Off, subtitles keep the times the addons give, and no speech is looked for. |

### How Polyfin measures

Polyfin compares a subtitle's lines with when the version itself has speech. The speech comes from one of two places, in this order.

First, a text track inside the file, in any language, when there is one. Its lines are timed for that file.

- A forced track comes after the others.
- A track needs at least 20 lines.
- The track must be read whole: through the file's index (see [Text tracks inside files](#text-tracks-inside-files)), or by the repackaging that extracted it.

Else, the version's audio. FFmpeg decodes its first audio track, and Polyfin measures the energy of the voice band, 300 to 3000 Hz, as subtitle aligners do.

- Speech is where the sound is loud compared with the quiet of the stretch.
- Speech also rises and falls with syllables, a few times a second, where music and effects hold steady.
- Speech is joined over short pauses.
- Polyfin listens to three stretches of up to 3 minutes, around a quarter, half and three quarters of the version.
- The stretches are joined when the version is too short for them to be apart.

Polyfin finds the speech once per version, in the background, at its first play, and keeps it in the database. A first play is when the app reports that a movie or an episode starts.

- One version is looked at at a time, and up to 8 wait.
- A version whose source fails is tried again at a play 15 minutes later.
- The files of local folders get stretches of 3 minutes, without a limit.
- A version that is not one file, such as a DVD, is not looked at.
- A replay is not looked at, as it would hold one of its provider's connections.
- A torrent version is not looked at: its stretches would download from the swarm beside the playback (see [Torrents](torrents.md#only-while-playing)). Its addon subtitles come as they are.

An addon's file is read for at most 300 MB:

- Its three stretches last as long as 300 MB reads at the file's bitrate, from its size and length, up to 3 minutes each.
- A 12 Mbit/s file gets about 45 seconds per stretch.
- Each stretch counts a tenth more: 10 seconds before it, where FFmpeg starts at a keyframe, and 8 MiB sent to FFmpeg beyond what it decodes.
- A file whose stretches would last under 30 seconds each, above about 16 Mbit/s, is not read.
- A read that goes past 300 MB stops.
- In both cases the version has no speech reference, and its subtitles come as they are.
- Shorter stretches hold fewer lines: a subtitle with less than 20 seconds of lines in the stretches is left as it is.

### What it corrects

| Difference | Range |
|---|---|
| A constant shift, such as a longer or shorter opening | Up to a minute either way |
| A speed difference between the usual frame rates | 23.976, 24 and 25 fps, either way |
| Another cut, against a text track inside the file only: a scene the version adds or removes | Up to 10 minutes (see [Across a cut](#across-a-cut)) |

A subtitle timed for 25 fps runs 4% fast on a 23.976 fps version, and comes earlier and earlier. After a cut, a subtitle runs early or late by the scene's length.

Shift and speed are found together, to the hundredth of a second.

- Two speeds may fit as well, such as 23.976 and 24 fps, which subtitles timed to the half second do not tell apart. Polyfin then keeps the subtitle's own speed, or the one for the version's frame rate.
- A subtitle is changed only when it fits clearly better changed.
- More of its lines must fall on speech, by at least 15% of their time.
- That timing must stand well above the other shifts: above any timing more than 2 seconds from it, and above the subtitle's own.
- A subtitle already in time is left as it is, and so is one of another title.

It does not correct:

- cuts within the version when its speech comes from its audio: three stretches cannot tell where a scene was added or removed, so the subtitle gets one timing for all its lines, and stays wrong on one side of the cut;
- shifts of more than a minute, except after a cut;
- other speeds;
- the subtitle files beside a local file, which are that file's own;
- the subtitles users add.

### Across a cut

A subtitle timed on another cut, with one scene more or less than the version, fits at one speed but changes shift at the scene. A text track inside the file gives the version's lines from start to end, so Polyfin can find where the shift changes.

- Polyfin finds the shift of each 2 minutes of the subtitle, a minute apart, among shifts of up to 10 minutes either way.
- Each shift is kept when it stands clearly above the others, as for a whole subtitle.
- Stretches of one shift that follow each other make a piece.
- Polyfin refines each piece's shift to the hundredth of a second, at the subtitle's speed.
- Between two pieces, Polyfin places the cut where the lines fit best.
- When the version lacks a scene, the subtitle's lines that start within that scene are left out. When the version adds a scene, no line is left out.

The pieces are kept only when each is clear. Else the subtitle gets one timing, as above. A clear piece has:

- at least 10 lines over at least 2 minutes;
- lines on speech that start within half a second of a line of the track, by their median;
- at least 15% more of its time on speech than at the shift of the piece next to it.

Other cases:

- A subtitle without a cut gets the timing it got before.
- Most of a subtitle may be shifted by more than a minute, as after a scene removed in its first minutes. Polyfin then tries each speed and keeps the one whose pieces fit best.
- A line or two right at a cut can be kept or left out wrongly when the track has speech on both sides of it.

### Served synced

Once the version's speech is known, each addon subtitle chosen for it is timed once, and that timing is kept.

- Apps get it synced in every format (WebVTT, SubRip, ASS or JSON) and in HLS.
- Its name ends with "(synced)" in the server language, « (recalé) » in French.
- PlaybackInfo shows that name once the subtitle was timed: for the subtitle the app shows, and for those read lately.
- A subtitle synced across a cut has the same name.
- A subtitle chosen before its version's speech was known comes as it is. It is synced from the next time it is chosen.
- Subtitles synced against a text track before Polyfin looked for cuts are timed again the next time they are chosen.

**Compared with Jellyfin**

- Jellyfin does not sync subtitles: it serves each with the times it has.

## Subtitle files listed with a stream

A stream addon may list subtitle files with each stream, written for that file. Polyfin reads them for every stream addon, for links and torrents alike.

- They are subtitles of that stream's version only.
- They are listed after the title's subtitles: those users added, then the addons'.
- They carry the language the addon gives, and the addon as their source.
- The item's details and PlaybackInfo list them as soon as the version is listed, a torrent version's too, before its first play.
- Each app gets them from Polyfin, in the format it asks for, as for addon subtitles. The addon's address never reaches apps.
- Polyfin downloads them only from public addresses for a user's own addons, as it does their streams.
- With **Sync addon subtitles** on, they are synced to their version as addon subtitles are. A torrent version's come as they are (see [Torrents](torrents.md#only-while-playing)).
- They reach only the users the version reaches: parental control, blocked genres and the torrent permission apply as to their version.

Their links may expire with the stream's:

- They last as long as the version list, set by **Keep version lists for (minutes)** (see [Addons and libraries](addons-and-libraries.md#catalog-limits-and-refresh)).
- A file asked for after that is read from the link the addon gives now.
- Polyfin uses the addon's answer if a newer one came within that time.
- Else it asks the addon again for the title's streams, or for the description that embeds the stream.
- A file the addon no longer lists is not served, nor is a file of a stream it no longer lists.

**Compared with Jellyfin**

- Jellyfin has no addons. Polyfin offers these files as Jellyfin offers the subtitle files beside a video: external subtitles of that version alone.

## Subtitle files beside local files

A video of a [local folder](local-folders.md) offers the subtitle files beside it that are named after it, such as `Movie (2001).fr.srt` or `Movie (2001).en.sdh.ass`. [Naming files](naming-and-matching.md#subtitle-files) tells how their names give their language and flags.

| Files | Kind |
|---|---|
| SubRip, ASS, SSA and WebVTT | Text |
| PGS files (`.sup`) | Picture |
| VobSub pairs (`.idx` and `.sub`) | Picture |

- They are subtitles of that file's version only, listed after the addons' subtitles.
- Each carries its language and title, and whether it is forced, for the hearing impaired or shown by default.
- Each app gets the text files in the format it asks for: WebVTT, SubRip, ASS or JSON.
- An ASS file asked for as ASS keeps its styles and positions.
- Files in UTF-8 (with or without a byte order mark) and UTF-16 are read as they are.
- A file that is not valid UTF-8 is read as Windows-1252, the usual encoding of older subtitles, so that accents survive.
- Polyfin reads them from the folder or the network share when an app asks, and keeps their text for a while.
- A file that changes is read again after the next scan.

Picture subtitle files:

- They are listed with the codec Jellyfin gives them, `PGSSUB` or `DVDSUB`.
- An app that draws PGS, such as the web player, gets the `.sup` file as it is, read by byte ranges.
- A PGS file is burned into the video for an app that does not draw PGS (see [Picture subtitles](#picture-subtitles)).
- A VobSub pair is not one file, so Polyfin burns it into the video. Jellyfin 12.4 likewise gives VobSub as a file only when extracted as Matroska.
- With **Turn picture subtitles into text** on, both are also read into text copies, which play without conversion (see [Picture subtitles read into text](#picture-subtitles-read-into-text)).

## Subtitle files inside torrents

A [torrent version](torrents.md) offers the text subtitle files of its torrent that go with the video it plays. Many torrents carry their subtitles as files beside the video, such as `.srt` files in a `Subs` folder.

- **Formats:** SubRip (`.srt`), ASS (`.ass`), SSA (`.ssa`) and WebVTT (`.vtt`).
- **Size:** up to 8 MiB per file.

Which files go with the video:

| Where | Files taken |
|---|---|
| The video's folder | Those whose name begins with the video's, then a dot, as for [subtitle files beside local files](#subtitle-files-beside-local-files): `Movie.2001.1080p.fr.srt` beside `Movie.2001.1080p.mkv` |
| A subtitles folder beside the video: `Subs`, `Sub`, `Subtitles` or `Subtitle`, in any letter case | Those whose name begins with the video's |
| | Those whose name tells its episode, such as `Show.S01E02.English.srt` |
| | All those of a folder in it named after the video or telling its episode, such as `Subs/Show.S01E02.1080p/2_English.srt` |
| | When the video is the only one of its folder, those whose name tells no episode, such as `Subs/English.srt` |

- A file whose name begins with another video's longer name is that video's.
- In a season's pack, only the files of the episode played are taken.
- A file whose name or folder tells another episode is left out.
- A file in a subtitles folder that tells no episode, beside several videos, is left out.

The rest of the name gives the file's language, title and flags, read as for files beside local files (see [Naming files](naming-and-matching.md#subtitle-files)). The rest is what follows the video's name, or the episode the name tells, or the whole name in a subtitles folder.

- `Movie.2001.1080p.fr.srt` is French.
- `Subs/English.srt` is English.
- `Movie.2001.1080p.en.forced.srt` is English and forced.
- A number before a name, as in `2_English.srt`, is left out.

How they are listed:

- They are subtitles of that version only, listed after its other subtitle files (those listed with its stream).
- They carry the version's addon as their source, and the language, title and flags their names tell.
- They are listed from the version's first play, once its torrent tells its files: in PlaybackInfo, then on the title's page.
- Before its first play, a torrent version's files are unknown.

The play that first lists them chooses its tracks by the user's preferences, whatever tracks the app names:

- The app named its tracks from the title's page, where the files were not listed yet, and their indexes have moved.
- The web player, for one, names no subtitle then ("Off", the page's only choice). The file in the preferred language is shown all the same.
- From the next play on, the tracks the app names count, as for any version.
- The web player keeps a title's page it showed, with the item it read then.
- Going back to that page after a first torrent play still shows no subtitle file, and its **Play** names none.
- Loading the page again shows them, with the preferred one chosen.
- The player's subtitle menu lists them in every play.

How they are fetched and kept:

- The file in the user's preferred subtitle language is fetched as the play starts.
- That is the one the app shows by default when it is one of them, else the first of them that is not forced.
- Without a preferred language, only the one the app shows by default is fetched.
- The others are fetched when a user picks them, and served once fetched.
- Each file costs a whole piece of the torrent (see [Torrents](torrents.md#subtitle-files-inside-the-torrent)).
- Once fetched, a file is kept in the database with its version, as tracks read from files are.
- It then stays listed and served once the torrent stopped, in the player's subtitle menu, on the title's page, and across restarts.
- A file not fetched yet and picked once the torrent stopped, as after a pause of more than 30 seconds, starts the torrent again.
- With torrents off, or their route down, such a file is not served. Apps get the error Jellyfin gives for a subtitle it cannot serve.
- The log then tells why: `A subtitle could not be read`.

How they are served:

- Each app gets them in the format it asks for, as for files beside local files: WebVTT, SubRip, ASS or JSON, and in HLS.
- An ASS file asked for as ASS keeps its styles and positions.
- Their encodings are read as for files beside local files.
- They come as they are, never synced (see [Torrents](torrents.md#only-while-playing)).
- They reach only the users the version reaches: parental control, blocked genres and the torrent permission apply as to their version.
- Fetching them follows the torrents' route, as their playback does.

**Compared with Jellyfin**

- Jellyfin has no torrents. It reads the subtitle files of a video's own folder whose name begins with the video's, never those of a subtitles folder (`GetExternalFiles` in `MediaBrowser.Providers/MediaInfo/MediaInfoResolver.cs`).
- Inside torrents, Polyfin also reads the subtitles folder beside the video, where many torrents put their subtitles. Its local folders read the video's folder alone, as Jellyfin does.

## Text tracks inside files

Apps that take subtitles only as files, the web player first, get the tracks inside a file from the start of playback.

For a Matroska file, Polyfin reads the track whole through the file's index, which tells where every subtitle line sits.

- Polyfin fetches little more than those lines, many in one request when the host allows it.
- It keeps the track, so reading takes seconds the first time and nothing after.
- With [Prepare playback in advance](reading-the-sources.md#preparing-playback-in-advance) on, Polyfin finds where the subtitle tracks sit before playback starts.

### Hosts that answer one range at a time

A host that answers one byte range at a time would take a request for nearly every line. Providers refuse requests past a rate.

- A track that would take such a host more than 64 requests is not read this way, nor offered as a file. The dialogue of a movie is one such track.
- Before it offers such a track, Polyfin learns whether the host serves several ranges at once.
- It learns this from what it already knows of the host, or with one request for two ranges.
- Polyfin logs the tracks it leaves out.

<a id="tracks-offered-after-a-remux"></a>

### Tracks offered after repackaging

Two kinds of tracks are offered as files once repackaging has read them whole (see [Transcoding](transcoding.md)):

- tracks left out for the reason above;
- tracks the index does not locate, as in MP4 files.

## ASS styles and fonts

ASS tracks keep their styles and positions. The fonts the file carries are listed for apps that render ASS.

Fallback fonts, which apps load when a subtitle's own fonts are missing, come from the folder `POLYFIN_FONTS_DIR` names (see [Environment variables](configuration.md#environment-variables)).

<a id="image-subtitles"></a>

## Picture subtitles

When an app cannot show picture subtitles, Polyfin burns them into the video. This covers a picture track inside the file, and a picture subtitle file beside a local file, which FFmpeg reads beside the video.

- Burning in is a video conversion, described in [Transcoding](transcoding.md).
- It counts toward **Video conversions at once (0 = no limit)** (see [Transcoding](transcoding.md#video-conversions-at-once)).
- A subtitle file burned in is read from its start.
- After a seek, a picture shown more than 10 seconds before the seek point and still on screen is not drawn until the next one.
- With **Turn picture subtitles into text** on, a text copy of the track spares that conversion once it is ready.

### Picture subtitles read into text

PGS and DVD (VobSub) subtitles are pictures, and many apps take only text. Polyfin can read these pictures into text with Tesseract, so that these apps get a text track instead of a conversion.

- The web player in Chrome lists text formats alone, unless set to draw PGS.
- Findroid lists text formats alone.
- Swiftfin's native player asks for PGS and VobSub burned in.

In **Settings › Conversion (transcoding) › Picture subtitles**:

| Setting | Default | What it does |
|---|---|---|
| **Turn picture subtitles into text** | **Off** | **When played** reads a version's picture tracks the first time an app that cannot draw them plays it, the chosen track first. **Also after library scans** also reads the tracks of the new or changed videos that scans of local folders find. |
| **Picture subtitle tracks read at once** | 1 (range 1 to 4) | How many tracks Tesseract reads at the same time, each in one process using one processor thread. |
| **Download the data of missing languages automatically** | **Off** | The first time a track is in a language Tesseract has no data for, Polyfin downloads that language's data once, in the background, then reads the track (see the languages below). |

What is read:

- PGS and VobSub tracks inside an addon's Matroska file, read through the file's index as text tracks are, within the same request limits (see [Text tracks inside files](#text-tracks-inside-files)).
- The tracks of local files and discs, which FFmpeg copies out without conversion.
- PGS files (`.sup`) and VobSub pairs (`.idx` and `.sub`) beside local files.
- Of a `.idx` file listing several tracks, the first is read.
- DVB subtitles are not read.

How the pictures are read:

- Polyfin decodes the pictures itself.
- Each picture is cropped to its text, and made dark text on a white background, as Tesseract reads best.
- A picture is enlarged when its lines are small, as those of DVDs are.
- The times of the lines are those of the pictures.
- For MPEG-TS files (such as a Blu-ray's `.m2ts`), MPEG program streams (such as `.vob`), AVI files and discs, times count from the start of the video, as their playbacks do.
- A picture is drawn only within the video the track names, at least a DVD's for VobSub, and at most 3840×2160. What a track places past it is left out.
- Tesseract reads a track's pictures in batches of at most 64 MiB, so that a long track never holds all of them in memory at once.

How apps see the copy:

- It is one more subtitle track, listed after the version's tracks so that no track's index moves.
- It carries the original's language, and its forced and hearing-impaired flags.
- Its title is the original's title or language followed by "(OCR)", such as `English (OCR) - SUBRIP - External`.
- It reaches apps as other text subtitles do, in the format the app asks for (WebVTT, SubRip, ASS or JSON) or in the HLS playlist.
- It needs no video conversion. It plays with **Conversion (transcoding)** off, and for users without **Can use conversion (transcoding)**.
- The picture track stays: it is listed and served as before.
- The picture track is burned in when an app names it, other than as the default track.
- The app may ask for no track, or name the default track it was given for the version, as jellyfin-web always does. The default choice then treats the copy like its original.
- The default choice picks the copy only for an app that would need the original burned in.
- A track the app names that is not the default stays as asked.

When it runs:

- Reading runs in the background and never delays a playback.
- Until a copy is ready, the track is burned in as before.
- The copy is listed from the next time the app asks how to play the version, which most apps do as each playback starts.
- Each track is read once. Its text is kept in the database and survives restarts, unlike the `cache` folder, which is emptied at startup.
- A file that changes is read again. Polyfin knows each track by:

| Track | Known by |
|---|---|
| Inside a local file | The file's folder, path, size and modification time |
| Inside an addon's file | Its version and stream |
| A subtitle file beside a local file | Its path, size and modification time |

- A track that could not be read is tried again after 6 hours.
- A track of a torrent version whose pieces did not come in time is tried again at the next play (see [Torrents](torrents.md#deadlines)).
- A track in which Tesseract finds no text gets no copy.

Languages:

- Tesseract reads each track with its language's data: `fre` and `fra` both use `fra`.
- A track with no stated language uses the server's language.
- The Docker image comes with the data of English, French, German, Spanish, Italian, Portuguese and Dutch.
- Data for other languages, `.traineddata` files, goes in the folder `POLYFIN_TESSDATA_DIR` names, `/data/tessdata` in the image (see [Configuration](configuration.md)). Polyfin can download it there.
- A track whose language has no data is not read. **System › Health** names the language and the file to add (see [Health](administration.md#health)).
- The track waits for the file. Once it is there, the waiting tracks are read and the warning goes.
- That is at once for a file Polyfin downloads. For a file added by hand, it is when Health next looks: within 5 minutes, or as its page opens.

Downloading a language's data:

- Polyfin downloads from Tesseract's official data: the fast models of `tesseract-ocr/tessdata_fast`, at a commit pinned in Polyfin.
- Polyfin carries the list of the 126 language files at that commit, with the SHA-256 of each. It keeps a file only when it matches.
- **From Health:** the warning of a language on that list offers **Download** while `POLYFIN_TESSDATA_DIR` is set.
- Polyfin creates the folder if missing, and writes the file under a temporary name.
- It checks the file's size and SHA-256, and only then renames it into place, such as `fin.traineddata`.
- On success, the warning goes and the tracks waiting for the language are read.
- Automatically, with **Download the data of missing languages automatically** on: the first time a track needs a missing language, Polyfin downloads its data once, in the background.
- It logs this at Info (`Downloading Tesseract's data for a language picture subtitles are in`, then `Downloaded Tesseract's data for a language`), then reads the track.
- Other tracks in that language wait for the same download.
- **Failures:** a folder that cannot be written, a failed download, and a file whose SHA-256 differs are reported. A file that does not match is never kept.
- They are named in the answer to **Download**, logged at Warn, and told in the Health warning, which keeps its button.
- After a failure, the automatic download is tried again an hour later at the soonest, when a track next needs the language.
- A language outside the list, such as a code Tesseract has no data for, is refused: add its file by hand.
- The files weigh a few megabytes each, 12.4 MB at most (Sanskrit's).
- `POLYFIN_TESSDATA_URL` names another address to download them from, such as a mirror (see [Configuration](configuration.md)). Their SHA-256 is checked all the same.
- `scripts/tessdata-list.sh` builds the list from the pinned commit, which changes only with Polyfin's versions.

Known limits:

- Italics, colors and positions are lost: the copy shows plain lines at the bottom.
- Stylized fonts and small DVD pictures can give misread letters.
- This is why the original stays, and the copy is marked.

**Compared with Jellyfin**

- Jellyfin never reads picture subtitles into text.
