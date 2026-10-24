# DLNA

Many TVs, receivers, game consoles and media players on a home network have no Jellyfin app, but can browse and play a UPnP/DLNA media server. Polyfin can be one: it shares the libraries of a user with each device, which browses them as folders and plays movies, episodes, songs and channels.

A device cannot sign in. An administrator chooses, for each device, the user whose libraries it shows: it browses and plays as that user, under their visible libraries, parental control, blocked genres, allowed hours and limits (see [Users](users.md)).

## Setup

Sharing is off by default. Turn it on under **Settings › DLNA**:

| Setting | Default | What it does |
|---|---|---|
| **Share libraries over DLNA** | Off | Announces Polyfin to the devices of the local network, and answers them. Applies at once, without a restart. |
| **Name on the network** | The server's name | The name devices list Polyfin under. |
| **Show new devices the libraries of** | **No one** | The user whose libraries a device seen for the first time shows. With **No one**, a new device sees an empty server until an administrator chooses a user for it. |
| **Mark played what DLNA devices play** | Off | Marks a title played once a device read past **Marked played after (%)** of it (see [Limits](#limits)). |

Polyfin then announces itself over SSDP, on UDP port 1900 and the multicast group `239.255.255.250`, as a `MediaServer:1` with `ContentDirectory:1` and `ConnectionManager:1`. Devices find it within a few seconds, or the next time they look for servers.

It answers devices' searches from port 1900, and sends its announcements from a port of its own, as Jellyfin did: on macOS, an interface that holds the announcements, such as a busy network interface or one up without a link, holds every other datagram of the socket they leave from, which would leave searches unanswered.

## Devices and users

**Settings › DLNA** lists the devices seen: by the name and model they announce, their manufacturer and their address. A device is recorded the first time it asks Polyfin for anything, and known by its address from then on. Each one shows:

- **Shows the libraries of**: a user, or **No one**, which gives it an empty server;
- **Blocked**: the device gets nothing, not even the server's description;
- **Profile**: the profile it plays with, **Automatic** and the one chosen from what it sends, or one forced for it (see [Profiles](#profiles)).

Changes apply at once. **Forget** drops a device with the choices made for it: if it asks again, it is a new device, and shows the libraries of the user chosen for new devices.

A device given another user browses again from the root: the addresses of what it played before were signed for the previous user, and no longer play.

### What a device browses

The root lists the user's views as Jellyfin apps see them: their libraries, local folders and music libraries, **Live TV** when they have channels, the Watchlist, then **Continue Watching** and **Next Up**. Libraries list their titles a page at a time, as the device asks for them, so a large catalog is read page by page. Series hold their seasons, and seasons their episodes; music libraries hold albums and playlists, which hold songs.

Posters are sent as JPEG images, 320 pixels wide. Search, on devices that offer it, looks for titles through the catalogs' search, and for songs and albums when the device asks for music.

Everything a device lists follows the user: their visible libraries, blocked genres and parental control, the titles they hid, and their allowed hours, outside of which the device sees an empty server and plays nothing.

## Playing

Each movie or episode offers one file: the version Polyfin would pick for the user, as for a Jellyfin app (see [Choosing a version](playback.md#choosing-a-version)), with the version rules and quality groups. Its address is signed for the user, as apps' are.

- When the device's profile takes the version's container and codecs, the file is sent as it is, and the device may jump through it with byte ranges.
- Otherwise FFmpeg converts it to an MPEG-TS stream the profile takes, copying the video or the audio when the profile takes them. The device jumps through it with `TimeSeekRange.dlna.org`, and the conversion starts again from the time asked.
- Conversions count against **Video conversions at once**, and every playback against the user's **Playbacks at once**. **Maximum quality** and **Can use conversion** apply as for apps: a user who may not have video converted plays only what the device takes as it is, or with its video copied.
- A text subtitle in the user's language, as their subtitle preferences choose it, is offered beside the video to profiles that take subtitle files, through `CaptionInfo.sec`. A picture subtitle the user chose is burned into converted video.
- **Live TV** channels are MPEG-TS streams, relayed as their source sends them.
- Songs are sent as they are when the profile takes their format, and otherwise converted to MP3.

## Profiles

A profile says what a family of devices plays as it is, and what Polyfin converts for it. Polyfin has a generic profile, for devices it does not recognize, and built-in ones for Samsung, LG, Sony Bravia, Panasonic and Philips TVs. A device gets the first profile that matches the headers it sends (`User-Agent`, `X-AV-Client-Info`) or the name, manufacturer and model it announces, added profiles first, else the generic one. Each device's row can force a profile instead.

Profiles use the XML format of Jellyfin's DLNA plugin, so existing profiles can be added under **Settings › DLNA** as they are: **Add a profile**, then paste the XML or open its file. Each profile's **View XML** shows it; an added one can be replaced from there, or removed, and the devices it was forced on get theirs chosen again. Polyfin reads their `Identification`, `MaxStreamingBitrate`, `DirectPlayProfiles`, `TranscodingProfiles` (MPEG-TS for video, MP3 for audio), `ContainerProfiles`, `CodecProfiles`, `ResponseProfiles` (MIME types) and `SubtitleProfiles` (`External` for `CaptionInfo.sec`, `Encode` for burning in), and ignores the rest.

## Docker networking

SSDP uses multicast, which reaches the container only on the host's network or on a network that gives the container its own address on the local network, such as macvlan or Unraid's `br0`, as for [local network discovery](installation.md#let-apps-find-polyfin-on-the-local-network). On a bridge network, publishing `1900:1900/udp` lets devices that search the host's address directly find Polyfin, but most devices search by multicast, which does not reach it.

Only one program on a host's network can use port 1900: next to another DLNA server on the same host, such as a NAS's media server, Polyfin starts anyway, logs it, and **System › Health** says that it cannot listen. Devices that know the description's address may still browse Polyfin.

The description, control and media addresses live under `/dlna/` on Polyfin's HTTP port, and answer 403 to any address outside the local network: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback. Behind a reverse proxy listed in Known proxies, the address of the client the proxy forwards counts. Devices never send `X-Forwarded-For`, `X-Real-IP` or `Forwarded`: a request that carries one of them from an address that is not a known proxy gets 403, as it comes through a reverse proxy missing from Known proxies, which would otherwise show every client on the internet as one device of the local network. A request that names its client in `Forwarded` alone, without `X-Forwarded-For` or `X-Real-IP`, gets 403 even from a known proxy: Polyfin does not read `Forwarded`, so it cannot tell that client. Posters, captions and media are only served for titles the device's user may see, as browsing lists them.

## Limits

- Devices send no position. Resume points are neither set nor offered: playback always starts at the beginning, unless the device jumps.
- A title counts as played once the device has read past **Marked played after (%)** of it: for a file sent as it is, by the share of the file's bytes sent to the device during the playback, wherever they are in the file, each counted once; for a conversion, by the time asked plus the time streamed. Reading the end of a file, as TVs do to find its index, therefore counts for those bytes only. Devices read ahead, so a title can be marked early: this is off unless **Mark played what DLNA devices play** is on. Played marks then reach the user's tracking services as other marks do.
- Playbacks appear in **Sessions** and in the statistics with the app named `DLNA` and the device's name. They end two minutes after the device last asked for anything.
- Polyfin does not control renderers: there is no "Play To".
- A text subtitle a profile does not take as a file is not burned in: FFmpeg's conversions burn in picture subtitles only.
- A channel whose source sends an HLS playlist does not play over DLNA.

**For app developers:**

- Settings: `/admin/api/settings` holds `dlna`, `dlnaName`, `dlnaNewDevicesUser` (a user's ID, or `null` for no one) and `dlnaMarkPlayed`; a `PUT` naming a user that does not exist answers 400 `invalid_dlna_user`, a name longer than 256 bytes 400 `invalid_dlna_name`.
- `GET /admin/api/dlna/devices` lists the devices, the latest seen first: `id`, `name`, `manufacturer`, `model`, `address`, `userAgent`, `clientInfo`, `userId` (or `null`), `userName`, `blocked`, `profile` (empty, `builtin:` and a built-in profile's name, or an added profile's ID), `matchedProfile` (the name of the profile it gets now), `firstSeen` and `lastSeen`.
- `PATCH /admin/api/dlna/devices/{id}` changes `userId`, `blocked` and `profile`, each kept when left out, and answers the device; `DELETE` forgets it.
- `GET /admin/api/dlna/profiles` lists the profiles, built-in ones first: `id`, `name`, `builtin`, and `createdAt` for added ones. `GET /admin/api/dlna/profiles/{id}` answers a profile's XML. `POST /admin/api/dlna/profiles`, with the XML as its body, adds one (201), and `PUT /admin/api/dlna/profiles/{id}` replaces it; XML that is not a profile answers 400 `invalid_dlna_profile`, with why in `message`. `DELETE` removes it, and the devices forced to it get theirs chosen again.
- `GET /admin/api/health/problems` lists the warning of a port Polyfin cannot listen on as `dlna_port`, with `port`.
- For tests, `POLYFIN_DLNA_PORT` changes the UDP port, which devices never use.
