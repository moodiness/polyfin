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
- Official Jellyfin apps
- Kodi

## What apps get

Jellyfin apps get the rest of what they ask a movie, series and Live TV server for.

### Title pages

A title's page opens as soon as its description is ready, with the versions Polyfin already knows, even from lists that expired, while it asks the stream addons without a current list in the background. The web player adds their versions as each addon answers. Other apps show the versions known when the page opened, and get them all when the user presses Play or opens the page again. See [Title pages](playback.md#title-pages).

**For app developers:** `/Polyfin/Items/{id}/Versions` tells how many addons are still asked, for the first time or again, how many media sources the title's details list now, and how many versions are known, and Polyfin pushes the same as a `PolyfinVersions` message on the live connection (WebSocket) as it changes. `POST /Polyfin/Items/{id}/Versions/Search` has the title's addons asked again, once every 20 seconds at most for a user's title (see [Title pages](playback.md#title-pages)).

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
