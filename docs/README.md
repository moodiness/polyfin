# Polyfin documentation

These pages explain how to run Polyfin and what it does for the people who use it. New here? Start with [Installation](installation.md), then [Getting started](getting-started.md).

<a id="set-up"></a>

## Start

| Page | What it covers |
|---|---|
| [Installation](installation.md) | Choosing between Docker and Polyfin Desktop, Docker Compose, Unraid, the health check and monitoring, and giving the container a GPU |
| [Polyfin Desktop](desktop.md) | Polyfin Desktop on Windows, macOS and Linux: installing it, the menu, starting with the computer, its folders, the secret key, updates, and moving to Docker and back |
| [Getting started](getting-started.md) | A first-hour tutorial from install to playing a title in a Jellyfin app, then users and Quick Connect |
| [Moving to Polyfin](moving-to-polyfin.md) | Importing accounts and watch history from Jellyfin, Emby, Plex, Letterboxd, Nuvio and Stremio, and how often connected accounts are imported |

<a id="sources"></a><a id="content"></a>

## Addons and libraries

| Page | What it covers |
|---|---|
| [Addons](addons.md) | Adding server and personal addons, who uses each source, which streams play, identifiers, manifests, failing addons, request limits, source order and languages |
| [Libraries](addons-and-libraries.md) | What catalogs become, library order, Show as, genre and maximum, hidden and local titles, catalog limits and refresh, genre pages and similar titles |
| [Smart lists and search](smart-lists-and-search.md) | Smart list rules and sorting, searches in plain words, and movies held back until their digital release |
| [Music, audiobooks and podcasts](music.md) | Music addons and libraries, My music, local music folders, playback, LRCLIB lyrics, volume between songs and parental control |
| [Collections, people and ratings](collections-people-and-ratings.md) | Addon and user collections, people and TMDB credits, MDBList ratings, and the TMDB key: what it brings and how Health explains it |
| [Artwork and editing](artwork-and-editing.md) | Library images, relayed artwork and its cache, poster shapes, badges and replacement posters, generated images, and Edit metadata, Edit images and Identify |

## Local files

| Page | What it covers |
|---|---|
| [Local folders](local-folders.md) | Adding folders mounted in the container, scanning and watching, changing what a folder holds, versions, and home videos |
| [Network shares](network-shares.md) | Reading SMB, WebDAV, S3, SFTP and rclone storage without a mount: addresses, credentials, SFTP host keys, performance and health |
| [Naming and matching files](naming-and-matching.md) | How to name files, extras, NFO and subtitle files, how files are matched, and artwork |
| [Unmatched files](unmatched-files.md) | Files left unmatched and how to link them, Identify in the web player, failed searches and retries, and plain videos |
| [Episode numbering](episode-numbering.md) | How show files' numbers map to the catalog's episodes, absolute numbers, the numbering choices with diagrams, and episodes only the files hold |
| [Discs](discs.md) | Blu-ray discs, DVD folders, disc images, discs on network shares, and shows kept on discs |
| [Torrents](torrents.md) | Playing the torrents addons list over a network interface, a SOCKS5 proxy or a WireGuard tunnel alone: the kill switch, the limits, the permission, and Docker |

## Live TV

| Page | What it covers |
|---|---|
| [Live TV](live-tv.md) | Channels from addons and IPTV sources, channels of your own titles, the programme guide (Native EPG and XMLTV), and recordings |
| [IPTV sources](iptv.md) | Adding M3U and Xtream sources, files of the server, the import options, the source page, the line-up, refreshing, and Dispatcharr |
| [Replay](iptv-replay.md) | The Replay view of past programmes: what lists tell, archive addresses and placeholders, time zones, the guide window, playing a programme and its connection |
| [IPTV movies and series](iptv-movies-and-series.md) | An IPTV source's movies and series: their libraries, how titles are read, how they show and play in apps, and enrichment |
| [IPTV stream headers and encryption](iptv-streams.md) | The headers and user agent sent with a source's streams, DASH channels, and ClearKey decryption |
| [HDHomeRun tuners](hdhomerun.md) | Finding and adding a tuner (Docker networking included), its channels, and sharing its tuners |

<a id="watching"></a>

## Playback

| Page | What it covers |
|---|---|
| [Playback](playback.md) | How a title plays (direct play, relay, repackage or convert), analysis and fallback, chapters, and Watch together |
| [Versions](versions.md) | Which addon streams become versions, title pages, tracks from RemuxDB, live videos, and choosing, ordering and naming versions |
| [Audio and subtitle tracks](audio-and-subtitle-tracks.md) | Default audio and subtitle languages, regional audio, and remembered track choices |
| [Reading the sources](reading-the-sources.md) | Preparing playback in advance, the source cache, keeping files across restarts, preloading, failing sources, and carrying a play on |
| [Scrubbing thumbnails](scrubbing-thumbnails.md) | Trickplay thumbnails and chapter images: settings, when they are made, request limits, look, and failing sources |
| [Transcoding](transcoding.md) | When Polyfin converts, adaptive quality, conversions at once, converted downloads, HLS segments and every Conversion setting |
| [Quality of converted video](conversion-quality.md) | How large converted video is, the bitrate its video and audio get, and what an app's profile and the settings allow |
| [GPUs and HDR](gpus-and-hdr.md) | Converting on NVIDIA, AMD, Intel and Apple GPUs, Docker GPU setup, and HDR tone mapping |
| [Loudness](loudness.md) | How loud titles play, the stereo mix, evening converted audio, and each user's louder-sound options |
| [Subtitles](subtitles.md) | Addon subtitles, text tracks inside files, ASS styles and fonts, and picture subtitles |
| [Skip segments](skip-segments.md) | The skip buttons for intros, recaps, credits and previews: the sources, their order, and their keys |
| [Tracking services](tracking.md) | Sending what you watch to Trakt, Simkl, MDBList, PublicMetaDB, AniList and MyAnimeList, and the songs you play to Last.fm and ListenBrainz, and importing what you watched there |

## Apps

| Page | What it covers |
|---|---|
| [Jellyfin apps](jellyfin-compatibility.md) | The apps that work with Polyfin, signing in with Quick Connect, finding the server, the Android integrated player, what apps get, and the Jellyfin features Polyfin does without |
| [Web player](web-client.md) | The built-in web player, what Polyfin's script adds to it, dashboard links and the sign-in handoff to the admin app, caching, and the license |
| [Home rows](home-rows.md) | Rows an administrator adds to the web player's home page: kinds, places, schedules, preview, and the taste profile behind Top picks for you |
| [Themes and custom code](themes-and-custom-code.md) | The web player's themes, a theme per user, LumaaGlaass, other themes and copies of them, custom CSS and JavaScript, the sign-in message and branding |
| [Silo's apps](silo-apps.md) | Silo's apps for Apple devices and Android on Polyfin: turning them on, what works, what they lack, and the contract followed |
| [DLNA](dlna.md) | Sharing the libraries with TVs and players that have no Jellyfin app: devices and their users, profiles, Docker networking, and limits |

<a id="users-and-administration"></a>

## Users

| Page | What it covers |
|---|---|
| [Users and permissions](users.md) | Accounts, groups, resets, end dates, a summary of the limits, and DLNA devices |
| [User limits](user-limits.md) | Parental control, blocked genres, visible libraries, allowed hours, remote access, playback and access limits, and quality groups |
| [Sign-in and security](sign-in-and-security.md) | Two-step sign-in, the identity provider and the security settings |
| [Invite and share links](invite-and-share-links.md) | Links that let people create their own account, or watch one title without one |
| [Watch state and preferences](watch-state.md) | Played marks, Continue Watching, Next Up, track choices, thresholds, metadata language and audio region |

## Server

| Page | What it covers |
|---|---|
| [The admin app](administration.md) | A tour of the admin app: the top bar, My account, Home and Now playing, Schedule, Logs, Settings and saved keys |
| [Health](health.md) | What **System › Health** shows, each check it runs, and each warning and error, with how to fix it |
| [Notifications](notifications.md) | Targets and their kinds, the events each one tells and who receives them, and how messages are delivered, paced and logged |
| [Webhook reference](webhook-reference.md) | The JSON a webhook receives, what each target kind receives, and the admin API for targets |
| [Statistics](statistics.md) | The playback history, the Statistics page, each user's own figures, the CSV export and their API |
| [Updates and restarts](updates-and-restarts.md) | How Polyfin tells you of a new version, and how you restart or update it from Home |
| [HTTPS and networking](https-and-networking.md) | Letting apps find Polyfin on the local network, and serving HTTPS with your own certificate or Let's Encrypt |
| [Reverse proxy](reverse-proxy.md) | Serving Polyfin over HTTPS behind a proxy: the headers to pass, WebSockets, known proxies, and an example with Traefik |
| [Configuration](configuration.md) | Environment variables and the Compose `.env` file |
| [Backups](backups.md) | Daily database backups, how many are kept, and restoring one |
| [Metrics and API keys](metrics-and-api-keys.md) | Prometheus metrics, API keys for other tools, and what Jellyfin apps can administer |

<a id="contributing"></a>

## Reference

| Page | What it covers |
|---|---|
| [Jellyfin API notes](jellyfin-api.md) | For app developers: where Polyfin's Jellyfin API differs from Jellyfin 12.4, Polyfin's own routes, and the admin app API |
| [Development](development.md) | Building, running and testing Polyfin, following Silo's changes, and its website |
