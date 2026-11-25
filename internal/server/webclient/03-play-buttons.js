// Part 3: the play buttons held until a version is known.
//
// On a title's page, it holds the play buttons (.btnPlay, .btnResume,
// .btnReplay) until a version is known, as /Polyfin/Items/{id}/Versions and
// its push tell (Known, the placeholder left out). While none is known and
// addons are asked (Pending), they are disabled, greyed, with a spinner and
// the tooltip "Looking for sources…"; as soon as one is known, they are given
// back as jellyfin-web made them. Until the first answer, they are held as
// searching when the version menu lists only the placeholder, a single
// source under the title's own identifier, from the page's first render:
// the script follows jellyfin-web's renders from the route's change, so
// that Play is never drawn enabled before a version is known. The first
// answer may come before the title's details asked any addon, so it never
// tells that no source is available. Polyfin's answer to that first
// request has it ask the title's addons, so the stream lists come with
// the details' metadata rather than after it. Once no addon is left to
// ask, they stay disabled and a line under them says that no source is
// available, with a "Try again" button: it posts to
// /Polyfin/Items/{id}/Versions/Search, which has Polyfin ask the title's
// addons again, then follows the versions anew; asked too soon, Polyfin
// answers 429 with Retry-After, and the button waits that long. A
// MutationObserver on the page holds the buttons again when jellyfin-web
// renders it anew. The buttons wait 30 seconds at most, as long as Polyfin
// asks an addon, and not at all when Polyfin fails to answer before any
// answer: they are given back while searching, and jellyfin-web's play
// waits for the addons itself. Its words follow jellyfin-web's language, the
// document's lang (English, French, German, Spanish, Italian or Brazilian
// Portuguese, which any Portuguese gets; English otherwise). It never changes
// the page during a video, and gives everything back when the route leaves
// the title.
//
// Anchors in jellyfin-web:
//   selector: .selectSource
//   selector: .btnPlay
//   selector: .btnReplay
//   selector: .detailRibbon
//   selector: .mainDetailButtons
//   class: padded-left
//   class: padded-right
//   class: raised
//   class: emby-button
//   global: ApiClient.ajax
//   global: ApiClient.getUrl

  // The play buttons held, each with what the script changed as it was
  // before and the tooltip it gave; the line telling that no source is
  // available; the page watched for jellyfin-web's renders, and whether the
  // page's changes are followed until the first answer (see
  // core/page-changes.js); whether the latest answer may have come before
  // the title's details asked any addon; and when the title's addons may be
  // asked again.
  var held = []
  var note = null
  var watched = null
  var watcher = null
  var following = false
  var unsure = false
  var late = false
  var lateTimer = null
  var againAt = 0
  var againTimer = null
  // holdFor is how long the play buttons wait for a version: an addon is
  // asked 30 seconds at most, so Polyfin knows by then. Past it, or once
  // Polyfin fails to answer before any answer, they are given back while
  // searching, as jellyfin-web made them: its play then waits for the
  // addons itself.
  var holdFor = 30000
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
    if (!progress) return !late && page && placeholder(page) ? 'searching' : ''
    if (!(progress.Count >= 1) || typeof progress.Known !== 'number' || progress.Known > 0) return ''
    if (progress.Pending > 0 || unsure) return late ? '' : 'searching'
    return 'none'
  }
  function placeholder(page) {
    var menu = page.querySelector('.selectSource')
    return !!menu && menu.options.length === 1 && String(menu.options[0].value).toLowerCase() === title
  }
  // hold brings the play buttons of the title page shown to the latest
  // answer: disabled, with a tooltip, greyed and a spinner while addons are
  // searched; disabled, with a line under them and a button to search again,
  // when no source is available; as jellyfin-web made them otherwise. It
  // follows jellyfin-web's renders while the buttons may be held, from the
  // page's first render, so that they are never drawn enabled before a
  // version is known, and changes only what differs. It never changes the
  // page during a video.
  function hold() {
    try {
      var page = shownPage()
      followPage(!latest ? !late : !!waiting(page))
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
        wait()
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
  // wait has the play buttons wait for a version for holdFor from now.
  function wait() {
    late = false
    clearTimeout(lateTimer)
    lateTimer = setTimeout(giveBack, holdFor)
  }
  // giveBack gives the play buttons back while addons are searched.
  function giveBack() {
    late = true
    clearTimeout(lateTimer)
    lateTimer = null
    try {
      hold()
    } catch (error) {
      release()
    }
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
  // followPage follows the page's changes, or stops, so that jellyfin-web's
  // renders find the play buttons held as soon as they are drawn.
  function followPage(on) {
    if (on === following) return
    following = on
    if (on) document.addEventListener('polyfinpagechange', hold)
    else document.removeEventListener('polyfinpagechange', hold)
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
