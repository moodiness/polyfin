# Backups

This page is for administrators who want to keep a copy of Polyfin's database and restore it. It covers daily backups, their encryption, destinations away from the server, and restoring from the admin app, the setup page or by hand.

Everything Polyfin keeps is in its PostgreSQL database: accounts and watch history, settings, addons and libraries, IPTV line-ups and keys. Polyfin can back it up every day into a folder, and keep the newest backups there.

## Turning backups on

Turn on **Back up the database every day**, under **Settings › Backups**. Backups are off until then.

| Setting | Where | Default | What it does |
| --- | --- | --- | --- |
| **Back up the database every day** | **Settings › Backups** | Off | Backs the database up every day into the backups folder. |
| **Backups folder** | **Settings › Backups** | Empty, for `backups` in the data folder: `/data/backups` in the Docker image's volume | The folder backups are written to. Polyfin creates the default one. |
| **Back up every day at** | **Settings › Backups** | 4:00 | The hour of the daily backup, in the server's time zone. |
| **Backups to keep** | **Settings › Backups** | 7 | How many of the newest backups the folder keeps, from 1 to 90. |
| **Backup passphrase** | **Settings › Backups** | Empty, for none | Encrypts every backup (see [Encrypting the backups](#encrypting-the-backups)). |

Polyfin must be able to write to the folder, as the container's user, UID 65532. It checks the folder when the setting is saved, and refuses one it cannot write to.

In a default Docker installation, the data volume is on the same disk as the database. To keep the backups on another disk, mount a folder of the server at `/data/backups`:

- **Docker Compose:**
  1. Create the folder: `mkdir backups && sudo chown 65532:65532 backups`.
  2. Add `- ./backups:/data/backups` under `volumes` in the `polyfin` service of `compose.yaml`.
  3. Run `docker compose up -d`.
- **Unraid:**
  1. Create the folder: `mkdir -p /mnt/user/appdata/polyfin/backups && chown 65532:65532 /mnt/user/appdata/polyfin/backups`.
  2. Set the template's **Backups folder** to it. The template mounts it at `/data/backups`.

A folder mounted elsewhere in the container works too: enter its path in **Backups folder**.

The Docker image includes `pg_dump` and `pg_restore` from PostgreSQL 18. Without the image, install PostgreSQL 18's client tools, so that both are in `PATH`. `pg_restore` is only needed to restore a backup from the admin app.

## What Polyfin does

Every day, at the hour of **Back up every day at**, the **Back up the database** task runs `pg_dump`.

- It writes a full copy of the database, in `pg_dump`'s custom format.
- The file is named after its start time, such as `polyfin-20261005-040000.dump`.
- The file is written under a hidden temporary name, then renamed once complete. A file named like a backup is always a whole one.
- Only its owner, the container's user, and root can read it. It holds password hashes and the keys of addons and services.
- The database password reaches `pg_dump` in its environment, never on its command line or in the logs.

After each backup, Polyfin deletes its oldest backups past **Backups to keep**. It deletes only files named like its backups, never other files in the folder.

To back up now, do either:

- press **Back up now** in the **Database backups** panel of **System › Schedule**;
- run the **Back up the database** task from the list on the same page.

Where to see how backups went:

| Where | What it shows |
| --- | --- |
| **Database backups** panel, **System › Schedule** | The last run and its result, the last backup made with its file and size, when the next one is, each destination's last upload, and the last restore |
| **Settings › Backups** | The folder and the last backup |
| **System › Health** | The same as the panel |

**System › Health** lists a problem when the last backup failed, or when the last one made is more than two days old.

The folder is on the same server as the database. Send the backups to a [destination](#destinations) too, so that a failed disk does not take them with it.

## Encrypting the backups

Set a **Backup passphrase**, under **Settings › Backups**, to encrypt every backup as it is written. It takes at least 12 characters.

- `pg_dump`'s output goes through the encryption on its way to the file. The dump is never on disk unencrypted.
- An encrypted backup is named `polyfin-YYYYMMDD-HHMMSS.dump.enc`, in the folder and at every destination.
- A destination then holds nothing it can read.
- Retention counts both kinds of names. The backups made before the passphrase was set stay as they are until retention deletes them.
- The passphrase is stored encrypted with `POLYFIN_SECRET_KEY`, like the other [stored keys](configuration.md#stored-keys-and-tokens), and never shown again.
- When that key cannot decrypt the passphrase, backups fail rather than be written unencrypted: enter the passphrase again.

A lost passphrase means lost backups. Nobody can restore an encrypted backup without the passphrase it was made with, Polyfin included.

- Keep the passphrase somewhere other than the server, such as a password manager.
- Keep an old passphrase as long as backups made with it are kept.

How the encryption works:

- The key is derived from the passphrase with scrypt (N = 2^17, r = 8, p = 1), and a random salt of its own for each file.
- The dump is sealed with AES-256-GCM, in chunks of 64 KiB.
- Each chunk is numbered, the last one is marked, and all are bound to the file's header.
- A backup with a chunk changed, moved, dropped or added, or cut short, is refused whole. Nothing of it is restored.
- A check in the header tells a wrong passphrase from a damaged file.

## Downloading a backup

**Restore**, in the **Database backups** panel, offers **Download** next to each backup of the folder, as it is.

An encrypted backup also offers **Download decrypted**.

- It asks for the backup's passphrase, and saves the `.dump` it holds. Keep that file as safe as the server.
- A download stopped by a damaged chunk fails rather than save part of the dump.

## Destinations

In **Settings › Backups**, under **Destinations**, **Add a destination** offers three kinds. A server has at most 10 destinations in all.

| Kind | What it asks for |
| --- | --- |
| **S3-compatible storage**, such as Amazon S3, Backblaze B2, Cloudflare R2, MinIO or Wasabi | The **Endpoint**, without the bucket, such as `https://s3.eu-west-3.amazonaws.com`; the **Bucket**; a **Folder prefix**, empty for the bucket's root; the **Region**, empty for `us-east-1` (Cloudflare R2 takes `auto`); the **Access key** and the **Secret key** |
| **SFTP** | The **Host**; the **Port**, 22 by default; the **User**; a **Password** or a **Private key** (OpenSSH or PEM, without a passphrase); the **Folder**, empty for the user's home |
| **WebDAV** | The **Folder address**, the address of an existing folder, such as Nextcloud's `https://cloud.example.org/remote.php/dav/files/sam/polyfin/`; the **User** and the **Password** |

- **S3 addressing:** Polyfin addresses the bucket in the path of the endpoint's address, which every such storage answers.
- **SFTP host key:** the server's host key is saved at the first connection, by **Test** or by the first upload. It is checked at every connection afterwards.
  - A server offering another key is refused, and its last upload fails saying so.
  - When the server's key changed on purpose, open the destination and press **Forget** next to **Host key**. The key it offers at the next connection is saved.
  - Changing the host or the port forgets the key too.
- **Test:** writes a small file to the destination, reads it back and deletes it.
- **Passwords, secret keys and private keys:** they are encrypted with `POLYFIN_SECRET_KEY` when it is set, like the other stored keys, and never shown again. The form shows that one is saved, and offers to replace it.

Each backup is written to the folder first, then sent to each destination under the same file name.

- On SFTP and WebDAV servers, it is written under a hidden temporary name, then renamed once complete.
- S3 storage makes an object appear only once it is whole.
- At each backup, Polyfin sends each destination the backups of the folder it keeps but does not have yet.
  - An upload that failed is tried again at the next backup.
  - A new destination gets the backups already made.
- **Back up now** sends them too.

### Retention per destination

Each destination has its own **Backups to keep**: 7 by default, from 1 to 90. After each upload, Polyfin deletes there its oldest backups past this number.

- It deletes only files named like its daily backups, directly in the destination's folder or prefix.
- Other files, folders, and what lies outside the prefix are never touched.

### Failures

A destination whose last upload failed shows it here:

| Where | Shown as |
| --- | --- |
| The **Database backups** panel, and **Settings › Backups** | Its error |
| **System › Health** | An error |

A destination whose newest backup is more than two days old is a warning in **System › Health**. Both reach the notification targets as Health problems (see [Health problems](notifications.md#health-problems)).

A destination that fails does not fail the backup itself. The backup is in the folder, and goes to the destination at the next backup that reaches it.

A destination that stops answering fails too, and Polyfin goes on with the next one. It stops answering when:

- no byte moves to or from it for 2 minutes;
- it has not taken the backups within 10 minutes, plus one second per 32 KiB they hold.

A restore never waits for an upload. The backup made before it only waits for a backup being written to the folder.

## Restoring a backup from the admin app

Open **Restore**, in the **Database backups** panel of **System › Schedule**. It lists the backups of the folder and of each destination, with their date and size. It also takes a `.dump` or `.dump.enc` file sent from your computer.

An encrypted backup asks for its passphrase first.

- Polyfin decrypts it into the backups folder, checking every chunk.
- A wrong passphrase can be typed again.

Then, as for any backup, Polyfin checks it without changing anything:

- it must be a backup of Polyfin's database, in `pg_dump`'s custom format;
- neither a newer Polyfin, nor a Polyfin whose database migrations differ, may have made it: update Polyfin first;
- the keys and passwords it stores must be readable with the current `POLYFIN_SECRET_KEY`.

The confirmation then says what happens, and asks you to type the server's name.

Some stored keys of the backup may not decrypt with the current key. The confirmation then counts them by kind:

- keys of the settings;
- tracking connections;
- app connections;
- notification targets;
- share passwords;
- backup destinations;
- keys of the Let's Encrypt certificate;
- keys of two-step sign-ins.

| Choice | What happens |
| --- | --- |
| **Cancel** | Everything stays as it was. Set the key that was set when the backup was made, and restore again. |
| **Restore anyway** | The restore goes ahead, and those keys must then be entered again. A user whose two-step sign-in key cannot be decrypted still signs in with a recovery code (see [Two-step sign-in](sign-in-and-security.md#for-administrators)). |

Files the restore writes to the backups folder:

- a backup from a destination, downloaded under a hidden temporary name;
- a file sent from your computer;
- the dump an encrypted backup holds, once opened.

So backups must be on, into a folder with room for the backup.

- **Size bounds:** a download stops with an error when the destination sends more bytes than it lists for the file.
  - A WebDAV server that does not tell a file's size lists it without one. Its download is bounded at 64 GiB, as a file sent is.
- **When the file is deleted:**
  - when the dialog is closed, or another backup is chosen;
  - an hour after it was checked, if the restore was not started;
  - as soon as a restore started past that hour fails;
  - at Polyfin's next start, if it stopped meanwhile.
- **`pg_restore`:** a restore needs `pg_restore` 18, which the Docker image includes. Without it, **Restore** says so.

### Restoring from the setup page

A new Polyfin, before any administrator exists, can restore a backup instead of creating one.

1. On the setup page, choose **Restore a backup instead**.
2. Enter the setup code (see [Getting started](getting-started.md)).
3. Choose a `.dump` or `.dump.enc` file, and give the passphrase of an encrypted one.

- **The setup code:** it is checked before the file is received. Wrong codes count against the same limit as a wrong code when creating the administrator.
- **The file:** it goes to the default backups folder, even while backups are off. It passes the same checks as above.
- **The restore:** it runs as below, except that nothing is backed up first. The database holds no administrator, so nothing was set up yet.
- **Afterwards:** Polyfin starts again on the backup. Sign in with one of its administrators.
- Once an administrator exists, the setup page and its restore are gone.
- If the restore fails, Polyfin starts again on the empty database, and prints a new setup code in its log.

### What a restore does

1. Polyfin backs the current database up first, into the backups folder, as `polyfin-YYYYMMDD-HHMMSS-before-restore.dump`.
   - Retention never deletes it: delete it yourself once it is no longer needed.
   - If this backup fails, nothing is restored.
2. Polyfin stops as it does when its container stops.
   - Playbacks, conversions and recordings in progress stop, and the tasks running are cancelled.
   - While the restore runs, apps get `503 Service Unavailable`, and `/healthz` answers.
3. The backup is restored into Polyfin's database, in one transaction.
   - What the database user owns there is dropped, and the backup's tables and data are created again.
   - If anything fails, the transaction is rolled back, and the database is exactly as it was.
4. The restored database is migrated to this Polyfin's schema. Its activity log records who restored which backup.
5. Polyfin starts again on it, in the same process, and the admin app reloads.
   - For a moment while it starts again, connections are refused, `/healthz` included.
   - Docker's health check counts a container unhealthy only after 3 failures in a row, so it rides this out.

Afterwards, everything is as it was when the backup was made: users, settings, addons, watch history, and the `devices` and `admin_sessions` tables.

- **Sign-ins:** apps signed in after that time must sign in again. So does the administrator who restored, unless they signed in before it.
- **Stream links:** those handed out before the restore stop working. The server secret that signs them comes from the backup too.
- **Server identity:** it is the backup's. Apps that knew the server before the backup reconnect without being set up again.
- **Theme files:** a backup holds the database only. The files of a [copy of a theme](themes-and-custom-code.md#keeping-a-copy-of-a-theme) stay in the data folder. On another system, **Update the copy** downloads them again.

What the restore needs from PostgreSQL:

- It runs inside Polyfin's own database, rather than in a new database swapped in for it.
- It needs no other privilege than owning Polyfin's tables, which every setup grants, since Polyfin creates them.
- It does not need the database user to create or rename databases.
- The transaction drops and creates every table and index at once. Measured on Polyfin's database at its current migrations, it holds about 6,150 locks at its end, nearly all in PostgreSQL's shared lock table: one for each table, index and sequence dropped and created, and for the objects that depend on them. A migration takes only the locks of the tables it changes.
- PostgreSQL's shared lock table holds `max_locks_per_transaction` (64) for each of its server processes: about 8,700 with PostgreSQL 18's defaults, 100 connections and the background processes. A server set with a smaller `max_connections` or `max_locks_per_transaction` may refuse the restore, with "out of shared memory". The restore then fails without changing anything. Raise `max_locks_per_transaction` to 128 to keep a margin.
- The restore leaves out the backup's `GRANT` statements. Privileges granted to other database users on Polyfin's tables must be granted again.

Jellyfin apps restore a backup of the folder the same way, through `/Backup/Restore` (see [Jellyfin compatibility](jellyfin-compatibility.md#backups)).

- They cannot show the warning about the key, nor ask for a passphrase.
- A backup holding keys the current key cannot decrypt is refused there with 400.
- An encrypted backup is refused there with 400 too, with a message to restore it from the admin app.

## Restoring a backup by hand

A backup also restores into the PostgreSQL 18 container with its own `pg_restore`, while Polyfin is stopped.

- The examples use the Compose services `postgres` and `polyfin`, and the database and user `polyfin` of `compose.yaml`.
- They read the backups from a `backups` folder of the server.
- When the backups are in Polyfin's data volume, copy the one to restore out of it first: `mkdir -p backups && docker compose cp polyfin:/data/backups/polyfin-20261005-040000.dump backups/`.
- On Unraid, use `docker stop`, `docker exec` and `docker start` with your containers' names instead.
- An encrypted backup restores by hand once decrypted. Download it decrypted from the panel, or restore it from the admin app or the setup page, which decrypt it.

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

Polyfin comes back as it was at the time of the backup. It has the same server identity, so apps reconnect without being set up again, and the same accounts, passwords and settings.

The secrets Polyfin encrypts with `POLYFIN_SECRET_KEY` can be read only with the same key. Set it again before starting Polyfin on a restored database (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
