import type { AnchorHTMLAttributes } from 'react'
import { cameFromWebClient } from '@/webClientSession'

/**
 * The official Jellyfin apps for phones show jellyfin-web in a web view, and its Dashboard opens
 * the admin app in that same view. Each app puts objects of its own on every page of the view,
 * which is how the admin app knows it runs in one; neither names itself in the user agent:
 *
 * - jellyfin-android adds `window.NativeInterface` with `addJavascriptInterface` (WebViewFragment.kt
 *   at v2.7.3, lines 185 to 193);
 * - the iOS app, jellyfin-expo, runs a script at the start of every page that sets
 *   `window.ExpoAppInfo`, `window.NativeShell` and `window.ExpoRouterShim`
 *   (NativeShellWebView.tsx at v1.8.0.5, lines 42 to 79).
 */
export type JellyfinApp = 'android' | 'ios'

export function jellyfinApp(): JellyfinApp | null {
  if ('NativeInterface' in window) return 'android'
  if ('ExpoAppInfo' in window) return 'ios'
  return null
}

/** Where Polyfin serves jellyfin-web, on this same origin. */
export const webPlayerPath = '/web/'

/**
 * Whether links to the web player load it in place rather than in a new tab: in a tab that came
 * from the web player, as Jellyfin's dashboard shares jellyfin-web's tab, and in the apps, where
 * Android hands any new tab to the system browser (JellyfinWebChromeClient.kt at v2.7.3, lines 40
 * to 67).
 */
export function webPlayerInPlace(): boolean {
  return cameFromWebClient() || jellyfinApp() !== null
}

/** A link's address and target for the web player, in place or in a new tab. */
export function webPlayerLink(inPlace: boolean): AnchorHTMLAttributes<HTMLAnchorElement> {
  return inPlace
    ? { href: webPlayerPath }
    : { href: webPlayerPath, target: '_blank', rel: 'noopener' }
}
