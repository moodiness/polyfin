import { useParams } from 'react-router'
import type { Scope } from '@/api'
import { useSessionUser } from '@/app/session'
import { SourcesPage } from './SourcesPage'

/**
 * `/sources/:scope/:id/:section?` and `/me/sources/:id/:section?` (with `scope="me"`): one
 * source's page, beside the list it belongs to: Content's Sources for an administrator, else My
 * sources. An IPTV source's page has its six parts (`:section`); an addon's or a folder's one.
 */
export default function SourceRoute({ scope: fixedScope }: { scope?: Scope }) {
  const params = useParams()
  const user = useSessionUser()
  const scopeParam = fixedScope ?? params.scope
  // An unknown scope names no source: the page says it was not found.
  const scope: Scope = scopeParam === 'shared' || scopeParam === 'me' ? scopeParam : 'shared'
  const known = scopeParam === scope
  return (
    <SourcesPage
      area={fixedScope === 'me' || !user.isAdministrator ? 'mine' : 'content'}
      open={{ scope, id: known ? (params.id ?? '') : '', section: params.section }}
    />
  )
}
