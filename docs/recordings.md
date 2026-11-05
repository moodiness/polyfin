# Recordings

This page is for administrators, and for users who record. It covers recording Live TV programmes: turning recording on, who can record, scheduling, how recordings are made and kept smaller, and watching them. The channels and the guide are on [Live TV](live-tv.md).

Polyfin records Live TV programmes, as Jellyfin's DVR does. Turn on **Record Live TV**, under **Settings › Recordings**.

## Recordings folder

Recordings are written to **Recordings folder**, in **Settings › Recordings**. Empty, recordings go to `recordings` in Polyfin's data folder, which Polyfin creates: `/data/recordings` in the Docker image's volume.

- **Another disk:** mount a folder of the server at `/data/recordings`.
  - With Compose, add `- ./recordings:/data/recordings` under `volumes`, in the `polyfin` service of `compose.yaml`.
  - On Unraid, set the template's **Recordings folder**.
  - A folder mounted elsewhere in the container works too: enter its path in **Recordings folder**.
- **Permissions:** Polyfin must be able to write to the folder, as the container's user, UID 65532. It checks the folder when the setting is saved, and refuses one it cannot write to.
- **Health:** **System › Health** lists a folder Polyfin can no longer write to.
- **Changing the folder:** the recordings already made do not move. They are found again once you move them there.

While recording is off, Polyfin answers as a server that records nothing:

- the recording, timer and series timer lists stay empty;
- what they would hold is not found;
- new timers are refused (400).

A recording that finished or failed can be sent to the targets of the user who scheduled it, and to the server's (see [Notifications](notifications.md)).

## Who can record

**Can record Live TV** lets a user schedule, change and cancel recordings, and delete them. It is at the end of the **Playback and access** section of a user's page, under **Users** (see [Users](users.md)).

- It is on for administrators, and off for other users, by default.
- Seeing recordings needs the **Live TV** permission only.

**Compared with Jellyfin**

- **Can record Live TV** is Jellyfin's `EnableLiveTvManagement`, which apps also read and set in the user's policy.
- Without it, requests are refused with 403, as Jellyfin answers.

## Scheduling

From a programme of the guide, apps make:

- a **timer**, which records that programme;
- a **series timer**, which records every programme of the same title in the guide, on the same channel at the same time of day (within ten minutes). As in Jellyfin, it can record on any channel, or at any time, instead.

Series timers:

- **Days of the week:** a series timer records only on the days it lists, in the server's time zone, where Jellyfin does not look at them. Listing none, or all of them, records every day.
- **"New episodes only":** it leaves out the programmes the guide marks previously shown, and records those it marks new.
  - A programme without either mark is left out when the series timer already recorded the same episode in full.
  - The episode is told by its season and episode numbers, else its episode title, else its description: the first of these both programmes have.
  - A programme with none of them is recorded. So is an episode recorded only in part, such as one whose stream failed for a while.
- **Episodes recorded:** those a series timer recorded in full are kept with it, even once their recordings are deleted, and go with it.
- **Other airings:** once an episode is recorded, the timers the series timer made for its other airings go.
- **Keep up to** deletes a series' oldest recordings past that many.

Timers, series timers and recordings are kept in the database.

API details: [Recordings and timers for apps](jellyfin-api.md#recordings-and-timers-for-apps).

## Recording settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Record Live TV** | **Settings › Recordings** | Off | Turns recording on. |
| **Recordings folder** | **Settings › Recordings** | Empty, `recordings` in the data folder | Where recordings are written (see [Recordings folder](#recordings-folder)). |
| **Start recordings early (minutes)** | **Settings › Recordings** | 0, none, as in Jellyfin (0 to 60) | Starts a recording this many minutes before its programme. Apps can change it per timer. |
| **Keep recording after the end (minutes)** | **Settings › Recordings** | 0, none, as in Jellyfin (0 to 60) | Goes on recording this many minutes after the programme. Apps can change it per timer. |
| **Keep recordings for (days, 0 = forever)** | **Settings › Recordings** | 0, forever (up to 3,650) | Deletes older recordings once a day. |
| **Find commercials in recordings** | **Settings › Recordings** | Off | Reads each recording once it ends, partial ones included, for its commercial breaks, which apps then offer to skip. See [Commercials of recordings](#commercials-of-recordings). |
| **Convert finished recordings to HEVC** | **Settings › Recordings** | Off | Converts finished recordings to HEVC at night, so that they take less space. See [Smaller recordings](#smaller-recordings). |
| **Convert between** | **Settings › Recordings** | 1:00 to 6:00, in the server's time zone | The hours conversions run. A conversion still running at the end stops, and starts over the next night. |
| **Use the processor when there is no graphics card** | **Settings › Recordings** | Off | Converts on the processor, with x265, when no GPU encodes HEVC. Without a GPU and with it off, nothing is converted. |

## How a recording is made

- **Stream:** FFmpeg copies the channel's stream, read as live playback reads it (see [How a channel plays](live-tv.md#how-a-channel-plays)). It uses the headers the stream needs, within its addon's confinement, from the guide of the user who scheduled it.
- **Limits:** a recording counts among that user's 4 channels played through FFmpeg at once, and the server's 16. An IPTV account has a limit of its own (see [Connection limits](live-tv.md#connection-limits)).
  - Past the user's 4, a recording takes the place of their oldest channel watched.
  - A channel watched never takes the place of a recording.
- **Interruptions:** a stream that stops before the end is read again 30 seconds later.
- **While it records:** the file is written in MPEG-TS, which apps can follow live.
- **Once the programme ends:** it becomes one Matroska file in the folder. It plays like a movie's version: as it is, or repackaged into HLS (see [Playback](playback.md)).
- **Polyfin stopping:** a recording under way when Polyfin stops is kept as a partial recording when Polyfin starts again. The rest of the programme, if it still airs, is recorded as another.

## Recordings and tuners

A recording reads its channel's stream from before FFmpeg starts until it ends. So the connection or tuner it takes stays its own between its analysis and FFmpeg's start.

| Source | A recording due |
|---|---|
| HDHomeRun tuner | Takes a tuner no one uses any more, else the tuner of the oldest channel only watched, whoever watches it. That channel stops for its viewers. It never takes another recording's tuner. |
| IPTV account | Keeps the account's rule: it takes a connection no one uses any more, never another user's channel, and is refused when they all play. |

On an HDHomeRun tuner:

- **Viewers:** a viewer's app that asks for the channel again gets `503` while the tuners are in use, as for a channel an IPTV account's connections refuse. The log tells it at INFO, saying the tuners are in use.
  - That holds for an app playing the channel through FFmpeg, as jellyfin-web does.
  - An app that plays the MPEG-TS stream as it is, such as Android TV, gets `502`. The log says `A channel's stream could not be served`, at Warn level.
- **Every tuner recording:** a recording that finds every tuner recording another channel is tried again every 30 seconds, as a stream that fails.
- **Nothing recorded:** if it recorded nothing by its programme's end, it fails. The **Recording failed** notification says every tuner was recording another channel.
- **Not enough tuners:** the admin app's lists of timers, under **Content › Live TV**, show it on the timers of a tuner's channel that would need more tuners than it has. The count includes the other timers recording at the same time.

## Commercials of recordings

With **Find commercials in recordings** on, Polyfin finds the commercial breaks of recordings, which apps then offer to skip. How it reads them, which recordings it reads and how apps get the breaks are on [Commercials of recordings](skip-segments.md#commercials-of-recordings).

## Smaller recordings

A recording copies the channel's stream at the channel's own bitrate. With **Convert finished recordings to HEVC** on, Polyfin converts finished recordings, partial ones included, to HEVC. HEVC takes about half the space of H.264, and a smaller share still of MPEG-2's.

- **Order:** one recording at a time, the one that finished first, between the hours of **Convert between**.
- **GPU:** the one **Graphics card used to convert** chooses, in **Settings › Conversion (transcoding) › Graphics card**. The conversion uses the **HEVC quality (0 = by bitrate)** and the **Deinterlacing method** of the conversion settings (see [Transcoding](transcoding.md#conversion-settings)).
  - With **HEVC quality (0 = by bitrate)** at 0, recordings use 28, Jellyfin's value.
  - Without a GPU that encodes HEVC, recordings are converted on the processor with x265, at its lowest priority. That needs **Use the processor when there is no graphics card** on.
- **Picture:** it keeps its size, in 8 bits, deinterlaced when it is interlaced. The audio and subtitles are copied as they are.
- **Bitrate:** the copy's never goes above the recording's.
- **Conversions at once:** a conversion counts as one of the **Video conversions at once (0 = no limit)**, and waits while playbacks fill them. Playbacks started meanwhile count it too.
- **Disk space:** a conversion starts only with free space on the recordings' disk for a copy as large as the recording. Otherwise it waits.
- **End of the hours:** a conversion still running stops, and its copy is deleted. The recording starts over the next night, as it does when Polyfin stops.

Before the copy replaces the recording, Polyfin checks that:

- its length is within a second of the recording's;
- it holds the same tracks, in the same order;
- its video decodes to its end;
- it is smaller.

Otherwise the copy is deleted, and the recording kept as it was and not tried again.

Some recordings are kept as they are: those in HEVC, AV1 or VP9 already, in HDR or in more than 8 bits, and those ffprobe cannot read.

A recording that cannot be read for a reason that says nothing of it is never kept as it is. It is tried again:

| Cause | What happens |
|---|---|
| The recordings' folder is missing or unreadable, as when its disk is not mounted yet | Conversions stop, and try again a minute later. |
| ffprobe is missing, or out of time | Conversions stop, and try again a minute later. |
| Only the recording's own file cannot be opened | It is passed over, and the next recordings are converted. Once none is left, conversions try it again a minute later. |

Once converted:

- **Playing:** a recording someone is playing is replaced once they stop.
- **Identity:** the recording keeps its identifier, so its played state, resume points and commercials stay. Its file becomes `<identifier>.mkv`, and what playback learned of the old file is learned again.
- **Space saved:** **Content › Live TV**, under **Recordings**, shows how many recordings were converted and the space saved.
- **Apps without HEVC:** they get a converted recording converted when they play it, as any HEVC video (see [Transcoding](transcoding.md#when-polyfin-converts)). With **Conversion (transcoding)** off, they cannot play it.

## Watching recordings

Recordings are listed newest first, and in a **Recordings** folder.

- They are shown to the users who reach the channel they come from, and to the user who scheduled them.
- Parental control and blocked genres apply to recordings as to titles, and to the programmes users schedule.

**Compared with Jellyfin**

- For parental control, Jellyfin judges recorded videos as "Other" items, and scheduled programmes as "LiveTvProgram".
- A recording plays through PlaybackInfo only once finished.
- Jellyfin's keep-until choices are kept, but only **Keep recordings for (days, 0 = forever)** and **Keep up to** delete recordings.
- Jellyfin's `IsPrePaddingRequired` and `IsPostPaddingRequired` are not kept.

API details: [Recordings and timers for apps](jellyfin-api.md#recordings-and-timers-for-apps).
