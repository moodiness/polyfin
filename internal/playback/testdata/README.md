# Fixtures

`codecs.mkv` holds a video track and three text subtitle tracks, the last
two with the codecs other muxers name them by, then two attached files that
are not fonts: a web page and cover art. Made with ffmpeg 9.0.2, in this
directory:

```sh
Q='-hide_banner -loglevel error -y'
ffmpeg $Q -f lavfi -i color=red:size=16x16 -frames:v 1 cover.jpg
ffmpeg $Q -f lavfi -i testsrc2=size=64x64:rate=24:duration=6 -i codecs.srt -i codecs-webvtt.vtt -i codecs-ascii.vtt \
  -map 0:v -map 1 -map 2 -map 3 -c:v libx264 -preset veryfast -crf 40 -g 48 -pix_fmt yuv420p \
  -c:s:0 srt -c:s:1 webvtt -c:s:2 webvtt \
  -attach page.html -metadata:s:t:0 mimetype=text/html -attach cover.jpg -metadata:s:t:1 mimetype=image/jpeg codecs.mkv
perl -0777 -pi -e 's/\x86\x92D_WEBVTT\/SUBTITLES/\x86\x92S_TEXT\/WEBVTT\0\0\0\0\0/; s/\x86\x92D_WEBVTT\/SUBTITLES/\x86\x92S_TEXT\/ASCII\0\0\0\0\0\0/' codecs.mkv
perl -0777 -pi -e 's/\n\nASCII (one|two)/ASCII $1\n\n/g' codecs.mkv
```

FFmpeg names WebVTT tracks `D_WEBVTT/SUBTITLES`. The first perl command
names the first one `S_TEXT/WEBVTT`, as mkvmerge does, and the second
`S_TEXT/ASCII`, padding each CodecID with zero bytes to the length it had,
as Matroska allows for strings. ffprobe then gives the first one no codec,
and the second the codec `text`. FFmpeg begins each WebVTT block with the
cue's identifier and settings lines, empty here: the second perl command
moves them after the text of the `S_TEXT/ASCII` track, whose blocks then
hold plain text, as such tracks do. FFmpeg makes cover art, an attached
JPEG, a video stream.

`codecs.ffprobe.json` is ffprobe's view of `codecs.mkv`, which Polyfin's
analysis reads:

```sh
ffprobe -v error -print_format json -show_format -show_streams codecs.mkv |
  jq --arg name codecs.mkv '.format.filename = $name' >codecs.ffprobe.json
```
