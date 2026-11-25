// Part 21: the badges of the version picked or played.
//
// It shows badges for the version picked or played, while
// the administrator leaves them on (Settings › Web player › Version
// badges): a row of pills, each with a short text (4K, DV, HEVC, Atmos,
// 7.1, IMAX, FR…), its words in full as its tooltip and its aria-label,
// and a border, in the page's font, without icon or animation; the row
// wraps where it does not fit, and the TV layout gets larger pills.
// Polyfin writes the pills, in the document's language, from the
// version's analysis once it has one, else from the addon's labels.
// On a title's page, as the route opens it, it asks
// /Polyfin/Items/{id}/Badges, every second until jellyfin-web made its
// client, 10 times at most: Show, the setting, and the versions as item
// details list them, each with its Id, its Name, the addon's own, and its
// Badges. The row of the version picked in the version menu (the source
// of its value, the first while the menu has none) goes right under the
// menu: after its container (.selectSourceContainer, in .trackSelections)
// where its label sits beside it, as in jellyfin-web's own look, in line
// with the menu; last in that container where a theme lays the track
// menus out itself, the label over the menu, as LumaaGlaass does in a
// grid, where a row after the container would move. While the version
// menu is hidden, as it is for one version, the row goes last in the
// track menus' block (.trackSelections), across its grid if it has one.
// The menu's tooltip names the version picked, as the addon names it,
// even while the setting is off, which shows no pill. A
// pick, as the menu closes, brings the row of the version picked; the
// page's changes put it back as jellyfin-web renders the page anew. It
// asks again, a second later, when a value of the menu is not in the
// answer, once for each, or when a PolyfinVersions push for the title
// tells another number of versions or new tracks. It changes nothing
// while the version menu has focus, but at a pick, nor during a video,
// and takes the row and the tooltip off as the route leaves the title.
// During a video, as it starts playing (a new element or address), and
// once jellyfin-web 12.4's controls show (.videoOsdBottom, without
// videoOsdBottom-hidden), it asks /Polyfin/Playback/Badges, the version
// this device plays, and again every 2 seconds, 5 times at most, while
// the answer names no title: jellyfin-web tells the server of a play a
// moment after the video starts. The row goes after the title's heading
// of the controls (.osdTitle, in .osdMainTextContainer), its pills on a
// dark ground, so that they read over a bright video, and shows and hides
// with the controls; it goes as the video ends or loses its source. Any
// error just stops it.
//
// Anchors in jellyfin-web:
//   route: details
//   selector: .videoPlayerContainer
//   selector: .page.itemDetailPage:not(.hide)
//   selector: .selectSource
//   selector: .trackSelections
//   selector: .selectSourceContainer
//   selector: .videoOsdBottom
//   selector: .osdTitle
//   selector: .layout-mobile
//   selector: .layout-tv
//   class: videoOsdBottom-hidden
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.subscribe
//   property: _callbacks

  var titleRoute = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // marks is a WeakSet of the page's elements, or, before Chrome 36, a
  // property kept on each element, as the other parts keep theirs.
  function marks(name) {
    if (typeof WeakSet === 'function') return new WeakSet()
    var key = '__polyfin' + name
    return {
      has: function (element) {
        return Object.prototype.hasOwnProperty.call(element, key)
      },
      add: function (element) {
        element[key] = true
        return this
      },
    }
  }
  // attached tells whether node is in the page, as isConnected does from
  // Chrome 54 on.
  function attached(node) {
    if (typeof node.isConnected === 'boolean') return node.isConnected
    return document.documentElement.contains(node)
  }
  // address is the address of a path of Polyfin's, asked in jellyfin-web's
  // language, which it gives the document.
  function address(api, path) {
    var language = String(document.documentElement.lang || '')
    return api.getUrl(path, language ? { language: language } : undefined)
  }
  // The title whose badges are followed, its last answer and the title it
  // is for, the title being asked and whether to ask again once it
  // answers, the ask to come, and the option value the script asked again
  // for.
  var title = ''
  var answer = null
  var answered = ''
  var asking = ''
  var again = false
  var timer = null
  var unknown = null
  // The rows drawn on the title's page and in the video's controls, each
  // with the badges it shows; the menu whose tooltip the script wrote; the
  // menus whose picks and focus it follows; the ApiClient whose messages it
  // hears; and whether it follows the page's changes.
  var pageRow = null
  var pageKey = ''
  var osdRow = null
  var osdKey = ''
  var titled = null
  var menus = marks('BadgeMenu')
  var heard = null
  var watching = false
  // The video played, as { video, src, asked, answer, timer }, and the
  // controls whose showing the script watches.
  var play = null
  var osdWatched = null
  var osdWatcher = null
  // follow follows the title the route opens: it asks for its badges, and
  // draws at once those it has for a title the route comes back to, as
  // after a video. It takes the row and the tooltip off the page the route
  // leaves, but for a video: the page shows them as it is shown again.
  function follow() {
    listen()
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === title) return
    title = id
    again = false
    unknown = null
    clearTimeout(timer)
    timer = null
    if (answered !== id) {
      answer = null
      answered = ''
    }
    if (!document.querySelector('.videoPlayerContainer')) {
      undraw()
      if (titled) titled.removeAttribute('title')
      titled = null
    }
    watch()
    if (!id) return
    place(false)
    ask(id, 0)
  }
  // ask asks for the badges of the title's versions, every second until
  // jellyfin-web made its client, 10 times at most; an ask while one is
  // awaited comes after it.
  function ask(id, tries) {
    timer = null
    if (title !== id) return
    var api = window.ApiClient
    if (!api || !api.getCurrentUserId()) {
      if (tries < 10) {
        timer = setTimeout(
          quietly(function () {
            ask(id, tries + 1)
          }),
          1000
        )
      }
      return
    }
    listen()
    if (asking === id) {
      again = true
      return
    }
    asking = id
    api.getJSON(address(api, 'Polyfin/Items/' + id + '/Badges')).then(
      quietly(function (got) {
        if (asking === id) asking = ''
        if (title !== id) return
        if (again) {
          again = false
          ask(id, 10)
        }
        if (!got || !Array.isArray(got.Versions)) return
        answer = got
        answered = id
        watch()
        place(false)
      }),
      function () {
        if (asking === id) asking = ''
      }
    )
  }
  // later asks again a second later, gathering the reasons that come
  // meanwhile.
  function later() {
    var id = title
    if (!id || timer) return
    timer = setTimeout(
      quietly(function () {
        ask(id, 10)
      }),
      1000
    )
  }
  // listen hears Polyfin's pushes of the title's versions on jellyfin-web's
  // socket, as the second part does.
  function listen() {
    var api = window.ApiClient
    if (!api || api === heard) return
    heard = api
    var heardVersions = quietly(hear)
    if (typeof api.subscribe === 'function') {
      api.subscribe(['PolyfinVersions'], heardVersions)
    } else {
      var callbacks = (api._callbacks = api._callbacks || {})
      ;(callbacks.message = callbacks.message || []).push(function (event, message) {
        heardVersions(message)
      })
    }
  }
  // hear asks again when a push tells the title's page lists another
  // number of versions than the answer, or a version's tracks changed.
  function hear(message) {
    if (!message || message.MessageType !== 'PolyfinVersions' || !message.Data || !answer) return
    var progress = message.Data
    if (String(progress.ItemId).toLowerCase() !== title) return
    if (progress.Tracks || progress.Count !== answer.Versions.length) later()
  }
  // watch follows the page's changes while there is a row to place: a
  // title's badges came, or a video plays.
  function watch() {
    var on = !!(title && answer) || !!play
    if (on === watching) return
    watching = on
    if (on) document.addEventListener('polyfinpagechange', looked)
    else document.removeEventListener('polyfinpagechange', looked)
  }
  var looked = quietly(function () {
    place(false)
    lookOsd()
  })
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // picked is the version picked in the menu: the source of its value,
  // the first without a menu or a value. A value the answer does not know
  // asks again, once for each.
  function picked(menu) {
    var versions = answer.Versions
    if (!menu || !menu.value) return versions[0] || null
    for (var i = 0; i < versions.length; i++) {
      if (versions[i] && versions[i].Id === menu.value) return versions[i]
    }
    if (unknown !== menu.value) {
      unknown = menu.value
      later()
    }
    return null
  }
  // place writes the tooltip of the version menu and puts the row of the
  // version picked under it (see where), or in the track menus' block when
  // it is hidden. It changes nothing while the menu has focus, where it
  // could be open, but at a pick, which closed it: the menu losing focus
  // places them.
  function place(picking) {
    if (!title || !answer) return
    var page = shownPage()
    if (!page) return
    var menu = page.querySelector('.selectSource')
    if (menu && !menus.has(menu)) {
      menus.add(menu)
      menu.addEventListener(
        'change',
        quietly(function () {
          place(true)
        })
      )
      menu.addEventListener(
        'blur',
        quietly(function () {
          place(false)
        })
      )
    }
    if (menu && !picking && document.activeElement === menu) return
    var version = picked(menu)
    if (menu) {
      var option = menu.options && menu.options[menu.selectedIndex]
      var name = (version && version.Name) || (option ? option.textContent : '')
      if (name && menu.getAttribute('title') !== name) menu.setAttribute('title', name)
      titled = menu
    }
    var form = page.querySelector('.trackSelections')
    var badges = answer.Show && version ? shown(version.Badges) : []
    if (!badges.length || !form || form.classList.contains('hide')) return undraw()
    var container = menu && menu.closest('.selectSourceContainer')
    var mode = container && !container.classList.contains('hide') ? where(container) : 'tracks'
    var parent = mode === 'beside' ? container.parentNode : mode === 'inside' ? container : form
    var placed = pageRow && pageRow.parentNode === parent
    if (placed) placed = mode === 'beside' ? container.nextSibling === pageRow : !pageRow.nextSibling
    var key = mode + ' ' + JSON.stringify(badges)
    if (placed && pageKey === key) return
    undraw()
    var classes = { beside: 'polyfinVersionBadgesMenu', inside: 'polyfinVersionBadgesIn', tracks: 'polyfinVersionBadgesTracks' }
    pageRow = row(badges, classes[mode])
    pageKey = key
    parent.insertBefore(pageRow, mode === 'beside' ? container.nextSibling : null)
  }
  // where tells where the row goes under the version menu, as the page
  // lays its container out: "beside" when the menu's label sits beside it,
  // as jellyfin-web's own look does (a flex row), the row then going after
  // the container, in line with the menu; "inside" otherwise, as a theme
  // that puts the label over the menu and lays the track menus out
  // itself, such as LumaaGlaass (a column, in a grid), the row then going
  // last in the container, under the menu, where the theme's layout keeps
  // it.
  function where(container) {
    var style = typeof getComputedStyle === 'function' ? getComputedStyle(container) : null
    if (!style) return 'beside'
    return /flex/.test(style.display) && !/column/.test(style.flexDirection) ? 'beside' : 'inside'
  }
  function undraw() {
    if (pageRow && pageRow.parentNode) pageRow.parentNode.removeChild(pageRow)
    pageRow = null
    pageKey = ''
  }
  // shown are the badges that show: those with a text.
  function shown(badges) {
    if (!Array.isArray(badges)) return []
    return badges.filter(function (badge) {
      return badge && typeof badge.Text === 'string' && badge.Text !== ''
    })
  }
  // row makes a list of badges: each a pill with its text, its label in
  // full words for screen readers and as its tooltip, and its kind.
  function row(badges, className) {
    style()
    var list = document.createElement('ul')
    list.className = 'polyfinVersionBadges ' + className
    list.setAttribute('role', 'list')
    badges.forEach(function (badge) {
      var label = typeof badge.Label === 'string' && badge.Label ? badge.Label : badge.Text
      var pill = document.createElement('span')
      pill.className = 'polyfinVersionBadge'
      pill.setAttribute('data-kind', String(badge.Kind || ''))
      pill.setAttribute('aria-label', label)
      pill.setAttribute('title', label)
      pill.textContent = badge.Text
      var item = document.createElement('li')
      item.appendChild(pill)
      list.appendChild(item)
    })
    return list
  }
  // played follows the videos that start: a new element, or a new address
  // for the same, is a new play, whose badges are asked once the video's
  // controls show.
  function played(event) {
    var video = event.target
    if (!video || video.tagName !== 'VIDEO') return
    var src = String(video.currentSrc || video.src || '')
    if (play && play.video === video && play.src === src) return
    stop()
    play = { video: video, src: src, asked: 0, answer: null, timer: null }
    watch()
    lookOsd()
  }
  // ended takes the row of a video off as it ends or loses its source.
  function ended(event) {
    if (!play || event.target !== play.video) return
    stop()
    watch()
  }
  function stop() {
    if (play) clearTimeout(play.timer)
    play = null
    if (osdRow && osdRow.parentNode) osdRow.parentNode.removeChild(osdRow)
    osdRow = null
    osdKey = ''
    if (osdWatcher) osdWatcher.disconnect()
    osdWatcher = null
    osdWatched = null
  }
  // hidden tells whether node or an element it is in is hidden.
  function hidden(node) {
    for (var at = node; at && at.classList; at = at.parentNode) {
      if (at.classList.contains('hide')) return true
    }
    return false
  }
  // shownOsd returns the video's controls (jellyfin-web 12.4's
  // .videoOsdBottom), in the video's page that is not hidden.
  function shownOsd() {
    var found = document.querySelectorAll('.videoOsdBottom')
    for (var i = 0; i < found.length; i++) {
      if (!hidden(found[i].parentNode)) return found[i]
    }
    return null
  }
  // lookOsd asks for the badges of the play once its controls show, and
  // puts the row after the title's heading (.osdTitle) in the controls,
  // where it shows and hides with them. Their showing is watched: the
  // page's changes do not tell their class.
  function lookOsd() {
    if (!play) return
    if (!attached(play.video)) {
      stop()
      watch()
      return
    }
    var osd = shownOsd()
    if (!osd) return
    if (osd !== osdWatched && typeof MutationObserver === 'function') {
      if (osdWatcher) osdWatcher.disconnect()
      osdWatched = osd
      osdWatcher = new MutationObserver(quietly(lookOsd))
      osdWatcher.observe(osd, { attributes: true, attributeFilter: ['class'] })
    }
    var showing = !osd.classList.contains('hide') && !osd.classList.contains('videoOsdBottom-hidden')
    if (!play.asked && showing) askPlaying(play)
    var heading = osd.querySelector('.osdTitle')
    var badges = play.answer && play.answer.Show ? shown(play.answer.Badges) : []
    if (!badges.length || !heading) {
      if (osdRow && osdRow.parentNode) osdRow.parentNode.removeChild(osdRow)
      osdRow = null
      osdKey = ''
      return
    }
    var key = JSON.stringify(badges)
    if (osdRow && osdKey === key && heading.nextSibling === osdRow) return
    if (osdRow && osdRow.parentNode) osdRow.parentNode.removeChild(osdRow)
    osdRow = row(badges, 'polyfinVersionBadgesOsd')
    osdKey = key
    heading.parentNode.insertBefore(osdRow, heading.nextSibling)
  }
  // askPlaying asks for the badges of the version this device plays, and
  // again every 2 seconds, 5 times at most, while the answer names no
  // title: jellyfin-web tells the server of a play a moment after the
  // video starts.
  function askPlaying(current) {
    current.timer = null
    if (play !== current) return
    var api = window.ApiClient
    if (!api || !api.getCurrentUserId()) return
    current.asked++
    api.getJSON(address(api, 'Polyfin/Playback/Badges')).then(
      quietly(function (got) {
        if (play !== current || !got) return
        if (!got.ItemId) {
          if (current.asked <= 5) {
            current.timer = setTimeout(
              quietly(function () {
                askPlaying(current)
              }),
              2000
            )
          }
          return
        }
        current.answer = got
        lookOsd()
      }),
      function () {}
    )
  }
  // style adds the look of the rows, once: pills with a border and their
  // text, in the page's font, wrapping where the row does not fit; beside
  // a label, in line with the menu; across the track menus' grid when it
  // stands for the hidden version menu; on a dark ground over a video;
  // larger in the TV layout.
  function style() {
    if (document.getElementById('polyfinVersionBadgesStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinVersionBadgesStyle'
    sheet.textContent =
      '.polyfinVersionBadges{display:flex;flex-wrap:wrap;align-items:center;margin:0;padding:0;list-style:none;' +
      'max-width:100%;white-space:normal}' +
      '.polyfinVersionBadges>li{display:block;margin:0 .35em .3em 0;padding:0}' +
      '[dir=rtl] .polyfinVersionBadges>li{margin:0 0 .3em .35em}' +
      '.polyfinVersionBadge{display:inline-block;box-sizing:border-box;padding:.05em .45em;border:1px solid currentColor;' +
      'border-radius:.3em;font:inherit;font-size:.8em;line-height:1.5;white-space:nowrap;cursor:default}' +
      '.polyfinVersionBadgesMenu{margin:0 0 .5em;padding-inline-start:6.45em}' +
      '.layout-mobile .polyfinVersionBadgesMenu{padding-inline-start:4.7em}' +
      '.polyfinVersionBadgesIn{flex:0 0 auto;width:100%;box-sizing:border-box}' +
      '.polyfinVersionBadgesTracks{grid-column:1/-1;margin:0 0 .7em}' +
      '.polyfinVersionBadgesOsd{flex:0 1 auto;min-width:0}' +
      '.polyfinVersionBadgesOsd .polyfinVersionBadge{background:rgba(0,0,0,.55);color:#fff}' +
      '.layout-tv .polyfinVersionBadge{font-size:1em;padding:.15em .6em}' +
      '.layout-tv .polyfinVersionBadges>li{margin-bottom:.4em}'
    document.head.appendChild(sheet)
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
  addEventListener('playing', quietly(played), true)
  addEventListener('ended', quietly(ended), true)
  addEventListener('emptied', quietly(ended), true)
  quietly(follow)()
