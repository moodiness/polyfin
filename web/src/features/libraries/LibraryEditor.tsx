import {
  ArrowsClockwiseIcon,
  CaretDownIcon,
  CaretRightIcon,
  CaretUpIcon,
  DotsSixVerticalIcon,
  DotsThreeIcon,
  FilmStripIcon,
  MagnifyingGlassIcon,
  MusicNoteIcon,
  PlusIcon,
  SquaresFourIcon,
  StopIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { Link } from 'react-router'
import {
  ApiError,
  fetchAddons,
  fetchLibraries,
  queryClient,
  queryKeys,
  MAX_LIBRARY_ITEMS,
  refreshLibrary,
  saveLibraries,
  stopLibraryRefresh,
  type Addon,
  type Library,
  type LibraryShowAs,
  type LibraryRefresh,
  type MusicContent,
  type Scope,
} from '@/api'
import { lineupPath } from '@/features/iptv/lineup'
import {
  failingGuides,
  guideSummary,
  guidesLink,
  isIptv,
  isTvCatalog,
} from '@/features/livetv/catalogs'
import { errorMessage, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  cx,
  EmptyState,
  Field,
  InlineError,
  Notice,
  Row,
  RowList,
  SaveBar,
  Select,
  SettingRow,
  SkeletonRows,
  StatusPill,
  TextInput,
  IconButton,
  useToast,
  Menu,
  MenuItem,
  MoveButtons,
  ProgressBar,
} from '@/ui'
import { LibraryImageEditor, LibraryThumb, LiveTvThumb } from './LibraryImage'
import { useRowDrag } from './useRowDrag'
import UserLibraryOrders from './UserLibraryOrders'

/** Above this many libraries the home screen of Jellyfin apps gets heavy; the server default. */
const recommendedLibraries = 20

/** The longest library name the server accepts. */
const nameMaxLength = 64

/**
 * One enabled library being edited. `name` is the text of its name field: empty = catalog name.
 * `genre` and `maxItems` narrow it; null for none. `hideInMenus` leaves it out of the web
 * player's menus. `skipLocal` leaves out the titles local folders hold. `showAs` is how it shows
 * its titles, null while none is chosen (see `Library.showAs`).
 */
type Entry = {
  key: string
  library: Library
  name: string
  genre: string | null
  maxItems: number | null
  hideInMenus: boolean
  skipLocal: boolean
  showAs: LibraryShowAs | null
}

/** The options of an entry the list saves, each counted as one change. */
const entryOptions = ['name', 'genre', 'maxItems', 'hideInMenus', 'skipLocal', 'showAs'] as const

/** Whether the server takes this maximum: none, or a whole number from 1 to MAX_LIBRARY_ITEMS. */
function validMax(maxItems: number | null): boolean {
  return (
    maxItems === null ||
    (Number.isInteger(maxItems) && maxItems >= 1 && maxItems <= MAX_LIBRARY_ITEMS)
  )
}

function catalogKey(library: Pick<Library, 'addonId' | 'catalogType' | 'catalogId'>): string {
  return JSON.stringify([library.addonId, library.catalogType, library.catalogId])
}

function sameEntries(a: Entry[], b: Entry[]): boolean {
  return (
    a.length === b.length &&
    a.every(
      (entry, i) =>
        entry.key === b[i].key && entryOptions.every((option) => entry[option] === b[i][option]),
    )
  )
}

/**
 * How many changes the dock counts between the list and the saved one: each library added or
 * removed, each option changed, and the order once when it changed.
 */
function countChanges(entries: Entry[], saved: Entry[]): number {
  const before = new Map(saved.map((entry) => [entry.key, entry]))
  const keys = new Set(entries.map((entry) => entry.key))
  let count = saved.filter((entry) => !keys.has(entry.key)).length
  for (const entry of entries) {
    const was = before.get(entry.key)
    if (was === undefined) count += 1
    else count += entryOptions.filter((option) => entry[option] !== was[option]).length
  }
  const order = entries.filter((entry) => before.has(entry.key))
  const savedOrder = saved.filter((entry) => keys.has(entry.key))
  if (order.some((entry, i) => entry.key !== savedOrder[i].key)) count += 1
  return Math.max(count, 1)
}

/** 64 px between the blocks of the editor (44 on a phone), as PageLayout spaces a page's. */
const blockSpacing = '[&>*+*]:mt-block max-md:[&>*+*]:mt-11'

/**
 * A list too long for the page scrolls in its own box, with a thin scrollbar, so the blocks below
 * and the dock stay close.
 */
const scrollBox =
  'max-h-[40rem] overflow-y-auto overscroll-contain [scrollbar-color:var(--color-line-3)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:w-2'

/** Above this many catalogs available, their list scrolls in its box. */
const boxedCatalogs = 12

/**
 * Chooses which catalogs of a scope's addons are libraries in Jellyfin apps, in which order and
 * under which name. Edits stay local until saved with the dock.
 *
 * It renders blocks (the libraries, the available catalogs, the dock) inside one form, so place
 * it in a PageLayout or among a page's blocks: `<LibraryEditor scope="me" />`.
 */
export default function LibraryEditor({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const libraries = useQuery({
    queryKey: queryKeys.libraries(scope),
    queryFn: ({ signal }) => fetchLibraries(scope, signal),
    // A refresh of descriptions under way or waiting shows its progress.
    refetchInterval: (query) =>
      query.state.data?.some((library) => refreshing(library.refresh)) === true ? 2000 : false,
  })
  // Only used to flag libraries of turned-off addons and music or IPTV rows; it works without.
  const addons = useQuery({
    queryKey: queryKeys.addons(scope),
    queryFn: ({ signal }) => fetchAddons(scope, signal),
  })

  if (libraries.isPending) {
    return (
      <Block title={t.libraries.shownTitle}>
        <SkeletonRows rows={3} label={t.libraries.loading} />
      </Block>
    )
  }
  if (libraries.isError) {
    return (
      <Block title={t.libraries.shownTitle}>
        <InlineError onRetry={() => void libraries.refetch()} retrying={libraries.isFetching}>
          {errorMessage(t, libraries.error)}
        </InlineError>
      </Block>
    )
  }
  return <LibraryForm scope={scope} catalogs={libraries.data} addons={addons.data ?? []} />
}

/** Whether a refresh of a library's descriptions is waiting or running. */
function refreshing(refresh: LibraryRefresh | null): boolean {
  return refresh !== null && (refresh.state === 'queued' || refresh.state === 'running')
}

/** How the last refresh of a library's descriptions stands, under its name. */
function RefreshStatus({ refresh, name }: { refresh: LibraryRefresh | null; name: string }) {
  const { t, language } = useI18n()
  const text = t.libraries.refresh
  if (refresh === null) return null
  const figure = (value: number) => value.toLocaleString(language)
  switch (refresh.state) {
    case 'queued':
      return <p className="mt-1.5 text-small text-ink-3">{text.queued}</p>
    case 'running':
      return (
        <div className="mt-1.5 max-w-[20rem]">
          <p className="figures text-small text-ink-3">
            {text.running(figure(refresh.done + refresh.failed), figure(refresh.titles))}
          </p>
          <ProgressBar
            className="mt-1.5"
            size="sm"
            label={text.progress(name)}
            value={refresh.titles === 0 ? 0 : (refresh.done + refresh.failed) / refresh.titles}
          />
        </div>
      )
    case 'done':
      return (
        <p className="figures mt-1.5 text-small text-ink-3">
          {text.done(figure(refresh.done), refresh.failed, figure(refresh.failed))}
        </p>
      )
    case 'stopped':
      return (
        <p className="figures mt-1.5 text-small text-ink-3">
          {text.stopped(figure(refresh.done + refresh.failed), figure(refresh.titles))}
        </p>
      )
    case 'failed':
      return <p className="mt-1.5 text-small text-danger">{text.failed}</p>
  }
}

function LibraryForm({
  scope,
  catalogs,
  addons,
}: {
  scope: Scope
  catalogs: Library[]
  addons: Addon[]
}) {
  const { t, language } = useI18n()
  const toast = useToast()
  const baseId = useId()
  const [draft, setDraft] = useState<Entry[] | null>(null)
  const [query, setQuery] = useState('')
  const [type, setType] = useState('')
  const [announcement, setAnnouncement] = useState('')
  // The entry whose image editor is open, below its row.
  const [imageOpen, setImageOpen] = useState<string | null>(null)
  // The entry whose options are open, below its row; never with its image editor.
  const [optionsOpen, setOptionsOpen] = useState<string | null>(null)
  // Element ids to try, in order, once the next render is committed (the focused row may be gone).
  const focusAfterRender = useRef<string[]>([])
  // The row to bring into view once the next render is committed, after a move.
  const scrollAfterRender = useRef<string | null>(null)
  // The box of available catalogs, back at its top when the filter changes.
  const availableBox = useRef<HTMLDivElement>(null)
  // The list of libraries shown, whose rows are dragged by their grip.
  const shownList = useRef<HTMLOListElement>(null)
  const rowDrag = useRowDrag({ list: shownList, onDrop: move })
  function filterBy(change: () => void) {
    change()
    availableBox.current?.scrollTo({ top: 0 })
  }

  useEffect(() => {
    const row = scrollAfterRender.current
    if (row !== null) {
      scrollAfterRender.current = null
      document.getElementById(row)?.scrollIntoView({ block: 'nearest' })
    }
    const candidates = focusAfterRender.current
    if (candidates.length === 0) return
    focusAfterRender.current = []
    for (const id of candidates) {
      const element = document.getElementById(id)
      if (element !== null) {
        focusIn(element)
        return
      }
    }
  })

  const refresh = useMutation({
    mutationFn: ({ library, stop }: { library: Library; name: string; stop: boolean }) => {
      const key = {
        addonId: library.addonId,
        catalogType: library.catalogType,
        catalogId: library.catalogId,
      }
      return stop ? stopLibraryRefresh(key) : refreshLibrary(key).then(() => undefined)
    },
    onSuccess: (_, { name, stop }) => {
      if (!stop) toast(t.libraries.refresh.started(name))
    },
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
    onSettled: () => void queryClient.invalidateQueries({ queryKey: queryKeys.libraries(scope) }),
  })

  const save = useMutation({
    mutationFn: (entries: Entry[]) =>
      saveLibraries(
        scope,
        entries.map(({ library, name, genre, maxItems, hideInMenus, skipLocal, showAs }) => ({
          addonId: library.addonId,
          catalogType: library.catalogType,
          catalogId: library.catalogId,
          name: name.trim() === '' ? null : name.trim(),
          genre,
          maxItems,
          hideInMenus,
          skipLocal,
          showAs,
        })),
      ),
    onSuccess: (saved, entries) => {
      queryClient.setQueryData(queryKeys.libraries(scope), saved)
      // Keep edits made while saving; otherwise show the saved list.
      setDraft((current) => (current === entries ? null : current))
      toast(t.libraries.saved)
    },
    onError: (error) => {
      // A catalog went away (addon refreshed or removed): fetch the list to flag it.
      if (error instanceof ApiError && error.code === 'invalid_library') {
        void queryClient.invalidateQueries({ queryKey: queryKeys.libraries(scope) })
      }
    },
  })

  const byKey = new Map(catalogs.map((library) => [catalogKey(library), library]))
  const serverEntries: Entry[] = catalogs
    .filter((library) => library.enabled)
    .map((library) => ({
      key: catalogKey(library),
      library,
      name: library.name ?? '',
      genre: library.genre,
      maxItems: library.maxItems,
      hideInMenus: library.hideInMenus,
      skipLocal: library.skipLocal,
      showAs: library.showAs,
    }))
  const savedByKey = new Map(serverEntries.map((entry) => [entry.key, entry]))
  const entries = draft ?? serverEntries
  const dirty = draft !== null
  const shownKeys = new Set(entries.map((entry) => entry.key))
  const offAddons = new Set(addons.filter((addon) => !addon.enabled).map((addon) => addon.id))
  const iptvAddons = new Set(addons.filter((addon) => isIptv(addon)).map((addon) => addon.id))
  // What each music addon's rows hold: their libraries are music or books libraries in apps.
  const musicContent = new Map(
    addons.flatMap((addon) =>
      addon.music === null ? [] : [[addon.id, addon.music.contentType] as const],
    ),
  )

  const types = [...new Set(catalogs.map((library) => library.catalogType))]
  const needle = query.trim().toLocaleLowerCase()
  const notShown = catalogs.filter((library) => !shownKeys.has(catalogKey(library)))
  const available = notShown.filter(
    (library) =>
      (type === '' || library.catalogType === type) &&
      (needle === '' ||
        library.catalogName.toLocaleLowerCase().includes(needle) ||
        library.addonName.toLocaleLowerCase().includes(needle)),
  )
  const groups = new Map<string, Library[]>()
  for (const library of available) {
    const group = groups.get(library.addonId)
    if (group === undefined) groups.set(library.addonId, [library])
    else group.push(library)
  }

  const ids = {
    remove: (key: string) => `${baseId}-remove-${key}`,
    add: (key: string) => `${baseId}-add-${key}`,
    row: (key: string) => `${baseId}-row-${key}`,
    image: (key: string) => `${baseId}-image-${key}`,
    thumb: (key: string) => `${baseId}-thumb-${key}`,
    options: (key: string) => `${baseId}-options-${key}`,
    optionsButton: (key: string) => `${baseId}-options-button-${key}`,
    maxItems: (key: string) => `${baseId}-max-items-${key}`,
    filter: `${baseId}-filter`,
  }

  function edit(next: Entry[]) {
    save.reset()
    // Back to the saved state: drop the draft so refetched data shows again.
    setDraft(sameEntries(next, serverEntries) ? null : next)
  }

  function displayName(entry: Entry): string {
    return entry.name.trim() === '' ? entry.library.catalogName : entry.name.trim()
  }

  function move(from: number, to: number) {
    const next = [...entries]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    scrollAfterRender.current = ids.row(moved.key)
    edit(next)
    setAnnouncement(t.common.moved(displayName(moved), to + 1, next.length))
  }

  function change(index: number, patch: Partial<Omit<Entry, 'key' | 'library'>>) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, ...patch } : entry)))
  }

  /** Opens the options of `key` below its row, or closes them with null, and the image editor. */
  function openOptions(key: string | null) {
    setImageOpen(null)
    setOptionsOpen(key)
  }

  function remove(index: number) {
    const removed = entries[index]
    const next = entries.filter((_, i) => i !== index)
    if (imageOpen === removed.key) setImageOpen(null)
    if (optionsOpen === removed.key) setOptionsOpen(null)
    focusAfterRender.current = [
      ...(index < next.length ? [ids.remove(next[index].key)] : []),
      ...(index > 0 ? [ids.remove(next[index - 1].key)] : []),
      ids.add(removed.key),
      ids.filter,
    ]
    edit(next)
    setAnnouncement(t.libraries.removedLive(displayName(removed)))
  }

  function add(library: Library) {
    const key = catalogKey(library)
    const index = available.indexOf(library)
    focusAfterRender.current = [
      ...available.slice(index + 1).map((item) => ids.add(catalogKey(item))),
      ...available
        .slice(0, index)
        .reverse()
        .map((item) => ids.add(catalogKey(item))),
      ids.filter,
      // The last catalog added: the filter is gone with the list, so go to its new row.
      ids.remove(key),
    ]
    edit([
      ...entries,
      {
        key,
        library,
        name: '',
        genre: null,
        maxItems: null,
        hideInMenus: false,
        skipLocal: false,
        showAs: null,
      },
    ])
    setAnnouncement(t.libraries.addedLive(library.catalogName, entries.length + 1))
  }

  if (catalogs.length === 0 && entries.length === 0) {
    return (
      <Block title={t.libraries.shownTitle}>
        <EmptyState
          icon={FilmStripIcon}
          title={t.libraries.noAddonsTitle}
          action={
            scope === 'shared' ? (
              <ButtonLink to="/sources" variant="primary" icon={PlusIcon}>
                {t.libraries.addSource}
              </ButtonLink>
            ) : undefined
          }
        >
          {t.libraries.noAddons}
        </EmptyState>
      </Block>
    )
  }

  return (
    <form
      className={blockSpacing}
      onSubmit={(event) => {
        event.preventDefault()
        // A maximum the server refuses: show it in its row's options rather than save.
        const invalid = entries.find((entry) => !validMax(entry.maxItems))
        if (invalid !== undefined) {
          const field = document.getElementById(ids.maxItems(invalid.key))
          if (optionsOpen === invalid.key && field !== null) {
            focusIn(field)
          } else {
            focusAfterRender.current = [ids.maxItems(invalid.key)]
            openOptions(invalid.key)
          }
          return
        }
        save.mutate(entries)
      }}
    >
      <Block title={t.libraries.shownTitle} count={entries.length}>
        <p className="sr-only" role="status">
          {announcement}
        </p>
        <p className="mb-5 max-w-[66ch] text-small text-ink-2">
          {scope === 'shared' ? t.libraries.shownHelp : t.libraries.shownHelpMine}
        </p>
        {entries.length > recommendedLibraries && (
          <Notice tone="warn" className="mb-4">
            {t.libraries.manyWarning}
          </Notice>
        )}
        {entries.length === 0 ? (
          <EmptyState icon={FilmStripIcon} title={t.libraries.shownEmptyTitle}>
            {t.libraries.shownEmpty} {t.libraries.shownEmptyHow}
          </EmptyState>
        ) : (
          <ol
            ref={shownList}
            aria-label={t.libraries.listLabel}
            className="grid grid-cols-[minmax(0,1fr)] gap-2"
          >
            {entries.map((entry, index) => {
              const library = byKey.get(entry.key) ?? entry.library
              const saved = savedByKey.get(entry.key)
              const missing = !(byKey.get(entry.key)?.browsable ?? false)
              const name = displayName(entry)
              // The server names saved libraries only: edits may change what apps show.
              const appName = dirty ? null : library.appName
              const renamed = appName !== null && appName !== name
              const music = musicContent.get(library.addonId)
              const iptv = iptvAddons.has(library.addonId)
              const tv = isTvCatalog(library)
              // How it shows its titles: its choice, else its catalog's default, the first.
              const shown = entry.showAs ?? library.showAsChoices[0]
              const otherVideos = library.showAsChoices.includes('home_videos')
              const hasOptions = !tv || library.filterable
              const imageShown = !tv && imageOpen === entry.key
              const optionsShown = hasOptions && optionsOpen === entry.key
              const dragged = rowDrag.dragged === index
              return (
                <li
                  key={entry.key}
                  id={ids.row(entry.key)}
                  className={cx('relative min-w-0', dragged && 'opacity-40')}
                >
                  {/* The drop place, in the gap above or below the row. */}
                  {rowDrag.dropBefore === index && (
                    <span
                      aria-hidden="true"
                      className="absolute inset-x-4 -top-[0.3125rem] h-0.5 rounded-full bg-accent"
                    />
                  )}
                  {rowDrag.dropBefore === entries.length && index === entries.length - 1 && (
                    <span
                      aria-hidden="true"
                      className="absolute inset-x-4 -bottom-[0.3125rem] h-0.5 rounded-full bg-accent"
                    />
                  )}
                  <div
                    className={cx(
                      'grid min-h-20 grid-cols-[auto_auto_minmax(0,1fr)_auto] items-center gap-x-4 gap-y-3 rounded-row py-3.5 pr-[1.375rem] pl-3',
                      'max-md:min-h-[4.25rem] max-md:grid-cols-[auto_auto_minmax(0,1fr)] max-md:gap-x-3 max-md:py-3 max-md:pr-4 max-md:pl-2',
                      optionsShown || imageShown ? 'bg-s2' : 'bg-s1',
                    )}
                  >
                    <span className="flex items-center gap-0.5">
                      {/* Mouse, pen and touch only: keyboards and screen readers use the arrows. */}
                      <span
                        aria-hidden="true"
                        title={t.libraries.dragHandle(name)}
                        className={cx(
                          'flex h-10 w-7 shrink-0 touch-none items-center justify-center rounded-control text-ink-3 select-none [-webkit-touch-callout:none] hover:text-ink',
                          dragged ? 'cursor-grabbing' : 'cursor-grab',
                        )}
                        {...rowDrag.grip(index)}
                      >
                        <DotsSixVerticalIcon size="1.25rem" />
                      </span>
                      <span
                        aria-hidden="true"
                        className="figures min-w-[1.4em] text-micro text-ink-3"
                      >
                        {index + 1}
                      </span>
                    </span>
                    {tv ? (
                      <LiveTvThumb />
                    ) : (
                      <LibraryThumb
                        id={ids.thumb(entry.key)}
                        library={library}
                        name={name}
                        open={imageShown}
                        controls={ids.image(entry.key)}
                        onToggle={() => {
                          setOptionsOpen(null)
                          setImageOpen(imageShown ? null : entry.key)
                        }}
                      />
                    )}
                    <div className="min-w-0">
                      <p className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                        <span className="min-w-0 text-h3 break-words text-ink">
                          {library.catalogName}
                        </span>
                        <MusicKind content={music} />
                        {entry.genre !== null && <Badge>{entry.genre}</Badge>}
                        {entry.maxItems !== null && (
                          <Badge tone={validMax(entry.maxItems) ? 'neutral' : 'danger'}>
                            {t.libraries.narrow.maxBadge(entry.maxItems.toLocaleString(language))}
                          </Badge>
                        )}
                      </p>
                      <p className="mt-0.5 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-small text-ink-3 max-md:text-micro">
                        <span className="break-words">
                          {library.addonName} · {stremioLabel(t.stremioTypes, library.catalogType)}
                        </span>
                        {(appName ?? name) !== library.catalogName && (
                          <span className="break-words">
                            · {t.libraries.appName(appName ?? name)}
                          </span>
                        )}
                      </p>
                      {(offAddons.has(library.addonId) ||
                        missing ||
                        (!tv && entry.hideInMenus) ||
                        (library.skippable && entry.skipLocal) ||
                        (!tv && shown !== undefined && (otherVideos || shown !== 'listed'))) && (
                        <p className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1">
                          {offAddons.has(library.addonId) && (
                            <StatusPill tone="danger">{t.libraries.addonOff}</StatusPill>
                          )}
                          {missing && <StatusPill tone="danger">{t.libraries.missing}</StatusPill>}
                          {!tv && entry.hideInMenus && (
                            <StatusPill tone="muted">{t.libraries.hiddenInMenus}</StatusPill>
                          )}
                          {library.skippable && entry.skipLocal && (
                            <StatusPill tone="muted">{t.libraries.skippedLocal}</StatusPill>
                          )}
                          {!tv && shown !== undefined && (otherVideos || shown !== 'listed') && (
                            <StatusPill tone="muted">{t.libraries.showAs.pill[shown]}</StatusPill>
                          )}
                        </p>
                      )}
                      {tv && <TvSummary scope={scope} library={library} name={name} iptv={iptv} />}
                      {library.refreshable && (
                        <RefreshStatus refresh={library.refresh} name={name} />
                      )}
                    </div>
                    <div className="flex flex-wrap items-center justify-end gap-1 max-md:col-span-full">
                      {hasOptions && (
                        <Button
                          id={ids.optionsButton(entry.key)}
                          size="sm"
                          variant="ghost"
                          iconEnd={optionsShown ? CaretUpIcon : CaretDownIcon}
                          aria-label={t.libraries.optionsLabel(name)}
                          aria-expanded={optionsShown}
                          aria-controls={optionsShown ? ids.options(entry.key) : undefined}
                          onClick={() => openOptions(optionsShown ? null : entry.key)}
                          className="max-md:mr-auto"
                        >
                          {t.libraries.options}
                        </Button>
                      )}
                      <MoveButtons
                        name={name}
                        index={index}
                        count={entries.length}
                        onMove={(to) => move(index, to)}
                      />
                      {library.refreshable && (
                        <Menu
                          label={t.libraries.moreActions(name)}
                          align="end"
                          trigger={(props) => (
                            <IconButton
                              {...props}
                              size="sm"
                              label={t.libraries.moreActions(name)}
                              icon={DotsThreeIcon}
                            />
                          )}
                        >
                          {refreshing(library.refresh) ? (
                            <MenuItem
                              icon={StopIcon}
                              onSelect={() => refresh.mutate({ library, name, stop: true })}
                            >
                              {t.libraries.refresh.stop}
                            </MenuItem>
                          ) : (
                            <MenuItem
                              icon={ArrowsClockwiseIcon}
                              onSelect={() => refresh.mutate({ library, name, stop: false })}
                            >
                              {t.libraries.refresh.start}
                            </MenuItem>
                          )}
                        </Menu>
                      )}
                      <IconButton
                        id={ids.remove(entry.key)}
                        size="sm"
                        icon={XIcon}
                        danger
                        label={t.libraries.removeLabel(name)}
                        onClick={() => remove(index)}
                      />
                    </div>
                  </div>
                  {imageShown && (
                    <div className="mt-2 pl-12 max-md:pl-0">
                      <LibraryImageEditor
                        id={ids.image(entry.key)}
                        scope={scope}
                        library={library}
                        name={name}
                        onClose={() => {
                          setImageOpen(null)
                          focusAfterRender.current = [ids.thumb(entry.key)]
                        }}
                      />
                    </div>
                  )}
                  {optionsShown && (
                    <section
                      id={ids.options(entry.key)}
                      aria-label={t.libraries.optionsLabel(name)}
                      className="mt-2 grid animate-rise grid-cols-[minmax(0,1fr)] gap-2 pl-12 max-md:pl-0"
                    >
                      {!tv && (
                        <>
                          <SettingRow
                            kind="text"
                            label={t.libraries.name}
                            detail={renamed ? t.libraries.appName(appName) : undefined}
                            help={t.libraries.nameHelp}
                            value={entry.name}
                            saved={saved?.name}
                            onChange={(value) => change(index, { name: value })}
                            placeholder={appName ?? library.catalogName}
                            maxLength={nameMaxLength}
                          />
                          <SettingRow
                            kind="switch"
                            label={t.libraries.hideInMenus}
                            help={t.libraries.hideInMenusHelp}
                            checked={entry.hideInMenus}
                            saved={saved?.hideInMenus}
                            onChange={(hidden) => change(index, { hideInMenus: hidden })}
                          />
                          {library.skippable && (
                            <SettingRow
                              kind="switch"
                              label={t.libraries.skipLocal}
                              help={t.libraries.skipLocalHelp}
                              checked={entry.skipLocal}
                              saved={saved?.skipLocal}
                              onChange={(skip) => change(index, { skipLocal: skip })}
                            />
                          )}
                          {shown !== undefined && (
                            <SettingRow
                              kind="choice"
                              label={t.libraries.showAs.label}
                              help={
                                (shown === 'movies_and_shows'
                                  ? otherVideos
                                    ? t.libraries.showAs.help.titles
                                    : t.libraries.showAs.help.mixed
                                  : t.libraries.showAs.help[shown]) +
                                (otherVideos && entry.showAs === null
                                  ? ' ' + t.libraries.showAs.undecided
                                  : '')
                              }
                              value={shown}
                              saved={
                                saved === undefined
                                  ? undefined
                                  : (saved.showAs ?? library.showAsChoices[0])
                              }
                              options={library.showAsChoices.map((value) => ({
                                value,
                                label: t.libraries.showAs.choices[value],
                              }))}
                              onChange={(value) => change(index, { showAs: value })}
                            />
                          )}
                        </>
                      )}
                      {library.filterable && (
                        <NarrowRows
                          maxItemsId={ids.maxItems(entry.key)}
                          scope={scope}
                          library={library}
                          entry={entry}
                          saved={saved}
                          onChange={(patch) => change(index, patch)}
                        />
                      )}
                      {!tv && (music !== undefined || iptv) && (
                        <div className="px-[1.875rem] pt-1 text-small text-ink-2 max-md:px-5">
                          {music !== undefined && (
                            <p>{t.libraries.music.help[music] ?? t.libraries.music.help.music}</p>
                          )}
                          {iptv && (
                            <p className={cx(music !== undefined && 'mt-1.5')}>
                              {t.libraries.iptvVod}{' '}
                              <Link
                                to={lineupPath(scope, library.addonId, 'options')}
                                className="focus-lit rounded-control font-medium text-link underline decoration-link/40 underline-offset-4 hover:decoration-link"
                              >
                                {t.libraries.iptvVodLink}
                              </Link>
                            </p>
                          )}
                        </div>
                      )}
                    </section>
                  )}
                </li>
              )
            })}
          </ol>
        )}
        {scope === 'shared' && <UserLibraryOrders dirty={dirty} />}
        <details className="group mt-6">
          <summary className="focus-lit inline-flex cursor-pointer list-none items-center gap-2 rounded-control px-2 py-1.5 text-control font-medium text-ink-2 transition-colors duration-300 ease-nuit hover:text-ink max-md:-mx-2 [&::-webkit-details-marker]:hidden">
            <CaretRightIcon
              size="1.125rem"
              aria-hidden="true"
              className="shrink-0 transition-transform duration-300 ease-nuit group-open:rotate-90"
            />
            {t.libraries.defaultTitle}
          </summary>
          <p className="mt-2 max-w-[66ch] pl-9 text-small text-ink-2 max-md:pl-6">
            {t.libraries.defaultHelp}
          </p>
        </details>
      </Block>

      <Block title={t.libraries.availableTitle} count={notShown.length}>
        <p className="mb-5 max-w-[66ch] text-small text-ink-2">{t.libraries.availableHelp}</p>
        {notShown.length === 0 ? (
          <EmptyState icon={SquaresFourIcon} title={t.libraries.availableEmpty} />
        ) : (
          <>
            <div className="mb-5 flex flex-wrap items-end gap-3">
              <div id={ids.filter} className="min-w-0 flex-[1_1_16.25rem] md:max-w-[27.5rem]">
                <Field label={t.libraries.filter}>
                  <TextInput
                    type="search"
                    size="sm"
                    icon={MagnifyingGlassIcon}
                    value={query}
                    onValue={(value) => filterBy(() => setQuery(value))}
                    placeholder={t.libraries.filterPlaceholder}
                    autoComplete="off"
                  />
                </Field>
              </div>
              <Field
                label={t.libraries.type}
                className="min-w-0 flex-[1_1_12.5rem] md:max-w-[17.5rem]"
              >
                <Select
                  size="sm"
                  value={type}
                  onValue={(value) => filterBy(() => setType(value))}
                  options={[
                    { value: '', label: t.libraries.allTypes },
                    ...types.map((value) => ({
                      value,
                      label: stremioLabel(t.stremioTypes, value),
                    })),
                  ]}
                />
              </Field>
            </div>
            {groups.size === 0 ? (
              <p role="status" className="text-small text-ink-2">
                {t.libraries.noMatch}
              </p>
            ) : (
              <div
                ref={availableBox}
                className={cx(
                  'grid grid-cols-[minmax(0,1fr)] gap-6',
                  notShown.length > boxedCatalogs && cx(scrollBox, 'pr-1'),
                )}
              >
                {[...groups].map(([addonId, items]) => {
                  const headingId = `${baseId}-addon-${addonId}`
                  return (
                    <section key={addonId} aria-labelledby={headingId} className="min-w-0">
                      <h3
                        id={headingId}
                        className="mb-2.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-group text-ink"
                      >
                        <span className="min-w-0 break-words">{items[0].addonName}</span>
                        {offAddons.has(addonId) && (
                          <StatusPill tone="danger">{t.libraries.addonOff}</StatusPill>
                        )}
                      </h3>
                      <RowList compact aria-label={t.libraries.availableLabel(items[0].addonName)}>
                        {items.map((library) => (
                          <AvailableRow
                            key={catalogKey(library)}
                            library={library}
                            addId={ids.add(catalogKey(library))}
                            content={musicContent.get(library.addonId)}
                            onAdd={() => add(library)}
                          />
                        ))}
                      </RowList>
                    </section>
                  )
                })}
              </div>
            )}
          </>
        )}
      </Block>

      <SaveBar
        dirty={dirty}
        changes={dirty ? countChanges(entries, serverEntries) : undefined}
        saving={save.isPending}
        onDiscard={() => {
          save.reset()
          setDraft(null)
        }}
        error={save.isError ? errorMessage(t, save.error) : undefined}
      />
    </form>
  )
}

/** Focuses the control an element holds: a field's input, or a setting row's head. */
function focusIn(element: HTMLElement) {
  const target = element.matches('input, button')
    ? element
    : element.querySelector<HTMLElement>('input, [data-setting-head]')
  target?.focus()
}

/**
 * The genre and maximum of a library, among its options: staged like its name and saved with
 * the list. The genre shows only when the catalog offers genres.
 */
function NarrowRows({
  maxItemsId,
  scope,
  library,
  entry,
  saved,
  onChange,
}: {
  /** The id of the maximum's row, to focus it. */
  maxItemsId: string
  scope: Scope
  library: Library
  entry: Entry
  /** The entry as saved; undefined for a library added since. */
  saved: Entry | undefined
  onChange: (change: Partial<Pick<Entry, 'genre' | 'maxItems'>>) => void
}) {
  const { t, language } = useI18n()
  const text = t.libraries.narrow
  const max = MAX_LIBRARY_ITEMS.toLocaleString(language)
  // A genre the catalog no longer offers stays listed, so the choice shows; saving reports it.
  const genres =
    entry.genre === null || library.genres.includes(entry.genre)
      ? library.genres
      : [entry.genre, ...library.genres]
  return (
    <>
      {genres.length > 0 && (
        <SettingRow
          kind="choice"
          label={text.genre}
          help={text.genreHelp}
          value={entry.genre ?? ''}
          saved={saved === undefined ? undefined : (saved.genre ?? '')}
          options={[
            { value: '', label: text.allGenres },
            ...genres.map((genre) => ({ value: genre, label: genre })),
          ]}
          onChange={(value) => onChange({ genre: value === '' ? null : value })}
        />
      )}
      <SettingRow
        anchor={maxItemsId}
        kind="number"
        picker="well"
        label={text.maxItems}
        help={
          scope === 'shared'
            ? text.maxHelp(max, t.nav.trail(t.nav.settings, t.nav.settingsSections.catalogs))
            : text.maxHelpMine(max)
        }
        error={validMax(entry.maxItems) ? undefined : text.maxRange(max)}
        value={entry.maxItems}
        saved={saved?.maxItems}
        min={1}
        max={MAX_LIBRARY_ITEMS}
        step={1}
        format={(value) => value.toLocaleString(language)}
        onChange={(value) => onChange({ maxItems: value })}
      />
    </>
  )
}

/** A TV row: its channels go to Live TV; its guides at a glance, with where they are managed. */
function TvSummary({
  scope,
  library,
  name,
  iptv,
}: {
  scope: Scope
  library: Library
  name: string
  iptv: boolean
}) {
  const { language, t } = useI18n()
  if (library.guides === null) {
    return (
      <div className="mt-1.5 text-small text-ink-2">
        <p>{t.libraries.liveTv}</p>
        <p className="mt-1">{t.libraries.guideAfterSave}</p>
      </div>
    )
  }
  const failing = failingGuides(library).length
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-small">
      <span className="text-ink-2">{guideSummary(t, language, library)}</span>
      {failing > 0 && (
        <span className="flex items-center gap-1.5 text-warn">
          <WarningIcon size="1.125rem" aria-hidden="true" className="shrink-0" />
          {t.livetv.failing(failing)}
        </span>
      )}
      <Link
        to={guidesLink(scope, library, iptv)}
        aria-label={iptv ? t.livetv.lineupLabel(name) : t.livetv.manageLabel(name)}
        className="focus-lit rounded-control font-medium text-link transition-colors duration-300 ease-nuit hover:text-link-hover"
      >
        {iptv ? t.livetv.lineup : t.livetv.manage}
        <CaretRightIcon size="1rem" aria-hidden="true" className="ml-1 inline align-[-2px]" />
      </Link>
    </div>
  )
}

function AvailableRow({
  library,
  addId,
  content,
  onAdd,
}: {
  library: Library
  addId: string
  /** What the row holds when its addon is a music addon. */
  content: MusicContent | undefined
  onAdd: () => void
}) {
  const { t } = useI18n()
  return (
    <Row
      title={library.catalogName}
      muted={!library.browsable}
      titleAside={<MusicKind content={content} />}
      meta={
        <span className="break-words">
          {stremioLabel(t.stremioTypes, library.catalogType)}
          {!library.browsable && ` · ${t.libraries.notBrowsableHelp}`}
        </span>
      }
      trailing={
        library.browsable ? (
          <Button
            id={addId}
            size="sm"
            icon={PlusIcon}
            aria-label={t.libraries.addLabel(library.catalogName)}
            onClick={onAdd}
          >
            {t.libraries.add}
          </Button>
        ) : (
          <Badge>{t.libraries.notBrowsable}</Badge>
        )
      }
    />
  )
}

/** The library a music addon's row becomes in apps, with a note icon; nothing for other rows. */
function MusicKind({ content }: { content: MusicContent | undefined }) {
  const { t } = useI18n()
  if (content === undefined) return null
  return (
    <Badge className="gap-1">
      <MusicNoteIcon size="0.875rem" aria-hidden="true" />
      {t.libraries.music.library[content] ?? t.libraries.music.library.music}
    </Badge>
  )
}
