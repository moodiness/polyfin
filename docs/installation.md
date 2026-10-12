# Installation

This page explains how to install Polyfin with Docker Compose or on Unraid. After installing, continue with [Getting started](getting-started.md).

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

See [Backups](backups.md) for the schedule, the number kept, and how to restore one.

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
- `/readyz` answers 200 when PostgreSQL is reachable too, and 503 when it is not.

Both answer `ok` in plain text; `/readyz` answers `database unavailable` with its 503. For Prometheus, Polyfin can also expose its metrics at `/metrics`, with a token: see [Metrics](administration.md#metrics).

## GPU

To convert video on a GPU, give the container access to it. See [Transcoding](transcoding.md) for NVIDIA, AMD and Intel GPUs.
