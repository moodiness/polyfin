# Statistics

Polyfin follows the playbacks Jellyfin apps report and keeps a history of the videos played. **System › Statistics** sums it for administrators, and each user sees their own figures under **My account › Statistics**. The same playbacks feed the playback events of [notifications](notifications.md#playbacks).

## Playbacks

Apps report a playback when it starts, as it goes on (every ten seconds or so for jellyfin-web, with whether it is paused), and when it stops. Polyfin turns these reports into playbacks, one per title and device:

- a playback starts with its first report;
- it pauses and resumes as the reports say;
- it stops with the stop report, when the device reports another title, or, for an app that closes without a stop report, five minutes after its last report, ended at that report, as Jellyfin does.

The time played counts only while the playback is not paused. Between two reports, at most two minutes count, so that an app put to sleep while playing does not count the hours it slept.

A playback keeps how it reached its app: **Direct play**, the app reads the file as it is; **Remux**, Polyfin repackages it without converting anything; **Conversion**, Polyfin converts the video or the audio. Apps report Polyfin's streams as converted even when Polyfin only repackages them: while the stream runs, Polyfin tells the two apart from what it actually does. A playback that was converted for a while counts as converted.

Movies, episodes, Live TV channels, recordings and Replay programmes are kept in the history. Songs and audiobooks are not: they only send [notifications](notifications.md#playbacks).

## The history

Each video that played for at least 10 seconds is kept once it stops, with:

- the user;
- the item, and what it was called then: its name, its series, season and episode numbers for an episode, its channel for a channel, a Replay programme or a recording;
- the app and the device;
- when it started and ended, how long it played, its last position, and how it reached the app;
- the bytes Polyfin sent the app for it (see [Bandwidth](administration.md#bandwidth)).

The bytes sent count from the end of the device's previous playback, so that what the app read before it reported the start, its first range requests or segments, count with it. A playback the app read from the source itself, without Polyfin, keeps none; so do the playbacks kept before Polyfin counted them.

Playbacks under way when Polyfin stops are kept, ended at their last report. A deleted user's playbacks are deleted with them.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Keep a playback history** | **Settings › Playback** | on | Turned off, no new playback is kept; those kept stay until they are too old. |
| **Days to keep playback history** | **Settings › Playback** | 365 | Playbacks that started earlier are deleted, from 1 to 3650 days. |

The "Clean the playback history" task, under **System › Schedule**, deletes the old playbacks every day.

## The Statistics page

**System › Statistics** shows, for **7 days**, **30 days**, a **Year** or **All** the history, for everyone or one user:

- the hours watched, the playbacks, the users, and the **Data sent** to the apps;
- **Hours watched per day**, each day's column split by user (per month for all the history);
- **Hours watched per user**;
- the **Most played movies**, **Most played series** and **Most played channels**, ten each, by hours watched (a channel counts its live playbacks and its Replay programmes);
- the **Apps** and **Devices**;
- **How videos play**: the share of **Direct play**, **Remux** (repackaged) and **Conversion**;
- the **Busiest hours**: the hours watched at each hour of the week, each playback spread over the hours it lasted, in the browser's time zone;
- **Recent playbacks**, the history itself, with the data sent for each.

**Export CSV** downloads the playbacks of the period, for everyone or the user chosen.

The same history feeds the [weekly summary](notifications.md#weekly-summary) notification: once a week, the server's hours watched and its five most played movies and series, and each user's own week; and the [month in review](notifications.md#month-in-review) and the [year in review](notifications.md#year-in-review): the same over a month, and over a year with its month with the most hours.

## My account

Each user, members too, sees the same figures for their own playbacks under **My account › Statistics**, with their recent playbacks and **Export CSV** for their own history.

<a id="compared-with-jellyfin"></a>

**Compared with Jellyfin**

- Jellyfin keeps no playback history of its own: a plugin adds one. Polyfin's is built in, and its API is Polyfin's admin API, not Jellyfin's.

<a id="for-app-developers"></a>

API details: [Statistics and history routes](jellyfin-api.md#statistics-and-history-routes).
