# Web client

Polyfin's Docker image includes jellyfin-web, Jellyfin's own web client. This page covers how Polyfin serves it, how it links to the admin app, how to customize it, and its license.

## The built-in web client

The Docker image includes jellyfin-web 12.4. Polyfin serves it at `/web/`, as Jellyfin does, and it connects to the server that served it.

- Once an administrator exists, `http://<server>:8096/` opens the web client.
- Before that, `/` and `/web/` lead to the setup page in the admin app, since no account could sign in yet. See [getting started](getting-started.md).
- `POLYFIN_WEB_DIR` can name another copy of jellyfin-web. Outside the image, without one, there is no web client and `/` leads to the admin app.
- The web client's scripts, styles and other text files go gzip-compressed to browsers that accept it. Each file is compressed once, on its first request, and kept in memory.
- On a title's page, the web client shows the title's IMDb rating, critics' score and audience score while the server has an **MDBList API key for ratings**, in place of the star and critic rating it shows otherwise. See [Ratings](addons-and-libraries.md#ratings).
- On the page of a Live TV programme that has not started, the web client shows **Remind me** after the record buttons, with a menu offering **Remind me of every airing**, and **Reminder set**, which removes it, for a programme with a reminder. When no target of the user receives reminders and their apps do not show them, a line under the button says so, with a link to **My account › Notifications**. See [Programme reminders](live-tv.md#programme-reminders).
- On the search page, while the field is empty, the web client lists the user's recent searches above the Suggestions. See [Recent searches](#recent-searches).
- While **Show ratings on posters** is on, a movie's or a series' poster shows its ratings as the pointer or the focus comes to it. See [Ratings on posters](#ratings-on-posters).

**Compared with Jellyfin:**

- Before an administrator exists, Jellyfin's web client opens its startup wizard instead.

### Recent searches

While the search field is empty, jellyfin-web's search page lists your last searches under "Recent searches", above the Suggestions and drawn as they are:

- up to 10, newest first;
- choosing one searches it again, in the library the search page was opened from, if any;
- the ✕ after a search removes it, and "Clear all" removes them all.

The [default theme](#lumaaglaass) hides the Suggestions, so the searches show right under the field.

A search is kept when you leave its results: you open a result or another page, clear the field, start another search, or close the tab. Only a search that found something is kept. Typing on, as from "the" to "the matrix", keeps the longer search only, and a search made again moves to the top, the same words in other capitals counting as one.

The list works with the keyboard as the rest of the page does: Tab, or the arrows in the TV layout, move to the searches and their buttons, and Enter chooses. After a search is chosen, the field has the focus; after one is removed, the next ✕ has it. Its words follow the web client's language, as the script's others (see [Dashboard links and single sign-on](#dashboard-links-and-single-sign-on)).

Recent searches stay in the browser, nothing goes to the server: each browser has its own, and other apps don't show them. They are kept for the server and the user signed in, so another user signing in on the same browser never sees them. **Sign out** keeps them for your next sign-in in that browser, as jellyfin-web keeps the settings it stores in the browser for each user. Clearing the site's data in the browser removes them.

**Compared with Jellyfin:**

- jellyfin-web on a Jellyfin server keeps no recent searches.

### Ratings on posters

While **Show ratings on posters**, under **Settings › Web player**, is on, the web client shows a movie's or a series' ratings over its poster as the pointer comes to its card, or the focus does, with the keyboard or a TV remote. They sit at the top, on the start side, under a count of versions when the card shows one, with an icon each and no words:

- a star, with the title's star rating, the one its page shows: the addon's, or the IMDb rating while the server has an **MDBList API key for ratings** (see [Ratings](addons-and-libraries.md#ratings));
- a review, with the critics' score, and a group of people, with the audience score, in percent, while the server has that key and MDBList gave them.

A rating the title lacks is left out, and a title without any shows nothing. Other cards, such as episodes, collections and people, and the list layouts, show nothing either. The ratings follow the same rules as the listings: a title the user may not see answers nothing. They show with every theme, **LumaaGlaass**, **None** or **Custom**, and in jellyfin-web's TV layout. It is off by default, for every user; users cannot change it.

The first card the pointer or the focus comes to asks the server once for the ratings of every movie and series card of the page, those of the card's row first, 100 at most. The cards answered then show theirs without asking again; the answers are kept for a minute, in the browser tab, for the user signed in. Polyfin reads them as a listing does: no addon and no MDBList request comes from them that the listing of those titles did not make. Turned off, the web client asks again a minute later at the earliest. Turning it on or off thus applies within a minute, without reloading the page.

**Compared with Jellyfin:**

- jellyfin-web shows no ratings on posters, on Jellyfin either: a card has its played mark, its count of unplayed episodes and, on hover, its play, played, favourite and ⋮ buttons ([cardBuilder.js, lines 997 to 1029](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/cardbuilder/cardBuilder.js#L997-L1029)). Polyfin adds them with its script, in the free corner.

**For app developers:**

- `GET /Polyfin/Items/Ratings?Ids=<id>,<id>…`, with the user's token, answers `{"Enabled": true, "Items": [{"Id", "CommunityRating", "CriticsScore", "AudienceScore"}]}`: for each movie or series named that the user may see and that has a rating, the scores from 0 to 10 and 0 to 100, `null` for those it lacks. It reads 100 identifiers at most. While **Show ratings on posters** is off, it answers `{"Enabled": false, "Items": []}` and reads nothing.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Show ratings on posters** | **Settings › Web player** | Off | Shows a movie's or a series' ratings over its poster in the web client as the pointer or the focus comes to it, for every user. |

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

When the server refuses to play a title, jellyfin-web 12.4 shows the reason, such as "This media cannot be played at this time due to rate limits." for a user playing on as many devices as allowed, then "There was an error processing the request" over it. The script closes that second, generic error as it opens, so only the reason shows. A real Jellyfin server gets the same two errors from jellyfin-web.

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
- Without a token it may use, the admin app shows its sign-in page.

The session lasts as long as the web client's sign-in it came from. It ends when:

- the web client signs out, with **Sign out**;
- its device is signed out, from **Devices** in **My account** or on the user's page, or for not being used (see [Signing out unused devices](users.md#signing-out-unused-devices));
- someone signs in to the web client in that browser again, the same user or another.

An admin app left open then shows, at its next request, the user the web client is signed in as now, or its sign-in page when the web client is signed out. The next person to use the browser never finds the admin app signed in as the one before.

Opened from the web client (its **Dashboard**, a dashboard page, the **Polyfin** entry, **Share** or a reminder's link to **Notifications**), the admin app shows the user signed in to the web client. A session of another user in the admin app, such as one opened with a password, is signed out first.

A session opened in the admin app with a password, and the code of two-step sign-in, does not depend on the web client: it lasts until **Sign out** in the admin app, or 30 days without use.

**Sign out** in the admin app leaves the web client signed in. The admin app does not take the web client's sign-in again until the web client signs in anew, or the admin app is opened again.

The upgrade that brought this ends every admin app session opened before, as nothing told those taken from the web client from those opened with a password: everyone signs in to the admin app once, which arriving from the web client does by itself, but for accounts with two-step sign-in.

**Compared with Jellyfin:**

- Jellyfin's dashboard is part of jellyfin-web and uses its sign-in ([routes.tsx, lines 18 to 20](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/apps/dashboard/routes/routes.tsx#L18-L20)): signing out of the web client signs out of the dashboard, and a user who is not an administrator is sent to the home page ([ConnectionRequired.tsx, lines 164 to 180](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/ConnectionRequired.tsx#L164-L180)). Polyfin's admin app has a session of its own, which follows the web client's in the same way when it came from it.
- Jellyfin ends a user's earlier sign-in on a device when that user signs in there again, but keeps another user's ([SessionManager.cs, lines 1784 to 1813](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Session/SessionManager.cs#L1784-L1813)). Polyfin does the same for Jellyfin apps; only the admin session taken from the earlier sign-in ends.

**For app developers:**

- The token goes to `POST /admin/api/session/jellyfin`, in the `Authorization` header only, never in the URL.
- The web client's links to the admin app carry `?from=web`, which the admin app takes out of the address as it opens.

## Caching and security headers

- The page itself is never cached.
- Files whose name or query carries a build hash are cached for a year.
- The other files are revalidated.
- The files of the [default theme](#lumaaglaass), at `/web/polyfin/themes/lumaaglaass/<version>/assets/`, are Polyfin's own: cached for a year at the version of the bundled files, `text/css` and `text/javascript`. Any other version, such as `latest`, is redirected to the current one, and revalidated.
- The files of a [copy of a theme](#keeping-a-copy-of-a-theme), at `/web/polyfin/themes/saved/<id>/`, are cached for a year, as `<id>` changes with each copy, with the type of their extension and a `Content-Security-Policy` that keeps them from running anything when opened on their own.
- `/web/` gets no Content-Security-Policy, as from Jellyfin: the admin app's would stop jellyfin-web from working. The other security headers stay.

**Compared with Jellyfin:**

- Jellyfin sends no caching header and leaves browsers to guess.

## Customizing the web client

**Settings › Web player** in the admin app holds the web client's **Theme**, two texts that are the **Custom** theme, **Custom CSS** and **Custom JavaScript**, the **Sign-in message**, two switches, **Add versions to an open title's page** and **Show ratings on posters** (see [Ratings on posters](#ratings-on-posters)), and the [home rows](#home-rows). By default, the theme is LumaaGlaass (see [Themes](#themes)), and **Sign-in message** is empty. **Open the web player** shows the result.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Theme** | **Settings › Web player** | LumaaGlaass | The web client's look: **LumaaGlaass**, the theme Polyfin bundles, **None**, jellyfin-web's own look, or **Custom**, the two texts below. |
| **Custom CSS** | **Settings › Web player** | Empty | CSS applied to every page of the web client while the theme is **Custom**. Up to 2 MB. |
| **Custom JavaScript** | **Settings › Web player** | Empty | Script run on every page of the web client while the theme is **Custom**. Up to 2 MB. |
| **Sign-in message** | **Settings › Web player** | Empty | Text shown under the sign-in form. Plain text, Markdown or HTML. Up to 8 KB. |
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

**For app developers:** `GET /Polyfin/HomeRows` answers the user's rows, in order, as `{"Items": [...]}`: each with its `Id`, its `Name`, its `Place` (`Top`, `AfterResume`, `AfterNextUp` or `Bottom`), its `Kind` (`Catalog`, `Collection`, `BecauseYouWatched` or `New`) and its first `Items`, as `BaseItemDto`, 20 unless `Limit` asks for up to 100. `Fields` asks for their fields, as listings do. A row's `Id` is a folder: `/Items?ParentId=` lists the whole row, up to 500 titles, and `/Items/{Id}` describes it.

### Themes

**Theme**, under **Settings › Web player**, is one of three:

- **LumaaGlaass**, the default: the theme Polyfin bundles (see [LumaaGlaass](#lumaaglaass)). Polyfin serves its CSS and its script itself, and the two fields show them without letting them be edited.
- **None**: jellyfin-web's own look, with no custom CSS or script. Each user then picks one of jellyfin-web's display themes, such as Light or Blue Radiance, under the web client's "Settings › Display › Theme". With LumaaGlaass, only Dark fits.
- **Custom**: **Custom CSS** and **Custom JavaScript**, your own theme (see [Use another theme](#use-another-theme)).

Choosing a theme keeps the **Custom** texts as they are, so choosing **Custom** again brings them back. **Edit as Custom**, beside the choice while it is **LumaaGlaass**, copies its two texts into **Custom CSS** and **Custom JavaScript** and chooses **Custom**, to change its options; it asks first when the **Custom** texts are not empty. The change applies when the page is saved.

jellyfin-web keeps the custom CSS it read for up to a minute, in the browser (its query cache), and the script changes at the next page load: after changing the theme, reload the web client's open pages a minute later, as the admin app says.

**Compared with Jellyfin:**

- Jellyfin has no theme choice: its custom CSS is empty by default, and it has no custom JavaScript.

#### LumaaGlaass

The theme **LumaaGlaass** is [LumaaGlaass](https://github.com/Dyhlio/LumaaGlaass), a theme for jellyfin-web: its stylesheet and its script. Polyfin bundles the theme: its files at commit `43dc3e0` of its repository (October 11, 2026, after its version 1.1.0), with two patches of Polyfin's. The theme is updated with Polyfin's releases.

- The web client serves the theme's files itself, under `/web/polyfin/themes/lumaaglaass/`: no browser loads them from elsewhere, and the theme works on a server cut off from the internet.
- Their address carries a version that changes with them, so a browser keeps each file for a year, and gets the new ones with the Polyfin release that brings them. The CSS and script that load them are made by Polyfin, not stored: a new copy of the theme changes them by itself.
- Each user chooses the theme's appearance and features on the LumaaGlaass page of the web client's settings menu. The choices are saved with their account, on every device.
- The theme's home carousel, the large rotating artwork above the home rows, is off by default: the home starts with its rows, over a still background. A user turns it on with "Home carousel" on the LumaaGlaass page, then "Save". An administrator can do it for another user from that user's settings.
  - The page starts from the options of the theme's script, `window.LumaaGlaassOptions`: `homeCarousel: false` there is its default for every user who never saved it. The options it leaves out keep the theme's own recommended values. To change them for the whole server, use **Edit as Custom** and change them in **Custom JavaScript**.
  - "Delete saved settings", and "Reset to defaults" or the "Recommended" profile followed by "Save", bring back these defaults, carousel off. The settings a user saved stay theirs.
  - The carousel picks up to 8 movies and series when the web player loads, a share from each library, and rotates through them until the page is loaded again. Polyfin draws its picks among each library's first 100 titles, not its first catalog page, and anew each time a page is loaded 2 minutes or more after the last draw (see [Titles at random](jellyfin-compatibility.md#titles-at-random)). Refreshing the picks without loading the page again is the theme's to do.
- The theme needs jellyfin-web's Dark theme, the default.
- Polyfin's patches, in [`third_party/lumaaglaass`](../third_party/lumaaglaass) with the theme's license:
  - the LumaaGlaass page takes the options of the script's loader as its defaults, where the theme's own page ignores them;
  - the theme's script, pasted rather than loaded, takes its extensions from Polyfin, where it would take them from the main branch of the theme's repository.
- `scripts/lumaaglaass.sh` makes the bundled copy from a commit of the theme's repository and applies the patches again, so that each update is a change to review.
- A server upgraded from a version without **Theme** chooses it from its two texts: **LumaaGlaass** when they still loaded the bundled theme, which empties them; **None** when both were empty; otherwise **Custom**, with the texts as they were, an edited LumaaGlaass included.

#### Follow the theme's repository

To get LumaaGlaass's changes as soon as they are published, rather than with Polyfin's releases, choose **Custom** and load the theme from a CDN that serves its repository, as Polyfin did until version 0.44. In **Custom CSS**:

```css
@import url('https://cdn.jsdelivr.net/gh/Dyhlio/LumaaGlaass@main/assets/lumaaglaass.css');
```

In **Custom JavaScript**, the bundled theme's loader with the address of the theme's script:

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
- To come back to the bundled copy, choose **LumaaGlaass**: the **Custom** texts stay for later.

### Use another theme

Any theme made for jellyfin-web can be the **Custom** theme: choose **Custom**, and put it in **Custom CSS** and **Custom JavaScript**. Both are server settings, the same with Polyfin Desktop and with Docker, and Polyfin runs no Jellyfin plugins.

- **A theme made of CSS only:** put the theme in **Custom CSS**, its stylesheet pasted whole, up to 2 MB, or an `@import` of its address, such as `@import url('https://themes.example/theme.css');`, and leave **Custom JavaScript** empty.
- **A theme with a script:** its stylesheet in **Custom CSS**, as above, and in **Custom JavaScript** its script pasted whole, or a loader like LumaaGlaass's, which adds it to the page once:

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

  Set the options the theme's documentation gives before the script loads, as LumaaGlaass's loader does with `window.LumaaGlaassOptions`.
- **A theme loaded from a CDN, kept in Polyfin:** **Keep a copy in Polyfin** saves the files the two texts load from elsewhere in Polyfin's data folder, so that browsers load them from Polyfin, and the theme works offline. See [Keeping a copy of a theme](#keeping-a-copy-of-a-theme).
- **Hosting a theme's files yourself:** you can also serve them from a web server of your own, such as the reverse proxy in front of Polyfin, and use their addresses in both fields. Keep the theme's folder as it is, as a script often loads more files from beside it. Serve stylesheets as `text/css` and scripts as `text/javascript`: browsers refuse them as anything else. Pasting a theme whole needs no server at all.
- **One user's own look:** each user can add CSS of their own under the web client's "Settings › Display", in "Custom CSS code". It applies for them only, along with the server's.
- **Security:** **Custom JavaScript** runs in the browser of every user who opens the web client, with their account. **Disable server-provided custom CSS code**, in a user's display settings, turns off the server's custom CSS only: a theme's script keeps running. Load or paste only code you trust, and pin a theme loaded from its repository to a commit or a tag.

### Keeping a copy of a theme

While the theme is **Custom**, **Keep a copy in Polyfin**, under **Custom JavaScript**, saves the files **Custom CSS** and **Custom JavaScript** load from elsewhere, such as a theme on a CDN, and rewrites both texts to load them from Polyfin. The theme then works on a server cut off from the internet, and browsers no longer reach its CDN. It is offered once the texts load something from elsewhere, and the texts must be saved first: the copy is made from the saved texts.

**What it saves:**

- the stylesheets **Custom CSS** imports (`@import`), and the fonts and images it names (`url()`);
- the http and https addresses **Custom JavaScript** names, in its code or in an HTML tag it writes (`src`, `href`), when they answer a stylesheet, a script, a font or an image;
- in what it saves, the stylesheets' own `@import` and `url()`, relative ones included, and the addresses scripts name, down to 4 levels below the texts.

Each address is asked once, from the internet only. A file kept is named after its address, numbered, in a folder of its own for the copy. The stylesheets saved name the other files saved beside them; the texts and the scripts saved name them at `polyfin/themes/saved/<id>/`, relative to the web client's address, as LumaaGlaass's are.

**What it cannot save:** these are listed under the copy with why, and still loaded from elsewhere:

- an address a script builds when it runs, such as a folder it adds file names to (`'https://themes.example/extensions/' + name`, or a template with `${...}`): no copy can follow it. A theme whose script loads its parts this way keeps loading them from its CDN, or does not find them offline;
- a file that is not a stylesheet, script, font or image, or not the kind its place loads, such as a page where a stylesheet is imported;
- an address on a local network, or one redirected there;
- a file past the limits below, or an address that answered an error or nothing.

**The admin app** shows what the copy holds, its size and when it was saved, each file with the address it came from, and each address it could not save with why; it never leaves one out. A copy that saved no file is not made, and the texts are left as they were.

- **Update the copy** downloads the files again from the addresses they came from, as a new copy under a new folder, so that browsers take the new files, and deletes the former one. It also saves addresses added to the texts since.
- **Remove the copy** puts the original addresses back in the texts, and deletes the files.
- Editing the texts keeps the copy: the addresses of its files stay in them, and **Update the copy** and **Remove the copy** still find their originals.
- Reload the web client's open pages a minute after any of these (see [Themes](#themes)).

**Limits:** they leave room above the largest themes made for jellyfin-web (an icon font of 5.3 MB, about 6 MB and 31 files for a whole theme, three levels of `@import`), and keep a copy far from filling the data folder.

| Limit | Value |
|---|---|
| One file | 10 MB |
| The whole copy | 32 MB |
| Files | 100 |
| Levels followed below the texts | 4 |
| Time for one file | 30 seconds |
| Time for the whole copy | 2 minutes |
| Redirects of one file | 5 |

**Safety:**

- Only administrators can keep, update or remove a copy.
- Polyfin asks http and https addresses on the internet only, checked at each connection, as for a user's own addons: an address, or a redirect, to a local network, the loopback, or the shared address space of carrier-grade NAT is refused, and so is a redirect to another scheme. No proxy is used.
- It keeps stylesheets (`text/css`), scripts (`text/javascript`), fonts (WOFF2, WOFF, TrueType, OpenType, EOT) and images (PNG, JPEG, GIF, WebP, AVIF, SVG, ICO), by the type the server sends, or by the file's extension when it sends plain text or bytes.
- The web client serves only the files of a copy's folder, by the names a copy gives, never a path out of it. They are cached for a year, as their folder changes with each copy, with the type of their extension, and with a policy that keeps a file opened on its own, such as an SVG image, from running a script.
- The script a copy saves runs as **Custom JavaScript** does: in the browser of every user, with their account. Keep a copy only of a theme you trust.

**Where the files are:** in `themes` in Polyfin's data folder: `/data/themes` in the Docker image's volume, and with Polyfin Desktop `themes` in its data folder, such as `~/Library/Application Support/Polyfin/themes` (see [Folders](desktop.md#folders)). Polyfin removes, at start, the folders of copies it no longer keeps, as a stop in the middle of a copy can leave.

**Backups:** a database backup holds the texts, which name the copy's files, and the list of those files with their original addresses, not the files themselves. Restored on a server without them, such as another system or another data folder, or after the files were deleted, the copy misses its files.

**Missing files:**

- The web client sends a browser that asks for a missing file of the copy to the address it came from, with a `302` that is not kept: while the internet and the theme's CDN are reachable, the theme keeps working, from its CDN, as it did before the copy. Offline, those files do not load, and the web client shows its own look where they were.
- Only the files of the copy kept are sent there, each to its own original address, http or https; any other name answers `404`.
- While the theme is **Custom** and its texts load the copy, **System › Health** shows a warning with how many of the copy's files are missing, linked to the copy (see [Health](administration.md#health)). The copy panel says so too.
- **Update the copy** downloads them again from their addresses, as a new copy, which clears the warning; so does choosing another theme. **Remove the copy** works without them too, and puts the original addresses back in the texts.

**Compared with Jellyfin:**

- Jellyfin serves the custom CSS as it is written and saves nothing of what it loads: every browser loads a theme's files from wherever the CSS names them.

**For app developers:**

- `GET /admin/api/settings/theme-copy` answers the copy, `copy`, null for none, with its `savedAt`, `size` in bytes, `files` (each with its `address`, the `text` that named it in the texts, its `name`, `kind` and `size`) and `unsaved` (each with its `address`, `reason` and a `detail` for some); `missing`, the names of its files not in its folder; `outside`, the addresses the texts load from elsewhere; and `limits`.
- `POST` keeps a copy, or makes it again, and `DELETE` removes it, both answering the same.
- Refusals: `409 theme_copy_running` while another copy is being made; `400 theme_copy_nothing` when the texts load nothing from elsewhere; `422 theme_copy_failed`, with `unsaved`, when no file could be saved; `409 custom_texts_changed` when the texts changed while the files were downloaded; `404 not_found` to remove no copy.
- The reasons are `private_address`, `not_http`, `unavailable` (with the HTTP status), `timeout`, `wrong_type` (with the type sent), `too_large`, `total_too_large`, `too_many_files`, `too_deep` and `built_at_run_time`.

### Custom CSS

The theme's CSS, LumaaGlaass's or **Custom CSS**, is Jellyfin's branding CSS. jellyfin-web applies it to every page, unless a user ticks **Disable server-provided custom CSS code** in their display settings. jellyfin-web keeps it for up to a minute before asking again.

**For app developers:**

- Served at `/Branding/Css` and `/Branding/Css.css` (`text/css`, empty when unset), and in `CustomCss` of `/Branding/Configuration`.

### Sign-in message

**Sign-in message** is Jellyfin's `LoginDisclaimer`, shown under jellyfin-web's sign-in form. It can be plain text, Markdown or HTML, which jellyfin-web sanitizes. jellyfin-web keeps it for up to a minute before asking again.

### Branding through the Jellyfin API

Administrators can also set the custom CSS and sign-in message through Jellyfin's branding configuration, which jellyfin-web's own branding page uses. Saving replaces the branding whole, as in Jellyfin: a text left out is cleared. Anyone signed in can read it. The CSS read is the theme's.

- Posting back the theme's CSS unchanged, as that page does when only the sign-in message changes, keeps the theme.
- Posting another CSS makes it **Custom CSS** and chooses **Custom**. When the theme was LumaaGlaass, its script becomes **Custom JavaScript**, so that the page keeps it; when it was None, **Custom JavaScript** becomes empty.

**Compared with Jellyfin:**

- Polyfin has no splash screen, so `SplashscreenEnabled` stays false whatever is posted.
- Jellyfin stores the CSS as posted, with no theme to keep ([`ConfigurationController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ConfigurationController.cs#L138) and [`BrandingController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/BrandingController.cs#L63), Jellyfin 12.4).
- An empty text is left out of the answer, where Jellyfin sends it as `""`.

**For app developers:**

- Set with `POST /System/Configuration/branding`; read with `GET /System/Configuration/branding`.

### Custom JavaScript

When the theme has a script, LumaaGlaass's or a **Custom JavaScript** that is not empty, Polyfin adds a script tag to jellyfin-web's `index.html`, after its own script. The script runs on every page of the web client, after jellyfin-web's scripts. It is never added to the admin app.

The script runs in the browser of every user who opens the web client, with their account. The display setting does not turn it off. Paste only code you trust.

**Compared with Jellyfin:**

- Custom JavaScript has no Jellyfin equivalent.

**For app developers:**

- The tag is `<script src="custom.js?v=<hash>" defer></script>`.
- `/web/custom.js` is served as `application/javascript`. It is cached for a year at the address with the hash of its content, which changes with it, and revalidated at any other address.
- With no script, there is no tag and no `/web/custom.js`.

## License and source code

jellyfin-web is free software by the Jellyfin contributors, and a separate program from Polyfin. Polyfin stays under the [MIT License](../LICENSE).

- The image contains jellyfin-web 12.4, unmodified, under the GNU General Public License, version 2.
- It is copied from the official `jellyfin/jellyfin:12.4` image into `/usr/share/polyfin/jellyfin-web`.
- Its license and a notice are in `/usr/share/doc/jellyfin-web` (from [`third_party/jellyfin-web`](../third_party/jellyfin-web)).
- Its complete source code is attached to every Polyfin release on GitHub as `jellyfin-web-12.4-source.tar.gz`, and is also at <https://github.com/jellyfin/jellyfin-web/tree/v12.4>.
- The Dockerfile pins both, side by side, to be changed together when the client is updated.

The default theme, LumaaGlaass by Om Gupta and Dyhlio, is under the MIT License. Polyfin embeds its files, with two patches, and serves them to the web client (see [LumaaGlaass](#lumaaglaass)). Its license and a notice naming the commit are in `/usr/share/doc/lumaaglaass` (from [`third_party/lumaaglaass`](../third_party/lumaaglaass)), and in `licenses/lumaaglaass` with Polyfin Desktop.

A copy of another theme's files, kept with **Keep a copy in Polyfin**, stays under that theme's own license: Polyfin saves it for your server only, and does not distribute it.
