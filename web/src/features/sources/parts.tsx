import { useId, useState, type ReactNode } from 'react'
import type { Addon } from '@/api'
import { useI18n } from '@/i18n'
import { cx, IconTile, StatusPill } from '@/ui'
import { folderFailure, kindIcon } from './model'

/**
 * The source's logo, once it loads, over its kind icon. Logos are third-party images: when one is
 * missing, broken or blocked by the page's security policy, the icon stays, so nothing moves.
 */
function Logo({ addon, className }: { addon: Addon; className: string }) {
  const [state, setState] = useState<'loading' | 'loaded' | 'failed'>('loading')
  const src =
    addon.logo !== null && addon.logo.startsWith('https://') && state !== 'failed'
      ? addon.logo
      : null
  if (src === null) return null
  return (
    <img
      key={src}
      src={src}
      alt=""
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onLoad={() => setState('loaded')}
      onError={() => setState('failed')}
      className={cx(
        'absolute inset-0 bg-s3 object-contain',
        className,
        state === 'loaded' ? 'opacity-100' : 'opacity-0',
      )}
    />
  )
}

/** The source's kind icon in a round bubble (36 px in the list), covered by its logo. */
export function SourceTile({ addon, selected = false }: { addon: Addon; selected?: boolean }) {
  return (
    <span className="relative inline-grid shrink-0 self-start">
      <IconTile icon={kindIcon(addon)} size="sm" selected={selected} />
      <Logo addon={addon} className="size-9 rounded-full p-1" />
    </span>
  )
}

/** The big mark at the head of a source's page: its kind icon (72 px, 56 on a phone), or logo. */
export function SourceMark({ addon }: { addon: Addon }) {
  const Glyph = kindIcon(addon)
  return (
    <span
      aria-hidden="true"
      className="relative grid size-[4.5rem] shrink-0 place-items-center rounded-row bg-s2 text-ink-2 max-md:size-14 max-md:rounded-[1.125rem]"
    >
      <Glyph size="2.125rem" className="max-md:size-7" />
      <Logo addon={addon} className="size-full rounded-[inherit] p-2" />
    </span>
  )
}

/**
 * Whether a source works: on, off, its last IPTV download failed, or its local folder cannot be
 * read, as when its SFTP server offers another host key, or is being scanned. Always an icon and a
 * word.
 */
export function SourceStatus({ addon, className }: { addon: Addon; className?: string }) {
  const { t } = useI18n()
  if (!addon.enabled)
    return (
      <StatusPill tone="muted" className={className}>
        {t.sources.off}
      </StatusPill>
    )
  if (addon.source !== null && addon.source.error !== '')
    return (
      <StatusPill tone="danger" className={className}>
        {t.sources.failed}
      </StatusPill>
    )
  if (addon.folder !== null && folderFailure(addon.folder) !== '')
    return (
      <StatusPill tone="danger" className={className}>
        {t.localFolders.unreadable}
      </StatusPill>
    )
  if (addon.folder?.scanning)
    return (
      <StatusPill tone="live" className={className}>
        {t.localFolders.scanning}
      </StatusPill>
    )
  return (
    <StatusPill tone="ok" className={className}>
      {t.sources.active}
    </StatusPill>
  )
}

/**
 * A group of a source's page: its title (25 px) with an aside on the right (a count, a link), then
 * its rows or tiles. Groups are 52 px apart.
 */
export function Group({
  title,
  aside,
  children,
  className,
}: {
  title: ReactNode
  aside?: ReactNode
  children: ReactNode
  className?: string
}) {
  const id = useId()
  return (
    <section aria-labelledby={id} className={cx('mt-13 max-md:mt-10', className)}>
      <div className="mb-4 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 max-md:mb-3">
        <h3 id={id} className="text-group text-ink">
          {title}
        </h3>
        {aside !== undefined && (
          <div className="-mr-3 flex items-center gap-2 text-small text-ink-2">{aside}</div>
        )}
      </div>
      {children}
    </section>
  )
}

/**
 * Facts of a source as tiles, two side by side (one on a phone): `Fact`s inside. A fact alone on
 * its line takes the whole line, so no line ends with a hole.
 */
export function Facts({ label, children }: { label?: string; children: ReactNode }) {
  return (
    <dl aria-label={label} className="figures flex flex-wrap gap-2.5">
      {children}
    </dl>
  )
}

/**
 * One fact on a tile: the label on the left and the value on the right, or, for a long value (an
 * address, a description), the value under the label across the whole row (`wide`).
 */
export function Fact({
  label,
  children,
  wide = false,
}: {
  label: ReactNode
  children: ReactNode
  wide?: boolean
}) {
  return (
    <div
      className={cx(
        'flex min-h-[4.5rem] min-w-0 rounded-row bg-s1 px-7 py-4 max-md:min-h-16 max-md:basis-full max-md:px-5 max-md:py-3',
        wide
          ? 'basis-full flex-col justify-center gap-1'
          : 'grow basis-[calc(50%-0.3125rem)] flex-wrap items-center justify-between gap-x-6 gap-y-1',
      )}
    >
      <dt className="text-h3 text-ink max-md:text-[1.125rem]">{label}</dt>
      <dd
        className={cx(
          'min-w-0 text-lead break-words text-ink-2 max-md:text-[1.0625rem]',
          !wide && 'text-right',
        )}
      >
        {children}
      </dd>
    </div>
  )
}
