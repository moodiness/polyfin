// Part 22: the place of the home page and of search results on Back.
//
// It brings back the place of jellyfin-web's home page
// (#/home, its Home tab) and of search results (#/search?…) on Back, in
// the modern and mobile layouts. There, jellyfin-web 12.4 builds the page
// anew on Back and scrolls it to the top while it is still short, its
// rows coming a moment later, so that the browser's own restore has
// nothing to land on. The script keeps one place for each entry of the
// history, by the key jellyfin-web's router gives it (history.state.key,
// "first" for the first entry of a load, which has none), as the route
// leaves the page: the scroll, the row at the top of the screen and how
// far it is from the top, and how far each row is scrolled sideways, as
// jellyfin-web's scroller tells it (getScrollPosition of .emby-scroller:
// on a desktop, rows move by a transform rather than scrollLeft). A row
// is known by its data-polyfin-row (the eleventh's rows), else its
// .sectionN class, else its title. On a popstate (Back or Forward) to
// such an entry, the page shows its top while it fills; then, looking
// once a frame, as titles and pictures widen rows without changing the
// page, and as soon as the page or a row scrolls, once the page is tall
// enough, the script brings the row back to its height, or the scroll
// itself while the row is not there, and scrolls each row sideways back
// through the scroller (scrollToPosition), again whenever jellyfin-web's
// scroller goes back to its start, as it does when a row's size changes.
// It follows the row while the rows above it fill, going from the scroll
// to the row at most once and never back, and stops a second after the
// page first reached its place, 3 seconds after Back at most, or at once
// at any wheel, touch, key or click, so that a user who scrolls is never
// pulled back. The place of an entry left while it is still being
// brought back stays as it was kept. Where jellyfin-web
// shows the page it kept rather than a new one, as the legacy layouts do
// with their home page, it does nothing: the place is marked on the
// element it was kept from, and that element is shown again. The TV
// layout (html.layout-tv) is left alone too. A push of the home page
// while it shows (the side menu's Home, or the phone app's Home tab
// through Emby.Page.goHome()), or the logo's (#/, which jellyfin-web
// replaces with #/home), goes to the top at once, where jellyfin-web
// would keep the place; the place of the entry left stays for Back. It
// keeps the places of the last 50 entries, in memory only. Any error
// just stops it.
//
// Anchors in jellyfin-web:
//   route: home
//   route: search
//   selector: .homeSectionsContainer
//   selector: .page
//   selector: .verticalSection
//   selector: .sectionTitle
//   selector: .emby-scroller
//   class: hide
//   class: layout-tv
//   class: section
//   id: searchPage
//   property: getScrollPosition
//   property: scrollToPosition

  var homeRoute = /^#!?\/home(?:\.html)?(?:[?#]|$)/i
  var tabParameter = /[?&]tab=([^&#]*)/
  var searchRoute = /^#!?\/search(?:\.html)?\?/i
  // The logo opens #/, which jellyfin-web replaces with #/home.
  var rootRoute = /^(?:#!?\/?)?$/
  var rowClass = /(?:^|\s)section(\d+)(?:\s|$)/
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The places kept, by the key of their history entry, and those keys,
  // the oldest first; the number the last place was marked with.
  var places = {}
  var kept = []
  var marks = 0
  // The entry shown, as { key, route, kind }, its kind being 'home',
  // 'search' or ''.
  var current = null
  // The place being brought back, as { place, row, ready, until, settle }:
  // row once the place's row was found, ready once the page reached its
  // place, until and settle the timers that stop it.
  var restoring = null
  function keyOf() {
    var state = history.state
    return state && typeof state.key === 'string' && state.key ? state.key : 'first'
  }
  // kindOf tells whether a route is the home page's Home tab, search
  // results, or neither.
  function kindOf(route) {
    if (homeRoute.test(route)) {
      var tab = tabParameter.exec(route)
      return !tab || tab[1] === '' || tab[1] === '0' ? 'home' : ''
    }
    return searchRoute.test(route) ? 'search' : ''
  }
  function tv() {
    var root = document.documentElement
    return !!(root && root.classList && root.classList.contains('layout-tv'))
  }
  // shown returns the element that holds the rows of the home page or of
  // the search page shown, or null.
  function shown(kind) {
    if (kind === 'search') {
      var page = document.getElementById('searchPage')
      return page && !page.classList.contains('hide') ? page : null
    }
    var containers = document.querySelectorAll('.homeSectionsContainer')
    for (var i = 0; i < containers.length; i++) {
      var holder = containers[i].closest('.page')
      if (!holder || !holder.classList.contains('hide')) return containers[i]
    }
    return null
  }
  // rowId names a row: by its data-polyfin-row, else its .sectionN class,
  // else its title; '' when it has none.
  function rowId(row) {
    var id = row.getAttribute('data-polyfin-row')
    if (id) return 'row ' + id
    var section = rowClass.exec(String(row.className))
    if (section) return 'section ' + section[1]
    var title = row.querySelector('.sectionTitle')
    var text = title ? String(title.textContent).replace(/\s+/g, ' ').trim() : ''
    return text ? 'title ' + text : ''
  }
  // topRow returns the named row at the top of the screen: the innermost
  // across its top edge, else the first below it.
  function topRow(container) {
    var rows = container.querySelectorAll('.verticalSection')
    var found = null
    for (var i = 0; i < rows.length; i++) {
      var box = rows[i].getBoundingClientRect()
      if (box.height <= 0 || box.bottom <= 0 || !rowId(rows[i])) continue
      if (box.top > 0) {
        if (!found) found = rows[i]
        break
      }
      found = rows[i]
    }
    return found
  }
  // findRow returns the row of the container named id, or null.
  function findRow(container, id) {
    var rows = container.querySelectorAll('.verticalSection')
    for (var i = 0; i < rows.length; i++) if (rowId(rows[i]) === id) return rows[i]
    return null
  }
  // sideways returns how far a row's scroller is scrolled, as
  // jellyfin-web's scroller tells it, its scrollLeft for a scroller
  // jellyfin-web has not made one; null while it cannot tell.
  function sideways(scroller) {
    var at = typeof scroller.getScrollPosition === 'function' ? scroller.getScrollPosition() : scroller.scrollLeft
    return typeof at === 'number' && isFinite(at) ? at : null
  }
  // slide scrolls a row's scroller sideways back to at. A row whose titles
  // have not come yet stays at its start, and jellyfin-web's scroller
  // goes back to its start, with an animation, as the row's size changes
  // or the row is drawn anew: each look slides it again.
  function slide(scroller, at) {
    var now = sideways(scroller)
    if (now === null || Math.abs(now - at) <= 1) return
    if (typeof scroller.scrollToPosition === 'function') scroller.scrollToPosition(at, true)
    else scroller.scrollLeft = at
  }
  // keep keeps the place of the entry the route leaves, when the page
  // shown is its own, and marks the page's element with it. A place still
  // being brought back stays as it was kept.
  function keep(entry) {
    if (!entry.kind || tv() || (restoring && !restoring.ready)) return
    var container = shown(entry.kind)
    if (!container) return
    var place = { y: pageYOffset, row: '', offset: 0, sideways: {}, mark: ++marks }
    container.__polyfinPlace = place.mark
    var row = topRow(container)
    if (row) {
      place.row = rowId(row)
      place.offset = row.getBoundingClientRect().top
    }
    var scrollers = container.querySelectorAll('.emby-scroller')
    for (var i = 0; i < scrollers.length; i++) {
      var holder = scrollers[i].closest('.verticalSection')
      var id = holder && rowId(holder)
      var at = sideways(scrollers[i])
      if (id && at) place.sideways[id] = at
    }
    if (!Object.prototype.hasOwnProperty.call(places, entry.key)) kept.push(entry.key)
    places[entry.key] = place
    while (kept.length > 50) delete places[kept.shift()]
  }
  // look brings the place back as far as the page allows.
  var look = quietly(function () {
    var at = restoring
    if (!at) return
    var place = at.place
    var container = shown(current.kind)
    if (!container) return
    if (container.__polyfinPlace === place.mark) return stop()
    var row = place.row ? findRow(container, place.row) : null
    var box = row && row.getBoundingClientRect()
    var target = null
    if (box && box.height > 0) {
      at.row = true
      target = pageYOffset + box.top - place.offset
    } else if (!at.row) {
      target = place.y
    }
    var root = document.scrollingElement || document.documentElement
    if (target !== null) {
      target = Math.max(0, Math.round(target))
      if (root.scrollHeight - innerHeight >= target - 1) {
        if (Math.abs(pageYOffset - target) > 1) scrollTo(0, target)
        if (!at.ready) {
          at.ready = true
          at.settle = setTimeout(quietly(stop), 1000)
        }
      }
    }
    var scrollers = container.querySelectorAll('.emby-scroller')
    for (var i = 0; i < scrollers.length; i++) {
      var holder = scrollers[i].closest('.verticalSection')
      var id = holder && rowId(holder)
      if (id && Object.prototype.hasOwnProperty.call(place.sideways, id)) slide(scrollers[i], place.sideways[id])
    }
  })
  // start brings a place back: it looks once a frame, as the rows' titles
  // and pictures widen them without changing the page, and as soon as a
  // row scrolls, which does not bubble, heard as it goes down.
  function start(place) {
    var mine = { place: place, row: false, ready: false, until: setTimeout(quietly(stop), 3000), settle: null }
    restoring = mine
    function frame() {
      if (restoring !== mine) return
      look()
      if (restoring !== mine) return
      if (typeof requestAnimationFrame === 'function') requestAnimationFrame(frame)
      else setTimeout(frame, 16)
    }
    addEventListener('scroll', look, true)
    frame()
  }
  function stop() {
    if (!restoring) return
    clearTimeout(restoring.until)
    clearTimeout(restoring.settle)
    restoring = null
    removeEventListener('scroll', look, true)
  }
  // follow keeps the place of the entry the route leaves, brings back the
  // place of the entry a popstate shows, and sends a push of the home page
  // while it shows to the top.
  function follow(how) {
    var key = keyOf()
    var route = location.hash
    if (current && current.key === key && current.route === route) return
    var left = current
    var moved = !left || left.key !== key
    if (left && moved) keep(left)
    stop()
    current = { key: key, route: route, kind: kindOf(route) }
    if (!current.kind || tv()) return
    if (how === 'pop') {
      var place = Object.prototype.hasOwnProperty.call(places, key) ? places[key] : null
      if (place && (place.y > 0 || Object.keys(place.sideways).length)) start(place)
    } else if ((how === 'push' || how === 'replace') && moved && current.kind === 'home' && left) {
      if (left.kind === 'home' || rootRoute.test(left.route)) scrollTo(0, 0)
    }
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; Back and Forward fire popstate, typed addresses
  // hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)(name === 'pushState' ? 'push' : 'replace')
      return result
    }
  })
  addEventListener(
    'hashchange',
    quietly(function () {
      follow('hash')
    })
  )
  addEventListener(
    'popstate',
    quietly(function () {
      follow('pop')
    })
  )
  // The user's own scrolling stops a place being brought back; these run
  // before the page's own listeners.
  ;['wheel', 'touchstart', 'keydown', 'mousedown', 'click'].forEach(function (type) {
    addEventListener(type, quietly(stop), true)
  })
  quietly(follow)('')
