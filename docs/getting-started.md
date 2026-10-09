# Getting started

This page covers what to do the first time you open Polyfin: creating the administrator and the other accounts, signing in from Jellyfin apps, and choosing the language of generated names.

## First run

### Create the administrator

Until an administrator exists, Polyfin prints a one-time setup code in its log. To see it, run:

```sh
docker compose logs polyfin
```

Enter the code on the setup page to create the administrator.

### Create users

Create the other accounts with **Create a user** under **Users**. See [Users](users.md) for more.

Moving from a Jellyfin server? **Import from Jellyfin**, under **Users** too, creates its accounts in Polyfin and brings over what each user watched: played movies and episodes, resume points and favorites. See [Moving from Jellyfin](users.md#moving-from-jellyfin).

### Sign in from Jellyfin apps

Jellyfin apps sign in with these accounts, in one of two ways:

- **Password:** the account's user name and password.
- **Quick Connect:** the app shows a 6-digit code, and a signed-in user approves it on the admin app's **Quick Connect** page.

### Change a password

Users can change their password from Jellyfin apps, or under **My account › Password** in the admin app (**Change password**). Doing so signs their other devices out.

## Language of generated names

Polyfin generates some names that Jellyfin apps show:

- seasons;
- untitled episodes;
- the type that tells apart libraries with the same name, such as "Popular (Movies)".

These names are in English or French. Set the language under **Settings**. It defaults to the language the setup page was in.

## Next steps

- [Addons and libraries](addons-and-libraries.md): add sources and build libraries.
- [Live TV](live-tv.md): watch and record channels.
- [IPTV](iptv.md): add IPTV sources.
- [Users](users.md): manage accounts and access.
- [Web client](web-client.md): use Polyfin in a browser.
