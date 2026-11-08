# Watch state and preferences

This page is for users. It covers your watch state, Continue Watching, Next Up, and your preferred language and dub.

## Watch state

Polyfin keeps each user's played titles, resume points, favorites and ratings, across all their Jellyfin apps.

- Playback moves the resume point, and marks a title played near its end, with Jellyfin's thresholds (see [Played and resume thresholds](#played-and-resume-thresholds)).
- **Continue Watching** lists titles under way; **Next Up**, the next episode of each series watched (see [Continue Watching and Next Up](#continue-watching-and-next-up)).
- The **Upcoming** row lists coming episodes of series watched or marked favorite.
- Next Up and Upcoming look at the 50 series watched last, as each series asks its addon for episodes. An imported history can hold hundreds (see [Importing your watch history](tracking-imports.md#importing-your-watch-history)).
- Marking a series or season played marks its released episodes.
- Apps keeping Jellyfin's live connection (WebSocket) open hear of changes from the user's other apps at once.

Users can make playlists of movies and episodes from their apps: private, shared with other users, or open to all (see [Playlists](#playlists)).

These actions take a title off the user's lists and change nothing else:

| Action | Takes off | Undone by |
|---|---|---|
| "Remove from Continue Watching" | A title, from Continue Watching | Playing it again, or an import with a resume point set after the removal |
| "Remove from Next Up" | A series, from Next Up | See [Continue Watching and Next Up](#continue-watching-and-next-up) |
| "Hide this title" | A movie, home video, series or folder of videos | "Show this title again" |

- A title removed from Continue Watching keeps its played state, plays and last played date. Tracking services are not told.
- Polyfin remembers the removal date for imports from tracking services and apps (see [How the history merges with Polyfin's](tracking-imports.md#how-the-history-merges-with-polyfin-s)).
- A hidden title leaves libraries, home rows (Latest, Continue Watching, Next Up, Upcoming), searches, similar titles, recommendations, person pages and collections. It stays in the Watchlist playlist, which leaves it out of what it lists.
- A hidden series hides its episodes; a hidden folder, its videos.
- A hidden title still opens and plays from a link, and its page still lists its seasons and episodes. Other users are not affected.

The web player has these actions in its ⋮ menus (see [Web player](web-client.md#dashboard-links-and-single-sign-on)). For other apps, use **My account** in the admin app:

| Section of **My account** | Lists | Button |
|---|---|---|
| **Continue Watching** | Each title | **Remove** |
| **Next Up** | Each episode | **Remove** |
| **Next Up**, below the episodes | Each series taken off | **Show again** |
| **Hidden titles** | Each hidden title | **Show again** |

Other apps see the change at their next refresh.

### Continue Watching and Next Up

**Continue Watching** lists movies, episodes and videos stopped part way, last played first.

- The stop must fall between **Resume point kept after (%)** and **Marked played after (%)**: 5% and 90% by default (see [Played and resume thresholds](#played-and-resume-thresholds)).
- So a 45-minute episode stopped in its first 2 minutes 15 seconds does not show.
- The resume point is the movie's or episode's, whatever version plays, a torrent's or another.

**Next Up** lists one episode per series watched, or two for a series being rewatched.

- A series counts as watched once any episode was played or started, even before the resume threshold.
- It shows the first released, unplayed episode after the furthest played, specials left out; else the first episode.
- An episode under way goes only to apps asking for one. The web player's home does not ask: the episode shows in Continue Watching instead.
- Series are ordered by when their furthest episode was played, latest first. Series with nothing played follow, latest started first.
- "Remove from Continue Watching" on an episode only clears its resume point. If it is next, it still shows in Next Up, web player included.

The web player's Display settings page has two Next Up settings:

| Setting | Default | What it does |
|---|---|---|
| "Max days in 'Next Up'" | A year | The home leaves out series not watched for longer. |
| "Enable Rewatching in Next Up" | Off | A series replayed from an earlier episode also shows the episode after the one played last, if that was played before. |

- A series finished and restarted from episode 1 shows episode 2.
- A series rewatched before its end shows twice: its next unplayed episode and the rewatch's next. Each is placed by when its previous episode was played.
- A rewatch episode under way is left out.
- The rewatching setting is saved per user in the browser only, as jellyfin-web does. The home and Next Up page send it; other apps may have their own.

To keep Continue Watching alone, in the web player, open **Settings › Home** and set the section showing **Next Up** (the sixth by default) to **None**.

- This is saved in the account, so it follows the user across browsers. Other apps have their own home settings.
- Rows the administrator placed **After Next Up** move up into its place (see [Home rows](home-rows.md#home-rows)).

**Remove from Next Up** (⋮ menu of a Next Up card in the web player) or **Remove** in **My account › Next Up** takes a series off Next Up. It covers whatever episode shows, and both entries of a rewatch. Every app's Next Up, DLNA's too, leaves the series out until:

- an episode is played after the removal: a play, a mark played dated after it, or an imported later play. jellyfin-web's ✓ dates the mark now; an undated mark dates only a never-played episode. An older imported play keeps it off;
- an episode comes out after the removal, its date known and passed; specials do not count;
- or the user picks **Show again** under **My account › Next Up**, which lists the series still off.

While it is off:

- Asked for that series alone, Next Up still answers, so its page's **Next Up** and playing it from its page work.
- Continue Watching and Upcoming do not change.
- Removing it again counts from the new removal.
- Tracking services are not told. Deleting the user deletes their removals.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Remove from Continue Watching also takes the series off Next Up** | **My account › Next Up** | Off | "Remove from Continue Watching" on an episode, season or series also takes its series off Next Up. **Reset my preferences** turns it off. |

The web player's home keeps its rows for a minute.

- jellyfin-web 12.4 reuses the Continue Watching and Next Up lists for up to a minute after last asking, even across reloads.
- Polyfin's script makes these rows ask again at once when an item's played state or resume point changes, and on each refresh for a minute after.
- So a card marked played with its ✓ gives way to the next episode within seconds (see [Web player](web-client.md#dashboard-links-and-single-sign-on)).
- A change in another app does the same, without a reload, once the web player hears it on its live connection. Other apps show it at their next refresh.
- A card removed with "Remove from Continue Watching" or "Remove from Next Up" leaves the row at once and stays off.

**Compared with Jellyfin**

- Next Up follows Jellyfin 12.4. A series is watched once an episode has a date, played or only started ([NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L42-L70)). A series with nothing played comes last, from its first unplayed episode ([NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L204-L247), [TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L145-L151)). An episode under way is left out when the app asks ([TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L227-L237)).
- The web player keeps its rows for a minute on Jellyfin too: queries stay fresh a minute ([queryClient.ts](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/utils/query/queryClient.ts#L12-L50)), and server announcements do not reach them ([QueryClientEventHandler.tsx](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/QueryClientEventHandler.tsx#L20)). Reported upstream as jellyfin/jellyfin-web#8543. Asking again after a played change is Polyfin's own. On Jellyfin, a card marked played stays until the home redraws over a minute after last asking.
- "Enable Rewatching in Next Up" (`enableRewatching`) follows Jellyfin 12.4, off unless the app sends it ([TvShowsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L90-L91)). The episode played last has the latest played date. The rewatch's next is the first played, released episode after it, specials left out ([NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L119-L156), [NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L249-L306)). It is left out when under way, dated with the previous play, and listed beside the next unplayed episode ([TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L154-L176), [TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L242-L248)). It applies to `seriesId` requests too, as in Jellyfin. DLNA's Next Up and next-episode preloading leave it off.
- Jellyfin 12.4 takes a series off Next Up only through its watch state: marked played, or every played or started episode marked unplayed ([TVSeriesManager.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/TV/TVSeriesManager.cs#L71-L171), [NextUpService.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/NextUpService.cs#L42-L70)). Remove from Next Up, its routes `DELETE` and `POST /Polyfin/UserItems/{itemId}/NextUp`, and its tie to Remove from Continue Watching are Polyfin's own.

### Track choices

**My account › Track choices** shows how many series and movies have remembered audio and subtitle tracks. Next plays use them by default in every app (see [Remembered track choices](audio-and-subtitle-tracks.md#remembered-track-choices)).

- **Forget my track choices** forgets them all, after asking. Titles then play the tracks the user's language preferences choose.
- **Reset my preferences** forgets them too (see [Your own preferences](users.md#your-own-preferences)).

API details: [Track choices](jellyfin-api.md#track-choices-for-apps).

### Played and resume thresholds

Two settings decide when a title counts as played and when a resume point is kept.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Marked played after (%)** | **Settings › Content** | 90 (from 50 to 100) | A position past this marks the title played. |
| **Resume point kept after (%)** | **Settings › Content** | 5 (from 0 to 50) | A position before this keeps no resume point. |

- The resume threshold must be below the played one, or Polyfin refuses it.
- A title under 5 minutes is marked played once past the resume threshold, as in Jellyfin.

**Compared with Jellyfin**

- These replace Jellyfin's `MaxResumePct` and `MinResumePct`, with the same defaults, 90 and 5.
- Jellyfin takes any values; Polyfin keeps them in the ranges above.

### Playlists

Users make playlists of movies and episodes in their apps, with "Add to playlist", and change them there: rename, reorder, remove, delete. A playlist is private, shared with other users to view or to edit, or open to all with "Allow public access". Each user also has a [Watchlist playlist](tracking-imports.md#the-watchlist-playlist), which their lists fill.

- **A series added:** its aired episodes of regular seasons, in order. Specials and episodes not aired yet are left out.
- **A season added:** its aired episodes, in order.
- **Later episodes:** an episode that airs after its series or season was added is not added.
- **Other items:** a movie or an episode is added itself.
- **Order:** apps listing a playlist get its order. Asked for a sort (`SortBy`), as for its items in a folder view, they get that sort, and **Shuffle** gets a random order, a new one each time.
- **Image:** each playlist gets a mosaic of its first entries, a series' poster standing for its episodes (see [Generated images](artwork-and-editing.md#generated-images)).

## Metadata language

**Metadata language** is the language a user reads titles in: names, overviews and images, episodes' included.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Metadata language** | Its own section in **My account**; for administrators, its own panel on the user's page under **Users** | **The server's** | **The server's** shows the addons' descriptions without a language. The other choices are the languages the addons were given (see [Languages](addons.md#languages)). |

- **Reset my preferences** puts back **The server's** (see [Your own preferences](users.md#your-own-preferences)).
- Users under parental control or blocking genres get their language too.
- Genres, ratings, people, versions and subtitles are the same in every language. So parental control, blocked genres, hidden titles and smart lists judge every user alike.
- A title no addon describes in the language shows the server's description.
- When Polyfin speaks the language, its generated names follow it (see [Language of generated names](getting-started.md#language-of-generated-names)).
- An invite link's model user passes on their metadata language.
- A shared link's page shows the title's name, overview, poster and backdrop in the sharer's language (see [Share links](invite-and-share-links.md#share-links)).
- **Edit images** lists TMDB's images in the language first (see [Edit images](artwork-and-editing.md#edit-images)).

API details: [Metadata language](jellyfin-api.md#metadata-language-for-apps).

## Audio region

**Audio region** is the dub a user prefers when a language has regional dubs.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Audio region** | Its own section in **My account**; for administrators, its own panel on the user's page under **Users** | **None** (no region) | **French (France)** or **French (Quebec)**, **Spanish (Spain)** or **Spanish (Latin America)**, **Portuguese (Brazil)** or **Portuguese (Portugal)**. |

- **Reset my preferences** puts back **None** (see [Your own preferences](users.md#your-own-preferences)).
- It refines the audio language, when that is the region's language. The audio language is the one chosen in the app, else the server's **Default audio language**.
- French with Quebec gets the Quebec dub first. English stays English, whatever the region.
- With no audio language of their own and no server default, the user gets the region's language.
- In that language, the region's track plays by default, then one of no known region, then another region's.
- With **Preferred audio language** in the version order, versions with that dub come first among those in the language.
- How Polyfin reads a track's or release's region: [Regional audio](audio-and-subtitle-tracks.md#regional-audio).
- An invite link's model user does not pass it on: it is the guest's own choice.

API details: [Audio region](jellyfin-api.md#audio-region-for-apps).
