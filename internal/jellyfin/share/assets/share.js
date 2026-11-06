// Polyfin (MIT License). The page of a share link, at /share/{token}: it
// shows the title's artwork, the link's expiry in the reader's language,
// and plays the title over HLS, through hls.js where the browser has no
// HLS of its own. Pressing Play asks the server to start a playback
// (POST …/play), which answers its play session, or why it does not
// start; the page then reports the position every 10 seconds, and the end
// of the playback when the video ends or the page closes.
//
// Where the guest stopped is kept in this browser only, per link, under
// the link's ID, never its token: on the next visit the page offers to
// resume there or to start from the beginning, and a playback the server
// ended after a long pause starts again there. A position near the end,
// one kept for longer than any link lasts, and the position of a link
// expired or revoked are forgotten.
'use strict'
;(function () {
  var video = document.getElementById('video')
  var data = document.getElementById('share-messages')
  var messages = data ? JSON.parse(data.textContent) : {}
  var base = location.pathname.replace(/\/+$/, '')
  var time = document.querySelector('time[datetime]')
  if (time) {
    try {
      time.textContent = new Date(time.getAttribute('datetime')).toLocaleString(document.documentElement.lang, {
        dateStyle: 'long',
        timeStyle: 'short',
      })
    } catch (e) {}
  }

  // The positions kept, by link: {position, savedAt}, in seconds and
  // milliseconds since the epoch.
  var prefix = 'polyfin.share.'
  // kept is how long a position is kept: longer than the longest expiry.
  var kept = 31 * 24 * 3600 * 1000
  var page = document.querySelector('main.share')
  var link = page && page.getAttribute('data-link')
  var key = link ? prefix + link : null
  var store = null
  try {
    store = window.localStorage
  } catch (e) {}

  function read(name) {
    try {
      var entry = JSON.parse(store.getItem(name))
      if (entry && typeof entry.position === 'number' && isFinite(entry.position) && typeof entry.savedAt === 'number') return entry
    } catch (e) {}
    return null
  }

  function forget() {
    if (!store || !key) return
    try {
      store.removeItem(key)
    } catch (e) {}
  }

  // gone tells the refusals of a link that will never play again.
  function gone(code) {
    return code === 'expired' || code === 'revoked'
  }

  if (store) {
    try {
      var now = Date.now()
      for (var i = store.length - 1; i >= 0; i--) {
        var name = store.key(i)
        if (!name || name.indexOf(prefix) !== 0) continue
        var entry = read(name)
        if (!entry || !(now - entry.savedAt <= kept)) store.removeItem(name)
      }
    } catch (e) {}
  }
  if (page && page.hasAttribute('data-gone')) forget()

  if (!video) return
  var button = document.getElementById('play')
  var resume = document.getElementById('resume')
  var message = document.getElementById('message')
  var poster = document.querySelector('img.poster')
  poster.addEventListener('load', function () {
    poster.hidden = false
  })
  poster.src = base + '/image/primary'
  // The backdrop is the player's poster, when the title has one, and
  // tints the top of the page, blurred far.
  var room = document.querySelector('.room img')
  var backdrop = new Image()
  backdrop.addEventListener('load', function () {
    if (!video.currentTime) video.poster = backdrop.src
    if (room) {
      room.src = backdrop.src
      room.classList.add('on')
    }
  })
  backdrop.src = base + '/image/backdrop'

  var runtime = Number(page.getAttribute('data-runtime')) || 0
  var label = button.textContent
  var session = null
  var hls = null
  var timer = null
  // at is where the next playback starts; started tells that the video
  // plays from there, so that its own position counts.
  var at = 0
  var started = false

  function say(code) {
    message.textContent = messages[code] || messages.error || ''
    message.hidden = false
  }

  function playback() {
    return base + '/play/' + session
  }

  function position() {
    return started ? video.currentTime || 0 : at
  }

  // nearEnd tells a position within the last 5 % of the title, or its
  // last 2 minutes, which end it.
  function nearEnd(position) {
    var length = video.duration > 0 && isFinite(video.duration) ? video.duration : runtime
    return length > 0 && position >= length - Math.max(length * 0.05, 120)
  }

  function save() {
    if (!store || !key) return
    var where = position()
    if (!(where > 0) || nearEnd(where)) return forget()
    try {
      store.setItem(key, JSON.stringify({ position: where, savedAt: Date.now() }))
    } catch (e) {}
  }

  // clock writes a position as h:mm:ss.
  function clock(seconds) {
    seconds = Math.floor(seconds)
    var minutes = Math.floor(seconds / 60) % 60
    var rest = seconds % 60
    return Math.floor(seconds / 3600) + ':' + (minutes < 10 ? '0' : '') + minutes + ':' + (rest < 10 ? '0' : '') + rest
  }

  // offer shows Resume at the position kept, with From the beginning,
  // or Play alone.
  function offer(position) {
    at = position
    resume.hidden = !(position >= 1)
    if (position >= 1) {
      resume.textContent = resume.getAttribute('data-label').replace('{time}', clock(position))
      button.textContent = button.getAttribute('data-again')
      button.classList.add('secondary')
    } else {
      button.textContent = label
      button.classList.remove('secondary')
    }
  }

  // end stops the playback on the page, and on the server unless it
  // already ended there, keeping where it was for the next one.
  function end(told) {
    if (session) {
      at = position()
      save()
    }
    started = false
    clearInterval(timer)
    timer = null
    if (hls) hls.destroy()
    hls = null
    if (session && !told) {
      if (navigator.sendBeacon) navigator.sendBeacon(playback() + '/stop', '')
      else fetch(playback() + '/stop', { method: 'POST', keepalive: true }).catch(function () {})
    }
    session = null
    video.removeAttribute('src')
    video.load()
    button.textContent = label
    button.classList.remove('secondary')
    resume.hidden = true
    button.hidden = false
    button.disabled = false
  }

  // report keeps where the playback is, and tells the server; an answer
  // that it no longer plays says why, and ends it on the page.
  function report() {
    save()
    if (!session) return
    fetch(playback() + '/progress', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ position: video.currentTime || 0, paused: video.paused }),
    })
      .then(function (response) {
        if (response.ok) return
        return response.json().then(function (answer) {
          say(answer.error)
          end(true)
          if (gone(answer.error)) forget()
        })
      })
      .catch(function () {})
  }

  function enable(enabled) {
    button.disabled = !enabled
    resume.disabled = !enabled
  }

  function start() {
    var from = at
    message.hidden = true
    enable(false)
    fetch(base + '/play', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ position: from }),
    })
      .then(function (response) {
        return response.json().then(function (answer) {
          if (!response.ok) {
            say(answer.error)
            if (gone(answer.error)) {
              forget()
              offer(0)
            }
            enable(true)
            return
          }
          session = answer.session
          started = false
          var source = playback() + '/master.m3u8'
          if (window.Hls && window.Hls.isSupported()) {
            hls = new window.Hls({ enableWorker: true, startPosition: from > 0 ? from : -1 })
            hls.on(window.Hls.Events.ERROR, function (event, error) {
              if (!error.fatal) return
              // The server converts as many videos as it may; any other
              // refusal is told by the answer to a report.
              if (error.response && error.response.code === 503) say('busy')
              else report()
            })
            hls.loadSource(source)
            hls.attachMedia(video)
          } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
            if (from > 0) {
              video.addEventListener('loadedmetadata', function seek() {
                video.removeEventListener('loadedmetadata', seek)
                video.currentTime = from
              })
            }
            video.src = source
          } else {
            say('error')
            end(false)
            return
          }
          button.hidden = true
          resume.hidden = true
          video.play().catch(function () {})
          timer = setInterval(report, 10000)
        })
      })
      .catch(function () {
        say('error')
        enable(true)
      })
  }

  var entry = key && store ? read(key) : null
  if (entry && nearEnd(entry.position)) forget()
  else if (entry) offer(entry.position)

  resume.addEventListener('click', start)
  button.addEventListener('click', function () {
    // Next to Resume, the button starts from the beginning.
    if (!resume.hidden) at = 0
    start()
  })
  video.addEventListener('play', function () {
    if (!session && !button.disabled) start()
  })
  // The video plays from where the playback started once it plays and
  // does not seek there any more.
  video.addEventListener('playing', function () {
    if (session && !video.seeking) started = true
  })
  video.addEventListener('seeked', function () {
    if (session && !video.paused) started = true
  })
  video.addEventListener('pause', report)
  video.addEventListener('ended', function () {
    end(false)
    at = 0
    forget()
  })
  window.addEventListener('pagehide', function () {
    end(false)
  })
})()
