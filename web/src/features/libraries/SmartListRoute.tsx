import { FunnelSimpleIcon, PlusIcon, TrashIcon, XIcon } from '@phosphor-icons/react'
import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import {
  ApiError,
  createSmartList,
  deleteSmartList,
  fetchSmartChoices,
  fetchSmartList,
  MAX_LIBRARY_ITEMS,
  previewSmartList,
  queryClient,
  queryKeys,
  saveSmartList,
  type Scope,
  type SmartCatalogChoice,
  type SmartChoices,
  type SmartField,
  type SmartList,
  type SmartListDraft,
  type SmartRule,
  type SmartSort,
} from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { errorMessage, stremioLabel } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  Checkbox,
  ConfirmDialog,
  cx,
  EmptyState,
  Field,
  FieldError,
  IconButton,
  InlineError,
  Notice,
  NumberInput,
  Panel,
  PanelFooter,
  PanelSection,
  Segmented,
  Select,
  SkeletonRows,
  SuggestInput,
  Table,
  TextInput,
  useToast,
} from '@/ui'
import { smartListPath } from './SmartLists'

const fields: Exclude<SmartField, 'group'>[] = [
  'genre',
  'year',
  'released',
  'rating',
  'votes',
  'language',
  'runtime',
  'contentRating',
  'person',
  'studio',
  'type',
  'rank',
  'state',
]
const sorts: SmartSort[] = [
  'catalog',
  'premiere',
  'latestEpisode',
  'seasonPremiere',
  'rating',
  'name',
  'random',
]
const states = ['unplayed', 'played', 'started', 'favorite']

/** The fields a title's description may have to tell, when its catalog does not. */
const describedFields: SmartField[] = ['language', 'runtime', 'contentRating', 'person', 'released']

/** Groups go this deep: the list's own, and groups within it. */
const maxDepth = 2

const emptyDraft: SmartListDraft = {
  name: '',
  shownAs: 'library',
  catalogs: [],
  rules: { field: 'group', match: 'all', rules: [] },
  exclude: noExclusions(),
  sort: 'catalog',
  maxItems: null,
}

/** The exclusions of a list that leaves no title out. */
function noExclusions(): SmartRule {
  return { field: 'group', match: 'any', rules: [] }
}

/** A new rule of a field, with the value it starts with. */
function newRule(field: Exclude<SmartField, 'group'>): SmartRule {
  switch (field) {
    case 'type':
      return { field, value: 'movie' }
    case 'state':
      return { field, value: 'unplayed' }
    case 'language':
      return { field, value: 'fr' }
    case 'rank':
      return { field, max: 100 }
    case 'runtime':
      return { field, min: 90 }
    case 'votes':
      return { field, min: 1000 }
    case 'contentRating':
      return { field, value: 'PG-13' }
    case 'released':
      return { field, min: -30, max: 0 }
    case 'genre':
    case 'person':
    case 'studio':
      return { field, value: '' }
    default:
      return { field }
  }
}

/** Whether a rule says all it needs, as the server checks it, groups within included. */
function complete(rule: SmartRule, depth = 1): boolean {
  switch (rule.field) {
    case 'group':
      return (
        (depth === 1 || (rule.rules ?? []).length > 0) &&
        (rule.rules ?? []).every((r) => complete(r, depth + 1))
      )
    case 'genre':
    case 'person':
    case 'studio':
      return (rule.value ?? '').trim() !== ''
    case 'released':
      return rule.min !== undefined && rule.max !== undefined && rule.min <= rule.max
    case 'year':
    case 'rating':
    case 'runtime':
    case 'votes':
      return (
        (rule.min !== undefined || rule.max !== undefined) &&
        (rule.min === undefined || rule.max === undefined || rule.min <= rule.max)
      )
    case 'rank':
      return rule.max !== undefined && rule.max >= 1
    default:
      return Boolean(rule.value)
  }
}

function usesField(rule: SmartRule, field: SmartField): boolean {
  return rule.field === field || (rule.rules ?? []).some((r) => usesField(r, field))
}

/** Whether a list's rules or exclusions look at one of fields. */
function listUses(draft: SmartListDraft, ...fields: SmartField[]): boolean {
  return fields.some((field) => usesField(draft.rules, field) || usesField(draft.exclude, field))
}

function catalogKey(c: { addonId: string; type: string; id: string }): string {
  return JSON.stringify([c.addonId, c.type, c.id])
}

/** `/libraries/smart/:id` and `/me/sources/smart/:id`: makes or changes a smart list of a scope. */
export default function SmartListRoute({ scope }: { scope: Scope }) {
  const { t } = useI18n()
  const { id = 'new' } = useParams()
  const isNew = id === 'new'
  const back = {
    to: scope === 'shared' ? '/libraries' : '/me/sources',
    label: scope === 'shared' ? t.smartLists.back : t.smartLists.backMine,
  }
  const list = useQuery({
    queryKey: queryKeys.smartList(scope, id),
    queryFn: ({ signal }) => fetchSmartList(scope, id, signal),
    enabled: !isNew,
  })
  const choices = useQuery({
    queryKey: queryKeys.smartChoices(scope),
    queryFn: ({ signal }) => fetchSmartChoices(scope, signal),
  })

  if (!isNew && list.isError) {
    const missing = list.error instanceof ApiError && list.error.status === 404
    return (
      <PageLayout title={t.smartLists.title} back={back}>
        {missing ? (
          <EmptyState
            icon={FunnelSimpleIcon}
            title={t.smartLists.notFound}
            action={<ButtonLink to={back.to}>{back.label}</ButtonLink>}
          />
        ) : (
          <InlineError onRetry={() => void list.refetch()} retrying={list.isFetching}>
            {errorMessage(t, list.error)}
          </InlineError>
        )}
      </PageLayout>
    )
  }
  if ((!isNew && list.isPending) || choices.isPending || choices.isError) {
    return (
      <PageLayout title={t.smartLists.newTitle} back={back}>
        {choices.isError ? (
          <InlineError onRetry={() => void choices.refetch()} retrying={choices.isFetching}>
            {errorMessage(t, choices.error)}
          </InlineError>
        ) : (
          <SkeletonRows rows={4} label={t.smartLists.loading} />
        )}
      </PageLayout>
    )
  }
  return (
    <Editor
      key={id}
      scope={scope}
      saved={isNew ? null : (list.data ?? null)}
      choices={choices.data}
      back={back}
    />
  )
}

function Editor({
  scope,
  saved,
  choices,
  back,
}: {
  scope: Scope
  saved: SmartList | null
  choices: SmartChoices
  back: { to: string; label: string }
}) {
  const { t, language } = useI18n()
  const toast = useToast()
  const navigate = useNavigate()
  const [draft, setDraft] = useState<SmartListDraft>(() =>
    saved
      ? {
          name: saved.name,
          shownAs: saved.shownAs,
          catalogs: saved.catalogs,
          rules: saved.rules,
          exclude: saved.exclude ?? noExclusions(),
          sort: saved.sort,
          maxItems: saved.maxItems,
        }
      : emptyDraft,
  )
  const [deleting, setDeleting] = useState(false)
  const change = (patch: Partial<SmartListDraft>) => setDraft((d) => ({ ...d, ...patch }))

  const save = useMutation({
    mutationFn: () =>
      saved ? saveSmartList(scope, saved.id, draft) : createSmartList(scope, draft),
    onSuccess: (list) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.smartLists(scope) })
      queryClient.setQueryData(queryKeys.smartList(scope, list.id), list)
      toast(saved ? t.smartLists.saved : t.smartLists.created)
      if (!saved) void navigate(smartListPath(scope, list.id), { replace: true })
    },
  })
  const remove = useMutation({
    mutationFn: () => deleteSmartList(scope, saved?.id ?? ''),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.smartLists(scope) })
      toast(t.smartLists.deleted)
      void navigate(back.to)
    },
  })
  const code = save.error instanceof ApiError ? save.error.code : null
  const errorFor = (codes: string[]) =>
    save.error && code && codes.includes(code) ? errorMessage(t, save.error) : undefined
  const generalError =
    save.isError &&
    !errorFor([
      'invalid_smart_list_name',
      'invalid_smart_list_catalogs',
      'invalid_smart_list_rules',
      'invalid_smart_list_max_items',
    ])
      ? errorMessage(t, save.error)
      : undefined

  const genres = useMemo(() => {
    const chosen = new Set(draft.catalogs.map(catalogKey))
    const all = new Set<string>()
    for (const c of choices.catalogs) {
      if (chosen.has(catalogKey({ addonId: c.addonId, type: c.catalogType, id: c.catalogId })))
        c.genres.forEach((genre) => all.add(genre))
    }
    return [...all].sort((a, b) => a.localeCompare(b, language))
  }, [draft.catalogs, choices.catalogs, language])
  const languages = useMemo(() => {
    const names = new Intl.DisplayNames([language], { type: 'language' })
    return choices.languages
      .map((code) => ({ value: code, label: names.of(code) ?? code }))
      .sort((a, b) => a.label.localeCompare(b.label, language))
  }, [choices.languages, language])
  const maxValid =
    draft.maxItems === null ||
    (Number.isInteger(draft.maxItems) && draft.maxItems >= 1 && draft.maxItems <= MAX_LIBRARY_ITEMS)
  const max = MAX_LIBRARY_ITEMS.toLocaleString(language)

  return (
    <PageLayout
      title={saved ? saved.name : t.smartLists.newTitle}
      lede={t.smartLists.lede}
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
            <div className="grid gap-6 md:grid-cols-2">
              <Field
                label={t.smartLists.name}
                help={t.smartLists.nameHelp}
                error={errorFor(['invalid_smart_list_name'])}
              >
                <TextInput value={draft.name} maxLength={64} onValue={(name) => change({ name })} />
              </Field>
              <div className="flex flex-col gap-2">
                <span className="text-control font-medium text-ink">{t.smartLists.shownAs}</span>
                <Segmented
                  label={t.smartLists.shownAs}
                  value={draft.shownAs}
                  onChange={(shownAs) => change({ shownAs })}
                  options={(['library', 'collection'] as const).map((value) => ({
                    value,
                    label: t.smartLists.shownAsChoices[value],
                  }))}
                />
                <p className="max-w-[60ch] text-small text-ink-3">
                  {t.smartLists.shownAsHelp[draft.shownAs]}
                </p>
              </div>
            </div>
          </PanelSection>
          <PanelSection title={t.smartLists.catalogs} description={t.smartLists.catalogsHelp}>
            <CatalogPicker
              choices={choices.catalogs}
              chosen={draft.catalogs}
              mine={scope === 'me'}
              onChange={(catalogs) => change({ catalogs })}
            />
            {errorFor(['invalid_smart_list_catalogs']) && (
              <div className="mt-3">
                <FieldError>{errorFor(['invalid_smart_list_catalogs'])}</FieldError>
              </div>
            )}
          </PanelSection>
          <PanelSection title={t.smartLists.rules} description={t.smartLists.rulesHelp}>
            <GroupEditor
              group={draft.rules}
              depth={1}
              genres={genres}
              languages={languages}
              contentRatings={choices.contentRatings}
              onChange={(rules) => change({ rules })}
            />
          </PanelSection>
          <PanelSection title={t.smartLists.exclude} description={t.smartLists.excludeHelp}>
            <GroupEditor
              group={draft.exclude}
              depth={1}
              label={t.smartLists.excludeMatch}
              genres={genres}
              languages={languages}
              contentRatings={choices.contentRatings}
              onChange={(exclude) => change({ exclude })}
            />
            {listUses(draft, ...describedFields) && (
              <Notice className="mt-4">{t.smartLists.describedHelp}</Notice>
            )}
            {listUses(draft, 'votes') && <Notice className="mt-4">{t.smartLists.votesHelp}</Notice>}
            {listUses(draft, 'studio') && (
              <Notice className="mt-4">{t.smartLists.studioHelp}</Notice>
            )}
            {listUses(draft, 'state') && <Notice className="mt-4">{t.smartLists.stateHelp}</Notice>}
            {errorFor(['invalid_smart_list_rules']) && (
              <div className="mt-3">
                <FieldError>{errorFor(['invalid_smart_list_rules'])}</FieldError>
              </div>
            )}
          </PanelSection>
          <PanelSection>
            <div className="grid gap-6 md:grid-cols-2">
              <Field
                label={t.smartLists.sort}
                help={
                  draft.sort === 'latestEpisode' || draft.sort === 'seasonPremiere'
                    ? t.smartLists.episodeSortHelp
                    : undefined
                }
              >
                <Select
                  value={draft.sort}
                  onValue={(sort) => change({ sort })}
                  options={sorts.map((value) => ({ value, label: t.smartLists.sorts[value] }))}
                />
              </Field>
              <Field
                label={t.smartLists.maxItems}
                help={t.smartLists.maxHelp(max)}
                error={
                  !maxValid
                    ? t.libraries.narrow.maxRange(max)
                    : errorFor(['invalid_smart_list_max_items'])
                }
              >
                <NumberInput
                  value={draft.maxItems}
                  min={1}
                  max={MAX_LIBRARY_ITEMS}
                  step={1}
                  placeholder={t.smartLists.noMax}
                  className="w-[160px]"
                  onValue={(maxItems) => change({ maxItems })}
                />
              </Field>
            </div>
          </PanelSection>
          <PanelFooter note={generalError && <FieldError>{generalError}</FieldError>}>
            {saved && (
              <Button variant="danger" icon={TrashIcon} onClick={() => setDeleting(true)}>
                {t.smartLists.delete}
              </Button>
            )}
            <Button type="submit" variant="primary" loading={save.isPending} disabled={!maxValid}>
              {saved ? t.smartLists.save : t.smartLists.create}
            </Button>
          </PanelFooter>
        </Panel>
      </form>
      <Preview scope={scope} draft={draft} />
      <ConfirmDialog
        open={deleting}
        onClose={() => setDeleting(false)}
        onConfirm={() => remove.mutate()}
        title={t.smartLists.deleteTitle(saved?.name ?? '')}
        confirmLabel={t.smartLists.delete}
        tone="danger"
        busy={remove.isPending}
        error={remove.isError ? errorMessage(t, remove.error) : undefined}
      >
        {t.smartLists.deleteBody}
      </ConfirmDialog>
    </PageLayout>
  )
}

/** Ticks the catalogs a list draws from, by addon, with a filter once there are many. */
function CatalogPicker({
  choices,
  chosen,
  mine,
  onChange,
}: {
  choices: SmartCatalogChoice[]
  chosen: SmartListDraft['catalogs']
  mine: boolean
  onChange: (catalogs: SmartListDraft['catalogs']) => void
}) {
  const { t } = useI18n()
  const [filter, setFilter] = useState('')
  const keys = new Set(chosen.map(catalogKey))
  const offered = new Set(
    choices.map((c) => catalogKey({ addonId: c.addonId, type: c.catalogType, id: c.catalogId })),
  )
  const missing = chosen.filter((c) => !offered.has(catalogKey(c)))
  const needle = filter.trim().toLocaleLowerCase()
  const shown = choices.filter(
    (c) =>
      needle === '' ||
      c.catalogName.toLocaleLowerCase().includes(needle) ||
      c.addonName.toLocaleLowerCase().includes(needle),
  )
  if (choices.length === 0 && missing.length === 0)
    return <Notice>{t.smartLists.noCatalogs}</Notice>
  const toggle = (ref: SmartListDraft['catalogs'][number], on: boolean) =>
    onChange(on ? [...chosen, ref] : chosen.filter((c) => catalogKey(c) !== catalogKey(ref)))
  return (
    <div className="flex flex-col gap-3">
      {choices.length > 8 && (
        <Field label={t.smartLists.catalogFilter} hideLabel>
          <TextInput
            value={filter}
            placeholder={t.smartLists.catalogFilterPlaceholder}
            onValue={setFilter}
            className="max-w-[360px]"
          />
        </Field>
      )}
      <div className="grid max-h-[360px] gap-x-6 gap-y-3 overflow-y-auto sm:grid-cols-2">
        {missing.map((c) => (
          <Checkbox
            key={catalogKey(c)}
            checked
            onChange={() => toggle(c, false)}
            label={c.id}
            help={t.libraries.missing}
          />
        ))}
        {shown.map((c) => {
          const ref = { addonId: c.addonId, type: c.catalogType, id: c.catalogId }
          return (
            <Checkbox
              key={catalogKey(ref)}
              checked={keys.has(catalogKey(ref))}
              onChange={(on) => toggle(ref, on)}
              label={c.catalogName || c.catalogId}
              help={`${c.addonName} · ${stremioLabel(t.stremioTypes, c.catalogType)}`}
              aside={mine && c.shared ? <Badge>{t.smartLists.serverBadge}</Badge> : undefined}
            />
          )
        })}
      </div>
      {shown.length === 0 && <p className="text-small text-ink-3">{t.smartLists.noCatalogMatch}</p>}
    </div>
  )
}

type Option = { value: string; label: string }

/** A group of rules: whether titles match all or any, its rules, and groups within it. */
function GroupEditor({
  group,
  depth,
  label,
  genres,
  languages,
  contentRatings,
  onChange,
  onRemove,
}: {
  group: SmartRule
  depth: number
  /** What the list's own group is called; groups within it are this group. */
  label?: string
  genres: string[]
  languages: Option[]
  contentRatings: string[]
  onChange: (group: SmartRule) => void
  onRemove?: () => void
}) {
  const { t } = useI18n()
  const title = depth === 1 ? (label ?? t.smartLists.match) : t.smartLists.groupMatch
  const rules = group.rules ?? []
  const set = (index: number, rule: SmartRule) =>
    onChange({ ...group, rules: rules.map((r, i) => (i === index ? rule : r)) })
  const drop = (index: number) => onChange({ ...group, rules: rules.filter((_, i) => i !== index) })
  return (
    <div
      className={cx(
        'flex flex-col gap-3',
        depth > 1 && 'rounded-panel border border-line-2 bg-s2/40 p-4',
      )}
    >
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-control font-medium text-ink">{title}</span>
        <Segmented
          label={title}
          size="sm"
          value={group.match ?? 'all'}
          onChange={(match) => onChange({ ...group, match })}
          options={(['all', 'any'] as const).map((value) => ({
            value,
            label: t.smartLists.matchChoices[value],
          }))}
        />
        {depth > 1 && (
          <Select
            aria-label={t.smartLists.negation}
            className="w-[140px]"
            value={group.not ? 'not' : 'is'}
            onValue={(value) => onChange({ ...group, not: value === 'not' })}
            options={[
              { value: 'is', label: t.smartLists.is },
              { value: 'not', label: t.smartLists.isNot },
            ]}
          />
        )}
        {onRemove && (
          <IconButton
            label={t.smartLists.removeGroup}
            icon={XIcon}
            onClick={onRemove}
            className="ml-auto"
          />
        )}
      </div>
      {rules.map((rule, index) =>
        rule.field === 'group' ? (
          <GroupEditor
            key={index}
            group={rule}
            depth={depth + 1}
            genres={genres}
            languages={languages}
            contentRatings={contentRatings}
            onChange={(next) => set(index, next)}
            onRemove={() => drop(index)}
          />
        ) : (
          <RuleEditor
            key={index}
            rule={rule}
            genres={genres}
            languages={languages}
            contentRatings={contentRatings}
            onChange={(next) => set(index, next)}
            onRemove={() => drop(index)}
          />
        ),
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          icon={PlusIcon}
          onClick={() => onChange({ ...group, rules: [...rules, newRule('genre')] })}
        >
          {t.smartLists.addRule}
        </Button>
        {depth < maxDepth && (
          <Button
            size="sm"
            variant="ghost"
            icon={PlusIcon}
            onClick={() =>
              onChange({
                ...group,
                rules: [
                  ...rules,
                  { field: 'group', match: 'any', rules: [newRule('genre'), newRule('genre')] },
                ],
              })
            }
          >
            {t.smartLists.addGroup}
          </Button>
        )}
      </div>
    </div>
  )
}

/** One rule: what it looks at, whether it matches or not, and its value. */
function RuleEditor({
  rule,
  genres,
  languages,
  contentRatings,
  onChange,
  onRemove,
}: {
  rule: SmartRule
  genres: string[]
  languages: Option[]
  contentRatings: string[]
  onChange: (rule: SmartRule) => void
  onRemove: () => void
}) {
  const { t } = useI18n()
  const bound = (key: 'min' | 'max', value: number | null) =>
    onChange({ ...rule, [key]: value === null ? undefined : value })
  let value
  switch (rule.field) {
    case 'genre':
      value = (
        <SuggestInput
          aria-label={t.smartLists.genre}
          value={rule.value ?? ''}
          suggestions={genres}
          onValue={(text) => onChange({ ...rule, value: text })}
          onPick={(text) => onChange({ ...rule, value: text })}
          className="w-[220px]"
        />
      )
      break
    case 'year':
    case 'rating':
    case 'runtime':
    case 'votes': {
      const step = rule.field === 'rating' ? 0.1 : 1
      value = (
        <span className="flex items-center gap-2 text-small text-ink-2">
          <NumberInput
            aria-label={t.smartLists.from}
            placeholder={t.smartLists.from}
            value={rule.min ?? null}
            min={0}
            step={step}
            onValue={(n) => bound('min', n)}
          />
          <NumberInput
            aria-label={t.smartLists.to}
            placeholder={t.smartLists.to}
            value={rule.max ?? null}
            min={0}
            step={step}
            onValue={(n) => bound('max', n)}
          />
          {rule.field === 'runtime' && t.smartLists.minutes}
        </span>
      )
      break
    }
    case 'released': {
      // The rules the editor makes: the last days up to today, or the next days from today.
      const next = (rule.min ?? 0) >= 0 && (rule.max ?? 0) > 0
      const days = next ? (rule.max ?? 0) : -(rule.min ?? 0)
      const set = (toNext: boolean, count: number | null) =>
        onChange({
          ...rule,
          min: count === null ? undefined : toNext ? 0 : -count,
          max: count === null ? undefined : toNext ? count : 0,
        })
      value = (
        <span className="flex items-center gap-2 text-small text-ink-2">
          <Select
            aria-label={t.smartLists.releasedWhen}
            className="w-[160px]"
            value={next ? 'next' : 'last'}
            onValue={(when) => set(when === 'next', days)}
            options={(['last', 'next'] as const).map((v) => ({
              value: v,
              label: t.smartLists.releasedWithin[v],
            }))}
          />
          <NumberInput
            aria-label={t.smartLists.days}
            value={days}
            min={1}
            max={36500}
            step={1}
            onValue={(n) => set(next, n)}
          />
          {t.smartLists.days}
        </span>
      )
      break
    }
    case 'contentRating':
      value = (
        <span className="flex items-center gap-2 text-small text-ink-2">
          {t.smartLists.atMost}
          <Select
            aria-label={t.smartLists.fields.contentRating}
            className="w-[160px]"
            value={rule.value ?? 'PG-13'}
            onValue={(rating) => onChange({ ...rule, value: rating })}
            options={contentRatings.map((v) => ({ value: v, label: v }))}
          />
        </span>
      )
      break
    case 'person':
    case 'studio':
      value = (
        <TextInput
          aria-label={t.smartLists.fields[rule.field]}
          placeholder={t.smartLists.namePlaceholder[rule.field]}
          value={rule.value ?? ''}
          maxLength={100}
          onValue={(text) => onChange({ ...rule, value: text })}
          className="w-[220px]"
        />
      )
      break
    case 'language':
      value = (
        <Select
          aria-label={t.smartLists.language}
          className="w-[220px]"
          value={rule.value ?? ''}
          onValue={(code) => onChange({ ...rule, value: code })}
          options={languages}
        />
      )
      break
    case 'type':
      value = (
        <Select
          aria-label={t.smartLists.fields.type}
          className="w-[160px]"
          value={rule.value ?? 'movie'}
          onValue={(kind) => onChange({ ...rule, value: kind })}
          options={['movie', 'series'].map((v) => ({ value: v, label: t.smartLists.types[v] }))}
        />
      )
      break
    case 'rank':
      value = (
        <span className="flex items-center gap-2 text-small text-ink-2">
          {t.smartLists.rankMax}
          <NumberInput
            aria-label={t.smartLists.rankMax}
            value={rule.max ?? null}
            min={1}
            max={MAX_LIBRARY_ITEMS}
            step={1}
            onValue={(n) => bound('max', n)}
          />
        </span>
      )
      break
    case 'state':
      value = (
        <Select
          aria-label={t.smartLists.fields.state}
          className="w-[200px]"
          value={rule.value ?? 'unplayed'}
          onValue={(state) => onChange({ ...rule, value: state })}
          options={states.map((v) => ({ value: v, label: t.smartLists.states[v] }))}
        />
      )
  }
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Select
        aria-label={t.smartLists.field}
        className="w-[200px]"
        value={rule.field}
        onValue={(field) =>
          onChange({ ...newRule(field as (typeof fields)[number]), not: rule.not })
        }
        options={fields.map((f) => ({ value: f, label: t.smartLists.fields[f] }))}
      />
      <Select
        aria-label={t.smartLists.negation}
        className="w-[120px]"
        value={rule.not ? 'not' : 'is'}
        onValue={(v) => onChange({ ...rule, not: v === 'not' })}
        options={[
          { value: 'is', label: t.smartLists.is },
          { value: 'not', label: t.smartLists.isNot },
        ]}
      />
      {value}
      <IconButton label={t.smartLists.removeRule} icon={XIcon} onClick={onRemove} />
    </div>
  )
}

/** The titles the list shows now, refreshed shortly after each change while descriptions come. */
function Preview({ scope, draft }: { scope: Scope; draft: SmartListDraft }) {
  const { t, language } = useI18n()
  const [settled, setSettled] = useState(draft)
  useEffect(() => {
    const timer = window.setTimeout(() => setSettled(draft), 500)
    return () => window.clearTimeout(timer)
  }, [draft])
  const ready =
    settled.catalogs.length > 0 &&
    complete(settled.rules) &&
    complete(settled.exclude) &&
    (settled.maxItems === null ||
      (Number.isInteger(settled.maxItems) &&
        settled.maxItems >= 1 &&
        settled.maxItems <= MAX_LIBRARY_ITEMS))
  const draftForServer = { ...settled, name: settled.name.trim() || '·' }
  const preview = useQuery({
    queryKey: queryKeys.smartPreview(scope, draftForServer),
    queryFn: ({ signal }) => previewSmartList(scope, draftForServer, signal),
    enabled: ready,
    placeholderData: keepPreviousData,
    refetchInterval: (query) => {
      const data = query.state.data
      return data && (data.unknown > 0 || !data.complete) ? 3000 : false
    },
  })
  const names = useMemo(() => new Intl.DisplayNames([language], { type: 'language' }), [language])
  const number = (n: number) => n.toLocaleString(language)

  let body
  if (settled.catalogs.length === 0) body = <Notice>{t.smartLists.previewNeedsCatalog}</Notice>
  else if (!ready) body = <Notice>{t.errors.invalid_smart_list_rules}</Notice>
  else if (preview.isPending) body = <SkeletonRows rows={4} />
  else if (preview.isError)
    body = (
      <InlineError onRetry={() => void preview.refetch()} retrying={preview.isFetching}>
        {errorMessage(t, preview.error)}
      </InlineError>
    )
  else {
    const data = preview.data
    body = (
      <div className="flex flex-col gap-4">
        <p className="text-body text-ink-2" role="status">
          <span className="figures font-medium text-ink">
            {t.smartLists.listed(number(data.listed))}
          </span>
          {data.titles.length < data.listed &&
            ` · ${t.smartLists.firstOf(number(data.titles.length), number(data.listed))}`}
        </p>
        {data.unknown > 0 && <Notice>{t.smartLists.unknown(number(data.unknown))}</Notice>}
        {!data.complete && <Notice tone="warn">{t.smartLists.incomplete}</Notice>}
        {data.titles.length === 0 ? (
          <EmptyState icon={FunnelSimpleIcon} title={t.smartLists.previewEmpty} />
        ) : (
          <Table label={t.smartLists.previewLabel} minWidth={640}>
            <thead>
              <tr>
                <th scope="col">{t.smartLists.columns.title}</th>
                <th scope="col">{t.smartLists.columns.type}</th>
                <th scope="col" className="text-right">
                  {t.smartLists.columns.year}
                </th>
                <th scope="col" className="text-right">
                  {t.smartLists.columns.rating}
                </th>
                <th scope="col">{t.smartLists.columns.genres}</th>
                <th scope="col">{t.smartLists.columns.languages}</th>
              </tr>
            </thead>
            <tbody>
              {data.titles.map((title) => (
                <tr key={title.id}>
                  <td className="font-medium !text-ink">{title.name}</td>
                  <td>{t.smartLists.types[title.type]}</td>
                  <td className="figures text-right">{title.year ?? '–'}</td>
                  <td className="figures text-right">
                    {title.rating?.toLocaleString(language) ?? '–'}
                  </td>
                  <td>{title.genres.join(', ') || '–'}</td>
                  <td>{title.languages.map((code) => names.of(code) ?? code).join(', ') || '–'}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </div>
    )
  }
  return (
    <Block
      title={t.smartLists.preview}
      aside={
        ready && (
          <Button
            size="sm"
            variant="ghost"
            loading={preview.isFetching}
            onClick={() => void preview.refetch()}
          >
            {t.smartLists.refresh}
          </Button>
        )
      }
    >
      <p className="mb-4 max-w-[70ch] text-small text-ink-3">{t.smartLists.previewHelp}</p>
      {body}
    </Block>
  )
}
