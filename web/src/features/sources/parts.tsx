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
 * shape: light on a dark bubble, dark in a lit row, where the ink turns dark. A dark logo stays
 * readable on the dark bubble and a white one in a lit row; on a colored logo the edge is a
 * hairline. Logos are third-party images without CORS, so their colors cannot be read to choose.
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

/** The source's kind icon in a round bubble (36 px in the list), or its logo alone once loaded. */
export function SourceTile({ addon, selected = false }: { addon: Addon; selected?: boolean }) {
  const logo = useLogo(addon)
  return (
    <span className="relative inline-grid shrink-0 self-start">
      <IconTile icon={logo.loaded ? undefined : kindIcon(addon)} size="sm" selected={selected} />
      <Logo logo={logo} className="size-9 rounded-full p-1" />
    </span>
  )
}

/** The big mark at the head of a source's page: its kind icon (72 px, 56 on a phone), or logo. */
export function SourceMark({ addon }: { addon: Addon }) {
  const logo = useLogo(addon)
  const Glyph = kindIcon(addon)
  return (
    <span
      aria-hidden="true"
      className="relative grid size-[4.5rem] shrink-0 place-items-center rounded-row bg-s2 text-ink-2 max-md:size-14 max-md:rounded-[1.125rem]"
    >
      {!logo.loaded && <Glyph size="2.125rem" className="max-md:size-7" />}
      <Logo logo={logo} className="size-full rounded-[inherit] p-2" />
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
