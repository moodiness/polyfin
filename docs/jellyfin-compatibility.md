# Jellyfin compatibility

Polyfin targets the Jellyfin 12.4 API, so standard Jellyfin apps sign in, browse, search and play. This page lists the compatible apps, what they get from Polyfin, and the Jellyfin features Polyfin does without.

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

**For app developers:** `/Polyfin/Items/{id}/Versions` tells how many addons are still asked, for the first time or again, how many media sources the title's details list now, how many versions are known, and whether the web player adds versions to a page already open (`AddVersions`), and Polyfin pushes the same as a `PolyfinVersions` message as it changes, on the live connection (WebSocket) of jellyfin-web only, the app signed in as `Jellyfin Web`: other apps get Jellyfin's own message types alone, as the Jellyfin SDKs some apps are built on fail on a type they do not know. `POST /Polyfin/Items/{id}/Versions/Search` has the title's addons asked again, once every 20 seconds at most for a user's title (see [Title pages](playback.md#title-pages)).

### Ratings

With the server's **MDBList API key for ratings**, `CommunityRating` carries a movie's or series' IMDb rating from MDBList (0 to 10) and `CriticRating` its critics' score or audience score (0 to 100), as **Critic rating shows** chooses; an administrator's edit still wins. Listings that fit on one page, addons' collections read whole among them, sort by `SortBy=CriticRating` as by `CommunityRating`, titles without one first. The Watchlist and smart lists keep their own order (see [Smart lists](addons-and-libraries.md#smart-lists)). See [Ratings](addons-and-libraries.md#ratings).

**For app developers:** `/Polyfin/Items/{id}/Ratings` answers a title's `CommunityRating`, `CriticsScore` and `AudienceScore` (null when it has none, all null without a key) and `Pending` while MDBList is being asked about it.

### Volume between songs

With **Even the volume between songs** on, the default, songs (`Audio`) and albums (`MusicAlbum`) carry `NormalizationGain`, in dB relative to -18 LUFS, in listings and details, once it is known, as on Jellyfin 10.9 and later: the ReplayGain tags of local songs, else their measured loudness (see [Volume between songs](addons-and-libraries.md#volume-between-songs)). The stream itself is not changed: apps apply the gain, the song's or the album's as their playback settings choose, and an app that ignores it plays as before. The web player takes a song's from the list it plays from, and its album's from the album's details. Music libraries' `LibraryOptions` report `EnableLUFSScan` as measuring is set for their songs. Off, no item carries a gain and nothing is measured.

### Libraries left out of the web player menus

On the admin app's **Libraries** page, an administrator can leave any of the server's libraries out of the web player's menus, and each user any of their own: its top bar, the bar's **More** menu and its side menu. The library keeps its row on the home screen, and other apps keep listing it. Live TV catalogs have no such option: their channels go to Live TV.

**For app developers:** `GET /Polyfin/UserViews` lists the views `/UserViews` gives the caller, in the same order, as `{"Items": [{"Id": <the view's Id>, "HideInMenus": <true when the web player's menus leave it out>}]}`. Polyfin's own Live TV, Playlists, Collections and Watchlist views are never left out.

### Other videos as movies and shows

An addon's catalog of other videos, such as a store's downloads, is a home videos library of folders and videos, which jellyfin-web and most apps browse and play. Some apps keep only libraries of movies, shows, collections and mixed ones, and list only `Movie` and `Series` items; others open a folder as a show and ask for its episodes, which a folder does not answer, in Polyfin as in Jellyfin. For them, **Show as movies and shows** under **Content › Libraries** shows such a library's titles as movies and shows: a title with two videos or more, or one numbered as an episode, is a show, whose episodes are its videos, a title without videos, or with one file, a movie, in a library without a content type (see [Shown as movies and shows](addons-and-libraries.md#shown-as-movies-and-shows)).

**Compared with Jellyfin:** Jellyfin has no such option. A library made without a content type, jellyfin-web's mixed movies and shows, has no `CollectionType` ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3844-L3849)), as such a library; and `/Shows/{id}/Episodes` answers 404 for an item that is not a series ([TvShowsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L261-L264)), as for a folder of home videos.

**For app developers:** the library is a `CollectionFolder` without `CollectionType`; `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Movie,Series` lists its titles as `Series` and `Movie` items, `IncludeItemTypes=Movie` or `Series` alone lists them with a `TotalRecordCount` of that kind alone, as for an `anime` library (see [What catalogs list](addons-and-libraries.md#what-catalogs-list)), and `/Shows/{id}/Seasons`, `/Shows/{id}/Episodes`, `/Shows/NextUp`, `/UserItems/Resume`, `PlaybackInfo` and the streams answer them as other shows, episodes and movies. Their `Id`s are not those of the folders and videos they were: switching the option gives users' data to the new items, and the former ones answer 404.

### Continue Watching and hidden titles

Jellyfin apps have no button to take a title off Continue Watching without marking it played or unplayed, to take a series off Next Up while keeping what was watched, nor to hide a title from one's lists: Jellyfin 12.4 has none of these. The web player gets them in its ⋮ menus, and the admin app's **My account** lists them for the other apps (see [Watch state](users.md#watch-state)). Apps see the result at their next refresh: the title is gone from `/UserItems/Resume`, a series taken off Next Up from `/Shows/NextUp` but when asked for alone (`seriesId`), and a hidden title from every listing; it still opens by its identifier. Apps that keep the live connection (WebSocket) open are told of a `UserDataChanged` for the item, as for any other change.

**For app developers:** `DELETE /Polyfin/UserItems/{id}/Resume` takes the item off Continue Watching, a series or a season its episodes, a folder of videos its videos, and answers its `UserItemData`. `POST /Polyfin/UserItems/{id}/Hidden` hides a movie, a home video, a series or a folder of videos (see [Folders of videos](addons-and-libraries.md#folders-of-videos)), a season or an episode standing for its series and a video for its folder, and `DELETE` shows it again; both answer 204. `GET /Polyfin/UserItems/Hidden` lists the hidden titles, the latest hidden first, as a listing describes them. `DELETE /Polyfin/UserItems/{id}/NextUp` takes a series off the user's Next Up, a season or an episode standing for its series, and `POST` puts it back; both answer 204, and 400 for an item that is neither a series nor in one. The series stays off until one of its episodes is played after the removal or one is released after it (see [Continue Watching and Next Up](users.md#continue-watching-and-next-up)).

### Home screen sections

In the web player's **Settings › Home**, each user can untick, for a library, "Display in home screen sections such as 'Recently Added Media' and 'Continue Watching'" ([`homeScreenSettings.js`](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/homeScreenSettings/homeScreenSettings.js#L354-L368)). The web player then leaves the library's latest row off the home screen, and, as in Jellyfin 12.4, Next Up and Continue Watching leave out the titles only such libraries list, an episode going with its series ([`TVSeriesManager.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L71-L74), [`ItemsController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemsController.cs#L953-L961)). A title another library lists too, such as a show a local folder holds (see [Titles your catalogs list too](local-folders.md#titles-your-catalogs-list-too)), stays. The library itself still opens, and other users are not affected.

- Asked within a library or for one series, as a library's or a series' page asks, Next Up and Continue Watching list its titles all the same, as in Jellyfin.
- Jellyfin knows every library that holds an item. Polyfin knows, without asking an addon, the library a title was last listed in, the local folders that hold it, and the catalog pages it keeps: a title a catalog lists on a page no one opened since the pages were last kept counts as the other libraries' only. A title last listed in none of the user's libraries, such as one found by a search, stays.

**For app developers:** the choice is the user's `Configuration.LatestItemsExcludes`, a list of library identifiers. `/Shows/NextUp` and `/UserItems/Resume` apply it when they have no `parentId`, and `/Shows/NextUp` when it has no `seriesId` either. `/Items/Latest` always needs a `parentId` in Polyfin: without one, it answers an empty list.

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
- Jellyfin searches its own library and answers at once. Polyfin asks the addons' search catalogs and answers with what they found within a few seconds, without waiting for an addon that is late (see [Catalog pages](addons-and-libraries.md#catalog-pages)): the same search may find more a moment later.

**For app developers:** `/Search/Hints` answers from the same search as `/Items`, without its channels and programmes.

### Search suggestions

Before anything is typed, jellyfin-web's search page shows Suggestions: the user's favorite and liked titles first, then movies and series from the top of each of the user's libraries, in their catalog's order: the most popular first in a popular catalog. The libraries are mixed, one title of each in turn, and the picks change each time the page opens, each library giving one of its next two titles at random. They stay within the user's libraries, parental control and blocked genres, without the titles they hid. Above them, the web client lists the user's recent searches (see [Recent searches](web-client.md#recent-searches)).

**Compared with Jellyfin:**

- Jellyfin picks suggestions at random among everything; Polyfin picks among the first page of each library, from its top, as a remote catalog puts its titles forward.

**For app developers:** `/Items` without `ParentId` answers such suggestions when it asks `Recursive=true` for `Movie`, `Series` or `MusicArtist` only, sorted by `Random`, alone or with `IsFavoriteOrLiked`, which puts favorite and liked titles first. `/Items/Suggestions` still picks at random among the first page of each library, as Jellyfin picks at random, the titles the user's taste profile ranks first coming first once it is confident (see [Top picks for you](web-client.md#top-picks-for-you)).

### Titles at random

An app that lists a library's titles at random, as the home carousel of the [LumaaGlaass](web-client.md#lumaaglaass) theme picks its slides, and as jellyfin-web's shuffle of a library plays, gets them drawn among the library's first 100 titles, in their catalog's order, rather than its first catalog page alone: the carousel's picks reach well past a library's first titles, and change from one page load to the next.

- The titles drawn among are those the user may see: within their parental control and blocked genres, without the titles they hid, a title hidden after the draw leaving it at once. A collection library gives the titles of its collections, as its other listings of titles do.
- A listing that asks for more than 100, from its first title, draws among as many, up to 500, the most a listing that asks for no limit gets. A library's **Maximum titles** bounds its draws too (see [Genre and maximum](addons-and-libraries.md#genre-and-maximum)).
- A draw is kept for 2 minutes, so that the pages an app asks for one after the other follow one order and list each title once. A listing after that draws anew.
- The listing's total is the number of titles drawn among: pages past it are empty.
- A library's first 100 titles are read as a listing of 100 reads them, a few catalog pages, and kept with the other pages read (see [Catalog pages](addons-and-libraries.md#catalog-pages)). While they are being read, a listing waits for them at most 3 seconds, then draws among those read so far, without keeping that draw, and the rest is read in the background for the next one.
- Smart lists keep their own sort (see [Smart lists](addons-and-libraries.md#smart-lists)).

**Compared with Jellyfin:**

- Jellyfin sorts the whole library at random, anew for each request ([`OrderMapper.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L55), applied by [`BaseItemRepository.QueryBuilding.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/BaseItemRepository.QueryBuilding.cs#L306-L341), Jellyfin 12.4), so the pages of one listing may repeat or skip titles. Polyfin keeps each draw 2 minutes so that its pages do not.
- Polyfin stops at a library's first 100 titles: a remote catalog is read a page at a time from its addon, and reaching its end, which may be thousands of titles away, would cost as many requests to the addon for every user's home page. 100 titles are a few pages, which listings read anyway.

**For app developers:** a listing of a library with `ParentId`, `Recursive=true` and only title types, such as `IncludeItemTypes=Movie,Series`, whose first `SortBy` is `Random`, is answered from the draw; the keys after `Random` change nothing. `TotalRecordCount` is the number of titles drawn among.

### Series by their latest episode

jellyfin-web's TV shows tab sorts a series library by "Date Episode Added" and "Date Played" ([`tvshows.js`, lines 249 to 268](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/apps/legacy/controllers/shows/tvshows.js#L249-L268), jellyfin-web 12.4). Polyfin answers both:

- "Date Episode Added" orders the series by their latest regular episode that has aired, as their descriptions date it, the latest first. Specials (season 0) and episodes to come do not count. A series without a dated episode comes last, the first in ascending order.
- "Date Played" orders them by the last time the user played an episode of theirs through, the latest first; series never played come last.
- As for a listing at random (see [Titles at random](#titles-at-random)), the series are sorted among the library's first 100, or as many as the listing asks for from its first title, up to 500, within the user's parental control and blocked genres, without the titles they hid. The order is kept 2 minutes, so that the pages of one listing follow it, and the listing's total is the number of series sorted.
- A series whose description Polyfin does not keep yet has it read in the background, at most 50 at each listing: it comes as a series without episodes meanwhile, and the order is not kept until every description is read.
- Smart lists keep their own sort (see [Smart lists](addons-and-libraries.md#smart-lists)), which may also be by the latest episode or the latest season premiere.

**Compared with Jellyfin:**

- Jellyfin orders series by `DateLastMediaAdded`, when their latest episode was added to the library ([`OrderMapper.cs`, line 61](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L61), Jellyfin 12.4). An addon's episodes are never added: they air. Polyfin takes the day the latest regular episode aired in its place.
- `SeriesDatePlayed` is, as in Jellyfin, the latest date the user played one of the series' episodes through ([`OrderMapper.cs`, lines 78 to 87](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L78-L87)); `DatePlayed` the date the user last played the series itself ([lines 32 to 48](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L32-L48)).
- Jellyfin sorts the whole library; Polyfin its first titles, as for titles at random, since a remote catalog is read a page at a time.

**For app developers:** a listing of a library with `ParentId`, `Recursive=true` and `IncludeItemTypes=Series` alone, whose first `SortBy` is `DateLastContentAdded`, `SeriesDatePlayed` or `DatePlayed`, is answered so. `SortOrder` applies to every key, as in Jellyfin, and the keys after the first, then the name, order the series of one date.

### Episode orders

A series' "Display order", set in jellyfin-web's **Edit metadata**, shows its seasons and episodes in one of its TMDB episode groups, for every user (see [Episode orders](addons-and-libraries.md#episode-orders)). jellyfin-web offers the orders by type ([`metadataEditor.js`, lines 737 to 754](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/metadataEditor/metadataEditor.js#L737-L754), jellyfin-web 12.4); the admin app chooses among a series' groups by name, and where its specials show.

**Compared with Jellyfin:**

- Jellyfin reads the first TMDB group of the type for each episode's metadata, the episodes being numbered in that order by their files ([`TmdbClientManager.cs`, lines 182 to 226 and 286 to 297](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs#L182-L297), Jellyfin 12.4). Polyfin's addons number episodes as aired, so Polyfin moves them to the group's seasons and numbers, as Jellyfin's mapping reads them: the group's part N is season N, its place N episode N+1. Episodes keep their identity: their identifiers, watch state, tracking and streams are those of the addon's numbers.
- Jellyfin refreshes the series' metadata when its order changes ([`ItemUpdateController.cs`, lines 122 to 133](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemUpdateController.cs#L122-L133)); Polyfin shows the new order at once, from TMDB's groups.
- Jellyfin has no place for the specials of an order, nor a choice among several groups of a type: both are Polyfin's own.

**For app developers:**

- `DisplayOrder` is kept from `POST /Items/{id}` on a series, as Jellyfin keeps it ([`ItemUpdateController.cs`, lines 412 to 415](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemUpdateController.cs#L412-L415)), and given back in the series' `DisplayOrder` ([`DtoService.cs`, lines 1207 to 1210](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Dto/DtoService.cs#L1207-L1210)). Empty or absent is aired.
- In an order, `/Shows/{id}/Seasons` lists the group's seasons, with identifiers of their own, which `/Items/{id}`, `/Items?ParentId=` and `/Shows/{id}/Episodes?seasonId=` take; `seasonId` also takes a season of the addon's. An episode's `ParentIndexNumber`, `IndexNumber`, `SeasonId`, `SeasonName` and `ParentId` are those of the order; its `Id` is the same in every order.
- `/Shows/{id}/Episodes?season=` filters by the order's season number.

### Movie recommendations

For each of the last movies the user played, recommendations give the "Because you watched" titles similar to it. They come from the similar titles of title pages, ordered half by the user's taste once their profile is confident (see [Top picks for you](web-client.md#top-picks-for-you)), without those already played. With **Similar titles** off there are none.

**Compared with Jellyfin:**

- Jellyfin's categories for liked titles and for the people of recent movies are left out: each would cost a search of the addons.

**For app developers:** `/Movies/Recommendations`.

### Years, groupings, artwork and viewing

**For app developers:**

- `/Years` lists the years the year pages offer (catalogs whose genre filter offers years).
- `/UserViews/GroupingOptions` lists the movie and series libraries.
- `/Items/{id}/Images` lists a title's artwork.
- `/Sessions/Viewing` records the item an app shows, which **Sessions** gives as `NowViewingItem`.

### Remote images

jellyfin-web's **Edit images** searches TMDB, while the server has a **TMDB API key**, and the server's metadata addons for an item's images, and keeps the one an administrator downloads as an upload. See [Edit images](addons-and-libraries.md#edit-images).

**For app developers:**

- `/Items/{id}/RemoteImages/Providers` lists `TMDB` and `Addons` with their `SupportedImages`: both for movies and series, `TMDB` alone (Primary) for seasons, episodes and people, none for other items or without a key.
- `/Items/{id}/RemoteImages` answers a `RemoteImageResult` for `type` (every type when left out), `providerName`, `includeAllLanguages`, `startIndex` and `limit`. An addon's image has its addon's name as `ProviderName`. `Url` and `ThumbnailUrl` are Polyfin addresses, signed for the item, which apps load without credentials.
- `POST /Items/{id}/RemoteImages/Download?type=&imageUrl=` is for administrators (403 otherwise). `imageUrl` must be a `Url` Polyfin listed for the item and type (400 otherwise); `providerName` is ignored.

### Trailers, channels and theme media

Trailer items and plugin channels answer empty lists, as on a Jellyfin server without them. Addons give trailers only with their titles, as `RemoteTrailers`.

The extras, local trailers and theme media local folders keep beside their titles are served as Jellyfin serves them (see [Extras and theme media](local-folders.md#extras-and-theme-media)): each is a `Video` item, or `Audio` for a theme song, with its `ExtraType` (`Trailer`, `BehindTheScenes`, `DeletedScene`, `Interview`, `Scene`, `Sample`, `Short`, `Featurette`, `Clip`, `Unknown`, `ThemeSong` or `ThemeVideo`) and its title, or season, as `ParentId`. No listing holds them; they play, with their own user data, as the title's local versions do.

**For app developers:**

- `/Items/{id}/LocalTrailers` lists a movie's, series' or season's local trailers, and `/Items/{id}/SpecialFeatures` its other extras, theme media left out, both as arrays of items; the `/Users/{userId}/Items/{id}/…` forms jellyfin-web calls answer the same.
- `/Items/{id}/ThemeSongs` and `/Items/{id}/ThemeVideos` answer a `ThemeMediaResult`, and `/Items/{id}/ThemeMedia` both, with no soundtrack songs. With `inheritFromParent=true`, a season without any gets its series', and an episode its season's, else its series'; `OwnerId` is the item they were found on. `sortBy=Random` shuffles them.
- An item's details give `LocalTrailerCount` and `SpecialFeatureCount`; `RemoteTrailers` stay the addons' trailers.

### Opening streams and downloading files

Sources need no opening. A title's file download serves its first working version, as the download route does, under the same permissions. Apps that download through PlaybackInfo with a device profile asking for video converted progressively (`Protocol` `http`), as Streamyfin does, get a `TranscodingUrl` at `/videos/{id}/stream.mp4` when **Converted downloads** is on. See [Converted downloads](transcoding.md#converted-downloads).

**Compared with Jellyfin:**

- Jellyfin does not ask for these permissions for its own files.

**For app developers:**

- `/LiveStreams/Open` answers 400, as Jellyfin does without an open token; `/LiveStreams/Close` answers 204.
- `/Items/{id}/File` serves the first working version.
- PlaybackInfo, `/Videos/{id}/stream` and `/Items/{id}/Download` answer 400 `Error processing request.` for an item that plays nothing itself, such as a folder of videos, a series, a season or a collection, as Jellyfin does for an item without media sources ([MediaSourceManager.cs, lines 383 to 393](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L383-L393), answered 400 by [ExceptionMiddleware.cs, line 127](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Middleware/ExceptionMiddleware.cs#L127)). No addon is asked for streams of it. An item that does not exist, or that the user may not see, answers 404.
- `/Videos/{id}/stream.mp4` and `/Videos/{id}/stream.ts` convert only for the `TranscodingUrl` of a converted download: with `static=true`, or without a play session and a `videoCodec`, or with **Converted downloads** off, they serve the version as it is, as before.

### What a streamed play sends

The session of a device playing a version Polyfin streams, over HLS or as one file, carries `TranscodingInfo`, as Jellyfin's does: whether the video and the audio are copied (`IsVideoDirect`, `IsAudioDirect`), the codecs, size, bitrate and audio channels sent, the container, the GPU converting the video, and `TranscodeReasons`. jellyfin-web's "Playback Info" reads it to say "Direct streaming" when only the audio is converted, rather than "Transcoding", with the target codecs and the reasons (see [Seeing how a play goes](transcoding.md#seeing-how-a-play-goes)). DLNA devices' sessions carry it too. A version played as it is has none.

**Compared with Jellyfin:**

- Jellyfin clears it when its transcode ends; Polyfin keeps the last one for the play session until the play stops, as its encodings stop while the player is idle and start again at its next request.
- `Framerate` and `CompletionPercentage`, FFmpeg's progress, are left out, and `HardwareAccelerationType` only names the GPU of a video conversion, `none` for the processor.

**For app developers:** `GET /Sessions` fills `TranscodingInfo` once the play session's first encoding opened its source, from the play session the app reported in `/Sessions/Playing`; nothing for a play reported as `DirectPlay`.

### Who a media URL plays for

Polyfin serves a title's streams, subtitles and attached fonts for the user a media URL names: by the play session or grant Polyfin signed into it, else by the caller's access token. That user's parental control, blocked genres, visible libraries and allowed hours apply, and addon subtitles come synced to the version they play (see [Addon subtitles synced to the version](subtitles.md#addon-subtitles-synced-to-the-version)).

The token is read as on any request: from the `Authorization` header or the `ApiKey` query parameter, and with legacy authorization from the legacy headers and `api_key`. On the routes Jellyfin serves to anyone, `api_key` counts even without legacy authorization, as some apps sign the addresses they build themselves with it: the streams of videos and audio tracks, subtitles, attachments and recordings under way.

**Compared with Jellyfin:**

- Jellyfin 12.4 reads `api_key` only with legacy authorization ([`AuthorizationContext.cs`, lines 108 to 110](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Security/AuthorizationContext.cs#L108-L110)), but serves those routes without checking who asks, for example the subtitle streams ([`SubtitleController.cs`, lines 208 and 294](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/SubtitleController.cs#L208)). Polyfin needs to know who plays, and refuses them without a user.
- The routes Jellyfin serves to signed-in callers only keep its rule, and refuse `api_key` without legacy authorization: `/Items/{id}/Download` and `/Items/{id}/File`, `/Audio/{id}/universal`, and the HLS playlists and segments of audio tracks.

**For app developers:** prefer the `ApiKey` query parameter, which Polyfin's own media URLs use. A token in an address never reaches the log: request paths are logged without their query, and the values of `api_key`, `ApiKey` and other tokens are hidden in log lines.

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

When jellyfin-web is set to draw PGS subtitles itself, a PGS track inside a Matroska file reaches it as a SUP file, as Jellyfin 12.4 serves its raw file. Polyfin reads the track whole through the file's index, as it does for text tracks, and keeps it for later playbacks. The app reads the file by ranges as it plays.

- A track the index does not list block by block, or larger than 32 MB, is not offered as a file.
- DVD (VobSub) tracks are not offered as files. With **Turn picture subtitles into text** on, their text copies are, once read, as are those of PGS tracks (see below).
- A PGS file (`.sup`) beside a local file reaches the app as it is, by ranges. A VobSub pair beside a local file (`.idx` and `.sub`) is burned in, as Jellyfin 12.4 does: it gives VobSub as a file only extracted as Matroska.

**For app developers:** `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/0/Stream.pgssub` (or `.sup`) answers `application/octet-stream` and supports `Range`.

### Text copies of picture subtitles

With **Turn picture subtitles into text** on, a PGS or VobSub track read into text is one more subtitle `MediaStream`, after the version's tracks so that no index moves (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)):

- `Codec` is `subrip`; `IsTextSubtitleStream`, `SupportsExternalStream` and `IsExternal` are true; `Language`, `IsForced` and `IsHearingImpaired` are the original's.
- `Title` is the original's title or language followed by "(OCR)", and `DisplayTitle` reads like `English (OCR) - SUBRIP - External`.
- It is served as other text subtitles are: `DeliveryMethod` `External` in the format asked (`.vtt`, `.srt`, `.ass` or `.js`), or `Hls`.
- When `PlaybackInfo` names no subtitle track, or names the version's `DefaultSubtitleStreamIndex` as the item gives it (jellyfin-web always sends it back), the default choice ranks the original, and gives the copy's index for a profile that would have the original burned in (`Encode`). Another track the app asks for is delivered as asked, the original still burned in when asked.
- A copy is listed from the first `PlaybackInfo` after it is read; no index of an earlier answer changes.

### Jellyfin 12.2 details

- **Codec tags:** each track's `CodecTag` is the tag ffprobe reads, such as `avc1` or `mp4a` in MP4. Matroska's empty tags are left out, and live channels report none. Attached files report theirs as they are, `[0][0][0][0]` in Matroska. Direct play checks a profile's `VideoCodecTag` conditions against the file's tag.
- **Rewatching:** starting a played movie, episode or audiobook again does not change its last played date. The date changes once the position passes the start, or when the title is played to the end. Next Up therefore does not move past an episode that was only just started. `/Shows/NextUp` with `enableRewatching=true` also lists, for a series played again, the played episode after the one played last, as Jellyfin 12.4 does (see [Continue Watching and Next Up](users.md#continue-watching-and-next-up)).
- **Letter filters:** `nameStartsWith`, `nameLessThan` and `nameStartsWithOrGreater` ignore letter case. They narrow people, music, favorites and other personal lists, and playlists. Movie and series libraries come from addons' catalogs, so they are not narrowed.
- **People counts:** `/Persons` counts only the people it can list.
- **Downmix:** music and audiobooks converted to stereo go through the downmix chosen in the server settings, as the audio of converted video does.

### Jellyfin 12.4 details

- **Ratings that are not numbers:** a `CommunityRating` or `CriticRating` that is not a finite number is none, as Jellyfin 12.4 keeps none. An addon's IMDb rating of `NaN` or `Infinity` leaves the title without a rating. Sent to the metadata editor (`POST /Items/{id}`), `"NaN"`, `"Infinity"`, `"-Infinity"` or a number too large for a float is taken as no value: the answer is 204, and the field follows the addon again. Gains (`NormalizationGain`) are always finite numbers.
- **Videos without a frame size:** a video track ffprobe gives no width or height, as a live stream probed early may be, takes its `AspectRatio` from ffprobe's display aspect ratio alone, and has none without one, as in Jellyfin 12.4, whose probe of such a track failed before. Jellyfin reports an unknown display aspect ratio as `0:1`; Polyfin leaves it out, with or without a frame size.

### Refresh metadata

**Refresh metadata** (administrators only) makes Polyfin forget the title's version and subtitle lists, expired ones included, the rating it looked up and its ratings from MDBList, and stop asking addons again for those lists. It asks the addons for the description again at once, and replaces the one it kept only when an addon answers. While the addon that described the title fails, such as when it answers `504`, the title keeps its description, cast and genres, and the log says so (`A title could not be refreshed: it keeps the description the addon gave before`). An addon still asked for one of those lists when the title is refreshed is asked again once it answers: its answer, asked for before the refresh, is not kept.

On a movie or series of a local folder, **Refresh metadata** first matches its files again, as a scan does (see [Identify in the web player](local-folders.md#identify-in-the-web-player)).

On a server movie or series library, **Refresh metadata** queues the paced refresh of its titles' descriptions, as **Content › Libraries** does (see [Refreshing a library's descriptions](addons-and-libraries.md#refreshing-a-library-s-descriptions)), and answers at once.

**For app developers:** `POST /Items/{id}/Refresh` answers `204` at once, as Jellyfin's does once it queued the refresh ([ItemRefreshController.cs, lines 58 to 92](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Jellyfin.Api/Controllers/ItemRefreshController.cs#L58-L92); [ProviderManager.cs, lines 1136 to 1162](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Providers/Manager/ProviderManager.cs#L1136-L1162)), and `404` for an item the user cannot reach. On a library, `metadataRefreshMode` or `imageRefreshMode` set to `Default` or `FullRefresh` starts the refresh; the replace options are ignored.

### Administration from Jellyfin apps

API keys, devices, user management, server configuration, the activity log, logs and scheduled tasks are covered in [administration](administration.md#api-keys-and-integrations).

### Signing in to apps with Quick Connect

Jellyfin apps ask for a name and a password, never for a code. An account with [two-step sign-in](users.md#two-step-sign-in), and an administrator while **Require two-step sign-in for administrators** is on, signs in to them with Quick Connect:

1. In the app, choose Quick Connect on the sign-in screen: it shows a 6-digit code.
2. Sign in to the admin app, with the password and the code of your authenticator app, and enter the app's code on the **Quick Connect** page: in the top bar for members, in the account menu for administrators.
3. The app signs in as you.

Quick Connect must stay on, under **Settings › General**. Typing the password in the app is refused as a wrong password is, and counts as one toward the client's failed attempts, so that the answer never tells whether the password was right. The apps signed in before two-step sign-in was turned on stay signed in; sign them out under **My account › Devices** if needed. API keys made by administrators work as before.

A code is approved once, as in Jellyfin, which refuses a request already authorized: approved, it waits no more on the **Quick Connect** page, and nobody else can take it. The page also takes the 8-digit codes of Silo's TV apps (see [Signing a TV in with a code](silo-apps.md#signing-a-tv-in-with-a-code)).

**Compared with Jellyfin:**

- Jellyfin has no second step of its own. Polyfin keeps Jellyfin's sign-in as it is for the apps, and asks for the code in the admin app only.

**For app developers:** `/Users/AuthenticateByName` answers 401 for such an account, as for a wrong password; `/Users/AuthenticateWithQuickConnect` signs it in once a request is approved.

### Client addresses behind a reverse proxy

Like Jellyfin's own Known proxies setting, **Known proxies**, under **Settings › Security**, lists the reverse proxies whose forwarded headers name the client of a request. See [Known proxies](reverse-proxy.md#known-proxies).

**For app developers:** a session's `RemoteEndPoint`, and `IsLocal` and `IsInNetwork` of `/System/Endpoint`, follow the client's address: the one the proxy names for a request from a known proxy, the connection's otherwise.

### Finding servers on the local network

Jellyfin apps that look for servers on the local network list Polyfin, as they list a Jellyfin server with its auto discovery on: its address does not have to be typed. **Answer local network discovery**, under **Settings › General**, is on by default, as Jellyfin's is, and **Address on the local network** plays the part of Jellyfin's `PublishedServerUrl`. In Docker, the container's network decides whether the apps' messages reach Polyfin: see [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network) and [Local network discovery](administration.md#local-network-discovery).

**For app developers:**

- Polyfin listens on UDP port 7359, IPv4 and IPv6. A datagram that holds `who is JellyfinServer?`, in any letter case, as Jellyfin 12.4 reads it, gets one answer, sent to its sender: `{"Address":"http://192.168.1.10:8096","Id":"<server ID>","Name":"<server name>","EndpointAddress":null}`. Polyfin answers Emby's `who is EmbyServer?` the same way, which Jellyfin 12.4 leaves unanswered.
- `Id` and `Name` are those of `/System/Info/Public`. `Address` is **Address on the local network** when set, as Jellyfin's `PublishedServerUrl`. Otherwise it is `http://`, the address of the server's interface that reaches the sender, and Polyfin's HTTP port.
- Only senders on the local network are answered: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback. Jellyfin answers any sender.
- `LocalAddress` of `/System/Info/Public` and `/System/Info` is **Address on the local network** when set, as Jellyfin gives its `PublishedServerUrl` there, and otherwise the address the request reached: over the HTTPS Polyfin serves itself, `https://` and the HTTPS port included (see [HTTPS](administration.md#https)).
- While Polyfin serves HTTPS, discovery answers keep the HTTP address: a certificate for a public name does not match the server's address on the local network, and HTTP is never redirected there. Jellyfin, too, announces its HTTP address unless `PublishedServerUrl` says otherwise. To announce HTTPS, set **Address on the local network** to an `https://` address whose name the certificate holds.

### DLNA devices

TVs and players that play over DLNA, without a Jellyfin app, browse and play the libraries of the user an administrator chose for each (see [DLNA](dlna.md)). Their playbacks show in the web client's dashboard and in `/Sessions` like an app's, with the app named `DLNA` and the device's name; Polyfin cannot control them.

**For app developers:**

- A DLNA playback is listed in `/Sessions` under its user, with `Client` `DLNA`, `DeviceName` the device's name, `DeviceId` `dlna-` and the device's ID, `RemoteEndPoint` its address, `NowPlayingItem` and `PlayState`, until two minutes after the device last asked for anything. `SupportsRemoteControl` and `SupportsMediaControl` are false, and a listing with `controllableByUserId` leaves it out.
- `PositionTicks` is how far the device read, as Polyfin infers it: devices report nothing. For a file sent as it is, that is the share of the file's bytes sent during the playback, wherever they are in it. A DLNA playback counts against the user's **Playbacks at once**, as an app's does.

### Live TV tuners

The server's HDHomeRun tuners (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)) are Jellyfin's tuner hosts of type `hdhomerun`. Their channels are listed with the others in Live TV. jellyfin-web's tuner pages open the tuner's page in the admin app, or **Add a source** on its **HDHomeRun tuner** tab for a new tuner (see [Dashboard links and single sign-on](web-client.md#dashboard-links-and-single-sign-on)).

**For app developers:**

- `/LiveTv/TunerHosts/Types` lists HDHomeRun (`hdhomerun`), as Jellyfin names it; M3U tuners are not offered, as IPTV sources are added in the admin app.
- `/LiveTv/Tuners/Discover` (and Jellyfin's `/LiveTv/Tuners/Discvover`) lists the tuners that answer discovery, as Jellyfin's `TunerHostInfo`: `Type`, `Url`, `DeviceId`, `FriendlyName`, `TunerCount`, without an `Id`. With `newDevicesOnly=true`, those already added are left out. Administrators only.
- `POST /LiveTv/TunerHosts` adds a tuner from its `Url`, with `ImportFavoritesOnly`, and answers it with its `Id`; with the `Id` of a tuner, it changes its address and `ImportFavoritesOnly`. The tuner is read first: one that does not answer is refused (500), and so is one already added (400). Another `Type` answers 404, as in Jellyfin. Administrators only.
- `DELETE /LiveTv/TunerHosts?id=` removes the tuner, and answers 204 for an identifier that is not a tuner's, as Jellyfin does, leaving other IPTV sources be.
- `/System/Configuration/livetv` is Jellyfin's `LiveTvOptions`: the recordings' paddings in `PrePaddingSeconds` and `PostPaddingSeconds`, and, to administrators, the recordings folder in `RecordingPath` (null while recording is off) and the tuners in `TunerHosts`; its other settings are Jellyfin's defaults. A member gets no folder or tuner there, as they are of the server's own disk and local network. Polyfin's recording and guide settings are changed in the admin app (see [Live TV](live-tv.md)), not through this route.
- Settings Polyfin's tuners do not have take Jellyfin's defaults: `AllowStreamSharing` is true, as a channel's stream is always shared between its viewers.

### Backups

The backups page of the web client's dashboard, since Jellyfin 10.11, shows the database backups of the backups folder (see [Backups](backups.md)): it lists them, makes one, and restores one. Polyfin's backups hold its database alone, whatever the options asked: Jellyfin's also hold its metadata, trickplay images and subtitles, which Polyfin does not keep on disk.

**For app developers:**

- Administrators only, as in Jellyfin. While backups are off, `/Backup` lists nothing, `/Backup/Create` answers 403, and `/Backup/Restore` answers 404.
- `GET /Backup` lists the backups of the folder, newest first, as `BackupManifestDto`: `DateCreated` (when the backup was made), `Path` (the file name, such as `polyfin-20261005-040000.dump`; Jellyfin gives the archive's full path), `ServerVersion` (the Jellyfin version of `/System/Info`), `BackupEngineVersion` (`1.0.0`, Polyfin's format), and `Options` with only `Database` true. The backups made before a restore are listed too.
- `POST /Backup/Create` runs **Back up the database** at once, sends the backup to the destinations, and answers its manifest. The options of the body are ignored.
- `GET /Backup/Manifest?path=` answers the manifest of the backup of the folder named `path`, and 404 for any other path.
- `POST /Backup/Restore` with `ArchiveFileName`, a file name `/Backup` lists, restores it as the admin app does, and answers 204: Polyfin backs the current database up, stops, restores it and starts again, answering 503 meanwhile. It answers 404 for a backup the folder does not have, and 400 for a file that is not a backup of Polyfin's database, a backup of a newer Polyfin, a server without `pg_restore`, a restore already under way, and a backup holding stored keys the current `POLYFIN_SECRET_KEY` cannot decrypt, as apps cannot show that warning: the admin app restores those.
- An encrypted backup (`.dump.enc`, see [Encrypting the backups](backups.md#encrypting-the-backups)) is listed by `/Backup` like the others, but `POST /Backup/Restore` refuses it with 400 and the text `This backup is encrypted: restore it from Polyfin's admin app, which asks for its passphrase.`, as apps cannot ask for one. Jellyfin answers only 204 or 404 there ([BackupController.cs](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Jellyfin.Api/Controllers/BackupController.cs#L57-L70)): its backups have no passphrase.

## Jellyfin features Polyfin does without

These dashboard features answer as a Jellyfin 12.4 on which nothing of the kind is configured:

- plugins, packages and plugin repositories;
- browsing the server's folders;
- library folders and metadata providers;
- Live TV listing providers;
- the startup wizard.

Polyfin never shows the host's folders. To change the server's settings, use the [admin app](administration.md#settings) or the server configuration API.

**For app developers:**

- Answers go to administrators only where Jellyfin requires one.
- `/Plugins`, `/Packages`, `/Repositories`, `/Environment/Drives`, `/Environment/DirectoryContents` and `/Library/PhysicalPaths` list nothing.
- `/Libraries/AvailableOptions` lists no provider (Jellyfin also lists its own, its NFO reader and saver among them).
- `/System/Configuration/MetadataOptions/Default` and `/LiveTv/ListingProviders/Default` are Jellyfin's defaults.
- `/Startup/Configuration` and `/Startup/User` show the server name, language and earliest administrator; setup is always complete.
- Writes are refused as Jellyfin refuses them for what does not exist (an unknown plugin or package, path, or listing provider).
- Writes Jellyfin would perform answer 403: saving plugin repositories, and the wizard's configuration, remote access and first user. Change server settings through `/System/Configuration` instead.
- `/Environment/ValidatePath` answers 404 for every path.
