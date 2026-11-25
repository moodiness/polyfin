// Part 14: the answers jellyfin-web keeps, dropped for another user.
//
// It drops what jellyfin-web keeps of a user's answers once
// another user signs in. jellyfin-web 12.4 keeps its answers in a React
// Query cache for a minute before asking again, and for 24 hours in all,
// in memory and in the browser's IndexedDB (database keyval-store, store
// keyval, key jellyfin-query-cache), which a later load restores. Some of
// them are keyed without the user: the search page's Suggestions
// (SearchSuggestions) and results (Search), a library's Suggestions tab
// (SuggestionSectionWithItems: its Continue Watching, Latest and Next Up),
// MovieRecommendations, filters and studios. Its own Sign out clears the
// cache; a session ended otherwise, such as one an administrator signs out
// or a browser that lost its sign-in, does not, and the next user to sign
// in would see the last one's answers, titles their parental control
// forbids included, until jellyfin-web asks again. The script keeps in
// the browser's storage the user, as server and user, the kept answers are
// for. As it starts, before jellyfin-web, when the user its saved
// credentials sign in (jellyfin_credentials, the server used last) is
// another, it deletes the copy in IndexedDB, so that jellyfin-web restores
// nothing. Whenever the route changes, as it does when a user signs in on
// jellyfin-web's sign-in page, it looks at the user jellyfin-web's client
// (ApiClient) signed in, and when that is another, it deletes that copy and
// empties the cache in memory, through jellyfin-web's own LANGUAGE_CHANGE
// listeners on the document (jellyfin-web clears the cache as the language
// changes). Signing out changes nothing, nor does the same user signing in
// again. When jellyfin-web lacks any of these, the rest still applies, and
// nothing else changes. Any error just stops it.
//
// Anchors in jellyfin-web:
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.serverId
//   property: _callbacks
//   event: LANGUAGE_CHANGE
//   storage: jellyfin_credentials
//   storage: keyval-store
//   storage: keyval
//   storage: jellyfin-query-cache

  var keptForKey = 'polyfinKeptFor'
  // The user, as server/user, the answers jellyfin-web keeps are for.
  var keptFor = null
  try {
    keptFor = window.localStorage.getItem(keptForKey)
  } catch (error) {}
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // forgetKept deletes jellyfin-web's copy of its cache in IndexedDB, then
  // empties the cache in memory as a language change does. A browser
  // without the database has nothing kept: the upgrade that would make it
  // is aborted, leaving it to jellyfin-web.
  function forgetKept() {
    try {
      var request = indexedDB.open('keyval-store')
      request.onupgradeneeded = function () {
        request.transaction.abort()
      }
      request.onerror = function (event) {
        event.preventDefault()
      }
      request.onsuccess = function () {
        var database = request.result
        try {
          if (database.objectStoreNames.contains('keyval')) {
            database.transaction('keyval', 'readwrite').objectStore('keyval').delete('jellyfin-query-cache')
          }
        } finally {
          database.close()
        }
      }
    } catch (error) {}
    var listeners = ((document._callbacks || {}).LANGUAGE_CHANGE || []).slice()
    for (var i = 0; i < listeners.length; i++) {
      try {
        listeners[i].apply(document, [{ type: 'LANGUAGE_CHANGE' }, document.documentElement.lang])
      } catch (error) {}
    }
  }
  // signedIn forgets the kept answers when user, signed in, is not the one
  // they were kept for.
  function signedIn(user) {
    if (!user || user === keptFor) return
    keptFor = user
    try {
      window.localStorage.setItem(keptForKey, user)
    } catch (error) {}
    forgetKept()
  }
  // savedUser is the user jellyfin-web's saved credentials sign in on the
  // server it used last, as server/user; "" when none.
  function savedUser() {
    var servers = JSON.parse(window.localStorage.getItem('jellyfin_credentials') || '{}').Servers || []
    var last = null
    for (var i = 0; i < servers.length; i++) {
      var server = servers[i]
      if (!server || !server.AccessToken || !server.UserId) continue
      if (!last || (server.DateLastAccessed || 0) > (last.DateLastAccessed || 0)) last = server
    }
    return last ? last.Id + '/' + last.UserId : ''
  }
  try {
    signedIn(savedUser())
  } catch (error) {}
  // A user signing in in the page goes with a route change: jellyfin-web
  // then leaves its sign-in page.
  function follow() {
    var api = window.ApiClient
    var user = api && api.getCurrentUserId()
    if (user) signedIn((typeof api.serverId === 'function' ? api.serverId() : '') + '/' + user)
  }
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
