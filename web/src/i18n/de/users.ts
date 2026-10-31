import type en from '../en/users'

const users: typeof en = {
  users: {
    title: 'Benutzer',
    description: 'Erstelle und verwalte die Konten, die diesen Server nutzen dürfen.',
    listTitle: 'Konten',
    empty: 'Noch keine Benutzer.',
    administrator: 'Administrator',
    hidden: 'Auf dem Anmeldebildschirm ausgeblendet',
    disabled: 'Deaktiviert',
    you: 'Du',
    createTitle: 'Benutzer erstellen',
    name: 'Name',
    password: 'Passwort',
    isAdministrator: 'Administrator',
    isAdministratorHelp: 'Administratoren können Benutzer und Servereinstellungen verwalten.',
    showOnSignIn: 'Auf dem Anmeldebildschirm anzeigen',
    showOnSignInHelp:
      'Sichtbare Benutzer werden auf dem Anmeldebildschirm der Jellyfin-Apps aufgeführt. Ausgeblendete Benutzer geben ihren Namen ein, wie standardmäßig in Jellyfin.',
    create: 'Benutzer erstellen',
    creating: 'Wird erstellt…',
    created: (name: string) => `${name} wurde erstellt.`,
    edit: 'Bearbeiten',
    close: 'Schließen',
    rename: 'Umbenennen',
    renamed: 'Der Name wurde geändert.',
    resetPassword: 'Passwort zurücksetzen',
    newPassword: 'Neues Passwort',
    resetPasswordHelp:
      'Wenn das Passwort geändert wird, werden alle Jellyfin-Geräte dieses Benutzers abgemeldet.',
    passwordReset: 'Das Passwort wurde zurückgesetzt.',
    accessTitle: 'Zugriff',
    isDisabled: 'Deaktiviert',
    isDisabledHelp:
      'Ein deaktivierter Benutzer kann sich nicht anmelden, und alle seine Geräte werden abgemeldet.',
    providerOnly: 'Nur Identitätsanbieter',
    providerOnlyHelp:
      'Der Benutzer meldet sich über den unter Einstellungen › Sicherheit eingerichteten Identitätsanbieter an, nie mit einem Passwort: Das Passwort wird abgelehnt, und ein so erstelltes Konto braucht keines. Jellyfin-Apps, die den Anbieter nicht öffnen können, melden sich mit Quick Connect an.',
    ended: 'Beendet',
    endsOn: (date: string) => `Endet am ${date}`,
    endedOn: (date: string) => `Beendet am ${date}`,
    endTitle: 'Endet am',
    endHelp:
      'Ab diesem Datum (in der Zeitzone des Servers) wird das Konto wie ein deaktiviertes abgewiesen: Es kann sich nicht anmelden, und seine Geräte werden abgewiesen. Alles, was es hat, bleibt erhalten, und ein späteres Datum oder „Nie“ stellt es wieder her.',
    endAdministratorHelp:
      'Das Konto eines Administrators endet nie: Wird ein Benutzer zum Administrator, wird sein Enddatum entfernt.',
    endDate: 'Enddatum',
    saveEnd: 'Datum speichern',
    endNever: 'Nie',
    canTranscode: 'Darf Konvertierung (Transkodierung) nutzen',
    canTranscodeHelp:
      'Erlaubt dem Server, Video und Audio neu zu kodieren, wenn die App dieses Benutzers eine Datei nicht unverändert abspielen kann. Ohne diese Berechtigung werden Dateien unverändert oder nur neu verpackt abgespielt.',
    canDownload: 'Darf herunterladen',
    canDownloadHelp:
      'Erlaubt diesem Benutzer, Titel in Jellyfin-Apps zu speichern, um sie offline anzusehen.',
    canPlayTorrents: 'Darf Torrent-Streams abspielen',
    canPlayTorrentsHelp:
      'Bietet diesem Benutzer die Torrents an, die Add-ons auflisten, wenn Einstellungen › Torrents sie abspielt. Für alle aus, auch für Administratoren, bis die Berechtigung erteilt wird.',
    noTranscoding: 'Keine Konvertierung',
    noDownloads: 'Keine Downloads',
    turnOffDownloads: 'Downloads für alle deaktivieren',
    turnOffDownloadsConfirm:
      'Alle Benutzer und alle Gruppen verlieren die Berechtigung zum Herunterladen, auch in bereits angemeldeten Jellyfin-Apps. Du kannst sie einem Benutzer oder einer Gruppe auf deren Seite unter „Zugriff“ wieder erteilen.',
    turningOffDownloads: 'Wird deaktiviert…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Niemand konnte herunterladen.'
        : count === 1
          ? '1 Benutzer kann nicht mehr herunterladen.'
          : `${count} Benutzer können nicht mehr herunterladen.`,
    lastAdminHelp:
      'Der letzte aktive Administrator kann nicht gelöscht, herabgestuft oder deaktiviert werden.',
    updated: 'Änderungen gespeichert.',
    metadataLanguageTitle: 'Metadatensprache',
    metadataLanguageHelp:
      'Die Sprache der Namen, Beschreibungen und Bilder von Titeln in den Apps dieses Benutzers, sofern eines der Add-ons des Servers sie in dieser Sprache beschreibt; Titel, die kein Add-on in dieser Sprache beschreibt, behalten die Beschreibung des Servers. Genres, Altersfreigaben und Personen bleiben für alle gleich, sodass Kindersicherung und gesperrte Genres jeden Benutzer gleich behandeln. Die Namen, die Polyfin selbst erzeugt, folgen ihr, wenn Polyfin sie spricht: Englisch, Französisch, Deutsch, Spanisch, Italienisch oder Portugiesisch.',
    metadataLanguageNone:
      'Noch kein Add-on des Servers hat eine Sprache. Weise einem Add-on auf seiner Seite unter Inhalte › Quellen eine Sprache zu.',
    webThemeTitle: 'Theme des Web-Players',
    webThemeHelp:
      'Das Aussehen des Web-Players für diesen Benutzer, in jedem Browser, in dem er sich anmeldet: das Theme des Servers, LumaaGlaass, das Aussehen von jellyfin-web, leichter, für Fernseher und ältere Telefone, oder das benutzerdefinierte Theme, solange es einen Text hat. Browser, die für LumaaGlaass zu alt sind, erhalten von selbst das Aussehen von jellyfin-web.',
    webThemeChange: 'Ändern erlauben',
    webThemeChangeHelp:
      'Unter Mein Konto › Theme des Web-Players. Ausgeschaltet behält er das hier gewählte Theme.',
    audioRegionTitle: 'Audioregion',
    audioRegionHelp:
      'Die Synchronfassung, die dieser Benutzer bevorzugt, wenn eine Sprache für eine Region eigens synchronisiert wird, etwa Französisch für Québec. Sie gilt für die Audiosprache, die er in seinen Apps gewählt hat, sonst die des Servers, wenn es die Sprache der Region ist, und legt sie fest, wenn keine gewählt ist: In dieser Sprache werden die Audiospuren der Region zuerst abgespielt, und ihre Versionen kommen zuerst, wenn der Server Versionen nach Sprache sortiert.',
    parentalTitle: 'Kindersicherung',
    maxRating: 'Maximale Altersfreigabe',
    maxRatingHelp:
      'Titel mit einer Altersfreigabe über dieser Grenze werden für diesen Benutzer ausgeblendet.',
    noLimit: 'Keine Begrenzung',
    blockUnratedMovies: 'Filme ohne Altersfreigabe sperren',
    blockUnratedShows: 'Serien ohne Altersfreigabe sperren',
    ratingLimit: (rating: string) => `Bis ${rating}`,
    saveParental: 'Kindersicherung speichern',
    devicesTitle: 'Geräte',
    delete: 'Benutzer löschen',
    deleteConfirm: (name: string) =>
      `${name} löschen? Die Geräte werden abgemeldet, und dies kann nicht rückgängig gemacht werden.`,
    deleting: 'Wird gelöscht…',
    deleted: (name: string) => `${name} wurde gelöscht.`,
    canAddAddons: 'Darf eigene Add-ons hinzufügen',
    canAddAddonsHelp:
      'Erlaubt diesem Benutzer, auf seiner Seite „Meine Quellen“ eigene Stremio-Add-ons hinzuzufügen. Ohne diese Berechtigung bleiben seine Add-ons erhalten, werden aber nicht verwendet.',
    noPersonalAddons: 'Keine eigenen Add-ons',
    blockedUntil: (time: string) => `Gesperrt bis ${time}`,
    blockedHelp:
      'Für dieses Konto wurden zu oft falsche Passwörter eingegeben: Es kann sich bis dahin nicht anmelden, auch nicht mit dem richtigen Passwort.',
    unblock: 'Entsperren',
    unblocking: 'Wird entsperrt…',
    unblocked: 'Das Konto ist entsperrt.',
    twoStepOn: 'Die zweistufige Anmeldung ist aktiviert.',
    twoStepHelp:
      'Wenn dieser Benutzer das Telefon mit seiner Authenticator-App verloren hat und keinen Wiederherstellungscode mehr hat, deaktiviere sie: Er meldet sich dann nur mit seinem Passwort an und richtet sie unter „Mein Konto“ erneut ein.',
    turnOffTwoStep: 'Zweistufige Anmeldung deaktivieren',
    turningOffTwoStep: 'Wird deaktiviert…',
    turnOffTwoStepTitle: (name: string) => `Zweistufige Anmeldung von ${name} deaktivieren?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} meldet sich dann nur mit dem Passwort an. Im Aktivitätsprotokoll wird festgehalten, dass du sie deaktiviert hast.`,
    twoStepTurnedOff: 'Die zweistufige Anmeldung ist für diesen Benutzer deaktiviert.',
    playbackAccessTitle: 'Wiedergabe und Zugriff',
    maxPlaybacks: 'Gleichzeitige Wiedergaben (0 = keine Begrenzung)',
    maxPlaybacksHelp:
      'Wie viele Geräte dieses Benutzers gleichzeitig abspielen können. Anmelden kann er sich trotzdem auf weiteren Geräten.',
    maxBitrate: 'Maximale Qualität',
    maxBitrateHelp:
      'Darüber wird das Video heruntergerechnet, wenn dieser Benutzer die Umwandlung nutzen darf; andernfalls wird eine leichtere Version abgespielt.',
    bitrateNoLimit: 'Keine Begrenzung',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, hohe Qualität)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'Live-TV',
    liveTvHelp: 'Zeigt diesem Benutzer die Live-TV-Kanäle an.',
    syncPlay: 'Gemeinsam schauen',
    syncPlayHelp:
      'Denselben Titel synchron mit anderen abspielen, in den Jellyfin-Apps, die das anbieten.',
    syncPlayCreateAndJoin: 'Gruppen erstellen und beitreten',
    syncPlayJoin: 'Nur beitreten',
    syncPlayNone: 'Nein',
    remoteControl: 'Darf Apps anderer Benutzer steuern',
    remoteControlHelp:
      'Erlaubt diesem Benutzer, in den Jellyfin-Apps anderer Benutzer die Wiedergabe zu starten, zu pausieren oder Nachrichten an sie zu senden. Seine eigenen Apps kann jeder steuern.',
    liveTvManagement: 'Darf Live-TV aufnehmen',
    liveTvManagementHelp: 'Aufnahmen planen und löschen.',
    savePlaybackAccess: 'Wiedergabe und Zugriff speichern',
    visibleLibrariesTitle: 'Sichtbare Bibliotheken',
    visibleLibrariesHelp:
      'Die Bibliotheken des Servers, die die Apps dieses Benutzers anzeigen. Später hinzugefügte Bibliotheken werden angezeigt. Damit wird nur festgelegt, welche Bibliotheken angezeigt werden: Titel bleiben über andere Bibliotheken, die Suche oder Links erreichbar. Um Titel zu sperren, nutze die Kindersicherung oder gesperrte Genres.',
    noServerLibraries: 'Der Server hat noch keine Bibliothek.',
    saveVisibleLibraries: 'Sichtbare Bibliotheken speichern',
    blockedGenresTitle: 'Gesperrte Genres',
    blockedGenresHelp:
      'Titel eines dieser Genres werden für diesen Benutzer überall ausgeblendet, wie Titel oberhalb seiner Altersfreigabe-Grenze der Kindersicherung.',
    genre: 'Genre',
    genreHint: 'Wähle ein Genre aus den Bibliotheken des Servers oder gib eines ein.',
    addGenre: 'Hinzufügen',
    removeGenre: (genre: string) => `${genre} entfernen`,
    noBlockedGenres: 'Kein Genre ist gesperrt.',
    saveBlockedGenres: 'Gesperrte Genres speichern',
    liveTvChannelsTitle: 'Live-TV-Sender',
    liveTvChannelsHelp:
      'Die Sender der Live-TV-Quellen des Servers, die dieser Benutzer sieht, nach Gruppe und nach Sender gewählt. Ausgeblendete Sender fehlen in der Senderliste, im Programmführer, in Suchen, in Replay, bei Aufnahmen, Timern und Erinnerungen und lassen sich nicht abspielen. Eigene Quellen des Benutzers sind nicht eingeschränkt.',
    channelsMode: 'Sender',
    channelsAll: 'Alle',
    channelsOnly: 'Nur diese',
    channelsExcept: 'Alle außer diesen',
    channelsOnlyHelp: 'Nur die angehakten Gruppen und Sender werden gezeigt.',
    channelsExceptHelp: 'Die angehakten Gruppen und Sender werden ausgeblendet.',
    channelGroups: 'Gruppen',
    filterChannelGroups: 'Gruppen filtern',
    moreChannelGroups: (count: number) => `${count} weitere: zum Finden filtern.`,
    newChannelGroup: 'Neu',
    noChannelGroups: 'Die Live-TV-Quellen des Servers haben noch keinen Sender.',
    singleChannels: 'Sender',
    searchChannels: 'Sender nach Name oder Nummer suchen',
    tickedChannels: 'Angehakte Sender',
    noTickedChannels: 'Kein Sender angehakt.',
    goneChannel: 'Nicht mehr gelisteter Sender',
    removeChannel: (name: string) => `${name} entfernen`,
    moreChannels: (count: number) => `${count} weitere: zum Finden suchen.`,
    hideNewChannelGroups: 'Sender aus später hinzugefügten Gruppen ausblenden',
    hideNewChannelGroupsHelp:
      'Gruppen, die eine Quelle nach dem Speichern hinzufügt, werden ausgeblendet. Sie erscheinen hier angehakt und als Neu markiert, bis erneut gespeichert wird.',
    saveLiveTvChannels: 'Live-TV-Sender speichern',
    allowedHoursTitle: 'Erlaubte Zeiten',
    allowedHoursHelp:
      'Wenn Zeiten festgelegt sind, kann sich dieser Benutzer nur während dieser Zeiten anmelden und den Server nutzen, in der Zeitzone des Servers. Außerhalb davon werden seine Apps abgewiesen.',
    noAllowedHours: 'Keine Zeiten festgelegt: Dieser Benutzer kann den Server jederzeit nutzen.',
    day: 'Tag',
    from: 'Von',
    to: 'Bis',
    addHours: 'Zeiten hinzufügen',
    removeHours: 'Entfernen',
    hoursOrder: 'Jede Endzeit muss nach der zugehörigen Startzeit liegen.',
    saveAllowedHours: 'Erlaubte Zeiten speichern',
    days: {
      Sunday: 'Sonntag',
      Monday: 'Montag',
      Tuesday: 'Dienstag',
      Wednesday: 'Mittwoch',
      Thursday: 'Donnerstag',
      Friday: 'Freitag',
      Saturday: 'Samstag',
      Everyday: 'Jeden Tag',
      Weekday: 'Werktags (Montag bis Freitag)',
      Weekend: 'Am Wochenende',
    },
    canManageCollections: 'Darf Sammlungen verwalten',
    canManageCollectionsHelp:
      'Erlaubt diesem Benutzer, in Jellyfin-Apps Sammlungen von Titeln zu erstellen, Titel hinzuzufügen oder zu entfernen und Sammlungen zu löschen. Alle Benutzer sehen die Sammlungen, jeweils nur mit den Titeln, die sie sehen dürfen.',
    resetPinRequested: 'Zurücksetzen des Passworts angefordert: PIN',
    resetPinValidUntil: (time: string) => `, gültig bis ${time}`,
    resetPinHelp:
      'Gib diese PIN an den Benutzer weiter: Er meldet sich damit an, und sie wird zu seinem neuen Passwort.',
    canManageSubtitles: 'Darf Untertitel verwalten',
    canManageSubtitlesHelp:
      'Erlaubt diesem Benutzer, Titeln aus seinen Jellyfin-Apps Untertiteldateien hinzuzufügen und dort die Untertitel der Add-ons zu durchsuchen.',
    canShare: 'Darf Titel teilen',
    canShareHelp:
      'Erlaubt diesem Benutzer, einen Film oder eine Folge über einen ablaufenden Link an jemanden ohne Konto weiterzugeben, über „Mein Konto“ oder die Seite eines Titels im Web-Player, solange Einstellungen › Sicherheit › „Freigabelinks erlauben“ aktiviert ist. Beim Deaktivieren werden seine Links widerrufen.',
    remoteAccess: 'Darf sich von außerhalb des lokalen Netzwerks verbinden',
    remoteAccessHelp:
      'Erlaubt diesem Benutzer, sich von überall anzumelden, zu stöbern und abzuspielen. Ausgeschaltet geht das nur aus dem lokalen Netzwerk, auch für Administratoren, so wie es die bekannten Proxys unter Einstellungen › Sicherheit bestimmen. Die Links, die er teilt, spielen weiterhin überall.',
    remoteAccessUnknownProxy:
      'Der Systemzustand sieht einen Reverse-Proxy, der nicht unter den bekannten Proxys steht: Alle, die über ihn kommen, wirken lokal, daher gilt diese Einschränkung für sie nicht.',
    openKnownProxies: 'Bekannte Proxys öffnen',
    shareLinksTitle: 'Freigabelinks',
    shareLinksHelp:
      'Die Links, die dieser Benutzer erstellt hat, um einen Film oder eine Folge zu teilen. „Widerrufen“ beendet einen sofort.',
    qualityGroup: 'Qualitätsgruppe',
    qualityGroupHelp:
      'Die höchste Auflösung, die diesem Benutzer angeboten wird. Höhere Versionen werden ausgelassen, solange eine passt; passt keine, werden sie herunterkonvertiert oder abgelehnt, wenn dieser Benutzer keine Konvertierung nutzen darf. Live-TV wird ebenfalls herunterkonvertiert. Nur passende Versionen werden unverändert heruntergeladen; ist „Konvertierte Downloads“ unter Einstellungen › Konvertierung (Transkodierung) aktiviert, laden die Apps, die es anfordern, die anderen herunterkonvertiert herunter.',
    qualityGroupOriginal: 'Original (ohne Begrenzung)',
    member: 'Mitglied',
    lastSignInLabel: 'Letzte Anmeldung:',
    active: 'Aktiv',
    pinRequested: 'PIN angefordert',
    emptyHelp:
      'Erstelle ein Konto für jede Person, die hier schaut, und melde dich dann damit in einer Jellyfin-App an.',
    devicesEmptyHelp: 'Apps erscheinen hier, sobald sie sich mit diesem Konto anmelden.',
    notFound: 'Benutzer nicht gefunden',
    notFoundHelp:
      'Dieser Benutzer existiert nicht mehr oder die Adresse ist falsch. Wähle einen Benutzer aus der Liste.',
    backToUsers: 'Zurück zu den Benutzern',
    sectionsLabel: 'Bereiche des Benutzers',
    profileTitle: 'Name und Passwort',
    accessHelp: 'Jede Änderung wird sofort gespeichert.',
    noServerLibrariesHelp:
      'Füge unter Inhalte › Bibliotheken eine Bibliothek hinzu und wähle dann hier, welche dieser Benutzer sieht.',
    jellyfinImport: {
      open: 'Von einem anderen Server importieren',
      title: 'Von einem anderen Server importieren',
      description:
        'Übernimm Konten und ihre Wiedergabedaten von einem Jellyfin-, Emby- oder Plex-Server: Kontonamen und Administratorstatus, dann für jeden Benutzer die gesehenen Filme und Folgen mit ihren Daten, Fortsetzungspunkte und Favoriten, zugeordnet über IMDb, TMDB oder TVDB, und auf Wunsch ihre Bewertungen, die Sammlungen des Servers und ihre Wiedergabelisten. Die Daten in Polyfin werden nur ergänzt, und auf den anderen Server wird nichts geschrieben. Server geben keine Passwörter heraus: Du legst für jedes neue Konto eines fest oder behältst das, mit dem sich ein Benutzer anmeldet.',
      serverKind: 'Server',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin oder ein Server, der dessen API spricht.',
        emby: 'Emby, mit eigenen API-Schlüsseln und Benutzern.',
        plex: 'Plex Media Server, gelesen mit dem Token seines Besitzers.',
      },
      connectTitle: (server: string) => `Mit ${server} verbinden`,
      connectHelp:
        'Adresse, Schlüssel und Passwörter werden nur für diesen Import verwendet: Polyfin speichert sie nicht.',
      address: 'Serveradresse',
      addressHelp: (server: string) =>
        `Die Adresse, unter der sich ${server} im Browser öffnet. Eine Adresse im lokalen Netzwerk funktioniert.`,
      plexAddressHelp:
        'Die Adresse von Plex Media Server selbst, meist auf Port 32400. Eine Adresse im lokalen Netzwerk funktioniert.',
      connectWith: 'Verbinden mit',
      withApiKey: 'Einem API-Schlüssel',
      withAccount: 'Einem Benutzerkonto',
      apiKey: 'API-Schlüssel',
      apiKeyHelp:
        'Erstelle einen in der Jellyfin-Übersicht unter „API-Schlüssel“: Er liest die Wiedergabedaten aller Benutzer.',
      embyApiKeyHelp:
        'Erstelle einen in den Emby-Einstellungen unter „API-Schlüssel“: Er liest die Wiedergabedaten aller Benutzer. Der eigene Schlüssel eines Benutzers wird nicht angenommen: Emby gibt nicht an, wem er gehört.',
      plexToken: 'Plex-Token',
      plexTokenHelp:
        'Der Token des Serverbesitzers, mit dem Polyfin den Server nur liest. Um ihn zu finden, öffne Plex im Browser, angemeldet als Besitzer. Öffne das Menü eines beliebigen Films, wähle „Get Info“ und dann „View XML“: Die Adresse der Seite, die sich öffnet, endet mit X-Plex-Token= gefolgt vom Token. Kopiere, was danach folgt.',
      accountName: 'Benutzername',
      accountNameHelp: (server: string) =>
        `Das Konto eines Administrators listet alle Benutzer auf. Polyfin meldet sich wie die Apps von ${server} an und danach wieder ab.`,
      accountPassword: 'Passwort',
      accountPasswordHelp: 'Lass das Feld leer, wenn das Konto keines hat.',
      connect: 'Verbinden',
      connecting: 'Wird verbunden…',
      chooseTitle: 'Auswählen, wer importiert wird',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. Wähle für jeden Benutzer das Polyfin-Konto, in das importiert wird, oder erstelle eines.`,
      changeServer: 'Server wechseln',
      noUsers: (product: string) => `Dieser ${product}-Server hat keine Benutzer.`,
      lastActivityLabel: 'Letzte Aktivität:',
      importAs: 'Importieren in',
      skip: 'Nicht importieren',
      newUser: 'Neuer Benutzer',
      watchData: 'Wiedergabedaten importieren',
      watchDataHelp: 'Gesehene Titel mit ihren Daten, Fortsetzungspunkte und Favoriten.',
      plexOwnerWatchDataHelp:
        'Gesehene Titel mit ihren Daten und Wiedergabezahlen, Fortsetzungspunkte und Bewertungen.',
      plexWatchDataHelp:
        'Die Titel, die dieses Konto auf diesem Server bis zum Ende abgespielt hat, aus dem Wiedergabeverlauf von Plex, mit ihren Daten und Wiedergabezahlen. Titel, die es als gesehen markiert oder aus seinem Plex-Konto synchronisiert hat, und seine Fortsetzungspunkte bleiben bei Plex.',
      watchDataNeeded: 'Ohne sie wird für diesen Benutzer nichts importiert.',
      extras: {
        title: 'Außerdem übernehmen',
        ratings: 'Bewertungen',
        ratingsHelp:
          'Die Bewertungen jedes Benutzers von 0 bis 10, wie sie sind. Ein Daumen hoch ohne Bewertung wird zum Favoriten. Eine Bewertung, die Polyfin schon hat, bleibt.',
        collections: 'Sammlungen',
        collectionsHelp:
          'Die Sammlungen des Servers, als Sammlungen in Polyfin, die jeder Benutzer sieht, mit den gefundenen Titeln. Eine Sammlung ohne gefundenen Titel wird nicht angelegt.',
        playlists: 'Wiedergabelisten',
        playlistsHelp:
          'Die Video-Wiedergabelisten jedes Benutzers, als seine eigenen Wiedergabelisten, in ihrer Reihenfolge, mit den gefundenen Titeln.',
        plexHelp:
          'Plex gibt Bewertungen und Wiedergabelisten nur für die Konten heraus, die mit ihrem eigenen Token gelesen werden: das des Besitzers, und die der anderen, sobald du dich bei Plex als Besitzer anmeldest.',
      },
      plexNotice:
        'Mit dem Token des Besitzers lässt Plex Polyfin die gesehenen Titel, Fortsetzungspunkte und Bewertungen des Besitzers lesen. Von den anderen Konten gibt es nur ihren Wiedergabeverlauf auf diesem Server: die Titel, die sie hier bis zum Ende abgespielt haben. Titel, die sie ohne Abspielen als gesehen markiert haben, aus ihrem Plex-Konto synchronisierte Titel, ihre Fortsetzungspunkte und ihre Bewertungen stehen nicht darin. Um die eigenen Wiedergabedaten jedes Kontos zu lesen, melde dich oben als Besitzer mit Plex an. Plex hat keine Favoriten zum Importieren.',
      plexOwner: {
        title: 'Jedes Konto mit Plex lesen',
        help: 'Melde dich als Besitzer dieses Servers mit Plex an. Plex gibt Polyfin dann das Token jedes Kontos, für das der Server freigegeben ist, und Polyfin liest damit, was das Konto abgespielt, als gesehen markiert oder synchronisiert hat, seine Fortsetzungspunkte und seine Bewertungen, nicht nur seine Wiedergaben hier. Die Profile deines Plex Home werden mit ihrer PIN gelesen, wenn sie eine haben.',
        signIn: 'Als Besitzer mit Plex anmelden',
        checking: 'Plex wird gefragt, für welche Konten es ein Token gibt…',
        signedInAs: (name: string) => `Bei Plex angemeldet als ${name}, Besitzer des Servers.`,
        signOut: 'Von Plex abmelden',
        notice:
          'Polyfin liest jedes Konto unten mit dem Token, das Plex dem Besitzer dafür gibt, so wie die Plex-Apps dieses Kontos diesen Server lesen. Die Tokens dienen nur diesem Import: Sie werden nie gespeichert, und deine Anmeldung wird bei Plex abgemeldet, sobald der Import endet.',
        all: 'Alles: die abgespielten, als gesehen markierten oder synchronisierten Titel, die Fortsetzungspunkte und die Bewertungen.',
        pin: 'Ein Plex-Home-Profil mit PIN: alles mit seiner PIN, ohne sie nur seine Wiedergaben auf diesem Server.',
        plays:
          'Nur seine Wiedergaben auf diesem Server, aus dem Wiedergabeverlauf von Plex: Plex gibt für dieses Konto kein Token.',
        everything: 'Alles',
        playsOnly: 'Nur Wiedergaben',
        pinLabel: 'PIN des Profils',
        pinHelp: (name: string) =>
          `Die PIN, die ${name} bei Plex verlangt. Leer gelassen werden nur seine Wiedergaben gelesen.`,
      },
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Angemeldet als ${owner ?? 'ein Benutzer'}, liest Polyfin nur die Wiedergabedaten dieses Benutzers.` : `Dieser Schlüssel gehört ${owner ?? 'einem Benutzer'}: Er liest nur die Wiedergabedaten dieses Benutzers.`} Um die eines anderen Benutzers zu importieren, gib das Passwort seines Kontos auf dem Server ein: Polyfin meldet sich als dieser Benutzer an, um sie zu lesen. Ohne Passwort kann sein Konto trotzdem erstellt werden.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Angemeldet als ${owner ?? 'dieser Benutzer'}` : `Dieser Schlüssel gehört ${owner ?? 'einem Benutzer'}`}, der kein Administrator dieses Servers ist: Polyfin importiert nur dieses Konto. Um die anderen Benutzer des Servers aufzulisten, verbinde dich mit dem Konto eines Administrators oder einem API-Schlüssel.`,
      serverPassword: (server: string) => `Passwort auf ${server}`,
      serverPasswordHelp: (user: string) =>
        `Polyfin meldet sich als ${user} an, um die Wiedergabedaten zu lesen, und danach wieder ab. Lass das Feld leer, wenn das Konto keines hat.`,
      keepPassword: 'Dieses Passwort in Polyfin übernehmen',
      keepPasswordHelp:
        'Das neue Konto meldet sich mit demselben Passwort bei Polyfin an, sofern es mindestens 8 Zeichen hat.',
      nothingToImport: 'Wähle einen Benutzer zum Erstellen oder Wiedergabedaten zum Importieren.',
      start: 'Import starten',
      starting: 'Wird gestartet…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'Der Import wurde gestartet.'
          : `${created === 1 ? '1 Benutzer erstellt.' : `${created} Benutzer erstellt.`}${importing ? ' Der Import wurde gestartet.' : ''}`,
      currentTitle: 'Import läuft',
      lastTitle: 'Letzter Import',
      startedLabel: 'Gestartet',
      endedLabel: 'Beendet',
      startedBy: (name: string, own: boolean) =>
        own ? `Eigener Wiedergabeverlauf von ${name}, aus „Mein Konto“` : `Gestartet von ${name}`,
      runningHelp:
        'Du kannst diese Seite verlassen: Der Import läuft weiter. Wird Polyfin zwischenzeitlich neu gestartet, stoppt der Import: Bereits Importiertes bleibt erhalten, und ein erneuter Import fügt nichts doppelt hinzu.',
      stop: 'Import stoppen',
      stopping: 'Wird gestoppt…',
      states: {
        running: 'Läuft',
        done: 'Abgeschlossen',
        stopped: 'Angehalten',
        failed: 'Fehlgeschlagen',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `${server} war während des Imports nicht erreichbar. Was bereits importiert wurde, bleibt erhalten.`,
        jellyfin_key_refused: (server: string) =>
          `${server} hat den API-Schlüssel während des Imports abgelehnt: Er wurde möglicherweise widerrufen. Was bereits importiert wurde, bleibt erhalten.`,
        not_jellyfin: (server: string) =>
          `Der Server hat während des Imports aufgehört, wie ${server} zu antworten. Was bereits importiert wurde, bleibt erhalten.`,
        internal: () =>
          'Bei Polyfin ist während des Imports ein Fehler aufgetreten: Details stehen im Serverprotokoll. Was bereits importiert wurde, bleibt erhalten.',
        jellyfin_user_forbidden: () =>
          'Der Server erlaubt diesem Schlüssel nicht, die Daten dieses Benutzers zu lesen. Importiere sie mit einem API-Schlüssel aus der Übersicht des Servers oder mit dem eigenen Schlüssel des Benutzers.',
      },
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Gib die Adresse des Emby-Servers ein, zum Beispiel http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'Emby hat diesen API-Schlüssel abgelehnt. Erstelle einen in den Einstellungen von Emby unter „API Keys“ und füge ihn erneut ein.',
          jellyfin_sign_in_refused: 'Emby hat diesen Namen oder dieses Passwort abgelehnt.',
          jellyfin_sign_in_forbidden:
            'Emby lässt dieses Konto nicht anmelden: Es ist möglicherweise deaktiviert oder außerhalb seiner erlaubten Zeiten.',
          jellyfin_password_refused:
            'Emby hat dieses Passwort abgelehnt. Lass das Feld leer, wenn das Konto keins hat.',
          jellyfin_unreachable:
            'Unter dieser Adresse hat nichts geantwortet. Überprüfe sie und ob Emby läuft.',
          not_jellyfin:
            'Unter dieser Adresse hat ein Server geantwortet, aber es ist nicht Emby. Überprüfe die Adresse.',
        },
        plex: {
          invalid_jellyfin_address:
            'Gib die Adresse von Plex Media Server ein, zum Beispiel http://192.168.1.10:32400.',
          jellyfin_key_refused:
            'Plex hat dieses Token abgelehnt. Kopiere das Token des Eigentümers erneut.',
          jellyfin_key_limited:
            'Dieses Token darf die Konten des Servers nicht auflisten: Es gehört nicht dem Eigentümer des Servers. Verwende das Token des Eigentümers.',
          jellyfin_unreachable:
            'Unter dieser Adresse hat nichts geantwortet. Überprüfe sie und ob Plex Media Server läuft.',
          not_jellyfin:
            'Unter dieser Adresse hat ein Server geantwortet, aber es ist nicht Plex Media Server. Überprüfe die Adresse.',
        },
      } as Record<'emby' | 'plex', Partial<Record<string, string>>>,
      userStates: {
        waiting: 'Wartet',
        reading: 'Wird gelesen',
        saving: 'Wird gespeichert',
        done: 'Abgeschlossen',
        failed: 'Fehlgeschlagen',
      },
      notImported: 'Nicht importiert',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count === 1 ? '1 Eintrag gelesen' : `${count} Einträge gelesen`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played} als gesehen markiert, ${resumed === 1 ? '1 Fortsetzungspunkt' : `${resumed} Fortsetzungspunkte`}, ${favorites === 1 ? '1 Favorit' : `${favorites} Favoriten`}`,
      upToDate: (count: number) => `${count} bereits in Polyfin`,
      ratings: (count: number) => `${count} bewertet`,
      collections: (count: number) =>
        count === 1 ? '1 Sammlung angelegt' : `${count} Sammlungen angelegt`,
      playlists: (count: number) =>
        count === 1 ? '1 Wiedergabeliste angelegt' : `${count} Wiedergabelisten angelegt`,
      listTitles: (count: number) =>
        count === 1
          ? '1 Titel zu früher übernommenen Listen hinzugefügt'
          : `${count} Titel zu früher übernommenen Listen hinzugefügt`,
      skipped: {
        collection: (name: string) => `Sammlung „${name}“: kein Titel gefunden, nicht angelegt.`,
        playlist: (name: string) =>
          `Wiedergabeliste „${name}“: kein Titel gefunden, nicht angelegt.`,
      },
      watchlist: (count: number) => `${count} zur Merkliste hinzugefügt`,
      undated: (count: number) =>
        count === 1
          ? '1 Fortsetzungspunkt ohne Datum, nicht übernommen'
          : `${count} Fortsetzungspunkte ohne Datum, nicht übernommen`,
      nothingAdded: {
        up_to_date: () => 'Alles Gelesene war bereits in Polyfin: Es gab nichts hinzuzufügen.',
        nothing_played: (server: string) =>
          `${server} listet für diesen Benutzer nichts Gesehenes, Begonnenes oder Favorisiertes.`,
        no_sections: () =>
          'Dieser Plex-Server hat keine Film- oder Serienbibliothek: Es gab nichts zu lesen.',
        plex_owner_nothing_watched: () =>
          'Der Besitzer hat in den Film- und Serienbibliotheken dieses Servers nichts gesehen oder begonnen.',
        plex_nothing_watched: () =>
          'Dieses Konto hat in den Film- und Serienbibliotheken dieses Servers nichts gesehen, begonnen oder bewertet.',
        plex_no_history: () =>
          'Plex listet keine Wiedergabe dieses Kontos auf diesem Server. Sein Wiedergabeverlauf enthält nur die Titel, die auf diesem Server bis zum Ende abgespielt wurden: Ohne Abspielen als gesehen markierte oder aus dem Plex-Konto synchronisierte Titel stehen nicht darin. Um alles zu übernehmen, Fortsetzungspunkte eingeschlossen, melde dich in diesem Import als Besitzer mit Plex an, mit der PIN eines Plex-Home-Profils, das eine hat, oder die Person meldet sich unter Mein Konto › Von einem anderen Server importieren mit Plex an.',
      },
      unmatched: (count: number) =>
        count === 1 ? '1 Titel nicht gefunden' : `${count} Titel nicht gefunden`,
      unmatchedTitle: (name: string) => `Nicht gefundene Titel für ${name}`,
      unmatchedHelp: 'Diese Titel wurden nicht importiert: Der Grund sagt, warum.',
      titleColumn: 'Titel',
      kindColumn: 'Art',
      yearColumn: 'Jahr',
      reasonColumn: 'Grund',
      kinds: {
        movie: 'Film',
        episode: 'Folge',
        series: 'Serie',
      },
      reasons: {
        no_identifier: 'Keine IMDb-, TMDB- oder TVDB-Kennung',
        not_found: 'Nicht in Polyfin',
        not_in_library: 'Nicht mehr in den Bibliotheken von Plex',
        other_episode_order: 'In einer anderen Folgenreihenfolge von TheTVDB nummeriert',
      },
      more: (count: number) => (count === 1 ? 'Und 1 weiterer.' : `Und ${count} weitere.`),
    },
    appImport: {
      open: 'Aus einer App importieren',
      title: 'Aus einer App importieren',
      description:
        'Übernimm die Wiedergabedaten eines Benutzers aus seinem Konto in einer App, die auf Add-ons basiert: die abgespielten Filme und Folgen mit Datum, die Fortsetzungspunkte und die Bibliothek der App als Favoriten, zugeordnet über IMDb, TMDB oder Kitsu. Die Daten in Polyfin werden nur ergänzt, und in die App wird nichts geschrieben. Du brauchst die Anmeldedaten des Benutzers für die App. Benutzer können auch selbst importieren, unter „Mein Konto“.',
      user: 'In das Konto von',
      userHelp: 'Der Polyfin-Benutzer, dessen Wiedergabedaten und Add-ons der Import ergänzt.',
    },
    letterboxdImport: {
      open: 'Aus Letterboxd importieren',
      title: 'Aus Letterboxd importieren',
      description:
        'Übernimm die Filme eines Benutzers aus seiner Letterboxd-Exportdatei: die gesehenen Filme mit Datum und Anzahl der Wiedergaben, die Merkliste, die Bewertungen und auf Wunsch die gemochten Filme als Favoriten. Die Daten in Polyfin werden nur ergänzt. Benutzer können ihre Daten auch selbst importieren, unter „Mein Konto“.',
      user: 'In das Konto von',
      userHelp: 'Der Polyfin-Benutzer, dessen Wiedergabedaten der Import ergänzt.',
    },
  },
  invites: {
    title: 'Einladungslinks',
    create: 'Einladungslink erstellen',
    empty: 'Noch kein Einladungslink vorhanden.',
    emptyHelp:
      'Mit einem Einladungslink können die Personen, denen du ihn schickst, ihr eigenes Konto mit den von dir gewählten Einstellungen erstellen.',
    maxUses: 'Anzahl erstellbarer Konten',
    maxUsesHelp: 'Von 1 bis 100. Standardmäßig 1.',
    expires: 'Läuft ab',
    after: (days: number) => (days === 1 ? 'Nach 1 Tag' : `Nach ${days} Tagen`),
    never: 'Nie',
    accountsEnd: 'Kontoende',
    accountsEndHelp:
      'Jedes über den Link erstellte Konto endet diese Anzahl von Tagen nach seiner Erstellung.',
    accountsEndAfter: (days: number) =>
      days === 1 ? 'Konten enden nach 1 Tag' : `Konten enden nach ${days} Tagen`,
    model: 'Einstellungen von',
    modelHelp:
      'Neue Konten übernehmen die Berechtigungen, sichtbaren Bibliotheken, gesperrten Genres, erlaubten Zeiten, Kindersicherung, Wiedergabe- und Zugriffsbeschränkungen sowie die Qualitätsgruppe dieses Benutzers. Sie sind nie Administratoren.',
    newUser: 'Ein neuer Benutzer, wie bei „Benutzer erstellen“',
    submit: 'Link erstellen',
    creating: 'Wird erstellt…',
    createdTitle: 'Einladungslink erstellt',
    createdNotice: 'Kopiere diesen Link jetzt: Er wird nur dieses eine Mal angezeigt.',
    copy: 'Link kopieren',
    copied: 'Link kopiert.',
    copyFailed: 'Der Link konnte nicht kopiert werden. Markiere ihn und kopiere ihn manuell.',
    done: 'Fertig',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1 ? `${uses} von 1 Konto erstellt` : `${uses} von ${maxUses} Konten erstellt`,
    settingsOf: (name: string) => `Einstellungen von ${name}`,
    newUserSettings: 'Einstellungen für neue Benutzer',
    expiresLabel: 'Läuft ab',
    expiredLabel: 'Abgelaufen',
    neverExpires: 'Läuft nie ab',
    createdBy: (name: string) => `Erstellt von ${name}`,
    createdByDeleted: 'Erstellt von einem gelöschten Benutzer',
    states: {
      active: 'Aktiv',
      used_up: 'Aufgebraucht',
      expired: 'Abgelaufen',
      revoked: 'Widerrufen',
    },
    revoke: 'Widerrufen',
    revokeTitle: 'Diesen Einladungslink widerrufen?',
    revokeConfirm:
      'Der Link funktioniert sofort nicht mehr. Bereits darüber erstellte Konten bleiben erhalten.',
    revoking: 'Wird widerrufen…',
    revoked: 'Der Einladungslink wurde widerrufen.',
  },
  groups: {
    tabsLabel: 'Benutzer und Gruppen',
    title: 'Gruppen',
    description:
      'Eine Gruppe bündelt die Einschränkungen der Seite eines Benutzers. Ihre Mitglieder folgen ihr: Eine Änderung an der Gruppe gilt sofort für alle.',
    listTitle: 'Gruppen',
    empty: 'Noch keine Gruppe.',
    emptyHelp:
      'Erstelle eine Gruppe, um Berechtigungen, Wiedergabebeschränkungen, Kindersicherung, Bibliotheken, Genres und Zeiten mehrerer Konten auf einmal festzulegen.',
    create: 'Gruppe erstellen',
    name: 'Name',
    nameTaken: 'Dieser Name wird bereits von einer anderen Gruppe verwendet.',
    nameRule: '1 bis 64 Zeichen, verschieden von den Namen der anderen Gruppen.',
    copyFrom: 'Einstellungen von',
    copyFromHelp:
      'Die Gruppe übernimmt anfangs die Berechtigungen, Wiedergabe- und Zugriffsbeschränkungen, Qualitätsgruppe, Kindersicherung, sichtbaren Bibliotheken, gesperrten Genres und erlaubten Zeiten dieses Benutzers.',
    submit: 'Gruppe erstellen',
    creating: 'Wird erstellt…',
    created: (name: string) => `Die Gruppe ${name} wurde erstellt.`,
    memberCount: (count: number) =>
      count === 0 ? 'Kein Mitglied' : count === 1 ? '1 Mitglied' : `${count} Mitglieder`,
    notFound: 'Gruppe nicht gefunden',
    notFoundHelp: 'Sie wurde möglicherweise gelöscht.',
    backToGroups: 'Zurück zu den Gruppen',
    sectionsLabel: 'Bereiche der Gruppe',
    pageHelp:
      'Jeder Bereich wird einzeln gespeichert und gilt sofort für die Mitglieder, die der Gruppe folgen.',
    rename: 'Umbenennen',
    renamed: 'Die Gruppe wurde umbenannt.',
    updated: 'Die Gruppe wurde aktualisiert.',
    accessHelp:
      'Die Berechtigungen, die Mitglieder von der Gruppe übernehmen. Ihre Rolle, der Anmeldebildschirm, „Deaktiviert“ und das Enddatum bleiben auf ihrer eigenen Seite.',
    delete: 'Gruppe löschen',
    deleteTitle: (name: string) => `Gruppe ${name} löschen?`,
    deleteConfirm:
      'Ihre Mitglieder behalten die Werte der Gruppe als eigene Einstellungen, die dann auf ihren Seiten geändert werden können.',
    deleting: 'Wird gelöscht…',
    deleted: (name: string) => `Die Gruppe ${name} wurde gelöscht.`,
    membersTitle: 'Mitglieder',
    membersHelp:
      'Ein Mitglied, das der Gruppe nicht mehr folgt, behält ihre Werte als eigene Einstellungen, gekennzeichnet mit „Eigene Einstellungen“.',
    noMembers: 'Noch kein Mitglied.',
    noMembersHelp:
      'Füge hier Mitglieder hinzu oder wähle die Gruppe im Bereich „Zugriff“ auf der Seite eines Benutzers.',
    ownSettings: 'Eigene Einstellungen',
    followsHelp: 'Folgt der Gruppe',
    ownSettingsHelp: 'Behält eigene Einstellungen',
    addMembers: 'Mitglieder hinzufügen',
    addMembersTitle: (name: string) => `Mitglieder zu ${name} hinzufügen`,
    addMembersHelp:
      'Die ausgewählten Benutzer folgen der Gruppe: Sie übernehmen deren Werte anstelle ihrer eigenen. Administratoren können keiner Gruppe beitreten.',
    noCandidates: 'Niemand zum Hinzufügen.',
    noCandidatesHelp:
      'Alle Benutzer, die keine Administratoren sind, sind bereits in dieser Gruppe.',
    movesFrom: (name: string) => `Verlässt die Gruppe ${name}`,
    add: 'Hinzufügen',
    adding: 'Wird hinzugefügt…',
    membersAdded: (count: number) =>
      count === 1 ? '1 Mitglied wurde hinzugefügt.' : `${count} Mitglieder wurden hinzugefügt.`,
    group: 'Gruppe',
    noGroup: 'Keine Gruppe',
    groupHelp:
      'Ein Mitglied folgt der Gruppe: Ihre Werte gelten, und diese Seite zeigt sie schreibgeschützt an. Beim Verlassen bleiben sie als eigene Einstellungen des Benutzers erhalten.',
    administratorNoGroup:
      'Administratoren können keiner Gruppe beitreten, da sich ihre Berechtigungen unterscheiden.',
    administratorLeaves: (name: string) =>
      `Wird dieser Benutzer zum Administrator, verlässt er die Gruppe ${name}.`,
    newAccountGroupHelp: 'Die erstellten Konten treten dieser Gruppe bei und folgen ihr.',
    newUserGroupHelp: 'Der Benutzer tritt dieser Gruppe bei und folgt ihr.',
    followGroup: 'Der Gruppe folgen',
    followGroupHelp:
      'Wenn ausgeschaltet, behält der Benutzer die aktuellen Werte der Gruppe als eigene, und diese Seite kann sie ändern.',
    setByGroup: (name: string) => `Von der Gruppe ${name} festgelegt.`,
    openGroup: 'Gruppe öffnen',
    joinTitle: 'Der Gruppe beitreten?',
    joinConfirm: (user: string, group: string) =>
      `${user} tritt ${group} bei und folgt der Gruppe: Ihre Werte ersetzen die eigenen in „Zugriff“, „Wiedergabe und Zugriff“, „Kindersicherung“, „Sichtbare Bibliotheken“, „Gesperrte Genres“ und „Erlaubte Zeiten“.`,
    join: 'Beitreten',
    followAgainTitle: 'Der Gruppe wieder folgen?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} übernimmt wieder die Werte von ${group}: Die eigenen Einstellungen in den Bereichen der Gruppe werden ersetzt.`,
    followAgain: 'Der Gruppe folgen',
    groupBadge: (name: string) => `Gruppe ${name}`,
    groupOwnBadge: (name: string) => `Gruppe ${name}, eigene Einstellungen`,
    filter: 'Benutzer anzeigen von',
    allGroups: 'Alle Gruppen',
    noneShown: 'Hier gibt es keine Benutzer.',
  },
  resetDefaults: {
    button: 'Auf Standardwerte zurücksetzen',
    title: (name: string) => `Die Einstellungen von ${name} zurücksetzen?`,
    groupTitle: (name: string) => `Die Einstellungen der Gruppe ${name} zurücksetzen?`,
    userHelp: (name: string) =>
      `Die Berechtigungen unter Zugriff, Wiedergabe und Zugriff mit der Qualitätsgruppe, Kindersicherung, Sichtbare Bibliotheken, Gesperrte Genres und Erlaubte Zeiten erhalten wieder die Einstellungen eines neuen Benutzers, die „Benutzer erstellen“ vergibt; bereits angemeldete Apps von ${name} haben sie sofort. Name und Passwort, Administrator, Auf dem Anmeldebildschirm anzeigen, Deaktiviert, Endet am, die Gruppe, die eigenen Addons, Verbindungen, Tracking-Konten und alles Angesehene bleiben.`,
    ownSettingsHelp: (group: string) =>
      `Der Benutzer bleibt mit eigenen Einstellungen in der Gruppe ${group}, ohne ihr zu folgen.`,
    groupHelp:
      'Die Berechtigungen unter Zugriff, Wiedergabe und Zugriff mit der Qualitätsgruppe, Kindersicherung, Sichtbare Bibliotheken, Gesperrte Genres und Erlaubte Zeiten erhalten wieder die Einstellungen eines neuen Benutzers. Name und Mitglieder der Gruppe bleiben.',
    followers: (count: number) =>
      count === 0
        ? 'Kein Mitglied folgt der Gruppe: Die Änderung erreicht niemanden.'
        : count === 1
          ? '1 Mitglied folgt der Gruppe und übernimmt diese Einstellungen sofort.'
          : `${count} Mitglieder folgen der Gruppe und übernehmen diese Einstellungen sofort.`,
    changesTitle: 'Was sich ändert',
    loading: 'Änderungen werden ermittelt…',
    noChange: 'Nichts ändert sich: Das sind bereits die Einstellungen eines neuen Benutzers.',
    confirm: 'Zurücksetzen',
    resetting: 'Wird zurückgesetzt…',
    done: (name: string) => `Die Einstellungen von ${name} wurden zurückgesetzt.`,
    groupDone: (name: string) => `Die Einstellungen der Gruppe ${name} wurden zurückgesetzt.`,
  },
}

export default users
