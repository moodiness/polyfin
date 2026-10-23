# Web client

Polyfin's Docker image includes jellyfin-web, Jellyfin's own web client. This page covers how Polyfin serves it, how it links to the admin app, how to customize it, and its license.

## The built-in web client

The Docker image includes jellyfin-web 12.2. Polyfin serves it at `/web/`, as Jellyfin does, and it connects to the server that served it.

- Once an administrator exists, `http://<server>:8096/` opens the web client.
- Before that, `/` and `/web/` lead to the setup page in the admin app, since no account could sign in yet. See [getting started](getting-started.md).
- `POLYFIN_WEB_DIR` can name another copy of jellyfin-web. Outside the image, without one, there is no web client and `/` leads to the admin app.
- The web client's scripts, styles and other text files go gzip-compressed to browsers that accept it. Each file is compressed once, on its first request, and kept in memory.
- On a title's page, the web client shows the title's IMDb rating, critics' score and audience score while the server has an **MDBList API key for ratings**, in place of the star and critic rating it shows otherwise. See [Ratings](addons-and-libraries.md#ratings).
- On the page of a Live TV programme that has not started, the web client shows **Remind me** after the record buttons, with a menu offering **Remind me of every airing**, and **Reminder set**, which removes it, for a programme with a reminder. When no target of the user receives reminders and their apps do not show them, a line under the button says so, with a link to **My account › Notifications**. See [Programme reminders](live-tv.md#programme-reminders).

**Compared with Jellyfin:**

- Before an administrator exists, Jellyfin's web client opens its startup wizard instead.

## Dashboard links and single sign-on

In the web client, the **Dashboard** (from the menu or the user menu), every dashboard page, the metadata manager, plugin pages and the startup wizard open Polyfin's [admin app](administration.md) at `/admin/` instead. A tuner's page under Live TV opens the tuner's source page, and the page to add a tuner opens **Add a source** on its **HDHomeRun tuner** tab (see [HDHomeRun tuners](iptv.md#hdhomerun-tuners)).

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

Its words follow the web client's language: English, French, German, Spanish, Italian or Portuguese (in Brazilian Portuguese), English for any other. It changes nothing during a video, holds the buttons again when jellyfin-web redraws the page, and gives everything back when you leave it. A Live TV recording's page has a single source under the recording's own identifier, as a title's page has while no version is known: Polyfin answers that it has no versions to wait for, so its buttons are given back at once. See [Title pages](playback.md#title-pages).

On the home page, it has the title of a collection library's "Recently Added in" row open the library on the default screen chosen under **Settings › Home**, as its links in the header do, rather than on Suggestions (see [Collections from addons](addons-and-libraries.md#collections-from-addons)).

It leaves out of the top bar, its **More** menu and the side menu the libraries hidden there under **Content › Libraries** or **My sources**, once a user is signed in. They keep their row on the home page (see [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar)). After a user's first load in a browser, a reload doesn't show them even briefly: the script keeps the last answer in the browser's storage and applies it before jellyfin-web draws its menus.

When the server refuses to play a title, jellyfin-web 12.2 shows the reason, such as "This media cannot be played at this time due to rate limits." for a user playing on as many devices as allowed, then "There was an error processing the request" over it. The script closes that second, generic error as it opens, so only the reason shows. A real Jellyfin server gets the same two errors from jellyfin-web.

It adds Polyfin's entries to the ⋮ menus of cards, which a right click on a card also opens, and of a title's page:

- "Remove from Continue Watching", on a card of a Continue Watching row, the home page's or a library's: the title leaves the row, and keeps whether it is played;
- "Hide this title", on a movie, a home video or a series, or on a season or an episode, which hides its series, or on a video of a folder, which hides its folder (see [Folders of videos](addons-and-libraries.md#folders-of-videos)): the title leaves the user's lists, its cards leave the page, and a message says so;
- "Show this title again", in its place, on a title the user hid.

Their words follow the web client's language, as above. See [Watch state](users.md#watch-state) for what they change.

jellyfin-web keeps what its home rows list for up to a minute, also across a reload, and draws a row again from it without asking. So that a card taken off does not come back meanwhile, the script remembers what it took off for 2 minutes, in the browser tab's session storage, for that server and user only, and leaves it out of the rows. Nothing is kept past that.

On the home page, it adds the rows chosen under **Settings › Web player › Home rows** (see [Home rows](#home-rows)), each at its place among the sections the user chose under jellyfin-web's **Settings › Home**. They look like the library rows: posters, played marks, counts of unplayed episodes and progress bars, with the ⋮ menu and the play button. A card opens the title's page, and the row's name opens its full list. The script asks for the rows each time the home page shows, and puts them back when jellyfin-web redraws the page. It changes nothing elsewhere.

On a movie's or an episode's page, it adds a **Share** button before the ⋮ button, for a user who may share titles while the server allows share links (see [Share links](users.md#share-links)). It opens **My account › Shared links** in the admin app, in the same tab, with the title chosen: the user picks when the link expires and how many may watch at once, then copies the link. A series, a season, a title the user may not see, or a user without the permission gets no button. Its word follows the web client's language, as above. It changes nothing during a video, comes back when jellyfin-web redraws the page, and goes when you leave the title.

**For app developers:**

- The script asks `GET /Polyfin/Items/{id}/Share`, with the web client's token. It answers `{"Shareable": true, "Url": "/admin/me/account?share=<id>#shared-links"}` when the title may be shared, and `{"Shareable": false, "Url": null}` otherwise.

jellyfin-web keeps its answers for up to a minute before asking again, and for a day in all, also across a reload. Some are not kept per user: the search page's suggestions and results, a library's Suggestions tab with its Continue Watching, Latest and Next Up, its recommendations, filters and studios. jellyfin-web's own **Sign out** forgets them all. A session that ends otherwise does not, such as a device signed out from the user's **Devices** section, or a browser that lost its sign-in. The next user to sign in in that browser would then see the last user's, titles their parental control forbids included. So when a different user signs in from the one they were kept for, the script has jellyfin-web forget everything it kept, as it does when the language changes. The same user signing in again keeps them.

**Compared with Jellyfin:**

- jellyfin-web keeps these answers the same way on a Jellyfin server, which fills the search page's suggestions too; there, the next user to sign in after a session that ended otherwise sees the last user's.

### Signing in to the admin app

A user signed in to the web client, administrator or member, arrives in the admin app signed in. Without a session, the admin app hands over the access token the web client keeps in the browser for this server.

- This opens a session in the admin app, logged as a sign-in. A member's session opens their own pages only, as when they sign in with their password.
- It works only for the signed-in device of a user who may sign in now. It does not work for an API key, which has no user, for a disabled or blocked account, outside the account's allowed hours, or for an account with [two-step sign-in](users.md#two-step-sign-in), or an administrator required to set it up, which signs in with its password and its code.
- An unknown token counts as a wrong password toward the client's failed attempts.
- It is tried once each time the admin app is opened, so signing out of the admin app holds until then.
- Without a token it may use, the admin app shows its sign-in page.

**For app developers:**

- The token goes to `POST /admin/api/session/jellyfin`, in the `Authorization` header only, never in the URL.

## Caching and security headers

- The page itself is never cached.
- Files whose name or query carries a build hash are cached for a year.
- The other files are revalidated.
- The files of the [default theme](#default-theme), at `/web/polyfin/themes/lumaaglaass/<version>/assets/`, are Polyfin's own: cached for a year at the version of the bundled files, `text/css` and `text/javascript`. Any other version, such as `latest`, is redirected to the current one, and revalidated.
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

By default, **Custom CSS** imports the stylesheet of [LumaaGlaass](https://github.com/Dyhlio/LumaaGlaass), a theme for jellyfin-web, and **Custom JavaScript** loads its script. Polyfin bundles the theme: its files at commit `43dc3e0` of its repository (October 11, 2026, after its version 1.1.0), with two patches of Polyfin's. The theme is updated with Polyfin's releases.

- The web client serves the theme's files itself, under `/web/polyfin/themes/lumaaglaass/`: no browser loads them from elsewhere, and the theme works on a server cut off from the internet.
- Their address carries a version that changes with them, so a browser keeps each file for a year, and gets the new ones with the Polyfin release that brings them.
- Each user chooses the theme's appearance and features on the LumaaGlaass page of the web client's settings menu. The choices are saved with their account, on every device.
- The theme's home carousel, the large rotating artwork above the home rows, is off by default: the home starts with its rows, over a still background. A user turns it on with "Home carousel" on the LumaaGlaass page, then "Save". An administrator can do it for another user from that user's settings.
  - The page starts from the options of **Custom JavaScript**: `homeCarousel: false` there is its default for every user who never saved it. The options it leaves out keep the theme's own recommended values.
  - "Delete saved settings", and "Reset to defaults" or the "Recommended" profile followed by "Save", bring back these defaults, carousel off. The settings a user saved stay theirs.
- The theme needs jellyfin-web's Dark theme, the default.
- To keep the web client's own look, empty both fields: emptying one leaves half the theme. See [Use another theme](#use-another-theme).
- Polyfin's patches, in [`third_party/lumaaglaass`](../third_party/lumaaglaass) with the theme's license:
  - the LumaaGlaass page takes the options of **Custom JavaScript** as its defaults, where the theme's own page ignores them;
  - the theme's script, pasted rather than loaded, takes its extensions from Polyfin, where it would take them from the main branch of the theme's repository.
- `scripts/lumaaglaass.sh` makes the bundled copy from a commit of the theme's repository and applies the patches again, so that each update is a change to review.
- A server upgraded from an earlier version gets the theme at its first start if both fields were empty. A server whose two fields still held an earlier default moves to the bundled copy, with the carousel off. A server whose fields hold its own CSS or script, or an edited default, keeps both as they are.

**Compared with Jellyfin:**

- Jellyfin's custom CSS is empty by default, and Jellyfin has no custom JavaScript.

#### Follow the theme's repository

To get the theme's changes as soon as they are published, rather than with Polyfin's releases, load it from a CDN that serves its repository, as Polyfin did until version 0.44. In **Custom CSS**:

```css
@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');
```

In **Custom JavaScript**, the default script with the address of the theme's script:

```js
(() => {
    window.LumaaGlaassOptions = {
        preferences: true,
        homeCarousel: false,
        collectionFilter: true
    };

    const id = 'lg-script';
    if (document.getElementById(id)) return;

    const script = document.createElement('script');
    script.id = id;
    script.src = 'https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.js';
    script.onerror = () => {
        script.remove();
        console.error('LumaaGlaass could not be loaded.');
    };

    document.head.appendChild(script);
})();
```

- **Security:** the script then changes with the theme's repository, without review by Polyfin or by you, and runs in the browser of every user who opens the web client, with their account. Replace `main` with a commit or a tag of the repository, in both addresses, to keep one version you have read.
- The CDN sees the address of every browser that opens the web client. A browser that cannot reach it shows the web client's own look.
- The CDN takes up to 12 hours to serve a change to the theme, and a browser keeps the files it loaded for up to 7 days.
- Without Polyfin's patches, the LumaaGlaass page ignores `homeCarousel` in the options: users who never saved it get the carousel on. `homeCarousel: false` hides it for every user only with `preferences: false`, which also removes the LumaaGlaass page.
- To come back to the bundled copy, paste the defaults back in both fields. The admin API gives them in its answer to `GET /admin/api/settings`, as `bounds.customCss.default` and `bounds.customJs.default`.

### Use another theme

**Settings › Web player** has no list of themes: **Custom CSS** and **Custom JavaScript** hold the theme, and any theme made for jellyfin-web can go there instead of LumaaGlaass. Both are server settings, the same with Polyfin Desktop and with Docker: there is no folder of themes to fill, and Polyfin runs no Jellyfin plugins.

- **jellyfin-web's own look:** empty both fields. Each user can then pick one of jellyfin-web's display themes, such as Light or Blue Radiance, under the web client's "Settings › Display › Theme". With LumaaGlaass, only Dark fits.
- **A theme made of CSS only:** empty **Custom JavaScript**, and put the theme in **Custom CSS**: its stylesheet pasted whole, up to 2 MB, or an `@import` of its address, such as `@import url('https://themes.example/theme.css');`. Emptying only one field leaves half of LumaaGlaass.
- **A theme with a script:** its stylesheet in **Custom CSS**, as above, and in **Custom JavaScript** its script pasted whole, or a loader like the default one, which adds it to the page once:

  ```js
  (() => {
      const id = 'my-theme-script';
      if (document.getElementById(id)) return;

      const script = document.createElement('script');
      script.id = id;
      script.src = 'https://themes.example/theme.js';
      document.head.appendChild(script);
  })();
  ```

  Set the options the theme's documentation gives before the script loads, as the default one does with `window.LumaaGlaassOptions`.
- **Hosting a theme's files yourself:** so that browsers load nothing from elsewhere, and the theme changes only when you update it, serve its files from a web server of your own, such as the reverse proxy in front of Polyfin, and use their addresses in both fields. Keep the theme's folder as it is, as a script often loads more files from beside it. Serve stylesheets as `text/css` and scripts as `text/javascript`: browsers refuse them as anything else. Pasting a theme whole needs no server at all.
- **One user's own look:** each user can add CSS of their own under the web client's "Settings › Display", in "Custom CSS code". It applies for them only, along with the server's.
- **Security:** **Custom JavaScript** runs in the browser of every user who opens the web client, with their account. **Disable server-provided custom CSS code**, in a user's display settings, turns off the server's **Custom CSS** only: a theme's script keeps running. Load or paste only code you trust, and pin a theme loaded from its repository to a commit or a tag.

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

The default theme, LumaaGlaass by Om Gupta and Dyhlio, is under the MIT License. Polyfin embeds its files, with two patches, and serves them to the web client (see [Default theme](#default-theme)). Its license and a notice naming the commit are in `/usr/share/doc/lumaaglaass` (from [`third_party/lumaaglaass`](../third_party/lumaaglaass)), and in `licenses/lumaaglaass` with Polyfin Desktop.
