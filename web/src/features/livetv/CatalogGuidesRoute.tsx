import { CalendarBlankIcon } from '@phosphor-icons/react'
import { useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchLibraries, queryClient, queryKeys, type CatalogTarget } from '@/api'
import { PageLayout } from '@/app/PageLayout'
import { catalogGuidesPath, routeScope } from '@/components/lineup/common'
import CatalogGuides from '@/features/iptv/CatalogGuides'
import Mappings from '@/features/iptv/Mappings'
import { useI18n } from '@/i18n'
import { ButtonLink, EmptyState, Tabs } from '@/ui'

/**
 * `/live-tv/guides/:scope/:addonId/:catalogId/:section?`: the guides and guide mapping of a
 * Stremio addon's TV catalog, with the IPTV area's guide and mapping components.
 */
export default function CatalogGuidesRoute() {
  const { t } = useI18n()
  const params = useParams()
  const scope = routeScope(params.scope)
  const section = params.section ?? 'guides'
  const target: CatalogTarget = {
    addonId: params.addonId ?? '',
    catalogType: 'tv',
    catalogId: params.catalogId ?? '',
  }
  const libraries = useQuery({
    queryKey: queryKeys.libraries(scope ?? 'me'),
    queryFn: ({ signal }) => fetchLibraries(scope ?? 'me', signal),
    enabled: scope !== null,
  })
  const library = libraries.data?.find(
    (l) =>
      l.addonId === target.addonId && l.catalogType === 'tv' && l.catalogId === target.catalogId,
  )
  const back = {
    to: scope === 'shared' ? '/live-tv' : '/me/sources',
    label: scope === 'shared' ? t.lineup.catalog.back : t.lineup.backMine,
  }

  if (scope === null || (section !== 'guides' && section !== 'mapping')) {
    return (
      <PageLayout title={t.lineup.notFoundTitle} back={back}>
        <EmptyState
          icon={CalendarBlankIcon}
          title={t.lineup.notFoundTitle}
          action={<ButtonLink to={back.to}>{back.label}</ButtonLink>}
        />
      </PageLayout>
    )
  }
  const changed = () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
  return (
    <PageLayout
      back={back}
      title={
        library
          ? t.lineup.catalog.title(library.name ?? library.catalogName, library.addonName)
          : t.lineup.catalog.titleLoading
      }
      lede={t.lineup.catalog.description}
    >
      <Tabs
        variant="underline"
        label={t.lineup.sectionsLabel}
        items={(['guides', 'mapping'] as const).map((name) => ({
          id: name,
          label: t.lineup.sections[name],
          to: catalogGuidesPath(scope, target, name === 'mapping'),
          end: true,
        }))}
      />
      <div className="!mt-8">
        {section === 'guides' ? (
          <CatalogGuides
            scope={scope}
            target={target}
            onChanged={changed}
            mappingPath={catalogGuidesPath(scope, target, true)}
          />
        ) : (
          <Mappings scope={scope} target={target} onChanged={changed} />
        )}
      </div>
    </PageLayout>
  )
}
