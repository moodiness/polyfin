# DLNA

This page is for administrators whose TVs, receivers, game consoles or media players have no Jellyfin app. Many such devices browse and play a UPnP/DLNA media server, and Polyfin can be one. Each device browses a user's libraries as folders, and plays movies, episodes, songs and channels.

A device cannot sign in. An administrator chooses, for each device, the user whose libraries it shows. The device browses and plays as that user, under their visible libraries, parental control, blocked genres, allowed hours and limits (see [Users](users.md)).

## Setup

Turn sharing on under **Settings › DLNA**:

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Share libraries over DLNA** | **Settings › DLNA** | Off | Announces Polyfin to the devices of the local network, and answers them. Applies at once, without a restart. |
| **Name on the network** | **Settings › DLNA** | The server's name | The name devices list Polyfin under. |
| **Show new devices the libraries of** | **Settings › DLNA** | **No one** | The user whose libraries a device seen for the first time shows. With **No one**, a new device sees an empty server until an administrator chooses a user for it. |
| **Mark played what DLNA devices play** | **Settings › DLNA** | Off | Marks a title played once a device read past **Marked played after (%)** of it, a setting under **Settings › Content › Playback thresholds** (see [Limits](#limits)). |

Polyfin then announces itself over SSDP:

- on UDP port 1900 and the multicast group `239.255.255.250`;
- as a `MediaServer:1`, with `ContentDirectory:1` and `ConnectionManager:1`.

Devices find it within a few seconds, or the next time they look for servers.

Polyfin answers devices' searches from port 1900. It sends its announcements from a port of its own, as Jellyfin did.

- On macOS, an interface that holds announcements, such as a busy one or one up without a link, holds every other datagram of their socket.
- From port 1900, that would leave searches unanswered.

## Devices and users

**Settings › DLNA** lists the devices seen, the latest seen first. Each shows the name and model it announces, its manufacturer, its address, and when it was last seen.

- A device is recorded the first time it asks Polyfin for anything.
- From then on, Polyfin knows it by its address.

Each device has three choices:

| Choice | What it does |
|---|---|
| **Shows the libraries of** | A user, or **No one**, which gives the device an empty server. |
| **Blocked** | The device gets nothing, not even the server's description. |
| **Profile** | The profile it plays with: **Automatic**, with the one chosen from what it sends, or one forced for it (see [Profiles](#profiles)). |

Changes apply at once.

- **Forget** drops a device with the choices made for it. If it asks again, it is a new device, and shows the libraries of the user chosen for new devices.
- A device given another user browses again from the root. The addresses of what it played before were signed for the previous user, and no longer play.

### What a device browses

The root lists the user's views as Jellyfin apps see them, in this order:

1. their libraries, local folders and music libraries;
2. **Live TV**, when they have channels;
3. the Watchlist;
4. **Continue Watching** and **Next Up**.

Inside them:

- Libraries list their titles a page at a time, as the device asks for them. So a large catalog is read page by page.
- Series hold their seasons, and seasons their episodes.
- Music libraries hold albums and playlists, which hold songs.
- Posters are sent as JPEG images, 320 pixels wide.
- Search, on devices that offer it, finds titles through the catalogs' search. It finds songs and albums when the device asks for music.

Everything a device lists follows the user:

- their visible libraries, blocked genres and parental control;
- the titles they hid;
- their allowed hours: outside them, the device sees an empty server and plays nothing.

## Playing

Each movie or episode offers one file: the version Polyfin would pick for the user, as for a Jellyfin app (see [Choosing a version](versions.md#choosing-a-version)).

- The version rules and quality groups apply.
- Its address is signed for the user, as apps' addresses are.

What the device gets depends on its profile:

| The profile | Polyfin sends | The device jumps through it with |
|---|---|---|
| Takes the version's container and codecs | The file as it is | Byte ranges |
| Does not | An MPEG-TS stream the profile takes, converted by FFmpeg, which copies the video or the audio when the profile takes them | `TimeSeekRange.dlna.org`: the conversion starts again from the time asked |

The user's limits apply as for apps (see [Playback and access limits](user-limits.md#playback-and-access-limits)):

- Conversions count against **Video conversions at once (0 = no limit)**, under **Settings › Conversion (transcoding) › General**.
- Every playback counts against the user's **Playbacks at once (0 = no limit)**.
- **Maximum quality** and **Can use conversion (transcoding)** apply.
- A user who may not have video converted plays only what the device takes as it is, or with its video copied.

Subtitles:

- A text subtitle in the user's language, as their subtitle preferences choose it, is offered beside the video through `CaptionInfo.sec`. Only profiles that take subtitle files get it.
- A picture subtitle the user chose is burned into converted video.

Channels and songs:

- **Live TV** channels are MPEG-TS streams, relayed as their source sends them.
- Songs are sent as they are when the profile takes their format. Otherwise they are converted to MP3.

## Profiles

A profile says what a family of devices plays as it is, and what Polyfin converts for it. Polyfin has a generic profile, for devices it does not recognize, and built-in ones for Samsung, LG, Sony Bravia, Panasonic and Philips TVs.

A device gets the first profile that matches, added profiles first, else the generic one. Profiles match on:

- the headers the device sends (`User-Agent`, `X-AV-Client-Info`);
- or the name, manufacturer and model it announces.

Each device's row can force a profile instead.

Profiles use the XML format of Jellyfin's DLNA plugin, so existing profiles can be added as they are:

1. Under **Settings › DLNA**, choose **Add a profile**.
2. Paste the XML, or open its file.

Each profile's **View XML** shows it.

- An added profile can be replaced from there, or removed.
- The devices a removed profile was forced on get theirs chosen again, from what they send.

Polyfin reads these parts of a profile, and ignores the rest:

- `Identification`;
- `MaxStreamingBitrate` and `MaxStaticBitrate`;
- `DirectPlayProfiles`;
- `TranscodingProfiles`: MPEG-TS for video, MP3 for audio;
- `ContainerProfiles` and `CodecProfiles`;
- `ResponseProfiles`: MIME types;
- `SubtitleProfiles`: `External` for `CaptionInfo.sec`, `Encode` for burning in.

## Docker networking

SSDP uses multicast. Multicast reaches the container only:

- on the host's network;
- or on a network that gives the container its own address on the local network, such as macvlan or Unraid's `br0`.

This is the same as for [local network discovery](installation.md#let-apps-find-polyfin-on-the-local-network).

On a bridge network, publishing `1900:1900/udp` lets devices that search the host's address directly find Polyfin. Most devices search by multicast, which does not reach it.

Only one program on a host's network can use port 1900. Next to another DLNA server on the same host, such as a NAS's media server:

- Polyfin starts anyway, and logs it;
- **System › Health** says that it cannot listen;
- saving the settings tries again;
- devices that know the description's address may still browse Polyfin.

### Local network only

The description, control and media addresses live under `/dlna/` on Polyfin's HTTP port.

- While sharing is off, they answer 404.
- They answer 403 to any address outside the local network. The local network is private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback.
- Behind a reverse proxy listed in [Known proxies](reverse-proxy.md#known-proxies), the address of the client the proxy forwards counts.
- Posters, captions and media are served only for titles the device's user may see, as browsing lists them.

Devices never send `X-Forwarded-For`, `X-Real-IP` or `Forwarded`.

- A request carrying one of them from an address that is not a known proxy gets 403. It comes through a reverse proxy missing from Known proxies, which would otherwise show every client on the internet as one device of the local network.
- A request that names its client in `Forwarded` alone, without `X-Forwarded-For` or `X-Real-IP`, gets 403 even from a known proxy. Polyfin does not read `Forwarded`, so it cannot tell that client.

## Limits

- **Resume points:** devices send no position. Resume points are neither set nor offered, so playback always starts at the beginning, unless the device jumps.
- **Played marks:** a title counts as played once the device has read past **Marked played after (%)** of it. Marking is off unless **Mark played what DLNA devices play** is on, as devices read ahead and a title can be marked early.

  | Played as | What counts |
  |---|---|
  | A file sent as it is | The share of the file's bytes sent to the device during the playback, wherever they are in the file, each counted once |
  | A conversion | The time asked, plus the time streamed |

  - Reading the end of a file, as TVs do to find its index, therefore counts for those bytes only.
  - Played marks then reach the user's tracking services, as other marks do.
- **Now playing:** playbacks appear under **Now playing** on the admin app's Home, and in the statistics. The app is named `DLNA`, with the device's name.
- **End of a playback:** it ends two minutes after the device last asked for anything.
- **No "Play To":** Polyfin does not control renderers.
- **Text subtitles:** one a profile does not take as a file is not burned in. FFmpeg's conversions burn in picture subtitles only.
- **HLS channels:** a channel whose source sends an HLS playlist does not play over DLNA.

API details: [DLNA routes in the admin API](jellyfin-api.md#dlna-routes-in-the-admin-api).
