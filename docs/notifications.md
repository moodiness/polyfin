# Notifications

Polyfin can tell users and administrators when something they care about happens: a new episode of a series they follow, a recording that finished or failed, a problem **System › Health** found or that was solved. Messages go to targets: a generic webhook, a Discord channel's webhook, an ntfy topic, an email address, a Telegram chat, a Gotify server or a Pushover user.

## Targets

There are two kinds of owners:

- **Settings › Notifications**, for administrators: the server's targets. They receive the events of every user, and the health events.
- **My account › Notifications**, for every user: their own targets. They receive that user's new episodes and recordings. An administrator's own targets may receive the health events too.

**Add a target** asks for its kind, a **Name**, the **Events** it receives, and:

- for a **Webhook**, its address, to which Polyfin posts each event as JSON (see [The webhook event](#the-webhook-event));
- for **Discord**, the channel webhook's address (in Discord: the channel's settings, Integrations, Webhooks, Copy Webhook URL);
- for **ntfy**, the **ntfy server** (`https://ntfy.sh` when left empty), the **Topic**, and an **Access token** for a topic that needs signing in;
- for **Email**, the **Email address** messages go to, through the server's SMTP server (see [Email](#email)); without one, the kind cannot be added, and the form says why;
- for **Telegram**, the **Chat**, a chat's number such as `-1001234567890` or a public channel's `@name`, and the **Bot token** BotFather gave when the bot was created; the bot must be a member of the chat;
- for **Gotify**, the **Gotify server**'s address and an **Application token** (in Gotify: Apps, Create Application);
- for **Pushover**, the **User key** (or a group key) and the **Application token** of an application created on Pushover.

Webhook and Discord addresses, ntfy access tokens, Telegram bot tokens, Gotify application tokens, and Pushover user keys and application tokens are secrets: they are stored encrypted with `POLYFIN_SECRET_KEY` (see [stored keys and tokens](configuration.md#stored-keys-and-tokens)), never shown again (the list shows a webhook's or Discord address's host only), and never written to the log. A secret that cannot be decrypted shows the target as **Enter it again**, and on **System › Health** under **Stored keys**. An email target's address, a Telegram chat and a Gotify server are shown as they are.

A user who is not an administrator may only add targets on public addresses, as their own addons: an address on the local network is refused, and messages never reach one.

Each target shows its state:

| State | Meaning |
|---|---|
| **Working** | The last message was delivered. |
| **Nothing sent yet** | No message was sent to it yet. |
| **Refused** | It answered 401, 403, 404 or 410: its address or token is wrong, or it was deleted. Telegram refusing an unknown chat, Pushover an unknown user key or application token, and an SMTP server refusing the user and password, the sender or the recipient, count too. |
| **Message refused** | It refused the last message with another 4xx status. |
| **Not reached** | Recent messages could not be delivered: no answer, server errors, or for email no SMTP server. |
| **Off** | **Send messages to this target** is off. |

**Send a test** sends a test message at once and tells whether the target accepted it. A delivered message, a test included, clears the target's problem. The list shows the HTTP status the target answered with its problem, or for email the SMTP server's code, such as `SMTP 535`.

### Email

Email targets go through one SMTP server, which an administrator sets under **Settings › Notifications**, in **Email**. Email targets can be added once an **SMTP server** and a **Sender address** are saved.

Polyfin connects to the server with the **SMTP encryption** chosen:

- **STARTTLS** connects without encryption, then asks the server to encrypt: when it does not offer to, nothing is sent;
- **TLS from the start** encrypts the connection from the start;
- **None** sends everything as it is. The password is then only sent to a server on this machine (`localhost`).

The server's certificate must name its host. Polyfin signs in with the **SMTP user** and **SMTP password**, by `PLAIN`, or `LOGIN` when the server offers only that, once the connection is encrypted; with no user, it sends without signing in. The password is stored encrypted with `POLYFIN_SECRET_KEY` and never shown again.

### Settings

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Public address** | **Settings › Notifications** | empty | The address people open Polyfin at, such as `https://media.example.org`. Links in messages start with it: an episode or a recording opens in the [web client](web-client.md), a health problem on **System › Health**. Empty, messages carry no link. |
| **SMTP server** | **Settings › Notifications** | empty | The host name of the SMTP server email targets go through, such as `smtp.example.org`. Empty, email targets cannot be added. |
| **SMTP port** | **Settings › Notifications** | 587 | Usually 587 with STARTTLS, 465 with TLS from the start. |
| **SMTP encryption** | **Settings › Notifications** | STARTTLS | STARTTLS, TLS from the start, or None (see [Email](#email)). |
| **SMTP user** | **Settings › Notifications** | empty | The user Polyfin signs in with. Empty, it sends without signing in. |
| **SMTP password** | **Settings › Notifications** | empty | Its password, stored encrypted. |
| **Sender address** | **Settings › Notifications** | empty | The address messages come from, such as `polyfin@example.org`. Email targets need it. |
| **Sender name** | **Settings › Notifications** | empty | The name shown with the sender address. Empty, the server name is. |

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

Nothing is sent while a request is answered. Each target has its own queue, sent in order in the background, at most four requests at once. Each request, or each email from connecting to the SMTP server to its last answer, waits 10 seconds at most. After a network error, a timeout, a 5xx answer, or a 4xx answer from an SMTP server (busy, or too many messages), the message is tried again after 30 seconds, then twice as long each time, up to 10 minutes; a 429 answer waits as long as its `Retry-After` header (or the `retry_after` of Discord, or of Telegram's `parameters`) asks. A message not delivered within an hour is dropped, and the target shows **Not reached**; it shows so after three failures in a row already. A 401, 403, 404 or 410, or a refusal as in **Refused** above, drops the message and shows **Refused** at once. Messages still waiting when Polyfin stops are dropped; at most 100 wait for one target.

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

## Messages for each kind

Failed recordings and health problems that are errors are urgent: ntfy, Gotify and Pushover show them with a higher priority.

A Discord target receives one embed: the event's title, its message as the description, its link, a color by event (blue for new episodes, green for finished recordings and solved problems, red for failures and errors, amber for warnings), the time, and the server name in the footer, sent as `Polyfin`. Messages never mention anyone (`allowed_mentions` is empty).

An ntfy target receives a JSON publication to its server's root address, with `topic`, `title`, `message`, `tags` (`tv` for new episodes, `red_circle` for finished recordings, with `warning` when partial, `x` for failures, `warning` for problems, `white_check_mark` for solved ones, `bell` for tests), a high `priority` (4) for urgent events, and `click`, the event's link. The access token, if any, is sent as `Authorization: Bearer`.

An email target receives a `multipart/alternative` message, in plain text and in HTML, in the server language:

- the subject is the event's title;
- the body is its message, its link (named "Open" in HTML), and "Sent by Polyfin from" the server name;
- the headers are `From` (the sender name and address), `To`, `Subject`, `Date` (when the event happened), `Message-ID` (the same for each try of a message, at the sender address's domain), `Auto-Submitted: auto-generated`, so that mail servers send no automatic reply, and `X-Polyfin-Event: <type>`.

A Telegram target receives the Bot API's `sendMessage` (`POST https://api.telegram.org/bot<token>/sendMessage`), with `chat_id`, and `text` in HTML (`parse_mode` is `HTML`): the event's title in bold, its message, and its link named "Open", without a preview of the page.

A Gotify target receives a `POST` to its server's `/message`, with the application token as the `X-Gotify-Key` header, and `title`, `message`, a `priority` of 8 for urgent events and 5 for the others, and the event's link as the address a click opens (`extras` `client::notification` `click` `url`).

A Pushover target receives a `POST` to `https://api.pushover.net/1/messages.json`, with `token`, `user`, `title`, `message`, `url` (the event's link), a `priority` of 1 for urgent events and 0 for the others, and `timestamp`, when the event happened.

## For app developers

The admin API serves the targets: the server's to administrators at `/admin/api/notifications`, and each user's own at `/admin/api/account/notifications`.

- `GET` lists the `targets`, the `events` and `kinds` they may choose from, and `emailAvailable`, whether the SMTP server and sender address are set.
- `POST …/targets` adds a target, `PATCH …/targets/{id}` changes one (fields left out keep their values), `DELETE …/targets/{id}` deletes one, and `POST …/targets/{id}/test` sends a test message, answering `delivered`, the `status` (an HTTP status, or an SMTP code for email), and the target.
- A target has `kind` (`webhook`, `discord`, `ntfy`, `email`, `telegram`, `gotify` or `pushover`), `name`, `events`, `enabled`, and:
  - `address`: a webhook's or Discord target's scheme and host, an ntfy or Gotify target's server, an email target's recipient, empty for Telegram and Pushover;
  - `topic`: an ntfy target's topic;
  - `chat`: a Telegram target's chat;
  - `tokenSet`: whether an ntfy, Telegram, Gotify or Pushover target has a token.

  It never answers a secret address, token or key.
- To add or change one, send `address` (a webhook's or Discord target's address, an ntfy or Gotify target's server, an email target's recipient), `topic`, `chat`, `token` (an ntfy target's access token, empty for none, a Telegram bot's token, or a Gotify or Pushover application's token) and `userKey` (a Pushover target's user key).
- Errors:
  - `email_unavailable` (409): an email target without an SMTP server;
  - `invalid_email_address`, `invalid_chat`, `invalid_token`, `invalid_user_key` (400): a malformed field;
  - `target_unreadable` (409): a Pushover target whose key and token cannot be decrypted, changed with only one of them.
- `GET /admin/api/settings` answers `smtpHost`, `smtpPort`, `smtpSecurity` (`starttls`, `tls` or `none`), `smtpUser`, `smtpPasswordSet`, `smtpFrom` and `smtpFromName`. `PUT` takes them and `smtpPassword`, which is never answered: left out, it is kept; empty, it is removed. The errors are `invalid_smtp_host`, `invalid_smtp_port`, `invalid_smtp_security`, `invalid_smtp_account` and `invalid_smtp_sender`.

## Compared with Jellyfin

Jellyfin sends notifications through its webhook plugin, which administrators install and configure with templates for each destination, and whose events are Jellyfin's own (item added, playback, users, tasks). Polyfin builds the messages itself, and:

- its events are those of Polyfin: a new episode of a followed series (there is no library scan to tell of added items), recordings, and Health problems;
- each user chooses their own targets and events under **My account**, besides the server's;
- the webhook format is one documented, versioned JSON event rather than templates; Discord, ntfy, email, Telegram, Gotify and Pushover get messages made for them;
- addresses, tokens, keys and the SMTP password are stored encrypted, and failing targets are shown.

Jellyfin apps see none of this: nothing of it is part of Jellyfin's API.
