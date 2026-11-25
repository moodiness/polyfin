// Part 10: Polyfin's entries in the item menus.
//
// It adds Polyfin's own entries to jellyfin-web's item menu, the
// action sheet a card's ⋮ button or a right click on a card opens, the ⋮
// button of a title's page (.btnMoreCommands), and the ⋮ of a selection
// (.btnSelectionPanelOptions) holding one card, which a long press makes
// on a touch screen, where cards of the home's rows have no ⋮ and a long
// press opens no menu; acting leaves the selection:
//   "Remove from Continue Watching", for a card of a Continue Watching row:
//     a row whose cards all have a resume point, as /UserItems/Resume
//     lists them, the libraries' own rows included. It deletes
//     /Polyfin/UserItems/{id}/Resume and takes the card off the row;
//   "Remove from Next Up", for an episode's card in a Next Up list: the
//     home's section whose title links to the Next Up page
//     (#/list?type=nextup), that page, the Next Up of a TV library's
//     tab (#nextUpItems), or the Next Up of a series' page
//     (.nextUpSection). It deletes /Polyfin/UserItems/{id}/NextUp, takes
//     the card off a list (a series' page keeps its next episode) and says
//     until when in a toast;
//   "Hide this title", for a movie, a home video or a series, or a season
//     or an episode of one, that the user has not hidden, as
//     /Polyfin/UserItems/Hidden lists them. It posts to
//     /Polyfin/UserItems/{id}/Hidden, takes the cards of the title off the
//     page and says so in a toast, as jellyfin-web's own;
//   "Show this title again", in its place for a title the user hid, its
//     series hidden counting for a season or an episode, which deletes
//     /Polyfin/UserItems/{id}/Hidden;
//   "Add to Watchlist", for a movie, an episode, a season or a series
//     that is not on the user's watchlist playlist, as
//     /Polyfin/UserItems/Watchlist lists its entries and the titles it
//     holds pending (a series none of whose episodes aired): a title is
//     on it when it is an entry or pending, a season or a series also
//     when an entry is an episode of it (its SeasonId or SeriesId). It
//     posts the title to /Polyfin/UserItems/{id}/Watchlist, which puts a
//     movie or an episode at the top of the playlist, a season or a series
//     its aired episodes, and makes the playlist again when the user
//     deleted it; and says so in a toast;
//   "Remove from Watchlist", in its place for a title on the watchlist,
//     which deletes /Polyfin/UserItems/{id}/Watchlist: a movie or an
//     episode goes off, a season or a series takes every entry of it off.
//     It takes the cards of the title, and of its entries, off the rows of
//     the watchlist and says so in a toast. A row of the watchlist is one
//     of Polyfin's Now available and Still waiting home rows (see the
//     eleventh), or the list of the user's watchlist playlist on its page
//     (#/details?id= it, its Id the PlaylistId /Polyfin/UserItems/Watchlist
//     gives, empty while the user has none); the title's cards elsewhere
//     stay;
//   "Watch together", on a movie's or an episode's page, for a user whose
//     SyncPlay access is Create and join (Policy.SyncPlayAccess
//     CreateAndJoinGroups, as ApiClient.getCurrentUser gives it), while
//     jellyfin-web shows its SyncPlay button: the default layout's
//     (aria-controls app-sync-play-menu) or the legacy layouts'
//     (.headerSyncButton), which it shows only with its SyncPlay plugin
//     and an access other than None. It starts the title in a SyncPlay
//     group with Jellyfin's own routes: in the group the page's session
//     is in, as the socket's SyncPlayGroupUpdate messages tell (GroupJoined,
//     until GroupLeft, NotInGroup or GroupDoesNotExist) and
//     GET /SyncPlay/{id} confirms, else in a new one (POST /SyncPlay/New,
//     named as jellyfin-web names its own). A new group starts with a
//     Stop, which jellyfin-web applies once its clock is set to the
//     server's, about a second later, pinging the group then
//     (ApiClient.sendSyncPlayPing): the script waits for that ping, 10
//     seconds at most, so that the Stop finds nothing playing. It sets the
//     title as the group's queue, from its start (POST
//     /SyncPlay/SetNewQueue), then pauses the group (POST /SyncPlay/Pause)
//     while it waits for its members to load the title: jellyfin-web
//     pauses a title it starts for a group and tells it ready, and the
//     group, paused meanwhile, stays paused once all are, until someone
//     presses Play. Polyfin starts a queue only when every member may play
//     it, the group then waiting (State Waiting), which GET /SyncPlay/{id}
//     checks before the pause. It then opens the picker of the people to
//     invite (below). A title without a known version, as
//     /Polyfin/Items/{id}/Versions tells, a queue the group refused, and
//     SyncPlay refused (403, the user's access asked again telling None
//     or Join only) each get a toast instead. Used again in a group that
//     plays the title, as the socket's PlayQueue updates tell, the entry
//     only opens the picker again.
//     The picker lists the people GET /Polyfin/SyncPlay/Invitees gives:
//     those the session's group may invite now, who have a web player
//     open and could join and play the title. It is a dialog over the
//     dimmed page, with a box to tick for each, Invite and Skip; Escape
//     or a click beside it skips, Tab and the arrows move between its
//     controls, Enter also ticks a box. Invite posts each person picked
//     to /Polyfin/SyncPlay/Invitations ({GroupId, UserId}) and says in a
//     toast who was invited, who was invited less than a minute ago
//     (429), and whether any failed. Skip, or nobody to invite, says in a
//     toast how the others join from their own device; a picker opened
//     again says nobody can be invited instead, and skips silently.
//     An invitation reaches the invitee's web players as a
//     PolyfinSyncPlayInvite message on the socket, State Invited, with
//     its Id, GroupId, HostName, and the title's ItemName and SeriesName.
//     It shows a prompt at the top right of the page, across the bottom
//     on a narrow screen, which leaves the page usable: who invites, the
//     title, Join, and Not now. The prompt takes the focus itself, not
//     Join, so a key meant for the page joins nothing; while the user
//     types in a field or a video plays, it leaves the focus where it is.
//     Join joins the group with
//     Jellyfin's POST /SyncPlay/Join, jellyfin-web then following the
//     group as when its SyncPlay menu joins one; Not now, or Escape,
//     posts /Polyfin/SyncPlay/Invitations/{Id}/Decline. Another
//     invitation replaces the prompt; a message of the same Id telling it
//     ended (Joined, Declined, Expired or Ended) closes it, as joining or
//     declining on another of the user's devices does, and it goes by
//     itself after the 5 minutes an invitation lasts.
// As the button is pressed or the card right-clicked, before jellyfin-web
// opens its menu, the script asks for the item (ApiClient.getItem), the
// hidden titles, the titles of the watchlist, for a card with a resume
// point, Continue Watching, and on a title's page, the user; the first
// action sheet that opens within 10 seconds, no other click coming
// between, gets the entries once the answers come. An entry closes the
// menu as Back does, then acts.
// jellyfin-web 12.4 keeps the answers of its rows for a minute, in memory
// and in the browser's IndexedDB, and draws a row from such an answer,
// asking nothing, when the change's UserDataChanged message refreshes it
// or when the page loads again within that minute. The script keeps what
// its entries took off in the browser's session storage, for the server
// and user, for 2 minutes, and has every row's answers (their fetchData)
// leave out, meanwhile, the item taken off Continue Watching from rows
// whose items all resume, unless played since its last play then, the
// episodes of a series taken off Next Up from the Next Up lists, unless
// played since, the titles hidden, with what is in them, from every row,
// and the titles taken off the watchlist from the rows of the watchlist.
// For as long, after a title is put on the watchlist or taken off, the
// rows of the watchlist ask Polyfin each time they draw, as the home's
// rows do after a played mark (see below), so that a title put on it
// shows there. Nothing is kept longer: an older answer is asked again,
// and Polyfin's leaves them out. Its words follow jellyfin-web's
// language, as the third's. Any error just stops it.
//
// With its entries, it also has the home page's Next Up, Continue Watching
// and Continue Listening rows (the home's rows that jellyfin-web refreshes
// on a played mark: data-monitor names markplayed) ask Polyfin again for a
// minute after a change of what the user played. jellyfin-web refreshes
// these rows on a UserDataChanged message, 10 seconds later, but draws
// them from its answers of the last minute, asking nothing
// (jellyfin/jellyfin-web#8543): a card marked played stayed as it was.
// When a UserDataChanged message, or the answer of a played mark the page
// sends (ApiClient.markPlayed and markUnplayed, as a card's ✓ does),
// changes Played or the resume point of an item of these rows, as the
// script last saw their answers, it refreshes the rows at once, and for a
// minute every refresh of theirs asks Polyfin. The change is kept as the
// item's state, so that the same change heard both ways refreshes them
// once.
// jellyfin-web's query client reuses an answer while Date.now() is less
// than a minute after it came: during the row's own fetchData call only,
// which asks the query client synchronously, Date.now() reads a minute
// later, so the query client asks Polyfin with the row's own request and
// keeps the new answer. Other rows, and anything else, are left as they
// are. Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: details
//   route: list
//   selector: .card[data-id]
//   selector: .card[data-positionticks]
//   selector: .btnMoreCommands
//   selector: .btnSelectionPanelOptions
//   selector: .chkItemSelect:checked
//   selector: button[data-action="menu"]
//   selector: .card[data-id], .listItem[data-id]
//   selector: button[aria-controls="app-sync-play-menu"], .headerSyncButton:not(.hide)
//   selector: .actionSheet
//   selector: .actionSheetScroller
//   selector: .actionSheetMenuItem
//   selector: .actionsheetMenuItemIcon
//   selector: .actionSheetItemText
//   selector: .actionSheetItemAsideText
//   selector: .dialog[data-history]
//   selector: .selectionCommandsPanel .btnCloseSelectionPanel
//   selector: .nextUpSection
//   selector: #nextUpItems
//   selector: .verticalSection
//   selector: .sectionTitleContainer a[href*="type=nextup"]
//   selector: .page.hide
//   selector: .homeSectionsContainer .itemsContainer[data-monitor]
//   selector: .card, .listItem
//   selector: .toastContainer
//   class: emby-button
//   class: raised
//   class: button-submit
//   class: button-cancel
//   class: toast
//   class: toastVisible
//   class: toastHide
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getCurrentUser
//   global: ApiClient.getItem
//   global: ApiClient.getUser
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.ajax
//   global: ApiClient.serverId
//   property: fetchData
//   property: notifyRefreshNeeded
//   text: app-sync-play-menu
//   text: markplayed
//   text: type=nextup

  // The item the last menu button pressed, or card right-clicked, opens a
  // menu for (see the header): its identifier, its card, when, and the
  // promise of what Polyfin tells of it; and how soon its menu must open.
  var aimed = null
  var menuWithin = 10000
  // aim follows the presses of the menu buttons of cards and of title
  // pages, and right clicks on cards; any other click forgets the item.
  function aim(event) {
    aimed = null
    var target = event.target
    var api = window.ApiClient
    if (!target || !target.closest || !api) return
    var card = null
    var id = ''
    if (event.type === 'contextmenu') {
      card = target.closest('.card[data-id]')
    } else if (target.closest('.btnMoreCommands')) {
      var route = titleRoute.exec(location.hash)
      id = route ? route[1] : ''
    } else if (target.closest('.btnSelectionPanelOptions')) {
      // On a touch screen, a long press selects a card instead of opening
      // its menu: the selection's ⋮ stands for the card selected alone.
      var selected = document.querySelectorAll('.chkItemSelect:checked')
      card = selected.length === 1 ? selected[0].closest('.card[data-id]') : null
    } else {
      var button = target.closest('button[data-action="menu"]')
      card = button && button.closest('.card[data-id], .listItem[data-id]')
    }
    if (card) id = card.getAttribute('data-id') || ''
    if (!id) return
    aimed = { id: plainId(id), card: card, at: Date.now(), facts: askFacts(api, plainId(id), card) }
  }
  function plainId(id) {
    return String(id).replace(/-/g, '').toLowerCase()
  }
  // askFacts asks for what the entries of the item's menu depend on: its
  // type and series, whether the user hid it or its series, whether it is
  // on the user's watchlist and which entries stand for it there, whether
  // its card stands in a Continue Watching row, in which Next Up it does,
  // and, on a title's page, whether the user may watch it together.
  function askFacts(api, id, card) {
    var user = api.getCurrentUserId()
    var row = card && card.getAttribute('data-positionticks') ? card.closest('.itemsContainer') : null
    var nextUp = nextUpRow(card)
    var resume = row
      ? api.getJSON(api.getUrl('UserItems/Resume', { userId: user, Limit: 100, MediaTypes: 'Video', EnableImages: false }))
      : null
    // The user signed in, which jellyfin-web keeps, for their SyncPlay
    // access; none leaves Watch together out.
    var viewer =
      !card && typeof api.getCurrentUser === 'function'
        ? api.getCurrentUser().then(null, function () {
            return null
          })
        : null
    var asked = [
      api.getItem(user, id),
      api.getJSON(api.getUrl('Polyfin/UserItems/Hidden')),
      resume,
      api.getJSON(api.getUrl('Polyfin/UserItems/Watchlist')),
      viewer,
    ]
    return Promise.all(asked).then(function (answers) {
      var item = answers[0] || {}
      var series = item.SeriesId ? plainId(item.SeriesId) : ''
      function listed(answer) {
        var ids = {}
        ;((answer && answer.Items) || []).forEach(function (listed) {
          ids[plainId(listed.Id)] = true
        })
        return ids
      }
      var hidden = listed(answers[1])
      var resumable = listed(answers[2])
      var watchlist = answers[3] || {}
      if (answers[3]) learnWatchlist(answers[3])
      // A movie, an episode, a season or a series is acted on itself: a
      // season or a series stands for its entries (whole). A title Polyfin
      // holds pending, a series none of whose episodes aired, is listed by
      // its own Id.
      var whole = item.Type === 'Series' ? 'series' : item.Type === 'Season' ? 'season' : ''
      var entries = (watchlist.Items || []).map(function (held) {
        return {
          id: plainId(held.Id),
          series: held.SeriesId ? plainId(held.SeriesId) : '',
          season: held.SeasonId ? plainId(held.SeasonId) : '',
        }
      })
      var held = entries.filter(function (held) {
        return held.id === id || (!!whole && held[whole] === id)
      })
      var cards = row ? row.querySelectorAll('.card[data-id]') : []
      return {
        series: series,
        hideable: /^(Movie|Video|Series)$/.test(item.Type) || (/^(Season|Episode)$/.test(item.Type) && !!series),
        hidden: !!(hidden[id] || (series && hidden[series])),
        // The watchlist's entries are movies and episodes: a movie or an
        // episode is on it when it is an entry, a season or a series when
        // an episode of it is.
        watchable: /^(Movie|Series|Season|Episode)$/.test(item.Type),
        watchlisted: held.length > 0,
        watchlist: {
          title: id,
          whole: whole,
          series: whole === 'series' ? id : series,
          season: whole === 'season' ? id : item.SeasonId ? plainId(item.SeasonId) : '',
          ids: [id].concat(
            held.map(function (held) {
              return held.id
            })
          ),
        },
        resume:
          cards.length > 0 &&
          Array.prototype.every.call(cards, function (other) {
            return resumable[plainId(other.getAttribute('data-id'))]
          }),
        nextUp: item.Type === 'Episode' && series ? nextUp : '',
        // A movie or an episode, for a user who may start a SyncPlay group
        // where jellyfin-web offers SyncPlay (see the header).
        together:
          /^(Movie|Episode)$/.test(item.Type) && !!answers[4] && mayStartGroups(answers[4]) && syncPlayOffered()
            ? answers[4]
            : null,
      }
    })
  }
  function mayStartGroups(user) {
    return !!user.Policy && user.Policy.SyncPlayAccess === 'CreateAndJoinGroups'
  }
  // syncPlayOffered tells whether jellyfin-web shows its SyncPlay button,
  // from which the others join: in the default layout's top bar, or the
  // legacy layouts' header.
  function syncPlayOffered() {
    return !!document.querySelector('button[aria-controls="app-sync-play-menu"], .headerSyncButton:not(.hide)')
  }
  // extendMenu adds the entries to the item menu a dialog container holds,
  // when it opens for the item aimed at, once Polyfin answered.
  function extendMenu(container) {
    var sheet = container.querySelector && container.querySelector('.actionSheet')
    var entry = aimed
    if (!sheet || !entry || Date.now() - entry.at > menuWithin) return
    aimed = null
    entry.facts
      .then(function (facts) {
        var scroller = sheet.querySelector('.actionSheetScroller')
        var model = scroller && scroller.querySelector('.actionSheetMenuItem')
        if (!attached(container) || !model) return
        if (facts.resume) {
          scroller.appendChild(
            menuItem(container, model, 'playlist_remove', say('removeResume'), function () {
              act(entry.id, 'DELETE', 'Resume', function (data) {
                // The answer is what the user did with the item afterwards:
                // its last play, before the resume point went.
                keepDrop({ kind: 'resume', id: entry.id, last: Date.parse((data && data.LastPlayedDate) || '') || 0 })
                dropCards(entry.card ? [entry.card] : [])
              })
            })
          )
        }
        if (facts.nextUp) {
          scroller.appendChild(
            menuItem(container, model, 'remove_from_queue', say('removeNextUp'), function () {
              act(entry.id, 'DELETE', 'NextUp', function () {
                keepDrop({ kind: 'nextup', id: facts.series, at: Date.now() })
                // A series' page keeps its next episode: only Next Up's
                // lists leave the series out.
                if (facts.nextUp === 'list' && entry.card) dropCards([entry.card])
                toast(say('removedNextUp'))
              })
            })
          )
        }
        if (facts.hideable && facts.hidden) {
          scroller.appendChild(
            menuItem(container, model, 'visibility', say('show'), function () {
              act(entry.id, 'DELETE', 'Hidden', function () {
                forgetDrops('hidden', [entry.id, facts.series])
                toast(say('shown'))
              })
            })
          )
        } else if (facts.hideable) {
          scroller.appendChild(
            menuItem(container, model, 'visibility_off', say('hide'), function () {
              act(entry.id, 'POST', 'Hidden', function () {
                var titles = [entry.id, facts.series].filter(Boolean)
                titles.forEach(function (title) {
                  keepDrop({ kind: 'hidden', id: title })
                })
                var cards = Array.prototype.filter.call(document.querySelectorAll('.card[data-id]'), function (card) {
                  return titles.indexOf(plainId(card.getAttribute('data-id'))) >= 0
                })
                if (entry.card && cards.indexOf(entry.card) < 0) cards.push(entry.card)
                dropCards(cards)
                toast(say('hidden'))
              })
            })
          )
        }
        // A movie or an episode goes on the watchlist and off it itself; a
        // season or a series puts its aired episodes on, and takes every
        // entry of it off.
        var listing = facts.watchlist
        if (facts.watchable && facts.watchlisted) {
          scroller.appendChild(
            menuItem(container, model, 'bookmark_remove', say('removeWatchlist'), function () {
              act(listing.title, 'DELETE', 'Watchlist', function () {
                keepDrop({
                  kind: 'watchlist',
                  id: listing.title,
                  series: listing.series,
                  season: listing.season,
                  whole: listing.whole,
                  playlist: knownWatchlist(),
                })
                var cards = Array.prototype.filter.call(document.querySelectorAll('.card[data-id], .listItem[data-id]'), function (card) {
                  return listing.ids.indexOf(plainId(card.getAttribute('data-id'))) >= 0 && watchlistRow(card)
                })
                if (entry.card && cards.indexOf(entry.card) < 0 && watchlistRow(entry.card)) cards.push(entry.card)
                dropCards(cards)
                toast(say('removedWatchlist'))
              })
            })
          )
        } else if (facts.watchable) {
          scroller.appendChild(
            menuItem(container, model, 'bookmark_add', say('addWatchlist'), function () {
              act(listing.title, 'POST', 'Watchlist', function () {
                forgetUnlisted(listing)
                var drop = { kind: 'watchlisted', id: listing.title, playlist: knownWatchlist() }
                keepDrop(drop)
                toast(say('addedWatchlist'))
                // A watchlist the user deleted is made again by the add:
                // its page is known once Polyfin gives its Id.
                if (drop.playlist) return
                var api = window.ApiClient
                api.getJSON(api.getUrl('Polyfin/UserItems/Watchlist')).then(
                  quietly(function (answer) {
                    learnWatchlist(answer)
                    drop.playlist = knownWatchlist()
                    saveDrops()
                  }),
                  function () {}
                )
              })
            })
          )
        }
        if (facts.together) {
          scroller.appendChild(
            menuItem(container, model, 'groups', say('watchTogether'), function () {
              watchTogether(entry.id, facts.together)
            })
          )
        }
        // The menu was placed for its own entries: it moves up, and left, as
        // far as the added ones need, measured without the transform of its
        // opening animation, which scales it down at first.
        var box = sheet.getBoundingClientRect()
        var top = parseFloat(sheet.style.top)
        var tall = sheet.offsetHeight || box.height
        if (sheet.style.top && top + tall > window.innerHeight) {
          sheet.style.top = Math.max(0, window.innerHeight - tall) + 'px'
        }
        var width = document.documentElement.clientWidth || window.innerWidth
        var left = parseFloat(sheet.style.left)
        var wide = sheet.offsetWidth || box.width
        if (left > 0 && left + wide > width) {
          sheet.style.left = Math.max(0, width - wide) + 'px'
        }
      })
      .catch(function () {})
  }
  // menuItem makes an entry as a copy of one of the menu's own, with its
  // own icon and words, which closes the menu, as Back does, then runs.
  function menuItem(container, model, icon, text, run) {
    var item = model.cloneNode(true)
    item.setAttribute('data-id', 'polyfin')
    item.classList.add('polyfinMenuItem')
    var mark = item.querySelector('.actionsheetMenuItemIcon')
    if (mark) mark.className = mark.className.replace(/\S+$/, icon)
    var words = item.querySelector('.actionSheetItemText')
    if (words) words.textContent = text
    var aside = item.querySelector('.actionSheetItemAsideText')
    if (aside) aside.remove()
    item.addEventListener(
      'click',
      quietly(function (event) {
        // Before the menu hears the click, which would ask jellyfin-web to
        // run a command it does not know.
        event.stopPropagation()
        event.preventDefault()
        var dialog = container.querySelector('.dialog')
        if (dialog && dialog.getAttribute('data-history') === 'true') history.back()
        else container.remove()
        run()
        // A menu of the selection leaves the selection, as jellyfin-web's
        // own commands do.
        var unselect = document.querySelector('.selectionCommandsPanel .btnCloseSelectionPanel')
        if (unselect) unselect.click()
      }),
      true
    )
    return item
  }
  // act sends a change of the user's lists, then runs done with the
  // answer: what the user did with the item, for Resume.
  function act(id, method, path, done) {
    var api = window.ApiClient
    var request = { type: method, url: api.getUrl('Polyfin/UserItems/' + id + '/' + path) }
    if (path === 'Resume') request.dataType = 'json'
    api.ajax(request).then(quietly(done), function () {})
  }
  // The SyncPlay group the page's session is in, as the socket's
  // SyncPlayGroupUpdate messages tell it, with the user signed in then,
  // and the title it plays, as its PlayQueue updates tell. Polyfin takes a
  // session out of its group as its socket closes, as a page loaded again
  // or another user signing in does.
  var syncGroup = null
  function heardGroup(message) {
    var update = message && message.Data
    if (!update) return
    if (update.Type === 'GroupJoined' && update.GroupId) {
      var api = window.ApiClient
      syncGroup = { user: api ? api.getCurrentUserId() : '', id: String(update.GroupId), item: '' }
    } else if (/^(GroupLeft|NotInGroup|GroupDoesNotExist)$/.test(update.Type)) {
      syncGroup = null
    } else if (update.Type === 'PlayQueue' && syncGroup && update.Data) {
      var playing = (update.Data.Playlist || [])[update.Data.PlayingItemIndex]
      syncGroup.item = playing && playing.ItemId ? plainId(playing.ItemId) : ''
    }
  }
  // Those waiting for jellyfin-web's next ping of its SyncPlay group.
  var pingWaiters = []
  function pinged() {
    var waiters = pingWaiters
    pingWaiters = []
    waiters.forEach(function (resolve) {
      resolve()
    })
  }
  // nextPing resolves at jellyfin-web's next ping, or within milliseconds
  // at the latest.
  function nextPing(within) {
    return new Promise(function (resolve) {
      pingWaiters.push(resolve)
      setTimeout(resolve, within)
    })
  }
  // watchTogether starts the title id in a SyncPlay group of host, the
  // user signed in, paused at its start (see the header), and opens the
  // picker of the people to invite, or says why it could not. In a group
  // that plays the title already, it only opens the picker again.
  function watchTogether(id, host) {
    var api = window.ApiClient
    var user = api.getCurrentUserId()
    var group = syncGroup && syncGroup.user === user ? syncGroup.id : ''
    if (group && syncGroup.item === id) {
      pickInvitees(true)
      return
    }
    function send(method, path, body) {
      var request = { type: method, url: api.getUrl(path) }
      if (body) {
        request.data = JSON.stringify(body)
        request.contentType = 'application/json'
      }
      if (method === 'GET' || path === 'SyncPlay/New') request.dataType = 'json'
      return api.ajax(request)
    }
    function refusal(key) {
      return { refused: key }
    }
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Versions'))
      .then(function (progress) {
        // A title without a known version would not play: jellyfin-web
        // holds its play buttons meanwhile (see the third).
        if (progress && progress.Count >= 1 && progress.Known === 0) {
          throw refusal(progress.Pending > 0 ? 'togetherSearching' : 'none')
        }
        // The group the session is in, unless it ended meanwhile.
        if (!group) return null
        return send('GET', 'SyncPlay/' + group).then(null, function () {
          return null
        })
      })
      .then(function (current) {
        if (current) return current
        // A new group starts stopped: it tells jellyfin-web to stop, which
        // jellyfin-web does once its clock is set to the server's, about
        // a second later, and would stop the title started meanwhile. It
        // pings the group then, the stop done.
        var ready = nextPing(10000)
        var groupName = say('groupName').replace('{name}', function () {
          return host.Name || ''
        })
        return send('POST', 'SyncPlay/New', { GroupName: groupName }).then(function (joined) {
          return ready.then(function () {
            return joined
          })
        })
      })
      .then(function (joined) {
        group = joined.GroupId
        return send('POST', 'SyncPlay/SetNewQueue', { PlayingQueue: [id], PlayingItemPosition: 0, StartPositionTicks: 0 })
      })
      .then(function () {
        return send('GET', 'SyncPlay/' + group)
      })
      .then(function (current) {
        // A queue some member may not play leaves the group as it was.
        if (!current || current.State !== 'Waiting') throw refusal('togetherRefused')
        return send('POST', 'SyncPlay/Pause')
      })
      .then(function () {
        return pickInvitees(false)
      })
      .catch(function (error) {
        if (error && error.refused) return toast(say(error.refused))
        if (!error || error.status !== 403 || typeof api.getUser !== 'function') return toast(say('togetherFailed'))
        // SyncPlay refused: the user's access changed since jellyfin-web
        // asked for it.
        return api.getUser(user).then(
          function (fresh) {
            var access = fresh && fresh.Policy ? fresh.Policy.SyncPlayAccess : ''
            toast(say(access === 'None' ? 'togetherOff' : access === 'JoinGroups' ? 'togetherJoinOnly' : 'togetherFailed'))
          },
          function () {
            toast(say('togetherFailed'))
          }
        )
      })
      .catch(function () {})
  }
  // The invitations to watch together (see the header): the picker of the
  // people the session's group may invite, and the prompt an invitation
  // shows, each with the element it is and the one focused before.
  var picker = null
  var prompt = null
  // inviteLasts is how long an invitation lasts, as Polyfin keeps it.
  var inviteLasts = 300000
  // pickInvitees asks whom the session's group may invite, and opens the
  // picker with them; again when the entry opens it for a group under
  // way. With nobody to invite, it says how the others join instead.
  function pickInvitees(again) {
    var api = window.ApiClient
    return api.getJSON(api.getUrl('Polyfin/SyncPlay/Invitees')).then(
      function (answer) {
        var people = (answer && answer.Items) || []
        if (people.length && answer.GroupId) openPicker(String(answer.GroupId), people, again)
        else if (again) toast(say('inviteNobody'))
        else toast(say('togetherStarted'), 10000)
      },
      function () {
        if (again) toast(say('inviteFailed'))
        else toast(say('togetherStarted'), 10000)
      }
    )
  }
  // closeInvite takes the picker or the prompt off the page, and gives the
  // focus back when it held it.
  function closeInvite(shown) {
    if (!shown) return
    clearTimeout(shown.timer)
    var focused = document.activeElement
    var held = !!focused && typeof shown.node.contains === 'function' && shown.node.contains(focused)
    if (shown.node.parentNode) shown.node.parentNode.removeChild(shown.node)
    if (held && shown.restore && typeof shown.restore.focus === 'function') shown.restore.focus()
  }
  function inviteButton(className, text) {
    var made = element('button', 'emby-button raised ' + className, text)
    made.setAttribute('type', 'button')
    return made
  }
  // inviteKeys keeps the keyboard in a dialog: Tab and the arrows move
  // between its controls, as a remote's arrows do, Enter ticks a box, and
  // Escape runs leave. From the dialog itself, which the prompt focuses,
  // every one of them goes to the first control, the one that declines:
  // no single key leads from the page to Join. jellyfin-web hears none of
  // these keys meanwhile.
  function inviteKeys(dialog, leave) {
    dialog.addEventListener(
      'keydown',
      quietly(function (event) {
        var key = event.key
        var focused = document.activeElement
        if (key === 'Escape' || key === 'Esc') {
          leave()
        } else if (key === 'Enter' && focused && focused.type === 'checkbox') {
          focused.click()
        } else if (key === 'Tab' || /^(Arrow)?(Up|Down|Left|Right)$/.test(key)) {
          var stops = Array.prototype.filter.call(dialog.querySelectorAll('input, button'), function (stop) {
            return !stop.disabled
          })
          var at = stops.indexOf(focused)
          var back = key === 'Tab' ? !!event.shiftKey : /Up|Left/.test(key)
          var next = at < 0 ? stops[0] : stops[(at + (back ? stops.length - 1 : 1)) % stops.length]
          if (next) next.focus()
        } else {
          return
        }
        event.preventDefault()
        event.stopPropagation()
      })
    )
  }
  // openPicker lists people, whom the group may invite, each with a box to
  // tick, Invite, and Skip, which says how the others join instead.
  function openPicker(group, people, again) {
    closeInvite(picker)
    inviteStyle()
    var send = inviteButton('button-submit polyfinInviteSend', say('inviteSend'))
    var skip = inviteButton('button-cancel polyfinInviteSkip', say('inviteSkip'))
    send.disabled = true
    var boxes = people.map(function (person) {
      var box = document.createElement('input')
      box.setAttribute('type', 'checkbox')
      box.type = 'checkbox'
      box.addEventListener(
        'change',
        quietly(function () {
          send.disabled = !boxes.some(function (other) {
            return other.checked
          })
        })
      )
      return box
    })
    var list = element(
      'div',
      'polyfinInviteList',
      people.map(function (person, i) {
        return element('label', 'polyfinInvitee', [boxes[i], element('span', 'polyfinInviteName', String(person.Name || ''))])
      })
    )
    var heading = element('h2', 'polyfinInviteTitle', say('watchTogether'))
    heading.id = 'polyfinInviteTitle'
    var text = element('p', 'polyfinInviteText', say(again ? 'inviteMore' : 'inviteIntro'))
    text.id = 'polyfinInviteText'
    var dialog = element('div', 'polyfinInvite', [heading, text, list, element('div', 'polyfinInviteButtons', [skip, send])])
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('aria-modal', 'true')
    dialog.setAttribute('aria-labelledby', 'polyfinInviteTitle')
    dialog.setAttribute('aria-describedby', 'polyfinInviteText')
    var backdrop = element('div', 'polyfinInviteBackdrop', [dialog])
    var shown = (picker = { node: backdrop, restore: document.activeElement, timer: null })
    function leave() {
      if (picker !== shown) return
      picker = null
      closeInvite(shown)
      if (!again) toast(say('togetherStarted'), 10000)
    }
    skip.addEventListener('click', quietly(leave))
    backdrop.addEventListener(
      'click',
      quietly(function (event) {
        if (event.target === backdrop) leave()
      })
    )
    send.addEventListener(
      'click',
      quietly(function () {
        var chosen = people.filter(function (person, i) {
          return boxes[i].checked
        })
        if (picker !== shown || !chosen.length) return
        picker = null
        closeInvite(shown)
        sendInvites(group, chosen, again)
      })
    )
    inviteKeys(dialog, leave)
    document.body.appendChild(backdrop)
    boxes[0].focus()
  }
  // sendInvites invites people to the group, one by one, and says who was
  // invited, who was invited less than a minute ago, and whether any
  // invitation failed.
  function sendInvites(group, people, again) {
    var api = window.ApiClient
    var outcomes = []
    Promise.all(
      people.map(function (person, i) {
        var request = {
          type: 'POST',
          url: api.getUrl('Polyfin/SyncPlay/Invitations'),
          data: JSON.stringify({ GroupId: group, UserId: person.Id }),
          contentType: 'application/json',
        }
        return api.ajax(request).then(
          function () {
            outcomes[i] = 'sent'
          },
          function (error) {
            outcomes[i] = error && error.status === 429 ? 'soon' : 'failed'
          }
        )
      })
    ).then(
      quietly(function () {
        function names(outcome) {
          return people
            .filter(function (person, i) {
              return outcomes[i] === outcome
            })
            .map(function (person) {
              return person.Name
            })
            .join(', ')
        }
        var sent = names('sent')
        var soon = names('soon')
        if (sent)
          toast(
            say(again ? 'invitedMore' : 'invitedSent').replace('{names}', function () {
              return sent
            }),
            again ? 0 : 10000
          )
        if (soon)
          toast(
            say('invitedSoon').replace('{names}', function () {
              return soon
            })
          )
        if (outcomes.indexOf('failed') >= 0) toast(say('inviteFailed'))
      })
    )
  }
  // invited hears a PolyfinSyncPlayInvite: an invitation shows a prompt in
  // place of any other; its end, joined, declined, expired or its group
  // ended, closes its prompt.
  function invited(message) {
    var invite = message && message.Data
    if (!invite || !invite.Id) return
    if (invite.State === 'Invited') {
      showPrompt(invite)
    } else if (prompt && prompt.id === String(invite.Id)) {
      closeInvite(prompt)
      prompt = null
    }
  }
  // showPrompt shows an invitation: who sent it, the title, Join, which
  // joins its group as jellyfin-web's SyncPlay menu does, and Not now. It
  // goes once the invitation lasted, should no end be heard.
  function showPrompt(invite) {
    closeInvite(prompt)
    prompt = null
    inviteStyle()
    var id = String(invite.Id)
    var join = inviteButton('button-submit polyfinInviteJoin', say('inviteJoin'))
    var later = inviteButton('button-cancel polyfinInviteLater', say('inviteLater'))
    var heading = element('h2', 'polyfinInviteTitle', say('watchTogether'))
    heading.id = 'polyfinPromptTitle'
    var host = say('invitePrompt').replace('{host}', function () {
      return String(invite.HostName || '')
    })
    var text = element('p', 'polyfinInviteText', host)
    text.id = 'polyfinPromptText'
    var parts = [heading, text]
    if (invite.ItemName) {
      var name = String(invite.ItemName)
      parts.push(
        element(
          'p',
          'polyfinInviteItem',
          invite.SeriesName ? [element('span', 'polyfinInviteSeries', String(invite.SeriesName)), element('span', '', name)] : name
        )
      )
    }
    parts.push(element('div', 'polyfinInviteButtons', [later, join]))
    var card = element('div', 'polyfinInvite polyfinInvitePrompt', parts)
    card.setAttribute('role', 'alertdialog')
    card.setAttribute('aria-labelledby', 'polyfinPromptTitle')
    card.setAttribute('aria-describedby', 'polyfinPromptText')
    var shown = (prompt = { id: id, node: card, restore: document.activeElement, timer: null })
    function done() {
      if (prompt !== shown) return false
      prompt = null
      closeInvite(shown)
      return true
    }
    shown.timer = setTimeout(quietly(done), inviteLasts)
    function answer(path, body, failed) {
      var api = window.ApiClient
      var request = { type: 'POST', url: api.getUrl(path) }
      if (body) {
        request.data = JSON.stringify(body)
        request.contentType = 'application/json'
      }
      api.ajax(request).then(null, quietly(failed))
    }
    join.addEventListener(
      'click',
      quietly(function () {
        if (!done()) return
        answer('SyncPlay/Join', { GroupId: invite.GroupId }, function () {
          toast(say('joinFailed'))
        })
      })
    )
    function decline() {
      if (done()) answer('Polyfin/SyncPlay/Invitations/' + id + '/Decline', null, function () {})
    }
    later.addEventListener('click', quietly(decline))
    inviteKeys(card, decline)
    document.body.appendChild(card)
    // The card takes the focus, not Join: a key meant for the page then
    // answers nothing until the user moves to a button. It leaves the
    // focus alone in a field being typed in, or while a video plays.
    card.tabIndex = -1
    if (!busy()) card.focus()
  }
  // busy tells whether the user is typing in a field or watching a video,
  // when a prompt nobody asked for leaves the focus where it is.
  function busy() {
    var focused = document.activeElement
    if (focused && (focused.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(focused.tagName || ''))) return true
    var videos = document.querySelectorAll('video')
    for (var i = 0; i < videos.length; i++) {
      if (!videos[i].paused && !videos[i].ended) return true
    }
    return false
  }
  // inviteStyle adds the look of the picker and the prompt, once: a panel
  // in the middle of a dimmed page for the picker; at the top right for
  // the prompt, which leaves the page usable, across the bottom on a
  // narrow screen. Their buttons are jellyfin-web's own.
  function inviteStyle() {
    if (document.getElementById('polyfinInviteStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinInviteStyle'
    sheet.textContent =
      '.polyfinInviteBackdrop{position:fixed;top:0;right:0;bottom:0;left:0;z-index:1100;display:flex;align-items:center;' +
      'justify-content:center;padding:1em;box-sizing:border-box;background:rgba(0,0,0,.6)}' +
      '.polyfinInvite{box-sizing:border-box;width:26em;max-width:100%;max-height:100%;overflow:auto;padding:1.5em;' +
      'border-radius:.3em;background:#202020;color:#fff;box-shadow:0 .2em .8em rgba(0,0,0,.6);text-align:left}' +
      '.polyfinInviteTitle{margin:0 0 .6em;font-size:1.3em;font-weight:500}' +
      '.polyfinInviteText{margin:0 0 1em;line-height:1.4}' +
      '.polyfinInviteList{margin:0 0 1.2em;max-height:40vh;overflow:auto}' +
      '.polyfinInvitee{display:flex;align-items:center;padding:.5em .2em;cursor:pointer}' +
      '.polyfinInvitee input{flex:none;width:1.3em;height:1.3em;margin:0 .8em 0 0;accent-color:#00a4dc;cursor:pointer}' +
      '.polyfinInviteName{overflow-wrap:anywhere}' +
      '.polyfinInviteItem{margin:0 0 1.2em;font-weight:600;overflow-wrap:anywhere}' +
      '.polyfinInviteSeries{display:block;font-weight:400;opacity:.8}' +
      '.polyfinInviteButtons{display:flex;flex-wrap:wrap;justify-content:flex-end;align-items:center;margin:-.25em}' +
      '.polyfinInviteButtons .emby-button{flex:0 1 auto;width:auto;min-width:6em;margin:.25em!important}' +
      '.polyfinInviteButtons .emby-button[disabled]{opacity:.45;cursor:default}' +
      '.polyfinInvitePrompt{position:fixed;top:4.5em;right:1em;z-index:1100;width:24em;max-width:calc(100% - 2em)}' +
      '@media (max-width:40em){.polyfinInvitePrompt{top:auto;right:.5em;bottom:.5em;left:.5em;width:auto;max-width:none}}'
    document.head.appendChild(sheet)
  }
  // What the entries took off (see the header), kept in the browser's
  // session storage under dropsKey for dropsKept: jellyfin-web keeps its
  // answers for a minute. Each names the server and user it is for, and
  // the item: taken off Continue Watching (resume), with its last play
  // then, hidden (hidden), a series standing for what is in it, a series
  // taken off Next Up (nextup), with when, or a title taken off the
  // watchlist (watchlist), with the Id of the watchlist playlist: a movie
  // or an episode, with its series and season, or a season or a series
  // with every entry of it (whole: season or series). A title put on the
  // watchlist (watchlisted), with that Id too, takes nothing off: it has
  // the watchlist's rows ask Polyfin.
  var dropsKey = 'polyfinDrops'
  var dropsKept = 120000
  var drops = []
  try {
    var savedDrops = JSON.parse(window.sessionStorage.getItem(dropsKey))
    if (Array.isArray(savedDrops)) drops = savedDrops
  } catch (error) {}
  // Those kept too long are forgotten as soon as the page loads.
  if (drops.length) saveDrops()
  function viewerKey() {
    var api = window.ApiClient
    if (!api) return ''
    return (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + (api.getCurrentUserId() || '')
  }
  // liveDrops forgets the drops kept for longer than dropsKept.
  function liveDrops() {
    var now = Date.now()
    drops = drops.filter(function (drop) {
      return !!drop && drop.until > now
    })
    return drops
  }
  function saveDrops() {
    try {
      if (liveDrops().length) window.sessionStorage.setItem(dropsKey, JSON.stringify(drops))
      else window.sessionStorage.removeItem(dropsKey)
    } catch (error) {}
  }
  function keepDrop(drop) {
    drop.viewer = viewerKey()
    drop.until = Date.now() + dropsKept
    drops.push(drop)
    saveDrops()
  }
  // forgetDrops forgets the drops of a kind for the items ids.
  function forgetDrops(kind, ids) {
    drops = drops.filter(function (drop) {
      return !(drop && drop.kind === kind && ids.indexOf(drop.id) >= 0)
    })
    saveDrops()
  }
  // forgetUnlisted forgets the drops of the watchlist that a title put
  // back on it undoes: the title's own, those of the season or series it
  // is in, and for a season or a series those of what is in it.
  function forgetUnlisted(listing) {
    drops = drops.filter(function (drop) {
      if (!drop || drop.kind !== 'watchlist') return true
      if (drop.id === listing.title) return false
      if (drop.whole && (drop.id === listing.series || drop.id === listing.season)) return false
      return !(listing.whole && (drop.series === listing.title || drop.season === listing.title))
    })
    saveDrops()
  }
  // unlists tells whether a drop of the watchlist takes an item off: the
  // item itself, or what is in a season or a series taken off whole.
  function unlists(drop, item) {
    if (plainId(item.Id) === drop.id) return true
    var whole = drop.whole === 'series' ? item.SeriesId : drop.whole === 'season' ? item.SeasonId : ''
    return !!whole && plainId(whole) === drop.id
  }
  // watchlistChanged tells whether the user signed in put a title on the
  // watchlist, or took one off, within dropsKept.
  function watchlistChanged() {
    var viewer = viewerKey()
    return liveDrops().some(function (drop) {
      return drop.viewer === viewer && (drop.kind === 'watchlist' || drop.kind === 'watchlisted')
    })
  }
  // screened is a row's answer without what the drops of the user signed
  // in take off; the answer itself is left as it is. row is the element
  // that asked, which tells a Next Up list and a row of the watchlist.
  function screened(answer, row) {
    var viewer = viewerKey()
    var mine = liveDrops().filter(function (drop) {
      return drop.viewer === viewer
    })
    var items = Array.isArray(answer) ? answer : answer && Array.isArray(answer.Items) ? answer.Items : null
    if (!mine.length || !items) return answer
    var nextUp =
      mine.some(function (drop) {
        return drop.kind === 'nextup'
      }) && nextUpRow(row) === 'list'
    var watchlist =
      mine.some(function (drop) {
        return drop.kind === 'watchlist'
      }) && watchlistRow(row)
    var resumes =
      items.length > 0 &&
      items.every(function (item) {
        return !!item && !!item.UserData && item.UserData.PlaybackPositionTicks > 0
      })
    var kept = items.filter(function (item) {
      return (
        !item ||
        !mine.some(function (drop) {
          if (drop.kind === 'hidden') {
            return plainId(item.Id) === drop.id || (!!item.SeriesId && plainId(item.SeriesId) === drop.id)
          }
          if (drop.kind === 'nextup') {
            // Kept when one of its episodes was played since.
            var since = Date.parse((item.UserData && item.UserData.LastPlayedDate) || '') || 0
            return nextUp && !!item.SeriesId && plainId(item.SeriesId) === drop.id && since <= drop.at
          }
          if (drop.kind === 'watchlist') return watchlist && unlists(drop, item)
          if (drop.kind !== 'resume') return false
          var played = Date.parse((item.UserData && item.UserData.LastPlayedDate) || '') || 0
          return resumes && plainId(item.Id) === drop.id && played <= drop.last
        })
      )
    })
    if (kept.length === items.length) return answer
    return Array.isArray(answer) ? kept : Object.assign({}, answer, { Items: kept })
  }
  // nextUpRow tells where an element stands: in a list of Next Up (list),
  // the home's section whose title links to the Next Up page, that page,
  // or the Next Up of a TV library's tab; in the Next Up of a series' page
  // (series); or elsewhere (empty).
  function nextUpRow(element) {
    if (!element || !element.closest) return ''
    if (element.closest('.nextUpSection')) return 'series'
    if (element.closest('#nextUpItems')) return 'list'
    var section = element.closest('.verticalSection')
    if (section && section.querySelector && section.querySelector('.sectionTitleContainer a[href*="type=nextup"]')) {
      return 'list'
    }
    if (/[?&]type=nextup(?:&|$)/i.test(location.hash) && element.closest('.itemsContainer')) return 'list'
    return ''
  }
  // The Id of the watchlist playlist of the user signed in, as
  // /Polyfin/UserItems/Watchlist last gave it, empty while they have none;
  // until it is asked, as the drops of the watchlist kept it, for a page
  // loaded again.
  var watchlistPlaylist = null
  function learnWatchlist(answer) {
    watchlistPlaylist = { viewer: viewerKey(), id: answer && answer.PlaylistId ? plainId(answer.PlaylistId) : '' }
  }
  function knownWatchlist() {
    var viewer = viewerKey()
    if (watchlistPlaylist && watchlistPlaylist.viewer === viewer) return watchlistPlaylist.id
    var kept = liveDrops().filter(function (drop) {
      return drop.viewer === viewer && drop.playlist
    })
    return kept.length > 0 ? kept[kept.length - 1].playlist : ''
  }
  // watchlistRow tells whether an element stands in a row of the user's
  // watchlist: Polyfin's Now available, Still waiting and Watchlist home
  // rows, or the list of the watchlist playlist's page shown.
  function watchlistRow(element) {
    if (!element || !element.closest) return false
    var custom = element.closest('.polyfinHomeRow')
    if (custom) return /^(NowAvailable|StillWaiting|Watchlist)$/.test(custom.getAttribute('data-polyfin-kind') || '')
    return opensWatchlist(location.hash) && !!element.closest('.itemsContainer') && !element.closest('.page.hide')
  }
  // opensWatchlist tells whether an address opens the page of the user's
  // watchlist playlist: #/details?id= it.
  function opensWatchlist(address) {
    var match = /^#!?\/details\?(?:[^#]*&)?id=([^&#]*)/i.exec(address || '')
    var playlist = knownWatchlist()
    return !!match && !!playlist && plainId(match[1]) === playlist
  }
  // Every row's fetchData goes through screened, as jellyfin-web sets it on
  // the row: rows are divs, and the script runs before jellyfin-web.
  if (typeof HTMLDivElement === 'function') {
    var fetchers = marks('map', 'Fetcher')
    Object.defineProperty(HTMLDivElement.prototype, 'fetchData', {
      configurable: true,
      get: function () {
        return fetchers.get(this)
      },
      set: function (fetch) {
        if (typeof fetch !== 'function') return fetchers.set(this, fetch)
        fetchers.set(this, function () {
          var row = this
          listen()
          var refreshed =
            row && ((playedRow(row) && Date.now() < freshUntil) || (watchlistRow(row) && watchlistChanged()))
          var answer = refreshed ? unstale(fetch, row, arguments) : fetch.apply(row, arguments)
          if (!answer || typeof answer.then !== 'function') return answer
          return answer.then(function (got) {
            try {
              if (row && playedRow(row)) seen.set(row, playState(got))
              return screened(got, row)
            } catch (error) {
              return got
            }
          })
        })
      },
    })
  }
  // The home rows jellyfin-web refreshes on a played mark ask Polyfin until
  // freshUntil (see the header). seen keeps, for each, what the user did
  // with the items of its last answer: Played and the resume point.
  var freshFor = 60000
  var freshUntil = 0
  var seen = marks('map', 'Seen')
  // playedRow tells a home row that jellyfin-web refreshes on a played
  // mark: Next Up's and Continue Watching's.
  function playedRow(row) {
    if (!row.closest || !row.closest('.homeSectionsContainer')) return false
    return (row.getAttribute('data-monitor') || '').split(',').indexOf('markplayed') >= 0
  }
  function playState(answer) {
    var items = Array.isArray(answer) ? answer : answer && Array.isArray(answer.Items) ? answer.Items : []
    var state = {}
    items.forEach(function (item) {
      if (!item || !item.Id) return
      var data = item.UserData || {}
      state[plainId(item.Id)] = { played: !!data.Played, position: data.PlaybackPositionTicks || 0 }
    })
    return state
  }
  // playedChanged refreshes the home rows at once, and has them ask
  // Polyfin for a minute, when a UserDataChanged message, or the answer of
  // a played mark the page sent, changes Played or the resume point of one
  // of their items. What it was told is kept as the item's state, so that
  // the same change heard both ways, the answer then the message, or the
  // other way round, refreshes the rows once.
  function playedChanged(data) {
    var changes = (data && data.UserDataList) || []
    var rows = Array.prototype.filter.call(document.querySelectorAll('.homeSectionsContainer .itemsContainer'), playedRow)
    var changed = false
    rows.forEach(function (row) {
      var state = seen.get(row)
      if (!state) return
      changes.forEach(function (change) {
        var was = change && change.ItemId && state[plainId(change.ItemId)]
        if (!was) return
        var played = !!change.Played
        var position = change.PlaybackPositionTicks || 0
        if (was.played === played && was.position === position) return
        was.played = played
        was.position = position
        changed = true
      })
    })
    if (!changed) return
    freshUntil = Date.now() + freshFor
    rows.forEach(function (row) {
      if (typeof row.notifyRefreshNeeded === 'function') row.notifyRefreshNeeded(true)
    })
  }
  // unstale calls a row's fetchData as if its answers were older than a
  // minute, jellyfin-web's query client then asking Polyfin (see the
  // header).
  function unstale(fetch, row, args) {
    var now = Date.now
    Date.now = function () {
      return now() + freshFor + 1000
    }
    try {
      return fetch.apply(row, args)
    } finally {
      Date.now = now
    }
  }
  // dropCards takes cards off the page, and hides a home row left empty.
  function dropCards(cards) {
    cards.forEach(function (card) {
      var row = card.closest('.itemsContainer')
      card.remove()
      var section = row && row.closest('.verticalSection')
      if (section && !row.querySelector('.card, .listItem')) section.classList.add('hide')
    })
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds, or for
  // lasts milliseconds when given, for a longer text. Clicks pass through
  // it: a long one lies over the video's Play button, which the group's
  // host is told to press.
  function toast(text, lasts) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = element('div', 'toastContainer', '')
      document.body.appendChild(holder)
    }
    var note = element('div', 'toast', text)
    if (note.style) note.style.pointerEvents = 'none'
    holder.appendChild(note)
    setTimeout(function () {
      note.classList.add('toastVisible')
    }, 50)
    setTimeout(function () {
      note.classList.add('toastHide')
      setTimeout(function () {
        note.remove()
      }, 300)
    }, lasts || 3300)
  }
  // Both run before jellyfin-web opens its item menu.
  addEventListener('click', quietly(aim), true)
  addEventListener('contextmenu', quietly(aim), true)
