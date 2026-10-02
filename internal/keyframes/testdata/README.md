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

The expected times in the tests are the keyframe packets of:

```sh
ffprobe -v error -select_streams v:0 -show_entries packet=pts_time,flags -of csv=p=0 FILE
```

`TestReadSignedCompositionOffsets` alters `negative.mp4` in memory (its
`ctts` box set to version 0, the first entry's offset to -512); its
expected times are what the same ffprobe command prints for the file so
altered.
