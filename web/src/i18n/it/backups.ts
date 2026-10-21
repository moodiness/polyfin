import type en from '../en/backups'

const backups: typeof en = {
  backups: {
    destinations: {
      title: 'Destinazioni',
      loading: 'Caricamento delle destinazioni…',
      empty: 'Ancora nessuna destinazione.',
      emptyHint:
        'Aggiungi uno storage compatibile S3, un server SFTP o un server WebDAV, per tenere i backup lontani dai dischi di questo server.',
      add: 'Aggiungi una destinazione',
      addTitle: 'Aggiungi una destinazione',
      editTitle: (name: string) => `Modifica ${name}`,
      kind: 'Tipo',
      kinds: { s3: 'Storage compatibile S3', sftp: 'SFTP', webdav: 'WebDAV' },
      kindHelp: {
        s3: 'Amazon S3, Backblaze B2, Cloudflare R2, MinIO, Wasabi e gli altri storage compatibili con S3. Usa un bucket dedicato, o un prefisso in cui scrive solo Polyfin.',
        sftp: 'Un server a cui Polyfin accede tramite SSH, come un NAS. La sua chiave host viene salvata alla prima connessione e verificata in seguito.',
        webdav: 'Una cartella di un server WebDAV, come Nextcloud, ownCloud o un NAS.',
      },
      endpoint: 'Endpoint',
      endpointHelp:
        'L’indirizzo dello storage, senza il bucket, ad esempio https://s3.eu-west-3.amazonaws.com.',
      bucket: 'Bucket',
      prefix: 'Prefisso della cartella',
      prefixHelp:
        'La cartella all’interno del bucket, ad esempio polyfin. Vuoto per la radice del bucket.',
      region: 'Regione',
      regionHelp:
        'Vuoto per us-east-1, accettato dalla maggior parte degli storage. Cloudflare R2 usa auto.',
      accessKey: 'Chiave di accesso',
      secretKey: 'Chiave segreta',
      host: 'Host',
      hostHelp: 'Il nome o l’indirizzo IP del server, senza sftp://.',
      port: 'Porta',
      user: 'Utente',
      password: 'Password',
      sftpPasswordHelp: 'Lascia vuoto se l’accesso avviene con una chiave privata.',
      privateKey: 'Chiave privata',
      privateKeyHelp:
        'Una chiave privata OpenSSH o PEM senza passphrase, la cui chiave pubblica è accettata dal server. Lascia vuoto per accedere con la password.',
      privateKeySaved: 'Una chiave è salvata: incollane un’altra per sostituirla.',
      privateKeyRemove: 'Rimuovi la chiave salvata',
      sftpFolder: 'Cartella',
      sftpFolderHelp:
        'Dove vanno i backup, ad esempio /volume1/backups/polyfin. Vuoto per la cartella home dell’utente.',
      webdavAddress: 'Indirizzo della cartella',
      webdavAddressHelp:
        'L’indirizzo di una cartella esistente, ad esempio https://cloud.example.org/remote.php/dav/files/sam/polyfin/.',
      webdavPasswordHelp: 'Una password per le applicazioni, se il server le offre.',
      kept: 'Backup da conservare',
      keptHelp: (min: number, max: number, byDefault: number) =>
        `Dopo ogni caricamento, Polyfin elimina lì i suoi backup più vecchi oltre questo numero. Gli altri file non vengono mai toccati. Da ${min} a ${max}; ${byDefault} per impostazione predefinita.`,
      hostKey: 'Chiave host',
      hostKeyNone: 'Salvata alla prima connessione.',
      hostKeyForget: 'Dimentica',
      hostKeyForgetHelp:
        'Dimentica la chiave salvata quando la chiave del server è cambiata intenzionalmente: verrà salvata quella offerta alla connessione successiva.',
      hostKeyForgotten: 'La chiave offerta dal server alla connessione successiva verrà salvata.',
      create: 'Aggiungi',
      creating: 'Aggiunta…',
      save: 'Salva',
      saving: 'Salvataggio…',
      created: (name: string) => `${name} aggiunta.`,
      saved: (name: string) => `${name} salvata.`,
      test: 'Verifica',
      testing: 'Verifica…',
      testHelp: 'Scrive un piccolo file, lo rilegge e lo elimina.',
      testOk: (name: string) => `${name} funziona.`,
      testFailed: (name: string) => `${name} non funziona.`,
      edit: 'Modifica',
      delete: 'Elimina',
      deleteTitle: (name: string) => `Eliminare ${name}?`,
      deleteBody: 'I backup non vengono più inviati lì. Quelli già presenti restano.',
      deleted: (name: string) => `${name} eliminata.`,
      keeps: (count: number) => (count === 1 ? 'conserva 1 backup' : `conserva ${count} backup`),
      status: {
        waiting: 'Ancora nessun caricamento',
        working: 'Funzionante',
        failed: 'Non riuscito',
        stale: 'Non aggiornata',
        unreadable: 'Chiave illeggibile',
      },
      lastUpload: 'Ultimo caricamento',
      newest: 'Backup più recente presente',
      none: 'Ancora nessuno',
      unreadable:
        'La password o la chiave non può essere decifrata con POLYFIN_SECRET_KEY: inseriscila di nuovo.',
      staleHelp: 'Il backup più recente presente risale a più di due giorni fa.',
    },
    restore: {
      button: 'Ripristina',
      title: 'Ripristina un backup',
      intro:
        'Il database torna com’era al momento del backup. Scegli un backup della cartella o di una destinazione, oppure invia un file .dump.',
      loading: 'Elenco dei backup…',
      folder: 'Cartella dei backup',
      none: 'Nessun backup qui.',
      beforeRestore: 'prima di un ripristino',
      choose: 'Ripristina questo backup',
      upload: 'Invia un file .dump',
      uploadHelp:
        'Un backup del database di Polyfin, creato da Polyfin o con pg_dump --format=custom.',
      checking: 'Verifica del backup…',
      unavailable: {
        pg_restore_missing:
          'pg_restore non è stato trovato sul server: installa gli strumenti client di PostgreSQL 18, inclusi nell’immagine Docker.',
      },
      confirmTitle: (file: string) => `Ripristinare ${file}?`,
      made: (date: string) => `Creato il ${date}.`,
      from: (name: string) => `Da ${name}.`,
      uploaded: 'Inviato da questo computer.',
      steps: [
        'Polyfin crea prima un backup del database attuale nella cartella dei backup, come backup pre-ripristino che non elimina mai.',
        'Le applicazioni ricevono un errore durante il ripristino, e le riproduzioni, le conversioni e le registrazioni in corso si interrompono.',
        'Polyfin si riavvia poi sul database ripristinato: tutto torna com’era al momento del backup, utenti, impostazioni e cronologia di riproduzione inclusi.',
        'Le applicazioni e gli amministratori che hanno effettuato l’accesso da allora devono accedere di nuovo, tu compreso, e i link di stream già distribuiti smettono di funzionare.',
      ],
      keyTitle: 'Alcune chiavi salvate non sono leggibili',
      keyBody:
        'Questo backup contiene chiavi e password che l’attuale POLYFIN_SECRET_KEY non riesce a decifrare. Imposta la chiave in uso al momento del backup, poi ripristina di nuovo; oppure ripristina comunque e reinseriscile in seguito:',
      kinds: {
        settings: (n: number) =>
          n === 1 ? '1 chiave delle impostazioni' : `${n} chiavi delle impostazioni`,
        tracking: (n: number) =>
          n === 1 ? '1 connessione di tracciamento' : `${n} connessioni di tracciamento`,
        apps: (n: number) =>
          n === 1 ? '1 connessione di applicazione' : `${n} connessioni di applicazioni`,
        notifications: (n: number) =>
          n === 1 ? '1 destinazione delle notifiche' : `${n} destinazioni delle notifiche`,
        shares: (n: number) =>
          n === 1 ? '1 password di condivisione' : `${n} password di condivisione`,
        destinations: (n: number) =>
          n === 1 ? '1 destinazione del backup' : `${n} destinazioni del backup`,
        certificates: (n: number) =>
          n === 1
            ? '1 chiave del certificato Let’s Encrypt'
            : `${n} chiavi del certificato Let’s Encrypt`,
        twoStep: (n: number) =>
          n === 1
            ? '1 chiave di un accesso in due passaggi'
            : `${n} chiavi di accessi in due passaggi`,
      },
      serverName: (name: string) => `Digita il nome del server, ${name}, per confermare`,
      confirm: 'Ripristina',
      back: 'Altri backup',
      anyway: 'Ripristina comunque',
      starting: 'Backup del database attuale…',
      restoringTitle: 'Ripristino in corso',
      restoringBody:
        'Polyfin sta ripristinando il backup, poi si riavvierà. Questa pagina si ricarica appena risponde.',
      lastRestore: 'Ultimo ripristino',
      lastRestoreOk: (file: string, by: string) => `${file}, da ${by}`,
      lastRestoreFailed: (file: string) =>
        `Impossibile ripristinare ${file}: il database è rimasto com’era.`,
    },
  },
}

export default backups
