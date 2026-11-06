import { SparkleIcon } from '@phosphor-icons/react'
import { useMutation, useQuery } from '@tanstack/react-query'
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
  SettingRow,
  SkeletonRows,
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
        <div className="space-y-6">
          <div className="max-w-lg space-y-2.5">
            <div className="flex items-baseline justify-between gap-4 text-body">
              <span className="text-ink">{text.certainty}</span>
              <span className="figures text-lead font-semibold text-ink">
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
            <p className="text-small text-ink-2">
              {text.builtFrom(taste.titles)} <RelativeTime iso={taste.built} />
            </p>
          </div>
          {!taste.confident && <Notice>{text.belowGate}</Notice>}
          {facets.length > 0 && (
            <dl className="grid gap-x-8 gap-y-6 sm:grid-cols-2">
              {facets.map((facet) => (
                <div key={facet.facet} className="min-w-0">
                  <dt className="text-h3 text-ink">{text.facets[facet.facet]}</dt>
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
      <SettingRow
        kind="switch"
        label={text.taglines}
        help={text.taglinesHelp}
        checked={taste.taglines}
        disabled={save.isPending}
        onChange={(on) => save.mutate(on)}
      />
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
