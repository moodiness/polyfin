// Part 1: the admin app for jellyfin-web's administration pages.
//
// It sends the pages of jellyfin-web that need a Jellyfin server's
// administration, which Polyfin does not have, to Polyfin's admin app. It
// watches the route only, never the page, so that a new jellyfin-web keeps
// working with it. jellyfin-web 12.4 routes after the "#", matched without
// regard to case as its router does, "#!/" being an older form:
//   #/dashboard and every #/dashboard/… page (users, libraries, playback,
//     Live TV, devices, activity, logs, API keys, scheduled tasks, plugins…);
//   #/metadata, the metadata manager;
//   #/configurationpage, plugin configuration pages;
//   #/wizard/…, the startup wizard: Polyfin is set up in its admin app.
// A dashboard page with a match in the admin app opens that page; any other
// opens the admin app's home. Every link of the script to the admin app,
// these included, carries ?from=web: the admin app, opened from the web
// client signed in as another user than its own session's, signs that
// session out and takes the web client's sign-in, so that a browser shows
// one user, as Jellyfin's dashboard, part of jellyfin-web, does.
//
// Anchors in jellyfin-web:
//   route: dashboard
//   route: dashboard/users
//   route: users/:userId/:tab
//   route: dashboard/devices
//   route: dashboard/libraries
//   route: dashboard/livetv/recordings
//   route: dashboard/livetv/tuner
//   route: dashboard/livetv
//   route: dashboard/playback/transcoding
//   route: dashboard/playback/resume
//   route: dashboard/playback/trickplay
//   route: dashboard/playback
//   route: dashboard/branding
//   route: dashboard/settings
//   route: dashboard/backups
//   route: dashboard/logs
//   route: dashboard/tasks
//   route: tasks/:id
//   route: dashboard/keys
//   route: metadata
//   route: configurationpage
//   route: wizard

  var adminRoute = /^#!?\/(dashboard|metadata|configurationpage|wizard)(?:[/?#]|$)/i
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
  ]
  // The admin page for a route: "" (home) unless it is a dashboard page with a match. A user's
  // dashboard pages, users/<id>/<tab> (jellyfin-web 12.4's users/:userId/:tab), open that user.
  // A tuner's page names it with ?id=, the identifier of its source among the server's, which
  // opens that source; without one, the page adds a tuner, which opens Add a source on its
  // HDHomeRun tuner tab. A task's page, tasks/<id> (12.4's tasks/:id), opens that task's
  // triggers on the schedule.
  function target(hash) {
    if (!/^#!?\/dashboard(?:[/?#]|$)/i.test(hash)) return ''
    var path = hash.replace(/^#!?\/dashboard\/?/i, '').split(/[?#]/)[0].toLowerCase()
    // The admin app names users by 32 lowercase hexadecimal digits.
    var user = /^users\/([0-9a-f-]+)(?:\/|$)/.exec(path)
    var userId = user ? user[1].replace(/-/g, '') : ''
    if (/^[0-9a-f]{32}$/.test(userId)) return 'users/' + userId
    if (/^livetv\/tuner(\/|$)/.test(path)) {
      var tuner = /[?&]id=([0-9a-f]{32})(?:[&#]|$)/i.exec(hash)
      return tuner ? 'sources/shared/' + tuner[1].toLowerCase() : 'sources?add=hdhomerun'
    }
    var task = /^tasks\/([0-9a-f]{32})(?:\/|$)/.exec(path)
    if (task) return 'system/schedule?task=' + task[1]
    for (var i = 0; i < pages.length; i++) {
      var prefix = pages[i][0]
      if (path === prefix || path.indexOf(prefix + '/') === 0) return pages[i][1]
    }
    return ''
  }
