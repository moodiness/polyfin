// Polyfin (MIT License). Served at /web/polyfin.js and loaded by the
// index.html of jellyfin-web, which Polyfin ships unmodified. It follows the
// client's routes for two things.
//
// First, it sends the pages of jellyfin-web that need a Jellyfin server's
// administration, which Polyfin does not have, to Polyfin's admin app. It
// watches the route only, never the page, so that a new jellyfin-web keeps
// working with it. jellyfin-web 12.1 routes after the "#", matched without
// regard to case as its router does, "#!/" being an older form:
//   #/dashboard and every #/dashboard/… page (users, libraries, playback,
//     Live TV, devices, activity, logs, API keys, scheduled tasks, plugins…);
//   #/metadata, the metadata manager;
//   #/configurationpage, plugin configuration pages;
//   #/wizard/…, the startup wizard: Polyfin is set up in its admin app.
// A dashboard page with a match in the admin app opens that page; any other
// opens the admin app's home.
//
// Second, on a title's page (#/details?id=…), it adds the versions of the
// addons that answer after the page opened. Item details list the versions
// Polyfin knows when they are asked, while it asks the other addons in the
// background, and asks each addon again 10 seconds after its first answer,
// then 30 seconds later while its answers grow; /Polyfin/Items/{id}/Versions
// tells how many addons it still asks (Pending) and how many versions
// details list now (Count). It asks every second while addons are pending,
// for at most 90 seconds: a first answer, which takes at most 15 seconds,
// and both follow-ups, 10 and 30 seconds later and as long each, end within
// 85. When the page lists fewer versions than Count, or a different number
// once no addon is pending, it asks for the title's details and lists their
// versions in the page's version menu, in place, keeping the version picked.
// It reloads the page's details instead, as jellyfin-web does when the page
// is shown again, when the version picked is gone or no longer the same
// version, since the track menus describe it. It never changes the page
// while a video plays or while the version menu has focus, where it could be
// open: it tries again at the next answer, or as soon as the menu loses
// focus. Any error just stops it.
;(function () {
  var adminRoute = /^#!?\/(dashboard|metadata|configurationpage|wizard)(?:[/?#]|$)/i
  var titleRoute = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  // The first prefix of the dashboard page's path that matches wins.
  var pages = [
    ['users', 'users'],
    ['devices', 'users'],
    ['libraries', 'libraries'],
    ['livetv/recordings', 'settings/recordings'],
    ['livetv', 'live-tv'],
    ['playback/transcoding', 'settings/conversion'],
    ['playback/resume', 'settings/content'],
    ['playback/trickplay', 'settings/thumbnails'],
    ['playback', 'settings/playback'],
    ['branding', 'settings/web-player'],
    ['settings', 'settings/general'],
    ['backups', 'settings/backups'],
    ['logs', 'system/logs'],
    ['tasks', 'system/schedule'],
    ['keys', 'system/api-keys'],
    ['apikeys', 'system/api-keys'],
  ]
  // The admin page for a route: "" (home) unless it is a dashboard page with a match. A user's
  // dashboard pages name the user with ?userId=, which opens that user.
  function target(hash) {
    if (!/^#!?\/dashboard(?:[/?#]|$)/i.test(hash)) return ''
    var path = hash.replace(/^#!?\/dashboard\/?/i, '').split(/[?#]/)[0].toLowerCase()
    var user = /[?&]userId=([0-9a-f-]+)/i.exec(hash)
    // The admin app names users by 32 lowercase hexadecimal digits.
    if (user && /^users(\/|$)/.test(path)) return 'users/' + user[1].replace(/-/g, '').toLowerCase()
    for (var i = 0; i < pages.length; i++) {
      var prefix = pages[i][0]
      if (path === prefix || path.indexOf(prefix + '/') === 0) return pages[i][1]
    }
    return ''
  }
  // The title whose versions are followed, and the next time they are asked.
  var title = ''
  var timer = null
  // The title's last answer, the count the page was last updated or
  // reloaded for, and whether its details are being asked.
  var latest = null
  var updatedFor = 0
  var asking = false
  // The options this script added to the version menu, and the size of the
  // version each option stood for when the script last listed it.
  // jellyfin-web fills the menu with new options, which forgets both.
  var added = new WeakSet()
  var sizes = new WeakMap()
  // The version picked among those added, until jellyfin-web lists it.
  var picking = null
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // reload reloads a title page's details as jellyfin-web does when the page
  // is shown again: it asks for them, lists their versions keeping the one
  // picked, and fills the track menus.
  function reload(page) {
    page.dispatchEvent(new CustomEvent('viewbeforehide', { detail: {} }))
    page.dispatchEvent(new CustomEvent('viewshow', { detail: { isRestored: false } }))
  }
  // stale tells whether an option no longer stands for a source with its
  // identifier: jellyfin-web names an option after its source, and the
  // title's own identifier passes from the placeholder to the first version,
  // then to whichever version comes first.
  function stale(option, source) {
    return option.textContent !== source.Name || (sizes.has(option) && sizes.get(option) !== source.Size)
  }
  // place lists sources in the version menu, in their order, keeping the
  // version picked, and tells whether it could. An option jellyfin-web
  // listed is kept while it stands for the same source, as jellyfin-web
  // knows that source's tracks.
  function place(menu, sources) {
    var picked = menu.options[menu.selectedIndex]
    var kept = {}
    for (var i = 0; i < menu.options.length; i++) kept[menu.options[i].value] = menu.options[i]
    var found = false
    var options = sources.map(function (source) {
      var option = kept[source.Id]
      if (option && stale(option, source)) {
        if (option === picked) return null
        option = null
      }
      if (!option) {
        option = document.createElement('option')
        option.value = source.Id
        option.textContent = source.Name
        added.add(option)
      }
      if (option === picked) found = true
      sizes.set(option, source.Size)
      return option
    })
    if (!found || options.indexOf(null) >= 0) return false
    options.forEach(function (option, i) {
      if (menu.options[i] !== option) menu.insertBefore(option, menu.options[i] || null)
    })
    while (menu.options.length > options.length) menu.removeChild(menu.options[options.length])
    menu.value = picked.value
    var container = menu.closest('.selectSourceContainer')
    if (container) container.classList.toggle('hide', options.length < 2)
    return true
  }
  // update brings the title page to the versions Polyfin knows when an
  // answer tells it lists fewer, or a different number once no addon is
  // pending. A hidden or empty version menu counts as one.
  function update(progress) {
    var id = title
    var page = shownPage()
    if (!id || !page || asking || progress.Count < 1 || progress.Count === updatedFor) return
    var menu = page.querySelector('.selectSource')
    var listed = !menu || menu.closest('.hide') ? 1 : Math.max(menu.options.length, 1)
    if (progress.Count < listed ? progress.Pending > 0 : progress.Count === listed) return
    if (!menu) {
      reload(page)
      updatedFor = progress.Count
      return
    }
    if (document.activeElement === menu) return
    var api = window.ApiClient
    asking = true
    api.getItem(api.getCurrentUserId(), id).then(
      function (item) {
        asking = false
        if (title !== id || shownPage() !== page || page.querySelector('.selectSource') !== menu) return
        if (document.activeElement === menu) return
        if (!place(menu, item.MediaSources || [])) reload(page)
        updatedFor = progress.Count
      },
      function () {
        asking = false
      }
    )
  }
  function followVersions() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === title) return
    clearTimeout(timer)
    timer = null
    title = id
    latest = null
    updatedFor = 0
    if (picking) picking.disconnect()
    picking = null
    if (!id) return
    var until = Date.now() + 90000
    var answers = 0
    function ask() {
      timer = null
      if (title !== id || Date.now() > until) return
      var api = window.ApiClient
      api
        .getJSON(api.getUrl('Polyfin/Items/' + id + '/Versions'))
        .then(function (progress) {
          if (title !== id) return
          answers++
          latest = progress
          update(progress)
          // The page asks for its details as the route changes: the first
          // answers may come before Polyfin asks any addon.
          if (progress.Pending > 0 || answers < 3) next()
        })
        .catch(function () {})
    }
    function next() {
      timer = setTimeout(function () {
        try {
          ask()
        } catch (error) {}
      }, 1000)
    }
    next()
  }
  // jellyfin-web keeps its own copy of the versions it listed, and fills the
  // track menus from it as soon as a version is picked, before it asks for
  // that version's details: a version this script added is not in it. Such a
  // pick goes to jellyfin-web only once it lists that version: the page's
  // details are reloaded, which keeps the pick, and it is made again when
  // jellyfin-web has filled the menu anew.
  function pick(event) {
    var menu = event.target
    var option = menu && menu.options && menu.options[menu.selectedIndex]
    var page = option && added.has(option) && menu.closest('.itemDetailPage')
    if (!page) return
    event.stopPropagation()
    if (picking) picking.disconnect()
    var value = menu.value
    var observer = new MutationObserver(function () {
      var picked = menu.options[menu.selectedIndex]
      // This script's own changes leave the pick among its options.
      if (picked && added.has(picked)) return
      observer.disconnect()
      if (picking === observer) picking = null
      if (picked && picked.value === value) menu.dispatchEvent(new Event('change', { bubbles: true }))
    })
    picking = observer
    observer.observe(menu, { childList: true })
    reload(page)
  }
  // A version menu that had focus is updated as soon as it loses it.
  function leave(event) {
    var menu = event.target
    if (latest && menu && menu.matches && menu.matches('.selectSource')) update(latest)
  }
  // quietly runs a listener, an error leaving the event to jellyfin-web.
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  function follow() {
    if (adminRoute.test(location.hash)) {
      // Replaced, so that Back returns to the page before.
      location.replace(new URL('../admin/' + target(location.hash), location.href).href)
      return
    }
    try {
      followVersions()
    } catch (error) {}
  }
  // The router moves through history.pushState and replaceState, which
  // fire no event; links and typed addresses fire hashchange.
  ;['pushState', 'replaceState'].forEach(function (name) {
    var original = history[name]
    history[name] = function () {
      var result = original.apply(this, arguments)
      follow()
      return result
    }
  })
  addEventListener('hashchange', follow)
  addEventListener('popstate', follow)
  // Both run before jellyfin-web's own listeners on the version menu.
  addEventListener('change', quietly(pick), true)
  addEventListener('blur', quietly(leave), true)
  follow()
})()
