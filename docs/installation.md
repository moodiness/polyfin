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

See [Configuration](configuration.md) for every setting, and the [`.env.example`](../.env.example) file for the Compose settings.

### Pin a version

`POLYFIN_VERSION` in `.env` pins a release, such as `0.1.0`. The value `latest` follows stable releases.

### Back up the database

Everything Polyfin keeps, from accounts and watch history to settings, addons and IPTV line-ups, is in its PostgreSQL database. To back it up every day, mount a folder of the server and name it in `POLYFIN_BACKUP_DIR`. In `compose.yaml`, uncomment the two lines of the `polyfin` service:

```yaml
    environment:
      POLYFIN_BACKUP_DIR: /backups
    volumes:
      - ./backups:/backups
```

Create the folder before starting, writable by the container's user (UID 65532):

```sh
mkdir backups && sudo chown 65532:65532 backups
```

See [Backups](backups.md) for the schedule, the number kept, and how to restore one.

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

To back up the database every day, set **Backups folder** to a folder of the server, such as `/mnt/user/appdata/polyfin/backups`, and **Backups folder in the container** to `/backups`. The container's user, UID 65532, must be able to write to the folder: create it first in Unraid's terminal with `mkdir -p /mnt/user/appdata/polyfin/backups && chown 65532:65532 /mnt/user/appdata/polyfin/backups`. See [Backups](backups.md).

## GPU

To convert video on a GPU, give the container access to it. See [Transcoding](transcoding.md) for NVIDIA, AMD and Intel GPUs.
