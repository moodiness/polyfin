# Statistics

This page is for administrators, and for users curious about their own viewing. Polyfin follows the playbacks Jellyfin apps report, and keeps a history of the videos played.

- **System › Statistics** sums the history for administrators.
- Each user sees their own figures under **My account › Statistics**.
- The same playbacks feed the playback events of [notifications](notifications.md#playbacks).

## Playbacks

Apps report a playback when it starts, as it goes on, and when it stops. The web player reports every ten seconds or so as it goes on, with whether it is paused.

Polyfin turns these reports into playbacks, one per title and device:

| A playback | When |
|---|---|
| Starts | With its first report. |
| Pauses and resumes | As the reports say. |
| Stops | With the stop report, or when the device reports another title. |
| Stops, for an app that closes without a stop report | Five minutes after its last report, ended at that report, as Jellyfin does. |

The time played counts only while the playback is not paused. Between two reports, at most two minutes count, so that an app put to sleep while playing does not count the hours it slept.

A playback keeps how it reached its app:

| How it plays | What it means |
|---|---|
| **Direct play** | The app reads the file as it is. |
| **Remux** | Polyfin repackages it without converting anything. |
| **Conversion** | Polyfin converts the video or the audio. |

- Apps report Polyfin's streams as converted even when Polyfin only repackages them. While the stream runs, Polyfin tells the two apart from what it actually does.
- A playback that was converted for a while counts as converted.

The history keeps movies, episodes, home videos, Live TV channels, recordings and Replay programmes. Songs and audiobooks are not kept: they only send [notifications](notifications.md#playbacks).

## The history

Each video that played for at least 10 seconds is kept once it stops, with:

- the user;
- the item, and what it was called then: its name, plus its series, season and episode numbers for an episode;
- for a channel, a Replay programme or a recording, its channel;
- the app and the device;
- when it started and ended, how long it played, its last position, and how it reached the app;
- the bytes Polyfin sent the app for it (see [Bandwidth](administration.md#bandwidth)).

The bytes sent count from the end of the device's previous playback. So what the app read before it reported the start, its first range requests or segments, counts with it.

- A playback the app read from the source itself, without Polyfin, keeps no bytes sent.
- Neither do the playbacks kept before Polyfin counted them.

Playbacks under way when Polyfin stops are kept, ended at their last report. A deleted user's playbacks are deleted with them.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Keep a playback history** | **Settings › Playback** | on | Turned off, no new playback is kept; those kept stay until they are too old. |
| **Days to keep playback history** | **Settings › Playback** | 365 | Playbacks that started earlier are deleted, from 1 to 3650 days. |

The "Clean the playback history" task, under **System › Schedule**, deletes the old playbacks when Polyfin starts, then every 24 hours.

## The Statistics page

**System › Statistics** shows the history for everyone or for one user, over **7 days**, **30 days**, a **Year** or **All** the history:

| Part | What it shows |
|---|---|
| Totals | **Hours watched**, **Playbacks**, **Users**, and the **Data sent** to the apps. |
| **Hours watched per day** | Each day's column, split by user. For **All**, it becomes **Hours watched per month**, one column per month. |
| **Hours watched per user** | Each user's hours. |
| **Most played movies**, **Most played series**, **Most played channels** | Ten each, by hours watched. A channel counts its live playbacks and its Replay programmes. |
| **Apps**, **Devices** | The apps and devices played on. |
| **How videos play** | The share of **Direct play**, **Remux** (repackaged) and **Conversion**. |
| **Busiest hours** | The hours watched at each hour of the week, in the browser's time zone. Each playback is spread over the hours it lasted. |
| **Recent playbacks** | The history itself, with the data sent for each playback. |

**Export CSV** downloads the playbacks of the period, for everyone or for the user chosen.

The same history feeds three [notifications](notifications.md):

- the [weekly summary](notifications.md#weekly-summary), once a week: the server's hours watched and its five most played movies and series, or a user's own week;
- the [month in review](notifications.md#month-in-review): the same over a month;
- the [year in review](notifications.md#year-in-review): the same over a year, with ten movies and ten series, and the month with the most hours.

## My account

Each user, members too, sees the same figures for their own playbacks under **My account › Statistics**, without the user choice and the parts per user. It shows their recent playbacks, and **Export CSV** downloads their own history.

<a id="compared-with-jellyfin"></a>

**Compared with Jellyfin**

- Jellyfin keeps no playback history of its own: a plugin adds one. Polyfin's is built in, and its API is Polyfin's admin API, not Jellyfin's.

<a id="for-app-developers"></a>

API details: [Statistics and history routes](jellyfin-api.md#statistics-and-history-routes).
