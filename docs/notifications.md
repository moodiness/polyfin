# Notifications

Polyfin can tell users and administrators when something they care about happens: a new episode of a series they follow, a recording that finished or failed, a problem **System › Health** found or that was solved, a user who joined through an invite link, a new version of Polyfin; and once a week, what happened that week (see [Weekly summary](#weekly-summary)). Messages go to targets: a generic webhook, a Discord channel's webhook, an ntfy topic, an email address, a Telegram chat, a Gotify server or a Pushover user.

## Targets

There are two kinds of owners:

- **Settings › Notifications**, for administrators: the server's targets. They receive the events of every user, the health events, the users who joined, new versions, and the server's weekly summary.
- **My account › Notifications**, for every user: their own targets. They receive that user's new episodes and recordings, and their weekly summary. An administrator's own targets may receive the health events, the users who joined and new versions too, and the server's weekly summary in place of their own.

**Add a target** asks for its kind, a **Name**, the **Events** it receives, and:

- for a **Webhook**, its address, to which Polyfin posts each event as JSON (see [The webhook event](#the-webhook-event));
- for **Discord**, the channel webhook's address (in Discord: the channel's settings, Integrations, Webhooks, Copy Webhook URL);
- for **ntfy**, the **ntfy server** (`https://ntfy.sh` when left empty), the **Topic**, and an **Access token** for a topic that needs signing in;
- for **Email**, the **Email address** messages go to, through the server's SMTP server (see [Email](#email)); without one, the kind cannot be added, and the form says why;
- for **Telegram**, the **Chat**, a chat's number such as `-1001234567890` or a public channel's `@name`, and the **Bot token** BotFather gave when the bot was created; the bot must be a member of the chat;
- for **Gotify**, the **Gotify server**'s address and an **Application token** (in Gotify: Apps, Create Application);
- for **Pushover**, the **User key** (or a group key) and the **Application token** of an application created on Pushover.

Webhook and Discord addresses, ntfy access tokens, Telegram bot tokens, Gotify application tokens, and Pushover user keys and application tokens are secrets: they are stored encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)), never shown again (the list shows a webhook's or Discord address's host only), and never written to the log. A secret that cannot be decrypted shows the target as **Enter it again**, and on **System › Health** under **Stored keys**. An email target's address, a Telegram chat and a Gotify server are shown as they are.

A user who is not an administrator may only add targets on public addresses, as their own addons: an address on the local network is refused, and messages never reach one.

Each target shows its state:

| State | Meaning |
|---|---|
| **Working** | The last message was delivered. |
| **Nothing sent yet** | No message was sent to it yet. |
| **Refused** | It answered 401, 403, 404 or 410: its address or token is wrong, or it was deleted. Telegram refusing an unknown chat, Pushover an unknown user key or application token, and an SMTP server refusing the user and password, the sender or the recipient, count too. |
| **Message refused** | It refused the last message with another 4xx status. |
| **Not reached** | Recent messages could not be delivered: no answer, server errors, or for email no SMTP server. |
| **Off** | **Send messages to this target** is off. |

**Send a test** sends a test message at once and tells whether the target accepted it. A delivered message, a test included, clears the target's problem. The list shows the HTTP status the target answered with its problem, or for email the SMTP server's code, such as `SMTP 535`.

### Email

Email targets go through one SMTP server, which an administrator sets under **Settings › Notifications**, in **Email**. Email targets can be added once an **SMTP server** and a **Sender address** are saved.

Polyfin connects to the server with the **SMTP encryption** chosen:

- **STARTTLS** connects without encryption, then asks the server to encrypt: when it does not offer to, nothing is sent;
- **TLS from the start** encrypts the connection from the start;
- **None** sends everything as it is. The password is then only sent to a server on this machine (`localhost`).

The server's certificate must name its host. Polyfin signs in with the **SMTP user** and **SMTP password**, by `PLAIN`, or `LOGIN` when the server offers only that, once the connection is encrypted; with no user, it sends without signing in. The password is stored encrypted with `POLYFIN_SECRET_KEY` and never shown again.

### Settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Public address** | **Settings › Notifications** | empty | The address people open Polyfin at, such as `https://media.example.org`. Links in messages start with it: an episode or a recording opens in the [web client](web-client.md), a health problem on **System › Health**. Empty, messages carry no link. |
| **SMTP server** | **Settings › Notifications** | empty | The host name of the SMTP server email targets go through, such as `smtp.example.org`. Empty, email targets cannot be added. |
| **SMTP port** | **Settings › Notifications** | 587 | Usually 587 with STARTTLS, 465 with TLS from the start. |
| **SMTP encryption** | **Settings › Notifications** | STARTTLS | STARTTLS, TLS from the start, or None (see [Email](#email)). |
| **SMTP user** | **Settings › Notifications** | empty | The user Polyfin signs in with. Empty, it sends without signing in. |
| **SMTP password** | **Settings › Notifications** | empty | Its password, stored encrypted. |
| **Sender address** | **Settings › Notifications** | empty | The address messages come from, such as `polyfin@example.org`. Email targets need it. |
| **Sender name** | **Settings › Notifications** | empty | The name shown with the sender address. Empty, the server name is. |
| **Send the weekly summary on** | **Settings › Notifications** | Monday | The day of the week the [weekly summary](#weekly-summary) is sent. |
| **Send the weekly summary at** | **Settings › Notifications** | 9:00 | Its hour, in the server's time zone. |

## Events

| Event | `type` | Who receives it |
|---|---|---|
| New episode | `new_episode` | The targets of each user following the series; the server's targets once per episode, whoever follows it. |
| Recording finished | `recording_finished` | The targets of the user who scheduled it, and the server's. |
| Recording failed | `recording_failed` | The same. |
| Health problem found | `health_problem` | The server's targets, and administrators' own. |
| Health problem solved | `health_solved` | The same. |
| User joined | `user_joined` | The server's targets, and administrators' own. |
| New version | `new_version` | The server's targets, and administrators' own. |
| Playback started | `playback_started` | The targets of the user who plays, and the server's. |
| Playback paused | `playback_paused` | The same. |
| Playback resumed | `playback_resumed` | The same. |
| Playback stopped | `playback_stopped` | The same. |
| Weekly summary | `weekly_summary` | The server's targets and administrators' own: the server's week. Every other user's targets: their own week. |

### New episodes

A user follows the series they played an episode of (the 50 most recently played) and the series they marked favorite, as the Upcoming row of Jellyfin apps shows them. Every two hours, starting five minutes after Polyfin starts, Polyfin looks at the episodes of these series for those that became available. Only the users with a target for new episodes or the weekly summary are looked at, or all of them when the server has one, or an administrator one for the weekly summary. An episode is told once per user; never one the series already had when Polyfin first looked at it (so neither starting to use notifications nor following a series sends old episodes); never one released more than a week ago, which its addon added late; never one without a release date. The series' descriptions are those the library keeps: an old one is asked for again in the background, so a new episode may be told one check later. Each new episode found is kept for the weekly summary, whether or not a target was told of it.

### Recordings

A recording that ends is finished, or partial when part of its programme is missing (see [Live TV](live-tv.md#recordings)). One that recorded nothing failed.

### Health problems

Every five minutes, Polyfin looks at the problems **System › Health** shows under **Needs attention**: the database, disk space, addons, IPTV sources, programme guides, failed tasks, backups and stored keys. Conversions at their limit and thumbnails paused for a host come and go with the load: they are not told. A problem is told once two checks in a row found it, and solved once two checks in a row no longer find it, so that a brief failure sends nothing. What was told is kept in the database: a restart neither tells a problem again nor forgets to tell when it is solved. Each problem told is kept for the weekly summary too.

### Users who joined

A user who created their account through an invite link (see [Invite links](users.md#invite-links)) is told once, naming who created the link: the title is "New user: sam", which is an email's subject, and the message "sam joined through alex's invite.". The message opens the user's page in the admin app. It is not urgent: ntfy, Gotify and Pushover show it with their normal priority. Each user who joined is kept for the weekly summary too.

### New versions

While **Check for new versions** is on, under **Settings › General**, Polyfin asks GitHub once a day for its latest release (see [New versions](administration.md#new-versions)). A version newer than the running one is told once: the title is "New version: Polyfin 1.5.0", and the message "Polyfin 1.5.0 is available. This server runs version 1.4.0.". The message opens the release notes on GitHub, with or without a public address. The version told is kept in the database: a restart does not tell it again. A development build never tells one. Targets saved before this event existed do not receive it until it is chosen.

### Playbacks

A video (a movie, an episode, a channel, a recording or a Replay programme) or a song or an audiobook that starts playing in a Jellyfin app, pauses, resumes or stops is told once per change, whatever the number of reports the app sends: positions as it plays are never told. A playback whose app stops reporting is told stopped five minutes after its last report (see [Statistics](statistics.md#playbacks)). Each message names the user, the title (a movie, an episode with its series, a channel, a song with its artist), the app and the device, the position, and how it plays: direct play, remux or conversion, such as "sam is watching Example Series S01E02" and "Pilot · Jellyfin Web on Chrome · 12:34 · Direct play".

Targets saved before these events existed do not receive them until they are chosen, and a new target in the admin app leaves them unchosen: each user plays many times a day.

### Weekly summary

Once a week, at the day and hour set under **Settings › Notifications** (Monday at 9:00 by default, in the server's time zone), Polyfin sends the summary of the 7 days before:

- to the server's targets and administrators' own, the server's week: the hours watched and the number of playbacks, the five movies and the five series played longest (from the [playback history](statistics.md)), the new episodes of the series users follow (as the new-episode check found them, see [New episodes](#new-episodes)), the users who joined through an invite link, the files added to the local folders, and the problems **System › Health** found that week;
- to every other user's targets, their own week: their hours watched and playbacks, the movies and series they watched (ten of each at most), and the new episodes of the series they follow.

A recipient with nothing to tell that week (no playback and no new episode, and for the server's week no user who joined, no file added and no problem) receives nothing.

The title is "The week on Home" for the server's week, and "Your week on Home" for a user's, Home being the server name. Discord, ntfy, Telegram, Gotify and Pushover receive a short text with the key figures, one per line, such as "Week of October 3 to 10.", "12.5 hours watched, 42 playbacks.", "Movies: Example Movie, Other Movie.", "New user: sam.", and the link to **System › Statistics**, or to the user's statistics under **My account**. An email holds the whole summary, each title linking to its page in the [web client](web-client.md) (see [Messages for each kind](#messages-for-each-kind)); a webhook receives the figures as JSON (see [The weekly summary event](#the-weekly-summary-event)).

The end of the last week sent is kept in the database: a restart does not send a week again, and a server that was off at the hour sends the summary when it starts again, for the week that ended at the hour. A server off for longer sends the last week only. Polyfin looks whether the summary is due every minute. **Send the weekly summary**, under **System › Schedule**, sends the summary of the last 7 days now, to every target that chose it, without changing when the next one is sent.

Targets saved before the weekly summary existed do not receive it until it is chosen in their **Events**. A new target has it chosen: it is one message a week.

Messages are written in the server language (**Settings › General**).

## Delivery

Nothing is sent while a request is answered. Each target has its own queue, sent in order in the background, at most four requests at once. Each request, or each email from connecting to the SMTP server to its last answer, waits 10 seconds at most. After a network error, a timeout, a 5xx answer, or a 4xx answer from an SMTP server (busy, or too many messages), the message is tried again after 30 seconds, then twice as long each time, up to 10 minutes; a 429 answer waits as long as its `Retry-After` header (or the `retry_after` of Discord, or of Telegram's `parameters`) asks. A message not delivered within an hour is dropped, and the target shows **Not reached**; it shows so after three failures in a row already. A 401, 403, 404 or 410, or a refusal as in **Refused** above, drops the message and shows **Refused** at once. Messages still waiting when Polyfin stops are dropped; at most 100 wait for one target.

## The webhook event

A webhook receives each event as JSON in a `POST`, with the headers `Content-Type: application/json`, `User-Agent: Polyfin/<version>` and `X-Polyfin-Event: <type>`. Any 2xx answer counts as delivered.

```json
{
  "version": 1,
  "id": "6f1d3c0a9b7e4f2c8a5d1e3b7c9f0a2d",
  "type": "new_episode",
  "at": "2026-10-09T18:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "New episode of Example Series",
  "message": "S02E05 · The Fifth",
  "url": "https://media.example.org/web/#/details?id=0a1b2c3d4e5f60718293a4b5c6d7e8f9&serverId=fedcba9876543210fedcba9876543210",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "episode": {
    "id": "0a1b2c3d4e5f60718293a4b5c6d7e8f9",
    "name": "The Fifth",
    "seriesId": "99887766554433221100ffeeddccbbaa",
    "seriesName": "Example Series",
    "season": 2,
    "number": 5,
    "premiereDate": "2026-10-09T00:00:00Z",
    "providerIds": { "Imdb": "tt0000000" }
  }
}
```

A user who joined through an invite link:

```json
{
  "version": 1,
  "id": "0f9e8d7c6b5a49382716051f2e3d4c5b",
  "type": "user_joined",
  "at": "2026-10-09T18:05:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "New user: sam",
  "message": "sam joined through alex's invite.",
  "url": "https://media.example.org/admin/users/11223344556677889900aabbccddeeff",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "invite": {
    "id": "a1b2c3d4e5f60718293a4b5c6d7e8f90",
    "createdBy": { "id": "ffeeddccbbaa00998877665544332211", "name": "alex" }
  }
}
```

| Field | Type | Meaning |
|---|---|---|
| `version` | number | The version of this format, 1. Fields may be added within a version; a field removed or changed in meaning makes a new version. |
| `id` | string | Unique to the event: a message tried again keeps it, so a webhook can tell a repeat. |
| `type` | string | `new_episode`, `recording_finished`, `recording_failed`, `health_problem`, `health_solved`, `user_joined`, `new_version`, `playback_started`, `playback_paused`, `playback_resumed`, `playback_stopped`, `weekly_summary`, or `test` for **Send a test**. |
| `at` | string | When the event happened, RFC 3339 in UTC. |
| `server` | object | `id` as Jellyfin apps know the server, `name`, and `url`, the public address, or null. |
| `title`, `message` | string | The event for people, in the server language. |
| `url` | string or null | Opens what the event is about; null without a public address. For `new_version`, the release notes on GitHub, always set. |
| `user` | object or null | The user the event is about (`id` as Jellyfin apps know it, `name`): for `user_joined`, the new user; for `weekly_summary`, the user whose week it is. Null for health events, new versions, new episodes sent to the server's targets, and the server's weekly summary. |
| `episode` | object | For `new_episode`: the episode's and its series' item IDs, names, season and number, release date, and the episode's provider IDs. |
| `recording` | object | For recordings: `id` (its item; gone once it failed), `name`, `channelId`, `channelName`, the programme's planned `start` and `end`, and `partial`. |
| `problem` | object | For health events: `key`, which names the problem the same way while it lasts, `severity` (`error` or `warning`), `text`, and `since`. |
| `invite` | object | For `user_joined`: the invite link's `id`, and `createdBy`, the administrator who created it (`id`, `name`), null once deleted. |
| `release` | object | For `new_version`: `version`, the new version, `url`, its release notes, and `current`, the version the server runs. |
| `playback` | object | For playbacks; see below. `user` is the user who plays, and `at` when the change happened. |
| `summary` | object | For `weekly_summary`; see [The weekly summary event](#the-weekly-summary-event). |

The playback events were added within version 1: their `playback` object is new, and no other field changed. So was `new_version`, with its `release` object:

```json
{
  "version": 1,
  "id": "5d4c3b2a1f0e49d8c7b6a5948372615f",
  "type": "new_version",
  "at": "2026-10-10T09:01:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": null },
  "title": "New version: Polyfin 1.5.0",
  "message": "Polyfin 1.5.0 is available. This server runs version 1.4.0.",
  "url": "https://github.com/moodiness/polyfin/releases/tag/v1.5.0",
  "user": null,
  "release": {
    "version": "1.5.0",
    "url": "https://github.com/moodiness/polyfin/releases/tag/v1.5.0",
    "current": "1.4.0"
  }
}
```

```json
{
  "version": 1,
  "id": "0c2b9e5d7a1f4e3c9b8a6d5e4f3c2b1a",
  "type": "playback_paused",
  "at": "2026-10-10T20:15:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": null },
  "title": "sam paused Example Series S01E02",
  "message": "Pilot · Jellyfin Web on Chrome · 12:34 · Conversion",
  "url": null,
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "playback": {
    "itemId": "0a1b2c3d4e5f60718293a4b5c6d7e8f9",
    "kind": "episode",
    "name": "Pilot",
    "seriesId": "99887766554433221100ffeeddccbbaa",
    "seriesName": "Example Series",
    "season": 1,
    "number": 2,
    "channelId": null,
    "channelName": null,
    "artist": null,
    "app": "Jellyfin Web",
    "device": "Chrome",
    "position": 754,
    "paused": true,
    "method": "conversion",
    "converted": true,
    "startedAt": "2026-10-10T20:02:00Z",
    "played": 754
  }
}
```

| `playback` field | Type | Meaning |
|---|---|---|
| `itemId` | string | The item played, as Jellyfin apps know it. |
| `kind` | string | `movie`, `episode`, `channel`, `recording`, `replay` (a Replay programme), `song` or `audiobook`. |
| `name` | string | The item's name. |
| `seriesId`, `seriesName`, `season`, `number` | string, number or null | For an episode: its series, season and number. |
| `channelId`, `channelName` | string or null | For a channel, a Replay programme or a recording: its channel. |
| `artist` | string or null | For a song or an audiobook: its first artist. |
| `app`, `device` | string | The app and the device, as they signed in. |
| `position` | number | Where the playback is, in seconds. |
| `paused` | boolean | Whether it is paused. |
| `method` | string or null | `direct_play`, `direct_stream` (remuxed, nothing converted) or `conversion`; null when not known yet. A playback converted for a while stays `conversion`. |
| `converted` | boolean | Whether `method` is `conversion`. |
| `startedAt` | string | When the playback started, RFC 3339 in UTC. |
| `played` | number | How long it played since, in seconds, pauses left out. |

### The weekly summary event

The weekly summary was added within version 1: its `summary` object is new, and no other field changed. The server's summary has `user` null; a user's names them, and leaves the server's parts empty. `at` is when it was sent, and `url` opens **System › Statistics**, or the user's statistics under **My account**.

```json
{
  "version": 1,
  "id": "4e5f6a7b8c9d40e1f2a3b4c5d6e7f809",
  "type": "weekly_summary",
  "at": "2026-10-12T07:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "The week on Home",
  "message": "Week of October 5 to 12.\n3.1 hours watched, 4 playbacks.\nMovies: Example Movie.\nSeries: Example Series.\n1 new episode of the series users follow.\nNew user: sam.\n2 files added to the local folders.\n1 problem found by System › Health.",
  "url": "https://media.example.org/admin/system/statistics",
  "user": null,
  "summary": {
    "scope": "server",
    "start": "2026-10-05T07:00:00Z",
    "end": "2026-10-12T07:00:00Z",
    "plays": 4,
    "played": 11100,
    "movies": [{ "id": "0b1c2d3e4f5061728394a5b6c7d8e9f0", "name": "Example Movie", "plays": 2, "played": 7200 }],
    "series": [{ "id": "99887766554433221100ffeeddccbbaa", "name": "Example Series", "plays": 2, "played": 3900 }],
    "newEpisodes": 1,
    "episodes": [
      {
        "id": "0a1b2c3d4e5f60718293a4b5c6d7e8f9",
        "name": "The Fifth",
        "seriesId": "99887766554433221100ffeeddccbbaa",
        "seriesName": "Example Series",
        "season": 2,
        "number": 5
      }
    ],
    "users": [
      {
        "id": "11223344556677889900aabbccddeeff",
        "name": "sam",
        "joinedAt": "2026-10-09T18:05:00Z",
        "invitedBy": { "id": "ffeeddccbbaa00998877665544332211", "name": "alex" }
      }
    ],
    "addedFiles": 2,
    "added": [{ "name": "Other Movie", "kind": "movie", "files": 2 }],
    "problems": [{ "key": "backup", "severity": "error", "text": "The last backup failed.", "foundAt": "2026-10-08T04:10:00Z" }]
  }
}
```

| `summary` field | Type | Meaning |
|---|---|---|
| `scope` | string | `server` for the server's week, sent to the server's targets and administrators' own; `user` for a user's. |
| `start`, `end` | string | The week, from `start` until before `end`, RFC 3339 in UTC: 7 days at the day and hour of the settings, or the 7 days before **Send the weekly summary** ran. |
| `plays`, `played` | number | How many videos played that week, and how long, in seconds, pauses left out. |
| `movies`, `series` | array | The movies and series played longest: five of each for the server, ten for a user. Each has the `id` of its item (a series' for a series) as Jellyfin apps know it, its `name`, its `plays`, and `played`, in seconds. |
| `newEpisodes` | number | How many new episodes the new-episode check found that week, of the series users follow, or for a user of the series they follow. |
| `episodes` | array | The first 20 of them, by series, season and number: the episode's `id` and `name`, its series' `seriesId` and `seriesName`, its `season` and `number`. |
| `users` | array | For the server: the users who joined through an invite link, with `id`, `name`, `joinedAt`, and `invitedBy`, the administrator who created the invite (`id`, `name`), null once deleted. Empty for a user. |
| `addedFiles` | number | For the server: how many files the scans of local folders first found that week. 0 for a user. |
| `added` | array | For the server: the titles of these files, those with the most files first, ten at most: the `name` of the title they were matched to, or else the name the files tell, their folder's `kind`, `movie` or `show`, and their number of `files`. Empty for a user. |
| `problems` | array | For the server: the problems **System › Health** found that week, as they were told: `key`, `severity`, `text`, and `foundAt`. Empty for a user. |

## Messages for each kind

Failed recordings and health problems that are errors are urgent: ntfy, Gotify and Pushover show them with a higher priority.

A Discord target receives one embed: the event's title, its message as the description, its link, a color by event (blue for new episodes, green for finished recordings and solved problems, red for failures and errors, amber for warnings, teal for users who joined and for playbacks that start or resume, grey for those that pause or stop, sky blue for new versions, indigo for weekly summaries), the time, and the server name in the footer, sent as `Polyfin`. Messages never mention anyone (`allowed_mentions` is empty).

An ntfy target receives a JSON publication to its server's root address, with `topic`, `title`, `message`, `tags` (`tv` for new episodes, `red_circle` for finished recordings, with `warning` when partial, `x` for failures, `warning` for problems, `white_check_mark` for solved ones, `wave` for users who joined, `package` for new versions, `arrow_forward` for playbacks that start or resume, `pause_button` and `stop_button` for those that pause or stop, `bar_chart` for weekly summaries, `bell` for tests), a high `priority` (4) for urgent events, and `click`, the event's link. The access token, if any, is sent as `Authorization: Bearer`.

An email target receives a `multipart/alternative` message, in plain text and in HTML, in the server language:

- the subject is the event's title;
- the body is its message, its link (named "Open" in HTML), and "Sent by Polyfin from" the server name. A weekly summary's body is the whole summary instead: the week and its figures, then a list for each part (the movies and series with how long and how many times they played, the new episodes, the users who joined, the files added, the problems found), each title linking to its page in the web client, each user to their page in the admin app, each problem to the page that shows it, and "Open the statistics";
- the headers are `From` (the sender name and address), `To`, `Subject`, `Date` (when the event happened), `Message-ID` (the same for each try of a message, at the sender address's domain), `Auto-Submitted: auto-generated`, so that mail servers send no automatic reply, and `X-Polyfin-Event: <type>`.

A Telegram target receives the Bot API's `sendMessage` (`POST https://api.telegram.org/bot<token>/sendMessage`), with `chat_id`, and `text` in HTML (`parse_mode` is `HTML`): the event's title in bold, its message, and its link named "Open", without a preview of the page.

A Gotify target receives a `POST` to its server's `/message`, with the application token as the `X-Gotify-Key` header, and `title`, `message`, a `priority` of 8 for urgent events and 5 for the others, and the event's link as the address a click opens (`extras` `client::notification` `click` `url`).

A Pushover target receives a `POST` to `https://api.pushover.net/1/messages.json`, with `token`, `user`, `title`, `message`, `url` (the event's link), a `priority` of 1 for urgent events and 0 for the others, and `timestamp`, when the event happened.

## For app developers

The admin API serves the targets: the server's to administrators at `/admin/api/notifications`, and each user's own at `/admin/api/account/notifications`.

- `GET` lists the `targets`, the `events` and `kinds` they may choose from, and `emailAvailable`, whether the SMTP server and sender address are set.
- `POST …/targets` adds a target, `PATCH …/targets/{id}` changes one (fields left out keep their values), `DELETE …/targets/{id}` deletes one, and `POST …/targets/{id}/test` sends a test message, answering `delivered`, the `status` (an HTTP status, or an SMTP code for email), and the target.
- A target has `kind` (`webhook`, `discord`, `ntfy`, `email`, `telegram`, `gotify` or `pushover`), `name`, `events`, `enabled`, and:
  - `address`: a webhook's or Discord target's scheme and host, an ntfy or Gotify target's server, an email target's recipient, empty for Telegram and Pushover;
  - `topic`: an ntfy target's topic;
  - `chat`: a Telegram target's chat;
  - `tokenSet`: whether an ntfy, Telegram, Gotify or Pushover target has a token.

  It never answers a secret address, token or key.
- To add or change one, send `address` (a webhook's or Discord target's address, an ntfy or Gotify target's server, an email target's recipient), `topic`, `chat`, `token` (an ntfy target's access token, empty for none, a Telegram bot's token, or a Gotify or Pushover application's token) and `userKey` (a Pushover target's user key).
- Errors:
  - `email_unavailable` (409): an email target without an SMTP server;
  - `invalid_email_address`, `invalid_chat`, `invalid_token`, `invalid_user_key` (400): a malformed field;
  - `target_unreadable` (409): a Pushover target whose key and token cannot be decrypted, changed with only one of them.
- `GET /admin/api/settings` answers `smtpHost`, `smtpPort`, `smtpSecurity` (`starttls`, `tls` or `none`), `smtpUser`, `smtpPasswordSet`, `smtpFrom` and `smtpFromName`. `PUT` takes them and `smtpPassword`, which is never answered: left out, it is kept; empty, it is removed. The errors are `invalid_smtp_host`, `invalid_smtp_port`, `invalid_smtp_security`, `invalid_smtp_account` and `invalid_smtp_sender`.
- `GET /admin/api/settings` answers `weeklySummaryDay`, the day the weekly summary is sent, 0 for Sunday to 6 for Saturday, and `weeklySummaryHour`, 0 to 23, in the server's time zone. `PUT` takes them; out of bounds, they are `invalid_weekly_summary_day` and `invalid_weekly_summary_hour`.

## Compared with Jellyfin

Jellyfin sends notifications through its webhook plugin, which administrators install and configure with templates for each destination, and whose events are Jellyfin's own (item added, playback, users, tasks). Polyfin builds the messages itself, and:

- its events are those of Polyfin: a new episode of a followed series (there is no library scan to tell of added items), recordings, Health problems, users who joined through an invite link, and a weekly summary for the server and for each user;
- each user chooses their own targets and events under **My account**, besides the server's;
- the webhook format is one documented, versioned JSON event rather than templates; Discord, ntfy, email, Telegram, Gotify and Pushover get messages made for them;
- addresses, tokens, keys and the SMTP password are stored encrypted, and failing targets are shown.

Jellyfin apps see none of this: nothing of it is part of Jellyfin's API.
