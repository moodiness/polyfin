import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  ApiError,
  fetchAddons,
  fetchLibraries,
  queryClient,
  queryKeys,
  refreshGuide,
  saveGuide,
  saveLibraries,
  type Addon,
  type Library,
  type Scope,
} from '@/api'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  Card,
  Loading,
  MoveButtons,
  Notice,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'

/** Above this many libraries the home screen of Jellyfin apps gets heavy; the server default. */
const recommendedLibraries = 20

/** One enabled library being edited. `name` is the text of its name field: empty = catalog name. */
type Entry = { key: string; library: Library; name: string }

function catalogKey(library: Pick<Library, 'addonId' | 'catalogType' | 'catalogId'>): string {
  return JSON.stringify([library.addonId, library.catalogType, library.catalogId])
}

function sameEntries(a: Entry[], b: Entry[]): boolean {
  return (
    a.length === b.length &&
    a.every((entry, i) => entry.key === b[i].key && entry.name === b[i].name)
  )
}

const fieldClass =
  'mt-1.5 block w-full rounded-lg border border-line bg-ink px-3 py-2 text-white placeholder:text-zinc-500'

/**
 * Chooses which catalogs of a scope's addons are libraries in Jellyfin apps, in which order and
 * under which name. Edits stay local until saved.
 */
export default function LibraryEditor({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const libraries = useQuery({
    queryKey: queryKeys.libraries(scope),
    queryFn: ({ signal }) => fetchLibraries(scope, signal),
  })
  // Only used to flag libraries of turned-off addons; the editor works without it.
  const addons = useQuery({
    queryKey: queryKeys.addons(scope),
    queryFn: ({ signal }) => fetchAddons(scope, signal),
  })

  if (libraries.isPending || libraries.isError) {
    return (
      <Card title={t.libraries.shownTitle}>
        {libraries.isPending ? (
          <Loading />
        ) : (
          <Notice kind="error">{errorMessage(t, libraries.error)}</Notice>
        )}
      </Card>
    )
  }
  return <LibraryForm scope={scope} catalogs={libraries.data} addons={addons.data ?? []} />
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
  const { t } = useI18n()
  const baseId = useId()
  const [draft, setDraft] = useState<Entry[] | null>(null)
  const [query, setQuery] = useState('')
  const [type, setType] = useState('')
  const [announcement, setAnnouncement] = useState('')
  // Element ids to try, in order, once the next render is committed (the focused row may be gone).
  const focusAfterRender = useRef<string[]>([])

  useEffect(() => {
    const candidates = focusAfterRender.current
    if (candidates.length === 0) return
    focusAfterRender.current = []
    for (const id of candidates) {
      const element = document.getElementById(id)
      if (element !== null) {
        element.focus()
        return
      }
    }
  })

  const save = useMutation({
    mutationFn: (entries: Entry[]) =>
      saveLibraries(
        scope,
        entries.map(({ library, name }) => ({
          addonId: library.addonId,
          catalogType: library.catalogType,
          catalogId: library.catalogId,
          name: name.trim() === '' ? null : name.trim(),
        })),
      ),
    onSuccess: (saved, entries) => {
      queryClient.setQueryData(queryKeys.libraries(scope), saved)
      // Keep edits made while saving; otherwise show the saved list.
      setDraft((current) => (current === entries ? null : current))
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
    .map((library) => ({ key: catalogKey(library), library, name: library.name ?? '' }))
  const entries = draft ?? serverEntries
  const dirty = draft !== null
  const shownKeys = new Set(entries.map((entry) => entry.key))
  const offAddons = new Set(addons.filter((addon) => !addon.enabled).map((addon) => addon.id))

  const types = [...new Set(catalogs.map((library) => library.catalogType))]
  const needle = query.trim().toLocaleLowerCase()
  const available = catalogs.filter(
    (library) =>
      !shownKeys.has(catalogKey(library)) &&
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
  const nothingAvailable = catalogs.every((library) => shownKeys.has(catalogKey(library)))

  const ids = {
    remove: (key: string) => `${baseId}-remove-${key}`,
    add: (key: string) => `${baseId}-add-${key}`,
    filter: `${baseId}-filter`,
    type: `${baseId}-type`,
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
    edit(next)
    setAnnouncement(t.common.moved(displayName(moved), to + 1, next.length))
  }

  function rename(index: number, name: string) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, name } : entry)))
  }

  function remove(index: number) {
    const removed = entries[index]
    const next = entries.filter((_, i) => i !== index)
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
    ]
    edit([...entries, { key, library, name: '' }])
    setAnnouncement(t.libraries.addedLive(library.catalogName, entries.length + 1))
  }

  if (catalogs.length === 0 && entries.length === 0) {
    return (
      <Card title={t.libraries.shownTitle}>
        <p className="text-sm text-muted">{t.libraries.noAddons}</p>
      </Card>
    )
  }

  return (
    <div className="space-y-6">
      <p className="sr-only" role="status">
        {announcement}
      </p>

      <Card title={t.libraries.shownTitle}>
        <div className="space-y-4">
          <p className="text-sm text-muted">
            {scope === 'shared' ? t.libraries.shownHelp : t.libraries.shownHelpMine}
          </p>
          <p className="text-sm text-muted">{t.libraries.defaultHelp}</p>
          <p className="text-sm font-medium text-zinc-200">{t.libraries.count(entries.length)}</p>
          {entries.length > recommendedLibraries && (
            <div className="flex gap-2 rounded-lg border border-amber-400/40 bg-amber-400/5 p-3 text-sm text-amber-100">
              <span aria-hidden="true">⚠</span>
              <p>{t.libraries.manyWarning}</p>
            </div>
          )}
          {entries.length === 0 ? (
            <p className="text-sm text-muted">{t.libraries.shownEmpty}</p>
          ) : (
            <ol className="divide-y divide-line rounded-xl border border-line">
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
                return (
                  <li
                    key={entry.key}
                    className="flex flex-col gap-3 p-4 sm:flex-row sm:items-end sm:justify-between"
                  >
                    <div className="min-w-0 flex-1">
                      <p id={titleId} className="flex flex-wrap items-center gap-2 text-sm">
                        <span aria-hidden="true" className="text-muted tabular-nums">
                          {index + 1}.
                        </span>
                        <span className="font-medium break-words text-white">
                          {library.catalogName}
                        </span>
                        <Badge tone="muted">{library.addonName}</Badge>
                        <Badge tone="fin">
                          {stremioLabel(t.stremioTypes, library.catalogType)}
                        </Badge>
                        {offAddons.has(library.addonId) && (
                          <Badge tone="danger">{t.libraries.addonOff}</Badge>
                        )}
                        {missing && <Badge tone="danger">{t.libraries.missing}</Badge>}
                      </p>
                      {library.catalogType === 'tv' ? (
                        <>
                          <p className="mt-2 text-xs text-muted">{t.libraries.liveTv}</p>
                          {library.guide !== null ? (
                            <GuidePanel scope={scope} library={library} name={name} />
                          ) : (
                            <p className="mt-2 text-xs text-muted">{t.libraries.guideAfterSave}</p>
                          )}
                        </>
                      ) : (
                        <>
                          <label
                            htmlFor={inputId}
                            className="mt-2 block text-xs font-medium text-zinc-300"
                          >
                            {t.libraries.name}
                          </label>
                          <input
                            id={inputId}
                            value={entry.name}
                            onChange={(event) => rename(index, event.target.value)}
                            placeholder={appName ?? library.catalogName}
                            aria-describedby={renamed ? `${titleId} ${appNameId}` : titleId}
                            maxLength={64}
                            autoComplete="off"
                            className={fieldClass}
                          />
                          {renamed && (
                            <p id={appNameId} className="mt-1 text-xs text-muted">
                              {t.libraries.appName(appName)}
                            </p>
                          )}
                        </>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      <MoveButtons
                        name={name}
                        index={index}
                        count={entries.length}
                        onMove={(to) => move(index, to)}
                      />
                      <button
                        id={ids.remove(entry.key)}
                        type="button"
                        className={buttonSecondary}
                        aria-label={t.libraries.removeLabel(name)}
                        onClick={() => remove(index)}
                      >
                        {t.libraries.remove}
                      </button>
                    </div>
                  </li>
                )
              })}
            </ol>
          )}
        </div>
      </Card>

      <Card title={t.libraries.availableTitle}>
        <div className="space-y-4">
          <p className="text-sm text-muted">{t.libraries.availableHelp}</p>
          {nothingAvailable ? (
            <p className="text-sm text-muted">{t.libraries.availableEmpty}</p>
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-[1fr_minmax(10rem,auto)]">
                <div>
                  <label htmlFor={ids.filter} className="block text-sm font-medium text-zinc-200">
                    {t.libraries.filter}
                  </label>
                  <input
                    id={ids.filter}
                    type="search"
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    placeholder={t.libraries.filterPlaceholder}
                    autoComplete="off"
                    className={fieldClass}
                  />
                </div>
                <div>
                  <label htmlFor={ids.type} className="block text-sm font-medium text-zinc-200">
                    {t.libraries.type}
                  </label>
                  <select
                    id={ids.type}
                    value={type}
                    onChange={(event) => setType(event.target.value)}
                    className={fieldClass}
                  >
                    <option value="">{t.libraries.allTypes}</option>
                    {types.map((value) => (
                      <option key={value} value={value}>
                        {stremioLabel(t.stremioTypes, value)}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              {groups.size === 0 ? (
                <p className="text-sm text-muted">{t.libraries.noMatch}</p>
              ) : (
                [...groups].map(([addonId, items]) => {
                  const headingId = `${baseId}-addon-${addonId}`
                  return (
                    <section key={addonId} aria-labelledby={headingId}>
                      <h3
                        id={headingId}
                        className="mb-2 flex flex-wrap items-center gap-2 text-sm font-semibold text-white"
                      >
                        <span className="break-words">{items[0].addonName}</span>
                        {offAddons.has(addonId) && (
                          <Badge tone="danger">{t.libraries.addonOff}</Badge>
                        )}
                      </h3>
                      <ul className="divide-y divide-line rounded-xl border border-line">
                        {items.map((library) => (
                          <AvailableRow
                            key={catalogKey(library)}
                            library={library}
                            addId={ids.add(catalogKey(library))}
                            onAdd={() => add(library)}
                          />
                        ))}
                      </ul>
                    </section>
                  )
                })
              )}
            </>
          )}
        </div>
      </Card>

      <div className="sticky bottom-0 z-10 space-y-3 rounded-2xl border border-line bg-surface/95 p-4 backdrop-blur">
        <p
          role="status"
          className={`flex items-center gap-2 text-sm font-medium ${dirty ? 'text-amber-200' : 'text-muted'}`}
        >
          <span aria-hidden="true">{dirty ? '●' : '✓'}</span>
          {dirty ? t.libraries.unsaved : t.libraries.upToDate}
        </p>
        {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
        {save.isSuccess && <Notice kind="success">{t.libraries.saved}</Notice>}
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className={buttonPrimary}
            disabled={save.isPending}
            onClick={() => save.mutate(entries)}
          >
            {save.isPending ? t.common.saving : t.common.save}
          </button>
          <button
            type="button"
            className={buttonSecondary}
            disabled={save.isPending}
            onClick={() => {
              save.reset()
              setDraft(null)
            }}
          >
            {t.libraries.reset}
          </button>
        </div>
      </div>
    </div>
  )
}

/** The XMLTV guide of a saved TV catalog: its address, how its last fetch went, and actions. */
function GuidePanel({ scope, library, name }: { scope: Scope; library: Library; name: string }) {
  const { t } = useI18n()
  const titleId = useId()
  const [editing, setEditing] = useState(false)
  const [url, setUrl] = useState('')
  const guide = library.guide
  const target = {
    addonId: library.addonId,
    catalogType: library.catalogType,
    catalogId: library.catalogId,
  }
  const save = useMutation({
    mutationFn: (value: string) => saveGuide(scope, target, value),
    onSuccess: (saved) => {
      queryClient.setQueryData(queryKeys.libraries(scope), saved)
      setEditing(false)
      setUrl('')
    },
  })
  const refresh = useMutation({
    mutationFn: () => refreshGuide(scope, target),
    onSuccess: (saved) => queryClient.setQueryData(queryKeys.libraries(scope), saved),
  })
  if (guide === null) return null
  const busy = save.isPending || refresh.isPending

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    refresh.reset()
    save.mutate(url.trim())
  }

  return (
    <section
      aria-labelledby={titleId}
      className="mt-3 space-y-2 rounded-lg border border-line bg-ink/40 p-3 text-sm"
    >
      <h4 id={titleId} className="font-medium text-zinc-200">
        {t.libraries.guideTitle}
      </h4>
      <p className="text-xs text-muted">{t.libraries.guideHelp}</p>
      {guide.url === '' ? (
        <p className="text-zinc-300">{t.libraries.guideNone}</p>
      ) : (
        <div className="space-y-1">
          <p className="font-mono text-xs break-all text-zinc-300">{guide.url}</p>
          <p className="text-xs text-muted">
            {guide.fetchedAt === null ? (
              t.libraries.guideNever
            ) : (
              <>
                {t.libraries.guideFetched} <RelativeTime iso={guide.fetchedAt} />.{' '}
                {t.libraries.guideMatched(guide.matched, guide.channels)}
              </>
            )}
          </p>
          {guide.nextAt !== null && (
            <p className="text-xs text-muted">
              {t.libraries.guideNext} <RelativeTime iso={guide.nextAt} />.
            </p>
          )}
          {guide.error !== '' && (
            <Notice kind="error">
              {Object.hasOwn(t.libraries.guideErrors, guide.error)
                ? t.libraries.guideErrors[guide.error]
                : t.errors.generic}
            </Notice>
          )}
        </div>
      )}
      {editing && (
        <form onSubmit={submit} noValidate className="space-y-2">
          <TextField
            label={t.libraries.guideAddress}
            hint={t.libraries.guideAddressHint}
            type="url"
            inputMode="url"
            value={url}
            onValue={(value) => {
              save.reset()
              setUrl(value)
            }}
            placeholder={t.libraries.guidePlaceholder}
            autoComplete="off"
            spellCheck={false}
            autoFocus
            required
          />
          <div className="flex flex-wrap gap-2">
            <button type="submit" className={buttonPrimary} disabled={busy || url.trim() === ''}>
              {save.isPending ? t.libraries.guideFetching : t.libraries.guideSave}
            </button>
            <button
              type="button"
              className={buttonSecondary}
              disabled={save.isPending}
              onClick={() => {
                save.reset()
                setEditing(false)
                setUrl('')
              }}
            >
              {t.common.cancel}
            </button>
          </div>
        </form>
      )}
      {!editing && (
        <div className="flex flex-wrap gap-2">
          {guide.url !== '' && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={busy}
              aria-label={t.libraries.guideRefreshLabel(name)}
              onClick={() => {
                save.reset()
                refresh.mutate()
              }}
            >
              {refresh.isPending ? t.libraries.guideFetching : t.libraries.guideRefresh}
            </button>
          )}
          <button
            type="button"
            className={buttonSecondary}
            disabled={busy}
            onClick={() => {
              save.reset()
              refresh.reset()
              setEditing(true)
            }}
          >
            {guide.url === '' ? t.libraries.guideAdd : t.libraries.guideChange}
          </button>
          {guide.url !== '' && (
            <button
              type="button"
              className={buttonSecondary}
              disabled={busy}
              onClick={() => {
                refresh.reset()
                save.mutate('')
              }}
            >
              {t.libraries.guideRemove}
            </button>
          )}
        </div>
      )}
      {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
      {refresh.isError && <Notice kind="error">{errorMessage(t, refresh.error)}</Notice>}
      {save.isSuccess && save.variables === '' && (
        <Notice kind="success">{t.libraries.guideRemoved}</Notice>
      )}
    </section>
  )
}

function AvailableRow({
  library,
  addId,
  onAdd,
}: {
  library: Library
  addId: string
  onAdd: () => void
}) {
  const { t } = useI18n()

  return (
    <li className="flex flex-wrap items-center justify-between gap-3 p-3">
      <div className="min-w-0">
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <span
            className={`font-medium break-words ${library.browsable ? 'text-white' : 'text-zinc-300'}`}
          >
            {library.catalogName}
          </span>
          <Badge tone="fin">{stremioLabel(t.stremioTypes, library.catalogType)}</Badge>
          {!library.browsable && <Badge tone="muted">{t.libraries.notBrowsable}</Badge>}
        </p>
        {!library.browsable && (
          <p className="mt-1 text-xs text-muted">{t.libraries.notBrowsableHelp}</p>
        )}
      </div>
      {library.browsable && (
        <button
          id={addId}
          type="button"
          className={`${buttonSecondary} shrink-0`}
          aria-label={t.libraries.addLabel(library.catalogName)}
          onClick={onAdd}
        >
          {t.libraries.add}
        </button>
      )}
    </li>
  )
}
