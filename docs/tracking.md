# Tracking services

Each user can connect their own Trakt, Simkl, MDBList and PublicMetaDB accounts. These services then hear of the movies and episodes the user watches and marks played. This page covers what is sent, how users connect, and what an administrator sets up.

## What gets sent

Only movies and episodes known by an IMDb, TMDB or TVDB identifier are sent. Episodes are sent by their series' identifiers and their season and episode numbers.

- Live TV, recordings, music and titles without an identifier send nothing.
- One user's activity never reaches another user's accounts.

### Playback

Starting, pausing, resuming and stopping a title reach Trakt, Simkl and MDBList as they happen (scrobbling), with how far into the title playback is. Each service decides from that whether the title was watched, as for its other apps (from 80%).

PublicMetaDB does not follow playback. It gets a resume point when playback pauses or stops, and the title in its history once Polyfin marks it played (**Marked played after (%)** under **Settings › Content**; see [Played and resume thresholds](users.md#played-and-resume-thresholds)).

### Played marks

Marking a movie, an episode, a season or a series played or unplayed from an app adds its movies and episodes to the history of each service, or removes them. An app that uploads a movie or an episode as played or unplayed through its user data does the same, as apps do with what they played offline.

- A mark sends only what Polyfin counts as a new play. Marking played again what already is played, without a date, sends nothing.
- One viewing makes one history entry. A played mark within six hours of a service counting the same title watched from its playback is not sent to that service again.

## Connecting your accounts

Each user connects their accounts under **My account › Tracking** in the admin app.

- **Trakt** and **Simkl** show a code to enter on their site.
- **MDBList** takes the user's API key, from MDBList's preferences.
- **PublicMetaDB** takes the user's API key, from **Settings › API** on its site.

Polyfin checks an API key with the service before saving it.

Trakt and Simkl are offered only once an administrator has set them up; see [Setting up Trakt and Simkl](#setting-up-trakt-and-simkl-administrators).

## Importing your watch history

Polyfin can also read what you watched elsewhere, in other apps that report to the same services, so that played marks, **Continue Watching** and **Next Up** match. It is off by default. Under **My account › Tracking**, each connected service has:

- **Import my … history**: turning it on imports at once, then every 6 hours.
- **Import now**: imports at once.
- A status line: when the last import ran, how many titles it marked played, how many resume points it set, how many titles of the history Polyfin could not find, and what stopped it, if anything.

Importing only adds to your data in Polyfin. It never sends anything to any service, and one user's history never reaches another user.

### What is imported

| Service | Watched movies and episodes | Resume points |
|---|---|---|
| Trakt | Its watch history, a page of 250 plays at a time, read again only when its last activities changed. | Its playback progress. |
| Simkl | Its watched movies and the watched episodes of its shows, one list status at a time, only what changed since the last import. Anime is left out: Simkl numbers it as AniDB does. | Its paused playbacks. |
| MDBList | Its watched movies and episodes, 1,000 at a time, only what changed since the last import. Whole shows or seasons marked watched without their episodes are left out. | Its paused playbacks. |
| PublicMetaDB | Its watch history, 500 plays at a time, whole every time. | Its resume points. |

Polyfin finds the titles by their IMDb identifier, the way the usual metadata addons name them: a movie `tt…`, an episode `tt…:<season>:<episode>`. A title that a catalog listed under its TMDB or TVDB identifier is found that way too. PublicMetaDB names titles by TMDB only: those Polyfin does not know by it are looked up by the IMDb identifier PublicMetaDB maps them to, when its contributors agree on one. Titles found no way are counted as not found, and the import goes on.

### How the history merges with Polyfin's

- **Played marks**: an import only adds them, never removes one. A title gets the service's date unless Polyfin's is later, and is played at least once; a title already played keeps its play count.
- **Resume points**: only for titles not played. Of Polyfin's resume point and the service's, the more recent one stays. A position past **Marked played after (%)** marks the title played; one before **Resume point kept after (%)** is ignored (see [Played and resume thresholds](users.md#played-and-resume-thresholds)).
- **Several services**: a title played on any of them is played, and the most recent resume point wins.
- Apps see the changes the next time they refresh: an import does not push them through Jellyfin's live connection, which would ask the addons for every title it changed.

### Pace and problems

- Each service is read at its pace (one page a second for Trakt, two for Simkl and MDBList, PublicMetaDB's limit per server address), waiting as long as it asks, up to 15 minutes.
- One import runs at a time per user and service. An import interrupted by a restart runs again later.
- When the history cannot be read whole, what was read is imported, and the status line says why: the service refused the connection (**Connect again**), could not be reached, or asked to wait too long.
- Disconnecting a service stops its imports and turns them off. What they imported stays in Polyfin.

## Setting up Trakt and Simkl (administrators)

MDBList and PublicMetaDB need nothing from the administrator. Trakt and Simkl are offered once an administrator has registered an app with them and pasted its credentials under **Settings › Tracking**.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Trakt client ID** | **Settings › Tracking** | Empty | Client ID of your Trakt app. |
| **Trakt client secret** | **Settings › Tracking** | Empty | Client secret of your Trakt app. Never shown again once saved. |
| **Simkl client ID** | **Settings › Tracking** | Empty | Client ID of your Simkl app. |

### Trakt

1. Create an application in your [Trakt API apps](https://app.trakt.tv/settings/apps), with the redirect URI `urn:ietf:wg:oauth:2.0:oob`.
2. Paste its **Client ID** and **Client Secret** in **Trakt client ID** and **Trakt client secret**.

The secret is never shown again once saved.

### Simkl

1. Register an app in your [Simkl developer settings](https://simkl.com/settings/developer/), of the **TV, devices & command line** type. This is Simkl's AUTH V2, which needs no redirect URI.
2. Paste its client ID in **Simkl client ID**.

Client IDs of older AUTH V1 apps are refused.

### When the server's app is refused

When Trakt or Simkl refuses the server's app (a wrong client ID or secret), users connecting are told that an administrator has to check it under **Settings › Tracking**, rather than that the service is down.

## Delivery and retries

Nothing waits for the services: reports and marks are sent in the background.

- They are sent at the pace each service allows: one change a second per user for Trakt and Simkl, and PublicMetaDB's limit per server address.
- When a service answers that it gets too many requests, Polyfin waits as long as it asks.
- Changes to the history and resume points are kept in the database and sent again, with longer waits each time, for up to two days, restarts included.
- Starts and pauses are sent once.

### Connection status

- A service that refuses the user's token or key shows **Connect again**. It gets nothing more until the user connects it again.
- A service that keeps failing shows as unreachable while Polyfin retries.

### Tokens and keys

- Trakt and Simkl tokens are refreshed before they expire, and revoked when the user disconnects.
- Deleting a user deletes their connections and what waited to be sent.
- Tokens and keys are never shown, nor written to the log.
