# Local folders

Polyfin serves your own video files beside what addons and IPTV sources provide. An administrator declares folders mounted in Polyfin's container, or network shares Polyfin reads itself; each file is matched to a title and becomes one more version of it, next to the addons' streams, with the same version picker, subtitles, audio tracks and transcoding. Each folder is also a library apps see, listing the titles found in it.

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
- A scan lists the share folder by folder. Listings over the network are slower than on a disk, but scans only read the files that are new or changed, so the first scan is the long one. Shares follow the same schedule as local folders, **Settings › Catalogs › Scan local folders every (hours, 0 = never)**, but are not watched for changes: raise it for a large share on a slow link, or set it to 0 and choose **Scan now** when files change. A share mounted on the host and declared as a local folder is watched, but inotify only sees the changes made through that host, not those made on the NAS.
- A video plays only as fast as the network carries it: a high bit rate movie needs a steady link to the server, which wired networks give more surely than Wi-Fi.

### Health

A share Polyfin cannot read is reported on its page and under **System › Health**, with why:

- the server does not answer: it is off, or the address's server or port is wrong (`unreachable`);
- the server refuses the user or password (`refused`);
- the share, or the folder in it, does not exist (`missing`);
- the user may not list the folder (`unreadable`);
- the address is a file (`not_folder`).

As with a local folder, the files found before stay in the library while the share cannot be read, so a NAS that is off for a while empties no library.

## Naming files

Movies, one file per movie, alone or in a folder of its own, or a Blu-ray disc copied as it is (see [Blu-ray discs](#blu-ray-discs)):

```
/media/movies/Night of the Living Dead (1968).mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/The.General.1926.1080p.BluRay.mkv
```

Shows, a folder per show, the season and episode in each file's name:

```
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.mkv
/media/shows/The Lone Ranger (1949)/The.Lone.Ranger.S02E10.720p.mkv
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E03-E04.mkv
```

`S01E02`, `s01e02`, `1x02` and, in a season folder, `E02` or `Episode 2` are read. A file holding several episodes, `S01E01-E02`, `S01E01E02` or `S01E01-02`, plays for each of them.

An identifier in a file's or folder's name settles its title: `{imdb-tt0063350}`, `[imdbid-tt0063350]`, `{tmdb-10331}` or `[tmdbid-10331]`.

Files whose names end in `sample`, hidden folders, and folders named `Extras`, `Featurettes`, `Trailers`, `Behind The Scenes`, `Deleted Scenes`, `Interviews` or `Samples` are left out, as are a Blu-ray disc's streams and playlists, which its titles play, and the files of a DVD.

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

In XML NFO files, Polyfin reads `<uniqueid type="imdb">` and `<uniqueid type="tmdb">`, else the older `<imdbid>` and `<tmdbid>`, directly in the root element. An NFO file that is not XML may hold the address of the title's IMDb page instead. File names are compared in any case. Only the identifiers are read: titles, plots, ratings and artwork still come from the metadata addons, and episodes' NFO files are not read.

### Subtitle files

Subtitle files beside a video are offered as subtitles of its version, as Jellyfin reads them: SubRip (`.srt`), ASS (`.ass`), SSA (`.ssa`) and WebVTT (`.vtt`) files in the video's folder whose name begins with the video's, followed by a dot.

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

When videos of the same folder begin alike, such as `Movie (2001).mkv` and `Movie (2001) - 1080p.mkv`, a subtitle file belongs to the longest name it begins with. A Blu-ray disc's subtitle files sit in its disc folder, beside `BDMV`, named after the disc folder: `Nosferatu (1922)/Nosferatu (1922).fr.srt`. Picture subtitle files (`.sup`, `.idx` and `.sub`) are not read. See [Subtitles](subtitles.md#subtitle-files-beside-local-files) for how they reach apps.

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

A disc plays its playlists from its folder rather than as one file, so it always plays converted, as a DVD does (see [DVD folders](#dvd-folders)): its video is converted over HLS, and its audio is copied when the app takes it.

Files beside `BDMV` in a disc folder are read as any other file, and folders a scan leaves out, such as `Extras`, still are.

Limits:

- On a network share, a disc plays its largest stream, as one file, played as it is when the app takes it: FFmpeg reads a disc's playlists from a path. A movie split over several streams then plays its largest stream only. Mount the share on the host and declare it as a local folder to play the whole playlist.
- A disc whose playlists cannot be read, or whose main title lasts less than 3 minutes, which FFmpeg's Blu-ray reader refuses, plays its largest stream the same way.
- There is no direct play and no download of a playlist, and no thumbnails or chapter images are made for it.
- The video is always converted, even when the app takes its codec: Polyfin does not read the keyframes of a disc's streams, which copying the video needs.
- The disc must not be encrypted: Polyfin reads discs copied without their copy protection.
- In a shows folder, discs are left out: a disc of episodes is not read as one episode. Extract its episodes to files of their own.

Folders scanned before Polyfin read discs held one file per stream; their next scan leaves one per disc, read as above, and a link made on one of a disc's streams moves to the disc. Folders scanned by Polyfin 0.30 held a disc's largest stream; their next scan reads the disc's playlists, and the disc keeps its link.

## DVD folders

A DVD copied as it is is a disc folder holding a `VIDEO_TS` folder. `VIDEO_TS.IFO` lists the disc's titles; each title is in a title set, whose video is in `VTS_01_1.VOB`, `VTS_01_2.VOB` and so on for set 1. The movie is the disc's main title; the other titles are extras, trailers and menus' clips.

```
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VIDEO_TS.IFO
/media/movies/Night of the Living Dead (1968)/VIDEO_TS/VTS_01_1.VOB
/media/movies/Nosferatu (1922)/Disc 1/VIDEO_TS/VTS_02_1.VOB
```

In a movies folder, any folder holding a `VIDEO_TS` folder (any case) is a DVD, and is one movie:

- it plays its main title, the longest the IFO files list, whole, with its audio tracks and chapters, through FFmpeg's DVD reader;
- its title, its year, a `Disc 1` folder and a DVD at the root of a folder are read as for [Blu-ray discs](#blu-ray-discs);
- its size is that of the main title's set, its `.VOB` files;
- a link covers the disc folder; the unmatched files list it as `DVD: …`;
- through the Jellyfin API, its version is named after the disc folder.

The DVD is read from its folder rather than as one file, so it always plays converted: its video is converted over HLS, as an MPEG-TS file's is, and its audio is copied when the app takes it. A user who may not have video converted is not offered it.

Limits:

- DVDs on network shares are left out: FFmpeg reads a DVD from a path. Mount the share on the host and declare it as a local folder instead.
- There is no direct play, and no download.
- A DVD's subtitles are pictures (VobSub). They are not offered as files, so an app that draws subtitles itself gets none; an app that cannot show them gets them burned into the video.
- No thumbnails or chapter images are made for a DVD.
- A DVD holding several movies plays its longest title only, and in a shows folder, DVDs are left out, as discs are.
- The DVD must not be encrypted: Polyfin reads DVDs copied without their copy protection.

## Matching

Each file, each disc, or each show's folder, is matched to a title:

1. a link made by hand (see below);
2. else an identifier written in its name: an IMDb identifier as it is; a TMDB identifier through the metadata addons, which give its IMDb identifier;
3. else an identifier its NFO file gives (see [NFO files](#nfo-files)), read the same way, the IMDb identifier when it gives both;
4. else its name and year are searched in the server's metadata addons, the searches the metadata editor uses: the match is kept only when the addons find exactly one title of that name and kind, of that year when the name gives one.

The title is kept under its IMDb identifier, which the addons answer streams for, with its TMDB identifier when known. Without a metadata addon offering searches, only identifiers in names and NFO files, and links, match files.

A folder's figures tell how its matched files were matched: by an identifier in the name, by an NFO file, by a search or by hand.

## Unmatched files

A folder's page lists the files no title was found for, with why: the name gives no title or episode number, no title of that name, several titles of that name (add the year), titles of other years, the search failed, or the file is another movie of a Blu-ray disc, which only a link matches. Rename the file and scan again, add an NFO file giving its identifier, or type the IMDb identifier of the title (the `tt…` part of its IMDb address) beside it and choose **Link**. In a shows folder, the link covers the whole show folder; for a Blu-ray disc or a DVD, the whole disc folder, or one title of a disc holding several movies. Links survive every scan, even when the file changes or is moved away and back; remove one from the folder's page to match it by its name or NFO file again. While the folder is scanned, the list follows the scan: the files found are added to it as they come, and an identifier being typed stays, so files can be linked before the scan ends.

To find the title without leaving Polyfin, choose **Find the title** beside a file. The search is filled with what the file's name was read as, and its year; change them and choose **Search**, or press Enter. Polyfin searches the server's metadata addons for titles of the folder's kind, movies or shows, and lists at most 10 with their poster, name, year and kind, those of the year given first. Choosing one links the file, its show folder or its disc, as typing its identifier does. Only titles with an IMDb identifier are listed, since links are made to IMDb identifiers; for a title the search does not find, type its identifier.

## Scanning

Folders are scanned at startup, every **Settings › Catalogs › Scan local folders every (hours, 0 = never)** (6 by default, 0 turns the schedule off), and when an administrator chooses **Scan now** on a folder or runs **Scan local folders** under **System › Schedule** (which scans every folder).

### Watching for changes

Folders mounted in the container are also watched for changes, with inotify on Linux. When files are added, renamed or removed in a folder, or in any folder under it, Polyfin waits until they stop changing for a few seconds, then scans that folder again. A movie copied into a folder shows in its library within a minute of the copy's end, without **Scan now**.

- **Settings › Catalogs › Watch local folders for changes** turns it off or on (on by default). Off, no folder is watched.
- Network shares (SMB and WebDAV) are not watched: they follow the schedule only.
- The schedule and **Scan now** stay, as a safety net for changes a watch misses.
- A folder that disappears is scanned, which reports it missing; when it is back, it is scanned and watched again within a minute.
- Hidden folders and the folders a scan leaves out, such as `Extras`, are not watched, and changes to files other than videos, NFO files, subtitle files, a DVD's `.IFO` and `.VOB` files and a Blu-ray disc's playlists do not start a scan.

Linux limits the folders one user may watch, with `fs.inotify.max_user_watches`. Polyfin watches each folder of a tree, so a large library may reach it. A folder that cannot be watched is still scanned on its schedule; **System › Health** lists it with why, and Polyfin tries again every 10 minutes. The container shares the host's limits, so raise it on the host:

```sh
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-inotify.conf
sudo sysctl --system
```

When Health says the limit of watchers or of open files is reached, raise `fs.inotify.max_user_instances` the same way, or the container's limit of open files (`--ulimit nofile=…` with Docker).

A scan reads only what changed: a file is new or changed by its path, size and modification time; files gone are forgotten; files whose title is known keep it without asking the addons again. An NFO file is read when it is new or changed the same way; one added, changed or removed matches its movie, disc or show again, even when the videos did not change, and one removed leaves it to its name and the search. Subtitle files added, changed or removed are offered as they are now from the next scan on. Files left unmatched are matched again at each scan. A few titles are looked up at once, so a large first scan takes a while; the folder's figures follow it. On a share, NFO and subtitle files are listed with the folders, and only the NFO files new or changed are read.

Symbolic links to files of the folder are followed; links leading out of the folder, and links to folders, are not.

## Versions

A matched file is one more version of its title, or of each of its episodes, named after the folder, with the resolution its name gives and its size, such as `Movies · 1080p · 4.2 GB`. Polyfin reads it from the disk or the share, with byte ranges, and plays it like any other version: direct play, remux or conversion, with its embedded audio and subtitle tracks and the subtitle files beside it. A Blu-ray disc's playlist and a DVD play converted only (see [Blu-ray discs](#blu-ray-discs) and [DVD folders](#dvd-folders)). The folder's library lists its titles, the latest found first.

## Compared with Jellyfin

- Jellyfin reads metadata from files and providers of its own; Polyfin takes titles and their descriptions from the metadata addons, and a local file only adds a version.
- Both watch folders for changes. Polyfin also scans them at startup, on its schedule and on demand, and does not watch network shares it reads itself.
- Jellyfin's "Identify" matches an item by hand from a search of its providers; Polyfin links an unmatched file to an IMDb identifier, typed or chosen from a search of the metadata addons.
- Both read the identifiers of NFO files; Polyfin reads nothing else of them, neither names, plots nor artwork.
- Both offer the text subtitle files beside a video, named after it. Polyfin reads only those in the video's own folder, and no picture subtitle files (`.sup`, `.idx` and `.sub`). Local artwork is not read.

## For app developers

A local folder or network share is an addon of kind `local` in the admin API, listed with the other sources (`GET /admin/api/scopes/shared/addons`), its `folder` field describing its path or address, kind, last scan, error, counts and links. `matchedBy` counts its matched files by how they were matched: `{"name", "nfo", "search", "link"}`. For a share, `share` is `"smb"` or `"webdav"` (empty for a local folder), `user` its user, and `passwordSet` whether a password is stored; the password itself is never answered. `error` is `missing`, `unreadable`, `not_folder`, or for a share `unreachable` or `refused`; empty after a scan that could read the folder. The administrator routes are:

| Route | What it does |
|---|---|
| `POST /admin/api/scopes/shared/folders` | adds a folder: `{"name", "path", "kind": "movies" or "shows"}`, and for a share `"user"` and `"password"`; `path` is a path in the container or a share's address |
| `PATCH /admin/api/scopes/shared/folders/{id}` | changes its `name`, `path`, `user` or `password`; a new path or address forgets the files found; a `password` left out keeps the stored one, an empty one removes it |
| `POST /admin/api/scopes/shared/folders/{id}/scan` | starts a scan (202) |
| `GET /admin/api/scopes/shared/folders/{id}/unmatched` | `{"total", "files": [{"path", "unit", "title", "year", "size", "reason", "disc"}]}`; `disc` is `"bluray"` for a Blu-ray disc's playlist or stream and `"dvd"` for a DVD, its `unit` the disc folder, or for a title other than a disc's main one the disc folder, `#` and its playlist's number (`Box/DISC02#00002`), and `null` for other files |
| `GET /admin/api/scopes/shared/folders/{id}/titles?query=…&year=…` | searches the metadata addons for titles of the folder's kind: `{"titles": [{"imdbId", "name", "year", "kind", "posterId"}]}`, at most 10, only titles with an IMDb identifier, those of `year` (optional) first; `kind` is `movie` or `show`, and `posterId` the item whose Primary image (`/Items/{posterId}/Images/Primary`) is the poster, null without one. A failed search answers 502, `addon_unreachable` when an addon could not be reached, `title_search_failed` otherwise |
| `PUT /admin/api/scopes/shared/folders/{id}/links` | links `{"path", "imdbId"}`; `path` is a file, a show's folder or a disc's folder |
| `DELETE /admin/api/scopes/shared/folders/{id}/links?path=…` | removes a link |

An address that is not `smb://host[:port]/share[/path]` or `http(s)://host/path`, or that holds a user or password, is refused with `invalid_share_address`; a user longer than 256 characters or a password longer than 1,024 with `invalid_share_user`. On **System › Health**, the problem of a folder (`code` `folder`) carries its error as `failure`, and `share` for a share. A folder in the container that cannot be watched is a problem of its own (`code` `folder_unwatched`), its `failure` being `watch_limit` (the limit of watches, ENOSPC), `instance_limit` (the limit of watchers or of open files, EMFILE) or `failed`. A share's password the key cannot decrypt is listed among the stored secrets as `{"folder": name}`.

Through the Jellyfin API, a local file is a media source of its title like any other, its `Name` the folder's name and its `Path` its file name. A Blu-ray disc's `Path` is its disc folder's name with the `.m2ts` extension, such as `Nosferatu (1922).m2ts`, or `Nosferatu (1922) - Disc 1.m2ts` for a folder named after the disc's number, and `Box - DISC02 - title 2.m2ts` for a title other than its main one. A playlist's media source, as a DVD's, has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A DVD's `Path` is its disc folder's name with its main title's `.VOB` extension, such as `Nosferatu (1922).VOB`; its media source has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A share's files are served the same way.
