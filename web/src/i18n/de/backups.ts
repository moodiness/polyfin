import type en from '../en/backups'

const backups: typeof en = {
  backups: {
    destinations: {
      title: 'Ziele',
      loading: 'Ziele werden geladen…',
      empty: 'Noch kein Ziel.',
      emptyHint:
        'Füge S3-kompatiblen Speicher, einen SFTP-Server oder einen WebDAV-Server hinzu, um Backups außerhalb der Festplatten dieses Servers aufzubewahren.',
      add: 'Ziel hinzufügen',
      addTitle: 'Ziel hinzufügen',
      editTitle: (name: string) => `${name} bearbeiten`,
      kind: 'Art',
      kinds: { s3: 'S3-kompatibler Speicher', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi und anderer Speicher, der S3 unterstützt. Verwende einen eigenen Bucket oder ein Präfix, in das nur Polyfin schreibt.',
        sftp: 'Ein Server, bei dem sich Polyfin über SSH anmeldet, etwa ein NAS. Sein Host-Schlüssel wird bei der ersten Verbindung gespeichert und danach überprüft.',
        webdav: 'Ein Ordner auf einem WebDAV-Server, etwa Nextcloud, ownCloud oder ein NAS.',
      },
      endpoint: 'Endpunkt',
      endpointHelp:
        'Die Adresse des Speichers ohne den Bucket, etwa https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Bucket',
      prefix: 'Ordnerpräfix',
      prefixHelp: 'Der Ordner im Bucket, etwa polyfin. Leer für das Stammverzeichnis des Buckets.',
      region: 'Region',
      regionHelp:
        'Leer für us-east-1, was die meisten Speicher akzeptieren. Cloudflare R2 erwartet auto.',
      accessKey: 'Zugriffsschlüssel',
      secretKey: 'Geheimer Schlüssel',
      host: 'Host',
      hostHelp: 'Der Name oder die IP-Adresse des Servers, ohne sftp://.',
      port: 'Port',
      user: 'Benutzer',
      password: 'Passwort',
      sftpPasswordHelp: 'Leer lassen, wenn die Anmeldung mit einem privaten Schlüssel erfolgt.',
      privateKey: 'Privater Schlüssel',
      privateKeyHelp:
        'Ein privater OpenSSH- oder PEM-Schlüssel ohne Passphrase, dessen öffentlichen Schlüssel der Server akzeptiert. Leer lassen, um dich mit dem Passwort anzumelden.',
      privateKeySaved: 'Ein Schlüssel ist gespeichert: Füge einen anderen ein, um ihn zu ersetzen.',
      privateKeyRemove: 'Gespeicherten Schlüssel entfernen',
      sftpFolder: 'Ordner',
      sftpFolderHelp:
        'Wohin die Backups kommen, etwa /volume1/backups/polyfin. Leer für das Benutzerverzeichnis.',
      webdavAddress: 'Ordneradresse',
      webdavAddressHelp:
        'Die Adresse eines vorhandenen Ordners, etwa https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'Ein App-Passwort, sofern der Server welche anbietet.',
      kept: 'Aufzubewahrende Backups',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `Nach jedem Upload löscht Polyfin dort seine ältesten Backups, die über diese Anzahl hinausgehen. Andere Dateien werden nie angetastet. Von ${min} bis ${max}, standardmäßig ${byDefault}.`,
      hostKey: 'Host-Schlüssel',
      hostKeyNone: 'Wird bei der ersten Verbindung gespeichert.',
      hostKeyForget: 'Vergessen',
      hostKeyForgetHelp:
        'Vergiss den gespeicherten Schlüssel, wenn sich der Schlüssel des Servers absichtlich geändert hat: Der Schlüssel, den er bei der nächsten Verbindung anbietet, wird gespeichert.',
      hostKeyForgotten:
        'Der Schlüssel, den der Server bei der nächsten Verbindung anbietet, wird gespeichert.',
      create: 'Hinzufügen',
      creating: 'Wird hinzugefügt…',
      save: 'Speichern',
      saving: 'Wird gespeichert…',
      created: (name: string) => `${name} hinzugefügt.`,
      saved: (name: string) => `${name} gespeichert.`,
      test: 'Testen',
      testing: 'Wird getestet…',
      testHelp: 'Schreibt eine kleine Datei, liest sie zurück und löscht sie.',
      testOk: (name: string) => `${name} funktioniert.`,
      testFailed: (name: string) => `${name} funktioniert nicht.`,
      edit: 'Bearbeiten',
      delete: 'Löschen',
      deleteTitle: (name: string) => `${name} löschen?`,
      deleteBody:
        'Backups werden nicht mehr dorthin gesendet. Bereits vorhandene bleiben erhalten.',
      deleted: (name: string) => `${name} gelöscht.`,
      keeps: (count: number) =>
        count === 1 ? 'bewahrt 1 Backup auf' : `bewahrt ${count} Backups auf`,
      status: {
        waiting: 'Noch kein Upload',
        working: 'Funktioniert',
        failed: 'Fehlgeschlagen',
        stale: 'Veraltet',
        unreadable: 'Schlüssel unlesbar',
      },
      lastUpload: 'Letzter Upload',
      newest: 'Neuestes Backup dort',
      none: 'Noch keines',
      unreadable:
        'Das Passwort oder der Schlüssel kann mit POLYFIN_SECRET_KEY nicht entschlüsselt werden: Gib es erneut ein.',
      staleHelp: 'Das neueste Backup dort ist älter als zwei Tage.',
    },
    restore: {
      button: 'Wiederherstellen',
      title: 'Backup wiederherstellen',
      intro:
        'Die Datenbank wird auf den Stand zurückgesetzt, den sie beim Erstellen des Backups hatte. Wähle ein Backup aus dem Ordner oder von einem Ziel, oder sende eine Backup-Datei.',
      loading: 'Backups werden aufgelistet…',
      folder: 'Backup-Ordner',
      none: 'Hier gibt es kein Backup.',
      beforeRestore: 'vor einer Wiederherstellung',
      encrypted: 'verschlüsselt',
      choose: 'Dieses Backup wiederherstellen',
      download: 'Herunterladen',
      downloadDecrypted: 'Entschlüsselt herunterladen',
      downloadTitle: (file: string) => `${file} entschlüsselt herunterladen`,
      downloadBody:
        'Gib die Passphrase ein, mit der dieses Backup verschlüsselt wurde. Der Dump wird unverschlüsselt auf diesem Computer gespeichert: Bewahre ihn so sicher auf wie den Server.',
      passphrase: 'Backup-Passphrase',
      downloading: 'Passphrase wird geprüft…',
      upload: 'Backup-Datei senden',
      uploadHelp:
        'Eine .dump-Datei der Polyfin-Datenbank, erstellt von Polyfin oder mit pg_dump --format=custom, oder eine von Polyfin verschlüsselte .dump.enc-Datei.',
      unlockTitle: (file: string) => `${file} öffnen`,
      unlockBody:
        'Dieses Backup ist verschlüsselt. Gib die Passphrase ein, mit der es erstellt wurde: Polyfin entschlüsselt es, prüft jeden Teil davon und zeigt dann, was die Wiederherstellung bewirkt.',
      unlock: 'Öffnen',
      unlocking: 'Wird entschlüsselt und geprüft…',
      checking: 'Backup wird geprüft…',
      unavailable: {
        pg_restore_missing:
          'pg_restore wurde auf dem Server nicht gefunden: Installiere die Client-Tools von PostgreSQL 18, die im Docker-Image enthalten sind.',
      },
      confirmTitle: (file: string) => `${file} wiederherstellen?`,
      made: (date: string) => `Erstellt am ${date}.`,
      from: (name: string) => `Von ${name}.`,
      uploaded: 'Von diesem Computer gesendet.',
      steps: [
        'Polyfin sichert zuerst die aktuelle Datenbank im Backup-Ordner, als Backup vor der Wiederherstellung, das nie gelöscht wird.',
        'Apps erhalten während der Wiederherstellung einen Fehler, und laufende Wiedergaben, Konvertierungen und Aufnahmen werden beendet.',
        'Anschließend startet Polyfin mit der wiederhergestellten Datenbank neu: Alles ist wie zum Zeitpunkt des Backups, einschließlich Benutzern, Einstellungen und Wiedergabeverlauf.',
        'Apps und Administratoren, die sich seitdem angemeldet haben, müssen sich erneut anmelden (auch du), und zuvor ausgegebene Stream-Links funktionieren nicht mehr.',
      ],
      keyTitle: 'Einige gespeicherte Schlüssel können nicht gelesen werden',
      keyBody:
        'Dieses Backup enthält Schlüssel und Passwörter, die der aktuelle POLYFIN_SECRET_KEY nicht entschlüsseln kann. Setze den Schlüssel, der beim Erstellen des Backups gesetzt war, und stelle dann erneut wieder her; oder stelle trotzdem wieder her und gib diese danach erneut ein:',
      kinds: {
        settings: (n: number) =>
          n === 1 ? '1 Schlüssel der Einstellungen' : `${n} Schlüssel der Einstellungen`,
        tracking: (n: number) => (n === 1 ? '1 Tracking-Verbindung' : `${n} Tracking-Verbindungen`),
        apps: (n: number) => (n === 1 ? '1 App-Verbindung' : `${n} App-Verbindungen`),
        notifications: (n: number) =>
          n === 1 ? '1 Benachrichtigungsziel' : `${n} Benachrichtigungsziele`,
        shares: (n: number) => (n === 1 ? '1 Freigabepasswort' : `${n} Freigabepasswörter`),
        destinations: (n: number) => (n === 1 ? '1 Backup-Ziel' : `${n} Backup-Ziele`),
        certificates: (n: number) =>
          n === 1
            ? '1 Schlüssel des Let’s-Encrypt-Zertifikats'
            : `${n} Schlüssel des Let’s-Encrypt-Zertifikats`,
        twoStep: (n: number) =>
          n === 1
            ? '1 Schlüssel einer zweistufigen Anmeldung'
            : `${n} Schlüssel zweistufiger Anmeldungen`,
      },
      serverName: (name: string) => `Gib zur Bestätigung den Servernamen ein: ${name}`,
      confirm: 'Wiederherstellen',
      back: 'Andere Backups',
      anyway: 'Trotzdem wiederherstellen',
      starting: 'Aktuelle Datenbank wird gesichert…',
      restoringTitle: 'Wird wiederhergestellt',
      restoringBody:
        'Polyfin stellt das Backup wieder her und startet dann neu. Diese Seite wird neu geladen, sobald der Server antwortet.',
      lastRestore: 'Letzte Wiederherstellung',
      lastRestoreOk: (file: string, by: string) => `${file}, von ${by}`,
      lastRestoreSetup: (file: string) => `${file}, auf der Einrichtungsseite`,
      lastRestoreFailed: (file: string) =>
        `${file} konnte nicht wiederhergestellt werden: Die Datenbank ist unverändert.`,
    },
  },
}

export default backups
