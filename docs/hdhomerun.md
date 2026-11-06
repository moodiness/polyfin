# HDHomeRun tuners

This page is for administrators with an HDHomeRun tuner on the local network, fed by an antenna or a cable. It covers adding the tuner, its channels, and how its tuners are shared. The tuner's free channels play and record as an [IPTV source](iptv.md)'s do. For the programme guide and recordings, see [Live TV](live-tv.md).

## Adding a tuner

Open **Add a source** in **Content › Sources**, then choose **HDHomeRun tuner** under **Kind of source**. Only administrators add tuners, to the server's sources.

- Only an administrator's sources may reach the local network.
- A tuner's tuners are the server's to share.

The form finds a tuner in one of two ways:

| Way | What it does |
|---|---|
| **Find tuners** | Asks the tuners of the local network to answer, with HDHomeRun's discovery: a UDP broadcast on port 65001. It lists those that answer, with their name, model and tuner count. A tuner already added says so. |
| **Add by address** | Takes the tuner's address, such as `192.168.1.20` or `http://192.168.1.20`, and reads its `discover.json` |

- An address with a path keeps it, such as `http://192.168.1.30:9191/hdhr` for a proxy that acts as a tuner. Polyfin reads `discover.json` and `lineup.json` under that path.
- The address of a tuner's `discover.json` or `lineup.json` names the same tuner.
- An address with a login, a query or a fragment is refused.
- To add Dispatcharr as a tuner, see [As an HDHomeRun tuner](iptv.md#as-an-hdhomerun-tuner).

The tuner is added under the name it gives itself, with its Live TV catalog only.

- The same tuner, by its device ID, cannot be added twice, as its tuners would be counted twice.
- Its channels are requested with Polyfin's own user agent.

**Compared with Jellyfin**

- Polyfin keeps the path of a tuner's address, as Jellyfin does ([HdHomerunHost.cs](https://github.com/jellyfin/jellyfin/blob/v12.4/src/Jellyfin.LiveTv/TunerHosts/HdHomerun/HdHomerunHost.cs#L165-L180)).

## Finding tuners with Docker

**Find tuners** broadcasts to UDP port 65001 of the local network. Like the messages of Jellyfin apps (see [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network)), a broadcast crosses no bridge network.

| Container network | **Find tuners** reaches the tuners |
|---|---|
| Docker Desktop, or a bridge network of Docker Engine | No |
| The host's network (`compose.host.yaml`) | Yes |
| A network that gives the container its own address | Yes |

When no tuner answers, the form says so. Two ways work on any network:

- **Add by address** works on any network the container reaches the tuner from.
- `POLYFIN_HDHOMERUN_ADDRESSES` lists the tuners' addresses, such as `192.168.1.20`. **Find tuners** then asks each one directly instead of broadcasting (see [Configuration](configuration.md)).

<a id="a-tuners-channels"></a><a id="a-tuner-s-channels"></a>

## Channels of a tuner

Polyfin reads the channels from the tuner's `lineup.json`, with their number (`GuideNumber`, such as "5.1") and name.

- Channels the tuner marks protected (`DRM`) are left out, as it does not play them.
- Apps show each channel's number as the tuner gives it, and order channels by it: 5, 5.1, 5.2, then 6.
- A fixed number set under **Channels** on the source page replaces the tuner's number.

The source page's **Summary** has a **Tuner** panel, with the tuner's model and tuner count.

| Control | Default | What it does |
|---|---|---|
| **Only the tuner's favorite channels** | Off | Keeps only the channels marked favorite on the tuner |
| **Scan for channels** | | Asks the tuner to scan for channels again, on its first source such as its antenna. It shows the progress, then reads the line-up. |

- The line-up is read again with **Refresh Live TV lists and guides every (hours)**, in **Settings › Live TV**.
- It is kept as an IPTV line-up is, edits included (see [How the line-up is kept](iptv.md#how-the-line-up-is-kept)).
- XMLTV guides attach to its catalog as to any catalog.
- A guide channel named by the channel's number takes that channel (see [How channels are matched to the guide](live-tv.md#how-channels-are-matched-to-the-guide)).
- A tuner's line-up gives no logos. Its channels show their guide channel's icon (see [Logos from the guide](live-tv.md#logos-from-the-guide)).

## Tuners in use

Each channel watched or recorded takes one of the tuner's tuners, whoever reads it. A channel's stream is read once for every viewer.

- With every tuner taken, a channel opens or is refused as under an account's [connection limit](live-tv.md#connection-limits).
- A recording takes the tuner of a channel only watched, never another recording's (see [Recordings and tuners](recordings.md#recordings-and-tuners)).

Streams play as IPTV channels do: as they come when the app takes them, else converted.

- Terrestrial video is often MPEG-2 and interlaced.
- A conversion deinterlaces it with **Deinterlacing method** (see [Transcoding](transcoding.md)).
- The user's **Live TV** permission, quality group and limits apply.

### Tuner problems

| The tuner | What Polyfin shows |
|---|---|
| Does not answer, or refuses a stream for another reason than its tuners being in use | A problem under **System › Health**, until one of its streams plays. This holds whether Polyfin knew the stream already or analyzed it for this play. |
| Refuses a stream because every tuner is in use, by Polyfin or another app | The stream is **Refused by the provider** (see [Stream health](iptv.md#stream-health)). It is no fault of the tuner's. |

- A tuner says every tuner is in use with HDHomeRun's error 805.
- A plain 503 counts as the same. Proxies that act as a tuner answer it once their connections are all in use.
