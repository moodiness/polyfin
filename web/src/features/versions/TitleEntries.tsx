import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { fetchTitleEntries, keepEntryApart, queryClient, queryKeys, type TitleEntry } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Badge, Block, Button, ConfirmDialog, InlineError, Row, RowList, useToast } from '@/ui'

/**
 * The entries of the title's IPTV source that share its TMDB id, for a source that groups the
 * copies of a title as versions: its versions first, each with Keep apart, asked first, then those
 * kept apart, each with Group again, at once. Shown once there are two, or one is kept apart; an
 * episode shows its series'.
 */
export function TitleEntries({ id, user }: { id: string; user: string }) {
  const { t } = useI18n()
  const text = t.versions
  const toast = useToast()
  /** The entry Keep apart asks about; null when not asking. */
  const [confirming, setConfirming] = useState<TitleEntry | null>(null)
  const entries = useQuery({
    queryKey: queryKeys.titleEntries(id, user),
    queryFn: ({ signal }) => fetchTitleEntries(id, user, signal),
  })
  const apart = useMutation({
    mutationFn: (entry: { key: string; apart: boolean }) =>
      keepEntryApart(id, user, entry.key, entry.apart),
    onSuccess: (answer, entry) => {
      queryClient.setQueryData(queryKeys.titleEntries(id, user), answer)
      toast(entry.apart ? text.keptApartDone : text.groupedAgainDone, { tone: 'ok' })
      setConfirming(null)
      // The versions of the title change, and those of its series' other episodes with them, as
      // may the titles a search lists; the item the page shows may be another now, which the page
      // follows.
      for (const key of [
        'title-versions',
        'title-entries',
        'version-title',
        'version-titles',
        'version-episodes',
      ])
        void queryClient.invalidateQueries({ queryKey: [key] })
    },
    // Keep apart shows its error in its dialog.
    onError: (error, entry) => {
      if (!entry.apart) toast(errorMessage(t, error), { tone: 'danger' })
    },
  })
  if (entries.isPending) return null
  if (entries.isError)
    return (
      <Block title={text.entries}>
        <InlineError onRetry={() => void entries.refetch()} retrying={entries.isFetching}>
          {errorMessage(t, entries.error)}
        </InlineError>
      </Block>
    )
  const list = entries.data.entries
  if (list.length < 2 && !list.some((entry) => entry.apart)) return null
  return (
    <Block title={text.entries} count={list.length}>
      <p className="mb-3 max-w-measure text-small text-ink-2">{text.entriesHelp}</p>
      <RowList compact aria-label={text.entries}>
        {list.map((entry) => (
          <Row
            key={entry.key}
            title={<span className="break-all">{entry.name || entry.key}</span>}
            titleAside={
              entry.leader || entry.apart || !entry.listed ? (
                <span className="flex flex-wrap gap-1.5">
                  {entry.leader && <Badge tone="accent">{text.firstEntry}</Badge>}
                  {entry.apart && <Badge tone="warn">{text.keptApart}</Badge>}
                  {!entry.listed && <Badge>{text.notListed}</Badge>}
                </span>
              ) : undefined
            }
            meta={[entry.quality, entry.category].filter(Boolean).join(' · ') || undefined}
            trailing={
              entry.listed && (entry.member || entry.apart) ? (
                <Button
                  size="sm"
                  loading={apart.isPending && apart.variables.key === entry.key}
                  disabled={apart.isPending && apart.variables.key !== entry.key}
                  onClick={() =>
                    entry.apart
                      ? apart.mutate({ key: entry.key, apart: false })
                      : setConfirming(entry)
                  }
                >
                  {entry.apart ? text.groupAgain : text.keepApart}
                </Button>
              ) : undefined
            }
          />
        ))}
      </RowList>
      <ConfirmDialog
        open={confirming !== null}
        onClose={() => {
          setConfirming(null)
          apart.reset()
        }}
        onConfirm={() => confirming && apart.mutate({ key: confirming.key, apart: true })}
        title={text.keepApartTitle(confirming?.name || confirming?.key || '')}
        confirmLabel={text.keepApart}
        tone="primary"
        busy={apart.isPending}
        error={apart.isError ? errorMessage(t, apart.error) : undefined}
      >
        <p>{confirming?.leader ? text.keepApartFirst : text.keepApartOther}</p>
      </ConfirmDialog>
    </Block>
  )
}
