// Part 5: a library's row opens the library on its chosen screen.
//
// The title of a collection library's or a catalog library's
// "Recently Added" row on the home page opens the library on the screen its
// user chose (Settings › Home › Default screen), as the library's links in
// the header and the menu do. jellyfin-web opens a library's Suggestions
// from that title (tab=1). It calls a library without a CollectionType
// mixed (#/mixed?…), and a collection library has no suggestions of its
// own. A catalog library lists an addon's catalog whose titles Polyfin does
// not store: its row is the catalog's first titles, not recent additions,
// so its Suggestions page is empty or repeats the row. The views
// /Polyfin/UserViews marks Catalog (see the seventh) tell those libraries'
// #/movies? and #/tv? links. The script drops the tab from such a link as
// it is pressed or clicked, before jellyfin-web follows it: pressed covers a
// link opened in a new tab, clicked one followed with the keyboard.
//
// Anchors in jellyfin-web:
//   route: mixed
//   route: movies
//   route: tv

  // The link pressed or clicked, if it opens a collection library or a
  // catalog library on Suggestions, without the tab: the library opens on
  // its chosen screen.
  function chosenScreen(event) {
    var link = event.target && event.target.closest && event.target.closest('a[href]')
    var href = link ? link.getAttribute('href') : ''
    if (!/^#!?\/mixed\?/i.test(href) && !(/^#!?\/(movies|tv)\?/i.test(href) && catalogView(href))) return
    var chosen = href.replace(/([?&])tab=1(&|$)/, function (match, before, after) {
      return after ? before : ''
    })
    if (chosen !== href) link.setAttribute('href', chosen)
  }
  // catalogView reports whether the topParentId of href is a view that the
  // answer kept for the user signed in marks Catalog.
  function catalogView(href) {
    var found = /[?&]topparentid=([^&]*)/i.exec(href)
    var saved = savedViews()
    if (!found || !saved || !hiddenFor || saved.user !== hiddenFor) return false
    var id = decodeURIComponent(found[1]).replace(/-/g, '').toLowerCase()
    return saved.items.some(function (view) {
      return !!view && view.Catalog === true && String(view.Id).replace(/-/g, '').toLowerCase() === id
    })
  }
  // Both run before jellyfin-web follows the link.
  addEventListener('mousedown', quietly(chosenScreen), true)
  addEventListener('click', quietly(chosenScreen), true)
