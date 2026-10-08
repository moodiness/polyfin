import {
  CaretRightIcon,
  FilmStripIcon,
  FunnelSimpleIcon,
  MagnifyingGlassIcon,
  MusicNoteIcon,
  PlusIcon,
  SquaresFourIcon,
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
  saveLibraries,
  type Addon,
  type Library,
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
  Checkbox,
  cx,
  EmptyState,
  Field,
  InlineError,
  Notice,
  Row,
  RowList,
  SaveBar,
  Select,
  Skeleton,
  NumberInput,
  StatusPill,
  TextInput,
  IconButton,
  useToast,
  MoveButtons,
} from '@/ui'
import { LibraryImageEditor, LibraryThumb, LiveTvThumb } from './LibraryImage'

/** Above this many libraries the home screen of Jellyfin apps gets heavy; the server default. */
const recommendedLibraries = 20

/** The longest library name the server accepts. */
const nameMaxLength = 64

/**
 * One enabled library being edited. `name` is the text of its name field: empty = catalog name.
 * `genre` and `maxItems` narrow it; null for none. `hideInMenus` leaves it out of the web
 * player's menus.
 */
type Entry = {
  key: string
  library: Library
  name: string
  genre: string | null
  maxItems: number | null
  hideInMenus: boolean
}

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
        entry.key === b[i].key &&
        entry.name === b[i].name &&
        entry.genre === b[i].genre &&
        entry.maxItems === b[i].maxItems &&
        entry.hideInMenus === b[i].hideInMenus,
    )
  )
}

/** 52 px between the blocks of the editor (44 on a phone), as PageLayout spaces a page's. */
const blockSpacing = '[&>*+*]:mt-block max-md:[&>*+*]:mt-11'

/** The columns of a library row: position, image and catalog, name in apps, buttons. */
const rowGrid =
  'grid grid-cols-[22px_minmax(0,1fr)_auto] gap-x-4 md:grid-cols-[22px_minmax(0,1fr)_minmax(0,300px)_auto]'

/**
 * A list too long for the page scrolls in its own box, about 480 px tall, with a thin scrollbar,
 * so the blocks below and the save bar stay close.
 */
const scrollBox =
  'max-h-[480px] overflow-y-auto overscroll-contain [scrollbar-color:var(--color-line-3)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:w-2'

/** Above this many libraries, the list of those shown scrolls in its box. */
const boxedLibraries = 10

/**
 * Chooses which catalogs of a scope's addons are libraries in Jellyfin apps, in which order and
 * under which name. Edits stay local until saved with the save bar at the bottom.
 *
 * It renders blocks (the libraries, the available catalogs, the save bar) inside one form, so place
 * it in a PageLayout or among a page's blocks: `<LibraryEditor scope="me" />`.
 */
export default function LibraryEditor({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const libraries = useQuery({
    queryKey: queryKeys.libraries(scope),
    queryFn: ({ signal }) => fetchLibraries(scope, signal),
  })
  // Only used to flag libraries of turned-off addons and music or IPTV rows; it works without.
  const addons = useQuery({
    queryKey: queryKeys.addons(scope),
    queryFn: ({ signal }) => fetchAddons(scope, signal),
  })

  if (libraries.isPending) {
    return (
      <Block title={t.libraries.shownTitle}>
        <LibrariesSkeleton />
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

function LibrariesSkeleton() {
  const { t } = useI18n()
  return (
    <div
      role="status"
      aria-label={t.libraries.loading}
      className="rounded-panel border border-line-2 bg-s1"
    >
      {Array.from({ length: 4 }, (_, index) => (
        <div
          key={index}
          className={cx(
            rowGrid,
            'items-center px-[18px] py-4',
            index > 0 && 'border-t border-line',
          )}
        >
          <Skeleton className="h-3 w-3" />
          <span className="flex gap-3.5">
            <Skeleton className="aspect-video w-14 shrink-0 sm:w-[72px]" />
            <span className="flex flex-1 flex-col gap-2 pt-1">
              <Skeleton className="h-3.5 w-2/5" />
              <Skeleton className="h-3 w-1/4" />
            </span>
          </span>
          <Skeleton className="hidden h-[34px] w-full md:block" />
          <Skeleton className="h-7 w-24" />
        </div>
      ))}
    </div>
  )
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
  // The entry whose genre and maximum editor is open, below its row; never with its image editor.
  const [narrowOpen, setNarrowOpen] = useState<string | null>(null)
  // Element ids to try, in order, once the next render is committed (the focused row may be gone).
  const focusAfterRender = useRef<string[]>([])
  // The row to bring into view in its box once the next render is committed, after a move.
  const scrollAfterRender = useRef<string | null>(null)
  // The box of available catalogs, back at its top when the filter changes.
  const availableBox = useRef<HTMLDivElement>(null)
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
        // The filter's id is on its field, whose label names the input inside.
        const target = element instanceof HTMLDivElement ? element.querySelector('input') : element
        target?.focus()
        return
      }
    }
  })

  const save = useMutation({
    mutationFn: (entries: Entry[]) =>
      saveLibraries(
        scope,
        entries.map(({ library, name, genre, maxItems, hideInMenus }) => ({
          addonId: library.addonId,
          catalogType: library.catalogType,
          catalogId: library.catalogId,
          name: name.trim() === '' ? null : name.trim(),
          genre,
          maxItems,
          hideInMenus,
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
    }))
  const entries = draft ?? serverEntries
  const dirty = draft !== null
  const anyFilterable = entries.some((entry) => (byKey.get(entry.key) ?? entry.library).filterable)
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
    narrow: (key: string) => `${baseId}-narrow-${key}`,
    narrowButton: (key: string) => `${baseId}-narrow-button-${key}`,
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

  function rename(index: number, name: string) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, name } : entry)))
  }

  function narrow(index: number, change: Partial<Pick<Entry, 'genre' | 'maxItems'>>) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, ...change } : entry)))
  }

  function hideInMenus(index: number, hidden: boolean) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, hideInMenus: hidden } : entry)))
  }

  /** Opens the genre and maximum editor of `key`, or closes it with null, and the image editor. */
  function openNarrow(key: string | null) {
    setImageOpen(null)
    setNarrowOpen(key)
  }

  function remove(index: number) {
    const removed = entries[index]
    const next = entries.filter((_, i) => i !== index)
    if (imageOpen === removed.key) setImageOpen(null)
    if (narrowOpen === removed.key) setNarrowOpen(null)
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
    edit([...entries, { key, library, name: '', genre: null, maxItems: null, hideInMenus: false }])
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
        // A maximum the server refuses: show it in its row's editor rather than save.
        const invalid = entries.find((entry) => !validMax(entry.maxItems))
        if (invalid !== undefined) {
          focusAfterRender.current = [ids.maxItems(invalid.key)]
          if (narrowOpen === invalid.key) {
            document.getElementById(ids.maxItems(invalid.key))?.querySelector('input')?.focus()
          } else {
            openNarrow(invalid.key)
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
        <p className="mb-5 max-w-[72ch] text-small text-ink-2">
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
          <div className="overflow-hidden rounded-panel border border-line-2 bg-s1">
            <div
              aria-hidden="true"
              className={cx(
                rowGrid,
                'border-b border-line px-[18px] py-2.5 text-micro font-medium text-ink-3 max-md:hidden',
              )}
            >
              <span>#</span>
              <span>{t.libraries.columnCatalog}</span>
              <span>{t.libraries.name}</span>
              <span />
            </div>
            <ol
              aria-label={t.libraries.listLabel}
              className={cx(entries.length > boxedLibraries && scrollBox)}
            >
              {entries.map((entry, index) => {
                const library = byKey.get(entry.key) ?? entry.library
                const missing = !(byKey.get(entry.key)?.browsable ?? false)
                const name = displayName(entry)
                const titleId = `${baseId}-title-${entry.key}`
                const inputId = `${baseId}-name-${entry.key}`
                const appNameId = `${baseId}-app-name-${entry.key}`
                // The server names saved libraries only: edits may change what apps show.
                const appName = dirty ? null : library.appName
                const renamed = appName !== null && appName !== name
                const music = musicContent.get(library.addonId)
                const iptv = iptvAddons.has(library.addonId)
                const tv = isTvCatalog(library)
                const imageShown = !tv && imageOpen === entry.key
                const narrowShown = library.filterable && narrowOpen === entry.key
                return (
                  <li
                    key={entry.key}
                    id={ids.row(entry.key)}
                    className={cx(
                      rowGrid,
                      'items-start gap-y-3 px-[18px] py-3.5 not-first:border-t not-first:border-line max-sm:px-4',
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className="pt-[3px] font-mono text-small text-ink-3 tabular-nums"
                    >
                      {index + 1}
                    </span>
                    <div className="flex min-w-0 items-start gap-3 sm:gap-3.5">
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
                            setNarrowOpen(null)
                            setImageOpen(imageShown ? null : entry.key)
                          }}
                        />
                      )}
                      <div className="min-w-0">
                        <p id={titleId} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                          <span className="text-[14.5px] font-medium break-words text-ink">
                            {library.catalogName}
                          </span>
                          <Badge tone="accent">
                            {stremioLabel(t.stremioTypes, library.catalogType)}
                          </Badge>
                          <MusicKind content={music} />
                          {entry.genre !== null && <Badge>{entry.genre}</Badge>}
                          {entry.maxItems !== null && (
                            <Badge tone={validMax(entry.maxItems) ? 'neutral' : 'danger'}>
                              {t.libraries.narrow.maxBadge(entry.maxItems.toLocaleString(language))}
                            </Badge>
                          )}
                        </p>
                        <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-ink-3">
                          <span className="break-words">{library.addonName}</span>
                          {offAddons.has(library.addonId) && (
                            <StatusPill tone="danger">{t.libraries.addonOff}</StatusPill>
                          )}
                          {missing && <StatusPill tone="danger">{t.libraries.missing}</StatusPill>}
                          {!tv && entry.hideInMenus && (
                            <StatusPill tone="muted">{t.libraries.hiddenInMenus}</StatusPill>
                          )}
                        </p>
                      </div>
                    </div>
                    <div className="col-span-2 col-start-2 row-start-2 min-w-0 md:col-span-1 md:col-start-3 md:row-start-1">
                      {tv ? (
                        <TvSummary scope={scope} library={library} name={name} iptv={iptv} />
                      ) : (
                        <>
                          <label
                            htmlFor={inputId}
                            className="mb-1.5 block text-small font-medium text-ink-2 md:sr-only"
                          >
                            {t.libraries.name}
                          </label>
                          <TextInput
                            id={inputId}
                            size="sm"
                            value={entry.name}
                            onValue={(value) => rename(index, value)}
                            placeholder={appName ?? library.catalogName}
                            aria-describedby={renamed ? `${titleId} ${appNameId}` : titleId}
                            maxLength={nameMaxLength}
                            autoComplete="off"
                          />
                          {renamed && (
                            <p id={appNameId} className="mt-1.5 text-small text-ink-3">
                              {t.libraries.appName(appName)}
                            </p>
                          )}
                          {music !== undefined && (
                            <p className="mt-1.5 text-small text-ink-3">
                              {t.libraries.music.help[music] ?? t.libraries.music.help.music}
                            </p>
                          )}
                          {iptv && (
                            <p className="mt-1.5 text-small text-ink-3">
                              {t.libraries.iptvVod}{' '}
                              <Link
                                to={lineupPath(scope, library.addonId, 'options')}
                                className="rounded-sm font-medium text-link underline decoration-link/40 underline-offset-4 hover:decoration-link"
                              >
                                {t.libraries.iptvVodLink}
                              </Link>
                            </p>
                          )}
                          <Checkbox
                            className="mt-3"
                            checked={entry.hideInMenus}
                            onChange={(hidden) => hideInMenus(index, hidden)}
                            label={t.libraries.hideInMenus}
                            help={entry.hideInMenus ? t.libraries.hideInMenusHelp : undefined}
                          />
                        </>
                      )}
                    </div>
                    <div className="col-start-3 row-start-1 flex items-center gap-1 md:col-start-4">
                      {library.filterable ? (
                        <IconButton
                          id={ids.narrowButton(entry.key)}
                          size="sm"
                          icon={FunnelSimpleIcon}
                          label={t.libraries.narrow.edit(name)}
                          aria-expanded={narrowShown}
                          aria-controls={narrowShown ? ids.narrow(entry.key) : undefined}
                          className={cx(narrowShown && 'bg-ink/8 text-ink')}
                          onClick={() => openNarrow(narrowShown ? null : entry.key)}
                        />
                      ) : (
                        // Keeps the name column as wide as in the rows that have the button.
                        anyFilterable && <span aria-hidden="true" className="size-7 shrink-0" />
                      )}
                      <MoveButtons
                        name={name}
                        index={index}
                        count={entries.length}
                        onMove={(to) => move(index, to)}
                      />
                      <IconButton
                        id={ids.remove(entry.key)}
                        size="sm"
                        icon={XIcon}
                        danger
                        label={t.libraries.removeLabel(name)}
                        onClick={() => remove(index)}
                      />
                    </div>
                    {imageShown && (
                      <div className="col-span-full row-start-3 min-w-0 md:col-span-3 md:col-start-2 md:row-start-2">
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
                    {narrowShown && (
                      <div className="col-span-full row-start-3 min-w-0 md:col-span-3 md:col-start-2 md:row-start-2">
                        <NarrowEditor
                          id={ids.narrow(entry.key)}
                          maxItemsId={ids.maxItems(entry.key)}
                          scope={scope}
                          library={library}
                          entry={entry}
                          name={name}
                          onChange={(change) => narrow(index, change)}
                          onClose={() => {
                            setNarrowOpen(null)
                            focusAfterRender.current = [ids.narrowButton(entry.key)]
                          }}
                        />
                      </div>
                    )}
                  </li>
                )
              })}
            </ol>
          </div>
        )}
        <details className="group mt-5">
          <summary className="inline-flex cursor-pointer list-none items-center gap-1.5 rounded-md text-control font-medium text-ink-2 transition-colors duration-160 hover:text-ink [&::-webkit-details-marker]:hidden">
            <CaretRightIcon
              size={14}
              aria-hidden="true"
              className="transition-transform duration-160 ease-nuit group-open:rotate-90"
            />
            {t.libraries.defaultTitle}
          </summary>
          <p className="mt-2 max-w-[72ch] pl-5 text-small text-ink-3">{t.libraries.defaultHelp}</p>
        </details>
      </Block>

      <Block title={t.libraries.availableTitle} count={notShown.length}>
        <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.libraries.availableHelp}</p>
        {notShown.length === 0 ? (
          <EmptyState icon={SquaresFourIcon} title={t.libraries.availableEmpty} />
        ) : (
          <>
            <div className="mb-4 grid gap-4 sm:grid-cols-[minmax(0,1fr)_220px]">
              <div id={ids.filter}>
                <Field label={t.libraries.filter}>
                  <TextInput
                    type="search"
                    icon={MagnifyingGlassIcon}
                    value={query}
                    onValue={(value) => filterBy(() => setQuery(value))}
                    placeholder={t.libraries.filterPlaceholder}
                    autoComplete="off"
                  />
                </Field>
              </div>
              <Field label={t.libraries.type}>
                <Select
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
              <p role="status" className="text-small text-ink-3">
                {t.libraries.noMatch}
              </p>
            ) : (
              <div
                ref={availableBox}
                className={cx('scroll-pt-12 rounded-panel border border-line-2 bg-s1', scrollBox)}
              >
                {[...groups].map(([addonId, items]) => {
                  const headingId = `${baseId}-addon-${addonId}`
                  return (
                    <section
                      key={addonId}
                      aria-labelledby={headingId}
                      className="not-first:border-t not-first:border-line-2"
                    >
                      <h3
                        id={headingId}
                        className="sticky top-0 z-[1] flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-line bg-s1/95 px-[18px] py-2.5 text-control font-semibold text-ink backdrop-blur-[14px] max-sm:px-4"
                      >
                        <span className="break-words">{items[0].addonName}</span>
                        {offAddons.has(addonId) && (
                          <StatusPill tone="danger">{t.libraries.addonOff}</StatusPill>
                        )}
                      </h3>
                      <RowList
                        variant="plain"
                        className="px-[18px] max-sm:px-4"
                        aria-label={t.libraries.availableLabel(items[0].addonName)}
                      >
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

/**
 * The genre and maximum of a library, below its row: staged like its name and saved with the
 * list. The genre shows only when the catalog offers genres.
 */
function NarrowEditor({
  id,
  maxItemsId,
  scope,
  library,
  entry,
  name,
  onChange,
  onClose,
}: {
  id: string
  /** The id of the box around the maximum's field, to focus it. */
  maxItemsId: string
  scope: Scope
  library: Library
  entry: Entry
  /** The library's name, for the close button's label. */
  name: string
  onChange: (change: Partial<Pick<Entry, 'genre' | 'maxItems'>>) => void
  onClose: () => void
}) {
  const { t, language } = useI18n()
  const text = t.libraries.narrow
  const labelId = `${id}-label`
  const max = MAX_LIBRARY_ITEMS.toLocaleString(language)
  // A genre the catalog no longer offers stays listed, so the choice shows; saving reports it.
  const genres =
    entry.genre === null || library.genres.includes(entry.genre)
      ? library.genres
      : [entry.genre, ...library.genres]
  return (
    <section
      id={id}
      aria-labelledby={labelId}
      className="animate-rise rounded-row border border-line-2 bg-s2/50 p-4 max-sm:p-3.5"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 id={labelId} className="text-control font-medium text-ink">
            {text.title}
          </h3>
          <p className="mt-0.5 text-small text-ink-3">{text.staged}</p>
        </div>
        <IconButton size="sm" icon={XIcon} label={text.close(name)} onClick={onClose} />
      </div>
      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        {genres.length > 0 && (
          <Field label={text.genre} help={text.genreHelp}>
            <Select
              value={entry.genre ?? ''}
              onValue={(value) => onChange({ genre: value === '' ? null : value })}
              options={[
                { value: '', label: text.allGenres },
                ...genres.map((genre) => ({ value: genre, label: genre })),
              ]}
            />
          </Field>
        )}
        <div id={maxItemsId}>
          <Field
            label={text.maxItems}
            help={
              scope === 'shared'
                ? text.maxHelp(max, t.nav.trail(t.nav.settings, t.nav.settingsSections.catalogs))
                : text.maxHelpMine(max)
            }
            error={validMax(entry.maxItems) ? undefined : text.maxRange(max)}
          >
            <NumberInput
              value={entry.maxItems}
              onValue={(value) => onChange({ maxItems: value })}
              min={1}
              max={MAX_LIBRARY_ITEMS}
              step={1}
              inputMode="numeric"
              placeholder={text.noMax}
              className="w-[160px]"
            />
          </Field>
        </div>
      </div>
    </section>
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
      <div className="text-small text-ink-3">
        <p>{t.libraries.liveTv}</p>
        <p className="mt-1">{t.libraries.guideAfterSave}</p>
      </div>
    )
  }
  const failing = failingGuides(library).length
  return (
    <div className="text-small">
      <p className="text-ink-2">{guideSummary(t, language, library)}</p>
      {failing > 0 && (
        <p className="mt-1 flex items-center gap-1.5 text-warn">
          <WarningIcon size={14} aria-hidden="true" className="shrink-0" />
          {t.livetv.failing(failing)}
        </p>
      )}
      <Link
        to={guidesLink(scope, library, iptv)}
        aria-label={iptv ? t.livetv.lineupLabel(name) : t.livetv.manageLabel(name)}
        className="mt-1 inline-flex items-center gap-1 rounded-md font-medium text-link transition-colors duration-160 hover:text-link-hover"
      >
        {iptv ? t.livetv.lineup : t.livetv.manage}
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
      titleAside={
        <>
          <Badge tone="accent">{stremioLabel(t.stremioTypes, library.catalogType)}</Badge>
          <MusicKind content={content} />
        </>
      }
      meta={library.browsable ? undefined : t.libraries.notBrowsableHelp}
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
      <MusicNoteIcon size={12} aria-hidden="true" />
      {t.libraries.music.library[content] ?? t.libraries.music.library.music}
    </Badge>
  )
}
