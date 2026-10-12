# Subtitles

This page explains where Polyfin's subtitles come from and how they reach each Jellyfin app: addon subtitles and how they are synced to the version played, subtitle files beside local files, text tracks inside files, ASS styles and fonts, and image subtitles.

## Where subtitles come from

Apps get three kinds of subtitles:

- subtitles from the addons;
- the subtitle files beside a local file, for that file's version;
- the text tracks inside a version's file.

All reach each app in the format it takes. Apps can save the user's subtitle language and subtitle mode, which choose the default subtitle track (see [Playback](playback.md#versions-and-analysis)).

## Addon subtitles synced to the version

Addons find subtitles for a title, written for one release of it, not always the version played: another cut, another frame rate, a longer opening. They then run early or late, by a few seconds, or by more and more as the version plays. Polyfin syncs them to the version played.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Sync addon subtitles** | **Settings › Playback** | On | Finds when each version played has speech, and syncs the addons' subtitles to it. Off, subtitles come with the times the addons give, and no speech is looked for. |

### How Polyfin measures

Polyfin compares a subtitle's lines with when the version itself has speech:

- A text track inside the file, in any language, when there is one: its lines are timed for that file. A forced track comes after the others, and a track needs at least 20 lines. It must be read whole: through the file's index (see [Text tracks inside files](#text-tracks-inside-files)), or by a remux that extracted it.
- Else the version's audio: FFmpeg decodes its first audio track, and Polyfin measures the energy of the voice band, 300 to 3000 Hz, as subtitle aligners do. Speech is where it is loud compared with the quiet of the stretch and rises and falls with syllables, a few times a second, where music and effects hold; it is joined over short pauses. Polyfin listens to three stretches of up to 3 minutes, around a quarter, half and three quarters of the version, joined when the version is too short for them to be apart.

Polyfin finds this once per version, in the background, at the version's first play (when the app reports that a movie or an episode starts), and keeps it in the database:

- One version is looked at at a time, and up to 8 wait. A version whose source fails is tried again at a play 15 minutes later.
- An addon's file is read for at most 300 MB. Its three stretches last as long as that reads at the file's bitrate, from its size and length, up to 3 minutes each: a 12 Mbit/s file gets about 45 seconds each. Each stretch counts a tenth more, 10 seconds before it, where FFmpeg starts at a keyframe, and 8 MB that FFmpeg is sent beyond what it decodes. A file whose stretches would last under 30 seconds each, above about 16 Mbit/s, is not read, and a read that goes past 300 MB stops: the version then has no speech reference, and its subtitles come as they are. Shorter stretches hold fewer lines: a subtitle with less than 20 seconds of them in the stretches is left as it is.
- The files of local folders get stretches of 3 minutes, without a limit.
- A version that is not one file, such as a DVD, and a replay, which would hold one of its provider's connections, are not looked at.

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

## Subtitle files beside local files

A video of a [local folder](local-folders.md) offers the SubRip, ASS, SSA and WebVTT files beside it, named after it, such as `Movie (2001).fr.srt` or `Movie (2001).en.sdh.ass`, and its picture subtitle files: PGS files (`.sup`) and VobSub pairs (`.idx` and `.sub`). [Naming files](local-folders.md#subtitle-files) tells how their names give their language and flags.

- They are subtitles of that file's version only, listed after the addons' subtitles, with their language, title, and whether they are forced, for the hearing impaired or shown by default.
- Each app gets them in the format it asks for, as it gets the addons' subtitles: WebVTT, SubRip, ASS or JSON. An ASS file asked as ASS keeps its styles and positions.
- Files in UTF-8 (with or without a byte order mark) and UTF-16 are read as they are. A file that is not valid UTF-8 is read as Windows-1252, the usual encoding of older subtitles, so that accents survive.
- Polyfin reads them from the folder or the network share when an app asks for them, and keeps their text for a while. A file that changes is read again after the next scan.
- Picture subtitle files are listed with the codec Jellyfin gives them, `PGSSUB` or `DVDSUB`. They are never converted to text. An app that draws PGS, such as jellyfin-web, gets the `.sup` file as it is, read by byte ranges. A VobSub pair is not one file: like Jellyfin 12.2, which gives VobSub as a file only extracted as Matroska, Polyfin burns it into the video, and so it does a PGS file for an app that does not draw PGS (see [Image subtitles](#image-subtitles)).

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
