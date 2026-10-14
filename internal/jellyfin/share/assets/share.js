// Polyfin (MIT License). The page of a share link, at /share/{token}: it
// shows the title's artwork, the link's expiry in the reader's language,
// and plays the title over HLS, through hls.js where the browser has no
// HLS of its own. Pressing Play asks the server to start a playback
// (POST …/play), which answers its play session, or why it does not
// start; the page then reports the position every 10 seconds, and the end
// of the playback when the video ends or the page closes.
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
  if (!video) return
  var button = document.getElementById('play')
  var message = document.getElementById('message')
  var poster = document.querySelector('img.poster')
  poster.addEventListener('load', function () {
    poster.hidden = false
  })
  poster.src = base + '/image/primary'
  // The backdrop is the player's poster, when the title has one.
  var backdrop = new Image()
  backdrop.addEventListener('load', function () {
    if (!video.currentTime) video.poster = backdrop.src
  })
  backdrop.src = base + '/image/backdrop'

  var session = null
  var hls = null
  var timer = null

  function say(code) {
    message.textContent = messages[code] || messages.error || ''
    message.hidden = false
  }

  function playback() {
    return base + '/play/' + session
  }

  // end stops the playback on the page, and on the server unless it
  // already ended there.
  function end(told) {
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
    button.hidden = false
    button.disabled = false
  }

  // report tells the server where the playback is; an answer that it no
  // longer plays says why, and ends it on the page.
  function report() {
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
        })
      })
      .catch(function () {})
  }

  function start() {
    message.hidden = true
    button.disabled = true
    fetch(base + '/play', { method: 'POST' })
      .then(function (response) {
        return response.json().then(function (answer) {
          if (!response.ok) {
            say(answer.error)
            button.disabled = false
            return
          }
          session = answer.session
          var source = playback() + '/master.m3u8'
          if (window.Hls && window.Hls.isSupported()) {
            hls = new window.Hls({ enableWorker: true })
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
            video.src = source
          } else {
            say('error')
            end(false)
            return
          }
          button.hidden = true
          video.play().catch(function () {})
          timer = setInterval(report, 10000)
        })
      })
      .catch(function () {
        say('error')
        button.disabled = false
      })
  }

  button.addEventListener('click', start)
  video.addEventListener('play', function () {
    if (!session && !button.disabled) start()
  })
  video.addEventListener('pause', report)
  video.addEventListener('ended', function () {
    end(false)
  })
  window.addEventListener('pagehide', function () {
    end(false)
  })
})()
