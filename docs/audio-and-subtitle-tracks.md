# Audio and subtitle tracks

This page, for administrators and users, explains which tracks a title plays by default.

See [Subtitles](subtitles.md) for where subtitles come from.

## Default audio and subtitle languages

Each user chooses an audio language, a subtitle language and a subtitle mode in their app. An administrator can choose languages for users who chose none:

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Default audio language** | **Settings › Playback** | None | Audio language of users who chose none. |
| **Default subtitle language** | **Settings › Playback** | None | Subtitle language of users who chose none, shown as their subtitle mode says. |

- Without any language, a user gets the file's flagged audio track, and the subtitles their mode gives, in any language.
- Both list the web player's languages, from `/Localization/Cultures`.
- A user's own choice always wins: each default fills only an empty audio or subtitle language.
- Defaults apply in every app: a title's details, `PlaybackInfo`, and the [version order](versions.md#version-order)'s **Preferred audio language**.
- `/Users/Me` and `/Users/{userId}` keep showing the user's empty language, so an app saving its configuration never adopts the default, and a changed default applies at the next play.

The subtitle mode (`SubtitleMode`) stays the user's:

| Mode | With a default subtitle language |
|---|---|
| `Always` | Full subtitles in it, else forced ones. |
| `Smart` | Subtitles in it when the audio is in another language. |
| `OnlyForced` | Forced subtitles in it or of no stated language. |
| `Default`, `None` | Ignores the language. |

`PlayDefaultAudioTrack`, on for new users until turned off in their app, plays the flagged track whatever its language. Audio order:

| User's audio language | Order |
|---|---|
| None, **Default audio language** set | That language, even with `PlayDefaultAudioTrack` on (the administrator chose it; the app setting was never the user's choice); else the flagged track; else the first. |
| Their own | With `PlayDefaultAudioTrack` on, the flagged track, then their language (Jellyfin's rule, [MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L14-L29)). |

**Compared with Jellyfin**

- Jellyfin 12.4 has no server-wide default, reading only the user's preferences ([MediaSourceManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L515), [L574-L578](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L574-L578)); empty means no language ([L477-L482](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L477-L482)).
- Without a language, Jellyfin plays the flagged audio, else the first ([MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L14-L29)), and any subtitle counts as preferred ([L154-L158](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L154-L158)).
- With both settings at None, the default, Polyfin matches Jellyfin; a default subtitle language acts as the user's own.
- Polyfin's default audio language beats the flagged track; a Jellyfin user's language yields to it while `PlayDefaultAudioTrack` is on.
- jellyfin-web 12.4 also offers "Original language" for audio. Polyfin does not offer it as a default.

## Regional audio

Some French, Spanish and Portuguese dubs differ by region. Each user may prefer one of six variants, by BCP 47 tag, as their **Audio region** (see [Audio region](watch-state.md#audio-region)):

| Variant | Tags that name it | Release words that name it |
|---|---|---|
| French (France), `fr-FR` | `fr-FR`, and `fr-BE`, `fr-CH`, `fr-LU`, `fr-MC`, which hear the same dub | VFF, TRUEFRENCH, VF2 |
| French (Quebec), `fr-CA` | `fr-CA` | VFQ, VF2 |
| Spanish (Spain), `es-ES` | `es-ES` | CASTELLANO |
| Spanish (Latin America), `es-419` | `es-419`, and the tag of any Latin American country or of the United States (`es-MX`, `es-AR`, `es-US`…) | LATINO, LATAM |
| Portuguese (Brazil), `pt-BR` | `pt-BR`, `pob` | DUBLADO, PTBR |
| Portuguese (Portugal), `pt-PT` | `pt-PT`, and the tags of the other Portuguese-speaking countries (`pt-AO`, `pt-MZ`…) | |

- A track's language tag names its variant, by any code (`fr-CA`, `fre-CA`), as Matroska keeps BCP 47 regions.
- Else its title does: a tag (`Portuguese (PT-BR)`), a release word (`VFQ`), or a region in English or the language (`French (Canada)`, `Français (Québec)`, `Spanish (Latin America)`, `Castellano`, `Brazilian`, `French (CA)`). `fre` titled `VFF` is French (France).
- Until RemuxDB or analysis describes a version, its labels name variants and language, as the [version order](versions.md#version-order) reads them: the release words above, two-word tags (`PT-BR`, `ES-MX`, `es-419`), and flags of Mexico, Argentina, Colombia (Latin America) and Brazil.
- Flags of France, Spain and Portugal mean the language at large: releases show them for any dub.

Subtitled releases keep their original audio; their subtitle languages never count as audio:

- Languages right after a subtitle word are the subtitles': Sub, Subs, Subtitles, Subbed, VOST, VOS, VOSE, Legenda, Legendado, Subtitulado, Sottotitoli, Untertitel… (`ENG Sub ITA`, `Audio: English | Subtitles: French, Spanish`).
- With none after, so is the language before Subs, Subbed, Subtitles, Subtitled or Legendado (`French Subs`, `PT-BR Legendado`).
- `MULTi-SUBS` does not mean several audio languages; VOSTFR and SUBFRENCH name none. Title words never count as these.

With an **Audio region**, in the user's audio language:

- The default audio track is the region's, else no known region's, else another region's. The flagged track still leads for users with their own language and `PlayDefaultAudioTrack` on, as in Jellyfin.
- With **Preferred audio language** in **Order versions by**, versions rank: the language in the region; with no known region; only in another region; MULTI or DUAL; unknown language; the others (see [Version order](versions.md#version-order)).
- The region applies only to its language: a user who chose English keeps English first. A user with no audio language, own or server's, gets the region's, ranked above the flagged track.
- A track picked in the region's language on a series or movie ranks by region there too (see [Remembered track choices](#remembered-track-choices)).

**Compared with Jellyfin**

- Jellyfin 12.4 matches a track's tag to the user's language as a whole string, any case ([MediaStreamSelector.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaStreamSelector.cs#L181-L190)): no region, `fr-CA` misses `fre`, two French tracks tie. Polyfin matches any code, with a separate region setting.
- Jellyfin reads no language from release names; Polyfin does, for the version order.

## Remembered track choices

When a user picks audio or subtitles other than the default, Polyfin remembers it for the series (all episodes) or movie, on later plays in every app.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Set audio track based on previous item** (`RememberAudioSelections`) | Web player playback settings | On | Remembers audio. |
| **Set subtitle track based on previous item** (`RememberSubtitleSelections`) | Web player playback settings | On | Remembers subtitles. |

- Turning one off stops its choices at once; the next playback report forgets them, as in Jellyfin.
- A choice is the language, never the track number, which differs across versions and episodes; untagged tracks are not remembered. Subtitles keep forced and hearing-impaired flags; "no subtitles" counts.
- A regional variant is remembered only when it differs from the user's **Audio region** in that language, such as France against **French (Quebec)** (see [Regional audio](#regional-audio)).
- Polyfin learns choices from track changes in `POST /Sessions/Playing` and `POST /Sessions/Playing/Progress`: a non-default track becomes the choice, the one the user's languages play anyway forgets it. A picture subtitle's text copy counts as the original.
- Later, the choice outranks the user's languages and server defaults in a title's details and `PlaybackInfo`, but a track the app names there wins.
- Audio in the chosen language ranks: the remembered variant, else the **Audio region** of that language; no known region; another region. The flagged track breaks ties, or leads with neither.
- Subtitles equally forced come first; hearing-impaired first if chosen so.
- Choices leave the version order alone (see [Version order](versions.md#version-order)). A version lacking the track plays by preferences; the choice stays for next episodes.
- The subtitle mode still applies: `None` shows no remembered subtitle; `OnlyForced` only a remembered forced track, or none.
- **My account › Track choices** counts series and movies with a choice; **Forget my track choices** or **Reset my preferences** clears them (see [Track choices](watch-state.md#track-choices)).

**Compared with Jellyfin**

- Jellyfin 12.4 stores a track number for that item alone, in the user's item data ([SessionManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Session/SessionManager.cs#L1016-L1055)), reused if the source has it ([MediaSourceManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L493-L543)), so it misses next episodes and other versions. Polyfin's language choice reaches both.
- Jellyfin keeps every reported track, defaults included, so language changes miss items played once. Polyfin keeps only non-default ones.
- With these settings on, jellyfin-web matches the next queued item's tracks to the last and names them in `PlaybackInfo`, so they win.
