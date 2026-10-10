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

Movies, one file per movie, alone or in a folder of its own:

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

Files whose names end in `sample`, hidden folders, and folders named `Extras`, `Featurettes`, `Trailers`, `Behind The Scenes`, `Deleted Scenes`, `Interviews` or `Samples` are left out.

## Matching

Each file, or each show's folder, is matched to a title:

1. a link made by hand (see below);
2. else an identifier written in its name: an IMDb identifier as it is; a TMDB identifier through the metadata addons, which give its IMDb identifier;
3. else its name and year are searched in the server's metadata addons, the searches the metadata editor uses: the match is kept only when the addons find exactly one title of that name and kind, of that year when the name gives one.

The title is kept under its IMDb identifier, which the addons answer streams for, with its TMDB identifier when known. Without a metadata addon offering searches, only identifiers in names and links match files.

## Unmatched files

A folder's page lists the files no title was found for, with why: the name gives no title or episode number, no title of that name, several titles of that name (add the year), titles of other years, or the search failed. Rename the file and scan again, or type the IMDb identifier of the title (the `tt…` part of its IMDb address) beside it and choose **Link**. In a shows folder, the link covers the whole show folder. Links survive every scan, even when the file changes or is moved away and back; remove one from the folder's page to match it by its name again.

## Scanning

Folders are scanned at startup, every **Settings › Catalogs › Scan local folders every (hours, 0 = never)** (6 by default, 0 turns the schedule off), and when an administrator chooses **Scan now** on a folder or runs **Scan local folders** under **System › Schedule** (which scans every folder).

### Watching for changes

Folders mounted in the container are also watched for changes, with inotify on Linux. When files are added, renamed or removed in a folder, or in any folder under it, Polyfin waits until they stop changing for a few seconds, then scans that folder again. A movie copied into a folder shows in its library within a minute of the copy's end, without **Scan now**.

- **Settings › Catalogs › Watch local folders for changes** turns it off or on (on by default). Off, no folder is watched.
- Network shares (SMB and WebDAV) are not watched: they follow the schedule only.
- The schedule and **Scan now** stay, as a safety net for changes a watch misses.
- A folder that disappears is scanned, which reports it missing; when it is back, it is scanned and watched again within a minute.
- Hidden folders and the folders a scan leaves out, such as `Extras`, are not watched, and changes to files other than videos do not start a scan.

Linux limits the folders one user may watch, with `fs.inotify.max_user_watches`. Polyfin watches each folder of a tree, so a large library may reach it. A folder that cannot be watched is still scanned on its schedule; **System › Health** lists it with why, and Polyfin tries again every 10 minutes. The container shares the host's limits, so raise it on the host:

```sh
echo fs.inotify.max_user_watches=524288 | sudo tee /etc/sysctl.d/90-inotify.conf
sudo sysctl --system
```

When Health says the limit of watchers or of open files is reached, raise `fs.inotify.max_user_instances` the same way, or the container's limit of open files (`--ulimit nofile=…` with Docker).

A scan reads only what changed: a file is new or changed by its path, size and modification time; files gone are forgotten; files whose title is known keep it without asking the addons again. Files left unmatched are matched again at each scan. A few titles are looked up at once, so a large first scan takes a while; the folder's figures follow it.

Symbolic links to files of the folder are followed; links leading out of the folder, and links to folders, are not.

## Versions

A matched file is one more version of its title, or of each of its episodes, named after the folder, with the resolution its name gives and its size, such as `Movies · 1080p · 4.2 GB`. Polyfin reads it from the disk or the share, with byte ranges, and plays it like any other version: direct play, remux or conversion, with its embedded audio and subtitle tracks. The folder's library lists its titles, the latest found first.

## Compared with Jellyfin

- Jellyfin reads metadata from files and providers of its own; Polyfin takes titles and their descriptions from the metadata addons, and a local file only adds a version.
- Both watch folders for changes. Polyfin also scans them at startup, on its schedule and on demand, and does not watch network shares it reads itself.
- Jellyfin's "Identify" matches an item by hand; Polyfin links an unmatched file to an IMDb identifier.
- NFO files, local artwork and side-car subtitle files are not read.

## For app developers

A local folder or network share is an addon of kind `local` in the admin API, listed with the other sources (`GET /admin/api/scopes/shared/addons`), its `folder` field describing its path or address, kind, last scan, error, counts and links. For a share, `share` is `"smb"` or `"webdav"` (empty for a local folder), `user` its user, and `passwordSet` whether a password is stored; the password itself is never answered. `error` is `missing`, `unreadable`, `not_folder`, or for a share `unreachable` or `refused`; empty after a scan that could read the folder. The administrator routes are:

| Route | What it does |
|---|---|
| `POST /admin/api/scopes/shared/folders` | adds a folder: `{"name", "path", "kind": "movies" or "shows"}`, and for a share `"user"` and `"password"`; `path` is a path in the container or a share's address |
| `PATCH /admin/api/scopes/shared/folders/{id}` | changes its `name`, `path`, `user` or `password`; a new path or address forgets the files found; a `password` left out keeps the stored one, an empty one removes it |
| `POST /admin/api/scopes/shared/folders/{id}/scan` | starts a scan (202) |
| `GET /admin/api/scopes/shared/folders/{id}/unmatched` | `{"total", "files": [{"path", "unit", "title", "year", "size", "reason"}]}` |
| `PUT /admin/api/scopes/shared/folders/{id}/links` | links `{"path", "imdbId"}`; `path` is a file or a show's folder |
| `DELETE /admin/api/scopes/shared/folders/{id}/links?path=…` | removes a link |

An address that is not `smb://host[:port]/share[/path]` or `http(s)://host/path`, or that holds a user or password, is refused with `invalid_share_address`; a user longer than 256 characters or a password longer than 1,024 with `invalid_share_user`. On **System › Health**, the problem of a folder (`code` `folder`) carries its error as `failure`, and `share` for a share. A folder in the container that cannot be watched is a problem of its own (`code` `folder_unwatched`), its `failure` being `watch_limit` (the limit of watches, ENOSPC), `instance_limit` (the limit of watchers or of open files, EMFILE) or `failed`. A share's password the key cannot decrypt is listed among the stored secrets as `{"folder": name}`.

Through the Jellyfin API, a local file is a media source of its title like any other, its `Name` the folder's name and its `Path` its file name. A share's files are served the same way.
