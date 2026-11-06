import { MagnifyingGlassIcon } from '@phosphor-icons/react'
import { useId, useState, type ReactNode } from 'react'
import { useQuery, type QueryKey } from '@tanstack/react-query'
import { excludedKeysLimit, type IptvOptions, type Preview, type PreviewBy } from '@/api'
import { errorMessage } from '@/format'
import { useI18n, type Messages } from '@/i18n'
import {
  Button,
  Checkbox,
  FieldError,
  InlineError,
  Notice,
  Segmented,
  SettingRow,
  Skeleton,
  TextInput,
} from '@/ui'
import { useNumber } from './shared'

type Patch = (patch: Partial<IptvOptions>) => void

/**
 * Everything a source imports and how, in the order an administrator decides it: what to import,
 * then the Live TV options and the movies and series options of what is on, each a group of setting
 * rows. `load` reads a preview. Used by the source page, which saves it as a whole with its dock,
 * and by the add-a-source flow. A tuner has live channels only.
 */
export function SourceOptions({
  value,
  onChange,
  queryKey,
  load,
  saved,
  tuner = false,
}: {
  value: IptvOptions
  onChange: Patch
  queryKey: QueryKey
  load: (by: PreviewBy, signal: AbortSignal) => Promise<Preview>
  /**
   * The options saved, for a source already added: its changed rows keep a dot and "was …", and
   * it warns before Live TV is turned off.
   */
  saved?: IptvOptions
  tuner?: boolean
}) {
  const { t } = useI18n()
  const text = t.lineup.content
  const vodTypes = (['movie', 'series'] as const).filter((type) =>
    type === 'movie' ? value.movies : value.series,
  )
  return (
    <div className="space-y-14 max-md:space-y-11">
      {!tuner && <ContentFields value={value} saved={saved} onChange={onChange} />}
      {saved?.liveTv && !value.liveTv && <Notice tone="danger">{text.liveOffWarning}</Notice>}
      {value.liveTv && (
        <OptionsGroup title={text.liveTitle}>
          <OptionsFields value={value} saved={saved} onChange={onChange} />
          <ExclusionPicker
            mode="live"
            queryKey={queryKey}
            load={load}
            excluded={value.excluded}
            onChange={(excluded) => onChange({ excluded })}
          />
        </OptionsGroup>
      )}
      {vodTypes.length > 0 && (
        <OptionsGroup title={text.vodTitle}>
          <VodFields value={value} saved={saved} onChange={onChange} />
          <ExclusionPicker
            mode="vod"
            types={vodTypes}
            queryKey={queryKey}
            load={load}
            excluded={value.vodExcluded}
            onChange={(vodExcluded) => onChange({ vodExcluded })}
          />
        </OptionsGroup>
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

/**
 * How many options differ from those saved, for the dock's "2 changes": each setting row, and each
 * list of categories left out as one.
 */
export function optionsChanges(value: IptvOptions, saved: IptvOptions): number {
  const sameKeys = (a: readonly string[], b: readonly string[]) => {
    const set = new Set(a)
    return a.length === b.length && b.every((key) => set.has(key))
  }
  return (Object.keys(value) as (keyof IptvOptions)[]).filter((key) => {
    const now = value[key]
    const before = saved[key]
    return Array.isArray(now) && Array.isArray(before) ? !sameKeys(now, before) : now !== before
  }).length
}

/** A group of options under its title: the setting rows, then what they leave out. */
function OptionsGroup({
  title,
  help,
  children,
}: {
  title: string
  help?: string
  children: ReactNode
}) {
  const titleId = useId()
  return (
    <section aria-labelledby={titleId} className="min-w-0">
      <h3 id={titleId} className="text-group text-ink">
        {title}
      </h3>
      {help !== undefined && <p className="mt-1 max-w-[66ch] text-small text-ink-2">{help}</p>}
      <div className="mt-5 space-y-10 max-md:mt-4 max-md:space-y-8">{children}</div>
    </section>
  )
}

/** Setting rows 8 px apart. */
const rows = 'grid grid-cols-[minmax(0,1fr)] gap-2'

/** What a source imports: its live channels, its movies, its series, each a row with a switch. */
function ContentFields({
  value,
  saved,
  onChange,
}: {
  value: IptvOptions
  saved?: IptvOptions
  onChange: Patch
}) {
  const { t } = useI18n()
  const text = t.lineup.content
  const none = !value.liveTv && !value.movies && !value.series
  const kinds: { key: 'liveTv' | 'movies' | 'series'; label: string; help: string }[] = [
    { key: 'liveTv', label: text.liveTv, help: text.liveTvHelp },
    { key: 'movies', label: text.movies, help: text.moviesHelp },
    { key: 'series', label: text.series, help: text.seriesHelp },
  ]
  return (
    <OptionsGroup title={text.title} help={text.help}>
      <div className={rows}>
        {kinds.map((kind) => (
          <SettingRow
            key={kind.key}
            kind="switch"
            label={kind.label}
            help={kind.help}
            checked={value[kind.key]}
            saved={saved?.[kind.key]}
            onChange={(on) => onChange({ [kind.key]: on })}
          />
        ))}
        {none && (
          <div className="px-[1.875rem] pt-2 max-md:px-5">
            <FieldError>{text.none}</FieldError>
          </div>
        )}
      </div>
    </OptionsGroup>
  )
}

/** How movies and series become libraries, and whether they are described by metadata addons. */
function VodFields({
  value,
  saved,
  onChange,
}: {
  value: IptvOptions
  saved?: IptvOptions
  onChange: Patch
}) {
  const { t } = useI18n()
  const text = t.lineup.vod
  return (
    <div className={rows}>
      <SettingRow
        kind="choice"
        label={text.libraries}
        help={text.librariesHelp}
        value={value.vodLibraries}
        saved={saved?.vodLibraries}
        options={[
          { value: 'type', label: text.librariesType },
          { value: 'category', label: text.librariesCategory },
        ]}
        onChange={(vodLibraries) => onChange({ vodLibraries })}
      />
      <SettingRow
        kind="switch"
        label={text.enrichment}
        help={text.enrichmentHelp}
        checked={value.enrichment}
        saved={saved?.enrichment}
        onChange={(enrichment) => onChange({ enrichment })}
      />
    </div>
  )
}

/** How the live list becomes a line-up: the options besides the exclusions. */
function OptionsFields({
  value,
  saved,
  onChange,
}: {
  value: IptvOptions
  saved?: IptvOptions
  onChange: Patch
}) {
  const { t } = useI18n()
  const text = t.lineup.options
  return (
    <div className={rows}>
      <SettingRow
        kind="choice"
        label={text.categoriesMode}
        help={text.categoriesModeHelp}
        value={value.categories}
        saved={saved?.categories}
        options={[
          { value: 'original', label: text.categoriesOriginal },
          { value: 'country', label: text.categoriesCountry },
        ]}
        onChange={(categories) => onChange({ categories })}
      />
      <SettingRow
        kind="choice"
        label={text.channelsMode}
        help={text.channelsModeHelp}
        value={value.channels}
        saved={saved?.channels}
        options={[
          { value: 'original', label: text.channelsOriginal },
          { value: 'merged', label: text.channelsMerged },
        ]}
        onChange={(channels) => onChange({ channels })}
      />
      <SettingRow
        kind="choice"
        label={text.numbering}
        help={text.numberingHelp}
        value={value.numbering}
        saved={saved?.numbering}
        options={[
          { value: 'provider', label: text.numberingProvider },
          { value: 'sequential', label: text.numberingSequential },
        ]}
        onChange={(numbering) => onChange({ numbering })}
      />
      <SettingRow
        kind="switch"
        label={text.newChannels}
        help={text.newChannelsHelp}
        checked={value.newChannels}
        saved={saved?.newChannels}
        onChange={(newChannels) => onChange({ newChannels })}
      />
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
      <legend className="text-h3 text-ink">{text.title}</legend>
      <p className="mt-1 max-w-[66ch] text-small text-ink-2">{text.help}</p>
      <div className="mt-4 flex flex-wrap items-center gap-2.5">
        {views.length > 1 && (
          <Segmented label={text.by} value={by} options={views} onChange={setBy} size="compact" />
        )}
        <TextInput
          type="search"
          size="compact"
          icon={MagnifyingGlassIcon}
          value={search}
          onValue={setSearch}
          placeholder={text.search}
          aria-label={text.search}
          autoComplete="off"
          className="min-w-48 flex-1 sm:ml-auto sm:max-w-80"
        />
      </div>
      {preview.isPending ? (
        <div
          role="status"
          aria-label={t.common.loading}
          className="mt-4 grid gap-4 rounded-row bg-s1 p-5 sm:grid-cols-2"
        >
          {Array.from({ length: 8 }, (_, index) => (
            <Skeleton key={index} className="h-6" />
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
            <p aria-live="polite" className="figures text-small text-ink-2">
              {text.keptInView(number(kept), number(preview.data.total))}
              {summary !== '' && ` · ${summary}`}
            </p>
            <div className="flex flex-wrap gap-1">
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
            <p className="mt-3 rounded-row bg-s1 px-5 py-8 text-center text-small text-ink-2">
              {text.noMatch}
            </p>
          ) : (
            <ul className="mt-3 grid max-h-[35.625rem] gap-x-8 gap-y-0.5 overflow-y-auto overscroll-contain rounded-row bg-s1 px-5 py-3 max-md:px-4 sm:grid-cols-2">
              {listed.map((category) => (
                <li
                  key={category.key}
                  className="min-w-0 [contain-intrinsic-size:auto_3.265625rem] [content-visibility:auto]"
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
                      <span className="figures">
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
