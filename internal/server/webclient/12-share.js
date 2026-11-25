// Part 12: Share on a title's page.
//
// On a movie's or an episode's page, it adds a "Share" button to
// the page's buttons (.mainDetailButtons), before its ⋮ button
// (.btnMoreCommands), drawn as jellyfin-web draws them, for a user who may
// give the title through a share link. As the route opens a title, it asks
// /Polyfin/Items/{id}/Share, which answers Shareable true when the server
// allows share links (Settings › Security › Allow share links), the user
// may share titles, and the title is a movie or an episode they see, with
// Url, the admin app's My account › Shared links with the title chosen.
// The button opens that address in the same tab, where the admin app signs
// the user in with the web client's token, as for the eighth. It asks every
// second until jellyfin-web made its client, 10 times at most. A
// MutationObserver on the page puts the button back as jellyfin-web renders
// the page anew; nothing changes during a video, and the button goes as the
// route leaves the title. Its word follows jellyfin-web's language, as for
// the third. Any error just stops it.
//
// Anchors in jellyfin-web:
//   selector: .mainDetailButtons
//   selector: .btnMoreCommands
//   class: material-icons
//   class: detailButton-icon
//   class: button-flat
//   class: detailButton
//   class: emby-button
//   class: detailButton-content
//   global: ApiClient.getJSON
//   global: ApiClient.getUrl

  // The title whose Share is asked about (see the header), the address its
  // button opens once Polyfin says it may be shared, the next ask while
  // jellyfin-web has no client, and the page watched for jellyfin-web's
  // renders of its buttons.
  var shareTitle = ''
  var shareAddress = ''
  var shareTimer = null
  var shareWatcher = null
  var shareWatched = null
  var shareWords = { en: 'Share', fr: 'Partager', de: 'Teilen', es: 'Compartir', it: 'Condividi', pt: 'Compartilhar' }
  // followShare asks whether the title the route opens may be shared, and
  // takes the button off the page of the title it leaves.
  function followShare() {
    var match = titleRoute.exec(location.hash)
    var id = match ? match[1].toLowerCase() : ''
    if (id === shareTitle) return
    shareTitle = id
    shareAddress = ''
    clearTimeout(shareTimer)
    shareTimer = null
    if (shareWatcher) shareWatcher.disconnect()
    shareWatcher = null
    shareWatched = null
    var added = document.querySelectorAll('.polyfinShare')
    for (var i = 0; i < added.length; i++) added[i].parentNode.removeChild(added[i])
    if (id) askShare(id, 1)
  }
  // askShare asks Polyfin whether the title may be shared: every second
  // until jellyfin-web made its client, 10 times at most.
  function askShare(id, asked) {
    shareTimer = null
    if (shareTitle !== id) return
    var api = window.ApiClient
    if (!api) {
      if (asked < 10) {
        shareTimer = setTimeout(function () {
          try {
            askShare(id, asked + 1)
          } catch (error) {}
        }, 1000)
      }
      return
    }
    api
      .getJSON(api.getUrl('Polyfin/Items/' + id + '/Share'))
      .then(function (answer) {
        if (shareTitle !== id || !answer || answer.Shareable !== true || typeof answer.Url !== 'string') return
        shareAddress = fromWebClient(resolved(answer.Url))
        showShare()
      })
      .catch(function () {})
  }
  // showShare puts the button among the buttons of the title page shown,
  // and watches the page so that its renders find it again, or the
  // document until the page shows. It never changes the page during a
  // video.
  function showShare() {
    if (!shareAddress) return
    var page = shownPage()
    var target = page || document.body
    if (target && shareWatched !== target) {
      if (shareWatcher) shareWatcher.disconnect()
      shareWatched = target
      shareWatcher = new MutationObserver(function () {
        try {
          showShare()
        } catch (error) {}
      })
      shareWatcher.observe(target, { childList: true, subtree: true })
    }
    if (!page) return
    var buttons = page.querySelector('.mainDetailButtons')
    if (!buttons || buttons.querySelector('.polyfinShare')) return
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    var label = shareWords[language] || shareWords.en
    var icon = element('span', 'material-icons detailButton-icon share', '')
    icon.setAttribute('aria-hidden', 'true')
    var button = element('button', 'button-flat detailButton emby-button polyfinShare', [
      element('div', 'detailButton-content', [icon]),
    ])
    button.setAttribute('type', 'button')
    button.setAttribute('aria-label', label)
    button.title = label
    var address = shareAddress
    button.addEventListener('click', function () {
      location.href = address
    })
    var more = buttons.querySelector('.btnMoreCommands')
    buttons.insertBefore(button, more && more.parentNode === buttons ? more : null)
  }
