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
- Turned on, with a database asked, the media sources of movies and episodes say `HasSegments: true`. jellyfin-web asks for segments only then, by the identifier of the version it plays, which `/MediaSegments/{id}` takes as well as the title's.

## The three databases

The skip buttons come from three community databases: TheIntroDB, IntroDB and PublicMetaDB. Polyfin asks them about a title the first time an app asks for its segments.

For each kind of segment (intro, recap, credits, preview), the first database that has one gives it. The others fill the gaps.

- PublicMetaDB also brings recap and preview markers, on episodes, along with intros and credits.
- PublicMetaDB is asked only with an API key; see [PublicMetaDB key](#publicmetadb-key).
- TheIntroDB answers without a key; see [TheIntroDB key](#theintrodb-key).

## Source order

By default the databases are preferred in the order TheIntroDB, IntroDB, PublicMetaDB, unless `POLYFIN_SEGMENTS` gives another. `POLYFIN_SEGMENTS` also chooses which databases are used at all; see [Configuration](configuration.md).

**Order of the skip marker sources**, under **Settings › Content** below the skip buttons, lists the three databases with arrows to move them. Each is marked as one of:

- on;
- needing a key (PublicMetaDB without one);
- turned off by `POLYFIN_SEGMENTS`.

The order saved there replaces the one `POLYFIN_SEGMENTS` gives, at once, and for the answers already kept too. It does not replace the choice of databases: a database `POLYFIN_SEGMENTS` leaves out is never asked, wherever it is in the list.

**Reset to the default order** follows `POLYFIN_SEGMENTS` again. So does saving the order `POLYFIN_SEGMENTS` gives.

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
