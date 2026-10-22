# Transcoding

This page covers when Polyfin converts (transcodes) video and audio, how to give it a GPU for that, how HDR is tone mapped, and every setting under **Settings › Conversion**.

## When Polyfin converts

Polyfin picks the lightest way an app can play a file:

- **Direct play:** when the app supports the file, Polyfin redirects the app to the stream and stays out of the video path.
- **Remux:** when the app can play the video but not the container, Polyfin repackages it into its own on-demand HLS, built on FFmpeg for remote sources.
- **Audio conversion:** when the app cannot take the audio, Polyfin converts the audio.
- **Video conversion:** when the app cannot take the video, or must have image subtitles (PGS, VobSub, DVB) burned in, Polyfin converts the video, HDR to SDR included. A text copy of a PGS or VobSub track, with **Turn picture subtitles into text** on, spares that conversion once it is ready. See [Subtitles](subtitles.md).

Video converts on a GPU when one is available, with the methods of the system Polyfin runs on (see [GPU methods per system](#gpu-methods-per-system)): an NVIDIA GPU through NVENC, or an AMD or Intel GPU through VAAPI, on Linux. Otherwise it converts on the processor. A GPU keeps the source's size, up to 4K; the processor converts to 1080p at most. Converted H.264 is in the High profile the master playlist announces, from x264 on the processor as from every GPU; converted HEVC is in the Main profile. For how apps choose between versions of a title, see [Playback](playback.md).

Converted video keeps the colors its source states: range, matrix, transfer and primaries, as Polyfin's analysis reads them, from the video track or, when only there, from its container, such as Matroska's color information. FFmpeg's encoders write the colors the decoded frames carry, and its HEVC decoder knows only those of the video track, so Polyfin tags the converted frames with the source's itself. A color the source does not state stays unstated, and apps pick their own. HDR tone mapped to SDR is tagged BT.709, by the GPU's or the processor's tone mapping.

With **Say in version names how they play** on, the default, apps see in each version's name which of these ways it takes: "(direct play)", "(direct stream)" when its tracks are copied into HLS, "(converted: reason)", or "(cannot play here)" when it needs a conversion the server or the user may not have. See [Version names](playback.md#version-names).

## Size of converted video

Converted video is never larger than its source. It is smaller when one of these limits is below the source's size, the lowest one winning:

- **GPU:** up to 4K.
- **Processor:** up to 1080p.
- **HDR converted by the processor:** up to **Maximum quality of HDR converted by the processor**, 720p or 1080p with **Automatic**, when no GPU tone maps it (see [HDR and tone mapping](#hdr-and-tone-mapping)).
- **Quality group:** the user's, set on their page under **Users**.
- **Maximum quality of converted video:** under **Settings › Conversion**.
- **Bitrate:** the bitrate limit leaves too little for a taller picture. The limit is the app's, from its quality setting, or the user's **Maximum quality** when that is lower. The video gets the limit less the bitrate of the audio sent: the converted audio's, 192 kb/s in stereo by default (see [Audio](#audio)), or the track's own when it is copied. A lossless track of several Mb/s converted to stereo AAC thus leaves 1080p 9.8 Mb/s within 10 Mb/s. The `TranscodingUrl` writes the two as `VideoBitrate` and `AudioBitrate`.

When two limits are equal, the user's quality group is named first, then **Maximum quality of converted video**, then the GPU or the processor. The bitrate is named only when it allows less than all of them.

On Home in the admin app, a playback's **Details** show which one limited the size, in the **Smaller than the source** row: for instance "720p at most: HDR converted by the processor", "Quality group: 720p" or "Bitrate allowed by the app: 5.6 Mbps". The row does not show when the video keeps the source's size.

Jellyfin takes off the limit its own estimate of the audio: 384 kb/s for stereo AAC and 640 kb/s for 5.1 when it converts, or the track's bitrate when it copies it, each capped by a share of the limit (1.5 Mb/s within 10 Mb/s). Polyfin takes off what it sends, so its video gets about 200 kb/s more when the audio is converted to stereo; a copied track above that cap leaves its video less than in Jellyfin.

## Adaptive quality

With **Several qualities when converting** under **Settings › Conversion**, converted video offers apps lower qualities to step down to when their connection slows, instead of stopping to load. It is **Off** by default: one quality, the one Polyfin picks (see [Size of converted video](#size-of-converted-video)). **Outside the local network** offers lower qualities only to apps outside the local network, judged as for password reset PINs: loopback, private and link-local addresses, through **Known proxies** (see [Reverse proxy](reverse-proxy.md#known-proxies)). **Always** offers them to every app.

- **Qualities:** the first is the one Polyfin picks. After it come up to two lower steps of the ladder (2160p, 1440p, 1080p, 720p, 540p, 480p, 360p), each the tallest whose bitrate is at most half that of the quality above. 1080p at 10 Mb/s adds 720p at 5 Mb/s and 480p at 2 Mb/s; 720p at 5 Mb/s adds 480p and 360p. Nothing goes below 360p, so a 360p conversion keeps one quality. Each keeps the first's codec, encoder, tone mapping, frame rate, audio and subtitles, at its own size and bitrate.
- **Only converted video:** video copied without conversion keeps one quality. A converted quality beside a copy would turn a playback that costs nothing into a conversion when the network is at its worst, which **Video conversions at once** could refuse mid-play, and would mix codecs, and HDR with SDR, in one playlist. Live TV and music keep one quality too.
- **Cost:** a lower quality is converted only once the app asks for one of its segments, by an FFmpeg of its own, from that segment; the quality the app asks for last runs as with one quality, preparing **Seconds prepared ahead**. While an app switches, as jellyfin-web's player does when it measures each quality, the qualities it asked for in the last 10 seconds keep converting, each with one FFmpeg, so that switching back does not start over. Another quality the app has not asked for in 10 seconds stops: the extra cost of switching lasts only that long. The segments already made stay until the playback ends. All the qualities count as one playback toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)).
- **Home:** a playback's **Details** show the **Quality played** among those offered, and the **Smaller than the source** row says when the app stepped down from the first.

Apps that do not switch play the first quality, as with the setting off. jellyfin-web plays HLS through hls.js, which picks among the qualities itself; Safari and iOS use their own HLS player, which does too, and Jellyfin Android TV's player adapts to the bandwidth.

**Compared with Jellyfin:**

- Jellyfin adds two lower-bitrate qualities only when the app asks with `EnableAdaptiveBitrateStreaming`, off by default, and never on the local network. Polyfin ignores that parameter and follows the setting.

**For app developers:**

- The master playlist lists the first quality first, then the lower ones, each with its own `BANDWIDTH`, `RESOLUTION` and `CODECS`, and says `#EXT-X-INDEPENDENT-SEGMENTS`. Every quality is cut on the same plan, with a keyframe forced at each segment's start, so segment n covers the same time in each, and an app may switch at any segment.
- The first quality's media playlist is `main.m3u8`, its segments under `hls1/main/`, as before. The lower ones' are `hls1/main1/main.m3u8` and `hls1/main2/main.m3u8`, their segments beside them under `hls1/main1/` and `hls1/main2/`, in the same play session and with the same query.
- Asking for one quality never stops another: an app may ask for the same segment of two qualities at once, and gets both.

## Turning conversion and downloads on or off

The server has one switch, **Conversion (transcoding)** under **Settings › Conversion**. With it off, Polyfin never re-encodes video or audio, and never burns subtitles in. Apps play files as they are, or remuxed into HLS with their tracks copied, and Polyfin picks the next version that plays that way. A title with no version that plays that way on an app does not start on that app, as a recording converted to HEVC on an app that does not play HEVC (see [Smaller recordings](live-tv.md#smaller-recordings)). Image subtitles the app cannot show are left out, but for the text copies of picture subtitles (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).

On a user's page under **Users**, the **Access** section holds each user's own **Can use conversion (transcoding)** and **Can download** permissions, on by default. Conversion needs both the server's switch and the user's permission; downloads need the user's permission only. **Turn off downloads for everyone**, on the **Users** page, takes it away from every user and every user group at once, also in the apps already signed in. A user group can hold both permissions for its members (see [User groups](users.md#user-groups)). See [Users](users.md).

**Compared with Jellyfin:**

- The per-user permissions are on by default, as in Jellyfin.
- A title that cannot play without conversion fails as Jellyfin's `NoCompatibleStream`.

**For app developers:**

- With conversion off, PlaybackInfo picks the next version that plays without conversion.
- Without the user's **Can download** permission, `/Items/{id}/Download` answers 403.
- Administrators' Jellyfin apps set the same permissions through the user policy (`EnableVideoPlaybackTranscoding`, `EnableAudioPlaybackTranscoding`, `EnableContentDownloading`), where video and audio conversion can be allowed separately.
- The policy shows the user's own permissions, whatever the server's switches.

## Converted downloads

Apps that keep titles for offline viewing download a version's file as it is, unless **Converted downloads**, under **Settings › Conversion**, is on. It is off by default.

With it on, an app that asks for a smaller copy, as Streamyfin's download sheet does with the quality the user picks, gets one:

- **Video:** copied when the app plays it and it fits the quality picked, else converted, within the same limits as any converted video (see [Size of converted video](#size-of-converted-video)): the quality picked, the user's **Maximum quality** and quality group, and **Maximum quality of converted video**. A version taller than the user's quality group downloads converted down to it, where its file would be refused.
- **Audio:** copied when the app takes its codec, else converted to one it takes.
- **Subtitles:** an image subtitle the app chose is burned into the video; text subtitles stay files the app saves beside it (see [Subtitles](subtitles.md)).
- **Made on the fly:** FFmpeg makes the copy as the app downloads it, at the app's pace, and Polyfin keeps nothing on disk. The file's size is only known at its end, so an interrupted download starts again from the beginning.
- **Permissions:** the user needs **Can download**, and, when the video or the audio is converted, **Can use conversion (transcoding)** with the server's **Conversion (transcoding)** switch on, as for a playback. Without them, the app downloads the file as it is, if the user may download.
- **Cost:** while it runs, a converted download counts toward **Video conversions at once** when it converts the video, on its own even next to the user's playback of the same title, and as one of the user's **Playbacks at once** on its device (see [Playback](playback.md#video-conversions-at-once)). At either limit, no converted download is planned, as no conversion is for a playback; one that starts past the cap anyway, planned at the same time as another, is refused with a `503`.
- **Home:** a converted download shows in the admin app's **Now playing** with a "Downloading" label, its bar telling how far the copy is made.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Converted downloads** | **Settings › Conversion** | Off | Sends apps that ask for it a copy of a title copied or converted to the quality picked, rather than its file. |

**Compared with Jellyfin:**

- Jellyfin gives such apps the same kind of `TranscodingUrl`, with no setting to turn it off.
- Jellyfin does not count downloads toward a user's playbacks, and has no quality groups.

**For app developers:**

- A device profile whose video `TranscodingProfile` has `Protocol` `http` and `Container` `mp4` or `ts` gets, when the version does not direct play, a `TranscodingUrl` of the form `/videos/{id}/stream.mp4?…`, with `TranscodingSubProtocol` `http`, as from Jellyfin. Polyfin sends one fragmented MP4 file, or MPEG-TS for `ts`, as FFmpeg writes it, without `Content-Length` and without byte ranges: a request with a `Range` header gets the whole file (200).
- `startTimeTicks` starts the copy later; the file then starts at zero.
- The `TranscodingUrl` stands for the user PlaybackInfo answered, through its play session. With another user's token, it is refused (403).
- `DELETE /Videos/ActiveEncodings?playSessionId=` stops it.
- `/Items/{id}/Download` and `/Videos/{id}/stream?static=true` keep serving the file as it is, under the quality group rule. So does a `TranscodingUrl` kept from before the setting was turned off.

## GPUs

### GPU methods per system

Each system offers only its own GPU methods, under **Graphics card used to convert** and in `POLYFIN_HWACCEL`, in the order **Automatic** tries them:

| System | Methods | Decoding | Encoders |
| --- | --- | --- | --- |
| Linux | NVIDIA (`nvenc`), then AMD or Intel (`vaapi`) | CUDA; VAAPI | `h264_nvenc`, `hevc_nvenc`; `h264_vaapi`, `hevc_vaapi` |
| macOS | VideoToolbox (`videotoolbox`) | VideoToolbox | `h264_videotoolbox`, `hevc_videotoolbox` |
| Windows | NVIDIA (`nvenc`), then Intel (`qsv`), then AMD (`amf`) | CUDA; Direct3D 11 (D3D11VA) for Intel and AMD | `h264_nvenc`, `hevc_nvenc`; `h264_qsv`, `hevc_qsv`; `h264_amf`, `hevc_amf` |

On Windows, Intel and AMD GPUs are opened through the Direct3D 11 adapter of their vendor, so a computer with two graphics cards uses the right one for each method. A system without a GPU that encodes, such as a Windows computer without a graphics card, converts on the processor, as on Linux.

A backup restored on another system keeps its choice of method; when the new system does not offer it, it reads as **Automatic**.

### Detection

At startup Polyfin encodes a few frames with each encoder of the system's methods, in the order above, and logs the one it converts video on. It skips this when **Settings › Conversion** chooses a GPU. For VAAPI, it tries each render node of `/dev/dri` in turn, unless **Graphics card for VAAPI** names one.

A method whose encoders all fail is logged with FFmpeg's reason (`A GPU method does not encode`, with the method, the encoder and the end of what FFmpeg wrote), at the level of the line that tells what video is converted on: information under **Automatic**, a warning when the settings choose that method.

It then measures, in the background, which way converts faster on that GPU (see [Frames on the GPU](#frames-on-the-gpu)), in a few seconds.

### NVIDIA driver

The FFmpeg 9.0 of the Docker image and of Polyfin Desktop needs NVIDIA driver 610.00 or newer for NVENC, on Linux and on Windows. In Docker, this is the host's driver. With an older driver, NVENC fails at startup: the log gives FFmpeg's reason, `Driver does not support the required nvenc API version`, and **System › Health** shows a warning naming the version needed. Video is then converted on another GPU of the computer if one encodes, or else on the processor.

NVIDIA's R580 drivers are the last for Maxwell, Pascal and Volta cards, such as the GeForce GTX 900 and 10 series, the Quadro M and P series and the Titan V. They never reach 610, so these cards cannot use NVENC with Polyfin. Turing cards (GeForce GTX 16 and RTX 20 series) and newer ones can, once their driver is updated.

### Frames on the GPU

A GPU's decoded frames can stay in its memory until it encodes them, or come back to memory for the filters and go to the GPU again. Depending on the card and its driver, either may be faster, so Polyfin measures both ways at startup and uses the faster one for each kind of conversion:

- **SDR video:** kept on the GPU, it is scaled there (`scale_cuda` on NVIDIA, `scale_vaapi` on AMD or Intel through VAAPI, `scale_vt` with VideoToolbox, `scale_qsv` on Intel through QSV, `vpp_amf` on AMD through AMF), and deinterlaced there with the matching filter (`yadif_cuda` or `bwdif_cuda`, `deinterlace_vaapi`, `yadif_videotoolbox`, or `vpp_qsv`'s deinterlacer; AMF has none). A deinterlacer is used only once it deinterlaced a short sample on the GPU at startup: one that fails there, or makes FFmpeg crash, leaves interlaced video to the processor. On Windows, Intel and AMD GPUs decode into Direct3D 11 frames, which QSV maps to its own.
- **HDR video on NVIDIA:** kept on the GPU, it is decoded into Vulkan frames, which libplacebo tone maps where they are. FFmpeg cannot hand Vulkan frames back to NVENC, so the converted picture, at its final size, goes to the encoder through memory.
- **HDR video on Intel and VideoToolbox:** always kept on the GPU, which deinterlaces it, scales it and tone maps it there, with `scale_vaapi` and `tonemap_vaapi` through VAAPI, `vpp_qsv` through QSV, `scale_vt` with VideoToolbox, then encodes it. Burned-in subtitles are laid on in memory after the tone mapping. This is not measured: there is no other way on the GPU.

Each measurement converts 2 seconds of generated 4K video to 1080p both ways. The log shows, for each kind, the way chosen and the speed of each (`Timed the GPU's conversion chains`). Conversions that start before the measurement ends go through memory. The processor's tone mapping is measured next (see [HDR and tone mapping](#hdr-and-tone-mapping)).

Frames always go through memory for burned-in subtitles (except on Intel and VideoToolbox when they tone map), for formats unchecked under **Read these formats on the graphics card**, for tone mapping on the processor, and on a GPU where the way through its memory failed.

### Giving the container an NVIDIA GPU

- Docker: `--runtime=nvidia`. Compose: `runtime: nvidia`.
- This needs the NVIDIA Container Toolkit, or Unraid's Nvidia Driver plugin.
- The host's NVIDIA driver must be 610.00 or newer (see [NVIDIA driver](#nvidia-driver)).
- HDR is also tone mapped on the NVIDIA GPU, through Vulkan. This needs `graphics` among `NVIDIA_DRIVER_CAPABILITIES`; the image sets it.

### Giving the container an AMD or Intel GPU

- Docker: `--device /dev/dri`. Compose: `devices`.

### Permissions

The container runs as user 65532. When the render nodes in `/dev/dri` are not open to every user, add the group that owns them with `--group-add`.

## HDR and tone mapping

When HDR video is converted to SDR, Polyfin tone maps it:

- **On an NVIDIA GPU:** with libplacebo, up to 4K, Dolby Vision profile 5 included, decoded straight into the frames libplacebo tone maps when that is faster (see [Frames on the GPU](#frames-on-the-gpu)).
- **On an Intel GPU, through VAAPI on Linux or QSV on Windows:** with `tonemap_vaapi` or `vpp_qsv`, up to 4K, without Dolby Vision profile 5. It takes only HDR10 videos whose frames carry their mastering display information (SMPTE ST 2086), which both filters need: HLG, and HDR10 files without that information, go to the processor.
- **With VideoToolbox, on macOS:** with `scale_vt`, which converts the colors to BT.709 through the system's own tone mapping, up to 4K: HDR10 and HLG, without Dolby Vision profile 5.
- **Elsewhere:** on the processor, up to **Maximum quality of HDR converted by the processor**, without Dolby Vision profile 5. With **Automatic**, Polyfin measures the processor at startup, in the background: it converts 2 seconds of generated 4K HDR10 to 1080p the way conversions run on the server, decoded and encoded on the GPU when there is one. 1080p when that runs at 1.5 times real time or faster, else 720p, which also applies until the measurement ends or when it fails. The log shows the speed and the height chosen (`Timed the processor's HDR tone mapping`).

AMD GPUs never tone map. Their Linux driver failed each way Polyfin tried to do it on the GPU with FFmpeg: frames handed to libplacebo from memory crashed the driver, VAAPI frames cannot be passed to Vulkan, and Vulkan's HEVC decoding froze the card's video decoder until the kernel reset it. On Windows, FFmpeg's AMF filter converts colors but does not tone map. Their HDR is tone mapped on the processor.

An Intel GPU, or VideoToolbox, tone maps only once a short HDR10 sample converted on it at startup; the check needs FFmpeg's x265 encoder to make that sample. Whether an HDR10 file carries its mastering display information comes from its container when Polyfin analyzes it. Otherwise, the first time an Intel GPU would tone map the file, Polyfin reads its first frame once and keeps the answer. Other GPUs and the processor never need it.

If HDR looks wrong on the graphics card, turn off **Tone map HDR on the graphics card**: the processor then tone maps it, on every GPU alike. You can also turn tone mapping off and pick the method under [Conversion settings](#conversion-settings).

## HLS segments

Remuxes and conversions of files are cut into HLS segments on the source's own keyframes:

- The first segment ends on the first keyframe at least 2 seconds in, and each next one on the first keyframe at least 4 seconds after its start. A file with a keyframe every 2 seconds plays its first segment after 2 seconds of picture; one with a keyframe every 10 seconds gets 10-second segments.
- An MPEG-TS file, which has no keyframe index, is converted and cut the same way: 2 seconds, then every 4 seconds. Its segments count from its start, whatever time its first packet tells, such as an hour into a broadcast, and a seek lands where asked, however far apart its keyframes are.
- A Blu-ray disc is cut on the keyframes its clips' information files list, and its video copied when the app takes it; a DVD is converted and cut every few seconds, as an MPEG-TS file is. See [Local folders](local-folders.md#blu-ray-discs).
- Polyfin makes segments up to **Seconds prepared ahead** past the last one the app asked for, then waits for the app.
- An app asking for a segment whose source stopped sending anything for 20 seconds gets `503` at once, rather than waiting minutes. Its next request waits for the same conversion, which goes on when the source answers again. A source that fails for good ends its conversion, and the next request answers `503` too.

**For app developers:**

- `EXT-X-TARGETDURATION` is the longest segment, rounded up.
- After a seek, FFmpeg probes 2 MB of the file's head, which the analysis already read, rather than 5 MB; conversions burning subtitles in, MPEG-TS files and discs probe as usual.
- After a seek in an MPEG-TS file or a disc, FFmpeg starts at the previous segment's start, as its MPEG-TS and MPEG program stream readers land on the first keyframe after the time asked, and what comes before the segment asked is left out, without being converted.
- When FFmpeg still lands past the segment's start, as in a file with a keyframe every 10 seconds, it starts again further back: as much further back as it went past the time asked, so at least twice as far, until it lands before the segment or reads the file from its start. Each try costs an FFmpeg start; the frames read before the segment are decoded but not converted.
- Polyfin keeps that distance with the file, in its database: the playback's later seeks, and those of the next playbacks of the same file, by any user and after a restart too, start as far back, and land where asked with one FFmpeg start. It knows a local file by its folder, path, size and modification time, so that a file that changed learns the distance again; an addon's file by its version, which follows the file by its name and size, else its link (see [Playback](playback.md)); an IPTV catch-up programme by its channel, whose programmes usually come from the same encoder; a recording by itself. A playback that needs to start further back keeps the larger distance; a shorter one never replaces it. Files FFmpeg lands in where asked, such as Matroska and MP4 files, never use it.

### Changing the audio track

Apps change the audio track of a converted video by playing it again: jellyfin-web asks `PlaybackInfo` for the new track, stops the old play session with `DELETE /Videos/ActiveEncodings`, and loads the new stream from the same position; Jellyfin Android TV, Swiftfin and Streamyfin stop the player and play again too. Off, as before, the new session converts the video again from the keyframe before the position, though the picture has not changed, and the segments already made are deleted.

With **Keep the converted video when the audio track changes** under **Settings › Conversion › Performance**, off by default:

- **Video and audio apart:** a converting FFmpeg makes the video alone, and a second FFmpeg the audio track played, copied or converted as before, cut at the same segment times. The audio's FFmpeg reads the file through the source cache, as the video's does, and makes no segment the video has not reached, nor further than **Seconds prepared ahead**. With [several qualities](#adaptive-quality), one audio FFmpeg serves them all.
- **Joined as served:** each segment joins the video and the audio of the same times, copying both, in fragmented MP4 (one initialization segment holding both tracks) and in MPEG-TS (the audio's packets interleaved with the video's). Apps see the same endpoints, master playlist, `CODECS` and segment numbers as before.
- **Kept video:** the converted video is known by its user, device, version, segment format and conversion (codec, size, bitrate, tone mapping, burned subtitle, quality), not by its play session or audio track. Once its play session stops, it is kept 30 seconds, its FFmpeg waiting where it was. A new play session asking for the same video takes it over, with the segments made: its track's FFmpeg starts at the segment the app asks for, the old track's stops, and the first segment after the change comes once its audio is made, without converting the picture again.
- **Late stops:** jellyfin-web sends the old session's `DELETE` again once the new stream plays; it stops nothing the new session took over.
- **Bounds:** one kept video per user, device and version: another quality or burned subtitle converts anew, and ends the video kept there. A play session that did not stop is never taken over, so two players of the same device each convert their own. Kept video counts once toward **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)). Kept video no session takes over within 30 seconds stops, and its segments are removed. Copied video (DirectStream), Live TV and [converted downloads](#converted-downloads) start again as before.

**Why the audio stays inside the segments:** HLS can carry audio tracks as renditions of their own, which a player switches between without reloading the stream. Jellyfin's master playlists list subtitle renditions only, and its apps never switch audio inside a converted stream: they ask for a new one, as above. Separate audio renditions would go unused, and players that take only the main playlist's segments would play no sound. Keeping each segment whole, its audio joined as it is served, works with every app as it is.

**Compared with Jellyfin:**

- Jellyfin converts the video again whenever the audio track changes, as Polyfin does with the setting off.

## Conversion settings

**Settings › Conversion** also tunes how video and audio are converted, for files and Live TV alike. Recordings copy the stream, and are converted like any file when played. See [Live TV](live-tv.md).

Finished recordings can also be converted to HEVC at night, to take less space, with **Convert finished recordings to HEVC** under **Settings › Recordings** (see [Smaller recordings](live-tv.md#smaller-recordings)). Those conversions use the **Graphics card used to convert**, the **Encoding speed**, the **HEVC quality**, 28 when it is 0, and the **Deinterlacing method** and **Double the frame rate** below, never tone mapping. Each counts as one of the **Video conversions at once**, and waits while playbacks fill them.

Every default but **Seconds prepared ahead**, **Tone map HDR on the graphics card** (Intel GPUs did not tone map before) and **Maximum quality of HDR converted by the processor** (720p before) keeps Polyfin's conversions as they were before these settings existed. A change applies to the next playback, without a restart.

### Picture subtitles

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Turn picture subtitles into text** | **Settings › Conversion** | **Off** | **When played** or **Also after library scans**: PGS and VobSub tracks are read into text copies with Tesseract, which apps that cannot draw them play without a video conversion. |
| **Picture subtitle tracks read at once** | **Settings › Conversion** | 1 (range 1 to 4) | How many tracks Tesseract reads at the same time, one processor thread each. |

See [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text).

### Graphics card

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Graphics card used to convert** | **Settings › Conversion** | `POLYFIN_HWACCEL` at the first start, else **Automatic** | **Automatic** (the first that works, in the order of [GPU methods per system](#gpu-methods-per-system)), the system's methods, or **None**, applied once saved: **NVIDIA (NVENC)** and **AMD or Intel (VAAPI)** on Linux, **VideoToolbox** on macOS, **NVIDIA (NVENC)**, **Intel (QSV)** and **AMD (AMF)** on Windows. |
| **Graphics card for VAAPI** | **Settings › Conversion** | **Each in turn** | On Linux, with **Automatic** or **AMD or Intel (VAAPI)**, the render node VAAPI converts video on when several graphics cards could: **Each in turn**, the first that works, or one of those found in `/dev/dri`. Applied once saved. |
| **Detected on this server** | **Settings › Conversion** | Read only | Shows the card chosen with its device, its encoders, whether it tone maps HDR and, for VAAPI, whether it takes a quality number. Also shows the processor's encoders and tone mapping, with the height it tone maps up to. |
| **Read these formats on the graphics card** | **Settings › Conversion** | All checked | H.264, HEVC, HEVC 10-bit (needs HEVC), VP9, AV1, MPEG-2 and VC-1. An unchecked format is decoded by the processor, for conversions and thumbnails. |

- The chosen card is detected when you save, once per choice. Playbacks already converting go on as they started.
- Options the hardware cannot do are disabled.
- Formats not in the list are tried on the card, as before.

### Video encoding

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Encoding speed** | **Settings › Conversion** | **Automatic** | **Automatic** keeps Polyfin's speeds: `veryfast` for x264 and x265, `p4` for NVENC, the encoder's own for VAAPI, VideoToolbox, QSV and AMF. **Very slow (best picture)** to **Ultra fast (lightest work)** are x264's presets, mapped to NVENC's `p7` to `p1`, VAAPI's compression levels 1 to 7, QSV's presets of the same names (`veryfast` for the two fastest), AMF's `quality`, `balanced` and `speed`, and VideoToolbox's speed priority (off from **Slow** down, on from **Fast** up). |
| **H.264 quality (0 = by bitrate)** and **HEVC quality (0 = by bitrate)** | **Settings › Conversion** | 0 (range 1 to 51) | 0 aims for the bitrate alone, as before. A number is a quality factor that the bitrate caps: CRF for x264 and x265, CQ for NVENC, QVBR's quality for the VAAPI drivers that have it, and VideoToolbox's quality, from 1 to 100, as 100 minus twice the number. Other VAAPI drivers, QSV and AMF ignore it: their quality modes leave the maximum bitrate out. Recordings converted to HEVC use it with every encoder. |
| **Allow converting to HEVC** | **Settings › Conversion** | Off | On: apps that list HEVC before H.264 get HEVC. Off: H.264 comes first, and only apps that take no H.264 get HEVC, as before. |
| **Several qualities when converting** | **Settings › Conversion** | Off | **Off**, **Outside the local network** or **Always**: converted video also offers up to two lower qualities that apps step down to on a slow connection. See [Adaptive quality](#adaptive-quality). |
| **Deinterlacing method** | **Settings › Conversion** | Yadif | Yadif or Bwdif, on the NVIDIA card too when it keeps the frames, and Yadif with VideoToolbox (Bwdif goes through the processor there). An AMD or Intel card keeping the frames uses its own deinterlacer whatever the method; AMF has none, and its interlaced video is deinterlaced on the processor. |
| **Double the frame rate** | **Settings › Conversion** | — | Makes a frame of each field of video up to 30 frames a second. |

### HDR

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Convert HDR to SDR (tone mapping)** | **Settings › Conversion** | On, as before | Off: HDR is converted without tone mapping and looks pale, without the processor's cap, and Dolby Vision without an HDR10 layer is not converted. |
| **Tone mapping method** | **Settings › Conversion** | **Automatic** | **Automatic** (BT.2390 on an NVIDIA card, Hable on the processor, as before), BT.2390, Hable, Reinhard, Möbius, Clip or Linear. BT.2390 works on NVIDIA cards only: the processor's filter has none and uses Hable. Intel cards and VideoToolbox use their own curve and ignore the method. |
| **Tone map HDR on the graphics card** | **Settings › Conversion** | On | NVIDIA cards, Intel cards through VAAPI or QSV, and VideoToolbox tone map HDR themselves, up to 4K. Off: the processor tone maps it on every card, a way out when a card's colors look wrong, and Dolby Vision without an HDR10 layer is not converted. |
| **Maximum quality of HDR converted by the processor** | **Settings › Conversion** | **Automatic (measured at startup)** | **Automatic** (1080p or 720p, as measured at startup, shown beside it), 720p, 1080p, 1440p or 4K. The processor's encoder stops at 1080p anyway: 1440p and 4K count only when a GPU encodes the video. |
| **Peak brightness in nits (0 = from the video)** | **Settings › Conversion** | 0 (the video's), or 100 to 10,000 nits | Processor only; the cards do not take it, so it is hidden when an NVIDIA card tone maps. |
| **Highlight desaturation (0 = off)** | **Settings › Conversion** | 0 to 10 | Processor only, shown as the peak is. |

### Audio

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Mix to stereo** | **Settings › Conversion** | FFmpeg's own | Or Jellyfin's Dave750, Night mode, RFC 7845 and AC-4, for the layouts Jellyfin lists for each, 5.1 and 7.1 among them. |
| **Volume when mixing to stereo** | **Settings › Conversion** | 1 (range 0.5 to 3) | Volume of the stereo mix. |
| **Most audio channels** | **Settings › Conversion** | As many as the app takes | Or mono, stereo or 5.1. Shapes converted audio. |
| **Audio bitrate per channel in kb/s (0 = automatic)** | **Settings › Conversion** | 0 (range 32 to 320 kb/s) | 0 keeps Polyfin's 192 kb/s in stereo and 64 kb/s a channel above stereo. Shapes converted audio. |

### Performance

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Processor threads per conversion (0 = automatic)** | **Settings › Conversion** | 0, FFmpeg's choice (up to 64) | Number of encoding threads. |
| **Seconds prepared ahead** | **Settings › Conversion** | 120 (range 30 to 600) | How many seconds of picture a remux or conversion of a file makes past the last segment the app asked for, before it waits. |
| **Keep the converted video when the audio track changes** | **Settings › Conversion** | Off | Converts the video apart from its audio, and keeps it 30 seconds once its play session stops, for a new session asking for another audio track to take over. Off, a track change converts the video again, as before. See [Changing the audio track](#changing-the-audio-track). |

**Compared with Jellyfin:**

- **Settings › Conversion** works as Jellyfin's Transcoding page does.
- Jellyfin checks only H.264 and VC-1 under hardware decoding by default.
- Polyfin maps encoding speeds to NVENC and VAAPI as Jellyfin does; Jellyfin sets VAAPI compression levels on Intel drivers only.
- Jellyfin's quality defaults are CRF 23 (H.264) and 28 (HEVC), and only for x264 and x265.
- **Allow converting to HEVC** matches Jellyfin's **Allow encoding in HEVC format**; with it off, Polyfin behaves as Jellyfin does.
- Jellyfin's HDR-to-SDR conversion is off by default; its peak brightness is 100 by default.
- **Double the frame rate** works as in Jellyfin.
- Jellyfin's stereo mix volume is 2 by default.
- Jellyfin has no **Most audio channels** or **Audio bitrate per channel** setting.
- **Processor threads per conversion** is Jellyfin's encoding thread count.
- Jellyfin also throttles by seconds, but not by default.
