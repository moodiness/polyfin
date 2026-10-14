# Web client

Polyfin's Docker image includes jellyfin-web, Jellyfin's own web client. This page covers how Polyfin serves it, how it links to the admin app, how to customize it, and its license.

## The built-in web client

The Docker image includes jellyfin-web 12.2. Polyfin serves it at `/web/`, as Jellyfin does, and it connects to the server that served it.

- Once an administrator exists, `http://<server>:8096/` opens the web client.
- Before that, `/` and `/web/` lead to the setup page in the admin app, since no account could sign in yet. See [getting started](getting-started.md).
- `POLYFIN_WEB_DIR` can name another copy of jellyfin-web. Outside the image, without one, there is no web client and `/` leads to the admin app.
- The web client's scripts, styles and other text files go gzip-compressed to browsers that accept it. Each file is compressed once, on its first request, and kept in memory.
- On a title's page, the web client shows the title's IMDb rating, critics' score and audience score while the server has an **MDBList API key for ratings**, in place of the star and critic rating it shows otherwise. See [Ratings](addons-and-libraries.md#ratings).

**Compared with Jellyfin:**

- Before an administrator exists, Jellyfin's web client opens its startup wizard instead.

## Dashboard links and single sign-on

In the web client, the **Dashboard** (from the menu or the user menu), every dashboard page, the metadata manager, plugin pages and the startup wizard open Polyfin's [admin app](administration.md) at `/admin/` instead.

Members, who don't have **Dashboard**, get a **Polyfin** entry in its place. It opens the admin app in the same tab, on their own pages: **Home**, **My sources**, **My account** and **Quick Connect**. It shows:

- in the user menu, above **Quick Connect** and **Sign out**;
- on the **Settings** page, before the "User" section;
- with a legacy layout ("Desktop (Legacy)", "Mobile (Legacy)" or "TV"), in the side menu, at the top of the "User" section. Right after the libraries, without the "Administration" heading, it would read as one of them.

The side menu of the default layout, in a narrow window, has no **Dashboard**, so no **Polyfin** entry either.

The entry is named "Polyfin" in every language. Administrators never see it, and it goes when you sign out. After a member's first load in a browser, it shows as soon as jellyfin-web draws its menus.

Polyfin does this with one script of its own added to jellyfin-web's page, `/web/polyfin.js`, which follows the client's routes. It changes none of jellyfin-web's files. On a movie's or an episode's page, the same script:

- adds the title's versions to the page's version menu as the addons answer, without reloading the page. The version selected, the first by default or the one the user picked, stays selected, even when a version that comes is listed before it;
- keeps the Play, Resume and Play from the start buttons disabled, with a spinner and the tooltip "Looking for sources…", until a version is known, then gives them back in place;
- when every addon has answered without a version, says "No source is available for this title." under the buttons, with a **Try again** button that has Polyfin ask the addons again;
- ends a long version, audio or subtitle name with "…" before its menu's arrow, which stream addons' detailed version names used to run under. The open menu shows whole names.

Its words follow the web client's language: English or French, English for any other. It changes nothing during a video, holds the buttons again when jellyfin-web redraws the page, and gives everything back when you leave it. See [Title pages](playback.md#title-pages).

On the home page, it has the title of a collection library's "Recently Added in" row open the library on the default screen chosen under **Settings › Home**, as its links in the header do, rather than on Suggestions (see [Collections from addons](addons-and-libraries.md#collections-from-addons)).

It leaves out of the top bar, its **More** menu and the side menu the libraries hidden there under **Content › Libraries** or **My sources**, once a user is signed in. They keep their row on the home page (see [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). After a user's first load in a browser, a reload doesn't show them even briefly: the script keeps the last answer in the browser's storage and applies it before jellyfin-web draws its menus.

When the server refuses to play a title, jellyfin-web 12.2 shows the reason, such as "This media cannot be played at this time due to rate limits." for a user playing on as many devices as allowed, then "There was an error processing the request" over it. The script closes that second, generic error as it opens, so only the reason shows. A real Jellyfin server gets the same two errors from jellyfin-web.

It adds Polyfin's entries to the ⋮ menus of cards, which a right click on a card also opens, and of a title's page:

- "Remove from Continue Watching", on a card of a Continue Watching row, the home page's or a library's: the title leaves the row, and keeps whether it is played;
- "Hide this title", on a movie, a home video or a series, or on a season or an episode, which hides its series: the title leaves the user's lists, its cards leave the page, and a message says so;
- "Show this title again", in its place, on a title the user hid.

Their words follow the web client's language, English or French. See [Watch state](users.md#watch-state) for what they change.

jellyfin-web keeps what its home rows list for up to a minute, also across a reload, and draws a row again from it without asking. So that a card taken off does not come back meanwhile, the script remembers what it took off for 2 minutes, in the browser tab's session storage, for that server and user only, and leaves it out of the rows. Nothing is kept past that.

On the home page, it adds the rows chosen under **Settings › Web player › Home rows** (see [Home rows](#home-rows)), each at its place among the sections the user chose under jellyfin-web's **Settings › Home**. They look like the library rows: posters, played marks, counts of unplayed episodes and progress bars, with the ⋮ menu and the play button. A card opens the title's page, and the row's name opens its full list. The script asks for the rows each time the home page shows, and puts them back when jellyfin-web redraws the page. It changes nothing elsewhere.

On a movie's or an episode's page, it adds a **Share** button before the ⋮ button, for a user who may share titles while the server allows share links (see [Share links](users.md#share-links)). It opens **My account › Shared links** in the admin app, in the same tab, with the title chosen: the user picks when the link expires and how many may watch at once, then copies the link. A series, a season, a title the user may not see, or a user without the permission gets no button. Its word follows the web client's language, English or French. It changes nothing during a video, comes back when jellyfin-web redraws the page, and goes when you leave the title.

**For app developers:**

- The script asks `GET /Polyfin/Items/{id}/Share`, with the web client's token. It answers `{"Shareable": true, "Url": "/admin/me/account?share=<id>#shared-links"}` when the title may be shared, and `{"Shareable": false, "Url": null}` otherwise.

### Signing in to the admin app

A user signed in to the web client, administrator or member, arrives in the admin app signed in. Without a session, the admin app hands over the access token the web client keeps in the browser for this server.

- This opens a session in the admin app, logged as a sign-in. A member's session opens their own pages only, as when they sign in with their password.
- It works only for the signed-in device of a user who may sign in now. It does not work for an API key, which has no user, for a disabled or blocked account, or outside the account's allowed hours.
- An unknown token counts as a wrong password toward the client's failed attempts.
- It is tried once each time the admin app is opened, so signing out of the admin app holds until then.
- Without a token it may use, the admin app shows its sign-in page.

**For app developers:**

- The token goes to `POST /admin/api/session/jellyfin`, in the `Authorization` header only, never in the URL.

## Caching and security headers

- The page itself is never cached.
- Files whose name or query carries a build hash are cached for a year.
- The other files are revalidated.
- `/web/` gets no Content-Security-Policy, as from Jellyfin: the admin app's would stop jellyfin-web from working. The other security headers stay.

**Compared with Jellyfin:**

- Jellyfin sends no caching header and leaves browsers to guess.

## Customizing the web client

**Settings › Web player** in the admin app holds three texts for the web client: **Custom CSS**, **Sign-in message** and **Custom JavaScript**, one switch, **Add versions to an open title's page**, and the [home rows](#home-rows). By default, **Custom CSS** and **Custom JavaScript** load the LumaaGlaass theme (see [Default theme](#default-theme)), and **Sign-in message** is empty. **Open the web player** shows the result.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Custom CSS** | **Settings › Web player** | The theme's stylesheet | CSS applied to every page of the web client. Up to 2 MB. |
| **Sign-in message** | **Settings › Web player** | Empty | Text shown under the sign-in form. Plain text, Markdown or HTML. Up to 8 KB. |
| **Custom JavaScript** | **Settings › Web player** | The theme's script | Script run on every page of the web client. Up to 2 MB. |
| **Add versions to an open title's page** | **Settings › Web player** | On | Adds the versions the addons find to a title's page while it is open. Off, a page opened before any version was found still lists the first ones, and the play buttons still wait for a version; the others show when the page is opened again. See [Title pages](playback.md#title-pages). |

### Home rows

**Settings › Web player › Home rows** adds rows to the web player's home page. The list is empty by default; it holds up to 20 rows, in order, each with **Edit**, **Delete** and arrows to move it. Each row has:

- a **Name**, its title on the home page;
- a **Kind**, what it lists:
  - **A catalog**: the first page of a catalog of the server's addons, a library or not, such as a trending one;
  - **A collection**: a collection made by users, a smart list shown as a collection, or an addon's collection;
  - **Because you watched**: one row for each of the last movies and series the user played, up to the number chosen (3 by default, 10 at most), of the titles similar to it as [movie recommendations](jellyfin-compatibility.md#movie-recommendations) give them, without those already played. Each row is the name followed by the title it follows, such as "Because you watched Example Movie". There are none while **Similar titles** is off;
  - **New in your libraries**: the titles that came into the catalogs of the user's libraries in the last 7 days, newest first;
- a **Place**: **At the top**, **After Continue Watching**, **After Next Up** or **At the bottom**. When a user's home page doesn't show Continue Watching or Next Up, the row takes the place before: after the rows above it, or at the top;
- **Shown to**: **Every user**, or **Chosen users**;
- **Also a library in other apps**, off by default.

Every row leaves out, for each user, what their other lists leave out: titles above their [parental control](users.md#parental-control), their blocked genres, the titles they hid, and the libraries of the server hidden from them, whose catalog or collection makes no row. A catalog row needs the user to use the server's addons. A row without any title isn't shown. Each user can turn off a row under **My account › Home rows** (see [Users](users.md#home-rows)); jellyfin-web's own sections stay under its **Settings › Home**.

**New in your libraries** counts from the first time Polyfin saw each title in each catalog, as apps list the libraries and as Polyfin reads their first pages every hour. The titles a catalog held when Polyfin first read it, or first read a page of it, were there before: they are not new. A library narrowed to a genre counts that genre's titles. Smart lists, IPTV sources and local folders are left out. A title that leaves a catalog and comes back after 180 days is new again.

No app can be given a new kind of home section. With **Also a library in other apps** on, the row is also a library of its own, without type, as the Watchlist is (see [Tracking](tracking.md)): other apps list it among the user's libraries and show its latest titles on their home. The web player shows it too, in its libraries and with its own "Recently Added" row. A **Because you watched** row is then one library for each title it follows.

**For app developers:** `GET /Polyfin/HomeRows` answers the user's rows, in order, as `{"Items": [...]}`: each with its `Id`, its `Name`, its `Place` (`Top`, `AfterResume`, `AfterNextUp` or `Bottom`) and its first `Items`, as `BaseItemDto`, 20 unless `Limit` asks for up to 100. `Fields` asks for their fields, as listings do. A row's `Id` is a folder: `/Items?ParentId=` lists the whole row, up to 500 titles, and `/Items/{Id}` describes it.

### Default theme

By default, **Custom CSS** imports the stylesheet of [LumaaGlaass](https://github.com/Dyhlio/LumaaGlaass), a theme for jellyfin-web, and **Custom JavaScript** loads its script. Both load the main branch of its repository, so the theme's changes reach the web client without a Polyfin update.

- Each browser loads the theme's files from jsDelivr (`cdn.jsdelivr.net`). A browser that cannot reach it shows the web client's own look.
- jsDelivr takes up to 12 hours to serve a change to the theme, and a browser keeps the files it loaded for up to 7 days.
- Each user chooses the theme's appearance and features on the LumaaGlaass page of the web client's settings menu. The choices are saved with their account, on every device.
- The theme needs jellyfin-web's Dark theme, the default.
- To keep the web client's own look, empty both fields: emptying one leaves half the theme.
- To keep one version of the theme, replace `main` with a commit or a tag of its repository in both addresses.
- A server upgraded from an earlier version gets the theme at its first start if both fields were empty, and moves to its main branch if both still held the theme at the commit Polyfin pinned before. A server whose fields hold its own CSS or script keeps both as they are.

**Compared with Jellyfin:**

- Jellyfin's custom CSS is empty by default, and Jellyfin has no custom JavaScript.

### Custom CSS

**Custom CSS** is Jellyfin's branding CSS. jellyfin-web applies it to every page, unless a user ticks **Disable server-provided custom CSS code** in their display settings. jellyfin-web keeps it for up to a minute before asking again.

**For app developers:**

- Served at `/Branding/Css` and `/Branding/Css.css` (`text/css`, empty when unset), and in `CustomCss` of `/Branding/Configuration`.

### Sign-in message

**Sign-in message** is Jellyfin's `LoginDisclaimer`, shown under jellyfin-web's sign-in form. It can be plain text, Markdown or HTML, which jellyfin-web sanitizes. jellyfin-web keeps it for up to a minute before asking again.

### Branding through the Jellyfin API

Administrators can also set the custom CSS and sign-in message through Jellyfin's branding configuration. Saving replaces the branding whole, as in Jellyfin: a text left out is cleared. Anyone signed in can read it.

**Compared with Jellyfin:**

- Polyfin has no splash screen, so `SplashscreenEnabled` stays false whatever is posted.
- An empty text is left out of the answer, where Jellyfin sends it as `""`.

**For app developers:**

- Set with `POST /System/Configuration/branding`; read with `GET /System/Configuration/branding`.

### Custom JavaScript

When **Custom JavaScript** is set, Polyfin adds a script tag to jellyfin-web's `index.html`, after its own script. The script runs on every page of the web client, after jellyfin-web's scripts. It is never added to the admin app.

The script runs in the browser of every user who opens the web client, with their account. The display setting does not turn it off. Paste only code you trust.

**Compared with Jellyfin:**

- Custom JavaScript has no Jellyfin equivalent.

**For app developers:**

- The tag is `<script src="custom.js?v=<hash>" defer></script>`.
- `/web/custom.js` is served as `application/javascript`. It is cached for a year at the address with the hash of its content, which changes with it, and revalidated at any other address.
- With no script, there is no tag and no `/web/custom.js`.

## License and source code

jellyfin-web is free software by the Jellyfin contributors, and a separate program from Polyfin. Polyfin stays under the [MIT License](../LICENSE).

- The image contains jellyfin-web 12.2, unmodified, under the GNU General Public License, version 2.
- It is copied from the official `jellyfin/jellyfin:12.2` image into `/usr/share/polyfin/jellyfin-web`.
- Its license and a notice are in `/usr/share/doc/jellyfin-web` (from [`third_party/jellyfin-web`](../third_party/jellyfin-web)).
- Its complete source code is attached to every Polyfin release on GitHub as `jellyfin-web-12.2-source.tar.gz`, and is also at <https://github.com/jellyfin/jellyfin-web/tree/v12.2>.
- The Dockerfile pins both, side by side, to be changed together when the client is updated.
