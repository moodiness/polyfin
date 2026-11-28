import {
  ArrowClockwiseIcon,
  CaretRightIcon,
  CheckIcon,
  CopyIcon,
  HouseIcon,
} from '@phosphor-icons/react'
import { use, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { isRouteErrorResponse, useRouteError } from 'react-router'
import { queryClient, queryKeys, type Status } from '@/api'
import { useShownLanguage, type UpFrontMessages } from '@/i18n'
import { Button, ButtonLink, cx, ExternalButtonLink, rem } from '@/ui'
import { SubAreaContext } from './PageLayout'
import { pageFrame } from './shellParts'

type Words = UpFrontMessages['appError']

/**
 * The words when no language is shown, as when the language's own file could not load: English,
 * as in `en/common.ts`.
 */
const fallback: { words: Words; colon: string } = {
  words: {
    title: 'This page could not be shown',
    description: 'Polyfin ran into an error while showing it. Reload to try again.',
    updatedTitle: 'Polyfin was updated',
    updatedDescription:
      'This tab still runs the version from before the update. Reload to use the new one.',
    reload: 'Reload',
    home: 'Back to the home page',
    details: 'Details for a bug report',
    copy: 'Copy the details',
    copied: 'Details copied.',
    copyFailed: 'The details could not be copied. Select them and copy them by hand.',
    version: 'Version',
    address: 'Address',
    browser: 'Browser',
  },
  colon: ':',
}

/**
 * A file of the app that did not load: Chrome's, Firefox's and Safari's words for a page's file,
 * and Vite's for its styles. After an update, a tab still open asks for its version's files,
 * which are gone.
 */
const missingFile =
  /dynamically imported module|Importing a module script failed|Unable to preload CSS/i

/** The error as a bug report wants it: its name and message, then its stack. */
function describe(error: unknown): string {
  if (isRouteErrorResponse(error)) return `${error.status} ${error.statusText}`.trim()
  if (!(error instanceof Error)) return String(error)
  const head = `${error.name}: ${error.message}`
  const stack = error.stack ?? ''
  // Chrome's stack starts with the name and message; Firefox's and Safari's hold the frames only.
  if (stack.startsWith(head)) return stack
  return stack === '' ? head : `${head}\n${stack}`
}

/**
 * What a bug report needs: Polyfin's version, from the status the shell reads (absent when it
 * never answered), the page's address, the browser, and the error. The address leaves out its
 * query, which may hold a code a sign-in sent back.
 */
function report(error: unknown, words: Words, colon: string): string {
  const version = queryClient.getQueryData<Status>(queryKeys.status)?.version
  return [
    ...(version ? [`${words.version}${colon} ${version}`] : []),
    `${words.address}${colon} ${window.location.pathname}`,
    `${words.browser}${colon} ${navigator.userAgent}`,
    '',
    describe(error),
  ].join('\n')
}

/**
 * The details, folded under a quiet button: the report, and a button that copies it. Where the
 * browser refuses the clipboard (plain http on a local network), the report is selected instead.
 */
function Details({ error, words, colon }: { error: unknown; words: Words; colon: string }) {
  const [open, setOpen] = useState(false)
  const [copy, setCopy] = useState<'idle' | 'copied' | 'failed'>('idle')
  const text = useRef<HTMLPreElement>(null)
  const id = useId()
  const content = report(error, words, colon)

  async function copyReport() {
    try {
      await navigator.clipboard.writeText(content)
      setCopy('copied')
    } catch {
      setCopy('failed')
      if (text.current) window.getSelection()?.selectAllChildren(text.current)
    }
  }

  return (
    <div className="mt-6">
      <Button
        variant="ghost"
        icon={CaretRightIcon}
        aria-expanded={open}
        aria-controls={id}
        onClick={() => setOpen(!open)}
        className="-ml-3.5 [&>svg]:transition-transform [&>svg]:duration-200 [&>svg]:ease-nuit aria-expanded:[&>svg]:rotate-90"
      >
        {words.details}
      </Button>
      <div id={id} hidden={!open} className="mt-3 max-w-[60rem] rounded-panel bg-s1 p-5 max-sm:p-4">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <Button
            size="sm"
            icon={copy === 'copied' ? CheckIcon : CopyIcon}
            onClick={() => void copyReport()}
          >
            {words.copy}
          </Button>
          <p role="status" className="text-small text-ink-2">
            {copy === 'copied' ? words.copied : copy === 'failed' ? words.copyFailed : ''}
          </p>
        </div>
        <pre
          ref={text}
          className="mt-4 font-mono text-micro leading-relaxed [overflow-wrap:anywhere] whitespace-pre-wrap text-ink-2"
        >
          {content}
        </pre>
      </div>
    </div>
  )
}

/**
 * What happened in one sentence, Reload, the way home, and the details folded. A file of the app
 * gone after an update says so, without the error's words. `home` is the link to Home. The head
 * is a page's (PageHeader's title and lede), drawn here: the lede's More reads the messages, which
 * may be missing. In an area (Content, System), the area's sub-area words show under it, as on
 * its other pages, so that they lead to them; elsewhere, the head sits lower, as a dead end's.
 */
function ErrorView({ error, home }: { error: unknown; home: (words: Words) => ReactNode }) {
  const shown = useShownLanguage()
  const subArea = use(SubAreaContext)
  const claim = subArea?.claim
  useLayoutEffect(() => claim?.(), [claim])
  const words = shown?.t.appError ?? fallback.words
  const colon = shown?.t.common.colon ?? fallback.colon
  const isUpdate = error instanceof Error && missingFile.test(error.message)
  return (
    <div
      data-error-page={isUpdate ? 'updated' : 'error'}
      className={cx('animate-fade', !subArea && 'pt-10 max-md:pt-4')}
    >
      <header className="mb-7 max-md:mb-6">
        <h1 className="hyphenate text-h1 text-ink">
          {isUpdate ? words.updatedTitle : words.title}
        </h1>
        <p className="mt-1.5 max-w-measure text-lead text-ink-2">
          {isUpdate ? words.updatedDescription : words.description}
        </p>
        {subArea && <div className="mt-3">{subArea.words}</div>}
      </header>
      <div className="flex flex-wrap gap-3">
        <Button
          variant="primary"
          icon={ArrowClockwiseIcon}
          onClick={() => window.location.reload()}
        >
          {words.reload}
        </Button>
        {home(words)}
      </div>
      <Details error={error} words={words} colon={colon} />
    </div>
  )
}

/**
 * In place of a page that failed to draw, or whose file did not load: the shell stays, so the
 * top bar and the menu still lead elsewhere, and leaving the page clears the error.
 */
export function PageError() {
  const error = useRouteError()
  return (
    <ErrorView
      error={error}
      home={(words) => (
        <ButtonLink to="/" icon={HouseIcon}>
          {words.home}
        </ButtonLink>
      )}
    />
  )
}

/**
 * In place of the whole app, when its frame failed: the logo and name, as signed out, and the
 * error, with nothing else of the app's, and a plain link home, which loads the app anew. It also
 * draws outside the router and without a language, when the language's file did not load
 * (`main.tsx`).
 */
export function AppErrorScreen({ error }: { error: unknown }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <header
        className={cx(
          pageFrame,
          'flex h-[6.25rem] items-center pt-[1.375rem] max-md:h-[4.75rem] max-md:pt-3',
        )}
      >
        <span className="flex shrink-0 items-center gap-3.5 text-[1.5rem] font-[650] tracking-[-0.01em] text-ink">
          <img
            src={`${import.meta.env.BASE_URL}polyfin.svg`}
            alt=""
            width={40}
            height={40}
            className="shrink-0"
            style={{ width: rem(40), height: rem(40) }}
          />
          <span className="max-md:sr-only">Polyfin</span>
        </span>
      </header>
      <main id="main" className="mx-auto w-full max-w-[880px] flex-1 px-5 pt-5 pb-20 max-md:pt-7">
        <ErrorView
          error={error}
          home={(words) => (
            <ExternalButtonLink href={import.meta.env.BASE_URL} icon={HouseIcon}>
              {words.home}
            </ExternalButtonLink>
          )}
        />
      </main>
    </div>
  )
}

/** The router's error in place of the whole app. */
export function AppError() {
  return <AppErrorScreen error={useRouteError()} />
}
