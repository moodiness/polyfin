# Addons and libraries

This page explains how Polyfin turns Stremio addons and Eclipse music addons into Jellyfin libraries. It also covers catalog limits, collections, genre and people pages, similar titles, artwork and editing items.

## Addons

Polyfin gets its content from Stremio addons. Each addon has a role:

- AIOMetadata provides catalogs and metadata.
- AIOStreams provides streams and subtitles.
- Any other addon that speaks the standard Stremio protocol also works.

### Server addons

To share an addon with every user, open **Add a source** under **Content › Sources**, keep the **Stremio addon** tab, and paste its **Manifest address** (from the addon's configure page). Then pick under **Content › Libraries** which of its catalogs become libraries in Jellyfin apps.

- When an addon offers collection catalogs, those are enabled first.
- Otherwise its first 20 movie, series and TV catalogs are enabled.
- This is only a starting point: you can enable any number of catalogs.

Only administrators can install addons hosted on a local network address.

### Personal addons

Each user can add their own addons and libraries under **My sources**. Users can also turn the server's sources off for themselves with **Use the server's sources**.

### Manifest addresses

Manifest addresses usually contain your addon settings or keys. Polyfin never shows them in full. To change one, use **Replace the address** in the source's more-actions menu.

Each source has its own page, from **Open its page** on its row, with its details, an **Active** switch and its place in the order. **Remove** asks first.

## Libraries

Stremio catalogs become Jellyfin libraries. Stremio streams become versions of the same item, and addon subtitles become external subtitle tracks. An app's subtitle search lists the subtitles from the user's subtitle addons. See [Playback](playback.md) and [Subtitles](subtitles.md).

**For app developers:**

- Versions are Jellyfin media sources.

### Library images

Under **Content › Libraries**, each library shows a small image at the start of its row. Select it to choose, under **Image in apps**, the image Jellyfin apps show on the library's tile:

- **None**: no image, as before. This is the default.
- **Automatic**: Polyfin takes it from the catalog's first page: the first backdrop, as library tiles are wide, else the first wide poster, else the first poster. A music library takes the first artwork of its row. A catalog without any shows no image. Users under parental control or blocking genres are not shown it, since it may come from a title hidden from them.
- **Custom**: **Upload an image**, or paste an address under **Or an image address**, which Polyfin downloads once. The picture follows the rules of [Edit images](#edit-images): a JPEG, PNG or WebP picture of up to 10 MB, kept in the database. Only administrators can use local network addresses. **Remove the image** deletes it, and the library shows none.

Image changes are saved at once, apart from the list's **Save**. A catalog just added to the list gets its image once the list is saved. Live TV catalogs make no library tile, so they have no image. Users choose the images of their own libraries the same way under **My sources**.

A library image uploaded with jellyfin-web's **Edit images** is its custom image too. Choosing **None** or **Automatic** deletes it.

A library taken out of the list keeps its custom image, for when it is added back. Removing the addon deletes the custom images of all its libraries.

**For app developers:**

- The library's image is its Primary image: `/UserViews` and `/Items` give its tag in `ImageTags`, `/Items/{id}/Images/Primary` serves it, and `/Library/VirtualFolders` names the library as its `PrimaryImageItemId`.
- The automatic image is looked up again with the catalog's pages, after **Refresh catalogs after (minutes)**. Its tag changes with the title it comes from. `/UserViews` never waits for it: until it is found, the library shows the image found last, or none.

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

- A listing of a library, of an addon's collection or of a collection made by users that sets no `limit`, as jellyfin-web's collection pages ask, lists up to 500 titles, or the library's maximum when lower. It waits for the addons 8 seconds at most, then lists the titles read by then, and never fewer than a page of 100. A listing with a `limit` lists as many as it asks, within the library's maximum.

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

A row lists its own items. When an app asks for another kind, the library derives it:

- the albums and artists its songs name; or
- the songs of its albums, playlists or artists, reading at most 50 of their pages, 8 at a time.

An album or artist that a song only names is found again through the addon's search when opened. So addons without catalogs still serve search and album, artist and playlist pages.

Polyfin keeps what music addons answer. Catalog pages are read again after **Refresh catalogs after (minutes)**, and album, artist and playlist pages after 6 hours. A page due to be read again is still served at once, for up to 24 more hours, while Polyfin asks the addon again in the background. Searches are kept 10 minutes. At most 8 requests for pages go to one addon at a time.

### Instant mixes and lyrics

- A song's instant mix is that song, then its album's and its artist's other songs.
- An album's mix is its songs and its artist's songs.
- An artist's or a playlist's mix is their songs.
- Every mix is shuffled.
- Addons give no lyrics.

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

### Health and parental control

Music addon requests count in **System › Health** like a Stremio addon's: last answer, failures and response time, with the same manifest check (see [Administration](administration.md)).

- Explicit songs and albums are hidden from users whose parental control sets a highest rating.
- Music has no rating, so all music (books, for audiobooks) is hidden from users who block unrated music.
- Visible libraries and allowed hours apply as elsewhere.

**Compared with Jellyfin:**

- Jellyfin's instant mixes pick songs that share genres, which addons rarely give. Polyfin's mixes are built from album and artist instead.
- The lyrics endpoints answer as Jellyfin does for a song without lyrics.
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

A library can have its own maximum instead, lower or higher, and list only one genre of its catalog: see [Genre and maximum](#genre-and-maximum).

A longer **Keep version lists for (minutes)** sends fewer requests to the stream addon, which helps with providers that refuse too many. But new versions show up later. Preparing playback ahead readies the next episode a minute before that time ends: at most 9 minutes and at least 1 minute before the end of the episode. Whatever the setting, Polyfin asks an addon again 10 seconds after its first answer for a title, and once more 30 seconds later while its answers grow, for addons that gather other addons' streams (see [Title pages](playback.md#title-pages)).

Once that time has passed, a title opened again still shows the versions known before at once, even old ones, while Polyfin asks the addons again: Polyfin keeps the old lists 24 hours more for this. Versions missing from a new answer that lists only part of them stay listed until Polyfin stops asking that addon again.

### Catalog pages

Catalog pages work the same way. Once a page is older than **Refresh catalogs after (minutes)**, it still shows at once, and Polyfin reads it again from the addon in the background. It is kept 24 hours more, and also kept when the addon fails. A new title shows up one visit later.

Polyfin also keeps catalog pages and titles' descriptions in its database. So libraries, collections, home rows and title pages show at once after a restart too. A page no one read for 24 hours past that time is forgotten, and so is a description after a week. Searches stay in memory.

To answer quickly:

- While you look at a page of a library or collection, Polyfin reads the next one ahead, unless the addon's last answer failed.
- When it starts, and every hour, Polyfin reads the first page of every library: the scheduled task "Read the libraries' first pages" in jellyfin-web's dashboard.
- Home rows (**Latest**) show the first page of each catalog only, as Stremio apps do.
- Polyfin remembers how many titles each catalog's pages hold and where a catalog ends. A collection read again asks its catalogs for all the pages it needs at once.
- jellyfin-web's collection pages ask for every title at once, up to 500 in Polyfin. The first time, reading them can take long: the page waits for the addons 8 seconds at most and shows the titles read by then, at least 100. The next time, it shows those at once and reads further.
- A title's description is read again after 6 hours, or when its page is opened after **Refresh catalogs after (minutes)**. Meanwhile, the one known shows.
- A search reads the first page of the search catalogs, as Stremio apps do, and lists up to 100 titles. It answers once the title searches have answered, and waits 0.3 seconds more for people-search catalogs. A slower answer is kept for the next search of the same term.
- When an app gives up on a request, what Polyfin was reading for it goes on and is kept for the next request. Other apps waiting for the same page get it.

Every change to these settings applies at once, to what is already kept too.

## Collections

### Collections from addons

An addon's collection catalogs (such as AIOMetadata's) become collection libraries. Each collection gathers the catalogs it groups, movies and series together.

A collection may also group other collections: a genre's movies and series, or a franchise's movies in several sagas. jellyfin-web shows those collections. Streamyfin asks for a collection's movies and series instead, and gets the titles of every collection within it, up to three levels deep. When they fit on one page, they are sorted as it asks, such as a franchise's movies by release date.

On the home page of Jellyfin apps, each collection library has a row of its collections, as a movie or series library has a row of its titles: "Recently Added in" the library's name. In jellyfin-web, a user can leave a library out of these rows under **Settings › Home**. jellyfin-web opens the library's Suggestions tab from the row's title, which it leaves empty for a library of mixed content; the library's tile opens its collections.

**Compared with Jellyfin:** a Jellyfin library of collections gets no home row. Polyfin's collection libraries are told to apps as libraries of mixed content, so that they get one. Opening one shows its collections. Without an image, its tile shows the icon of a mixed library.

**For app developers:**

- A collection library has no `CollectionType`, unlike Jellyfin's Collections view of collections made by users, which keeps `boxsets`. jellyfin-web and Streamyfin leave `boxsets` libraries out of their rows of latest items.
- `/Items/Latest` lists its collections, `BoxSet` items.
- A listing that asks for `Folder` items keeps them, as jellyfin-web lists a library without type by its folders, movies and series.
- A `Recursive` listing of an addon's collection that asks only for titles (no `BoxSet` or `Folder`), as Streamyfin's collection pages ask, lists the titles of the catalogs the collection and the collections within it group, merged one of each in turn, rather than those collections. A listing that fits on its first page is then sorted by its `sortBy`; a listing in pages keeps the catalogs' order.

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

- The **Collections** view has CollectionType `boxsets`. A listing of BoxSets across the server includes user collections.
- Hidden titles do not count in `ChildCount`, `RecursiveItemCount` or the played count.
- Without the permission, `POST /Collections` and `POST` or `DELETE /Collections/{id}/Items` answer an empty 403, as in Jellyfin, administrators included.
- Delete a collection with `DELETE /Items/{id}`, as in Jellyfin. Users without permission get 401.
- A missing name, or a title the user may not see, answers 400.

## Genre, studio and year pages

A genre, studio or year page lists the titles of the catalogs you reach, as libraries or through collections, that offer that name in their genre filter. Addons use the genre filter for genres, and some for years or studios. Titles from catalogs that cannot be filtered by that name are not on the page.

## People

The actors, directors and writers a title credits open as people, with the photo the metadata addon gives.

A person's titles are:

- those your addons' people-search catalogs (such as AIOMetadata's People Search) find for their name; plus
- the titles of your addons that Polyfin already knows them in.

Without a people-search catalog, only the second kind show.

The people list holds the people credited in the titles of your addons that someone opened. People credited only in other users' own addons stay hidden.

## Similar titles

A movie's or series' similar titles come from the first page of the catalogs of your addons that offer one of its genres in their genre filter.

- They are ranked by shared genres, directors and actors, and release year.
- The title itself and titles you played are left out.

Trailers reach apps as remote trailers. Addons give no other extras.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Similar titles** | **Settings › Content** | On | Lists titles close to the one open. Off, no similar titles are listed and no addon catalog is read for them. |

The change applies at once.

**For app developers:**

- With **Similar titles** off, every Similar route answers an empty list. An unknown title is still not found.

## Artwork

Polyfin relays images from the addons' artwork servers, IPTV channel logos included, so Jellyfin apps only ever talk to Polyfin.

- Polyfin downloads an image once, however many apps ask for it at once, and asks one artwork server for at most 16 images at a time. It asks for at most 4 at a time from the servers of IPTV sources and of their logos, often the provider's panel, and for 10 minutes from a server that reset a connection or answered 429, 502, 503 or 504.
- It keeps the images it relayed in memory (128 MB) and in the `images` folder of `POLYFIN_CACHE_DIR` (1 GB), the least recently used going first. They are not downloaded again after a restart.
- An image that could not be downloaded is not asked for again for 2 minutes.
- An image its artwork server does not have (`404` or `410`) answers `404`, as Jellyfin answers for an image an item does not have, and apps show their placeholder. An artwork server that fails otherwise (no answer, a `5xx`) makes it answer `502`.
- An app asking for an image much smaller than the original (`maxWidth`, `maxHeight`, `fillWidth`, `fillHeight`, `width` or `height`) gets it resized, as a JPEG, or a PNG when it was one. Sizes are rounded up to a few steps, which are kept too.
- An image asked for with its `tag` may be kept by the app for good: the tag changes with the image.

Seasons show their own poster when the metadata addon lists the seasons' posters, as Jellyfin shows a season's own image. A season without one shows its series' poster.

That list does not name the seasons. So Polyfin reads it only when it has one poster per season of the series' episodes, with specials included or left out. Otherwise it shows the series' poster on every season, rather than risk showing one season's poster on another. Polyfin asks the addon for nothing more: the posters come with the series' description.

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
- People, provider identifiers and the editor's other fields (locks, display order, content type, date added) are not kept: they stay the addons'.

### Edit images

**Edit images** has no image provider to search, as on a Jellyfin server without one. It uploads a Primary, Backdrop, Logo, Thumb or Banner image in place of the addon's.

- The image must be a JPEG, PNG or WebP picture of up to 10 MB.
- It is scaled down to 3,840 pixels and kept in the database, as profile pictures are.
- Other image types, and anything that is not such a picture, are refused.
- Deleting an upload shows the addon's image again. The addon's own artwork cannot be deleted.

### Identify

**Identify** searches the server's addons that describe titles by name, for movies and series. It finds nothing for other types. Applying a result fails, because an item is its addon's title and cannot be bound to another.

**Compared with Jellyfin:**

- Jellyfin keeps any uploaded image file as sent; Polyfin refuses other types with 400.
- Applying an **Identify** result fails as a failed identification does in Jellyfin (500).
- Jellyfin picks suggestions among everything; Polyfin picks among the first page of each library.

**For app developers:**

- `/Items/Root` is the user's root folder; its children are the user's views.
- `/Items/Counts` counts each library's first page of titles, plus one when more follow, as genre and studio pages count, as far as Polyfin keeps them: it asks the addons nothing. The user's favorites are counted exactly with `isFavorite=true`.
- `/Items/Suggestions` returns titles picked at random among the first page of each library.
