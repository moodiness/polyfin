<a id="desktop-version"></a>

# Polyfin Desktop

Polyfin Desktop runs Polyfin on your own computer, with Windows, macOS or Linux, without Docker. This page is for anyone who installs it, and its last section for those who build it.

An icon in the system tray starts Polyfin in the background and stops it. On macOS the icon is in the menu bar.

Everything the Docker image brings comes with the app, or is set up at its first start:

- a PostgreSQL 18 database of its own;
- FFmpeg and ffprobe 9.0;
- pg_dump and pg_restore, for backups;
- jellyfin-web, the web player;
- the secret key that encrypts the stored keys and tokens.

The app runs the same server as the Docker image, `polyfin serve`, with the same admin app and the same apps. For a NAS or a server that stays on, use the Docker image instead (see [Installation](installation.md)). A backup moves from one to the other (see [Moving to Docker and back](#moving-to-docker-and-back)).

## Install

Download the package for your system from the files of a GitHub release:

| System | Package |
| --- | --- |
| Windows, x64 or arm64 | `Polyfin-Desktop-<version>-windows-x64-setup.exe` or `-windows-arm64-setup.exe`, an installer |
| macOS 13 or later, Apple silicon | `Polyfin-Desktop-<version>-macos-arm64.dmg` |
| macOS 13 or later, Intel | `Polyfin-Desktop-<version>-macos-x64.dmg` |
| Linux, x64 or arm64 | `polyfin-desktop_<version>_amd64.deb` or `_arm64.deb`, and `Polyfin-Desktop-<version>-linux-x86_64.AppImage` or `-aarch64.AppImage` |

Each package holds the app, `polyfin-desktop`, beside what it runs:

- the server, `polyfin`, the same one as in the Docker image;
- PostgreSQL 18's programs;
- jellyfin-web, unmodified;
- the fallback fonts;
- their licenses, in the folder `licenses`.

FFmpeg is downloaded at the first start (see [First start](#first-start)).

| System | Install | Remove |
| --- | --- | --- |
| Windows | Run the installer. It installs Polyfin for you alone, without administrator rights, in `%LOCALAPPDATA%\Programs\Polyfin`, adds it to the Start menu, and offers to start it at the end. | Uninstall Polyfin under Settings › Apps › Installed apps. Its programs, its Start menu entry and Start with the computer go; the data folder stays (see [Folders](#folders)). |
| macOS | Open the disk image and drag Polyfin to Applications. | Move Polyfin from Applications to the Bin. |
| Linux, `.deb` | Run `sudo apt install ./polyfin-desktop_<version>_amd64.deb`, then start Polyfin from the applications menu. It goes in `/opt/polyfin`, with the command `polyfin-desktop`. | Run `sudo apt remove polyfin-desktop`. |
| Linux, AppImage | No installation: make it executable with `chmod +x`, then run it. | Delete the file. |

- **Linux:** both packages need glibc 2.35 or later, as in Ubuntu 22.04, Debian 12 and later releases. On a system without FUSE, run an AppImage with `--appimage-extract-and-run` after its name.
- **Windows on Arm:** an install of the x64 package moves to the arm64 one with a backup (see [Moving to the arm64 package on Windows on Arm](#moving-to-the-arm64-package-on-windows-on-arm)).

The packages are not signed yet, so Windows and macOS warn before opening them the first time:

- **Windows:** SmartScreen says that it protected your computer. Choose More info, then Run anyway.
- **macOS:** once Polyfin is in Applications, right-click it and choose Open, then Open again in the dialog. Recent versions of macOS offer no Open there. Try to open the app once, then open System Settings › Privacy & Security, and choose Open Anyway beside the line about Polyfin.

## First start

At its first start, Polyfin Desktop:

1. makes a secret key and keeps it in the system's key store (see [The secret key](#the-secret-key));
2. creates its database cluster in the data folder, with a random password, listening on `127.0.0.1` only;
3. downloads FFmpeg and ffprobe 9.0 for the system into the data folder, 60 to 90 MB depending on the system, with the progress in the menu;
4. starts Polyfin, which creates its tables as in Docker;
5. opens the admin app in the browser, at `http://localhost:8096/admin/`.

- **Checksums:** each FFmpeg download is checked against the checksum the app carries. A download that differs is refused.
- **Without a network:** Polyfin starts without FFmpeg, and tries the download again at its next start.

The admin app asks for the setup code to create the administrator (see [Getting started](getting-started.md#create-the-administrator)). The menu shows the code while no administrator exists. Choose it to copy the code and open the admin app again.

### Firewall prompts

Polyfin listens on these ports, and the system asks whether to let it the first time it does:

| Port | For |
| --- | --- |
| 8096 (TCP) | Apps and the browser |
| 7359 (UDP) | Jellyfin apps that look for servers on the local network (see [Local network discovery](https-and-networking.md#local-network-discovery)) |
| 1900 (UDP) | DLNA, once turned on (see [DLNA](dlna.md)) |

- **Windows:** Windows Defender Firewall asks whether to allow Polyfin on private and public networks. Allow it on private networks, so that the TVs, phones and other computers of your home reach it.
- **macOS:** macOS asks whether `polyfin` may accept incoming network connections. Allow it, so that other devices reach it. As the app is not signed, macOS may ask again after an update.
- **Linux:** a firewall such as `ufw` asks nothing. Open the ports yourself, such as with `sudo ufw allow 8096/tcp` and `sudo ufw allow 7359/udp`.

If you refuse, Polyfin still works on the computer itself, at `localhost`, but other devices cannot reach it.

<a id="nvidia-graphics-cards"></a>

## NVIDIA GPUs

On Windows and Linux, Polyfin converts video on an NVIDIA GPU through NVENC. Which FFmpeg it uses depends on the NVIDIA driver:

| NVIDIA driver | What converts the video |
| --- | --- |
| 610.00 or newer | The downloaded FFmpeg 9.0, on the GPU |
| 570 to 609 | An FFmpeg built for these drivers, on the GPU. Polyfin downloads it into the data folder's `tools/nvenc13` the first time it needs it. |
| Older than 570 | Another GPU of the computer, if one encodes, or else the processor |

- **Older cards:** drivers 570 to 609 include the R580 drivers, the last for Maxwell, Pascal and Volta cards: the GeForce GTX 900 and 10 series, and the Quadro M and P series.
- **Until the download ends:** with a driver from 570 to 609, video converts as with an older driver until that FFmpeg is downloaded.
- **System › Health** says why video does not convert on the NVIDIA GPU. See [NVIDIA driver](gpus-and-hdr.md#nvidia-driver).

Polyfin Desktop always runs the FFmpeg it downloads, or the one for older drivers. Another FFmpeg can be given only to `polyfin serve` and Docker (see [Which FFmpeg runs](gpus-and-hdr.md#which-ffmpeg-runs)).

## The menu

The icon shows whether Polyfin runs: it is faded while Polyfin starts or is stopped. Its menu has these items:

| Item | What it does |
| --- | --- |
| The first line | Says what Polyfin is doing: starting, downloading FFmpeg or an update, rebuilding the database's indexes (see [Updates](#updates)), running, stopping, or stopped |
| A second line | Shown after a problem: the key store could not be read, FFmpeg could not be downloaded, or an update could not be installed. Opens the log folder, whose `desktop.log` says why |
| Setup code | Shown until the administrator exists. Copies the code and opens the admin app |
| Open the admin app | Opens `http://localhost:8096/admin/` in the browser |
| Open the web player | Opens the web player at `http://localhost:8096/web/` (see [Web player](web-client.md)) |
| Open the log folder | Opens the folder of the logs in the system's file manager, such as Explorer on Windows or Finder on macOS (see [Folders](#folders)) |
| Copy the secret key | Copies the secret key from the system's key store to the clipboard, to keep with your backups (see [The secret key](#the-secret-key)) |
| Stop Polyfin, Start Polyfin | Stops Polyfin, then its database, or starts them again. The app stays open |
| Start with the computer | Whether the system starts the app when you sign in (see [Start with the computer](#start-with-the-computer)) |
| Install Polyfin `<version>` | Shown when a new version is out. Downloads it, stops Polyfin, installs the new version and quits (see [Updates](#updates)) |
| Quit Polyfin | Stops Polyfin, then its database, and quits. No process is left |

- **Opening the app** while it runs opens the admin app.
- **Language:** the menu speaks the system's language when Polyfin does: English, French, German, Spanish, Italian or Brazilian Portuguese. Otherwise it speaks English.
- **No icon:** when the system shows no icon, such as in a Windows session without its taskbar, Polyfin starts all the same. The app writes an error to `desktop.log` and runs on without it: at once when Windows refuses the icon, otherwise after 30 seconds without it. `polyfin-desktop -quit` quits it then (see [Quitting from a terminal](#quitting-from-a-terminal)).

Polyfin stops only when you stop it:

- **The server ends by itself:** the app starts it again a few seconds later.
- **The database stops by itself:** the app starts it again too.
- **After a crash:** after a crash of the app or of the computer, the next start stops the server and the database left running. For a database that crashed, it clears the lock file, and the database recovers its data as it starts.

### Start with the computer

Start with the computer turns on by itself once the administrator exists, unless you chose otherwise before. The system then starts Polyfin when you sign in, without opening the browser. Uncheck it to start Polyfin yourself.

| System | What it adds | Where the system lists it |
| --- | --- | --- |
| Windows | A value named `io.github.moodiness.polyfin-desktop` under `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run` | Task Manager, under Startup apps |
| macOS | A launch agent, `~/Library/LaunchAgents/io.github.moodiness.polyfin-desktop.plist` | System Settings › General › Login Items, under Allow in the Background |
| Linux | An autostart entry, `~/.config/autostart/polyfin-desktop.desktop` | The desktop's startup applications |

### Quitting from a terminal

`polyfin-desktop -quit` quits the app as Quit Polyfin does. It stops Polyfin, then its database, and waits up to 4 minutes for the app to end.

- **Windows:** the program is `%LOCALAPPDATA%\Programs\Polyfin\polyfin-desktop.exe`.
- **macOS:** the program is `/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop`.

Its exit code says what happened:

| Code | Meaning |
| --- | --- |
| 0 | The app has quit |
| 1 | An error, which it prints |
| 2 | Wrong arguments |
| 3 | No app runs on the data folder |
| 4 | The app still runs after 4 minutes |

It quits the app that uses the same data folder (see [Folders](#folders)). It does so through two files of that folder, which only you can open:

- `desktop.pid`, which the app writes as it starts, with its process number and a random token;
- `desktop.quit`, the request, which must carry that token.

Other users of the computer cannot quit your app. A request that a crash left behind does not quit the next start.

## Folders

Polyfin Desktop keeps its files in the system's usual place:

| System | Data folder |
| --- | --- |
| Windows | `%LOCALAPPDATA%\Polyfin`, such as `C:\Users\Alex\AppData\Local\Polyfin` |
| macOS | `~/Library/Application Support/Polyfin` |
| Linux | `$XDG_DATA_HOME/polyfin`, else `~/.local/share/polyfin` |

The data folder is Polyfin's `POLYFIN_DATA_DIR` (see [Configuration](configuration.md)). It holds:

| Folder or file | What it holds |
| --- | --- |
| `postgresql` | The database cluster, with the database password in `polyfin.password`, which only you can read |
| `tools` | FFmpeg and ffprobe |
| `log` | The logs: `desktop.log`, the app's; `polyfin.log`, the server's, which **System › Logs** shows too; `postgresql.log`, the database's. At a start, a log past 10 MB is renamed with `.1` |
| `backups` | The database backups, while **Back up the database** is on, unless **Backups folder** names another folder (see [Backups](backups.md)) |
| `recordings` | The Live TV recordings, unless **Recordings folder** names another folder (see [Live TV](live-tv.md)) |
| `cache` | The work files: segments being played, guides and picture subtitles being read, parts of the files read, and relayed images. **Conversion folder**, **Folder for files being read** and **Cache folder** can name other folders (see [Data folder](configuration.md#data-folder) and [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)) |
| `themes` | The copy of a web player theme's files, when one is kept (see [Keeping a copy of a theme](themes-and-custom-code.md#keeping-a-copy-of-a-theme)) |
| `updates` | The downloaded update. Until the new version answers, also the copy of the database's folder, `postgresql.previous` (see [Going back to the previous version](#going-back-to-the-previous-version)) |
| `desktop.json` | The app's choice of Start with the computer |
| `desktop-update-pending.json` | The update whose new version has not answered yet, with what was kept to go back |
| `desktop-update-rollback.json` | The version the app went back from, and why |

Uninstalling the app leaves the data folder. Remove the folder to remove everything Polyfin kept.

To reach the database with your own tools:

- **Address:** `127.0.0.1` alone, on a port chosen at the first start and kept while it stays free. Another one is chosen when another program takes it, even as the database starts. The port is in `postgresql/polyfin.port`.
- **User:** `polyfin`, with the password in `polyfin.password`.
- **Settings:** light ones, appended to `postgresql/postgresql.conf`, with 20 connections at most (see [A light PostgreSQL](installation.md#a-light-postgresql)). A setting written below them, or with `ALTER SYSTEM`, wins.
- **Version:** PostgreSQL 18. A `postgresql` folder another major version made does not start: the app says so. Move it with a backup and a restore (see [PostgreSQL versions](installation.md#postgresql-versions)).

## The secret key

Polyfin Desktop makes the secret key at its first start (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)). It keeps the key in the system's key store, never in the data folder:

| System | Key store |
| --- | --- |
| Windows | Windows Credential Manager |
| macOS | The macOS keychain |
| Linux | The Secret Service, such as GNOME Keyring or KWallet |

The key is stored for the service `Polyfin`, under the account `Secret key of <data folder>`. An example is `Secret key of /Users/alex/Library/Application Support/Polyfin`.

Keep a copy with your backups: a backup restored elsewhere needs the key to read its keys and tokens. Choose Copy the secret key in the menu, then paste the key into your password manager or a file kept beside the backups. The item says for a few seconds to keep it with your backups.

The key never leaves the computer otherwise. The admin app does not show it, as administrators may sign in from elsewhere. The system's own tools read it too:

- **macOS:** `security find-generic-password -s Polyfin -a "Secret key of $HOME/Library/Application Support/Polyfin" -w`. The answer starts with `go-keyring-base64:`. The key is what follows, decoded with `base64 -d`.
- **Linux:** `secret-tool lookup service Polyfin username "Secret key of $HOME/.local/share/polyfin"`.
- **Windows:** the key is the generic credential `Polyfin:Secret key of C:\Users\<you>\AppData\Local\Polyfin` in Credential Manager. Credential Manager does not show its password, so use the menu item.

When the key store cannot be read at a start, Polyfin starts without the key, and the menu says so. A Linux session without a Secret Service is one such case. Until a start reads the key again:

- keys and tokens saved meanwhile are stored unencrypted, and are encrypted at that start;
- those encrypted before count as not set.

## Updates

Polyfin looks for new releases while **Check for new versions** is on, in **Settings › General**, as for the Docker image. It looks for pre-releases too while **Include pre-releases** is on (see [New versions](updates-and-restarts.md#new-versions)).

When a release is out, the app asks GitHub for the release's package for its system and the release's signed manifest. It checks the manifest (see [Signed manifests](#signed-manifests)), then the menu offers to install the release.

Installing it:

1. checks again that the manifest has not expired;
2. downloads the package, and checks it against the SHA-256 that both GitHub and the manifest give for it, refusing it otherwise;
3. stops Polyfin and its database;
4. keeps a copy of the database's folder and of the previous app (see [Going back to the previous version](#going-back-to-the-previous-version));
5. installs the new version, in each system's way:

| System | What the update does |
| --- | --- |
| Windows | Copies the install folder to `Polyfin.old` beside it, opens the installer's window as a download does, and quits. Choose Next to replace the app; the installer's last page offers to start the new version |
| macOS | Replaces the app with the one in the disk image, keeps the previous one as `Polyfin.app.old` beside it, and starts the new one |
| Linux, AppImage | Replaces the AppImage, keeps the previous one with `.old` after its name, and starts the new one |
| Linux, `.deb` | Opens the `.deb` in the system's package installer, which keeps nothing |

Polyfin runs on as it was when an update fails. The menu's second line says that the update could not be installed, and the menu offers it again. An update fails when:

- the package cannot be downloaded, checked or installed;
- the copy of the database cannot be made;
- the installer cannot start.

A release whose manifest is refused is not offered. The app's log, `desktop.log`, says why.

The data folder stays as it is. The new version upgrades the database's tables as it starts, as in Docker.

The new version's PostgreSQL may sort text with another ICU version. If it does, Polyfin first rebuilds the database's indexes, which follow the order of text:

- it records the new collation version, so that PostgreSQL stops warning of a mismatch;
- the menu's first line says so meanwhile, for seconds, or a few minutes on a large library;
- the log says what was rebuilt.

A development build offers no update.

The admin app offers the same update to administrators while the menu offers it. It appears as **Install Polyfin `<version>`** in the notice of the new version on **Home** (see [Polyfin Desktop update](updates-and-restarts.md#polyfin-desktop-update)). It does what the menu item does, through the data folder, which only you can open:

- the app writes the update it offers to `desktop-update.json` while the server runs, with the download's progress and whether the last install failed;
- the server, asked by an administrator, writes the app's token and the version to `desktop.install`, which the app looks for twice a second.

A request left by a crash carries another run's token, and installs nothing. A request is ignored when it names a version the app does not offer, or arrives while Polyfin is not running. The server knows it runs in the app from `POLYFIN_DESKTOP=1`, which the app sets.

### Signed manifests

Each release carries `polyfin-<version>.manifest.json`, signed with ed25519 by the release workflow. It lists:

- each package of Polyfin Desktop, with its size and SHA-256;
- the time it was issued;
- the time it expires, 30 days later.

The app carries two public keys: the one the workflow signs with, and a spare. The signing key can then change without a release that cannot install the next one.

The app installs a package only when:

- a key it carries signed the manifest, and nothing in it changed since;
- the manifest is the release's own, and has not expired;
- the manifest lists the package with the size and SHA-256 that GitHub gives for it, and the download matches them.

This adds to GitHub's own checksum. Someone who could replace a package on GitHub, and its checksum with it, cannot sign a manifest that lists it.

- **Renewal:** a workflow renews the manifests of the latest releases every week, so that a release stays installable while it is among the latest.
- **Expired manifests:** they are refused, such as that of a release left behind. This also keeps an older manifest from being passed off again.
- **Releases without a manifest:** they are never offered, such as the releases before signed manifests. Install them by hand from their release page.

### Going back to the previous version

Until the new version's server answers `/healthz`, the app keeps two things:

- the previous app, beside the new one;
- a copy of the database's folder, `updates/postgresql.previous` in the data folder, taken while the database was stopped.

The new version may change the database's tables as it starts, and the previous version refuses tables newer than its own. Once the new server answers, both copies are removed. The database copy takes as much room as the database: an update that cannot make it is not installed.

The app goes back when the new version's server ends before it answers, 3 starts in a row. Quitting and opening the app counts as a start too. Going back:

1. stops the database;
2. puts the copy of the database and the previous app back, removing the new ones;
3. starts the previous version.

**System › Health** then says, under **To look at**, which version did not start and why, as the app's log does. That version is no longer offered; the next one is. A server that runs without answering, such as one still upgrading a large database, is left to finish.

A `.deb` cannot go back. The system's package installer replaces it and keeps no previous app, and the app keeps no copy of the database. Reinstall the previous `.deb` from its release page, and restore a backup if the new version changed the database (see [Backups](backups.md)).

## Restart

**Restart**, on the admin app's **Home**, stops the server, which exits with code 75. The app starts it again within seconds, as when the server ends by itself.

- The menu's first line says that Polyfin is starting meanwhile.
- The database keeps running.
- No restart policy is needed, and the admin app does not warn.

## Moving to Docker and back

The database is a PostgreSQL 18 database like the Docker image's. A backup made on the desktop restores into a Docker server, and back, from the admin app (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)):

1. On the source, choose **Back up now** in the **Database backups** panel of **System › Schedule**, or wait for the daily backup. Copy the backup from the `backups` folder.
2. Give the destination the source's secret key, so that it reads the backup's keys and tokens:
   - **to Docker:** set `POLYFIN_SECRET_KEY` in `.env` to the desktop's key (see [The secret key](#the-secret-key)), then start the container again;
   - **to the desktop:** write Docker's `POLYFIN_SECRET_KEY` in a file and quit Polyfin Desktop. Run `polyfin-desktop -import-secret-key <file>`, which puts the key in the key store in place of the desktop's own, then start the app again. On macOS the program is `/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop`.
3. On the destination, choose **Restore** in the **Database backups** panel of **System › Schedule**. Before restoring, the admin app counts the keys and tokens that the destination's key cannot read, and asks before going ahead.

The `aio` image, which runs its own database in one container (see [One container](installation.md#one-container)), moves the same way. Its setup page restores the backup at its first start (see [Restoring from the setup page](backups.md#restoring-from-the-setup-page)). Copying the `postgresql` folder is not a way to move: a backup is.

Paths do not move with the backup. Local folders, the recordings folder and the backups folder still name folders of the source. Change them on the destination in **Content › Sources**, **Settings › Recordings** and **Settings › Backups**.

## Moving to the arm64 package on Windows on Arm

An install of the x64 package on Windows on Arm runs under Windows' emulation. It keeps working, but stays x64, and its updates are the x64 package's.

Do not move its data folder to the arm64 package as it is. Its database was made by the x64 package's PostgreSQL, another build with another ICU.

Move the database with a backup instead, into a new data folder, as in [Moving to Docker and back](#moving-to-docker-and-back). On the same computer, the secret key and the paths stay the same, and nothing needs importing or changing.

1. In the admin app, turn on **Back up the database** in **Settings › Backups** if it is off. Then choose **Back up now** in the **Database backups** panel of **System › Schedule**.
2. Quit Polyfin from its menu, then uninstall it under Settings › Apps › Installed apps. The data folder stays.
3. Rename the data folder, `%LOCALAPPDATA%\Polyfin`, such as to `Polyfin-x64`.
4. Install the arm64 package. Polyfin makes a new data folder and database, and finds the secret key it kept in Windows Credential Manager for the data folder's path.
5. Create an administrator with the setup code (see [First start](#first-start)). Turn on **Back up the database** in **Settings › Backups**, which a restore needs.
6. Choose **Restore** in the **Database backups** panel of **System › Schedule**, and send the file from `Polyfin-x64\backups` (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)). The backup brings back its users, settings and libraries.

Then finish the move:

- **Recordings:** if Live TV recorded into the data folder, move the `recordings` folder from `Polyfin-x64` into the new data folder. The restored settings name the same folders as before.
- **Older backups:** move those of `Polyfin-x64\backups` too if you want to keep them.
- **Start with the computer:** it turns on again once the administrator exists. Uncheck it in the menu if you had turned it off.
- **Everything else:** it comes back by itself. The arm64 FFmpeg downloads at the first start, and the rest is made again.

Once Polyfin runs as before, remove `Polyfin-x64`.

<a id="development"></a>

## Building and signing the packages

Building and signing the packages, and running the app for development, is in [Polyfin Desktop packages](development.md#polyfin-desktop-packages).
