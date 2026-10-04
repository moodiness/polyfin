import { useId, useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  addChannelStream,
  deleteChannelStream,
  fetchLineupCategories,
  fetchLineupChannel,
  queryClient,
  queryKeys,
  saveChannelStreams,
  updateLineupChannel,
  type ChannelPatch,
  type LineupChannel,
  type Scope,
} from '@/api'
import { iptvCatalog, invalidateLineup } from '@/components/lineup/common'
import { MappingControls, mappingWords, MappingText } from '@/components/lineup/MappingControls'
import { ChannelLogo, fieldClass } from '@/components/lineup/shared'
import { Skeleton } from '@/components/panels'
import {
  Badge,
  buttonPrimary,
  buttonSecondary,
  Checkbox,
  MoveButtons,
  Notice,
  TextField,
} from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

/** Edits one channel: its name, logo, description, category, number, streams and guide. */
export default function ChannelEditor({
  scope,
  id,
  channelId,
}: {
  scope: Scope
  id: string
  channelId: string
}) {
  const { t } = useI18n()
  const key = [...queryKeys.lineup(scope, id), 'channel', channelId]
  const channel = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => fetchLineupChannel(scope, id, channelId, signal),
  })
  const show = (updated: LineupChannel) => {
    queryClient.setQueryData(key, updated)
    invalidateLineup(scope, id)
  }
  if (channel.isPending) return <Skeleton rows={6} label={t.common.loading} />
  if (channel.isError) return <Notice kind="error">{errorMessage(t, channel.error)}</Notice>
  return (
    <div className="space-y-8">
      <Details scope={scope} id={id} channel={channel.data} onSaved={show} />
      <Streams scope={scope} id={id} channel={channel.data} onSaved={show} />
      <Guide scope={scope} id={id} channel={channel.data} />
    </div>
  )
}

function Details({
  scope,
  id,
  channel,
  onSaved,
}: {
  scope: Scope
  id: string
  channel: LineupChannel
  onSaved: (channel: LineupChannel) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.editor
  const ids = { description: useId(), category: useId() }
  const [name, setName] = useState(channel.name)
  const [logo, setLogo] = useState(channel.logo ?? '')
  const [description, setDescription] = useState(channel.description)
  const [category, setCategory] = useState(channel.category.id)
  const [number, setNumber] = useState(
    channel.fixedNumber === null ? '' : String(channel.fixedNumber),
  )
  const categories = useQuery({
    queryKey: [...queryKeys.lineup(scope, id), 'categories'],
    queryFn: ({ signal }) => fetchLineupCategories(scope, id, signal),
  })
  const save = useMutation({
    mutationFn: (patch: ChannelPatch) => updateLineupChannel(scope, id, channel.id, patch),
    onSuccess: (updated) => {
      setName(updated.name)
      setLogo(updated.logo ?? '')
      setDescription(updated.description)
      setCategory(updated.category.id)
      setNumber(updated.fixedNumber === null ? '' : String(updated.fixedNumber))
      onSaved(updated)
    },
  })
  const numberValue = number.trim() === '' ? null : Number(number)
  const numberInvalid =
    numberValue !== null &&
    (!Number.isInteger(numberValue) || numberValue < 1 || numberValue > 99999)

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (numberInvalid) return
    const patch: ChannelPatch = {}
    if (name.trim() !== channel.name) patch.name = name.trim()
    if (logo.trim() !== (channel.logo ?? '')) patch.logo = logo.trim() === '' ? null : logo.trim()
    if (description !== channel.description) patch.description = description
    if (category !== channel.category.id) {
      patch.category = category === channel.providerCategoryId ? null : category
    }
    if (numberValue !== channel.fixedNumber) patch.number = numberValue
    save.mutate(patch)
  }

  return (
    <form onSubmit={submit} noValidate className="space-y-5">
      <div className="flex items-center gap-3">
        <ChannelLogo id={channel.id} logo={channel.logo} name={channel.name} size="size-14" />
        <div className="min-w-0 text-sm">
          <p className="truncate font-medium text-white">{channel.name}</p>
          <p className="text-xs text-muted">
            {channel.shown ? text.shownInApps : text.notShownInApps}
            {channel.number !== null && ` · ${text.numberNow(channel.number)}`}
          </p>
        </div>
      </div>
      <div className="space-y-2">
        <TextField
          label={text.name}
          hint={text.providerValue(channel.providerName)}
          value={name}
          maxLength={100}
          autoComplete="off"
          onValue={setName}
        />
        {channel.renamed && (
          <button type="button" className={linkButton} onClick={() => save.mutate({ name: null })}>
            {text.resetName}
          </button>
        )}
      </div>
      <div className="space-y-2">
        <TextField
          label={text.logo}
          hint={channel.providerLogo ? text.providerValue(channel.providerLogo) : text.logoHint}
          type="url"
          inputMode="url"
          value={logo}
          autoComplete="off"
          spellCheck={false}
          placeholder="https://…/logo.png"
          onValue={setLogo}
        />
        {channel.logo !== channel.providerLogo && (
          <button type="button" className={linkButton} onClick={() => save.mutate({ logo: null })}>
            {text.resetLogo}
          </button>
        )}
      </div>
      <div>
        <label htmlFor={ids.description} className="block text-sm font-medium text-zinc-200">
          {text.description}
        </label>
        <textarea
          id={ids.description}
          value={description}
          maxLength={2000}
          rows={3}
          onChange={(event) => setDescription(event.target.value)}
          className={fieldClass}
        />
      </div>
      <div>
        <label htmlFor={ids.category} className="block text-sm font-medium text-zinc-200">
          {text.category}
        </label>
        <select
          id={ids.category}
          value={category}
          onChange={(event) => setCategory(event.target.value)}
          aria-describedby={`${ids.category}-hint`}
          className={fieldClass}
        >
          {(categories.data?.items ?? [channel.category]).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
              {c.id === channel.providerCategoryId ? ` ${text.providerCategory}` : ''}
            </option>
          ))}
        </select>
        {channel.moved && (
          <p id={`${ids.category}-hint`} className="mt-1 text-xs text-muted">
            {text.movedHint}
          </p>
        )}
      </div>
      <TextField
        label={text.number}
        hint={
          channel.providerNumber !== null
            ? `${text.numberHint} ${text.providerValue(String(channel.providerNumber))}`
            : text.numberHint
        }
        error={numberInvalid ? text.numberInvalid : undefined}
        type="number"
        inputMode="numeric"
        min={1}
        max={99999}
        step={1}
        value={number}
        onValue={setNumber}
      />
      {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
      {save.isSuccess && <Notice kind="success">{text.saved}</Notice>}
      <button type="submit" className={buttonPrimary} disabled={save.isPending}>
        {save.isPending ? t.common.saving : t.common.save}
      </button>
    </form>
  )
}

function Streams({
  scope,
  id,
  channel,
  onSaved,
}: {
  scope: Scope
  id: string
  channel: LineupChannel
  onSaved: (channel: LineupChannel) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.streams
  const titleId = useId()
  const [streams, setStreams] = useState(channel.streams)
  const [url, setUrl] = useState('')
  const [label, setLabel] = useState('')
  const dirty =
    streams.map((s) => `${s.id}:${s.enabled}`).join() !==
    channel.streams.map((s) => `${s.id}:${s.enabled}`).join()
  const save = useMutation({
    mutationFn: () =>
      saveChannelStreams(
        scope,
        id,
        channel.id,
        streams.map((s) => ({ id: s.id, enabled: s.enabled })),
      ),
    onSuccess: (updated) => {
      setStreams(updated.streams)
      onSaved(updated)
    },
  })
  const add = useMutation({
    mutationFn: () =>
      addChannelStream(scope, id, channel.id, {
        url: url.trim(),
        label: label.trim() || text.defaultLabel,
      }),
    onSuccess: (updated) => {
      setUrl('')
      setLabel('')
      setStreams(updated.streams)
      onSaved(updated)
    },
  })
  const remove = useMutation({
    mutationFn: (streamId: string) => deleteChannelStream(scope, id, channel.id, streamId),
    onSuccess: (updated) => {
      setStreams(updated.streams)
      onSaved(updated)
    },
  })

  function move(from: number, to: number) {
    setStreams((current) => {
      const next = [...current]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return next
    })
  }

  return (
    <section aria-labelledby={titleId} className="space-y-3 border-t border-line pt-6">
      <h3 id={titleId} className="font-semibold text-white">
        {text.title}
      </h3>
      <p className="text-xs text-muted">{text.help}</p>
      <ol className="divide-y divide-line rounded-lg border border-line">
        {streams.map((stream, index) => (
          <li key={stream.id} className="flex flex-wrap items-center gap-3 p-3 text-sm">
            <span className="w-5 text-right text-xs text-muted tabular-nums">{index + 1}</span>
            <div className="min-w-0 flex-1">
              <Checkbox
                label={stream.label || text.unnamed}
                help={stream.custom ? (stream.address ?? '') : text.provider}
                checked={stream.enabled}
                onChange={(enabled) =>
                  setStreams((current) =>
                    current.map((s) => (s.id === stream.id ? { ...s, enabled } : s)),
                  )
                }
              />
            </div>
            {stream.custom && <Badge tone="fin">{text.custom}</Badge>}
            <MoveButtons
              name={stream.label}
              index={index}
              count={streams.length}
              onMove={(to) => move(index, to)}
            />
            {stream.custom && (
              <button
                type="button"
                className={rowButton}
                disabled={remove.isPending || dirty}
                title={dirty ? text.saveFirst : undefined}
                aria-label={text.removeLabel(stream.label)}
                onClick={() => remove.mutate(stream.id)}
              >
                {text.remove}
              </button>
            )}
          </li>
        ))}
      </ol>
      {(save.isError || remove.isError) && (
        <Notice kind="error">{errorMessage(t, save.error ?? remove.error)}</Notice>
      )}
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          className={buttonPrimary}
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate()}
        >
          {save.isPending ? t.common.saving : text.save}
        </button>
        <button
          type="button"
          className={buttonSecondary}
          disabled={!dirty}
          onClick={() => setStreams(channel.streams)}
        >
          {t.libraries.reset}
        </button>
      </div>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          if (url.trim() !== '') add.mutate()
        }}
        noValidate
        className="space-y-3 rounded-lg border border-line bg-ink/40 p-3"
      >
        <h4 className="text-sm font-medium text-zinc-200">{text.addTitle}</h4>
        <TextField
          label={text.url}
          hint={text.urlHint}
          type="url"
          inputMode="url"
          value={url}
          autoComplete="off"
          spellCheck={false}
          placeholder="https://…/stream.m3u8"
          onValue={(value) => {
            add.reset()
            setUrl(value)
          }}
        />
        <TextField
          label={text.label}
          hint={text.labelHint}
          value={label}
          maxLength={32}
          autoComplete="off"
          placeholder={text.defaultLabel}
          onValue={setLabel}
        />
        {add.isError && <Notice kind="error">{errorMessage(t, add.error)}</Notice>}
        <button
          type="submit"
          className={buttonSecondary}
          disabled={add.isPending || url.trim() === '' || dirty}
          title={dirty ? text.saveFirst : undefined}
        >
          {add.isPending ? text.adding : text.add}
        </button>
      </form>
    </section>
  )
}

function Guide({ scope, id, channel }: { scope: Scope; id: string; channel: LineupChannel }) {
  const { t } = useI18n()
  const text = t.lineup.editor
  const titleId = useId()
  const [changed, setChanged] = useState<string | null>(null)
  const key = [...queryKeys.lineup(scope, id), 'channel', channel.id]
  return (
    <section aria-labelledby={titleId} className="space-y-3 border-t border-line pt-6">
      <h3 id={titleId} className="font-semibold text-white">
        {text.guideTitle}
      </h3>
      <dl className="grid gap-2 text-sm sm:grid-cols-[auto_1fr]">
        <dt className="text-muted">{text.providerGuideId}</dt>
        <dd className="font-mono text-xs break-all text-zinc-200 sm:self-center">
          {channel.guideId || '–'}
        </dd>
        <dt className="text-muted">{text.mappedTo}</dt>
        <dd>
          <MappingText mapping={channel.mapping} />
        </dd>
      </dl>
      <MappingControls
        scope={scope}
        target={iptvCatalog(id)}
        channelId={channel.id}
        channelName={channel.name}
        mapping={channel.mapping}
        onChanged={(item) => {
          setChanged(t.lineup.mapping.changed(channel.name, mappingWords(t, item)))
          queryClient.setQueryData<LineupChannel>(key, (old) =>
            old ? { ...old, mapping: item.mapping } : old,
          )
          invalidateLineup(scope, id)
        }}
      />
      <p aria-live="polite" className="text-xs text-emerald-300 empty:hidden">
        {changed}
      </p>
    </section>
  )
}

const linkButton =
  'text-xs font-medium text-fin-5 underline decoration-fin-5/40 underline-offset-4 hover:decoration-fin-5'
const rowButton =
  'inline-flex min-h-9 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4 disabled:cursor-not-allowed disabled:opacity-50'
