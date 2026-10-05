# Transcoding

This page covers when Polyfin converts (transcodes) video and audio, how to give it a GPU for that, how HDR is tone mapped, and every setting under **Settings › Conversion**.

## When Polyfin converts

Polyfin picks the lightest way an app can play a file:

- **Direct play:** when the app supports the file, Polyfin redirects the app to the stream and stays out of the video path.
- **Remux:** when the app can play the video but not the container, Polyfin repackages it into its own on-demand HLS, built on FFmpeg for remote sources.
- **Audio conversion:** when the app cannot take the audio, Polyfin converts the audio.
- **Video conversion:** when the app cannot take the video, or must have image subtitles (PGS, VobSub, DVB) burned in, Polyfin converts the video, HDR to SDR included. See [Subtitles](subtitles.md).

Video converts on an NVIDIA GPU through NVENC, or on an AMD or Intel GPU through VAAPI, when one is available. Otherwise it converts on the processor. For how apps choose between versions of a title, see [Playback](playback.md).

## Turning conversion and downloads on or off

Two switches apply to the whole server:

- **Conversion (transcoding)** under **Settings › Conversion**.
- **Downloads** under **Settings › Playback**.

With **Conversion (transcoding)** off, Polyfin never re-encodes video or audio, and never burns subtitles in. Apps play files as they are, or remuxed into HLS with their tracks copied, and Polyfin picks the next version that plays that way. A title with no version that plays that way on an app does not start on that app. Image subtitles the app cannot show are left out.

With **Downloads** off, nobody can download: apps hide their download button.

Under **Users**, each user also has their own **Can use conversion (transcoding)** and **Can download** permissions, on by default. Both the server's switch and the user's permission must be on. See [Users](users.md).

**Compared with Jellyfin:**

- The per-user permissions are on by default, as in Jellyfin.
- A title that cannot play without conversion fails as Jellyfin's `NoCompatibleStream`.

**For app developers:**

- With conversion off, PlaybackInfo picks the next version that plays without conversion.
- With **Downloads** off, `/Items/{id}/Download` answers 403.
- Administrators' Jellyfin apps set the same permissions through the user policy (`EnableVideoPlaybackTranscoding`, `EnableAudioPlaybackTranscoding`, `EnableContentDownloading`), where video and audio conversion can be allowed separately.
- The policy shows the user's own permissions, whatever the server's switches.

## GPUs

### Detection

At startup Polyfin encodes a few frames on each GPU it can reach, NVIDIA first, then AMD or Intel, and logs the one it converts video on. It skips this when **Settings › Conversion** or `POLYFIN_HWACCEL` chooses a GPU. See [Configuration](configuration.md) for `POLYFIN_HWACCEL` and `POLYFIN_VAAPI_DEVICE`.

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

- **On an NVIDIA GPU:** up to 1080p, Dolby Vision profile 5 included.
- **Elsewhere:** on the processor, up to 720p, without Dolby Vision profile 5.

You can turn tone mapping off and pick the method under [Conversion settings](#conversion-settings).

## Conversion settings

**Settings › Conversion** also tunes how video and audio are converted, for files and Live TV alike. Recordings copy the stream, and are converted like any file when played. See [Live TV](live-tv.md).

Every default keeps Polyfin's conversions as they were before these settings existed. A change applies to the next playback, without a restart.

### Graphics card

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Graphics card used to convert** | **Settings › Conversion** | Follows `POLYFIN_HWACCEL` | **Automatic**, **NVIDIA (NVENC)**, **AMD or Intel (VAAPI)** or **None** overrides the variable once saved. |
| **Detected on this server** | **Settings › Conversion** | Read only | Shows the card chosen with its device, its encoders, whether it tone maps HDR and, for VAAPI, whether it takes a quality number. Also shows the processor's encoders and tone mapping. |
| **Read these formats on the graphics card** | **Settings › Conversion** | All checked | H.264, HEVC, HEVC 10-bit (needs HEVC), VP9, AV1, MPEG-2 and VC-1. An unchecked format is decoded by the processor, for conversions and thumbnails. |

- The chosen card is detected when you save, once per choice. Playbacks already converting go on as they started.
- Options the hardware cannot do are disabled.
- Formats not in the list are tried on the card, as before.

### Video encoding

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Encoding speed** | **Settings › Conversion** | **Automatic** | **Automatic** keeps Polyfin's speeds: `veryfast` for x264 and x265, `p4` for NVENC, the driver's for VAAPI. **Very slow** to **Ultra fast** are x264's presets, mapped to NVENC's `p7` to `p1` and VAAPI's compression levels 1 to 7. |
| **H.264 quality** and **HEVC quality** | **Settings › Conversion** | 0 (range 1 to 51) | 0 aims for the bitrate alone, as before. A number is a quality factor that the bitrate caps: CRF for x264 and x265, CQ for NVENC, and QVBR's quality for the VAAPI drivers that have it; other drivers ignore it. |
| **Allow converting to HEVC** | **Settings › Conversion** | Off | On: apps that list HEVC before H.264 get HEVC. Off: H.264 comes first, and only apps that take no H.264 get HEVC, as before. |
| **Deinterlacing method** | **Settings › Conversion** | Yadif | Yadif or Bwdif. |
| **Double the frame rate** | **Settings › Conversion** | — | Makes a frame of each field of video up to 30 frames a second. |

### HDR

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Convert HDR to SDR** | **Settings › Conversion** | On, as before | Off: HDR is converted without tone mapping and looks pale, without the processor's 720p cap, and Dolby Vision without an HDR10 layer is not converted. |
| **Tone mapping method** | **Settings › Conversion** | **Automatic** | **Automatic** (BT.2390 on the card, Hable on the processor, as before), BT.2390, Hable, Reinhard, Möbius, Clip or Linear. BT.2390 works on the card only: the processor's filter has none and uses Hable. |
| **Peak brightness** | **Settings › Conversion** | 0 (the video's), or 100 to 10,000 nits | Processor only; libplacebo on the card does not take it. |
| **Highlight desaturation** | **Settings › Conversion** | 0 to 10 | Processor only; libplacebo on the card does not take it. |

### Audio

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Mix to stereo** | **Settings › Conversion** | FFmpeg's own | Or Jellyfin's Dave750, Night mode, RFC 7845 and AC-4, for the layouts Jellyfin lists for each, 5.1 and 7.1 among them. |
| **Volume when mixing to stereo** | **Settings › Conversion** | 1 (range 0.5 to 3) | Volume of the stereo mix. |
| **Most audio channels** | **Settings › Conversion** | As many as the app takes | Or mono, stereo or 5.1. Shapes converted audio. |
| **Audio bitrate per channel** | **Settings › Conversion** | 0 (range 32 to 320 kb/s) | 0 keeps Polyfin's 192 kb/s in stereo and 64 kb/s a channel above stereo. Shapes converted audio. |

### Performance

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Processor threads per conversion** | **Settings › Conversion** | 0, FFmpeg's choice (up to 64) | Number of encoding threads. |
| **Segments prepared ahead** | **Settings › Conversion** | 10 (range 1 to 60) | How many segments of about 6 seconds a remux or conversion of a file makes past the last one the app asked for, before it waits. |

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
- Jellyfin throttles by seconds instead of segments, and not by default.
