import { useId, useState } from 'react'
import { useQuery, type QueryKey } from '@tanstack/react-query'
import { excludedKeysLimit, type IptvOptions, type Preview } from '@/api'
import { icons } from '@/components/icons'
import { Segmented, SelectField, smallField } from '@/components/lineup/shared'
import { Checkbox, Loading, Notice } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export type By = 'group' | 'country'

/** How the list becomes a line-up: the options besides the exclusions. */
export function OptionsFields({
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
 * The categories of a list, by group or by country, each ticked when its entries are imported. The
 * preview is read once per view and filtered here: a new account's list is downloaded for it.
 */
export function ExclusionPicker({
  queryKey,
  load,
  excluded,
  onChange,
}: {
  queryKey: QueryKey
  load: (by: By, signal: AbortSignal) => Promise<Preview>
  excluded: string[]
  onChange: (excluded: string[]) => void
}) {
  const { language, t } = useI18n()
  const text = t.lineup.exclusions
  const [by, setBy] = useState<By>('group')
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
  const groups = excluded.filter((key) => key.startsWith('g:')).length
  const countries = excluded.filter((key) => key.startsWith('c:')).length

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
        <Segmented
          label={text.by}
          value={by}
          options={[
            { value: 'group', label: text.byGroup },
            { value: 'country', label: text.byCountry },
          ]}
          onChange={setBy}
        />
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
              {(groups > 0 || countries > 0) && ` · ${text.excludedKeys(groups, countries)}`}
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
                      {categoryName(category.key, category.name, text, regionNames)}
                    </span>
                    <span className="shrink-0 text-xs text-muted tabular-nums">
                      {text.channelCount(category.channels)}
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

/** The name a preview category is shown under: entries without group, countries by name. */
function categoryName(
  key: string,
  name: string,
  text: { noGroup: string; otherCountries: string },
  regionNames: Intl.DisplayNames,
): string {
  if (key === 'g:') return text.noGroup
  if (key === 'c:OTHER') return text.otherCountries
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
