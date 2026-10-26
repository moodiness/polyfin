# Administration

This page covers Polyfin's admin app: its Home, its System pages (Health, Statistics, Schedule, Logs and API keys) and Settings, API keys for other tools, and the administration features Jellyfin apps can use.

## The admin app

The admin app is at `/admin/`. From the [web client](web-client.md), the dashboard link opens it too, and jellyfin-web's dashboard pages open their counterpart: its users, libraries, logs, scheduled tasks, API keys and playback settings. Members open it from the **Polyfin** entry the web client shows them instead of **Dashboard**. Either way, a user signed in to the web client arrives signed in (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

A bar at the top holds the sections, for administrators:

- **Home** (`/admin/`): what plays now and what needs a look;
- **Content**: **Sources** (`/admin/sources`), **Libraries** (`/admin/libraries`) and **Live TV** (`/admin/live-tv`);
- **Users** (`/admin/users`);
- **System**: **Health**, **Statistics**, **Schedule**, **Logs** and **API keys** (`/admin/system/…`);
- **Settings**, one page per section (`/admin/settings/general`, `/admin/settings/content`…).

The account menu, on the right, holds **My account**, **My sources** and **Quick Connect** (`/admin/me/…`), the language and **Sign out**. Members see a shorter bar: **Home**, **My sources**, **My account** and **Quick Connect**. The other pages tell them they may not open them. On a phone, the bar keeps the logo, the search and a menu button that opens all of these.

The admin app is in English, French, German, Spanish, Italian and Brazilian Portuguese. It starts in the first of these the browser prefers, any Portuguese in Brazilian Portuguese, else in English; the language list at the top of the sign-in and setup pages, and **Language** in the account menu, change it for this browser.

**My account** has these sections: **Statistics**, the hours you watched, your most played titles, apps and devices, and your recent playbacks, with **Export CSV** (see [Statistics](statistics.md#my-account)), **Tracking** (see [Tracking services](tracking.md)), **Continue Watching** and **Hidden titles**, **Reminders**, the programmes and series you asked to be reminded of from the web client, with **Delete**, shown with the **Live TV** permission (see [Programme reminders](live-tv.md#programme-reminders)), **Notifications** (see [Notifications](notifications.md)), with **Remind me before** and **Also show reminders on my apps** under the targets, **Devices**, the apps signed in with the account, **Password**, and **Two-step sign-in**, a code from an authenticator app after the password (see [Two-step sign-in](users.md#two-step-sign-in)). Signing out a device asks first.

Signing in to the admin app takes the account's name and password, then, for an account with two-step sign-in, the code of its authenticator app or a recovery code. Administrators can require it of administrators under **Settings › Security**, and turn it off for a user who lost their phone (see [For administrators](users.md#for-administrators)).

**Search** (⌘K on a Mac, Ctrl K elsewhere, or `/`) finds pages, settings, users and sources by name, with or without accents. Use the arrows and Enter to open one, Escape to close it.

Addresses from before this layout, such as `/admin/health` or `/admin/settings#settings-tracking`, still open the right page.

**Compared with Jellyfin:**

- None of the admin app's pages exist in Jellyfin's API. They use Polyfin's admin API only.

## Home

**Home** greets you by name and shows:

- **Now playing**, updated every 3 seconds. A playback whose app reported nothing for five minutes, as a tab closed while it played, leaves it (see [Playbacks at once](users.md#playbacks-at-once)). For each playback you see the user, the device and the app, the title and the position, and **How it plays**: direct play, remux or conversion. **Details** adds Jellyfin's reasons, the user's quality group, the resolution and bitrate sent, and whether the GPU or the CPU encodes. The source it describes is the version played: when the app names only the title, as jellyfin-web does when it plays a title's first version as it is, the version the app's last PlaybackInfo chose for it. When the video sent is smaller than the source, **Smaller than the source** tells what limited it: the GPU's 4K, the processor's 1080p, HDR converted by the processor, the user's quality group, **Maximum quality of converted video**, or the bitrate allowed by the app or by the user's **Maximum quality**. See [Size of converted video](transcoding.md#size-of-converted-video). A [converted download](transcoding.md#converted-downloads) shows there too, labeled "Downloading", its bar telling how far the copy is made.
- a notice when a new version of Polyfin is out, with **Release notes** (see [New versions](#new-versions));
- **To look at**: the problems Health found, with a link to it.
- **Server state**: version, database, cache and graphics card.
- **Recent activity**, the activity log, which you can filter by kind and search. Its list scrolls in its own box and loads older events as you reach its end.

### Stopping a playback or sending a message

An administrator can stop a playback (**Stop**, which asks first), or show a message on its app (**Send a message**), when the app accepts remote control. This works as Jellyfin's remote control does. An administrator whose **Can control other users' apps** permission was taken away is refused. See [users](users.md) for permissions.

## Statistics

**System › Statistics** shows how the server is used, for **7 days**, **30 days**, a **Year** or **All** the history, for everyone or one user: hours watched per user and per day, the most played movies, series and channels, the apps and devices, how videos play (direct play, remux or conversion) and the busiest hours of the week, then the recent playbacks, which **Export CSV** downloads. **Settings › Playback** turns the history off or sets how long it is kept. See [Statistics](statistics.md) for how playbacks are counted, the settings and the API.

## Schedule

**System › Schedule** lists:

- the **Tasks**, with their last run, result and next run; **Run now** and **Stop** run or stop each one by hand;
- **Database backups**, with **Back up now**, each destination's last upload, and **Restore** (see [Backups](backups.md));
- the **Upcoming recordings** (see [Live TV](live-tv.md));
- the **Live TV refreshes**: when the IPTV lists and guides are fetched again (see [IPTV](iptv.md)).

Schedule covers the server's addons, IPTV sources and guides first, then those users keep under **My sources**, each marked with its owner.

## Health

**System › Health** puts problems first, under **Needs attention**, followed there by a new version of Polyfin when one is out (see [New versions](#new-versions)). It then shows:

- **Addons**, from the requests made for apps since start: last answer, last failure and response time. **Check** asks an addon for its manifest once, at most once a minute. For a member's own addon, it reaches only public addresses.
- **IPTV sources** and **Programme guides**.
- **Network shares**, when there are some: whether each is **Notified** of its changes by its server, **Polled** by listing its folders again every 2 minutes, or **Not watched**, and why (see [Local folders](local-folders.md#watching-for-changes)).
- Under **Server**:
  - the **Transcoder**: GPU, encoders, and conversions against the limit (see [transcoding](transcoding.md));
  - the **Database** and its size;
  - the **Source cache**, against **Disk space for files being read (GB)**, with the part torrent pieces take;
  - the **Image cache**, against its 1 GB on disk, with the images it keeps there and in memory, and **Clear the image cache** (see [Clearing the image cache](#clearing-the-image-cache));
  - **Torrents**, while they are on: the route, the torrents playing, against **Torrents at once** when it is set, their peers and those reached over uTP, and what was shared (see [Torrents](torrents.md));
  - **Disk space** for the cache, recordings and backups folders, and for the conversion folder and the folder for files being read when they are other folders than the cache's (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache));
  - **Backups**: the last run and its result, the last backup made, its file and size, the next one, each destination's last upload, and the last restore. A failed backup, or a last backup older than two days, is a problem; so is a destination whose last upload failed (an error), or whose newest backup is older than two days (a warning). See [Backups](backups.md);
  - **Thumbnails (trickplay)**: the queue and paused sources;
  - **Polyfin**: memory, goroutines, uptime and version, and **Download the goroutines**: a text file of what every goroutine of the server is doing, with its stack and how long it has waited, as Go prints it when a program crashes. When a request never answers, download it while the request still waits, and send it with the report. It holds the paths of Polyfin's source files, but no address, token or password. Only administrators may download it (`GET /admin/api/health/goroutines`);
  - **Stored keys**: whether `POLYFIN_SECRET_KEY` encrypts them, how many are stored unencrypted, and which cannot be decrypted with it (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)). Keys stored unencrypted show as a warning, keys that cannot be decrypted as an error;
  - **Known proxies**: while some are listed by host name, the addresses each name resolves to (see [Proxies by name](reverse-proxy.md#proxies-by-name)).

An address on the local network that sends `X-Forwarded-For` or `X-Real-IP` without being one of the **Known proxies** is a warning, named for an hour after its last such request, with a link to the setting: it is usually a reverse proxy missing from it (see [Known proxies](reverse-proxy.md#known-proxies)). A host name of **Known proxies** that resolves to nothing is a warning too, with a link to the setting: it trusts nothing.

Local network discovery has two warnings, linked to its settings (see [Local network discovery](#local-network-discovery)): its UDP port is used by another program, such as a Jellyfin server on the same host, or its answers give an address of Polyfin's container, which apps on the local network cannot reach.

HTTPS has four, linked to its settings: a port it cannot listen on, certificate files that cannot be served, a failed attempt with Let's Encrypt, and a certificate that expires within 14 days, an error once it expired (see [HTTPS problems](#https-problems)).

A picture subtitle track that could not be read into text, its language having no Tesseract data, is a warning, one per language, naming the language and the `.traineddata` file to put in the folder `POLYFIN_TESSDATA_DIR` names, with a link to **Turn picture subtitles into text**. It goes away once the file is there (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).

When every metadata addon of the server has a language, a warning names the first one, with a link to its page: they then describe titles in their language for every user, whatever the user's metadata language. Keep **Language of its descriptions** at **None** on that addon, or on one more metadata addon (see [Languages](addons-and-libraries.md#languages)).

An NVIDIA graphics card whose driver is older than the version FFmpeg needs for NVENC is a warning naming that version, with a link to **Transcoder**: video is converted on another graphics card or on the processor until the driver is updated (see [NVIDIA driver](transcoding.md#nvidia-driver)).

A local folder whose files are left unmatched because the search of their titles failed is a warning, naming the addon that failed most of them and why, with a link to the folder's page. It goes away once the searches go through, which Polyfin tries again on its own (see [Failed searches](local-folders.md#failed-searches)).

An addon whose search catalog did not answer a search in time is a warning, with a link to **Addons**: searches leave it out for 5 minutes, then ask it again (see [Catalog pages](addons-and-libraries.md#catalog-pages)). It comes and goes with the searches, so it is not sent as a health problem; the addon's own failure, "did not answer in time", is.

A local folder whose files are left unmatched because no addon of the server describes and searches titles of its kind is a warning too, with a link to **Content › Sources**, where such an addon is added: users' own addons, under **My sources**, do not count. It goes away once a scan matches those files (see [What a search needs](local-folders.md#what-a-search-needs)).

A copy of the web player's **Custom** theme kept in Polyfin that misses some of its files, as after a backup restored on another system or into another data folder, is a warning with how many are missing, linked to the copy under **Settings › Web player**, while the **Custom** theme is chosen and its texts load the copy. Browsers meanwhile load those files from their original addresses, which needs the internet. **Update the copy** downloads them again and clears it; so does choosing another theme (see [Keeping a copy of a theme](web-client.md#keeping-a-copy-of-a-theme)).

A **Conversion folder**, **Folder for files being read** or **Cache folder** that Polyfin could not write into when it started, such as a disk not mounted, is a warning naming the folder, with a link to its setting: the data folder's `cache` holds those files until a start finds the folder writable (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)). **For app developers:** `GET /admin/api/health/problems` lists it as `folder_unwritable`, with `folder` (`conversion`, `reading` or `cache`), `path` and `error`; `/admin/api/settings` holds the folders as `conversionFolder`, `readingFolder` and `cacheFolder`, with `cacheFolderDefault`, and a `PUT` with a folder Polyfin cannot write into answers 400 `invalid_conversion_folder`, `invalid_reading_folder` or `invalid_cache_folder`.

A user's app connection or tracking service that stopped, its app or service having refused what Polyfin kept, is a warning, one per user and app or service, which opens the user's page. Only that user can connect it again. It is not sent as a health problem: the **Connection stopped** notification told it (see [Stopped connections](notifications.md#stopped-connections)).

Torrent streams that cannot play are an error, naming why: the network interface missing, down or without an address, the SOCKS5 proxy not answering or refusing its user, or the WireGuard server not answering the handshake. A proxy without `UDP ASSOCIATE` is a warning: torrents play over TCP, with UDP trackers and the DHT off. Both link to **Settings › Torrents** (see [Kill switch](torrents.md#kill-switch)).

Health sends no request outside the server, but for the SOCKS5 proxy and the WireGuard server of torrents, checked as their kill switch checks them. Like Schedule, it covers the server's addons, IPTV sources and guides first, then those users keep under **My sources**, each marked with its owner. Their problems count in the summary. The problems it finds, and those solved, can be sent to webhooks, Discord, ntfy, email, Telegram, Gotify or Pushover: see [Notifications](notifications.md).


### Clearing the image cache

**Clear the image cache**, under **Image cache** in **System › Health**, empties at once the cache of the images Polyfin serves to apps (see [Artwork](addons-and-libraries.md#artwork)), and tells in a message how many images and bytes it freed. It sits beside the cache's size, which then shows empty: a task under **System › Schedule** could not show what it freed, and the cache needs no schedule, as it keeps within its limit on its own.

It empties:

- the artwork relayed from the addons' artwork servers, IPTV channel logos and people's photos included, in memory and in the `images` folder of Polyfin's cache folder;
- the copies resized for the apps, and the posters warmed ahead with their sizes, those of local folders included;
- the images Polyfin draws for collections, smart lists and libraries;
- the downloads remembered as failed, which are asked for again at once rather than after 2 minutes.

Each image is downloaded, resized or drawn again at its next request, and the posters of the libraries' first pages are warmed again by the next run of "Read the libraries' first pages", within the hour. An image being downloaded or resized as the cache is cleared is kept once it ends.

It leaves alone the scrubbing thumbnails and chapter images, kept in the database within **Space for images (GB)** under **Settings › Thumbnails**; the images uploaded with **Edit images** or chosen for libraries, kept in the database; the artwork files of local folders, which stay beside their files; the copy of the web player's theme kept under **Settings › Web player**; and which titles each generated image shows.

**Compared with Jellyfin:** Jellyfin's "Clean Cache Directory" task (`DeleteCacheFiles`) runs every 24 hours and deletes the files of its cache folder older than 30 days, its resized images included ([DeleteCacheFileTask.cs, lines 71 to 83](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteCacheFileTask.cs#L71-L83)). Polyfin's image cache keeps within 1 GB instead, the least recently used going first, and is emptied whole by hand.

**For app developers:** `GET /admin/api/health` gives the cache as `images`: `files` and `used` (bytes) on disk, `limit`, `memoryImages` and `memoryBytes`. `POST /admin/api/health/images/clear`, for administrators, empties it and answers what it freed in the same shape.

## New versions

While **Check for new versions** is on, under **Settings › General**, Polyfin asks GitHub for its latest release a minute after it starts, then once a day. This is the one request Polyfin makes to GitHub for itself:

- it asks `https://api.github.com/repos/moodiness/polyfin/releases/latest`, which leaves out pre-releases and drafts;
- it sends nothing but Polyfin's version, in its `User-Agent: Polyfin/<version>` header;
- it goes through the proxy the environment sets (`HTTPS_PROXY`), and gives up after 10 seconds.

A failed request, or GitHub asking to wait (403 or 429), changes nothing, and is tried again an hour later. A release whose tag is a pre-release is left out too.

When the release is newer than the running version, in semantic version order:

- **Home** shows a notice, and **System › Health** an item under **Needs attention**, with the new version and **Release notes**, which opens them on GitHub;
- Jellyfin apps see `HasUpdateAvailable` true in `/System/Info`;
- the **New version** notification is sent once per version, to the server's targets and administrators' own (see [Notifications](notifications.md#new-versions)).

The version found and the version told are kept in the database: after a restart, the notice shows at once, and the notification is not sent again. Once Polyfin runs that version, the notice goes.

A development build, whose version is `dev`, asks nothing and never shows a new version. Turned off, Polyfin never contacts GitHub, the notice and the item go, and `HasUpdateAvailable` is false.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Check for new versions** | **Settings › General** | On | Asks GitHub once a day, and a minute after Polyfin starts, for Polyfin's latest release, and tells administrators about a newer one. Off, GitHub is never asked. |

## Restart and updates

Administrators restart Polyfin, and start its update, from **Home**, under **Server state**, without opening the host or Docker. Polyfin has no action that stops it for good: in a container with a restart policy, or in Polyfin Desktop, it would start again at once, and a remote off switch is easy to misuse from a stolen session. Polyfin is never given the Docker socket either: it is root on the host for whoever takes over a server.

Each action:

- is an administrator's alone;
- is confirmed each time with the administrator's password, or, with two-step sign-in on, a code of their app or a recovery code: wrong ones count as wrong passwords at sign-in do (see [Two-step sign-in](users.md#two-step-sign-in));
- runs at most once a minute: another request within the minute is refused, and so is a restart within a minute of Polyfin starting, so that restarts cannot follow each other across the restart;
- is recorded in the activity log, with who asked, and shows in **Recent activity** under **Server and addons**.

### Restart

**Restart** stops Polyfin as a stop signal does: playbacks, conversions, recordings and tasks end, open connections are given up to 15 seconds, then Polyfin exits with code 75 (`EX_TEMPFAIL`), which tells that it should be started again. Whatever runs Polyfin starts it again:

- in Docker, the container's restart policy: `unless-stopped` and `always`, and `on-failure` too, as the code is not 0 (see [Restart from the admin app](installation.md#restart-from-the-admin-app));
- under systemd, `Restart=always` or `Restart=on-failure`;
- in Polyfin Desktop, the app itself, as when the server ends by itself (see [Restart](desktop.md#restart)).

The admin app then waits, and reloads once a new Polyfin answers, which apps reconnect to as well. Sessions are kept in the database: nobody signs in again.

A container does not tell the processes in it its restart policy: only the Docker socket does. So Polyfin cannot know whether it will be started again, unless it runs in Polyfin Desktop or an administrator turns on **Polyfin is started again when it stops**, under **Settings › General**. Until then, **Restart** warns that Polyfin may not come back, and asks to choose **Restart anyway**; the API refuses a restart that does not say so. Without a restart policy, Polyfin stays stopped until it is started on the host, and the admin app says so after 2 minutes.

### Polyfin Desktop's update

In Polyfin Desktop, the notice of a new version on **Home** offers **Install Polyfin `<version>`** while the app's menu offers it: a release newer than the server, with a package for the system (see [Updates](desktop.md#updates)). It asks the app to do what its menu item does: download the package and check it, stop Polyfin, install the new version and start it. The notice shows the download's progress, then the admin app waits for the new version to answer. When the install fails, Polyfin runs on as it was, and the admin app says so. Elsewhere than in Polyfin Desktop, this is never offered.

### Update now

A container cannot update its own image without the Docker socket. An update tool that you run beside Polyfin can, and many offer an HTTP API that updates the containers it watches when asked. Give its address under **Settings › General**, and **Update now** shows under **Server state**. Polyfin sends this request, and nothing else:

- method `POST`, with no body;
- to the address exactly as given, path and query included, such as `http://updater:8080/v1/update`: match it to your tool's update endpoint;
- with the header `Authorization: Bearer <token>` when a token is set, and `User-Agent: Polyfin/<version>`;
- it gives up after 10 seconds to connect, and 50 seconds for the answer, and follows no redirect.

The admin app shows what the tool answered, its HTTP status and the start of its text, whatever the status, or that it could not be reached or did not answer in time. A tool that answers only once it has updated may replace Polyfin's container before it answers: the admin app then waits for Polyfin to answer again, and reloads. The token is stored encrypted, as the other secrets, never sent back to the admin app, and shown again only on demand (see [Showing a saved key](#showing-a-saved-key)).

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Polyfin is started again when it stops** | **Settings › General** | Off | Confirms that a restart policy or a service manager starts Polyfin again: **Restart** then warns no more. Not shown in Polyfin Desktop, which always does. |
| **Update tool address** | **Settings › General** | Empty | The update API **Update now** sends its request to. Empty hides **Update now**. |
| **Update tool token** | **Settings › General** | Empty | The bearer token sent with the request, when the tool asks for one. |

### Compared with Jellyfin

Jellyfin's dashboard has "Restart" and "Shut down" buttons, through `POST /System/Restart`, allowed to administrators and to any user on the local network ([SystemController.cs](https://github.com/jellyfin/jellyfin/blob/v12.3/Jellyfin.Api/Controllers/SystemController.cs#L114-L122)), and `POST /System/Shutdown`, to administrators ([SystemController.cs](https://github.com/jellyfin/jellyfin/blob/v12.3/Jellyfin.Api/Controllers/SystemController.cs#L130-L138)). Jellyfin restarts within its own process: it stops its host and starts a new one in a loop ([SystemManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.3/Emby.Server.Implementations/SystemManager.cs#L126-L139), [Program.cs](https://github.com/jellyfin/jellyfin/blob/v12.3/Jellyfin.Server/Program.cs#L145-L163)). Polyfin differs on purpose:

- it exits and lets what runs it start it again, which also recovers a server stuck in a way a restart within the process would not;
- it has no shutdown;
- its actions are the admin app's, for administrators only, never for a local address, and each asks for the password or a two-step code again;
- `/System/Restart` and `/System/Shutdown` are not offered to Jellyfin apps.

## Local network discovery

Jellyfin apps look for servers on the local network by broadcasting `who is JellyfinServer?` to UDP port 7359, and Emby apps `who is EmbyServer?`. While **Answer local network discovery** is on, Polyfin answers them with its address, ID and name, and the apps list it among the servers found (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)). It answers only addresses of the local network: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and the server itself. A port forwarded from the internet therefore gets no answer.

The address in the answers is **Address on the local network**, when set. Otherwise it is the address of the server's network interface that reaches the app, with Polyfin's HTTP port, or the address `POLYFIN_LISTEN` names when it names one. In Docker, it depends on the container's network: see [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network).

Both settings apply at once, without a restart. Turned off, Polyfin frees the port. When it cannot listen on the port, Polyfin starts anyway, logs it, and **System › Health** shows a warning; turning the setting off ends the warning, and saving the settings tries the port again. In a container, without an address set, **System › Health** also warns when an answer gave an address that is not on the network of the app that asked: the container's own, on Docker's bridge network.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Answer local network discovery** | **Settings › General** | On | Answers the discovery messages Jellyfin and Emby apps broadcast to UDP port 7359. Off, the port is free. |
| **Address on the local network** | **Settings › General** | Empty | The address announced, such as `http://192.168.1.10:8096`, as Jellyfin's `PublishedServerUrl`. It is also the `LocalAddress` of `/System/Info/Public`. Empty, the address of the interface that reaches the app is announced. |

**For app developers:** `GET /admin/api/health/problems` lists the warning of a port it cannot listen on as `discovery_port`, with `port`, and the warning of a container's address as `discovery_address`, with `address`, the address announced. `/admin/api/settings` holds the settings as `localDiscovery` and `localAddress`; a `PUT` whose address is not an `http` or `https` URL, or has a user, query or fragment, answers 400 `invalid_local_address`. For tests, `POLYFIN_DISCOVERY_PORT` changes the UDP port, which Jellyfin apps never use.

## HTTPS

Polyfin can serve HTTPS itself, without a reverse proxy, on a port of its own next to HTTP. It is set under **Settings › Security**, in its **HTTPS** part, and every change applies at once, without a restart. HTTP keeps `POLYFIN_LISTEN` and keeps working whatever happens to HTTPS: a port HTTPS cannot listen on, files it cannot read and renewals that fail are told in **System › Health**, and never stop Polyfin from starting.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Serve HTTPS** | **Settings › Security** | Off | **Certificate files**: a certificate another tool obtains. **Let's Encrypt**: a certificate Polyfin obtains and renews itself. |
| **HTTPS port** | **Settings › Security** | 8920 | The TCP port HTTPS listens on in the container, as Jellyfin's. Publish it, as 8096 is for HTTP (see [Serve HTTPS](installation.md#serve-https)). |
| **Redirect HTTP to HTTPS** | **Settings › Security** | Off | Sends the requests of clients on the internet that come over HTTP to HTTPS (see [The redirect](#the-redirect)). |
| **Certificate**, **Private key** | **Settings › Security**, with **Certificate files** | Empty | The paths in the container of PEM files: the certificate with its chain, such as `fullchain.pem`, and its RSA or ECDSA key, such as `privkey.pem`. |
| **Host names** | **Settings › Security**, with **Let's Encrypt** | Empty | The public names that point at the server, one a line, such as `media.example.org`, up to 100, all in one certificate. |
| **Email** | **Settings › Security**, with **Let's Encrypt** | Empty | Optional: Let's Encrypt may write to it about the account. |
| **I agree to Let's Encrypt's terms of service** | **Settings › Security**, with **Let's Encrypt** | Off | Required: Let's Encrypt issues certificates only once its terms, at [letsencrypt.org/repository](https://letsencrypt.org/repository/), are agreed to. |
| **Challenge** | **Settings › Security**, with **Let's Encrypt** | HTTP-01 | How Let's Encrypt checks that the names point at the server (see [Let's Encrypt](#let-s-encrypt)). |

The container's user, UID 65532, cannot listen below 1024: to reach Polyfin on the usual ports, 443 for HTTPS and 80 for Let's Encrypt's HTTP-01, forward them to Polyfin's ports on the router, or in the container's `ports`. When 443 reaches the HTTPS port, set **Public address**, under **Settings › Notifications**, to `https://your-host`, without a port: [the redirect](#the-redirect) then goes to 443 rather than to the HTTPS port.

### Certificate files

Mount the folder of the files read-only in the container, and give their paths. Saving checks that both can be read, by user 65532, and that the key belongs to the certificate: files that are missing, unreadable or mismatched are refused, and nothing is saved. Saving other settings with the same files checks nothing.

Polyfin looks at the files every minute, and reads them again when they change: a certificate another tool renews, such as Certbot, acme.sh or a reverse proxy's, is served within a minute, without a restart. While a change cannot be read, missing or mismatched, for instance between the writes of the certificate and of its key, the last good certificate stays in use, and **System › Health** warns.

### Let's Encrypt

Polyfin obtains one certificate for every host name, with an account key of its own, and keeps the account key, the certificate and its private key in the database, the keys encrypted with `POLYFIN_SECRET_KEY` as the other [stored keys](configuration.md#stored-keys-and-tokens), so that backups hold them, with the challenge and the ACME server it came from. It renews the certificate once a third of its lifetime remains: 30 days before the end of a 90-day certificate. Changing the host names or the challenge, or starting with another `POLYFIN_ACME_DIRECTORY`, obtains a new one at once. A failed attempt is tried again an hour later, or at once when the host names, email or challenge change; meanwhile the certificate it had stays in use. An attempt that Polyfin stopping interrupts is no failure: it is made again at the next start. Polyfin waits for the attempt to end as it stops, so that a certificate obtained at that moment is kept before Polyfin stops, never while a backup is restored.

Each name must point at the server from the internet, and one of two ports must reach Polyfin:

- **HTTP-01**, the default: Let's Encrypt asks `http://<name>/.well-known/acme-challenge/` on port 80, which must reach Polyfin's HTTP port, 8096 by default. Polyfin answers it there, and never redirects it.
- **TLS-ALPN-01**: Let's Encrypt connects to port 443, which must reach Polyfin's HTTPS port, 8920 by default.

Names that resolve only on the local network, and wildcards such as `*.example.org`, need the DNS-01 challenge, which Polyfin does not answer: it would issue certificates without any open port, but needs Polyfin to create a TXT record under `_acme-challenge.<name>`, through each DNS host's own API, with credentials stored encrypted, or through RFC 2136 dynamic updates for the DNS servers that accept them. Meanwhile, obtain such a certificate with another tool that answers DNS-01, such as Certbot, acme.sh or lego, and give its files under **Certificate files**.

With **Let's Encrypt**, while **Public address**, under **Settings › Notifications**, is empty, it offers `https://` and the first host name: without a port, as the internet reaches it once 443 is forwarded to the HTTPS port, and with the HTTPS port when it is not 443. Links in notifications then use HTTPS, and the redirect goes to the port of the address chosen.

**For tests:** `POLYFIN_ACME_DIRECTORY` names another ACME server, such as Let's Encrypt's staging environment, a private ACME server, or a test server such as Pebble, and `POLYFIN_ACME_CA_FILE` the certificate authorities its HTTPS is trusted with (see [Configuration](configuration.md)).

### The redirect

While **Redirect HTTP to HTTPS** is on and HTTPS is served, a request from the internet that comes over HTTP gets a 308 to the same path and query on `https://`, the host it asked for and the port the internet reaches HTTPS at: a 308 keeps the method and body, so sign-ins follow it too. That port is the one of **Public address**, under **Settings › Notifications**, when it starts with `https://`, 443 when it names none, such as `https://media.example.org`; otherwise it is the HTTPS port. With the router's 443 forwarded to the HTTPS port, 8920, set **Public address** to `https://your-host`: without it, clients would be sent to port 8920, which the router does not open. These are never redirected:

- requests from the local network: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback, as [Local network discovery](#local-network-discovery) reads them, behind a known proxy the client it names;
- Let's Encrypt's HTTP-01 checks, under `/.well-known/acme-challenge/`;
- requests from a known proxy that sends `X-Forwarded-Proto: https`: the proxy received them over HTTPS (see [Reverse proxy](reverse-proxy.md#https-and-the-public-address)).

While HTTPS is not served, its port taken or no certificate read yet, nothing is redirected.

### What apps see

Over HTTPS, the stream addresses Polyfin gives apps start with `https://`, and the admin app's session cookie is marked secure, as behind a proxy that sends `X-Forwarded-Proto: https`. Without **Address on the local network**, `LocalAddress` of `/System/Info/Public` and `/System/Info` stays the address the request reached, `https://` and the HTTPS port included. Discovery answers keep the HTTP address: a certificate for a public name does not match `192.168.1.10`, and HTTP is never redirected on the local network. To announce HTTPS on the local network, set **Address on the local network** to an `https://` address whose name the certificate holds (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)).

### HTTPS problems

**System › Health** warns, with a link to the setting, of a port HTTPS cannot listen on, used by another program or below 1024, of certificate files that are missing, unreadable or do not belong together, of an attempt with Let's Encrypt that failed, with Let's Encrypt's error, and of a certificate that expires within 14 days; an expired certificate is an error. They reach notification targets as health problems (see [Notifications](notifications.md)).

**For app developers:** `/admin/api/settings` holds the settings as `httpsMode` (`off`, `files` or `letsencrypt`), `httpsPort`, `httpsRedirect`, `httpsCertificate`, `httpsPrivateKey`, `acmeHosts`, `acmeEmail`, `acmeTerms` and `acmeChallenge` (`http-01` or `tls-alpn-01`). A `PUT` with files that cannot be served answers 400 `https_certificate_missing`, `https_private_key_missing`, `https_certificate_unreadable`, `https_private_key_unreadable` or `https_files_mismatch`; Let's Encrypt without a host name answers `invalid_acme_hosts`, and without the terms agreed to `acme_terms_required`. `GET /admin/api/health/problems` lists `https_port`, with `port`, `https_files`, with `failure` (`missing`, `unreadable` or `mismatch`) and `file`, `https_renewal`, with `error`, and `https_expiry`, with `expires`.

## Logs

**System › Logs** follows the in-memory log, redacted as it is for Jellyfin apps (see [Server logs](#server-logs)). It has a **Level** filter, a search box, **Follow** and **Pause**, and **Download**.

A listing of a library or a collection that takes more than 5 seconds logs "A listing was slow" (Info), with the folder, whether the app asked for every title, how many it got, and where the time went:

- `checked`: the user and the request;
- `read`: the addons and the database;
- `described`: the titles for the app.

## Metrics

Polyfin can expose its metrics to Prometheus, in its text format, at `/metrics`. Turn **Expose metrics** on under **Settings › General**: Polyfin then makes the **Metrics token**, shown under it as saved keys are (see [Showing a saved key](#showing-a-saved-key)). Prometheus sends the token as a bearer token with every request.

- Off, the default, `/metrics` answers 404.
- Without the token, or with another, it answers 401.
- **Make a new token**, beside the token, asks first, then makes another: the former stops working at once. The activity log records who made it, never the token.
- The token is stored encrypted with `POLYFIN_SECRET_KEY` when it is set, as the other saved keys are. Turned off, metrics keep their token for when they are turned on again.

The metrics name no user, title, address, device or token. Their labels take a few fixed values, so that the series stay few; addons are named as they are listed under **Content › Sources**, and addons with the same name are counted together.

| Metric | Type | Labels | What it counts |
|---|---|---|---|
| `polyfin_playbacks` | gauge | `kind`: `video`, `live_tv`, `music`, `audiobook`; `delivery`: `direct_play`, `remux`, `conversion` | Playbacks under way, as **Home** shows them. |
| `polyfin_conversions` | gauge | `method`: `nvidia`, `vaapi`, `videotoolbox`, `qsv`, `amf`, `processor` | Conversions under way, of files and channels: those that convert the video. |
| `polyfin_encoding_failures_total` | counter | `method`: as above, or `copy` for a remux | Remuxes and conversions FFmpeg failed. |
| `polyfin_segments_total` | counter | `method`: as above | HLS segments made of files. |
| `polyfin_addon_request_duration_seconds` | histogram | `addon`; `resource`: `manifest`, `catalog`, `meta`, `stream`, `subtitles`, `other`; `outcome`: `success`, `failure` | Requests to addons and how long their answers took. An answer, even "not found", is a success, as on **System › Health**. |
| `polyfin_imports_total` | counter | `service`: the app (`stremio`, `nuvio`), the tracking service (`trakt`, `simkl`, …) or `letterboxd`; `outcome` | Imports of app connections, of watch histories and of Letterboxd exports. One that could not read the account whole counts as a failure. |
| `polyfin_tracking_sent_total` | counter | `service`; `outcome` | What was sent to tracking services, such as a title watched, each try counted. |
| `polyfin_notifications_total` | counter | `target`: `webhook`, `discord`, `ntfy`, `email`, `telegram`, `gotify`, `pushover`; `outcome` | Notifications sent, each try counted. |
| `polyfin_http_request_duration_seconds` | histogram | `api`: `jellyfin`, `admin` (the admin app and its API), `web` (the web client), `other` (`/healthz`, `/readyz`, `/metrics`); `status`: `1xx` to `5xx` | HTTP requests and how long they took, a stream's until it ended. Requests under `/jellyfin` count as at the root: the web client's as `web`, the rest as `jellyfin`, the admin app and monitoring routes included, since they answer at the root only. |
| `polyfin_scans_running` | gauge | | Scans of local folders under way. |
| `polyfin_scan_duration_seconds` | histogram | | Scans of local folders and how long they took. |
| `polyfin_database_connections` | gauge | `state`: `in_use`, `idle`, `max` | Connections to PostgreSQL. |
| `polyfin_disk_free_bytes` | gauge | `folder`: `cache`, `conversion`, `reading` (files being read), `recordings`, `backups` | Space free on the disks Polyfin writes to, those in use, as **System › Health** checks them: `conversion` and `reading` when they are other folders than the cache's. |

Prometheus' own metrics of the process follow: memory, goroutines and garbage collection (`go_*`), and on Linux processor time, memory and open files (`process_*`).

A scrape configuration, the token in a file Prometheus reads:

```yaml
scrape_configs:
  - job_name: polyfin
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/polyfin-token
    static_configs:
      - targets: ["polyfin:8096"]
```

`credentials: <the token>` puts it in the configuration itself instead. `/healthz` and `/readyz` tell whether Polyfin answers and reaches PostgreSQL (see [Health check](installation.md#health-check)).

**For app developers:** `/admin/api/settings` holds the setting as `metrics`, and `metricsTokenSet` tells whether the token was made; no answer holds the token. `POST /admin/api/settings/secrets/metricsToken/reveal` answers it, and `POST /admin/api/settings/metrics-token` makes a new one and answers the settings.

## Settings

**Settings** has one page per section, listed on its left, with a search box over all of them (**Search a setting**). Each section page saves only its own changes. Leaving a page with unsaved changes asks first. **Settings › Diagnostics** ends with the `POLYFIN_*` variables in effect, read only:

- secrets are hidden;
- the database URL shows its host and database only.

A **Web player** link appears when the server serves a web client. See [configuration](configuration.md) for the variables and [web client](web-client.md) for **Settings › Web player**.

## Showing a saved key

Fields that hold a key or a secret hide it. While you type one, an eye button (**Show key**, **Hide key**) shows what you typed. Once saved, the key shows as dots, with its own eye:

- Under **Settings**, administrators can show the server's **PublicMetaDB key**, **TheIntroDB key**, **Trakt client secret**, **Last.fm shared secret** and **Metrics token**.
- Under **My account › Tracking**, each user can show their own **MDBList** and **PublicMetaDB** API keys and their **ListenBrainz user token**. Trakt, Simkl and Last.fm hold tokens or session keys rather than keys, which are never shown.

The key is fetched from the server when the eye is clicked, and hidden again on a second click, after a minute, or when you leave the page. Each time, the activity log records who showed which key, never the key itself, and the answer is never cached. A key that cannot be decrypted with `POLYFIN_SECRET_KEY` counts as not saved and cannot be shown.

## API keys and integrations

### API keys

**API keys** let tools such as request managers use the Jellyfin API, as Jellyfin's keys do. You create them under **System › API keys** (**Create a key**), or in jellyfin-web's dashboard. **Revoke** asks first.

- A key has administrator rights and no user.
- Requests that name a user act for that user. That user's parental control, blocked genres, visible libraries and allowed hours apply.
- Requests that name no user see the server's libraries.
- Polyfin stores only a hash of each key, with its app name, creation date and last use. The admin app shows a key once.
- A key cannot open a WebSocket.

**For app developers:**

- Keys are managed through `/Auth/Keys`.
- Send the key as the token of the `Authorization` header, or as the `ApiKey` query parameter. `api_key` and `X-Emby-Token` work only with legacy authorization.
- A user is named with `userId` or `/Users/{id}/…`. Without one, `/Users/Me` answers 400, as in Jellyfin.
- `/Auth/Keys` lists a key made from a Jellyfin app in full only on the next listing, then its identifier. `DELETE /Auth/Keys/{key}` accepts the identifier as well as the key.

### Devices

Administrators get Jellyfin's device list, with custom names. Deleting a device signs out every user on it.

**For app developers:** devices are at `/Devices`.

### Users from jellyfin-web

Administrators can create, rename and delete users from jellyfin-web, with Polyfin's rules:

- a password of at least 8 characters is required, unlike Jellyfin;
- the last administrator stays.

See [users](users.md).

### Server configuration

Jellyfin apps can read and save the server configuration. It answers Polyfin's server name, language, resume thresholds, Quick Connect and legacy authorization, with Jellyfin's defaults for the rest. Saving applies those fields only, and keeps those left out.

**For app developers:**

- Configuration: `/System/Configuration`.
- Libraries: `/Library/MediaFolders`.
- Storage: `/System/Info/Storage` lists the cache folder, the conversion folder's segments as the transcoding folder, and, while recording is on, the recordings folder.
- The authentication providers are available too.

### Activity log

The activity log records:

- sign-ins and failed sign-ins, wrong codes of two-step sign-in included, and sign-ins with a recovery code;
- two-step sign-in turned on or off, and by which administrator for another user;
- playback start and stop;
- users created, changed or deleted, and accounts that reach their end date (see [Ending an account](users.md#ending-an-account));
- user groups created, changed, naming the sections changed, as "Group Kids was changed: Access, Allowed hours", or deleted, and members added, removed or no longer following their group, as "sam no longer follows the group Kids: changed from Jellyfin Web" (see [User groups](users.md#user-groups));
- the settings of a user or a group reset to their defaults, naming the administrator, as "alex reset the settings of sam to their defaults" (see [Resetting settings to their defaults](users.md#resetting-settings-to-their-defaults));
- settings saved;
- addons installed or removed;
- share links made, revoked or expired, and each guest playback through one, as "A guest played Example Movie (2008) through the link shared by alex" (see [Share links](users.md#share-links)).

It keeps 30 days. Its last entries show on the admin app's [**Home**](#home), and Jellyfin apps can read it.

### Server logs

The server log is kept in memory. Apps can download its last 5 MB as one file, `polyfin.log`. URLs are reduced to their host, and tokens, keys and passwords are replaced. Logs that apps upload go to the server log, redacted the same way, up to 1 MB each.

**For app developers:**

- Server log: `/System/Logs`.
- Uploaded app logs: `/ClientLog/Document`, 1 MB at most.

### Scheduled tasks

Jellyfin apps see Polyfin's own jobs and can start and stop them:

- signing out unused devices, hourly;
- cleaning the activity log, daily;
- recording ended accounts in the activity log, hourly;
- recording expired share links in the activity log, every 10 minutes, and deleting the links that ended more than 30 days ago;
- fetching the IPTV channel lists, then the Live TV guides due under **Settings › Live TV**. Polyfin looks for due ones every 5 minutes; started by hand, the task fetches every list and guide;
- refreshing ratings, which asks addons and runs only when started.

Their schedules cannot be changed.

**For app developers:** tasks are at `/ScheduledTasks`; changing a schedule answers 400.

### Forgotten password

1. A user on the local network asks for a PIN from the app's sign-in screen.
2. The PIN, valid 30 minutes, is logged and shown on the user's page under **Users**, as there is no server file to read it from.
3. Entering it in the app makes it the user's password and signs them out everywhere. It resets the password only: an account with [two-step sign-in](users.md#two-step-sign-in) keeps it, and still signs in to apps with Quick Connect.

Wrong PINs count as failed sign-ins. Asked from outside the local network, the answer is the same and no PIN is made, as in Jellyfin. Behind a reverse proxy on the local network, list it under **Known proxies**: otherwise every request counts as coming from it, and so from the local network (see [Known proxies](reverse-proxy.md#known-proxies)).
