# Unmatched files

This page is for administrators of [local folders](local-folders.md). It covers the files Polyfin could not match to a title: why, how to link them by hand or from the web player, how failed searches are retried, and how unmatched files still show as plain videos. Naming and matching are on [Naming and matching files](naming-and-matching.md).

A folder's page lists the files no title was found for, with why. Meanwhile its library lists most of them as plain videos (see [Plain videos](#plain-videos)).

| Reason | What it means | What to do |
| --- | --- | --- |
| Its name gives no title, or no episode number | The name could not be read (see [Naming files](naming-and-matching.md#naming-files)) | Rename it, such as `Title (Year).mkv` or `Show (Year)/Season 01/Show S01E01.mkv`, and scan |
| Its name gives an episode number but no season | A show's file named `E05` or `Episode 5` outside a season's folder | Move it into its season's folder, such as `Season 02`, or rename it with its season, such as `S02E05`, and scan |
| Its name gives an air date | A show's file named with the date it was aired, `Show 2024-03-15.mkv`, which Polyfin does not read | Rename it with the catalog's season and number, and scan |
| Its name gives no episode number | A show's file whose name gives no number at all | Rename it with the catalog's season and number, and scan |
| The metadata addons know no title of this name | The search went through, and found no title of that name and kind, even with numerals read alike | Check the name as it was read, shown beside the file. Rename it, add an NFO file, or link it |
| Several titles have this name | Titles of that name and kind were found, and the name gives no year that leaves one. Or no title of the exact name and year, but several a looser rule leaves, such as titles of the year before and of the year after | Add the year to the name, correct it, or link it |
| Titles of this name are of other years | Titles of that name were found, none of the year the name gives nor of the year before or after. Or the file was unlinked from a looser match, and is matched by the exact rule only | Correct the year, or link it |
| No addon of the server describes and searches titles of this kind | No addon among the server's sources describes titles of the folder's kind and has a search catalog for them (see [What a search needs](naming-and-matching.md#what-a-search-needs)). A user's own addons do not count | Add such an addon under **Content › Sources** and scan, or link the file |
| The search failed | A metadata addon's search could not be read. The file names the addon and why (see [Failed searches](#failed-searches)) | Nothing at first: Polyfin searches again on its own. If it keeps failing, check the addon, or link the file |
| Another movie of the disc | A title of a Blu-ray disc, a DVD or a disc image other than its main one | Link it |
| A title of a show's disc | Its episodes are not known | Give its episodes (see [Shows on discs](discs.md#shows-on-discs)) |
| An extra without its title beside it | Its name is an extra's, but its movie is not beside it (see [Extras and theme media](naming-and-matching.md#extras-and-theme-media)) | Link it, to make it a title |

To fix a file:

- rename it and scan again;
- add an NFO file giving its identifier (see [NFO files](naming-and-matching.md#nfo-files));
- or type the title's IMDb identifier (the `tt…` part of its IMDb address) beside it and choose **Link**.

**What a link covers:**

| Unit | The link covers |
|---|---|
| A file in a movies folder | The file |
| A file in a shows folder | The whole show folder |
| A Blu-ray disc or a DVD | The whole disc folder, or one title of a disc holding several movies |
| A disc image | The image |

**Links to unknown titles:** a link to an identifier no metadata addon of the server describes as the folder's kind is flagged under **Linked by hand**. That happens, for example, when an episode's identifier was typed for its series'.

- The title then shows only its name, and its episodes those its files hold.
- This is checked when the link is made, and each time its files are matched.
- An addon that failed at that moment flags it too, until the files are matched again.

**Links last:** they survive every scan, even when the file changes or is moved away and back. Remove one from the folder's page to match the file by its name or NFO file again.

A file unlinked from a looser match (see [Looser matches](naming-and-matching.md#looser-matches)) is listed with the reason the exact rule gives, such as **Titles of this name are of other years**.

While the folder is scanned, the list follows the scan. Files found are added as they come, and an identifier being typed stays, so files can be linked before the scan ends.

**Titles of shows' discs** are listed with their disc, number and length, such as `Blu-ray disc: The Lone Ranger (1949)/Season 01/Disc 2 · title 1`. Give them their episodes there: see [Shows on discs](discs.md#shows-on-discs).

**Find the title:** to find a title without leaving Polyfin, choose **Find the title** beside a file.

1. The search is filled with what the file's name was read as, and its year. Change them if needed.
2. Choose **Search**, or press Enter. Polyfin searches the server's metadata addons for titles of the folder's kind, movies or shows.
3. It lists at most 10, with their poster, name, year and kind, those of the year given first.
4. Choosing one links the file, its show folder or its disc, as typing its identifier does.

- Only titles with an IMDb identifier are listed, since links are made to IMDb identifiers. For a title the search does not find, type its identifier.
- When some addons fail, the titles the others found are listed. When none found any, the search says it failed.

API details: [Local folder routes](jellyfin-api.md#local-folder-routes).

## Identify in the web player

An administrator can also link a movie's file or a show folder from the web player's **Identify**, in the ⋮ menu of its page. It works on:

- a plain video (see [Plain videos](#plain-videos));
- a title the folder's files were matched to, a wrong one a search chose included.

Identify looks the title up as Jellyfin does:

- **By an identifier typed, first.**
  - An IMDb identifier is looked up in the server's metadata addons.
  - A TMDB identifier is turned into the title's IMDb identifier by TMDB, with the server's **TMDB API key** (see [Edit images](artwork-and-editing.md#edit-images)). Otherwise, as without the key, it is looked up as `tmdb:…` in the metadata addons that take such identifiers.
  - A TVDB identifier is found on TMDB, with the key only. Polyfin has no access to TVDB of its own, so without the key a TVDB identifier finds nothing.
  - The title found is the one result, whatever the name and year say.
  - An identifier of a title of the other kind, such as a movie's typed for a show, finds nothing.
- **By the name and year typed:** when no identifier is typed or none finds a title. The server's metadata addons are searched, and what they find is listed.

Choosing a title and **OK** links the file, or the whole show folder, as **Find the title** does.

- The link is the same: listed on the folder's page under **Linked by hand**, and kept through every scan.
- A title the files were matched to has all of them linked: every file or disc of the movie, every folder of the show, in every movies or shows folder, since they all hold that one title.
- The page then shows the title chosen, with its description, artwork and versions. A show shows its seasons and episodes, the folder's files among them.
- The web player opens the item again by its identifier. A former title, last listed by the folder, answers the title chosen, or its season or episode of the same numbers, under their own identifiers. That lasts as long as none of the folder's files is matched to it again.
- The played marks, resume points and favorites of the former title's items follow the files, as for a plain video (see [Once matched](#once-matched)).

**Limits:**

- The web player offers Identify for movies and series, not for seasons or episodes. Other apps may apply a result to a season or episode: it links the show's whole folder too.
- Links are made to IMDb identifiers: a result without one is refused (400). Identify lists every title the addons find, those without an IMDb identifier included. **Find the title** lists only those a link can name.
- A title only addons list, which no local file is matched to, is its addon's title. Identify still finds titles for it, but applying a result fails, as a failed identification does in Jellyfin (see [Identify](artwork-and-editing.md#identify)).
- Identify is for administrators, as in Jellyfin.

**Edit metadata:** an IMDb identifier typed in the web player's **Edit metadata** of such a movie or show, plain or matched, links it the same way once saved. Other identifiers typed there, and one typed for a season or an episode, change nothing.

**Refresh metadata** on such a title, or one of its seasons or episodes, matches its files again before the metadata addons describe it again. It matches as a scan does: by its link, else the identifier its name or NFO file gives, else a search.

- A plain video whose search failed or found nothing is matched once a title is found.
- A title already matched keeps its files when nothing is found.
- It works from what the last scan read. New files, and NFO files added or changed, are read by the next scan, which watching the folder starts.
- The web player shows the result once the title's page is opened again.

To match a file or show folder to another title, use Identify, or type the title's IMDb identifier in its name (see [Naming files](naming-and-matching.md#naming-files)) or in an NFO file. Unlinking on the folder's page removes a link, or a match made by a looser rule. It does not apply to a title an exact search found.

**Compared with Jellyfin**

- Polyfin describes the title found from its metadata addons, by its IMDb identifier. TMDB only turns a TMDB or TVDB identifier into one, and only with the server's key. Jellyfin's TMDB providers look a title up by its TMDB identifier, then its IMDb one, then its TVDB one through TMDB's find, then its name, and answer what the first finds. Polyfin tries them in the same order ([TmdbSeriesProvider.cs, lines 55 to 127](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs#L55-L127), [TmdbMovieProvider.cs, lines 55 to 115](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Movies/TmdbMovieProvider.cs#L55-L115)).
- Polyfin links the files of a local title, the same title in every folder, unmatched or matched, to an IMDb identifier. The item opened again by its identifier answers the title it became, under the title's own identifier. A title only addons list cannot be bound to another. Jellyfin sets the provider identifiers of any item identified, keeps its identifier and refreshes it ([ItemLookupController.cs, lines 249 to 282](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemLookupController.cs#L249-L282), `ApplySearchCriteria`).
- Polyfin's admin app also takes an identifier typed, and identifies a file from its show's episodes too. The web player offers Identify for movies and series only. Both match an item by hand from a search of their providers, or by an identifier typed.
- Polyfin keeps only the IMDb identifier typed in **Edit metadata** of a local movie or show, as a link, at once. Jellyfin keeps every provider identifier typed, used at the item's next refresh ([ItemUpdateController.cs, lines 435 to 438](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemUpdateController.cs#L435-L438)).
- Polyfin's **Refresh metadata** matches the title's files again before it answers, from what the folder's last scan read. Jellyfin's queues the item's refresh, which reads its files again ([ItemRefreshController.cs, lines 62 to 93](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemRefreshController.cs#L62-L93)).

## Failed searches

A search fails when a metadata addon:

- could not be reached, or did not answer in time;
- asked to slow down (HTTP 429, or 503 with `Retry-After`);
- answered with a server error;
- refused the request (another HTTP error, "not found" included);
- or sent an answer Polyfin cannot read.

Each file left unmatched this way names the addon and why, as in `The search failed: Meta answered with a server error`.

- The folder's figures tell how many such files there are, and when Polyfin searches them again.
- **System › Health**, and the notifications of health problems, report it while files are left so.
- The log reports it once per scan, as a warning with the addon's error (`The metadata addons could not search the titles of a local folder's files`).

One addon failing fails no other: a title another addon finds is matched. A file none of the answering addons knows stays unmatched as a failed search, not as a title not found, since the addon that failed may know it. So does a file only a looser rule would match, since the failed addon may know the title of its exact name and year.

Within a scan, Polyfin retries a failed search:

| The addon's search | Polyfin, during the scan |
|---|---|
| Fails with a server error (5xx) | Tries again after 1 s, then 3 s |
| Asks to slow down | Waits as its `Retry-After` asks, else 1, 2, then 4 s, then tries again, 3 times at most. A `Retry-After` longer than 1 minute fails the search at once. |
| Refuses (any other error, "not found" included) | Does not try again |

A folder whose files failed searches left unmatched is scanned again 5 minutes after the scan. The wait doubles each time, up to 6 hours. **Scan now** searches again at once.

- The wait is never shorter than an addon's `Retry-After` asks.
- The scans due are looked for every 10 minutes.
- Once the searches go through, the files are matched without anyone acting, and the folder returns to its schedule.

**When searches keep failing:** check the addon under **System › Health › Addons**, and that the computer running Polyfin reaches it. Meanwhile:

- link files by hand, or give them an NFO file;
- or add their title's IMDb identifier to the name of a show's folder or a movie's file, such as `Victorious (2010) [imdbid-tt1604099]`. An identifier in a name is matched without any search (see [Matching](naming-and-matching.md#matching)). The folder's page says so beside the failed searches;
- or add a second metadata addon with a search catalog under **Content › Sources**: a title either finds is matched.

### Through TMDB when every addon fails

While the server has a **TMDB API key** (see [Edit images](artwork-and-editing.md#edit-images)), a file whose search failed at every metadata addon of the server is searched on TMDB instead, by the name and year its name gives.

- **The same strict rule:** the file is matched only to the one movie or show of that name, of the folder's kind, and of that year when the name gives one, as an addon search matches first (see [Matching](naming-and-matching.md#matching)). TMDB's original names count as names. No looser rule applies.
- **Its IMDb identifier:** the file is matched to the title's IMDb identifier, from TMDB's external identifiers, as a link would. The addons describe the title once they answer again. Meanwhile it shows TMDB's name, year and poster.
- **Told apart:** the folder's figures count such files apart, as matched "by a TMDB search, as the metadata addons' searches failed". They no longer count among the failed searches of the folder's page and of **System › Health**.
- **Otherwise, as before:** a name TMDB does not find, finds several titles for, or whose title has no IMDb identifier, stays unmatched as a failed search. It is searched again on the schedule above. So is every such file without a key, or when TMDB cannot be reached, which the log reports as a warning.
- **Only failures:** a search an addon answered, even finding nothing, is never asked of TMDB. Nor is one that only some addons failed while another answered. Files once matched keep their title, as other matches do.

**Compared with Jellyfin**

- Polyfin asks TMDB only when all its metadata addons fail, and keeps a title only when it is the only one of its name and year. Jellyfin searches TMDB by name and year whenever an item has no provider identifier ([TmdbSeriesProvider.cs, lines 198 to 214](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs#L198-L214), [TmdbMovieProvider.cs, lines 160 to 176](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Movies/TmdbMovieProvider.cs#L160-L176)). It keeps the best-scored result, with its IMDb identifier from TMDB's external identifiers ([TmdbSeriesProvider.cs, lines 306 to 310](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs#L306-L310)).

## Plain videos

A movies or shows folder's library lists the files it leaves unmatched. So a folder whose files match no title does not look empty in Jellyfin apps, as Jellyfin lists the files it cannot identify. Each is a plain video: a title of the folder's own, which no metadata addon is asked about, with no description, rating or provider identifier.

```
/media/movies/Harbor.Lights.2.2011.1080p.mkv                → the movie "Harbor Lights 2", 2011
/media/movies/[Group].mkv                                    → the movie "[Group]"
/media/shows/Harbor Show (2011)/Season 01/Harbor Show S01E02.mkv → "Harbor Show", 2011: season 1, episode 2, "Harbor Show S01E02"
```

- **Movies folder:** each file, disc or disc image left unmatched is a `Movie`. It is named as its name was read for the search, the name shown beside it in the unmatched files, with the year the name gives. A name that gives no title names it as its file, without its extension.
- **Shows folder:** each show folder left unmatched is a `Series`, named as its folder's name was read, with the year it gives.
  - Its seasons and episodes are those its files' names give, read as a matched show's are (see [Naming files](naming-and-matching.md#naming-files)).
  - A file holding two episodes is both. A title of a show's disc is the episodes given to it (see [Shows on discs](discs.md#shows-on-discs)).
  - Apps order them by season, specials being season 0, then by number.
  - Each episode is named after its file, without its extension, as `Harbor Show S01E02`. A disc's title is named as `Episode 5`.
- **No episode number:** a file whose name gives no episode is left out of its show, matched or not. It has no place among the episodes, nor an episode to keep once the show is matched. The folder's page still lists it: rename it with its episode, such as `S01E05`, and scan.
- **Not plain videos:** the other movies of a disc, the titles of a show's disc without episodes, and the extras without their title. Jellyfin lists a disc as one movie and an extra only under its title. Link them to list them.

Plain videos play as the folder's other files do: direct play, repackaging or conversion, with the subtitle files beside them, and a disc over HLS (see [Versions](local-folders.md#versions)).

- The images beside them are theirs, as a matched title's are, when the folder shows its artwork (see [Artwork](naming-and-matching.md#artwork)).
- Search, watch state, resume points, favorites, Continue Watching, collections and downloads work as for other titles.
- Each user's hidden titles and visible libraries apply to them. So do smart lists drawing from the folder, by their rules, and the version rules and quality groups.
- Tracking services and scrobbling never hear of them: they have no identifier a service knows.

Parental control judges them as unrated titles, as Jellyfin judges the items it cannot identify. A plain movie is an unrated **Movie**. A plain series, with its seasons and episodes, is an unrated **Series**. A user blocking unrated movies sees no plain movies, and a highest rating alone hides none.

### Identifiers

A plain video's identifier is made of its folder's identifier and the path, in the folder, of its file, disc or show folder. It is `polyfin-plain:` and the first 32 hexadecimal digits of their SHA-256 hash. An episode's adds its season and number.

- It is the same through every scan and restart, so apps' links, watch states and resume points stay with it.
- A file or show folder renamed or moved, within its folder or to another, is another plain video. Jellyfin likewise makes another item of an unidentified file whose path changed.
- Polyfin does not follow it. What users did with the former one stays with it, no longer listed.

### Once matched

A plain video leaves the library once its file, disc or show is matched, by a scan or by hand (see [Matching](naming-and-matching.md#matching)). Its title is listed in its place.

What users did with it goes to that title, as when what a folder holds changes, by an import's rules (see [Changing what a folder holds](local-folders.md#changing-what-a-folder-holds)):

- a played mark is added to the title's;
- the newer resume point is kept, and moves, so Continue Watching lists the file once, under its title;
- a favorite is set;
- a plain episode's go to the matched show's episode of the same season and number.

A link made by hand moves them at once, a scan at its end.

The plain video is gone. No list shows it any more: not the library, Continue Watching, the favorites nor the played items.

- An app that opens it again by its identifier, as the web player does right after **Identify**, is answered the title it became. It may instead get that title's season or episode of the same numbers, with the title's own identifier, as Jellyfin keeps an item it identifies.
- Anything else asked of that identifier, such as playing or marking it, is answered that it does not exist.
- It keeps its played mark and favorite all the same.

Removing the link, or the match going, as when an NFO file is removed, lists the file as the same plain video again. It has the played mark and favorite it had. What users did with the title meanwhile stays with the title.

A file renamed and matched at once is a new file to the scan: its title gets nothing of the plain video it was.

**Compared with Jellyfin**

- Polyfin names a plain movie as its name was read for the search, its dots and underscores read as spaces. Jellyfin names it after its file, cleaned of what follows its year or a release tag, its dots kept, as `curse.of.chucky` for `curse.of.chucky.2013.1080p.bluray` ([CleanDateTimeTests.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/tests/Jellyfin.Naming.Tests/Video/CleanDateTimeTests.cs#L22)), with the year it gives ([MovieResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Server.Implementations/Library/Resolvers/Movies/MovieResolver.cs#L305-L306), [VideoResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Naming/Video/VideoResolver.cs#L87-L96)).
- Both name a series after its folder, `Name (Year)` giving its year ([SeriesResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Naming/TV/SeriesResolver.cs#L36-L51)), and an episode after its file as written ([EpisodeResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Server.Implementations/Library/Resolvers/TV/EpisodeResolver.cs#L58), [BaseVideoResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Server.Implementations/Library/Resolvers/BaseVideoResolver.cs#L117-L122)).
- Polyfin leaves out an episode whose name gives no number. Jellyfin lists it without one.
- Neither lists an extra without its title ([VideoListResolver.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Naming/Video/VideoListResolver.cs#L51-L55)). Both judge plain videos as unrated movies and series ([BaseItem.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/MediaBrowser.Controller/Entities/BaseItem.cs#L2000-L2024)).
- Both make an identifier that changes when the file is renamed. Polyfin's is a hash of the folder and path. Jellyfin's is a hash of the item's type and path ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/c7ce6a1b56ee7dcc1896250df0f2f9deaa828a81/Emby.Server.Implementations/Library/LibraryManager.cs#L792-L818)).
- Polyfin lists the title found in place of the plain video, which gives it its watch state, resume point and favorite (see [Once matched](#once-matched)). Jellyfin keeps the item it identifies later, its metadata filled in.

API details: [Plain videos in the Jellyfin API](jellyfin-api.md#plain-videos-in-the-jellyfin-api).
