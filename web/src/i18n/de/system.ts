import type en from '../en/system'

const system: typeof en = {
  apiKeys: {
    title: 'API-Schlüssel',
    description:
      'Mit Schlüsseln können Tools und Apps, etwa Request-Manager oder Skripte, die Jellyfin-API dieses Servers mit Administratorrechten nutzen. Wer einen Schlüssel hat, hat diese Rechte: Halte Schlüssel geheim und widerrufe die, die du nicht mehr verwendest.',
    listTitle: 'Schlüssel',
    empty: 'Noch kein API-Schlüssel.',
    created: 'Erstellt',
    lastUsed: 'Zuletzt verwendet',
    createTitle: 'Schlüssel erstellen',
    app: 'App-Name',
    appHint:
      'Wer diesen Schlüssel verwendet, damit du ihn später wiedererkennst. 1 bis 64 Zeichen.',
    create: 'Schlüssel erstellen',
    creating: 'Wird erstellt…',
    newKeyTitle: (app: string) => `Schlüssel für ${app}`,
    newKeyNotice: 'Kopiere diesen Schlüssel jetzt: Er wird nicht noch einmal angezeigt.',
    copy: 'Kopieren',
    copied: 'Kopiert.',
    copyFailed: 'Der Schlüssel konnte nicht kopiert werden. Markiere ihn und kopiere ihn manuell.',
    done: 'Ich habe ihn kopiert',
    revoke: 'Widerrufen',
    revoking: 'Wird widerrufen…',
    revokeConfirm: (app: string) =>
      `Schlüssel für ${app} widerrufen? Tools und Apps, die ihn verwenden, funktionieren dann nicht mehr. Dies kann nicht rückgängig gemacht werden.`,
    revoked: (app: string) => `Der Schlüssel für ${app} wurde widerrufen.`,
    emptyHint:
      'Erstelle unten für jedes Tool und jede App einen eigenen Schlüssel, damit du jeden einzeln widerrufen kannst.',
    revokeTitle: 'Diesen Schlüssel widerrufen?',
    revokeLabel: (app: string) => `Schlüssel für ${app} widerrufen`,
  },
  system: {
    schedule: {
      title: 'Zeitplan',
      description:
        'Die Aufgaben des Servers, anstehende Aufnahmen und die nächsten Live-TV-Aktualisierungen.',
      tasksTitle: 'Aufgaben',
      task: 'Aufgabe',
      lastRun: 'Letzte Ausführung',
      nextRun: 'Nächste Ausführung',
      notRunYet: 'Seit dem Serverstart nicht ausgeführt',
      byHandOnly: 'Nur manuelle Ausführung',
      every: (span: string) => `Intervall: ${span}`,
      daily: (time: string) => `Täglich um ${time}, Serverzeit`,
      took: (span: string) => `dauerte ${span}`,
      results: {
        Completed: 'Abgeschlossen',
        Failed: 'Fehlgeschlagen',
        Cancelled: 'Gestoppt',
      } as Record<string, string>,
      running: 'Läuft',
      cancelling: 'Wird gestoppt',
      runNow: 'Jetzt ausführen',
      runLabel: (name: string) => `${name} jetzt ausführen`,
      stop: 'Stoppen',
      stopLabel: (name: string) => `${name} stoppen`,
      started: (name: string) => `${name} gestartet.`,
      autoRefresh: 'Wird alle 5 Sekunden aktualisiert.',
      recordingsTitle: 'Anstehende Aufnahmen',
      recordingsOff: 'Aufnahmen sind deaktiviert. Aktiviere sie unter Einstellungen › Aufnahmen.',
      noTimers: 'Keine Aufnahme geplant.',
      recordingNow: 'Wird aufgenommen',
      failed: 'Fehlgeschlagen',
      series: 'Serie',
      scheduledBy: (name: string) => `Geplant von ${name}`,
      unknownChannel: 'Kanal nicht gefunden',
      tunersShort: 'Nicht genügend Tuner',
      tunersShortHelp:
        'Auf den Kanälen dieses Tuners laufen gleichzeitig mehr Aufnahmen, als er Tuner hat: Eine davon kann fehlschlagen.',
      refreshesTitle: 'Live-TV-Aktualisierungen',
      refreshesHelp: (hours: number) =>
        `IPTV-Kanallisten und Fernsehprogramme, die des Servers und die eigenen der Benutzer, werden alle ${hours} h erneut heruntergeladen (Einstellungen › Live-TV), durch die Aufgabe „Live-TV-Fernsehprogramme aktualisieren“.`,
      noRefreshes: 'Keine IPTV-Quelle und kein Fernsehprogramm.',
      channelList: 'Kanalliste',
      iptvLists: 'Kanal-, Film- und Serienlisten',
      guide: 'Fernsehprogramm',
      due: 'Bei der nächsten Prüfung',
      lastDownload: 'Letzter Download',
      nextDownload: 'Nächster Download',
      notYet: 'Noch nicht',
      noTasks: 'Keine Aufgabe.',
      noTasksHint: 'Der Server listet seine Aufgaben hier auf, sobald er gestartet ist.',
      recordingsHint:
        'Aufnahmen, die in einer App oder im Fernsehprogramm geplant werden, erscheinen hier.',
      refreshesHint:
        'Füge unter Inhalte › Quellen eine IPTV-Quelle oder einem TV-Katalog ein Fernsehprogramm hinzu, dann erscheinen deren Downloads hier.',
      backupsTitle: 'Datenbank-Backups',
      backUpNow: 'Jetzt sichern',
    },
    health: {
      title: 'Systemzustand',
      noIptv: 'Keine IPTV-Quelle.',
      owner: { server: 'Server', user: (name: string) => `Add-ons von ${name}` },
      description:
        'Zustand des Servers und seiner Quellen, basierend auf dem, was Polyfin bereits aufzeichnet: Diese Seite sendet keine Anfragen an sie.',
      problemsTitle: 'Handlungsbedarf',
      allGood: 'Kein Problem gefunden.',
      update: (version: string) => `Polyfin ${version} ist verfügbar`,
      updateRunning: (version: string) => `Auf diesem Server läuft Version ${version}.`,
      releaseNotes: 'Versionshinweise',
      checkedAt: (time: string) => `Geprüft um ${time}. Wird alle 10 Sekunden aktualisiert.`,
      ok: 'OK',
      warning: 'Warnung',
      error: 'Fehler',
      problems: {
        database: 'Die Datenbank antwortet nicht.',
        disk: (folder: string, free: string) => `Wenig Speicherplatz für ${folder}: ${free} frei.`,
        addon: (name: string, failure: string) => `${name}: ${failure}`,
        iptv: (name: string) => `${name}: Der letzte Download der Kanalliste ist fehlgeschlagen.`,
        tuner: (name: string) => `${name}: Der Tuner konnte einen Kanal nicht abspielen.`,
        tunerUnreachable: (name: string) => `${name}: Der Tuner antwortet nicht.`,
        folder: (name: string) =>
          `${name}: Polyfin kann diesen lokalen Ordner nicht lesen. Prüfe, ob er eingehängt und für den Benutzer 65532 lesbar ist.`,
        share: (name: string) =>
          `${name}: Polyfin kann diese Netzwerkfreigabe nicht lesen. Prüfe ihre Adresse und ob ihr Benutzer sie lesen darf.`,
        shareUnreachable: (name: string) =>
          `${name}: Polyfin kann diese Netzwerkfreigabe nicht erreichen.`,
        shareRefused: (name: string) =>
          `${name}: Die Netzwerkfreigabe lehnt den Benutzer oder das Passwort von Polyfin ab.`,
        shareHostKey: (name: string) =>
          `${name}: Der SFTP-Server bietet einen anderen Hostschlüssel an als den gespeicherten, und Polyfin lehnt ihn ab, bis er akzeptiert wird.`,
        folderWatchLimit: (name: string) =>
          `${name}: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für überwachte Ordner ist erreicht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe fs.inotify.max_user_watches auf dem Host.`,
        folderInstanceLimit: (name: string) =>
          `${name}: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für Überwachungsinstanzen oder offene Dateien ist erreicht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe fs.inotify.max_user_instances auf dem Host oder das Limit für offene Dateien des Containers.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen: Das Systemlimit für offene Dateien ist erreicht, da macOS jeden Ordner und jede Datei über eine offene Datei überwacht. Er wird weiterhin nach seinem Zeitplan gescannt. Erhöhe kern.maxfilesperproc und kern.maxfiles mit sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}: Polyfin kann diesen lokalen Ordner nicht auf Änderungen überwachen. Er wird weiterhin nach seinem Zeitplan gescannt.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen: Sie ist nicht erreichbar. Sie wird nach ihrem Zeitplan gescannt und erneut, sobald sie antwortet.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen: Sie lehnt den Benutzer oder das Passwort von Polyfin ab. Prüfe beides und gib es erneut ein.`,
        shareUnwatched: (name: string) =>
          `${name}: Polyfin kann diese Netzwerkfreigabe nicht auf Änderungen überwachen. Sie wird weiterhin nach ihrem Zeitplan gescannt.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}: Die Titel seiner Dateien konnten nicht gesucht werden${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}: ${failure})`}. Sie bleiben nicht zugeordnet, bis eine Suche gelingt, was Polyfin von selbst erneut versucht, erst nach ein paar Minuten, dann seltener.`,
        guide: (name: string) =>
          `${name}: Der letzte Abruf des Fernsehprogramms ist fehlgeschlagen.`,
        paused: (host: string) =>
          `Vorschaubilder für ${host} pausiert: Der Host hat um Verlangsamung gebeten.`,
        unknownProxy: (address: string) =>
          `${address} leitet Anfragen weiter (X-Forwarded-For), ist aber kein bekannter Proxy, daher zählen alle seine Clients als ${address}. Wenn es dein Reverse-Proxy ist, füge ihn unter Einstellungen › Sicherheit zu „Bekannte Proxys“ hinzu.`,
        discoveryPort: (port: number) =>
          `Polyfin kann nicht auf UDP-Port ${port} lauschen, daher finden Jellyfin-Apps im lokalen Netzwerk den Server nicht von selbst. Wahrscheinlich nutzt ein anderes Programm den Port, etwa ein Jellyfin-Server auf demselben Host. Beim Speichern der Einstellungen wird es erneut versucht.`,
        torrentRoute: (why: string) =>
          `Torrent-Streams werden nicht abgespielt: ${why}. Es wird nichts auf anderem Weg gesendet; prüfe die Route unter Einstellungen › Torrents.`,
        torrentWhy: {
          interface_missing: (name: string) => `die Netzwerkschnittstelle ${name} fehlt`,
          interface_down: (name: string) => `die Netzwerkschnittstelle ${name} ist deaktiviert`,
          interface_no_address: (name: string) =>
            `die Netzwerkschnittstelle ${name} hat keine Adresse`,
          interface_unsupported: () => 'eine Netzwerkschnittstelle funktioniert nur unter Linux',
          proxy_unreachable: (name: string) => `der SOCKS5-Proxy ${name} antwortet nicht`,
          proxy_refused: (name: string) =>
            `der SOCKS5-Proxy ${name} hat Benutzer und Passwort abgelehnt`,
          wireguard_endpoint: (name: string) =>
            `der WireGuard-Server ${name} kann nicht aufgelöst werden`,
          wireguard_down: (name: string) => `der WireGuard-Tunnel zu ${name} kann nicht starten`,
          wireguard_handshake: (name: string) =>
            `der WireGuard-Server ${name} antwortet nicht auf den Handshake: Er ist nicht erreichbar, oder ein Schlüssel der Konfiguration ist falsch`,
          no_route: () => 'es ist keine Route festgelegt',
        },
        torrentUdp: (name: string) =>
          `Der SOCKS5-Proxy ${name} bietet kein UDP ASSOCIATE an: Torrents werden über TCP abgespielt, ohne UDP-Tracker und DHT.`,
        torrentStart: (why: string) =>
          `Ein Torrent ist nicht rechtzeitig gestartet: ${why}. Prüfe, ob die Route das Internet erreicht; das Protokoll nennt auf der Debug-Stufe die Antwort jedes Trackers.`,
        torrentStartWhy: {
          no_udp:
            'es wurde kein Peer gefunden, und die Route überträgt kein UDP, das DHT und UDP-Tracker brauchen',
          refused:
            'seine Tracker haben die Ankündigung abgelehnt oder nicht geantwortet, und das DHT hat keinen Peer gefunden',
          no_peers: 'seine Tracker und das DHT haben keinen Peer gefunden',
          silent: 'es wurden Peers gefunden, aber keiner hat ihn gesendet',
        },
        dlnaPort: (port: number) =>
          `Polyfin kann nicht auf UDP-Port ${port} lauschen, daher finden DLNA-Geräte im lokalen Netzwerk den Server nicht von selbst. Wahrscheinlich nutzt ein anderes Programm den Port, etwa ein anderer DLNA-Server auf demselben Host. Beim Speichern der Einstellungen wird es erneut versucht.`,
        discoveryAddress: (address: string) =>
          `Jellyfin-Apps, die im lokalen Netzwerk nach Servern suchen, erhalten ${address}, eine Adresse des Polyfin-Containers, die sie nicht erreichen können. Lege „Adresse im lokalen Netzwerk“ unter Einstellungen › Allgemein fest.`,
        proxyNameUnresolved: (name: string) =>
          `Der bekannte Proxy ${name} wird zu keiner Adresse aufgelöst, daher wird keiner Anfrage vertraut, die von ihm kommt. Prüfe seinen Namen unter Einstellungen › Sicherheit und ob Polyfin ein Netzwerk mit ihm teilt.`,
        task: (name: string) => `Die Aufgabe „${name}“ ist fehlgeschlagen.`,
        httpsPort: (port: number) =>
          `Polyfin kann nicht auf TCP-Port ${port} lauschen, daher wird HTTPS nicht bereitgestellt. HTTP funktioniert weiterhin. Wahrscheinlich nutzt ein anderes Programm den Port, oder er liegt unter 1024. Wähle unter Einstellungen › Sicherheit einen anderen HTTPS-Port.`,
        httpsFileMissing: (file: string) =>
          `Die HTTPS-Zertifikatsdatei ${file} existiert nicht. Das zuletzt gelesene Zertifikat bleibt in Verwendung.`,
        httpsFilesMismatch:
          'Der private HTTPS-Schlüssel gehört nicht zum Zertifikat. Das zuletzt gelesene Zertifikat bleibt in Verwendung.',
        httpsFileUnreadable: (file: string) =>
          `Polyfin kann die HTTPS-Zertifikatsdatei ${file} nicht lesen, oder sie enthält kein lesbares PEM-Zertifikat bzw. keinen lesbaren PEM-Schlüssel. Das zuletzt gelesene Zertifikat bleibt in Verwendung.`,
        httpsRenewal: (error: string) =>
          `Let’s Encrypt hat das HTTPS-Zertifikat nicht ausgestellt: ${error}`,
        httpsExpires: (date: string) => `Das HTTPS-Zertifikat läuft am ${date} ab.`,
        httpsExpired: (date: string) => `Das HTTPS-Zertifikat ist am ${date} abgelaufen.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `Bilduntertitel in ${name} werden nicht in Text umgewandelt: Tesseract hat keine Daten für diese Sprache. Füge die Datei ${file} dem Ordner hinzu, den POLYFIN_TESSDATA_DIR angibt.`,
        metadataLanguages: (name: string) =>
          `${name}: Jedes Metadaten-Add-on des Servers hat eine Sprache, daher beschreiben sie Titel für jeden Benutzer in ihrer Sprache, unabhängig von seiner Metadatensprache. Belasse „Sprache der Beschreibungen“ bei diesem Add-on oder bei einem weiteren Metadaten-Add-on auf „Keine“.`,
        backupFailed: 'Das letzte Datenbank-Backup ist fehlgeschlagen.',
        backupStale: 'Das letzte Datenbank-Backup ist älter als zwei Tage.',
        backupDestinationFailed: (name: string) =>
          `${name}: Der letzte Upload der Datenbank-Backups ist fehlgeschlagen.`,
        backupDestinationStale: (name: string) =>
          `${name}: Das neueste Datenbank-Backup dort ist älter als zwei Tage.`,
        conversionsFull:
          'Videokonvertierungen sind am Limit: Neue warten oder werden anders abgespielt.',
        unreadableSecrets: (names: string) =>
          `Gespeicherte Schlüssel können mit POLYFIN_SECRET_KEY nicht entschlüsselt werden und gelten als nicht festgelegt: ${names}. Lege den Schlüssel, mit dem sie verschlüsselt wurden, erneut fest oder gib sie erneut ein.`,
        plaintextSecrets:
          'Schlüssel und Tokens werden unverschlüsselt gespeichert. Lege POLYFIN_SECRET_KEY fest, um sie zu verschlüsseln.',
        appStopped: (app: string, user: string) =>
          `${app} hat die Anmeldung abgelehnt, die Polyfin für ${user} gespeichert hat: Es wird nichts importiert, bis sich ${user} unter „Mein Konto“ erneut anmeldet, mit aktiviertem „Angemeldet bleiben“.`,
        trackingStopped: (service: string, user: string) =>
          `${service} hat das Token oder den Schlüssel abgelehnt, den Polyfin für ${user} gespeichert hat: Es wird nichts dorthin gesendet oder von dort importiert, bis ${user} unter Mein Konto › Tracking erneut auf „Verbinden“ klickt.`,
      },
      addonsTitle: 'Add-ons',
      addonsHelp:
        'Basierend auf den Anfragen von Apps seit dem Serverstart. „Prüfen“ fragt ein Add-on einmalig nach seinem Manifest, höchstens einmal pro Minute.',
      noAddons: 'Der Server hat kein Add-on.',
      addonStatus: {
        ok: 'Antwortet',
        failing: 'Fehlerhaft',
        unknown: 'Keine Anfrage seit dem Start',
        off: 'Ausgeschaltet',
      },
      failures: {
        unreachable: 'war nicht erreichbar',
        timeout: 'hat nicht rechtzeitig geantwortet',
        private_network: 'befindet sich in einem lokalen Netzwerk',
        rate_limited: 'hat um Verlangsamung gebeten (zu viele Anfragen)',
        server_error: 'hat mit einem Serverfehler geantwortet',
        http_error: 'hat die Anfrage abgelehnt',
        invalid_response: 'hat eine zu große Antwort gesendet',
      } as Record<string, string>,
      lastSuccess: 'Letzte Antwort',
      lastFailure: 'Letzter Fehler',
      responseTime: 'Antwortzeit',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'Anfrage' : 'Anfragen'}, ${failures} fehlgeschlagen`,
      check: 'Prüfen',
      checkLabel: (name: string) => `${name} jetzt prüfen`,
      checking: 'Wird geprüft…',
      iptvTitle: 'IPTV-Quellen',
      guidesTitle: 'Fernsehprogramme',
      sharesTitle: 'Netzwerkfreigaben',
      shareKinds: {
        smb: 'SMB',
        webdav: 'WebDAV',
        s3: 'S3',
        sftp: 'SFTP',
        rclone: 'rclone',
      } as Record<string, string>,
      shareStatus: {
        notified: 'Benachrichtigt',
        polled: 'Abgefragt',
        unwatched: 'Nicht überwacht',
        scans: 'Nur bei Scans',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'S3-Buckets melden' : share === 'sftp' ? 'SFTP-Server melden' : 'rclone-Remotes melden'} keine Änderungen: Die Unterordner des Ordners werden ${minutes === 1 ? 'jede Minute' : `alle ${minutes} Minuten`} erneut aufgelistet, wie in seinen Einstellungen festgelegt.`,
      shareScansOnly:
        'Laut seinen Einstellungen wird nur bei Scans nach Änderungen gesucht: Der Ordner wird nach seinem Zeitplan gelesen und wenn er manuell gescannt wird.',
      shareNotified:
        'Der Server meldet Polyfin jede Änderung: Die Freigabe wird wenige Sekunden danach erneut gescannt. Ihre Ordner werden außerdem alle 10 Minuten aufgelistet, falls eine Änderung nicht gemeldet wird.',
      sharePartial:
        'Der Server meldet Polyfin die über das Netzwerk vorgenommenen Änderungen, hat aber eine in einem Ordner der Freigabe verpasst, etwa eine direkt auf der Festplatte des Servers: Die Ordner der Freigabe werden außerdem alle 2 Minuten erneut aufgelistet.',
      sharePolled: {
        webdav:
          'WebDAV-Server melden keine Änderungen: Die Ordner der Freigabe werden alle 2 Minuten erneut aufgelistet.',
        unsupported:
          'Dieser SMB-Server meldet keine Änderungen: Die Ordner der Freigabe werden alle 2 Minuten erneut aufgelistet.',
        silent:
          'Dieser SMB-Server hat eine Änderung nicht gemeldet, die beim Auflisten gefunden wurde: Die Ordner der Freigabe werden alle 2 Minuten erneut aufgelistet.',
        failed:
          'Polyfin hat die Benachrichtigungen des Servers verloren und fordert sie in Kürze erneut an. Bis dahin werden die Ordner der Freigabe alle 2 Minuten erneut aufgelistet.',
        starting:
          'Polyfin bittet den Server, Änderungen zu melden. Bis dahin werden die Ordner der Freigabe alle 2 Minuten erneut aufgelistet.',
      } as Record<string, string>,
      shareUnwatched: {
        unreachable:
          'Die Freigabe ist nicht erreichbar. Sie wird nach ihrem Zeitplan gescannt und erneut, sobald sie antwortet.',
        refused:
          'Die Freigabe lehnt den Benutzer oder das Passwort von Polyfin ab. Prüfe beides und gib es erneut ein.',
        host_key_changed:
          'Der SFTP-Server bietet einen anderen Hostschlüssel an als den gespeicherten. Akzeptiere ihn unter „Ordner bearbeiten“, wenn die Änderung erwartet ist.',
        failed:
          'Polyfin kann diese Freigabe nicht überwachen. Sie wird weiterhin nach ihrem Zeitplan gescannt.',
      } as Record<string, string>,
      sharesOff:
        'Die Überwachung auf Änderungen ist in den Einstellungen ausgeschaltet: Freigaben werden nur nach ihrem Zeitplan gescannt.',
      transcoderTitle: 'Transkodierung',
      gpu: 'GPU',
      noGpu: 'Keine: Videos werden auf der CPU konvertiert',
      toneMapping: 'HDR zu SDR auf der GPU',
      cpuEncoders: 'CPU-Encoder',
      conversions: 'Videokonvertierungen',
      noLimit: 'unbegrenzt',
      remuxes: 'Remuxe',
      maxHeight: 'Konvertiert bis',
      transcodingOff: 'Die Konvertierung ist in den Einstellungen ausgeschaltet.',
      databaseTitle: 'Datenbank',
      reachable: 'Antwortet',
      unreachable: 'Antwortet nicht',
      size: 'Größe',
      cacheTitle: 'Quellen-Cache',
      cacheHelp:
        'Teile der gerade wiedergegebenen Dateien, auf der Festplatte gespeichert bis zur unter Einstellungen › Wiedergabe festgelegten Größe. Ein voller Cache ist normal: Die ältesten Teile machen Platz.',
      cacheUse: (used: string, limit: string) => `${used} von ${limit}`,
      sourcesOpen: 'Geöffnete Dateien',
      torrentPieces: 'Davon Torrent-Teile',
      torrentsTitle: 'Torrents',
      torrentsHelp:
        'Die Torrents, die die Wiedergabe gerade liest, ausschließlich über ihre Route. Ein Torrent stoppt, wenn seine Wiedergabe endet: keine Peers mehr, kein Teilen mehr.',
      torrentRoute: {
        interface: 'Netzwerkschnittstelle',
        proxy: 'SOCKS5-Proxy',
        wireguard: 'WireGuard-Tunnel',
        '': 'Keine Route',
      },
      torrentAddresses: 'Tunneladressen',
      torrentHandshake: 'Letzter Handshake',
      torrentNoHandshake: 'Noch keiner',
      torrentsPlaying: 'Laufende Torrents',
      torrentPeers: 'Peers',
      torrentUTPPeers: 'Peers über uTP',
      torrentsShared: 'Geteilt seit dem Start',
      disksTitle: 'Speicherplatz',
      folders: { cache: 'Cache', recordings: 'Aufnahmen', backups: 'Backups' } as Record<
        string,
        string
      >,
      folderTitle: {
        cache: 'Cache-Ordner',
        recordings: 'Aufnahmeordner',
        backups: 'Backup-Ordner',
      } as Record<string, string>,
      free: (free: string) => `${free} frei`,
      notMeasured: 'Kann hier nicht gemessen werden.',
      thumbnailsTitle: 'Miniaturansichten',
      thumbnailsOff:
        'Miniaturansichten und Kapitelbilder sind deaktiviert (Einstellungen › Miniaturansichten).',
      waiting: (count: number, length: number) => `${count} von ${length} wartend`,
      working: 'Bilder werden gerade erstellt',
      idle: 'Inaktiv',
      pausedHosts: 'Pausierte Quellen',
      noPausedHosts: 'Keine Quelle ist pausiert.',
      until: (time: string) => `bis ${time}`,
      ratingsTitle: 'Bewertungen von MDBList',
      ratingsHelp:
        'Die Anfragen, die MDBList dem Bewertungsschlüssel des Servers pro Tag erlaubt, und die bis zum täglichen Zurücksetzen verbleibenden. Titel, deren Bewertungen nicht gespeichert sind, werden im Hintergrund stapelweise abgefragt.',
      requestsLeft: 'Verbleibende Anfragen heute',
      requestsLeftOf: (left: string, limit: string) => `${left} von ${limit}`,
      notToldYet: 'Bekannt nach der nächsten Anfrage an MDBList',
      resets: 'Tägliches Zurücksetzen',
      ratingsHeld: 'Erneute Anfrage',
      heldReasons: {
        limit: 'MDBList begrenzt die Anfragen',
        refused: 'MDBList hat den Schlüssel abgelehnt',
        failed: 'MDBList hat nicht geantwortet',
      } as Record<string, string>,
      notHeld: 'Jetzt',
      titlesWaiting: 'Wartende Titel',
      titlesKept: 'Titel mit gespeicherten Bewertungen',
      backupTitle: 'Backups',
      backupOff: 'Backups sind deaktiviert. Aktiviere sie unter Einstellungen › Backups.',
      backup: {
        result: 'Letzte Ausführung',
        none: 'Noch kein Backup',
        succeeded: 'Erfolgreich',
        failed: 'Fehlgeschlagen',
        made: 'Letztes Backup erstellt',
        file: 'Datei',
        size: 'Größe',
        next: 'Nächstes Backup',
        runHint:
          'Führe „Datenbank sichern“ auf der Seite „Zeitplan“ aus, um jetzt ein Backup zu erstellen.',
      },
      processTitle: 'Polyfin',
      version: 'Version',
      uptime: 'Läuft seit',
      memory: 'Arbeitsspeicher',
      heap: 'Belegt',
      goroutines: 'Goroutinen',
      secretsTitle: 'Gespeicherte Schlüssel',
      secretsHelp:
        'Die Schlüssel des Servers, die Tracking-Tokens und -Schlüssel der Benutzer und die Passwörter der Netzwerkfreigaben, in der Datenbank verschlüsselt mit POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Verschlüsselung',
      encrypted: 'Ein',
      notEncrypted: 'Aus: POLYFIN_SECRET_KEY ist nicht gesetzt',
      storedPlain: 'Unverschlüsselt gespeichert',
      unreadable: 'Nicht entschlüsselbar',
      noneUnreadable: 'Keine',
      proxiesTitle: 'Bekannte Proxys',
      proxiesHelp:
        'Die unter „Bekannte Proxys“ eingetragenen Hostnamen, aufgelöst bei jeder Änderung der Einstellungen, jede Minute und sobald eine unbekannte Adresse Anfragen weiterleitet. Jeder Adresse, zu der ein Name aufgelöst wird, wird als Proxy vertraut.',
      resolvesToNothing: 'Wird zu keiner Adresse aufgelöst',
      secretNames: {
        publicMetaDbKey: 'PublicMetaDB-Schlüssel',
        theIntroDbKey: 'TheIntroDB-Schlüssel',
        mdblistRatingsKey: 'MDBList-API-Schlüssel für Bewertungen',
        tmdbKey: 'TMDB-API-Schlüssel',
        traktClientSecret: 'Trakt-Client-Secret',
        lastFmSecret: 'Last.fm-Shared-Secret',
        aniListClientSecret: 'AniList-Client-Secret',
        myAnimeListClientSecret: 'MyAnimeList-Client-Secret',
        smtpPassword: 'SMTP-Passwort',
        torrentProxyPassword: 'Passwort des SOCKS5-Proxys für Torrents',
        torrentWireGuard: 'WireGuard-Konfiguration der Torrents',
        metricsToken: 'Metrik-Token',
      },
      connectionOf: (service: string, user: string) => `${service}-Verbindung von ${user}`,
      targetOf: (target: string, user: string) => `Benachrichtigungsziel „${target}“ von ${user}`,
      serverTargetOf: (target: string) => `Server-Benachrichtigungsziel „${target}“`,
      sharePasswordOf: (folder: string) => `Passwort oder Schlüssel der Freigabe „${folder}“`,
      destinationSecretOf: (name: string) => `Passwort oder Schlüssel des Backup-Ziels „${name}“`,
      certificateKeys: 'Schlüssel des Let’s-Encrypt-Zertifikats',
      twoStepOf: (user: string) => `Schlüssel der zweistufigen Anmeldung von ${user}`,
      noAddonsHint: 'Füge unter Inhalte › Quellen ein Stremio-Add-on hinzu.',
      noIptvHint: 'Füge unter Inhalte › Quellen ein IPTV-Konto hinzu.',
      noGuides: 'Kein Fernsehprogramm.',
      noGuidesHint: 'Füge unter Inhalte › Live-TV einem TV-Katalog ein Fernsehprogramm hinzu.',
      serverTitle: 'Server',
    },
    logs: {
      title: 'Protokolle',
      description:
        'Die neuesten Zeilen des Serverprotokolls, im Arbeitsspeicher gehalten. Von Adressen bleibt nur der Host erhalten, Passwörter, Tokens und Schlüssel werden ausgeblendet.',
      follow: 'Mitverfolgen',
      pause: 'Pausieren',
      level: 'Stufe',
      levels: {
        all: 'Alle',
        info: 'Info und höher',
        warn: 'Warnungen und Fehler',
        error: 'Nur Fehler',
      },
      search: 'Suchen',
      download: 'Herunterladen',
      clear: 'Ansicht leeren',
      empty: 'Noch keine Zeile.',
      noMatch: 'Keine Zeile passt.',
      shown: (shown: number, total: number) => `${shown} von ${total} Zeilen angezeigt.`,
      following: 'Live-Verfolgung: Neue Zeilen erscheinen alle 2 Sekunden.',
      paused:
        'Pausiert: Die zwischenzeitlich geschriebenen Zeilen erscheinen, sobald du wieder mitverfolgst.',
      detailedHint:
        'Debug-Zeilen erscheinen, sobald „Detailliertes Protokoll“ unter Einstellungen › Diagnose aktiviert ist.',
      region: 'Serverprotokoll',
      emptyHint: 'Hier erscheinen Zeilen, sobald der Server sie schreibt.',
      noMatchHint: 'Wähle eine andere Stufe oder ändere die Suche.',
      filters: 'Protokoll filtern',
    },
  },
}

export default system
