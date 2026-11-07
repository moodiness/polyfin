import { createContext, use, useLayoutEffect, useState, type ReactNode } from 'react'
import { cx, DockSlotContext, PageHeader } from '@/ui'

export type PageLayoutProps = (
  | {
      /** The h1: the same words as the page's area word, sub-area word or menu entry. */
      title: ReactNode
      head?: never
    }
  | {
      /**
       * The page's own head instead of the title row: the band of a person's or a group's page,
       * which holds the h1, the link back and the actions. Used with `nav`; the props of the title
       * row (`titleAside`, `lede`, `actions`, `back`, `greeting`) are then left out.
       */
      head: ReactNode
      title?: never
    }
) & {
  /** Badges beside the title, outside the h1 (a source's kind). */
  titleAside?: ReactNode
  /** One or two sentences under the title. */
  lede?: ReactNode
  /** The page's main actions, on the right of the title. */
  actions?: ReactNode
  /** A link back to the list a detail page belongs to. Such a page shows no sub-area words. */
  back?: { to: string; label: string }
  /** The title at the display size of a greeting ("Good afternoon, Morgan"), for Home. */
  greeting?: boolean
  /**
   * A SectionNav: the page becomes a split, with the section list in a 300 px column and the
   * content beside it. The unsaved dock of a SaveBar docks under the list. On a phone the list
   * becomes a strip that scrolls sideways, and the dock a bar along the bottom.
   */
  nav?: ReactNode
  /** The blocks of the page (Block, Panel, RowList…), 64 px apart, rising one after the other. */
  children: ReactNode
  className?: string
}

/**
 * The sub-area words of the area a page belongs to (Content's, System's), which `SectionTabs`
 * hands to the page's PageLayout to show under the title. `claim` tells it that a page does, so
 * that it shows them itself above a page without one (a loading state).
 */
export const SubAreaContext = createContext<{
  words: ReactNode
  claim: () => () => void
} | null>(null)

/** Set inside a PageLayout: a PageLayout nested in it is a part of the page, not the page. */
const InsidePage = createContext(false)

/** 64 px between the blocks of a page (44 on a phone), none above the first. */
const blockSpacing = '[&>*+*]:mt-block'

/**
 * The frame of a page inside the shell: its head (the title, the lede, the actions, and the area's
 * sub-area words under them), then its blocks, rising with a fade one after the other. The shell
 * already sets the width (1280 px with 80 px margins, 56 under 1400 px and 20 on a phone) and the
 * space under the top row.
 *
 *   <PageLayout title={t.nav.health} lede={…} actions={<Button>…</Button>}>
 *     <Block title={…}>…</Block>
 *     <Block title={…}>…</Block>
 *   </PageLayout>
 */
export function PageLayout({
  title,
  head,
  titleAside,
  lede,
  actions,
  back,
  greeting,
  nav,
  children,
  className,
}: PageLayoutProps) {
  const nested = use(InsidePage)
  const subArea = nested ? null : use(SubAreaContext)
  const claim = subArea?.claim
  useLayoutEffect(() => claim?.(), [claim])
  const [dock, setDock] = useState<HTMLElement | null>(null)

  const header = head ?? (
    <PageHeader
      title={title}
      titleAside={titleAside}
      lede={lede}
      actions={actions}
      back={back}
      greeting={greeting}
      nav={back === undefined ? subArea?.words : undefined}
    />
  )
  if (nav) {
    return (
      <InsidePage value>
        <DockSlotContext value={dock}>
          <div className={cx('stagger', className)}>
            {header}
            <div className="grid items-start gap-[4.5rem] md:grid-cols-[18.75rem_minmax(0,1fr)] max-xl:gap-12 max-lg:md:grid-cols-[14.5rem_minmax(0,1fr)] max-lg:gap-8 max-md:block">
              {/* As tall as the content beside it: the list stays in view while scrolling, and
                  the dock sits at the foot of the column, over the bottom of the screen. The list
                  sticks inside its own part, which ends above the dock's place, so at the end of
                  the page it rises clear of the dock. The place takes no clicks, the dock only
                  while it shows, and without a dock (a page with no SaveBar) it takes no room. */}
              <div className="flex flex-col self-stretch max-md:contents">
                <div className="flex-1 max-md:contents">{nav}</div>
                <div
                  ref={setDock}
                  className="pointer-events-none sticky bottom-7 mt-6 empty:hidden max-md:hidden"
                />
              </div>
              {/* A band has no space under it for the strip of a phone: the content keeps 24 px. */}
              <div className={cx('min-w-0', blockSpacing, head !== undefined && 'max-md:mt-6')}>
                {children}
              </div>
            </div>
          </div>
        </DockSlotContext>
      </InsidePage>
    )
  }
  return (
    <InsidePage value>
      <div className={cx('stagger', '[&>*:nth-child(n+3)]:mt-block', className)}>
        {header}
        {children}
      </div>
    </InsidePage>
  )
}
