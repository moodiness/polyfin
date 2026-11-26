# Moonfin

This page is for administrators whose users have Moonfin's apps, and for those users. It covers the companion Polyfin serves them, what it brings, the defaults an administrator sets, and how each user is served or set apart.

Moonfin's apps run on TVs, phones, tablets, desktops and in a browser. On a server, they look for a companion of their own, Moonbase, for features beyond the Jellyfin API. Polyfin answers the companion's routes itself: no plugin to install, and nothing changes for other apps.

## Turning it on

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Serve Moonfin's companion** | **Settings › General** | On | Moonfin's apps find Polyfin's companion. Each user can be set apart (see [A user set apart](#a-user-set-apart)). |
| **Moonfin's defaults** | **Settings › General**, under it | None | The settings Moonfin's apps take for what a user never set (see [The defaults](#the-defaults)). |
| **Messages to Moonfin's apps** | **Settings › General**, under it | None | Notes from you that the apps show in their messages window (see [Server messages](#server-messages)). |

- With the setting off, Polyfin answers Moonfin's apps as a server without the companion. The apps hide what depends on it.
- Playback, home rows, skip buttons and Watch together (SyncPlay in Jellyfin) go through the Jellyfin API: they work either way.

## Connecting an app

Moonfin's apps find the companion on their own once they are signed in:

- **As a Jellyfin server:** add Polyfin by the address Jellyfin apps use, such as `http://192.168.1.10:8096`.
- **As a Silo server:** add it the same way, with **Allow Silo's apps** on (see [Silo's apps](silo-apps.md)). Polyfin lists the companion among the profile's plugins.

Moonfin's setup offers to install Moonbase on a Jellyfin server. That offer can't succeed on Polyfin, which has no plugin catalog, and it is not needed: skip it.

## What Moonfin gets

| Feature | What Polyfin serves |
|---|---|
| Settings synced across devices | Each user's settings, kept on the server, shared by their devices (see [Settings synced across devices](#settings-synced-across-devices)) |
| Live updates | A change saved on one device reaches the user's other open apps at once, on the Jellyfin address |
| Ratings on title pages | The IMDb, critics', audience and Metacritic scores Polyfin keeps from MDBList, with the server's MDBList key |
| TMDB extras | Reviews, a person's IMDb page and social accounts, episode and season ratings, a series' next episode, and details of titles the server doesn't hold, with the server's TMDB key |
| Server messages | Notes the administrator writes, each to its audience and between its dates, in the apps' messages window (see [Server messages](#server-messages)) |
| App reports | **Send report to server** works on the Silo address too, as on the Jellyfin address |
| Custom themes | None: the list is empty, and the apps keep their own themes |

The server's MDBList and TMDB keys stay on the server: Polyfin asks MDBList and TMDB itself. A feature without its key is hidden in the apps. A user who enters their own key in Moonfin keeps it for the features the app runs itself.

### What Moonfin does without

Polyfin tells the apps these features are not served, and the apps hide them:

- the request service and its calendar, download queue and notifications;
- phone notifications and sign-in PINs kept on the server;
- smart search, the seasonal row, custom rows and chart rows;
- anime filler markers, poster labels and retro games;
- watch parties, which Watch together replaces;
- Moonfin's own similar titles: the apps take Polyfin's, which follow each user's taste (see [Movie recommendations](jellyfin-compatibility.md#movie-recommendations));
- Moonfin's web app at `/Moonfin/Web/`: Polyfin serves the web player only.

## Settings synced across devices

A user's settings live in a global profile, shared by every device. A TV, desktop or mobile profile holds only what the user changed for that kind of device.

When an app asks for its settings, each takes the first value set, in this order:

1. The device's profile: TV, Desktop or Mobile.
2. The user's global profile.
3. The administrator's defaults for that kind of device.
4. The administrator's Global defaults.

- The home layout comes whole from the first of these that has one, never mixed.
- The titles a user hid from Continue Watching and Next Up follow every device. A device that brings one back brings it back for all.
- Settings stay as the apps send them, so a setting a newer app adds keeps syncing.
- Each user's settings are theirs alone. Deleting a user deletes them.

## The defaults

The defaults are the settings Moonfin's apps take for what a user never set. They live under **Serve Moonfin's companion**, in **Settings › General**, as four profiles:

| Profile | Applies to |
|---|---|
| Global | Every device |
| TV, Desktop, Mobile | That kind of device, over Global |

Moonfin has many settings, so the defaults come from an account rather than from a form:

1. Set Moonfin up as you want it on one account.
2. In **Moonfin's defaults**, open a profile and choose **Take from an account**. Pick the account and one of its synced profiles, then **Copy**.
3. Adjust the JSON in the editor if needed, then **Save**.

- A setting left out of the defaults stays each user's choice.
- A user's own API keys and hidden titles never go into the defaults.
- The defaults hold 256 KB at most per profile.

Moonfin's apps keep every setting they synced once. A default changed later reaches only the settings a user never set. **Apply to users** writes the saved defaults into the settings of every user who synced:

| Choice | What happens |
|---|---|
| **Merge** | Each setting the defaults set takes their value. Users keep their other settings. The Global defaults also reach each device profile a user has, unless that kind of device has a default of its own. |
| **Replace** | Each user's settings become the defaults. Only their own API keys and hidden titles stay. |

The open apps of the users updated pick the change up at once on the Jellyfin address, and at their next sign-in otherwise.

## A user set apart

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Companion** | The **Moonfin** section of the user's page, under **Users** | Server default | **On** or **Off** sets the user apart from **Serve Moonfin's companion**. |
| **Let them change it** | The same section | On | Off, the user can't change the choice themselves. |
| **Moonfin's companion** | **My account › Moonfin** | Server default | The user's own choice, unless the administrator keeps them from changing it. |

- A user the companion is not served to finds no companion: their apps keep their settings on each device.
- Their synced settings stay on the server, and come back if they are served again.
- An invite's model passes its choice and its lock on to the accounts made with the invite.

## Synced settings of a user

The **Moonfin** section of a user's page, and **My account › Moonfin**, show what the user's apps synced:

- how many settings, in which profiles;
- when they were last saved, and by which app.

**Reset** on the user's page, and **Reset my synced settings** on My account, forget them. The user's devices then start again from the defaults, or from their own settings at their next sync.

## Server messages

**Messages to Moonfin's apps**, under **Serve Moonfin's companion** in **Settings › General**, holds the notes you write to the users of Moonfin's apps: a maintenance, a change, news. The apps show them in their messages window, in the order of the list. **Write a message** opens the form, with a preview of the message as the apps show it:

| Field | What it does |
|---|---|
| **Title** and **Message** | The message's words, a title or a message at least. The message is Markdown: bold, italic, headings, lists, quotes, code and links. Images are left out. 120 and 2,000 characters at most. |
| **Color** | The color the apps give it: white, blue, green, yellow or red. |
| **How it arrives** | **Badge on the menu** marks the messages button unread. **Opens on its own** opens the messages window by itself, once. |
| **Shown to** | **Everyone**, **Chosen users**, picked by name, or **Administrators**. |
| **From** and **Until** | When it shows, in your time zone. Empty: from now on, and with no end. |
| **Link button** | A label and an `http` or `https` address: a button under the message that opens it. |

- Each user gets only the messages meant for them that show now: Polyfin picks them, so a message meant for one user never reaches another.
- A message reaches the open apps of the users it is meant for at once, on the Jellyfin address, when it is saved, deleted, or reaches its start or end. A user whose app was closed gets it at their next sign-in.
- Each app keeps which messages its user read: Polyfin keeps no read state. An edited message keeps its place in the list, and stays read where it was read.
- The arrows beside a message move it up or down the list. A new message comes first. Up to 50 messages are kept, ended ones included, until you delete them.
- A user who is deleted leaves the users a message was chosen for.
- The messages reach Moonfin's apps only: not the web player, other Jellyfin apps or the notification targets.

API details: [Companion routes for Moonfin](jellyfin-api.md#companion-routes-for-moonfin) and [Moonfin in the admin API](jellyfin-api.md#moonfin-in-the-admin-api).
