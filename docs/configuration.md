# Configuration

Polyfin is set in the admin app, under **Settings**. The environment only gives it what it needs before it can read its settings: where its database is, and the key its stored secrets are encrypted with.

## Environment variables

With the Compose file, `POSTGRES_PASSWORD` in `.env` is the only value to set: Compose builds `POLYFIN_DATABASE_URL` from it. `POLYFIN_SECRET_KEY` is optional but advised. The Docker image sets the others.

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, for example `postgresql://polyfin:password@postgres:5432/polyfin`. Besides its pool, Polyfin keeps one connection that listens for changes to addons and libraries, so that it answers them from memory; a pooler in transaction mode cannot carry it, and Polyfin then reads them from the database for every request. |
| `POLYFIN_SECRET_KEY` | (unset) | Key the stored API keys, client secret and tracking tokens are encrypted with: 32 random bytes in base64, made with `openssl rand -base64 32`, or 64 hexadecimal digits. Unset keeps them unencrypted, and **Health** warns about it. A malformed key stops Polyfin from starting. Keep it with your backups: see [Stored keys and tokens](#stored-keys-and-tokens). |
| `POLYFIN_DATA_DIR` | `polyfin` in the system temporary directory; `/data` in the Docker image, a volume | Folder Polyfin keeps its files in, as an absolute path: in `cache`, the parts of the files being read and the HLS segments being played, emptied when Polyfin starts; and, unless their settings name other folders, the Live TV recordings in `recordings` and the database backups in `backups`. |
| `POLYFIN_LISTEN` | `:8096` | HTTP address. 8096 is the port Jellyfin clients try by default. |
| `POLYFIN_FFPROBE` | `ffprobe` | ffprobe executable (FFmpeg 9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_FFMPEG` | `ffmpeg` | FFmpeg executable (9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_FONTS_DIR` | `/usr/share/fonts`, with DejaVu in the Docker image | Fallback fonts apps load to render subtitles whose own fonts are missing: the `.ttf`, `.otf`, `.woff` and `.woff2` files of this folder and its subfolders. Mount more fonts here for other scripts. A missing folder offers none. See [Subtitles](subtitles.md). |
| `POLYFIN_WEB_DIR` | `/usr/share/polyfin/jellyfin-web`, where the Docker image puts jellyfin-web | Folder of jellyfin-web, the web client served at `/web/`. A folder without its `index.html` leaves the web client off, as happens without the Docker image. See [Web client](web-client.md). |

Polyfin logs at the `info` level, and at the `debug` level while **Detailed log**, under **Settings › Diagnostics**, is on. At the `info` level, it logs each Jellyfin endpoint an app calls that it does not serve, with the app's name and version, at most once an hour per endpoint, without the request's identifiers, query or token.

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

Polyfin stores a few secrets in its database: the server's PublicMetaDB key, TheIntroDB key and Trakt client secret, and each user's tracking tokens and API keys (Trakt, Simkl, MDBList, PublicMetaDB). With `POLYFIN_SECRET_KEY` set, they are encrypted with AES-256-GCM, so that a copy of the database alone does not hand them out:

- At startup, the secrets still stored unencrypted are encrypted in place, in one go. Running again changes nothing.
- New values are written encrypted, and read back decrypted.
- Without the key, nothing changes from earlier versions: secrets are stored as they are, and **System › Health** shows a warning while any is stored that way.

If the key is missing, changed or wrong while encrypted secrets exist, Polyfin still starts. The secrets it cannot decrypt count as not set: the services they belong to act as not configured or not connected. The log shows one error naming them, never their values, and **System › Health** lists them. Set the key they were encrypted with again, or enter them again: a server key under **Settings**, a user's connection under **My account › Tracking**. Until one is entered again, saving other settings keeps the value the right key can still decrypt.

Addon addresses, IPTV passwords and guide addresses stay unencrypted.

Administrators can read the server's saved keys again, and each user their own MDBList and PublicMetaDB keys: see [Administration](administration.md#showing-a-saved-key).
