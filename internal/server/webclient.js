// Polyfin (MIT License). Served at /web/polyfin.js and loaded by the
// index.html of jellyfin-web, which Polyfin ships unmodified. It follows the
// client's routes for three things, adds a style for a fourth, mends a link
// for a fifth, closes a duplicate error for a sixth, leaves libraries out of
// the menus for a seventh, gives members a link to the admin app for an
// eighth, shows a title's three scores for a ninth, and adds entries to the
// item menus for a tenth.
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
//
// Eighth, it gives a user signed in who is not an administrator, a member,
// a "Polyfin" entry where jellyfin-web 12.2 shows administrators Dashboard:
// it opens the admin app at /admin/ in the same tab, where the admin app
// signs the member in with the web client's token, as it does an
// administrator. It is named "Polyfin" in every language and drawn as
// Dashboard is, without the Administration heading over it:
//   the default layout's user menu (#app-user-menu), an MUI Menu, as a copy
//     of its Settings item after a divider of its own, before the divider
//     over Quick Connect, Select server and Sign out;
//   the legacy layouts' side menu (.mainDrawer), first in its user section
//     (.userMenuOptions): after the libraries without a heading, where
//     Dashboard is, it would read as one of them;
//   the settings page (#/mypreferencesmenu), which the user menu's Settings
//     and the legacy layouts' user button open, in a section of its own
//     before the user section (.userSection).
// The legacy layouts are Desktop (Legacy), Mobile (Legacy) and TV. The
// default layout's side menu has no Dashboard. It is a plain link:
// jellyfin-web's emby-linkbutton would route it within jellyfin-web. The
// script asks jellyfin-web's client (ApiClient.getCurrentUser) whether the
// user is an administrator whenever the user signed in changes, as for the
// seventh, and keeps the last member in the browser's storage, so that the
// entry shows as soon as jellyfin-web draws a menu. A MutationObserver on
// the page puts it back as jellyfin-web draws its menus anew. It never shows
// where Dashboard shows, nor before sign-in, and goes as soon as the user
// signed in is another, signing out included. Any error just stops it.
//
// Ninth, on a title's page, it shows the title's three scores in the first
// line of its details (.itemMiscInfo-primary): its IMDb rating, its
// critics' score and its audience score, labelled, as
// /Polyfin/Items/{id}/Ratings answers them while the server has an MDBList
// key for ratings (Settings › Content). jellyfin-web shows only
// CommunityRating, with a star, and CriticRating, with its own icon: the
// script hides both in that line while its scores show, and gives them back
// as the route leaves the title. The answer may come while MDBList is
// asked about the title (Pending): it asks again 3 seconds later, 10 times
// at most. A MutationObserver on the page puts the scores back as
// jellyfin-web renders the line anew. An answer without a score, or no
// answer, changes nothing; nor does anything during a video. Its words
// follow jellyfin-web's language, as for the third. Any error just stops
// it.
//
// Tenth, it adds Polyfin's own entries to jellyfin-web's item menu, the
// action sheet a card's ⋮ button or a right click on a card opens, and the
// ⋮ button of a title's page (.btnMoreCommands):
//   "Remove from Continue Watching", for a card of a Continue Watching row:
//     a row whose cards all have a resume point, as /UserItems/Resume
//     lists them, the libraries' own rows included. It deletes
//     /Polyfin/UserItems/{id}/Resume and takes the card off the row;
//   "Hide this title", for a movie, a home video or a series, or a season
//     or an episode of one, that the user has not hidden, as
//     /Polyfin/UserItems/Hidden lists them. It posts to
//     /Polyfin/UserItems/{id}/Hidden, takes the cards of the title off the
//     page and says so in a toast, as jellyfin-web's own;
//   "Show this title again", in its place for a title the user hid, its
//     series hidden counting for a season or an episode, which deletes
//     /Polyfin/UserItems/{id}/Hidden.
// As the button is pressed or the card right-clicked, before jellyfin-web
// opens its menu, the script asks for the item (ApiClient.getItem), the
// hidden titles and, for a card with a resume point, Continue Watching;
// the first action sheet that opens within 10 seconds, no other click
// coming between, gets the entries once the answers come. An entry closes
// the menu as Back does, then acts. jellyfin-web 12.2 keeps the answers of
// its rows for a minute, and a row the change's UserDataChanged message
// refreshes within it draws that answer again, asking nothing: the
// script has the rows' refreshes (their fetchData) leave out the item
// taken off Continue Watching, unless played since its last play then,
// and the titles hidden, until shown again. Its words follow
// jellyfin-web's language, as the third's. Any error just stops it.
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
  // The options this script added to the version menu, and the size and
  // the ETag of the version each option stood for when the script last
  // listed it. Polyfin gives each source the identifier of its version as
  // its ETag, the first source's included, which carries the title's.
  // jellyfin-web fills the menu with new options, which forgets them all:
  // given, the sources the script was last given by identifier, tells them
  // again for an option jellyfin-web listed under a source's identifier and
  // name.
  var added = new WeakSet()
  var sizes = new WeakMap()
  var tags = new WeakMap()
  var given = {}
  // The version picked among those added, until jellyfin-web lists it: the
  // observer that waits for jellyfin-web's options, and the identifier of
  // the version picked.
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
    if (!id || !page || asking || progress.Count < 1 || progress.Count === updatedFor) return
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
    en: {
      searching: 'Looking for sources…',
      none: 'No source is available for this title.',
      again: 'Try again',
      removeResume: 'Remove from Continue Watching',
      hide: 'Hide this title',
      show: 'Show this title again',
      hidden: 'This title is hidden from your lists.',
      shown: 'This title is back in your lists.',
    },
    fr: {
      searching: 'Recherche des sources…',
      none: 'Aucune source disponible pour ce titre.',
      again: 'Réessayer',
      removeResume: 'Retirer de « Continuer de regarder »',
      hide: 'Masquer ce titre',
      show: 'Afficher de nouveau ce titre',
      hidden: 'Ce titre est masqué de vos listes.',
      shown: 'Ce titre est de retour dans vos listes.',
    },
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
    given = {}
    pushed = false
    if (picking) picking.observer.disconnect()
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
    try {
      followScores()
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
      placeEntries()
      if (user) {
        askViews(api, user)
        askRole(api, user)
      }
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
  // The member the "Polyfin" entry is for (see the header), as the last
  // answer kept says, and where that answer is kept, as {user}.
  var savedMemberKey = 'polyfinMember'
  var memberFor = ''
  try {
    var savedMember = JSON.parse(window.localStorage.getItem(savedMemberKey))
    if (savedMember && typeof savedMember.user === 'string') memberFor = savedMember.user
  } catch (error) {}
  // askRole asks jellyfin-web's client whether user is an administrator,
  // keeps the answer, the member or none, unless another user signed in
  // meanwhile, and shows or takes away the entry.
  function askRole(api, user) {
    function plain(id) {
      return String(id).replace(/-/g, '').toLowerCase()
    }
    api
      .getCurrentUser()
      .then(function (current) {
        if (viewer !== user || !current || !current.Policy || plain(current.Id) !== plain(user)) return
        memberFor = current.Policy.IsAdministrator === true ? '' : user
        try {
          if (memberFor) window.localStorage.setItem(savedMemberKey, JSON.stringify({ user: memberFor }))
          else window.localStorage.removeItem(savedMemberKey)
        } catch (error) {}
        placeEntries()
      })
      .catch(function () {})
  }
  // What marks the script's entries, each also with the class of its kind;
  // where jellyfin-web's Dashboard links; and Dashboard's icon in the user
  // menu, MUI's.
  var entryClass = 'polyfinEntry'
  var dashboardLink = 'a[href="#/dashboard"]'
  var dashboardIcon = 'M3 13h8V3H3zm0 8h8v-6H3zm10 0h8V11h-8zm0-18v6h8V3z'
  // placeEntries shows the entry in each menu drawn where jellyfin-web shows
  // administrators Dashboard, while the user signed in is the member, and
  // takes away every other. It changes only what differs: its own changes
  // call it again. While no entry is shown and the user is no member, it
  // reads nothing of the page, which changes often during a video.
  var shown = false
  function placeEntries() {
    try {
      var api = window.ApiClient
      var user = (api && api.getCurrentUserId()) || ''
      var member = user !== '' && user === memberFor
      if (!member && !shown) return
      var kept = []
      if (member) {
        // The legacy layout's side menu, first in its user section: right
        // after the libraries, without a heading, it would read as one.
        each('.mainDrawer .userMenuOptions', function (section) {
          var heading = section.querySelector('.sidebarHeader')
          if (!heading || section.parentNode.querySelector(dashboardLink)) return
          var entry = heading.nextElementSibling
          if (!entry || !entry.classList.contains('polyfinDrawerEntry')) {
            entry = drawerEntry()
            section.insertBefore(entry, heading.nextSibling)
          }
          kept.push(entry)
        })
        // The settings page, before its user section.
        each('.userSection', function (section) {
          if (!section.parentNode.querySelector(dashboardLink)) kept.push(before(section, 'polyfinSettingsEntry', settingsEntry))
        })
        // The default layout's user menu, before the divider over
        // Quick Connect, Select server and Sign out, after one of its own.
        each('#app-user-menu', function (menu) {
          var settings = menu.querySelector('a[href="#/mypreferencesmenu"]')
          var icon = menu.querySelector('svg[data-testid="LogoutIcon"]')
          var divider = icon && icon.closest('.MuiMenuItem-root')
          while (divider && !(divider.classList.contains('MuiDivider-root') && !divider.classList.contains(entryClass))) {
            divider = divider.previousElementSibling
          }
          if (!settings || !divider || menu.querySelector(dashboardLink)) return
          var item = before(divider, 'polyfinMenuEntry', function () {
            return menuEntry(settings)
          })
          if (!item) return
          kept.push(
            item,
            before(item, 'polyfinMenuDivider', function () {
              var line = divider.cloneNode(true)
              line.classList.add(entryClass, 'polyfinMenuDivider')
              return line
            }),
          )
        })
      }
      each('.' + entryClass, function (entry) {
        if (kept.indexOf(entry) < 0) entry.remove()
      })
      shown = kept.length > 0
    } catch (error) {}
  }
  function each(selector, run) {
    Array.prototype.forEach.call(document.querySelectorAll(selector), run)
  }
  // before returns the entry of a kind right before node, made by make
  // unless it is there; null when make makes none.
  function before(node, kind, make) {
    var previous = node.previousElementSibling
    if (previous && previous.classList.contains(kind)) return previous
    var entry = make()
    if (entry) node.parentNode.insertBefore(entry, node)
    return entry
  }
  // element makes an element with classes and a text or children.
  function element(tag, className, content) {
    var made = document.createElement(tag)
    made.className = className
    if (typeof content === 'string') made.textContent = content
    else content.forEach(made.appendChild, made)
    return made
  }
  // A link to the admin app, as its dashboard routes open it. A plain link:
  // jellyfin-web's emby-linkbutton would open it within jellyfin-web.
  function adminLink(className, content) {
    var link = element('a', className, content)
    link.setAttribute('href', new URL('../admin/', location.href).href)
    return link
  }
  function materialIcon(className) {
    var icon = element('span', 'material-icons ' + className + ' dashboard', '')
    icon.setAttribute('aria-hidden', 'true')
    return icon
  }
  // The entry as the legacy side menu draws Dashboard.
  function drawerEntry() {
    return adminLink(entryClass + ' polyfinDrawerEntry navMenuOption lnkMediaFolder emby-button', [
      materialIcon('navMenuOptionIcon'),
      element('span', 'navMenuOptionText', 'Polyfin'),
    ])
  }
  // The entry as the settings page draws Dashboard, in a section of its own
  // without the Administration heading.
  function settingsEntry() {
    var body = element('div', 'listItemBody', [element('div', 'listItemBodyText', 'Polyfin')])
    var link = adminLink('emby-button listItem-border', [
      element('div', 'listItem', [materialIcon('listItemIcon listItemIcon-transparent'), body]),
    ])
    link.setAttribute('style', 'display:block;margin:0;padding:0')
    var section = 'verticalSection verticalSection-extrabottompadding'
    return element('div', entryClass + ' polyfinSettingsEntry ' + section, [link])
  }
  // The entry as the user menu draws Dashboard: a copy of its Settings
  // item, MUI's own, with Dashboard's icon.
  function menuEntry(settings) {
    var item = settings.cloneNode(true)
    var text = item.querySelector('.MuiListItemText-primary')
    var path = item.querySelector('svg path')
    if (!text || !path) return null
    item.classList.add(entryClass, 'polyfinMenuEntry')
    item.classList.remove('Mui-focusVisible')
    item.setAttribute('href', new URL('../admin/', location.href).href)
    text.textContent = 'Polyfin'
    path.setAttribute('d', dashboardIcon)
    path.parentNode.removeAttribute('data-testid')
    var ripple = item.querySelector('.MuiTouchRipple-root')
    if (ripple) ripple.textContent = ''
    return item
  }
  // jellyfin-web draws its menus anew, which the script, loaded in the
  // head, follows once the body exists.
  function watchMenus() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(placeEntries).observe(document.body, { childList: true, subtree: true })
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
  // Alerts and item menus are dialog containers jellyfin-web adds to the
  // body, which the script, loaded in the head, observes once it exists.
  function watchDialogs() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(
      quietly(function (records) {
        for (var i = 0; i < records.length; i++) {
          var added = records[i].addedNodes || []
          for (var j = 0; j < added.length; j++) {
            if (added[j].classList && added[j].classList.contains('dialogContainer')) {
              // Each on its own: one failing leaves the other to run.
              quietly(closeDuplicate)(added[j])
              quietly(extendMenu)(added[j])
            }
          }
        }
      }),
    ).observe(document.body, { childList: true })
  }
  // The title whose scores are asked for, its last answer holding one,
  // the next time they are asked, and the page watched for jellyfin-web's
  // renders of its first line of details.
  var rated = ''
  var scores = null
  var scoresTimer = null
  var scoresWatcher = null
  var scoresWatched = null
  var scoreWords = {
    en: { imdb: 'IMDb rating', critics: 'Critics', criticsTitle: 'Critics’ score', audience: 'Audience', audienceTitle: 'Audience score', percent: '%' },
    fr: { imdb: 'Note IMDb', critics: 'Critiques', criticsTitle: 'Score des critiques', audience: 'Public', audienceTitle: 'Score du public', percent: '\u00a0%' },
  }
  function sayScore(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (scoreWords[language] || scoreWords.en)[key]
  }
  // followScores asks for the scores of the title the route opens, and
  // gives back the page of the title it leaves.
  function followScores() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === rated) return
    rated = id
    scores = null
    clearTimeout(scoresTimer)
    scoresTimer = null
    if (scoresWatcher) scoresWatcher.disconnect()
    scoresWatcher = null
    scoresWatched = null
    unrate()
    if (id) askScores(id, 0)
  }
  // askScores asks for the title's scores: again 3 seconds later while
  // MDBList is asked about it (Pending), 10 times at most, and every second
  // until jellyfin-web made its client.
  function askScores(id, asked) {
    scoresTimer = null
    if (rated !== id || asked >= 10) return
    var again = function (delay) {
      scoresTimer = setTimeout(function () {
        try {
          askScores(id, asked + 1)
        } catch (error) {}
      }, delay)
    }
    var api = window.ApiClient
    if (!api) return again(1000)
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Ratings'))
      .then(function (answer) {
        if (rated !== id || !answer) return
        if (answer.CommunityRating != null || answer.CriticsScore != null || answer.AudienceScore != null) {
          scores = answer
          showScores()
        }
        if (answer.Pending) again(3000)
      })
      .catch(function () {})
  }
  // unrate gives jellyfin-web's own ratings back, wherever the script hid
  // them.
  function unrate() {
    var lines = document.querySelectorAll('.polyfinRated')
    for (var i = 0; i < lines.length; i++) {
      lines[i].classList.remove('polyfinRated')
      var added = lines[i].querySelector('.polyfinScores')
      if (added) added.parentNode.removeChild(added)
    }
  }
  // showScores puts the title's scores in the first line of details of the
  // title page shown, in place of jellyfin-web's own, and watches the page
  // so that its renders find them again, or the document until the page
  // shows. It never changes the page during a video.
  function showScores() {
    if (!scores) return
    var page = shownPage()
    var target = page || document.body
    if (target && scoresWatched !== target) {
      if (scoresWatcher) scoresWatcher.disconnect()
      scoresWatched = target
      scoresWatcher = new MutationObserver(function () {
        try {
          showScores()
        } catch (error) {}
      })
      scoresWatcher.observe(target, { childList: true, subtree: true })
    }
    if (!page) return
    var line = page.querySelector('.itemMiscInfo-primary')
    if (!line || line.querySelector('.polyfinScores')) return
    scoresStyle()
    var entries = []
    if (scores.CommunityRating != null) entries.push(['IMDb', Number(scores.CommunityRating).toFixed(1), sayScore('imdb')])
    if (scores.CriticsScore != null) {
      entries.push([sayScore('critics'), Math.round(scores.CriticsScore) + sayScore('percent'), sayScore('criticsTitle')])
    }
    if (scores.AudienceScore != null) {
      entries.push([sayScore('audience'), Math.round(scores.AudienceScore) + sayScore('percent'), sayScore('audienceTitle')])
    }
    var group = document.createElement('div')
    group.className = 'mediaInfoItem polyfinScores'
    entries.forEach(function (entry) {
      var score = document.createElement('span')
      score.className = 'polyfinScore'
      score.title = entry[2]
      var label = document.createElement('span')
      label.className = 'polyfinScoreLabel'
      label.textContent = entry[0]
      score.appendChild(label)
      score.appendChild(document.createTextNode(entry[1]))
      group.appendChild(score)
    })
    var own = line.querySelector('.starRatingContainer') || line.querySelector('.mediaInfoCriticRating')
    line.insertBefore(group, own)
    if (!line.classList.contains('polyfinRated')) line.classList.add('polyfinRated')
  }
  // scoresStyle adds the look of the scores, which hides jellyfin-web's own
  // in the same line, once.
  function scoresStyle() {
    if (document.getElementById('polyfinScoresStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinScoresStyle'
    sheet.textContent =
      '.polyfinRated .starRatingContainer,.polyfinRated .mediaInfoCriticRating{display:none!important}' +
      '.polyfinScores{display:inline-flex;flex-wrap:wrap;gap:.3em 1em}' +
      '.polyfinScoreLabel{font-weight:600;opacity:.75;margin-right:.35em}'
    document.head.appendChild(sheet)
  }
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
  // type and series, whether the user hid it or its series, and whether
  // its card stands in a Continue Watching row.
  function askFacts(api, id, card) {
    var user = api.getCurrentUserId()
    var row = card && card.getAttribute('data-positionticks') ? card.closest('.itemsContainer') : null
    var resume = row
      ? api.getJSON(api.getUrl('UserItems/Resume', { userId: user, Limit: 100, MediaTypes: 'Video', EnableImages: false }))
      : null
    return Promise.all([api.getItem(user, id), api.getJSON(api.getUrl('Polyfin/UserItems/Hidden')), resume]).then(
      function (answers) {
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
        var cards = row ? row.querySelectorAll('.card[data-id]') : []
        return {
          series: series,
          hideable: /^(Movie|Video|Series)$/.test(item.Type) || (/^(Season|Episode)$/.test(item.Type) && !!series),
          hidden: !!(hidden[id] || (series && hidden[series])),
          resume:
            cards.length > 0 &&
            Array.prototype.every.call(cards, function (other) {
              return resumable[plainId(other.getAttribute('data-id'))]
            }),
        }
      },
    )
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
        if (!container.isConnected || !model) return
        if (facts.resume) {
          scroller.appendChild(
            menuItem(container, model, 'playlist_remove', say('removeResume'), function () {
              act(entry.id, 'DELETE', 'Resume', function (data) {
                // The answer is what the user did with the item afterwards:
                // its last play, before the resume point went.
                var last = Date.parse((data && data.LastPlayedDate) || '') || 0
                var row = entry.card && entry.card.closest('.itemsContainer')
                if (row) {
                  screen(row, function (item) {
                    var played = Date.parse((item.UserData && item.UserData.LastPlayedDate) || '') || 0
                    return plainId(item.Id) === entry.id && played <= last
                  })
                }
                dropCards(entry.card ? [entry.card] : [])
              })
            }),
          )
        }
        if (facts.hideable && facts.hidden) {
          scroller.appendChild(
            menuItem(container, model, 'visibility', say('show'), function () {
              act(entry.id, 'DELETE', 'Hidden', function () {
                delete hiddenNow[entry.id]
                delete hiddenNow[facts.series]
                toast(say('shown'))
              })
            }),
          )
        } else if (facts.hideable) {
          scroller.appendChild(
            menuItem(container, model, 'visibility_off', say('hide'), function () {
              act(entry.id, 'POST', 'Hidden', function () {
                var titles = [entry.id, facts.series].filter(Boolean)
                titles.forEach(function (title) {
                  hiddenNow[title] = true
                })
                Array.prototype.forEach.call(document.querySelectorAll('.itemsContainer'), function (row) {
                  screen(row, hiddenItem)
                })
                var cards = Array.prototype.filter.call(document.querySelectorAll('.card[data-id]'), function (card) {
                  return titles.indexOf(plainId(card.getAttribute('data-id'))) >= 0
                })
                if (entry.card && cards.indexOf(entry.card) < 0) cards.push(entry.card)
                dropCards(cards)
                toast(say('hidden'))
              })
            }),
          )
        }
        // The menu was placed for its own entries: it moves up as far as
        // the added ones need.
        var box = sheet.getBoundingClientRect()
        if (sheet.style.top && box.bottom > window.innerHeight) {
          sheet.style.top = Math.max(0, window.innerHeight - box.height) + 'px'
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
      }),
      true,
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
  // jellyfin-web 12.2 keeps its rows' answers for a minute, and a row that
  // refreshes within it, as the change's message on the socket has it do,
  // draws them again from the answer kept: the cards an entry took off
  // would come back until the page loads again. screen has the rows'
  // refreshes leave out, from then on, what drop matches; the answer kept
  // is left as it is. The titles hidden are left out of every row of the
  // page, until shown again.
  var hiddenNow = {}
  function hiddenItem(item) {
    return !!(hiddenNow[plainId(item.Id)] || (item.SeriesId && hiddenNow[plainId(item.SeriesId)]))
  }
  function screen(row, drop) {
    if (typeof row.fetchData !== 'function') return
    if (!row.polyfinDrops) {
      var fetch = row.fetchData
      var drops = (row.polyfinDrops = [])
      row.fetchData = function () {
        return fetch.apply(this, arguments).then(function (answer) {
          function kept(item) {
            return !item || !drops.some(function (drop) {
              return drop(item)
            })
          }
          if (Array.isArray(answer)) return answer.filter(kept)
          if (answer && Array.isArray(answer.Items)) return Object.assign({}, answer, { Items: answer.Items.filter(kept) })
          return answer
        })
      }
    }
    if (row.polyfinDrops.indexOf(drop) < 0) row.polyfinDrops.push(drop)
  }
  // dropCards takes cards off the page, and hides a home row left empty.
  function dropCards(cards) {
    cards.forEach(function (card) {
      var row = card.closest('.itemsContainer')
      card.remove()
      var section = row && row.closest('.verticalSection')
      if (section && !row.querySelector('.card')) section.classList.add('hide')
    })
  }
  // toast says text as jellyfin-web's toasts do, for 3.3 seconds.
  function toast(text) {
    var holder = document.querySelector('.toastContainer')
    if (!holder) {
      holder = element('div', 'toastContainer', '')
      document.body.appendChild(holder)
    }
    var note = element('div', 'toast', text)
    holder.appendChild(note)
    setTimeout(function () {
      note.classList.add('toastVisible')
    }, 50)
    setTimeout(function () {
      note.classList.add('toastHide')
      setTimeout(function () {
        note.remove()
      }, 300)
    }, 3300)
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
  // Both run before jellyfin-web opens its item menu.
  addEventListener('click', quietly(aim), true)
  addEventListener('contextmenu', quietly(aim), true)
  if (document.body) {
    watchDialogs()
    watchMenus()
  } else {
    addEventListener('DOMContentLoaded', watchDialogs)
    addEventListener('DOMContentLoaded', watchMenus)
  }
  follow()
})()
