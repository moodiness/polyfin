// Part 11: the server's home rows.
//
// It adds to jellyfin-web's home page the rows the administrator
// chose under Settings › Web player › Home rows, as
// /Polyfin/HomeRows answers them for the user signed in: each with its
// Name, its Place and its first Items, the rows the user turned off, or
// without a title, left out. Each time the home page shows (#/home, its
// .homeSectionsContainer in a page that is not hidden), it asks again,
// with the user's home sections, as jellyfin-web reads them from the
// display preferences "usersettings" of the client "emby" (homesection0 to
// homesection9, jellyfin-web's defaults for those not set, the TV layout
// starting with the library tiles when they lack); then it puts each row
// at its place, in the answer's order: Top before the first section,
// AfterResume after the section of Continue Watching (resume), AfterNextUp
// after Next Up's (nextup), Bottom after the last; a row after a section
// the user's home page does not show takes the place before. jellyfin-web
// fills the sections .section0 to .section9 it made first, in place: the
// rows go between them. A row is drawn as jellyfin-web draws a library's
// row of latest items, with its classes, so that its styles, scroll
// buttons and item actions apply, and its Kind (data-polyfin-kind): the
// row's name opens its list (#/list?parentId= the row's Id), or, for a row
// of a playlist (its PlaylistId), the playlist's page; each card a poster
// with the title's played mark, or count of unplayed episodes, and
// progress, opening the title's page. A poster loads as its card comes
// within half a screen of the screen, at the width the card shows it at,
// as jellyfin-web loads its own (a browser without IntersectionObserver
// loads them all at once, 272 pixels wide). An episode shows as
// jellyfin-web shows one in a row: its series' poster, the series' name,
// then its season and episode numbers before its name. When jellyfin-web
// draws the home page anew, which empties it, the page's changes put the
// rows back from the last answer. Every time it draws them, it leaves out
// the titles the item menus hid meanwhile, and from the Now available,
// Still waiting and Watchlist rows those they took off the watchlist, as
// the tenth keeps them in the session storage for the user signed in (a
// series hidden taking its items with it), and a row left without a
// title. Nothing else changes, nor anything while the answer is awaited.
// A row with a Note shows it under its name, after "From <NoteBy>:" when
// it is signed. A row shown to chosen users (Suggested) has a Hide button
// after its name, which posts /Polyfin/HomeRows/{RowId}/Hidden, takes the
// row off the page and says so in a toast; and while it is New, a "New"
// badge, which goes, with a post of /Polyfin/HomeRows/{RowId}/Seen, once
// the user opens the row's list or one of its titles from the page. Its
// words follow jellyfin-web's language, as the third's.
// Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: home
//   route: details
//   route: list
//   selector: .homeSectionsContainer
//   selector: .page
//   selector: .card
//   selector: .itemAction
//   selector: .sectionTitleTextButton
//   selector: .toastContainer
//   selector: .card[data-index][data-isfolder][data-serverid][data-id][data-type][data-mediatype][data-context]
//   selector: .itemAction[data-action]
//   selector: .emby-scroller[data-centerfocus]
//   class: hide
//   class: layout-tv
//   class: material-icons
//   class: more
//   class: button-flat
//   class: button-flat-mini
//   class: emby-button
//   class: sectionTitle
//   class: sectionTitle-cards
//   class: sectionTitleContainer
//   class: sectionTitleContainer-cards
//   class: padded-left
//   class: itemsContainer
//   class: scrollSlider
//   class: focuscontainer-x
//   class: animatedScrollX
//   class: padded-top-focusscale
//   class: padded-bottom-focusscale
//   class: verticalSection
//   class: emby-scroller-container
//   class: cardImageContainer
//   class: coveredImage
//   class: cardContent
//   class: cardImageIcon
//   class: cardIndicators
//   class: countIndicator
//   class: indicator
//   class: playedIndicator
//   class: indicatorIcon
//   class: itemProgressBarForeground
//   class: innerCardFooter
//   class: fullInnerCardFooter
//   class: innerCardFooterClear
//   class: itemProgressBar
//   class: cardOverlayContainer
//   class: cardOverlayButton
//   class: cardOverlayButton-hover
//   class: paper-icon-button-light
//   class: cardOverlayFab-primary
//   class: cardOverlayButtonIcon
//   class: cardOverlayButtonIcon-hover
//   class: cardOverlayButton-br
//   class: flex
//   class: cardText
//   class: cardTextCentered
//   class: cardText-first
//   class: textActionButton
//   class: cardText-secondary
//   class: overflowPortraitCard
//   class: card-hoverable
//   class: card-withuserdata
//   class: cardBox
//   class: cardBox-bottompadded
//   class: cardScalable
//   class: cardPadder
//   class: cardPadder-overflowPortrait
//   class: toast
//   class: toastVisible
//   class: toastHide
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.getDisplayPreferences
//   global: ApiClient.serverId
//   global: ApiClient.ajax
//   global: ApiClient.getScaledImageUrl
//   text: usersettings
//   text: homesection
//   text: smalllibrarytiles
//   text: librarybuttons
//   text: emby-linkbutton
//   text: emby-itemscontainer
//   text: verticalSection section

  var homeRoute = /^#!?\/home(?:\.html)?(?:[?#]|$)/i
  // jellyfin-web's home sections by default, in order.
  var defaultSections = [
    'smalllibrarytiles',
    'resume',
    'resumeaudio',
    'resumebook',
    'livetv',
    'nextup',
    'latestmedia',
    'none',
    'none',
    'none',
  ]
  // The places of rows, from the top: the section each follows, Top
  // following none.
  var placeSections = { Top: '', AfterResume: 'resume', AfterNextUp: 'nextup' }
  var placeOrder = ['Top', 'AfterResume', 'AfterNextUp']
  // The home page's container of sections the rows were last asked for,
  // the answer for it, as {user, container, rows, sections}, and the
  // container being asked for.
  var shown = null
  var answer = null
  var asking = null
  // The words of the rows shown to chosen users, in jellyfin-web's
  // language: the note's signature, the badge of a row not seen yet, and
  // its Hide button.
  var rowWords = {
    en: {
      from: 'From %s:',
      fresh: 'New',
      hide: 'Hide',
      hidden: 'Row hidden. My account › Home rows shows it again.',
      notHidden: 'The row could not be hidden. Try again.',
    },
    fr: {
      from: 'De la part de %s\u202f:',
      fresh: 'Nouveau',
      hide: 'Masquer',
      hidden: 'Rangée masquée. Mon compte › Rangées d’accueil la réaffiche.',
      notHidden: 'La rangée n’a pas pu être masquée. Réessayez.',
    },
    de: {
      from: 'Von %s:',
      fresh: 'Neu',
      hide: 'Ausblenden',
      hidden: 'Zeile ausgeblendet. Unter Mein Konto › Startseitenzeilen wird sie wieder angezeigt.',
      notHidden: 'Die Zeile konnte nicht ausgeblendet werden. Versuche es erneut.',
    },
    es: {
      from: 'De parte de %s:',
      fresh: 'Nuevo',
      hide: 'Ocultar',
      hidden: 'Fila oculta. Mi cuenta › Filas de inicio la vuelve a mostrar.',
      notHidden: 'No se pudo ocultar la fila. Inténtelo de nuevo.',
    },
    it: {
      from: 'Da parte di %s:',
      fresh: 'Nuovo',
      hide: 'Nascondi',
      hidden: 'Riga nascosta. Il mio account › Righe della Home la mostra di nuovo.',
      notHidden: 'Non è stato possibile nascondere la riga. Riprova.',
    },
    pt: {
      from: 'Da parte de %s:',
      fresh: 'Novo',
      hide: 'Ocultar',
      hidden: 'Fileira oculta. Minha conta › Fileiras do início a mostra de novo.',
      notHidden: 'Não foi possível ocultar a fileira. Tente de novo.',
    },
  }
  function say(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (rowWords[language] || rowWords.en)[key]
  }
  // homeContainer returns the container of sections of the home page
  // shown, or null.
  function homeContainer() {
    if (!homeRoute.test(location.hash)) return null
    var containers = document.querySelectorAll('.homeSectionsContainer')
    for (var i = 0; i < containers.length; i++) {
      var page = containers[i].closest('.page')
      if (!page || !page.classList.contains('hide')) return containers[i]
    }
    return null
  }
  // check asks for the rows each time the home page shows, and draws them
  // again when jellyfin-web emptied the page.
  function check() {
    var container = homeContainer()
    if (!container) {
      shown = null
      return
    }
    if (container !== shown) {
      shown = container
      ask(container)
      return
    }
    if (answer && answer.container === container && !drawn(container)) draw(container)
  }
  // ask asks for the rows and the user's home sections, and draws them
  // once both answered, unless the home page was left meanwhile.
  function ask(container) {
    var api = window.ApiClient
    var user = api && api.getCurrentUserId()
    if (!user) {
      shown = null
      return
    }
    asking = container
    var rows = api.getJSON(api.getUrl('Polyfin/HomeRows', { Fields: 'PrimaryImageAspectRatio' }))
    var preferences = api.getDisplayPreferences('usersettings', user, 'emby')
    Promise.all([rows, preferences])
      .then(function (answers) {
        if (asking !== container || shown !== container || api.getCurrentUserId() !== user) return
        asking = null
        answer = {
          user: user,
          container: container,
          rows: (answers[0] && answers[0].Items) || [],
          sections: sectionsOf((answers[1] && answers[1].CustomPrefs) || {}),
        }
        draw(container)
      })
      .catch(function () {})
  }
  // sectionsOf lists the user's home sections, in order, as jellyfin-web
  // reads them.
  function sectionsOf(preferences) {
    var sections = []
    for (var i = 0; i < 10; i++) {
      var section = preferences['homesection' + i] || defaultSections[i]
      if (section === 'folders') section = defaultSections[0]
      sections.push(section)
    }
    var tv = document.documentElement.classList.contains('layout-tv')
    if (tv && sections.indexOf('smalllibrarytiles') < 0 && sections.indexOf('librarybuttons') < 0) {
      sections.unshift('smalllibrarytiles')
    }
    return sections
  }
  // shownRows returns the rows of the answer without the titles the item
  // menus hid meanwhile (see the tenth), and without those they took off
  // the watchlist in the rows of the watchlist (Now available and Still
  // waiting): those the tenth keeps in the session storage under
  // polyfinDrops for the user signed in, until they expire, a hidden series
  // taking its items with it, as a season or a series taken off the
  // watchlist takes its episodes. A row left without a title is left out.
  // Storage that fails, or holds anything else, hides nothing.
  function shownRows() {
    var api = window.ApiClient
    var viewer = (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + (api.getCurrentUserId() || '')
    var hidden = {}
    var unlisted = {}
    var unlistedWhole = { series: {}, season: {} }
    try {
      var drops = JSON.parse(window.sessionStorage.getItem('polyfinDrops'))
      if (Array.isArray(drops)) {
        drops.forEach(function (drop) {
          if (!drop || drop.viewer !== viewer || !(drop.until > Date.now())) return
          if (drop.kind === 'hidden') hidden[drop.id] = true
          if (drop.kind === 'watchlist') unlisted[drop.id] = true
          if (drop.kind === 'watchlist' && unlistedWhole[drop.whole]) unlistedWhole[drop.whole][drop.id] = true
        })
      }
    } catch (error) {}
    var plain = function (id) {
      return String(id || '')
        .replace(/-/g, '')
        .toLowerCase()
    }
    return answer.rows
      .map(function (row) {
        var watchlist = row.Kind === 'NowAvailable' || row.Kind === 'StillWaiting' || row.Kind === 'Watchlist'
        var items = (row.Items || []).filter(function (item) {
          if (
            watchlist &&
            (unlisted[plain(item.Id)] ||
              (item.SeriesId && unlistedWhole.series[plain(item.SeriesId)]) ||
              (item.SeasonId && unlistedWhole.season[plain(item.SeasonId)]))
          ) {
            return false
          }
          return !hidden[plain(item.Id)] && !(item.SeriesId && hidden[plain(item.SeriesId)])
        })
        return Object.assign({}, row, { Items: items })
      })
      .filter(function (row) {
        return row.Items.length > 0
      })
  }
  // drawn tells whether the container holds every row shown, each with all
  // its titles. Rows are found by their identifiers: draw puts them in the
  // order of their places, which may not be the order of the answer.
  function drawn(container) {
    var rows = shownRows()
    var drawnRows = {}
    var count = 0
    for (var i = 0; i < container.children.length; i++) {
      var child = container.children[i]
      if (child.classList.contains('polyfinHomeRow')) {
        drawnRows[child.getAttribute('data-polyfin-row')] = child
        count++
      }
    }
    return (
      count === rows.length &&
      rows.every(function (row) {
        var element = drawnRows[row.Id]
        return !!element && element.querySelectorAll('.card').length === row.Items.length
      })
    )
  }
  // draw puts the rows of the answer at their places in the container,
  // in place of those drawn before.
  function draw(container) {
    if (!container.querySelector('.section0')) return
    var old = container.querySelectorAll('.polyfinHomeRow')
    for (var i = 0; i < old.length; i++) old[i].parentNode.removeChild(old[i])
    var rows = shownRows()
    // The element the next row goes after, null for the top.
    var after = null
    placeOrder.forEach(function (place) {
      var index = placeSections[place] ? answer.sections.indexOf(placeSections[place]) : -1
      var section = index >= 0 ? container.querySelector('.section' + index) : null
      if (section && section.parentNode === container) after = section
      rows.forEach(function (row) {
        if (row.Place !== place) return
        var made = rowElement(row)
        container.insertBefore(made, after ? after.nextSibling : container.firstChild)
        after = made
      })
    })
    rows.forEach(function (row) {
      if (placeOrder.indexOf(row.Place) < 0) container.appendChild(rowElement(row))
    })
  }
  // made makes an element with its class, attributes and children.
  function made(tag, className, attributes, children) {
    var element = document.createElement(tag)
    if (className) element.className = className
    Object.keys(attributes || {}).forEach(function (name) {
      element.setAttribute(name, attributes[name])
    })
    ;(children || []).forEach(function (child) {
      element.appendChild(typeof child === 'string' ? document.createTextNode(child) : child)
    })
    return element
  }
  function icon(name, className) {
    return made('span', 'material-icons ' + (className ? className + ' ' : '') + name, { 'aria-hidden': 'true' })
  }
  // rowElement draws a row as jellyfin-web draws a library's row of
  // latest items, with its Kind, which tells the tenth a row of the
  // watchlist.
  function rowElement(row) {
    var api = window.ApiClient
    var server = typeof api.serverId === 'function' ? api.serverId() : ''
    // A row of a playlist opens the playlist's page, as its card would.
    var more = row.PlaylistId
      ? '#/details?id=' + encodeURIComponent(row.PlaylistId) + '&serverId=' + encodeURIComponent(server)
      : '#/list?parentId=' + encodeURIComponent(row.Id) + '&serverId=' + encodeURIComponent(server)
    var head = [
      made('a', 'more button-flat button-flat-mini sectionTitleTextButton emby-button', { is: 'emby-linkbutton', href: more }, [
        made('h2', 'sectionTitle sectionTitle-cards', {}, [row.Name]),
        icon('chevron_right'),
      ]),
    ]
    if (row.Suggested && row.RowId) {
      style()
      if (row.New) head.push(made('span', 'polyfinRowNew', {}, [say('fresh')]))
      var hide = made('button', 'polyfinRowHide button-flat button-flat-mini emby-button', { type: 'button', is: 'emby-button' }, [
        icon('close'),
        made('span', '', {}, [say('hide')]),
      ])
      hide.addEventListener('click', function (event) {
        event.preventDefault()
        event.stopPropagation()
        hideRow(row)
      })
      head.push(hide)
    }
    var parts = [made('div', 'sectionTitleContainer sectionTitleContainer-cards padded-left', {}, head)]
    if (row.Note) {
      style()
      var note = row.NoteBy ? [made('b', '', {}, [say('from').replace('%s', row.NoteBy)]), ' ' + row.Note] : [row.Note]
      parts.push(made('p', 'polyfinRowNote padded-left', {}, note))
    }
    var cards = (row.Items || []).map(function (item, index) {
      return card(api, server, item, index)
    })
    var items = made('div', 'itemsContainer scrollSlider focuscontainer-x animatedScrollX', { is: 'emby-itemscontainer' }, cards)
    parts.push(
      made('div', 'padded-top-focusscale padded-bottom-focusscale emby-scroller', { is: 'emby-scroller', 'data-centerfocus': 'true' }, [
        items,
      ])
    )
    var attributes = { 'data-polyfin-row': row.Id, 'data-polyfin-rowid': row.RowId || '', 'data-polyfin-kind': row.Kind || '' }
    var element = made('div', 'verticalSection emby-scroller-container polyfinHomeRow', attributes, parts)
    if (row.Suggested && row.New) {
      // In the capture phase: jellyfin-web stops the clicks on cards it
      // handles before they reach the row.
      element.addEventListener(
        'click',
        function (event) {
          try {
            opened(row, event)
          } catch (error) {}
        },
        true
      )
    }
    return element
  }
  // opened marks a row not seen yet seen once the user opens its list or
  // one of its titles: a click on its name, a card, or a card's buttons.
  function opened(row, event) {
    var target = event.target
    if (!target || typeof target.closest !== 'function' || target.closest('.polyfinRowHide')) return
    if (!target.closest('.itemAction') && !target.closest('.sectionTitleTextButton')) return
    var api = window.ApiClient
    var fresh = answer.rows.some(function (r) {
      return r.RowId === row.RowId && r.New
    })
    if (!fresh) return
    answer.rows.forEach(function (r) {
      if (r.RowId === row.RowId) r.New = false
    })
    rowElementsOf(row).forEach(function (element) {
      var badge = element.querySelector('.polyfinRowNew')
      if (badge) badge.parentNode.removeChild(badge)
    })
    api.ajax({ type: 'POST', url: api.getUrl('Polyfin/HomeRows/' + row.RowId + '/Seen') }).then(
      function () {},
      function () {}
    )
  }
  // hideRow turns a row off for the user, as My account › Home rows
  // does, then takes it off the page, and says so.
  function hideRow(row) {
    var api = window.ApiClient
    var user = api.getCurrentUserId()
    api.ajax({ type: 'POST', url: api.getUrl('Polyfin/HomeRows/' + row.RowId + '/Hidden') }).then(
      function () {
        if (!answer || answer.user !== user) return
        answer.rows = answer.rows.filter(function (r) {
          return r.RowId !== row.RowId
        })
        rowElementsOf(row).forEach(function (element) {
          element.parentNode.removeChild(element)
        })
        toast(say('hidden'))
      },
      function () {
        toast(say('notHidden'))
      }
    )
  }
  // rowElementsOf lists the elements drawn of row: those of a Because you
  // watched row are several.
  function rowElementsOf(row) {
    var container = answer && answer.container
    if (!container) return []
    return Array.prototype.filter.call(container.querySelectorAll('.polyfinHomeRow'), function (element) {
      return element.getAttribute('data-polyfin-rowid') === row.RowId
    })
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds.
  function toast(text) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = made('div', 'toastContainer')
      document.body.appendChild(holder)
    }
    var shown = made('div', 'toast', {}, [text])
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
  // style adds the look of the notes, the badges and the Hide buttons,
  // once.
  function style() {
    if (document.getElementById('polyfinRowsStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinRowsStyle'
    sheet.textContent =
      '.polyfinRowNote{margin:-.3em 0 .4em;font-size:.95em;line-height:1.4;opacity:.85;max-width:70ch}' +
      '.polyfinRowNew{margin-left:.6em;padding:.15em .6em;border-radius:1em;background:#00a4dc;color:#fff;font-size:.75em;' +
      'font-weight:600;letter-spacing:.02em;white-space:nowrap}' +
      '.polyfinRowHide{margin-left:.6em;display:inline-flex;align-items:center;gap:.25em;opacity:.8}' +
      '.polyfinRowHide .material-icons{font-size:1.2em}'
    document.head.appendChild(sheet)
  }
  // A row's posters load as they come near the screen, as jellyfin-web
  // loads its own: a row holds up to 20, and a home page several rows,
  // which a phone would otherwise all load at once. Each is asked at the
  // width its card shows it at, which jellyfin-web's styles give the
  // row's cards (on a phone, about half the 272 pixels of a desktop's),
  // and half as tall again; getScaledImageUrl multiplies both by the
  // screen's pixel ratio. A browser without IntersectionObserver loads
  // them at once, 272 wide.
  var posterWatcher = null
  function loadPoster(image) {
    var load = image.polyfinPoster
    if (!load) return
    image.polyfinPoster = null
    var width = image.offsetWidth > 0 ? Math.ceil(image.offsetWidth) : 272
    image.style.backgroundImage = 'url("' + load(width) + '")'
  }
  function lazyPoster(image, load) {
    image.polyfinPoster = load
    if (typeof IntersectionObserver !== 'function') return loadPoster(image)
    if (!posterWatcher) {
      // Half a screen ahead, so that a poster is there as its row scrolls.
      posterWatcher = new IntersectionObserver(
        function (entries) {
          for (var i = 0; i < entries.length; i++) {
            if (!entries[i].isIntersecting) continue
            posterWatcher.unobserve(entries[i].target)
            try {
              loadPoster(entries[i].target)
            } catch (error) {}
          }
        },
        { rootMargin: '50%' }
      )
    }
    posterWatcher.observe(image)
  }
  // card draws a title as jellyfin-web's cards of a row of posters. An
  // episode shows its series' poster, the series' name, then its season
  // and episode numbers before its own name, as jellyfin-web shows one.
  function card(api, server, item, index) {
    var link = '#/details?id=' + encodeURIComponent(item.Id) + '&serverId=' + encodeURIComponent(server)
    var data = item.UserData || {}
    var episode = item.Type === 'Episode'
    var poster = episode && item.SeriesId && item.SeriesPrimaryImageTag
    var tag = poster ? item.SeriesPrimaryImageTag : item.ImageTags && item.ImageTags.Primary
    var image = made('a', 'cardImageContainer coveredImage cardContent itemAction', {
      href: link,
      'data-action': 'link',
      'aria-label': item.Name || '',
      role: 'img',
    })
    if (tag) {
      var target = poster ? item.SeriesId : item.Id
      lazyPoster(image, function (width) {
        return api.getScaledImageUrl(target, {
          type: 'Primary',
          fillHeight: Math.round(width * 1.5),
          fillWidth: width,
          quality: 96,
          tag: tag,
        })
      })
    } else {
      image.appendChild(icon(item.Type === 'Series' || episode ? 'tv' : item.Type === 'BoxSet' ? 'folder' : 'movie', 'cardImageIcon'))
    }
    if (data.UnplayedItemCount) {
      image.appendChild(
        made('div', 'cardIndicators', {}, [
          made('div', 'countIndicator indicator', {}, [
            String(Math.min(data.UnplayedItemCount, 99)) + (data.UnplayedItemCount > 99 ? '+' : ''),
          ]),
        ])
      )
    } else if (data.Played) {
      image.appendChild(made('div', 'cardIndicators', {}, [made('div', 'playedIndicator indicator', {}, [icon('check', 'indicatorIcon')])]))
    }
    if (!data.Played && data.PlayedPercentage > 0 && data.PlayedPercentage < 100) {
      var bar = made('div', 'itemProgressBarForeground')
      bar.style.width = data.PlayedPercentage + '%'
      image.appendChild(
        made('div', 'innerCardFooter fullInnerCardFooter innerCardFooterClear', {}, [made('div', 'itemProgressBar', {}, [bar])])
      )
    }
    var overlay = made('div', 'cardOverlayContainer itemAction', { 'data-action': 'link' }, [
      made(
        'button',
        'cardOverlayButton cardOverlayButton-hover itemAction paper-icon-button-light cardOverlayFab-primary',
        {
          is: 'paper-icon-button-light',
          type: 'button',
          'data-action': 'resume',
        },
        [icon('play_arrow', 'cardOverlayButtonIcon cardOverlayButtonIcon-hover')]
      ),
      made('div', 'cardOverlayButton-br flex', {}, [
        made(
          'button',
          'cardOverlayButton cardOverlayButton-hover itemAction paper-icon-button-light',
          {
            is: 'paper-icon-button-light',
            type: 'button',
            'data-action': 'menu',
          },
          [icon('more_vert', 'cardOverlayButtonIcon cardOverlayButtonIcon-hover')]
        ),
      ]),
    ])
    var numbered =
      item.ParentIndexNumber != null && item.IndexNumber != null
        ? 'S' + item.ParentIndexNumber + ':E' + item.IndexNumber + ' - ' + (item.Name || '')
        : item.Name || ''
    var first = episode && item.SeriesName ? item.SeriesName : item.Name || ''
    var second = episode && item.SeriesName ? numbered : item.ProductionYear ? String(item.ProductionYear) : '\u00a0'
    var name = made('div', 'cardText cardTextCentered cardText-first', {}, [
      made('bdi', '', {}, [made('a', 'itemAction textActionButton', { href: link, 'data-action': 'link' }, [first])]),
    ])
    var year = made('div', 'cardText cardTextCentered cardText-secondary', {}, [made('bdi', '', {}, [second])])
    return made(
      'div',
      'card overflowPortraitCard card-hoverable card-withuserdata',
      {
        'data-index': String(index),
        'data-isfolder': item.IsFolder ? 'true' : 'false',
        'data-serverid': server,
        'data-id': item.Id,
        'data-type': item.Type || '',
        'data-mediatype': item.MediaType || '',
        'data-context': 'home',
      },
      [
        made('div', 'cardBox cardBox-bottompadded', {}, [
          made('div', 'cardScalable', {}, [made('div', 'cardPadder cardPadder-overflowPortrait'), image, overlay]),
          name,
          year,
        ]),
      ]
    )
  }
  // watch checks the page as jellyfin-web changes it (see
  // core/page-changes.js), the route included.
  function watch() {
    function checked() {
      try {
        check()
      } catch (error) {}
    }
    document.addEventListener('polyfinpagechange', checked)
    addEventListener('hashchange', checked)
    addEventListener('popstate', checked)
    checked()
  }
  if (document.body) watch()
  else addEventListener('DOMContentLoaded', watch)
