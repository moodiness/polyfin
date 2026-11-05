# Metrics and API keys

This page is for administrators who connect other tools to Polyfin. It covers Prometheus metrics, API keys for tools such as request managers, and the administration features Jellyfin apps can use.

## Metrics

Polyfin can expose its metrics to Prometheus, in its text format, at `/metrics`. Turn **Expose metrics** on in **Settings › General**. Polyfin then makes the **Metrics token**, shown under it as saved keys are (see [Showing a saved key](administration.md#showing-a-saved-key)). Prometheus sends the token as a bearer token with every request.

| Request to `/metrics` | Answer |
|---|---|
| **Expose metrics** off, the default | 404 |
| Without the token, or with another | 401 |
| With the token | The metrics |

- **Make a new token**, beside the token, asks first, then makes another. The former token stops working at once.
- The activity log records who made a token, never the token.
- The token is stored encrypted with `POLYFIN_SECRET_KEY` when it is set, as the other saved keys are.
- Turned off, metrics keep their token for when they are turned on again.

The metrics name no user, title, address, device or token. Their labels take a few fixed values, so that the series stay few. Addons are named as they are listed under **Content › Sources**, and addons with the same name are counted together.

| Metric | Type | Labels | What it counts |
|---|---|---|---|
| `polyfin_playbacks` | gauge | `kind`: `video`, `live_tv`, `music`, `audiobook`; `delivery`: `direct_play`, `remux`, `conversion` | Playbacks under way, as **Home** shows them. |
| `polyfin_conversions` | gauge | `method`: `nvidia`, `vaapi`, `videotoolbox`, `qsv`, `amf`, `processor` | Conversions under way, of files and channels: those that convert the video. |
| `polyfin_encoding_failures_total` | counter | `method`: as above, or `copy` for a repackaging | Repackagings and conversions FFmpeg failed. |
| `polyfin_segments_total` | counter | `method`: as above | HLS segments made of files. |
| `polyfin_addon_request_duration_seconds` | histogram | `addon`; `resource`: `manifest`, `catalog`, `meta`, `stream`, `subtitles`, `other`; `outcome`: `success`, `failure` | Requests to addons and how long their answers took. An answer, even "not found", is a success, as on **System › Health**. |
| `polyfin_imports_total` | counter | `service`: the app (`stremio`, `nuvio`), the tracking service (`trakt`, `simkl`, …) or `letterboxd`; `outcome` | Imports of app connections, of watch histories and of Letterboxd exports. One that could not read the account whole counts as a failure. |
| `polyfin_tracking_sent_total` | counter | `service`; `outcome` | What was sent to tracking services, such as a title watched, each try counted. |
| `polyfin_notifications_total` | counter | `target`: `webhook`, `discord`, `ntfy`, `email`, `telegram`, `gotify`, `pushover`; `outcome` | Notifications sent, each try counted. |
| `polyfin_http_request_duration_seconds` | histogram | `api`: `jellyfin`, `admin` (the admin app and its API), `web` (the web player), `other` (`/healthz`, `/readyz`, `/metrics`); `status`: `1xx` to `5xx` | HTTP requests and how long they took, a stream's until it ended. Requests under `/jellyfin` count as at the root: the web player's as `web`, the rest as `jellyfin`, the admin app and monitoring routes included, since they answer at the root only. |
| `polyfin_scans_running` | gauge | | Scans of local folders under way. |
| `polyfin_scan_duration_seconds` | histogram | | Scans of local folders and how long they took. |
| `polyfin_database_connections` | gauge | `state`: `in_use`, `idle`, `max` | Connections to PostgreSQL. |
| `polyfin_disk_free_bytes` | gauge | `folder`: `cache`, `conversion`, `reading` (files being read), `recordings`, `backups` | Space free on the disks Polyfin writes to, those in use, as **System › Health** checks them: `conversion` and `reading` when they are other folders than the cache's. |
| `polyfin_bytes_sent_total` | counter | `delivery`: `direct_play`, `remux`, `conversion` | Bytes of media sent to apps, by how they reach them: by the play session's encodings under way, else by the route, a file sent as it is counting as direct play and an HLS segment as repackaged. Answers with an error are left out. |

Prometheus' own metrics of the process follow:

- memory, goroutines and garbage collection (`go_*`);
- on Linux, processor time, memory and open files (`process_*`).

A scrape configuration, with the token in a file Prometheus reads:

```yaml
scrape_configs:
  - job_name: polyfin
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/polyfin-token
    static_configs:
      - targets: ["polyfin:8096"]
```

`credentials: <the token>` puts the token in the configuration itself instead.

`/healthz` and `/readyz` tell whether Polyfin answers and reaches PostgreSQL (see [Health check](installation.md#health-check)).

API details: [Metrics settings](jellyfin-api.md#metrics-settings-in-the-admin-api).

## API keys and integrations

Tools and Jellyfin apps reach Polyfin's administration through the Jellyfin API. This section covers what they can do there.

### API keys

**API keys** let tools such as request managers use the Jellyfin API, as Jellyfin's keys do. Create them under **System › API keys** with **Create a key**, or in jellyfin-web's dashboard. **Revoke** asks first.

- A key has administrator rights and no user.
- Requests that name a user act for that user. That user's parental control, blocked genres, visible libraries and allowed hours apply.
- Requests that name no user see the server's libraries.
- Polyfin stores only a hash of each key, with its app name, creation date and last use. The admin app shows a key once.
- A key cannot open a WebSocket.

API details: [API keys](jellyfin-api.md#api-keys-in-the-jellyfin-api).

### Devices

Administrators get Jellyfin's device list, with custom names. Deleting a device signs out every user on it. It also signs out the admin app sessions taken from their web player there (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

API details: [Device list](jellyfin-api.md#device-list-in-the-jellyfin-api).

### Users from jellyfin-web

Administrators can create, rename and delete users from jellyfin-web, with Polyfin's rules:

- a password of at least 8 characters is required, unlike Jellyfin;
- the last administrator stays.

See [Users](users.md).

### Server configuration

Jellyfin apps can read and save the server configuration.

- **Reading** answers Polyfin's server name, language, resume thresholds, Quick Connect and legacy authorization, with Jellyfin's defaults for the rest.
- **Saving** applies those fields only, and keeps those left out.

API details: [Server configuration](jellyfin-api.md#server-configuration-in-the-jellyfin-api).

### Forgotten password

1. A user on the local network asks for a PIN from the app's sign-in screen.
2. The PIN, valid 30 minutes, is logged and shown on the user's page under **Users**, as there is no server file to read it from.
3. Entering it in the app makes it the user's password and signs them out everywhere.

The PIN resets the password only. An account with [two-step sign-in](sign-in-and-security.md#two-step-sign-in) keeps it, and still signs in to apps with Quick Connect.

- Wrong PINs count as failed sign-ins.
- Asked from outside the local network, the answer is the same and no PIN is made, as in Jellyfin.
- Behind a reverse proxy on the local network, list it under **Known proxies**. Otherwise every request counts as coming from it, and so from the local network (see [Known proxies](reverse-proxy.md#known-proxies)).

### Activity log and logs for apps

Jellyfin apps can read the [activity log](administration.md#activity-log), download the [server log](administration.md#server-logs) and run the [scheduled tasks](administration.md#scheduled-tasks).
