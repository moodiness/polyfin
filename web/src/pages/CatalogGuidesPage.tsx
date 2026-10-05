import { Link, NavLink, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchLibraries, queryClient, queryKeys, type CatalogTarget } from '@/api'
import CatalogGuides from '@/components/lineup/CatalogGuides'
import Mappings from '@/components/lineup/Mappings'
import { buttonSecondary, PageHeader } from '@/components/ui'
import { useI18n } from '@/i18n'
import { catalogGuidesPath, routeScope } from '@/components/lineup/common'

/** The guides and guide mapping of a Stremio addon's Live TV catalog. */
export default function CatalogGuidesPage() {
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
  const back = scope === 'shared' ? '/libraries' : '/me/sources'

  if (scope === null || (section !== 'guides' && section !== 'mapping')) {
    return (
      <>
        <PageHeader title={t.notFound.title} description={t.notFound.description} />
        <Link to={back} className={buttonSecondary}>
          {t.lineup.catalog.back}
        </Link>
      </>
    )
  }
  const changed = () => void queryClient.invalidateQueries({ queryKey: queryKeys.scope(scope) })
  return (
    <>
      <p className="mb-3 text-sm">
        <Link
          to={back}
          className="text-muted underline decoration-line underline-offset-4 hover:text-white hover:decoration-fin-5"
        >
          ← {t.lineup.catalog.back}
        </Link>
      </p>
      <PageHeader
        title={
          library
            ? t.lineup.catalog.title(library.name ?? library.catalogName, library.addonName)
            : t.lineup.catalog.titleLoading
        }
        description={t.lineup.catalog.description}
      />
      <nav aria-label={t.lineup.sectionsLabel} className="mb-6">
        <ul className="flex gap-1 border-b border-line text-sm">
          {(['guides', 'mapping'] as const).map((name) => (
            <li key={name}>
              <NavLink
                to={catalogGuidesPath(scope, target, name === 'mapping')}
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
      {section === 'guides' ? (
        <CatalogGuides scope={scope} target={target} onChanged={changed} />
      ) : (
        <Mappings scope={scope} target={target} onChanged={changed} />
      )}
    </>
  )
}
