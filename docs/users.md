# Users and permissions

This page covers user accounts: what Polyfin keeps for each user, and the limits an administrator can set on them. It also covers the server-wide security settings.

## Accounts and permissions overview

Polyfin has separate accounts for each user, with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points and Next Up are tracked per user, and each user can be limited to titles up to a rating.

An administrator creates users with **Create a user** at the top of **Users**, and sets each user's limits on that user's own page, opened from the list. The page has these sections: **Name and password**, **Access**, **Playback and access**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours** and **Devices**. **Access** switches save as soon as they change, and its **Ends on** date with **Save date** (see [Ending an account](#ending-an-account)); each other section has its own save button, such as **Save playback and access**. Signing out a device asks first. Administrators' Jellyfin apps can set most of the same limits from the user's settings. Each section below says where a setting lives.

Each user can also connect their own tracking accounts; see [Tracking services](tracking.md). Moving from a Jellyfin, Emby or Plex server, an administrator can bring its accounts and what each user watched over with **Import from another server**; see [Moving from Jellyfin](#moving-from-jellyfin) and [Moving from Emby and Plex](#moving-from-emby-and-plex). Users can import their own watch history too; see [Importing your own watch history](#importing-your-own-watch-history). What users watched in an app built on addons, such as Stremio or Nuvio, comes over with **Import from an app**; see [Importing from an app](#importing-from-an-app). Rather than choosing a password for each person, an administrator can also send them an invite link, with which they create their own account; see [Invite links](#invite-links).

### Deleting a user

**Delete user**, on a user's page, deletes the account with its devices and sessions, and everything kept for it; deleting the user from a Jellyfin app does the same. Polyfin also signs out, in the background, what it opened for that user at apps and services outside Polyfin:

- the Stremio auth keys it got by a link code or a password, and the Nuvio sessions it opened, its own only, for a connection (see [Staying connected](#staying-connected)) or for an account still held while the user chose what to import. A Stremio key the user pasted is only forgotten, as signing it out would sign their browser out;
- the Trakt and Simkl tokens of their tracking connections, which are revoked. The keys they typed, for MDBList, PublicMetaDB and ListenBrainz, and their Last.fm session key, are only forgotten (see [Tokens and keys](tracking.md#tokens-and-keys)).

The deletion never waits for an app or a service, and does not fail when one fails or does not answer: Polyfin logs a session it could not sign out, without its key or token, and that session stays valid there until it expires or the user signs it out there.

Stopping Polyfin also signs out the sessions it opened for the accounts held while users choose what to import, waiting 5 seconds at most.

## Watch state

Polyfin keeps each user's played titles, resume points, favorites and ratings. They follow the user from one Jellyfin app to another.

- Playback moves the resume point, and marks a title played near its end, with Jellyfin's thresholds (see [Played and resume thresholds](#played-and-resume-thresholds)).
- **Continue Watching** lists what is under way.
- **Next Up** lists the next episode of each series being watched.
- The **Upcoming** row lists the coming episodes of the series the user watches or marked favorite.
- Both look at the 50 series the user played most recently, as each series asks its addon for its episodes. A history imported from a tracking service can hold hundreds of series (see [Importing your watch history](tracking.md#importing-your-watch-history)).
- Marking a series or a season played marks its released episodes.
- Apps that keep Jellyfin's live connection (WebSocket) open are told of these changes as they happen, from any of the user's apps.

Users can also make playlists of movies and episodes from their apps. A playlist can be private, shared with other users, or open to all.

### Played and resume thresholds

Two settings under **Settings › Content** decide when a title counts as played and when a resume point is kept.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Marked played after (%)** | **Settings › Content** | 90 (from 50 to 100) | A position past this marks the title played. |
| **Resume point kept after (%)** | **Settings › Content** | 5 (from 0 to 50) | A position before this keeps no resume point. |

- The resume threshold must be below the played one; Polyfin refuses one that is not.
- A title shorter than 5 minutes is still marked played as soon as it is past the resume threshold, as in Jellyfin.

**Compared with Jellyfin:**

- These settings replace Jellyfin's `MaxResumePct` and `MinResumePct`, and default to Jellyfin's 90 and 5.
- Jellyfin takes any values; Polyfin keeps them in the ranges above.

## Moving from Jellyfin

**Import from another server**, at the top of **Users** next to **Create a user**, brings a Jellyfin server's accounts over, and what each of their users watched. It sits under **Users** because it creates accounts and fills each account's watch state. Only administrators see it. **Server** chooses the kind of server: **Jellyfin**, for Jellyfin and the servers that speak its API, as described here, or **Emby** and **Plex** (see [Moving from Emby and Plex](#moving-from-emby-and-plex)).

The page shows the import running, or the last one, whoever started it: an administrator here, or a user importing their own watch history (see [Importing your own watch history](#importing-your-own-watch-history)).

### Connecting to the server

- **Server address**: the address Jellyfin opens at in a browser, such as `http://192.168.1.10:8096`, with the path it is served under if any. A host and port alone mean `http://`. Local network addresses work.
- **Connect with** chooses how Polyfin reads the server:
  - **An API key**: create one in Jellyfin's dashboard, under API Keys. It reads every user's watch data at once.
  - **A user account**: a **User name** and its **Password** on the server, empty if the account has none. Polyfin signs in as Jellyfin's apps do, which every server that speaks Jellyfin's API allows, even one without API keys, and signs out once done. An administrator's account lists every user. Any other account lists only itself, even on a server that shows every user to anyone: only that account can be imported.

Polyfin uses the key and passwords for this import only, and never saves them: each import asks for them again.

**Connect** lists the server's users, with their administrator status and whether they are disabled. A wrong key, name or password, a key or account that may not list the server's users, an address where nothing answers, and an address where something other than Jellyfin answers each say so.

### Reading each user's watch data

- A key from the server's dashboard belongs to no user: it reads every user's watch data.
- A user's own key, the access token of a user's session, or a user account belongs to that user, and reads only their watch data. Polyfin asks the server whose key it is, and names its owner on the page.
- When that user is not an administrator of the server, Polyfin lists and imports only them, and the page says so. To list the server's other users, connect with an administrator's account or an API key. Emby, whose users' own keys are refused, follows the same rule when connected with a user account.

Jellyfin refuses a user's key the other users' data. Some servers that speak Jellyfin's API answer it with that user's data whatever user is asked, which would put one user's history into another's account. So, connected with an administrator's account or own key, Polyfin reads each other user's watch data signed in as them: with **Import watch data** ticked, their row asks for their **Password on …** the server, left empty if their account has none. Polyfin signs in as them when the import starts, reads their watch data, then signs out. Without their password, untick **Import watch data**: their account can still be created.

For a new account, **Keep this password in Polyfin** gives it that same password, so the user signs in to Polyfin as before. Polyfin passwords have at least 8 characters: a shorter one is refused, and the row asks for another.

### Choosing who to import

For each Jellyfin user, **Import into** chooses where their data goes:

- **New user**: a Polyfin account is created, named as in Jellyfin, an administrator if they were one, hidden from the sign-in screen if they were. Jellyfin does not give out passwords: set one for each new account, under the same rules as **Create a user**, or keep the one the user signs in with (see above). The name can be changed too.
- An existing Polyfin user: the one with the same name, whatever its case, is chosen at first. Several Jellyfin users can go into one account.
- **Do not import**: the user is left out. Disabled Jellyfin users start so.

**Import watch data** tells, user by user, whether their watch data comes along; a new account can be created without it. **Start import** creates the new accounts, all of them or none: when one name is taken, one password is too short, or the server refuses a user's password, nothing is created, and the row says why. Then the watch data is imported in the background.

### What is imported

| From Jellyfin | Into Polyfin |
|---|---|
| User names, administrator status, hidden from the sign-in screen | New accounts, with the passwords the administrator sets or keeps. |
| Played movies and episodes, with the date last played and the play count | Played marks. |
| Resume points of movies and episodes | Resume points, for titles not played. |
| Favorite movies, series and episodes | Favorites. |

Nothing else comes over: ratings, playlists, collections, users' limits and settings, profile pictures, and music, books and Live TV.

### How titles are matched

Titles are found as an [imported watch history](tracking.md#importing-your-watch-history) finds them: by the IMDb identifier Jellyfin gives, the way the usual metadata addons name titles, or by TMDB, or by TVDB for series, when a catalog listed the title under one of those. Episodes are found by their series' identifiers and their season and episode numbers; a file holding several episodes counts for each of them. A title no catalog listed yet shows in the user's lists at once, described by the addons. One known by its TMDB identifier only, or a series by its TVDB one, is asked of the server's metadata addons that describe titles by it (`tmdb:<id>`, `tvdb:<id>`): it is found by the other identifiers their description gives, else shows under `tmdb:<id>` or `tvdb:<id>`.

A title that cannot be matched is left out, and the import goes on. Each user's result lists these titles with why: **No IMDb, TMDB or TVDB identifier** (a home video, or an episode Jellyfin did not number) or **Not in Polyfin** (it has no IMDb identifier, no catalog listed it under its TMDB or TVDB one, and no metadata addon of the server describes it by its TMDB one, or a series by its TVDB one).

### How the data merges with Polyfin's

The data merges as an imported watch history does (see [How the history merges with Polyfin's](tracking.md#how-the-history-merges-with-polyfin-s)):

- Played marks, resume points and favorites are only added, never removed.
- A title keeps Polyfin's date when it is later than Jellyfin's, and the larger of the two play counts.
- Of two resume points, the more recent one stays.
- Importing again adds nothing twice and never overwrites newer Polyfin data, so an import can safely run again.

Nothing is written to the Jellyfin server: the import only reads it. Signing in opens a session there, as an app does, which Polyfin ends as soon as it is done with it.

### Progress and limits

- One import runs at a time. Leaving the page does not stop it, and the page shows how far it is, user by user, then what it added: titles marked played, resume points and favorites, and the titles not found.
- **Stop import** stops it at once. What was imported stays: a user whose data was being saved is saved whole, and one whose data was being read gets nothing until the next import.
- Jellyfin is read 200 titles at a time, one request after the other. A request that fails is tried 3 times more; a server that keeps failing, or refuses the key, ends the import, after saving what was read of the current user. A user whose data the server does not let the key read is marked so, and the import goes on with the next users.
- A user signed in is signed out once their watch data is read, and the import's own session when it ends, stopped or failed. A restart during an import leaves them open: they show among the users' devices on the server, which can end them.
- Polyfin keeps the running import and the last result in memory only: a restart stops an import under way and forgets the result. Start it again: it adds only what is missing.

**Compared with Jellyfin:**

- Jellyfin keeps watch data for the files of its libraries; Polyfin keeps it for the titles of its catalogs. Titles are therefore matched by their identifiers, not by their files.

**For app developers:**

- The admin API serves the import to administrators:
  - `POST /admin/api/jellyfin-import/users` lists a server's users, given `kind` (`jellyfin`, the default, `emby` or `plex`), `address`, and `apiKey`, or `account` with `name` and `password`. It answers `server` with its `kind`, and `keyOwner`, the user the key belongs to or who signed in, `null` for a server's key. An account the server refuses is `jellyfin_sign_in_refused`, and one it does not let sign in `jellyfin_sign_in_forbidden`.
  - `POST /admin/api/jellyfin-import` takes the same, creates the accounts and starts the import. An entry's `jellyfinPassword`, given with `watchData`, has the import sign in as that user. Connected as a user, another user's watch data needs it: without it, the answer is `jellyfin_key_owner_only`. A password the server refuses is `jellyfin_password_refused`, an account it does not let sign in `jellyfin_sign_in_forbidden`, and a sign-in that opens another user's session `jellyfin_other_user`. Each names the `jellyfinId`, and comes before anyone is created.
  - `GET /admin/api/jellyfin-import` reads the import, and `POST /admin/api/jellyfin-import/stop` stops it. The import names its `server.kind`, `startedBy` (`id` and `name`), and `own`, set for a user's import of their own watch history.
  - While any import runs, starting another is `jellyfin_import_running` (409).
  - The key and passwords are never answered back.
- Polyfin reads Jellyfin with `GET` requests, sending the key or session token as `Authorization: MediaBrowser Token="…"`: `/System/Info/Public`, `/Users`, `/Users/Me` (which Jellyfin answers a server's key with an error, and a user's key with that user), and `/Users/{id}/Items` with `Recursive`, `IncludeItemTypes`, `Filters` (`IsPlayed`, `IsResumable`, `IsFavorite`) and `Fields=ProviderIds`, then the episodes' series by `Ids`.
- It signs in with `POST /Users/AuthenticateByName`, naming itself and a device of its own in the `Authorization` header as Jellyfin's apps do, and signs out with `POST /Sessions/Logout`.

## Moving from Emby and Plex

Under **Import from another server**, **Server** also offers **Emby** and **Plex**. Accounts are chosen, created and filled as for Jellyfin (see [Choosing who to import](#choosing-who-to-import)), titles are matched the same way (see [How titles are matched](#how-titles-are-matched)), and the data merges the same way. The result shows, user by user, what was added and the titles not found.

### Emby

The flow is Jellyfin's: **Server address**, then **Connect with** an API key or a user account, then each user's **Password on …** the server when connected as one user.

- **API key**: create one in Emby's settings, under API Keys. It reads every user's watch data.
- A user's own key or access token is refused (`emby_user_key`): Emby has no way to tell whose a key is, and it answers a user's key with that user's own data whatever user is asked. Connect with a user account instead: the user who signs in is known.
- Emby keeps listing Polyfin among its devices after Polyfin signs out, without a session.
- Polyfin first reads what the server says of itself, which Jellyfin and Emby tell anyone, and sends the key or password only to a server of the kind chosen: Jellyfin names itself, Emby does not. **Emby** chosen for a Jellyfin server, or **Jellyfin** for an Emby server, stops there with `not_jellyfin`: a server answered, but not of that kind.

What comes over is what comes over from Jellyfin: played movies and episodes with their dates and play counts, resume points, and favorites.

### Plex

- **Server address**: the address of Plex Media Server itself, such as `http://192.168.1.10:32400`.
- **Plex token**: the server owner's token. To find it, open Plex in a browser, signed in as the owner, open the menu of any movie, choose Get Info, then View XML: the address of the page that opens ends with `X-Plex-Token=` followed by the token.

The users listed are the accounts the server knows, the owner first, marked as an administrator.

| From Plex | Into Polyfin |
|---|---|
| The owner's played movies and episodes, with their play counts and the date last played | Played marks. |
| The owner's resume points | Resume points, for titles not played. |
| Another account's playback history: each movie and episode played, with the number of plays and the date of the last one | Played marks. |

Plex does not let the owner's token read the resume points of other accounts: they stay on Plex. Plex has no favorites to import, and its watchlist is not imported. Titles are found by the IMDb, TMDB and TVDB identifiers of their `Guid` list; an episode by its show's identifiers and its season and episode numbers. A title that left the libraries since it was played has no identifiers left, and is listed as not found.

Plex is only read, with `GET` requests: it is never written to.

**For app developers:**

- The import's API is Jellyfin's, with `kind` set to `emby` or `plex`. Plex takes its owner's token as `apiKey`, and no `account` nor `jellyfinPassword`.
- Emby is read under `/emby` when the address does not end with it, else at the address given, with the key in `X-Emby-Token`. The requests are Jellyfin's, but for `/Users/Me`, which Emby does not have: a key is taken only when `GET /Auth/Keys` lists it among the server's API keys. Lists ask for `Fields=ProviderIds,UserDataPlayCount,UserDataLastPlayedDate`, without which Emby leaves the play count and date out. Signing in sends the app and device in `X-Emby-Authorization`.
- Plex is read in JSON (`Accept: application/json`), with the token in `X-Plex-Token` and Polyfin's device in `X-Plex-Client-Identifier`: `/identity`, `/`, `/accounts` (account 1 is the owner, account 0 nobody), `/library/sections`, each movie and show section's `/library/sections/{key}/all` with `type=1` or `type=2` and `includeGuids=1`, the show sections' episodes with `type=4` for the owner, and `/status/sessions/history/all?accountID=…` for the other accounts, a page at a time with `X-Plex-Container-Start` and `X-Plex-Container-Size`.

## Importing your own watch history

Under **My account**, **Import from another server** lets each user bring their own watch history over from a Jellyfin or Emby server they used before, when an administrator could not: a server without API keys, or whose passwords the administrator does not know.

- **Server**: **Jellyfin** or **Emby**.
- **Server address**, then **Your user name there** and **Your password there**, empty if the account has none.

**Import my watch history** signs in to that server as the user, as its apps do, and imports that user's played movies and episodes, resume points and favorites into their own Polyfin account only. Titles are matched and merged as an administrator's import does. The section then shows the import, and its result: what was added, and the titles not found. Polyfin never lists the server's users, which a user's session may not, and reads only the user signed in: even a server that answers every user with the session's data gives them their own. It signs out once done. The password is used for that import only, and never kept.

- One import runs at a time on the whole server. While another runs, the user's or an administrator's, the import is refused (`server_import_running`): try again a few minutes later.
- A user sees only their own imports. The administrator's **Import from another server** page shows the import running or the last one, saying who started it.
- **Users may import their watch history from another server or app**, under **Settings › Security**, on by default, turns the section off: it is hidden, and the API refuses (`server_imports_disabled`). It turns off the import from an app too (see [Importing from an app](#importing-from-an-app)).
- Plex has no sign-in for its users: only its owner can import from it, under **Users**.

**For app developers:**

- `GET /admin/api/account/server-import` answers `enabled`, and `import`, the user's own import running or their last one, `null` when none.
- `POST /admin/api/account/server-import` takes `kind` (`jellyfin` or `emby`), `address`, `name` and `password`, and answers `import`. A refused name or password is `jellyfin_sign_in_refused`, an account the server does not let sign in `jellyfin_sign_in_forbidden`; the address errors are those of the administrator's import.
- Polyfin sends the sign-in, the items requests and the sign-out of the administrator's import, never `/Users`.

## Importing from an app

Apps built on addons, as Polyfin is, keep what their users watched in an account: Polyfin imports from Nuvio and Stremio. Under **My account**, **Import from an app** lets each user bring theirs over: the movies and episodes they played, their resume points, the app's library, and the app's addons they choose.

1. Choose the **App**, tick **Stay connected** to have it imported again on its own, every 6 hours by default (see [Staying connected](#staying-connected)), and sign in to it, the way that app signs in (see [Nuvio](#nuvio) and [Stremio](#stremio)). Polyfin reads the account once, then signs out, but for a Stremio key you paste, and unless you stay connected.
2. **Profile** lists the account's profiles by name, with what each watched, resumed and keeps in its library: choose the one to import. An app without profiles, as Stremio, has one: **The whole account**. Under **Addons**, tick those to add, if any (see [The app's addons](#the-app-s-addons)).
3. **Import this profile** imports it into the user's account, in the background. The section then shows the import, and its result: what was added, the titles not found, and the addons added.

Polyfin keeps what it read of the account in memory only, while the profile is chosen, and for 5 minutes at most: after that, or after **Sign in again**, it is gone, and the import asks to sign in again (`app_import_expired`). The password is used for that sign-in only: it is never kept, nor logged. The key or the session the app gives is used for that sign-in only too, unless you stay connected; a session Polyfin opened for a connection and that is not imported in those 5 minutes is signed out, as it is when Polyfin stops or the user is deleted meanwhile.

An administrator can run the same import for a user: **Import from an app**, at the top of **Users**, asks **Into the account of** which user, then goes on as the user's import does, with the user's sign-in to the app. **Users may import their watch history from another server or app**, under **Settings › Security**, turns the users' own import off (`server_imports_disabled`), not the administrator's.

### What comes over from an app

| From the app | Into Polyfin |
|---|---|
| Movies and episodes watched, with the date when the app keeps one | Played marks. |
| Resume points of movies and episodes | Resume points, for titles not played. |
| The library's movies and series | The [Watchlist](tracking.md#the-watchlist). |
| The addons ticked | The user's own addons. |

An app's library is a watchlist: its titles go on the user's Watchlist, the view Jellyfin apps show beside the libraries, never in the favorites. A one-off import adds its titles there until the user starts them; a connection's import replaces what the connection put there before, so a title taken off the app's library leaves at its next import, unless another list holds it. Favorites stay the user's own: imports set none, and those earlier imports set stay. Nothing else comes over: settings, collections, home screen layouts, and what the app keeps on the device only.

### How an app's titles are matched

Apps built on addons name titles by the identifiers of the addons that described them. Polyfin finds them by IMDb (`tt…`) and TMDB (`tmdb:…`) identifiers, as an [imported watch history](tracking.md#importing-your-watch-history) does, and by Kitsu identifiers (`kitsu:…`), through the [anime mapping](tracking.md#anime) and as anime catalogs list them. An episode is found by its series' identifier and its season and episode numbers; an episode of a Kitsu entry by its number in the entry. A title no catalog listed yet shows in the user's lists at once, described by the addons. One named `tmdb:…` that no catalog listed is asked of the server's metadata addons that describe titles by it: it is found by the IMDb and TVDB identifiers their description gives, else shows under its `tmdb:…` identifier, its episodes as the description lists them.

A title that cannot be matched is left out, and the import goes on. The result lists these titles with why: **Not an IMDb, TMDB or Kitsu identifier** (an addon's own identifier) or **Not in Polyfin** (no catalog listed it under its identifier, nor, for a `tmdb:…` one, does a metadata addon of the server describe it). A Stremio series can also be listed with **Its episodes do not match Polyfin's list** (see [Stremio](#stremio)): its episodes watched are not marked played, but the series goes on the Watchlist when it is in the library.

The data merges as the other imports' does (see [How the data merges with Polyfin's](#how-the-data-merges-with-polyfin-s)): played marks and resume points are only added, a title keeps Polyfin's date when it is later, the more recent of two resume points stays, and the resume points imported follow Polyfin's thresholds (see [Played and resume thresholds](#played-and-resume-thresholds)): one past the played threshold marks the title played. Importing again adds nothing twice. The result counts the titles of the library added to the Watchlist: those it did not hold, and the user has not started.

### The app's addons

When users' own addons are allowed, by **Allow users' own addons** and the user's **Can add their own addons** (see [Users' own addons](#users-own-addons)), **Addons** offers the profile's addons one by one, by name. Each one ticked becomes one of the user's own addons, under **My sources**, as if they had added it there. When they are not allowed, none are offered, and choosing some anyway is refused (`personal_addons_disabled`).

An addon's address may hold its user's settings and keys: Polyfin shows only its host, never logs it, and keeps it only as the user's addon. A user's own import reaches public addresses only, as their own addons do; an administrator's may reach the local network. The result tells, addon by addon, whether it was added, or why not: already installed, on a local network, not answering, or not an addon.

### Progress of an app import

- One import into an account runs at a time: while one runs, another is refused (`app_import_running`). Imports into different accounts run side by side.
- Leaving the page does not stop an import. Polyfin keeps the running import and the last result of each account in memory only: a restart stops an import under way and forgets the result. Start it again: it adds only what is missing.
- A user sees only the imports into their own account; an administrator's page shows the import into the user chosen, saying who started it.

### Staying connected

Someone who goes on watching in Nuvio or Stremio on some devices, and through Polyfin on others, can stay connected rather than import again by hand. **Stay connected**, ticked before signing in, keeps what reads the account again, and the import that follows becomes the first of a connection: Polyfin imports the account again as often as **Import connected accounts every** says, every 6 hours by default, the same way (see [How often connected accounts are imported](#how-often-connected-accounts-are-imported)).

- What is kept: for Stremio, the account's auth key: the one Polyfin got by a link code or a password is not signed out at the end of the import, and a key you paste is kept as it is. For Nuvio, the backend's address, the profile chosen, and the session's refresh token, saved again each time Nuvio renews it, at every import. The password is never kept. One connection per app per user: signing in again with **Stay connected** replaces it, and signs out the session Polyfin opened before.
- How it is protected: it is stored in the database encrypted with `POLYFIN_SECRET_KEY`, as tracking tokens are (see [Configuration](configuration.md)), never shown, never sent back to the browser, and never logged. Without the key, it is stored as it is, and **System › Health** warns about it. A connection the key cannot decrypt counts as none: sign in again.
- The imports: at that interval, Polyfin reads the account again and imports the profile chosen when connecting, with the same rules as the first import: played marks only added, the newer resume point wins. The library replaces what the connection put on the Watchlist: a title taken off it leaves the Watchlist, unless another list holds it. Disconnecting takes off the Watchlist the titles the connection alone held; those a one-off import brought stay until started. The addons are chosen once, at the first import: later imports never install any. Polyfin only reads the account: nothing is ever written to the app.
- Status: under **Import from an app**, **Connected apps** shows each connection: the account, by the email the app gives, who connected it and when, the last import with what it added, and when the next one runs. **Import now** imports at once. When the app refuses what Polyfin kept, a key signed out or a session ended, the connection stops, says so, and is not imported until you sign in again with **Stay connected** ticked. An app that does not answer, or asks to wait, is tried again at the next import, and nothing is told.
- Told when it stops: the connection stopping is told once, as it stops, to the user's notification targets and the server's that chose **Connection stopped** (see [Stopped connections](notifications.md#stopped-connections)), and **System › Health** lists the connections stopped, by user and app. Signing in again, then a new refusal, tells again.
- Disconnecting: **Disconnect** forgets everything kept, after signing out the session Polyfin opened: the Stremio key it got by a link code or a password, and the Nuvio session. A key you pasted is only forgotten, as signing it out would sign your browser out. What was imported stays, but for the titles of the Watchlist the connection alone held, which leave it. Deleting the user signs out the same sessions, in the background (see [Deleting a user](#deleting-a-user)).
- Who: users connect their own accounts while **Users may import their watch history from another server or app**, under **Settings › Security**, allows it: turned off, their connections are paused, still shown, and they may still disconnect them. An administrator can connect an app for a user from **Import from an app**, at the top of **Users**, as for the import; the page lists the connections of the user chosen, with **Import now** and **Disconnect**. An administrator's own connections are never paused.

Staying connected to Stremio deepens the risk of reading it (see [Stremio](#stremio)): its terms forbid extracting its data by automated means, and reading an account again and again is more plainly automated than a one-off import, the more so at a short interval.

**For app developers:**

- `GET /admin/api/account/app-import` also answers `connections`, each with its `app`, `account` (`null` when the app gives none), `connectedAt`, `connectedBy` (`id` and `name`, `null` once that account is deleted), `problem` (`sign_in_refused`, or `null`), `paused`, `importing`, `lastImport` (`at`, `read`, `played`, `resumed`, `watchlist`, `unmatched` and `problem`: `unreachable`, `rate_limited`, `unexpected`, `profile_missing`, `sign_in_refused`, `internal`, or `null`) and `nextImportAt`, the last import, or the connection, plus the interval set (`null` while stopped or paused), and `importEveryMinutes`, the interval. What a connection signs in with is never answered.
- The sign-in routes take `stayConnected`; the account read answers it back, and the import of it saves the connection.
- `POST /admin/api/account/app-connections/{app}/import` imports at once, and answers `connection`. A connection stopped is `app_connection_stopped`, none `app_not_connected`, and the setting off for a user `server_imports_disabled`.
- `DELETE /admin/api/account/app-connections/{app}` disconnects, even with the setting off, and answers `204`.
- An administrator's routes are the same under `/admin/api/users/{id}`, and answer the user's connections under `/admin/api/users/{id}/app-import`.

### Nuvio

A Nuvio account keeps, for each of its profiles, up to 6, the movies and episodes watched with their dates, the resume point of each, its library, and its addons. Polyfin reads them all at once, in the account backup Nuvio's own scripts move accounts with.

- **Nuvio backend**: empty for Nuvio's own, or the address of a self-hosted backend, on the local network as well.
- **Email** and **Password**: the account's, as in Nuvio's apps. **Sign in to Nuvio** signs in, reads the account, and signs out, unless you stay connected.

A wrong email or password, an address where nothing answers, or where something other than a Nuvio backend answers, each say so. Nuvio may ask to wait before signing in again: Polyfin waits up to 30 seconds, twice at most, and says so past that. The account backup is versioned: Polyfin reads its version 1, and refuses another, saying so, until it reads it too.

What comes over, profile by profile:

- Watched: each movie and episode, with the date it was marked watched, older apps' dates included. A series marked watched as a whole, without its episodes, is not marked played: Polyfin counts a series played from its episodes.
- Resume points: each movie's and episode's, with its runtime and date, unless it was watched since. Those of the titles of a Jellyfin or Emby server connected in Nuvio (`srv1:…`) stay in Nuvio.
- Library: its movies and series, on the Watchlist from the date Nuvio added them.
- Addons: those turned on in the profile, or in the first profile for a profile that uses its addons. Nuvio's plugins are not addons: they do not come over.

What Nuvio keeps without an account, as a guest, stays on the device. Progress Nuvio sends to Trakt or Simkl is imported from there (see [Tracking services](tracking.md)).

**For app developers:**

- `GET /admin/api/account/app-import` answers `enabled`, `addonsAllowed`, and `import`, the import into the user's account running or the last one, `null` when none.
- `POST /admin/api/account/app-import/nuvio` takes `backend` (empty for Nuvio's own), `email`, `password` and `stayConnected`, and answers `key`, `app`, `stayConnected`, and `profiles`, each with its `id`, `name`, counts (`entries`, `played`, `resumes`, `library`) and `addons`, each with its `index`, `name` and `address`, redacted to its host. A refused email or password is `nuvio_sign_in_refused`, an invalid address `invalid_nuvio_address`, a backend that does not answer `nuvio_unreachable`, something else answering `not_nuvio`, a backend asking to wait too long `nuvio_rate_limited`, and a backup of another version `nuvio_backup_unsupported`, with its `version`.
- `POST /admin/api/account/app-import` takes `key`, `profile` and `addons`, the indexes of the addons to add, and answers `import`.
- An administrator's routes are the same under `/admin/api/users/{id}/app-import`, for the user `{id}`.
- An import names its `app`, `profile`, `user` and `startedBy` (`id` and `name`), `state` (`running`, `done` or `failed`), its counts (`read`, `played`, `resumed`, `watchlist`: the titles added to the Watchlist, `unmatchedCount`), `unmatched` (`id`, `name`, `type`, `season`, `episode`, `reason`), and `addons` (`name`, `address`, `installed`, `problem`).
- Polyfin reads Nuvio's public configuration at `GET {backend}/.well-known/nuvio` for its publishable key, signs in with `POST {backend}/auth/v1/token?grant_type=password`, reads the account with one `POST {backend}/rest/v1/rpc/sync_export_account_backup`, which must answer `format` `nuvio_account_backup` and `version` 1, and signs out with `POST {backend}/auth/v1/logout?scope=local`, which ends its own session only. A connection renews its session with `POST {backend}/auth/v1/token?grant_type=refresh_token` before each read, and keeps the new refresh token, the old one being refused from then on. The key goes in the `apikey` header and the session's token in `Authorization: Bearer …`, never in an address. A `429` or `503` answer with `Retry-After` is waited for.

### Stremio

A Stremio account keeps one library for the whole account: for each title, whether it was watched and when last, the resume point of the video played last, and whether it is in the user's library; and the addons the user installed. Polyfin reads it all at once, with the calls Stremio's own apps make.

**Sign in with** offers three ways, the first working for every account:

- **A link code**: **Get a link code** asks Stremio for a code, as Stremio's TV apps sign in, and shows it with **Open the link**. Open the link in a browser where you are signed in to Stremio, check that it shows the code, and approve it. The page asks every few seconds whether it is approved, then reads the account and signs out, unless you stay connected. A code lasts 5 minutes: past that, ask for another. This works however the account signs in, with Facebook and Apple too, and Polyfin never sees the password.
- **Email and password**: the account's, as in Stremio's apps. **Sign in to Stremio** signs in, reads the account, and signs out, unless you stay connected. An account made with Facebook or Apple has no password: use a link code.
- **An auth key**: the key Stremio's web app keeps once you sign in there, which `JSON.parse(localStorage.getItem("profile")).auth.key` gives in the browser's console on that page. **Read the account** reads it with the key, and does not sign it out: that would sign your browser out too.

The key Polyfin gets, or is given, serves this one read: it stays in memory for the read only, and is never logged, nor sent back to the browser. It is stored, encrypted, only to stay connected (see [Staying connected](#staying-connected)). A wrong email or password, a refused key, and Stremio not answering each say so.

What comes over:

- Watched movies: each movie Stremio counts watched, or saw watched most of the way, with the date it was last watched, else of its last change, and how many times it was.
- Watched episodes: Stremio keeps those of a series as a row of bits, one per episode, in the order of the series' episode list as the user's app listed it, anchored on the episode watched last. Polyfin reads the row against the series' episodes as the server's addons list them, in the same order (by season, episode, then release date, episodes without numbers first), shifted on the anchor, so that episodes listed since before it still fall in place. Stremio keeps no date per episode: each takes the date the series was last watched.
- Resume points: one per title, at the video played last, the movie or an episode, with its runtime and date, while the title shows in Continue Watching: in the library, or played without being added. A title removed from the library keeps its watched marks, not its resume point.
- Library: its movies and series, on the Watchlist from the date Stremio first kept them. Titles only played, and those removed from the library, are not on it.
- Addons: those the user installed. Stremio's own, which come with it, are left out.

Channels, TV channels and other videos do not come over.

The row of bits has its limit: it means something only against the same episode list as the user's app had. When the server's addons do not list the anchor episode at all, none of the series' episodes is marked played, and the result lists the series with **Its episodes do not match Polyfin's list**. When they list other episodes after the anchor, or other specials, the episodes marked can be the wrong ones, and nothing tells it: the server's metadata addons should list series as the user's app did.

Reading a Stremio account has its risks:

- Stremio's account API is not documented. Polyfin calls it as Stremio's own apps do; Stremio may change it, or turn other clients away, without notice.
- Stremio's General Terms (section 5.4.1) forbid using Stremio for "monitoring, copying or extracting any information from Stremio, including its contents, graphic user interface or Database whether done mechanically or by automated means". A user reading their own account with their own key is a grey zone: nothing says it is allowed. Staying connected, which reads the account again at the interval set, every 6 hours by default, makes that grey zone more pronounced, the more so as the interval is short.
- Stremio's own export of an account's data, under its settings, is reported broken: its library repeats one title many times (Stremio/stremio-bugs#614). Polyfin does not read it.

Users who connected Trakt in Stremio can import their history from Trakt as well (see [Tracking services](tracking.md)).

**For app developers:**

- `POST /admin/api/account/app-import/stremio/link` asks for a link code, and answers `code` and `link`, the address to approve it at, never a key. A user waits for one code at a time, for 5 minutes.
- `POST /admin/api/account/app-import/stremio/link/poll` takes `stayConnected`, answers `202` and `pending` until the code is approved, then reads the account, signs out unless asked to stay connected, and answers it as `…/nuvio` does. A code expired, already used, or never asked for is `stremio_link_expired`.
- `POST /admin/api/account/app-import/stremio/password` takes `email`, `password` and `stayConnected`, and `POST /admin/api/account/app-import/stremio/key` takes `key` and `stayConnected`; both answer the account read. A refused email or password is `stremio_sign_in_refused`, a refused key `stremio_key_refused`, Stremio not answering `stremio_unreachable`, and an answer Polyfin does not read `stremio_unexpected`.
- An unmatched series whose episodes could not be read has the `reason` `episode_list`.
- Polyfin asks for a link code with `GET https://link.stremio.com/api/v2/create?type=Create` and polls it with `GET https://link.stremio.com/api/v2/read?type=Read&code=…`, signs in with `POST https://api.strem.io/api/login` (`email`, `password`, `facebook` `false`), reads the library with `POST https://api.strem.io/api/datastoreGet` (`collection` `libraryItem`, `ids` `[]`, `all` `true`) and the addons with `POST https://api.strem.io/api/addonCollectionGet` (`update` `true`), reads the account's email with `POST https://api.strem.io/api/getUser` to stay connected, and signs out with `POST https://api.strem.io/api/logout` the keys it obtained, never a key pasted. The key goes in each body as `authKey`, never in an address.

## Ending an account

An account made for a friend's weekend need not stay until someone remembers to delete it. **Ends on**, in the **Access** section of a user's page, takes a date, kept with **Save date**; **Never**, the default, clears it. From that date, in the server's time zone, the account is refused as a disabled one is: it cannot sign in, from a Jellyfin app, Quick Connect or the admin app, and its devices and admin sessions are refused. Unlike **Disabled**, an end signs nothing out: everything the account has stays, its devices included, and a later date, or **Never**, brings it back as it was.

- **Users** lists each account's end date, as "Ends on October 20, 2026", and shows **Ended** once it has passed, as does the user's page.
- The activity log records each account that ends, once, as "The account of sam ended", within the hour: a scheduled task looks for them every hour (see [Scheduled tasks](administration.md#scheduled-tasks)). An account given a later date is recorded again when it reaches it.
- Jellyfin apps see an ended account as disabled. Jellyfin has no end date: an app sending back the policy it read leaves **Disabled** off, and only the admin app changes the date.
- An administrator's account never ends, so that the last administrator is never locked out: an administrator cannot be given an end date, and making a user an administrator clears theirs.
- An ended account's Live TV recordings, new episode notifications, weekly summary and imports from an app stop, as a disabled one's do. Its devices still count as unused for **Sign out devices unused for (days, 0 = never)** (see [Signing out unused devices](#signing-out-unused-devices)).

**For app developers:**

- `GET /admin/api/users` and `PATCH /admin/api/users/{id}` answer `endsOn`, a `YYYY-MM-DD` date in the server's time zone or null for never, and `ended`, true once that date has come. `PATCH` takes `endsOn`, a date or null; anything else is 400 `invalid_ends_on`, and a date for an administrator, or for a user made one in the same change, is 409 `administrator_cannot_end`.
- An ended account signs in as a disabled one does: 403 from `/Users/AuthenticateByName`, Quick Connect and `POST /admin/api/session` (`account_disabled`). Its access tokens answer 401, and its policy's `IsDisabled` is true.

## Invite links

An invite link lets the people an administrator sends it to create their own account, so that nobody has to choose a password for them and send it in a message. **Create an invite link**, at the top of **Users**, asks for:

- **Accounts it may create**: from 1 to 100, 1 by default. One link can serve a whole family.
- **Expires**: after 1, 7 (the default), 30 or 90 days, or **Never**.
- **Accounts end**: **Never**, the default, or 1, 7, 30 or 90 days after each is created. Each account the link creates is given that end date (see [Ending an account](#ending-an-account)).
- **Settings of**: the user whose settings the new accounts copy, or a new user, as with **Create a user**.

With a model, a new account copies everything the model's page sets except the name, the password, **Administrator** and **Ends on**: the **Access** permissions, the **Playback and access** limits with the **Quality group**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours**, and whether it shows on the sign-in screen. The model is read when each account is created, so a change to the model reaches the accounts created after it. Without a model, a new account has the settings of a user made with **Create a user**, hidden from the sign-in screen. An invite link never makes an administrator.

The link is shown once, with **Copy link**: Polyfin keeps only a hash of it and cannot show it again. It starts with the **Public address** (**Settings › Notifications**) when one is set, and with the address the admin app is open at otherwise.

**Invite links**, under the accounts, lists each link with the accounts it created out of those it may create, whose settings it gives, when it expires, when its accounts end, who created it, and its state: **Active**, **Used up**, **Expired** or **Revoked**. **Revoke** stops an active link at once; the accounts it created are kept. Deleting the user a link copies revokes the link too.

### The guest's page

The link opens a page of the admin app, `/admin/invite/…`, that needs no account, in English or French as the guest chooses. It shows the server's name and asks for **Your name**, a **Password** and **Confirm password**, under the rules of **Create a user**. Once the account is created, the guest lands on the [web client](web-client.md), signed in. Without the web client, they land on the admin app instead, signed in, where a member sees their own pages.

A link that is used up, expired or revoked says so, and so does an address that is no link at all.

Each account created through a link is written to the activity log, as "sam joined through alex's invite", and is told to the notification targets that chose **User joined** (see [Notifications](notifications.md#events)).

### How links are kept safe

- A link's token is the invite's ID followed by 32 random bytes, as many as an admin session's. Polyfin stores only the SHA-256 hash of the random part, and compares it in constant time.
- The guest's requests count toward the client's failed sign-ins: a token that is no link's counts as a wrong password does. After too many failures, the client is refused for a while (429), as on the sign-in page.
- A link's uses are counted in the transaction that creates the account, with the link locked: two guests on the last use of a link create one account, and the other is told the link is used up. A name or password that is refused uses nothing up.
- The guest sends a name and a password only. Anything else in the request is ignored: the account's settings come from the link.

**Compared with Jellyfin:**

- Jellyfin has no invite links: its administrators create every account and choose its password.

**For app developers:**

- The admin API serves the links to administrators:
  - `GET /admin/api/invites` lists them, the newest first: `id`, `maxUses`, `uses`, `expiresAt` (null for never), `createdAt`, `revokedAt`, `model` and `createdBy` (each `id` and `name`, or null; `createdBy` is null once that user is deleted), `state`: `active`, `used_up`, `expired` or `revoked`, and `accountsEndInDays` (null for never).
  - `POST /admin/api/invites` creates one from `maxUses` (1 by default, up to 100), `expiresInDays` (7 by default, up to 365, `null` for never), `modelUserId` (null or left out for a new user's settings) and `accountsEndInDays` (up to 365, null or left out for never). It answers 201 with the link as listed, plus its `token` and its `url`, which is null without a public address: the link is then `/admin/invite/<token>` on the server's address. The token is never answered again. Errors: `invalid_invite_uses`, `invalid_invite_expiry`, `invalid_invite_model` and `invalid_invite_account_end`.
  - `POST /admin/api/invites/{id}/revoke` revokes one, and answers it.
- The guest's page uses two routes that need no session:
  - `GET /admin/api/invite/{token}` answers `serverName` while the link may create an account.
  - `POST /admin/api/invite/{token}` with `name` and `password` creates the account. It answers 201 with `user` (`id`, `name`, `isAdministrator`, `imageTag`) and `webClient`. Without the web client (`webClient` false), the answer opens an admin session, as signing in does. With it, the page signs in with `POST /Users/AuthenticateByName` as jellyfin-web does, naming jellyfin-web's app and device, and keeps the session where jellyfin-web 12.2 reads it, in `jellyfin_credentials` in the browser's local storage.
  - A token that is no link's answers 404 `invite_unknown`, and a link used up, expired or revoked 410 `invite_used_up`, `invite_expired` or `invite_revoked`. A refused name or password answers as creating a user does: `invalid_name`, `invalid_password` or `name_taken`. Too many failures answer 429 `too_many_attempts`, with `Retry-After`.

## Parental control

On a user's page under **Users**, the **Parental control** section can limit the user to titles up to a rating (**Maximum rating**). The administrator can also hide from them the movies or shows that have no rating (**Block unrated movies**, **Block unrated shows**). Administrators' Jellyfin apps can set the same limit from their user settings.

### How a title's rating is found

- A title's rating is the certification the server's metadata addon gives: the US one for AIOMetadata, or the local one when there is no US one.
- Polyfin also reads the French, German and British ratings, and plain ages ("12", "16+").
- A rating Polyfin does not know counts as none.
- Users' own addons never rate titles.
- Seasons and episodes follow their series.

A limited user's apps show the server's addons and libraries only. Their own addons are kept but not used, and they cannot turn the server's addons off.

### What a limited user sees

A title above the limit is hidden from the user everywhere, as Jellyfin hides it:

- libraries, collections, genre, studio and year pages, search, latest, Continue Watching, Next Up, Upcoming, playlists, similar titles and the titles of a person leave it out;
- people credited only in hidden titles are not listed;
- opening, playing, downloading it or searching subtitles for it is refused.

Artwork stays reachable without signing in, as in Jellyfin.

### Ratings looked up for limited users

Addon catalogs rarely carry ratings. For a limited user, Polyfin asks the metadata addon for each listed title whose rating it does not know yet, a few at a time, once per title.

- The rating is kept, whichever user opened the title.
- It is asked again after a week, or after a few hours for a title that had none.
- A listing waits a few seconds for these ratings. It then stops at the titles still unknown, which appear in later listings.
- A listing reads at most a few times as far into a catalog as it would for a user without a limit.

## Blocked genres, visible libraries and allowed hours

On a user's page under **Users**, three more sections hold three more settings, each with its own save button. Administrators' Jellyfin apps can also set them from their user pages.

### Visible libraries

**Visible libraries** chooses which of the server's libraries the user's apps show. By default the apps show all of them, and libraries added later show too.

This setting only chooses the libraries shown; it blocks no title. A hidden library's titles stay reachable through other libraries, search or links. Listing the hidden library itself is refused, as Jellyfin refuses a folder the user may not access (401). Parental control and blocked genres are what block titles.

**For app developers:**

- Apps read and set this as the policy's `EnableAllFolders` and `EnabledFolders`.
- Jellyfin keeps the libraries shown, while Polyfin keeps those hidden. That way a library added later shows even when an app chose the libraries.

### Blocked genres

**Blocked genres** hides the titles of any of these genres, in any letter case, everywhere parental control hides titles: lists, search, details, playback, downloads, Continue Watching, Next Up, similar titles and person pages.

- A title's genres are those the server's addons give in its complete description. They are looked up and kept like its rating.
- A title is hidden until its genres are known.
- A user who blocks genres browses the server's addons only, as under parental control.
- The admin app offers the genres of the server's libraries (those their genre pages are named after), or any genre typed.

**Compared with Jellyfin:**

- Jellyfin has no such setting. Its `BlockedTags` are tags, which Polyfin's titles do not have, so the policy reports none and Polyfin ignores those apps send.

### Allowed hours

**Allowed hours** are Jellyfin's access schedules. Each row has:

- a day: one day of the week, every day, weekdays or weekends;
- a start and end hour from 0 to 24, both included, in the server's time zone.

A user with at least one row who is outside all of them cannot sign in (403). Their apps' requests are refused with an empty 403, as Jellyfin does, except reading their own user and the server's information.

- As in Jellyfin, administrators are refused sign-in outside their hours, but their apps already signed in are served.
- A Quick Connect sign-in is not checked, though the app's requests then are.
- Polyfin refuses hours outside 0 to 24, or ending before they start.
- In the admin app, nobody signs in outside their hours, and a member already signed in sees only who they are.

**Compared with Jellyfin:**

- Jellyfin serves direct streams without checking who asks; Polyfin also refuses a user's media outside their hours.
- jellyfin-web never sends hours outside 0 to 24 or ending before they start.

## Playback and access limits

On a user's page under **Users**, the **Playback and access** section holds five more limits, saved with **Save playback and access**. Administrators' Jellyfin apps also set them from the user's settings.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Playbacks at once (0 = no limit)** | **Users › Playback and access** | 0, no limit (up to 20) | How many of the user's devices may play at once. |
| **Maximum quality** | **Users › Playback and access** | No limit | Caps the bitrate of everything the user plays. |
| **Live TV** | **Users › Playback and access** | On | Whether the user can see and play Live TV. |
| **Watch together** | **Users › Playback and access** | Create and join | Whether the user may create and join SyncPlay groups, only join them, or not watch together at all. |
| **Can control other users' apps** | **Users › Playback and access** | On for administrators, off for others | Lets the user see and control the apps of other users from theirs. |

The same section also holds **Quality group**; see [Quality groups](#quality-groups).

### Playbacks at once

Once that many of the user's other devices are playing, as **Sessions** lists them, one more device cannot start playing. jellyfin-web explains this as media that cannot be played at this time; Polyfin's web client script closes the generic error jellyfin-web shows over it (see [Web client](web-client.md#dashboard-links-and-single-sign-on)). A device already playing can go on to another title.

**Compared with Jellyfin:**

- PlaybackInfo from the extra device answers Jellyfin's `RateLimitExceeded`.
- Jellyfin's `MaxActiveSessions` counts signed-in sessions and refuses signing in. Polyfin counts playbacks instead, on purpose, so a user stays signed in on every device.
- Polyfin refuses a policy setting more than 20 (400).

### Maximum quality

Polyfin uses the lower of the app's limit and this one. A version above it is:

- converted down to it, when the user may use conversion (see [Transcoding](transcoding.md));
- else skipped for the next version under it;
- else nothing plays (`NoCompatibleStream`).

A version above the limit is never played as it is or with its video copied, even from a kept or made-up URL.

**Compared with Jellyfin:**

- Jellyfin's `RemoteClientBitrateLimit` applies to remote apps only. Polyfin applies it, on purpose, to all of the user's playback.

### Live TV

With **Live TV** off, the Live TV view leaves the user's apps, and channels and programmes can be neither found nor played. See [Live TV](live-tv.md).

**For app developers:**

- The `/LiveTv` routes answer 403, as Jellyfin's do without `EnableLiveTvAccess`.

### Watch together

**Watch together** chooses, as Jellyfin's `SyncPlayAccess` does, whether the user may:

- create and join SyncPlay groups (the default);
- only join them;
- not watch together at all, in which case jellyfin-web hides its button.

Creating a group takes the first choice. Listing and joining groups takes either of the first two. Other SyncPlay requests answer 403 unless the user is in a group.

### Can control other users' apps

**Can control other users' apps** is Jellyfin's `EnableRemoteControlOfOtherUsers`.

- Administrators have it when created, and when upgrading from an older Polyfin.
- Other users do not.
- A user made administrator later keeps the value they had.

## Quality groups

**Quality group**, on a user's page under **Users** in **Playback and access** next to the maximum quality, sets the highest resolution each user is offered.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Quality group** | **Users › Playback and access** | **Original (no limit)** | Highest resolution offered: **Original (no limit)**, **4K**, **1440p**, **1080p**, **720p** or **480p**. |

### How the height of a version is found

Once Polyfin has analyzed a version, its height is the one the analysis found. Before that, it is the one RemuxDB found in its file, when [Describe versions from RemuxDB](playback.md#tracks-from-remuxdb) is on and RemuxDB knows the file. Otherwise, it is the one its labels name, in the addon's stream name, title, description or file name:

- 2160p, 4K or UHD;
- 1440p, 2K or QHD;
- 1080p or FHD;
- 720p or HD;
- 576p, 480p or SD.

Labels that disagree, or name none, leave the height unknown.

### Which versions a user is offered

While at least one version fits the group or has an unknown height:

- versions taller than the group are left out of what the user is offered and of the versions tried for playback;
- they are not analyzed;
- the others keep their order.

When every version is taller, all are kept rather than none, the closest to the group first (720p before 1080p before 4K; versions of the same height keep their order). That way the least is read and decoded.

- The version played is converted down to the group, or to **Maximum quality of converted video** when that is lower, and to 1080p at most when the processor converts (4K on a GPU), as every conversion is. See [Transcoding](transcoding.md).
- A version that fits plays as it is, or remuxed, as before.
- A taller version is never sent at full size.
- A user who may not have video converted (their **Can use conversion** permission, or the server's **Conversion** switch, off) is refused taller versions, as other conversions are refused (`NoCompatibleStream`).

### Live TV, recordings and downloads

- Live TV channels follow the same rule once analyzed, as their height is only known then. So do recordings.
- Only versions that fit can be downloaded, as downloads are never converted. When none fits, the download is refused (403), as without the permission.

**Compared with Jellyfin:**

- Jellyfin has no such setting. Apps neither show nor change it, and a policy they post keeps it.

**For app developers:**

- PlaybackInfo reports the conversion of a taller version as Jellyfin reports a resolution limit (`VideoResolutionNotSupported`), describing its video at the size sent.

## Security settings

**Settings › Security** gathers six settings, and **Settings › Diagnostics** a seventh. They apply at once, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Allow users' own addons** | **Settings › Security** | On | Lets users add addons of their own under **My sources**. |
| **Users may import their watch history from another server or app** | **Settings › Security** | On | Shows the imports from a Jellyfin or Emby server and from an app under **My account** (see [Importing your own watch history](#importing-your-own-watch-history) and [Importing from an app](#importing-from-an-app)). |
| **Import connected accounts every** | **Settings › Security** | 6 h (or 15 min, 30 min, 1 h, 2 h, 3 h, 12 h, 24 h) | How often the apps users stay connected to, and the watch histories they import from tracking services, are imported again (see [How often connected accounts are imported](#how-often-connected-accounts-are-imported)). |
| **Block an account after this many wrong passwords (0 = never)** | **Settings › Security** | 0, never (or 3 to 20) | Blocks an account for 15 minutes after that many wrong passwords in a row. |
| **Sign out devices unused for (days, 0 = never)** | **Settings › Security** | 0, never (or 1 to 365 days) | Signs out Jellyfin apps not used for that many days. |
| **Known proxies** | **Settings › Security** | None (up to 64 addresses, networks or host names) | The reverse proxies whose `X-Forwarded-For` or `X-Real-IP` header names the client of their requests. |
| **Detailed log** | **Settings › Diagnostics** | Off | Logs at the `debug` level until turned off. |

### Users' own addons

Under **My sources**, users choose which catalogs of their own addons are libraries, with their names, images, genres and maximums, and whether the web player's top bar shows them, as administrators do under **Content › Libraries** (see [Library images](addons-and-libraries.md#library-images), [Genre and maximum](addons-and-libraries.md#genre-and-maximum) and [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). Their image addresses must be public: only administrators can use local network addresses.

On a user's page under **Users**, the **Access** section also holds **Can add their own addons**, on by default. While either this or **Allow users' own addons** is off:

- the user's own addons are kept but not used;
- their apps show the server's addons and libraries only;
- **My sources** says why;
- their addons cannot be added, replaced, refreshed or turned on (403 `personal_addons_disabled`).

The addons come back once both are on again. See [Addons and libraries](addons-and-libraries.md).

### How often connected accounts are imported

**Import connected accounts every** sets how often Polyfin imports again the Nuvio and Stremio accounts users stay connected to (see [Staying connected](#staying-connected)) and the watch histories they import from tracking services (see [Importing your watch history](tracking.md#importing-your-watch-history)): from 15 minutes to 24 hours, 6 hours by default. Someone who watches in one of those apps, then opens a Jellyfin app, finds their progress sooner with a short interval, or at once with **Import now**.

- A change applies at once: the next import of each connection comes that long after its last import, or after it was connected, so a shorter interval brings the imports due forward, and a longer one puts them off. The next import that **Connected apps** and **My account › Tracking** show follows it. Polyfin looks for the imports due every minute.
- Each import reads the whole account again, so a short interval sends more requests to those services. Polyfin already waits when one asks it to, and keeps to each service's pace (see [Pace and problems](tracking.md#pace-and-problems)).
- Stremio's terms forbid extracting its data by automated means: the more often Polyfin reads a Stremio account, the more pronounced that grey zone grows (see [Stremio](#stremio)).

**For app developers:**

- `GET /admin/api/settings` answers `importEveryMinutes`, in minutes, its choices in `bounds`. `PUT` takes one of them; any other is `invalid_import_every_minutes`.

### Blocking after wrong passwords

After that many wrong passwords in a row for one account, from a Jellyfin app or the admin interface, the account refuses every sign-in for 15 minutes. That includes the right password, which gets the answer of a wrong password.

The count starts again after any of these:

- a sign-in;
- a new password set by an administrator;
- **Unblock** on the user's page under **Users**, where the account shows until when it is blocked;
- setting the limit to 0.

Each block is told once to the notification targets that chose **Account blocked** (see [Blocked accounts](notifications.md#blocked-accounts)): the server's targets and administrators' own, with a link to the user's page where **Unblock** is, and the user's own, with until when, and the address and the app of the last attempt. Attempts from a client refused for a while after too many failed sign-ins from its address (429) never reach the account, and are not told: such attempts come all the time from anywhere on the internet. A sign-in from a device the account never used is told to the user (see [Sign-ins on new devices](notifications.md#sign-ins-on-new-devices)).

**Compared with Jellyfin:**

- Jellyfin disables the account until an administrator enables it again. Polyfin only blocks it for a while, so that the last administrator can never be locked out for good.

**For app developers:**

- Jellyfin apps read the limit (-1 when there is none) and the count in the user's policy, as `LoginAttemptsBeforeLockout` and `InvalidLoginAttemptCount`.
- The limit is the server's, so a policy an app posts does not change it, nor the count.

### Signing out unused devices

Polyfin signs out the Jellyfin apps not used for that many days, at startup and every hour. It closes their connections and takes them out of SyncPlay groups, as any sign-out does. Admin interface sessions are not concerned.

### Known proxies

Behind a reverse proxy, every request reaches Polyfin from the proxy's address. Listing the proxy's address, its network, or its host name, such as its container's name on a Docker network, has Polyfin take each request to come from the client the proxy names, for failed sign-ins, the addresses devices show and the local network. A host name is looked up again every minute, so the proxy may change addresses. See [Known proxies](reverse-proxy.md#known-proxies).

### Detailed log

With **Detailed log** on, Polyfin logs at the `debug` level until it is turned off again, and at the `info` level otherwise. See [Configuration](configuration.md).
