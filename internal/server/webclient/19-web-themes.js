// Part 19: each user's web player theme.
//
// It gives the page the web player theme of the user, which
// an administrator, or the user under My account, chooses in the admin
// app: the server's, LumaaGlaass, jellyfin-web's own look (none) or the
// Custom theme. The page describes the themes in its head, as JSON
// (#polyfinWebThemes): the server's theme, the stylesheet and the script
// of each theme that has some, and the cookie that tells the server the
// theme shown. As it starts, before jellyfin-web, the script picks the
// theme: the one it kept for the user jellyfin-web's saved credentials
// sign in (as the fourteenth finds them), else, signed out, the one of the
// user last signed in in this browser, else the server's. It keeps, in the
// browser's storage (polyfinWebTheme), each user's own theme, empty for the
// server's, up to 20 users, the last first. A browser that cannot run
// LumaaGlaass, without CSS's :has() or the syntax of ES2021, which the
// theme's stylesheet and script need, gets jellyfin-web's own look instead.
// It writes the theme's stylesheet in the head while the page is parsed,
// so that the page shows the theme from its first paint, unless the user
// turned off the server's CSS in jellyfin-web's display settings
// (disableCustomCss, kept in the browser for the user); and the theme's
// script, deferred, which runs after jellyfin-web's own scripts, as the
// server's custom script did. It names the theme in the cookie, so that
// Jellyfin's branding, which jellyfin-web applies after its own CSS, gives
// the same theme's CSS, imported from the same address; jellyfin-web's
// SDK asks for it through fetch without the browser's cookies, so those
// requests, to the branding alone, go with the cookies of the page's own
// server. Once jellyfin-web shows that CSS, loaded, the script takes its
// own stylesheet away, and jellyfin-web keeps the theme where it always
// did, after its own CSS, and turns it off with the user's display
// settings. When the stylesheet differs from the one shown at the last
// load in this browser, it deletes the answers jellyfin-web kept in
// IndexedDB, as the fourteenth does, which would show the CSS of the theme
// before. Once jellyfin-web has a user signed in, as the route changes, or
// as a load signed in starts, looked for every half second for half a
// minute, it asks /Polyfin/WebTheme, once for each user signed in, and
// keeps their own theme. When that theme is another than the page shows:
// if the theme shown has a script, which cannot be taken back, it reloads
// the page, once for that user and theme in the tab, and only if the
// browser's storage kept the theme, which the load then shows; else it
// changes the stylesheet at once, turns off jellyfin-web's copy of the one
// before, and adds the new theme's script. Signing out keeps the theme.
// Storage that fails counts as none, and any error just stops it.
//
// Anchors in jellyfin-web:
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.serverId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   storage: jellyfin_credentials
//   storage: disableCustomCss
//   storage: keyval-store
//   storage: keyval
//   storage: jellyfin-query-cache

  var themes = null
  try {
    var element = document.getElementById('polyfinWebThemes')
    themes = element && JSON.parse(element.textContent)
  } catch (error) {}
  if (!themes || !themes.Themes || typeof themes.Cookie !== 'string') return
  var storageKey = 'polyfinWebTheme'
  var reloadKey = 'polyfinWebThemeReload'
  var mostUsers = 20
  // resetKey keeps [server/user, time] of the last Reset my preferences
  // the browser applied for each user, the last first (see applyReset).
  var resetKey = 'polyfinPreferencesReset'
  var linkId = 'polyfinWebThemeCss'
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // jellyfin-web asks for the branding through fetch without the
  // browser's cookies (its SDK's axios sends credentials: 'omit'), which
  // would leave out the one naming the theme: those requests go with the
  // cookies of the page's own server. Its XMLHttpRequests send them anyway.
  var branding = /\/Branding\/(?:Configuration|Css(?:\.css)?)(?:[?#]|$)/i
  if (typeof window.fetch === 'function') {
    var originalFetch = window.fetch
    window.fetch = function (input, init) {
      var address = typeof input === 'string' ? input : input && input.url
      if (typeof address === 'string' && branding.test(address)) {
        var options = {}
        for (var name in init || {}) options[name] = init[name]
        options.credentials = 'same-origin'
        init = options
      }
      return originalFetch.call(window, input, init)
    }
  }
  // kept is what the browser keeps: Users, [server/user, theme] with the
  // last first, the theme empty for the server's; Last, the user last
  // signed in; Applied, the stylesheet the page showed, null when unknown.
  function kept() {
    try {
      var value = JSON.parse(window.localStorage.getItem(storageKey))
      if (value && Object.prototype.toString.call(value.Users) === '[object Array]') return value
    } catch (error) {}
    return { Users: [], Last: '', Applied: null }
  }
  // keep stores value, and tells whether the browser kept it.
  function keep(value) {
    try {
      var text = JSON.stringify(value)
      window.localStorage.setItem(storageKey, text)
      return window.localStorage.getItem(storageKey) === text
    } catch (error) {
      return false
    }
  }
  function ownTheme(value, user) {
    for (var i = 0; i < value.Users.length; i++) {
      if (value.Users[i] && value.Users[i][0] === user) return String(value.Users[i][1] || '')
    }
    return ''
  }
  function setOwnTheme(value, user, theme) {
    var users = [[user, theme]]
    for (var i = 0; i < value.Users.length && users.length < mostUsers; i++) {
      if (value.Users[i] && value.Users[i][0] !== user) users.push(value.Users[i])
    }
    value.Users = users
    value.Last = user
  }
  // savedUser is the user jellyfin-web's saved credentials sign in on the
  // server it used last, as server/user; "" when none.
  function savedUser() {
    try {
      var servers = JSON.parse(window.localStorage.getItem('jellyfin_credentials') || '{}').Servers || []
      var last = null
      for (var i = 0; i < servers.length; i++) {
        var server = servers[i]
        if (!server || !server.AccessToken || !server.UserId) continue
        if (!last || (server.DateLastAccessed || 0) > (last.DateLastAccessed || 0)) last = server
      }
      return last ? last.Id + '/' + last.UserId : ''
    } catch (error) {
      return ''
    }
  }
  var runsLumaaGlaass = null
  // shown is the theme the page shows for theme: LumaaGlaass where the
  // browser runs it, the Custom theme, else jellyfin-web's own look.
  function shown(theme) {
    if (theme === 'lumaaglaass' && runsLumaaGlaass === null) {
      runsLumaaGlaass = false
      try {
        runsLumaaGlaass =
          !!(window.CSS && CSS.supports && CSS.supports('selector(:has(a))')) &&
          typeof new Function('a', 'b', 'a ||= b?.c ?? 1; return a') === 'function'
      } catch (error) {}
    }
    if (theme === 'lumaaglaass' && runsLumaaGlaass) return theme
    return theme === 'custom' ? theme : 'none'
  }
  function filesOf(theme) {
    var files = themes.Themes[theme] || {}
    return { css: typeof files.Css === 'string' ? files.Css : '', js: typeof files.Js === 'string' ? files.Js : '' }
  }
  // cssOff tells whether the user, by their ID, "" signed out, turned off
  // the server's CSS in jellyfin-web's display settings, which jellyfin-web
  // keeps in the browser for them (appSettings).
  function cssOff(id) {
    try {
      return window.localStorage.getItem((id ? id + '-' : '') + 'disableCustomCss') === 'true'
    } catch (error) {
      return false
    }
  }
  function name(theme) {
    try {
      document.cookie = themes.Cookie + '=' + theme + '; path=/; samesite=lax'
    } catch (error) {}
  }
  // forgetKept deletes jellyfin-web's copy of its answers in IndexedDB, as
  // the fourteenth does.
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
  }
  function escaped(text) {
    return text.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;')
  }
  // copies are the style elements jellyfin-web shows the CSS at address in,
  // as Jellyfin's branding imports it.
  function copies(address) {
    var found = []
    var styles = document.getElementsByTagName('style')
    for (var i = 0; i < styles.length; i++) {
      if ((styles[i].textContent || '').indexOf("url('" + address + "')") >= 0) found.push(styles[i])
    }
    return found
  }
  // loaded tells whether the stylesheet a style element imports came.
  function loaded(style) {
    try {
      var rule = style.sheet && style.sheet.cssRules[0]
      return !!(rule && rule.styleSheet && rule.styleSheet.cssRules.length > 0)
    } catch (error) {
      return false
    }
  }
  var link = null
  var handing = null
  // handOver takes the script's stylesheet away once jellyfin-web shows
  // the same, loaded; it looks again every quarter of a second, for two
  // minutes.
  function handOver(tries) {
    clearTimeout(handing)
    if (!link) return
    var found = copies(link.getAttribute('href'))
    for (var i = 0; i < found.length; i++) {
      if (loaded(found[i])) {
        if (link.parentNode) link.parentNode.removeChild(link)
        link = null
        return
      }
    }
    if (tries < 480) {
      handing = setTimeout(
        quietly(function () {
          handOver(tries + 1)
        }),
        250
      )
    }
  }
  // setStylesheet shows the stylesheet at address, "" for none, in place
  // of the script's own: at the end of the page, after jellyfin-web's own
  // CSS, where jellyfin-web puts the branding's.
  function setStylesheet(address) {
    if (link && link.getAttribute('href') === address) return
    if (link && link.parentNode) link.parentNode.removeChild(link)
    link = null
    if (!address) return
    link = document.createElement('link')
    link.rel = 'stylesheet'
    link.id = linkId
    link.setAttribute('href', address)
    ;(document.body || document.head).appendChild(link)
    handOver(0)
  }

  var value = kept()
  var saved = savedUser()
  var user = saved || String(value.Last || '')
  var theme = shown((user && ownTheme(value, user)) || themes.Theme)
  var files = filesOf(theme)
  var off = cssOff(saved ? saved.slice(saved.indexOf('/') + 1) : '')
  // What the page shows: its theme, stylesheet and script.
  var applied = { theme: theme, css: files.css, js: files.js }
  name(theme)
  if (value.Applied !== files.css) forgetKept()
  value.Applied = files.css
  if (saved) value.Last = saved
  keep(value)
  // A load that shows the theme a reload was for ends that reload's mark.
  try {
    if (window.sessionStorage.getItem(reloadKey) === user + ' ' + theme) window.sessionStorage.removeItem(reloadKey)
  } catch (error) {}
  if (files.css && !off) {
    if (document.readyState === 'loading') {
      document.write('<link rel="stylesheet" id="' + linkId + '" href="' + escaped(files.css) + '">')
      link = document.getElementById(linkId)
      handOver(0)
    } else {
      setStylesheet(files.css)
    }
  }
  if (files.js) {
    if (document.readyState === 'loading') {
      document.write('<script src="' + escaped(files.js) + '" defer></' + 'script>')
    } else {
      var script = document.createElement('script')
      script.src = files.js
      document.head.appendChild(script)
    }
  }

  // The user whose theme was asked last, as server/user.
  var asked = ''
  // show shows theme, which the page did not show, for user, by their ID.
  function show(user, id, theme, value) {
    var files = filesOf(theme)
    if (applied.js) {
      // A theme's script cannot be taken back: the page loads again, and
      // shows the theme from its start, once for this user and theme.
      var reload = user + ' ' + theme
      var reloaded = ''
      try {
        reloaded = window.sessionStorage.getItem(reloadKey)
      } catch (error) {}
      if (reloaded === reload || !keep(value)) return
      try {
        window.sessionStorage.setItem(reloadKey, reload)
      } catch (error) {}
      name(theme)
      location.reload()
      return
    }
    // jellyfin-web's copy of the CSS before is turned off until it asks
    // again, and the next load forgets what it kept.
    name(theme)
    value.Applied = null
    keep(value)
    var before = copies(applied.css)
    for (var i = 0; i < before.length; i++) {
      if (before[i].sheet) before[i].sheet.disabled = true
    }
    setStylesheet(cssOff(id) ? '' : files.css)
    if (files.js) {
      var script = document.createElement('script')
      script.src = files.js
      document.head.appendChild(script)
    }
    applied = { theme: theme, css: files.css, js: files.js }
  }
  // applyReset forgets what this browser keeps for the user, as
  // server/user, by their ID, once they reset their preferences after the
  // reset it last applied, mark being the time of their last one:
  // jellyfin-web's settings kept in the browser for them ("<id>-<name>",
  // such as its display theme, custom CSS, screensaver and backdrops),
  // LumaaGlaass's copy of its settings and jellyfin-web's copy of its
  // answers. It tells whether the page must load again for jellyfin-web
  // to read the server's: never when the browser cannot keep the mark,
  // which would load it again and again.
  function applyReset(user, id, mark) {
    if (typeof mark !== 'string' || !mark) return false
    try {
      var storage = window.localStorage
      var marks = null
      try {
        marks = JSON.parse(storage.getItem(resetKey))
      } catch (error) {}
      if (Object.prototype.toString.call(marks) !== '[object Array]') marks = []
      var after = [[user, mark]]
      for (var i = 0; i < marks.length; i++) {
        if (!marks[i]) continue
        if (marks[i][0] === user && marks[i][1] === mark) return false
        if (marks[i][0] !== user && after.length < mostUsers) after.push(marks[i])
      }
      var names = []
      for (var j = 0; j < storage.length; j++) {
        var name = storage.key(j)
        if (name && name.indexOf(id + '-') === 0) names.push(name)
      }
      var lumaaGlaass = 'lumaaglaass-settings:' + user.replace('/', ':')
      if (storage.getItem(lumaaGlaass) !== null) names.push(lumaaGlaass)
      for (var k = 0; k < names.length; k++) storage.removeItem(names[k])
      forgetKept()
      var text = JSON.stringify(after)
      storage.setItem(resetKey, text)
      return names.length > 0 && storage.getItem(resetKey) === text
    } catch (error) {
      return false
    }
  }
  // follow asks the theme of the user jellyfin-web signed in, once for
  // each, and shows it when the page shows another. A reset of their
  // preferences the browser has not applied yet loads the page again.
  function follow() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId()
    if (!id) return
    var user = (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id
    if (user === asked) return
    asked = user
    api.getJSON(api.getUrl('Polyfin/WebTheme')).then(
      quietly(function (answer) {
        if (asked !== user || !answer || typeof answer.Theme !== 'string') return
        var value = kept()
        setOwnTheme(value, user, typeof answer.UserTheme === 'string' ? answer.UserTheme : '')
        if (applyReset(user, id, answer.PreferencesReset)) {
          keep(value)
          location.reload()
          return
        }
        var theme = shown(answer.Theme)
        if (theme !== applied.theme) {
          show(user, id, theme, value)
          return
        }
        keep(value)
        // The user who signed in may have turned off the server's CSS.
        if (cssOff(id)) setStylesheet('')
      }),
      function () {}
    )
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
  // A load signed in may move no route: jellyfin-web's user is looked for
  // every half second, for half a minute, until one is found.
  var looks = 0
  ;(function look() {
    quietly(follow)()
    if (!asked && looks++ < 60) setTimeout(look, 500)
  })()
