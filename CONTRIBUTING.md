# Contributing to Polyfin

Thank you for helping. Each kind of contribution has its place:

- **A question or help with a setup:** ask in [Q&A](https://github.com/moodiness/polyfin/discussions/categories/q-a), after searching it and the [documentation](https://moodiness.github.io/polyfin/docs/).
- **An idea:** post it in [Ideas](https://github.com/moodiness/polyfin/discussions/categories/ideas), or vote for an existing one. The ideas that are picked become issues on the [roadmap](https://github.com/users/moodiness/projects/1).
- **A bug:** use the [bug report form](https://github.com/moodiness/polyfin/issues/new?template=bug_report.yml).
- **A vulnerability:** follow the [security policy](SECURITY.md), never a public issue.
- **A fix to the documentation:** use **Edit this page on GitHub** at the bottom of any page of the [website](https://moodiness.github.io/polyfin/docs/).

## Changing the code or the documentation

1. For anything beyond a small fix, open or comment on an issue first, so that the change is agreed before you write it.
2. Run Polyfin from source and its tests as [Development](docs/development.md) describes. `make check` runs the formatting checks, vet and the tests.
3. Keep the documentation in `docs/` in step with the behavior you change.
4. Open a pull request against `main`. Its title follows [Conventional Commits](https://www.conventionalcommits.org/), such as `feat: ...` or `fix(addons): ...`: pull requests are squash-merged, and the title becomes the commit message.

## Writing the documentation

The documentation in `docs/` is read on GitHub and on the [website](https://moodiness.github.io/polyfin/docs/). `docs/README.md` is its index and the website's sidebar: each `##` section is a group, and each table row a link. A page's title (its `#` heading) is the same as its row's link text. `npm --prefix docs run check` tests the rules below that a script can, and CI runs it on every pull request that changes the documentation.

### Sentences

- One fact per sentence: aim for 15 to 20 words, 30 at most.
- Lead with what the user does or sees. Conditions and exceptions come after, as bullets.
- Use a table when two or more things are compared on the same points: the choices of a setting, routes, file kinds, error cases. Numbers go in tables, not in chains of prose.
- A section about settings starts with a table: Setting, Where, Default, What it does.
- American spelling, and the words the UI uses.
- No release history ("since 0.58", "before 0.60"): the release notes hold it.

### Words

- No em or en dashes (`—`, `–`). Use a colon, a comma, parentheses or two sentences, and "1 to 5" for ranges.
- "movie", never `film`.
- "repackage" for changing a stream's container without converting it. `remux` appears only where the docs quote the UI, the API, a metric or a log line, in bold or as code: "**Remux** (repackaged)". RemuxDB keeps its name, as the admin app's settings use it.
- Never name sites that host or index releases, RemuxDB aside. Never say `debrid`: "provider" says the same.

| Say | For | Not |
|---|---|---|
| web player | the jellyfin-web app Polyfin serves; "jellyfin-web" only for the upstream project or its code | web client |
| admin app | Polyfin's own administration app; "dashboard" only for jellyfin-web's | admin interface |
| series | the item; "show folder" only for local folder naming | TV show |
| programme guide | the guide, as the UI says; XMLTV only for the format | EPG, Live TV guide |
| skip buttons | the feature; "segments" for the data | skip markers |
| **System › Health** | the Health page; **System › Health › Transcoder** for a part of it | Health, the transcoder of Health |
| Watch together | the feature, with "(SyncPlay in Jellyfin)" once | SyncPlay |
| picture subtitles | subtitles drawn as pictures, as the UI says | image subtitles |
| GPU | in prose; **Graphics card** where the UI says so | graphics card |
| Polyfin Desktop | the desktop app | desktop version |
| Silo's apps | Silo's native apps | |
| watch state | what a user watched, and where they stopped | watched state |

### Settings paths

- `**Settings › Playback**` always means the admin app. The path names navigation only: a section, and a group at most. The setting's label follows in bold, exactly as the UI writes it, units included: **Order versions by**, in **Settings › Playback › Version order**.
- A setting named for the first time on a page comes with its path. A bare **Settings** is never enough: write **Settings › General**.
- The other admin app pages take their full path too: **Content › Sources**, **Content › Libraries**, **System › Health**, **My account › Statistics**.
- The web player's own settings say so right before the path: the web player's **Settings › Home**, or "in the web player, open **Settings › Home**". Notes on jellyfin-web's code may write jellyfin-web's **Settings › Home**.
- Other sites get plain words, without bold or arrows: on Trakt, under Settings, Connected apps.
- The sections of a user's page are not navigation: the **Access** section of the user's page.
- The arrow is `›`, never `→`.

### Compared with Jellyfin, and For app developers

Each `##` section holds the user's prose first, then at most one **Compared with Jellyfin** block, then at most one **For app developers** block, in this shape:

```md
**Compared with Jellyfin**

- Polyfin keeps the version menu in its order. Jellyfin 12.4 moves the version an app opens to the top.

<details>
<summary>For app developers</summary>

- `MediaSourceId` equal to the item's own ID picks no version.

</details>
```

- **Compared with Jellyfin** is a bold label, then bullets: never a heading, never mid-paragraph. Each bullet starts with what differs for the user, and ends with the link to Jellyfin's source. Other comparisons, such as with Silo or Plex, take the same shape.
- **For app developers** is a `<details>` block, collapsed on GitHub and on the website. Keep the blank lines around the Markdown inside it, or neither renders it.

### Headings and anchors

Release notes, issues and discussions link to sections of the docs, in their GitHub form (`blob/main/docs/users.md#…`) and in their website form (`docs/users#…`). GitHub and the website make a heading's anchor differently: "A title's page" is `#a-titles-page` on GitHub and `#a-title-s-page` on the website.

- Headings are noun phrases ("Version order"). "How to …" fits task pages only.
- No apostrophes, slashes, colons or periods in headings, so that both anchors are the same: "Theme per user", not "Each user's theme".
- No two headings of a page are the same: the second gets a `-1` anchor, which changes when sections move.
- A heading renamed or removed keeps its old anchors alive, with an alias right above the heading or the text that replaces it, one for each form when they differ:

  ```md
  <a id="how-a-show-s-files-are-numbered"></a><a id="how-a-shows-files-are-numbered"></a>

  ### How show files are numbered
  ```

- Links within the docs point to a heading's current anchor, not to an alias.

Everyone follows the [code of conduct](CODE_OF_CONDUCT.md). Polyfin does not host or distribute content: never share or request content, links to it, addon manifest URLs or credentials.
