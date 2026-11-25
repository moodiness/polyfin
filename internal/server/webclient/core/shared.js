// The core of the parts that run in one function, whose scope they share
// (see webScriptGroups, in internal/server/webclientminify_test.go): the
// helpers several of them call, the socket messages they hear, and the
// words of the third and the tenth. It runs first in that function, and
// core/start.js last, once every part has set itself up.
//
// Anchors in jellyfin-web:
//   route: details
//   selector: .videoPlayerContainer
//   selector: .page.itemDetailPage:not(.hide)
//   class: dialogContainer
//   global: ApiClient.subscribe
//   global: ApiClient.markPlayed
//   global: ApiClient.markUnplayed
//   global: ApiClient.sendSyncPlayPing
//   property: _callbacks

  // A title's page, as jellyfin-web routes it.
  var titleRoute = /^#!?\/details\?(?:[^#]*&)?id=([0-9a-f]{32})(?:[&#]|$)/i
  // Addresses are read and written by hand, never with URL: the script runs
  // before jellyfin-web's polyfills, and browsers jellyfin-web is built for
  // lack URL (before Chrome 32) or its searchParams (before Chrome 51).
  //
  // webFolder is the address of the web client's folder, /web/ or a folder
  // under Polyfin's base path, with its last slash.
  function webFolder() {
    var page = location.href.split('#')[0].split('?')[0]
    return page.slice(0, page.lastIndexOf('/') + 1)
  }
  // resolved is address as the page resolves it: one with a scheme as it
  // is, one from the server's root, such as /admin/…, on the page's server,
  // and any other in the web client's folder.
  function resolved(address) {
    if (/^[a-z][a-z0-9+.-]*:/i.test(address)) return address
    var root = /^([a-z][a-z0-9+.-]*:)\/\/[^/?#]*/i.exec(location.href)
    if (root && address.indexOf('//') === 0) return root[1] + address
    if (root && address.charAt(0) === '/') return root[0] + address
    return webFolder() + address
  }
  // adminAddress is the address of the admin app's page path ("" for its
  // home), which '../admin/' leads to from the web client's folder, never
  // above the server's root.
  function adminAddress(path) {
    var folder = webFolder()
    var root = /^[a-z][a-z0-9+.-]*:\/\/[^/]*\//i.exec(folder)
    var above = folder.replace(/[^/]+\/$/, '')
    return (root && above.length < root[0].length ? root[0] : above) + 'admin/' + path
  }
  // An address of the admin app, marked as opened from the web client
  // (?from=web): the admin app then shows the user signed in here, and
  // signs out a session of another user it had in this browser.
  function fromWebClient(address) {
    var at = address.indexOf('#')
    var rest = at < 0 ? address : address.slice(0, at)
    var join = rest.indexOf('?') < 0 ? '?' : /[?&]$/.test(rest) ? '' : '&'
    return rest + join + 'from=web' + (at < 0 ? '' : address.slice(at))
  }
  // marks is a WeakSet (kind "set") or a WeakMap ("map") of the page's
  // elements, or, before Chrome 36, which has neither, the same kept on
  // each element under a property named after name: either way, an element
  // takes what was kept for it when it goes.
  function marks(kind, name) {
    if (kind === 'set' && typeof WeakSet === 'function') return new WeakSet()
    if (kind === 'map' && typeof WeakMap === 'function') return new WeakMap()
    var key = '__polyfin' + name
    return {
      has: function (element) {
        return Object.prototype.hasOwnProperty.call(element, key)
      },
      get: function (element) {
        return Object.prototype.hasOwnProperty.call(element, key) ? element[key] : undefined
      },
      set: function (element, value) {
        element[key] = value
        return this
      },
      add: function (element) {
        element[key] = true
        return this
      },
    }
  }
  // attached tells whether node is in the page, as isConnected does from
  // Chrome 54 on.
  function attached(node) {
    if (typeof node.isConnected === 'boolean') return node.isConnected
    return document.documentElement.contains(node)
  }
  // shownPage returns the title page shown, unless a video is.
  function shownPage() {
    if (document.querySelector('.videoPlayerContainer')) return null
    return document.querySelector('.page.itemDetailPage:not(.hide)')
  }
  // The ApiClient whose messages the script hears.
  var heard = null
  // listen hears the messages of jellyfin-web's ApiClient, as its message
  // event gives them: jellyfin-web's events keep an object's listeners in
  // its _callbacks, by event, and call each with the event then the message.
  // That event only fires for the socket of jellyfin-apiclient itself,
  // which jellyfin-web opens no more since 12.0: its socket is its SDK's,
  // whose messages ApiClient.subscribe gives by type, any type
  // (ServerConnections.js). So UserDataChanged, PolyfinVersions,
  // SyncPlayGroupUpdate and PolyfinSyncPlayInvite are heard there when
  // jellyfin-web has it, through the message event otherwise.
  // It also hears the answers of the played marks the page sends, as
  // markPlayed and markUnplayed give them: what the user did with the item
  // afterwards. They come at once, before the socket's message. And it
  // hears jellyfin-web ping its SyncPlay group (sendSyncPlayPing), as it
  // does once its clock is set to the server's (see the tenth).
  function listen() {
    var api = window.ApiClient
    if (!api || api === heard) return
    heard = api
    function heardData(message) {
      try {
        if (message) playedChanged(message.Data)
      } catch (error) {}
    }
    function heardVersions(message) {
      try {
        hear(message)
      } catch (error) {}
    }
    function heardSyncPlay(message) {
      try {
        heardGroup(message)
      } catch (error) {}
    }
    function heardInvite(message) {
      try {
        invited(message)
      } catch (error) {}
    }
    if (typeof api.subscribe === 'function') {
      api.subscribe(['UserDataChanged'], heardData)
      api.subscribe(['PolyfinVersions'], heardVersions)
      api.subscribe(['SyncPlayGroupUpdate'], heardSyncPlay)
      api.subscribe(['PolyfinSyncPlayInvite'], heardInvite)
    } else {
      var callbacks = (api._callbacks = api._callbacks || {})
      ;(callbacks.message = callbacks.message || []).push(function (event, message) {
        if (message && message.MessageType === 'UserDataChanged') heardData(message)
        else if (message && message.MessageType === 'SyncPlayGroupUpdate') heardSyncPlay(message)
        else if (message && message.MessageType === 'PolyfinSyncPlayInvite') heardInvite(message)
        else heardVersions(message)
      })
    }
    ;['markPlayed', 'markUnplayed'].forEach(function (name) {
      var mark = api[name]
      if (typeof mark !== 'function') return
      api[name] = function () {
        var answer = mark.apply(this, arguments)
        if (answer && typeof answer.then === 'function') {
          answer.then(function (data) {
            try {
              if (data && data.ItemId) playedChanged({ UserDataList: [data] })
            } catch (error) {}
          }, function () {})
        }
        return answer
      }
    })
    var ping = api.sendSyncPlayPing
    if (typeof ping === 'function') {
      api.sendSyncPlayPing = function () {
        try {
          pinged()
        } catch (error) {}
        return ping.apply(this, arguments)
      }
    }
  }
  // The words the script adds to a page (those of 03-play-buttons.js and
  // 10-item-menus.js), in jellyfin-web's language, which it gives the
  // document; English for any other.
  var words = {
    en: {
      searching: 'Looking for sources…',
      none: 'No source is available for this title.',
      again: 'Try again',
      removeResume: 'Remove from Continue Watching',
      removeNextUp: 'Remove from Next Up',
      removedNextUp: 'This series is off Next Up until you play one of its episodes or a new one comes out.',
      hide: 'Hide this title',
      show: 'Show this title again',
      hidden: 'This title is hidden from your lists.',
      shown: 'This title is back in your lists.',
      addWatchlist: 'Add to Watchlist',
      removeWatchlist: 'Remove from Watchlist',
      addedWatchlist: 'This title is on your Watchlist.',
      removedWatchlist: 'This title is off your Watchlist.',
      watchTogether: 'Watch together',
      groupName: '{name}’s group',
      togetherStarted: 'Your SyncPlay group is ready, paused. The others join it on their own device, from the SyncPlay button (the people icon at the top). Press Play once they are in.',
      togetherOff: 'SyncPlay is off for your account.',
      togetherJoinOnly: 'Your account can join SyncPlay groups, but not start one.',
      togetherSearching: 'Sources for this title are still being looked for. Try again in a moment.',
      togetherRefused: 'Not everyone in your SyncPlay group can play this title.',
      togetherFailed: 'Watch together could not start. Try again.',
      inviteIntro: 'Your SyncPlay group is ready, paused. Choose whom to invite: a prompt to join shows in their web player. Press Play once they are in.',
      inviteMore: 'Choose whom to invite to your SyncPlay group: a prompt to join shows in their web player.',
      inviteSend: 'Invite',
      inviteSkip: 'Skip',
      inviteNobody: 'Nobody else can be invited to this title right now.',
      invitedSent: 'Invitation sent to {names}. Press Play once they are in.',
      invitedMore: 'Invitation sent to {names}.',
      invitedSoon: 'Invited less than a minute ago already: {names}.',
      inviteFailed: 'The invitation could not be sent. Try again.',
      invitePrompt: '{host} invites you to watch together.',
      inviteJoin: 'Join',
      inviteLater: 'Not now',
      joinFailed: 'The group could not be joined. Try again from the SyncPlay button (the people icon at the top).',
    },
    fr: {
      searching: 'Recherche des sources…',
      none: 'Aucune source disponible pour ce titre.',
      again: 'Réessayer',
      removeResume: 'Retirer de «\u202fContinuer de regarder\u202f»',
      removeNextUp: 'Retirer de «\u202fÀ suivre\u202f»',
      removedNextUp: 'Cette série n’est plus dans «\u202fÀ suivre\u202f» jusqu’à ce que vous en lisiez un épisode ou qu’un nouvel épisode sorte.',
      hide: 'Masquer ce titre',
      show: 'Afficher de nouveau ce titre',
      hidden: 'Ce titre est masqué de vos listes.',
      shown: 'Ce titre est de retour dans vos listes.',
      addWatchlist: 'Ajouter à «\u202fÀ regarder\u202f»',
      removeWatchlist: 'Retirer de «\u202fÀ regarder\u202f»',
      addedWatchlist: 'Ce titre est dans «\u202fÀ regarder\u202f».',
      removedWatchlist: 'Ce titre n’est plus dans «\u202fÀ regarder\u202f».',
      watchTogether: 'Regarder ensemble',
      groupName: 'Groupe de {name}',
      togetherStarted: 'Votre groupe SyncPlay est prêt, en pause. Les autres le rejoignent sur leur propre appareil, depuis le bouton SyncPlay (l’icône de personnes en haut). Appuyez sur Lire une fois qu’ils sont là.',
      togetherOff: 'SyncPlay est désactivé pour votre compte.',
      togetherJoinOnly: 'Votre compte peut rejoindre des groupes SyncPlay, mais pas en créer.',
      togetherSearching: 'Les sources de ce titre sont encore recherchées. Réessayez dans un instant.',
      togetherRefused: 'Certains membres de votre groupe SyncPlay ne peuvent pas lire ce titre.',
      togetherFailed: 'Impossible de lancer «\u202fRegarder ensemble\u202f». Réessayez.',
      inviteIntro:
        'Votre groupe SyncPlay est prêt, en pause. Choisissez qui inviter\u202f: une invitation à le rejoindre s’affiche dans leur lecteur web. Appuyez sur Lire une fois qu’ils sont là.',
      inviteMore: 'Choisissez qui inviter dans votre groupe SyncPlay\u202f: une invitation à le rejoindre s’affiche dans leur lecteur web.',
      inviteSend: 'Inviter',
      inviteSkip: 'Passer',
      inviteNobody: 'Personne d’autre ne peut être invité à ce titre pour le moment.',
      invitedSent: 'Invitation envoyée à {names}. Appuyez sur Lire une fois qu’ils sont là.',
      invitedMore: 'Invitation envoyée à {names}.',
      invitedSoon: 'Déjà invités il y a moins d’une minute\u202f: {names}.',
      inviteFailed: 'L’invitation n’a pas pu être envoyée. Réessayez.',
      invitePrompt: '{host} vous invite à regarder ensemble.',
      inviteJoin: 'Rejoindre',
      inviteLater: 'Pas maintenant',
      joinFailed: 'Impossible de rejoindre le groupe. Réessayez depuis le bouton SyncPlay (l’icône de personnes en haut).',
    },
    de: {
      searching: 'Quellen werden gesucht…',
      none: 'Für diesen Titel ist keine Quelle verfügbar.',
      again: 'Erneut versuchen',
      removeResume: 'Aus „Weiterschauen“ entfernen',
      removeNextUp: 'Aus „Als Nächstes“ entfernen',
      removedNextUp: 'Diese Serie ist nicht mehr in „Als Nächstes“, bis du eine ihrer Folgen abspielst oder eine neue erscheint.',
      hide: 'Diesen Titel ausblenden',
      show: 'Diesen Titel wieder einblenden',
      hidden: 'Dieser Titel ist in deinen Listen ausgeblendet.',
      shown: 'Dieser Titel ist wieder in deinen Listen.',
      addWatchlist: 'Zur Merkliste hinzufügen',
      removeWatchlist: 'Von der Merkliste entfernen',
      addedWatchlist: 'Dieser Titel ist auf deiner Merkliste.',
      removedWatchlist: 'Dieser Titel ist nicht mehr auf deiner Merkliste.',
      watchTogether: 'Gemeinsam ansehen',
      groupName: 'Gruppe von {name}',
      togetherStarted: 'Deine SyncPlay-Gruppe ist bereit und pausiert. Die anderen treten ihr auf ihrem eigenen Gerät bei, über die Schaltfläche Synchronwiedergabe (das Personen-Symbol oben). Drücke auf Abspielen, sobald alle da sind.',
      togetherOff: 'SyncPlay ist für dein Konto ausgeschaltet.',
      togetherJoinOnly: 'Dein Konto kann SyncPlay-Gruppen beitreten, aber keine starten.',
      togetherSearching: 'Für diesen Titel werden noch Quellen gesucht. Versuche es gleich noch einmal.',
      togetherRefused: 'Nicht alle in deiner SyncPlay-Gruppe können diesen Titel abspielen.',
      togetherFailed: '„Gemeinsam ansehen“ konnte nicht starten. Versuche es erneut.',
      inviteIntro:
        'Deine SyncPlay-Gruppe ist bereit und pausiert. Wähle aus, wen du einladen möchtest: Im Web-Player der Eingeladenen erscheint eine Einladung zum Beitreten. Drücke auf Abspielen, sobald alle da sind.',
      inviteMore: 'Wähle aus, wen du in deine SyncPlay-Gruppe einladen möchtest: Im Web-Player der Eingeladenen erscheint eine Einladung zum Beitreten.',
      inviteSend: 'Einladen',
      inviteSkip: 'Überspringen',
      inviteNobody: 'Gerade kann niemand sonst zu diesem Titel eingeladen werden.',
      invitedSent: 'Einladung an {names} gesendet. Drücke auf Abspielen, sobald alle da sind.',
      invitedMore: 'Einladung an {names} gesendet.',
      invitedSoon: 'Schon vor weniger als einer Minute eingeladen: {names}.',
      inviteFailed: 'Die Einladung konnte nicht gesendet werden. Versuche es erneut.',
      invitePrompt: '{host} lädt dich zum gemeinsamen Ansehen ein.',
      inviteJoin: 'Beitreten',
      inviteLater: 'Nicht jetzt',
      joinFailed: 'Der Gruppe konnte nicht beigetreten werden. Versuche es erneut über die Schaltfläche Synchronwiedergabe (das Personen-Symbol oben).',
    },
    es: {
      searching: 'Buscando fuentes…',
      none: 'No hay ninguna fuente disponible para este título.',
      again: 'Reintentar',
      removeResume: 'Quitar de «Seguir viendo»',
      removeNextUp: 'Quitar de «A continuación»',
      removedNextUp: 'Esta serie ya no está en «A continuación» hasta que reproduzca uno de sus episodios o salga uno nuevo.',
      hide: 'Ocultar este título',
      show: 'Volver a mostrar este título',
      hidden: 'Este título está oculto en sus listas.',
      shown: 'Este título vuelve a estar en sus listas.',
      addWatchlist: 'Añadir a «Para ver»',
      removeWatchlist: 'Quitar de «Para ver»',
      addedWatchlist: 'Este título está en «Para ver».',
      removedWatchlist: 'Este título ya no está en «Para ver».',
      watchTogether: 'Ver juntos',
      groupName: 'Grupo de {name}',
      togetherStarted: 'Su grupo de SyncPlay está listo, en pausa. Los demás se unen desde su propio dispositivo, con el botón Reproducción sincronizada (el icono de personas de arriba). Pulse Reproducir cuando estén todos.',
      togetherOff: 'SyncPlay está desactivado para su cuenta.',
      togetherJoinOnly: 'Su cuenta puede unirse a grupos de SyncPlay, pero no crearlos.',
      togetherSearching: 'Todavía se están buscando fuentes para este título. Vuelva a intentarlo en un momento.',
      togetherRefused: 'No todos en su grupo de SyncPlay pueden reproducir este título.',
      togetherFailed: '«Ver juntos» no pudo empezar. Vuelva a intentarlo.',
      inviteIntro:
        'Su grupo de SyncPlay está listo, en pausa. Elija a quién invitar: verán una invitación para unirse en su reproductor web. Pulse Reproducir cuando estén todos.',
      inviteMore: 'Elija a quién invitar a su grupo de SyncPlay: verán una invitación para unirse en su reproductor web.',
      inviteSend: 'Invitar',
      inviteSkip: 'Omitir',
      inviteNobody: 'Ahora mismo no se puede invitar a nadie más a este título.',
      invitedSent: 'Invitación enviada a {names}. Pulse Reproducir cuando estén todos.',
      invitedMore: 'Invitación enviada a {names}.',
      invitedSoon: 'Ya invitados hace menos de un minuto: {names}.',
      inviteFailed: 'No se pudo enviar la invitación. Vuelva a intentarlo.',
      invitePrompt: '{host} le invita a ver juntos.',
      inviteJoin: 'Unirse',
      inviteLater: 'Ahora no',
      joinFailed: 'No se pudo unir al grupo. Vuelva a intentarlo desde el botón Reproducción sincronizada (el icono de personas de arriba).',
    },
    it: {
      searching: 'Ricerca delle origini…',
      none: 'Nessuna origine disponibile per questo titolo.',
      again: 'Riprova',
      removeResume: 'Rimuovi da «Continua a guardare»',
      removeNextUp: 'Rimuovi da «Prossimo»',
      removedNextUp: 'Questa serie non è più in «Prossimo» finché non ne riproduci un episodio o ne esce uno nuovo.',
      hide: 'Nascondi questo titolo',
      show: 'Mostra di nuovo questo titolo',
      hidden: 'Questo titolo è nascosto dai tuoi elenchi.',
      shown: 'Questo titolo è di nuovo nei tuoi elenchi.',
      addWatchlist: 'Aggiungi a «Da guardare»',
      removeWatchlist: 'Rimuovi da «Da guardare»',
      addedWatchlist: 'Questo titolo è in «Da guardare».',
      removedWatchlist: 'Questo titolo non è più in «Da guardare».',
      watchTogether: 'Guarda insieme',
      groupName: 'Gruppo di {name}',
      togetherStarted: 'Il tuo gruppo SyncPlay è pronto, in pausa. Gli altri si uniscono dal proprio dispositivo, con il pulsante Sincronizza Riproduzione (l’icona delle persone in alto). Premi Riproduci quando ci sono tutti.',
      togetherOff: 'SyncPlay è disattivato per il tuo account.',
      togetherJoinOnly: 'Il tuo account può unirsi ai gruppi SyncPlay, ma non crearne.',
      togetherSearching: 'Le origini di questo titolo sono ancora in ricerca. Riprova tra un momento.',
      togetherRefused: 'Non tutti nel tuo gruppo SyncPlay possono riprodurre questo titolo.',
      togetherFailed: '«Guarda insieme» non è riuscito ad avviarsi. Riprova.',
      inviteIntro:
        'Il tuo gruppo SyncPlay è pronto, in pausa. Scegli chi invitare: nel loro lettore web compare un invito a unirsi. Premi Riproduci quando ci sono tutti.',
      inviteMore: 'Scegli chi invitare nel tuo gruppo SyncPlay: nel loro lettore web compare un invito a unirsi.',
      inviteSend: 'Invita',
      inviteSkip: 'Salta',
      inviteNobody: 'Al momento nessun altro può essere invitato a questo titolo.',
      invitedSent: 'Invito inviato a {names}. Premi Riproduci quando ci sono tutti.',
      invitedMore: 'Invito inviato a {names}.',
      invitedSoon: 'Già invitati meno di un minuto fa: {names}.',
      inviteFailed: 'Impossibile inviare l’invito. Riprova.',
      invitePrompt: '{host} ti invita a guardare insieme.',
      inviteJoin: 'Unisciti',
      inviteLater: 'Non ora',
      joinFailed: 'Impossibile unirsi al gruppo. Riprova dal pulsante Sincronizza Riproduzione (l’icona delle persone in alto).',
    },
    pt: {
      searching: 'Procurando fontes…',
      none: 'Nenhuma fonte disponível para este título.',
      again: 'Tentar novamente',
      removeResume: 'Remover de “Continuar assistindo”',
      removeNextUp: 'Remover de “A seguir”',
      removedNextUp: 'Esta série saiu de “A seguir” até você reproduzir um dos episódios dela ou sair um novo.',
      hide: 'Ocultar este título',
      show: 'Mostrar este título novamente',
      hidden: 'Este título está oculto nas suas listas.',
      shown: 'Este título voltou para as suas listas.',
      addWatchlist: 'Adicionar a “Para assistir”',
      removeWatchlist: 'Remover de “Para assistir”',
      addedWatchlist: 'Este título está em “Para assistir”.',
      removedWatchlist: 'Este título saiu de “Para assistir”.',
      watchTogether: 'Assistir juntos',
      groupName: 'Grupo de {name}',
      togetherStarted: 'Seu grupo do SyncPlay está pronto, em pausa. Os outros entram pelo próprio dispositivo, no botão Reprodução sincronizada (o ícone de pessoas no topo). Toque em Reproduzir quando todos estiverem lá.',
      togetherOff: 'O SyncPlay está desativado para a sua conta.',
      togetherJoinOnly: 'Sua conta pode entrar em grupos do SyncPlay, mas não criá-los.',
      togetherSearching: 'Ainda estamos procurando fontes para este título. Tente novamente em instantes.',
      togetherRefused: 'Nem todos no seu grupo do SyncPlay podem reproduzir este título.',
      togetherFailed: '“Assistir juntos” não conseguiu começar. Tente novamente.',
      inviteIntro:
        'Seu grupo do SyncPlay está pronto, em pausa. Escolha quem convidar: um convite para entrar aparece no player web dessas pessoas. Toque em Reproduzir quando todos estiverem lá.',
      inviteMore: 'Escolha quem convidar para o seu grupo do SyncPlay: um convite para entrar aparece no player web dessas pessoas.',
      inviteSend: 'Convidar',
      inviteSkip: 'Pular',
      inviteNobody: 'Ninguém mais pode ser convidado para este título agora.',
      invitedSent: 'Convite enviado para {names}. Toque em Reproduzir quando todos estiverem lá.',
      invitedMore: 'Convite enviado para {names}.',
      invitedSoon: 'Já convidados há menos de um minuto: {names}.',
      inviteFailed: 'Não foi possível enviar o convite. Tente novamente.',
      invitePrompt: '{host} convida você para assistir juntos.',
      inviteJoin: 'Entrar',
      inviteLater: 'Agora não',
      joinFailed: 'Não foi possível entrar no grupo. Tente novamente pelo botão Reprodução sincronizada (o ícone de pessoas no topo).',
    },
  }
  function say(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (words[language] || words.en)[key]
  }
  // quietly runs a listener, an error leaving the event to jellyfin-web.
  function quietly(listener) {
    return function (event) {
      try {
        listener(event)
      } catch (error) {}
    }
  }
  // element makes an element with classes and a text or children.
  function element(tag, className, content) {
    var made = document.createElement(tag)
    made.className = className
    if (typeof content === 'string') made.textContent = content
    else content.forEach(made.appendChild, made)
    return made
  }
  // Alerts and item menus are dialog containers jellyfin-web adds to the
  // body, which the script, loaded in the head, observes once it exists.
  function watchDialogs() {
    if (typeof MutationObserver !== 'function' || !document.body) return
    new MutationObserver(
      quietly(function (records) {
        for (var i = 0; i < records.length; i++) {
          var added = records[i].addedNodes || []
          for (var j = 0; j < added.length; j++) {
            if (added[j].classList && added[j].classList.contains('dialogContainer')) {
              // Each on its own: one failing leaves the other to run.
              quietly(closeDuplicate)(added[j])
              quietly(extendMenu)(added[j])
            }
          }
        }
      })
    ).observe(document.body, { childList: true })
  }
