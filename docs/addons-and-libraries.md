<a id="addons-and-libraries"></a>

# Libraries

This page is for administrators who turn addon catalogs into Jellyfin libraries. It covers what catalogs become, how libraries are ordered and narrowed, how Polyfin reads and refreshes catalogs, and the genre and similar titles pages.

Stremio catalogs become Jellyfin libraries. Stremio streams become versions of the same item (see [Playback](playback.md)). Addon subtitles become external subtitle tracks, and an app's subtitle search lists the subtitles from the user's subtitle addons (see [Subtitles](subtitles.md)).

The rest of the old "Addons and libraries" page is now on:

- [Addons](addons.md): adding addons, who uses them (from the source's page or a group's), identifiers, failures, request limits and languages;
- [Smart lists and search](smart-lists-and-search.md): smart lists, searches in plain words and movies waiting for their digital release;
- [Music, audiobooks and podcasts](music.md): music addons and music folders;
- [Collections, people and ratings](collections-people-and-ratings.md): collections, people, ratings and the TMDB key;
- [Artwork and editing](artwork-and-editing.md): library images, posters, artwork and editing items.

API details: [Versions as media sources](jellyfin-api.md#versions-as-media-sources).

## The order of libraries

Jellyfin apps list the server's libraries in the order of **Content › Libraries**. To move a library:

- **Drag** its row by the grip at its start, with the mouse or a finger. A line shows where it will land. A long list scrolls while the row is held near its edge. Escape, during a drag, leaves the library where it was.
- **Arrows:** use the up and down arrows of the row, from the keyboard too. Screen readers announce each move.

Save the new order with the list's **Save**, as for the libraries' names. Until then, apps keep the order saved last.

Smart lists shown as libraries, and users' own libraries, come after the server's libraries. Users order their own libraries under **My sources** the same way.

### Order chosen by a user

Each user can order all of their libraries in the web player's **Settings › Home**, with the arrows of its list, then **Save**:

- **It wins:** an order changed there is the user's own, and wins over the administrator's, as in Jellyfin.
- **New libraries:** a library added later, or removed and added again, comes after the ones the user ordered.
- **Unchanged orders:** jellyfin-web saves the order of every library each time that page is saved, even when only a home section changed. An order saved as shown, the server's own, is not kept as the user's. The user goes on following **Content › Libraries**, libraries added later included.
- **Going back:** the user moves the libraries back to the administrator's order there. Or they choose **Reset my preferences** in **My account**, which also resets their other app preferences (see [Your own preferences](users.md#your-own-preferences)).

**Apply this order to every user**, under the list of **Content › Libraries**, forgets the orders users saved. Every user's apps then list the libraries in the saved order of **Content › Libraries**, so save the list first.

- It asks first, and says how many users ordered their libraries themselves.
- It keeps the rest of their settings. Users can order their libraries again afterwards.
- It is for administrators only. The activity log records who did it: "alex applied the order of the libraries to every user (2 with an order of their own)".

jellyfin-web keeps the list of libraries for up to a minute, across a reload too (see [Dashboard links and single sign-on](web-client.md#dashboard-links-and-single-sign-on)). So a new order shows there within a minute. A user's own change in the web player's **Settings › Home** shows at once.

**Compared with Jellyfin**

- Polyfin has an order the administrator sets for every user, which Jellyfin does not. Jellyfin lists a user's libraries in their order from the web player's **Settings › Home**, then by name ([`UserViewManager.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/UserViewManager.cs#L140-L158), Jellyfin 12.4). In Polyfin, the order of **Content › Libraries** takes the place of the order by name.
- Polyfin keeps a user on the administrator's order when they saved it unchanged. jellyfin-web 12.4 saves every library's place in `OrderedViews` whenever the web player's **Settings › Home** is saved ([`homeScreenSettings.js`](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/homeScreenSettings/homeScreenSettings.js#L498-L506)). Jellyfin keeps it as posted, and answers it back in the user's `Configuration`. Polyfin stores an order identical to the server's as empty, `"OrderedViews": []`.
- Polyfin can apply one order to every user. Jellyfin has no such action.

## What catalogs list

An addon gives each catalog, and each title, a Stremio type. The type decides what the titles are in Jellyfin apps:

| Stremio type | Titles in Jellyfin apps | Library type |
| --- | --- | --- |
| `movie`, `anime.movie` | Movies; or each title as its own type tells (see [Show as](#show-as)) | `movies`; none (mixed) when shown as movies and shows |
| `series`, `anime.series` | Shows, with their seasons and episodes; or each title as its own type tells | `tvshows`; none (mixed) when shown as movies and shows |
| `anime` | Shows, or movies for its `anime.movie` titles | none (mixed) |
| `other`, `channel` | Folders of videos, or videos; or shows and movies (see [Show as](#show-as)) | `homevideos`; none (mixed) when shown as movies and shows |
| `collection` | Collections (see [Collections from addons](collections-people-and-ratings.md#collections-from-addons)) | none (mixed) |
| `tv` | Live TV channels, not a library (see [Live TV](live-tv.md)) | |
| `track`, `album`, `artist`, `playlist` of a music addon | Music (see [Music addons](music.md#music-addons)) | `music` |
| Any other type, such as `sport` or `events` | Folders of videos, or videos, as for `other`; or shows and movies, as when its titles are typed movies and series | `homevideos`; none (mixed) when shown as movies and shows |

- **Other types:** a type Polyfin has no particular use for is listed as `other` is, as Stremio apps do. A sports addon's `sport` catalogs, for instance, become libraries of matches and recaps. Each is a video, or a folder when its details list videos.
- **No type:** a title without a type, in a catalog without one, is left out.

A library of several kinds, such as an `anime` library or one shown as movies and shows, lists and counts only the kinds an app asks for:

- It reads the catalog's first 500 titles, or its **Maximum titles** when lower, and keeps the kinds asked for.
- It answers them a page at a time, with a count of those kinds alone, so that apps page through them to the end.
- A catalog longer than 500 titles counts its first 500.
- A title whose details are not known after 5 seconds has no kind yet. It is left out until they are, and the listing says that more may follow.

API details: [Libraries of mixed kinds](jellyfin-api.md#libraries-of-mixed-kinds-for-apps).

### Show as

**Show as** chooses how a library of an addon's catalog shows its titles. It is in the library's row under **Content › Libraries**, saved with the list's **Save**. The row then says what it shows, such as **Shown as movies and shows**. Users choose the same for their own libraries under **My sources**.

| Catalog | Choices | Unless chosen |
| --- | --- | --- |
| `other`, `channel` and any type Polyfin has no particular use for | **Home videos** or **Movies and shows** | **Movies and shows** when the catalog's first titles are all movies and series, else **Home videos** |
| `movie`, `anime.movie`, `series`, `anime.series` | **As listed** or **Movies and shows** | **As listed** |
| `anime`, `collection`, `tv`, a music addon's, a local folder's, an IPTV source's | None | |

What each choice shows:

- **Home videos:** the titles as folders of their videos, or as videos (see [Folders of videos](#folders-of-videos)).
- **Movies and shows, for other videos:** the titles as the movies and shows their videos make (see [Other videos as movies and shows](#other-videos-as-movies-and-shows)). Titles the catalog already types as movies or series stay what they are.
- **As listed:** a movie catalog as a movies library, a series catalog as a shows library. A title of the other kind, such as a series a movie catalog also lists, is still that series when opened, searched or played. But no tab of the library shows it: jellyfin-web asks a movies library for movies only. The count of its movies includes the series.
- **Movies and shows, for movies or series:** the library has no type, as Jellyfin's libraries of mixed movies and shows. Each title shows as its own type tells. jellyfin-web's **Folders** tab and its tab of movies and shows list a movie catalog's series with its movies. Apps that ask for movies or series alone get each kind with its own count (see [What catalogs list](#what-catalogs-list)).

The titles are the same items in both choices. Switching changes nothing of what users watched, rated or hid.

#### When no one chose

Some addons list movies and shows under a type of their own, each title typed `movie` or `series`. A home videos library would show none of them: jellyfin-web asks it for folders, videos and photos only. Its **Folders** tab listed none of their titles, with a count of all of them.

So a library of other videos shows its titles as movies and shows, when no one chose, if the first page of its catalog lists only titles typed `movie`, `anime.movie`, `series`, `anime.series` or `anime`.

- **When Polyfin decides:** once, from the first page it reads. That is when the library is saved. A library made earlier is decided when Polyfin reads the libraries' first pages, at start and then every hour.
- **What it keeps:** the choice, which the row shows and an administrator may change.
- **A first page it cannot read:** the library stays home videos until it can.
- **Titles further in:** its movies and shows are the same items either way. A title of another type that the library listed as a home video, further in the catalog, becomes a show or a movie. Its users' played marks, resume points and favorites follow it, as when an administrator switches.

Polyfin decides once, rather than at each listing, so that a library keeps its type. Apps keep the type a library had when they listed it, and a catalog whose first page changed would change it under them.

#### Choices not offered

- **Movies only, or shows only, for other videos:** a season's download would become many unrelated movies, or a movie's download a show of one episode. **Movies and shows** already makes each title the movie or show its videos make.
- **Turning movies into series, or the reverse:** addons describe and stream a title by its type. A title typed `movie`, asked as a series, finds nothing, unless another addon describes it so. A title typed wrongly is an error in its addon, to report to its author. **Movies and shows** lists each title as its own type tells.
- **Each title as a collection:** it would lose its seasons and Next Up. Apps that leave collection libraries out of their home screen would leave it out.
- **Music, books, photos or music videos for a video catalog:** such libraries list audio, books, images, or artists and albums. A video catalog has none, so their views would stay empty.
- **Other catalogs:** `anime` libraries are mixed already. Local folders choose what they hold under **It holds** (see [Changing what a folder holds](local-folders.md#changing-what-a-folder-holds)). Collection, live TV, music and IPTV catalogs have their own settings.

**Compared with Jellyfin**

- Polyfin can change a library's type; Jellyfin never does once the library is made. Jellyfin writes the type as an empty `<type>.collection` file in the library's folder when it is added ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3807-L3849)), and reads it back from there ([L1674-L1687](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L1674-L1687)).
- Of Jellyfin's library endpoints ([LibraryStructureController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/LibraryStructureController.cs#L89-L107)), only the one that adds a library takes a type. Renaming, the media paths and the library options do not, and jellyfin-web's library editor has no field for it. A type changes by removing the library and adding it again. **Show as** goes further, as **It holds** does for local folders.
- A library shown as movies and shows is like a mixed library jellyfin-web makes: one made without a type ([mediaLibraryCreator.js](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/mediaLibraryCreator/mediaLibraryCreator.js#L61-L63)).

API details: [Show as](jellyfin-api.md#show-as-for-apps).

### Folders of videos

`other` is Stremio's type for what is neither a movie nor a series. An example is a cloud store, whose addon lists each download as a title and its files as the title's videos. `channel` is a channel's uploads, in the same shape. So is every type Polyfin has no particular use for, such as `sport`. Polyfin lists them as Stremio apps do:

- **Titles with videos:** a title whose details list videos is a folder. Apps open it to list its videos, in the addon's order. Each is named after its video, or "Video 3" without a name, with its date and image when the addon gives them.
- **Titles without videos:** such a title is a video itself. It plays the streams the addon lists for the title.
- **Unknown titles:** catalogs do not say which titles have videos; their details do. Listing a library asks the addon for the details of the titles it does not know yet, a few at a time, for up to 5 seconds. A title still unknown then is listed as a folder. If it turns out to have no videos, the folder lists itself as its one video, and the title is listed as it is from then on.

What the library does:

- **Its views:** it is a home videos library, as Jellyfin's for videos that are neither movies nor shows. jellyfin-web shows its **Folders** view, the titles, and its **Videos** view. **Videos** lists every video through the folders, for the first 500 titles, or the **Maximum titles** when lower (see [Genre and maximum](#genre-and-maximum)). It can show them as movies and shows instead (see [Other videos as movies and shows](#other-videos-as-movies-and-shows)).
- **Watch state:** a folder shows how many of its videos are left to watch, and is played once all are. Marking it played or unplayed marks its videos. Removing it from Continue Watching removes their resume points.
- **Playback:** videos play as movies do: directly, repackaged or converted, with their resume point and in Continue Watching. A video whose stream has no end, such as a match on now, plays live (see [Live videos](versions.md#live-videos)).
- **Tracking:** the tracking services hear nothing of them.
- **Search:** it finds them as folders or videos, in the catalogs that offer search.
- **Parental control:** it judges them as **Other**, unrated unless their details give a rating, and a video by its folder. Blocked genres hide them by their details' genres.
- **Hiding:** hiding a folder, or one of its videos, hides the folder with its videos.
- **Collections:** a folder can be added to a collection.

**Compared with Jellyfin**

- A folder lists its videos in the addon's order, whatever sort the app asks. Jellyfin sorts a folder's children as asked, and jellyfin-web asks a folder's page for them by name. The addon's order is that of the files or uploads it lists, which their names do not always keep. A listing through the folders that names item types, as jellyfin-web's **Videos** view, is sorted as asked.

API details: [Folders of videos](jellyfin-api.md#folders-of-videos-for-apps).

<a id="shown-as-movies-and-shows"></a>

### Other videos as movies and shows

Some Jellyfin apps list only movies and shows. They leave out every library that is not of movies, shows, collections or mixed, and ask libraries for movies and series only. Others open a folder as a show and ask for its episodes. **Show as** **Movies and shows**, in the row of a library of other videos, shows its titles as such apps expect (see [Show as](#show-as)).

- **The library** has no content type, as Jellyfin's libraries of mixed movies and shows, so these apps keep it. jellyfin-web lists its titles in a plain list.
- **Shows:** a title whose details list two videos or more is a show. So is a single video numbered as an episode (`S35E01`, a show of one episode).
- **Episodes:** a show's videos are its episodes, in the seasons and with the numbers the addon gives them. A video with neither is in season 1. A video without a number comes after its season's numbered episodes, in the addon's order. A video of season 0 with a number is a special. Addons of a store's downloads number the videos from their file names, when these tell one.
- **Movies:** a title without videos is a movie, which plays the streams the addon lists for the title. So is a title of one video that its details number as no episode, a movie's file. It plays that video's streams, as Stremio apps play a title's only video.
- **Downloads:** a store's download of one movie is a movie, a download of one episode a show, and a season pack a show of its episodes.
- **Unknown titles:** listing the library asks the addon for the details of titles it does not know yet, for up to 5 seconds. A title still unknown then is listed as a show. It is a movie from then on, if its details make it one.

How they behave:

- **Playback:** episodes and movies play as others do. They play the streams the title's details give the video (see [Streams in title details](#streams-in-title-details)), then those addons list for it: directly, repackaged or converted, with seeking. A video whose stream has no end plays live.
- **Watch state:** watch state, resume points, Continue Watching, Next Up, favorites and played marks of a whole show or season work as for other movies and shows. So do search and the filters by type and watch state.
- **Parental control:** it judges them as the movies and shows they are shown as, unrated unless their details give a rating. A user who blocks unrated movies or shows does not see them. Blocked genres and hidden titles apply as for folders.
- **Tracking:** they have no IMDb, TMDB or TVDB identifier. The tracking services hear nothing of them: no scrobble, played mark, resume point or rating, as for every title no service can identify. Imports from tracking services and other servers do not match them.
- **Details:** they come from the addon only. TMDB is never asked about them.

What happens when the choice changes:

- A title is one item whether it is a show or a movie. It stays the same when its details gain or lose videos.
- Its items are not those of home videos. Switching between **Home videos** and **Movies and shows**, or turning the library off and on again, moves each user's played marks, favorites and resume points to the new items. A movie's file moves to the movie, and back. It works as when a local folder changes what it holds (see [Changing what a folder holds](local-folders.md#changing-what-a-folder-holds)).
- The former items are no longer found. Apps that kept them, in a list open since, open nothing.
- Ratings, administrators' edits and uploaded images, playlists and collections stay with the former items.
- A title the catalog types `movie` or `series` is not one of these. It stays the movie or series it is, the same item either way.

**Compared with Jellyfin**

- Polyfin can show other videos as movies and shows; Jellyfin's home videos stay folders and videos. A library made without a content type, jellyfin-web's mixed movies and shows, has no `CollectionType` ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3844-L3849)), as this one.
- A folder shown as a show answers as a show. A folder of a home videos library has no seasons or episodes in Jellyfin ([TvShowsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L261-L264)), nor in Polyfin.

API details: [Other videos as movies and shows](jellyfin-api.md#other-videos-as-movies-and-shows-for-apps).

<a id="streams-in-a-title-s-details"></a><a id="streams-in-a-titles-details"></a>

### Streams in title details

A video, or an episode of a series, may carry its own streams in the title's details (`videos[].streams`). Stremio apps play them without asking any addon for streams. Polyfin lists them as the video's first versions, then the streams of the other addons.

- They get their names, file names and sizes as other versions do.
- The addon that gave them is not asked for the video's streams. Such an addon often answers that request with a notice alone, with nothing to play.
- A stream whose link expires is renewed by asking the addon for the title's details again.

## Genre and maximum

**Options**, on a library's row under **Content › Libraries**, opens its settings under the row, **Genre** and **Maximum titles** among them. They are saved with the list's **Save**, in the bar of unsaved changes. The row shows a library's genre and maximum at a glance. Users set the same for their own libraries under **My sources**.

They apply to catalogs of titles and of collections. Live TV catalogs and music libraries have neither.

| Setting | What it does |
|---|---|
| **Genre** | Lists only that genre's titles, as the addon narrows them. The choices are the genres the catalog offers in its genre filter. |
| **Maximum titles** | The most titles the library lists, from 1 to 20,000, and as many in each of its collections. Empty, **Titles read per movie and series catalog** applies. |

**Genre:**

- Without a name of its own, apps name the library after its catalog and genre, such as "Popular · Comedy".
- The choice shows only for a catalog that offers genres to choose from. It does not show for one that requires a single genre, as many collection catalogs do.
- A genre the addon stops offering lists the whole catalog again, until another is chosen.

A library narrowed to a genre:

- offers that genre alone in apps' genre filters, and lists nothing for another;
- adds its titles to that genre's page only (see [Genre, studio and year pages](#genre-studio-and-year-pages));
- finds its automatic image among that genre's titles.

**Maximum titles:**

- It replaces **Titles read per movie and series catalog** for this library, lower or higher (see [Catalog limits and refresh](#catalog-limits-and-refresh)).
- Polyfin reads no page of the catalog past it.
- A library that leaves out the titles local folders hold reads as many more as it leaves out (see [Titles local folders hold](#titles-local-folders-hold)).

**Compared with Jellyfin**

- A library's genre and maximum are Polyfin's own. Jellyfin libraries hold the files of their folders.
- Polyfin lists up to 500 titles when an app sets no limit, as catalogs can be nearly endless. Jellyfin lists every item of a folder then.

API details: [Library listings without a limit](jellyfin-api.md#library-listings-without-a-limit).

## Libraries hidden from the top bar

**Hide from the web player's top bar**, in a library's row under **Content › Libraries**, leaves the library out of the web player's menus. Those are the buttons of its top bar, the bar's **More** menu, and its side menu, in both of its layouts. It is saved with the list's **Save**, and the row then says **Hidden from the top bar**. Users set the same for their own libraries under **My sources**.

- **Live TV catalogs** have no such choice: their channels go to Live TV, never to a menu.
- **Only the web player:** jellyfin-web, at `/web/`, hides it. There, the library keeps its home screen row, and its pages still open from it. Other Jellyfin apps keep listing it.
- **The More button:** it goes too, when every library it would list is hidden at the window's width.
- **When it applies:** the next time the web player loads, or a user signs in to it. After a user's first load in a browser, a reload does not show the hidden links, even briefly: the browser keeps the last choice.

**Compared with Jellyfin**

- Polyfin keeps the home row of a hidden library. Jellyfin has no such choice: a library in a user's `MyMediaExcludes` leaves the web player's menus and its home screen rows together, in Polyfin as in Jellyfin.

API details: [Libraries hidden from the top bar](jellyfin-api.md#libraries-hidden-from-the-top-bar-for-apps).

## Titles local folders hold

A movie or show of a local folder, linked to the identifier a catalog lists it under, is the catalog's title. So both libraries list it (see [Titles your catalogs list too](local-folders.md#titles-your-catalogs-list-too)).

**Leave out the titles local folders hold**, in the row of a movie or series library under **Content › Libraries**, takes such titles out of that library. The folder's library alone lists them. It is saved with the list's **Save**, and the row then says **Without local folders' titles**. Users set the same for their own libraries under **My sources**.

- **The title stays as it was:** the same item, with its versions (the folder's files among them), its watch state, resume point and favorite. Search, Next Up, Continue Watching and the series page list it once, as before.
- **Only the library's own listings leave it out:** its pages in apps, its home screen row of latest titles, and the home rows made of its catalog. Smart lists, collections, genre pages and the **New in your libraries** row read the catalog as it is.
- **Matching is by identifier:** the titles of movie and show folders, matched by a scan or linked by hand, against the identifiers the catalog lists. A title the catalog lists under another identifier, such as a TMDB one, is another title. The folder's files do not play it, so it stays.
- **Per user:** only the folders whose library a user sees count for that user. A user whose visible libraries leave out the folder's library still finds the title in the catalog's library (see [Visible libraries](user-limits.md#visible-libraries)). So does everyone once the folder or its library is off, or the folder no longer holds a file of the title.
- **Pages stay full:** titles left out are taken out of the catalog's pages before counting, as a user's hidden titles are. So an app's page from an index starts with the next title listed, whatever the addon's page size.
- **Limits:** **Maximum titles** and **Titles read per movie and series catalog** count the titles the library lists. The library reads as many more as it leaves out. The count apps show grows as pages are read, as for any catalog.
- **Home row:** it shows the catalog's first page, as Stremio apps do, without the titles left out. It shows fewer titles when the first page holds some.
- **Which catalogs:** catalogs of movies and series of addons, `anime` types included. A local folder's own library, an IPTV source's, a music library, a collection library, a Live TV catalog and a catalog of other videos have no such choice.

**Compared with Jellyfin**

- Polyfin can leave out of a library what another holds; Jellyfin has no such option. Jellyfin makes a separate item of a series for each library that holds it, the libraries being part of the series' key ([Series.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/TV/Series.cs#L79-L121)), and lists it in each.

API details: [Titles local folders hold](jellyfin-api.md#titles-local-folders-hold-for-apps).

## Catalog limits and refresh

Some catalogs are nearly endless, so Polyfin stops reading a catalog after a set number of items. A higher number shows more, but lists load more slowly and the addons get more requests.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Titles read per movie and series catalog** | **Settings › Catalogs** | 2,000 (100 to 20,000) | Item limit for every addon catalog except Live TV ones. IPTV sources' movies and series are stored whole, not bounded (see [Movies and series libraries](iptv-movies-and-series.md#movies-and-series-libraries)). |
| **Channels read per Live TV catalog** | **Settings › Catalogs** | 10,000 (100 to 50,000) | Item limit for Live TV catalogs (see [Live TV](live-tv.md)). |
| **Keep version lists for (minutes)** | **Settings › Catalogs** | 10 (1 to 360) | How long a title's versions and subtitles from the addons are used before Polyfin asks the addons again. |
| **Refresh catalogs after (minutes)** | **Settings › Catalogs** | 60 (1 to 1,440) | How old a catalog page, the programme guide's included, may get before Polyfin reads it again. Apps never wait for it. |
| **Read every collection each day** | **Settings › Catalogs** | Never (or an hour, 0 to 23) | Reads every collection of the server's collection libraries at this hour of the server's time zone, two at a time, then the catalogs of the server's [smart lists](smart-lists-and-search.md#smart-lists), so that they open at once. It asks the addons for many pages: leave it off for an addon you share with others. |
| **Apps that show collection libraries by collection** | **Settings › Catalogs** | None | The apps, among those that have signed in to the server, told that collection libraries hold collections, as Jellyfin's Collections view does. They then list a collection library's collections, then a collection's titles (see [Collections from addons](collections-people-and-ratings.md#collections-from-addons)). Do not tick jellyfin-web or Streamyfin: they leave such libraries out of their home rows. |
| **Scan local folders every (hours, 0 = never)** | **Settings › Catalogs** | 6 (0 to 168) | How long after its last scan a local folder or network share is scanned again (see [Local folders](local-folders.md#scanning)). 0 turns the schedule off. |
| **Look for versions of watchlist titles every (hours, 0 = never)** | **Settings › Catalogs** | 6 (0 to 168) | How long after its last check a title of a user's Watchlist playlist they have not started is checked again for versions, for the **Now available** and **Still waiting** home rows (see [Tracking](tracking-imports.md#now-available-and-still-waiting)). Each hour, up to 20 titles of each user are checked. 0 turns the checks off. |
| **Watch folders and shares for changes** | **Settings › Catalogs** | On | Scans folders and shares a few seconds after they change (see [Watching for changes](local-folders.md#watching-for-changes)). |

Every change to these settings applies at once, to what is already kept too.

**Watch folders and shares for changes** notices changes this way:

| Source | Scanned |
|---|---|
| A folder mounted in the container | A few seconds after its files stop changing |
| An SMB share | A few seconds after its server tells of a change |
| A WebDAV share | A few seconds after a listing of its folders, made every 2 minutes, differs from its last scan's |
| S3 buckets, SFTP folders and rclone remotes | As often as the folder's **Look for changes every (minutes, 0 = only at scans)** asks |

A library can have its own maximum instead, lower or higher, and list only one genre of its catalog: see [Genre and maximum](#genre-and-maximum).

### Version lists

A longer **Keep version lists for (minutes)** sends fewer requests to the stream addon. That helps with providers that refuse too many, but new versions show up later.

- **Preparing playback ahead:** it readies the next episode a minute before that time ends, at most 9 minutes and at least 1 minute before the end of the episode.
- **Follow-up requests:** whatever the setting, Polyfin asks an addon again 10 seconds after its first answer for a title. It asks once more 30 seconds later while the answers grow, for addons that gather other addons' streams (see [Title pages](versions.md#title-pages)).
- **Old lists:** once that time has passed, a title opened again still shows the versions known before at once, even old ones, while Polyfin asks the addons again. Polyfin keeps the old lists 24 hours more for this.
- **Partial answers:** versions missing from a new answer that lists only part of them stay listed, until Polyfin stops asking that addon again.

### Catalog pages

Catalog pages work the same way:

- Once a page is older than **Refresh catalogs after (minutes)**, it still shows at once. Polyfin reads it again from the addon in the background.
- It is kept 24 hours more, and also kept when the addon fails.
- A new title shows up one visit later.

Polyfin also keeps catalog pages and titles' descriptions in its database. So libraries, collections, home rows and title pages show at once after a restart too, with the images the descriptions give.

- A page read from the database past that time is read again in the background, as above.
- A page no one read for 24 hours past that time is forgotten, and so is a description after a week.
- Searches stay in memory.

To answer quickly, Polyfin reads ahead:

- **Next page:** while you look at a page of a library or collection, Polyfin reads the next one, unless the addon's last answer failed.
- **First pages:** at start, and every hour, Polyfin reads the first page of every library. This is the scheduled task "Read the libraries' first pages" in jellyfin-web's dashboard. It then warms the posters of those pages, so that a home row or library shows them at once the first time too (see [Posters warmed ahead](artwork-and-editing.md#posters-warmed-ahead)).
- **Home rows** (**Latest**) show the first page of each catalog only, as Stremio apps do.
- **Page sizes:** Polyfin remembers how many titles each catalog's pages hold, and where a catalog ends. A collection read again asks its catalogs for all the pages it needs at once.
- **Collection pages:** jellyfin-web's collection pages ask for every title at once, up to 500 in Polyfin. Reading them can take long the first time. The page waits for the addons 3 seconds at most, and shows the titles read by then, at least the first page of each catalog. Polyfin reads the rest in the background. The next time, the page shows them all at once. If Polyfin is still reading, it shows those read so far after 1 second at most.
- **Every collection, each day:** with **Read every collection each day** set to an hour, Polyfin reads every collection of the server's collection libraries whole at that hour. This is the scheduled task "Read every collection", which an administrator can also run by hand. It reads two collections at a time, then every catalog of the server's smart lists up to its maximum. It asks the addons again for the pages past **Refresh catalogs after (minutes)**, and waits for their answers. Every collection page and smart list then opens at once with all its titles. The server log tells how many collections and smart lists it read, and how long it took.
- **Descriptions:** a title's description is read again after 6 hours, or when its page opens after **Refresh catalogs after (minutes)**. Meanwhile, the one known shows. A whole library's are read again on request (see [Library description refresh](#library-description-refresh)).
- **Apps that give up:** when an app gives up on a request, what Polyfin was reading for it goes on, and is kept for the next request. Other apps waiting for the same page get it.

Polyfin checks that a description is the title's:

- A description of another identifier and another type than the title asked is not kept. Some addons answer a request they refuse, as when they limit requests, with HTTP 200 and an error written as a title. It counts as a failed description, and the title is described again at the next request.
- A description by another identifier of the same type, or without a type, is the title's.
- A description whose name or text says the addon refused the request for its rate ("rate limit", "too many requests") is never a description. It pauses Polyfin's background requests to the addon (see [How Polyfin paces addons](addons.md#how-polyfin-paces-addons)).

#### Searches

A search reads the first page of the search catalogs, as Stremio apps do. Those are the catalogs that take the `search` property and require no other. It lists up to 100 titles.

| Step | Time |
|---|---|
| The search answers once the title searches have answered, or after the first found its page | 3 seconds after the first page found |
| When no title search found a page | 10 seconds at most |
| Then it waits for people-search catalogs | 0.3 seconds more |
| A search is kept for the next search of the same term | 10 minutes |
| An empty answer is kept | 1 minute, as an addon may answer nothing while what it reads from fails |

- A later answer is kept for the next search of the same term.
- A search catalog that did not answer in time is left out of searches for 5 minutes, unless one of its pages comes in meanwhile. So an addon that never answers delays one search, not each of them. **System › Health** tells it meanwhile, and the log names the addon.
- Matching a local folder's files, and **Identify**, still wait for every addon (see [Failed searches](unmatched-files.md#failed-searches)).

<a id="refreshing-a-library-s-descriptions"></a><a id="refreshing-a-librarys-descriptions"></a>

### Library description refresh

**Refresh descriptions** reads again, in the background, the descriptions of a library's titles, as **Refresh metadata** does for one title. It is in the menu at the end of the row of a server movie or series library, under **Content › Libraries**.

- A title's overview, people, genres, rating and artwork then follow its metadata addon's new answer.
- An addon that fails leaves the description as it was.
- Libraries of collections, Live TV, music and IPTV sources do not offer it, nor do users' own libraries.

Jellyfin apps start the same refresh. jellyfin-web's **Refresh metadata** on a server movie or series library queues it, from the menu of its card in the "My Media" home row. It joins the same queue, and the library's row under **Content › Libraries** shows it. The request answers at once, as Jellyfin's does once queued. Asked again while the library is refreshing or waiting, it starts no other.

- **Modes:** "Scan for new and updated files" and a full refresh start it, as jellyfin-web sends them. A request that asks no metadata provider starts nothing. Images come with the descriptions, so an image mode starts it too.
- **Options ignored:** "Replace all metadata", "Replace existing images" and trickplay regeneration. A description read again always replaces the one kept, the images follow it, and the image cache stays as it is (see [Clearing the image cache](health.md#clearing-the-image-cache)). Every mode does the same refresh.
- **Libraries that do not offer it:** a collection, music or IPTV library, a smart list, or a user's own library answers as before, with nothing refreshed, as Jellyfin answers for any item it queues. A library the user cannot see answers as not found, as Jellyfin answers for an item it does not find.

How it runs:

- **Which titles:** those of the catalog pages Polyfin keeps, up to the library's maximum, in the catalog's order. Those are the pages apps opened, and the first pages read every hour. For a local folder, every title it lists. Polyfin does not read the rest of a remote catalog to find more titles: that alone could cost thousands of requests.
- **Its pace:** one title every 2 seconds, 30 a minute at most, whatever the addons. A title asks one request of the addon that describes it, or of the next one when it fails. A library of 600 titles kept takes about 20 minutes. Public metadata addons limit how often one server may ask them, and some block a server that asks too fast. At this pace, a refresh stays well below what apps browsing ask for.
- **Rate limits:** an addon that answers "too many requests" (HTTP 429) is waited for as long as it asks (`Retry-After`). When it does not say, the wait is 30 seconds, then doubled, 15 minutes at most. The title is asked again 3 times at most, then counts as not described, and its description stays. Its failures show in **System › Health** as any request's.
- **One at a time:** libraries asked to refresh while one is refreshing wait their turn, so the pace bounds them all. The refresh asks only for descriptions, never for catalog pages. "Read the libraries' first pages", every hour, and the apps' requests go on beside it, its one request at a time added to theirs.
- **Progress:** the library's row shows **Waiting to refresh its descriptions**. Then it shows the titles described against those to describe, with a bar. Then it shows the result: the titles described, and those no addon described.
- **Stopping:** **Stop refreshing**, in the same menu, stops it, or takes it out of the queue. The titles described stay so. A restart stops it too.

**Compared with Jellyfin**

- Polyfin's refresh is always a full one, as "Replace all metadata", since a description is the addon's whole answer. jellyfin-web's **Refresh metadata** on a library offers three modes ([refreshdialog.js, lines 25 to 28 and 76 to 79](https://github.com/jellyfin/jellyfin-web/blob/b29095510e80/src/components/refreshdialog/refreshdialog.js#L25-L79)): "Scan for new and updated files", "Search for missing metadata" and "Replace all metadata", with "Replace existing images". Images follow the new description; there is no separate image mode.
- Polyfin takes the titles it keeps, one at a time at a fixed pace, since its libraries are remote catalogs and their metadata addons public services. Jellyfin queues the library ([ProviderManager.cs, lines 1136 to 1162](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Providers/Manager/ProviderManager.cs#L1136-L1162)), then refreshes each item of its folders without a pace. It refreshes several at once: as many as "Library metadata refresh concurrency" allows, the processor's count by default ([Folder.cs, lines 903 to 912](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Controller/Entities/Folder.cs#L903-L912), [ServerConfiguration.cs, line 251](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Model/Configuration/ServerConfiguration.cs#L251)).
- Polyfin starts no second refresh of a library already refreshing or waiting. Jellyfin queues any item an administrator refreshes, a second time too when asked again. Polyfin has nothing to refresh in a library that does not offer it.

API details: [Library description refresh](jellyfin-api.md#library-description-refresh-for-apps).

## Genre, studio and year pages

A genre, studio or year page lists the titles of the catalogs that offer that name in their genre filter. Those are the catalogs you reach, as libraries or through collections.

- Addons use the genre filter for genres, and some for years or studios.
- Titles from catalogs that cannot be filtered by that name are not on the page.

## Similar titles

A movie's or series' similar titles come from the first page of the catalogs of your addons that offer one of its genres in their genre filter.

- They are ranked by shared genres, directors and actors, and release year.
- The title itself, and titles you played, are left out.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Similar titles** | **Settings › Content › Title pages** | On | Lists titles close to the one open. Off, no similar titles are listed, and no addon catalog is read for them. |

The change applies at once.

Trailers reach apps as remote trailers. Addons give no other extras. Local folders give the extras, local trailers and theme media beside their titles (see [Extras and theme media](naming-and-matching.md#extras-and-theme-media)).

API details: [Similar titles](jellyfin-api.md#similar-titles-for-apps).

<div class="moved-sections">

<a id="addons-that-ask-to-wait"></a>*Addons that ask to wait* moved to [Addons](addons.md#addons-that-ask-to-wait).

<a id="addons-that-fail"></a>*Addons that fail* moved to [Addons](addons.md#addons-that-fail).

<a id="how-polyfin-paces-addons"></a>*How Polyfin paces addons* moved to [Addons](addons.md#how-polyfin-paces-addons).

<a id="identifiers-addons-accept"></a>*Identifiers addons accept* moved to [Addons](addons.md#identifiers-addons-accept).

<a id="languages"></a>*Languages* moved to [Addons](addons.md#languages).

<a id="personal-addons"></a>*Personal addons* moved to [Addons](addons.md#personal-addons).

<a id="who-uses-a-server-addon"></a>*Who uses a server addon* moved to [Addons](addons.md#who-uses-a-server-addon).

<a id="badges-on-posters"></a>*Badges on posters* moved to [Artwork and editing](artwork-and-editing.md#badges-on-posters).

<a id="edit-images"></a>*Edit images* moved to [Artwork and editing](artwork-and-editing.md#edit-images).

<a id="episode-orders"></a>*Episode orders* moved to [Artwork and editing](artwork-and-editing.md#episode-orders).

<a id="generated-images"></a>*Generated images* moved to [Artwork and editing](artwork-and-editing.md#generated-images).

<a id="poster-address-template"></a>*Poster address template* moved to [Artwork and editing](artwork-and-editing.md#poster-address-template).

<a id="poster-shapes"></a>*Poster shapes* moved to [Artwork and editing](artwork-and-editing.md#poster-shapes).

<a id="posters-from-tmdb"></a>*Posters from TMDB* moved to [Artwork and editing](artwork-and-editing.md#posters-from-tmdb).

<a id="cast-and-crew-from-tmdb"></a>*Cast and crew from TMDB* moved to [Collections, people and ratings](collections-people-and-ratings.md#cast-and-crew-from-tmdb).

<a id="collections-from-addons"></a>*Collections from addons* moved to [Collections, people and ratings](collections-people-and-ratings.md#collections-from-addons).

<a id="people"></a>*People* moved to [Collections, people and ratings](collections-people-and-ratings.md#people).

<a id="photos-and-biographies-from-tmdb"></a>*Photos and biographies from TMDB* moved to [Collections, people and ratings](collections-people-and-ratings.md#photos-and-biographies-from-tmdb).

<a id="ratings"></a>*Ratings* moved to [Collections, people and ratings](collections-people-and-ratings.md#ratings).

<a id="titles-a-person-is-known-for"></a>*Titles a person is known for* moved to [Collections, people and ratings](collections-people-and-ratings.md#titles-a-person-is-known-for).

<a id="choosing-lyrics"></a>*Choosing lyrics* moved to [Music, audiobooks and podcasts](music.md#choosing-lyrics).

<a id="lyrics"></a>*Lyrics* moved to [Music, audiobooks and podcasts](music.md#lyrics).

<a id="music-addons"></a>*Music addons* moved to [Music, audiobooks and podcasts](music.md#music-addons).

<a id="volume-between-songs"></a>*Volume between songs* moved to [Music, audiobooks and podcasts](music.md#volume-between-songs).

<a id="movies-waiting-for-their-digital-release"></a>*Movies waiting for their digital release* moved to [Smart lists and search](smart-lists-and-search.md#movies-waiting-for-their-digital-release).

<a id="searches-in-plain-words"></a>*Searches in plain words* moved to [Smart lists and search](smart-lists-and-search.md#searches-in-plain-words).

<a id="smart-lists"></a>*Smart lists* moved to [Smart lists and search](smart-lists-and-search.md#smart-lists).

</div>
