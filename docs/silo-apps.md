# Silo's apps

Silo's native apps, for iPhone, iPad, Apple TV and Mac, and for Android phones and TVs, can use Polyfin as their server. They speak Silo's own API, under `/api/v2`, not Jellyfin's: Polyfin answers the part of it they need to sign in, browse and play, while the setting allows them.

## Turning it on

**Settings › General › Allow Silo's apps**, off by default. While it is off, Polyfin answers nothing under `/api/v2`: the apps report that they cannot reach a Silo server there.

In the app, add the server by the address Jellyfin apps use, such as `http://192.168.1.10:8096`, and sign in with a Polyfin user's name and password. Behind a reverse proxy, `/api/v2` must reach Polyfin like the rest; a base path such as `/jellyfin` is for Jellyfin apps only.

## How it works

Each answer of Silo's API is made of the Jellyfin API's answer for the same user and device. What Silo's apps see and play is what Jellyfin apps get, under the same limits:

- parental control, blocked genres, the libraries the user may see or hid, and the titles they hid;
- allowed hours, disabled and ended accounts;
- version rules, quality groups, the user's bitrate limit and conversion permissions;
- the sign-in throttle, and two-step sign-in.

A signed-in app is a device of the user, listed with the others and signed out the same ways. Its playbacks show in the dashboard and the statistics, and its played state and resume points are the user's, shared with Jellyfin apps.

## What works

- **Connecting:** the server's information, identity and setup state.
- **Signing in** with a name and password, staying signed in, and signing out.
- **Profiles:** each Polyfin user is one profile, named after them, without a PIN.
- **Home:** Continue Watching, Next Up, and the latest titles of each library.
- **Libraries** of movies and series, and mixed ones, page by page, sorted by title, year, release date, date added, rating, runtime or at random; favorites; search.
- **Title pages:** details, cast and crew, versions, series with their seasons and episodes, and artwork.
- **Playback** of movies and episodes. The app sends what it plays; Polyfin answers with a plan:
  - the file as it is, when the app takes it;
  - the file repackaged to HLS, when only its container does not suit the app;
  - converted to HLS otherwise, as the user's conversion permissions allow;
  - audio tracks chosen by the app, or the user's preferred one;
  - text subtitles as WebVTT files beside the video, picture subtitles burned in when chosen;
  - another plan when the app asks for other tracks, another quality, or a conversion after the file failed to play.
- **Progress and stop**, numbered per installation, which keep the resume point; marking a title played or unplayed.

## What does not work yet

- Finding the server on the local network, and signing a TV in with a code: add the address by hand, and sign in with a password. An account with two-step sign-in cannot sign in from Silo's apps, as from Jellyfin apps.
- Editing profiles, PINs, several profiles per user, avatars.
- Downloads, watch-together, remote control, notifications.
- Collections, watchlists, history, requests, recommendations, people's pages, settings synced from the app, subtitle searches, skip markers and trickplay.
- Music, audiobooks, books and Live TV: Silo's apps have no Live TV, and Polyfin lists its libraries of movies and series only.
- Operations not listed here answer 404.

## The contract followed

Polyfin follows Silo's published OpenAPI contract, `contracts/api/v2/openapi.json` in Silo-Server/silo-server, at commit `6de6d5e5e285a9b423e95a1122cb39e5bb9ca93b` (25 October 2026). `GET /api/v2/system/info` gives that file's SHA-256 as `contract_digest`. Silo has no release yet, and its contract still changes: a later version of the apps may need operations or fields Polyfin does not answer.

Polyfin's code is its own, written from the contract's facts; no part of Silo, which is under the AGPL-3.0, is in Polyfin.

**Compared with Jellyfin:**

- Jellyfin does not answer Silo's apps.
- Library and file identifiers are numbers in Silo's API, which Polyfin gives each library and version the first time it lists them, and keeps.
- Polyfin's sessions do not expire: the app's refresh token is its session's token, and refreshing it tells the app whether it was signed out.
