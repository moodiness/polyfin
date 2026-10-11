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
