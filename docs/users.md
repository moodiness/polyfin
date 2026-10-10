# Users and permissions

This page covers user accounts: what Polyfin keeps for each user, and the limits an administrator can set on them. It also covers the server-wide security settings.

## Accounts and permissions overview

Polyfin has separate accounts for each user, with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points and Next Up are tracked per user, and each user can be limited to titles up to a rating.

An administrator creates users with **Create a user** at the top of **Users**, and sets each user's limits on that user's own page, opened from the list. The page has these sections: **Name and password**, **Access**, **Playback and access**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours** and **Devices**. **Access** switches save as soon as they change; each other section has its own save button, such as **Save playback and access**. Signing out a device asks first. Administrators' Jellyfin apps can set most of the same limits from the user's settings. Each section below says where a setting lives.

Each user can also connect their own tracking accounts; see [Tracking services](tracking.md). Moving from a Jellyfin server, an administrator can bring its accounts and what each user watched over with **Import from Jellyfin**; see [Moving from Jellyfin](#moving-from-jellyfin). Rather than choosing a password for each person, an administrator can also send them an invite link, with which they create their own account; see [Invite links](#invite-links).

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

**Import from Jellyfin**, at the top of **Users** next to **Create a user**, brings a Jellyfin server's accounts over, and what each of their users watched. It sits under **Users** because it creates accounts and fills each account's watch state. Only administrators see it.

### Connecting to the server

- **Server address**: the address Jellyfin opens at in a browser, such as `http://192.168.1.10:8096`, with the path it is served under if any. A host and port alone mean `http://`. Local network addresses work.
- **Connect with** chooses how Polyfin reads the server:
  - **An API key**: create one in Jellyfin's dashboard, under API Keys. It reads every user's watch data at once.
  - **A user account**: a **User name** and its **Password** on the server, empty if the account has none. Polyfin signs in as Jellyfin's apps do, which every server that speaks Jellyfin's API allows, even one without API keys, and signs out once done. An administrator's account lists every user.

Polyfin uses the key and passwords for this import only, and never saves them: each import asks for them again.

**Connect** lists the server's users, with their administrator status and whether they are disabled. A wrong key, name or password, a key or account that may not list the server's users, an address where nothing answers, and an address where something other than Jellyfin answers each say so.

### Reading each user's watch data

- A key from the server's dashboard belongs to no user: it reads every user's watch data.
- A user's own key, the access token of a user's session, or a user account belongs to that user, and reads only their watch data. Polyfin asks the server whose key it is, and names its owner on the page.

Jellyfin refuses a user's key the other users' data. Some servers that speak Jellyfin's API answer it with that user's data whatever user is asked, which would put one user's history into another's account. So, connected as one user, Polyfin reads each other user's watch data signed in as them: with **Import watch data** ticked, their row asks for their **Password on …** the server, left empty if their account has none. Polyfin signs in as them when the import starts, reads their watch data, then signs out. Without their password, untick **Import watch data**: their account can still be created.

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

Titles are found as an [imported watch history](tracking.md#importing-your-watch-history) finds them: by the IMDb identifier Jellyfin gives, the way the usual metadata addons name titles, or by TMDB, or by TVDB for series, when a catalog listed the title under one of those. Episodes are found by their series' identifiers and their season and episode numbers; a file holding several episodes counts for each of them. A title no catalog listed yet shows in the user's lists at once, described by the addons.

A title that cannot be matched is left out, and the import goes on. Each user's result lists these titles with why: **No IMDb, TMDB or TVDB identifier** (a home video, or an episode Jellyfin did not number) or **Not in Polyfin** (it has no IMDb identifier, and no catalog listed it under its TMDB or TVDB one).

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
  - `POST /admin/api/jellyfin-import/users` lists a server's users, given `apiKey`, or `account` with `name` and `password`. It answers `keyOwner`, the Jellyfin user the key belongs to or who signed in, `null` for a server's key. An account the server refuses is `jellyfin_sign_in_refused`, and one it does not let sign in `jellyfin_sign_in_forbidden`.
  - `POST /admin/api/jellyfin-import` takes the same, creates the accounts and starts the import. An entry's `jellyfinPassword`, given with `watchData`, has the import sign in as that user. Connected as a user, another user's watch data needs it: without it, the answer is `jellyfin_key_owner_only`. A password the server refuses is `jellyfin_password_refused`, an account it does not let sign in `jellyfin_sign_in_forbidden`, and a sign-in that opens another user's session `jellyfin_other_user`. Each names the `jellyfinId`, and comes before anyone is created.
  - `GET /admin/api/jellyfin-import` reads the import, and `POST /admin/api/jellyfin-import/stop` stops it.
  - The key and passwords are never answered back.
- Polyfin reads Jellyfin with `GET` requests, sending the key or session token as `Authorization: MediaBrowser Token="…"`: `/System/Info/Public`, `/Users`, `/Users/Me` (which Jellyfin answers a server's key with an error, and a user's key with that user), and `/Users/{id}/Items` with `Recursive`, `IncludeItemTypes`, `Filters` (`IsPlayed`, `IsResumable`, `IsFavorite`) and `Fields=ProviderIds`, then the episodes' series by `Ids`.
- It signs in with `POST /Users/AuthenticateByName`, naming itself and a device of its own in the `Authorization` header as Jellyfin's apps do, and signs out with `POST /Sessions/Logout`.

## Invite links

An invite link lets the people an administrator sends it to create their own account, so that nobody has to choose a password for them and send it in a message. **Create an invite link**, at the top of **Users**, asks for:

- **Accounts it may create**: from 1 to 100, 1 by default. One link can serve a whole family.
- **Expires**: after 1, 7 (the default), 30 or 90 days, or **Never**.
- **Settings of**: the user whose settings the new accounts copy, or a new user, as with **Create a user**.

With a model, a new account copies everything the model's page sets except the name, the password and **Administrator**: the **Access** permissions, the **Playback and access** limits with the **Quality group**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours**, and whether it shows on the sign-in screen. The model is read when each account is created, so a change to the model reaches the accounts created after it. Without a model, a new account has the settings of a user made with **Create a user**, hidden from the sign-in screen. An invite link never makes an administrator.

The link is shown once, with **Copy link**: Polyfin keeps only a hash of it and cannot show it again. It starts with the **Public address** (**Settings › Notifications**) when one is set, and with the address the admin app is open at otherwise.

**Invite links**, under the accounts, lists each link with the accounts it created out of those it may create, whose settings it gives, when it expires, who created it, and its state: **Active**, **Used up**, **Expired** or **Revoked**. **Revoke** stops an active link at once; the accounts it created are kept. Deleting the user a link copies revokes the link too.

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
  - `GET /admin/api/invites` lists them, the newest first: `id`, `maxUses`, `uses`, `expiresAt` (null for never), `createdAt`, `revokedAt`, `model` and `createdBy` (each `id` and `name`, or null; `createdBy` is null once that user is deleted), and `state`: `active`, `used_up`, `expired` or `revoked`.
  - `POST /admin/api/invites` creates one from `maxUses` (1 by default, up to 100), `expiresInDays` (7 by default, up to 365, `null` for never) and `modelUserId` (null or left out for a new user's settings). It answers 201 with the link as listed, plus its `token` and its `url`, which is null without a public address: the link is then `/admin/invite/<token>` on the server's address. The token is never answered again. Errors: `invalid_invite_uses`, `invalid_invite_expiry` and `invalid_invite_model`.
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

**Settings › Security** gathers three settings, and **Settings › Diagnostics** a fourth. They apply at once, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Allow users' own addons** | **Settings › Security** | On | Lets users add addons of their own under **My sources**. |
| **Block an account after this many wrong passwords (0 = never)** | **Settings › Security** | 0, never (or 3 to 20) | Blocks an account for 15 minutes after that many wrong passwords in a row. |
| **Sign out devices unused for (days, 0 = never)** | **Settings › Security** | 0, never (or 1 to 365 days) | Signs out Jellyfin apps not used for that many days. |
| **Detailed log** | **Settings › Diagnostics** | Off | Logs at the `debug` level until turned off. |

### Users' own addons

Under **My sources**, users choose which catalogs of their own addons are libraries, with their names, images, genres and maximums, and whether the web player's top bar shows them, as administrators do under **Content › Libraries** (see [Library images](addons-and-libraries.md#library-images), [Genre and maximum](addons-and-libraries.md#genre-and-maximum) and [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). Their image addresses must be public: only administrators can use local network addresses.

On a user's page under **Users**, the **Access** section also holds **Can add their own addons**, on by default. While either this or **Allow users' own addons** is off:

- the user's own addons are kept but not used;
- their apps show the server's addons and libraries only;
- **My sources** says why;
- their addons cannot be added, replaced, refreshed or turned on (403 `personal_addons_disabled`).

The addons come back once both are on again. See [Addons and libraries](addons-and-libraries.md).

### Blocking after wrong passwords

After that many wrong passwords in a row for one account, from a Jellyfin app or the admin interface, the account refuses every sign-in for 15 minutes. That includes the right password, which gets the answer of a wrong password.

The count starts again after any of these:

- a sign-in;
- a new password set by an administrator;
- **Unblock** on the user's page under **Users**, where the account shows until when it is blocked;
- setting the limit to 0.

**Compared with Jellyfin:**

- Jellyfin disables the account until an administrator enables it again. Polyfin only blocks it for a while, so that the last administrator can never be locked out for good.

**For app developers:**

- Jellyfin apps read the limit (-1 when there is none) and the count in the user's policy, as `LoginAttemptsBeforeLockout` and `InvalidLoginAttemptCount`.
- The limit is the server's, so a policy an app posts does not change it, nor the count.

### Signing out unused devices

Polyfin signs out the Jellyfin apps not used for that many days, at startup and every hour. It closes their connections and takes them out of SyncPlay groups, as any sign-out does. Admin interface sessions are not concerned.

### Detailed log

With **Detailed log** on, Polyfin logs at the `debug` level until it is turned off again, and at the `info` level otherwise. See [Configuration](configuration.md).
