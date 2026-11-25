// Part 7: libraries hidden from the menus.
//
// It leaves out of jellyfin-web's menus the libraries chosen to be
// hidden there on the Libraries page of Polyfin's admin app, by an
// administrator for the server's libraries or by the user for their own;
// each keeps its row on the home page, and other apps list it. Once a user
// is signed in, it asks /Polyfin/UserViews, which lists the user's views as
// /UserViews does, in its order, each with HideInMenus and Catalog (see
// the fifth), and keeps one style that hides the links of each such library:
// the buttons of the top bar (jellyfin-web 12.4's UserViewNav, MUI Buttons
// in the AppBar whose address holds the view's identifier), its More menu
// (#user-view-overflow-menu), the side menu of that layout (an MUI Drawer,
// used below 900 pixels wide instead of the buttons) and the legacy
// layout's side menu (.libraryMenuOptions, by data-itemid). Other links to
// the library, such as its home row's title, the tabs of its page or the
// search button on that page, are left alone. The top bar shows the first 3
// views, 5 from 1200 pixels wide and 8 from 1536 (MUI's lg and xl
// breakpoints), or all of them when only one more would go under More, its
// button listing the rest; the views come after config.json's menuLinks,
// none in the one Polyfin ships.
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
// Anchors in jellyfin-web:
//   selector: .MuiAppBar-root a.MuiButton-root[href*="id"]
//   selector: #user-view-overflow-menu a[href*="id"]
//   selector: .MuiDrawer-root a[href*="id"]
//   selector: .libraryMenuOptions a[data-itemid="id"]
//   selector: .MuiAppBar-root button[aria-controls="user-view-overflow-menu"]
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl

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
