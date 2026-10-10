# Polyfin documentation

These pages explain how to run Polyfin and what it does for the people who use it. New here? Start with [Installation](installation.md), then [Getting started](getting-started.md).

## Set up

| Page | What it covers |
|---|---|
| [Installation](installation.md) | Docker Compose, Unraid, and giving the container a GPU |
| [Getting started](getting-started.md) | The first run, the administrator, users, Quick Connect, and the language of generated names |
| [Configuration](configuration.md) | Environment variables and the Compose `.env` file |
| [Backups](backups.md) | Daily database backups, how many are kept, and restoring one |

## Content

| Page | What it covers |
|---|---|
| [Addons and libraries](addons-and-libraries.md) | Stremio and music addons, libraries, song lyrics, catalog limits, collections, people, similar titles, artwork, and editing items |
| [Live TV](live-tv.md) | Channels from addons, programme guides (Native EPG and XMLTV), and recordings |
| [IPTV](iptv.md) | M3U and Xtream Codes sources, the line-up page, Replay of the providers' archives, and IPTV movies and series |
| [Local folders](local-folders.md) | Your own video files from folders mounted in the container or from SMB and WebDAV shares: permissions, network shares, naming, matching, scanning, unmatched files and versions |

## Watching

| Page | What it covers |
|---|---|
| [Playback](playback.md) | How a title plays, versions, chapters, preparing playback ahead, choosing a version, watching together, and thumbnails |
| [Transcoding](transcoding.md) | When Polyfin converts, GPUs, HDR tone mapping, and the conversion settings |
| [Subtitles](subtitles.md) | Addon subtitles, text tracks inside files, ASS styles and fonts, and image subtitles |
| [Skip segments](skip-segments.md) | Skipping intros, recaps, credits and previews: the sources, their order, and their keys |
| [Tracking](tracking.md) | Sending what you watch to Trakt, Simkl, MDBList and PublicMetaDB, and the songs you play to Last.fm and ListenBrainz, and importing what you watched there |

## Users and administration

| Page | What it covers |
|---|---|
| [Users](users.md) | Watch state, moving from Jellyfin, parental control, per-user limits, quality groups, and the security settings |
| [Administration](administration.md) | The admin app (home, search, system pages, settings) and API keys |
| [Notifications](notifications.md) | Webhook, Discord, ntfy, email, Telegram, Gotify and Pushover targets for new episodes, recordings and Health problems, and the webhook event's JSON |
| [Web client](web-client.md) | The built-in jellyfin-web, single sign-on to the admin app, and custom CSS and JavaScript |
| [Jellyfin compatibility](jellyfin-compatibility.md) | Compatible apps, what apps get, and the Jellyfin features Polyfin does without |

## Contributing

| Page | What it covers |
|---|---|
| [Development](development.md) | Building, running and testing Polyfin, and its website |
