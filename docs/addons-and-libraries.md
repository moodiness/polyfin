# Addons and libraries

This page explains how Polyfin turns Stremio addons and Eclipse music addons into Jellyfin libraries. It also covers catalog limits, collections, genre and people pages, similar titles, artwork and editing items.

## Addons

Polyfin gets its content from Stremio addons. Each addon has a role:

- AIOMetadata provides catalogs and metadata.
- AIOStreams provides streams and subtitles.
- Any other addon that speaks the standard Stremio protocol also works.

### Server addons

To share an addon with every user, paste its manifest URL (from the addon's configure page) under **Content › Sources**. Then pick under **Libraries** which of its catalogs become libraries in Jellyfin apps.

- When an addon offers collection catalogs, those are enabled first.
- Otherwise its first 20 movie, series and TV catalogs are enabled.
- This is only a starting point: you can enable any number of catalogs.

Only administrators can install addons hosted on a local network address.

### Personal addons

Each user can add their own addons and libraries under **My sources**. Users can also turn the server's addons off for themselves.

### Manifest URLs

Manifest URLs usually contain your addon settings or keys. Polyfin never shows them in full.

## Libraries

Stremio catalogs become Jellyfin libraries. Stremio streams become versions of the same item, and addon subtitles become external subtitle tracks. An app's subtitle search lists the subtitles from the user's subtitle addons. See [Playback](playback.md) and [Subtitles](subtitles.md).

**For app developers:**

- Versions are Jellyfin media sources.

## Music addons

Eclipse music addons install like Stremio addons: under **Content › Sources** for the server, or under **My sources** for a user's own. You can add one by its manifest URL or by its base address (`https://addon.example/{token}/`). Polyfin tells the two kinds of addon apart by their manifest. Music addon addresses are redacted like manifest URLs.

### Music addon settings

Change the settings an addon declares (pickers, switches, texts and numbers) with its **Settings** button under **Content › Sources** or **My sources**.

- Each setting holds one value.
- A per-network setting keeps its Wi-Fi default.
- Settings travel as query parameters with every request to the addon, as Eclipse sends them.

### Music libraries

Each catalog row (songs, albums, artists or playlists) is a library under **Libraries**:

- a Jellyfin music library; or
- a books library, for an addon whose `contentType` is `audiobook`. Its tracks are audiobooks, with the chapters their stream gives.

Podcasts have no Jellyfin type: their episodes are songs.

A row lists its own items. When an app asks for another kind, the library derives it:

- the albums and artists its songs name; or
- the songs of its albums, playlists or artists, reading at most 50 of their pages.

An album or artist that a song only names is found again through the addon's search when opened. So addons without catalogs still serve search and album, artist and playlist pages.

### Instant mixes and lyrics

- A song's instant mix is that song, then its album's and its artist's other songs.
- An album's mix is its songs and its artist's songs.
- An artist's or a playlist's mix is their songs.
- Every mix is shuffled.
- Addons give no lyrics.

### Music playback

Polyfin decides how to play a track with the app's device profile, as Jellyfin decides for audio. It uses the codec, container, sample rate and bit depth from the addon's stream reply.

- The stream is analyzed only when the reply gives none of these.
- The reply does not give a lossless stream's bitrate. Polyfin takes it as the bitrate of its samples in stereo, so that a bitrate limit converts it.
- Polyfin plays the track as it is, through the same source and relay rules as videos (see [Playback](playback.md)).
- Or it converts the track with FFmpeg to the codec, bitrate, sample rate and channels the app asks for, progressively or in 3-second HLS segments (see [Transcoding](transcoding.md)).
- An expired link is asked for again, before it expires (`expiresAt`) or once it fails.

The user's conversion permission, bitrate limit and number of playbacks at once apply (see [Users](users.md)). Quality groups, which are about video heights, do not.

Playback reports count plays and mark songs played, without resume points. Audiobooks keep their resume points, with Jellyfin's audiobook thresholds. The activity log records audio playback as Jellyfin does.

Eclipse's video renditions are not used: Jellyfin music apps cannot show a video for a song.

### Health and parental control

Music addon requests count in **Health** like a Stremio addon's: last answer, failures and response time, with the same manifest check (see [Administration](administration.md)).

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
| **Keep version lists for (minutes)** | **Settings › Catalogs** | 10 (1 to 360) | How long a title's versions and subtitles from the addons are kept. |
| **Refresh catalogs every (minutes)** | **Settings › Catalogs** | 10 (1 to 1,440) | How long catalog pages are kept, the Live TV guide's included. |

A longer **Keep version lists for (minutes)** sends fewer requests to the stream addon, which helps with providers that refuse too many. But new versions show up later. Preparing playback ahead readies the next episode a minute before that time ends: at most 9 minutes and at least 1 minute before the end of the episode.

Every change applies at once, to what is already kept too.

## Collections

### Collections from addons

An addon's collection catalogs (such as AIOMetadata's) become collection libraries. Each collection gathers the catalogs it groups, movies and series together.

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

Under **Users**, the **Access** section's **Can manage collections** permission is on for administrators, existing ones included, and off for other users. Without it, nobody can create a collection or add and remove its titles, administrators included. Administrators and users who may manage collections can delete a collection. See [Users](users.md).

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

Polyfin relays images from the addons' artwork servers, so Jellyfin apps only ever talk to Polyfin.

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
- `/Items/Counts` counts each library's first page of titles, plus one when more follow, as genre and studio pages count. The user's favorites are counted exactly with `isFavorite=true`.
- `/Items/Suggestions` returns titles picked at random among the first page of each library.
