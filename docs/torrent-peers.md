# Torrent peers and pieces

This page is for administrators who want to know how Polyfin downloads a torrent. It covers the trackers and the DHT, the pieces, the peers, and uTP. [Torrents](torrents.md) covers turning torrents on, their routes and their limits.

Every connection on this page goes through the torrents' route alone (see [Routes](torrents.md#routes)).

## Trackers and the DHT

- The trackers of `sources` (`tracker:<url>`, HTTP, HTTPS or UDP) are announced to, and so are those of **Trackers added to every torrent** (see [Trackers](torrents.md#trackers)).
- The DHT entries of `sources` (`dht:<info hash>` or `dht:<host>:<port>`) have the torrent looked up on the DHT.
- So has a torrent whose sources name no tracker, whatever the setting adds.
- A lookup starts from the nodes nearest the torrent that answered its last lookup, the nodes the addon names, the well-known bootstrap nodes other clients use, and those the peers tell.
- The DHT and UDP trackers need a route that carries UDP. Over a proxy without `UDP ASSOCIATE`, none of them is asked.
- The DHT is only read: Polyfin announces no torrent there.
- An announce tells the trackers a port of the dynamic range (49152 to 65535), the same for the whole run, though nothing listens on it. Trackers refuse port 0, and many refuse 6881 to 6889.
- The info hash and the peer ID are escaped byte by byte.
- A tracker that refuses the announce has its own reason logged at debug level, with its host.

## Pieces and reading ahead

- Pieces at the read position come first, then the start and the end of the file, for its index.
- A read fetches ahead within the file played alone, up to the piece that holds its last byte, which it may share with the next file.
- It never fetches the pieces of the files after it, such as the next episode of a season's pack or the files beside the video.
- Measured on the local test swarm: a 9.2 MB video played whole, followed by 1.9 MB of other files in 256 KiB pieces. Polyfin downloads none of the other files, but the piece of the subtitle file it fetches on purpose (see [Subtitle files inside the torrent](torrents.md#subtitle-files-inside-the-torrent)).
- Pieces are kept on disk within a quarter of **Disk space for files being read (GB)** (**Settings › Playback**). They count in Health's **Source cache**.
- A read fetches 16 MiB ahead, 4 pieces at least, within half the room that disk space leaves for the torrent's pieces.
- The window rolls with the read. Once a read moves past the first half of the pieces last asked for, the peers are asked for the next ones, so that they arrive before the read reaches them.
- Measured on the local test swarm, a file in 4 MiB pieces is read at 1 MiB a second, and downloaded at 16 Mb/s. No read waits but those of the first pieces, as the play starts.
- The download comes in bursts of half the window. Once the pieces ahead are there, nothing more is asked until the read moves on.
- So a transfer that pauses for a few seconds while the video plays is normal. Played in an app at 16 Mb/s, once the source cache had read as far ahead as it reads, the swarm sent 8 MB every 4 seconds.

## Peers

- Peers are reached over TCP and uTP, both through the route (see [uTP](#utp)).
- A torrent connects to 30 peers at most, and dials 16 at once. Each dial has 20 seconds for its handshake.
- The connections through a proxy stay within the 30.
- On the local test swarm, with 23 peers that never answer listed before the one seeder, the torrent starts in 20 seconds with 16 dials at once, and in 40 with 8.
- A peer whose dial failed, or whose connection ended, is dialed again 10 seconds later. Each failure in a row doubles the wait, up to 5 minutes.
- A peer refuses while its slots are full, or a route drops a connection, and the peer may take the next one.
- A connection that lasted a minute counts as no failure.
- A peer given up for sending none of the blocks asked waits the 5 minutes.
- A dial the route did not carry, as through a proxy that does not answer, is not the peer's failure.
- Once the route answers again after an outage, the peers waiting are dialed at once, those given up aside.

## Peer exchange

- Peers tell Polyfin the other peers they are connected to. Polyfin dials those over the same route as the others, with what the peers tell of each (a seed, uTP).
- A torrent keeps 200 such peers at most, and reads 3 messages a minute of each peer.
- A private torrent, as its info dictionary says, uses no peer exchange: its peers come from its trackers.
- The other peers told before a private torrent's info dictionary arrived are dropped without being dialed.
- Polyfin tells no peer of its own. It accepts no connection, so no peer could reach it through another's message, and the peers it would tell are those the others find themselves.
- On the local test swarm, a tracker lists one peer that sends a block every 50 ms and tells four others. Polyfin finds 5 peers rather than 1, and reads the first MiB in 10 ms rather than 3.3 seconds.

A private torrent takes its peers from its trackers alone: no peer exchange, and no DHT.

- Once its info dictionary arrives, a running DHT lookup stops, and none starts again, whatever its sources ask.
- Before that, a torrent is looked up as its sources ask, as any other.

## Slow and silent peers

When every block a read waits for has been asked of a peer, the blocks a slow peer still holds are asked of another peer too (the endgame):

- The first to arrive is kept, and the other is cancelled.
- At most 16 blocks of 16 KiB are asked twice at once.
- Only blocks a read waits for are asked twice. The pieces fetched ahead of a read, or for the file's index, stay with the peer asked.
- On the local test swarm, with a peer sending a block every 100 ms asked first, the endgame brings the first byte from 1.6 seconds to 4 ms.

A peer may drop a request without telling it:

- Peers send the blocks asked in order. A request is taken as dropped when blocks asked after it arrived, and it is still missing 5 seconds later.
- The block is then cancelled and asked again: of another peer that has it, or of the same peer when no other does. So a torrent a single peer serves does not wait for it for good.
- A slow peer is not taken for one that drops: its blocks keep coming, however long the last ones asked wait.
- A peer may send not a byte for four times as long as a block takes it, 5 seconds at least. It may have dropped the last requests, or its connection may stall, as uTP's do on a loaded link.
- That peer is asked for one more block, never asked of it yet, and nothing is asked twice. That block arriving first tells the requests before it were dropped.
- Only when no such block is left, after 30 seconds without a byte, is its oldest request asked again.
- A peer that drops the same block three times is no longer asked for it, and keeps sending the others.
- A peer that sends no block asked for 3 minutes, keep-alives aside, is given up. Its connection is closed, and it is dialed again only 5 minutes later.

## Bounds on what others send

- What peers, trackers, DHT nodes and the proxy send is checked against bounds before Polyfin keeps it.
- A malformed message ends only its own connection, announce or lookup, and the torrent goes on with the others.
- A peer exchange message of more than 8 KiB, or of more than 100 peers added or dropped, is malformed.
- A peer that keeps asking without reading the answers is disconnected once 4 MiB wait for it.

## uTP

uTP (BEP 29) carries the peer protocol over UDP. It yields to the link's other traffic as the delay it adds grows (LEDBAT). Many peers take it, and some take nothing else. Polyfin reaches peers over it beside TCP, over every route, with its own implementation written in Go.

How a peer is reached:

- A peer is tried over uTP first.
- When uTP has not answered within half a second, TCP is tried too, and at once when uTP fails.
- While a torrent has no peer yet, TCP connects at once beside uTP, so that its start waits for nothing.
- Only the connections race. The BitTorrent handshake goes over one at a time: uTP's when it is up, and the other one only if the first fails. A peer drops a second connection that brings the same peer ID.
- A peer that refuses TCP and does not answer uTP holds a connection attempt 3 seconds, after two SYNs.
- Peers with an IPv6 address are reached over TCP.

Its connections:

- A uTP connection that ends tells its peer with a FIN, the torrent's stop included. A peer not told keeps the connection, and may refuse Polyfin's next one from the same address as a duplicate.
- uTP needs a route that carries UDP, like the DHT and UDP trackers. Over a proxy without `UDP ASSOCIATE`, it stays off and peers are reached over TCP alone.
- Each torrent has one UDP socket of the route for uTP, opened at its first uTP connection and closed when it stops. It is apart from the sockets of the DHT and the UDP trackers.
- Other clients share one socket and port for all three, so that peers reach them. Polyfin listens for no peer, so a shared port would bring nothing, and each socket ends with what it serves.
- Through a proxy, that is one more `UDP ASSOCIATE` for each torrent playing.
- A uTP connection whose peer stops answering ends after about 15 seconds of retransmissions. The blocks asked of it go to the other peers.

What peers send over uTP is checked against bounds like the rest:

- A packet's header and its extensions are read within the packet.
- A selective acknowledgment past 32 bytes is ignored.
- Each connection keeps at most 2 MiB received and not yet read, 1 MiB of it out of order.

**System › Health** tells how many of the peers were reached over uTP. With **Detailed log (to diagnose a problem)** on under **Settings › Diagnostics**, the log says over which each peer was reached: `A torrent reached a peer over=uTP`.

### Measured on the local test swarm

On a Mac with an Apple M1 Max, through a SOCKS5 proxy with `UDP ASSOCIATE` on the same machine, 5 runs each (`go test -run '^$' -bench UTPSwarm -benchtime 5x ./internal/torrent`):

- The mixed swarm has 4 seeders over TCP alone, 4 over uTP alone, and 4 addresses where nothing answers.
- The TCP swarm has 4 seeders over TCP alone.
- Peers are counted 3 seconds after the first byte.

| Swarm | Peers reached over | Time to the first byte | Peers reached | Over uTP |
| --- | --- | --- | --- | --- |
| Mixed | TCP alone | 9.6 ms | 4 | 0 |
| Mixed | uTP and TCP | 30.8 ms | 8 | 4 |
| TCP | TCP alone | 8.8 ms | 4 | 0 |
| TCP | uTP and TCP | 31.0 ms | 4 | 0 |

- uTP reaches the peers TCP does not.
- A swarm without uTP starts nearly as fast. Until a torrent has its first peer, TCP connects beside uTP, and its handshake waits for uTP only about as long as its own connection took, 20 ms at least.
- With a half-second head start for uTP from the first peer on, that swarm's first byte would take 510 ms. So the head start delays only the later peers, each reached over TCP half a second later.
- On the loopback, a first byte comes within milliseconds. Over the internet, the round trips of trackers and peers set it.

### Over a path with jitter

A WireGuard tunnel to a far server adds delay, and a delay that varies.

- LEDBAT takes a delay above the lowest of the last two minutes for a queue, and yields to it.
- So that a jitter alone is not taken for a queue, the current delay is the lowest of about the last round trip, as RFC 6817 recommends.
- The two-minute history also follows the drift between the two clocks.

Measured on the same Mac between two network stacks, joined by a path of 50 Mbit/s with 250 ms of queue (`go test -run '^$' -bench Path -benchtime 3x ./internal/torrent/utp`). The delay and the jitter are each way, packets keep their order, and each run moves 4 MiB, 3 runs each. TCP is the WireGuard route's, with selective acknowledgments:

| Path, each way | uTP | TCP |
| --- | --- | --- |
| 40 ms ± 15 ms | 3.41 MB/s | 3.59 MB/s |
| 40 ms ± 15 ms, 1% loss | 0.35 MB/s | 0.33 MB/s |
| 80 ms ± 40 ms | 1.93 MB/s | 2.10 MB/s |
| 80 ms ± 40 ms, 1% loss | 0.16 MB/s | 0.16 MB/s |
