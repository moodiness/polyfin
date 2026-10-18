# Torrents

Some addons list torrents rather than links: a stream with an `infoHash`, the index of the file to play (`fileIdx`), and its trackers and DHT nodes (`sources`). Polyfin can play them, with all of their traffic kept to one route, a network interface such as a VPN's, or a SOCKS5 proxy. Nothing is ever sent another way. It is off by default.

Playing a torrent means joining its swarm: the other peers see the address the route gives, and Polyfin shares what it downloaded while the upload limit allows. You answer for what the server downloads and shares.

## Turning torrents on

Under **Settings › Torrents**:

- **Play torrent streams** turns torrents on. It cannot be on without a route.
- **Route** chooses between **Network interface** and **SOCKS5 proxy**. On Windows and macOS, as on Polyfin Desktop, only the proxy is offered: those systems cannot bind a socket to a network interface.
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
- Through a proxy, no peer can connect in: Polyfin only connects out. It does the same over an interface.

The proxy itself is reached the server's usual way: run it on the host or inside the VPN's network.

## Kill switch

With the interface missing, down or without an address, or the proxy not answering, torrent versions do not play: they are skipped like a version that cannot be read (see [Playback](playback.md)), and **System › Health** says why under **Needs attention**. Nothing falls back to another route: the route is checked every 5 seconds while a torrent plays, and a torrent whose route drops stops at once. A route's failure is not the version's: the version plays again as soon as the route is back, and Health clears.

## Reading and storage

- A torrent version is read through the source cache, as a link is: FFmpeg, ffprobe and apps read it the same way, and it is always relayed, never sent to an app.
- The file played is `fileIdx`, or the largest one when the addon names none, as Stremio does. The trackers of `sources` (`tracker:<url>`, HTTP, HTTPS or UDP) are announced to, and its DHT entries (`dht:<info hash>` or `dht:<host>:<port>`) have the torrent looked up on the DHT. Polyfin names no DHT bootstrap node: a lookup starts from the nodes the addon names and those the peers tell.
- Pieces at the read position come first, then the start and the end of the file, for its index. They are kept on disk within a quarter of **Disk space for files being read (GB)** (**Settings › Playback**), and count in Health's **Source cache**.
- uTP is off: peers are reached over TCP.

## Only while playing

Only a playback starts a torrent. Preparing playback in advance, thumbnails and downloads do not: a torrent version cannot be downloaded, and has no thumbnails. When the playback ends, after 30 seconds without a read, as when a player pauses, the torrent stops: no more sharing, no more peers, and its pieces are removed. The trackers are told it stopped.

## Docker

`compose.yaml` runs Polyfin read-only, as user 65532, with no capabilities. A proxy needs none of the following; an interface must be seen by the container:

- WireGuard on the host: run Polyfin with the host's network (`compose.host.yaml`), and name the host's interface.
- A WireGuard container: give it `NET_ADMIN`, and have Polyfin's container share its network with `network_mode: service:<name>`. Polyfin's ports are then published on the WireGuard container.

On Linux 5.7 or later, Polyfin's own container needs no added capability.

## Health

**System › Health** shows a **Torrents** panel while torrents are on: the route, the torrents playing, their peers, and what was shared since the start. The **Source cache** tells the part torrent pieces take. Under **Needs attention**, an error names a route that cannot carry the torrents and why, and a warning a proxy without `UDP ASSOCIATE`.

## Study: WireGuard built in

Polyfin could run its own tunnel from a provider's `.conf` pasted in the settings, used by torrent traffic alone: userspace WireGuard with its netstack, no host setup, no `NET_ADMIN`, no `/dev/net/tun`. Still to study: throughput and CPU against a kernel interface, the `.conf`'s DNS servers, keeping the private key encrypted with `POLYFIN_SECRET_KEY`, and incoming connections. Not built yet.
