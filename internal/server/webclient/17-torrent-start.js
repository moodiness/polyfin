// Part 17: how a torrent starts, under the spinner.
//
// While jellyfin-web's loading spinner shows (its .docspinner,
// .mdlSpinnerActive while it turns), it writes under it how the torrent the
// play waits for starts: "Finding peers: 12 found, 3 connected", then
// "Receiving the start: 2.4 MB from 3 peers", or why it did not start. It
// asks /Polyfin/Playback/Starting as the spinner shows, then every second
// after each answer, and stops as the spinner hides, the line going with
// it. Polyfin answers for the play of this browser's own session, the
// torrents its PlaybackInfo tries, then the one it chose while the player
// reads its first segments, and an empty Stage for anything else, which
// shows no line: a play of a link, a page loading. jellyfin-web's spinner
// takes no text (loading.ts), and the Jellyfin API tells a play nothing
// before it starts. Its words follow jellyfin-web's language, as the
// third's. Any error just stops it.
//
// Anchors in jellyfin-web:
//   selector: .docspinner.mdlSpinnerActive
//   global: ApiClient.getCurrentUserId
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl

  var startWords = {
    en: {
      looking: 'Finding peers…',
      finding: 'Finding peers: {found} found, {connected} connected',
      receiving: 'Receiving the start: {mb} MB from {peers} peers',
      receivingOne: 'Receiving the start: {mb} MB from 1 peer',
      failed: 'The torrent did not start: {why}',
      silent: 'its peers sent nothing',
      noUdp: 'no peer found, and the route carries no UDP',
      refused: 'its trackers did not answer',
      noTrackers: 'the addon named no tracker, and none was found otherwise',
      noPeers: 'no peer found',
      route: 'the route for torrents is down',
      atCap: 'as many torrents play as allowed',
      infoRefused: 'its list of files is refused',
    },
    fr: {
      looking: 'Recherche de pairs…',
      finding: 'Recherche de pairs\u202f: {found} trouvés, {connected} connectés',
      receiving: 'Réception du début\u202f: {mb} Mo depuis {peers} pairs',
      receivingOne: 'Réception du début\u202f: {mb} Mo depuis 1 pair',
      failed: 'Le torrent n’a pas démarré\u202f: {why}',
      silent: 'ses pairs n’ont rien envoyé',
      noUdp: 'aucun pair trouvé, et la route ne transporte pas l’UDP',
      refused: 'ses trackers n’ont pas répondu',
      noTrackers: 'l’addon n’a indiqué aucun tracker, et aucun pair n’a été trouvé autrement',
      noPeers: 'aucun pair trouvé',
      route: 'la route des torrents est coupée',
      atCap: 'autant de torrents sont lus que permis',
      infoRefused: 'sa liste de fichiers est refusée',
    },
    de: {
      looking: 'Suche nach Peers…',
      finding: 'Suche nach Peers: {found} gefunden, {connected} verbunden',
      receiving: 'Empfange den Anfang: {mb} MB von {peers} Peers',
      receivingOne: 'Empfange den Anfang: {mb} MB von 1 Peer',
      failed: 'Der Torrent ist nicht gestartet: {why}',
      silent: 'seine Peers haben nichts gesendet',
      noUdp: 'kein Peer gefunden, und die Route überträgt kein UDP',
      refused: 'seine Tracker haben nicht geantwortet',
      noTrackers: 'das Add-on hat keinen Tracker genannt, und anders wurde kein Peer gefunden',
      noPeers: 'kein Peer gefunden',
      route: 'die Route für Torrents ist unterbrochen',
      atCap: 'es laufen so viele Torrents wie erlaubt',
      infoRefused: 'seine Dateiliste wird abgelehnt',
    },
    es: {
      looking: 'Buscando pares…',
      finding: 'Buscando pares: {found} encontrados, {connected} conectados',
      receiving: 'Recibiendo el inicio: {mb} MB de {peers} pares',
      receivingOne: 'Recibiendo el inicio: {mb} MB de 1 par',
      failed: 'El torrent no se inició: {why}',
      silent: 'sus pares no enviaron nada',
      noUdp: 'no se encontró ningún par, y la ruta no transporta UDP',
      refused: 'sus trackers no respondieron',
      noTrackers: 'la extensión no indicó ningún tracker, y no se encontró ningún par de otro modo',
      noPeers: 'no se encontró ningún par',
      route: 'la ruta de los torrents está caída',
      atCap: 'se reproducen tantos torrents como se permite',
      infoRefused: 'su lista de archivos es rechazada',
    },
    it: {
      looking: 'Ricerca dei peer…',
      finding: 'Ricerca dei peer: {found} trovati, {connected} connessi',
      receiving: 'Ricezione dell’inizio: {mb} MB da {peers} peer',
      receivingOne: 'Ricezione dell’inizio: {mb} MB da 1 peer',
      failed: 'Il torrent non è partito: {why}',
      silent: 'i suoi peer non hanno inviato nulla',
      noUdp: 'nessun peer trovato, e il percorso non trasporta UDP',
      refused: 'i suoi tracker non hanno risposto',
      noTrackers: 'l’add-on non ha indicato nessun tracker, e non è stato trovato nessun peer in altro modo',
      noPeers: 'nessun peer trovato',
      route: 'il percorso dei torrent è interrotto',
      atCap: 'sono in riproduzione tanti torrent quanti consentiti',
      infoRefused: 'il suo elenco di file è rifiutato',
    },
    pt: {
      looking: 'Procurando peers…',
      finding: 'Procurando peers: {found} encontrados, {connected} conectados',
      receiving: 'Recebendo o início: {mb} MB de {peers} peers',
      receivingOne: 'Recebendo o início: {mb} MB de 1 peer',
      failed: 'O torrent não começou: {why}',
      silent: 'seus peers não enviaram nada',
      noUdp: 'nenhum peer encontrado, e a rota não transporta UDP',
      refused: 'seus trackers não responderam',
      noTrackers: 'o complemento não indicou nenhum tracker, e nenhum peer foi encontrado de outro modo',
      noPeers: 'nenhum peer encontrado',
      route: 'a rota dos torrents está fora do ar',
      atCap: 'há tantos torrents em reprodução quanto o permitido',
      infoRefused: 'sua lista de arquivos é recusada',
    },
  }
  // The words of each failure Polyfin tells.
  var failures = { silent: 'silent', no_udp: 'noUdp', refused: 'refused', no_trackers: 'noTrackers', no_peers: 'noPeers', route: 'route', at_cap: 'atCap', info_refused: 'infoRefused' }
  function language() {
    return /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
  }
  function say(key) {
    return (startWords[language()] || startWords.en)[key]
  }
  function quietly(listener) {
    return function () {
      try {
        listener.apply(this, arguments)
      } catch (error) {}
    }
  }
  // The spinner watched, the round of asks while it shows (0 while it does
  // not), the ask to come, and the line drawn.
  var watched = null
  var rounds = 0
  var polling = 0
  var timer = null
  var line = null
  function spinning() {
    return !!document.querySelector('.docspinner.mdlSpinnerActive')
  }
  // watch follows jellyfin-web's spinner, which it makes at its first
  // show, then turns on and off by its class.
  function watch() {
    var loader = document.querySelector('.docspinner')
    if (loader && loader !== watched) {
      watched = loader
      new MutationObserver(quietly(check)).observe(loader, { attributes: true, attributeFilter: ['class'] })
    }
    check()
  }
  function check() {
    if (spinning()) {
      if (!polling) {
        polling = ++rounds
        ask(polling)
      }
      return
    }
    if (!polling) return
    polling = 0
    clearTimeout(timer)
    timer = null
    undraw()
  }
  function ask(round) {
    timer = null
    if (round !== polling) return
    var api = window.ApiClient
    if (!api || !api.getCurrentUserId()) return again(round)
    api
      .getJSON(api.getUrl('Polyfin/Playback/Starting'))
      .then(
        quietly(function (progress) {
          if (round !== polling) return
          draw(describe(progress))
          again(round)
        }),
        quietly(function () {
          if (round === polling) again(round)
        })
      )
      .catch(function () {})
  }
  function again(round) {
    timer = setTimeout(
      quietly(function () {
        ask(round)
      }),
      1000
    )
  }
  // describe words how the start goes, empty when nothing starts.
  function describe(progress) {
    if (!progress || !progress.Stage) return ''
    if (progress.Stage === 'failed') return say('failed').replace('{why}', say(failures[progress.Failure] || 'noPeers'))
    if (progress.Stage === 'pieces') {
      return say(progress.PeersConnected === 1 ? 'receivingOne' : 'receiving')
        .replace('{mb}', megabytes(progress.Received))
        .replace('{peers}', String(progress.PeersConnected))
    }
    if (!progress.PeersFound) return say('looking')
    return say('finding').replace('{found}', String(progress.PeersFound)).replace('{connected}', String(progress.PeersConnected))
  }
  // megabytes writes bytes in megabytes, with one decimal, as the
  // language writes numbers.
  function megabytes(bytes) {
    var lang = String(document.documentElement.lang || 'en')
    try {
      return (bytes / 1e6).toLocaleString(lang, { minimumFractionDigits: 1, maximumFractionDigits: 1 })
    } catch (error) {
      return (bytes / 1e6).toFixed(1)
    }
  }
  function draw(text) {
    if (!text) return undraw()
    style()
    if (!line) {
      line = document.createElement('div')
      line.className = 'polyfinTorrentStart'
      line.setAttribute('role', 'status')
      line.setAttribute('aria-live', 'polite')
    }
    if (line.textContent !== text) line.textContent = text
    if (!line.parentNode) document.body.appendChild(line)
    place()
  }
  // place puts the line under the spinner where it shows: jellyfin-web
  // centres it in the window, a web theme may move it, as the default
  // theme does under its header.
  function place() {
    var loader = document.querySelector('.docspinner.mdlSpinnerActive')
    var box = loader && loader.getBoundingClientRect && loader.getBoundingClientRect()
    if (!box || !box.height || !line.style) return
    line.style.top = Math.round(box.bottom + 12) + 'px'
    line.style.left = Math.round(box.left + box.width / 2) + 'px'
  }
  function undraw() {
    if (line && line.parentNode) line.parentNode.removeChild(line)
  }
  // style places the line under the spinner as jellyfin-web places it,
  // centred in the window, 10vh wide and tall, above everything, until
  // place finds where it shows.
  function style() {
    if (document.getElementById('polyfinTorrentStartStyle')) return
    var sheet = document.createElement('style')
    sheet.id = 'polyfinTorrentStartStyle'
    sheet.textContent =
      '.polyfinTorrentStart{position:fixed;left:50%;top:calc(50% + 7vh);transform:translateX(-50%);z-index:9999999;' +
      'padding:.35em .9em;border-radius:1em;background:rgba(0,0,0,.65);color:#fff;font-size:1em;line-height:1.4;' +
      'text-align:center;white-space:nowrap;pointer-events:none}'
    document.head.appendChild(sheet)
  }
  // The script runs from the page's head: the body, where jellyfin-web
  // puts its spinner, comes later.
  function start() {
    new MutationObserver(quietly(watch)).observe(document.body, { childList: true })
    watch()
  }
  if (document.body) quietly(start)()
  else addEventListener('DOMContentLoaded', quietly(start))
