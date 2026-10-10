# Statistics

Polyfin follows the playbacks Jellyfin apps report and keeps a history of the videos played. **System › Statistics** sums it for administrators, and each user sees their own figures under **My account › Statistics**. The same playbacks feed the playback events of [notifications](notifications.md#playbacks).

## Playbacks

Apps report a playback when it starts, as it goes on (every ten seconds or so for jellyfin-web, with whether it is paused), and when it stops. Polyfin turns these reports into playbacks, one per title and device:

- a playback starts with its first report;
- it pauses and resumes as the reports say;
- it stops with the stop report, when the device reports another title, or, for an app that closes without a stop report, five minutes after its last report, ended at that report, as Jellyfin does.

The time played counts only while the playback is not paused. Between two reports, at most two minutes count, so that an app put to sleep while playing does not count the hours it slept.

A playback keeps how it reached its app: **Direct play**, the app reads the file as it is; **Remux**, Polyfin repackages it without converting anything; **Conversion**, Polyfin converts the video or the audio. Apps report Polyfin's streams as converted even when Polyfin only remuxes them: while the stream runs, Polyfin tells the two apart from what it actually does. A playback that was converted for a while counts as converted.

Movies, episodes, Live TV channels, recordings and Replay programmes are kept in the history. Songs and audiobooks are not: they only send [notifications](notifications.md#playbacks).

## The history

Each video that played for at least 10 seconds is kept once it stops, with:

- the user;
- the item, and what it was called then: its name, its series, season and episode numbers for an episode, its channel for a channel, a Replay programme or a recording;
- the app and the device;
- when it started and ended, how long it played, its last position, and how it reached the app.

Playbacks under way when Polyfin stops are kept, ended at their last report. A deleted user's playbacks are deleted with them.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Keep a playback history** | **Settings › Playback** | on | Turned off, no new playback is kept; those kept stay until they are too old. |
| **Days to keep playback history** | **Settings › Playback** | 365 | Playbacks that started earlier are deleted, from 1 to 3650 days. |

The "Clean the playback history" task, under **System › Schedule**, deletes the old playbacks every day.

## The Statistics page

**System › Statistics** shows, for **7 days**, **30 days**, a **Year** or **All** the history, for everyone or one user:

- the hours watched, the playbacks and the users;
- **Hours watched per day**, each day's column split by user (per month for all the history);
- **Hours watched per user**;
- the **Most played movies**, **Most played series** and **Most played channels**, ten each, by hours watched (a channel counts its live playbacks and its Replay programmes);
- the **Apps** and **Devices**;
- **How videos play**: the share of direct play, remux and conversion;
- the **Busiest hours**: the hours watched at each hour of the week, each playback spread over the hours it lasted, in the browser's time zone;
- **Recent playbacks**, the history itself.

**Export CSV** downloads the playbacks of the period, for everyone or the user chosen.

The same history feeds the [weekly summary](notifications.md#weekly-summary) notification: once a week, the server's hours watched and its five most played movies and series, and each user's own week.

## My account

Each user, members too, sees the same figures for their own playbacks under **My account › Statistics**, with their recent playbacks and **Export CSV** for their own history.

## For app developers

The admin API serves the statistics, the history and the CSV, everyone's to administrators and one's own to every user:

| Everyone's (administrators) | One's own |
|---|---|
| `GET /admin/api/statistics` | `GET /admin/api/account/statistics` |
| `GET /admin/api/history` | `GET /admin/api/account/history` |
| `GET /admin/api/history/export` | `GET /admin/api/account/history/export` |

- Every route takes `period`: `7d`, `30d` (the default), `year` or `all`. The administrators' routes also take `user`, a user's ID, to keep that user's playbacks; the own routes ignore it.
- The statistics take `timeZone`, an IANA name such as `Europe/Paris`, UTC by default, which days, months and hours of the week are counted in. They answer `period`, `since` (null for all), `unit` (`day` or `month`), `plays`, `played`, and:
  - `users`: `id`, `name`, `plays`, `played`;
  - `buckets`: each day or month with playbacks, `start` as `YYYY-MM-DD`, `played`, and `users` (`id`, `played`);
  - `movies`, `series`, `channels`: `id`, the latest `name`, `plays`, `users` (how many played it), `played`;
  - `apps` (`name`), `devices` (`name`, `app`) and `methods` (`method`: `direct_play`, `direct_stream`, `conversion`, or empty when no report told), each with `plays` and `played`;
  - `hours`: 168 numbers, the seconds watched in each hour of the week from Monday 0:00;
  - `historyEnabled` and `historyDays`, the settings.

  Times are in seconds, pauses left out.
- The history takes `limit` (1 to 100, 20 by default) and `start`, and answers `items`, the latest first, and `total`. Each item has `id`, `user` (`id`, `name`), `item` (`id`, `kind`: `movie`, `episode`, `channel`, `recording` or `replay`, `name`, and `seriesName`, `season`, `episode`, `channelName`, null when they do not apply), `app`, `device`, `startedAt`, `endedAt`, `played`, `position` and `method`.
- The export answers `text/csv` with a header line: `started_at`, `ended_at` (RFC 3339, UTC), `user`, `kind`, `title`, `series`, `season`, `episode`, `channel`, `app`, `device`, `played_seconds`, `position_seconds`, `method`. A name that a spreadsheet would read as a formula starts with an apostrophe.
- Errors: `invalid_period`, `invalid_user`, `invalid_time_zone`, `invalid_limit` (400). A member asking for everyone's gets 403.
- `GET /admin/api/settings` answers `playbackHistory` and `playbackHistoryDays`, which `PUT` takes; a number of days out of bounds is `invalid_playback_history_days`.

## Compared with Jellyfin

- Jellyfin keeps no playback history of its own: a plugin adds one. Polyfin's is built in, and its API is Polyfin's admin API, not Jellyfin's.
