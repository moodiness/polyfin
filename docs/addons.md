# Addons

This page is for administrators, and for users who add their own addons. Polyfin gets its content from Stremio addons and Eclipse music addons. It covers adding addons, who uses each source, which streams play, how addons are asked, what to do when one fails, how Polyfin keeps to addons' request limits, and the order and languages of sources.

Each addon has a role:

- **Metadata addons:** they give catalogs and metadata.
- **Stream addons:** they give streams and subtitles.

Any addon that speaks the standard Stremio protocol works. Music addons are on [Music, audiobooks and podcasts](music.md). What catalogs become is on [Libraries](addons-and-libraries.md).

## Server addons

A server addon is shared with every user. To add one:

1. Open **Content › Sources**, then **Add a source**.
2. Keep the **Stremio addon** tab.
3. Paste its **Manifest address**, copied from the addon's configure page.
4. In **Content › Libraries**, choose which of its catalogs become libraries in Jellyfin apps.

Polyfin turns some catalogs on for you:

- **Collection catalogs** first, when the addon has some.
- **Otherwise, movie, series and TV catalogs.** Catalogs of other types stay off, such as a cloud store's `other` catalog or a sports addon's `sport` catalogs. Turn them on in **Content › Libraries** (see [What catalogs list](addons-and-libraries.md#what-catalogs-list)).
- **Up to 20 libraries in all**, counting every addon, so that home screens do not fill up. A user's own addons count the user's own libraries under **My sources** the same way.

These are a starting point: you can turn on any number of catalogs. The message that confirms the addon was added says when it got no library:

- the server already has 20 libraries;
- or none of its catalogs was turned on, as for an addon whose catalogs are all of other types.

The message then says that its catalogs can be turned on in **Content › Libraries**. For a user's own addon, it says **My sources**.

Only administrators can add an addon hosted on a local network address.

API details: [Addons added to a scope](jellyfin-api.md#addons-added-to-a-scope-for-apps).

## Personal addons

Each user can add their own addons and libraries under **My sources**. A user can also turn the server's sources off for themselves, with **Use the server's sources**.

Each user plays only versions from their own sources:

- their addons under **My sources**;
- the server's addons, while **Use the server's sources** is on.

Polyfin refuses any other version, even by its identifier or from a saved link. When several sources list the same file, the user gets the link of the first of their own sources that lists it. That link keeps only the mirrors their sources list (see [Which streams become versions](versions.md#which-streams-become-versions)).

- Share-link guests play as the user who shared the title.
- DLNA devices play as the user whose libraries they show.

**Compared with Jellyfin**

- An item outside a user's access answers as one that does not exist, as in Jellyfin ([`MediaInfoController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/MediaInfoController.cs#L171-L175), Jellyfin 12.4).

API details: [Versions a user may not play](jellyfin-api.md#versions-a-user-may-not-play-for-apps).

## Who uses a server addon

Every server source has a **Used by** setting: a Stremio or Eclipse addon, an IPTV source, a tuner or a local folder. Set it on the source's page under **Content › Sources**, or in **Add a source**.

| Choice | Who gets the source |
|---|---|
| **Everyone** (default) | Every user, including users created later. |
| **Only these groups and users** | The users ticked, and the members of the groups ticked. |

- Sources made before this setting existed are on **Everyone**, so nothing changes until an administrator limits one.
- A group member who keeps **Own settings** still belongs to the group, and gets its sources.
- Administrators are users like others here. An administrator left out still manages the source under **Content › Sources**. The source's page warns that their own apps will not use it.

### What a user without the source misses

For a user who does not use a source, it is gone from everything their apps read:

- **Lists:** its libraries, the home rows of its catalogs, search results, smart list titles, new arrivals and Live TV channels.
- **Titles only it knows:** they answer as titles that do not exist. This holds when opened by identifier, from Continue Watching, favorites, a playlist, a people page or a saved link.
- **Titles another source knows too:** they stay, with the versions the user's own sources list. An example is a movie known by its IMDb identifier.
- **Its data:** its descriptions, translations and subtitles.
- **Its versions:** the versions it lists, and the mirrors it adds to another source's version. A version's identifier, or a saved URL, opens nothing.

Its versions are refused on every path:

- playback, the stream, HLS, downloads and converted downloads;
- subtitles and attachments;
- the Watchlist's availability checks;
- share links, which play as the user who shared them;
- DLNA devices, which play as the user they show;
- Live TV.

What Polyfin keeps of a listing stays each user's own:

- A version remembered from another user's listing keeps the mirrors that user's sources found, for the users who have them.
- A library listed in random order is drawn again once who uses its source changes.

Background work uses every server source, such as reading catalogs ahead or warming posters. So do requests made with an API key.

### Rules of Used by

- **New users:** a new user in no group gets none of the sources limited to others.
- **Deleting:** deleting a group or a user removes them from every **Used by** list.
- **Empty lists:** a source whose list empties is used by no one, never by everyone.
- **Groups:** the setting belongs to the source, not to the group's sections. It is not part of **Reset to defaults**. A member who follows their group, or not, uses the same sources.
- **Own addons:** a user who may add their own addons can still add the same addon under **My sources**, at a public address. To prevent it, turn **Can add their own addons** off for them or their group (see [Users' own addons](sign-in-and-security.md#users-own-addons)).
- **Own sources:** a user's own sources have no **Used by**. They belong to their owner alone.
- **Local addresses:** a limited source stays a server source, so it may keep a local network address.

The activity log records each change, such as "alex limited the addon Stream Two to Family, sam" or "alex made the addon Stream Two used by everyone".

**Compared with Jellyfin**

- Polyfin limits who uses each addon, and Jellyfin has no addons. Jellyfin's closest settings are a user's "Library access" (`EnabledFolders`, which Polyfin maps to **Visible libraries**) and "Channel access" for channel plugins (`EnableAllChannels`, `EnabledChannels`). Jellyfin checks them in [Channel.cs, lines 26 to 45](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Channels/Channel.cs#L26-L45), with the fields of [UserPolicy.cs, lines 164 to 182](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Users/UserPolicy.cs#L164-L182).
- Polyfin does not map addons to Jellyfin channels. **Used by** lives on the source, so a policy jellyfin-web posts cannot change it.
- jellyfin-web's "Library access" still lists the libraries of a source a user does not use, as the server's libraries. Ticking them grants nothing.

API details: [Used by in the admin API](jellyfin-api.md#used-by-in-the-admin-api).

## Which streams play

| An addon lists | It plays |
|---|---|
| A link to a video, over `http` or `https` | Always |
| A torrent: an `infoHash`, with its `fileIdx` and `sources` | Only while **Play torrent streams** is on in **Settings › Torrents**, for users with **Can play torrent streams** (see [Torrents](torrents.md)) |
| A web page (`externalUrl`) | Never |
| A YouTube video (`ytId`) | Never |

A torrent that cannot play is left out of the versions. So a title whose addons list only torrents has no version then.

Every rule from an addon's streams to a title's versions, in the order Polyfin applies them, is in [Which streams become versions](versions.md#which-streams-become-versions).

## Identifiers addons accept

A stream addon's manifest says which identifiers it lists streams for (`idPrefixes`, for each resource and type):

| Prefix | Database |
|---|---|
| `tt` | IMDb |
| `tmdb:` | TMDB |
| `kitsu:`, `mal:`, `anidb:` | An anime database: Kitsu, MyAnimeList or AniDB |

Polyfin asks for a movie or an episode by its own identifier when the addon takes it. Otherwise, it asks by the first of its other identifiers that the addon takes, in this order:

1. its IMDb identifier;
2. its TMDB identifier;
3. its anime's Kitsu, MyAnimeList, then AniDB identifier, from the [anime mapping](tracking.md#anime) that tracking uses.

Asked by another identifier, a title's type is `movie` or `series`. This holds even when its catalog types it otherwise, as some anime catalogs do (`anime`). Such a title is also asked by its own identifier with that type, when the addon takes only that type.

### How episodes are asked for

Polyfin names an episode as Stremio addons do:

| By | The episode is | Example, season 2, episode 3 |
|---|---|---|
| IMDb or TMDB | The series' identifier, with season and number | `tt…:2:3`, `tmdb:…:2:3` |
| An anime database | The entry's identifier, with the number the anime gives it | `kitsu:<id>:3` |

Anime databases make each season, or part of a season, an entry of its own. Its episodes are numbered from 1. So `kitsu:<id>:3` uses the id of the entry for that season.

### Where the other identifiers come from

**A title named by IMDb or TMDB:**

- It has the identifiers its description gives: `imdb_id`, and TMDB's when the addon gives it. Episodes keep the same season and number.
- A movie's anime entry is the one the mapping gives for its IMDb or TMDB identifier.
- An episode's anime entry is the one that holds its TVDB season and number, as for [AniList and MyAnimeList](tracking.md#finding-the-entry).
- The series' TVDB identifier comes from its description. Otherwise it comes from its IMDb identifier's entries, when they agree.

**Titles an anime catalog names:** `kitsu:<id>`, `mal:<id>` or `anidb:<id>`.

- It has the other anime databases' identifiers of the same entry, with the same episode number.
- Through the mapping, a movie also has its IMDb and TMDB identifiers.
- An episode also has its series' IMDb identifier, with its season and episode on TVDB.
- An episode is never asked by TMDB, whose seasons may be split otherwise.
- An episode is not asked by IMDb when the anime is numbered as TVDB's absolute numbering.

### Titles not asked by a mapped identifier

- **Ambiguous entries:** an identifier from the mapping is used only when one entry alone matches. A movie two entries claim is not asked by any anime identifier. Nor is a series whose entries point to different TVDB series.
- **Specials and unplaced episodes:** they are asked only by the identifiers their description gives.
- **Before the mapping loads:** until its lists are read, titles are asked only by their own identifiers and those of their description.
- **Other sources:** only Stremio addons are asked by other identifiers. IPTV sources and local folders list their own titles. Home videos and other videos belong to their folder alone.

### What Polyfin keeps of the answers

- Each addon's streams are kept under what it was asked. Its versions are renewed by asking the same again.
- **Refresh** forgets them, with the title's other lists.
- The streams of every addon merge into the title's versions. The same file listed twice is one version (see [Which streams become versions](versions.md#which-streams-become-versions)).
- At the `Debug` log level, each request for streams logs `An addon is asked for streams`. The line names the addon, the type and the identifier asked.

## Manifest addresses

Manifest addresses usually hold your addon settings or keys. Polyfin never shows them in full. To change one, use **Replace the address** in the source's more-actions menu.

Each source has its own page, from **Open its page** on its row. The page shows its details, an **Active** switch and its place in the order. **Remove** asks first.

## Addons that fail

### How Polyfin asks an addon

Polyfin asks an addon at the address it was added with, under the header `User-Agent: Polyfin/<version>`. An Eclipse addon also gets its settings in the query (see [Music addon settings](music.md#music-addon-settings)).

An address may have a `?` part after `manifest.json`, where some addons keep your token: `https://addon.example/manifest.json?token=…`. Polyfin asks for each resource in place of `manifest.json`, and keeps that part:

| Addon | Request |
|---|---|
| Stremio | The part comes after the resource, as Stremio sends it: `https://addon.example/stream/movie/tt1.json?token=…` |
| Eclipse | The part joins its settings and the request's own parameters: `https://addon.example/search?token=…&q=…` |

For an Eclipse addon, a setting replaces a parameter of the address with the same name. The request's own parameters (`q`, `skip`) replace both.

### What the log and Health say

An addon that answers with an error was reached. The log and **System › Health** tell what it answered:

| The addon | Health says | The log's error |
|---|---|---|
| Answers `401`, `403` or another error but `404` and `429` | refused the request | `addon refused the request: HTTP 403`, with the addon's reason when it gives one: `addon refused the request: HTTP 403 (Unavailable)` |
| Answers `429`, or `503` with `Retry-After` | asked to slow down (too many requests) | `addon asked to slow down: HTTP 429`, or `addon asked to wait: HTTP 503, retry after 30s` |
| Answers `500` or another server error | answered with a server error | `addon failed: HTTP 502` |
| Does not answer | could not be reached, or did not answer in time | `addon unreachable: ...` |

The reason of a refusal is the `error` or `message` of the addon's JSON answer. A short plain-text answer is the reason too. Polyfin puts it on one line and cuts it at 120 characters. It replaces with `<redacted>` anything that looks like an address, a token or a key.

Other places that tell of a failure:

- **Libraries:** a library whose addon fails shows nothing in the apps. The log tells each such request as `Browsing failed`, with the addon's name, the catalog and the error: `addon=Songs catalog=top error="addon refused the request: HTTP 403 (Unavailable)"`.
- **Music search:** a music addon whose search fails logs `A music addon could not search`, with its name. The apps get the other addons' results.
- **The Check button:** **Check**, in **System › Health**, asks for the manifest only. An addon that answers its manifest but refuses its catalogs or searches shows as answering, until the apps ask it for them again.

### When an addon refuses Polyfin

A refusal comes from the addon, or from the site behind it. Polyfin cannot change it. Common causes:

- the token in the addon's address has expired;
- the addon refuses requests from servers;
- the addon refuses apps other than its own.

Check that the same address works in the addon's own app, on the same network as Polyfin. Or ask the addon's author.

Some addons serve only their own app. That app adds to each request an access key, which its account server gives signed-in users only. Such an addon answers its manifest, and refuses everything else, usually with `403`. Polyfin cannot use it, and does not pose as another app.

### Testing an addon outside Polyfin

Ask the addon for a search or a catalog from the Polyfin machine, as Polyfin asks. Put the addon's address, up to `/manifest.json`, in place of `<address>`:

```
curl -i -A "Polyfin/<version>" "<address>/search?q=test"
curl -i -A "Polyfin/<version>" "<address>/catalog/movie/top.json"
```

- The first line is for an Eclipse addon.
- The second is for a Stremio addon, with a type and a catalog its manifest lists.
- For an address with a `?` part, put that part after the resource. For an Eclipse addon, join it with `&` to the addon's own parameters.

A `200` with results means Polyfin gets them too. A `401` or `403` shows the addon's reason in its body.

## Addon request limits

Public addons limit how many requests one server may send them. Polyfin keeps to their limits when it reads catalogs, and paces the requests it sends on its own.

### Addons that ask to wait

Public addons limit how often one server may ask them. Polyfin keeps to what they ask:

| Rule | What Polyfin does |
|---|---|
| At most 8 pages at once | Asks one addon's server for at most 8 catalog pages at a time, over all listings, refreshes and users. The others wait their turn. |
| The wait it asks for | After "too many requests" (HTTP 429), or 503 with `Retry-After`, asks for no catalog page until the wait (`Retry-After`) ends, 15 minutes at most. |
| No `Retry-After` | Waits 5 seconds, then twice as long each time it refuses again after a wait. |
| Failed refreshes | A page whose refresh failed, for any reason, is not read again for a minute. The page kept shows meanwhile. |

- **Shared limit:** users' own copies of an addon, and the addons one server serves, share its 8.
- **Searches** are not counted. A search asks each search catalog once a term, and searches are bounded where they are made.
- **Requests under way:** those sent before it refused still get their answers. The others are not sent.

While Polyfin waits:

- The pages kept still show, and none is read again.
- A page never read, such as another genre or a page further down, cannot be listed. A listing shows the titles before it as the whole listing, and its total counts them only.
- A listing that would start past them fails as before, and so does a library whose first page was never read. Apps show their error.
- Once the wait ends, the next visit reads the rest.
- Searches are still asked, one request a term. [Local folders](unmatched-files.md#failed-searches) wait as the addon asks.
- **Watchlist checks:** they ask no stream of an addon whose server waits. A check the addon answers "too many requests" starts its server's wait too (see [Now available and Still waiting](tracking-imports.md#now-available-and-still-waiting)). Title pages and playback ask for streams as before.

What the log and Health say:

- The log says once, at Info, that the addon asked to wait. It names the catalog, the page (`skip`, `genre`) and the wait: `An addon asked to wait: no catalog page is asked of its server until the wait ends`.
- With **Detailed log (to diagnose a problem)**, each refresh that failed otherwise says which page.
- **System › Health › Addons** shows the addon as "asked to slow down (too many requests)" until the wait ends, even when it answers other requests meanwhile.

The same is true of a page that cannot be read for another reason, such as a server error: a listing shows the titles before it. A longer **Refresh catalogs after (minutes)** has an addon asked less often.

**Compared with Jellyfin**

- A Polyfin listing may lack titles because a remote source refused them. Jellyfin lists its own database, so its listings never do. Polyfin's count then covers only the titles it could read, so that apps page through what is there rather than ask for titles it cannot give.

### How Polyfin paces addons

Besides what users open, Polyfin asks addons for streams and descriptions on its own:

- the titles of Continue Watching and Next Up, listed ahead;
- the next episodes, prepared or preloaded;
- the Watchlist checks;
- descriptions read again, or in a user's language;
- links renewed for scrubbing images;
- the second and third requests that follow each stream answer (see [Title pages](versions.md#title-pages)).

Some addons limit how many requests one address may send, commonly 10 stream requests per 15 seconds and 15 descriptions per 5 seconds. They answer the others with a notice, often with HTTP 200, not 429. So these background requests take their turn, by the server an addon is reached at. Its users' own copies, and the other addons it serves, share that turn.

| Requests | At once, at most | None while |
|---|---|---|
| Background streams | 2 | 8 stream requests went to the server in the last 15 seconds |
| Background descriptions | 2 | 12 descriptions were asked of the server in the last 5 seconds |

- **In order:** background requests take their turn in the order they came. The requests asked again after an answer wait behind the others: a title's first answer comes before another title's longer one.
- **Users first:** what a user waits for is never queued: opening a title, playback info, a play. It is sent at once, joining a request for the same title already under way. It counts in these figures, which leave it room below such limits.
- **No double asking:** a background request that waited uses the answer a user's request brought meanwhile.

After a refusal for its rate, an addon's server gets no background request for a while:

- **Refusals that count:** HTTP 429, 503 with `Retry-After`, or notices alone that say so ("rate limit", "too many requests"), including a description written as a title.
- **The pause:** as long as `Retry-After` asks, 15 minutes at most. Without it, a minute, when the refused title is asked again anyway.
- **Meanwhile:** requests users wait for are still sent. Watchlist checks skip the addon, and check its titles at their next hour.
- **The log:** it says once per pause, at Info, `An addon refused a request for its rate: Polyfin's background requests to its server pause`, with the addon and the wait.

There is nothing to set. An addon without limits only has Polyfin's background work spread over a few more seconds. A home page with 20 titles of Continue Watching and Next Up has them listed within about half a minute from one such addon, with their follow-ups after.

## The order of sources

The order of **Content › Sources** comes first. Each user's own addons follow, in their order under **My sources**. This order decides:

- which addon describes a title: the first that knows it;
- the order of the addons' subtitles;
- the order of search results;
- the order of a title's versions, addon after addon. **Order versions by**, in **Settings › Playback › Version order**, can say otherwise (see [Version order](versions.md#version-order)).

## Languages

A title's name, overview and images come from the addon that describes it. They are in the language set on that addon's configure page, which lives in its manifest address.

To let users read titles in their own language:

1. Add the same metadata addon once more for each language.
2. Configure each copy in its language, on the addon's configure page.
3. Name the language in **Language of its descriptions**, on the copy's page under **Content › Sources**. It is **None** by default.

Leave **Language of its descriptions** at **None** on the metadata addon the server had first, the one the copies come from. Do so whatever language its configure page sets, even one that is not the server's. Name a language only on the extra copies. The addons left at **None** give every user the titles' facts and episodes.

Each user then picks a **Metadata language** among those languages, under **My account**. An administrator can set it on the user's page under **Users** (see [Users](watch-state.md#metadata-language)). **The server's** is the default.

### What a user with a language sees

- **Title pages:** the description of the server's addon in that language. That is the name, overview, poster, backdrop and logo, and the episodes' names, overviews and images.
- **Lists:** libraries, collections, smart lists, search results and home rows show the name and poster in that language, once Polyfin keeps that description. Until then, they show the catalog's. Polyfin reads the descriptions in the background, a few at a time.
- **Search:** it also asks the search catalogs of the addons in that language, so a title is found by its name in that language too.
- **Titles with no description in the language:** they show the server's description.
- **Parental control:** users under parental control or blocking genres get their language too.

What does not change:

- A title's genres, ratings, people, versions, subtitles and identifiers stay those of the addons without a language. The first is the one whose catalog listed it.
- Genre pages, blocked genres, parental control, hidden titles and smart lists judge every user alike.
- Library names stay the administrator's, and generated images keep them.

### Addons with a language

- An addon with a language describes titles for the users of that language only. This holds even when its catalogs are libraries, unless no addon without a language describes them.
- An addon with a language and no library of its own only describes titles.
- Only the server's Stremio addons take a language.

When no addon without a language describes a title, the addons with a language describe it for every user, facts and episodes included. Those in the user's language come first, then the order of the sources. Search, people search and similar titles work the same way: when no addon without a language offers the search or catalogs they need, the addons with a language serve every user.

So a server whose metadata addons all have a language works as if they had none. **System › Health** warns about it (see [Health](administration.md#health)). Where an addon without a language describes the title, none of this applies.

Each language costs one more request per title. The addon in that language is asked once for each title opened or shown. Its descriptions are kept and refreshed as every description is (see [Catalog pages](addons-and-libraries.md#catalog-pages)). Catalogs are read once, whatever the languages.
