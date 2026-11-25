// Part 13: reminders on a programme's page.
//
// On the page of a Live TV programme that has not started, it
// adds Polyfin's reminders after the record buttons (.recordingFields,
// which jellyfin-web draws on every title page and fills for programmes):
// "Remind me", which has Polyfin tell the user, on their own notification
// targets, a few minutes before the programme starts, and a menu button
// whose menu offers "Remind me of every airing": the programmes of the
// same title on the same channel at the same time of day, within ten
// minutes, as a series timer finds them. A programme with a reminder shows
// "Reminder set" instead, which removes it, or the series reminder that
// made it: a line under the button then tells every airing at about which
// time. When no target of the user chose Programme starting, and their
// apps do not show reminders, a line says so, with a link to My account ›
// Notifications in the admin app. Jellyfin has no reminder: the script asks
// Polyfin's /Polyfin/Reminders/Programs/{id}, which answers how the
// programme stands, and an empty 204 for an item that is not a programme
// of the user's guide or for a user without Live TV, who get no button; it
// posts to /Polyfin/Reminders to set one, with the page's language, which
// the message is written in, and deletes /Polyfin/Reminders/{id} to remove
// one. It asks once jellyfin-web has drawn the page's recording fields,
// and the page's changes put the buttons back as jellyfin-web renders the
// page anew. Its words follow jellyfin-web's language, as the third's. It
// never changes the page during a video, and takes the buttons off as the
// route leaves the programme. Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: details
//   selector: .videoPlayerContainer
//   selector: .page.itemDetailPage:not(.hide)
//   selector: .recordingFields
//   selector: .toastContainer
//   class: raised
//   class: emby-button
//   class: material-icons
//   class: toast
//   class: toastVisible
//   class: toastHide
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.ajax

  var route = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  var words = {
    en: {
      remind: 'Remind me',
      set: 'Reminder set',
      every: 'Remind me of every airing',
      more: 'More reminders',
      remove: 'Remove the reminder',
      everyAiring: 'Reminds you of every airing at about ',
      noTarget: 'No target of yours receives reminders yet: choose Programme starting for one in ',
      notifications: 'My account › Notifications',
      added: 'Reminder set: you will be told before it starts.',
      removed: 'Reminder removed.',
    },
    fr: {
      remind: 'Me le rappeler',
      set: 'Rappel prévu',
      every: 'Me rappeler chaque diffusion',
      more: 'Autres rappels',
      remove: 'Supprimer le rappel',
      everyAiring: 'Vous rappelle chaque diffusion vers ',
      noTarget: 'Aucune de vos cibles ne reçoit encore les rappels\u202f: choisissez «\u202fDébut d’une émission\u202f» pour l’une d’elles dans ',
      notifications: 'Mon compte › Notifications',
      added: 'Rappel prévu\u202f: vous serez prévenu avant le début.',
      removed: 'Rappel supprimé.',
    },
    de: {
      remind: 'Erinnern',
      set: 'Erinnerung gesetzt',
      every: 'An jede Ausstrahlung erinnern',
      more: 'Weitere Erinnerungen',
      remove: 'Erinnerung entfernen',
      everyAiring: 'Erinnert dich an jede Ausstrahlung gegen ',
      noTarget: 'Noch keines deiner Ziele erhält Erinnerungen: Wähle „Sendung beginnt“ für eines davon unter ',
      notifications: 'Mein Konto › Benachrichtigungen',
      added: 'Erinnerung gesetzt: Du wirst vor dem Beginn benachrichtigt.',
      removed: 'Erinnerung entfernt.',
    },
    es: {
      remind: 'Recordármelo',
      set: 'Recordatorio programado',
      every: 'Recordarme cada emisión',
      more: 'Más recordatorios',
      remove: 'Quitar el recordatorio',
      everyAiring: 'Le recuerda cada emisión hacia las ',
      noTarget: 'Ninguno de sus destinos recibe todavía recordatorios: elija «Programa a punto de empezar» para uno de ellos en ',
      notifications: 'Mi cuenta › Notificaciones',
      added: 'Recordatorio programado: se le avisará antes de que empiece.',
      removed: 'Recordatorio quitado.',
    },
    it: {
      remind: 'Ricordamelo',
      set: 'Promemoria impostato',
      every: 'Ricordami ogni messa in onda',
      more: 'Altri promemoria',
      remove: 'Rimuovi il promemoria',
      everyAiring: 'Ti ricorda ogni messa in onda verso le ',
      noTarget: 'Nessuna delle tue destinazioni riceve ancora i promemoria: scegli «Programma in partenza» per una di esse in ',
      notifications: 'Il mio account › Notifiche',
      added: 'Promemoria impostato: riceverai un avviso prima dell’inizio.',
      removed: 'Promemoria rimosso.',
    },
    pt: {
      remind: 'Lembrar-me',
      set: 'Lembrete definido',
      every: 'Lembrar-me de cada exibição',
      more: 'Mais lembretes',
      remove: 'Remover o lembrete',
      everyAiring: 'Lembra você de cada exibição por volta das ',
      noTarget: 'Nenhum dos seus destinos recebe lembretes ainda: escolha “Programa começando” para um deles em ',
      notifications: 'Minha conta › Notificações',
      added: 'Lembrete definido: você será avisado antes do início.',
      removed: 'Lembrete removido.',
    },
  }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (words[language()] || words.en)[key]
  }
  // The programme whose page is followed, how it stands as Polyfin last
  // answered, whether it was asked, whether a change is under way, the
  // buttons drawn and the answer they were drawn for, and whether the
  // page's changes are followed, as they are while the page is (see
  // core/page-changes.js).
  var program = ''
  var state = null
  var asked = false
  var busy = false
  var drawn = null
  var drawnFor = null
  var watching = false
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // follow starts following the page of the programme the route opens,
  // or stops, taking the buttons off.
  function follow() {
    var match = route.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === program) return
    program = id
    state = null
    asked = false
    busy = false
    undraw()
    if (!id) {
      if (watching) document.removeEventListener('polyfinpagechange', placed)
      watching = false
      return
    }
    // The script runs in the page's head: a page opened on a programme's
    // address has no body yet, which watch waits for.
    if (document.body) watch()
    else addEventListener('DOMContentLoaded', quietly(watch))
  }
  // watch follows the page's changes while a programme's page is followed,
  // and looks at the page.
  function watch() {
    if (!program || !document.body) return
    if (!watching) {
      watching = true
      document.addEventListener('polyfinpagechange', placed)
    }
    place()
  }
  // place as it listens to the page's changes.
  var placed = quietly(place)
  // undraw takes the buttons off.
  function undraw() {
    if (drawn && drawn.parentNode) drawn.parentNode.removeChild(drawn)
    drawn = null
    drawnFor = null
  }
  // place asks Polyfin how the programme stands once jellyfin-web drew
  // the page's recording fields, then puts the buttons after them, again
  // whenever jellyfin-web renders the page anew.
  function place() {
    if (!program) return
    var page = shownPage()
    var fields = page && page.querySelector('.recordingFields')
    if (!fields) return
    if (!asked) return ask(program)
    if (!state || state.Started) return undraw()
    if (drawn && drawnFor === state && drawn.parentNode === fields.parentNode && drawn.previousSibling === fields) return
    undraw()
    style()
    drawn = draw()
    drawnFor = state
    fields.parentNode.insertBefore(drawn, fields.nextSibling)
  }
  // ask asks Polyfin how the programme id stands: an item that is not a
  // programme of the user's guide, or a user without Live TV, gets an
  // empty answer, which getJSON fails to read, and no button.
  function ask(id) {
    var api = window.ApiClient
    if (!api) return
    asked = true
    api.getJSON(api.getUrl('Polyfin/Reminders/Programs/' + id)).then(
      quietly(function (answer) {
        if (program !== id || !answer || String(answer.ProgramId || '').toLowerCase() !== id) return
        state = answer
        place()
      }),
      function () {}
    )
  }
  // button makes a button styled as jellyfin-web's record buttons.
  function button(className, icon, text) {
    var made = document.createElement('button')
    made.type = 'button'
    made.className = 'raised emby-button ' + className
    var mark = document.createElement('span')
    mark.className = 'material-icons polyfinReminderIcon ' + icon
    mark.setAttribute('aria-hidden', 'true')
    made.appendChild(mark)
    if (text) {
      var label = document.createElement('span')
      label.textContent = text
      made.appendChild(label)
    }
    return made
  }
  // draw makes the buttons for the answer: Remind me, or Reminder set,
  // and the menu button, unless a series reminder made the reminder; then
  // the lines that tell of a series reminder, and that no target receives
  // reminders.
  function draw() {
    var block = document.createElement('div')
    block.className = 'polyfinReminders'
    var row = document.createElement('div')
    row.className = 'polyfinReminderRow'
    block.appendChild(row)
    var set = !!state.ReminderId
    var main = button('polyfinRemind' + (set ? ' polyfinReminderSet' : ''), set ? 'notifications_active' : 'notifications_none', say(set ? 'set' : 'remind'))
    if (set) main.title = say('remove')
    main.disabled = busy
    main.addEventListener('click', quietly(set ? remove : remind(false)))
    row.appendChild(main)
    if (!state.SeriesReminderId) {
      var more = button('polyfinReminderMore', 'expand_more', '')
      more.title = say('more')
      more.setAttribute('aria-label', say('more'))
      more.setAttribute('aria-haspopup', 'menu')
      more.setAttribute('aria-expanded', 'false')
      more.disabled = busy
      var menu = document.createElement('div')
      menu.className = 'polyfinReminderMenu hide'
      menu.setAttribute('role', 'menu')
      var every = document.createElement('button')
      every.type = 'button'
      every.className = 'polyfinReminderEvery'
      every.setAttribute('role', 'menuitem')
      every.textContent = say('every')
      every.addEventListener('click', quietly(remind(true)))
      menu.appendChild(every)
      more.addEventListener(
        'click',
        quietly(function () {
          var open = menu.classList.contains('hide')
          menu.classList.toggle('hide', !open)
          more.setAttribute('aria-expanded', open ? 'true' : 'false')
          if (open) every.focus()
        })
      )
      row.appendChild(more)
      row.appendChild(menu)
    }
    if (state.SeriesReminderId && state.SeriesStartDate) {
      var time = new Date(state.SeriesStartDate).toLocaleTimeString(document.documentElement.lang || undefined, {
        hour: '2-digit',
        minute: '2-digit',
      })
      block.appendChild(note(say('everyAiring') + time + '.'))
    }
    if (!state.Reaches) {
      var hint = note(say('noTarget'))
      var link = document.createElement('a')
      link.className = 'polyfinReminderLink'
      // Marked as opened from the web client, as the script's other links
      // to the admin app are (see fromWebClient). The server names the page
      // from its root, /admin/…: the web client's folder is one below it,
      // or below Polyfin's base path. Written by hand, as the script writes
      // its addresses (see adminAddress).
      var page = location.href.split('#')[0].split('?')[0]
      var path = (state.NotificationsUrl || '/admin/me/account#notifications').replace(/^\/+/, '')
      var at = path.indexOf('#')
      var rest = at < 0 ? path : path.slice(0, at)
      var root = /^[a-z][a-z0-9+.-]*:\/\/[^/]*\//i.exec(page)
      var above = page.slice(0, page.lastIndexOf('/') + 1).replace(/[^/]+\/$/, '')
      link.href =
        (root && above.length < root[0].length ? root[0] : above) +
        rest +
        (rest.indexOf('?') < 0 ? '?' : '&') +
        'from=web' +
        (at < 0 ? '' : path.slice(at))
      link.textContent = say('notifications')
      hint.appendChild(link)
      hint.appendChild(document.createTextNode('.'))
      block.appendChild(hint)
    }
    return block
  }
  function note(text) {
    var line = document.createElement('div')
    line.className = 'polyfinReminderNote'
    line.textContent = text
    return line
  }
  // remind sets a reminder of the programme, or of every airing.
  function remind(series) {
    return function () {
      send('POST', 'Polyfin/Reminders', { ProgramId: program, Series: series, Language: language() })
    }
  }
  // remove removes the reminder shown: the series reminder that made it,
  // if any.
  function remove() {
    send('DELETE', 'Polyfin/Reminders/' + (state.SeriesReminderId || state.ReminderId))
  }
  // send sends a change, then shows how the programme stands, and says so.
  function send(method, path, body) {
    var api = window.ApiClient
    if (!api || busy) return
    var id = program
    busy = true
    drawnFor = null
    place()
    var request = { type: method, url: api.getUrl(path) }
    if (body) {
      request.data = JSON.stringify(body)
      request.contentType = 'application/json'
      request.dataType = 'json'
    }
    api.ajax(request).then(
      quietly(function (answer) {
        if (program !== id) return
        busy = false
        if (body && answer && answer.ProgramId) state = answer
        else state = Object.assign({}, state, { ReminderId: '', SeriesReminderId: '', SeriesStartDate: null })
        toast(say(body ? 'added' : 'removed'))
        place()
      }),
      quietly(function () {
        if (program !== id) return
        busy = false
        drawnFor = null
        place()
      })
    )
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds.
  function toast(text) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = document.createElement('div')
      holder.className = 'toastContainer'
      document.body.appendChild(holder)
    }
    var shown = document.createElement('div')
    shown.className = 'toast'
    shown.textContent = text
    holder.appendChild(shown)
    setTimeout(function () {
      shown.classList.add('toastVisible')
    }, 50)
    setTimeout(function () {
      shown.classList.add('toastHide')
      setTimeout(function () {
        if (shown.parentNode) shown.parentNode.removeChild(shown)
      }, 300)
    }, 3300)
  }
  // style adds the look of the buttons, the menu and the lines, once.
  function style() {
    if (document.getElementById('polyfinRemindersStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinRemindersStyle'
    sheet.textContent =
      '.polyfinReminders{margin:-.5em 0 1.5em}' +
      '.polyfinReminderRow{position:relative;display:inline-flex;align-items:center;gap:.3em}' +
      '.polyfinRemind{min-width:10em;margin:0}' +
      '.polyfinReminderIcon{margin-right:.4em}' +
      '.polyfinReminderSet .polyfinReminderIcon{color:#00a4dc}' +
      '.polyfinReminderMore{margin:0;padding-left:.5em;padding-right:.5em;min-width:0}' +
      '.polyfinReminderMore .polyfinReminderIcon{margin:0}' +
      '.polyfinReminderMenu{position:absolute;top:100%;left:0;z-index:10;margin-top:.3em;padding:.3em 0;border-radius:.3em;' +
      'background:#202020;box-shadow:0 .2em .8em rgba(0,0,0,.6)}' +
      '.polyfinReminderMenu.hide{display:none}' +
      '.polyfinReminderEvery{display:block;width:100%;padding:.8em 1.2em;border:0;background:none;color:inherit;font:inherit;' +
      'text-align:left;white-space:nowrap;cursor:pointer}' +
      '.polyfinReminderEvery:hover,.polyfinReminderEvery:focus{background:rgba(255,255,255,.1)}' +
      '.polyfinReminderNote{margin-top:.6em;opacity:.8}' +
      '.polyfinReminderLink{color:#00a4dc}'
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
  // A click outside the menu closes it.
  addEventListener(
    'click',
    quietly(function (event) {
      var menu = drawn && drawn.querySelector('.polyfinReminderMenu')
      if (!menu || menu.classList.contains('hide')) return
      var target = event.target
      if (target && target.closest && target.closest('.polyfinReminderRow')) return
      menu.classList.add('hide')
    }),
    true
  )
  quietly(follow)()
