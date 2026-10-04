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
import { icons } from '@/components/icons'
import { Empty, Panel, Skeleton, Stat, StatusText } from '@/components/panels'
import {
  buttonPrimary,
  buttonSecondary,
  ConfirmButton,
  MoveButtons,
  Notice,
  RelativeTime,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** The most guides a catalog takes. */
const maxGuides = 10

/** A guide of the list being edited: one kept, sent by its id, or an address added. */
type Entry = { kind: 'kept'; guide: CatalogGuide } | { kind: 'new'; url: string; key: string }

/**
 * A Live TV catalog's XMLTV guides, in the order channels take them, with how each was last
 * downloaded, the mapping counts, and the automatic mapping.
 */
export default function CatalogGuides({
  scope,
  target,
  onChanged,
}: {
  scope: Scope
  target: CatalogTarget
  onChanged?: () => void
}) {
  const { t } = useI18n()
  const guides = useQuery({
    queryKey: queryKeys.catalogGuides(scope, target),
    queryFn: ({ signal }) => fetchCatalogGuides(scope, target, signal),
  })
  if (guides.isPending) return <Skeleton rows={4} label={t.common.loading} />
  if (guides.isError) return <Notice kind="error">{errorMessage(t, guides.error)}</Notice>
  return (
    <div className="space-y-6">
      <Counts guides={guides.data} />
      <GuideList
        key={guides.data.guides.map((g) => g.id).join()}
        scope={scope}
        target={target}
        guides={guides.data}
        onChanged={onChanged}
      />
      <Automap scope={scope} target={target} onChanged={onChanged} />
    </div>
  )
}

function Counts({ guides }: { guides: Guides }) {
  const { language, t } = useI18n()
  const text = t.lineup.guides
  const number = (n: number) => n.toLocaleString(language)
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Stat label={text.channels} value={number(guides.channels)} />
      <Stat
        label={text.mapped}
        value={number(guides.mapped)}
        tone={guides.channels > 0 && guides.mapped === guides.channels ? 'ok' : undefined}
      />
      <Stat
        label={text.unmapped}
        value={number(guides.channels - guides.mapped)}
        tone={guides.channels - guides.mapped > 0 ? 'warning' : undefined}
      />
      <Stat label={text.manual} value={number(guides.manual)} />
    </div>
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
  const text = t.lineup.guides
  const initial: Entry[] = guides.guides.map((guide) => ({ kind: 'kept', guide }))
  const [entries, setEntries] = useState<Entry[]>(initial)
  const [url, setUrl] = useState('')
  const [counter, setCounter] = useState(0)
  const dirty =
    entries.length !== initial.length ||
    entries.some((entry, i) => entry.kind === 'new' || entry.guide.id !== guides.guides[i].id)
  const settle = (answer: Guides) => {
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
    onSuccess: settle,
  })
  const refresh = useMutation({
    mutationFn: () => refreshCatalogGuides(scope, target),
    onSuccess: settle,
  })
  const busy = save.isPending || refresh.isPending

  function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (url.trim() === '' || entries.length >= maxGuides) return
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
    <Panel
      title={text.title}
      description={text.help}
      actions={
        guides.guides.length > 0 ? (
          <button
            type="button"
            className={buttonSecondary}
            disabled={busy || dirty}
            title={dirty ? text.saveFirst : undefined}
            onClick={() => {
              save.reset()
              refresh.mutate()
            }}
          >
            <icons.download className="size-4" />
            {refresh.isPending ? text.refreshing : text.refresh}
          </button>
        ) : undefined
      }
    >
      <div className="space-y-4">
        {entries.length === 0 ? (
          <Empty>{text.none}</Empty>
        ) : (
          <ol className="divide-y divide-line rounded-xl border border-line">
            {entries.map((entry, index) => {
              const label = entry.kind === 'kept' ? entry.guide.url : entry.url
              return (
                <li
                  key={entry.kind === 'kept' ? entry.guide.id : entry.key}
                  className="p-3 text-sm"
                >
                  <div className="flex flex-wrap items-start gap-3">
                    <span className="mt-0.5 w-5 text-right text-xs text-muted tabular-nums">
                      {index + 1}
                    </span>
                    <div className="min-w-0 flex-1 space-y-1">
                      <p className="font-mono text-xs break-all text-zinc-100">{label}</p>
                      {entry.kind === 'new' ? (
                        <p className="text-xs text-amber-200">{text.notSaved}</p>
                      ) : (
                        <GuideStatus guide={entry.guide} />
                      )}
                    </div>
                    <MoveButtons
                      name={label}
                      index={index}
                      count={entries.length}
                      onMove={(to) => move(index, to)}
                    />
                    <button
                      type="button"
                      className={rowButton}
                      aria-label={text.removeLabel(label)}
                      onClick={() => {
                        save.reset()
                        setEntries((current) => current.filter((_, i) => i !== index))
                      }}
                    >
                      {text.remove}
                    </button>
                  </div>
                </li>
              )
            })}
          </ol>
        )}
        <form onSubmit={add} noValidate className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="flex-1">
            <TextField
              label={text.address}
              hint={entries.length >= maxGuides ? text.tooMany : text.addressHint}
              type="url"
              inputMode="url"
              value={url}
              autoComplete="off"
              spellCheck={false}
              placeholder="https://…/guide.xml.gz"
              onValue={setUrl}
            />
          </div>
          <button
            type="submit"
            className={`${buttonSecondary} sm:mb-5`}
            disabled={url.trim() === '' || entries.length >= maxGuides}
          >
            {text.add}
          </button>
        </form>
        <div aria-live="polite" className="space-y-2 empty:hidden">
          {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
          {refresh.isError && <Notice kind="error">{errorMessage(t, refresh.error)}</Notice>}
          {save.isSuccess && !dirty && <Notice kind="success">{text.saved}</Notice>}
          {refresh.isSuccess && <Notice kind="success">{text.refreshed}</Notice>}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
          <p className={`text-sm ${dirty ? 'text-amber-200' : 'text-muted'}`}>
            {dirty ? t.dashboard.settings.unsaved : t.dashboard.settings.upToDate}
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              className={buttonSecondary}
              disabled={!dirty || busy}
              onClick={() => setEntries(initial)}
            >
              {t.libraries.reset}
            </button>
            <button
              type="button"
              className={buttonPrimary}
              disabled={!dirty || busy}
              onClick={() => save.mutate()}
            >
              {save.isPending ? text.saving : text.save}
            </button>
          </div>
        </div>
      </div>
    </Panel>
  )
}

/** How a guide's last download went. */
export function GuideStatus({ guide }: { guide: CatalogGuide }) {
  const { language, t } = useI18n()
  const text = t.lineup.guides
  const number = (n: number) => n.toLocaleString(language)
  return (
    <div className="space-y-1 text-xs">
      <p className="flex flex-wrap items-center gap-x-3 gap-y-1">
        {guide.error !== '' ? (
          <StatusText tone="warning">{text.failing}</StatusText>
        ) : guide.fetchedAt ? (
          <StatusText tone="ok">{text.ok}</StatusText>
        ) : (
          <StatusText tone="muted">{text.notYet}</StatusText>
        )}
        {guide.fetchedAt && (
          <span className="text-muted">
            {text.fetched} <RelativeTime iso={guide.fetchedAt} /> ·{' '}
            {text.contents(number(guide.channels), number(guide.programmes))}
          </span>
        )}
        {guide.nextAt && (
          <span className="text-muted">
            {text.next} <RelativeTime iso={guide.nextAt} />
          </span>
        )}
      </p>
      {guide.error !== '' && (
        <p className="text-amber-200">{t.libraries.guideErrors[guide.error] ?? t.errors.generic}</p>
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
  const { language, t } = useI18n()
  const text = t.lineup.automap
  const run = useMutation({
    mutationFn: (mode: 'unmapped' | 'remap') => automapCatalog(scope, target, mode),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.catalogGuides(scope, target) })
      onChanged?.()
    },
  })
  const number = (n: number) => n.toLocaleString(language)
  return (
    <Panel title={text.title} description={text.help}>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          className={buttonPrimary}
          disabled={run.isPending}
          onClick={() => run.mutate('unmapped')}
        >
          {run.isPending && run.variables === 'unmapped' ? text.running : text.unmapped}
        </button>
        <ConfirmButton
          label={text.remap}
          busyLabel={text.running}
          message={text.remapConfirm}
          busy={run.isPending}
          onConfirm={() => run.mutate('remap')}
        />
      </div>
      <div aria-live="polite" className="mt-3 empty:hidden">
        {run.isError && <Notice kind="error">{errorMessage(t, run.error)}</Notice>}
        {run.isSuccess && (
          <Notice kind="success">
            {text.done(
              number(run.data.changed),
              number(run.data.mapped),
              number(run.data.channels),
            )}
          </Notice>
        )}
      </div>
    </Panel>
  )
}

const rowButton =
  'inline-flex min-h-9 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4'
