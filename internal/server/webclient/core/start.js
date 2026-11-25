// The end of the function that runs the parts of core/shared.js: it follows
// jellyfin-web's routes for them, once each has set itself up.
//
// Anchors in jellyfin-web: none.

  // follow sends an administration page to the admin app (see
  // 01-admin-pages.js), else tells the parts that follow the routes.
  function follow() {
    if (adminRoute.test(location.hash)) {
      // Replaced, so that Back returns to the page before.
      location.replace(fromWebClient(adminAddress(target(location.hash))))
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
    try {
      followShare()
    } catch (error) {}
    // The socket's SyncPlay messages tell the group the session is in from
    // the first route on (see the tenth).
    try {
      listen()
    } catch (error) {}
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
  if (document.body) watchDialogs()
  else addEventListener('DOMContentLoaded', watchDialogs)
  follow()
