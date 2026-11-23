import {
  DotsSixVerticalIcon,
  FilmSlateIcon,
  MagnifyingGlassIcon,
  PlusIcon,
  StackIcon,
  TrashIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { useRef, useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import {
  addCollectionEntries,
  ApiError,
  deleteCollection,
  fetchCollection,
  fetchCollectionChoices,
  orderCollectionEntries,
  queryClient,
  queryKeys,
  removeCollectionEntries,
  updateCollection,
  type CollectionChoice,
  type CollectionDetail,
  type CollectionDisplayOrder,
  type CollectionEntry,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { TitlePoster } from '@/features/account/WatchingLists'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  IconButton,
  IconTile,
  InlineError,
  MoveButtons,
  Notice,
  Row,
  RowList,
  SettingRow,
  SkeletonRows,
  TextInput,
  useToast,
} from '@/ui'
import { collectionError, collectionPath } from './Collections'
import { useRowDrag } from './useRowDrag'

const displayOrders = ['premiere', 'chosen'] as const

/** `/libraries/collections/:id`: a collection of the server, its entries and what it may take. */
export default function CollectionRoute() {
  const { t } = useI18n()
  const c = t.collections
  const { id = '' } = useParams()
  const back = { to: '/libraries', label: c.back }
  const collection = useQuery({
    queryKey: queryKeys.collection(id),
    queryFn: ({ signal }) => fetchCollection(id, signal),
  })

  if (collection.isError) {
    const missing = collection.error instanceof ApiError && collection.error.status === 404
    return (
      <PageLayout title={c.title} back={back}>
        {missing ? (
          <EmptyState
            icon={StackIcon}
            title={c.notFound}
            action={<ButtonLink to={back.to}>{back.label}</ButtonLink>}
          />
        ) : (
          <InlineError onRetry={() => void collection.refetch()} retrying={collection.isFetching}>
            {errorMessage(t, collection.error)}
          </InlineError>
        )}
      </PageLayout>
    )
  }
  if (collection.isPending) {
    return (
      <PageLayout title={c.title} back={back}>
        <SkeletonRows rows={4} label={c.loading} />
      </PageLayout>
    )
  }
  return <CollectionPage key={id} collection={collection.data} back={back} />
}

/** Keeps a collection as the server answered, and refreshes the lists that count its entries. */
function stored(collection: CollectionDetail) {
  queryClient.setQueryData(queryKeys.collection(collection.id), collection)
  void queryClient.invalidateQueries({ queryKey: queryKeys.collections, exact: true })
}

function CollectionPage({
  collection,
  back,
}: {
  collection: CollectionDetail
  back: { to: string; label: string }
}) {
  const { t } = useI18n()
  const c = t.collections
  const toast = useToast()
  const navigate = useNavigate()
  const [name, setName] = useState(collection.name)
  const [deleting, setDeleting] = useState(false)

  const rename = useMutation({
    mutationFn: () => updateCollection(collection.id, { name: name.trim() }),
    onSuccess: (saved) => {
      stored(saved)
      setName(saved.name)
      toast(c.renamed)
    },
  })
  const order = useMutation({
    mutationFn: (displayOrder: CollectionDisplayOrder) =>
      updateCollection(collection.id, { displayOrder }),
    onSuccess: (saved) => {
      stored(saved)
      toast(c.displayOrderSaved)
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteCollection(collection.id),
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: queryKeys.collection(collection.id) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.collections })
      toast(c.deleted)
      void navigate(back.to)
    },
  })
  const displayOrder = order.isPending ? order.variables : collection.displayOrder

  function submitName(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (name.trim() !== '' && name.trim() !== collection.name) rename.mutate()
  }

  return (
    <PageLayout
      title={collection.name}
      back={back}
      actions={
        <Button variant="danger" icon={TrashIcon} onClick={() => setDeleting(true)}>
          {c.delete}
        </Button>
      }
    >
      <Block title={c.settings}>
        <form
          onSubmit={submitName}
          noValidate
          className="mb-4 flex max-w-[40rem] flex-wrap items-end gap-2.5"
        >
          <Field
            label={c.name}
            className="min-w-0 flex-[1_1_15rem]"
            error={rename.isError ? collectionError(t, rename.error) : undefined}
          >
            <TextInput value={name} maxLength={500} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Button
            type="submit"
            loading={rename.isPending}
            disabled={name.trim() === '' || name.trim() === collection.name}
          >
            {t.common.save}
          </Button>
        </form>
        <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
          <SettingRow
            kind="choice"
            label={c.displayOrder}
            help={c.displayOrderHelp[displayOrder]}
            error={order.isError ? collectionError(t, order.error) : undefined}
            value={displayOrder}
            options={displayOrders.map((value) => ({ value, label: c.displayOrders[value] }))}
            onChange={(value) => {
              if (value !== collection.displayOrder) order.mutate(value)
            }}
          />
        </div>
      </Block>
      <Entries collection={collection} displayOrder={displayOrder} />
      {collection.holders.length > 0 && (
        <Block title={c.heldBy} count={collection.holders.length}>
          <p className="mb-3 max-w-[66ch] text-small text-ink-2">{c.heldByHelp}</p>
          <RowList variant="boxed" aria-label={c.heldBy}>
            {collection.holders.map((holder) => (
              <Row
                key={holder.id}
                leading={StackIcon}
                to={collectionPath(holder.id)}
                title={holder.name}
              />
            ))}
          </RowList>
        </Block>
      )}
      <Picker collection={collection} />
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={c.deleteTitle(collection.name)}
        confirmLabel={c.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {c.deleteBody}
      </ConfirmDialog>
    </PageLayout>
  )
}

/** The thumbnail of an entry: its poster, else an icon of what it is. */
function EntryThumb({ entry }: { entry: CollectionEntry }) {
  if (entry.kind === 'collection') return <IconTile icon={StackIcon} size="sm" />
  if (entry.kind === 'missing') return <IconTile icon={WarningIcon} size="sm" />
  if (entry.posterId !== null)
    return <TitlePoster title={{ kind: entry.kind, posterId: entry.posterId }} />
  return <IconTile icon={FilmSlateIcon} size="sm" />
}

/** The entries in stored order: dragged by their grip or moved by their arrows, and removed. */
function Entries({
  collection,
  displayOrder,
}: {
  collection: CollectionDetail
  displayOrder: CollectionDisplayOrder
}) {
  const { t } = useI18n()
  const c = t.collections
  const toast = useToast()
  const list = useRef<HTMLOListElement>(null)
  const rowDrag = useRowDrag({ list, onDrop: move })
  const entries = collection.entries
  const key = queryKeys.collection(collection.id)

  const reorder = useMutation({
    mutationFn: (ids: string[]) => orderCollectionEntries(collection.id, ids),
    onSuccess: (saved) => queryClient.setQueryData(key, saved),
  })
  const remove = useMutation({
    mutationFn: (entry: CollectionEntry) => removeCollectionEntries(collection.id, [entry.id]),
    onSuccess: (saved, entry) => {
      stored(saved)
      void queryClient.invalidateQueries({ queryKey: ['collections', collection.id, 'choices'] })
      toast(c.removed(entryName(entry)))
    },
    onError: (error) => toast(collectionError(t, error), { tone: 'danger' }),
  })

  function entryName(entry: CollectionEntry): string {
    return entry.kind === 'missing' ? c.missing : entry.name
  }

  function move(from: number, to: number) {
    const previous = queryClient.getQueryData<CollectionDetail>(key) ?? collection
    const next = [...previous.entries]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    void queryClient.cancelQueries({ queryKey: key, exact: true })
    queryClient.setQueryData(key, { ...previous, entries: next })
    reorder.mutate(
      next.map((entry) => entry.id),
      {
        onError: (error) => {
          queryClient.setQueryData(key, previous)
          toast(collectionError(t, error), { tone: 'danger' })
          if (error instanceof ApiError && error.code === 'entries_changed')
            void queryClient.invalidateQueries({ queryKey: key, exact: true })
        },
      },
    )
  }

  function meta(entry: CollectionEntry): string | null {
    if (entry.kind === 'missing') return null
    if (entry.kind === 'collection') return c.collectionMeta(entry.entries ?? 0)
    const parts: string[] = [c.kinds[entry.kind]]
    if (entry.year !== null && entry.year > 0) parts.push(String(entry.year))
    return parts.join(' · ')
  }

  return (
    <Block title={c.entries} count={entries.length}>
      {displayOrder === 'premiere' && entries.length > 1 && (
        <Notice className="mb-3">{c.premiereNote}</Notice>
      )}
      {entries.length === 0 ? (
        <EmptyState icon={StackIcon} title={c.entriesEmptyTitle}>
          {c.entriesEmptyHow}
        </EmptyState>
      ) : (
        <ol ref={list} aria-label={c.entries} className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
          {entries.map((entry, index) => {
            const name = entryName(entry)
            const dragged = rowDrag.dragged === index
            const detail = meta(entry)
            return (
              <li
                key={entry.id}
                data-entry-kind={entry.kind}
                className={cx('relative min-w-0', dragged && 'opacity-40')}
              >
                {/* The drop place, in the gap above or below the row. */}
                {rowDrag.dropBefore === index && (
                  <span
                    aria-hidden="true"
                    className="absolute inset-x-4 -top-[0.25rem] h-0.5 rounded-full bg-accent"
                  />
                )}
                {rowDrag.dropBefore === entries.length && index === entries.length - 1 && (
                  <span
                    aria-hidden="true"
                    className="absolute inset-x-4 -bottom-[0.25rem] h-0.5 rounded-full bg-accent"
                  />
                )}
                <div className="grid min-h-15 grid-cols-[auto_auto_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-2 rounded-row bg-s1 py-2 pr-3 pl-2 max-md:gap-x-2.5 max-md:pl-1.5">
                  <span className="flex items-center gap-0.5">
                    {/* Mouse, pen and touch only: keyboards and screen readers use the arrows. */}
                    <span
                      aria-hidden="true"
                      title={c.dragHandle(name)}
                      className={cx(
                        'flex h-8 w-6 shrink-0 touch-none items-center justify-center rounded-control text-ink-3 select-none [-webkit-touch-callout:none] hover:text-ink',
                        dragged ? 'cursor-grabbing' : 'cursor-grab',
                      )}
                      {...rowDrag.grip(index)}
                    >
                      <DotsSixVerticalIcon size="1.125rem" />
                    </span>
                    <span
                      aria-hidden="true"
                      className="figures min-w-[1.4em] text-micro text-ink-3"
                    >
                      {index + 1}
                    </span>
                  </span>
                  <EntryThumb entry={entry} />
                  <div className="min-w-0">
                    <p className="min-w-0 text-h3 break-words">
                      {entry.kind === 'collection' ? (
                        <Link to={collectionPath(entry.id)} className="text-ink hover:underline">
                          {name}
                        </Link>
                      ) : (
                        <span className={entry.kind === 'missing' ? 'text-ink-3' : 'text-ink'}>
                          {name}
                        </span>
                      )}
                    </p>
                    {detail !== null && (
                      <p className="mt-0.5 text-small break-words text-ink-3 max-md:text-micro">
                        {detail}
                      </p>
                    )}
                  </div>
                  <div className="flex items-center justify-end gap-1">
                    <MoveButtons
                      name={name}
                      index={index}
                      count={entries.length}
                      onMove={(to) => move(index, to)}
                    />
                    <IconButton
                      size="sm"
                      icon={XIcon}
                      label={c.removeLabel(name)}
                      loading={remove.isPending && remove.variables.id === entry.id}
                      onClick={() => remove.mutate(entry)}
                    />
                  </div>
                </div>
              </li>
            )
          })}
        </ol>
      )}
    </Block>
  )
}

/** One search for the titles and the collections a collection may take, each added in a click. */
function Picker({ collection }: { collection: CollectionDetail }) {
  const { t } = useI18n()
  const c = t.collections
  const toast = useToast()
  const [query, setQuery] = useState('')
  const [asked, setAsked] = useState('')
  const choices = useQuery({
    queryKey: queryKeys.collectionChoices(collection.id, asked),
    queryFn: ({ signal }) => fetchCollectionChoices(collection.id, asked, signal),
    placeholderData: keepPreviousData,
  })
  const add = useMutation({
    mutationFn: (choice: { id: string; name: string }) =>
      addCollectionEntries(collection.id, [choice.id]),
    onSuccess: (saved, choice) => {
      stored(saved)
      void queryClient.invalidateQueries({ queryKey: ['collections', collection.id, 'choices'] })
      toast(c.added(choice.name))
    },
    onError: (error) => toast(collectionError(t, error), { tone: 'danger' }),
  })

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = query.trim()
    if (next === asked) void choices.refetch()
    else setAsked(next)
  }

  function addButton(choice: { id: string; name: string; held: boolean }) {
    if (choice.held) return <Badge tone="ok">{c.addedBadge}</Badge>
    return (
      <IconButton
        size="sm"
        icon={PlusIcon}
        label={c.addLabel(choice.name)}
        loading={add.isPending && add.variables.id === choice.id}
        onClick={() => add.mutate(choice)}
      />
    )
  }

  function collectionRow(choice: CollectionChoice) {
    const meta = [c.entryCount(choice.entries)]
    if (choice.refusal !== null) meta.push(c.refusals[choice.refusal])
    if (choice.refusal !== null) {
      return (
        <Row
          key={choice.id}
          leading={StackIcon}
          title={<span aria-disabled="true">{choice.name}</span>}
          meta={meta.join(' · ')}
          muted
          className="opacity-60"
          trailing={
            <IconButton
              size="sm"
              icon={PlusIcon}
              label={c.addLabel(choice.name)}
              disabled
              aria-disabled="true"
            />
          }
        />
      )
    }
    return (
      <Row
        key={choice.id}
        leading={StackIcon}
        title={choice.name}
        meta={meta.join(' · ')}
        trailing={addButton(choice)}
      />
    )
  }

  let results
  if (choices.isPending) results = <SkeletonRows rows={3} label={c.searching} />
  else if (choices.isError)
    results = (
      <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
        {errorMessage(t, choices.error)}
      </InlineError>
    )
  else {
    const { titles, collections } = choices.data
    results = (
      <div className="flex flex-col gap-5">
        <section>
          <h3 className="mb-2 text-control font-semibold text-ink">{c.titlesGroup}</h3>
          {asked === '' ? (
            <p className="text-small text-ink-2">{c.searchHint}</p>
          ) : titles.length === 0 ? (
            <p className="text-small text-ink-2">{c.noTitles}</p>
          ) : (
            <RowList variant="boxed" aria-label={c.titlesGroup}>
              {titles.map((title) => (
                <Row
                  key={title.id}
                  leading={
                    title.posterId !== null ? (
                      <TitlePoster title={{ kind: title.kind, posterId: title.posterId }} />
                    ) : (
                      <IconTile icon={FilmSlateIcon} size="sm" />
                    )
                  }
                  title={title.name}
                  meta={[c.kinds[title.kind], ...(title.year ? [String(title.year)] : [])].join(
                    ' · ',
                  )}
                  trailing={addButton(title)}
                />
              ))}
            </RowList>
          )}
        </section>
        <section>
          <h3 className="mb-2 text-control font-semibold text-ink">{c.collectionsGroup}</h3>
          {collections.length === 0 ? (
            <p className="text-small text-ink-2">{c.noCollections}</p>
          ) : (
            <RowList variant="boxed" aria-label={c.collectionsGroup}>
              {collections.map(collectionRow)}
            </RowList>
          )}
        </section>
      </div>
    )
  }

  return (
    <Block title={c.addTitle}>
      <div className="max-w-[40rem]">
        <form onSubmit={submit} noValidate className="mb-4 flex flex-wrap items-end gap-2.5">
          <Field label={c.search} className="min-w-0 flex-[1_1_15rem]">
            <TextInput
              icon={MagnifyingGlassIcon}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={c.searchPlaceholder}
            />
          </Field>
          <Button type="submit" loading={choices.isFetching}>
            {c.searchButton}
          </Button>
        </form>
        {results}
      </div>
    </Block>
  )
}
