// Part 4: long version names in the menus of a title's page.
//
// It keeps a long name from running under the arrow of the menus of
// a title's page: stream addons name versions with every detail, and
// jellyfin-web leaves no room for the arrow in those menus. Their text ends
// with "…" before it; the open menu still lists whole names.
//
// Anchors in jellyfin-web:
//   selector: select.detailTrackSelect.emby-select-withcolor

  var menus = document.createElement('style')
  menus.textContent =
    'select.detailTrackSelect.emby-select-withcolor{padding-right:2.2em!important;text-overflow:ellipsis}'
  document.head.appendChild(menus)
