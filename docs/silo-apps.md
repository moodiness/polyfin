# Silo's apps

Silo's native apps, for iPhone, iPad, Apple TV and Mac, and for Android phones and TVs, can use Polyfin as their server. They speak Silo's own API, under `/api/v2`, not Jellyfin's: Polyfin answers the part of it they need to sign in, browse and play, while the setting allows them.

## Turning it on

**Settings › General › Allow Silo's apps**, off by default. While it is off, Polyfin answers nothing under `/api/v2`: the apps report that they cannot reach a Silo server there.

In the app, add the server by the address Jellyfin apps use, such as `http://192.168.1.10:8096`, and sign in with a Polyfin user's name and password. Behind a reverse proxy, `/api/v2` must reach Polyfin like the rest; a base path such as `/jellyfin` is for Jellyfin apps only.

## How it works

Each answer of Silo's API is made of the Jellyfin API's answer for the same user and device. What Silo's apps see and play is what Jellyfin apps get, under the same limits:

- parental control, blocked genres, the libraries the user may see or hid, and the titles they hid;
- allowed hours, disabled and ended accounts: outside their hours, a user's apps are refused, except an administrator's, but may still sign out;
- version rules, quality groups, the user's bitrate limit and conversion permissions;
- the sign-in throttle, and two-step sign-in.

A signed-in app is a device of the user, listed with the others and signed out the same ways. Its playbacks show in the dashboard and the statistics, and its played state and resume points are the user's, shared with Jellyfin apps.

When a source of the server fails to answer, the app is told to try again later (503) rather than that the server failed. The media of a playback (its file, playlists, segments and subtitles) are fetched with the app's token and are never kept by a cache. Artwork needs no token, as Jellyfin's images do not. Polyfin reads the bodies the apps send as JSON whatever their `Content-Type`.

## What works

- **Connecting:** the server's information, identity and setup state.
- **Signing in** with a name and password, staying signed in, and signing out.
- **Profiles:** each Polyfin user is one profile, named after them, without a PIN.
- **Home:** Continue Watching, Next Up, the home rows the administrator adds for the web player at their places, collection rows among them, and a row of each library, in the order the user set for their libraries, without those they left out of their latest titles: its latest titles, or for a library of collections the first of its titles, titled with its name.
- **Libraries:** those the user sees in Jellyfin apps, in the order they set, without those they left out of My Media, whether the web player's menus show them or not. Libraries of movies, of series, and of both, libraries of collections among them: an addon's catalog of collections, and Polyfin's Collections view, the same whether or not Silo's apps are ticked under **Settings › Catalogs**, in **Apps that show collection libraries by collection**. A library opens on its titles, page by page, sorted by title, year, release date, date added, rating, runtime or at random, or narrowed to its favorites or a search; a library of collections on the titles of its collections, each once, in its order. Its first page shows what the user is watching in it and its latest titles, or, for a library of collections, a row of each of its first 24 collections.
- **Collections**, read-only: a library's Collections tab lists the collections with a title in it, as the Jellyfin API lists a library's collections, and for a library of collections its own; each opens on its titles. Each user sees, and counts, only the titles they may see, without those they hid.

  Where to find them: open **Libraries**, then a library, then its **Collections** chip. Home shows the titles of collections, in the rows of collections the administrator adds and in the row of a library of collections, but not the collections themselves: the Apple app shows on Home only cards of movies, series and episodes.
- **Title pages:** details, cast and crew, versions, series with their seasons and episodes, artwork, and titles similar to it, as the web player shows them, without those the user played.
- **Favorites:** the heart of a title page adds it to the user's favorites, those of Jellyfin apps, and takes it off; the Favorites screen lists them by name. Polyfin does not record when a title became a favorite: the apps are told the day it joined the server.
- **Watchlist:** the watchlist button puts a movie or a series on the user's [Watchlist](tracking.md#the-watchlist), a season or an episode its series, and takes it off, off every list holding it: a list read again whole, such as Trakt's watchlist, puts back a title it still holds. The Watchlist screen lists the Watchlist view, as Jellyfin apps show it: a title the user started leaves it, and shows in Continue Watching or Next Up.
- **People:** tapping someone in a cast opens their page and their titles, those the user may see; a search finds people credited in such titles. Polyfin knows people by the titles that credit them, without biographies.
- **For You:** a row of the titles similar to each of the movies the user played last, without those they played, as Jellyfin's movie recommendations give them. It is empty until the user played a movie, or when similar titles are turned off in the settings.
- **Calendar:** the coming episodes of the series the user watches or marked favorite, from yesterday on, as Jellyfin's Upcoming lists them, each on its air date (Polyfin knows air dates, not times). Following, All and the user's favorites or watchlist list those episodes; Trending lists none.
- **History:** what the user played, the latest first.
- Shuffles of a library, a series, a season or a collection: no title twice until all have played, a series playing one of its episodes.
- Removing a card from Continue Watching takes off its resume point, as in the web player.
- The server's name on the sign-in screen, its addresses offered to the user's other devices (**Public address**, under **Settings › Notifications**, then **Address on the local network**, under **Settings › General**; none when both are empty), and artwork at the width each screen draws it.
- **Playback** of movies and episodes. The app sends what it plays; Polyfin answers with a plan:
  - the file as it is, when the app takes it;
  - the file repackaged to HLS, when only its container does not suit the app;
  - converted to HLS otherwise, as the user's conversion permissions allow;
  - audio tracks chosen by the app, or the user's preferred one;
  - text subtitles as WebVTT files beside the video, picture subtitles burned in when chosen. The app chooses a playback's subtitle itself, from the user's subtitle language and mode, which Polyfin tells it: a playback the app starts without a subtitle shows none;
  - lower qualities, 1080p, 720p and 480p, below a taller file: converted to that height, at 8, 4 and 1.5 Mb/s at most;
  - another plan when the app asks for other tracks, another quality, or a conversion after the file failed to play.
- **Progress and stop**, numbered per installation, which keep the resume point; marking a title played or unplayed. The diagnostics of their playbacks the apps send are accepted, and the failures they report logged.

## What does not work yet

- Finding the server on the local network, and signing a TV in with a code: add the address by hand, and sign in with a password. An account with two-step sign-in cannot sign in from Silo's apps, as from Jellyfin apps.
- Editing profiles, PINs, several profiles per user, avatars.
- Downloads, watch-together, remote control, notifications and push, requests, subtitle searches and AI subtitles or translations, books, diagnostics uploads, external sign-in, settings synced from the app and the app's welcome tour: the app is told each is not supported, and hides it. The account tells the apps downloads are not allowed, whatever the user's permission to download in Jellyfin apps.
- The apps' own collections: the app lists none, and making or changing one is refused as not supported. Collections are made in Jellyfin apps or the web player.
- Taking a series off Next Up (refused as not supported: playing it moves it on), hiding watches from the history (mark a title unplayed instead), refreshing a person's details, trending or popular titles, skip markers and trickplay.
- Playback features the apps can use but Polyfin does not offer, and so does not announce: the app's own player showing the file's subtitles, subtitles as tracks of the HLS stream, quality adapting itself to buffering and to the measured speed during a playback, a playback moving to another screen or output without a new plan, and the apps' device fixes chosen by the server. Their plans still play: the app keeps to what Polyfin announces.
- Music, audiobooks, books and Live TV: Silo's apps have no Live TV, and Polyfin lists its libraries of movies and series only.
- Operations not listed here answer 404. Polyfin logs each, as it does for Jellyfin apps: "A Silo app asked for something Polyfin does not serve", with its method and path, at most once an hour each.

## The contract followed

Polyfin follows Silo's published OpenAPI contract, `contracts/api/v2/openapi.json` in Silo-Server/silo-server, at commit `6de6d5e5e285a9b423e95a1122cb39e5bb9ca93b` (25 October 2026). `GET /api/v2/system/info` gives that file's SHA-256 as `contract_digest`, and the links the contract requires; Polyfin serves neither the contract file nor the list of capabilities they name. Silo has no release yet, and its contract still changes: a later version of the apps may need operations or fields Polyfin does not answer.

Its tests also check the capabilities, plans, libraries, rows, catalog pages, Collections tabs, favorites and watchlist, people, similar titles, For You, the calendar and the discovery documents Polyfin gives against the rules the apps apply, read from their code: Silo-Server/silo-apple at commit `2c8f2e1b70b7de6f3edefb01ecd51074924381a9` and Silo-Server/silo-android at commit `9867edfa4500ab962d4abf72341e76596fe595b8` (both 26 October 2026, the head of their main branches when checked). Among them, the apps start a playback only when the server announces every feature they need, and take only playback sessions named by a UUID; they show only libraries of movies, of series, or of both, and leave out of a row on Home or on a library's first page any card but a movie, a series or an episode; the Android app opens a person's page only by a number, which Polyfin gives each person as it does libraries and files; and a feature whose discovery document is not available is hidden.

Polyfin's code is its own, written from the contract's facts; no part of Silo, which is under the AGPL-3.0, is in Polyfin.

**Compared with Jellyfin:**

- Jellyfin does not answer Silo's apps.
- Library and file identifiers are numbers in Silo's API, which Polyfin gives each library and version the first time it lists them, and keeps.
- Polyfin's sessions do not expire: the app's refresh token is its session's token, and refreshing it tells the app whether it was signed out.
