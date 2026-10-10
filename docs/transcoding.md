# Transcoding

This page covers when Polyfin converts (transcodes) video and audio, how to give it a GPU for that, how HDR is tone mapped, and every setting under **Settings › Conversion**.

## When Polyfin converts

Polyfin picks the lightest way an app can play a file:

- **Direct play:** when the app supports the file, Polyfin redirects the app to the stream and stays out of the video path.
- **Remux:** when the app can play the video but not the container, Polyfin repackages it into its own on-demand HLS, built on FFmpeg for remote sources.
- **Audio conversion:** when the app cannot take the audio, Polyfin converts the audio.
- **Video conversion:** when the app cannot take the video, or must have image subtitles (PGS, VobSub, DVB) burned in, Polyfin converts the video, HDR to SDR included. See [Subtitles](subtitles.md).

Video converts on an NVIDIA GPU through NVENC, or on an AMD or Intel GPU through VAAPI, when one is available. Otherwise it converts on the processor. A GPU keeps the source's size, up to 4K; the processor converts to 1080p at most. For how apps choose between versions of a title, see [Playback](playback.md).

## Size of converted video

Converted video is never larger than its source. It is smaller when one of these limits is below the source's size, the lowest one winning:

- **GPU:** up to 4K.
- **Processor:** up to 1080p.
- **HDR converted by the processor:** up to **Maximum quality of HDR converted by the processor**, 720p or 1080p with **Automatic**, when no GPU tone maps it (see [HDR and tone mapping](#hdr-and-tone-mapping)).
- **Quality group:** the user's, set on their page under **Users**.
- **Maximum quality of converted video:** under **Settings › Conversion**.
- **Bitrate:** the bitrate limit leaves too little for a taller picture. The limit is the app's, from its quality setting, or the user's **Maximum quality** when that is lower.

When two limits are equal, the user's quality group is named first, then **Maximum quality of converted video**, then the GPU or the processor. The bitrate is named only when it allows less than all of them.

On Home in the admin app, a playback's **Details** show which one limited the size, in the **Smaller than the source** row: for instance "720p at most: HDR converted by the processor", "Quality group: 720p" or "Bitrate allowed by the app: 5.6 Mbps". The row does not show when the video keeps the source's size.

## Turning conversion and downloads on or off

The server has one switch, **Conversion (transcoding)** under **Settings › Conversion**. With it off, Polyfin never re-encodes video or audio, and never burns subtitles in. Apps play files as they are, or remuxed into HLS with their tracks copied, and Polyfin picks the next version that plays that way. A title with no version that plays that way on an app does not start on that app. Image subtitles the app cannot show are left out.

On a user's page under **Users**, the **Access** section holds each user's own **Can use conversion (transcoding)** and **Can download** permissions, on by default. Conversion needs both the server's switch and the user's permission; downloads need the user's permission only. **Turn off downloads for everyone**, on the **Users** page, takes it away from every user at once, also in the apps already signed in. See [Users](users.md).

**Compared with Jellyfin:**

- The per-user permissions are on by default, as in Jellyfin.
- A title that cannot play without conversion fails as Jellyfin's `NoCompatibleStream`.

**For app developers:**

- With conversion off, PlaybackInfo picks the next version that plays without conversion.
- Without the user's **Can download** permission, `/Items/{id}/Download` answers 403.
- Administrators' Jellyfin apps set the same permissions through the user policy (`EnableVideoPlaybackTranscoding`, `EnableAudioPlaybackTranscoding`, `EnableContentDownloading`), where video and audio conversion can be allowed separately.
- The policy shows the user's own permissions, whatever the server's switches.

## GPUs

### Detection

At startup Polyfin encodes a few frames on each GPU it can reach, NVIDIA first, then AMD or Intel, and logs the one it converts video on. It skips this when **Settings › Conversion** chooses a GPU. For AMD and Intel, it tries each render node of `/dev/dri` in turn, unless **Graphics card for VAAPI** names one.

It then measures, in the background, which way converts faster on that GPU (see [Frames on the GPU](#frames-on-the-gpu)), in a few seconds.

### Frames on the GPU

A GPU's decoded frames can stay in its memory until it encodes them, or come back to memory for the filters and go to the GPU again. Depending on the card and its driver, either may be faster, so Polyfin measures both ways at startup and uses the faster one for each kind of conversion:

- **SDR video:** kept on the GPU, it is scaled there (`scale_cuda` on NVIDIA, `scale_vaapi` on AMD or Intel), and deinterlaced there when FFmpeg has the matching filter (`yadif_cuda` or `bwdif_cuda`, `deinterlace_vaapi`).
- **HDR video on NVIDIA:** kept on the GPU, it is decoded into Vulkan frames, which libplacebo tone maps where they are. FFmpeg cannot hand Vulkan frames back to NVENC, so the converted picture, at its final size, goes to the encoder through memory.
- **HDR video on Intel:** always kept on the GPU, which deinterlaces it with `deinterlace_vaapi`, scales it in 10 bits with `scale_vaapi` and tone maps it with `tonemap_vaapi`, then encodes it. Burned-in subtitles are laid on in memory after the tone mapping. This is not measured: there is no other way on the GPU.

Each measurement converts 2 seconds of generated 4K video to 1080p both ways. The log shows, for each kind, the way chosen and the speed of each (`Timed the GPU's conversion chains`). Conversions that start before the measurement ends go through memory. The processor's tone mapping is measured next (see [HDR and tone mapping](#hdr-and-tone-mapping)).

Frames always go through memory for burned-in subtitles (except on Intel when it tone maps), for formats unchecked under **Read these formats on the graphics card**, for tone mapping on the processor, and on a GPU where the way through its memory failed.

### Giving the container an NVIDIA GPU

- Docker: `--runtime=nvidia`. Compose: `runtime: nvidia`.
- This needs the NVIDIA Container Toolkit, or Unraid's Nvidia Driver plugin.
- HDR is also tone mapped on the NVIDIA GPU, through Vulkan. This needs `graphics` among `NVIDIA_DRIVER_CAPABILITIES`; the image sets it.

### Giving the container an AMD or Intel GPU

- Docker: `--device /dev/dri`. Compose: `devices`.

### Permissions

The container runs as user 65532. When the render nodes in `/dev/dri` are not open to every user, add the group that owns them with `--group-add`.

## HDR and tone mapping

When HDR video is converted to SDR, Polyfin tone maps it:

- **On an NVIDIA GPU:** with libplacebo, up to 4K, Dolby Vision profile 5 included, decoded straight into the frames libplacebo tone maps when that is faster (see [Frames on the GPU](#frames-on-the-gpu)).
- **On an Intel GPU, through VAAPI:** with `tonemap_vaapi`, up to 4K, without Dolby Vision profile 5. It takes only HDR10 videos whose frames carry their mastering display information (SMPTE ST 2086), which `tonemap_vaapi` needs: HLG, and HDR10 files without that information, go to the processor.
- **Elsewhere:** on the processor, up to **Maximum quality of HDR converted by the processor**, without Dolby Vision profile 5. With **Automatic**, Polyfin measures the processor at startup, in the background: it converts 2 seconds of generated 4K HDR10 to 1080p the way conversions run on the server, decoded and encoded on the GPU when there is one. 1080p when that runs at 1.5 times real time or faster, else 720p, which also applies until the measurement ends or when it fails. The log shows the speed and the height chosen (`Timed the processor's HDR tone mapping`).

AMD GPUs never tone map. Their Linux driver failed each way Polyfin tried to do it on the GPU with FFmpeg: frames handed to libplacebo from memory crashed the driver, VAAPI frames cannot be passed to Vulkan, and Vulkan's HEVC decoding froze the card's video decoder until the kernel reset it. Their HDR is tone mapped on the processor.

An Intel GPU tone maps only once a short HDR10 sample converted on it at startup; the check needs FFmpeg's x265 encoder to make that sample. Whether an HDR10 file carries its mastering display information comes from its container when Polyfin analyzes it. Otherwise, the first time an Intel GPU would tone map the file, Polyfin reads its first frame once and keeps the answer. Other GPUs and the processor never need it.

If HDR looks wrong on the graphics card, turn off **Tone map HDR on the graphics card**: the processor then tone maps it, on NVIDIA and Intel alike. You can also turn tone mapping off and pick the method under [Conversion settings](#conversion-settings).

## HLS segments

Remuxes and conversions of files are cut into HLS segments on the source's own keyframes:

- The first segment ends on the first keyframe at least 2 seconds in, and each next one on the first keyframe at least 4 seconds after its start. A file with a keyframe every 2 seconds plays its first segment after 2 seconds of picture; one with a keyframe every 10 seconds gets 10-second segments.
- An MPEG-TS file, which has no keyframe index, is converted and cut the same way: 2 seconds, then every 4 seconds.
- Polyfin makes segments up to **Seconds prepared ahead** past the last one the app asked for, then waits for the app.
- An app asking for a segment whose source stopped sending anything for 20 seconds gets `503` at once, rather than waiting minutes. Its next request waits for the same conversion, which goes on when the source answers again. A source that fails for good ends its conversion, and the next request answers `503` too.

**For app developers:**

- `EXT-X-TARGETDURATION` is the longest segment, rounded up.
- After a seek, FFmpeg probes 2 MB of the file's head, which the analysis already read, rather than 5 MB; conversions burning subtitles in, and MPEG-TS files, probe as usual.

## Conversion settings

**Settings › Conversion** also tunes how video and audio are converted, for files and Live TV alike. Recordings copy the stream, and are converted like any file when played. See [Live TV](live-tv.md).

Every default but **Seconds prepared ahead**, **Tone map HDR on the graphics card** (Intel GPUs did not tone map before) and **Maximum quality of HDR converted by the processor** (720p before) keeps Polyfin's conversions as they were before these settings existed. A change applies to the next playback, without a restart.

### Graphics card

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Graphics card used to convert** | **Settings › Conversion** | `POLYFIN_HWACCEL` at the first start, else **Automatic** | **Automatic**, **NVIDIA (NVENC)**, **AMD or Intel (VAAPI)** or **None**, applied once saved. |
| **Graphics card for VAAPI** | **Settings › Conversion** | **Each in turn** | With **Automatic** or **AMD or Intel (VAAPI)**, the render node VAAPI converts video on when several graphics cards could: **Each in turn**, the first that works, or one of those found in `/dev/dri`. Applied once saved. |
| **Detected on this server** | **Settings › Conversion** | Read only | Shows the card chosen with its device, its encoders, whether it tone maps HDR and, for VAAPI, whether it takes a quality number. Also shows the processor's encoders and tone mapping, with the height it tone maps up to. |
| **Read these formats on the graphics card** | **Settings › Conversion** | All checked | H.264, HEVC, HEVC 10-bit (needs HEVC), VP9, AV1, MPEG-2 and VC-1. An unchecked format is decoded by the processor, for conversions and thumbnails. |

- The chosen card is detected when you save, once per choice. Playbacks already converting go on as they started.
- Options the hardware cannot do are disabled.
- Formats not in the list are tried on the card, as before.

### Video encoding

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Encoding speed** | **Settings › Conversion** | **Automatic** | **Automatic** keeps Polyfin's speeds: `veryfast` for x264 and x265, `p4` for NVENC, the driver's for VAAPI. **Very slow (best picture)** to **Ultra fast (lightest work)** are x264's presets, mapped to NVENC's `p7` to `p1` and VAAPI's compression levels 1 to 7. |
| **H.264 quality (0 = by bitrate)** and **HEVC quality (0 = by bitrate)** | **Settings › Conversion** | 0 (range 1 to 51) | 0 aims for the bitrate alone, as before. A number is a quality factor that the bitrate caps: CRF for x264 and x265, CQ for NVENC, and QVBR's quality for the VAAPI drivers that have it; other drivers ignore it. |
| **Allow converting to HEVC** | **Settings › Conversion** | Off | On: apps that list HEVC before H.264 get HEVC. Off: H.264 comes first, and only apps that take no H.264 get HEVC, as before. |
| **Deinterlacing method** | **Settings › Conversion** | Yadif | Yadif or Bwdif, on the NVIDIA card too when it keeps the frames. An AMD or Intel card keeping the frames uses its own deinterlacer whatever the method. |
| **Double the frame rate** | **Settings › Conversion** | — | Makes a frame of each field of video up to 30 frames a second. |

### HDR

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Convert HDR to SDR (tone mapping)** | **Settings › Conversion** | On, as before | Off: HDR is converted without tone mapping and looks pale, without the processor's cap, and Dolby Vision without an HDR10 layer is not converted. |
| **Tone mapping method** | **Settings › Conversion** | **Automatic** | **Automatic** (BT.2390 on an NVIDIA card, Hable on the processor, as before), BT.2390, Hable, Reinhard, Möbius, Clip or Linear. BT.2390 works on NVIDIA cards only: the processor's filter has none and uses Hable. Intel cards use their own curve and ignore the method. |
| **Tone map HDR on the graphics card** | **Settings › Conversion** | On | NVIDIA cards, and Intel cards through VAAPI, tone map HDR themselves, up to 4K. Off: the processor tone maps it on every card, a way out when a card's colors look wrong, and Dolby Vision without an HDR10 layer is not converted. |
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
