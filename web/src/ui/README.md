# `src/ui`: the « Nuit » design system

Import everything from `@/ui`. Each component documents its props in its file; `/admin/dev/ui` (development builds only) shows them all in their states. Colors, radii and type come from the tokens in `src/index.css` (`bg-s1`, `text-ink-2`, `border-line-2`, `rounded-panel`, `text-h1`, `shadow-pop`, `figures`…): never write a raw color. Icons come from `@phosphor-icons/react`, regular weight, 16/18/20 px.

## Page structure

| Need                                                                    | Use                                                           |
| ----------------------------------------------------------------------- | ------------------------------------------------------------- |
| The frame of a page: title, lede, actions, blocks 52 px apart           | `PageLayout` (in `@/app/PageLayout`), which uses `PageHeader` |
| A page split with a list of sections on the left (Settings, My account) | `PageLayout` with `nav={<SectionNav … />}`                    |
| A titled part of a page, with a count or a link on the right            | `Block` (+ `Count`, `TextLink`)                               |
| Content that needs an edge: a detail beside a list, a form              | `Panel`, split by `PanelSection`, closed by `PanelFooter`     |
| Navigation between sibling pages, or sections of a detail               | `Tabs` (`pill` above a page, `underline` inside a panel)      |

## Lists and data

| Need                                                                  | Use                                                                         |
| --------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Things with a name, a meta line and a state (sources, devices, users) | `RowList` + `Row` (`boxed`, `plain` for a feed, `separate` beside a detail) |
| The icon or letters at the start of a row                             | `IconTile`, or `Avatar` for a person                                        |
| Columns of comparable values                                          | `Table` (scrolls inside itself on a phone)                                  |
| Figures that are compared (counts, times, IPs, versions)              | the `figures` class                                                         |
| How far something is                                                  | `ProgressBar` (`brand` only for playback)                                   |

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

| Need                                      | Use                                                    |
| ----------------------------------------- | ------------------------------------------------------ |
| Any labelled control, with help and error | `Field` around the control                             |
| Text, a filter, a password                | `TextInput` (`icon`, `suffix`, `revealable`)           |
| A number with a unit                      | `NumberInput`                                          |
| One choice among many                     | `Select`; two to four side by side: `Segmented`        |
| Several lines                             | `Textarea` (`mono` for code)                           |
| A setting that turns something on         | `Switch` (`stateText` to write On / Off beside it)     |
| Picking items of a list                   | `Checkbox`                                             |
| An API key, a client secret               | `SecretField` (masked typing, reveal, replace, remove) |
| Saving a whole page of settings           | `SaveBar`, inside the `<form>`, once per page          |

## Actions

| Need                                                | Use                                                                |
| --------------------------------------------------- | ------------------------------------------------------------------ |
| An action                                           | `Button`: `primary` (one per view), `secondary`, `ghost`, `danger` |
| A link that looks like a button                     | `ButtonLink`, or `ExternalButtonLink` outside the app              |
| An icon only (more actions, delete in a row)        | `IconButton`, whose `label` is required                            |
| Several actions behind one button, the account menu | `Menu` with `MenuItem`, `MenuChoice`, `MenuSeparator`              |
| Asking before a destructive action                  | `ConfirmDialog`                                                    |
| An editor too large for its row, the phone menu     | `Drawer`                                                           |
| A hint on hover or focus                            | `Tooltip` (never the only copy of something needed)                |
| A keyboard shortcut                                 | `Kbd` (`modKey` is ⌘ or Ctrl)                                      |

## Rules

- Copy goes through `useI18n()`; the components' own words are in `t.ui`.
- Every control has a label; toggles set `aria-pressed` or `aria-checked`, disclosures `aria-expanded`.
- Motion comes from the tokens (`duration-160`/`220`, `ease-nuit`, `animate-rise`, `stagger`) and stops under `prefers-reduced-motion`.
- Add a component here only when two areas need it; otherwise keep it in `src/features/<area>/`.
