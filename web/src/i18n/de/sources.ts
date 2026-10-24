import type en from '../en/sources'

const sources: typeof en = {
  sources: {
    title: 'Quellen',
    lede: 'Stremio-Add-ons, Eclipse-Add-ons, IPTV-Quellen und lokale Ordner in einer Liste. Die Quellen des Servers stehen allen Benutzern zur Verfügung; die eigenen Quellen eines Mitglieds nur diesem.',
    add: 'Quelle hinzufügen',
    listLabel: 'Quellen',
    listNote: 'Eigene Quellen von Mitgliedern stehen nur ihrem Besitzer zur Verfügung.',
    loading: 'Quellen werden geladen',
    kindFilter: 'Nach Art filtern',
    all: 'Alle',
    kinds: { stremio: 'Stremio', eclipse: 'Eclipse', iptv: 'IPTV', local: 'Ordner' },
    owner: 'Besitzer',
    everyOwner: 'Alle Besitzer',
    server: 'Der Server',
    search: 'Quellen durchsuchen',
    searchPlaceholder: 'Nach Name suchen',
    clearFilters: 'Filter zurücksetzen',
    emptyTitle: 'Noch keine Quelle',
    emptyShared:
      'Füge ein Stremio-Add-on, ein Eclipse-Add-on, eine IPTV-Quelle oder einen lokalen Ordner hinzu: Sie steht allen Benutzern dieses Servers zur Verfügung.',
    emptyMine:
      'Füge ein Stremio-Add-on, ein Eclipse-Add-on oder eine IPTV-Quelle hinzu: Sie steht nur dir zur Verfügung, nach denen des Servers.',
    noMatchTitle: 'Keine passende Quelle',
    noMatch: 'Ändere die Art, den Besitzer oder die Suche.',
    tag: {
      stremio: 'Stremio',
      eclipse: 'Eclipse',
      m3u: 'M3U',
      xtream: 'Xtream',
      hdhomerun: 'HDHomeRun',
      local: 'Ordner',
    },
    kindName: {
      stremio: 'Stremio-Add-on',
      eclipse: 'Eclipse-Add-on',
      m3u: 'M3U-Wiedergabeliste',
      xtream: 'Xtream-Codes-Konto',
      hdhomerun: 'HDHomeRun-Tuner',
      local: 'Lokaler Ordner',
    },
    ownedBy: (name: string) => `Quelle von ${name}`,
    yours: 'Deine eigene Quelle',
    active: 'Aktiv',
    off: 'Aus',
    failed: 'Download fehlgeschlagen',
    channelsShown: (count: string) => `${count} Kanäle angezeigt`,
    notDownloaded: 'Noch nicht heruntergeladen',
    moreActions: (name: string) => `Weitere Aktionen für ${name}`,
    openPage: 'Seite öffnen',
    enabled: 'Aktiviert',
    enabledHelp:
      'Wenn du eine Quelle deaktivierst, werden ihre Bibliotheken in Jellyfin-Apps ausgeblendet, ohne verloren zu gehen.',
    order: 'Reihenfolge',
    position: (position: number, count: number) => `${position} von ${count}`,
    moveUp: (name: string) => `${name} nach oben verschieben`,
    moveDown: (name: string) => `${name} nach unten verschieben`,
    moved: (name: string, position: number, count: number) =>
      `${name} wurde auf Position ${position} von ${count} verschoben.`,
    description: 'Beschreibung',
    details: 'Details',
    manifestUrl: 'Manifest-Adresse',
    address: 'Adresse',
    version: 'Version',
    provides: 'Bietet',
    types: 'Typen',
    catalogs: 'Kataloge',
    catalogCount: (count: number) => (count === 1 ? '1 Katalog' : `${count} Kataloge`),
    lastRefresh: 'Letzte Aktualisierung',
    channels: 'Kanäle',
    notImported: 'Nicht importiert',
    shownOf: (shown: string, total: string) => `${shown} von ${total} angezeigt`,
    vod: 'Filme und Serien',
    vodShort: (movies: string, series: string) => `${movies} Filme und ${series} Serien importiert`,
    lastFetch: 'Letzter Download',
    nextFetch: 'Nächster Download',
    never: 'Nie',
    figures: {
      entries: 'Einträge in der Quelle',
      shown: 'Angezeigte Kanäle',
      mapped: 'Mit Fernsehprogramm',
      unmapped: 'Ohne Fernsehprogramm',
    },
    openSource: 'Quelle öffnen',
    iptvHint:
      'Was importiert wird, Kategorien, Kanäle, Nummern und Fernsehprogramme legst du auf der Seite der Quelle fest.',
    refresh: 'Aktualisieren',
    refreshLabel: (name: string) => `${name} aktualisieren`,
    refreshed: (name: string) => `${name}: Das Manifest wurde erneut heruntergeladen.`,
    refreshedList: (name: string) => `${name}: Die Liste wurde erneut heruntergeladen.`,
    replace: 'Adresse ersetzen',
    newManifestUrl: 'Neue Manifest-Adresse',
    replaceHint:
      'Verwende dies, nachdem du das Add-on auf seiner Konfigurationsseite neu eingerichtet hast. Seine Bibliotheken bleiben erhalten.',
    replaceSubmit: 'Ersetzen',
    replaced: 'Die Manifest-Adresse wurde ersetzt.',
    language: 'Sprache der Beschreibungen',
    languageHelp:
      'Lass beim einzigen Metadaten-Add-on des Servers „Keine“ eingestellt, egal welche Sprache seine Konfigurationsseite festlegt: Ein Add-on mit einer Sprache beschreibt Titel nur für die Benutzer, die diese Sprache gewählt haben, es sei denn, kein Add-on mit „Keine“ beschreibt sie. Damit Benutzer Titel in anderen Sprachen lesen können, füge dasselbe Metadaten-Add-on für jede Sprache ein weiteres Mal hinzu, jeweils auf seiner Konfigurationsseite in dieser Sprache eingerichtet, und gib die Sprache nur bei diesen zusätzlichen Kopien hier an: Benutzer, die diese Metadatensprache wählen, sehen dessen Namen, Beschreibungen und Bilder. Ein Add-on mit einer Sprache und ohne eigene Bibliothek beschreibt nur Titel.',
    languageNone: 'Keine',
    languageSaved: (name: string) => `${name}: Die Sprache der Beschreibungen wurde gespeichert.`,
    edit: 'Konto bearbeiten',
    saved: 'Quelle gespeichert.',
    settings: 'Einstellungen',
    remove: 'Entfernen',
    removeTitle: (name: string) => `${name} entfernen?`,
    removeShared:
      'Ihre Bibliotheken verschwinden für alle Benutzer aus den Jellyfin-Apps. Dies kann nicht rückgängig gemacht werden.',
    removeMine:
      'Ihre Bibliotheken verschwinden aus deinen Jellyfin-Apps. Dies kann nicht rückgängig gemacht werden.',
    removed: (name: string) => `${name} wurde entfernt.`,
    readOnly: (name: string) =>
      `Dies ist eine eigene Quelle von ${name}. Nur diese Person kann sie ändern, über ihre Seite „Meine Quellen“.`,
    back: 'Quellen',
    backMine: 'Meine Quellen',
    notFoundTitle: 'Quelle nicht gefunden',
    notFound: 'Diese Quelle existiert nicht oder du kannst sie nicht verwalten.',
  },
  sourceAdd: {
    title: 'Quelle hinzufügen',
    kind: 'Art der Quelle',
    kinds: {
      stremio: 'Stremio-Add-on',
      music: 'Eclipse-Add-on',
      iptv: 'IPTV-Quelle',
      tuner: 'HDHomeRun-Tuner',
      local: 'Ordner',
    },
    stremioHelp:
      'Ein Stremio-Add-on liefert Kataloge, Metadaten und Streams. Füge seinen Installationslink ein.',
    musicHelp:
      'Ein Eclipse-Musik-Add-on liefert Musik, Hörbücher oder Podcasts. Füge seine Manifest-Adresse ein.',
    manifestUrl: 'Manifest-Adresse',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Kopiere den Installationslink von der Konfigurationsseite des Add-ons. Er enthält oft deine Einstellungen oder Schlüssel: Polyfin hält ihn privat und zeigt immer nur eine gekürzte Form an.',
    install: 'Installieren',
    installed: (name: string) => `${name} wurde installiert.`,
    installedNoLibraries: (name: string) =>
      `${name} wurde installiert. Noch keiner seiner Kataloge ist eine Bibliothek: Aktiviere sie unter Inhalte › Bibliotheken.`,
    installedNoLibrariesMine: (name: string) =>
      `${name} wurde installiert. Noch keiner seiner Kataloge ist eine Bibliothek: Aktiviere sie unter Meine Quellen.`,
    iptvHelp:
      'Importiere die Kanäle einer M3U-Wiedergabeliste oder eines Xtream-Codes-Kontos direkt. Sie erscheinen im Live-TV wie ein TV-Katalog, den du unter Bibliotheken anordnen kannst. Die Liste wird in dem Intervall neu heruntergeladen, das unter Einstellungen › Live-TV festgelegt ist.',
    steps: 'Schritte',
    stepAccount: 'Konto',
    stepImport: 'Was importiert wird',
    name: 'Name',
    account: 'Konto',
    m3u: 'M3U-Wiedergabeliste',
    xtream: 'Xtream-Codes-Konto',
    playlistUrl: 'Adresse der Wiedergabeliste',
    playlistUrlHint:
      'Die M3U-Adresse, die dein Anbieter dir gibt. Sie kann deine Zugangsdaten enthalten: Sie wird nie vollständig angezeigt.',
    server: 'Serveradresse',
    serverHint: 'Zum Beispiel https://server:8080, so wie dein Anbieter sie angibt.',
    username: 'Benutzername',
    password: 'Passwort',
    keepHint: 'Leer lassen, um die aktuelle beizubehalten.',
    passwordKeepHint: 'Leer lassen, um das aktuelle Passwort beizubehalten.',
    providerGuide: 'Fernsehprogramm des Anbieters verwenden',
    providerGuideHelp:
      'Xtream-Codes-Server veröffentlichen ein Fernsehprogramm für das Konto: Es füllt das Programm der Kanäle.',
    guideUrl: 'Adresse des Fernsehprogramms (XMLTV, optional)',
    guideUrlHint:
      'Unkomprimierte oder komprimierte XMLTV-Datei. Du kannst sie auch später unter Bibliotheken hinzufügen oder ändern.',
    next: 'Weiter: Auswählen, was importiert wird',
    back: 'Zurück zum Konto',
    import: 'Importieren',
    added: (name: string, parts: string[]) => `${name} wurde hinzugefügt: ${parts.join(', ')}.`,
    channels: (count: string) => `${count} Kanäle`,
    movies: (count: string) => `${count} Filme`,
    series: (count: string) => `${count} Serien`,
    tunerHelp:
      'Ein HDHomeRun-Tuner in deinem Netzwerk liefert die Kanäle, die er über Antenne oder Kabel empfängt. Sie erscheinen im Live-TV wie die einer IPTV-Quelle, und der Tuner spielt so viele Kanäle gleichzeitig ab, wie er Tuner hat.',
    findTuners: 'Tuner suchen',
    findTunersHelp:
      'Fragt das lokale Netzwerk, welche Tuner vorhanden sind. Das dauert einige Sekunden.',
    finding: 'Lokales Netzwerk wird abgefragt…',
    foundTitle: 'Tuner im Netzwerk',
    tunerCount: (count: number, formatted: string) =>
      count === 1 ? '1 Tuner' : `${formatted} Tuner`,
    addTuner: 'Hinzufügen',
    addTunerLabel: (name: string) => `${name} hinzufügen`,
    tunerAdded: 'Hinzugefügt',
    openTuner: (name: string) => `${name} öffnen`,
    noTuners: 'Kein Tuner hat geantwortet.',
    noTunersHelp:
      'Unter Docker erkennt Polyfin Tuner nur mit Host-Netzwerk (network_mode: host). Du kannst einen Tuner auch über seine Adresse hinzufügen.',
    byAddress: 'Über Adresse hinzufügen',
    tunerAddress: 'Tuner-Adresse',
    tunerAddressHint: 'Seine IP-Adresse, etwa 192.168.1.20, oder seine http://-Adresse.',
    tunerAddressInvalid:
      'Gib die IP-Adresse des Tuners ein, etwa 192.168.1.20, oder seine http://-Adresse.',
    tunerName: 'Name (optional)',
    tunerNameHint: 'Leer gelassen, wird der eigene Name des Tuners verwendet.',
    favoritesOnly: 'Nur die Lieblingskanäle des Tuners',
    favoritesOnlyHelp:
      'Behält die Kanäle, die auf dem Tuner, in seiner App oder auf seiner Webseite als Favoriten markiert sind, und lässt die anderen weg.',
  },
  localFolders: {
    help: {
      container:
        'Ein im Container von Polyfin eingebundener Ordner, Lesezugriff genügt. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons; ein Ordner mit Musik wird zu einer Musikbibliothek. Polyfin läuft als Benutzer 65532, der ihn lesen können muss.',
      computer:
        'Ein Ordner auf dem Computer, auf dem Polyfin läuft, oder auf einem Laufwerk oder einer Freigabe, die er erreicht, Lesezugriff genügt. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons; ein Ordner mit Musik wird zu einer Musikbibliothek. Das Konto, unter dem Polyfin läuft, muss ihn lesen können.',
    },
    location: 'Ort',
    locations: {
      local: 'Lokaler Ordner',
      smb: 'SMB-Freigabe',
      webdav: 'WebDAV-Ordner',
      s3: 'S3-Bucket',
      sftp: 'SFTP-Ordner',
      rclone: 'rclone-Remote',
    },
    shareHelp: {
      smb: 'Ein Ordner, den ein NAS oder ein anderer Computer über SMB (Windows-Dateifreigabe) freigibt und den Polyfin selbst liest, ohne ihn einzubinden. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons; ein Ordner mit Musik wird zu einer Musikbibliothek.',
      webdav:
        'Ein Ordner, den ein Server über WebDAV freigibt und den Polyfin selbst liest, ohne ihn einzubinden. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons; ein Ordner mit Musik wird zu einer Musikbibliothek.',
      s3: 'Ein Präfix eines Buckets in S3-kompatiblem Speicher, den Polyfin selbst liest, ohne ihn einzubinden. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons, gelesen über den Cache der gerade gelesenen Dateien; ein Ordner mit Musik wird zu einer Musikbibliothek.',
      sftp: 'Ein Ordner auf einem über SSH erreichbaren Server, den Polyfin selbst liest, ohne ihn einzubinden. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons, gelesen über den Cache der gerade gelesenen Dateien; ein Ordner mit Musik wird zu einer Musikbibliothek.',
      rclone:
        'Ein Ordner auf einem der Speicher, die rclone erreicht, gelesen über eine rclone-Instanz, die rclone rcd --rc-serve ausführt, etwa ein Container neben Polyfin. Seine Videos werden anhand ihrer Namen Titeln zugeordnet und als Versionen dieser Titel abgespielt, neben den Streams der Add-ons, gelesen über den Cache der gerade gelesenen Dateien; ein Ordner mit Musik wird zu einer Musikbibliothek.',
    },
    address: 'Adresse',
    addressHint: {
      smb: 'smb://, dann der Server, die Freigabe und gegebenenfalls der Ordner darin. Füge :port nach dem Server hinzu, wenn ein anderer Port als 445 verwendet wird.',
      webdav:
        'Die WebDAV-Adresse des Ordners. Nimm lieber https://, da http:// das Passwort unverschlüsselt sendet.',
      s3: 's3://, dann der Bucket und gegebenenfalls das Präfix darin. Seine Objekte werden nach / als Ordner aufgelistet.',
      sftp: 'sftp://, dann der Server und der vollständige Pfad des Ordners darauf. Füge :port nach dem Server hinzu, wenn ein anderer Port als 22 verwendet wird.',
      rclone: '',
    },
    addressPlaceholder: {
      smb: 'smb://nas.local/media/filme',
      webdav: 'https://nas.local/dav/filme',
      s3: 's3://media/filme',
      sftp: 'sftp://nas.local/srv/media/filme',
      rclone: '',
    },
    user: 'Benutzer',
    userHint: {
      smb: 'Leer für eine Freigabe, die für Gäste offen ist. DOMÄNE\\benutzer für ein Domänenkonto.',
      webdav: 'Leer für einen Ordner, der für alle offen ist.',
      s3: 'Beide leer für einen Bucket, den alle lesen dürfen.',
      sftp: 'Das Konto, mit dem sich Polyfin anmeldet.',
      rclone:
        'Der Benutzer der Fernsteuerung von rclone (--rc-user), leer, wenn keiner festgelegt ist.',
    },
    password: 'Passwort',
    passwordHint: 'Polyfin zeigt es nie wieder an.',
    sftpPasswordHint:
      'Leer, wenn die Anmeldung mit dem privaten Schlüssel erfolgt. Polyfin zeigt es nie wieder an.',
    accessKey: 'Zugriffsschlüssel',
    secretKey: 'Geheimer Schlüssel',
    endpoint: 'Endpunkt',
    endpointHint:
      'Die Adresse des S3-Dienstes des Speichers. Polyfin nennt den Bucket im Pfad, wie es jeder S3-kompatible Speicher akzeptiert, außer bei den Endpunkten von Amazon selbst.',
    region: 'Region',
    regionHint: 'Leer für us-east-1, was die meisten S3-kompatiblen Speicher akzeptieren.',
    privateKey: 'Privater Schlüssel',
    privateKeyHint:
      'Ein privater OpenSSH- oder PEM-Schlüssel, anstelle des Passworts oder zusätzlich dazu. Polyfin zeigt ihn nie wieder an.',
    privateKeySaved: 'Ein Schlüssel ist gespeichert. Füge einen anderen ein, um ihn zu ersetzen.',
    privateKeyNotSent:
      'Der gespeicherte Schlüssel wird nicht an einen anderen Server gesendet: Füge ihn erneut ein, sonst wird er beim Speichern entfernt.',
    secretNotSent:
      'Der gespeicherte Wert wird nicht an einen anderen Server gesendet: Gib ihn erneut ein, sonst wird er beim Speichern entfernt.',
    privateKeyRemove: 'Gespeicherten Schlüssel entfernen',
    passphrase: 'Passphrase',
    passphraseHint: 'Nur für einen privaten Schlüssel, der eine hat.',
    hostKey: 'Hostschlüssel',
    hostKeyHint:
      'Wird bei der ersten Verbindung gespeichert. Polyfin lehnt den Server ab, wenn er einen anderen anbietet.',
    hostKeyNone: 'Wird bei der ersten Verbindung gespeichert',
    hostKeyOffered: (fingerprint: string) =>
      `Der Server bietet jetzt einen anderen Hostschlüssel an, ${fingerprint}, und Polyfin lehnt ihn ab. Akzeptiere ihn nur, wenn der Server neu installiert oder sein Schlüssel absichtlich geändert wurde.`,
    acceptHostKey: 'Neuen Schlüssel akzeptieren',
    hostKeyAccepted: 'Der neue Hostschlüssel wurde akzeptiert. Polyfin scannt den Ordner erneut.',
    rcloneInstance: 'rclone-Adresse',
    rcloneInstanceHint:
      'Wo rclone rcd --rc-serve antwortet, erreichbar über http://, etwa http://rclone:5572 für einen Container neben Polyfin.',
    rcloneRemote: 'Remote',
    rcloneRemoteHint: 'Eines der Remotes aus der Konfiguration von rclone.',
    listRemotes: 'Remotes auflisten',
    remotesListed: (count: number, figure: string) =>
      count === 1 ? 'rclone hat ein Remote.' : `rclone hat ${figure} Remotes.`,
    noRemotes: 'rclone hat kein Remote in seiner Konfiguration.',
    remotesFailed: {
      unreachable: 'rclone antwortet unter dieser Adresse nicht.',
      refused: 'rclone lehnt diesen Benutzer oder dieses Passwort ab.',
    } as Record<string, string>,
    rcloneFolder: 'Pfad im Remote',
    rcloneFolderHint:
      'Der Ordner des Remotes, der gelesen wird. Leer lassen für das Stammverzeichnis.',
    pollMinutes: 'Intervall der Änderungssuche (Minuten, 0 = nur bei Scans)',
    pollMinutesHint: {
      s3: 'Die Ordner des Buckets werden in diesem Abstand erneut aufgelistet, um hinzugefügte und entfernte Dateien zu finden. Anbieter können jede Auflistung berechnen. Standardmäßig 30.',
      sftp: 'Der Ordner und seine Unterordner werden in diesem Abstand erneut aufgelistet, um hinzugefügte und entfernte Dateien zu finden. Standardmäßig 2.',
      rclone:
        'Die Ordner des Remotes werden in diesem Abstand erneut aufgelistet, um hinzugefügte und entfernte Dateien zu finden. Standardmäßig 2.',
    },
    name: 'Name',
    nameHint: 'Der Name seiner Bibliothek in Jellyfin-Apps.',
    path: { container: 'Pfad im Container', computer: 'Pfad' },
    pathHint: {
      container:
        'Wo der Ordner im Container von Polyfin eingebunden ist, nicht sein Pfad auf dem Host.',
      linux: 'Der vollständige Pfad des Ordners auf dem Computer, auf dem Polyfin läuft.',
      macos:
        'Der vollständige Pfad des Ordners auf dem Mac, auf dem Polyfin läuft. Ordner auf externen Laufwerken liegen unter /Volumes.',
      windows:
        'Der vollständige Pfad des Ordners auf dem PC, auf dem Polyfin läuft: auf einem Laufwerk, etwa D:\\Movies, oder auf einer Freigabe, etwa \\\\nas\\media\\Movies.',
    },
    pathPlaceholder: {
      container: '/media/movies',
      linux: '/srv/media/movies',
      macos: '/Users/me/Movies',
      windows: 'D:\\Movies',
    },
    kind: 'Inhalt',
    kinds: { movies: 'Filme', shows: 'Serien', music: 'Musik', homevideos: 'Heimvideos' },
    kindHelp: {
      movies:
        'Eine Datei pro Film, einzeln oder in einem eigenen Ordner, benannt nach dem Film und seinem Erscheinungsjahr.',
      shows: 'Ein Ordner pro Serie, mit Staffel und Folge im Namen jeder Datei, etwa S01E02.',
      music:
        'Audiodateien werden anhand ihrer Tags gelesen, sonst anhand ihrer Namen, etwa Artist/Album/01 Title.flac, und zu Alben und Interpreten gruppiert.',
      homevideos:
        'Jedes Video ist ein eigener Titel, benannt nach seiner Datei oder nach seinem Ordner, wenn es allein darin liegt, und datiert nach seiner NFO-Datei, seiner Aufnahme oder der Datei. Es wird kein Metadaten-Add-on abgefragt.',
    },
    kindNouns: { movies: 'Filme', shows: 'Serien', music: 'Musik', homevideos: 'Heimvideos' },
    changeKind: 'Ändern',
    changeKindTitle: (name: string) => `Inhalt von ${name} ändern?`,
    changeKindScan: (from: string, to: string) =>
      `Er enthält ${from}, künftig wird er ${to} enthalten. Der Ordner und seine Bibliothek behalten ihre Kennungen, ihre Adresse und ihre Einstellungen, und Polyfin scannt ihn im Hintergrund erneut als ${to}.`,
    changeKindFollows:
      'Was Benutzer mit jedem Video gemacht haben (Gesehen-Status, Fortsetzungspunkt und Favorit), geht auf seinen neuen Titel über: Gesehen-Markierungen und Favoriten werden übernommen und bleiben auch beim bisherigen Titel erhalten. Der Fortsetzungspunkt wandert mit der Datei, der bisherige Titel verliert ihn.',
    changeKindNothing:
      'Nichts wird übernommen: Songs und Videos sind andere Dateien. Was Benutzer mit seinen Titeln gemacht haben, bleibt bei diesen und wird nicht mehr angezeigt.',
    changeKindBack:
      'Seine Heimvideos werden 30 Tage lang gemerkt: Enthält er innerhalb dieser Zeit wieder Heimvideos, führt er sie als dieselben Titel auf, samt dem, was Benutzer mit ihnen gemacht haben.',
    linksDropped: (count: number, figure: string) =>
      count === 1
        ? 'die manuell erstellte Zuordnung'
        : `die ${figure} manuell erstellten Zuordnungen`,
    episodesDropped: (count: number, figure: string) =>
      count === 1
        ? 'die dem Titel einer Disc zugewiesenen Folgen'
        : `die ${figure} Disc-Titeln zugewiesenen Folgen`,
    changeKindDrops: (parts: string[]) =>
      `Entfernt, da für den bisherigen Inhalt erstellt: ${parts.join(' und ')}.`,
    useArtwork: 'Eigenes Bildmaterial verwenden',
    useArtworkHelp:
      'Poster, Hintergründe und Logos neben den Dateien, etwa poster.jpg oder Movie (2001)-fanart.jpg, werden für ihre Titel anstelle derer der Metadaten-Add-ons angezeigt, und Bilder von Folgen anstelle ihrer Standbilder. Aus: Die der Add-ons werden angezeigt.',
    readExtras: 'Extras und Theme-Medien einlesen',
    readExtrasHelp:
      'Extras, lokale Trailer und Titelmusik neben den Titeln, benannt wie bei Jellyfin, etwa Extras/Making of.mkv, Movie (2001)-trailer.mkv oder theme.mp3, erscheinen im Bonusmaterial ihrer Titel, werden über die Schaltfläche „Trailer“ abgespielt oder laufen im Hintergrund ihrer Seiten. Aus: Der nächste Scan lässt sie aus.',
    add: 'Ordner hinzufügen',
    added: (name: string) => `${name} wurde hinzugefügt. Der erste Scan hat begonnen.`,
    edit: 'Ordner bearbeiten',
    save: 'Speichern',
    cancel: 'Abbrechen',
    saved: 'Ordner gespeichert.',
    movedHint:
      'Bei einem neuen Pfad werden die im bisherigen gefundenen Dateien vergessen, einschließlich Zuordnungen.',
    movedShareHint:
      'Bei einer neuen Adresse werden die in der bisherigen gefundenen Dateien vergessen, einschließlich Zuordnungen.',
    scan: 'Jetzt scannen',
    scanLabel: (name: string) => `${name} jetzt scannen`,
    scanStarted: (name: string) => `${name}: Der Scan hat begonnen.`,
    scanning: 'Wird gescannt',
    unreadable: 'Nicht lesbar',
    figuresTitle: 'Dateien',
    files: 'Videodateien',
    matched: 'Einem Titel zugeordnet',
    unmatched: 'Nicht zugeordnet',
    extras: 'Extras',
    tracks: 'Titel',
    albums: 'Alben',
    artists: 'Interpreten',
    gained: 'Mit Lautstärkeausgleich',
    measured: 'Gemessen',
    leftOut: 'Ausgelassen',
    videos: 'Videos',
    datedBy: {
      nfo: 'Datiert durch eine NFO-Datei',
      created: 'Datiert durch die Aufnahme',
      file: 'Datiert durch die Datei',
    },
    matchedBy: (parts: string[]) => `Zugeordnet: ${parts.join(', ')}.`,
    matchMethods: {
      name: (count: string) => `${count} über eine Kennung im Namen`,
      nfo: (count: string) => `${count} über eine NFO-Datei`,
      search: (count: string) => `${count} über eine Suche`,
      year: (count: string) => `${count} über eine Suche, ein Jahr abweichend`,
      numerals: (count: string) => `${count} über eine Suche, Zahlen gleich gelesen`,
      yearNumerals: (count: string) =>
        `${count} über eine Suche, ein Jahr abweichend und Zahlen gleich gelesen`,
      link: (count: string) => `${count} manuell`,
    },
    holds: 'Inhalt',
    lastScan: 'Letzter Scan',
    lastAttempt: 'Letzter Versuch',
    never: 'Noch nicht',
    errorTitle: 'Polyfin kann diesen Ordner nicht lesen',
    errorTitleShare: 'Polyfin kann diese Freigabe nicht lesen',
    errors: {
      missing:
        'Der Pfad existiert dort nicht, wo Polyfin läuft: Prüfe, ob der Ordner (oder das Laufwerk bzw. die Freigabe, auf der er liegt) vorhanden ist und unter Docker im Container eingebunden ist.',
      unreadable:
        'Der Ordner existiert, aber Polyfin kann seinen Inhalt nicht auflisten: Gib dem Konto, unter dem Polyfin läuft (unter Docker Benutzer 65532), das Recht, ihn zu lesen und seine Ordner zu öffnen.',
      not_folder: 'Dieser Pfad ist eine Datei, kein Ordner.',
    } as Record<string, string>,
    shareErrors: {
      missing: 'Die Freigabe oder der Ordner darin existiert nicht: Prüfe die Adresse.',
      unreadable:
        'Der Benutzer der Freigabe darf diesen Ordner nicht auflisten: Prüfe seine Berechtigungen auf dem Server.',
      not_folder: 'Diese Adresse ist eine Datei, kein Ordner.',
      unreachable:
        'Polyfin kann die Freigabe nicht erreichen: Prüfe, ob der Server eingeschaltet ist, sowie Server und Port in der Adresse.',
      refused:
        'Die Freigabe lehnt den Benutzer oder das Passwort ab: Prüfe beides und gib es erneut ein.',
      host_key_changed:
        'Der SFTP-Server bietet einen anderen Hostschlüssel an als den, der bei der ersten Verbindung gespeichert wurde, und Polyfin lehnt ihn ab. Wenn die Änderung erwartet ist, akzeptiere den neuen Schlüssel unter „Ordner bearbeiten“.',
    } as Record<string, string>,
    keptNote:
      'Die bisher gefundenen Dateien bleiben in der Bibliothek, bis der Ordner wieder gelesen werden kann.',
    unmatchedTitle: 'Nicht zugeordnete Dateien',
    unmatchedHelp:
      'Für diese Dateien wurde kein Titel gefunden. Benenne sie mit Titel und Jahr um oder füge eine NFO-Datei mit der Kennung des Titels hinzu und scanne dann; oder verknüpfe sie mit der IMDb-Kennung aus der IMDb-Adresse des Titels (tt gefolgt von Ziffern). Eine Verknüpfung bleibt bei erneuten Scans erhalten; in einem Serienordner gilt sie für die ganze Serie, bei einer Blu-ray-Disc oder DVD für die ganze Disc oder für einen Film einer Disc, die mehrere enthält. Die Titel einer Serien-Disc erhalten ihre Folge so, wie ein Dateiname sie angibt, etwa S01E05 oder S01E05-E06 für zwei; die für eine Disc vorgeschlagenen Folgen schließen an die der vorherigen Discs der Staffel an.',
    noUnmatched: 'Jede Datei ist einem Titel zugeordnet.',
    leftOutTitle: 'Ausgelassene Titel',
    leftOutHelp:
      'Weder die Tags noch die Namen dieser Dateien ergeben einen Titel oder ein Album. Versieh sie mit Tags oder verschiebe sie in einen Ordner Künstler/Album und scanne dann.',
    noLeftOut: 'Jeder Titel gehört zu einem Album.',
    shownOf: (shown: string, total: string) => `${shown} von ${total} angezeigt.`,
    reasons: {
      unreadable_name: 'Der Name ergibt keinen Titel oder keine Folgennummer',
      disc_title: 'Ein weiterer Film der Disc: Verknüpfe ihn mit seinem Titel',
      disc_episode: 'Ein Titel einer Serien-Disc: Gib seine Folge an',
      lone_extra:
        'Ein Extra ohne zugehörigen Titel daneben: Verknüpfe es, um es zu einem Titel zu machen',
      not_found: 'Die Metadaten-Add-ons kennen keinen Titel mit diesem Namen',
      no_search: 'Kein Add-on des Servers beschreibt und sucht Titel dieser Art',
      ambiguous: 'Mehrere Titel haben diesen Namen: Ergänze das Jahr oder verknüpfe ihn',
      other_year: 'Titel mit diesem Namen stammen aus anderen Jahren',
      search_failed: 'Die Metadaten-Add-ons konnten nicht durchsucht werden',
      no_title: 'Tags und Name ergeben keinen Titel',
      no_album: 'Tags und Ordner ergeben kein Album',
    } as Record<string, string>,
    searchFailedBy: (addon: string, failure: string) =>
      failure === ''
        ? `Die Suche von ${addon} ist fehlgeschlagen`
        : `Die Suche ist fehlgeschlagen: ${addon} ${failure}`,
    searchFailures: {
      unreachable: 'war nicht erreichbar',
      timeout: 'hat nicht rechtzeitig geantwortet',
      private_network: 'befindet sich in einem lokalen Netzwerk',
      rate_limited: 'hat um Verlangsamung gebeten (zu viele Anfragen)',
      server_error: 'hat mit einem Serverfehler geantwortet',
      http_error: 'hat die Anfrage abgelehnt',
      invalid_response: 'hat eine Antwort gesendet, die Polyfin nicht lesen kann',
    } as Record<string, string>,
    noSearchTitle: (count: number, figure: string) =>
      count === 1
        ? 'Kein Add-on kann den Titel von 1 Datei suchen'
        : `Kein Add-on kann die Titel von ${figure} Dateien suchen`,
    noSearchHelp:
      'Die Zuordnung braucht ein Add-on des Servers unter Inhalte › Quellen, das diese Titel beschreibt und einen Suchkatalog hat. Die eigenen Add-ons der Benutzer unter „Meine Quellen“ zählen nicht. Scanne den Ordner, sobald eines hinzugefügt ist.',
    searchFailedTitle: (count: number, figure: string) =>
      count === 1
        ? 'Der Titel von 1 Datei konnte nicht gesucht werden'
        : `Die Titel von ${figure} Dateien konnten nicht gesucht werden`,
    searchRetry: (at: string) =>
      `Polyfin sucht von selbst erneut, als Nächstes am ${at}, dann seltener, solange die Suche fehlschlägt, und bei jedem Scan.`,
    searchHelp:
      'Die Zuordnung braucht ein Server-Add-on, das diese Titel beschreibt und einen Suchkatalog hat. Schlagen seine Suchen weiter fehl, prüfe es unter System › Systemzustand oder ordne die Dateien unten von Hand zu.',
    read: (title: string) => `Gelesen als „${title}“`,
    showFolder: (unit: string) => `Serienordner: ${unit}`,
    discTitle: (disc: string, title: number) => `${disc} · Titel ${title}`,
    disc: {
      bluray: (unit: string) => `Blu-ray-Disc: ${unit}`,
      dvd: (unit: string) => `DVD: ${unit}`,
      bluray_image: (unit: string) => `Blu-ray-Image: ${unit}`,
      dvd_image: (unit: string) => `DVD-Image: ${unit}`,
    },
    minutes: (minutes: number) => `${minutes} min`,
    episodeLabel: (title: string) => `Folge von ${title}`,
    episodePlaceholder: 'S01E01',
    giveEpisode: 'Speichern',
    episodesGiven: (count: number) =>
      count === 1 ? 'Der Titel ist nummeriert.' : `${count} Titel sind nummeriert.`,
    suggestion: (disc: string, first: string, last: string) =>
      first === last
        ? `Vorschlag für ${disc}: ${first}`
        : `Vorschlag für ${disc}: ${first} bis ${last}`,
    confirmSuggestion: 'Diese Folgen verwenden',
    imdbId: 'IMDb-Kennung',
    imdbPlaceholder: 'tt0000000',
    link: 'Verknüpfen',
    linkLabel: (path: string) => `IMDb-Kennung für ${path}`,
    linked: (unit: string, id: string) => `${unit} ist mit ${id} verknüpft.`,
    findTitle: 'Titel suchen',
    searchLabel: (path: string) => `Titelsuche für ${path}`,
    searchQuery: 'Titel',
    searchYear: 'Jahr',
    search: 'Suchen',
    searching: 'Metadaten-Add-ons werden durchsucht…',
    noTitles:
      'Die Metadaten-Add-ons haben keinen Titel mit diesem Namen und einer IMDb-Kennung gefunden. Ändere die Suche oder gib die Kennung ein.',
    titlesFound: (query: string) => `Gefundene Titel für „${query}“`,
    titleKinds: { movie: 'Film', show: 'Serie' },
    pickTitle: (name: string, details: string) => `Mit ${name} verknüpfen, ${details}`,
    linksTitle: 'Manuell verknüpft',
    unlink: 'Verknüpfung entfernen',
    unlinkLabel: (unit: string) => `Verknüpfung von ${unit} entfernen`,
    unlinked: (unit: string) =>
      `Die Verknüpfung von ${unit} wurde entfernt: Die Zuordnung erfolgt wieder anhand des Namens oder der NFO-Datei.`,
    removeEpisodeLabel: (title: string) => `Folge von ${title} entfernen`,
    episodeRemoved: (title: string) =>
      `Die Folge von ${title} wurde entfernt: Sie wird über den Namen der Disc nummeriert oder erneut vorgeschlagen.`,
    looserTitle: 'Über eine lockerere Regel zugeordnet',
    looserHelp:
      'Kein Titel hat genau den Namen und das Jahr dieser Dateien. Polyfin hat sie daher dem einzigen Titel zugeordnet, den eine lockerere Regel übrig lässt: einem aus dem Jahr davor oder danach, oder einem mit gleichem Namen, wenn Zahlen gleich gelesen werden, etwa 2 für II oder Part 1 für Part One. Prüfe sie. Hebe eine falsche Zuordnung auf: Die Datei wird dann nur noch nach genauem Namen und Jahr zugeordnet und bleibt nicht zugeordnet, bis du sie mit ihrem Titel verknüpfst.',
    looserRules: {
      year: 'Ein Titel mit einem Jahr Abweichung',
      numerals: 'Zahlen gleich gelesen',
      year_numerals: 'Ein Titel mit einem Jahr Abweichung, Zahlen gleich gelesen',
    } as Record<string, string>,
    matchedTo: (title: string) => `Zugeordnet zu „${title}“`,
    fileCount: (count: number, figure: string) => (count === 1 ? '1 Datei' : `${figure} Dateien`),
    looserUnlinkLabel: (unit: string) => `Zuordnung von ${unit} aufheben`,
    looserUnlinked: (unit: string) =>
      `Die Zuordnung von ${unit} wurde aufgehoben: Sie erfolgt nur noch nach genauem Namen und Jahr.`,
    openPage: 'Seite des Ordners öffnen',
  },
  mySources: {
    lede: 'Füge deine eigenen Stremio-, Eclipse- und IPTV-Quellen hinzu und wähle aus, welche ihrer Kataloge als Bibliotheken in deinen Jellyfin-Apps erscheinen.',
    preferenceTitle: 'Die Quellen des Servers',
    useShared: 'Quellen des Servers verwenden',
    useSharedHelp:
      'Ein: Du erhältst die Quellen und Bibliotheken des Servers, gefolgt von deinen eigenen. Aus: Du erhältst nur deine eigenen Quellen und Bibliotheken.',
    preferenceSaved: 'Einstellung gespeichert.',
    parentalControl:
      'Für dein Konto gilt die Kindersicherung: Deine Jellyfin-Apps zeigen nur die Quellen und Bibliotheken des Servers, da diese die Altersfreigaben liefern, auf die sie sich stützt. Deine eigenen Quellen bleiben erhalten, werden aber nicht verwendet.',
    personalAddonsOff:
      'Ein Administrator hat deine eigenen Quellen deaktiviert: Deine Jellyfin-Apps zeigen nur die Quellen und Bibliotheken des Servers. Deine eigenen Quellen bleiben erhalten, werden aber nicht verwendet.',
    listTitle: 'Deine Quellen',
  },
  music: {
    content: { music: 'Musik', audiobook: 'Hörbücher', podcast: 'Podcasts' } as Record<
      string,
      string
    >,
    library: {
      music: 'Musikbibliothek',
      audiobook: 'Buchbibliothek',
      podcast: 'Musikbibliothek',
    } as Record<string, string>,
    settings: 'Einstellungen',
    settingsTitle: 'Add-on-Einstellungen',
    settingsHelp:
      'Vom Add-on festgelegt und mit jeder Anfrage gesendet, die Polyfin an es stellt. Sie können deine Kontodaten enthalten: Nur du (oder bei Server-Add-ons die Administratoren) kannst sie sehen.',
    noSettings: 'Dieses Add-on hat keine Einstellungen.',
    defaultValue: (value: string) => `Standard: ${value}`,
    defaultEmpty: 'Standard: leer',
    on: 'Ein',
    off: 'Aus',
    range: (min: string, max: string) => `Von ${min} bis ${max}.`,
    atLeast: (min: string) => `Mindestens ${min}.`,
    atMost: (max: string) => `Höchstens ${max}.`,
    stepOf: (step: string) => `In Schritten von ${step}.`,
    maxLength: (count: number) => `Höchstens ${count} ${count === 1 ? 'Zeichen' : 'Zeichen'}.`,
    tooLong: (count: number) => `Verwende höchstens ${count} Zeichen.`,
    belowMin: (min: string) => `Gib ${min} oder mehr ein.`,
    aboveMax: (max: string) => `Gib ${max} oder weniger ein.`,
    offStep: (step: string) => `Verwende ein Vielfaches von ${step}.`,
    resetDefaults: 'Standardwerte verwenden',
    saved: 'Einstellungen gespeichert. Das Add-on erhält sie ab seiner nächsten Anfrage.',
  },
}

export default sources
