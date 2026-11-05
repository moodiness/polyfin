# Getting started

This page covers what to do the first time you open Polyfin: creating the administrator and the other accounts, the steps **Home** suggests next, signing in from Jellyfin apps, and choosing the language of generated names. To install Polyfin first, see [Installation](installation.md).

## First run

### Create the administrator

Until an administrator exists, Polyfin prints a one-time setup code in its log, which `docker compose logs polyfin` shows (see [Steps](installation.md#steps)). Enter the code on the setup page to create the administrator. In [Polyfin Desktop](desktop.md#first-start), the menu shows the code until then.

### The Get started card

Once the administrator exists, the admin app's **Home** shows **Get started**: the steps of a good setup, each marked **Done** or **To do**. Each step opens the page where it is set. The card reads the server's state each time Home loads, and again every 10 seconds for what **System › Health** tells:

| Step | Done when | Opens |
|---|---|---|
| **Add a metadata addon** | An enabled Stremio addon of the server gives metadata (its manifest lists the `meta` resource), the addons that the **System › Health** warning about metadata languages counts. | **Content › Sources** |
| **Choose your libraries** | At least one of the server's libraries is enabled. | **Content › Libraries** |
| **Create users or an invite link** | Another account than the first exists, or an invite link is active. | **Users** |
| **Back up the database every day** | **Back up the database every day** is on, as the **Backups** part of **System › Health** tells. | **Settings › Backups** |
| **Set POLYFIN_SECRET_KEY** | `POLYFIN_SECRET_KEY` is set, as the **Stored keys** part of **System › Health** tells (see [Configuration](configuration.md#stored-keys-and-tokens)). | **System › Health**, at **Stored keys** |
| **Serve HTTPS or set a public address** | **Public address** is set, in **Settings › Notifications**, or **Serve HTTPS** is on and **System › Health** finds no problem with its port, its certificate files or Let's Encrypt. | **Settings › Security**, or the page of the HTTPS problem in **System › Health** |
| **Let apps on your network find Polyfin** | **Answer local network discovery** is on, **Address on the local network** is set, and **System › Health** finds no problem with discovery's port or the address it announces. | **Settings › General**, or the page of the discovery problem in **System › Health** |

Discovery asks for **Address on the local network** because, without it, a container on a bridge network announces an address apps cannot reach. See [Local network discovery](administration.md#local-network-discovery).

The card goes away once every step is done. It comes back if a step is undone later, such as backups turned off. **Dismiss** hides it for good in this browser; the steps still show their state on their pages.

### Create users

Create the other accounts with **Create a user**, in **Users**. See [Users](users.md) for more.

Moving from a Jellyfin, Emby or Plex server? **Import from another server**, in **Users** too, creates its accounts in Polyfin and brings over what each user watched: played movies and episodes, resume points and favorites. See [Moving from Jellyfin](users.md#moving-from-jellyfin) and [Moving from Emby and Plex](users.md#moving-from-emby-and-plex).

Moving from an app built on addons, such as Stremio or Nuvio? **Import from an app** brings over what a user watched there. See [Importing from an app](users.md#importing-from-an-app).

### Sign in from Jellyfin apps

Jellyfin apps sign in with these accounts, in one of two ways:

- **Password:** the account's user name and password.
- **Quick Connect:** the app shows a 6-digit code, and a signed-in user approves it on the admin app's **Quick Connect** page. Silo's TV apps show an 8-digit code, approved the same way (see [Signing a TV in with a code](silo-apps.md#signing-a-tv-in-with-a-code)). An account with [two-step sign-in](users.md#two-step-sign-in) signs in to apps this way only.

### Change a password

Users can change their password from Jellyfin apps, or in the admin app's **My account › Password** (**Change password**). Doing so signs their other devices out. When the web player signed the admin app in, the web player in that browser stays signed in too, as the admin app's session lasts as long as its sign-in (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

## Language of generated names

Polyfin generates some names that Jellyfin apps show:

- seasons;
- untitled episodes;
- the type that tells apart libraries with the same name, such as "Popular (Movies)";
- a music addon's "My music", and the Replay and Watchlist views;
- the ending of version names that says how the app plays each, such as "(direct play)" (see [Version names](playback.md#version-names)).

These names are in English, French, German, Spanish, Italian or Brazilian Portuguese. Set the language with **Language of generated names**, in **Settings › General**. It defaults to the language the setup page was in.

A user whose **Metadata language** is one of these languages (any Portuguese counts as Brazilian Portuguese) sees the names of seasons, untitled episodes, library types, "My music" and the Replay and Watchlist views in that language instead. Other metadata languages keep the server's language, and version names always do. See [Metadata language](users.md#metadata-language).

## Next steps

- [Addons and libraries](addons-and-libraries.md): add sources and build libraries.
- [Music addons](addons-and-libraries.md#music-addons): music, audiobooks and podcasts.
- [Local folders](local-folders.md): add your own files and network shares.
- [Torrents](torrents.md): play the torrents addons list.
- [Live TV](live-tv.md): watch and record channels.
- [IPTV](iptv.md): add IPTV sources.
- [Users](users.md): manage accounts and access.
- [Web player](web-client.md): use Polyfin in a browser.
