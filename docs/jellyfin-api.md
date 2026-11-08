# Jellyfin API notes

This page is for developers of apps that talk to Polyfin, and for anyone scripting its admin app. Polyfin answers the Jellyfin 12.4 API, so a Jellyfin app works unchanged. The notes below list where Polyfin answers differently, and the routes it adds.

- **Routes:** Jellyfin's own routes keep their Jellyfin names. Polyfin's additions live under `/Polyfin/`, and the admin app's under `/admin/api/`.
- **Order:** each section follows the user page that links to it. That page explains the feature; this one, how apps see it.
- **Comparisons:** a **Compared with Jellyfin** block names the Jellyfin 12.4 source it was checked against.

## Signing in and finding servers

Apps sign in with a name and password, or with Quick Connect, as on Jellyfin. These notes cover two-step sign-in, API keys, local network discovery and the client address behind a proxy.

### Quick Connect for two-step accounts

- `/Users/AuthenticateByName` answers 401 for such an account, as for a wrong password; `/Users/AuthenticateWithQuickConnect` signs it in once a request is approved.

### Two-step sign-in for apps

- `POST /admin/api/session` answers `{"user": …}` without a second step, `{"twoStep": "code"}`, or `{"twoStep": "setup", "setup": {"secret", "uri", "qr"}}` for an administrator setting it up, with an HttpOnly cookie for the step. `POST /admin/api/session/two-step` with `{"code"}` then opens the session (`invalid_code`, or 401 `two_step_expired`), with `recoveryCodes` after a setup.
- `GET /admin/api/account/two-step` answers `on`, `recoveryCodes` (those left), `unreadable` and `required`. `POST …/two-step/setup` answers the key, its `otpauth://` address and its QR code as rows of `0` and `1`; `POST …/two-step/confirm` with `{"code"}` turns it on and answers the recovery codes. `POST …/two-step/recovery-codes` and `POST …/two-step/off` take `{"code"}` or `{"password"}`.
- `POST /admin/api/users/{id}/two-step/off`, for administrators, answers the user, whose `twoStep` tells whether it is on.
- The codes follow RFC 6238: HMAC-SHA-1, 30-second steps, 6 digits, a 160-bit key.
- A recovery code holds 80 random bits, in lowercase Crockford base32. Polyfin stores its SHA-256 hash: at 80 bits, a copy of the database does not let anyone search it out.

### Wrong password blocking for apps

- Jellyfin apps read the limit (-1 when there is none) and the count in the user's policy, as `LoginAttemptsBeforeLockout` and `InvalidLoginAttemptCount`.
- The limit is the server's, so a policy an app posts does not change it, nor the count.

### Identity provider sign-in for apps

- `GET /admin/api/status` answers `provider`, `{"name", "start"}`, or `null` without a provider or a public address. `start` is `<public address>/admin/api/provider/start`, to which the admin app adds `?app=admin` and the web player `?app=web`.
- `GET /admin/api/provider/callback` is the redirect URI. It sends the browser to `/admin/`, signed in, or `/admin/?provider=<error>`; for the web player, to `/web/?polyfinProvider=signed-in` or `/web/?polyfinProvider=<error>`. The errors are `provider_refused`, `provider_no_account`, `account_disabled`, `outside_allowed_hours`, `outside_local_network`, `too_many_attempts`, `provider_state`, `provider_failed`, `provider_off`, `provider_unreachable` and `internal`.
- `POST /admin/api/provider/web`, with the ticket's cookie and the device in a `MediaBrowser` `Authorization` header (`Client`, `Device`, `DeviceId`, `Version`), signs that device in once and answers `{"accessToken", "userId", "serverId", "serverName"}`; 401 `invalid_credentials` without a valid ticket, 403 `account_disabled`, `outside_local_network` or `outside_allowed_hours`.
- A user's `providerOnly` tells whether they sign in through the provider only; `POST /admin/api/users` and `PATCH /admin/api/users/{id}` take it.

### API keys in the Jellyfin API

- Keys are managed through `/Auth/Keys`.
- Send the key as the token of the `Authorization` header, or as the `ApiKey` query parameter. `api_key` and `X-Emby-Token` work only with legacy authorization.
- A user is named with `userId` or `/Users/{id}/…`. Without one, `/Users/Me` answers 400, as in Jellyfin.
- `/Auth/Keys` lists a key made from a Jellyfin app in full only on the next listing, then its identifier. `DELETE /Auth/Keys/{key}` accepts the identifier as well as the key.

### Local network discovery for apps

- Polyfin listens on UDP port 7359, IPv4 and IPv6. A datagram that holds `who is JellyfinServer?`, in any letter case, as Jellyfin 12.4 reads it, gets one answer, sent to its sender: `{"Address":"http://192.168.1.10:8096","Id":"<server ID>","Name":"<server name>","EndpointAddress":null}`. Polyfin answers Emby's `who is EmbyServer?` the same way, which Jellyfin 12.4 leaves unanswered.
- `Id` and `Name` are those of `/System/Info/Public`. `Address` is **Address on the local network** when set, as Jellyfin's `PublishedServerUrl`. Otherwise it is `http://`, the address of the server's interface that reaches the sender, and Polyfin's HTTP port.
- Only senders on the local network are answered: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback. Jellyfin answers any sender.
- `LocalAddress` of `/System/Info/Public` and `/System/Info` is **Address on the local network** when set, as Jellyfin gives its `PublishedServerUrl` there, and otherwise the address the request reached: over the HTTPS Polyfin serves itself, `https://` and the HTTPS port included (see [HTTPS](https-and-networking.md#https)).
- While Polyfin serves HTTPS, discovery answers keep the HTTP address: a certificate for a public name does not match the server's address on the local network, and HTTP is never redirected there. Jellyfin, too, announces its HTTP address unless `PublishedServerUrl` says otherwise. To announce HTTPS, set **Address on the local network** to an `https://` address whose name the certificate holds.

### Client addresses for apps

- A session's `RemoteEndPoint`, and `IsLocal` and `IsInNetwork` of `/System/Endpoint`, follow the client's address: the one the proxy names for a request from a known proxy, the connection's otherwise.

### Remote access for apps

- jellyfin-web's user editor shows "Allow remote connections to this server" only when the server's network configuration allows remote access. `GET /System/Configuration/network` answers Jellyfin's `NetworkConfiguration` with `EnableRemoteAccess` true, the server's port, local discovery and Known proxies, and Jellyfin's defaults for the rest.

### Admin app sign-in from the web player for apps

- The token goes to `POST /admin/api/session/jellyfin`, in the `Authorization` header only, never in the URL.
- The web player's links to the admin app carry `?from=web`, which the admin app takes out of the address as it opens.

### Admin app inside the Jellyfin mobile apps

- The admin app tells it runs in a Jellyfin app by the objects the app puts on every page of its web view: `window.NativeInterface` in the Android app, `window.ExpoAppInfo` in the iOS app. Neither app names itself in its user agent.
- In those apps, it sets what they call on the page: `window.NavigationHelper.goBack()`, which the Android app calls on **Back** ([WebappFunctionChannel.kt, line 25](https://github.com/jellyfin/jellyfin-android/blob/v2.7.3/app/src/main/java/org/jellyfin/mobile/webapp/WebappFunctionChannel.kt#L25)), and `window.Emby.Page` with `goHome()`, `back()` and `canGoBack()`, which the iOS app calls on its **Home** tab ([ExpoRouterShim.staticjs, lines 6 to 31](https://github.com/jellyfin/jellyfin-expo/blob/v1.8.0.5/assets/js/ExpoRouterShim.staticjs#L6-L31)). On the web player's pages, jellyfin-web and the Android app's own plugin set them.
- Since version 2.7.3, the Android app hands any new tab to the system browser, the server's own pages included ([JellyfinWebChromeClient.kt, lines 40 to 67](https://github.com/jellyfin/jellyfin-android/blob/v2.7.3/app/src/main/java/org/jellyfin/mobile/webapp/JellyfinWebChromeClient.kt#L40-L67)), so links that must stay in the app open in place. The iOS app loads a new tab in its own view.
- A tab that came from the web player keeps `polyfin.cameFromWebClient` in its session storage.

## Users and permissions

Administrators manage users in the admin app, under `/admin/api/users` and `/admin/api/groups`. Jellyfin apps read and set the same limits through the user policy.

### User groups for apps

- `GET /admin/api/groups` lists the groups, sorted by name: `id`, `name` and `memberCount`.
- `POST /admin/api/groups` creates one from `name` and `copyFrom`, a user's `id`, or null or left out for a new user's settings; it answers 201 with the group. `GET /admin/api/groups/{id}` answers the group: `id`, `name`, `memberCount`, and the fields of a user's JSON the group holds: `transcoding`, `downloads`, `personalAddons`, `collectionManagement`, `subtitleManagement`, `canShare`, `maxPlaybacks`, `maxBitrate`, `qualityGroup`, `localFirst`, `syncPlay`, `liveTv`, `remoteControl`, `liveTvManagement`, `parentalControl`, `hiddenLibraries`, `blockedGenres` and `accessSchedules`.
- `PATCH /admin/api/groups/{id}` takes `name` and any of those fields, as `PATCH /admin/api/users/{id}` takes them, with the same errors, and answers the group. `DELETE /admin/api/groups/{id}` deletes it (204).
- `GET /admin/api/groups/{id}/members` lists the members, sorted by name: `id`, `name` and `followGroup`. `POST /admin/api/groups/{id}/members` with `userIds` adds those users, all of them or none, and answers the members.
- A user's JSON has `groupId` and `groupName`, null for none, and `followGroup`. `PATCH /admin/api/users/{id}` takes `groupId`, a group's `id` or `""` for none, and `followGroup`. A field the group holds, sent for a member who follows it without `followGroup` false in the same change, is 409 `follows_group`. `POST /admin/api/users` and `POST /admin/api/invites` take `groupId`.
- Errors: 409 `admin_cannot_join_group` for an administrator joining, or added, or created in a group; 400 `invalid_group` for a group that does not exist; 400 `invalid_name` and 409 `name_taken` for a group's name, from 1 to 64 characters, unique without regard to case; 400 `invalid_copy_from` for a user to copy who does not exist; 404 `not_found` for a group or a user to add that does not exist.

### Resetting to defaults for apps

- `GET /admin/api/users/{id}/defaults` answers the fields of a user's JSON a group holds (see [User groups](users.md#user-groups)) as **Reset to defaults** would set them, changing nothing. `POST /admin/api/users/{id}/reset` sets them and answers the user. Both are 409 `follows_group` for a member who follows their group, and 404 `not_found` for a user who does not exist.
- `GET /admin/api/groups/{id}/defaults` and `POST /admin/api/groups/{id}/reset` do the same for a group; the reset answers the group, and is 404 `not_found` for a group that does not exist.
- `POST /admin/api/account/preferences/reset`, for any user signed in to the admin app, resets their own preferences and answers 204. Their Jellyfin apps then read a new user's `Configuration` in `GET /Users/{id}` and `GET /Users/Me`, and `GET /DisplayPreferences/{id}` answers the defaults it gives a user who never saved any.

### Account end dates for apps

- `GET /admin/api/users` and `PATCH /admin/api/users/{id}` answer `endsOn`, a `YYYY-MM-DD` date in the server's time zone or null for never, and `ended`, true once that date has come. `PATCH` takes `endsOn`, a date or null; anything else is 400 `invalid_ends_on`, and a date for an administrator, or for a user made one in the same change, is 409 `administrator_cannot_end`.
- An ended account signs in as a disabled one does: 403 from `/Users/AuthenticateByName`, Quick Connect and `POST /admin/api/session` (`account_disabled`), and `denied` to a Silo TV's poll. Its access tokens answer 401, and its policy's `IsDisabled` is true.

### Visible libraries for apps

- Apps read and set this as the policy's `EnableAllFolders` and `EnabledFolders`.
- Jellyfin keeps the libraries shown, while Polyfin keeps those hidden. That way a library added later shows even when an app chose the libraries.

### Live TV access for apps

- The `/LiveTv` routes answer 403, as Jellyfin's do without `EnableLiveTvAccess`.

### Live TV channels per user for apps

- `GET /admin/api/live-tv/channel-choices`, for administrators, answers every group (`key`, `name`, `source`, `channels`) and a page of channels (`key`, `name`, `number`, `group`), with `total`; `q` searches channels by name or number, `group` keeps one group's, repeated `key` the channels named, and `offset` and `limit` (100, at most 500) page them.
- The user's and the group's JSON hold `liveTvChannels`: `mode` (`all`, `only` or `except`), `groups`, `channels`, `hideNewGroups` and `knownGroups`. `PATCH` takes the first four; saving with `hideNewGroups` sets `knownGroups` to the groups the server's sources have then, and had before. A rule that is not valid, or lists more than 10,000 groups or channels, is 400 `invalid_live_tv_channels`.

### Quality groups for apps

- PlaybackInfo reports the conversion of a taller version as Jellyfin reports a resolution limit (`VideoResolutionNotSupported`), describing its video at the size sent.

### Local files first per user for apps

- The user's JSON in `/admin/api/users` and a group's hold `localFirst`, which `PATCH` takes.

### Conversion permissions for apps

- With conversion off, PlaybackInfo picks the next version that plays without conversion.
- Without the user's **Can download** permission, `/Items/{id}/Download` answers 403.
- Administrators' Jellyfin apps set the same permissions through the user policy (`EnableVideoPlaybackTranscoding`, `EnableAudioPlaybackTranscoding`, `EnableContentDownloading`), where video and audio conversion can be allowed separately.
- The policy shows the user's own permissions, whatever the server's switches.

### Profile pictures for apps

- `/UserImage`, and the older `/Users/{id}/Images/{type}`.

### Metadata language for apps

- Jellyfin sets a metadata language per library, and its apps send none per user: Polyfin reads the user's own, and the server configuration's `PreferredMetadataLanguage` stays `en`.
- Images come without a user. An image asked for with the tag of a user's language gets that image.

### Audio region for apps

- The user's JSON in `/admin/api/users` holds `audioRegion`, a BCP 47 tag (`fr-CA`, `es-419`, `pt-BR`) or empty, and `audioRegions`, the tags offered; `PATCH /admin/api/users/{id}` takes `audioRegion`.
- `GET /admin/api/account/audio-region` answers the caller's own as `{"region": "fr-CA", "regions": [...]}`, and `PUT` with `{"region": "..."}` changes it. A tag not offered is 400 `invalid_audio_region`.
- Jellyfin's `AudioLanguagePreference` has no region, and Jellyfin apps neither show nor send this setting.

### Track choices for apps

- `GET /admin/api/account/track-choices` answers `{"titles": n}`, the number of series and movies with a choice of the user signed in to the admin app.
- `DELETE /admin/api/account/track-choices` forgets them, answering 204.

### Web player theme per user for apps

- A user's JSON in the admin API has `webTheme`, `lumaaglaass`, `none` or `custom`, empty for the server's, and `webThemeLocked`, `true` while **Let them change it** is off; `PATCH /admin/api/users/{id}` sets them. A theme not offered is `400 invalid_web_theme`.
- `GET /admin/api/web-themes`, for any user signed in to the admin app, answers `serverTheme` and `themes`, those a user may be given: `lumaaglaass` and `none`, and `custom` while the Custom theme has a text.
- `GET /admin/api/account/web-theme` answers the caller's `webTheme`, `locked`, and the same `serverTheme` and `themes`; `PUT` with `{"webTheme": "none"}` changes it, `403 web_theme_locked` while **Let them change it** is off.

## Moving to Polyfin

Imports read another server or app with that side's own API, and report through the admin API. These notes list both the routes and the requests Polyfin sends.

### Server import for apps

- The admin API serves the import to administrators:
  - `POST /admin/api/jellyfin-import/users` lists a server's users, given `kind` (`jellyfin`, the default, `emby` or `plex`), `address`, and `apiKey`, or `account` with `name` and `password`. It answers `server` with its `kind`, and `keyOwner`, the user the key belongs to or who signed in, `null` for a server's key. An account the server refuses is `jellyfin_sign_in_refused`, and one it does not let sign in `jellyfin_sign_in_forbidden`.
  - `POST /admin/api/jellyfin-import` takes the same, creates the accounts and starts the import. An entry's `jellyfinPassword`, given with `watchData`, has the import sign in as that user. Connected as a user, another user's watch data needs it: without it, the answer is `jellyfin_key_owner_only`. A password the server refuses is `jellyfin_password_refused`, an account it does not let sign in `jellyfin_sign_in_forbidden`, and a sign-in that opens another user's session `jellyfin_other_user`. Each names the `jellyfinId`, and comes before anyone is created. `ratings`, `collections` and `playlists`, each `true` when left out, choose the extras (see [Ratings, collections and playlists](moving-to-polyfin.md#ratings-collections-and-playlists)).
  - `GET /admin/api/jellyfin-import` reads the import, and `POST /admin/api/jellyfin-import/stop` stops it. The import names its `server.kind`, `startedBy` (`id` and `name`), `own`, set for a user's import of their own watch history, and `extras` (`ratings`, `collections`, `playlists`), what it brings. Each user names what was read: `read` (the items with watch data, and the titles of the collections and playlists), for Plex `sections`, `listed` and `history` (`null` for Jellyfin and Emby), and `matched`; what was done: `played`, `resumed`, `favorites`, `ratings`, `watchlist`, `collections` and `playlists` (those created), `listTitles` (the titles added to collections and playlists an earlier import brought), `upToDate` (the marks, resume points, favorites and ratings Polyfin already had, and the collections and playlists an earlier import brought that held every title found) and `undated` (the resume points without a date, not added); `unmatchedCount`, the titles not found, kept under the source `jellyfin`, `emby` or `plex` of the import-titles routes (see "Titles an import did not find") with the `reason` `no_identifier`, `not_found`, `not_in_library` or `other_episode_order`; `skipped`, the collections and playlists not created, each with its `kind` (`collection` or `playlist`) and `name`; `problem`; and `nothingAdded`, `null` unless the user's import ended without a problem and added nothing: `up_to_date`, `nothing_played`, `no_sections`, `plex_owner_nothing_watched` or `plex_no_history`.
  - While any import runs, starting another is `jellyfin_import_running` (409).
  - The key and passwords are never answered back.
- Polyfin reads Jellyfin with `GET` requests, sending the key or session token as `Authorization: MediaBrowser Token="…"`: `/System/Info/Public`, `/Users`, `/Users/Me` (which Jellyfin answers a server's key with an error, and a user's key with that user), and `/Users/{id}/Items` with `Recursive`, `IncludeItemTypes`, `Filters` (`IsPlayed`, `IsResumable`, `IsFavorite`, and `Likes` for the ratings) and `Fields=ProviderIds`, then the episodes' series by `Ids`. For the collections, it lists `IncludeItemTypes=BoxSet`, then each one's titles with `ParentId`; for the playlists, `IncludeItemTypes=Playlist`, then each one's entries with `GET /Playlists/{id}/Items?UserId=…`, as that user sees them.
- It signs in with `POST /Users/AuthenticateByName`, naming itself and a device of its own in the `Authorization` header as Jellyfin's apps do, and signs out with `POST /Sessions/Logout`.

### Emby and Plex import for apps

- The import's API is Jellyfin's, with `kind` set to `emby` or `plex`. Plex takes its owner's token as `apiKey`, and no `account` nor `jellyfinPassword`. With the owner signed in to Plex, `plexOwner` `true` reads each account with its own token, and an entry's `plexPin` is a Home profile's PIN; a PIN Plex refuses is `plex_profile_pin_refused` with the entry's `jellyfinId`, and an account that does not own the server `plex_not_owner`.
- `POST /admin/api/jellyfin-import/plex/sign-in`, `POST /admin/api/jellyfin-import/plex/sign-in/poll` and `DELETE /admin/api/jellyfin-import/plex/sign-in` sign the administrator in to Plex as a user's own sign-in does (see [Plex, signing in](moving-to-polyfin.md#plex-signing-in)), whether or not users may import from another server. `POST /admin/api/jellyfin-import/plex/owner` takes the connection (`kind` `plex`, `address`, `apiKey`) and answers `accounts`, each account but the owner's by its identifier: `all`, `pin` (a Home profile with a PIN: everything with it, plays only without) or `plays`. The import takes the sign-in over: once it starts, the sign-in has ended.
- With the owner signed in, Polyfin also asks plex.tv `GET /api/v2/resources?includeHttps=1&includeRelay=1` with the account's token, to find the server among those it owns; `GET /api/servers/{machineIdentifier}/shared_servers`, in XML, whose `SharedServer` entries give each account's `userID`, the identifier `/accounts` lists it by, and its `accessToken`; `GET /api/v2/home/users`, whose `id` is that same identifier; and, for a profile, `POST /api/v2/home/users/{uuid}/switch?pin=…`, then `GET /api/v2/resources` with the profile's token for its own token for the server. An account read with its own token is read as a user's own import reads it, never `/accounts` nor the history.
- Emby is read under `/emby` when the address does not end with it, else at the address given, with the key in `X-Emby-Token`. The requests are Jellyfin's, but for `/Users/Me`, which Emby does not have: a key is taken only when `GET /Auth/Keys` lists it among the server's API keys. Lists ask for `Fields=ProviderIds,UserDataPlayCount,UserDataLastPlayedDate`, without which Emby leaves the play count and date out. Signing in sends the app and device in `X-Emby-Authorization`.
- Plex is read in JSON (`Accept: application/json`), with the token in `X-Plex-Token` and Polyfin's device in `X-Plex-Client-Identifier`: `/identity`, `/`, `/accounts` (account 1 is the owner, account 0 nobody), `/library/sections`, each movie and show section's `/library/sections/{key}/all` with `type=1` or `type=2` and `includeGuids=1`, the show sections' episodes with `type=4` for the owner, and `/status/sessions/history/all?accountID=…&sort=viewedAt:desc` for the other accounts, a page at a time with `X-Plex-Container-Start` and `X-Plex-Container-Size`. A history row whose `accountID` is not the one asked is left out, and one without a `ratingKey`, a title deleted from the libraries, is listed as `not_in_library` once per title. The collections come from each movie and show section's `/library/sections/{key}/collections`, then `/library/collections/{ratingKey}/children`; the playlists of an account read with its own token from `/playlists?playlistType=video`, then `/playlists/{ratingKey}/items`. A collection's or playlist's title that no library section lists is `not_in_library`.

### Own watch history import for apps

- `GET /admin/api/account/server-import` answers `enabled`, `letterboxd` (whether a Letterboxd export may be imported), `searchable` (whether a metadata addon of the server searches movies), `collections` (whether the user may manage collections, and so bring a server's), and `import`, the user's latest import running or ended, from a server or Letterboxd (`kind` `letterboxd`), `null` when none.
- `POST /admin/api/account/server-import` takes `kind` (`jellyfin` or `emby`), `address`, `name` and `password`, and `ratings`, `collections` and `playlists`, each `true` when left out, and answers `import`; `collections` is ignored for a user who may not manage collections. A refused name or password is `jellyfin_sign_in_refused`, an account the server does not let sign in `jellyfin_sign_in_forbidden`; the address errors are those of the administrator's import. For Plex it takes `kind` `plex`, `server` and `watchlist` (see [Plex, signing in](moving-to-polyfin.md#plex-signing-in)). A Letterboxd export is sent as described below.
- Polyfin sends the sign-in, the items requests and the sign-out of the administrator's import, never `/Users`.

### Plex sign-in for apps

- `POST /admin/api/account/server-import/plex/sign-in` starts the caller's sign-in, dropping the one they held, and answers `link`, Plex's sign-in page with the PIN's code, and `expiresAt`.
- `POST /admin/api/account/server-import/plex/sign-in/poll` answers 202 and `pending` until Plex signs the caller in, then the account: `name`, `profiles` (`id`, `name`, `managed`, `protected`), `profile`, the one read, and `servers` (`id`, the machine identifier, `name`, `owned`, `preselected`). An expired or ended sign-in is `plex_sign_in_ended` (409).
- `POST /admin/api/account/server-import/plex/profile` takes `profile` and `pin`, and answers the account; a PIN Plex refuses is `plex_profile_pin_refused`.
- `DELETE /admin/api/account/server-import/plex/sign-in` drops the sign-in and signs it out.
- `POST /admin/api/account/server-import` with `kind` `plex`, `server` and `watchlist` (by default `true`), and `ratings`, `collections` and `playlists` as for Jellyfin, starts the import; a server the account does not reach is `plex_unknown_server`, one none of whose addresses answers as it `plex_server_unreachable` (the sign-in is kept), and plex.tv failing `plex_tv_failed` or `plex_tv_unreachable`.
- Polyfin asks plex.tv, with its client identifier in `X-Plex-Client-Identifier` and `X-Plex-Product` `Polyfin`: `POST /api/v2/pins?strong=true`, `GET /api/v2/pins/{id}` once a second at most, `GET /api/v2/user`, `GET /api/v2/home/users`, `POST /api/v2/home/users/{uuid}/switch?pin=…`, `GET /api/v2/resources?includeHttps=1&includeRelay=1`, and `DELETE /api/v2/users/signout`; and Plex's Discover service `GET /library/sections/watchlist/all?includeGuids=1`, 100 items a page, and `GET /library/metadata/{ratingKey}?includeGuids=1`. The server is read as the owner's import reads it, with the server's token, but neither `/accounts` nor the history.

### Letterboxd import for apps

- `POST /admin/api/account/server-import`, or `/admin/api/users/{id}/server-import` for an administrator, takes the export as `multipart/form-data`: `kind` (`letterboxd`), `file`, and `watchlist`, `ratings` and `likesAsFavorites` (`true` or `false`, by default `true`, `true` and `false`).
- It answers `import`: `movies`, `searched`, `added` (`played`, `watchlist`, `ratings`, `favorites`), `unmatchedCount` and `unmatched` (`name`, `year`, `link`, `reason`: `not_found`, `ambiguous` or `search_failed`).
- `GET /admin/api/users/{id}/server-import` answers `searchable` and the user's Letterboxd `import`.

### App connections for apps

- `GET /admin/api/account/app-import` also answers `connections`, each with its `app`, `account` (`null` when the app gives none), `connectedAt`, `connectedBy` (`id` and `name`, `null` once that account is deleted), `problem` (`sign_in_refused`, or `null`), `paused`, `importing`, `lastImport` (`at`, `read`, `played`, `resumed`, `watchlist`, `unmatched` and `problem`: `unreachable`, `rate_limited`, `unexpected`, `profile_missing`, `profile_locked` (a Stremio profile asking for a PIN Polyfin does not have), `sign_in_refused`, `internal`, or `null`) and `nextImportAt`, the last import, or the connection, plus the interval set (`null` while stopped or paused), and `importEveryMinutes`, the interval. What a connection signs in with is never answered.
- The sign-in routes take `stayConnected`; the account read answers it back, and the import of it saves the connection.
- `POST /admin/api/account/app-connections/{app}/import` imports at once, and answers `connection`. A connection stopped is `app_connection_stopped`, none `app_not_connected`, and the setting off for a user `server_imports_disabled`.
- `DELETE /admin/api/account/app-connections/{app}` disconnects, even with the setting off, and answers `204`.
- An administrator's routes are the same under `/admin/api/users/{id}`, and answer the user's connections under `/admin/api/users/{id}/app-import`.

### Titles an import did not find

- Each import keeps the titles it did not find, per account and source (`nuvio`, `stremio`, `trakt`, `simkl`, `letterboxd` for the import by name and year, `jellyfin`, `emby`, `plex`), with what each would have imported. A title linked to its IMDb or TMDB title gets that at once, and each later import of the source imports it as that title.
- `GET /admin/api/account/import-titles` answers `sources`, each with its `source`, `notFound`, the titles not linked, and `linked`, sorted by source, only the sources with titles kept.
- `GET /admin/api/account/import-titles/{source}` answers `titles`, each with its `key`, `kind` (`movie`, `series` or `episode`), `name` (the series' for an episode), `year` (`null` when unknown), `ids`, what the source names it by, `season` and `episode` (`0` but for an episode), `reason`, and `link`, `null` when not linked, else `kind` (`movie` or `series`), `imdb` (`""` when none), `tmdb` (`0` when none), `name`, `year` (`null` when unknown), `linkedAt` and `linkedBy` (`id` and `name`, `null` once that account is deleted). The episodes of a series share its `key`, and one link covers them all.
- `PUT /admin/api/account/import-titles/{source}/links` takes `key` and `target`, an IMDb or TMDB identifier or address, and answers `link`. A target that is neither is `invalid_target`, a movie for a series or a series for a movie `kind_mismatch`, a TMDB identifier no title has `target_not_found`, and a key the list does not have `not_found`.
- `DELETE /admin/api/account/import-titles/{source}/links` takes `key`, and answers `204`, or `not_found` when the title has no link. What the link imported stays.
- An administrator's routes are the same under `/admin/api/users/{id}`, for the user `{id}`.

### Nuvio import for apps

- `GET /admin/api/account/app-import` answers `enabled`, `addonsAllowed`, and `import`, the import into the user's account running or the last one, `null` when none.
- `POST /admin/api/account/app-import/nuvio` takes `backend` (empty for Nuvio's own), `email`, `password` and `stayConnected`, and answers `key`, `app`, `stayConnected`, and `profiles`, each with its `id`, `name`, counts (`entries`, `played`, `resumes`, `library`) and `addons`, each with its `index`, `name` and `address`, redacted to its host. A refused email or password is `nuvio_sign_in_refused`, an invalid address `invalid_nuvio_address`, a backend that does not answer `nuvio_unreachable`, something else answering `not_nuvio`, a backend asking to wait too long `nuvio_rate_limited`, and a backup of another version `nuvio_backup_unsupported`, with its `version`.
- `POST /admin/api/account/app-import` takes `key`, `profile` and `addons`, the indexes of the addons to add, and answers `import`.
- An administrator's routes are the same under `/admin/api/users/{id}/app-import`, for the user `{id}`.
- An import names its `app`, `profile`, `user` and `startedBy` (`id` and `name`), `state` (`running`, `done` or `failed`), its counts (`read`, `played`, `resumed`, `watchlist`: the titles added to the Watchlist, `unmatchedCount`), `unmatched` (`id`, `name`, `type`, `season`, `episode`, `reason`), and `addons` (`name`, `address`, `installed`, `problem`).
- Polyfin reads Nuvio's public configuration at `GET {backend}/.well-known/nuvio` for its publishable key, signs in with `POST {backend}/auth/v1/token?grant_type=password`, reads the account with one `POST {backend}/rest/v1/rpc/sync_export_account_backup`, which must answer `format` `nuvio_account_backup` and `version` 1, and signs out with `POST {backend}/auth/v1/logout?scope=local`, which ends its own session only. A connection renews its session with `POST {backend}/auth/v1/token?grant_type=refresh_token` before each read, and keeps the new refresh token, the old one being refused from then on. The key goes in the `apikey` header and the session's token in `Authorization: Bearer …`, never in an address. A `429` or `503` answer with `Retry-After` is waited for.

### Stremio import for apps

- `POST /admin/api/account/app-import/stremio/link` asks for a link code, and answers `code` and `link`, the address to approve it at, never a key. A user waits for one code at a time, for 5 minutes.
- `POST /admin/api/account/app-import/stremio/link/poll` takes `stayConnected`, answers `202` and `pending` until the code is approved, then reads the account, signs out unless asked to stay connected, and answers it as `…/nuvio` does. A code expired, already used, or never asked for is `stremio_link_expired`.
- `POST /admin/api/account/app-import/stremio/password` takes `email`, `password` and `stayConnected`, and `POST /admin/api/account/app-import/stremio/key` takes `key` and `stayConnected`; both answer the account read. A refused email or password is `stremio_sign_in_refused`, a refused key `stremio_key_refused`, Stremio not answering `stremio_unreachable`, and an answer Polyfin does not read `stremio_unexpected`.
- Each profile of the account read has `locked` `true` when it asks for its PIN: it has no counts nor addons until `POST /admin/api/account/app-import/stremio/unlock` opens it, which takes `key`, the account held, `profile`, its `id`, and `pin`, and answers the account held again, the profile read. A wrong PIN is `stremio_pin_refused`, a profile that is not the account's or asks for no PIN `invalid_request`, and the account no longer held `app_import_expired`. Importing a profile still locked is `invalid_request`.
- An unmatched series whose episodes could not be read has the `reason` `episode_list`.
- Polyfin asks for a link code with `GET https://link.stremio.com/api/v2/create?type=Create` and polls it with `GET https://link.stremio.com/api/v2/read?type=Read&code=…`, signs in with `POST https://api.strem.io/api/login` (`email`, `password`, `facebook` `false`), reads the account's email and profiles with `POST https://api.strem.io/api/getUser` (`premiumPrefs.userProfiles`, each with its `_id`, `name`, `isMaster` and `hasPin`), opens a profile's session with `POST https://api.strem.io/api/premiumPrefs/authenticateProfile` (`type` `AuthenticateProfile`, `profileId`, `pin`, `null` for none), which answers its `authKey`, reads the library with `POST https://api.strem.io/api/datastoreGet` (`collection` `libraryItem`, `ids` `[]`, `all` `true`) and the addons with `POST https://api.strem.io/api/addonCollectionGet` (`update` `true`), with the account's key or the profile's, and signs out with `POST https://api.strem.io/api/logout` the keys it obtained, never a key pasted. Keys and PINs go in bodies, `authKey` and `pin`, never in an address.

### Connected account import interval for apps

- `GET /admin/api/settings` answers `importEveryMinutes`, in minutes, its choices in `bounds`. `PUT` takes one of them; any other is `invalid_import_every_minutes`.

## Invites and share links

Invite links create accounts; share links let a guest watch one title without one. Both are admin API routes, with public pages for the guest.

### Invite links for apps

- The admin API serves the links to administrators:
  - `GET /admin/api/invites` lists them, the newest first: `id`, `maxUses`, `uses`, `expiresAt` (null for never), `createdAt`, `revokedAt`, `model`, `group` and `createdBy` (each `id` and `name`, or null; `createdBy` is null once that user is deleted, `group` once that group is), `state`: `active`, `used_up`, `expired` or `revoked`, and `accountsEndInDays` (null for never).
  - `POST /admin/api/invites` creates one from `maxUses` (1 by default, up to 100), `expiresInDays` (7 by default, up to 365, `null` for never), `modelUserId` (null or left out for a new user's settings), `groupId` (null or left out for none) and `accountsEndInDays` (up to 365, null or left out for never). It answers 201 with the link as listed, plus its `token` and its `url`, which is null without a public address: the link is then `/admin/invite/<token>` on the server's address. The token is never answered again. Errors: `invalid_invite_uses`, `invalid_invite_expiry`, `invalid_invite_model`, `invalid_group` and `invalid_invite_account_end`.
  - `POST /admin/api/invites/{id}/revoke` revokes one, and answers it.
- The guest's page uses two routes that need no session:
  - `GET /admin/api/invite/{token}` answers `serverName` while the link may create an account.
  - `POST /admin/api/invite/{token}` with `name` and `password` creates the account. It answers 201 with `user` (`id`, `name`, `isAdministrator`, `imageTag`) and `webClient`. Without the web player (`webClient` false), the answer opens an admin session, as signing in does. With it, the page signs in with `POST /Users/AuthenticateByName` as jellyfin-web does, naming jellyfin-web's app and device, and keeps the session where jellyfin-web 12.4 reads it, in `jellyfin_credentials` in the browser's local storage.
  - A token that is no link's answers 404 `invite_unknown`, and a link used up, expired or revoked 410 `invite_used_up`, `invite_expired` or `invite_revoked`. A refused name or password answers as creating a user does: `invalid_name`, `invalid_password` or `name_taken`. Too many failures answer 429 `too_many_attempts`, with `Retry-After`.

### Share links for apps

- The admin API serves a user's own links: `GET /admin/api/account/share-links` answers `allowed` (whether the user may make one now) and `links`; each link has `id`, `user` (`id`, `name`), `item` (`id`, `kind`: `movie` or `episode`, `name`), `maxViewers`, `createdAt`, `expiresAt`, `revokedAt`, `plays` and `state`: `active`, `expired` or `revoked`. `POST /admin/api/account/share-links` makes one from `itemId`, `expiresInHours` (1, 24 by default, 168 or 720) and `maxViewers` (1 by default, up to 5), and answers 201 with the link, its `token` and its `url` (null without a public address: the link is then `/share/<token>`). Errors: 403 `share_links_off`, 400 `not_shareable`, `invalid_share_expiry` or `invalid_share_viewers`, 404 `not_found`. `POST /admin/api/account/share-links/{id}/revoke` revokes one.
- `GET /admin/api/account/share-titles?search=…` finds movies and series to share, `GET /admin/api/account/share-titles/{id}` describes one title and `GET /admin/api/account/share-titles/{id}/episodes` lists a series' episodes.
- Administrators: `GET /admin/api/users/{id}/share-links` and `POST /admin/api/share-links/{id}/revoke`. Users carry `canShare`, and the settings `shareLinks`.
- The guest's page uses `GET /share/{token}`, `GET /share/{token}/image/primary` and `…/backdrop`, `POST /share/{token}/play` (whose JSON body may give `position`, in seconds, to start at, and which answers `session`, or `error`: `limit`, `viewers`, `hours`, `busy` or `unavailable`), `GET /share/{token}/play/{session}/master.m3u8` with its playlists and segments, and `POST /share/{token}/play/{session}/progress` and `…/stop`. A link that opens nothing answers 404 `unknown`, or 410 `expired`, `revoked`, `off` or `unavailable`. The page carries the link's ID as `data-link`, and keeps the guest's position in the browser's `localStorage` as `polyfin.share.<link ID>`: `{"position": seconds, "savedAt": milliseconds}`.

### Share button for apps

- The script asks `GET /Polyfin/Items/{id}/Share`, with the web player's token. It answers `{"Shareable": true, "Url": "/admin/me/account?share=<id>#shared-links"}` when the title may be shared, and `{"Shareable": false, "Url": null}` otherwise.

## Libraries, search and sort

Libraries come from addons' catalogs, read a page at a time. Listings, search and sorting answer from those pages, so some answers differ from a Jellyfin library scanned whole.

### Library menu visibility for apps

- `GET /Polyfin/UserViews` lists the views `/UserViews` gives the caller, in the same order, as `{"Items": [{"Id": <the view's Id>, "HideInMenus": <true when the web player's menus leave it out>}]}`.
- Polyfin's own Live TV, Playlists, Collections and Watchlist views are never left out.

### Libraries shown as movies and shows for apps

- The library is a `CollectionFolder` without `CollectionType`. `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Movie,Series` lists its titles as `Series` and `Movie` items, and `IncludeItemTypes=Movie` or `Series` alone lists them with a `TotalRecordCount` of that kind alone, as for an `anime` library (see [What catalogs list](addons-and-libraries.md#what-catalogs-list)).
- `/Shows/{id}/Seasons`, `/Shows/{id}/Episodes`, `/Shows/NextUp`, `/UserItems/Resume`, `PlaybackInfo` and the streams answer them as other series, episodes and movies.
- The `Id`s of a catalog of other videos' titles are not those of the folders and videos they were: switching gives users' data to the new items, and the former ones answer 404. Titles typed movies and series, and those of a movie or series catalog, keep their `Id`s.

### Show as for apps

- A library shown as movies and shows is a `CollectionFolder` without `CollectionType`. A movie or series catalog's library shown so lists `Movie` and `Series` items, and a title of another type as a folder or a video; `IncludeItemTypes` lists and counts the kinds asked for. Item `Id`s are those of the same titles as listed.
- The admin API's libraries carry `showAs`, one of `showAsChoices` (`home_videos`, `listed`, `movies_and_shows`), or null while no one chose, saved with `PUT /admin/api/scopes/{scope}/libraries`. A library saved with `showAs` null keeps the one it has. `showAsChoices` is empty for the catalogs that offer none, and a `showAs` the catalog does not offer is refused with `invalid_library`.

### Other videos as movies and shows for apps

- The library is a `CollectionFolder` without `CollectionType`. Its titles are `Series` and `Movie` items: `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Movie,Series` lists them, `IncludeItemTypes=Movie` or `Series` alone lists and counts that kind only (see [What catalogs list](addons-and-libraries.md#what-catalogs-list)), and `/Shows/{id}/Seasons`, `/Shows/{id}/Episodes` and `/Shows/NextUp` answer a show as any other.
- The `Id` of a title and of its videos changes when the choice is switched; a title's `Id` stays when its details gain or lose videos.

### Libraries of mixed kinds for apps

- A library whose titles are of several kinds, such as an `anime` library or one shown as movies and shows, lists and counts only the kinds a listing asks for. `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Series` reads the catalog's first 500 titles, or its **Maximum titles** when lower, keeps its shows, and answers them a page at a time with a `TotalRecordCount` that counts them alone, so that apps page through them to the end. A title whose details are not known after 5 seconds has no kind yet: it is left out until they are, and the listing tells that more may follow.
- Jellyfin counts and pages the items of the types asked for in the same query ([ItemsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemsController.cs#L381-L394), [L626-L629](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemsController.cs#L626-L629)); Polyfin counts them among the titles it reads, so a catalog longer than 500 titles counts its first 500.
- A library shown as movies and shows answers the apps that ask for `Movie` or `Series` alone each kind with its own count. Apps that list only movies and shows ask libraries for `Movie` and `Series` items only.

### Folders of videos for apps

- A folder is a `Folder` item (`IsFolder` true) with `ChildCount`, `RecursiveItemCount` and `UserData.UnplayedItemCount`; its videos are `Video` items whose `ParentId` is the folder, and `/Items?ParentId=` lists them. A title without videos is a `Video` item of the library. The library's `CollectionType` is `homevideos`.
- `/Items?ParentId=<library>&Recursive=true&IncludeItemTypes=Video` lists the videos through the folders, and tells more may follow while a folder's details are not known yet. `IncludeItemTypes=Folder` with `searchTerm` searches folders.
- A title keeps its item when its details gain or lose videos: it is listed as a folder or a video as they tell.
- A folder's videos listed with `IncludeItemTypes`, `ExcludeItemTypes`, `MediaTypes` or `Filters=IsFolder` count only those kept in `TotalRecordCount`, as Jellyfin counts a folder's children.
- A folder plays nothing itself: its `PlaybackInfo`, `/Videos/{id}/stream` and `/Items/{id}/Download` answer 400, as Jellyfin's do for an item without media sources (see [Opening streams and downloading files](#opening-streams-and-downloading-files)). It is no series: `/Shows/{id}/Seasons` and `/Shows/{id}/Episodes` answer 404, as Jellyfin's do ([TvShowsController.cs, lines 261 to 263 and 351 to 355](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L261-L355)). Apps open it with `/Items?ParentId=`, and play it as jellyfin-web does, through its videos.

### Folders of videos sorting for apps

- A folder lists its videos in the order its addon gives them, whatever `SortBy` asks, unless the listing is `Recursive` and names item types, as jellyfin-web's **Videos** view does: Jellyfin sorts a folder's children as asked, and jellyfin-web asks a folder's page for them by name.
- A folder of a home videos library answers `/Shows/{id}/Episodes` and `/Shows/{id}/Seasons` with 404 in Jellyfin ([TvShowsController.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/TvShowsController.cs#L261-L264)) as in Polyfin; shown as a show, the title answers them.

### Libraries hidden from the top bar for apps

- The web player's Polyfin script reads the choice from `GET /Polyfin/UserViews` (see [Libraries left out of the web player menus](jellyfin-compatibility.md#libraries-left-out-of-the-web-player-menus)) and hides the library's links with a style. `/UserViews` and every other answer list the library as before.

### Titles local folders hold for apps

- `/Items?ParentId=<library>` and `/Items/Latest?ParentId=<library>` leave the titles out; the title's own requests, search, `/Shows/NextUp` and `/UserItems/Resume` answer it as before, under the same `Id`.
- The admin API's libraries carry `skipLocal`, saved with `PUT /admin/api/scopes/{scope}/libraries`, and `skippable`, which tells the catalogs that take it. `skipLocal` on another catalog is refused with `invalid_library`.

### Library images for apps

- The library's image is its Primary image: `/UserViews` and `/Items` give its tag in `ImageTags`, `/Items/{id}/Images/Primary` serves it, and `/Library/VirtualFolders` names the library as its `PrimaryImageItemId`.
- The automatic image is looked up again with the catalog's pages, after **Refresh catalogs after (minutes)**. Its tag changes with the title it comes from. `/UserViews` never waits for it: until it is found, the library shows the image found last, or none.
- A generated image's tag changes with the library's name and with the titles it shows (see [Generated images](artwork-and-editing.md#generated-images)). `/UserViews` never waits for it: it comes from the pages Polyfin keeps.

### Library listings without a limit

- A listing of a library, of an addon's collection or of a collection made by users that sets no `limit`, as jellyfin-web's collection pages ask, lists up to 500 titles, or the library's maximum when lower. For a library or an addon's collection, the titles kept from earlier listings answer at once when they make the whole listing. Otherwise the listing waits for the addons 3 seconds at most, or 1 second when the titles kept fill a page of 100, then lists those read by then, and at least the first page of each catalog. It reads the rest in the background, for the next listing. A listing with a `limit` lists as many as it asks, within the library's maximum.

### Collections from addons for apps

- A collection library has no `CollectionType`, unlike Jellyfin's Collections view of collections made by users, which keeps `boxsets`. jellyfin-web and Streamyfin leave `boxsets` libraries out of their rows of latest items. For the apps chosen under **Apps that show collection libraries by collection**, whose `Client` in their authorization matches one of the names in any letter case, its `CollectionType` is `boxsets` wherever it is described: `/UserViews`, `/Users/{userId}/Views`, its own description, `/Items?Ids=`, `/Library/MediaFolders`, `/Library/VirtualFolders` and the listing of the root folder. Requests made with an API key never are. Its listings are the same for every app: `IncludeItemTypes=BoxSet&Recursive=true` lists its collections, and a collection with `IncludeItemTypes=Movie,Series&Recursive=true` its titles.
- `/admin/api/settings` holds the setting as `collectionLibraryApps`, a list of app names, and `GET /admin/api/settings/apps` answers the apps to choose among, `{"apps": [...]}`: the names the signed-in devices of every user give, each once whatever its letter case, by name.
- A collection without a poster gets `Primary` and `Thumb` tags in `ImageTags` once its generated images are known. `/Items/{id}/Images/Primary` serves a 1,000 × 1,500 JPEG and `/Items/{id}/Images/Thumb` a 1,920 × 1,080 one, resized as other artwork is. `/admin/api/settings` holds the setting as `generateImages`.
- `/Items/Latest` lists its collections, `BoxSet` items.
- A listing that asks for `Folder` items keeps them, as jellyfin-web lists a library without type by its folders, movies and series.
- A `Recursive` listing of a collection library that asks only for titles (no `BoxSet` or `Folder`), such as `IncludeItemTypes=Movie,Series`, lists the titles of its collections, merged one of each catalog in turn as a collection lists them, as Jellyfin lists the movies and series within a mixed library's folders. Apps that make a library's home row so show its titles. Other listings of the library list its collections. Such a listing takes the filters of what the user did, `Filters=IsPlayed`, `IsUnplayed`, `IsFavorite` or `IsResumable` and `isPlayed` or `isFavorite`, as a library of titles does: it lists the titles kept, in pages, with a `TotalRecordCount` that counts them.
- An addon's collection lists the titles of the catalogs it and the collections within it group, merged one of each in turn, never those collections. A collection that groups other collections, listed whole on its first page, is ordered by premiere date. A `Recursive` listing that asks only for titles (no `BoxSet` or `Folder`) and fits on its first page, as Streamyfin's collection pages ask, is sorted by its `sortBy`.
- A listing narrowed by `IncludeItemTypes` reads only the catalogs whose titles it keeps, by the catalog's type, movie or series: `IncludeItemTypes=Movie` lists a collection's movies, page after page, with a total that counts them alone, and `IncludeItemTypes=BoxSet` lists none, with a total of 0, as an addon's collection holds no collection. A collection library listed by its collections lists none for `Movie` or `Series` without `Recursive`, out of 0, and a movie or series library none for the other type.
- A listing of collections across the server (`IncludeItemTypes=BoxSet` without `ParentId`), as Strand asks for its shelves, lists every collection of the user's collection libraries, each once, with the collections made by users, sorted by name. jellyfin-web asks the same for its **Add to collection** dialog, where only collections made by users take a title: it gets those alone.

### Collections made by users for apps

- The **Collections** view has CollectionType `boxsets`. A listing of BoxSets across the server includes user collections, and for apps other than jellyfin-web the addons' collections too (see [Collections from addons](collections-people-and-ratings.md#collections-from-addons)).
- Hidden titles do not count in `ChildCount`, `RecursiveItemCount` or the played count.
- Without the permission, `POST /Collections` and `POST` or `DELETE /Collections/{id}/Items` answer an empty 403, as in Jellyfin, administrators included.
- Delete a collection with `DELETE /Items/{id}`, as in Jellyfin. Users without permission get 401.
- A missing name, or a title the user may not see, answers 400.

### Smart lists for apps

- A list shown as a library is a `CollectionFolder` in `/UserViews`, after the libraries. Its `CollectionType` is `movies` or `tvshows` when all its catalogs are of that type; otherwise it has none, as the Watchlist. `/Items?ParentId=` lists its titles and `/Items/Latest?ParentId=` gives its home row.
- A list shown as a collection is a `BoxSet` in the **Collections** view, by name with the collections made by users. The view shows once such a list exists.
- The collections listing across the server lists it, except for jellyfin-web's **Add to collection** dialog. `POST` and `DELETE /Collections/{id}/Items` answer an empty `403`.
- The admin API serves the lists under `/admin/api/scopes/{scope}/smart-lists`. A list's `rules` and `exclude` are each a group of rules (`{"field": "group", "match": "all" | "any", "rules": [...]}`); a list sent without `exclude` has none. A rule's `field` is `genre`, `year`, `released`, `homeRelease`, `rating`, `votes`, `language`, `runtime`, `contentRating`, `person`, `studio`, `type`, `rank` or `state`; `released` takes `min` and `max` in days from today, negative in the past; `homeRelease` takes `value` `released`, `upcoming` or `both`, and `max` days, from 1 to 36,500. `sort` is `catalog`, `premiere`, `latestEpisode`, `seasonPremiere`, `rating`, `name` or `random`. `.../catalogs` gives the catalogs a list may draw from, the languages its rules may name and the `contentRatings` a rule may set as the highest. `POST .../preview` answers `titles`, `listed`, `unknown` (titles waiting for their description, votes, studios or home release) and `complete` (whether every catalog answered in time).

### Movies waiting for their digital release for apps

- A movie listed by a smart list of upcoming titles carries its future day as `PremiereDate`, as Jellyfin's episodes not aired yet do.
- Its `PlaybackInfo` answers no source with `ErrorCode` `NotAllowed`, Jellyfin's "Playback of the item is not allowed" ([PlaybackErrorCode.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Dlna/PlaybackErrorCode.cs#L8-L11)), rather than the `NoCompatibleStream` Jellyfin answers for an item without a source ([MediaInfoHelper.cs, line 108](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Helpers/MediaInfoHelper.cs#L103-L109)): the movie has no source because it is not out, not because the app cannot play it.

### Searches in plain words for apps

- `/Items?searchTerm=` lists the collection first, a `BoxSet`, when the request keeps `BoxSet` items, as jellyfin-web's search asks for its Collections row with `IncludeItemTypes=BoxSet` and as a search without type filters does, and has no `ParentId`. `/Search/Hints` does not list it.
- `/Items?ParentId=` lists its titles, in their order. Another user cannot reach it.
- `POST /UserFavoriteItems/{id}` (or `/Users/{userId}/FavoriteItems/{id}`) saves it as the user's own smart list and answers its `UserData` with `IsFavorite` set; `DELETE` deletes the list. The refusal is an empty `403`, as is the save of a user who has 100 smart lists already. `UserData.IsFavorite` tells whether a list saved from it exists.

### Root folder, counts and suggestions for apps

- `/Items/Root` is the user's root folder; its children are the user's views.
- `/Items/Counts` counts each library's first page of titles, plus one when more follow, as genre and studio pages count, as far as Polyfin keeps them: it asks the addons nothing. The user's favorites are counted exactly with `isFavorite=true`.
- `/Items/Suggestions` returns titles picked at random among the first page of each library, those the user's taste profile ranks first coming first once it is confident (see [Top picks for you](home-rows.md#top-picks-for-you)).

### Search hints

Search hints answer from the same search as the main item search, in this order:

1. the movies and series the addons find;
2. the episodes Polyfin knows by name from series already opened (addons search titles, not episodes);
3. the people credited in titles the user reaches.

All results stay within the user's libraries, parental control and blocked genres. Channels and programmes, which the item search also finds (see [Searching](live-tv.md#searching)), are left out of search hints.

- `/Search/Hints` answers from the same search as `/Items`, without its channels and programmes.

**Compared with Jellyfin**

- Jellyfin ranks every kind together; Polyfin keeps the order above.
- Jellyfin searches its own library and answers at once. Polyfin asks the addons' search catalogs and answers with what they found within a few seconds, without waiting for an addon that is late (see [Catalog pages](addons-and-libraries.md#catalog-pages)): the same search may find more a moment later.

### Search suggestions

Before anything is typed, the web player's search page shows Suggestions: the user's favorite and liked titles first, then movies and series from the top of each of the user's libraries, in their catalog's order: the most popular first in a popular catalog. The libraries are mixed, one title of each in turn, and the picks change each time the page opens, each library giving one of its next two titles at random. They stay within the user's libraries, parental control and blocked genres, without the titles they hid. Above them, the web player lists the user's recent searches (see [Recent searches](web-client.md#recent-searches)).

- `/Items` without `ParentId` answers such suggestions when it asks `Recursive=true` for `Movie`, `Series` or `MusicArtist` only, sorted by `Random`, alone or with `IsFavoriteOrLiked`, which puts favorite and liked titles first.
- `/Items/Suggestions` still picks at random among the first page of each library, as Jellyfin picks at random, the titles the user's taste profile ranks first coming first once it is confident (see [Top picks for you](home-rows.md#top-picks-for-you)).

**Compared with Jellyfin**

- Jellyfin picks suggestions at random among everything; Polyfin picks among the first page of each library, from its top, as a remote catalog puts its titles forward.

### Titles at random

An app that lists a library's titles at random, as the home carousel of the [LumaaGlaass](themes-and-custom-code.md#lumaaglaass) theme picks its slides, and as the web player's shuffle of a library plays, gets them drawn among the library's first 100 titles, in their catalog's order, rather than its first catalog page alone. The carousel's picks reach well past a library's first titles, and change from one page load to the next.

- The titles drawn among are those the user may see: within their parental control and blocked genres, without the titles they hid, a title hidden after the draw leaving it at once. A collection library gives the titles of its collections, as its other listings of titles do.
- A listing that asks for more than 100, from its first title, draws among as many, up to 500, the most a listing that asks for no limit gets. A library's **Maximum titles** bounds its draws too (see [Genre and maximum](addons-and-libraries.md#genre-and-maximum)).
- A draw is kept for 2 minutes, so that the pages an app asks for one after the other follow one order and list each title once. A listing after that draws anew.
- The listing's total is the number of titles drawn among: pages past it are empty.
- A library's first 100 titles are read as a listing of 100 reads them, a few catalog pages, and kept with the other pages read (see [Catalog pages](addons-and-libraries.md#catalog-pages)). While they are being read, a listing waits for them at most 3 seconds, then draws among those read so far, without keeping that draw, and the rest is read in the background for the next one.
- Smart lists keep their own sort (see [Smart lists](smart-lists-and-search.md#smart-lists)).

- A listing of a library with `ParentId`, `Recursive=true` and only title types, such as `IncludeItemTypes=Movie,Series`, whose first `SortBy` is `Random`, is answered from the draw; the keys after `Random` change nothing.
- `TotalRecordCount` is the number of titles drawn among.

**Compared with Jellyfin**

- Jellyfin sorts the whole library at random, anew for each request ([`OrderMapper.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L55), applied by [`BaseItemRepository.QueryBuilding.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/BaseItemRepository.QueryBuilding.cs#L306-L341), Jellyfin 12.4), so the pages of one listing may repeat or skip titles. Polyfin keeps each draw 2 minutes so that its pages do not.
- Polyfin stops at a library's first 100 titles: a remote catalog is read a page at a time from its addon, and reaching its end, which may be thousands of titles away, would cost as many requests to the addon for every user's home page. 100 titles are a few pages, which listings read anyway.

### Series by their latest episode

The web player's TV shows tab sorts a series library by "Date Episode Added" and "Date Played" ([`tvshows.js`, lines 249 to 268](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/apps/legacy/controllers/shows/tvshows.js#L249-L268), jellyfin-web 12.4). Polyfin answers both:

- "Date Episode Added" orders the series by their latest regular episode that has aired, as their descriptions date it, the latest first. Specials (season 0) and episodes to come do not count. A series without a dated episode comes last, the first in ascending order.
- "Date Played" orders them by the last time the user played an episode of theirs through, the latest first; series never played come last.
- As for a listing at random (see [Titles at random](#titles-at-random)), the series are sorted among the library's first 100, or as many as the listing asks for from its first title, up to 500, within the user's parental control and blocked genres, without the titles they hid. The order is kept 2 minutes, so that the pages of one listing follow it, and the listing's total is the number of series sorted.
- A series whose description Polyfin does not keep yet has it read in the background, at most 50 at each listing: it comes as a series without episodes meanwhile, and the order is not kept until every description is read.
- Smart lists keep their own sort (see [Smart lists](smart-lists-and-search.md#smart-lists)), which may also be by the latest episode or the latest season premiere.

- A listing of a library with `ParentId`, `Recursive=true` and `IncludeItemTypes=Series` alone, whose first `SortBy` is `DateLastContentAdded`, `SeriesDatePlayed` or `DatePlayed`, is answered so.
- `SortOrder` applies to every key, as in Jellyfin, and the keys after the first, then the name, order the series of one date.

**Compared with Jellyfin**

- Jellyfin orders series by `DateLastMediaAdded`, when their latest episode was added to the library ([`OrderMapper.cs`, line 61](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L61), Jellyfin 12.4). An addon's episodes are never added: they air. Polyfin takes the day the latest regular episode aired in its place.
- `SeriesDatePlayed` is, as in Jellyfin, the latest date the user played one of the series' episodes through ([`OrderMapper.cs`, lines 78 to 87](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L78-L87)); `DatePlayed` the date the user last played the series itself ([lines 32 to 48](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Item/OrderMapper.cs#L32-L48)).
- Jellyfin sorts the whole library; Polyfin its first titles, as for titles at random, since a remote catalog is read a page at a time.

### Live TV in item search

- `/Items` with `searchTerm` finds channels asked as `TvChannel` or `LiveTvChannel`, and programmes asked as `LiveTvProgram`, `Program` or `TvProgram`, filtered by `isMovie`, `isSeries`, `isNews`, `isKids` and `isSports`. It finds at most `limit` of each.
- A search that names no type finds channels, and programmes only when it sets one of those five filters. A search that names types finds programmes only when one of them is a programme type.
- Search hints leave channels and programmes out.

### Years, groupings, artwork and viewing

- `/Years` lists the years the year pages offer (catalogs whose genre filter offers years).
- `/UserViews/GroupingOptions` lists the movie and series libraries.
- `/Items/{id}/Images` lists a title's artwork.
- `/Sessions/Viewing` records the item an app shows, which `/Sessions` gives as `NowViewingItem`.

## Titles and metadata

A title's details come from metadata addons, local files and TMDB. These notes cover the fields and routes that differ from Jellyfin's.

### Ratings for apps

- `/Polyfin/Items/{id}/Ratings` answers a title's `CommunityRating`, `CriticsScore` and `AudienceScore` (null when it has none, all null without a key) and `Pending` while MDBList is being asked about it.
- In a title's details, a movie's or series' `CommunityRating` is the IMDb rating from MDBList, else the addon's, from 0 to 10. `CriticRating` is the score **Critic rating shows** picks, the critics' score or the audience score, from 0 to 100.

### Ratings on posters for apps

- `GET /Polyfin/Items/Ratings?Ids=<id>,<id>…`, with the user's token, answers `{"Enabled": true, "Items": [{"Id", "CommunityRating", "CriticsScore", "AudienceScore"}]}`: for each movie or series named that the user may see and that has a rating, the scores from 0 to 10 and 0 to 100, `null` for those it lacks. It reads 100 identifiers at most. While **Show ratings on posters** is off, or **Badges on posters** is on, it answers `{"Enabled": false, "Items": []}` and reads nothing.

### Similar titles for apps

- With **Similar titles** off, every Similar route answers an empty list. An unknown title is still not found.

### Artwork requests for apps

- An image its artwork server does not have (`404` or `410`) answers `404`, and an artwork server that fails otherwise (no answer, a `5xx`) makes it answer `502`. An artwork server that answered `429`, or `503`, with a `Retry-After` has its images answer `503` at once, with a `Retry-After` of the time left.
- An app asking for an image much smaller than the original (`maxWidth`, `maxHeight`, `fillWidth`, `fillHeight`, `width` or `height`) gets it resized, as a JPEG, or a PNG when it was one.
- An image asked for with its `tag` may be kept by the app for good: the tag changes with the image.
- A wide poster gets a `PrimaryImageAspectRatio` of 16:9 and a square one 1:1. Other posters keep 2:3.
- Jellyfin measures a Primary image kept on its disk, and gives a remote one the shape of the item's type ([DtoService.cs, lines 1849 to 1879](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Dto/DtoService.cs#L1849-L1879)).
- With **Badges on posters** on, an image's tag is the image's own followed by what the badges show and Polyfin's signature: when a rating changes, the tag changes. A tag Polyfin did not sign draws nothing. Turned off, an app that asks with a badged tag still gets the image without badges, with a cache lifetime of a week.
- With **Poster address template** empty, the addons' posters show with their own tags.
- A title not yet looked up on TMDB gives a poster tag of its own; a title TMDB has no poster of answers `404` that once, then gives no tag. A lookup that does not end within 5 seconds answers `404` that time.

### Episode orders for apps

- `DisplayOrder` is kept from `POST /Items/{id}` on a series, as Jellyfin keeps it ([`ItemUpdateController.cs`, lines 412 to 415](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/ItemUpdateController.cs#L412-L415)), and given back in the series' `DisplayOrder` ([`DtoService.cs`, lines 1207 to 1210](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Dto/DtoService.cs#L1207-L1210)). Empty or absent is aired.
- In an order, `/Shows/{id}/Seasons` lists the group's seasons, with identifiers of their own, which `/Items/{id}`, `/Items?ParentId=` and `/Shows/{id}/Episodes?seasonId=` take; `seasonId` also takes a season of the addon's. An episode's `ParentIndexNumber`, `IndexNumber`, `SeasonId`, `SeasonName` and `ParentId` are those of the order; its `Id` is the same in every order.
- `/Shows/{id}/Episodes?season=` filters by the order's season number.

### Movie recommendations for apps

- `/Movies/Recommendations`.

### Remote images for apps

- `/Items/{id}/RemoteImages/Providers` lists `TMDB` and `Addons` with their `SupportedImages`: both for movies and series, `TMDB` alone (Primary) for seasons, episodes and people, none for other items or without a key.
- `/Items/{id}/RemoteImages` answers a `RemoteImageResult` for `type` (every type when left out), `providerName`, `includeAllLanguages`, `startIndex` and `limit`. An addon's image has its addon's name as `ProviderName`. `Url` and `ThumbnailUrl` are Polyfin addresses, signed for the item, which apps load without credentials.
- `POST /Items/{id}/RemoteImages/Download?type=&imageUrl=` is for administrators (403 otherwise). `imageUrl` must be a `Url` Polyfin listed for the item and type (400 otherwise); `providerName` is ignored.

### Edit images and Identify for apps

- Downloading an image from an address Polyfin did not list for the item and image type answers `400`. An upload of another image type, or of anything that is not a JPEG, PNG or WebP picture, answers `400`; Jellyfin keeps any uploaded image file as sent.
- Applying an **Identify** result to a title only addons list fails as a failed identification does in Jellyfin (`500`). Applied to a title of a local folder, it answers `204` as Jellyfin does.
- Creating a collection with a missing name, or an identifier that is not one: Jellyfin creates the collection and then fails with `500`; Polyfin answers `400` and changes nothing.

### Extras and theme media for apps

- `/Items/{id}/LocalTrailers` lists a movie's, series' or season's local trailers, and `/Items/{id}/SpecialFeatures` its other extras, theme media left out, both as arrays of items; the `/Users/{userId}/Items/{id}/…` forms jellyfin-web calls answer the same.
- `/Items/{id}/ThemeSongs` and `/Items/{id}/ThemeVideos` answer a `ThemeMediaResult`, and `/Items/{id}/ThemeMedia` both, with no soundtrack songs. With `inheritFromParent=true`, a season without any gets its series', and an episode its season's, else its series'; `OwnerId` is the item they were found on. `sortBy=Random` shuffles them.
- An item's details give `LocalTrailerCount` and `SpecialFeatureCount`; `RemoteTrailers` stay the addons' trailers.

### Refresh metadata for apps

- `POST /Items/{id}/Refresh` answers `204` at once, as Jellyfin's does once it queued the refresh ([ItemRefreshController.cs, lines 58 to 92](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Jellyfin.Api/Controllers/ItemRefreshController.cs#L58-L92); [ProviderManager.cs, lines 1136 to 1162](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/MediaBrowser.Providers/Manager/ProviderManager.cs#L1136-L1162)), and `404` for an item the user cannot reach.
- On a library, `metadataRefreshMode` or `imageRefreshMode` set to `Default` or `FullRefresh` starts the refresh; the replace options are ignored.

### Library refresh from Jellyfin apps

- **Modes:** the refresh starts when the metadata or image refresh mode is "Scan for new and updated files" (`Default`) or a full refresh (`FullRefresh`), what jellyfin-web sends. Without a mode, `None` in Jellyfin, or with `ValidationOnly`, which ask no metadata provider, it starts nothing. Images come with the descriptions, so an image mode starts it too.
- **Ignored:** "Replace all metadata" (`replaceAllMetadata`), "Replace existing images" (`replaceAllImages`) and `regenerateTrickplay`.
- **Libraries that do not offer it:** a collection, music or IPTV library, a smart list, or a user's own library answers as before, `204` with nothing refreshed, as Jellyfin answers for any item it queues. A library the user cannot see answers `404`, as Jellyfin answers for an item it does not find.
- jellyfin-web's three modes send `MetadataRefreshMode=Default`, `FullRefresh`, and `FullRefresh` with `ReplaceAllMetadata`.

### Local files in the Jellyfin API

Through the Jellyfin API, an episode only a file holds is an `Episode` of its series like the others, `Name` its file's without extension, without `Overview`, its `Id` made from the series' identifier and its season and number, as an addon's episode's; an episode without an image has a `Primary` image tag, its frame, which `/Items/{id}/Images/Primary` serves. Through the Jellyfin API, a local file is a media source of its title like any other, its `Name` the folder's name and its `Path` its file name. A Blu-ray disc's `Path` is its disc folder's name with the `.m2ts` extension, such as `Nosferatu (1922).m2ts`, or `Nosferatu (1922) - Disc 1.m2ts` for a folder named after the disc's number, and `Box - DISC02 - title 2.m2ts` for a title other than its main one. A playlist's media source, as a DVD's, has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A DVD's `Path` is its disc folder's name with its main title's `.VOB` extension, such as `Nosferatu (1922).VOB`, or `Box - DISC02 - title 2.VOB` for a title other than its main one; its media source has `SupportsDirectPlay` and `SupportsDirectStream` false, PlaybackInfo offers it only with a `TranscodingUrl`, and its download answers 400. A disc image's `Path` is its file name, such as `Nosferatu (1922).iso`, or `Box - title 2.iso` for a title other than its main one, and its media source plays as a DVD's does. A title of a show's disc is a media source of its episodes, named as a disc's title other than its main one: `Season 01 - Disc 2 - title 1.m2ts`, `Season 01 - Disc 1 - title 3.VOB` or `Disc 1 - title 2.iso`. A share's files are served the same way.

### Local artwork in the Jellyfin API

A title's local artwork is one of its images like the addons': its `ImageTags`, `BackdropImageTags` and `/Items/{id}/Images` list it by tags that change when the image's size or modification time changes, and `/Items/{id}/Images/{type}` serves it, resized as apps ask, from the folder or the share. Besides `Primary`, `Backdrop`, `Logo`, `Thumb` and `Banner`, a local image may be an `Art` or a `Disc` image. No path of the server's shows: `/Items/{id}/Images` gives no `Path`.

### Plain videos in the Jellyfin API

Through the Jellyfin API, a plain video of a movies folder is a `Movie` item of the folder's library, and one of a shows folder a `Series` with its `Season` and `Episode` items (see [Plain videos](unmatched-files.md#plain-videos)). They have no `Overview`, `OfficialRating` nor `ProviderIds`; a plain movie's or series' `ProductionYear` is the year its name gives, and an episode's `Name` its file's, without extension. Their identifiers are those of other titles, made from the plain video's own (`polyfin-plain:…`), which an app never needs. An item of a plain video whose file is gone or was renamed answers 404, as Jellyfin answers for an item that is gone. One matched since answers `/Items/{id}` and `/Users/{userId}/Items/{id}` with the title it became, or its season or episode of the same numbers, whose `Id` is the title's own, not the identifier asked; it answers 404 to everything else, and lists never show it. `POST /Items/RemoteSearch/Apply/{id}` on a plain movie, series, season or episode links its unit to the result's `ProviderIds.Imdb` and answers 204, 400 for a result without an IMDb identifier, 404 for a plain video matched already, and 500 for any other item; administrators only (403 otherwise). The admin API lists the files as before, among the unmatched files of the folder.

### Home videos in the API

A home videos folder's `kind` is `homevideos` in the admin API, added with `"kind": "homevideos"`; it takes no `useArtwork`, its artwork always showing. Its `files` and `matched` count its videos, `unmatched` is 0, and `datedBy` counts them by where their dates come from: `{"nfo", "created", "file"}` (zero for other folders). Its unmatched list is empty, and its `titles` and `links` routes answer 404. Through the Jellyfin API, its library's `CollectionType` is `homevideos`, and its titles have `Type` and `MediaType` `Video`, `PremiereDate` and `ProductionYear` from their date, `Overview` from their NFO file's plot, no `ProviderIds`, and a `Primary` image, the poster beside them or a frame; they answer `IncludeItemTypes=Video`, never `Movie`. In the playback history and the statistics, their `kind` is `video`.

## Watch state and home rows

Polyfin adds routes for what Jellyfin 12.4 has no route for: taking a title off Continue Watching or Next Up, hiding it, and the watchlist. Apps see the result in Jellyfin's own listings.

### Continue Watching, Next Up and hidden titles for apps

- `DELETE /Polyfin/UserItems/{id}/Resume` takes the item off Continue Watching, a series or a season its episodes, a folder of videos its videos, and answers its `UserItemData`.
- `POST /Polyfin/UserItems/{id}/Hidden` hides a movie, a home video, a series or a folder of videos (see [Folders of videos](addons-and-libraries.md#folders-of-videos)), a season or an episode standing for its series and a video for its folder, and `DELETE` shows it again; both answer 204.
- `GET /Polyfin/UserItems/Hidden` lists the hidden titles, the latest hidden first, as a listing describes them.
- `DELETE /Polyfin/UserItems/{id}/NextUp` takes a series off the user's Next Up, a season or an episode standing for its series, and `POST` puts it back; both answer 204, and 400 for an item that is neither a series nor in one. The series stays off until one of its episodes is played after the removal or one is released after it (see [Continue Watching and Next Up](watch-state.md#continue-watching-and-next-up)).

### Home screen sections for apps

- The choice is the user's `Configuration.LatestItemsExcludes`, a list of library identifiers.
- `/Shows/NextUp` and `/UserItems/Resume` apply it when they have no `parentId`, and `/Shows/NextUp` when it has no `seriesId` either.
- `/Items/Latest` always needs a `parentId` in Polyfin: without one, it answers an empty list.

### Home rows for apps

- `GET /Polyfin/HomeRows` answers the user's rows, in order, as `{"Items": [...]}`: each with its `Id`, its `Name`, its `Place` (`Top`, `AfterResume`, `AfterNextUp` or `Bottom`), its `Kind` (`Catalog`, `Collection`, `BecauseYouWatched`, `New`, `NowAvailable`, `StillWaiting`, `Trakt` or `TopPicks`) and its first `Items`, as `BaseItemDto`, 20 unless `Limit` asks for up to 100. `Fields` asks for their fields, as listings do.
- A row's `Id` is a folder: `/Items?ParentId=` lists the whole row, up to 500 titles, and `/Items/{Id}` describes it.
- The admin API names the two watchlist kinds `available` and `waiting`. Its rows carry a `schedule`, with `startDate` and `endDate` (`YYYY-MM-DD`, empty for none), `days` (0 for Sunday to 6) and `startHour` and `endHour` (0 to 23 and 1 to 24, both null for all day), and `shownNow`.
- `GET /admin/api/home-rows/preview/{userId}` answers the preview: `rows`, each with its folder's `id`, its `rowId`, `name`, `kind`, `place` and `titles` (`id`, `kind`, `name`, `year` and `posterId`).

### Watchlist for apps

- The Watchlist view is a `CollectionFolder` without `CollectionType`, as a library of mixed content, with the same identifier on every server, listed in `/UserViews` after the Collections view.
- `/Items?ParentId=` lists its movies and series, the latest added first, whatever `sortBy` says; `/Items/Latest?ParentId=` gives its home row.
- `POST /Polyfin/UserItems/{itemId}/Watchlist` puts a movie or a series on the user's watchlist (a season or an episode stands for its series) unless a list holds it already, and `DELETE` takes it off every list. Both answer 204, and 400 for an item that is neither.
- `GET /Polyfin/UserItems/Watchlist` answers `{"Items": [{"Id", "AddedAt"}]}`, the titles the user's lists hold, started or not, the latest added first.
- `GET /admin/api/account/tracking` answers, for each service, `watchlist` (Trakt and Simkl) and `importWatchlist`, and `lastImport.watchlist`, the titles added. `PATCH /admin/api/account/tracking/{service}` takes `importWatchlist`, besides `importHistory`, and answers `not_found` for a service without a watchlist.

## Music, audiobooks and podcasts

Music libraries come from music addons and local folders. Jellyfin music apps browse them with Jellyfin's routes, and scrobbles follow the playback reports apps already send.

### Music libraries for apps

- Music libraries have CollectionType `music`. Audiobook tracks are `AudioBook` items; podcast episodes are `Audio` items.
- Jellyfin music apps browse `/Artists`, `/Artists/AlbumArtists`, and `/Items` by `parentId`, `artistIds` or `albumIds`, with Jellyfin's sorting and paging.
- The latest albums, search and search hints go through the addons' `/search`. Artwork comes from the addons.

### Music playback for apps

- A track whose addon gives no stream (it cannot be reached, answers an HTTP error other than `404`, or gives no stream address) does not play: `PlaybackInfo` answers `NoCompatibleStream`, as for a title none of whose versions plays.

### Song lyrics for apps

- `GET /Audio/{itemId}/Lyrics` answers Jellyfin's `LyricDto`. Synced lines have `Text`, `Start` in ticks and an empty `Cues`; plain lines have only `Text`, which apps tell plain lyrics by. `Metadata` gives the song's `Artist`, `Album`, `Title`, `Length` and `IsSynced`.
- A song without lyrics answers `404`, as in Jellyfin.
- Songs carry `HasLyrics`, `true` once their lyrics are known, in listings as in details.
- `GET /Audio/{itemId}/RemoteSearch/Lyrics` answers Jellyfin's `RemoteLyricInfoDto` list: each result's `Id`, `ProviderName` (`LRCLIB`) and `Lyrics`, a `LyricDto` whose `Metadata` describes the song LRCLIB has them for. `POST /Audio/{itemId}/RemoteSearch/Lyrics/{lyricId}` chooses a result and answers the song's new `LyricDto`; an identifier that is no result, and lyrics LRCLIB no longer has, answer `404`.
- `DELETE /Audio/{itemId}/Lyrics` removes the lyrics Polyfin keeps for the song and answers `204`, whether it had lyrics or not, as Jellyfin answers whether it deleted a lyrics file or found none.
- `POST /Audio/{itemId}/Lyrics?fileName={name}` uploads a lyrics file, as jellyfin-web sends it: its text as the body, `text/plain`. Files named `.lrc`, `.elrc` or `.txt` are taken, and the answer is the song's new `LyricDto`. Like Jellyfin, it answers `400` without `fileName`, without a body, for a name without an extension, and for a file it cannot read: another extension, text that is not UTF-8 or holds control characters other than tabs and line breaks, or no line of text. A body over 1 MiB answers `413`.
- These four answer `403` to users other than administrators and API keys, as Jellyfin answers users without the permission to manage lyrics, and `404` for an item that is no song the user sees. Users' `Policy.EnableLyricManagement` is `true` for administrators.

### Scrobbles from playback reports

- There is nothing to add. Polyfin follows the reports apps send for songs: `POST /Sessions/Playing`, `/Sessions/Playing/Progress` and `/Sessions/Playing/Stopped` (and the older `/PlayingItems` ones).
- Send `PositionTicks` and `IsPaused` as playback goes, every few seconds as Jellyfin's apps do, and a new start for each play of a song, repeats included.
- The scrobble goes out with the first report past the threshold, or the stop.

## Versions and PlaybackInfo

A title's versions are the streams its addons and folders list. PlaybackInfo picks among them as Jellyfin picks among media sources, with the differences below.

### Direct play and media source paths for apps

- Direct play answers with a `302` redirect to the provider URL.
- Media sources are `Protocol` `Http`, their `Path` Polyfin's signed address, except for the Jellyfin app for Android's integrated player, which plays a version as it is only from a source described as a file: PlaybackInfo describes its versions that are one file as `Protocol` `File` (see [The integrated player of the Jellyfin app for Android](jellyfin-compatibility.md#the-integrated-player-of-the-jellyfin-app-for-android)).

### Version picking in PlaybackInfo for apps

- Unreadable or unplayable versions are left out of the versions `PlaybackInfo` lists only when the app has not picked a version.
- A `MediaSourceId` equal to the item's own identifier picks no version: `PlaybackInfo` tries the versions in order, as without one, and answers with the version chosen alone, under its own identifier unless it is the first. Any other `MediaSourceId` plays that version or nothing. With **Prefer versions the app plays without conversion**, the item's own identifier from a device whose profile Polyfin already knew picks the first version, the one its version menu showed first (see [Prefer versions the app plays without conversion](versions.md#prefer-versions-the-app-plays-without-conversion)).
- Elsewhere, the item's own identifier as a media source (the `mediaSourceId` of `/Videos/{id}/stream`, subtitle and attachment URLs, playback reports) stands for the first version the item's details and `PlaybackInfo` list, after leaving out the versions that failed lately and those the [version order](versions.md#version-order) rules or the user's quality group leave out, and past the torrents that did not start lately, which a play picking no version passes over. A version that fails thus passes the identifier to the next one, wherever its addon is in the addons' order, and the next one's own identifier keeps naming it. With **Prefer versions the app plays without conversion**, it is the first of the order made for the device the request's token belongs to; a media URL with no token stands for the server's order.
- `StartTimeTicks` in the query or the body tells where an HLS play starts, so that Polyfin reads ahead from there.

### Version list progress for apps

- Item details (`/Items/{id}`, `/Users/{userId}/Items/{id}`) describe the versions known when asked, an expired list's included. With none known, they describe one placeholder source under the title's own identifier, which plays the first version. `PlaybackInfo` waits for every addon asked for the first time, or asked again because its list expired, joining the requests the details started, but never for an addon asked again after an answer. Once an addon gave a version to play, it waits at most 5 seconds for each next answer: an addon slower than that is left out of the play, and its versions reach the next ones (see [How Play waits for addons](versions.md#how-play-waits-for-addons)).
- `GET /Polyfin/Items/{id}/Versions` answers `{"Pending": <addons still asked for the title, for the first time or again>, "Count": <media sources the details would list now>, "Known": <versions known now>, "AddVersions": <whether the web player adds versions to a page already open>}`, with the authentication and access checks of item details. An addon to be asked again counts from its first answer until it is no longer asked. `Count` includes the placeholder; `Known` leaves it out, so it is `0` until a version is known, old versions of an expired list included. Both drop when the versions an addon's new answer lacked are no longer listed. Items other than movies and episodes answer `0` for all three. Polyfin also pushes it on the live connections (WebSockets) of the user's jellyfin-web, the app signed in as `Jellyfin Web`, as it changes, for 2 minutes after the details or this endpoint were last asked, in a `PolyfinVersions` message whose `Data` is `{"ItemId": <the identifier the page was opened with>, "Pending": …, "Count": …, "Known": …, "AddVersions": …, "Tracks": …}`. `Tracks` is `true` when a version of the title was analyzed, or described by RemuxDB once the details had stopped waiting for it, since the last push: the tracks the details list for it may have changed. Other apps get no such message: the Jellyfin SDKs some apps are built on fail on a message type they do not know. `AddVersions` is **Add versions to an open title's page**, under **Settings › Web player**. Polyfin's web player script listens to it, and polls at once when a title's page opens, then every second while `Pending` is above `0`, every 10 seconds once a push came, for at most 150 seconds. The 150 seconds cover a first answer (at most 30 seconds) and both requests that follow it (10 and 30 seconds later, at most 30 seconds each). When `Count` is above what the version menu lists, or differs from it once `Pending` is `0`, the script asks for the item's details (`/Users/{userId}/Items/{id}`) and lists their media sources in the menu. When `AddVersions` is `false`, it does so only while the menu lists the placeholder alone, a single source under the title's own identifier named after the title. Each media source's `ETag` is its version's own identifier, the first one's included. When the version selected loses its identifier, to a source listed first or as Polyfin names it otherwise, the script finds it by the `ETag` it listed it with, else as the one source with its name, and its size when both are known, and selects it under its new identifier before reloading the page's details, which jellyfin-web then keeps selected. A push with `Tracks` has the script ask for the item's details a second later, once no video plays and none of the page's menus has focus, a push heard during a video waiting for the page to show again; when the media source selected lists other tracks there than the Video, Audio and Subtitles menus, by `Index`, the script reloads the page's details, then has jellyfin-web fill the track menus as picking the version does, and picks again the audio and subtitle tracks the user picked, by name. It holds the play buttons while `Known` is `0`, searching while `Pending` is above `0`, with no source once it is `0`. Its first answer may come before the details asked any addon, so it takes no source from it, only from the next. Before any answer, it holds them as searching when the version menu lists a single source whose identifier is the title's own, the placeholder.
- `POST /Polyfin/Items/{id}/Versions/Search` asks the user's stream addons again for a movie or an episode, as [Refresh metadata](jellyfin-compatibility.md#refresh-metadata) does for its streams only: each addon's stream list for the title is dropped, the saved one included, its follow-ups stop, and the addons are asked in the background, as item details ask them. Addons still asked for the title are left to answer. It has the checks of `GET /Polyfin/Items/{id}/Versions` and answers the same progress, `Pending` counting the addons now asked. A user may ask once every 20 seconds for a title; sooner, it answers `429 Too Many Requests` with `Retry-After` in seconds. Items other than movies and episodes ask nothing and answer `0` for all three.
- With **Prepare playback in advance** on, `GET /UserItems/Resume`, `GET /Users/{userId}/Items/Resume` and `GET /Shows/NextUp` queue the first 10 movies and episodes of their answer before answering, and never wait for them. Each queued title is listed waiting for every addon and joining any request already under way. The first 2 then have their first version analyzed.

### Version descriptions from RemuxDB for apps

- Item details and listings describe a version not analyzed yet with the tracks RemuxDB found, numbered as ffprobe numbers them, after the subtitle files as usual. These `MediaStreams` lack what only an analysis reads, such as `TimeBase` and `CodecTag`; the version has no chapters nor attachments yet. `PlaybackInfo` always answers from the analysis.

### Version names for apps

- Only `MediaSources[].Name` changes. `Id` and `ETag` stay, so the web player keeps a picked version selected.

### Versions as media sources

- Versions are Jellyfin media sources.

### Versions a user may not play for apps

- A refused version answers as a missing one on every path: PlaybackInfo, the stream, HLS, downloads, subtitles and attachments.

### Android integrated player for apps

- The integrated player reads `Protocol` to play a source as it is ([QueueManager.kt, lines 342 to 367](https://github.com/jellyfin/jellyfin-android/blob/v2.7.3/app/src/main/java/org/jellyfin/mobile/player/queue/QueueManager.kt#L342-L367)): for `File`, it builds `/Videos/{id}/stream?static=true&playSessionId=&mediaSourceId=&deviceId=` and reads it as one file, with its token in the `Authorization` header; for `Http`, it loads `Path` as an HLS playlist, whatever it holds.
- PlaybackInfo answers `Protocol` `File` when the request comes from the client `Jellyfin for Android` with a `DeviceProfile` named `Jellyfin for Android`, the integrated player's ([DeviceProfileBuilder.kt, lines 165 and 166](https://github.com/jellyfin/jellyfin-android/blob/v2.7.3/app/src/main/java/org/jellyfin/mobile/player/deviceprofile/DeviceProfileBuilder.kt#L165-L166)), for every version that is one file. `Path` stays Polyfin's signed address, which that player does not read. Item details and listings keep `Http`.
- The app is told by the `Authorization` header of the PlaybackInfo request itself, not by its sign-in: an app updated since it signed in keeps its token, and Polyfin keeps the name and version it signed in with.
- Before 2.7.0, the app and that profile were named `Jellyfin Android` ([Constants.kt, line 11, at v2.6.4](https://github.com/jellyfin/jellyfin-android/blob/v2.6.4/app/src/main/java/org/jellyfin/mobile/utils/Constants.kt#L11)). Versions 2.6.3 and 2.6.4 read `Protocol` and send their token as 2.7.3 does, and get `File` too.
- Versions 2.4.0 to 2.6.2 read `Protocol` the same way and build the same address ([MediaQueueManager.kt, lines 146 to 156, at v2.4.0](https://github.com/jellyfin/jellyfin-android/blob/v2.4.0/app/src/main/java/org/jellyfin/mobile/player/source/MediaQueueManager.kt#L146-L156); [QueueManager.kt, lines 210 to 220, at v2.6.2](https://github.com/jellyfin/jellyfin-android/blob/v2.6.2/app/src/main/java/org/jellyfin/mobile/player/queue/QueueManager.kt#L210-L220)), but send no token with it, in a header or in the address: their player sends only its user agent ([AppModule.kt, lines 82 to 105, at v2.6.2](https://github.com/jellyfin/jellyfin-android/blob/v2.6.2/app/src/main/java/org/jellyfin/mobile/app/AppModule.kt#L82-L105)), and their SDK's `getVideoStreamUrl` adds no credentials by default. They get `File` too, and their play session, the one credential that address carries, makes Polyfin relay the version rather than send the player to the source, which may be a redirect from http to https that the player refuses. Its bytes count for the device that asked PlaybackInfo, in Now playing and in the history, as with the token.
- Versions before 2.4.0 build the file's address without the play session ([MediaQueueManager.kt, lines 140 to 149, at v2.3.0](https://github.com/jellyfin/jellyfin-android/blob/v2.3.0/app/src/main/java/org/jellyfin/mobile/player/source/MediaQueueManager.kt#L140-L149)), which tells Polyfin no user. They keep `Http`.
- The address the app builds is served as `Path` is: the play session is a signed grant for the user, the header's token, or before 2.6.3 the play session itself, makes Polyfin relay the source rather than redirect the player, so that a redirect from http to https, which the player refuses, never comes, and byte ranges are answered with 206.

### iOS native player profile for apps

- The native player is told by its device profile's `Name`, `iOS 11+ Native Profile`, `iOS 13+ Native Profile` or `iOS 13+ Native Profile + fMP4` (jellyfin-expo 1.8.0.5's [`utils/profiles`](https://github.com/jellyfin/jellyfin-expo/tree/v1.8.0.5/utils/profiles)), with the client `Jellyfin iOS` or `Jellyfin iPadOS` in the request's `Authorization` header; both are required.
- Its `TranscodingProfile`s with `Protocol` `http` and `Context` `Streaming` are read as HLS, with the `Container`, `MinSegments` and `BreakOnNonKeyFrames` of its first HLS video profile.

### Scrubbing thumbnails for apps

- Apps get images as Jellyfin gives them: the item's `Trickplay` field, by media source and width.
- During a first play, a version's set lists the thumbnails made so far: its `ThumbnailCount` covers the start of the version, and grows. Tiles past it are not found.
- `Trickplay` is keyed as the item's `MediaSources` name the versions (the first under the title's own identifier), and limited to the versions the user is offered under their quality group.
- Tiles are served at `/Videos/{itemId}/Trickplay/{width}/tiles.m3u8` and `{index}.jpg`, which follow parental control, blocked genres and allowed hours.
- Chapters carry an `ImageTag`, served by `/Items/{itemId}/Images/Chapter/{index}` without credentials, like all images.

## Streams and conversion

Polyfin plays a version as it is, relays it, repackages it or converts it. These notes cover the media URLs, the HLS playlists and segments, and what a play reports.

### Who a media URL plays for

Polyfin serves a title's streams, subtitles and attached fonts for the user a media URL names: by the play session or grant Polyfin signed into it, else by the caller's access token. That user's parental control, blocked genres, visible libraries and allowed hours apply, and addon subtitles come synced to the version they play (see [Addon subtitles synced to the version](subtitles.md#addon-subtitles-synced-to-the-version)).

The token is read as on any request: from the `Authorization` header or the `ApiKey` query parameter, and with legacy authorization from the legacy headers and `api_key`. On the routes Jellyfin serves to anyone, `api_key` counts even without legacy authorization, as some apps sign the addresses they build themselves with it: the streams of videos and audio tracks, subtitles, attachments and recordings under way.

- Prefer the `ApiKey` query parameter, which Polyfin's own media URLs use.
- A token in an address never reaches the log: request paths are logged without their query, and the values of `api_key`, `ApiKey` and other tokens are hidden in log lines.

**Compared with Jellyfin**

- Jellyfin 12.4 reads `api_key` only with legacy authorization ([`AuthorizationContext.cs`, lines 108 to 110](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Server.Implementations/Security/AuthorizationContext.cs#L108-L110)), but serves those routes without checking who asks, for example the subtitle streams ([`SubtitleController.cs`, lines 208 and 294](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/SubtitleController.cs#L208)). Polyfin needs to know who plays, and refuses them without a user.
- The routes Jellyfin serves to signed-in callers only keep its rule, and refuse `api_key` without legacy authorization: `/Items/{id}/Download` and `/Items/{id}/File`, `/Audio/{id}/universal`, and the HLS playlists and segments of audio tracks.

### Opening streams and downloading files

Sources need no opening. A title's file download serves its first working version, as the download route does, under the same permissions. Apps that ask for a smaller copy to download, such as Streamyfin, get one while **Converted downloads** is on: see [Converted downloads](transcoding.md#converted-downloads).

- `/LiveStreams/Open` answers 400, as Jellyfin does without an open token; `/LiveStreams/Close` answers 204.
- `/Items/{id}/File` serves the first working version.
- PlaybackInfo, `/Videos/{id}/stream` and `/Items/{id}/Download` answer 400 `Error processing request.` for an item that plays nothing itself, such as a folder of videos, a series, a season or a collection, as Jellyfin does for an item without media sources ([MediaSourceManager.cs, lines 383 to 393](https://github.com/jellyfin/jellyfin/blob/v12.4/Emby.Server.Implementations/Library/MediaSourceManager.cs#L383-L393), answered 400 by [ExceptionMiddleware.cs, line 127](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Middleware/ExceptionMiddleware.cs#L127)). No addon is asked for streams of it. An item that does not exist, or that the user may not see, answers 404.
- `/Videos/{id}/stream.mp4` and `/Videos/{id}/stream.ts` convert only for the `TranscodingUrl` of a converted download: with `static=true`, or without a play session and a `videoCodec`, or with **Converted downloads** off, they serve the version as it is.

**Compared with Jellyfin**

- Jellyfin does not ask for these permissions for its own files.

### First segment reads for apps

- `PlaybackInfo` for a version played over HLS starts reading the bytes of its first segment; with `StartTimeTicks`, those of the segment the app resumes at.

### Torrent start progress

- `GET /Polyfin/Playback/Starting`, signed in, tells how the start of the play of the caller's own session goes: `Stage` (`""` when its play waits for no torrent, `peers`, `details`, `pieces`, or `failed`), `PeersFound`, `PeersConnected`, `Received` and `Waiting` (bytes: those received since the torrent started, and those the reads under way still wait for), and `Failure` for `failed` (`no_peers`, `silent`, `refused`, `no_trackers`, `no_udp`, `route` or `at_cap`). It tells the torrents the session's latest PlaybackInfo tries, then the one it chose for 3 minutes; never another user's or another session's.

### Lower quality menu for apps

- PlaybackInfo's media source says `SupportsTranscoding: true` for a version played as it is under the conditions above, with no `TranscodingUrl`, as from Jellyfin: a `TranscodingUrl` comes only with a source not played as it is.
- A lower `MaxStreamingBitrate` in a new PlaybackInfo, as jellyfin-web sends it, gives the `TranscodingUrl` of the conversion.

### Adaptive quality playlists for apps

- The master playlist lists the first quality first, then the lower ones, each with its own `BANDWIDTH` (its video's peak with the audio sent), `AVERAGE-BANDWIDTH` (its video's bitrate with the audio sent), `RESOLUTION` and `CODECS`, and says `#EXT-X-INDEPENDENT-SEGMENTS`. Every quality is cut on the same plan, with a keyframe forced at each segment's start, so segment n covers the same time in each, and an app may switch at any segment.
- The first quality's media playlist is `main.m3u8`, its segments under `hls1/main/`, as with one quality. The lower ones' are `hls1/main1/main.m3u8` and `hls1/main2/main.m3u8`, their segments beside them under `hls1/main1/` and `hls1/main2/`, in the same play session and with the same query.
- Asking for one quality never stops another: an app may ask for the same segment of two qualities at once, and gets both.

### Converted download streams for apps

- A device profile whose video `TranscodingProfile` has `Protocol` `http` and `Container` `mp4` or `ts` gets, when the version does not direct play, a `TranscodingUrl` of the form `/videos/{id}/stream.mp4?…`, with `TranscodingSubProtocol` `http`, as from Jellyfin. Polyfin sends one fragmented MP4 file, or MPEG-TS for `ts`, as FFmpeg writes it, without `Content-Length` and without byte ranges: a request with a `Range` header gets the whole file (200).
- The Jellyfin iOS app's native player never gets one: its plays go over HLS (see [Two players in the Jellyfin iOS app](transcoding.md#two-players-in-the-jellyfin-ios-app)).
- `startTimeTicks` starts the copy later; the file then starts at zero.
- The `TranscodingUrl` stands for the user PlaybackInfo answered, through its play session. With another user's token, it is refused (403).
- `DELETE /Videos/ActiveEncodings?playSessionId=` stops it.
- `/Items/{id}/Download` and `/Videos/{id}/stream?static=true` keep serving the file as it is, under the quality group rule. So does a `TranscodingUrl` kept from before the setting was turned off.

### Conversion cap answers for apps

- At the cap, `PlaybackInfo` answers `NoCompatibleStream` when no version plays without conversion.
- An HLS request that would start one more conversion answers `503`, as when Live TV is at its cap.

### HLS segment cutting for apps

Repackaged and converted plays of files are cut into HLS segments on the source's own keyframes:

- The first segment ends on the first keyframe at least 2 seconds in, and each next one on the first keyframe at least 4 seconds after its start. A file with a keyframe every 2 seconds plays its first segment after 2 seconds of picture; one with a keyframe every 10 seconds gets 10-second segments.
- An MPEG-TS file of a local folder or a network share (`.ts`, `.m2ts`, `.mts`, `.m2t`, `.tp`, `.trp`) has no keyframe index either: Polyfin reads its keyframes once, with ffprobe reading the whole file, as Jellyfin's keyframe extraction does. It does so in the background when the title's details open, with [Prepare playback in advance](reading-the-sources.md#preparing-playback-in-advance), else at the file's first play over HLS, which waits for the read 5 seconds at most: about what a recording of an hour or two takes on a local SSD. A play that comes before the read is done plays converted and cut every few seconds, and keeps that cut to its end, while the read goes on, 3 minutes at most; its version is named "(converted)" meanwhile. The keyframes are kept in the database, as a file's index is, and the next plays cut the file on them as other files are: its video is copied when the app takes its codec, a user who may not have video converted plays it, and a seek lands where asked, Polyfin starting FFmpeg a segment early as below. A file whose keyframes cannot be read within 3 minutes, or are unusable, as in a file joined from recordings whose times go back where the next one starts, plays converted and cut every few seconds, as before, and is not read again for 6 hours.
- An addon's MPEG-TS file, a local MPEG-TS file whose keyframes could not be read, an MPEG program stream (`.mpg`, `.mpeg`, `.vob`) or an AVI file, which have no keyframe index Polyfin reads, is converted and cut the same way: 2 seconds, then every 4 seconds, even when the app takes its video, as browsers take H.264. Its segments count from its start, whatever time its first packet tells, such as an hour into a broadcast, and a seek lands where asked, however far apart its keyframes are. An AVI file's index lists its frames in decoding order, without the times they show at, which B-frames reorder: its video could not be copied with the right times even with an index. A user who may not have video converted is not offered such a file over HLS, and its version's name says so (see [Version names](versions.md#version-names)). Apps that play the file as it is, such as Android TV and Swiftfin for MPEG-TS, need no segments (see [Video files](naming-and-matching.md#video-files)).
- A Blu-ray disc is cut on the keyframes its clips' information files list, and its video copied when the app takes it; a DVD is converted and cut every few seconds, as an MPEG-TS file is. See [Local folders](discs.md#blu-ray-discs).
- Polyfin makes segments up to **Seconds prepared ahead** past the last one the app asked for, then waits for the app.
- An app asking for a segment whose source stopped sending anything for 20 seconds gets `503` at once, rather than waiting minutes. Its next request waits for the same conversion, which goes on when the source answers again. A source that fails for good reads on from a mirror of its file, or else the play goes on from another version of the title, at the same time and with the same segment numbers, converted into what the app plays (see [Carrying a play on](reading-the-sources.md#carrying-a-play-on)); when neither can, its conversion ends, and the next request answers `503` too.
- Repackaging or conversion stops when its app stops its play session (`/Sessions/Playing/Stopped`, or `DELETE /Videos/ActiveEncodings`), and once its app has asked nothing of it for 3 minutes, as when a player stays paused; it starts again at the next request.
- An app may play another title without stopping the one before, as an app that sends no stop does, or one that crashed and started again. As it reports the new title, the repackaging and conversions of the play sessions its device reported for the title before stop, as its stop would have stopped them, and they no longer read their source, a torrent's swarm included (see [Torrents](torrents.md#only-while-playing)). The same title played again, as for another audio track, stops nothing: the app stops what it leaves itself. Only the device's latest playback changes its title: a start, or a report of its latest play session, or of one it never reported before. A late report of a play session of the title it left, such as a progress sent before the switch that arrives after it, stops nothing. The playbacks of the user's other devices, of DLNA devices, of guests through share links and of converted downloads go on.

- `EXT-X-TARGETDURATION` is the longest segment, rounded up.
- After a seek, FFmpeg probes 2 MB of the file's head, which the analysis already read, rather than 5 MB; conversions burning subtitles in, files cut every few seconds and discs probe as usual.
- After a seek in an MPEG-TS file, an MPEG program stream, an AVI file or a disc, FFmpeg starts at the previous segment's start, as its MPEG-TS and MPEG program stream readers land on the first keyframe after the time asked, and an AVI file's keyframe may show after the time it is listed at; what comes before the segment asked is left out, without being converted. A seek to the second segment reads the file from its start.
- When FFmpeg still lands past the segment's start, as in a file with a keyframe every 10 seconds, it starts again further back: as much further back as it went past the time asked, so at least twice as far, until it lands before the segment or reads the file from its start. Each try costs an FFmpeg start; the frames read before the segment are decoded but not converted.
- Polyfin keeps that distance with the file, in its database: the playback's later seeks, and those of the next playbacks of the same file, by any user and after a restart too, start as far back, and land where asked with one FFmpeg start. It knows a local file by its folder, path, size and modification time, so that a file that changed learns the distance again; an addon's file by its version, which follows the file by its name and size, else its link (see [Playback](playback.md)); an IPTV catch-up programme by its channel, whose programmes usually come from the same encoder; a recording by itself. A playback that needs to start further back keeps the larger distance; a shorter one never replaces it. Files FFmpeg lands in where asked, such as Matroska and MP4 files, never use it.

**Compared with Jellyfin**

- Jellyfin cuts every file but a Matroska one into segments of equal length when it repackages it, FFmpeg ending each on the next keyframe, so that their lengths in the playlist are approximate ([DynamicHlsPlaylistGenerator.cs, lines 37 to 47](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs#L37-L47); [EncodingOptions.cs, line 63](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Model/Configuration/EncodingOptions.cs#L63)), and copies the video of an MPEG-TS file or an MPEG program stream when the app takes its codec ([`DynamicHlsController.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/Jellyfin.Api/Controllers/DynamicHlsController.cs#L1853-L1866)). Polyfin's segments are exact, of the lengths its playlist gives, which, without an index, needs the video converted. So it copies the video of a local MPEG-TS file only once it has read the file's keyframes, which Jellyfin reads from the whole file only in its scheduled keyframe extraction, and does not use for MPEG-TS files ([`FfProbeKeyframeExtractor.cs`](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.MediaEncoding.Keyframes/FfProbe/FfProbeKeyframeExtractor.cs#L28-L33); [DynamicHlsPlaylistGenerator.cs, line 30](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs#L30)). An addon's MPEG-TS file is not read whole, and plays converted. Both convert the H.264 of an AVI file ([EncodingHelper.cs, lines 2630 to 2636](https://github.com/jellyfin/jellyfin/blob/v12.4/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs#L2630-L2636)), and the MPEG-2 and MPEG-4 Part 2 video no browser plays.
- Jellyfin stops repackaging or a conversion one minute after its last segment request or playback report naming its play session, and never because its device started another title: a conversion left without a stop runs on for that minute. Polyfin keeps a paused conversion 3 minutes, and stops that of a title its device left at once.

### Fast start segments for apps

- **Playlist:** an `#EXT-X-DISCONTINUITY` comes before the first segment after the fast ones, and before the first fast one when the play resumes past the start. In fragmented MP4, the fast segments have their own initialization segment, `-2.mp4`, which an `#EXT-X-MAP` names after the discontinuity, `-1.mp4` naming the others'.
- A play PlaybackInfo starts with fast segments has `FastStartTicks`, where it starts, last in its `TranscodingUrl`; every request of the play cuts it the same way.

### Audio track switching for apps

Apps change the audio track of a converted video by playing it again: jellyfin-web asks `PlaybackInfo` for the new track, stops the old play session with `DELETE /Videos/ActiveEncodings`, and loads the new stream from the same position; Jellyfin Android TV, Swiftfin and Streamyfin stop the player and play again too. Without the setting below, the new session converts the video again from the keyframe before the position, though the picture has not changed, and the segments already made are deleted.

- **Video and audio apart:** a converting FFmpeg makes the video alone, and a second FFmpeg the audio track played, copied or converted as before, cut at the same segment times. The audio's FFmpeg reads the file through the source cache, as the video's does, and makes no segment the video has not reached, nor further than **Seconds prepared ahead**. With [several qualities](transcoding.md#adaptive-quality), one audio FFmpeg serves them all.
- **Joined as served:** each segment joins the video and the audio of the same times, copying both, in fragmented MP4 (one initialization segment holding both tracks) and in MPEG-TS (the audio's packets interleaved with the video's). Apps see the same endpoints, master playlist, `CODECS` and segment numbers as before.
- **Kept video:** the converted video is known by its user, device, version, segment format and conversion (codec, size, bitrate, tone mapping, burned subtitle, quality), not by its play session or audio track. Once its play session stops, it is kept 30 seconds, its FFmpeg waiting where it was. A new play session asking for the same video takes it over, with the segments made: its track's FFmpeg starts at the segment the app asks for, the old track's stops, and the first segment after the change comes once its audio is made, without converting the picture again.
- **Late stops:** jellyfin-web sends the old session's `DELETE` again once the new stream plays; it stops nothing the new session took over.

**Why the audio stays inside the segments:** HLS can carry audio tracks as renditions of their own, which a player switches between without reloading the stream. Jellyfin's master playlists list subtitle renditions only, and its apps never switch audio inside a converted stream: they ask for a new one, as above. Separate audio renditions would go unused, and players that take only the main playlist's segments would play no sound. Keeping each segment whole, its audio joined as it is served, works with every app as it is.

### Streamed play information for apps

- `GET /Sessions` fills `TranscodingInfo` once the play session's first encoding opened its source, from the play session the app reported in `/Sessions/Playing`; nothing for a play reported as `DirectPlay`.

## Subtitles and skip buttons

Subtitles come from addons, from files beside local videos and from tracks inside the files. Skip buttons read Jellyfin's media segments.

### Subtitle files for apps

- Add: `POST /Videos/{id}/Subtitles`.
- Delete: `DELETE /Videos/{id}/Subtitles/{index}`; addon subtitles and embedded tracks answer 400.

### PGS subtitle files for apps

- `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/0/Stream.pgssub` (or `.sup`) answers `application/octet-stream` and supports `Range`.

### Text copies of picture subtitles for apps

- The copy is one more subtitle `MediaStream`, after the version's tracks so that no index moves.
- `Codec` is `subrip`; `IsTextSubtitleStream`, `SupportsExternalStream` and `IsExternal` are true; `Language`, `IsForced` and `IsHearingImpaired` are the original's.
- `Title` is the original's title or language followed by "(OCR)", and `DisplayTitle` reads like `English (OCR) - SUBRIP - External`.
- It is served as other text subtitles are: `DeliveryMethod` `External` in the format asked (`.vtt`, `.srt`, `.ass` or `.js`), or `Hls`.
- When `PlaybackInfo` names no subtitle track, or names the version's `DefaultSubtitleStreamIndex` as the item gives it (jellyfin-web always sends it back), the default choice ranks the original, and gives the copy's index for a profile that would have the original burned in (`Encode`). Another track the app asks for is delivered as asked, the original still burned in when asked.
- A copy is listed from the first `PlaybackInfo` after it is read; no index of an earlier answer changes.

### Fallback fonts for apps

- `/FallbackFont/Fonts`.

### Media segments for apps

- Turned off, `/MediaSegments/{id}` lists none.
- Turned on, the media sources of movies and episodes say `HasSegments: true` while a database is asked, when the version's own chapters name an intro, a recap, a preview or credits (see [Segments of each version](skip-segments.md#segments-of-each-version)), or when an intro or credits were found in the audio of an episode's local file (see [Intros and credits found in the audio of local episodes](skip-segments.md#intros-and-credits-found-in-the-audio-of-local-episodes)). The media source of a recording says it when commercial breaks were found in it (see [Commercials of recordings](skip-segments.md#commercials-of-recordings)). jellyfin-web asks for segments only then, by the identifier of the version it plays, which `/MediaSegments/{id}` takes as well as the title's. The title's identifier stands for its first version.
- `/MediaSegments/{id}` answers Jellyfin's `MediaSegmentDto` list, in ticks, filtered by `includeSegmentTypes`, repeated or comma-separated. Apps choose what to do with each type; the server has no setting per type.
- `GET /Polyfin/MediaSegments/{id}` lists the same segments, every type, each with its `Type`, `StartTicks` and `EndTicks` and where it comes from, which Jellyfin's segments do not tell: `Found`, `database`, `chapters` or `audio`, absent for the commercials of recordings; `Database`, the database of a `database` segment, as `POLYFIN_SEGMENTS` names it; and `Confidence`, from 0 to 1, the database's own rating of the segment, which only SkipDB gives. Credits split around a scene keep where they come from. [Silo's apps](silo-apps.md#skip-markers) get them this way.
- `/MediaSegments/{id}` lists the credits of such a version as several `Outro` segments, one before each scene and one after the last when they go on after it.

## Live TV, recordings and DLNA

Channels, the programme guide, recordings and HDHomeRun tuners use Jellyfin's Live TV routes. Reminders and DLNA devices are Polyfin's own.

### Channel start and PlaybackInfo timing

- The 20 seconds, with the analysis, stay within what the Android apps' client gives PlaybackInfo: 30 seconds in all.
- While it waits, PlaybackInfo sends its answer's status after 5 seconds, then a space every 5 seconds, which JSON allows before the answer. Swiftfin gives up on an answer after 20 seconds without a byte.

### Programme guide fields for apps

- Programmes from either guide give apps their episode title and their season and episode numbers, as Jellyfin's do.
- XMLTV programmes the guide marks previously shown, and not new, are repeats (`IsRepeat`), as Jellyfin reads XMLTV guides.
- `/LiveTv/GuideInfo` still answers the seven days Jellyfin's guide spans.
- `/LiveTv/Programs/Recommended?isAiring=true` answers from the channel and guide pages Polyfin keeps, and never waits for an addon. A page it lacks is fetched in the background, once for every request that needs it meanwhile, and listed from the next request. jellyfin-web's home fills none of its rows before this request answers (see [On Now](live-tv.md#on-now)).

### Programme reminder routes

Polyfin's own routes, under the Live TV permission:

- `GET /Polyfin/Reminders` lists the user's reminders and series reminders.
- `GET /Polyfin/Reminders/Programs/{programId}` tells how a programme stands (`Started`, `ReminderId`, `SeriesReminderId`, `Reaches`, `BeforeMinutes`). It answers 204 with nothing, without the permission too, for an item that is no programme of the user's guide or for a user without Live TV, as the web player's script asks it of every page.
- `POST /Polyfin/Reminders` with `ProgramId`, `Series` and `Language` sets one.
- `DELETE /Polyfin/Reminders/{id}` deletes a reminder or a series reminder.

### Recordings and timers for apps

- Guide programmes show their timers (`TimerId`, `SeriesTimerId`).
- A recording under way can be followed at `/LiveTv/LiveRecordings/{id}/stream`. Unlike Jellyfin, it needs a signed-in user's credentials or play session, like Polyfin's other media.
- Recordings are listed under `/LiveTv/Recordings`, and the folder at `/LiveTv/Recordings/Folders`.
- A timer or series timer asked for a programme Polyfin does not know answers 404 (`Timers/Defaults`) or 400 (`SeriesTimers`), where Jellyfin fails with 500.
- `POST /LiveTv/Timers/{id}` changes the timer the route names, not the one in the body.

### Replay view for apps

- The Replay view is a `CollectionFolder` of `CollectionType` `folders`, with the same identifier on every server, listed in `/UserViews` after Live TV. Apps open it as a folder list; it has no Latest row.
- Its folders are `Folder` items. Its programmes are `Video` items with `ChannelId`, `StartDate`, `EndDate`, `RunTimeTicks`, and `DateCreated` set to the air time. They play through `PlaybackInfo` and the video stream and HLS routes, as movies do.
- `/UserItems/Resume` lists them with movies and episodes, as Jellyfin lists its other videos there.

### Live TV tuners for apps

- `/LiveTv/TunerHosts/Types` lists HDHomeRun (`hdhomerun`), as Jellyfin names it; M3U tuners are not offered, as IPTV sources are added in the admin app.
- `/LiveTv/Tuners/Discover` (and Jellyfin's `/LiveTv/Tuners/Discvover`) lists the tuners that answer discovery, as Jellyfin's `TunerHostInfo`: `Type`, `Url`, `DeviceId`, `FriendlyName`, `TunerCount`, without an `Id`. With `newDevicesOnly=true`, those already added are left out. Administrators only.
- `POST /LiveTv/TunerHosts` adds a tuner from its `Url`, with `ImportFavoritesOnly`, and answers it with its `Id`; with the `Id` of a tuner, it changes its address and `ImportFavoritesOnly`. The tuner is read first: one that does not answer is refused (500), and so is one already added (400). Another `Type` answers 404, as in Jellyfin. Administrators only.
- `DELETE /LiveTv/TunerHosts?id=` removes the tuner, and answers 204 for an identifier that is not a tuner's, as Jellyfin does, leaving other IPTV sources be.
- `/System/Configuration/livetv` is Jellyfin's `LiveTvOptions`: the recordings' paddings in `PrePaddingSeconds` and `PostPaddingSeconds`, and, to administrators, the recordings folder in `RecordingPath` (null while recording is off) and the tuners in `TunerHosts`; its other settings are Jellyfin's defaults. A member gets no folder or tuner there, as they are of the server's own disk and local network. Polyfin's recording and guide settings are changed in the admin app (see [Live TV](live-tv.md)), not through this route.
- Settings Polyfin's tuners do not have take Jellyfin's defaults: `AllowStreamSharing` is true, as a channel's stream is always shared between its viewers.

### DLNA sessions for apps

- A DLNA playback is listed in `/Sessions` under its user, with `Client` `DLNA`, `DeviceName` the device's name, `DeviceId` `dlna-` and the device's ID, `RemoteEndPoint` its address, `NowPlayingItem` and `PlayState`, until two minutes after the device last asked for anything. `SupportsRemoteControl` and `SupportsMediaControl` are false, and a listing with `controllableByUserId` leaves it out.
- `PositionTicks` is how far the device read, as Polyfin infers it: devices report nothing. For a file sent as it is, that is the share of the file's bytes sent during the playback, wherever they are in it. A DLNA playback counts against the user's **Playbacks at once**, as an app's does.

## Silo apps

Silo's apps use their own API, under `/api/v2/`. These notes cover what Polyfin answers there.

### Silo box sets API

- `UserLibrary.box_sets_enabled` is true on the libraries of movies, and `box_sets` true in `GET /api/v2/catalog/search/capabilities`, while the key is saved. `GET /api/v2/catalog/box-sets/{id}` needs `library_id` (422 without), and answers 404 for a box set the library does not show the user; `GET /api/v2/catalog/items/{id}/box-set` answers `{}` for a movie in none, and 404 for a season or an episode. `group=box_set` is answered as without it, in `GET /api/v2/catalog` and `POST /api/v2/catalog/query`, as Silo's contract has had since it dropped it. Without the key, the three operations answer 404, as on a server without box sets. A box set's artwork is at `/api/v2/artwork/box-sets/{id}/Primary` and `Backdrop`, at the width the app asks, as titles' artwork is at `/api/v2/artwork/{id}/{kind}`.

### Silo TV sign-in with a code

- `GET /api/v2/auth/device/capability` answers the `state` `available` with `cancel` while Quick Connect is on, `disabled` otherwise. `remote_playback_handoff`, `jellyfin_quick_connect` and `opened_signal` are false.
- `POST /api/v2/auth/device/start` answers 201 with a `user_code` written `4821-7730`, `verification_uri` ending in `/activate`, `verification_uri_complete` with `?code=48217730`, `expires_in` 600 and `interval` 5. A remote playback's start (`client_purpose` `remote_playback`, or `temporary`) is 501 `capability_unsupported`; while 1,000 Quick Connect requests are pending, a start is 429 `rate_limited`, which the TV retries; with Quick Connect off, every operation below is 404.
- `POST /api/v2/auth/device/poll` answers `pending` with `expires_at`, then `approved` with `tokens` once, `consumed` after. `denied` and `canceled` end it too; a request that expired, or never was, is 404.
- `POST /api/v2/auth/device/cancel` withdraws a request pending or approved but not collected, and answers its state.
- `GET /api/v2/auth/device?code=`, `POST /api/v2/auth/device/approve` and `POST /api/v2/auth/device/deny` need a signed-in session (401 without). A code no TV waits with is 404 `not_found`; one already decided, 409 `already_decided`.
- `GET /activate?code=` leads to `/admin/me/quick-connect?code=`.
- Polyfin's tests check these answers against the rules the apps apply to them, read from their code at the commits named in [The contract followed](silo-apps.md#the-contract-followed).

## Server administration from Jellyfin apps

The web player's dashboard and other Jellyfin apps reach the server's administration through Jellyfin's routes. Polyfin answers them for what it has, and as an empty Jellyfin server for the rest.

### Device list in the Jellyfin API

- Devices are at `/Devices`.

### Server configuration in the Jellyfin API

- Configuration: `/System/Configuration`.
- Libraries: `/Library/MediaFolders`.
- Storage: `/System/Info/Storage` lists the cache folder, the conversion folder's segments as the transcoding folder, and, while recording is on, the recordings folder.
- The authentication providers are available too.

### Server logs in the Jellyfin API

- Server log: `/System/Logs`.
- Uploaded app logs: `/ClientLog/Document`, 1 MB at most.

### Scheduled tasks in the Jellyfin API

- Tasks are at `/ScheduledTasks`, every one listed, none hidden. Changing a schedule answers 400.

### Backups for apps

- Administrators only, as in Jellyfin. While backups are off, `/Backup` lists nothing, `/Backup/Create` answers 403, and `/Backup/Restore` answers 404.
- `GET /Backup` lists the backups of the folder, newest first, as `BackupManifestDto`: `DateCreated` (when the backup was made), `Path` (the file name, such as `polyfin-20261005-040000.dump`; Jellyfin gives the archive's full path), `ServerVersion` (the Jellyfin version of `/System/Info`), `BackupEngineVersion` (`1.0.0`, Polyfin's format), and `Options` with only `Database` true. The backups made before a restore are listed too.
- `POST /Backup/Create` runs **Back up the database** at once, sends the backup to the destinations, and answers its manifest. The options of the body are ignored.
- `GET /Backup/Manifest?path=` answers the manifest of the backup of the folder named `path`, and 404 for any other path.
- `POST /Backup/Restore` with `ArchiveFileName`, a file name `/Backup` lists, restores it as the admin app does, and answers 204: Polyfin backs the current database up, stops, restores it and starts again, answering 503 meanwhile. It answers 404 for a backup the folder does not have, and 400 for a file that is not a backup of Polyfin's database, a backup of a newer Polyfin, a server without `pg_restore`, a restore already under way, and a backup holding stored keys the current `POLYFIN_SECRET_KEY` cannot decrypt, as apps cannot show that warning: the admin app restores those.
- An encrypted backup (`.dump.enc`, see [Encrypting the backups](backups.md#encrypting-the-backups)) is listed by `/Backup` like the others, but `POST /Backup/Restore` refuses it with 400 and the text `This backup is encrypted: restore it from Polyfin's admin app, which asks for its passphrase.`, as apps cannot ask for one. Jellyfin answers only 204 or 404 there ([BackupController.cs](https://github.com/jellyfin/jellyfin/blob/07866611e9d8/Jellyfin.Api/Controllers/BackupController.cs#L57-L70)): its backups have no passphrase.

### Plugins, packages and the startup wizard for apps

- Answers go to administrators only where Jellyfin requires one.
- `/Plugins`, `/Packages`, `/Repositories`, `/Environment/Drives`, `/Environment/DirectoryContents` and `/Library/PhysicalPaths` list nothing.
- `/Libraries/AvailableOptions` lists no provider (Jellyfin also lists its own, its NFO reader and saver among them).
- `/System/Configuration/MetadataOptions/Default` and `/LiveTv/ListingProviders/Default` are Jellyfin's defaults.
- `/Startup/Configuration` and `/Startup/User` show the server name, language and earliest administrator; setup is always complete.
- Writes are refused as Jellyfin refuses them for what does not exist (an unknown plugin or package, path, or listing provider).
- Writes Jellyfin would perform answer 403: saving plugin repositories, and the wizard's configuration, remote access and first user. Change server settings through `/System/Configuration` instead.
- `/Environment/ValidatePath` answers 404 for every path.

## Web player themes and branding

The web player loads its theme, custom CSS and custom JavaScript through Jellyfin's branding routes and Polyfin's own script.

### Web player theme loading for apps

- The page describes the themes in its head, before Polyfin's script, as JSON in `<script type="application/json" id="polyfinWebThemes">`: `Cookie`, the name of the cookie the script names the theme it shows in, `polyfinWebTheme_<server ID>`; `Theme`, the server's theme; and `Themes`, the `Css` and `Js` addresses of each theme that has some, relative to the page.
- The script writes the theme's `<link rel="stylesheet" id="polyfinWebThemeCss">` and its `<script defer>` while the page is parsed, and takes its stylesheet away once jellyfin-web shows the same, loaded. A theme changed at once has its stylesheet at the end of `<body>`. The script keeps what it knows in the browser's storage under `polyfinWebTheme`, and drops what jellyfin-web kept in IndexedDB when the stylesheet differs from the last load's.
- jellyfin-web's SDK asks for the branding with `fetch` and `credentials: 'omit'`, without the browser's cookies: the script sends the page's cookies with jellyfin-web's requests to `/Branding/Configuration` and `/Branding/Css`, and with no other, so that the branding gives the CSS of the theme the page shows.
- `GET /Polyfin/WebTheme`, with the user's token, answers `{"Theme": "none", "UserTheme": "none"}`: the theme the web player shows the user, and their own, empty while they keep the server's. An API key gets the server's.

### Branding CSS for apps

- Served at `/Branding/Css` and `/Branding/Css.css` (`text/css`, empty when there is none), and in `CustomCss` of `/Branding/Configuration`: with the cookie `polyfinWebTheme_<server ID>`, the CSS of the theme it names, `lumaaglaass`, `none` or `custom`; else, with a user's token, the CSS of their theme; with an API key, the server's; otherwise none.
- With the cookie, `/Branding/Configuration` gives the **Custom CSS** as the web player's page loads it, `@import url('custom.css?v=<hash>');`, relative to the web player's address; `/Branding/Css` gives it as written.
- `/web/custom.css` serves **Custom CSS**, while it is set, as `text/css`, cached for a year at the address with the hash of its content, and revalidated at any other.

### Branding configuration for apps

- Set with `POST /System/Configuration/branding`; read with `GET /System/Configuration/branding`.

### Custom JavaScript for apps

- The tag is `<script src="custom.js?v=<hash>" defer></script>` for the Custom theme, and `<script src="polyfin/themes/lumaaglaass/<version>/loader.js" defer></script>` for LumaaGlaass, whose loader is the text the **Custom JavaScript** field shows for it.
- `/web/custom.js` is served as `application/javascript`, while **Custom JavaScript** is set, whatever the server's theme. It is cached for a year at the address with the hash of its content, which changes with it, and revalidated at any other address.
- Without a script, there is no tag; without **Custom JavaScript**, no `/web/custom.js`.

### Theme copy for apps

- `GET /admin/api/settings/theme-copy` answers the copy, `copy`, null for none, with its `savedAt`, `size` in bytes, `files` (each with its `address`, the `text` that named it in the texts, its `name`, `kind` and `size`) and `unsaved` (each with its `address`, `reason` and a `detail` for some); `missing`, the names of its files not in its folder; `outside`, the addresses the texts load from elsewhere; and `limits`.
- `POST` keeps a copy, or makes it again, and `DELETE` removes it, both answering the same.
- Refusals: `409 theme_copy_running` while another copy is being made; `400 theme_copy_nothing` when the texts load nothing from elsewhere; `422 theme_copy_failed`, with `unsaved`, when no file could be saved; `409 custom_texts_changed` when the texts changed while the files were downloaded; `404 not_found` to remove no copy.
- The reasons are `private_address`, `not_http`, `unavailable` (with the HTTP status), `timeout`, `wrong_type` (with the type sent), `too_large`, `total_too_large`, `too_many_files`, `too_deep` and `built_at_run_time`.

## Admin app settings and health

The admin app reads and saves the settings at `/admin/api/settings`, and the health page at `/admin/api/health`. A refused setting answers 400 with an error code.

### Playback bandwidth in the admin API

- Each playback of `GET /admin/api/sessions` has `bandwidth`, with `sent` and `received` in bytes per second, and `redirected`; and `origin`, with `kind` (`addon`, `torrent`, `localFolder`, `networkShare` or `iptv`), `name` and, for a torrent, `peers`, or null when not known.
- `GET /admin/api/sessions` answers an administrator every playback, and a member only their own, without the guests playing through their share links. Stopping a playback and sending it a message stay the administrators'.
- `GET /admin/api/bandwidth`, for administrators, answers the server's totals: `sent` and `received`, in bytes per second.

### Health problems in the admin API

`GET /admin/api/health/problems` lists:

- a picture subtitle language as `subtitle_ocr_language`, with `name`, `file`, `download` true when it can be downloaded, and `error`;
- a return to the previous version of Polyfin Desktop as `desktop_rolled_back`, with `name`, the version that did not start, `previous` and `error`;
- an unwritable folder as `folder_unwritable`, with `folder` (`conversion`, `reading` or `cache`), `path` and `error`;
- torrent streams that cannot play as `torrent_route`, with `failure` and `name`, and `grace`, the seconds the torrents playing go on for, while they ride the outage out;
- a torrent that stopped while it played as `torrent_stopped`, with `at`, `failure` (`proxy_unreachable`, `proxy_refused`, `interface_missing`, `interface_down`, `interface_no_address`, `wireguard_endpoint`, `wireguard_down`, `wireguard_handshake`, or `route` for another failure) and `name`, the interface, proxy or WireGuard server;
- a host answering an error file instead of the videos as `host_error_file`, with `host`, its name only, and `at`, when it started. `GET /admin/api/titles/{id}/versions` gives it on each stream from that host as `hostErrorFile`, with `host` and `since`, `null` for none.

Also:

- `GET /admin/api/health` gives the end of an addon's wait for its catalog pages as the addon's `catalogsWaitUntil`, `null` when none runs, and lists the torrent outages in `torrents.outages`, oldest first, each with `at` and `seconds`.
- `GET /admin/api/health/goroutines`, for administrators only, answers the goroutines file of **Download the goroutines**.
- `POST /admin/api/health/languages/{language}/download`, for administrators, with the language's Tesseract code such as `fin`, answers 204 once the file is kept; 404 `tessdata_unknown_language` for a language Polyfin has no file for; 409 `tessdata_no_folder` without `POLYFIN_TESSDATA_DIR`; 500 `tessdata_folder` for a folder that cannot be written and 502 `tessdata_download` for a download that failed, each with the cause in `detail`; and 502 `tessdata_checksum` for a file whose SHA-256 differs.
- `/admin/api/settings` holds the automatic download of Tesseract data as `subtitleOcrDownload`, and the folders as `conversionFolder`, `readingFolder` and `cacheFolder`, with `cacheFolderDefault`. A `PUT` with a folder Polyfin cannot write into answers 400 `invalid_conversion_folder`, `invalid_reading_folder` or `invalid_cache_folder`.

### Artwork and people health in the admin API

- `GET /admin/api/health` gives them as `artwork`, with `failed`, `kinds` (`not_found`, `rate_limited`, `timeout`, `server_error`, `other`), `hosts` (`host`, `failed`), `waiting` and `spared` (`host`, `until`);
- and `people`, with `keySaved`, `found`, `notFound`, `failing`, `waiting`, `limitedAt` (`null` for none) and `limitWait` (seconds, `0` for none).

### Image cache in the admin API

- `GET /admin/api/health` gives the cache as `images`: `files` and `used` (bytes) on disk, `limit`, `memoryImages` and `memoryBytes`.
- `POST /admin/api/health/images/clear`, for administrators, empties it and answers what it freed in the same shape.

### Source cache in the admin API

- `GET /admin/api/health` gives the cache as `cache`: `used`, `limit`, `sources` (files open), `kept` (files with parts kept), `hits` and `misses` (bytes read since the start that were read before or kept from before the start, and bytes read for the first time since they were fetched), `days` and `torrents`. `GET /admin/api/settings` gives `sourceCachePreset`, `sourceCacheDays`, `sourceCacheCreditDays`, each preset's values as `sourceCachePresetLimits`, and `readingFree`, the bytes free on the disk of the folder for files being read. A `PUT` that sets `cacheSizeGb`, `sourceCacheDays`, `sourceCacheCreditDays` or `preloadWholeFiles` to another value than its preset's, without a preset, makes it `custom`.

### Local network discovery in the admin API

- `GET /admin/api/health/problems` lists the warning of a port it cannot listen on as `discovery_port`, with `port`, and the warning of a container's address as `discovery_address`, with `address`, the address announced.
- `/admin/api/settings` holds the settings as `localDiscovery` and `localAddress`. A `PUT` whose address is not an `http` or `https` URL, or has a user, query or fragment, answers 400 `invalid_local_address`.
- For tests, `POLYFIN_DISCOVERY_PORT` changes the UDP port, which Jellyfin apps never use.

### HTTPS settings in the admin API

- `/admin/api/settings` holds the settings as `httpsMode` (`off`, `files` or `letsencrypt`), `httpsPort`, `httpsRedirect`, `httpsCertificate`, `httpsPrivateKey`, `acmeHosts`, `acmeEmail`, `acmeTerms` and `acmeChallenge` (`http-01` or `tls-alpn-01`).
- A `PUT` with files that cannot be served answers 400 `https_certificate_missing`, `https_private_key_missing`, `https_certificate_unreadable`, `https_private_key_unreadable` or `https_files_mismatch`. Let's Encrypt without a host name answers `invalid_acme_hosts`, and without the terms agreed to `acme_terms_required`.
- `GET /admin/api/health/problems` lists `https_port`, with `port`, `https_files`, with `failure` (`missing`, `unreadable` or `mismatch`) and `file`, `https_renewal`, with `error`, and `https_expiry`, with `expires`.

### Identity provider settings in the admin API

- `/admin/api/settings` holds `providerIssuer`, `providerName`, `providerClientId`, `providerClientSecretSet` (the secret itself is only sent, never answered, empty to remove it), `providerUserClaim`, `providerCreateAccounts`, `providerAdminClaim`, `providerAdminValue`, and `providerRedirectUri`, read-only, empty without a public address.
- Settings refused answer 400 `invalid_provider`.

### Metrics settings in the admin API

- `/admin/api/settings` holds the setting as `metrics`, and `metricsTokenSet` tells whether the token was made; no answer holds the token.
- `POST /admin/api/settings/secrets/metricsToken/reveal` answers it, and `POST /admin/api/settings/metrics-token` makes a new one and answers the settings.

### Known proxies in the admin API

- In `/admin/api/settings`, `knownProxies` lists the addresses and networks first, then the names. A `PUT` with an entry that is no address, network or host name, or with more than 64 entries, answers 400 `invalid_known_proxies`.
- `GET /admin/api/health` lists the names under `proxyNames`, each with its `addresses`, sorted, and `resolvedAt`.
- `GET /admin/api/health/problems` warns of a name that resolves to nothing as `proxy_name_unresolved`, with `name`.

### Statistics and history routes

The admin API serves the statistics, the history and the CSV, everyone's to administrators and one's own to every user:

| Everyone's (administrators) | One's own |
|---|---|
| `GET /admin/api/statistics` | `GET /admin/api/account/statistics` |
| `GET /admin/api/history` | `GET /admin/api/account/history` |
| `GET /admin/api/history/export` | `GET /admin/api/account/history/export` |

- Every route takes `period`: `7d`, `30d` (the default), `year` or `all`. The administrators' routes also take `user`, a user's ID, to keep that user's playbacks; the own routes ignore it.
- The statistics take `timeZone`, an IANA name such as `Europe/Paris`, UTC by default, which days, months and hours of the week are counted in. They answer `period`, `since` (null for all), `unit` (`day` or `month`), `plays`, `played`, `sent` (the bytes sent to the apps), and:
  - `users`: `id`, `name`, `plays`, `played`;
  - `buckets`: each day or month with playbacks, `start` as `YYYY-MM-DD`, `played`, and `users` (`id`, `played`);
  - `movies`, `series`, `channels`: `id`, the latest `name`, `plays`, `users` (how many played it), `played`;
  - `apps` (`name`, `shared`), `devices` (`name`, `app`, `shared`) and `methods` (`method`: `direct_play`, `direct_stream`, `conversion`, or empty when no report told), each with `plays` and `played`. `shared` is true for the app and device of guests' playbacks through share links, which the admin app names "Shared link" and "Web browser" in its language;
  - `hours`: 168 numbers, the seconds watched in each hour of the week from Monday 0:00;
  - `historyEnabled` and `historyDays`, the settings.

  Times are in seconds, pauses left out.
- The history takes `limit` (1 to 100, 20 by default) and `start`, and answers `items`, the latest first, and `total`. Each item has `id`, `user` (`id`, `name`), `item` (`id`, `kind`: `movie`, `episode`, `channel`, `recording` or `replay`, `name`, and `seriesName`, `season`, `episode`, `channelName`, null when they do not apply), `app`, `device`, `startedAt`, `endedAt`, `played`, `position`, `method`, `shared`, and `sent`, the bytes sent to the app, 0 when none were counted.
- The export answers `text/csv` with a header line: `started_at`, `ended_at` (RFC 3339, UTC), `user`, `kind`, `title`, `series`, `season`, `episode`, `channel`, `app`, `device`, `played_seconds`, `position_seconds`, `method`, `shared`, `sent_bytes` (empty when none were counted). A name that a spreadsheet would read as a formula starts with an apostrophe.
- Errors: `invalid_period`, `invalid_user`, `invalid_time_zone`, `invalid_limit` (400). A member asking for everyone's gets 403.
- `GET /admin/api/settings` answers `playbackHistory` and `playbackHistoryDays`, which `PUT` takes; a number of days out of bounds is `invalid_playback_history_days`.

### DLNA routes in the admin API

- Settings: `/admin/api/settings` holds `dlna`, `dlnaName`, `dlnaNewDevicesUser` (a user's ID, or `null` for no one) and `dlnaMarkPlayed`; a `PUT` naming a user that does not exist answers 400 `invalid_dlna_user`, a name longer than 256 bytes 400 `invalid_dlna_name`.
- `GET /admin/api/dlna/devices` lists the devices, the latest seen first: `id`, `name`, `manufacturer`, `model`, `address`, `userAgent`, `clientInfo`, `userId` (or `null`), `userName`, `blocked`, `profile` (empty, `builtin:` and a built-in profile's name, or an added profile's ID), `matchedProfile` (the name of the profile it gets now), `firstSeen` and `lastSeen`.
- `PATCH /admin/api/dlna/devices/{id}` changes `userId`, `blocked` and `profile`, each kept when left out, and answers the device; `DELETE` forgets it.
- `GET /admin/api/dlna/profiles` lists the profiles, built-in ones first: `id`, `name`, `builtin`, and `createdAt` for added ones. `GET /admin/api/dlna/profiles/{id}` answers a profile's XML. `POST /admin/api/dlna/profiles`, with the XML as its body, adds one (201), and `PUT /admin/api/dlna/profiles/{id}` replaces it; XML that is not a profile answers 400 `invalid_dlna_profile`, with why in `message`. `DELETE` removes it, and the devices forced to it get theirs chosen again.
- `GET /admin/api/health/problems` lists the warning of a port Polyfin cannot listen on as `dlna_port`, with `port`.
- For tests, `POLYFIN_DLNA_PORT` changes the UDP port, which devices never use.

### Channels of your titles in the admin API

- The admin app's routes, for administrators: `GET` and `POST /admin/api/live-tv/channels`, `GET`, `PUT` and `DELETE /admin/api/live-tv/channels/{id}`, `GET /admin/api/live-tv/channels/choices` for the collections and smart lists, and `GET /admin/api/live-tv/channels/titles?q=` for movies and series.
- A `PUT` that changes the sources, the blocks or the shuffle, or sets `rebuild`, reads the titles again and starts the schedule again today.

## Addons in the admin API

Administrators add addons to the server's scope, and users to their own, under `/admin/api/scopes/{scope}`. These notes cover the answers about addons and libraries.

### Addons added to a scope for apps

- The answer to `POST /admin/api/scopes/{scope}/addons` tells `enabledLibraries`, how many of the addon's catalogs became libraries, and `libraryLimit`, 20 when none did because the scope had that many libraries already, absent otherwise. An Eclipse addon's `music.rows` counts its catalog rows: with none, "My music" is its library (see [My music](music.md#my-music)).

### Used by in the admin API

- An addon's JSON, from `GET /admin/api/scopes/{scope}/addons` and every answer about an addon, has `usedBy`: `mode`, `everyone` or `listed`, with `groups` and `users`, the identifiers listed, empty for everyone and for a user's own addon. `PATCH /admin/api/scopes/shared/addons/{id}` takes `usedBy`; `POST /admin/api/scopes/shared/addons`, `POST /admin/api/scopes/shared/iptv` and `POST /admin/api/scopes/shared/folders` take it too, everyone when left out. Lists given with `everyone` are not kept. 400 `invalid_used_by` answers an unknown mode or identifier, a group or a user that does not exist, and any `usedBy` for a user's own addon (`scopes/me`). Only administrators reach the server's scope.
- `PUT /admin/api/groups/{id}/sources/{source}` lists the group in the server source's `usedBy`, and `DELETE` on the same path takes it off; the rest of the list stays, so changes made at once all count. Both answer the source, the same again when nothing changed, and only a change goes to the activity log. 409 `used_by_everyone` answers a source used by everyone, left unchanged; 404 `not_found` a group, or a server source, that does not exist. Only administrators may call them.

### Removing an addon in the admin API

- `GET /admin/api/scopes/{scope}/addons/{id}/removal` tells what removing the source loses, with the same rights as its `DELETE`: `homeRows`, the home rows on its catalogs and collections; `libraries`, its enabled libraries; `hiddenLibraries`, the users who have one of its libraries hidden; `recordings`, the timers and series recordings on its channels. 404 `not_found` answers a source that does not exist. What removal deletes is on [Removing a source](addons.md#removing-a-source).

### Library description refresh for apps

- `GET /admin/api/scopes/shared/libraries` tells, for each library, `refreshable` and `refresh`, the last refresh since the server started, null for none: `state` (`queued`, `running`, `done`, `stopped` or `failed`), `titles`, `done`, `failed`, `startedAt` and `endedAt`. `POST /admin/api/scopes/shared/libraries/refresh` with `addonId`, `catalogType` and `catalogId` starts it (`202`, with the refresh; `400` `library_not_refreshable` for a library that does not offer it), and `POST /admin/api/scopes/shared/libraries/refresh/stop` with the same body stops it (`204`; `404` when none is waiting or running). Both are for administrators.

## Local folder and share routes

A local folder or network share is a source of kind `local` in the admin API. These routes add, scan and match its files.

### Local folders in the admin API

The admin app's folder form names paths for the system Polyfin runs on, which it learns from `GET /admin/api/status`: its `system` is `container` in a container, else `linux`, `macos` or `windows`.

A local folder or network share is an addon of kind `local` in the admin API, listed with the other sources (`GET /admin/api/scopes/shared/addons`), its `folder` field describing its path or address, kind (`movies`, `shows`, `music` or `homevideos`), last scan, error, counts and links, `useArtwork`, whether its titles show the artwork beside its files (see [Artwork](naming-and-matching.md#artwork)), `readExtras`, whether its scans read the extras beside its titles, and `extras`, how many its last scan found (see [Extras and theme media](naming-and-matching.md#extras-and-theme-media)). `links` lists the units linked by hand, `[{"unit", "imdbId", "described"}]`, `described` false when no metadata addon of the server described the title as the folder's kind when the unit's files were last matched to it. `unlistedEpisodes` counts the episodes its files hold that their show's metadata addon does not list, as the shows were last described (see [Episodes the metadata addon does not list](episode-numbering.md#episodes-the-metadata-addon-does-not-list)), which `GET …/folders/{id}/unlisted` lists. `episodes` lists the episodes given by hand to the titles of shows' discs: `[{"path", "disc", "discPath", "number", "episode"}]`, `disc` the disc's kind as in the unmatched files (empty while an image is gone), `discPath` its folder or image, `number` the title's and `episode` such as `S01E05` or `S01E05-E06`. `matchedBy` counts its matched files by how they were matched: `{"name", "nfo", "search", "year", "numerals", "yearNumerals", "link"}`, `year`, `numerals` and `yearNumerals` those a looser rule matched (see [Looser matches](naming-and-matching.md#looser-matches)): a title of the year before or after, one named alike once numerals are read, or both. `searchFailed` counts the files left unmatched as their titles' searches failed (see [Failed searches](unmatched-files.md#failed-searches)), `searchFailure` tells the addon that failed most of them and why, `{"addon", "failure"}` (null when none did), and `searchRetryAt` when the folder is scanned again to search them again (null when it is not to be); `failure` is `unreachable`, `timeout`, `private_network`, `rate_limited`, `server_error`, `http_error` or `invalid_response`, or `failed` with an empty `addon` for a failure that is no addon's. `noSearch` counts the files left unmatched with `reason` `no_search`, as no addon of the server describes and searches titles of the folder's kind. `absoluteEpisodes` counts the files named by an absolute number that are versions of the catalog's episodes of their places (see [Absolute episode numbers](episode-numbering.md#absolute-episode-numbers)), and `unmappedEpisodes` the files their show's numbering could not place (see [How show files are numbered](episode-numbering.md#how-show-files-are-numbered)). For a music folder, `files` counts its tracks, `matched` those in an album, `unmatched` those left out, and `albums` and `artists` its albums and their artists; its unmatched files are the tracks left out, with `reason` `no_title` or `no_album`. For a share, `share` is `"smb"` or `"webdav"` (empty for a local folder), `user` its user, and `passwordSet` whether a password is stored; the password itself is never answered. `error` is `missing`, `unreadable`, `not_folder`, or for a share `unreachable` or `refused`; empty after a scan that could read the folder. The administrator routes are listed under [Local folder routes](#local-folder-routes).

### Local folder routes

| Route | What it does |
|---|---|
| `POST /admin/api/scopes/shared/folders` | adds a folder: `{"name", "path", "kind"}`, `kind` being `movies`, `shows`, `music` or `homevideos`, and for a share `"user"` and `"password"`; `path` is a path in the container or a share's address; `"useArtwork": false` adds a movies or shows folder whose artwork its titles do not show, `"readExtras": false` one whose scans read no extras. An S3 bucket also takes `"endpoint"` and `"region"`, its `user` and `password` being its access key and secret key; an SFTP folder `"privateKey"` and `"passphrase"`; an rclone remote `"rcloneRemote"`; those three `"pollMinutes"`, from 0 to 1,440, their kind's default when left out |
| `POST /admin/api/scopes/shared/folders/rclone-remotes` | lists the remotes of an rclone instance, `{"address", "user", "password"}`, `address` such as `http://rclone:5572`; with `"folder"`, a folder's identifier, and no `password`, the one stored for that folder when `address` is that rclone folder's own instance, else none: `{"remotes": […], "error"}`, `error` `unreachable` or `refused` when rclone could not tell them |
| `PATCH /admin/api/scopes/shared/folders/{id}` | changes its `name`, `path`, `user`, `password`, `endpoint`, `region`, `privateKey`, `passphrase`, `rcloneRemote`, `pollMinutes`, `useArtwork`, `readExtras` (applied at the next scan) or `kind`; `"acceptHostKey": true` accepts the host key an SFTP server offers in place of the one saved; a new path or address forgets the files found; a `password`, `privateKey` or `passphrase` left out keeps the stored one, an empty one removes it, but a `path` on another server, port or kind of share, or an S3 `endpoint` on another server, removes those left out; a new `kind` (`movies`, `shows`, `music` or `homevideos`, else `invalid_folder_kind`) keeps the folder's identifier and its library's, stops its scan under way, forgets its files, links and discs' episodes, and scans it again |
| `POST /admin/api/scopes/shared/folders/{id}/scan` | starts a scan (202) |
| `GET /admin/api/scopes/shared/folders/{id}/unmatched` | `{"total", "files": [{"path", "unit", "title", "year", "size", "reason", "search", "disc", "discTitle"}]}`; `search` is, for `reason` `search_failed`, the addon that failed the search and why, `{"addon", "failure"}` as the folder's `searchFailure` gives them, and `null` for other files. `disc` is `"bluray"` for a Blu-ray disc's playlist or stream and `"dvd"` for a DVD, its `unit` the disc folder, or for a title other than a disc's main one the disc folder, `#` and its playlist's or title's number (`Box/DISC02#00002`), `"dvd_image"` or `"bluray_image"` for a disc image, its `unit` the image, or for a title other than its main one the image, `#` and its number (`Box.iso#00002`), and `null` for other files. A DVD's or an image's title other than its main one has as `path` its disc's file with `#` and its number (`Box/VIDEO_TS/VTS_02_1.VOB#2`, `Box.iso#2`), which links take as any path. A title of a show's disc has as `unit` its show's folder, as `reason` `disc_episode` while its show is matched, and `discTitle` `{"discPath", "number", "length", "episode", "suggested"}`: its disc's folder or image, its number, how long it plays in seconds (null when unknown), its episodes and those suggested, such as `S01E05` (null for none); `discTitle` is null for other files. A disc's titles follow one another, in the order of their numbers. A matched show's file whose name gives no episode has as `reason`, rather than `unreadable_name`, why: `no_season` (`E05` or `Episode 5` outside a season's folder), `air_date` or `no_episode_number`; `number` is the absolute number of an `absolute_number` file, null for other files |
| `GET /admin/api/scopes/shared/folders/{id}/looser` | lists the units a looser rule matched (see [Looser matches](naming-and-matching.md#looser-matches)), by unit, at most 1,000: `{"total", "units": [{"unit", "path", "files", "title", "year", "imdbId", "name", "titleYear", "rule"}]}`; `unit` is what an unlink names (a file, a show's folder, a disc's folder), `path` its first file and `files` how many it has, `title` and `year` what its name was read as (`year` null when it gives none), `imdbId`, `name` and `titleYear` the title it was matched to, and `rule` `year` (a title of the year before or after), `numerals` (named alike once numerals are read) or `year_numerals` (both) |
| `GET /admin/api/scopes/shared/folders/{id}/unlisted` | lists the shows whose files hold episodes their metadata addon does not list, as the shows were last described (see [Episodes the metadata addon does not list](episode-numbering.md#episodes-the-metadata-addon-does-not-list)), by name: `{"shows": [{"imdbId", "name", "unit", "described", "catalogFirst", "catalogLast", "seasons": [{"season", "files", "catalog"}]}]}`; `unit` is the first of its files' units, a show's folder or a file. `files` are, as ranges `[first, last]` of numbers that follow one another, the numbers its files hold in that season that the addon does not list, and `catalog` those the addon lists in the season, empty when it has no such season. `catalogFirst` and `catalogLast` are the addon's first and last episodes, `{"season", "episode"}`, specials left out unless it lists only specials, null when it lists none. `described` is false for a show last described by an older Polyfin, whose addon's numbers are not known until an app opens it again: `catalog` is then empty and `catalogFirst` and `catalogLast` null. Seasons whose files hold no such episode are left out. `numbering` and `group` tell how the show's files are numbered, as `/shows` gives them |
| `GET /admin/api/scopes/shared/folders/{id}/shows` | lists the shows a shows folder holds, by name (see [How show files are numbered](episode-numbering.md#how-show-files-are-numbered)): `{"shows": [{"imdbId", "name", "unit", "files", "numbering", "group", "mapped", "unmapped": [{"path", "season", "episode", "lastEpisode", "absolute", "listed": {"season", "episode"}, "reason"}]}]}`; `numbering` is `catalog`, `per_season`, `continuing`, `absolute` or `order`, `group` the TMDB episode group of `order`, empty otherwise, `files` how many files the show has and `mapped` how many of them the numbering mapped to the catalog's episodes. `unmapped` lists the files it could not place, by path: the season and episodes they tell, `absolute` the absolute number a name telling one alone gives (null otherwise), `listed` the episode of their own they are listed as, and `reason` why: `outside_season`, `beyond_catalog`, `not_in_order`, `not_listed`, `no_order` or `no_catalog` |
| `PUT /admin/api/scopes/shared/folders/{id}/shows/{imdbId}/numbering` | says how a show's files are numbered, `{"numbering", "group"}`, `numbering` as `/shows` gives it and `group` the TMDB episode group of `order`, one of those `/groups` lists, and places them at once, what users did with their episodes following them. Answers the folder; `invalid_numbering` (400) for another numbering or an `order` without a group, `unknown_episode_group` (400) for a group the series does not have, `no_tmdb_key` (409) without a TMDB key, `episode_groups_unreachable` (502) when TMDB could not tell the series' groups, 404 for a show the folder does not hold |
| `GET /admin/api/scopes/shared/folders/{id}/shows/{imdbId}/groups` | lists the series' TMDB episode groups a show's files may be numbered in: `{"groups": [{"id", "name", "type", "episodes", "seasons"}], "lookup"}`, as `GET /admin/api/titles/{id}/episode-order` gives them; `lookup` is `no_key` without a TMDB key, `unknown` for a series TMDB does not know or no metadata addon describes, `failed` when TMDB could not be asked, empty otherwise |
| `GET /admin/api/scopes/shared/folders/{id}/titles?query=…&year=…` | searches the metadata addons for titles of the folder's kind: `{"titles": [{"imdbId", "name", "year", "kind", "posterId"}]}`, at most 10, only titles with an IMDb identifier, those of `year` (optional) first; `kind` is `movie` or `show`, and `posterId` the item whose Primary image (`/Items/{posterId}/Images/Primary`) is the poster, null without one. When some addons fail, it answers the titles the others found; when none found any as addons failed, it answers 502, `addon_unreachable` when an addon could not be reached, `title_search_failed` otherwise |
| `PUT /admin/api/scopes/shared/folders/{id}/links` | links `{"path", "imdbId"}`; `path` is a file, a show's folder or a disc's folder |
| `DELETE /admin/api/scopes/shared/folders/{id}/links?path=…` | removes a link; for a unit a looser rule matched, without a link, removes that match for good: the unit is matched again by the exact rule only, through every scan. Answers the folder; 404 for a unit with neither |
| `PUT /admin/api/scopes/shared/folders/{id}/episodes` | gives titles of shows' discs their episodes, `{"episodes": [{"path", "episode"}]}`, at most 1,000: `path` a title's, as the unmatched files give it, `episode` as a file's name gives it (`S01E05`, `s1e5`, `S01E05-E06`, `S01E05E06`), an empty one removing those given. Answers the folder; `invalid_episode` (400) for an episode written otherwise, 404 for a path that is no title of a show's disc |

### Share addresses the admin API refuses

An address that is not `smb://host[:port]/share[/path]`, `http(s)://host/path`, `s3://bucket[/prefix]` (without a port), `sftp://host[:port][/path]` or `rclone://host[:port][/path]`, or that holds a user or password, is refused with `invalid_share_address`; a user longer than 256 characters or a password longer than 1,024 with `invalid_share_user`; an S3 bucket without an `http(s)://` endpoint, or with a region longer than 64 characters or holding spaces, with `invalid_share_endpoint`; a private key that cannot be read, or opened with its passphrase, with `invalid_share_key`; an rclone remote empty, longer than 128 characters or holding `:` or `/` with `invalid_rclone_remote`; `pollMinutes` out of 0 to 1,440 with `invalid_poll_minutes`. A folder's `folder` field gives `endpoint`, `region`, `rcloneRemote`, `privateKeySet` and `passphraseSet`, never the key nor the passphrase, `hostKey` and `offeredHostKey`, the SHA-256 fingerprints of the SFTP host key saved and of another one its server offers, and `pollMinutes`, null for the other folders; `error` may be `host_key_changed`. On **System › Health**, the problem of a folder (`code` `folder`) carries its error as `failure`, and `share` for a share. A folder that cannot be watched is a problem of its own (`code` `folder_unwatched`): for a local folder, its `failure` is `watch_limit` (on Linux, the limit of watches, ENOSPC), `instance_limit` (on Linux, the limit of watchers or of open files, EMFILE), `open_files_limit` (on macOS, the limit of open files, EMFILE) or `failed`, the only one on Windows; for a share, which carries `share`, it is `unreachable`, `refused`, `host_key_changed` or `failed`. A share's password the key cannot decrypt is listed among the stored secrets as `{"folder": name}`.

### Network share watching in the health API

`GET /admin/api/health` lists the enabled shares under `shares`, each with its `id`, `name`, `share` (`smb`, `webdav`, `s3`, `sftp` or `rclone`), `pollMinutes` (null for SMB and WebDAV, 0 for a folder listed only at its scans, which is not watched), and `watch`: `notified`, `polled`, or empty while it is not watched. A polled share's `polled` tells why: `webdav`, `s3`, `sftp` or `rclone`, `unsupported` (the SMB server refuses change notifications), `silent` (it did not tell of a change a listing found), `failed` (its notifications were lost, and are asked for again), or empty while they are asked for. A notified share's `polled` is `partial` when its server missed a change in one of its folders, and its folders are listed every 2 minutes too. `unwatched` is the share's `folder_unwatched` failure, and `watchFolders` whether **Watch folders and shares for changes** is on.

## Jellyfin 12.2 and 12.4 details

Polyfin follows changes Jellyfin made in 12.2 and 12.4 to fields and filters that apps read.

<a id="jellyfin-12-2-details"></a><a id="jellyfin-122-details"></a>

### Jellyfin 12.2 details

- **Codec tags:** each track's `CodecTag` is the tag ffprobe reads, such as `avc1` or `mp4a` in MP4. Matroska's empty tags are left out, and live channels report none. Attached files report theirs as they are, `[0][0][0][0]` in Matroska. Direct play checks a profile's `VideoCodecTag` conditions against the file's tag.
- **Rewatching:** starting a played movie, episode or audiobook again does not change its last played date. The date changes once the position passes the start, or when the title is played to the end. Next Up therefore does not move past an episode that was only just started. `/Shows/NextUp` with `enableRewatching=true` also lists, for a series played again, the played episode after the one played last, as Jellyfin 12.4 does (see [Continue Watching and Next Up](watch-state.md#continue-watching-and-next-up)).
- **Letter filters:** `nameStartsWith`, `nameLessThan` and `nameStartsWithOrGreater` ignore letter case. They narrow people, music, favorites and other personal lists, and playlists. Movie and series libraries come from addons' catalogs, so they are not narrowed.
- **People counts:** `/Persons` counts only the people it can list.
- **Downmix:** music and audiobooks converted to stereo go through the downmix chosen in the server settings, as the audio of converted video does.

<a id="jellyfin-12-4-details"></a><a id="jellyfin-124-details"></a>

### Jellyfin 12.4 details

- **Ratings that are not numbers:** a `CommunityRating` or `CriticRating` that is not a finite number is none, as Jellyfin 12.4 keeps none. An addon's IMDb rating of `NaN` or `Infinity` leaves the title without a rating. Sent to the metadata editor (`POST /Items/{id}`), `"NaN"`, `"Infinity"`, `"-Infinity"` or a number too large for a float is taken as no value: the answer is 204, and the field follows the addon again. Gains (`NormalizationGain`) are always finite numbers.
- **Videos without a frame size:** a video track ffprobe gives no width or height, as a live stream probed early may be, takes its `AspectRatio` from ffprobe's display aspect ratio alone, and has none without one, as in Jellyfin 12.4, whose probe of such a track failed before. Jellyfin reports an unknown display aspect ratio as `0:1`; Polyfin leaves it out, with or without a frame size.
