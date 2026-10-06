# Skip segments

This page covers the buttons that let viewers skip intros, recaps, credits and previews: where Polyfin gets them, and how to set up the databases they come from.

## How skip buttons work

**Skip intro and credits buttons**, under **Settings › Content**, is on by default. It gives apps the intros, recaps and credits to skip.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Skip intro and credits buttons** | **Settings › Content** | On | Gives apps the segments to skip. Off, apps get none and no segment database is asked. |

**Compared with Jellyfin:**

- Turned off, the server behaves as a Jellyfin server without a segment provider.

**For app developers:**

- Turned off, `/MediaSegments/{id}` lists none.
- Turned on, the media sources of movies and episodes say `HasSegments: true` while a database is asked, or when the version's own chapters name an intro or credits (see [Segments of each version](#segments-of-each-version)). jellyfin-web asks for segments only then, by the identifier of the version it plays, which `/MediaSegments/{id}` takes as well as the title's. The title's identifier stands for its first version.

## The three databases

The skip buttons come from three community databases: TheIntroDB, IntroDB and PublicMetaDB. Polyfin asks them about a title the first time an app asks for its segments.

For each kind of segment (intro, recap, credits, preview), the first database that has one gives it. The others fill the gaps.

- PublicMetaDB also brings recap and preview markers, on episodes, along with intros and credits.
- PublicMetaDB is asked only with an API key; see [PublicMetaDB key](#publicmetadb-key).
- TheIntroDB answers without a key; see [TheIntroDB key](#theintrodb-key).

## Segments of each version

The databases time a title once, but the versions addons offer may be cut differently. Once Polyfin has analyzed a version, on its first play, it fits the segments to that version:

- A file's own chapters win, for that file. When a version's chapters name an intro or credits, they give that version's intro or credits in place of the databases'. Other versions keep the databases'.
- Segments never pass the end. A segment ends at the latest where the version ends. One starting at or after its end is left out.
- Credits that run to the end stop 1 second early, so that the web player (jellyfin-web) offers its skip button. Skipping then lands a second before the end, and the next episode starts, or the movie ends.
- When the title's listed runtime is shorter than the file, such credits end exactly at the file's end instead. The web player then shows its "Up Next" card rather than a skip button.
- The web player shows neither while its controls are on screen when the credits start: it offers a segment only once, as playback enters it.

A version not analyzed yet gets the databases' segments as they are.

**Chapter names:** names count in any letter case, as whole words.

- An intro: intro, introduction, opening, OP, or "générique de début". "Opening Credits" is an intro.
- Credits: credits (end or closing credits), ending, ED, outro, or "générique de fin".

OP and ED count only as the whole name, maybe followed by a number: "OP", "op 1", "ED2" and "ED - 2" count, while "Ed Wood" and "OP Center" do not. Chapters with other names, such as "Chapter 2", give no segment. Chapters give segments even with no database asked.

**Why credits stop early:** jellyfin-web shows no skip button for credits that reach the runtime the title is listed with, when something follows in its queue. Its video page shows its "Up Next" card instead, but only for credits that reach the end of the version it plays. Credits count as running to the end when they end within 2 seconds of it. They stop early only when the listed runtime is at least the version's length, and when they still last 3 seconds, as jellyfin-web ignores shorter segments.

## Source order

By default the databases are preferred in the order TheIntroDB, IntroDB, PublicMetaDB, all on. At the first start, `POLYFIN_SEGMENTS` gives the order and which are on; see [Configuration](configuration.md).

**Skip marker sources**, under **Settings › Content** below the skip buttons, lists the three databases with arrows to move them and a switch to turn each on or off. PublicMetaDB, on without a key, is marked as needing one.

The order and the switches apply at once when saved, to the answers already kept too. A database turned off is never asked, wherever it is in the list.

## PublicMetaDB key

PublicMetaDB is asked only with an API key. Create one on PublicMetaDB under **Settings → API**, then paste it in **PublicMetaDB key** under **Settings › Content**.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **PublicMetaDB key** | **Settings › Content** | Empty | API key PublicMetaDB is asked with. Without one, PublicMetaDB is not asked. |

- Polyfin checks the key with PublicMetaDB before saving it. It refuses a key PublicMetaDB refuses, or cannot be asked about.
- The key is never logged, and never sent back with the settings. It shows as dots; administrators can show it again with its eye (see [Showing a saved key](administration.md#showing-a-saved-key)). With `POLYFIN_SECRET_KEY` set, it is stored encrypted (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
- An empty field removes the key.
- Saving, changing or removing the key applies at once, without a restart.
- Titles asked about before a key was saved are asked again once one is.
- Should PublicMetaDB refuse the key later, Polyfin logs it once and asks no more until another key is saved.

### How PublicMetaDB matches titles

PublicMetaDB knows titles by their TMDB identifier only: a movie's, or a series' with the season and episode numbers. It keeps one record per contributor and release. Polyfin uses the record of a streaming release giving the most segments, the latest first.

## TheIntroDB key

**TheIntroDB key (optional)**, next to the PublicMetaDB key, is a TheIntroDB API key.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **TheIntroDB key (optional)** | **Settings › Content** | Empty | Raises TheIntroDB's daily limit and includes your own submissions. |

TheIntroDB answers without a key. With one:

- its daily limit goes from 500 requests per address to 1,000 per account;
- its answers include your own submissions, even pending ones, weighted more.

### How the key is checked

TheIntroDB's answers ignore a key they do not know. So Polyfin checks the key before saving it, by sending TheIntroDB an empty submission with it, which submits nothing.

- The key is saved when TheIntroDB only refuses the empty submission.
- It is refused when TheIntroDB refuses the key.
- It is not saved either when TheIntroDB cannot be asked, or answers otherwise.

The key is kept secret, shown and stored like PublicMetaDB's, and applied at once. Should TheIntroDB refuse it later, Polyfin logs it once and keeps asking TheIntroDB without it until another key is saved.

## Caching

Polyfin keeps the databases' answers:

- 30 days when they found segments;
- a day when they found none;
- an hour after a failure.

A database that answers 429 with a `Retry-After` is asked about no title until then.
