# Configuration

This page is for administrators who run Polyfin and need to know what the environment sets. It covers the environment variables, the data folder, the work folders and how stored secrets are encrypted.

Polyfin is set in the admin app, in the sections of **Settings**, such as **Settings › General**. The environment gives only what Polyfin needs before it can read its settings: where its database is, and the key that encrypts its stored secrets.

## Environment variables

With the Compose file, `POSTGRES_PASSWORD` in `.env` is the only value to set. Compose builds `POLYFIN_DATABASE_URL` from it. `POLYFIN_SECRET_KEY` is optional but advised. The Docker image sets the others.

**Settings › Diagnostics** ends with the `POLYFIN_*` variables in effect, read only (see [Settings](administration.md#settings)). Secrets are hidden there, and the database URL shows its host and database only.

The database and the secret key:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, such as `postgresql://polyfin:password@postgres:5432/polyfin`. |
| `POLYFIN_SECRET_KEY` | (unset) | Key that encrypts the stored secrets: API keys, client secrets, tokens and passwords. See [Stored keys and tokens](#stored-keys-and-tokens). |

- **The database connection:** besides its pool, Polyfin keeps one connection that listens for changes to addons and libraries. It then answers them from memory.
  - A pooler in transaction mode cannot carry that connection. Polyfin then reads addons and libraries from the database for every request.
- **The key's form:** 32 random bytes in base64, made with `openssl rand -base64 32`, or 64 hexadecimal digits.
  - Unset, the secrets stay unencrypted, and **System › Health** warns about it.
  - A malformed key stops Polyfin from starting.

Network addresses and ports:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_LISTEN` | `:8096` | HTTP address, as host:port. 8096 is the port Jellyfin clients try by default. |
| `POLYFIN_DISCOVERY_PORT` | `7359` | UDP port of [local network discovery](https-and-networking.md#local-network-discovery). |
| `POLYFIN_DLNA_PORT` | `1900` | UDP port of SSDP, which [DLNA](dlna.md) devices search for servers on. |
| `POLYFIN_HDHOMERUN_PORT` | `65001` | UDP port **Find tuners** asks [HDHomeRun tuners](iptv.md#hdhomerun-tuners) to answer on. |
| `POLYFIN_HDHOMERUN_ADDRESSES` | (unset) | Addresses **Find tuners** asks instead of broadcasting on the local networks, separated by commas, such as `192.168.1.20,192.168.1.21`. |

- **HTTPS:** when served, it listens on the same host as `POLYFIN_LISTEN`, on the **HTTPS port** of **Settings › Security**, 8920 by default (see [HTTPS](https-and-networking.md#https)).
- **The three UDP ports:** Jellyfin apps only send to 7359, devices only use 1900, and tuners only answer on 65001. Another port serves tests.
- **Tuner addresses:** they suit a container whose broadcasts do not reach the tuners (see [Find HDHomeRun tuners](installation.md#find-hdhomerun-tuners)).
  - An address without a port is asked on `POLYFIN_HDHOMERUN_PORT`.
  - A value that is not a host or host:port stops Polyfin from starting.
- **Values that stop Polyfin from starting:** a `POLYFIN_LISTEN` that is not host:port with a port from 1 to 65535, and a port variable that is not a port from 1 to 65535.

Programs Polyfin runs:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_FFPROBE` | `ffprobe` | ffprobe executable (FFmpeg 9.0, see below). |
| `POLYFIN_FFMPEG` | `ffmpeg` | FFmpeg executable (9.0, see below). |
| `POLYFIN_TESSERACT` | `tesseract`; `/usr/bin/tesseract` in the Docker image | Tesseract executable (5, see below), which reads picture subtitles into text while **Turn picture subtitles into text** is on. |
| `POLYFIN_TESSDATA_DIR` | (unset); `/data/tessdata` in the Docker image | Folder of Tesseract `.traineddata` files for languages Tesseract's own data lacks, such as `jpn.traineddata` for Japanese. |
| `POLYFIN_TESSDATA_URL` | `https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/<commit>/`: Tesseract's own data, at the commit Polyfin pins | Address the `.traineddata` files are downloaded from into `POLYFIN_TESSDATA_DIR`, followed by their name, such as `fin.traineddata`. |

- **Executables:** each is a path, or a name looked up in `PATH`. The Docker image includes all three.
- **FFmpeg's version:** 9.0 is the version the Docker image and Polyfin Desktop ship, and the one the tests run with. Older versions are not tested, and Polyfin does not check the version (see [GPUs and HDR](gpus-and-hdr.md)).
- **Tesseract in the image:** version 5.5, with the data of English, French, German, Spanish, Italian, Portuguese and Dutch. Tesseract 5 is the version tested. The data Polyfin downloads, `tessdata_fast`, holds LSTM models, which Tesseract reads from version 4 on.
- **Without Tesseract:** picture subtitles are not read. While the setting is on, the log says so at start.
- **The data folder for Tesseract:** it is read besides Tesseract's own data.
  - **System › Health** names the language and the file a track needs, and offers to download it.
  - **Download the data of missing languages automatically** downloads it too.
  - Polyfin creates the folder if missing, and must be able to write it.
  - In the image, it is the `tessdata` folder of the data volume. Files can also be put there by hand.
  - Unset, nothing is downloaded.
- **The download address:** a mirror of the pinned commit, or a test server.
  - Each file's SHA-256 is checked against the list built into Polyfin all the same. A file that differs is never kept.
  - A value that is not an `http` or `https` address stops Polyfin from starting.
- **Picture subtitles:** see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text).

Folders:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DATA_DIR` | `polyfin` in the system temporary directory; `/data` in the Docker image, a volume | Where Polyfin keeps its files, as an absolute path. See [Data folder](#data-folder). |
| `POLYFIN_FONTS_DIR` | `/usr/share/fonts`, with DejaVu in the Docker image | Fallback fonts apps load to render subtitles whose own fonts are missing. |
| `POLYFIN_WEB_DIR` | `/usr/share/polyfin/jellyfin-web`, where the Docker image puts jellyfin-web | Folder of jellyfin-web, the web player served at `/web/`. See [Web player](web-client.md). |

- **Data folder:** a path that is not absolute stops Polyfin from starting.
- **Fallback fonts:** apps get the `.ttf`, `.otf`, `.woff` and `.woff2` files of the folder and its subfolders (see [Subtitles](subtitles.md)).
  - Mount more fonts there for other scripts.
  - A missing folder offers none.
  - The names on [generated images](artwork-and-editing.md#generated-images) are written with its `.ttf`, `.otf`, `.ttc` and `.otc` fonts, else with Go Bold, built into Polyfin.
- **Web player:** a folder without its `index.html` leaves the web player off, as happens without the Docker image.

HTTPS certificates from an ACME server:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_ACME_DIRECTORY` | Let's Encrypt's, `https://acme-v02.api.letsencrypt.org/directory` | Directory of the ACME server certificates come from when **Serve HTTPS** is **Let’s Encrypt** (see [Let's Encrypt](https-and-networking.md#let-s-encrypt)). |
| `POLYFIN_ACME_CA_FILE` | (unset) | PEM file of the certificate authorities that sign the HTTPS of that ACME server, trusted besides the system's. |

- **Another directory:** Let's Encrypt's staging environment to try a setup, a private ACME server, or a test server.
  - A value that is not an `http` or `https` address stops Polyfin from starting.
- **The CA file:** for a private or test ACME server. A file that cannot be read is logged, and the system's authorities are trusted alone.

Polyfin Desktop and tests:

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DESKTOP` | (unset) | Set to `1` by Polyfin Desktop for the server it runs. Not for other setups. |
| `POLYFIN_NVENC13_TOOLS` | (unset) | For tests: a JSON file of the downloads of the FFmpeg for NVIDIA drivers older than 610, such as a local server's, instead of the pinned ones (see [NVIDIA driver](gpus-and-hdr.md#nvidia-driver)). |

With `POLYFIN_DESKTOP` set, the server knows that the app starts it again after a restart. It then offers the app's updates in the admin app (see [Restart and updates](updates-and-restarts.md#restart-and-updates)).

### Detailed log

Polyfin logs at the `info` level. It logs at the `debug` level while **Detailed log (to diagnose a problem)**, in **Settings › Diagnostics**, is on.

- It is off by default.
- It applies at once, without a restart.

At the `info` level, Polyfin logs each Jellyfin endpoint an app calls that it does not serve:

- with the app's name and version;
- at most once an hour per endpoint;
- without the request's identifiers, query or token.

### Data folder

`POLYFIN_DATA_DIR` holds these folders:

| Folder | Holds | Kept after a restart |
| --- | --- | --- |
| `cache` | HLS segments being played, guides and picture subtitles being read | No, emptied when Polyfin starts |
| `cache` | Parts of the files read | Yes, as **Settings › Playback** says (see [Keeping the files read across restarts](reading-the-sources.md#keeping-the-files-read-across-restarts)) |
| `cache` | Relayed images | Yes |
| `anime` | Lists that map anime between identifiers (see [Anime](tracking.md#anime)) | Yes |
| `themes` | The copy of a web player theme's files, when one is kept (see [Keeping a copy of a theme](themes-and-custom-code.md#keeping-a-copy-of-a-theme)) | Yes |
| `tools` | The FFmpeg for NVIDIA drivers older than 610, when NVENC needs it (see [NVIDIA driver](gpus-and-hdr.md#nvidia-driver)) | Yes |
| `recordings` | Live TV recordings | Yes |
| `backups` | Database backups | Yes |

- **Parts of `cache`:** the settings can move each to another folder (see [Folders for conversion, files being read and the cache](#folders-for-conversion-files-being-read-and-the-cache)).
- **Recordings and backups:** they go elsewhere when their settings name other folders.

### Polyfin Desktop

[Polyfin Desktop](desktop.md) sets these variables for the server, from its own folders:

| Variable | Value |
| --- | --- |
| `POLYFIN_DATABASE_URL` | Its own database, on `127.0.0.1` |
| `POLYFIN_DATA_DIR` | Its data folder, such as `~/Library/Application Support/Polyfin` on macOS (see [Folders](desktop.md#folders)) |
| `POLYFIN_FFMPEG`, `POLYFIN_FFPROBE` | The FFmpeg and ffprobe it downloads into that folder |
| `POLYFIN_WEB_DIR`, `POLYFIN_FONTS_DIR` | The jellyfin-web and fonts of its package |
| `POLYFIN_SECRET_KEY` | The key it keeps in the system's key store (see [The secret key](desktop.md#the-secret-key)) |
| `POLYFIN_DESKTOP` | `1` |

- **`PATH`:** it starts with the app's PostgreSQL programs, where backups find `pg_dump` and `pg_restore`.
- **Left out:** the values of these variables, and the `PG` variables, in the environment the app starts from.
- **Passed on as they are:** the other variables, such as `POLYFIN_LISTEN`.

### On Windows and macOS

`polyfin serve` runs on Windows and macOS as on Linux, outside Docker. Polyfin Desktop runs it so.

Paths are the system's, absolute as on Linux:

- `POLYFIN_DATA_DIR` takes `C:\Users\me\AppData\Local\Polyfin` or `/Users/me/Library/Application Support/Polyfin`. The executables and folders of the other variables take the same form.
- The defaults of `POLYFIN_FONTS_DIR` and `POLYFIN_WEB_DIR`, under `/usr/share`, are the Docker image's. They exist on neither system: set them, or apps get no fallback fonts and the web player stays off.
- The recordings, backups, conversion, files being read and cache folders of the settings take Windows paths too, such as `D:\Recordings` or `\\nas\media\Backups`.
- So do local folders, such as `D:\Movies` or `\\nas\media\Movies` (see [Local folders on Windows and macOS](local-folders.md#on-windows-and-macos)).

What else differs:

| Topic | Linux and macOS | Windows |
| --- | --- | --- |
| Stopping cleanly | On SIGTERM or SIGINT | On Ctrl-C or Ctrl-Break in its console, and when its console closes or the session ends |
| Restoring a backup | Polyfin replaces its process with a new Polyfin once the restore is done | Polyfin starts a new one as it was started itself, waits for it, and exits as it does. Whatever watches its process, such as Polyfin Desktop, still does. |
| FFmpeg's progress and extracted subtitles | On outputs of their own, besides the standard output | Windows passes a program no such outputs. FFmpeg writes them to `tcp://127.0.0.1`, on a port Polyfin picks for each and accepts once. |
| Disk space in **System › Health** | Measured through `statfs` | Measured through the volume each folder is on: a drive such as `C:\`, or a folder a volume is mounted in |
| FFmpeg reading in the background (loudness, commercials) | `nice` 19, the lowest priority | The idle priority class, the lowest priority |
| Watching folders for changes | inotify on Linux; kqueue on macOS, which keeps a file open for each folder and file watched | ReadDirectoryChangesW, which has no limit to raise |

On Windows only:

- The programs Polyfin runs there, FFmpeg, ffprobe and pg_dump, end with it even when it is killed. They belong to a job object of Polyfin's.
- A recording is stopped by FFmpeg's `q` key, sent on its standard input, rather than by an interrupt.

**System › Health** measures the disks of the cache, the conversion folder, the folder for files being read, the recordings and the backups on each system. For watching, see [Watching for changes](local-folders.md#watching-for-changes).

### Variables that became settings

These variables are now settings of the admin app, and are no longer read at every start.

- At the first start of a version that made them settings, Polyfin copies the value of those still set into their setting, once. It never reads them again.
- A value it cannot use is not copied, and the log says so. It never stops Polyfin from starting.
- Until they are removed, Polyfin logs at each start that it no longer reads five of them, as the table says.

| Variable | Setting | Logged at each start |
| --- | --- | --- |
| `POLYFIN_CACHE_SIZE` | **Disk space for files being read (GB)**, in **Settings › Playback** | Yes |
| `POLYFIN_HWACCEL` | The GPU choice, in **Settings › Conversion (transcoding)** (see [Transcoding](transcoding.md)) | No |
| `POLYFIN_VAAPI_DEVICE` | **Graphics card for VAAPI**, in **Settings › Conversion (transcoding)** | Yes |
| `POLYFIN_SEGMENTS` | The databases turned on, and their order, in **Settings › Content** (see [Skip segments](skip-segments.md)) | No |
| `POLYFIN_RECORDINGS_DIR` | **Record Live TV**, turned on, and **Recordings folder**, in **Settings › Recordings** (see [Live TV](live-tv.md)) | Yes |
| `POLYFIN_BACKUP_DIR` | **Back up the database every day**, turned on, and **Backups folder**, in **Settings › Backups** (see [Backups](backups.md)) | Yes |
| `POLYFIN_LOG_LEVEL` | **Detailed log (to diagnose a problem)**, in **Settings › Diagnostics**. `debug` turns it on; the other levels are not copied. | Yes |
| `POLYFIN_CACHE_DIR` | None, and never read: `POLYFIN_DATA_DIR` replaced it, and its `cache` folder holds the cache | No |

<a id="work-folders"></a>

## Folders for conversion, files being read and the cache

Polyfin keeps the files it works on in `cache`, in its data folder. Three settings move them, each to a folder of its own:

| Setting | Where | Default | What it holds |
| --- | --- | --- | --- |
| **Conversion folder** | **Settings › Conversion (transcoding)** | Empty, for `cache` | In `segments`, the segments of the files converted or repackaged while they play, Live TV's included. These are the busiest writes, which a fast disk or a memory folder suits (see [A memory folder for conversion](installation.md#a-memory-folder-for-conversion)). |
| **Folder for files being read** | **Settings › Playback**, below **Disk space for files being read (GB)** | Empty, for `cache` | In `sources`, the parts being read of the addons' files and of the files of S3 buckets, SFTP folders and rclone remotes. In `torrents`, the torrents' pieces. Both stay within that disk space (see [Reading the sources](reading-the-sources.md#reading-the-sources)). They often take several GB, which a large disk suits. |
| **Cache folder** | **Settings › General** | Empty, for `cache` | In `images`, the relayed images, up to 1 GB (see [Clearing the image cache](health.md#clearing-the-image-cache)). In `guides`, the guides being read. In `ocr`, the picture subtitles being read into text. |

**Checks on saving:** they are the same as for the recordings and backups folders. Saving refuses a folder that:

- is not an absolute path;
- does not exist;
- Polyfin cannot write a file into.

Polyfin never creates one of these folders. It is where a disk or a memory folder is mounted, and a folder made in its place would fill the disk beneath.

**At the next start:** a new folder applies when Polyfin starts again, and nothing is moved.

- The places above are emptied at each start, except `images` and `sources`.
  - `images` is kept from one start to the next. In a new cache folder, the images are relayed again as apps ask for them.
  - `sources` is kept as **Disk space and retention**, under **Settings › Playback**, says (see [Keeping the files read across restarts](reading-the-sources.md#keeping-the-files-read-across-restarts)). In a new folder for files being read, the files are read again from their sources.
- At that start, Polyfin removes what the data folder's `cache` kept for the folders now chosen elsewhere, the images and the files read included.
- A folder chosen before is left as it is: delete it by hand.

**Unwritable at start:** a folder that cannot be written into when Polyfin starts, such as a disk not mounted, is a problem in **System › Health**.

- The problem links to its setting.
- Polyfin keeps those files in the data folder's `cache` meanwhile, until a start finds the folder writable.

**Free space:** **System › Health** and the `polyfin_disk_free_bytes` metric give the free space of each folder (see [Metrics](metrics-and-api-keys.md#metrics)). They list `conversion` and `reading` besides `cache` when each is another folder than those before it.

**Polyfin Desktop:** the folders are folders of the computer, as its recordings folder is, such as `D:\Polyfin\Conversion`.

- On Linux, `/dev/shm` is a memory folder already there at every start, which can be chosen as it is.
- macOS and Windows have no memory folder of their own. A RAM disk made with another program must be there when Polyfin starts. Otherwise the data folder's `cache` takes its place until the next start.

**Compared with Jellyfin**

- Polyfin's conversion folder and cache folder match two Jellyfin settings, with the differences below. Jellyfin's dashboard has a "Transcode path", `TranscodingTempPath` ([EncodingOptions.cs, line 76](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Configuration/EncodingOptions.cs#L76)), empty for `transcodes` in its cache folder ([EncodingConfigurationExtensions.cs, lines 29 to 40](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Common/Configuration/EncodingConfigurationExtensions.cs#L29-L40)). It also has a "Cache path", `CachePath` ([BaseApplicationConfiguration.cs, line 37](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Configuration/BaseApplicationConfiguration.cs#L37)), which holds its image cache in `images` ([BaseApplicationPaths.cs, line 55](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/AppBase/BaseApplicationPaths.cs#L55)).
- Polyfin refuses its three folders as Jellyfin refuses a Cache path that does not exist or that it cannot write a file into ([BaseConfigurationManager.cs, lines 239 to 258](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/AppBase/BaseConfigurationManager.cs#L239-L258)).
- Polyfin never creates a folder chosen, and starts with the data folder's `cache` in place of one it cannot write into, which **System › Health** tells. Jellyfin does not check a Transcode path when it is saved. It creates it when missing, at start and at each use, and one it cannot create at start stops it from starting ([Program.cs, lines 195 to 199](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server/Program.cs#L195-L199)).
- Polyfin applies a new folder at the next start, so that the playbacks under way keep their files where they are. Jellyfin applies a new Cache path as the configuration is saved ([BaseConfigurationManager.cs, lines 179 and 202 to 232](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/AppBase/BaseConfigurationManager.cs#L202-L232)), and the Transcode path at each conversion.
- Polyfin empties its places at start, but keeps the images and the files read. Jellyfin empties neither folder at start. It deletes the transcoding files older than a day at start and every 24 hours ([DeleteTranscodeFileTask.cs, lines 66 to 89](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/ScheduledTasks/Tasks/DeleteTranscodeFileTask.cs#L66-L89)). It deletes the cache's files older than 30 days every 24 hours (see [Clearing the image cache](health.md#clearing-the-image-cache)).
- Polyfin keeps the parts of the files it reads from addons and torrents in a folder of its own. Jellyfin has no addons or torrents, and no folder for files being read: it reads files where they are.

## Compose settings in `.env`

The Compose file reads `POSTGRES_PASSWORD` and `POLYFIN_SECRET_KEY` from `.env` (see [`.env.example`](../.env.example)).

- **`POSTGRES_PASSWORD`:** Compose places it inside a PostgreSQL URL, so keep it URL-safe. Make one with `openssl rand -hex 24`.
- **`RCLONE_RC_PASSWORD`:** only with the optional rclone service, the password of rclone's remote control. Compose gives it to rclone with the user `polyfin`, and the rclone source in Polyfin signs in with that user and password, as entered in **Content › Sources** (see [An rclone container](installation.md#an-rclone-container)). Polyfin itself does not read the variable.
- **Another port:** to publish Polyfin on another port, change the first `8096` of `ports` in `compose.yaml`.
- **A pinned release:** replace the image's `latest` tag with the version you run now (see [Pin a version](installation.md#pin-a-version)).

## Stored keys and tokens

Polyfin stores a few secrets in its database. With `POLYFIN_SECRET_KEY` set, they are encrypted with AES-256-GCM, so that a copy of the database alone does not hand them out.

| Whose | Secrets |
| --- | --- |
| The server's | PublicMetaDB key, TheIntroDB key, TMDB API key, MDBList API key for ratings, Trakt client secret, AniList client secret, MyAnimeList client secret, Last.fm shared secret, SMTP password, backup passphrase, metrics token and **Update tool token** (see [Restart and updates](updates-and-restarts.md#restart-and-updates)) |
| Torrents | The SOCKS5 proxy password and the WireGuard configuration |
| Each user's | Tracking tokens and API keys (Trakt, Simkl, MDBList, PublicMetaDB, Last.fm, ListenBrainz), what their app connections sign in with (see [Staying connected](moving-to-polyfin.md#staying-connected)), and the keys of their [two-step sign-in](sign-in-and-security.md#two-step-sign-in) |
| Notification targets | Their addresses, tokens and keys (see [Notifications](notifications.md#targets)) |
| [Network shares](network-shares.md#credentials) | Passwords, secret keys, private keys and passphrases |
| [Backup destinations](backups.md#destinations) | Passwords, secret keys and private keys |
| [Let's Encrypt](https-and-networking.md#let-s-encrypt) | The account key and the certificate's private key. The certificate is stored as it is. |
| [Identity provider](sign-in-and-security.md#identity-provider-settings) | Its **Client secret** |

Addon addresses, IPTV passwords and guide addresses stay unencrypted.

How the key applies:

- At startup, the secrets still stored unencrypted are encrypted in place, in one go. Running again changes nothing.
- New values are written encrypted, and read back decrypted.
- Without the key, nothing changes from earlier versions. Secrets are stored as they are, and **System › Health** shows a warning while any is stored that way.

**A missing, changed or wrong key:** Polyfin still starts while encrypted secrets exist.

- The secrets it cannot decrypt count as not set. Their services act as not configured or not connected, and a backup destination sends nothing.
- The log shows one error naming them, never their values.
- **System › Health** lists them.

Set again the key they were encrypted with, or enter them again:

| Secret | Where to enter it again |
| --- | --- |
| A server key | Its section of **Settings** |
| The keys of the Let's Encrypt certificate | None: they are obtained again by themselves, with a new account |
| A backup destination's password or key | **Settings › Backups** |
| A user's connection | **My account › Tracking** |
| An app connection | Sign in again in **My account › Import from an app** |
| A user's two-step sign-in key | The user signs in with a recovery code, then turns two-step sign-in off and on again in **My account**. Or an administrator turns it off with **Turn off two-step sign-in** on their page. |

Until a secret is entered again, saving other settings keeps the value the right key can still decrypt.

**Backups:** they hold these secrets as they are stored, encrypted with the key set when the backup was made. Keep the key with your backups.

- Before restoring a backup from the admin app, Polyfin counts the backup's secrets that the current key cannot decrypt.
- It then asks before going ahead (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)).

Administrators can read the server's saved keys again. Each user can read their own MDBList and PublicMetaDB keys and ListenBrainz token. See [Administration](administration.md#showing-a-saved-key).
