# Installation

This page is for anyone installing Polyfin as a server, with Docker Compose or on Unraid. After installing, continue with [Getting started](getting-started.md).

<a id="docker-or-the-desktop-version"></a>

## Docker or Polyfin Desktop

| | Docker | [Polyfin Desktop](desktop.md) |
| --- | --- | --- |
| Runs on | A NAS, or a computer that stays on | The computer you have: Windows, macOS or Linux |
| Database | A PostgreSQL container beside Polyfin | Its own, in the package |
| FFmpeg | In the image | Downloaded at its first start |
| Started by | Docker | An icon in the tray, or the menu bar on macOS |
| Runs | At any hour | While the computer is on and you are signed in |
| Suits | A household that watches from TVs and phones at any hour, and GPUs given to the container | One computer, with no Docker and nothing else to install |

See [Install](desktop.md#install) for the desktop packages.

Both run the same server, with the same admin app and apps. A backup moves from one to the other (see [Moving to Docker and back](desktop.md#moving-to-docker-and-back)).

## Docker Compose

### Requirements

- Docker with Compose v2.
- The image, `ghcr.io/moodiness/polyfin`, is published for linux/amd64 and linux/arm64.

### Steps

1. Clone the repository and create your `.env` file:

   ```sh
   git clone https://github.com/moodiness/polyfin.git
   cd polyfin
   cp .env.example .env
   ```

2. Set `POSTGRES_PASSWORD` in `.env`. You can generate one with `openssl rand -hex 24`.
3. Optionally but preferably, set `POLYFIN_SECRET_KEY` in `.env` to the output of `openssl rand -base64 32`. Polyfin then encrypts the secrets it stores. Keep a copy with your backups (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
4. Start Polyfin:

   ```sh
   docker compose up -d
   ```

5. Open `http://<server>:8096/admin/`. Polyfin waits for PostgreSQL and creates its tables on startup.

Everything else is set in the admin app, in the sections of **Settings** (see [Configuration](configuration.md)).

### Let apps find Polyfin on the local network

Jellyfin apps find servers on the local network by broadcasting a message to UDP port 7359. They list the servers that answer (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)). Whether the message reaches Polyfin depends on the container's network:

| Network | Do broadcasts reach Polyfin | What to do |
| --- | --- | --- |
| Docker Desktop (macOS, Windows), with the default `compose.yaml` | Yes: Docker Desktop forwards the published `7359/udp` from the computer, broadcasts included | Polyfin only sees its container's address. Set **Address on the local network**, in **Settings › General**, to the computer's address and Polyfin's port, such as `http://192.168.1.10:8096` |
| Docker Engine on Linux, with a bridge network such as the default one | No: a published port only receives messages sent to the host's own address | Use the host's network, below |
| A network that gives the container its own address, such as macvlan or Unraid's `br0` | Yes, directly | Nothing: Polyfin announces the right address |

The network also decides which of Polyfin's ports the local network reaches:

```mermaid
flowchart TD
  B["Bridge network (default)"] --> BP["Published ports reach Polyfin:<br/>8096 web, 7359 discovery,<br/>8920 HTTPS once published"]
  B --> BX["On Docker Engine on Linux,<br/>broadcasts and multicast do not:<br/>7359 discovery, 1900 DLNA,<br/>Find tuners on 65001"]
  H["Host network<br/>(compose.host.yaml)"] --> A["Polyfin is on the local network:<br/>8096, 8920, 7359, 1900,<br/>and Find tuners on 65001"]
  M["macvlan or Unraid br0<br/>(its own address)"] --> A
```

- **Port 8096 (TCP):** the web player, the admin app and the apps.
- **Port 8920 (TCP):** HTTPS, once turned on (see [Serve HTTPS](#serve-https)).
- **Port 7359 (UDP):** local network discovery. Docker Desktop forwards its broadcasts on a bridge network; Docker Engine on Linux does not.
- **Port 1900 (UDP):** DLNA, once turned on (see below).
- **Port 65001 (UDP):** the port HDHomeRun tuners answer **Find tuners** on (see [Find HDHomeRun tuners](#find-hdhomerun-tuners)).

To run Polyfin on the host's network, on Linux, add `compose.host.yaml`:

```sh
docker compose -f compose.yaml -f compose.host.yaml up -d
```

- **PostgreSQL:** the file also publishes PostgreSQL on the host's loopback, where Polyfin then reaches it.
- **Address:** Polyfin announces the address of the host's network interface that reaches the app, and needs no address set.
- **Docker Desktop:** keep the default file there. Its host's network is that of Docker's virtual machine, which the local network does not reach.

With `docker run`, publish the port, or use the host's network:

```sh
docker run -d --name polyfin -p 8096:8096 -p 7359:7359/udp -v polyfin_data:/data \
  -e POLYFIN_DATABASE_URL=postgresql://polyfin:password@db.example.lan:5432/polyfin ghcr.io/moodiness/polyfin:latest
docker run -d --name polyfin --network host -v polyfin_data:/data \
  -e POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin ghcr.io/moodiness/polyfin:latest
```

With a PostgreSQL server of your own, searches by name use its `pg_trgm` extension, which the official PostgreSQL image has. Polyfin adds it, with the indexes it serves, the first time a version of Polyfin that uses it starts, if its user may create in its database. Without it, searches still work, only slower, and adding the extension later does not add the indexes.

Only one program on a host's network can use port 7359:

- **Next to a Jellyfin server:** on the same host's network, Polyfin starts anyway and logs it. **System › Health** says that it cannot answer.
- **Container address:** when the answers would give the container's address, **System › Health** says so too.
- **To stop answering:** turn off **Answer local network discovery**, in **Settings › General**.

See [Local network discovery](https-and-networking.md#local-network-discovery) for both settings.

TVs and players that browse DLNA servers find Polyfin the same way, once **Share libraries over DLNA** is on in **Settings › DLNA**. They search by multicast on UDP port 1900:

- **Where it works:** the multicast reaches the container only on the host's network, or on a network that gives it its own address.
- **Bridge network:** publishing `1900:1900/udp` on a bridge network only helps devices that search the host's address directly.
- **Next to another DLNA server:** on the same host's network, Polyfin starts anyway, and **System › Health** says that it cannot listen on 1900.

See [DLNA](dlna.md#docker-networking).

### Find HDHomeRun tuners

**Find tuners** reaches HDHomeRun tuners only from the host's network, or from a network that gives the container its own address. See [Finding tuners with Docker](hdhomerun.md#finding-tuners-with-docker) for the other ways to add a tuner.

### Serve HTTPS

Polyfin can serve HTTPS itself, on port 8920 by default, with certificate files or a certificate from Let's Encrypt (see [HTTPS](https-and-networking.md#https)).

1. Publish the HTTPS port next to 8096 in `compose.yaml`:

   ```yaml
       ports:
         - "8096:8096"
         - "7359:7359/udp"
         - "8920:8920"
   ```

2. Choose **Serve HTTPS** in **Settings › Security**.

With **Certificate files**, mount the folder that holds them read-only, and give their paths in the container, such as `/certs/fullchain.pem` and `/certs/privkey.pem`. User 65532 must be able to read them:

```yaml
    volumes:
      - polyfin_data:/data
      - /srv/certs/media.example.org:/certs:ro
```

- **Certbot:** its `live` folders hold links into its `archive` folder, and only root can read its keys. Have its deploy hook copy the two files into such a folder, readable by user 65532, at each renewal.
- **Renewals:** Polyfin serves the new certificate within a minute.

With **Let's Encrypt**, the container's user cannot listen below 1024. The internet's port 80 or 443 must reach Polyfin's ports through the router or `ports`:

| Challenge | Forward |
| --- | --- |
| HTTP-01 | Port 80 to Polyfin's HTTP port |
| TLS-ALPN-01 | Port 443 to Polyfin's HTTPS port |

On the host itself, that is one of:

```yaml
    ports:
      - "80:8096"     # HTTP-01: Let's Encrypt's checks reach the HTTP port
      - "443:8920"    # TLS-ALPN-01, and HTTPS on the usual port
```

Each line needs the host's own port free. When another service uses it, forward the port on the router instead, to the host's 8096 or 8920.

Apps reach Polyfin at `https://media.example.org:8920`, or at `https://media.example.org` once 443 reaches the HTTPS port. In that case, set **Public address**, in **Settings › Notifications**, to `https://media.example.org`, without a port. **Redirect HTTP to HTTPS** sends clients:

- to the port of an `https://` public address, or 443 when it names none;
- without a public address, to the HTTPS port, 8920, which the internet may not reach.

See [The redirect](https-and-networking.md#the-redirect).

### Keep torrents to a VPN

Torrent streams are off by default, and go through one route alone (see [Torrents](torrents.md)):

| Route | What the container needs |
| --- | --- |
| A SOCKS5 proxy | Nothing more |
| A WireGuard tunnel, which Polyfin runs itself from a provider's `.conf` | Nothing more. On Linux 5.7 or later, Polyfin's own container needs no added capability |
| A network interface | The container must see it: use `compose.host.yaml` with WireGuard on the host, or share the network of a WireGuard container that has `NET_ADMIN` (`network_mode: service:<name>`), publishing Polyfin's ports on that container |

### Pin a version

Replace the image's `latest` tag in `compose.yaml` with a release: the version you run now, or a later one.

- `latest` follows stable releases.
- An older version than the one that last ran refuses the database, whose tables are newer than its own.

### Restart from the admin app

**Restart**, on the admin app's **Home**, stops Polyfin and exits with code 75 (`EX_TEMPFAIL`), not 0. The container's restart policy then starts it again (see [Restart and updates](updates-and-restarts.md#restart)):

| Restart policy | After Restart |
|---|---|
| `unless-stopped`, as in `compose.yaml` | started again |
| `always` | started again |
| `on-failure` | started again: 75 is not 0 |
| `no`, Docker's default for `docker run` | stays stopped until `docker start` |

- **With `docker run`:** give the policy with `--restart unless-stopped`.
- **The warning:** Polyfin cannot see the policy from inside the container. The admin app warns before restarting until an administrator turns on **Polyfin is started again when it stops**, in **Settings › General**.

Updating the image needs Docker itself: pull the new image and recreate the container. An update tool that runs beside Polyfin can do it too, which **Update now** can ask (see [Update now](updates-and-restarts.md#update-now)). Polyfin is never given the Docker socket.

### Back up the database

Everything Polyfin keeps is in its PostgreSQL database. To back it up every day, turn on **Back up the database every day** in **Settings › Backups**.

The backups go to Polyfin's data volume, on the same disk as the database. To keep them on another disk, mount a folder of the server at `/data/backups`, writable by UID 65532 (see [Turning backups on](backups.md#turning-backups-on)).

See [Backups](backups.md) for:

- the schedule and the number kept;
- the destinations they are also sent to: S3-compatible storage, SFTP or WebDAV;
- how to restore one from the admin app.

### A light PostgreSQL

`compose.yaml` starts PostgreSQL with settings that keep it small:

| Setting | `compose.yaml` | PostgreSQL's default |
| --- | --- | --- |
| `shared_buffers` | `32MB` | `128MB` |
| `max_connections` | `20` | `100` |
| `max_locks_per_transaction` | `512` | `64` |
| `max_worker_processes` | `2` | `8` |
| `max_parallel_workers` | `0` | `8` |
| `autovacuum_max_workers` | `1` | `3` |
| `max_wal_size` | `256MB` | `1GB` |
| `jit` | `off` | `on` |

- **Measured** in `docker stats`, with a database of 132,000 items: 62 MiB rather than 233 MiB after a workload, and 22 MiB rather than 106 MiB idle. Searches and lookups ran as fast.
- **What Polyfin needs:** no parallel query nor extra workers. A restore of a backup takes about 6,150 locks in one transaction, hence the 512 locks per connection (see [What a restore does](backups.md#what-a-restore-does)).
- **The pool:** Polyfin opens 10 connections at most, whatever the number of CPUs. With the one that follows the changes of addons, and pg_dump's during a backup, that makes 12 of the 20. To set another bound, add `pool_max_conns` to `POLYFIN_DATABASE_URL`, such as `postgresql://polyfin:password@postgres:5432/polyfin?pool_max_conns=6`. Keep `max_connections` a few above it.
- **Polyfin Desktop:** its own database has the same settings, clusters made by older versions included.

To apply them to an existing PostgreSQL container:

- **With Compose:** copy the `command` of the `postgres` service from the current `compose.yaml` into yours, then run `docker compose up -d`. PostgreSQL restarts with its data, and Polyfin reconnects.
- **With `docker run`, or on Unraid:** add the `-c` options after the image's name, such as `-c shared_buffers=32MB -c max_connections=20`. On Unraid, they go in the PostgreSQL container's Post Arguments, in the advanced view of its template.
- **A PostgreSQL that other services share:** their needs count too. Keep its settings if unsure, and add `pool_max_conns` to Polyfin's URL if it allows few connections.

Check them with `docker compose exec postgres psql -U polyfin -c 'SHOW shared_buffers'`.

Polyfin does not support SQLite instead of PostgreSQL (see [discussion #1061](https://github.com/moodiness/polyfin/discussions/1061)). Its queries rely on PostgreSQL's JSON indexes, arrays and row locks, and SQLite's single writer would hold playback progress back behind long imports, for a smaller saving than these settings give: most of PostgreSQL's memory is its cache.

### Folders for conversion, files being read and the cache

The segments being played, the files being read and the cache go to Polyfin's data volume by default. **Conversion folder**, **Folder for files being read** and **Cache folder** can name other folders (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)).

To keep each on a disk of its own:

1. Create the folders, writable by the container's user (UID 65532):

   ```sh
   sudo mkdir -p /mnt/large/polyfin/reading /mnt/ssd/polyfin/cache
   sudo chown 65532:65532 /mnt/large/polyfin/reading /mnt/ssd/polyfin/cache
   ```

2. Mount them in the `polyfin` service of `compose.yaml`, and the conversion folder in memory:

   ```yaml
       volumes:
         - polyfin_data:/data
         - /mnt/large/polyfin/reading:/reading
         - /mnt/ssd/polyfin/cache:/cache
       tmpfs:
         - /conversion:size=4g
   ```

3. Run `docker compose up -d`, so that the container gets them.
4. Set the folders in the admin app:

   | Folder | Setting | Where |
   | --- | --- | --- |
   | `/conversion` | **Conversion folder** | **Settings › Conversion (transcoding)** |
   | `/reading` | **Folder for files being read** | **Settings › Playback** |
   | `/cache` | **Cache folder** | **Settings › General** |

5. Restart Polyfin (see [Restart from the admin app](#restart-from-the-admin-app)). The folders apply from that start on.

A folder that is not mounted when Polyfin starts shows in **System › Health**. The data volume holds its files until the next start.

#### A memory folder for conversion

The `tmpfs` above is a memory folder. It is emptied at each start of the container, and every user can write to it.

It suits the conversion folder, the busiest one. It needs room for the segments of every conversion and repackaging running at once:

- **Per playback:** each keeps the segments of **Seconds prepared ahead** (120 by default) past the last one the app asked for, and about 24 seconds more around it (see [HLS segments](transcoding.md#hls-segments)).
- **Examples:** about 150 MB for a 1080p conversion at 8 Mbps, and up to 1 GB for a 4K file repackaged at 60 Mbps.
- **[Adaptive quality](transcoding.md#adaptive-quality):** the lower qualities' segments stay until the playback ends.

A `tmpfs` takes memory only as it fills, up to its `size`. 4 GB serves a few 4K playbacks at once.

A conversion that finds it full cannot write its segments. Give it more room, or lower **Seconds prepared ahead**.

### An rclone container

Polyfin reads folders from the many storages rclone reaches, through an rclone instance that serves its remote control interface (see [rclone remotes](network-shares.md#addresses)).

1. Make rclone's configuration with `rclone config`, in a folder beside `compose.yaml`.
2. Add a service to `compose.yaml`:

   ```yaml
     rclone:
       image: rclone/rclone:latest
       command: rcd --rc-serve --rc-addr=:5572 --rc-user=polyfin --rc-pass=${RCLONE_RC_PASSWORD:?Set RCLONE_RC_PASSWORD in .env}
       volumes:
         - ./rclone:/config/rclone
       restart: unless-stopped
   ```

3. Set `RCLONE_RC_PASSWORD=<a password>` in `.env`. `.env.example` lists it, commented out.
4. Publish no port: Polyfin reaches rclone on the network the services share.
5. In **Content › Sources**, choose **Add a source**, **Folder** and **rclone remote**. Give `http://rclone:5572` as its rclone address, and `polyfin` and that password as its user and password.
6. Pick its remote with **List the remotes**.

The storage's own credentials stay in rclone's configuration. Give the remote read-only credentials where the storage offers them: anyone with the remote control's password may write through it.

### Upgrade from an older Compose file

Older Compose files set more variables, such as `POLYFIN_CACHE_SIZE` or `POLYFIN_BACKUP_DIR`. They keep working. Polyfin copies their values into the settings at its first start with a version that made them settings (see [Variables that became settings](configuration.md#variables-that-became-settings)).

To move to the current file:

1. Start the new version once with your old file, so that Polyfin copies the values it sets.
2. Replace `compose.yaml` with the current one. In `.env`, only `POSTGRES_PASSWORD` and `POLYFIN_SECRET_KEY` are read now. If you changed `POSTGRES_USER` or `POSTGRES_DB`, write their values in place of `polyfin` in the new file.
3. Copy into it the `volumes` lines of the recordings and backups folders you mounted, such as `./backups:/backups`. Their settings name these folders.
4. Start it again with `docker compose up -d`.
5. Remove the cache volume, no longer used: `docker volume rm polyfin_polyfin_cache`.

### Build from source

To build the image from source instead of pulling it, run:

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

## Unraid

The template is [`templates/unraid/polyfin.xml`](../templates/unraid/polyfin.xml). It needs a PostgreSQL 18 container.

To add the template:

1. Run this in Unraid's terminal:

   ```sh
   wget -O /boot/config/plugins/dockerMan/templates-user/my-Polyfin.xml https://raw.githubusercontent.com/moodiness/polyfin/main/templates/unraid/polyfin.xml
   ```

2. Choose **Polyfin** under **Docker › Add Container › Template**.
3. Optionally but preferably, fill **Secret key** with the output of `openssl rand -base64 32`, run in Unraid's terminal. The secrets Polyfin stores are then encrypted. Keep a copy with your backups (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).

Every option other than those below is set in the admin app, in the sections of **Settings**.

**Backups and recordings:**

1. Create a folder of the server, writable by UID 65532, such as `/mnt/user/appdata/polyfin/backups`.
2. Set it as the template's **Backups folder** or **Recordings folder**.
3. Turn on **Back up the database every day** in **Settings › Backups** (see [Turning backups on](backups.md#turning-backups-on)), or **Record Live TV** in **Settings › Recordings**.

**Work folders on another disk:** for the segments being played, the files being read or the cache:

1. Add a `Path` for each, such as `/mnt/user/polyfin-reading` to `/reading`, created with `chown 65532:65532` as above.
2. Name the container's folder in **Folder for files being read**, **Conversion folder** or **Cache folder**.
3. Restart Polyfin (see [Folders for conversion, files being read and the cache](#folders-for-conversion-files-being-read-and-the-cache)).

For a memory folder, add `--tmpfs /conversion:size=4g` to the container's extra parameters, in the advanced view of its template.

**Local network discovery:** for Jellyfin apps to find Polyfin by themselves, set the container's **Network Type** to one of:

- `br0`, which gives the container its own address on your network;
- `Host`.

On the default `bridge`, the apps' broadcasts do not reach Polyfin, even with **Discovery port** published (see [Let apps find Polyfin on the local network](#let-apps-find-polyfin-on-the-local-network)).

**HTTPS:**

1. Add the HTTPS port to the container with **Add another Path, Port, Variable, Label or Device**: type `Port`, `8920` as both the container's and the host's port, over TCP.
2. Choose **Serve HTTPS** in the admin app, in **Settings › Security** (see [HTTPS](https-and-networking.md#https)).
3. For **Certificate files**, add a `Path` that maps the folder holding them, read-only, such as `/mnt/user/appdata/certs` to `/certs`.

For **Let's Encrypt**, Unraid's own web interface usually holds ports 80 and 443 on the server. Forward the router's ports rather than mapping them on the container:

- for **HTTP-01**, the router's port 80 to the server's port 8096;
- for **TLS-ALPN-01**, the router's port 443 to the server's port 8920.

When the router forwards 443 to 8920, set **Public address**, in **Settings › Notifications**, to `https://your-host`. **Redirect HTTP to HTTPS** then sends clients to 443.

**Older templates:** containers made from an older template keep working. Their variables, such as `POLYFIN_CACHE_SIZE` or `POLYFIN_BACKUP_DIR`, are copied into the settings at their first start with a version that made them settings. You can remove them from the container afterwards.

## Health check

The image tells Docker whether Polyfin answers. `docker ps`, and Unraid's **Docker** page, show the container's health: starting, healthy or unhealthy.

Docker runs `polyfin healthcheck` in the container. It asks Polyfin for `/healthz` at the address `POLYFIN_LISTEN` gives. When that names no host, or `0.0.0.0` or `::`, it asks `127.0.0.1`.

| | While Polyfin starts | Once started |
| --- | --- | --- |
| How often | Every 2 seconds, for up to 2 minutes | Every 30 seconds |
| A failure | Does not count yet | Counts: after 3 in a row, the container is unhealthy |

- **A failed check:** Polyfin did not answer 200 within 3 seconds.
- **At start:** the container turns healthy within 2 seconds of Polyfin answering.
- **Docker before 25:** it checks every 30 seconds from the start.

The check tells whether Polyfin's process serves:

- **Waiting for the database:** Polyfin answers once it has reached PostgreSQL and created its tables. The health stays starting meanwhile, and turns unhealthy when Polyfin still waits 3 checks after the 2 minutes.
- **Database away later:** once started, Polyfin keeps answering while PostgreSQL is away. A database briefly unreachable shows in **System › Health**, and leaves the container healthy. Restarting Polyfin, or leaving it out of a proxy, would not bring the database back.

Docker itself does not restart an unhealthy container: `restart: unless-stopped` only restarts a container whose process has ended. Swarm services, and tools that watch containers' health, do restart it.

### Behind Traefik

Traefik's Docker provider leaves out containers whose health is starting or unhealthy. Until Polyfin is healthy, Traefik has no route to it, and answers 404 at its address.

- **Each start:** checking every 2 seconds while Polyfin starts keeps that to about 2 seconds. With Docker before 25, it can last 30 seconds.
- **`allowEmptyServices`:** this Traefik option keeps the route, which then answers 503 while the container is not healthy.

See [Reverse proxy](reverse-proxy.md).

### Turn it off

| Where | How |
| --- | --- |
| `docker run` | Add `--no-healthcheck` |
| Unraid | Add `--no-healthcheck` to the container's extra parameters, in the advanced view of its template |
| Compose | Add the block below to the `polyfin` service |

```yaml
    healthcheck:
      disable: true
```

### Monitoring

Polyfin answers two addresses for monitoring tools, without signing in:

| Address | Answers 200 `ok` | Answers 503 `database unavailable` |
| --- | --- | --- |
| `/healthz` | While Polyfin's process serves, PostgreSQL reachable or not. The health check asks it | Not for the database |
| `/readyz` | When PostgreSQL is reachable too | When PostgreSQL is not reachable, or while a backup is being restored |

Both answer in plain text. For Prometheus, Polyfin can also expose its metrics at `/metrics`, with a token (see [Metrics](metrics-and-api-keys.md#metrics)).

## GPU

To convert video on a GPU, give the container access to it. See [Transcoding](transcoding.md) for NVIDIA, AMD and Intel GPUs.
