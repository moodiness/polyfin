# Smart lists and search

This page is for administrators and users who gather titles by what they are. It covers smart lists, searches in plain words, and the movies Polyfin holds back until their digital release.

## Smart lists

A smart list gathers titles by what they are, from the catalogs it names, such as "French movies of the 1990s not watched yet".

| Lists of | Made in | Seen by |
|---|---|---|
| The server | **Content › Libraries**, with **Add a smart list** | Every user, unless their visible libraries leave out a list shown as a library |
| A user | **My sources** | That user only (see [Users' own addons](sign-in-and-security.md#users-own-addons)) |

A list draws from any catalog of movies or series of the server's addons, turned on as a library or not. A user's own list may also draw from their own addons.

### Rules

A list keeps the titles that match its rules:

| Rule | Matches |
|---|---|
| **Genre** | A title with this genre, in any letter case. |
| **Year** | A title that came out from one year to another. Leave either empty for no bound. |
| **Release date** | A title released in the last so many days, up to today, or in the next so many days, from today. A title whose catalog and description give only its year has no release date. |
| **Home release** | A title out at home in the last so many days (today included), in the next so many days, or either. See [Home release rule](#home-release-rule). |
| **IMDb rating** | A title whose IMDb rating is within the bounds given. |
| **IMDb votes** | A title whose IMDb rating counts a number of votes within the bounds given, as MDBList tells it (see [Ratings](collections-people-and-ratings.md#ratings)). Without the server's **MDBList API key for ratings**, no title has votes. |
| **Language** | A title whose description gives this language. |
| **Runtime** | A title whose runtime is within the bounds given, in minutes. |
| **Content rating** | A title rated at most this rating, such as PG-13, on the scale parental control uses (see [Parental control](user-limits.md#parental-control)). PG-13 matches PG-13, PG and G titles, and the ratings of other countries for the same ages. **is not** matches the titles rated above it. |
| **Actor or director** | A title that credits this person as an actor or a director, in any letter case. |
| **Studio or network** | A title made by this studio, or a series aired on this network, in any letter case. Names are as TMDB gives them, or as an administrator set a title's studios in its metadata. Without the server's **TMDB API key**, only those an administrator set count. |
| **Movie or series** | Movies, or series. |
| **Place in its catalog** | A title among the first so many of its catalog, in the catalog's order: by popularity for a popular catalog. |
| **Watch state** | A title the user opening the list played, did not play, started without finishing, or marked favorite. A series is played once every released episode is. |

How rules combine:

- **Negation:** each rule can be negated with **is not**.
- **All or any:** titles match **All of the rules** or **Any of the rules**. A group of rules inside the list matches all or any of its own.
- **No rules:** a list without rules keeps every title of its catalogs.
- **Exclusions:** these are rules too, in a group of their own. A title they match is left out, whatever its rules say. A list without exclusions leaves no title out.
- **Missing data:** a title that lacks what a rule looks at matches neither the rule nor its negation. A movie whose description gives no runtime is in neither a list of movies of at least 90 minutes, nor one of movies not of at least 90 minutes. Exclusions do not leave it out for that either.

#### Home release rule

- A movie's home release is its first digital or physical release. Otherwise it is 90 days after it came out in cinemas, as TMDB tells (see [Movies waiting for their digital release](#movies-waiting-for-their-digital-release)).
- A series' home release is its premiere date.
- Without the server's **TMDB API key**, no movie has one.
- A list with this rule, not negated, of upcoming titles or of either, is the only list that shows the movies waiting for their digital release.

### Sort, maximum and shape

**Sort** offers these orders:

| Choice | Notes |
|---|---|
| **As its catalogs list them, one of each in turn** | As an addon's collection lists its catalogs |
| **Premiere date, latest first** | |
| **Latest episode aired, latest first** | Movies come by their premiere date; see below |
| **Latest season premiere, latest first** | Movies come by their premiere date; see below |
| **IMDb rating, highest first** | Titles without one come last |
| **Name** | |
| **At random, drawn again each day** | |

- **Latest episode aired, latest first** orders series by their latest regular episode that has aired. **Latest season premiere, latest first** orders them by the first episode of their latest season that has aired. Both use the dates in the series' descriptions.
- For both, specials (season 0) and episodes to come do not count. A movie comes by its premiere date. A series whose description is not read yet comes last until it is. So does a series without a dated episode.
- **Maximum titles:** from 1 to 20,000, applied after the rules and the sort. Empty, the list has no maximum but its catalogs'.
- **Shown as:** a library or a collection. A list shown as a collection shows an image Polyfin draws from its first titles, as an addon's collection without a poster does (see [Generated images](artwork-and-editing.md#generated-images)). A list shown as a library shows none.

The list's page shows a preview of its titles as they stand now, before you save. It also says how many titles still wait for what their rules look at: their description, their votes or their studios.

### How a list is made

**Reading the catalogs:**

- Polyfin reads each catalog up to its maximum, from the catalog pages it keeps and reads again after **Refresh catalogs after (minutes)**.
- That maximum is the library's own, when the catalog is one of the user's libraries with a maximum. Otherwise it is **Titles read per movie and series catalog**.
- When the rules require a genre the catalog offers in its genre filter, Polyfin asks the catalog for that genre's titles, as for a library's genre. It does not read it whole. A place in that catalog is then a place among that genre's titles.
- A listing waits for the addons 3 seconds at most. Past that, it lists the titles of the pages already kept, and reads the others in the background.

**Applying the rules:**

- The rules are applied each time the list is listed.
- The rules on what the user did are applied for each user, so a movie leaves "not watched yet" once played.
- A title the user hid leaves their lists at once, and comes back as soon as they show it again. Other users still see it.

**Filling in what titles lack:**

- **Descriptions:** a catalog page usually gives a title's genres, year and IMDb rating. It sometimes gives its runtime, content rating, people and release date. The language, and a series' episodes, come only with the title's full description.
- Polyfin reads the missing descriptions in the background, a few at a time, as it looks up ratings for users under parental control. The server's lists ask the server's addons only. Once read, what a description does not give, the title does not have.
- Until then, a title whose rules need its description is left out, negated rules included. The list grows as descriptions come.
- **Votes:** a title's IMDb votes come with its MDBList ratings, looked up in the background in batches (see [Ratings](collections-people-and-ratings.md#ratings)). A title still waiting for MDBList is left out until it is answered.
- **Studios and networks:** they come from TMDB, by a title's TMDB identifier, else its IMDb one. Polyfin asks TMDB in the background for at most 50 titles at each listing, within the requests to TMDB it allows at once. It keeps the answers for every user, and asks again after 30 days. A title waiting for TMDB is left out until it is answered. A title TMDB does not know has no studio.
- **Home release:** a movie's comes from TMDB the same way, kept and asked again as for [the wait for the digital release](#movies-waiting-for-their-digital-release). A movie not known yet is left out until it is. One TMDB does not know, or knows no date of, has none. A movie to come shows the day it is out as its premiere date, and first in its overview.

### Limits

- A list sees only the titles its catalogs list within their maximum, and changes as they change.
- A list whose catalogs are long is slow to fill the first time. **Read every collection each day** reads the server's smart lists too (see [Catalog limits and refresh](addons-and-libraries.md#catalog-limits-and-refresh)).
- Parental control, blocked genres and allowed hours apply as everywhere. A user under parental control sees the server's lists only.
- A list keeps its own sort: apps cannot sort it otherwise.
- A library made of a list offers no genre to filter by.

API details: [Smart lists](jellyfin-api.md#smart-lists-for-apps).

## Searches in plain words

A search in plain words also finds a collection named after the search. Examples are "80s sci-fi under 2 hours", "highly rated korean series" or « comédies des années 80 de moins de 2 heures ». The collection lists the movies and series of your libraries that match what the search says, as a smart list of your own would (see [Smart lists](#smart-lists)). Any other search finds what it found before, and no collection.

Polyfin understands these words:

| Words | Mean |
|---|---|
| A decade: "80s", "1980s", "the eighties", « années 80 », „80er“, "los ochenta", "anni 80", "anos 80" | Titles that came out in it. Two digits from 20 to 90 are the 1920s to the 1990s. 00 and 10 are the 2000s and the 2010s: write "2020s" in full. |
| A year from 1900 to 2099 | Titles that came out that year. |
| A genre of your libraries | Titles of that genre, one the genre filters of your libraries' catalogs offer. Write it by its name, or by its common name in your language ("sci-fi", « horreur », „Komödien“), for the genres catalogs commonly name in English. "sci-fi" stands for every one of "Science Fiction", "Sci-Fi" and "Sci-Fi & Fantasy" your libraries have. |
| "under 2 hours", "under 90 minutes", "under an hour" | Titles shorter than that. |
| "short" | Titles shorter than 90 minutes. |
| "highly rated", "top rated" | Titles with an IMDb rating of 7.5 or more. |
| A language: "french", "korean" | Titles whose description gives that language. |
| "movies", "series", "shows" | Movies, or series. |
| "the", "of", "from", "and" | Nothing: they only link the others. |

- **Languages:** Polyfin reads searches in English, French, German, Spanish, Italian and Portuguese, without regard to letter case or accents. All the words of a search are in one language, apart from its numbers.
- **Every word counts:** every word must be understood.
- **Two things at least:** "Drama", "1917" or "short" alone stay searches of titles. So does "80s westerns" when none of your libraries has westerns.
- **Once each:** a search names each thing once, except genres: "comedy drama movies" are movies of both genres.
- **Privacy:** searches are read on the server, from word tables built into Polyfin. Nothing is sent to any other service.

### The collection

- **Its titles:** it draws from the catalogs of your libraries of movies and series, or of the type the search names. It lists their titles in their order, within their maximum.
- **Your limits:** it applies parental control, blocked genres and the titles you hid, as smart lists do.
- **Languages:** when the search names a language, a title whose description is not read yet waits for it, as in a smart list. The collection grows as descriptions come.
- **Where it shows:** it is not in the **Collections** view, and has no generated image.
- **How long it stays:** Polyfin keeps your last 20 searches in plain words for 24 hours after you last made them, until the server restarts. Searching the same words again finds the same collection.
- **Saving it:** in jellyfin-web, its "Add to favorites" heart saves it as your own smart list. The list is shown as a collection, named after the search. You then find it in the **Collections** view and under **My sources**.
- **Unsaving it:** removing the heart deletes that list, unless you changed its name or rules since.
- **Who cannot save:** a user who cannot have their own smart lists is refused. That is a user under parental control or blocking genres, or one whose own addons are turned off.

**Compared with Jellyfin**

- Polyfin reads a search's meaning; Jellyfin's search matches names only.
- The collection shows where jellyfin-web lists the collections a search finds, under "Collections" ([search.ts, lines 103 to 104 and 138](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/apps/legacy/features/search/utils/search.ts#L103-L138)).
- "Add to favorites" on this collection saves a smart list. On a Jellyfin collection, it marks it favorite.

API details: [Searches in plain words](jellyfin-api.md#searches-in-plain-words-for-apps).

## Movies waiting for their digital release

Addons' catalogs list movies still in cinemas, which stream addons have no version of yet. With **Wait for the digital release** on, a movie shows only once it is out at home.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Wait for the digital release** | **Settings › Content › Movie releases** | Off | Hides the movies of the addons' catalogs until they are out at home. It needs the **TMDB API key**. Off, every movie is listed and plays as its versions allow. |

A movie is out at home:

- on its first digital or physical release, in any country, as TMDB's release dates tell;
- else, when TMDB knows neither, 90 days after it came out in cinemas, limited or wide;
- else, without a cinema date, 90 days after it was first shown anywhere.

### Until it is out

- **Hidden:** the movie is left out of libraries, home rows, searches, similar titles and person pages, as a title the user hid is.
- **Smart lists:** those with a **Home release** rule of upcoming titles list it (see [Home release rule](#home-release-rule)). The day it is out is its premiere date, and comes first in its overview: "Out at home on 2026-11-20.".
- **Opening it:** it still opens by its identifier, from such a list or a link, with that day in its overview.
- **Playing it:** it has no version, and its addons are not asked for streams. It does not play: jellyfin-web says "Playback of this media is not allowed.".
- **Watchlist:** a movie of a user's Watchlist stays there. The Watchlist checks find no version of it before that day (see [Now available and Still waiting](tracking-imports.md#now-available-and-still-waiting)).

### Movies that never wait

- **Files already there:** only movies of the addons' catalogs wait. An IPTV source's and a local folder's movies are files already there.
- **Nothing to tell:** a movie TMDB does not know, one TMDB knows no date of, and one without an IMDb or TMDB identifier stay listed: nothing says they are not out.
- **Old movies:** a movie whose catalog or description says it came out more than a year ago, or in a year before last year, is out. TMDB is not asked about it.
- **No key:** without the server's **TMDB API key**, no movie waits.

### How Polyfin asks TMDB

- **Lookup:** by the movie's TMDB identifier, else through TMDB's lookup of the IMDb identifier, in the background.
- **Pace:** within the requests to TMDB Polyfin allows at once, 4. One request starts at most 100 lookups.
- **Listings:** a listing waits for the answers 3 seconds at most, as for ratings under parental control. It leaves out the movies still not known then. They show at a later listing, once known.
- **Kept:** the answers are kept in the database, for every user.

| The movie | Asked again |
|---|---|
| Not out yet | After 3 days, as its dates change |
| Not known to TMDB, or of no date | After 30 days |
| Out | Never |

Once its day has passed, a movie stays out, whatever TMDB says later. A movie listed is not hidden again.

**Compared with Jellyfin**

- Polyfin can hold back movies not out yet. Jellyfin lists the files of its folders, which are there to play, so it has no such wait. A movie waiting is not listed, as Jellyfin does not list what a user cannot access.
- A movie listed by a smart list of upcoming titles carries its future day as its premiere date, as Jellyfin's episodes not aired yet do.

API details: [Movies waiting for their digital release](jellyfin-api.md#movies-waiting-for-their-digital-release-for-apps).
