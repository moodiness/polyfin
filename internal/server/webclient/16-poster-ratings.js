// Part 16: ratings on posters.
//
// While the administrator turned it on (Settings › Web player ›
// Show ratings on posters), it shows a movie's or a series' ratings over
// its poster, at the top on the start side, as the pointer comes to its
// card or the focus does, the TV layout's included: its CommunityRating
// with a star, then its critics' and audience scores, in percent, each
// with an icon of its own, without words, those it lacks left out. Cards
// are jellyfin-web 12.4's (.card[data-id] with data-type Movie or
// Series, the poster in .cardScalable); others, and lists, get nothing.
// The first card met that is not known asks /Polyfin/Items/Ratings once
// for it and the other title cards of the page not known, those of its
// row first, 100 at most, which answers Enabled and the ratings of those
// with some; another card coming meanwhile waits for that answer. Every
// card answered gets its ratings at once, hidden by the style until the
// pointer or the focus comes, so that a row shows them without asking
// again; a card jellyfin-web draws anew gets them back as the pointer or
// the focus comes. Answers are kept for a minute, for the user signed in,
// and forgotten when another signs in. Turned off, nothing shows, and the
// server is asked again a minute later at the earliest, as is a server
// that did not answer; a server turning it off takes the ratings shown
// away. Nothing changes during a video. Any error just stops it.
//
// Anchors in jellyfin-web:
//   route: video
//   selector: .card[data-id]
//   selector: .itemsContainer
//   selector: .cardScalable
//   selector: .cardScalable:has(.mediaSourceIndicator)
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl

  var titleCard = '.card[data-id]'
  var titleTypes = { Movie: true, Series: true }
  // How long an answer is kept, so that turning the ratings on posters on
  // or off applies within a minute, how long a server that shows none, or
  // did not answer, is left unasked, and how many titles one request names.
  var keptFor = 60 * 1000
  var offFor = 60 * 1000
  var most = 100
  // The ratings answered, by identifier, as { at, rating }, rating null for
  // a title without any, for keptUser; until when not to ask; whether a
  // request is under way, and the card last met meanwhile.
  var kept = {}
  var keptUser = ''
  var offUntil = 0
  var asking = false
  var waiting = null
  // The icons, MUI's Material Icons (Apache License 2.0) as jellyfin-web
  // draws its own: a star, a review and a group of people.
  var icons = {
    community: 'M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z',
    critics:
      'M20 2H4c-1.1 0-1.99.9-1.99 2L2 22l4-4h14c1.1 0 2-.9 2-2V4c0-1.1-.9-2-2-2zM6 14v-2.47l6.88-6.88c.2-.2.51-.2.71 0l1.77 1.77c.2.2.2.51 0 .71L8.47 14H6zm12 0h-7.5l2-2H18v2z',
    audience:
      'M16 11c1.66 0 2.99-1.34 2.99-3S17.66 5 16 5c-1.66 0-3 1.34-3 3s1.34 3 3 3zm-8 0c1.66 0 2.99-1.34 2.99-3S9.66 5 8 5C6.34 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z',
  }
  // The percent sign as each of jellyfin-web's languages writes it after a
  // number, "%" for the others.
  var percents = { fr: '\u00a0%', de: '\u00a0%', es: '\u00a0%' }
  function percent() {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return percents[language] || '%'
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  function plainId(id) {
    return String(id).replace(/-/g, '').toLowerCase()
  }
  // titleOf returns the title card an element is in, null outside one.
  function titleOf(element) {
    var card = element && element.closest ? element.closest(titleCard) : null
    return card && titleTypes[card.getAttribute('data-type')] ? card : null
  }
  function known(id, now) {
    var entry = kept[id]
    return entry && now - entry.at < keptFor ? entry : null
  }
  // arrive follows the pointer and the focus to a title card: it shows the
  // card's ratings when known, and asks for them otherwise.
  function arrive(event) {
    var card = titleOf(event.target)
    var api = window.ApiClient
    if (!card || !api || /^#!?\/video(?:[/?#]|$)/i.test(location.hash)) return
    var user = api.getCurrentUserId() || ''
    if (!user) return
    if (user !== keptUser) {
      kept = {}
      keptUser = user
      offUntil = 0
    }
    var now = Date.now()
    var entry = known(plainId(card.getAttribute('data-id')), now)
    if (entry) return show(card, entry.rating)
    if (now < offUntil) return
    if (asking) {
      waiting = card
      return
    }
    ask(api, user, card, now)
  }
  // ask asks for the ratings of card and of the other title cards of the
  // page not known, those of its row first, 100 at most, then shows them.
  function ask(api, user, card, now) {
    var ids = []
    var row = card.closest('.itemsContainer')
    var cards = [card]
      .concat(row ? Array.prototype.slice.call(row.querySelectorAll(titleCard)) : [])
      .concat(Array.prototype.slice.call(document.querySelectorAll(titleCard)))
    for (var i = 0; i < cards.length && ids.length < most; i++) {
      if (!titleTypes[cards[i].getAttribute('data-type')]) continue
      var id = plainId(cards[i].getAttribute('data-id'))
      if (ids.indexOf(id) < 0 && !known(id, now)) ids.push(id)
    }
    asking = true
    var done = function () {
      asking = false
      var next = waiting
      waiting = null
      if (next) quietly(arrive)({ target: next })
    }
    api
      .getJSON(api.getUrl('Polyfin/Items/Ratings', { Ids: ids.join(',') }))
      .then(
        quietly(function (answer) {
          if (keptUser !== user) return
          var at = Date.now()
          if (!answer || !answer.Enabled) {
            offUntil = at + offFor
            strip()
            return
          }
          ids.forEach(function (id) {
            kept[id] = { at: at, rating: null }
          })
          ;(answer.Items || []).forEach(function (item) {
            if (item && item.Id) kept[plainId(item.Id)] = { at: at, rating: item }
          })
          decorate(at)
        }),
        function () {
          offUntil = Date.now() + offFor
        }
      )
      .then(done, done)
  }
  // decorate gives every title card of the page whose ratings are known
  // its ratings.
  function decorate(now) {
    var cards = document.querySelectorAll(titleCard)
    for (var i = 0; i < cards.length; i++) {
      if (!titleTypes[cards[i].getAttribute('data-type')]) continue
      var entry = known(plainId(cards[i].getAttribute('data-id')), now)
      if (entry) show(cards[i], entry.rating)
    }
  }
  // strip takes every card's ratings away.
  function strip() {
    var shown = document.querySelectorAll('.polyfinPosterRatings')
    for (var i = 0; i < shown.length; i++) if (shown[i].parentNode) shown[i].parentNode.removeChild(shown[i])
  }
  // show puts a title's ratings over its card's poster, once.
  function show(card, rating) {
    var poster = card.querySelector('.cardScalable')
    if (!rating || !poster || poster.querySelector('.polyfinPosterRatings')) return
    var entries = []
    if (rating.CommunityRating != null) entries.push(['community', Number(rating.CommunityRating).toFixed(1)])
    if (rating.CriticsScore != null) entries.push(['critics', Math.round(rating.CriticsScore) + percent()])
    if (rating.AudienceScore != null) entries.push(['audience', Math.round(rating.AudienceScore) + percent()])
    if (!entries.length) return
    style()
    var badge = document.createElement('div')
    badge.className = 'polyfinPosterRatings'
    // What the pointer or the focus already reaches: the card names the
    // title, and its page tells the ratings in words.
    badge.setAttribute('aria-hidden', 'true')
    entries.forEach(function (entry) {
      var score = document.createElement('span')
      score.className = 'polyfinPosterRating polyfinPosterRating-' + entry[0]
      var icon = document.createElement('span')
      icon.className = 'polyfinPosterRatingIcon'
      icon.innerHTML = '<svg viewBox="0 0 24 24" focusable="false"><path d="' + icons[entry[0]] + '"></path></svg>'
      score.appendChild(icon)
      score.appendChild(document.createTextNode(entry[1]))
      badge.appendChild(score)
    })
    poster.appendChild(badge)
  }
  // style adds the look of the ratings, once: hidden until the pointer or
  // the focus comes to the card, above jellyfin-web's hover menu, under a
  // version count, never in the pointer's way.
  function style() {
    if (document.getElementById('polyfinPosterRatingsStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinPosterRatingsStyle'
    sheet.textContent =
      '.polyfinPosterRatings{position:absolute;top:.45em;inset-inline-start:.45em;z-index:3;display:flex;flex-direction:column;' +
      'align-items:flex-start;gap:.3em;pointer-events:none;opacity:0;transition:opacity .15s ease}' +
      '.cardScalable:has(.mediaSourceIndicator) .polyfinPosterRatings{top:2.4em}' +
      '.card:hover .polyfinPosterRatings,.card:focus .polyfinPosterRatings,.card:focus-within .polyfinPosterRatings{opacity:1}' +
      '.polyfinPosterRating{display:inline-flex;align-items:center;gap:.3em;padding:.2em .5em;border-radius:1em;' +
      'background:rgba(0,0,0,.68);color:#fff;font-size:.8rem;font-weight:600;line-height:1.25;white-space:nowrap;' +
      'font-variant-numeric:tabular-nums;-webkit-backdrop-filter:blur(6px);backdrop-filter:blur(6px)}' +
      '.polyfinPosterRatingIcon{display:inline-flex}' +
      '.polyfinPosterRatingIcon svg{width:1.1em;height:1.1em;fill:currentColor}' +
      '.polyfinPosterRating-community .polyfinPosterRatingIcon{color:#ffc83d}' +
      '@media (prefers-reduced-motion:reduce){.polyfinPosterRatings{transition:none}}'
    document.head.appendChild(sheet)
  }
  addEventListener('mouseover', quietly(arrive), true)
  addEventListener('focusin', quietly(arrive), true)
