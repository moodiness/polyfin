// Part 24: the type filter of a collection's page.
//
// It offers a type filter on a collection's page, which
// loads only the type chosen. jellyfin-web 12.4 lists a collection's
// entries in one request without a limit (ParentId of the page's title,
// no SortBy, IncludeItemTypes or Recursive, through ApiClient.getItems,
// whose requests go through fetch) and groups them by type, in sections
// of its own (.collectionItems): Movies, Series, Episodes, Videos,
// Collections, then the other entries. The script sees that request go
// out, as it wraps fetch from the page's head, before jellyfin-web: the
// page is then a collection's, and it asks
// /Polyfin/Items/{id}/CollectionTypes, which answers the Jellyfin types
// the collection lists for the user. When there are several, a row of
// buttons goes before the sections, "All" then one for each of those
// types jellyfin-web has a section for, the one shown pressed
// (aria-pressed), in rounded outlines. Under the LumaaGlaass theme (its
// loader's script, lg-script, in the page), the row takes the classes,
// and so the look, of the theme's own filter, which it hides: that one
// offers Movies and Series and only hides the cards loaded. Pressing one
// keeps the choice and has jellyfin-web list the page anew, through the
// notifyRefreshNeeded that jellyfin-web gives the container of each
// section, as it does when an entry is taken out of the collection: the
// request then goes with IncludeItemTypes of that type, so that the
// browser gets and draws that type alone, and Polyfin reads, for an
// addon's collection, only the catalogs that list it. "All" asks as
// jellyfin-web does. The choice is kept in the browser's storage,
// under polyfinCollectionTypes/ then the server and the user (server/user,
// as the fifteenth names them), for the last 100 collections, so that it
// applies from the page's first request the next time, without waiting
// for any answer; it is the device's, as small screens are what the
// filter is for. Other apps, other users and other browsers get every
// entry. A choice the collection no longer offers is forgotten, and the
// page listed again whole. A page shown again as jellyfin-web left it,
// without asking again, gets the filter back from the types answered
// for it before, for the last 20 collections. The page's changes put the
// row back as jellyfin-web renders the page anew; it goes as the route
// leaves the collection. Its words follow jellyfin-web's language, as the
// third's. Without fetch it does nothing, and any error just stops it.
//
// Anchors in jellyfin-web:
//   route: details
//   selector: .videoPlayerContainer
//   selector: .page.itemDetailPage:not(.hide)
//   selector: .collectionItems
//   selector: .collectionItemsContainer
//   class: verticalSection
//   class: detailVerticalSection
//   class: padded-left
//   class: padded-right
//   class: emby-button
//   class: layout-tv
//   class: show-focus
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.serverId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.getItems
//   property: notifyRefreshNeeded

  if (typeof window === 'undefined' || typeof window.fetch !== 'function') return
  var route = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  // listing is the address of a listing of items, with its query.
  var listing = /\/(?:Users\/[^\/?#]+\/)?Items\?([^#]*)$/i
  // A collection's page asks for none of these; other listings of the
  // title's children do.
  var otherListings = ['sortby', 'includeitemtypes', 'excludeitemtypes', 'mediatypes', 'filters', 'recursive', 'limit', 'startindex', 'ids']
  var choicesKey = 'polyfinCollectionTypes/'
  // most is how many collections' choices are kept for a user, the last
  // first; mostKnown how many collections' types are kept in memory.
  var most = 100
  var mostKnown = 20
  // The types offered, in the order jellyfin-web draws their sections,
  // with the key of their word.
  var offered = [
    ['Movie', 'movies'],
    ['Series', 'series'],
    ['Episode', 'episodes'],
    ['Video', 'videos'],
    ['BoxSet', 'collections'],
  ]
  var typeWords = {
    en: { label: 'Show', all: 'All', movies: 'Movies', series: 'Series', episodes: 'Episodes', videos: 'Videos', collections: 'Collections' },
    fr: { label: 'Afficher', all: 'Tout', movies: 'Films', series: 'Séries', episodes: 'Épisodes', videos: 'Vidéos', collections: 'Collections' },
    de: { label: 'Anzeigen', all: 'Alle', movies: 'Filme', series: 'Serien', episodes: 'Episoden', videos: 'Videos', collections: 'Sammlungen' },
    es: { label: 'Mostrar', all: 'Todo', movies: 'Películas', series: 'Series', episodes: 'Episodios', videos: 'Vídeos', collections: 'Colecciones' },
    it: { label: 'Mostra', all: 'Tutto', movies: 'Film', series: 'Serie', episodes: 'Episodi', videos: 'Video', collections: 'Collezioni' },
    pt: { label: 'Mostrar', all: 'Tudo', movies: 'Filmes', series: 'Séries', episodes: 'Episódios', videos: 'Vídeos', collections: 'Coleções' },
  }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (typeWords[language()] || typeWords.en)[key]
  }
  function quietly(listener) {
    return function () {
      try {
        return listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The title the route opens, lowercase, "" off a title's page; the
  // collection whose page is followed and its types, as Polyfin answered
  // them, null until it does; the collection asked since the route opened
  // it; the collection whose page is to be listed again, once drawn; the
  // types of the last collections listed, as [collection, types] with the
  // last first; the filter drawn and what it was drawn for; and whether
  // the page's changes are followed, as they are while a collection's
  // page is (see core/page-changes.js).
  var title = ''
  var collection = ''
  var types = null
  var asked = ''
  var due = ''
  var known = []
  var drawn = null
  var drawnFor = ''
  var watching = false
  // user is the user signed in, as server/user; "" when none.
  function user() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId()
    return id ? (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id : ''
  }
  // choices are who's choices, [collection, type] with the last first.
  function choices(who) {
    try {
      var list = JSON.parse(window.localStorage.getItem(choicesKey + who))
      if (Object.prototype.toString.call(list) !== '[object Array]') return []
      return list
        .filter(function (pair) {
          return pair && typeof pair[0] === 'string' && typeof pair[1] === 'string'
        })
        .slice(0, most)
    } catch (error) {
      return []
    }
  }
  // chosen is the type who chose for a collection, "" for all.
  function chosen(who, id) {
    if (!who) return ''
    var list = choices(who)
    for (var i = 0; i < list.length; i++) if (list[i][0] === id) return list[i][1]
    return ''
  }
  // choose keeps who's choice of type for a collection, first; all, "",
  // forgets it.
  function choose(who, id, type) {
    var list = choices(who).filter(function (pair) {
      return pair[0] !== id
    })
    if (type) list.unshift([id, type])
    try {
      if (list.length) window.localStorage.setItem(choicesKey + who, JSON.stringify(list.slice(0, most)))
      else window.localStorage.removeItem(choicesKey + who)
    } catch (error) {}
  }
  // params are a query's parameters, by their name in lower case.
  function params(query) {
    var found = {}
    var parts = query.split('&')
    for (var i = 0; i < parts.length; i++) {
      if (!parts[i]) continue
      var at = parts[i].indexOf('=')
      var name = at < 0 ? parts[i] : parts[i].slice(0, at)
      var value = at < 0 ? '' : parts[i].slice(at + 1)
      try {
        found[decodeURIComponent(name).toLowerCase()] = decodeURIComponent(value.replace(/\+/g, ' '))
      } catch (error) {
        found[name.toLowerCase()] = value
      }
    }
    return found
  }
  // narrowed is the address a request is sent to: jellyfin-web's listing
  // of the page's collection, which asks for every entry in one request
  // and groups them by type, asks for the type chosen alone. Any other
  // request goes as it is.
  function narrowed(address) {
    var match = title && listing.exec(address)
    if (!match) return address
    var asked = params(match[1])
    if (String(asked.parentid || '').toLowerCase() !== title) return address
    for (var i = 0; i < otherListings.length; i++) if (otherListings[i] in asked) return address
    var id = title
    setTimeout(
      quietly(function () {
        listed(id)
      }),
      0
    )
    var type = chosen(user(), id)
    return type ? address + '&IncludeItemTypes=' + encodeURIComponent(type) : address
  }
  var originalFetch = window.fetch
  window.fetch = function (input, init) {
    var address = input
    try {
      if (typeof input === 'string') address = narrowed(input)
    } catch (error) {}
    return originalFetch.call(window, address, init)
  }
  // listed follows the page of the collection jellyfin-web listed, and
  // asks Polyfin the types it holds, once each time the route opens it. A
  // type chosen that the collection no longer offers gives way to all of
  // them, and the page is listed again.
  function listed(id) {
    if (id !== title || id === asked) return
    asked = id
    if (collection !== id) types = null
    collection = id
    var api = window.ApiClient
    if (!api) return
    api.getJSON(api.getUrl('Polyfin/Items/' + id + '/CollectionTypes')).then(
      quietly(function (answer) {
        if (!answer || Object.prototype.toString.call(answer.Types) !== '[object Array]') return
        remember(id, answer.Types)
        if (collection !== id) return
        types = answer.Types
        var who = user()
        var type = chosen(who, id)
        var offer = offers()
        for (var i = 0; i < offer.length; i++) if (offer[i][0] === type) type = ''
        if (type) {
          choose(who, id, '')
          due = id
        }
        watch()
      }),
      function () {}
    )
  }
  // remember keeps a collection's types for its page shown again, which
  // jellyfin-web shows as it was, without listing it again.
  function remember(id, list) {
    known = known.filter(function (entry) {
      return entry[0] !== id
    })
    known.unshift([id, list])
    known = known.slice(0, mostKnown)
  }
  // offers are the types offered for the collection's types, as
  // [type, word]: none when it holds a single type.
  function offers() {
    if (!types || types.length < 2) return []
    return offered.filter(function (entry) {
      return types.indexOf(entry[0]) >= 0
    })
  }
  // follow follows the route: the page of a collection known follows its
  // filter again; another page takes it off.
  function follow() {
    var match = route.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === title) return
    title = id
    undraw()
    collection = ''
    types = null
    asked = ''
    due = ''
    for (var i = 0; i < known.length; i++) {
      if (known[i][0] === id) {
        collection = id
        types = known[i][1]
      }
    }
    if (collection) watch()
    else if (watching) {
      watching = false
      document.removeEventListener('polyfinpagechange', placed)
    }
  }
  // watch follows the page's changes while a collection's page is
  // followed, and looks at the page.
  function watch() {
    if (!collection || !document.body) return
    if (!watching) {
      watching = true
      document.addEventListener('polyfinpagechange', placed)
    }
    place()
  }
  var placed = quietly(place)
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  function undraw() {
    if (drawn && drawn.parentNode) drawn.parentNode.removeChild(drawn)
    drawn = null
    drawnFor = ''
  }
  // place puts the filter before the collection's sections (.collectionItems)
  // while it offers types, again whenever jellyfin-web renders the page
  // anew, and marks the type chosen. A page to be listed again is, once
  // drawn.
  function place() {
    var page = collection === title ? shownPage() : null
    var sections = page && page.querySelector('.collectionItems')
    if (!sections || sections.classList.contains('hide')) return undraw()
    if (due === collection && refresh(page)) due = ''
    var offer = offers()
    var who = user()
    if (!offer.length || !who) return undraw()
    var themed = themeFilter()
    var key = [collection, who, language(), themed]
      .concat(
        offer.map(function (entry) {
          return entry[0]
        })
      )
      .join('\n')
    if (!drawn || drawnFor !== key) {
      undraw()
      style()
      drawn = draw(offer, themed)
      drawnFor = key
    }
    if (drawn.parentNode !== sections.parentNode || drawn.nextSibling !== sections) sections.parentNode.insertBefore(drawn, sections)
    mark(chosen(who, collection))
  }
  function made(tag, className, text) {
    var element = document.createElement(tag)
    element.className = className
    if (text) element.textContent = text
    return element
  }
  // themeFilter tells whether the LumaaGlaass theme runs (its loader's
  // script, lg-script, is in the page), which draws a filter of its own
  // above a collection that mixes movies and series: it only hides the
  // cards loaded. The filter then takes the look of the theme's, and the
  // style hides the theme's.
  function themeFilter() {
    return !!document.getElementById('lg-script')
  }
  // draw makes the filter: a button for all the types, then one for each
  // type offered, pressed for the type shown; in the theme's look under
  // LumaaGlaass, in rounded outlines otherwise. The TV layout scales the
  // button with the focus, as jellyfin-web's own.
  function draw(offer, themed) {
    var root = document.documentElement
    var focus = root.classList && root.classList.contains('layout-tv') ? ' show-focus' : ''
    var filter = made(
      'div',
      themed ? 'polyfinCollectionTypes lg-collection-filter' : 'polyfinCollectionTypes polyfinCollectionTypesOwn verticalSection detailVerticalSection padded-left padded-right'
    )
    var group = themed ? made('div', 'lg-collection-filter-group') : filter
    if (themed) filter.appendChild(group)
    group.setAttribute('role', 'group')
    group.setAttribute('aria-label', say('label'))
    ;[['', 'all']].concat(offer).forEach(function (entry) {
      var button = made('button', (themed ? 'lg-collection-filter-button' : 'emby-button') + ' polyfinCollectionType' + focus, say(entry[1]))
      button.setAttribute('type', 'button')
      button.setAttribute('data-type', entry[0])
      button.addEventListener(
        'click',
        quietly(function () {
          pick(entry[0])
        })
      )
      group.appendChild(button)
    })
    return filter
  }
  // mark presses the button of type, "" for all.
  function mark(type) {
    var buttons = drawn ? drawn.querySelectorAll('.polyfinCollectionType') : []
    for (var i = 0; i < buttons.length; i++) {
      var pressed = buttons[i].getAttribute('data-type') === type
      if (buttons[i].getAttribute('aria-pressed') === String(pressed)) continue
      buttons[i].setAttribute('aria-pressed', String(pressed))
      buttons[i].classList[pressed ? 'add' : 'remove']('polyfinCollectionTypeChosen')
    }
  }
  // pick keeps the type chosen for the collection and the user, and has
  // jellyfin-web list the page's entries again.
  function pick(type) {
    var who = user()
    if (!who || !collection || chosen(who, collection) === type) return
    choose(who, collection, type)
    mark(type)
    refresh(shownPage())
  }
  // refresh has jellyfin-web list the collection's entries on page again,
  // as it does when one is taken out of it: each container of its
  // sections carries the function that renders them anew. It tells
  // whether it found one.
  function refresh(page) {
    var container = page && page.querySelector('.collectionItemsContainer')
    if (!container || typeof container.notifyRefreshNeeded !== 'function') return false
    container.notifyRefreshNeeded()
    return true
  }
  // style adds the look of the filter, once: buttons in a row, each a
  // rounded outline, the one pressed filled; the theme's own filter
  // hidden before it.
  function style() {
    if (document.getElementById('polyfinCollectionTypesStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinCollectionTypesStyle'
    sheet.textContent =
      '.polyfinCollectionTypesOwn{display:flex;flex-wrap:wrap;align-items:center}' +
      '.polyfinCollectionTypesOwn>.polyfinCollectionType{margin:0 .5em .5em 0;padding:.4em 1.1em;border:1px solid currentColor;' +
      'border-radius:2em;background:transparent;color:inherit;opacity:.75;font-size:inherit}' +
      '.polyfinCollectionTypesOwn>.polyfinCollectionType:hover,.polyfinCollectionTypesOwn>.polyfinCollectionType:focus{opacity:1}' +
      '.polyfinCollectionTypesOwn>.polyfinCollectionTypeChosen{opacity:1;background:rgba(128,128,128,.35);font-weight:600}' +
      '.lg-collection-filter:not(.polyfinCollectionTypes):has(~.polyfinCollectionTypes){display:none!important}'
    document.head.appendChild(sheet)
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; links and typed addresses fire hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      quietly(follow)()
      return result
    }
  })
  addEventListener('hashchange', quietly(follow))
  addEventListener('popstate', quietly(follow))
  quietly(follow)()
