import { useId, useState, type ReactNode } from 'react'
import type { Addon } from '@/api'
import { useI18n } from '@/i18n'
import { cx, IconTile, StatusPill } from '@/ui'
import { folderFailure, kindIcon } from './model'

type LogoState = 'loading' | 'loaded' | 'failed'

/** A logo to try (`src`, null once failed or when there is none), whether it has loaded, and how to report it. */
interface SourceLogo {
  src: string | null
  loaded: boolean
  set: (next: LogoState) => void
}

/**
 * The source's logo and how far it has loaded, shared by the image and the bubble it sits in, so
 * the kind icon shows only while there is no logo: none set, still loading, broken or blocked by
 * the page's security policy. Logos are third-party images, so only `https://` ones are tried.
 */
function useLogo(addon: Addon): SourceLogo {
  const [state, setState] = useState<{ src: string; value: LogoState } | null>(null)
  const url = addon.logo !== null && addon.logo.startsWith('https://') ? addon.logo : null
  const value: LogoState = url !== null && state?.src === url ? state.value : 'loading'
  return {
    src: url !== null && value !== 'failed' ? url : null,
    loaded: url !== null && value === 'loaded',
    set: (next: LogoState) => url !== null && setState({ src: url, value: next }),
  }
}

/**
 * The logo image, laid over its bubble and hidden until it loads. The bubble's fill shows through
 * its transparent parts, so the logo wears a thin edge of the ink color that follows its own
 * shape: a dark logo stays readable on the dark bubble, a lit row's included, and on a light or
 * colored logo the edge is a hairline. Logos are third-party images without CORS, so their colors
 * cannot be read to choose.
 */
function Logo({ logo, className }: { logo: SourceLogo; className: string }) {
  if (logo.src === null) return null
  return (
    <img
      key={logo.src}
      src={logo.src}
      alt=""
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onLoad={() => logo.set('loaded')}
      onError={() => logo.set('failed')}
      className={cx(
        'absolute inset-0 object-contain',
        '[filter:drop-shadow(0_0_0.5px_color-mix(in_srgb,var(--color-ink)_90%,transparent))_drop-shadow(0_0_0.5px_color-mix(in_srgb,var(--color-ink)_90%,transparent))]',
        className,
        logo.loaded ? 'opacity-100' : 'opacity-0',
      )}
    />
  )
}

/** The source's kind icon in a round bubble (30 px in the list), or its logo alone once loaded. */
export function SourceTile({ addon, selected = false }: { addon: Addon; selected?: boolean }) {
  const logo = useLogo(addon)
  return (
    <span className="relative inline-grid shrink-0">
      <IconTile icon={logo.loaded ? undefined : kindIcon(addon)} size="sm" selected={selected} />
      <Logo logo={logo} className="size-7.5 rounded-full p-0.5" />
    </span>
  )
}

/** The mark at the head of a source's page: its kind icon (48 px, 44 on a phone), or logo. */
export function SourceMark({ addon }: { addon: Addon }) {
  const logo = useLogo(addon)
  const Glyph = kindIcon(addon)
  return (
    <span
      aria-hidden="true"
      className="relative grid size-12 shrink-0 place-items-center rounded-row bg-s2 text-ink-2 max-md:size-11"
    >
      {!logo.loaded && <Glyph size="1.5rem" />}
      <Logo logo={logo} className="size-full rounded-[inherit] p-1.5" />
    </span>
  )
}

/**
 * Whether a source works: on, off, its last IPTV download failed, or its local folder cannot be
 * read, as when its SFTP server offers another host key, or is being scanned. Always an icon and a
 * word. `quiet` says nothing while it works or is off, beside a switch that says it already.
 */
export function SourceStatus({
  addon,
  quiet = false,
  className,
}: {
  addon: Addon
  quiet?: boolean
  className?: string
}) {
  const { t } = useI18n()
  if (!addon.enabled)
    return quiet ? null : (
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
  if (quiet) return null
  return (
    <StatusPill tone="ok" className={className}>
      {t.sources.active}
    </StatusPill>
  )
}

/**
 * A group of a source's page: its title (18 px) with an aside on the right (a count, a link), then
 * its rows or tiles. Groups are 28 px apart.
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
    <section aria-labelledby={id} className={cx('mt-7 max-md:mt-6', className)}>
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
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
    <dl aria-label={label} className="figures flex flex-wrap gap-1.5">
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
        'flex min-h-12 min-w-0 rounded-row bg-s1 px-4 py-2.5 max-md:basis-full',
        wide
          ? 'basis-full flex-col justify-center gap-1'
          : 'grow basis-[calc(50%-0.1875rem)] flex-wrap items-center justify-between gap-x-5 gap-y-0.5',
      )}
    >
      <dt className="text-h3 text-ink">{label}</dt>
      <dd className={cx('min-w-0 text-control break-words text-ink-2', !wide && 'text-right')}>
        {children}
      </dd>
    </div>
  )
}
