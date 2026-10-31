import { FunnelSimpleIcon, PlusIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { fetchSmartLists, queryKeys, type Scope, type SmartList, type SmartRule } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Badge, Block, ButtonLink, EmptyState, InlineError, Row, RowList, SkeletonRows } from '@/ui'

/** The page of a scope's smart list, `new` for one being made. */
export function smartListPath(scope: Scope, id: string): string {
  return scope === 'shared' ? `/libraries/smart/${id}` : `/me/sources/smart/${id}`
}

/** Counts the rules of a group, those of the groups within it included. */
export function ruleCount(group: SmartRule): number {
  return (group.rules ?? []).reduce(
    (count, rule) => count + (rule.field === 'group' ? ruleCount(rule) : 1),
    0,
  )
}

/**
 * The smart lists of a scope, each opening its page, with the button that makes another: the
 * server's under Content › Libraries, a user's own under My sources.
 */
export default function SmartLists({ scope }: { scope: Scope }) {
  const { t, language } = useI18n()
  const lists = useQuery({
    queryKey: queryKeys.smartLists(scope),
    queryFn: ({ signal }) => fetchSmartLists(scope, signal),
  })
  const add = (
    <ButtonLink to={smartListPath(scope, 'new')} icon={PlusIcon} size="sm">
      {t.smartLists.add}
    </ButtonLink>
  )

  let body
  if (lists.isPending) body = <SkeletonRows rows={2} label={t.smartLists.loading} />
  else if (lists.isError)
    body = (
      <InlineError onRetry={() => void lists.refetch()} retrying={lists.isFetching}>
        {errorMessage(t, lists.error)}
      </InlineError>
    )
  else if (lists.data.length === 0)
    body = (
      <EmptyState icon={FunnelSimpleIcon} title={t.smartLists.emptyTitle} action={add}>
        {t.smartLists.emptyHow}
      </EmptyState>
    )
  else
    body = (
      <RowList variant="boxed">
        {lists.data.map((list) => (
          <Row
            key={list.id}
            leading={FunnelSimpleIcon}
            to={smartListPath(scope, list.id)}
            title={list.name}
            titleAside={
              <Badge>
                {list.shownAs === 'library'
                  ? t.smartLists.shownAsLibrary
                  : t.smartLists.shownAsCollection}
              </Badge>
            }
            meta={summary(list)}
          />
        ))}
      </RowList>
    )

  function summary(list: SmartList): string {
    const parts = [
      t.smartLists.catalogCount(list.catalogs.length),
      t.smartLists.ruleCount(ruleCount(list.rules) + ruleCount(list.exclude)),
      t.smartLists.sorts[list.sort],
    ]
    if (list.maxItems !== null)
      parts.push(t.smartLists.maxBadge(list.maxItems.toLocaleString(language)))
    return parts.join(' · ')
  }

  return (
    <Block
      title={t.smartLists.title}
      count={lists.data?.length}
      aside={lists.data && lists.data.length > 0 ? add : undefined}
    >
      <p className="mb-4 max-w-[70ch] text-small text-ink-3">
        {scope === 'shared' ? t.smartLists.help : t.smartLists.helpMine}
      </p>
      {body}
    </Block>
  )
}
