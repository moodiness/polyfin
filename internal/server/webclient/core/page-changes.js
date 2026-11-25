// Polyfin (MIT License). The web player script, served at /web/polyfin.js
// and loaded by the index.html of jellyfin-web, which Polyfin ships
// unmodified. It is assembled from the files of this folder, a file for
// each of its 24 parts, numbered, and the core they share, in the order of
// webScriptGroups (internal/server/webclientminify_test.go); this file runs
// first. Each part's file tells what it does, and what it relies on in
// jellyfin-web, its anchors (see docs/development.md).
//
// It is written in ES5 and runs before jellyfin-web's scripts and their
// polyfills, so that it works on every browser jellyfin-web 12.4 is built
// for, down to Chrome 27: it uses no syntax newer than ES5 (a test checks
// it), writes addresses by hand rather than with URL or URLSearchParams,
// and falls back where a later API (WeakMap, WeakSet, isConnected) is
// missing.
//
// Seven parts follow the whole page as jellyfin-web renders it, through one
// observer of the body that serves them all, rather than one each: it
// gathers the page's changes until the browser next draws the page, then
// tells them once, as a "polyfinpagechange" event on the document.
// jellyfin-web changes the page many times as it renders a route; each
// part then looks at the page once a frame, before it is drawn, and only
// while it listens: the menus' entry (the eighth) and the home page's rows
// (the eleventh) always, the play buttons (the third) on a title's page
// while they are or may be held, the reminders (the thirteenth) on a title's
// page, which may be a programme's, the recent searches (the fifteenth)
// on the search page, the version badges (the twenty-first) on a title's
// page once its badges came, and while a video plays, and the type filter
// (the twenty-fourth) on a collection's page once jellyfin-web listed it.
// The other parts that follow the page watch less of it: a title's page, a
// menu, or the body's own children.
//
// Anchors in jellyfin-web: none.

  if (typeof MutationObserver !== 'function') return
  // Whether the page changed since the parts were last told.
  var due = false
  function tell() {
    due = false
    document.dispatchEvent(new Event('polyfinpagechange'))
  }
  // A browser without requestAnimationFrame tells at once.
  function changed() {
    if (due) return
    if (typeof requestAnimationFrame !== 'function') return tell()
    due = true
    requestAnimationFrame(tell)
  }
  // The script runs in the page's head, before the body exists.
  function watch() {
    new MutationObserver(changed).observe(document.body, { childList: true, subtree: true })
  }
  if (document.body) watch()
  else addEventListener('DOMContentLoaded', watch)
