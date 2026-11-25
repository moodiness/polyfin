// Part 2: the versions added to a title's page, and its track menus.
//
// On a title's page (#/details?id=…), it adds the versions of the
// addons that answer after the page opened. Item details list the versions
// Polyfin knows when they are asked, while it asks the other addons in the
// background, and asks each addon again 10 seconds after its first answer,
// then 30 seconds later while its answers grow; /Polyfin/Items/{id}/Versions
// tells how many addons it still asks (Pending) and how many versions
// details list now (Count). Polyfin also pushes it, as it changes, on the
// socket jellyfin-web keeps open, in a PolyfinVersions message whose Data
// names the page's identifier (ItemId): the script hears it through
// ApiClient.subscribe, which gives the messages of jellyfin-web's socket
// since 12.0 (ApiClient's message event before, without subscribe). It
// asks at once as the route changes, then every second while addons are
// pending, every 10 seconds once a push came, the socket then telling the
// changes,
// for at most 150 seconds: a first answer, which takes at most 30 seconds,
// and both follow-ups, 10 and 30 seconds later and as long each, end within
// 130. When the page lists fewer versions than Count, or a different number
// once no addon is pending, it asks for the title's details and lists their
// versions in the page's version menu, in place, keeping the version picked.
// It reloads the page's details instead, as jellyfin-web does when the page
// is shown again, when the version picked is gone or no longer the same
// version, since the track menus describe it. The first version carries the
// title's own identifier, and each source its version's own identifier as
// its ETag: when a version listed first takes the title's identifier, or
// Polyfin names the version picked otherwise, the version picked, by
// jellyfin-web or the user, is found by its ETag, else as the one source
// with its name, and its size when both are known, and picked under its
// new identifier before the reload, which jellyfin-web then keeps. It never
// changes the page while a video plays or while the version menu has focus,
// where it could be open: it tries again at the next answer, or as soon as
// the menu loses focus. When the answers say AddVersions false (Settings ›
// Web player › Add versions to an open title's page, off), it only brings
// a menu that lists the placeholder to the versions: the others show when
// the page opens again. Any error just stops it.
//
// The same page refreshes its track menus once a version is analyzed:
// jellyfin-web fills them from the details it got when the page opened, or
// from its own copy of them when the page is shown again after a video,
// so a version analyzed since, as a play analyzes it, shows no track until
// the browser tab reloads. Polyfin's push then says Tracks: a second
// later, gathering the pushes of the versions analyzed together, the
// script asks for the title's details, and when they list other tracks
// for the version picked than its menus, reloads the page's details and,
// once jellyfin-web has listed the versions anew, tells it the version was
// picked, as picking it does: jellyfin-web fills the track menus from
// those details, and from the version's own once it has them. The version
// picked stays picked, and so do the audio and subtitle tracks the user
// picked on the page, by name, where the menus still list them. A push
// heard while a video plays is kept for the page shown again; the check
// waits for the video to end, and for the page's menus to lose focus.
// The page of the title the route left last is checked as it is shown
// again, push or not, as after its video: when the tracks are the same but
// the details name the versions otherwise, as once a first play made the
// device's profile known and the names say how each version plays, the
// version menu takes their names in place, the version picked staying.
//
// Anchors in jellyfin-web:
//   selector: .itemDetailPage
//   selector: .selectSourceContainer
//   selector: .selectSource
//   selector: .hide
//   selector: .trackSelections
//   selector: .selectVideo
//   selector: .selectAudio
//   selector: .selectSubtitles
//   global: ApiClient.getItem
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   event: viewshow
//   event: viewbeforehide

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
  // Whether Polyfin pushed the title's progress.
  var pushed = false
  // The options this script added to the version menu, and the size and
  // the ETag of the version each option stood for when the script last
  // listed it. Polyfin gives each source the identifier of its version as
  // its ETag, the first source's included, which carries the title's.
  // jellyfin-web fills the menu with new options, which forgets them all:
  // given, the sources the script was last given by identifier, tells them
  // again for an option jellyfin-web listed under a source's identifier and
  // name.
  var added = marks('set', 'Added')
  var sizes = marks('map', 'Size')
  var tags = marks('map', 'Tag')
  var given = {}
  // The version picked among those added, until jellyfin-web lists it: the
  // observer that waits for jellyfin-web's options, and the identifier of
  // the version picked.
  var picking = null
  // The title whose track menus Polyfin told may lack tracks, until they
  // are checked; the title the route left last; the timer that gathers
  // Polyfin's pushes before the check; the refill of the version menu a
  // refresh waits for; the tracks the user picked on the page shown, and
  // those a refresh keeps through jellyfin-web's renders; and whether the
  // script is telling jellyfin-web of a pick.
  var tracksDue = ''
  var left = ''
  var tracksTimer = null
  var refreshing = null
  var chosen = {}
  var keeping = null
  var dispatching = false
  var trackMenus = ['.selectSource', '.selectVideo', '.selectAudio', '.selectSubtitles']
  // reload reloads a title page's details as jellyfin-web does when the page
  // is shown again: it asks for them, lists their versions keeping the one
  // picked, and fills the track menus.
  function reload(page) {
    stopKeeping()
    page.dispatchEvent(new CustomEvent('viewbeforehide', { detail: {} }))
    page.dispatchEvent(new CustomEvent('viewshow', { detail: { isRestored: false } }))
  }
  // stale tells whether an option no longer stands for a source with its
  // identifier: the title's own identifier passes from the placeholder to
  // the first version, then to whichever version comes first, the version
  // it leaves taking an identifier of its own. When the script knows the
  // option's ETag, a version keeping its ETag as its name or size changes
  // is the same; otherwise jellyfin-web names an option after its source,
  // and the name (see named) and the size the script knows tell.
  function stale(option, source) {
    if (tags.has(option) && source.ETag) return tags.get(option) !== source.ETag
    return !named(option.textContent, source.Name) || (sizes.has(option) && sizes.get(option) !== source.Size)
  }
  // named tells whether an option's text names a source: its name, which
  // may have gained, lost or changed the ending in parentheses that says
  // how the app plays it, such as " (direct play)", since jellyfin-web
  // listed it (Settings › Playback › Say in version names how they play).
  function named(text, name) {
    var noted = / \([^()]*\)$/
    return (
      text === name ||
      (name.indexOf(text + ' (') === 0 && noted.test(name)) ||
      (text.indexOf(name + ' (') === 0 && noted.test(text)) ||
      (noted.test(text) && noted.test(name) && text.replace(noted, '') === name.replace(noted, ''))
    )
  }
  // remember notes the size and the ETag of the source an option stands for.
  function remember(option, source) {
    sizes.set(option, source.Size)
    if (source.ETag) tags.set(option, source.ETag)
    else tags.delete(option)
  }
  // place lists sources in the version menu, in their order, keeping the
  // version picked, and tells whether it could. An option jellyfin-web
  // listed is kept while it stands for the same source, as jellyfin-web
  // knows that source's tracks, and named as the source is now. When the
  // version picked lost its identifier, to another or because Polyfin names
  // it otherwise now, it cannot: it is picked under its new one (see keep).
  function place(menu, sources) {
    var picked = menu.options[menu.selectedIndex]
    var kept = {}
    for (var i = 0; i < menu.options.length; i++) {
      var listed = menu.options[i]
      var known = given[listed.value]
      if (!sizes.has(listed) && known && named(listed.textContent, known.Name)) remember(listed, known)
      kept[listed.value] = listed
    }
    given = {}
    sources.forEach(function (source) {
      given[source.Id] = source
    })
    var found = false
    var options = sources.map(function (source) {
      var option = kept[source.Id]
      if (option && stale(option, source)) option = null
      if (!option) option = sourceOption(source)
      else if (option.textContent !== source.Name) option.textContent = source.Name
      if (option === picked) found = true
      remember(option, source)
      return option
    })
    if (!found) {
      if (picked) keep(menu, picked, sources)
      return false
    }
    options.forEach(function (option, i) {
      if (menu.options[i] !== option) menu.insertBefore(option, menu.options[i] || null)
    })
    while (menu.options.length > options.length) menu.removeChild(menu.options[options.length])
    menu.value = picked.value
    var container = menu.closest('.selectSourceContainer')
    if (container) container.classList.toggle('hide', options.length < 2)
    return true
  }
  // sourceOption makes an option for a source, as jellyfin-web does.
  function sourceOption(source) {
    var option = document.createElement('option')
    option.value = source.Id
    option.textContent = source.Name
    added.add(option)
    remember(option, source)
    return option
  }
  // keep picks, in place of the option picked, the version it stood for
  // when its identifier no longer names it: the title's identifier passed
  // from it to a version listed first, or Polyfin names it otherwise now.
  // That version is the source with its ETag, when the script knows it,
  // else the one source with its name, and its size when both are known,
  // under another identifier; several such sources tell nothing. The
  // page's details are then reloaded, and jellyfin-web keeps the identifier
  // picked when it lists it, where it would pick the version that took the
  // old one, or the first. Without such a source, as when the placeholder
  // gives way to the first version, the option stays, and jellyfin-web
  // picks the version that took its identifier, or the first. A version
  // picked among those added is then made again under its new identifier
  // (see pick).
  function keep(menu, picked, sources) {
    var tag = tags.get(picked)
    var size = sizes.get(picked)
    var found = null
    var alike = []
    for (var i = 0; i < sources.length; i++) {
      var source = sources[i]
      if (source.Id === picked.value) continue
      if (tag && source.ETag === tag) found = found || source
      var sized = !(size > 0 && source.Size > 0 && size !== source.Size)
      if (named(picked.textContent, source.Name) && sized) alike.push(source)
    }
    if (!found && alike.length === 1) found = alike[0]
    if (!found) return
    menu.insertBefore(sourceOption(found), picked)
    menu.removeChild(picked)
    menu.value = found.Id
    if (picking) picking.value = found.Id
  }
  // update brings the title page to the versions Polyfin knows when an
  // answer tells it lists fewer, or a different number once no addon is
  // pending. A hidden or empty version menu counts as one. When Polyfin
  // adds no version to an open page (AddVersions false), only a menu that
  // lists the placeholder, named after the title, is brought to them.
  function update(progress) {
    var id = title
    var page = shownPage()
    if (!id || !page || asking || refreshing || progress.Count < 1 || progress.Count === updatedFor) return
    var adding = progress.AddVersions !== false
    var menu = page.querySelector('.selectSource')
    if (!adding && (!menu || !placeholder(page))) return
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
        if (adding || (placeholder(page) && menu.options[0].textContent === item.Name)) {
          if (!place(menu, item.MediaSources || [])) reload(page)
        }
        updatedFor = progress.Count
      },
      function () {
        asking = false
      }
    )
  }
  // hear takes a pushed progress of the title followed as an answer. One
  // that tells the tracks of its versions changed (Tracks) has the track
  // menus checked, as soon as the page is shown for the title the route
  // left, as for a video.
  function hear(message) {
    if (!message || message.MessageType !== 'PolyfinVersions' || !message.Data) return
    var progress = message.Data
    var id = String(progress.ItemId).toLowerCase()
    if (progress.Tracks && id && (id === title || id === left)) {
      tracksDue = id
      dueTracks()
    }
    if (!title || id !== title) return
    pushed = true
    unsure = false
    take(progress)
  }
  // take brings the page to an answer. One that no longer searches ends
  // the wait for a version (see holdFor).
  function take(progress) {
    latest = progress
    if (waiting(null) !== 'searching') {
      clearTimeout(lateTimer)
      lateTimer = null
    }
    hold()
    update(progress)
    dueTracks()
  }
  function followVersions() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === title) return
    clearTimeout(timer)
    timer = null
    if (title) left = title
    title = id
    latest = null
    updatedFor = 0
    given = {}
    pushed = false
    if (picking) picking.observer.disconnect()
    picking = null
    release()
    if (watcher) watcher.disconnect()
    watcher = null
    watched = null
    followPage(false)
    unsure = true
    late = false
    clearTimeout(lateTimer)
    lateTimer = null
    clearTimeout(againTimer)
    againTimer = null
    againAt = 0
    // A check due waits through the routes that show no title, as a
    // video's, for its page to be shown again. The title the route left
    // last is checked as it comes back: jellyfin-web shows its own copy of
    // the page, whose version names may have changed since, as a first
    // play makes the device's profile known.
    if (id && tracksDue !== id) tracksDue = id === left ? id : ''
    clearTimeout(tracksTimer)
    tracksTimer = null
    if (refreshing) refreshing.observer.disconnect()
    refreshing = null
    chosen = {}
    stopKeeping()
    if (!id) return
    // From the route's change, the page is followed as jellyfin-web renders
    // it, so that its play buttons are held from its first render.
    wait()
    hold()
    poll(id, 0)
  }
  // poll asks for the title's versions for 150 seconds, from scratch, first
  // after delay, then every second, or every 10 once a push came.
  function poll(id, delay) {
    clearTimeout(timer)
    timer = null
    var round = ++polls
    var until = Date.now() + 150000
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
        .catch(function () {
          // Without an answer, the buttons wait for nothing.
          if (title === id && round === polls && !latest) giveBack()
        })
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
  // jellyfin-web has filled the menu anew, under the version's identifier
  // then, which keep follows when the title's identifier leaves it.
  function pick(event) {
    var menu = event.target
    var option = menu && menu.options && menu.options[menu.selectedIndex]
    var page = option && added.has(option) && menu.closest('.itemDetailPage')
    if (!page) return
    event.stopPropagation()
    if (picking) picking.observer.disconnect()
    var entry = { observer: null, value: menu.value }
    entry.observer = new MutationObserver(function () {
      var picked = menu.options[menu.selectedIndex]
      // This script's own changes leave the pick among its options.
      if (picked && added.has(picked)) return
      entry.observer.disconnect()
      if (picking === entry) picking = null
      if (picked && picked.value === entry.value) menu.dispatchEvent(new Event('change', { bubbles: true }))
    })
    picking = entry
    entry.observer.observe(menu, { childList: true })
    reload(page)
  }
  // A version menu that had focus is updated as soon as it loses it, and
  // track menus due a check are checked once none has focus.
  function leave(event) {
    var menu = event.target
    if (latest && menu && menu.matches && menu.matches('.selectSource')) update(latest)
    dueTracks()
  }
  // dueTracks checks the track menus of the title page shown a second
  // later, when Polyfin told the tracks of the title's versions changed:
  // the pushes of versions analyzed together come within it, and
  // jellyfin-web is done showing a page shown again.
  function dueTracks() {
    if (!title || tracksDue !== title || tracksTimer) return
    tracksTimer = setTimeout(function () {
      tracksTimer = null
      try {
        checkTracks()
      } catch (error) {}
    }, 1000)
  }
  // checkTracks asks for the title's details, and refreshes the track menus
  // of its page when they list other tracks than the details list for the
  // version picked. It waits while a video plays and while one of the
  // menus has focus, where it could be open: the next answer or push, a
  // show of the page or a loss of focus checks again. While the script
  // asks for the details or reloads them already, it checks once done.
  function checkTracks() {
    var id = title
    var page = shownPage()
    if (!id || tracksDue !== id || !page || refreshing || trackFocus(page)) return
    if (asking) return dueTracks()
    var menu = page.querySelector('.selectSource')
    var selections = page.querySelector('.trackSelections')
    tracksDue = ''
    if (!menu || !menu.value || !selections || selections.classList.contains('hide')) return
    var api = window.ApiClient
    asking = true
    api.getItem(api.getCurrentUserId(), id).then(
      function (item) {
        asking = false
        if (title !== id) return
        if (shownPage() !== page || page.querySelector('.selectSource') !== menu || trackFocus(page)) {
          tracksDue = id
          return
        }
        var sources = item.MediaSources || []
        for (var i = 0; i < sources.length; i++) {
          if (sources[i].Id === menu.value) {
            if (lacking(page, sources[i])) refresh(page, menu)
            else place(menu, sources)
            return
          }
        }
      },
      function () {
        asking = false
      }
    )
  }
  // trackFocus tells whether one of the menus of page has focus.
  function trackFocus(page) {
    var focused = document.activeElement
    return (
      !!focused &&
      trackMenus.some(function (selector) {
        return page.querySelector(selector) === focused
      })
    )
  }
  // lacking tells whether the track menus of page list other tracks than
  // source has, of each type: jellyfin-web lists them by their Index,
  // after "Off" (-1) for subtitles.
  function lacking(page, source) {
    var menus = { Video: '.selectVideo', Audio: '.selectAudio', Subtitle: '.selectSubtitles' }
    var streams = source.MediaStreams || []
    for (var type in menus) {
      var select = page.querySelector(menus[type])
      if (!select) continue
      var listed = []
      for (var i = 0; i < select.options.length; i++) {
        if (String(select.options[i].value) !== '-1') listed.push(String(select.options[i].value))
      }
      var known = []
      for (var j = 0; j < streams.length; j++) {
        if (streams[j].Type === type) known.push(String(streams[j].Index))
      }
      if (listed.sort().join() !== known.sort().join()) return true
    }
    return false
  }
  // refresh fills the track menus of page anew, as picking a version does:
  // the page's details are reloaded, which keeps the version picked, and
  // once jellyfin-web has listed the versions anew, the version menu tells
  // it that version was picked, which fills the track menus from those
  // details. The tracks the user picked are kept where the menus still
  // list them (see keepPicks).
  function refresh(page, menu) {
    var picks = chosen
    var entry = { observer: null }
    entry.observer = new MutationObserver(function () {
      entry.observer.disconnect()
      if (refreshing !== entry) return
      refreshing = null
      if (shownPage() !== page) {
        tracksDue = title
        return
      }
      keepPicks(page, picks)
      dispatching = true
      try {
        menu.dispatchEvent(new Event('change', { bubbles: true }))
      } finally {
        dispatching = false
      }
      restorePicks()
      // Polyfin may have told of other tracks meanwhile.
      dueTracks()
    })
    refreshing = entry
    entry.observer.observe(menu, { childList: true })
    reload(page)
  }
  // keepPicks has the audio and subtitle menus of page keep the tracks the
  // user picked, picks, through jellyfin-web's renders of them, until the
  // user picks again or the page hides: jellyfin-web renders them once the
  // version is picked, then again once it has that version's own details.
  function keepPicks(page, picks) {
    stopKeeping()
    var kept = []
    for (var selector in picks) {
      var select = page.querySelector(selector)
      if (select) kept.push({ select: select, pick: picks[selector] })
    }
    if (!kept.length) return
    keeping = { observer: new MutationObserver(restorePicks), kept: kept }
    kept.forEach(function (entry) {
      keeping.observer.observe(entry.select, { childList: true })
    })
  }
  // restorePicks picks again in each menu kept the track the user picked:
  // the one option with its name, or the one with its name and index.
  function restorePicks() {
    if (!keeping) return
    keeping.kept.forEach(function (entry) {
      var select = entry.select
      var named = []
      for (var i = 0; i < select.options.length; i++) {
        if (select.options[i].textContent === entry.pick.text) named.push(select.options[i])
      }
      var found = named.length === 1 ? named[0] : null
      for (var j = 0; j < named.length; j++) if (named[j].value === entry.pick.value) found = named[j]
      if (found && select.value !== found.value) select.value = found.value
    })
  }
  function stopKeeping() {
    if (keeping) keeping.observer.disconnect()
    keeping = null
  }
  // choose notes the tracks the user picks on the title page shown, and
  // forgets them as they pick a version, for which jellyfin-web lists its
  // default tracks. Either stops the picks a refresh keeps.
  function choose(event) {
    var select = event.target
    if (dispatching || !select || !select.matches || !title || !select.closest || !select.closest('.itemDetailPage')) return
    if (select.matches('.selectSource')) {
      chosen = {}
      stopKeeping()
      return
    }
    ;['.selectAudio', '.selectSubtitles'].forEach(function (selector) {
      if (!select.matches(selector)) return
      var option = select.options[select.selectedIndex]
      if (option) chosen[selector] = { value: option.value, text: option.textContent }
      stopKeeping()
    })
  }
  // hidden forgets the tracks the user picked as a page hides, as for a
  // video: jellyfin-web shows the page again with its default tracks.
  function hidden() {
    chosen = {}
    stopKeeping()
  }
  // These run before jellyfin-web's own listeners on the version menu.
  addEventListener('change', quietly(choose), true)
  addEventListener('change', quietly(pick), true)
  addEventListener('blur', quietly(leave), true)
  // jellyfin-web's views tell as they show and hide.
  addEventListener('viewshow', quietly(dueTracks))
  addEventListener('viewbeforehide', quietly(hidden))
