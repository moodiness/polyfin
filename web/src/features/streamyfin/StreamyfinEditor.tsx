import { PlusIcon, RowsIcon, SparkleIcon, XIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import {
  ApiError,
  fetchStreamyfinCollections,
  fetchStreamyfinHome,
  MAX_STREAMYFIN_ROWS,
  queryClient,
  queryKeys,
  saveStreamyfinHome,
  type StreamyfinLibrary,
  type StreamyfinRow,
  type StreamyfinRowKind,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  cx,
  EmptyState,
  Field,
  IconButton,
  InlineError,
  MoveButtons,
  Notice,
  SaveBar,
  Select,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
} from '@/ui'

/** The longest row title the server accepts. */
const titleMaxLength = 64

/**
 * One row being edited. `title` is the text of its title field: empty = `defaultName`.
 * `libraryName` and `available` come from the server for saved rows; a row added here is available.
 */
type Entry = {
  key: string
  kind: StreamyfinRowKind
  itemId: string | null
  title: string
  defaultName: string
  libraryName: string | null
  available: boolean
}

/** A row's identity: one row per kind for resume and nextUp, one per item for the others. */
function rowKey(kind: StreamyfinRowKind, itemId: string | null): string {
  return `${kind}:${itemId ?? ''}`
}

/** 52 px between the blocks of the editor (44 on a phone), as PageLayout spaces a page's. */
const blockSpacing = '[&>*+*]:mt-block max-md:[&>*+*]:mt-11'

/** The columns of a row: position, row, title in Streamyfin, buttons. */
const rowGrid =
  'grid grid-cols-[22px_minmax(0,1fr)_auto] gap-x-4 md:grid-cols-[22px_minmax(0,1fr)_minmax(0,300px)_auto]'

/** A list too long for the page scrolls in its own box, about 480 px tall, with a thin scrollbar. */
const scrollBox =
  'max-h-[480px] overflow-y-auto overscroll-contain [scrollbar-color:var(--color-line-3)_transparent] [scrollbar-width:thin] [&::-webkit-scrollbar]:w-2'

/** Above this many rows, the list scrolls in its box. */
const boxedRows = 10

/**
 * Chooses the rows of Streamyfin's home screen, in order: continue watching, next up, a library,
 * or the titles of one collection. Edits are staged and saved together with the save bar.
 */
export default function StreamyfinEditor() {
  const { t } = useI18n()
  const home = useQuery({
    queryKey: queryKeys.streamyfin,
    queryFn: ({ signal }) => fetchStreamyfinHome(signal),
  })

  if (home.isPending) {
    return (
      <Block title={t.streamyfin.rowsTitle}>
        <SkeletonRows rows={3} label={t.streamyfin.loading} />
      </Block>
    )
  }
  if (home.isError) {
    return (
      <Block title={t.streamyfin.rowsTitle}>
        <InlineError onRetry={() => void home.refetch()} retrying={home.isFetching}>
          {errorMessage(t, home.error)}
        </InlineError>
      </Block>
    )
  }
  return <HomeForm rows={home.data.rows} libraries={home.data.libraries} />
}

function HomeForm({ rows, libraries }: { rows: StreamyfinRow[]; libraries: StreamyfinLibrary[] }) {
  const { t } = useI18n()
  const toast = useToast()
  const baseId = useId()
  const [draft, setDraft] = useState<Entry[] | null>(null)
  const [announcement, setAnnouncement] = useState('')
  // Element ids to try, in order, once the next render is committed (the focused row may be gone).
  const focusAfterRender = useRef<string[]>([])
  // The row to bring into view in its box once the next render is committed, after a move.
  const scrollAfterRender = useRef<string | null>(null)

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
        // The add field's id is on its wrapper, around the Select's button.
        const target = element instanceof HTMLDivElement ? element.querySelector('button') : element
        target?.focus()
        return
      }
    }
  })

  const save = useMutation({
    mutationFn: (entries: Entry[]) =>
      saveStreamyfinHome(
        entries.map(({ kind, itemId, title }) => ({
          kind,
          itemId,
          title: title.trim() === '' ? null : title.trim(),
        })),
      ),
    onSuccess: (saved, entries) => {
      queryClient.setQueryData(queryKeys.streamyfin, saved)
      // Keep edits made while saving; otherwise show the saved rows.
      setDraft((current) => (current === entries ? null : current))
      toast(t.streamyfin.saved)
    },
    onError: (error) => {
      // A library or collection went away: fetch the rows to flag it.
      if (error instanceof ApiError && error.code === 'invalid_streamyfin_row') {
        void queryClient.invalidateQueries({ queryKey: queryKeys.streamyfin })
      }
    },
  })

  const serverEntries: Entry[] = rows.map((row) => ({
    key: rowKey(row.kind, row.itemId),
    kind: row.kind,
    itemId: row.itemId,
    title: row.title ?? '',
    defaultName: row.defaultName,
    libraryName: row.libraryName,
    available: row.available,
  }))
  const entries = draft ?? serverEntries
  const dirty = draft !== null
  const full = entries.length >= MAX_STREAMYFIN_ROWS

  const ids = {
    row: (key: string) => `${baseId}-row-${key}`,
    remove: (key: string) => `${baseId}-remove-${key}`,
    add: `${baseId}-add`,
  }

  function edit(next: Entry[]) {
    save.reset()
    // Back to the saved state: drop the draft so refetched data shows again.
    const saved =
      next.length === serverEntries.length &&
      next.every(
        (entry, i) => entry.key === serverEntries[i].key && entry.title === serverEntries[i].title,
      )
    setDraft(saved ? null : next)
  }

  function displayName(entry: Entry): string {
    return entry.title.trim() === '' ? entry.defaultName : entry.title.trim()
  }

  function move(from: number, to: number) {
    const next = [...entries]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    scrollAfterRender.current = ids.row(moved.key)
    edit(next)
    setAnnouncement(t.common.moved(displayName(moved), to + 1, next.length))
  }

  function retitle(index: number, title: string) {
    edit(entries.map((entry, i) => (i === index ? { ...entry, title } : entry)))
  }

  function remove(index: number) {
    const removed = entries[index]
    const next = entries.filter((_, i) => i !== index)
    focusAfterRender.current = [
      ...(index < next.length ? [ids.remove(next[index].key)] : []),
      ...(index > 0 ? [ids.remove(next[index - 1].key)] : []),
      ids.add,
    ]
    edit(next)
    setAnnouncement(t.streamyfin.removedLive(displayName(removed)))
  }

  function add(entry: Entry) {
    // The Add button turns off until the next pick: keep the focus in the add form.
    focusAfterRender.current = [ids.add]
    edit([...entries, entry])
    setAnnouncement(t.streamyfin.addedLive(entry.defaultName, entries.length + 1))
  }

  /** Continue watching, next up, then one row per library, in the server's order. */
  function suggest() {
    const next: Entry[] = [
      ...(['resume', 'nextUp'] as const).map((kind) =>
        newEntry(kind, null, t.streamyfin.kinds[kind]),
      ),
      ...libraries.map((library) => newEntry('library', library.itemId, library.name)),
    ].slice(0, MAX_STREAMYFIN_ROWS)
    focusAfterRender.current = next.length > 0 ? [ids.remove(next[0].key)] : []
    edit(next)
    setAnnouncement(t.streamyfin.suggestedLive(next.length))
  }

  return (
    <form
      className={blockSpacing}
      onSubmit={(event) => {
        event.preventDefault()
        save.mutate(entries)
      }}
    >
      <Block title={t.streamyfin.rowsTitle} count={entries.length}>
        <p className="sr-only" role="status">
          {announcement}
        </p>
        <p className="max-w-[72ch] text-small text-ink-2">{t.streamyfin.rowsHelp}</p>
        <p className="mt-2 mb-5 max-w-[72ch] text-small text-ink-2">{t.streamyfin.refreshHelp}</p>
        {entries.length === 0 ? (
          <EmptyState
            icon={RowsIcon}
            title={t.streamyfin.emptyTitle}
            action={
              <Button icon={SparkleIcon} onClick={suggest}>
                {t.streamyfin.suggest}
              </Button>
            }
          >
            {dirty ? t.streamyfin.emptyDraft : t.streamyfin.empty}
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
              <span>{t.streamyfin.columnRow}</span>
              <span>{t.streamyfin.titleLabel}</span>
              <span />
            </div>
            <ol
              aria-label={t.streamyfin.listLabel}
              className={cx(entries.length > boxedRows && scrollBox)}
            >
              {entries.map((entry, index) => {
                const name = displayName(entry)
                const nameId = `${baseId}-name-${entry.key}`
                const inputId = `${baseId}-title-${entry.key}`
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
                    <div className="min-w-0">
                      <p id={nameId} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="text-[14.5px] font-medium break-words text-ink">
                          {name}
                        </span>
                        <Badge tone="accent">{t.streamyfin.kinds[entry.kind]}</Badge>
                      </p>
                      {(entry.libraryName !== null || !entry.available) && (
                        <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-small text-ink-3">
                          {entry.libraryName !== null && (
                            <span className="break-words">
                              {t.streamyfin.inLibrary(entry.libraryName)}
                            </span>
                          )}
                          {!entry.available && (
                            <StatusPill tone="warn">{t.streamyfin.unavailable}</StatusPill>
                          )}
                        </p>
                      )}
                      {!entry.available && (
                        <p className="mt-1 text-small text-ink-3">{t.streamyfin.unavailableHelp}</p>
                      )}
                    </div>
                    <div className="col-span-2 col-start-2 row-start-2 min-w-0 md:col-span-1 md:col-start-3 md:row-start-1">
                      <label
                        htmlFor={inputId}
                        className="mb-1.5 block text-small font-medium text-ink-2 md:sr-only"
                      >
                        {t.streamyfin.titleLabel}
                      </label>
                      <TextInput
                        id={inputId}
                        size="sm"
                        value={entry.title}
                        onValue={(value) => retitle(index, value)}
                        placeholder={entry.defaultName}
                        aria-describedby={nameId}
                        maxLength={titleMaxLength}
                        autoComplete="off"
                      />
                    </div>
                    <div className="col-start-3 row-start-1 flex items-center gap-1 md:col-start-4">
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
                        label={t.streamyfin.removeLabel(name)}
                        onClick={() => remove(index)}
                      />
                    </div>
                  </li>
                )
              })}
            </ol>
          </div>
        )}
      </Block>

      <Block title={t.streamyfin.addTitle}>
        <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.streamyfin.addHelp}</p>
        {full ? (
          <Notice tone="warn">{t.streamyfin.full(MAX_STREAMYFIN_ROWS)}</Notice>
        ) : (
          <AddRow
            addId={ids.add}
            libraries={libraries}
            shown={new Set(entries.map((entry) => entry.key))}
            onAdd={add}
          />
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

function newEntry(kind: StreamyfinRowKind, itemId: string | null, defaultName: string): Entry {
  return {
    key: rowKey(kind, itemId),
    kind,
    itemId,
    title: '',
    defaultName,
    libraryName: null,
    available: true,
  }
}

/**
 * Picks a new row: continue watching and next up once each, a library, or one collection of a
 * collection library. Rows already in the list are offered but turned off.
 */
function AddRow({
  addId,
  libraries,
  shown,
  onAdd,
}: {
  /** The id of the first field, where the focus goes when the last row is removed. */
  addId: string
  libraries: StreamyfinLibrary[]
  /** The keys of the rows in the list. */
  shown: Set<string>
  onAdd: (entry: Entry) => void
}) {
  const { t } = useI18n()
  const [choice, setChoice] = useState<StreamyfinRowKind | null>(null)
  const [libraryId, setLibraryId] = useState('')
  const [collectionLibraryId, setCollectionLibraryId] = useState('')
  const [collectionId, setCollectionId] = useState('')

  const kinds: StreamyfinRowKind[] = ['resume', 'nextUp', 'library', 'collection']
  const kindOptions = kinds.map((kind) => {
    const taken = (kind === 'resume' || kind === 'nextUp') && shown.has(rowKey(kind, null))
    return {
      value: kind,
      label: taken ? t.streamyfin.added(t.streamyfin.choices[kind]) : t.streamyfin.choices[kind],
      disabled: taken,
    }
  })
  // The chosen kind, or the first one still free: continue watching and next up go once added.
  const kind =
    kindOptions.find((option) => option.value === choice && !option.disabled)?.value ??
    kindOptions.find((option) => !option.disabled)?.value ??
    'library'

  const collectionLibraries = libraries.filter((library) => library.collections)
  const collectionLibrary = collectionLibraries.find(
    (library) => library.itemId === collectionLibraryId,
  )
  const collections = useQuery({
    queryKey: queryKeys.streamyfinCollections(collectionLibraryId),
    queryFn: ({ signal }) => fetchStreamyfinCollections(collectionLibraryId, signal),
    enabled: kind === 'collection' && collectionLibrary !== undefined,
  })

  const library = libraries.find(
    (item) => item.itemId === libraryId && !shown.has(rowKey('library', item.itemId)),
  )
  const collection = collections.data?.find(
    (item) => item.itemId === collectionId && !shown.has(rowKey('collection', item.itemId)),
  )

  // What the Add button adds, once every field it needs is filled.
  let entry: Entry | null = null
  if (kind === 'resume' || kind === 'nextUp') {
    entry = newEntry(kind, null, t.streamyfin.kinds[kind])
  } else if (kind === 'library' && library !== undefined) {
    entry = newEntry('library', library.itemId, library.name)
  } else if (kind === 'collection' && collectionLibrary !== undefined && collection !== undefined) {
    entry = {
      ...newEntry('collection', collection.itemId, collection.name),
      libraryName: collectionLibrary.name,
    }
  }

  function itemOption(item: { itemId: string; name: string }, rowKind: StreamyfinRowKind) {
    const taken = shown.has(rowKey(rowKind, item.itemId))
    return {
      value: item.itemId,
      label: taken ? t.streamyfin.added(item.name) : item.name,
      disabled: taken,
    }
  }

  return (
    <div>
      <div className="grid items-start gap-4 md:grid-cols-3">
        <div id={addId}>
          <Field label={t.streamyfin.shows} help={t.streamyfin.choiceHelp[kind]}>
            <Select value={kind} options={kindOptions} onValue={setChoice} />
          </Field>
        </div>

        {kind === 'library' &&
          (libraries.length === 0 ? (
            <p className="text-small text-ink-3 md:pt-8">{t.streamyfin.noLibraries}</p>
          ) : (
            <Field label={t.streamyfin.library}>
              <Select
                value={library?.itemId ?? ''}
                options={[
                  { value: '', label: t.streamyfin.chooseLibrary, disabled: true },
                  ...libraries.map((item) => itemOption(item, 'library')),
                ]}
                onValue={setLibraryId}
              />
            </Field>
          ))}

        {kind === 'collection' &&
          (collectionLibraries.length === 0 ? (
            <p className="text-small text-ink-3 md:pt-8">{t.streamyfin.noCollectionLibraries}</p>
          ) : (
            <>
              <Field label={t.streamyfin.collectionLibrary}>
                <Select
                  value={collectionLibrary?.itemId ?? ''}
                  options={[
                    { value: '', label: t.streamyfin.chooseLibrary, disabled: true },
                    ...collectionLibraries.map((item) => ({
                      value: item.itemId,
                      label: item.name,
                    })),
                  ]}
                  onValue={(value) => {
                    setCollectionLibraryId(value)
                    setCollectionId('')
                  }}
                />
              </Field>
              {collectionLibrary !== undefined &&
                (collections.isPending ? (
                  <Field label={t.streamyfin.collection}>
                    <Select
                      value=""
                      options={[{ value: '', label: t.streamyfin.collectionsLoading }]}
                      onValue={() => {}}
                      disabled
                    />
                  </Field>
                ) : collections.isError ? (
                  <InlineError
                    onRetry={() => void collections.refetch()}
                    retrying={collections.isFetching}
                  >
                    {errorMessage(t, collections.error)}
                  </InlineError>
                ) : collections.data.length === 0 ? (
                  <p className="text-small text-ink-3 md:pt-8">{t.streamyfin.noCollections}</p>
                ) : (
                  <Field label={t.streamyfin.collection}>
                    <Select
                      value={collection?.itemId ?? ''}
                      options={[
                        { value: '', label: t.streamyfin.chooseCollection, disabled: true },
                        ...collections.data.map((item) => itemOption(item, 'collection')),
                      ]}
                      onValue={setCollectionId}
                    />
                  </Field>
                ))}
            </>
          ))}
      </div>
      <Button
        className="mt-4"
        icon={PlusIcon}
        disabled={entry === null}
        onClick={() => {
          if (entry === null) return
          onAdd(entry)
          setLibraryId('')
          setCollectionId('')
        }}
      >
        {t.streamyfin.add}
      </Button>
    </div>
  )
}
