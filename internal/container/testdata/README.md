# Fixtures

Made with ffmpeg 9.0.2, in this directory:

```sh
V='-f lavfi -i testsrc2=size=64x64:rate=24:duration=15'
A='-f lavfi -i sine=frequency=440:sample_rate=8000:duration=15'
X='-c:v libx264 -preset veryfast -crf 40 -bf 2 -g 48 -x264-params scenecut=0 -force_key_frames 0,1.3,2.9,3.1,6,7.7,11,12.5 -pix_fmt yuv420p'
Q='-hide_banner -loglevel error -y'

ffmpeg $Q $V $A -map 1:a -map 0:v $X -c:a libopus -b:a 6k forced.mkv
ffmpeg $Q $V $X -f matroska - > piped.mkv
ffmpeg $Q $V -c:v libvpx-vp9 -b:v 20k -g 60 -force_key_frames 0,4.5 -cues_to_front 1 front.webm
ffmpeg $Q $V $A -map 1:a -map 0:v $X -c:a aac -b:a 8k -movflags +faststart faststart.mp4
ffmpeg $Q $V $X tail.mp4
ffmpeg $Q $V $X -movflags +negative_cts_offsets negative.mp4
ffmpeg $Q -itsoffset 1.5 $V $A -map 1:a -map 0:v $X -c:a aac -b:a 8k delayed.mp4
ffmpeg $Q -ss 4.2 -i faststart.mp4 -map 0 -c copy trimmed.mp4
ffmpeg $Q $V $X -movflags +frag_keyframe+empty_moov fragmented.mp4
```

`hevc.mkv` is a minute of HEVC in open GOPs, a CRA keyframe every 4 s, as
streaming encodes are: a decoder fed several of its keyframes may output
them in another order, which the thumbnail tests check against:

```sh
ffmpeg $Q -f lavfi -i testsrc2=size=128x72:rate=24:duration=60 -c:v libx265 -preset veryfast -crf 40 -pix_fmt yuv420p \
  -x265-params keyint=96:min-keyint=96:scenecut=0:open-gop=1:bframes=3:log-level=error hevc.mkv
```

The expected times in the tests are the keyframe packets of:

```sh
ffprobe -v error -select_streams v:0 -show_entries packet=pts_time,flags -of csv=p=0 FILE
```

`TestKeyframesSignedCompositionOffsets` alters `negative.mp4` in memory (its
`ctts` box set to version 0, the first entry's offset to -512); its
expected times are what the same ffprobe command prints for the file so
altered.

`subtitles.mkv` holds a video track, the SubRip track `subtitles.srt`
(French, titled), the ASS track `subtitles.ass` (default and forced) and
`Dummy.ttf`, a font attachment of dummy bytes, all committed here:

```sh
ffmpeg $Q -f lavfi -i testsrc2=size=64x64:rate=24:duration=12 -i subtitles.srt -i subtitles.ass \
  -map 0:v -map 1 -map 2 -c:v libx264 -preset veryfast -crf 40 -g 48 -pix_fmt yuv420p -c:s:0 srt -c:s:1 ass \
  -metadata:s:s:0 language=fre -metadata:s:s:0 title=Français -metadata:s:s:1 language=eng \
  -disposition:s:0 0 -disposition:s:1 default+forced \
  -attach Dummy.ttf -metadata:s:t mimetype=application/x-truetype-font subtitles.mkv
```

FFmpeg lists every subtitle block in the Cues, with its relative position
and duration. The expected blocks in the tests are the subtitle packets of:

```sh
ffprobe -v error -show_entries packet=stream_index,pts_time,duration_time -of csv=p=0 subtitles.mkv
```

with the text of the SubRip cues, and the ASS events as Matroska stores
them: `ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text`.

`subtitles.ffprobe.json` is ffprobe's view of `subtitles.mkv`, which
Polyfin's analysis reads, for the tests that play it end to end:

```sh
ffprobe -v error -print_format json -show_format -show_streams subtitles.mkv |
  jq --arg name subtitles.mkv '.format.filename = $name' >subtitles.ffprobe.json
```
