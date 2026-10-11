# Subtitles

This page explains where Polyfin's subtitles come from and how they reach each Jellyfin app: addon subtitles, subtitle files beside local files, text tracks inside files, ASS styles and fonts, and image subtitles.

## Where subtitles come from

Apps get three kinds of subtitles:

- subtitles from the addons;
- the subtitle files beside a local file, for that file's version;
- the text tracks inside a version's file.

All reach each app in the format it takes. Apps can save the user's subtitle language and subtitle mode, which choose the default subtitle track (see [Playback](playback.md#versions-and-analysis)).

## Subtitle files beside local files

A video of a [local folder](local-folders.md) offers the SubRip, ASS, SSA and WebVTT files beside it, named after it, such as `Movie (2001).fr.srt` or `Movie (2001).en.sdh.ass`. [Naming files](local-folders.md#subtitle-files) tells how their names give their language and flags.

- They are subtitles of that file's version only, listed after the addons' subtitles, with their language, title, and whether they are forced, for the hearing impaired or shown by default.
- Each app gets them in the format it asks for, as it gets the addons' subtitles: WebVTT, SubRip, ASS or JSON. An ASS file asked as ASS keeps its styles and positions.
- Files in UTF-8 (with or without a byte order mark) and UTF-16 are read as they are. A file that is not valid UTF-8 is read as Windows-1252, the usual encoding of older subtitles, so that accents survive.
- Polyfin reads them from the folder or the network share when an app asks for them, and keeps their text for a while. A file that changes is read again after the next scan.

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

When an app cannot show image subtitles, Polyfin burns them into the video. This is a video conversion: it counts toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)) and is described in [Transcoding](transcoding.md).
