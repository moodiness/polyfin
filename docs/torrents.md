# Torrents

This page is for administrators who let Polyfin play the torrents some addons list. It covers turning torrents on, the routes their traffic takes, how a torrent starts and stops, the limits, and what to check when one does not play.

An addon may list a torrent rather than a link. Such a stream has an `infoHash`, the index of the file to play (`fileIdx`), and its trackers and DHT nodes (`sources`). Polyfin keeps all of a torrent's traffic to one route, and never sends any of it another way. Torrents are off by default.

Playing a torrent means joining its swarm:

- The other peers see the address the route gives.
- Polyfin shares what it downloaded while the upload limit allows.
- You answer for what the server downloads and shares.

## Turning torrents on

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Play torrent streams** | **Settings › Torrents** | Off | Turns torrents on. It cannot be on without a route. |
| **Route** | **Settings › Torrents** | **SOCKS5 proxy**, with no address | **Network interface**, **SOCKS5 proxy** or **WireGuard tunnel** (see [Routes](#routes)). |
| **Download limit (Mbps, 0 = none)** | **Settings › Torrents** | 0 | Bounds the download, from 0 to 10000. |
| **Upload limit (Mbps, 0 = no sharing)** | **Settings › Torrents** | 0 | Bounds the upload, from 0 to 10000. Sharing stops when the playback ends. |
| **Torrents at once (0 = no limit)** | **Settings › Torrents** | 0 | Bounds the torrents running at once for the whole server, from 0 to 100 (see [Torrents at once](#torrents-at-once)). |
| **Trackers added to every torrent** | **Settings › Torrents** | Empty | Announce URLs asked for every torrent, one a line, 30 at most (see [Trackers](#trackers)). Polyfin comes with no list. |
| **Can play torrent streams** | The **Access** section of the user's page under **Users**, next to **Can download** | Off | Offers this user torrent versions (see [Playing torrent streams](user-limits.md#playing-torrent-streams)). |

- Peers favor those that share back, so a start may be slower with the upload limit at 0.
- **Can play torrent streams** is off for every user, administrators included.
- Without it, or with **Play torrent streams** off, a user's versions hold no torrent, from the server's addons or their own.
- A link to the same file that another addon lists after the torrent then takes its place.
- With both on, torrent versions are listed like the addon's other versions, in its order.

[Which streams become versions](versions.md#which-streams-become-versions) gives every rule between an addon's streams and the versions.

## Routes

Every torrent socket takes the route chosen under **Settings › Torrents**: peers, trackers and the DHT alike. The three routes differ in what they carry and what they need:

| | **Network interface** | **SOCKS5 proxy** | **WireGuard tunnel** |
|---|---|---|---|
| Systems | Linux only | Every system | Every system, Polyfin Desktop included |
| Set up outside Polyfin | The interface, seen by Polyfin | The proxy | Nothing |
| Peers over TCP | Yes | Yes, through `CONNECT` | Yes |
| Peers over uTP | Yes | Only with `UDP ASSOCIATE` | Yes |
| HTTP and HTTPS trackers | Yes | Yes, through `CONNECT` | Yes |
| UDP trackers and the DHT | Yes | Only with `UDP ASSOCIATE` | Yes |
| Tracker names looked up by | The name servers of `/etc/resolv.conf`, over the interface | The proxy | The configuration's `DNS` servers, through the tunnel |
| Peers connecting in | No | No | No |

- Polyfin only connects out, on every route.
- On Windows and macOS, as in Polyfin Desktop, **Network interface** is not offered: those systems cannot bind a socket to one.
- Choosing a route clears the other routes' interface, proxy address or WireGuard configuration. **Proxy user** and **Proxy password** are kept.

### Network interface

**Network interface for torrents** names an interface, such as `wg0`. It works on Linux only.

- Every torrent socket, TCP and UDP, is bound to the interface (`SO_BINDTODEVICE`): peers, trackers and the DHT alike.
- Tracker names are looked up over the interface, by the name servers of `/etc/resolv.conf`.
- A name server on the loopback is never asked, as a local resolver there would ask another way. Name such a tracker by its address, or give the container a resolver on the VPN.
- From Linux 5.7, a process needs no capability to bind a socket not yet bound to a device.
- Before Linux 5.7, it needs `CAP_NET_RAW`, which `compose.yaml` drops.

In Docker, the container must see the interface (see [Docker](#docker)).

### SOCKS5 proxy

**SOCKS5 proxy for torrents** is the proxy's address and port, such as `10.0.0.2:1080`.

- **Proxy user** and **Proxy password** stay empty when the proxy asks for none.
- The password is stored encrypted when `POLYFIN_SECRET_KEY` is set (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
- Peers and HTTP and HTTPS trackers go through its `CONNECT`, and the proxy looks tracker names up.
- UDP trackers, the DHT and uTP need its `UDP ASSOCIATE`, which not every SOCKS5 server offers.
- Without `UDP ASSOCIATE`, those stay off, the torrents play over TCP, and **System › Health** says so.

Polyfin reaches the proxy itself the server's usual way. Run the proxy on the host, or inside the VPN's network.

#### A proxy for long plays

A torrent plays through the proxy for as long as the title lasts. A proxy that gives no answer for 60 seconds stops it (see [A route that stops answering during a play](#a-route-that-stops-answering-during-a-play)). So:

- **A host that stays on:** no sleep, and no power saving on its network adapter. A host asleep freezes every connection through it.
- **A wired link:** prefer it to Wi-Fi for that host.
- **Enough connections at once:** each torrent playing keeps up to 30 peers, and dials up to 16 more at once.
  - Each peer takes a connection of its own to the proxy, and so does each tracker asked.
  - A proxy capped lower may leave the next connections unanswered, which looks like a proxy that does not answer.
- **`UDP ASSOCIATE`:** uTP and the DHT need it, as UDP trackers do. Each torrent playing takes one association for uTP, besides those of the DHT and the UDP trackers.
- **Restarts:** a proxy that restarts, on a schedule or as its network changes, closes the connections through it. The plays go on if it answers again within 60 seconds.

### WireGuard tunnel

**WireGuard configuration** takes the `.conf` a VPN provider gives for WireGuard, pasted whole. Polyfin runs the tunnel itself, for torrents alone, and the rest of the server keeps its usual network.

- Nothing is set up on the host: no `NET_ADMIN`, no `/dev/net/tun`, no route or firewall rule.
- It works the same on Linux, in Docker, on Windows and macOS, and in Polyfin Desktop.
- The tunnel is userspace WireGuard (wireguard-go, MIT license), with its own network stack (gVisor's, Apache 2.0 license), written in Go.

What the tunnel carries:

- Every torrent socket is a socket of the tunnel's network stack, so UDP trackers, the DHT and uTP always work, unlike through a proxy.
- Its packets go, encrypted, to the configuration's server alone, over one UDP socket of the host.
- Incoming connections are not supported, even through a port the provider forwards.

How names are looked up:

- Tracker names are looked up through the tunnel by the configuration's `DNS` servers, never by the system's resolver: no DNS leak.
- A configuration without `DNS` is accepted, and then no name is looked up at all.
- Without `DNS`, trackers named by an address, peers and the DHT still work. Trackers named by a host name are skipped, and **Settings › Torrents** says so once the configuration is saved.
- The server's own name, when `Endpoint` names one, is looked up the host's usual way, as any WireGuard client does.
- That lookup tells only that name, which is how the tunnel is reached. It runs again when a handshake fails.

The configuration is checked when it is saved. A refusal names its line, its key and why:

| Key | Must be |
|---|---|
| `PrivateKey`, `PublicKey`, `PresharedKey` | Keys of 32 bytes in base64 |
| `Address` | IP addresses, with one IPv4 address among them |
| `DNS` | IP addresses |
| `MTU` | 576 to 65535; 1420 by default |
| `ListenPort` | A port from 0 to 65535 |
| `Endpoint` | A host and a port |
| `AllowedIPs` | IP prefixes, with `0.0.0.0/0` among them, as the peers of a torrent are anywhere |
| `PersistentKeepalive` | Off, or 0 to 65535 seconds |

- `PrivateKey`, `Address`, `PublicKey`, `Endpoint` and `AllowedIPs` are required.
- One `[Peer]` is supported: a second is refused, as is a key WireGuard does not know.
- The configuration is 8192 bytes at most.
- The keys wg-quick uses to set its host up (`PostUp`, `PreDown`, `Table` and the like) are left aside: Polyfin never runs them.

How it is kept:

- The configuration, its private and preshared keys with it, is stored encrypted when `POLYFIN_SECRET_KEY` is set (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
- It is never shown again: the settings say a configuration is saved, with its server, its addresses and its DNS servers.
- Logs never hold a key.
- Choosing another route removes it.

#### Throughput and processor use

Measured on a Mac with an Apple M1 Max, the server and the source on the same machine, with other work running beside it (`go test -run '^$' -bench WireGuardRoute -benchtime 5x ./internal/torrent`). The processor time is Polyfin's side alone, the server running in a process of its own:

| Download | Throughput | Processor time per MB |
| --- | --- | --- |
| Direct connection | 4,975 MB/s | 0.21 ms |
| WireGuard tunnel | 83 MB/s (about 660 Mbit/s) | 32 ms |

The tunnel encrypts every packet and runs its own TCP in userspace. It costs far more processor time than a kernel connection: about 0.4 of a core for a 100 Mbit/s download. Torrents rarely go that fast, and the download limit bounds them.

## Docker

`compose.yaml` runs Polyfin read-only, as user 65532, with no capabilities. A proxy and a WireGuard tunnel need nothing more. A network interface must be seen by the container:

| Where WireGuard runs | What to do |
|---|---|
| On the host | Run Polyfin with the host's network (`compose.host.yaml`), and name the host's interface. |
| In a container of its own | Give that container `NET_ADMIN`, and have Polyfin's container share its network with `network_mode: service:<name>`. Polyfin's ports are then published on the WireGuard container. |

On Linux 5.7 or later, Polyfin's own container needs no added capability. [Keep torrents to a VPN](installation.md#keep-torrents-to-a-vpn) sums this up for an installation.

## Kill switch

Torrent versions do not start while their route is down, and nothing falls back to another route:

| Route | Down when |
|---|---|
| **Network interface** | It is missing, down or without an address. |
| **SOCKS5 proxy** | It does not answer, or refuses the user and password. |
| **WireGuard tunnel** | Its server does not answer the handshake (as with a wrong key), its name cannot be looked up, or the tunnel cannot start (as when its `ListenPort` is taken). |

- A play passes those versions over, as it does a version that cannot be read.
- They stay listed, and are tried as any other version (see [Which streams become versions](versions.md#which-streams-become-versions)).
- **System › Health** says why under **To look at**.
- A route's failure is not the version's: the version plays again as soon as the route is back, and Health clears.
- A WireGuard tunnel counts as up while its last handshake is less than 30 seconds old.
- Past that, a check makes a new handshake and waits 8 seconds for its answer.

A WireGuard tunnel may stop, as when its configuration changes during a playback or the server shuts down. It first closes its sockets and waits for what they were sending, then closes. The torrents on it stop, and nothing is sent through the tunnel as it closes.

### A route that stops answering during a play

While a torrent plays, its route is checked every 5 seconds.

- A check of a proxy waits 10 seconds for its answer, as each new connection through it does.
- One check runs at a time: the reads, plays and Health pages that ask meanwhile share its answer.
- A torrent does not stop at its route's first failed check.
- It stops at the first check once the route has given no answer, and its peers no byte, for 60 seconds.
- Those 60 seconds count from the later of the first failed check and the last byte received.

Meanwhile:

- **Open connections:** the connections to peers already open go on through the route. A proxy that answers no new connection often still carries those it opened, and the play reads on from them.
- **Nothing at all:** while nothing comes, as when the proxy's host sleeps, the play goes on as long as what the app and the server read ahead lasts.
- **New connections:** to peers and trackers alike, they still go through the route alone, and fail while it does not answer. Nothing is sent another way.
- **Playbacks:** those reading the torrent, and those joining it, read it without waiting for the route. PlaybackInfo, the version's analysis, a new conversion after a seek and the segments answer as usual.
- **Reads:** a read waiting for pieces that do not come waits on, rather than being taken for stalled after 30 seconds without a byte. So does a first play waiting for the version's analysis (see [Deadlines](#deadlines)).
- **Torrents not running:** they do not start until the route answers.
- **Once the route answers:** the peers waiting to be dialed again, as their connections or dials failed meanwhile, are dialed at once. A dial the route did not carry is not counted among the peer's failures.
- **The log:** it says once, at Warn level, `The route for torrents does not answer`. Once the route is back, it says at Info level `The route for torrents answers again`, with how long it did not (`down=16s`).

A torrent stopped this way tells its reads why, and the log says `A torrent stopped: its route gave no answer, and its peers no byte, for the grace`.

- It keeps its pieces for 15 minutes, within the quarter of **Disk space for files being read (GB)** (**Settings › Playback**) that torrent pieces take.
- Once the route is back, the next play reads those pieces rather than downloading them again.
- A torrent whose playbacks ended removes its pieces as it stops.
- Changing the route in the settings, or turning torrents off, stops the torrents playing at once.

Measured on the local test swarm: a real Polyfin plays a 6-minute 480p movie through a SOCKS5 proxy on the same machine. The download limit is 4 Mbit/s, and the app plays HLS with 30 seconds buffered.

| The proxy | What happened |
|---|---|
| Answers no new connection for 30 seconds | 15 MB came through the connection open to the seeder meanwhile, and the app saw no error |
| Frozen for 45 seconds | The play went on from what was read ahead |
| Frozen for 80 seconds | The app ran out of video, and the torrent stopped 60 seconds after the route's first failed check, 75 seconds into the freeze |

To tell the route from the swarm:

| | A play stopped by its route | A swarm that sends too little |
|---|---|---|
| **System › Logs** | `The route for torrents does not answer`, then `A torrent stopped: its route gave no answer, and its peers no byte, for the grace` | No route line |
| The error | Names the proxy, the interface or the WireGuard server | None while the torrent runs: its reads wait for pieces |
| **System › Health** | Tells it for 15 minutes (see [Health](#health)) | A torrent that does not start logs `A torrent did not start in time`, with its peers and trackers (see [When torrents do not start](#when-torrents-do-not-start)) |

## How a torrent starts

1. **The title's page:** it lists the torrent versions. Nothing reads them yet: preparing playback ahead leaves them alone, and a torrent version never played has no analysis.
2. **Play** sends PlaybackInfo. Among the versions it tries (**Versions tried when one does not work**, see [Playback](playback.md)), each torrent version is tried as the play's own, even when it is not the first. The play joins its swarm, through the route, before anything reads it.
3. **The swarm:** the torrent asks its trackers and the DHT for peers, and connects to them. It receives its info dictionary from one of them. The pieces at the start and the end of the file come first.
4. **The analysis:** it runs inside that playback. Its first bytes are waited for, then ffprobe reads it, both as long as the play waits for the torrent (see [Deadlines](#deadlines)). The analysis is kept for later plays, and so are the subtitle files inside the torrent that go with the file.
5. **The answer:** PlaybackInfo answers the version as the app plays it: as it is, repackaged, or converted.
6. **The stream:** the stream or HLS requests the app sends next read the torrent as the same playback.

A torrent version analyzed once plays at once the next time: its analysis and its subtitle files are kept, and only the stream joins the swarm.

Played again within 15 minutes of its stop, a torrent starts from the peers, the info dictionary and the DHT nodes its last run found (see [Only while playing](#only-while-playing)). It then needs one peer connected, and no metadata from the swarm.

### Watching a torrent start

While a torrent starts, the web player writes how it goes under jellyfin-web's spinner, once a second:

| Line | When |
|---|---|
| "Finding peers…", then "Finding peers: 12 found, 3 connected" | The torrent looks for peers and receives its info dictionary |
| "Receiving the start: 2.4 MB from 3 peers" | Its pieces come, while ffprobe reads the file and the player reads its first segments. The bytes count from the torrent's start. |
| "The torrent did not start:" and why | Every torrent the play tried did not start in time |

The reasons the last line gives:

- no peer found, or peers found that sent nothing;
- trackers that did not answer, or no tracker named;
- a route that carries no UDP, or a route that is down;
- as many torrents playing as **Torrents at once** allows.

- With several torrent versions tried at once (see [How a torrent starts](#how-a-torrent-starts)), the line follows the one that got furthest.
- Once PlaybackInfo answers, it follows the version chosen while the spinner still turns, up to 3 minutes.
- It names no peer, tracker or address.
- A play of a link, and a page loading, show no line.
- It shows in the web player, and in the Jellyfin apps that show the server's web player, as Jellyfin's apps for Android and iOS do.
- Android TV, Swiftfin and other apps draw their own spinner, and show nothing more: the Jellyfin API gives a play no progress before it starts.
- Its words follow the web player's language, as the script's other words do (see [Web player](web-client.md)).

**Compared with Jellyfin**

- jellyfin-web's spinner takes no text ([loading.ts](https://github.com/jellyfin/jellyfin-web/blob/v12.4/src/components/loading/loading.ts)), and Jellyfin, which has no torrents, has nothing to tell meanwhile. The line is Polyfin's own.

API details: [Torrent start progress](jellyfin-api.md#torrent-start-progress).

### Deadlines

A link's host sends its headers within seconds. A torrent needs peers first, from trackers that may each take 30 seconds to answer, and from DHT lookups that hop from node to node. Then it needs its info dictionary, then its first pieces. So a torrent's playback waits longer:

| What waits | For how long |
|---|---|
| The torrent's start: peers found, one connected, and the info dictionary received | 60 seconds |
| A piece, at the start as later | 30 seconds without a byte of the torrent, twice in a row |
| The version's first bytes, then ffprobe's read | 105 seconds (the start, 15 seconds and a piece), then on while pieces keep coming, up to 3 minutes from the play |
| PlaybackInfo | 90 seconds (the start and a piece), then on while pieces keep coming, up to 3 minutes from the play |
| The read of the version's index, for its HLS segments | As the analysis, and within 3 minutes in any case |

"While pieces keep coming" means a byte at least every 30 seconds, or a route waited for within its grace (see [A route that stops answering during a play](#a-route-that-stops-answering-during-a-play)).

The start:

- 60 seconds fit two rounds of a slow tracker beside the DHT lookups.
- While the torrent has no peer connected, the DHT is looked up again every 15 seconds, whether the last lookup found peers or not. The peers a lookup finds may all be gone or unreachable, and the next one may find others.
- Once a peer is connected, the DHT is looked up again 5 minutes after the last lookup that found peers.

A piece:

- A peer chooses whom it sends to every 10 seconds, and tries a new peer every 30.
- What counts is a byte of the torrent arriving, not a whole block of the file.
- A piece is read only once it is whole and checked. So a slow swarm, such as a peer sending 16 KB/s, takes longer than 30 seconds for the 1 MiB a read asks, while its blocks keep coming.
- A read is taken as stalled only when no block arrives for 30 seconds, twice in a row.

The analysis waits for the torrent rather than within **Maximum time to analyze a version**.

PlaybackInfo waits for a torrent version's start and a piece, not for its whole analysis:

- A slow swarm that keeps sending plays at the first play, once the version's analysis and the read of its index are in.
- A torrent version still being read once its pieces stop coming, or at 3 minutes, is passed over.
- The answer is then `RateLimitExceeded` when no other version plays (see [When torrents do not start](#when-torrents-do-not-start)).
- Its analysis and the read of its index go on as long as they wait for the torrent, and are kept: the next play of the title uses them at once.
- The play itself waits no longer for them, and leaves nothing behind waiting.
- A swarm that sends nothing never holds the app's request past 90 seconds.
- A slow swarm, or a route that stops answering after the torrent started, never holds it past 3 minutes.
- jellyfin-web sets no limit on that request, nor do Traefik and Caddy by default. nginx closes a response silent for 60 seconds unless told otherwise (see [Reverse proxy](reverse-proxy.md)).

The read of the index ends within 3 minutes, read whole or failed. What it reads waits at most a piece's wait at a time, twice, so it ends well before. A read that would still wait then is tried again at the next play.

A read that gave up on a torrent's pieces, as they stopped coming or came too slowly for the play, is a torrent that did not start in time:

- This holds for the analysis and the read of the index.
- The play answers `RateLimitExceeded` when no other version plays, never that the version suits no app.
- The version is not kept as failed: the next play tries it again at once.
- The same holds for the other reads of its file, which otherwise remember a failure for a while. They locate its subtitle tracks through its index, read one of them whole, read the first frame of its HDR video, or read a picture subtitle track into text.
- Such a read is made again at the next play when it gave up on the torrent's pieces, or ran while no playback played the torrent. That happens when the title's page opened, or after a pause past the torrent's 30 seconds.
- A torrent that never started, and what the file itself holds, as a track the index does not list, are remembered as for any version.

A play may join an analysis or a read of the index that an earlier play began, as when an app asks again or another device plays the title:

- It waits for the torrent as long as it would alone.
- When that read gives up as the earlier play's wait runs out, the file is read again for the later play.
- The later play is told to try later only once its own wait is over.

After a deadline:

- A torrent that does not start in time is not tried again for 15 minutes by a play that picks no version, and the next versions are tried meanwhile.
- It stays listed in its place among the title's versions, so that the version menu keeps the addon's order (see [When torrents do not start](#when-torrents-do-not-start)).
- A torrent that started but whose peers then send nothing, as when they hold its pieces, did not start in time too.
- The play then answers `RateLimitExceeded` when no other version plays. That torrent is not kept out: the next play tries it again at once, and it plays as soon as the peers send.

## Only while playing

Only a playback starts a torrent, and a subtitle file inside it that a user picks before it was fetched (see [Subtitle files inside the torrent](#subtitle-files-inside-the-torrent)).

- Preparing playback in advance, thumbnails and downloads never start one.
- A torrent version cannot be downloaded, and has no thumbnails.
- The playback ends after 30 seconds without a read, as when a player pauses, or between its requests.
- Then the torrent stops: no more sharing, no more peers, and its pieces are removed. The trackers are told it stopped.

What a torrent learned is kept in memory for 15 minutes once it stopped: the peers it connected to (50 at most), its info dictionary, and the DHT nodes nearest it. Playing it again within that time, as after closing the player and refreshing the page, starts from them:

- The peers that just worked are dialed first.
- No peer needs to send the info dictionary again.
- The DHT lookups start near the torrent.
- Its pieces are still removed at the stop.
- A torrent that connected to no peer keeps what an earlier run of it learned, unchanged.
- At most 64 torrents, and 64 MiB of info dictionaries, are kept, those kept longest dropped first.

A playback of another title ends the 30 seconds' wait at once. When a device reports that it plays another title, a torrent's or not, the torrents it played before stop then. So do the torrents its user played on another device that stopped since. Exceptions:

- A torrent runs on while another device, a DLNA device, a guest or a converted download still plays it, or another user stopped it less than a minute ago.
- A pause, a seek, another audio track, a replay of the same title, and another episode of the same torrent, as in a season pack, keep it.
- A stop alone ends nothing at once: jellyfin-web may send the stop of a playback late, once the next playback of the same title started.
- A late report of the title the device left ends nothing and keeps nothing, such as a progress of its play session that arrives after the other title started. The device plays the title of its latest playback.
- The repackaging and conversions the device made of the title it left stop as it reports the other title, even when its app sent no stop (see [Transcoding](transcoding.md#hls-segments)). The torrent stops with them.
- A torrent still read otherwise as the other title starts stops as soon as that read ends.

Once no playback reads a torrent's file, Polyfin stops reading ahead in it and closes its reads at once. It no longer finishes them and keeps them open for a next read. What the peers were asked for still comes. Measured on the local test swarm, with a peer sending about 160 KB/s:

| Case | Before | After |
|---|---|---|
| A stop | 3.3 MB more over 20 seconds | 0.6 MB over 4 seconds |
| Switching to another title, both torrents running 30 seconds | 3.2 MB over 19 seconds | 1.2 MB over 6 seconds, the first torrent stopping as the second title's playback was reported |

**Sync addon subtitles** (see [Subtitles](subtitles.md#addon-subtitles-synced-to-the-version)) looks for no speech in a torrent version:

- Looking would download three stretches of it from the swarm, up to 300 MB, as its playback starts, beside it.
- What the playback holds then, its start and its index, has none of them.
- A torrent version's addon subtitles come with the times the addons give.

The subtitle files an addon lists with a torrent stream are that version's own, listed from the title's page on, before any play. Polyfin downloads them from their links, without joining the swarm (see [Subtitles](subtitles.md#subtitle-files-listed-with-a-stream)).

### Subtitle files inside the torrent

The text subtitle files of the torrent that go with the video are listed for its version from its first play, once the torrent tells its files. Examples are `Movie.fr.srt` beside it, or `Subs/English.srt`. [Subtitle files inside torrents](subtitles.md#subtitle-files-inside-torrents) tells which files go with the video, and how apps get them. Their text comes from the swarm:

- **Their cost:** a file costs the pieces that hold it, whole, whatever its size. That is one piece, or two when it lies across them, of up to 16 MiB each in the torrents of movies and episodes.
- **Only those pieces:** none past the file, nor the start and end of a file that a video's read fetches for its index.
- **Shared pieces:** files that share a piece, as small files next to each other often do, cost it once while the torrent runs.
- **At the start:** so only the file in the user's preferred subtitle language is fetched as the play starts, beside the video's first pieces.
- **The others:** they are fetched when picked, and kept once fetched.
- **The route:** a fetch joins the torrent as a playback does, through the route and under the kill switch.
- **After the stop:** a file picked once the torrent stopped starts it again, for that fetch alone. The torrent then stops 30 seconds later, as after a playback.
- **How long:** the fetch runs to its end, for up to 105 seconds (the torrent's start, 15 seconds and a piece's wait), even when the app stops waiting.

Measured on a Mac with an Apple M1 Max, on the local test swarm (`go test -run '^$' -bench SubtitleFetch -benchtime 3x ./internal/torrent`). One peer sends a block of 16 KiB every 10 ms (about 1.6 MB/s), and Polyfin fetches a SubRip file of 60 KB once the torrent started, 3 runs each:

| Piece length | Downloaded | Time |
| --- | --- | --- |
| 1 MiB | 1 MiB | 0.73 s |
| 4 MiB | 4 MiB | 2.96 s |
| 16 MiB | 16 MiB | 11.92 s |

The time is the piece's at the swarm's speed. The playback shares that speed while the file comes.

## Torrents at once

On a slow link, or a route that allows few connections, torrents running at once share the bandwidth and the peers. Each then starts more slowly. **Torrents at once (0 = no limit)**, under **Settings › Torrents**, bounds them for the whole server.

There is no limit per user: **Playbacks at once**, on each user's page, counts their devices playing, not their torrents.

What counts:

- **Each torrent, not each playback:** every torrent running counts once, however many devices or users play it. A title played on two devices, or two episodes of one season pack, take one place.
- **Its linger too:** a torrent counts until it stops. That is 30 seconds after the last read of its playback, as when a player pauses, or at once when the same device or user plays another title (see [Only while playing](#only-while-playing)).
- **Whatever starts it:** a playback through a Jellyfin app, a DLNA device or a share link starts the torrent, and it counts.
- **Subtitle fetches:** so does the fetch of a subtitle file inside a torrent not fetched yet (see [Subtitle files inside the torrent](#subtitle-files-inside-the-torrent)).
- **Not converted downloads:** a torrent version cannot be downloaded.
- **Not an analysis without a play:** a torrent version is analyzed only inside its play (see [How a torrent starts](#how-a-torrent-starts)).

At the cap:

- **A torrent not running:** it does not start. A play passes over its torrent version, as one that did not start in time. It tries the next versions within **Versions tried when one does not work**, and a link plays.
- **With no other version:** PlaybackInfo answers `RateLimitExceeded`, and jellyfin-web says the media cannot be played at this time (see [When torrents do not start](#when-torrents-do-not-start)).
- **Not a failure:** the torrent version is not kept as failed, and the next play tries it again.
- **DLNA devices and guests:** through share links, they count the same, and a torrent not running does not start for them either.
- **A torrent already running:** it is joined as usual, and another device playing the same title takes no more place.
- **Two torrents starting at the same moment:** for the last place, the first to start takes it, and the other is refused as at the cap.
- **The log:** it tells it at Info level: `A torrent version is passed over: as many torrents run as Torrents at once allows`.

A device that left a torrent for another title gives its place to that title, as that title's start would end the torrent at once:

- When its next PlaybackInfo finds the cap reached, the torrents the device or its user left are ended first, as the start would end them.
- The new torrent then starts in their place.
- A torrent ended stops once its last read ends. An app's player that just stopped, as jellyfin-web's, may still read the file for a moment.
- So PlaybackInfo waits up to 5 seconds for it to stop, before it tells the title cannot be played now.
- The cap is never passed meanwhile.
- Another user's torrent is never ended for it, nor one another device still plays.
- That holds whether the other device's app names the version it plays or only the title, as jellyfin-web does when playing a title as it is. The version is then the one that device's PlaybackInfo chose.

**Can play torrent streams**, **Playbacks at once**, the version rules and the quality groups apply as before. While the cap is reached, it only removes torrent versions from those a play tries.

## When torrents do not start

A torrent may not start because its route is down or no peer sends it in time. When no version of the title plays then, PlaybackInfo answers Jellyfin's `RateLimitExceeded`, not `NoCompatibleStream`. jellyfin-web then says the media cannot be played at this time, rather than that the app is not compatible with it.

The log tells why in one line at Info level, `A torrent did not start in time`, and **System › Health** tells it too. The line holds:

- the peers found;
- how the trackers answered, or `no tracker: the addon named none` when the addon named none and the settings add none;
- the trackers of the settings counted apart, such as `3 trackers (2 of the settings), 1 answered`;
- the peers not connected, counted by how their last dial or connection ended, such as `23 peers found, 0 connected (15 timed out, 8 refused)`.

| Count | Meaning |
|---|---|
| timed out | No answer within 20 seconds, over TCP and over uTP, or a proxy that tells the connection timed out |
| refused | The peer refused the connection, directly or as the SOCKS5 proxy tells (its reply 5) |
| unreachable | No route to the peer, directly or as the proxy tells |
| refused by the proxy | Another refusal of the SOCKS5 proxy, by its rules or for its own failure |
| closed the connection | The peer ended the connection before its handshake, as a peer does with a torrent it does not have |
| left | The peer was connected, and its connection ended |
| failed otherwise | Another failure |
| not dialed yet | The peer waits for a dial, or its first dial runs |

A peer tried over both uTP and TCP is counted by TCP's answer.

The torrent versions that did not start in time stay in the title's versions, in the addon's order. Item details and the version menus list them:

- **Plays that pick none:** for 15 minutes, a play that picks no version passes them over. It tries the next ones, up to **Versions tried when one does not work**. jellyfin-web's Play, Android TV and Swiftfin send the title's own identifier, which picks none. When none of the others plays either, the play answers `RateLimitExceeded`.
- **A version picked:** a version the user picks in the version menu, under its own identifier, is tried again at once, its torrent started anew. The first version carries the title's own identifier, so picking it is a play that picks none.
- **Tried again by every play:** after 15 minutes, after a restart, or once a version picked starts.

**Compared with Jellyfin**

- Jellyfin lists every version of a title, also those that cannot be read (see [Playback](playback.md#versions-and-analysis)). Polyfin lists these torrents too, and only its own choice of a version for a play passes them over for a while. Other failures leave the versions out for 15 minutes.

What to check:

- **System › Health**, under **To look at:** a route that cannot carry torrents, a proxy without `UDP ASSOCIATE`, or a torrent that did not start, and why.
- **The route reaches the internet:** an interface that is up but routes nowhere, or a proxy that cannot reach out, finds no peer.
- **UDP:** through a proxy, it needs `UDP ASSOCIATE`, and the proxy must relay the datagrams back. Without UDP, only HTTP trackers find peers, many torrents list UDP trackers alone, and the peers that take uTP alone are not reached.
- **The trackers' answers:** turn on **Detailed log (to diagnose a problem)** under **Settings › Diagnostics**, and play again. Then read `A tracker could not be reached` in **System › Logs**, with each tracker's own reason for a refusal.
- **The swarm itself:** a torrent with no seeder does not start anywhere. Try another version.
- **No tracker:** a torrent whose addon names no tracker is found through the DHT alone (see [Trackers](#trackers)). Trackers you trust, under **Trackers added to every torrent**, find it more peers.

## Trackers

A torrent's peers come from the trackers its addon names in the stream's `sources`, from the DHT, and from the peers themselves (peer exchange).

Some addons list a torrent without any tracker: the stream then has an info hash and no `sources`. The DHT alone then finds its peers, where many listed are gone, or behind a router that accepts no connection. The log says so when such a torrent does not start: `no tracker: the addon named none`.

**Trackers added to every torrent**, under **Settings › Torrents**, lists announce URLs, one a line, such as `udp://tracker.example:1337/announce`.

- Each is HTTP, HTTPS, or UDP with a port, of 500 characters at most. The list holds 30 trackers at most.
- Polyfin asks them for the peers of every torrent, besides those the addon names, through the route, as the others.
- It is empty by default: Polyfin comes with no list, and you choose the trackers you trust.
- A torrent whose addon names no tracker is still looked up on the DHT: the trackers of the settings do not replace it.
- A private torrent asks its own trackers alone (BEP 27). Once its info dictionary tells it is private, the trackers of the settings are told it stopped, and asked no more.
- A change applies to the torrents that start next, and to those running as another playback joins them.
- The log line of a torrent that did not start counts those trackers apart (see [When torrents do not start](#when-torrents-do-not-start)).

## Reading and storage

A torrent version is read through the source cache, as a link is. FFmpeg, ffprobe and apps read it the same way. It is always relayed, never sent to an app.

### The file played

The file played is `fileIdx`. When the addon gives none, as some do for a season's pack:

1. It is the file whose name is the stream's `behaviorHints.filename`.
2. Else, for an episode, it is the largest video whose name tells that episode (see [Naming files](naming-and-matching.md#naming-files)).
3. Else it is the largest file.

An episode's name may tell it as:

- `S01E02`, `s1e2`, `S01E01-E02` or `1x02`;
- `E02` and `Episode 2` in a season's folder: `Season 01`, `S01`, `Specials`, or the pack's own folder, as `Show.S01.1080p`;
- a year or a resolution glued to the episode, as in `S01E01-2022` or `S09E14-1080p`, ending no range.

The version stays the same version whichever file it finds: its identity, its analysis and the version rules do not depend on it.

[Torrent peers and pieces](torrent-peers.md) tells how a torrent's trackers, DHT lookups, pieces, peers and uTP work.

## Health

**System › Health** shows a **Torrents** panel while torrents are on:

- the route, and a WireGuard tunnel's addresses and last handshake;
- the torrents playing, against **Torrents at once** when it is set, such as `1 of 2`;
- their peers, and how many of them were reached over uTP;
- what was shared since the start;
- the outages of the route the torrents playing went through in the last 15 minutes, with when and for how long (see [A route that stops answering during a play](#a-route-that-stops-answering-during-a-play)).

The **Source cache** tells the part torrent pieces take.

Under **To look at**:

| Problem | Kind |
|---|---|
| A route that cannot carry the torrents, and why, such as a WireGuard server that does not answer the handshake | Error |
| A proxy without `UDP ASSOCIATE` | Warning |
| A torrent that did not start in time in the last 15 minutes, and why, with its peers counted by how their last dial ended (see [When torrents do not start](#when-torrents-do-not-start)), until a torrent starts | Warning |
| A torrent that stopped while it played in the last 15 minutes, as its route gave no answer, and its peers no byte, for 60 seconds | Warning, which the route's own error replaces while the route still does not answer |

- Health waits 2 seconds for a route's check.
- A proxy's check runs in the background for its 10 seconds, and a WireGuard tunnel's gives the handshake its 8.
- Past Health's 2 seconds, a proxy or a server that did not answer is told as not answering. Health then reads that check's answer rather than waiting again.
- While Health is open, a tunnel found down is checked again 5 seconds after each check. So one that comes back stops being reported within 13 seconds.
- That takes a few seconds more when `Endpoint` names the server by a host name, which is then looked up again.

[Health](health.md) describes every problem it lists.

<div class="moved-sections">

<a id="utp"></a>*uTP* moved to [Torrent peers and pieces](torrent-peers.md#utp).

</div>
