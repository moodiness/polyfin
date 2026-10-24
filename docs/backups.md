# Backups

Everything Polyfin keeps is in its PostgreSQL database: accounts and watch history, settings, addons and libraries, IPTV line-ups and keys. Polyfin can back it up every day into a folder, and keep the newest backups there.

## Turning backups on

Backups are off until **Back up the database every day** is turned on, under **Settings › Backups**. They are written to **Backups folder**: when it is empty, to `backups` in Polyfin's data folder, `/data/backups` in the Docker image's volume, which Polyfin creates. Polyfin must be able to write to the folder, as the container's user, UID 65532: it checks it when the setting is saved, and refuses a folder it cannot write to.

In a default Docker installation, the data volume is on the same disk as the database. To keep the backups on another one, mount a folder of the server at `/data/backups`:

- **Docker Compose:** add `- ./backups:/data/backups` under `volumes` in the `polyfin` service of `compose.yaml`. Create the folder first with `mkdir backups && sudo chown 65532:65532 backups`, then run `docker compose up -d`.
- **Unraid:** set the template's **Backups folder** to a folder of the server, such as `/mnt/user/appdata/polyfin/backups`, which it mounts at `/data/backups`. Create the folder first with `mkdir -p /mnt/user/appdata/polyfin/backups && chown 65532:65532 /mnt/user/appdata/polyfin/backups`.

A folder mounted elsewhere in the container works too: enter its path in **Backups folder**.

The Docker image includes `pg_dump` and `pg_restore` from PostgreSQL 18. Without the image, install PostgreSQL 18's client tools so that both are in `PATH`: `pg_restore` is only needed to restore a backup from the admin app.

## What Polyfin does

Every day, at the hour set under **Settings › Backups › Back up every day at** (4:00 by default, in the server's time zone), the **Back up the database** task runs `pg_dump`. It writes a full copy of the database in `pg_dump`'s custom format to a file named after its start time, such as `polyfin-20261005-040000.dump`.

- The file is written under a hidden temporary name and renamed when it is complete, so a file named like a backup is always a whole one.
- Only its owner, the container's user, and root can read it: it holds password hashes and the keys of addons and services.
- After each backup, Polyfin deletes its oldest backups past **Backups to keep** (7 by default, from 1 to 90). It deletes only files named like its backups, never other files in the folder.
- The database password is passed to `pg_dump` in its environment, never on its command line or in the logs.

To back up now, press **Back up now** in the **Database backups** panel of **System › Schedule**, or run the **Back up the database** task from the list on the same page.

The **Database backups** panel shows the last run and its result, the last backup made with its file and size, when the next one is, each destination's last upload, and the last restore. **Settings › Backups** shows the folder and the last backup. **System › Health** shows the same as the panel. It lists a problem when the last backup failed, or when the last one made is more than two days old.

The folder is on the same server as the database: send the backups to a destination too, so that a failed disk does not take them with it.

## Destinations

Under **Settings › Backups › Destinations**, **Add a destination** offers three kinds, at most 10 destinations in all:

- **S3-compatible storage**, such as Amazon S3, Backblaze B2, Cloudflare R2, MinIO or Wasabi: its **Endpoint** (such as `https://s3.eu-west-3.amazonaws.com`, without the bucket), the **Bucket**, a **Folder prefix** (empty for the bucket's root), the **Region** (empty for `us-east-1`; Cloudflare R2 takes `auto`), the **Access key** and the **Secret key**. Polyfin addresses the bucket in the path of the endpoint's address, which every such storage answers.
- **SFTP**: the **Host**, the **Port** (22 by default), the **User**, a **Password** or a **Private key** (OpenSSH or PEM, without a passphrase), and the **Folder** (empty for the user's home). The server's host key is saved at the first connection, by **Test** or by the first upload, and checked at every connection afterwards: a server offering another key is refused, and its last upload fails saying so. When the server's key changed on purpose, open the destination and press **Forget** next to **Host key**: the key it offers at the next connection is saved. Changing the host or the port forgets it too.
- **WebDAV**: the **Folder address**, the address of an existing folder (such as Nextcloud's `https://cloud.example.org/remote.php/dav/files/sam/polyfin/`), the **User** and the **Password**.

**Test** writes a small file to the destination, reads it back and deletes it. Passwords, secret keys and private keys are encrypted with `POLYFIN_SECRET_KEY` when it is set, like the other stored keys, and never shown again: the form shows that one is saved, and offers to replace it.

Each backup is written to the folder first, then sent to each destination under the same file name. On SFTP and WebDAV servers it is written under a hidden temporary name, renamed once complete; S3 storage makes an object appear only once it is whole. At each backup, Polyfin sends each destination the backups of the folder it keeps but does not have yet: an upload that failed is tried again at the next backup, and a new destination gets the backups already made. **Back up now** sends them too.

### Retention per destination

Each destination has its own **Backups to keep** (7 by default, from 1 to 90). After each upload, Polyfin deletes there its oldest backups past this number. It deletes only files named like its daily backups, directly in the destination's folder or prefix: other files, folders, and what lies outside the prefix are never touched.

### Failures

A destination whose last upload failed shows its error in the **Database backups** panel, under **Settings › Backups**, and as an error under **System › Health**. A destination whose newest backup is more than two days old is a warning there. Both reach the notification targets as Health problems (see [Health problems](notifications.md#health-problems)). A destination that fails does not fail the backup itself: the backup is in the folder, and goes to the destination at the next backup that reaches it.

A destination that stops answering fails too, and Polyfin goes on with the next one: when no byte moves to or from it for 2 minutes, or when it has not taken the backups within 10 minutes plus one second per 32 KB they hold. A restore never waits for an upload: the backup made before it only waits for a backup being written to the folder.

## Restoring a backup from the admin app

**Restore**, in the **Database backups** panel of **System › Schedule**, lists the backups of the folder and of each destination, with their date and size, and also takes a `.dump` file sent from your computer. Choosing one, Polyfin checks it first, without changing anything:

- it must be a backup of Polyfin's database, in `pg_dump`'s custom format;
- a newer Polyfin must not have made it, nor a Polyfin whose database migrations differ: update Polyfin first;
- the keys and passwords it stores must be readable with the current `POLYFIN_SECRET_KEY`.

The confirmation then says what happens, and asks you to type the server's name. When some stored keys of the backup cannot be decrypted with the current key, it counts them by kind: settings, tracking connections, app connections, notification targets, share passwords, backup destinations and the keys of two-step sign-ins. **Cancel** leaves everything as it was: set the key that was set when the backup was made, and restore again. **Restore anyway** goes ahead, and those keys must then be entered again. A user whose two-step sign-in key cannot be decrypted still signs in with a recovery code (see [Two-step sign-in](users.md#for-administrators)).

A backup from a destination is downloaded into the backups folder under a hidden temporary name, and a file sent is written there too: backups must be on, into a folder with room for it. The download stops with an error when the destination sends more bytes than it lists for the file; a WebDAV server that does not tell a file's size lists it without one, and its download is bounded at 64 GB, as a file sent is. The file is deleted when the dialog is closed or another backup is chosen, an hour after it was checked if the restore was not started (or as soon as a restore started past that hour fails), and at Polyfin's next start if it stopped meanwhile. A restore needs `pg_restore` 18, which the Docker image includes; without it, **Restore** says so.

### What a restore does

1. Polyfin backs the current database up first, into the backups folder, as `polyfin-YYYYMMDD-HHMMSS-before-restore.dump`. Retention never deletes it: delete it yourself once it is no longer needed. If this backup fails, nothing is restored.
2. Polyfin stops as it does when its container stops: playbacks, conversions and recordings in progress stop, and the tasks running are cancelled. While the restore runs, apps get `503 Service Unavailable`, and `/healthz` answers.
3. The backup is restored into Polyfin's database in one transaction: what the database user owns there is dropped, and the backup's tables and data are created again. If anything fails, the transaction is rolled back, and the database is exactly as it was.
4. The restored database is migrated to this Polyfin's schema, and records who restored which backup in its activity log.
5. Polyfin starts again on it, in the same process, and the admin app reloads. For a moment while it starts again, connections are refused, `/healthz` included: Docker's health check, which counts a container unhealthy only after 3 failures in a row, rides this out.

Afterwards, everything is as it was when the backup was made, users, settings, addons, watch history and the `devices` and `admin_sessions` tables included. A backup holds the database only: the files of a [copy of a theme](web-client.md#keeping-a-copy-of-a-theme) stay in the data folder, and on another system **Update the copy** downloads them again. Apps signed in after that time must sign in again, and so does the administrator who restored, unless they signed in before it. Stream links handed out before the restore stop working, as the server secret that signs them comes from the backup too. The server identity is the backup's, so apps that knew the server before the backup reconnect without being set up again.

The restore runs inside Polyfin's own database rather than in a new database swapped in for it: it needs no other privilege than owning Polyfin's tables, which every setup grants since Polyfin creates them. It does not need the database user to create or rename databases. The transaction drops and creates every table and index at once, which takes about 1,500 of the server's locks: PostgreSQL's defaults allow 6,400. A server set with a much smaller `max_connections` or `max_locks_per_transaction` may refuse it, and the restore then fails without changing anything. The restore leaves out the `GRANT` statements of the backup: privileges granted to other database users on Polyfin's tables must be granted again.

Jellyfin apps restore a backup of the folder the same way, through `/Backup/Restore` (see [Jellyfin compatibility](jellyfin-compatibility.md#backups)). They cannot show the warning about the key: a backup holding keys the current key cannot decrypt is refused there, and restored from the admin app.

## Restoring a backup by hand

A backup also restores into the PostgreSQL 18 container with its own `pg_restore`, with Polyfin stopped. The examples below use the Compose services `postgres` and `polyfin`, the database and user `polyfin` of `compose.yaml`, and a `backups` folder of the server holding the backups. When they are in Polyfin's data volume, copy the one to restore out of it first: `mkdir -p backups && docker compose cp polyfin:/data/backups/polyfin-20261005-040000.dump backups/`. On Unraid, use `docker stop`, `docker exec` and `docker start` with your containers' names instead.

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
