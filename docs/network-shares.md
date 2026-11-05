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
| SFTP folder | `sftp://` then server and the folder's full path: `sftp://nas.local/srv/media/movies` | Another port than 22 follows the name: `sftp://nas.local:2222/srv/media/movies`. Symbolic links on the server are followed. |
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

Accept it only when the server was reinstalled or its key changed on purpose. Polyfin then saves it, tries again at once the titles that could not play meanwhile, and scans the folder again.

A folder moved to another server forgets the former server's key.

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
  - SFTP: one SSH connection per folder, made again when it drops.
- **Scans** list the share folder by folder. Network listings are slower than a disk's, but a scan reads only new or changed files, so only the first scan is long.
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

An empty prefix of a bucket is an empty folder: S3 cannot tell it from a prefix that does not exist.

As with a local folder, the files found before stay in the library while the share cannot be read. A NAS that is off for a while empties no library.

A share that was read but cannot be listed while watched is a warning under **System › Health**. The reason is `unreachable`, `refused` or `host_key_changed`. It keeps its schedule meanwhile, and is scanned again once it answers.

**System › Health › Network shares** lists each enabled share with how it is watched:

| Status | Meaning |
|---|---|
| **Notified** | Its server tells of its changes. |
| **Polled** | Its folders are listed again every few minutes. The reason shows: a WebDAV server; an SMB server that does not tell of changes, missed one a listing found, or whose notifications were lost and are asked for again; or an S3 bucket, SFTP folder or rclone remote, with how often. |
| **At scans only** | It is listed only at its scans. |
| **Not watched** | It cannot be watched. The reason shows. |

API details: [Network share watching in the health API](jellyfin-api.md#network-share-watching-in-the-health-api).
