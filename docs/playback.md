# Playback

This page explains how Polyfin plays a title in Jellyfin apps: versions, chapters, preparing playback in advance, the settings that choose a version, watching together, and scrubbing thumbnails.

## How a title plays

Polyfin always handles sign-in, accounts, browsing, metadata, source selection and playback state. Apps never see the addons' stream URLs: they receive Polyfin's own, signed for the user.

A version reaches the app in one of three ways:

- **Direct play.** The app plays the version as it is. Polyfin sends it to the provider's URL, so the stream does not pass through the server.
- **Relay.** Polyfin reads the stream and passes it on when the app could not reach the source itself: a source that needs request headers, one on a local network address, or a redirect the app could not follow.
- **HLS.** FFmpeg reads the stream through Polyfin's cache and Polyfin serves it as HLS segments, remuxed or transcoded. See [Transcoding](transcoding.md).

Whether a version plays directly or is converted decides how much bandwidth the server uses.

**For app developers:**

- Direct play answers with a `302` redirect to the debrid or provider URL.

## Versions and analysis

A title's details list every stream the addons offer as a version (see [Title pages](#title-pages) for when). The first time a version is played, Polyfin analyzes it with ffprobe, which takes a few seconds, and keeps the result. It then tells the app whether the app can play the version as it is, as a Jellyfin server would.

- Versions keep the addons' order, addon after addon: the server's sources in the order of **Content › Sources**, folders included, then the user's own addons in theirs. The [version order](#version-order) settings may order them otherwise, by their source too, or hide some. Picking or playing one never moves it.
- When the app has not picked a version, a version that cannot be read, or does not play on the app, is skipped in favor of the next one. It is also left out of the versions Polyfin lists, which keep their order starting from the one chosen, for apps that let the user pick at that point.
- A version that would play over HLS but whose keyframe index cannot be read is skipped the same way, as nothing could stream it.
- The first versions are analyzed together, up to 3 at once, and taken in order. Once a later one is ready, Polyfin waits 3 more seconds for those before it, then plays the later one: a source that never answers delays playback by those 3 seconds rather than by the whole analysis time. The analyses still running go on in the background and are kept for the next play.
- The title's page plays the version the app sends. Most apps (the web player, Android TV, Swiftfin) send the title's own identifier, which names the first version: Polyfin then chooses among all the versions as when the app picks none. Only a version picked in the version menu, under its own identifier, is played alone.
- Apps can save the user's playback preferences (audio and subtitle languages, subtitle mode). These choose the default audio and subtitle tracks, with the server's [default languages](#default-audio-and-subtitle-languages) for a user who chose none. See [Subtitles](subtitles.md).
- Apps that keep titles for offline viewing can download the movies and episodes whose versions they were shown. A download is the version's file, under its file name, sent to the source or relayed as playback would be. With **Converted downloads** on, apps that ask for a lower quality, such as Streamyfin, get a smaller copy instead, made as they download it. See [Converted downloads](transcoding.md#converted-downloads).
- Active playback appears in Jellyfin apps' dashboards.
- Apps can control one another (play, pause, seek, messages) when the controlled app keeps Jellyfin's live connection (WebSocket) open.
- The skip intro, recap and credits buttons come from [TheIntroDB](https://theintrodb.org), [IntroDB](https://introdb.app) and [SkipDB](https://skipdb.tv), community databases, from the chapters of the version played, and, for the episodes of local folders, from the intro and the credits their season's episodes share in their audio. Polyfin sends the databases only titles' IDs, episode numbers and runtimes. See [Skip segments](skip-segments.md).
- With **Describe versions from RemuxDB** on, a title's page shows the tracks of its versions before they are played, from what [RemuxDB](https://remuxdb.1632022.xyz), a community database, found in the same files. Polyfin sends it only titles' IDs. See [Tracks from RemuxDB](#tracks-from-remuxdb).

**Compared with Jellyfin:**

- Jellyfin 12.4 lists first the version an app opens as an item. jellyfin-web's version menu does this once a version is picked, so the menu reorders itself there. Polyfin keeps the menu as it was.
- Jellyfin lists all versions in PlaybackInfo, including the ones that cannot be read or played.

**For app developers:**

- Unreadable or unplayable versions are left out of the versions `PlaybackInfo` lists only when the app has not picked a version.
- A `MediaSourceId` equal to the item's own identifier picks no version: `PlaybackInfo` tries the versions in order, as without one, and answers with the version chosen alone, under its own identifier unless it is the first. Any other `MediaSourceId` plays that version or nothing.
- Elsewhere, the item's own identifier as a media source (the `mediaSourceId` of `/Videos/{id}/stream`, subtitle and attachment URLs, playback reports) stands for the first version the item's details and `PlaybackInfo` list, after leaving out the versions that failed lately and those the [version order](#version-order) rules or the user's quality group leave out, and past the torrents that did not start lately, which a play picking no version passes over. A version that fails thus passes the identifier to the next one, wherever its addon is in the addons' order, and the next one's own identifier keeps naming it.
- `StartTimeTicks` in the query or the body tells where an HLS play starts, so that Polyfin reads ahead from there.

### Which streams become versions

Polyfin does nothing to an addon's streams but what this list says, in this order. By default, only steps 1 to 7 leave streams out, and the versions keep the addons' order.

1. **The addons asked:** the enabled addons the user browses that list streams (`stream` in their manifest) for the title's type and identifier, or for another identifier of the title they accept (see [Identifiers addons accept](addons-and-libraries.md#identifiers-addons-accept)): the server's sources in the order of **Content › Sources**, unless the user turned **Use the server's sources** off, then the user's own addons under **My sources**, in theirs (see [Personal addons](addons-and-libraries.md#personal-addons)). A user under parental control or blocking genres gets the server's sources only. A video whose title's description carries its own streams gets those first, and the addon that gave them is not asked for them (see [Streams in a title's details](addons-and-libraries.md#streams-in-a-title-s-details)).
2. **What is asked:** each addon is asked at the address it was installed with, its configuration included, for the title or the episode (`/stream/<type>/<id>.json`), by its own identifier or the other one the addon accepts. For example, an episode of a series an anime catalog lists as `kitsu:<id>:3` is asked of an addon that takes IMDb identifiers only as `tt…:2:3`, when the anime mapping places it there. Polyfin adds nothing to the request, and takes the streams in the order the addon gives them: what the addon's configuration filters, sorts or limits is what Polyfin gets. An addon that fails, or does not answer within 15 seconds, gives no new version; the versions Polyfin kept from its earlier answers for the title may still show (see [Title pages](#title-pages)).
3. **Nothing to play:** a stream with nothing to play is left out. A stream plays when it is a link over `http` or `https`, or a torrent's file (an `infoHash` of 40 hexadecimal digits). A web page (`externalUrl`), a YouTube video (`ytId`), or a notice the addon lists among its streams makes no version.
4. **Torrents:** they are left out unless **Play torrent streams** is on, under **Settings › Torrents**, and the user has **Can play torrent streams** (see [Turning torrents on](torrents.md#turning-torrents-on)). Both are off by default.
5. **The same file twice:** two streams that give the same file name and the same size, from one addon or two, as a torrent and a link or two torrents of the same release, are one version, the first in the order above. Without both its name and its size, a stream is known by its link, a torrent by its info hash and file index. A torrent left out by the previous step leaves its place to a link to the same file listed after it. The links to the same file that other addons list after it, up to 3, are kept as the version's hidden mirrors: they are listed nowhere, and carry a play on when the version's source fails (see [Carrying a play on](#carrying-a-play-on)). A torrent has no mirror and is none.
6. **Failed lately:** versions that failed are left out for 15 minutes: a source that could not be read or analyzed (see [Sources that fail](#sources-that-fail)). A torrent that did not start in time stays listed, in its place: a play that picks no version passes it over for those 15 minutes and tries the next ones, and a version the user picks in the version menu is tried again at once (see [When torrents do not start](torrents.md#when-torrents-do-not-start)). A torrent whose peers stopped sending, a route that was down, the limit of torrents at once, or a host asking to slow down leaves the version listed and tried as any other. A restart forgets these failures, and so does a change of a local folder's address or credentials, for its files. A failure is the file's: a torrent that did not start leaves out, for those 15 minutes, the link to the same file that users without torrents get instead, and a link that failed leaves out the torrent of its file.
7. **Another title:** a stream whose labels name another episode, a season without the episode, or another movie of the same name is left out, without a setting, as a hide rule leaves a version out: never every version of a title (see [Versions of another title](#versions-of-another-title)).
8. **Hide rules:** when set, **Hide versions larger than**, **Hide these video codecs**, **Hide versions whose labels contain** and **Hide low-quality releases**, under **Settings › Playback**, leave out what they name. All are off by default, and none ever hides every version of a title (see [Version order](#version-order)).
9. **Quality group:** the user's group, when set, leaves out the versions taller than it, while one fits. It is **Original (no limit)** by default (see [Quality groups](users.md#quality-groups)).
10. **The order:** it stays the addons' unless **Order versions by**, under **Settings › Playback**, has criteria. It has none by default. Versions the criteria tie on keep the addons' order.
11. **Plays without a pick:** when an app plays without picking a version, `PlaybackInfo` leaves out the versions it passed over: those that could not be read or did not play on the app, and the torrents that did not start lately (see above).

Polyfin keeps no maximum of versions, and looks at no seeders, source, size or language unless a setting above says so.

**Why a stream is missing:** apps do not say why. To find out:

- An addon that failed, timed out or answered only a notice logs `An addon could not list streams`, at Warn, with the addon and the error, and **System › Health › Addons** shows its last failure.
- A version a play could not analyze logs `A version could not be analyzed`, at Info, with the addon and why; a source that fails later in a play logs why too, never its address. A torrent that did not start logs `A torrent did not start in time`, and **System › Health** shows it under **Needs attention**.
- A stream left out as another title's logs `A version names another title`, at Debug, with the addon and the marker that names it (`S01E03`, `S02`, `1998`), once while Polyfin remembers the version.
- Streams with nothing to play, torrents left out, files listed twice, and the hide rules and quality group log nothing. Compare the addon's list with the steps above: turn the hide rules and the quality group off, and check the addon's configuration where it was installed.
- `GET /Polyfin/Items/{id}/Versions` tells how many versions are known and how many addons are still asked (see [Title pages](#title-pages)).

#### Versions of another title

Addons that gather releases from many places sometimes list, for an episode, a release of another episode, or, for a movie, another movie of the same name. Polyfin reads the stream's name, title and description as its addon gives them, and its file name, line by line, and leaves the stream out when a line contradicts the title:

- **An episode:** the seasons and episodes a line names after the series' own words, written `S01E02`, `S01E02E03`, `1x02`, `S01 E02`, `Season 1 Episode 2`, a range `S01E01-E04`, `S01E01-04` or `1x01-1x04`, a season `S01` or `Season 1`, or seasons `S01-S03`, `S01-03` or `Seasons 1-3`, in any letter case. When none of them holds the episode, the stream is another title's. A season pack or a range holding it is kept: the torrent's file is picked inside it (see [Reading and storage](torrents.md#reading-and-storage)). So, for S01E02, `Some.Show.S01E03.mkv`, `Some.Show.S02.COMPLETE` and `Some.Show.S01E05-E08` are left out, and `Some.Show.S01.1080p`, `Some.Show.S01-S03` and `Some.Show.S01E01-03` are kept. A pack whose file name names another episode, as in `Some.Show.S01.1080p` with the file `Some.Show.S01E06.mkv`, is left out too.
- **A movie:** a line naming the movie, as its addon names it, whose next word is a year more than one year from the movie's. So, for Some Movie (2008), `Some.Movie.1985.1080p` is left out, and `Some.Movie.2007`, `Some Movie (2009)` and `Some.Movie.Remastered.2020` are kept. The movie's own name may hold a year: for 1917 (2019), `1917.2019.1080p` is kept. A line that does not name the movie, such as a translated title, tells no year.

Names without these markers are kept: `Some Show Complete Series`, an addon's `4K` line, or an episode numbered from the series' start, as in `Some Show - 25`. In a line naming the series, only what follows its name is read, so that a series named `1x1 Club` is not taken for its first episode; in a line that does not name it, every word is. When every version of a title names another, they are all kept, as with the hide rules: the addon may number episodes otherwise than the title's description.

**Compared with Jellyfin:** Jellyfin 12.4 lists no addon's streams, so it has no such step: its versions are the files of a title it found in its folders. Polyfin's own folders are read the same way as addons' streams.

### Default audio and subtitle languages

Each user chooses an audio language, a subtitle language and a subtitle mode in their app. A user who chose no language gets the audio track the file flags as default, and the subtitles their mode gives in any language. An administrator can choose languages for them:

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Default audio language** | **Settings › Playback** | None | The audio language of the users who chose none in their app. |
| **Default subtitle language** | **Settings › Playback** | None | The subtitle language of the users who chose none in their app, shown as each user's own subtitle mode says. |

- They offer the languages a user's own preferences offer in jellyfin-web, from `/Localization/Cultures`.
- A user's own choice always wins: a default applies only where the user's language is empty, the audio and the subtitle language each on its own. It applies wherever Polyfin chooses the tracks, for every app and the web player alike: the default tracks of a title's details and of `PlaybackInfo`, and the **Preferred audio language** of the [version order](#version-order).
- The subtitle mode stays the user's (`SubtitleMode`): with `Always`, full subtitles in the default language, else forced ones; with `Smart`, subtitles in the default language when the audio is in another one; with `OnlyForced`, forced subtitles in the default language or of no stated language; `Default` and `None` do not look at the language.
- The audio track: for a user without a language of their own, the track in **Default audio language** comes first, before the track the file flags as default, even when the user plays the default track whatever its language (`PlayDefaultAudioTrack`, which every new user has on until they turn it off in their app): the administrator chose that language for them, while the app setting was never their choice. A file without a track in that language plays the track it flags as default, else its first. A user's own language keeps Jellyfin's rule: with `PlayDefaultAudioTrack` on, the flagged track comes first, then the track in their language ([MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L14-L29)).
- Apps keep showing the user's own preferences, with no language when they chose none, in `/Users/Me` and `/Users/{userId}`. An app saving its configuration back thus never makes the default the user's own choice, and a changed default reaches every user without a language at their next play.

**Compared with Jellyfin:**

- Jellyfin 12.4 has no server-wide default: it reads the user's own preferences alone ([MediaSourceManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L515), [L574-L578](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L574-L578)), and an empty one is no language at all ([L477-L482](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L477-L482)). Without a language, its audio is the track flagged default, else the first ([MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L14-L29)), and every subtitle counts as in the preferred language ([L154-L158](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L154-L158)). With both settings at None, as by default, Polyfin chooses as Jellyfin does; a default subtitle language makes it choose a user's subtitles as Jellyfin would for a user who chose that language.
- Polyfin's default audio language ranks above the track a file flags as default whatever `PlayDefaultAudioTrack` says, where a language a user chose in Jellyfin gives way to the flagged track while that setting is on.
- jellyfin-web 12.4 also offers "Original language" as an audio preference. Polyfin does not offer it as a default.

### Regional audio

French, Spanish and Portuguese are dubbed apart for some regions. Polyfin tells six regional variants apart, by their BCP 47 tags, and each user may prefer one as their **Audio region** (see [Audio region](users.md#audio-region)):

| Variant | Tags that name it | Release words that name it |
|---|---|---|
| French (France), `fr-FR` | `fr-FR`, and `fr-BE`, `fr-CH`, `fr-LU`, `fr-MC`, which hear the same dub | VFF, TRUEFRENCH, VF2 |
| French (Quebec), `fr-CA` | `fr-CA` | VFQ, VF2 |
| Spanish (Spain), `es-ES` | `es-ES` | CASTELLANO |
| Spanish (Latin America), `es-419` | `es-419`, and the tag of any Latin American country or of the United States (`es-MX`, `es-AR`, `es-US`…) | LATINO, LATAM |
| Portuguese (Brazil), `pt-BR` | `pt-BR`, `pob` | DUBLADO, PTBR |
| Portuguese (Portugal), `pt-PT` | `pt-PT`, and the tags of the other Portuguese-speaking countries (`pt-AO`, `pt-MZ`…) | |

- A track's variant is the one its language tag names, as Matroska files keep a BCP 47 tag with its region, by any code of the language (`fr-CA`, `fre-CA`); else the one its title names: a tag (`Portuguese (PT-BR)`), the region's name or code in English or in the language (`French (Canada)`, `Français (Québec)`, `Spanish (Latin America)`, `Castellano`, `Brazilian`, `French (CA)`), or a release word of its language (`VFQ`). A track tagged `fre` and titled `VFF` is French (France).
- A version's variants, until it is analyzed or described by RemuxDB, are those its labels name, as the [version order](#version-order) reads them: the release words above, a tag written in two words (`PT-BR`, `ES-MX`, `es-419`), and the flags of Mexico, Argentina and Colombia (Latin America) and of Brazil. Each also names the language. The flags of France, Spain and Portugal stand for the language at large, as releases show them for any dub of it.
- Subtitled releases keep their original audio. The languages named right after a word saying subtitles (Sub, Subs, Subtitles, Subbed, VOST, VOS, VOSE, Legenda, Legendado, Subtitulado, Sottotitoli, Untertitel…) are the subtitles', as in `ENG Sub ITA` or `Audio: English | Subtitles: French, Spanish`, and so is the language named right before Subs, Subbed, Subtitles, Subtitled or Legendado when none follows, as in `French Subs` or `PT-BR Legendado`. None of these counts as the version's audio, and `MULTi-SUBS` does not say it has several audio languages. Tags such as VOSTFR and SUBFRENCH name no audio language either. A word of the title is never taken for one of these words.

With an **Audio region**, among the tracks and versions in the user's audio language:

- the default audio track is the one of the region, else one of no known region, else one of another region. The flagged track still comes first for a user who chose their language in their app and plays the default track whatever its language (`PlayDefaultAudioTrack`), as Jellyfin does;
- with **Preferred audio language** in **Order versions by**, the versions carrying the language in that region come first, then those carrying it with no known region, then those carrying it only in another region, then those that say MULTI or DUAL, then those of unknown language, then the others (see [Version order](#version-order)).

The region applies only to its own language: a user who chose English in their app keeps English tracks and versions first, whatever their region. A user with no audio language, their own or the server's, gets the region's language, which ranks above the flagged track as the server's **Default audio language** does.

**Compared with Jellyfin:**

- Jellyfin 12.4 keeps a track's language as tagged and compares it with the user's preferred language as a whole string, in any letter case ([MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L181-L190)): its preference has no region, a track tagged `fr-CA` does not match a preference of `fre`, and two French tracks rank alike. Polyfin compares languages by any of their codes, and the region is its own setting.
- Jellyfin reads no language from release names. Polyfin's reading of regions and subtitled releases is its own, for the version order.

### Remembered track choices

When a user picks an audio track or subtitles other than those a title plays by default, Polyfin remembers the choice for the series, on all its episodes, or for the movie, and plays it by default on the user's next plays, in every app.

- It follows the user's **Set audio track based on previous item** (`RememberAudioSelections`) and **Set subtitle track based on previous item** (`RememberSubtitleSelections`), in jellyfin-web's playback settings, each for its own kind of track. Both are on for every new user. A setting turned off stops its choices from applying at once, and the next playback report forgets them, as in Jellyfin.
- A choice is the track's language, with a subtitle's forced and hearing-impaired flags, or no subtitles; never the track's number, as the versions of a title, and the episodes of a series, number their tracks differently. A track of no stated language is not remembered.
- Polyfin learns the choice from the playback reports apps send (`POST /Sessions/Playing` and `POST /Sessions/Playing/Progress`), when their tracks change: a track other than the one the title plays by default becomes the user's choice, and the track their language preferences play anyway forgets it. The text copy of a picture subtitle counts as its original.
- On later plays, the choice comes first, before the user's languages and the server's defaults, in the default tracks of a title's details and of `PlaybackInfo`. A track the app names in `PlaybackInfo` still wins. Among the audio tracks in the chosen language, the one flagged default comes first; among the subtitles, those equally forced, a hearing-impaired one first when the choice was one.
- A version without such a track plays as the user's preferences have it, and the choice stays for the next episodes.
- The subtitle mode still applies: with `None`, no remembered subtitle shows, and with `OnlyForced`, only a remembered forced track, or no subtitles.
- **My account › Track choices** counts the series and movies with a choice, and **Forget my track choices** forgets them all; **Reset my preferences** does too (see [Track choices](users.md#track-choices)).

**Compared with Jellyfin:**

- Jellyfin 12.4 remembers the number of the track for the item played alone, in the user's data of the item ([SessionManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Session/SessionManager.cs#L1016-L1055)), and plays that number again when the item's source has such a track ([MediaSourceManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L493-L543)). Its choice reaches neither the next episode nor another version, which numbers its tracks otherwise; Polyfin's, a language per series or movie, reaches both.
- Jellyfin keeps every track reported, the default one included, so a later change of a user's languages does not reach an item played once. Polyfin keeps only a track other than the default.
- With the same settings on, jellyfin-web picks the tracks of the next item of its queue to match the last ones, and names them in `PlaybackInfo`; as any track an app names, they win.

### Title pages

A title's page opens as soon as its description is ready, without waiting for the stream addons, some of which are slow. It lists the versions Polyfin already knows: those of the addons that answered for the title within **Keep version lists for (minutes)**, and those of IPTV sources. Polyfin asks the other addons in the background meanwhile.

A title opened again once **Keep version lists for (minutes)** has passed shows the versions known before at once, even old ones, while Polyfin asks the addons again. For this, Polyfin keeps an addon's list for 24 hours after that time, also across restarts: the last list of each addon for each title is saved in the database. Listings may show these versions too.

- Play does not wait for the addon's new answer: it plays the old versions, asking the addon again in the background. Expired links are renewed when the source refuses them. Only when none of the old versions plays does Play wait for the new answer, then choose among its versions.
- After a restart, a title shows the versions saved before at once, as old versions, while Polyfin asks the addons again.
- The new answer replaces the old list. The versions it lacks stay listed after its own until Polyfin stops asking the addon again (see below): an addon that gathers other addons' streams often answers first with only part of them. They still play meanwhile.
- Once Polyfin stops asking, the list is the addon's last answer alone: the versions the addon no longer lists disappear.
- [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) drops the old lists too.
- Subtitles from the addons are kept the same way.
- An answer made only of notices, with nothing to play, as an addon sends when it limits requests, counts as the addon failing: the versions it listed before stay listed, and it is asked again as before. The log tells it (`An addon could not list streams`). An empty answer still replaces the list: the addon has nothing for the title any more.

Some addons gather other addons' streams. They answer the first request for a title with the streams that came in within their own time limit, and get the others moments later. So Polyfin asks an addon again 10 seconds after its first answer for a title, and once more 30 seconds later if that answer listed more playable streams:

- Polyfin keeps the longer list, from then on for **Keep version lists for (minutes)**. It never replaces a list with a shorter one. Each answer is compared with the addon's previous answer, not with the old versions still listed after it.
- It stops at the first answer that lists no more streams, at an error, or once the list is dropped, by [Refresh metadata](jellyfin-compatibility.md#refresh-metadata), or has expired, at the end of **Keep version lists for (minutes)**. A late answer never brings back a dropped list, nor makes an expired one current again.
- It asks again only when it asked the addon itself. A list it already kept is used as it is. IPTV sources are never asked again: their streams are Polyfin's own.
- Subtitles from the addons are asked again the same way, and the longer list kept.
- Opening a title and playing never wait for these requests.
- Renewing an expired link asks the addon as opening the title does, joining a request already under way, and asks again in the same way. When the answer lacks the version's file, Polyfin asks the addon again at once and waits for that answer, 10 seconds at most, before giving the link up. Meanwhile the title keeps listing every version.
- A version keeps its identifier from one answer of its addon to the next, and with it its place on a title's page already open and what Polyfin learned of its file. Polyfin knows a file by its name and size when the addon gives both, else by its link. When an answer gives a new link, as links that carry a token do, or tells the file's name or size only then, the stream takes the identifier of the version of the list it replaces that is the same file: their names and sizes agree wherever both give them, and either the link is the same, or the file's name is, or only a token differs, the link keeping its host and the last part of its path, which is the file's name or comes with the same stream name and description. A match counts only when it is the one on each side, so two versions never share an identifier. Saved lists keep these identifiers across restarts, and renewing an expired link finds the version the same way.

In apps:

- In the web player, the versions appear in the page's version menu as each addon answers, then as an addon asked again lists more. They are added in place: the page is not reloaded, and a version already picked stays picked with its audio and subtitle choices. The page reloads its details only when the version picked is gone or has changed, for instance when the placeholder gives way to the first version. The version selected, the first by default or one the user picked, stays selected even when a version that comes is listed first: the first version carries the title's own identifier, which passes to the new first one, and the page then selects the version it had under that version's own identifier, even when a later answer renamed it or told its size. A version that loses its identifier all the same is selected again when it is the one version listed with its name, and its size when both are known. Nothing changes while a video plays or while the version menu has focus, since it may be open: the versions come once it loses focus.
- With **Add versions to an open title's page** off, under **Settings › Web player**, the web player adds no version to a page already open. A page opened before any version was known still has the placeholder give way to the versions known then, once, and its play buttons still wait for a version. The others show when the page is opened again.
- Other Jellyfin apps cannot be told to refresh the page. They show the versions known when it opened, and get every version when the user presses Play, or when the page is opened again.
- With [Prepare playback in advance](#preparing-playback-in-advance) on, Polyfin lists the versions of the titles of Continue Watching and Next Up as soon as an app asks for these rows. Once they are listed, every app opens those titles with all their versions.
- In the web player, a movie's or an episode's Play, Resume and Play from the start buttons wait for a version. While none is known and addons are still asked, they are greyed and disabled, with a small spinner and the tooltip "Looking for sources…". They come back as soon as the first version is known, without reloading the page. The page asks Polyfin as soon as it opens; until Polyfin answers, the buttons wait already when the page lists only the placeholder, as it does while no version is known.
- When every addon has answered without a version, the buttons stay disabled and a line under them says "No source is available for this title.", with a **Try again** button. It has Polyfin drop the title's version lists for the user's addons and ask them again, and the buttons wait again meanwhile. A user can ask again once every 20 seconds for a title. The words follow the web player's language (English, French, German, Spanish, Italian or Portuguese; English otherwise). Nothing changes during a video, and leaving the page gives the buttons back as they were.
- In other apps, until a version is known, the title still shows as playable. Play waits for the addons still answering for the first time, then picks among all the versions, as before.
- An addon that fails, or does not answer within 15 seconds, only leaves its versions out.
- Subtitles from the addons follow the same way: those known show at once, and the others with Play or the next opening. Play waits for them one second at most once the versions are in: later ones come with the next play or track change.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Add versions to an open title's page** | **Settings › Web player** | On | Has the web player add the versions that come to a title's page while it is open. Off, it only has the placeholder give way to the versions known then. |

**Compared with Jellyfin:**

- Jellyfin knows every version of its titles beforehand, so its pages list them all at once.

**For app developers:**

- Item details (`/Items/{id}`, `/Users/{userId}/Items/{id}`) describe the versions known when asked, an expired list's included. With none known, they describe one placeholder source under the title's own identifier, which plays the first version. `PlaybackInfo` waits for every addon asked for the first time, or asked again because its list expired, joining the requests the details started, but never for an addon asked again after an answer.
- `GET /Polyfin/Items/{id}/Versions` answers `{"Pending": <addons still asked for the title, for the first time or again>, "Count": <media sources the details would list now>, "Known": <versions known now>, "AddVersions": <whether the web player adds versions to a page already open>}`, with the authentication and access checks of item details. An addon to be asked again counts from its first answer until it is no longer asked. `Count` includes the placeholder; `Known` leaves it out, so it is `0` until a version is known, old versions of an expired list included. Both drop when the versions an addon's new answer lacked are no longer listed. Items other than movies and episodes answer `0` for all three. Polyfin also pushes it on the live connections (WebSockets) of the user's jellyfin-web, the app signed in as `Jellyfin Web`, as it changes, for 2 minutes after the details or this endpoint were last asked, in a `PolyfinVersions` message whose `Data` is `{"ItemId": <the identifier the page was opened with>, "Pending": …, "Count": …, "Known": …, "AddVersions": …}`. Other apps get no such message: the Jellyfin SDKs some apps are built on fail on a message type they do not know. `AddVersions` is **Add versions to an open title's page**, under **Settings › Web player**. Polyfin's web player script listens to it, and polls at once when a title's page opens, then every second while `Pending` is above `0`, every 10 seconds once a push came, for at most 90 seconds. The 90 seconds cover a first answer (at most 15 seconds) and both requests that follow it (10 and 30 seconds later, at most 15 seconds each). When `Count` is above what the version menu lists, or differs from it once `Pending` is `0`, the script asks for the item's details (`/Users/{userId}/Items/{id}`) and lists their media sources in the menu. When `AddVersions` is `false`, it does so only while the menu lists the placeholder alone, a single source under the title's own identifier named after the title. Each media source's `ETag` is its version's own identifier, the first one's included. When the version selected loses its identifier, to a source listed first or as Polyfin names it otherwise, the script finds it by the `ETag` it listed it with, else as the one source with its name, and its size when both are known, and selects it under its new identifier before reloading the page's details, which jellyfin-web then keeps selected. It holds the play buttons while `Known` is `0`, searching while `Pending` is above `0`, with no source once it is `0`. Its first answer may come before the details asked any addon, so it takes no source from it, only from the next. Before any answer, it holds them as searching when the version menu lists a single source whose identifier is the title's own, the placeholder.
- `POST /Polyfin/Items/{id}/Versions/Search` asks the user's stream addons again for a movie or an episode, as [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) does for its streams only: each addon's stream list for the title is dropped, the saved one included, its follow-ups stop, and the addons are asked in the background, as item details ask them. Addons still asked for the title are left to answer. It has the checks of `GET /Polyfin/Items/{id}/Versions` and answers the same progress, `Pending` counting the addons now asked. A user may ask once every 20 seconds for a title; sooner, it answers `429 Too Many Requests` with `Retry-After` in seconds. Items other than movies and episodes ask nothing and answer `0` for all three.
- With **Prepare playback in advance** on, `GET /UserItems/Resume`, `GET /Users/{userId}/Items/Resume` and `GET /Shows/NextUp` queue the first 10 movies and episodes of their answer before answering, and never wait for them. Each queued title is listed waiting for every addon and joining any request already under way. The first 2 then have their first version analyzed.

### Tracks from RemuxDB

Until a version is played, Polyfin knows only what its addon says of it. With **Describe versions from RemuxDB** on, a title's page shows the tracks of each version RemuxDB knows before it is played:

- the video: resolution, codec, HDR or Dolby Vision;
- the audio tracks: language, codec, channels;
- the subtitles inside the file: language, forced, for the deaf and hard of hearing;
- the file's size, length and bitrate.

[RemuxDB](https://remuxdb.1632022.xyz) is a community database of what probing release files found in them.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Describe versions from RemuxDB** | **Settings › Playback** | Off | Asks RemuxDB about the titles whose pages open, and describes their versions from its answers until they are analyzed. |
| **RemuxDB address** | **Settings › Playback** | `https://remuxdb.1632022.xyz` | The RemuxDB server Polyfin asks. |

- When a title's page opens, Polyfin asks RemuxDB about the title by its IMDb identifier, with the season and episode numbers for an episode. It waits for the answer one second at most; a later answer shows from the next opening. Titles the addons know by other identifiers are not asked about.
- Polyfin finds each version among the files RemuxDB lists by the name of its file, as the addon gives it, folders left out and case ignored. When the addon gives the file's size, the sizes must agree within 64 MiB or 1%, the larger, as addons round sizes. When several files share the name, the one of the closest size wins; without a size, a shared name describes nothing. A file under a tenth of the title's runtime, a sample, describes nothing either.
- What RemuxDB said of a version, whether it knew the file or not, is kept in memory for 6 hours. A version listed since has the title asked about again. After RemuxDB fails to answer, it is not asked for a minute, or for as long as its `Retry-After` says.
- Playback still analyzes each version with ffprobe, and the analysis then replaces the description. The tracks keep the numbers ffprobe gives them, so a track picked before the first play stays the same track.
- Until the version is analyzed, the height RemuxDB found places it in the user's quality group (see [How the height of a version is found](users.md#how-the-height-of-a-version-is-found)).
- RemuxDB knows the files of torrents and NZBs that its contributors probed. A version is described only when its addon gives its file's name and RemuxDB probed that file.
- Polyfin sends RemuxDB only the titles' identifiers and a name for the server, derived from the server's identifier, which RemuxDB requires. It sends none of its own analyses.

**For app developers:**

- Item details and listings describe a version not analyzed yet with the tracks RemuxDB found, numbered as ffprobe numbers them, after the subtitle files as usual. These `MediaStreams` lack what only an analysis reads, such as `TimeBase` and `CodecTag`; the version has no chapters nor attachments yet. `PlaybackInfo` always answers from the analysis.

### Live videos

A video an addon lists among its titles of other types, such as a sports addon's match on now, may stream with no end: an HLS playlist without `#EXT-X-ENDLIST`, or a stream that announces no size and gives no duration. Before its first play, or as its play is prepared ahead when its details open, Polyfin reads the first answer of its first version: a stream that announces no size is told as soon as it answers, without waiting for its bytes, which a live stream sends as they come; otherwise its first bytes tell, and a file, of a known size, plays as any video. A playlist, or a stream of no size, is analyzed as a Live TV channel's stream is (see [Live TV](live-tv.md)), and plays live when it has no duration. The analysis is kept, and later plays are told by it at once. Interlaced sports, as most TV sports in 1080i50, play at 25 frames a second converted unless **Double the frame rate** is on (see [Starting a channel](live-tv.md#starting-a-channel)).

A live video plays as a channel does:

- From its live edge, with no duration and no seeking. The app gets an infinite stream (`IsInfiniteStream`, no `RunTimeTicks`), relayed as it is, or repackaged or converted by FFmpeg as it comes, as the app takes it.
- No keyframe index is read, and no conversion waits for its end. It counts among the channels played through FFmpeg at once, as a channel does.
- It keeps no resume point and is never marked played, as channels keep no user data, and it has no line in the activity log. Its playbacks show on Home as any other. No scrubbing thumbnails are made of it.

A video whose stream ends, such as a recap's file, plays as a video, with its duration, resume point and played state.

**Compared with Jellyfin:**

- Jellyfin plays a live item without a duration or a resume point too (`LiveTvChannel.SupportsPositionTicksResume` is false). It marks it played as it starts, and when it stops without a runtime (`SessionManager.OnPlaybackStart`, `UserDataManager.UpdatePlayState`). Polyfin keeps no user data of a live video, as of a channel, so that a match on now does not show as watched in its library while it goes on.

## Chapters

The analysis also reads the version's chapters, which apps show and let you skip through. Apps get the chapters of the version the title was opened or played as. A version never played has no chapters yet. Chapters named as an intro, a recap, a preview or credits also give that version's skip buttons (see [Segments of each version](skip-segments.md#segments-of-each-version)).

Chapters come without chapter images unless you turn those on (see [Scrubbing thumbnails and chapter images](#scrubbing-thumbnails-and-chapter-images)). Chapters are always sent: they are read along with the analysis every first play needs, so they never delay playback.

## Preparing playback in advance

The first play of a version waits for some reads. **Prepare playback in advance** does them before the user presses Play:

- as soon as a title's details open in an app, it analyzes the first 2 versions the title would play, those a play reads at once, and again when an addon answering later puts another version first;
- it then reads each version's keyframe index and where its subtitle tracks sit, which HLS playback needs, and the bytes of its first segment (see [Reading the sources](#reading-the-sources));
- it readies the next episode the same way, with its versions and subtitles, once the episode playing has 9 minutes left;
- as a song or audiobook starts, it resolves the next track of the app's queue (else of the album) the same way: where it streams from, and its analysis when its addon does not describe it;
- when an app asks for Continue Watching or Next Up, it asks the addons, in the background, for the versions of the first 10 movies and episodes of each row. A title opened from these rows then lists its versions at once, instead of a placeholder while the addons answer (see [Title pages](#title-pages)). The first version of the first 2 titles of each row is analyzed too, so that resuming from these rows starts at once.

Playback then starts at once instead of waiting a second or two for these reads.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Prepare playback in advance** | **Settings › Playback** | On | Analyzes and reads ahead as described above. |

It costs a few more requests to the sources, also for titles opened but not played. Limits:

- at most 2 preparations run at once; the others wait, the newest first, so that the title opened last is prepared first. Beyond 4 waiting, the oldest is skipped;
- a title is prepared once per user every 10 minutes, unless its preparation failed;
- the titles of Continue Watching and Next Up are listed 2 at a time over the server, apart from the preparations above, in the order the rows asked for them. A title waiting or being listed is not queued again, and a title whose versions are still kept (**Keep version lists for (minutes)**) asks nothing.

A torrent version is left out of these preparations while no playback runs its torrent: only a playback joins a torrent's swarm, and its first play analyzes it (see [How a torrent starts](torrents.md#how-a-torrent-starts)). Opening a title's page then neither reads it nor logs it.

## Choosing a version

Five more settings change how Polyfin picks a version and how much video the server converts. Their defaults keep Polyfin's earlier behavior. A change applies to the next request, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Maximum time to analyze a version** | **Settings › Playback** | 20 seconds (5 to 120) | Bounds every ffprobe analysis, of files and of live streams, but a torrent's, which waits for its pieces as its play does (see [Torrents](torrents.md#deadlines)); a channel is analyzed for 8 seconds at most. A source that does not answer in time is given up for 15 minutes, and Polyfin moves on to the next version sooner. |
| **Versions tried when one does not work** | **Settings › Playback** | 3 (1 to 10) | How many versions Polyfin analyzes when the app picked none, for titles and channels alike, up to 3 at once for titles. Later versions analyzed before are still tried, as they cost nothing. |
| **Prefer versions the app plays without conversion** | **Settings › Playback** | Off | Looks among the versions analyzed at once for one that plays on the app as it is or remuxed with its tracks copied. If none does, falls back to the first that plays at all. |
| **Video conversions at once (0 = no limit)** | **Settings › Conversion (transcoding)** | 0, no limit (up to 32) | Caps the playbacks whose video the server converts. |
| **Maximum quality of converted video** | **Settings › Conversion (transcoding)** | Original (or 480p to 2160p) | Scales converted video down to that height. |

### Prefer versions the app plays without conversion

- The first play of a title may analyze more versions. Nothing changes once they are known.
- It looks among the first versions in the order the [version order](#version-order) settings give them.
- A version above the user's **Maximum quality** never counts as one that plays without conversion.

**Compared with Jellyfin:**

- Jellyfin 12.4 keeps the version opened first however it plays, as Polyfin does with this setting off.

### Version order

The **Version order** group of **Settings › Playback** orders a movie's or an episode's versions, and hides some of them. By default it changes nothing: versions keep the addons' order and none is hidden.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Order versions by** | **Settings › Playback** | None | Up to four criteria, each grouping the versions and the next sorting within them: **Preferred audio language**; **Resolution, highest first** or **Resolution, lowest first**; **File size, largest first** or **File size, smallest first**; **Video codec, in the order below**; **HDR and Dolby Vision first** or **HDR and Dolby Vision last**; **Source, in the order below**. Versions that tie keep the addons' order. |
| **Order of video codecs** | **Settings › Playback** | AV1, HEVC, VP9, H.264, VC-1, MPEG-4 Part 2, MPEG-2 | The order the video codec criterion puts versions in. |
| **Order of sources** | **Settings › Playback** | The order of **Content › Sources** | The order the source criterion puts versions in: the server's addons and folders that give versions of movies and episodes, moved by drag or with the arrows. |
| **Hide versions larger than (GB, 0 = no limit)** | **Settings › Playback** | 0 (up to 1,000) | Hides the versions known to be larger. |
| **Hide these video codecs** | **Settings › Playback** | None | Hides the versions known to be in these codecs. |
| **Hide versions whose labels contain** | **Settings › Playback** | None (up to 50 lines) | Hides the versions whose name, description or file name holds one of these words, one a line, in any letter case: for addons that say in their labels that a file is not ready to play yet. |
| **Hide low-quality releases** | **Settings › Playback** | None | Hides the versions whose labels tag them as one of the kinds ticked: **Camera copies (CAM, HDCAM)**, **Telesyncs (TS, HDTS, TELESYNC)**, **Telecines (TC, TELECINE)**, **Screeners (SCR, DVDSCR)** and **Early copies (R5, WORKPRINT)**, read as told below. |

**What the rules know of a version.** Nothing is read from the source for them. For each thing they look at, they take the version's analysis when Polyfin has one, else the description of its file from [RemuxDB](#tracks-from-remuxdb) when it is on, else its addon's labels:

- the height, as [quality groups](users.md#how-the-height-of-a-version-is-found) find it;
- the size the addon gives (`videoSize`);
- the codec its labels name: HEVC, x265 or H.265; H.264, x264 or AVC; AV1; VP9; VC-1; MPEG-2; Xvid or DivX;
- HDR when they name HDR, HDR10, HDR10+, HLG, DV, DoVi or Dolby Vision. Labels that name the height or the codec but no HDR count as SDR;
- the audio languages they name: language names in English (French, German…), some others used by releases (TRUEFRENCH, VFF, Latino, Castellano…), ISO 639-2 codes in capitals (ENG, ITA, GER…) and flags, with their regional variants (see [Regional audio](#regional-audio)). MULTI and DUAL say a version has several languages. The languages named as the subtitles' (`Sub ITA`, `French Subs`) are not the audio's. A word of the movie's or the series' name is never taken for a language.

A version a criterion knows nothing of comes after those it knows, and a hide rule never hides it.

**Hide low-quality releases** reads the stream's name, title and description as its addon gives them, and its file name, line by line. A tag counts only as a whole word, between dots, dashes, underscores, spaces, brackets or other signs, in any letter case: "Its" and "Nights" hold no `TS`, and `HDCAM` and `hdcam` are both camera copies. It counts only after the title's own words, so that a movie named Cam, or The Cam, is never taken for a camera copy:

- in a line naming the movie or the series, as its addon names it, or the episode's own name, the tags count after the last of these names;
- in a line naming none of them, after its first year (1900 to 2099) or episode number (`S01`, `S01E01`, `1x01`), which follows the title in a release's name;
- in a line holding neither, such as an addon's "CAM 720p", every word may be a tag.

A line ending with `.ts` names an MPEG-TS file: that extension is no telesync. So, for the movie Cam, `Cam.2018.1080p.WEB-DL.mkv` is kept; `Some.Movie.2026.HDCAM.x264.mkv` is a camera copy and `Some.Movie.2026.1080p.WEB.ts` is kept; `Its.Always.Something.S01E01.1080p.mkv` is kept and `Some.Show.S01E01.TS.mkv` is a telesync; `Some Movie (2026) [HDTS]` is a telesync. A tag before the title in the same line, as in `[HDCAM] Some Movie 2026`, is not read.

**Preferred audio language** is each user's own, the audio language they set in their app, else the server's [Default audio language](#default-audio-and-subtitle-languages). Versions known to carry it come first, then those that say MULTI or DUAL without naming it, then those of unknown language, then the others. For a user without a preferred language, and without a default one, it changes nothing. A user's **Audio region** orders the versions carrying the language among themselves, and gives the language to a user without one (see [Regional audio](#regional-audio)).

**Source, in the order below** groups a title's versions by the addon or folder that gives them, the next criteria sorting each source's versions; placed after other criteria, it orders the versions they tie on. The sources **Order of sources** lists come first, in its order, then the others in the addons' order: a source added since the list was saved comes after those it lists, until it is moved there, and a user's own addons come after the server's sources. A source removed leaves the list. The list orders versions only: which addon describes a title, the order of the addons' subtitles and that of search results keep the order of **Content › Sources**, as versions do while the criterion is not chosen (see [Addons and libraries](addons-and-libraries.md)). It is how local files come first, whatever the other criteria (see [Local folders](local-folders.md#versions)).

**In what order things happen:** these steps follow steps 1 to 5 of [Which streams become versions](#which-streams-become-versions).

1. The versions that failed lately are left out (see [Sources that fail](#sources-that-fail)), but the torrents that did not start in time, which stay in place (see [Which streams become versions](#which-streams-become-versions), step 6).
2. The hide rules apply, with the versions of [another title](#versions-of-another-title), which are hidden whatever the rules. When they would hide every version of a title, none is hidden.
3. The user's [quality group](users.md#quality-groups) leaves out the taller versions. When no version fits, the closest height comes first, and the criteria order the versions of the same height.
4. The criteria order the others.

- Item details and `PlaybackInfo` list the versions in that order, so version menus show it. The first, named after the title, is the one Play starts first and **Prepare playback in advance** analyzes. **Versions tried when one does not work** and **Prefer versions the app plays without conversion** take the versions in that order.
- Hidden versions are listed nowhere: not in details, `PlaybackInfo`, downloads or trickplay. A version an app asks for by its own identifier, one it was shown before a rule hid it, is refused as if it were gone.
- While a title's page is open, and Polyfin follows it (see [Title pages](#title-pages)), the versions it shows keep their places as long as the rules stay the same, the order of sources included, even when an analysis tells more of them; versions that come later take their places by the rules. A page opened again, or listed again once the rules changed, lists them in the rules' order.
- Merged Live TV channels keep their own order (see [IPTV](iptv.md)).

**Compared with Jellyfin:**

- Jellyfin 12.4 has no setting that hides versions: it lists every version of a title, whatever its size, codec or name (its server and library options have none, [`ServerConfiguration.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Configuration/ServerConfiguration.cs), [`LibraryOptions.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Configuration/LibraryOptions.cs)). The version order rules are Polyfin's own.
- Jellyfin knows the same release tags only to cut them from a file's name when it finds a title's name: its first `CleanStrings` expression ([`NamingOptions.cs`, line 154](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Naming/Common/NamingOptions.cs#L152-L154)) ends the name at the first of `cam`, `ts`, `tc`, `r5`, `dvdscr`, `screener`, `telesync` or `telecine`, among many other tags, as a whole word after at least one word of the title, in any letter case. Polyfin reads tags the same way, after the title's words, to hide versions rather than to name titles.

### Version names

With **Say in version names how they play**, in **Settings › Playback** and on by default, the name of a movie's or an episode's version ends with how the app plays it, as a synced subtitle's ends with "(synced)" (see [Subtitles](subtitles.md)). The rest of the name does not change.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Say in version names how they play** | **Settings › Playback** | On | Ends version names with how the app plays each. Off, names are the addons' as before. |

- "(direct play)": the app plays the file as it is.
- "(direct stream)": its tracks are copied, without conversion, into HLS.
- "(converted: reason)", with one reason, the first that applies: "HDR" when the app cannot show the video's HDR or Dolby Vision; "quality limit" when the app's quality setting, the user's **Maximum quality** or their quality group needs the video converted down; "subtitles" when a subtitle is burned into the video; "video" for any other reason about the video; "audio" when only the audio is converted. "(converted)" alone for any other reason.
- "(cannot play here)": the version needs a conversion the server or the user may not have (see [Transcoding](transcoding.md)).

How names are decided:

- Only versions analyzed, or described by [RemuxDB](#tracks-from-remuxdb), are named. Naming reads nothing: it runs the decision of a play on what is known of each version. A version not known yet has no ending.
- `PlaybackInfo` names every version it lists for the device profile the app sends, not only the one chosen.
- Item details know no device profile. Polyfin keeps in memory each device's last `PlaybackInfo` profile and bitrate, and names versions for that device with them: the web player's version menu, Android TV's **Select version** and Swiftfin's version menu read names there. Before a device's first play, after it signs out, or after a restart, names have no ending.
- The decision uses the audio and subtitle tracks the user's preferences choose. Picking other tracks in the app may change how the version plays; the name cannot tell.
- A file whose keyframe index turns out to be missing once played, such as some MPEG-TS files, has its video converted all the same, while its name said "(direct stream)": the index is not read for names.
- Names are in the server language, **Language of generated names** (see [Getting started](getting-started.md#language-of-generated-names)): in French, "(lecture directe)", "(flux direct)", "(converti : HDR)" and so on, "(illisible ici)". An ending is 30 characters at most; apps that cut long names to one line may hide it.

**For app developers:**

- Only `MediaSources[].Name` changes. `Id` and `ETag` stay, so the web player keeps a picked version selected.

### Video conversions at once

- The cap counts conversions of files and Live TV together, burning subtitles in included, and [converted downloads](transcoding.md#converted-downloads) whose video is converted, from their start to their end. Each converted download counts on its own, even next to the user's playback of the same title, as it runs a conversion of its own.
- Live TV keeps its own caps on top: 4 channels per user and 16 for the server. See [Live TV](live-tv.md).
- A playback is what a user plays of a version, from its first converted segment until the app stops it or asks for nothing for 3 minutes (1 minute for Live TV). Its next segments, its seeks, and the new play session apps open to switch tracks never count again.
- The lower qualities of a playback with **Several qualities when converting** count with it, as one playback: an app stepping down at the cap is never refused. While an app switches, the qualities it asked for in the last 10 seconds keep converting, so that switching back does not start over: that costs more of the machine for those seconds only, never another place under the cap. See [Adaptive quality](transcoding.md#adaptive-quality).
- With **Keep the converted video when the audio track changes**, a playback whose session stopped keeps its converted video 30 seconds for the next session to take over, and counts as one playback meanwhile, with that session once it does. See [Changing the audio track](transcoding.md#changing-the-audio-track).
- A recording converted to HEVC in the background counts as one conversion while it runs, and starts only below the cap: it waits while playbacks fill it. See [Smaller recordings](live-tv.md#smaller-recordings).
- At the cap, Polyfin plans no new video conversion, as when conversion is off. It picks the next version that plays without one, or else reports that no version is compatible.

**Compared with Jellyfin:**

- Jellyfin has no such cap. Its nearest, a tuner's stream limit, fails with a `500`, which apps take the same way, as a server error.

**For app developers:**

- At the cap, `PlaybackInfo` answers `NoCompatibleStream` when no version plays without conversion.
- An HLS request that would start one more conversion answers `503`, as when Live TV is at its cap.

### Maximum quality of converted video

- Video keeps its shape and is converted at the bitrate of that height. It fits that height's 16:9 frame: a 2.40:1 picture converted to 1080p is 1920×800, as in a 1080p release.
- A running conversion keeps the size it started with.
- Files played as they are, or with their tracks copied, are untouched.
- A GPU converts up to 4K (2160p), at the source's own size when nothing lower is set: a 3832×1600 picture stays 3832×1600. The processor converts to 1080p at most, so 1440p and 2160p change nothing there.

See [Transcoding](transcoding.md) for how conversion works.

## Watching together

Apps that offer SyncPlay, jellyfin-web first, play the same titles in step across devices and users.

- Anyone in a group can set or change what plays (up to 5,000 entries), play, pause and seek.
- The group starts and resumes on Polyfin's clock, a little ahead so that every app gets there in time.
- The group waits for apps still loading or buffering, for 30 seconds at most.
- Every user may create and join groups. A group is listed to, and can be joined by, only the users who can see everything it plays. Nothing a member cannot see is queued.
- A session leaves its group when its app disconnects or is signed out (by the app, a password change, or an administrator). A group ends once empty.
- The apps need Jellyfin's live connection (WebSocket) open.

**Compared with Jellyfin:**

- Like Jellyfin, Polyfin keeps the groups in memory.

## Reading the sources

FFmpeg, ffprobe and relayed apps read a version's file through Polyfin's source cache, as large as **Disk space for files being read (GB)** under **Settings › Playback** (10 GB by default; a lower value applies at once). It fetches the file from the source in blocks and keeps them on disk for seeks, in the data folder's `cache`, or in **Folder for files being read**, below it, from the start after it is set (see [Folders for conversion, files being read and the cache](configuration.md#folders-for-conversion-files-being-read-and-the-cache)).

The files of local folders go through it too. Those of S3 buckets, SFTP folders and rclone remotes are fetched straight from the storage, in blocks, as addons' streams are, and count against the same disk space: S3 and rclone over HTTP with ranges, SFTP over the folder's SSH connection; the limits per host below apply to their storage. The files of folders in the container, SMB shares and WebDAV folders are read by Polyfin itself at each request, through its loopback interface (see [network shares](local-folders.md#performance)).

- A file is read over up to 3 connections when its host serves each one at its own pace: one serves what is being read, the others the stretches read next. A host that asks to slow down (`429`, `503`), refuses one more connection, or serves several no faster than one, is read over one connection per file for an hour.
- Polyfin opens at most 4 connections to a host at once, over all the files it reads there, and 2 for 10 minutes after the host answers `429` or `503`. A playback never waits for them: the remux or relay of the version playing, its seeks, the analysis of the version a play chose, the bytes of its first segment, and the subtitles read through a file's index. Background reads wait for a free connection, and give theirs up to a playback: the reads ahead of a title whose details opened, the keyframe indexes read during an analysis, the other versions a play analyzes alongside the first, a file's second and third connections, which are not opened while the host has none free, and the requests of scrubbing images.
- Each request of FFmpeg or of an app is served on its own, the newest first. A seek is not slowed down by the request FFmpeg is leaving, and a request that ends stops what it waited for.
- Polyfin reads further ahead of a playback as it goes on, up to 256 MiB (at most an eighth of the cache). While FFmpeg waits for the app, the connection stays open for 30 seconds instead of being opened again for each segment. A torrent's file is read ahead only while a playback reads it: once the last one ends, its connections close at once, but those serving a read still waiting, so that a stop leaves the swarm (see [Torrents](torrents.md#only-while-playing)).
- When a stream's address redirects, as addons' resolvers do, Polyfin keeps the address it led to for its own reads of the file. Each connection then skips the resolver, and the file cannot change under a playback. Apps sent to the source still get the addon's address. An address that stops working sends Polyfin back to the addon's, then to a renewed link.
- Once a version's keyframe index is known, Polyfin reads the bytes of its first segment before the app asks for them, 64 MiB at most: from `PlaybackInfo`, and when a title's details open with [Prepare playback in advance](#preparing-playback-in-advance) on. For a resume, it reads the file's head, which FFmpeg probes, and the segment the playback resumes at. The index of a Matroska file is read during its analysis.
- A file relayed to an app is served from the same cache: what was read before takes no request, and a seek reads only what is missing.
- Before sending an app to a source for direct play, Polyfin checks that it answers, unless it answered Polyfin's own reads at that address within 2 minutes, as the analysis just did.

### Keeping the files read across restarts

The source cache keeps the parts of the files it read from one start to the next: a title played again, or resumed after a restart or an update, reads from the disk without a request to its source. **Disk space and retention** under **Settings › Playback** chooses a preset, which sets the fields below it; changing one of them makes the preset **Custom**. Its help tells the space free on the disk of the folder for files being read.

| Preset | Disk space | Kept for | Extra days for each play again | Next episodes preloaded |
|---|---|---|---|---|
| **Off** | 2 GB | Emptied at each start, as before 0.57 | 0 | Opening and end |
| **Small** (the default) | 10 GB | 3 days | 1 | Opening and end |
| **Everyday** | 50 GB | 14 days | 3 | Opening and end |
| **Large** | 250 GB | 60 days | 7 | Whole files |

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Disk space for files being read (GB)** | **Settings › Playback** | 10 GB | Bounds the parts kept, as above. |
| **Keep parts read for (days)** | **Settings › Playback** | 3 (0 to 365) | How long a part is kept once last read, restarts included. 0 empties the cache at each start. |
| **Extra days for each play again** | **Settings › Playback** | 1 (0 to 30) | How much longer the parts of a title played again are kept, for each play after the first. |

- **What is kept:** the files of addons, and of S3 buckets, SFTP folders and rclone remotes, by their version, whatever link the addon gives for them next time. A torrent's file is never kept: its pieces are removed as it stops, by design (see [Torrents](torrents.md)). A file whose size was never told, such as a live video, is not kept either.
- **A link that changed:** after a restart, a file opened at the same link is read from the disk at once. One its addon now gives another link for is asked once, for its size: the same size reads the parts kept, another size is another file, and its parts kept are removed (`A source's link now names a file of another size: the blocks kept of it were removed`, at Info).
- **Plays again:** a title counts as played again when a playback reads it 6 hours or more after the last one. Each play after the first, 8 at most, adds the extra days to its parts, which are removed after those of titles played once, both when the disk space is full and when their days are over.
- **Which parts go first:** past the disk space, the parts read longest ago go first, the extra days counted; those read in the last 30 seconds stay. Past their days, parts are removed at start and every hour.
- **Crashes:** what each file keeps is written beside its parts 30 seconds after a change, and as Polyfin stops, once the parts reached the disk. After a crash, the parts read during the last 30 seconds are read again; a part never claimed is never served.
- **Health:** **System › Health › Source cache** tells the space used against the disk space, how long parts are kept, the files open and the files kept, and **Read from the disk**: the share of the bytes FFmpeg, ffprobe and the apps read since the start that were on disk when asked for, reads ahead included.

**For app developers:** `GET /admin/api/health` gives the cache as `cache`: `used`, `limit`, `sources` (files open), `kept` (files with parts kept), `hits` and `misses` (bytes read since the start that were on disk when asked for, and bytes waited for), `days` and `torrents`. `GET /admin/api/settings` gives `sourceCachePreset`, `sourceCacheDays`, `sourceCacheCreditDays`, each preset's values as `sourceCachePresetLimits`, and `readingFree`, the bytes free on the disk of the folder for files being read. A `PUT` that sets `cacheSizeGb`, `sourceCacheDays`, `sourceCacheCreditDays` or `preloadWholeFiles` to another value than its preset's, without a preset, makes it `custom`.

### Preloading the next episodes

With **Preload the next episodes** on, under **Settings › Playback** and off by default, Polyfin reads ahead the next episodes of the series each user watches, so that they start from the disk, after a restart too:

- every 15 minutes, starting 2 minutes after Polyfin starts, while nothing plays: no playback in **Home › Now playing**;
- for each user who played or started an episode in the last 14 days, the first 5 series of their Next Up, the latest first: the episode Next Up offers and the ones after it, **Episodes per series** in all (1 by default, 3 at most);
- the version Play would pick first, under the user's parental control, hidden titles, [version order](#version-order) and quality group: its first 128 MiB and its last 4 MiB, where many files keep their index, or the whole file with **Preload whole files**, which the **Large** preset turns on. A torrent version is never preloaded, nor a version that failed lately;
- at most a quarter of **Disk space for files being read (GB)** a pass, so that what was played stays;
- at **Speed limit (Mbit/s)** on average (20 by default), and only while the disk of the folder for files being read keeps **Free space to keep (GB)** free (20 by default).

A playback starting stops it before the next 8 MiB. Its reads are background reads, which give way to playbacks on the same host (see above). What it read is kept as any part read, and preloading the same episodes again asks nothing.

**Compared with Jellyfin:** Jellyfin reads files where they are, and has no source cache to keep or preload (see [Configuration](configuration.md#folders-for-conversion-files-being-read-and-the-cache)). Polyfin's apply to addons' and remote storages' files, which a restart would otherwise download again.

### Sources that fail

- A source that sends no headers within 15 seconds fails.
- A connection that sends nothing for 10 seconds is opened again. A host that then does not answer within a few seconds fails, and the segments waiting for it get an error within seconds instead of two minutes.
- A host that asks to slow down (`429`, `503`) is asked again, when opening a file or later in it, up to 4 times: after the delay it asks (`Retry-After`, 5 seconds at most), else after 0.5 seconds, 1 second, then 2 seconds each time. The reads wait meanwhile. Past that, the reads fail, and the file is tried again 5 seconds later.
- An answer of another size than the file, such as an error page or a link now naming another file, is asked once more, after renewing the link when the file was known, then fails. Its bytes are never served as the file's. A `416` that tells another size counts the same.
- A host that answers a range with the whole file (`200`) is taken to ignore ranges, and the file is read from its start to reach the part asked. Once it served a range, it is asked again instead, up to 4 times, 0.5 seconds, 1 second, then 2 seconds apart: some hosts answer a range so once in a while, and reading through would hold a seek for minutes.
- Once an addon's file sizes proved right, the size it announces for a file is checked at the first answer: a file more than 1% larger or smaller is another file, which fails at once. Within 1%, the file's own size is used, as some addons' sizes are a little off. An addon whose sizes are wrong more often than right is never used this way.
- The analysis of a version whose source fails stops at once, and the version is skipped for 15 minutes: the next one is tried within seconds. A version whose source fails while it plays is skipped the same way. The log tells what the source answered, never its address. A torrent that did not start in time stays listed instead, and only plays that pick no version skip it (see [When torrents do not start](torrents.md#when-torrents-do-not-start)).
- A source that answers, but not with a video, fails its analysis the same way; an empty answer fails at once. The log line (`A version could not be analyzed`) tells what it answered, so that the cause shows: its status, its type and size as its headers tell them, and what its first bytes are, such as a web page, JSON, XML, text, a playlist, a playlist that does not start with `#EXTM3U`, or an empty body, as in `not a playable video: the source answered a web page (HTTP 200, text/html, 2.1 kB)`. An error status tells the same of what came with it, as in `source unavailable: HTTP 403, a web page (text/html, 2.1 kB)`. Neither tells the address nor the bytes themselves. A page or an error instead of the video most often means that the site wants a header the addon does not declare for the stream (`proxyHeaders`), such as a player's user agent: Polyfin sends `Polyfin/<version>` when the addon gives none.
- A host asking to slow down never has a version skipped: not when the analysis, the keyframe index, the playback or the check before direct play meets it, nor when an analysis runs out of time while its reads waited for the host. The version is tried again at the next play.
- A host that failed to serve a version's keyframe index is asked again at the next play. A failed renewal of a link never counts as the file's failure.

**For app developers:**

- `PlaybackInfo` for a version played over HLS starts reading the bytes of its first segment; with `StartTimeTicks`, those of the segment the app resumes at.

### Carrying a play on

A source that fails for good during a play, past the tries above, need not end it:

- Mirrors: the source cache reads on from the version's next mirror, the same file another addon lists (see [Which streams become versions](#which-streams-become-versions), step 5), from the byte where the read stopped. The blocks already read stay, as the file is the same, and an answer of another size is no mirror and fails in turn. Relayed apps and HLS plays go on without an error; the log says `A source failed: reading on from another copy of the file`, at Info. The version is kept as failed only once its last mirror failed too. A relayed file that has mirrors is always read through the cache, which alone can switch.
- Another version, over HLS: once the version and its mirrors failed, a repackaged or converted HLS play goes on from the next version of the title, in the order the user is offered them (the [version order](#version-order) rules and the user's quality group included), then those before it, from the segment the app waits for. The segments keep their numbers and times; the playlist served after the switch has one `#EXT-X-DISCONTINUITY` before the first segment of the other version. The other version is made into what the app plays: its video converted into the codec, size and frame rate the app received, its audio track the first in the same language, copied when it is alike, else converted into the codec, channels and sample rate the app received. When both versions' speech is known (see [Subtitles](subtitles.md)), its times are moved by how much later its speech comes. The log says `A play went on from another version: its source failed`, and Home's playback **Details** show **Switched version**.
- Limits of the switch: the search for another version takes 40 seconds at most; the app's requests meanwhile wait, and those past 20 seconds are told the server is busy, which apps ask again. A play switches 3 times at most. The video conversion counts toward [Video conversions at once](#video-conversions-at-once): at the cap, there is no switch. The versions passed over: torrents, whose swarm takes longer to start than a player waits; versions that failed lately; MPEG-TS files and discs; versions more than 1% longer or shorter, or of another frame shape (2%), which are another cut; and versions whose tracks cannot be converted into the app's. A play that copies HDR or 10-bit video, a play in fragmented MP4 segments whose video is copied (the app keeps the first version's initialization segment), MPEG-TS files and discs, Live TV, and plays with **Keep the converted video when the audio track changes** on do not switch, and end as before. A picture subtitle of the first version's own file is not burned into the other version, and its embedded text subtitles stop at the switch.
- Direct play cannot be carried on: the app reads the provider's link itself, and Polyfin is not asked again when it fails. Neither can a relayed file without mirrors.

**Compared with Jellyfin:**

- Jellyfin has no mirrors and never switches a play to another version: a source that fails ends the play. Apps keep requesting the same play session's segments, so the switch needs nothing of them.

## Scrubbing thumbnails and chapter images

**Settings › Thumbnails (trickplay)** makes Jellyfin's scrubbing thumbnails and chapter images. Both are off by default, because both read parts of the source.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Thumbnails when moving through a title** | **Settings › Thumbnails (trickplay)** | Off | Makes trickplay thumbnails. |
| **Chapter images** | **Settings › Thumbnails (trickplay)** | Off | Makes chapter images. |
| **One thumbnail every (seconds, at least)** | **Settings › Thumbnails (trickplay)** | 10 seconds (5 to 60) | Shortest time between thumbnails. A version longer than the requests allow at that pace gets longer steps (see [How the images look](#how-the-images-look)). |
| **Thumbnail width** | **Settings › Thumbnails (trickplay)** | 320 (or 240, 480) | Thumbnail width in pixels. |
| **Space for images (GB)** | **Settings › Thumbnails (trickplay)** | 2 GB (1 to 50) | Past this, the versions whose images were used longest ago lose them, and get them again on their next play. |

Turning either setting off hides what was made. It shows again when you turn the setting back on.

### When images are made

Images are made after watching, at a gentle pace:

- once a version has been played, and for the next episode when [playback is prepared in advance](#preparing-playback-in-advance);
- Polyfin waits until that playback has stopped and nobody plays anything from the same host;
- it then reads in the background only keyframes, located through the file's index (Matroska Cues or MP4 sample tables), each with a request for its own bytes, never the whole file;
- one version is made at a time, with a short queue.

### Request limits

- A version takes at most 60 requests, its index included.
- Requests are sent one every 3 seconds, and at most 120 an hour per host, across versions. Past that hour's share, they wait.

### How the images look

- Each thumbnail gets a keyframe of its own: a version whose thumbnails would outnumber the requests left once its index is read gets its thumbnails spread evenly over its runtime, one per request, a whole number of seconds apart. With about 58 requests left, that is every 47 seconds for a 45-minute episode, and every 2 minutes 5 seconds for a 2-hour movie.
- Each thumbnail shows the keyframe nearest its time, a few seconds from it at most, unless the file has keyframes further apart.
- Chapter images show the keyframe nearest each chapter's start: one read for it when the requests left allow one per thumbnail and per chapter, otherwise the nearest of the thumbnails' reads.
- FFmpeg decodes each keyframe on its own as it arrives, tied to its time whatever the order it was read in, on the GPU when there is one, and tone maps HDR.
- Thumbnails are packed into Jellyfin's 10x10 tile JPEGs. Chapter images are at most 640 pixels wide.
- Images made by Polyfin 0.7.0, which could show their keyframes out of order, are dropped on upgrade and made again on the next play.
- Images are kept in the database.

### When a source misbehaves

- A request answered oddly, as some hosts do now and then (the whole file for a range, another range, an answer cut short, or none), is asked once more after a short pause. The retry counts in the budget.
- A link the source says expired is renewed, once per version, then asked again.
- Two odd answers in a row fail the version for 30 minutes. The log tells what the source answered, never its address.
- A host that asks to slow down (`429`) or fails as overloaded (`502`, `503`, `504`) is not asked again: the version stops at once, nothing of it is kept, and the version waits a day. That host gets no image request for 2 hours, while other hosts go on.
- A version that fails otherwise is left alone for 30 minutes, or a day when its file cannot give images (no index, an unsupported codec).

**Compared with Jellyfin:**

- Thumbnails are of the keyframes read, so their timing is approximate (see above).
- Chapters carry no `ImagePath`.
- Turning either setting off hides what was made, which shows again when turned back on.

**For app developers:**

- Apps get images as Jellyfin gives them: the item's `Trickplay` field, by media source and width.
- `Trickplay` is keyed as the item's `MediaSources` name the versions (the first under the title's own identifier), and limited to the versions the user is offered under their quality group.
- Tiles are served at `/Videos/{itemId}/Trickplay/{width}/tiles.m3u8` and `{index}.jpg`, which follow parental control, blocked genres and allowed hours.
- Chapters carry an `ImageTag`, served by `/Items/{itemId}/Images/Chapter/{index}` without credentials, like all images.
