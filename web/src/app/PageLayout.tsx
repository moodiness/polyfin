import type { ReactNode } from 'react'
import { cx, PageHeader } from '@/ui'

export type PageLayoutProps = {
  /** The h1: the same words as the page's tab or menu entry. */
  title: ReactNode
  /** Badges beside the title, outside the h1 (a source's kind). */
  titleAside?: ReactNode
  /** One or two sentences under the title. */
  lede?: ReactNode
  /** The page's main actions, on the right of the title. */
  actions?: ReactNode
  /** A link back to the list a detail page belongs to. */
  back?: { to: string; label: string }
  /**
   * A SectionNav: the page becomes a split, with the list of sections in a 216 px column and the
   * content beside it (784 px at most). On a phone the list becomes a sticky row of tabs.
   */
  nav?: ReactNode
  /** The blocks of the page (Block, Panel, RowList…), 52 px apart, rising one after the other. */
  children: ReactNode
  className?: string
}

/** 52 px between the blocks of a page (44 on a phone), none above the first. */
const blockSpacing = '[&>*+*]:mt-block max-md:[&>*+*]:mt-11'

/**
 * The frame of a page inside the shell: its header, then its blocks, rising 6 px with a fade one
 * after the other. The shell already sets the width (1232 px with 56 px margins, 20 px on a phone)
 * and the space under the TopBar; section tabs, when the route has them, sit above.
 *
 *   <PageLayout title={t.nav.health} lede={…} actions={<Button>…</Button>}>
 *     <Block title={…}>…</Block>
 *     <Block title={…}>…</Block>
 *   </PageLayout>
 */
export function PageLayout({
  title,
  titleAside,
  lede,
  actions,
  back,
  nav,
  children,
  className,
}: PageLayoutProps) {
  const header = (
    <PageHeader title={title} titleAside={titleAside} lede={lede} actions={actions} back={back} />
  )
  if (nav) {
    return (
      <div className={cx('stagger', className)}>
        {header}
        <div className="grid items-start gap-16 md:grid-cols-[216px_minmax(0,1fr)] max-lg:gap-10 max-md:block">
          {/* As tall as the content beside it, so the section nav stays in view while scrolling. */}
          <div className="self-stretch max-md:contents">{nav}</div>
          <div className={cx('max-w-[784px] min-w-0', blockSpacing)}>{children}</div>
        </div>
      </div>
    )
  }
  return (
    <div
      className={cx(
        'stagger',
        '[&>*:nth-child(n+3)]:mt-block max-md:[&>*:nth-child(n+3)]:mt-11',
        className,
      )}
    >
      {header}
      {children}
    </div>
  )
}
