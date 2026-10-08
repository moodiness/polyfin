// Polyfin (MIT License). Served at /web/polyfin.js and loaded by the
// index.html of jellyfin-web, which Polyfin ships unmodified. It follows the
// client's routes for three things, adds a style for a fourth, mends a link
// for a fifth, closes a duplicate error for a sixth, and leaves libraries
// out of the menus for a seventh.
//
// First, it sends the pages of jellyfin-web that need a Jellyfin server's
// administration, which Polyfin does not have, to Polyfin's admin app. It
// watches the route only, never the page, so that a new jellyfin-web keeps
// working with it. jellyfin-web 12.2 routes after the "#", matched without
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
// details list now (Count). Polyfin also pushes it, as it changes, on the
// socket jellyfin-web keeps open, in a PolyfinVersions message whose Data
// names the page's identifier (ItemId): the script hears it through
// ApiClient's message event. It asks at once as the route changes, then
// every second while addons are pending, every 10 seconds once a push came,
// the socket then telling the changes,
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
//
// Third, on the same page, it holds the play buttons (.btnPlay, .btnResume,
// .btnReplay) until a version is known, as /Polyfin/Items/{id}/Versions and
// its push tell (Known, the placeholder left out). While none is known and
// addons are asked (Pending), they are disabled, greyed, with a spinner and
// the tooltip "Looking for sources…"; as soon as one is known, they are given
// back as jellyfin-web made them. Until the first answer, they are held as
// searching when the version menu lists only the placeholder, a single
// source under the title's own identifier; the first answer may come before
// the title's details asked any addon, so it never tells that no source is
// available. Once no addon is left to ask, they stay disabled and a line
// under them says that no source is available, with a
// "Try again" button: it posts to /Polyfin/Items/{id}/Versions/Search, which
// has Polyfin ask the title's addons again, then follows the versions anew;
// asked too soon, Polyfin answers 429 with Retry-After, and the button waits
// that long. A MutationObserver on the page holds the buttons again when
// jellyfin-web renders it anew. Its words follow jellyfin-web's language, the
// document's lang (English and French, English otherwise). It never changes
// the page during a video, and gives everything back when the route leaves
// the title.
//
// Fourth, it keeps a long name from running under the arrow of the menus of
// a title's page: stream addons name versions with every detail, and
// jellyfin-web leaves no room for the arrow in those menus. Their text ends
// with "…" before it; the open menu still lists whole names.
//
// Fifth, the title of a collection library's "Recently Added" row on the
// home page opens the library on the screen its user chose (Settings › Home
// › Default screen), as the library's links in the header and the menu do.
// jellyfin-web, which calls a library without a CollectionType mixed, opens
// every library's Suggestions from that title (#/mixed?…&tab=1), and a
// collection library has no suggestions of its own. The script drops the
// tab from such a link as it is pressed or clicked, before jellyfin-web
// follows it: pressed covers a link opened in a new tab, clicked one
// followed with the keyboard.
//
// Sixth, it closes the generic error jellyfin-web shows on top of a refused
// playback's own error. When a title's PlaybackInfo answers with an
// ErrorCode, such as the RateLimitExceeded of a user playing on as many
// devices as allowed, jellyfin-web 12.2 explains the code, then rejects the
// playback without a reason, and its rejection handler shows "There was an
// error processing the request" over it: the user meets the wrong error
// first. Of two alerts with the same title shown within a second of each
// other, the first still open, the script hides the second at once, with
// its backdrop, and closes it once it is open, as its button would.
//
// Seventh, it leaves out of jellyfin-web's menus the libraries chosen to be
// hidden there on the Libraries page of Polyfin's admin app, by an
// administrator for the server's libraries or by the user for their own;
// each keeps its row on the home page, and other apps list it. Once a user
// is signed in, it asks /Polyfin/UserViews, which lists the user's views as
// /UserViews does, in its order, each with HideInMenus, and keeps one style
// that hides the links of each such library: the buttons of the top bar
// (jellyfin-web 12.2's UserViewNav, MUI Buttons in the AppBar whose address
// holds the view's identifier), its More menu (#user-view-overflow-menu),
// the side menu of that layout (an MUI Drawer, used below 900 pixels wide
// instead of the buttons) and the legacy layout's side menu
// (.libraryMenuOptions, by data-itemid). Other links to the library, such as
// its home row's title, the tabs of its page or the search button on that
// page, are left alone. The top bar shows the first 3 views, 5 from 1200
// pixels wide and 8 from 1536 (MUI's lg and xl breakpoints), or all of them
// when only one more would go under More, its button listing the rest; the
// views come after config.json's menuLinks, none in the one Polyfin ships.
// Where every view More would list is hidden, its button is hidden too. It
// asks again whenever the user signed in changes, which a route change goes
// with; jellyfin-web may restore the user after the first route, so after
// each route change it looks for the user every second for 30 seconds.
// jellyfin-web draws its menus before the answer comes, so the script keeps
// the last answer in the browser's storage, with the user it was for, and
// applies it as soon as it starts: it runs from the page's head, before
// jellyfin-web's deferred scripts. A user's first load in a browser may
// still show those libraries until the answer comes; every later load hides
// them at once, then asks again and keeps the new answer. The rules kept
// stay while the user signed in is the one they were for; another user's
// are dropped as soon as that user is known, and signing out drops the
// rules but keeps the answer. Storage that fails, or holds anything else,
// counts as none. Any error just stops it.
;(function () {
  var menus = document.createElement('style')
  menus.textContent =
    'select.detailTrackSelect.emby-select-withcolor{padding-right:2.2em!important;text-overflow:ellipsis}'
  document.head.appendChild(menus)
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
  // The title whose versions are followed, the next time they are asked,
  // and how many times the script began asking, the latest only going on.
  var title = ''
  var timer = null
  var polls = 0
  // The title's last answer, the count the page was last updated or
  // reloaded for, and whether its details are being asked.
  var latest = null
  var updatedFor = 0
  var asking = false
  // Whether Polyfin pushed the title's progress, and the ApiClient whose
  // messages the script hears.
  var pushed = false
  var heard = null
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
  // listen hears the messages of jellyfin-web's ApiClient, as its message
  // event gives them: jellyfin-web's events keep an object's listeners in
  // its _callbacks, by event, and call each with the event then the message.
  function listen() {
    var api = window.ApiClient
    if (!api || api === heard) return
    heard = api
    var callbacks = (api._callbacks = api._callbacks || {})
    ;(callbacks.message = callbacks.message || []).push(function (event, message) {
      try {
        hear(message)
      } catch (error) {}
    })
  }
  // hear takes a pushed progress of the title followed as an answer.
  function hear(message) {
    if (!message || message.MessageType !== 'PolyfinVersions' || !message.Data) return
    var progress = message.Data
    if (!title || String(progress.ItemId).toLowerCase() !== title) return
    pushed = true
    unsure = false
    take(progress)
  }
  // take brings the page to an answer.
  function take(progress) {
    latest = progress
    if (early) early.disconnect()
    early = null
    hold()
    update(progress)
  }
  // The words the script adds to a page, in jellyfin-web's language, which
  // it gives the document; English for any other.
  var words = {
    en: { searching: 'Looking for sources…', none: 'No source is available for this title.', again: 'Try again' },
    fr: { searching: 'Recherche des sources…', none: 'Aucune source disponible pour ce titre.', again: 'Réessayer' },
  }
  function say(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (words[language] || words.en)[key]
  }
  // The play buttons held, each with what the script changed as it was
  // before and the tooltip it gave; the line telling that no source is
  // available; the page watched for jellyfin-web's renders, and the document
  // until the first answer; whether the latest answer may have come before
  // the title's details asked any addon; and when the title's addons may be
  // asked again.
  var held = []
  var note = null
  var watched = null
  var watcher = null
  var early = null
  var unsure = false
  var againAt = 0
  var againTimer = null
  // waiting tells what the play buttons of page wait for: "searching" while
  // no version is known and addons are asked, "none" once none is left to
  // ask, "" when a version is known or the item is not a movie or an
  // episode, which counts no source at all. The first answer for a title
  // opened is asked as the route changes, before its details may have asked
  // any addon: no source then counts as searching until the next answer.
  // Before any answer, the buttons wait as searching when the version menu
  // lists only the placeholder, as item details do while no version is
  // known: a single source under the title's own identifier.
  function waiting(page) {
    var progress = latest
    if (!progress) return placeholder(page) ? 'searching' : ''
    if (!(progress.Count >= 1) || typeof progress.Known !== 'number' || progress.Known > 0) return ''
    return progress.Pending > 0 || unsure ? 'searching' : 'none'
  }
  function placeholder(page) {
    var menu = page.querySelector('.selectSource')
    return !!menu && menu.options.length === 1 && String(menu.options[0].value).toLowerCase() === title
  }
  // hold brings the play buttons of the title page shown to the latest
  // answer: disabled, with a tooltip, greyed and a spinner while addons are
  // searched; disabled, with a line under them and a button to search again,
  // when no source is available; as jellyfin-web made them otherwise. It
  // watches the page, so that jellyfin-web's renders find them held again,
  // and changes only what differs. It never changes the page during a video.
  function hold() {
    try {
      var page = shownPage()
      if (!page) return
      var state = waiting(page)
      if (!state && !held.length && !note) return
      watch(page)
      if (!state) return release()
      style()
      var buttons = page.querySelectorAll('.btnPlay, .btnResume, .btnReplay')
      var tooltip = say(state === 'searching' ? 'searching' : 'none')
      held = held.filter(function (entry) {
        if (Array.prototype.indexOf.call(buttons, entry.button) >= 0) return true
        restore(entry)
        return false
      })
      Array.prototype.forEach.call(buttons, function (button) {
        holdButton(button, tooltip, state === 'searching')
      })
      if (state === 'none' && buttons.length) tell(buttons[0])
      else if (note) {
        note.remove()
        note = null
      }
    } catch (error) {}
  }
  function holdButton(button, tooltip, spinning) {
    var entry = null
    for (var i = 0; i < held.length; i++) if (held[i].button === button) entry = held[i]
    if (!entry) {
      entry = {
        button: button,
        title: button.getAttribute('title'),
        disabled: button.hasAttribute('disabled'),
        aria: button.getAttribute('aria-disabled'),
        tooltip: null,
      }
      held.push(entry)
    } else if (button.getAttribute('title') !== entry.tooltip) {
      // jellyfin-web rendered the page again, with its own tooltip.
      entry.title = button.getAttribute('title')
    }
    entry.tooltip = tooltip
    if (button.getAttribute('title') !== tooltip) button.setAttribute('title', tooltip)
    if (!button.hasAttribute('disabled')) button.setAttribute('disabled', '')
    if (button.getAttribute('aria-disabled') !== 'true') button.setAttribute('aria-disabled', 'true')
    if (!button.classList.contains('polyfinHeld')) button.classList.add('polyfinHeld')
    var spinner = button.querySelector('.polyfinSpinner')
    if (spinning && !spinner) {
      spinner = document.createElement('span')
      spinner.className = 'polyfinSpinner'
      spinner.setAttribute('aria-hidden', 'true')
      button.appendChild(spinner)
    } else if (!spinning && spinner) spinner.remove()
  }
  // restore gives a button back as jellyfin-web made it.
  function restore(entry) {
    var button = entry.button
    if (entry.title === null) button.removeAttribute('title')
    else if (button.getAttribute('title') === entry.tooltip) button.setAttribute('title', entry.title)
    if (!entry.disabled) button.removeAttribute('disabled')
    if (entry.aria === null) button.removeAttribute('aria-disabled')
    else button.setAttribute('aria-disabled', entry.aria)
    button.classList.remove('polyfinHeld')
    var spinner = button.querySelector('.polyfinSpinner')
    if (spinner) spinner.remove()
  }
  // release restores every button held and removes the line.
  function release() {
    var entries = held
    held = []
    entries.forEach(restore)
    if (note) note.remove()
    note = null
  }
  // tell shows, under the play buttons, that no source is available, with a
  // button that asks the addons again. The buttons share a row with the
  // title's name, which the line goes under, aligned with them.
  function tell(button) {
    var row = button.closest('.detailRibbon') || button.closest('.mainDetailButtons') || button.parentNode
    if (!note) {
      note = document.createElement('div')
      note.className = 'polyfinNoSource padded-left padded-right'
      note.appendChild(document.createElement('span'))
      var again = document.createElement('button')
      again.setAttribute('type', 'button')
      again.className = 'raised emby-button polyfinAgain'
      again.addEventListener('click', function () {
        try {
          searchAgain()
        } catch (error) {}
      })
      note.appendChild(again)
    }
    var text = note.firstChild
    var again = note.lastChild
    if (text.textContent !== say('none')) text.textContent = say('none')
    if (again.textContent !== say('again')) again.textContent = say('again')
    var wait = Date.now() < againAt
    if (again.hasAttribute('disabled') !== wait) {
      if (wait) again.setAttribute('disabled', '')
      else again.removeAttribute('disabled')
    }
    if (row.nextSibling !== note) row.parentNode.insertBefore(note, row.nextSibling)
  }
  // searchAgain has Polyfin ask the title's addons again, then follows its
  // versions anew. Asked too soon, the button waits as long as Polyfin says.
  function searchAgain() {
    var id = title
    if (!id || Date.now() < againAt) return
    var api = window.ApiClient
    api.ajax({ type: 'POST', url: api.getUrl('Polyfin/Items/' + id + '/Versions/Search'), dataType: 'json' }).then(
      function (progress) {
        if (title !== id) return
        unsure = false
        take(progress)
        poll(id, 1000)
      },
      function (response) {
        var seconds = Number(response && response.headers && response.headers.get('Retry-After'))
        if (title !== id || !(seconds > 0)) return
        againAt = Date.now() + seconds * 1000
        hold()
        clearTimeout(againTimer)
        againTimer = setTimeout(hold, seconds * 1000)
      }
    )
  }
  // watch has jellyfin-web's renders of the page shown find its buttons held.
  function watch(page) {
    if (watched === page) return
    if (watcher) watcher.disconnect()
    watched = page
    watcher = new MutationObserver(hold)
    var changes = { childList: true, subtree: true, attributes: true, attributeFilter: ['title', 'disabled', 'class'] }
    watcher.observe(page, changes)
  }
  // style adds the look of the buttons held and of the line, once.
  function style() {
    if (document.getElementById('polyfinStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinStyle'
    sheet.textContent =
      '.polyfinHeld{position:relative;opacity:.45;cursor:default}' +
      '.polyfinSpinner{position:absolute;top:50%;left:50%;width:1.1em;height:1.1em;margin:-.55em 0 0 -.55em;box-sizing:border-box;' +
      'border:.15em solid currentColor;border-right-color:transparent;border-radius:50%;animation:polyfinSpin .8s linear infinite}' +
      '@keyframes polyfinSpin{to{transform:rotate(360deg)}}' +
      '.polyfinNoSource{display:flex;flex-wrap:wrap;justify-content:flex-end;align-items:center;gap:.5em 1em;margin:.5em 0}' +
      '.polyfinAgain[disabled]{opacity:.45;cursor:default}'
    document.head.appendChild(sheet)
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
    pushed = false
    if (picking) picking.disconnect()
    picking = null
    release()
    if (watcher) watcher.disconnect()
    watcher = null
    watched = null
    if (early) early.disconnect()
    early = null
    unsure = true
    clearTimeout(againTimer)
    againTimer = null
    againAt = 0
    if (!id) return
    // Until the first answer, the page is looked for as jellyfin-web renders
    // it, in case it lists only the placeholder.
    if (document.body) {
      early = new MutationObserver(hold)
      early.observe(document.body, { childList: true, subtree: true })
    }
    hold()
    poll(id, 0)
  }
  // poll asks for the title's versions for 90 seconds, from scratch, first
  // after delay, then every second, or every 10 once a push came.
  function poll(id, delay) {
    clearTimeout(timer)
    timer = null
    var round = ++polls
    var until = Date.now() + 90000
    var answers = 0
    function ask() {
      timer = null
      if (title !== id || round !== polls || Date.now() >= until) return
      var api = window.ApiClient
      // jellyfin-web may not have made its client yet, on a page opened at
      // a title's address.
      if (!api) return next()
      listen()
      api
        .getJSON(api.getUrl('Polyfin/Items/' + id + '/Versions'))
        .then(function (progress) {
          if (title !== id || round !== polls) return
          answers++
          if (answers > 1) unsure = false
          take(progress)
          // The page asks for its details as the route changes: the first
          // answers may come before Polyfin asks any addon.
          if (progress.Pending > 0 || answers < 3) next()
        })
        .catch(function () {})
    }
    function next() {
      timer = setTimeout(attempt, pushed ? 10000 : 1000)
    }
    function attempt() {
      try {
        ask()
      } catch (error) {}
    }
    if (delay) timer = setTimeout(attempt, delay)
    else attempt()
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
    try {
      followViews()
    } catch (error) {}
  }
  // The user signed in whose views were asked for, the style hiding the
  // views they hide from the menus, and until when the script looks for a
  // user signed in after a route change.
  var viewer = ''
  var viewSheet = null
  var viewTimer = null
  var viewsUntil = 0
  // The views the top bar shows before its More button, at the widths of
  // MUI's breakpoints that UserViewNav follows: below lg (1200px), from lg
  // to xl (1536px), from xl. The ranges end as MUI's own queries do.
  var barWidths = [
    [3, '(max-width:1199.95px)'],
    [5, '(min-width:1200px) and (max-width:1535.95px)'],
    [8, '(min-width:1536px)'],
  ]
  // Where the last answer is kept, as {user, items}: the user it was for
  // and its Items, all of them, in order, as the More button needs.
  var savedViewsKey = 'polyfinUserViews'
  // savedViews returns the answer kept, or null when there is none, storage
  // fails or it holds anything else.
  function savedViews() {
    try {
      var saved = JSON.parse(window.localStorage.getItem(savedViewsKey))
      if (saved && typeof saved.user === 'string' && saved.user && Array.isArray(saved.items)) return saved
    } catch (error) {}
    return null
  }
  function saveViews(user, items) {
    try {
      window.localStorage.setItem(savedViewsKey, JSON.stringify({ user: user, items: items }))
    } catch (error) {}
  }
  // The user whose views the style hides now: the answer kept hides them
  // as the script starts, before jellyfin-web draws its menus.
  var hiddenFor = ''
  try {
    var saved = savedViews()
    if (saved) {
      hideViews(saved.items)
      hiddenFor = saved.user
    }
  } catch (error) {}
  // followViews asks for the views of the user signed in when the user
  // changes (see the header), forgetting those of the user before.
  function followViews() {
    viewsUntil = Date.now() + 30000
    lookForViewer()
  }
  function lookForViewer() {
    clearTimeout(viewTimer)
    viewTimer = null
    var api = window.ApiClient
    var user = (api && api.getCurrentUserId()) || ''
    if (user !== viewer) {
      viewer = user
      // The rules kept stay for their own user until the answer comes.
      if (user !== hiddenFor) {
        hiddenFor = ''
        hideViews([])
      }
      if (user) askViews(api, user)
    }
    if (!user && Date.now() < viewsUntil) viewTimer = setTimeout(quietly(lookForViewer), 1000)
  }
  // askViews asks for the views of user, hides them and keeps the answer,
  // unless another user signed in meanwhile.
  function askViews(api, user) {
    api
      .getJSON(api.getUrl('Polyfin/UserViews'))
      .then(function (views) {
        if (viewer !== user) return
        var items = (views && views.Items) || []
        hideViews(items)
        hiddenFor = user
        saveViews(user, items)
      })
      .catch(function () {})
  }
  // hideViews has the style hide the links of the views with HideInMenus,
  // and the More button at the widths where it would list only such views.
  // An identifier goes in as a JSON string, whose escaped quotes and
  // backslashes CSS reads the same way.
  function hideViews(views) {
    function hides(view) {
      return !!view && view.HideInMenus === true
    }
    var rules = ''
    views.forEach(function (view) {
      if (!hides(view)) return
      var id = JSON.stringify(String(view.Id))
      rules +=
        '.MuiAppBar-root a.MuiButton-root[href*=' + id + '],' +
        '#user-view-overflow-menu a[href*=' + id + '],' +
        '.MuiDrawer-root a[href*=' + id + '],' +
        '.libraryMenuOptions a[data-itemid=' + id + ']{display:none!important}'
    })
    barWidths.forEach(function (width) {
      var shown = width[0]
      if (views.length > shown + 1 && views.slice(shown).every(hides)) {
        rules += '@media ' + width[1] + '{.MuiAppBar-root button[aria-controls="user-view-overflow-menu"]{display:none!important}}'
      }
    })
    if (!viewSheet) {
      if (!rules) return
      viewSheet = document.createElement('style')
      document.head.appendChild(viewSheet)
    }
    if (viewSheet.textContent !== rules) viewSheet.textContent = rules
  }
  // The link pressed or clicked, if it opens a collection library on
  // Suggestions, without the tab: the library opens on its chosen screen.
  function chosenScreen(event) {
    var link = event.target && event.target.closest && event.target.closest('a[href]')
    var href = link ? link.getAttribute('href') : ''
    if (!/^#!?\/mixed\?/i.test(href)) return
    var chosen = href.replace(/([?&])tab=1(&|$)/, function (match, before, after) {
      return after ? before : ''
    })
    if (chosen !== href) link.setAttribute('href', chosen)
  }
  // The last alert shown, and how close a second alert with the same title
  // follows it to be the duplicate one.
  var alerted = null
  var duplicateWithin = 1000
  // closeDuplicate hides and closes a dialog container that repeats, as an
  // alert of the same title opened just before and still open, the alert
  // before it (see the header).
  function closeDuplicate(container) {
    var dialog = container.querySelector && container.querySelector('.dialog')
    var title = dialog && dialog.querySelector('.formDialogHeaderTitle')
    var buttons = dialog ? dialog.querySelectorAll('.btnOption') : []
    if (!title || buttons.length !== 1) return
    var shown = { container: container, title: title.textContent, at: Date.now() }
    var before = alerted
    if (!before || !before.container.isConnected || shown.at - before.at > duplicateWithin || before.title !== shown.title) {
      alerted = shown
      return
    }
    container.style.visibility = 'hidden'
    var backdrop = container.previousElementSibling
    if (backdrop && backdrop.classList.contains('dialogBackdrop')) backdrop.style.visibility = 'hidden'
    // Closed once open, as a click on its button closes it, so that
    // jellyfin-web undoes its opening, history entry included; at most 2
    // seconds later all the same.
    var waited = 0
    ;(function close() {
      if (!container.isConnected) return
      if (dialog.classList.contains('opened') || waited >= 2000) {
        buttons[0].click()
        return
      }
      waited += 50
      setTimeout(close, 50)
    })()
  }
  // Alerts are dialog containers jellyfin-web adds to the body, which the
  // script, loaded in the head, observes once it exists.
  function watchDialogs() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(
      quietly(function (records) {
        for (var i = 0; i < records.length; i++) {
          var added = records[i].addedNodes || []
          for (var j = 0; j < added.length; j++) {
            if (added[j].classList && added[j].classList.contains('dialogContainer')) closeDuplicate(added[j])
          }
        }
      }),
    ).observe(document.body, { childList: true })
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
  // Both run before jellyfin-web follows the link.
  addEventListener('mousedown', quietly(chosenScreen), true)
  addEventListener('click', quietly(chosenScreen), true)
  if (document.body) watchDialogs()
  else addEventListener('DOMContentLoaded', watchDialogs)
  follow()
})()
