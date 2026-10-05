import {
  FilmSlateIcon,
  MagnifyingGlassIcon,
  MonitorPlayIcon,
  TelevisionIcon,
  type Icon,
} from '@phosphor-icons/react'
import { useId, useState } from 'react'
import { useQuery, type QueryKey } from '@tanstack/react-query'
import { excludedKeysLimit, type IptvOptions, type Preview, type PreviewBy } from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  Checkbox,
  Field,
  FieldError,
  IconTile,
  InlineError,
  Notice,
  Segmented,
  Select,
  Skeleton,
  Switch,
  TextInput,
} from '@/ui'
import { useNumber } from './shared'

type Patch = (patch: Partial<IptvOptions>) => void

/**
 * Everything a source imports and how, in the order an administrator decides it: what to import,
 * then the Live TV options and the movies and series options of what is on. `load` reads a preview.
 * Used by the source page and by the add-a-source flow.
 */
export function SourceOptions({
  value,
  onChange,
  queryKey,
  load,
  saved,
}: {
  value: IptvOptions
  onChange: Patch
  queryKey: QueryKey
  load: (by: PreviewBy, signal: AbortSignal) => Promise<Preview>
  /** The options saved, for a source already added: warns before Live TV is turned off. */
  saved?: IptvOptions
}) {
  const { t } = useI18n()
  const text = t.lineup.content
  const vodTypes = (['movie', 'series'] as const).filter((type) =>
    type === 'movie' ? value.movies : value.series,
  )
  return (
    <div className="space-y-10">
      <ContentFields value={value} onChange={onChange} />
      {saved?.liveTv && !value.liveTv && <Notice tone="danger">{text.liveOffWarning}</Notice>}
      {value.liveTv && (
        <section className="space-y-7 border-t border-line pt-8">
          <h3 className="text-h3 text-ink">{text.liveTitle}</h3>
          <ExclusionPicker
            mode="live"
            queryKey={queryKey}
            load={load}
            excluded={value.excluded}
            onChange={(excluded) => onChange({ excluded })}
          />
          <OptionsFields value={value} onChange={onChange} />
        </section>
      )}
      {vodTypes.length > 0 && (
        <section className="space-y-7 border-t border-line pt-8">
          <h3 className="text-h3 text-ink">{text.vodTitle}</h3>
          <ExclusionPicker
            mode="vod"
            types={vodTypes}
            queryKey={queryKey}
            load={load}
            excluded={value.vodExcluded}
            onChange={(vodExcluded) => onChange({ vodExcluded })}
          />
          <VodFields value={value} onChange={onChange} />
        </section>
      )}
    </div>
  )
}

/** Whether the options can be saved: something is imported, and within the server's limits. */
export function optionsValid(value: IptvOptions): boolean {
  return (
    (value.liveTv || value.movies || value.series) &&
    value.excluded.length <= excludedKeysLimit &&
    value.vodExcluded.length <= excludedKeysLimit
  )
}

/** What a source imports: its live channels, its movies, its series, each a row with a switch. */
function ContentFields({ value, onChange }: { value: IptvOptions; onChange: Patch }) {
  const { t } = useI18n()
  const text = t.lineup.content
  const none = !value.liveTv && !value.movies && !value.series
  const rows: { key: 'liveTv' | 'movies' | 'series'; icon: Icon; label: string; help: string }[] = [
    { key: 'liveTv', icon: TelevisionIcon, label: text.liveTv, help: text.liveTvHelp },
    { key: 'movies', icon: FilmSlateIcon, label: text.movies, help: text.moviesHelp },
    { key: 'series', icon: MonitorPlayIcon, label: text.series, help: text.seriesHelp },
  ]
  return (
    <fieldset>
      <legend className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
        {text.title}
      </legend>
      <p className="mt-1 text-small text-ink-3">{text.help}</p>
      <ul className="mt-4 overflow-hidden rounded-row border border-line-2 bg-bg">
        {rows.map((row) => (
          <ContentRow
            key={row.key}
            icon={row.icon}
            label={row.label}
            help={row.help}
            checked={value[row.key]}
            onChange={(on) => onChange({ [row.key]: on })}
          />
        ))}
      </ul>
      {none && (
        <div className="mt-3">
          <FieldError>{text.none}</FieldError>
        </div>
      )}
    </fieldset>
  )
}

function ContentRow({
  icon,
  label,
  help,
  checked,
  onChange,
}: {
  icon: Icon
  label: string
  help: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  const ids = { label: useId(), help: useId() }
  return (
    <li className="flex items-center gap-3.5 px-4 py-3.5 not-first:border-t not-first:border-line">
      <IconTile icon={icon} />
      <div className="min-w-0 flex-1">
        <p id={ids.label} className={checked ? 'font-medium text-ink' : 'font-medium text-ink-2'}>
          {label}
        </p>
        <p id={ids.help} className="text-small text-ink-3">
          {help}
        </p>
      </div>
      <Switch
        checked={checked}
        onChange={onChange}
        labelledBy={ids.label}
        describedById={ids.help}
      />
    </li>
  )
}

/** How movies and series become libraries, and whether they are described by metadata addons. */
function VodFields({ value, onChange }: { value: IptvOptions; onChange: Patch }) {
  const { t } = useI18n()
  const text = t.lineup.vod
  return (
    <div className="grid gap-x-8 gap-y-6 md:grid-cols-2">
      <Field label={text.libraries} help={text.librariesHelp}>
        <Select
          value={value.vodLibraries}
          options={[
            { value: 'type', label: text.librariesType },
            { value: 'category', label: text.librariesCategory },
          ]}
          onValue={(vodLibraries) => onChange({ vodLibraries })}
        />
      </Field>
      <div className="md:pt-7">
        <Checkbox
          label={text.enrichment}
          help={text.enrichmentHelp}
          checked={value.enrichment}
          onChange={(enrichment) => onChange({ enrichment })}
        />
      </div>
    </div>
  )
}

/** How the live list becomes a line-up: the options besides the exclusions. */
function OptionsFields({ value, onChange }: { value: IptvOptions; onChange: Patch }) {
  const { t } = useI18n()
  const text = t.lineup.options
  return (
    <div className="grid gap-x-8 gap-y-6 md:grid-cols-2">
      <Field label={text.categoriesMode} help={text.categoriesModeHelp}>
        <Select
          value={value.categories}
          options={[
            { value: 'original', label: text.categoriesOriginal },
            { value: 'country', label: text.categoriesCountry },
          ]}
          onValue={(categories) => onChange({ categories })}
        />
      </Field>
      <Field label={text.channelsMode} help={text.channelsModeHelp}>
        <Select
          value={value.channels}
          options={[
            { value: 'original', label: text.channelsOriginal },
            { value: 'merged', label: text.channelsMerged },
          ]}
          onValue={(channels) => onChange({ channels })}
        />
      </Field>
      <Field label={text.numbering} help={text.numberingHelp}>
        <Select
          value={value.numbering}
          options={[
            { value: 'provider', label: text.numberingProvider },
            { value: 'sequential', label: text.numberingSequential },
          ]}
          onValue={(numbering) => onChange({ numbering })}
        />
      </Field>
      <div className="md:pt-7">
        <Checkbox
          label={text.newChannels}
          help={text.newChannelsHelp}
          checked={value.newChannels}
          onChange={(newChannels) => onChange({ newChannels })}
        />
      </div>
    </div>
  )
}

/**
 * The categories of a list, each ticked when its entries are imported: live entries by group or by
 * country, or the titles of each VOD type. The preview is read once per view and filtered here: a new
 * account's list is downloaded for it.
 */
function ExclusionPicker({
  mode,
  types = [],
  queryKey,
  load,
  excluded,
  onChange,
}: {
  mode: 'live' | 'vod'
  /** The VOD types imported, whose categories are shown. */
  types?: readonly ('movie' | 'series')[]
  queryKey: QueryKey
  load: (by: PreviewBy, signal: AbortSignal) => Promise<Preview>
  excluded: string[]
  onChange: (excluded: string[]) => void
}) {
  const { language, t } = useI18n()
  const number = useNumber()
  const live = t.lineup.exclusions
  const vod = t.lineup.vodExclusions
  const text = mode === 'live' ? live : vod
  const views: { value: PreviewBy; label: string }[] =
    mode === 'live'
      ? [
          { value: 'group', label: live.byGroup },
          { value: 'country', label: live.byCountry },
        ]
      : types.map((type) => ({ value: type, label: type === 'movie' ? vod.byMovie : vod.bySeries }))
  const [chosen, setBy] = useState<PreviewBy>(views[0].value)
  // A type turned off leaves its view: the first one left is shown.
  const by = views.some((view) => view.value === chosen) ? chosen : views[0].value
  const [search, setSearch] = useState('')
  const preview = useQuery({
    queryKey: [...queryKey, by],
    queryFn: ({ signal }) => load(by, signal),
    // A preview of a new account downloads its list: never again for the same view.
    staleTime: Infinity,
    gcTime: 10 * 60_000,
  })
  const regionNames = new Intl.DisplayNames([language], { type: 'region' })
  const set = new Set(excluded)
  const needle = search.trim().toLocaleLowerCase(language)
  const categories = preview.data?.categories ?? []
  const listed =
    needle === ''
      ? categories
      : categories.filter(
          (c) =>
            c.name.toLocaleLowerCase(language).includes(needle) ||
            c.key.toLocaleLowerCase(language).includes(needle),
        )
  const kept = categories.reduce((sum, c) => sum + (set.has(c.key) ? 0 : c.channels), 0)
  // Keys left out, of each kind: groups and countries, or movie and series categories.
  const [first, second] = (mode === 'live' ? ['g:', 'c:'] : ['movie:', 'series:']).map(
    (prefix) => excluded.filter((key) => key.startsWith(prefix)).length,
  )
  const summary =
    first + second === 0
      ? ''
      : mode === 'live'
        ? live.excludedKeys(first, second)
        : vod.excludedKeys(first, second)

  function apply(keys: string[], exclude: boolean) {
    const next = new Set(excluded)
    for (const key of keys) {
      if (exclude) next.add(key)
      else next.delete(key)
    }
    onChange([...next])
  }

  return (
    <fieldset className="min-w-0">
      <legend className="text-[15px] font-semibold tracking-[-0.01em] text-ink">
        {text.title}
      </legend>
      <p className="mt-1 text-small text-ink-3">{text.help}</p>
      <div className="mt-4 flex flex-wrap items-center gap-3">
        {views.length > 1 && (
          <Segmented
            label={text.by}
            value={by}
            options={views}
            onChange={setBy}
            size="sm"
            className="scrollbar-none max-w-full overflow-x-auto [&>button]:shrink-0 [&>button]:whitespace-nowrap"
          />
        )}
        <TextInput
          type="search"
          size="sm"
          icon={MagnifyingGlassIcon}
          value={search}
          onValue={setSearch}
          placeholder={text.search}
          aria-label={text.search}
          autoComplete="off"
          className="min-w-48 flex-1 sm:max-w-72 sm:ml-auto"
        />
      </div>
      {preview.isPending ? (
        <div role="status" aria-label={t.common.loading} className="mt-4 grid gap-3 sm:grid-cols-2">
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="h-5" />
          ))}
        </div>
      ) : preview.isError ? (
        <InlineError
          className="mt-4"
          onRetry={() => void preview.refetch()}
          retrying={preview.isFetching}
        >
          {errorMessage(t, preview.error)}
        </InlineError>
      ) : (
        <>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
            <p aria-live="polite" className="text-small text-ink-2 tabular-nums">
              {text.keptInView(number(kept), number(preview.data.total))}
              {summary !== '' && ` · ${summary}`}
            </p>
            <div className="flex flex-wrap gap-1.5">
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  apply(
                    listed.map((c) => c.key),
                    false,
                  )
                }
              >
                {needle === '' ? text.includeAll : text.includeListed}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  apply(
                    listed.map((c) => c.key),
                    true,
                  )
                }
              >
                {needle === '' ? text.excludeAll : text.excludeListed}
              </Button>
            </div>
          </div>
          {excluded.length > excludedKeysLimit && (
            <div className="mt-3">
              <FieldError>{text.tooMany}</FieldError>
            </div>
          )}
          {listed.length === 0 ? (
            <p className="mt-3 rounded-row border border-dashed border-line-2 px-3 py-6 text-center text-small text-ink-3">
              {text.noMatch}
            </p>
          ) : (
            <ul className="mt-3 grid max-h-[26rem] gap-x-8 overflow-y-auto rounded-row border border-line-2 bg-bg px-4 py-1.5 sm:grid-cols-2">
              {listed.map((category) => (
                <li
                  key={category.key}
                  className="py-2 [contain-intrinsic-size:auto_2.25rem] [content-visibility:auto]"
                >
                  <Checkbox
                    checked={!set.has(category.key)}
                    onChange={(checked) => apply([category.key], !checked)}
                    label={
                      <span className="block truncate">
                        {categoryName(category.key, category.name, t, regionNames)}
                      </span>
                    }
                    aside={
                      <span className="tabular-nums">
                        {mode === 'live'
                          ? live.channelCount(category.channels)
                          : vod.titleCount(category.channels)}
                      </span>
                    }
                  />
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </fieldset>
  )
}

/** The name a preview category is shown under: entries without category, countries by name. */
function categoryName(
  key: string,
  name: string,
  t: Messages,
  regionNames: Intl.DisplayNames,
): string {
  if (key === 'g:') return t.lineup.exclusions.noGroup
  if (key === 'c:OTHER') return t.lineup.exclusions.otherCountries
  if (key === 'movie:' || key === 'series:') return t.lineup.vodExclusions.noCategory
  if (key.startsWith('c:')) {
    const code = key.slice(2)
    try {
      const country = regionNames.of(code)
      if (country && country !== code) return `${country} (${code})`
    } catch {
      // Not a region code the browser knows: shown as sent.
    }
  }
  return name
}
