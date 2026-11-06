# User limits

This page is for administrators. It covers the limits you set on each user: ratings, genres, libraries, hours, remote access, playback and quality.

## Parental control

The **Parental control** section of a user's page, under **Users**, limits them to titles up to a **Maximum rating**. **Block unrated movies** and **Block unrated shows** hide unrated movies or shows.

- Administrators' Jellyfin apps can set the same limit from their user settings.
- A followed group sets it for its members; an app changing it detaches the member (see [User groups](users.md#user-groups)).

<a id="how-a-title-s-rating-is-found"></a><a id="how-a-titles-rating-is-found"></a>

### How the rating of a title is found

- A title's rating is the certification the server's metadata addon gives: usually the US one, else the local one.
- Polyfin also reads French, German and British ratings, and plain ages ("12", "16+").
- An unknown rating counts as none.
- Users' own addons never rate titles.
- Seasons and episodes follow their series.

A limited user's apps show only the server's addons and libraries. Their own addons are kept but unused, and they cannot turn the server's addons off.

### What a limited user sees

A title above the limit is hidden everywhere, as in Jellyfin:

- libraries, collections, genre, studio and year pages, search, latest, Continue Watching, Next Up, Upcoming, playlists, similar titles and person pages leave it out;
- people credited only in hidden titles are not listed;
- opening, playing, downloading it or searching its subtitles is refused.

Artwork stays reachable without signing in, as in Jellyfin.

### Ratings looked up for limited users

Addon catalogs rarely carry ratings. For a limited user, Polyfin asks the metadata addon for each listed title of unknown rating, a few at a time, once per title.

- The rating is kept for every user.
- It is asked again after a week, or a few hours if there was none.
- A listing waits a few seconds, then stops at titles still unknown; they appear in later listings.
- A listing reads at most a few times as far into a catalog as for an unlimited user.

## Blocked genres, visible libraries and allowed hours

These three sections of a user's page each have their own save button. Administrators' Jellyfin apps can also set them. A followed group sets all three; an app changing visible libraries or allowed hours detaches the member (see [User groups](users.md#user-groups)).

### Visible libraries

**Visible libraries** chooses which server libraries the user's apps show. The default is all, libraries added later included.

It blocks no title: a hidden library's titles stay reachable through other libraries, search or links. Listing the hidden library itself is refused (401), as Jellyfin refuses a folder the user may not access. Parental control and blocked genres block titles.

API details: [Visible libraries](jellyfin-api.md#visible-libraries-for-apps).

### Blocked genres

**Blocked genres** hides titles of these genres, in any letter case, wherever parental control hides titles: lists, search, details, playback, downloads, Continue Watching, Next Up, similar titles and person pages.

- Genres come from the server's addons' complete description, looked up and kept like ratings.
- A title is hidden until its genres are known.
- A user with blocked genres browses only the server's addons, as under parental control.
- The admin app offers the genres of the server's libraries (their genre page names), or any typed genre.

**Compared with Jellyfin**

- Jellyfin has no such setting. Its `BlockedTags` are tags, which Polyfin's titles lack, so the policy reports none and Polyfin ignores those apps send.

### Allowed hours

**Allowed hours** are Jellyfin's access schedules. Each row has:

- a day: one weekday, every day, weekdays or weekends;
- a start and end hour from 0 to 24, both included, in the server's time zone.

A user with rows cannot sign in outside all of them (403). Their apps' requests get an empty 403, as in Jellyfin, except reading their own user and the server's information.

- As in Jellyfin, administrators cannot sign in outside their hours, but their signed-in apps are served.
- Quick Connect sign-ins, a Silo TV's code included, are not checked; the app's later requests are.
- Polyfin refuses hours outside 0 to 24, or ending before they start.
- The admin app lets nobody sign in outside their hours; a signed-in member sees only who they are.

**Compared with Jellyfin**

- Jellyfin serves direct streams without checking who asks; Polyfin also refuses a user's media outside their hours.
- jellyfin-web never sends hours outside 0 to 24 or ending before they start.

### Connections from outside the local network

**Can connect from outside the local network**, in the **Access** section of a user's or group's page, is Jellyfin's "Allow remote connections to this server" (`EnableRemoteAccess`).

- It is on for every user and group until an administrator turns it off.
- jellyfin-web's user editor sets it through `POST /Users/{userId}/Policy`. A policy without it turns it on, as in Jellyfin.
- Groups, invites' model users and **Reset to defaults** carry it like other **Access** settings.

Turned off, the user, administrator or not, may use the server only from the local network. From outside:

- sign-in is refused (403): by password, from a Jellyfin app, Silo's apps or the admin app, through the identity provider, at two-step sign-in's second step, and through Quick Connect;
- each refusal is a failed sign-in in the activity log, about the user, with the client's address;
- app requests get an empty 403, including those served outside allowed hours;
- WebSockets, stream and download addresses, repackaged streams and Live TV relays are refused too (401 for media, as for another user's play session);
- an admin app session is refused (`outside_local_network`), but can still read and end itself;
- `/Users/Public` leaves the user off the sign-in screen.

The local network is judged as for a forgotten password's PIN: loopback, private and link-local addresses of the client that [Known proxies](reverse-proxy.md#known-proxies) reports. Behind a proxy missing from **Known proxies**, every client looks like the proxy, often local, so the setting cannot hold. When **System › Health** sees such a proxy, the section warns while the setting is off.

Still allowed:

- the user's [share links](invite-and-share-links.md#share-links) play anywhere: sharing is the user's choice;
- Quick Connect checks both requests: approval is refused where the approver may not connect, sign-in where the user may not;
- DLNA answers only the local network anyway;
- API keys are not users and are never refused.

An administrator connected from outside cannot turn off their own remote access, in the admin app or a Jellyfin app's user editor, as it would lock them out at once.

- The change is refused (409): `remote_access_self` in the admin app, a server-language message from `POST /Users/{userId}/Policy`.
- From the local network it goes through.
- A group never holds an administrator, so a group change cannot lock its author out.

**Compared with Jellyfin**

- Jellyfin refuses the sign-in ([`UserManager.cs`, lines 633 to 641](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Users/UserManager.cs#L633-L641)) and every request, administrators' too ([`DefaultAuthorizationHandler.cs`, lines 58 to 71](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Auth/DefaultAuthorizationPolicy/DefaultAuthorizationHandler.cs#L58-L71)), and leaves the user out of `/Users/Public` ([`UserController.cs`, lines 626 to 632](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/UserController.cs#L626-L632)), as Polyfin does.
- Jellyfin refuses an outside password sign-in before checking the password, and only logs it. Polyfin refuses once the password is right, so the refusal reveals nothing, and writes it to the activity log.
- Jellyfin lets a Quick Connect sign-in through and refuses its requests; Polyfin refuses the sign-in.
- Jellyfin serves direct streams without checking who asks; Polyfin refuses them from outside, as outside allowed hours.
- Jellyfin's local network is set in its networking options; Polyfin's is the private, link-local and loopback ranges, Jellyfin's default.
- Jellyfin does not stop an administrator removing their own remote access from outside; Polyfin refuses it on purpose.

API details: [Remote access](jellyfin-api.md#remote-access-for-apps).

## Playback and access limits

The **Playback and access** section of a user's page holds five more limits, saved with the page's **Save** (see [Users and permissions](users.md#accounts-and-permissions-overview)). Administrators' Jellyfin apps also set them. A followed group sets them, with the **Quality group**; an app changing one detaches the member (see [User groups](users.md#user-groups)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Playbacks at once (0 = no limit)** | The user's **Playback and access** section | 0, no limit (up to 20) | How many of the user's devices may play at once. |
| **Maximum quality** | The user's **Playback and access** section | No limit | Caps the bitrate of everything the user plays. |
| **Live TV** | The user's **Playback and access** section | On | Whether the user can see and play Live TV. |
| **Watch together** | The user's **Playback and access** section | Create and join | Whether the user may create and join SyncPlay groups, only join them, or not watch together at all. |
| **Can control other users' apps** | The user's **Playback and access** section | On for administrators, off for others | Lets the user see and control the apps of other users from theirs. |

The section also holds **Quality group** (see [Quality groups](#quality-groups)) and **Play local files first**, off by default. It offers local folder and network share versions before addon streams, whatever the server's version order (see [Local files first for some users](versions.md#local-files-first-for-some-users)). Jellyfin apps neither show nor set it.

API details: [Local files first per user](jellyfin-api.md#local-files-first-per-user-for-apps).

### Playbacks at once

Once that many of the user's other devices play, no further device can start. Devices count as in **Now playing** on **Home**, DLNA devices showing the user's libraries included.

- jellyfin-web says the media cannot be played at this time. Polyfin's web player script closes the generic error it shows over that (see [Web player](web-client.md#dashboard-links-and-single-sign-on)).
- A playing device can move on to another title.
- A device making a [converted download](transcoding.md#converted-downloads) counts as playing meanwhile.

Apps report playback every 10 seconds, paused too. After five minutes without a report, a device stops playing at its last reported position (see [Playbacks](statistics.md#playbacks)): a tab closed mid-play, a phone off the network, a crashed app.

- Its resume point stays there.
- It leaves this count and **Now playing**.
- Its conversions end; its torrent lingers then stops, as after a stop.

**Compared with Jellyfin**

- PlaybackInfo from the extra device answers Jellyfin's `RateLimitExceeded`.
- Jellyfin also stops a playback silent for five minutes, paused or not, at the last reported position. It checks every five minutes, so stops come five to ten minutes after the last report; Polyfin checks every minute, so five to six, as the playback holds a place here.
- Jellyfin's `MaxActiveSessions` counts signed-in sessions and refuses sign-in. Polyfin counts playbacks instead, on purpose, so users stay signed in everywhere.
- Polyfin refuses a policy setting above 20 (400).

### Maximum quality

Polyfin uses the lower of the app's limit and this one. A version above it is:

- converted down, when the user may use conversion (see [Transcoding](transcoding.md));
- else skipped for the next version under it;
- else nothing plays (`NoCompatibleStream`).

It is never played as is or with its video copied, even from a kept or made-up URL.

**Compared with Jellyfin**

- Jellyfin's `RemoteClientBitrateLimit` covers remote apps only. Polyfin applies it to all the user's playback, on purpose.

### Live TV

With **Live TV** off, the Live TV view leaves the user's apps, and channels and programmes can be neither found nor played. See [Live TV](live-tv.md).

API details: [Live TV access](jellyfin-api.md#live-tv-access-for-apps).

### Live TV channels

**Live TV channels** chooses which channels of the server's Live TV sources the user sees while **Live TV** is on. It is its own section of a user's or group's page, saved with the page's **Save**.

| Choice | Channels shown |
|---|---|
| **All** (the default) | Every channel. |
| **Only these** | Only the groups and channels ticked. |
| **All except these** | Every channel but the groups and channels ticked. |

With **All except these**, **Hide channels from groups added later** also hides groups a source adds after saving. They show ticked and marked **New**; saving again with them unticked shows them.

A group is a category of an IPTV line-up, or the first genre an addon's TV catalog gives a channel (else the catalog itself). Groups show their source and channel count. A filter finds groups, and a search finds channels by name or number, in line-ups of tens of thousands.

Rules name groups and channels by keys kept across refreshes (see [How the line-up is kept](iptv.md#how-the-line-up-is-kept)). A channel the administrator renumbers, renames or recategorizes stays ticked. An M3U entry the provider moves to another group or renames becomes a new channel.

A hidden channel is hidden wherever **Live TV** off hides Live TV: channel lists, the guide, what airs now, searches, the channel and its programmes opened by identifier, playback, Replay, timers, series timers, reminders and recordings shown.

- Its recordings are hidden even from the user who scheduled them.
- Timers on it record nothing, as for a user without **Live TV**.
- Other users still see it.
- It keeps its number, so numbers match for everyone.

The rule covers only the server's sources; a user's own sources, under **My sources**, are theirs. Groups, invites' model users and **Reset to defaults** carry it like other sections.

**Compared with Jellyfin**

- Jellyfin has no such Live TV setting. Its `EnableAllChannels` and `EnabledChannels` choose channel plugins, not Live TV channels ([Channel.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Channels/Channel.cs#L38-L39)). Polyfin always reports `EnableAllChannels` on, so jellyfin-web's user editor neither shows nor changes the rule, and saving a policy keeps it.

API details: [Live TV channels per user](jellyfin-api.md#live-tv-channels-per-user-for-apps).

### Watch together

**Watch together** chooses, like Jellyfin's `SyncPlayAccess`, whether the user may:

- create and join SyncPlay groups (the default);
- only join them;
- not watch together at all; jellyfin-web then hides its button.

Creating a group needs the first choice; listing and joining, either of the first two. Other SyncPlay requests answer 403 unless the user is in a group.

### Playing torrent streams

**Can play torrent streams**, in **Access** next to **Can download**, offers the user torrents that addons list, while **Play torrent streams** is on under **Settings › Torrents** (see [Torrents](torrents.md)).

- It is off for every user, administrators included, and every group, until given.
- Without it, the user gets no torrent versions, from server or own addons, and a torrent version listed to another user is not found for them.
- Torrent versions never download, whatever **Can download** says.

### Can control other users' apps

**Can control other users' apps** is Jellyfin's `EnableRemoteControlOfOtherUsers`.

- Administrators have it when created, and when upgrading from an older Polyfin.
- Other users do not.
- A user made administrator later keeps their value.

## Quality groups

**Quality group**, next to the maximum quality in the **Playback and access** section of a user's page, sets the highest resolution offered. A followed group sets it (see [User groups](users.md#user-groups)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Quality group** | The user's **Playback and access** section | **Original (no limit)** | Highest resolution offered: **Original (no limit)**, **4K**, **1440p**, **1080p**, **720p** or **480p**. |

### How the height of a version is found

A version's height is, in order:

1. the analyzed height, once Polyfin has analyzed it;
2. the height RemuxDB found in its file, when [Describe versions from RemuxDB](versions.md#tracks-from-remuxdb) is on and knows the file;
3. the height its labels name, in the addon's stream name, title, description or file name.

The labels read are:

- 2160p, 4K or UHD;
- 1440p, 2K or QHD;
- 1080p or FHD;
- 720p or HD;
- 576p, 480p or SD.

Disagreeing labels, or none, leave the height unknown.

### Which versions a user is offered

While at least one version fits the group or has an unknown height:

- taller versions are left out of what the user is offered and of the versions tried for playback;
- they are not analyzed;
- the others keep their order: the addons', or the [version order](versions.md#version-order) settings'.

When every version is taller, all are kept, closest to the group first (720p, then 1080p, then 4K), so the least is read and decoded. Same-height versions keep their order, or the version order settings'.

The group applies after the version order settings hide versions and before they order them (see [Version order](versions.md#version-order)).

- The played version is converted down to the group, or to **Maximum quality of converted video** if lower. Like every conversion, it is at most 1080p on the processor (4K on a GPU). See [Transcoding](transcoding.md).
- A fitting version plays as is, or repackaged, as before.
- A taller version is never sent at full size.
- A user who may not have video converted (**Can use conversion** off, or the server's **Conversion** switch off) is refused taller versions, like other conversions (`NoCompatibleStream`).

### Live TV, recordings and downloads

- Live TV channels and recordings follow the same rule once analyzed, as their height is only known then.
- A version's file downloads only if it fits. If none fits, the file is refused (403), as without the permission.
- With **Converted downloads** on, under **Settings › Conversion (transcoding)**, apps asking for a smaller copy, such as Streamyfin, download a taller version converted down to the group, if the user may have video converted. See [Converted downloads](transcoding.md#converted-downloads).

**Compared with Jellyfin**

- Jellyfin has no such setting. Apps neither show nor change it, and policies they post keep it.

API details: [Quality groups](jellyfin-api.md#quality-groups-for-apps).
