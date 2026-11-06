# Invite and share links

Polyfin makes two kinds of link. An invite link lets someone create their own account. A share link lets someone without an account watch one movie or episode for a while.

## Invite links

An invite link lets people create their own accounts, so nobody has to choose and send them a password. **Create an invite link**, at the top of **Users**, asks for:

| Setting | Choices | Default | What it does |
|---|---|---|---|
| **Accounts it may create** | 1 to 100 | 1 | How many accounts the link creates: one link can serve a whole family. |
| **Expires** | 1, 7, 30 or 90 days, or **Never** | 7 days | When the link stops working. |
| **Accounts end** | **Never**, or 1, 7, 30 or 90 days after each is created | **Never** | Gives each account that end date (see [Ending an account](users.md#ending-an-account)). |
| **Settings of** | a user, or a new user | | Whose settings the new accounts copy, as with **Create a user**. |
| **Group** | **No group**, or a group | **No group** | A group the new accounts join and follow (see [User groups](users.md#user-groups)). |

A new account copies everything its model's page sets except the name, password, **Administrator**, **Ends on** and **Group**. That is the **Access** permissions, the **Playback and access** limits with the **Quality group**, **Parental control**, **Visible libraries**, **Blocked genres**, **Allowed hours**, and showing on the sign-in screen.

- The model is read as each account is created, so a change reaches later accounts only.
- Without a model, the account has the settings of **Create a user**, hidden from the sign-in screen.
- With a group, the account then takes and follows the group's values: a change reaches existing accounts too.
- An invite link never makes an administrator.

The link is shown once, with **Copy link**: Polyfin keeps only its hash and cannot show it again. It starts with the **Public address** (**Settings › Notifications**) when set, else the admin app's current address.

**Invite links**, under the accounts, lists each link with:

- accounts created out of those allowed, and whose settings it gives;
- when it expires, when its accounts end, and who created it;
- its state: **Active**, **Used up**, **Expired** or **Revoked**.

**Revoke** stops an active link at once and keeps its accounts. Deleting the user a link copies revokes it too.

<a id="the-guest-s-page"></a><a id="the-guests-page"></a>

### The guest page

The link opens `/admin/invite/…`, an admin app page needing no account, in the admin app language the guest picks.

- It shows the server's name and asks for **Your name**, a **Password** and **Confirm password**, under the rules of **Create a user**.
- The new account lands on the [web player](web-client.md), signed in. Without the web player, it lands signed in on the admin app, where a member sees their own pages.
- A used up, expired or revoked link says so, as does an address that is no link.

The activity log records each account, as "sam joined through alex's invite", then "sam joined the group Kids" when the link names a group. Notification targets that chose **User joined** are told (see [Notifications](notifications.md#events)).

### How links are kept safe

- A token is the invite's ID plus 32 random bytes, as many as an admin session's. Polyfin stores only the random part's SHA-256 hash, compared in constant time.
- An unknown token counts as a wrong password toward the client's failed sign-ins. After too many, the client is refused for a while (429), as on the sign-in page.
- Uses are counted, with the link locked, in the transaction creating the account. Two guests on the last use create one account; the other is told the link is used up.
- A refused name or password uses nothing up.
- The guest sends only a name and password; anything else is ignored, as settings come from the link.

**Compared with Jellyfin**

- Jellyfin has no invite links: administrators create every account and choose its password.

API details: [Invite links](jellyfin-api.md#invite-links-for-apps).

## Share links

A share link lets a member give one movie or episode to someone without an account, for a while. The guest sees that title alone.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Allow share links** | **Settings › Security** | Off | Allows links on the server. Off, none can be made, and all are refused with 410 but kept: on again, they work until they expire. |
| **Can share titles** | the **Access** section of a user's page | Off | Lets that user, administrators included, make links. An invite link's model passes it on, as other **Access** permissions. |

A user loses their links at once on losing **Can share titles**, being disabled or ending (see [Ending an account](users.md#ending-an-account)): they are revoked. Deleting the user deletes them.

### Making a link

Under **My account › Shared links**, **Share a title** searches the libraries the user sees.

- It finds what the search catalogs of the user's sources find (see [Catalog pages](addons-and-libraries.md#catalog-pages)), then the titles Polyfin already knows from their catalogs, whose name holds the words typed.
- Those known titles are the ones a library, a home row or a search listed before, for any user. So a catalog without a search, such as an addon's catalog that takes no `search` property, is found too. Polyfin asks no source for them.
- A title of such a catalog that no one listed yet is not found until its page is opened, and neither is one found by its name in another language only.
- A movie is shared as is; a series opens its episodes, and one episode is shared.
- A series, channel or recording is never shared.
- Parental control, blocked genres and hidden titles apply: a title the user may not see cannot be shared. So does **Used by**: a title known only from a source the user does not use is not found.
- In the web player, **Share** on a movie or episode page opens the same form with the title chosen (see [Web player](web-client.md)).

| Field | Choices | Default |
|---|---|---|
| **Expires after** | 1 hour, 1 day, 7 days or 30 days | 1 day |
| **Viewers at once** | 1 to 5 guests at the same time | 1 |

**Create link** shows the link once, with **Copy link**; Polyfin keeps only its hash. Like an invite link, it starts with the **Public address** (**Settings › Notifications**) when set, else the admin app's address.

**My account › Shared links** lists each link's title, expiry, play count, state (**Active**, **Expired** or **Revoked**) and **Revoke**. Administrators see and revoke a user's links in the **Shared links** section of the user's page. Links ended over 30 days ago are deleted.

### What the guest sees

The link opens `/share/…`, a page needing no account. Its language is the first Polyfin speaks among the browser's preferred ones, else the server's.

- It shows the server's name, the title, its artwork and overview, the link's expiry, and a player.
- The captions menu offers the played version's text subtitles.
- Name, overview and artwork are as the sharer sees them, in their [metadata language](watch-state.md#metadata-language).
- There is no library, search or other title.
- An expired, revoked, turned off or unknown link says so.

The page plays HTTP Live Streaming natively where it can, else with [hls.js](https://github.com/video-dev/hls.js) 1.6.16, served by Polyfin. Its words are in [Figtree](https://github.com/erikdkennedy/figtree), the admin app's font, served by Polyfin too: nothing loads from elsewhere.

Figtree is under the SIL Open Font License 1.1. Its license and a notice are in `/usr/share/doc/figtree` in the image (from [`third_party/figtree`](../third_party/figtree)), and in `licenses/figtree` with Polyfin Desktop.

The page remembers where the guest stopped, in their browser only:

- It saves the position every 10 seconds of play, on pause and on close, under the link's ID, never its token.
- Reopened in that browser, it offers "Resume at 0:42:10" and "From the beginning". Resuming starts there, and Polyfin reads the source ahead from there.
- Polyfin ends a playback after 2 minutes without word from the page (a device asleep, a background tab). The page says "This playback ended. Press Play to continue where you stopped.", and Play resumes.
- The position is forgotten in the last 5 % or 2 minutes, whichever is longer, after 31 days, or once the link answers expired or revoked.
- The server keeps nothing: the sharer's resume points and Continue Watching are untouched, and plays count in statistics as before.
- The position stays in that browser: not on other devices, and lost in a private window or by clearing site data.

### Limits

The guest plays as the sharer would, with less:

- Everything goes through Polyfin, as stream addresses can hold keys. The page gets HLS playlists and segments only: no download.
- The sharer's version rules choose the version (see [Playback](playback.md)).
- Video is at most 1080p, and never above the sharer's [quality group](user-limits.md#quality-groups) or **Maximum quality**. A taller version is converted down, only if the sharer may have video converted.
- A guest counts toward the sharer's **Playbacks at once**, and **Video conversions at once** when converted (see [Playback](transcoding.md#video-conversions-at-once)). At the limit, the page says the link's owner is watching on as many devices as allowed.
- A link takes at most its **Viewers at once** guests.
- The sharer's **Allowed hours** apply: outside them, nothing plays.
- A guest changes none of the sharer's played marks, resume points or Next Up.
- Plays count in the sharer's statistics, marked **Shared**. Home in the admin app shows them under the sharer, with "Shared link" as the app.

The activity log records each link made, revoked or expired, and each guest playback, as "A guest played Example Movie through the link shared by alex" (see [Administration](administration.md#activity-log)). **Shared link played** tells the sharer's and the server's notification targets, at most hourly per link (see [Notifications](notifications.md#events)).

### How share links are kept safe

- Tokens work as invite tokens: ID plus 32 random bytes, only their SHA-256 hash kept, compared in constant time (see [How links are kept safe](#how-links-are-kept-safe)).
- An unknown token counts as a failed sign-in of the client; after too many, it is refused for a while (429).
- The token opens the page's routes only: the Jellyfin API, admin API and web player refuse it as unknown.
- The guest's play session is signed for no one, so the API's own HLS routes refuse it too.
- The server fixes what is converted and sent at playback start; guest requests cannot change it.

**Compared with Jellyfin**

- Jellyfin has no share links: watching needs an account.

API details: [Share links](jellyfin-api.md#share-links-for-apps).
