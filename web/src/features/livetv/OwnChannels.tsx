import {
  FilmSlateIcon,
  MagnifyingGlassIcon,
  PlusIcon,
  TelevisionSimpleIcon,
  TrashIcon,
  XIcon,
} from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router'
import {
  ApiError,
  createOwnChannel,
  deleteOwnChannel,
  fetchChannelChoices,
  fetchOwnChannel,
  fetchOwnChannels,
  queryClient,
  queryKeys,
  saveOwnChannel,
  searchChannelTitles,
  type ChannelChoices,
  type ChannelSource,
  type ChannelSourceKind,
  type OwnChannel,
  type OwnChannelDraft,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, formatEpisode, formatSpan } from '@/format'
import { useI18n } from '@/i18n'
import {
  Block,
  Button,
  ButtonLink,
  Checkbox,
  ConfirmDialog,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  NumberInput,
  Panel,
  PanelFooter,
  PanelSection,
  Row,
  RowList,
  Segmented,
  Select,
  SkeletonRows,
  Switch,
  TextInput,
  useToast,
} from '@/ui'

/** The page of a channel of the server's titles, `new` for one being made. */
export function ownChannelPath(id: string): string {
  return `/live-tv/channels/${id}`
}

const kinds: ChannelSourceKind[] = ['collection', 'smartlist', 'series', 'titles']

/** A time of the day in minutes, as an `<input type="time">` writes it. */
function clock(minutes: number): string {
  const m = Math.min(Math.max(minutes, 0), 24 * 60 - 1)
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
}

function minutesOf(value: string, end: boolean): number {
  const [h, m] = value.split(':').map(Number)
  const minutes = (h || 0) * 60 + (m || 0)
  // A block that ends at midnight ends at the end of the day.
  return end && minutes === 0 ? 24 * 60 : minutes
}

/**
 * The channels Polyfin airs from the server's titles, on the Live TV page, each opening its page,
 * with what it airs now.
 */
export function OwnChannels() {
  const { t, language } = useI18n()
  const channels = useQuery({
    queryKey: queryKeys.ownChannels,
    queryFn: ({ signal }) => fetchOwnChannels(signal),
  })
  const add = (
    <ButtonLink to={ownChannelPath('new')} icon={PlusIcon} size="sm">
      {t.ownChannels.add}
    </ButtonLink>
  )
  let body
  if (channels.isPending) body = <SkeletonRows rows={2} label={t.ownChannels.loading} />
  else if (channels.isError)
    body = (
      <InlineError onRetry={() => void channels.refetch()} retrying={channels.isFetching}>
        {errorMessage(t, channels.error)}
      </InlineError>
    )
  else if (channels.data.length === 0)
    body = (
      <EmptyState icon={FilmSlateIcon} title={t.ownChannels.emptyTitle} action={add}>
        {t.ownChannels.empty}
      </EmptyState>
    )
  else
    body = (
      <RowList aria-label={t.ownChannels.label}>
        {channels.data.map((channel) => {
          const parts = [
            t.ownChannels.entries(channel.entries, channel.entries.toLocaleString(language)),
            formatSpan(channel.runtime, language),
          ]
          if (channel.blocks.length > 0) parts.push(t.ownChannels.blockCount(channel.blocks.length))
          const now = channel.airing[0]
          return (
            <Row
              key={channel.id}
              leading={FilmSlateIcon}
              to={ownChannelPath(channel.id)}
              title={channel.number === null ? channel.name : `${channel.number} · ${channel.name}`}
              muted={!channel.enabled}
              meta={
                <span className="truncate">
                  {channel.enabled ? parts.join(' · ') : t.ownChannels.off}
                  {channel.enabled && now && ` · ${t.ownChannels.now(now.name)}`}
                </span>
              }
            />
          )
        })}
      </RowList>
    )
  return (
    <Block
      title={t.ownChannels.title}
      count={channels.data?.length}
      aside={channels.data && channels.data.length > 0 ? add : undefined}
    >
      <p className="mb-5 max-w-[72ch] text-small text-ink-2">{t.ownChannels.help}</p>
      {body}
    </Block>
  )
}

const emptyDraft: OwnChannelDraft = {
  name: '',
  number: null,
  enabled: true,
  source: { kind: 'collection', items: [] },
  shuffle: false,
  blocks: [],
}

/** `/live-tv/channels/:id`: makes or changes a channel of the server's titles. */
export default function OwnChannelRoute() {
  const { t } = useI18n()
  const { id = 'new' } = useParams()
  const isNew = id === 'new'
  const back = { to: '/live-tv', label: t.ownChannels.back }
  const channel = useQuery({
    queryKey: queryKeys.ownChannel(id),
    queryFn: ({ signal }) => fetchOwnChannel(id, signal),
    enabled: !isNew,
  })
  const choices = useQuery({
    queryKey: queryKeys.channelChoices,
    queryFn: ({ signal }) => fetchChannelChoices(signal),
  })
  if (!isNew && channel.isError) {
    const missing = channel.error instanceof ApiError && channel.error.status === 404
    return (
      <PageLayout title={t.ownChannels.title} back={back}>
        {missing ? (
          <EmptyState icon={TelevisionSimpleIcon} title={t.ownChannels.notFound} />
        ) : (
          <InlineError onRetry={() => void channel.refetch()} retrying={channel.isFetching}>
            {errorMessage(t, channel.error)}
          </InlineError>
        )}
      </PageLayout>
    )
  }
  if ((!isNew && channel.isPending) || choices.isPending || choices.isError) {
    return (
      <PageLayout title={t.ownChannels.newTitle} back={back}>
        {choices.isError ? (
          <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
            {errorMessage(t, choices.error)}
          </InlineError>
        ) : (
          <SkeletonRows rows={4} label={t.ownChannels.loading} />
        )}
      </PageLayout>
    )
  }
  return (
    <Editor
      key={id}
      saved={isNew ? null : (channel.data ?? null)}
      choices={choices.data}
      back={back}
    />
  )
}

function Editor({
  saved,
  choices,
  back,
}: {
  saved: OwnChannel | null
  choices: ChannelChoices
  back: { to: string; label: string }
}) {
  const { t, language } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const [draft, setDraft] = useState<OwnChannelDraft>(() =>
    saved
      ? {
          name: saved.name,
          number: saved.number,
          enabled: saved.enabled,
          source: saved.source,
          shuffle: saved.shuffle,
          blocks: saved.blocks.map(({ start, end, source, shuffle }) => ({
            start,
            end,
            source,
            shuffle,
          })),
        }
      : emptyDraft,
  )
  const [rebuild, setRebuild] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const change = (patch: Partial<OwnChannelDraft>) => setDraft((d) => ({ ...d, ...patch }))

  const save = useMutation({
    mutationFn: () =>
      saved ? saveOwnChannel(saved.id, { ...draft, rebuild }) : createOwnChannel(draft),
    onSuccess: (channel) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownChannels })
      queryClient.setQueryData(queryKeys.ownChannel(channel.id), channel)
      setRebuild(false)
      toast(saved ? t.ownChannels.saved : t.ownChannels.created)
      if (!saved) void navigate(ownChannelPath(channel.id), { replace: true })
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteOwnChannel(saved?.id ?? ''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.ownChannels })
      toast(t.ownChannels.deleted)
      void navigate(back.to)
    },
  })
  const code = save.error instanceof ApiError ? save.error.code : null
  const nameError = code === 'invalid_own_channel_name' ? errorMessage(t, save.error) : undefined
  const numberError = code === 'invalid_channel_number' ? errorMessage(t, save.error) : undefined
  const generalError =
    save.isError && !nameError && !numberError ? errorMessage(t, save.error) : undefined

  return (
    <PageLayout
      title={saved ? saved.name : t.ownChannels.newTitle}
      lede={t.ownChannels.lede}
      back={back}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          save.mutate()
        }}
      >
        <Panel flush>
          <PanelSection>
            <div className="grid gap-6 md:grid-cols-3">
              <Field label={t.ownChannels.name} error={nameError}>
                <TextInput value={draft.name} maxLength={64} onValue={(name) => change({ name })} />
              </Field>
              <Field
                label={t.ownChannels.number}
                help={t.ownChannels.numberHelp}
                error={numberError}
              >
                <NumberInput
                  value={draft.number}
                  min={1}
                  max={9999}
                  step={1}
                  placeholder={t.ownChannels.noNumber}
                  onValue={(number) => change({ number })}
                />
              </Field>
              <Field label={t.ownChannels.enabled}>
                <Switch checked={draft.enabled} onChange={(enabled) => change({ enabled })} />
              </Field>
            </div>
          </PanelSection>
          <PanelSection title={t.ownChannels.programmes} description={t.ownChannels.programmesHelp}>
            <SourcePicker
              source={draft.source}
              choices={choices}
              onChange={(source) => change({ source })}
            />
            <div className="mt-4">
              <Checkbox
                checked={draft.shuffle}
                onChange={(shuffle) => change({ shuffle })}
                label={t.ownChannels.shuffle}
                help={t.ownChannels.shuffleHelp}
              />
            </div>
          </PanelSection>
          <PanelSection title={t.ownChannels.blocks} description={t.ownChannels.blocksHelp}>
            <div className="space-y-6">
              {draft.blocks.map((block, i) => {
                const set = (patch: Partial<OwnChannelDraft['blocks'][number]>) =>
                  change({ blocks: draft.blocks.map((b, j) => (j === i ? { ...b, ...patch } : b)) })
                return (
                  <fieldset key={i} className="rounded-field border border-line p-4">
                    <legend className="px-1 text-control font-medium text-ink">
                      {t.ownChannels.block(i + 1)}
                    </legend>
                    <div className="mb-4 flex flex-wrap items-end gap-4">
                      <Field label={t.ownChannels.from}>
                        <TextInput
                          type="time"
                          value={clock(block.start)}
                          onValue={(value) => set({ start: minutesOf(value, false) })}
                          className="w-[140px]"
                        />
                      </Field>
                      <Field label={t.ownChannels.until}>
                        <TextInput
                          type="time"
                          value={clock(block.end === 24 * 60 ? 0 : block.end)}
                          onValue={(value) => set({ end: minutesOf(value, true) })}
                          className="w-[140px]"
                        />
                      </Field>
                      <Button
                        variant="danger"
                        size="sm"
                        icon={TrashIcon}
                        onClick={() => change({ blocks: draft.blocks.filter((_, j) => j !== i) })}
                      >
                        {t.ownChannels.removeBlock(i + 1)}
                      </Button>
                    </div>
                    <SourcePicker
                      source={block.source}
                      choices={choices}
                      onChange={(source) => set({ source })}
                    />
                    <div className="mt-4">
                      <Checkbox
                        checked={block.shuffle}
                        onChange={(shuffle) => set({ shuffle })}
                        label={t.ownChannels.shuffle}
                      />
                    </div>
                  </fieldset>
                )
              })}
              <Button
                icon={PlusIcon}
                size="sm"
                disabled={draft.blocks.length >= 12}
                onClick={() =>
                  change({
                    blocks: [
                      ...draft.blocks,
                      {
                        start: 18 * 60,
                        end: 20 * 60,
                        source: { kind: 'series', items: [] },
                        shuffle: false,
                      },
                    ],
                  })
                }
              >
                {t.ownChannels.addBlock}
              </Button>
            </div>
          </PanelSection>
          {saved && (
            <PanelSection title={t.ownChannels.airing} description={t.ownChannels.airingHelp}>
              {saved.airing.length === 0 ? (
                <p className="text-small text-ink-3">{t.ownChannels.nothing}</p>
              ) : (
                <ul className="space-y-1.5 text-small">
                  {saved.airing.map((airing) => (
                    <li key={airing.start} className="flex gap-3">
                      <span className="figures w-[110px] shrink-0 text-ink-3">
                        {new Intl.DateTimeFormat(language, { timeStyle: 'short' }).format(
                          new Date(airing.start),
                        )}
                        {' – '}
                        {new Intl.DateTimeFormat(language, { timeStyle: 'short' }).format(
                          new Date(airing.end),
                        )}
                      </span>
                      <span className="text-ink">
                        {airing.name}
                        {airing.episode !== '' &&
                          ` · ${formatEpisode(airing.season, airing.number)} · ${airing.episode}`}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
              <div className="mt-4">
                <Checkbox
                  checked={rebuild}
                  onChange={setRebuild}
                  label={t.ownChannels.rebuild}
                  help={t.ownChannels.rebuildHelp}
                />
              </div>
            </PanelSection>
          )}
          <PanelFooter note={generalError && <FieldError>{generalError}</FieldError>}>
            {saved && (
              <Button variant="danger" icon={TrashIcon} onClick={() => setDeleting(true)}>
                {t.ownChannels.delete}
              </Button>
            )}
            <Button type="submit" variant="primary" loading={save.isPending}>
              {saved ? t.ownChannels.save : t.ownChannels.create}
            </Button>
          </PanelFooter>
        </Panel>
      </form>
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={t.ownChannels.deleteTitle(saved?.name ?? '')}
        confirmLabel={t.ownChannels.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {t.ownChannels.deleteBody}
      </ConfirmDialog>
    </PageLayout>
  )
}

/** Chooses what a channel or a block is built from. */
function SourcePicker({
  source,
  choices,
  onChange,
}: {
  source: ChannelSource
  choices: ChannelChoices
  onChange: (source: ChannelSource) => void
}) {
  const { t } = useI18n()
  const listed = source.kind === 'collection' ? choices.collections : choices.smartLists
  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2">
        <span className="text-control font-medium text-ink">{t.ownChannels.kind}</span>
        <Segmented
          label={t.ownChannels.kind}
          value={source.kind}
          onChange={(kind) => onChange({ kind, items: [] })}
          options={kinds.map((value) => ({ value, label: t.ownChannels.kinds[value] }))}
        />
      </div>
      {(source.kind === 'collection' || source.kind === 'smartlist') &&
        (listed.length === 0 ? (
          <p className="text-small text-ink-3">
            {source.kind === 'collection'
              ? t.ownChannels.noCollections
              : t.ownChannels.noSmartLists}
          </p>
        ) : (
          <Field
            label={
              source.kind === 'collection' ? t.ownChannels.collection : t.ownChannels.smartList
            }
          >
            <Select
              value={source.items[0]?.id ?? ''}
              className="w-full max-w-[420px]"
              options={[
                { value: '', label: t.ownChannels.choose, disabled: true },
                ...listed.map((c) => ({ value: c.id, label: c.name })),
              ]}
              onValue={(id) => {
                const choice = listed.find((c) => c.id === id)
                if (choice)
                  onChange({
                    kind: source.kind,
                    items: [{ id, name: choice.name, kind: source.kind }],
                  })
              }}
            />
          </Field>
        ))}
      {(source.kind === 'series' || source.kind === 'titles') && (
        <TitlePicker source={source} onChange={onChange} />
      )}
    </div>
  )
}

/** Searches movies and series: a series replaces the one chosen, titles add up. */
function TitlePicker({
  source,
  onChange,
}: {
  source: ChannelSource
  onChange: (source: ChannelSource) => void
}) {
  const { t } = useI18n()
  const [query, setQuery] = useState('')
  const [asked, setAsked] = useState('')
  const found = useQuery({
    queryKey: queryKeys.channelTitles(asked),
    queryFn: ({ signal }) => searchChannelTitles(asked, signal),
    enabled: asked !== '',
    staleTime: 5 * 60_000,
  })
  const series = source.kind === 'series'
  const results = (found.data ?? []).filter((title) => !series || title.kind === 'series')
  function submit(event: FormEvent) {
    event.preventDefault()
    setAsked(query.trim())
  }
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-2">
        <Field
          label={series ? t.ownChannels.searchSeries : t.ownChannels.searchTitles}
          className="min-w-[220px] flex-1"
        >
          <TextInput
            value={query}
            onValue={setQuery}
            icon={MagnifyingGlassIcon}
            onKeyDown={(event) => {
              if (event.key === 'Enter') submit(event)
            }}
          />
        </Field>
        <Button onClick={(event) => submit(event)} loading={found.isFetching}>
          {t.ownChannels.search}
        </Button>
      </div>
      {found.isError && <InlineError>{errorMessage(t, found.error)}</InlineError>}
      {asked !== '' && found.isSuccess && results.length === 0 && (
        <p className="text-small text-ink-3">{t.ownChannels.noResults}</p>
      )}
      {results.length > 0 && (
        <ul className="max-h-[240px] space-y-1 overflow-y-auto rounded-field border border-line p-2">
          {results.map((title) => (
            <li key={title.id} className="flex items-center justify-between gap-3">
              <span className="truncate text-small text-ink">
                {title.name}
                {title.year !== null && ` (${title.year})`} ·{' '}
                {title.kind === 'series' ? t.ownChannels.series : t.ownChannels.movie}
              </span>
              <IconButton
                label={t.ownChannels.pick(title.name)}
                icon={PlusIcon}
                onClick={() => {
                  const item = { id: title.id, name: title.name, kind: title.kind }
                  if (series) onChange({ kind: 'series', items: [item] })
                  else if (!source.items.some((i) => i.id === title.id))
                    onChange({ kind: 'titles', items: [...source.items, item] })
                }}
              />
            </li>
          ))}
        </ul>
      )}
      <div>
        <p className="mb-1 text-control font-medium text-ink">{t.ownChannels.picked}</p>
        {source.items.length === 0 ? (
          <p className="text-small text-ink-3">{t.ownChannels.pickedNone}</p>
        ) : (
          <ul className="space-y-1">
            {source.items.map((item) => (
              <li key={item.id} className="flex items-center gap-2 text-small text-ink">
                <span className="truncate">{item.name}</span>
                <IconButton
                  label={t.ownChannels.remove(item.name)}
                  icon={XIcon}
                  onClick={() =>
                    onChange({ ...source, items: source.items.filter((i) => i.id !== item.id) })
                  }
                />
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
