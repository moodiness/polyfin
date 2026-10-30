import { ArrowClockwiseIcon } from '@phosphor-icons/react'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { fetchPosterPreviews, queryKeys, type PosterBadgeSize, type PosterPreview } from '@/api'
import { errorMessage } from '@/format'
import { useI18n } from '@/i18n'
import { Button, InlineError, Skeleton } from '@/ui'

/** A poster through Polyfin, as apps load it, at a width asked of the server. */
function posterUrl(title: PosterPreview, tag: string, width: number) {
  return `/Items/${encodeURIComponent(title.id)}/Images/Primary?tag=${encodeURIComponent(tag)}&maxWidth=${width}`
}

/** One poster of the preview, with what it shows under it. */
function Poster({
  src,
  caption,
  displayWidth,
}: {
  src: string
  caption: string
  /** The width it is shown at, in CSS pixels. */
  displayWidth: number
}) {
  return (
    <figure className="flex flex-col items-start gap-2">
      <img
        src={src}
        alt=""
        loading="lazy"
        style={{ width: displayWidth }}
        className="aspect-[2/3] rounded-field border border-line-2 bg-s2 object-cover"
      />
      <figcaption className="text-small text-ink-3">{caption}</figcaption>
    </figure>
  )
}

/**
 * Settings › Content › Posters: a real title's poster as apps show it, without badges, with
 * them at the size chosen, and on a small card, whether the badges are on or not; Another title
 * goes through the titles of the administrator's libraries.
 */
export default function PosterPreviewPanel({ size }: { size: PosterBadgeSize }) {
  const { t } = useI18n()
  const text = t.settings.posterPreview
  const [index, setIndex] = useState(0)
  const previews = useQuery({
    queryKey: queryKeys.posterPreviews(size),
    queryFn: ({ signal }) => fetchPosterPreviews(size, signal),
  })
  if (previews.isPending) return <Skeleton className="h-72 w-full max-w-xl" />
  if (previews.isError) {
    return (
      <InlineError onRetry={() => void previews.refetch()} retrying={previews.isFetching}>
        {errorMessage(t, previews.error)}
      </InlineError>
    )
  }
  const titles = previews.data
  if (titles.length === 0) return <p className="text-small text-ink-3">{text.none}</p>
  const title = titles[index % titles.length]
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <p className="text-[15px] font-medium text-ink">{title.name}</p>
        {titles.length > 1 && (
          <Button
            size="sm"
            icon={ArrowClockwiseIcon}
            onClick={() => setIndex((current) => (current + 1) % titles.length)}
          >
            {text.another}
          </Button>
        )}
      </div>
      <div className="flex flex-wrap items-end gap-6">
        <Poster src={posterUrl(title, title.tag, 400)} caption={text.original} displayWidth={200} />
        <Poster
          src={posterUrl(title, title.badgedTag, 400)}
          caption={text.badged}
          displayWidth={200}
        />
        <Poster
          src={posterUrl(title, title.badgedTag, 240)}
          caption={text.card}
          displayWidth={120}
        />
      </div>
      {title.badgedTag === title.tag && <p className="text-small text-ink-3">{text.nothing}</p>}
    </div>
  )
}
