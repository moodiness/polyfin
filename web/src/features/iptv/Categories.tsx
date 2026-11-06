import {
  ArrowCounterClockwiseIcon,
  ListBulletsIcon,
  MagnifyingGlassIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { useId, useState, type FormEvent } from 'react'
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
import { invalidateLineup, lineupPath } from '@/features/iptv/lineup'
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
  FieldError,
  IconButton,
  InlineError,
  SkeletonRows,
  Switch,
  TextInput,
  useToast,
  MoveButtons,
} from '@/ui'
import { DragGrip, useNumber } from './shared'

/** A source's categories, in order: shown or not, renamed, reordered, custom ones added. */
export default function Categories({ scope, id }: { scope: Scope; id: string }) {
  const { language, t } = useI18n()
  const toast = useToast()
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
    onSuccess: (result) => {
      setAnnouncement(text.bulkDone(result.changed))
      toast(text.bulkDone(result.changed))
    },
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
    <div className="space-y-12 max-md:space-y-10">
      <CreateForm scope={scope} id={id} />
      <Block title={text.title} count={categories.data ? items.length : undefined}>
        <p className="mb-5 max-w-[66ch] text-small text-ink-2">
          {filtering ? text.reorderOff : text.help}
        </p>
        <div className="mb-4 flex flex-wrap items-center gap-2.5">
          <TextInput
            type="search"
            size="compact"
            icon={MagnifyingGlassIcon}
            value={search}
            onValue={setSearch}
            placeholder={text.search}
            aria-label={text.search}
            autoComplete="off"
            className="min-w-48 flex-1 sm:max-w-80"
          />
          <div className="ml-auto flex flex-wrap gap-2">
            <Button
              size="sm"
              disabled={bulk.isPending || items.length === 0}
              onClick={() => bulk.mutate(true)}
            >
              {filtering ? text.enableListed : text.enableAll}
            </Button>
            <Button
              size="sm"
              disabled={bulk.isPending || items.length === 0}
              onClick={() => bulk.mutate(false)}
            >
              {filtering ? text.disableListed : text.disableAll}
            </Button>
          </div>
        </div>
        <p aria-live="polite" className="sr-only">
          {announcement}
        </p>
        {(order.isError || bulk.isError) && (
          <div className="mb-4">
            <FieldError>{errorMessage(t, order.error ?? bulk.error)}</FieldError>
          </div>
        )}
        {categories.isPending ? (
          <SkeletonRows rows={6} />
        ) : categories.isError ? (
          <InlineError onRetry={() => void categories.refetch()} retrying={categories.isFetching}>
            {errorMessage(t, categories.error)}
          </InlineError>
        ) : listed.length === 0 ? (
          <EmptyState icon={ListBulletsIcon} title={items.length === 0 ? text.empty : text.noMatch}>
            {items.length === 0 ? t.lineup.channelsEmptyHelp : undefined}
          </EmptyState>
        ) : (
          <ol
            aria-label={text.title}
            className="grid grid-cols-[minmax(0,1fr)] gap-1.5"
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
      </Block>
    </div>
  )
}

function CreateForm({ scope, id }: { scope: Scope; id: string }) {
  const { t } = useI18n()
  const toast = useToast()
  const text = t.lineup.categories
  const titleId = useId()
  const [name, setName] = useState('')
  const create = useMutation({
    mutationFn: () => createLineupCategory(scope, id, name.trim()),
    onSuccess: (created) => {
      setName('')
      toast(text.created(created.name))
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (name.trim() !== '') create.mutate()
  }
  return (
    <section
      aria-labelledby={titleId}
      className="rounded-row bg-s1 px-[1.875rem] pt-6 pb-7 max-md:px-5 max-md:pt-5 max-md:pb-5"
    >
      <h2 id={titleId} className="text-h3 text-ink">
        {text.createTitle}
      </h2>
      <p className="mt-1 text-small text-ink-2">{text.createHelp}</p>
      <form onSubmit={submit} noValidate className="mt-4 flex flex-wrap items-start gap-2.5">
        <Field
          label={text.createLabel}
          hideLabel
          error={create.isError ? errorMessage(t, create.error) : undefined}
          className="min-w-48 flex-1 sm:max-w-[42.75rem]"
        >
          <TextInput
            size="sm"
            value={name}
            maxLength={64}
            autoComplete="off"
            placeholder={text.createLabel}
            onValue={(value) => {
              create.reset()
              setName(value)
            }}
          />
        </Field>
        <Button
          type="submit"
          variant="primary"
          icon={PlusIcon}
          loading={create.isPending}
          disabled={name.trim() === ''}
        >
          {text.create}
        </Button>
      </form>
    </section>
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
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.categories
  const [renaming, setRenaming] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [name, setName] = useState(category.name)
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
    onSuccess: () => setConfirming(false),
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
      className={cx(
        'relative min-w-0 rounded-[1.125rem] bg-s1 transition-colors duration-300 ease-nuit hover:bg-s2 [contain-intrinsic-size:auto_4.15625rem] [content-visibility:auto]',
        dragging && 'opacity-40',
      )}
    >
      {dropAbove && (
        <span
          aria-hidden="true"
          className="absolute inset-x-4 -top-1 h-0.5 rounded-full bg-accent"
        />
      )}
      {dropBelow && (
        <span
          aria-hidden="true"
          className="absolute inset-x-4 -bottom-1 h-0.5 rounded-full bg-accent"
        />
      )}
      <div className="flex min-h-14 flex-wrap items-center gap-x-3 gap-y-1 py-1.5 pr-3 pl-5 max-sm:pl-4">
        {reorder && <DragGrip name={shown} />}
        <span className="figures w-7 shrink-0 text-right text-micro text-ink-3">
          {category.position}
        </span>
        <div className="min-w-0 flex-1 basis-32">
          {renaming ? (
            <form onSubmit={submit} className="flex flex-wrap items-center gap-2 py-1">
              <TextInput
                size="compact"
                value={name}
                maxLength={64}
                autoFocus
                autoComplete="off"
                aria-label={text.renameLabel(shown)}
                onValue={setName}
                onKeyDown={(event) => event.key === 'Escape' && setRenaming(false)}
                className="min-w-40 flex-1 sm:max-w-[23.75rem]"
              />
              <Button type="submit" size="sm" variant="primary" loading={update.isPending}>
                {t.common.save}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setRenaming(false)}>
                {t.common.cancel}
              </Button>
            </form>
          ) : (
            <p className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-0.5">
              <span
                className={cx(
                  'min-w-0 truncate text-control font-semibold',
                  category.enabled ? 'text-ink' : 'text-ink-2',
                )}
              >
                {shown}
              </span>
              {category.custom && <Badge tone="accent">{text.custom}</Badge>}
            </p>
          )}
          <p className="mt-0.5 flex min-w-0 flex-wrap gap-x-2 text-micro text-ink-3">
            <span className="figures">
              {text.channelCounts(number(category.enabledChannels), number(category.channels))}
            </span>
            {renamed && !renaming && (
              <span className="min-w-0 break-words">
                · {text.providerName(category.providerName)}
              </span>
            )}
          </p>
        </div>
        <div className="flex items-center gap-0.5 max-sm:order-last max-sm:w-full max-sm:flex-wrap max-sm:justify-end max-sm:pb-1">
          <ButtonLink
            size="sm"
            variant="ghost"
            to={`${lineupPath(scope, id, 'channels')}?category=${category.id}`}
            aria-label={text.channelsLabel(shown)}
          >
            {text.channels}
          </ButtonLink>
          {!renaming && (
            <IconButton
              size="sm"
              icon={PencilSimpleIcon}
              label={text.renameLabel(shown)}
              onClick={() => {
                update.reset()
                setName(category.name)
                setRenaming(true)
              }}
            />
          )}
          {renamed && (
            <IconButton
              size="sm"
              icon={ArrowCounterClockwiseIcon}
              label={text.resetNameLabel(shown)}
              disabled={update.isPending}
              onClick={() => update.mutate({ name: null })}
            />
          )}
          {category.custom && (
            <IconButton
              size="sm"
              danger
              icon={TrashIcon}
              label={text.deleteLabel(shown)}
              onClick={() => {
                remove.reset()
                setConfirming(true)
              }}
            />
          )}
          {reorder && <MoveButtons name={shown} index={index} count={count} onMove={onMove} />}
        </div>
        <Switch
          label={text.enabledLabel(shown)}
          checked={category.enabled}
          disabled={update.isPending}
          onChange={(enabled) => update.mutate({ enabled })}
          className="max-sm:p-1 max-sm:[&>span:first-child]:hidden"
        />
      </div>
      {update.isError && (
        <div className="px-5 pb-3 max-sm:px-4">
          <FieldError>{errorMessage(t, update.error)}</FieldError>
        </div>
      )}
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={() => remove.mutate()}
        title={text.deleteConfirm(shown)}
        confirmLabel={text.delete}
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      />
    </li>
  )
}
