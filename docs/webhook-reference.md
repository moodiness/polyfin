# Webhook reference

This page is for integrators who read Polyfin's notifications with a program. It describes the JSON event a webhook target receives, what each other kind of target receives, and the admin API that manages targets. To set up targets and choose events, see [Notifications](notifications.md).

## The webhook event

A webhook receives each event as JSON in a `POST`. Any 2xx answer counts as delivered.

- The headers are `Content-Type: application/json`, `User-Agent: Polyfin/<version>` and `X-Polyfin-Event: <type>`.
- An address that is a Discord webhook's receives Discord's message instead (see [Adding a target](notifications.md#adding-a-target)).

A new episode:

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

### Event fields

| Field | Type | Meaning |
|---|---|---|
| `version` | number | The version of this format, 1. Fields may be added within a version; a field removed or changed in meaning makes a new version. |
| `id` | string | Unique to the event. A message tried again keeps it, so a webhook can tell a repeat. |
| `type` | string | `new_episode`, `recording_finished`, `recording_failed`, `health_problem`, `health_solved`, `server_crashed`, `user_joined`, `new_version`, `playback_started`, `playback_paused`, `playback_resumed`, `playback_stopped`, `playback_failed`, `weekly_summary`, `monthly_summary`, `yearly_summary`, `connection_stopped`, `account_blocked`, `new_device`, `share_played`, `programme_reminder`, `title_available`, `grouped` for a target's grouped message, or `test` for **Send a test**. |
| `at` | string | When the event happened, RFC 3339 in UTC. |
| `server` | object | `id` as Jellyfin apps know the server, `name`, and `url`, the public address, or null. |
| `title`, `message` | string | The event for people, in the server language. A `programme_reminder` is in the language its reminder was set in when Polyfin speaks it, else the server's. |
| `url` | string or null | Opens what the event is about; null without a public address. For `new_version`, the release notes on GitHub, always set. |
| `user` | object or null | The user the event is about (`id` as Jellyfin apps know it, `name`). See [Who the user is](#who-the-user-is). |
| `episode` | object | For `new_episode`: the episode's and its series' item IDs, names, season and number, release date, and the episode's provider IDs. |
| `recording` | object | For recordings: `id` (its item; gone once it failed), `name`, `channelId`, `channelName`, the programme's planned `start` and `end`, and `partial`. |
| `problem` | object | For health events: `key`, which names the problem the same way while it lasts, `severity` (`error` or `warning`), `text`, and `since`. |
| `crash` | object | For `server_crashed`: `at`, when it crashed, and `trace`, the first 20 lines of its trace, redacted as the log is. |
| `invite` | object | For `user_joined`: the invite link's `id`, and `createdBy`, the administrator who created it (`id`, `name`), null once deleted. |
| `release` | object | For `new_version`: `version`, the new version, `url`, its release notes, and `current`, the version the server runs. |
| `playback` | object | For playbacks. See [Playback events](#playback-events). |
| `failure` | object | For `playback_failed`: the title's `itemId` (null when it is not known), `kind`, `name`, `seriesId`, `seriesName`, `season` and `number` for an episode, `app`, `device`, and `reason`: `no_compatible_stream`, `limit` or `stream_error`. |
| `share` | object | For `share_played`: the link's `id`, the title's `itemId`, `kind` (`movie` or `episode`) and `name`, and the link's `expiresAt`. |
| `summary` | object | For `weekly_summary`, `monthly_summary` and `yearly_summary`. See [The weekly summary event](#the-weekly-summary-event) and [The month and year in review events](#the-month-and-year-in-review-events). |
| `connection` | object | For `connection_stopped`. See [Connection events](#connection-events). |
| `signIn`, `block` | object | For `account_blocked` and `new_device`. See [The sign-in events](#the-sign-in-events). |
| `programme` | object | For `programme_reminder`: `id`, the programme's item as Jellyfin apps know it, `name`, `channelId`, `channelName`, and its `start` and `end`, RFC 3339 in UTC. |
| `watchlistTitle` | object | For `title_available`: `id`, the movie's, series' or episode's item as Jellyfin apps know it, `kind` (`movie`, `series` or `episode`), `name` (an episode's own), `year` (0 when unknown) and `providerIds`, as `episode`'s. |
| `events` | array | For `grouped`: each event of the group whole, as it would have been sent alone (see [Pacing](notifications.md#pacing)). |

Every event added after the first ones was added within version 1, with its own object; no other field changed. These are the playback events, `new_version`, `connection_stopped`, `account_blocked`, `new_device`, the summaries, `programme_reminder` and `title_available`.

### Who the user is

| Event | `user` |
|---|---|
| `user_joined` | The new user. |
| `weekly_summary`, `monthly_summary`, `yearly_summary` | The user whose week, month or year it is; null for the server's summaries. |
| `connection_stopped` | The user whose connection stopped. |
| `account_blocked`, `new_device` | The account. |
| `programme_reminder` | The user who set the reminder. |
| Playback events | The user who plays. |
| Health events, `new_version` | Null. |
| `new_episode` | The user who follows the series; null when sent to the server's targets. |

### New version event

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

### Playback events

`user` is the user who plays, and `at` is when the change happened.

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
| `app`, `device` | string | The app and the device, as they signed in. A guest's playback through a share link has "Shared link" as its `app`. |
| `shared` | boolean | True for a guest's playback through a share link, which counts as the sharer's. |
| `position` | number | Where the playback is, in seconds. |
| `paused` | boolean | Whether it is paused. |
| `method` | string or null | `direct_play`, `direct_stream` (repackaged, nothing converted) or `conversion`; null when not known yet. A playback converted for a while stays `conversion`. |
| `converted` | boolean | Whether `method` is `conversion`. |
| `startedAt` | string | When the playback started, RFC 3339 in UTC. |
| `played` | number | How long it played since, in seconds, pauses left out. |

### Connection events

`connection_stopped` carries a `connection` object:

| `connection` field | Meaning |
|---|---|
| `kind` | `app` for an app connection, `tracking` for a tracking service. |
| `service` | The app or service as the API names it: `nuvio`, `stremio`, `trakt`, `simkl`, `mdblist`, `publicmetadb`, `anilist`, `myanimelist`, `lastfm` or `listenbrainz`. |
| `name` | The app or service as people name it. |

```json
{
  "version": 1,
  "id": "7e6d5c4b3a2f41e0d9c8b7a6f5e4d3c2",
  "type": "connection_stopped",
  "at": "2026-10-11T06:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "Connection stopped: Trakt",
  "message": "Trakt refused the token or key Polyfin kept for sam: nothing is sent to it or imported from it until sam presses Connect again under My account › Tracking.",
  "url": "https://media.example.org/admin/me/account#tracking",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "connection": { "kind": "tracking", "service": "trakt", "name": "Trakt" }
}
```

### The sign-in events

`account_blocked` and `new_device` carry `signIn` and `block` objects. `user` is the account, and `at` is when it was blocked or signed in.

```json
{
  "version": 1,
  "id": "7a8b9c0d1e2f43a5b6c7d8e9f0a1b2c3",
  "type": "account_blocked",
  "at": "2026-10-12T19:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "Account blocked: sam",
  "message": "5 wrong passwords in a row: sam cannot sign in until 21:15. Last attempt from 203.0.113.5, with Jellyfin Web on Chrome. Unblock it from its page.",
  "url": "https://media.example.org/admin/users/11223344556677889900aabbccddeeff",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "signIn": { "address": "203.0.113.5", "app": "Jellyfin Web", "appVersion": "10.11.0", "device": "Chrome", "deviceId": "TW96aWxsYS81LjA" },
  "block": { "until": "2026-10-12T19:15:00Z", "attempts": 5 }
}
```

| Field | Type | Meaning |
|---|---|---|
| `signIn` | object | For `new_device`, the sign-in; for `account_blocked`, the wrong password that blocked the account: `address`, the client's address as Polyfin resolves it, `app` and `appVersion`, and `device` and `deviceId`, the device's name and identifier, as the app sent them, empty when it sent none. Through the admin app, `app` is `Polyfin admin`, with no device. |
| `block` | object | For `account_blocked` only: `until`, when the block ends, RFC 3339 in UTC, and `attempts`, the wrong passwords in a row that blocked it. |

### The weekly summary event

`weekly_summary` carries a `summary` object.

- The server's summary has `user` null.
- A user's summary names them in `user`, and leaves the server's parts empty.
- `at` is when it was sent.
- `url` opens **System › Statistics**, or the user's statistics in **My account**.

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
| `added` | array | For the server: the titles of these files, those with the most files first, ten at most. Each has the `name` of the title they were matched to, or else the name the files tell, or for a music folder's tracks the title of their album; its `kind`, `movie`, `show` or `album`; and its number of `files` (an album's tracks). Empty for a user. |
| `problems` | array | For the server: the problems **System › Health** found that week, as they were told: `key`, `severity`, `text`, and `foundAt`. Empty for a user. |

### The month and year in review events

`monthly_summary` and `yearly_summary` carry the weekly summary's `summary` object, with one more field.

- `start` and `end` are the month or the year, from its first day at 0:00 in the server's time zone until before the first day of the next. This holds for the tasks that send them by hand too.
- A month in review fills the `summary` as the weekly summary does, over the month.
- A year in review leaves `newEpisodes` at 0 and `episodes` and `problems` empty, and has ten `movies` and ten `series` for the server too.
- A year in review adds `topMonth`.

```json
{
  "version": 1,
  "id": "5f6a7b8c9d0e41f2a3b4c5d6e7f80912",
  "type": "yearly_summary",
  "at": "2026-01-01T08:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "Your year on Home",
  "message": "Year 2025.\n41.5 hours watched, 30 playbacks.\nMost watched month: March, 9 hours.\nMovies: Example Movie.\nSeries: Example Series.",
  "url": "https://media.example.org/admin/me/account#statistics",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "summary": {
    "scope": "user",
    "start": "2024-12-31T23:00:00Z",
    "end": "2025-12-31T23:00:00Z",
    "plays": 30,
    "played": 149400,
    "movies": [{ "id": "0b1c2d3e4f5061728394a5b6c7d8e9f0", "name": "Example Movie", "plays": 2, "played": 7200 }],
    "series": [{ "id": "99887766554433221100ffeeddccbbaa", "name": "Example Series", "plays": 28, "played": 142200 }],
    "newEpisodes": 0,
    "episodes": [],
    "users": [],
    "addedFiles": 0,
    "added": [],
    "problems": [],
    "topMonth": { "month": "2025-03", "plays": 6, "played": 32400 }
  }
}
```

| `summary` field | Type | Meaning |
|---|---|---|
| `topMonth` | object | For `yearly_summary` only: the month with the most hours, as `month`, `YYYY-MM` in the server's time zone, with its `plays` and `played`, in seconds. On a tie, the one with more playbacks, then the earlier. Absent for a week or a month. |

## Messages for each kind

Each kind other than a webhook receives a message made for it. Some events are urgent, and ntfy, Gotify and Pushover show them with a higher priority:

- failed recordings;
- health problems that are errors;
- server crashes.

| Kind | Request | Priority of urgent events |
|---|---|---|
| Discord | One embed, sent as `Polyfin` | none |
| ntfy | A JSON publication to its server's root address | `priority` 4 |
| Email | A `multipart/alternative` message, in plain text and HTML | none |
| Telegram | The Bot API's `sendMessage` | none |
| Gotify | A `POST` to its server's `/message` | `priority` 8, else 5 |
| Pushover | A `POST` to `https://api.pushover.net/1/messages.json` | `priority` 1, else 0 |

### Discord messages

A Discord target receives one embed, sent as `Polyfin`:

- the event's title, its message as the description, and its link;
- the time, and the server name in the footer;
- a color by event, as below.

Messages never mention anyone: `allowed_mentions` is empty.

| Color | Events |
|---|---|
| Blue | New episodes |
| Green | Finished recordings, solved problems |
| Red | Failures, errors |
| Amber | Warnings, stopped connections, sign-ins on new devices |
| Orange | Blocked accounts |
| Teal | Users who joined, playbacks that start or resume |
| Grey | Playbacks that pause or stop |
| Sky blue | New versions |
| Indigo | Weekly summaries, month and year in review |

### ntfy messages

An ntfy target receives a JSON publication to its server's root address, with:

- `topic`, `title` and `message`;
- `tags`, as below;
- a high `priority` (4) for urgent events;
- `click`, the event's link.

The access token, if any, is sent as `Authorization: Bearer`.

| Tag | Events |
|---|---|
| `tv` | New episodes |
| `red_circle` | Finished recordings, with `warning` when partial |
| `x` | Failures |
| `warning` | Problems, stopped connections |
| `white_check_mark` | Solved problems |
| `wave` | Users who joined |
| `package` | New versions |
| `arrow_forward` | Playbacks that start or resume |
| `pause_button`, `stop_button` | Playbacks that pause, or stop |
| `bar_chart` | Weekly summaries, month and year in review |
| `lock` | Blocked accounts |
| `key` | Sign-ins on new devices |
| `bell` | Tests |

### Email messages

An email target receives a `multipart/alternative` message, in plain text and in HTML, in the server language.

- The subject is the event's title.
- The body is its message, its link (named "Open" in HTML), and "Sent by Polyfin from" the server name.

A summary's body is the whole summary instead, for a week, a month or a year:

- the period and its figures;
- a list for each part: the movies and series with how long and how many times they played, the month with the most hours, the new episodes, the users who joined, the files added, the problems found;
- each title links to its page in the web player, each user to their page in the admin app, and each problem to the page that shows it;
- "Open the statistics" links to the statistics.

| Header | Value |
|---|---|
| `From` | The sender name and address. |
| `To` | The target's address. |
| `Subject` | The event's title. |
| `Date` | When the event happened. |
| `Message-ID` | The same for each try of a message, at the sender address's domain. |
| `Auto-Submitted` | `auto-generated`, so that mail servers send no automatic reply. |
| `X-Polyfin-Event` | The event's `type`. |

### Telegram messages

A Telegram target receives the Bot API's `sendMessage`, as `POST https://api.telegram.org/bot<token>/sendMessage`.

- `chat_id` is the target's chat.
- `text` is in HTML (`parse_mode` is `HTML`): the event's title in bold, its message, and its link named "Open".
- The link shows no preview of its page.

### Gotify messages

A Gotify target receives a `POST` to its server's `/message`, with the application token as the `X-Gotify-Key` header.

- `title` and `message` are the event's.
- `priority` is 8 for urgent events and 5 for the others.
- The event's link is the address a click opens (`extras` `client::notification` `click` `url`).

### Pushover messages

A Pushover target receives a `POST` to `https://api.pushover.net/1/messages.json`, with:

- `token`, `user`, `title` and `message`;
- `url`, the event's link;
- `priority`, 1 for urgent events and 0 for the others;
- `timestamp`, when the event happened.

<a id="for-app-developers"></a>

## Admin API for notification targets

The admin API serves the targets, the SMTP settings and the summary previews. Jellyfin apps see none of it: it is not part of Jellyfin's API.

### Target routes

| Route | Who | What it does |
|---|---|---|
| `/admin/api/notifications` | Administrators | The server's targets. |
| `/admin/api/account/notifications` | Each user | Their own targets. |
| `GET` on either | | Lists the `targets`, the `events` and `kinds` they may choose from, and `emailAvailable`, whether the SMTP server and sender address are set. |
| `POST …/targets` | | Adds a target. |
| `PATCH …/targets/{id}` | | Changes one; fields left out keep their values. |
| `DELETE …/targets/{id}` | | Deletes one. |
| `POST …/targets/{id}/test` | | Sends a test message, answering `delivered`, the `status` (an HTTP status, or an SMTP code for email), and the target. |
| `GET …/targets/{id}/deliveries` | | Lists a target's recent deliveries: `id`, `eventId`, `type`, `title`, `events`, `outcome`, `status` (null when the target did not answer), `error` (null when it accepted the message), `attempts`, `queuedAt` and `at`. |

### Target fields

A target answers `kind` (`webhook`, `discord`, `ntfy`, `email`, `telegram`, `gotify` or `pushover`), `name`, `events` and `enabled`, and:

- `address`: a webhook's or Discord target's scheme and host, an ntfy or Gotify target's server, an email target's recipient, empty for Telegram and Pushover;
- `topic`: an ntfy target's topic;
- `chat`: a Telegram target's chat;
- `tokenSet`: whether an ntfy, Telegram, Gotify or Pushover target has a token;
- `perMinute` and `groupMinutes`, null when unset.

It never answers a secret address, token or key.

To add or change a target, send:

- `address`: a webhook's or Discord target's address, an ntfy or Gotify target's server, an email target's recipient;
- `topic` and `chat`;
- `token`: an ntfy target's access token (empty for none), a Telegram bot's token, or a Gotify or Pushover application's token;
- `userKey`: a Pushover target's user key;
- `perMinute` and `groupMinutes`, 0 to unset.

| Error | Status | Cause |
|---|---|---|
| `email_unavailable` | 409 | An email target without an SMTP server. |
| `invalid_email_address`, `invalid_chat`, `invalid_token`, `invalid_user_key` | 400 | A malformed field. |
| `target_unreadable` | 409 | A Pushover target whose key and token cannot be decrypted, changed with only one of them. |
| `invalid_per_minute`, `invalid_group_minutes` | | Pacing out of bounds. |

### Notification settings

`GET /admin/api/settings` answers the SMTP and summary settings, and `PUT` takes them:

| Field | Meaning | Error |
|---|---|---|
| `smtpHost` | **SMTP server** | `invalid_smtp_host` |
| `smtpPort` | **SMTP port** | `invalid_smtp_port` |
| `smtpSecurity` | `starttls`, `tls` or `none` | `invalid_smtp_security` |
| `smtpUser` | **SMTP user** | `invalid_smtp_account` |
| `smtpPasswordSet` | Whether a password is saved | |
| `smtpFrom`, `smtpFromName` | **Sender address** and **Sender name** | `invalid_smtp_sender` |
| `weeklySummaryDay` | The day the weekly summary is sent, 0 for Sunday to 6 for Saturday | `invalid_weekly_summary_day` |
| `weeklySummaryHour` | Its hour, 0 to 23, in the server's time zone | `invalid_weekly_summary_hour` |

`PUT` also takes `smtpPassword`, which is never answered. Left out, it is kept; empty, it is removed.

### Summary preview

`GET /admin/api/notifications/preview?event=weekly_summary&user=<id>`, for administrators, answers a summary as it would be sent now. Nothing is sent.

- `event` is `weekly_summary`, `monthly_summary` or `yearly_summary`, else the error is `invalid_summary`.
- Without `user`, it is the server's summary; with it, that user's.
- It answers `subject`, `text` and `html`, exactly the email's parts, `start` and `end`, and `empty`, true when nothing would be sent.

**Preview**, in **Settings › Notifications › Weekly summary**, shows it in a sandboxed frame.
