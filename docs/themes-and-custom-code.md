# Themes and custom code

This page is for administrators who change the web player's look. It covers the themes, the theme each user gets, the bundled LumaaGlaass theme, other themes and copies of them, custom CSS and JavaScript, the sign-in message and Jellyfin's branding. The web player itself is on [Web player](web-client.md), and its home page on [Home rows](home-rows.md).

<a id="customizing-the-web-client"></a>

## Customizing the web player

**Settings › Web player** in the admin app holds the web player's settings. **Open the web player** shows the result.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Server theme** | **Settings › Web player** | LumaaGlaass | The web player's look for the users who keep the server's: **LumaaGlaass**, the theme Polyfin bundles, **jellyfin-web's own look**, or **Custom**, the two texts below. Each user can have their own (see [Theme per user](#theme-per-user)). |
| **Custom CSS** | **Settings › Web player** | Empty | CSS applied to every page of the web player for the users whose theme is **Custom**. Up to 2 MB. |
| **Custom JavaScript** | **Settings › Web player** | Empty | Script run on every page of the web player for the users whose theme is **Custom**. Up to 2 MB. |
| **Sign-in message** | **Settings › Web player** | Empty | Text shown under the sign-in form. Plain text, Markdown or HTML. Up to 8 KB. |
| **Add versions to an open title's page** | **Settings › Web player** | On | Adds the versions the addons find to a title's page while it is open. Off, a page opened before any version was found still lists the first ones, and the play buttons still wait for a version. The others show when the page is opened again. See [Title pages](versions.md#title-pages). |
| **Show ratings on posters** | **Settings › Web player** | Off | See [Ratings on posters](web-client.md#ratings-on-posters). |

The same page holds the [home rows](home-rows.md).

## Themes

**Server theme** is the theme of the users who keep the server's. Every user does until given their own (see [Theme per user](#theme-per-user)).

| Theme | What it is |
|---|---|
| **LumaaGlaass** | The default: the theme Polyfin bundles (see [LumaaGlaass](#lumaaglaass)). Polyfin serves its CSS and its script itself. The two fields show them without letting them be edited. |
| **jellyfin-web's own look** | No custom CSS or script. Each user then picks one of jellyfin-web's display themes, such as Light or Blue Radiance, in the web player's "Settings › Display › Theme". With LumaaGlaass, only Dark fits. |
| **Custom** | **Custom CSS** and **Custom JavaScript**: your own theme (see [Use another theme](#use-another-theme)). |

- **Custom texts kept:** choosing a theme keeps the **Custom** texts as they are. Choosing **Custom** again brings them back, and the users given the Custom theme keep it.
- **Edit as Custom** shows beside the choice while it is **LumaaGlaass**. It copies the theme's two texts into **Custom CSS** and **Custom JavaScript** and chooses **Custom**, so that you can change its options. It asks first when the **Custom** texts are not empty.
- The change applies when the page is saved.

After changing the server's theme or the **Custom** texts, reload the web player's open pages. Each load takes the theme and its texts as the server has them then (see [How the web player loads the theme](#how-the-web-player-loads-the-theme)).

**Compared with Jellyfin**

- Jellyfin has no theme choice: its custom CSS is empty by default, and it has no custom JavaScript.

<a id="each-user-s-theme"></a>
<a id="each-users-theme"></a>
<a id="web-player-theme"></a>

## Theme per user

**Web player theme** gives a user a theme of their own, on every browser they sign in with.

- **Administrators** set it in its own panel on the user's page under **Users**.
- **Users** set their own under **My account › Web player theme**, while allowed.

| Choice | What it does |
|---|---|
| **The server's** | The default. It names the server's theme. |
| **LumaaGlaass** | The bundled theme. |
| **jellyfin-web's own look** | No custom CSS or script. |
| **Custom** | Offered while the Custom theme has a text. |

A household can keep LumaaGlaass for most of its users. It can give jellyfin-web's own look, the lightest, to those who watch on a TV's browser or an older phone.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Let them change it** | The user's page under **Users**, beside **Web player theme** | On | Lets the user choose their own theme under **My account › Web player theme**. Off, the administrator's choice stays theirs. |

- **Locked:** while **Let them change it** is off, the user's **My account** shows the theme read only, with "Your administrator chose your web player theme.", and refuses a change.
- **Reset my preferences** puts the theme back to the server's, unless **Let them change it** is off (see [Your own preferences](users.md#your-own-preferences)).
- **Invite links:** an invite link's model user passes on their theme and **Let them change it**.
- **Old browsers:** an old browser gets jellyfin-web's own look, whatever the theme. LumaaGlaass's stylesheet and script need both:
  - CSS's `:has()`, from Chrome 105, Safari 15.4 and Firefox 121;
  - the syntax of ES2021, from Chrome 85, Safari 14 and Firefox 79.

  On an older browser the theme's script would not run, and its stylesheet would apply only in part.
- **Other apps:** the theme applies to the web player, in browsers and in the Jellyfin apps that show it. The other apps draw their own screens.
- **The Custom theme for some users only:** write the Custom texts while **Custom** is the server's theme, and save. Then choose the server's theme again: the texts stay.

**Compared with Jellyfin**

- Jellyfin has no theme per user: its custom CSS is the server's, for everyone. jellyfin-web keeps each user's display theme, such as Light, in the browser, not in the account.

API details: [Web player theme per user](jellyfin-api.md#web-player-theme-per-user-for-apps).

<a id="how-the-web-client-loads-the-theme"></a>

## How the web player loads the theme

The web player's page loads the theme itself as it opens, before jellyfin-web starts. The page thus shows it from its first paint, the sign-in page included. It loads:

1. the theme of the user signed in in this browser, as the browser kept it the last time they signed in there;
2. signed out, the theme of the user signed in last in this browser;
3. otherwise, as on a browser the user never signed in with, the server's theme.

Once a user is signed in, as they sign in or as a page opens signed in, the page asks the server for their theme. It keeps it in the browser for the next loads. When it differs from the one the page shows:

- a page that shows a theme with a script loads again, once, as a theme's script cannot be taken back. LumaaGlaass has a script, and so has a Custom theme with **Custom JavaScript**;
- any other page changes its theme at once. Its stylesheet goes after jellyfin-web's own CSS, as the next loads show it.

A theme changed by an administrator, or by the user on another device, thus shows in a browser once the web player is opened again there, or the user signs in there.

A browser that keeps nothing, as some private windows, shows the server's theme as each page opens. It shows the user's once they sign in, and only when the server's has no script.

The theme's stylesheet stays where jellyfin-web puts the server's custom CSS, after its own. **Disable server-provided custom CSS code**, in a user's display settings, still turns it off for them, in that browser. The theme's script still runs.

**Compared with Jellyfin**

- Jellyfin has one custom CSS for the whole server, which its branding gives everyone. jellyfin-web keeps each user's display theme, such as Light, in the browser, not in the account. It does not follow the user to another browser, on Jellyfin as on Polyfin ([userSettings.js, lines 452 to 458](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/scripts/settings/userSettings.js#L452-L458)).

API details: [Web player theme loading](jellyfin-api.md#web-player-theme-loading-for-apps).

<a id="default-theme"></a>

## LumaaGlaass

The theme **LumaaGlaass** is [LumaaGlaass](https://github.com/Dyhlio/LumaaGlaass), a theme for jellyfin-web: its stylesheet and its script. Polyfin bundles its files at commit `43dc3e0` of its repository (October 11, 2026, after its version 1.1.0), with three patches of Polyfin's. The theme is updated with Polyfin's releases.

- **Served by Polyfin:** the web player serves the theme's files itself, under `/web/polyfin/themes/lumaaglaass/`. No browser loads them from elsewhere, and the theme works on a server cut off from the internet.
- **Cached:** their address carries a version that changes with them. A browser keeps each file for a year, and gets the new ones with the Polyfin release that brings them.
- **Made, not stored:** Polyfin makes the CSS and script that load the files. A new copy of the theme changes them by itself.
- **Each user's choices:** each user chooses the theme's appearance and features on the LumaaGlaass page of the web player's settings menu. The choices are saved with their account, on every device.
- **jellyfin-web's Dark theme:** the theme needs it. It is the default.
- **Upgraded servers:** a server upgraded from a version without a theme chooses it from its two texts:
  - **LumaaGlaass** when they still loaded the bundled theme, which empties them;
  - **jellyfin-web's own look** when both were empty;
  - otherwise **Custom**, with the texts as they were, an edited LumaaGlaass included.

### Home carousel

The theme's home carousel is the large rotating artwork above the home rows. It is off by default: the home starts with its rows, over a still background.

- **Turning it on:** a user turns it on with "Home carousel" on the LumaaGlaass page, then "Save". An administrator can do it for another user from that user's settings.
- **The defaults:** the page starts from the options of the theme's script, `window.LumaaGlaassOptions`. `homeCarousel: false` there is the default for every user who never saved it. The options it leaves out keep the theme's own recommended values.
- **Changing them for the whole server:** use **Edit as Custom**, and change them in **Custom JavaScript**.
- **Going back to the defaults:** "Delete saved settings", and "Reset to defaults" or the "Recommended" profile followed by "Save", bring back these defaults, carousel off. The settings a user saved stay theirs.
- **Its picks:** the carousel picks up to 8 movies and series when the web player loads, a share from each library. It rotates through them until the page is loaded again.
  - Polyfin draws its picks among each library's first 100 titles, not its first catalog page.
  - It draws anew each time a page is loaded 2 minutes or more after the last draw (see [Titles at random](jellyfin-api.md#titles-at-random)).
  - Refreshing the picks without loading the page again is the theme's to do.

### Polyfin's patches

The patches are in [`third_party/lumaaglaass`](../third_party/lumaaglaass), with the theme's license:

- The LumaaGlaass page takes the options of the script's loader as its defaults. The theme's own page ignores them.
- The theme's script, pasted rather than loaded, takes its extensions from Polyfin. Otherwise it would take them from the main branch of the theme's repository.
- The theme's script goes over the page once per frame, rather than after each of jellyfin-web's changes to it, which made the web player slower to start and to change pages.

`scripts/lumaaglaass.sh` makes the bundled copy from a commit of the theme's repository and applies the patches again. Each update is thus a change to review.

<a id="follow-the-theme-s-repository"></a>
<a id="follow-the-themes-repository"></a>

## Follow the theme repository

To get LumaaGlaass's changes as soon as they are published, rather than with Polyfin's releases, choose **Custom**. Then load the theme from a CDN that serves its repository.

In **Custom CSS**:

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

- **Security:** the script then changes with the theme's repository, without review by Polyfin or by you. It runs in the browser of every user who opens the web player, with their account. To keep one version you have read, replace `main` with a commit or a tag of the repository, in both addresses.
- **Privacy:** the CDN sees the address of every browser that opens the web player.
- **Offline:** a browser that cannot reach the CDN shows the web player's own look.
- **Delays:** the CDN takes up to 12 hours to serve a change to the theme. A browser keeps the files it loaded for up to 7 days.
- **The carousel:** without Polyfin's patches, the LumaaGlaass page ignores `homeCarousel` in the options, so users who never saved it get the carousel on. `homeCarousel: false` hides it for every user only with `preferences: false`, which also removes the LumaaGlaass page.
- **Coming back:** to come back to the bundled copy, choose **LumaaGlaass**. The **Custom** texts stay for later.

## Use another theme

Any theme made for jellyfin-web can be the **Custom** theme: choose **Custom**, and put it in **Custom CSS** and **Custom JavaScript**. Both are server settings, the same with Polyfin Desktop and with Docker. Polyfin runs no Jellyfin plugins.

- **A theme made of CSS only:** put it in **Custom CSS**, and leave **Custom JavaScript** empty. Paste its stylesheet whole, up to 2 MB, or an `@import` of its address, such as `@import url('https://themes.example/theme.css');`.
- **A theme with a script:** put its stylesheet in **Custom CSS**, as above. In **Custom JavaScript**, paste its script whole, or a loader like LumaaGlaass's, which adds it to the page once:

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
- **A theme loaded from a CDN, kept in Polyfin:** **Keep a copy in Polyfin** saves the files the two texts load from elsewhere in Polyfin's data folder. Browsers then load them from Polyfin, and the theme works offline. See [Keeping a copy of a theme](#keeping-a-copy-of-a-theme).
- **Hosting a theme's files yourself:** serve them from a web server of your own, such as the reverse proxy in front of Polyfin, and use their addresses in both fields.
  - Keep the theme's folder as it is: a script often loads more files from beside it.
  - Serve stylesheets as `text/css` and scripts as `text/javascript`. Browsers refuse them as anything else.
  - Pasting a theme whole needs no server at all.
- **One user's own look:** each user can add CSS of their own in the web player's "Settings › Display", in "Custom CSS code". It applies for them only, in that browser, along with the theme's. An administrator can also give a user another theme than the server's (see [Theme per user](#theme-per-user)).
- **Security:** **Custom JavaScript** runs in the browser of every user whose theme is **Custom**, with their account. **Disable server-provided custom CSS code**, in a user's display settings, turns off the theme's custom CSS only: a theme's script keeps running. Load or paste only code you trust, and pin a theme loaded from its repository to a commit or a tag.

## Keeping a copy of a theme

**Keep a copy in Polyfin**, under **Custom JavaScript**, saves the files **Custom CSS** and **Custom JavaScript** load from elsewhere, such as a theme on a CDN. It rewrites both texts to load them from Polyfin. The theme then works on a server cut off from the internet, and browsers no longer reach its CDN.

- It is offered while the theme is **Custom**, once the texts load something from elsewhere.
- Save the texts first: the copy is made from the saved texts.

### What a copy saves

- the stylesheets **Custom CSS** imports (`@import`), and the fonts and images it names (`url()`);
- the http and https addresses **Custom JavaScript** names, in its code or in an HTML tag it writes (`src`, `href`), when they answer a stylesheet, a script, a font or an image;
- in what it saves, the stylesheets' own `@import` and `url()`, relative ones included, and the addresses scripts name, down to 4 levels below the texts.

Each address is asked once, from the internet only.

- A file kept is named after its address, numbered, in a folder of its own for the copy.
- The stylesheets saved name the other files saved beside them.
- The texts and the scripts saved name them at `polyfin/themes/saved/<id>/`, relative to the web player's address, as LumaaGlaass's are.

### What a copy cannot save

These are listed under the copy with why, and still loaded from elsewhere:

- an address a script builds when it runs, such as a folder it adds file names to (`'https://themes.example/extensions/' + name`, or a template with `${...}`). No copy can follow it. A theme whose script loads its parts this way keeps loading them from its CDN, or does not find them offline;
- a file that is not a stylesheet, script, font or image, or not the kind its place loads, such as a page where a stylesheet is imported;
- an address on a local network, or one redirected there;
- a file past the limits below, or an address that answered an error or nothing.

### Managing the copy

The admin app shows what the copy holds, its size and when it was saved. It lists each file with the address it came from, and each address it could not save with why. It never leaves one out.

- **Nothing saved:** a copy that saved no file is not made, and the texts are left as they were.
- **Update the copy** downloads the files again from the addresses they came from, as a new copy under a new folder, so that browsers take the new files. It deletes the former copy. It also saves addresses added to the texts since.
- **Remove the copy** puts the original addresses back in the texts, and deletes the files.
- **Edits:** editing the texts keeps the copy: the addresses of its files stay in them. **Update the copy** and **Remove the copy** still find their originals.
- Reload the web player's open pages after any of these (see [Themes](#themes)).

### Copy limits

The limits leave room above the largest themes made for jellyfin-web: an icon font of 5.3 MB, about 6 MB and 31 files for a whole theme, three levels of `@import`. They also keep a copy far from filling the data folder.

| Limit | Value |
|---|---|
| One file | 10 MB |
| The whole copy | 32 MB |
| Files | 100 |
| Levels followed below the texts | 4 |
| Time for one file | 30 seconds |
| Time for the whole copy | 2 minutes |
| Redirects of one file | 5 |

### Copy safety

- Only administrators can keep, update or remove a copy.
- Polyfin asks http and https addresses on the internet only, checked at each connection, as for a user's own addons.
  - An address, or a redirect, to a local network, the loopback, or the shared address space of carrier-grade NAT is refused.
  - A redirect to another scheme is refused.
  - No proxy is used.
- Polyfin keeps these kinds of files, by the type the server sends, or by the file's extension when it sends plain text or bytes:
  - stylesheets (`text/css`);
  - scripts (`text/javascript`);
  - fonts (WOFF2, WOFF, TrueType, OpenType, EOT);
  - images (PNG, JPEG, GIF, WebP, AVIF, SVG, ICO).
- The web player serves only the files of a copy's folder, by the names a copy gives, never a path out of it.
  - They are cached for a year, as their folder changes with each copy.
  - They go with the type of their extension, and with a policy that keeps a file opened on its own, such as an SVG image, from running a script.
- The script a copy saves runs as **Custom JavaScript** does: in the browser of every user, with their account. Keep a copy only of a theme you trust.

### Copy files and backups

- **Where the files are:** in `themes` in Polyfin's data folder.
  - In the Docker image's volume, `/data/themes`.
  - With Polyfin Desktop, `themes` in its data folder, such as `~/Library/Application Support/Polyfin/themes` (see [Folders](desktop.md#folders)).
- **Leftovers:** at start, Polyfin removes the folders of copies it no longer keeps, as a stop in the middle of a copy can leave.
- **Backups:** a database backup holds the texts, which name the copy's files, and the list of those files with their original addresses. It does not hold the files themselves. Restored without them, the copy misses its files: on another system or another data folder, or after the files were deleted.

### Missing copy files

- **Redirects to the original:** the web player sends a browser that asks for a missing file of the copy to the address it came from, with a `302` that is not kept. While the internet and the theme's CDN are reachable, the theme keeps working from its CDN, as it did before the copy.
- **Offline:** those files do not load, and the web player shows its own look where they were.
- **Only the copy's files** are sent there, each to its own original address, http or https. Any other name answers `404`.
- **A warning:** while the theme is **Custom** and its texts load the copy, **System › Health** shows a warning with how many of the copy's files are missing, linked to the copy (see [Health](administration.md#health)). The copy panel says so too.
- **Fixing it:** **Update the copy** downloads them again from their addresses, as a new copy, which clears the warning. So does choosing another theme. **Remove the copy** works without them too, and puts the original addresses back in the texts.

**Compared with Jellyfin**

- Jellyfin serves the custom CSS as it is written, and saves nothing of what it loads. Every browser loads a theme's files from wherever the CSS names them.

API details: [Theme copy](jellyfin-api.md#theme-copy-for-apps).

## Custom CSS

The theme's CSS, LumaaGlaass's or **Custom CSS**, is Jellyfin's branding CSS: each user gets their theme's.

- jellyfin-web applies it to every page, after its own CSS.
- A user who ticks **Disable server-provided custom CSS code** in their display settings does not get it.
- The web player's page loads it too, as it opens, so that it shows from the first paint (see [How the web player loads the theme](#how-the-web-player-loads-the-theme)).

**Compared with Jellyfin**

- Jellyfin's branding gives the server's CSS to everyone, signed in or not ([BrandingController.cs, lines 30 to 64](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/BrandingController.cs#L30-L64)). Polyfin's gives the CSS of the theme the web player shows in the browser that asks, which the web player's page names in a cookie. Without the cookie, it gives the CSS of the theme of the user whose token the request carries. A request without a token gets none, as the web player's page loads the theme of the browser itself.

API details: [Branding CSS](jellyfin-api.md#branding-css-for-apps).

## Custom JavaScript

When a user's theme has a script, Polyfin's script adds a script tag for it to jellyfin-web's page as it loads (see [How the web player loads the theme](#how-the-web-player-loads-the-theme)). LumaaGlaass has one, and so has a **Custom JavaScript** that is not empty.

- The script runs on every page of the web player, after jellyfin-web's scripts.
- It is never added to the admin app.
- It runs in the browser of every user whose theme it is, with their account.
- The display setting does not turn it off. Paste only code you trust.

**Compared with Jellyfin**

- Custom JavaScript has no Jellyfin equivalent.

API details: [Custom JavaScript](jellyfin-api.md#custom-javascript-for-apps).

## Sign-in message

**Sign-in message**, under **Settings › Web player**, shows under jellyfin-web's sign-in form. It is Jellyfin's `LoginDisclaimer`.

- It can be plain text, Markdown or HTML, which jellyfin-web sanitizes.
- jellyfin-web keeps it for up to a minute before asking again.

## Branding through the Jellyfin API

Administrators can also set the custom CSS and sign-in message through Jellyfin's branding configuration. jellyfin-web's own branding page uses it.

- **Saving:** it replaces the branding whole, as in Jellyfin: a text left out is cleared.
- **Reading:** anyone signed in can read it. The CSS read is the server's theme's, whatever the theme of the user who reads it.
- **Same CSS:** posting back the theme's CSS unchanged keeps the theme. That page does so when only the sign-in message changes.
- **Another CSS:** posting another CSS makes it **Custom CSS** and chooses **Custom**.
  - When the theme was LumaaGlaass, its script becomes **Custom JavaScript**, so that the page keeps it.
  - When it was jellyfin-web's own look, **Custom JavaScript** becomes empty.

**Compared with Jellyfin**

- Polyfin has no splash screen, so `SplashscreenEnabled` stays false whatever is posted.
- Jellyfin stores the CSS as posted, with no theme to keep ([`ConfigurationController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ConfigurationController.cs#L138) and [`BrandingController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/BrandingController.cs#L63), Jellyfin 12.4).
- An empty text is left out of the answer, where Jellyfin sends it as `""`.

API details: [Branding configuration](jellyfin-api.md#branding-configuration-for-apps).
