# Naming and matching files

This page is for administrators of [local folders](local-folders.md). It covers how to name files, how Polyfin matches them to titles, and the artwork beside files.

Files Polyfin could not match, Identify, failed searches and plain videos are on [Unmatched files](unmatched-files.md). How a show's episode numbers map to the catalog is on [Episode numbering](episode-numbering.md). Discs are on [Discs](discs.md).

## Naming files

**Movies:** one file per movie, alone or in a folder of its own. A movie may also be a Blu-ray disc or a DVD copied as it is, or a disc image (see [Discs](discs.md)).

```
/media/movies/Night of the Living Dead (1968).mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/The.General.1926.1080p.BluRay.mkv
```

**Shows:** a folder per show, with the season and episode in each file's name. A show may also be on discs copied as they are (see [Shows on discs](discs.md#shows-on-discs)).

```
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.mkv
/media/shows/The Lone Ranger (1949)/The.Lone.Ranger.S02E10.720p.mkv
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E03-E04.mkv
```

Polyfin reads these episode names:

- `S01E02`, `s01e02` and `1x02`;
- in a season folder, `E02` or `Episode 2`;
- an absolute number alone, as `Show - 41` (see [Absolute episode numbers](episode-numbering.md#absolute-episode-numbers)).

How these numbers map to the catalog's episodes is on [Episode numbering](episode-numbering.md).

**Several episodes in one file:** `S01E01-E02`, `S01E01E02` or `S01E01-02` plays for each episode. Polyfin reads ranges as Jellyfin does:

- The episode ending a range has at most 3 digits, followed by no other digit, nor by a resolution's `p` or `i`.
- So a year or a resolution glued to the episode ends no range. `Show.S01E01-2022.mkv` and `Show.S09E14-1080p.mkv` hold one episode each.
- `S01E99-E101` holds three.
- A range ending lower than it starts is no range either.
- Each scan reads names again. A file an older version read otherwise, such as `S01E01-2022` read as episodes 1 to 2022, is read anew at the first scan, at startup, without its title being searched again.

Jellyfin's rule: [`EpisodePathParser.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Naming/TV/EpisodePathParser.cs#L151-L167).

**Identifiers in names:** an identifier in a file's or folder's name settles its title: `{imdb-tt0063350}`, `[imdbid-tt0063350]`, `{tmdb-10331}` or `[tmdbid-10331]`.

**Left out:**

- extras, local trailers and theme media named as Jellyfin names them, which are no titles: they show under their titles (see [Extras and theme media](#extras-and-theme-media));
- hidden folders, and folders named `sample`;
- a Blu-ray disc's streams and playlists, which its titles play, and the files of a DVD.

### Video files

A scan reads videos by their extension, in any letter case:

| Format | Extensions |
|---|---|
| Matroska, WebM | `.mkv`, `.webm` |
| MP4, QuickTime | `.mp4`, `.m4v`, `.mov` |
| MPEG-TS | `.ts`, `.m2ts`, `.mts`, and `.m2t`, `.tp` and `.trp`, as TV recordings are often named |
| MPEG program stream | `.mpg`, `.mpeg`, `.vob` |
| AVI | `.avi` |
| Windows Media | `.wmv` |
| Flash Video | `.flv` |
| Ogg | `.ogv` |

The `.VOB` files of a `VIDEO_TS` folder belong to its DVD, never videos of their own (see [DVD folders](discs.md#dvd-folders)).

Each file plays as it is on an app that takes its container and codecs. Otherwise:

| Format | Over HLS |
|---|---|
| Matroska, WebM, MP4, QuickTime | Repackaged, cut on their keyframe index. Video copied when the app takes its codec, else converted. |
| MPEG-TS | Repackaged as Matroska is, once Polyfin has read their keyframes (below). A seek lands where asked. |
| MPEG program stream, AVI | Video always converted, cut every few seconds. A seek lands where asked. A user who may not have video converted is offered them only as they are. |
| Windows Media, Flash Video, Ogg | Never cut into HLS segments. They play only on an app that takes them as they are. |

**MPEG-TS keyframes:** MPEG-TS files have no index, so Polyfin reads their keyframes once from the whole file.

- It reads them in the background when the title's details open, or at the first play over HLS.
- That first play waits 5 seconds at most for them. Otherwise it plays converted while the read goes on.
- A file whose keyframes cannot be read plays as an MPEG program stream does (see [HLS segments](transcoding.md#hls-segments)).

**In apps:** the web player plays none of the MPEG-TS, MPEG program stream or AVI files as they are. The Android TV app and Swiftfin play MPEG-TS as it is. The Android TV app plays MPEG program streams and AVI files over HLS.

### Extras and theme media

A movie in a folder of its own, a show's folder and a season's folder may hold extras beside the titles, read as Jellyfin reads them:

```
/media/movies/Nosferatu (1922)/Nosferatu (1922).mkv
/media/movies/Nosferatu (1922)/Extras/Making of.mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922)-featurette.mkv
/media/movies/Nosferatu (1922)/trailer.mp4
/media/movies/Nosferatu (1922)/backdrops/intro.mkv
/media/shows/The Lone Ranger (1949)/theme.mp3
/media/shows/The Lone Ranger (1949)/Season 01/Deleted Scenes/Ambush.mkv
```

| Kind | Read from |
|---|---|
| Extras, by folder | Videos in folders named `Behind The Scenes`, `Deleted Scenes`, `Interviews`, `Scenes`, `Samples`, `Shorts`, `Featurettes`, `Clips`, `Other`, `Extras` or `Trailers`, in any letter case. `Extras` and `Other` hold extras of no kind in particular. |
| Extras, by name | Videos whose names end in `-trailer`, `.trailer`, `_trailer`, ` trailer`, `-sample`, `.sample`, `_sample`, ` sample`, `-scene`, `-clip`, `-interview`, `-behindthescenes`, `-deleted`, `-deletedscene`, `-featurette`, `-short`, `-other` or `-extra`, and videos named `trailer` or `sample` |
| Theme songs | Audio files named `theme`, such as `theme.mp3`, and those in a `theme-music` folder |
| Theme videos | Videos in a `backdrops` folder |

**What an extra is:**

- It is never matched, listed nor offered as a version.
- Its name is its file's, without its extension or suffix. Else its NFO file's `<title>`. Else its file's as written. `Nosferatu (1922)-featurette.mkv` is named `Nosferatu (1922)`, and `trailer.mp4` takes the `<title>` of `trailer.nfo` beside it.
- A link made by hand wins over the name. A file linked to a title (see [Unmatched files](unmatched-files.md)) stays that title, or a version of it, whatever its name or folder. Removing the link makes it an extra again, at once.

**Whose extra it is:**

- A movie's extras belong to the movies directly in their folder, versions included.
- In a folder shared by several movies, an extra named with a suffix belongs to the movie its name begins with, such as `Nosferatu (1922)-trailer.mkv` beside `Nosferatu (1922).mkv`. The extras folders belong to none.
- An extra named with a suffix whose movie is not beside it is listed in the unmatched files as "An extra without its title beside it", so that it can be linked. Linked, it is a title.
- A disc folder is its movie's own.
- A show's folder holds the show's extras. A season's folder, such as `Season 01` or `Specials`, holds the season's.
- Only the files directly in an extras folder are read. Its other files, discs and audio files among them, are left out, as is an audio file other than a theme song.

**In apps:** extras show as in Jellyfin. The web player:

- lists a title's extras under its Special Features;
- plays its local trailer from its Trailer button, while the addons' trailers still show;
- plays its theme songs and videos in the background of its page, and of its seasons' and episodes' pages, for users who turned theme songs or videos on in its display settings.

An extra shows its title's backdrop, and its title's poster to apps that ask for its Primary image. It plays as a local file does (see [Versions](local-folders.md#versions)): direct play, repackaging or conversion, with the subtitle files beside it, named after it. It has its own watch state and resume point, apart from its title's. A user sees it while they see its title and its folder is one of their sources, as for the title's versions.

**The setting:** **Read extras and theme media** (fr « Lire les bonus et les thèmes »), on a movies or shows folder's page, is on by default. A folder's page shows how many extras its last scan found.

- Off, the next scan leaves extras out.
- Files with a suffix, and those in `Clips` and `backdrops` folders, are then read as titles.
- The files of the other extras folders, samples and audio files are then left out.
- Home videos folders read no extras: their extras folders are left out.

### NFO files

Folders organized by Jellyfin or a media manager often hold NFO files, which give each title's identifiers. Polyfin reads them from:

- a movie's own NFO file, the video's name with `.nfo`, such as `Nosferatu (1922) - 1080p.nfo`;
- else `movie.nfo` in the movie's folder, when that folder is the movie's own. Its videos are all read as one title, such as the versions of one movie;
- a Blu-ray disc's `movie.nfo`, in its disc folder;
- a show's `tvshow.nfo`, in its folder.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/Nosferatu (1922)/movie.nfo
/media/shows/The Lone Ranger (1949)/tvshow.nfo
```

In XML NFO files, Polyfin reads:

1. `<uniqueid type="imdb">` and `<uniqueid type="tmdb">`;
2. else the older `<imdbid>` and `<tmdbid>`, or the `<imdb_id>` Jellyfin writes in a show's `tvshow.nfo`, directly in the root element.

An NFO file that is not XML may hold the address of the title's IMDb page instead. File names are compared in any letter case.

Only identifiers are read. Titles, plots and ratings still come from the metadata addons, and artwork from them or the images beside the files (see [Artwork](#artwork)). Episodes' NFO files are not read.

### Subtitle files

Subtitle files beside a video are offered as subtitles of its version, as Jellyfin reads them. A file counts when it is in the video's folder and its name begins with the video's, followed by a dot:

- SubRip (`.srt`), ASS (`.ass`), SSA (`.ssa`) and WebVTT (`.vtt`) files;
- PGS picture subtitles (`.sup`);
- VobSub picture subtitles: an `.idx` file with the `.sub` file of the same name.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.mkv
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.fr.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.en.forced.srt
/media/movies/Nosferatu (1922)/Nosferatu (1922) - 1080p.en.sdh.ass
/media/shows/The Lone Ranger (1949)/Season 01/The Lone Ranger - S01E02.fre.srt
```

The words after the video's name, separated by dots, are read in any order and letter case:

| Word | Meaning |
|---|---|
| A language: an ISO 639-1 code (`fr`), an ISO 639-2 code (`fre` or `fra`), with a region (`pt-BR`), or a name in English or French (`French`, `Français`) | The subtitles' language |
| `forced` or `foreign` | Forced subtitles, which translate only what is not in the audio's language |
| `sdh`, `hi` or `cc` | Subtitles for the deaf and hard of hearing |
| `default` | The subtitles shown by default |
| Any other word | The subtitles' title, such as `Movie (2001).en.Commentary.srt` |

- **Similar names:** when videos of one folder begin alike, such as `Movie (2001).mkv` and `Movie (2001) - 1080p.mkv`, a subtitle file belongs to the longest name it begins with.
- **Blu-ray discs:** subtitle files sit in the disc folder, beside `BDMV`, named after the disc folder: `Nosferatu (1922)/Nosferatu (1922).fr.srt`.
- **VobSub pairs:** they are named by their `.idx` file. An `.idx` file without its `.sub` file is left out, and a `.sub` file alone is not read. When the pair's name tells no language, the language of the first track its `.idx` file lists is used, and only that track is offered.
- **Picture subtitles into text:** with **Turn picture subtitles into text** on, PGS files and VobSub pairs are also read into text copies. These are read again when the file changes (see [Picture subtitles read into text](subtitles.md#picture-subtitles-read-into-text)).

How subtitle files reach apps: [Subtitles](subtitles.md#subtitle-files-beside-local-files).

**Compared with Jellyfin**

- Polyfin reads only the identifiers of NFO files for movies and shows: neither names, plots nor artwork. For home videos, it reads the title, plot and date. Both read the identifiers of NFO files.
- Polyfin reads fewer video extensions: those listed in [Video files](#video-files), `.trp` among them, which Jellyfin does not read. Jellyfin reads some fifty ([`NamingOptions.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Naming/Common/NamingOptions.cs#L24-L79)). Both read MPEG-TS files (`.ts`, `.m2ts`, `.mts`, `.m2t` and `.tp`) and MPEG program streams, `.vob` among them.
- Polyfin converts the video of an MPEG program stream in a browser. Jellyfin copies it into HLS. Both copy an MPEG-TS file's video, Polyfin once it has read its keyframes (see [HLS segments](transcoding.md#hls-segments)).
- Polyfin reads only the subtitle files in the video's own folder. Both offer the subtitle files beside a video, named after it, text and pictures (`.sup`, and `.idx` with `.sub`).
- Polyfin reads extras only beside movies in folders of their own, shows and seasons, not beside episodes. It reads only the files directly in an extras folder, and names an extra without its suffix. Both read extras, local trailers and theme media with the same names.

## Matching

Each file, disc, or show folder is matched to a title, by the first of these that applies:

1. a link made by hand (see [Unmatched files](unmatched-files.md));
2. an identifier written in its name. An IMDb identifier is used as it is. A TMDB identifier goes through the metadata addons, which give its IMDb identifier;
3. an identifier its NFO file gives (see [NFO files](#nfo-files)), read the same way, the IMDb identifier when it gives both;
4. its name and year, searched in the server's metadata addons with the searches the metadata editor uses. The match is kept only when the addons find exactly one title of that name and kind, and of that year when the name gives one. When they find none, a looser rule may leave exactly one (see [Looser matches](#looser-matches)).

The title is kept under its IMDb identifier, which the addons answer streams for, with its TMDB identifier when known.

A folder's figures tell how its files were matched: by an identifier in the name, by an NFO file, by a search, by a search under each looser rule, or by hand.

### Looser matches

When the addons find no title of the exact name and year a file's name gives, two looser rules are tried. The match is kept only when exactly one title is left.

1. **Either rule:** a title of the same name, of the year before or after, such as `Harbor Lights (2011).mkv` for a title the addon dates 2012. Or a title of the same year, named alike once numerals are read. Both count together: a title of each leaves two, and none is kept.
2. **Both rules:** else a title named alike once numerals are read, of the year before or after.

"Named alike once numerals are read" means:

- Roman and Arabic numerals read alike, in the file's name and the titles found: `Harbor Lights 2` finds `Harbor Lights II`.
- Numbers written in words after "Part" read alike too: `Tidewater Part 1` finds `Tidewater: Part One`.

**An exact title always wins:**

- A looser rule never chooses among titles when one matches exactly.
- Several titles of the exact name and year leave the file unmatched, as **Several titles have this name**.
- A name without a year is matched by its numerals only.
- While a metadata addon's search fails, a looser match waits, as the file would wait when nothing is found (see [Failed searches](unmatched-files.md#failed-searches)). The addon that failed may know the title of the exact name and year.

**Numerals are read with care:** so that words are not taken for numbers:

- Only Roman numerals written with I, V and X are read, from I to XXXIX. Words such as "Mix", "Civil" or "Liv" stay words.
- A title of a single word is never read as a number: "I", "X" or "XXX" is a title, not 1, 10 or 30.
- A single letter, I, V or X, is a numeral only as the title's last word or after "Part". "I, Robot" and "V for Vendetta" keep their words, while "Harbor Lights V" is "Harbor Lights 5".
- Numbers in words, from one to twenty, are read only after "Part": "One Day" stays as it is.

**A second search:** many addons search names as they are written. Asked for `Harbor Lights 2`, they do not answer `Harbor Lights II`. So when a search finds no title of the name, not even one named alike, and the name holds a numeral, Polyfin searches once more with the numerals written the other way:

| Written as | Searched again as |
|---|---|
| Arabic numerals from 1 to 39 | Roman numerals, or words after "Part" up to twenty |
| Roman numerals, or words after "Part" | Figures |

So `Harbor Lights 2` is searched again as `Harbor Lights II`, `Gull Point II` as `Gull Point 2`, and `Tidewater Part 1` as `Tidewater Part One`. What that search finds is matched by the same rules, and flagged the same way.

There is no second search:

- for a name found at once;
- for a name without such a numeral;
- for a unit unlinked from a looser match;
- while the first search fails.

**Cost:** at most one more search per name.

- For a large folder, that is one more search per distinct name with a numeral the first search does not find.
- Each reads the first page of each search catalog, as the first does, at the same pace (4 at once), with the same waits for addons that ask to slow down (see [Failed searches](unmatched-files.md#failed-searches)).
- Each name is searched once per scan, both forms included.
- Unmatched files are searched again at each scan. Such a file costs two searches per scan until it is matched, renamed or linked.
- Polyfin answers a search whose results it still keeps in memory without asking the addon again.

**Checking them:** looser matches apply without anyone acting, but they are flagged.

- The folder's figures count them apart, by rule.
- The folder's page lists them under **Matched by a looser rule**. Each shows its rule (**A title of a year one apart**, **Numerals read alike**, or both), the name its file was read as, and the title it was matched to.
- Unlinking one, with the broken link beside it, removes the match for good. The file, show folder or disc is then matched by the exact rule only, through every scan. When that finds nothing, it is left unmatched with its reason: link it by hand then.
- A link made later wins, as always. Removing that link leaves it to the exact rule again.

### What a search needs

Matching files by name needs a metadata addon among the server's addons, under **Content › Sources**, not a user's own under **My sources**. The addon must:

- describe titles of the folder's kind, movies or shows. Its manifest offers the `meta` resource for them, as metadata addons do. Addons that only list streams do not;
- have a search catalog of that kind: a catalog that takes the `search` property and requires no other, the catalogs Stremio apps search.

How Polyfin searches:

- It sends each file's title as its name gives it, without the year, such as `The Matrix` for `The Matrix (1999).mkv`.
- It reads the first page of each such catalog, as Stremio apps do, then compares the year with the titles found.
- A name with a numeral this search does not find is searched once more (see [Looser matches](#looser-matches)).
- Catalogs that search the people credited in titles are not searched, nor are Polyfin's own sources (IPTV sources, local folders).
- An addon given a language (see [Languages](addons.md#languages)) is searched only when no addon without one has such a catalog.
- A scan searches up to 4 titles at once, each name once.

**Without such an addon:** only identifiers in names and NFO files, and links, match files.

- The other files are listed as **No addon of the server describes and searches titles of this kind**, and the folder's page says how many.
- **System › Health** warns, pointing to **Content › Sources**, until a scan matches them or they are gone.
- A user's own metadata addon, under **My sources**, does not count.
- Once such an addon is added, scan the folder.

**Compared with Jellyfin**

- Polyfin keeps a title of the very year whenever there is one. It keeps a title a year off only when it is the only one left, and flags it. Jellyfin 12.4 scores a year one off as high as the very year, and keeps the provider's first title of the best score, so a better known title of the year after can win ([`TmdbUtils.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs#L246-L255), `ScoreYear`, and lines 188-227, `FindBestMatch`).
- Polyfin never takes a title whose name only starts with the file's. Jellyfin does. Both accept a title dated a year apart from the name's, as release dates straddle a new year.
- Polyfin reads Roman and Arabic numerals, and numbers written in words after "Part", alike. It searches a name with a numeral it does not find once more, with the numeral written the other way. Jellyfin compares names as they are, keeping the provider's first title when none matches ([`TmdbUtils.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs#L180-L187)).

## Artwork

Folders organized by Jellyfin or a media manager often hold posters, backdrops and logos beside their files, chosen by hand. Polyfin reads them as Jellyfin does and shows them in place of the metadata addons' artwork of the same type.

- Formats: `.jpg`, `.jpeg`, `.png` or `.webp`, names compared in any letter case.
- Types a folder has no image of still come from the addons.

```
/media/movies/Nosferatu (1922)/Nosferatu (1922).mkv
/media/movies/Nosferatu (1922)/poster.jpg
/media/movies/Nosferatu (1922)/fanart.jpg
/media/movies/The General (1926).mkv
/media/movies/The General (1926)-poster.jpg
/media/shows/The Lone Ranger (1949)/poster.jpg
/media/shows/The Lone Ranger (1949)/season01-poster.jpg
/media/shows/The Lone Ranger (1949)/Season 02/folder.jpg
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger - S02E01.mkv
/media/shows/The Lone Ranger (1949)/Season 02/The Lone Ranger - S02E01-thumb.jpg
```

**A movie's images:** they are named after its file, `<file name>-<name>`, such as `The General (1926)-poster.jpg`. Its poster may also be `<file name>.jpg`.

- The names alone count too, such as `poster.jpg`, when the movie is alone in its folder: every video directly in it is the same movie, its versions included.
- They count too for a Blu-ray disc or a DVD, whose images sit in its disc folder.
- In a folder holding several movies, `poster.jpg` belongs to none of them.

Of each type, the first name found counts:

| Type | Names |
|---|---|
| Primary (poster) | `poster`, `folder`, `cover`, `default`, `movie` |
| Backdrop | `fanart`, `fanart-1` to `fanart-20`, `background`, `background-1`…, `art`, `art-1`…, `backdrop`, `backdrop1`… |
| Logo | `logo`, `clearlogo` |
| Thumb | `landscape`, `thumb` |
| Banner | `banner` |
| Art | `clearart` |
| Disc | `disc`, `cdart`, `discart` |

The names after the file's name come before the names alone, and `.jpg` before `.jpeg`, `.png` and `.webp`. A title has one backdrop: the first found.

**A show's images:** they are in its folder, with the same names, alone or after the folder's name. `show` replaces `movie`, and there is no Disc image.

**A season's images:** they are first in the show's folder, named after the season: `season01-poster`, `season01-fanart`, `season01-banner` and `season01-landscape`.

- `season1-…` comes first when both are there.
- For the specials: `specials-…` or `season-specials-…`.
- Then in its season folder, the folder directly in the show's holding its episodes, with the names of the table, such as `folder.jpg` or `poster.jpg`.
- A season without a poster of its own shows its show's, as with the addons' artwork.

**An episode's image:** it is beside its file, named after it, with the names Jellyfin reads for episodes, in this order:

1. `<file name>-thumb`, such as `The Lone Ranger - S02E01-thumb.jpg`;
2. `<file name>` alone, such as `The Lone Ranger - S02E01.jpg`;
3. the same two names in a `metadata` folder beside the file.

- It replaces the metadata addon's still, for each episode the file holds (both episodes of `S01E03-E04`).
- Other images beside the file, such as `<file name>-poster.jpg`, are not the episode's.
- The episodes of shows' discs and disc images (see [Shows on discs](discs.md#shows-on-discs)) have no file of their own: they keep the addon's still.

**A frame for episodes without an image:** an episode with neither the addon's still nor an image beside its file shows a frame of its file. Jellyfin's Screen Grabber takes one the same way ([`VideoImageProvider.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Providers/MediaInfo/VideoImageProvider.cs#L57-L117)).

- The frame is taken a tenth into the file, else at its first frame, tone mapped when it is HDR and FFmpeg can.
- It is made when an app first asks for the image, two at most at once. Each gets a minute to read the file and another to take the frame. Apps asking meanwhile wait for that one.
- It is then kept with the other artwork, and made again once the file changes. Listings do not wait for it.
- A frame that could not be made, as from a file FFmpeg cannot read, is not tried again for 10 minutes unless the file changes. The episode shows no image meanwhile.
- Episodes of shows matched or not, plain episodes among them (see [Plain videos](unmatched-files.md#plain-videos)), have one, whatever **Use its artwork** says.
- Those of shows' discs and disc images have none, as in Jellyfin.

**Whose images:** the images belong to the title the file, disc or show folder is matched to, a link made by hand included, from the moment the link is made. An episode's belong to the episodes its file holds. When a title's files are in several folders with images:

- each type comes from the folder listed first on the **Content › Sources** page;
- then from the first of the title's files or folders by path;
- a disabled folder gives none.

**Use its artwork,** on the form of a movies or shows folder, chooses whether its titles and episodes show its images. It is on by default. Turning it off brings back the addons' artwork and stills at once. Artwork an administrator uploads in a Jellyfin app wins over both.

**Reading:** images are read from the folder or the share when an app first asks for them. They are then kept with the other artwork, at the sizes apps ask for. A scan notices an image added, replaced or removed, an episode's too, by its size and modification time. Apps see the new one, since its tag changes.

**Compared with Jellyfin**

- Polyfin keeps one backdrop per title, and shows the addons' artwork of the types a folder has none of. Both read the artwork beside movies and shows, and the image beside an episode's file, with the same names.
- Polyfin reads no `extrafanart` folder, no `.gif`, `.tbn` or `.svg` image, and no images for the episodes of discs.
- Polyfin makes frames for episodes only, when an app first asks. Jellyfin also makes frames for movies, and makes them during scans. Both show a frame of the file for an episode without an image.

API details: [Local artwork in the Jellyfin API](jellyfin-api.md#local-artwork-in-the-jellyfin-api).
