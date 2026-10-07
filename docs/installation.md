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

Containers made from an older template keep working: the variables they set, such as `POLYFIN_CACHE_SIZE` or `POLYFIN_BACKUP_DIR`, are copied into the settings at their first start with a version that made them settings. You can remove them from the container afterwards.

## GPU

To convert video on a GPU, give the container access to it. See [Transcoding](transcoding.md) for NVIDIA, AMD and Intel GPUs.
