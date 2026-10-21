# Torrents

Some addons list torrents rather than links: a stream with an `infoHash`, the index of the file to play (`fileIdx`), and its trackers and DHT nodes (`sources`). Polyfin can play them, with all of their traffic kept to one route: a network interface such as a VPN's, a SOCKS5 proxy, or a WireGuard tunnel Polyfin runs itself. Nothing is ever sent another way. It is off by default.

Playing a torrent means joining its swarm: the other peers see the address the route gives, and Polyfin shares what it downloaded while the upload limit allows. You answer for what the server downloads and shares.

## Turning torrents on

Under **Settings › Torrents**:

- **Play torrent streams** turns torrents on. It cannot be on without a route.
- **Route** chooses between **Network interface**, **SOCKS5 proxy** and **WireGuard tunnel**. On Windows and macOS, as on Polyfin Desktop, the network interface is not offered: those systems cannot bind a socket to one.
- **Download limit (Mbps, 0 = none)**, 0 by default.
- **Upload limit (Mbps, 0 = no sharing)**, 0 by default. Peers favour those that share back, so a start may be slower at 0.

Each user also needs **Can play torrent streams**, in the **Access** section of their page under **Users**, next to **Can download** (see [Users](users.md#playing-torrent-streams)). It is off for every user, administrators included. Without it, or with the switch off, a user's versions hold no torrent, from the server's addons or their own. With both, torrent versions are listed like the other versions of an addon, in its order.

## Routes

### Network interface

**Network interface for torrents** names an interface such as `wg0`. Every torrent socket, TCP and UDP, for peers, trackers and the DHT alike, is bound to it (`SO_BINDTODEVICE`). Linux only.

- Tracker names are looked up over the interface, by the name servers of `/etc/resolv.conf`. A name server on the loopback, such as a local resolver that would ask another way, is never asked: name such a tracker by its address, or give the container a resolver on the VPN.
- Since Linux 5.7, a process needs no capability to bind a socket not yet bound to a device. Before 5.7 it needs `CAP_NET_RAW`, which `compose.yaml` drops.

### SOCKS5 proxy

**SOCKS5 proxy for torrents** is the proxy's address and port, such as `10.0.0.2:1080`, with **Proxy user** and **Proxy password** when it asks for them. The password is stored encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)).

- Peers and HTTP trackers go through its `CONNECT`, tracker names being looked up by the proxy.
- UDP trackers and the DHT need its `UDP ASSOCIATE`, which not every SOCKS5 server offers. Without it they stay off, the torrents play over TCP, and **System › Health** says so.
- Through a proxy, no peer can connect in: Polyfin only connects out. It does the same over an interface and through a WireGuard tunnel.

The proxy itself is reached the server's usual way: run it on the host or inside the VPN's network.

### WireGuard tunnel

**WireGuard configuration** takes the `.conf` a VPN provider gives for WireGuard, pasted whole. Polyfin runs the tunnel itself, for torrents alone: userspace WireGuard (wireguard-go, MIT licence) with its own network stack (gVisor's, Apache 2.0 licence), written in Go. Nothing is set up on the host: no `NET_ADMIN`, no `/dev/net/tun`, no route or firewall rule. It works the same on Linux, in Docker, on Windows and macOS, and in Polyfin Desktop. The rest of the server keeps its usual network.

- Every torrent socket is a socket of the tunnel's network stack: peers over TCP, HTTP and HTTPS trackers, UDP trackers and the DHT. Its packets go, encrypted, to the configuration's server alone, over one UDP socket of the host. UDP trackers and the DHT always work, unlike through a proxy.
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

With the interface missing, down or without an address, the proxy not answering, or the WireGuard server not answering its handshake, as with a wrong key, torrent versions do not play: they are skipped like a version that cannot be read (see [Playback](playback.md)), and **System › Health** says why under **Needs attention**. Nothing falls back to another route: the route is checked every 5 seconds while a torrent plays, and a torrent whose route drops stops at once. A WireGuard tunnel counts as up while its last handshake is less than 30 seconds old; past that, a check makes a new one and waits 8 seconds for its answer. A route's failure is not the version's: the version plays again as soon as the route is back, and Health clears.

## Reading and storage

- A torrent version is read through the source cache, as a link is: FFmpeg, ffprobe and apps read it the same way, and it is always relayed, never sent to an app.
- The file played is `fileIdx`, or the largest one when the addon names none, as Stremio does. The trackers of `sources` (`tracker:<url>`, HTTP, HTTPS or UDP) are announced to, and its DHT entries (`dht:<info hash>` or `dht:<host>:<port>`) have the torrent looked up on the DHT, as has a torrent whose sources name no tracker. A lookup starts from the nodes the addon names, the well-known bootstrap nodes other clients use, and those the peers tell. The DHT and UDP trackers need a route that carries UDP: over a proxy without `UDP ASSOCIATE`, none of them is asked.
- An announce tells the trackers a port of the dynamic range (49152 to 65535), the same for the whole run, though nothing listens on it: trackers refuse port 0, and many refuse 6881 to 6889. The info hash and the peer ID are escaped byte by byte. A tracker that refuses the announce has its own reason logged at debug level, with its host.
- Pieces at the read position come first, then the start and the end of the file, for its index. They are kept on disk within a quarter of **Disk space for files being read (GB)** (**Settings › Playback**), and count in Health's **Source cache**.
- uTP is off: peers are reached over TCP.
- What peers, trackers, DHT nodes and the proxy send is checked against bounds before Polyfin keeps it: a malformed message ends only its own connection, announce or lookup, and the torrent goes on with the others. A peer that keeps asking without reading the answers is disconnected once 4 MiB wait for it.

## Only while playing

Only a playback starts a torrent. Preparing playback in advance, thumbnails and downloads do not: a torrent version cannot be downloaded, and has no thumbnails. When the playback ends, after 30 seconds without a read, as when a player pauses, the torrent stops: no more sharing, no more peers, and its pieces are removed. The trackers are told it stopped.

## How a torrent starts

1. Opening a title's page lists its torrent versions. Nothing reads them yet: preparing playback ahead leaves them alone, and a torrent version that was never played has no analysis.
2. Pressing **Play** sends PlaybackInfo. Among the versions it tries (**Versions tried when one does not work**, see [Playback](playback.md)), each torrent version is tried as the play's own, even when it is not the first: the play joins its swarm before anything reads it, through the route.
3. The torrent asks its trackers and the DHT for peers, connects to them, and receives its info dictionary from one of them. The pieces at the start and the end of the file come first.
4. The version is analyzed inside that playback: its first bytes are waited for, then ffprobe reads it within **Maximum time to analyze a version** as it reads any file, and the analysis is kept for later plays. PlaybackInfo then answers the version as the app plays it: as it is, remuxed, or converted.
5. The stream or HLS requests the app sends next read the torrent as the same playback.

A torrent version analyzed once plays at once the next time: its analysis is kept, and only the stream joins the swarm.

### Deadlines

A link's host sends its headers within seconds. A torrent needs peers first, from trackers that may each take 30 seconds to answer and from DHT lookups that hop from node to node, then its info dictionary, then its first pieces. So a torrent's playback waits longer:

- 60 seconds for the torrent to start: peers found and the info dictionary received. That fits two rounds of a slow tracker beside the DHT lookups.
- 30 seconds for a piece, at the start as later: a peer chooses whom it sends to every 10 seconds, and tries a new peer every 30.
- Its first bytes, within those, before ffprobe's own **Maximum time to analyze a version** starts.

A torrent that does not start in time is not tried again for 15 minutes, like a version that cannot be read, and the other versions are played meanwhile.

## When torrents do not start

When no version of a title plays because a torrent could not start, its route being down or no peer sending it in time, PlaybackInfo answers Jellyfin's `RateLimitExceeded` rather than `NoCompatibleStream`: jellyfin-web says the media cannot be played at this time, rather than that the app is not compatible with it. The log tells why at Info level (`A torrent did not start in time`, with the peers found and how the trackers answered), and **System › Health** too.

What to check:

- **System › Health** under **Needs attention**: a route that cannot carry torrents, a proxy without `UDP ASSOCIATE`, or a torrent that did not start, and why.
- That the route reaches the internet: an interface that is up but routes nowhere, or a proxy that cannot reach out, finds no peer.
- UDP: through a proxy, it needs `UDP ASSOCIATE`, and the proxy must relay the datagrams back. Without UDP, only HTTP trackers find peers, and many torrents list UDP trackers alone.
- The trackers' answers: turn on **Detailed log** under **Settings › Diagnostics**, play again, and read `A tracker could not be reached` in **System › Logs**, with each tracker's own reason for a refusal.
- The swarm itself: a torrent with no seeder does not start anywhere. Try another version.

## Docker

`compose.yaml` runs Polyfin read-only, as user 65532, with no capabilities. A proxy and a WireGuard tunnel need none of the following; an interface must be seen by the container:

- WireGuard on the host: run Polyfin with the host's network (`compose.host.yaml`), and name the host's interface.
- A WireGuard container: give it `NET_ADMIN`, and have Polyfin's container share its network with `network_mode: service:<name>`. Polyfin's ports are then published on the WireGuard container.

On Linux 5.7 or later, Polyfin's own container needs no added capability.

## Health

**System › Health** shows a **Torrents** panel while torrents are on: the route, a WireGuard tunnel's addresses and last handshake, the torrents playing, their peers, and what was shared since the start. The **Source cache** tells the part torrent pieces take. Under **Needs attention**, an error names a route that cannot carry the torrents and why, such as a WireGuard server that does not answer the handshake, a warning a proxy without `UDP ASSOCIATE`, and a warning a torrent that did not start in time in the last 15 minutes and why, until a torrent starts.
