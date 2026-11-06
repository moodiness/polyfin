/**
 * The size the admin app shows in, a choice of My account (see `html` in index.css): Compact, the
 * default, has screens from 48rem up 15 % denser than a phone; Large keeps a phone's sizes on
 * every screen. A phone shows both alike.
 */
export type InterfaceSize = 'compact' | 'large'

const storageKey = 'polyfin.interfaceSize'

function show(size: InterfaceSize) {
  document.documentElement.dataset.interfaceSize = size
}

/**
 * Shows the app in the size last used on this browser, Compact without one. main.tsx calls it
 * before React draws anything, so that the first frame has the size the session then confirms;
 * the pages signed out keep it too. The admin app's security policy refuses inline scripts, so
 * no earlier script in index.html can do it.
 */
export function showKeptInterfaceSize() {
  let kept: string | null = null
  try {
    kept = window.localStorage.getItem(storageKey)
  } catch {
    // Storage can be disabled by the browser: Compact it is, until the session says otherwise.
  }
  show(kept === 'large' ? 'large' : 'compact')
}

/** Shows the app in the account's size, and keeps it on this browser for the next page. */
export function keepInterfaceSize(size: InterfaceSize) {
  show(size)
  try {
    window.localStorage.setItem(storageKey, size)
  } catch {
    // The size still applies to this page when storage is unavailable.
  }
}
