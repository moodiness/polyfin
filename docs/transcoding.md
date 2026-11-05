# Transcoding

This page covers when Polyfin converts (transcodes) video and audio. It also covers adaptive quality, converted downloads and every setting under **Settings › Conversion (transcoding)**. It is for administrators and for anyone wondering why a title plays converted.

Three topics have their own pages: [Quality of converted video](conversion-quality.md), for its size and bitrate; [GPUs and HDR](gpus-and-hdr.md), for converting on a graphics card and tone mapping; and [Loudness](loudness.md), for how loud converted audio plays.

## When Polyfin converts

Polyfin picks the lightest way an app can play a file:

| Way | When | What Polyfin does |
|---|---|---|
| Direct play | The app supports the file | Redirects the app to the stream, and stays out of the video path |
| Repackage | The app plays the video but not the container | Repackages it into its own on-demand HLS, built on FFmpeg for remote sources |
| Audio conversion | The app cannot take the audio | Converts the audio |
| Video conversion | The app cannot take the video, or must have picture subtitles (PGS, VobSub, DVB) burned in | Converts the video, HDR to SDR included |

A text copy of a PGS or VobSub track spares that conversion once it is ready, with **Turn picture subtitles into text** on. See [Subtitles](subtitles.md).

The audio Polyfin checks is the track the app names. Else it is the one that plays:

1. the track the user's preferences choose;
2. else the one the file flags as default;
3. else the first.

A file whose tracks carry no default flag has its first track checked even when the app names none. jellyfin-web names none for a version not analyzed yet, whose page lists no tracks. An AC-3 track there plays in Chrome converted, with sound.

**Compared with Jellyfin**

- Jellyfin 12.4 checks no audio when the app names no track and none is flagged default. It plays such a file as it is, and a browser then plays the picture without sound ([`StreamBuilder.cs`, lines 675 to 707](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Dlna/StreamBuilder.cs#L675-L707)). See [Versions and analysis](playback.md#versions-and-analysis).

**Where it converts:** Video converts on a GPU when one is available, else on the processor (see [GPUs and HDR](gpus-and-hdr.md#gpu-methods-per-system)). A GPU keeps the source's size, up to 4K; the processor converts to 1080p at most.

**Profiles:** Converted H.264 is in the High profile the master playlist announces, from x264 on the processor as from every GPU. The exception is an app whose profile allows only Main or Baseline (see [What the app allows](conversion-quality.md#what-the-app-allows)). Converted HEVC is in the Main profile. For how apps choose between versions of a title, see [Playback](playback.md).

**Colors:** Converted video keeps the colors its source states: range, matrix, transfer and primaries.

- Polyfin's analysis reads them from the video track or, when only there, from its container, such as Matroska's color information.
- FFmpeg's encoders write the colors the decoded frames carry, and its HEVC decoder knows only those of the video track. So Polyfin tags the converted frames with the source's colors itself.
- A color the source does not state stays unstated, and apps pick their own.
- HDR tone mapped to SDR is tagged BT.709 (see [HDR and tone mapping](gpus-and-hdr.md#hdr-and-tone-mapping)).

**Version names.** With **Say in version names how they play** on, the default, each version's name says which way it takes:

- "(direct play)";
- "(direct stream)" when its tracks are copied into HLS;
- "(converted: reason)";
- "(cannot play here)" when it needs a conversion the server or the user may not have.

See [Version names](versions.md#version-names).

### Direct play turned off by the app

An app may ask for a title again with direct play turned off. The Jellyfin app for Android's integrated player does so after two playback errors within 30 seconds. Polyfin then still checks whether the app would play the file as it is:

- **It would:** the file is repackaged or converted to what the app's conversion formats take, and **Direct play failed** joins the reasons.
  - The reasons show in **Home › Now playing › Details** under **Why**, and as `DirectPlayError` in the `TranscodeReasons` of the `TranscodingUrl`.
  - A version converted so ends its name with "(converted: app’s request)".
  - For instance, the integrated player plays an HEVC Main 10 file with E-AC-3 audio as it is. Asked again, it is converted to H.264 for its MPEG-TS format, with the reasons "Video codec not supported" and "Direct play failed". The codec alone would point at the wrong cause.
- **It would not:** the reasons are those the check finds, such as "File format not supported" for an AVI file, as when direct play is on.

Some plays skip the check: discs and DASH channels, which never play as they are, and DLNA devices and guests through share links, which Polyfin sends a stream itself. Their reasons are those of their conversion alone, or "Direct play failed" when no other reason applies.

The Jellyfin app for Android's integrated player gets its versions described as files, and plays them as they are (see [The integrated player of the Jellyfin app for Android](jellyfin-compatibility.md#the-integrated-player-of-the-jellyfin-app-for-android)).

**Compared with Jellyfin**

- Jellyfin 12.4 runs the check too when the app turns direct play off, and gives the same reasons ([`MediaInfoHelper.cs`, lines 227 to 230](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Helpers/MediaInfoHelper.cs#L227-L230); [`StreamBuilder.cs`, lines 1392 to 1426](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Dlna/StreamBuilder.cs#L1392-L1426)). Version names are Polyfin's own.

### Audio in HLS segments

The audio of a video repackaged or converted into HLS is copied only when the app takes its codec and the HLS segments carry it:

| Segments | Codecs copied |
|---|---|
| MPEG-TS | AAC, AC-3, E-AC-3, MP3 |
| Fragmented MP4 | The same, plus ALAC, FLAC, Opus, DTS and TrueHD |

Another track is converted to a codec the app takes that the segments carry, even when the app's profile names its own codec. jellyfin-web names MP2 for Chrome, which refuses it in both kinds of segments. So in Chrome, the MP2 of TV recordings and channels is converted to AAC, whether the video is copied or converted, and plays at the first try.

- The `TranscodingUrl` writes those codecs alone as `AudioCodec`.
- A stream sent as one file, to an app that asks for one or to a DLNA device, keeps to the app's codecs as they are.

**Compared with Jellyfin**

- Jellyfin keeps the same codecs, in `StreamBuilder.BuildStreamVideoItem`, for HLS only, as Polyfin does.
- Jellyfin's `TranscodingUrl` writes the codec of a copied track alone as `AudioCodec`. Polyfin writes the codecs the segments carry either way, as it does when the track is converted.

### Dolby audio in browsers

Browsers built on Chromium, such as Chrome and Android's web view, and Firefox do not decode AC-3 or E-AC-3 (Dolby Digital and Dolby Digital Plus). jellyfin-web asks the browser what it plays, so in these browsers such a file is not played as it is:

- Polyfin repackages it into HLS.
- It copies the video when the browser takes it, as HEVC with a GPU that decodes it.
- It converts only the audio, to AAC. Nothing is encoded for the video, and the GPU does no work for it.

Edge on Windows and Safari decode both, and play such a file as it is when they take its video and container.

To play it without any conversion in such a browser, use a version with AAC or Opus audio. **Prefer versions the app plays without conversion** picks one first when there is one (see [Prefer versions the app plays without conversion](versions.md#prefer-versions-the-app-plays-without-conversion)).

**Compared with Jellyfin**

- Jellyfin 12.4 decides the same for the same file and browser, with the same reason, `AudioCodecNotSupported` (`StreamBuilder.BuildStreamVideoItem`).

### Two players in the Jellyfin iOS app

The Jellyfin app for iPhone and iPad has two players, and each tells the server something different:

- **Its web player:** the default, is jellyfin-web in Safari's engine. It takes AC-3 and E-AC-3, in files and in both kinds of HLS segments, so their audio is copied.
- **Its native player:** **Use Native Video Player** in the app's settings (beta), declares its own formats.
  - Its HLS format takes AAC, MP3, FLAC and ALAC. After it comes a progressive MP4, taking H.264 with AC-3 and E-AC-3 too.
  - With **Prefer fMP4 in HLS** on, its default, the HLS segments are fragmented MP4 and HEVC is copied into them.
  - With it off, they are MPEG-TS with H.264, so HEVC is converted to H.264.

Polyfin ranks the native player's formats as Jellyfin does. Where the progressive MP4 wins, Polyfin sends what that MP4 would carry over HLS instead, in the segments the app chose. The progressive MP4 wins where it keeps what the HLS format would convert, so AC-3 and E-AC-3 are copied:

| Source | **Prefer fMP4 in HLS** on | Off |
|---|---|---|
| HEVC with E-AC-3, in MKV | HEVC copied, E-AC-3 converted to AAC 5.1 (the HLS format wins, as it copies the video) | H.264, E-AC-3 copied, in MPEG-TS |
| H.264 with E-AC-3 or AC-3, in MKV | Both copied, in fragmented MP4 | Both copied, in MPEG-TS |

- **Downloads:** the native player's plays are never sent as one file, even with **Converted downloads** on. Its downloads ask under another profile name and without HLS, and get a converted MP4 as any app's (see [Converted downloads](#converted-downloads)).
- **MKV:** neither player plays MKV as it is, so an MKV is always repackaged on iPhone and iPad. HEVC is tagged `hvc1`, which Apple's players need. For the fewest conversions, turn **Use Native Video Player** off.

Polyfin never sends the native player a conversion as one file. The native player is Apple's AVPlayer, which cannot play one:

- Sent without byte ranges (`Accept-Ranges: none`), it stops AVPlayer at once with "The server is not correctly configured".
- Sent with byte ranges, AVPlayer reads every fragment before the play starts. A conversion made as it plays would start only once the whole title is converted.

Over HLS, the same tracks start at once. Seeking, the bandwidth in **Now playing** and stopping work as for any HLS play.

**Compared with Jellyfin**

- Jellyfin 12.4 ranks the formats the same way, and sends the native player the progressive MP4 itself ([`StreamBuilder.cs`, lines 874 to 927](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Dlna/StreamBuilder.cs#L874-L927)). Polyfin does not, on purpose.
- Jellyfin sends a conversion without byte ranges, which stops AVPlayer as above ([`FileStreamResponseHelpers.cs`, line 135](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Helpers/FileStreamResponseHelpers.cs#L135)).

API details: [iOS native player profile for apps](jellyfin-api.md#ios-native-player-profile-for-apps).

### Loudness and louder sound

How loud converted audio plays, the stereo mix, evening the loudness and each user's louder sound are on [Loudness](loudness.md).

### Seeing how a play goes

- **In the web player:** "Playback Info", in the player's settings menu, names how the play goes.
  - "Direct playing";
  - "Direct streaming" when the video is copied and the audio converted;
  - "Remuxing" (repackaged) when both are copied into HLS;
  - "Transcoding" when the video is converted.
  
  A play streamed by Polyfin adds a section with the target codecs, "(direct)" beside a copied one, and the reasons, such as "The audio codec is not supported". The section shows once the play's first segment is made. It stays while the encoding waits between requests.
- **In the version's name:** it ends with how it plays, such as "(converted: audio)" (see [Version names](versions.md#version-names)).
- **In the admin app:** **Home › Now playing › Details** shows the encoder, "Copied" for copied video, the audio sent, and why.
- **In the detailed log:** one line for each play (see [Logs](administration.md#logs)). Neither line below names an address.
  - `A version plays as it is`, or `A version plays over HLS` (`A version plays as one stream` for a stream sent as one file). It has `video` (`copied`, or the codec and encoder, such as `h264 with h264_nvenc`), `audio` (`copied`, `none`, or the codec and channels, such as `aac 2 ch`) and `reasons`.
  - Each FFmpeg run then logs `Started a remux`, with `video` (`copy` or the encoder) and `audio` (`copy`, `none` or the encoder).

**Compared with Jellyfin**

- Jellyfin reports the same on the device's session, as `TranscodingInfo`, and clears it when its transcode ends ([TranscodeManager.cs, lines 343 to 366](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.MediaEncoding/Transcoding/TranscodeManager.cs#L343-L366); [SessionManager.cs, lines 2008 to 2023](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Session/SessionManager.cs#L2008-L2023)). Polyfin keeps it until the play stops, as its encodings stop and start again within a play.
- Jellyfin also reports the conversion's frame rate and progress, which Polyfin leaves out. It names its configured hardware acceleration even for copied video; Polyfin names the GPU of a video conversion alone.
- Jellyfin logs its decision at the debug level, with the file's path and URL ([StreamBuilder.cs, lines 833 to 841](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Dlna/StreamBuilder.cs#L833-L841)). Polyfin's line names no address.

### Lower quality during a play

jellyfin-web's player offers **Quality** in its settings menu during a play, to lower the bitrate of a play that stutters. Picking a quality plays the version again from the same position at that bitrate:

- A version whose bitrate is above it is converted down to it (see [Size of converted video](conversion-quality.md#size-of-converted-video)).
- One within it plays as before.
- A version or a channel whose bitrate is not known, as a live HLS stream's, counts as the usual bitrate of its height, the one its conversion gets (see [Bitrate of converted video](conversion-quality.md#bitrate-of-converted-video)). A 1080p channel counts as 10 Mb/s, and converts at 3 Mbps. This holds for every bitrate limit, the app's and the user's **Maximum quality** too.

The menu shows for a version played as it is (a movie, an episode, a recording or a channel), as for one Polyfin repackages or converts. It shows only when a lower quality would play it converted:

- the server converts video (**Conversion (transcoding)**), and the user may have it converted (**Can use conversion (transcoding)**), while the server is not at its **Video conversions at once**;
- FFmpeg can convert its video to a codec the app takes;
- a file is known to be cut into HLS segments (see [HLS segments](#hls-segments)):
  - it is an MPEG-TS file, an MPEG program stream or an AVI file;
  - or its keyframe index or, for a local MPEG-TS file, its keyframes were read. [Prepare playback in advance](reading-the-sources.md#preparing-playback-in-advance) reads it when a title's details open, or an earlier play over HLS did.

A play as it is never waits to read the index. A version whose index is not read yet, such as a torrent's at its first play, plays without the menu. So does one that has no index Polyfin reads, such as a WMV file. A channel's stream is converted as it comes, and needs none.

**Compared with Jellyfin**

- Jellyfin offers the menu whenever the app's profile can convert video for streaming, which jellyfin-web's always can, unless the user may neither convert nor repackage (`MediaInfoHelper.SetDeviceSpecificData`). Polyfin offers it only when a lower quality would play, so that picking one never ends the play.
- Jellyfin never lowers a remote source, such as a channel or a `.strm` stream, to the quality picked. It counts a local file whose bitrate is not known as 40 Mb/s (`StreamBuilder.IsBitrateLimitExceeded`), so its menu does nothing for a channel played as it is. Polyfin lowers any version or channel above the quality picked, counting one whose bitrate is not known as the usual bitrate of its height.

API details: [Lower quality menu for apps](jellyfin-api.md#lower-quality-menu-for-apps).

### Size and quality of converted video

How large converted video is, the bitrate of its video and audio, and what an app's profile allows are on [Quality of converted video](conversion-quality.md).

## Adaptive quality

**Several qualities when converting**, in **Settings › Conversion (transcoding) › Video**, lets an app step down to a lower quality when its connection slows, instead of stopping to load.

| Choice | Lower qualities go to |
|---|---|
| **Off** (default) | No app. Converted video has one quality, the one Polyfin picks (see [Size of converted video](conversion-quality.md#size-of-converted-video)). |
| **Outside the local network** | Apps outside the local network |
| **Always** | Every app |

The local network is judged as for password reset PINs: loopback, private and link-local addresses. Behind a reverse proxy, add the proxy to [Known proxies](reverse-proxy.md#known-proxies), or every app looks local.

- **Qualities:** the first is the one Polyfin picks, at about the source's bitrate (see [Bitrate of converted video](conversion-quality.md#bitrate-of-converted-video)).
  - Up to two lower steps of the ladder come after it: 2160p, 1440p, 1080p, 720p, 540p, 480p, 360p.
  - Each is the tallest whose usual bitrate is at most half that of the quality above, at that bitrate.
  - 1080p at 26 Mb/s or at 10 Mb/s adds 720p at 5 Mb/s and 480p at 2 Mb/s. 720p at 5 Mb/s adds 480p and 360p.
  - Nothing goes below 360p, so a 360p conversion keeps one quality.
  - Each keeps the first's codec, encoder, tone mapping, frame rate, audio and subtitles, at its own size and bitrate. It peaks at one and a half times that bitrate, below the first quality's peak, and so within the limit.
- **Only converted video:** video copied without conversion keeps one quality. Live TV and music keep one quality too. A converted quality beside a copy would cause three problems:
  - a playback that costs nothing would turn into a conversion when the network is at its worst;
  - **Video conversions at once** could refuse it mid-play;
  - one playlist would mix codecs, and HDR with SDR.
- **Cost:** a lower quality is converted only once the app asks for one of its segments, by an FFmpeg of its own, from that segment.
  - The quality the app asks for last runs as with one quality, preparing **Seconds prepared ahead**.
  - While an app switches, as jellyfin-web's player does when it measures each quality, the qualities it asked for in the last 10 seconds keep converting. Each has one FFmpeg, so switching back does not start over.
  - Another quality the app has not asked for in 10 seconds stops: the extra cost of switching lasts only that long.
  - The segments already made stay until the playback ends.
  - All the qualities count as one playback toward **Video conversions at once** (see [Video conversions at once](#video-conversions-at-once)).
- **Home:** a playback's **Details** show the **Quality played** among those offered. The **Smaller than the source** row says when the app stepped down from the first.

Apps that do not switch play the first quality, as with the setting off. Apps that do:

- jellyfin-web plays HLS through hls.js, which picks among the qualities itself;
- Safari and iOS use their own HLS player, which does too;
- Jellyfin Android TV's player adapts to the bandwidth.

**Compared with Jellyfin**

- Jellyfin adds two lower-bitrate qualities only when the app asks with `EnableAdaptiveBitrateStreaming`, off by default, and never on the local network. Polyfin ignores that parameter and follows the setting.

API details: [Adaptive quality playlists for apps](jellyfin-api.md#adaptive-quality-playlists-for-apps).

## Turning conversion and downloads on or off

The server has one switch, **Conversion (transcoding)**, in **Settings › Conversion (transcoding) › General**. With it off, Polyfin never re-encodes video or audio, and never burns subtitles in.

- Apps play files as they are, or repackaged into HLS with their tracks copied.
- Polyfin picks the next version that plays that way.
- A title with no version that plays that way on an app does not start on that app. An example is a recording converted to HEVC, on an app that does not play HEVC (see [Smaller recordings](recordings.md#smaller-recordings)).
- Picture subtitles the app cannot show are left out, except their text copies (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).

Each user also has two permissions, in the **Access** section of their page under **Users**, both on by default:

| Permission | Needed for |
|---|---|
| **Can use conversion (transcoding)** | Conversion, together with the server's switch |
| **Can download** | Downloads, on its own |

- **Turn off downloads for everyone**, on the **Users** page, takes downloads away from every user and every user group at once. It also applies in the apps already signed in.
- A user group can hold both permissions for its members (see [User groups](users.md#user-groups)). See [Users](users.md).

**Compared with Jellyfin**

- The per-user permissions are on by default, as in Jellyfin.
- A title that cannot play without conversion fails as Jellyfin's `NoCompatibleStream`.

API details: [Conversion permissions for apps](jellyfin-api.md#conversion-permissions-for-apps).

## Video conversions at once

**Video conversions at once (0 = no limit)**, in **Settings › Conversion (transcoding) › General**, caps the playbacks whose video the server converts. It is 0, no limit, by default, and goes up to 32.

- **What counts:** conversions of files and Live TV together, burning subtitles in included.
- **Converted downloads:** those whose video is converted count from their start to their end (see [Converted downloads](#converted-downloads)). Each counts on its own, even next to the user's playback of the same title, as it runs a conversion of its own.
- **Live TV:** it keeps its own caps on top, 4 channels per user and 16 for the server. See [Live TV](live-tv.md).
- **One playback:** what a user plays of a version, from its first converted segment until the app stops it or asks for nothing for 3 minutes (1 minute for Live TV). Its next segments, its seeks, and the new play session apps open to switch tracks never count again.
- **Several qualities:** the lower qualities of a playback with **Several qualities when converting** count with it, as one playback. An app stepping down at the cap is never refused. While an app switches, the qualities it asked for in the last 10 seconds keep converting, so that switching back does not start over. That costs more of the machine for those seconds only, never another place under the cap. See [Adaptive quality](#adaptive-quality).
- **Kept video:** with **Keep the converted video when the audio track changes**, a playback whose session stopped keeps its converted video 30 seconds, for the next session to take over. It counts as one playback meanwhile, then with that session once it does. See [Changing the audio track](#changing-the-audio-track).
- **Recordings:** a recording converted to HEVC in the background counts as one conversion while it runs. It starts only below the cap, and waits while playbacks fill it. See [Smaller recordings](recordings.md#smaller-recordings).
- **At the cap:** Polyfin plans no new video conversion, as when conversion is off. It picks the next version that plays without one, or else reports that no version is compatible.

**Compared with Jellyfin**

- Jellyfin has no such cap. Its nearest, a tuner's stream limit, fails with a `500`, which apps take the same way, as a server error.

API details: [Conversion cap answers for apps](jellyfin-api.md#conversion-cap-answers-for-apps).

## Converted downloads

Apps that keep titles for offline viewing download a version's file as it is. **Converted downloads**, in **Settings › Conversion (transcoding) › General**, changes that. It is off by default.

With it on, an app that asks for a smaller copy gets one, as Streamyfin's download sheet does with the quality the user picks:

- **Video:** copied when the app plays it and it fits the quality picked, else converted.
  - It stays within the same limits as any converted video (see [Size of converted video](conversion-quality.md#size-of-converted-video)): the quality picked, the user's **Maximum quality** and quality group, and **Maximum quality of converted video**.
  - A version taller than the user's quality group downloads converted down to it, where its file would be refused.
- **Audio:** copied when the app takes its codec and, when the video is converted, when it fits the audio's share of the quality picked (see [Audio bitrate](conversion-quality.md#audio-bitrate)). Else it is converted to one the app takes.
- **Subtitles:** a picture subtitle the app chose is burned into the video. Text subtitles stay files the app saves beside it (see [Subtitles](subtitles.md)).
- **Made on the fly:** FFmpeg makes the copy as the app downloads it, at the app's pace, and Polyfin keeps nothing on disk. The file's size is known only at its end, so an interrupted download starts again from the beginning.
- **Permissions:** the user needs **Can download**. When the video or the audio is converted, they also need **Can use conversion (transcoding)**, with the server's **Conversion (transcoding)** switch on, as for a playback. Without them, the app downloads the file as it is, if the user may download.
- **Cost:** while it runs, a converted download counts toward **Video conversions at once** when it converts the video (see [Video conversions at once](#video-conversions-at-once)).
  - It counts on its own, even next to the user's playback of the same title.
  - It also counts as one of the user's **Playbacks at once** on its device.
  - At either limit, no converted download is planned, as no conversion is for a playback.
  - One that starts past the cap anyway, planned at the same time as another, is refused with a `503`.
- **Home:** a converted download shows in the admin app's **Now playing** with a "Downloading" label. Its bar tells how far the copy is made.

**Compared with Jellyfin**

- Jellyfin gives such apps the same kind of `TranscodingUrl`, with no setting to turn it off.
- Jellyfin does not count downloads toward a user's playbacks, and has no quality groups.

API details: [Converted download streams for apps](jellyfin-api.md#converted-download-streams-for-apps).

## HLS segments

Repackaged and converted plays of files are cut into HLS segments on the source's own keyframes.

- **Segment length:** the first segment ends on the first keyframe at least 2 seconds in. Each next one ends on the first keyframe at least 4 seconds after its start. A file with a keyframe every 2 seconds plays its first segment after 2 seconds of picture; one with a keyframe every 10 seconds gets 10-second segments.
- **Local MPEG-TS files** (`.ts`, `.m2ts`, `.mts`, `.m2t`, `.tp`, `.trp`) have no keyframe index. Polyfin reads their keyframes once, in the background when the title's details open (see [Prepare playback in advance](reading-the-sources.md#preparing-playback-in-advance)), else at the first play over HLS.
  - That first play waits 5 seconds at most for the read, about what a recording of an hour or two takes on a local SSD.
  - A play before the read is done plays converted, its version named "(converted)" meanwhile. It keeps that cut to its end, while the read goes on, 3 minutes at most.
  - Once read, the file's video is copied when the app takes its codec, and a seek lands where asked. A user who may not have video converted then plays it too.
  - A file whose keyframes cannot be read within 3 minutes, or are unusable, plays converted and is not read again for 6 hours.
- **Files without an index Polyfin reads:** they are converted and cut every 2, then 4 seconds, even when the app takes their video. A seek lands where asked, however far apart their keyframes are. These are an addon's MPEG-TS file, an MPEG program stream (`.mpg`, `.mpeg`, `.vob`) or an AVI file. A user who may not have video converted is not offered such a file over HLS, and its version's name says so (see [Version names](versions.md#version-names)). Apps that play the file as it is, such as Android TV and Swiftfin for MPEG-TS, need no segments (see [Video files](naming-and-matching.md#video-files)).
- **Discs:** a Blu-ray disc is cut on the keyframes its clips' information files list, and its video copied when the app takes it. A DVD is converted and cut every few seconds, as an MPEG-TS file is. See [Local folders](discs.md#blu-ray-discs).
- **Prepared ahead:** Polyfin makes segments up to **Seconds prepared ahead** past the last one the app asked for, then waits for the app.
- **A stalled source:** after 20 seconds without data, the app gets `503` at once rather than waiting minutes. Its next request waits for the same conversion, which goes on when the source answers again. A source that fails for good reads on from a mirror, or else from another version of the title (see [Carrying a play on](reading-the-sources.md#carrying-a-play-on)). When neither can, the conversion ends, and the next request answers `503` too.
- **Stopping:** repackaging or conversion stops when the app stops its play session, or once the app has asked nothing for 3 minutes, as when a player stays paused. It starts again at the next request.
- **Another title:** when a device starts another title without stopping the one before, the conversions of the title it left stop. They no longer read their source, a torrent's swarm included (see [Torrents](torrents.md#only-while-playing)). The same title played again, as for another audio track, stops nothing. The user's other devices, DLNA devices, guests and converted downloads go on.

**Compared with Jellyfin**

- Jellyfin cuts files other than Matroska into segments of approximate length, and copies the video of an MPEG-TS file or an MPEG program stream when the app takes its codec ([DynamicHlsPlaylistGenerator.cs, lines 37 to 47](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs#L37-L47); [`DynamicHlsController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/DynamicHlsController.cs#L1853-L1866)). Polyfin's segments are exact, which, without an index, needs the video converted. Both convert the H.264 of an AVI file ([EncodingHelper.cs, lines 2630 to 2636](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs#L2630-L2636)), and the MPEG-2 and MPEG-4 Part 2 video no browser plays. The full comparison is in [HLS segment cutting for apps](jellyfin-api.md#hls-segment-cutting-for-apps).
- Jellyfin stops repackaging or a conversion one minute after its last request, and never because its device started another title. Polyfin keeps a paused conversion 3 minutes, and stops that of a title its device left at once.

API details: [HLS segment cutting for apps](jellyfin-api.md#hls-segment-cutting-for-apps).

### A faster start for remote sources

A play shows its first picture once its first segment is made, and a segment ends on a keyframe. From an addon's link or a torrent with a keyframe every 10 seconds, Polyfin first reads 10 seconds of the file. It reads them at the file's bitrate, from a host that may send it barely faster than it plays.

**Start remote plays faster**, in **Settings › Conversion (transcoding) › Performance**, is off by default. With it on:

- **Which plays:** a repackaged or converted HLS play of an addon's link or a torrent, whose segment where it starts lasts more than 4 seconds. That is at the start, or where it resumes, as PlaybackInfo's `StartTimeTicks` tells. Local folders' files, recordings and Live TV are left out.
- **Fast segments:** that segment is cut into 2-second segments, its last part up to 4 seconds long.
  - They are converted quickly (x264's `ultrafast`, or the GPU's fastest preset), by bitrate, at 720p at most and 5 Mb/s at most.
  - They are in the codec the app gets: the conversion's own, or the copied video's H.264 or HEVC.
  - The first is ready once 2 seconds of the file are read and converted.
  - The play then goes on as without the setting, copied or at full quality, from the keyframe the next segment starts on. One FFmpeg makes the fast segments, and hands over to another there.
- **Apps:** in MPEG-TS segments, which carry what decodes them, every app, as when a play goes on from another version (see [Carrying a play on](reading-the-sources.md#carrying-a-play-on)). In fragmented MP4, jellyfin-web only, whose players load each initialization segment. Other apps in fragmented MP4 start as without the setting.
- **Left as they were:**
  - copied video that is HDR, 10-bit, or neither H.264 nor HEVC, which fast segments would show differently;
  - converted video kept with **Keep the converted video when the audio track changes**, which a new session takes over with its segments;
  - a user who may not have video converted.
- **Changes:** a setting changed applies to the next plays. A play keeps the segments its PlaybackInfo gave it.
- **Seeks:** a seek inside a play gets no fast segments, as the app loaded the playlist once. Apps that start a new play to change the audio track, as jellyfin-web does, get them where the new play starts.
- **Conversions at once:** the fast segments of a repackaged play do not count toward **Video conversions at once**: they last a few seconds at 720p.

Measured on a 1080p file of about 7 Mb/s with a keyframe every 10 seconds, from a host sending 1 MB/s per connection. Times run from PlaybackInfo to the first segment played, median of 3 plays, each with an empty source cache:

| Play | Off | On |
|---|---|---|
| Repackaged into MPEG-TS (Android TV's profile), from the start | 9.9 s | 3.9 s |
| Repackaged, resumed 25 seconds in | 10.8 s | 7.7 s |
| Converted into fragmented MP4 (jellyfin-web's profile), from the start | 7.2 s | 3.9 s |
| Converted, resumed 25 seconds in | 14.5 s | 7.4 s |

**Compared with Jellyfin**

- Jellyfin has no faster start: its first segment is made at full quality, as long as the others.

API details: [Fast start segments for apps](jellyfin-api.md#fast-start-segments-for-apps).

### Changing the audio track

Apps change the audio track of a converted video by playing it again, from the same position. By default, the new play converts the video again from the keyframe before the position, though the picture has not changed. The segments already made are deleted.

**Keep the converted video when the audio track changes**, in **Settings › Conversion (transcoding) › Performance**, is off by default. With it on:

- **Video and audio apart:** the video and the audio track are converted separately, and joined in each segment as it is served. Apps see the same stream as before.
- **Kept video:** once its play session stops, the converted video is kept 30 seconds. A new play session asking for the same video takes it over, with the segments made. The first segment after the change comes once its audio is made, without converting the picture again.
- **Same video:** the video is known by its user, device, version, segment format and conversion (codec, size, bitrate, tone mapping, burned subtitle, quality), not by its audio track.
- **Bounds:**
  - one kept video per user, device and version: another quality or burned subtitle converts anew, and ends the video kept there;
  - a play session that did not stop is never taken over, so two players of the same device each convert their own;
  - kept video counts once toward **Video conversions at once** (see [Video conversions at once](#video-conversions-at-once));
  - kept video no session takes over within 30 seconds stops, and its segments are removed;
  - copied video (DirectStream), Live TV and [converted downloads](#converted-downloads) start again as before.

**Compared with Jellyfin**

- Jellyfin converts the video again whenever the audio track changes, as Polyfin does with the setting off.

API details: [Audio track switching for apps](jellyfin-api.md#audio-track-switching-for-apps).

## Conversion settings

**Settings › Conversion (transcoding)** tunes how video and audio are converted, for files and Live TV alike. Its groups follow below in the admin app's order. A change applies to the next playback, without a restart.

**Recordings:** they copy the stream, and are converted like any file when played (see [Live TV](live-tv.md)). Finished recordings can also be converted to HEVC at night, to take less space, with **Convert finished recordings to HEVC** in **Settings › Recordings** (see [Smaller recordings](recordings.md#smaller-recordings)). Those conversions:

- use the **Graphics card used to convert**, the **Encoding speed**, the **HEVC quality** (28 when it is 0), the **Deinterlacing method** and **Double the frame rate**;
- never tone map;
- count each as one of the **Video conversions at once**, and wait while playbacks fill them.

### General

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Conversion (transcoding)** | **Settings › Conversion (transcoding) › General** | On | The server's switch for every conversion. See [Turning conversion and downloads on or off](#turning-conversion-and-downloads-on-or-off). |
| **Video conversions at once (0 = no limit)** | **Settings › Conversion (transcoding) › General** | 0, no limit (up to 32) | How many playbacks may have their video converted at the same time. See [Video conversions at once](#video-conversions-at-once). |
| **Maximum quality of converted video** | **Settings › Conversion (transcoding) › General** | **Original** (or 480p to 2160p) | The tallest picture converted video gets. See [Maximum quality of converted video](conversion-quality.md#maximum-quality-of-converted-video). |
| **Converted downloads** | **Settings › Conversion (transcoding) › General** | Off | See [Converted downloads](#converted-downloads). |
| **Conversion folder** | **Settings › Conversion (transcoding) › General** | Empty, `cache` in the data folder | The folder the segments of files converted or repackaged are written to, from the next start on. See [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache). |

### Picture subtitles

**Turn picture subtitles into text**, **Picture subtitle tracks read at once** and **Download the data of missing languages automatically**, in **Settings › Conversion (transcoding) › Picture subtitles**, are described in [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text).

### Graphics card

**Graphics card used to convert**, **Graphics card for VAAPI**, **Detected on this server** and **Read these formats on the graphics card**, in **Settings › Conversion (transcoding) › Graphics card**, are described in [Graphics card settings](gpus-and-hdr.md#graphics-card-settings).

<a id="video-encoding"></a>

### Video

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Encoding speed** | **Settings › Conversion (transcoding) › Video** | **Automatic** | **Automatic** keeps Polyfin's speeds. The other choices, **Very slow (best picture)** to **Ultra fast (lightest work)**, are x264's presets, mapped to each encoder below. |
| **H.264 quality (0 = by bitrate)** and **HEVC quality (0 = by bitrate)** | **Settings › Conversion (transcoding) › Video** | 0 (range 1 to 51) | 0 aims for the bitrate alone. A number is a quality factor that the video's peak caps (see [Bitrate of converted video](conversion-quality.md#bitrate-of-converted-video)). Recordings converted to HEVC use it with every encoder. |
| **Allow converting to HEVC** | **Settings › Conversion (transcoding) › Video** | Off | On: apps that list HEVC before H.264 get HEVC. Off: H.264 comes first, and only apps that take no H.264 get HEVC. |
| **Several qualities when converting** | **Settings › Conversion (transcoding) › Video** | Off | **Off**, **Outside the local network** or **Always**: converted video also offers up to two lower qualities that apps step down to on a slow connection. See [Adaptive quality](#adaptive-quality). |

How each encoder takes **Encoding speed** and the quality number:

| Encoder | **Automatic** speed | Speed choices | Quality number |
|---|---|---|---|
| x264 and x265 | `veryfast` | x264's presets | CRF |
| NVENC | `p4` | `p7` to `p1` | CQ |
| VAAPI | The encoder's own | Compression levels 1 to 7 | QVBR's quality, for the drivers that have it; other drivers ignore it |
| QSV | The encoder's own | Presets of the same names (`veryfast` for the two fastest) | Ignored |
| AMF | The encoder's own | `quality`, `balanced` and `speed` | Ignored |
| VideoToolbox | The encoder's own | Speed priority, off from **Slow** down, on from **Fast** up | Its quality from 1 to 100, as 100 minus twice the number |

The VAAPI drivers without QVBR, QSV and AMF ignore the quality number because their quality modes leave the maximum bitrate out.

### HDR

**Convert HDR to SDR (tone mapping)**, **Tone mapping method**, **Tone map HDR on the graphics card**, **Maximum quality of HDR converted by the processor**, **Peak brightness in nits (0 = from the video)** and **Highlight desaturation (0 = off)**, in **Settings › Conversion (transcoding) › HDR**, are described in [HDR settings](gpus-and-hdr.md#hdr-settings).

### Interlaced video

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Deinterlacing method** | **Settings › Conversion (transcoding) › Interlaced video** | Yadif | Yadif or Bwdif. See the list below for each GPU. |
| **Double the frame rate** | **Settings › Conversion (transcoding) › Interlaced video** | Off | Makes a frame of each field of video up to 30 frames a second. |

- An NVIDIA GPU keeping the frames uses the method chosen.
- VideoToolbox uses Yadif; Bwdif goes through the processor there.
- An AMD or Intel GPU keeping the frames uses its own deinterlacer, whatever the method.
- AMF has none: its interlaced video is deinterlaced on the processor.

### Audio

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Mix to stereo** | **Settings › Conversion (transcoding) › Audio** | FFmpeg's own | Or Jellyfin's Dave750, Night mode, RFC 7845 and AC-4, for the layouts Jellyfin lists for each, 5.1 and 7.1 among them. See [Loudness](loudness.md#mixing-to-stereo). |
| **Volume when mixing to stereo** | **Settings › Conversion (transcoding) › Audio** | 2, as in Jellyfin (range 0.5 to 3) | Volume of the stereo mix, 1 changing nothing. Above 1, a limiter follows it. See [Loudness](loudness.md#mixing-to-stereo). |
| **Even the loudness of converted videos** | **Settings › Conversion (transcoding) › Audio** | Off | Brings the audio of the videos and channels the server converts to -16 LUFS, then limits it. Direct play, copied audio and music never change. See [Loudness](loudness.md#evening-the-loudness). |
| **Dynamic range of converted Dolby audio** | **Settings › Conversion (transcoding) › Audio** | **As encoded** | **None (full range)** or **Night (quiet passages louder)**: how converted AC-3 and E-AC-3 apply the compression their tracks carry. See [Loudness](loudness.md#dynamic-range-of-dolby-audio). |
| **Most audio channels** | **Settings › Conversion (transcoding) › Audio** | As many as the app takes | Or mono, stereo or 5.1. Shapes converted audio. |
| **Audio bitrate per channel in kb/s (0 = automatic)** | **Settings › Conversion (transcoding) › Audio** | 0 (range 32 to 320 kb/s) | 0 gives Jellyfin's bitrates, less under a low bitrate limit (see [Audio bitrate](conversion-quality.md#audio-bitrate)). A number gives that much per channel, whatever the limit. Shapes converted audio. |

### Performance

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Processor threads per conversion (0 = automatic)** | **Settings › Conversion (transcoding) › Performance** | 0, FFmpeg's choice (up to 64) | Number of encoding threads. |
| **Seconds prepared ahead** | **Settings › Conversion (transcoding) › Performance** | 120 (range 30 to 600) | How many seconds of picture repackaging or converting a file makes past the last segment the app asked for, before it waits. |
| **Keep the converted video when the audio track changes** | **Settings › Conversion (transcoding) › Performance** | Off | Keeps converted video 30 seconds once its play session stops, for a new session with another audio track to take over. See [Changing the audio track](#changing-the-audio-track). |
| **Start remote plays faster** | **Settings › Conversion (transcoding) › Performance** | Off | Starts plays of addon links and torrents whose first segment is long with quick 2-second segments, at 720p at most. See [A faster start for remote sources](#a-faster-start-for-remote-sources). |

**Compared with Jellyfin**

- **Settings › Conversion (transcoding)** works as Jellyfin's Transcoding page does.
- Polyfin maps encoding speeds to NVENC and VAAPI as Jellyfin does. Jellyfin sets VAAPI compression levels on Intel drivers only.
- Jellyfin's quality defaults are CRF 23 (H.264) and 28 (HEVC), and only for x264 and x265.
- **Allow converting to HEVC** matches Jellyfin's **Allow encoding in HEVC format**. With it off, Polyfin behaves as Jellyfin does.
- **Double the frame rate** works as in Jellyfin.
- Jellyfin's stereo mix volume is 2 by default too, without a limiter.
- Jellyfin has no **Most audio channels**, **Audio bitrate per channel**, **Even the loudness of converted videos** or **Dynamic range of converted Dolby audio** setting.
- **Processor threads per conversion** is Jellyfin's encoding thread count.
- Jellyfin also throttles by seconds, but not by default.

<div class="moved-sections">

<a id="audio-bitrate"></a>*Audio bitrate* moved to [Quality of converted video](conversion-quality.md#audio-bitrate).

<a id="bitrate-of-converted-video"></a>*Bitrate of converted video* moved to [Quality of converted video](conversion-quality.md#bitrate-of-converted-video).

<a id="size-of-converted-video"></a>*Size of converted video* moved to [Quality of converted video](conversion-quality.md#size-of-converted-video).

<a id="a-gpu-too-slow-for-hdr"></a>*A GPU too slow for HDR* moved to [GPUs and HDR](gpus-and-hdr.md#a-gpu-too-slow-for-hdr).

<a id="gpu-methods-per-system"></a>*GPU methods per system* moved to [GPUs and HDR](gpus-and-hdr.md#gpu-methods-per-system).

<a id="hdr-and-tone-mapping"></a>*HDR and tone mapping* moved to [GPUs and HDR](gpus-and-hdr.md#hdr-and-tone-mapping).

<a id="nvidia-driver"></a>*NVIDIA driver* moved to [GPUs and HDR](gpus-and-hdr.md#nvidia-driver).

<a id="loudness"></a>*Loudness* moved to [Loudness](loudness.md#loudness).

</div>
