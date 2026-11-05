# Sign-in and security

This page covers sign-in and account protection: two-step sign-in, an identity provider, and server-wide security settings. Users read the first part; administrators read all of it.

## Two-step sign-in

Each user can add a second step after their password: a 6-digit code from an authenticator app on their phone, such as their phone's password manager. Once it is on, the password alone signs in nowhere.

### Turning it on

Under **My account › Two-step sign-in**, **Turn on** shows a new key as a QR code and as text, with a link opening an authenticator app on the same device.

- Polyfin draws the QR code itself: the key goes to no other service.
- Scanning it adds the account to the app, named Polyfin.
- It is on only once you enter a code the app shows. An unconfirmed key signs in no one; **Turn on** again makes a new one.

Then come ten recovery codes, shown this once, like `7k2m-q9xd-4tbh-z0ce`. Each signs in once in place of an app code, should the phone be lost.

- A code is 16 letters and digits in four groups, without i, l, o or u, which look like other characters.
- Any case is accepted, with or without dashes, or with spaces in their place.
- Polyfin keeps only their hashes and can never show them again: keep them in a password manager, or printed.
- The section counts the codes left. **New recovery codes** makes ten new ones, voiding the former ones.

**Turn off** and **New recovery codes** first ask for an app code, a recovery code, or the password.

### Signing in with a code

In the admin app, after a right password, the sign-in page asks for the code, or a recovery code in the same field.

- No session exists until it is entered. The step belongs to the browser that entered the password, and ends after 5 minutes: the password is then asked again.
- The activity log records each sign-in with a recovery code.
- The app's code changes every 30 seconds. Polyfin also accepts those of the 30 seconds before and after, for a phone clock a little off.
- Each code is accepted once: entered again, even within its 30 seconds, it is refused.
- Wrong codes count as wrong passwords toward the client's failed attempts (429 once too many). They also count toward the account's, whatever the client, with the same limit.
- A right password between wrong codes does not reset them. They do not count toward **Block an account after this many wrong passwords (0 = never)**.
- Each code counts toward the limit on arrival, before checking, as passwords, PINs and tokens do. Of codes sent at once, those past the limit are refused unchecked.
- The key is encrypted with `POLYFIN_SECRET_KEY`, like other stored keys, and never shown again once set up (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).

Turning it on or off keeps open sessions, in the admin app and Jellyfin apps. Sign out devices you no longer trust under **My account › Devices**.

### Jellyfin apps

Jellyfin apps, the web player included, cannot ask for a code. An account with two-step sign-in signs in to them:

- with Quick Connect, approved on the admin app's **Quick Connect** page, which asked for the code (see [Signing in to apps with Quick Connect](jellyfin-compatibility.md#signing-in-to-apps-with-quick-connect));
- or through an identity provider in the web player (see [Signing in with an identity provider](#signing-in-with-an-identity-provider)).

The password alone is refused there as a wrong password. Administrators' API keys work as before. For such an account, the web player opens no admin app session unless it signed in through the identity provider: the admin app asks for the password and code (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

<a id="silo-s-apps"></a><a id="silos-apps"></a>

### Apps by Silo

Silo's apps cannot ask for a code either.

- Silo's TV apps sign in with their own code, also a Quick Connect request. An account with two-step sign-in approves it on the admin app's **Quick Connect** page, which `/activate` leads to (see [Signing a TV in with a code](silo-apps.md#signing-a-tv-in-with-a-code)).
- Silo's phone, tablet and Mac apps take a password only, refused for such an account as a wrong password.

### For administrators

**Require two-step sign-in for administrators**, under **Settings › Security**, off by default, makes the second step a must for administrators:

- one without it sees the QR code right after their password, and gets a session and recovery codes once an app code confirms it;
- their password alone signs in to no Jellyfin app, set up or not: Quick Connect does;
- they cannot turn theirs off under **My account**;
- open sessions stay until they sign out.

A user who lost their phone and recovery codes asks an administrator. **Turn off two-step sign-in**, on their page under **Users**, turns it off after a confirmation.

- The activity log records who turned it off for whom.
- The user then signs in with their password alone, and sets it up again from **My account**, or right after their password if required.
- An administrator turns their own off from **My account**.

A forgotten-password reset by PIN from a Jellyfin app changes the password only: two-step sign-in stays on (see [Forgotten password](metrics-and-api-keys.md#forgotten-password)).

A database restored under another `POLYFIN_SECRET_KEY` holds keys the current key cannot read. Their users still sign in with a recovery code, and **My account** says so. An administrator can turn theirs off, and **System › Health** names them (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).

API details: [Two-step sign-in](jellyfin-api.md#two-step-sign-in-for-apps).

<a id="identity-provider"></a>

## Identity provider settings

People can sign in to the admin app and the web player with an OpenID Connect provider, set in the **Identity provider** part of **Settings › Security**. Changes apply at once, without a restart. [Signing in with an identity provider](#signing-in-with-an-identity-provider) covers the sign-in.

To set it up:

1. Set **Public address**, in **Settings › Notifications**, to the address people open Polyfin at: the sign-in starts and ends there.
2. At the provider, register Polyfin as a client, confidential or public, with the authorization code flow and the **Redirect URI** the part shows, `<public address>/admin/api/provider/callback`, exactly.
3. Fill in **Issuer** and **Client ID**, and **Client secret** for a confidential client, then save. The sign-in pages show their button once the issuer, client ID and public address are set.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Issuer** | **Settings › Security** | Empty, off | The provider's issuer, exactly as its discovery document gives it, `http://` or `https://`, without a query or fragment. Polyfin reads `<issuer>/.well-known/openid-configuration`. `http://` is accepted for a provider on your own network, but see the warning below. |
| **Button name** | **Settings › Security** | Empty | What the buttons say after "Sign in with", up to 64 bytes. Empty shows the issuer's host name. |
| **Client ID** | **Settings › Security** | Empty, off | The ID the provider gave Polyfin's client. |
| **Client secret** | **Settings › Security** | Empty | A confidential client's secret, sent with HTTP Basic authentication, or in the form when the provider lists only `client_secret_post`. Kept encrypted with `POLYFIN_SECRET_KEY`, and never shown again. |
| **Claim naming the user** | **Settings › Security** | `preferred_username` | The ID token's claim compared with account names the first time someone signs in; the verified email comes next. |
| **Create accounts** | **Settings › Security** | Off | Creates an account, signing in through the provider only, for someone no account matches. Off, they are refused. |
| **Administrators' claim**, **Administrators' value** | **Settings › Security** | Empty | Both set, someone whose claim is the value, or a list holding it, becomes an administrator on sign-in. With the claim `groups`, Polyfin asks for the `groups` scope when the provider lists it. |

**Use HTTPS.** Polyfin accepts an `http://` issuer, as self-hosted providers often sit on the local network. Over HTTP, though, the client secret, codes and tokens cross the network unencrypted, between Polyfin and the provider, and between browsers and both. Anyone on the way can read them and sign in as their owners. Give the provider an `https://` address, and serve Polyfin's public address over HTTPS too, whenever the sign-in leaves a network you fully trust.

Polyfin asks for the scopes `openid`, `profile` and `email`, the last two when the provider lists them or lists no scope.

- A provider unreachable when someone starts to sign in is logged as a warning, with its issuer, and the browser comes back with an error.
- A refused ID token is logged with the check it failed, never with the token.

API details: [Identity provider settings](jellyfin-api.md#identity-provider-settings-in-the-admin-api).

## Signing in with an identity provider

Once an administrator sets up a provider (see [Identity provider](#identity-provider-settings)), the admin app and web player sign-in pages get a **Sign in with …** button named after it. It leads to the provider's page, which asks for its password and own second step, then back to Polyfin, signed in:

- in the admin app, the button sits under **Sign in**. The browser returns to the admin app's home, with a session lasting as a password one does;
- in the web player, Polyfin's script adds the button first on jellyfin-web's sign-in page. The browser returns signed in as its own device, as with a password: it shows under **My account › Devices**, and signing out signs it out.

The sign-in starts and ends at the **Public address** (**Settings › Notifications**), which the button needs. A web player opened at another address comes back at the public one.

### Which account signs in

The first time someone signs in through the provider, Polyfin looks for:

1. the account named by the **Claim naming the user** claim (`preferred_username` by default), ignoring case;
2. else the account named by their email, only if the ID token says it is verified (`email_verified` true). An email the provider does not vouch for, or says nothing of, matches no account;
3. else, with **Create accounts** on, a new account named after the claim, or the email when the claim is no valid name. It has the permissions of an account created under **Users** without a group, and **Identity provider only** set. With **Create accounts** off, the default, the sign-in is refused.

The account is then linked by the provider's issuer and the person's subject (`sub`), which never change there.

- Later sign-ins find it by that link, even if the claim or email changed.
- An account linked to someone else at the provider, matched by name or email, is refused.
- Changing the issuer under **Settings › Security** starts the links over.

With **Administrators' claim** and **Administrators' value** set, someone whose claim holds the value, such as a `groups` claim listing `polyfin-admins`, becomes an administrator on sign-in, unless the account is disabled. The group only grants: removing someone from it at the provider does not demote them. They stay an administrator until you turn **Administrator** off on their page under **Users**.

As with a password, these are refused (see [Connections from outside the local network](user-limits.md#connections-from-outside-the-local-network)):

- a disabled or ended account;
- a sign-in outside the account's allowed hours, or from outside the local network without remote access.

### Accounts without a password

**Identity provider only**, on a user's page under **Users** and when creating one, limits the account to the provider.

- Its password, even the right one, is refused as a wrong password, in the admin app and Jellyfin apps.
- It counts toward the client's failed attempts, not the account's, so nobody can block it with passwords.
- An account created so needs no password: Polyfin gives it one nobody knows.
- Turning it off lets the password sign in again: set one first.

### Two-step sign-in and the provider

Polyfin skips its own two-step code after the provider, which asked for its own second step, as Jellyfin's SSO plugins leave it to the provider.

- An administrator required to use two-step sign-in signs in through the provider without setting it up.
- A web player signed in through the provider also opens the admin app without a code, unlike one signed in with Quick Connect (see [Signing in to the admin app](web-client.md#signing-in-to-the-admin-app)).

Likewise, the block after wrong passwords blocks passwords only (see [Blocking after wrong passwords](#blocking-after-wrong-passwords)). An account blocked by someone guessing its password still signs in through the provider, which checked its own credentials, and stays blocked for passwords.

<a id="jellyfin-apps-1"></a>

### Jellyfin apps and the provider

Jellyfin apps cannot open the provider's page. They use Quick Connect: the app shows a code, approved on the admin app's **Quick Connect** page or the web player's own, opened through the provider (see [Signing in to apps with Quick Connect](jellyfin-compatibility.md#signing-in-to-apps-with-quick-connect)). An account with a password may also type it in the app, unless it is provider only.

### How the sign-in is kept safe

- Polyfin uses the authorization code flow with PKCE (S256), a state and a nonce, each 32 random bytes.
- The state also goes in a browser cookie, and is taken once, within 10 minutes. A replayed, made up or expired state, or one brought back by another browser, is refused.
- The ID token must be signed with RS256 or ES256 by a key of the provider's JWKS, from the issuer set, exactly, for the client ID set, with the sign-in's nonce.
- Its expiry must not have passed nor its issue time be in the future, with 2 minutes allowed either way for clock drift.
- A token naming a key the JWKS lacks makes Polyfin refetch the keys, at most once every 30 seconds.
- The discovery document must name the issuer set, exactly. Polyfin keeps it an hour, and the keys until a token needs another.
- The redirect URI is built from the public address, and must be registered at the provider exactly as **Settings › Security** shows it.
- An `http://` issuer is accepted for a local network provider, but sends the client secret, codes and tokens unencrypted: use HTTPS for the provider and public address (see [Identity provider](#identity-provider-settings)).
- Each return from the provider counts toward the client's failed attempts as a password sign-in: a refused state, code or token counts as a wrong password. Once too many, the browser comes back with "Too many attempts".
- Someone whose account is refused counts as no wrong guess.
- No token, code, state or secret is logged or sent back.
- The browser gets its device's Jellyfin access token, as a password sign-in gives it, through a one-time ticket valid 2 minutes, in a cookie the page cannot read.
- The client secret is stored encrypted with `POLYFIN_SECRET_KEY` (see [Stored keys and tokens](configuration.md#stored-keys-and-tokens)).
- The activity log records sign-ins, refused ones with the name the provider gave, and accounts created.

**Compared with Jellyfin**

- Jellyfin has no OpenID Connect sign-in of its own: plugins add it, with their own sign-in page. Polyfin's script adds the button to jellyfin-web's sign-in page rather than redirecting `/web/` to the provider. jellyfin-web keeps its sign-in in the browser, unseen by the server, so a redirect would also send there people already signed in, or wanting their password or Quick Connect.
- Polyfin finds accounts by provider link, then name, then email, and makes administrators from a group. It never takes administrators back, so a provider's change cannot leave the server without one.

API details: [Identity provider sign-in](jellyfin-api.md#identity-provider-sign-in-for-apps).

## Security settings

<a id="known-proxies"></a>

**Settings › Security** holds the eight settings below, the identity provider and HTTPS (see [HTTPS](https-and-networking.md#https)). **Settings › Diagnostics** holds a ninth. All apply at once, without a restart.

| Setting | Where | Default | What it does |
|---|---|---|---|
| **Allow users' own addons** | **Settings › Security** | On | Lets users add their own addons under **My sources**. |
| **Users may import their watch history from another server or app** | **Settings › Security** | On | Shows under **My account** the imports from a Jellyfin, Emby or Plex server, a Letterboxd export and an app (see [Importing your own watch history](moving-to-polyfin.md#importing-your-own-watch-history) and [Importing from an app](moving-to-polyfin.md#importing-from-an-app)). |
| **Allow share links** | **Settings › Security** | Off | Lets users with **Can share titles** give a movie or episode to someone without an account, through an expiring link (see [Share links](invite-and-share-links.md#share-links)). Off, every link is refused but kept. |
| **Import connected accounts every** | **Settings › Security** | 6 h (or 15 min, 30 min, 1 h, 2 h, 3 h, 12 h, 24 h) | How often apps users stay connected to, and watch histories from tracking services, are imported again (see [How often connected accounts are imported](moving-to-polyfin.md#how-often-connected-accounts-are-imported)). |
| **Block an account after this many wrong passwords (0 = never)** | **Settings › Security** | 0, never (or 3 to 20) | Blocks an account for 15 minutes after that many wrong passwords in a row. |
| **Require two-step sign-in for administrators** | **Settings › Security** | Off | Administrators must enter an authenticator app code after their password (see [Two-step sign-in](#two-step-sign-in)). |
| **Sign out devices unused for (days, 0 = never)** | **Settings › Security** | 0, never (or 1 to 365 days) | Signs out Jellyfin apps unused for that many days. |
| **Known proxies** | **Settings › Security** | None (up to 64 addresses, networks or host names) | Reverse proxies whose `X-Forwarded-For` or `X-Real-IP` header names their requests' client (see [Known proxies](reverse-proxy.md#known-proxies)). |
| **Identity provider** | **Settings › Security** | None | An OpenID Connect provider for signing in to the admin app and web player (see [Signing in with an identity provider](#signing-in-with-an-identity-provider) and [Identity provider](#identity-provider-settings)). |
| **Detailed log (to diagnose a problem)** | **Settings › Diagnostics** | Off | Logs at the `debug` level until turned off (see [Detailed log](configuration.md#detailed-log)). |

### Users' own addons

Under **My sources**, users set up their own addons' catalogs as administrators do under **Content › Libraries**: which are libraries, their order, names, images, genres and maximums, and whether the web player's top bar shows them. See [The order of libraries](addons-and-libraries.md#the-order-of-libraries), [Library images](artwork-and-editing.md#library-images), [Genre and maximum](addons-and-libraries.md#genre-and-maximum) and [Libraries hidden from the top bar](addons-and-libraries.md#libraries-hidden-from-the-top-bar). Their image addresses must be public: only administrators can use local network addresses.

A server source limited to some groups and users keeps others from it (see [Who uses a server addon](addons.md#who-uses-a-server-addon)). A user allowed their own addons can still add the same addon under **My sources**, at a public address. To prevent it, turn **Can add their own addons** off for them or their group.

Users also make their own smart lists under **My sources**, with **Add a smart list**, as administrators make the server's (see [Smart lists](smart-lists-and-search.md#smart-lists)).

- A user's list may draw from their own addons' catalogs, and from the server's while they use the server's sources.
- Only they see it, as one of their libraries or in the Collections view.
- Rules on what was played, started or marked favorite follow what they did, in their lists as in the server's.

The **Access** section of a user's page under **Users** also holds **Can add their own addons**, on by default. While it or **Allow users' own addons** is off:

- the user's own addons are kept but not used;
- their apps show only the server's addons, libraries and smart lists. Their own smart lists are kept but hidden, and cannot be made or changed;
- **My sources** says why;
- their addons cannot be added, replaced, refreshed or turned on (403 `personal_addons_disabled`).

The addons come back once both are on. See [Addons and libraries](addons-and-libraries.md).

### Blocking after wrong passwords

After that many wrong passwords in a row, from a Jellyfin app or the admin app, the account refuses every sign-in for 15 minutes. The right password then gets a wrong password's answer.

The count restarts after:

- a sign-in;
- a new password set by an administrator;
- **Unblock** on the user's page under **Users**, which shows until when the account is blocked;
- setting the limit to 0.

Each block is told once to notification targets that chose **Account blocked** (see [Blocked accounts](notifications.md#blocked-accounts)):

- the server's and administrators' own targets get a link to the user's page with **Unblock**;
- the user's own targets get until when, and the last attempt's address and app.

Attempts from a client refused for a while after too many failed sign-ins from its address (429) never reach the account and are not told: they come constantly from all over the internet. A sign-in from a device the account never used is told to the user (see [Sign-ins on new devices](notifications.md#sign-ins-on-new-devices)).

**Compared with Jellyfin**

- Jellyfin disables the account until an administrator enables it. Polyfin only blocks it for a while, so the last administrator can never be locked out for good.

API details: [Wrong password blocking](jellyfin-api.md#wrong-password-blocking-for-apps).

### Signing out unused devices

Polyfin signs out Jellyfin apps unused for that many days, at startup and hourly. It closes their connections and removes them from SyncPlay groups, as any sign-out does. Admin app sessions are not affected.
