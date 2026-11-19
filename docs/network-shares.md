# Network shares

This page is for administrators who want Polyfin to read folders on a NAS, a server or cloud storage without mounting them. It covers SMB shares, WebDAV folders, S3 buckets, SFTP folders and rclone remotes: their addresses, credentials, performance and health. Folders in general are on [Local folders](local-folders.md).

Polyfin reads these shares itself, without mounting them:

- SMB shares (Windows file sharing, which most NAS offer);
- WebDAV folders;
- prefixes of S3-compatible buckets;
- folders of SFTP servers;
- folders of the many storages rclone reaches, through an rclone instance.

To add one, open **Content › Sources**, choose **Add a source**, then **Folder**. Choose **SMB share**, **WebDAV folder**, **S3 bucket**, **SFTP folder** or **rclone remote**. Give its name, its address, its credentials, and what it holds under **It holds**.

Everything else works as for a local folder: naming, matching, NFO files, discs and images, music, unmatched files, links, scans and versions.

Without this, such storage has to be mounted on the host, for example with `rclone mount`, and added as a local folder. That has costs:

- the mount has to be kept alive, and a dropped one leaves the folder `missing`;
- inotify sees none of the changes made on the storage itself;
- the credentials live in the host's configuration, not in Polyfin, which encrypts them when `POLYFIN_SECRET_KEY` is set.

## Addresses

| Kind | Address | Notes |
|---|---|---|
| SMB share | `smb://` then server, share, and folder if the videos are not at its root: `smb://nas.local/media/movies` | Another port than 445 follows the name: `smb://nas.local:4450/media`. SMB 2 and 3 with NTLM sign-in. SMB 1 is not supported. |
| WebDAV folder | Its address: `https://nas.local/dav/movies` | HTTP Basic sign-in. Prefer `https://`: `http://` sends the password unencrypted. |
| S3 bucket | `s3://` then bucket, and prefix if the videos are not at its root: `s3://media/movies` | Takes an **Endpoint** and a **Region** (below). |
| SFTP folder | `sftp://` then server and the folder's full path: `sftp://nas.local/srv/media/movies` | Another port than 22 follows the name: `sftp://nas.local:2222/srv/media/movies`. Symbolic links on the server are followed, except a link to a folder that leads to its own folder or to one above it. |
| rclone remote | `rclone://` then the instance's host and port, and the path: `rclone://rclone:5572/movies` | Read through an rclone instance (below). |

The address holds no user or password. They have fields of their own, and an address holding them is refused.

**S3 buckets:**

- The **Endpoint** is the address of the storage's S3 service, such as `https://s3.example.org`.
- The **Region** is the one requests are signed for. Left empty, it is `us-east-1`, which most S3-compatible storages accept.
- Objects are listed as folders by `/`, with their size and last modified date. A folder is a prefix. The empty objects some tools make for folders are left out.
- Polyfin names the bucket in the request's path (path-style), which every S3-compatible storage answers. At Amazon's own endpoints (`*.amazonaws.com`), the bucket goes in the host instead, when its name allows it.
- A storage that answers a range with `200 OK` and a `Content-Range`, as `rclone serve s3` does, is read as if it answered `206`.

**rclone remotes:** Polyfin reads them through an rclone instance rather than supporting each storage itself.

1. Run rclone's remote control with `--rc-serve` (`rclone rcd --rc-serve`), such as in a container next to Polyfin (see [installation](installation.md#an-rclone-container)).
2. Give the **rclone address**, such as `http://rclone:5572`. It is reached over `http://`.
3. Choose a **Remote** from rclone's configuration. **List the remotes** asks rclone for them (`config/listremotes`).
4. Give the **Path in the remote**.

Folders are listed with `operations/list`. Files are read by ranges from the objects `--rc-serve` serves.

A server sharing over WebDAV already works as a WebDAV folder, `rclone serve webdav` included.

API details: [Share addresses the admin API refuses](jellyfin-api.md#share-addresses-the-admin-api-refuses).

## Credentials

| Kind | Fields | Empty means |
|---|---|---|
| SMB, WebDAV | **User** and **Password** of an account that may read the folder. For an SMB domain account, write `DOMAIN\user`. | A share open to guests |
| S3 bucket | **Access key** and **Secret key** of a key that may list the bucket and read its objects | A bucket everyone may read. Requests are sent unsigned. |
| SFTP folder | **User**, and a **Password**, a **Private key**, or both. The key is an OpenSSH or PEM private key, pasted whole, with its **Passphrase** when it has one. | |
| rclone remote | The user and password of rclone's remote control (`--rc-user` and `--rc-pass`). The storage's own credentials stay in rclone's configuration. | A remote control without them |

A private key that cannot be read, or a passphrase that does not open it, is refused.

**Stored secrets:** passwords, secret keys, private keys and passphrases are kept like the other stored secrets (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)).

- They are encrypted with `POLYFIN_SECRET_KEY` when that key is set.
- They are never shown again, and never written to the log.
- One the key cannot decrypt counts as wrong. The share fails as `refused`, and **System › Health** lists it under **Stored keys**.

**Changing a secret:**

- Edit the folder and choose **Replace** beside the secret. Left alone, the stored one stays.
- A private key is replaced by pasting another, and removed with **Remove the saved key**.
- New credentials scan the share again at once and keep the files found. Titles that could not play meanwhile are tried again at once, rather than 15 minutes later.
- A new address forgets the files found at the former one, links included.

**Where secrets go:** a stored secret is only sent to the server it was given for.

- A folder moved to another server, port or kind of share keeps none of them. So does an S3 bucket given another endpoint.
- **Edit the folder** then asks for them again. Those left empty are removed on save.
- **List the remotes** uses the stored rclone password only at the folder's own rclone address.
- Another folder or bucket on the same server or endpoint keeps them.

## SFTP host keys

The SFTP server's host key is saved at the first connection. Its fingerprint shows under **Edit the folder**.

When the server later offers another key, Polyfin refuses to connect, as `ssh` does:

- the folder fails with `host_key_changed` and keeps its files;
- its page shows **Cannot be read** as soon as the server offers the key, before the next scan;
- **Edit the folder** shows the new key's fingerprint, with **Accept the new key**.

Accept it only when the server was reinstalled or its key changed on purpose, after comparing the fingerprint shown with the server's own. Polyfin accepts the key of that fingerprint and no other: it saves it, tries again at once the titles that could not play meanwhile, and scans the folder again.

When the server has offered yet another key by the time you accept, nothing is accepted and the saved key stays. The page says so and shows the fingerprint now offered, to compare again.

A folder moved to another server, or another port, forgets the former server's key, and saves none of the new server's without your acceptance:

- the new server's key is refused before the password or the private key is sent, and the folder fails with `host_key_changed` and keeps no file;
- until the first connection, **Edit the folder** says the key will be shown at the next connection;
- then it shows the key's fingerprint, with **Accept the new key**, as above.

SFTP folders that had no saved key when Polyfin was updated to 0.84.0 are treated the same way at their next connection, as Polyfin cannot tell whether they moved since their last one. A folder added since saves its first key, as before.

## Performance

- **Reading:** files are read from the share at each request, with byte ranges. Direct play, repackaging, conversions, analysis and thumbnails read them through Polyfin, as they read a local file. A seek asks the share only for the part needed.
- **Source cache:** the videos of S3 buckets, SFTP folders and rclone remotes are read in blocks through Polyfin's source cache, as addons' streams are (see [reading the sources](reading-the-sources.md#reading-the-sources)).
  - They are read ahead of playback, kept on disk for seeks, and limited per host.
  - They count against **Disk space for files being read (GB)**, in **Settings › Playback**.
  - S3 and rclone are read over HTTP with ranges, SFTP in blocks over the folder's SSH connection.
  - Subtitle files, images, NFO files and discs are read by ranges, as on the other shares.
- **Connections:** they are kept between requests:
  - SMB: one session per share, signed in again when it ends, such as when the server restarts;
  - WebDAV, S3 and rclone: HTTP keep-alive;
  - SFTP: one SSH connection per folder, made again when it drops. It opens two sessions in it, one to read files and one to list folders. A server that refuses the second session is listed over the first, each folder read whole and only then checked against the limits below; Polyfin's log tells it once per connection, at Info, with the server's host.
- **Scans** list the share folder by folder. Network listings are slower than a disk's, but a scan reads only new or changed files, so only the first scan is long. The scheduled scans of the folders of one server run one after the other, and those of different servers side by side, so a slow server delays only its own folders.
- **Limits:** one scan reads folders up to 32 deep, 1,000,000 folders and 4,000,000 files, and of one folder 500,000 entries, each named by at most 1,024 bytes. It lasts at most 12 hours. A share that holds more fails its scan with the limit it met (see [Health](#health)): add parts of it as separate folders.
- **Schedule:** shares follow the same schedule as local folders (see [Scanning](local-folders.md#scanning)), and are watched for changes (see [Watching for changes](local-folders.md#watching-for-changes)).
- **Network:** a video plays only as fast as the network carries it. A high bit rate movie needs a steady link to the server, which wired networks give more surely than Wi-Fi.

Blu-ray discs, DVDs and disc images on a share are read on the share itself: see [Discs on shares](discs.md#discs-on-shares).

## Health

A share Polyfin cannot read is reported on its page and under **System › Health**, with why:

| Status | Meaning |
|---|---|
| `unreachable` | The server does not answer: it is off, or the address's server or port is wrong. |
| `refused` | The server refuses the user or password, the access key or secret key, or the private key. Also when `POLYFIN_SECRET_KEY` cannot decrypt the stored one. |
| `missing` | The share, or the folder in it, does not exist. For S3, a bucket the storage does not have. For rclone, a remote the instance does not know, or a folder missing in it. |
| `unreadable` | The user may not list the folder. |
| `not_folder` | The address is a file. |
| `host_key_changed` | An SFTP server offers another host key than the one saved (see [SFTP host keys](#sftp-host-keys)). |
| `too_deep` | The share's folders go more than 32 deep. A link on the server that leads back to a folder above it makes such folders. |
| `too_many_folders` | The share holds more than 1,000,000 folders. |
| `too_many_files` | The share holds more than 4,000,000 files, counting those beside the videos or the tracks, such as subtitles and images. |
| `listing_too_large` | One folder of the share lists more than 500,000 entries, a name longer than 1,024 bytes, or an answer larger than Polyfin reads: 64 MiB from a WebDAV or SFTP server or for a page of a bucket, 256 MiB from rclone. The server's log tells which, and in which folder. |
| `too_slow` | A scan of the share lasted more than 12 hours, and was stopped. What it read stays, and the next scan goes on from there. A share this slow at every scan points to a slow server or network. |

An empty prefix of a bucket is an empty folder: S3 cannot tell it from a prefix that does not exist.

As with a local folder, the files found before stay in the library while the share cannot be read. A NAS that is off for a while empties no library.

A share that met one of the limits of a scan (`too_deep`, `too_many_folders`, `too_many_files`, `listing_too_large` or `too_slow`) keeps its schedule, and can be scanned by hand. Its next scan on schedule counts from the end of the one that stopped. Each scan reads it up to the limit again, so the listings that watch a share for changes have it scanned once more, then no longer until a scan reads it whole. An SMB server that tells of its changes still has it scanned after them.

A share that was read but cannot be listed while watched is a warning under **System › Health**. The reason is `unreachable`, `refused` or `host_key_changed`. It keeps its schedule meanwhile, and is scanned again once it answers.

**System › Health › Network shares** lists each enabled share with how it is watched:

| Status | Meaning |
|---|---|
| **Notified** | Its server tells of its changes. |
| **Polled** | Its folders are listed again every few minutes. The reason shows: a WebDAV server; an SMB server that does not tell of changes, missed one a listing found, or whose notifications were lost and are asked for again; or an S3 bucket, SFTP folder or rclone remote, with how often. |
| **At scans only** | It is listed only at its scans. |
| **Not watched** | It cannot be watched. The reason shows. |

API details: [Network share watching in the health API](jellyfin-api.md#network-share-watching-in-the-health-api).
