import { inBrowser, type Router } from 'vitepress'

/**
 * Sends the address of a section that moved to its new page, from .vitepress/redirects.json
 * (`"old-page#anchor": "new-page#anchor"`, written by `npm run redirects`). It acts before the
 * router looks for the anchor on the page, and it replaces the old address in the history.
 * Docs pages are served at <base>docs/<page>, docs/README.md at <base>docs/ (config.mts, rewrites).
 */
type Redirects = Record<string, string>

// Loaded only once an address with an anchor needs it, in a chunk of its own.
let redirects: Promise<Redirects> | undefined

/** Where `href` goes now, or null when it did not move. */
async function movedTo(href: string, base: string): Promise<string | null> {
  const url = new URL(href, location.href)
  const docs = `${base}docs/`
  if (!url.hash || url.origin !== location.origin || !url.pathname.startsWith(docs)) return null
  const name = url.pathname
    .slice(docs.length)
    .replace(/\.html$/, '')
    .replace(/^index$/, '')
  redirects ??= import('../redirects.json').then((json) => json.default)
  const target = (await redirects)[`${name || 'README'}#${decodeURIComponent(url.hash.slice(1))}`]
  if (!target) return null
  const [page, anchor] = target.split('#')
  return `${docs}${page === 'README' ? '' : page}${url.search}#${anchor}`
}

/** Installs the redirects; resolves once the first address is settled. */
export async function redirectMovedSections(router: Router, base: string): Promise<void> {
  if (!inBrowser) return

  // In-site links: go to the new address instead, so the old one never enters the history.
  const before = router.onBeforeRouteChange
  router.onBeforeRouteChange = async (to) => {
    const target = await movedTo(to, base)
    if (target) {
      await router.go(target)
      return false
    }
    return before?.(to)
  }

  // Back and forward, and anchors changed on the same page: replace the old address.
  const replace = async () => {
    const target = await movedTo(location.href, base)
    if (!target) return
    history.replaceState(history.state, '', target)
    await router.go(target)
  }
  const after = router.onAfterRouteChange
  router.onAfterRouteChange = async (to) => {
    await after?.(to)
    await replace()
  }
  window.addEventListener('hashchange', replace)

  // First load, before the router loads the page: an anchor on the same page only needs the
  // address replaced; another page needs its own HTML, so the app never mounts this one.
  const target = await movedTo(location.href, base)
  if (!target) return
  if (new URL(target, location.href).pathname === location.pathname) {
    history.replaceState(history.state, '', target)
  } else {
    location.replace(target)
    await new Promise<never>(() => {})
  }
}
