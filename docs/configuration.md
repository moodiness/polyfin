# Configuration

This page lists the environment variables Polyfin reads. Many settings can also be changed in the admin app under **Settings**.

## Environment variables

| Variable | Default | What it does |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, for example `postgresql://polyfin:password@postgres:5432/polyfin`. |
| `POLYFIN_LISTEN` | `:8096` | HTTP address. 8096 is the port Jellyfin clients try by default. |
| `POLYFIN_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. From `info`, Polyfin logs each Jellyfin endpoint an app calls that it does not serve, with the app's name and version. It logs each endpoint at most once an hour, without the request's identifiers, query or token. **Settings › Diagnostics › Detailed log** switches to `debug` at once, without a restart, and back to this level when turned off. |
| `POLYFIN_FFPROBE` | `ffprobe` | ffprobe executable (FFmpeg 9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_FFMPEG` | `ffmpeg` | FFmpeg executable (9.0 or later): a path, or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_CACHE_DIR` | System temporary directory; `/cache` in the Docker image | Where Polyfin keeps parts of the files being read and the HLS segments being played. Emptied when Polyfin starts. |
| `POLYFIN_CACHE_SIZE` | `10GB` (at least 256 MiB) | Disk space the parts of files being read may use, for example `10GB` or `512MiB`. Parts read in the last 30 seconds are kept even above it. HLS segments come on top: about a minute ahead of each player by default (**Segments prepared ahead**), up to 1 GB for a 4K remux. See [Playback](playback.md). |
| `POLYFIN_HWACCEL` | `auto` | GPU video is converted on by default: `auto` for the first that works, `nvenc` (NVIDIA), `vaapi` (AMD, Intel), or `none` for the processor. **Settings › Conversion** can choose another. See [Transcoding](transcoding.md). |
| `POLYFIN_VAAPI_DEVICE` | Each render node in turn | Render node VAAPI opens, for example `/dev/dri/renderD128`, when several GPUs could. |
| `POLYFIN_SEGMENTS` | `theintrodb,introdb,publicmetadb` | Databases the skip intro, recap, credits and preview buttons come from, preferred first: `theintrodb`, `introdb`, `publicmetadb`, or `none` for no buttons. They are asked only while **Skip intro and credits buttons** is on under **Settings › Content**; when it is off, there are no buttons whatever this says. An order saved under **Settings › Content** replaces the order given here, but not the choice of databases. PublicMetaDB is asked only while its API key is saved under **Settings › Content**. See [Skip segments](skip-segments.md). |
| `POLYFIN_FONTS_DIR` | `/usr/share/fonts`, with DejaVu in the Docker image | Fallback fonts apps load to render subtitles whose own fonts are missing: the `.ttf`, `.otf`, `.woff` and `.woff2` files of this folder and its subfolders. Mount more fonts here for other scripts. A missing folder offers none. See [Subtitles](subtitles.md). |
| `POLYFIN_RECORDINGS_DIR` | (unset) | Folder Live TV recordings are written to, as an absolute path. Unset leaves recording off. Polyfin must be able to write to it, or it does not start. See [Live TV](live-tv.md). |
| `POLYFIN_WEB_DIR` | `/usr/share/polyfin/jellyfin-web`, where the Docker image puts jellyfin-web | Folder of jellyfin-web, the web client served at `/web/`. A folder without its `index.html` leaves the web client off, as happens without the Docker image. See [Web client](web-client.md). |
| `POLYFIN_SECRET_KEY` | (unset) | Key the stored API keys, client secret and tracking tokens are encrypted with: 32 random bytes in base64, made with `openssl rand -base64 32`, or 64 hexadecimal digits. Unset keeps them unencrypted, and **Health** warns about it. A malformed key stops Polyfin from starting. Keep it with your backups: see [Stored keys and tokens](#stored-keys-and-tokens). |
| `POLYFIN_BACKUP_DIR` | (unset) | Folder the database is backed up into every day, as an absolute path. Unset leaves backups off. Polyfin must be able to write to it, or it does not start. The hour and the number of backups kept are under **Settings › Backups**. See [Backups](backups.md). |

## Compose settings in `.env`

The Compose files read their own settings (passwords, ports, image version) from `.env`. See [`.env.example`](../.env.example).

## Stored keys and tokens

Polyfin stores a few secrets in its database: the server's PublicMetaDB key, TheIntroDB key and Trakt client secret, and each user's tracking tokens and API keys (Trakt, Simkl, MDBList, PublicMetaDB). With `POLYFIN_SECRET_KEY` set, they are encrypted with AES-256-GCM, so that a copy of the database alone does not hand them out:

- At startup, the secrets still stored unencrypted are encrypted in place, in one go. Running again changes nothing.
- New values are written encrypted, and read back decrypted.
- Without the key, nothing changes from earlier versions: secrets are stored as they are, and **System › Health** shows a warning while any is stored that way.

If the key is missing, changed or wrong while encrypted secrets exist, Polyfin still starts. The secrets it cannot decrypt count as not set: the services they belong to act as not configured or not connected. The log shows one error naming them, never their values, and **System › Health** lists them. Set the key they were encrypted with again, or enter them again: a server key under **Settings**, a user's connection under **My account › Tracking**. Until one is entered again, saving other settings keeps the value the right key can still decrypt.

Addon addresses, IPTV passwords and guide addresses stay unencrypted.

Administrators can read the server's saved keys again, and each user their own MDBList and PublicMetaDB keys: see [Administration](administration.md#showing-a-saved-key).
