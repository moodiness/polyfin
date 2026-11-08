# Silo's apps

This page is for administrators whose users have Silo's native apps: turning them on, what works, what does not yet, and the contract Polyfin follows.

Silo's apps run on iPhone, iPad, Apple TV and Mac, and on Android phones and TVs. They can use Polyfin as their server. They speak Silo's own API, under `/api/v2`, not Jellyfin's. Polyfin answers the part of it they need to sign in, browse and play, while the setting allows them.

## Turning it on

Turn on **Allow Silo's apps** under **Settings › General**. It is off by default.

- While it is off, Polyfin answers nothing under `/api/v2`. The apps report that they cannot reach a Silo server there.

To connect an app:

1. Add the server by the address Jellyfin apps use, such as `http://192.168.1.10:8096`.
2. Sign in with a Polyfin user's name and password. On a TV, sign in with a code instead (see [Signing a TV in with a code](#signing-a-tv-in-with-a-code)).

Behind a reverse proxy:

- `/api/v2` must reach Polyfin like the rest.
- A base path such as `/jellyfin` is for Jellyfin apps only.

## How it works

Polyfin makes each answer of Silo's API from the Jellyfin API's answer for the same user and device. So Silo's apps see and play what Jellyfin apps get, under the same limits:

- parental control, blocked genres, the libraries the user may see or hid, and the titles they hid;
- allowed hours, disabled and ended accounts;
- version rules, quality groups, the user's bitrate limit and conversion permissions;
- the sign-in throttle, and two-step sign-in.

Outside their allowed hours, a user's apps are refused, except an administrator's. They may still sign out.

A signed-in app is one of the user's devices. It is listed with the others, and signed out the same ways.

- Its playbacks show in the web player's dashboard and in the statistics.
- Its watch state is the user's, shared with Jellyfin apps: played titles and resume points alike.

How Polyfin answers the apps' requests:

- **A source of the server fails to answer:** the app is told to try again later (503), not that the server failed.
- **Media:** a playback's file, playlists, segments and subtitles are fetched with the app's token. No cache ever keeps them.
- **Artwork:** it needs no token, as Jellyfin's images need none.
- **Request bodies:** Polyfin reads them as JSON, whatever their `Content-Type`.
- **Playlists:** a repackaged or converted playback's playlists name only addresses the apps send their token to. These are `master.m3u8` itself, with `silo_rendition` (and `silo_track` for a subtitle track), and `segment/` beside it.
- **One quality per plan:** those playlists offer the one quality the plan plays. The apps change quality by asking for another plan, so [**Several qualities when converting**](transcoding.md#adaptive-quality) does not apply to them.

## What works

The apps can do the following.

### Connecting and signing in

- **Connecting:** the server's information, identity and setup state.
- **Signing in:** with a name and password, or on a TV with a code; staying signed in, and signing out.
- **Profiles:** each Polyfin user is one profile, named after them, without a PIN.
- **Sign-in screen:** it shows the server's name.
- **Addresses for the user's other devices:** the app offers the **Public address**, under **Settings › Notifications**, then the **Address on the local network**, under **Settings › General**. It offers none when both are empty.
- **Artwork:** at the width each screen draws it.

### Home

Home shows these rows:

- Continue Watching and Next Up;
- the home rows the administrator adds for the web player, at their places, collection rows among them;
- a row for each library, titled with its name: its latest titles, or, for a library of collections, the first of its titles.

Library rows follow the order the user set for their libraries, without those they left out of their latest titles.

- The apps may ask for each row's first titles only (`item_limit`), and open the rest on the row's own page.
- Removing a card from Continue Watching takes off its resume point, as in the web player.
- Home's layout names the same rows without their titles. From it, the Android TV app fills the TV's Watch Next row, and the Apple TV app its Top Shelf, with Continue Watching and Next Up (see [Home layout](#home-layout)).

### Libraries

The apps list the libraries the user sees in Jellyfin apps, in the order the user set.

- A library the user left out of My Media is not listed, whether or not the web player's menus show it.
- Libraries of movies, of series and of both show, libraries of collections among them: an addon's catalog of collections, and Polyfin's Collections view.
- Ticking Silo's apps in **Apps that show collection libraries by collection**, under **Settings › Catalogs**, changes nothing for them.

A library opens on its titles, page by page.

- **Sort:** by title, year, release date, date added, rating, runtime or at random.
- **Narrow:** to its favorites or a search.
- **A library of collections:** it opens on the titles of its collections, each once, in its order.
- **The first page:** what the user is watching in the library, and its latest titles. A library of collections shows instead a row for each of its first 24 collections.
- **Shuffle:** a library, a series, a season or a collection. No title plays twice until all have played, and a series plays one of its episodes.

### Filters

The filter button shows in three places:

- on a library;
- on the Android TV app's Browse screen, which searches every library;
- on the Android apps' Favorites, Watchlist and History.

Its sheet offers genres and content ratings. The app adds its own choices beside them:

- decades;
- watch statuses: unwatched, in progress, watched, favorited, on the watchlist;
- in the Apple apps, movies or series, in a library of both.

Filters picked together keep the titles that match all of them, or any, as the app is set. The results come page by page, in the order chosen.

The genres offered depend on where the button is:

| Where | Genres offered |
|---|---|
| A library | Those of its catalog's genre filter, which Jellyfin apps offer for it. A genre lists the titles the catalog gives for it, as in Jellyfin apps (see [Genre, studio and year pages](addons-and-libraries.md#genre-studio-and-year-pages)). |
| A library of collections | The genres of its titles. A genre keeps the titles of that genre: the catalog's own genre filter, if it has one, chooses collections, not titles. |
| Every library (Browse) | The genres of the libraries of movies and series. |
| Favorites, Watchlist and History | The genres of their titles. |

- The Browse screen lists nothing until something is picked. So a decade or a content rating alone finds nothing there.
- The content ratings offered are those of the titles among the first 2,000.
- A decade, a content rating or an unwatched filter keeps the matching titles among a library's first 2,000 titles. That is as many as a catalog lists by default, as addon catalogs narrow their titles by genre only.
- A genre does the same when any filter may match.
- When all filters must match, favorited, watched and in progress keep the titles of the user's own lists, however many the library holds.

Filters keep to the user's limits:

- a genre they may not see is not offered, and keeps nothing;
- a library they hid offers nothing;
- parental control and the titles they hid leave titles out, as everywhere.

### Collections and box sets

Collections are read-only. To find them, open **Libraries**, then a library, then its **Collections** chip.

- **The Collections tab:** the collections with a title in that library, as the Jellyfin API lists a library's collections. A library of collections lists its own. Each opens on its titles.
- **Counts:** each user sees and counts only the titles they may see, without those they hid.
- **Home:** it shows the titles of collections, in the administrator's collection rows and in the row of a library of collections. It shows no collection itself, as the Apple app shows only cards of movies, series and episodes there.
- **Box sets:** while the server has a **TMDB API key**, a movie's page and a search show box sets (see [Box sets](#box-sets)).

### Title pages and people

Title pages show details, cast and crew, versions, a series' seasons and episodes, artwork, and similar titles, as the web player does. Similar titles leave out those the user played.

An episode's card also carries its series' poster and backdrop, and its season's poster when the season has one. The apps show these in place of the episode's still.

Tapping someone in a cast opens their page, with their titles that the user may see.

- A search finds people credited in such titles.
- Polyfin knows people by the titles that credit them.
- A person's page shows the photo from the addon or from TMDB (see [Photos and biographies from TMDB](collections-people-and-ratings.md#photos-and-biographies-from-tmdb)).
- It shows their TMDB and IMDb identifiers once found, and no biography.

### Favorites, Watchlist and History

- **Favorites:** the heart on a title page adds the title to the user's favorites, those of Jellyfin apps, and takes it off.
  - The Favorites screen lists them by name.
  - Polyfin does not record when a title became a favorite. The apps are told the day it joined the server.
- **Watchlist:** the watchlist button puts a title on the user's [Watchlist playlist](tracking-imports.md#the-watchlist-playlist), as the web player's "Add to Watchlist" does: a movie or an episode itself, a series or a season as its aired episodes, at the top. A series or a season shows as on the watchlist while the playlist holds one of its episodes.
  - Taking a movie or an episode off removes that entry; taking a series or a season off removes all its entries. Either stays off while a list still holds it; a list that newly holds it brings it back.
  - The Watchlist screen lists the playlist, in its order, as Jellyfin apps show it. Started and played titles stay on it.
- **History:** what the user played, the latest first.

### For You and the calendar

- **For You:** a row of titles similar to each of the movies the user played last, without those they played, as Jellyfin's movie recommendations give them.
  - It stays empty until the user played a movie, or while similar titles are turned off in the settings.
- **Calendar:** the coming episodes of the series the user watches or marked favorite, from yesterday on, as Jellyfin's Upcoming lists them.
  - Each episode shows on its air date: Polyfin knows air dates, not times.
  - Following, All, and the user's favorites or watchlist list those episodes. Trending lists none.

### Playback

Movies and episodes play. The app sends what it plays, and Polyfin answers with a plan:

| The app gets | When |
|---|---|
| The file as it is | The app takes it. |
| The file repackaged to HLS | Only its container does not suit the app. |
| The file converted to HLS | Otherwise, as the user's conversion permissions allow. |
| A lower quality, converted to that height | The file is taller (see the table below). |
| Another plan | The app asks for other tracks or another quality, or for a conversion after the file failed to play. |

| Lower quality | Bitrate at most |
|---|---|
| 1080p | 8 Mb/s |
| 720p | 4 Mb/s |
| 480p | 1.5 Mb/s |

Tracks:

- **Audio:** the track the app chooses, or the user's preferred one.
- **Text subtitles:** WebVTT files beside the video.
- **Picture subtitles:** burned in when chosen.
- **The subtitle shown:** the app chooses it, from the user's subtitle language and mode, which Polyfin tells it. A playback the app starts without a subtitle shows none.
- **A track FFmpeg did not recognize:** listed as unavailable. Asking for it is refused, as Silo refuses it, and a playback already playing goes on as it was.
- **Text tracks inside the file:** in a repackaged or converted playback, they become subtitle tracks of its HLS stream. The app's own player lists them and switches between them without loading the video again (see [Subtitle tracks of the stream](#subtitle-tracks-of-the-stream)).

During and after a playback:

- **Skip buttons:** for the intro, recap and credits, and in the Android apps for previews, those Jellyfin apps get (see [Skip markers](#skip-markers)).
- **Progress and stop:** these reports, numbered per installation, keep the resume point.
- **Marking:** a title played or unplayed.
- **Diagnostics:** the apps' reports on their playbacks are accepted, and the failures they report are logged.

## What does not work yet

- **Finding the server on the local network:** add the address by hand.
- **Signing in with a code on a phone, a tablet or a Mac:** there, Silo's apps take a password only, which an account with two-step sign-in cannot use. Such an account signs in to Silo's TV apps only, with a code.
- **Filtering by dynamic range, resolution, audio or subtitle language, studio, network, country or original language:** these filters are not offered.
  - Polyfin knows a title's files only once it is asked to play it, and its catalogs tell none of the others.
  - The apps always show HDR and Dolby Vision anyway. A page filtered by one is refused as not supported, and the app says it could not load it until the filter is taken off.
  - Home's sections offer no filters.
- **Profiles:** editing them, PINs, several profiles per user, avatars.
- **Features the apps hide:** downloads, Watch together, remote control, notifications and push, requests, subtitle searches, AI subtitles or translations, books, diagnostics uploads, external sign-in, settings synced from the app, and the app's welcome tour.
  - The app is told each is not supported, and hides it.
  - The account tells the apps downloads are not allowed, whatever the user's permission to download in Jellyfin apps.
- **The apps' own collections:** the app lists none, and making or changing one is refused as not supported. Make collections in Jellyfin apps or the web player.
- **Taking a series off Next Up:** refused as not supported. Playing the series moves it on.
- **Hiding watches from the history:** mark a title unplayed instead.
- **Other features:** refreshing a person's details, trending or popular titles, and trickplay.
- **Playback features the apps can use but Polyfin does not offer, and so does not announce:**
  - the app's own player showing the file's subtitles, when it plays the file as it is;
  - quality adapting itself to buffering and to the measured speed during a playback;
  - a playback moving to another screen or output without a new plan;
  - the apps' device fixes chosen by the server.

  Their plans still play: the app keeps to what Polyfin announces.
- **Music, audiobooks, books and Live TV:** Silo's apps have no Live TV, and Polyfin lists its libraries of movies and series only.
- **Anything else:** operations not listed here answer 404.
  - Polyfin logs each, as it does for Jellyfin apps: "A Silo app asked for something Polyfin does not serve", with its method and path.
  - It logs each at most once an hour.

## Skip markers

The watch details of a movie or an episode give each version's skip markers. They are the segments Jellyfin apps get for that version (see [Skip buttons](skip-segments.md)).

- **Skip intro and credits buttons**, under **Settings › Content**, must be on. With it off, versions have no markers.
- The Apple apps offer to skip intros, recaps and credits. The Android apps offer to skip previews too.

The markers come in these fields:

- **`marker_segments`** lists every intro, credits, recap and preview of the version, in the order they start.
  - Credits split around a scene during them are two segments.
  - The apps take this list in place of the single markers, even when it is empty.
  - Commercial breaks are left out: the apps know no such marker.
- **`intro`, `credits`, `recap` and `preview`** are the first of each kind. Each tells where it comes from:

  | Found | `source` | `provider` | `algorithm` | `confidence` |
  |---|---|---|---|---|
  | In a segment database | `online` | the database, as `POLYFIN_SEGMENTS` names it | `polyfin.database:v1` | SkipDB's own rating, from 0 to 1; absent for the other databases, which rate none |
  | In the version's chapters | `scanner` | absent | `polyfin.chapters:v1` | absent |
  | In the audio of a season's episodes | `scanner` | absent | `polyfin.audio:v1` | absent |

  A marker is never sent with a made-up `confidence`: an absent one means the source gave no rating.
- **The detail's own `intro`, `credits`, `recap` and `preview`** are those of the version `file_id` names, when the app names one. Otherwise they are the first of each kind among the versions, in their order, as Silo answers them.

A version whose markers cannot be read is listed without any, and still plays.

## Home layout

`GET /api/v2/home/layout` lists Home's rows, in order, without their titles.

- Each row gives its `id`, `section_type`, `title` and `item_limit`.
- The rows are those `GET /api/v2/home/sections` lists, under the user's limits.
- Each row is read alone at `GET /api/v2/home/sections/{id}/items`, which answers the same `section_type`.

Continue Watching is `continue_watching`, and Next Up is `next_up`. The TV apps read the layout, then those two rows only, as Home whole is too large for them:

- the Android TV app mirrors them on the TV's Watch Next row;
- the Apple TV app mirrors them on its Top Shelf.

`include_items`, from 0 to 4, also answers the first rows with their titles, in `included_sections`.

- Each row comes as its own read answers it, cut by `item_limit` as that read is.
- `GET /api/v2/home/capabilities` says so, as `include_items_max`.
- The apps do not ask for it yet.

## Subtitle tracks of the stream

Silo's apps ask for text subtitles as subtitle tracks of the HLS stream, and Polyfin offers them. Silo names this `hls_subtitle_renditions_v1`, with `hls_remux_subtitle_renditions_v1` and `hls_subtitle_renditions_all_v1`.

In a repackaged or converted playback, each text track inside the file becomes a WebVTT track of the stream. These are the tracks the app would otherwise get as WebVTT files.

- The HLS master playlist lists them by their number in the file: a Matroska file's track number, an MP4 file's track ID.
- Each is named by its title, else its language.
- The app's player shows them like any other track.
- Switching between them, or turning subtitles off, keeps the video playing as it is, with no new conversion.
- The playback's subtitle list still gives each track's file, which the app falls back to.

Limits:

- **Same text as the web player:** the tracks are those Polyfin's conversions already extract as WebVTT, cut like the video.
- **Picture subtitles and subtitle files beside the video:** they stay as they are. Picture subtitles are burned in when chosen.
- **A burned-in subtitle:** while one is, the stream has no subtitle tracks.
- **Only the chosen track:** an app that asks for it gets that track alone in the stream.
- **Players that cannot show the tracks:** the app says so, and gets files for the rest of the playback.
- **Track numbers:** read from the file's head once, the first time the file plays. A track whose number cannot be read within 3 seconds stays a file for that playback.
- **The subtitle list:** it gives every track inside the file its number in the file, when known.
- **The file's own tracks:** none is offered for the app's own player to pick from the file (Silo's `native_selectable`), as Polyfin does not have the app's player show them.

## Box sets

A box set is a TMDB collection: the movies of a series of movies, such as a trilogy. A library has one when it shows the user two or more of its movies.

While the server has a **TMDB API key**, every library of movies has box sets, and the apps show them on their own. The key is under **Settings › Content** (see [Edit images](artwork-and-editing.md#edit-images)).

| Where | What the apps show |
|---|---|
| A movie's page | Its box set (see below). |
| A search | Beside the titles, the box sets whose name holds every word searched, "box set" or "collection" among them. Then the box sets two of whose movies' titles hold every word. |
| A library's grid | Each movie, as Silo lists them. Older Silo apps that ask for a grid of box set cards get the grid without them. |

On a movie's page, the box set shows:

- the collection's name, with its "Collection" read as "Box Set";
- the collection's poster and backdrop from TMDB, else those of its first movie;
- the years of its movies, how many it holds, and how many the user played.

It opens on its movies, in the order they came out.

- Polyfin downloads the artwork from TMDB once and relays it, as it relays titles' artwork.
- The apps never reach TMDB.

### How Polyfin finds box sets

Addons do not tell which collection a movie is in. So Polyfin asks TMDB, in the background, while the key is saved:

- **Ahead:** a pass runs a minute after Polyfin starts or the key is saved, then each day.
  - It lists the first 2,000 movies of each of the server's libraries of movies, as its addons list them, and looks up those it does not know.
  - It reads no other library: not those of series or of both, nor those of collections, though their collections may group catalogs of movies.
  - A first pass takes about five minutes per 1,000 movies, as a lookup takes a quarter of a second at least.
  - The daily passes ask only for the answers grown old.
- **First:** the movies a movie's page or a search reads are asked ahead of the pass.
  - A movie's page waits up to a second for its own movie.
  - So its first visit shows its box set, once the other movies are known.

Each lookup follows these rules:

- A movie is looked up by its TMDB identifier, else its IMDb one.
- At most four movies are looked up a second. Box sets share this pace with titles' credits (see [Cast and crew from TMDB](collections-people-and-ratings.md#cast-and-crew-from-tmdb)).
- When TMDB says to slow down, the lookups pause as long as it asks.
- The answers are kept for every user.
- They are asked again after three months, or after a month for a movie TMDB did not know.

Which movies have box sets:

- A library's box sets are those of its first 2,000 movies, as many as a catalog lists by default.
- Each user sees and counts only the movies they may see. Parental control, blocked genres and the titles they hid leave movies out, as everywhere.
- A box set left with one movie for a user is no box set for them.
- Libraries of series, of both, and of collections have none.
- A movie's page shows a box set only for a movie a library of movies lists.
- Without the key, no library has box sets, and TMDB is never asked.

In the server's log, each pass says at Info:

- how many libraries of movies it read;
- how many movies it listed, queued for a lookup, and already knew;
- each library of movies it could not read, with the reason.

On a server without a library of movies, it logs "No library of movies: box sets have none until one is added".

**Compared with Silo**

- Polyfin has box sets in every library of movies while the TMDB key is saved, from two movies, with no setting: the key is the setting. Silo's administrators turn box sets on for each library, with the least number of movies.
- Polyfin does not give the collections' overview. Silo gives it.

**Compared with Jellyfin**

- Polyfin makes no collections of TMDB's collections: its box sets are for Silo's apps, and Jellyfin apps see the collections users and addons make. Jellyfin keeps the collection TMDB names for a movie ([TmdbMovieProvider.cs, lines 218 to 221](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Movies/TmdbMovieProvider.cs#L218-L221)). For a library whose "Automatically add to collection" is on, it makes a collection of each with two or more of its movies ([CollectionPostScanTask.cs, lines 53 to 123](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/Validators/CollectionPostScanTask.cs#L53-L123)).

API details: [Silo box sets API](jellyfin-api.md#silo-box-sets-api).

## Signing a TV in with a code

Silo's apps for Apple TV and Android TV show an 8-digit code, such as `4821 7730`, and a QR code, beside their password sign-in. The code is a Quick Connect request, as Jellyfin apps make (see [Signing in to apps with Quick Connect](jellyfin-compatibility.md#signing-in-to-apps-with-quick-connect)). A signed-in user approves it in one of two ways:

- **In a browser:** open `/activate` at Polyfin's address, as the TV says, or scan the QR code.
  1. It leads to the admin app's **Quick Connect** page. The code is filled in when it comes from the QR code.
  2. Sign in there if asked.
  3. Check the device, and approve it.
- **On a phone signed in to Silo's app on the same server:** choose "Sign in a TV", then enter the code.
  - "Not now" turns it down, and the TV says the sign-in was declined.

The TV then signs in as the user who approved it, with the session a password gives. It becomes one of their devices, named as the TV names itself, and is listed and signed out as the others.

- **Lifetime:** the code lasts Quick Connect's 10 minutes. The TV then shows a new one by itself.
- **Leaving:** a TV that leaves its sign-in screen withdraws its code.
- **Once:** a code is approved once, by a signed-in user, for themselves.
  - Once approved, turned down or withdrawn, nobody can approve it again.
  - The TV collects its session once.
- **Disabled or ended accounts:** they get no session, and the TV says the sign-in was declined.
- **Two-step sign-in:** an account with it signs a TV in this way. The admin app asked for its code when it signed in.
- **Address:** the QR code and the address the TV shows start with the **Public address**, under **Settings › Notifications**, when it is set. Otherwise they start with the address the TV reached Polyfin at.
- **Settings:** it needs **Allow Quick Connect**, under **Settings › General**, as well as **Allow Silo's apps**. With Quick Connect off, the TV asks for a password.

Polyfin does not:

- tell the TV when a phone opened its code;
- approve Jellyfin apps' 6-digit codes from Silo's app;
- move a phone's playback to a TV.

The apps are told so, and offer none of these.

**Compared with Jellyfin**

- Jellyfin does not answer Silo's apps.
- Silo's TV apps take 8-digit codes, which Polyfin gives them from the same requests. Jellyfin's Quick Connect codes have 6 digits.
- A Silo TV's secret signs it in once, as Silo's apps expect. The secret a Jellyfin app polls with signs it in until its request expires, as in Jellyfin, so that an app retrying after a network error is not refused.

API details: [Silo TV sign-in with a code](jellyfin-api.md#silo-tv-sign-in-with-a-code).

## The contract followed

Polyfin follows Silo's published OpenAPI contract, `contracts/api/v2/openapi.json` in Silo-Server/silo-server. It follows it at the commit [`internal/silo/followed.json`](../internal/silo/followed.json) records.

- `GET /api/v2/system/info` gives that file's SHA-256 as `contract_digest`, and the links the contract requires.
- Polyfin serves neither the contract file nor the list of capabilities those links name.
- Silo has no release yet, and its contract still changes. A later version of the apps may need operations or fields Polyfin does not answer.

Polyfin's tests also check what it gives against the rules the apps apply. They cover the capabilities, plans, libraries, rows, Home's layout, catalog pages, Collections tabs, favorites and watchlist, people, similar titles, For You, the calendar, skip markers and the discovery documents.

Those rules are read from the apps' code: Silo-Server/silo-apple and Silo-Server/silo-android, at the commits that file records. Among them:

- the apps start a playback only when the server announces every feature they need;
- they take only playback sessions named by a UUID;
- they show only libraries of movies, of series, or of both;
- they leave out of a row on Home or on a library's first page any card but a movie, a series or an episode;
- the Android app opens a person's page only by a number, which Polyfin gives each person as it does libraries and files;
- a row of Home's layout reads alone as the same type, or the TV apps leave the home screen's rows as they were;
- a version's `marker_segments` holds its single markers, as the apps take it in their place;
- a feature whose discovery document is not available is hidden.

That record also gives the contract's SHA-256, and the app files the tests' rules cite.

- A weekly workflow compares the record with the main branches of Silo's repositories, the operations Polyfin answers as unsupported included.
- When something changed, it opens or updates an issue labelled `clients`.
- See [Following Silo](development.md#following-silo).

The tests ask for filters as the apps do:

- The Apple apps send the facets picked as the `groups` of `GET /api/v2/catalog`, their values typed.
- The Android apps send them in the body of `POST /api/v2/catalog/query`, flags as booleans and every other value a text.
- The apps need every list of `GET /api/v2/catalog/filters`, and the Apple app lists each value once.
- A filter Polyfin cannot apply, such as HDR, is refused as not supported (501 `capability_unsupported`).

Polyfin's code is its own, written from the contract's facts; no part of Silo, which is under the AGPL-3.0, is in Polyfin.

**Compared with Jellyfin**

- Jellyfin does not answer Silo's apps.
- Library and file identifiers are numbers in Silo's API. Polyfin gives each library and version one the first time it lists them, and keeps it.
- Polyfin's sessions do not expire. The app's refresh token is its session's token, and refreshing it tells the app whether it was signed out.
- Polyfin tells Silo's apps where each skip segment comes from, and `GET /Polyfin/MediaSegments/{id}` tells other apps (see [Skip buttons](skip-segments.md#how-skip-buttons-work)). Jellyfin's media segments do not say.
- Polyfin's `/Items/Filters` lists the genres of a library's catalog's filter (see [Genre, studio and year pages](addons-and-libraries.md#genre-studio-and-year-pages)). Jellyfin's lists the genres, official ratings and years of the items a user sees under a folder ([FilterController.cs, lines 53 to 95](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/FilterController.cs#L53-L95)). Silo's apps get Polyfin's genres, with the content ratings of the library's first 2,000 titles.
