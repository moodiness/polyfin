# Torrents

Some addons list torrents rather than links: a stream with an `infoHash`, the index of the file to play (`fileIdx`), and its trackers and DHT nodes (`sources`). Polyfin can play them, with all of their traffic kept to one route: a network interface such as a VPN's, a SOCKS5 proxy, or a WireGuard tunnel Polyfin runs itself. Nothing is ever sent another way. It is off by default.

Playing a torrent means joining its swarm: the other peers see the address the route gives, and Polyfin shares what it downloaded while the upload limit allows. You answer for what the server downloads and shares.

## Turning torrents on

Under **Settings › Torrents**:

- **Play torrent streams** turns torrents on. It cannot be on without a route.
- **Route** chooses between **Network interface**, **SOCKS5 proxy** and **WireGuard tunnel**. On Windows and macOS, as on Polyfin Desktop, the network interface is not offered: those systems cannot bind a socket to one.
- **Download limit (Mbps, 0 = none)**, 0 by default.
- **Upload limit (Mbps, 0 = no sharing)**, 0 by default. Peers favour those that share back, so a start may be slower at 0.
- **Torrents at once (0 = no limit)**, 0 by default, from 0 to 100: how many torrents may run at once, for the whole server (see [Torrents at once](#torrents-at-once)).

Each user also needs **Can play torrent streams**, in the **Access** section of their page under **Users**, next to **Can download** (see [Users](users.md#playing-torrent-streams)). It is off for every user, administrators included. Without it, or with the switch off, a user's versions hold no torrent, from the server's addons or their own. With both, torrent versions are listed like the other versions of an addon, in its order.

## Routes

### Network interface

**Network interface for torrents** names an interface such as `wg0`. Every torrent socket, TCP and UDP, for peers, trackers and the DHT alike, is bound to it (`SO_BINDTODEVICE`). Linux only.

- Tracker names are looked up over the interface, by the name servers of `/etc/resolv.conf`. A name server on the loopback, such as a local resolver that would ask another way, is never asked: name such a tracker by its address, or give the container a resolver on the VPN.
- Since Linux 5.7, a process needs no capability to bind a socket not yet bound to a device. Before 5.7 it needs `CAP_NET_RAW`, which `compose.yaml` drops.

### SOCKS5 proxy

**SOCKS5 proxy for torrents** is the proxy's address and port, such as `10.0.0.2:1080`, with **Proxy user** and **Proxy password** when it asks for them. The password is stored encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)).

- Peers and HTTP trackers go through its `CONNECT`, tracker names being looked up by the proxy.
- UDP trackers, the DHT and uTP need its `UDP ASSOCIATE`, which not every SOCKS5 server offers. Without it they stay off, the torrents play over TCP, and **System › Health** says so.
- Through a proxy, no peer can connect in: Polyfin only connects out. It does the same over an interface and through a WireGuard tunnel.

The proxy itself is reached the server's usual way: run it on the host or inside the VPN's network.

### WireGuard tunnel

**WireGuard configuration** takes the `.conf` a VPN provider gives for WireGuard, pasted whole. Polyfin runs the tunnel itself, for torrents alone: userspace WireGuard (wireguard-go, MIT licence) with its own network stack (gVisor's, Apache 2.0 licence), written in Go. Nothing is set up on the host: no `NET_ADMIN`, no `/dev/net/tun`, no route or firewall rule. It works the same on Linux, in Docker, on Windows and macOS, and in Polyfin Desktop. The rest of the server keeps its usual network.

- Every torrent socket is a socket of the tunnel's network stack: peers over TCP and uTP, HTTP and HTTPS trackers, UDP trackers and the DHT. Its packets go, encrypted, to the configuration's server alone, over one UDP socket of the host. UDP trackers, the DHT and uTP always work, unlike through a proxy.
- Tracker names are looked up through the tunnel, by the `DNS` servers of the configuration, never by the system's resolver: no DNS leak. A configuration without `DNS` is accepted, and then no name is looked up at all: trackers named by an address, peers and the DHT still work, trackers named by a host name are skipped. **Settings › Torrents** says so once it is saved.
- The server's own name, when `Endpoint` names one, is looked up the host's usual way, as any WireGuard client does: it is how the tunnel is reached, and tells only that name. It is looked up again when a handshake fails.
- The configuration is checked when it is saved, and refused with its line, its key and why: `PrivateKey`, `PublicKey` and `PresharedKey` must be keys of 32 bytes in base64, `Address` IP addresses with one IPv4 address among them, `Endpoint` a host and a port, `AllowedIPs` IP prefixes with `0.0.0.0/0` among them (the peers of a torrent are anywhere), `DNS` IP addresses, `MTU` 576 to 65535 (1420 by default), `PersistentKeepalive` off or 0 to 65535 seconds. One `[Peer]` alone is taken. The keys wg-quick uses to set its host up (`PostUp`, `PreDown`, `Table` and the like) are left aside: Polyfin never runs them.
- The configuration is stored encrypted with `POLYFIN_SECRET_KEY`, its private and preshared keys with it (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)), and never shown again: the settings say a configuration is saved, with its server, its addresses and its DNS servers. Logs never hold a key. Choosing another route removes it.
- Incoming connections are not supported: no peer can connect in, even through a port the provider forwards. Polyfin only connects out, as over the other routes.

#### Throughput and processor use

Measured on a Mac with an Apple M1 Max, the server and the source on the same machine (`go test -run '^$' -bench WireGuardRoute -benchtime 5x ./internal/torrent`), with other work running beside it. The processor time is Polyfin's side alone, the server running in a process of its own:

| Download | Throughput | Processor time per MB |
| --- | --- | --- |
| Direct connection | 4,975 MB/s | 0.21 ms |
| WireGuard tunnel | 83 MB/s (about 660 Mbit/s) | 32 ms |

The tunnel encrypts every packet and runs its own TCP in userspace, so it costs far more processor time than a kernel connection: about 0.4 of a core for a 100 Mbit/s download. Torrents rarely go that fast, and the download limit bounds them.

## Kill switch

With the interface missing, down or without an address, the proxy not answering, or the WireGuard server not answering its handshake, as with a wrong key, torrent versions do not play: they are skipped like a version that cannot be read (see [Playback](playback.md)), and **System › Health** says why under **Needs attention**. Nothing falls back to another route: the route is checked every 5 seconds while a torrent plays, and a torrent whose route drops stops at once. A WireGuard tunnel counts as up while its last handshake is less than 30 seconds old; past that, a check makes a new one and waits 8 seconds for its answer. A route's failure is not the version's: the version plays again as soon as the route is back, and Health clears. A WireGuard tunnel that stops, as when its configuration changes during a playback or the server shuts down, first closes its sockets and waits for what they were sending, then closes: the torrents on it stop, and nothing is sent through the tunnel as it closes.

## Reading and storage

- A torrent version is read through the source cache, as a link is: FFmpeg, ffprobe and apps read it the same way, and it is always relayed, never sent to an app.
- The file played is `fileIdx`. When the addon gives none, as some do for a season's pack, it is the file whose name is the stream's `behaviorHints.filename`; else, for an episode, the largest video whose name tells that episode, as `S01E02`, `s1e2`, `S01E01-E02`, `1x02`, or `E02` and `Episode 2` in a season's folder (`Season 01`, `S01`, `Specials`, or the pack's own folder, as `Show.S01.1080p`); else the largest file. The version stays the same version whichever file it finds: its identity, its analysis and the version rules do not depend on it.
- The trackers of `sources` (`tracker:<url>`, HTTP, HTTPS or UDP) are announced to, and its DHT entries (`dht:<info hash>` or `dht:<host>:<port>`) have the torrent looked up on the DHT, as has a torrent whose sources name no tracker. A lookup starts from the nodes the addon names, the well-known bootstrap nodes other clients use, and those the peers tell. The DHT and UDP trackers need a route that carries UDP: over a proxy without `UDP ASSOCIATE`, none of them is asked.
- An announce tells the trackers a port of the dynamic range (49152 to 65535), the same for the whole run, though nothing listens on it: trackers refuse port 0, and many refuse 6881 to 6889. The info hash and the peer ID are escaped byte by byte. A tracker that refuses the announce has its own reason logged at debug level, with its host.
- Pieces at the read position come first, then the start and the end of the file, for its index. A read fetches ahead within the file played alone, up to the piece that holds its last byte, which it may share with the next file: never the pieces of the files after it, such as the next episode of a season's pack or the files beside the video. Measured on the local test swarm, playing whole a 9.2 MB video followed by 1.9 MB of other files, with 256 KiB pieces, Polyfin used to download all 1.9 MB of them; it now downloads none of them, but the piece of the subtitle file it fetches on purpose (see [Subtitle files inside the torrent](#subtitle-files-inside-the-torrent)). They are kept on disk within a quarter of **Disk space for files being read (GB)** (**Settings › Playback**), and count in Health's **Source cache**.
- Peers tell Polyfin the other peers they are connected to (peer exchange), which it dials over the same route as the others, with what they tell of each (a seed, uTP). A torrent keeps 200 such peers at most, and reads 3 messages a minute of each peer. A private torrent, as its info dictionary says, uses no peer exchange: its peers come from its trackers, and those other peers told before its info dictionary arrived are dropped without being dialed. Polyfin tells no peer of its own: it accepts no connection, so no peer could reach it through another's message, and the peers it would tell are those the others find themselves. On the local test swarm, with a tracker listing one peer that sends a block every 50 ms and tells four others, Polyfin finds 5 peers rather than 1, and reads the first MiB in 10 ms rather than 3.3 seconds.
- A private torrent, as its info dictionary says, takes its peers from its trackers alone: no peer exchange, and no DHT. Once its info dictionary arrives, a DHT lookup running stops and none starts again, whatever its sources ask; before, a torrent is looked up as its sources ask, as any other. The DHT is only read: Polyfin announces no torrent there.
- When every block a read waits for has been asked of a peer, the blocks a slow peer still holds are asked of another peer too (the endgame): the first to arrive is kept and the other is cancelled. At most 16 blocks of 16 KiB are asked twice at once, and only blocks a read waits for: the pieces fetched ahead of a read, or for the file's index, stay with the peer asked. On the local test swarm, with a peer sending a block every 100 ms asked first, the endgame brings the first byte from 1.6 seconds to 4 ms.
- A peer may drop a request without telling it. Peers send the blocks asked in order, so a request is taken as dropped when blocks asked after it arrived and it is still missing 5 seconds later. The block is then cancelled and asked again: of another peer that has it, or of the same peer when no other does, so that a torrent a single peer serves does not wait for it for good. A slow peer is not taken for one that drops: its blocks keep coming, however long the last ones asked wait. A peer that sends not a byte for four times as long as a block takes it, 5 seconds at least, may have dropped the last requests asked of it, or its connection may stall, as uTP's do on a loaded link: it is asked for one more block, never asked of it yet, and nothing is asked twice; that block arriving first tells the requests before it were dropped. Only when no such block is left, after 30 seconds without a byte, is its oldest request asked again. A peer that drops the same block three times is no longer asked for it, and keeps sending the others. A peer that sends no block asked for 3 minutes, keep-alives aside, is given up: its connection is closed, and it is dialed again only 5 minutes later.
- Peers are reached over TCP and uTP, both through the route (see [uTP](#utp)). A torrent connects to 30 peers at most, and dials 16 at once, each dial having 20 seconds for its handshake. On the local test swarm, with 23 peers that never answer listed before the one seeder, the torrent starts in 20 seconds with 16 dials at once, and in 40 with 8. The connections through a proxy stay within the 30.
- A peer whose dial failed, or whose connection ended, is dialed again 10 seconds later, then after twice as long for each failure in a row, up to 5 minutes: a peer refuses while its slots are full, or a route drops a connection, and the peer may take the next one. A connection that lasted a minute counts as no failure. A peer given up for sending none of the blocks asked waits the 5 minutes.
- What peers, trackers, DHT nodes and the proxy send is checked against bounds before Polyfin keeps it: a malformed message ends only its own connection, announce or lookup, and the torrent goes on with the others. A peer exchange message of more than 8 KiB, or of more than 100 peers added or dropped, is malformed. A peer that keeps asking without reading the answers is disconnected once 4 MiB wait for it.

## uTP

uTP (BEP 29) carries the peer protocol over UDP, and yields to the other traffic of the link as the delay it adds grows (LEDBAT). Many peers take it, and some take nothing else: Polyfin reaches peers over it beside TCP, over every route, its implementation written in Go within Polyfin.

- A peer is tried over uTP first. When uTP has not answered within half a second, TCP is tried too, and at once when uTP fails. While a torrent has no peer yet, TCP connects at once beside uTP, so that its start waits for nothing. Only the connections race: the BitTorrent handshake goes over one at a time, uTP's when it is up, and over the other one only if the first fails, as a peer drops a second connection that brings the same peer ID. A peer that refuses TCP and does not answer uTP holds a connection attempt 3 seconds, after two SYNs.
- A uTP connection that ends tells its peer with a FIN, the torrent's stop included: a peer that was not told keeps it, and may refuse Polyfin's next connection from the same address as a duplicate.
- uTP needs a route that carries UDP, like the DHT and UDP trackers: over a proxy without `UDP ASSOCIATE`, it stays off and peers are reached over TCP alone. Peers with an IPv6 address are reached over TCP.
- Each torrent has one UDP socket of the route for uTP, opened at its first uTP connection and closed when it stops, apart from the sockets of the DHT and the UDP trackers. Other clients share one socket and port for all three, so that peers reach them in; Polyfin listens for no peer, so a shared port would bring nothing, and each socket ends with what it serves. Through a proxy, that is one more `UDP ASSOCIATE` for each torrent playing.
- A uTP connection whose peer stops answering ends after about 15 seconds of retransmissions, and the blocks asked of it go to the other peers.
- What peers send over uTP is checked against bounds like the rest: a packet's header and its extensions are read within the packet, a selective acknowledgment past 32 bytes is ignored, and each connection keeps at most 2 MiB received and not yet read, 1 MiB of it out of order.
- **System › Health** tells how many of the peers were reached over uTP. With **Detailed log** on under **Settings › Diagnostics**, the log says for each peer reached whether it was over uTP or TCP: `A torrent reached a peer over=uTP`.

### Measured on the local test swarm

On a Mac with an Apple M1 Max, through a SOCKS5 proxy with `UDP ASSOCIATE` on the same machine, 5 runs each (`go test -run '^$' -bench UTPSwarm -benchtime 5x ./internal/torrent`). The mixed swarm has 4 seeders over TCP alone, 4 over uTP alone, and 4 addresses where nothing answers; the TCP swarm 4 seeders over TCP alone. Peers are counted 3 seconds after the first byte:

| Swarm | Peers reached over | Time to the first byte | Peers reached | Over uTP |
| --- | --- | --- | --- | --- |
| Mixed | TCP alone | 9.6 ms | 4 | 0 |
| Mixed | uTP and TCP | 30.8 ms | 8 | 4 |
| TCP | TCP alone | 8.8 ms | 4 | 0 |
| TCP | uTP and TCP | 31.0 ms | 4 | 0 |

uTP reaches the peers TCP does not. A swarm without uTP starts nearly as fast: until a torrent has its first peer, TCP connects beside uTP, and its handshake waits for uTP only about as long as its own connection took, 20 ms at least. When uTP had its half-second head start from the first peer on, that swarm's first byte took 510 ms; the head start now delays only the later peers, each reached over TCP half a second later. On the loopback, a first byte comes within milliseconds; over the internet, the round trips of trackers and peers set it.

### Over a path with jitter

A WireGuard tunnel to a far server adds delay, and a delay that varies. LEDBAT takes a delay above the lowest of the last two minutes for a queue, and yields to it; so that a jitter alone is not taken for one, the current delay is the lowest of about the last round trip, as RFC 6817 recommends, and the two-minute history also follows the drift between the two clocks. Measured on the same Mac between two network stacks joined by a path of 50 Mbit/s with 250 ms of queue, the delay and the jitter each way, in order, 4 MiB at a time, 3 runs each (`go test -run '^$' -bench Path -benchtime 3x ./internal/torrent/utp`). TCP is the WireGuard route's, with selective acknowledgments:

| Path, each way | uTP | TCP |
| --- | --- | --- |
| 40 ms ± 15 ms | 3.41 MB/s | 3.59 MB/s |
| 40 ms ± 15 ms, 1% loss | 0.35 MB/s | 0.33 MB/s |
| 80 ms ± 40 ms | 1.93 MB/s | 2.10 MB/s |
| 80 ms ± 40 ms, 1% loss | 0.16 MB/s | 0.16 MB/s |

## Only while playing

Only a playback starts a torrent, and a subtitle file inside it that a user picks before it was fetched (see [Subtitle files inside the torrent](#subtitle-files-inside-the-torrent)). Preparing playback in advance, thumbnails and downloads do not: a torrent version cannot be downloaded, and has no thumbnails. When the playback ends, after 30 seconds without a read, as when a player pauses or between its requests, the torrent stops: no more sharing, no more peers, and its pieces are removed. The trackers are told it stopped.

A playback of another title ends that wait at once. When a device reports that it plays another title, a torrent's or not, the torrents it played before stop then, and so do those its user played on another device that stopped since:

- A torrent another device, a DLNA device, a guest or a converted download still plays, or that another user stopped less than a minute ago, runs on.
- A pause, a seek, another audio track, a replay of the same title, and another episode of the same torrent, as in a season pack, keep it.
- A stop alone ends nothing at once: jellyfin-web may send the stop of a playback late, once the next playback of the same title started.
- A late report of the title the device left, such as a progress of its play session that arrives after the other title started, ends nothing and keeps nothing: the device plays the title of its latest playback.
- The repackaging and conversions the device made of the title it left stop as it reports the other title, even when its app sent no stop (see [Transcoding](transcoding.md#hls-segments)), and the torrent with them. A torrent still read otherwise as the other title starts stops as soon as that read ends.

Once no playback reads a torrent's file any longer, Polyfin stops reading ahead in it and closes its reads at once, rather than finishing them and keeping them open for a next read. What the peers were asked for still comes: on the local test swarm, with a peer sending about 160 KB/s, a stop went from 3.3 MB more over 20 seconds to 0.6 MB over 4 seconds, and switching to another title from 3.2 MB over 19 seconds, with both torrents running 30 seconds, to 1.2 MB over 6 seconds, the first torrent stopping as the second title's playback was reported.

**Sync addon subtitles** (see [Subtitles](subtitles.md#addon-subtitles-synced-to-the-version)) looks for no speech in a torrent version: that would download three stretches of it from the swarm, up to 300 MB, as its playback starts, beside it, and what the playback holds then, its start and its index, has none of them. A torrent version's addon subtitles come with the times the addons give.

The subtitle files an addon lists with a torrent stream are that version's own, listed from the title's page on, before any play. Polyfin downloads them from their links, without joining the swarm (see [Subtitles](subtitles.md#subtitle-files-listed-with-a-stream)).

### Subtitle files inside the torrent

The text subtitle files of the torrent that go with the video played, such as `Movie.fr.srt` beside it or `Subs/English.srt`, are listed for its version from its first play, once the torrent tells its files, and kept with it (see [Subtitles](subtitles.md#subtitle-files-inside-torrents)). Their text comes from the swarm:

- A file costs the pieces that hold it, whole, whatever its size: one piece, or two when it lies across them, of up to 16 MiB each in the torrents of movies and episodes. Only those pieces are fetched: none past the file, nor the start and end of a file that a video's read fetches for its index. Files that share a piece, as small files next to each other often do, cost it once while the torrent runs.
- So the file in the user's preferred subtitle language alone is fetched as the play starts, beside the video's first pieces; each other file is fetched when a user picks it, and the app waits for it. Once fetched, a file is kept, and costs nothing more.
- A fetch joins the torrent as a playback does, through the route and under the kill switch. A file picked once the torrent stopped starts it again, for that fetch alone; the torrent then stops 30 seconds later, as after a playback. The fetch runs to its end, for up to 105 seconds, the torrent's start and a piece's wait, even when the app stops waiting.

Measured on a Mac with an Apple M1 Max, on the local test swarm, one peer sending a block of 16 KiB every 10 ms (about 1.6 MB/s), fetching a SubRip file of 60 KB once the torrent started, 3 runs each (`go test -run '^$' -bench SubtitleFetch -benchtime 3x ./internal/torrent`):

| Piece length | Downloaded | Time |
| --- | --- | --- |
| 1 MiB | 1 MiB | 0.73 s |
| 4 MiB | 4 MiB | 2.96 s |
| 16 MiB | 16 MiB | 11.92 s |

The time is the piece's at the swarm's speed; the playback shares that speed while the file comes.

## How a torrent starts

1. Opening a title's page lists its torrent versions. Nothing reads them yet: preparing playback ahead leaves them alone, and a torrent version that was never played has no analysis.
2. Pressing **Play** sends PlaybackInfo. Among the versions it tries (**Versions tried when one does not work**, see [Playback](playback.md)), each torrent version is tried as the play's own, even when it is not the first: the play joins its swarm before anything reads it, through the route.
3. The torrent asks its trackers and the DHT for peers, connects to them, and receives its info dictionary from one of them. The pieces at the start and the end of the file come first.
4. The version is analyzed inside that playback: its first bytes are waited for, then ffprobe reads it, both as long as the play waits for the torrent (see [Deadlines](#deadlines)), and the analysis is kept for later plays. The subtitle files inside the torrent that go with the file are listed and kept too. PlaybackInfo then answers the version as the app plays it: as it is, remuxed, or converted.
5. The stream or HLS requests the app sends next read the torrent as the same playback.

A torrent version analyzed once plays at once the next time: its analysis and its subtitle files are kept, and only the stream joins the swarm. A version analyzed before Polyfin 0.46 has its subtitle files listed at its next play, whose PlaybackInfo waits for the torrent's files, within the same deadlines.

### Deadlines

A link's host sends its headers within seconds. A torrent needs peers first, from trackers that may each take 30 seconds to answer and from DHT lookups that hop from node to node, then its info dictionary, then its first pieces. So a torrent's playback waits longer:

- 60 seconds for the torrent to start: peers found and the info dictionary received. That fits two rounds of a slow tracker beside the DHT lookups. A lookup that found no peer, as when its nodes' answers were lost or came after 3 seconds, is made again 15 seconds later; one that found peers, every 5 minutes.
- 30 seconds for a piece, at the start as later: a peer chooses whom it sends to every 10 seconds, and tries a new peer every 30. What counts is a byte of the torrent arriving, not a whole block of the file: a piece is read only once it is whole and checked, so a slow swarm, such as a peer sending 16 KB/s, takes longer than 30 seconds for the 1 MiB a read asks, while its blocks keep coming. A read is taken as stalled only when no block arrives for 30 seconds, twice in a row.
- Its first bytes, then what ffprobe reads of it, are waited for as the play waits for the torrent, rather than within **Maximum time to analyze a version**: as long as its start, 15 seconds and a piece (105 seconds), then on while the torrent keeps receiving its pieces, a byte at least every 30 seconds, up to 3 minutes from the play.
- PlaybackInfo waits for a torrent version as long as its start and a piece, 90 seconds, not for its whole analysis. Past that, it waits on while the torrent keeps receiving its pieces, a byte at least every 30 seconds, up to 3 minutes from the play: a slow swarm that keeps sending plays at the first play, once the version's analysis and the read of its index are in. A torrent version still being read once its pieces stop coming, or at 3 minutes, is passed over, and the answer is `RateLimitExceeded` (see [When torrents do not start](#when-torrents-do-not-start)) when no other version plays. Its analysis and the read of its index go on as long as they wait for the torrent, and are kept: the next play of the title uses them at once. The play itself waits no longer for them, and leaves nothing behind waiting. A route that drops while the torrent starts, or a swarm that sends nothing, so never holds the app's request past 90 seconds, nor a slow swarm past 3 minutes. jellyfin-web sets no limit on that request, nor do Traefik and Caddy by default; nginx closes a response silent for 60 seconds unless told otherwise (see [Reverse proxy](reverse-proxy.md)).
- The read of a version's index, for its HLS segments, waits for a torrent as its analysis does, and ends within 3 minutes in any case, read whole or failed: what it reads waits at most a piece's wait at a time, twice, so it ends well before; a read that would still wait then is tried again at the next play.
- An analysis or a read of the index that gave up on a torrent's pieces, as they stopped coming or came too slowly for the play, is a torrent that did not start in time: the play answers `RateLimitExceeded` when no other version plays, never that the version suits no app, and the version is not kept as failed: the next play tries it again at once. So do the other reads of its file, which otherwise remember a failure for a while: locating its subtitle tracks through its index, reading one of them whole, reading the first frame of its HDR video, reading a picture subtitle track into text. One that gave up on the torrent's pieces, or ran while no playback played the torrent, as when the title's page opened or after a pause past the torrent's 30 seconds, is made again at the next play. A torrent that never started, and what the file itself holds, as a track the index does not list, are remembered as before.

A torrent that does not start in time is not tried again for 15 minutes, like a version that cannot be read, and the other versions are played meanwhile. A torrent that started but whose peers then send nothing, as when they hold its pieces, is a torrent that did not start in time too: the play answers `RateLimitExceeded` when no other version plays, but the torrent is not kept out, and the next play tries it again at once, which plays as soon as the peers send.

## Torrents at once

On a slow link, or a route that allows few connections, torrents running at once share the bandwidth and the peers, and each starts more slowly. **Torrents at once (0 = no limit)**, under **Settings › Torrents**, bounds them for the whole server. There is no limit per user; **Playbacks at once**, on each user's page, counts their devices playing, not their torrents.

What counts:

- Each torrent, not each playback. Every torrent running counts once, however many devices or users play it: a title played on two devices, or two episodes of one season pack, take one place.
- Its linger too. A torrent counts until it stops: 30 seconds after the last read of its playback, as when a player pauses, or at once when the same device or user plays another title (see [Only while playing](#only-while-playing)).
- Whatever starts it. A playback through a Jellyfin app, a DLNA device or a share link, and the fetch of a subtitle file inside a torrent that was not fetched yet (see [Subtitle files inside the torrent](#subtitle-files-inside-the-torrent)), each start the torrent, and it counts. Converted downloads never do: a torrent version cannot be downloaded. Nor does an analysis without a play: a torrent version is analysed only inside its play (see [How a torrent starts](#how-a-torrent-starts)).

At the cap:

- A torrent that does not run does not start. A play passes over its torrent version, as one that did not start in time, and tries the next versions within **Versions tried when one does not work**: a version that is a link plays. With none, PlaybackInfo answers `RateLimitExceeded`, and jellyfin-web says the media cannot be played at this time (see [When torrents do not start](#when-torrents-do-not-start)). The torrent version is not kept as failed: the next play tries it again.
- DLNA devices and guests through share links count the same, and at the cap a torrent that does not run does not start for them either.
- A torrent already running is joined as usual: another device playing the same title takes no place more.
- A device that left a torrent for another title, which that title's start would end at once, gives its place to that title: when its next PlaybackInfo finds the cap reached, the torrents the device or its user left are ended first, as the start would end them, and the new torrent starts in their place. A torrent ended stops once its last read ends: an app's player that just stopped, as jellyfin-web's, may still read the file for a moment, so PlaybackInfo waits up to 5 seconds for it to stop before it tells the title cannot be played now. The cap is never passed meanwhile. Another user's torrent is never ended for it, nor one another device still plays, whether its app names the version it plays or, as jellyfin-web playing a title as it is, only the title: the version is then the one that device's PlaybackInfo chose.
- Two torrents starting at the same moment for the last place: the first to start takes it, the other is refused as at the cap.
- The log tells it at Info level: `A torrent version is passed over: as many torrents run as Torrents at once allows`.

**Can play torrent streams**, **Playbacks at once**, the version rules and the quality groups apply as before: the cap only removes torrent versions from those a play tries while it is reached.

## When torrents do not start

When no version of a title plays because a torrent could not start, its route being down or no peer sending it in time, PlaybackInfo answers Jellyfin's `RateLimitExceeded` rather than `NoCompatibleStream`: jellyfin-web says the media cannot be played at this time, rather than that the app is not compatible with it. The log tells why in one line at Info level (`A torrent did not start in time`, with the peers found, how the trackers answered, and the peers not connected counted by how their last dial or connection ended), and **System › Health** too. For example, `23 peers found, 0 connected (15 timed out, 8 refused)`:

- timed out: no answer within 20 seconds, over TCP and over uTP, or a proxy that tells the connection timed out;
- refused: the peer refused the connection, directly or as the SOCKS5 proxy tells (its reply 5);
- unreachable: no route to the peer, directly or as the proxy tells;
- refused by the proxy: another refusal of the SOCKS5 proxy, by its rules or for its own failure;
- closed the connection: the peer ended the connection before its handshake, as a peer does with a torrent it does not have;
- left: the peer was connected, and its connection ended;
- failed otherwise: another failure;
- not dialed yet: the peer waits for a dial, or its first dial runs.

A peer tried over both uTP and TCP is counted by TCP's answer.

What to check:

- **System › Health** under **Needs attention**: a route that cannot carry torrents, a proxy without `UDP ASSOCIATE`, or a torrent that did not start, and why.
- That the route reaches the internet: an interface that is up but routes nowhere, or a proxy that cannot reach out, finds no peer.
- UDP: through a proxy, it needs `UDP ASSOCIATE`, and the proxy must relay the datagrams back. Without UDP, only HTTP trackers find peers, many torrents list UDP trackers alone, and the peers that take uTP alone are not reached.
- The trackers' answers: turn on **Detailed log** under **Settings › Diagnostics**, play again, and read `A tracker could not be reached` in **System › Logs**, with each tracker's own reason for a refusal.
- The swarm itself: a torrent with no seeder does not start anywhere. Try another version.

## Docker

`compose.yaml` runs Polyfin read-only, as user 65532, with no capabilities. A proxy and a WireGuard tunnel need none of the following; an interface must be seen by the container:

- WireGuard on the host: run Polyfin with the host's network (`compose.host.yaml`), and name the host's interface.
- A WireGuard container: give it `NET_ADMIN`, and have Polyfin's container share its network with `network_mode: service:<name>`. Polyfin's ports are then published on the WireGuard container.

On Linux 5.7 or later, Polyfin's own container needs no added capability.

## Health

**System › Health** shows a **Torrents** panel while torrents are on: the route, a WireGuard tunnel's addresses and last handshake, the torrents playing (against **Torrents at once** when it is set, such as `1 of 2`), their peers and how many of them were reached over uTP, and what was shared since the start. The **Source cache** tells the part torrent pieces take. Under **Needs attention**, an error names a route that cannot carry the torrents and why, such as a WireGuard server that does not answer the handshake, a warning a proxy without `UDP ASSOCIATE`, and a warning a torrent that did not start in time in the last 15 minutes and why, with its peers counted by how their last dial ended (see [When torrents do not start](#when-torrents-do-not-start)), until a torrent starts. Health waits 2 seconds for a route's check. A WireGuard tunnel's check runs in the background and gives the handshake its 8 seconds: past Health's 2, a server that did not answer is told as not answering, and Health then tells the tunnel's last check at once rather than waiting again. While Health is open, a tunnel found down is checked again 5 seconds after each check, so one that comes back stops being reported within 13 seconds, a few more when `Endpoint` names the server by a host name, which is then looked up again.
