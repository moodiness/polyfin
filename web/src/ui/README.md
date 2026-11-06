# `src/ui`: the « Salon » design system

The admin app with the calm of a TV interface: big type, few things per screen, soft rounded tiles with no borders, and one strong, friendly focus. What is under the keyboard, the mouse or the remote is **lit**: it turns pale lavender-white with dark words, or gets a thick white halo (art tiles). Values sit on the right with a chevron, and help shows for the lit row only.

Import everything from `@/ui`. Each component documents its props in its file; `/admin/dev/ui` (development builds only) shows them all in their states. Colors, radii and type come from the tokens in `src/index.css`, under the « Nuit » names with Salon values (`bg-s1`, `text-ink-2`, `rounded-row`, `text-h1`, `shadow-lift`…): never write a raw color. Icons come from `@phosphor-icons/react`, regular weight.

## The look

- **Sizes are in rem, with one density knob.** The root is 16 px on a phone and 85 % of that from 48rem (768 px) up, so the whole app is 15 % denser on a desktop. The page's frame stays 1440 px at most, margins included (`max-w-page`, in px), so a denser desktop shows more of the page. Every size the file quotes is in design pixels at 16 px to the rem: a phone shows it as is, a desktop at 85 % (a 64 px well is about 54 px there, the 64 px between blocks too). Write classes in rem (`h-16`, `text-[1.3125rem]`, `rounded-[1.5rem]`); keep px for the page's frame, hairlines of 2 px or less, shadows, blurs and the breakpoints of media queries. Sizes set from code go through `rem(px)` (an icon's `size`, an inline style), and code that places or measures in pixels uses `scaledPx(px)`.
- **Surfaces are fills, never boxes:** `bg-s1` for tiles, rows and panels, `bg-s2` for hover, fields and the current section. No hairlines, and no card inside a card: rows, figures, tables and panels inside a `Panel` drop their own fill.
- **Radii:** art tiles `rounded-tile` (34), panels `rounded-panel` (28), rows `rounded-row` (24), buttons `rounded-control` (17), choices `rounded-choice` (18), wells `rounded-field` (20).
- **Type, Figtree:** `text-display` 56 (a greeting), `text-h1` 48 (a page title), `text-h2` 27 (a shelf title), `text-group` 25 (a group of rows), `text-h3` 21 (a row label), `text-lead` 20 (values, ledes), `text-body` 19, `text-control` 18, `text-small` 17 (help), `text-micro` 15.5 (captions, times). `figures` gives tabular figures.
- **The lit state:** the `lit` utility (inside it, ink, surfaces and state colors switch to their lit variants by themselves), `lift` (the rise over a soft shadow, `--lift` sets how far) and `focus-lit` (both on `:focus-visible`, in place of an outline ring). Every control of `@/ui` wears it. It is never the only cue: keep `aria-current`, `aria-checked`, `aria-selected`, `aria-expanded`.
- **Motion:** one spring, `ease-nuit`, on transform, background and color only; `prefers-reduced-motion` removes the motion and the lift.
- **Floating surfaces** (menus, lists, dialogs, tooltips, toasts, the unsaved dock) are `raised`: a glass over the page with `shadow-pop`.

## Page structure

| Need                                                                                            | Use                                                                       |
| ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| The frame of a page: title, lede, actions, sub-area words, blocks 64 px apart (54 on a desktop) | `PageLayout` (in `@/app/PageLayout`), which uses `PageHeader`             |
| A greeting at the top of Home                                                                   | `PageLayout` with `greeting`                                              |
| A page split with a list of sections on the left (Settings, a user)                             | `PageLayout` with `nav={<SectionNav … />}`; the dock docks under the list |
| A split page opening on a band (a user, a group, My account)                                    | `PageLayout` with `head={<Band … />}` and `nav`                           |
| A shelf of a page, with a count or a link on the right                                          | `Block` (+ `Count`, `TextLink`)                                           |
| Content that needs a ground of its own: a detail, a figure with its rows                        | `Panel`, split by `PanelSection`, closed by `PanelFooter`                 |
| The sub-area words under a page title (Sources · Libraries · Live TV)                           | `Tabs` with links (`words`, the default), passed as PageHeader's `nav`    |
| Sections inside a page part (a source's Summary, Categories…)                                   | `Tabs` with `variant="underline"`                                         |

## Rows, tiles and data

| Need                                                                     | Use                                                                                                                                      |
| ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Things with a label, a second line and a value (sources, devices, state) | `RowList` + `Row` (`boxed` tiles, `separate` beside a detail, `plain` for a feed)                                                        |
| Long lists (logs, channels, versions)                                    | `RowList compact`: 56 px rows (48 on a desktop)                                                                                          |
| A setting: switch, choice, number with unit, text, path, secret          | `SettingRow`, in a `grid grid-cols-[minmax(0,1fr)] gap-2`                                                                                |
| The icon or letters at the start of a row, or a tinted state bubble      | `IconTile` (`sm`, `md`, `lg`, `xl` for a band; `ok`, `warn`, `danger`), or `Avatar` for a person                                         |
| What is playing, on its poster                                           | `ArtShelf` + `ArtTile`: the lit tile opens wide with a halo and its tools; `fit="contain"` for logos, `fallback` when there is no poster |
| A problem to look at, with its cause and fix                             | `AlertTile`                                                                                                                              |
| A big number ("4 of 4")                                                  | `FigureTile`, side by side in a grid                                                                                                     |
| Columns of comparable values                                             | `Table` (`compact` for long ones; values right; scrolls inside itself)                                                                   |
| How far something is                                                     | `ProgressBar` (`playback` for what plays)                                                                                                |
| A time ("18 min ago"), with the full date on hover                       | `RelativeTime` (`className` for its color and size)                                                                                      |
| Reordering an item of a list from the keyboard                           | `MoveButtons` (focus follows the item after a move)                                                                                      |

### Setting rows

`SettingRow` is the label on the left, the value and a chevron on the right, and the help shown only while the row is lit (always in `aria-describedby`). Enter, Space or a click opens its control inside the row; Escape closes it and returns to the row; ↑ ↓ move between rows without trapping Tab.

| `kind`   | Opens                                                                           | Its own props                                             |
| -------- | ------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `switch` | Nothing: the row is the `role="switch"`, with the toggle and its word           | `checked`, `onChange`, `stateText`                        |
| `choice` | A strip of big choices (`role="radiogroup"`, ← →), or a searchable list past 12 | `value`, `options`, `onChange`                            |
| `number` | A strip of values up to 33 of them, else a number well with its unit            | `value`, `min`, `max`, `step`, `unit`, `format`, `picker` |
| `text`   | A text well; Enter closes it                                                    | `value`, `onChange`, `placeholder`, `maxLength`, `mono`   |
| `path`   | The same well; the value goes under the label on a phone                        | as `text`                                                 |
| `secret` | The `SecretField` (masked typing, reveal, replace, remove)                      | the SecretField's own                                     |

Every kind takes `saved`: a row whose value differs keeps a dot, "was …", and its value in the link color. A row without `saved` (settings that apply at once) keeps no mark. `anchor` gives it the id of its `#anchor`, `error` shows under it, and `children` add a note under it.

`OpenRow` draws the same head and lit state for a row that no `kind` covers: with `children`, Enter, Space or a click opens them inside the row (a date with its buttons, a list to choose from) and Escape closes them; with `onClick`, the row is a button for an action shown elsewhere (a dialog).

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

| Need                                      | Use                                                                        |
| ----------------------------------------- | -------------------------------------------------------------------------- |
| Any labelled control, with help and error | `Field` around the control                                                 |
| Text, a filter, a password                | `TextInput` (`icon`, `suffix`, `revealable`; `size` `md`, `sm`, `compact`) |
| A number with a unit                      | `NumberInput`                                                              |
| One choice among many                     | `Select`; two to four side by side: `Segmented`                            |
| Free text with suggestions (a genre)      | `SuggestInput`                                                             |
| Several lines                             | `Textarea` (`mono` for code)                                               |
| Something that turns on or off            | `Switch` (the word of its state beside it), or a `switch` SettingRow       |
| Picking items of a list                   | `Checkbox`                                                                 |
| An API key, a client secret               | `SecretField` (masked typing, reveal, replace, remove)                     |
| Saving a whole page of settings           | `SaveBar`, inside the `<form>`, once per page: the unsaved dock            |

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
- Nothing is wider than a phone (390 px, in English and German): a row of actions wraps, labels wrap before values do, and a row that scrolls sideways shows it with the `scroll-fade` class and `useScrollFade`, which fade the edge it scrolls past (`Tabs` and `SectionNav` already do).
- Every control has a label; toggles set `aria-pressed` or `aria-checked`, disclosures `aria-expanded`. Arrows may move within a list, never trap Tab.
- Posters and art load lazily (`loading="lazy"`); nothing heavy on every page.
- Add a component here only when two areas need it; otherwise keep it in `src/features/<area>/`.
