# Administration

This page covers Polyfin's admin app: its Overview, Schedule, Health, Logs and Settings pages, API keys for other tools, and the administration features Jellyfin apps can use.

## The admin app

The admin app is at `/admin/`. It opens on the **Overview** for administrators. From the [web client](web-client.md), the **Dashboard** link opens it too.

**Compared with Jellyfin:**

- None of the admin app's pages exist in Jellyfin's API. They use Polyfin's admin API only.

## Overview

The **Overview** shows what plays now, updated every 3 seconds. For each playback you see:

- the user, the device and the app;
- the title and the position;
- how it plays: direct play, remux or conversion, with Jellyfin's reasons;
- the user's quality group;
- the resolution and bitrate sent;
- whether the GPU or the CPU encodes.

Below that is the activity log, which you can filter by kind.

### Stopping a playback or sending a message

An administrator can stop a playback, or show a message on its app, when the app accepts remote control. This works as Jellyfin's remote control does. An administrator whose **Can control other users' apps** permission was taken away is refused. See [users](users.md) for permissions.

## Schedule

**Schedule** lists:

- the tasks, with their last run, result, duration and next run; you can run or stop each one by hand;
- the upcoming Live TV recordings (see [Live TV](live-tv.md));
- when the IPTV lists and guides are fetched again (see [IPTV](iptv.md)).

Schedule covers the server's addons, IPTV sources and guides first, then those users keep under **My addons**, each marked with its owner.

## Health

**Health** puts problems first. It then shows:

- **Addons**, from the requests made for apps since start: last answer, last failure and response time. **Check** asks an addon for its manifest once, at most once a minute. For a member's own addon, it reaches only public addresses.
- **IPTV sources** and **guides**.
- **The transcoder**: GPU, encoders, and conversions against the limit (see [transcoding](transcoding.md)).
- **The database** and its size.
- **The source cache**, against `POLYFIN_CACHE_SIZE`.
- **Disk space** for the cache, recordings and backups folders.
- **Backups**: the last run and its result, the last backup made, its file and size, and the next one. A failed backup, or a last backup older than two days, is a problem. See [Backups](backups.md).
- **The thumbnail queue** and paused hosts.
- **Polyfin itself**: memory, goroutines, uptime and version.
- **Stored keys**: whether `POLYFIN_SECRET_KEY` encrypts them, how many are stored unencrypted, and which cannot be decrypted with it (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)). Keys stored unencrypted show as a warning, keys that cannot be decrypted as an error.

Health sends no request outside the server. Like Schedule, it covers the server's addons, IPTV sources and guides first, then those users keep under **My addons**, each marked with its owner. Their problems count in the summary.

## Logs

**Logs** follows the in-memory log, redacted as it is for Jellyfin apps (see [Server logs](#server-logs)). It has a level filter, a search box and a download button.

## Settings

**Settings** is split into sections, with a search box. It ends with the `POLYFIN_*` variables in effect, read only:

- secrets are hidden;
- the database URL shows its host and database only.

A **Web player** link appears when the server serves a web client. See [configuration](configuration.md) for the variables and [web client](web-client.md) for **Settings › Web player**.

## Showing a saved key

Fields that hold a key or a secret hide it. While you type one, an eye button (**Show key**, **Hide key**) shows what you typed. Once saved, the key shows as dots, with its own eye:

- Under **Settings**, administrators can show the server's **PublicMetaDB key**, **TheIntroDB key** and **Trakt client secret**.
- Under **My account › Tracking**, each user can show their own **MDBList** and **PublicMetaDB** API keys. Trakt and Simkl hold tokens rather than keys, which are never shown.

The key is fetched from the server when the eye is clicked, and hidden again on a second click, after a minute, or when you leave the page. Each time, the activity log records who showed which key, never the key itself, and the answer is never cached. A key that cannot be decrypted with `POLYFIN_SECRET_KEY` counts as not saved and cannot be shown.

## API keys and integrations

### API keys

**API keys** let tools such as request managers use the Jellyfin API, as Jellyfin's keys do. You create them in the admin app, or in jellyfin-web's dashboard.

- A key has administrator rights and no user.
- Requests that name a user act for that user. That user's parental control, blocked genres, visible libraries and allowed hours apply.
- Requests that name no user see the server's libraries.
- Polyfin stores only a hash of each key, with its app name, creation date and last use. The admin app shows a key once.
- A key cannot open a WebSocket.

**For app developers:**

- Keys are managed through `/Auth/Keys`.
- Send the key as the token of the `Authorization` header, or as the `ApiKey` query parameter. `api_key` and `X-Emby-Token` work only with legacy authorization.
- A user is named with `userId` or `/Users/{id}/…`. Without one, `/Users/Me` answers 400, as in Jellyfin.
- `/Auth/Keys` lists a key made from a Jellyfin app in full only on the next listing, then its identifier. `DELETE /Auth/Keys/{key}` accepts the identifier as well as the key.

### Devices

Administrators get Jellyfin's device list, with custom names. Deleting a device signs out every user on it.

**For app developers:** devices are at `/Devices`.

### Users from jellyfin-web

Administrators can create, rename and delete users from jellyfin-web, with Polyfin's rules:

- a password of at least 8 characters is required, unlike Jellyfin;
- the last administrator stays.

See [users](users.md).

### Server configuration

Jellyfin apps can read and save the server configuration. It answers Polyfin's server name, language, resume thresholds, Quick Connect and legacy authorization, with Jellyfin's defaults for the rest. Saving applies those fields only, and keeps those left out.

**For app developers:**

- Configuration: `/System/Configuration`.
- Libraries: `/Library/MediaFolders`.
- Storage: `/System/Info/Storage` lists the cache folder and, when `POLYFIN_RECORDINGS_DIR` is set, the recordings folder.
- The authentication providers are available too.

### Activity log

The activity log records:

- sign-ins and failed sign-ins;
- playback start and stop;
- users created, changed or deleted;
- settings saved;
- addons installed or removed.

It keeps 30 days. Its last entries show on the admin app's [**Overview**](#overview), and Jellyfin apps can read it.

### Server logs

The server log is kept in memory. Apps can download its last 5 MB as one file, `polyfin.log`. URLs are reduced to their host, and tokens, keys and passwords are replaced. Logs that apps upload go to the server log, redacted the same way, up to 1 MB each.

**For app developers:**

- Server log: `/System/Logs`.
- Uploaded app logs: `/ClientLog/Document`, 1 MB at most.

### Scheduled tasks

Jellyfin apps see Polyfin's own jobs and can start and stop them:

- signing out unused devices, hourly;
- cleaning the activity log, daily;
- fetching the IPTV channel lists, then the Live TV guides due under **Settings › Live TV**. Polyfin looks for due ones every 30 minutes; started by hand, the task fetches every list and guide;
- refreshing ratings, which asks addons and runs only when started.

Their schedules cannot be changed.

**For app developers:** tasks are at `/ScheduledTasks`; changing a schedule answers 400.

### Forgotten password

1. A user on the local network asks for a PIN from the app's sign-in screen.
2. The PIN, valid 30 minutes, is logged and shown on the admin app's **Users** page, as there is no server file to read it from.
3. Entering it in the app makes it the user's password and signs them out everywhere.

Wrong PINs count as failed sign-ins. Asked from outside the local network, the answer is the same and no PIN is made, as in Jellyfin.
