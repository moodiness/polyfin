# Quality of converted video

This page covers how large converted video is, what bitrate its video and audio get, and how an app's device profile and the conversion settings limit them. For when Polyfin converts at all, see [Transcoding](transcoding.md).

## Size of converted video

Converted video is never larger than its source. It is smaller when one of these limits is below the source's size, the lowest one winning:

| Limit | Up to |
|---|---|
| GPU | 4K |
| Processor | 1080p |
| HDR converted by the processor, when no GPU tone maps it | **Maximum quality of HDR converted by the processor**: 720p or 1080p with **Automatic** (see [HDR and tone mapping](gpus-and-hdr.md#hdr-and-tone-mapping)) |
| HDR on a GPU too slow for it, slower than real time at 1080p | 720p (see [A GPU too slow for HDR](gpus-and-hdr.md#a-gpu-too-slow-for-hdr)) |
| Quality group | The user's, set on their page under **Users** |
| **Maximum quality of converted video** | See [Maximum quality of converted video](#maximum-quality-of-converted-video) |
| The app's profile | The largest picture, or the highest codec level, the app says it decodes (see [What the app allows](#what-the-app-allows)) |
| Bitrate | The tallest picture the bitrate limit leaves room for |

**The bitrate limit:** it is the app's, from its quality setting, or the user's **Maximum quality** when that is lower. The video gets the limit less the bitrate of the audio sent:

- the converted audio's (see [Audio bitrate](#audio-bitrate)), or the track's own when it is copied;
- a copied track whose bitrate is not known counts as converted audio of its channels. AC-3 or DTS in a Matroska file often is such a track. It counts 128 kb/s in mono, 384 kb/s up to five channels, 640 kb/s from six.

A lossless track of several Mb/s converted to stereo AAC at 384 kb/s thus leaves 1080p 9.6 Mb/s within 10 Mb/s. The `TranscodingUrl` writes the two as `VideoBitrate` and `AudioBitrate`.

When two limits are equal, the one named is, in this order: the user's quality group, **Maximum quality of converted video**, the app's profile, then the GPU or the processor. The bitrate is named only when it allows less than all of them.

On Home in the admin app, a playback's **Details** show which one limited the size, in the **Smaller than the source** row. Examples: "720p at most: HDR converted by the processor", "Quality group: 720p", "Quality allowed by the app: 720p" or "Bitrate allowed by the app: 5.6 Mbps". The row does not show when the video keeps the source's size.

### What the app allows

An app's device profile can limit the video it decodes, in its codec profiles: for instance, H.264 no taller than 720 lines. A source above such a limit does not play as it is, and Polyfin converts it within the limit, as Jellyfin does.

Polyfin reads these conditions:

- those of the codec profiles of the codecs the app takes converted video in (its transcoding profile's `VideoCodec`), in the transcoding profile's container, whose `ApplyConditions` the source meets;
- those of the transcoding profile itself, when the video cannot play as it is or the file is above the bitrate limit.

A condition applies whether `IsRequired` is set or not. One about another codec, which the app is not sent converted, does not.

| Condition | Effect | `TranscodingUrl` |
|---|---|---|
| `Width` and `Height` | The picture is scaled down to fit both, keeping its shape: a 1080p source under `Height` 720 converts to 1280×720, and under `Width` 1280 too. Its bitrate follows the smaller picture (see [Bitrate of converted video](#bitrate-of-converted-video)). | `MaxWidth`, `MaxHeight` |
| `VideoBitrate` | The video's bitrate stays within it, and the picture is no taller than that bitrate allows, as under a bitrate limit. | The lower of the two, as `VideoBitrate` |
| `VideoFramerate` | A source above it is brought down to it, frames dropped. The master playlist's `FRAME-RATE` says so. | `MaxFramerate` |
| `VideoLevel` | Polyfin declares the lowest level that holds the picture it sends, at its frame rate and bitrates (see [Frame rate and codec level](#frame-rate-and-codec-level)). When that is above the app's, the picture goes down the ladder until a level the app decodes holds it: under H.264 level 3.1, 1080p converts to 720p. A peak above what the app's level holds is brought within it. | `h264-level` or `hevc-level` |
| `VideoProfile` | Converted H.264 is in the highest of High, Main and Baseline the condition allows, Baseline as constrained baseline, under the name each encoder gives it. A condition allowing none of the three, such as `high 10` alone, leaves High. HEVC stays in Main. | The profiles allowed, as `h264-profile` |

The other conditions decide only whether the source plays as it is or its video is copied, as in Jellyfin, whose encoder does not read them either. They cover reference frames, bit depth, dynamic range, codec tag, and anamorphic or interlaced video.

**Compared with Jellyfin**

- Jellyfin applies these conditions in `StreamBuilder.BuildStreamVideoItem` and `ApplyTranscodingConditions`. Its encoder scales the picture within `MaxWidth` and `MaxHeight` (`EncodingHelper.GetFixedOutputSize`), and aims for the `VideoBitrate`. It drops frames above `MaxFramerate` (`GetFramerateParam`, `-r`), and passes the profile and the level asked to the encoder (`GetVideoQualityParam`). Polyfin does the same, but for the level and the profile.
- Jellyfin passes the app's level to the encoder whatever the picture, starting from the source's own level, so a 1080p picture can be declared at a level that does not hold it. Polyfin keeps the level it computes from the picture, and makes the picture smaller instead.
- Jellyfin passes the encoder the profile the source has when the condition allows it, else the first the condition lists. Polyfin writes the highest the condition allows.
- Jellyfin keeps the source's whole bitrate when the picture is made smaller. Polyfin keeps the share of it the smaller picture is, as for its own caps.
- Under the app's `VideoBitrate`, Jellyfin decides the audio under the app's bitrate limit only. Polyfin decides the audio under that cap too, with the audio sent, so that PlaybackInfo and the conversion agree: the conversion reads only the `TranscodingUrl`'s bitrates.

### Bitrate of converted video

Converted video keeps about its source's bitrate, as in Jellyfin, up to what the bitrate limit leaves it:

- **Share of the picture:** the source's bitrate, for the share of its picture the conversion keeps. A 4K source converted to 1080p keeps a quarter of it.
- **Codec:** more for a codec that needs more bits. H.264 gets 1/0.6 times (about 1.67 times) the bitrate of an HEVC or VP9 source, and twice an AV1 source's. HEVC gets as much as an H.264 source. Nothing is added from 30 Mb/s.
- **10-bit video:** which converted video turns into 8 bits: 5 % more above 12 Mb/s, up to 50 % more at 4 Mb/s and below.
- **Low bitrates:** up to two and a half times more below 3 Mb/s, so that a small source loses less to a second encoding.
- **Unknown bitrate:** a source whose bitrate is not known gets the usual bitrate of its height.

| Height | 4K | 1440p | 1080p | 720p | 540p | 480p | 360p |
|---|---|---|---|---|---|---|---|
| Usual bitrate | 40 Mb/s | 20 Mb/s | 10 Mb/s | 5 Mb/s | 3 Mb/s | 2 Mb/s | 1 Mb/s |

A 15 Mb/s 10-bit HEVC movie in 1080p thus converts to H.264 at 26 Mb/s with no limit. Within 10 Mb/s, with its audio converted to stereo, it converts at 9.6 Mb/s. The user's quality group, **Maximum quality of converted video** and the app's profile lower the bitrate with the picture's size.

**Peaks:** The encoder lets the video peak at one and a half times its bitrate, over a buffer of two seconds of it, both within its codec level (see [Frame rate and codec level](#frame-rate-and-codec-level)).

- Under the app's or the user's limit, the peak stays within what the limit leaves once the audio sent is taken off. Video and audio together thus never go above the limit.
- Where the limit sets the bitrate, as with 10 Mb/s above, the video peaks at its bitrate. Within 3 Mb/s, 720p H.264 aims for 2.6 Mb/s and peaks at 2.6 Mb/s, with 384 kb/s of audio.
- With **H.264 quality** or **HEVC quality** set, the encoder aims for that quality, and peaks as much.
- The master playlist's `BANDWIDTH` is that peak with the audio sent, counted as above. Its `AVERAGE-BANDWIDTH` is the video's bitrate with that audio. Within 3 Mb/s, both are 3000000.

**Cost:** a higher bitrate takes more bandwidth, and more of the processor without a GPU. In Auto on the local network, jellyfin-web asks for at least 140 Mb/s, so its conversions get about the source's bitrate. To keep them smaller, set a **Maximum quality** for the user, or a quality in the app.

### Frame rate and codec level

Converted video has one frame rate, which the encoder is told, and declares a codec level that holds it:

- **Frame rate:** the source's average, else its real one, as its analysis found them; twice that with **Double the frame rate**, or less under `VideoFramerate`. A rate of 0, or above 240 frames a second, is left out: for a stream that states none, the analysis gives its time base, 1000 in Matroska and 90000 in MPEG-TS. A source with no rate left converts at 30 frames a second. The encoder drops or repeats frames to keep the rate.
- **Codec level:** the lowest that holds the picture at that rate and its peak, never below H.264 4.1 (4.2 above 30 frames a second) or HEVC 4.1. HEVC is always at its Main tier, since many TVs and boxes decode no other. The buffer is brought within the level's, which holds at least a second of the peak. A 4K movie converted to HEVC at 26 Mb/s peaks at 39 Mb/s: level 5 holds 25 Mb/s, level 5.1 holds 40 Mb/s and a 40 Mbit buffer, so it is level 5.1 with a 40 Mbit buffer.
- **The app's level:** under the app's `VideoLevel`, the peak, the buffer and the bitrate are brought within the highest level the app decodes, as within 6.1 without one. 1080p H.264 at 30 Mb/s peaks and aims at 25 Mb/s under level 4.0, and 1080p HEVC at 40 Mb/s at 20 Mb/s under level 4.1.
- **Master playlist:** its `CODECS` and `FRAME-RATE` state the same level and frame rate.

An NVIDIA GPU refuses to start a conversion at a level that does not hold what its encoder is told, as x265 does for the frame rate. QSV and VideoToolbox's HEVC encoder pick their own level.

**Compared with Jellyfin**

- Jellyfin passes NVENC no level, and its other encoders the app's level, whatever the frame rate and bitrate.

### Audio bitrate

Converted audio gets Jellyfin's bitrates, with **Audio bitrate per channel in kb/s (0 = automatic)** at 0:

| Channels | Bitrate |
|---|---|
| Mono | 128 kb/s |
| Stereo to five channels | 384 kb/s |
| 5.1 | 640 kb/s |

Under a low bitrate limit, it gets at most its share of the limit, as in Jellyfin:

| Limit | Audio at most |
|---|---|
| Within 640 kb/s | 128 kb/s |
| Within 2 Mb/s | 384 kb/s |
| Within 3 Mb/s | 448 kb/s |
| Above | 640 kb/s |

A number in the setting gives that much per channel, whatever the limit.

**Copied audio under a limit:** when the video is converted under the app's or the user's limit, a track the app takes is copied only when its bitrate is within that share. The share is 448 kb/s within 3 Mb/s, 1.536 Mb/s within 10 Mb/s, and up to 7.168 Mb/s above 20 Mb/s.

- **A heavier track:** it is converted to a lossy codec the app takes, at the bitrates above, with as many channels as the app takes. It thus does not leave the video too little.
- **Example:** within 3 Mb/s, a FLAC 5.1 track counts as 2.88 Mb/s (480 kb/s a channel when it states no bitrate). It becomes AAC 5.1 at 448 kb/s, and leaves the video 2.55 Mb/s, where it left it the 300 kb/s floor. An AC-3 5.1 track at 448 kb/s is still copied.
- **No limit, or video copied:** the track is copied as before. The video is copied only when the whole file fits the limit.
- **Unknown bitrate:** a track whose bitrate is not known at all is copied.
- **Lossy first:** audio converted under a limit goes to a lossy codec when the app takes one, even when it lists FLAC first.

**Compared with Jellyfin**

- Jellyfin keeps the source's whole bitrate when the picture is made smaller. Polyfin keeps the share of it the smaller picture is, so that a 4K source converted to 1080p does not get the bitrate of 4K.
- With no limit at all, Jellyfin's encoder aims for no bitrate. Its apps always send one: jellyfin-web asks for at least 140 Mb/s on the local network, which gives what Polyfin gives with no limit.
- Jellyfin's encoders peak at the bitrate they aim for, whatever the limit, over a buffer of twice the bitrate. That is `-maxrate` equal to the bitrate, VideoToolbox without one, QSV one bit per second above. Polyfin does the same where a limit sets the bitrate. Otherwise it peaks at one and a half times the bitrate, within the limit. A conversion well below the limit thus keeps room for its hardest scenes.
- Jellyfin counts 384 kb/s for stereo AAC in its `TranscodingUrl`, but its encoder makes 256 kb/s. Polyfin sends what it counts.
- When the audio is copied, Jellyfin takes off the limit at most its share for audio (1.5 Mb/s within 10 Mb/s). Polyfin takes off the track's own bitrate, which is within that share. A copied track whose bitrate is not known counts the same in both.
- Jellyfin decides whether to copy a track when the encoding starts. Its encoder refuses to copy one above the audio bitrate it counts, that share at most (`EncodingHelper.GetAudioStreamCopyFailureReasons`). Its PlaybackInfo already plans to convert the audio when the file is above the limit (`StreamBuilder.BuildStreamVideoItem`). Polyfin decides it in PlaybackInfo, by the same share, when the video is converted under a limit. A file whose video is copied fits the limit whole, so its track is copied even above the share, where Jellyfin would convert it.
- Jellyfin's master playlist writes the same value as both `BANDWIDTH` and `AVERAGE-BANDWIDTH`: the bitrate its encoder aims for, with the audio bitrate it counts. For a copied track, that is the request's audio bitrate capped by its channels (256 kb/s in stereo), whatever the track's own. Polyfin writes the video's peak and its average apart, each with the audio actually sent.
- Jellyfin's lower qualities, offered only when an app asks, are the first less 50 kb/s to 2 Mb/s, then twice that. Polyfin's are the steps of its ladder.

## Maximum quality of converted video

**Maximum quality of converted video**, in **Settings › Conversion (transcoding) › General**, scales converted video down to that height. It is **Original** by default, or 480p to 2160p.

- Video keeps its shape, and is converted at the bitrate of that height. It fits that height's 16:9 frame: a 2.40:1 picture converted to 1080p is 1920×800, as in a 1080p release.
- A running conversion keeps the size it started with.
- Files played as they are, or with their tracks copied, are untouched.
- A GPU converts up to 4K (2160p), at the source's own size when nothing lower is set: a 3832×1600 picture stays 3832×1600.
- The processor converts to 1080p at most, so 1440p and 2160p change nothing there.
