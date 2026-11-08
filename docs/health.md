# Health

This page is for administrators. It covers what **System › Health** shows, each check it runs, and each warning and error it raises, in the order the page shows them.

## What Health shows

**System › Health** puts problems first, under **To look at**: each one is a tile, marked **Error** or **Warning**, with a link to what fixes it. A new version of Polyfin is a tile there too, when one is out (see [New versions](updates-and-restarts.md#new-versions)). The problems update every 10 seconds, the rest of the page every 30.

Under **Checks**, a tile for each main part tells its state at a glance: **Addons**, **IPTV sources**, **Programme guides**, **Database**, **Transcoder**, **Backups**, **Source cache** and **Stored keys**. The other parts are rows under **Also on this page**, each with its value, when they apply. A tile or a row opens its details over the page, and has its own address, such as `/admin/system/health#process`, which the problems and notifications link to.

- **Addons**, from the requests made for apps since start: last answer, last failure and response time.
  - **Check**, in its details, asks an addon for its manifest, at most once a minute. For a member's own addon, it reaches only public addresses.
  - An addon whose server asked to wait for its catalog pages shows "asked to slow down (too many requests)" until the wait ends, whatever it answered meanwhile (see [Addons that ask to wait](addons.md#addons-that-ask-to-wait)).
- **IPTV sources** and **Programme guides**.
- **Network shares**, when there are some. Each is **Notified** of its changes by its server, **Polled** by listing its folders again, **At scans only**, or **Not watched**, with why. Polling runs every 2 minutes by default, 30 for S3 buckets, as each folder's settings ask (see [Local folders](local-folders.md#watching-for-changes)).
- The server's parts below.

| Part | What its details show |
|---|---|
| **Transcoder** | GPU, encoders, and conversions against the limit (see [Transcoding](transcoding.md)) |
| **Database** | The database and its size |
| **Source cache** | Use against **Disk space for files being read (GB)**, how long it keeps parts, the files open and kept, the share of the bytes read since the start that came from the disk, and the part torrent pieces take (see [Keeping the files read across restarts](reading-the-sources.md#keeping-the-files-read-across-restarts)) |
| **Image cache** | Use against its 1 GB on disk, the images it keeps there and in memory, and **Clear the image cache** (see [Clearing the image cache](#clearing-the-image-cache)) |
| **Torrents** | While they are on: the route, the torrents playing against **Torrents at once (0 = no limit)** when it is set, their peers and those reached over uTP, what was shared, and the route's outages the plays went on through in the last 15 minutes, as "Outage at 21:04: the proxy did not answer for 16 s; the play went on" (see [Torrents](torrents.md)) |
| **Disk space** | The cache, recordings and backups folders, and the conversion folder and the folder for files being read when they are other folders than the cache's (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)) |
| **Backups** | The last run and its result, the last backup made with its file and size, the next one, each destination's last upload, and the last restore (see [Backups](backups.md)) |
| **Thumbnails (trickplay)** | The queue and paused sources |
| **Artwork** | The artwork downloads that failed in the last hour, by kind and by server, and the servers waited for (see [Artwork and people](#artwork-and-people)) |
| **People** | Whether a TMDB API key is saved, and how people's lookups on TMDB stand (see [Artwork and people](#artwork-and-people)) |
| **Polyfin** | Memory, goroutines, uptime and version, and **Download the goroutines** (below) |
| **Stored keys** | Whether `POLYFIN_SECRET_KEY` encrypts them, how many are stored unencrypted, and which cannot be decrypted with it. Unencrypted keys are a warning; keys that cannot be decrypted are an error (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)) |
| **Known proxies** | While some are listed by host name, the addresses each name resolves to (see [Proxies by name](reverse-proxy.md#proxies-by-name)) |

**Backups** problems:

- a failed backup, or a last backup older than two days, is a problem;
- a destination whose last upload failed is an error;
- a destination whose newest backup is older than two days is a warning.

**Download the goroutines** gives a text file of what every goroutine of the server is doing. It holds each one's stack and how long it has waited, as Go prints it when a program crashes.

- When a request never answers, download it while the request still waits, and send it with the report.
- It holds the paths of Polyfin's source files, but no address, token or password.
- Only administrators may download it.

## What Health covers

Health sends no request outside the server, except to the SOCKS5 proxy and the WireGuard server of torrents. It checks them as their kill switch does.

Like Schedule, it covers the server's addons, IPTV sources and guides first. Those users keep under **My sources** follow, each marked with its owner, and their problems count in the summary.

The problems it finds, and those solved, can be sent to webhooks, Discord, ntfy, email, Telegram, Gotify or Pushover (see [Notifications](notifications.md)).

## Warnings and errors

Each problem below shows under **To look at**, most with a link to the setting or page that fixes it.

### Proxies

Both are warnings, with a link to the setting (see [Known proxies](reverse-proxy.md#known-proxies)):

- A local network address sends `X-Forwarded-For` or `X-Real-IP` without being one of the **Known proxies**. It is usually a reverse proxy missing from the list. The address is named for an hour after its last such request.
- A host name of **Known proxies** resolves to nothing, so it trusts nothing.

### Local network discovery problems

Two warnings, linked to its settings (see [Local network discovery](https-and-networking.md#local-network-discovery)):

- its UDP port is used by another program, such as a Jellyfin server on the same host;
- its answers give an address of Polyfin's container, which apps on the local network cannot reach.

### HTTPS problems on Health

Four problems, linked to its settings (see [HTTPS problems](https-and-networking.md#https-problems)):

- a port it cannot listen on;
- certificate files that cannot be served;
- a failed attempt with Let's Encrypt;
- a certificate that expires within 14 days, an error once it expired.

### Picture subtitles without Tesseract data

A picture subtitle track could not be read into text, as its language has no Tesseract data. This is a warning, one per language (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).

- It names the language and the `.traineddata` file to put in the folder `POLYFIN_TESSDATA_DIR` names.
- While that folder is set and Polyfin carries the language's file in its list, the warning has **Download**. It downloads the file there and checks its SHA-256.
- Otherwise it links to **Turn picture subtitles into text**.
- It goes away once the file is there, and the tracks waiting for it are then read.
- A failed download names why: the folder cannot be written, the download's error, or a SHA-256 that differs. A file whose SHA-256 differs is not kept.
- The warning then tells why its last download failed, the automatic one included.

### Metadata addons all with a language

Every metadata addon of the server has a language. This is a warning naming the first one, with a link to its page (see [Languages](addons.md#languages)).

- They then describe titles in their language for every user, whatever the user's metadata language.
- Keep **Language of its descriptions** at **None** on that addon, or on one more metadata addon.

### Old NVIDIA driver

An NVIDIA GPU's driver is older than the version FFmpeg needs for NVENC. This is a warning naming that version, with a link to **Transcoder** (see [NVIDIA driver](gpus-and-hdr.md#nvidia-driver)).

- Video is converted on another GPU or on the processor until the driver is updated.
- With a driver from 570 to 609, Polyfin downloads an FFmpeg built for it instead.
- A warning tells, with why, when that FFmpeg could not be downloaded, or when NVENC fails with it too.
- Polyfin tries the download again at its next start, or when the conversion settings are saved.

### Failed searches in a local folder

A local folder's files are left unmatched because the search of their titles failed. This is a warning naming the addon that failed most of them and why, with a link to the folder's page. It goes away once the searches go through, which Polyfin retries on its own (see [Failed searches](unmatched-files.md#failed-searches)).

### Search catalog too slow

An addon's search catalog did not answer a search in time. This is a warning, with a link to **Addons** (see [Catalog pages](addons-and-libraries.md#catalog-pages)).

- Searches leave the addon out for 5 minutes, then ask it again.
- It comes and goes with the searches, so it is not sent as a health problem. The addon's own failure, "did not answer in time", is.

### Addon not answering in time

An addon left 3 requests for versions in a row unanswered within 30 seconds, so Polyfin's background requests to its server pause (see [Addons that do not answer in time](addons.md#addons-that-do-not-answer-in-time)). This is a warning naming the addon and when the pause ends, with a link to its page for a server's addon.

- Plays and opened titles still ask it.
- It is sent as a health problem: the addon "did not answer in time", the same problem as its failure once the pause ends.
- **What to do:** if the addon gathers other addons' versions, turn off there the one that no longer answers, or shorten how long it waits for them. Otherwise, turn the addon off.

### Smart list without catalogs

Every catalog a smart list drew from was removed with its addon, or dropped by it, so the list lists nothing. This is a warning naming the list, and its owner for a user's own list (see [Removing a source](addons.md#removing-a-source)). It links to the list's editor for a server's list, and to the owner's page for a user's own.

- The list is kept, and its editor shows the same until other catalogs are chosen there (see [Smart lists](smart-lists-and-search.md#smart-lists)).
- It goes away once the list draws from a catalog again.

### Channel airing a removed source

A channel of your titles airs a collection, smart list, series or titles that no longer exist. This is a warning naming the channel, with a link to its page (see [Channels of your titles](live-tv.md#channels-of-your-titles)).

- It goes away once another source is chosen for the channel.

### No addon for a local folder

A local folder's files are left unmatched because no addon of the server describes and searches titles of its kind. This is a warning, with a link to **Content › Sources**, where such an addon is added (see [What a search needs](naming-and-matching.md#what-a-search-needs)).

- Users' own addons, under **My sources**, do not count.
- It goes away once a scan matches those files.

### Incomplete copy of the Custom theme

The copy of the web player's **Custom** theme kept in Polyfin misses some of its files, as after a backup restored on another system or into another data folder. This is a warning with how many are missing (see [Keeping a copy of a theme](themes-and-custom-code.md#keeping-a-copy-of-a-theme)).

- It shows while the **Custom** theme is chosen and its texts load the copy, and links to the copy under **Settings › Web player**.
- Browsers meanwhile load those files from their original addresses, which needs the internet.
- **Update the copy** downloads them again and clears the warning. So does choosing another theme.

### Polyfin Desktop went back a version

A new version's server ended 3 times before it answered, so Polyfin Desktop returned to the previous version. This is a warning (see [Going back to the previous version](desktop.md#going-back-to-the-previous-version)).

- It names the version that did not start, the one put back, and why, as the app's log tells it.
- It links to the server's version, in the details of **Polyfin**.
- It lasts while a version older than the one that did not start runs, and goes once a newer version answers.

### Unwritable folder

Polyfin could not write into the **Conversion folder**, **Folder for files being read** or **Cache folder** when it started, such as when a disk is not mounted. This is a warning naming the folder, with a link to its setting. The data folder's `cache` holds those files until a start finds the folder writable (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)).

### Stopped connection

A user's app connection or tracking service stopped, its app or service having refused what Polyfin kept. This is a warning, one per user and app or service, which opens the user's page (see [Stopped connections](notifications.md#stopped-connections)).

- Only that user can connect it again.
- It is not sent as a health problem: the **Connection stopped** notification told it.

### Torrent route down

Torrent streams cannot play. This is an error, naming why, with a link to **Settings › Torrents** (see [Kill switch](torrents.md#kill-switch)):

- the network interface is missing, down or without an address;
- the SOCKS5 proxy does not answer, or refuses its user;
- the WireGuard server does not answer the handshake.

While torrents play through such an outage, the error says so instead, as "The SOCKS5 proxy 192.168.1.5:1080 does not answer. The torrents playing go on for up to 60 s; new ones cannot start". With none playing, it says torrent streams do not play.

### Proxy without UDP

A proxy without `UDP ASSOCIATE` is a warning, with a link to **Settings › Torrents**. Torrents play over TCP, with UDP trackers and the DHT off.

### Torrent stopped while playing

A torrent's route gave no answer, and its peers no byte, for a minute. This is a warning for 15 minutes that tells when and why, as "A torrent stopped while playing at 21:04: the SOCKS5 proxy 192.168.1.5:1080 did not answer" (see [Kill switch](torrents.md#kill-switch)).

- It stays apart from a torrent that did not start in time, which tells of its swarm.
- It does not show while the route still does not answer: the route error tells it then.
- A short outage the plays went on through is no problem: it shows under **Torrents**.

### Host answering an error file

A host answers the same small file, such as an error video, for several different files, each far larger, as a provider's host does while its service is down. This is a warning naming the host, never the link, and since when, as "The host files.example answers an error file instead of the videos, since 20:40" (see [Hosts answering an error file](reading-the-sources.md#hosts-answering-an-error-file)).

- It shows from the third file answered with that size within 10 minutes. One file answered with another size is no problem.
- It goes as soon as the host answers a file of its own size, and the versions refused meanwhile are tried again at once.
- **Content › Versions** shows it too, on the versions from that host.

API details: [Health problems](jellyfin-api.md#health-problems-in-the-admin-api).

## Artwork and people

**Artwork**, a row under **Also on this page** in **System › Health**, tells why images stay blank in apps. It needs no **Detailed log (to diagnose a problem)**. Its details show:

- the artwork downloads that failed in the last hour, by kind: **Not found** (`404` or `410`), **Asked to slow down** (`429`, or a `503` with `Retry-After`), **Timed out**, **Server errors** (other `5xx`) and **Other failures**;
- the servers that failed, by host name, the 10 that failed most first. No address is shown;
- the **Servers waited for**: those that asked to wait, asked nothing until then, and those that showed they were overloaded, asked less at once until then (see [Artwork](artwork-and-editing.md#artwork)).

An hour with failures also logs one INFO line, `Artwork downloads failed in the last hour`. It holds the same counts, the 5 servers that failed most and those waited for.

**People**, a row beside it, tells how the photos and biographies of people from TMDB stand (see [Photos and biographies from TMDB](collections-people-and-ratings.md#photos-and-biographies-from-tmdb)):

- whether a **TMDB API key** is saved under **Settings › Content**;
- the people TMDB found and did not find, and those whose last lookup failed;
- the lookups waiting or under way;
- TMDB's last rate limit (`429`) since the server started, with the wait it asked.

Polyfin bundles no TMDB key, unlike Jellyfin, and asks the administrator for one (see [Edit images](artwork-and-editing.md#edit-images)). With no key saved, **People** lists what a key brings, and links to the key's setting under **Settings › Content**:

- people's photos and biographies;
- each title's cast and crew from TMDB in billing order, with photos (see [Cast and crew from TMDB](collections-people-and-ratings.md#cast-and-crew-from-tmdb));
- person pages listing every library title they are credited in;
- the titles TMDB knows each person best for, on their page, beyond the libraries (see [Titles a person is known for](collections-people-and-ratings.md#titles-a-person-is-known-for));
- posters for movies and series whose addon gives none;
- box sets in Silo's apps (see [Box sets](silo-apps.md#box-sets));
- TMDB images in **Edit images**.

API details: [Artwork and people health](jellyfin-api.md#artwork-and-people-health-in-the-admin-api).

## Clearing the image cache

**Clear the image cache**, in the details of **Image cache** in **System › Health**, empties at once the cache of the images Polyfin serves to apps (see [Artwork](artwork-and-editing.md#artwork)). A message tells how many images and bytes it freed, and the cache's size beside it then shows empty.

It is a button rather than a task under **System › Schedule** for two reasons:

- a task could not show what it freed;
- the cache needs no schedule, as it keeps within its limit on its own.

It empties:

- the artwork relayed from the addons' artwork servers, IPTV channel logos and people's photos included, in memory and in the `images` folder of Polyfin's cache folder;
- the copies resized for the apps, and the posters warmed ahead with their sizes, those of local folders included;
- the images Polyfin draws for collections, smart lists and libraries;
- the downloads remembered as failed, which are asked for again at once rather than once their wait ends;
- the images found dead, which titles and people show again.

After clearing:

- each image is downloaded, resized or drawn again at its next request;
- the posters of the libraries' first pages are warmed again by the next run of "Read the libraries' first pages", within the hour;
- an image being downloaded or resized as the cache is cleared is kept once it ends.

It leaves alone:

- the scrubbing thumbnails and chapter images, kept in the database within **Space for images (GB)** in **Settings › Thumbnails (trickplay)**;
- the images uploaded with **Edit images** or chosen for libraries, kept in the database;
- the artwork files of local folders, which stay beside their files;
- the copy of the web player's theme kept in **Settings › Web player**;
- which titles each generated image shows.

**Compared with Jellyfin**

- Jellyfin's "Clean Cache Directory" task (`DeleteCacheFiles`) runs every 24 hours and deletes the files of its cache folder older than 30 days, its resized images included ([DeleteCacheFileTask.cs, lines 71 to 83](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteCacheFileTask.cs#L71-L83)). Polyfin's image cache keeps within 1 GB instead, the least recently used going first, and is emptied whole by hand.

API details: [Image cache](jellyfin-api.md#image-cache-in-the-admin-api).
