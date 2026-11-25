// Part 15: recent searches.
//
// On jellyfin-web's search page (#/search, its #searchPage), it
// lists the user's recent searches while the field is empty: up to 10,
// newest first, between the field (.searchFields) and the Suggestions
// (.searchSuggestions), drawn as those are, under a "Recent searches"
// heading. Each is a link that searches it again, with a button that
// removes it, and "Clear all" removes them all. jellyfin-web 12.4 keeps the
// field in the route (#/search?query=…, replaced as the user types, by
// useSearchParam), and shows what the field's search found half a second
// after the last key (useDebounceValue): its results (.searchResults), a
// line that nothing was found (.noItemsMessage), or the Suggestions for an
// empty field. As jellyfin-web draws results with a card, the script notes
// the route's search as shown, for the user signed in; a line that nothing
// was found drops it. It keeps the search shown as the user leaves it:
// opening a result or any other page, clearing the field, typing a search
// that neither starts with it nor is its start, or closing the tab. A
// search typed on, or back, thus replaces the one before, and a search
// that found nothing is never kept. A search kept again moves to the top,
// the same words in other capitals counting as one. A link opens the
// search page on its search, in the page's library if any (parentId,
// collectionType), as jellyfin-web's own "Retry with a global search" link
// opens #/search?query=…: jellyfin-web fills the field from the route and
// searches. The searches stay in the browser, nothing goes to the server:
// in its storage, under polyfinRecentSearches/ then the server and the
// user (server/user, as the fourteenth names the user), so that another
// user signing in never sees them. Signing out keeps them for the user's
// next sign-in in this browser, as jellyfin-web keeps the settings it
// stores in the browser for a user (appSettings, keyed by the user's ID)
// through its own Sign out. Storage that fails, or holds anything else,
// counts as none. Its words follow jellyfin-web's language, as the
// third's. Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: search
//   selector: .searchResults
//   selector: .card
//   selector: .noItemsMessage
//   selector: .searchFields
//   selector: .material-icons
//   class: hide
//   class: layout-tv
//   class: show-focus
//   class: button-link
//   class: emby-button
//   class: paper-icon-button-light
//   class: padded-left
//   class: padded-right
//   id: searchPage
//   id: searchTextInput
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.serverId

  var route = /^#!?\/search(?:[?#]|$)/i
  var recentKey = 'polyfinRecentSearches/'
  var most = 10
  // removeOne names the button that removes the search in {0}.
  var searchWords = {
    en: { title: 'Recent searches', remove: 'Remove', removeOne: 'Remove “{0}”', clear: 'Clear all' },
    fr: { title: 'Recherches récentes', remove: 'Supprimer', removeOne: 'Supprimer «\u202f{0}\u202f»', clear: 'Tout effacer' },
    de: { title: 'Letzte Suchen', remove: 'Entfernen', removeOne: '„{0}“ entfernen', clear: 'Alle löschen' },
    es: { title: 'Búsquedas recientes', remove: 'Eliminar', removeOne: 'Eliminar «{0}»', clear: 'Borrar todo' },
    it: { title: 'Ricerche recenti', remove: 'Rimuovi', removeOne: 'Rimuovi «{0}»', clear: 'Cancella tutto' },
    pt: { title: 'Buscas recentes', remove: 'Remover', removeOne: 'Remover “{0}”', clear: 'Limpar tudo' },
  }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (searchWords[language()] || searchWords.en)[key]
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The route's search, trimmed, when last followed, null off the search
  // page; the search shown with results, with the user it is for, as
  // { user, query }; the first card of the results and the line that
  // nothing was found when last looked at; the list drawn and what it was
  // drawn for; and whether the page's changes are followed, as they are
  // while the page is (see core/page-changes.js).
  var typed = null
  var shown = null
  var shownCard = null
  var shownNothing = null
  var drawn = null
  var drawnFor = ''
  var watching = false
  // user is the user signed in, as server/user; "" when none.
  function user() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId()
    return id ? (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id : ''
  }
  // params are the parameters of a search route, null for another route.
  function params(hash) {
    if (!route.test(hash)) return null
    var at = hash.indexOf('?')
    return new RouteParams(at < 0 ? '' : hash.slice(at + 1).split('#')[0])
  }
  // RouteParams are a route's parameters, read and written as
  // URLSearchParams does, which browsers before Chrome 49 lack: the script
  // runs before jellyfin-web's polyfills. get gives a parameter's first
  // value, set replaces its first and drops the others, and toString
  // writes them back.
  function RouteParams(text) {
    this.pairs = []
    var parts = text.split('&')
    for (var i = 0; i < parts.length; i++) {
      if (!parts[i]) continue
      var at = parts[i].indexOf('=')
      this.pairs.push([decoded(at < 0 ? parts[i] : parts[i].slice(0, at)), at < 0 ? '' : decoded(parts[i].slice(at + 1))])
    }
  }
  RouteParams.prototype.get = function (name) {
    for (var i = 0; i < this.pairs.length; i++) if (this.pairs[i][0] === name) return this.pairs[i][1]
    return null
  }
  RouteParams.prototype.set = function (name, value) {
    var kept = []
    var placed = false
    for (var i = 0; i < this.pairs.length; i++) {
      if (this.pairs[i][0] !== name) {
        kept.push(this.pairs[i])
      } else if (!placed) {
        kept.push([name, String(value)])
        placed = true
      }
    }
    if (!placed) kept.push([name, String(value)])
    this.pairs = kept
  }
  RouteParams.prototype.toString = function () {
    return this.pairs
      .map(function (pair) {
        return encoded(pair[0]) + '=' + encoded(pair[1])
      })
      .join('&')
  }
  // decoded reads a parameter's name or value, + standing for a space; a
  // malformed escape leaves it as it is.
  function decoded(text) {
    var plain = text.replace(/\+/g, ' ')
    try {
      return decodeURIComponent(plain)
    } catch (error) {
      return plain
    }
  }
  // encoded writes a parameter's name or value as URLSearchParams does
  // (application/x-www-form-urlencoded): a space as +, and every character
  // but letters, digits and *-._ escaped.
  function encoded(text) {
    return encodeURIComponent(text)
      .replace(/[!'()~]/g, function (character) {
        return '%' + character.charCodeAt(0).toString(16).toUpperCase()
      })
      .replace(/%20/g, '+')
  }
  // recent are the searches kept for who, newest first.
  function recent(who) {
    try {
      var list = JSON.parse(window.localStorage.getItem(recentKey + who))
      if (!Array.isArray(list)) return []
      return list
        .filter(function (query) {
          return typeof query === 'string' && query.trim() !== ''
        })
        .slice(0, most)
    } catch (error) {
      return []
    }
  }
  function save(who, list) {
    try {
      if (list.length) window.localStorage.setItem(recentKey + who, JSON.stringify(list))
      else window.localStorage.removeItem(recentKey + who)
    } catch (error) {}
  }
  // same tells whether two searches are the same words, in any capitals.
  function same(a, b) {
    return a.toLowerCase() === b.toLowerCase()
  }
  // without is the list without query.
  function without(list, query) {
    return list.filter(function (kept) {
      return !same(kept, query)
    })
  }
  // leave keeps the search shown, at the top, as the user leaves it.
  function leave() {
    var search = shown
    shown = null
    if (!search) return
    var list = without(recent(search.user), search.query)
    list.unshift(search.query)
    save(search.user, list.slice(0, most))
  }
  // follow follows the search page's field, as the route holds it: it
  // keeps the search shown when the route leaves the page, clears the
  // field, or holds a search that neither starts with it nor is its start.
  function follow() {
    var found = params(location.hash)
    var query = found ? (found.get('query') || '').trim() : null
    if (shown) {
      var a = shown.query.toLowerCase()
      var b = (query || '').toLowerCase()
      if (!b || (a.indexOf(b) !== 0 && b.indexOf(a) !== 0)) leave()
    }
    typed = query
    if (query === null) {
      if (watching) document.removeEventListener('polyfinpagechange', looked)
      watching = false
      shownCard = null
      shownNothing = null
      undraw()
      return
    }
    // The script runs in the page's head: a page opened on the search
    // page's address has no body yet, which watch waits for.
    if (document.body) watch()
    else addEventListener('DOMContentLoaded', quietly(watch))
  }
  // watch follows the page's changes while the search page is followed,
  // and looks at the page.
  function watch() {
    if (typed === null || !document.body) return
    if (!watching) {
      watching = true
      document.addEventListener('polyfinpagechange', looked)
    }
    look()
  }
  // look as it listens to the page's changes.
  var looked = quietly(look)
  function searchPage() {
    var page = document.getElementById('searchPage')
    return page && !page.classList.contains('hide') ? page : null
  }
  // look notes the field's search as shown when jellyfin-web draws its
  // results with a card, or drops it when jellyfin-web says that nothing
  // was found, then lists the recent searches while the field is empty
  // and jellyfin-web shows no result.
  function look() {
    if (typed === null) return
    var page = searchPage()
    if (!page) return undraw()
    var results = page.querySelector('.searchResults')
    var card = results && results.querySelector('.card')
    if (card !== shownCard) {
      shownCard = card
      var who = user()
      if (card && typed && who) shown = { user: who, query: typed }
    }
    var nothing = page.querySelector('.noItemsMessage')
    if (nothing !== shownNothing) {
      shownNothing = nothing
      if (nothing) shown = null
    }
    place(page, !typed && !results && !nothing)
  }
  // undraw takes the list off.
  function undraw() {
    if (drawn && drawn.parentNode) drawn.parentNode.removeChild(drawn)
    drawn = null
    drawnFor = ''
  }
  // place puts the list after the field when it is to show, again when
  // it changes or jellyfin-web draws the page anew.
  function place(page, empty) {
    var who = user()
    var list = empty && who ? recent(who) : []
    var fields = page.querySelector('.searchFields')
    if (!list.length || !fields) return undraw()
    var key = [who, language(), location.hash].concat(list).join('\n')
    if (drawn && drawnFor === key && drawn.parentNode === fields.parentNode && drawn.previousSibling === fields) return
    undraw()
    style()
    drawn = draw(who, list)
    drawnFor = key
    fields.parentNode.insertBefore(drawn, fields.nextSibling)
  }
  function made(tag, className, children) {
    var element = document.createElement(tag)
    element.className = className
    ;(children || []).forEach(function (child) {
      if (typeof child === 'string') element.textContent = child
      else element.appendChild(child)
    })
    return element
  }
  // address is the route that searches query again on the search page,
  // in the page's library, if any.
  function address(query) {
    var found = params(location.hash) || new RouteParams('')
    found.set('query', query)
    return '#/search?' + found.toString()
  }
  // draw makes the list, drawn as jellyfin-web draws the Suggestions:
  // centred, a heading, then a link for each search, with its button that
  // removes it, then the button that removes them all. The TV layout
  // scales the button with the focus, as jellyfin-web's own. A link
  // chosen gives the focus to the field, as the list leaves with it.
  function draw(who, list) {
    var root = document.documentElement
    var focus = root.classList && root.classList.contains('layout-tv') ? ' show-focus' : ''
    var entries = made('div', 'polyfinRecentSearchesList padded-left padded-right')
    list.forEach(function (query) {
      var link = made('a', 'button-link emby-button polyfinRecentSearchLink' + focus, [query])
      link.setAttribute('href', address(query))
      link.addEventListener('click', quietly(focusField))
      var remove = made('button', 'paper-icon-button-light polyfinRecentSearchRemove' + focus, [
        made('span', 'material-icons close'),
      ])
      remove.firstChild.setAttribute('aria-hidden', 'true')
      remove.setAttribute('type', 'button')
      remove.setAttribute(
        'aria-label',
        say('removeOne').replace('{0}', function () {
          return query
        })
      )
      remove.title = say('remove')
      remove.addEventListener(
        'click',
        quietly(function () {
          forget(who, query)
        })
      )
      entries.appendChild(made('div', 'polyfinRecentSearch', [made('span', 'polyfinRecentSearchItem', [link, remove])]))
    })
    var clear = made('button', 'button-link emby-button polyfinRecentSearchesClear' + focus, [say('clear')])
    clear.setAttribute('type', 'button')
    clear.addEventListener(
      'click',
      quietly(function () {
        forget(who, null)
      })
    )
    var heading = made('h2', 'sectionTitle padded-left padded-right', [say('title')])
    return made('div', 'verticalSection polyfinRecentSearches', [made('div', '', [heading]), entries, made('div', '', [clear])])
  }
  // forget removes query from who's searches, or all of them for null,
  // then gives the focus to the next button that removes one, the last
  // one, or the field when none is left.
  function forget(who, query) {
    var list = recent(who)
    var at = query === null ? 0 : list.indexOf(query)
    save(who, query === null ? [] : without(list, query))
    look()
    var removes = drawn ? drawn.querySelectorAll('.polyfinRecentSearchRemove') : []
    if (removes.length) removes[Math.min(Math.max(at, 0), removes.length - 1)].focus()
    else focusField()
  }
  function focusField() {
    var field = document.getElementById('searchTextInput')
    if (field) field.focus()
  }
  // style adds the look of the list, once: the searches centred as the
  // Suggestions, each button that removes one after its search.
  function style() {
    if (document.getElementById('polyfinRecentSearchesStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinRecentSearchesStyle'
    sheet.textContent =
      '.polyfinRecentSearches{text-align:center}' +
      '.polyfinRecentSearchItem{position:relative;display:inline-block}' +
      '.polyfinRecentSearchLink,.polyfinRecentSearchesClear{display:inline-block;padding:.5em 1em}' +
      '.polyfinRecentSearchRemove{position:absolute;top:0;bottom:0;left:100%;margin:0;padding:0 .2em;opacity:.6}' +
      '.polyfinRecentSearchRemove>.material-icons{font-size:1.2em}' +
      '.polyfinRecentSearchRemove:hover,.polyfinRecentSearchRemove:focus{opacity:1}' +
      '.polyfinRecentSearchesClear{margin-top:.3em;opacity:.7}' +
      '.polyfinRecentSearchesClear:hover,.polyfinRecentSearchesClear:focus{opacity:1}'
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
  // Closing the tab, or loading another page in it, leaves the search too.
  addEventListener('pagehide', quietly(leave))
  quietly(follow)()
