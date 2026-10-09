# Users and permissions

This page covers user accounts: what Polyfin keeps for each user, and the limits an administrator can set on them. It also covers the server-wide security settings.

## Accounts and permissions overview

Polyfin has separate accounts for each user, with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points and Next Up are tracked per user, and each user can be limited to titles up to a rating.

An administrator creates users with **Create a user** at the top of **Users**, and sets each user's limits on that user's own page, opened from the list. The page has these sections: **Name and password**, **Access**, **Playback and access**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours** and **Devices**. **Access** switches save as soon as they change; each other section has its own save button, such as **Save playback and access**. Signing out a device asks first. Administrators' Jellyfin apps can set most of the same limits from the user's settings. Each section below says where a setting lives.

Each user can also connect their own tracking accounts; see [Tracking services](tracking.md). Moving from a Jellyfin server, an administrator can bring its accounts and what each user watched over with **Import from Jellyfin**; see [Moving from Jellyfin](#moving-from-jellyfin).

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
- **API key**: create one in Jellyfin's dashboard, under API Keys. Polyfin uses it for this import only, and never saves it: each import asks for it again.

**Connect** lists the server's users, with their administrator status and whether they are disabled. A wrong key, an address where nothing answers, and an address where something other than Jellyfin answers each say so.

### Choosing who to import

For each Jellyfin user, **Import into** chooses where their data goes:

- **New user**: a Polyfin account is created, named as in Jellyfin, an administrator if they were one, hidden from the sign-in screen if they were. Jellyfin does not give out passwords: set one for each new account, under the same rules as **Create a user**. The name can be changed too.
- An existing Polyfin user: the one with the same name, whatever its case, is chosen at first. Several Jellyfin users can go into one account.
- **Do not import**: the user is left out. Disabled Jellyfin users start so.

**Import watch data** tells, user by user, whether their watch data comes along; a new account can be created without it. **Start import** creates the new accounts, all of them or none: when one name is taken or one password is too short, nothing is created, and the row says why. Then the watch data is imported in the background.

### What is imported

| From Jellyfin | Into Polyfin |
|---|---|
| User names, administrator status, hidden from the sign-in screen | New accounts, with the passwords the administrator sets. |
| Played movies and episodes, with the date last played and the play count | Played marks. |
| Resume points of movies and episodes | Resume points, for titles not played. |
| Favorite movies, series and episodes | Favorites. |

Nothing else comes over: passwords, ratings, playlists, collections, users' limits and settings, profile pictures, and music, books and Live TV.

### How titles are matched

Titles are found as an [imported watch history](tracking.md#importing-your-watch-history) finds them: by the IMDb identifier Jellyfin gives, then by TMDB, then by TVDB for series, among the titles Polyfin's catalogs listed. Episodes are found by their series' identifiers and their season and episode numbers; a file holding several episodes counts for each of them.

A title that cannot be matched is left out, and the import goes on. Each user's result lists these titles with why: **No IMDb, TMDB or TVDB identifier** (a home video, or an episode Jellyfin did not number) or **Not in Polyfin** (no title of Polyfin's catalogs has its identifiers).

### How the data merges with Polyfin's

The data merges as an imported watch history does (see [How the history merges with Polyfin's](tracking.md#how-the-history-merges-with-polyfin-s)):

- Played marks, resume points and favorites are only added, never removed.
- A title keeps Polyfin's date when it is later than Jellyfin's, and the larger of the two play counts.
- Of two resume points, the more recent one stays.
- Importing again adds nothing twice and never overwrites newer Polyfin data, so an import can safely run again.

Nothing is ever written to the Jellyfin server: the import only reads it.

### Progress and limits

- One import runs at a time. Leaving the page does not stop it, and the page shows how far it is, user by user, then what it added: titles marked played, resume points and favorites, and the titles not found.
- **Stop import** stops it at once. What was imported stays: a user whose data was being saved is saved whole, and one whose data was being read gets nothing until the next import.
- Jellyfin is read 200 titles at a time, one request after the other. A request that fails is tried 3 times more; a server that keeps failing, or refuses the key, ends the import, after saving what was read of the current user.
- Polyfin keeps the running import and the last result in memory only: a restart stops an import under way and forgets the result. Start it again: it adds only what is missing.

**Compared with Jellyfin:**

- Jellyfin keeps watch data for the files of its libraries; Polyfin keeps it for the titles of its catalogs. Titles are therefore matched by their identifiers, not by their files.

**For app developers:**

- The admin API serves the import to administrators: `POST /admin/api/jellyfin-import/users` lists a server's users, `POST /admin/api/jellyfin-import` creates the accounts and starts the import, `GET /admin/api/jellyfin-import` reads it, and `POST /admin/api/jellyfin-import/stop` stops it. The API key is never answered back.
- Polyfin reads Jellyfin with `GET` requests only, sending the key as `Authorization: MediaBrowser Token="…"`: `/System/Info/Public`, `/Users`, and `/Users/{id}/Items` with `Recursive`, `IncludeItemTypes`, `Filters` (`IsPlayed`, `IsResumable`, `IsFavorite`) and `Fields=ProviderIds`, then the episodes' series by `Ids`.

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
