# Local folders

Polyfin serves your own video files beside what addons and IPTV sources provide. An administrator declares folders mounted in Polyfin's container; each file is matched to a title and becomes one more version of it, next to the addons' streams, with the same version picker, subtitles, audio tracks and transcoding. Each folder is also a library apps see, listing the titles found in it.

Network shares (SMB, WebDAV) are not read by Polyfin itself: mount them on the host, or as Docker volumes, and declare the folder they appear in.

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

Then, in the admin app, open **Sources**, choose **Add a source › Local folder**, and give its name (the library's name in apps), its path in the container (`/media/movies`, not the host's path), and whether it holds movies or shows. What a folder holds cannot be changed afterwards: remove it and add it again.

### Permissions

Polyfin runs as user 65532 (group 65532). It must be able to read every file and to open every folder under the path: on the host, files need the "read" permission and folders "read" and "execute" for that user, its group or everyone, such as `chmod -R a+rX /srv/media`. Polyfin never writes to these folders.

A folder Polyfin cannot read is reported on its page in the admin app and under **System › Health**:

- the path does not exist in the container, usually a share that is not mounted (`missing`);
- it exists but Polyfin may not list it, a permissions problem (`unreadable`);
- the path is a file (`not_folder`).

The files found before stay in the library while the folder cannot be read, so a share that is unmounted for a while empties no library.

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

Folders are scanned at startup, every **Settings › Catalogs › Scan local folders every (hours, 0 = never)** (6 by default, 0 turns the schedule off), and when an administrator chooses **Scan now** on a folder or runs **Scan local folders** under **System › Schedule** (which scans every folder). Changes are not watched as they happen.

A scan reads only what changed: a file is new or changed by its path, size and modification time; files gone are forgotten; files whose title is known keep it without asking the addons again. Files left unmatched are matched again at each scan. A few titles are looked up at once, so a large first scan takes a while; the folder's figures follow it.

Symbolic links to files of the folder are followed; links leading out of the folder, and links to folders, are not.

## Versions

A matched file is one more version of its title, or of each of its episodes, named after the folder, with the resolution its name gives and its size, such as `Movies · 1080p · 4.2 GB`. Polyfin reads it from the disk, with byte ranges, and plays it like any other version: direct play, remux or conversion, with its embedded audio and subtitle tracks. The folder's library lists its titles, the latest found first.

## Compared with Jellyfin

- Jellyfin reads metadata from files and providers of its own; Polyfin takes titles and their descriptions from the metadata addons, and a local file only adds a version.
- Jellyfin watches folders for changes; Polyfin scans them at startup, on its schedule and on demand.
- Jellyfin's "Identify" matches an item by hand; Polyfin links an unmatched file to an IMDb identifier.
- NFO files, local artwork and side-car subtitle files are not read.

## For app developers

A local folder is an addon of kind `local` in the admin API, listed with the other sources (`GET /admin/api/scopes/shared/addons`), its `folder` field describing its path, kind, last scan, error, counts and links. The administrator routes are:

| Route | What it does |
|---|---|
| `POST /admin/api/scopes/shared/folders` | adds a folder: `{"name", "path", "kind": "movies" or "shows"}` |
| `PATCH /admin/api/scopes/shared/folders/{id}` | changes its `name` or `path`; a new path forgets the files found |
| `POST /admin/api/scopes/shared/folders/{id}/scan` | starts a scan (202) |
| `GET /admin/api/scopes/shared/folders/{id}/unmatched` | `{"total", "files": [{"path", "unit", "title", "year", "size", "reason"}]}` |
| `PUT /admin/api/scopes/shared/folders/{id}/links` | links `{"path", "imdbId"}`; `path` is a file or a show's folder |
| `DELETE /admin/api/scopes/shared/folders/{id}/links?path=…` | removes a link |

Through the Jellyfin API, a local file is a media source of its title like any other, its `Name` the folder's name and its `Path` its file name.
