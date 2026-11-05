# Reverse proxy

This page is for administrators who serve Polyfin over HTTPS through a reverse proxy, such as Traefik, Caddy or nginx, at an address like `https://media.example.org`. It covers the headers to pass, WebSockets, HTTPS, **Known proxies** and the public address.

Polyfin listens on plain HTTP, on port 8096 by default (see [Configuration](configuration.md)).

- The proxy terminates HTTPS and forwards each request to that port.
- Apps and browsers on the local network can keep connecting to port 8096 directly.

Polyfin can also serve HTTPS itself, on port 8920 by default, with no proxy (see [HTTPS](https-and-networking.md#https)). It takes certificate files or a certificate from Let's Encrypt, and needs one port forwarded on the router. A proxy is still the way when:

- Polyfin shares port 443 with other services under several names;
- the certificate is a wildcard, or for names that resolve only on the local network, and another tool should not write its files where Polyfin reads them;
- the proxy also adds authentication, rate limits or a CDN in front.

## Headers to pass

The proxy passes these headers on to Polyfin:

| Header | What Polyfin does with it |
|---|---|
| `Host`, or `X-Forwarded-Host` | Starts the stream addresses Polyfin gives apps, so that they use the address the app reached. Keep the original `Host`, or pass it as `X-Forwarded-Host`. |
| `X-Forwarded-Proto` | `https` makes those stream addresses start with `https://`, and marks the admin app's session cookie secure. |
| `X-Forwarded-For` | Names the client, when the request comes from a known proxy (see [Known proxies](#known-proxies)). |
| `X-Real-IP` | Names the client when `X-Forwarded-For` is missing, from a known proxy too. |

By default, Traefik and Caddy keep `Host`, and send `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host`.

- Traefik also sends `X-Real-Ip`.
- With nginx, set them with `proxy_set_header`.

## WebSockets

Jellyfin apps, the web player among them, keep a live connection open at `/socket`. It tells them of changes, and carries remote control and Watch together (SyncPlay in Jellyfin).

The proxy must pass the WebSocket upgrade, with the `Upgrade` and `Connection` headers:

- Traefik and Caddy do so by default.
- nginx needs `proxy_http_version 1.1` and the two headers set.
- nginx also closes a connection idle for longer than its `proxy_read_timeout`.

The admin app uses no WebSocket: it asks the server again every few seconds.

## Slow answers

A torrent version's first play over a slow swarm can keep the app's PlaybackInfo request waiting for up to 3 minutes (see [Torrents](torrents.md#deadlines)). The request sends nothing until it answers.

| Proxy | How long it waits for an answer |
|---|---|
| Traefik, Caddy | As long as it takes, by default |
| nginx | 60 seconds without a byte (`proxy_read_timeout`), then it closes the response |

With nginx, set `proxy_read_timeout` to `180s` or more for Polyfin, or such a play fails at the proxy.

## HTTPS and the public address

Get the certificate on the proxy, and have it redirect HTTP to HTTPS. Leave **Serve HTTPS**, in **Settings › Security**, off: the proxy forwards to Polyfin's HTTP port.

If Polyfin's own HTTPS is on behind the proxy anyway, with **Redirect HTTP to HTTPS**, requests are redirected as follows:

| Request | Redirected |
|---|---|
| From a known proxy that sends `X-Forwarded-Proto: https` | Never: the proxy received it over HTTPS. |
| From a known proxy without that header, for a client on the internet | Yes, which may send the client where it cannot go (see below). |
| From a proxy on the local network missing from **Known proxies** | Never: every client looks local. |

A redirected request goes to `https://` and the host it asked for, at one of these ports:

- the port of the `https://` **Public address**, or 443 when it names none;
- without an `https://` **Public address**, Polyfin's HTTPS port, which clients on the internet may not reach.

Have the proxy pass `X-Forwarded-Proto` to avoid this.

Then set **Public address**, in **Settings › Notifications**, to the address people open Polyfin at, such as `https://media.example.org`. Polyfin drops a trailing slash when it saves the address.

- Links in notifications and invite links start with it.
- Apps do not need it: stream addresses come from the headers above.

## Known proxies

Without a known proxy, Polyfin takes each request to come from its connection's address. Behind a proxy, that is the proxy's address for everyone, so:

- failed sign-ins count for everyone together. Ten failures in 15 minutes, from anyone, refuse every user's sign-in for up to 15 minutes, in apps, the admin app, invite links and password reset PINs;
- **Now playing**, on **Home**, and each user's devices show the proxy's address;
- when the proxy is on the local network, every client counts as on it. `/System/Endpoint` tells apps so, and a forgotten password asked from the internet makes a PIN.

List the proxy under **Known proxies**, in **Settings › Security**, one entry per line, up to 64 entries:

| Entry | Example |
|---|---|
| An IPv4 or IPv6 address | `172.30.0.2` |
| A network | `172.30.0.0/24` |
| A host name | `traefik` (see [Proxies by name](#proxies-by-name)) |

A request from a known proxy is then taken to come from the client the proxy names:

- in `X-Forwarded-For`, the rightmost address that is not a known proxy, read across every line of the header;
- without that header, the address in `X-Real-IP`.

Requests from any other address keep their connection's address. Their `X-Forwarded-For` and `X-Real-IP` are ignored, since anyone can send them.

List only proxies: a client connecting from a known address could name any address it likes.

DLNA's addresses under `/dlna/` serve the local network only (see [DLNA](dlna.md)). They refuse:

- any request that carries `X-Forwarded-For`, `X-Real-IP` or `Forwarded` from an address that is not a known proxy;
- any request that names its client in `Forwarded` alone, even from a known proxy, since Polyfin reads only `X-Forwarded-For` and `X-Real-IP`.

**System › Health** warns when an address on the local network sends `X-Forwarded-For` or `X-Real-IP` without being a known proxy. That is how a proxy missing from the list shows up.

- The warning names that address, for an hour after its last such request.
- Addresses on the internet are never named, since a client can send the headers itself.

### Proxies by name

A host name, such as a container's name on a Docker network, is looked up with the system's resolver:

- when the settings are saved;
- when Polyfin starts;
- then every minute.

Every address the name resolves to is a known proxy, until a lookup no longer returns it.

- When the proxy gets another address, the old one stops being trusted at the next lookup, within a minute.
- A name removed from the list stops being trusted at once.
- A name that resolves to nothing, or whose lookup fails, trusts nothing.

While names are listed, an address on the local network that is not a known proxy and sends `X-Forwarded-For` or `X-Real-IP` sets off a new lookup at once. This happens at most once every 10 seconds.

- A proxy that starts after Polyfin, as after a reboot, or comes back with another address, is thus trusted within seconds rather than a minute.
- The lookup runs in the background: the request that sets it off still counts as coming from its connection's address.
- The next requests count as coming from the client the proxy names, once the proxy's address is found.

**System › Health** shows what each name resolves to, under **Known proxies**, and warns of a name that resolves to nothing.

A name is only as trusted as the DNS that answers for it: whoever controls that answer chooses the addresses Polyfin trusts.

- List only names your own network resolves, such as the containers' names Docker's own DNS answers on a network you made.
- Never list a name a public DNS server answers.

Names are written in letters, digits, hyphens, underscores and dots. Polyfin keeps them in lower case.

API details: [Known proxies in the admin API](jellyfin-api.md#known-proxies-in-the-admin-api).

### A proxy in a Docker network

When the proxy and Polyfin are containers on the same Docker network, Polyfin sees the proxy's address on that network, such as `172.30.0.2`, not the host's.

- A container's address can change when the containers start in another order. List the proxy's container name instead (see [the example](#example-with-traefik-and-docker-labels)).
- Docker's own DNS answers that name on a network the containers share, one you declare rather than the default `bridge` network.
- Listing the network's subnet, fixed in Compose, works too, but then every container on that network is trusted.

The proxy may run on the host, or use the host's network, and reach Polyfin through its published port. Polyfin then usually sees the address of the Docker network's gateway, such as `172.18.0.1`. **System › Health** names the address it sees.

### A CDN or a second proxy in front

With a CDN or a second proxy in front, the proxy next to Polyfin receives the requests from that one, and adds its address to `X-Forwarded-For`. Polyfin then takes that address for the client, and every client counts as the CDN or the second proxy again.

| In front | What to do |
|---|---|
| A second proxy you run | Add its address to **Known proxies** too. Have the proxy next to Polyfin keep the `X-Forwarded-For` it receives from it. |
| A CDN | Have the proxy next to Polyfin trust the CDN's published address ranges, so that it keeps the client the CDN names and drops the header from anyone else. Listing the CDN's ranges under **Known proxies** works too, while they fit in 64 entries. |

<a id="example-traefik-with-docker-labels"></a>

## Example with Traefik and Docker labels

This adds Traefik to the `compose.yaml` of [Installation](installation.md), with certificates from Let's Encrypt.

- Traefik's container is named `traefik`.
- Polyfin joins Traefik's `proxy` network, and keeps its port for the local network.

```yaml
services:
  traefik:
    image: traefik:v3.5
    container_name: traefik
    command:
      - --providers.docker=true
      - --providers.docker.exposedbydefault=false
      - --providers.docker.network=polyfin_proxy
      - --entrypoints.web.address=:80
      - --entrypoints.web.http.redirections.entrypoint.to=websecure
      - --entrypoints.web.http.redirections.entrypoint.scheme=https
      - --entrypoints.websecure.address=:443
      - --certificatesresolvers.le.acme.tlschallenge=true
      - --certificatesresolvers.le.acme.email=you@example.org
      - --certificatesresolvers.le.acme.storage=/letsencrypt/acme.json
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - letsencrypt:/letsencrypt
    networks:
      - proxy
    restart: unless-stopped

  polyfin:
    # As in compose.yaml, plus:
    labels:
      - traefik.enable=true
      - traefik.http.routers.polyfin.rule=Host(`media.example.org`)
      - traefik.http.routers.polyfin.entrypoints=websecure
      - traefik.http.routers.polyfin.tls.certresolver=le
      - traefik.http.services.polyfin.loadbalancer.server.port=8096
    networks:
      - default
      - proxy

networks:
  proxy:

volumes:
  letsencrypt:
```

Compose names the `proxy` network `polyfin_proxy`, after the project's name, `polyfin`. Point `media.example.org` at the server, open ports 80 and 443, and start the stack. Then:

1. In **Settings › Security**, add `traefik` to **Known proxies**. **System › Health** then shows the address it resolves to on the `proxy` network.
2. In **Settings › Notifications**, set **Public address** to `https://media.example.org`.

Traefik passes WebSockets, and sets `X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host` and `X-Real-Ip` by itself.

In apps, the server's address is `https://media.example.org`.

- Polyfin also answers apps under `https://media.example.org/jellyfin`, as at the root. A proxy that already serves Jellyfin at `/jellyfin` can forward that path to Polyfin as it is (see [Compatible apps](jellyfin-compatibility.md#compatible-apps)).
- The admin app stays at the root only.

Traefik routes to Polyfin only once its container is healthy, which it becomes within seconds of starting (see [Behind Traefik](installation.md#behind-traefik)).
