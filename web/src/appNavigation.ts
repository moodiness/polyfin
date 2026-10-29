import { jellyfinApp, webPlayerPath } from '@/jellyfinApps'
import { cameFromWebClient } from '@/webClientSession'

/** The objects the apps call on the page, as jellyfin-web and the apps' own scripts define them. */
type AppPage = {
  /** Android's Back: `window.NavigationHelper.goBack();` (WebappFunctionChannel.kt, line 25). */
  NavigationHelper?: { goBack: () => void }
  /**
   * jellyfin-web's router, which the iOS app's ExpoRouterShim calls: `goHome()` when the Home tab
   * is pressed again, `canGoBack()` and `back()` on Back (ExpoRouterShim.staticjs, lines 6 to 31).
   */
  Emby?: { Page: { canGoBack: () => boolean; back: () => void; goHome: () => void } }
}

/**
 * Answers the apps' Back and Home on the admin app's pages, where jellyfin-web, which answers them
 * on its own, is not loaded: without it, Android's Back did nothing and the iOS Home tab neither.
 * Back closes the dialog on top, else goes back among the admin app's pages (with `navigate(-1)`,
 * which asks first about unsaved changes), else returns to the web player. Nothing is set outside
 * the apps, or where an app's object already exists.
 */
export function answerAppNavigation(navigate: (delta: number) => unknown) {
  const app = jellyfinApp()
  if (app === null) return
  const page = window as Window & AppPage
  const back = () => {
    const dialogs = document.querySelectorAll('dialog[open]')
    const top = dialogs[dialogs.length - 1]
    if (top) {
      // As Escape does: each dialog of the app closes itself on cancel.
      top.dispatchEvent(new Event('cancel', { cancelable: true }))
      return
    }
    // React Router counts the entries of the admin app from 0, the page it was opened on.
    const state: unknown = window.history.state
    const index =
      state !== null && typeof state === 'object' && 'idx' in state && typeof state.idx === 'number'
        ? state.idx
        : 0
    if (index > 0) {
      void navigate(-1)
      return
    }
    // The page before is the web player's that opened the admin app: its Dashboard replaced its
    // own entry with the admin app, and Share or a reminder's link added one after it.
    if (cameFromWebClient() && window.history.length > 1) window.history.back()
    else window.location.assign(webPlayerPath)
  }
  if (app === 'android' && page.NavigationHelper === undefined) {
    page.NavigationHelper = { goBack: back }
  }
  if (app === 'ios' && page.Emby === undefined) {
    page.Emby = {
      Page: { canGoBack: () => true, back, goHome: () => window.location.assign(webPlayerPath) },
    }
  }
}
