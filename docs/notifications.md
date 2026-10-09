# Notifications

Polyfin can tell users and administrators when something they care about happens: a new episode of a series they follow, a recording that finished or failed, a problem **System › Health** found or that was solved. Messages go to targets: a generic webhook, a Discord channel's webhook, or an ntfy topic.

## Targets

There are two kinds of owners:

- **Settings › Notifications**, for administrators: the server's targets. They receive the events of every user, and the health events.
- **My account › Notifications**, for every user: their own targets. They receive that user's new episodes and recordings. An administrator's own targets may receive the health events too.

**Add a target** asks for its kind, a **Name**, the **Events** it receives, and:

- for a **Webhook**, its address, to which Polyfin posts each event as JSON (see [The webhook event](#the-webhook-event));
- for **Discord**, the channel webhook's address (in Discord: the channel's settings, Integrations, Webhooks, Copy Webhook URL);
- for **ntfy**, the **ntfy server** (`https://ntfy.sh` when left empty), the **Topic**, and an **Access token** for a topic that needs signing in.

Webhook and Discord addresses, and ntfy access tokens, are secrets: they are stored encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)), never shown again (the list shows the address's host only), and never written to the log. A secret that cannot be decrypted shows the target as **Enter it again**, and on **System › Health** under **Stored keys**.

A user who is not an administrator may only add targets on public addresses, as their own addons: an address on the local network is refused, and messages never reach one.

Each target shows its state:

| State | Meaning |
|---|---|
| **Working** | The last message was delivered. |
| **Nothing sent yet** | No message was sent to it yet. |
| **Refused** | It answered 401, 403, 404 or 410: its address or token is wrong, or it was deleted. |
| **Message refused** | It refused the last message with another 4xx status. |
| **Not reached** | Recent messages could not be delivered: no answer, or server errors. |
| **Off** | **Send messages to this target** is off. |

**Send a test** sends a test message at once and tells whether the target accepted it. A delivered message, a test included, clears the target's problem.

### Settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Public address** | **Settings › Notifications** | empty | The address people open Polyfin at, such as `https://media.example.org`. Links in messages start with it: an episode or a recording opens in the [web client](web-client.md), a health problem on **System › Health**. Empty, messages carry no link. |

## Events

| Event | `type` | Who receives it |
|---|---|---|
| New episode | `new_episode` | The targets of each user following the series; the server's targets once per episode, whoever follows it. |
| Recording finished | `recording_finished` | The targets of the user who scheduled it, and the server's. |
| Recording failed | `recording_failed` | The same. |
| Health problem found | `health_problem` | The server's targets, and administrators' own. |
| Health problem solved | `health_solved` | The same. |

### New episodes

A user follows the series they played an episode of (the 50 most recently played) and the series they marked favorite, as the Upcoming row of Jellyfin apps shows them. Every two hours, starting five minutes after Polyfin starts, Polyfin looks at the episodes of these series for those that became available. Only the users with a target for new episodes are looked at, or all of them when the server has one. An episode is told once per user; never one the series already had when Polyfin first looked at it (so neither starting to use notifications nor following a series sends old episodes); never one released more than a week ago, which its addon added late; never one without a release date. The series' descriptions are those the library keeps: an old one is asked for again in the background, so a new episode may be told one check later.

### Recordings

A recording that ends is finished, or partial when part of its programme is missing (see [Live TV](live-tv.md#recordings)). One that recorded nothing failed.

### Health problems

Every five minutes, Polyfin looks at the problems **System › Health** shows under **Needs attention**: the database, disk space, addons, IPTV sources, programme guides, failed tasks, backups and stored keys. Conversions at their limit and thumbnails paused for a host come and go with the load: they are not told. A problem is told once two checks in a row found it, and solved once two checks in a row no longer find it, so that a brief failure sends nothing. What was told is kept in the database: a restart neither tells a problem again nor forgets to tell when it is solved.

Messages are written in the server language (**Settings › General**).

## Delivery

Nothing is sent while a request is answered. Each target has its own queue, sent in order in the background, at most four requests at once. Each request waits 10 seconds at most. After a network error, a timeout or a 5xx answer, the message is tried again after 30 seconds, then twice as long each time, up to 10 minutes; a 429 answer waits as long as its `Retry-After` header (or Discord's `retry_after`) asks. A message not delivered within an hour is dropped, and the target shows **Not reached**; it shows so after three failures in a row already. A 401, 403, 404 or 410 drops the message and shows **Refused** at once. Messages still waiting when Polyfin stops are dropped; at most 100 wait for one target.

## The webhook event

A webhook receives each event as JSON in a `POST`, with the headers `Content-Type: application/json`, `User-Agent: Polyfin/<version>` and `X-Polyfin-Event: <type>`. Any 2xx answer counts as delivered.

```json
{
  "version": 1,
  "id": "6f1d3c0a9b7e4f2c8a5d1e3b7c9f0a2d",
  "type": "new_episode",
  "at": "2026-10-09T18:00:00Z",
  "server": { "id": "fedcba9876543210fedcba9876543210", "name": "Home", "url": "https://media.example.org" },
  "title": "New episode of Example Series",
  "message": "S02E05 · The Fifth",
  "url": "https://media.example.org/web/#/details?id=0a1b2c3d4e5f60718293a4b5c6d7e8f9&serverId=fedcba9876543210fedcba9876543210",
  "user": { "id": "11223344556677889900aabbccddeeff", "name": "sam" },
  "episode": {
    "id": "0a1b2c3d4e5f60718293a4b5c6d7e8f9",
    "name": "The Fifth",
    "seriesId": "99887766554433221100ffeeddccbbaa",
    "seriesName": "Example Series",
    "season": 2,
    "number": 5,
    "premiereDate": "2026-10-09T00:00:00Z",
    "providerIds": { "Imdb": "tt0000000" }
  }
}
```

| Field | Type | Meaning |
|---|---|---|
| `version` | number | The version of this format, 1. Fields may be added within a version; a field removed or changed in meaning makes a new version. |
| `id` | string | Unique to the event: a message tried again keeps it, so a webhook can tell a repeat. |
| `type` | string | `new_episode`, `recording_finished`, `recording_failed`, `health_problem`, `health_solved`, or `test` for **Send a test**. |
| `at` | string | When the event happened, RFC 3339 in UTC. |
| `server` | object | `id` as Jellyfin apps know the server, `name`, and `url`, the public address, or null. |
| `title`, `message` | string | The event for people, in the server language. |
| `url` | string or null | Opens what the event is about; null without a public address. |
| `user` | object or null | The user the event is about (`id` as Jellyfin apps know it, `name`); null for health events, and for new episodes sent to the server's targets. |
| `episode` | object | For `new_episode`: the episode's and its series' item IDs, names, season and number, release date, and the episode's provider IDs. |
| `recording` | object | For recordings: `id` (its item; gone once it failed), `name`, `channelId`, `channelName`, the programme's planned `start` and `end`, and `partial`. |
| `problem` | object | For health events: `key`, which names the problem the same way while it lasts, `severity` (`error` or `warning`), `text`, and `since`. |

## Discord and ntfy

A Discord target receives one embed: the event's title, its message as the description, its link, a color by event (blue for new episodes, green for finished recordings and solved problems, red for failures and errors, amber for warnings), the time, and the server name in the footer, sent as `Polyfin`. Messages never mention anyone (`allowed_mentions` is empty).

An ntfy target receives a JSON publication to its server's root address, with `topic`, `title`, `message`, `tags` (`tv` for new episodes, `red_circle` for finished recordings, with `warning` when partial, `x` for failures, `warning` for problems, `white_check_mark` for solved ones, `bell` for tests), a high `priority` (4) for failed recordings and error problems, and `click`, the event's link. The access token, if any, is sent as `Authorization: Bearer`.

## Compared with Jellyfin

Jellyfin sends notifications through its webhook plugin, which administrators install and configure with templates for each destination, and whose events are Jellyfin's own (item added, playback, users, tasks). Polyfin builds the messages itself, and:

- its events are those of Polyfin: a new episode of a followed series (there is no library scan to tell of added items), recordings, and Health problems;
- each user chooses their own targets and events under **My account**, besides the server's;
- the webhook format is one documented, versioned JSON event rather than templates; Discord and ntfy get messages made for them;
- addresses and tokens are stored encrypted, and failing targets are shown.

Jellyfin apps see none of this: nothing of it is part of Jellyfin's API.
