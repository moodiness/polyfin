# Notifications

This page is for administrators and users who want Polyfin to tell them when something happens. It covers the targets messages go to, the events they tell, and how they are delivered. Integrators who read the messages with a program will find their format in the [Webhook reference](webhook-reference.md).

Polyfin can tell users and administrators of:

- a new episode of a series they follow;
- a recording that finished or failed;
- a problem **System › Health** found, or that was solved, and a crash of the server;
- a user who joined through an invite link, and a new version of Polyfin;
- a playback that started, paused, resumed or stopped, and a play that failed;
- an app connection or a tracking service that stopped;
- an account blocked after wrong passwords, and a sign-in on a new device;
- a guest who played a title through a share link;
- a Live TV programme a user asked to be reminded of, which starts soon;
- a title of a user's Watchlist playlist that versions were found for;
- what happened each week, month and year (see [Weekly summary](#weekly-summary), [Month in review](#month-in-review) and [Year in review](#year-in-review)).

[Events](#events) lists each one, with who receives it.

## Targets

A target is where messages go. Polyfin has seven kinds:

- a generic webhook;
- a Discord channel's webhook;
- an ntfy topic;
- an email address;
- a Telegram chat;
- a Gotify server;
- a Pushover user.

Targets have two kinds of owners:

| Where | Whose targets | What they receive |
|---|---|---|
| **Settings › Notifications** | The server's, for administrators | The events of every user, the health events, server crashes, failed plays, the users who joined, new versions, the blocked accounts, and the server's weekly summary, month and year in review. |
| **My account › Notifications** | Each user's own | That user's new episodes, recordings and playbacks, their stopped connections, their share links played, the blocks of their account and its sign-ins on new devices, their programme reminders, the titles of their Watchlist playlist found available, and their own weekly summary, month and year in review. |

- Sign-ins on new devices, programme reminders and Watchlist titles go to the user's own targets only.
- An administrator's own targets may also receive the health events, server crashes, failed plays, the users who joined, new versions and every blocked account.
- An administrator's own targets receive the server's summaries in place of their own.

### Adding a target

**Add a target** asks for its kind, a **Name** and the **Events** it receives. Each kind then asks for more:

| Kind | What it asks for |
|---|---|
| **Webhook** | Its address. Polyfin posts each event to it as JSON (see [The webhook event](webhook-reference.md#the-webhook-event)). |
| **Discord** | The channel webhook's address. In Discord, open the channel's settings, then Integrations, Webhooks, Copy Webhook URL. |
| **ntfy** | The **ntfy server** (`https://ntfy.sh` when left empty), the **Topic**, and an **Access token** for a topic that needs signing in. |
| **Email** | The **Email address** messages go to. They go through the server's SMTP server (see [Email](#email)). |
| **Telegram** | The **Chat**, and the **Bot token** BotFather gave when the bot was created. The chat is a number such as `-1001234567890`, or a public channel's `@name`. |
| **Gotify** | The **Gotify server**'s address and an **Application token**. In Gotify, open Apps, then Create Application. |
| **Pushover** | The **User key** (or a group key), and the **Application token** of an application created on Pushover. |

- A Discord webhook's address pasted in a new **Webhook** target makes it a **Discord** target, and the form says so.
- Such an address is `https://discord.com/api/webhooks/…`, or the same on `discordapp.com` and their `ptb.` and `canary.` hosts.
- A **Webhook** target saved before with such an address is sent Discord's message, since Discord refuses any other.
- Without an SMTP server, the email kind cannot be added, and the form says why.
- The Telegram bot must be a member of the chat.
- A user who is not an administrator may only add targets on public addresses, as with their own addons. An address on the local network is refused, and messages never reach one.

### Secrets

Polyfin stores a target's secrets encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)). The secrets are:

- webhook and Discord addresses;
- ntfy access tokens;
- Telegram bot tokens;
- Gotify application tokens;
- Pushover user keys and application tokens.

- A secret is never shown again: the list shows a webhook's or Discord address's host only.
- A secret is never written to the log.
- A secret that cannot be decrypted shows the target as **Enter it again**, and on **System › Health** under **Stored keys**.
- An email target's address, a Telegram chat and a Gotify server are not secrets, and are shown as they are.

### Target states

Each target shows its state:

| State | Meaning |
|---|---|
| **Working** | The last message was delivered. |
| **Nothing sent yet** | No message was sent to it yet. |
| **Refused** | It answered 401, 403, 404 or 410: its address or token is wrong, or it was deleted. Telegram refusing an unknown chat, Pushover an unknown user key or application token, and an SMTP server refusing the user and password, the sender or the recipient, count too. |
| **Message refused** | It refused the last message with another 4xx status. |
| **Not reached** | Recent messages could not be delivered: no answer, server errors, or for email no SMTP server. |
| **Off** | **Send messages to this target** is off. |

- The list shows the HTTP status the target answered with its problem, or for email the SMTP server's code, such as `SMTP 535`.
- A delivered message, a test included, clears the target's problem.

### Sending a test

**Send a test** sends a test message at once and tells whether the target accepted it.

- When the target refused it, the result shows the start of its answer, up to 200 characters, such as Discord's `{"message":"Cannot send an empty message","code":50006}`.
- That answer keeps nothing of the target's address, its path, its tokens or its topic, even in part: what it repeats of them shows as ….
- The log tells the result too.

### Email

Email targets go through one SMTP server, which an administrator sets in **Settings › Notifications › Email**. Email targets can be added once an **SMTP server** and a **Sender address** are saved.

Polyfin connects to the server with the **SMTP encryption** chosen:

| Choice | What it does |
|---|---|
| **STARTTLS** | Connects without encryption, then asks the server to encrypt. When the server does not offer to, nothing is sent. |
| **TLS from the start** | Encrypts the connection from the start. |
| **None** | Sends everything as it is. The password is then only sent to a server on this machine (`localhost`). |

- The server's certificate must name its host.
- Polyfin signs in with the **SMTP user** and **SMTP password** once the connection is encrypted, by `PLAIN`, or by `LOGIN` when the server offers only that.
- With no user, it sends without signing in.
- The password is stored encrypted with `POLYFIN_SECRET_KEY` and never shown again.

### Settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Public address** | **Settings › Notifications** | empty | The address people open Polyfin at, such as `https://media.example.org`. Links in messages start with it: an episode or a recording opens in the [web player](web-client.md), a health problem on **System › Health**. Empty, messages carry no link. |
| **SMTP server** | **Settings › Notifications** | empty | The host name of the SMTP server email targets go through, such as `smtp.example.org`. Empty, email targets cannot be added. |
| **SMTP port** | **Settings › Notifications** | 587 | Usually 587 with STARTTLS, 465 with TLS from the start. |
| **SMTP encryption** | **Settings › Notifications** | STARTTLS | STARTTLS, TLS from the start, or None (see [Email](#email)). |
| **SMTP user** | **Settings › Notifications** | empty | The user Polyfin signs in with. Empty, it sends without signing in. |
| **SMTP password** | **Settings › Notifications** | empty | Its password, stored encrypted. |
| **Sender address** | **Settings › Notifications** | empty | The address messages come from, such as `polyfin@example.org`. Email targets need it. |
| **Sender name** | **Settings › Notifications** | empty | The name shown with the sender address. Empty, the server name is. |
| **Send the weekly summary on** | **Settings › Notifications** | Monday | The day of the week the [weekly summary](#weekly-summary) is sent. |
| **Send the weekly summary at** | **Settings › Notifications** | 9:00 | Its hour, in the server's time zone. The [month in review](#month-in-review) and the [year in review](#year-in-review) are sent at this hour too. |
| **Remind me before** | **My account › Notifications** | 5 minutes | How long before its programme each of the user's [programme reminders](#programme-reminders) comes: 1, 5, 10, 15, 30 or 60 minutes. |
| **Also show reminders on my apps** | **My account › Notifications** | off | Shows each of the user's programme reminders on their Jellyfin apps that accept messages too (see [Programme reminders](#programme-reminders)). |

## Events

Each target chooses the events it receives in its **Events**. The `type` is the event's name in the [webhook event](webhook-reference.md#the-webhook-event).

| Event | `type` | Who receives it |
|---|---|---|
| New episode | `new_episode` | The targets of each user following the series; the server's targets once per episode, whoever follows it. |
| Recording finished | `recording_finished` | The targets of the user who scheduled it, and the server's. |
| Recording failed | `recording_failed` | The same. |
| Health problem found | `health_problem` | The server's targets, and administrators' own. |
| Health problem solved | `health_solved` | The same. |
| Server crashed | `server_crashed` | The server's targets, and administrators' own. |
| User joined | `user_joined` | The server's targets, and administrators' own. |
| New version | `new_version` | The server's targets, and administrators' own. |
| Playback started | `playback_started` | The targets of the user who plays, and the server's. |
| Playback paused | `playback_paused` | The same. |
| Playback resumed | `playback_resumed` | The same. |
| Playback stopped | `playback_stopped` | The same. |
| Playback failed | `playback_failed` | The server's targets, and administrators' own: at most once an hour per user and title. |
| Weekly summary | `weekly_summary` | The server's targets and administrators' own: the server's week. Every other user's targets: their own week. |
| Month in review | `monthly_summary` | The server's targets and administrators' own: the server's month. Every other user's targets: their own month. |
| Year in review | `yearly_summary` | The server's targets and administrators' own: the server's year. Every other user's targets: their own year. |
| Connection stopped | `connection_stopped` | The targets of the user whose connection stopped, and the server's. |
| Account blocked | `account_blocked` | The server's targets and administrators' own, and the targets of the user whose account it is. |
| Sign-in on a new device | `new_device` | The targets of the user who signed in only: the server's targets cannot choose it. |
| Shared link played | `share_played` | The targets of the user who made the share link, and the server's: at most once an hour per link. |
| Programme starting | `programme_reminder` | The targets of the user who set the reminder only: the server's targets cannot choose it. |
| Watchlist title available | `title_available` | The targets of the user whose Watchlist playlist holds the title only, never those of a user it is shared with: the server's targets cannot choose it. |

Some events are urgent: ntfy, Gotify and Pushover show them with a higher priority (see [Messages for each kind](webhook-reference.md#messages-for-each-kind)). The sections below say which.

### Shared links played

**Shared link played** tells that a guest without an account started a title through a share link (see [Share links](invite-and-share-links.md#share-links)).

- The message reads "Shared link played: Example Movie (2008)" and "A guest played Example Movie (2008) through the link shared by alex.".
- It opens **My account › Shared links**.
- The guest's playback also counts as the sharer's for the playback events, with "Shared link" as its app.
- It is not chosen by default for a new target.

### New episodes

A user follows two kinds of series, as the Upcoming row of Jellyfin apps shows them:

- the series they played an episode of (the 50 most recently played);
- the series they marked favorite.

Every two hours, starting five minutes after Polyfin starts, Polyfin looks at these series for episodes that became available.

- Only users with a target for new episodes, the weekly summary or the month in review are looked at.
- All users are looked at when the server has such a target, or an administrator has one for the weekly summary or the month in review.

An episode is told once per user. Polyfin never tells:

- an episode the series already had when Polyfin first looked at it, so neither starting to use notifications nor following a series sends old episodes;
- an episode released more than a week ago, which its addon added late;
- an episode without a release date.

The series' descriptions are those the library keeps. An old one is asked for again in the background, so a new episode may be told one check later. Each new episode found is kept for the weekly summary and the month in review, whether or not a target was told of it.

### Recordings

A recording that ends is finished, or partial when part of its programme is missing (see [Live TV](live-tv.md#recordings)). One that recorded nothing failed. A failed recording is urgent.

### Health problems

Every five minutes, Polyfin looks at the problems **System › Health** shows under **To look at**. They include the database, disk space, addons, IPTV sources and tuners, programme guides, failed tasks, backups, stored keys, HTTPS and local network discovery.

- A problem is told once two checks in a row found it, so that a brief failure sends nothing.
- It is told solved once two checks in a row no longer find it.
- What was told is kept in the database: a restart neither tells a problem again nor forgets to tell when it is solved.
- Each problem told is kept for the weekly summary and the month in review too.
- A problem that is an error is urgent.

Some problems are never told:

- conversions at their limit and thumbnails paused for a host, which come and go with the load;
- addons left out of searches for a few minutes, which come and go with the searches;
- the stopped connections **System › Health** lists, which their own event told (see [Stopped connections](#stopped-connections)).

### Users who joined

A user who created their account through an invite link (see [Invite links](invite-and-share-links.md#invite-links)) is told once, naming who created the link.

- The title is "New user: sam", which is an email's subject.
- The message is "sam joined through alex's invite.".
- It opens the user's page in the admin app.
- It is not urgent: ntfy, Gotify and Pushover show it with their normal priority.
- Each user who joined is kept for the weekly summary, the month and the year in review too.

### New versions

Polyfin asks GitHub once a day for its latest release while **Check for new versions** is on, in **Settings › General**. It also asks for the latest pre-release while **Include pre-releases** is on (see [New versions](updates-and-restarts.md#new-versions)).

A version newer than the running one is told once:

- The title is "New version: Polyfin 1.5.0".
- The message is "Polyfin 1.5.0 is available. This server runs version 1.4.0.".
- It opens the release notes on GitHub, with or without a public address.
- The version told is kept in the database, so a restart does not tell it again.
- A development build never tells one.
- Targets saved before this event existed do not receive it until it is chosen.

### Playbacks

Polyfin tells when a title starts playing in a Jellyfin app, pauses, resumes or stops. The title can be a movie, an episode, a channel, a recording, a Replay programme, a song or an audiobook.

- Each change is told once, whatever the number of reports the app sends.
- Positions as it plays are never told.
- A playback whose app stops reporting is told stopped five minutes after its last report (see [Statistics](statistics.md#playbacks)).

Each message names the user, the title, the app and the device, the position, and how it plays. The title is a movie, an episode with its series, a channel, or a song with its artist. How it plays is **Direct play**, **Remux** (repackaged) or **Conversion**. For example: "sam is watching Example Series S01E02" and "Pilot · Jellyfin Web on Chrome · 12:34 · Direct play".

A new target in the admin app leaves these events unchosen, since each user plays many times a day. Targets saved before these events existed do not receive them until they are chosen.

### Failed plays

A play that fails is told to the server's targets and administrators' own, so that they hear of it before the user complains. A play fails when:

- none of the title's versions plays on the app (Jellyfin's `NoCompatibleStream`, or a torrent version that could not start);
- a limit refuses it: the user's **Playbacks at once**, or a provider whose connections are all in use;
- its stream breaks while it plays: a segment that cannot be served because its source or its encoding failed, or a direct stream whose source failed.

The message names the user, the title, the reason, the app and the device. For example: "sam could not play Example Movie" and "None of its versions plays on this app now. · Jellyfin Web on Chrome". It opens the title.

- The same user and title are told once an hour at most. An app that tries again, or asks for each segment of a broken stream, is told of once.
- Users' own targets cannot choose it: they saw the failure.
- A new target has it chosen. Targets saved before it existed do not receive it until it is chosen.

### Crashes

Polyfin tells the server's targets and administrators' own when it stops on an unrecovered panic or a fatal error.

- Go writes the trace to standard error and, through `debug.SetCrashOutput`, to `crash.log` in the data folder.
- A restart under Docker or a service manager loses standard error from the admin app, so the next start reads the file instead.
- Polyfin keeps the last ten crashes in the database.
- **System › Health › Polyfin** shows the last one with its time and the first lines of its trace.
- The message is "Polyfin crashed", with the time and the first six lines of the trace, and opens **System › Health**. It is sent once the targets are read.
- It is urgent: ntfy, Gotify and Pushover show it with a high priority.

A Go trace holds function names, file paths and memory addresses, never request data. A panic's message may still hold anything, so the lines kept are redacted as the log is: addresses keep only their host, and tokens, keys and passwords are hidden.

The file is moved to `crash.previous.log` until the crash is kept in the database, then deleted. A start that cannot reach the database tells the crash at the next start.

Targets that chose **Health problem found** before this event existed have it chosen.

### Blocked accounts

The wrong password that blocks an account is told once, while **Block an account after this many wrong passwords** is set in **Settings › Security** (see [Blocking after wrong passwords](sign-in-and-security.md#blocking-after-wrong-passwords)).

- Neither the attempts during the block nor the right password then tell it again.
- A new block after the first one ends is told again.
- Attempts refused for their address, after too many failed sign-ins from it (429), never reach the account and are not told: they come all the time from anywhere on the internet.

| Who | Title | Message | Opens |
|---|---|---|---|
| The server's targets and administrators' own | "Account blocked: sam" | "5 wrong passwords in a row: sam cannot sign in until 21:15. Last attempt from 203.0.113.5, with Jellyfin Web on Chrome. Unblock it from its page." | The user's page in the admin app, where **Unblock** is. |
| The user's own targets | "Your account is blocked" | The same figures, and "If it was not you, someone is trying your password." | **My account**, where the password is changed. |

- The time is the server's.
- The address is the client's, as Polyfin resolves it. Behind a reverse proxy, it is the client's only once the proxy is a [known proxy](reverse-proxy.md#known-proxies).
- A sign-in through the admin app names the admin app.

### Sign-ins on new devices

An account's own targets are told when it signs in from a Jellyfin app or the web player on a device it never used.

- The title is "Sign-in on a new device".
- The message is "Jellyfin Web on Chrome signed in to sam from 203.0.113.5. If it was not you, change your password: a new password signs every other device out.".
- It opens **My account**.

A device is the identifier the app sends. Polyfin keeps the devices each account signed in from, even after they sign out. Polyfin does not tell:

- the account's first device;
- a device it used before;
- a sign-in through the admin app;
- the devices signed in when Polyfin started keeping them, which count as used.

A new target in the admin app has both **Account blocked** and **Sign-in on a new device** chosen. Targets saved before they existed receive them once chosen.

### Programme reminders

A user with the **Live TV** permission sets a reminder from a programme's page in the web player, for that programme or for every airing of it (see [Programme reminders](live-tv.md#programme-reminders)). Their own targets are told as long before the programme as **Remind me before** says, 5 minutes by default.

- The title is "Starting at 20:30: Example Show", with the server's time.
- The message is "Example Show starts in 5 minutes on Example TV.".
- It opens the channel in the web player when a **Public address** is set.
- It is in the language of the web player the reminder was set in when Polyfin speaks it, else in the server language.
- A reminder sent late, after Polyfin was off at its time, tells the minutes left.
- It is not urgent: ntfy, Gotify and Pushover show it with their normal priority.

**Also show reminders on my apps**, in **My account › Notifications**, also shows the reminder on the user's Jellyfin apps that accept messages. It is off by default.

- The reminder shows as **Send a message** shows one (see [Administration](administration.md#stopping-a-playback-or-sending-a-message)), until dismissed.
- Those apps are the web player while it is open, and the apps that keep a connection open and accept remote control.
- When no target of the user chose the event and this option is off, the web player's button and **My account › Notifications** say that reminders reach the user nowhere.

A new target in the admin app has the event chosen. Targets saved before it existed receive it once chosen.

### Watchlist titles available

Polyfin checks the titles of each user's Watchlist playlist they have not started for versions (see [Now available and Still waiting](tracking-imports.md#now-available-and-still-waiting)). The user's own targets are told the first time versions are found for a title that earlier checks found none for.

- The title is "Versions found: Example Movie (2008)".
- The message is "Versions of Example Movie (2008), on your watchlist, were found. They have not been tried yet.".
- An episode is named after its series, with its season and number, such as "Example Series S01E02", without a year. Its `watchlistTitle.kind` is `episode` (see [Webhook reference](webhook-reference.md)).
- It opens the title in the [web player](web-client.md).
- It says versions were found, not that the title plays: a version is only tried when it plays.
- Each title is told of once: not when it already had versions at its first check, as when it was added, nor again when it finds versions anew after losing them.
- A title that parental control, blocked genres or the user's hidden titles keep from them is never told of.
- It is not urgent: ntfy, Gotify and Pushover show it with their normal priority.

A new target in the admin app has the event chosen. Targets saved before it existed receive it once chosen.

### Weekly summary

Once a week, Polyfin sends the summary of the 7 days before. It goes at the day and hour set in **Settings › Notifications**, Monday at 9:00 by default, in the server's time zone.

| Recipients | What it tells |
|---|---|
| The server's targets and administrators' own | The server's week: the hours watched and the number of playbacks, the five movies and the five series played longest (from the [playback history](statistics.md)), the new episodes of the series users follow (as the new-episode check found them, see [New episodes](#new-episodes)), the users who joined through an invite link, the files added to the local folders, and the problems **System › Health** found that week. |
| Every other user's targets | Their own week: their hours watched and playbacks, the movies and series they watched (ten of each at most), and the new episodes of the series they follow. |

A recipient with nothing to tell that week receives nothing. Nothing means no playback and no new episode, and for the server's week, no user who joined, no file added and no problem.

The title is "The week on Home" for the server's week, and "Your week on Home" for a user's, Home being the server name. Each kind receives it in its own form:

- Discord, ntfy, Telegram, Gotify and Pushover receive a short text with the key figures, one per line. For example: "Week of October 3 to 10.", "12.5 hours watched, 42 playbacks.", "Movies: Example Movie, Other Movie.", "New user: sam.". It links to **System › Statistics**, or to the user's statistics in **My account**.
- An email holds the whole summary, each title linking to its page in the [web player](web-client.md) (see [Messages for each kind](webhook-reference.md#messages-for-each-kind)).
- A webhook receives the figures as JSON (see [The weekly summary event](webhook-reference.md#the-weekly-summary-event)).

The end of the last week sent is kept in the database:

- A restart does not send a week again.
- A server that was off at the hour sends the summary when it starts again, for the week that ended at the hour.
- A server off for longer sends the last week only.
- Polyfin looks whether the summary is due every minute.

**Send the weekly summary**, in **System › Schedule**, sends the summary of the last 7 days now to every target that chose it. It does not change when the next one is sent.

A new target has the weekly summary chosen: it is one message a week. Targets saved before it existed do not receive it until it is chosen in their **Events**.

### Month in review

On the 1st of each month, at the hour of the weekly summary (9:00 by default), Polyfin sends the review of the month that ended. The month runs from its 1st at 0:00 until the 1st of the next at 0:00, in the server's time zone.

- It tells what the [weekly summary](#weekly-summary) tells, over the month.
- Its lists are as long as the weekly summary's: five movies and five series for the server, ten of each for a user, the first 20 new episodes, the first ten titles added to the local folders.
- The title is "The month on Home" for the server's month, and "Your month on Home" for a user's.
- The short text starts with "Month of September 2026.".
- A recipient with nothing to tell that month receives nothing.

No target has the month in review chosen at first, not even a new one. A target chooses it beside the weekly summary, or in its place.

### Year in review

On January 1st, at the same hour, Polyfin sends the review of the year that ended, from January 1st at 0:00 until the next January 1st at 0:00.

| Recipients | What it tells |
|---|---|
| The server's targets and administrators' own | The server's year: the hours watched and the number of playbacks, the ten movies and the ten series played longest, the month with the most hours, the users who joined through an invite link, and the files added to the local folders. |
| Every other user's targets | Their own year: their hours watched and playbacks, the ten movies and the ten series they played longest, and their month with the most hours. |

- The title is "The year on Home" for the server's year, and "Your year on Home" for a user's.
- The short text starts with "Year 2026.", then the hours watched, and "Most watched month: March, 41.5 hours.".
- A recipient with nothing to tell that year receives nothing.

The year counts the playbacks the [playback history](statistics.md) still keeps. **Days to keep playback history** is 365 by default, so a year in review sent after January 1st misses as many days at the start of the year. With fewer days kept, its first months are missing.

A new target has the year in review chosen. So does every target that had the weekly summary chosen when Polyfin added the year in review. It is an event of its own: unchosen, the target keeps its weekly summary or month in review.

The month and year in review keep the last month and year sent in the database, as the weekly summary does:

- A restart does not send them again.
- A server that was off on the 1st at the hour sends them when it starts again, within that month only.
- A server off until the 1st of the next month never sends that review.

The "Send the month in review" and "Send the year in review" tasks, in **System › Schedule**, send the review of the last month or year that ended now. They send it to every target that chose it, without changing when the next one is sent.

Messages are written in the server language (**Settings › General**).

### Stopped connections

A connection stops when the app or service refuses what Polyfin kept for the user:

- an app connection (see [Staying connected](moving-to-polyfin.md#staying-connected)) stops when Nuvio or Stremio refuses the sign-in Polyfin kept, such as a key signed out or a session ended;
- a tracking service (see [Connection status](tracking.md#connection-status)) stops when it refuses the user's token or key, and shows **Connect again**.

Either way, nothing comes over from it, or reaches it, until the user connects it again.

A connection that stops is told once, as it stops:

- The title is "Connection stopped: Stremio".
- The message names the app or service and the user, and says what to do. For example: "Stremio refused the sign-in Polyfin kept for sam: nothing is imported from it until sam signs in again under My account › Import from an app, with Stay connected ticked.". For a tracking service, it ends "until sam presses Connect again under My account › Tracking.".
- With a public address, the message opens that page of the admin app.
- It is not urgent: ntfy, Gotify and Pushover show it with their normal priority.

Only refusals are told:

- An app or service that does not answer, fails or asks to wait is tried again, and nothing is told.
- An AniList token that expires, after a year, stops the connection as a refusal does, and is told the same way, with the service `anilist`. MyAnimeList's is told with `myanimelist` (see [AniList and MyAnimeList](tracking.md#anilist-and-myanimelist)).
- What stopped is kept in the database: neither a restart nor a later refusal of the same connection tells it again.
- A connection made again, then refused again, is told again.
- **System › Health** lists the connections stopped, by user and app or service, each opening the user's page. They are not told as health problems, since this event already told them.

A new target has this event chosen. Targets saved before it existed do not receive it until it is chosen in their **Events**.

## Delivery

Nothing is sent while a request is answered. Each target has its own queue, sent in order in the background.

| Limit | Value |
|---|---|
| Requests at once, across all targets | 4 |
| Wait for one request, or one email from connecting to the SMTP server to its last answer | 10 seconds |
| First retry | after 30 seconds, then twice as long each time, up to 10 minutes |
| Messages waiting for one target | 100 at most |
| Time before an undelivered message is dropped | 1 hour |

- A message is tried again after a network error, a timeout, a 5xx answer, or a 4xx answer from an SMTP server (busy, or too many messages).
- A 429 answer waits as long as its `Retry-After` header asks, or Discord's `retry_after`, or the `retry_after` of Telegram's `parameters`.
- A message not delivered within an hour is dropped, and the target shows **Not reached**. It shows so after three failures in a row already.
- A 401, 403, 404 or 410, or another refusal listed under **Refused** in [Target states](#target-states), drops the message and shows **Refused** at once.
- Messages still waiting when Polyfin stops are dropped.

### Pacing

Each target's form has two fields that pace its messages:

| Field | Values | What it does |
|---|---|---|
| **Messages a minute, at most** | 1 to 60, empty for no limit | Past it, the next messages wait their turn, in order, until a minute has passed since the oldest of the last ones. Each request counts, retries included; **Send a test** does not. |
| **Group plays and new episodes, in minutes** | 1 to 1,440, empty to send each at once | The playback events, shared links played and new episodes wait that long after the first of them, then go as one message. |

A group goes as "3 notifications", with a line per event: its title, then its message.

- A group of one goes as the event alone.
- A group reaching 100 events goes at once.
- Every other event still goes at once, failures, crashes and Health problems among them.
- A webhook receives the group as one event of type `grouped` (see [The webhook event](webhook-reference.md#the-webhook-event)).
- Messages a group holds when Polyfin stops are dropped, as are those waiting in a queue.

The targets list shows both fields, as "At most 5 a minute" and "Grouped every 15 min".

### Recent deliveries

**Recent deliveries**, on each target, lists the last 100 messages sent to it, newest first, kept 30 days. Users see their own targets' deliveries only.

Each delivery shows when it was sent, the message's title (a test, or how many events a group held), and its outcome:

| Outcome | Meaning |
|---|---|
| **Delivered** | The target accepted it. |
| **Refused** | It answered 401, 403, 404 or 410. |
| **Message refused** | It answered another 4xx. |
| **Not delivered** | Polyfin gave up after an hour of retries. |
| **Dropped** | Too many messages waited. |
| **Cannot be decrypted** | The target's secret cannot be decrypted. |

Each delivery also shows the status the target answered, the start of its answer without its address or tokens, and how many attempts it took.

<a id="compared-with-jellyfin"></a>

**Compared with Jellyfin**

- Polyfin builds the messages itself. Jellyfin sends notifications through its webhook plugin, which administrators install and configure with templates for each destination ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Polyfin's events are its own: new episodes of followed series, recordings, Health problems, crashes, playbacks and failed plays, stopped connections, users who joined, blocked accounts, sign-ins on new devices, share links played, programme reminders, Watchlist titles available, and the weekly summary, month and year in review. Jellyfin's are item added, playback, users and tasks; Polyfin has no library scan to tell of added items ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Each Polyfin target may cap its messages a minute and group plays and new episodes, and keeps its recent deliveries ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Each Polyfin user chooses their own targets and events in **My account**, besides the server's ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Polyfin's webhook sends one documented, versioned JSON event rather than templates, and Discord, ntfy, email, Telegram, Gotify and Pushover get messages made for them ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Polyfin stores addresses, tokens, keys and the SMTP password encrypted, and shows failing targets ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
- Jellyfin apps see none of this: nothing of it is part of Jellyfin's API ([jellyfin-plugin-webhook](https://github.com/jellyfin/jellyfin-plugin-webhook)).
