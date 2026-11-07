import {
  ArrowsClockwiseIcon,
  CalendarBlankIcon,
  MagicWandIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react'
import { useState } from 'react'
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
import { useSessionUser } from '@/app/session'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  SaveBar,
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
        <div className="grid grid-cols-4 gap-2.5 max-md:grid-cols-2">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-[5.75rem] rounded-row" />
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
    <div className="space-y-8 max-md:space-y-7">
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
  // An administrator may give a file of the server instead of an address.
  const files = useSessionUser().isAdministrator
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

  function add() {
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

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (dirty && !busy) save.mutate()
      }}
      noValidate
    >
      <Block title={text.title} count={entries.length}>
        <div className="mb-3 flex flex-wrap items-start justify-between gap-x-5 gap-y-2">
          <p className="max-w-[66ch] text-small text-ink-2">{text.help}</p>
          {guides.guides.length > 0 && (
            <Button
              size="sm"
              icon={ArrowsClockwiseIcon}
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
          )}
        </div>
        {entries.length === 0 ? (
          <EmptyState icon={CalendarBlankIcon} title={text.none}>
            {t.lineup.guidesNoneHelp}
          </EmptyState>
        ) : (
          <ol aria-label={text.title} className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
            {entries.map((entry, index) => {
              const label = entry.kind === 'kept' ? entry.guide.url : entry.url
              return (
                <li
                  key={entry.kind === 'kept' ? entry.guide.id : entry.key}
                  className="flex min-h-12 min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1 rounded-[0.625rem] bg-s1 py-1.5 pr-1.5 pl-3.5 max-sm:pl-3"
                >
                  <span className="figures w-5 shrink-0 text-right text-small text-ink-3">
                    {index + 1}
                  </span>
                  <div className="min-w-0 flex-1 basis-56 space-y-1">
                    <p className="figures text-small font-semibold break-all text-ink">{label}</p>
                    {entry.kind === 'new' ? (
                      <Badge tone="warn">{text.notSaved}</Badge>
                    ) : (
                      <GuideStatus guide={entry.guide} />
                    )}
                  </div>
                  <span className="ml-auto flex items-center">
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
        <div className="mt-4 flex flex-wrap items-start gap-x-2 gap-y-2.5">
          <Field
            label={files ? text.addressOrFile : text.address}
            help={full ? text.tooMany : files ? text.addressOrFileHint : text.addressHint}
            className="min-w-56 flex-1 max-sm:basis-full"
          >
            <TextInput
              type={files ? 'text' : 'url'}
              inputMode="url"
              size="sm"
              mono
              value={url}
              autoComplete="off"
              spellCheck={false}
              placeholder="https://…/guide.xml.gz"
              onValue={setUrl}
              // Enter adds the address; the dock saves the list.
              onKeyDown={(event) => {
                if (event.key !== 'Enter') return
                event.preventDefault()
                add()
              }}
            />
          </Field>
          <Button
            icon={PlusIcon}
            className="mt-7 max-sm:mt-0"
            disabled={url.trim() === '' || full}
            onClick={add}
          >
            {text.add}
          </Button>
        </div>
        {refresh.isError && (
          <div aria-live="polite" className="mt-4">
            <FieldError>{errorMessage(t, refresh.error)}</FieldError>
          </div>
        )}
      </Block>
      {(dirty || save.isPending || save.isError) && (
        <SaveBar
          dirty={dirty}
          saving={save.isPending}
          onDiscard={() => {
            save.reset()
            setEntries(initial)
          }}
          error={save.isError ? errorMessage(t, save.error) : undefined}
          saveLabel={text.save}
          savingLabel={text.saving}
        />
      )}
    </form>
  )
}

/** How a guide's last download went: a state with its icon and word, then its figures. */
function GuideStatus({ guide }: { guide: CatalogGuide }) {
  const { t } = useI18n()
  const number = useNumber()
  const text = t.lineup.guides
  return (
    <div className="space-y-1 text-micro">
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
            <span className="figures">
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
    <Block title={text.title}>
      <p className="mb-3 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      <div className="flex flex-wrap gap-2.5">
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
      <div aria-live="polite" className="mt-4 empty:hidden">
        {run.isError && !confirming && <FieldError>{errorMessage(t, run.error)}</FieldError>}
        {run.isSuccess && (
          <p className="figures text-small text-ink-2">
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
    </Block>
  )
}
