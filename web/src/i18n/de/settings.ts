import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Jeder Benutzer kann auf seiner Seite „Mein Konto“ eigene Konten bei Trakt, Simkl, MDBList, PublicMetaDB, AniList und MyAnimeList verbinden, und Polyfin teilt diesen Diensten mit, was er schaut; Last.fm und ListenBrainz erfahren, welche Songs er abspielt. Trakt, Simkl, AniList, MyAnimeList und Last.fm brauchen zuerst eine App dieses Servers, die hier eingerichtet wird. MDBList, PublicMetaDB und ListenBrainz brauchen nichts.',
      traktSetup:
        'Erstelle auf trakt.tv eine App, unter Settings, Your API Apps, New application. Gib ihr einen beliebigen Namen und kopiere dann ihre Client-ID und ihr Client-Secret hierher.',
      redirectUri: 'In der App einzutragende Weiterleitungs-URI',
      traktClientId: 'Trakt-Client-ID',
      traktClientIdHelp:
        'Leer lassen, um Trakt auszuschalten. Benutzer können sich verbinden, sobald Client-ID und Client-Secret gespeichert sind.',
      traktClientSecret: 'Trakt-Client-Secret',
      traktClientSecretHelp:
        'Wird auf der Seite der App auf trakt.tv unter der Client-ID angezeigt.',
      simklSetup:
        'Erstelle auf simkl.com eine App, unter Settings, Developer, Create new app, vom Typ „TV, devices & command line“ (AUTH V2 von Simkl; sie braucht keine Weiterleitungs-URI, und Client-IDs älterer AUTH-V1-Apps werden abgelehnt). Gib ihr einen beliebigen Namen und kopiere dann ihre Client-ID hierher.',
      simklClientId: 'Simkl-Client-ID',
      simklClientIdHelp:
        'Leer lassen, um Simkl auszuschalten. Ein Client-Secret wird nicht benötigt.',
      lastFmSetup:
        'Erstelle auf last.fm ein API-Konto, unter last.fm/api/account/create. Gib ihm einen beliebigen Namen und lass die Callback-URL leer: Benutzer erlauben Polyfin den Zugriff auf Last.fm, und Polyfin fragt ihre Sitzung selbst bei Last.fm ab. Kopiere dann den API-Schlüssel und das Shared Secret hierher.',
      lastFmApiKey: 'Last.fm-API-Schlüssel',
      lastFmApiKeyHelp:
        'Leer lassen, um Last.fm auszuschalten. Benutzer können sich verbinden, sobald API-Schlüssel und Shared Secret gespeichert sind.',
      lastFmSecret: 'Last.fm-Shared-Secret',
      lastFmSecretHelp:
        'Wird auf der Seite deines API-Kontos auf last.fm unter dem API-Schlüssel angezeigt.',
      aniListSetup:
        'Erstelle auf anilist.co eine App, unter Settings, Developer, Create New Client. Gib ihr einen beliebigen Namen und als Weiterleitungs-URL die Adresse unten, auf der AniList den Benutzern den Code anzeigt, den sie in Polyfin einfügen. Kopiere dann ihre Client-ID und ihr Client-Secret hierher.',
      aniListClientId: 'AniList-Client-ID',
      aniListClientIdHelp:
        'Leer lassen, um AniList auszuschalten. Benutzer können sich verbinden, sobald Client-ID und Client-Secret gespeichert sind.',
      aniListClientSecret: 'AniList-Client-Secret',
      aniListClientSecretHelp:
        'Wird auf der Seite der App auf anilist.co unter der Client-ID angezeigt.',
      myAnimeListSetup:
        'Erstelle auf myanimelist.net eine App, unter Account Settings, API, Create ID, vom Typ „web“. Gib ihr einen beliebigen Namen und als Weiterleitungs-URL die Adresse unten: MyAnimeList leitet den Browser der Benutzer dorthin zurück, sobald sie Polyfin den Zugriff erlauben. Nur ihr Browser muss sie erreichen können. Kopiere dann ihre Client-ID und ihr Client-Secret hierher.',
      myAnimeListRedirectHelp:
        'Die öffentliche Adresse dieses Servers unter Einstellungen › Allgemein, sonst die Adresse, die du gerade verwendest.',
      myAnimeListClientId: 'MyAnimeList-Client-ID',
      myAnimeListClientIdHelp:
        'Leer lassen, um MyAnimeList auszuschalten. Benutzer können sich verbinden, sobald Client-ID und Client-Secret gespeichert sind.',
      myAnimeListClientSecret: 'MyAnimeList-Client-Secret',
      myAnimeListClientSecretHelp:
        'Wird auf der Seite der App auf myanimelist.net unter der Client-ID angezeigt.',
    },
    title: 'Einstellungen',
    description: 'Optionen, die für den gesamten Server gelten.',
    serverName: 'Servername',
    serverNameHelp: (r: RangeText) =>
      `${r.min} bis ${r.max} Zeichen, wird in Jellyfin-Apps angezeigt.`,
    quickConnect: 'Quick Connect erlauben',
    quickConnectHelp:
      'Erlaubt TV- und Handy-Apps die Anmeldung mit einem 6-stelligen Code, der hier bestätigt wird.',
    language: 'Sprache generierter Namen',
    languageHelp:
      'Polyfin benennt in Jellyfin-Apps einiges selbst: Staffeln („Staffel 1“, „Specials“), Folgen ohne Titel und den Typ, der an Bibliotheken mit gleichem Namen angehängt wird („Beliebt (Filme)“).',
    legacyAuthorization: 'Veraltete Autorisierung erlauben',
    legacyAuthorizationHelp:
      'Akzeptiert die alten X-Emby-*-Header, den Parameter api_key und das Emby-Schema für Apps, die sie noch brauchen. Standardmäßig aus, wie in Jellyfin 12.3.',
    updateCheck: 'Nach neuen Versionen suchen',
    updateCheckHelp:
      'Einmal täglich und kurz nach dem Start fragt Polyfin bei GitHub die neueste Veröffentlichung ab und sendet dabei nur seine Version. Eine neuere Version wird auf der Startseite und unter System › Systemzustand angezeigt und an die Benachrichtigungen für neue Versionen gesendet. Wenn aus, kontaktiert Polyfin GitHub nie.',
    restartConfirmed: 'Polyfin wird nach dem Beenden neu gestartet',
    restartConfirmedHelp:
      'Einschalten, sobald der Container von Polyfin eine Neustart-Richtlinie hat, die ihn wieder startet, etwa unless-stopped, always oder on-failure, oder sein Dienstverwalter das tut. Ohne den Docker-Socket, den Polyfin nie erhält, sieht Polyfin die Neustart-Richtlinie seines Containers nicht: Solange dies aus ist, warnt Neu starten auf der Startseite, dass Polyfin vielleicht nicht zurückkommt.',
    updateToolUrl: 'Adresse des Update-Werkzeugs',
    updateToolUrlHelp:
      'Die Update-API eines Werkzeugs, das neben Polyfin läuft und seinen Container aktualisiert, etwa http://updater:8080/v1/update. Jetzt aktualisieren auf der Startseite sendet ihm eine POST-Anfrage ohne Inhalt. Leer blendet die Schaltfläche aus.',
    updateToolToken: 'Token des Update-Werkzeugs',
    updateToolTokenHelp:
      'Wird bei jeder Anfrage als Authorization: Bearer gesendet, wenn das Werkzeug eines verlangt. Verschlüsselt gespeichert, wie die anderen Geheimnisse.',
    localDiscovery: 'Auf Suche im lokalen Netzwerk antworten',
    localDiscoveryHelp:
      'Jellyfin- und Emby-Apps, die im lokalen Netzwerk nach Servern suchen, finden Polyfin und listen es auf, sodass seine Adresse nicht eingetippt werden muss. Sie fragen über UDP-Port 7359, und Polyfin antwortet nur Adressen aus dem lokalen Netzwerk. Wirkt sofort, ohne Neustart. In Docker ist dafür Host-Networking oder der veröffentlichte Port 7359/udp nötig: siehe Installationsdokumentation.',
    localAddress: 'Adresse im lokalen Netzwerk',
    localAddressHelp:
      'Die Adresse, unter der Apps im lokalen Netzwerk Polyfin erreichen, etwa http://192.168.1.10:8096. Antworten auf die Suche nennen sie, und Jellyfin-Apps lesen sie als lokale Adresse des Servers. Wenn leer, nennen die Antworten die Adresse der Netzwerkschnittstelle, über die die App erreicht wird. Das passt, außer Polyfin läuft in einem Docker-Bridge-Netzwerk: Gib dort die Adresse des Hosts und den Port von Polyfin ein.',
    torrents: {
      description:
        'Manche Add-ons listen Torrents statt Links. Polyfin kann sie abspielen: Es tritt dem Schwarm eines Torrents nur bei, solange eine Wiedergabe ihn liest, und leitet seinen gesamten Verkehr über eine einzige Route, eine Netzwerkschnittstelle wie die eines VPN, einen SOCKS5-Proxy oder einen WireGuard-Tunnel, den Polyfin selbst betreibt. Nichts wird je auf anderem Weg gesendet. Die Teile werden im Speicherplatz für gelesene Dateien abgelegt, unter Einstellungen › Wiedergabe.',
      play: 'Torrent-Streams abspielen',
      playHelp:
        'Standardmäßig aus. Beim Abspielen wird ein Torrent von anderen Peers heruntergeladen, und das Heruntergeladene wird geteilt, soweit das Upload-Limit es erlaubt: Du bist für das verantwortlich, was dieser Server herunterlädt und teilt. Benutzer brauchen außerdem „Darf Torrent-Streams abspielen“ auf ihrer Seite unter Benutzer. Erfordert eine Route.',
      route: 'Route',
      routeHelp:
        'Jeder Torrent-Socket, für Peers, Tracker und das DHT, läuft ausschließlich über diesen Weg. Ist er nicht verfügbar, werden Torrent-Versionen übersprungen, auch während einer Wiedergabe, und System › Systemzustand nennt den Grund.',
      routeHelpNoInterface:
        'Jeder Torrent-Socket, für Peers, Tracker und das DHT, läuft ausschließlich über diesen Weg. Dieses System bindet keinen Socket an eine Netzwerkschnittstelle (das kann nur Linux), so auch Polyfin Desktop für Windows und macOS: Ein SOCKS5-Proxy oder ein WireGuard-Tunnel trägt die Torrents. Ist er nicht verfügbar, werden Torrent-Versionen übersprungen, auch während einer Wiedergabe, und System › Systemzustand nennt den Grund.',
      routes: {
        interface: 'Netzwerkschnittstelle',
        proxy: 'SOCKS5-Proxy',
        wireguard: 'WireGuard-Tunnel',
      },
      interface: 'Netzwerkschnittstelle für Torrents',
      interfaceHelp:
        'Ihr Name, etwa wg0. Jeder Torrent-Socket wird an sie gebunden, und Tracker-Namen werden über sie aufgelöst. Nur Linux. In Docker muss der Container die Schnittstelle sehen: siehe Torrent-Dokumentation.',
      proxy: 'SOCKS5-Proxy für Torrents',
      proxyHelp:
        'Seine Adresse und sein Port, etwa 10.0.0.2:1080. Peers und HTTP-Tracker laufen über sein CONNECT. UDP-Tracker, das DHT und uTP brauchen sein UDP ASSOCIATE und bleiben ohne es aus. Über einen Proxy kann sich kein Peer eingehend verbinden.',
      proxyUser: 'Proxy-Benutzer',
      proxyUserHelp: 'Leer, wenn der Proxy keinen verlangt.',
      proxyPassword: 'Proxy-Passwort',
      proxyPasswordHelp: 'Wird verschlüsselt gespeichert, wenn POLYFIN_SECRET_KEY gesetzt ist.',
      wireGuard: 'WireGuard-Konfiguration',
      wireGuardHelp:
        'Füge die .conf ein, die dein VPN-Anbieter für WireGuard bereitstellt. Polyfin betreibt den Tunnel selbst, nur für Torrents, ohne dass auf diesem System etwas einzurichten ist. Tracker-Namen werden über den Tunnel von seinen DNS-Servern aufgelöst, nie auf anderem Weg. Kein Peer kann sich von außen verbinden. Verschlüsselt gespeichert, wenn POLYFIN_SECRET_KEY gesetzt ist, und nie wieder angezeigt.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `Eine Konfiguration ist gespeichert: Server ${endpoint}, Adressen ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `Tracker-Namen werden von ${servers} über den Tunnel aufgelöst.`,
      wireGuardNoDns:
        'Sie nennt keinen DNS-Server: Tracker mit Hostnamen werden nicht erreicht, Tracker mit Adresse schon.',
      wireGuardReplace: 'Füge eine andere Konfiguration ein, um die gespeicherte zu ersetzen.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Zeile ${problem.line}: ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'ein Schlüssel von 32 Bytes in Base64',
          PublicKey: 'ein Schlüssel von 32 Bytes in Base64',
          PresharedKey: 'ein Schlüssel von 32 Bytes in Base64',
          Address: 'IP-Adressen wie 10.2.0.2/32, durch Kommas getrennt',
          DNS: 'IP-Adressen, durch Kommas getrennt',
          MTU: 'eine Zahl von 576 bis 65535',
          ListenPort: 'ein Port von 0 bis 65535',
          Endpoint: 'ein Hostname oder eine IP-Adresse mit Port, etwa vpn.example.com:51820',
          AllowedIPs: 'IP-Adressen mit ihrer Präfixlänge wie 0.0.0.0/0, durch Kommas getrennt',
          PersistentKeepalive: 'off oder eine Anzahl Sekunden von 0 bis 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'Die WireGuard-Konfiguration muss ein Text von höchstens 8192 Bytes sein.'
          case 'syntax':
            return `${where}Jede Zeile muss eine [Sektion], ein Schlüssel = Wert oder ein #-Kommentar sein.`
          case 'outside':
            return `${where}${problem.key} steht vor [Interface] oder [Peer].`
          case 'unknown':
            return `${where}${problem.key} gehört nicht zu einer WireGuard-Konfiguration.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}Nur eine [Peer]-Sektion wird unterstützt.`
              : `${where}${problem.key} ist zweimal angegeben.`
          case 'missing':
            return `${where}${problem.key} fehlt in der WireGuard-Konfiguration.`
          case 'no_ipv4':
            return `${where}Address braucht eine IPv4-Adresse, die UDP-Tracker und das DHT verwenden.`
          case 'not_all':
            return `${where}AllowedIPs braucht 0.0.0.0/0: Die Peers eines Torrents sind überall.`
          default:
            return `${where}${problem.key} muss ${expected[problem.key] ?? 'gültig'} sein.`
        }
      },
      downloadLimit: 'Download-Limit (Mbit/s, 0 = keins)',
      downloadLimitHelp: (r: RangeText) => `Von 0 bis ${r.max}; standardmäßig ${r.default}.`,
      uploadLimit: 'Upload-Limit (Mbit/s, 0 = kein Teilen)',
      uploadLimitHelp: (r: RangeText) =>
        `Peers bevorzugen diejenigen, die zurückteilen, daher kann der Start bei 0 langsamer sein. Das Teilen endet mit der Wiedergabe. Von 0 bis ${r.max}; standardmäßig ${r.default}.`,
      atOnce: 'Torrents gleichzeitig (0 = keine Grenze)',
      atOnceHelp: (r: RangeText) =>
        `Wie viele Torrents gleichzeitig laufen dürfen, für alle Benutzer zusammen. An der Grenze spielt ein Titel eine andere seiner Versionen, die kein Torrent ist, oder gilt vorerst als nicht abspielbar; ein Platz wird frei, wenn ein Torrent stoppt, 30 Sekunden nach seiner Wiedergabe, oder sofort, wenn dasselbe Gerät oder derselbe Benutzer einen anderen Titel abspielt. Von 0 bis ${r.max}; standardmäßig ${r.default}.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'Fernseher, Receiver, Konsolen und Player im lokalen Netzwerk, die keine Jellyfin-App haben, können die Bibliotheken über DLNA durchsuchen und abspielen. Ein Gerät kann sich nicht anmelden: Jedes zeigt die Bibliotheken des Benutzers, der dafür gewählt wurde, mit den Einschränkungen dieses Benutzers.',
      share: 'Bibliotheken über DLNA freigeben',
      shareHelp:
        'Macht Polyfin den Geräten im lokalen Netzwerk über UDP-Port 1900 bekannt und antwortet nur ihnen. Gilt sofort, ohne Neustart. In Docker ist dafür Host-Networking oder ein Netzwerk nötig, das dem Container eine eigene Adresse gibt: siehe DLNA-Dokumentation.',
      name: 'Name im Netzwerk',
      nameHelp: 'Der Name, unter dem Geräte Polyfin anzeigen. Leer: der Servername.',
      newDevicesUser: 'Neuen Geräten die Bibliotheken zeigen von',
      newDevicesUserHelp:
        'Der Benutzer, dessen Bibliotheken ein zum ersten Mal gesehenes Gerät zeigt. Mit „Niemand“ sieht ein neues Gerät einen leeren Server, bis du unten einen Benutzer dafür wählst.',
      noOne: 'Niemand',
      markPlayed: 'Was DLNA-Geräte abspielen, als gesehen markieren',
      markPlayedHelp:
        'Geräte melden keine Position: Ein Titel gilt als gesehen, sobald das Gerät mehr als „Als gesehen markiert nach (%)“ davon gelesen hat. Geräte lesen voraus, daher kann ein Titel zu früh markiert werden.',
      devices: 'Geräte',
      devicesEmpty: 'Bisher hat noch kein Gerät etwas angefragt.',
      device: 'Gerät',
      address: 'Adresse',
      model: 'Modell',
      lastSeen: 'Zuletzt gesehen',
      showsLibrariesOf: 'Zeigt die Bibliotheken von',
      blocked: 'Gesperrt',
      profile: 'Profil',
      automaticProfile: (name: string) => `Automatisch (${name})`,
      forget: 'Vergessen',
      profiles: 'Profile',
      profilesHelp:
        'Profile verwenden das XML-Format des DLNA-Plugins von Jellyfin. Ein Gerät erhält das erste, das zu seinen gesendeten Angaben passt, zuerst hinzugefügte Profile, sonst das generische.',
      builtin: 'Integriert',
      addProfile: 'Profil hinzufügen',
      replaceProfile: 'Ersetzen',
      removeProfile: 'Entfernen',
      profileXml: 'Profil-XML',
      devicesHelp:
        'Die Geräte im lokalen Netzwerk, die bei Polyfin etwas angefragt haben, zuletzt gesehene zuerst, jeweils an ihrer Adresse erkannt. Änderungen gelten sofort.',
      devicesLabel: 'DLNA-Geräte',
      automatic: 'Automatisch',
      deviceSaved: (name: string) => `${name} gespeichert.`,
      forgetTitle: (name: string) => `${name} vergessen?`,
      forgetBody:
        'Polyfin vergisst den Benutzer, die Sperre und das Profil, die dafür gewählt wurden. Fragt es erneut an, ist es ein neues Gerät und zeigt die Bibliotheken des Benutzers, der für neue Geräte gewählt ist.',
      forgotten: (name: string) => `${name} vergessen.`,
      profilesLabel: 'DLNA-Profile',
      added: 'Hinzugefügt',
      viewXml: 'XML anzeigen',
      profileXmlHelp:
        'Füge das XML eines Profils für das DLNA-Plugin von Jellyfin ein oder öffne seine Datei.',
      openFile: 'Datei öffnen',
      add: 'Hinzufügen',
      adding: 'Wird hinzugefügt…',
      replacing: 'Wird ersetzt…',
      profileAdded: (name: string) => `Profil ${name} hinzugefügt.`,
      profileReplaced: (name: string) => `Profil ${name} ersetzt.`,
      removeTitle: (name: string) => `Profil ${name} entfernen?`,
      removeBody:
        'Die Geräte, denen es fest zugewiesen ist, erhalten ihr Profil wieder anhand dessen, was sie senden.',
      profileRemoved: (name: string) => `Profil ${name} entfernt.`,
    },
    metrics: 'Metriken bereitstellen',
    metricsHelp:
      'Stellt die Metriken von Polyfin unter /metrics für Prometheus bereit: Wiedergaben, Konvertierungen, Add-on-Anfragen, Importe, Benachrichtigungen, HTTP-Anfragen, Scans, die Datenbank, den Speicherplatz und den Prozess. Sie nennen keine Benutzer, Titel, Adressen oder Geräte. Prometheus sendet das Metrik-Token mit jeder Anfrage; ohne es antwortet /metrics mit 401 und, solange dies ausgeschaltet ist, mit 404.',
    metricsToken: 'Metrik-Token',
    metricsTokenHelp:
      'Wird von Polyfin erstellt, wenn Metriken zum ersten Mal eingeschaltet werden. Prometheus sendet es als Bearer-Token: Eine Scrape-Konfiguration findest du in der Administrationsdokumentation.',
    metricsTokenPending: 'Polyfin erstellt das Metrik-Token beim Speichern.',
    newMetricsToken: 'Neues Token erstellen',
    newMetricsTokenTitle: 'Neues Metrik-Token erstellen?',
    newMetricsTokenConfirm:
      'Das aktuelle Token funktioniert sofort nicht mehr: Prometheus braucht das neue, um die Metriken zu lesen.',
    newMetricsTokenMade: 'Neues Metrik-Token erstellt',
    legacyWarningTitle: 'Sicherheitswarnung',
    legacyWarning:
      'Veraltete Methoden können Anmeldedaten in URLs senden, die in Protokollen, im Browserverlauf und in Proxys landen. Schalte dies nur ein, wenn sich eine deiner Apps sonst nicht anmelden kann.',
    prepareAhead: 'Wiedergabe im Voraus vorbereiten',
    prepareAheadHelp:
      'Polyfin liest die Datei, sobald die Seite eines Titels geöffnet wird, und bereitet gegen Ende der aktuellen Folge die nächste vor, damit die Wiedergabe sofort startet. Außerdem listet es die Versionen der Titel in „Weiterschauen“ und „Als Nächstes“ im Voraus auf, damit ihre Seiten sie sofort zeigen. Dadurch gehen etwas mehr Anfragen an deine Quellen, auch für Titel, die geöffnet, aber nicht abgespielt werden.',
    transcoding: 'Konvertierung (Transkodierung)',
    transcodingHelp:
      'Kodiert Video und Audio neu für Apps, die eine Datei nicht unverändert abspielen können. Ist dies ausgeschaltet, spielen Apps Dateien unverändert oder nur neu verpackt ohne Neukodierung ab, und ein Titel, den eine App so nicht abspielen kann, startet auf dieser App nicht. Das gilt auch für nach HEVC konvertierte Aufnahmen auf Apps, die kein HEVC abspielen.',
    analysisTimeout: 'Maximale Zeit zum Analysieren einer Version',
    analysisTimeoutHelp: (r: RangeText) =>
      `Vor der ersten Wiedergabe analysiert Polyfin die Datei oder den Kanal, um zu wissen, wie sie abgespielt werden. Antwortet die Quelle nicht innerhalb dieser Anzahl Sekunden, gibt Polyfin diese Version auf und geht zur nächsten über. Ein niedrigerer Wert wechselt schneller, kann aber langsame Quellen aufgeben, die funktioniert hätten. Ein Kanal wird höchstens 8 Sekunden lang analysiert. Von ${r.min} bis ${r.max} Sekunden; standardmäßig ${r.default}.`,
    versionAttempts: 'Versuchte Versionen, wenn eine nicht funktioniert',
    versionAttemptsHelp: (r: RangeText) =>
      `Wenn eine App einen Titel ohne Wahl einer Version abspielt, versucht Polyfin die Versionen der Reihe nach, bis eine funktioniert, analysiert aber höchstens so viele. Bereits analysierte Versionen werden ebenfalls versucht, da sie nichts kosten. Ein höherer Wert findet öfter eine funktionierende Version, aber bei einem Titel, der nicht abspielbar ist, dauert es länger, bis das feststeht. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    preferDirectPlay: 'Versionen bevorzugen, die die App ohne Konvertierung abspielt',
    preferDirectPlayHelp:
      'Wenn eine App einen Titel ohne Wahl einer Version abspielt, wählt Polyfin die erste Version, die die App unverändert oder nur neu verpackt abspielt, statt der ersten, die überhaupt abspielbar ist. Die allererste Wiedergabe eines Titels kann etwas langsamer sein, da mehr Versionen analysiert werden können; sobald sie bekannt sind, ändert sich nichts mehr.',
    nameHowVersionsPlay: 'In Versionsnamen angeben, wie sie abgespielt werden',
    nameHowVersionsPlayHelp:
      'Versionsnamen enden damit, wie die App die jeweilige Version abspielt: „(Direktes Abspielen)“, „(Direktes Streamen)“, wenn sie ohne Umwandlung kopiert wird, „(umgewandelt: Grund)“ mit HDR, dem Qualitätslimit, Untertiteln, Video oder Audio als Grund, oder „(hier nicht abspielbar)“. Polyfin entscheidet das anhand des Geräteprofils, das die App bei ihrer letzten Wiedergabe gesendet hat. Vor der ersten Wiedergabe eines Geräts oder nach einem Neustart sagen die Namen daher nichts, ebenso bei einer Version, die noch nicht analysiert oder von RemuxDB beschrieben wurde. Die Namen stehen in der Sprache der generierten Namen. Dafür wird nichts gelesen.',
    versionOrderGroup: 'Versionsreihenfolge',
    versionOrder: {
      label: 'Versionen sortieren nach',
      help: 'Jedes Kriterium gruppiert die Versionen eines Films oder einer Folge, und das nächste sortiert sie innerhalb jeder Gruppe; gleichrangige Versionen behalten die Reihenfolge der Add-ons. Die erste Version ist die, die „Abspielen“ startet, und die Versionen, die versucht werden, wenn sie nicht funktioniert, folgen danach. Polyfin richtet sich nach dem, was es über eine Version weiß, ohne die Datei zu lesen: nach ihrer Analyse, sobald sie abgespielt wurde, sonst nach der Beschreibung von RemuxDB, sonst nach den Wörtern ihrer Bezeichnungen. Eine Version, über die ein Kriterium nichts weiß, kommt nach den anderen. Die bevorzugte Audiosprache ist die jedes einzelnen Benutzers, eingestellt in seiner App: Versionen, die bekanntermaßen in dieser Sprache sind, kommen zuerst, dann solche mit MULTI oder DUAL, dann solche mit unbekannter Sprache. Solange die Seite eines Titels geöffnet ist, behalten die dort angezeigten Versionen ihre Plätze.',
      none: 'Keine: Die Versionen behalten die Reihenfolge der Add-ons.',
      add: 'Kriterium hinzufügen',
      remove: (name: string) => `${name} entfernen`,
      criterion: (position: number) => `Kriterium ${position}`,
      criteria: {
        language: 'Bevorzugte Audiosprache',
        'resolution-high': 'Auflösung, höchste zuerst',
        'resolution-low': 'Auflösung, niedrigste zuerst',
        'size-large': 'Dateigröße, größte zuerst',
        'size-small': 'Dateigröße, kleinste zuerst',
        codec: 'Videocodec, in der Reihenfolge unten',
        'hdr-first': 'HDR und Dolby Vision zuerst',
        'hdr-last': 'HDR und Dolby Vision zuletzt',
        source: 'Quelle, in der Reihenfolge unten',
      },
    },
    versionCodecOrder: {
      label: 'Reihenfolge der Videocodecs',
      help: 'Die Reihenfolge, in die das Kriterium „Videocodec“ die Versionen bringt.',
    },
    versionSourceOrder: {
      label: 'Reihenfolge der Quellen',
      help: 'Die Reihenfolge, in die das Kriterium „Quelle“ die Versionen bringt: die Add-ons und Ordner des Servers, die Versionen von Filmen und Folgen liefern. Eine später hinzugefügte Quelle steht zuletzt, bis du sie verschiebst, und die eigenen Add-ons der Benutzer kommen nach den Quellen des Servers. Das ändert nur die Reihenfolge der Versionen: Beschreibungen, Untertitel und Suchergebnisse folgen der Reihenfolge unter Inhalte › Quellen.',
      none: 'Noch keine Quelle des Servers liefert Versionen von Filmen oder Folgen.',
      dragHandle: (name: string) => `${name} ziehen`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 Part 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Versionen ausblenden, die größer sind als (GB, 0 = keine Grenze)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Versionen, die bekanntermaßen größer sind, werden nirgends aufgeführt: weder auf Titelseiten noch bei der Wiedergabe oder bei Downloads. Wenn diese und die folgenden Regeln alle Versionen eines Titels ausblenden würden, wird keine ausgeblendet. Bis zu ${r.max} GB; standardmäßig ${r.default}.`,
    hiddenVersionCodecs: 'Diese Videocodecs ausblenden',
    hiddenVersionCodecsHelp:
      'Versionen, die bekanntermaßen diese Codecs verwenden, werden nirgends aufgeführt.',
    hiddenVersionWords: 'Versionen ausblenden, deren Bezeichnungen Folgendes enthalten',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `Ein Wort oder eine Wortgruppe pro Zeile, unabhängig von Groß- und Kleinschreibung gesucht in Name, Beschreibung und Dateiname einer Version: für Add-ons, die in ihren Bezeichnungen angeben, dass eine Datei noch nicht abspielbereit ist. Bis zu ${r.max} Zeilen; standardmäßig keine.`,
    syncAddonSubtitles: 'Add-on-Untertitel synchronisieren',
    syncAddonSubtitlesHelp:
      'Passt die Untertitel der Add-ons an die abgespielte Version an, wenn sie zu früh, zu spät, schneller oder langsamer laufen. Bei der ersten Wiedergabe einer Version ermittelt Polyfin im Hintergrund, wann gesprochen wird: anhand eines Textuntertitels in der Datei, sonst anhand von drei Abschnitten ihrer Audiospur, wobei höchstens 300 MB der Datei eines Add-ons gelesen werden. Torrent-Versionen sind ausgenommen, da sie dafür aus ihrem Schwarm heruntergeladen werden müssten. Ab dann wird ein Untertitel, der verschoben oder beschleunigt deutlich besser passt, synchronisiert geliefert, und sein Name endet mit „(synchronisiert)“.',
    remuxDb: 'Versionen mit RemuxDB beschreiben',
    remuxDbHelp:
      'Zeigt die Audio-, Untertitel- und Videospuren einer Version vor ihrer ersten Wiedergabe so an, wie RemuxDB sie in derselben Datei gefunden hat. Wenn die Details eines Titels geöffnet werden, fragt Polyfin RemuxDB, eine Community-Datenbank, anhand der IMDb-Kennung des Titels ab und findet die Versionen über ihre Dateinamen. Bei der Wiedergabe wird trotzdem jede Version analysiert.',
    remuxDbUrl: 'RemuxDB-Adresse',
    remuxDbUrlHelp: 'Der RemuxDB-Server, den Polyfin abfragt.',
    playbackHistoryGroup: 'Wiedergabeverlauf',
    playbackHistory: 'Wiedergabeverlauf speichern',
    playbackHistoryHelp:
      'Speichert jedes abgespielte Video, mit wem es abgespielt hat, in welcher App und wie, für System › Statistiken und die Statistiken jedes Benutzers unter Mein Konto. Ausgeschaltet wird nichts mehr gespeichert; bereits Gespeichertes bleibt, bis es zu alt ist.',
    playbackHistoryDays: 'Aufbewahrungsdauer des Wiedergabeverlaufs (Tage)',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Ältere Wiedergaben werden gelöscht. Das wird täglich geprüft. Von ${r.min} bis ${r.max}, standardmäßig ${r.default}.`,
    publicAddress: 'Öffentliche Adresse',
    publicAddressHelp:
      'Die Adresse, unter der Polyfin aufgerufen wird, etwa https://media.example.org. Links in Benachrichtigungen beginnen damit; ist sie leer, enthalten Nachrichten keinen Link. Beginnt sie mit https://, leitet „HTTP auf HTTPS umleiten“ unter Einstellungen › Sicherheit auf ihren Port um, oder auf 443, wenn sie keinen angibt.',
    email: 'E-Mail',
    emailHelp:
      'E-Mail-Ziele laufen über diesen SMTP-Server. Sie können hinzugefügt werden, sobald ein Server und eine Absenderadresse gespeichert sind.',
    smtpHost: 'SMTP-Server',
    smtpHostHelp:
      'Sein Hostname, etwa smtp.example.org. Ist er leer, können keine E-Mail-Ziele hinzugefügt werden.',
    smtpPort: 'SMTP-Port',
    smtpPortHelp: 'Meist 587 mit STARTTLS und 465 mit TLS von Anfang an.',
    smtpSecurity: 'SMTP-Verschlüsselung',
    smtpSecurityHelp:
      'Wie die Verbindung verschlüsselt wird. Mit STARTTLS wird nichts gesendet, wenn der Server es nicht anbietet. Ohne Verschlüsselung wird das Passwort nur an diesen Rechner gesendet.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS von Anfang an', none: 'Keine' },
    smtpUser: 'SMTP-Benutzer',
    smtpUserHelp: 'Leer lassen, um ohne Anmeldung zu senden.',
    smtpPassword: 'SMTP-Passwort',
    smtpPasswordHelp: 'Es wird verschlüsselt gespeichert und nie wieder angezeigt.',
    smtpFrom: 'Absenderadresse',
    smtpFromHelp: 'Die Adresse, von der Nachrichten kommen, etwa polyfin@example.org.',
    smtpFromName: 'Absendername',
    smtpFromNameHelp: 'Wird mit der Adresse angezeigt. Ist er leer, wird der Servername angezeigt.',
    weeklySummary: 'Wochenzusammenfassung',
    weeklySummaryHelp:
      'Ziele, die unter ihren Ereignissen „Wochenzusammenfassung“ gewählt haben, erhalten sie einmal pro Woche: die Ziele des Servers und der Administratoren nach der Woche des Servers, die anderen nach ihrer eigenen Woche. Für eine Woche ohne Neuigkeiten wird nichts gesendet. System › Zeitplan sendet sie sofort.',
    weeklySummaryDay: 'Wochenzusammenfassung senden am',
    weeklySummaryDayHelp: (day: string) => `Standardmäßig ${day}.`,
    weeklySummaryHour: 'Wochenzusammenfassung senden um',
    weeklySummaryHourHelp: (hour: string) =>
      `In der Zeitzone des Servers. Monats- und Jahresrückblick werden am 1. ebenfalls zu dieser Uhrzeit gesendet. Standardmäßig ${hour}.`,
    maxConversions: 'Gleichzeitige Videokonvertierungen (0 = unbegrenzt)',
    maxConversionsHelp: (r: RangeText) =>
      `Das Konvertieren von Video ist die aufwendigste Arbeit des Servers. Sobald bei so vielen Wiedergaben das Video konvertiert wird (einschließlich ins Bild eingebrannter Untertitel), erhält eine neue Wiedergabe eine Version, die keine Konvertierung braucht, oder sie startet nicht. Bereits laufende Wiedergaben werden nie abgebrochen. Live-TV behält außerdem eigene Grenzen: 4 Kanäle pro Benutzer und 16 für den Server. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    maxConversionHeight: 'Maximale Qualität konvertierter Videos',
    maxConversionHeightHelp:
      'Konvertierte Videos werden höchstens auf diese Höhe verkleinert, ohne verzerrt zu werden, damit sie auch über eine langsamere Verbindung flüssig laufen. Unverändert abgespielte oder nur neu verpackte Dateien behalten ihre Qualität. Eine Grafikkarte konvertiert bis 4K; der Prozessor hört bei 1080p auf und, wenn er HDR in SDR konvertiert, bei „Maximale Qualität von vom Prozessor konvertiertem HDR“ unter Konvertierung (Transkodierung) › HDR.',
    conversionHeightOriginal: 'Original',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Konvertierte Downloads',
    convertedDownloadsHelp:
      'Apps, die danach fragen, etwa Streamyfin, speichern einen Titel in der vom Benutzer gewählten Qualität: Polyfin sendet eine kleinere Kopie, die beim Herunterladen durch die App erstellt und nie auf dem Server gespeichert wird. Ohne diese Option speichern sie die Datei unverändert. Jeder Download zählt während der Konvertierung zu „Gleichzeitige Videokonvertierungen“ und als eine der „Gleichzeitigen Wiedergaben“ des Benutzers. Der Benutzer braucht „Darf herunterladen“ und, wenn Video oder Audio konvertiert wird, „Darf Konvertierung (Transkodierung) nutzen“. Ein unterbrochener Download beginnt von vorn. Standardmäßig aus.',
    conversion: {
      description:
        'Wenn eine App eine Datei nicht so abspielen kann, wie sie ist, konvertiert Polyfin sie mit FFmpeg. Diese Einstellungen legen fest, wie das geschieht, für Filme, Serien und Live-TV gleichermaßen. Die Standardwerte passen für die meisten Server.',
      groups: {
        general: 'Allgemein',
        subtitles: 'Bilduntertitel',
        gpu: 'Grafikkarte',
        video: 'Video',
        hdr: 'HDR',
        interlaced: 'Interlaced-Video',
        audio: 'Audio',
        performance: 'Leistung',
      },
      subtitleOcr: 'Bilduntertitel in Text umwandeln',
      subtitleOcrHelp:
        'PGS- und DVD-Untertitel (VobSub) sind Bilder, und viele Apps akzeptieren nur Text: Für sie brennt Polyfin die Spur ins Video ein, was eine Konvertierung erfordert. Ist dies aktiviert, liest Tesseract jede Bildspur einmal im Hintergrund in eine Textkopie ein, die als zusätzliche Spur mit der Markierung „(OCR)“ aufgeführt wird und ohne Konvertierung abgespielt wird. Bis eine Kopie fertig ist, wird die Spur wie bisher eingebrannt. Die Bildspur bleibt erhalten: Kursivschrift, Farben und Positionen gehen verloren, und verzierte Schriftarten können falsch gelesen werden. „Bei Wiedergabe“ liest die Spuren einer Version, sobald eine App, die sie nicht darstellen kann, sie zum ersten Mal abspielt; „Auch nach Bibliotheks-Scans“ liest zusätzlich die Spuren neuer lokaler Dateien. Standardmäßig aus.',
      subtitleOcrChoices: {
        off: 'Aus',
        played: 'Bei Wiedergabe',
        scans: 'Auch nach Bibliotheks-Scans',
      },
      subtitleOcrTracks: 'Gleichzeitig gelesene Bilduntertitelspuren',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Jede Spur wird von einem Tesseract-Prozess gelesen, der einen Prozessorkern nutzt. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
      hardwareAcceleration: 'Grafikkarte für die Konvertierung',
      hardwareAccelerationHelp:
        'Eine Grafikkarte (GPU) konvertiert Video viel schneller als der Prozessor. Jedes System bietet eigene Optionen: NVIDIA und VAAPI unter Linux, VideoToolbox unter macOS, NVIDIA, Intel und AMD unter Windows. Ohne funktionierende Karte konvertiert der Prozessor. Eine Änderung gilt nach dem Speichern, ohne Neustart.',
      vaapiDevice: 'Grafikkarte für VAAPI',
      vaapiDeviceHelp:
        'Der Render-Node, auf dem VAAPI Video konvertiert, wenn mehrere Grafikkarten infrage kommen. Standardmäßig jede der Reihe nach, die erste, die funktioniert.',
      vaapiDeviceEach: 'Jede der Reihe nach',
      vaapiDeviceMissing: (path: string) => `${path} (nicht gefunden)`,
      hardware: {
        auto: 'Automatisch: die erste, die funktioniert, in der Reihenfolge unten',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD oder Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'Keine: nur der Prozessor',
      },
      detected: 'Auf diesem Server erkannt',
      noGpu: 'Es wird keine Grafikkarte verwendet: Der Prozessor konvertiert das Video.',
      gpu: 'Grafikkarte',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD oder Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Encoder auf der Karte',
      gpuToneMapping: 'HDR zu SDR auf der Karte',
      processorToneMappingHint:
        'HDR wird dann vom Prozessor konvertiert, bis zu „Maximale Qualität von HDR, das vom Prozessor konvertiert wird“.',
      qualityFactor: 'Akzeptiert einen Qualitätswert',
      processor: 'Encoder auf dem Prozessor',
      processorToneMapping: 'HDR zu SDR auf dem Prozessor',
      yes: 'Ja',
      no: 'Nein',
      none: 'Keine',
      hardwareDecoding: 'Diese Formate auf der Grafikkarte lesen',
      hardwareDecodingHelp:
        'Die Karte liest (dekodiert) diese Formate selbst, wodurch der Prozessor entlastet wird. Entferne den Haken bei einem Format, wenn seine Videos nicht konvertiert werden oder fehlerhaft aussehen: Dann liest der Prozessor es. Andere Formate werden auf der Karte versucht. HEVC 10-Bit erfordert HEVC.',
      hardwareDecodingNoGpu:
        'Es wird keine Grafikkarte verwendet, daher liest der Prozessor alle Formate.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10-Bit',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Kodiergeschwindigkeit',
      encoderPresetHelp:
        'Langsamer ergibt ein besseres Bild bei gleicher Größe, braucht aber mehr Leistung. Wenn konvertierte Videos ruckeln, wähle eine schnellere Stufe. Automatisch behält die Wahl von Polyfin bei: sehr schnell auf dem Prozessor, mittel auf NVIDIA-Karten, die eigene der Karte bei den anderen.',
      presets: {
        auto: 'Automatisch',
        veryslow: 'Sehr langsam (bestes Bild)',
        slower: 'Langsamer',
        slow: 'Langsam',
        medium: 'Mittel',
        fast: 'Schnell',
        faster: 'Schneller',
        veryfast: 'Sehr schnell',
        superfast: 'Superschnell',
        ultrafast: 'Ultraschnell (geringste Last)',
      },
      h264Quality: 'H.264-Qualität (0 = nach Bitrate)',
      hevcQuality: 'HEVC-Qualität (0 = nach Bitrate)',
      qualityHelp: (r: RangeText) =>
        `Eine niedrigere Zahl ergibt ein besseres Bild und mehr Daten; die Bitrate bleibt das Maximum. 0 richtet sich nur nach der Bitrate. Jellyfin verwendet 23 für H.264 und 28 für HEVC. 0 oder von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
      qualityIgnored:
        'Diese Grafikkarte kann keinen Qualitätswert verwenden: Sie richtet sich weiter nach der Bitrate.',
      allowHevcEncoding: 'Konvertierung in HEVC erlauben',
      allowHevcEncodingHelp:
        'HEVC braucht für dasselbe Bild weniger Daten als H.264, die Erzeugung kostet aber mehr Leistung. Wenn aktiviert, erhalten Apps, die HEVC an erster Stelle nennen, HEVC. Wenn deaktiviert, erhalten nur Apps HEVC, die kein H.264 abspielen können.',
      noHevcEncoder: 'FFmpeg kann auf diesem Server kein HEVC erzeugen.',
      adaptiveQuality: 'Mehrere Qualitätsstufen beim Konvertieren',
      adaptiveQualityHelp:
        'Konvertierte Videos bieten zusätzlich bis zu zwei niedrigere Qualitätsstufen an, etwa 720p und 480p unter 1080p. Apps, die wechseln können, schalten darauf herunter, wenn ihre Verbindung langsamer wird, statt zum Laden anzuhalten. Eine Qualitätsstufe, die die App verlässt, wird noch 10 Sekunden weiter konvertiert, damit ein Zurückwechseln nicht von vorn beginnt, und alle zählen als eine Konvertierung. Kopierte Videos, Live-TV und Musik behalten eine Qualitätsstufe. „Lokales Netzwerk“ bedeutet dasselbe wie bei PINs zum Zurücksetzen des Passworts, über die bekannten Proxys. Standardmäßig aus.',
      adaptiveQualities: { off: 'Aus', remote: 'Außerhalb des lokalen Netzwerks', always: 'Immer' },
      toneMapping: 'HDR in SDR konvertieren (Tone-Mapping)',
      toneMappingHelp:
        'Erhält die richtigen Farben und Helligkeit von HDR-Videos auf Bildschirmen, die nur SDR darstellen. Wenn deaktiviert, wirken konvertierte HDR-Videos blass, und Dolby-Vision-Videos ohne HDR10-Ebene werden nicht konvertiert.',
      toneMappingUnavailable:
        'Weder die Grafikkarte noch FFmpeg können das auf diesem Server: HDR-Videos werden nur konvertiert, wenn diese Option deaktiviert ist.',
      toneMappingAlgorithm: 'Tone-Mapping-Methode',
      toneMappingAlgorithmHelp:
        'Wie helle Bereiche auf SDR heruntergeregelt werden. Automatisch verwendet BT.2390 auf NVIDIA-Karten und Hable auf dem Prozessor. Intel-Karten und VideoToolbox verwenden ihre eigene Methode und ignorieren diese Auswahl.',
      algorithms: {
        auto: 'Automatisch',
        bt2390: 'BT.2390 (nur NVIDIA-Karten)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Abschneiden',
        linear: 'Linear',
      },
      gpuToneMappingSetting: 'HDR-Tone-Mapping auf der Grafikkarte',
      gpuToneMappingSettingHelp:
        'NVIDIA-Karten, Intel-Karten (über VAAPI unter Linux, QSV unter Windows) und VideoToolbox unter macOS konvertieren HDR selbst in SDR, bis 4K. Deaktiviere diese Option, wenn ihre Farben falsch aussehen: Dann übernimmt das der Prozessor, in geringerer Qualität. Intel-Karten nehmen nur HDR10-Videos an, die Angaben zu ihrem Mastering-Display enthalten, und VideoToolbox HDR10 und HLG; die übrigen konvertiert der Prozessor, und Dolby Vision ohne HDR10-Ebene wird nur auf NVIDIA-Karten konvertiert. AMD-Karten tun es nie: Ihr Linux-Treiber ist bei jedem Weg, den Polyfin versucht hat, gescheitert oder hängen geblieben, und der AMF-Filter von FFmpeg unter Windows beherrscht kein Tone-Mapping.',
      gpuToneMappingUnavailable:
        'Die Grafikkarte dieses Servers kann das nicht: Der Prozessor konvertiert HDR in SDR.',
      processorToneMappingHeight: 'Maximale Qualität von HDR, das der Prozessor konvertiert',
      processorToneMappingHeightHelp:
        'HDR in SDR zu konvertieren ist schwere Arbeit für den Prozessor: etwa das Vierfache der eigentlichen Kodierung bei 1080p. Automatisch misst diesen Server beim Start: 1080p, wenn er mindestens anderthalbmal so schnell konvertiert, wie das Video abgespielt wird, sonst 720p, was auch bis zum Ende der Messung gilt. Mehr als 1080p zählt nur, wenn eine Grafikkarte das Video kodiert: Allein hört der Prozessor bei 1080p auf.',
      toneMappingHeightAuto: 'Automatisch (beim Start gemessen)',
      height4k: '4K',
      measuredHeight: (height: number) => `Gemessen: ${height}p`,
      upToHeight: (height: number) => `bis ${height}p`,
      toneMappingPeak: 'Spitzenhelligkeit in Nits (0 = aus dem Video)',
      toneMappingDesat: 'Entsättigung heller Bereiche (0 = aus)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Nur wenn der Prozessor HDR in SDR konvertiert: Eine Grafikkarte, die das übernimmt, ignoriert diese Werte. Die Spitzenhelligkeit ersetzt die höchste Helligkeit, die das Video laut eigenen Angaben erreicht: 0 oder von ${peak.min} bis ${peak.max}; standardmäßig ${peak.default}. Die Entsättigung schwächt die Farbe sehr heller Bereiche ab: von ${desat.min} bis ${desat.max}; standardmäßig ${desat.default}.`,
      processorCannotToneMap:
        'FFmpeg kann auf diesem Server kein Tone-Mapping auf dem Prozessor ausführen.',
      deinterlaceMethod: 'Deinterlacing-Methode',
      deinterlaceMethodHelp:
        'TV-Sendungen und DVDs sind oft im Zeilensprungverfahren aufgenommen, was nach dem Konvertieren kammartige Linien zeigt. Yadif ist schnell; Bwdif ist etwas schärfer.',
      noBwdif: 'FFmpeg auf diesem Server hat kein Bwdif.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Bildrate verdoppeln',
      deinterlaceDoubleRateHelp:
        'Erzeugt aus jedem Halbbild ein eigenes Bild, für flüssigere Bewegungen bei Sport und TV-Sendungen. Nur für Videos mit bis zu 30 Bildern pro Sekunde.',
      downmixAlgorithm: 'Stereo-Downmix',
      downmixAlgorithmHelp:
        'Wie Surround-Sound auf zwei Lautsprecher heruntergemischt wird. Dave750 und Nachtmodus halten Stimmen klar; RFC 7845 und AC-4 folgen Standards.',
      downmixes: {
        None: 'Eigener Mix von FFmpeg',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Nachtmodus (klarere Stimmen)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Lautstärke beim Stereo-Downmix',
      downmixBoostHelp: (r: RangeText) =>
        `Stereo-Mixe klingen oft leiser: Die Lautstärke wird mit dieser Zahl multipliziert. Von ${r.min} bis ${r.max}; standardmäßig ${r.default} (keine Änderung). Jellyfin verwendet 2.`,
      maxAudioChannels: 'Maximale Anzahl an Audiokanälen',
      maxAudioChannelsHelp:
        'Konvertierter Ton behält höchstens so viele Kanäle, auch wenn die App mehr annimmt.',
      audioChannels: (channels: number): string =>
        channels === 0
          ? 'So viele, wie die App annimmt'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Stereo'
              : '5.1',
      audioBitratePerChannel: 'Audiobitrate pro Kanal in kb/s (0 = automatisch)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automatisch ergibt die Bitraten von Jellyfin: 128 kb/s bei Mono, 384 kb/s bei Stereo und 640 kb/s bei 5.1, weniger unter einer niedrigen Bitratengrenze. 0 oder von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
      encodingThreads: 'Prozessor-Threads pro Konvertierung (0 = automatisch)',
      encodingThreadsHelp: (r: RangeText) =>
        `Begrenzt, wie viel vom Prozessor eine Konvertierung nutzt, um Platz für andere Arbeit zu lassen. Von ${r.min} bis ${r.max}; bei 0 wählt FFmpeg.`,
      aheadSeconds: 'Im Voraus vorbereitete Sekunden',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin bereitet ein Video höchstens so viele Sekunden über die von der App angeforderte Stelle hinaus vor und wartet dann. Mehr hilft bei langsamen Quellen und hält deren Verbindung ausgelastet, verbraucht aber mehr Leistung und Speicherplatz, wenn früh mit dem Schauen aufgehört wird. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
      keepConvertedVideo: 'Konvertiertes Video beim Wechsel der Audiospur behalten',
      keepConvertedVideoHelp:
        'Konvertiert Bild und Ton getrennt, sodass bei Auswahl einer anderen Audiospur im Player nur der neue Ton vorbereitet wird: Das bereits konvertierte Bild läuft weiter, und die neue Spur startet schneller. Das Bild wird 30 Sekunden aufbewahrt, nachdem ein Player es beendet, und zählt in dieser Zeit als eine Konvertierung.',
    },
    catalogsTitle: 'Kataloge',
    catalogLimit: 'Gelesene Titel pro Film- und Serienkatalog',
    catalogLimitHelp: (r: RangeText) =>
      `Manche Kataloge sind nahezu endlos, daher hört Polyfin nach dieser Anzahl an Titeln auf, einen Katalog zu lesen. Eine höhere Zahl zeigt mehr Titel, aber Listen laden langsamer und das Add-on erhält mehr Anfragen. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    channelLimit: 'Gelesene Kanäle pro Live-TV-Katalog',
    channelLimitHelp: (r: RangeText) =>
      `Polyfin hört nach dieser Anzahl an Kanälen auf, einen Live-TV-Katalog zu lesen, und liest für das Fernsehprogramm höchstens so viele Sendungen pro Tag. Eine höhere Zahl zeigt mehr, lädt aber langsamer und das Add-on erhält mehr Anfragen. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    skipButtons: 'Schaltflächen zum Überspringen von Intro und Abspann',
    skipButtonsHelp:
      'Apps bieten an, Intros, Rückblicke und Abspanne zu überspringen, die in Community-Datenbanken gefunden werden. Wenn aus, zeigen Apps keine Überspringen-Schaltflächen und die Datenbanken werden nicht abgefragt.',
    publicMetaDbKey: 'PublicMetaDB-Schlüssel',
    publicMetaDbKeyHelp:
      'Optional. Ein PublicMetaDB-API-Schlüssel fügt PublicMetaDB als Quelle für Sprungmarken hinzu.',
    theIntroDbKey: 'TheIntroDB-Schlüssel (optional)',
    theIntroDbKeyHelp:
      'Erhöht das tägliche Limit von TheIntroDB und bezieht deine eigenen Einreichungen ein. Lesen funktioniert auch ohne.',
    segmentSources: {
      label: 'Quellen für Sprungmarken',
      help: 'Für jede Art von Abschnitt (Intro, Rückblick, Abspann, Vorschau) gewinnt die erste aktivierte Quelle in dieser Liste, die ihn kennt; die nächsten füllen die Lücken. Deaktivierte Quellen werden nie abgefragt.',
      turnOn: (name: string) => `${name} abfragen`,
      needsKey: 'Schlüssel erforderlich',
    },
    audioIntros: 'Intros und Abspanne im Ton lokaler Folgen finden',
    audioIntrosHelp:
      'Wenn eine Folge aus einem lokalen Ordner ohne Intro oder Abspann aus ihren Kapiteln oder den obigen Quellen abgespielt wird, vergleicht Polyfin im Hintergrund den Ton der Folgen ihrer Staffel und weist jeder den gemeinsamen Abschnitt nahe dem Anfang als Intro und den gemeinsamen Abschnitt nahe dem Ende als Abspann zu. Die übrigen Folgen der Staffel haben ihre dann sofort, die abgespielte ab ihrer nächsten Wiedergabe. Erfordert die aktivierten Überspringen-Schaltflächen.',
    similarTitles: 'Ähnliche Titel',
    similarTitlesHelp:
      'Die Seite eines Titels listet ähnliche Titel aus den Katalogen der Add-ons auf. Wenn aus, ist die Liste leer und die Add-ons erhalten weniger Anfragen.',
    generateImages: 'Bilder für Sammlungen ohne Bild erzeugen',
    generateImagesHelp:
      'Eine Sammlung eines Add-ons ohne Poster sowie eine als Sammlung angezeigte Intelligente Liste zeigen die Poster ihrer ersten Titel mit ihrem Namen, von Polyfin gezeichnet, sobald ihre Titel gelesen wurden. Benutzer mit Kindersicherung oder gesperrten Genres sehen sie nicht. Wenn aus, zeigen sie kein Bild. Bibliotheken, die auf ein erzeugtes Bild eingestellt sind, behalten es.',
    tmdbKey: 'TMDB-API-Schlüssel',
    tmdbKeyHelp:
      'Optional. Mit einem API-Schlüssel oder einem Lesezugriffstoken eines TMDB-Kontos durchsucht „Bilder bearbeiten“ in jellyfin-web auch TMDB nach Postern, Hintergründen und Logos eines Titels, Standbildern einer Folge, Postern einer Staffel und Fotos einer Person, die Polyfin weiterleitet. Ohne Schlüssel listet „Bilder bearbeiten“ nur die Bilder der Add-ons auf und TMDB wird nie abgefragt.',
    mdblistRatingsKey: 'MDBList-API-Schlüssel für Bewertungen',
    mdblistRatingsKeyHelp:
      'Optional. Mit einem Schlüssel eines MDBList-Kontos zeigen Filme und Serien ihre IMDb-Bewertung, Kritikerwertung und Publikumswertung von MDBList, im Hintergrund abgerufen und für alle Benutzer gespeichert. Die eigenen MDBList-Schlüssel der Benutzer für das Tracking werden dafür nie verwendet. Ohne Schlüssel wird MDBList nie abgefragt.',
    criticRatingShows: 'Kritikerbewertung zeigt',
    criticRatingShowsHelp:
      'Jellyfin-Apps haben ein einziges Feld für eine Kritikerbewertung von 0 bis 100: Es zeigt die Kritikerwertung oder die Publikumswertung. Auf der Seite eines Titels zeigt der Web-Player beide, zusammen mit der IMDb-Bewertung.',
    criticRatingChoices: { critics: 'Kritikerwertung', audience: 'Publikumswertung' },
    lyrics: 'Songtexte von LRCLIB',
    lyricsHelp:
      'Apps zeigen die Texte von Songs, Zeile für Zeile synchronisiert, wenn LRCLIB, eine freie Songtext-Datenbank, sie hat. Polyfin sucht einen Song einmalig anhand von Interpret, Titel, Album und Länge, wenn er abgespielt wird oder seine Details oder sein Text geöffnet werden. Wenn aus, haben Songs keine Texte und LRCLIB wird nie abgefragt.',
    evenVolume: 'Lautstärke zwischen Songs angleichen',
    evenVolumeHelp:
      'Apps heben oder senken jeden Song oder jedes Album auf dieselbe Lautheit, anhand der ReplayGain-Tags lokaler Songs oder ihrer gemessenen Lautheit. Die Songs selbst werden nicht verändert. Wenn aus, erhalten Apps keine Verstärkung und es wird nichts gemessen.',
    measureLoudness: 'Lautheit von Songs ohne Verstärkungswert messen',
    measureLoudnessHelp:
      'FFmpeg misst die Lautheit der Songs ohne ReplayGain-Tag, jeweils zwei gleichzeitig mit niedrigster Priorität und jeden nur einmal: lokale Songs nach einem Scan, Add-on-Songs nach ihrer ersten Wiedergabe. Songs über 15 Minuten und Hörbücher werden nie gemessen.',
    measureLoudnessChoices: { off: 'Aus', local: 'Lokale Songs', all: 'Lokale und Add-on-Songs' },
    playedPercent: 'Als gesehen markiert nach (%)',
    playedPercentHelp: (r: RangeText) =>
      `Ein Titel wird als gesehen markiert, sobald die Wiedergabe diesen Anteil seiner Länge überschreitet. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}, wie bei Jellyfin.`,
    resumePercent: 'Fortsetzungspunkt gespeichert nach (%)',
    resumePercentHelp: (r: RangeText) =>
      `Die Stelle, an der die Wiedergabe gestoppt wurde, wird zum Fortsetzen gespeichert, sobald dieser Anteil der Länge eines Titels überschritten ist. Der Wert muss niedriger sein als die Schwelle für „gesehen“. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}, wie bei Jellyfin. Ein Titel unter 5 Minuten wird als gesehen markiert, sobald dieser Punkt überschritten ist.`,
    versionListMinutes: 'Versionslisten behalten für (Minuten)',
    versionListMinutesHelp: (r: RangeText) =>
      `Wie lange Polyfin die von den Add-ons für einen Titel gelisteten Versionen und Untertitel verwendet, bevor es erneut nachfragt. Eine ältere Liste wird beim erneuten Öffnen des Titels trotzdem sofort angezeigt, während Polyfin nachfragt. Längeres Behalten sendet weniger Anfragen an das Stream-Add-on, was bei Anbietern hilft, die zu viele Anfragen ablehnen, aber neue Versionen erscheinen später. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    catalogRefreshMinutes: 'Kataloge aktualisieren nach (Minuten)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `Wie alt eine von einem Add-on gelesene Katalogseite, einschließlich des Live-TV-Fernsehprogramms, werden darf, bevor Polyfin sie erneut liest. Apps warten nie darauf: Eine ältere Seite wird trotzdem sofort angezeigt, während Polyfin sie im Hintergrund neu liest, und sie wird noch einen Tag länger behalten. Eine längere Zeit sendet weniger Anfragen an die Add-ons, aber neue Titel erscheinen später. Von ${r.min} bis ${r.max} (ein Tag); standardmäßig ${r.default}.`,
    collectionReadHour: 'Alle Sammlungen täglich lesen',
    collectionReadHourNever: 'Nie',
    collectionReadHourHelp:
      'Liest zu dieser Uhrzeit nacheinander alle Sammlungen der Sammlungsbibliotheken des Servers, dann die Kataloge der Intelligenten Listen des Servers, damit sie sofort öffnen. Dabei werden viele Seiten von den Add-ons abgefragt: Lass es bei einem Add-on, das du mit anderen teilst, ausgeschaltet.',
    collectionLibraryApps: 'Apps, die Sammlungsbibliotheken nach Sammlung anzeigen',
    collectionLibraryAppsHelp:
      'Sammlungsbibliotheken werden Apps als Bibliotheken mit gemischten Inhalten gemeldet, damit jellyfin-web und Streamyfin ihnen eine Zeile auf dem Startbildschirm geben. Den angehakten Apps wird mitgeteilt, dass sie Sammlungen enthalten, wie es die Sammlungsansicht von Jellyfin tut: Apps, die eine Bibliothek nach ihrem Typ auflisten, zeigen dann ihre Sammlungen und nach dem Öffnen die Titel einer Sammlung, statt die Titel aller Sammlungen zusammen. jellyfin-web und Streamyfin lassen solche Bibliotheken in den Zeilen ihres Startbildschirms weg, hake sie also nicht an. Die Liste zeigt die Apps, die sich bei diesem Server angemeldet haben.',
    collectionLibraryAppsNone: 'Bisher hat sich keine App bei diesem Server angemeldet.',
    localScanHours: 'Lokale Ordner scannen alle (Stunden, 0 = nie)',
    localScanHoursHelp: (r: RangeText) =>
      `Wie oft jeder lokale Ordner erneut nach hinzugefügten, geänderten oder entfernten Dateien gescannt wird. Ordner werden außerdem beim Start, auf Anforderung und bei Änderungen an überwachten Ordnern gescannt. 0 schaltet den Zeitplan aus. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    watchLocalFolders: 'Ordner und Freigaben auf Änderungen überwachen',
    watchLocalFoldersHelp: {
      container:
        'Scannt einen Ordner im Container erneut, wenige Sekunden nachdem sich seine Dateien nicht mehr ändern, sodass hinzugefügte, umbenannte oder entfernte Dateien innerhalb einer Minute erscheinen.',
      computer:
        'Scannt einen Ordner auf dem Computer, auf dem Polyfin läuft, erneut, wenige Sekunden nachdem sich seine Dateien nicht mehr ändern, sodass hinzugefügte, umbenannte oder entfernte Dateien innerhalb einer Minute erscheinen.',
    },
    watchSharesHelp:
      'Eine SMB-Freigabe wird wenige Sekunden, nachdem ihr Server eine Änderung meldet, erneut gescannt. WebDAV-Ordner werden alle 2 Minuten neu aufgelistet, S3-Buckets, SFTP-Ordner und rclone-Remotes so oft, wie jeder Ordner es verlangt, eine Freigabe mit vielen Ordnern in mehreren Durchgängen, und erneut gescannt, wenn sich eine Auflistung geändert hat. System › Systemzustand zeigt, wie jede Freigabe überwacht wird und welche Ordner oder Freigaben nicht überwacht werden können und warum.',
    personalAddons: 'Eigene Add-ons der Benutzer erlauben',
    personalAddonsHelp:
      'Erlaubt Benutzern, zusätzlich zu denen des Servers eigene Stremio-Add-ons hinzuzufügen. Wenn aus, werden ihre Add-ons behalten, aber nicht verwendet, und ihre Jellyfin-Apps zeigen nur die Add-ons des Servers.',
    serverImports:
      'Benutzer dürfen ihren Wiedergabeverlauf von einem anderen Server oder einer App importieren',
    serverImportsHelp:
      'Unter „Mein Konto“ kann jeder Benutzer seine gesehenen Filme und Folgen, Fortsetzungspunkte und Favoriten von einem Jellyfin- oder Emby-Server oder aus seinem Stremio- oder Nuvio-Konto importieren, indem er sich dort selbst anmeldet. Wenn aus, sind diese Bereiche ausgeblendet. Ein Administrator kann unter „Benutzer“ weiterhin für einen Benutzer importieren.',
    importEveryMinutes: 'Verbundene Konten importieren alle',
    importEveryMinutesHelp:
      'Wie oft Polyfin die Stremio- und Nuvio-Konten, mit denen Benutzer verbunden bleiben, sowie die Wiedergabeverläufe, die sie aus Tracking-Diensten importieren, erneut importiert. Eine Änderung gilt sofort: Jeder nächste Import erfolgt so lange nach dem letzten. Jeder Import liest das ganze Konto erneut, daher sendet ein kurzes Intervall mehr Anfragen an diese Dienste und verstärkt bei Stremio, dessen Bedingungen automatisierte Extraktion verbieten, diese Grauzone. Standardmäßig 6 Stunden.',
    shareLinks: 'Freigabelinks erlauben',
    shareLinksHelp:
      'Benutzer mit „Kann Titel teilen“ (auf ihrer Seite unter Benutzer) können einen Film oder eine Folge an jemanden ohne Konto weitergeben, über einen Link, der nach 1 Stunde bis 30 Tagen abläuft. Der Gast schaut im Browser, mit höchstens 1080p und innerhalb der Qualitätsgruppe des Benutzers, und zählt sofort zu den Wiedergaben des Benutzers. Wenn aus, kann kein Link erstellt werden und jeder Link wird abgelehnt, ohne gelöscht zu werden. Standardmäßig aus.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Konto nach so vielen falschen Passwörtern sperren (0 = nie)',
    loginAttemptsHelp: (r: RangeText) =>
      `Nach so vielen falschen Passwörtern in Folge kann sich das Konto 15 Minuten lang nicht anmelden, selbst mit dem richtigen Passwort. Ein Administrator kann es auf der Seite Benutzer früher entsperren. 0 oder von ${r.min} bis ${r.max}.`,
    twoStepAdministrators: 'Zweistufige Anmeldung für Administratoren verlangen',
    twoStepAdministratorsHelp:
      'Administratoren melden sich bei dieser Admin-App nach ihrem Passwort mit einem Code aus einer Authenticator-App an. Wer sie noch nicht eingerichtet hat, tut dies direkt nach dem Passwort, bevor eine Sitzung entsteht. Ihr Passwort allein meldet sie dann bei keiner Jellyfin-App mehr an: Sie nutzen Quick Connect, hier bestätigt. Bereits angemeldete Sitzungen und Geräte bleiben angemeldet. Standardmäßig aus.',
    inactiveDeviceDays: 'Geräte abmelden, die nicht genutzt wurden seit (Tage, 0 = nie)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Jellyfin-Apps, die so viele Tage nicht genutzt wurden, werden abgemeldet und müssen sich erneut anmelden. Dies wird stündlich geprüft. Die Anmeldung an dieser Admin-Seite ist nicht betroffen. Von ${r.min} bis ${r.max}.`,
    knownProxies: 'Bekannte Proxys',
    knownProxiesHelp: (r: RangeText) =>
      `Die Reverse-Proxys vor Polyfin, einer pro Zeile: eine IP-Adresse, ein Netzwerk wie 172.18.0.0/16 oder ein Hostname wie der Name eines Containers in einem Docker-Netzwerk (traefik), bis zu ${r.max}. Ein Name wird jede Minute aufgelöst und jeder Adresse, zu der er auflöst, wird vertraut, also trage nur Namen ein, die dein eigenes Netzwerk auflöst. Eine Anfrage von einem davon stammt von dem Client, den ihr X-Forwarded-For- oder X-Real-IP-Header nennt: Diese Adresse begrenzt fehlgeschlagene Anmeldungen, wird bei Geräten angezeigt und bestimmt das lokale Netzwerk. Weitergeleitete Header von jeder anderen Adresse werden ignoriert. Lass das Feld leer, wenn nichts vor Polyfin steht.`,
    https: {
      title: 'HTTPS',
      intro:
        'Polyfin kann HTTPS selbst bereitstellen, auf einem eigenen Port, ohne vorgeschalteten Reverse-Proxy. HTTP funktioniert auf seinem Port weiter, egal was mit HTTPS passiert: Ein Port, der nicht geöffnet werden kann, oder ein Zertifikat, das nicht gelesen oder erneuert werden kann, erscheint unter System › Systemzustand. Änderungen gelten sofort, ohne Neustart.',
      mode: 'HTTPS bereitstellen',
      modeHelp:
        'Zertifikatsdateien: ein Zertifikat, das ein anderes Tool bezieht, wird bei Änderungen erneut gelesen. Let’s Encrypt: ein Zertifikat, das Polyfin selbst bezieht und erneuert, für öffentliche Hostnamen, die auf diesen Server zeigen. Standardmäßig aus.',
      modes: { off: 'Aus', files: 'Zertifikatsdateien', letsencrypt: 'Let’s Encrypt' },
      port: 'HTTPS-Port',
      portHelp: (r: RangeText) =>
        `Der TCP-Port, auf dem HTTPS im Container lauscht: Veröffentliche ihn, so wie 8096 für HTTP. Der Benutzer des Containers kann nicht unter 1024 lauschen: Um Polyfin auf 443 zu erreichen, leite 443 am Router oder in den Ports des Containers auf diesen Port weiter und setze die öffentliche Adresse unter Einstellungen › Benachrichtigungen auf https:// und den Hostnamen, ohne Port. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}, wie bei Jellyfin.`,
      redirect: 'HTTP auf HTTPS umleiten',
      redirectHelp:
        'Anfragen aus dem Internet über HTTP erhalten eine dauerhafte Umleitung (308) auf dieselbe Adresse über HTTPS, auf dem Port, über den das Internet HTTPS erreicht: der Port der öffentlichen Adresse unter Einstellungen › Benachrichtigungen, wenn sie mit https:// beginnt, 443, wenn sie keinen nennt, sonst der HTTPS-Port. Das lokale Netzwerk, die Prüfungen von Let’s Encrypt und Anfragen, die ein bekannter Proxy über HTTPS empfangen hat, bleiben bei HTTP. Gilt nur, solange HTTPS bereitgestellt wird. Standardmäßig aus.',
      certificate: 'Zertifikat',
      certificateHelp:
        'Der Pfad im Container zur PEM-Datei des Zertifikats mit seiner Kette, z. B. /certs/fullchain.pem. Beim Speichern wird geprüft, ob sie lesbar ist. Polyfin liest sie innerhalb einer Minute nach einer Änderung erneut und behält das letzte gültige Zertifikat, solange eine Änderung nicht gelesen werden kann.',
      privateKey: 'Privater Schlüssel',
      privateKeyHelp:
        'Der Pfad im Container zum privaten RSA- oder ECDSA-Schlüssel des Zertifikats als PEM-Datei, z. B. /certs/privkey.pem. Beim Speichern wird geprüft, ob er zum Zertifikat gehört.',
      hosts: 'Hostnamen',
      hostsHelp: (r: RangeText) =>
        `Die öffentlichen Namen, die auf diesen Server zeigen, einer pro Zeile, z. B. media.example.org, bis zu ${r.max}. Ein Zertifikat enthält sie alle. Namen, die nur im lokalen Netzwerk aufgelöst werden, und Platzhalter benötigen die DNS-01-Challenge, die Polyfin nicht beantwortet: Verwende für sie Zertifikatsdateien aus einem anderen Tool.`,
      email: 'E-Mail',
      emailHelp: 'Optional. Let’s Encrypt kann dir an diese Adresse zum Konto schreiben.',
      terms: 'Ich stimme den Nutzungsbedingungen von Let’s Encrypt zu',
      termsHelp:
        'Let’s Encrypt stellt Zertifikate erst aus, wenn den Nutzungsbedingungen unter letsencrypt.org/repository zugestimmt wurde.',
      challenge: 'Challenge',
      challengeHelp:
        'Wie Let’s Encrypt prüft, dass die Namen auf diesen Server zeigen. HTTP-01: Polyfin antwortet auf seinem HTTP-Port, den Port 80 aus dem Internet erreichen muss. TLS-ALPN-01: Polyfin antwortet auf seinem HTTPS-Port, den Port 443 aus dem Internet erreichen muss. Das Zertifikat, sein Schlüssel und der Schlüssel des Kontos werden in der Datenbank gespeichert, die Schlüssel mit POLYFIN_SECRET_KEY verschlüsselt, und das Zertifikat wird erneuert, sobald ein Drittel seiner Laufzeit übrig ist.',
      challenges: {
        'http-01': 'HTTP-01 (Port 80 auf den HTTP-Port)',
        'tls-alpn-01': 'TLS-ALPN-01 (Port 443 auf den HTTPS-Port)',
      },
      useAddress: (address: string) => `${address} verwenden`,
      offeredAddress:
        'Die Adressen, unter denen Let’s Encrypt HTTPS bereitstellt, unter Einstellungen › Sicherheit: ohne Port, sobald der Router 443 auf den HTTPS-Port weiterleitet. „HTTP auf HTTPS umleiten“ führt zum Port der gewählten Adresse.',
    },
    detailedLog: 'Ausführliches Protokoll (zur Diagnose eines Problems)',
    detailedLogHelp:
      'Polyfin schreibt viel mehr in sein Protokoll, sofort und ohne Neustart. Schalte es aus, sobald das Problem gefunden ist.',
    saved: 'Einstellungen gespeichert.',
    thumbnailsHelp:
      'Bilder aus den Titeln selbst, nachdem sie angesehen wurden. Dafür liest Polyfin kleine Teile der Datei von der Quelle, in gemächlichem Tempo, sobald niemand von dieser Quelle abspielt: höchstens 60 Anfragen pro Titel, eine alle 3 Sekunden, und 120 pro Stunde und Quelle, nie die ganze Datei. Eine Quelle, die Polyfin bittet, langsamer zu werden, erhält 2 Stunden lang keine Bildanfragen. Standardmäßig aus.',
    trickplay: 'Vorschaubilder beim Spulen durch einen Titel',
    trickplayHelp:
      'Apps zeigen in der Wiedergabeleiste ein kleines Bild der Stelle, zu der der Benutzer springt. Polyfin erstellt sie, sobald ein Titel angesehen wurde, und für die nächste Folge, wenn die Wiedergabe im Voraus vorbereitet wird. Lange Filme erhalten gröbere Vorschaubilder: einige Minuten statt einiger Sekunden Abstand.',
    trickplayInterval: 'Ein Vorschaubild alle (Sekunden, mindestens)',
    trickplayIntervalHelp: (r: RangeText) =>
      `Wie oft die Wiedergabeleiste das Bild wechselt. Polyfin liest höchstens 60 Bilder pro Titel, daher erhält ein Titel, der länger als etwa 10 Minuten ist, ein Bild pro Schritt, mit gleichmäßig über die Laufzeit verteilten Schritten: etwa 1 Minute Abstand bei einer einstündigen Folge, 2 Minuten bei einem zweistündigen Film. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}, wie bei Jellyfin.`,
    trickplayWidth: 'Breite der Vorschaubilder',
    trickplayWidthHelp:
      'Breitere Vorschaubilder wirken auf großen Bildschirmen schärfer, brauchen aber mehr Platz. Eine Änderung erzeugt neue Vorschaubilder, wenn ein Titel das nächste Mal abgespielt wird.',
    pixels: (width: number) => `${width} Pixel`,
    chapterImages: 'Kapitelbilder',
    chapterImagesHelp:
      'Apps zeigen für jedes Kapitel eines Titels ein Bild in der Szenenliste. Polyfin verwendet das gelesene Bild, das dem Beginn jedes Kapitels am nächsten liegt, aus denselben Lesevorgängen wie die Vorschaubilder, sobald der Titel angesehen wurde.',
    thumbnailStorage: 'Speicherplatz für Bilder (GB)',
    thumbnailStorageHelp: (r: RangeText) =>
      `Vorschau- und Kapitelbilder werden in der Datenbank gespeichert. Über dieser Größe werden die Bilder der am längsten nicht mehr angesehenen Titel entfernt; sie werden neu erstellt, wenn der Titel erneut abgespielt wird. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    cacheSize: 'Speicherplatz für gelesene Dateien (GB)',
    cacheSizeHelp: (r: RangeText) =>
      `Speicherplatz, den Polyfin nutzen darf, um die Teile der abgespielten Dateien zu behalten, damit Spulen und Neustarts sie nicht erneut herunterladen. Teile, die in den letzten 30 Sekunden gelesen wurden, werden auch darüber hinaus behalten. Von ${r.min} bis ${r.max} GB; standardmäßig ${r.default}.`,
    workFolders: {
      conversionFolder: 'Konvertierungsordner',
      conversionFolderHelp:
        'Wohin die Segmente konvertierter oder neu verpackter Dateien während ihrer Wiedergabe geschrieben werden: die meisten Schreibzugriffe, für die sich eine schnelle Festplatte oder ein Ordner im Arbeitsspeicher eignet. Er braucht Platz für die Segmente aller laufenden Konvertierungen.',
      readingFolder: 'Ordner für gelesene Dateien',
      readingFolderHelp:
        'Wo die Teile der Dateien von Add-ons und die Stücke von Torrents während der Wiedergabe behalten werden, innerhalb des Speicherplatzes oben: oft mehrere GB, für die sich eine große Festplatte eignet.',
      cacheFolder: 'Cache-Ordner',
      cacheFolderHelp:
        'Wo die weitergeleiteten Bilder behalten werden, bis zu 1 GB, von einem Start zum nächsten, zusammen mit den Programmführern und den Bilduntertiteln, die gerade gelesen werden.',
      where: {
        container:
          'Ein Ordner im Container von Polyfin, in dem eine Festplatte oder ein Ordner im Arbeitsspeicher eingehängt ist; leer der Cache in seinem Datenvolume, der angezeigt wird. Polyfin muss hineinschreiben können. Ein neuer Ordner gilt ab dem nächsten Start von Polyfin; der vorherige bleibt, wie er ist.',
        computer:
          'Ein Ordner des Computers, auf dem Polyfin läuft; leer der Cache in seinem Datenordner, der angezeigt wird. Polyfin muss hineinschreiben können. Ein neuer Ordner gilt ab dem nächsten Start von Polyfin; der vorherige bleibt, wie er ist.',
      },
    },
    recordingsTitle: 'Aufnahmen',
    recordingsFolder: (folder: string) => `Aufnahmen werden in ${folder} gespeichert.`,
    recording: 'Live-TV aufnehmen',
    recordingHelp:
      'Ermöglicht Benutzern, Aufnahmen von Live-TV-Sendungen und Serien zu planen, die im Ordner unten gespeichert werden.',
    recordingsFolderLabel: 'Aufnahmeordner',
    recordingsFolderHelp: {
      container:
        'Ein Ordner im Container von Polyfin, wenn leer der angezeigte, in seinem Datenvolume. Um Aufnahmen auf einem anderen Datenträger zu speichern, binde ihn dort ein, oder an anderer Stelle und gib hier seinen Pfad ein. Polyfin muss darin schreiben können. Eine Änderung verschiebt bereits erstellte Aufnahmen nicht.',
      computer:
        'Ein Ordner auf dem Computer, auf dem Polyfin läuft, wenn leer der angezeigte, in seinem Datenordner. Um Aufnahmen auf einem anderen Datenträger zu speichern, gib hier einen Ordner auf diesem Datenträger ein. Polyfin muss darin schreiben können. Eine Änderung verschiebt bereits erstellte Aufnahmen nicht.',
    },
    recordingPrePadding: 'Aufnahmen früher starten (Minuten)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Wie viele Minuten vor der Sendung eine Aufnahme beginnt. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    recordingPostPadding: 'Nach dem Ende weiter aufnehmen (Minuten)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Wie viele Minuten nach der Sendung eine Aufnahme weiterläuft. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    recordingRetentionDays: 'Aufnahmen behalten für (Tage, 0 = für immer)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Ältere Aufnahmen werden gelöscht. Dies wird täglich geprüft. Von ${r.min} bis ${r.max}.`,
    recordingCommercials: 'Werbung in Aufnahmen erkennen',
    recordingCommercialsHelp:
      'Sobald eine Aufnahme endet, liest Polyfin sie im Hintergrund, eine Aufnahme nach der anderen, nach Momenten, die zugleich schwarz und still sind, und wertet die kurzen Abschnitte dazwischen als Werbeunterbrechungen. Apps bieten dann an, sie zu überspringen, so wie jeder Benutzer es in den Wiedergabeeinstellungen seiner App gewählt hat, solange die Schaltflächen zum Überspringen von Intro und Abspann aktiviert sind. Aufnahmen, die länger als 4 Stunden sind, werden nicht gelesen.',
    convertRecordings: 'Fertige Aufnahmen in HEVC umwandeln',
    convertRecordingsHelp:
      'Fertige Aufnahmen werden einzeln in den unten angegebenen Stunden in HEVC umgewandelt, damit sie weniger Platz belegen. Das Bild behält seine Größe; Audio und Untertitel werden unverändert kopiert. Die Umwandlung nutzt die „Grafikkarte für die Umwandlung“, mit der „HEVC-Qualität“ und der „Deinterlacing-Methode“ aus Umwandlung (Transkodierung), und zählt als eine der „Gleichzeitigen Videoumwandlungen“: Sie wartet, solange Wiedergaben diese belegen. Sie startet nur, wenn genug freier Platz für eine Kopie in der Größe der Aufnahme vorhanden ist. Die Kopie ersetzt die Aufnahme nur, wenn sie gleich lang ist, dieselben Spuren enthält, bis zum Ende dekodiert werden kann und kleiner ist; andernfalls wird sie gelöscht, und die Aufnahme bleibt unverändert und wird nicht erneut versucht. Eine Aufnahme, die gerade abgespielt wird, wird ersetzt, sobald die Wiedergabe endet; Gesehen-Status, Fortsetzungspunkte und Werbung bleiben erhalten. Apps, die kein HEVC abspielen, erhalten die Aufnahme bei der Wiedergabe umgewandelt: Ist „Umwandlung (Transkodierung)“ deaktiviert, können sie sie nicht abspielen. Standardmäßig aus.',
    convertRecordingsBetween: 'Umwandeln zwischen',
    convertRecordingsAnd: 'und',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `In der Zeitzone des Servers. Eine Umwandlung, die am Ende noch läuft, wird abgebrochen und in der nächsten Nacht neu begonnen. Standardmäßig ${from} bis ${until}.`,
    convertRecordingsFrom: 'Beginn der Umwandlungen',
    convertRecordingsUntil: 'Ende der Umwandlungen',
    convertRecordingsOnProcessor: 'Prozessor verwenden, wenn keine Grafikkarte vorhanden ist',
    convertRecordingsOnProcessorHelp:
      'Ohne eine Grafikkarte, die HEVC kodiert, werden Aufnahmen auf dem Prozessor mit x265 umgewandelt, mit niedrigster Priorität. Das ist langsam und lastet den Prozessor in den oben angegebenen Stunden aus. Ohne Grafikkarte und mit deaktivierter Option wird nichts umgewandelt. Standardmäßig aus.',
    liveTvRefreshHours: 'Live-TV-Listen und Fernsehprogramme aktualisieren alle (Stunden)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `Wie oft die IPTV-Kanallisten und die XMLTV-Fernsehprogramme erneut heruntergeladen werden. Ein fehlgeschlagener Download wird früher wiederholt: nach 5 Minuten, nach 15 Minuten, dann stündlich. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    backupsFolder: (folder: string) => `Backups der Datenbank werden in ${folder} gespeichert.`,
    backups: 'Datenbank täglich sichern',
    backupsHelp:
      'Schreibt jeden Tag zur gewählten Stunde eine Kopie der Datenbank in den unten angegebenen Ordner und löscht die ältesten, die über die behaltene Anzahl hinausgehen.',
    backupFolderLabel: 'Backup-Ordner',
    backupFolderHelp: {
      container:
        'Ein Ordner im Container von Polyfin, wenn leer der angezeigte, in seinem Datenvolume. Backups sind auf einem anderen Datenträger als dem der Datenbank sicherer: Binde einen dort ein, oder an anderer Stelle und gib hier seinen Pfad ein. Polyfin muss darin schreiben können.',
      computer:
        'Ein Ordner auf dem Computer, auf dem Polyfin läuft, wenn leer der angezeigte, in seinem Datenordner. Backups sind auf einem anderen Datenträger als dem der Datenbank sicherer: Gib hier einen Ordner auf einem solchen ein. Polyfin muss darin schreiben können.',
    },
    backupHour: 'Täglich sichern um',
    backupHourHelp: (hour: string) => `In der Zeitzone des Servers. Standardmäßig ${hour}.`,
    backupsKept: 'Zu behaltende Backups',
    backupsKeptHelp: (r: RangeText) =>
      `Nach jedem Backup löscht Polyfin seine ältesten Backups, die über diese Anzahl hinausgehen. Andere Dateien im Ordner werden nie angetastet. Von ${r.min} bis ${r.max}; standardmäßig ${r.default}.`,
    lastBackup: 'Letztes Backup',
    backupDestinations: 'Backup-Ziele',
    backupDestinationsHelp:
      'Jedes Backup wird, sobald es in den Ordner geschrieben ist, unter demselben Dateinamen auch an diese Orte gesendet: S3-kompatiblen Speicher, SFTP- oder WebDAV-Server. Ein fehlgeschlagener Upload wird beim nächsten Backup erneut versucht.',
    webPlayerHelp:
      'Was der Web-Player (jellyfin-web, unter /web/) zusätzlich zu seinen eigenen Seiten anzeigt. Das Skript wirkt beim nächsten Laden einer Seite des Web-Players; der Web-Player behält CSS und Anmeldenachricht bis zu einer Minute.',
    openWebPlayer: 'Web-Player öffnen',
    webTheme: 'Theme',
    webThemeHelp:
      'Das Aussehen des Web-Players. LumaaGlaass ist das Theme, das Polyfin mitbringt. Keines behält das Aussehen von jellyfin-web, dessen Anzeige-Theme jeder Benutzer unter Einstellungen › Darstellung wählt. Benutzerdefiniert verwendet dein eigenes CSS und JavaScript unten, die erhalten bleiben, während ein anderes Theme gewählt ist.',
    webThemeChoices: { lumaaglaass: 'LumaaGlaass', none: 'Keines', custom: 'Benutzerdefiniert' },
    webThemeReload:
      'Der Web-Player behält das gelesene benutzerdefinierte CSS bis zu einer Minute: Lade seine offenen Seiten nach einer Änderung eine Minute später neu.',
    webThemeTexts: {
      lumaaglaass:
        'Was Polyfin für LumaaGlaass ausliefert, hier angezeigt und nicht bearbeitbar. Wähle Benutzerdefiniert oder „Als benutzerdefiniert bearbeiten“, um dein eigenes zu schreiben.',
      none: 'Keines liefert weder benutzerdefiniertes CSS noch JavaScript aus. Wähle Benutzerdefiniert, um dein eigenes zu schreiben.',
    },
    editAsCustom: 'Als benutzerdefiniert bearbeiten',
    editAsCustomTitle: 'Deine benutzerdefinierten Texte ersetzen?',
    editAsCustomConfirm:
      'Dein benutzerdefiniertes CSS und JavaScript werden durch die von LumaaGlaass ersetzt, und das Theme wird Benutzerdefiniert. Speichern der Seite übernimmt die Änderung.',
    themeCopy: 'Eine Kopie in Polyfin behalten',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Speichert die Stylesheets, Skripte, Schriften und Bilder, die die benutzerdefinierten Texte von anderswo laden, etwa ein Theme auf einem CDN, im Datenordner von Polyfin und lässt die Texte sie von Polyfin laden: Das Theme funktioniert dann, ohne sein CDN zu erreichen. Folgt @import, url() und den Adressen, die Skripte nennen, ${depth} Ebenen tief, nur von Adressen im Internet, bis ${file} pro Datei und insgesamt ${total} und ${files} Dateien.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `Eine Kopie von ${files === 1 ? 'einer Datei' : `${files} Dateien`}, ${size}, gespeichert am ${date}.`,
    themeCopyFiles: 'Gespeicherte Dateien',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'Eine Adresse konnte nicht gespeichert werden; sie wird weiter von anderswo geladen:'
        : `${count} Adressen konnten nicht gespeichert werden; sie werden weiter von anderswo geladen:`,
    themeCopyMissing: (count: number) =>
      count === 1
        ? 'Eine Datei der Kopie fehlt im Datenordner von Polyfin, wie nach dem Wiederherstellen einer Sicherung auf einem anderen System. Browser laden sie bis dahin von ihrer ursprünglichen Adresse, was Internet braucht. Aktualisiere die Kopie, um sie erneut herunterzuladen.'
        : `${count} Dateien der Kopie fehlen im Datenordner von Polyfin, wie nach dem Wiederherstellen einer Sicherung auf einem anderen System. Browser laden sie bis dahin von ihren ursprünglichen Adressen, was Internet braucht. Aktualisiere die Kopie, um sie erneut herunterzuladen.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'Die benutzerdefinierten Texte laden eine Adresse von anderswo:'
        : `Die benutzerdefinierten Texte laden ${count} Adressen von anderswo:`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'Eine seit der Kopie hinzugefügte Adresse wird von anderswo geladen; aktualisiere die Kopie, um sie auch zu speichern:'
        : `${count} seit der Kopie hinzugefügte Adressen werden von anderswo geladen; aktualisiere die Kopie, um sie auch zu speichern:`,
    themeCopyNothing: 'Die benutzerdefinierten Texte laden nichts von anderswo.',
    themeCopyKeep: 'Eine Kopie in Polyfin behalten',
    themeCopyUpdate: 'Kopie aktualisieren',
    themeCopyRemove: 'Kopie entfernen',
    themeCopyRemoveTitle: 'Kopie entfernen?',
    themeCopyRemoveConfirm:
      'Ihre Dateien werden gelöscht, und die benutzerdefinierten Texte laden sie wieder von ihren ursprünglichen Adressen.',
    themeCopyKept:
      'Kopie gespeichert. Lade die offenen Seiten des Web-Players in einer Minute neu.',
    themeCopyRemoved:
      'Kopie entfernt. Lade die offenen Seiten des Web-Players in einer Minute neu.',
    themeCopyFailed: 'Keine Datei konnte gespeichert werden, daher wurde keine Kopie angelegt:',
    themeCopySaveFirst:
      'Speichere zuerst deine Änderungen: Die Kopie wird aus den gespeicherten Texten erstellt.',
    themeCopyKinds: { stylesheet: 'Stylesheet', script: 'Skript', font: 'Schrift', image: 'Bild' },
    themeCopyReasons: {
      private_address: 'in einem lokalen Netz oder dorthin umgeleitet',
      not_http: 'zu einer Adresse umgeleitet, die weder http noch https ist',
      unavailable: 'nicht verfügbar',
      timeout: 'hat zu lange gedauert',
      wrong_type: 'keine Datei der Art, als die sie geladen wird',
      too_large: 'größer als die Grenze für eine Datei',
      total_too_large: 'über der Grenze für die ganze Kopie',
      too_many_files: 'über der Anzahl erlaubter Dateien',
      too_deep: 'zu viele Ebenen tiefer genannt',
      built_at_run_time:
        'wird von einem Skript erst beim Ausführen vervollständigt, was keine Kopie verfolgen kann',
    },
    customCss: 'Benutzerdefiniertes CSS',
    customCssHelp:
      'Wird auf jede Seite des Web-Players angewendet, für jeden Benutzer, als benutzerdefiniertes CSS von Jellyfin. Benutzer können es unter Einstellungen › Darstellung deaktivieren.',
    customJs: 'Benutzerdefiniertes JavaScript',
    customJsHelp:
      'Wird von jeder Seite des Web-Players nach dessen eigenen Skripten geladen. Leer lädt nichts.',
    customJsWarningTitle: 'Füge nur Code ein, dem du vertraust',
    customJsWarning:
      'Dieses Skript läuft im Browser jedes Benutzers, der den Web-Player auf diesem Server öffnet, mit dessen Konto. Es kann lesen, was er sieht, und in seinem Namen handeln.',
    loginDisclaimer: 'Anmeldenachricht',
    loginDisclaimerHelp:
      'Wird unter dem Anmeldeformular des Web-Players angezeigt. Reiner Text, Markdown oder HTML; der Web-Player entfernt unsicheres HTML.',
    addVersionsToOpenPage: 'Versionen zur geöffneten Seite eines Titels hinzufügen',
    addVersionsToOpenPageHelp:
      'Fügt die von den Add-ons gefundenen Versionen dem Versionsmenü der Seite eines Films oder einer Folge hinzu, während sie geöffnet ist, und behält die gewählte Version bei. Wenn aus, zeigt eine Seite, die geöffnet wurde, bevor eine Version gefunden wurde, die ersten trotzdem an, sobald sie eintreffen, und ihre Wiedergabeschaltflächen warten weiterhin auf eine Version; die übrigen erscheinen beim erneuten Öffnen der Seite.',
    homeRows: 'Startseiten-Zeilen',
    homeRowsHelp:
      'Zeilen, die der Web-Player an ihrer Stelle zu seiner Startseite hinzufügt, für alle Benutzer oder für ausgewählte. Jeder Benutzer kann eine Zeile unter Mein Konto › Startseiten-Zeilen ausblenden; die eigenen Bereiche des Web-Players bleiben in dessen Einstellungen › Startseite. Jede Zeile lässt weg, was der Benutzer nicht sehen darf: Titel über seiner Kindersicherung, gesperrte Genres, ausgeblendete Titel und die für ihn ausgeblendeten Bibliotheken.',
    homeRow: {
      loading: 'Startseitenzeilen werden geladen…',
      empty: 'Keine Startseitenzeile',
      emptyHint: 'Die Startseite des Web-Players zeigt nur ihre eigenen Bereiche.',
      add: 'Zeile hinzufügen',
      addTitle: 'Startseitenzeile hinzufügen',
      editTitle: (name: string) => `${name} bearbeiten`,
      edit: 'Bearbeiten',
      delete: 'Löschen',
      deleteTitle: (name: string) => `${name} löschen?`,
      deleteBody:
        'Die Zeile verschwindet von der Startseite aller Benutzer, und ihre Bibliothek verschwindet aus anderen Apps.',
      deleted: (name: string) => `${name} wurde gelöscht.`,
      saved: (name: string) => `${name} wurde gespeichert.`,
      create: 'Hinzufügen',
      creating: 'Wird hinzugefügt…',
      save: 'Speichern',
      saving: 'Wird gespeichert…',
      name: 'Name',
      nameHelp: 'Der Titel der Zeile auf der Startseite.',
      becauseNameHelp:
        'Jede Zeile trägt diesen Namen, gefolgt vom Titel, auf den sie sich bezieht: „Weil du Beispielfilm gesehen hast“.',
      kind: 'Art',
      kinds: {
        catalog: 'Ein Katalog',
        collection: 'Eine Sammlung',
        because: 'Weil du … gesehen hast',
        new: 'Neu in deinen Bibliotheken',
      },
      kindHelp: {
        catalog:
          'Die erste Seite eines Katalogs der Server-Add-ons, ob Bibliothek oder nicht, etwa ein Trend-Katalog. Benutzer, die die Server-Add-ons nicht verwenden oder für die seine Bibliothek ausgeblendet ist, sehen sie nicht.',
        collection:
          'Die Titel einer Sammlung: einer von Benutzern erstellten Sammlung, einer als Sammlung angezeigten intelligenten Liste oder einer Sammlung eines Add-ons.',
        because:
          'Eine Zeile für jeden der zuletzt abgespielten Filme und jede der zuletzt abgespielten Serien des Benutzers, mit ähnlichen Titeln, wie sie die Filmempfehlungen liefern. Keine, solange „Ähnliche Titel“ ausgeschaltet ist.',
        new: 'Die Titel, die in den letzten 7 Tagen in die Kataloge der Bibliotheken des Benutzers gekommen sind, neueste zuerst. Polyfin zählt sie ab dem Zeitpunkt, an dem es jeden Titel zum ersten Mal in jedem Katalog gesehen hat.',
      },
      defaultNames: {
        because: 'Weil du gesehen hast',
        new: 'Neu in deinen Bibliotheken',
      },
      catalog: 'Katalog',
      noCatalog: 'Die Server-Add-ons haben keinen Katalog zum Anzeigen.',
      collection: 'Sammlung',
      noCollection: 'Es gibt noch keine Sammlung.',
      collectionSources: {
        users: 'Von Benutzern erstellt',
        smart: 'Intelligente Liste',
        addon: 'Add-on',
      },
      choose: 'Auswählen…',
      unknownCatalog: 'Ein nicht mehr angebotener Katalog',
      unknownCollection: 'Eine nicht mehr angebotene Sammlung',
      count: 'Zeilen',
      countHelp:
        'Wie viele der zuletzt abgespielten Filme und Serien eine Zeile erhalten, von 1 bis 10.',
      place: 'Position',
      places: {
        top: 'Ganz oben',
        resume: 'Nach „Weiterschauen“',
        nextup: 'Nach „Als Nächstes“',
        bottom: 'Ganz unten',
      },
      placeHelp:
        'Wo der Web-Player sie anzeigt. Wenn die Startseite eines Benutzers diesen Bereich nicht anzeigt, nimmt die Zeile die vorherige Position ein.',
      shownTo: 'Angezeigt für',
      everyone: 'Alle Benutzer',
      chosenUsers: 'Ausgewählte Benutzer',
      users: 'Benutzer',
      noUser: 'Wähle mindestens einen Benutzer aus.',
      shownToUsers: (count: number) => (count === 1 ? '1 Benutzer' : `${count} Benutzer`),
      rowsCount: (count: number) => (count === 1 ? '1 Zeile' : `${count} Zeilen`),
      library: 'Auch eine Bibliothek in anderen Apps',
      libraryHelp:
        'Anderen Apps kann keine neue Art von Startseitenbereich hinzugefügt werden: Dadurch wird die Zeile auch zu einer Bibliothek ohne Typ, die sie unter den Bibliotheken des Benutzers auflisten, mit einer Zeile ihrer neuesten Titel auf ihrer Startseite.',
      libraryBadge: 'Bibliothek',
      moveLabel: 'Verschieben',
    },
    codeSize: (used: number, max: number) =>
      `${used} KB von ${max >= 1024 ? `${max / 1024} MB` : `${max} KB`}`,
    codeKeys: 'Tab fügt Leerzeichen ein; drücke Escape und dann Tab, um das Feld zu verlassen.',
  },
  settingsPage: {
    searchHint: 'Nach Name oder Beschreibung.',
    noMatch: 'Keine Einstellung entspricht dieser Suche.',
    sections: {
      variables: 'Umgebungsvariablen',
    },
    variablesHelp:
      'Nur lesbar. Diese Variablen werden am Container gesetzt (Docker-Umgebung, Compose-Datei oder Unraid-Vorlage) und gelten beim Start von Polyfin: Ändere sie dort und starte den Container dann neu. Die Variablen, mit denen früher andere Optionen festgelegt wurden, werden beim ersten Start dieser Version einmalig in die Einstellungen übernommen und danach nicht mehr gelesen: Ändere diese in den Einstellungen. Geheimnisse werden ausgeblendet, und bei der Datenbankadresse werden nur Host und Datenbank angezeigt.',
    value: 'Wirksamer Wert',
    defaultValue: 'Standard',
    setValue: 'Gesetzt',
    hidden: 'Ausgeblendet',
    empty: 'Leer',
    notRead: 'Wird von Polyfin nicht gelesen',
    searchPlaceholder: 'Einstellung suchen',
    results: (count: number) =>
      count === 1 ? '1 Einstellung gefunden' : `${count} Einstellungen gefunden`,
    noMatchHelp:
      'Versuch es mit einem anderen Wort, etwa dem Namen einer Variable oder eines Dienstes.',
    variableHint: 'Umgebungsvariable',
    ledes: {
      general:
        'Der Name des Servers, seine Sprache, wie Apps sich anmelden und ihn finden, und die Suche nach neuen Versionen.',
      playback: 'Wie Polyfin die Versionen auswählt und vorbereitet, die es abspielt.',
      content:
        'Schaltflächen zum Überspringen des Intros, ähnliche Titel, generierte Bilder, Bewertungen, Songtexte, gleichmäßige Lautstärke zwischen Songs und Wiedergabeschwellen.',
      catalogs: 'Wie viel Polyfin aus den Katalogen der Add-ons liest und wie oft.',
      security:
        'Eigene Add-ons und Importe der Benutzer, gesperrte Konten, ungenutzte Geräte und Reverse-Proxys.',
      liveTv: 'Wie oft die IPTV-Kanallisten und Fernsehprogramme erneut heruntergeladen werden.',
      diagnostics: 'Das detaillierte Protokoll und die wirksamen Umgebungsvariablen.',
      notifications:
        'Wohin Polyfin für alle Benutzer neue Folgen, Aufnahmen, Wiedergaben und abbrechende Verbindungen sowie Systemzustand-Probleme, neue Benutzer, neue Versionen und gesperrte Konten meldet und die Wochenzusammenfassung sowie den Monats- und Jahresrückblick sendet.',
    },
    groups: {
      skip: 'Intro und Abspann überspringen',
      titlePages: 'Titelseiten',
      images: 'Bilder',
      ratings: 'Bewertungen',
      music: 'Musik',
      thresholds: 'Wiedergabeschwellen',
    },
    leaveTitle: 'Ohne Speichern verlassen?',
    leaveBody:
      'Die Änderungen in diesem Abschnitt sind noch nicht gespeichert. Sie gehen verloren, wenn du die Seite verlässt.',
    leave: 'Ohne Speichern verlassen',
    stay: 'Bleiben',
    variablesEmpty: 'Keine Variablen zum Anzeigen',
    variablesEmptyHelp: 'Setze POLYFIN_-Variablen am Container und starte ihn dann neu.',
  },
}

export default settings
