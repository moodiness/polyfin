# `src/ui`: the « Salon » design system

The admin app with the calm of a TV interface and the density of a desktop tool: clear type, soft rounded tiles with no borders, and one strong, friendly focus. What is under the keyboard, the mouse or the remote is **lit**: a button or a field turns pale lavender-white with dark words, and a row or a tile stays dark with a thin edge of the accent. A setting is edited in its own row, with its explanation always shown under its name.

Import everything from `@/ui`. Each component documents its props in its file; `/admin/dev/ui` (development builds only) shows them all in their states. Colors, radii and type come from the tokens in `src/index.css`, under the « Nuit » names with Salon values (`bg-s1`, `text-ink-2`, `rounded-row`, `text-h1`, `shadow-lift`…): never write a raw color. Icons come from `@phosphor-icons/react`, regular weight.

## The look

- **Sizes are in rem, with one density knob.** Compact and Large are one layout: the tokens and the components hold its sizes, and the root scales it. The root is 16 px on a phone and in Large, and 85 % of that in Compact from 48rem (768 px) up. The page's frame stays 1440 px at most, margins included (`max-w-page`, in px), so a denser desktop shows more of the page. Every size the file quotes is in design pixels at 16 px to the rem: a phone and Large show it as is, Compact at 85 % (a 38 px field is 32 px there, a 34 px page title 29 px). Write classes in rem (`h-9.5`, `text-[1.0625rem]`, `rounded-[0.5rem]`); keep px for the page's frame, hairlines of 2 px or less, shadows, blurs and the breakpoints of media queries. Sizes set from code go through `rem(px)` (an icon's `size`, an inline style), and code that places or measures in pixels uses `scaledPx(px)`.
- **The interface size turns that knob.** A user's **Interface size** in My account is Compact, the 85 % above, or Large, 16 px to the rem on every screen: the same layout 18 % bigger. A phone shows both alike. `<html data-interface-size>` holds it, set by `interfaceSize.ts` before the first frame. Sizes in rem follow both, so a page never asks which one is on.
- **Surfaces are fills, never boxes:** `bg-s1` for tiles, rows and panels, `bg-s2` for hover, fields and the current section. No hairlines, and no card inside a card: rows, figures, tables and panels inside a `Panel` drop their own fill.
- **Radii:** art tiles `rounded-tile` (22), panels `rounded-panel` (18), rows `rounded-row` (14), buttons `rounded-control` (10), side by side choices `rounded-choice` (10), wells `rounded-field` (10).
- **Type, Figtree,** in design pixels (Compact at 85 %): `text-display` 40 (a greeting), `text-h1` 34 (a page title), `text-h2` 24 (a section or shelf title), `text-group` 18 (a group of rows, a panel's title), `text-h3` 17.5 (a row label, a setting's name), `text-area` 17 (the top row's words), `text-lead` 16.5 (values, ledes), `text-body` 16.5, `text-control` 16.5 (buttons and fields), `text-small` 15.5 (help, captions), `text-micro` 14 (times, counts). A phone has its own steps (`index.css`). `figures` gives tabular figures.
- **The lit state of a control** (a button, a choice, a field being typed in): the `lit` utility (inside it, ink, surfaces and state colors switch to their lit variants by themselves), `lift` (the rise over a soft shadow, `--lift` sets how far) and `focus-lit` (both on `:focus-visible`, in place of an outline ring). Every control of `@/ui` wears it.
- **The lit state of a row or a tile** (a setting row, a row of a list, the chosen source, the current section, a figure, a Health tile, an art tile): never white. The `lit-row` utility keeps its dark fill, one step up at most (`--lit-fill`, `s2` by default), its white words at their size, and draws a 2 px edge of `lit-edge`, the accent a step brighter; `focus-lit-row` wears it on `:focus-visible`. Nothing inside it switches color, and nothing rises. An art tile wears the same edge as an outline.
- **Either is never the only cue:** keep `aria-current`, `aria-checked`, `aria-selected`, `aria-expanded`.
- **Motion:** one spring, `ease-nuit`, on transform, background and color only; `prefers-reduced-motion` removes the motion and the lift.
- **Floating surfaces** (menus, lists, dialogs, tooltips, toasts, the unsaved dock) are `raised`: a glass over the page with `shadow-pop`.

## Page structure

| Need                                                                                          | Use                                                                       |
| --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| The frame of a page: title, lede, actions, sub-area words, blocks 40 px apart (34 in Compact) | `PageLayout` (in `@/app/PageLayout`), which uses `PageHeader`             |
| A greeting at the top of Home                                                                 | `PageLayout` with `greeting`                                              |
| A page split with a list of sections on the left (Settings, a user)                           | `PageLayout` with `nav={<SectionNav … />}`; the dock docks under the list |
| A split page opening on a band (a user, a group, My account)                                  | `PageLayout` with `head={<Band … />}` and `nav`                           |
| A shelf of a page, with a count or a link on the right                                        | `Block` (+ `Count`, `TextLink`)                                           |
| Content that needs a ground of its own: a detail, a figure with its rows                      | `Panel`, split by `PanelSection`, closed by `PanelFooter`                 |
| The sub-area words under a page title (Sources · Libraries · Live TV)                         | `Tabs` with links (`words`, the default), passed as PageHeader's `nav`    |
| Sections inside a page part (a source's Summary, Categories…)                                 | `Tabs` with `variant="underline"`                                         |

## Rows, tiles and data

| Need                                                                     | Use                                                                                                                                                                  |
| ------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Things with a label, a second line and a value (sources, devices, state) | `RowList` + `Row` (`boxed` tiles, `separate` beside a detail, `plain` for a feed), 60 px a row; controls of a linked row go in `actions` (a source's switch and ↑ ↓) |
| Long lists (logs, channels, devices)                                     | `RowList compact`: 48 px rows                                                                                                                                        |
| A setting: switch, choice, number with unit, text, path, secret          | `SettingRow`, in a `grid grid-cols-[minmax(0,1fr)] gap-1.5`                                                                                                          |
| The icon or letters at the start of a row, or a tinted state bubble      | `IconTile` (`sm`, `md`, `lg`, `xl` for a band; `ok`, `warn`, `danger`), or `Avatar` for a person                                                                     |
| What is playing, on its poster                                           | `ArtShelf` + `ArtTile`: the lit tile opens wide with the accent edge and its tools; `fit="contain"` for logos, `fallback` when there is no poster                    |
| A problem to look at, with its cause and fix                             | `AlertTile`                                                                                                                                                          |
| A big number ("4 of 4")                                                  | `FigureTile`, side by side in a grid                                                                                                                                 |
| Columns of comparable values                                             | `Table` (`compact` for long ones; values right; scrolls inside itself)                                                                                               |
| How far something is                                                     | `ProgressBar` (`playback` for what plays)                                                                                                                            |
| A time ("18 min ago"), with the full date on hover                       | `RelativeTime` (`className` for its color and size)                                                                                                                  |
| Reordering an item of a list from the keyboard                           | `MoveButtons` (focus follows the item after a move)                                                                                                                  |

### Setting rows

`SettingRow` is edited in the row itself, in both sizes: the name on the left, its help under it as a muted caption (two lines at most, with More for the rest, and always in `aria-describedby`), and the control on the right, which wraps under the name when the row is too narrow. Nothing opens below the row. ↑ ↓ move to the previous or next row's control (`data-setting-head`), except in a number well, where they step, and in an open list; a click on the name or the caption focuses the control.

| `kind`   | In the row                                                                                                                                    | Its own props                                           |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| `switch` | The switch alone toggles (`role="switch"`, with its word); a click elsewhere on the row focuses it                                            | `checked`, `onChange`, `stateText`                      |
| `choice` | Up to 4 short choices side by side (`role="radiogroup"`, ← → choose); more, or longer, a `Select` that opens in one click, searchable past 12 | `value`, `options`, `onChange`                          |
| `number` | A number well with its unit, typed in place, ↑ ↓ step                                                                                         | `value`, `min`, `max`, `step`, `unit`, `format`         |
| `text`   | A text well: each keystroke is a change for the dock, no Enter needed                                                                         | `value`, `onChange`, `placeholder`, `maxLength`, `mono` |
| `path`   | The same, wider, in tabular figures                                                                                                           | as `text`                                               |
| `secret` | The `SecretField` under the name (masked typing, reveal, replace, remove)                                                                     | the SecretField's own                                   |

Every kind takes `saved`: a row whose value differs keeps a dot, "was …", and its value in the link color. A row without `saved` (settings that apply at once) keeps no mark. `anchor` gives it the id of its `#anchor`, `error` shows under it, and `children` add a note under it. Rules and lists (version order, label rules, Home rows, allowed hours) keep their editors beside the rows; a list of items to tick is a two-column checklist of `Checkbox`es.

`OpenRow` draws the same head and caption for a row that no `kind` covers: with `children`, Enter, Space or a click opens them inside the row (a date with its buttons, a list to choose from) and Escape closes them; with `onClick`, the row is a button for an action shown elsewhere (a dialog).

## States

| Need                                                 | Use                                                                 |
| ---------------------------------------------------- | ------------------------------------------------------------------- |
| Loading                                              | `Skeleton`, `SkeletonText`, `SkeletonRows`, shaped like the content |
| Nothing yet                                          | `EmptyState`, saying how to fill it, with the action                |
| A load failed                                        | `InlineError` with `onRetry`                                        |
| A state of a thing (active, incomplete, unreachable) | `StatusPill`: always an icon and a word                             |
| A kind or a flag beside a name (Stremio, Default)    | `Badge`                                                             |
| A note, a warning or a tip inside a page             | `Notice`                                                            |
| "Saved", or an action whose result is elsewhere      | `useToast()`; never `window.alert`                                  |

## Forms

| Need                                      | Use                                                                                    |
| ----------------------------------------- | -------------------------------------------------------------------------------------- |
| Any labelled control, with help and error | `Field` around the control                                                             |
| Text, a filter, a password                | `TextInput` (`icon`, `suffix`, `revealable`; `size` `md` 38 px, `sm` 34, `compact` 32) |
| A number with a unit                      | `NumberInput`                                                                          |
| One choice among many                     | `Select`; two to four side by side: `Segmented`                                        |
| Free text with suggestions (a genre)      | `SuggestInput`                                                                         |
| Several lines                             | `Textarea` (`mono` for code)                                                           |
| Something that turns on or off            | `Switch` (the word of its state beside it), or a `switch` SettingRow                   |
| Picking items of a list                   | `Checkbox`                                                                             |
| An API key, a client secret               | `SecretField` (masked typing, reveal, replace, remove)                                 |
| Saving a whole page of settings           | `SaveBar`, inside the `<form>`, once per page: the unsaved dock                        |

`SaveBar` is the unsaved dock: "2 changes" (pass `changes`), Discard changes and Save. It shows only while there are changes. On a split page it docks under the section list (`DockSlotContext`, which `PageLayout` provides), elsewhere it sticks to the end of the form, and on a phone it is a bar along the bottom with room kept under the last row: it never covers a row for good. Its Save button submits its form, even from the dock.

## Actions

| Need                                                | Use                                                                |
| --------------------------------------------------- | ------------------------------------------------------------------ |
| An action                                           | `Button`: `primary` (one per view), `secondary`, `ghost`, `danger` |
| A link that looks like a button                     | `ButtonLink`, or `ExternalButtonLink` outside the app              |
| An icon only (more actions, delete in a row)        | `IconButton`, whose `label` is required                            |
| Several actions behind one button, the account menu | `Menu` with `MenuItem`, `MenuChoice`, `MenuSeparator`              |
| Asking before a destructive action                  | `ConfirmDialog`                                                    |
| A form or editor too large for its row, phone menu  | `Modal`: a centered floating panel, `width` per use (480 to 640)   |
| A hint on hover or focus                            | `Tooltip` (never the only copy of something needed)                |
| A keyboard shortcut                                 | `Kbd` (`modKey` is ⌘ or Ctrl)                                      |
| A quiet link with a chevron ("Open Health ›")       | `TextLink`                                                         |

## Rules

- Copy goes through `useI18n()`; the components' own words are in `t.ui`.
- No browser-drawn list: `Select` and `SuggestInput` draw their own popover listbox, portaled out of tables, scroll boxes and dialogs, with the keyboard of a native list (arrows, Home, End, Enter, Escape, Tab, typing a letter). A `Select` with more than 12 options starts with a search, accents and case ignored. Never use `<select>` or `<datalist>`.
- Nothing that floats over the page covers the top row: the list of a `Select` or `SuggestInput` stays under the element marked `data-top-bar` and closes when its field scrolls out of view under it; a `Menu` scrolls beneath it.
- Nothing is wider than a phone (390 px, in English and German): a row of actions wraps, labels wrap before values do, and a row that scrolls sideways shows it with the `scroll-fade` class and `useScrollFade`, which fade the edge it scrolls past (`Tabs` and `SectionNav` already do). A name or title that may be one long word, as a German compound, wears the `hyphenate` class: it breaks with a hyphen where the page's language allows, and anywhere as a last resort (`Row`, `PageHeader`, `Panel` and `ArtTile` titles already do).
- Every control has a label; toggles set `aria-pressed` or `aria-checked`, disclosures `aria-expanded`. Arrows may move within a list, never trap Tab.
- Posters and art load lazily (`loading="lazy"`); nothing heavy on every page.
- Add a component here only when two areas need it; otherwise keep it in `src/features/<area>/`.
