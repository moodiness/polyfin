# Moving to Polyfin

This page brings accounts and watch history over from Jellyfin, Emby, Plex, Letterboxd, Nuvio or Stremio. Every import only reads its source, and never overwrites newer Polyfin data.

## Moving from Jellyfin

**Import from another server**, at the top of **Users** next to **Create a user**, brings over a Jellyfin server's accounts and what each user watched. It sits there because it creates accounts and fills their watch state. Only administrators see it.

**Server** chooses **Jellyfin**, for Jellyfin and servers that speak its API, or **Emby** or **Plex** (see [Moving from Emby and Plex](#moving-from-emby-and-plex)).

The page shows the running or last import, whoever started it: an administrator, or a user importing their own history (see [Importing your own watch history](#importing-your-own-watch-history)).

### Connecting to the server

- **Server address**: the address Jellyfin opens at in a browser, such as `http://192.168.1.10:8096`, with its path if any. A host and port alone mean `http://`. Local network addresses work.
- **Connect with**:
  - **An API key**: create one in Jellyfin's dashboard, under API Keys. It reads every user's watch data at once.
  - **A user account**: a **User name** and **Password**, empty if the account has none. Polyfin signs in as Jellyfin's apps do, then signs out. Every server that speaks Jellyfin's API allows this, even without API keys. An administrator's account lists every user. Any other account lists and imports only itself, even on a server that shows every user to anyone.

Polyfin uses the key and passwords for this import only and never saves them. Each import asks again.

**Connect** lists the server's users, with their administrator status and whether they are disabled. The page says so for a wrong key, name or password, a key or account that may not list users, an address where nothing answers, or one where something other than Jellyfin answers.

<a id="reading-each-user-s-watch-data"></a><a id="reading-each-users-watch-data"></a>

### Reading the watch data of each user

- A key from the server's dashboard belongs to no user: it reads every user's watch data.
- A user's own key, session access token or account reads only that user's data. Polyfin asks the server whose key it is and names the owner on the page.
- When that user is not a server administrator, Polyfin lists and imports only them, and says so. To list other users, connect with an administrator's account or an API key. Emby, which refuses users' own keys, follows the same rule with a user account.

Jellyfin refuses a user's key other users' data. Some servers that speak its API instead answer with that user's data, whoever is asked, which would mix histories. So, connected with an administrator's account or own key, Polyfin signs in as each other user:

- With **Import watch data** ticked, their row asks for their **Password on …** the server, empty if they have none.
- Polyfin signs in as them when the import starts, reads their data, then signs out.
- Without their password, untick **Import watch data**: their account can still be created.

For a new account, **Keep this password in Polyfin** keeps that password, so the user signs in as before. Polyfin passwords need at least 8 characters; a shorter one is refused and the row asks for another.

### Choosing who to import

**Import into** sets where each Jellyfin user's data goes:

- **New user**: a Polyfin account with the Jellyfin name, administrator or hidden from the sign-in screen if they were. Jellyfin gives out no passwords: set one under **Create a user**'s rules, or keep the user's own (see above). The name can be changed.
- An existing Polyfin user: the one with the same name, in any case, is chosen at first. Several Jellyfin users can go into one account.
- **Do not import**: the user is left out. Disabled Jellyfin users start here.

**Import watch data** says, per user, whether watch data comes along. A new account can be created without it.

**Start import** creates all new accounts or none. If a name is taken, a password is too short, or the server refuses a user's password, nothing is created and the row says why. Watch data then imports in the background.

### What is imported

| From Jellyfin | Into Polyfin |
|---|---|
| User names, administrator status, hidden from sign-in | New accounts, with passwords the administrator sets or keeps. |
| Played movies and episodes, last played date, play count | Played marks. |
| Resume points of movies and episodes | Resume points, for unplayed titles. |
| Favorite movies, series and episodes | Favorites. |
| Each user's 0 to 10 ratings of movies, series and episodes, with **Ratings** | Ratings; existing Polyfin ratings stay. A thumb up without a rating becomes a favorite. |
| Server collections, with **Collections** | Collections every user sees, with the titles found. |
| Each user's video playlists, with **Playlists** | That user's playlists, in order, with the titles found. |

Nothing else comes over: user limits and settings, profile pictures, music, books and Live TV.

### Ratings, collections and playlists

**Also bring**, under the users, offers **Ratings**, **Collections** and **Playlists**, all ticked by default. Unticked ones are not read.

- **Ratings**: a 0 to 10 rating comes as is. An Emby thumb up without a rating becomes a favorite, like Letterboxd's liked movies (see [Letterboxd](#letterboxd)). A thumb down alone brings nothing. A rating Polyfin already has stays, in every import.
- **Collections**: each becomes a Polyfin collection every user sees (see [Collections made by users](collections-people-and-ratings.md#collections-made-by-users)), with the titles found in the server's order. Titles not found are listed with the others. One with no title found is not created, and the result names it. One several users see is created by the first and gets the next ones' titles added.
- **Playlists**: each video playlist a user sees becomes their own Polyfin playlist, with its movies and episodes found, in order, repeats kept. Other entries, and music, photo or book playlists, are left out. One with no title found is not created, and the result names it. One several users see, such as a shared one, becomes a playlist of each.

Polyfin remembers each created list's source, by the server's and the list's identifiers. Importing again creates nothing twice:

- A collection gets the titles found it lacks; a playlist gets, at its end, those it does not hold at all.
- The result counts these as added to lists brought before, and a list already holding every title as already in Polyfin.
- A list deleted in Polyfin since is created again.

**Compared with Jellyfin**

- Jellyfin keeps one rating per user and title and derives the thumb: up from 6.5; setting a thumb sets 10 or 1 ([`UserItemData.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/UserItemData.cs#L96-L118), Jellyfin 12.4). Its API lists liked titles (`Filters=Likes`, [`InternalItemsQuery.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/InternalItemsQuery.cs#L599-L614)), but no list holds only lower-rated ones. Those come only when played, in progress or favorite, which Polyfin reads anyway.
- Jellyfin's collections are seen by everyone who sees their titles, like Polyfin's. Its playlists belong to one user, and can be shared or open to everyone ([`PlaylistsController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/PlaylistsController.cs#L509-L568)). Polyfin gives each user their own copy of a shared playlist, without the sharing.
- A user's own import (see [Importing your own watch history](#importing-your-own-watch-history)) creates collections only for users who may manage collections, as in Jellyfin.

### How titles are matched

Titles are found as an [imported watch history](tracking.md#importing-your-watch-history) finds them: by Jellyfin's IMDb identifier, as the usual metadata addons name titles, or by TMDB, or TVDB for series, when a catalog listed them so. Episodes are found by their series' identifiers and season and episode numbers. A multi-episode file counts for each episode.

A title no catalog listed yet shows in the user's lists at once, described by the addons. One known only by TMDB, or a series only by TVDB, is asked of the metadata addons that describe titles by it (`tmdb:<id>`, `tvdb:<id>`). It is found by the other identifiers they give, else shows under `tmdb:<id>` or `tvdb:<id>`.

An unmatched title is left out and the import goes on. Each user's result lists it with why:

- **No IMDb, TMDB or TVDB identifier**: a home video, or an episode Jellyfin did not number.
- **Not in Polyfin**: no IMDb identifier, no catalog lists its TMDB or TVDB one, and no server metadata addon describes it by TMDB, or a series by TVDB.

Plex adds two more (see [Plex](#plex)).

<a id="how-the-data-merges-with-polyfin-s"></a><a id="how-the-data-merges-with-polyfins"></a>

### How the data merges with Polyfin data

As for an imported watch history (see [How the history merges with Polyfin's](tracking.md#how-the-history-merges-with-polyfin-s)):

- Played marks, resume points and favorites are only added, never removed.
- A title keeps Polyfin's date when it is later, and the larger play count.
- Of two resume points, the more recent stays.
- Importing again adds nothing twice and never overwrites newer data, so it is safe to rerun.

Nothing is written to Jellyfin. Signing in opens a session there, as an app does, which Polyfin ends as soon as it is done.

### Progress and limits

- One import runs at a time. Leaving the page does not stop it.
- The page shows progress per user, then the counts: items read, titles marked played, resume points, favorites, titles rated, lists created, what Polyfin already had, titles not found, and lists not created for lack of titles found.
- **Stop import** stops at once. Imported data stays. A user being saved is saved whole; a user being read gets nothing until the next import.
- Jellyfin is read 200 titles per request, one request at a time. A failed request is retried 3 times.
- A server that keeps failing, or refuses the key, ends the import after saving what was read of the current user.
- A user the key may not read is marked so, and the import moves on.
- A resume point without a date is counted, not added, since Polyfin cannot tell if its own is newer. Jellyfin dates every resume point; Plex dates those a player saved.
- An unreadable answer ends the import with `not_jellyfin`. The log says `An import could not read an answer` at warning level, with the path and why, never the query or key.
- After each user is saved, the log says `An import read a user's watch data` at info level, with the user's server identifier and:
  - for Plex, the library `sections`, items `listed`, and for an account other than the owner, `history` rows and `otherAccounts` rows left out;
  - then `read`, `matched`, `played`, `resumed`, `favorites`, `ratings`, `watchlist`, `collections`, `playlists`, `listTitles`, `skippedLists`, `upToDate`, `undated`, `unmatched`, `nothingAdded` and `problem`.
  - Never the server's address or the key.
- Each signed-in user is signed out once read; the import's own session when it ends, stops or fails. A restart mid-import leaves them open, listed among the users' devices on the server, which can end them.
- The running import and last result live in memory only: a restart stops the import and forgets the result. Start again: it adds only what is missing.

#### Nothing imported?

A user whose import added nothing, without a problem, says why under the counts:

| Message | Code | Meaning |
|---|---|---|
| **Everything read was already in Polyfin** | `up_to_date` | An earlier import, or Polyfin itself, has it all. |
| **The server lists nothing played, in progress or favorite for this user** | `nothing_played` | Jellyfin and Emby. |
| **This Plex server has no movie or show library: there was nothing to read.** | `no_sections` | Plex. |
| "The owner has nothing played or in progress in this server's movie and show libraries." | `plex_owner_nothing_watched` | Plex. A user's own Plex import says the same of their account as `plex_nothing_watched` (see [Plex, signing in](#plex-signing-in)). |
| **Plex lists no play of this account on this server** | `plex_no_history` | See [Plex](#plex). All comes over if the administrator signs in as owner (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)) or the person signs in to Plex (see [Plex, signing in](#plex-signing-in)). |

Titles read but not matched show only in the titles not found, with why.

**Compared with Jellyfin**

- Jellyfin keeps watch data per library file; Polyfin per catalog title. So titles match by identifier, not file.

API details: [Server import](jellyfin-api.md#server-import-for-apps).

## Moving from Emby and Plex

**Server**, under **Import from another server**, also offers **Emby** and **Plex**. Accounts are chosen, created and filled as for Jellyfin (see [Choosing who to import](#choosing-who-to-import)). Titles match (see [How titles are matched](#how-titles-are-matched)) and data merges the same way. The result shows, per user, what was added and the titles not found.

### Emby

The flow is Jellyfin's: **Server address**, **Connect with** an API key or a user account, then each user's **Password on …** the server when connected as one user.

- **API key**: create one in Emby's settings, under API Keys. It reads every user's watch data.
- A user's own key or access token is refused (`emby_user_key`). Emby cannot tell whose a key is, and answers it with its owner's data whoever is asked. Connect with a user account instead, so the user is known.
- After Polyfin signs out, Emby still lists it among its devices, without a session.
- Polyfin first reads the server's self-description, which Jellyfin and Emby give anyone. It sends the key or password only to the kind chosen: Jellyfin names itself, Emby does not. Choosing **Emby** for a Jellyfin server, or **Jellyfin** for Emby, stops with `not_jellyfin`.

The same data comes over as from Jellyfin: played movies and episodes with dates and play counts, resume points, favorites, and, as chosen, ratings, collections and playlists.

### Plex

- **Server address**: Plex Media Server's own address, such as `http://192.168.1.10:32400`.
- **Plex token**: the server owner's token. Open Plex in a browser as the owner, open any movie's menu, choose Get Info, then View XML. The page address ends with `X-Plex-Token=` and the token.

The users listed are the accounts the server knows, the owner first, marked as administrator.

| From Plex | Into Polyfin |
|---|---|
| Owner's played movies and episodes, play counts, last played date | Played marks. |
| Owner's resume points | Resume points, for unplayed titles. |
| Owner's 0 to 10 ratings of movies, shows and episodes, with **Ratings** | Ratings, as from Letterboxd; existing ratings stay. |
| Another account's playback history here: movies and episodes played to the end, play count, last play date | Played marks. |
| Movie and show section collections, with **Collections** | Collections, as Jellyfin's (see [Ratings, collections and playlists](#ratings-collections-and-playlists)), read once with the owner's token. |
| Owner's video playlists, with **Playlists** | The owner's playlists. |

The owner's token gives the owner's watch state, but only the playback history of other accounts on this server. That history holds only titles played to the end on this server. It lacks:

- titles marked watched without playing: since Plex Media Server 1.40, marking makes no history entry;
- titles synced from the account's Plex account, as when it gets access to a new server;
- resume points, which stay on Plex.

So an account showing watched titles in Plex may have nothing to import, and the page says Plex lists no play of it. Everything comes over if the administrator signs in as the owner (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)), or the person signs in under **My account** (see [Plex, signing in](#plex-signing-in)). Their own token reads marked and synced titles, resume points and ratings.

Plex has no favorites to import, and the owner's Watchlist is not imported here. History rows of an account other than the one asked are left out.

#### Signing in to Plex as the owner

Once connected, **Sign in with Plex as the owner** shows above the accounts. **Open Plex’s sign-in page** opens Plex's page in a new tab, where the administrator signs in to the owner account. Polyfin never sees the Plex password.

Polyfin checks with plex.tv that this account owns the connected server (same machine identifier). Another account is refused (**This Plex account does not own the server**, `plex_not_owner`), and the sign-in stays for another try or **Sign out of Plex**.

With the owner's account, Plex gives Polyfin each shared account's token for this server, the one its Plex apps use. Polyfin also switches to the profiles of the owner's Plex Home. Each account then shows what will be read:

| Account | Read with | What comes over |
|---|---|---|
| The owner | The owner's token, as without sign-in | Played titles, resume points, ratings and playlists. |
| A shared account | Its own token for the server | **Everything**: titles played, marked or synced as watched, resume points, ratings and playlists, as its own import (see [Plex, signing in](#plex-signing-in)), but not its Watchlist. |
| An owner's Plex Home profile | Its own token, switched to as Plex's apps do | **Everything**, as above. A PIN profile asks for it under **Profile PIN** in its row. A refused PIN stops the import before anyone is created (`plex_profile_pin_refused`). Without a PIN, it is read as the next row. |
| An account Plex gives no token for | The owner's token | **Plays only**: its playback history here, as without sign-in, with the reason when empty. |

Transparency: these tokens read only the server's libraries, as the accounts' own Plex apps do, and write nothing. They live in memory for this import only, never stored, logged nor answered. Once the import ends, the owner's and Home profiles' sign-ins are signed out of Plex. Shared accounts' tokens come from the server's sharing, not sign-ins, so they are just dropped. Without the sign-in, the import works as before.

Titles are found by the IMDb, TMDB and TVDB identifiers of their `Guid` list; episodes by their show's identifiers and season and episode numbers. Items an older Plex agent matched have no `Guid` list. Their `guid` names the identifier instead: `com.plexapp.agents.imdb://tt…`, `com.plexapp.agents.themoviedb://…`, `com.plexapp.agents.thetvdb://…`, or an agent's own scheme with `://tvdb-…`, `://tmdb-…` or `://imdb-tt…`. Two more reasons list titles as not found:

- **No longer in Plex's libraries**: a history names it, but no library section lists it, so it has no identifier.
- **Numbered in another of TheTVDB's episode orders**: an episode of a show an older agent matched as `tvdb2-`, `tvdb3-`…, not TheTVDB's usual numbers. The show is known, but marking the episode by its numbers could mark the wrong one.

Plex is only read, with `GET` requests, never written to.

API details: [Emby and Plex import](jellyfin-api.md#emby-and-plex-import-for-apps).

## Importing your own watch history

**Import from another server**, under **My account**, lets each user import their own history from a Jellyfin, Emby or Plex server when an administrator could not. Examples: a server without API keys, unknown passwords, or a Plex server where the owner's token reads only other accounts' plays (see [Plex](#plex)). It also takes a Letterboxd export file (see [Letterboxd](#letterboxd)).

- **Server**: **Jellyfin**, **Emby**, **Plex** (see [Plex, signing in](#plex-signing-in)), or **Letterboxd (export file)**.
- **Server address**, **Your user name there** and **Your password there**, empty if none.

**Import my watch history** signs in as the user, as the server's apps do. It imports into their own Polyfin account only: played movies and episodes, resume points and favorites. **Also bring** adds their ratings, the server's collections and their playlists, as for an administrator (see [Ratings, collections and playlists](#ratings-collections-and-playlists)). **Collections** shows only for users who may manage collections. Matching and merging are the administrator's. The section then shows the import and its result: what was added and the titles not found.

Polyfin never lists the server's users, which a user's session may not. It reads only the signed-in user, even from a server that answers everyone with the session's data. It signs out when done. The password serves this import only and is never kept.

- One server import runs at a time on the whole server, and one import per account, from a server or Letterboxd. Another is refused meanwhile (`server_import_running`): try again a few minutes later.
- Users see only their own imports: the section shows their latest, from a server or Letterboxd. The administrator's **Import from another server** page shows the running or last server import, and who started it.
- **Users may import their watch history from another server or app**, under **Settings › Security**, is on by default. Off, the section is hidden, Letterboxd included, the API refuses (`server_imports_disabled`), and the import from an app is off too (see [Importing from an app](#importing-from-an-app)).

API details: [Own watch history import](jellyfin-api.md#own-watch-history-import-for-apps).

### Plex, signing in

Choose **Plex**, then **Sign in with Plex**. **Open Plex’s sign-in page** opens Plex's page in a new tab to sign in, as on any Plex app. Polyfin never sees the Plex password. Then the section names the account and lists the Plex servers it reaches, the one administrators last imported from first.

- **Plex Home profile**, for a Plex Home account: whose data comes over, the signed-in account first. A managed profile has no sign-in of its own, as Plex says (see Plex's support article "Managed Accounts"). A Home member signs in, picks it here with its **Profile PIN** if any, and **Use this profile** switches as Plex's apps do.
- **Server**: the server to import from. Polyfin tries the addresses Plex gives: local network first, then remote, then Plex relays. Each is an address the server import takes. One counts only when the server answers with its own machine identifier.
- **Watchlist to my Watchlist** (on by default): the movies and shows of the profile's Plex Watchlist, kept by Plex with the account, not a server, go to the user's [Watchlist](tracking.md#the-watchlist). Like a one-off app import's library, they stay until started.

**Import my watch history** reads the server with the profile's own token, as the owner's import reads the owner, into the user's own account only:

| From Plex | Into Polyfin |
|---|---|
| Movies and episodes played, marked or synced as watched, play counts, last played date | Played marks. |
| Resume points | Resume points, for unplayed titles. |
| 0 to 10 ratings of movies, shows and episodes, with **Ratings** | Ratings, as from Letterboxd; existing ratings stay. Apps see `Likes` from 5. Season ratings have no Polyfin title and stay on Plex. |
| Server movie and show section collections, with **Collections**, for users who may manage collections | Collections every user sees, as in the administrator's import. |
| Profile's video playlists, with **Playlists** | The user's playlists, in order. |
| The Watchlist, when ticked | The Watchlist. |

An account with nothing played, in progress or rated there says so (`plex_nothing_watched`). Titles match as in the administrator's Plex import. A Watchlist item lacking identifiers in Plex's list is asked of Plex one by one, 500 at most, and listed as not found if it has none.

About the tokens:

- Plex gives three: the account's, which only lists profiles and servers, switches profiles and reads the Watchlist; the chosen profile's; and the server's.
- They stay in memory for the signed-in user only, never stored, logged nor answered.
- They are dropped 10 minutes after the last choice, or when the import ends. Polyfin then signs them out of Plex, removing it from the account's authorized devices.
- Polyfin names itself to Plex with one identifier, kept with the server's identity, so repeated sign-ins reuse one entry. If an entry stays, as after a restart mid-import, remove Polyfin from the authorized devices in Plex's settings.
- The administrator's owner sign-in is held and signed out the same way.

**Compared with the administrator's Plex import**

- The owner's token reads only the plays Plex recorded for other accounts. The profile's own token reads its whole watched state, resume points, ratings and Watchlist.
- With the owner signed in to Plex, the administrator's import also reads accounts with their own tokens, but not their Watchlists (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)). Without it, each non-owner account that gave nothing is told to import this way.

API details: [Plex sign-in](jellyfin-api.md#plex-sign-in-for-apps).

### Letterboxd

Letterboxd has no API for this, but gives members their data as a file: Settings › Import & Export, then Export your data, downloads a ZIP of CSV files. Choose **Letterboxd (export file)**, pick the ZIP as is, 20 MB at most, under **Export file**, tick the extras, and choose **Import from Letterboxd**.

| Option | Default | What it does |
|---|---|---|
| **Watchlist to my Watchlist** | On | Letterboxd watchlist movies go to the user's [Watchlist](tracking.md#the-watchlist), until started, like a one-off app import's library. |
| **Ratings** | On | Stars × 2 give the 0 to 10 rating, so 3.5 stars is 7. Apps see `Rating` and `Likes` (5 or more) as for their own ratings. A movie rated only in the diary takes its latest diary rating. |
| **Liked movies as favorites** | Off | Liked movies become favorites. |

Movies in `watched.csv` and `diary.csv` are marked played. The date is the latest diary `Watched Date`, else the `watched.csv` date. The play count is the number of diary lines, at least 1. Diary lines join movies by name and year, as their links point to the diary entry.

Polyfin reads only those files, `ratings.csv`, `watchlist.csv` and the liked movies' file in `likes`, by column name, so new Letterboxd columns do no harm. Reviews, lists and the profile are not read.

- No `watched.csv` or `diary.csv` at the ZIP's root or in their folder: refused (`letterboxd_export_invalid`).
- Over 20 MB, or over 50,000 movies: refused (`letterboxd_export_too_large`).
- A line whose name, link, year, date or rating exceeds 512 bytes is skipped.
- The file is read in memory, never written to disk, and dropped once read.

**Matching:** The export gives only name, year and Letterboxd link. Each movie is searched by name in the server's metadata addons, as local files are (see [Matching](naming-and-matching.md#matching)). It matches only if exactly one movie has that name and year. Searches run one at a time, once per name per import, whatever file it is in. Without a metadata addon that searches movies, nothing can match, and the form says so before upload. The result lists unmatched movies with their Letterboxd link and why: **Not found** (no movie of that name and year, even one listed under another year or name), **Several movies of that name and year**, or **Search failed**.

**Merging:** As the other imports (see [How the data merges with Polyfin data](#how-the-data-merges-with-polyfin-data)): played marks, ratings and favorites are only added, Polyfin's later date and the larger play count stay, and existing ratings stay. The same export imported again adds nothing twice.

**Progress:** One import per account at a time. An export sent while another uploads or imports into that account is refused (`server_import_running`) before being read. During searching, the section shows how many movies were searched. Then it counts movies marked played, Watchlist additions, and ratings and favorites set, and lists movies not found. The import and result stay in memory, like an app import's (see [Progress of an app import](#progress-of-an-app-import)).

An administrator can run it for a user: **Import from Letterboxd**, at the top of **Users**, asks **Into the account of** which user, then takes the file the same way. Turning off users' own imports does not turn off the administrator's.

API details: [Letterboxd import](jellyfin-api.md#letterboxd-import-for-apps).

## Importing from an app

Apps built on addons, like Polyfin, keep users' watch history in an account. Polyfin imports from Nuvio and Stremio. **Import from an app**, under **My account**, brings over played movies and episodes, resume points, the app's library, and chosen addons.

1. Choose the **App**, tick **Stay connected** to re-import automatically, every 6 hours by default (see [Staying connected](#staying-connected)), and sign in as that app does (see [Nuvio](#nuvio) and [Stremio](#stremio)). Polyfin reads the account once, then signs out, unless you paste a Stremio key or stay connected.
2. **Profile** lists the profiles by name, with what each watched, resumed and keeps in its library: choose one. A Stremio account without profiles has **The whole account**. A Stremio profile with a PIN asks for it before showing its contents (see [Stremio](#stremio)). Under **Addons**, tick any to add (see [Addons from the app](#addons-from-the-app)).
3. **Import this profile** imports in the background. The section then shows the result: what was added, titles not found, and addons added.

What Polyfin read stays in memory only while the profile is chosen, 5 minutes at most. After that, or **Sign in again**, it is gone and the import asks to sign in again (`app_import_expired`).

- The password serves that sign-in only, never kept nor logged.
- The app's key or session also serves that sign-in only, unless you stay connected or a Stremio profile asks for its PIN, which needs it meanwhile.
- A session Polyfin opened for a connection or a PIN is signed out if not imported within those 5 minutes, or if Polyfin stops or the user is deleted meanwhile.

An administrator can import for a user: **Import from an app**, at the top of **Users**, asks **Into the account of** which user, then continues as the user's import, with the user's app sign-in. **Users may import their watch history from another server or app**, under **Settings › Security**, turns off only users' own imports (`server_imports_disabled`).

### What comes over from an app

| From the app | Into Polyfin |
|---|---|
| Movies and episodes watched, with the date when the app keeps one | Played marks. |
| Resume points of movies and episodes | Resume points, for titles not played. |
| The library's movies and series | The [Watchlist](tracking.md#the-watchlist). |
| The addons ticked | The user's own addons. |

An app's library is a watchlist: its titles go on the user's Watchlist, which Jellyfin apps show beside the libraries, never in favorites. A one-off import's titles stay until started. A connection's import replaces what it put there before, so a title removed from the app's library leaves at the next import, unless another list holds it. Imports set no favorites, and those earlier imports set stay.

Nothing else comes over: settings, collections, home screen layouts, and device-only data.

<a id="how-an-app-s-titles-are-matched"></a><a id="how-an-apps-titles-are-matched"></a>

### How app titles are matched

Addon-based apps name titles by their addons' identifiers. Polyfin finds IMDb (`tt…`) and TMDB (`tmdb:…`) ones as an [imported watch history](tracking.md#importing-your-watch-history) does, and Kitsu ones (`kitsu:…`) through the [anime mapping](tracking.md#anime) and as anime catalogs list them. An episode is found by its series' identifier and season and episode numbers, or for Kitsu, by its number in the entry.

A title no catalog listed yet shows in the user's lists at once, described by the addons. An unlisted `tmdb:…` title is asked of the metadata addons that describe titles by it. It is found by the IMDb and TVDB identifiers they give, else shows under its `tmdb:…` identifier, with episodes as described.

Unmatched titles are left out and the import goes on. The result lists them with why:

- **Not an IMDb, TMDB or Kitsu identifier**: an addon's own identifier.
- **Not in Polyfin**: no catalog lists its identifier, and for `tmdb:…`, no server metadata addon describes it.
- **Its episodes do not match Polyfin's list**: a Stremio series (see [Stremio](#stremio)). Its watched episodes are not marked, but it still goes on the Watchlist if in the library.

Data merges as in the other imports (see [How the data merges with Polyfin data](#how-the-data-merges-with-polyfin-data)). Played marks and resume points are only added, Polyfin's later date stays, and the more recent resume point stays. Resume points follow Polyfin's thresholds (see [Played and resume thresholds](watch-state.md#played-and-resume-thresholds)): one past the played threshold marks the title played. Re-importing adds nothing twice. The result counts library titles added to the Watchlist: those not already there and not started.

<a id="the-app-s-addons"></a><a id="the-apps-addons"></a>

### Addons from the app

When **Allow users' own addons** and the user's **Can add their own addons** allow it (see [Users' own addons](sign-in-and-security.md#users-own-addons)), **Addons** offers the profile's addons by name. Each ticked one becomes the user's own addon under **My sources**, as if added there. Otherwise none are offered, and requesting some is refused (`personal_addons_disabled`).

An addon address may hold the user's settings and keys. Polyfin shows only its host, never logs it, and keeps it only as the user's addon. A user's own import reaches public addresses only, like their addons; an administrator's may reach the local network. The result says per addon whether it was added, or why not: already installed, on a local network, not answering, or not an addon.

### Progress of an app import

- One import per account at a time; another is refused meanwhile (`app_import_running`). Different accounts import side by side.
- Leaving the page does not stop an import. The running import and each account's last result live in memory only: a restart stops the import and forgets the result. Start again: it adds only what is missing.
- Users see only imports into their own account. An administrator's page shows the chosen user's import and who started it.

### Staying connected

Someone watching in Nuvio or Stremio on some devices and Polyfin on others can stay connected instead of re-importing by hand. Tick **Stay connected** before signing in: Polyfin keeps the account access, and the next import starts a connection. It then re-imports the same way as often as **Import connected accounts every** says, every 6 hours by default (see [How often connected accounts are imported](#how-often-connected-accounts-are-imported)).

**What is kept:**

| App | Kept |
|---|---|
| Stremio | The account's auth key. One got by link code or password is not signed out after the import; a pasted key is kept as is. For a PIN profile, also the session opened with the PIN, never the PIN. |
| Nuvio | The backend's address, the chosen profile, and the session's refresh token, saved again whenever Nuvio renews it at an import. |

The password is never kept. Each user has one connection per app. Signing in again with **Stay connected** replaces it and signs out the sessions Polyfin opened before.

**How it is protected:** It is stored encrypted with `POLYFIN_SECRET_KEY`, like tracking tokens (see [Configuration](configuration.md)). It is never shown, sent to the browser, or logged. Without the key it is stored as is, and **System › Health** warns. A connection the key cannot decrypt counts as none: sign in again.

**The imports:**

- Each interval, Polyfin re-reads the account and imports the profile chosen when connecting, by the first import's rules: played marks only added, newer resume point wins.
- The library replaces what the connection put on the Watchlist. A title removed there leaves the Watchlist unless another list holds it.
- Disconnecting removes from the Watchlist the titles only the connection held. Those a one-off import brought stay until started.
- Addons are chosen once, at the first import; later imports install none.
- Nothing is ever written to the app.

**Status:**

- **Connected apps**, under **Import from an app**, shows each connection: the account's email from the app, who connected it and when, the last import and what it added, and the next run.
- **Import now** imports at once.
- If the app refuses what Polyfin kept, such as a signed-out key or ended session, the connection stops and says so. It stays stopped until you sign in again with **Stay connected** ticked.
- An app that does not answer, or asks to wait, is retried at the next import, silently.
- If the chosen profile is gone, or a Stremio profile now asks for a PIN Polyfin lacks (set or changed since), the last import says so. Nothing imports until you sign in again to choose it.

**Told when it stops:** A stopping connection is told once to the user's notification targets and to the server's that chose **Connection stopped** (see [Stopped connections](notifications.md#stopped-connections)). **System › Health** lists stopped connections by user and app. A new sign-in followed by a new refusal tells again.

**Disconnecting:** **Disconnect** signs out the sessions Polyfin opened, then forgets everything kept. Those are the Stremio key got by link code or password, a Stremio profile's PIN session, and the Nuvio session. A pasted key is only forgotten, since signing it out would sign your browser out. Imported data stays, except Watchlist titles only the connection held. Deleting the user signs out the same sessions in the background (see [Deleting a user](users.md#deleting-a-user)).

**Who:**

- Users connect their own accounts while **Users may import their watch history from another server or app**, under **Settings › Security**, allows it. Off, their connections pause, still shown, and can still be disconnected.
- An administrator can connect an app for a user from **Import from an app**, at the top of **Users**. The page lists that user's connections with **Import now** and **Disconnect**.
- An administrator's own connections never pause.

Staying connected to Stremio raises the risk of reading it (see [Stremio](#stremio)). Its terms forbid automated data extraction, and repeated reading is plainer automation than a one-off import, more so at short intervals.

API details: [App connections](jellyfin-api.md#app-connections-for-apps).

### Nuvio

A Nuvio account has up to 6 profiles. Each keeps watched movies and episodes with dates, resume points, a library and addons. Polyfin reads all at once from the account backup that Nuvio's own scripts use to move accounts.

- **Nuvio backend**: empty for Nuvio's own, or a self-hosted backend's address, local network included.
- **Email** and **Password**: as in Nuvio's apps. **Sign in to Nuvio** signs in, reads the account, and signs out unless you stay connected.

The page says so for a wrong email or password, an address where nothing answers, or one where something other than a Nuvio backend answers. If Nuvio asks to wait before signing in again, Polyfin waits up to 30 seconds, twice at most, then says so. Polyfin reads backup version 1, and refuses other versions, saying so, until it supports them.

What comes over, per profile:

- Watched: each movie and episode, with the date marked watched, older apps' dates included. A series marked watched as a whole, without its episodes, is not marked played: Polyfin counts series as played from their episodes.
- Resume points: per movie and episode, with runtime and date, unless watched since. Those of titles from a Jellyfin or Emby server connected in Nuvio (`srv1:…`) stay in Nuvio.
- Library: movies and series, on the Watchlist from the date Nuvio added them.
- Addons: those on in the profile, or in the first profile for a profile using its addons. Nuvio's plugins are not addons and do not come over.

Guest data, kept without an account, stays on the device. Progress Nuvio sends to Trakt or Simkl is imported from there (see [Tracking services](tracking.md)).

API details: [Nuvio import](jellyfin-api.md#nuvio-import-for-apps).

### Stremio

A Stremio account keeps the user's installed addons and a library. For each title, the library says whether and when it was last watched, the resume point of the last video played, and whether it is in the user's library. Stremio supporters have up to 6 profiles, each with its own library and addons. An account without profiles keeps one for the whole account. Polyfin reads it all at once, with the calls Stremio's apps make.

**Sign in with** offers three ways; the first works for every account:

- **A link code**: **Get a link code** asks Stremio for a code, as Stremio's TV apps sign in, and shows it with **Open the link**. Open the link in a browser signed in to Stremio, check the code, and approve it. The page checks every few seconds for approval, then reads the account and signs out, unless you stay connected. A code lasts 5 minutes; then ask for another. It works with any sign-in, Facebook and Apple included, and Polyfin never sees the password.
- **Email and password**: as in Stremio's apps. **Sign in to Stremio** signs in, reads the account, and signs out unless you stay connected. Facebook or Apple accounts have no password: use a link code.
- **An auth key**: the key Stremio's web app keeps after you sign in there. Run `JSON.parse(localStorage.getItem("profile")).auth.key` in the browser console on that page. **Read the account** reads with it and does not sign it out, which would sign your browser out too.

The key Polyfin gets or is given serves this one read. It stays in memory for the read only, never logged nor sent to the browser. It is stored, encrypted, only to stay connected (see [Staying connected](#staying-connected)). A wrong email or password, a refused key, or Stremio not answering each say so.

What comes over:

- Watched movies: each movie Stremio counts as watched, or saw watched most of the way, with the last watched date, else the last change date, and how many times.
- Watched episodes: Stremio stores a series' episodes as a row of bits, one per episode, in the order the user's app listed them, anchored on the last watched episode. Polyfin matches the row to the server addons' episode list in the same order: by season, episode, then release date, unnumbered episodes first. It shifts on the anchor, so episodes listed since before it still fall in place. Stremio keeps no date per episode: each takes the series' last watched date.
- Resume points: one per title, at the last video played, movie or episode, with runtime and date, while the title shows in Continue Watching: in the library, or played without being added. A title removed from the library keeps its watched marks, not its resume point.
- Library: movies and series, on the Watchlist from the date Stremio first kept them. Titles only played, or removed from the library, are not on it.
- Addons: those the user installed. Stremio's built-in ones are left out.

Channels, TV channels and other videos do not come over.

The row of bits only makes sense against the episode list the user's app had:

- If the server's addons do not list the anchor episode, no episode of the series is marked played. The result lists the series with **Its episodes do not match Polyfin's list**.
- If they list other episodes after the anchor, or other specials, the wrong episodes can be marked, silently. The server's metadata addons should list series as the user's app did.

Profiles: **Profile** lists a supporter account's profiles by name, in Stremio's order, with what each watched, resumed and keeps in its library. **Addons** offers the chosen profile's addons. Polyfin reads profiles as Stremio's apps switch to them: the account's own with the account key, others with their own session Stremio opens for the account, signed out once read. An account without profiles keeps **The whole account**.

- A PIN profile shows "(PIN)" after its name, without counts. **PIN**, then **Open this profile**, reads it. The PIN goes only to Stremio, to open the profile's session. Polyfin never keeps, logs nor returns it. A wrong PIN says so (`stremio_pin_refused`).
- Until a profile is imported, or **Sign in again**, the account's session stays open for 5 minutes at most, then is signed out, except a pasted key.
- **Staying connected** re-imports the same profile each time. A profile without a PIN is reopened per import, then signed out. For a PIN profile, Polyfin keeps the session opened with the PIN, never the PIN, and reads with it.
- If the profile is removed, the last import says it is gone. If a PIN is set since, or Stremio refuses the kept session, as a new PIN may end it, the last import asks to sign in again and choose it with its PIN. Either way, nothing imports until you sign in again.
- Connections made before Polyfin read profiles keep using the account key, so they read the account's own profile.

Reading a Stremio account has risks:

- Stremio's account API is undocumented. Polyfin calls it as Stremio's apps do; Stremio may change it, or block other clients, without notice.
- Profile support was built from Stremio's web app alone, untested on an account with several. That each profile's library and addons need its own session, and that a PIN change ends it, is how that app behaves as far as it shows.
- Stremio's General Terms (section 5.4.1) forbid using Stremio for "monitoring, copying or extracting any information from Stremio, including its contents, graphic user interface or Database whether done mechanically or by automated means". Reading your own account with your own key is a grey zone: nothing says it is allowed.
- Staying connected re-reads the account at the set interval, every 6 hours by default, which deepens that grey zone, more so at short intervals.
- Stremio's own account data export, in its settings, is reported broken: its library repeats one title many times (Stremio/stremio-bugs#614). Polyfin does not read it.

Users who connected Trakt in Stremio can also import from Trakt (see [Tracking services](tracking.md)).

API details: [Stremio import](jellyfin-api.md#stremio-import-for-apps).

## How often connected accounts are imported

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Import connected accounts every** | **Settings › Security** | 6 hours | How often connected accounts are imported again, from 15 minutes to 24 hours. |

It applies to:

- Nuvio and Stremio accounts users stay connected to (see [Staying connected](#staying-connected));
- watch histories users import from tracking services (see [Importing your watch history](tracking.md#importing-your-watch-history));
- AniList and MyAnimeList lists: **Import my AniList lists**, **Import my MyAnimeList lists**, their **Plan to Watch** switches, and **Sync my ratings**, which imports their scores onto unrated titles each time (see [AniList and MyAnimeList](tracking.md#anilist-and-myanimelist)). All are off by default.

With a short interval, someone who watches in one of those apps finds their progress sooner in a Jellyfin app. **Import now** brings it at once.

- A change applies at once. Each connection's next import comes that long after its last import, or after connecting. A shorter interval brings due imports forward; a longer one delays them.
- The next import shown in **Connected apps** and **My account › Tracking** follows it. Polyfin checks for due imports every minute.
- Each import re-reads the whole account, so a short interval means more requests. Polyfin still waits when asked and keeps each service's pace (see [Pace and problems](tracking.md#pace-and-problems)).
- Stremio's terms forbid automated data extraction: the more often Polyfin reads a Stremio account, the deeper that grey zone (see [Stremio](#stremio)).

API details: [Connected account import interval](jellyfin-api.md#connected-account-import-interval-for-apps).
