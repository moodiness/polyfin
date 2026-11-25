// Part 9: a title's three scores.
//
// On a title's page, it shows the title's three scores in the first
// line of its details (.itemMiscInfo-primary): its IMDb rating, its
// critics' score and its audience score, labelled, as
// /Polyfin/Items/{id}/Ratings answers them while the server has an MDBList
// key for ratings (Settings › Content). jellyfin-web shows only
// CommunityRating, with a star, and CriticRating, with its own icon: the
// script hides both in that line while its scores show, and gives them back
// as the route leaves the title. The answer may come while MDBList is
// asked about the title (Pending): it asks again 3 seconds later, 10 times
// at most. A MutationObserver on the page puts the scores back as
// jellyfin-web renders the line anew. An answer without a score, or no
// answer, changes nothing; nor does anything during a video. Its words
// follow jellyfin-web's language, as for the third. Any error just stops
// it.
//
// Anchors in jellyfin-web:
//   selector: .itemMiscInfo-primary
//   selector: .starRatingContainer
//   selector: .mediaInfoCriticRating
//   class: mediaInfoItem
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl

  // The title whose scores are asked for, its last answer holding one,
  // the next time they are asked, and the page watched for jellyfin-web's
  // renders of its first line of details.
  var rated = ''
  var scores = null
  var scoresTimer = null
  var scoresWatcher = null
  var scoresWatched = null
  var scoreWords = {
    en: { imdb: 'IMDb rating', critics: 'Critics', criticsTitle: 'Critics’ score', audience: 'Audience', audienceTitle: 'Audience score', percent: '%' },
    fr: { imdb: 'Note IMDb', critics: 'Critiques', criticsTitle: 'Score des critiques', audience: 'Public', audienceTitle: 'Score du public', percent: '\u00a0%' },
    de: { imdb: 'IMDb-Bewertung', critics: 'Kritiker', criticsTitle: 'Kritikerwertung', audience: 'Publikum', audienceTitle: 'Publikumswertung', percent: '\u00a0%' },
    es: { imdb: 'Valoración de IMDb', critics: 'Crítica', criticsTitle: 'Puntuación de la crítica', audience: 'Público', audienceTitle: 'Puntuación del público', percent: '\u00a0%' },
    it: { imdb: 'Voto IMDb', critics: 'Critica', criticsTitle: 'Punteggio della critica', audience: 'Pubblico', audienceTitle: 'Punteggio del pubblico', percent: '%' },
    pt: { imdb: 'Nota do IMDb', critics: 'Crítica', criticsTitle: 'Nota da crítica', audience: 'Público', audienceTitle: 'Nota do público', percent: '%' },
  }
  function sayScore(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (scoreWords[language] || scoreWords.en)[key]
  }
  // followScores asks for the scores of the title the route opens, and
  // gives back the page of the title it leaves.
  function followScores() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === rated) return
    rated = id
    scores = null
    clearTimeout(scoresTimer)
    scoresTimer = null
    if (scoresWatcher) scoresWatcher.disconnect()
    scoresWatcher = null
    scoresWatched = null
    unrate()
    if (id) askScores(id, 0)
  }
  // askScores asks for the title's scores: again 3 seconds later while
  // MDBList is asked about it (Pending), 10 times at most, and every second
  // until jellyfin-web made its client.
  function askScores(id, asked) {
    scoresTimer = null
    if (rated !== id || asked >= 10) return
    var again = function (delay) {
      scoresTimer = setTimeout(function () {
        try {
          askScores(id, asked + 1)
        } catch (error) {}
      }, delay)
    }
    var api = window.ApiClient
    if (!api) return again(1000)
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Ratings'))
      .then(function (answer) {
        if (rated !== id || !answer) return
        if (answer.CommunityRating != null || answer.CriticsScore != null || answer.AudienceScore != null) {
          scores = answer
          showScores()
        }
        if (answer.Pending) again(3000)
      })
      .catch(function () {})
  }
  // unrate gives jellyfin-web's own ratings back, wherever the script hid
  // them.
  function unrate() {
    var lines = document.querySelectorAll('.polyfinRated')
    for (var i = 0; i < lines.length; i++) {
      lines[i].classList.remove('polyfinRated')
      var added = lines[i].querySelector('.polyfinScores')
      if (added) added.parentNode.removeChild(added)
    }
  }
  // showScores puts the title's scores in the first line of details of the
  // title page shown, in place of jellyfin-web's own, and watches the page
  // so that its renders find them again, or the document until the page
  // shows. It never changes the page during a video.
  function showScores() {
    if (!scores) return
    var page = shownPage()
    var target = page || document.body
    if (target && scoresWatched !== target) {
      if (scoresWatcher) scoresWatcher.disconnect()
      scoresWatched = target
      scoresWatcher = new MutationObserver(function () {
        try {
          showScores()
        } catch (error) {}
      })
      scoresWatcher.observe(target, { childList: true, subtree: true })
    }
    if (!page) return
    var line = page.querySelector('.itemMiscInfo-primary')
    if (!line || line.querySelector('.polyfinScores')) return
    scoresStyle()
    var entries = []
    if (scores.CommunityRating != null) entries.push(['IMDb', Number(scores.CommunityRating).toFixed(1), sayScore('imdb')])
    if (scores.CriticsScore != null) {
      entries.push([sayScore('critics'), Math.round(scores.CriticsScore) + sayScore('percent'), sayScore('criticsTitle')])
    }
    if (scores.AudienceScore != null) {
      entries.push([sayScore('audience'), Math.round(scores.AudienceScore) + sayScore('percent'), sayScore('audienceTitle')])
    }
    var group = document.createElement('div')
    group.className = 'mediaInfoItem polyfinScores'
    entries.forEach(function (entry) {
      var score = document.createElement('span')
      score.className = 'polyfinScore'
      score.title = entry[2]
      var label = document.createElement('span')
      label.className = 'polyfinScoreLabel'
      label.textContent = entry[0]
      score.appendChild(label)
      score.appendChild(document.createTextNode(entry[1]))
      group.appendChild(score)
    })
    var own = line.querySelector('.starRatingContainer') || line.querySelector('.mediaInfoCriticRating')
    line.insertBefore(group, own)
    if (!line.classList.contains('polyfinRated')) line.classList.add('polyfinRated')
  }
  // scoresStyle adds the look of the scores, which hides jellyfin-web's own
  // in the same line, once.
  function scoresStyle() {
    if (document.getElementById('polyfinScoresStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinScoresStyle'
    sheet.textContent =
      '.polyfinRated .starRatingContainer,.polyfinRated .mediaInfoCriticRating{display:none!important}' +
      '.polyfinScores{display:inline-flex;flex-wrap:wrap;gap:.3em 1em}' +
      '.polyfinScoreLabel{font-weight:600;opacity:.75;margin-right:.35em}'
    document.head.appendChild(sheet)
  }
