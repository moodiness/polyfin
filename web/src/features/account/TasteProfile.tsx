import { SparkleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useId } from 'react'
import {
  fetchOwnTaste,
  queryClient,
  queryKeys,
  saveOwnTaste,
  type OwnTaste,
  type TasteLeaning,
} from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import {
  Badge,
  EmptyState,
  InlineError,
  Notice,
  ProgressBar,
  RelativeTime,
  SkeletonRows,
  Switch,
  useToast,
} from '@/ui'

/**
 * My account › Taste profile: what the server learned of the user's taste, how sure it is, what
 * they like and dislike of each facet, and whether their titles' details tell why a title is for
 * them, saved as it changes. The profile stays on the server: only its user sees it.
 */
export default function TasteProfile() {
  const { language, t } = useI18n()
  const text = t.account.taste
  const toast = useToast()
  const labelId = useId()
  const helpId = useId()
  const profile = useQuery({
    queryKey: queryKeys.ownTaste,
    queryFn: ({ signal }) => fetchOwnTaste(signal),
  })
  const save = useMutation({
    mutationFn: saveOwnTaste,
    onSuccess: (saved) => {
      queryClient.setQueryData<OwnTaste>(queryKeys.ownTaste, saved)
      toast(saved.taglines ? text.taglinesOn : text.taglinesOff)
    },
  })

  if (profile.isPending) return <SkeletonRows rows={2} boxed label={t.common.loading} />
  if (profile.isError) {
    return (
      <InlineError onRetry={() => void profile.refetch()} retrying={profile.isFetching}>
        {errorMessage(t, profile.error)}
      </InlineError>
    )
  }
  const taste = profile.data
  const facets = taste.facets.filter((f) => f.liked.length + f.disliked.length > 0)
  return (
    <div className="space-y-6">
      {taste.built === null ? (
        <EmptyState icon={SparkleIcon} title={text.notBuilt}>
          {text.notBuiltHelp}
        </EmptyState>
      ) : (
        <div className="space-y-5">
          <div className="max-w-md space-y-2">
            <div className="flex items-baseline justify-between gap-4 text-small">
              <span className="text-ink">{text.certainty}</span>
              <span className="font-mono text-ink-2 tabular-nums">
                {new Intl.NumberFormat(language, {
                  style: 'percent',
                  maximumFractionDigits: 0,
                }).format(taste.certainty)}
              </span>
            </div>
            <ProgressBar
              value={taste.certainty}
              label={text.certainty}
              size="lg"
              variant={taste.confident ? 'accent' : 'neutral'}
            />
            <p className="text-small text-ink-3">
              {text.builtFrom(taste.titles)} <RelativeTime iso={taste.built} />
            </p>
          </div>
          {!taste.confident && <Notice>{text.belowGate}</Notice>}
          {facets.length > 0 && (
            <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2">
              {facets.map((facet) => (
                <div key={facet.facet} className="min-w-0">
                  <dt className="text-small font-medium text-ink">{text.facets[facet.facet]}</dt>
                  <dd className="mt-2 space-y-2">
                    {facet.liked.length > 0 && (
                      <Leanings label={text.liked} leanings={facet.liked} />
                    )}
                    {facet.disliked.length > 0 && (
                      <Leanings label={text.disliked} leanings={facet.disliked} />
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          )}
        </div>
      )}
      <div className="flex max-w-xl items-start justify-between gap-6 rounded-row border border-line bg-bg px-[18px] py-4 max-sm:gap-4 max-sm:p-3.5">
        <div className="min-w-0">
          <p id={labelId} className="text-control font-medium text-ink">
            {text.taglines}
          </p>
          <p id={helpId} className="mt-0.5 text-small text-ink-3">
            {text.taglinesHelp}
          </p>
        </div>
        <Switch
          checked={taste.taglines}
          disabled={save.isPending}
          labelledBy={labelId}
          describedById={helpId}
          onChange={(on) => save.mutate(on)}
          className="mt-0.5"
        />
      </div>
      {save.isError && <Notice tone="danger">{errorMessage(t, save.error)}</Notice>}
    </div>
  )
}

/** Values the user leans to or away from, the weightiest first; a vetoed genre says so. */
function Leanings({ label, leanings }: { label: string; leanings: TasteLeaning[] }) {
  const { t } = useI18n()
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="sr-only">{label}</span>
      {leanings.map((leaning) => (
        <Badge
          key={leaning.name}
          tone={leaning.weight > 0 ? 'accent' : leaning.vetoed ? 'danger' : 'neutral'}
        >
          {leaning.weight > 0 ? '' : '− '}
          {leaning.name}
          {leaning.vetoed && ` · ${t.account.taste.vetoed}`}
        </Badge>
      ))}
    </div>
  )
}
