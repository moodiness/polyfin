import { FilmStripIcon } from '@phosphor-icons/react'
import {
  createContext,
  use,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react'
import { cx } from './cx'

type ShelfContextValue = {
  /** The id of the lit tile, or null before any was lit (the first one is wide then). */
  lit: string | null
  light: (id: string) => void
}

const ShelfContext = createContext<ShelfContextValue | null>(null)

/**
 * A row of ArtTiles (Playing now). The lit tile, under the keyboard or the mouse, opens wide (7
 * to 5 beside another) and the others narrow; before any is lit, the first is wide. Tiles wrap
 * under each other when there is no room, and stack on a phone. The arrows move between tiles.
 */
export function ArtShelf({
  label,
  children,
  className,
}: {
  /** The accessible name, when no heading names the shelf. */
  label?: string
  children: ReactNode
  className?: string
}) {
  const [lit, setLit] = useState<string | null>(null)
  return (
    <ShelfContext value={{ lit, light: setLit }}>
      <ul
        aria-label={label}
        // Tiles keep their own height: one whose details open does not stretch its neighbour.
        className={cx(
          'flex flex-wrap items-start gap-6 max-md:flex-col max-md:items-stretch max-md:gap-4',
          className,
        )}
      >
        {children}
      </ul>
    </ShelfContext>
  )
}

/** Whether the mouse may light a tile: a wide screen with a pointer that hovers. */
const hoverLights = '(min-width: 48rem) and (hover: hover)'

/**
 * A poster tile for what is playing: the poster on a blurred, darkened backdrop of itself under a
 * scrim, then who watches, the title, how it plays, and a foot with progress and rates. In an
 * ArtShelf the lit tile opens wide and gets a white halo; its `tools` (icon buttons) and its
 * `detail` show only then. Images load lazily.
 *
 *   <ArtShelf label={t.home.playingNow}>
 *     <ArtTile poster={url} title="Amber Road" top={…} meta="Movie · 2023"
 *       tools={<IconButton …/>} footer={<ProgressBar variant="playback" …/>}>…</ArtTile>
 *   </ArtShelf>
 */
export function ArtTile({
  title,
  titleAs: Title = 'h3',
  poster,
  fit = 'cover',
  fallback,
  backdrop,
  top,
  meta,
  children,
  detail,
  footer,
  tools,
  onLight,
  className,
}: {
  /** The title of what plays. */
  title: ReactNode
  /** Heading level, so pages keep a correct outline. */
  titleAs?: 'h2' | 'h3'
  /** The poster's address; without it and without `fallback`, the tile has no poster column. */
  poster?: string
  /**
   * `cover` fills the poster's frame, cropping it; `contain` shows the whole art on a dark fill,
   * for channel logos and square art.
   */
  fit?: 'cover' | 'contain'
  /**
   * What the poster's frame shows when there is no poster or it fails to load: an initial or a
   * Phosphor icon. A film strip by default when a poster failed.
   */
  fallback?: ReactNode
  /** The backdrop's address; the poster by default. */
  backdrop?: string
  /** The line above the title: who watches, on which device. */
  top?: ReactNode
  /** The line under the title: "Movie · 2023". */
  meta?: ReactNode
  /** Under the meta line: how it plays, the app, the source. */
  children?: ReactNode
  /** More, shown only while the tile is wide. */
  detail?: ReactNode
  /** Across the bottom: progress, times, rates. */
  footer?: ReactNode
  /** Icon buttons at the top right (Details, message, stop), shown while the tile is lit. */
  tools?: ReactNode
  /** Called when the tile lights up, for a page that takes its color (Home's room light). */
  onLight?: () => void
  className?: string
}) {
  const shelf = use(ShelfContext)
  const id = useId()
  const titleId = `${id}-title`
  const item = useRef<HTMLLIElement>(null)
  const [first, setFirst] = useState(false)
  useLayoutEffect(() => {
    setFirst(item.current?.parentElement?.firstElementChild === item.current)
  })
  const lit = shelf?.lit === id
  const wide = shelf === null || lit || (shelf.lit === null && first)
  // The addresses that failed to load; a new address is tried again.
  const [failed, setFailed] = useState<ReadonlySet<string>>(() => new Set())
  const posterShown = poster !== undefined && !failed.has(poster) ? poster : undefined
  const backdropSrc = backdrop ?? poster
  const art = backdropSrc !== undefined && !failed.has(backdropSrc) ? backdropSrc : undefined
  const frame = poster !== undefined || fallback !== undefined
  const frameClass = cx(
    'aspect-[400/560] rounded-[18px] shadow-[0_24px_48px_-16px_rgb(0_0_0/0.6),0_0_0_1px_rgb(255_255_255/0.08)] transition-[width] duration-700 ease-nuit motion-reduce:transition-none max-md:w-24 max-md:rounded-[14px]',
    wide ? 'w-[clamp(150px,22vh,204px)]' : 'w-[clamp(108px,15.5vh,140px)]',
  )

  function light() {
    if (lit) return
    shelf?.light(id)
    onLight?.()
  }

  function onKeyDown(event: KeyboardEvent<HTMLElement>) {
    if (event.target !== event.currentTarget) return
    if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return
    const sibling =
      event.key === 'ArrowRight'
        ? item.current?.nextElementSibling
        : item.current?.previousElementSibling
    const next = sibling?.querySelector<HTMLElement>('[data-art-tile]')
    if (!next) return
    event.preventDefault()
    next.focus()
  }

  return (
    <li
      ref={item}
      className={cx(
        'min-w-[min(100%,380px)] transition-[flex-grow] duration-700 ease-nuit motion-reduce:transition-none max-md:min-w-0',
        wide ? 'grow-7 basis-0' : 'grow-5 basis-0',
      )}
    >
      <article
        data-art-tile
        tabIndex={0}
        aria-labelledby={titleId}
        onFocus={light}
        onMouseEnter={() => {
          if (matchMedia(hoverLights).matches) light()
        }}
        onKeyDown={onKeyDown}
        className={cx(
          'group/art relative isolate grid h-full cursor-default grid-rows-[1fr_auto] gap-x-[30px] gap-y-5 overflow-hidden rounded-tile bg-s1-solid px-8 pt-[26px] pb-6',
          'outline-4 outline-offset-[6px] outline-solid',
          'transition-[scale,box-shadow,outline-color] duration-500 ease-nuit',
          'after:absolute after:inset-0 after:-z-1 after:bg-[linear-gradient(100deg,rgb(22_20_27/0.35)_0%,rgb(22_20_27/0.72)_42%,rgb(22_20_27/0.82)_100%)]',
          'max-md:gap-x-[18px] max-md:gap-y-4 max-md:rounded-[28px] max-md:p-[18px] max-md:outline-offset-4',
          frame ? 'grid-cols-[auto_minmax(0,1fr)]' : 'grid-cols-[minmax(0,1fr)]',
          lit
            ? 'outline-focus shadow-[0_46px_90px_-36px_rgb(0_0_0/0.75)] md:motion-safe:scale-[1.015]'
            : 'outline-transparent md:motion-safe:hover:scale-[1.008]',
          className,
        )}
      >
        {art && (
          <img
            src={art}
            alt=""
            aria-hidden="true"
            loading="lazy"
            decoding="async"
            onError={() => setFailed((old) => new Set(old).add(art))}
            className="absolute -inset-[60px] -z-2 size-[calc(100%+120px)] max-w-none scale-110 object-cover brightness-[0.62] saturate-[1.25] blur-[46px]"
          />
        )}
        {posterShown !== undefined ? (
          <img
            src={posterShown}
            alt=""
            loading="lazy"
            decoding="async"
            onError={() => setFailed((old) => new Set(old).add(posterShown))}
            className={cx(
              frameClass,
              fit === 'cover' ? 'object-cover' : 'bg-bg object-contain p-[8%]',
            )}
          />
        ) : (
          frame && (
            <div
              aria-hidden="true"
              className={cx(
                frameClass,
                'grid place-items-center bg-s2 text-[44px] font-bold text-ink-2 max-md:text-[32px]',
              )}
            >
              {fallback ?? <FilmStripIcon size="1em" />}
            </div>
          )
        )}
        <div className="flex min-w-0 flex-col">
          {top !== undefined && (
            <div
              className={cx(
                'flex min-w-0 items-center gap-2.5 text-[18px] text-ink-2 max-md:text-micro',
                tools !== undefined && wide && 'md:pr-40',
              )}
            >
              {top}
            </div>
          )}
          <Title
            id={titleId}
            className={cx(
              'mt-3 leading-[1.05] font-bold tracking-[-0.02em] break-words text-ink max-md:mt-2 max-md:text-[26px]',
              wide ? 'text-[42px]' : 'text-[34px]',
            )}
          >
            {title}
          </Title>
          {meta !== undefined && (
            <p className="mt-1.5 text-[18px] text-ink-2 max-md:text-micro">{meta}</p>
          )}
          {children !== undefined && (
            <div className="mt-4 text-[17px] text-ink-2 max-md:mt-2.5 max-md:text-micro">
              {children}
            </div>
          )}
          {detail !== undefined && wide && (
            <div className="mt-3 text-[17px] text-ink-2 max-md:text-micro">{detail}</div>
          )}
        </div>
        {footer !== undefined && <div className="col-span-full min-w-0">{footer}</div>}
        {tools !== undefined && (
          <div
            className={cx(
              'flex gap-2 transition-opacity duration-300 ease-nuit md:absolute md:top-[22px] md:right-[22px] max-md:col-span-full max-md:opacity-100',
              lit ? 'opacity-100' : 'opacity-0 group-focus-within/art:opacity-100',
            )}
          >
            {tools}
          </div>
        )}
      </article>
    </li>
  )
}
