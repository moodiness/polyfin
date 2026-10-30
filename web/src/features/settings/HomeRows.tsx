import { PlusIcon, RowsIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  createHomeRow,
  deleteHomeRow,
  fetchHomeRowChoices,
  fetchHomeRows,
  fetchUsers,
  orderHomeRows,
  queryClient,
  queryKeys,
  updateHomeRow,
  type HomeRow,
  type HomeRowCatalog,
  type HomeRowCollection,
  type HomeRowDraft,
  type HomeRowKind,
  type HomeRowPlace,
  type User,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  InlineError,
  Modal,
  MoveButtons,
  Notice,
  NumberInput,
  Row,
  RowList,
  Segmented,
  Select,
  SkeletonRows,
  Switch,
  TextInput,
  useToast,
} from '@/ui'

const kinds: HomeRowKind[] = [
  'catalog',
  'collection',
  'because',
  'new',
  'available',
  'waiting',
  'trakt',
  'picks',
]
const places: HomeRowPlace[] = ['top', 'resume', 'nextup', 'bottom']

/** The bounds of Because you watched's rows, as the server checks them. */
const count = { min: 1, max: 10, byDefault: 3 }

/** A catalog's key in the catalog list. */
function catalogKey(row: Pick<HomeRow, 'addonId' | 'catalogType' | 'catalogId'>) {
  return `${row.addonId}/${row.catalogType}/${row.catalogId}`
}

/** A catalog's name: its addon's, then its own. */
function catalogName(catalog: HomeRowCatalog) {
  return `${catalog.addonName} › ${catalog.catalogName}`
}

/**
 * Settings › Web player › Home rows: the rows the web player adds to its home page, in order,
 * each with its kind, place and who it is shown to, Move, Edit and Delete, and Add.
 */
export default function HomeRows() {
  const { t } = useI18n()
  const text = t.settings.homeRow
  const toast = useToast()
  const [editing, setEditing] = useState<HomeRow | 'new' | null>(null)
  const rows = useQuery({
    queryKey: queryKeys.homeRows,
    queryFn: ({ signal }) => fetchHomeRows(signal),
  })
  const choices = useQuery({
    queryKey: queryKeys.homeRowChoices,
    queryFn: ({ signal }) => fetchHomeRowChoices(signal),
  })
  const users = useQuery({
    queryKey: queryKeys.users,
    queryFn: ({ signal }) => fetchUsers(signal),
  })
  const order = useMutation({
    mutationFn: (ids: string[]) => orderHomeRows(ids),
    onSuccess: (answer) => queryClient.setQueryData(queryKeys.homeRows, answer),
    onError: (error) => toast(errorMessage(t, error), { tone: 'danger' }),
  })
  if (rows.isPending) return <SkeletonRows rows={2} boxed label={text.loading} />
  if (rows.isError) {
    return (
      <InlineError onRetry={() => void rows.refetch()} retrying={rows.isFetching}>
        {errorMessage(t, rows.error)}
      </InlineError>
    )
  }
  const list = rows.data.rows
  const move = (from: number, to: number) => {
    const ids = list.map((row) => row.id)
    const [moved] = ids.splice(from, 1)
    ids.splice(to, 0, moved)
    order.mutate(ids)
  }
  return (
    <div className="flex flex-col gap-4">
      {list.length === 0 ? (
        <EmptyState icon={RowsIcon} title={text.empty}>
          {text.emptyHint}
        </EmptyState>
      ) : (
        <RowList aria-label={t.settings.homeRows}>
          {list.map((row, index) => (
            <HomeRowEntry
              key={row.id}
              row={row}
              index={index}
              count={list.length}
              moving={order.isPending}
              onMove={(to) => move(index, to)}
              catalogs={choices.data?.catalogs ?? []}
              collections={choices.data?.collections ?? []}
              onEdit={() => setEditing(row)}
            />
          ))}
        </RowList>
      )}
      <div>
        <Button icon={PlusIcon} onClick={() => setEditing('new')}>
          {text.add}
        </Button>
      </div>
      {editing !== null && (
        <HomeRowModal
          row={editing === 'new' ? null : editing}
          catalogs={choices.data?.catalogs ?? []}
          collections={choices.data?.collections ?? []}
          users={users.data ?? []}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

/** A row of the list: its name, what it lists, its place and who sees it. */
function HomeRowEntry({
  row,
  index,
  count: rowCount,
  moving,
  onMove,
  catalogs,
  collections,
  onEdit,
}: {
  row: HomeRow
  index: number
  count: number
  moving: boolean
  onMove: (to: number) => void
  catalogs: HomeRowCatalog[]
  collections: HomeRowCollection[]
  onEdit: () => void
}) {
  const { t } = useI18n()
  const text = t.settings.homeRow
  const toast = useToast()
  const [confirming, setConfirming] = useState(false)
  const remove = useMutation({
    mutationFn: () => deleteHomeRow(row.id),
    onSuccess: () => {
      setConfirming(false)
      toast(text.deleted(row.name))
      void queryClient.invalidateQueries({ queryKey: queryKeys.homeRows })
    },
  })
  const catalog = catalogs.find((c) => catalogKey(c) === catalogKey(row))
  const collection = collections.find((c) => c.id === row.collectionId)
  const source =
    row.kind === 'catalog'
      ? catalog
        ? catalogName(catalog)
        : text.unknownCatalog
      : row.kind === 'collection'
        ? (collection?.name ?? text.unknownCollection)
        : row.kind === 'because'
          ? `${text.kinds.because} · ${text.rowsCount(row.count)}`
          : text.kinds[row.kind]
  return (
    <Row
      title={row.name}
      titleAside={row.library ? <Badge>{text.libraryBadge}</Badge> : undefined}
      meta={
        <span className="truncate">
          {[
            source,
            text.places[row.place],
            row.everyone ? text.everyone : text.shownToUsers(row.users.length),
          ].join(' · ')}
        </span>
      }
      trailing={
        <div className="flex items-center gap-1.5">
          <MoveButtons
            name={row.name}
            index={index}
            count={rowCount}
            disabled={moving}
            onMove={onMove}
          />
          <Button variant="ghost" size="sm" onClick={onEdit}>
            {text.edit}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => setConfirming(true)}>
            {text.delete}
          </Button>
          <ConfirmDialog
            open={confirming}
            onClose={() => {
              setConfirming(false)
              remove.reset()
            }}
            onConfirm={() => remove.mutate()}
            title={text.deleteTitle(row.name)}
            confirmLabel={text.delete}
            tone="danger"
            busy={remove.isPending}
            error={remove.isError ? errorMessage(t, remove.error) : undefined}
          >
            {text.deleteBody}
          </ConfirmDialog>
        </div>
      }
    />
  )
}

/** The form to add a row, or to change one, in a floating panel. */
function HomeRowModal({
  row,
  catalogs,
  collections,
  users,
  onClose,
}: {
  row: HomeRow | null
  catalogs: HomeRowCatalog[]
  collections: HomeRowCollection[]
  users: User[]
  onClose: () => void
}) {
  const { t } = useI18n()
  const text = t.settings.homeRow
  const toast = useToast()
  const formId = useId()
  const [kind, setKind] = useState<HomeRowKind>(row?.kind ?? 'catalog')
  const [name, setName] = useState(row?.name ?? '')
  // A name the administrator typed is kept; otherwise it follows what the row lists.
  const [named, setNamed] = useState(row !== null)
  const [catalog, setCatalog] = useState(row?.kind === 'catalog' ? catalogKey(row) : '')
  const [collection, setCollection] = useState(row?.collectionId ?? '')
  const [rows, setRows] = useState<number | null>(row?.count ?? count.byDefault)
  const [place, setPlace] = useState<HomeRowPlace>(row?.place ?? 'top')
  const [everyone, setEveryone] = useState(row?.everyone ?? true)
  const [chosen, setChosen] = useState<string[]>(row?.users ?? [])
  const [library, setLibrary] = useState(row?.library ?? false)
  const mutation = useMutation({
    mutationFn: (draft: HomeRowDraft) =>
      row === null ? createHomeRow(draft) : updateHomeRow(row.id, draft),
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.homeRows })
      toast(text.saved(saved.name))
      onClose()
    },
  })

  const catalogOptions = catalogs.map((c) => ({ value: catalogKey(c), label: catalogName(c) }))
  if (catalog !== '' && !catalogOptions.some((option) => option.value === catalog)) {
    catalogOptions.unshift({ value: catalog, label: text.unknownCatalog })
  }
  const collectionOptions = collections.map((c) => ({
    value: c.id,
    label: `${c.name} (${text.collectionSources[c.source]})`,
  }))
  if (collection !== '' && !collectionOptions.some((option) => option.value === collection)) {
    collectionOptions.unshift({ value: collection, label: text.unknownCollection })
  }

  /** The name a row gets from what it lists, until one is typed. */
  const nameFor = (nextKind: HomeRowKind, nextCatalog: string, nextCollection: string) => {
    if (nextKind === 'catalog') {
      return catalogs.find((c) => catalogKey(c) === nextCatalog)?.catalogName ?? ''
    }
    if (nextKind === 'collection')
      return collections.find((c) => c.id === nextCollection)?.name ?? ''
    return text.defaultNames[nextKind]
  }
  const follow = (nextKind: HomeRowKind, nextCatalog: string, nextCollection: string) => {
    mutation.reset()
    if (!named) setName(nameFor(nextKind, nextCatalog, nextCollection))
  }

  const missingUser = !everyone && chosen.length === 0
  function submit(event: FormEvent) {
    event.preventDefault()
    if (missingUser) return
    const [addonId, catalogType, ...rest] = kind === 'catalog' ? catalog.split('/') : []
    mutation.mutate({
      name: name.trim(),
      kind,
      place,
      addonId: addonId ?? '',
      catalogType: catalogType ?? '',
      catalogId: rest.join('/'),
      collectionId: kind === 'collection' ? collection : '',
      count: kind === 'because' ? (rows ?? count.byDefault) : count.byDefault,
      everyone,
      users: everyone ? [] : chosen,
      library,
    })
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={row === null ? text.addTitle : text.editTitle(row.name)}
      width={560}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            {t.common.cancel}
          </Button>
          <Button variant="primary" type="submit" form={formId} loading={mutation.isPending}>
            {row === null
              ? mutation.isPending
                ? text.creating
                : text.create
              : mutation.isPending
                ? text.saving
                : text.save}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={submit} noValidate className="space-y-5 p-5">
        <Field label={text.kind} help={text.kindHelp[kind]}>
          <Select
            value={kind}
            options={kinds.map((value) => ({ value, label: text.kinds[value] }))}
            onValue={(value) => {
              setKind(value)
              follow(value, catalog, collection)
            }}
            className="max-w-xs"
          />
        </Field>
        {kind === 'catalog' &&
          (catalogOptions.length === 0 ? (
            <Notice tone="warn">{text.noCatalog}</Notice>
          ) : (
            <Field label={text.catalog}>
              <Select
                value={catalog}
                options={[{ value: '', label: text.choose, disabled: true }, ...catalogOptions]}
                onValue={(value) => {
                  setCatalog(value)
                  follow(kind, value, collection)
                }}
              />
            </Field>
          ))}
        {kind === 'collection' &&
          (collectionOptions.length === 0 ? (
            <Notice tone="warn">{text.noCollection}</Notice>
          ) : (
            <Field label={text.collection}>
              <Select
                value={collection}
                options={[{ value: '', label: text.choose, disabled: true }, ...collectionOptions]}
                onValue={(value) => {
                  setCollection(value)
                  follow(kind, catalog, value)
                }}
              />
            </Field>
          ))}
        <Field label={text.name} help={kind === 'because' ? text.becauseNameHelp : text.nameHelp}>
          <TextInput
            value={name}
            onValue={(value) => {
              mutation.reset()
              setNamed(value.trim() !== '')
              setName(value)
            }}
            maxLength={100}
            autoComplete="off"
          />
        </Field>
        {kind === 'because' && (
          <Field label={text.count} help={text.countHelp}>
            <NumberInput value={rows} onValue={setRows} min={count.min} max={count.max} step={1} />
          </Field>
        )}
        <Field label={text.place} help={text.placeHelp}>
          <Select
            value={place}
            options={places.map((value) => ({ value, label: text.places[value] }))}
            onValue={setPlace}
            className="max-w-xs"
          />
        </Field>
        <Field label={text.shownTo} error={missingUser ? text.noUser : undefined}>
          <Segmented
            label={text.shownTo}
            value={everyone ? 'everyone' : 'chosen'}
            options={[
              { value: 'everyone', label: text.everyone },
              { value: 'chosen', label: text.chosenUsers },
            ]}
            onChange={(value) => setEveryone(value === 'everyone')}
          />
        </Field>
        {!everyone && (
          <fieldset className="space-y-2.5">
            <legend className="mb-2 text-control font-medium text-ink">{text.users}</legend>
            {users.map((user) => (
              <Checkbox
                key={user.id}
                label={user.name}
                checked={chosen.includes(user.id)}
                onChange={(checked) =>
                  setChosen((current) =>
                    checked ? [...current, user.id] : current.filter((id) => id !== user.id),
                  )
                }
              />
            ))}
          </fieldset>
        )}
        <div className="flex items-start justify-between gap-4">
          <div>
            <p id={`${formId}-library`} className="text-control font-medium text-ink">
              {text.library}
            </p>
            <p className="mt-1 max-w-[60ch] text-small text-ink-3">{text.libraryHelp}</p>
          </div>
          <Switch checked={library} onChange={setLibrary} labelledBy={`${formId}-library`} />
        </div>
        {mutation.isError && <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>}
      </form>
    </Modal>
  )
}
