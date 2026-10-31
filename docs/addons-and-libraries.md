# Addons and libraries

This page explains how Polyfin turns Stremio addons and Eclipse music addons into Jellyfin libraries. It also covers song lyrics, catalog limits, collections, genre and people pages, similar titles, artwork and editing items.

## Addons

Polyfin gets its content from Stremio addons. Each addon has a role:

- Metadata addons provide catalogs and metadata.
- Stream addons provide streams and subtitles.
- Any addon that speaks the standard Stremio protocol works.

### Server addons

To share an addon with every user, open **Add a source** under **Content › Sources**, keep the **Stremio addon** tab, and paste its **Manifest address** (from the addon's configure page). Then pick under **Content › Libraries** which of its catalogs become libraries in Jellyfin apps.

- When an addon offers collection catalogs, those are enabled first.
- Otherwise its movie, series and TV catalogs are enabled. Its catalogs of other types, such as a cloud store's `other` catalog or a sports addon's `sport` catalogs, are not: enable them under **Content › Libraries** (see [What catalogs list](#what-catalogs-list)).
- Catalogs are enabled so only until the server has 20 libraries in all, counting those of every addon, so that home screens do not fill up. A user's own addons count the user's own libraries under **My sources** the same way. An addon added past that gets no library, and the message that confirms it was added says so: the server already has 20 libraries, and its catalogs can be enabled under **Content › Libraries**.
- This is only a starting point: you can enable any number of catalogs.
- When none of the addon's catalogs is enabled, as for an addon whose catalogs are all of other types, the message that confirms the addon was added says so, and that its catalogs can be enabled under **Content › Libraries**, rather than leaving nothing new to see in the apps. A user's own addon says the same of **My sources**.

**For app developers:** the answer to `POST /admin/api/scopes/{scope}/addons` tells `enabledLibraries`, how many of the addon's catalogs became libraries, and `libraryLimit`, 20 when none did because the scope had that many libraries already, absent otherwise. An Eclipse addon's `music.rows` counts its catalog rows: with none, "My music" is its library (see [My music](#my-music)).

Only administrators can install addons hosted on a local network address.

### Personal addons

Each user can add their own addons and libraries under **My sources**. Users can also turn the server's sources off for themselves with **Use the server's sources**.

A user plays only the versions their own sources list: their addons under **My sources**, and the server's while they use them. A version another user's own addon listed, or a server addon's for a user who turned **Use the server's sources** off, is refused to them on every path (PlaybackInfo, the stream, HLS, downloads, subtitles and attachments), as a version that does not exist, even by its identifier or from a URL kept from before. A file several sources list is, for each user, the link of the first of their own sources that lists it, with only the mirrors their sources list (see [Which streams become versions](playback.md#which-streams-become-versions)). Share-link guests play as the user who shared the title, and DLNA devices as the user they show the libraries of. Jellyfin likewise answers an item outside a user's access as one it cannot find ([`MediaInfoController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/MediaInfoController.cs#L171-L175), Jellyfin 12.4).

### Which streams play

A stream an addon lists plays when it is a link to a video, over `http` or `https`. A torrent's stream, an `infoHash` with its `fileIdx` and `sources`, plays only while **Play torrent streams** is on under **Settings › Torrents**, for the users with **Can play torrent streams**; otherwise it is left out of the versions, and a title whose addons list only torrents has none. See [Torrents](torrents.md). Web pages (`externalUrl`) and YouTube videos (`ytId`) never play. Every rule between an addon's streams and a title's versions, in the order Polyfin applies them, is in [Which streams become versions](playback.md#which-streams-become-versions).

### Identifiers addons accept

A stream addon's manifest tells which identifiers it lists streams for (`idPrefixes`, for each resource and type): some take IMDb's only (`tt`), some TMDB's (`tmdb:`), some an anime database's (`kitsu:`, `mal:`, `anidb:`). A movie or an episode is asked for by its own identifier when the addon takes it. Otherwise it is asked by the first of its other identifiers the addon takes, in this order:

1. its IMDb identifier;
2. its TMDB identifier;
3. its anime's Kitsu, MyAnimeList, then AniDB identifier, from the [anime mapping](tracking.md#anime) tracking uses.

Asked by another identifier, a title's type is `movie` or `series`, even when its catalog types it otherwise, as some anime catalogs do (`anime`). Such a title is also asked by its own identifier with that type when the addon takes only that type.

An episode is asked for as Stremio addons name episodes:

- By IMDb or TMDB, with its season and number: `tt…:2:3`, `tmdb:…:2:3`.
- By an anime database, with the number the anime gives it: anime databases make each season, or part of a season, an entry of its own with its episodes numbered from 1. The third episode of a series' second season may be `kitsu:<id>:3`, the id of the entry for that season.

Where the other identifiers come from:

- A title named by IMDb or TMDB has those its description gives (`imdb_id`, and TMDB's identifier when the addon gives it), with the same season and number. Its anime entry is the one the mapping gives for its IMDb or TMDB identifier, for a movie. For an episode, it is the entry that holds its TVDB season and number, as for [AniList and MyAnimeList](tracking.md#finding-the-entry). The series' TVDB identifier comes from its description, else from its IMDb identifier's entries when they agree.
- A title an anime catalog names (`kitsu:<id>`, `mal:<id>`, `anidb:<id>`) has the other anime databases' identifiers of the same entry, with the same episode number. Through the mapping, a movie also has its IMDb and TMDB identifiers, and an episode its series' IMDb identifier with its season and episode on TVDB. An episode is not asked by TMDB, whose seasons may be split otherwise, nor by IMDb when the anime is numbered as TVDB's absolute numbering.

Some titles are not asked by a mapped identifier:

- An identifier from the mapping is used only when one entry alone matches. A movie two entries claim, or a series whose entries point to different TVDB series, is not asked by any anime identifier.
- Specials, and episodes the mapping does not place, are asked only by the identifiers the description gives.
- Until the mapping's lists are read, titles are asked only by their own identifiers and those of their description.

Only Stremio addons are asked by other identifiers. IPTV sources and local folders list their own titles, and home videos and other videos stay their folder's alone.

Each addon's streams are kept under what it was asked, so its versions are renewed by asking the same again, and **Refresh** forgets them with the title's other lists. The streams of every addon merge into the title's versions as they always do, the same file listed twice being one version (see [Which streams become versions](playback.md#which-streams-become-versions)). At the `Debug` log level, each request to an addon for streams logs `An addon is asked for streams`, with the addon, the type and the identifier asked.

### Manifest addresses

Manifest addresses usually contain your addon settings or keys. Polyfin never shows them in full. To change one, use **Replace the address** in the source's more-actions menu.

Each source has its own page, from **Open its page** on its row, with its details, an **Active** switch and its place in the order. **Remove** asks first.

### Addons that fail

Polyfin asks an addon at the address it was added with, under the header `User-Agent: Polyfin/<version>`, with an Eclipse addon's settings in the query (see [Music addon settings](#music-addon-settings)). An addon that answers with an error was reached, and the log and **System › Health** tell what it answered:

| The addon | Health says | The log's error |
|---|---|---|
| Answers `401`, `403` or another error but `404` and `429` | refused the request | `addon refused the request: HTTP 403` |
| Answers `429`, or `503` with `Retry-After` | asked to slow down (too many requests) | `addon asked to slow down: HTTP 429`, or `addon asked to wait: HTTP 503, retry after 30s` |
| Answers `500` or another server error | answered with a server error | `addon failed: HTTP 502` |
| Does not answer | could not be reached, or did not answer in time | `addon unreachable: ...` |

A refusal comes from the addon, or from the site behind it, and Polyfin cannot change it. The token in the addon's address may have expired, or the addon may refuse requests from servers, or from apps other than its own. Check that the same address works in the addon's own app, on the same network as Polyfin, or ask the addon's author.

- A library whose addon fails shows nothing in the apps. The log tells each such request as `Browsing failed`, with the addon's name, the catalog and the error, as in `addon=Songs catalog=top error="addon refused the request: HTTP 403"`.
- A music addon whose search fails logs `A music addon could not search`, with its name, and the apps get the other addons' results.
- **Check**, in **System › Health**, asks for the manifest only: an addon that answers its manifest but refuses its catalogs or searches shows as answering until the apps ask it for them again.

### The order of sources

The order of **Content › Sources**, then each user's own order under **My sources** for their addons, which come after the server's, decides:

- which addon describes a title, the first that knows it;
- the order of the addons' subtitles;
- the order of search results;
- the order of a title's versions, addon after addon, while **Settings › Playback › Order versions by** has no criterion, and among the versions its criteria tie on.

To order versions by their source without moving an addon for the rest, choose **Source, in the order below** and order the sources under **Order of sources**, in **Settings › Playback** (see [Version order](playback.md#version-order)). That list changes the order of versions only.

### Languages

A title's name, overview and images are those of the addon that describes it, in the language set on that addon's configure page, which lives in its manifest address. To let users read titles in their own language, add the same metadata addon once more for each language, each configured on its configure page in that language, and name the language under **Language of its descriptions** on its page under **Content › Sources**. It is **None** by default.

Leave **Language of its descriptions** at **None** on the server's only metadata addon, whatever language its configure page sets, even when it is not the server's: name a language only on the extra copies added for other languages. The addons left at **None** give every user the titles' facts and episodes.

Each user then picks a **Metadata language** among those languages, under **My account**, or an administrator on the user's page under **Users** (see [Users](users.md#metadata-language)). **The server's** is the default.

For a user with a language:

- A title's page shows the description of the server's addon in that language: the name, overview, poster, backdrop and logo, and the episodes' names, overviews and images.
- In libraries, collections, smart lists, search results and home rows, a title shows its name and poster in that language once Polyfin keeps that description. Until then it shows the catalog's, and Polyfin reads the descriptions in the background, a few at a time.
- Search also asks the search catalogs of the addons in that language, so a title is found by its name in that language too.
- A title that no addon in the language describes shows the server's description.
- Users under parental control or blocking genres get their language too.

What does not change: a title's genres, ratings, people, versions, subtitles and identifiers stay those of the addons without a language, starting with the one whose catalog listed it. Genre pages, blocked genres, parental control, hidden titles and smart lists judge every user alike. Library names stay the administrator's, and generated images keep them.

An addon with a language describes titles for the users of that language only, even when its catalogs are libraries, unless no addon without a language describes them. An addon with a language and no library of its own describes titles only. Only the server's Stremio addons take a language.

When no addon without a language describes a title, the addons with a language describe it for every user, facts and episodes included: those in the user's language first, else in the order of the sources. So do search, people search and similar titles: when no addon without a language offers the search or the catalogs they need, those of the addons with a language serve every user. A server whose metadata addons all have a language thus works as if they had none, and **System › Health** warns about it (see [Health](administration.md#health)). Where an addon without a language describes the title, nothing of this applies.

Each language costs one more request per title: the addon in that language is asked once for each title opened or shown, and its descriptions are kept and refreshed as every description is (see [Catalog pages](#catalog-pages)). Catalogs are read once, whatever the languages.

## Libraries

Stremio catalogs become Jellyfin libraries. Stremio streams become versions of the same item, and addon subtitles become external subtitle tracks. An app's subtitle search lists the subtitles from the user's subtitle addons. See [Playback](playback.md) and [Subtitles](subtitles.md).

**For app developers:**

- Versions are Jellyfin media sources.

### The order of libraries

Jellyfin apps list the server's libraries in the order of **Content › Libraries**. To move a library, drag its row by the grip at its start, with the mouse or a finger: a line shows where it will land, and a list too long for its box scrolls while the row is held near its edge. Escape, during a drag, leaves the library where it was. The up and down arrows of the row move it too, from the keyboard as well, and screen readers announce each move. The new order is saved with the list's **Save**, like the libraries' names: until then, apps keep the order saved last.

Smart lists shown as libraries, and users' own libraries, come after the server's libraries. Users order their own libraries under **My sources** the same way, by dragging or with the arrows.

Each user can also order all of their libraries in the web player's **Settings › Home**, with the arrows of its list, then **Save**:

- An order changed there is the user's own: it wins over the administrator's, as in Jellyfin. A library added later, or removed and added again, comes after the ones the user ordered.
- jellyfin-web saves the order of every library each time that page is saved, even when only a home section changed. An order saved as it was shown, the server's own, is not kept as the user's: they go on following **Content › Libraries**, libraries added later included.
- The user goes back to the administrator's order by moving the libraries back to it there, or with **Reset my preferences** in **My account**, which also resets their other app preferences (see [Your own preferences](users.md#your-own-preferences)).

**Apply this order to every user**, under the list of **Content › Libraries**, forgets the orders users saved, so that every user's apps list the libraries in the order of **Content › Libraries**, as saved: save the list first. It asks first, saying how many users ordered their libraries themselves, and keeps the rest of their settings. Administrators only; the activity log records who did it, as "alex applied the order of the libraries to every user (2 with an order of their own)". Users can order their libraries again afterwards.

jellyfin-web keeps the list of libraries for up to a minute, also across a reload (see [Dashboard links and single sign-on](web-client.md#dashboard-links-and-single-sign-on)): a new order shows there within a minute. A user's own change in **Settings › Home** shows at once.

**Compared with Jellyfin:**

- Jellyfin lists a user's libraries in their order from **Settings › Home**, then by name ([`UserViewManager.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/UserViewManager.cs#L140-L158), Jellyfin 12.4). It has no order an administrator sets for every user: in Polyfin, the order of **Content › Libraries** takes the place of the order by name.
- jellyfin-web 12.4 saves every library's place in `OrderedViews` whenever **Settings › Home** is saved ([`homeScreenSettings.js`](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/homeScreenSettings/homeScreenSettings.js#L498-L506)). Jellyfin keeps it as posted, and answers it back in the user's `Configuration`; Polyfin stores an order identical to the server's own as empty, `"OrderedViews": []`, so that the user keeps following the administrator's order.
- Jellyfin has no action that applies one order to every user.

### What catalogs list

An addon gives each catalog, and each title, a Stremio type, which decides what the titles are in Jellyfin apps:

| Stremio type | Titles in Jellyfin apps | Library type |
| --- | --- | --- |
| `movie`, `anime.movie` | Movies | `movies` |
| `series`, `anime.series` | Shows, with their seasons and episodes | `tvshows` |
| `anime` | Shows, or movies for its `anime.movie` titles | none (mixed) |
| `other`, `channel` | Folders of videos, or videos | `homevideos` |
| `collection` | Collections (see [Collections from addons](#collections-from-addons)) | none (mixed) |
| `tv` | Live TV channels, not a library (see [Live TV](live-tv.md)) | |
| `track`, `album`, `artist`, `playlist` of a music addon | Music (see [Music addons](#music-addons)) | `music` |
| Any other type, such as `sport` or `events` | Folders of videos, or videos, as for `other` | `homevideos` |

A type Polyfin has no particular use for is listed as `other` is: Stremio apps show such types the same way. A sports addon's `sport` catalogs, for instance, become libraries of its matches and recaps, each a video, or a folder when its details list videos. A title without a type, in a catalog without one, is left out.

#### Folders of videos

`other` is Stremio's type for what is neither a movie nor a series: the files of a cloud store, for instance, whose addon lists each download as a title and its files as the title's videos. `channel` is a channel's uploads, the same shape, and so is every type Polyfin has no particular use for, such as `sport`. Polyfin lists them as Stremio apps do:

- A title whose details list videos is a folder. Apps open it to list its videos, in the order the addon gives them, each named after its video (or "Video 3" without a name), with its date and image when the addon gives them.
- A title without videos is a video itself, which plays the streams the addon lists for the title.
- Catalogs do not tell which titles have videos: their details do. Listing a library asks the addon for the details of the titles it does not know yet, a few at a time, for up to 5 seconds; a title still unknown then is listed as a folder, which lists itself as its one video when it turns out to have none, and is listed as it is from then on.
- The library is a home videos library, as Jellyfin's for videos that are not movies nor shows: jellyfin-web shows it with its **Folders** view, the titles, and its **Videos** view, every video of the library, through its folders, for its first 500 titles, or its **Maximum titles** when lower (see [Genre and maximum](#genre-and-maximum)).
- A folder shows how many of its videos are left to watch, and is played once all are. Marking it played, or unplayed, marks its videos; removing it from Continue Watching removes their resume points.
- Videos play as movies do: directly, remuxed or converted, with their resume point and in Continue Watching. The tracking services hear nothing of them. A video whose stream has no end, such as a match on now, plays live (see [Live videos](playback.md#live-videos)).
- Search finds them in the catalogs that offer it, as folders or videos. Parental control judges them as **Other**, unrated unless their details give a rating, and a video by its folder; blocked genres hide them by their details' genres. Hiding a folder, or one of its videos, hides the folder with its videos. A folder can be added to a collection.

#### Streams in a title's details

A video, or an episode of a series, may carry its own streams in the title's details (`videos[].streams`), which Stremio apps play without asking any addon for streams. Polyfin lists them as the video's first versions, with their names, file names and sizes as other versions get theirs, then the streams of the other addons. The addon that gave them is not asked for the video's streams: such an addon often answers that request with a notice alone, nothing to play. A stream whose link expires is renewed by asking the addon for the title's details again.

**For app developers:**

- A folder is a `Folder` item (`IsFolder` true) with `ChildCount`, `RecursiveItemCount` and `UserData.UnplayedItemCount`; its videos are `Video` items whose `ParentId` is the folder, and `/Items?ParentId=` lists them. A title without videos is a `Video` item of the library. The library's `CollectionType` is `homevideos`.
- `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Video` lists the videos through the folders, and tells more may follow while a folder's details are not known yet. `IncludeItemTypes=Folder` with `searchTerm` searches folders.
- A title keeps its item when its details gain or lose videos: it is listed as a folder or a video as they tell.

### Library images

Under **Content › Libraries**, each library shows a small image at the start of its row. Select it to choose, under **Image in apps**, the image Jellyfin apps show on the library's tile:

- **None**: no image. Libraries made before Polyfin 0.45 keep it until changed here.
- **Automatic**: Polyfin takes it from the catalog's first page: the first backdrop, as library tiles are wide, else the first wide poster, else the first poster. A music library takes the first artwork of its row. A catalog without any shows no image. Users under parental control or blocking genres are not shown it, since it may come from a title hidden from them.
- **Generated**: the default of the movie and series libraries made since Polyfin 0.45, as Jellyfin draws an image for each of its libraries; other libraries start with **None**. Polyfin draws a wide 16:9 image from the posters of the catalog's first titles, up to six, under a gradient, with the library's name written across them (see [Generated images](#generated-images)). A collection library takes the posters of its first collections, and for a collection without one, the first poster of its own generated image. A music library has no posters to draw from: it does not offer this choice. The image comes once Polyfin has read the catalog's first page, as apps do when they open the library. Users under parental control or blocking genres are not shown it, as for **Automatic**.
- **Custom**: **Upload an image**, or paste an address under **Or an image address**, which Polyfin downloads once. The picture follows the rules of [Edit images](#edit-images): a JPEG, PNG or WebP picture of up to 10 MB, kept in the database. Only administrators can use local network addresses. **Remove the image** deletes it, and the library shows none.

Apps are told the shape of the image: 16:9 for a generated image and for an automatic backdrop or wide poster, so that apps sizing tiles by it show it whole.

Image changes are saved at once, apart from the list's **Save**. A catalog just added to the list gets its image once the list is saved. Live TV catalogs make no library tile, so they have no image. Users choose the images of their own libraries the same way under **My sources**.

A library image uploaded with jellyfin-web's **Edit images** is its custom image too, and wins over the automatic or generated one; deleting it brings that one back. Choosing **None**, **Automatic** or **Generated** in the admin app deletes it.

A library taken out of the list keeps its custom image, for when it is added back. Removing the addon deletes the custom images of all its libraries.

**For app developers:**

- The library's image is its Primary image: `/UserViews` and `/Items` give its tag in `ImageTags`, `/Items/{id}/Images/Primary` serves it, and `/Library/VirtualFolders` names the library as its `PrimaryImageItemId`.
- The automatic image is looked up again with the catalog's pages, after **Refresh catalogs after (minutes)**. Its tag changes with the title it comes from. `/UserViews` never waits for it: until it is found, the library shows the image found last, or none.
- A generated image's tag changes with the library's name and with the titles it shows (see [Generated images](#generated-images)). `/UserViews` never waits for it: it comes from the pages Polyfin keeps.

### Genre and maximum

Under **Content › Libraries**, the funnel button at the end of a library's row opens its **Genre and maximum**. Both are saved with the list's **Save**, like the library's name. They apply to catalogs of titles and of collections, not to Live TV catalogs or music libraries. Users set the same for their own libraries under **My sources**. The row shows a library's genre and maximum at a glance.

- **Genre**: one of the genres the catalog offers in its genre filter. The library then lists only that genre's titles, as the addon narrows them. Without a name of its own, apps name it after its catalog and its genre, such as "Popular · Comedy". The choice shows only for a catalog that offers genres to choose from: not for one that requires a single genre, as many collection catalogs do.
- **Maximum titles**: the most titles the library lists, from 1 to 20,000, and as many in each of its collections. It replaces **Titles read per movie and series catalog** for this library, lower or higher (see [Catalog limits and refresh](#catalog-limits-and-refresh)). Polyfin reads no page of the catalog past it. Empty, that setting applies.

A library narrowed to a genre:

- offers that genre alone in apps' genre filters, and lists nothing for another;
- adds its titles to that genre's page only (see [Genre, studio and year pages](#genre-studio-and-year-pages));
- finds its automatic image among that genre's titles.

A genre the addon stops offering lists the whole catalog again, until another is chosen.

**Compared with Jellyfin:**

- Jellyfin libraries hold the files of their folders; a library's genre and maximum have no Jellyfin counterpart.
- Jellyfin lists every item of a folder when an app sets no `limit`. Polyfin lists up to 500: catalogs can be nearly endless.

**For app developers:**

- A listing of a library, of an addon's collection or of a collection made by users that sets no `limit`, as jellyfin-web's collection pages ask, lists up to 500 titles, or the library's maximum when lower. For a library or an addon's collection, the titles kept from earlier listings answer at once when they make the whole listing. Otherwise the listing waits for the addons 3 seconds at most, or 1 second when the titles kept fill a page of 100, then lists those read by then, and at least the first page of each catalog. It reads the rest in the background, for the next listing. A listing with a `limit` lists as many as it asks, within the library's maximum.

### Smart lists

A smart list gathers titles by what they are, from the catalogs it names, such as "French movies of the 1990s not watched yet". Administrators make the server's under **Content › Libraries**, with **Add a smart list**: every user sees them, unless their visible libraries leave a list shown as a library out. Users make their own under **My sources**: only they see them (see [Users' own addons](users.md#users-own-addons)).

A list draws from any catalog of movies or series of the server's addons, enabled as a library or not. A user's own list may also draw from their own addons. It then keeps the titles that match its rules:

| Rule | Matches |
|---|---|
| **Genre** | A title with this genre, in any letter case. |
| **Year** | A title that came out from one year to another, either left empty for no bound. |
| **Release date** | A title released in the last so many days, up to today, or in the next so many days, from today. A title whose catalog and description give only its year has no release date. |
| **IMDb rating** | A title whose IMDb rating is within the bounds given. |
| **IMDb votes** | A title whose IMDb rating counts a number of votes within the bounds given, as MDBList tells it (see [Ratings](#ratings)). Without the server's **MDBList API key for ratings**, no title has votes. |
| **Language** | A title whose description gives this language. |
| **Runtime** | A title whose runtime is within the bounds given, in minutes. |
| **Content rating** | A title rated at most this rating, such as PG-13, on the scale parental control compares ratings on (see [Parental control](users.md#parental-control)): PG-13 matches PG-13, PG and G titles, and the ratings of other countries of the same ages. **is not** matches the titles rated above it. |
| **Actor or director** | A title that credits this person, in any letter case, as an actor or a director. |
| **Studio or network** | A title made by this studio, or a series aired on this network, in any letter case, as TMDB names them, or as an administrator set a title's studios in its metadata. Without the server's **TMDB API key**, only those set by an administrator count. |
| **Movie or series** | Movies, or series. |
| **Place in its catalog** | A title among the first so many of its catalog, as the catalog orders them: by popularity for a popular catalog. |
| **Watch state** | A title the user opening the list played, did not play, started without finishing, or marked favorite. A series is played once every released episode is. |

- Each rule can be negated with **is not**.
- Titles match **All of the rules** or **Any of the rules**, and a group of rules inside the list matches all or any of its own.
- A list without rules keeps every title of its catalogs.
- **Exclusions** are rules too, in a group of their own: a title they match is left out of the list, whatever its rules say. A list without exclusions leaves no title out.
- A title that lacks what a rule looks at matches neither the rule nor its negation: a movie whose description gives no runtime is neither in a list of movies of at least 90 minutes nor in one of movies not of at least 90 minutes. Nor do exclusions leave it out for it.
- **Sort**: as its catalogs list them, one of each in turn, as an addon's collection does; by premiere date, the latest first; by the latest episode aired, or by the latest season premiere, the latest first; by IMDb rating, the highest first, those without one last; by name; or at random, in an order drawn again each day.
- **Latest episode aired** orders series by their latest regular episode that has aired, and **Latest season premiere** by the first episode of their latest season that has aired, as their descriptions date them. Specials (season 0) and episodes to come do not count. A movie comes by its premiere date. A series whose description is not read yet comes last until it is, and one without a dated episode last.
- **Maximum titles**: from 1 to 20,000, applied after the rules and the sort. Empty, the list has no maximum but its catalogs'.
- **Shown as**: a library or a collection. A list shown as a collection shows an image Polyfin draws from its first titles, as an addon's collection without a poster does (see [Generated images](#generated-images)); one shown as a library shows none.

The list's page shows a preview of its titles as they stand now, before you save. It also says how many titles still wait for what their rules look at: their description, their votes or their studios.

How a list is made:

- Polyfin reads each catalog up to its maximum, from the catalog pages it keeps and reads again after **Refresh catalogs after (minutes)**. That maximum is the library's own when the catalog is one of the user's libraries with a maximum, else **Titles read per movie and series catalog**.
- When the rules require a genre that a catalog offers in its genre filter, Polyfin asks the catalog for that genre's titles, as for a library's genre, rather than reading it whole. A place in that catalog is then a place among that genre's titles.
- The rules are applied each time the list is listed. The rules on what the user did are applied for each user, so a movie leaves "not watched yet" once played. A title the user hid leaves their lists at once, and comes back as soon as they show it again; other users still see it.
- A catalog page usually gives a title's genres, year and IMDb rating, sometimes its runtime, content rating, people and release date. The language, and a series' episodes, come only with the title's full description. Polyfin reads the missing descriptions in the background, a few at a time, as it looks up ratings for users under parental control. The server's lists ask the server's addons only. Once read, what a description does not give the title does not have.
- Until then, a title whose rules need its description is left out, negated rules included, and the list grows as descriptions come.
- A title's IMDb votes come with its MDBList ratings, looked up in the background in batches (see [Ratings](#ratings)); a title still waiting for MDBList is left out until it is answered.
- Studios and networks come from TMDB, by a title's TMDB identifier, else its IMDb one: Polyfin asks TMDB in the background for at most 50 titles at each listing, through the requests to TMDB it allows at once, and keeps the answers for every user, asking again after 30 days. A title waiting for TMDB is left out until it is answered; one TMDB does not know has no studio.
- A listing waits for the addons 3 seconds at most. Past that, it lists the titles of the pages already kept and reads the others in the background.

Limits:

- A list sees only the titles its catalogs list within their maximum, and it changes as they change.
- A list whose catalogs are long is slow to fill the first time. **Read every collection each day** reads the server's smart lists too.
- Parental control, blocked genres and allowed hours apply as everywhere. A user under parental control sees the server's lists only.
- A list keeps its own sort: apps cannot sort it otherwise.
- A library made of a list offers no genre to filter by.

**For app developers:**

- A list shown as a library is a `CollectionFolder` in `/UserViews`, after the libraries. Its `CollectionType` is `movies` or `tvshows` when all its catalogs are of that type; otherwise it has none, as the Watchlist. `/Items?ParentId=` lists its titles and `/Items/Latest?ParentId=` gives its home row.
- A list shown as a collection is a `BoxSet` in the **Collections** view, by name with the collections made by users. The view shows once such a list exists.
- The collections listing across the server lists it, except for jellyfin-web's **Add to collection** dialog. `POST` and `DELETE /Collections/{id}/Items` answer an empty `403`.
- The admin API serves the lists under `/admin/api/scopes/{scope}/smart-lists`. A list's `rules` and `exclude` are each a group of rules (`{"field": "group", "match": "all" | "any", "rules": [...]}`); a list sent without `exclude` has none. A rule's `field` is `genre`, `year`, `released`, `rating`, `votes`, `language`, `runtime`, `contentRating`, `person`, `studio`, `type`, `rank` or `state`; `released` takes `min` and `max` in days from today, negative in the past. `sort` is `catalog`, `premiere`, `latestEpisode`, `seasonPremiere`, `rating`, `name` or `random`. `.../catalogs` gives the catalogs a list may draw from, the languages its rules may name and the `contentRatings` a rule may set as the highest. `POST .../preview` answers `titles`, `listed`, `unknown` (titles waiting for their description, votes or studios) and `complete` (whether every catalog answered in time).

### Libraries hidden from the top bar

Under **Content › Libraries**, **Hide from the web player's top bar** in a library's row leaves the library out of the web player's menus: the buttons of its top bar, the bar's **More** menu, and its side menu, in both of its layouts. It is saved with the list's **Save**, like the library's name, and the row then says **Hidden from the top bar**. Users set the same for their own libraries under **My sources**. Live TV catalogs have no such choice: their channels go to Live TV, never to a menu.

- Only the web player (jellyfin-web, at `/web/`) hides it. There, the library keeps its row on the home screen, and its pages still open from it. Other Jellyfin apps keep listing it.
- When every library the **More** menu would list is hidden, at the window's width, the **More** button goes too.
- It applies the next time the web player is loaded, or a user signs in to it. After a user's first load in a browser, a reload doesn't show the hidden links, even briefly: the browser keeps the last choice.

**Compared with Jellyfin:**

- Jellyfin has no counterpart that keeps the home row: a library in a user's `MyMediaExcludes` leaves the web player's menus and its home screen rows together, in Polyfin as in Jellyfin.

**For app developers:**

- The web player's Polyfin script reads the choice from `GET /Polyfin/UserViews` (see [Libraries left out of the web player menus](jellyfin-compatibility.md#libraries-left-out-of-the-web-player-menus)) and hides the library's links with a style. `/UserViews` and every other answer list the library as before.

## Music addons

Eclipse music addons install like Stremio addons, with the **Eclipse addon** tab of **Add a source**: under **Content › Sources** for the server, or under **My sources** for a user's own. You can add one by its manifest address or by its base address (`https://addon.example/{token}/`). Polyfin tells the two kinds of addon apart by their manifest. Eclipse addon addresses are redacted like manifest addresses. The sources list names their kind **Eclipse**, in its filter and on their badge, and **System › Health** marks their row with the same badge.

### Music addon settings

Change the settings an addon declares (pickers, switches, texts and numbers) under **Addon settings**, on the addon's own page under **Content › Sources** or **My sources**.

- Each setting holds one value.
- A per-network setting keeps its Wi-Fi default.
- Settings travel as query parameters with every request to the addon, as Eclipse sends them.

### Music libraries

Each catalog row (songs, albums, artists or playlists) is a library under **Content › Libraries**:

- a Jellyfin music library; or
- a books library, for an addon whose `contentType` is `audiobook`. Its tracks are audiobooks, with the chapters their stream gives.

Podcasts have no Jellyfin type: their episodes are songs.

A [local music folder](local-folders.md#music-folders) is a music library too: its row of albums, read from its scans rather than from an addon, with albums, artists, search, lyrics, scrobbling and playback as an Eclipse addon's. It is never kept as addon answers are, and its songs view lists every album's songs, not the first 50.

Each user can have the songs they play scrobbled to their own Last.fm or ListenBrainz account: see [Music: Last.fm and ListenBrainz](tracking.md#music-last-fm-and-listenbrainz).

A row lists its own items. When an app asks for another kind, the library derives it:

- the albums and artists its songs name; or
- the songs of its albums, playlists or artists, reading at most 50 of their pages, 8 at a time.

An album or artist that a song only names is found again through the addon's search when opened. So addons without catalogs still serve search and album, artist and playlist pages. An artist whose page at the addon lists no albums shows those its top songs name.

Polyfin keeps what music addons answer. Catalog pages are read again after **Refresh catalogs after (minutes)**, and album, artist and playlist pages after 6 hours. A page due to be read again is still served at once, for up to 24 more hours, while Polyfin asks the addon again in the background. Searches are kept 10 minutes. At most 8 requests for pages go to one addon at a time.

#### My music

Every music addon with a search also offers "My music", a catalog Polyfin makes, listed with the addon's rows under **Content › Libraries** and **My sources**. It is enabled, named and ordered like any library; without a name of its own, apps show it as "My music" in the server's language, such as "Ma musique" in French. Each user sees their own music of the addon in it:

- the songs they played, most recently played first, then the songs they marked favorite;
- then the albums, artists and playlists they marked favorite.

Asked for another kind, it lists the albums and artists its songs name (and its albums' artists), or the songs of its albums, playlists and artists, as a row does. Each kind lists at most **Titles read per movie and series catalog**. Searching from it searches the addon, as in any music library.

An addon installed without catalog rows of its own, as many music addons that only search, gets "My music" enabled as its library, so that music apps starting from a library find the addon. It starts empty, for every user: a user's songs show up there once they found them with search and played them or marked them favorite. The message that confirms the addon was added says so. An addon with rows keeps them as its libraries, with "My music" offered but off. Like other addons' catalogs, rows and "My music" are enabled so only until the server has 20 libraries (see [Server addons](#server-addons)). Addons installed before keep their libraries: an administrator enables "My music" under **Content › Libraries** (users, under **My sources**). Polyfin never asks the addon for this catalog, and it has no automatic image, since every user sees a different list.

### Instant mixes

- A song's instant mix is that song, then its album's and its artist's other songs.
- An album's mix is its songs and its artist's songs.
- An artist's or a playlist's mix is their songs.
- Every mix is shuffled.

### Lyrics

Addons give no lyrics. With **Song lyrics from LRCLIB** on, Polyfin finds them on [LRCLIB](https://lrclib.net), a free and open lyrics database, and apps show them. A song of a [local music folder](local-folders.md#lyrics) with a `.lrc` file or a lyrics tag has those instead, whatever this setting:

- synced line by line when LRCLIB has them, so that apps follow the song;
- else as plain text.

Instrumentals, songs LRCLIB doesn't know and audiobooks have no lyrics.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Song lyrics from LRCLIB** | **Settings › Content** | On | Looks songs up on LRCLIB and gives apps their lyrics. Off, songs have no lyrics and LRCLIB is never asked. Lyrics found before are kept, and shown again once it is back on. |

Polyfin looks a song up when it starts playing, when its details open, or when an app asks for its lyrics. It never looks up a whole album or list at once: listings only show what is already known.

- Polyfin asks LRCLIB by the song's artist, title, album and length, and sends nothing else. Its requests name Polyfin, its version and its repository, as LRCLIB asks.
- Each song is asked about once. Polyfin keeps what LRCLIB answered, lyrics or not, in its database. A song LRCLIB didn't know is asked about again after 30 days, as LRCLIB grows every day.
- A request that fails (LRCLIB unreachable, an error, or no answer within 10 seconds) is not taken for "no lyrics": the song is asked about again later. After a failure, Polyfin leaves LRCLIB alone for a minute, or as long as LRCLIB asks.
- Playback and listings never wait for LRCLIB. A song's details wait for it one second at most, and a request for its lyrics 5 seconds. A later answer serves the next request.
- At most 4 requests go to LRCLIB at a time.

Apps show their lyrics button once a song's lyrics are known. The web player takes it from the list the song was played from: a song looked up as it plays for the first time shows the button the next time it is played from a list, or at once on its own page.

#### Choosing lyrics

When the lyrics found are another song's, or LRCLIB found none under the song's name, an administrator can choose others in the web player, with **Song lyrics from LRCLIB** on:

1. In the song's menu, choose **Edit lyrics**. The song's lyrics, if any, show at the top.
2. Below them, choose the search button. Polyfin searches LRCLIB for the song's title, artist and album, and lists what it found under the name LRCLIB: the lyrics of the songs whose length is closest first, synced before plain. Each result tells whether it is synced, and its preview button shows all its lines.
3. Choose the download button of the lyrics you want. They are the song's lyrics at once, for every user, synced or plain as listed.

An administrator can also upload a lyrics file of their own, whether **Song lyrics from LRCLIB** is on or not: in the editor, choose the add button beside the search button, pick an `.lrc` or `.txt` file, and upload it. Its lyrics are the song's at once, for every user.

- The file is read as a local song's `.lrc` file is (see [local music folders](local-folders.md#lyrics)): synced line by line when its lines start with a time, plain text otherwise, a `.txt` file too. The word timing of enhanced LRC is left out.
- A file over 1 MiB, one that is not UTF-8 text, or one without a line of text is refused, and the song keeps its lyrics. 1 MiB holds any song's lyrics many times over.
- Lyrics chosen and lyrics uploaded are kept in Polyfin's database as those found are, and replace them: no automatic look-up replaces them later. Choosing or uploading other lyrics replaces them again.
- They replace a local song's own lyrics too, from its `.lrc` file or lyrics tag.
- The editor's delete button, once confirmed, removes the lyrics Polyfin keeps for the song, chosen, uploaded or found, and the song has none. The removal is kept as a choice too: no automatic look-up gives the song lyrics again, until lyrics are chosen or uploaded for it. A local song's own lyrics are never deleted: its `.lrc` file and its tag stay as they are, and its lyrics show again.
- With **Song lyrics from LRCLIB** off, the search finds nothing and LRCLIB is not asked. Lyrics chosen are kept, and shown again once it is back on. Lyrics uploaded are not LRCLIB's: they are shown whatever the setting. The delete button removes either.
- Searches and choices are asked of LRCLIB as look-ups are: they name Polyfin in the same way and count among its 4 requests at a time. While LRCLIB is left alone after a failure, or when it fails, the search finds nothing. Uploads never ask LRCLIB.
- Only administrators see **Edit lyrics**. Polyfin refuses others' searches, choices, uploads and removals.

**Compared with Jellyfin:**

- Jellyfin reads lyrics from the `.lrc` and `.txt` files next to the songs, or downloads them with a lyrics plugin. Polyfin reads the `.lrc` files and lyrics tags of local music folders, and asks LRCLIB itself, without a plugin.
- Jellyfin saves the lyrics chosen or uploaded in the lyrics editor as a file next to the song, and its delete button deletes the song's lyrics files. Polyfin keeps them in its database and removes only those: the song's files are left as they are.
- Jellyfin reads an uploaded `.txt` file as plain text. Polyfin reads it as an `.lrc` file, synced when its lines start with a time.
- Lines are synced, not words: the word timing of enhanced LRC is left out.

**For app developers:**

- `GET /Audio/{itemId}/Lyrics` answers Jellyfin's `LyricDto`. Synced lines have `Text`, `Start` in ticks and an empty `Cues`; plain lines have only `Text`, which apps tell plain lyrics by. `Metadata` gives the song's `Artist`, `Album`, `Title`, `Length` and `IsSynced`.
- A song without lyrics answers `404`, as in Jellyfin.
- Songs carry `HasLyrics`, `true` once their lyrics are known, in listings as in details.
- `GET /Audio/{itemId}/RemoteSearch/Lyrics` answers Jellyfin's `RemoteLyricInfoDto` list: each result's `Id`, `ProviderName` (`LRCLIB`) and `Lyrics`, a `LyricDto` whose `Metadata` describes the song LRCLIB has them for. `POST /Audio/{itemId}/RemoteSearch/Lyrics/{lyricId}` chooses a result and answers the song's new `LyricDto`; an identifier that is no result, and lyrics LRCLIB no longer has, answer `404`.
- `DELETE /Audio/{itemId}/Lyrics` removes the lyrics Polyfin keeps for the song and answers `204`, whether it had lyrics or not, as Jellyfin answers whether it deleted a lyrics file or found none.
- `POST /Audio/{itemId}/Lyrics?fileName={name}` uploads a lyrics file, as jellyfin-web sends it: its text as the body, `text/plain`. Files named `.lrc`, `.elrc` or `.txt` are taken, and the answer is the song's new `LyricDto`. Like Jellyfin, it answers `400` without `fileName`, without a body, for a name without an extension, and for a file it cannot read: another extension, text that is not UTF-8 or holds control characters other than tabs and line breaks, or no line of text. A body over 1 MiB answers `413`.
- These four answer `403` to users other than administrators and API keys, as Jellyfin answers users without the permission to manage lyrics, and `404` for an item that is no song the user sees. Users' `Policy.EnableLyricManagement` is `true` for administrators.

### Music playback

Polyfin decides how to play a track with the app's device profile, as Jellyfin decides for audio. It uses the codec, container, sample rate and bit depth from the addon's stream reply. When the reply or the track listing gives only a known format (`flac`, `mp3`, `aac`, `m4a`, `opus`, `ogg` or `wav`), Polyfin takes the codec and container from it.

- The stream is analyzed, as an audio file, only when neither gives its codec and container. Tracks listed with a `streamURL` play from it the same way.
- The reply does not give a lossless stream's bitrate. Polyfin takes it as the bitrate of its samples in stereo, so that a bitrate limit converts it.
- Polyfin plays the track as it is, through the same source and relay rules as videos (see [Playback](playback.md)). A track whose link expires before the track ends is always relayed, so that Polyfin can renew the link while it plays.
- Or it converts the track with FFmpeg to the codec, bitrate, sample rate and channels the app asks for, progressively or in 3-second HLS segments (see [Transcoding](transcoding.md)).
- An expired link is asked for again, a minute before it expires (`expiresAt`) or once it fails. A link given less than 15 seconds ago is used until it expires, so one play asks for it once. Plays of the same track at the same time share one request.
- A track whose addon gives no stream (it cannot be reached, answers an HTTP error other than `404`, or gives no stream address) does not play: `PlaybackInfo` answers `NoCompatibleStream`, as for a title none of whose versions plays, and apps say the track cannot be played. A `404` means the track no longer exists: it is not found.
- With **Prepare playback in advance** on, Polyfin resolves the next track as a track starts: the next one in the queue the app reports, else the next one on the album (see [Playback](playback.md#preparing-playback-in-advance)).

The user's conversion permission, bitrate limit and number of playbacks at once apply (see [Users](users.md)). Quality groups, which are about video heights, do not.

Playback reports count plays and mark songs played, without resume points. Audiobooks keep their resume points, with Jellyfin's audiobook thresholds. The activity log records audio playback as Jellyfin does.

Eclipse's video renditions are not used: Jellyfin music apps cannot show a video for a song.

### Volume between songs

Songs from different albums and addons are mastered at very different volumes. Polyfin tells apps how much to raise or lower each song and each album, as Jellyfin's `NormalizationGain` does, and apps that apply it, such as the web player, play them all at about the same volume. The songs themselves are not changed: an app that ignores the gain plays them as before.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Even the volume between songs** | **Settings › Content** | On | Gives apps the gains known. Off, apps get none and nothing is measured. |
| **Measure the loudness of songs without a gain** | **Settings › Content** | Off | **Off**, **Local songs** or **Local and addon songs**: measures, with FFmpeg's `ebur128` filter (EBU R128), the songs that have no track gain. |

- Gains are relative to -18 LUFS, the reference of ReplayGain 2.0 and of Jellyfin's gains.
- A song's gain is its ReplayGain track tag (see [Files and tags](local-folders.md#files-and-tags)), else its measured loudness.
- An album's gain is its album tag, when all its songs agree on one, else the loudness of all its songs together, each weighed by its length, once each one's is known.
- Songs of a [local music folder](local-folders.md#music-folders) are measured in the background after a scan. Addon songs are measured after their first play, by reading their stream once more through the same relay and link renewal as playback; never a whole album or catalog ahead.
- A song is measured once, its audio alone decoded. Its loudness is kept in the database with the song, as an analysis is, and measured again only when a local file changes.
- Two songs are measured at a time, over local and addon songs, with FFmpeg at the lowest priority, as for the audio of local episodes (see [Skip segments](skip-segments.md)).
- Songs longer than 15 minutes, podcasts' long episodes among them, songs whose length the addon does not give, and audiobooks are never measured.
- A song whose stream fails is tried again after a day, three times at most. A silent song has no gain.

### Health and parental control

Music addon requests count in **System › Health** like a Stremio addon's: last answer, failures and response time, with the same manifest check (see [Administration](administration.md)). What a failure says, and what to check when an addon refuses Polyfin's requests, is in [Addons that fail](#addons-that-fail).

- Explicit songs and albums are hidden from users whose parental control sets a highest rating.
- Music has no rating, so all music (books, for audiobooks) is hidden from users who block unrated music.
- Visible libraries and allowed hours apply as elsewhere.

**Compared with Jellyfin:**

- Jellyfin's instant mixes pick songs that share genres, which addons rarely give. Polyfin's mixes are built from album and artist instead.
- Songs have no resume points, as in Jellyfin.

**For app developers:**

- Music libraries have CollectionType `music`. Audiobook tracks are `AudioBook` items; podcast episodes are `Audio` items.
- Jellyfin music apps browse `/Artists`, `/Artists/AlbumArtists`, and `/Items` by `parentId`, `artistIds` or `albumIds`, with Jellyfin's sorting and paging.
- The latest albums, search and search hints go through the addons' `/search`. Artwork comes from the addons.

## Catalog limits and refresh

Some catalogs are nearly endless, so Polyfin stops reading a catalog after a set number of items. A higher number shows more, but lists load more slowly and the addons get more requests.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Titles read per movie and series catalog** | **Settings › Catalogs** | 2,000 (100 to 20,000) | Item limit for every catalog except Live TV ones. |
| **Channels read per Live TV catalog** | **Settings › Catalogs** | 10,000 (100 to 50,000) | Item limit for Live TV catalogs (see [Live TV](live-tv.md)). |
| **Keep version lists for (minutes)** | **Settings › Catalogs** | 10 (1 to 360) | How long a title's versions and subtitles from the addons are used before Polyfin asks the addons again. |
| **Refresh catalogs after (minutes)** | **Settings › Catalogs** | 60 (1 to 1,440) | How old a catalog page, the Live TV guide's included, may get before Polyfin reads it again. Apps never wait for it. |
| **Read every collection each day** | **Settings › Catalogs** | Never (or an hour, 0 to 23) | Reads every collection of the server's collection libraries at this hour of the server's time zone, one after the other, then the catalogs of the server's [smart lists](#smart-lists), so that they open at once. It asks the addons for many pages: leave it off for an addon you share with others. |
| **Apps that show collection libraries by collection** | **Settings › Catalogs** | None | The apps, among those that have signed in to the server, told that collection libraries hold collections, as Jellyfin's Collections view does, so that they list a collection library's collections, then a collection's titles (see [Collections from addons](#collections-from-addons)). Do not tick jellyfin-web or Streamyfin: they leave such libraries out of their home rows. |
| **Scan local folders every (hours, 0 = never)** | **Settings › Catalogs** | 6 (0 to 168) | How long after its last scan a local folder or network share is scanned again (see [Local folders](local-folders.md#scanning)). 0 turns the schedule off. |
| **Look for versions of watchlist titles every (hours, 0 = never)** | **Settings › Catalogs** | 6 (0 to 168) | How long after its last check a title of a user's Watchlist they have not started is checked again for versions, for the **Now available** and **Still waiting** home rows (see [Tracking](tracking.md#now-available-and-still-waiting)). Each hour, up to 20 titles of each user are checked. 0 turns the checks off. |
| **Watch folders and shares for changes** | **Settings › Catalogs** | On | Scans a folder mounted in the container a few seconds after its files stop changing, an SMB share a few seconds after its server tells of a change, and a WebDAV share a few seconds after a listing of its folders, made every 2 minutes, differs from its last scan's (see [Watching for changes](local-folders.md#watching-for-changes)). |

A library can have its own maximum instead, lower or higher, and list only one genre of its catalog: see [Genre and maximum](#genre-and-maximum).

A longer **Keep version lists for (minutes)** sends fewer requests to the stream addon, which helps with providers that refuse too many. But new versions show up later. Preparing playback ahead readies the next episode a minute before that time ends: at most 9 minutes and at least 1 minute before the end of the episode. Whatever the setting, Polyfin asks an addon again 10 seconds after its first answer for a title, and once more 30 seconds later while its answers grow, for addons that gather other addons' streams (see [Title pages](playback.md#title-pages)).

Once that time has passed, a title opened again still shows the versions known before at once, even old ones, while Polyfin asks the addons again: Polyfin keeps the old lists 24 hours more for this. Versions missing from a new answer that lists only part of them stay listed until Polyfin stops asking that addon again.

### Catalog pages

Catalog pages work the same way. Once a page is older than **Refresh catalogs after (minutes)**, it still shows at once, and Polyfin reads it again from the addon in the background. It is kept 24 hours more, and also kept when the addon fails. A new title shows up one visit later.

Polyfin also keeps catalog pages and titles' descriptions in its database. So libraries, collections, home rows and title pages show at once after a restart too, with the images the descriptions give, and a page read from the database past that time is read again in the background, as above. A page no one read for 24 hours past that time is forgotten, and so is a description after a week. Searches stay in memory.

To answer quickly:

- While you look at a page of a library or collection, Polyfin reads the next one ahead, unless the addon's last answer failed.
- When it starts, and every hour, Polyfin reads the first page of every library: the scheduled task "Read the libraries' first pages" in jellyfin-web's dashboard. It then warms the posters of those pages, so that a home row or a library shows them at once the first time too (see [Artwork](#artwork)).
- Home rows (**Latest**) show the first page of each catalog only, as Stremio apps do.
- Polyfin remembers how many titles each catalog's pages hold and where a catalog ends. A collection read again asks its catalogs for all the pages it needs at once.
- jellyfin-web's collection pages ask for every title at once, up to 500 in Polyfin. Reading them all can take long the first time: the page waits for the addons 3 seconds at most and shows the titles read by then, at least the first page of each catalog, while Polyfin reads the rest in the background. The next time, the page shows them all at once; if Polyfin is still reading, it shows those read so far after 1 second at most.
- With **Read every collection each day** set to an hour, Polyfin reads every collection of the server's collection libraries whole at that hour: the scheduled task "Read every collection", which an administrator can also run by hand. It reads one collection after the other, then every catalog of the server's smart lists up to its maximum, asks the addons again for the pages past **Refresh catalogs after (minutes)** and waits for their answers, so that every collection page and smart list then opens at once with all its titles. The server log tells how many collections and smart lists it read and how long it took.
- A title's description is read again after 6 hours, or when its page is opened after **Refresh catalogs after (minutes)**. Meanwhile, the one known shows. A whole library's are read again on request (see [Refreshing a library's descriptions](#refreshing-a-library-s-descriptions)).
- A search reads the first page of the search catalogs, those that take the `search` property and require no other, as Stremio apps do, and lists up to 100 titles. It answers once the title searches have answered, or 3 seconds after the first of them found its page, 10 seconds at most when none did, then waits 0.3 seconds more for people-search catalogs. A later answer is kept for the next search of the same term: searches are kept 10 minutes, an empty answer 1 minute, as an addon may answer nothing while what it reads from fails.
- A search catalog that did not answer a search in time is left out of the searches for 5 minutes, unless one of its pages comes in meanwhile: an addon that never answers delays one search, not each of them. **System › Health** tells it meanwhile, and the log names the addon. Matching a local folder's files and **Identify** still wait for every addon (see [Failed searches](local-folders.md#failed-searches)).
- When an app gives up on a request, what Polyfin was reading for it goes on and is kept for the next request. Other apps waiting for the same page get it.

Every change to these settings applies at once, to what is already kept too.

#### Addons that ask to wait

Public addons limit how often one server may ask them. Polyfin keeps to what they ask:

- **At most 8 pages at once:** Polyfin asks one addon's server for at most 8 catalog pages at a time, over all the listings, refreshes and users asking. The others wait their turn. Users' own copies of an addon, and the addons a server serves, share its 8. Searches are not counted: a search asks each search catalog once a term, and they are bounded where they are made.
- **The wait it asks for:** an addon that answers a catalog page "too many requests" (HTTP 429), or 503 with `Retry-After`, is asked for no catalog page until the wait it asks for (`Retry-After`) ends, 15 minutes at most. Without `Retry-After`, it waits 5 seconds, then twice as long each time it refuses again after a wait. The requests already under way when it refused still get their answers; the others are not sent.
- **Meanwhile:** the pages kept still show, and none is read again. A page never read, such as another genre or a page further down, cannot be listed: a listing shows the titles before it as the whole listing, and its total counts them only. A listing that would start past them, or a library whose first page was never read, fails as before, and apps show their error. Once the wait ends, the next visit reads the rest. Searches are still asked, one request a term: [local folders](local-folders.md#failed-searches) wait as the addon asks.
- **Failed refreshes:** a page whose refresh failed, whatever the reason, is not read again for a minute: the page kept shows meanwhile.
- **Watchlist checks:** the checks of the users' Watchlists for versions ask no stream of an addon whose server waits, and a check the addon answers "too many requests" starts its server's wait too (see [Now available and Still waiting](tracking.md#now-available-and-still-waiting)). Title pages and playback ask for streams as before.
- **Log and Health:** the log says once, at Info, that the addon asked to wait, with the catalog, the page (`skip`, `genre`) and the wait: `An addon asked to wait: no catalog page is asked of its server until the wait ends`. With **Detailed log**, each refresh that failed otherwise says which page. **System › Health › Addons** shows the addon as "asked to slow down (too many requests)" until the wait ends, even when it answers other requests meanwhile.

The same is true of a page that cannot be read for another reason, such as a server error: a listing shows the titles before it. A longer **Refresh catalogs after (minutes)** has an addon asked less often.

**Compared with Jellyfin:** Jellyfin lists its own database, so a listing never lacks titles because a remote source refused it. Polyfin's `TotalRecordCount` then counts only the titles it could read, so that apps page through what is there rather than ask for titles it cannot give.


### Refreshing a library's descriptions

Under **Content › Libraries**, the menu at the end of the row of a movie or series library of the server's has **Refresh descriptions**: Polyfin reads again, in the background, the descriptions of the library's titles, as **Refresh metadata** does for one title. A title's overview, people, genres, rating and artwork then follow its metadata addon's new answer. An addon that fails leaves the description as it was. Libraries of collections, Live TV, music and IPTV sources, and users' own libraries, do not offer it.

Jellyfin apps start the same refresh: jellyfin-web's **Refresh metadata** on a server movie or series library, from the menu of its card in the "My Media" row of the home screen, queues it as the admin app's menu does, in the same queue, and the library's row under **Content › Libraries** shows it. The request answers at once, as Jellyfin's does once it queued the refresh. Asked again while the library is refreshing or waiting to be, it starts no other.

- **Modes:** the refresh starts when the metadata or image refresh mode is "Scan for new and updated files" (`Default`) or a full refresh (`FullRefresh`), what jellyfin-web sends. Without a mode, `None` in Jellyfin, or with `ValidationOnly`, which ask no metadata provider, it starts nothing. Images come with the descriptions, so an image mode starts it too.
- **Ignored:** "Replace all metadata" (`replaceAllMetadata`), "Replace existing images" (`replaceAllImages`) and `regenerateTrickplay`: a description read again always replaces the one kept, the images follow it, and the image cache stays as it is (see [Clearing the image cache](administration.md#clearing-the-image-cache)). Both modes do the same refresh.
- **Libraries that do not offer it:** a collection, music or IPTV library, a smart list, or a user's own library answers as before, `204` with nothing refreshed, as Jellyfin answers for any item it queues. A library the user cannot see answers `404`, as Jellyfin answers for an item it does not find.

- **Which titles:** those of the library's catalog pages Polyfin keeps, the pages apps opened and the first pages read every hour, up to the library's maximum, in the catalog's order; for a local folder, every title it lists. Polyfin does not read the rest of a remote catalog to find more titles: that alone could cost thousands of requests.
- **Its pace:** one title every 2 seconds, 30 a minute at most, whatever the addons. A title asks one request of the addon that describes it, or of the next one when it fails. A library of 600 titles kept takes about 20 minutes. Public metadata addons limit how often one server may ask them, and some block a server that asks too fast: at this pace, a refresh stays well below what apps browsing ask for.
- **Rate limits:** an addon that answers "too many requests" (HTTP 429) is waited for as long as it asks (`Retry-After`), 30 seconds then doubled when it does not say, at most 15 minutes, and asked again, 3 times at most; the title then counts as not described, and its description stays. Its failures show in **System › Health** as any request's.
- **One at a time:** libraries asked to be refreshed while one is refreshing wait their turn, so the pace bounds them all. The refresh asks only for descriptions, never for catalog pages: "Read the libraries' first pages", every hour, and the apps' requests go on beside it, its one request at a time added to theirs.
- **Progress:** the library's row shows **Waiting to refresh its descriptions**, then the titles described against those to describe, with a bar, then the result: the titles described and those no addon described. **Stop refreshing**, in the same menu, stops it, or takes it out of the queue: the titles described stay so. A restart stops it too.

**Compared with Jellyfin:** jellyfin-web's **Refresh metadata** on a library offers three modes ([refreshdialog.js, lines 25 to 28 and 76 to 79](https://github.com/jellyfin/jellyfin-web/blob/b29095510e80/src/components/refreshdialog/refreshdialog.js#L25-L79)): "Scan for new and updated files" (`MetadataRefreshMode=Default`), "Search for missing metadata" (`FullRefresh`) and "Replace all metadata" (`FullRefresh` with `ReplaceAllMetadata`), with "Replace existing images". The server queues the library ([ProviderManager.cs, lines 1136 to 1162](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Providers/Manager/ProviderManager.cs#L1136-L1162)), then refreshes each item of its folders, several at once: as many as "Library metadata refresh concurrency" allows, the processor's count by default ([Folder.cs, lines 903 to 912](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Controller/Entities/Folder.cs#L903-L912), [ServerConfiguration.cs, line 251](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Model/Configuration/ServerConfiguration.cs#L251)), without a pace. Polyfin's refresh is always a full one, as "Replace all metadata" (a description is the addon's whole answer), takes the titles Polyfin keeps rather than every item, and goes one title at a time at a fixed pace, as its libraries are remote catalogs and their metadata addons public services. Images follow the new description; there is no separate image mode. Jellyfin queues any item an administrator refreshes, a second time too when asked again; Polyfin starts no second refresh of a library being refreshed or waiting to be, and has nothing to refresh in a library that does not offer it.

**For app developers:** `GET /admin/api/scopes/shared/libraries` tells, for each library, `refreshable` and `refresh`, the last refresh since the server started, null for none: `state` (`queued`, `running`, `done`, `stopped` or `failed`), `titles`, `done`, `failed`, `startedAt` and `endedAt`. `POST /admin/api/scopes/shared/libraries/refresh` with `addonId`, `catalogType` and `catalogId` starts it (`202`, with the refresh; `400` `library_not_refreshable` for a library that does not offer it), and `POST /admin/api/scopes/shared/libraries/refresh/stop` with the same body stops it (`204`; `404` when none is waiting or running). Both are for administrators.

## Collections

### Collections from addons

An addon's collection catalogs become collection libraries. Each collection gathers the catalogs it groups, movies and series together.

A collection may also group other collections: a genre's movies and series, or a franchise's movies in several sagas. Opening it shows the titles of every collection within it together, up to three levels deep, as a collection of catalogs does, never those collections as folders. When the app reads them all at once, as jellyfin-web's collection page does, they come by release date once Polyfin has read them all, as Jellyfin orders a collection. Until then, and in pages, they come one of each catalog in turn. A collection that groups only catalogs keeps their order, such as their titles' popularity.

On the home page of Jellyfin apps, each collection library has a row of its collections, as a movie or series library has a row of its titles: "Recently Added in" the library's name. In jellyfin-web, a user can leave a library out of these rows under **Settings › Home**. The row's title, like the library's links in the header and the menu, opens the library on the screen chosen for it there under "Default screen": "Folders" by default, which lists its collections. jellyfin-web reads that choice for these libraries in the browser where it was saved: another browser opens "Folders" until the choice is saved there too.

Some apps list a library by its type: told a collection library holds mixed content, they show the titles of all its collections together, on their home and on the library's page. Tick such apps under **Settings › Catalogs**, in **Apps that show collection libraries by collection**: the list shows the apps that have signed in to the server, by the name they give, and the change applies to their next request. On the home of the apps chosen there, a collection library lists its collections by its own type, and a collection opens on its titles. jellyfin-web and Streamyfin leave libraries of collections out of their home rows: leave them unticked.

A collection whose catalog gives no poster shows one Polyfin draws: the posters of its first four titles, in its own order, under a gradient, with its name written across them, as its Primary image, and of its first six as its Thumb, a wide image for the apps that show wide cards (see [Generated images](#generated-images)). It comes once Polyfin has read the collection's titles, as when an app opens it or **Read every collection** runs: until then the collection shows none, and listing the collections asks the addon for nothing more. A collection whose catalog gives a poster keeps it.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Generate images for collections without one** | **Settings › Content** | On | Draws an image for the addons' collections without a poster, and for the smart lists shown as collections. Off, they show none, as before. Libraries set to **Generated** keep theirs. |

The change applies at once.

**Compared with Jellyfin:**

- A Jellyfin library of collections gets no home row. Polyfin's collection libraries are told to apps as libraries of mixed content, so that they get one, except to the apps chosen under **Apps that show collection libraries by collection**, which are told they hold collections, as Jellyfin's Collections view. Opening one shows its collections. Without an image, its tile shows the icon of a mixed library: set its image to **Generated** to show its first collections' posters (see [Library images](#library-images)).
- Jellyfin draws no image for a collection without one: apps show their placeholder.
- jellyfin-web opens a library's Suggestions from the title of its row, whatever its default screen. Polyfin's [web client script](web-client.md#dashboard-links-and-single-sign-on) opens a collection library on its default screen instead: such a library has no suggestions of its own.

**For app developers:**

- A collection library has no `CollectionType`, unlike Jellyfin's Collections view of collections made by users, which keeps `boxsets`. jellyfin-web and Streamyfin leave `boxsets` libraries out of their rows of latest items. For the apps chosen under **Apps that show collection libraries by collection**, whose `Client` in their authorization matches one of the names in any letter case, its `CollectionType` is `boxsets` wherever it is described: `/UserViews`, `/Users/{userId}/Views`, its own description, `/Items?Ids=`, `/Library/MediaFolders`, `/Library/VirtualFolders` and the listing of the root folder. Requests made with an API key never are. Its listings are the same for every app: `IncludeItemTypes=BoxSet&Recursive=true` lists its collections, and a collection with `IncludeItemTypes=Movie,Series&Recursive=true` its titles.
- `/admin/api/settings` holds the setting as `collectionLibraryApps`, a list of app names, and `GET /admin/api/settings/apps` answers the apps to choose among, `{"apps": [...]}`: the names the signed-in devices of every user give, each once whatever its letter case, by name.
- A collection without a poster gets `Primary` and `Thumb` tags in `ImageTags` once its generated images are known. `/Items/{id}/Images/Primary` serves a 1,000 × 1,500 JPEG and `/Items/{id}/Images/Thumb` a 1,920 × 1,080 one, resized as other artwork is. `/admin/api/settings` holds the setting as `generateImages`.
- `/Items/Latest` lists its collections, `BoxSet` items.
- A listing that asks for `Folder` items keeps them, as jellyfin-web lists a library without type by its folders, movies and series.
- A `Recursive` listing of a collection library that asks only for titles (no `BoxSet` or `Folder`), such as `IncludeItemTypes=Movie,Series`, lists the titles of its collections, merged one of each catalog in turn as a collection lists them, as Jellyfin lists the movies and series within a mixed library's folders. Apps that make a library's home row so show its titles. Other listings of the library list its collections. Such a listing takes the filters of what the user did, `Filters=IsPlayed`, `IsUnplayed`, `IsFavorite` or `IsResumable` and `isPlayed` or `isFavorite`, as a library of titles does: it lists the titles kept, in pages, with a `TotalRecordCount` that counts them.
- An addon's collection lists the titles of the catalogs it and the collections within it group, merged one of each in turn, never those collections. A collection that groups other collections, listed whole on its first page, is ordered by premiere date. A `Recursive` listing that asks only for titles (no `BoxSet` or `Folder`) and fits on its first page, as Streamyfin's collection pages ask, is sorted by its `sortBy`.
- A listing narrowed by `IncludeItemTypes` reads only the catalogs whose titles it keeps, by the catalog's type, movie or series: `IncludeItemTypes=Movie` lists a collection's movies, page after page, with a total that counts them alone, and `IncludeItemTypes=BoxSet` lists none, with a total of 0, as an addon's collection holds no collection. A collection library listed by its collections lists none for `Movie` or `Series` without `Recursive`, out of 0, and a movie or series library none for the other type.
- A listing of collections across the server (`IncludeItemTypes=BoxSet` without `ParentId`), as Strand asks for its shelves, lists every collection of the user's collection libraries, each once, with the collections made by users, sorted by name. jellyfin-web asks the same for its **Add to collection** dialog, where only collections made by users take a title: it gets those alone.

### Collections made by users

Users who may manage collections group titles into collections from their apps (**Add to collection** in jellyfin-web). A collection can hold movies, series, seasons, episodes, an addon's collections and Live TV channels.

Collections belong to the server:

- Once one exists, every user's apps show a **Collections** view listing them by name.
- A listing of collections across the server lists them too.
- A library's Collections tab lists the collections holding a title from it.

Each user sees only the titles they may see. Titles hidden by parental control or blocked genres do not show in a collection and do not count in its item counts or played count. Allowed hours apply as everywhere. A collection none of whose titles a user may see still shows to them, empty.

A collection:

- lists its titles by premiere date;
- takes their genres, their least restrictive rating and the earliest premiere date;
- when played as a whole, brings a series' released episodes;
- uses as poster that of the first title the user sees, as their listing tags it (an episode shows its series' poster). Without a tag, it uses its first title's poster.

Nothing else is looked up for a collection.

Rules:

- A collection needs a name.
- The titles added must be ones the user adding them may see. Otherwise the request changes nothing.
- Collections hold no people, programmes or other users' collections.
- The Collections view is not among the visible libraries an administrator chooses, as the Playlists view is not.

### Permission

On a user's page under **Users**, the **Access** section's **Can manage collections** permission is on for administrators, existing ones included, and off for other users. Without it, nobody can create a collection or add and remove its titles, administrators included. Administrators and users who may manage collections can delete a collection. See [Users](users.md).

**Compared with Jellyfin:**

- User collections are Jellyfin's BoxSets.
- A collection lists its titles, takes genres, rating and premiere date, and plays a series' released episodes as in Jellyfin.
- Jellyfin's IsLocked is kept as apps send it.
- Jellyfin hides a collection none of whose titles the user may see; Polyfin shows it empty.
- Jellyfin creates the collection and then fails (with a 500 for a missing name or an identifier that is not one); Polyfin answers 400 and changes nothing.
- **Can manage collections** is Jellyfin's `EnableCollectionManagement`, which administrators' apps also set from the user's policy.

**For app developers:**

- The **Collections** view has CollectionType `boxsets`. A listing of BoxSets across the server includes user collections, and for apps other than jellyfin-web the addons' collections too (see [Collections from addons](#collections-from-addons)).
- Hidden titles do not count in `ChildCount`, `RecursiveItemCount` or the played count.
- Without the permission, `POST /Collections` and `POST` or `DELETE /Collections/{id}/Items` answer an empty 403, as in Jellyfin, administrators included.
- Delete a collection with `DELETE /Items/{id}`, as in Jellyfin. Users without permission get 401.
- A missing name, or a title the user may not see, answers 400.

## Genre, studio and year pages

A genre, studio or year page lists the titles of the catalogs you reach, as libraries or through collections, that offer that name in their genre filter. Addons use the genre filter for genres, and some for years or studios. Titles from catalogs that cannot be filtered by that name are not on the page.

## People

The actors, directors and writers a title credits open as people, with the photo the metadata addon gives, and, while the server has a **TMDB API key**, what TMDB knows of them (see [Photos and biographies from TMDB](#photos-and-biographies-from-tmdb)). A person is one for every title that credits them by that name: a title's cast shows the photo Polyfin keeps of someone its own description gives none of, one another title's addon gave, else TMDB's, as Jellyfin shows a person's image wherever they are credited.

An addon may give them as Stremio links rather than as lists, as Stremio's protocol now advises: links of the categories `Cast`, `actor` or `actors`, `Directors` or `director`, and `Writers` or `writer`, in any letter case. Polyfin reads them in their order, as Stremio apps show them, for each kind the description gives no list of. A list the addon gives wins over its links, and Polyfin keeps no other link. A description kept from before Polyfin read links shows them once it is read again (see [Catalog pages](#catalog-pages)), or at once with **Refresh metadata**.

A person's titles are:

- those your addons' people-search catalogs (such as a catalog named People Search) find for their name; plus
- the titles of your addons that Polyfin already knows them in.

A title of the second kind that a people search also finds, under another addon's identifier (such as `tmdb:…` for the IMDb identifier a library lists), is listed once, as the title of your library. Without a people-search catalog, only the second kind show. A title's people are those of the description its page shows them from: the first addon without a language that describes it, starting with the one whose catalog listed it (see [Languages](#languages)). When that description is read again, in the background or with **Refresh metadata**, someone it no longer credits loses the title on their page, as in Jellyfin. Other addons' descriptions, and those of addons given a language, change no one's titles.

The people list holds the people credited in the titles of your addons that someone opened. People credited only in other users' own addons stay hidden.

### Photos and biographies from TMDB

Metadata addons give people's names, and some their photos; none gives a biography, and few give photos of directors and writers. While the server has a **TMDB API key** (see [Edit images](#edit-images)), Polyfin looks each person up on TMDB once, for every user:

- **What it takes:** their profile photo, for people the addon gives none of, and their biography, birth date, date of death and place of birth, which jellyfin-web shows on their page. A person whose addon gives a photo keeps it and gets the rest.
- **When:** the first time their page opens, which waits up to 3 seconds for TMDB, else shows it the next time; and when a title's page shows them in its cast without a photo, which does not wait: the photo shows the next time the cast shows. Two people are looked up at a time, at most 500 wait, and nothing is asked without a key.
- **Who they are:** the person TMDB identifies by the TMDB identifier the addon gives, when it gives one (`tmdb_id`, or a numeric `id`, in `app_extras.cast`); else the one TMDB finds by that very name, whatever the case. Of several people of the same name, Polyfin takes the one whose photo the addon gives, when it is TMDB's, else the one credited in the titles Polyfin knows the person in, matched by their TMDB identifier, or by name and year, a year apart at most. When no title tells them apart, the person gets nothing, rather than someone else's photo and life.
- **Language:** the biography is the one in the [metadata language](users.md#metadata-language) of the user who opens the page, else in the server's, each asked once; when TMDB has none in it, its English one.
- **What wins:** the addon's photo, then an image an administrator chose in **Edit images** or a field changed in **Edit metadata**, which TMDB never replaces.
- **Kept:** on the person, across restarts. A person TMDB did not find is asked about again after 30 days; a lookup that failed, after 10 minutes, then twice as long each time, up to a day.

**Compared with Jellyfin:** Jellyfin fills a person from TMDB the same way, by the TMDB identifier it has, else the first person TMDB's search finds by name, with their biography, birth and death dates, place of birth and photo in the server's metadata language ([TmdbPersonProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/People/TmdbPersonProvider.cs#L89-L144)). Polyfin asks TMDB only for people a page shows, rather than when a library scans; of several people of the same name it takes the one credited in the person's titles, or no one, rather than the first; and it shows each user the biography in their own language, else TMDB's English one, where Jellyfin keeps one per person.

## Similar titles

A movie's or series' similar titles come from the first page of the catalogs of your addons that offer one of its genres in their genre filter.

- They are ranked by shared genres, directors and actors, and release year.
- The title itself and titles you played are left out.

Trailers reach apps as remote trailers. Addons give no other extras; local folders give the extras, local trailers and theme media beside their titles (see [Extras and theme media](local-folders.md#extras-and-theme-media)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Similar titles** | **Settings › Content** | On | Lists titles close to the one open. Off, no similar titles are listed and no addon catalog is read for them. |

The change applies at once.

**For app developers:**

- With **Similar titles** off, every Similar route answers an empty list. An unknown title is still not found.

## Ratings

With a server key from MDBList, movies and series show three ratings from MDBList: their IMDb rating, their critics' score and their audience score.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **MDBList API key for ratings** | **Settings › Content** | Empty | The server's own MDBList key, checked with MDBList when saved. Empty, nothing changes and MDBList is never asked. Users' own MDBList keys for tracking are never used for this. |
| **Critic rating shows** | **Settings › Content** | Critics' score | Which score `CriticRating` carries: the critics' score or the audience score. |

- `CommunityRating` is the IMDb rating from MDBList, else the addon's, from 0 to 10. `CriticRating` is the score chosen above, from 0 to 100. The web client shows all three on a title's page (see [the web client](web-client.md)), and on posters while **Show ratings on posters** is on (see [Ratings on posters](web-client.md#ratings-on-posters)).
- An administrator's edit still wins (see [Edit metadata](#edit-metadata)). An edit that leaves the ratings as shown keeps following MDBList.
- Polyfin asks MDBList when a title's page opens, or when a movie or series whose ratings it does not keep is listed, in the background, up to 100 titles a request. Listings and title pages never wait: they show what is kept, and the next listing shows the rest. Titles without an IMDb or TMDB identifier, episodes, Live TV and music are not asked about.
- Ratings are kept in the database by identifier, for every user, and asked again after 7 days, or 30 days for a title released over a year before. A title MDBList does not know is asked about again after 30 days. **Refresh metadata** forgets a title's ratings.
- The votes the IMDb rating counts are kept with it, for smart lists' **IMDb votes** rule (see [Smart lists](#smart-lists)). The ratings kept before Polyfin read votes are asked about again as their titles are next listed.
- MDBList allows 1,000 requests a day on its free tier. On a `429`, Polyfin asks nothing more for as long as `Retry-After` says, and after `Daily API limit exceeded!` until MDBList's daily reset. **System › Health** shows the requests left today.

## Artwork

Polyfin relays images from the addons' artwork servers, IPTV channel logos included, so Jellyfin apps only ever talk to Polyfin.

- Polyfin downloads an image once, however many apps ask for it at once, and asks one artwork server for at most 16 images at a time. It asks for at most 4 at a time from the servers of IPTV sources and of their logos, often the provider's panel, and for 10 minutes from a server that reset a connection or answered 429, 502, 503 or 504.
- It keeps the images it relayed in memory (128 MB) and in the `images` folder of Polyfin's cache folder (1 GB), the least recently used going first. They are not downloaded again after a restart. **Clear the image cache**, in **System › Health**, empties both (see [Clearing the image cache](administration.md#clearing-the-image-cache)).
- An image that could not be downloaded is not asked for again for 2 minutes. Meanwhile, its requests answer the same. With **Detailed log** on under **Settings › Diagnostics**, each of them logs `Artwork could not be downloaded` with the first download's cause: `the last download failed: ` followed by its error, such as `addon unreachable: HTTP 500`.
- An image its artwork server does not have (`404` or `410`) answers `404`, as Jellyfin answers for an image an item does not have, and apps show their placeholder. An artwork server that fails otherwise (no answer, a `5xx`) makes it answer `502`.
- An app asking for an image much smaller than the original (`maxWidth`, `maxHeight`, `fillWidth`, `fillHeight`, `width` or `height`) gets it resized, as a JPEG, or a PNG when it was one. Sizes are rounded up to a few steps, which are kept too. Polyfin resizes as many images at once as half its processors, 2 at least.
- An image asked for with its `tag` may be kept by the app for good: the tag changes with the image.

Seasons show their own poster when the metadata addon lists the seasons' posters, as Jellyfin shows a season's own image. A season without one shows its series' poster.

That list does not name the seasons. So Polyfin reads it only when it has one poster per season of the series' episodes, with specials included or left out. Otherwise it shows the series' poster on every season, rather than risk showing one season's poster on another. Polyfin asks the addon for nothing more: the posters come with the series' description.

### Poster shapes

An addon may declare the shape of a title's or collection's poster, in its catalogs and descriptions, as Stremio's addon protocol defines `posterShape`: `poster` (2 by 3, also when it declares none), `landscape` (wide, about 16 by 9) or `square`. Polyfin tells apps that shape: a wide poster gets a `PrimaryImageAspectRatio` of 16:9 and a square one 1:1. Other posters keep 2:3.

- A library, collection, smart list, search result or home row gives the shape the catalog declares; a title's page gives the one of the description it shows. A poster in the user's [metadata language](users.md#metadata-language) comes with the shape its description declares.
- An image in place of the addon's poster has no shape of the addon's and shows as 2:3: one uploaded or downloaded with [Edit images](#edit-images), a local folder's poster, and the poster Polyfin draws for a collection without one.
- Seasons and episodes keep their own shapes, 2:3 and 16:9.

jellyfin-web shows a title's or collection's page with a poster of its shape ([cardImage.ts, lines 15 to 31](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/cardbuilder/cardImage.ts#L15-L31)). A row or grid takes one shape for all its cards, from the median ratio of its items ([imageLoader.js, lines 178 to 234](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/images/imageLoader.js#L178-L234), [cardBuilder.js, lines 81 to 105](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/cardbuilder/cardBuilder.js#L81-L105)): a catalog of wide posters shows wide cards, while one that mixes shapes shows all its cards in the shape nearest that median.

**Compared with Jellyfin:** Jellyfin measures a Primary image kept on its disk, and gives a remote one the shape of the item's type ([DtoService.cs, lines 1849 to 1879](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Dto/DtoService.cs#L1849-L1879)): 2:3 for movies, series and collections ([Movie.cs, lines 48 to 57](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/Movies/Movie.cs#L48-L57), [Series.cs, lines 71 to 77](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/TV/Series.cs#L71-L77), [BoxSet.cs, lines 78 to 79](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/Movies/BoxSet.cs#L78-L79)). Polyfin measures no image: it lists the addons' artwork without downloading it, so it takes the shape the addon declares, else that of the type, as Jellyfin does for a remote image. An uploaded poster shows as 2:3 whatever its size, where Jellyfin would measure it.

### Posters warmed ahead

The first time a poster shows, it waits for the artwork server, then for its resize. So every hour, once it has read the libraries' first pages, Polyfin downloads their posters and resizes them to the sizes jellyfin-web asks for its cards: 240, 400 and 480 pixels wide for a poster of 2 by 3. A home row or a library then shows them from Polyfin's cache the first time too, and its next posters on scrolling as well.

- Polyfin warms the first 100 titles of each library that some user sees, as each user sees it: a server library hidden from every user is left out, and so is a title that every user who sees its library hides, or that parental control or blocked genres hide from them. A title whose rating or genres Polyfin does not know yet waits for the next hour.
- A user's own libraries are warmed only once the user opened them in an app, and for a week after. A home row does not open its library. Polyfin forgets the libraries opened when it restarts.
- Music libraries and smart lists are left out, and so are the images Polyfin draws or an administrator uploaded, and the posters a user's metadata language shows instead of the server's. A local folder's posters are warmed as the addons' are.
- Warming downloads at most a quarter of the images Polyfin asks one artwork server for at a time: 4, and 1 from a server of an IPTV source or one Polyfin spares. It resizes at most half as many images at once as apps' requests do. The rest stays for apps.
- What it keeps goes in the `images` folder only, never in memory, and fills it up to 90% of its bound: warming deletes no image for room, and stops once the folder is that full. The images it warmed go first when apps' images fill the folder, until an app asks for them. Without a cache folder, nothing is warmed.
- A poster warmed is not downloaded again while the folder keeps it.

Jellyfin downloads a title's images when it scans its library, and resizes them the first time an app asks. Polyfin has no scan of the addons' catalogs: it warms what apps show first, and resizes it ahead too.

### Badges on posters

While **Badges on posters**, under **Settings › Content › Posters**, is on, Polyfin draws badges over the Primary image of every movie and series. Every app shows them, Jellyfin's and Silo's alike, as they are part of the image.

- At the top left, one under the other: the star rating, the one the title's page shows, with "IMDb" in place of the star when it is the IMDb rating MDBList gave; then, while the server has an **MDBList API key for ratings** (see [Ratings](#ratings)), the critics' score, with a speech bubble, and the audience score, with a group of people, in percent. A rating the title lacks is left out.
- At the top right, for a series: a play mark when an episode came out in the last 30 days, a stack when a season other than the first started in them. Specials, season 0, count for neither, as they are often extras. Polyfin reads them from the series' descriptions it keeps, whichever addon gave them, never asking an addon: a series nobody opened yet shows none. What it read is kept for 10 minutes.
- Badges hold icons and numbers only, written in Go Bold, built into Polyfin, so they read the same in every language.
- **Badge size** sets how large they are for the poster's width: **Small**, **Medium** (the default) or **Large**. On a small card they keep a least size, and only as many as fit in the poster's upper half are drawn.
- **Preview** shows titles of your libraries as apps show them, as they are, with badges, and on a small card, at the size chosen before it is saved, whether the badges are on or not. **Another title** goes to the next one, those with something to show first.
- A title with nothing to show, an image an administrator uploaded, and the images Polyfin draws keep their image as it is.
- The image keeps its address. Its tag, which apps send to ask for it, is the image's own followed by what the badges show and Polyfin's signature: when a rating changes, the tag changes and apps load the image again once they list the title again. jellyfin-web keeps the listings it read in the browser and shows them first, so a change shows there once it reads them again. A tag Polyfin did not sign draws nothing.
- The image as the addon gives it stays in the image cache, and each one with badges is kept beside it, at each width asked for, under the same bound.
- Turned off, every title gets its image and its tag back. An app that asks with a badged tag still gets the image without badges, which it keeps for a week at most.
- The badges follow the listings: a title a user may not see, through parental control, blocked genres, a hidden library or a hidden title, is not listed to them, badges or not.
- While it is on, the web client shows no ratings on hover (see [Ratings on posters](web-client.md#ratings-on-posters)).

**Compared with Jellyfin:**

- Jellyfin draws nothing over images; its apps show the ratings their own way, or not at all. Polyfin's badges are a choice of its own, off by default.

### Poster address template

**Poster address template**, under **Settings › Content › Posters**, is an address movies' and series' posters are downloaded from in place of the addons' ones, such as `https://posters.example/{type}/{imdb_id}.jpg`. `{imdb_id}`, `{tmdb_id}`, `{type}`, `movie` or `series`, and `{tmdb_type}`, TMDB's `movie` or `tv`, are filled in; the address names `{imdb_id}` or `{tmdb_id}`.

- A title lacking an identifier the address names keeps its addon's poster. When its listing lacks it, Polyfin looks in its description kept, so a title's page may show the template's poster while a listing shows the addon's.
- A poster the address cannot give, for any reason, is replaced by the addon's, which apps keep for a week at most so that the template's comes once it can.
- A local folder's poster and an uploaded one still come first. Seasons and episodes keep their own images.
- The address is the administrator's: it may point to a server of the local network.
- Empty, the default, the addons' posters show, with their tags.

**Compared with Jellyfin:**

- Jellyfin takes posters from its metadata providers and plugins. Polyfin takes them from the addons, and from this one address when it is set.

### Generated images

Polyfin draws the images of the addons' collections without a poster, of the smart lists shown as collections, and of the libraries whose image is set to **Generated** (see [Library images](#library-images) and [Collections from addons](#collections-from-addons)).

- The image is a mosaic of the posters of the first titles the item lists, in its own order: four for a poster, six for a wide image. A gradient darkens it toward the bottom, and the item's name is written across the middle. A name too long for three lines, two on a wide image, is cut with an ellipsis.
- The name is written with the fonts of `POLYFIN_FONTS_DIR`, as the fallback fonts of subtitles are (see [Configuration](configuration.md)): DejaVu in the Docker image, for Latin, Greek and Cyrillic. Each character takes the first font that has it, bold sans-serif fonts first. Mount fonts for other scripts in that folder, for example CJK fonts. A character none of them has is written with Go Bold, built into Polyfin, else left out. Letters are drawn one by one: scripts whose letters join or that are written right to left do not show as they should.
- Polyfin chooses the titles from the catalog pages and descriptions it keeps, and asks no addon for them: an item gets its image's tag once they are known. It draws the image the first time an app asks for it, from the posters it relays, and keeps it with the relayed artwork, in the cache folder's `images` folder: 1,000 × 1,500 pixels for a poster, 1,920 × 1,080 for a wide image. Apps asking for a smaller size get it resized, as other artwork is.
- A poster its artwork server does not have is left out. One that cannot be downloaded for now is left out of the image served, which is drawn again at the next request.
- The titles shown stay while the item keeps its name and fewer than half of them, as a poster or as a wide image, have left its first 20 titles. Past that, when the item is renamed, with **Edit metadata** too, or when it shows fewer titles than it could, Polyfin chooses them again: the image's tag changes, and apps that keep images by tag fetch the new one. Polyfin compares them at most once a minute for each item.
- Users under parental control or blocking genres get no generated image, since it may show a title hidden from them, and neither do users who hid one of the titles it shows.
- An image uploaded with **Edit images** replaces the generated one of its type; deleting it brings the generated one back.

## Editing items

Administrators can use jellyfin-web's **Edit metadata**, **Edit images** and **Identify** on the items apps show: titles, seasons, episodes, people, channels, music, libraries, views, collections and playlists.

### Edit metadata

Polyfin keeps your changes apart from what the addons say. Your changes win for every user, wherever the item shows:

- details and listings;
- search: a renamed title is also found by its new name;
- parental control, which judges a title by the custom rating set, else the official rating, and by the genres set, as Jellyfin does.

A series' name and ratings show on its seasons and episodes.

Polyfin keeps these fields: name, original title, sort name, overview, tagline, genres, tags, studios, official and custom ratings, community and critic ratings, premiere and end dates, and production year.

- A field left empty, or set back to the addon's value, follows the addon again, so the addon's later changes show.
- People, provider identifiers and the editor's other fields (locks, display order, content type, date added) are not kept: they stay the addons'. An IMDb identifier typed for a movie or show of a local folder links its files to that title instead (see [Identify in the web player](local-folders.md#identify-in-the-web-player)).

### Edit images

**Edit images** uploads a Primary, Backdrop, Logo, Thumb or Banner image in place of the addon's, or searches two image providers for one:

- **TMDB**, while the server has a **TMDB API key**: a movie's or series' posters, backdrops and logos, an episode's stills, a season's posters and a person's photos, with their language, size and votes. A title is found by the TMDB identifier its addons give, else through TMDB's lookup of its IMDb identifier; a person by name, the most popular of that very name. The same key fills people's photos and biographies (see [Photos and biographies from TMDB](#photos-and-biographies-from-tmdb)), and matches a local folder's files when every metadata addon fails to search them (see [Through TMDB, when every addon fails](local-folders.md#through-tmdb-when-every-addon-fails)).
- **Addons**: the poster, background, logo and wide poster each of the server's metadata addons gives in its description of a movie or series, shown under the addon's name. An addon that lists the title for a user is asked too.

Posters are listed for Primary, backdrops for Backdrop and Thumb (with the addons' wide posters first), logos for Logo. Banner lists nothing. TMDB's images follow the language of the user who searches: their [metadata language](users.md#metadata-language), else the server's. Unless jellyfin-web's All languages box is ticked, they are those in that language and those without text; with it, every language, that language first, then by votes. TMDB's logos given as SVG files are left out, since they cannot be kept.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **TMDB API key** | **Settings › Content** | Empty | The server's TMDB API key or read access token, checked with TMDB when saved. Edit images searches TMDB with it, people's photos and biographies are looked up with it, local folders' files are matched through TMDB's search when every metadata addon's fails, and smart lists' **Studio or network** rules look titles' studios and networks up with it. Empty, only the addons' images are listed, people show what the addons give, such files stay unmatched, studio rules see only those an administrator set, and TMDB is never asked. |

- Images are relayed by Polyfin, as all artwork is: apps never reach TMDB or an addon's artwork server. Polyfin relays only the images it listed for the item, and an addon's under the same limits as the addon: a user's own addon reaches public addresses only.
- TMDB's answers are kept for 24 hours, and at most 4 requests go to TMDB at a time, its images included.
- Downloading an image, for administrators only, keeps it as an upload is kept, below. An address Polyfin did not list for the item and image type is refused (400), so the server is never sent to an arbitrary address.

An uploaded or downloaded image:

- must be a JPEG, PNG or WebP picture of up to 10 MB;
- is scaled down to 3,840 pixels and kept in the database, as profile pictures are;
- wins over the image Polyfin draws (see [Generated images](#generated-images)).

Other image types, and anything that is not such a picture, are refused. Deleting one shows the addon's image again, or for a collection without a poster the image Polyfin draws. The addon's own artwork cannot be deleted.

This product uses the TMDB API but is not endorsed or certified by TMDB.

### Identify

**Identify** looks movies and series up by the IMDb, TMDB or TVDB identifier typed, then by name, in the server's addons that describe titles, TMDB turning TMDB and TVDB identifiers into IMDb ones with the server's **TMDB API key**. It finds nothing for other types. Applying a result to a title only addons list fails, because an item is its addon's title and cannot be bound to another. Applying it to a title of a local folder, a plain video or a title its files were matched to, links the file, or its whole show folder, to the title chosen, as **Find the title** does on the folder's page; an IMDb identifier typed in **Edit metadata** for such a movie or show does the same (see [Identify in the web player](local-folders.md#identify-in-the-web-player)).

**Compared with Jellyfin:**

- Jellyfin keeps any uploaded image file as sent; Polyfin refuses other types with 400.
- Applying an **Identify** result to a title only addons list fails as a failed identification does in Jellyfin (500). Applied to a title of a local folder, it answers 204 as Jellyfin does.
- Jellyfin picks suggestions among everything; Polyfin picks among the first page of each library.

**For app developers:**

- `/Items/Root` is the user's root folder; its children are the user's views.
- `/Items/Counts` counts each library's first page of titles, plus one when more follow, as genre and studio pages count, as far as Polyfin keeps them: it asks the addons nothing. The user's favorites are counted exactly with `isFavorite=true`.
- `/Items/Suggestions` returns titles picked at random among the first page of each library, those the user's taste profile ranks first coming first once it is confident (see [Top picks for you](web-client.md#top-picks-for-you)).
