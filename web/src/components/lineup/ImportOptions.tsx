import { useId, useState } from 'react'
import { useQuery, type QueryKey } from '@tanstack/react-query'
import { excludedKeysLimit, type IptvOptions, type Preview, type PreviewBy } from '@/api'
import { icons } from '@/components/icons'
import { Segmented, SelectField, smallField } from '@/components/lineup/shared'
import { Checkbox, Loading, Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import type { Messages } from '@/i18n/en'

/**
 * Everything a source imports and how, in the order an administrator decides it: what to import,
 * then the Live TV options and the movies and series options of what is on. `load` reads a preview.
 */
export function SourceOptions({
  value,
  onChange,
  queryKey,
  load,
  saved,
}: {
  value: IptvOptions
  onChange: (patch: Partial<IptvOptions>) => void
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
    <div className="space-y-6">
      <ContentFields value={value} onChange={onChange} />
      {saved?.liveTv && !value.liveTv && <Notice kind="error">{text.liveOffWarning}</Notice>}
      {value.liveTv && (
        <section className="space-y-5 border-t border-line pt-5">
          <h3 className="text-base font-semibold text-white">{text.liveTitle}</h3>
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
        <section className="space-y-5 border-t border-line pt-5">
          <h3 className="text-base font-semibold text-white">{text.vodTitle}</h3>
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

/** What a source imports: its live channels, its movies, its series. */
function ContentFields({
  value,
  onChange,
}: {
  value: IptvOptions
  onChange: (patch: Partial<IptvOptions>) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.content
  const noneId = useId()
  const none = !value.liveTv && !value.movies && !value.series
  return (
    <fieldset className="space-y-3" aria-describedby={none ? noneId : undefined}>
      <legend className="text-sm font-medium text-zinc-200">{text.title}</legend>
      <p className="text-xs text-muted">{text.help}</p>
      <div className="grid gap-3 md:grid-cols-3">
        <Checkbox
          label={text.liveTv}
          help={text.liveTvHelp}
          checked={value.liveTv}
          onChange={(liveTv) => onChange({ liveTv })}
        />
        <Checkbox
          label={text.movies}
          help={text.moviesHelp}
          checked={value.movies}
          onChange={(movies) => onChange({ movies })}
        />
        <Checkbox
          label={text.series}
          help={text.seriesHelp}
          checked={value.series}
          onChange={(series) => onChange({ series })}
        />
      </div>
      {none && (
        <p id={noneId} role="alert" className="text-sm text-rose-300">
          {text.none}
        </p>
      )}
    </fieldset>
  )
}

/** How movies and series become libraries, and whether they are described by metadata addons. */
function VodFields({
  value,
  onChange,
}: {
  value: IptvOptions
  onChange: (patch: Partial<IptvOptions>) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.vod
  return (
    <div className="grid gap-5 md:grid-cols-2">
      <SelectField
        label={text.libraries}
        hint={text.librariesHelp}
        value={value.vodLibraries}
        options={[
          { value: 'type', label: text.librariesType },
          { value: 'category', label: text.librariesCategory },
        ]}
        onChange={(vodLibraries) => onChange({ vodLibraries })}
      />
      <div className="md:pt-6">
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
function OptionsFields({
  value,
  onChange,
}: {
  value: IptvOptions
  onChange: (patch: Partial<IptvOptions>) => void
}) {
  const { t } = useI18n()
  const text = t.lineup.options
  return (
    <div className="grid gap-5 md:grid-cols-2">
      <SelectField
        label={text.categoriesMode}
        hint={text.categoriesModeHelp}
        value={value.categories}
        options={[
          { value: 'original', label: text.categoriesOriginal },
          { value: 'country', label: text.categoriesCountry },
        ]}
        onChange={(categories) => onChange({ categories })}
      />
      <SelectField
        label={text.channelsMode}
        hint={text.channelsModeHelp}
        value={value.channels}
        options={[
          { value: 'original', label: text.channelsOriginal },
          { value: 'merged', label: text.channelsMerged },
        ]}
        onChange={(channels) => onChange({ channels })}
      />
      <SelectField
        label={text.numbering}
        hint={text.numberingHelp}
        value={value.numbering}
        options={[
          { value: 'provider', label: text.numberingProvider },
          { value: 'sequential', label: text.numberingSequential },
        ]}
        onChange={(numbering) => onChange({ numbering })}
      />
      <div className="md:pt-6">
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
  const searchId = useId()
  const preview = useQuery({
    queryKey: [...queryKey, by],
    queryFn: ({ signal }) => load(by, signal),
    // A preview of a new account downloads its list: never again for the same view.
    staleTime: Infinity,
    gcTime: 10 * 60_000,
  })
  const number = (n: number) => n.toLocaleString(language)
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
    <fieldset className="space-y-3">
      <legend className="text-sm font-medium text-zinc-200">{text.title}</legend>
      <p className="text-xs text-muted">{text.help}</p>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        {views.length > 1 && (
          <Segmented label={text.by} value={by} options={views} onChange={setBy} />
        )}
        <div className="relative flex-1">
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
      </div>
      {preview.isPending ? (
        <Loading />
      ) : preview.isError ? (
        <Notice kind="error">{errorMessage(t, preview.error)}</Notice>
      ) : (
        <>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p aria-live="polite" className="text-xs text-muted tabular-nums">
              {text.keptInView(number(kept), number(preview.data.total))}
              {summary !== '' && ` · ${summary}`}
            </p>
            <div className="flex flex-wrap gap-2">
              <button
                type="button"
                className={chip}
                onClick={() =>
                  apply(
                    listed.map((c) => c.key),
                    false,
                  )
                }
              >
                {needle === '' ? text.includeAll : text.includeListed}
              </button>
              <button
                type="button"
                className={chip}
                onClick={() =>
                  apply(
                    listed.map((c) => c.key),
                    true,
                  )
                }
              >
                {needle === '' ? text.excludeAll : text.excludeListed}
              </button>
            </div>
          </div>
          {excluded.length > excludedKeysLimit && <Notice kind="error">{text.tooMany}</Notice>}
          {listed.length === 0 ? (
            <p className="rounded-lg border border-dashed border-line px-3 py-6 text-center text-sm text-muted">
              {text.noMatch}
            </p>
          ) : (
            <ul className="max-h-96 overflow-y-auto rounded-lg border border-line">
              {listed.map((category) => (
                <li
                  key={category.key}
                  className="border-b border-line px-3 last:border-b-0 [contain-intrinsic-size:auto_2.75rem] [content-visibility:auto]"
                >
                  <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm">
                    <input
                      type="checkbox"
                      checked={!set.has(category.key)}
                      onChange={(event) => apply([category.key], !event.target.checked)}
                      className="size-4 shrink-0 accent-fin-3"
                    />
                    <span className="min-w-0 flex-1 truncate text-zinc-100">
                      {categoryName(category.key, category.name, t, regionNames)}
                    </span>
                    <span className="shrink-0 text-xs text-muted tabular-nums">
                      {mode === 'live'
                        ? live.channelCount(category.channels)
                        : vod.titleCount(category.channels)}
                    </span>
                  </label>
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

const chip =
  'inline-flex min-h-8 items-center rounded-lg border border-line bg-ink px-3 text-xs font-medium text-white transition-colors hover:border-fin-4'
