<p align="center"><img src="assets/polyfin-social.png" alt="Polyfin"></p>

Polyfin is a self-hosted, Jellyfin-compatible server that sources its content from Stremio addons: catalogs, metadata, streams, and subtitles. It provides real user accounts and transcoding, so any Jellyfin client can connect to it like a regular Jellyfin server, without Jellyfin installed.

> [!NOTE]
> Polyfin is in early development: Jellyfin apps can sign in with a password or Quick Connect, browse the libraries, collections, titles, seasons, and episodes the addons provide, with their artwork and search, play the versions their device supports as they are, remuxed into HLS, or transcoded when the app cannot take the video or audio, on an NVIDIA, AMD or Intel GPU when there is one, with the addons' subtitles and the text subtitles inside the files, ASS subtitles with the fonts the files carry, image subtitles burned in when the app cannot show them, and keep each user's watched state, resume points and favorites. HDR video converted to SDR is tone mapped on an NVIDIA GPU up to 1080p, Dolby Vision profile 5 included; elsewhere tone mapping runs on the processor, stops at 720p, and leaves profile 5 out.

## Features

- **Jellyfin-compatible API**: standard Jellyfin clients sign in, browse, search, and play. Polyfin targets the Jellyfin 12.1 API.
- **Stremio addons as the content source**: AIOMetadata for catalogs and metadata, AIOStreams for streams and subtitles, and any other addon that speaks the standard Stremio protocol.
  - Stremio catalogs become Jellyfin libraries. An addon's collection catalogs (such as AIOMetadata's) become collection libraries, where each collection gathers the catalogs it groups, movies and series together.
  - A genre, studio, or year page lists the titles of the catalogs you reach, as libraries or through collections, that offer that name in their genre filter (addons use it for genres, and some for years or studios); titles from catalogs that cannot be filtered by it are not on it.
  - The actors, directors, and writers a title credits open as people, with the photo the metadata addon gives. A person's titles are those your addons' people-search catalogs (such as AIOMetadata's People Search) find for their name, plus the titles of your addons Polyfin already knows them in; without such a catalog, only the latter. The people list holds the people credited in the titles of your addons that someone opened; people credited only in other users' own addons stay hidden.
  - A movie's or series' similar titles come from the first page of the catalogs of your addons that offer one of its genres in their genre filter, ranked by shared genres, directors and actors, and release year, leaving out the title itself and those you played. Trailers reach apps as remote trailers; addons give no other extras.
  - Stremio streams become versions (media sources) of the same item, and addon subtitles become external subtitle tracks. An app's subtitle search lists the user's subtitle addons' subtitles.
  - The subtitles inside a file reach apps that take subtitles only as files, such as jellyfin-web, from the start of playback. ASS subtitles keep their styles and positions, with the fonts the file carries.
  - Stremio TV catalogs (the `tv` type) become Jellyfin Live TV: their channels, numbered in catalog order with their logos, and the programmes of the addons that publish a guide (Stremio's Native EPG).
- **Multiple users**: separate accounts with Jellyfin authentication and Quick Connect. Watched state, favorites, resume points, and Next Up are tracked per user, and each user can be limited to titles up to a rating.
- **Transcoding**: direct play when the client supports the file, with Polyfin redirecting the client to the stream and staying out of the video path; otherwise Polyfin's own on-demand HLS, built on FFmpeg for remote sources: a remux when the app can play the video but not the container, converting the audio when the app cannot take it, and converting the video, HDR to SDR included, when it cannot take that or must have image subtitles (PGS, VobSub, DVB) burned in. Video converts on an NVIDIA GPU through NVENC, or on an AMD or Intel GPU through VAAPI, when one is available, else on the processor.

## How it works

```text
Jellyfin client ──Jellyfin API──> Polyfin ──Stremio protocol──> AIOMetadata / AIOStreams / other addons
      │                              │
      │ direct play: 302 ────────────┼──────────────> debrid / provider URL
      │                              │
      └── transcoding: HLS <── FFmpeg (reads the stream through Polyfin's cache, serves the segments)
```

Polyfin always handles authentication, accounts, browsing, metadata, source selection, and playback state. It reads the remote stream itself only when transcoding, or to relay a source the app could not reach (one that needs request headers or is on a local network address) or a redirect it could not follow, so the direct play or transcoding decision determines how much bandwidth the server uses. Apps never see the addons' stream URLs: they receive Polyfin's own, signed for the user.

## Compatible clients

Polyfin targets the clients that connect to a Jellyfin server, including:

- Infuse
- Swiftfin
- Findroid
- Streamyfin
- Nuvio
- Strand
- Official Jellyfin apps
- Kodi

## Quick start (Docker)

Requirements: Docker with Compose v2. The image, `ghcr.io/moodiness/polyfin`, is published for linux/amd64 and linux/arm64.

```sh
git clone https://github.com/moodiness/polyfin.git
cd polyfin
cp .env.example .env    # then set POSTGRES_PASSWORD, e.g. openssl rand -hex 24
docker compose up -d
```

`POLYFIN_VERSION` in `.env` pins a release, such as `0.1.0`; `latest` follows stable releases. To build the image from source instead, run `docker compose -f compose.yaml -f compose.build.yaml up -d --build`.

Open `http://<server>:8096/admin/`. Polyfin waits for PostgreSQL and creates its tables on startup.

**First run:** until an administrator exists, Polyfin prints a one-time setup code in its log (`docker compose logs polyfin`). Enter it on the setup page to create the administrator, then create the other accounts under **Users**. Jellyfin apps sign in with these accounts, by password or with **Quick Connect**: the app shows a 6-digit code that a signed-in user approves on the Quick Connect page. Users can change their password from Jellyfin apps, which signs their other devices out.

**Language:** the names Polyfin generates for Jellyfin apps (seasons, untitled episodes, and the type that tells apart libraries with the same name, such as "Popular (Movies)") are in English or French, set under **Settings** and defaulting to the language the setup page was in.

**Addons:** paste an addon's manifest URL (from its configure page) under **Addons** to share it with every user, then pick under **Libraries** which of its catalogs become libraries in Jellyfin apps. When an addon offers collection catalogs, those are enabled first; otherwise its first 20 movie, series and TV catalogs are. This is only a starting point: any number of catalogs can be enabled. Each user can also add their own addons and libraries under **My addons**, and turn the server's addons off for themselves. Manifest URLs usually contain your addon settings or keys: Polyfin never shows them in full. Only administrators can install addons hosted on a local network address.

**Artwork:** Polyfin relays images from the addons' artwork servers, so Jellyfin apps only ever talk to Polyfin.

**Playback:** a title's details list every stream the addons offer as a version. The first time a version is played, Polyfin analyzes it with ffprobe (a few seconds) and keeps the result, then tells the app whether it can play it as is, as a Jellyfin server would. The analysis also reads the version's chapters, which apps then show and skip through: those of the version the title was opened or played as, without chapter images; a version never played has no chapters yet. When the app has not picked a version, one that cannot be read is skipped in favor of the next, and the app is told of the other versions too, for apps that let the user pick one at that point. Apps can save the user's playback preferences (audio and subtitle languages, subtitle mode), which choose the default audio and subtitle tracks. Apps that keep titles for offline viewing can download the movies and episodes whose versions they were shown: a download is the version's file, under its file name, sent to the source or relayed as playback would be. Active playback appears in Jellyfin apps' dashboards. Apps can control one another (play, pause, seek, messages) when the controlled app keeps Jellyfin's live connection (WebSocket) open. The skip intro, recap and credits buttons come from [TheIntroDB](https://theintrodb.org) and [IntroDB](https://introdb.app), community databases; Polyfin sends them only titles' IDs, episode numbers and runtimes.

**Watching together:** apps that offer SyncPlay, jellyfin-web first, play the same titles in step across devices and users. Anyone in a group can set or change what plays, up to 5,000 entries, play, pause and seek; the group starts and resumes on Polyfin's clock, a little ahead so that every app gets there in time, and waits for the apps still loading or buffering, for 30 seconds at most. Every user may create and join groups, but a group is listed to and joined by only the users who can see everything it plays, and nothing a member cannot see is queued. Like Jellyfin, Polyfin keeps the groups in memory: a session leaves its group when its app disconnects or is signed out (by the app, a password change, or an administrator), and a group ends once empty. The apps need Jellyfin's live connection (WebSocket) open.

**Live TV:** a TV catalog enabled under **Libraries** adds no library: its channels appear in the Live TV view of Jellyfin apps, numbered in the order the catalogs list them. Addons that publish a guide (Stremio's Native EPG: a TV catalog taking a date) fill the programme guide, read a day at a time and kept ten minutes; without one, the guide is empty, as Jellyfin's is without guide data, and the channels play all the same. A channel plays as the addon streams it when the app takes it: an HLS stream is relayed through Polyfin, with the headers the source needs, unless the app can reach the source itself (jellyfin-web always gets it relayed, as browsers cannot read other sites' playlists). Otherwise FFmpeg remuxes it, or converts what the app cannot take, into HLS as it comes, from the moment the app opens the channel until it leaves it (a minute without requests stops it). Polyfin records nothing: there are no recordings or timers.

**Subtitles:** addon subtitles and the text tracks inside a version reach each app in the format it takes. Apps that take subtitles only as files, jellyfin-web first, get the tracks inside a Matroska file read whole through its index, which tells where every subtitle line sits: Polyfin fetches little more than those lines, many in one request when the host allows it, and keeps the track, so it takes seconds the first time and nothing after. ASS tracks keep their styles and positions, and the fonts the file carries are listed for apps that render ASS. A host that answers one byte range at a time would take a request for nearly every line, which providers refuse past a rate: a track that would take it more than 64 requests, the dialogue of a film for one, is not read so. Those tracks, and the ones the index does not locate, as in MP4 files, are offered as files once a remux has read them whole.

**Watch state:** each user's played titles, resume points, favorites and ratings are kept by Polyfin, so they follow the user from one Jellyfin app to another. Playback moves the resume point and marks a title played near its end, with Jellyfin's thresholds. Continue Watching lists what is under way, and Next Up the next episode of each series being watched. The Upcoming row lists the coming episodes of the series the user watches or marked favorite. Marking a series or a season played marks its released episodes. Apps that keep Jellyfin's live connection (WebSocket) open are told of these changes as they happen, from any of the user's apps. Users can also make playlists of movies and episodes from their apps, private, shared with other users, or open to all.

**Parental control:** under **Users**, an administrator can limit a user to titles up to a rating, and hide from them the movies or shows that have none; administrators' Jellyfin apps can set the same limit from their user settings. A title's rating is the certification the server's metadata addon gives, the US one for AIOMetadata, or the local one when there is no US one; Polyfin also reads the French, German and British ratings and plain ages ("12", "16+"), and a rating it does not know counts as none. Users' own addons never rate titles, and a limited user's apps show the server's addons and libraries only: their own addons are kept but not used, and they cannot turn the server's addons off. Seasons and episodes follow their series. A title above the limit is hidden from the user everywhere, as Jellyfin hides it: libraries, collections, genre, studio and year pages, search, latest, Continue Watching, Next Up, Upcoming, playlists, similar titles and the titles of a person leave it out, people credited only in hidden titles are not listed, and opening, playing, downloading it or searching subtitles for it is refused. Addon catalogs rarely carry ratings, so for a limited user Polyfin asks the metadata addon for each listed title it does not know the rating of yet, a few at a time, once per title: the rating is kept, whichever user opened the title, and asked again after a week, or after a few hours for a title that had none. A listing waits a few seconds for these ratings; it then stops at the titles still unknown, which appear in later listings, and it reads at most a few times as far into a catalog as for a user without a limit. Artwork stays reachable without signing in, as in Jellyfin.

**GPU:** at startup Polyfin encodes a few frames on each GPU it can reach, NVIDIA first, then AMD or Intel, and logs the one it converts video on. Give the container an NVIDIA GPU with `--runtime=nvidia` (Compose: `runtime: nvidia`), which needs the NVIDIA Container Toolkit, or Unraid's Nvidia Driver plugin; give it an AMD or Intel GPU with `--device /dev/dri` (Compose: `devices`). On an NVIDIA GPU, HDR is also tone mapped there, through Vulkan, which needs `graphics` among `NVIDIA_DRIVER_CAPABILITIES`, as the image sets them. The container runs as user 65532: when the render nodes in `/dev/dri` are not open to every user, add the group that owns them with `--group-add`.

**Unraid:** the template lives in [`templates/unraid/polyfin.xml`](templates/unraid/polyfin.xml) and needs a PostgreSQL 18 container. To add it, run in Unraid's terminal `wget -O /boot/config/plugins/dockerMan/templates-user/my-Polyfin.xml https://raw.githubusercontent.com/moodiness/polyfin/main/templates/unraid/polyfin.xml`, then choose **Polyfin** under **Docker › Add Container › Template**.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `POLYFIN_DATABASE_URL` | (required) | PostgreSQL URL, e.g. `postgresql://polyfin:password@postgres:5432/polyfin` |
| `POLYFIN_LISTEN` | `:8096` | HTTP address. 8096 is the port Jellyfin clients try by default. |
| `POLYFIN_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. From `info`, Polyfin logs each Jellyfin endpoint an app calls that it does not serve, with the app's name and version, at most once an hour per endpoint, without the identifiers, query or token of the request. |
| `POLYFIN_FFPROBE` | `ffprobe` | ffprobe executable (FFmpeg 9.0 or later), a path or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_FFMPEG` | `ffmpeg` | FFmpeg executable (9.0 or later), a path or a name looked up in `PATH`. The Docker image includes one. |
| `POLYFIN_CACHE_DIR` | system temporary directory, `/cache` in the Docker image | Where parts of the files being read, and the HLS segments being played, are kept. Emptied when Polyfin starts. |
| `POLYFIN_CACHE_SIZE` | `10GB` | Disk space the parts of files being read may use, e.g. `10GB` or `512MiB`; at least 256 MiB. Parts read in the last 30 seconds are kept even above it. HLS segments come on top: about a minute ahead of each player, up to 1 GB for a 4K remux. |
| `POLYFIN_HWACCEL` | `auto` | GPU video is converted on: `auto` for the first that works, `nvenc` (NVIDIA), `vaapi` (AMD, Intel), or `none` for the processor. |
| `POLYFIN_VAAPI_DEVICE` | each render node in turn | Render node VAAPI opens, e.g. `/dev/dri/renderD128`, when several GPUs could. |
| `POLYFIN_SEGMENTS` | `theintrodb,introdb` | Databases the skip intro, recap and credits buttons come from, the preferred first: `theintrodb`, `introdb`, or `none` for no buttons. |

The Compose files read their own settings (passwords, ports, image version) from `.env`; see [`.env.example`](.env.example).

## Development

Requirements: Go 1.27, Node.js 24, a PostgreSQL 18 server, and FFmpeg 9.0 or later (`ffmpeg` and `ffprobe`) to play titles. With `POSTGRES_PASSWORD` set in `.env`, `docker compose up -d postgres` starts one on `127.0.0.1:5432`.

```sh
export POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin
make dev                      # builds the admin app, then runs the server on :8096
npm --prefix web run dev      # optional: admin app with hot reload, proxied to :8096
make check                    # formatting, vet and tests
```

Database tests run when `POLYFIN_TEST_DATABASE_URL` points to a disposable PostgreSQL database, and remux tests when `POLYFIN_TEST_FFMPEG` names an `ffmpeg` executable with `ffprobe` beside it; CI always provides both. Each database test works in its own schema.

Jellyfin API responses are checked against the JSON structure of a real Jellyfin 12.1 server, recorded in `internal/jellyfin/testdata/`. `scripts/jellyfin-fixtures.sh` records them again from a disposable Jellyfin container (requires Docker, curl and jq).

## Legal disclaimer

Polyfin does not host, store, or distribute any content. It only relays what the addons and services configured by its operator provide. You are solely responsible for the addons and services you configure and for complying with the laws that apply to you.

Polyfin is an independent project, not affiliated with or endorsed by Jellyfin or Stremio.

## License

Polyfin is released under the [MIT License](LICENSE). To report a vulnerability, see the [security policy](SECURITY.md).
