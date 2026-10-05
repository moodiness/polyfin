import {
  ArrowsClockwiseIcon,
  CalendarBlankIcon,
  MagicWandIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  automapCatalog,
  fetchCatalogGuides,
  queryClient,
  queryKeys,
  refreshCatalogGuides,
  saveCatalogGuides,
  type CatalogGuide,
  type CatalogGuides as Guides,
  type CatalogTarget,
  type Scope,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Panel,
  PanelFooter,
  Skeleton,
  SkeletonRows,
  StatusPill,
  TextInput,
  useToast,
  MoveButtons,
  RelativeTime,
} from '@/ui'
import { Figures, useNumber } from './shared'

/** The most guides a catalog takes. */
const maxGuides = 10

/** A guide of the list being edited: one kept, sent by its id, or an address added. */
type Entry = { kind: 'kept'; guide: CatalogGuide } | { kind: 'new'; url: string; key: string }

/**
 * A Live TV catalog's XMLTV guides, in the order channels take them, with how each was last
 * downloaded, the mapping counts, and the automatic mapping. `mappingPath` turns the counts into
 * links to the mapping.
 */
export default function CatalogGuides({
  scope,
  target,
  onChanged,
  mappingPath,
}: {
  scope: Scope
  target: CatalogTarget
  onChanged?: () => void
  mappingPath?: string
}) {
  const { t } = useI18n()
  const guides = useQuery({
    queryKey: queryKeys.catalogGuides(scope, target),
    queryFn: ({ signal }) => fetchCatalogGuides(scope, target, signal),
  })
  if (guides.isPending) {
    return (
      <div role="status" aria-label={t.common.loading} className="space-y-8">
        <div className="grid grid-cols-2 gap-6 border-y border-line py-5 md:grid-cols-4">
          {Array.from({ length: 4 }, (_, index) => (
            <span key={index} className="space-y-2.5">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-6 w-16" />
            </span>
          ))}
        </div>
        <SkeletonRows rows={2} />
      </div>
    )
  }
  if (guides.isError) {
    return (
      <InlineError onRetry={() => void guides.refetch()} retrying={guides.isFetching}>
        {errorMessage(t, guides.error)}
      </InlineError>
    )
  }
  return (
    <div className="space-y-10">
      <Counts guides={guides.data} mappingPath={mappingPath} />
      <GuideList scope={scope} target={target} guides={guides.data} onChanged={onChanged} />
      <Automap scope={scope} target={target} onChanged={onChanged} />
    </div>
  )
}

function Counts({ guides, mappingPath }: { guides: Guides; mappingPath?: string }) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.guides
  const unmapped = guides.channels - guides.mapped
  return (
    <Figures
      items={[
        { label: text.channels, value: number(guides.channels) },
        { label: text.mapped, value: number(guides.mapped), to: mappingPath },
        {
          label: text.unmapped,
          value: number(unmapped),
          tone: unmapped > 0 ? 'warn' : undefined,
          to: mappingPath,
        },
        { label: text.manual, value: number(guides.manual) },
      ]}
    />
  )
}

function GuideList({
  scope,
  target,
  guides,
  onChanged,
}: {
  scope: Scope
  target: CatalogTarget
  guides: Guides
  onChanged?: () => void
}) {
  const { t } = useI18n()
  const toast = useToast()
  const text = t.lineup.guides
  const initial: Entry[] = guides.guides.map((guide) => ({ kind: 'kept', guide }))
  const [entries, setEntries] = useState<Entry[]>(initial)
  const [url, setUrl] = useState('')
  const [counter, setCounter] = useState(0)
  const dirty =
    entries.length !== initial.length ||
    entries.some((entry, i) => entry.kind === 'new' || entry.guide.id !== guides.guides[i].id)
  const settle = (answer: Guides) => {
    // The list follows what the server answered: its downloads and the ids of new guides.
    setEntries(answer.guides.map((guide) => ({ kind: 'kept', guide })))
    queryClient.setQueryData(queryKeys.catalogGuides(scope, target), answer)
    void queryClient.invalidateQueries({ queryKey: queryKeys.catalogGuides(scope, target) })
    void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
    onChanged?.()
  }
  const save = useMutation({
    mutationFn: () =>
      saveCatalogGuides(
        scope,
        target,
        entries.map((entry) => (entry.kind === 'kept' ? { id: entry.guide.id } : entry.url)),
      ),
    onSuccess: (answer) => {
      settle(answer)
      toast(text.saved)
    },
  })
  const refresh = useMutation({
    mutationFn: () => refreshCatalogGuides(scope, target),
    onSuccess: (answer) => {
      settle(answer)
      toast(text.refreshed)
    },
  })
  const busy = save.isPending || refresh.isPending
  const full = entries.length >= maxGuides

  function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (url.trim() === '' || full) return
    save.reset()
    setEntries((current) => [...current, { kind: 'new', url: url.trim(), key: `new-${counter}` }])
    setCounter((n) => n + 1)
    setUrl('')
  }

  function move(from: number, to: number) {
    save.reset()
    setEntries((current) => {
      const next = [...current]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return next
    })
  }

  const refreshButton = (className: string) =>
    guides.guides.length > 0 && (
      <Button
        icon={ArrowsClockwiseIcon}
        className={className}
        loading={refresh.isPending}
        disabled={busy || dirty}
        title={dirty ? text.saveFirst : undefined}
        onClick={() => {
          save.reset()
          refresh.mutate()
        }}
      >
        {refresh.isPending ? text.refreshing : text.refresh}
      </Button>
    )

  return (
    <Panel
      title={text.title}
      description={text.help}
      flush
      actions={refreshButton('max-sm:hidden') || undefined}
      footer={
        <PanelFooter note={dirty ? t.lineup.unsaved : t.lineup.upToDate}>
          <Button variant="ghost" disabled={!dirty || busy} onClick={() => setEntries(initial)}>
            {t.lineup.reset}
          </Button>
          <Button
            variant="primary"
            loading={save.isPending}
            disabled={!dirty || busy}
            onClick={() => save.mutate()}
          >
            {save.isPending ? text.saving : text.save}
          </Button>
        </PanelFooter>
      }
    >
      <div className="space-y-5 p-6 max-sm:p-4">
        <div className="empty:hidden sm:hidden">{refreshButton('')}</div>
        {entries.length === 0 ? (
          <EmptyState icon={CalendarBlankIcon} title={text.none}>
            {t.lineup.guidesNoneHelp}
          </EmptyState>
        ) : (
          <ol aria-label={text.title} className="rounded-row border border-line-2 bg-bg">
            {entries.map((entry, index) => {
              const label = entry.kind === 'kept' ? entry.guide.url : entry.url
              return (
                <li
                  key={entry.kind === 'kept' ? entry.guide.id : entry.key}
                  className="flex flex-wrap items-start gap-x-3.5 gap-y-2 px-4 py-3.5 not-first:border-t not-first:border-line"
                >
                  <span className="figures mt-0.5 w-5 text-right text-[12.5px] text-ink-3">
                    {index + 1}
                  </span>
                  <div className="min-w-0 flex-1 basis-56 space-y-1.5">
                    <p className="figures text-small break-all text-ink">{label}</p>
                    {entry.kind === 'new' ? (
                      <Badge tone="warn">{text.notSaved}</Badge>
                    ) : (
                      <GuideStatus guide={entry.guide} />
                    )}
                  </div>
                  <span className="flex items-center">
                    <MoveButtons
                      name={label}
                      index={index}
                      count={entries.length}
                      onMove={(to) => move(index, to)}
                    />
                    <IconButton
                      size="sm"
                      danger
                      icon={TrashIcon}
                      label={text.removeLabel(label)}
                      onClick={() => {
                        save.reset()
                        setEntries((current) => current.filter((_, i) => i !== index))
                      }}
                    />
                  </span>
                </li>
              )
            })}
          </ol>
        )}
        <form onSubmit={add} noValidate className="flex flex-col gap-3 sm:flex-row sm:items-start">
          <Field
            label={text.address}
            help={full ? text.tooMany : text.addressHint}
            className="flex-1"
          >
            <TextInput
              type="url"
              inputMode="url"
              mono
              value={url}
              autoComplete="off"
              spellCheck={false}
              placeholder="https://…/guide.xml.gz"
              onValue={setUrl}
            />
          </Field>
          <Button
            type="submit"
            icon={PlusIcon}
            className="h-10 sm:mt-[30px]"
            disabled={url.trim() === '' || full}
          >
            {text.add}
          </Button>
        </form>
        <div aria-live="polite" className="space-y-2 empty:hidden">
          {save.isError && <FieldError>{errorMessage(t, save.error)}</FieldError>}
          {refresh.isError && <FieldError>{errorMessage(t, refresh.error)}</FieldError>}
        </div>
      </div>
    </Panel>
  )
}

/** How a guide's last download went: a state with its icon and word, then its figures. */
export function GuideStatus({ guide }: { guide: CatalogGuide }) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.guides
  return (
    <div className="space-y-1 text-small">
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-ink-3">
        {guide.error !== '' ? (
          <StatusPill tone="warn">{text.failing}</StatusPill>
        ) : guide.fetchedAt ? (
          <StatusPill tone="ok">{text.ok}</StatusPill>
        ) : (
          <StatusPill tone="muted">{text.notYet}</StatusPill>
        )}
        {guide.fetchedAt && (
          <span>
            {text.fetched} <RelativeTime iso={guide.fetchedAt} /> ·{' '}
            <span className="tabular-nums">
              {text.contents(number(guide.channels), number(guide.programmes))}
            </span>
          </span>
        )}
        {guide.nextAt && (
          <span>
            {text.next} <RelativeTime iso={guide.nextAt} />
          </span>
        )}
      </p>
      {guide.error !== '' && (
        <p className="text-warn">{t.lineup.guideErrors[guide.error] ?? t.errors.generic}</p>
      )}
    </div>
  )
}

function Automap({
  scope,
  target,
  onChanged,
}: {
  scope: Scope
  target: CatalogTarget
  onChanged?: () => void
}) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.automap
  const [confirming, setConfirming] = useState(false)
  const run = useMutation({
    mutationFn: (mode: 'unmapped' | 'remap') => automapCatalog(scope, target, mode),
    onSuccess: () => {
      setConfirming(false)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.catalogGuides(scope, target) })
      onChanged?.()
    },
  })
  return (
    <Panel title={text.title} description={text.help}>
      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          icon={MagicWandIcon}
          loading={run.isPending && run.variables === 'unmapped'}
          disabled={run.isPending}
          onClick={() => run.mutate('unmapped')}
        >
          {run.isPending && run.variables === 'unmapped' ? text.running : text.unmapped}
        </Button>
        <Button
          variant="danger"
          disabled={run.isPending}
          onClick={() => {
            run.reset()
            setConfirming(true)
          }}
        >
          {text.remap}
        </Button>
      </div>
      <div aria-live="polite" className="mt-3 empty:hidden">
        {run.isError && !confirming && <FieldError>{errorMessage(t, run.error)}</FieldError>}
        {run.isSuccess && (
          <p className="text-small text-ink-2 tabular-nums">
            {text.done(
              number(run.data.changed),
              number(run.data.mapped),
              number(run.data.channels),
            )}
          </p>
        )}
      </div>
      <ConfirmDialog
        open={confirming}
        onClose={() => setConfirming(false)}
        onConfirm={() => run.mutate('remap')}
        title={text.remapConfirm}
        confirmLabel={text.remap}
        busy={run.isPending}
        error={run.isError ? errorMessage(t, run.error) : undefined}
      />
    </Panel>
  )
}
