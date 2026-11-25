// Part 23: trailers in YouTube's player.
//
// It lets jellyfin-web's trailers play in YouTube's
// player, which refuses to play (error 153) without a Referer. The page
// sends none: Polyfin's Referrer-Policy header and jellyfin-web's own
// <meta name="referrer"> both say no-referrer, and keep saying it for
// every other request. YouTube's player script marks its frame with a
// referrerpolicy attribute, but gives the frame its address once the
// frame is in the page, and some browsers apply the attribute only as a
// frame is inserted, so that the frame's load sends nothing. The script
// watches the body's own children for jellyfin-web's trailer player
// (.youtubePlayerContainer, which jellyfin-web 12.4 puts first in the
// body), then that player's frames: a frame there whose address is a
// YouTube embed (https://www.youtube.com/embed/…, or the same on
// www.youtube-nocookie.com) gets referrerpolicy="strict-origin", so that
// it sends the server's origin only, never a path, and is put back in
// its place, which loads it again with it. A browser that applied the
// attribute already loads it twice, the first load cut short. A new
// address of the frame gets the same, once. Any other frame, and every
// other request, is left as it was. Any error just stops it.
//
// Anchors in jellyfin-web:
//   class: youtubePlayerContainer

  if (typeof MutationObserver !== 'function') return
  var embed = /^https:\/\/www\.youtube(?:-nocookie)?\.com\/embed\//
  // The server's origin only, never a path, and nothing to an address
  // over http.
  var policy = 'strict-origin'
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // mend gives the policy to the YouTube frames of the trailer's player,
  // and loads again each one at an address it was not loaded again for.
  function mend(player) {
    var live = player.getElementsByTagName('iframe')
    // Putting a frame back changes the live list.
    var frames = []
    for (var i = 0; i < live.length; i++) frames.push(live[i])
    frames.forEach(function (frame) {
      var address = frame.getAttribute('src') || ''
      if (!embed.test(address) || frame.polyfinReferrerFor === address) return
      frame.polyfinReferrerFor = address
      frame.setAttribute('referrerpolicy', policy)
      if (frame.parentNode) frame.parentNode.insertBefore(frame, frame.nextSibling)
    })
  }
  function follow(player) {
    player.polyfinTrailerFollowed = true
    new MutationObserver(
      quietly(function () {
        mend(player)
      })
    ).observe(player, { childList: true, subtree: true, attributes: true, attributeFilter: ['src'] })
    mend(player)
  }
  function watch() {
    var children = document.body.children
    for (var i = 0; i < children.length; i++) {
      var child = children[i]
      if (!child.polyfinTrailerFollowed && child.classList && child.classList.contains('youtubePlayerContainer')) follow(child)
    }
  }
  // The script runs from the page's head: the body comes later.
  function start() {
    new MutationObserver(quietly(watch)).observe(document.body, { childList: true })
    watch()
  }
  if (document.body) quietly(start)()
  else addEventListener('DOMContentLoaded', quietly(start))
