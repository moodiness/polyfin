# Administration

This page covers Polyfin's admin app: its Home, its System pages (Health, Schedule, Logs and API keys) and Settings, API keys for other tools, and the administration features Jellyfin apps can use.

## The admin app

The admin app is at `/admin/`. From the [web client](web-client.md), the dashboard link opens it too, and jellyfin-web's dashboard pages open their counterpart: its users, libraries, logs, scheduled tasks, API keys and playback settings.

A bar at the top holds the sections, for administrators:

- **Home** (`/admin/`): what plays now and what needs a look;
- **Content**: **Sources** (`/admin/sources`), **Libraries** (`/admin/libraries`) and **Live TV** (`/admin/live-tv`);
- **Users** (`/admin/users`);
- **System**: **Health**, **Schedule**, **Logs** and **API keys** (`/admin/system/…`);
- **Settings**, one page per section (`/admin/settings/general`, `/admin/settings/content`…).

The account menu, on the right, holds **My account**, **My sources** and **Quick Connect** (`/admin/me/…`), the language and **Sign out**. Members see a shorter bar: **Home**, **My sources**, **My account** and **Quick Connect**. On a phone, the bar keeps the logo, the search and a menu button that opens all of these.

**My account** has three sections: **Tracking** (see [Tracking services](tracking.md)), **Devices**, the apps signed in with the account, and **Password**. Signing out a device asks first.

**Search** (⌘K on a Mac, Ctrl K elsewhere, or `/`) finds pages, settings, users and sources by name, with or without accents. Use the arrows and Enter to open one, Escape to close it.

Addresses from before this layout, such as `/admin/health` or `/admin/settings#settings-tracking`, still open the right page.

**Compared with Jellyfin:**

- None of the admin app's pages exist in Jellyfin's API. They use Polyfin's admin API only.

## Home

**Home** greets you by name and shows:

- **Now playing**, updated every 3 seconds. For each playback you see the user, the device and the app, the title and the position, and **How it plays**: direct play, remux or conversion. **Details** adds Jellyfin's reasons, the user's quality group, the resolution and bitrate sent, and whether the GPU or the CPU encodes.
- **To look at**: the problems Health found, with a link to it.
- **Server state**: version, database, cache and graphics card.
- **Recent activity**, the activity log, which you can filter by kind and search. Its list scrolls in its own box and loads older events as you reach its end.

### Stopping a playback or sending a message

An administrator can stop a playback (**Stop**, which asks first), or show a message on its app (**Send a message**), when the app accepts remote control. This works as Jellyfin's remote control does. An administrator whose **Can control other users' apps** permission was taken away is refused. See [users](users.md) for permissions.

## Schedule

**System › Schedule** lists:

- the **Tasks**, with their last run, result and next run; **Run now** and **Stop** run or stop each one by hand;
- **Database backups**, with **Back up now** (see [Backups](backups.md));
- the **Upcoming recordings** (see [Live TV](live-tv.md));
- the **Live TV refreshes**: when the IPTV lists and guides are fetched again (see [IPTV](iptv.md)).

Schedule covers the server's addons, IPTV sources and guides first, then those users keep under **My sources**, each marked with its owner.

## Health

**System › Health** puts problems first, under **Needs attention**. It then shows:

- **Addons**, from the requests made for apps since start: last answer, last failure and response time. **Check** asks an addon for its manifest once, at most once a minute. For a member's own addon, it reaches only public addresses.
- **IPTV sources** and **Programme guides**.
- Under **Server**:
  - the **Transcoder**: GPU, encoders, and conversions against the limit (see [transcoding](transcoding.md));
  - the **Database** and its size;
  - the **Source cache**, against **Disk space for files being read (GB)**;
  - **Disk space** for the cache, recordings and backups folders;
  - **Backups**: the last run and its result, the last backup made, its file and size, and the next one. A failed backup, or a last backup older than two days, is a problem. See [Backups](backups.md);
  - **Thumbnails**: the queue and paused sources;
  - **Polyfin**: memory, goroutines, uptime and version;
  - **Stored keys**: whether `POLYFIN_SECRET_KEY` encrypts them, how many are stored unencrypted, and which cannot be decrypted with it (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)). Keys stored unencrypted show as a warning, keys that cannot be decrypted as an error.

Health sends no request outside the server. Like Schedule, it covers the server's addons, IPTV sources and guides first, then those users keep under **My sources**, each marked with its owner. Their problems count in the summary.

## Logs

**System › Logs** follows the in-memory log, redacted as it is for Jellyfin apps (see [Server logs](#server-logs)). It has a **Level** filter, a search box, **Follow** and **Pause**, and **Download**.

A listing of a library or a collection that takes more than 5 seconds logs "A listing was slow" (Info), with the folder, whether the app asked for every title, how many it got, and where the time went:

- `checked`: the user and the request;
- `read`: the addons and the database;
- `described`: the titles for the app.

## Settings

**Settings** has one page per section, listed on its left, with a search box over all of them (**Search a setting**). Each section page saves only its own changes. Leaving a page with unsaved changes asks first. **Settings › Diagnostics** ends with the `POLYFIN_*` variables in effect, read only:

- secrets are hidden;
- the database URL shows its host and database only.

A **Web player** link appears when the server serves a web client. See [configuration](configuration.md) for the variables and [web client](web-client.md) for **Settings › Web player**.

## Showing a saved key

Fields that hold a key or a secret hide it. While you type one, an eye button (**Show key**, **Hide key**) shows what you typed. Once saved, the key shows as dots, with its own eye:

- Under **Settings**, administrators can show the server's **PublicMetaDB key**, **TheIntroDB key**, **Trakt client secret** and **Last.fm shared secret**.
- Under **My account › Tracking**, each user can show their own **MDBList** and **PublicMetaDB** API keys and their **ListenBrainz user token**. Trakt, Simkl and Last.fm hold tokens or session keys rather than keys, which are never shown.

The key is fetched from the server when the eye is clicked, and hidden again on a second click, after a minute, or when you leave the page. Each time, the activity log records who showed which key, never the key itself, and the answer is never cached. A key that cannot be decrypted with `POLYFIN_SECRET_KEY` counts as not saved and cannot be shown.

## API keys and integrations

### API keys

**API keys** let tools such as request managers use the Jellyfin API, as Jellyfin's keys do. You create them under **System › API keys** (**Create a key**), or in jellyfin-web's dashboard. **Revoke** asks first.

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
- Storage: `/System/Info/Storage` lists the cache folder and, while recording is on, the recordings folder.
- The authentication providers are available too.

### Activity log

The activity log records:

- sign-ins and failed sign-ins;
- playback start and stop;
- users created, changed or deleted;
- settings saved;
- addons installed or removed.

It keeps 30 days. Its last entries show on the admin app's [**Home**](#home), and Jellyfin apps can read it.

### Server logs

The server log is kept in memory. Apps can download its last 5 MB as one file, `polyfin.log`. URLs are reduced to their host, and tokens, keys and passwords are replaced. Logs that apps upload go to the server log, redacted the same way, up to 1 MB each.

**For app developers:**

- Server log: `/System/Logs`.
- Uploaded app logs: `/ClientLog/Document`, 1 MB at most.

### Scheduled tasks

Jellyfin apps see Polyfin's own jobs and can start and stop them:

- signing out unused devices, hourly;
- cleaning the activity log, daily;
- fetching the IPTV channel lists, then the Live TV guides due under **Settings › Live TV**. Polyfin looks for due ones every 5 minutes; started by hand, the task fetches every list and guide;
- refreshing ratings, which asks addons and runs only when started.

Their schedules cannot be changed.

**For app developers:** tasks are at `/ScheduledTasks`; changing a schedule answers 400.

### Forgotten password

1. A user on the local network asks for a PIN from the app's sign-in screen.
2. The PIN, valid 30 minutes, is logged and shown on the user's page under **Users**, as there is no server file to read it from.
3. Entering it in the app makes it the user's password and signs them out everywhere.

Wrong PINs count as failed sign-ins. Asked from outside the local network, the answer is the same and no PIN is made, as in Jellyfin.
