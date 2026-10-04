import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, useParams } from 'react-router'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  fetchAddons,
  previewSource,
  queryClient,
  queryKeys,
  refreshAddon,
  updateIptvSource,
  type Addon,
  type IptvOptions,
  type IptvSource,
  type Scope,
} from '@/api'
import { icons } from '@/components/icons'
import { invalidateLineup, iptvCatalog, lineupPath, routeScope } from '@/components/lineup/common'
import CatalogGuides from '@/components/lineup/CatalogGuides'
import Categories from '@/components/lineup/Categories'
import Channels from '@/components/lineup/Channels'
import { ExclusionPicker, OptionsFields } from '@/components/lineup/ImportOptions'
import Mappings from '@/components/lineup/Mappings'
import { Panel, Skeleton, Stat } from '@/components/panels'
import { buttonPrimary, buttonSecondary, Notice, PageHeader, RelativeTime } from '@/components/ui'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'

export const lineupSections = [
  'summary',
  'options',
  'categories',
  'channels',
  'guides',
  'mapping',
] as const
export type LineupSection = (typeof lineupSections)[number]

/** One IPTV source's line-up: its categories, channels, options and guides, section by section. */
export default function LineupPage() {
  const { t } = useI18n()
  const params = useParams()
  const scope = routeScope(params.scope)
  const id = params.id ?? ''
  const section = (lineupSections as readonly string[]).includes(params.section ?? 'summary')
    ? ((params.section ?? 'summary') as LineupSection)
    : null
  const addons = useQuery({
    queryKey: queryKeys.addons(scope ?? 'me'),
    queryFn: ({ signal }) => fetchAddons(scope ?? 'me', signal),
    enabled: scope !== null,
  })
  const addon = addons.data?.find((item) => item.id === id && item.source !== null)
  const back = scope === 'shared' ? '/addons' : '/my-addons'
  const tabs = useRef<HTMLElement>(null)
  // On a phone the sections scroll sideways: the one open stays in view.
  useEffect(() => {
    tabs.current
      ?.querySelector('[aria-current="page"]')
      ?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [section])

  if (scope === null || section === null || (addons.isSuccess && addon === undefined)) {
    return (
      <>
        <PageHeader title={t.notFound.title} description={t.lineup.notFound} />
        <Link to={back} className={buttonSecondary}>
          {scope === 'shared' ? t.lineup.backShared : t.lineup.backMine}
        </Link>
      </>
    )
  }

  return (
    <>
      <p className="mb-3 text-sm">
        <Link
          to={back}
          className="text-muted underline decoration-line underline-offset-4 hover:text-white hover:decoration-fin-5"
        >
          ← {scope === 'shared' ? t.lineup.backShared : t.lineup.backMine}
        </Link>
      </p>
      <PageHeader
        title={addon ? t.lineup.title(addon.name) : t.lineup.titleLoading}
        description={t.lineup.description}
      />
      <nav
        ref={tabs}
        aria-label={t.lineup.sectionsLabel}
        className="-mx-4 mb-6 overflow-x-auto px-4"
      >
        <ul className="flex gap-1 border-b border-line text-sm whitespace-nowrap">
          {lineupSections.map((name) => (
            <li key={name}>
              <NavLink
                to={lineupPath(scope, id, name === 'summary' ? undefined : name)}
                end
                className={({ isActive }) =>
                  `-mb-px inline-flex min-h-10 items-center border-b-2 px-3 font-medium transition-colors ${
                    isActive
                      ? 'border-fin-5 text-white'
                      : 'border-transparent text-muted hover:text-white'
                  }`
                }
              >
                {t.lineup.sections[name]}
              </NavLink>
            </li>
          ))}
        </ul>
      </nav>
      {addons.isPending ? (
        <Skeleton rows={4} label={t.common.loading} />
      ) : addons.isError ? (
        <Notice kind="error">{errorMessage(t, addons.error)}</Notice>
      ) : addon === undefined ? null : section === 'summary' ? (
        <Summary scope={scope} addon={addon} source={addon.source as IptvSource} />
      ) : section === 'options' ? (
        <Options scope={scope} id={id} source={addon.source as IptvSource} />
      ) : section === 'categories' ? (
        <Categories scope={scope} id={id} />
      ) : section === 'channels' ? (
        <Channels scope={scope} id={id} />
      ) : section === 'guides' ? (
        <CatalogGuides
          scope={scope}
          target={iptvCatalog(id)}
          onChanged={() => invalidateLineup(scope, id)}
        />
      ) : (
        <Mappings
          scope={scope}
          target={iptvCatalog(id)}
          onChanged={() => {
            // The line-up's counts follow; the mapping list keeps its rows until it is read again.
            void queryClient.invalidateQueries({ queryKey: queryKeys.lineup(scope, id) })
            void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
          }}
        />
      )}
    </>
  )
}

function Summary({ scope, addon, source }: { scope: Scope; addon: Addon; source: IptvSource }) {
  const { language, t } = useI18n()
  const text = t.lineup.summary
  const number = (n: number) => n.toLocaleString(language)
  const lineup = source.lineup
  const refresh = useMutation({
    mutationFn: () => refreshAddon(scope, addon.id),
    onSettled: () => invalidateLineup(scope, addon.id),
  })
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat
          label={text.categories}
          value={number(lineup.enabledCategories)}
          detail={text.ofTotal(number(lineup.categories))}
          to={lineupPath(scope, addon.id, 'categories')}
        />
        <Stat
          label={text.shown}
          value={number(lineup.shownChannels)}
          detail={text.enabledOf(number(lineup.enabledChannels), number(lineup.channels))}
          tone={lineup.shownChannels > 0 ? 'active' : undefined}
          to={lineupPath(scope, addon.id, 'channels')}
        />
        <Stat
          label={text.mapped}
          value={number(lineup.mapped)}
          detail={text.unmapped(number(lineup.unmapped))}
          to={lineupPath(scope, addon.id, 'mapping')}
        />
        <Stat label={text.entries} value={number(source.channels)} detail={source.address} />
      </div>
      <Panel
        title={text.listTitle}
        description={text.refreshHelp}
        actions={
          <button
            type="button"
            className={buttonPrimary}
            disabled={refresh.isPending}
            onClick={() => refresh.mutate()}
          >
            <icons.download className="size-4" />
            {refresh.isPending ? text.refreshing : text.refresh}
          </button>
        }
      >
        <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-3">
          <div>
            <dt className="text-xs text-muted">{t.iptv.lastFetch}</dt>
            <dd className="text-zinc-100">
              {source.fetchedAt ? <RelativeTime iso={source.fetchedAt} /> : t.iptv.never}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted">{text.lastAttempt}</dt>
            <dd className="text-zinc-100">
              {source.checkedAt ? <RelativeTime iso={source.checkedAt} /> : t.iptv.never}
            </dd>
          </div>
          <div>
            <dt className="text-xs text-muted">{t.iptv.nextFetch}</dt>
            <dd className="text-zinc-100">
              {source.nextAt ? <RelativeTime iso={source.nextAt} /> : '–'}
            </dd>
          </div>
        </dl>
        <div aria-live="polite" className="mt-4 space-y-2 empty:hidden">
          {source.error !== '' && (
            <Notice kind="error">{t.iptv.errors[source.error] ?? t.errors.generic}</Notice>
          )}
          {refresh.isError && <Notice kind="error">{errorMessage(t, refresh.error)}</Notice>}
          {refresh.isSuccess && <Notice kind="success">{text.refreshed}</Notice>}
        </div>
      </Panel>
    </div>
  )
}

function Options({ scope, id, source }: { scope: Scope; id: string; source: IptvSource }) {
  const { t } = useI18n()
  const text = t.lineup.options
  const [draft, setDraft] = useState<IptvOptions>(source.options)
  const dirty = JSON.stringify(draft) !== JSON.stringify(source.options)
  const save = useMutation({
    mutationFn: () => updateIptvSource(scope, id, { options: draft }),
    onSuccess: (updated) => {
      queryClient.setQueryData<Addon[]>(queryKeys.addons(scope), (list) =>
        list?.map((item) => (item.id === updated.id ? updated : item)),
      )
      if (updated.source !== null) setDraft(updated.source.options)
      // The preview's ticks follow the options saved.
      void queryClient.invalidateQueries({ queryKey: ['lineup', scope, id, 'preview'] })
    },
    onSettled: () => invalidateLineup(scope, id),
  })
  return (
    <Panel title={text.title} description={text.help}>
      <div className="space-y-6">
        <OptionsFields
          value={draft}
          onChange={(patch) => {
            save.reset()
            setDraft((current) => ({ ...current, ...patch }))
          }}
        />
        <ExclusionPicker
          queryKey={['lineup', scope, id, 'preview']}
          load={(by, signal) => previewSource(scope, id, by, signal)}
          excluded={draft.excluded}
          onChange={(excluded) => {
            save.reset()
            setDraft((current) => ({ ...current, excluded }))
          }}
        />
        <div className="sticky bottom-0 -mx-5 -mb-5 space-y-3 rounded-b-2xl border-t border-line bg-surface/95 px-5 py-3 backdrop-blur">
          {save.isError && <Notice kind="error">{errorMessage(t, save.error)}</Notice>}
          {save.isSuccess && !dirty && <Notice kind="success">{text.saved}</Notice>}
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p aria-live="polite" className={`text-sm ${dirty ? 'text-amber-200' : 'text-muted'}`}>
              {dirty ? t.dashboard.settings.unsaved : t.dashboard.settings.upToDate}
            </p>
            <div className="flex gap-2">
              <button
                type="button"
                className={buttonSecondary}
                disabled={!dirty || save.isPending}
                onClick={() => setDraft(source.options)}
              >
                {t.libraries.reset}
              </button>
              <button
                type="button"
                className={buttonPrimary}
                disabled={!dirty || save.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? text.saving : text.save}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Panel>
  )
}
