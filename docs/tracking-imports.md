# Tracking imports and the Watchlist

This page is for users who connected a tracking service (see [Tracking services](tracking.md)). It covers importing your watch history and watchlist from those services, the Watchlist view, the Now available and Still waiting rows, and Trakt's recommendations.

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
| Listed by a catalog under its TMDB or TVDB identifier | By that identifier |
| Known by TMDB only, or a series by TVDB only, and listed by no catalog | Polyfin asks the server's metadata addons that describe titles by it (`tmdb:<id>`, `tvdb:<id>`). It is found by the IMDb, TMDB and, for a series, TVDB identifiers of their description, as above. Otherwise it shows under `tmdb:<id>` or `tvdb:<id>`, with its episodes as the description lists them. |
| From PublicMetaDB, which names titles by TMDB only, and described by no addon | By the IMDb identifier PublicMetaDB maps it to, when its contributors agree on one |
| A Simkl anime | By the identifiers and numbers the [anime mapping](tracking.md#anime) gives it |
| Found no way | Counted as not found. The import goes on. |

A title no catalog listed yet shows in your lists at once, described by the addons. Each description is kept a week, so that importing again does not ask again.

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
- Its movies and series go to your Watchlist. A season or an episode on the list counts for its series.
- Titles are found as the history's are. Those not found count among the titles not found.
- The status line tells how many titles the import added to the Watchlist.
- Simkl's anime are found through the [anime mapping](tracking.md#anime). Until the mapping is read, they wait for a later import.

The Watchlist view shows in every Jellyfin app beside the libraries, once your watchlist holds a title you have not started. Its row of latest items shows on the home screen. It takes its name from the server language, such as « À regarder » in French. Each user has their own.

It lists, the latest added first, the movies and series of every list feeding it:

- Trakt's watchlist and Simkl's Plan to Watch;
- AniList's and MyAnimeList's Plan to Watch (see [Importing the lists](tracking.md#importing-the-lists));
- the library of a Stremio or Nuvio account (see [Importing from an app](moving-to-polyfin.md#what-comes-over-from-an-app));
- the watchlist of a Letterboxd export (see [Letterboxd](moving-to-polyfin.md#letterboxd));
- the Watchlist of a Plex account a user signs in with (see [Plex, signing in](moving-to-polyfin.md#plex-signing-in)).

Parental control and blocked genres hide its titles, as everywhere.

| When | What leaves the view |
|---|---|
| You start a title: a movie with a resume point or played, a series with an episode started or played | That title. It shows in **Continue Watching** or **Next Up** instead. |
| No list holds a title any longer | That title, at the next import of the list that brought it. An import that could not read the list whole takes nothing off. |
| A one-off app import, a Letterboxd export or a Plex Watchlist brought the title | Nothing, until you start it |
| You turn the switch off, or disconnect the service | The titles that list alone held. Those another list holds stay. |

Jellyfin apps have no watchlist button, so the view is not edited from them. Change the lists where they live, or use [Silo's apps](silo-apps.md):

- their watchlist button puts a title on your Watchlist until you start it;
- it takes a title off every list holding it;
- a list read again whole, such as Trakt's watchlist, puts back a title it still holds.

API details: [Watchlist for apps](jellyfin-api.md#watchlist-for-apps).

## Now available and Still waiting

Polyfin looks for versions of the Watchlist titles each user has not started. Users then know when a title they wait for comes.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Look for versions of watchlist titles every (hours, 0 = never)** | **Settings › Catalogs** | 6 | How long before a title is checked again. 0 turns the checks off. |

Every hour, Polyfin checks up to 20 titles of each user's Watchlist: those never checked first, then those checked longest ago.

- The scheduled task "Look for versions of watchlist titles" runs the checks. An administrator can run it by hand.
- Disabled and ended accounts are left out.

For a movie, or a series' first regular episode out, Polyfin asks the user's stream addons, as a title's page does. Those are the server's addons the user uses, and their own.

- Torrents' files count for a user who may play them.
- A title has versions when one is neither hidden by the [version order](versions.md#version-order) settings nor taller than the user's [quality group](user-limits.md#quality-groups).
- A title found only in versions the user is not offered, while others exist, still waits.
- A movie whose release date is to come, and a series with no episode out, wait without asking any addon. With **Wait for the digital release** on, so does a movie not out at home yet (see [Movies waiting for their digital release](smart-lists-and-search.md#movies-waiting-for-their-digital-release)).
- Titles kept from the user by parental control, blocked genres, hidden libraries or their hidden titles are never asked for, listed or told of.

The checks follow the addons' wait rules:

- An addon whose server asked to wait is not asked (see [Addons that ask to wait](addons.md#addons-that-ask-to-wait)).
- An addon that answers a check "too many requests" (429, or 503 with `Retry-After`) starts its server's wait, as a refused catalog page does.
- The checks are paced background work. An addon whose background requests pause after a refusal is not asked either (see [How Polyfin paces addons](addons.md#how-polyfin-paces-addons)).
- A title some addon was not asked for, or failed on, keeps what it had. It is checked again at the next hour.
- A version list an addon gave within **Keep version lists for (minutes)**, in **Settings › Catalogs**, is used as it is.

Each check records whether versions were found, and the first time they were. This survives a restart. Two [home rows](home-rows.md#home-rows) list the results:

| Home row | Lists | Order |
|---|---|---|
| **Now available** | Titles that versions were found for | Latest found first |
| **Still waiting** | Titles checked without any version, or not out yet | Latest added first |

- A title not checked yet is in neither row.
- A title whose versions are gone goes back to **Still waiting**, at the check after the one where the addon first answered without them. As on a title's page, a shorter answer is listed with the versions of the list it replaces while Polyfin asks the addon again (see [Title pages](versions.md#title-pages)).
- With **Also a library in other apps**, each row is a library in other apps too, as the Watchlist is.
- "Versions found" is what the addons listed. A version is only tried when it plays, and may still fail.

The first time versions are found for a title that earlier checks found none for, the user gets a message, once per title. It goes to their own targets that chose **Watchlist title available**: see [Watchlist titles available](notifications.md#watchlist-titles-available).

**Compared with Jellyfin**

- Polyfin checks the Watchlist for versions. Jellyfin has no watchlist and no such check. The rows reach other Jellyfin apps as libraries, through **Also a library in other apps**.

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
- With **Also a library in other apps** on, the row is also a library in other apps, as the Watchlist view is.

**Compared with Jellyfin**

- Polyfin lists Trakt's recommendations in a home row. Jellyfin has no such row, and its apps cannot be given a new kind of home section. Polyfin's row reaches them as a library.
