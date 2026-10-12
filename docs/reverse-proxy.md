# Reverse proxy

This page explains how to run Polyfin behind a reverse proxy, such as Traefik, Caddy or nginx, which serves it over HTTPS at an address like `https://media.example.org`: the headers to pass, WebSockets, HTTPS, **Known proxies** and the public address.

Polyfin listens on plain HTTP, on port 8096 by default (see [Configuration](configuration.md)). The proxy terminates HTTPS and forwards each request to that port. Apps and browsers on the local network can keep connecting to port 8096 directly.

## Headers to pass

| Header | What Polyfin does with it |
|---|---|
| `Host`, or `X-Forwarded-Host` | Starts the stream addresses Polyfin gives apps, so that they use the address the app reached. Keep the original `Host`, or pass it as `X-Forwarded-Host`. |
| `X-Forwarded-Proto` | `https` makes those stream addresses start with `https://`, and marks the admin app's session cookie secure. |
| `X-Forwarded-For` | Names the client, when the request comes from a known proxy (see [Known proxies](#known-proxies)). |
| `X-Real-IP` | Names the client when `X-Forwarded-For` is missing, from a known proxy too. |

Traefik and Caddy keep `Host` and send `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host` by default; Traefik also sends `X-Real-Ip`. With nginx, set them with `proxy_set_header`.

## WebSockets

Jellyfin apps, the built-in web client among them, keep a live connection open at `/socket`: it tells them of changes, carries remote control and watching together. The proxy must pass the WebSocket upgrade, with the `Upgrade` and `Connection` headers. Traefik and Caddy do so by default; nginx needs `proxy_http_version 1.1` and the two headers set, and closes a connection idle for longer than its `proxy_read_timeout`.

The admin app uses no WebSocket: it asks the server again every few seconds.

## HTTPS and the public address

Get the certificate on the proxy, and have it redirect HTTP to HTTPS. Polyfin itself never serves HTTPS.

Then set **Public address**, under **Settings › Notifications**, to the address people open Polyfin at, such as `https://media.example.org`, without a trailing slash. Links in notifications and invite links start with it. Polyfin does not need it for apps: stream addresses come from the headers above.

## Known proxies

Without a known proxy, Polyfin takes every request to come from the address its connection comes from. Behind a proxy, that is the proxy's for everyone, so:

- failed sign-ins are counted for everyone together: 10 failures in 15 minutes, from anyone, refuse every user's sign-in for up to 15 minutes, in apps, the admin app, invite links and password reset PINs;
- **Sessions**, the playbacks on **Home** and each user's devices show the proxy's address;
- when the proxy is on the local network, every client counts as on it: apps are told so by `/System/Endpoint`, and a forgotten password asked from the internet makes a PIN.

List the proxy under **Known proxies**, in **Settings › Security**, one entry per line, up to 64: an IPv4 or IPv6 address, a network such as `172.30.0.0/24`, or a host name such as `traefik` (see [Proxies by name](#proxies-by-name)). A request from a known proxy is then taken to come from the client its `X-Forwarded-For` header names: the rightmost address there that is not a known proxy, read across every line of the header. Without that header, `X-Real-IP` names the client. Requests from any other address keep their connection's address, and their forwarded headers are ignored, since anyone can send them.

List only proxies: a client connecting from a known address could name any address it likes.

**System › Health** warns when an address on the local network sends `X-Forwarded-For` or `X-Real-IP` without being a known proxy, naming that address, for an hour after its last such request. That is how a proxy missing from the list shows up. Addresses on the internet are never named: a client can send the headers itself.

### Proxies by name

A host name, such as a container's name on a Docker network, is looked up with the system's resolver when the settings are saved, when Polyfin starts, and then every minute. Every address it resolves to is a known proxy, until a lookup no longer returns it: when the proxy gets another address, the old one stops being trusted at the next lookup, within a minute. A name removed from the list stops being trusted at once. A name that resolves to nothing, or whose lookup fails, trusts nothing.

While names are listed, a request with `X-Forwarded-For` or `X-Real-IP` from an address on the local network that is not a known proxy has them looked up again at once, at most once every 10 seconds. A proxy that starts after Polyfin, as after a reboot, or comes back with another address, is thus trusted within seconds rather than a minute. The lookup runs in the background: the request that sets it off is still taken to come from its connection's address, and the next ones from the client the proxy names once its address is found.

**System › Health** shows what each name resolves to, under **Known proxies**, and warns of a name that resolves to nothing.

A name is only as trusted as the DNS that answers for it: whoever controls that answer chooses the addresses Polyfin trusts. List only names your own network resolves, such as the containers' names Docker's own DNS answers on a network you made, never a name a public DNS server answers.

Names are written in letters, digits, hyphens, underscores and dots; Polyfin keeps them in lower case.

**For app developers:** in `/admin/api/settings`, `knownProxies` lists the addresses and networks first, then the names; a `PUT` with an entry that is no address, network or host name, or with more than 64 entries, answers 400 `invalid_known_proxies`. `GET /admin/api/health` lists the names under `proxyNames`, each with its `addresses`, sorted, and `resolvedAt`, and `GET /admin/api/health/problems` warns of one that resolves to nothing as `proxy_name_unresolved`, with `name`.

### A proxy in a Docker network

When the proxy and Polyfin are containers on the same Docker network, Polyfin sees the proxy's address on that network, such as `172.30.0.2`, not the host's. A container's address can change when the containers start in another order, so list the proxy's container name instead (see the example below): Docker's own DNS answers it on a network the containers share, one you declare rather than the default `bridge` network. Listing the network's subnet, fixed in Compose, works too, but then every container on that network is trusted.

When the proxy runs on the host, or uses the host's network, and reaches Polyfin through its published port, Polyfin usually sees the address of the Docker network's gateway, such as `172.18.0.1`. **System › Health** names the address it sees.

### A CDN or a second proxy in front

With a CDN, or a second proxy, in front of the one that reaches Polyfin, the first proxy receives the requests from that one, and adds its address to `X-Forwarded-For`. Polyfin then takes that address for the client, and every client counts as the CDN or the second proxy again:

- a second proxy you run: add its address to **Known proxies** too, and have the proxy next to Polyfin keep the `X-Forwarded-For` it receives from it;
- a CDN: have the proxy next to Polyfin trust the CDN's published address ranges, so that it keeps the client the CDN names and drops the header from anyone else. Listing the CDN's ranges under **Known proxies** works too, while they fit in 64 entries.

## Example: Traefik with Docker labels

This adds Traefik to the `compose.yaml` of [Installation](installation.md), with certificates from Let's Encrypt. Traefik's container is named `traefik`, Polyfin joins its `proxy` network, and keeps its port for the local network.

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

`polyfin_proxy` is the `proxy` network as Compose names it, after the project's name, `polyfin`. Point `media.example.org` at the server, open ports 80 and 443, start the stack, then:

1. Under **Settings › Security**, add `traefik` to **Known proxies**. **System › Health** then shows the address it resolves to on the `proxy` network.
2. Under **Settings › Notifications**, set **Public address** to `https://media.example.org`.

Traefik passes WebSockets and sets `X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host` and `X-Real-Ip` by itself. In apps, the server's address is `https://media.example.org`. Polyfin also answers apps under `https://media.example.org/jellyfin`, as at the root, so a proxy that already serves Jellyfin at `/jellyfin` can forward that path to Polyfin as it is (see [Compatible apps](jellyfin-compatibility.md#compatible-apps)). The admin app stays at the root only.

Traefik routes to Polyfin only once its container is healthy, which it becomes within seconds of starting (see [Behind Traefik](installation.md#behind-traefik)).
