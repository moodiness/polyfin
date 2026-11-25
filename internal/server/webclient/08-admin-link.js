// Part 8: the admin app's entry for members.
//
// It gives a user signed in who is not an administrator, a member,
// a "Polyfin" entry where jellyfin-web 12.4 shows administrators Dashboard:
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
// entry shows as soon as jellyfin-web draws a menu. The page's changes put
// it back as jellyfin-web draws its menus anew. It never shows where
// Dashboard shows, nor before sign-in, and goes as soon as the user signed
// in is another, signing out included. Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: dashboard
//   route: mypreferencesmenu
//   selector: a[href="#/dashboard"]
//   selector: .mainDrawer .userMenuOptions
//   selector: .sidebarHeader
//   selector: .userSection
//   selector: #app-user-menu
//   selector: a[href="#/mypreferencesmenu"]
//   selector: svg[data-testid="LogoutIcon"]
//   selector: .MuiMenuItem-root
//   selector: .MuiListItemText-primary
//   selector: .MuiTouchRipple-root
//   class: MuiDivider-root
//   class: Mui-focusVisible
//   class: material-icons
//   class: dashboard
//   class: navMenuOption
//   class: lnkMediaFolder
//   class: emby-button
//   class: navMenuOptionIcon
//   class: navMenuOptionText
//   class: listItem
//   class: listItem-border
//   class: listItemBody
//   class: listItemBodyText
//   class: listItemIcon
//   class: listItemIcon-transparent
//   class: verticalSection
//   class: verticalSection-extrabottompadding
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getCurrentUser

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
            })
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
  // A link to the admin app, as its dashboard routes open it. A plain link:
  // jellyfin-web's emby-linkbutton would open it within jellyfin-web.
  function adminLink(className, content) {
    var link = element('a', className, content)
    link.setAttribute('href', fromWebClient(adminAddress('')))
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
    item.setAttribute('href', fromWebClient(adminAddress('')))
    text.textContent = 'Polyfin'
    path.setAttribute('d', dashboardIcon)
    path.parentNode.removeAttribute('data-testid')
    var ripple = item.querySelector('.MuiTouchRipple-root')
    if (ripple) ripple.textContent = ''
    return item
  }
  // jellyfin-web draws its menus anew, which the page's changes tell (see
  // core/page-changes.js).
  document.addEventListener('polyfinpagechange', placeEntries)
