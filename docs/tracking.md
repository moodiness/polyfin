# Tracking services

Each user can connect their own Trakt, Simkl, MDBList and PublicMetaDB accounts, which hear of the movies and episodes the user watches and marks played, and their own Last.fm and ListenBrainz accounts, which hear of the songs they play. This page covers what is sent, how users connect, and what an administrator sets up.

## What gets sent

Only movies and episodes known by an IMDb, TMDB or TVDB identifier are sent to Trakt, Simkl, MDBList and PublicMetaDB. Episodes are sent by their series' identifiers and their season and episode numbers.

- Anime that an addon names by its Kitsu, MyAnimeList or AniDB identifier (`kitsu:<id>`, `mal:<id>`, `anidb:<id>`, and `kitsu:<id>:<episode>` for an episode) are sent by the identifiers the [anime mapping](#anime) gives: a movie by its IMDb and TMDB identifiers, an episode by its series' TVDB and IMDb identifiers and its season and episode on TVDB. Those the mapping does not know are sent by the identifiers the addon gives, if any.
- Live TV, recordings, audiobooks and titles without an identifier send nothing.
- Songs go to Last.fm and ListenBrainz only, and movies and episodes never reach those two. See [Music: Last.fm and ListenBrainz](#music-last-fm-and-listenbrainz).
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

- **Trakt** and **Simkl** show a code to enter on their site, after **Connect**.
- **Last.fm** opens its site after **Connect**: sign in there if asked, and allow Polyfin.
- **MDBList** takes the user's API key (**MDBList API key**), from MDBList's preferences.
- **PublicMetaDB** takes the user's API key (**PublicMetaDB API key**), from **Settings → API** on its site.
- For ListenBrainz, the user pastes their token in **ListenBrainz user token**, from the settings of its site.

Polyfin checks an API key or user token with the service before saving it.

Trakt, Simkl and Last.fm are offered only once an administrator has set them up; see [Setting up Trakt, Simkl and Last.fm](#setting-up-trakt-simkl-and-last-fm-administrators).

## Importing your watch history

Polyfin can also read what you watched elsewhere, in other apps that report to the same services, so that played marks, **Continue Watching** and **Next Up** match. It is off by default. Under **My account › Tracking**, each connected service has:

- The **Import my … history** switch (such as **Import my Trakt history**): turning it on imports at once, then every 6 hours.
- **Import now**: imports at once.
- A status line: when the last import ran, how many titles it marked played, how many resume points it set, how many titles of the history Polyfin could not find, and what stopped it, if anything.

Importing only adds to your data in Polyfin. It never sends anything to any service, and one user's history never reaches another user. Last.fm and ListenBrainz have no import: Polyfin only sends them what you play. An administrator moving from a Jellyfin, Emby or Plex server imports its users' watch data the same way (see [Moving from Jellyfin](users.md#moving-from-jellyfin) and [Moving from Emby and Plex](users.md#moving-from-emby-and-plex)), and you can import your own from a Jellyfin or Emby server you used before (see [Importing your own watch history](users.md#importing-your-own-watch-history)).

### What is imported

| Service | Watched movies and episodes | Resume points |
|---|---|---|
| Trakt | Its watch history, a page of 250 plays at a time, read again only when its last activities changed. | Its playback progress. |
| Simkl | Its watched movies and the watched episodes of its shows and anime, one list status at a time, only what changed since the last import. Anime, which Simkl numbers as AniDB does, is translated to the seasons and episodes of IMDb and TVDB through the [anime mapping](#anime). | Its paused playbacks, anime included. |
| MDBList | Its watched movies and episodes, 1,000 at a time, only what changed since the last import. Whole shows or seasons marked watched without their episodes are left out. | Its paused playbacks. |
| PublicMetaDB | Its watch history, 500 plays at a time, whole every time. | Its resume points. |

Polyfin finds the titles by their IMDb identifier, the way the usual metadata addons name them: a movie `tt…`, an episode `tt…:<season>:<episode>`. A title that a catalog listed under its TMDB or TVDB identifier is found that way too. A title no catalog listed yet shows in your lists at once, described by the addons. PublicMetaDB names titles by TMDB only: those Polyfin does not know by it are looked up by the IMDb identifier PublicMetaDB maps them to, when its contributors agree on one. Simkl's anime are found by the identifiers and numbers the [anime mapping](#anime) gives them. Titles found no way are counted as not found, and the import goes on.

### How the history merges with Polyfin's

- **Played marks**: an import only adds them, never removes one. A title gets the service's date unless Polyfin's is later, and is played at least once; a title already played keeps its play count.
- **Resume points**: only for titles not played. Of Polyfin's resume point and the service's, the more recent one stays. A position past **Marked played after (%)** marks the title played; one before **Resume point kept after (%)** is ignored (see [Played and resume thresholds](users.md#played-and-resume-thresholds)).
- **Several services**: a title played on any of them is played, and the most recent resume point wins.
- Apps see the changes the next time they refresh: an import does not push them through Jellyfin's live connection, which would ask the addons for every title it changed.

### Pace and problems

- Each service is read at its pace (one page a second for Trakt, two for Simkl and MDBList, PublicMetaDB's limit per server address), waiting as long as it asks, up to 15 minutes.
- One import runs at a time per user and service. An import interrupted by a restart runs again later.
- When the history cannot be read whole, what was read is imported, and the status line says why: the service refused the connection (**Connect again**), could not be reached, or asked to wait too long.
- **Disconnect** asks first. Disconnecting a service stops its imports and turns them off. What they imported stays in Polyfin.

## Anime

AniDB, and Simkl's and Kitsu's anime that follow it, make each season of a series, and often each half of a season, a title of its own, with its episodes numbered from 1. Polyfin's titles, like the usual metadata addons', follow the seasons of IMDb and TVDB. Polyfin maps the two with two community lists:

- [Fribb/anime-lists](https://github.com/Fribb/anime-lists) (`anime-list-full.json`) gives the AniDB, Kitsu, MyAnimeList, AniList, TVDB, TMDB and IMDb identifiers of each anime.
- [Anime-Lists/anime-lists](https://github.com/Anime-Lists/anime-lists) (`anime-list-master.xml`) gives where each AniDB episode is on TVDB: the season of each anime and the number to add to its episodes, the episodes it maps one by one, and the series numbered as TVDB's absolute numbering.

### How numbering is translated

- An episode goes to its series' TVDB identifier, and IMDb's when the lists give the same series, with the season and episode on TVDB. For example, the first episode of an anime the lists place at the 13th episode of TVDB's first season is that episode of the series.
- Episodes the list maps one by one, or by range, follow it first; the others follow the anime's season and offset.
- An anime numbered as TVDB's absolute numbering, a long series counted from its first episode across its seasons, is placed among the episodes Polyfin listed of the series: its 30th episode is the 30th of the series' regular episodes, by season then number. A series Polyfin has not listed yet leaves such episodes not found.
- Specials are mapped only where the list names them.
- An anime movie is the movie its IMDb and TMDB identifiers name. One that TVDB lists among a series' specials, without a movie identifier, is that special.
- Simkl's anime are also marked on the titles anime catalogs list under the anime's Kitsu, MyAnimeList or AniDB identifier (`kitsu:<id>`), an episode as the anime numbers it (`kitsu:<id>:<episode>`), when a catalog listed them: these count once with the title found by IMDb and TVDB. Specials are not marked there, and nothing is recorded for an anime no catalog listed.

### What stays unmatched

An anime the lists do not know and no anime catalog listed, an episode they map to nothing, and a special they do not name are counted as not found, as other titles are. Until the lists are read, Simkl's anime is left for a later import, which then reads all the anime that changed since the last one that read them, or all of it the first time. A title an addon names by its Kitsu, MyAnimeList or AniDB identifier that the lists do not know is sent by the identifiers the addon gives, if any.

### Refreshing the lists

Polyfin downloads both lists from GitHub when it first starts, keeps them in `anime` in its data folder, and reads that copy at each start, downloading nothing while it is less than a day old. It checks each list again once a day, with a conditional request, so that a list downloads only when it changed. When a download fails, or brings something that is not the list, the copy in use stays, and Polyfin tries again after 10 minutes, then less often, up to every 6 hours. Nothing waits for a download: until the first one ends, anime is not mapped.

## Music: Last.fm and ListenBrainz

The songs users play from music addons go to Last.fm and ListenBrainz: what is playing now, and a scrobble once a song has played long enough. Podcast episodes, which Polyfin serves as songs, go too. Audiobooks do not.

### Connecting Last.fm and ListenBrainz

Under **My account › Tracking**:

- Last.fm: **Connect** shows a button that opens Last.fm. Sign in there if asked, and allow Polyfin to use your account. The page updates by itself once you have. The request ends after 15 minutes; **Start again** makes a new one.
- ListenBrainz: paste your token in **ListenBrainz user token**. You find it on listenbrainz.org, in your settings, under User token. Polyfin asks ListenBrainz whether the token is valid before saving it, and shows the account name ListenBrainz gives.

A connected ListenBrainz token shows as dots, and its owner can show it with its eye. Last.fm gives Polyfin a session key, which is never shown. Last.fm is offered once an administrator has set it up; see [Last.fm](#last-fm).

### What is sent and when

- What is playing now: sent when a song starts, and when it plays on after a pause. It is sent once: Last.fm and ListenBrainz show it for a moment only, so it is not worth sending late.
- The scrobble: sent once the song has played for half its length or for 4 minutes, whichever comes first. Last.fm and ListenBrainz both get it, with the time the song started.
- Songs of 30 seconds or less are never scrobbled, as Last.fm asks. A song whose length is unknown is scrobbled after 4 minutes of playing.
- Only the time the song actually played counts. Time paused counts for nothing, and so do skips: seeking forward adds nothing, and after seeking back the song counts again only as it plays on. Polyfin takes the smaller of the time between two reports from the app and how far the song moved meanwhile, so a pause the app does not report counts for nothing either.
- A song is scrobbled once per play. Played again from its start, or repeated, it counts anew.
- A song is sent with its artist, title, album, album artist (when not the song's artist), place on its album, and length, as its music addon names them. ListenBrainz also gets its ISRC, when the addon gives one, and Polyfin's name and version as the player.
- A song whose start Polyfin did not see, as after a server restart, counts from its first report.

**Compared with Jellyfin:** Jellyfin sends songs to Last.fm or ListenBrainz only through a plugin installed apart. Polyfin does it itself, for each user who connects an account, from the playback reports apps already send.

**For app developers:** there is nothing to add. Polyfin follows the reports apps send for songs: `POST /Sessions/Playing`, `/Sessions/Playing/Progress` and `/Sessions/Playing/Stopped` (and the older `/PlayingItems` ones). Send `PositionTicks` and `IsPaused` as playback goes, every few seconds as Jellyfin's apps do, and a new start for each play of a song, repeats included. The scrobble goes out with the first report past the threshold, or the stop.

## Setting up Trakt, Simkl and Last.fm (administrators)

MDBList, PublicMetaDB and ListenBrainz need nothing from the administrator. Trakt, Simkl and Last.fm are offered once an administrator has registered an app with them and pasted its credentials under **Settings › Tracking**.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Trakt client ID** | **Settings › Tracking** | Empty | Client ID of your Trakt app. |
| **Trakt client secret** | **Settings › Tracking** | Empty | Client secret of your Trakt app. Shown as dots once saved; administrators can show it again with its eye. |
| **Simkl client ID** | **Settings › Tracking** | Empty | Client ID of your Simkl app. |
| **Last.fm API key** | **Settings › Tracking** | Empty | API key of your Last.fm API account. |
| **Last.fm shared secret** | **Settings › Tracking** | Empty | Shared secret of your Last.fm API account. Shown as dots once saved; administrators can show it again with its eye. |

### Trakt

1. Create an application in your [Trakt API apps](https://app.trakt.tv/settings/apps), with the redirect URI `urn:ietf:wg:oauth:2.0:oob`.
2. Paste its **Client ID** and **Client Secret** in **Trakt client ID** and **Trakt client secret**.

The secret is never sent back with the settings. Once saved, it shows as dots, and its eye shows it again on demand (see [Showing a saved key](administration.md#showing-a-saved-key)).

### Simkl

1. Register an app in your [Simkl developer settings](https://simkl.com/settings/developer/), of the **TV, devices & command line** type. This is Simkl's AUTH V2, which needs no redirect URI.
2. Paste its client ID in **Simkl client ID**.

Client IDs of older AUTH V1 apps are refused.

### Last.fm

1. Create an [API account on Last.fm](https://www.last.fm/api/account/create). Give it any name, and leave its callback URL empty.
2. Paste its API key and shared secret, shown on its page, in **Last.fm API key** and **Last.fm shared secret**.

The shared secret is kept as the Trakt client secret is: never sent back with the settings, shown as dots once saved, and shown again on demand with its eye.

Users connect through Last.fm's desktop authentication: they allow Polyfin on Last.fm's page, then Polyfin asks Last.fm for their session itself. Nothing comes back to Polyfin through the browser, so no callback address has to reach the server. Connecting works the same behind a reverse proxy, at any address, and on a server only reachable on the local network. Last.fm's web authentication would send the browser back to an address of Polyfin's, which the server cannot know for sure behind a proxy.

### When the server's app is refused

When Trakt, Simkl or Last.fm refuses the server's app (a wrong client ID, client secret, API key or shared secret), users connecting are told that an administrator has to check it under **Settings › Tracking**, rather than that the service is down.

## Delivery and retries

Nothing waits for the services: reports and marks are sent in the background.

- They are sent at the pace each service allows: one change a second per user for Trakt and Simkl, four a second per user for MDBList and ListenBrainz, and the limits per server address of PublicMetaDB and Last.fm (four requests a second for Last.fm, shared by all users).
- When a service answers that it gets too many requests, Polyfin waits as long as it asks.
- Changes to the history and resume points, and scrobbles, are kept in the database and sent again, with longer waits each time, for up to two days, restarts included. A scrobble sent again keeps the time the song started, and is sent once the service takes it, never twice.
- Starts, pauses and what is playing now are sent once.
- A scrobble Last.fm ignores (an artist or a song it ignores, or a time too old) is dropped. One past the account's daily limit is sent again later.

### Connection status

- A service that refuses the user's token or key shows **Connect again**. It gets nothing more until the user connects it again, and what waited for it is dropped. Last.fm refuses the session once the user removed Polyfin from the applications of their Last.fm account.
- A service that keeps failing shows as unreachable while Polyfin retries.

### Tokens and keys

- Trakt and Simkl tokens are refreshed before they expire, and revoked when the user disconnects. Last.fm session keys do not expire; disconnecting forgets the key, and removing Polyfin from the applications of the Last.fm account revokes it.
- Deleting a user deletes their connections and what waited to be sent.
- Tokens and keys are never written to the log, nor sent back by the admin app's answers. On **My account › Tracking**, a connected MDBList or PublicMetaDB key or ListenBrainz token shows as dots, and its owner can show it with its eye; Trakt and Simkl tokens and Last.fm session keys are never shown. See [Showing a saved key](administration.md#showing-a-saved-key).
- With `POLYFIN_SECRET_KEY` set, tokens and keys are stored encrypted. A connection whose tokens cannot be decrypted with it counts as not connected until the user connects again or the right key is set; nothing is sent with it meanwhile. See [Stored keys and tokens](configuration.md#stored-keys-and-tokens).
