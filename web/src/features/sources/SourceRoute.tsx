import { ArrowLeftIcon } from '@phosphor-icons/react'
import { useNavigate, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchAddons, queryKeys, type Addon, type Scope } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { useSessionUser } from '@/app/session'
import IptvSourceRoute from '@/features/iptv/IptvSourceRoute'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { ButtonLink, InlineError, Skeleton } from '@/ui'
import { isIptv } from './model'
import { SourceDetail } from './SourceDetail'

/**
 * `/sources/:scope/:id/:section?` and `/me/sources/:id/:section?` (with `scope="me"`): one
 * source. An IPTV source gets its own page from the IPTV area; a Stremio or music addon gets its
 * details, actions and settings here.
 */
export default function SourceRoute({ scope: fixedScope }: { scope?: Scope }) {
  const { t } = useI18n()
  const params = useParams()
  const scopeParam = fixedScope ?? params.scope
  const scope: Scope | null = scopeParam === 'shared' || scopeParam === 'me' ? scopeParam : null
  const back =
    scope === 'me'
      ? { to: '/me/sources', label: t.sources.backMine }
      : { to: '/sources', label: t.sources.back }
  const addons = useQuery({
    queryKey: queryKeys.addons(scope ?? 'shared'),
    queryFn: ({ signal }) => fetchAddons(scope ?? 'shared', signal),
    enabled: scope !== null,
  })

  if (scope === null) return <NotFound back={back} />
  if (addons.isPending) {
    return (
      <div aria-busy="true">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="mt-block h-[420px] rounded-panel" />
      </div>
    )
  }
  if (addons.isError) {
    return (
      <PageLayout title={t.sources.title} back={back}>
        <InlineError onRetry={() => void addons.refetch()} retrying={addons.isRefetching}>
          {errorMessage(t, addons.error)}
        </InlineError>
      </PageLayout>
    )
  }
  const index = addons.data.findIndex((addon) => addon.id === params.id)
  if (index === -1) return <NotFound back={back} />
  const addon = addons.data[index]
  if (isIptv(addon)) return <IptvSourceRoute scope={fixedScope} />
  return (
    <AddonPage scope={scope} addon={addon} index={index} count={addons.data.length} back={back} />
  )
}

function AddonPage({
  scope,
  addon,
  index,
  count,
  back,
}: {
  scope: Scope
  addon: Addon
  index: number
  count: number
  back: { to: string; label: string }
}) {
  const user = useSessionUser()
  const navigate = useNavigate()
  return (
    <PageLayout title={addon.name} back={back}>
      <div className="max-w-[880px]">
        <SourceDetail
          entry={{ key: `${scope}:${addon.id}`, addon, owner: null, scope }}
          selfId={user.id}
          mode="page"
          index={index}
          count={count}
          onRemoved={() => void navigate(back.to, { replace: true })}
        />
      </div>
    </PageLayout>
  )
}

function NotFound({ back }: { back: { to: string; label: string } }) {
  const { t } = useI18n()
  return (
    <PageLayout title={t.sources.notFoundTitle} lede={t.sources.notFound} back={back}>
      <div>
        <ButtonLink to={back.to} icon={ArrowLeftIcon}>
          {back.label}
        </ButtonLink>
      </div>
    </PageLayout>
  )
}
