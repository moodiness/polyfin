# Web client

Polyfin's Docker image includes jellyfin-web, Jellyfin's own web client. This page covers how Polyfin serves it, how it links to the admin app, how to customize it, and its license.

## The built-in web client

The Docker image includes jellyfin-web 12.2. Polyfin serves it at `/web/`, as Jellyfin does, and it connects to the server that served it.

- Once an administrator exists, `http://<server>:8096/` opens the web client.
- Before that, `/` and `/web/` lead to the setup page in the admin app, since no account could sign in yet. See [getting started](getting-started.md).
- `POLYFIN_WEB_DIR` can name another copy of jellyfin-web. Outside the image, without one, there is no web client and `/` leads to the admin app.
- The web client's scripts, styles and other text files go gzip-compressed to browsers that accept it. Each file is compressed once, on its first request, and kept in memory.

**Compared with Jellyfin:**

- Before an administrator exists, Jellyfin's web client opens its startup wizard instead.

## Dashboard links and single sign-on

In the web client, the **Dashboard** (from the menu or the user menu), every dashboard page, the metadata manager, plugin pages and the startup wizard open Polyfin's [admin app](administration.md) at `/admin/` instead.

Polyfin does this with one script of its own added to jellyfin-web's page, `/web/polyfin.js`, which follows the client's routes. It changes none of jellyfin-web's files. On a movie's or an episode's page, the same script:

- adds the title's versions to the page's version menu as the addons answer, without reloading the page;
- keeps the Play, Resume and Play from the start buttons disabled, with a spinner and the tooltip "Looking for sources…", until a version is known, then gives them back in place;
- when every addon has answered without a version, says "No source is available for this title." under the buttons, with a **Try again** button that has Polyfin ask the addons again;
- ends a long version, audio or subtitle name with "…" before its menu's arrow, which stream addons' detailed version names used to run under. The open menu shows whole names.

Its words follow the web client's language: English or French, English for any other. It changes nothing during a video, holds the buttons again when jellyfin-web redraws the page, and gives everything back when you leave it. See [Title pages](playback.md#title-pages).

On the home page, it has the title of a collection library's "Recently Added in" row open the library on the default screen chosen under **Settings › Home**, as its links in the header do, rather than on Suggestions (see [Collections from addons](addons-and-libraries.md#collections-from-addons)).

It leaves out of the top bar, its **More** menu and the side menu the libraries hidden there under **Content › Libraries** or **My sources**, once a user is signed in. They keep their row on the home page (see [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). After a user's first load in a browser, a reload doesn't show them even briefly: the script keeps the last answer in the browser's storage and applies it before jellyfin-web draws its menus.

When the server refuses to play a title, jellyfin-web 12.2 shows the reason, such as "This media cannot be played at this time due to rate limits." for a user playing on as many devices as allowed, then "There was an error processing the request" over it. The script closes that second, generic error as it opens, so only the reason shows. A real Jellyfin server gets the same two errors from jellyfin-web.

### Signing in to the admin app

An administrator signed in to the web client arrives in the admin app signed in. Without a session, the admin app hands over the access token the web client keeps in the browser for this server.

- This opens an admin session, logged as a sign-in.
- It works only for the signed-in device of an administrator who may sign in now. It does not work for an API key, which has no user, for a disabled or blocked account, or outside the account's allowed hours.
- An unknown token counts as a wrong password toward the client's failed attempts.
- It is tried once each time the admin app is opened, so signing out of the admin app holds until then.
- Other users get the sign-in page.

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

**Settings › Web player** in the admin app holds three texts for the web client: **Custom CSS**, **Sign-in message** and **Custom JavaScript**. All are empty by default, which changes nothing. **Open the web player** shows the result.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Custom CSS** | **Settings › Web player** | Empty | CSS applied to every page of the web client. Up to 2 MB. |
| **Sign-in message** | **Settings › Web player** | Empty | Text shown under the sign-in form. Plain text, Markdown or HTML. Up to 8 KB. |
| **Custom JavaScript** | **Settings › Web player** | Empty | Script run on every page of the web client. Up to 2 MB. |

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
