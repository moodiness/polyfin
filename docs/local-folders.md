# Local folders

Polyfin serves your own video files beside what addons and IPTV sources provide. An administrator declares folders mounted in Polyfin's container, or network shares Polyfin reads itself; each file is matched to a title and becomes one more version of it, next to the addons' streams, with the same version picker, subtitles, audio tracks and transcoding. Each folder is also a library apps see, listing the titles found in it. A folder may also hold music, see [Music folders](#music-folders), or videos no metadata addon knows, see [Home videos](#home-videos).

Folders on a NAS or another computer can be mounted on the host and declared as local folders, or read by Polyfin over SMB or WebDAV, with no mount: see [network shares](#network-shares).

## Mounting folders

Mount each folder in the container, read-only is enough. With Docker:

```sh
docker run … -v /srv/media/movies:/media/movies:ro -v /srv/media/shows:/media/shows:ro ghcr.io/moodiness/polyfin:latest
```

With compose, add the volumes to the `polyfin` service:

```yaml
  polyfin:
    volumes:
      - polyfin_data:/data
      - /srv/media/movies:/media/movies:ro
      - /srv/media/shows:/media/shows:ro
```

Then, in the admin app, open **Sources**, choose **Add a source › Folder**, keep **Local folder**, and give its name (the library's name in apps), its path in the container (`/media/movies`, not the host's path), and whether it holds movies or shows. What a folder holds cannot be changed afterwards: remove it and add it again.

### Permissions

Polyfin runs as user 65532 (group 65532). It must be able to read every file and to open every folder under the path: on the host, files need the "read" permission and folders "read" and "execute" for that user, its group or everyone, such as `chmod -R a+rX /srv/media`. Polyfin never writes to these folders.

A folder Polyfin cannot read is reported on its page in the admin app and under **System › Health**:

- the path does not exist in the container, usually a share that is not mounted (`missing`);
- it exists but Polyfin may not list it, a permissions problem (`unreadable`);
- the path is a file (`not_folder`).

The files found before stay in the library while the folder cannot be read, so a share that is unmounted for a while empties no library.

A folder Polyfin cannot watch for changes is listed under **System › Health** too, as a warning: see [watching for changes](#watching-for-changes).

## Network shares

Polyfin reads folders shared over the network itself, without mounting them: SMB shares (Windows file sharing, which most NAS offer) and WebDAV folders. Open **Sources**, choose **Add a source › Folder**, choose **SMB share** or **WebDAV folder**, and give its name, its address, a user and a password, and whether it holds movies or shows. Everything else works as for a local folder: naming, matching, unmatched files, links, scans and versions.

### Addresses

- An SMB share is `smb://` then the server, the share, and the folder in the share if the videos are not at its root: `smb://nas.local/media/movies`. A server listening on another port than 445 takes it after its name: `smb://nas.local:4450/media`. Polyfin speaks SMB 2 and 3, and signs in with NTLM; SMB 1 is not supported.
- A WebDAV folder is its address: `https://nas.local/dav/movies`. Polyfin signs in with HTTP Basic authentication: prefer `https://`, since `http://` sends the password unencrypted.

The address holds no user or password: they have fields of their own, and an address with them is refused.

### Credentials

- **User** and **Password** are those of an account that may read the folder on the server. Leave both empty for a share open to guests. For an SMB domain account, write the user as `DOMAIN\user`.
- The password is stored encrypted with `POLYFIN_SECRET_KEY` like the other stored secrets (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)), never shown again, and never written to the log. A password the key cannot decrypt counts as wrong, and **System › Health** lists it under **Stored keys**.
- To change it, edit the folder and choose **Replace** beside the password; left alone, the stored one stays. A new user or password scans the share again at once, and keeps the files found. A new address forgets the files found at the former one, links included.

### Performance

- Files are read from the share at each request, with byte ranges, and never copied. Direct play, remuxing, conversions, analysis and thumbnails read them through Polyfin as they read a local file; seeking asks the share for the part needed only.
- Connections are kept between requests: one SMB session per share, signed in again when it ends, such as when the server restarts; HTTP keep-alive for WebDAV.
- A scan lists the share folder by folder. Listings over the network are slower than on a disk, but scans only read the files that are new or changed, so the first scan is the long one. Shares follow the same schedule as local folders, **Settings › Catalogs › Scan local folders every (hours, 0 = never)**, and are watched for changes: through the server's notifications for SMB, by listing their folders again every 2 minutes for WebDAV (see [watching for changes](#watching-for-changes)). A share mounted on the host and declared as a local folder is watched with inotify instead, which only sees the changes made through that host, not those made on the NAS.
- A video plays only as fast as the network carries it: a high bit rate movie needs a steady link to the server, which wired networks give more surely than Wi-Fi.

### Discs on shares

Blu-ray discs, DVDs and disc images on a share are read on the share itself, as on a local folder: they have the same titles, units, links and names (see [Blu-ray discs](#blu-ray-discs), [DVD folders](#dvd-folders) and [Disc images](#disc-images)).

- A scan reads a disc's playlists and IFO files, and an image's ISO 9660 or UDF file system, by byte ranges: a few reads per disc, the image never read whole.
- FFmpeg cannot read a disc from a share, so Polyfin serves it each title as one file:
  - for a Blu-ray disc, its playlist's clips, each from its in time to its out time, in order, the bytes FFmpeg's Blu-ray reader reads of a local disc, as one MPEG-TS stream;
  - for a DVD, its title's cells, in order, from its title set's `.VOB` files, as one MPEG program stream.
- A Blu-ray disc's video is copied when the app takes it, cut on the keyframes its clips' information files list, read on the share; a DVD's is converted, as on a local folder.
- A DVD's track languages and length come from its IFO files, as FFmpeg's DVD reader reads them on a local folder.

### Health

A share Polyfin cannot read is reported on its page and under **System › Health**, with why:

- the server does not answer: it is off, or the address's server or port is wrong (`unreachable`);
- the server refuses the user or password (`refused`);
- the share, or the folder in it, does not exist (`missing`);
- the user may not list the folder (`unreadable`);
- the address is a file (`not_folder`).

As with a local folder, the files found before stay in the library while the share cannot be read, so a NAS that is off for a while empties no library.

A share that was read but cannot be listed while it is watched is reported under **System › Health** as a warning, with why: it does not answer (`unreachable`), or it refuses the user or password (`refused`). It keeps its schedule meanwhile, and is scanned again once it answers.

**System › Health › Network shares** lists each enabled share with how it is watched: **Notified** when its server tells of its changes, **Polled** when its folders are listed again every 2 minutes, with why: a WebDAV server, an SMB server that does not tell of changes, or did not tell of one a listing found, or whose notifications were lost and are asked for again. A share that cannot be watched shows **Not watched**, with why.

## Naming files

Movies, one file per movie, alone or in a folder of its own, a Blu-ray disc or a DVD copied as it is (see [Blu-ray discs](#blu-ray-discs) and [DVD folders](#dvd-folders)), or a disc image (see [Disc images](#disc-images)):

```
/media/movies/Night of the Living Dead (1968).mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/The.General.1926.1080p.BluRay.mkv
```

Shows, a folder per show, the season and episode in each file's name, or discs copied as they are (see [Shows on discs](#shows-on-discs)):

```
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.mkv
/media/shows/The Lone Ranger (1949)/The.Lone.Ranger.S02E10.720p.mkv
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E03-E04.mkv
```

`S01E02`, `s01e02`, `1x02` and, in a season folder, `E02` or `Episode 2` are read. A file holding several episodes, `S01E01-E02`, `S01E01E02` or `S01E01-02`, plays for each of them.

An identifier in a file's or folder's name settles its title: `{imdb-tt0063350}`, `[imdbid-tt0063350]`, `{tmdb-10331}` or `[tmdbid-10331]`.

Files whose names end in `sample`, hidden folders, and folders named `Extras`, `Featurettes`, `Trailers`, `Behind The Scenes`, `Deleted Scenes`, `Interviews` or `Samples` are left out, as are a Blu-ray disc's streams and playlists, which its titles play, and the files of a DVD.

### Shows on discs

A show kept on discs, as box sets are, may be copied as it is into the show's folder or one of its season folders: a Blu-ray disc's folder, a DVD's folder or a disc image (see [Blu-ray discs](#blu-ray-discs), [DVD folders](#dvd-folders) and [Disc images](#disc-images)).

```
/media/shows/The Lone Ranger (1949)/Season 01/Disc 1/VIDEO_TS/VIDEO_TS.IFO
/media/shows/The Lone Ranger (1949)/Season 01/Disc 2/BDMV/PLAYLIST/00001.mpls
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger S02E01-E04.iso
```

The show is matched by its folder, as for its files: its name, its `tvshow.nfo` or a link. A disc's episodes are its titles, a Blu-ray disc's playlists or a DVD's titles:

- a title plays at least 3 minutes: shorter ones are trailers, menus' clips and most extras;
- a "play all" title, which chains other titles, is left out: it plays mostly the parts of two or more other titles, whatever their order, a DVD's cells or the parts of a Blu-ray disc's clips;
- a title playing the same parts as one of a lower number, such as an episode with another audio track, counts once;
- a disc keeps at most 40 titles, the first by number, in the order of their numbers.

A disc in a season folder is of that season; else of the season its name gives, as `Season 2 Disc 1` or `S02D01` do; else of season 1. Its place among the season's discs is the number its name gives: `Disc 2`, `DISC02`, `Disk 2`, `DVD 2`, `BD 2`, `Vol. 2`, `Part 2`, or the `D` of `S01D02`.

Each title holds the episodes:

- given by hand: the unmatched files list each title without episodes, as `DVD: The Lone Ranger (1949)/Season 01/Disc 1 · title 3`, with its length; type its episode beside it as a file's name gives it, `S01E05`, or `S01E05-E06` for a title holding two (see [Unmatched files](#unmatched-files));
- else those of the disc's name, as `The Lone Ranger S02E01-E04.iso` or a disc folder named `S01E05-E08` give: its titles are numbered in order without asking, when the disc has as many titles as the name gives episodes.

Polyfin suggests each disc's episodes: its titles in order, after the episodes the season's discs before it hold, from episode 1 for a season's first, or from the first its name gives. **Use these episodes**, beside the disc, gives them all at once. The episodes given survive every scan, even while the disc is away, and are listed under **Linked by hand**, where removing them leaves the title to its disc's name, or to the suggestion, again.

An episode plays as another movie of a disc does: a DVD's title converted, through FFmpeg's DVD reader; a Blu-ray disc's playlist through its Blu-ray reader, its video copied when the app takes it; an image's from the image; and on a share, as on a local folder (see [Discs on shares](#discs-on-shares)). Through the Jellyfin API, its version is named after the disc and its title, such as `Season 01 - Disc 2 - title 1.m2ts` or `The Lone Ranger S02E01-E04 - title 2.iso`.

Limits:

- A DVD whose episodes are the chapters of one title lists that title alone: give it all its episodes, such as `S01E01-E04`.
- A disc whose titles cannot be read is left out: a Blu-ray disc without playlists Polyfin reads, a DVD without IFO files it reads, and an image whose UDF file system it does not read.
- Extras of 3 minutes or more are listed as titles too: leave them without episodes.
- A DVD folder's episode is sized as its cells, and a Blu-ray disc folder's as the clips it plays, so the episodes cut from one clip each have the clip's size; an image's episodes each have the image's size, as its movies do.
- A disc at the root of a shows folder, or an image at its top, belongs to no show, and is left out.
- The titles of shows' discs have no subtitle files.
- A title's episodes are typed in the admin app or given by the disc's name: episodes' NFO files and chapter names are not read.

### NFO files

Folders organized by Jellyfin or a media manager often hold NFO files, which give each title's identifiers. Polyfin reads them from:

- a movie's own NFO file, the video's name with `.nfo`, such as `Nosferatu (1922) - 1080p.nfo`;
- else `movie.nfo` in the movie's folder, when that folder is the movie's own: its videos are all read as one title, such as the versions of one movie;
- a Blu-ray disc's `movie.nfo`, in its disc folder;
- a show's `tvshow.nfo`, in its folder.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/Nosferatu (1922)/movie.nfo
/media/shows/The Lone Ranger (1949)/tvshow.nfo
```

In XML NFO files, Polyfin reads `<uniqueid type="imdb">` and `<uniqueid type="tmdb">`, else the older `<imdbid>` and `<tmdbid>`, directly in the root element. An NFO file that is not XML may hold the address of the title's IMDb page instead. File names are compared in any case. Only the identifiers are read: titles, plots and ratings still come from the metadata addons, and artwork from them or the images beside the files (see [Artwork](#artwork)), and episodes' NFO files are not read.

### Subtitle files

Subtitle files beside a video are offered as subtitles of its version, as Jellyfin reads them: SubRip (`.srt`), ASS (`.ass`), SSA (`.ssa`) and WebVTT (`.vtt`) files, PGS picture subtitles (`.sup`) and VobSub picture subtitles (an `.idx` file with the `.sub` file of the same name) in the video's folder whose name begins with the video's, followed by a dot.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.fr.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.en.forced.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.en.sdh.ass
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.fre.srt
```

The words after the video's name, separated by dots, are read in any order and letter case:

- a language, as an ISO 639-1 code (`fr`), an ISO 639-2 code (`fre` or `fra`), with a region (`pt-BR`), or a name in English or French (`French`, `Français`);
- `forced` or `foreign`: forced subtitles, which translate only what is not in the audio's language;
- `sdh`, `hi` or `cc`: subtitles for the deaf and hard of hearing;
- `default`: the subtitles shown by default;
- any other word is the subtitles' title, such as `Movie (2001).en.Commentary.srt`.

When videos of the same folder begin alike, such as `Movie (2001).mkv` and `Movie (2001) - 1080p.mkv`, a subtitle file belongs to the longest name it begins with. A Blu-ray disc's subtitle files sit in its disc folder, beside `BDMV`, named after the disc folder: `Nosferatu (1922)/Nosferatu (1922).fr.srt`. A VobSub pair is named by its `.idx` file; an `.idx` file without its `.sub` file is left out, and a `.sub` file alone is not read. When the name of a VobSub pair tells no language, the language of the first track its `.idx` file lists is used, and only that first track is offered. See [Subtitles](subtitles.md#subtitle-files-beside-local-files) for how they reach apps.

## Blu-ray discs

A Blu-ray disc copied as it is is a folder tree: the disc folder holds a `BDMV` folder, whose `STREAM` folder holds the disc's streams, `.m2ts` files, and whose `PLAYLIST` folder holds its playlists, `.mpls` files, beside `index.bdmv`, `CLIPINF` and often a `CERTIFICATE` folder. A movie is a playlist, which plays one or more streams in order; the other playlists are extras, menus and other cuts.

```
/media/movies/Night of the Living Dead (1968)/BDMV/PLAYLIST/00800.mpls
/media/movies/Night of the Living Dead (1968)/BDMV/STREAM/00001.m2ts
/media/movies/Box/DISC02/BDMV/PLAYLIST/00002.mpls
```

In a movies folder, any folder holding a `BDMV` folder (any case) is a disc. Its main title is its longest playlist:

- it plays whole, every stream in order, with its audio tracks, through FFmpeg's Blu-ray reader;
- its size is that of the streams it plays;
- its title is read from the disc folder's name, as a movie file's name is, the year coming from the folder above when the disc folder's name has none;
- a disc folder named only after the disc's number, such as `Disc 1`, `DISC01`, `Disk 2`, `CD1`, `BD 1` or `Part 2`, takes its title from the folder above;
- a disc at the root of a folder takes the folder's own name;
- a link covers the disc folder; the unmatched files list it as `Blu-ray disc: …`;
- through the Jellyfin API, its version is named after the disc folder.

A disc holding several movies, as a box set's discs do, lists each other movie as a title of its own:

- a title is a playlist of at least 20 minutes that plays mostly streams the longer titles do not: another cut, angle or language of a movie counts once;
- a disc that would have more than 6 titles keeps its main title only, as some discs hide their movie among hundreds of playlists;
- a title other than the main one is not matched by the disc folder's name: link it by hand, beside its playlist in the unmatched files, listed as `Blu-ray disc: Box/DISC02 · title 2` for playlist `00002.mpls`;
- through the Jellyfin API, its version is named after the disc folder and its number, such as `Box - DISC02 - title 2`.

A disc plays its playlists from its folder rather than as one file, so it plays over HLS only. Its segments are cut on the keyframes its clips' information files (`BDMV/CLIPINF/*.clpi`) list, so its video is copied when the app takes its codec, as a file's is: H.264 and HEVC, HDR10 included, on most apps; VC-1 and MPEG-2 video only on apps that declare them, and converted otherwise. Its audio is copied when the app takes it. A disc whose clips' information cannot be read is cut every few seconds and its video converted, as a DVD's is (see [DVD folders](#dvd-folders)).

Files beside `BDMV` in a disc folder are read as any other file, and folders a scan leaves out, such as `Extras`, still are.

In a shows folder, a show's disc is its episodes, its playlists chosen otherwise (see [Shows on discs](#shows-on-discs)).

Limits:

- A disc whose playlists cannot be read, or whose main title lasts less than 3 minutes, which FFmpeg's Blu-ray reader refuses, plays its largest stream, as one file, played as it is when the app takes it. A movie split over several streams then plays its largest stream only.
- On a network share, a disc is read as on a local folder (see [Discs on shares](#discs-on-shares)). A play item that ends before its clip does ends on the clip's next keyframe, as libbluray reads it: a title whose play items replay parts of clips, as seamless branching does, may show a few frames twice where they join.
- There is no direct play and no download of a playlist, and no thumbnails or chapter images are made for it.
- A seek in a disc starts FFmpeg one segment early, as its Blu-ray reader lands on the keyframe after the time asked: the first segment after a seek takes a little longer to come.
- The disc must not be encrypted: Polyfin reads discs copied without their copy protection.

Folders scanned before Polyfin read discs held one file per stream; their next scan leaves one per disc, read as above, and a link made on one of a disc's streams moves to the disc. Folders scanned by Polyfin 0.30 held a disc's largest stream; their next scan reads the disc's playlists, and the disc keeps its link.

## DVD folders

A DVD copied as it is is a disc folder holding a `VIDEO_TS` folder. `VIDEO_TS.IFO` lists the disc's titles; each title is in a title set, whose video is in `VTS_01_1.VOB`, `VTS_01_2.VOB` and so on for set 1. The movie is the disc's main title; the other titles are extras, trailers and menus' clips, or the other movies of a disc holding several.

```
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VIDEO_TS.IFO
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VTS_01_1.VOB
/media/movies/Nosferatu (1922)/Disc 1/VIDEO_TS/VTS_02_1.VOB
```

In a movies folder, any folder holding a `VIDEO_TS` folder (any case) is a DVD, and its main title is one movie:

- it plays its main title, the longest the IFO files list, whole, with its audio tracks and chapters, through FFmpeg's DVD reader;
- its title, its year, a `Disc 1` folder and a DVD at the root of a folder are read as for [Blu-ray discs](#blu-ray-discs);
- its size is that of the main title's set, its `.VOB` files;
- a link covers the disc folder; the unmatched files list it as `DVD: …`;
- through the Jellyfin API, its version is named after the disc folder.

A DVD holding several movies lists each other movie as a title of its own, with the rules of [Blu-ray discs](#blu-ray-discs):

- a title is one the IFO files list that plays at least 20 minutes, and mostly cells the longer titles do not: another cut of a movie, playing the same scenes, counts once;
- a DVD that would have more than 6 titles keeps its main title only;
- a title other than the main one is not matched by the disc folder's name: link it by hand, listed in the unmatched files as `DVD: Box/DISC02 · title 2` for title 2, beside the first `.VOB` of its set with its number, such as `VTS_02_1.VOB#2`;
- its size is that of its set's `.VOB` files;
- through the Jellyfin API, its version is named after the disc folder and its number, such as `Box - DISC02 - title 2`.

The DVD is read from its folder rather than as one file, so it always plays converted: its video is converted over HLS, cut every few seconds as an MPEG-TS file's is, and its audio is copied when the app takes it. A user who may not have video converted is not offered it.

In a shows folder, a show's DVD is its episodes, its titles chosen otherwise (see [Shows on discs](#shows-on-discs)).

Limits:

- On a network share, a DVD's title is read as one MPEG program stream of its cells (see [Discs on shares](#discs-on-shares)) rather than through FFmpeg's DVD reader: it has no chapters, and its cells' timestamps are read as they are, without the corrections FFmpeg's DVD reader makes where they restart.
- There is no direct play, and no download.
- A DVD's subtitles are pictures (VobSub). They are not offered as files, so an app that draws subtitles itself gets none; an app that cannot show them gets them burned into the video.
- No thumbnails or chapter images are made for a DVD.
- A DVD's MPEG-2 video is converted even for an app that takes it: the IFO files' time maps tell where each second starts, not where each keyframe does, and FFmpeg's DVD reader starts a seek's timestamps at the time asked rather than at the keyframe it lands on, so copied segments would not join.
- The DVD must not be encrypted: Polyfin reads DVDs copied without their copy protection.

## Disc images

A disc kept as an image is an `.iso` file. In a movies folder, an image of a DVD or of a Blu-ray disc is one movie, its main title:

```
/media/movies/Night of the Living Dead (1968).iso
/media/movies/Nosferatu (1922)/Nosferatu (1922).iso
```

- its title is read from the image's file name, as a movie file's name is, the year coming from the folder above when the name gives none;
- a link covers the image; the unmatched files list it as `DVD image: …` or `Blu-ray image: …`;
- it plays its main title from the image itself: a DVD image its longest title, as a DVD folder does, and a Blu-ray image its longest playlist;
- through the Jellyfin API, its version is named after the image's file.

An image holding several movies, as a box set's often does, lists each other movie as a title of its own, chosen as a DVD folder's or a Blu-ray disc folder's are: link it by hand, listed in the unmatched files as `Blu-ray image: Box.iso · title 2` beside the image's path with its number, `Box.iso#2`. Through the Jellyfin API, its version is named after the image and its number, such as `Box - title 2.iso`. In a shows folder, a show's image is its episodes, each beside the image's path with its number, `Disc 1.iso#2` (see [Shows on discs](#shows-on-discs)).

A scan reads what each new or changed image is, and its titles, and keeps the answer while the image's size and modification time stay the same. It reads the image's ISO 9660 file system, or its UDF one when the image has no ISO 9660 one holding the disc, as many Blu-ray images do (UDF 1.02 to 2.60, a UDF 2.50 metadata partition included):

- an image holding a `VIDEO_TS` folder is a DVD's, as DVD images are;
- one holding a `BDMV` folder is a Blu-ray disc's; when its playlists cannot be read, its main title is the playlist FFmpeg's reader takes for the longest;
- one whose UDF file system Polyfin does not read, as images of rewritable and write-once media have (a sparable or virtual partition, blocks other than 2,048 bytes), is a Blu-ray disc's when FFmpeg's Blu-ray reader opens it, which it tries once per new or changed image: as before Polyfin read UDF, it is one movie, the playlist FFmpeg's reader takes for the longest, and it plays converted;
- any other image, such as a data disc's, is left out.

An image plays as the disc it holds does (see [Blu-ray discs](#blu-ray-discs) and [DVD folders](#dvd-folders)): a Blu-ray image's video is copied when the app takes its codec, its keyframes read through the image's file system, and a DVD image's video is converted.

Images a scan read before Polyfin listed their other movies, and UDF-only images, are read again at the next scan; their main titles keep their links.

Limits:

- On a network share, an image is read on the share (see [Discs on shares](#discs-on-shares)); one whose titles cannot be read there, or whose UDF file system Polyfin does not read, is left out, as FFmpeg's Blu-ray reader cannot open it from the share.
- There is no direct play and no download, and no thumbnails or chapter images are made for an image.
- A Blu-ray image's main title must last at least 3 minutes, as FFmpeg's Blu-ray reader requires.
- An image whose UDF file system Polyfin does not read (see above) lists its main title only, and plays it converted.
- The image's disc must not be encrypted, and a DVD image's subtitles are pictures, as a DVD folder's are.

## Matching

Each file, each disc, or each show's folder, is matched to a title:

1. a link made by hand (see below);
2. else an identifier written in its name: an IMDb identifier as it is; a TMDB identifier through the metadata addons, which give its IMDb identifier;
3. else an identifier its NFO file gives (see [NFO files](#nfo-files)), read the same way, the IMDb identifier when it gives both;
4. else its name and year are searched in the server's metadata addons, the searches the metadata editor uses: the match is kept only when the addons find exactly one title of that name and kind, of that year when the name gives one.

The title is kept under its IMDb identifier, which the addons answer streams for, with its TMDB identifier when known. Without a metadata addon offering searches, only identifiers in names and NFO files, and links, match files.

A folder's figures tell how its matched files were matched: by an identifier in the name, by an NFO file, by a search or by hand.

## Unmatched files

A folder's page lists the files no title was found for, with why: the name gives no title or episode number, no title of that name, several titles of that name (add the year), titles of other years, the search failed, the file is another movie of a Blu-ray disc, which only a link matches, or it is a title of a show's disc without its episodes. Rename the file and scan again, add an NFO file giving its identifier, or type the IMDb identifier of the title (the `tt…` part of its IMDb address) beside it and choose **Link**. In a shows folder, the link covers the whole show folder; for a Blu-ray disc or a DVD, the whole disc folder, or one title of a disc holding several movies; for a disc image, the image. Links survive every scan, even when the file changes or is moved away and back; remove one from the folder's page to match it by its name or NFO file again. While the folder is scanned, the list follows the scan: the files found are added to it as they come, and an identifier being typed stays, so files can be linked before the scan ends.

A title of a show's disc is listed with its disc and number, such as `Blu-ray disc: The Lone Ranger (1949)/Season 01/Disc 2 · title 1`, and its length; a disc's titles follow one another, in order, under the episodes Polyfin suggests for them (see [Shows on discs](#shows-on-discs)). Type a title's episode beside it, `S01E05` or `S01E05-E06`, and choose **Save**, or choose **Use these episodes** to give the disc's titles those suggested. A title whose show is matched leaves the list once numbered, and its episodes are listed under **Linked by hand**, where they can be removed. While its show is unmatched, a title keeps the show's reason and its link form, which links the show.

To find the title without leaving Polyfin, choose **Find the title** beside a file. The search is filled with what the file's name was read as, and its year; change them and choose **Search**, or press Enter. Polyfin searches the server's metadata addons for titles of the folder's kind, movies or shows, and lists at most 10 with their poster, name, year and kind, those of the year given first. Choosing one links the file, its show folder or its disc, as typing its identifier does. Only titles with an IMDb identifier are listed, since links are made to IMDb identifiers; for a title the search does not find, type its identifier.

## Scanning

Folders are scanned at startup, every **Settings › Catalogs › Scan local folders every (hours, 0 = never)** (6 by default, 0 turns the schedule off), and when an administrator chooses **Scan now** on a folder or runs **Scan local folders** under **System › Schedule** (which scans every folder).

### Watching for changes

Folders mounted in the container are also watched for changes, with inotify on Linux. When files are added, renamed or removed in a folder, or in any folder under it, Polyfin waits until they stop changing for a few seconds, then scans that folder again. A movie copied into a folder shows in its library within a minute of the copy's end, without **Scan now**.

- **Settings › Catalogs › Watch folders and shares for changes** turns it off or on, for folders and network shares alike (on by default). Off, no folder or share is watched.
- Network shares are watched through their server's notifications for SMB, and by listing their folders again for WebDAV, as described below.
- The schedule and **Scan now** stay, as a safety net for changes a watch misses.
- A folder that disappears is scanned, which reports it missing; when it is back, it is scanned and watched again within a minute.
- Hidden folders and the folders a scan leaves out, such as `Extras`, are not watched, and changes to files other than videos, disc images, NFO files, subtitle files, images (see [Artwork](#artwork)), a DVD's `.IFO` and `.VOB` files and a Blu-ray disc's playlists do not start a scan. In a music folder, its subfolders named `Extras` or `Other` are watched, as its scans read them, and its audio files, artwork, lyrics files and playlists start a scan.

An SMB server tells Polyfin of the changes on a share: Polyfin asks it to report each change in the share's folder and in every folder under it (SMB's change notifications). After a change, Polyfin waits until the changes stop for a few seconds, as for a folder in the container, then scans the share again, which reads only what changed. A movie copied onto the share, renamed or removed shows within seconds of the copy's end. When the server had too many changes to tell them one by one, the share is scanned all the same.

- The share's folders are still listed again every 10 minutes, as described below, in case a change goes untold.
- Samba tells of every change made through the network, but of those made on the server's own disk, such as by a download client running on the NAS, only those in the share's folder itself, not in the folders under it. When a listing finds such a change in a folder of the share that the server did not tell of, Polyfin keeps the notifications and lists the share's folders every 2 minutes too.
- When the server ends Polyfin's session, such as when it restarts, Polyfin signs in again, asks again for its changes, and scans the share for what changed meanwhile. While the server cannot be reached, Polyfin asks again every 30 seconds, and lists the share's folders every 2 minutes.
- Windows servers were seen to accept a watch of a whole share from its top folder and never answer it. When a listing finds a change in a share's top folder that the server did not tell of, Polyfin watches that top folder and each folder in it apart, which Windows answers. When that watch is silent too, or the top folder holds more than 256 folders, the share is listed every 2 minutes until Polyfin restarts. An address that names a folder of the share, such as `smb://nas.local/media/Movies`, rather than the share itself, avoids this.
- The folders watched stay open to others: they can be renamed, moved or deleted while Polyfin watches them. Polyfin never writes to a share.
- **System › Health › Network shares** tells which shares are notified of their changes and which are polled, and why (see [Health](#health)).

WebDAV servers give no notice of their changes, nor do SMB servers that refuse to. So Polyfin lists those shares' folders again every 2 minutes, and compares each listing with the one its last scan read: the names of its folders, and the names, sizes and modification times of its videos, NFO, subtitle and other files a scan reads. When one differs, the share is scanned again a few seconds later, which reads only what changed. A movie copied onto such a share shows within a few minutes of the copy's end.

- Each round lists the share's top folder first, where a new movie's folder lands, then the folders under it in turn, at most 100 listings a round, half a second apart. A share of more folders is listed whole over several rounds, from where the last round stopped: with 1,000 folders, a change deep in it is seen within 20 minutes, a new folder at the top within 2.
- The folders listed are those the last scan read: hidden folders and those a scan leaves out are not, and a new folder is seen through its parent's listing.
- The first round comes 2 minutes after Polyfin starts, or after the share is added, which are scanned then anyway.
- A copy still under way may be scanned before it ends; the next round sees its size change and scans it again.
- A share that cannot be listed is reported under **System › Health** (see [Health](#health)), and scanned again once it answers.

Linux limits the folders one user may watch, with `fs.inotify.max_user_watches`. Polyfin watches each folder of a tree, so a large library may reach it. A folder that cannot be watched is still scanned on its schedule; **System › Health** lists it with why, and Polyfin tries again every 10 minutes. The container shares the host's limits, so raise it on the host:

```sh
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-inotify.conf
sudo sysctl --system
```

When Health says the limit of watchers or of open files is reached, raise `fs.inotify.max_user_instances` the same way, or the container's limit of open files (`--ulimit nofile=…` with Docker).

A scan reads only what changed: a file is new or changed by its path, size and modification time; files gone are forgotten; files whose title is known keep it without asking the addons again. An NFO file is read when it is new or changed the same way; one added, changed or removed matches its movie, disc or show again, even when the videos did not change, and one removed leaves it to its name and the search. Subtitle files and images added, changed or removed are offered as they are now from the next scan on. Files left unmatched are matched again at each scan. A few titles are looked up at once, so a large first scan takes a while; the folder's figures follow it. On a share, NFO and subtitle files and images are listed with the folders, and only the NFO files new or changed are read.

Symbolic links to files of the folder are followed; links leading out of the folder, and links to folders, are not.

## Versions

A matched file is one more version of its title, or of each of its episodes, named after the folder, with the resolution its name gives and its size, such as `Movies · 1080p · 4.2 GB`. Polyfin reads it from the disk or the share, with byte ranges, and plays it like any other version: direct play, remux or conversion, with its embedded audio and subtitle tracks and the subtitle files beside it. A Blu-ray disc's playlist, a DVD and a disc image play over HLS only, a Blu-ray disc's video copied when the app takes it and a DVD's converted (see [Blu-ray discs](#blu-ray-discs), [DVD folders](#dvd-folders) and [Disc images](#disc-images)). The folder's library lists its titles, the latest found first.

## Artwork

Folders organized by Jellyfin or a media manager often hold posters, backdrops and logos beside their files, chosen by hand. Polyfin reads them as Jellyfin does, as `.jpg`, `.jpeg`, `.png` or `.webp` files, names compared in any letter case, and shows them in place of the metadata addons' artwork of the same type; the types a folder has no image of still come from the addons.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922).mkv
/media/movies/Nosferatu (1922)/poster.jpg
/media/movies/Nosferatu (1922)/fanart.jpg
/media/movies/The General (1926).mkv
/media/movies/The General (1926)-poster.jpg
/media/shows/The Lone Ranger (1949)/poster.jpg
/media/shows/The Lone Ranger (1949)/season01-poster.jpg
/media/shows/The Lone Ranger (1949)/Season 02/folder.jpg
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger - S02E01.mkv
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger - S02E01-thumb.jpg
```

A movie's images are named after its file, `<file name>-<name>`, such as `The General (1926)-poster.jpg`, its poster also `<file name>.jpg`; when the movie is alone in its folder (every video directly in it is the same movie, its versions included), or is a Blu-ray disc or a DVD, whose images sit in its disc folder, the names alone count too, such as `poster.jpg`. In a folder holding several movies, `poster.jpg` belongs to none of them. Of each type, the first name found counts:

| Type | Names |
|---|---|
| Primary (poster) | `poster`, `folder`, `cover`, `default`, `movie` |
| Backdrop | `fanart`, `fanart-1` to `fanart-20`, `background`, `background-1`…, `art`, `art-1`…, `backdrop`, `backdrop1`… |
| Logo | `logo`, `clearlogo` |
| Thumb | `landscape`, `thumb` |
| Banner | `banner` |
| Art | `clearart` |
| Disc | `disc`, `cdart`, `discart` |

The names after the file's name come before the names alone, and `.jpg` before `.jpeg`, `.png` and `.webp`. A title has one backdrop: the first found.

A show's images are in its folder, with the same names (`show` in place of `movie`, and no Disc image), alone or after the folder's name. A season's are in the show's folder, named after the season, `season01-poster`, `season01-fanart`, `season01-banner` and `season01-landscape` (`season1-…` comes first when both are there), and for the specials `specials-…` or `season-specials-…`; then in its season folder, the folder directly in the show's holding its episodes, with the names of the table, such as `folder.jpg` or `poster.jpg`. A season without a poster of its own shows its show's, as with the addons' artwork.

An episode's image is beside its file, named after it, with the names Jellyfin reads for episodes: `<file name>-thumb`, such as `The Lone Ranger - S02E01-thumb.jpg`, then `<file name>` alone, such as `The Lone Ranger - S02E01.jpg`, then the same two names in a `metadata` folder beside the file. It shows in place of the metadata addon's still, for each episode the file holds (both episodes of `S01E03-E04`). Other images beside the file, such as `<file name>-poster.jpg`, are not the episode's. The episodes of shows' discs and disc images (see [Shows on discs](#shows-on-discs)) have no file of their own: they keep the addon's still.

The images belong to the title the file, disc or show folder is matched to, and an episode's to the episodes its file holds, a link made by hand included, from the moment the link is made. When the files of a title in several folders have images, each type comes from the folder listed first on the **Sources** page, then from the first of the title's files or folders by path; a disabled folder gives none.

**Use its artwork**, on the form of a movies or shows folder, chooses whether its titles and episodes show its images; it is on by default, and turning it off brings the addons' artwork and stills back at once. Artwork an administrator uploads in a Jellyfin app wins over both.

Images are read from the folder or the share when an app first asks for them, then kept with the other artwork, at the sizes apps ask for. A scan notices an image added, replaced or removed, an episode's too, by its size and modification time, and apps see the new one, since its tag changes.

## Music folders

A folder or share added as holding **Music** is a music library of its own. Its audio files are read with their tags and grouped into albums and artists, and the library answers as an [Eclipse music addon](addons-and-libraries.md#music-addons)'s does: albums, artists and songs views, album and artist pages, search, lyrics, scrobbling, statistics and playback all work as for an addon's music. What a folder holds is chosen when it is added and cannot change.

```
/media/music/Tone Quartet/Sine Studies/01 First Light.flac
/media/music/Tone Quartet/Sine Studies/CD2/01 Second Wind.flac
/media/music/Tone Quartet/Sine Studies/cover.jpg
```

### Files and tags

A scan lists the files ending in `.mp3`, `.flac`, `.m4a`, `.m4b`, `.alac`, `.aac`, `.ogg`, `.oga`, `.opus`, `.wav`, `.aif`, `.aiff`, `.wma`, `.ape`, `.wv` and `.dsf`, and beside them the artwork images, the lyrics files (`.lrc`) and the playlists (`.m3u` and `.m3u8`). Hidden folders are left out; a music folder's subfolders named `Extras` or `Other` are read, as an album may bear such a name.

Each new or changed file is read with ffprobe: its title, artist, artists, album, album artist, track and disc numbers, year (from its date), genres (several separated by `;`), whether it is part of a compilation, its lyrics, its length, its audio and whether it embeds a cover. Tags of MP3 (ID3), FLAC, Ogg and Opus (Vorbis comments), MP4 and M4A, WMA and WAV are read as ffprobe reads them.

A file without some tags takes them from its path, as in `Artist/Album/01 Title.flac`:

- its title is its name without the number heading it, which is its track number (`1-07 Title` gives disc 1, track 7);
- its album is its folder's name; a folder named only after a disc, such as `CD2` or `Disc 2`, gives the disc number, and the folder above gives the album;
- its artist is the name of the folder above the album's.

A file whose tags and path give no title or no album, such as an untagged file at the folder's root, is left out of the library, and its page lists it under **Tracks left out** with why.

On a share, ffprobe and FFmpeg read each new or changed file through Polyfin, at most 16 MB of it per scan: the tags, the stream headers and the embedded cover at its head, and the index at its end for the MP4 files that keep it there. A file whose tags lie beyond is read by its path.

### Albums and artists

- Tracks of the same album name and album artist, in any letter case, are one album. A track's album artist is its album artist tag, else "Various Artists" for a compilation (the compilation tag set to 1), else its artist tag as written.
- A track's artists are the names of its artists tag (a tag given several times, as Vorbis comments allow), else of its artist tag, separated by `;`: `Ann; Ben` gives Ann and Ben, each with their page, the song listed under each. Its artist tag is kept as written, for search. An album's artists are its album artist tag's, split the same way.
- An album's tracks are in disc, then track order, then by path. Its title and artist are those of its first track; its year, the latest of its tracks'; its genres, all of theirs.
- Albums group into artists by album artist. An artist's page lists their albums and their songs, including those on other artists' albums, at most 100.
- An album's artwork is the cover its first track embedding one embeds, else `cover`, `folder` or `front` (`.jpg`, `.jpeg`, `.png` or `.webp`, any letter case) beside its first track.
- An artist's picture is `artist`, `folder` or `photo` (`.jpg`, `.jpeg`, `.png` or `.webp`, any letter case) in their folder: the folder holding the folders of their albums, and of no album of another artist. Without one, it is the artwork of their first album. The artists views show it.
- Search finds songs by title, artist and album, albums by title and artist, and artists by name; every word must be found, in any letter case.

The folder's figures count its tracks, albums and album artists, and the tracks left out.

### Playlists

The `.m3u` and `.m3u8` files of the folder are the playlists of its library: its Playlists view lists them with the user's own playlists, by name. A playlist's title is its `#PLAYLIST:` line, else its file's name. Its lines name tracks by paths relative to the playlist's folder, or absolute within the folder's path in the container (as paths or `file:` addresses), with slashes or backslashes, in any letter case; its songs are those, in order. A line that names no track of the folder, or leads out of it, is left out; a playlist naming none is not listed. Search finds playlists by title.

### Lyrics

A song's lyrics are those of the `.lrc` file of the same name beside it, else those of its lyrics tag, synced line by line when timed, plain otherwise. They come before LRCLIB's: Polyfin never asks LRCLIB about a song with lyrics of its own, and serves them with **Song lyrics from LRCLIB** off too. Other songs have LRCLIB's (see [Lyrics](addons-and-libraries.md#lyrics)).

### Playback

Songs play from the folder or the share through Polyfin, as other local files do: as they are when the app takes their format, else converted as an addon's songs are (see [Music playback](addons-and-libraries.md#music-playback)). Their format, codec, sample rate and bit depth come from the scan, so playback decides without probing them.

### Rescans

A rescan reads again the files new or changed by size and modification time, forgets the files gone, and chooses again the artwork of the albums and artists whose first track or image changed. Lyrics files and playlists are read again when they change. Watching notices audio files, artwork, lyrics files and playlists, as it notices videos.

The tracks a scan first finds count among the files added of the [weekly summary](notifications.md#weekly-summary), which names them by album.

### Limits

- A folder's playlists are read only: apps cannot add songs to them, reorder or delete them. On a share, a playlist's absolute paths name nothing; relative paths do.
- Artist names split at `;` only: `A feat. B`, `A & B` or `A/B` is one artist unless an artists tag names them. Without an album artist tag, an album's artist is the artist tag as written, one name.
- Apps list a song's artists one by one, as Jellyfin does, rather than its artist tag as written.
- The artists views list album artists; an artist credited only on songs is reached from those songs and from search.
- Lyrics are read from `.lrc` files and tags, not from `.txt` files.

## Home videos

A folder or share added as holding **Home videos** (fr « Vidéos personnelles ») lists videos no metadata addon knows: recordings of a family, a concert or a school play. Each video is a title of its own, which the folder names, dates and plays itself: nothing is sent to the metadata addons, and nothing is ever unmatched. What a folder holds is chosen when it is added and cannot change.

```
/media/family/Beach.mp4
/media/family/School play 2018/play.mp4
/media/family/School play 2018/play.nfo
/media/family/Trip/day 1.mov
/media/family/Trip/day 2.mov
/media/family/Trip/day 2-poster.jpg
```

### Titles

- A video alone in a folder of its own, with its side files, is one title named after the folder (`School play 2018`). Several videos in one folder are as many titles, each named after its file without its extension (`day 1`, `day 2`); so is a video at the folder's top (`Beach`). Names are kept as written: no year or quality is read out of them.
- A title's NFO file is the video's own (`play.nfo`), else `movie.nfo` in the video's own folder. Its `<title>` names the title, its `<plot>` describes it, and its date is its `<premiered>`, else `<aired>`, else `<releasedate>`.
- Without a date in its NFO file, a title is dated by the creation time its video tells, as phones and cameras write it (`creation_time`, or the time Apple's devices write with its time zone), else by its file's modification time. A creation time before 1971, from a clock never set, is not believed. An NFO file that gives a `<year>` alone keeps the video's own date when it is of that year, and is the 1st of January of it otherwise.
- Its artwork is the images beside it, with the names a movie's take (see [Artwork](#artwork)): `day 2-poster.jpg`, or `poster.jpg` in its own folder. Without a poster, it shows a frame of the video, taken a tenth into it as Jellyfin takes one, at most 640 pixels wide as chapter images are, kept with the scan and made again when the video changes.
- Subtitle files beside a video are offered with it, as for any local file. Discs and disc images are left out, as in a shows folder.

A title's identifier comes from its folder and its path in it: it stays through scans and restarts. A video renamed or moved is another title: what users did with the former one, its watch state, resume point, favorites and collections, stays with it and no longer shows.

### In apps

The folder is a library of Jellyfin's `homevideos` type, its titles `Video` items, as Jellyfin's home videos are: jellyfin-web shows it with its Folders and Videos views, newest recording first in the Folders view. Search, watch state, resume points, Continue Watching, collections, the libraries a user sees, allowed hours and downloads work as for other titles, and the videos play, seek and show their scrubbing thumbnails as other local files do.

Parental control judges them as Jellyfin judges its videos: unrated, of the **Other** kind. A user blocking unrated **Other** items sees none of them; a highest rating alone hides none.

Tracking services and scrobbling never hear of them: they have no identifier a service knows. The [weekly summary](notifications.md#weekly-summary) counts the videos a scan first finds among the files added, as videos, never as movies.

The folder's page counts its videos, and how many are dated by an NFO file, by their recording and by their file. It has no unmatched files and no links.

### Limits

- Polyfin reads no other field of NFO files, such as people, tags or ratings, and does not write them.
- A video is one title: several files of one recording are not joined, and a folder holding several videos is no title of its own.
- A frame is made once per video as it is; one that cannot be made (an unreadable video) is tried again only once the video changes.

## Compared with Jellyfin

- Jellyfin reads metadata from files and providers of its own; Polyfin takes titles and their descriptions from the metadata addons, and a local file adds a version, and its artwork when the folder holds some.
- Both watch folders for changes. Polyfin also scans them at startup, on its schedule and on demand, and watches the network shares it reads itself by listing them again every few minutes.
- Jellyfin's "Identify" matches an item by hand from a search of its providers; Polyfin links an unmatched file to an IMDb identifier, typed or chosen from a search of the metadata addons.
- Both read the identifiers of NFO files; Polyfin reads nothing else of them for movies and shows, neither names, plots nor artwork, and only the title, plot and date for home videos.
- Both offer the subtitle files beside a video, named after it, text and pictures (`.sup`, and `.idx` with `.sub`). Polyfin reads only those in the video's own folder.
- Both read the artwork beside movies and shows, and the image beside an episode's file, with the same names. Polyfin keeps one backdrop per title, reads no `extrafanart` folder, `.gif`, `.tbn` or `.svg` image, nor images for the episodes of discs, and shows the addons' artwork of the types a folder has none of.
- Both read music folders with their tags, their playlists and their lyrics files. Polyfin groups albums by album artist and name rather than by folder, splits artists at `;` only, and reads no `.txt` lyrics.
- Both list home videos as `Video` items of a `homevideos` library, judged as **Other** by parental control, with a frame of the video when no image is beside it. Polyfin dates them by the recording's creation time as well, lists no photos nor photo albums in such a library, and makes a video alone in its folder a title named after the folder.

## For app developers

A local folder or network share is an addon of kind `local` in the admin API, listed with the other sources (`GET /admin/api/scopes/shared/addons`), its `folder` field describing its path or address, kind (`movies`, `shows` or `music`), last scan, error, counts and links, and `useArtwork`, whether its titles show the artwork beside its files (see [Artwork](#artwork)). `episodes` lists the episodes given by hand to the titles of shows' discs: `[{"path", "disc", "discPath", "number", "episode"}]`, `disc` the disc's kind as in the unmatched files (empty while an image is gone), `discPath` its folder or image, `number` the title's and `episode` such as `S01E05` or `S01E05-E06`. `matchedBy` counts its matched files by how they were matched: `{"name", "nfo", "search", "link"}`. For a music folder, `files` counts its tracks, `matched` those in an album, `unmatched` those left out, and `albums` and `artists` its albums and their artists; its unmatched files are the tracks left out, with `reason` `no_title` or `no_album`. For a share, `share` is `"smb"` or `"webdav"` (empty for a local folder), `user` its user, and `passwordSet` whether a password is stored; the password itself is never answered. `error` is `missing`, `unreadable`, `not_folder`, or for a share `unreachable` or `refused`; empty after a scan that could read the folder. The administrator routes are:

| Route | What it does |
|---|---|
| `POST /admin/api/scopes/shared/folders` | adds a folder: `{"name", "path", "kind": "movies" or "shows"}`, and for a share `"user"` and `"password"`; `path` is a path in the container or a share's address; `"useArtwork": false` adds a movies or shows folder whose artwork its titles do not show |
| `PATCH /admin/api/scopes/shared/folders/{id}` | changes its `name`, `path`, `user`, `password` or `useArtwork`; a new path or address forgets the files found; a `password` left out keeps the stored one, an empty one removes it |
| `POST /admin/api/scopes/shared/folders/{id}/scan` | starts a scan (202) |
| `GET /admin/api/scopes/shared/folders/{id}/unmatched` | `{"total", "files": [{"path", "unit", "title", "year", "size", "reason", "disc", "discTitle"}]}`; `disc` is `"bluray"` for a Blu-ray disc's playlist or stream and `"dvd"` for a DVD, its `unit` the disc folder, or for a title other than a disc's main one the disc folder, `#` and its playlist's or title's number (`Box/DISC02#00002`), `"dvd_image"` or `"bluray_image"` for a disc image, its `unit` the image, or for a title other than its main one the image, `#` and its number (`Box.iso#00002`), and `null` for other files. A DVD's or an image's title other than its main one has as `path` its disc's file with `#` and its number (`Box/VIDEO_TS/VTS_02_1.VOB#2`, `Box.iso#2`), which links take as any path. A title of a show's disc has as `unit` its show's folder, as `reason` `disc_episode` while its show is matched, and `discTitle` `{"discPath", "number", "length", "episode", "suggested"}`: its disc's folder or image, its number, how long it plays in seconds (null when unknown), its episodes and those suggested, such as `S01E05` (null for none); `discTitle` is null for other files. A disc's titles follow one another, in the order of their numbers |
| `GET /admin/api/scopes/shared/folders/{id}/titles?query=…&year=…` | searches the metadata addons for titles of the folder's kind: `{"titles": [{"imdbId", "name", "year", "kind", "posterId"}]}`, at most 10, only titles with an IMDb identifier, those of `year` (optional) first; `kind` is `movie` or `show`, and `posterId` the item whose Primary image (`/Items/{posterId}/Images/Primary`) is the poster, null without one. A failed search answers 502, `addon_unreachable` when an addon could not be reached, `title_search_failed` otherwise |
| `PUT /admin/api/scopes/shared/folders/{id}/links` | links `{"path", "imdbId"}`; `path` is a file, a show's folder or a disc's folder |
| `DELETE /admin/api/scopes/shared/folders/{id}/links?path=…` | removes a link |
| `PUT /admin/api/scopes/shared/folders/{id}/episodes` | gives titles of shows' discs their episodes, `{"episodes": [{"path", "episode"}]}`, at most 1,000: `path` a title's, as the unmatched files give it, `episode` as a file's name gives it (`S01E05`, `s1e5`, `S01E05-E06`, `S01E05E06`), an empty one removing those given. Answers the folder; `invalid_episode` (400) for an episode written otherwise, 404 for a path that is no title of a show's disc |

An address that is not `smb://host[:port]/share[/path]` or `http(s)://host/path`, or that holds a user or password, is refused with `invalid_share_address`; a user longer than 256 characters or a password longer than 1,024 with `invalid_share_user`. On **System › Health**, the problem of a folder (`code` `folder`) carries its error as `failure`, and `share` for a share. A folder that cannot be watched is a problem of its own (`code` `folder_unwatched`): in the container, its `failure` is `watch_limit` (the limit of watches, ENOSPC), `instance_limit` (the limit of watchers or of open files, EMFILE) or `failed`; for a share, which carries `share`, it is `unreachable`, `refused` or `failed`. A share's password the key cannot decrypt is listed among the stored secrets as `{"folder": name}`.

`GET /admin/api/health` lists the enabled shares under `shares`, each with its `id`, `name`, `share` (`smb` or `webdav`), and `watch`: `notified`, `polled`, or empty while it is not watched. A polled share's `polled` tells why: `webdav`, `unsupported` (the SMB server refuses change notifications), `silent` (it did not tell of a change a listing found), `failed` (its notifications were lost, and are asked for again), or empty while they are asked for. A notified share's `polled` is `partial` when its server missed a change in one of its folders, and its folders are listed every 2 minutes too. `unwatched` is the share's `folder_unwatched` failure, and `watchFolders` whether **Watch folders and shares for changes** is on.

Through the Jellyfin API, a local file is a media source of its title like any other, its `Name` the folder's name and its `Path` its file name. A Blu-ray disc's `Path` is its disc folder's name with the `.m2ts` extension, such as `Nosferatu (1922).m2ts`, or `Nosferatu (1922) - Disc 1.m2ts` for a folder named after the disc's number, and `Box - DISC02 - title 2.m2ts` for a title other than its main one. A playlist's media source, as a DVD's, has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A DVD's `Path` is its disc folder's name with its main title's `.VOB` extension, such as `Nosferatu (1922).VOB`, or `Box - DISC02 - title 2.VOB` for a title other than its main one; its media source has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A disc image's `Path` is its file name, such as `Nosferatu (1922).iso`, or `Box - title 2.iso` for a title other than its main one, and its media source plays as a DVD's does. A title of a show's disc is a media source of its episodes, named as a disc's title other than its main one: `Season 01 - Disc 2 - title 1.m2ts`, `Season 01 - Disc 1 - title 3.VOB` or `Disc 1 - title 2.iso`. A share's files are served the same way.

A title's local artwork is one of its images like the addons': its `ImageTags`, `BackdropImageTags` and `/Items/{id}/Images` list it by tags that change when the image's size or modification time changes, and `/Items/{id}/Images/{type}` serves it, resized as apps ask, from the folder or the share. Besides `Primary`, `Backdrop`, `Logo`, `Thumb` and `Banner`, a local image may be an `Art` or a `Disc` image. No path of the server's shows: `/Items/{id}/Images` gives no `Path`.

A home videos folder's `kind` is `homevideos` in the admin API, added with `"kind": "homevideos"`; it takes no `useArtwork`, its artwork always showing. Its `files` and `matched` count its videos, `unmatched` is 0, and `datedBy` counts them by where their dates come from: `{"nfo", "created", "file"}` (zero for other folders). Its unmatched list is empty, and its `titles` and `links` routes answer 404. Through the Jellyfin API, its library's `CollectionType` is `homevideos`, and its titles have `Type` and `MediaType` `Video`, `PremiereDate` and `ProductionYear` from their date, `Overview` from their NFO file's plot, no `ProviderIds`, and a `Primary` image, the poster beside them or a frame; they answer `IncludeItemTypes=Video`, never `Movie`. In the playback history and the statistics, their `kind` is `video`.
