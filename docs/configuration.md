# Configuration

Polyfin is set in the admin app, under **Settings**. The environment only gives it what it needs before it can read its settings: where its database is, and the key its stored secrets are encrypted with.

## Environment variables

With the Compose file, `POSTGRES_PASSWORD` in `.env` is the only value to set: Compose builds `POLYFIN_DATABASE_URL` from it. `POLYFIN_SECRET_KEY` is optional but advised. The Docker image sets the others.

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, for example `postgresql://polyfin:password@postgres:5432/polyfin`. Besides its pool, Polyfin keeps one connection that listens for changes to addons and libraries, so that it answers them from memory; a pooler in transaction mode cannot carry it, and Polyfin then reads them from the database for every request. |
| `POLYFIN_SECRET_KEY` | (unset) | Key the stored API keys, client secret and tracking tokens are encrypted with: 32 random bytes in base64, made with `openssl rand -base64 32`, or 64 hexadecimal digits. Unset keeps them unencrypted, and **Health** warns about it. A malformed key stops Polyfin from starting. Keep it with your backups: see [Stored keys and tokens](#stored-keys-and-tokens). |
| `POLYFIN_DATA_DIR` | `polyfin` in the system temporary directory; `/data` in the Docker image, a volume | Folder Polyfin keeps its files in, as an absolute path: in `cache`, the parts of the files being read and the HLS segments being played, emptied when Polyfin starts; in `anime`, the lists that map anime between identifiers (see [Anime](tracking.md#anime)); and, unless their settings name other folders, the Live TV recordings in `recordings` and the database backups in `backups`. |
| `POLYFIN_LISTEN` | `:8096` | HTTP address. 8096 is the port Jellyfin clients try by default. HTTPS, when served, listens on the same host, on the **HTTPS port** of **Settings › Security**, 8920 by default (see [HTTPS](administration.md#https)). |
| `POLYFIN_DLNA_PORT` | `1900` | UDP port of SSDP, which DLNA devices search for servers on (see [DLNA](dlna.md)). Devices only use 1900: another port serves tests. |
| `POLYFIN_DISCOVERY_PORT` | `7359` | UDP port of local network discovery (see [Local network discovery](administration.md#local-network-discovery)). Jellyfin apps only send to 7359: another port serves tests. |
| `POLYFIN_HDHOMERUN_PORT` | `65001` | UDP port **Find tuners** asks HDHomeRun tuners to answer on (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)). Tuners only answer on 65001: another port serves tests. A value that is not a port stops Polyfin from starting. |
| `POLYFIN_HDHOMERUN_ADDRESSES` | (unset) | Addresses **Find tuners** asks, separated by commas, such as `192.168.1.20,192.168.1.21`, instead of broadcasting on the local networks: for a container whose broadcasts do not reach them (see [Find HDHomeRun tuners](installation.md#find-hdhomerun-tuners)). An address without a port is asked on `POLYFIN_HDHOMERUN_PORT`. A value that is not a host or host:port stops Polyfin from starting. |
| `POLYFIN_FFPROBE` | `ffprobe` | ffprobe executable (FFmpeg 9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_FFMPEG` | `ffmpeg` | FFmpeg executable (9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_TESSERACT` | `tesseract`; `/usr/bin/tesseract` in the Docker image | Tesseract executable (5 or later), which reads picture subtitles into text with **Turn picture subtitles into text** on: a path, or a name looked up in `PATH`. The Docker image includes Tesseract 5.5 with the data of English, French, German, Spanish, Italian, Portuguese and Dutch. Without it, picture subtitles are not read, and the log says so at start while the setting is on. See [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text). |
| `POLYFIN_TESSDATA_DIR` | (unset); `/data/tessdata` in the Docker image | Folder of Tesseract `.traineddata` files for languages Tesseract's own data lacks, such as `jpn.traineddata` for Japanese, read besides its own. **Health** names the language and the file a track would need. In the image, create the folder `tessdata` in the data volume and put the files there. |
| `POLYFIN_FONTS_DIR` | `/usr/share/fonts`, with DejaVu in the Docker image | Fallback fonts apps load to render subtitles whose own fonts are missing: the `.ttf`, `.otf`, `.woff` and `.woff2` files of this folder and its subfolders. Mount more fonts here for other scripts. A missing folder offers none. See [Subtitles](subtitles.md). The names of [generated images](addons-and-libraries.md#generated-images) are written with its `.ttf`, `.otf` and `.ttc` fonts, else with Go Bold, built into Polyfin. |
| `POLYFIN_WEB_DIR` | `/usr/share/polyfin/jellyfin-web`, where the Docker image puts jellyfin-web | Folder of jellyfin-web, the web client served at `/web/`. A folder without its `index.html` leaves the web client off, as happens without the Docker image. See [Web client](web-client.md). |
| `POLYFIN_ACME_DIRECTORY` | Let's Encrypt's, `https://acme-v02.api.letsencrypt.org/directory` | Directory of the ACME server certificates come from when **Serve HTTPS** is **Let's Encrypt** (see [Let's Encrypt](administration.md#let-s-encrypt)): Let's Encrypt's staging environment to try a setup, a private ACME server, or a test server. A value that is not an `http` or `https` address stops Polyfin from starting. |
| `POLYFIN_ACME_CA_FILE` | (unset) | PEM file of the certificate authorities that sign the HTTPS of the ACME server of `POLYFIN_ACME_DIRECTORY`, trusted besides the system's: for a private or test ACME server. A file that cannot be read is logged, and the system's are trusted alone. |

Polyfin logs at the `info` level, and at the `debug` level while **Detailed log**, under **Settings › Diagnostics**, is on. At the `info` level, it logs each Jellyfin endpoint an app calls that it does not serve, with the app's name and version, at most once an hour per endpoint, without the request's identifiers, query or token.

### On Windows and macOS

`polyfin serve` runs on Windows and macOS as on Linux, outside Docker; the desktop version runs it so. What differs:

- Paths are the system's, absolute as on Linux: `C:\Users\me\AppData\Local\Polyfin` or `/Users/me/Library/Application Support/Polyfin` for `POLYFIN_DATA_DIR`, and the same for the executables and folders of the other variables. The defaults of `POLYFIN_FONTS_DIR` and `POLYFIN_WEB_DIR`, under `/usr/share`, are the Docker image's and exist on neither system: set them, or apps get no fallback fonts and the web client stays off. Local folders take Windows paths too, `D:\Movies` or `\\nas\media\Movies` (see [Local folders on Windows and macOS](local-folders.md#on-windows-and-macos)).
- **Stopping:** Polyfin stops cleanly on SIGTERM or SIGINT on macOS, as on Linux. Windows has no signals: Polyfin stops cleanly on Ctrl-C or Ctrl-Break in its console, and when its console closes or the session ends. The programs it runs there, FFmpeg, ffprobe, pg_dump, end with it even when it is killed, as they belong to a job object of Polyfin's. A recording is stopped by FFmpeg's `q` key, sent on its standard input, rather than by an interrupt.
- **Restoring a backup:** on Linux and macOS Polyfin replaces its process with a new Polyfin once the restore is done. Windows cannot: Polyfin starts a new one, as it was started itself, waits for it and exits as it does, so that whatever watches its process, such as the desktop app, still does.
- **FFmpeg's outputs:** FFmpeg tells its progress, and gives the subtitles it extracts, on outputs of their own besides its standard output. Windows passes a program no such outputs, so FFmpeg writes them to connections on the loopback, `tcp://127.0.0.1` on a port Polyfin picks for each, which it accepts once.
- **Disk space:** **System › Health** measures the disks of the data folder, the recordings and the backups on each system: through `statfs` on Linux and macOS, and on Windows through the volume each folder is on, a drive such as `C:\` or a folder a volume is mounted in.
- **Priority:** FFmpeg reading in the background, to measure loudness or find commercials, runs at the lowest priority on each system: `nice` 19 on Linux and macOS, the idle priority class on Windows.
- Watching folders for changes goes through inotify on Linux, kqueue on macOS, which keeps a file open for each folder and file watched, and ReadDirectoryChangesW on Windows, which has no limit to raise (see [Watching for changes](local-folders.md#watching-for-changes)).

### Variables that became settings

These variables are no longer read at every start: they are settings of the admin app. At its first start with a version that made them settings, Polyfin copies the value of those still set into their setting, once, and never reads them again. Until they are removed, it logs at each start that it no longer reads them. A value it cannot use is not copied, and the log says so: it never stops Polyfin from starting.

| Variable | Setting |
| --- | --- |
| `POLYFIN_CACHE_SIZE` | **Disk space for files being read (GB)**, under **Settings › Playback** |
| `POLYFIN_HWACCEL` | The GPU choice, under **Settings › Conversion** (see [Transcoding](transcoding.md)) |
| `POLYFIN_VAAPI_DEVICE` | **Graphics card for VAAPI**, under **Settings › Conversion** |
| `POLYFIN_SEGMENTS` | The databases turned on, and their order, under **Settings › Content** (see [Skip segments](skip-segments.md)) |
| `POLYFIN_RECORDINGS_DIR` | **Record Live TV**, turned on, and **Recordings folder**, under **Settings › Recordings** (see [Live TV](live-tv.md)) |
| `POLYFIN_BACKUP_DIR` | **Back up the database every day**, turned on, and **Backups folder**, under **Settings › Backups** (see [Backups](backups.md)) |
| `POLYFIN_LOG_LEVEL` | **Detailed log**, under **Settings › Diagnostics**: `debug` turns it on; the other levels are not copied |
| `POLYFIN_CACHE_DIR` | Not a setting: replaced by `POLYFIN_DATA_DIR`, whose `cache` folder holds the cache |

## Compose settings in `.env`

The Compose file reads `POSTGRES_PASSWORD` and `POLYFIN_SECRET_KEY` from `.env`. See [`.env.example`](../.env.example). To publish Polyfin on another port, change the first `8096` of `ports` in `compose.yaml`; to pin a release, replace the image's `latest` tag with its version, such as `0.22.0`.

## Stored keys and tokens

Polyfin stores a few secrets in its database: the server's PublicMetaDB key, TheIntroDB key, Trakt client secret, Last.fm shared secret and SMTP password, each user's tracking tokens and API keys (Trakt, Simkl, MDBList, PublicMetaDB, Last.fm, ListenBrainz), what each user's app connections sign in with (see [Staying connected](users.md#staying-connected)), the addresses, tokens and keys of notification targets (see [Notifications](notifications.md#targets)), the passwords, secret keys, private keys and passphrases of [network shares](local-folders.md#credentials), the passwords, secret keys and private keys of [backup destinations](backups.md#destinations), the account key and private key of the certificate of [Let's Encrypt](administration.md#let-s-encrypt), whose certificate is stored as it is, and the keys of the users' [two-step sign-in](users.md#two-step-sign-in). With `POLYFIN_SECRET_KEY` set, they are encrypted with AES-256-GCM, so that a copy of the database alone does not hand them out:

- At startup, the secrets still stored unencrypted are encrypted in place, in one go. Running again changes nothing.
- New values are written encrypted, and read back decrypted.
- Without the key, nothing changes from earlier versions: secrets are stored as they are, and **System › Health** shows a warning while any is stored that way.

If the key is missing, changed or wrong while encrypted secrets exist, Polyfin still starts. The secrets it cannot decrypt count as not set: the services they belong to act as not configured or not connected, and a backup destination sends nothing. The log shows one error naming them, never their values, and **System › Health** lists them. Set the key they were encrypted with again, or enter them again: a server key under **Settings**, the keys of the certificate of Let's Encrypt obtained again by themselves, with a new account, a backup destination's password or key under **Settings › Backups**, a user's connection under **My account › Tracking**, an app connection by signing in again under **My account › Import from an app**. Until one is entered again, saving other settings keeps the value the right key can still decrypt. A user whose two-step sign-in key cannot be decrypted signs in with a recovery code, then turns it off and on again under **My account**, or an administrator turns it off with **Turn off two-step sign-in** on their page.

Backups hold these secrets as they are stored: encrypted with the key set when the backup was made. Keep the key with your backups. Before restoring a backup from the admin app, Polyfin counts the secrets of the backup the current key cannot decrypt, and asks before going ahead (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)).

Addon addresses, IPTV passwords and guide addresses stay unencrypted.

Administrators can read the server's saved keys again, and each user their own MDBList and PublicMetaDB keys and ListenBrainz token: see [Administration](administration.md#showing-a-saved-key).
