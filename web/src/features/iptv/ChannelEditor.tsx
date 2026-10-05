import { ArrowCounterClockwiseIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react'
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
import { invalidateLineup, iptvCatalog } from '@/components/lineup/common'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Button,
  Checkbox,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Select,
  Skeleton,
  SkeletonText,
  StatusPill,
  Textarea,
  TextInput,
  useToast,
} from '@/ui'
import { MappingControls, mappingWords, MappingText } from './MappingControls'
import { ChannelLogo, MoveButtons } from './shared'

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
  if (channel.isPending) {
    return (
      <div role="status" aria-label={t.common.loading} className="space-y-6 p-5">
        <div className="flex items-center gap-3">
          <Skeleton className="size-14 rounded-field" />
          <SkeletonText className="flex-1" />
        </div>
        {Array.from({ length: 4 }, (_, index) => (
          <span key={index} className="block space-y-2">
            <Skeleton className="h-3 w-24" />
            <Skeleton className="h-10 w-full" />
          </span>
        ))}
      </div>
    )
  }
  if (channel.isError) {
    return (
      <div className="p-5">
        <InlineError onRetry={() => void channel.refetch()} retrying={channel.isFetching}>
          {errorMessage(t, channel.error)}
        </InlineError>
      </div>
    )
  }
  return (
    <div className="divide-y divide-line">
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
  const toast = useToast()
  const text = t.lineup.editor
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
      toast(text.saved)
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
    <form onSubmit={submit} noValidate className="space-y-5 p-5">
      <div className="flex items-center gap-3.5">
        <ChannelLogo id={channel.id} logo={channel.logo} name={channel.name} large />
        <div className="min-w-0">
          <p className="truncate text-[15px] font-semibold text-ink">{channel.name}</p>
          <p className="mt-1 flex flex-wrap items-center gap-2 text-small text-ink-3">
            <StatusPill tone={channel.shown ? 'ok' : 'muted'}>
              {channel.shown ? text.shownInApps : text.notShownInApps}
            </StatusPill>
            {channel.number !== null && (
              <span className="tabular-nums">{text.numberNow(channel.number)}</span>
            )}
          </p>
        </div>
      </div>
      <Field
        label={text.name}
        help={text.providerValue(channel.providerName)}
        aside={
          channel.renamed && (
            <Button
              size="sm"
              variant="ghost"
              icon={ArrowCounterClockwiseIcon}
              onClick={() => save.mutate({ name: null })}
            >
              {text.resetName}
            </Button>
          )
        }
      >
        <TextInput value={name} maxLength={100} autoComplete="off" onValue={setName} />
      </Field>
      <Field
        label={text.logo}
        help={channel.providerLogo ? text.providerValue(channel.providerLogo) : text.logoHint}
        aside={
          channel.logo !== channel.providerLogo && (
            <Button
              size="sm"
              variant="ghost"
              icon={ArrowCounterClockwiseIcon}
              onClick={() => save.mutate({ logo: null })}
            >
              {text.resetLogo}
            </Button>
          )
        }
      >
        <TextInput
          type="url"
          inputMode="url"
          value={logo}
          autoComplete="off"
          spellCheck={false}
          placeholder="https://…/logo.png"
          onValue={setLogo}
        />
      </Field>
      <Field label={text.description}>
        <Textarea value={description} maxLength={2000} rows={3} onValue={setDescription} />
      </Field>
      <Field label={text.category} help={channel.moved ? text.movedHint : undefined}>
        <Select
          value={category}
          options={(categories.data?.items ?? [channel.category]).map((c) => ({
            value: c.id,
            label: `${c.name || t.lineup.exclusions.noGroup}${
              c.id === channel.providerCategoryId ? ` ${text.providerCategory}` : ''
            }`,
          }))}
          onValue={setCategory}
        />
      </Field>
      <Field
        label={text.number}
        help={
          channel.providerNumber !== null
            ? `${text.numberHint} ${text.providerValue(String(channel.providerNumber))}`
            : text.numberHint
        }
        error={numberInvalid ? text.numberInvalid : undefined}
      >
        <TextInput
          type="number"
          inputMode="numeric"
          mono
          min={1}
          max={99999}
          step={1}
          value={number}
          onValue={setNumber}
          className="max-w-40"
        />
      </Field>
      {save.isError && <FieldError>{errorMessage(t, save.error)}</FieldError>}
      <Button type="submit" variant="primary" loading={save.isPending}>
        {save.isPending ? t.common.saving : t.common.save}
      </Button>
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
  const toast = useToast()
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
      toast(t.lineup.editor.saved)
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
    <section aria-labelledby={titleId} className="space-y-4 p-5">
      <div>
        <h3 id={titleId} className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
          {text.title}
        </h3>
        <p className="mt-1 text-small text-ink-3">{text.help}</p>
      </div>
      <ol className="rounded-row border border-line-2 bg-bg">
        {streams.map((stream, index) => (
          <li
            key={stream.id}
            className="flex flex-wrap items-center gap-3 px-3.5 py-3 not-first:border-t not-first:border-line"
          >
            <span className="figures w-4 text-right text-[12.5px] text-ink-3">{index + 1}</span>
            <div className="min-w-0 flex-1 basis-40">
              <Checkbox
                label={stream.label || text.unnamed}
                help={
                  stream.custom ? (
                    <span className="figures break-all">{stream.address ?? ''}</span>
                  ) : (
                    text.provider
                  )
                }
                checked={stream.enabled}
                onChange={(enabled) =>
                  setStreams((current) =>
                    current.map((s) => (s.id === stream.id ? { ...s, enabled } : s)),
                  )
                }
              />
            </div>
            {stream.custom && <Badge tone="accent">{text.custom}</Badge>}
            <span className="flex items-center">
              <MoveButtons
                name={stream.label}
                index={index}
                count={streams.length}
                onMove={(to) => move(index, to)}
              />
              {stream.custom && (
                <IconButton
                  size="sm"
                  danger
                  icon={TrashIcon}
                  disabled={remove.isPending || dirty}
                  title={dirty ? text.saveFirst : undefined}
                  label={text.removeLabel(stream.label)}
                  onClick={() => remove.mutate(stream.id)}
                />
              )}
            </span>
          </li>
        ))}
      </ol>
      {dirty && <p className="text-small text-warn">{text.saveFirst}</p>}
      {(save.isError || remove.isError) && (
        <FieldError>{errorMessage(t, save.error ?? remove.error)}</FieldError>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="primary"
          loading={save.isPending}
          disabled={!dirty}
          onClick={() => save.mutate()}
        >
          {save.isPending ? t.common.saving : text.save}
        </Button>
        <Button variant="ghost" disabled={!dirty} onClick={() => setStreams(channel.streams)}>
          {t.lineup.reset}
        </Button>
      </div>
      <form
        onSubmit={(event) => {
          event.preventDefault()
          if (url.trim() !== '') add.mutate()
        }}
        noValidate
        className="space-y-4 rounded-row border border-line-2 bg-s2/50 p-4"
      >
        <h4 className="text-control font-semibold text-ink">{text.addTitle}</h4>
        <Field
          label={text.url}
          help={text.urlHint}
          error={add.isError ? errorMessage(t, add.error) : undefined}
        >
          <TextInput
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
        </Field>
        <Field label={text.label} help={text.labelHint}>
          <TextInput
            value={label}
            maxLength={32}
            autoComplete="off"
            placeholder={text.defaultLabel}
            onValue={setLabel}
          />
        </Field>
        <Button
          type="submit"
          icon={PlusIcon}
          loading={add.isPending}
          disabled={url.trim() === '' || dirty}
          title={dirty ? text.saveFirst : undefined}
        >
          {add.isPending ? text.adding : text.add}
        </Button>
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
    <section aria-labelledby={titleId} className="space-y-4 p-5">
      <h3 id={titleId} className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
        {text.guideTitle}
      </h3>
      <dl className="grid gap-x-4 gap-y-2 text-control sm:grid-cols-[auto_1fr]">
        <dt className="text-ink-3">{text.providerGuideId}</dt>
        <dd className="figures text-small break-all text-ink sm:self-center">
          {channel.guideId || '–'}
        </dd>
        <dt className="text-ink-3">{text.mappedTo}</dt>
        <dd>
          <MappingText mapping={channel.mapping} />
        </dd>
      </dl>
      <div className="flex flex-wrap items-center gap-2">
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
      </div>
      <p aria-live="polite" className="text-small text-ok empty:hidden">
        {changed}
      </p>
    </section>
  )
}
