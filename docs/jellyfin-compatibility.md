<a id="jellyfin-compatibility"></a>

# Jellyfin apps

This page is for people who watch Polyfin in a Jellyfin app, and for the administrators who set those apps up. Polyfin targets the Jellyfin 12.4 API, so standard Jellyfin apps sign in, browse, search and play. The page lists the compatible apps, how they sign in and find the server, what they get from Polyfin, and the Jellyfin features Polyfin does without. App developers find the details in [Jellyfin API notes](jellyfin-api.md).

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

Apps reach Polyfin at its address, such as `http://192.168.1.10:8096`. They also reach it under `/jellyfin` at that address, such as `http://192.168.1.10:8096/jellyfin`, where a Jellyfin server behind a reverse proxy often lives: given an address without a path, some apps try it there first. Under `/jellyfin`, in any letter case, the Jellyfin API, its WebSocket at `/jellyfin/socket` and the web player at `/jellyfin/web/` answer as at the root. The admin app, `/healthz` and `/readyz` are at the root only.

## Signing in to apps with Quick Connect

Jellyfin apps ask for a name and a password, never for a code. An account with [two-step sign-in](sign-in-and-security.md#two-step-sign-in), and an administrator while **Require two-step sign-in for administrators** is on, signs in to them with Quick Connect:

1. In the app, choose Quick Connect on the sign-in screen: it shows a 6-digit code.
2. Sign in to the admin app, with the password and the code of your authenticator app, and enter the app's code on the **Quick Connect** page: in the top bar for members, in the account menu for administrators.
3. The app signs in as you.

**Allow Quick Connect**, in **Settings › General**, must stay on. Typing the password in the app is refused as a wrong password is, and counts as one toward the client's failed attempts, so that the answer never tells whether the password was right. The apps signed in before two-step sign-in was turned on stay signed in; sign them out under **My account › Devices** if needed. API keys made by administrators keep working. See also [Jellyfin apps](sign-in-and-security.md#jellyfin-apps) under two-step sign-in.

A code is approved once, as in Jellyfin, which refuses a request already authorized: approved, it waits no more on the **Quick Connect** page, and nobody else can take it. The page also takes the 8-digit codes of Silo's TV apps (see [Signing a TV in with a code](silo-apps.md#signing-a-tv-in-with-a-code)).

**Compared with Jellyfin**

- Jellyfin has no second step of its own. Polyfin keeps Jellyfin's sign-in as it is for the apps, and asks for the code in the admin app only.

API details: [Quick Connect for two-step accounts](jellyfin-api.md#quick-connect-for-two-step-accounts).

## Finding servers on the local network

Jellyfin apps that look for servers on the local network list Polyfin, so its address does not have to be typed: see [Local network discovery](https-and-networking.md#local-network-discovery), and for Docker [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network).

**Compared with Jellyfin**

- Apps list Polyfin as they list a Jellyfin server with its auto discovery on. **Answer local network discovery**, in **Settings › General**, is on by default, as Jellyfin's is, and **Address on the local network** plays the part of Jellyfin's `PublishedServerUrl`.

API details: [Local network discovery for apps](jellyfin-api.md#local-network-discovery-for-apps).

<a id="the-jellyfin-app-for-android-s-integrated-player"></a><a id="the-jellyfin-app-for-androids-integrated-player"></a>

## The integrated player of the Jellyfin app for Android

The Jellyfin app for Android plays videos with its integrated player, or with the web player in its web view when its "Video player type" setting says "Web player". Its integrated player, from app version 2.4.0 on, plays a version as it is (direct play), from a local folder, a network share, an addon's link or a torrent, as it plays a file of a Jellyfin server: seeking, resuming, playback reports, Now playing and its bandwidth work as for other apps. A disc, an HLS or DASH version and Live TV channels play over Polyfin's HLS.

**Compared with Jellyfin**

- Jellyfin describes its own files as `Protocol` `File` ([BaseItem.cs, line 1231](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/BaseItem.cs#L1231); [MediaSourceManager.cs, lines 240 to 260](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L240-L260)), and Polyfin describes its versions so to that player alone.
- Other apps, the web player in the app's web view among them, get `Protocol` `Http` with Polyfin's signed `Path`: the web player needs that address for the grant, the relay and the volume boost.
- A Jellyfin `.strm` item with an http link is `Http` on Jellyfin, and that player fails it, as its developers note ([jellyfin-android #1443](https://github.com/jellyfin/jellyfin-android/issues/1443)).

API details: [Android integrated player for apps](jellyfin-api.md#android-integrated-player-for-apps).

## What apps get

Jellyfin apps get the rest of what they ask a movie, series and Live TV server for.

### Title pages

A title's page opens as soon as its description is ready, with the versions Polyfin already knows, even from lists that expired, while it asks the stream addons without a current list in the background. The web player adds their versions as each addon answers. Other apps show the versions known when the page opened, and get them all when the user presses Play or opens the page again. See [Title pages](versions.md#title-pages).

API details: [Version list progress for apps](jellyfin-api.md#version-list-progress-for-apps).

### Ratings

With the **MDBList API key for ratings**, in **Settings › Content**, `CommunityRating` carries a movie's or series' IMDb rating from MDBList (0 to 10) and `CriticRating` its critics' score or audience score (0 to 100), as **Critic rating shows** chooses; an administrator's edit still wins. Listings that fit on one page, addons' collections read whole among them, sort by `SortBy=CriticRating` as by `CommunityRating`, titles without one first. The Watchlist and smart lists keep their own order (see [Smart lists](smart-lists-and-search.md#smart-lists)). See [Ratings](collections-people-and-ratings.md#ratings).

API details: [Ratings for apps](jellyfin-api.md#ratings-for-apps).

### Volume between songs

With **Even the volume between songs** on, in **Settings › Content** (the default), songs (`Audio`) and albums (`MusicAlbum`) carry `NormalizationGain`, in dB relative to -18 LUFS, in listings and details, once it is known, as on Jellyfin 10.9 and later: the ReplayGain tags of local songs, else their measured loudness (see [Volume between songs](music.md#volume-between-songs)). The stream itself is not changed: apps apply the gain, the song's or the album's as their playback settings choose, and an app that ignores it plays unchanged. The web player takes a song's from the list it plays from, and its album's from the album's details. Music libraries' `LibraryOptions` report `EnableLUFSScan` as measuring is set for their songs. Off, no item carries a gain and nothing is measured.

### Libraries left out of the web player menus

In **Content › Libraries**, **Hide from the web player's top bar** lets an administrator leave any of the server's libraries out of the web player's menus, and each user any of their own: its top bar, the bar's **More** menu and its side menu. The library keeps its row on the home screen, and other apps keep listing it. Live TV catalogs have no such option: their channels go to Live TV.

API details: [Library menu visibility for apps](jellyfin-api.md#library-menu-visibility-for-apps).

### Libraries shown as movies and shows

Apps that list only movies and series, or open a folder as a series, see a library of other videos as movies and series with **Show as** **Movies and shows** in **Content › Libraries**: see [Other videos as movies and shows](addons-and-libraries.md#other-videos-as-movies-and-shows) and [Show as](addons-and-libraries.md#show-as).

**Compared with Jellyfin**

- Jellyfin never changes a library's type once it is made: it is a `<type>.collection` file written when the library is added ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3807-L3849)), and no endpoint or field of the library editor changes it. A type changes by removing the library and adding it again. Polyfin's choice goes further.
- A library made without a content type, the web player's mixed movies and shows, has no `CollectionType` ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3844-L3849)), as such a library.
- `/Shows/{id}/Episodes` answers 404 for an item that is not a series ([TvShowsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L261-L264)), as for a folder of home videos.

API details: [Libraries shown as movies and shows for apps](jellyfin-api.md#libraries-shown-as-movies-and-shows-for-apps).

### Continue Watching and hidden titles

Jellyfin apps have no button to take a title off Continue Watching without marking it played or unplayed, to take a series off Next Up while keeping what was watched, nor to hide a title from one's lists: Jellyfin 12.4 has none of these. The web player gets them in its ⋮ menus, and the admin app's **My account** lists them for the other apps (see [Watch state](watch-state.md#watch-state)). Apps see the result at their next refresh: the title is gone from `/UserItems/Resume`, a series taken off Next Up from `/Shows/NextUp` but when asked for alone (`seriesId`), and a hidden title from every listing; it still opens by its identifier. Apps that keep the live connection (WebSocket) open are told of a `UserDataChanged` for the item, as for any other change.

API details: [Continue Watching, Next Up and hidden titles for apps](jellyfin-api.md#continue-watching-next-up-and-hidden-titles-for-apps).

### Home screen sections

In the web player's **Settings › Home**, each user can untick, for a library, "Display in home screen sections such as 'Recently Added Media' and 'Continue Watching'" ([`homeScreenSettings.js`](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/homeScreenSettings/homeScreenSettings.js#L354-L368)). The web player then leaves the library's latest row off the home screen, and, as in Jellyfin 12.4, Next Up and Continue Watching leave out the titles only such libraries list, an episode going with its series ([`TVSeriesManager.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L71-L74), [`ItemsController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemsController.cs#L953-L961)). A title another library lists too, such as a show a local folder holds (see [Titles your catalogs list too](local-folders.md#titles-your-catalogs-list-too)), stays. The library itself still opens, and other users are not affected.

- Asked within a library or for one series, as a library's or a series' page asks, Next Up and Continue Watching list its titles all the same, as in Jellyfin.
- Jellyfin knows every library that holds an item. Polyfin knows, without asking an addon, the library a title was last listed in, the local folders that hold it, and the catalog pages it keeps: a title a catalog lists on a page no one opened since the pages were last kept counts as the other libraries' only. A title last listed in none of the user's libraries, such as one found by a search, stays.

API details: [Home screen sections for apps](jellyfin-api.md#home-screen-sections-for-apps).

### Profile pictures

A user changes their own profile picture from their app; an administrator can change anyone's.

- Polyfin accepts a JPEG, PNG or WebP picture of up to 5 MB and keeps it in the database.
- It is scaled down to 512 pixels and stored as JPEG, or PNG when it is transparent.
- It shows in users, sign-in screens and sessions, and on the admin app's **Users** page.

API details: [Profile pictures for apps](jellyfin-api.md#profile-pictures-for-apps).

### Search and sort in apps

Search, suggestions and some sorts answer from the addons' catalogs, so they differ a little from a Jellyfin server's:

- **Search:** results come in a fixed order. Movies and series the addons find come first, then episodes known by name, then people.
- **Late addons:** Polyfin answers within a few seconds without waiting for a late addon, so the same search may find more a moment later.
- **Suggestions:** before anything is typed, the web player suggests the user's favorite and liked titles, then titles from the top of each library.
- **At random:** a library listed at random draws among its first 100 titles, or up to 500 when an app asks for more. Each draw is kept for 2 minutes.
- **Series by date:** "Date Episode Added" orders series by their latest aired regular episode, and "Date Played" by the last episode the user played through.

API details: [Search hints](jellyfin-api.md#search-hints), [Search suggestions](jellyfin-api.md#search-suggestions), [Titles at random](jellyfin-api.md#titles-at-random), [Series by their latest episode](jellyfin-api.md#series-by-their-latest-episode).

### Episode orders

A series' "Display order", set in the web player's **Edit metadata**, shows its seasons and episodes in one of its TMDB episode groups, for every user (see [Episode orders](artwork-and-editing.md#episode-orders)). jellyfin-web offers the orders by type ([`metadataEditor.js`, lines 737 to 754](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/metadataEditor/metadataEditor.js#L737-L754), jellyfin-web 12.4); the admin app chooses among a series' groups by name, and where its specials show.

**Compared with Jellyfin**

- Jellyfin reads the first TMDB group of the type for each episode's metadata, the episodes being numbered in that order by their files ([`TmdbClientManager.cs`, lines 182 to 226 and 286 to 297](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs#L182-L297), Jellyfin 12.4). Polyfin's addons number episodes as aired, so Polyfin moves them to the group's seasons and numbers, as Jellyfin's mapping reads them: the group's part N is season N, its place N episode N+1. Episodes keep their identity: their identifiers, watch state, tracking and streams are those of the addon's numbers.
- Jellyfin refreshes the series' metadata when its order changes ([`ItemUpdateController.cs`, lines 122 to 133](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemUpdateController.cs#L122-L133)); Polyfin shows the new order at once, from TMDB's groups.
- Jellyfin has no place for the specials of an order, nor a choice among several groups of a type: both are Polyfin's own.

API details: [Episode orders for apps](jellyfin-api.md#episode-orders-for-apps).

### Movie recommendations

For each of the last movies the user played, recommendations give the "Because you watched" titles similar to it. They come from the similar titles of title pages, ordered half by the user's taste once their profile is confident (see [Top picks for you](home-rows.md#top-picks-for-you)), without those already played. With **Similar titles** off there are none.

**Compared with Jellyfin**

- Jellyfin's categories for liked titles and for the people of recent movies are left out: each would cost a search of the addons.

API details: [Movie recommendations for apps](jellyfin-api.md#movie-recommendations-for-apps).

### Remote images

The web player's **Edit images** searches TMDB, while the server has a **TMDB API key**, and the server's metadata addons for an item's images, and keeps the one an administrator downloads as an upload. See [Edit images](artwork-and-editing.md#edit-images).

API details: [Remote images for apps](jellyfin-api.md#remote-images-for-apps).

### Trailers, channels and theme media

Trailer items and plugin channels answer empty lists, as on a Jellyfin server without them. Addons give trailers only with their titles, as `RemoteTrailers`.

The extras, local trailers and theme media local folders keep beside their titles are served as Jellyfin serves them (see [Extras and theme media](naming-and-matching.md#extras-and-theme-media)): each is a `Video` item, or `Audio` for a theme song, with its `ExtraType` (`Trailer`, `BehindTheScenes`, `DeletedScene`, `Interview`, `Scene`, `Sample`, `Short`, `Featurette`, `Clip`, `Unknown`, `ThemeSong` or `ThemeVideo`) and its title, or season, as `ParentId`. No listing holds them; they play, with their own user data, as the title's local versions do.

API details: [Extras and theme media for apps](jellyfin-api.md#extras-and-theme-media-for-apps).

### Media addresses and downloads

Polyfin serves a title's streams, subtitles and fonts for the user its address names. That user's parental control, blocked genres, visible libraries and allowed hours apply.

- **Who plays:** the play session or grant Polyfin signed into the address names the user, else the app's access token.
- **Downloads:** a title's file download serves its first working version, under the same permissions.
- **Smaller copies:** apps that ask for a smaller copy to download, such as Streamyfin, get one while **Converted downloads** is on (see [Converted downloads](transcoding.md#converted-downloads)).

API details: [Who a media URL plays for](jellyfin-api.md#who-a-media-url-plays-for), [Opening streams and downloading files](jellyfin-api.md#opening-streams-and-downloading-files).

### What a streamed play sends

The session of a device playing a version Polyfin streams, over HLS or as one file, carries `TranscodingInfo`, as Jellyfin's does: whether the video and the audio are copied (`IsVideoDirect`, `IsAudioDirect`), the codecs, size, bitrate and audio channels sent, the container, the GPU converting the video, and `TranscodeReasons`. The web player's "Playback Info" reads it to say "Direct streaming" when only the audio is converted, rather than "Transcoding", with the target codecs and the reasons (see [Seeing how a play goes](transcoding.md#seeing-how-a-play-goes)). DLNA devices' sessions carry it too. A version played as it is has none.

**Compared with Jellyfin**

- Jellyfin clears it when its transcode ends; Polyfin keeps the last one for the play session until the play stops, as its encodings stop while the player is idle and start again at its next request.
- `Framerate` and `CompletionPercentage`, FFmpeg's progress, are left out, and `HardwareAccelerationType` only names the GPU of a video conversion, `none` for the processor.

API details: [Streamed play information for apps](jellyfin-api.md#streamed-play-information-for-apps).

### Fallback fonts

Fallback fonts come from `POLYFIN_FONTS_DIR`, its subfolders included; the Docker image ships DejaVu (Latin, Greek and Cyrillic), and more fonts can be mounted there for other scripts, such as CJK. See [Environment variables](configuration.md#environment-variables).

**Compared with Jellyfin**

- Jellyfin reads only the top of its folder.

API details: [Fallback fonts for apps](jellyfin-api.md#fallback-fonts-for-apps).

### Subtitle files

On a user's page under **Users**, the **Access** section's **Can manage subtitles** permission is on for administrators and off for other users. Administrators' apps can set it too. See [users](users.md) and [subtitles](subtitles.md).

- With it, a user can add an SRT, WebVTT, ASS or SSA file to a movie or an episode from their app. Every version then offers that file first among its subtitles.
- Only administrators delete such files, as in Jellyfin.
- Addon subtitles and tracks inside files cannot be deleted.
- The subtitle search stays open to every user, as it lists only what every version already offers. Apps show it only with the permission.

**Compared with Jellyfin**

- The permission is Jellyfin's `EnableSubtitleManagement`.
- Jellyfin's subtitle search is not open to every user.

API details: [Subtitle files for apps](jellyfin-api.md#subtitle-files-for-apps).

### Picture subtitles drawn by the app

When the web player is set to draw PGS subtitles itself, a PGS track inside a Matroska file reaches it as a SUP file, as Jellyfin 12.4 serves its raw file. Polyfin reads the track whole through the file's index, as it does for text tracks, and keeps it for later playbacks. The app reads the file by ranges as it plays.

- A track the index does not list block by block, or larger than 32 MB, is not offered as a file.
- DVD (VobSub) tracks are not offered as files. With **Turn picture subtitles into text** on, their text copies are, once read, as are those of PGS tracks (see below).
- A PGS file (`.sup`) beside a local file reaches the app as it is, by ranges. A VobSub pair beside a local file (`.idx` and `.sub`) is burned in, as Jellyfin 12.4 does: it gives VobSub as a file only extracted as Matroska.

API details: [PGS subtitle files for apps](jellyfin-api.md#pgs-subtitle-files-for-apps).

### Text copies of picture subtitles

With **Turn picture subtitles into text** on, in **Settings › Conversion (transcoding)**, apps get a text copy of each PGS or VobSub track once it is read: see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text).

API details: [Text copies of picture subtitles for apps](jellyfin-api.md#text-copies-of-picture-subtitles-for-apps).

### Rewatching, downmix and ratings

- **Rewatching:** starting a played movie, episode or audiobook again keeps its last played date. The date changes once the position passes the start, or at the end. Next Up therefore does not move past an episode only just started.
- **Downmix:** music and audiobooks converted to stereo go through the downmix chosen in the server settings, as converted video does.
- **Ratings that are not numbers:** an addon's IMDb rating of `NaN` or `Infinity` leaves the title without a rating.

API details: [Jellyfin 12.2 details](jellyfin-api.md#jellyfin-12-2-details), [Jellyfin 12.4 details](jellyfin-api.md#jellyfin-12-4-details).

### Refresh metadata

**Refresh metadata** (administrators only) makes Polyfin forget the title's version and subtitle lists, expired ones included, the rating it looked up and its ratings from MDBList, and stop asking addons again for those lists. It asks the addons for the description again at once, and replaces the one it kept only when an addon answers. While the addon that described the title fails, such as when it answers `504`, the title keeps its description, cast and genres, and the log says so (`A title could not be refreshed: it keeps the description the addon gave before`). An addon still asked for one of those lists when the title is refreshed is asked again once it answers: its answer, asked for before the refresh, is not kept.

On a movie or series of a local folder, **Refresh metadata** first matches its files again, as a scan does (see [Identify in the web player](unmatched-files.md#identify-in-the-web-player)).

On a server movie or series library, **Refresh metadata** queues the paced refresh of its titles' descriptions, as **Content › Libraries** does (see [Library description refresh](addons-and-libraries.md#library-description-refresh)), and answers at once.

API details: [Refresh metadata for apps](jellyfin-api.md#refresh-metadata-for-apps).

### Administration from Jellyfin apps

API keys, devices, user management, server configuration, the activity log, logs and scheduled tasks are covered in [administration](metrics-and-api-keys.md#api-keys-and-integrations).

### Client addresses behind a reverse proxy

**Known proxies**, in **Settings › Security**, lists the reverse proxies whose forwarded headers name the client of a request, like Jellyfin's own Known proxies setting. See [Known proxies](reverse-proxy.md#known-proxies).

API details: [Client addresses for apps](jellyfin-api.md#client-addresses-for-apps).

### DLNA devices

TVs and players that play over DLNA, without a Jellyfin app, browse and play the libraries of the user an administrator chose for each (see [DLNA](dlna.md)). Their playbacks show in the web player's dashboard and in `/Sessions` like an app's, with the app named `DLNA` and the device's name; Polyfin cannot control them.

API details: [DLNA sessions for apps](jellyfin-api.md#dlna-sessions-for-apps).

### Live TV tuners

The server's HDHomeRun tuners (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)) are Jellyfin's tuner hosts of type `hdhomerun`. Their channels are listed with the others in Live TV. The web player's tuner pages open the tuner's page in the admin app, or **Add a source** on its **HDHomeRun tuner** tab for a new tuner (see [Dashboard links and single sign-on](web-client.md#dashboard-links-and-single-sign-on)).

API details: [Live TV tuners for apps](jellyfin-api.md#live-tv-tuners-for-apps).

### Backups

The backups page of the web player's dashboard (Jellyfin 10.11 and later) shows the database backups of the backups folder (see [Backups](backups.md)): it lists them, makes one, and restores one. Polyfin's backups hold its database alone, whatever the options asked: Jellyfin's also hold its metadata, trickplay images and subtitles, which Polyfin does not keep on disk.

API details: [Backups for apps](jellyfin-api.md#backups-for-apps).

## Jellyfin features Polyfin does without

These dashboard features answer as a Jellyfin 12.4 on which nothing of the kind is configured:

- plugins, packages and plugin repositories;
- browsing the server's folders;
- library folders and metadata providers;
- Live TV listing providers;
- the startup wizard.

Polyfin never shows the host's folders. To change the server's settings, use the [admin app](administration.md#settings) or the server configuration API.

API details: [Plugins, packages and the startup wizard for apps](jellyfin-api.md#plugins-packages-and-the-startup-wizard-for-apps).

<div class="moved-sections">

<a id="series-by-their-latest-episode"></a>*Series by their latest episode* moved to [Jellyfin API notes](jellyfin-api.md#series-by-their-latest-episode).

<a id="titles-at-random"></a>*Titles at random* moved to [Jellyfin API notes](jellyfin-api.md#titles-at-random).

<a id="who-a-media-url-plays-for"></a>*Who a media URL plays for* moved to [Jellyfin API notes](jellyfin-api.md#who-a-media-url-plays-for).

</div>
