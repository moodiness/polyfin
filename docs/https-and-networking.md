# HTTPS and networking

This page is for administrators. It covers how Jellyfin apps find Polyfin on the local network, and how Polyfin serves HTTPS itself, with certificate files or with Let's Encrypt.

## Local network discovery

Jellyfin and Emby apps list Polyfin among the servers they find on the local network. They find it while **Answer local network discovery** is on, in **Settings › General**.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Answer local network discovery** | **Settings › General** | On | Answers the discovery messages Jellyfin and Emby apps broadcast to UDP port 7359. Off, the port is free. |
| **Address on the local network** | **Settings › General** | Empty | The address announced, such as `http://192.168.1.10:8096`, as Jellyfin's `PublishedServerUrl`. It is also the `LocalAddress` of `/System/Info/Public`. Empty, the address of the interface that reaches the app is announced. |

- **The question:** Jellyfin apps broadcast `who is JellyfinServer?` to UDP port 7359, and Emby apps `who is EmbyServer?`.
- **The answer:** Polyfin's address, ID and name (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)).
- **Who gets an answer:** local network addresses only. These are private IPv4 ranges, IPv6 unique local addresses, link-local addresses and the server itself. A port forwarded from the internet gets no answer.

The address in the answers is **Address on the local network** when it is set. Otherwise it is:

- the address of the server's network interface that reaches the app, with Polyfin's HTTP port;
- or the address `POLYFIN_LISTEN` names, when it names one.

In Docker, the address depends on the container's network: see [Let apps find Polyfin on the local network](installation.md#let-apps-find-polyfin-on-the-local-network).

Both settings apply at once, without a restart. Turned off, Polyfin frees the port. **System › Health** shows two warnings, each linked to these settings:

- **The port is taken:** another program uses it, such as a Jellyfin server on the same host. Polyfin starts anyway and logs it. Turning the setting off ends the warning, and saving the settings tries the port again.
- **The address is the container's:** in a container without an address set, an answer gave an address that is not on the app's network. It is the container's own, on Docker's bridge network, which apps cannot reach.

API details: [Local network discovery](jellyfin-api.md#local-network-discovery-in-the-admin-api).

## HTTPS

Polyfin can serve HTTPS itself, without a reverse proxy, on its own port next to HTTP. Set it in **Settings › Security**, in its **HTTPS** part. Every change applies at once, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Serve HTTPS** | **Settings › Security** | Off | **Certificate files**: a certificate another tool obtains. **Let's Encrypt**: a certificate Polyfin obtains and renews itself. |
| **HTTPS port** | **Settings › Security** | 8920 | The TCP port HTTPS listens on in the container, as Jellyfin's. Publish it, as 8096 is for HTTP (see [Serve HTTPS](installation.md#serve-https)). |
| **Redirect HTTP to HTTPS** | **Settings › Security** | Off | Sends the requests of clients on the internet that come over HTTP to HTTPS (see [The redirect](#the-redirect)). |
| **Certificate**, **Private key** | **Settings › Security**, with **Certificate files** | Empty | The paths in the container of PEM files: the certificate with its chain, such as `fullchain.pem`, and its RSA or ECDSA key, such as `privkey.pem`. |
| **Host names** | **Settings › Security**, with **Let's Encrypt** | Empty | The public names that point at the server, one a line, such as `media.example.org`, up to 100, all in one certificate. |
| **Email** | **Settings › Security**, with **Let's Encrypt** | Empty | Optional: Let's Encrypt may write to it about the account. |
| **I agree to Let's Encrypt's terms of service** | **Settings › Security**, with **Let's Encrypt** | Off | Required: Let's Encrypt issues certificates only once its terms, at [letsencrypt.org/repository](https://letsencrypt.org/repository/), are agreed to. |
| **Challenge** | **Settings › Security**, with **Let's Encrypt** | HTTP-01 | How Let's Encrypt checks that the names point at the server (see [Let's Encrypt](#let-s-encrypt)). |

HTTP keeps `POLYFIN_LISTEN`, and keeps working whatever happens to HTTPS. **System › Health** reports HTTPS problems: a port it cannot listen on, files it cannot read, renewals that fail. None of them stops Polyfin from starting.

The container's user, UID 65532, cannot listen below 1024. To reach Polyfin on the usual ports, forward them on the router, or in the container's `ports`:

| Usual port | Used for | Forward to |
|---|---|---|
| 443 | HTTPS, and Let's Encrypt's TLS-ALPN-01 | the **HTTPS port**, 8920 by default |
| 80 | Let's Encrypt's HTTP-01 | Polyfin's HTTP port, 8096 by default |

When 443 reaches the HTTPS port, set **Public address**, in **Settings › Notifications**, to `https://your-host`, without a port. [The redirect](#the-redirect) then goes to 443 rather than to the HTTPS port.

### Certificate files

Mount the folder of the files read-only in the container, and give their paths. Saving checks two things:

- both files can be read by user 65532;
- the key belongs to the certificate.

Files that are missing, unreadable or mismatched are refused, and nothing is saved. Saving other settings with the same files checks nothing.

Polyfin looks at the files every minute, and reads them again when they change. A certificate another tool renews, such as Certbot, acme.sh or a reverse proxy's, is served within a minute, without a restart. A change may be unreadable, missing or mismatched, for instance between the writes of the certificate and of its key. The last good certificate then stays in use, and **System › Health** warns.

<a id="let-s-encrypt"></a><a id="lets-encrypt"></a>

### Let's Encrypt

Polyfin obtains one certificate for every host name, with an account key of its own. It keeps in the database:

- the account key, the certificate and its private key, the keys encrypted with `POLYFIN_SECRET_KEY` as the other [stored keys](configuration.md#stored-keys-and-tokens), so that backups hold them;
- the challenge and the ACME server the certificate came from.

| Event | What Polyfin does |
|---|---|
| A third of the certificate's lifetime remains (30 days before the end of a 90-day certificate) | Renews it. |
| The host names or the challenge change, or Polyfin starts with another `POLYFIN_ACME_DIRECTORY` | Obtains a new one at once. |
| An attempt fails | Tries again an hour later, or at once when the host names, email or challenge change. The current certificate stays in use meanwhile. |
| Polyfin stops during an attempt | Waits for the attempt to end, so that a certificate obtained at that moment is kept before Polyfin stops, never later, as while a backup is restored. An attempt cut short this way is no failure: it is made again at the next start. |

Each name must point at the server from the internet, and one port must reach Polyfin:

| Challenge | Let's Encrypt connects to | Which must reach |
|---|---|---|
| HTTP-01, the default | `http://<name>/.well-known/acme-challenge/` on port 80 | Polyfin's HTTP port, 8096 by default. Polyfin answers there, and never redirects it. |
| TLS-ALPN-01 | port 443 | Polyfin's HTTPS port, 8920 by default |

Names that resolve only on the local network, and wildcards such as `*.example.org`, need the DNS-01 challenge. Polyfin does not answer DNS-01:

- it would issue certificates without any open port;
- but Polyfin would need to create a TXT record under `_acme-challenge.<name>`;
- that takes each DNS host's own API, with credentials stored encrypted, or RFC 2136 dynamic updates for DNS servers that accept them.

Meanwhile, obtain such a certificate with another tool that answers DNS-01, such as Certbot, acme.sh or lego. Give its files under **Certificate files**.

With **Let's Encrypt**, while **Public address**, in **Settings › Notifications**, is empty, that field offers `https://` and the first host name:

- without a port, as the internet reaches it once 443 is forwarded to the HTTPS port;
- with the HTTPS port, when that port is not 443.

Links in notifications then use HTTPS, and the redirect goes to the port of the address chosen.

**For tests:** `POLYFIN_ACME_DIRECTORY` names another ACME server, such as Let's Encrypt's staging environment, a private ACME server, or a test server such as Pebble. `POLYFIN_ACME_CA_FILE` names the certificate authorities its HTTPS is trusted with (see [Configuration](configuration.md)).

### The redirect

While **Redirect HTTP to HTTPS** is on and HTTPS is served, a request from the internet over HTTP gets a 308. It goes to the same path and query on `https://`, with the host asked for and the port the internet reaches HTTPS at. A 308 keeps the method and body, so sign-ins follow it too.

| **Public address** in **Settings › Notifications** | Port the redirect uses |
|---|---|
| Starts with `https://` and names a port | That port |
| Starts with `https://` and names no port, such as `https://media.example.org` | 443 |
| Anything else | The **HTTPS port** |

With the router's 443 forwarded to the HTTPS port, 8920, set **Public address** to `https://your-host`. Without it, clients are sent to port 8920, which the router does not open.

These are never redirected:

- requests from the local network: private IPv4 ranges, IPv6 unique local addresses, link-local addresses and loopback, as [Local network discovery](#local-network-discovery) reads them, behind a known proxy the client it names;
- Let's Encrypt's HTTP-01 checks, under `/.well-known/acme-challenge/`;
- requests from a known proxy that sends `X-Forwarded-Proto: https`: the proxy received them over HTTPS (see [Reverse proxy](reverse-proxy.md#https-and-the-public-address)).

While HTTPS is not served, its port taken or no certificate read yet, nothing is redirected.

### What apps see

Over HTTPS, apps see the same as behind a proxy that sends `X-Forwarded-Proto: https`:

- the stream addresses Polyfin gives apps start with `https://`;
- the admin app's session cookie is marked secure;
- without **Address on the local network**, `LocalAddress` of `/System/Info/Public` and `/System/Info` stays the address the request reached, `https://` and the HTTPS port included.

Discovery answers keep the HTTP address. A certificate for a public name does not match `192.168.1.10`, and HTTP is never redirected on the local network. To announce HTTPS on the local network, set **Address on the local network** to an `https://` address whose name the certificate holds (see [Finding servers on the local network](jellyfin-compatibility.md#finding-servers-on-the-local-network)).

### HTTPS problems

**System › Health** warns of these, each with a link to the setting:

- a port HTTPS cannot listen on, used by another program or below 1024;
- certificate files that are missing, unreadable or do not belong together;
- an attempt with Let's Encrypt that failed, with Let's Encrypt's error;
- a certificate that expires within 14 days. An expired certificate is an error.

They reach notification targets as health problems (see [Notifications](notifications.md)).

API details: [HTTPS settings](jellyfin-api.md#https-settings-in-the-admin-api).
