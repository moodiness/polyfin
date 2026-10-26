# Desktop version

Polyfin Desktop runs Polyfin on the computer you have, Windows, macOS or Linux, without Docker. An icon in the system tray, or in the menu bar on macOS, starts Polyfin in the background and stops it. Everything the Docker image brings comes with it or is set up at its first start: a PostgreSQL 18 database of its own, FFmpeg and ffprobe 9.0, pg_dump and pg_restore for backups, jellyfin-web, and the secret key the stored keys and tokens are encrypted with.

It runs the very server of the Docker image, `polyfin serve`, with the same admin app and the same apps. To run Polyfin on a NAS or a server that stays on, the Docker image remains the way (see [Installation](installation.md)); a backup moves from one to the other (see [Moving to Docker and back](#moving-to-docker-and-back)).

## Install

Each release has a package per system, among the files of its GitHub release:

| System | Package |
| --- | --- |
| Windows, x64 or arm64 | `Polyfin-Desktop-<version>-windows-x64-setup.exe` or `-windows-arm64-setup.exe`, an installer |
| macOS 13 or later, Apple silicon | `Polyfin-Desktop-<version>-macos-arm64.dmg` |
| macOS 13 or later, Intel | `Polyfin-Desktop-<version>-macos-x64.dmg` |
| Linux, x64 or arm64 | `polyfin-desktop_<version>_amd64.deb` or `_arm64.deb`, and `Polyfin-Desktop-<version>-linux-x86_64.AppImage` or `-aarch64.AppImage` |

Each holds the app, `polyfin-desktop`, beside what it runs: the server, `polyfin`, the very one of the Docker image; PostgreSQL 18's programs; jellyfin-web, unmodified; the fallback fonts; and their licenses, in the folder `licenses`. FFmpeg is downloaded at the first start (see [First start](#first-start)).

- **Windows:** the installer installs Polyfin for you alone, without administrator rights, in `%LOCALAPPDATA%\Programs\Polyfin`, adds it to the Start menu, and offers to start it at the end. To remove it, uninstall Polyfin under Settings › Apps › Installed apps: its programs, its Start menu entry and Start with the computer go, and the data folder stays (see [Folders](#folders)). On Windows on Arm, an install of the x64 package moves to the arm64 one with a backup (see [Moving to the arm64 package on Windows on Arm](#moving-to-the-arm64-package-on-windows-on-arm)).
- **macOS:** open the disk image and drag Polyfin to Applications. To remove it, move it from Applications to the Bin.
- **Linux:** install the `.deb` with `sudo apt install ./polyfin-desktop_<version>_amd64.deb`, then start Polyfin from the applications menu. It goes in `/opt/polyfin`, with the command `polyfin-desktop`; `sudo apt remove polyfin-desktop` removes it. An AppImage needs no installation: make it executable with `chmod +x`, then run it. Both need glibc 2.35 or later, as in Ubuntu 22.04 or Debian 12, and later releases. A system without FUSE runs an AppImage with `--appimage-extract-and-run` after its name.

The packages are not signed yet, so Windows and macOS warn before opening them the first time:

- **Windows:** SmartScreen says that it protected your computer. Choose More info, then Run anyway.
- **macOS:** once Polyfin is in Applications, open it with a right click on it and Open, and Open again in the dialog. On recent versions of macOS, which offer no Open there, try to open it once, then open System Settings › Privacy & Security, and choose Open Anyway beside the line about Polyfin.

## First start

At its first start, Polyfin Desktop:

1. makes a secret key and keeps it in the system's key store (see [The secret key](#the-secret-key));
2. creates its database cluster in the data folder, with a random password, listening on `127.0.0.1` only;
3. downloads FFmpeg and ffprobe 9.0 for the system into the data folder, 60 to 90 MB depending on the system: the menu shows the progress. Each download is checked against the checksum the app carries, and one that differs is refused. Without a network, Polyfin starts without them, and tries again at its next start;
4. starts Polyfin, which creates its tables as in Docker, and opens the admin app in the browser, at `http://localhost:8096/admin/`.

The admin app asks for the setup code, to create the administrator (see [Getting started](getting-started.md#create-the-administrator)). The menu shows the code while no administrator exists: choose it to copy the code and open the admin app again.

### Firewall prompts

Polyfin listens on port 8096 for apps and the browser, and on UDP port 7359 for Jellyfin apps that look for servers on the local network (see [Local network discovery](administration.md#local-network-discovery)). The first time it does, the system asks whether to let it:

- **Windows:** Windows Defender Firewall asks whether to allow Polyfin on private and public networks. Allow it on private networks for the TVs, phones and other computers of your home to reach it.
- **macOS:** macOS asks whether `polyfin` may accept incoming network connections. Allow it for other devices to reach it. As the app is not signed, macOS may ask again after an update.

Refused, Polyfin still works on the computer itself, at `localhost`, and other devices cannot reach it. On Linux, a firewall such as `ufw` asks nothing: open the ports yourself, such as `sudo ufw allow 8096/tcp` and `sudo ufw allow 7359/udp`. DLNA, once on, listens on UDP port 1900 too (see [DLNA](dlna.md)).

## NVIDIA graphics cards

On Windows and Linux, Polyfin converts video on an NVIDIA graphics card through NVENC, which the downloaded FFmpeg 9.0 opens only with NVIDIA driver 610.00 or newer. With an older driver, video is converted on another graphics card of the computer if one encodes, or else on the processor, and **System › Health** shows a warning naming the version needed.

Maxwell, Pascal and Volta cards, such as the GeForce GTX 900 and 10 series and the Quadro M and P series, get no driver past NVIDIA's R580 series, so they cannot use NVENC with Polyfin. See [NVIDIA driver](transcoding.md#nvidia-driver).

## The menu

The icon shows whether Polyfin runs: faded while it starts or is stopped. Its menu has:

| Item | What it does |
| --- | --- |
| The first line | What Polyfin is doing: starting, downloading FFmpeg or an update, rebuilding the database's indexes (see [Updates](#updates)), running, stopping, or stopped. A second line tells about a problem, and opens the log folder, whose `desktop.log` tells why: the key store that could not be read, FFmpeg that could not be downloaded, or an update that could not be installed |
| Setup code | Shown until the administrator exists: copies the code and opens the admin app |
| Open the admin app | Opens `http://localhost:8096/admin/` in the browser |
| Open the web client | Opens jellyfin-web, at `http://localhost:8096/web/` (see [Web client](web-client.md)) |
| Open the log folder | Opens the folder of the logs in a window of the system's file manager, such as Explorer on Windows or Finder on macOS (see [Folders](#folders)) |
| Copy the secret key | Copies the secret key from the system's key store to the clipboard, to keep with your backups (see [The secret key](#the-secret-key)) |
| Stop Polyfin, Start Polyfin | Stops Polyfin, then its database, or starts them again, the app staying open |
| Start with the computer | Whether the system starts the app when you sign in (see below) |
| Install Polyfin `<version>` | Shown when a new version is out: downloads it, stops Polyfin, installs the new version and quits (see [Updates](#updates)) |
| Quit Polyfin | Stops Polyfin, then its database, and quits: no process is left |

Opening the app while it runs opens the admin app. The menu speaks the system's language when Polyfin does (English, French, German, Spanish, Italian or Portuguese, in Brazilian Portuguese), and English otherwise. When the system shows no icon, such as a Windows session without its taskbar, Polyfin starts all the same: after 30 seconds without its icon, the app writes an error to `desktop.log` (see [Folders](#folders)) and runs on without it, until `polyfin-desktop -quit` quits it (see [Quitting from a terminal](#quitting-from-a-terminal)).

Polyfin stops by itself only when you stop it. When the server ends by itself, the app starts it again, a few seconds later. When the database stops by itself, it is started again too. After a crash of the app, or of the computer, the next start stops the server and the database that were left running, or clears the lock file of a database that crashed, which recovers its data as it starts.

### Start with the computer

Once the administrator exists, Start with the computer turns on by itself, unless you chose otherwise before: the system starts Polyfin when you sign in, without opening the browser. Uncheck it to start Polyfin yourself. Each system has its own way:

- **Windows:** a value named `io.github.moodiness.polyfin-desktop` under `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run`, which Task Manager lists under Startup apps.
- **macOS:** a launch agent, `~/Library/LaunchAgents/io.github.moodiness.polyfin-desktop.plist`, which System Settings › General › Login Items lists under Allow in the Background.
- **Linux:** an autostart entry, `~/.config/autostart/polyfin-desktop.desktop`, which the desktop's startup applications list.

### Quitting from a terminal

`polyfin-desktop -quit` quits the app as Quit Polyfin does: it stops Polyfin, then its database, and waits until the app has ended, for up to 4 minutes. On Windows the program is `%LOCALAPPDATA%\Programs\Polyfin\polyfin-desktop.exe`, on macOS `/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop`. Its exit code tells what happened:

| Code | Meaning |
| --- | --- |
| 0 | The app has quit |
| 1 | An error, which it prints |
| 3 | No app runs on the data folder |
| 4 | The app still runs after 4 minutes |

It quits the app that uses the same data folder (see [Folders](#folders)), through two files of that folder, which is yours alone: `desktop.pid`, which the app writes as it starts, with its process number and a random token, and `desktop.quit`, the request, which must carry that token. Other users of the computer cannot quit your app, and a request a crash left behind does not quit the next start.

## Folders

Polyfin Desktop keeps its files in the system's usual place:

| System | Data folder |
| --- | --- |
| Windows | `%LOCALAPPDATA%\Polyfin`, such as `C:\Users\Alex\AppData\Local\Polyfin` |
| macOS | `~/Library/Application Support/Polyfin` |
| Linux | `$XDG_DATA_HOME/polyfin`, else `~/.local/share/polyfin` |

The data folder is Polyfin's `POLYFIN_DATA_DIR` (see [Configuration](configuration.md)), and holds:

| Folder or file | What it holds |
| --- | --- |
| `postgresql` | The database cluster, with the database password in `polyfin.password`, readable by you only |
| `tools` | FFmpeg and ffprobe |
| `log` | The logs: `desktop.log`, the app's; `polyfin.log`, the server's, which **System › Logs** shows too; `postgresql.log`, the database's. Each is renamed with `.1` when it passes 10 MB, at a start |
| `backups` | The database backups, while **Back up the database every day** is on, unless **Backups folder** names another (see [Backups](backups.md)) |
| `recordings` | The Live TV recordings, unless **Recordings folder** names another (see [Live TV](live-tv.md)) |
| `cache` | The parts of the files being read, the segments being played, and the guides and picture subtitles being read, emptied at each start, and the relayed images, unless **Conversion folder**, **Folder for files being read** or **Cache folder** names another (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)) |
| `themes` | The copy of a web player theme's files, when one is kept (see [Keeping a copy of a theme](web-client.md#keeping-a-copy-of-a-theme)) |
| `desktop.json` | The app's choice of Start with the computer |

Uninstalling the app leaves the data folder: remove it to remove everything Polyfin kept. The database listens on `127.0.0.1` alone, on a port chosen at the first start and kept while it stays free; its port is in `postgresql/polyfin.port`, and its user is `polyfin`, with the password of `polyfin.password`, to reach it with your own tools.

## The secret key

The key the stored keys and tokens are encrypted with (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)) is made at the first start and kept in the system's key store, never in the data folder: in Windows Credential Manager, in the macOS keychain, or in the Secret Service on Linux, such as GNOME Keyring or KWallet. It is stored for the service `Polyfin`, under the account `Secret key of <data folder>`, such as `Secret key of /Users/alex/Library/Application Support/Polyfin`.

Keep a copy with your backups: a backup restored elsewhere needs it to read its keys and tokens. Copy the secret key, in the menu, copies it to the clipboard, and says for a few seconds to keep it with your backups: paste it into your password manager, or a file kept beside them. The key never leaves the computer otherwise: the admin app does not show it, as administrators may sign in from elsewhere. The system's own tools read it too:

- **macOS:** `security find-generic-password -s Polyfin -a "Secret key of $HOME/Library/Application Support/Polyfin" -w`. The answer starts with `go-keyring-base64:`: the key is what follows, decoded with `base64 -d`.
- **Linux:** `secret-tool lookup service Polyfin username "Secret key of $HOME/.local/share/polyfin"`.
- **Windows:** the key is the generic credential `Polyfin:Secret key of C:\Users\<you>\AppData\Local\Polyfin` of Credential Manager, which does not show its password: use the menu's item.

When the key store cannot be read at a start, such as a Linux session without a Secret Service, Polyfin starts without the key, and the menu says so. Keys and tokens saved meanwhile are stored unencrypted, and are encrypted once the key is back, at the next start that reads it; those encrypted before count as not set until then.

## Updates

While **Check for new versions** is on, under **Settings › General**, Polyfin looks for new releases, as for the Docker image (see [New versions](administration.md#new-versions)). When one is out, the app asks GitHub for the release's package for its system, and the menu offers to install it. Installing it downloads the package, checks it against the SHA-256 GitHub gives for it, refusing it otherwise, stops Polyfin and its database, then:

- **Windows:** opens the installer's window, as a download does, and quits: choose Next to replace the app, and its last page offers to start the new version;
- **macOS:** replaces the app with the one of the disk image, and starts it;
- **Linux:** replaces the AppImage with the new one, and starts it, or opens the `.deb` in the system's package installer.

When the package cannot be downloaded, checked or installed, or the installer cannot start, Polyfin runs on as it was: the menu's second line says that the update could not be installed, and the menu offers it again.

The data folder stays as it is: the new version upgrades the database's tables as it starts, as in Docker. When the new version's PostgreSQL sorts text with another ICU version, it first rebuilds the database's indexes, which follow the order of text, and records the new collation version, so that PostgreSQL stops warning of a mismatch: the menu's first line says so meanwhile, for seconds or a few minutes on a large library, and the log tells what was rebuilt. A development build offers no update.

The admin app offers the same update, as **Install Polyfin `<version>`** in the notice of the new version on **Home**, to administrators, while the menu offers it (see [Restart and updates](administration.md#polyfin-desktop-s-update)). It does what the menu item does, through the data folder, which is yours alone: the app writes the update it offers to `desktop-update.json` while the server runs, with the download's progress and whether the last install failed, and the server, asked by an administrator, writes the app's token and the version to `desktop.install`, which the app looks for twice a second. A request left by a crash carries another run's token, and installs nothing; one for a version the app does not offer, or while Polyfin is not running, is ignored. The server knows it runs in the app from `POLYFIN_DESKTOP=1`, which the app sets.

## Restart

**Restart**, on the admin app's **Home**, stops the server, which exits with code 75, and the app starts it again within seconds, as when the server ends by itself: the menu's first line says Starting meanwhile. The database keeps running. No restart policy is needed, and the admin app does not warn.

## Moving to Docker and back

The database is a PostgreSQL 18 database like the Docker image's, and a backup made on the desktop restores into a Docker server, and back, from the admin app (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)):

1. On the source, make a backup with **Back up now** in the **Database backups** panel of **System › Schedule**, or wait for the daily one, and copy it from the `backups` folder.
2. Give the destination the source's secret key, so that it reads the backup's keys and tokens:
   - to Docker, set `POLYFIN_SECRET_KEY` in `.env` to the desktop's key (see [The secret key](#the-secret-key)), then start the container again;
   - to the desktop, write Docker's `POLYFIN_SECRET_KEY` in a file, quit Polyfin Desktop, then run `polyfin-desktop -import-secret-key <file>`, which keeps it in the key store in place of the desktop's own, and start the app again. On macOS the program is `/Applications/Polyfin.app/Contents/MacOS/polyfin-desktop`.
3. On the destination, restore the backup with **Restore**, in the **Database backups** panel of **System › Schedule**. Before restoring, the admin app counts the keys and tokens the destination's key cannot read, and asks before going ahead.

Paths do not move with the backup: local folders, the recordings folder and the backups folder name folders of the source. Change them on the destination, under **Sources** and **Settings**.

## Moving to the arm64 package on Windows on Arm

Before 0.42.1, Polyfin Desktop ran on Windows on Arm from the x64 package, under Windows' emulation. That install keeps working, but stays x64: its updates are the x64 package's. Its data folder should not go to the arm64 package as it is: its database was made by the x64 package's PostgreSQL, another build with another ICU. The database moves with a backup instead, into a new data folder, as in [Moving to Docker and back](#moving-to-docker-and-back), but on the same computer: the secret key and the paths stay the same, and nothing needs to be imported or changed.

1. In the admin app, turn **Back up the database every day** on under **Settings › Backups** if it is off, then press **Back up now** in the **Database backups** panel of **System › Schedule**.
2. Quit Polyfin from its menu, then uninstall it under Settings › Apps › Installed apps. The data folder stays.
3. Rename the data folder, `%LOCALAPPDATA%\Polyfin`, such as to `Polyfin-x64`.
4. Install the arm64 package. Polyfin makes a new data folder and database, and finds the secret key it kept in Windows Credential Manager for the data folder's path. Create an administrator with the setup code (see [First start](#first-start)), and turn **Back up the database every day** on under **Settings › Backups**, which a restore needs.
5. Restore the backup with **Restore**, in the **Database backups** panel of **System › Schedule**, sending the file from `Polyfin-x64\backups` (see [Restoring a backup from the admin app](backups.md#restoring-a-backup-from-the-admin-app)). The backup brings back its users, settings and libraries.

Then move the `recordings` folder from `Polyfin-x64` into the new data folder if Live TV recorded there, and the older backups of its `backups` folder if you want to keep them: the restored settings name the same folders as before. Nothing else is needed: the arm64 FFmpeg is downloaded at the first start, and the rest is made again. Start with the computer turns on again once the administrator exists: uncheck it in the menu if you had turned it off. Once Polyfin runs as before, remove `Polyfin-x64`.

## Development

For development and tests, these variables of the app change where it runs from. The server never reads them.

| Variable | What it does |
| --- | --- |
| `POLYFIN_DESKTOP_INSTALL_DIR` | Install folder to run the programs from, instead of the app's own: it holds `polyfin` (`polyfin.exe`), `postgresql/bin`, `postgresql/lib` and `postgresql/share`, `jellyfin-web` and `fonts`, as the packages lay them out |
| `POLYFIN_DESKTOP_DATA_DIR` | Data folder, as an absolute path, instead of the system's usual place |
| `POLYFIN_DESKTOP_DATABASE_PORT` | Port of the database, instead of a free one |
| `POLYFIN_DESKTOP_TOOLS` | JSON file of the FFmpeg downloads, instead of the pinned ones: a local server's, for tests |

`polyfin-desktop -headless` runs the app without the icon, the browser and the login item, logging to the terminal too, until it is interrupted or `-quit` quits it. `scripts/desktop-smoke.py` starts it twice, on a data folder of its own, with a local server for FFmpeg and a proxy that refuses every request for outside, and checks that quitting leaves no process: CI runs it on Linux, Windows and macOS. It quits the app with `-quit`, but for the first start on Linux and macOS, which it interrupts; on Windows, it runs the build without a console, as the packages do, the second time with its icon, which must show when explorer.exe runs in the session, and else the app must log that it runs without it. `DESKTOP_SMOKE_NO_TRAY=1` expects the latter even with explorer.exe: CI sets it on Windows on Arm, whose runners' sessions refuse every notification icon, x64 programs' included. The other variables, such as `POLYFIN_LISTEN`, reach the server as they are. See [Development](development.md) to build the server and the admin app; the app itself builds with `go build ./cmd/polyfin-desktop`, which needs cgo on macOS, for the menu bar.

### Building the packages

The scripts of `scripts/packaging` build the packages into `dist/packages`, as CI does, for the version in `VERSION` (`dev` otherwise). Each needs Go, Node.js for the admin app, Python 3, and Docker:

| Script | Builds | Where |
| --- | --- | --- |
| `macos.sh arm64`, `macos.sh x64` | The `.dmg` of Apple silicon or Intel | On a Mac |
| `windows.sh x64` | The Windows x64 installer, with NSIS | On Linux or macOS |
| `windows.sh arm64` | The Windows arm64 installer, with NSIS | On Windows on Arm, in Git Bash |
| `linux.sh amd64`, `linux.sh arm64` | The `.deb` and the AppImage | On Linux or macOS |

Each takes PostgreSQL 18 from a pinned build: EnterpriseDB's binaries for Windows x64 and macOS, which postgresql.org links, and for Linux a build from PostgreSQL's source made by `postgresql-linux.Dockerfile`, on Ubuntu 22.04 with ICU, which runs from any folder. EnterpriseDB makes none for Windows on Arm: `postgresql-windows.sh` builds it from the same source, with PostgreSQL's upstream change for Visual Studio on Arm applied (`postgresql-msvc-arm64.patch`), using Meson and Visual Studio 2022's ARM64 build tools, with ICU's own ARM64 build and zlib, and puts Visual Studio's runtime beside the programs. It then makes a cluster with ICU, and restores a backup made with `pg_dump`. `windows.sh arm64` runs it, and keeps the build in `dist/packaging/postgresql-windows-arm64` while its inputs stay the same: it needs Visual Studio 2022 with the ARM64 build tools, a native Perl such as Strawberry Perl, Ninja and NSIS, as GitHub's `windows-11-arm` runners have them, and Docker only when `POLYFIN_DESKTOP_FILES` names no folder made by `files.sh`. jellyfin-web and the fonts come from the Docker image's stages, through `files.sh`; a Mac without Docker is given the folder it made with `POLYFIN_DESKTOP_FILES`. Every download is checked against its checksum, and kept in `~/.cache/polyfin-packaging`.

`scripts/packaging/test.sh <package>` installs or opens a package on the system it is for, checks what it holds, runs `desktop-smoke.py --installed` on the installed app, which then finds its programs itself, and removes it: CI runs it on Windows, macOS and Linux runners for every package, before a release gets them. On Windows it checks too that the version info of `polyfin.exe` and `polyfin-desktop.exe`, which `windows.sh` adds with Polyfin's icon using go-winres, is the package's. It changes the system it runs on: run it on a throwaway machine or container.
