# Backups

Everything Polyfin keeps is in its PostgreSQL database: accounts and watch history, settings, addons and libraries, IPTV line-ups and keys. Polyfin can back it up every day into a folder, and keep the newest backups there.

## Turning backups on

Backups are off until `POLYFIN_BACKUP_DIR` names a folder (see [Configuration](configuration.md)). The folder must exist and the container's user, UID 65532, must be able to write to it; otherwise Polyfin does not start.

- **Docker Compose:** uncomment `POLYFIN_BACKUP_DIR: /backups` and the `./backups:/backups` volume of the `polyfin` service in `compose.yaml`. Create the folder first with `mkdir backups && sudo chown 65532:65532 backups`, then run `docker compose up -d`.
- **Unraid:** set **Backups folder** to a folder of the server, such as `/mnt/user/appdata/polyfin/backups`, and **Backups folder in the container** to `/backups`. Create the folder first with `mkdir -p /mnt/user/appdata/polyfin/backups && chown 65532:65532 /mnt/user/appdata/polyfin/backups`.

The Docker image includes `pg_dump` from PostgreSQL 18. Without the image, install PostgreSQL 18's client tools so that `pg_dump` is in `PATH`.

## What Polyfin does

Every day, at the hour set under **Settings › Backups › Back up every day at** (4:00 by default, in the server's time zone), the **Back up the database** task runs `pg_dump`. It writes a full copy of the database in `pg_dump`'s custom format to a file named after its start time, such as `polyfin-20261005-040000.dump`.

- The file is written under a hidden temporary name and renamed when it is complete, so a file named like a backup is always a whole one.
- Only its owner, the container's user, and root can read it: it holds password hashes and the keys of addons and services.
- After each backup, Polyfin deletes its oldest backups past **Backups to keep** (7 by default, from 1 to 90). It deletes only files named like its backups, never other files in the folder.
- The database password is passed to `pg_dump` in its environment, never on its command line or in the logs.

To back up now, press **Back up now** in the **Database backups** panel of **System › Schedule**, or run the **Back up the database** task from the list on the same page.

The **Database backups** panel shows the last run and its result, the last backup made with its file and size, and when the next one is. **Settings › Backups** shows the folder and the last backup. **System › Health** shows the same as the panel. It lists a problem when the last backup failed, or when the last one made is more than two days old.

The folder is on the same server as the database. Copy the backups elsewhere too, for example with your usual file backup, so that a failed disk does not take them with it.

## Restoring a backup

A backup restores into the PostgreSQL 18 container with its own `pg_restore`. The examples below use the Compose services `postgres` and `polyfin`, and the database and user `polyfin` from `.env`. On Unraid, use `docker stop`, `docker exec` and `docker start` with your containers' names instead.

1. Stop Polyfin, so that nothing writes to the database:

   ```sh
   docker compose stop polyfin
   ```

2. Restore the backup into the Polyfin database. `--clean --if-exists` first drops what the database holds:

   ```sh
   docker compose exec -T postgres pg_restore --username=polyfin --dbname=polyfin --clean --if-exists --exit-on-error < backups/polyfin-20261005-040000.dump
   ```

   To keep the current database until the restore has worked, restore into a fresh database instead, then swap the two:

   ```sh
   docker compose exec postgres createdb --username=polyfin polyfin_restored
   docker compose exec -T postgres pg_restore --username=polyfin --dbname=polyfin_restored --exit-on-error < backups/polyfin-20261005-040000.dump
   docker compose exec postgres psql --username=polyfin --dbname=postgres -c 'ALTER DATABASE polyfin RENAME TO polyfin_old' -c 'ALTER DATABASE polyfin_restored RENAME TO polyfin'
   ```

   Once Polyfin runs well on it, drop the old one with `docker compose exec postgres dropdb --username=polyfin polyfin_old`.

3. Start Polyfin:

   ```sh
   docker compose start polyfin
   ```

Polyfin comes back as it was at the time of the backup: the same server identity, so apps reconnect without being set up again, and the same accounts, passwords and settings.

Secrets Polyfin encrypts with `POLYFIN_SECRET_KEY` can be read only with the same key: keep that key with your backups, and set it again before starting Polyfin on a restored database (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
