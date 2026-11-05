# Polyfin documentation

These pages explain how to run Polyfin and what it does for the people who use it. New here? Start with [Installation](installation.md), then [Getting started](getting-started.md).

<a id="set-up"></a>

## Start

| Page | What it covers |
|---|---|
| [Installation](installation.md) | Choosing between Docker and Polyfin Desktop, Docker Compose (network modes, ports, HTTPS and work folders), Unraid, the health check, and giving the container a GPU |
| [Polyfin Desktop](desktop.md) | Polyfin Desktop on Windows, macOS and Linux: installing it, the first start, NVIDIA GPUs, the menu, its folders, the secret key, updates, restarting, and moving to Docker and back |
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
| [Torrents](torrents.md) | Turning torrents on, the three routes and what each carries, Docker, the kill switch, how a torrent starts and when it stops, the cap, failures to start, trackers, the file played, and Health |
| [Torrent peers and pieces](torrent-peers.md) | How Polyfin downloads a torrent: trackers and the DHT, pieces and reading ahead, peers, peer exchange, slow peers, bounds, and uTP |

## Live TV

| Page | What it covers |
|---|---|
| [Live TV](live-tv.md) | Channels from addon TV catalogs and IPTV sources (how they play, start, share connections and fail), channels of your titles, the programme guide (Native EPG and XMLTV) and programme reminders |
| [Recordings](recordings.md) | Recording Live TV: the recordings folder, who can record, scheduling and series timers, settings, tuners, smaller HEVC recordings and watching recordings |
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
| [Subtitles](subtitles.md) | Where each app's subtitles come from: addon subtitles and their sync, subtitle files from streams, local folders and torrents, text tracks inside files, ASS fonts, and picture subtitles read into text |
| [Skip segments](skip-segments.md) | Skip buttons: which source wins, chapters, the four databases and their keys, intros and credits found in local episodes' audio, and commercials in recordings |
| [Tracking services](tracking.md) | Connecting Trakt, Simkl, MDBList, PublicMetaDB, AniList, MyAnimeList, Last.fm and ListenBrainz, what each one is sent, the anime mapping, music scrobbling, administrator set-up, and delivery |
| [Tracking imports and the Watchlist](tracking-imports.md) | Importing watch history and watchlists from tracking services, the Watchlist view, the Now available and Still waiting rows, and Recommended on Trakt |

## Apps

| Page | What it covers |
|---|---|
| [Jellyfin apps](jellyfin-compatibility.md) | The apps that work with Polyfin, signing in with Quick Connect, finding the server, the Android integrated player, what apps get, and the Jellyfin features Polyfin does without |
| [Web player](web-client.md) | The built-in web player, what Polyfin's script adds to it, dashboard links and the sign-in handoff to the admin app, caching, and the license |
| [Home rows](home-rows.md) | Rows an administrator adds to the web player's home page: kinds, places, schedules, preview, and the taste profile behind Top picks for you |
| [Themes and custom code](themes-and-custom-code.md) | The web player's themes, a theme per user, LumaaGlaass, other themes and copies of them, custom CSS and JavaScript, the sign-in message and branding |
| [Silo's apps](silo-apps.md) | Using Silo's apps with Polyfin: setup, what works, what does not yet, skip markers, Home layout, subtitle tracks, box sets, TV sign-in with a code, and the Silo contract Polyfin follows |
| [DLNA](dlna.md) | Sharing the libraries with TVs and players over DLNA: setup, devices and their users, browsing, playing, profiles, Docker networking and limits |

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
| [Statistics](statistics.md) | How playbacks are tracked, the playback history and its settings, and the Statistics pages of System and My account |
| [Updates and restarts](updates-and-restarts.md) | How Polyfin tells you of a new version, and how you restart or update it from Home |
| [HTTPS and networking](https-and-networking.md) | Letting apps find Polyfin on the local network, and serving HTTPS with your own certificate or Let's Encrypt |
| [Reverse proxy](reverse-proxy.md) | Running Polyfin behind a reverse proxy: headers, WebSockets, slow answers, HTTPS, known proxies and a worked Traefik example |
| [Configuration](configuration.md) | The environment variables, the data folder, the work folders, `.env`, and how stored secrets are encrypted with `POLYFIN_SECRET_KEY` |
| [Backups](backups.md) | Daily database backups, their encryption, destinations, and restoring from the admin app, the setup page or by hand |
| [Metrics and API keys](metrics-and-api-keys.md) | Prometheus metrics, API keys for other tools, and what Jellyfin apps can administer |

<a id="contributing"></a>

## Reference

| Page | What it covers |
|---|---|
| [Jellyfin API notes](jellyfin-api.md) | For app developers: where Polyfin's Jellyfin API differs from Jellyfin 12.4, Polyfin's own routes, and the admin app API |
| [Development](development.md) | Running Polyfin from source, its tests, the Jellyfin fixtures, CI, following Silo, working on the website, and building and signing Polyfin Desktop's packages |
