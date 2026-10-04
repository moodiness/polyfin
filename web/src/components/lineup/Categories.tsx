import { useId, useState, type FormEvent } from 'react'
import { Link } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  bulkLineupCategories,
  createLineupCategory,
  deleteLineupCategory,
  fetchLineupCategories,
  orderLineupCategories,
  queryClient,
  queryKeys,
  updateLineupCategory,
  type LineupCategory,
  type Scope,
} from '@/api'
import { icons } from '@/components/icons'
import { smallField, Switch } from '@/components/lineup/shared'
import { Empty, Panel, Skeleton } from '@/components/panels'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  ConfirmButton,
  MoveButtons,
  Notice,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { invalidateLineup, lineupPath } from '@/components/lineup/common'

/** A source's categories, in order: shown or not, renamed, reordered, custom ones added. */
export default function Categories({ scope, id }: { scope: Scope; id: string }) {
  const { language, t } = useI18n()
  const text = t.lineup.categories
  const key = [...queryKeys.lineup(scope, id), 'categories']
  const categories = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchLineupCategories(scope, id, signal),
  })
  const [search, setSearch] = useState('')
  const [announcement, setAnnouncement] = useState('')
  const [dragged, setDragged] = useState<string | null>(null)
  const [dropBefore, setDropBefore] = useState<number | null>(null)
  const searchId = useId()
  const items = categories.data?.items ?? []
  const needle = search.trim().toLocaleLowerCase(language)
  const listed =
    needle === ''
      ? items
      : items.filter((c) =>
          `${c.name || t.lineup.exclusions.noGroup} ${c.providerName}`
            .toLocaleLowerCase(language)
            .includes(needle),
        )
  const filtering = needle !== ''

  const order = useMutation({
    mutationFn: (ids: string[]) => orderLineupCategories(scope, id, ids),
    onMutate: (ids) => {
      const byId = new Map(items.map((c) => [c.id, c]))
      queryClient.setQueryData(key, {
        items: ids.map((categoryId, index) => ({ ...byId.get(categoryId)!, position: index + 1 })),
      })
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const bulk = useMutation({
    mutationFn: (enabled: boolean) =>
      bulkLineupCategories(scope, id, enabled, filtering ? listed.map((c) => c.id) : undefined),
    onSuccess: (result) => setAnnouncement(text.bulkDone(result.changed)),
    onSettled: () => invalidateLineup(scope, id),
  })

  function move(from: number, to: number) {
    if (to < 0 || to >= items.length || from === to) return
    const ids = items.map((c) => c.id)
    const [moved] = ids.splice(from, 1)
    ids.splice(to, 0, moved)
    setAnnouncement(t.common.moved(items[from].name, to + 1, items.length))
    order.mutate(ids)
  }

  return (
    <div className="space-y-6">
      <CreateForm scope={scope} id={id} />
      <Panel
        title={text.title}
        description={filtering ? text.reorderOff : text.help}
        actions={
          <>
            <button
              type="button"
              className={buttonSecondary}
              disabled={bulk.isPending || items.length === 0}
              onClick={() => bulk.mutate(true)}
            >
              {filtering ? text.enableListed : text.enableAll}
            </button>
            <button
              type="button"
              className={buttonSecondary}
              disabled={bulk.isPending || items.length === 0}
              onClick={() => bulk.mutate(false)}
            >
              {filtering ? text.disableListed : text.disableAll}
            </button>
          </>
        }
      >
        <div className="relative mb-4 max-w-md">
          <label htmlFor={searchId} className="mb-1.5 block text-xs font-medium text-muted">
            {text.search}
          </label>
          <icons.search className="pointer-events-none absolute bottom-2.5 left-3 size-4 text-muted" />
          <input
            id={searchId}
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            autoComplete="off"
            className={`${smallField} pl-9`}
          />
        </div>
        <p aria-live="polite" className="sr-only">
          {announcement}
        </p>
        {(order.isError || bulk.isError) && (
          <div className="mb-3">
            <Notice kind="error">{errorMessage(t, order.error ?? bulk.error)}</Notice>
          </div>
        )}
        {categories.isPending ? (
          <Skeleton rows={6} label={t.common.loading} />
        ) : categories.isError ? (
          <Notice kind="error">{errorMessage(t, categories.error)}</Notice>
        ) : listed.length === 0 ? (
          <Empty>{items.length === 0 ? text.empty : text.noMatch}</Empty>
        ) : (
          <ol
            className="rounded-xl border border-line"
            onDragLeave={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                setDropBefore(null)
              }
            }}
          >
            {listed.map((category) => {
              const index = items.indexOf(category)
              return (
                <CategoryRow
                  key={category.id}
                  scope={scope}
                  id={id}
                  category={category}
                  index={index}
                  count={items.length}
                  reorder={!filtering}
                  dragging={dragged === category.id}
                  dropAbove={dropBefore === index}
                  dropBelow={dropBefore === items.length && index === items.length - 1}
                  onMove={(to) => move(index, to)}
                  onDragStart={() => setDragged(category.id)}
                  onDragEnd={() => {
                    setDragged(null)
                    setDropBefore(null)
                  }}
                  onDragOverHalf={(after) => setDropBefore(after ? index + 1 : index)}
                  onDrop={() => {
                    const from = items.findIndex((c) => c.id === dragged)
                    if (from >= 0 && dropBefore !== null) {
                      move(from, dropBefore > from ? dropBefore - 1 : dropBefore)
                    }
                    setDragged(null)
                    setDropBefore(null)
                  }}
                />
              )
            })}
          </ol>
        )}
      </Panel>
    </div>
  )
}

function CreateForm({ scope, id }: { scope: Scope; id: string }) {
  const { t } = useI18n()
  const text = t.lineup.categories
  const inputId = useId()
  const [name, setName] = useState('')
  const create = useMutation({
    mutationFn: () => createLineupCategory(scope, id, name.trim()),
    onSuccess: () => setName(''),
    onSettled: () => invalidateLineup(scope, id),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (name.trim() !== '') create.mutate()
  }
  return (
    <Panel title={text.createTitle} description={text.createHelp}>
      <form onSubmit={submit} noValidate className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1">
          <label htmlFor={inputId} className="mb-1.5 block text-xs font-medium text-muted">
            {text.createLabel}
          </label>
          <input
            id={inputId}
            value={name}
            maxLength={64}
            autoComplete="off"
            onChange={(event) => {
              create.reset()
              setName(event.target.value)
            }}
            className={smallField}
          />
        </div>
        <button
          type="submit"
          className={buttonPrimary}
          disabled={create.isPending || name.trim() === ''}
        >
          {text.create}
        </button>
      </form>
      <div aria-live="polite" className="mt-3 empty:hidden">
        {create.isError && <Notice kind="error">{errorMessage(t, create.error)}</Notice>}
        {create.isSuccess && <Notice kind="success">{text.created(create.data.name)}</Notice>}
      </div>
    </Panel>
  )
}

function CategoryRow({
  scope,
  id,
  category,
  index,
  count,
  reorder,
  dragging,
  dropAbove,
  dropBelow,
  onMove,
  onDragStart,
  onDragEnd,
  onDragOverHalf,
  onDrop,
}: {
  scope: Scope
  id: string
  category: LineupCategory
  index: number
  count: number
  reorder: boolean
  dragging: boolean
  dropAbove: boolean
  dropBelow: boolean
  onMove: (to: number) => void
  onDragStart: () => void
  onDragEnd: () => void
  onDragOverHalf: (after: boolean) => void
  onDrop: () => void
}) {
  const { language, t } = useI18n()
  const text = t.lineup.categories
  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(category.name)
  const nameId = useId()
  // Entries the provider lists without a group share a category with no
  // name; it is shown under the import preview's "No group".
  const shown = category.name || t.lineup.exclusions.noGroup
  const key = [...queryKeys.lineup(scope, id), 'categories']
  const update = useMutation({
    mutationFn: (patch: Partial<{ name: string | null; enabled: boolean }>) =>
      updateLineupCategory(scope, id, category.id, patch),
    onSuccess: (updated) => {
      queryClient.setQueryData<{ items: LineupCategory[] }>(key, (old) =>
        old ? { items: old.items.map((c) => (c.id === updated.id ? updated : c)) } : old,
      )
      setRenaming(false)
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  const remove = useMutation({
    mutationFn: () => deleteLineupCategory(scope, id, category.id),
    onSettled: () => invalidateLineup(scope, id),
  })
  const renamed = !category.custom && category.name !== category.providerName

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (name.trim() !== '') update.mutate({ name: name.trim() })
  }

  return (
    <li
      draggable={reorder && !renaming}
      onDragStart={(event) => {
        event.dataTransfer.effectAllowed = 'move'
        event.dataTransfer.setData('text/plain', category.id)
        onDragStart()
      }}
      onDragEnd={onDragEnd}
      onDragOver={(event) => {
        if (!reorder) return
        event.preventDefault()
        const box = event.currentTarget.getBoundingClientRect()
        onDragOverHalf(event.clientY > box.top + box.height / 2)
      }}
      onDrop={(event) => {
        event.preventDefault()
        onDrop()
      }}
      className={`relative border-b border-line px-3 py-2.5 last:border-b-0 [contain-intrinsic-size:auto_3.75rem] [content-visibility:auto] ${
        dragging ? 'opacity-40' : ''
      }`}
    >
      {dropAbove && (
        <span aria-hidden="true" className="absolute inset-x-0 -top-px h-0.5 bg-fin-5" />
      )}
      {dropBelow && (
        <span aria-hidden="true" className="absolute inset-x-0 -bottom-px h-0.5 bg-fin-5" />
      )}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        {reorder && (
          <span
            aria-hidden="true"
            className="hidden cursor-grab text-zinc-600 select-none sm:inline"
            title={text.dragHint}
          >
            ⋮⋮
          </span>
        )}
        <span className="w-10 shrink-0 text-right text-xs text-muted tabular-nums">
          {category.position}
        </span>
        <Switch
          label={text.enabledLabel(shown)}
          checked={category.enabled}
          disabled={update.isPending}
          onChange={(enabled) => update.mutate({ enabled })}
        />
        <div className="min-w-0 flex-1 basis-48">
          {renaming ? (
            <form onSubmit={submit} className="flex flex-wrap items-center gap-2">
              <label htmlFor={nameId} className="sr-only">
                {text.renameLabel(shown)}
              </label>
              <input
                id={nameId}
                value={name}
                maxLength={64}
                autoFocus
                autoComplete="off"
                onChange={(event) => setName(event.target.value)}
                onKeyDown={(event) => event.key === 'Escape' && setRenaming(false)}
                className={`${smallField} max-w-xs`}
              />
              <button type="submit" className={rowButton} disabled={update.isPending}>
                {t.common.save}
              </button>
              <button type="button" className={rowButton} onClick={() => setRenaming(false)}>
                {t.common.cancel}
              </button>
            </form>
          ) : (
            <p className="flex flex-wrap items-center gap-2 text-sm">
              <span className={`font-medium ${category.enabled ? 'text-white' : 'text-zinc-400'}`}>
                {shown}
              </span>
              {category.custom && <Badge tone="fin">{text.custom}</Badge>}
              {renamed && (
                <span className="text-xs text-muted">
                  {text.providerName(category.providerName)}
                </span>
              )}
            </p>
          )}
          <p className="text-xs text-muted tabular-nums">
            {text.channelCounts(
              category.enabledChannels.toLocaleString(language),
              category.channels.toLocaleString(language),
            )}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Link
            to={`${lineupPath(scope, id, 'channels')}?category=${category.id}`}
            className={rowButton}
            aria-label={text.channelsLabel(shown)}
          >
            {text.channels}
          </Link>
          {!renaming && (
            <button
              type="button"
              className={rowButton}
              aria-label={text.renameLabel(shown)}
              onClick={() => {
                update.reset()
                setName(category.name)
                setRenaming(true)
              }}
            >
              {text.rename}
            </button>
          )}
          {renamed && (
            <button
              type="button"
              className={rowButton}
              disabled={update.isPending}
              onClick={() => update.mutate({ name: null })}
            >
              {text.resetName}
            </button>
          )}
          {category.custom && (
            <ConfirmButton
              label={text.delete}
              busyLabel={text.deleting}
              message={text.deleteConfirm(shown)}
              busy={remove.isPending}
              onConfirm={() => remove.mutate()}
            />
          )}
          {reorder && <MoveButtons name={shown} index={index} count={count} onMove={onMove} />}
        </div>
      </div>
      {(update.isError || remove.isError) && (
        <div className="mt-2">
          <Notice kind="error">{errorMessage(t, update.error ?? remove.error)}</Notice>
        </div>
      )}
    </li>
  )
}

const rowButton =
  'inline-flex min-h-9 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-progress disabled:opacity-60'
