// Polyfin (MIT License). Served at /web/polyfin.js and loaded by the
// index.html of jellyfin-web, which Polyfin ships unmodified: it sends the
// pages of jellyfin-web that need a Jellyfin server's administration, which
// Polyfin does not have, to Polyfin's admin app instead.
//
// It watches the route only, never the page, so that a new jellyfin-web
// keeps working with it. jellyfin-web 12.1 routes after the "#", matched
// without regard to case as its router does, "#!/" being an older form:
//   #/dashboard and every #/dashboard/… page (users, libraries, playback,
//     Live TV, devices, activity, logs, API keys, scheduled tasks, plugins…);
//   #/metadata, the metadata manager;
//   #/configurationpage, plugin configuration pages;
//   #/wizard/…, the startup wizard: Polyfin is set up in its admin app.
;(function () {
  var adminRoute = /^#!?\/(dashboard|metadata|configurationpage|wizard)(?:[/?#]|$)/i
  function follow() {
    if (adminRoute.test(location.hash)) {
      // Replaced, so that Back returns to the page before.
      location.replace(new URL('../admin/', location.href).href)
    }
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
  follow()
})()
