# Tracking services

This page is for every user who connects a tracking account, and for the administrator who sets the services up. Each user connects their own accounts:

| Services | What they hear of |
|---|---|
| Trakt, Simkl, MDBList, PublicMetaDB | The movies and episodes the user watches and marks played |
| AniList, MyAnimeList | The anime the user watches, on the user's anime lists |
| PublicMetaDB, with its watchlist switch on | Also the titles the user adds to or removes from their Watchlist playlist |
| Last.fm, ListenBrainz | The songs the user plays |

It covers what is sent, how users connect and import, and what an administrator sets up.

## What gets sent

Trakt, Simkl, MDBList and PublicMetaDB get only movies and episodes known by an IMDb, TMDB or TVDB identifier. An episode is sent by its series' identifiers, with its season and episode numbers.

PublicMetaDB, Trakt and MDBList number episodes as TMDB does. When TMDB numbers an episode otherwise than the addon, Polyfin sends it to them with TMDB's numbers, so what is imported back marks the same episode. An episode Polyfin cannot place surely among TMDB's is not sent to them. Simkl gets the addon's numbers. See [How Polyfin finds each title](tracking-imports.md#what-is-imported).

- An addon may name an anime by its Kitsu, MyAnimeList or AniDB identifier: `kitsu:<id>`, `mal:<id>`, `anidb:<id>`, and `kitsu:<id>:<episode>` for an episode. Such a title is sent by the identifiers the [anime mapping](#anime) gives:
  - a movie, by its IMDb and TMDB identifiers;
  - an episode, by its series' TVDB and IMDb identifiers, with its season and episode on TVDB.
- An anime the mapping does not know is sent by the identifiers the addon gives, if any.
- Live TV, recordings, audiobooks and titles without an identifier send nothing.
- Songs go to Last.fm and ListenBrainz only, and movies and episodes never reach those two. See [Music: Last.fm and ListenBrainz](#music-last-fm-and-listenbrainz).
- One user's activity never reaches another user's accounts.

### Playback

Trakt, Simkl and MDBList hear as it happens when a title starts, pauses, resumes and stops (scrobbling). Each report says how far into the title playback is. Each service decides from that whether the title was watched, as for its other apps (from 80%).

PublicMetaDB does not follow playback:

- it gets a resume point when playback pauses or stops;
- it gets the title in its history once Polyfin marks it played, past **Marked played after (%)** in **Settings › Content** (see [Played and resume thresholds](watch-state.md#played-and-resume-thresholds)).

### Played marks

Marking a movie, an episode, a season or a series played in an app adds its movies and episodes to each service's history. Marking it unplayed removes them.

- An app that uploads a movie or an episode as played or unplayed through its user data does the same. Apps do this with what they played offline.
- A mark sends only what Polyfin counts as a new play. Marking a played title played again, without a date, sends nothing.
- One viewing makes one history entry. A played mark within 6 hours of a service counting the same title watched from its playback is not sent to that service again.

## Connecting your accounts

Each user connects their accounts in the admin app, under **My account › Tracking**.

| Service | How to connect |
|---|---|
| Trakt, Simkl | **Connect** shows a code to enter on the service's site. |
| Last.fm | **Connect** opens Last.fm. Sign in there if asked, and allow Polyfin. |
| MDBList | Paste your key in **MDBList API key**. It is on mdblist.com, in Preferences. |
| PublicMetaDB | Paste your key in **PublicMetaDB API key**. It is on the PublicMetaDB website, under Settings, then API. |
| ListenBrainz | Paste your token in **ListenBrainz user token**. It is in your settings on the ListenBrainz site. |
| AniList | **Connect** opens AniList. Allow Polyfin, copy the code AniList shows, and paste it in **AniList code**. |
| MyAnimeList | **Connect** opens MyAnimeList in the same tab. Once you allow Polyfin, MyAnimeList sends your browser back to Polyfin, which connects. |

- Polyfin checks an API key or user token with the service before saving it.
- A user's MDBList key serves only their tracking. Title ratings come from the server's own key (see [Ratings](collections-people-and-ratings.md#ratings)).
- Trakt, Simkl, Last.fm, AniList and MyAnimeList are offered only once an administrator has set them up (see [Setting up the services](#setting-up-the-services-administrators)).
- AniList and MyAnimeList have more to know: see [AniList and MyAnimeList](#anilist-and-myanimelist).

Trakt refuses some sign-ins without saying why:

- only the people an administrator added to the server's Trakt app, under its Members, can sign in through it;
- a free Trakt account connects one community app at a time.

When Trakt refuses your sign-in, or the code ends without connecting, **My account › Tracking** names both checks. Ask an administrator to add your Trakt username to the app's Members. On a free account, disconnect the other app on the Trakt website (under Settings, Connected apps), or use Trakt VIP. Trakt's answer does not tell these apart from a wrong app, so the message lists them all.

## Imports and the Watchlist

A connected Trakt, Simkl, MDBList or PublicMetaDB account can also bring your watch history into Polyfin. Trakt's watchlist, Simkl's Plan to Watch and PublicMetaDB's Watchlist can fill your Watchlist playlist, PublicMetaDB's also receiving the titles you add and remove yourself, and Trakt can recommend titles in a home row. See [Tracking imports and the Watchlist](tracking-imports.md).

## Anime

AniDB makes each season of a series, and often each half of a season, a title of its own, with its episodes numbered from 1. Simkl's and Kitsu's anime follow it. Polyfin's titles, like the usual metadata addons', follow the seasons of IMDb and TVDB. Polyfin maps the two with two community lists:

| List | File | What it gives |
|---|---|---|
| [Fribb/anime-lists](https://github.com/Fribb/anime-lists) | `anime-list-full.json` | The AniDB, Kitsu, MyAnimeList, AniList, TVDB, TMDB and IMDb identifiers of each anime |
| [Anime-Lists/anime-lists](https://github.com/Anime-Lists/anime-lists) | `anime-list-master.xml` | Where each AniDB episode is on TVDB: each anime's season and the number to add to its episodes, the episodes it maps one by one, and the series numbered as TVDB's absolute numbering |

The same mapping gives the identifiers to ask stream addons that take only an anime database's, or only IMDb's (see [Identifiers addons accept](addons.md#identifiers-addons-accept)).

### How numbering is translated

An episode goes to its series' TVDB identifier, with its season and episode on TVDB, and to IMDb's when the lists give the same series. Say the lists place an anime at the 13th episode of TVDB's first season: the anime's first episode is that episode of the series.

- Episodes the list maps one by one, or by range, follow it first. The others follow the anime's season and offset.
- An anime numbered as TVDB's absolute numbering is a long series counted from its first episode across its seasons. Polyfin places it among the episodes it listed of the series: its 30th episode is the 30th regular episode, by season then number. A series Polyfin has not listed yet leaves such episodes not found.
- Specials are mapped only where the list names them.
- An anime movie is the movie its IMDb and TMDB identifiers name. One that TVDB lists among a series' specials, without a movie identifier, is that special.
- Simkl's anime are also marked on the titles anime catalogs list under the anime's Kitsu, MyAnimeList or AniDB identifier (`kitsu:<id>`), an episode as the anime numbers it (`kitsu:<id>:<episode>`). These count once with the title found by IMDb and TVDB. Specials are not marked there, and nothing is recorded for an anime no catalog listed.

### What stays unmatched

These count as not found, as other titles do:

- an anime the lists do not know and no anime catalog listed;
- an episode the lists map to nothing;
- a special the lists do not name.

Until the lists are read, Simkl's anime waits for a later import. That import reads all the anime that changed since the last import that read them, or all of it the first time.

A title an addon names by its Kitsu, MyAnimeList or AniDB identifier, that the lists do not know, is sent by the identifiers the addon gives, if any.

### Refreshing the lists

| When | What Polyfin does |
|---|---|
| First start | Downloads both lists from GitHub, and keeps them in `anime` in its data folder |
| Each start | Reads that copy, and downloads nothing while it is less than a day old |
| Once a day | Checks each list again with a conditional request, so a list downloads only when it changed |
| A download fails, or brings something that is not the list | Keeps the copy in use. Tries again after 10 minutes, then less often, up to every 6 hours. |

Nothing waits for a download. Until the first one ends, anime is not mapped.

## AniList and MyAnimeList

AniList and MyAnimeList keep a list of anime for each user, split into seasons and parts as AniDB splits series. Each entry has a status and a count of the episodes watched. The statuses are Watching, Completed, On hold, Dropped and Plan to Watch, which AniList calls Planning.

Polyfin raises that count as you watch, and can import the lists back. Both are offered once an administrator has set up their apps (see [AniList](#anilist) and [MyAnimeList](#myanimelist)).

### Connecting

**AniList:** **Connect** opens AniList. Allow Polyfin, copy the code AniList shows, and paste it in **AniList code**.

- Nothing has to reach the server, as for Last.fm.
- AniList tokens last a year and cannot be refreshed. The status line shows when yours expires.
- Once it has expired, the connection stops and shows **Connect again**.

**MyAnimeList:** **Connect** opens MyAnimeList, with PKCE (MyAnimeList takes its `plain` method only). Once you allow Polyfin, MyAnimeList sends your browser back to the address the administrator registered, a page of the admin app.

- Only your browser has to reach that page.
- Your browser may land on an address it cannot open, such as the server's public address from the local network. Copy that address from the address bar into **Address or code**: Polyfin reads its code.
- A request expires after 15 minutes.
- Tokens are refreshed before they expire.

### Finding the entry

Polyfin goes from a title to its entry through the [anime mapping](#anime), the other way from how it reads Simkl's anime.

| The title | The entry |
|---|---|
| Named by an anime addon with `kitsu:`, `mal:` or `anidb:` | That entry, with the addon's episode number (`kitsu:<id>:<episode>`), even when the mapping places it nowhere on TVDB |
| Named by IMDb or TVDB | The AniDB entry whose mapping covers its TVDB season and episode. The entry's own episode number is sent. |
| A movie | The entry with its IMDb or TMDB identifier, when one entry has it |
| A special | Only where the list names it, and never as an episode of the series' entry |

For an episode named by IMDb or TVDB, Polyfin tries in turn:

1. an episode the list maps one by one;
2. a range;
3. the entry's season and offset: of two entries sharing a season, the one starting nearest before the episode;
4. absolute numbering, from the episode's place among the regular episodes Polyfin lists of its series.

Nothing is sent for a title the mapping does not place, nor for live TV, recordings or songs.

### Progress

Neither service follows playback. A title counts once Polyfin marks it played (**Marked played after (%)**), or once you mark it played, as for PublicMetaDB.

- The entry's progress is raised to that episode, and never lowered: an earlier episode sends nothing.
- An entry not yet on your list, or on Plan to Watch, becomes Watching. It becomes Completed at its last episode, or for a movie.
- An entry you are watching again keeps its status.
- Unplayed marks send nothing: the services keep a count of episodes, not a history.

### Importing the lists

**Import my AniList lists** and **Import my MyAnimeList lists** are off by default. They run on the schedule of the other imports, with **Import now** (see [How often connected accounts are imported](moving-to-polyfin.md#how-often-connected-accounts-are-imported)). Each import reads your whole list.

| Entry status | Episodes marked played |
|---|---|
| Watching, On hold, Dropped | Episodes 1 to the entry's progress |
| Completed | All its episodes, dated with the entry's completion date when it has one |
| Being watched again | All its episodes, as completed, undated |

- Episodes are placed on TVDB through the mapping, as Simkl's anime are, and on the titles anime catalogs list under the entry.
- Until the mapping is read, nothing of the list is imported.
- **Import my AniList Plan to Watch list** and **Import my MyAnimeList Plan to Watch list**, also off, send Plan to Watch (AniList's Planning) to your Watchlist playlist. Its usual rules apply (see [The Watchlist playlist](tracking-imports.md#the-watchlist-playlist)).
- Imports only add, as for the other services (see [How the history merges with Polyfin's](tracking-imports.md#how-the-history-merges-with-polyfin-s)). They write nothing to the services.

### Ratings

**Sync my ratings**, off by default, keeps your ratings and the services' scores together. Polyfin keeps each user's rating from 0 to 10: a like is 10, a dislike 1.

- A rating you give in a Jellyfin app is sent as the entry's score, on a movie or on a series that maps to one entry. AniList gets its raw score (rating × 10), MyAnimeList its score (rounded, 1 to 10).
- Clearing a rating clears the score.
- Only an entry already on your list gets a score: a rating never adds a title to the list.
- Each import gives your unrated titles the service's score. Ratings Polyfin already has stay.
- A series spread over several entries, such as one whose seasons are entries of their own, is neither sent nor imported. The status line counts it as skipped, from the last import and the ratings sent since.

### Disconnecting

**Disconnect** forgets the tokens and stops the imports. What they imported stays.

Neither AniList nor MyAnimeList lets an app revoke a token. To cut Polyfin off entirely, also remove the app from your account on the service: in AniList's settings (Apps), or MyAnimeList's account settings (API).

<a id="music-last-fm-and-listenbrainz"></a><a id="music-lastfm-and-listenbrainz"></a>

## Music: Last.fm and ListenBrainz

The songs users play from music addons go to Last.fm and ListenBrainz: what is playing now, and a scrobble once a song has played long enough. Podcast episodes, which Polyfin serves as songs, go too. Audiobooks do not.

<a id="connecting-last-fm-and-listenbrainz"></a><a id="connecting-lastfm-and-listenbrainz"></a>

### Connecting the music services

Users connect under **My account › Tracking**, as in [Connecting your accounts](#connecting-your-accounts).

- Last.fm: **Connect** shows a button that opens Last.fm. Allow Polyfin to use your account there, and the page updates by itself. The request ends after 15 minutes; **Start again** makes a new one.
- ListenBrainz: the token is on listenbrainz.org, in your settings, under User token. Polyfin asks ListenBrainz whether it is valid before saving it, and shows the account name ListenBrainz gives.
- A connected ListenBrainz token shows as dots, and its owner can show it with its eye. Last.fm gives Polyfin a session key, which is never shown.
- Last.fm is offered once an administrator has set it up (see [Last.fm](#last-fm)).

### What is sent and when

| What | When | Sent |
|---|---|---|
| What is playing now | When a song starts, and when it plays on after a pause | Once: the services show it for a moment only, so it is not worth sending late |
| The scrobble | Once the song has played for half its length or for 4 minutes, whichever comes first | To both services, with the time the song started |

- Songs of 30 seconds or less are never scrobbled, as Last.fm asks.
- A song whose length is unknown is scrobbled after 4 minutes of playing.
- Only the time the song actually played counts:
  - time paused counts for nothing;
  - seeking forward adds nothing, and after seeking back the song counts again only as it plays on;
  - Polyfin takes the smaller of the time between two app reports and how far the song moved meanwhile. A pause the app does not report counts for nothing either.
- A song is scrobbled once per play. Played again from its start, or repeated, it counts anew.
- A song is sent with its artist, title, album, album artist (when not the song's artist), place on its album, and length, as its music addon names them.
- ListenBrainz also gets the song's ISRC, when the addon gives one, and Polyfin's name and version as the player.
- A song whose start Polyfin did not see, as after a server restart, counts from its first report.

**Compared with Jellyfin**

- Polyfin scrobbles songs itself, for each user who connects an account, from the playback reports apps already send. Jellyfin does it only through a plugin installed apart.

API details: [Scrobbles from playback reports](jellyfin-api.md#scrobbles-from-playback-reports).

<a id="setting-up-trakt-simkl-and-last-fm-administrators"></a><a id="setting-up-trakt-simkl-and-lastfm-administrators"></a>

## Setting up the services (administrators)

MDBList, PublicMetaDB and ListenBrainz need nothing from the administrator. Trakt, Simkl, Last.fm, AniList and MyAnimeList are offered once an administrator has registered an app with them, and pasted its credentials under **Settings › Tracking**.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Trakt client ID** | **Settings › Tracking** | Empty | Client ID of your Trakt app. |
| **Trakt client secret** | **Settings › Tracking** | Empty | Optional: only Trakt apps created before October 2026 have one. |
| **Simkl client ID** | **Settings › Tracking** | Empty | Client ID of your Simkl app. |
| **Last.fm API key** | **Settings › Tracking** | Empty | API key of your Last.fm API account. |
| **Last.fm shared secret** | **Settings › Tracking** | Empty | Shared secret of your Last.fm API account. |
| **AniList client ID** | **Settings › Tracking** | Empty | Client ID of your AniList app. |
| **AniList client secret** | **Settings › Tracking** | Empty | Client secret of your AniList app. |
| **MyAnimeList client ID** | **Settings › Tracking** | Empty | Client ID of your MyAnimeList app. |
| **MyAnimeList client secret** | **Settings › Tracking** | Empty | Client secret of your MyAnimeList app. |

Secrets and shared secrets are never sent back with the settings. Once saved, they show as dots, and administrators can show them again with their eye (see [Showing a saved key](administration.md#showing-a-saved-key)).

### Trakt

1. Open [My Apps on Trakt's developer portal](https://developer.trakt.tv/apps). Link your GitHub account, which Trakt asks of every developer.
2. Create an app of the Development type, with any name and the redirect URI `http://localhost`. Trakt keeps Production apps for its paid developer plan.
3. Under the app's Members, add the Trakt username of each Polyfin user who will connect, up to 16. You can always sign in yourself.
4. Paste its Client ID in **Trakt client ID**.
5. Paste its Client Secret in **Trakt client secret** only if Trakt shows one. Apps created since October 2026 have none, and Trakt is offered with the client ID alone.

An app created before October 2026 keeps working as it is, with its client secret and the redirect URI `urn:ietf:wg:oauth:2.0:oob`. Link your GitHub account to it by January 1, 2027, or Trakt stops letting users in.

Free Trakt accounts connect one community app at a time. A user who already connected another one disconnects it on the Trakt website (under Settings, Connected apps), or uses Trakt VIP.

- Polyfin signs users in on `auth.trakt.tv`, the host Trakt's API specification names for its OAuth calls. Trakt requires it from February 1, 2027.
- It calls the rest of Trakt's API on `api.trakt.tv`.
- When it refreshes a user's token, it names the redirect URI the app was registered with, as Trakt's specification asks: `urn:ietf:wg:oauth:2.0:oob` with a client secret, `http://localhost` without. Register exactly `http://localhost` on a new app.
- A saved secret that `POLYFIN_SECRET_KEY` cannot decrypt keeps Trakt off, until the key or the secret is corrected.

### Simkl

1. Register an app in your [Simkl developer settings](https://simkl.com/settings/developer/), of the "TV, devices & command line" type. This is Simkl's AUTH V2, which needs no redirect URI.
2. Paste its client ID in **Simkl client ID**.

Client IDs of older AUTH V1 apps are refused.

<a id="last-fm"></a><a id="lastfm"></a>

### Last.fm

1. Create an [API account on Last.fm](https://www.last.fm/api/account/create). Give it any name, and leave its callback URL empty.
2. Paste the API key and shared secret its page shows in **Last.fm API key** and **Last.fm shared secret**.

Users connect through Last.fm's desktop authentication: they allow Polyfin on Last.fm's page, then Polyfin asks Last.fm for their session itself.

- Nothing comes back to Polyfin through the browser, so no callback address has to reach the server.
- Connecting works the same behind a reverse proxy, at any address, and on a server only reachable on the local network.
- Last.fm's web authentication would send the browser back to an address of Polyfin's, which the server cannot know for sure behind a proxy.

### AniList

1. Create a client in your [AniList developer settings](https://anilist.co/settings/developer), with the redirect URL `https://anilist.co/api/v2/oauth/pin`. AniList then shows users a code to paste in Polyfin.
2. Paste its client ID and secret in **AniList client ID** and **AniList client secret**.

### MyAnimeList

1. Create an app in your [MyAnimeList API settings](https://myanimelist.net/apiconfig), of the "web" type. Its redirect URL is the address **Settings › Tracking** shows under MyAnimeList.
2. Paste its client ID and secret in **MyAnimeList client ID** and **MyAnimeList client secret**.

The redirect URL is the admin app's `/admin/me/account/myanimelist` page, at the server's **Public address** (**Settings › Notifications**). When there is none, it is at the address you use the admin app at. Only users' browsers have to reach it.

<a id="when-the-server-s-app-is-refused"></a><a id="when-the-servers-app-is-refused"></a>

### When the server app is refused

Trakt, Simkl or Last.fm may refuse the server's app, for a wrong client ID, client secret, API key or shared secret. Users connecting then see **App refused**, and are told that an administrator has to check it under **Settings › Tracking**, rather than that the service is down.

Trakt's message also names what users check themselves (see [Connecting your accounts](#connecting-your-accounts)):

- being among the app's Members;
- the one-app limit of a free Trakt account.

## Delivery and retries

Nothing waits for the services: reports and marks are sent in the background, at the pace each service allows.

| Service | Pace |
|---|---|
| Trakt, Simkl, MyAnimeList | One change a second per user |
| MDBList, ListenBrainz | Four a second per user |
| Last.fm | Four requests a second per server address, shared by all users |
| AniList | One request a second per server address, shared by all users |
| PublicMetaDB | Its limit per server address, shared by all users and by [skip segment](skip-segments.md#publicmetadb-key) lookups |

- When a service answers that it gets too many requests, Polyfin waits as long as it asks.
- AniList and MyAnimeList get the progress and scores of entries. Each change is sent after reading the entry, so that progress is never lowered.
- Changes to the history and resume points, scrobbles, and AniList and MyAnimeList changes are kept in the database. They are sent again, with longer waits each time, for up to two days, restarts included.
- A scrobble sent again keeps the time the song started. It is sent once the service takes it, never twice.
- Starts, pauses and what is playing now are sent once.
- A scrobble Last.fm ignores (an artist or a song it ignores, or a time too old) is dropped. One past the account's daily limit is sent again later.

### Connection status

| The service | Status on **My account › Tracking** | What follows |
|---|---|---|
| Refuses the user's token or key | **Needs connecting again**, with **Connect again** | It gets nothing more until the user connects again. What waited for it is dropped. |
| Keeps failing | **Retrying** | Polyfin retries. Nothing is told: the connection has not stopped. |

- Last.fm refuses the session once the user removed Polyfin from the applications of their Last.fm account.
- A refusal is told once, as the connection stops. It goes to the user's notification targets, and the server's, that chose **Connection stopped** (see [Stopped connections](notifications.md#stopped-connections)).
- **System › Health** lists the connections stopped, by user and service.
- Connecting again, then a new refusal, tells again.

### Tokens and keys

| Service | Tokens | On **Disconnect** |
|---|---|---|
| Trakt, Simkl | Refreshed before they expire | Revoked |
| MyAnimeList | Refreshed before they expire | Forgotten only, as it offers no way to revoke them |
| AniList | Last a year and cannot be refreshed. An expired one stops the connection, told once as **Connection stopped**, like a refused one. | Forgotten |
| Last.fm | Session keys do not expire | Forgotten. Removing Polyfin from the applications of the Last.fm account revokes the key. |

- Deleting a user deletes their connections and what waited to be sent. It revokes their Trakt and Simkl tokens in the background, as disconnecting does; their other keys are only forgotten. See [Deleting a user](users.md#deleting-a-user).
- The deletion never waits for a service, and does not fail when one does not answer. A token Polyfin could not revoke is logged, without the token.
- Tokens and keys are never written to the log, nor sent back by the admin app's answers.
- On **My account › Tracking**, a connected MDBList or PublicMetaDB key, or ListenBrainz token, shows as dots. Its owner can show it with its eye (see [Showing a saved key](administration.md#showing-a-saved-key)).
- Trakt, Simkl, AniList and MyAnimeList tokens, and Last.fm session keys, are never shown.
- With `POLYFIN_SECRET_KEY` set, tokens and keys are stored encrypted (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)). A connection whose tokens cannot be decrypted with it counts as not connected, until the user connects again or the right key is set. Nothing is sent with it meanwhile.

<div class="moved-sections">

<a id="importing-your-watch-history"></a>*Importing your watch history* moved to [Tracking imports and the Watchlist](tracking-imports.md#importing-your-watch-history).

<a id="now-available-and-still-waiting"></a>*Now available and Still waiting* moved to [Tracking imports and the Watchlist](tracking-imports.md#now-available-and-still-waiting).

<a id="recommended-on-trakt"></a>*Recommended on Trakt* moved to [Tracking imports and the Watchlist](tracking-imports.md#recommended-on-trakt).

<a id="the-watchlist"></a>*The watchlist* moved to [Tracking imports and the Watchlist](tracking-imports.md#the-watchlist).

</div>
