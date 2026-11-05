# Polyfin documentation

These pages explain how to run Polyfin and what it does for the people who use it. New here? Start with [Installation](installation.md), then [Getting started](getting-started.md).

<a id="set-up"></a>

## Start

| Page | What it covers |
|---|---|
| [Installation](installation.md) | Choosing between Docker and Polyfin Desktop, Docker Compose, Unraid, the health check and monitoring, and giving the container a GPU |
| [Polyfin Desktop](desktop.md) | Polyfin Desktop on Windows, macOS and Linux: installing it, the menu, starting with the computer, its folders, the secret key, updates, and moving to Docker and back |
| [Getting started](getting-started.md) | The first run, the administrator, users, Quick Connect, and the language of generated names |
| [Moving to Polyfin](users.md#moving-from-jellyfin) | Bringing accounts and watch history from Jellyfin, Emby or Plex, and importing what you watched in other apps |

<a id="content"></a>

## Sources

| Page | What it covers |
|---|---|
| [Addons and libraries](addons-and-libraries.md) | Stremio and music addons, libraries, song lyrics, catalog limits, collections, people, similar titles, artwork, and editing items |
| [Local folders](local-folders.md) | Your own video files from folders mounted in the container, or from SMB and WebDAV shares, S3 buckets, SFTP servers and rclone remotes: permissions, naming, matching, scanning, watching for changes, unmatched files and versions |
| [Torrents](torrents.md) | Playing the torrents addons list over a network interface, a SOCKS5 proxy or a WireGuard tunnel alone: the kill switch, the limits, the permission, and Docker |
| [Live TV](live-tv.md) | Channels from addons and IPTV sources, channels of your own titles, the programme guide (Native EPG and XMLTV), and recordings |
| [IPTV sources](iptv.md) | M3U and Xtream Codes sources, HDHomeRun tuners, the line-up page, Replay of the providers' archives, and IPTV movies and series |

<a id="watching"></a>

## Playback

| Page | What it covers |
|---|---|
| [Playback](playback.md) | How a title plays, versions, chapters, preparing playback ahead, choosing a version, watching together, and thumbnails |
| [Transcoding](transcoding.md) | When Polyfin converts, GPUs, HDR tone mapping, and the conversion settings |
| [Subtitles](subtitles.md) | Addon subtitles, text tracks inside files, ASS styles and fonts, and picture subtitles |
| [Skip segments](skip-segments.md) | The skip buttons for intros, recaps, credits and previews: the sources, their order, and their keys |
| [Tracking services](tracking.md) | Sending what you watch to Trakt, Simkl, MDBList, PublicMetaDB, AniList and MyAnimeList, and the songs you play to Last.fm and ListenBrainz, and importing what you watched there |

## Apps

| Page | What it covers |
|---|---|
| [Jellyfin apps](jellyfin-compatibility.md) | Compatible apps, what apps get, and the Jellyfin features Polyfin does without |
| [Web player](web-client.md) | The built-in jellyfin-web, single sign-on to the admin app, home rows, themes, and custom CSS and JavaScript |
| [Silo's apps](silo-apps.md) | Silo's apps for Apple devices and Android on Polyfin: turning them on, what works, what they lack, and the contract followed |
| [DLNA](dlna.md) | Sharing the libraries with TVs and players that have no Jellyfin app: devices and their users, profiles, Docker networking, and limits |

<a id="users-and-administration"></a>

## Server

| Page | What it covers |
|---|---|
| [Users and permissions](users.md) | Watch state, parental control, per-user limits, quality groups, sign-in, and the security settings |
| [Administration](administration.md) | The admin app (Home, search, System pages, settings), the check for new versions, API keys and metrics for Prometheus |
| [Notifications](notifications.md) | Webhook, Discord, ntfy, email, Telegram, Gotify and Pushover targets for new episodes, recordings, Health problems, new versions and playbacks, and the webhook event's JSON |
| [Statistics](statistics.md) | The playback history, the Statistics page, each user's own figures, the CSV export and their API |
| [Configuration](configuration.md) | Environment variables and the Compose `.env` file |
| [Reverse proxy](reverse-proxy.md) | Serving Polyfin over HTTPS behind a proxy: the headers to pass, WebSockets, known proxies, and an example with Traefik |
| [Backups](backups.md) | Daily database backups, how many are kept, and restoring one |

## Contributing

| Page | What it covers |
|---|---|
| [Development](development.md) | Building, running and testing Polyfin, following Silo's changes, and its website |
