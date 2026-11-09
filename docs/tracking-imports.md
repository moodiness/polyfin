# Tracking imports and the Watchlist

This page is for users who connected a tracking service (see [Tracking services](tracking.md)). It covers importing your watch history and watchlist from those services, the Watchlist playlist, the Now available and Still waiting rows, and Trakt's recommendations.

## Importing your watch history

Polyfin can read what you watched in other apps that report to the same services. Played marks, **Continue Watching** and **Next Up** then match. Importing is off by default.

Under **My account › Tracking**, each connected service has:

| Control | What it does |
|---|---|
| **Import my … history**, such as **Import my Trakt history** | Imports at once when turned on, then on the server's schedule |
| **Import now** | Imports at once |
| Status line | When the last import ran, titles marked played, resume points set, titles of the history not found, when the next import runs, and what stopped it, if anything |

- The schedule is the administrator's **Import connected accounts every**, in **Settings › Security**: every 6 hours by default. See [How often connected accounts are imported](moving-to-polyfin.md#how-often-connected-accounts-are-imported).
- A switch turned on while the service imports, the watchlist or ratings ones included, is imported once that import ends. The service is imported once more.
- Importing only adds to your data in Polyfin. It never sends anything to any service.
- One user's history never reaches another user.
- Last.fm and ListenBrainz have no import: Polyfin only sends them what you play.

Watch data also comes over from other places, the same way:

- an administrator moving from a Jellyfin, Emby or Plex server imports its users' watch data (see [Moving from Jellyfin](moving-to-polyfin.md#moving-from-jellyfin) and [Moving from Emby and Plex](moving-to-polyfin.md#moving-from-emby-and-plex));
- you can import your own from a Jellyfin or Emby server you used before (see [Importing your own watch history](moving-to-polyfin.md#importing-your-own-watch-history));
- you can import from your account on an app built on addons, such as Stremio or Nuvio (see [Importing from an app](moving-to-polyfin.md#importing-from-an-app)).

### What is imported

| Service | Watched movies and episodes | How it is read | Resume points |
|---|---|---|---|
| Trakt | Its watch history | 250 plays a page, read again only when its last activities changed | Its playback progress |
| Simkl | Its watched movies, and the watched episodes of its series and anime | One list status at a time, only what changed since the last import | Its paused playbacks, anime included |
| MDBList | Its watched movies and episodes, without whole series or seasons marked watched without their episodes | 1,000 a page, only what changed since the last import | Its paused playbacks |
| PublicMetaDB | Its watch history | 500 plays a page, whole every time | Its resume points |

Simkl numbers anime as AniDB does. Polyfin translates them to the seasons and episodes of IMDb and TVDB through the [anime mapping](tracking.md#anime).

Polyfin finds each title of the history this way:

| The title | How Polyfin finds it |
|---|---|
| Named by IMDb | By that identifier, as the usual metadata addons name titles: a movie `tt…`, an episode `tt…:<season>:<episode>` |
| Listed by a catalog under its TMDB or TVDB identifier, or whose description gives it | By that identifier. A description gives it as `_tmdbId` or `moviedb_id`, and for a series `_tvdbId` or `tvdb_id` (see [Where the other identifiers come from](addons.md#where-the-other-identifiers-come-from)). So a title PublicMetaDB names by TMDB only is found among those listed, without mapping it to IMDb. |
| Whose TMDB or TVDB identifier an administrator [edited](artwork-and-editing.md#edit-metadata) | By the edited identifier, no longer by the one it replaced |
| Known by TMDB only, or a series by TVDB only, and listed by no catalog | Polyfin asks the server's metadata addons that describe titles by it (`tmdb:<id>`, `tvdb:<id>`). It is found by the IMDb, TMDB and, for a series, TVDB identifiers of their description, as above. Otherwise it shows under `tmdb:<id>` or `tvdb:<id>`, with its episodes as the description lists them. |
| From PublicMetaDB, which names titles by TMDB only, and described by no addon | By the IMDb identifier PublicMetaDB maps it to, chosen as [below](#when-contributors-disagree) |
| A Simkl anime | By the identifiers and numbers the [anime mapping](tracking.md#anime) gives it |
| An episode from PublicMetaDB, Trakt or MDBList | These number episodes as TMDB does, and the addon may number them otherwise. Polyfin pairs TMDB's numbering with the addon's. It reads TMDB's numbering from, in order: the addon's own TMDB video identifiers; a server addon that describes the series by its TMDB identifier; with a [TMDB key](getting-started.md#add-a-metadata-addon-and-a-stream-addon), TMDB's seasons. Episodes are paired by the day they aired, within a day: those released together on both sides, as many on each, by their order; then by name, specials included; then the only one aired that day. A series both number alike keeps its numbers. In a series numbered otherwise, an episode Polyfin cannot place surely is counted as not found, for a manual link: no other episode is guessed. Without any source, the numbers are taken as they are. The pairs are kept a week, or until the addon lists another count of episodes. |
| Found no way | Counted as not found. The import goes on. |

A title the service names by its IMDb identifier is never found as a listed title with another IMDb identifier, even when both have the same TMDB or TVDB identifier. An addon may give a title another title's TMDB identifier, and an import never takes a mark back. With a TMDB key, such a `moviedb_id` is corrected in the background (see [Where the other identifiers come from](addons.md#where-the-other-identifiers-come-from)); a listed title whose identifier was corrected is no longer refused, and is found by the corrected one.

Titles already kept get the `moviedb_id` and `tvdb_id` of their description as it is read again. A library's [description refresh](addons-and-libraries.md#library-description-refresh) reads them all at once.

What Polyfin sends to the tracking services carries a title's edited TMDB and TVDB identifiers, anime titles included, in place of the anime mapping's. Events already queued keep the identifier they were queued with.

A title no catalog listed yet shows in your lists at once, described by the addons. Each description is kept a week, in memory and in the database, as every title's description is (see [Addons and libraries](addons-and-libraries.md)). Importing again within 6 hours asks the addons nothing. Later, the import uses the description kept, and the addon is asked again in the background.

#### When contributors disagree

Each PublicMetaDB contributor maps a TMDB identifier to an IMDb one, and they may disagree. Polyfin chooses this way:

1. It refuses each IMDb identifier its own data names as another title:
   - a catalog listed it under another TMDB identifier, or as a movie for a series (a series for a movie);
   - TMDB gave Polyfin another TMDB identifier for it, or another IMDb identifier for the title;
   - with a [TMDB key](collections-people-and-ratings.md#tmdb-key), TMDB finds it under another TMDB identifier, or only as a movie for a series (a series for a movie).

   When contributors disagree, it also refuses one whose reverse lookup on PublicMetaDB gives other TMDB identifiers and not this one, or gives only titles of the other kind.
2. Of those left, it takes the one the most contributors give. Between as many, it takes the one whose contributions have the most votes. One left alone is taken too.
3. When a tie remains, it takes none. The title is counted as not found, and you can [link it](moving-to-polyfin.md#linking-titles-not-found) to the right title.

Each choice is kept a day, so a mapping contributors correct is followed the next day.

<a id="how-the-history-merges-with-polyfin-s"></a><a id="how-the-history-merges-with-polyfins"></a>

### How the history merges with Polyfin's

| Data | Rule |
|---|---|
| Played marks | An import only adds them, never removes one. A title gets the service's date unless Polyfin's is later, and is played at least once. A title already played keeps its play count. |
| Resume points | Only for titles not played. Of Polyfin's resume point and the service's, the more recent one stays. |
| Several services | A title played on any of them is played. The most recent resume point wins. |

- A title taken off **Continue Watching** gets back only a resume point set after it was taken off (see [Watch state](watch-state.md#watch-state)).
- A position past **Marked played after (%)** marks the title played. One before **Resume point kept after (%)** is ignored. See [Played and resume thresholds](watch-state.md#played-and-resume-thresholds).
- Apps see the changes the next time they refresh. An import does not push them through Jellyfin's live connection, which would ask the addons for every title it changed.

### Pace and problems

Polyfin reads one page a second from Trakt, two from Simkl and MDBList, and PublicMetaDB at its limit per server address. It waits as long as a service asks, up to 15 minutes.

- One import runs at a time per user and service.
- An import interrupted by a restart runs again later.
- When the history cannot be read whole, what was read is imported. The status line says why: the service refused the connection (**Connect again**), could not be reached, or asked to wait too long.
- **Disconnect** asks first. It stops the service's imports and turns them off. What they imported stays in Polyfin.

## The watchlist

Trakt and Simkl also keep the titles you mean to watch. A connected Trakt or Simkl has a second switch under **My account › Tracking**, off by default:

| Service | Its list | Switch |
|---|---|---|
| Trakt | Watchlist | **Import my Trakt watchlist** |
| Simkl | Plan to Watch | **Import my Simkl Plan to Watch list** |

- Turned on, the list is read whole with each import of the service, whether the history switch is on or not.
- Its movies and series go to your Watchlist playlist. A season or an episode on the list counts for its series.
- Titles are found as the history's are. Those not found count among the titles not found.
- The status line tells how many titles the import added to the Watchlist playlist.
- Simkl's anime are found through the [anime mapping](tracking.md#anime). Until the mapping is read, they wait for a later import.

### The Watchlist playlist

Every user has a Watchlist from the start: a playlist of their own, listed with their other playlists in every Jellyfin app, so the Playlists library shows for everyone, even before they make a playlist. It is a real playlist, so every app plays it, shuffles it, reorders it and edits it as any playlist (see [Playlists](watch-state.md#playlists)).

- **Its name:** "Watchlist", in the language of whoever sees it, such as « À regarder » in French (see [Language of generated names](getting-started.md#language-of-generated-names)). Rename it as any playlist; an empty name goes back to the default one.
- **Its image:** a mosaic of its first entries, a series' poster standing for its episodes (see [Generated images](artwork-and-editing.md#generated-images)).
- **What it holds:** movies and episodes only. A series comes as its aired episodes of regular seasons, in order, and a season as its aired episodes. Specials and episodes not aired yet are left out, and episodes aired later are not added, as in any playlist.
- **A series not known or not aired yet:** it waits, and its episodes are added once, in the background, as soon as they are known and one has aired. The task "Add the episodes of watchlist series" does it every hour and at start.
- **Imports add only what is left to watch:** a title a list or an import brings adds a movie you have not started or played, and the aired episodes of a series or a season you have not played, an episode in progress included. A title with nothing left to watch adds nothing, and is not waited for. It stays known to the list, so a list that newly holds it later adds it by the same rule. This holds for the episodes added later in the background too.
- **By hand:** adding a title adds everything, as in Jellyfin: a movie you played, or a series' aired episodes, watched or not.
- **Each item once:** adding a movie or an episode already there changes nothing and keeps its place.
- **Its home row:** an administrator can add a **Watchlist** row to the web player's home page, which shows each user their own Watchlist playlist, in its order and under its name for them (see [Playlist rows](home-rows.md#playlist-rows)). Each user can turn it off under **My account › Home rows**.

It is filled by every list feeding it:

- Trakt's watchlist and Simkl's Plan to Watch;
- AniList's and MyAnimeList's Plan to Watch (see [Importing the lists](tracking.md#importing-the-lists));
- the library of a Stremio or Nuvio account (see [Importing from an app](moving-to-polyfin.md#what-comes-over-from-an-app));
- the watchlist of a Letterboxd export (see [Letterboxd](moving-to-polyfin.md#letterboxd));
- the Watchlist of a Plex account a user signs in with (see [Plex, signing in](moving-to-polyfin.md#plex-signing-in));
- what you add yourself.

**Order:** new titles go at the top, the latest first within one import, by the date the list gives; a series' episodes keep their order within it. An import never moves an entry. An app that adds at a given place, or moves an entry, is followed.

**What stays and what leaves:**

| When | What happens |
|---|---|
| You start or finish a title | It stays. It also shows in **Continue Watching** or **Next Up**. |
| A list read whole no longer holds a title | Its entries leave at that list's next import, unless another list holds the title. Episodes you started or played stay, and so do entries you added or moved yourself. An import that could not read the list whole takes nothing off. |
| A one-off app import, a Letterboxd export or a Plex Watchlist brought the title | It stays until you remove it. |
| You turn the switch off, or disconnect the service | As when the list no longer holds its titles. |
| You remove an entry | It stays off while a list holds its title, episode by episode for a series. A list that newly holds the title, or holds it again after dropping it, brings it back. |
| You hide a title | Its entries stay, and are left out of what apps list (see [Watch state](watch-state.md#watch-state)). Parental control and blocked genres leave titles out too, as everywhere. |

You can add and remove titles in every Jellyfin app, with its "Add to playlist" and its playlist editing. Besides:

- **In the web player:** the ⋮ menu of a movie, an episode, a series or a season has "Add to Watchlist", or "Remove from Watchlist" for a title on it (see [Card menus](web-client.md#card-menus)). On a series or a season, adding puts its aired episodes and removing takes all its entries off; it counts as on it while any of its episodes is. The Jellyfin apps for Android and iOS show the server's web player, so they have it too.
- **In [Silo's apps](silo-apps.md):** their watchlist button.

What you add, or move, is yours: no import takes it off.

**Deleting it:** delete it as any playlist. Your own additions go with it, and the titles your lists hold count as removed, so nothing comes back until a list newly holds a title. It comes back, holding only that, at the next title added, by a list or by you.

**Sharing:** share it, or open it to all, as any playlist (see [Playlists](watch-state.md#playlists)). Its rows, checks, messages and imports stay yours, and what a user it is shared with to edit adds counts as yours. While it keeps its default name, others see it with your name, such as "Watchlist (Alice)", in their own language, so that several users' Watchlists never look the same.

Tracking services are not told of changes: the lists only flow into Polyfin.

API details: [Watchlist for apps](jellyfin-api.md#watchlist-for-apps).

**Compared with Jellyfin**

- Jellyfin has no watchlist. Polyfin's is one of Jellyfin's playlists, so apps need nothing new to show it.

## Now available and Still waiting

Polyfin looks for versions of the titles of each user's Watchlist playlist they have not started: each movie, each series its lists brought, and each episode added by itself. Users then know when a title they wait for comes. The Watchlist playlist itself can have a home row too (see [The Watchlist playlist](#the-watchlist-playlist)), and these two rows stay.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Look for versions of watchlist titles every (hours, 0 = never)** | **Settings › Catalogs** | 6 | How long before a title is checked again. 0 turns the checks off. |

Every hour, Polyfin checks up to 20 titles of each user's Watchlist playlist: those never checked first, then those checked longest ago.

- The scheduled task "Look for versions of watchlist titles" runs the checks. An administrator can run it by hand.
- Disabled and ended accounts are left out.

For a movie or an episode, or a series' first regular episode out, Polyfin asks the user's stream addons, as a title's page does. Those are the server's addons the user uses, and their own.

- Torrents' files count for a user who may play them.
- A title has versions when one is neither hidden by the [version order](versions.md#version-order) settings nor taller than the user's [quality group](user-limits.md#quality-groups).
- A title found only in versions the user is not offered, while others exist, still waits.
- A movie whose release date is to come, and a series with no episode out, wait without asking any addon. With **Wait for the digital release** on, so does a movie not out at home yet (see [Movies waiting for their digital release](smart-lists-and-search.md#movies-waiting-for-their-digital-release)).
- Titles kept from the user by parental control, blocked genres, hidden libraries or their hidden titles are never asked for, listed or told of.

The checks follow the addons' wait rules:

- An addon whose server asked to wait is not asked (see [Addons that ask to wait](addons.md#addons-that-ask-to-wait)).
- An addon that answers a check "too many requests" (429, or 503 with `Retry-After`) starts its server's wait, as a refused catalog page does.
- The checks are paced background work. An addon whose background requests pause after a refusal is not asked either (see [How Polyfin paces addons](addons.md#how-polyfin-paces-addons)).
- A title some addon was not asked for, or failed on, keeps what it had. It is checked again at the next hour. So is a title an addon refused with notices alone saying it limits requests.
- An addon that answers only other notices, such as its own summary of the streams its filters left out, counts as answered: with the list it gave before, if Polyfin still keeps one, else with nothing to play. The title is recorded, waits in **Still waiting** when no addon has versions, and is checked again after the hours set (see [Title pages](versions.md#title-pages)).
- A version list an addon gave within **Keep version lists for (minutes)**, in **Settings › Catalogs**, is used as it is.

Each check records whether versions were found, and the first time they were. This survives a restart. Two [home rows](home-rows.md#home-rows) list the results:

| Home row | Lists | Order |
|---|---|---|
| **Now available** | Titles that versions were found for | Latest found first |
| **Still waiting** | Titles checked without any version, or not out yet | The Watchlist playlist's order, then the latest added first |

- A title not checked yet is in neither row.
- A title whose versions are gone goes back to **Still waiting**, at the check after the one where the addon first answered without them. As on a title's page, a shorter answer is listed with the versions of the list it replaces while Polyfin asks the addon again (see [Title pages](versions.md#title-pages)).
- With **Also a library in other apps**, each row is a library in other apps too.
- "Versions found" is what the addons listed. A version is only tried when it plays, and may still fail.

The first time versions are found for a title that earlier checks found none for, the user gets a message, once per title. It goes to their own targets that chose **Watchlist title available**: see [Watchlist titles available](notifications.md#watchlist-titles-available).

**Compared with Jellyfin**

- Polyfin checks the Watchlist playlist for versions. Jellyfin has no watchlist and no such check. The rows reach other Jellyfin apps as libraries, through **Also a library in other apps**.

## Recommended on Trakt

Trakt recommends movies and series to each of its users, from what they watched and rated. An administrator can add a **Recommended on Trakt** row to the web player's home page (see [Home rows](home-rows.md#home-rows)). Users who connected Trakt then see there what Trakt recommends to them, read with their connection.

- Nothing is to turn on: connecting Trakt is enough. Each user can turn the row off under **My account › Home rows**.
- Polyfin asks Trakt for up to 50 movies and 50 series, without those in your Trakt collection and watchlist. It lists them in Trakt's order, a movie and a series in turn.
- Titles are found as the watchlist's are, by their IMDb, TMDB or TVDB identifier.
- Left out: titles found by none, and those the server's metadata addons do not describe. So are titles you played in Polyfin or hid, and those your parental control or blocked genres hide.

| When | What happens |
|---|---|
| The row first asks | The recommendations are read. The row shows once Trakt has answered, at the next visit of the home page. |
| Every 12 hours | They are read again, while the row asked for them within the last week. |
| A read fails | The last recommendations stay. The read is tried again an hour later. |

- A user without a Trakt connection, or whose connection waits for **Connect again**, sees no such row, nor its library in other apps.
- Disconnecting Trakt, or connecting another account, forgets the recommendations read.
- With **Also a library in other apps** on, the row is also a library in other apps.

**Compared with Jellyfin**

- Polyfin lists Trakt's recommendations in a home row. Jellyfin has no such row, and its apps cannot be given a new kind of home section. Polyfin's row reaches them as a library.
