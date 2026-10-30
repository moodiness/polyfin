# Users and permissions

This page covers user accounts: what Polyfin keeps for each user, and the limits an administrator can set on them. It also covers the server-wide security settings.

## Accounts and permissions overview

Polyfin has separate accounts for each user, with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points and Next Up are tracked per user, and each user can be limited to titles up to a rating.

An administrator creates users with **Create a user** at the top of **Users**, and sets each user's limits on that user's own page, opened from the list. The page has these sections: **Name and password**, **Access**, **Playback and access**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours** and **Devices**. **Access** switches save as soon as they change, and its **Ends on** date with **Save date** (see [Ending an account](#ending-an-account)); each other section has its own save button, such as **Save playback and access**. Signing out a device asks first. Administrators' Jellyfin apps can set most of the same limits from the user's settings. Each section below says where a setting lives. To give several accounts the same limits at once, an administrator puts them in a group (see [User groups](#user-groups)). **Reset to defaults**, at the top of the page, puts the limits back to a new user's (see [Resetting settings to their defaults](#resetting-settings-to-their-defaults)).

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
- **Continue Watching** lists what is under way. "Remove from Continue Watching" takes a title off it and changes nothing else: the title stays played or not, with its plays and the date it was last played. Polyfin remembers when: an import from a tracking service or an app brings the title back only with a resume point set after that (see [How the history merges with Polyfin's](tracking.md#how-the-history-merges-with-polyfin-s)). Playing the title again sets a resume point as usual, and nothing is sent to tracking services.
- "Hide this title" takes a movie, a home video, a series or a folder of videos off the user's lists: libraries, home rows (Latest, Continue Watching, Next Up, Upcoming), searches, similar titles, recommendations, person pages, collections and the Watchlist. A hidden series takes its episodes with it, and a hidden folder its videos. The title still opens and plays from a link, and its page still lists its seasons and episodes. Other users are not affected. "Show this title again" brings it back.
- In the web player, both are in the ⋮ menus (see [Web client](web-client.md#dashboard-links-and-single-sign-on)). For the other apps, which have no such buttons, **My account** in the admin app has a **Continue Watching** section, each title with **Remove**, and a **Hidden titles** section, each with **Show again**. Other apps see the change at their next refresh.
- **Next Up** lists the next episode of each series being watched (see [Continue Watching and Next Up](#continue-watching-and-next-up)).
- The **Upcoming** row lists the coming episodes of the series the user watches or marked favorite.
- Both look at the 50 series the user watched most recently, as each series asks its addon for its episodes. A history imported from a tracking service can hold hundreds of series (see [Importing your watch history](tracking.md#importing-your-watch-history)).
- Marking a series or a season played marks its released episodes.
- Apps that keep Jellyfin's live connection (WebSocket) open are told of these changes as they happen, from any of the user's apps.

Users can also make playlists of movies and episodes from their apps. A playlist can be private, shared with other users, or open to all.

### Continue Watching and Next Up

**Continue Watching** lists the movies, episodes and videos the user stopped past **Resume point kept after (%)** and before **Marked played after (%)**, 5% and 90% by default (see [Played and resume thresholds](#played-and-resume-thresholds)), the most recently played first. A 45-minute episode stopped in its first 2 minutes 15 seconds keeps no resume point, and does not show there. Whatever version plays, a torrent's or another, the resume point is the title's: the movie's or the episode's.

**Next Up** lists one episode for each series the user watches:

- A series is watched once the user played or started one of its episodes, even an episode stopped before the resume threshold.
- Its episode is the first released, unplayed one after the furthest episode played, specials left out. For a series none of whose episodes is played, it is the first episode.
- When that episode is under way, it is offered to the apps that ask for such episodes. The web player's home does not, and leaves the series out: the episode is in Continue Watching instead.
- Series come by when their furthest played episode was played, the latest first, then the series with no episode played, the latest started first.
- The web player's home also leaves out the series not watched for longer than its "Max days in 'Next Up'" setting, on its Display settings page: a year by default.

A user who wants Continue Watching alone hides Next Up in the web player: under **Settings › Home**, the section that shows **Next Up**, the sixth by default, set to **None**. It is saved in the account, so it follows the user to other browsers. Other apps have their own home settings. Rows the administrator placed **After Next Up** then take the place before it (see [Home rows](web-client.md#home-rows)).

The web player's home keeps its rows for a minute. jellyfin-web 12.4 keeps what Continue Watching and Next Up list for up to a minute after it last asked, also across a reload, and draws the rows again from it without asking: a title stopped or marked played moves in these rows only once that minute has passed. Polyfin answers the change at once, and other apps show it at their next refresh. A card taken off with "Remove from Continue Watching" leaves the row at once (see [Web client](web-client.md#dashboard-links-and-single-sign-on)).

**Compared with Jellyfin:**

- Next Up follows Jellyfin 12.4: a series is watched once one of its episodes has a date, played or only started ([NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L42-L70)), and a series with no episode played comes after the others, from its first unplayed episode ([NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L204-L247), [TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L145-L151)). An episode under way is left out when the app asks so ([TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L227-L237)).
- The web player keeps its rows for a minute on a Jellyfin server too: its queries stay fresh for a minute ([queryClient.ts](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/utils/query/queryClient.ts#L12-L50)), and the changes the server announces do not reach the rows' queries ([QueryClientEventHandler.tsx](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/QueryClientEventHandler.tsx#L20)). This is reported upstream as jellyfin/jellyfin-web#8543.

### Home rows

**My account › Home rows** lists the rows the administrator added to the web player's home page for the user (see [Home rows](web-client.md#home-rows)), each with its kind and place, and a switch. Off, the row leaves the user's home page, and, when it is also a library in other apps, their libraries there. Other users keep it. jellyfin-web's own home sections stay under its **Settings › Home**. **Reset my preferences** turns every row on again (see [Your own preferences](#your-own-preferences)).

### Track choices

**My account › Track choices** tells for how many series and movies Polyfin keeps the audio and subtitle tracks the user chose, which their next plays show by default in every app (see [Remembered track choices](playback.md#remembered-track-choices)). **Forget my track choices** forgets them all, after asking first: every title plays the tracks the user's language preferences choose again. **Reset my preferences** forgets them too (see [Your own preferences](#your-own-preferences)).

**For app developers:** `GET /admin/api/account/track-choices` answers `{"titles": n}`, the number of series and movies with a choice of the user signed in to the admin app, and `DELETE /admin/api/account/track-choices` forgets them, answering 204.

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

A title that cannot be matched is left out, and the import goes on. Each user's result lists these titles with why: **No IMDb, TMDB or TVDB identifier** (a home video, or an episode Jellyfin did not number) or **Not in Polyfin** (it has no IMDb identifier, no catalog listed it under its TMDB or TVDB one, and no metadata addon of the server describes it by its TMDB one, or a series by its TVDB one). Plex adds two more (see [Plex](#plex)).

### How the data merges with Polyfin's

The data merges as an imported watch history does (see [How the history merges with Polyfin's](tracking.md#how-the-history-merges-with-polyfin-s)):

- Played marks, resume points and favorites are only added, never removed.
- A title keeps Polyfin's date when it is later than Jellyfin's, and the larger of the two play counts.
- Of two resume points, the more recent one stays.
- Importing again adds nothing twice and never overwrites newer Polyfin data, so an import can safely run again.

Nothing is written to the Jellyfin server: the import only reads it. Signing in opens a session there, as an app does, which Polyfin ends as soon as it is done with it.

### Progress and limits

- One import runs at a time. Leaving the page does not stop it, and the page shows how far it is, user by user, then what it read and added: the items read, titles marked played, resume points and favorites, what Polyfin already had, and the titles not found.
- **Stop import** stops it at once. What was imported stays: a user whose data was being saved is saved whole, and one whose data was being read gets nothing until the next import.
- Jellyfin is read 200 titles at a time, one request after the other. A request that fails is tried 3 times more; a server that keeps failing, or refuses the key, ends the import, after saving what was read of the current user. A user whose data the server does not let the key read is marked so, and the import goes on with the next users.
- A resume point without a date is counted, not added: whether Polyfin's own is newer cannot be told. Jellyfin dates every resume point; Plex dates the ones a player saved.
- An answer the import cannot read ends it with `not_jellyfin`, and Polyfin's log says where, at the warning level: `An import could not read an answer`, with the path asked and why it could not be read, never the query or the key.
- Once each user is read and saved, Polyfin's log says, at the info level, `An import read a user's watch data`, with the user's identifier on the server and what was read and done: for Plex the library `sections`, the items `listed` and, for an account other than the owner, the `history` rows and the rows of `otherAccounts` left out; then `read`, `matched`, `played`, `resumed`, `favorites`, `upToDate`, `undated`, `unmatched`, `nothingAdded` and `problem`. Never the server's address nor the key.
- A user signed in is signed out once their watch data is read, and the import's own session when it ends, stopped or failed. A restart during an import leaves them open: they show among the users' devices on the server, which can end them.
- Polyfin keeps the running import and the last result in memory only: a restart stops an import under way and forgets the result. Start it again: it adds only what is missing.

#### Nothing imported?

A user whose import ended without a problem and added nothing says why, under the counts:

- **Everything read was already in Polyfin**: an earlier import, or Polyfin itself, has it all (`up_to_date`).
- **The server lists nothing played, in progress or favorite for this user**: Jellyfin and Emby (`nothing_played`).
- **This Plex server has no movie or show library** (`no_sections`).
- **The owner has nothing played or in progress** in the Plex server's movie and show libraries (`plex_owner_nothing_watched`); a user's own Plex import says the same of their account (`plex_nothing_watched`, see [Plex, signing in](#plex-signing-in)).
- **Plex lists no play of this account on this server** (`plex_no_history`): see [Plex](#plex) for what Plex's playback history holds. Everything comes over when the administrator signs in to Plex as the owner (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)), or when the person signs in to Plex themselves (see [Plex, signing in](#plex-signing-in)).

A user whose titles were read but could not be matched says nothing more: the titles not found tell why.

**Compared with Jellyfin:**

- Jellyfin keeps watch data for the files of its libraries; Polyfin keeps it for the titles of its catalogs. Titles are therefore matched by their identifiers, not by their files.

**For app developers:**

- The admin API serves the import to administrators:
  - `POST /admin/api/jellyfin-import/users` lists a server's users, given `kind` (`jellyfin`, the default, `emby` or `plex`), `address`, and `apiKey`, or `account` with `name` and `password`. It answers `server` with its `kind`, and `keyOwner`, the user the key belongs to or who signed in, `null` for a server's key. An account the server refuses is `jellyfin_sign_in_refused`, and one it does not let sign in `jellyfin_sign_in_forbidden`.
  - `POST /admin/api/jellyfin-import` takes the same, creates the accounts and starts the import. An entry's `jellyfinPassword`, given with `watchData`, has the import sign in as that user. Connected as a user, another user's watch data needs it: without it, the answer is `jellyfin_key_owner_only`. A password the server refuses is `jellyfin_password_refused`, an account it does not let sign in `jellyfin_sign_in_forbidden`, and a sign-in that opens another user's session `jellyfin_other_user`. Each names the `jellyfinId`, and comes before anyone is created.
  - `GET /admin/api/jellyfin-import` reads the import, and `POST /admin/api/jellyfin-import/stop` stops it. The import names its `server.kind`, `startedBy` (`id` and `name`), and `own`, set for a user's import of their own watch history. Each user names what was read: `read` (the items with watch data), for Plex `sections`, `listed` and `history` (`null` for Jellyfin and Emby), and `matched`; what was done: `played`, `resumed`, `favorites`, `upToDate` (the marks, resume points and favorites Polyfin already had) and `undated` (the resume points without a date, not added); `unmatchedCount` and `unmatched`, whose `reason` is `no_identifier`, `not_found`, `not_in_library` or `other_episode_order`; `problem`; and `nothingAdded`, `null` unless the user's import ended without a problem and added nothing: `up_to_date`, `nothing_played`, `no_sections`, `plex_owner_nothing_watched` or `plex_no_history`.
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
| The owner's ratings of movies, shows and episodes, from 0 to 10 | Ratings, as an imported Letterboxd rating: a rating Polyfin has stays. |
| Another account's playback history on this server: each movie and episode it played to the end there, with the number of plays and the date of the last one | Played marks. |

With the owner's token, Plex gives Polyfin the owner's own watch state, but of the other accounts only their playback history on this server, and Plex's history holds only plays:

- a title played to the end on this server;
- not a title marked as watched without playing it: since Plex Media Server 1.40, marking makes no history entry;
- not a title synced from the account's Plex account, as Plex does when an account is given access to a new server;
- not a resume point: those stay on Plex too.

So an account that shows titles as watched in Plex may have nothing to import: the page then says that Plex lists no play of this account on this server, and that everything comes over when the administrator signs in to Plex as the owner (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)), or when the person signs in to Plex under **My account** (see [Plex, signing in](#plex-signing-in)): their own token reads their marked and synced titles, resume points and ratings. Plex has no favorites to import, and the owner's Watchlist is not imported here. A history row of another account than the one asked is left out.

#### Signing in to Plex as the owner

Once connected, the page offers **Sign in with Plex as the owner**, above the accounts. **Open Plex’s sign-in page** opens Plex's own page in a new tab, where the administrator signs in to the Plex account that owns the server; Polyfin never sees the Plex password. Polyfin then checks, with plex.tv, that this account owns the server connected to (the same machine identifier): another account is refused (**This Plex account does not own the server**, `plex_not_owner`), and the sign-in is kept for another try or **Sign out of Plex**.

With the owner's account, Plex gives Polyfin the token each account the server is shared with has for this server, the one that account's own Plex apps use to read it, and switches to the profiles of the owner's Plex Home. Each account then shows what will be read of it:

| Account | Read with | What comes over |
|---|---|---|
| The owner | The owner's token, as without the sign-in | Played titles, resume points and ratings. |
| An account the server is shared with | Its own token for the server | **Everything**: the titles it played, marked or synced as watched, its resume points and its ratings, as its own import would bring them (see [Plex, signing in](#plex-signing-in)), but not its Watchlist. |
| A profile of the owner's Plex Home | Its own token, once switched to as Plex's apps do | **Everything**, as above. A profile with a PIN asks for it in its row, under **Profile PIN**; a PIN Plex refuses stops the import before anyone is created (`plex_profile_pin_refused`), and without it the profile is read as the next row. |
| An account Plex gives no token for | The owner's token | **Plays only**: its playback history on this server, as without the sign-in, and the reason when it gives nothing. |

Transparency: the import reads the other accounts with the tokens Plex gives the owner for the accounts the server is shared with, as those accounts' own Plex apps read the server. It reads only the server's libraries with them, and writes nothing. The tokens are held in memory for this import only, never stored, logged nor answered. The owner's sign-in, and the sign-ins of the Home profiles switched to, are signed out of Plex once the import ends; the shared accounts' tokens, which are not sign-ins but the server's sharing, are just dropped. An administrator who does not sign in has the import as before.

Titles are found by the IMDb, TMDB and TVDB identifiers of their `Guid` list; an episode by its show's identifiers and its season and episode numbers. An item an older Plex agent matched has no `Guid` list: its `guid` names its identifier instead, as `com.plexapp.agents.imdb://tt…`, `com.plexapp.agents.themoviedb://…` or `com.plexapp.agents.thetvdb://…`, or after an agent's own scheme as `://tvdb-…`, `://tmdb-…` or `://imdb-tt…`. Two kinds of titles are listed as not found:

- **No longer in Plex's libraries**: a title a history names that no library section lists any longer, which leaves no identifier to find it by.
- **Numbered in another of TheTVDB's episode orders**: an episode of a show an older agent matched as `tvdb2-`, `tvdb3-`…, whose numbers are not TheTVDB's usual ones. Its show's identifier is known, but marking the episode its numbers name could mark the wrong one.

Plex is only read, with `GET` requests: it is never written to.

**For app developers:**

- The import's API is Jellyfin's, with `kind` set to `emby` or `plex`. Plex takes its owner's token as `apiKey`, and no `account` nor `jellyfinPassword`. With the owner signed in to Plex, `plexOwner` `true` reads each account with its own token, and an entry's `plexPin` is a Home profile's PIN; a PIN Plex refuses is `plex_profile_pin_refused` with the entry's `jellyfinId`, and an account that does not own the server `plex_not_owner`.
- `POST /admin/api/jellyfin-import/plex/sign-in`, `POST /admin/api/jellyfin-import/plex/sign-in/poll` and `DELETE /admin/api/jellyfin-import/plex/sign-in` sign the administrator in to Plex as a user's own sign-in does (see [Plex, signing in](#plex-signing-in)), whether or not users may import from another server. `POST /admin/api/jellyfin-import/plex/owner` takes the connection (`kind` `plex`, `address`, `apiKey`) and answers `accounts`, each account but the owner's by its identifier: `all`, `pin` (a Home profile with a PIN: everything with it, plays only without) or `plays`. The import takes the sign-in over: once it starts, the sign-in has ended.
- With the owner signed in, Polyfin also asks plex.tv `GET /api/v2/resources?includeHttps=1&includeRelay=1` with the account's token, to find the server among those it owns; `GET /api/servers/{machineIdentifier}/shared_servers`, in XML, whose `SharedServer` entries give each account's `userID`, the identifier `/accounts` lists it by, and its `accessToken`; `GET /api/v2/home/users`, whose `id` is that same identifier; and, for a profile, `POST /api/v2/home/users/{uuid}/switch?pin=…`, then `GET /api/v2/resources` with the profile's token for its own token for the server. An account read with its own token is read as a user's own import reads it, never `/accounts` nor the history.
- Emby is read under `/emby` when the address does not end with it, else at the address given, with the key in `X-Emby-Token`. The requests are Jellyfin's, but for `/Users/Me`, which Emby does not have: a key is taken only when `GET /Auth/Keys` lists it among the server's API keys. Lists ask for `Fields=ProviderIds,UserDataPlayCount,UserDataLastPlayedDate`, without which Emby leaves the play count and date out. Signing in sends the app and device in `X-Emby-Authorization`.
- Plex is read in JSON (`Accept: application/json`), with the token in `X-Plex-Token` and Polyfin's device in `X-Plex-Client-Identifier`: `/identity`, `/`, `/accounts` (account 1 is the owner, account 0 nobody), `/library/sections`, each movie and show section's `/library/sections/{key}/all` with `type=1` or `type=2` and `includeGuids=1`, the show sections' episodes with `type=4` for the owner, and `/status/sessions/history/all?accountID=…&sort=viewedAt:desc` for the other accounts, a page at a time with `X-Plex-Container-Start` and `X-Plex-Container-Size`. A history row whose `accountID` is not the one asked is left out, and one without a `ratingKey`, a title deleted from the libraries, is listed as `not_in_library` once per title.

## Importing your own watch history

Under **My account**, **Import from another server** lets each user bring their own watch history over from a Jellyfin, Emby or Plex server they used before, when an administrator could not: a server without API keys, or whose passwords the administrator does not know, or a Plex server whose owner's token reads only the plays of other accounts (see [Plex](#plex)). It also takes the export file of a Letterboxd account (see [Letterboxd](#letterboxd)).

- **Server**: **Jellyfin**, **Emby**, **Plex** (see [Plex, signing in](#plex-signing-in)), or **Letterboxd (export file)**.
- **Server address**, then **Your user name there** and **Your password there**, empty if the account has none.

**Import my watch history** signs in to that server as the user, as its apps do, and imports that user's played movies and episodes, resume points and favorites into their own Polyfin account only. Titles are matched and merged as an administrator's import does. The section then shows the import, and its result: what was added, and the titles not found. Polyfin never lists the server's users, which a user's session may not, and reads only the user signed in: even a server that answers every user with the session's data gives them their own. It signs out once done. The password is used for that import only, and never kept.

- One import from a server runs at a time on the whole server, and one import into an account, from a server or Letterboxd. While another runs, the import is refused (`server_import_running`): try again a few minutes later.
- A user sees only their own imports: the section shows their latest one, from a server or Letterboxd. The administrator's **Import from another server** page shows the import from a server running or the last one, saying who started it.
- **Users may import their watch history from another server or app**, under **Settings › Security**, on by default, turns the section off, Letterboxd included: it is hidden, and the API refuses (`server_imports_disabled`). It turns off the import from an app too (see [Importing from an app](#importing-from-an-app)).

**For app developers:**

- `GET /admin/api/account/server-import` answers `enabled`, `letterboxd` (whether a Letterboxd export may be imported), `searchable` (whether a metadata addon of the server searches movies), and `import`, the user's latest import running or ended, from a server or Letterboxd (`kind` `letterboxd`), `null` when none.
- `POST /admin/api/account/server-import` takes `kind` (`jellyfin` or `emby`), `address`, `name` and `password`, and answers `import`. A refused name or password is `jellyfin_sign_in_refused`, an account the server does not let sign in `jellyfin_sign_in_forbidden`; the address errors are those of the administrator's import. For Plex it takes `kind` `plex`, `server` and `watchlist` (see [Plex, signing in](#plex-signing-in)). A Letterboxd export is sent as described below.
- Polyfin sends the sign-in, the items requests and the sign-out of the administrator's import, never `/Users`.

### Plex, signing in

Choose **Plex**, then **Sign in with Plex**: **Open Plex’s sign-in page** opens Plex's own page in a new tab, where the user signs in to their Plex account as on any Plex app. Polyfin never sees the Plex password. Once Plex signed them in, the section names the account and lists the Plex servers it reaches, the server the administrators last imported from chosen first:

- **Plex Home profile**, for an account in a Plex Home: the profile whose watch data comes over, the account signed in first. A managed profile has no sign-in of its own, as Plex says (see Plex's support article "Managed Accounts"): the Home's member signs in, then chooses it here, with its **Profile PIN** when it has one, and **Use this profile** switches to it as Plex's apps do.
- **Server**: the server to import from. Polyfin reaches it at the addresses Plex gives for it, those on the local network first, then the remote ones, then those Plex relays, each an address the server's import takes; an address counts only when the server answers there with its own machine identifier.
- **Watchlist to my Watchlist** (on by default): the movies and shows of the profile's Plex Watchlist, which Plex keeps with the account rather than on a server, go to the user's [Watchlist](tracking.md#the-watchlist), as a one-off app import's library does: they stay until started.

**Import my watch history** then reads the server with the profile's own token for it, as the owner's import reads the owner's data, into the user's own Polyfin account only:

| From Plex | Into Polyfin |
|---|---|
| The movies and episodes played, marked as watched or synced as watched, with their play counts and the date last played | Played marks. |
| The resume points | Resume points, for titles not played. |
| The ratings of movies, shows and episodes, from 0 to 10 | Ratings, as an imported Letterboxd rating: a rating Polyfin has stays, and apps see `Likes` for a rating of 5 or more. A season's rating has no title in Polyfin, and stays on Plex. |
| The Watchlist, when ticked | The Watchlist. |

An account that has nothing played, in progress or rated there says so (`plex_nothing_watched`). Titles are matched as the administrator's Plex import matches them; a Watchlist item whose identifiers Plex's list leaves out is asked of Plex one by one, 500 at most, and is listed as not found when it has none.

About the tokens: Plex gives Polyfin a token for the account, which only lists the profiles and servers, switches profiles and reads the Watchlist, a token for the profile chosen, and one for the server. They are held in memory for the user who signed in only, never stored, logged nor answered, and dropped 10 minutes after the last choice, or once the import ends: then Polyfin signs them out of Plex, which takes Polyfin off the account's authorized devices for them. Polyfin names itself to Plex by one identifier, kept with the server's identity, so that repeated sign-ins reuse one entry. Should an entry stay, as after a restart during an import, remove Polyfin from the account's authorized devices in Plex's settings. The administrator's sign-in as the owner is held, and signed out, the same way.

**Compared with the administrator's Plex import:** the owner's token reads only the plays Plex recorded for other accounts on the server; the profile's own token reads its whole watched state, its resume points, its ratings and its Watchlist. With the owner signed in to Plex, the administrator's import reads the accounts with their own tokens too, but not their Watchlists (see [Signing in to Plex as the owner](#signing-in-to-plex-as-the-owner)). Without it, it tells each account other than the owner that gave nothing to import this way.

**For app developers:**

- `POST /admin/api/account/server-import/plex/sign-in` starts the caller's sign-in, dropping the one they held, and answers `link`, Plex's sign-in page with the PIN's code, and `expiresAt`.
- `POST /admin/api/account/server-import/plex/sign-in/poll` answers 202 and `pending` until Plex signs the caller in, then the account: `name`, `profiles` (`id`, `name`, `managed`, `protected`), `profile`, the one read, and `servers` (`id`, the machine identifier, `name`, `owned`, `preselected`). An expired or ended sign-in is `plex_sign_in_ended` (409).
- `POST /admin/api/account/server-import/plex/profile` takes `profile` and `pin`, and answers the account; a PIN Plex refuses is `plex_profile_pin_refused`.
- `DELETE /admin/api/account/server-import/plex/sign-in` drops the sign-in and signs it out.
- `POST /admin/api/account/server-import` with `kind` `plex`, `server` and `watchlist` (by default `true`) starts the import; a server the account does not reach is `plex_unknown_server`, one none of whose addresses answers as it `plex_server_unreachable` (the sign-in is kept), and plex.tv failing `plex_tv_failed` or `plex_tv_unreachable`.
- Polyfin asks plex.tv, with its client identifier in `X-Plex-Client-Identifier` and `X-Plex-Product` `Polyfin`: `POST /api/v2/pins?strong=true`, `GET /api/v2/pins/{id}` once a second at most, `GET /api/v2/user`, `GET /api/v2/home/users`, `POST /api/v2/home/users/{uuid}/switch?pin=…`, `GET /api/v2/resources?includeHttps=1&includeRelay=1`, and `DELETE /api/v2/users/signout`; and Plex's Discover service `GET /library/sections/watchlist/all?includeGuids=1`, 100 items a page, and `GET /library/metadata/{ratingKey}?includeGuids=1`. The server is read as the owner's import reads it, with the server's token, but neither `/accounts` nor the history.

### Letterboxd

Letterboxd has no API for this, but gives every member their data as a file: on Letterboxd, Settings › Import & Export, then Export your data, downloads a ZIP of CSV files. Choose **Letterboxd (export file)**, then the ZIP as it is, of 20 MB at most, under **Export file**, tick what to bring besides the movies watched, and choose **Import from Letterboxd**:

- **Watchlist to my Watchlist** (on by default): the movies of the Letterboxd watchlist go to the user's [Watchlist](tracking.md#the-watchlist), as a one-off app import's library does: they stay until started.
- **Ratings** (on by default): stars × 2 give Polyfin's rating from 0 to 10, so 3.5 stars is 7, and apps see `Rating` and `Likes` (a rating of 5 or more) as for a rating set in them. A movie rated in the diary only takes that rating, its latest.
- **Liked movies as favorites** (off by default): the movies liked on Letterboxd become favorites.

The movies of `watched.csv` and `diary.csv` are marked played. Each one's date is the latest `Watched Date` of the diary when it has one, else the date of `watched.csv`, and its play count the number of its diary lines, at least 1. Diary lines are joined to movies by name and year, as their links point to the diary entry. Only those files, `ratings.csv`, `watchlist.csv` and the liked movies' file of the `likes` folder are read, by their columns' names, so columns Letterboxd adds later do no harm; reviews, lists and the profile are not. A file without `watched.csv` or `diary.csv`, at the ZIP's root or in the folder holding them, is refused (`letterboxd_export_invalid`), and one over 20 MB, or naming more than 50,000 movies, too (`letterboxd_export_too_large`). A line whose name, link, year, date or rating is longer than 512 bytes is skipped. Polyfin reads the file in memory, never writes it to disk, and drops it once read.

**Matching:** The export names movies by their name, year and Letterboxd link only. Each one is searched as a movie by its name in the server's metadata addons, the searches local files use (see [Matching](local-folders.md#matching)), and matches only when they find exactly one movie of that name and year. Searches run one at a time, and each name is searched once per import, whatever the files it is in. Without a metadata addon that searches movies, nothing can match: the form says so before the upload. The result lists the movies not found, with their Letterboxd link and why: **Not found** (no movie of that name and year, a movie listed under another year or name included), **Several movies of that name and year**, or **Search failed**.

**Merging:** As the other imports do (see [How the data merges with Polyfin's](#how-the-data-merges-with-polyfin-s)): played marks, ratings and favorites are only added, a title keeps Polyfin's date when it is later and the larger play count, and a rating Polyfin already has stays. Importing the same export again adds nothing twice.

**Progress:** One import into an account runs at a time: an export sent while another is being uploaded or imported into the same account is refused (`server_import_running`) before it is read. While the movies are searched, the section shows how many were; then it counts the movies marked played, the titles added to the Watchlist, the ratings and favorites set, and lists the movies not found. The import and its result are kept in memory, as an app import's are (see [Progress of an app import](#progress-of-an-app-import)).

An administrator can run it for a user: **Import from Letterboxd**, at the top of **Users**, asks **Into the account of** which user, then takes the file the same way. The setting that turns the users' own imports off does not turn off the administrator's.

**For app developers:** `POST /admin/api/account/server-import`, or `/admin/api/users/{id}/server-import` for an administrator, takes the export as `multipart/form-data`: `kind` (`letterboxd`), `file`, and `watchlist`, `ratings` and `likesAsFavorites` (`true` or `false`, by default `true`, `true` and `false`), and answers `import`: `movies`, `searched`, `added` (`played`, `watchlist`, `ratings`, `favorites`), `unmatchedCount` and `unmatched` (`name`, `year`, `link`, `reason`: `not_found`, `ambiguous` or `search_failed`). `GET /admin/api/users/{id}/server-import` answers `searchable` and the user's Letterboxd `import`.

## Importing from an app

Apps built on addons, as Polyfin is, keep what their users watched in an account: Polyfin imports from Nuvio and Stremio. Under **My account**, **Import from an app** lets each user bring theirs over: the movies and episodes they played, their resume points, the app's library, and the app's addons they choose.

1. Choose the **App**, tick **Stay connected** to have it imported again on its own, every 6 hours by default (see [Staying connected](#staying-connected)), and sign in to it, the way that app signs in (see [Nuvio](#nuvio) and [Stremio](#stremio)). Polyfin reads the account once, then signs out, but for a Stremio key you paste, and unless you stay connected.
2. **Profile** lists the account's profiles by name, with what each watched, resumed and keeps in its library: choose the one to import. A Stremio account without profiles has one: **The whole account**. A Stremio profile with a PIN asks for it before it shows what it holds (see [Stremio](#stremio)). Under **Addons**, tick those to add, if any (see [The app's addons](#the-app-s-addons)).
3. **Import this profile** imports it into the user's account, in the background. The section then shows the import, and its result: what was added, the titles not found, and the addons added.

Polyfin keeps what it read of the account in memory only, while the profile is chosen, and for 5 minutes at most: after that, or after **Sign in again**, it is gone, and the import asks to sign in again (`app_import_expired`). The password is used for that sign-in only: it is never kept, nor logged. The key or the session the app gives is used for that sign-in only too, unless you stay connected, or a Stremio profile asks for its PIN, which needs it while the profile is chosen; a session Polyfin opened for a connection or a PIN and that is not imported in those 5 minutes is signed out, as it is when Polyfin stops or the user is deleted meanwhile.

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

- What is kept: for Stremio, the account's auth key: the one Polyfin got by a link code or a password is not signed out at the end of the import, and a key you paste is kept as it is; for a profile with a PIN, the session Polyfin opened with the PIN as well, never the PIN. For Nuvio, the backend's address, the profile chosen, and the session's refresh token, saved again each time Nuvio renews it, at every import. The password is never kept. One connection per app per user: signing in again with **Stay connected** replaces it, and signs out the sessions Polyfin opened before.
- How it is protected: it is stored in the database encrypted with `POLYFIN_SECRET_KEY`, as tracking tokens are (see [Configuration](configuration.md)), never shown, never sent back to the browser, and never logged. Without the key, it is stored as it is, and **System › Health** warns about it. A connection the key cannot decrypt counts as none: sign in again.
- The imports: at that interval, Polyfin reads the account again and imports the profile chosen when connecting, with the same rules as the first import: played marks only added, the newer resume point wins. The library replaces what the connection put on the Watchlist: a title taken off it leaves the Watchlist, unless another list holds it. Disconnecting takes off the Watchlist the titles the connection alone held; those a one-off import brought stay until started. The addons are chosen once, at the first import: later imports never install any. Polyfin only reads the account: nothing is ever written to the app.
- Status: under **Import from an app**, **Connected apps** shows each connection: the account, by the email the app gives, who connected it and when, the last import with what it added, and when the next one runs. **Import now** imports at once. When the app refuses what Polyfin kept, a key signed out or a session ended, the connection stops, says so, and is not imported until you sign in again with **Stay connected** ticked. An app that does not answer, or asks to wait, is tried again at the next import, and nothing is told. When the profile chosen is no longer in the account, or, for Stremio, asks for a PIN Polyfin does not have, one set or changed since it was chosen, the last import says so, and nothing is imported until you sign in again to choose it.
- Told when it stops: the connection stopping is told once, as it stops, to the user's notification targets and the server's that chose **Connection stopped** (see [Stopped connections](notifications.md#stopped-connections)), and **System › Health** lists the connections stopped, by user and app. Signing in again, then a new refusal, tells again.
- Disconnecting: **Disconnect** forgets everything kept, after signing out the sessions Polyfin opened: the Stremio key it got by a link code or a password, a Stremio profile's session it opened with the PIN, and the Nuvio session. A key you pasted is only forgotten, as signing it out would sign your browser out. What was imported stays, but for the titles of the Watchlist the connection alone held, which leave it. Deleting the user signs out the same sessions, in the background (see [Deleting a user](#deleting-a-user)).
- Who: users connect their own accounts while **Users may import their watch history from another server or app**, under **Settings › Security**, allows it: turned off, their connections are paused, still shown, and they may still disconnect them. An administrator can connect an app for a user from **Import from an app**, at the top of **Users**, as for the import; the page lists the connections of the user chosen, with **Import now** and **Disconnect**. An administrator's own connections are never paused.

Staying connected to Stremio deepens the risk of reading it (see [Stremio](#stremio)): its terms forbid extracting its data by automated means, and reading an account again and again is more plainly automated than a one-off import, the more so at a short interval.

**For app developers:**

- `GET /admin/api/account/app-import` also answers `connections`, each with its `app`, `account` (`null` when the app gives none), `connectedAt`, `connectedBy` (`id` and `name`, `null` once that account is deleted), `problem` (`sign_in_refused`, or `null`), `paused`, `importing`, `lastImport` (`at`, `read`, `played`, `resumed`, `watchlist`, `unmatched` and `problem`: `unreachable`, `rate_limited`, `unexpected`, `profile_missing`, `profile_locked` (a Stremio profile asking for a PIN Polyfin does not have), `sign_in_refused`, `internal`, or `null`) and `nextImportAt`, the last import, or the connection, plus the interval set (`null` while stopped or paused), and `importEveryMinutes`, the interval. What a connection signs in with is never answered.
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

A Stremio account keeps a library: for each title, whether it was watched and when last, the resume point of the video played last, and whether it is in the user's library; and the addons the user installed. Stremio's supporters have up to 6 profiles, each with its own library and addons; an account without profiles keeps one for the whole account. Polyfin reads it all at once, with the calls Stremio's own apps make.

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

Profiles: **Profile** lists the profiles of a supporter's account by name, in Stremio's order, each with what it watched, resumed and keeps in its library, and **Addons** offers the chosen profile's addons. Polyfin reads each profile as Stremio's apps switch to it: the account's own with the account's key, any other with a session of the profile's own that Stremio opens for the account, which Polyfin signs out once the profile is read. An account without profiles keeps **The whole account**.

- A profile with a PIN is listed with "(PIN)" after its name, and no counts: **PIN**, then **Open this profile**, reads it. The PIN goes to Stremio only, to open the profile's session: Polyfin never keeps it, logs it, nor sends it back. A wrong PIN says so (`stremio_pin_refused`). Until a profile is chosen and imported, or **Sign in again**, the account's session stays open, for 5 minutes at most, then is signed out, but for a key you pasted.
- **Staying connected** to a profile imports that profile again each time. A profile without a PIN is opened again for each import, then signed out; for a profile with a PIN, Polyfin keeps the session it opened with the PIN, never the PIN, and reads the profile with it. When the profile is removed from the account, the last import says it is no longer there; when a PIN is set on it since, or Stremio refuses the session kept, as a new PIN may end it, the last import asks to sign in again to choose it with its PIN. Either way, nothing is imported until you sign in again.
- A connection made before Polyfin read profiles keeps reading with the account's own key, as it did: the account's own profile.

Reading a Stremio account has its risks:

- Stremio's account API is not documented. Polyfin calls it as Stremio's own apps do; Stremio may change it, or turn other clients away, without notice. Its profiles were read from Stremio's web app alone, not tried against an account with several: that each profile's library and addons are read with its own session, and that a profile's session ends when its PIN changes, are how that app behaves as far as it shows.
- Stremio's General Terms (section 5.4.1) forbid using Stremio for "monitoring, copying or extracting any information from Stremio, including its contents, graphic user interface or Database whether done mechanically or by automated means". A user reading their own account with their own key is a grey zone: nothing says it is allowed. Staying connected, which reads the account again at the interval set, every 6 hours by default, makes that grey zone more pronounced, the more so as the interval is short.
- Stremio's own export of an account's data, under its settings, is reported broken: its library repeats one title many times (Stremio/stremio-bugs#614). Polyfin does not read it.

Users who connected Trakt in Stremio can import their history from Trakt as well (see [Tracking services](tracking.md)).

**For app developers:**

- `POST /admin/api/account/app-import/stremio/link` asks for a link code, and answers `code` and `link`, the address to approve it at, never a key. A user waits for one code at a time, for 5 minutes.
- `POST /admin/api/account/app-import/stremio/link/poll` takes `stayConnected`, answers `202` and `pending` until the code is approved, then reads the account, signs out unless asked to stay connected, and answers it as `…/nuvio` does. A code expired, already used, or never asked for is `stremio_link_expired`.
- `POST /admin/api/account/app-import/stremio/password` takes `email`, `password` and `stayConnected`, and `POST /admin/api/account/app-import/stremio/key` takes `key` and `stayConnected`; both answer the account read. A refused email or password is `stremio_sign_in_refused`, a refused key `stremio_key_refused`, Stremio not answering `stremio_unreachable`, and an answer Polyfin does not read `stremio_unexpected`.
- Each profile of the account read has `locked` `true` when it asks for its PIN: it has no counts nor addons until `POST /admin/api/account/app-import/stremio/unlock` opens it, which takes `key`, the account held, `profile`, its `id`, and `pin`, and answers the account held again, the profile read. A wrong PIN is `stremio_pin_refused`, a profile that is not the account's or asks for no PIN `invalid_request`, and the account no longer held `app_import_expired`. Importing a profile still locked is `invalid_request`.
- An unmatched series whose episodes could not be read has the `reason` `episode_list`.
- Polyfin asks for a link code with `GET https://link.stremio.com/api/v2/create?type=Create` and polls it with `GET https://link.stremio.com/api/v2/read?type=Read&code=…`, signs in with `POST https://api.strem.io/api/login` (`email`, `password`, `facebook` `false`), reads the account's email and profiles with `POST https://api.strem.io/api/getUser` (`premiumPrefs.userProfiles`, each with its `_id`, `name`, `isMaster` and `hasPin`), opens a profile's session with `POST https://api.strem.io/api/premiumPrefs/authenticateProfile` (`type` `AuthenticateProfile`, `profileId`, `pin`, `null` for none), which answers its `authKey`, reads the library with `POST https://api.strem.io/api/datastoreGet` (`collection` `libraryItem`, `ids` `[]`, `all` `true`) and the addons with `POST https://api.strem.io/api/addonCollectionGet` (`update` `true`), with the account's key or the profile's, and signs out with `POST https://api.strem.io/api/logout` the keys it obtained, never a key pasted. Keys and PINs go in bodies, `authKey` and `pin`, never in an address.

## User groups

To give several accounts the same limits, such as a family's children, an administrator puts them in a group: a change to the group reaches all of them at once, rather than one page at a time.

**Users › Groups** lists the groups with how many members each has, and **Create a group** asks for a **Name** and **Settings of**: a new user, as with **Create a user**, or a user whose settings the group copies when it is created. A group's page holds the sections of a user's page that set limits, each saved on its own as on a user's page:

- **Access**: **Can use conversion (transcoding)**, **Can download**, **Can add their own addons**, **Can manage collections**, **Can manage subtitles**, **Can share titles** and **Can play torrent streams**. **Administrator**, **Show on the sign-in screen**, **Disabled** and **Ends on** stay each user's own;
- **Playback and access**, with the **Quality group**;
- **Parental control**, **Visible libraries**, **Blocked genres** and **Allowed hours**.

Its page also lists its **Members**, with **Add members**, and offers **Reset to defaults** (see [A group's limits](#a-group-s-limits)) and **Delete group**.

### Members

- A user belongs to one group at most, chosen with **Group**, in the **Access** section of their page, or **No group**, the default. Joining a group asks first, as it replaces the user's values with the group's. Adding a user who is in another group moves them.
- Administrators cannot join a group, as their permissions differ: making a member an administrator takes them out of it, and they keep its values as their own.
- **Users** shows each user's group, and filters the list by group.
- **Create a user** and **Create an invite link** can name a group: the accounts made join it and follow it (see [Invite links](#invite-links)).

### Following the group

**Follow the group**, in the **Access** section of a member's page, is on by default. While it is on, the sections the group holds show its values, read only, with "Set by the group Kids.", and a change to the group reaches the member at once, as saving their own page would: their apps already signed in have the new limits from their next request, downloads and allowed hours included, and a member who may no longer share titles loses their share links.

Turned off, the member stays in the group, listed as **Own settings**, keeps the group's current values as their own, and their page can change them; the group's later changes pass them by. Turned on again, after asking, they take the group's values back.

Leaving a group, with **No group**, or deleting it, works as turning **Follow the group** off: the members keep its values as their own. An invite link that named a deleted group creates accounts in no group.

**Turn off downloads for everyone** (see [Transcoding](transcoding.md)) also turns **Can download** off on every group, so that its members keep following it.

### Groups and Jellyfin apps

- `GET /Users` and `GET /Users/{id}` show a member's policy with the values that apply to them, the group's while they follow it.
- `POST /Users/{id}/Policy`, from an administrator's app, changes the fields no group holds as for any user, such as `IsAdministrator`, `IsDisabled` and `IsHidden`. A field the group holds sent with another value than the group's detaches the member, as turning **Follow the group** off would, and the activity log says so, as "sam no longer follows the group Kids: changed from Jellyfin Web": the policy is kept whole, and nothing the app sent is lost. A policy sent back as the app read it, with only `IsDisabled` changed for instance, leaves the member following. A library the group hides that the server no longer has does not count as a change, as apps cannot name it.
- Jellyfin has no groups: apps show neither the group nor **Follow the group**.

The activity log records groups created, changed, naming the sections changed, and deleted, and members added, removed, or no longer following their group (see [Activity log](administration.md#activity-log)).

**For app developers:**

- `GET /admin/api/groups` lists the groups, sorted by name: `id`, `name` and `memberCount`.
- `POST /admin/api/groups` creates one from `name` and `copyFrom`, a user's `id`, or null or left out for a new user's settings; it answers 201 with the group. `GET /admin/api/groups/{id}` answers the group: `id`, `name`, `memberCount`, and the fields of a user's JSON the group holds: `transcoding`, `downloads`, `personalAddons`, `collectionManagement`, `subtitleManagement`, `canShare`, `maxPlaybacks`, `maxBitrate`, `qualityGroup`, `syncPlay`, `liveTv`, `remoteControl`, `liveTvManagement`, `parentalControl`, `hiddenLibraries`, `blockedGenres` and `accessSchedules`.
- `PATCH /admin/api/groups/{id}` takes `name` and any of those fields, as `PATCH /admin/api/users/{id}` takes them, with the same errors, and answers the group. `DELETE /admin/api/groups/{id}` deletes it (204).
- `GET /admin/api/groups/{id}/members` lists the members, sorted by name: `id`, `name` and `followGroup`. `POST /admin/api/groups/{id}/members` with `userIds` adds those users, all of them or none, and answers the members.
- A user's JSON has `groupId` and `groupName`, null for none, and `followGroup`. `PATCH /admin/api/users/{id}` takes `groupId`, a group's `id` or `""` for none, and `followGroup`. A field the group holds, sent for a member who follows it without `followGroup` false in the same change, is 409 `follows_group`. `POST /admin/api/users` and `POST /admin/api/invites` take `groupId`.
- Errors: 409 `admin_cannot_join_group` for an administrator joining, or added, or created in a group; 400 `invalid_group` for a group that does not exist; 400 `invalid_name` and 409 `name_taken` for a group's name, from 1 to 64 characters, unique without regard to case; 400 `invalid_copy_from` for a user to copy who does not exist; 404 `not_found` for a group or a user to add that does not exist.

## Resetting settings to their defaults

Rather than finding each value and setting it again, an administrator can put a user's limits, or a group's, back to a new user's settings, and each user can put back the server's defaults for what they chose themselves.

### A user's limits

**Reset to defaults**, at the top of a user's page, puts the sections that set limits back to a new user's settings, the ones **Create a user** gives: the permissions of **Access**, **Playback and access** with the **Quality group**, **Parental control**, **Visible libraries**, **Blocked genres** and **Allowed hours**. It asks first, listing the sections that change and, for **Access** and **Playback and access**, the settings that change in them, or saying that nothing changes.

- An administrator gets an administrator's settings: **Can control other users' apps**, **Can manage collections**, **Can manage subtitles** and **Can record Live TV** on, as **Create a user** gives them with **Administrator**. Any other user gets them off.
- Kept: the name and password, **Administrator**, **Show on the sign-in screen**, **Disabled**, **Ends on**, the group, the **Metadata language**, the **Web player theme**, the **Audio region**, the user's own addons, connections and tracking accounts, and everything they watched.
- It applies as saving does: the user's apps already signed in have the new limits from their next request. A new user may not share titles, so a user who could loses their share links (see [Share links](#share-links)).
- A member who follows their group has its values: their page does not offer the button, and the group's page resets them. A member with **Own settings** gets a new user's settings and stays in the group, still not following it.
- The activity log records who reset whose settings, as "alex reset the settings of sam to their defaults".

### A group's limits

**Reset to defaults**, at the top of a group's page, does the same for the group's sections: they take the values a group starts with when **Create a group** makes it from a new user's settings. It asks first, listing what changes, and says how many members follow the group, as the change reaches them at once, as any change to the group does. Members with **Own settings** keep theirs. The group keeps its name and its members. The activity log records it as "alex reset the settings of the group Kids to their defaults".

### Your own preferences

**Reset my preferences**, at the top of **My account**, puts back the server's defaults for what the user chose themselves, after asking first:

- the **Metadata language** goes back to **The server's** (see [Metadata language](#metadata-language));
- the **Web player theme** goes back to the server's, unless the administrator turned off **Let them change it** (see [Web player theme](#web-player-theme));
- the **Audio region** goes back to **None** (see [Audio region](#audio-region));
- every row of **Home rows** is shown again (see [Home rows](#home-rows));
- the tracks the user chose on series and movies are forgotten (see [Track choices](#track-choices));
- the preferences Jellyfin apps saved in the account are forgotten: the audio and subtitle languages and the subtitle mode, the home screen (the order of the libraries and those left out), and the display preferences, such as each library's sorting and view. The user has a new user's again, which their apps already signed in read from their next request. Settings an app also keeps on the device stay there: jellyfin-web keeps a copy of its home sections in the browser, and still shows them on that browser.

It never touches what the user watched, their favorites, resume points and hidden titles, their connections, tracking accounts and imports, or their limits, which are the administrator's.

**Compared with Jellyfin:**

- Jellyfin has no such button. jellyfin-web's only resets are a user's password, which an administrator's "Reset Password" empties ([UserPasswordForm.tsx, lines 116 to 137](https://github.com/jellyfin/jellyfin-web/blob/c06cb17b274c/src/components/dashboard/users/UserPasswordForm.tsx#L116-L137)), the filters of a library ([LibraryToolbar.tsx, lines 128 to 152](https://github.com/jellyfin/jellyfin-web/blob/c06cb17b274c/src/apps/modern/features/libraries/components/LibraryToolbar.tsx#L128-L152), and [filterdialog.js, lines 302 to 309](https://github.com/jellyfin/jellyfin-web/blob/c06cb17b274c/src/components/filterdialog/filterdialog.js#L302-L309) in the older views), and an item's metadata, whose "Reset" in the metadata editor empties the form before it is saved ([metadataEditor.js, lines 305 to 341](https://github.com/jellyfin/jellyfin-web/blob/c06cb17b274c/src/components/metadataEditor/metadataEditor.js#L305-L341)). In Jellyfin, an administrator sets each limit of a user again by hand, and a user each preference.
- Jellyfin apps see a reset user's new policy in `GET /Users` and `GET /Users/{id}`, and the activity log's entries have Jellyfin's types: `UserPolicyUpdated`, about the user, for a user's reset, and Polyfin's `GroupChanged` for a group's.

**For app developers:**

- `GET /admin/api/users/{id}/defaults` answers the fields of a user's JSON a group holds (see [User groups](#user-groups)) as **Reset to defaults** would set them, changing nothing. `POST /admin/api/users/{id}/reset` sets them and answers the user. Both are 409 `follows_group` for a member who follows their group, and 404 `not_found` for a user who does not exist.
- `GET /admin/api/groups/{id}/defaults` and `POST /admin/api/groups/{id}/reset` do the same for a group; the reset answers the group, and is 404 `not_found` for a group that does not exist.
- `POST /admin/api/account/preferences/reset`, for any user signed in to the admin app, resets their own preferences and answers 204. Their Jellyfin apps then read a new user's `Configuration` in `GET /Users/{id}` and `GET /Users/Me`, and `GET /DisplayPreferences/{id}` answers the defaults it gives a user who never saved any.

## Ending an account

An account made for a friend's weekend need not stay until someone remembers to delete it. **Ends on**, in the **Access** section of a user's page, takes a date, kept with **Save date**; **Never**, the default, clears it. From that date, in the server's time zone, the account is refused as a disabled one is: it cannot sign in, from a Jellyfin app or Silo's apps, Quick Connect (a Silo TV's code included) or the admin app, and its devices and admin sessions are refused. Unlike **Disabled**, an end signs nothing out: everything the account has stays, its devices included, and a later date, or **Never**, brings it back as it was.

- **Users** lists each account's end date, as "Ends on October 20, 2026", and shows **Ended** once it has passed, as does the user's page.
- The activity log records each account that ends, once, as "The account of sam ended", within the hour: a scheduled task looks for them every hour (see [Scheduled tasks](administration.md#scheduled-tasks)). An account given a later date is recorded again when it reaches it.
- Jellyfin apps see an ended account as disabled. Jellyfin has no end date: an app sending back the policy it read leaves **Disabled** off, and only the admin app changes the date.
- An administrator's account never ends, so that the last administrator is never locked out: an administrator cannot be given an end date, and making a user an administrator clears theirs.
- An ended account's Live TV recordings, new episode notifications, weekly summary and imports from an app stop, as a disabled one's do. Its devices still count as unused for **Sign out devices unused for (days, 0 = never)** (see [Signing out unused devices](#signing-out-unused-devices)).

**For app developers:**

- `GET /admin/api/users` and `PATCH /admin/api/users/{id}` answer `endsOn`, a `YYYY-MM-DD` date in the server's time zone or null for never, and `ended`, true once that date has come. `PATCH` takes `endsOn`, a date or null; anything else is 400 `invalid_ends_on`, and a date for an administrator, or for a user made one in the same change, is 409 `administrator_cannot_end`.
- An ended account signs in as a disabled one does: 403 from `/Users/AuthenticateByName`, Quick Connect and `POST /admin/api/session` (`account_disabled`), and `denied` to a Silo TV's poll. Its access tokens answer 401, and its policy's `IsDisabled` is true.

## Invite links

An invite link lets the people an administrator sends it to create their own account, so that nobody has to choose a password for them and send it in a message. **Create an invite link**, at the top of **Users**, asks for:

- **Accounts it may create**: from 1 to 100, 1 by default. One link can serve a whole family.
- **Expires**: after 1, 7 (the default), 30 or 90 days, or **Never**.
- **Accounts end**: **Never**, the default, or 1, 7, 30 or 90 days after each is created. Each account the link creates is given that end date (see [Ending an account](#ending-an-account)).
- **Settings of**: the user whose settings the new accounts copy, or a new user, as with **Create a user**.
- **Group**: **No group**, the default, or a group the new accounts join and follow (see [User groups](#user-groups)).

With a model, a new account copies everything the model's page sets except the name, the password, **Administrator**, **Ends on** and **Group**: the **Access** permissions, the **Playback and access** limits with the **Quality group**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours**, and whether it shows on the sign-in screen. The model is read when each account is created, so a change to the model reaches the accounts created after it. Without a model, a new account has the settings of a user made with **Create a user**, hidden from the sign-in screen. With a group, the account then takes the group's values for the settings it holds, and follows it: a change to the group reaches the accounts already created too. An invite link never makes an administrator.

The link is shown once, with **Copy link**: Polyfin keeps only a hash of it and cannot show it again. It starts with the **Public address** (**Settings › Notifications**) when one is set, and with the address the admin app is open at otherwise.

**Invite links**, under the accounts, lists each link with the accounts it created out of those it may create, whose settings it gives, when it expires, when its accounts end, who created it, and its state: **Active**, **Used up**, **Expired** or **Revoked**. **Revoke** stops an active link at once; the accounts it created are kept. Deleting the user a link copies revokes the link too.

### The guest's page

The link opens a page of the admin app, `/admin/invite/…`, that needs no account, in the language the guest chooses among the admin app's. It shows the server's name and asks for **Your name**, a **Password** and **Confirm password**, under the rules of **Create a user**. Once the account is created, the guest lands on the [web client](web-client.md), signed in. Without the web client, they land on the admin app instead, signed in, where a member sees their own pages.

A link that is used up, expired or revoked says so, and so does an address that is no link at all.

Each account created through a link is written to the activity log, as "sam joined through alex's invite", followed by "sam joined the group Kids" when the link names a group, and is told to the notification targets that chose **User joined** (see [Notifications](notifications.md#events)).

### How links are kept safe

- A link's token is the invite's ID followed by 32 random bytes, as many as an admin session's. Polyfin stores only the SHA-256 hash of the random part, and compares it in constant time.
- The guest's requests count toward the client's failed sign-ins: a token that is no link's counts as a wrong password does. After too many failures, the client is refused for a while (429), as on the sign-in page.
- A link's uses are counted in the transaction that creates the account, with the link locked: two guests on the last use of a link create one account, and the other is told the link is used up. A name or password that is refused uses nothing up.
- The guest sends a name and a password only. Anything else in the request is ignored: the account's settings come from the link.

**Compared with Jellyfin:**

- Jellyfin has no invite links: its administrators create every account and choose its password.

**For app developers:**

- The admin API serves the links to administrators:
  - `GET /admin/api/invites` lists them, the newest first: `id`, `maxUses`, `uses`, `expiresAt` (null for never), `createdAt`, `revokedAt`, `model`, `group` and `createdBy` (each `id` and `name`, or null; `createdBy` is null once that user is deleted, `group` once that group is), `state`: `active`, `used_up`, `expired` or `revoked`, and `accountsEndInDays` (null for never).
  - `POST /admin/api/invites` creates one from `maxUses` (1 by default, up to 100), `expiresInDays` (7 by default, up to 365, `null` for never), `modelUserId` (null or left out for a new user's settings), `groupId` (null or left out for none) and `accountsEndInDays` (up to 365, null or left out for never). It answers 201 with the link as listed, plus its `token` and its `url`, which is null without a public address: the link is then `/admin/invite/<token>` on the server's address. The token is never answered again. Errors: `invalid_invite_uses`, `invalid_invite_expiry`, `invalid_invite_model`, `invalid_group` and `invalid_invite_account_end`.
  - `POST /admin/api/invites/{id}/revoke` revokes one, and answers it.
- The guest's page uses two routes that need no session:
  - `GET /admin/api/invite/{token}` answers `serverName` while the link may create an account.
  - `POST /admin/api/invite/{token}` with `name` and `password` creates the account. It answers 201 with `user` (`id`, `name`, `isAdministrator`, `imageTag`) and `webClient`. Without the web client (`webClient` false), the answer opens an admin session, as signing in does. With it, the page signs in with `POST /Users/AuthenticateByName` as jellyfin-web does, naming jellyfin-web's app and device, and keeps the session where jellyfin-web 12.4 reads it, in `jellyfin_credentials` in the browser's local storage.
  - A token that is no link's answers 404 `invite_unknown`, and a link used up, expired or revoked 410 `invite_used_up`, `invite_expired` or `invite_revoked`. A refused name or password answers as creating a user does: `invalid_name`, `invalid_password` or `name_taken`. Too many failures answer 429 `too_many_attempts`, with `Retry-After`.

## Share links

A share link lets a member give one movie or one episode to someone who has no account, for a while, without an administrator creating an account. The guest sees that title alone.

Two switches allow it, both off by default:

- **Allow share links**, under **Settings › Security**, for the server. Turned off, no link can be made, and every link is refused with 410, without being deleted: turning it on again makes the links work again until they expire.
- **Can share titles**, under **Access** on a user's page, for each user, administrators included. An invite link's model passes it on, as the other **Access** permissions.

A user who loses **Can share titles**, is disabled, ends (see [Ending an account](#ending-an-account)) or is deleted loses their links at once: they are revoked, or deleted with the account.

### Making a link

Under **My account › Shared links**, **Share a title** searches the libraries the user sees. A movie is shared as it is; a series opens its episodes, and one episode is shared. A series, a channel or a recording is never shared. Parental control, blocked genres and the titles the user hid apply: a title the user may not see cannot be shared. On a movie's or an episode's page in the web client, **Share** opens the same form with the title chosen (see [Web client](web-client.md)).

The form asks for:

- **Expires after**: 1 hour, 1 day (the default), 7 days or 30 days.
- **Viewers at once**: from 1 (the default) to 5 guests watching through the link at the same time.

**Create link** shows the link once, with **Copy link**: Polyfin keeps only a hash of it. It starts with the **Public address** (**Settings › Notifications**) when one is set, as an invite link does, and with the address the admin app is open at otherwise.

**My account › Shared links** lists the user's links, with the title, when each expires, how many times it was played, its state (**Active**, **Expired** or **Revoked**) and **Revoke**. Administrators see a user's links in the **Shared links** section of that user's page, and can revoke them. Links that ended more than 30 days ago are deleted.

### What the guest sees

The link opens a page Polyfin serves at `/share/…`, which needs no account, in the first language Polyfin speaks among those the guest's browser prefers (else the server's language). It shows the server's name, the title, its artwork and overview, when the link expires, a player, and the text subtitles of the version played, which the player's captions menu offers. The title's name, overview and artwork are those the sharer sees, in their [metadata language](#metadata-language). It lists no library, no search and no other title. A link expired, revoked, turned off or unknown says so.

The page plays HTTP Live Streaming, natively where the browser can, else with [hls.js](https://github.com/video-dev/hls.js) 1.6.16, which Polyfin serves itself: the page loads nothing from elsewhere.

### Limits

The guest plays as the sharer would, with less:

- Everything goes through Polyfin: the guest is never sent to the source, as stream addresses can hold keys. The page gets HLS playlists and segments only: there is no download.
- The version played is the one the sharer's version rules choose (see [Playback](playback.md)). Video is sent at 1080p at most, and never above the sharer's [quality group](#quality-groups) or **Maximum quality**: a taller version is converted down, and plays only when the sharer may have video converted.
- A guest counts as one of the sharer's playbacks for **Playbacks at once**, and a conversion for **Video conversions at once** (see [Playback](playback.md#video-conversions-at-once)). At the limit, the page says that the owner of the link is already watching on as many devices as allowed. A link takes no more guests at once than its **Viewers at once**.
- The sharer's **Allowed hours** apply: outside them, nothing plays.
- A guest changes nothing of the sharer's played marks, resume points or Next Up. Their plays count in the sharer's statistics, marked **Shared**, and Home in the admin app shows them under the sharer's name, with "Shared link" as the app.

Each link made, revoked or expired, and each guest playback, is written to the activity log, as "A guest played Example Movie through the link shared by alex" (see [Administration](administration.md#activity-log)). The event **Shared link played** tells the sharer's notification targets and the server's, at most once an hour per link (see [Notifications](notifications.md#events)).

### How share links are kept safe

- A link's token is made and checked as an invite link's: its ID followed by 32 random bytes, only the SHA-256 hash of which is kept, compared in constant time (see [How links are kept safe](#how-links-are-kept-safe)).
- A token that is no link's counts as a failed sign-in of the client; after too many, the client is refused for a while (429).
- The token opens the page's routes only: the Jellyfin API, the admin API and the web client refuse it as any unknown token. The guest's play session is signed for no one, so the API's own HLS routes refuse it too.
- What is converted and sent is kept by the server when the playback starts: the guest's requests cannot change it.

**Compared with Jellyfin:**

- Jellyfin has no share links: watching needs an account.

**For app developers:**

- The admin API serves a user's own links: `GET /admin/api/account/share-links` answers `allowed` (whether the user may make one now) and `links`; each link has `id`, `user` (`id`, `name`), `item` (`id`, `kind`: `movie` or `episode`, `name`), `maxViewers`, `createdAt`, `expiresAt`, `revokedAt`, `plays` and `state`: `active`, `expired` or `revoked`. `POST /admin/api/account/share-links` makes one from `itemId`, `expiresInHours` (1, 24 by default, 168 or 720) and `maxViewers` (1 by default, up to 5), and answers 201 with the link, its `token` and its `url` (null without a public address: the link is then `/share/<token>`). Errors: 403 `share_links_off`, 400 `not_shareable`, `invalid_share_expiry` or `invalid_share_viewers`, 404 `not_found`. `POST /admin/api/account/share-links/{id}/revoke` revokes one.
- `GET /admin/api/account/share-titles?search=…` finds movies and series to share, `GET /admin/api/account/share-titles/{id}` describes one title and `GET /admin/api/account/share-titles/{id}/episodes` lists a series' episodes.
- Administrators: `GET /admin/api/users/{id}/share-links` and `POST /admin/api/share-links/{id}/revoke`. Users carry `canShare`, and the settings `shareLinks`.
- The guest's page uses `GET /share/{token}`, `GET /share/{token}/image/primary` and `…/backdrop`, `POST /share/{token}/play` (which answers `session`, or `error`: `limit`, `viewers`, `hours`, `busy` or `unavailable`), `GET /share/{token}/play/{session}/master.m3u8` with its playlists and segments, and `POST /share/{token}/play/{session}/progress` and `…/stop`. A link that opens nothing answers 404 `unknown`, or 410 `expired`, `revoked`, `off` or `unavailable`.

## Parental control

On a user's page under **Users**, the **Parental control** section can limit the user to titles up to a rating (**Maximum rating**). The administrator can also hide from them the movies or shows that have no rating (**Block unrated movies**, **Block unrated shows**). Administrators' Jellyfin apps can set the same limit from their user settings. For a member who follows a group, the group sets it, and an app that changes it detaches them from the group (see [User groups](#user-groups)).

### How a title's rating is found

- A title's rating is the certification the server's metadata addon gives, usually the US one, or its local one when it gives no other.
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

On a user's page under **Users**, three more sections hold three more settings, each with its own save button. Administrators' Jellyfin apps can also set them from their user pages. For a member who follows a group, the group sets all three, and an app that changes the visible libraries or the allowed hours detaches them from the group (see [User groups](#user-groups)).

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
- A Quick Connect sign-in is not checked, a Silo TV's code included, though the app's requests then are.
- Polyfin refuses hours outside 0 to 24, or ending before they start.
- In the admin app, nobody signs in outside their hours, and a member already signed in sees only who they are.

**Compared with Jellyfin:**

- Jellyfin serves direct streams without checking who asks; Polyfin also refuses a user's media outside their hours.
- jellyfin-web never sends hours outside 0 to 24 or ending before they start.

## Metadata language

**Metadata language** is the language a user reads titles in: their names, overviews and images, and their episodes'. Users set it in its own section under **My account**, and administrators in its own panel on the user's page under **Users**. **The server's**, the default, shows the descriptions of the server's addons without a language. The other choices are the languages the server's addons were given (see [Languages](addons-and-libraries.md#languages)). **Reset my preferences** puts it back to **The server's** (see [Your own preferences](#your-own-preferences)).

- A user under parental control or blocking genres gets their language too.
- A title's genres, ratings, people, versions and subtitles are the same in every language, so parental control, blocked genres, hidden titles and smart lists judge every user alike.
- A title that no addon in the language describes shows the server's description.
- When Polyfin speaks the language, the names it generates follow it too (see [Language of generated names](getting-started.md#language-of-generated-names)).
- An invite link's model user passes on their metadata language.
- The page of a link the user shares shows the title in their language: its name, overview, poster and backdrop (see [Share links](#share-links)).
- **Edit images** lists TMDB's images in their language first (see [Edit images](addons-and-libraries.md#edit-images)).

**For app developers:**

- Jellyfin sets a metadata language per library, and its apps send none per user: Polyfin reads the user's own, and the server configuration's `PreferredMetadataLanguage` stays `en`.
- Images come without a user. An image asked for with the tag of a user's language gets that image.

## Web player theme

**Web player theme** is the look of the web player, Polyfin's web client, for a user, on every browser they sign in with: **The server's**, the default, which names the server's theme (see [Themes](web-client.md#themes)), **LumaaGlaass**, **jellyfin-web's own look**, or **Custom** while the Custom theme has a text. jellyfin-web's own look is the lightest, for the browsers of TVs and older phones. Administrators set it in its own panel on the user's page under **Users**; users set their own in its own section under **My account**.

- **Let them change it**, beside the administrator's choice, is on by default. Off, the theme the administrator chose stays the user's: their **My account** shows it, read only, with "Your administrator chose your web player theme.", and refuses a change.
- **Reset my preferences** puts it back to the server's, unless **Let them change it** is off (see [Your own preferences](#your-own-preferences)).
- An invite link's model user passes on their theme and **Let them change it**.
- A browser too old for LumaaGlaass gets jellyfin-web's own look by itself, whatever the theme (see [Each user's theme](web-client.md#each-user-s-theme)).
- It applies to the web player only, in browsers and in the Jellyfin apps that show it; other apps draw their own screens.

**Compared with Jellyfin:**

- Jellyfin has no theme per user: its custom CSS is the server's, for everyone, and jellyfin-web keeps each user's display theme, such as Light, in the browser, not in the account.

**For app developers:**

- A user's JSON in the admin API has `webTheme`, `lumaaglaass`, `none` or `custom`, empty for the server's, and `webThemeLocked`, `true` while **Let them change it** is off; `PATCH /admin/api/users/{id}` sets them. A theme not offered is `400 invalid_web_theme`.
- `GET /admin/api/web-themes`, for any user signed in to the admin app, answers `serverTheme` and `themes`, those a user may be given: `lumaaglaass` and `none`, and `custom` while the Custom theme has a text.
- `GET /admin/api/account/web-theme` answers the caller's `webTheme`, `locked`, and the same `serverTheme` and `themes`; `PUT` with `{"webTheme": "none"}` changes it, `403 web_theme_locked` while **Let them change it** is off.

## Audio region

**Audio region** is the dub a user prefers where a language is dubbed apart for a region: **French (France)** or **French (Quebec)**, **Spanish (Spain)** or **Spanish (Latin America)**, **Portuguese (Brazil)** or **Portuguese (Portugal)**. Users set it in its own section under **My account**, and administrators in its own panel on the user's page under **Users**. **None**, the default, prefers no region. **Reset my preferences** puts it back to **None** (see [Your own preferences](#your-own-preferences)).

- It refines the user's audio language: the one they chose in their app, else the server's **Default audio language**, when that is the region's language. A user who chose French and Quebec gets the Quebec dub first; a user who chose English keeps English, whatever their region.
- A user with no audio language of their own and no server default gets the region's language, as the server's default would give it.
- In that language, the audio track of the region plays by default, then one of no known region, then one of another region; and with **Preferred audio language** in the version order, the versions carrying that dub come first among those in the language. How Polyfin reads a track's or a release's region is in [Regional audio](playback.md#regional-audio).
- An invite link's model user does not pass it on: it is the guest's own choice.

**For app developers:**

- The user's JSON in `/admin/api/users` holds `audioRegion`, a BCP 47 tag (`fr-CA`, `es-419`, `pt-BR`) or empty, and `audioRegions`, the tags offered; `PATCH /admin/api/users/{id}` takes `audioRegion`.
- `GET /admin/api/account/audio-region` answers the caller's own as `{"region": "fr-CA", "regions": [...]}`, and `PUT` with `{"region": "..."}` changes it. A tag not offered is 400 `invalid_audio_region`.
- Jellyfin's `AudioLanguagePreference` has no region, and Jellyfin apps neither show nor send this setting.

## Playback and access limits

On a user's page under **Users**, the **Playback and access** section holds five more limits, saved with **Save playback and access**. Administrators' Jellyfin apps also set them from the user's settings. For a member who follows a group, the group sets them, with the **Quality group**, and an app that changes one detaches them from the group (see [User groups](#user-groups)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Playbacks at once (0 = no limit)** | **Users › Playback and access** | 0, no limit (up to 20) | How many of the user's devices may play at once. |
| **Maximum quality** | **Users › Playback and access** | No limit | Caps the bitrate of everything the user plays. |
| **Live TV** | **Users › Playback and access** | On | Whether the user can see and play Live TV. |
| **Watch together** | **Users › Playback and access** | Create and join | Whether the user may create and join SyncPlay groups, only join them, or not watch together at all. |
| **Can control other users' apps** | **Users › Playback and access** | On for administrators, off for others | Lets the user see and control the apps of other users from theirs. |

The same section also holds **Quality group**; see [Quality groups](#quality-groups).

### Playbacks at once

Once that many of the user's other devices are playing, as **Sessions** lists them, DLNA devices showing the user's libraries included, one more device cannot start playing. jellyfin-web explains this as media that cannot be played at this time; Polyfin's web client script closes the generic error jellyfin-web shows over it (see [Web client](web-client.md#dashboard-links-and-single-sign-on)). A device already playing can go on to another title. A device making a [converted download](transcoding.md#converted-downloads) counts as playing while it runs.

Apps report their playback every 10 seconds, paused too. A device whose app reported nothing for five minutes stops playing, as if the app had reported the stop at the position it last reported: a tab closed while it played, a phone that lost its network, an app that crashed. Its resume point stays at that position, and it no longer counts here, nor in **Sessions** and **Now playing**. Its conversions end, and its torrent lingers then stops as after a stop.

**Compared with Jellyfin:**

- PlaybackInfo from the extra device answers Jellyfin's `RateLimitExceeded`.
- Jellyfin also stops a playback whose app reported nothing for five minutes, paused or not, at the position last reported. It checks every five minutes, so the playback stops five to ten minutes after the last report; Polyfin checks every minute, so five to six, as the playback holds a place here.
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

### Playing torrent streams

**Can play torrent streams**, in **Access** next to **Can download**, offers the user the torrents addons list, while **Play torrent streams** is on under **Settings › Torrents** (see [Torrents](torrents.md)). It is off for every user, administrators included, and in every group, until given. Without it, the user's versions hold no torrent, from the server's addons or their own, and a torrent version listed to another user is not found for them. Torrent versions never download, whatever **Can download** says.

### Can control other users' apps

**Can control other users' apps** is Jellyfin's `EnableRemoteControlOfOtherUsers`.

- Administrators have it when created, and when upgrading from an older Polyfin.
- Other users do not.
- A user made administrator later keeps the value they had.

## Quality groups

**Quality group**, on a user's page under **Users** in **Playback and access** next to the maximum quality, sets the highest resolution each user is offered. For a member who follows a group, the group sets it (see [User groups](#user-groups)).

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
- the others keep their order: the addons', or the one the [version order](playback.md#version-order) settings give.

When every version is taller, all are kept rather than none, the closest to the group first (720p before 1080p before 4K; versions of the same height keep their order, or are ordered by the version order settings). That way the least is read and decoded.

The group applies after the version order settings hide versions, and before they order them (see [Version order](playback.md#version-order)).

- The version played is converted down to the group, or to **Maximum quality of converted video** when that is lower, and to 1080p at most when the processor converts (4K on a GPU), as every conversion is. See [Transcoding](transcoding.md).
- A version that fits plays as it is, or remuxed, as before.
- A taller version is never sent at full size.
- A user who may not have video converted (their **Can use conversion** permission, or the server's **Conversion** switch, off) is refused taller versions, as other conversions are refused (`NoCompatibleStream`).

### Live TV, recordings and downloads

- Live TV channels follow the same rule once analyzed, as their height is only known then. So do recordings.
- A version's file downloads only when it fits. When none fits, the file is refused (403), as without the permission.
- With **Converted downloads** on, under **Settings › Conversion (transcoding)**, apps that ask for a smaller copy, such as Streamyfin, download a taller version converted down to the group, when the user may have video converted. See [Converted downloads](transcoding.md#converted-downloads).

**Compared with Jellyfin:**

- Jellyfin has no such setting. Apps neither show nor change it, and a policy they post keeps it.

**For app developers:**

- PlaybackInfo reports the conversion of a taller version as Jellyfin reports a resolution limit (`VideoResolutionNotSupported`), describing its video at the size sent.

## Two-step sign-in

Each user can ask for a second step after their password: a 6-digit code from an authenticator app on their phone, such as the one their phone's password manager includes. Once it is on, the password alone signs in nowhere.

### Turning it on

Under **My account › Two-step sign-in**, **Turn on** shows a new key, as a QR code and as text, with a link that opens an authenticator app on the same device. The QR code is drawn by Polyfin itself: the key goes to no other service. Scanning it adds the account to the app, named Polyfin. Two-step sign-in is on only once a code the app shows is entered: a key that was never confirmed signs in no one, and **Turn on** again makes a new one.

Then come ten recovery codes, shown this once, written like `7k2m-q9xd-4tbh-z0ce`: 16 letters and digits in four groups, without i, l, o or u, which look like other characters. Each signs in once in place of a code from the app, should the phone be lost, typed in capitals or not, with or without its dashes, or with spaces in their place. Polyfin keeps only their hashes, and can never show them again: keep them in a password manager, or printed. The section counts the codes left. **New recovery codes** makes ten new ones, which void the former ones.

**Turn off** and **New recovery codes** ask first for a code of the app, a recovery code, or the password.

### Signing in with a code

In the admin app, after a right password, the sign-in page asks for the code. No session exists until it is entered: the step between the two belongs to the browser that entered the password, and ends after 5 minutes, after which the password is asked again. A recovery code is entered in the same field. The activity log records each sign-in with a recovery code.

- The app's code changes every 30 seconds. Polyfin accepts the codes of the 30 seconds before and after too, for a phone whose clock is a little off.
- Each code is accepted once: entered again, even within its 30 seconds, it is refused.
- Wrong codes count as wrong passwords do toward the client's failed attempts (429 once too many), and also toward the account's, whatever the client, with the same limit. A right password between wrong codes does not reset them. They do not count toward **Block an account after this many wrong passwords (0 = never)**.
- Each code counts toward the limit as soon as it arrives, before it is checked, as passwords, PINs and tokens do: of codes sent at once, those past the limit are refused without being checked.
- The key is encrypted with `POLYFIN_SECRET_KEY`, like the other stored keys, and never shown again once set up (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).

Turning it on or off keeps the sessions already open, in the admin app and in Jellyfin apps: sign out the devices you no longer trust under **My account › Devices**.

### Jellyfin apps

Jellyfin apps, the web client included, cannot ask for a code. An account with two-step sign-in signs in to them with Quick Connect, approved on the admin app's **Quick Connect** page, which itself asked for the code (see [Signing in to apps with Quick Connect](jellyfin-compatibility.md#signing-in-to-apps-with-quick-connect)), or through an identity provider in the web client (see [Signing in with an identity provider](#signing-in-with-an-identity-provider)). The password alone is refused there as a wrong password is. API keys made by administrators work as before. The web client does not open an admin app session for such an account, unless it signed in through the identity provider: the admin app asks for the password and the code (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

### Silo's apps

Silo's apps cannot ask for a code either. Silo's TV apps sign in with a code of their own, which is a Quick Connect request too: an account with two-step sign-in approves it on the admin app's **Quick Connect** page, which `/activate` leads to, as for Jellyfin apps (see [Signing a TV in with a code](silo-apps.md#signing-a-tv-in-with-a-code)). Silo's apps for phones, tablets and Mac take a password only, which is refused for such an account as a wrong password is.

### For administrators

**Require two-step sign-in for administrators**, under **Settings › Security**, off by default, makes the second step a must for administrators:

- an administrator without it is shown the QR code right after their password, and gets a session once a code of the app confirms it, with their recovery codes;
- their password alone signs in to no Jellyfin app, set up or not: Quick Connect does;
- they cannot turn theirs off under **My account**;
- sessions already open stay, until they sign out.

A user who lost their phone and their recovery codes asks an administrator: **Turn off two-step sign-in**, on their page under **Users**, turns it off after a confirmation. The activity log records who turned it off for whom. They then sign in with their password alone, and set it up again from **My account**, or right after their password if it is required of them. An administrator turns their own off from **My account**.

A reset of a forgotten password by PIN from a Jellyfin app changes the password only: two-step sign-in stays on (see [Forgotten password](administration.md#forgotten-password)).

A database restored under another `POLYFIN_SECRET_KEY` holds keys that the current key cannot read. Their users still sign in with a recovery code, and **My account** says so; an administrator can turn theirs off, and **System › Health** names them (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).

**For app developers:**

- `POST /admin/api/session` answers `{"user": …}` without a second step, `{"twoStep": "code"}`, or `{"twoStep": "setup", "setup": {"secret", "uri", "qr"}}` for an administrator setting it up, with an HttpOnly cookie for the step. `POST /admin/api/session/two-step` with `{"code"}` then opens the session (`invalid_code`, or 401 `two_step_expired`), with `recoveryCodes` after a setup.
- `GET /admin/api/account/two-step` answers `on`, `recoveryCodes` (those left), `unreadable` and `required`. `POST …/two-step/setup` answers the key, its `otpauth://` address and its QR code as rows of `0` and `1`; `POST …/two-step/confirm` with `{"code"}` turns it on and answers the recovery codes. `POST …/two-step/recovery-codes` and `POST …/two-step/off` take `{"code"}` or `{"password"}`.
- `POST /admin/api/users/{id}/two-step/off`, for administrators, answers the user, whose `twoStep` tells whether it is on.
- The codes follow RFC 6238: HMAC-SHA-1, 30-second steps, 6 digits, a 160-bit key.
- A recovery code holds 80 random bits, in lowercase Crockford base32. Polyfin stores its SHA-256 hash: at 80 bits, a copy of the database does not let anyone search it out.

## Signing in with an identity provider

When an administrator sets up an OpenID Connect provider under **Settings › Security** (see [Identity provider](administration.md#identity-provider)), the sign-in pages of the admin app and of the web client get a **Sign in with …** button, named after the provider. It leads to the provider's own sign-in page, which asks for its password and its own second step, then back to Polyfin, signed in:

- in the admin app, the button sits under **Sign in**; the browser comes back to the admin app's home, with a session that lasts as one opened with a password does;
- in the web client, Polyfin's script adds the button first among the buttons of jellyfin-web's sign-in page. The browser comes back to the web client, signed in as a device of its own, as a password signs it in: the device shows under **My account › Devices**, and signing out signs it out.

The sign-in starts and ends at the server's **Public address**, set under **Settings › Notifications**, which must be set for the button to show: a web client opened at another address comes back at the public one.

### Which account signs in

The first time someone signs in through the provider, Polyfin looks for their account:

1. the account whose name is the claim set under **Claim naming the user**, `preferred_username` by default, without regard to case;
2. else the account whose name is their email, only when the ID token says the email is verified (`email_verified` true): an email the provider does not vouch for, or says nothing of, matches no account;
3. else, with **Create accounts** on, a new account, named after the claim, or the email when the claim is no valid name. It has the permissions of an account created under **Users** without a group, and **Identity provider only** set. With **Create accounts** off, the default, the sign-in is refused.

The account found is linked to the person at the provider, by the provider's issuer and their subject (`sub`), which never change there. Later sign-ins find the same account by that link, even when the claim or the email changed. An account linked to someone else at the provider, matched by name or email, is refused. Changing the issuer under **Settings › Security** starts the links over.

With **Administrators' claim** and **Administrators' value** set, someone whose claim holds the value, such as a `groups` claim listing `polyfin-admins`, becomes an administrator as they sign in, unless the account is disabled. The group only grants: removing someone from the group in the provider does not demote them in Polyfin. They stay an administrator until you turn **Administrator** off on their page under **Users**.

A disabled account, and one that ended, is refused as with a password, and so is a sign-in outside the account's allowed hours.

### Accounts without a password

**Identity provider only**, on a user's page under **Users** and when creating one, has the account sign in through the provider alone. Its password, even the right one, is refused as a wrong password is, in the admin app and in Jellyfin apps, and counts toward the client's failed attempts but not toward the account's, so that nobody can block it with passwords. An account created so needs no password: Polyfin gives it one nobody knows. Turning it off lets the account's password sign in again: set one first.

### Two-step sign-in and the provider

Polyfin does not ask for the code of its own two-step sign-in after the provider: the provider asked for its own second step, as Jellyfin's SSO plugins leave it to the provider. An administrator required to use two-step sign-in signs in through the provider without setting it up. The web client signed in through the provider also opens the admin app without a code (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)), which a web client signed in with Quick Connect does not.

In the same way, the block after wrong passwords does not stop a sign-in through the provider: it blocks passwords only (see [Blocking after wrong passwords](#blocking-after-wrong-passwords)). An account blocked by someone guessing its password still signs in through the provider, which checked its own credentials, and stays blocked for passwords.

### Jellyfin apps

Jellyfin apps cannot open the provider's page. They sign in with Quick Connect: the app shows a code, approved on the admin app's **Quick Connect** page, or in the web client's own **Quick Connect** page, opened through the provider (see [Signing in to apps with Quick Connect](jellyfin-compatibility.md#signing-in-to-apps-with-quick-connect)). An account with a password may also type it in the app, unless it signs in through the provider only.

### How the sign-in is kept safe

- Polyfin uses the authorization code flow with PKCE (S256), a state and a nonce, each made of 32 random bytes. The state is given to the browser in a cookie too, and taken once, within 10 minutes: a state replayed, made up, expired, or brought back by another browser is refused.
- The ID token must be signed with RS256 or ES256 by a key of the provider's JWKS, issued by the issuer set, exactly, for the client ID set, with the nonce of the sign-in; its expiry must not have passed and its issue time must not be to come, 2 minutes allowed either way for clocks that differ. A token naming a key the JWKS lacks has Polyfin fetch the keys again, at most once every 30 seconds.
- The discovery document must name the issuer set, exactly. Polyfin keeps it an hour, and the keys until a token needs another.
- The redirect URI is built from the public address, and must be registered at the provider exactly as **Settings › Security** shows it.
- An `http://` issuer is accepted, for a provider on the local network, but over HTTP the client secret, the codes and the tokens cross the network unencrypted: use HTTPS for the provider and for the public address (see [Identity provider](administration.md#identity-provider)).
- Each return from the provider counts toward the client's failed attempts as a sign-in with a password does: a refused state, code or token counts as a wrong password, and once too many are refused, the browser comes back with "Too many attempts". Someone whose account is refused counts as no wrong guess.
- No token, code, state or secret is written to the log or sent back. The browser gets the Jellyfin access token of its own device, as a sign-in with a password gives it, through a one-time ticket in a cookie its page cannot read, valid 2 minutes.
- The client secret is stored encrypted with `POLYFIN_SECRET_KEY` (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
- The activity log records the sign-ins, the refused ones with the name the provider gave, and the accounts it created.

**Compared with Jellyfin:**

- Jellyfin has no OpenID Connect sign-in of its own: plugins add it, with a sign-in page of their own. Polyfin adds the button to jellyfin-web's own sign-in page, through its script, rather than sending `/web/` to the provider: jellyfin-web keeps its sign-in in the browser, which the server cannot see, so a redirect would also send to the provider people already signed in, or wishing to use their password or Quick Connect.
- Polyfin finds accounts by the link to the provider, then by name, then by email, and makes administrators from a group; it never takes administrators back, so that a provider's change cannot leave the server without one.

**For app developers:**

- `GET /admin/api/status` answers `provider`, `{"name", "start"}`, or `null` without a provider or a public address. `start` is `<public address>/admin/api/provider/start`, to which the admin app adds `?app=admin` and the web client `?app=web`.
- `GET /admin/api/provider/callback` is the redirect URI. It sends the browser to `/admin/`, signed in, or `/admin/?provider=<error>`; for the web client, to `/web/?polyfinProvider=signed-in` or `/web/?polyfinProvider=<error>`. The errors are `provider_refused`, `provider_no_account`, `account_disabled`, `outside_allowed_hours`, `too_many_attempts`, `provider_state`, `provider_failed`, `provider_off`, `provider_unreachable` and `internal`.
- `POST /admin/api/provider/web`, with the ticket's cookie and the device in a `MediaBrowser` `Authorization` header (`Client`, `Device`, `DeviceId`, `Version`), signs that device in once and answers `{"accessToken", "userId", "serverId", "serverName"}`; 401 `invalid_credentials` without a valid ticket, 403 `account_disabled` or `outside_allowed_hours`.
- A user's `providerOnly` tells whether they sign in through the provider only; `POST /admin/api/users` and `PATCH /admin/api/users/{id}` take it.

## Security settings

**Settings › Security** gathers eight settings and the identity provider, and **Settings › Diagnostics** a ninth setting. They apply at once, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Allow users' own addons** | **Settings › Security** | On | Lets users add addons of their own under **My sources**. |
| **Users may import their watch history from another server or app** | **Settings › Security** | On | Shows the imports from a Jellyfin or Emby server and from an app under **My account** (see [Importing your own watch history](#importing-your-own-watch-history) and [Importing from an app](#importing-from-an-app)). |
| **Allow share links** | **Settings › Security** | Off | Lets the users with **Can share titles** give a movie or an episode to someone without an account, through a link that expires (see [Share links](#share-links)). Off, every link is refused, without being deleted. |
| **Import connected accounts every** | **Settings › Security** | 6 h (or 15 min, 30 min, 1 h, 2 h, 3 h, 12 h, 24 h) | How often the apps users stay connected to, and the watch histories they import from tracking services, are imported again (see [How often connected accounts are imported](#how-often-connected-accounts-are-imported)). |
| **Block an account after this many wrong passwords (0 = never)** | **Settings › Security** | 0, never (or 3 to 20) | Blocks an account for 15 minutes after that many wrong passwords in a row. |
| **Require two-step sign-in for administrators** | **Settings › Security** | Off | Administrators must sign in with a code from an authenticator app after their password (see [Two-step sign-in](#two-step-sign-in)). |
| **Sign out devices unused for (days, 0 = never)** | **Settings › Security** | 0, never (or 1 to 365 days) | Signs out Jellyfin apps not used for that many days. |
| **Known proxies** | **Settings › Security** | None (up to 64 addresses, networks or host names) | The reverse proxies whose `X-Forwarded-For` or `X-Real-IP` header names the client of their requests. |
| **Identity provider** | **Settings › Security** | None | An OpenID Connect provider people sign in with, in the admin app and the web client (see [Signing in with an identity provider](#signing-in-with-an-identity-provider) and [Identity provider](administration.md#identity-provider)). |
| **Detailed log** | **Settings › Diagnostics** | Off | Logs at the `debug` level until turned off. |

### Users' own addons

Under **My sources**, users choose which catalogs of their own addons are libraries, in which order, with their names, images, genres and maximums, and whether the web player's top bar shows them, as administrators do under **Content › Libraries** (see [The order of libraries](addons-and-libraries.md#the-order-of-libraries), [Library images](addons-and-libraries.md#library-images), [Genre and maximum](addons-and-libraries.md#genre-and-maximum) and [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). Their image addresses must be public: only administrators can use local network addresses.

Users also make their own smart lists under **My sources**, with **Add a smart list**, as administrators make the server's (see [Smart lists](addons-and-libraries.md#smart-lists)). A user's own list may draw from their own addons' catalogs and, while they use the server's sources, from the server's. Only they see it: as a library among their own, or in the Collections view. The rules on what was played, started or marked favorite follow what they did, in their own lists as in the server's.

On a user's page under **Users**, the **Access** section also holds **Can add their own addons**, on by default. While either this or **Allow users' own addons** is off:

- the user's own addons are kept but not used;
- their apps show the server's addons, libraries and smart lists only: their own smart lists are kept but not shown, and cannot be made or changed;
- **My sources** says why;
- their addons cannot be added, replaced, refreshed or turned on (403 `personal_addons_disabled`).

The addons come back once both are on again. See [Addons and libraries](addons-and-libraries.md).

### How often connected accounts are imported

**Import connected accounts every** sets how often Polyfin imports again the Nuvio and Stremio accounts users stay connected to (see [Staying connected](#staying-connected)) and the watch histories they import from tracking services (see [Importing your watch history](tracking.md#importing-your-watch-history)): from 15 minutes to 24 hours, 6 hours by default. The AniList and MyAnimeList lists follow the same interval: **Import my AniList lists** and **Import my MyAnimeList lists**, their **Plan to Watch** switches, and **Sync my ratings**, which imports their scores onto unrated titles with each import (see [AniList and MyAnimeList](tracking.md#anilist-and-myanimelist)). All are off by default. Someone who watches in one of those apps, then opens a Jellyfin app, finds their progress sooner with a short interval, or at once with **Import now**.

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

## DLNA devices

TVs and players that play over DLNA cannot sign in: each shows the libraries of the user an administrator chose for it under **Settings › DLNA**, or of **No one** until then (see [DLNA](dlna.md#devices-and-users)). **Show new devices the libraries of** chooses the user of the devices seen for the first time, **No one** by default.

A device browses and plays as its user: their visible libraries, blocked genres, parental control, hidden titles, allowed hours, **Playbacks at once**, **Maximum quality**, quality group and **Can use conversion** apply. Its playbacks show under the user in **Sessions**, and count in their statistics. Deleting the user leaves the device showing **No one**'s libraries; blocking it with **Blocked** gives it nothing at all.
