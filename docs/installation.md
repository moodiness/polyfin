# Installation

This page explains how to install Polyfin with Docker Compose or on Unraid. After installing, continue with [Getting started](getting-started.md).

## Docker or the desktop version

- Docker, below, runs Polyfin as a server, on a NAS or a computer that stays on, beside a PostgreSQL container: the way for a household that watches from TVs and phones at any hour, and for GPUs given to the container.
- The desktop version, [Polyfin Desktop](desktop.md), runs it on the computer you have, Windows, macOS or Linux, with no Docker and nothing else to install: its package brings its own database, and downloads FFmpeg at its first start. Each release has an installer for Windows, x64 or arm64, a disk image for macOS, and a `.deb` and an AppImage for Linux (see [Install](desktop.md#install)). An icon in the tray, or the menu bar, starts and stops it, and it runs while the computer is on and you are signed in.

Both run the same server, with the same admin app and apps, and a backup moves from one to the other (see [Moving to Docker and back](desktop.md#moving-to-docker-and-back)).

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
   Optionally but preferably, also set `POLYFIN_SECRET_KEY` to the output of `openssl rand -base64 32`: Polyfin then encrypts the API keys and tracking tokens it stores (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)). Keep a copy with your database backups; with another key or none, those keys have to be entered again.
3. Start Polyfin:

   ```sh
   docker compose up -d
   ```

4. Open `http://<server>:8096/admin/`. Polyfin waits for PostgreSQL and creates its tables on startup.

Everything else is set in the admin app, under **Settings**. See [Configuration](configuration.md).

### Let apps find Polyfin on the local network

Jellyfin apps look for servers on the local network by broadcasting a message to UDP port 7359, and list those that answer (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)). Whether the message reaches Polyfin depends on the container's network:

- **Docker Desktop** (macOS, Windows), with the default `compose.yaml`, which publishes `7359/udp`: Docker Desktop forwards the port from the computer, broadcasts included. Polyfin only sees its container's address, though, so set **Address on the local network**, under **Settings › General**, to the computer's address and Polyfin's port, such as `http://192.168.1.10:8096`.
- **Docker Engine on Linux**, with a bridge network such as the default one: a published port only receives the messages sent to the host's own address, and broadcasts never reach the container. Use the host's network instead, below.
- **A network that gives the container its own address on the local network**, such as macvlan or Unraid's `br0`: broadcasts reach the container directly, and Polyfin announces the right address.

To run Polyfin on the host's network, on Linux, add `compose.host.yaml`, which also publishes PostgreSQL on the host's loopback, where Polyfin then reaches it:

```sh
docker compose -f compose.yaml -f compose.host.yaml up -d
```

Polyfin then announces the address of the host's network interface that reaches the app, and needs no address set. On Docker Desktop, the host's network is that of Docker's virtual machine, which the local network does not reach: keep the default file there.

With `docker run`, publish the port, or use the host's network:

```sh
docker run -d --name polyfin -p 8096:8096 -p 7359:7359/udp -v polyfin_data:/data \
  -e POLYFIN_DATABASE_URL=postgresql://polyfin:password@db.example.lan:5432/polyfin ghcr.io/moodiness/polyfin:latest
docker run -d --name polyfin --network host -v polyfin_data:/data \
  -e POLYFIN_DATABASE_URL=postgresql://polyfin:password@127.0.0.1:5432/polyfin ghcr.io/moodiness/polyfin:latest
```

Only one program on a host's network can use port 7359: next to a Jellyfin server on the same host's network, Polyfin starts anyway, logs it, and **System › Health** says that it cannot answer. When the answers would give the container's address, **System › Health** says so too. To stop answering, turn off **Answer local network discovery**, under **Settings › General**. See [Local network discovery](administration.md#local-network-discovery) for both settings.

TVs and players that browse DLNA servers find Polyfin the same way, once **Share libraries over DLNA** is on under **Settings › DLNA**: they search by multicast on UDP port 1900, which reaches the container only on the host's network or on a network that gives it its own address. Publishing `1900:1900/udp` on a bridge network only helps devices that search the host's address directly. Next to another DLNA server on the same host's network, Polyfin starts anyway and **System › Health** says that it cannot listen on 1900. See [DLNA](dlna.md#docker-networking).

### Find HDHomeRun tuners

**Find tuners**, on the **HDHomeRun tuner** tab of **Add a source**, broadcasts HDHomeRun's discovery to UDP port 65001 of the local network (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)). Like the messages of Jellyfin apps above, a broadcast crosses no bridge network: from a container on Docker Desktop or on a bridge network of Docker Engine, it never reaches the tuners. On the host's network (`compose.host.yaml`), or on a network that gives the container its own address, it does.

Elsewhere, add the tuner by its address (**Add by address**), which works on any network the container reaches the tuner from, or set `POLYFIN_HDHOMERUN_ADDRESSES` to the tuners' addresses, such as `192.168.1.20`: **Find tuners** then asks each one directly instead of broadcasting (see [Configuration](configuration.md)).

### Serve HTTPS

Polyfin can serve HTTPS itself, on port 8920 by default, with certificate files or a certificate from Let's Encrypt (see [HTTPS](administration.md#https)). Publish the HTTPS port next to 8096 in `compose.yaml`, then choose **Serve HTTPS** under **Settings › Security**:

```yaml
    ports:
      - "8096:8096"
      - "7359:7359/udp"
      - "8920:8920"
```

With **Certificate files**, mount the folder that holds them read-only, and give their paths in the container, such as `/certs/fullchain.pem` and `/certs/privkey.pem`. User 65532 must be able to read them:

```yaml
    volumes:
      - polyfin_data:/data
      - /srv/certs/media.example.org:/certs:ro
```

Certbot's `live` folders hold links into its `archive` folder, and its keys are readable by root only: have its deploy hook copy the two files into such a folder, readable by user 65532, at each renewal. Polyfin serves the new certificate within a minute.

With **Let's Encrypt**, the container's user cannot listen below 1024, so the internet's port 80 or 443 must reach Polyfin's ports through the router or `ports`. For **HTTP-01**, forward port 80 to Polyfin's HTTP port; for **TLS-ALPN-01**, port 443 to its HTTPS port. On the host itself, that is one of:

```yaml
    ports:
      - "80:8096"     # HTTP-01: Let's Encrypt's checks reach the HTTP port
      - "443:8920"    # TLS-ALPN-01, and HTTPS on the usual port
```

Each line needs the host's own port free: forward the port on the router instead, to the host's 8096 or 8920, when another service uses it. Apps reach Polyfin at `https://media.example.org:8920`, or at `https://media.example.org` once 443 reaches the HTTPS port. In that case, set **Public address**, under **Settings › Notifications**, to `https://media.example.org`, without a port: **Redirect HTTP to HTTPS** sends clients to the port of an `https://` public address, 443 when it names none, and without one to the HTTPS port, 8920, which the internet may not reach (see [The redirect](administration.md#the-redirect)).

### Keep torrents to a VPN

Torrent streams, off by default, go through one route alone (see [Torrents](torrents.md)). A SOCKS5 proxy needs nothing more. A network interface must be seen by the container: use `compose.host.yaml` with WireGuard on the host, or share the network of a WireGuard container that has `NET_ADMIN` (`network_mode: service:<name>`), publishing Polyfin's ports on that container. On Linux 5.7 or later, Polyfin's own container needs no added capability.

### Pin a version

Replace the image's `latest` tag in `compose.yaml` with a release, such as `0.22.0`. `latest` follows stable releases.

### Back up the database

Everything Polyfin keeps, from accounts and watch history to settings, addons and IPTV line-ups, is in its PostgreSQL database. To back it up every day, turn **Back up the database every day** on, under **Settings › Backups**. The backups go to Polyfin's data volume. They are safer on another disk than the database's: mount a folder of the server there, in the `polyfin` service of `compose.yaml`:

```yaml
    volumes:
      - polyfin_data:/data
      - ./backups:/data/backups
```

Create the folder before starting, writable by the container's user (UID 65532):

```sh
mkdir backups && sudo chown 65532:65532 backups
```

See [Backups](backups.md) for the schedule, the number kept, the destinations they are also sent to (S3-compatible storage, SFTP or WebDAV), and how to restore one from the admin app.

### An rclone container

Polyfin reads folders from the many storages rclone reaches through an rclone instance serving its remote control interface (see [rclone remotes](local-folders.md#addresses)). Run it next to Polyfin: add a service to `compose.yaml`, with its configuration in a folder beside the file, made with `rclone config`:

```yaml
  rclone:
    image: rclone/rclone:latest
    command: rcd --rc-serve --rc-addr=:5572 --rc-user=polyfin --rc-pass=${RCLONE_RC_PASSWORD:?Set RCLONE_RC_PASSWORD in .env}
    volumes:
      - ./rclone:/config/rclone
    restart: unless-stopped
```

Set `RCLONE_RC_PASSWORD` in `.env`. Publish no port: Polyfin reaches it on the network the services share. Then add the folder under **Sources › Add a source › Folder › rclone remote**, with `http://rclone:5572` as its rclone address, `polyfin` and that password as its user and password, and pick its remote with **List the remotes**. The storage's own credentials stay in rclone's configuration; give the remote read-only credentials where the storage offers them, since anyone with the remote control's password may write through it.

### Upgrade from an older Compose file

Older Compose files set more variables, such as `POLYFIN_CACHE_SIZE` or `POLYFIN_BACKUP_DIR`. They keep working: Polyfin copies their values into the settings at its first start with a version that made them settings (see [Variables that became settings](configuration.md#variables-that-became-settings)). To move to the current file:

1. Start the new version once with your old file, so that Polyfin copies the values it sets.
2. Replace `compose.yaml` with the current one. In `.env`, only `POSTGRES_PASSWORD` and `POLYFIN_SECRET_KEY` are read now: if you changed `POSTGRES_USER` or `POSTGRES_DB`, write their values in place of `polyfin` in the new file.
3. Copy into it the `volumes` lines of the recordings and backups folders you mounted, such as `./backups:/backups`: their settings name these folders.
4. Start it again with `docker compose up -d`, then remove the cache volume no longer used: `docker volume rm polyfin_polyfin_cache`.

### Build from source

To build the image from source instead of pulling it, run:

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

## Unraid

The template lives in [`templates/unraid/polyfin.xml`](../templates/unraid/polyfin.xml). It needs a PostgreSQL 18 container.

To add the template:

1. Run this in Unraid's terminal:

   ```sh
   wget -O /boot/config/plugins/dockerMan/templates-user/my-Polyfin.xml https://raw.githubusercontent.com/moodiness/polyfin/main/templates/unraid/polyfin.xml
   ```

2. Choose **Polyfin** under **Docker › Add Container › Template**.
3. Optionally but preferably, fill **Secret key** with the output of `openssl rand -base64 32`, run in Unraid's terminal, so that the keys and tokens Polyfin stores are encrypted. Keep a copy with your backups.

To back up the database every day, set **Backups folder** to a folder of the server, such as `/mnt/user/appdata/polyfin/backups`, then turn **Back up the database every day** on in the admin app, under **Settings › Backups**. The container's user, UID 65532, must be able to write to the folder: create it first in Unraid's terminal with `mkdir -p /mnt/user/appdata/polyfin/backups && chown 65532:65532 /mnt/user/appdata/polyfin/backups`. See [Backups](backups.md). Live TV recordings work the same way, with **Recordings folder** and **Record Live TV**, under **Settings › Recordings**. Every other option is set in the admin app, under **Settings**.

For Jellyfin apps to find Polyfin on the local network by themselves, set the container's **Network Type** to `br0`, which gives it its own address on your network, or to `Host`: on the default `bridge`, their broadcasts do not reach it, even with **Discovery port** published (see [Let apps find Polyfin on the local network](#let-apps-find-polyfin-on-the-local-network)).

To serve HTTPS, add the HTTPS port to the container: **Add another Path, Port, Variable, Label or Device**, of type `Port`, with `8920` as both the container's and the host's port, over TCP. Then choose **Serve HTTPS** in the admin app, under **Settings › Security** (see [HTTPS](administration.md#https)). For **Certificate files**, add a `Path` mapping the folder that holds them, read-only, such as `/mnt/user/appdata/certs` to `/certs`. For **Let's Encrypt**, Unraid's own web interface usually holds ports 80 and 443 on the server: forward the router's port 80 to the server's port 8096 for **HTTP-01**, or its port 443 to the server's 8920 for **TLS-ALPN-01**, rather than mapping them on the container. When the router forwards 443 to 8920, set **Public address**, under **Settings › Notifications**, to `https://your-host`, so that **Redirect HTTP to HTTPS** sends clients to 443.

Containers made from an older template keep working: the variables they set, such as `POLYFIN_CACHE_SIZE` or `POLYFIN_BACKUP_DIR`, are copied into the settings at their first start with a version that made them settings. You can remove them from the container afterwards.

## Health check

The image tells Docker whether Polyfin answers. Every 30 seconds, Docker runs `polyfin healthcheck` in the container, which asks Polyfin for `/healthz` at the address `POLYFIN_LISTEN` gives (`127.0.0.1` when it names no host, or `0.0.0.0` or `::`): the check fails when Polyfin does not answer 200 within 3 seconds. After 3 failures in a row, the container is unhealthy. `docker ps`, and Unraid's **Docker** page, show its health: starting, healthy or unhealthy.

While Polyfin starts, for up to 2 minutes, Docker checks every 2 seconds, and failures do not count yet: the container turns healthy within 2 seconds of Polyfin answering. This needs Docker 25 or later; an older Docker checks every 30 seconds from the start.

What it checks is that Polyfin's process serves. Polyfin answers once it has reached PostgreSQL and created its tables, so the health stays starting while it waits for the database, and turns unhealthy when it still waits 3 checks after those 2 minutes. Once started, Polyfin keeps answering while PostgreSQL is away: a database briefly unreachable shows under **System › Health**, and leaves the container healthy, as restarting Polyfin or leaving it out of a proxy would not bring the database back.

Docker itself does not restart an unhealthy container: `restart: unless-stopped` only restarts a container whose process has ended. Swarm services, and tools that watch containers' health, do restart it.

### Behind Traefik

Traefik's Docker provider leaves out containers whose health is starting or unhealthy: until Polyfin is healthy, Traefik has no route to it, and answers 404 at its address. Checking every 2 seconds while Polyfin starts keeps that to about 2 seconds after each start; with Docker before 25, it can last 30 seconds. Traefik's `allowEmptyServices` option keeps the route, which then answers 503 while the container is not healthy. See [Reverse proxy](reverse-proxy.md).

### Turn it off

With `docker run`, add `--no-healthcheck`; on Unraid, add it to the container's extra parameters, in the advanced view of its template. In Compose, add this to the `polyfin` service:

```yaml
    healthcheck:
      disable: true
```

### Monitoring

Polyfin answers two addresses, without signing in, for monitoring tools:

- `/healthz` answers 200 while Polyfin's process serves, PostgreSQL reachable or not. The health check asks it.
- `/readyz` answers 200 when PostgreSQL is reachable too, and 503 when it is not, or while a backup is being restored.

Both answer `ok` in plain text; `/readyz` answers `database unavailable` with its 503. For Prometheus, Polyfin can also expose its metrics at `/metrics`, with a token: see [Metrics](administration.md#metrics).

## GPU

To convert video on a GPU, give the container access to it. See [Transcoding](transcoding.md) for NVIDIA, AMD and Intel GPUs.
