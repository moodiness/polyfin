import { FunnelSimpleIcon, PlusIcon, TrashIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import {
  deleteSmartList,
  fetchSmartLists,
  queryClient,
  queryKeys,
  type Scope,
  type SmartList,
  type SmartRule,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  Block,
  Button,
  ButtonLink,
  ConfirmDialog,
  EmptyState,
  InlineError,
  Row,
  RowList,
  SkeletonRows,
  useToast,
} from '@/ui'

/** The page of a scope's smart list, `new` for one being made. */
export function smartListPath(scope: Scope, id: string): string {
  return scope === 'shared' ? `/libraries/smart/${id}` : `/me/sources/smart/${id}`
}

/** Counts the rules of a group, those of the groups within it included. */
function ruleCount(group: SmartRule): number {
  return (group.rules ?? []).reduce(
    (count, rule) => count + (rule.field === 'group' ? ruleCount(rule) : 1),
    0,
  )
}

/**
 * The smart lists of a scope, each opening its page, with the button that makes another: the
 * server's under Content › Libraries, a user's own under My sources. `kept` lists a user's own
 * lists while their own sources are not used: no list can be made or changed then, so each row
 * offers Delete only, and nothing shows without lists.
 */
export default function SmartLists({ scope, kept = false }: { scope: Scope; kept?: boolean }) {
  const { t, language } = useI18n()
  const toast = useToast()
  const lists = useQuery({
    queryKey: queryKeys.smartLists(scope),
    queryFn: ({ signal }) => fetchSmartLists(scope, signal),
  })
  const [deleting, setDeleting] = useState<SmartList>()
  const remove = useMutation({
    mutationFn: (list: SmartList) => deleteSmartList(scope, list.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.smartLists(scope) })
      toast(t.smartLists.deleted)
      setDeleting(undefined)
    },
  })
  if (kept && lists.data?.length === 0) return null
  const add = kept ? undefined : (
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
            to={kept ? undefined : smartListPath(scope, list.id)}
            title={list.name}
            titleAside={
              <Badge>
                {list.shownAs === 'library'
                  ? t.smartLists.shownAsLibrary
                  : t.smartLists.shownAsCollection}
              </Badge>
            }
            meta={summary(list)}
            muted={kept}
            trailing={
              kept ? (
                <Button
                  variant="ghost"
                  size="sm"
                  icon={TrashIcon}
                  aria-label={t.smartLists.deleteLabel(list.name)}
                  onClick={() => {
                    remove.reset()
                    setDeleting(list)
                  }}
                >
                  {t.smartLists.delete}
                </Button>
              ) : undefined
            }
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

  let help = t.smartLists.help
  if (kept) help = t.smartLists.helpKept
  else if (scope === 'me') help = t.smartLists.helpMine
  return (
    <Block
      title={t.smartLists.title}
      count={lists.data?.length}
      aside={lists.data && lists.data.length > 0 ? add : undefined}
    >
      <p className="mb-3 max-w-[66ch] text-small text-ink-2">{help}</p>
      {body}
      {kept && (
        <ConfirmDialog
          open={deleting !== undefined}
          onClose={() => setDeleting(undefined)}
          onConfirm={() => deleting && remove.mutate(deleting)}
          title={t.smartLists.deleteTitle(deleting?.name ?? '')}
          confirmLabel={t.smartLists.delete}
          tone="danger"
          busy={remove.isPending}
          error={remove.isError ? errorMessage(t, remove.error) : undefined}
        >
          {t.smartLists.deleteKeptBody}
        </ConfirmDialog>
      )}
    </Block>
  )
}
