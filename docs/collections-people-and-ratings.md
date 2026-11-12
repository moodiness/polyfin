# Collections, people and ratings

This page is for administrators. It covers collections from addons and from users, people and their pages, ratings from MDBList, and the TMDB key that fills in people, casts, posters and release dates.

## Collections

### Collections from addons

An addon's collection catalogs become collection libraries. Each collection gathers the catalogs it groups, movies and series together.

A collection may also group other collections, such as a genre's movies and series, or a franchise's movies in several sagas:

- **Opening it:** it shows the titles of every collection within it together, up to three levels deep, as a collection of catalogs does. It never shows those collections as folders.
- **Order:** when the app reads them all at once, as jellyfin-web's collection page does, they come by release date once Polyfin has read them all, as Jellyfin orders a collection. Until then, and in pages, they come one of each catalog in turn.
- **A collection of catalogs only** keeps their order, such as their titles' popularity.

**Refresh descriptions**, in the menu of a collection library's row under **Content › Libraries**, reads again the descriptions of its collections, then of their titles (see [Library description refresh](addons-and-libraries.md#library-description-refresh)).

#### Home rows of collection libraries

On the home page of Jellyfin apps, each collection library has a row of its collections, "Recently Added in" the library's name. A movie or series library has a row of its titles the same way.

- **Leaving it out:** in jellyfin-web, a user can leave a library out of these rows in the web player's **Settings › Home**.
- **Where it opens:** the row's title, like the library's links in the header and the menu, opens the library on the screen chosen for it there, under "Default screen". The default is "Folders", which lists its collections.
- **Per browser:** jellyfin-web reads that choice for these libraries in the browser where it was saved. Another browser opens "Folders" until the choice is saved there too.

Some apps list a library by its type. Told a collection library holds mixed content, they show the titles of all its collections together, on their home and on the library's page. Tick such apps in **Apps that show collection libraries by collection**, in **Settings › Catalogs**:

- The list shows the apps that have signed in to the server, by the name they give.
- The change applies to their next request.
- On the home of the apps chosen, a collection library lists its collections by its own type, and a collection opens on its titles.
- jellyfin-web and Streamyfin leave libraries of collections out of their home rows: leave them unticked.

#### Images of collections

A collection whose catalog gives no poster shows one Polyfin draws (see [Generated images](artwork-and-editing.md#generated-images)):

| Image | Drawn from |
|---|---|
| Primary | The posters of its first four titles, in its own order, under a gradient, with its name across them |
| Thumb, a wide image for apps that show wide cards | Its first six titles |

- It comes once Polyfin has read the collection's titles, as when an app opens it or the task "Read every collection" runs. Until then the collection shows none.
- Listing the collections asks the addon for nothing more.
- A collection whose catalog gives a poster keeps it.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Generate images for collections without one** | **Settings › Content › Images** | On | Draws an image for the addons' collections without a poster, and for the smart lists shown as collections. Off, they show none. Libraries set to **Generated** keep theirs. |

The change applies at once.

**Compared with Jellyfin**

- Polyfin's collection libraries get a home row; a Jellyfin library of collections does not. Polyfin tells apps they are libraries of mixed content, so that they get one. The apps chosen in **Apps that show collection libraries by collection** are told they hold collections, as Jellyfin's Collections view. Opening one shows its collections.
- Without an image, a collection library's tile shows the icon of a mixed library. Set its image to **Generated** to show its first collections' posters (see [Library images](artwork-and-editing.md#library-images)).
- Polyfin draws an image for a collection without one. Jellyfin draws none: apps show their placeholder.
- Polyfin's [web player script](web-client.md#dashboard-links-and-single-sign-on) opens a collection library on its default screen from its row's title, as such a library has no suggestions of its own. jellyfin-web opens a library's Suggestions from there, whatever its default screen.

API details: [Collections from addons](jellyfin-api.md#collections-from-addons-for-apps).

### Collections made by users

Users who may manage collections group titles into collections from their apps, with **Add to collection** in jellyfin-web. A collection can hold movies, series, seasons, episodes, an addon's collections and Live TV channels.

Collections belong to the server:

- Once one exists, every user's apps show a **Collections** view, listing them by name.
- A listing of collections across the server lists them too.
- A library's Collections tab lists the collections holding a title from it.

Each user sees only the titles they may see:

- Titles hidden by parental control or blocked genres do not show in a collection. They do not count in its item counts or played count.
- Allowed hours apply as everywhere.
- A collection none of whose titles a user may see still shows to them, empty.

A collection:

- lists its titles by premiere date;
- takes their genres, their least restrictive rating and the earliest premiere date;
- brings a series' released episodes when played as a whole;
- uses as its poster that of the first title the user sees, as their listing tags it. An episode shows its series' poster. Without a tag, it uses its first title's poster.

Nothing else is looked up for a collection.

Rules:

- A collection needs a name.
- The titles added must be ones the user adding them may see. Otherwise the request changes nothing.
- Collections hold no people, programmes or other users' collections.
- The Collections view is not among the visible libraries an administrator chooses, as the Playlists view is not.

### Permission

**Can manage collections**, in the **Access** section of a user's page under **Users**, lets a user manage collections (see [Users](users.md)).

- It is on for administrators, existing ones included, and off for other users.
- Without it, nobody can create a collection, or add and remove its titles, administrators included.
- Administrators, and users who may manage collections, can delete a collection.

**Compared with Jellyfin**

- User collections are Jellyfin's BoxSets. A collection lists its titles, takes genres, rating and premiere date, and plays a series' released episodes, as in Jellyfin.
- Jellyfin's IsLocked is kept as apps send it.
- Polyfin shows a collection empty when none of its titles is visible to the user. Jellyfin hides it.
- Polyfin refuses a missing name, or a title that does not exist, and changes nothing. Jellyfin creates the collection and then fails.
- **Can manage collections** is Jellyfin's `EnableCollectionManagement`, which administrators' apps also set from the user's policy.

API details: [Collections made by users](jellyfin-api.md#collections-made-by-users-for-apps).

## People

The actors, directors, writers and producers a title credits open as people, with their photo.

| | Without a TMDB key | With a TMDB key |
|---|---|---|
| A title's people | Those its metadata addon gives | Those TMDB credits (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb)) |
| A person's details | Their name and the addon's photo | What TMDB knows of them (see [Photos and biographies from TMDB](#photos-and-biographies-from-tmdb)) |
| Who is who | One person per name | One person per TMDB identifier, when the credit gives one |

A title's cast shows the photo Polyfin keeps of someone its own credits give none of. That is the photo another title's addon gave, else TMDB's, as Jellyfin shows a person's image wherever they are credited.

### Credits as links

An addon may give people as Stremio links rather than lists, as Stremio's protocol now advises:

| Kind | Link categories, in any letter case |
|---|---|
| Cast | `Cast`, `actor`, `actors` |
| Directors | `Directors`, `director` |
| Writers | `Writers`, `writer` |

- Polyfin reads them in their order, as Stremio apps show them, for each kind the description gives no list of.
- A list the addon gives wins over its links. Polyfin keeps no other link.
- A description kept from before Polyfin read links shows them once it is read again (see [Catalog pages](addons-and-libraries.md#catalog-pages)), or at once with **Refresh metadata**.

### Who is who

While the server has a **TMDB API key**, a person is kept by their TMDB identifier when the credit gives one. TMDB's credits always do, and some addons do (`tmdb_id`, or a numeric `id`, in `app_extras.cast`). So two people sharing a name are two people, each with their own titles, photo and biography.

A credit by name alone, and every credit without a key, is the person of that name:

- **Becoming identified:** a person kept by name becomes the person of a TMDB identifier when a credit tells who they are. That is a credit of the same name and that identifier. For someone whose identifier is not known, it is TMDB's credits of a title the addon credited them in. Their photo, biography, credits and the users' favorites move to that person. The name keeps pointing to them, so its links, and credits of that name alone, still reach them.
- **Free names:** a name no one is kept by points to the first person of that name a credit identifies.
- **Namesakes:** a person kept by name, known as someone else (another identifier), stays apart.
- **Upgrading:** the people an addon gave a TMDB identifier move to that identifier, merging the names of one identifier. Others stay kept by name until a credit tells who they are.

### Titles on a person page

A person's page lists, in this order:

1. the titles your addons' people-search catalogs find for their name, such as a catalog named People Search;
2. the titles of your addons that Polyfin already knows them in;
3. with a **TMDB API key**, the titles of your libraries TMDB credits them in, though no one opened them (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb));
4. with a **TMDB API key**, up to 15 movies and series TMDB knows them best for, even outside your libraries (see [Titles a person is known for](#titles-a-person-is-known-for)).

- **Listed once:** a title of the second kind that a people search also finds, under another addon's identifier, is listed once, as the title of your library. An example is `tmdb:…` for the IMDb identifier a library lists.
- **No people search:** without a people-search catalog, only the other kinds show.
- **Without a key:** a title's people are those of the description its page shows them from. That is the first addon without a language that describes it, starting with the one whose catalog listed it (see [Languages](addons.md#languages)).
- **Changed credits:** when that description is read again, in the background or with **Refresh metadata**, someone it no longer credits loses the title on their page, as in Jellyfin.
- **Other descriptions:** other addons' descriptions, and those of addons given a language, change no one's titles.

The people list holds the people credited in the titles of your addons that someone opened, or whose credits TMDB gave. People credited only in other users' own addons stay hidden.

### Cast and crew from TMDB

Metadata addons credit what they credit: some only a title's first actors, by name, and no crew but its directors and writers. While the server has a [TMDB API key](#tmdb-key), a movie or series TMDB knows takes its people from TMDB. TMDB knows it by its TMDB identifier, else by its IMDb one through TMDB's find.

- **Who:** its first 15 actors in billing order. Then its first 15 directors, writers (of the script, screenplay or novel) and producers, in TMDB's order.
- **Series:** a series' actors are those of its whole run. An actor of several characters is credited once a character, the one they played the longest first.
- **With:** each person's TMDB identifier, which is who they are (see [Who is who](#who-is-who)). Also their role, the character in the server's metadata language, and their TMDB photo, else the photo the addon gives them.
- **Fallback:** the addon's people stay without a key. They also stay for a title of no TMDB or IMDb identifier, one TMDB does not know or credits no one in, while TMDB fails, and until the title's credits were read. Taking the key off brings the addon's back.

When Polyfin reads credits:

- **Title pages:** a title's page asks for its own, ahead of the rest. It waits up to a second, else shows them the next time.
- **Daily pass:** a minute after Polyfin starts or the key is saved, then each day, a pass reads the credits of the first 2,000 movies and series of each server library. It reads those not read yet, or read 30 days ago or more, so that people list titles no one opened. Each pass logs, at Info, the libraries it read and the titles it queued.
- **Collection libraries** are not read. On a server whose libraries are all collections, titles' credits are read when their pages open.
- **Beyond 2,000:** a title beyond a library's first 2,000 is read when its page opens.

**Pace:**

- Titles' credits and box sets (see [Box sets](silo-apps.md#box-sets)) wait their turn together, at most four titles a second.
- One request to TMDB a title, or two for one known only by its IMDb identifier that box sets did not look up.
- They pause as long as TMDB asks when it says to slow down. A page's title goes ahead of the passes.
- A first pass takes about 10 minutes per library of 2,000 titles. Later ones ask only for the credits grown old.
- The bound keeps TMDB's load in check. For every title of a server of 43,000, 26,000 of them movies, they would take about 3 hours and up to 84,000 requests. That is nearly 5 hours with the movies' box sets.

**Kept:**

- Credits are kept for every user, in the database, 30 days.
- A title TMDB did not know is asked about again after 30 days. One whose lookup failed is asked again when next opened or passed.
- **Refresh metadata** on a movie or series reads its credits from TMDB again, ahead of the passes, as Jellyfin's refresh asks its providers again. The kept ones show until the new ones come.
- Refreshing a library's descriptions does not. Its titles' credits are read again by the passes once 30 days old.

#### Library titles on person pages

A person's page lists the titles of your libraries TMDB credits them in, from TMDB's list of their titles.

- The list is read once the page of a person of a known TMDB identifier opens, waiting up to 3 seconds. It is kept 30 days.
- A title matches by its TMDB identifier, or by the one TMDB's credits or box sets learnt for its IMDb identifier. One neither learnt yet is missed until they do.
- Parental control, blocked genres, the titles you hid and the addons you use apply, as to the titles you know them in.
- Of the titles of TMDB's list your libraries do not hold, those the person is best known for follow (see [Titles a person is known for](#titles-a-person-is-known-for)).

**Compared with Jellyfin**

- Jellyfin takes a title's people from TMDB the same way: 15 actors in billing order and 15 directors, writers and producers by default, a series' actors from its aggregate credits, one credit per role ([TmdbMovieProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Movies/TmdbMovieProvider.cs#L276-L360), [TmdbSeriesProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs#L365-L420), [TmdbUtils.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs#L262-L329), [PluginConfiguration.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Configuration/PluginConfiguration.cs#L86-L91)). The bounds are not settings in Polyfin.
- Polyfin reads credits in a daily pass bounded to each library's first 2,000 titles, and for any title whose page opens. Jellyfin reads them as its scans and refreshes reach each item.
- Polyfin keeps people by TMDB identifier. Jellyfin keeps them by name ([LibraryManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L1221-L1253)), so namesakes share one person and its TMDB identifier.
- A Polyfin person's page lists the titles whose credits it read, plus the titles of your libraries TMDB credits them in. That comes to the same as Jellyfin without reading every title. Jellyfin lists the items of its libraries crediting them ([Person.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/Person.cs#L67-L70)), which its scans read.
- The titles beyond the libraries are Polyfin's own (see [Titles a person is known for](#titles-a-person-is-known-for)).

### Studios and taglines from TMDB

While the server has a [TMDB API key](#tmdb-key), a movie or series whose credits were read from TMDB (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb)) also gets its studios and tagline from TMDB.

- **Studios:** a movie's are the companies that made it. A series' are the networks it aired on, then the companies that made it, each name once. jellyfin-web lists them on the title's page, each a link to the studio's page (see [Studio pages](#studio-pages)), and Silo's apps on its cards.
- **Tagline:** TMDB's, in the server's metadata language. A title TMDB has no tagline for in that language shows none. A tagline read in another language is hidden until the title is read again in the new one. On a title's page, it comes before the "for you" tagline (see [When the profile is confident](home-rows.md#when-the-profile-is-confident)).
- **Cost:** none of its own. They come in the same request as the credits, kept and read again with them, with each network's and company's TMDB identifier and logo. Credits read before Polyfin kept studios, or their identifiers, are read again once, at a title's page or in the next pass.
- **Edits win:** studios or a tagline set in the metadata editor replace TMDB's. Saving the editor without changing them keeps following TMDB. Emptying an edited list goes back to TMDB's.
- **Fallback:** no studios and no tagline without a key, for a title whose credits are not read, or whose credits TMDB does not know.

A studio is a name, whatever its letter case: a network and a company of one name are one studio.

#### Studio logos

A studio shows its TMDB logo, drawn by Polyfin on a light grey plate. TMDB's logos are mostly dark on a transparent background, which a dark app would not show.

- **Which logo:** its network's, else its company's. Among several, the lowest TMDB identifier that has a logo.
- **Shapes:** a wide image (Thumb, 960 by 540 pixels) for jellyfin-web's Studios tab of a library and for favorites, and a square one (Primary, 600 by 600) for search results. The logo fits within the plate, an eighth of each side left free around it.
- **Smaller margin:** where a logo shows small, an app may ask for a plate with a sixteenth of each side left free, so the logo takes more of it (see [Studios for apps](jellyfin-api.md#studios-for-apps)). That plate is kept apart from the other, so neither is served for the other.
- **Cost:** the logo comes with the title's details, with no request of its own. Polyfin downloads TMDB's 500-pixel PNG the first time an app asks for the image, and keeps the plate in the image cache, as the images it draws for collections (see [Images of collections](#images-of-collections)). Smaller asks get it resized, as any artwork.
- **Changes:** a new logo on TMDB shows once a title of that studio is read again, under a new image tag, so apps fetch it again.
- A studio TMDB has no logo for shows its name only.

#### Studio pages

A studio's link opens its page, which lists, in this order and each title once:

1. **Catalogs:** the titles of the catalogs that offer its name in their genre filter, as a genre's page (see [Genre, studio and year pages](addons-and-libraries.md#genre-studio-and-year-pages)).
2. **Your titles:** the titles you reach whose details from TMDB name the studio, by name, at most 1,000.
3. **From TMDB:** the 40 most popular movies and the 40 most popular series of the studio on TMDB, from TMDB's discover. Movies are those of its companies; series those of its networks, else of its companies. Titles for adults, not out yet, or of no date are left out.

The titles from TMDB open as the titles a person is known for do (see [Titles a person is known for](#titles-a-person-is-known-for)): through your metadata addons, by TMDB identifier, else by IMDb identifier. A title none of your addons describes is left out.

Rules:

- Parental control, blocked genres, the titles you hid and movies waiting for their digital release apply, as on every listing.
- The page's counts of movies and series come from the same listing: its first page, plus one when more follow.
- Without a key, the page lists the catalogs' titles and your titles only.

Requests:

- Discover costs two requests of each kind, kept in memory for 24 hours.
- An IMDb identifier that box sets or TMDB's credits did not learn costs one request a title, in the queue of TMDB's lookups, kept as for a person's titles.
- The page waits up to 3 seconds for them. Titles that come later show the next time.

jellyfin-web's Studios tab of a library lists the studios of its first 2,000 titles whose details are kept, sorted by name, each with its counts of movies and series. Asked without a library, Polyfin lists those of every library of movies or series you reach. Search finds them by name, and they can be favorites.

**Compared with Jellyfin**

- Jellyfin gives a movie its production companies and TMDB's tagline. It gives a series its networks only, and no tagline ([TmdbMovieProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/Movies/TmdbMovieProvider.cs), [TmdbSeriesProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TV/TmdbSeriesProvider.cs)).
- Jellyfin's studio images are wide only, ready-made pictures from its own artwork repository, matched by name ([StudiosImageProvider.cs, lines 54 to 56](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/StudioImages/StudiosImageProvider.cs#L54-L56), [Plugin.cs, line 22](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/StudioImages/Plugin.cs#L22)). Polyfin draws TMDB's logos, wide and square.
- Jellyfin's studio page and Studios tab list the items of its libraries ([StudiosController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/StudiosController.cs)). Polyfin also lists the studio's catalog titles and TMDB's most popular titles of it that your addons describe.

### Titles a person is known for

A person credited in few titles of your libraries would show few titles. While the server has a **TMDB API key**, the page of a person of a known TMDB identifier also lists the movies and series TMDB knows them best for. They come from the same list of their titles, after those of your libraries.

- **Order:** the titles most voted for on TMDB first, a lasting measure of how well known a title is. Among equal votes, the most popular on TMDB now, then movies before series. This is close to the "Known For" of TMDB's pages.
- **Left out:** talk shows and the news; titles for adults; and titles not out yet, or of no date. Also left out are the person's appearances as themselves, a character TMDB names "Self", "Himself", "Herself" or "Themselves", such as "Self - Host". They stay when the person also plays a role in the title, or works on it.
- **How many:** at most 15, from the first 30 of that order, those you may open and see. A title of your libraries among them counts, stays where it is, and is listed once.
- **Filling in:** other titles Polyfin knows the person in, that no catalog of yours listed, fill in after them within the 15. An example is one of these whose description was read. Without a key, they are all listed.

Opening them:

- Each is a title of your metadata addons. A title Polyfin already knows under its TMDB or IMDb identifier, and that your addons reach, is that one.
- Otherwise it is the title `tmdb:<id>`, when one of your addons describes titles of that prefix and type. Else it is the title of its IMDb identifier, when one describes those. TMDB's external identifiers give it.
- A title none of your addons describes is left out.
- Until its addon describes it, a title shows TMDB's name, date and poster. It opens, and plays, through your addons as a search result does.

Requests:

- The person's list of titles is the one their library titles are matched from, with no request of its own.
- An IMDb identifier that box sets or TMDB's credits did not learn costs one request a title. It joins the queue of TMDB's lookups (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb)), ahead of the passes.
- It is kept for good once found, 30 days when TMDB gives none.
- The page waits up to 3 seconds for them. Titles whose identifier comes later show the next time.

Rules:

- Parental control, blocked genres, the titles you hid and movies waiting for their digital release apply, as on every listing.
- For a user whose parental control hides titles, a title whose rating is not known yet is left out, and looked up. It shows once its rating is known and allows it.

Without a key, or for a person of no known TMDB identifier, the page lists the other titles only. Upgrading forgets the lists of titles kept before, which lacked votes and popularity. Each person's list is asked of TMDB again, once, when their page next opens.

**Compared with Jellyfin**

- Polyfin also lists the titles a person is known for, which your addons describe outside your libraries. Jellyfin lists only the items of its libraries crediting a person (`Person.GetTaggedItems`, [Person.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/Entities/Person.cs#L67-L70)), and never TMDB's list of their titles.

### Photos and biographies from TMDB

Metadata addons give people's names, and some their photos. None gives a biography, and few give photos of directors and writers. While the server has a [TMDB API key](#tmdb-key), Polyfin looks each person up on TMDB once, for every user.

- **What it takes:** their profile photo, for people their credits give none of, or whose photo is dead (see below). Also their biography, birth date, date of death and place of birth, which jellyfin-web shows on their page. A person whose credits give a working photo keeps it and gets the rest. TMDB's credits of a title (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb)) give their people's photos at once.
- **When:** the first time their page opens, which waits up to 3 seconds for TMDB, else shows it the next time. Also when a title's page shows them in its cast without a photo. That does not wait: the photo shows the next time the cast shows.
- **Pace:** two people are looked up at a time, and at most 500 wait. Nothing is asked without a key.
- **Language:** the biography is in the [metadata language](watch-state.md#metadata-language) of the user who opens the page, else in the server's, each asked once. When TMDB has none in it, it is TMDB's English one.

**Who they are:**

- The person of the TMDB identifier their credits give, without a search. TMDB's credits of a title give everyone's. Some addons give it (`tmdb_id`, or a numeric `id`, in `app_extras.cast`).
- Else the one TMDB finds by that very name, whatever the case.
- Of several people of the same name, Polyfin takes the one whose photo the addon gives, when it is TMDB's. Else it takes the one credited in the titles Polyfin knows the person in, matched by TMDB identifier, or by name and year, a year apart at most.
- When no title tells them apart, the person gets nothing, rather than someone else's photo and life.

What wins, first to last:

1. an image an administrator chose in **Edit images**, or a field changed in **Edit metadata**, which TMDB never replaces;
2. the photo of their credits, while it works: TMDB's for TMDB's credits of a title, else the addon's;
3. TMDB's photo of the person.

**Dead photos:**

- A photo whose artwork server answers `404` or `410`, or that failed 3 times running, is dead on that person for a day (see [Artwork](artwork-and-editing.md#artwork)).
- The person shows TMDB's photo in its place, looked up as for a person without one. With no photo left, none: apps then draw their person placeholder rather than an empty card.
- The addon describing the title again with the same photo keeps it dead. A new photo from the addon is tried at once.
- A person kept by name who becomes the person of a TMDB identifier (see [Who is who](#who-is-who)) keeps their dead photos.

**Kept:**

- On the person, across restarts.
- A person TMDB did not find is asked about again after 30 days.
- A lookup that failed is tried again after 10 minutes, then twice as long each time, up to a day.

**Refreshing:**

- **Refresh metadata** on a title forgets the failed downloads of its people's photos, and the dead ones. It looks the people without a working photo up on TMDB again.
- On a person, it forgets their photo's failures, and looks them up again, whatever they had.
- Either asks TMDB anew, rather than reusing its answers of the last day.

**System › Health** shows how the lookups stand (see [Health](administration.md#health)).

**Compared with Jellyfin**

- Jellyfin fills a person from TMDB the same way: by the TMDB identifier it has, else the first person TMDB's search finds by name. It takes their biography, birth and death dates, place of birth and photo in the server's metadata language ([TmdbPersonProvider.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/People/TmdbPersonProvider.cs#L89-L144)), and runs it again when the person is refreshed.
- Polyfin asks TMDB only for people a page shows. Jellyfin asks when a library scans.
- Of several people of the same name, Polyfin takes the one credited in the person's titles, or no one. Jellyfin takes the first.
- Polyfin shows each user the biography in their own language, else TMDB's English one. Jellyfin keeps one per person.
- Polyfin sets a dead photo aside for a day, as the addon's server may serve it again, and **Refresh metadata** tries it at once. Jellyfin removes a remote image that answers `404` or `403` from its item, so that it is not tried over and over, and a later refresh may fill another ([LibraryManager.cs, lines 3778 to 3795](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/LibraryManager.cs#L3778-L3795)).

## Ratings

With a server key from MDBList, movies and series show three ratings from MDBList: their IMDb rating, their critics' score and their audience score.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **MDBList API key for ratings** | **Settings › Content › Ratings** | Empty | The server's own MDBList key, checked with MDBList when saved. Empty, nothing changes and MDBList is never asked. Users' own MDBList keys for tracking are never used for this. |
| **Critic rating shows** | **Settings › Content › Ratings** | **Critics' score** | Which score apps show as the critic rating: **Critics' score** or **Audience score**. |

**What apps show:**

| Rating | Holds | Scale |
|---|---|---|
| Community rating | The IMDb rating from MDBList, else the addon's | 0 to 10 |
| Critic rating | The score chosen in **Critic rating shows** | 0 to 100 |

- The web player shows all three on a title's page (see [Web player](web-client.md)). It shows them on posters too, while **Show ratings on posters** is on (see [Ratings on posters](web-client.md#ratings-on-posters)).
- An administrator's edit still wins (see [Edit metadata](artwork-and-editing.md#edit-metadata)). An edit that leaves the ratings as shown keeps following MDBList.

**When Polyfin asks MDBList:**

- When a title's page opens, or when a movie or series whose ratings it does not keep is listed. It asks in the background, up to 100 titles a request.
- Listings and title pages never wait. They show what is kept, and the next listing shows the rest.
- Titles without an IMDb or TMDB identifier, episodes, Live TV and music are not asked about.

**What it keeps:**

| Ratings | Asked again after |
|---|---|
| Of a title released in the last year | 7 days |
| Of a title released over a year before | 30 days |
| Of a title MDBList does not know | 30 days |

- Ratings are kept in the database by identifier, for every user.
- **Refresh metadata** forgets a title's ratings.
- The votes the IMDb rating counts are kept with it, for smart lists' **IMDb votes** rule (see [Smart lists](smart-lists-and-search.md#smart-lists)). Ratings kept before Polyfin read votes are asked about again as their titles are next listed.

**MDBList's limits:**

- MDBList allows 1,000 requests a day on its free tier. Each account's daily allowance is told in MDBList's answers, and a limit over 5 minutes applies too.
- On a `429`, Polyfin asks nothing more for as long as `Retry-After` says.
- After `Daily API limit exceeded!`, it asks nothing until MDBList's daily reset.
- **System › Health** shows the requests left today.

## TMDB key

Many features need a TMDB key, which the administrator supplies. Polyfin bundles no key. Without one, TMDB is never asked, and people, casts and images are what the addons give.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **TMDB API key** | **Settings › Content › Images** | Empty | The server's TMDB API key, or a read access token, from a TMDB account. TMDB checks it when it is saved. |

To add it:

1. Create a free account on the TMDB website, and copy its API key or read access token.
2. Paste it in **TMDB API key**, in **Settings › Content › Images**, and save.

What happens when you save:

- **TMDB accepts it:** the field shows that a key is saved. Administrators can reveal the saved key; other users cannot.
- **TMDB refuses it, or it is malformed:** the admin app says "TMDB did not accept this key", and nothing is saved.
- **TMDB cannot be reached:** the admin app says so, and nothing is saved. Try again later.
- **Removing it:** empty the field and save.

The key is kept with the server's other secrets (see [Configuration](configuration.md)).

### What a key brings

| Feature | With a key | Without a key |
|---|---|---|
| People's photos and biographies | Looked up on TMDB (see [Photos and biographies from TMDB](#photos-and-biographies-from-tmdb)) | Only what the addons give |
| A title's cast and crew | TMDB's credits, in billing order, with photos (see [Cast and crew from TMDB](#cast-and-crew-from-tmdb)) | What the addon gives |
| Person pages | Every library title they are credited in, and the titles TMDB knows them best for (see [Titles a person is known for](#titles-a-person-is-known-for)) | Only titles people searches find and titles Polyfin knows them in |
| Studios | TMDB's studios and logos on titles, and studio pages with TMDB's most popular titles of each (see [Studios and taglines from TMDB](#studios-and-taglines-from-tmdb)) | Only studios an administrator set; studio pages list catalog titles and titles whose details were kept |
| Posters | TMDB's, for movies and series whose addon gives none or a dead one (see [Posters from TMDB](artwork-and-editing.md#posters-from-tmdb)) | None for such titles |
| **Edit images** | TMDB's images too (see [Edit images](artwork-and-editing.md#edit-images)) | Only the addons' images |
| Unmatched local files | Matched through TMDB's search when every metadata addon's fails (see [Through TMDB, when every addon fails](unmatched-files.md#through-tmdb-when-every-addon-fails)) | Stay unmatched |
| Smart list rules | **Studio or network** and **Home release** look titles up on TMDB (see [Smart lists](smart-lists-and-search.md#smart-lists)) | Studio rules see only those an administrator set; no movie has a home release |
| **Wait for the digital release** | Hides movies until TMDB says they are out (see [Movies waiting for their digital release](smart-lists-and-search.md#movies-waiting-for-their-digital-release)) | No movie waits; the switch is greyed out |
| Episode orders | TMDB's episode groups (see [Episode orders](artwork-and-editing.md#episode-orders)) | Every series shows as aired |
| **Identify** | Turns TMDB and TVDB identifiers into IMDb ones (see [Identify](artwork-and-editing.md#identify)) | TMDB and TVDB identifiers are not turned into IMDb ones |
| TMDB identifiers from `moviedb_id` | Checked against TMDB in the background; a wrong one is corrected or dropped (see [Where the other identifiers come from](addons.md#where-the-other-identifiers-come-from)) | Used as the addon gives them |
| Silo's apps | Box sets, from the TMDB collections of movies (see [Box sets](silo-apps.md#box-sets)) | No box sets |

### How Health explains it

**System › Health** has a **People** section (see [Artwork and people](health.md#artwork-and-people)):

- **No key:** it says that people and casts show only what the addons give. It lists what a key from a free TMDB account brings, with a link to add the key in **Settings › Content**.
- **A saved key:** it shows the key as saved, and how the lookups stand: **Found on TMDB**, **Not found**, **Lookups failing** and **Lookups waiting**. It also shows **TMDB's last rate limit**.

### Requests to TMDB

- At most 4 requests go to TMDB at a time, its images included.
- TMDB's answers are kept in memory for 24 hours.
- Polyfin relays TMDB's images, so apps never reach TMDB.

This product uses the TMDB API but is not endorsed or certified by TMDB.

**Compared with Jellyfin**

- Polyfin asks the administrator for a key. Jellyfin's TMDB plugin ships its own key, used when the administrator saves none ([TmdbUtils.cs, line 35](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs#L35), [TmdbClientManager.cs, lines 50 to 51](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs#L50-L51)). So Jellyfin's people, casts and images come from TMDB out of the box.
- Without a key, Polyfin lacks people's photos and biographies, casts from TMDB, person pages' library titles, Silo's box sets and TMDB's images in **Edit images**.
