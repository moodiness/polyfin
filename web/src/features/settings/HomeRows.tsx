import {
  EyeIcon,
  FilmSlateIcon,
  PlusIcon,
  RowsIcon,
  TelevisionSimpleIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId, useState, type FormEvent } from 'react'
import {
  createHomeRow,
  deleteHomeRow,
  fetchHomeRowChoices,
  fetchHomeRowPreview,
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
  type HomeRowPreviewTitle,
  type HomeRowSchedule,
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

const kinds: HomeRowKind[] = ['catalog', 'collection', 'because', 'new', 'available', 'waiting']
const places: HomeRowPlace[] = ['top', 'resume', 'nextup', 'bottom']

/** The bounds of Because you watched's rows, as the server checks them. */
const count = { min: 1, max: 10, byDefault: 3 }

/** The days of the week, from Sunday, as schedules number them. */
const weekdays = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
] as const

/** An hour of the day on the clock: 18 is 18:00. */
function clockHour(hour: number) {
  return `${String(hour).padStart(2, '0')}:00`
}

/** Whether a schedule bounds the row at all. */
function scheduled(schedule: HomeRowSchedule) {
  return (
    schedule.startDate !== '' ||
    schedule.endDate !== '' ||
    schedule.days.length > 0 ||
    schedule.startHour !== null
  )
}

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
 * each with its kind, place, who it is shown to and whether its schedule shows it now, Move, Edit
 * and Delete, Add, and Preview as…, which shows a user's rows as their web player does.
 */
export default function HomeRows() {
  const { t } = useI18n()
  const text = t.settings.homeRow
  const toast = useToast()
  const [editing, setEditing] = useState<HomeRow | 'new' | null>(null)
  const [previewing, setPreviewing] = useState(false)
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
      <div className="flex flex-wrap gap-2">
        <Button icon={PlusIcon} onClick={() => setEditing('new')}>
          {text.add}
        </Button>
        {list.length > 0 && (
          <Button variant="ghost" icon={EyeIcon} onClick={() => setPreviewing(true)}>
            {text.preview}
          </Button>
        )}
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
      {previewing && (
        <HomeRowPreviewModal users={users.data ?? []} onClose={() => setPreviewing(false)} />
      )}
    </div>
  )
}

/** A row of the list: its name, what it lists, its place, who sees it and when. */
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
      titleAside={
        row.library || !row.shownNow ? (
          <span className="flex gap-1.5">
            {row.library && <Badge>{text.libraryBadge}</Badge>}
            {!row.shownNow && <Badge tone="warn">{text.notShownNow}</Badge>}
          </span>
        ) : undefined
      }
      meta={
        <span className="truncate">
          {[
            source,
            text.places[row.place],
            row.everyone ? text.everyone : text.shownToUsers(row.users.length),
            ...(scheduled(row.schedule) ? [text.scheduled] : []),
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
  const [startDate, setStartDate] = useState(row?.schedule.startDate ?? '')
  const [endDate, setEndDate] = useState(row?.schedule.endDate ?? '')
  const [days, setDays] = useState<number[]>(row?.schedule.days ?? [])
  const [someHours, setSomeHours] = useState((row?.schedule.startHour ?? null) !== null)
  const [startHour, setStartHour] = useState(row?.schedule.startHour ?? 18)
  const [endHour, setEndHour] = useState(row?.schedule.endHour ?? 23)
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
  // Dates as YYYY-MM-DD compare as text.
  const datesOutOfOrder = startDate !== '' && endDate !== '' && endDate < startDate
  function submit(event: FormEvent) {
    event.preventDefault()
    if (missingUser || datesOutOfOrder) return
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
      schedule: {
        startDate,
        endDate,
        days: [...days].sort((a, b) => a - b),
        startHour: someHours ? startHour : null,
        endHour: someHours ? endHour : null,
      },
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
        <fieldset className="space-y-4 border-t border-line pt-5">
          <legend className="sr-only">{text.schedule}</legend>
          <div>
            <p className="text-control font-medium text-ink" aria-hidden="true">
              {text.schedule}
            </p>
            <p className="mt-1 max-w-[60ch] text-small text-ink-3">{text.scheduleHelp}</p>
          </div>
          <div className="flex flex-wrap gap-4">
            <Field label={text.startDate}>
              <TextInput
                type="date"
                value={startDate}
                onValue={(value) => {
                  mutation.reset()
                  setStartDate(value)
                }}
                className="w-44"
              />
            </Field>
            <Field
              label={text.endDate}
              error={datesOutOfOrder ? text.dateOrder : undefined}
              help={datesOutOfOrder ? undefined : text.datesHelp}
            >
              <TextInput
                type="date"
                value={endDate}
                onValue={(value) => {
                  mutation.reset()
                  setEndDate(value)
                }}
                className="w-44"
              />
            </Field>
          </div>
          {library ? (
            <Notice>{text.libraryDatesOnly}</Notice>
          ) : (
            <>
              <fieldset>
                <legend className="mb-2 text-control font-medium text-ink">{text.days}</legend>
                <div className="flex flex-wrap gap-x-4 gap-y-2">
                  {weekdays.map((weekday, day) => (
                    <Checkbox
                      key={weekday}
                      label={t.users.days[weekday]}
                      checked={days.includes(day)}
                      onChange={(checked) =>
                        setDays((current) =>
                          checked ? [...current, day] : current.filter((d) => d !== day),
                        )
                      }
                    />
                  ))}
                </div>
                <p className="mt-1.5 text-small text-ink-3">{text.daysHelp}</p>
              </fieldset>
              <Field label={text.hours} help={someHours ? text.hoursHelp : undefined}>
                <Segmented
                  label={text.hours}
                  value={someHours ? 'some' : 'all'}
                  options={[
                    { value: 'all', label: text.allDay },
                    { value: 'some', label: text.someHours },
                  ]}
                  onChange={(value) => setSomeHours(value === 'some')}
                />
              </Field>
              {someHours && (
                <div className="flex flex-wrap gap-4">
                  <Field label={text.startHour}>
                    <Select
                      value={startHour}
                      options={Array.from({ length: 24 }, (_, hour) => ({
                        value: hour,
                        label: clockHour(hour),
                        disabled: hour === endHour,
                      }))}
                      onValue={setStartHour}
                      className="w-32"
                    />
                  </Field>
                  <Field label={text.endHour}>
                    <Select
                      value={endHour}
                      options={Array.from({ length: 24 }, (_, i) => ({
                        value: i + 1,
                        label: clockHour(i + 1),
                        disabled: i + 1 === startHour,
                      }))}
                      onValue={setEndHour}
                      className="w-32"
                    />
                  </Field>
                </div>
              )}
            </>
          )}
        </fieldset>
        {mutation.isError && <Notice tone="danger">{errorMessage(t, mutation.error)}</Notice>}
      </form>
    </Modal>
  )
}

/**
 * Preview as…: the rows a chosen user's web player shows on its home page now, grouped by place
 * in the order the page shows them, each with its first titles.
 */
function HomeRowPreviewModal({ users, onClose }: { users: User[]; onClose: () => void }) {
  const { t } = useI18n()
  const text = t.settings.homeRow
  const [userId, setUserId] = useState('')
  const preview = useQuery({
    queryKey: queryKeys.homeRowPreview(userId),
    queryFn: ({ signal }) => fetchHomeRowPreview(userId, signal),
    enabled: userId !== '',
  })
  const rows = preview.data?.rows ?? []
  return (
    <Modal
      open
      onClose={onClose}
      title={text.previewTitle}
      width={880}
      footer={
        <Button variant="ghost" onClick={onClose}>
          {t.ui.close}
        </Button>
      }
    >
      <div className="space-y-5 p-5">
        <Field label={text.previewUser} help={text.previewHelp}>
          <Select
            value={userId}
            options={[
              { value: '', label: t.settings.homeRow.choose, disabled: true },
              ...users.map((user) => ({ value: user.id, label: user.name })),
            ]}
            onValue={setUserId}
            className="max-w-xs"
          />
        </Field>
        {userId === '' ? (
          <p className="text-small text-ink-3">{text.previewChoose}</p>
        ) : preview.isPending ? (
          <SkeletonRows rows={2} label={text.previewLoading} />
        ) : preview.isError ? (
          <InlineError onRetry={() => void preview.refetch()} retrying={preview.isFetching}>
            {errorMessage(t, preview.error)}
          </InlineError>
        ) : rows.length === 0 ? (
          <EmptyState icon={RowsIcon} title={text.previewEmpty} />
        ) : (
          places
            .filter((place) => rows.some((row) => row.place === place))
            .map((place) => (
              <section key={place} aria-label={text.places[place]} className="space-y-3">
                <p className="text-small font-medium uppercase tracking-wide text-ink-3">
                  {text.places[place]}
                </p>
                {rows
                  .filter((row) => row.place === place)
                  .map((row) => (
                    <div key={row.id} className="space-y-2">
                      <p className="text-control font-medium text-ink">{row.name}</p>
                      <ul className="flex gap-3 overflow-x-auto pb-2">
                        {row.titles.map((title) => (
                          <li key={title.id} className="w-24 shrink-0">
                            <PreviewPoster title={title} />
                            <p className="mt-1 truncate text-small text-ink-2" title={title.name}>
                              {title.name}
                            </p>
                          </li>
                        ))}
                      </ul>
                    </div>
                  ))}
              </section>
            ))
        )}
      </div>
    </Modal>
  )
}

/** A title's poster in a preview, or the icon of its kind when it has none or fails to load. */
function PreviewPoster({ title }: { title: HomeRowPreviewTitle }) {
  const [failed, setFailed] = useState(false)
  const KindIcon =
    title.kind === 'series' || title.kind === 'episode' ? TelevisionSimpleIcon : FilmSlateIcon
  if (failed) {
    return (
      <span
        aria-hidden="true"
        className="grid aspect-[2/3] w-full place-items-center rounded-check border border-line-2 bg-s2 text-ink-3"
      >
        <KindIcon size={20} />
      </span>
    )
  }
  return (
    <img
      src={`/Items/${encodeURIComponent(title.posterId)}/Images/Primary?maxHeight=216&quality=85`}
      alt=""
      loading="lazy"
      onError={() => setFailed(true)}
      className="aspect-[2/3] w-full rounded-check object-cover ring-1 ring-ink/8"
    />
  )
}
