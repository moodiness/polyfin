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

## What apps get

Jellyfin apps get the rest of what they ask a movie, series and Live TV server for.

### Title pages

A title's page opens as soon as its description is ready, with the versions Polyfin already knows, even from lists that expired, while it asks the stream addons without a current list in the background. The web player adds their versions as each addon answers. Other apps show the versions known when the page opened, and get them all when the user presses Play or opens the page again. See [Title pages](playback.md#title-pages).

**For app developers:** `/Polyfin/Items/{id}/Versions` tells how many addons are still asked, for the first time or again, how many media sources the title's details list now, and how many versions are known, and Polyfin pushes the same as a `PolyfinVersions` message on the live connection (WebSocket) as it changes. `POST /Polyfin/Items/{id}/Versions/Search` has the title's addons asked again, once every 20 seconds at most for a user's title (see [Title pages](playback.md#title-pages)).

### Streamyfin home

Under **Content › Streamyfin home**, administrators choose the rows of Streamyfin's home screen, in order. Each row shows one of these:

- **Continue watching**: the titles the user started and did not finish;
- **Next up**: the next episodes of the series the user watches;
- **A library**: a movie or series library shows its titles, a collection library its collections;
- **A collection's titles**: the movies and series of one collection of a collection library, those of the collections within it included.

Each row takes a title of its own. Without one, it is named after its library or collection, and Continue watching and Next up are named as in jellyfin-web, in the server's language. **Suggested rows** fills an empty list with Continue watching, Next up and a row for each library.

- Each user sees only the rows of the libraries and collections they can see.
- With no row, or none the user can see, Streamyfin shows its own home screen.
- Streamyfin reads the rows when it comes back to the foreground, or when its home screen is pulled down.
- Users cannot change them in the app.

Other apps keep their own home screens.

**Compared with Jellyfin:** Streamyfin's server plugin lets a Jellyfin administrator write these rows by hand. Polyfin answers the plugin's settings itself, so no plugin is needed. It sends only the home screen, and none of the plugin's other settings.

**For app developers:**

- `GET /Streamyfin/config` answers `{"settings": {"home": {"locked": true, "value": {"sections": [...]}}}}`, or 404 without a row the user can see.
- Continue watching is a `custom` section of `/UserItems/Resume`, and Next up a `nextUp` section.
- A library or collection row is an `items` section with its `parentId`. A collection row asks for `Movie` and `Series` items, which a recursive listing of a collection gives through the collections within it.
- Rows of collections show wide images (`"orientation": "horizontal"`), rows of titles posters.
- The setting is locked: Streamyfin keeps no copy of it, and shows its own home screen again as soon as Polyfin answers none.
- The admin app reads and saves the rows at `GET` and `PUT /admin/api/streamyfin`, administrators only, and lists a collection library's collections at `/admin/api/streamyfin/libraries/{id}/collections`.

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

All results stay within the user's libraries, parental control and blocked genres.

**Compared with Jellyfin:**

- Jellyfin ranks every kind together; Polyfin keeps the order above.

**For app developers:** `/Search/Hints` answers from the same search as `/Items`.

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

**For app developers:** `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/0/Stream.pgssub` (or `.sup`) answers `application/octet-stream` and supports `Range`.

### Jellyfin 12.2 details

- **Codec tags:** each track's `CodecTag` is the tag ffprobe reads, such as `avc1` or `mp4a` in MP4. Matroska's empty tags are left out, and live channels report none. Attached files report theirs as they are, `[0][0][0][0]` in Matroska. Direct play checks a profile's `VideoCodecTag` conditions against the file's tag.
- **Rewatching:** starting a played movie, episode or audiobook again does not change its last played date. The date changes once the position passes the start, or when the title is played to the end. Next Up therefore does not move past an episode that was only just started.
- **Letter filters:** `nameStartsWith`, `nameLessThan` and `nameStartsWithOrGreater` ignore letter case. They narrow people, music, favorites and other personal lists, and playlists. Movie and series libraries come from addons' catalogs, so they are not narrowed.
- **People counts:** `/Persons` counts only the people it can list.
- **Downmix:** music and audiobooks converted to stereo go through the downmix chosen in the server settings, as the audio of converted video does.

### Refresh metadata

**Refresh metadata** (administrators only) makes Polyfin forget the title's description, version and subtitle lists, expired ones included, and the rating it looked up, and stop asking addons again for those lists. It then asks the addons for the description again at once.

**For app developers:** `POST /Items/{id}/Refresh`.

### Administration from Jellyfin apps

API keys, devices, user management, server configuration, the activity log, logs and scheduled tasks are covered in [administration](administration.md#api-keys-and-integrations).

## Jellyfin features Polyfin does without

These dashboard features answer as a Jellyfin 12.2 on which nothing of the kind is configured:

- plugins, packages and plugin repositories;
- browsing the server's folders;
- library folders and metadata providers;
- Live TV tuners and listing providers;
- the startup wizard;
- backups.

Polyfin never shows the host's folders. To change the server's settings, use the [admin app](administration.md#settings) or the server configuration API.

**For app developers:**

- Answers go to administrators only where Jellyfin requires one.
- `/Plugins`, `/Packages`, `/Repositories`, `/Environment/Drives`, `/Environment/DirectoryContents`, `/Library/PhysicalPaths`, `/LiveTv/Tuners/Discover`, `/LiveTv/TunerHosts/Types` and `/Backup` list nothing.
- `/Libraries/AvailableOptions` lists no provider (Jellyfin also lists its own, its NFO reader and saver among them).
- `/System/Configuration/MetadataOptions/Default` and `/LiveTv/ListingProviders/Default` are Jellyfin's defaults.
- `/Startup/Configuration` and `/Startup/User` show the server name, language and earliest administrator; setup is always complete.
- Writes are refused as Jellyfin refuses them for what does not exist (an unknown plugin or package, path, tuner, listing provider or backup).
- Writes Jellyfin would perform answer 403: saving plugin repositories, creating a backup, and the wizard's configuration, remote access and first user. Change server settings through `/System/Configuration` instead.
- `/Environment/ValidatePath` answers 404 for every path.
