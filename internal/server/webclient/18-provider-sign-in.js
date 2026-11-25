// Part 18: the identity provider's sign-in.
//
// On jellyfin-web's sign-in page (#/login, its #loginPage), it
// offers to sign in with the identity provider the administrator set up
// (Settings › Security › Identity provider), as /admin/api/status names it:
// a "Sign in with …" button first among the page's buttons
// (.readOnlyContent), drawn as jellyfin-web's own, which opens the
// provider's sign-in at the server's public address (the provider's start,
// with ?app=web). The provider sends the browser back to /web/ with
// ?polyfinProvider=signed-in, and a ticket in a cookie the script cannot
// read. The script takes the parameter out of the address at once, waits
// for jellyfin-web to draw its first page, by which time it has saved its
// own connection to the server, then trades the ticket at
// /admin/api/provider/web, as jellyfin-web's device (ApiClient's app,
// device name, device ID and version, else those jellyfin-web keeps in
// the browser), for a Jellyfin sign-in of that device. It keeps the
// sign-in where jellyfin-web reads it (jellyfin_credentials), in this
// server's entry, as the admin app's sign-in to the web client does, and
// opens /web/ again, signed in. Any other value of the parameter, and a
// trade refused, is what went wrong, said under the button. It looks for
// the page as the route changes, for 5 seconds. Its words follow
// jellyfin-web's language, as the third's. Without fetch or storage it
// does nothing, and any error just stops it.
//
// Anchors in jellyfin-web:
//   selector: .page
//   selector: #loginPage .readOnlyContent
//   class: raised
//   class: block
//   class: emby-button
//   class: fieldDescription
//   global: ApiClient.deviceId
//   global: ApiClient.appName
//   global: ApiClient.deviceName
//   global: ApiClient.appVersion
//   storage: jellyfin_credentials
//   storage: _deviceId2

  var providerWords = {
    en: {
      signIn: 'Sign in with {0}',
      signingIn: 'Signing in…',
      refused: 'The sign-in was cancelled.',
      noAccount: 'No account on this server is linked to your account at the identity provider. If you have an account here, sign in with its password and link it under My account, in the admin app. Otherwise, ask an administrator.',
      disabled: 'This account is disabled.',
      hours: 'This account may not sign in at this time.',
      remote: 'This account may only sign in from the local network.',
      tooMany: 'Too many attempts. Try again later.',
      failed: 'The sign-in with the identity provider failed. Try again.',
    },
    fr: {
      signIn: 'Se connecter avec {0}',
      signingIn: 'Connexion…',
      refused: 'La connexion a été annulée.',
      noAccount: 'Aucun compte de ce serveur n’est lié à votre compte chez le fournisseur d’identité. Si vous avez un compte ici, connectez-vous avec son mot de passe et liez-le dans Mon compte, dans l’appli d’administration. Sinon, demandez à un administrateur.',
      disabled: 'Ce compte est désactivé.',
      hours: 'Ce compte ne peut pas se connecter à cette heure.',
      remote: 'Ce compte ne peut se connecter que depuis le réseau local.',
      tooMany: 'Trop de tentatives. Réessayez plus tard.',
      failed: 'La connexion avec le fournisseur d’identité a échoué. Réessayez.',
    },
    de: {
      signIn: 'Mit {0} anmelden',
      signingIn: 'Anmeldung…',
      refused: 'Die Anmeldung wurde abgebrochen.',
      noAccount: 'Kein Konto dieses Servers ist mit deinem Konto beim Identitätsanbieter verknüpft. Wenn du hier ein Konto hast, melde dich mit dessen Passwort an und verknüpfe es unter Mein Konto in der Admin-App. Wende dich sonst an einen Administrator.',
      disabled: 'Dieses Konto ist deaktiviert.',
      hours: 'Dieses Konto darf sich zu dieser Zeit nicht anmelden.',
      remote: 'Dieses Konto darf sich nur im lokalen Netzwerk anmelden.',
      tooMany: 'Zu viele Versuche. Versuche es später erneut.',
      failed: 'Die Anmeldung beim Identitätsanbieter ist fehlgeschlagen. Versuche es erneut.',
    },
    es: {
      signIn: 'Iniciar sesión con {0}',
      signingIn: 'Iniciando sesión…',
      refused: 'Se canceló el inicio de sesión.',
      noAccount: 'Ninguna cuenta de este servidor está vinculada a su cuenta del proveedor de identidad. Si tiene una cuenta aquí, inicie sesión con su contraseña y vincúlela en Mi cuenta, en la aplicación de administración. Si no, pídaselo a un administrador.',
      disabled: 'Esta cuenta está desactivada.',
      hours: 'Esta cuenta no puede iniciar sesión a esta hora.',
      remote: 'Esta cuenta solo puede iniciar sesión desde la red local.',
      tooMany: 'Demasiados intentos. Vuelva a intentarlo más tarde.',
      failed: 'El inicio de sesión con el proveedor de identidad falló. Vuelva a intentarlo.',
    },
    it: {
      signIn: 'Accedi con {0}',
      signingIn: 'Accesso in corso…',
      refused: 'L’accesso è stato annullato.',
      noAccount: 'Nessun account di questo server è collegato al tuo account presso il provider di identità. Se hai un account qui, accedi con la sua password e collegalo in Il mio account, nell’applicazione di amministrazione. Altrimenti, chiedi a un amministratore.',
      disabled: 'Questo account è disattivato.',
      hours: 'Questo account non può accedere a quest’ora.',
      remote: 'Questo account può accedere solo dalla rete locale.',
      tooMany: 'Troppi tentativi. Riprova più tardi.',
      failed: 'L’accesso con il provider di identità non è riuscito. Riprova.',
    },
    pt: {
      signIn: 'Entrar com {0}',
      signingIn: 'Entrando…',
      refused: 'O login foi cancelado.',
      noAccount: 'Nenhuma conta deste servidor está vinculada à sua conta no provedor de identidade. Se você tem uma conta aqui, entre com a senha dela e vincule-a em Minha conta, no aplicativo de administração. Caso contrário, peça a um administrador.',
      disabled: 'Esta conta está desativada.',
      hours: 'Esta conta não pode entrar neste horário.',
      remote: 'Esta conta só pode entrar pela rede local.',
      tooMany: 'Tentativas demais. Tente novamente mais tarde.',
      failed: 'O login com o provedor de identidade falhou. Tente novamente.',
    },
  }
  // The words for what the server says went wrong; any other, failed.
  var providerOutcomes = {
    provider_refused: 'refused',
    provider_no_account: 'noAccount',
    account_disabled: 'disabled',
    outside_allowed_hours: 'hours',
    outside_local_network: 'remote',
    too_many_attempts: 'tooMany',
    signing_in: 'signingIn',
  }
  function sayProvider(key) {
    var language = /^[a-z]*/.exec(String(document.documentElement.lang || '').toLowerCase())[0]
    return (providerWords[language] || providerWords.en)[key]
  }
  if (typeof fetch !== 'function' || typeof localStorage === 'undefined' || !document.querySelectorAll) return
  var credentialsKey = 'jellyfin_credentials'
  // provider is the provider /admin/api/status names, null for none;
  // message what went wrong, or signing_in, "" for nothing.
  var provider = null
  var message = ''
  var outcome = ''
  try {
    var address = new URL(location.href)
    outcome = address.searchParams.get('polyfinProvider') || ''
    if (outcome) {
      address.searchParams.delete('polyfinProvider')
      history.replaceState(history.state, '', address.pathname + address.search + address.hash)
    }
  } catch (error) {
    return
  }
  message = outcome === 'signed-in' ? 'signing_in' : outcome
  function quietly(run) {
    return function () {
      try {
        return run.apply(this, arguments)
      } catch (error) {
        return undefined
      }
    }
  }
  fetch('/admin/api/status', { credentials: 'same-origin' })
    .then(function (response) {
      return response.ok ? response.json() : null
    })
    .then(
      quietly(function (status) {
        var named = status && status.provider
        provider = named && typeof named.start === 'string' && typeof named.name === 'string' ? named : null
        follow()
      })
    )
    .catch(function () {})
  if (outcome === 'signed-in') settle(0)
  // settle trades the ticket once jellyfin-web drew its first page, or
  // after 20 seconds.
  function settle(waited) {
    if ((/^#!?\//.test(location.hash) && document.querySelector('.page')) || waited >= 20000) return trade()
    setTimeout(
      quietly(function () {
        settle(waited + 250)
      }),
      250
    )
  }
  // device is how jellyfin-web names itself: its client's, else what it
  // keeps in the browser and the browser it runs in.
  function device() {
    var api = typeof window !== 'undefined' && window.ApiClient
    var ask = function (name) {
      try {
        return api && typeof api[name] === 'function' ? String(api[name]() || '') : ''
      } catch (error) {
        return ''
      }
    }
    var agent = typeof navigator !== 'undefined' ? String(navigator.userAgent || '') : ''
    var id = ask('deviceId') || localStorage.getItem('_deviceId2')
    if (!id) {
      id = btoa(agent + '|' + Date.now()).replace(/=/g, '1')
      localStorage.setItem('_deviceId2', id)
    }
    var browser = /Firefox\//.test(agent)
      ? 'Firefox'
      : /Edg\//.test(agent)
        ? 'Edge Chromium'
        : /OPR\//.test(agent)
          ? 'Opera'
          : /Chrome\//.test(agent)
            ? 'Chrome'
            : /Safari\//.test(agent)
              ? 'Safari'
              : 'Web Browser'
    return {
      client: ask('appName') || 'Jellyfin Web',
      name: ask('deviceName') || browser,
      id: id,
      version: ask('appVersion') || '12.4.0',
    }
  }
  // encode escapes a value of the header once, as jellyfin-web sends it:
  // its client gives some values escaped already, such as its app's name
  // (Jellyfin%20Web), which the server takes back once.
  function encode(value) {
    try {
      return encodeURIComponent(decodeURIComponent(value))
    } catch (error) {
      return encodeURIComponent(value)
    }
  }
  function trade() {
    var named = device()
    var header =
      'MediaBrowser Client="' +
      encode(named.client) +
      '", Device="' +
      encode(named.name) +
      '", DeviceId="' +
      encode(named.id) +
      '", Version="' +
      encode(named.version) +
      '"'
    fetch('/admin/api/provider/web', { method: 'POST', credentials: 'same-origin', headers: { Authorization: header } })
      .then(function (response) {
        return response.json().then(function (body) {
          return { ok: response.ok, body: body }
        })
      })
      .then(
        quietly(function (answer) {
          if (!answer.ok || !answer.body || typeof answer.body.accessToken !== 'string') {
            message = (answer.body && answer.body.error) || 'failed'
            return follow()
          }
          keep(answer.body)
          location.replace(location.pathname)
        })
      )
      .catch(function () {
        message = 'failed'
        follow()
      })
  }
  // keep puts the sign-in in this server's entry of jellyfin-web's
  // credentials, first, keeping what jellyfin-web saved there.
  function keep(signedIn) {
    var stored
    try {
      stored = JSON.parse(localStorage.getItem(credentialsKey) || '{}')
    } catch (error) {
      stored = {}
    }
    if (!stored || typeof stored !== 'object' || Array.isArray(stored)) stored = {}
    var servers = Array.isArray(stored.Servers) ? stored.Servers : []
    var entry = { ManualAddress: location.origin, LastConnectionMode: 2 }
    var others = []
    servers.forEach(function (server) {
      if (server && typeof server === 'object' && server.Id === signedIn.serverId) entry = server
      else others.push(server)
    })
    entry.Id = signedIn.serverId
    entry.Name = entry.Name || signedIn.serverName
    entry.UserId = signedIn.userId
    entry.AccessToken = signedIn.accessToken
    entry.DateLastAccessed = Date.now()
    stored.Servers = [entry].concat(others)
    localStorage.setItem(credentialsKey, JSON.stringify(stored))
  }
  // place draws the button, and the line under it, on the sign-in pages
  // shown, and reports whether there was one.
  function place() {
    var found = false
    var pages = document.querySelectorAll('#loginPage .readOnlyContent')
    for (var i = 0; i < pages.length; i++) {
      var container = pages[i]
      found = true
      var button = container.querySelector('.polyfinProviderButton')
      var line = container.querySelector('.polyfinProviderMessage')
      if (!provider) {
        if (button) button.remove()
        if (line) line.remove()
        continue
      }
      if (!button) {
        button = document.createElement('button')
        button.setAttribute('type', 'button')
        button.className = 'raised block emby-button polyfinProviderButton'
        button.appendChild(document.createElement('span'))
        button.addEventListener(
          'click',
          quietly(function () {
            location.href = provider.start + '?app=web'
          })
        )
        container.insertBefore(button, container.firstChild)
      }
      button.firstChild.textContent = sayProvider('signIn').replace('{0}', function () {
        return provider.name
      })
      if (!message) {
        if (line) line.remove()
        continue
      }
      if (!line) {
        line = document.createElement('p')
        line.className = 'fieldDescription polyfinProviderMessage'
        line.setAttribute('role', 'status')
        container.insertBefore(line, button.nextSibling)
      }
      line.textContent = sayProvider(providerOutcomes[message] || 'failed')
    }
    return found
  }
  // follow looks for the sign-in page as the route changes, every 200
  // milliseconds for 5 seconds, until it finds it.
  var looking = null
  function follow() {
    clearTimeout(looking)
    var tries = 0
    var look = quietly(function () {
      looking = null
      if (place() || ++tries >= 25) return
      looking = setTimeout(look, 200)
    })
    look()
  }
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
