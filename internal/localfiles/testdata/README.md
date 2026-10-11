# Fixtures

`dvd` is a DVD copied as it is, its `VIDEO_TS` folder without the `.BUP`
backups of its IFO files. Title 1, in title set 1, is a 2-second clip with
one audio track. Title 2, in title set 2, is the main title: 8 seconds of
352x240 MPEG-2 video in two chapters, two AC-3 audio tracks, English and
French, and an English VobSub track. Made with the ffmpeg and dvdauthor
0.7.2 packages of Debian trixie, in this directory:

```sh
common="-target ntsc-dvd -s 352x240 -b:v 150k -maxrate 300k -bufsize 224k -g 15 -c:a ac3 -b:a 64k -ar 48000 -ac 2 -muxrate 1000k"
ffmpeg -v error -y -f lavfi -i "testsrc2=size=352x240:rate=30000/1001" -f lavfi -i "sine=frequency=330:sample_rate=48000" \
  -t 2 -map 0:v -map 1:a $common intro.mpg
ffmpeg -v error -y -f lavfi -i "testsrc=size=352x240:rate=30000/1001" -f lavfi -i "sine=frequency=440:sample_rate=48000" \
  -f lavfi -i "sine=frequency=880:sample_rate=48000" -t 8 -map 0:v -map 1:a -map 2:a $common main.mpg
printf '1\n00:00:01,000 --> 00:00:03,000\nFirst line\n\n2\n00:00:05,000 --> 00:00:07,000\nSecond line\n' > main.srt
mkdir -p ~/.spumux && ln -sf /usr/share/fonts/truetype/dejavu/DejaVuSans.ttf ~/.spumux/DejaVuSans.ttf
spumux -m dvd -s 0 subs.xml < main.mpg > main_sub.mpg
VIDEO_FORMAT=NTSC dvdauthor -x dvd.xml
rm dvd/VIDEO_TS/*.BUP
```

`subs.xml`:

```xml
<subpictures format="NTSC">
  <stream>
    <textsub filename="main.srt" characterset="UTF-8" fontsize="16.0" font="DejaVuSans.ttf"
      horizontal-alignment="center" vertical-alignment="bottom" movie-width="352" movie-height="240"
      movie-fps="29.97" subtitle-fps="29.97" bottom-margin="10"/>
  </stream>
</subpictures>
```

`dvd.xml`:

```xml
<dvdauthor dest="dvd">
  <vmgm/>
  <titleset>
    <titles>
      <video format="ntsc" aspect="4:3" resolution="352x240"/>
      <audio lang="en"/>
      <pgc><vob file="intro.mpg"/></pgc>
    </titles>
  </titleset>
  <titleset>
    <titles>
      <video format="ntsc" aspect="4:3" resolution="352x240"/>
      <audio lang="en"/>
      <audio lang="fr"/>
      <subpicture lang="en"/>
      <pgc><vob file="main_sub.mpg" chapters="0,0:04"/></pgc>
    </titles>
  </titleset>
</dvdauthor>
```

`bluray.tar.gz` is a Blu-ray disc copied as it is, its `BDMV` folder,
packed: its streams are mostly MPEG-TS packets that repeat, which pack
well. Playlist `00000` is the main title: 190 seconds of 64x64 H.264 video
and silent AC-3 audio, split into two clips, `00000` and `00001`, on
keyframes. Playlist `00001` plays a third clip, `00005`, 30 seconds long.
The main title lasts over 3 minutes, as FFmpeg's bluray protocol requires
of a disc. Made with the ffmpeg package of Debian trixie and tsMuxeR 2.7.0
(its Linux release), in an amd64 container, in this directory:

```sh
V="-c:v libx264 -preset veryslow -profile:v high -level 4.1 -pix_fmt yuv420p -crf 45 -g 24 -keyint_min 24 -sc_threshold 0 -bsf:v h264_mp4toannexb -f h264"
ffmpeg -f lavfi -i testsrc2=size=64x64:rate=24 -t 190 $V main.264
ffmpeg -f lavfi -i anullsrc=r=48000:cl=stereo -t 190 -c:a ac3 -b:a 96k -ac 2 main.ac3
ffmpeg -f lavfi -i testsrc=size=64x64:rate=24 -t 30 $V extra.264
ffmpeg -f lavfi -i anullsrc=r=48000:cl=stereo -t 30 -c:a ac3 -b:a 96k -ac 2 extra.ac3
printf 'MUXOPT --blu-ray --vbr --split-duration=95 --mplsOffset=0 --m2tsOffset=0\nV_MPEG4/ISO/AVC, "main.264", fps=24, insertSEI, contSPS\nA_AC3, "main.ac3", lang=eng\n' > main.meta
printf 'MUXOPT --blu-ray --vbr --mplsOffset=1 --m2tsOffset=5\nV_MPEG4/ISO/AVC, "extra.264", fps=24, insertSEI, contSPS\nA_AC3, "extra.ac3", lang=fra\n' > extra.meta
tsMuxeR main.meta bluray && tsMuxeR extra.meta bluray
tar -C bluray -czf bluray.tar.gz BDMV
```

`dvd.iso.gz` and `bluray.iso.gz` are images of the two discs above, `dvd`
with an empty `AUDIO_TS` folder beside `VIDEO_TS`, and the unpacked
`bluray.tar.gz`, packed: DVD images carry ISO 9660 and UDF file systems,
as `-dvd-video` writes them, and this Blu-ray image too, as `-udf` does.
Made with the genisoimage package of Debian trixie:

```sh
genisoimage -quiet -dvd-video -V MOVIE_DVD -o dvd.iso dvdroot
genisoimage -quiet -udf -iso-level 3 -V MOVIE_BD -o bluray.iso bdroot
gzip -9 -n dvd.iso bluray.iso
```
