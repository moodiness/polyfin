# Discs

This page is for administrators who keep Blu-ray discs, DVDs or disc images in [local folders](local-folders.md). It covers how Polyfin finds a disc's movies, how they play, discs on network shares, and shows kept on discs.

The disc must not be encrypted: Polyfin reads discs copied without their copy protection.

## Blu-ray discs

A Blu-ray disc copied as it is is a folder tree. The disc folder holds a `BDMV` folder, which holds:

- a `STREAM` folder with the disc's streams, `.m2ts` files;
- a `PLAYLIST` folder with its playlists, `.mpls` files;
- `index.bdmv`, a `CLIPINF` folder, and often a `CERTIFICATE` folder.

A movie is a playlist, which plays one or more streams in order. The other playlists are extras, menus and other cuts.

```
/media/movies/Night of the Living Dead (1968)/BDMV/PLAYLIST/00800.mpls
/media/movies/Night of the Living Dead (1968)/BDMV/STREAM/00001.m2ts
/media/movies/Box/DISC02/BDMV/PLAYLIST/00002.mpls
```

In a movies folder, any folder holding a `BDMV` folder (any letter case) is a disc. Its main title is its longest playlist:

- It plays whole, every stream in order, with its audio tracks, through FFmpeg's Blu-ray reader.
- Its size is that of the streams it plays.
- Its title is read from the disc folder's name, as a movie file's name is. The year comes from the folder above when the disc folder's name has none.
- A disc folder named only after the disc's number, such as `Disc 1`, `DISC01`, `Disk 2`, `CD1`, `BD 1` or `Part 2`, takes its title from the folder above.
- A disc at the root of a folder takes the folder's own name.
- A link covers the disc folder. The unmatched files list it as `Blu-ray disc: …`.
- Through the Jellyfin API, its version is named after the disc folder.

**Discs holding several movies:** as a box set's discs do, list each other movie as a title of its own:

- A title is a playlist of at least 20 minutes that plays mostly streams the longer titles do not. Another cut, angle or language of a movie counts once.
- A disc that would have more than 6 titles keeps its main title only, as some discs hide their movie among hundreds of playlists.
- A title other than the main one is not matched by the disc folder's name. Link it by hand, beside its playlist in the unmatched files. It is listed as `Blu-ray disc: Box/DISC02 · title 2` for playlist `00002.mpls`.
- Through the Jellyfin API, its version is named after the disc folder and its number, such as `Box - DISC02 - title 2`.

**Playback:** a disc plays its playlists from its folder rather than as one file, so it plays over HLS only.

- Its segments are cut on the keyframes its clips' information files (`BDMV/CLIPINF/*.clpi`) list. So its video is copied when the app takes its codec, as a file's is.
- H.264 and HEVC, HDR10 included, are copied on most apps. VC-1 and MPEG-2 video only on apps that declare them, and are converted otherwise.
- Its audio is copied when the app takes it.
- A disc whose clips' information cannot be read is cut every few seconds and its video converted, as a DVD's is (see [DVD folders](#dvd-folders)).

Files beside `BDMV` in a disc folder are read as any other file. Its extras, such as an `Extras` folder, are its movie's (see [Extras and theme media](naming-and-matching.md#extras-and-theme-media)).

In a shows folder, a show's disc is its episodes, its playlists chosen otherwise (see [Shows on discs](#shows-on-discs)).

**Limits:**

- A disc whose playlists cannot be read, or whose main title lasts less than 3 minutes, which FFmpeg's Blu-ray reader refuses, plays its largest stream as one file. That file plays as it is when the app takes it. A movie split over several streams then plays its largest stream only.
- On a network share, a disc is read as on a local folder (see [Discs on shares](#discs-on-shares)).
- There is no direct play and no download of a playlist. No thumbnails or chapter images are made for it.
- A seek starts FFmpeg one segment early, as its Blu-ray reader lands on the keyframe after the time asked. The first segment after a seek takes a little longer to come.

## DVD folders

A DVD copied as it is is a disc folder holding a `VIDEO_TS` folder.

- `VIDEO_TS.IFO` lists the disc's titles.
- Each title is in a title set, whose video is in `VTS_01_1.VOB`, `VTS_01_2.VOB` and so on, for set 1.
- The movie is the disc's main title. The other titles are extras, trailers and menus' clips, or the other movies of a disc holding several.

```
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VIDEO_TS.IFO
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VTS_01_1.VOB
/media/movies/Nosferatu (1922)/Disc 1/VIDEO_TS/VTS_02_1.VOB
```

In a movies folder, any folder holding a `VIDEO_TS` folder (any letter case) is a DVD, and its main title is one movie:

- It plays its main title, the longest the IFO files list, whole, with its audio tracks and chapters, through FFmpeg's DVD reader.
- Its title, its year, a `Disc 1` folder and a DVD at the root of a folder are read as for [Blu-ray discs](#blu-ray-discs).
- Its size is that of the main title's set, its `.VOB` files.
- A link covers the disc folder. The unmatched files list it as `DVD: …`.
- Through the Jellyfin API, its version is named after the disc folder.

**DVDs holding several movies** list each other movie as a title of its own, with the rules of [Blu-ray discs](#blu-ray-discs):

- A title is one the IFO files list that plays at least 20 minutes, and mostly cells the longer titles do not. Another cut of a movie, playing the same scenes, counts once.
- A DVD that would have more than 6 titles keeps its main title only.
- A title other than the main one is not matched by the disc folder's name. Link it by hand. It is listed in the unmatched files as `DVD: Box/DISC02 · title 2` for title 2, beside the first `.VOB` of its set with its number, such as `VTS_02_1.VOB#2`.
- Its size is that of its set's `.VOB` files.
- Through the Jellyfin API, its version is named after the disc folder and its number, such as `Box - DISC02 - title 2`.

**Playback:** a DVD is read from its folder rather than as one file, so it always plays converted.

- Its video is converted over HLS, cut every few seconds as an MPEG-TS file's is.
- Its audio is copied when the app takes it.
- A user who may not have video converted is not offered it.
- A DVD's MPEG-2 video is converted even for an app that takes it. The IFO files' time maps tell where each second starts, not where each keyframe does. FFmpeg's DVD reader starts a seek's timestamps at the time asked, not at the keyframe it lands on, so copied segments would not join.

In a shows folder, a show's DVD is its episodes, its titles chosen otherwise (see [Shows on discs](#shows-on-discs)).

**Limits:**

- On a network share, a DVD's title is read as one MPEG program stream of its cells (see [Discs on shares](#discs-on-shares)), not through FFmpeg's DVD reader. It has no chapters. Its cells' timestamps are read as they are, without the corrections FFmpeg's DVD reader makes where they restart.
- There is no direct play, and no download.
- A DVD's subtitles are pictures (VobSub). They are not offered as files, so an app that draws subtitles itself gets none. An app that cannot show them gets them burned into the video. With **Turn picture subtitles into text** on, their text copies are offered as files once read, to every app (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).
- No thumbnails or chapter images are made for a DVD.

## Disc images

A disc kept as an image is an `.iso` file. In a movies folder, an image of a DVD or a Blu-ray disc is one movie, its main title:

```
/media/movies/Night of the Living Dead (1968).iso
/media/movies/Nosferatu (1922)/Nosferatu (1922).iso
```

- Its title is read from the image's file name, as a movie file's name is. The year comes from the folder above when the name gives none.
- A link covers the image. The unmatched files list it as `DVD image: …` or `Blu-ray image: …`.
- It plays its main title from the image itself: a DVD image its longest title, as a DVD folder does, and a Blu-ray image its longest playlist.
- Through the Jellyfin API, its version is named after the image's file.

**Images holding several movies:** as a box set's often do, list each other movie as a title of its own. Titles are chosen as for a DVD folder or a Blu-ray disc folder.

- Link such a title by hand. It is listed in the unmatched files as `Blu-ray image: Box.iso · title 2`, beside the image's path with its number, `Box.iso#2`.
- Through the Jellyfin API, its version is named after the image and its number, such as `Box - title 2.iso`.

In a shows folder, a show's image is its episodes, each beside the image's path with its number, `Disc 1.iso#2` (see [Shows on discs](#shows-on-discs)).

**What a scan reads:** for each new or changed image, what it is and its titles. It keeps the answer while the image's size and modification time stay the same. It reads the image's ISO 9660 file system, or its UDF one when no ISO 9660 one holds the disc, as on many Blu-ray images. UDF 1.02 to 2.60 is read, a UDF 2.50 metadata partition included.

| The image holds | It is |
|---|---|
| A `VIDEO_TS` folder | A DVD's image |
| A `BDMV` folder | A Blu-ray disc's image. When its playlists cannot be read, its main title is the playlist FFmpeg's reader takes for the longest. |
| A UDF file system Polyfin does not read, as images of rewritable and write-once media have (a sparable or virtual partition, blocks other than 2,048 bytes) | A Blu-ray disc's image when FFmpeg's Blu-ray reader opens it, tried once per new or changed image. It is one movie, the playlist FFmpeg's reader takes for the longest, and it plays converted. |
| Anything else, such as a data disc | Left out |

An image plays as the disc it holds does (see [Blu-ray discs](#blu-ray-discs) and [DVD folders](#dvd-folders)). A Blu-ray image's video is copied when the app takes its codec, its keyframes read through the image's file system. A DVD image's video is converted.

After an update, images an older scan read before their other movies were listed, and UDF-only images, are read again at the next scan. Their main titles keep their links.

**Limits:**

- On a network share, an image is read on the share (see [Discs on shares](#discs-on-shares)). One whose titles cannot be read there, or whose UDF file system Polyfin does not read, is left out, as FFmpeg's Blu-ray reader cannot open it from the share.
- There is no direct play and no download, and no thumbnails or chapter images are made for an image.
- A Blu-ray image's main title must last at least 3 minutes, as FFmpeg's Blu-ray reader requires.
- An image whose UDF file system Polyfin does not read lists its main title only, and plays it converted.
- A DVD image's subtitles are pictures, as a DVD folder's are.

## Discs on shares

Blu-ray discs, DVDs and disc images on a [network share](network-shares.md) are read on the share itself, as on a local folder. They have the same titles, units, links and names.

- **Scans** read a disc's playlists and IFO files, and an image's ISO 9660 or UDF file system, by byte ranges. That is a few reads per disc, and the image is never read whole.
- **Playback:** FFmpeg cannot read a disc from a share, so Polyfin serves it each title as one file:
  - a Blu-ray disc's title is its playlist's clips, each from its in time to its out time, in order, as one MPEG-TS stream. These are the bytes FFmpeg's Blu-ray reader reads of a local disc;
  - a DVD's title is its cells, in order, from its title set's `.VOB` files, as one MPEG program stream.
- **Video:** a Blu-ray disc's video is copied when the app takes it, cut on the keyframes its clips' information files list, read on the share. A DVD's is converted, as on a local folder.
- **DVD details:** track languages and length come from the IFO files, as FFmpeg's DVD reader reads them on a local folder.
- **Seamless branching:** a play item that ends before its clip does ends on the clip's next keyframe, as libbluray reads it. A title whose play items replay parts of clips may show a few frames twice where they join.

## Shows on discs

A show kept on discs, as box sets are, may be copied as it is into the show's folder or one of its season folders. That may be a Blu-ray disc's folder, a DVD's folder or a disc image.

```
/media/shows/The Lone Ranger (1949)/Season 01/Disc 1/VIDEO_TS/VIDEO_TS.IFO
/media/shows/The Lone Ranger (1949)/Season 01/Disc 2/BDMV/PLAYLIST/00001.mpls
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger S02E01-E04.iso
```

The show is matched by its folder, as for its files: its name, its `tvshow.nfo` or a link (see [Matching](naming-and-matching.md#matching)). A disc's episodes are its titles: a Blu-ray disc's playlists or a DVD's titles.

- A title plays at least 3 minutes. Shorter ones are trailers, menus' clips and most extras.
- A "play all" title, which chains other titles, is left out. It plays mostly the parts of two or more other titles, in any order: a DVD's cells or the parts of a Blu-ray disc's clips.
- A title playing the same parts as one of a lower number, such as an episode with another audio track, counts once.
- A disc keeps at most 40 titles, the first by number, in the order of their numbers.

**Season and place:**

- A disc in a season folder is of that season. Else of the season its name gives, as `Season 2 Disc 1` or `S02D01` do. Else of season 1.
- Its place among the season's discs is the number its name gives: `Disc 2`, `DISC02`, `Disk 2`, `DVD 2`, `BD 2`, `Vol. 2`, `Part 2`, or the `D` of `S01D02`.

**A title's episodes:** they come from:

1. **Given by hand:** The unmatched files list each title without episodes, with its length, such as `DVD: The Lone Ranger (1949)/Season 01/Disc 1 · title 3`. Type its episode beside it as a file's name gives it, `S01E05`, or `S01E05-E06` for a title holding two, and choose **Save**.
2. **Else from the disc's name:** as `The Lone Ranger S02E01-E04.iso` or a disc folder named `S01E05-E08` gives. Its titles are numbered in order without asking, when the disc has as many titles as the name gives episodes.

**Suggestions:** Polyfin suggests each disc's episodes. They are its titles in order, after the episodes the season's earlier discs hold: from episode 1 for a season's first disc, or from the first its name gives.

- In the unmatched files, a disc's titles follow one another, in order, under the suggested episodes.
- **Use these episodes**, beside the disc, gives them all at once.
- Episodes given survive every scan, even while the disc is away.
- A title whose show is matched leaves the list once numbered. Its episodes are listed under **Linked by hand**, where removing them leaves the title to its disc's name, or to the suggestion, again.
- While its show is unmatched, a title keeps the show's reason and its link form, which links the show.

**Playback:** an episode plays as another movie of a disc does.

- A DVD's title is converted, through FFmpeg's DVD reader.
- A Blu-ray disc's playlist plays through its Blu-ray reader, its video copied when the app takes it.
- An image's title plays from the image.
- On a share, it plays as on a local folder (see [Discs on shares](#discs-on-shares)).
- Through the Jellyfin API, its version is named after the disc and its title, such as `Season 01 - Disc 2 - title 1.m2ts` or `The Lone Ranger S02E01-E04 - title 2.iso`.

**Limits:**

- A DVD whose episodes are the chapters of one title lists that title alone. Give it all its episodes, such as `S01E01-E04`.
- A disc whose titles cannot be read is left out: a Blu-ray disc without playlists Polyfin reads, a DVD without IFO files it reads, and an image whose UDF file system it does not read.
- Extras of 3 minutes or more are listed as titles too: leave them without episodes.
- A DVD folder's episode is sized as its cells. A Blu-ray disc folder's is sized as the clips it plays, so episodes cut from one clip each have the clip's size. An image's episodes each have the image's size, as its movies do.
- A disc at the root of a shows folder, or an image at its top, belongs to no show, and is left out.
- The titles of shows' discs have no subtitle files.
- A title's episodes are typed in the admin app or given by the disc's name. Episodes' NFO files and chapter names are not read.

API details: [Local files in the Jellyfin API](jellyfin-api.md#local-files-in-the-jellyfin-api) and [Local folder routes](jellyfin-api.md#local-folder-routes).
