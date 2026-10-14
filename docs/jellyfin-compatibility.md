# Jellyfin compatibility

Polyfin targets the Jellyfin 12.2 API, so standard Jellyfin apps sign in, browse, search and play. This page lists the compatible apps, what they get from Polyfin, and the Jellyfin features Polyfin does without.

## Compatible apps

Polyfin targets the apps that connect to a Jellyfin server, including:

- Infuse
- Swiftfin
- Findroid
- Streamyfin
- Nuvio
- Strand
- Odin
- VidHub
- Official Jellyfin apps
- Kodi

Apps reach Polyfin at its address, such as `http://192.168.1.10:8096`, and also under `/jellyfin` at that address, such as `http://192.168.1.10:8096/jellyfin`, where a Jellyfin server behind a reverse proxy often lives: given an address without a path, some apps try it there first. Under `/jellyfin`, in any letter case, the Jellyfin API, its WebSocket at `/jellyfin/socket` and the web client at `/jellyfin/web/` answer as at the root. The admin app, `/healthz` and `/readyz` are at the root only.

## What apps get

Jellyfin apps get the rest of what they ask a movie, series and Live TV server for.

### Title pages

A title's page opens as soon as its description is ready, with the versions Polyfin already knows, even from lists that expired, while it asks the stream addons without a current list in the background. The web player adds their versions as each addon answers. Other apps show the versions known when the page opened, and get them all when the user presses Play or opens the page again. See [Title pages](playback.md#title-pages).

**For app developers:** `/Polyfin/Items/{id}/Versions` tells how many addons are still asked, for the first time or again, how many media sources the title's details list now, how many versions are known, and whether the web player adds versions to a page already open (`AddVersions`), and Polyfin pushes the same as a `PolyfinVersions` message on the live connection (WebSocket) as it changes. `POST /Polyfin/Items/{id}/Versions/Search` has the title's addons asked again, once every 20 seconds at most for a user's title (see [Title pages](playback.md#title-pages)).

### Ratings

With the server's **MDBList API key for ratings**, `CommunityRating` carries a movie's or series' IMDb rating from MDBList (0 to 10) and `CriticRating` its critics' score or audience score (0 to 100), as **Critic rating shows** chooses; an administrator's edit still wins. Listings that fit on one page, addons' collections read whole among them, sort by `SortBy=CriticRating` as by `CommunityRating`, titles without one first. The Watchlist and smart lists keep their own order (see [Smart lists](addons-and-libraries.md#smart-lists)). See [Ratings](addons-and-libraries.md#ratings).

**For app developers:** `/Polyfin/Items/{id}/Ratings` answers a title's `CommunityRating`, `CriticsScore` and `AudienceScore` (null when it has none, all null without a key) and `Pending` while MDBList is being asked about it.

### Libraries left out of the web player menus

On the admin app's **Libraries** page, an administrator can leave any of the server's libraries out of the web player's menus, and each user any of their own: its top bar, the bar's **More** menu and its side menu. The library keeps its row on the home screen, and other apps keep listing it. Live TV catalogs have no such option: their channels go to Live TV.

**For app developers:** `GET /Polyfin/UserViews` lists the views `/UserViews` gives the caller, in the same order, as `{"Items": [{"Id": <the view's Id>, "HideInMenus": <true when the web player's menus leave it out>}]}`. Polyfin's own Live TV, Playlists, Collections and Watchlist views are never left out.

### Continue Watching and hidden titles

Jellyfin apps have no button to take a title off Continue Watching without marking it played or unplayed, nor to hide a title from one's lists. The web player gets both in its ⋮ menus, and the admin app's **My account** lists both for the other apps (see [Watch state](users.md#watch-state)). Apps see the result at their next refresh: the title is gone from `/UserItems/Resume`, and a hidden title from every listing; it still opens by its identifier. Apps that keep the live connection (WebSocket) open are told of a `UserDataChanged` for the item, as for any other change.

**For app developers:** `DELETE /Polyfin/UserItems/{id}/Resume` takes the item off Continue Watching, a series or a season its episodes, and answers its `UserItemData`. `POST /Polyfin/UserItems/{id}/Hidden` hides a movie, a home video or a series, a season or an episode standing for its series, and `DELETE` shows it again; both answer 204. `GET /Polyfin/UserItems/Hidden` lists the hidden titles, the latest hidden first, as a listing describes them.

### Profile pictures

A user changes their own profile picture from their app; an administrator can change anyone's.

- Polyfin accepts a JPEG, PNG or WebP picture of up to 5 MB and keeps it in the database.
- It is scaled down to 512 pixels and stored as JPEG, or PNG when it is transparent.
- It shows in users, sign-in screens and sessions, and on the admin app's **Users** page.

**For app developers:** `/UserImage`, and the older `/Users/{id}/Images/{type}`.

### Search hints

Search hints answer from the same search as the main item search, in this order:

1. the movies and series the addons find;
2. the episodes Polyfin knows by name from series already opened (addons search titles, not episodes);
3. the people credited in titles the user reaches.

All results stay within the user's libraries, parental control and blocked genres. Channels and programmes, which the item search also finds (see [Searching](live-tv.md#searching)), are left out of search hints.

**Compared with Jellyfin:**

- Jellyfin ranks every kind together; Polyfin keeps the order above.

**For app developers:** `/Search/Hints` answers from the same search as `/Items`, without its channels and programmes.

### Movie recommendations

For each of the last movies the user played, recommendations give the "Because you watched" titles similar to it. They come from the similar titles of title pages, without those already played. With **Similar titles** off there are none.

**Compared with Jellyfin:**

- Jellyfin's categories for liked titles and for the people of recent movies are left out: each would cost a search of the addons.

**For app developers:** `/Movies/Recommendations`.

### Years, groupings, artwork and viewing

**For app developers:**

- `/Years` lists the years the year pages offer (catalogs whose genre filter offers years).
- `/UserViews/GroupingOptions` lists the movie and series libraries.
- `/Items/{id}/Images` lists a title's artwork.
- `/Sessions/Viewing` records the item an app shows, which **Sessions** gives as `NowViewingItem`.

### Trailers, channels and theme media

Trailer items, plugin channels, and theme songs and videos answer empty lists, as on a Jellyfin server without them. Addons give trailers only with their titles.

### Opening streams and downloading files

Sources need no opening. A title's file download serves its first working version, as the download route does, under the same permissions.

**Compared with Jellyfin:**

- Jellyfin does not ask for these permissions for its own files.

**For app developers:**

- `/LiveStreams/Open` answers 400, as Jellyfin does without an open token; `/LiveStreams/Close` answers 204.
- `/Items/{id}/File` serves the first working version.

### Fallback fonts

Fallback fonts are the fonts in `POLYFIN_FONTS_DIR`, folders within included. The image ships DejaVu (Latin, Greek and Cyrillic). Mount more fonts there for other scripts, for example CJK.

**Compared with Jellyfin:**

- Jellyfin reads only the top of its folder.

**For app developers:** `/FallbackFont/Fonts`.

### Subtitle files

On a user's page under **Users**, the **Access** section's **Can manage subtitles** permission is on for administrators and off for other users. Administrators' apps can set it too. See [users](users.md) and [subtitles](subtitles.md).

- With it, a user can add an SRT, WebVTT, ASS or SSA file to a movie or an episode from their app. Every version then offers that file first among its subtitles.
- Only administrators delete such files, as in Jellyfin.
- Addon subtitles and tracks inside files cannot be deleted.
- The subtitle search stays open to every user, as it lists only what every version already offers. Apps show it only with the permission.

**Compared with Jellyfin:**

- The permission is Jellyfin's `EnableSubtitleManagement`.
- Jellyfin's subtitle search is not open to every user.

**For app developers:**

- Add: `POST /Videos/{id}/Subtitles`.
- Delete: `DELETE /Videos/{id}/Subtitles/{index}`; addon subtitles and embedded tracks answer 400.

### Picture subtitles drawn by the app

When jellyfin-web is set to draw PGS subtitles itself, a PGS track inside a Matroska file reaches it as a SUP file, as Jellyfin 12.2 serves its raw file. Polyfin reads the track whole through the file's index, as it does for text tracks, and keeps it for later playbacks. The app reads the file by ranges as it plays.

- A track the index does not list block by block, or larger than 32 MB, is not offered as a file.
- DVD (VobSub) tracks are not offered as files.
- A PGS file (`.sup`) beside a local file reaches the app as it is, by ranges. A VobSub pair beside a local file (`.idx` and `.sub`) is burned in, as Jellyfin 12.2 does: it gives VobSub as a file only extracted as Matroska.

**For app developers:** `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/0/Stream.pgssub` (or `.sup`) answers `application/octet-stream` and supports `Range`.

### Jellyfin 12.2 details

- **Codec tags:** each track's `CodecTag` is the tag ffprobe reads, such as `avc1` or `mp4a` in MP4. Matroska's empty tags are left out, and live channels report none. Attached files report theirs as they are, `[0][0][0][0]` in Matroska. Direct play checks a profile's `VideoCodecTag` conditions against the file's tag.
- **Rewatching:** starting a played movie, episode or audiobook again does not change its last played date. The date changes once the position passes the start, or when the title is played to the end. Next Up therefore does not move past an episode that was only just started.
- **Letter filters:** `nameStartsWith`, `nameLessThan` and `nameStartsWithOrGreater` ignore letter case. They narrow people, music, favorites and other personal lists, and playlists. Movie and series libraries come from addons' catalogs, so they are not narrowed.
- **People counts:** `/Persons` counts only the people it can list.
- **Downmix:** music and audiobooks converted to stereo go through the downmix chosen in the server settings, as the audio of converted video does.

### Refresh metadata

**Refresh metadata** (administrators only) makes Polyfin forget the title's description, version and subtitle lists, expired ones included, the rating it looked up and its ratings from MDBList, and stop asking addons again for those lists. It then asks the addons for the description again at once.

**For app developers:** `POST /Items/{id}/Refresh`.

### Administration from Jellyfin apps

API keys, devices, user management, server configuration, the activity log, logs and scheduled tasks are covered in [administration](administration.md#api-keys-and-integrations).

### Client addresses behind a reverse proxy

Like Jellyfin's own Known proxies setting, **Known proxies**, under **Settings › Security**, lists the reverse proxies whose forwarded headers name the client of a request. See [Known proxies](reverse-proxy.md#known-proxies).

**For app developers:** a session's `RemoteEndPoint`, and `IsLocal` and `IsInNetwork` of `/System/Endpoint`, follow the client's address: the one the proxy names for a request from a known proxy, the connection's otherwise.

### Finding servers on the local network

Jellyfin apps that look for servers on the local network list Polyfin, as they list a Jellyfin server with its auto discovery on: its address does not have to be typed. **Answer local network discovery**, under **Settings › General**, is on by default, as Jellyfin's is, and **Address on the local network** plays the part of Jellyfin's `PublishedServerUrl`. In Docker, the container's network decides whether the apps' messages reach Polyfin: see [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network) and [Local network discovery](administration.md#local-network-discovery).

**For app developers:**

- Polyfin listens on UDP port 7359, IPv4 and IPv6. A datagram that holds `who is JellyfinServer?`, in any letter case, as Jellyfin 12.2 reads it, gets one answer, sent to its sender: `{"Address":"http://192.168.1.10:8096","Id":"<server ID>","Name":"<server name>","EndpointAddress":null}`. Polyfin answers Emby's `who is EmbyServer?` the same way, which Jellyfin 12.2 leaves unanswered.
- `Id` and `Name` are those of `/System/Info/Public`. `Address` is **Address on the local network** when set, as Jellyfin's `PublishedServerUrl`. Otherwise it is `http://`, the address of the server's interface that reaches the sender, and Polyfin's HTTP port.
- Only senders on the local network are answered: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback. Jellyfin answers any sender.
- `LocalAddress` of `/System/Info/Public` and `/System/Info` is **Address on the local network** when set, as Jellyfin gives its `PublishedServerUrl` there, and otherwise the address the request reached.

### Backups

The backups page of the web client's dashboard, since Jellyfin 10.11, shows the database backups of the backups folder (see [Backups](backups.md)): it lists them, makes one, and restores one. Polyfin's backups hold its database alone, whatever the options asked: Jellyfin's also hold its metadata, trickplay images and subtitles, which Polyfin does not keep on disk.

**For app developers:**

- Administrators only, as in Jellyfin. While backups are off, `/Backup` lists nothing, `/Backup/Create` answers 403, and `/Backup/Restore` answers 404.
- `GET /Backup` lists the backups of the folder, newest first, as `BackupManifestDto`: `DateCreated` (when the backup was made), `Path` (the file name, such as `polyfin-20261005-040000.dump`; Jellyfin gives the archive's full path), `ServerVersion` (the Jellyfin version of `/System/Info`), `BackupEngineVersion` (`1.0.0`, Polyfin's format), and `Options` with only `Database` true. The backups made before a restore are listed too.
- `POST /Backup/Create` runs **Back up the database** at once, sends the backup to the destinations, and answers its manifest. The options of the body are ignored.
- `GET /Backup/Manifest?path=` answers the manifest of the backup of the folder named `path`, and 404 for any other path.
- `POST /Backup/Restore` with `ArchiveFileName`, a file name `/Backup` lists, restores it as the admin app does, and answers 204: Polyfin backs the current database up, stops, restores it and starts again, answering 503 meanwhile. It answers 404 for a backup the folder does not have, and 400 for a file that is not a backup of Polyfin's database, a backup of a newer Polyfin, a server without `pg_restore`, a restore already under way, and a backup holding stored keys the current `POLYFIN_SECRET_KEY` cannot decrypt, as apps cannot show that warning: the admin app restores those.

## Jellyfin features Polyfin does without

These dashboard features answer as a Jellyfin 12.2 on which nothing of the kind is configured:

- plugins, packages and plugin repositories;
- browsing the server's folders;
- library folders and metadata providers;
- Live TV tuners and listing providers;
- the startup wizard.

Polyfin never shows the host's folders. To change the server's settings, use the [admin app](administration.md#settings) or the server configuration API.

**For app developers:**

- Answers go to administrators only where Jellyfin requires one.
- `/Plugins`, `/Packages`, `/Repositories`, `/Environment/Drives`, `/Environment/DirectoryContents`, `/Library/PhysicalPaths`, `/LiveTv/Tuners/Discover` and `/LiveTv/TunerHosts/Types` list nothing.
- `/Libraries/AvailableOptions` lists no provider (Jellyfin also lists its own, its NFO reader and saver among them).
- `/System/Configuration/MetadataOptions/Default` and `/LiveTv/ListingProviders/Default` are Jellyfin's defaults.
- `/Startup/Configuration` and `/Startup/User` show the server name, language and earliest administrator; setup is always complete.
- Writes are refused as Jellyfin refuses them for what does not exist (an unknown plugin or package, path, tuner or listing provider).
- Writes Jellyfin would perform answer 403: saving plugin repositories, and the wizard's configuration, remote access and first user. Change server settings through `/System/Configuration` instead.
- `/Environment/ValidatePath` answers 404 for every path.
