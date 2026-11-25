// Part 20: the volume boost.
//
// It makes videos louder in jellyfin-web's player for a user
// with Boost the volume in the web player (My account › Sound, or their
// user's or group's page), as /Polyfin/Sound answers it (BoostVolume): it
// sends the sound of the <video> element through Web Audio, the element
// into a gain of 2 then a compressor (threshold -6 dB, knee 6 dB, ratio
// 12, attack 3 ms, release 250 ms) then the speakers, so that quiet
// passages rise by about 8 dB and loud ones by about 5, their peaks held
// near full scale. The element's own volume, jellyfin-web's slider,
// applies before the gain. It asks the server once for each user signed
// in, as the route changes, and again as each video starts, which also
// tells the server that this device runs the script: the server then
// relays the user's direct plays rather than sending the player to their
// source, and marks their address (PolyfinRelay=1). For a user who
// boosts, the page's one AudioContext is made and started at their first
// click or tap, inside it: Safari starts a context only there, and the
// video starts later, once jellyfin-web has asked the server how to play
// it. The context is suspended again 3 s after a click, a pause or an end
// while no video it boosts plays, so that it holds no audio session, and
// resumed as one plays, which Safari allows once a tap started it. As a
// video starts playing, the script builds the graph for its element,
// once, on that context; an element whose sound Web Audio would play
// silent is left alone: a stream from
// another site fetched without CORS, as one jellyfin-web plays from its
// source unless the address is marked or the element fetched it with CORS
// (crossorigin, set for the server's own files); and HLS the browser plays
// itself (an .m3u8 address, not hls.js's blob: one), as Safari and iOS
// do, whose sound Web Audio may not get. A graph built stays, as Web Audio
// cannot give an element back: turning the boost off sends the element
// straight to the speakers, and a later video of an element whose sound
// would be silent through Web Audio is not one the server sends a user who
// boosts. Audio elements, which jellyfin-web's music player sends through
// Web Audio itself, are never touched. During a video, the player's
// settings menu (the action sheet .btnVideoOsdSettings opens, which lists
// such entries as stats or playbackrate) gets "Boost the volume", On or
// Off: it turns the boost on or off at once, for this page, and posts the
// choice to /Polyfin/Sound, which a member following their group may not
// change; a video that cannot be boosted now is from the next one, which a
// toast says. Its words follow jellyfin-web's language, as the third's.
// Without Web Audio it does nothing, and any error just stops it.
//
// Anchors in jellyfin-web:
//   selector: .dialog[data-history="true"]
//   selector: .toastContainer
//   selector: .actionSheetScroller
//   selector: .actionSheetMenuItem
//   selector: .actionSheetItemText
//   selector: .actionSheetItemAsideText
//   selector: .btnVideoOsdSettings
//   class: toast
//   class: toastVisible
//   class: toastHide
//   class: listItemAside
//   class: dialogContainer
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.serverId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl
//   global: ApiClient.ajax

  var Context = typeof window === 'object' && window && (window.AudioContext || window.webkitAudioContext)
  if (typeof Context !== 'function') return
  var boostWords = {
    en: { boost: 'Boost the volume', on: 'On', off: 'Off', next: 'The volume is boosted from the next video.' },
    fr: { boost: 'Augmenter le volume', on: 'Activé', off: 'Désactivé', next: 'Le volume est augmenté à partir de la prochaine vidéo.' },
    de: { boost: 'Lautstärke anheben', on: 'Ein', off: 'Aus', next: 'Die Lautstärke wird ab dem nächsten Video angehoben.' },
    es: { boost: 'Subir el volumen', on: 'Activado', off: 'Desactivado', next: 'El volumen se sube a partir del próximo vídeo.' },
    it: { boost: 'Aumenta il volume', on: 'Attivo', off: 'Disattivo', next: 'Il volume viene aumentato dal prossimo video.' },
    pt: { boost: 'Aumentar o volume', on: 'Ativado', off: 'Desativado', next: 'O volume é aumentado a partir do próximo vídeo.' },
  }
  function say(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (boostWords[language] || boostWords.en)[key]
  }
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // The player's own settings entries, which tell its menu from others.
  var playerEntries = ['aspectratio', 'playbackrate', 'quality', 'repeatmode', 'suboffset', 'stats']
  var menuWithin = 10000
  // chosen is the user's choice as the server last answered it, for the
  // user signed in (server/user); own, once the player's menu changed it,
  // the page's, until it loads again.
  var chosen = { user: '', boost: false }
  var own = null
  var asked = ''
  var context = null
  // The graphs built: each element, its source, gain and compressor, and
  // whether it goes through the gain.
  var graphs = []
  var menuAt = 0
  function viewer() {
    var api = window.ApiClient
    var id = api && api.getCurrentUserId && api.getCurrentUserId()
    return id ? (typeof api.serverId === 'function' ? api.serverId() : '') + '/' + id : ''
  }
  function wanted() {
    if (own !== null && own.user === viewer()) return own.boost
    return chosen.user === viewer() && chosen.boost
  }
  // ask asks the user's choice, then applies it to the videos playing.
  function ask() {
    var api = window.ApiClient
    var user = viewer()
    if (!user) return
    asked = user
    api.getJSON(api.getUrl('Polyfin/Sound')).then(
      quietly(function (answer) {
        if (!answer || viewer() !== user) return
        chosen = { user: user, boost: answer.BoostVolume === true }
        applyAll()
      }),
      function () {}
    )
  }
  // follow asks once for each user signed in, as the route changes.
  function follow() {
    var user = viewer()
    if (user && user !== asked) ask()
  }
  function sameSite(address) {
    var here = location.protocol + '//' + location.host + '/'
    return address.slice(0, here.length).toLowerCase() === here.toLowerCase()
  }
  // safe tells whether Web Audio gets the sound of the video an element
  // plays (see the header).
  function safe(video) {
    var address = String(video.currentSrc || video.src || '')
    if (!address) return false
    if (/^blob:/i.test(address)) return true
    if (/\.m3u8(?:[?#]|$)/i.test(address)) return false
    if (video.getAttribute('crossorigin') !== null) return true
    return sameSite(address) && /[?&]PolyfinRelay=1(?:[&#]|$)/.test(address)
  }
  function attached(node) {
    return document.documentElement.contains ? document.documentElement.contains(node) : true
  }
  function graphOf(video) {
    for (var i = 0; i < graphs.length; i++) {
      if (graphs[i].video === video) return graphs[i]
    }
    return null
  }
  function route(graph, on) {
    if (graph.boosted === on) return
    graph.source.disconnect()
    graph.source.connect(on ? graph.gain : context.destination)
    graph.boosted = on
  }
  // build sends the element through Web Audio, once the context runs: in
  // a suspended context the element would play silent.
  function build(video, done) {
    if (!context) context = new Context()
    var make = quietly(function () {
      if (context.state !== 'running' || graphOf(video)) return
      // The graphs of elements gone are let go.
      graphs = graphs.filter(function (graph) {
        if (attached(graph.video)) return true
        graph.source.disconnect()
        graph.compressor.disconnect()
        return false
      })
      var gain = context.createGain()
      gain.gain.value = 2
      var compressor = context.createDynamicsCompressor()
      compressor.threshold.value = -6
      compressor.knee.value = 6
      compressor.ratio.value = 12
      compressor.attack.value = 0.003
      compressor.release.value = 0.25
      gain.connect(compressor)
      compressor.connect(context.destination)
      var graph = { video: video, source: context.createMediaElementSource(video), gain: gain, compressor: compressor, boosted: null }
      graphs.push(graph)
      done(graph)
    })
    if (context.state === 'running') make()
    else if (typeof context.resume === 'function') context.resume().then(make, function () {})
  }
  // apply boosts a video as the choice says: a graph built is routed, one
  // not built is built for a safe element only.
  function apply(video) {
    var on = wanted()
    var graph = graphOf(video)
    if (graph) {
      if (context.state !== 'running' && typeof context.resume === 'function') context.resume().then(null, function () {})
      route(graph, on)
      return
    }
    if (!on || !safe(video)) return
    build(video, function (built) {
      route(built, wanted())
    })
  }
  function videos() {
    return Array.prototype.slice.call(document.getElementsByTagName('video'))
  }
  function applyAll() {
    videos().forEach(quietly(apply))
  }
  function played(event) {
    var video = event.target
    if (!video || String(video.tagName).toUpperCase() !== 'VIDEO') return
    apply(video)
    ask()
  }
  // idle suspends the context while no video it boosts plays, so that an
  // idle context holds no audio session, as on an iPhone, where it could
  // stop another app's music. A tap having started it once, Safari lets
  // the page resume it later without one, as build and apply do.
  var resting = 0
  function idle() {
    if (!context || context.state !== 'running' || typeof context.suspend !== 'function') return
    for (var i = 0; i < graphs.length; i++) {
      if (attached(graphs[i].video) && !graphs[i].video.paused) return
    }
    context.suspend().then(null, function () {})
  }
  function rest() {
    clearTimeout(resting)
    resting = setTimeout(quietly(idle), 3000)
  }
  function stopped(event) {
    if (graphOf(event.target)) rest()
  }
  function close(container) {
    var dialog = container.querySelector('.dialog')
    if (dialog && dialog.getAttribute('data-history') === 'true') history.back()
    else container.remove()
  }
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
        shown.remove()
      }, 300)
    }, 3300)
  }
  // toggle turns the boost on or off for the page, applies it, and saves
  // the choice.
  function toggle() {
    var on = !wanted()
    own = { user: viewer(), boost: on }
    var later = false
    videos().forEach(function (video) {
      if (on && !graphOf(video) && !safe(video)) later = true
      quietly(apply)(video)
    })
    if (later) toast(say('next'))
    var api = window.ApiClient
    api.ajax({
      type: 'POST',
      url: api.getUrl('Polyfin/Sound'),
      data: JSON.stringify({ BoostVolume: on }),
      contentType: 'application/json',
      dataType: 'json',
    }).then(
      quietly(function (answer) {
        if (answer && own && own.user === viewer()) chosen = { user: own.user, boost: answer.BoostVolume === true }
      }),
      function () {}
    )
  }
  // extend adds the entry to the player's settings menu a dialog
  // container holds, opened from its button lately, during a video.
  function extend(container) {
    if (Date.now() - menuAt > menuWithin || !videos().length) return
    var scroller = container.querySelector && container.querySelector('.actionSheetScroller')
    if (!scroller || scroller.querySelector('.polyfinBoost')) return
    var items = scroller.querySelectorAll('.actionSheetMenuItem')
    var model = null
    for (var i = 0; i < items.length; i++) {
      if (playerEntries.indexOf(items[i].getAttribute('data-id')) >= 0) model = items[i]
    }
    if (!model) return
    menuAt = 0
    var item = model.cloneNode(true)
    item.setAttribute('data-id', 'polyfinBoost')
    item.classList.add('polyfinBoost')
    var words = item.querySelector('.actionSheetItemText')
    if (words) words.textContent = say('boost')
    var aside = item.querySelector('.actionSheetItemAsideText')
    if (!aside) {
      aside = document.createElement('div')
      aside.className = 'listItemAside actionSheetItemAsideText'
      item.appendChild(aside)
    }
    aside.textContent = say(wanted() ? 'on' : 'off')
    item.addEventListener(
      'click',
      quietly(function (event) {
        // Before the menu hears the click, which would ask jellyfin-web to
        // run a setting it does not know.
        event.stopPropagation()
        event.preventDefault()
        close(container)
        toggle()
      }),
      true
    )
    scroller.appendChild(item)
  }
  // aim notes a click on the player's settings button, and starts the
  // page's context inside the click for a user who boosts: Safari and iOS
  // start an AudioContext only there, not when the video starts playing,
  // after jellyfin-web's PlaybackInfo, so the first video would stay
  // unboosted. build reuses the context; idle suspends it again 3 s later
  // when no video it boosts plays.
  function aim(event) {
    var target = event.target
    menuAt = target && target.closest && target.closest('.btnVideoOsdSettings') ? Date.now() : 0
    if (!wanted()) return
    if (!context) context = new Context()
    if (context.state !== 'running' && typeof context.resume === 'function') context.resume().then(null, function () {})
    rest()
  }
  function watch() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(
      quietly(function (records) {
        for (var i = 0; i < records.length; i++) {
          var added = records[i].addedNodes || []
          for (var j = 0; j < added.length; j++) {
            if (added[j].classList && added[j].classList.contains('dialogContainer')) quietly(extend)(added[j])
          }
        }
      })
    ).observe(document.body, { childList: true })
  }
  addEventListener('play', quietly(played), true)
  addEventListener('pause', quietly(stopped), true)
  addEventListener('ended', quietly(stopped), true)
  addEventListener('click', quietly(aim), true)
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
  if (document.body) watch()
  else addEventListener('DOMContentLoaded', watch)
  quietly(follow)()
