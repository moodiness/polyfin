import type en from '../en/system'

const system: typeof en = {
  apiKeys: {
    title: 'Chiavi API',
    description:
      'Le chiavi permettono a strumenti e applicazioni, come gestori di richieste o script, di usare l’API Jellyfin di questo server con i diritti di amministratore. Chiunque possieda una chiave ha questi diritti: mantieni le chiavi riservate e revoca quelle che non usi più.',
    listTitle: 'Chiavi',
    empty: 'Ancora nessuna chiave API.',
    created: 'Creata',
    lastUsed: 'Ultimo utilizzo',
    createTitle: 'Crea una chiave',
    app: 'Nome dell’applicazione',
    appHint: 'Chi userà questa chiave, per riconoscerla in seguito. Da 1 a 64 caratteri.',
    create: 'Crea chiave',
    creating: 'Creazione…',
    newKeyTitle: (app: string) => `Chiave per ${app}`,
    newKeyNotice: 'Copia subito questa chiave: non verrà più mostrata.',
    copy: 'Copia',
    copied: 'Copiata.',
    copyFailed: 'Impossibile copiare la chiave. Selezionala e copiala manualmente.',
    done: 'L’ho copiata',
    revoke: 'Revoca',
    revoking: 'Revoca in corso…',
    revokeConfirm: (app: string) =>
      `Revocare la chiave per ${app}? Gli strumenti e le applicazioni che la usano smetteranno di funzionare. L’operazione non può essere annullata.`,
    revoked: (app: string) => `La chiave per ${app} è stata revocata.`,
    emptyHint:
      'Creane una qui sotto per ogni strumento o applicazione, così potrai revocarle singolarmente.',
    revokeTitle: 'Revocare questa chiave?',
    revokeLabel: (app: string) => `Revoca la chiave per ${app}`,
  },
  system: {
    schedule: {
      title: 'Pianificazione',
      description:
        'Le operazioni del server, le prossime registrazioni e i prossimi aggiornamenti della Diretta TV.',
      tasksTitle: 'Operazioni',
      task: 'Operazione',
      lastRun: 'Ultima esecuzione',
      nextRun: 'Prossima esecuzione',
      notRunYet: 'Non eseguita dall’avvio del server',
      byHandOnly: 'Solo esecuzione manuale',
      every: (span: string) => `Intervallo: ${span}`,
      daily: (time: string) => `Ogni giorno alle ${time}, ora del server`,
      took: (span: string) => `durata ${span}`,
      results: {
        Completed: 'Completata',
        Failed: 'Non riuscita',
        Cancelled: 'Interrotta',
      } as Record<string, string>,
      running: 'In esecuzione',
      cancelling: 'Interruzione in corso',
      runNow: 'Esegui ora',
      runLabel: (name: string) => `Esegui ${name} ora`,
      stop: 'Interrompi',
      stopLabel: (name: string) => `Interrompi ${name}`,
      started: (name: string) => `${name} avviata.`,
      autoRefresh: 'Aggiornamento ogni 5 secondi.',
      recordingsTitle: 'Prossime registrazioni',
      recordingsOff: 'La registrazione è disattivata. Attivala in Impostazioni › Registrazioni.',
      noTimers: 'Nessuna registrazione pianificata.',
      recordingNow: 'In registrazione',
      failed: 'Non riuscita',
      series: 'Serie TV',
      scheduledBy: (name: string) => `Pianificata da ${name}`,
      unknownChannel: 'Canale non trovato',
      tunersShort: 'Sintonizzatori insufficienti',
      tunersShortHelp:
        'Sono in corso contemporaneamente più registrazioni sui canali di questo sintonizzatore di quanti sintonizzatori abbia: una di esse potrebbe non riuscire.',
      refreshesTitle: 'Aggiornamenti della Diretta TV',
      refreshesHelp: (hours: number) =>
        `Gli elenchi canali IPTV e le guide dei programmi, sia del server sia degli utenti, vengono scaricati di nuovo ogni ${hours} h (Impostazioni › Diretta TV) dall’operazione «Aggiorna le guide della Diretta TV».`,
      noRefreshes: 'Nessuna origine IPTV né guida dei programmi.',
      channelList: 'Elenco canali',
      iptvLists: 'Elenchi di canali, film e serie TV',
      guide: 'Guida dei programmi',
      due: 'Al prossimo controllo',
      lastDownload: 'Ultimo download',
      nextDownload: 'Prossimo download',
      notYet: 'Non ancora',
      noTasks: 'Nessuna operazione.',
      noTasksHint: 'Il server elenca qui le sue operazioni una volta avviato.',
      recordingsHint:
        'Le registrazioni pianificate da un’applicazione o dalla guida compaiono qui.',
      refreshesHint:
        'Aggiungi un’origine IPTV in Contenuti › Origini, o una guida dei programmi a un catalogo TV, e i relativi download compariranno qui.',
      backupsTitle: 'Backup del database',
      backUpNow: 'Esegui backup ora',
    },
    health: {
      title: 'Stato di salute',
      noIptv: 'Nessuna origine IPTV.',
      owner: { server: 'Server', user: (name: string) => `Add-on di ${name}` },
      description:
        'Lo stato del server e delle sue origini, in base a ciò che Polyfin registra già: questa pagina non invia loro alcuna richiesta.',
      problemsTitle: 'Richiede attenzione',
      allGood: 'Nessun problema rilevato.',
      update: (version: string) => `Polyfin ${version} è disponibile`,
      updateRunning: (version: string) => `Questo server esegue la versione ${version}.`,
      releaseNotes: 'Note di rilascio',
      checkedAt: (time: string) => `Controllato alle ${time}. Aggiornamento ogni 10 secondi.`,
      ok: 'OK',
      warning: 'Avviso',
      error: 'Errore',
      problems: {
        database: 'Il database non risponde.',
        disk: (folder: string, free: string) =>
          `Poco spazio rimasto per ${folder}: ${free} liberi.`,
        addon: (name: string, failure: string) => `${name}: ${failure}`,
        iptv: (name: string) => `${name}: l’ultimo download dell’elenco canali non è riuscito.`,
        tuner: (name: string) =>
          `${name}: il sintonizzatore non è riuscito a riprodurre un canale.`,
        tunerUnreachable: (name: string) => `${name}: il sintonizzatore non risponde.`,
        folder: (name: string) =>
          `${name}: Polyfin non riesce a leggere questa cartella locale. Verifica che sia montata e leggibile dall’utente 65532.`,
        share: (name: string) =>
          `${name}: Polyfin non riesce a leggere questa condivisione di rete. Verifica il suo indirizzo e che il suo utente possa leggerla.`,
        shareUnreachable: (name: string) =>
          `${name}: Polyfin non riesce a raggiungere questa condivisione di rete.`,
        shareRefused: (name: string) =>
          `${name}: la condivisione di rete rifiuta l’utente o la password di Polyfin.`,
        shareHostKey: (name: string) =>
          `${name}: il server SFTP presenta una chiave host diversa da quella salvata e Polyfin la rifiuta finché non viene accettata.`,
        folderWatchLimit: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di cartelle monitorate del sistema. Viene comunque scansionata secondo la sua pianificazione. Aumenta fs.inotify.max_user_watches sull’host.`,
        folderInstanceLimit: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di osservatori o di file aperti del sistema. Viene comunque scansionata secondo la sua pianificazione. Aumenta fs.inotify.max_user_instances sull’host, oppure il limite di file aperti del container.`,
        folderOpenFilesLimit: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa cartella locale: è stato raggiunto il limite di file aperti del sistema, poiché macOS monitora ogni cartella e ogni file tramite uno di essi. Viene comunque scansionata secondo la sua pianificazione. Aumenta kern.maxfilesperproc e kern.maxfiles con sysctl.`,
        folderUnwatched: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa cartella locale. Viene comunque scansionata secondo la sua pianificazione.`,
        shareUnwatchedUnreachable: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete: non è raggiungibile. Viene scansionata secondo la sua pianificazione e di nuovo non appena risponde.`,
        shareUnwatchedRefused: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete: rifiuta l’utente o la password di Polyfin. Verificali e inseriscili di nuovo.`,
        shareUnwatched: (name: string) =>
          `${name}: Polyfin non riesce a monitorare le modifiche di questa condivisione di rete. Viene comunque scansionata secondo la sua pianificazione.`,
        folderNoSearch: (name: string) =>
          `${name}: nessun add-on del server descrive e cerca i titoli dei suoi file, che quindi restano non associati. Aggiungine uno in Contenuti › Origini: gli add-on personali degli utenti, in Le mie origini, non contano.`,
        folderSearch: (name: string, addon: string, failure: string) =>
          `${name}: non è stato possibile cercare i titoli dei suoi file${addon === '' ? '' : failure === '' ? ` (${addon})` : ` (${addon}: ${failure})`}. Restano non associati finché una ricerca non va a buon fine, cosa che Polyfin riprova da solo, dopo qualche minuto, poi meno spesso.`,
        searchLate: (name: string) =>
          `${name}: la sua ricerca non ha risposto in tempo. Le ricerche lo escludono per qualche minuto, poi lo interrogano di nuovo.`,
        guide: (name: string) => `${name}: l’ultimo recupero della guida TV non è riuscito.`,
        paused: (host: string) => `Miniature in pausa per ${host}: ha chiesto di rallentare.`,
        unknownProxy: (address: string) =>
          `${address} inoltra richieste (X-Forwarded-For) ma non è un proxy noto, quindi tutti i suoi client vengono considerati come ${address}. Se è il tuo reverse proxy, aggiungilo a Proxy noti in Impostazioni › Sicurezza.`,
        discoveryPort: (port: number) =>
          `Polyfin non riesce a mettersi in ascolto sulla porta UDP ${port}, quindi le applicazioni Jellyfin sulla rete locale non lo trovano da sole. Probabilmente la usa un altro programma, ad esempio un server Jellyfin sullo stesso host. Salvando le impostazioni si riprova.`,
        torrentRoute: (why: string) =>
          `Gli stream torrent non vengono riprodotti: ${why}. Nulla viene inviato per altra via; verifica il percorso in Impostazioni › Torrent.`,
        torrentWhy: {
          interface_missing: (name: string) => `l’interfaccia di rete ${name} non esiste`,
          interface_down: (name: string) => `l’interfaccia di rete ${name} è disattivata`,
          interface_no_address: (name: string) =>
            `l’interfaccia di rete ${name} non ha un indirizzo`,
          interface_unsupported: () => 'un’interfaccia di rete funziona solo su Linux',
          proxy_unreachable: (name: string) => `il proxy SOCKS5 ${name} non risponde`,
          proxy_refused: (name: string) =>
            `il proxy SOCKS5 ${name} ha rifiutato l’utente e la password`,
          wireguard_endpoint: (name: string) =>
            `il server WireGuard ${name} non può essere risolto`,
          wireguard_down: (name: string) => `il tunnel WireGuard verso ${name} non può avviarsi`,
          wireguard_handshake: (name: string) =>
            `il server WireGuard ${name} non risponde all’handshake: non è raggiungibile oppure una chiave della configurazione è errata`,
          no_route: () => 'nessun percorso impostato',
        },
        torrentUdp: (name: string) =>
          `Il proxy SOCKS5 ${name} non supporta UDP ASSOCIATE: i torrent vengono riprodotti tramite TCP, con tracker UDP e DHT disattivati.`,
        torrentStart: (why: string) =>
          `Un torrent non è partito in tempo: ${why}. Verifica che il percorso raggiunga Internet; il log riporta la risposta di ogni tracker a livello di debug.`,
        torrentStartWhy: {
          no_udp:
            'non è stato trovato nessun peer e il percorso non trasporta UDP, necessario alla DHT e ai tracker UDP',
          refused:
            'i suoi tracker hanno rifiutato l’annuncio o non hanno risposto, e la DHT non ha trovato nessun peer',
          no_peers: 'i suoi tracker e la DHT non hanno trovato nessun peer',
          silent: 'sono stati trovati dei peer, ma nessuno l’ha inviato',
        },
        torrentStartPeers: (found: number, why: string) =>
          `${found} ${found === 1 ? 'peer trovato' : 'peer trovati'}: ${why}`,
        torrentPeerWhy: {
          connected: (n: number) => `${n} ${n === 1 ? 'connesso' : 'connessi'}`,
          timedOut: (n: number) => `${n} senza risposta in tempo`,
          refused: (n: number) =>
            `${n} ${n === 1 ? 'ha rifiutato' : 'hanno rifiutato'} la connessione`,
          unreachable: (n: number) => `${n} ${n === 1 ? 'irraggiungibile' : 'irraggiungibili'}`,
          proxy: (n: number) => `${n} ${n === 1 ? 'rifiutato' : 'rifiutati'} dal proxy`,
          closed: (n: number) =>
            `${n} ${n === 1 ? 'ha chiuso' : 'hanno chiuso'} la connessione durante l’handshake`,
          left: (n: number) => `${n} ${n === 1 ? 'connesso, poi uscito' : 'connessi, poi usciti'}`,
          other: (n: number) => `${n} ${n === 1 ? 'fallito' : 'falliti'} per altri motivi`,
          untried: (n: number) => `${n} non ancora ${n === 1 ? 'contattato' : 'contattati'}`,
        },
        dlnaPort: (port: number) =>
          `Polyfin non riesce a mettersi in ascolto sulla porta UDP ${port}, quindi i dispositivi DLNA sulla rete locale non lo trovano da soli. Probabilmente la usa un altro programma, ad esempio un altro server DLNA sullo stesso host. Salvando le impostazioni si riprova.`,
        discoveryAddress: (address: string) =>
          `Le applicazioni Jellyfin che cercano server sulla rete locale ricevono ${address}, un indirizzo del container di Polyfin che non possono raggiungere. Imposta Indirizzo sulla rete locale in Impostazioni › Generale.`,
        proxyNameUnresolved: (name: string) =>
          `Il proxy noto ${name} non corrisponde a nessun indirizzo, quindi nessuna richiesta viene considerata attendibile come proveniente da esso. Verifica il suo nome in Impostazioni › Sicurezza e che Polyfin condivida una rete con esso.`,
        task: (name: string) => `L’operazione «${name}» non è riuscita.`,
        httpsPort: (port: number) =>
          `Polyfin non riesce a mettersi in ascolto sulla porta TCP ${port}, quindi HTTPS non è disponibile. HTTP continua a funzionare. Probabilmente la usa un altro programma, oppure è inferiore a 1024. Scegli un’altra porta HTTPS in Impostazioni › Sicurezza.`,
        httpsFileMissing: (file: string) =>
          `Il file del certificato HTTPS ${file} non esiste. Resta in uso l’ultimo certificato letto.`,
        httpsFilesMismatch:
          'La chiave privata HTTPS non corrisponde al certificato. Resta in uso l’ultimo certificato letto.',
        httpsFileUnreadable: (file: string) =>
          `Polyfin non riesce a leggere il file del certificato HTTPS ${file}, oppure non contiene un certificato o una chiave PEM leggibile. Resta in uso l’ultimo certificato letto.`,
        httpsRenewal: (error: string) =>
          `Let’s Encrypt non ha rilasciato il certificato HTTPS: ${error}`,
        httpsExpires: (date: string) => `Il certificato HTTPS scade il ${date}.`,
        httpsExpired: (date: string) => `Il certificato HTTPS è scaduto il ${date}.`,
        subtitleOcrLanguage: (name: string, file: string) =>
          `I sottotitoli grafici in ${name} non vengono convertiti in testo: Tesseract non ha dati per questa lingua. Aggiungi il file ${file} alla cartella indicata da POLYFIN_TESSDATA_DIR.`,
        metadataLanguages: (name: string) =>
          `${name}: ogni add-on di metadati del server ha una lingua, quindi descrivono i titoli nella propria lingua per tutti gli utenti, qualunque sia la loro lingua dei metadati. Lascia Lingua delle descrizioni su Nessuna in questo add-on, o in un altro add-on di metadati.`,
        nvidiaDriver: (version: string) =>
          `La scheda grafica NVIDIA non converte i video: il suo driver è precedente alla versione ${version}, necessaria a FFmpeg per NVENC. Aggiorna il driver per usare la scheda.`,
        backupFailed: 'L’ultimo backup del database non è riuscito.',
        backupStale: 'L’ultimo backup del database risale a più di due giorni fa.',
        backupDestinationFailed: (name: string) =>
          `${name}: l’ultimo caricamento dei backup del database non è riuscito.`,
        backupDestinationStale: (name: string) =>
          `${name}: il backup del database più recente presente lì risale a più di due giorni fa.`,
        conversionsFull:
          'Le conversioni video hanno raggiunto il limite: le nuove restano in attesa o vengono riprodotte in altro modo.',
        unreadableSecrets: (names: string) =>
          `Le chiavi salvate non possono essere decifrate con POLYFIN_SECRET_KEY e vengono considerate non impostate: ${names}. Imposta di nuovo la chiave con cui sono state cifrate, oppure inseriscile di nuovo.`,
        plaintextSecrets:
          'Chiavi e token sono salvati senza cifratura. Imposta POLYFIN_SECRET_KEY per cifrarli.',
        appStopped: (app: string, user: string) =>
          `${app} ha rifiutato l’accesso che Polyfin conservava per ${user}: non viene importato nulla finché ${user} non accede di nuovo in Il mio account, con Resta connesso selezionato.`,
        trackingStopped: (service: string, user: string) =>
          `${service} ha rifiutato il token o la chiave che Polyfin conservava per ${user}: non viene inviato né importato nulla finché ${user} non preme di nuovo Connetti in Il mio account › Monitoraggio.`,
      },
      addonsTitle: 'Add-on',
      addonsHelp:
        'In base alle richieste fatte per le applicazioni dall’avvio del server. «Verifica» chiede una volta il manifest a un add-on, al massimo una volta al minuto.',
      noAddons: 'Il server non ha alcun add-on.',
      addonStatus: {
        ok: 'Risponde',
        failing: 'Non funzionante',
        unknown: 'Nessuna richiesta dall’avvio',
        off: 'Disattivato',
      },
      failures: {
        unreachable: 'non è raggiungibile',
        timeout: 'non ha risposto in tempo',
        private_network: 'si trova su una rete locale',
        rate_limited: 'ha chiesto di rallentare (troppe richieste)',
        server_error: 'ha risposto con un errore del server',
        http_error: 'ha rifiutato la richiesta',
        invalid_response: 'ha inviato una risposta troppo grande',
      } as Record<string, string>,
      lastSuccess: 'Ultima risposta',
      lastFailure: 'Ultimo errore',
      responseTime: 'Tempo di risposta',
      requests: (count: number, failures: number) =>
        `${count} ${count === 1 ? 'richiesta' : 'richieste'}, ${failures} non ${failures === 1 ? 'riuscita' : 'riuscite'}`,
      check: 'Verifica',
      checkLabel: (name: string) => `Verifica ${name} ora`,
      checking: 'Verifica in corso…',
      iptvTitle: 'Origini IPTV',
      guidesTitle: 'Guide dei programmi',
      sharesTitle: 'Condivisioni di rete',
      shareKinds: {
        smb: 'SMB',
        webdav: 'WebDAV',
        s3: 'S3',
        sftp: 'SFTP',
        rclone: 'rclone',
      } as Record<string, string>,
      shareStatus: {
        notified: 'Notificato',
        polled: 'Controllato periodicamente',
        unwatched: 'Non monitorato',
        scans: 'Solo durante le scansioni',
      },
      sharePolledEvery: (share: string, minutes: number) =>
        `${share === 's3' ? 'I bucket S3 non segnalano' : share === 'sftp' ? 'I server SFTP non segnalano' : 'I remote rclone non segnalano'} le modifiche: le sottocartelle della cartella vengono rielencate ${minutes === 1 ? 'ogni minuto' : `ogni ${minutes} minuti`}, come richiesto dalle sue impostazioni.`,
      shareScansOnly:
        'Le sue impostazioni richiedono di cercare le modifiche solo durante le scansioni: la cartella viene letta secondo la sua pianificazione e quando viene scansionata manualmente.',
      shareNotified:
        'Il server segnala a Polyfin ogni modifica: la condivisione viene scansionata di nuovo pochi secondi dopo. Le sue cartelle vengono anche rielencate ogni 10 minuti, nel caso una modifica non venga segnalata.',
      sharePartial:
        'Il server segnala a Polyfin le modifiche fatte tramite la rete, ma ne ha mancata una in una cartella della condivisione, ad esempio una fatta direttamente sul disco del server: le cartelle della condivisione vengono anche rielencate ogni 2 minuti.',
      sharePolled: {
        webdav:
          'I server WebDAV non segnalano le modifiche: le cartelle della condivisione vengono rielencate ogni 2 minuti.',
        unsupported:
          'Questo server SMB non segnala le modifiche: le cartelle della condivisione vengono rielencate ogni 2 minuti.',
        silent:
          'Questo server SMB non ha segnalato una modifica trovata durante un elenco: le cartelle della condivisione vengono rielencate ogni 2 minuti.',
        failed:
          'Polyfin ha perso le notifiche del server e le richiederà a breve. Nel frattempo, le cartelle della condivisione vengono rielencate ogni 2 minuti.',
        starting:
          'Polyfin sta chiedendo al server di segnalare le modifiche. Nel frattempo, le cartelle della condivisione vengono rielencate ogni 2 minuti.',
      } as Record<string, string>,
      shareUnwatched: {
        unreachable:
          'La condivisione non è raggiungibile. Viene scansionata secondo la sua pianificazione e di nuovo non appena risponde.',
        refused:
          'La condivisione rifiuta l’utente o la password di Polyfin. Verificali e inseriscili di nuovo.',
        host_key_changed:
          'Il server SFTP presenta una chiave host diversa da quella salvata. Accettala in Modifica la cartella se la modifica è prevista.',
        failed:
          'Polyfin non può monitorare questa condivisione. Viene comunque scansionata secondo la sua pianificazione.',
      } as Record<string, string>,
      sharesOff:
        'Il monitoraggio delle modifiche è disattivato nelle impostazioni: le condivisioni vengono scansionate solo secondo la loro pianificazione.',
      transcoderTitle: 'Transcodifica',
      gpu: 'GPU',
      noGpu: 'Nessuna: il video viene convertito dalla CPU',
      toneMapping: 'Da HDR a SDR sulla GPU',
      cpuEncoders: 'Encoder CPU',
      conversions: 'Conversioni video',
      noLimit: 'nessun limite',
      remuxes: 'Remux',
      maxHeight: 'Convertito fino a',
      transcodingOff: 'La conversione è disattivata nelle impostazioni.',
      databaseTitle: 'Database',
      reachable: 'Risponde',
      unreachable: 'Non risponde',
      size: 'Dimensione',
      cacheTitle: 'Cache delle origini',
      cacheHelp:
        'Parti dei file in riproduzione, conservate su disco fino alla dimensione impostata in Impostazioni › Riproduzione. Una cache piena è normale: le parti più vecchie lasciano spazio.',
      cacheUse: (used: string, limit: string) => `${used} di ${limit}`,
      imagesTitle: 'Cache delle immagini',
      imagesHelp:
        'Le immagini inoltrate alle app, le loro copie ridimensionate e le immagini che Polyfin disegna, conservate su disco fino al suo limite, eliminando prima quelle usate meno di recente. Svuotarla fa scaricare, ridimensionare o disegnare di nuovo ogni immagine alla sua prossima richiesta. Le miniature per scorrere, le immagini caricate, le immagini delle cartelle locali e il tema del lettore web non ne fanno parte.',
      imagesFiles: 'Immagini su disco',
      imagesMemory: 'In memoria',
      clearImages: 'Svuota la cache delle immagini',
      imagesCleared: (images: string, bytes: string) =>
        `Cache delle immagini svuotata: ${images} immagini, ${bytes} liberati su disco.`,
      sourcesOpen: 'File aperti',
      torrentPieces: 'Di cui parti di torrent',
      torrentsTitle: 'Torrent',
      torrentsHelp:
        'I torrent che la riproduzione sta leggendo ora, solo attraverso il loro percorso. Un torrent si ferma quando termina la sua riproduzione: niente più peer, niente più condivisione.',
      torrentRoute: {
        interface: 'Interfaccia di rete',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Tunnel WireGuard',
        '': 'Nessun percorso',
      },
      torrentAddresses: 'Indirizzi del tunnel',
      torrentHandshake: 'Ultimo handshake',
      torrentNoHandshake: 'Ancora nessuno',
      torrentsPlaying: 'Torrent in riproduzione',
      torrentsPlayingOf: (playing: number, atOnce: number) => `${playing} su ${atOnce}`,
      torrentPeers: 'Peer',
      torrentUTPPeers: 'Peer tramite uTP',
      torrentsShared: 'Condiviso dall’avvio',
      disksTitle: 'Spazio su disco',
      folders: { cache: 'cache', recordings: 'registrazioni', backups: 'backup' } as Record<
        string,
        string
      >,
      folderTitle: {
        cache: 'Cartella della cache',
        recordings: 'Cartella delle registrazioni',
        backups: 'Cartella dei backup',
      } as Record<string, string>,
      free: (free: string) => `${free} liberi`,
      notMeasured: 'Impossibile misurare qui.',
      thumbnailsTitle: 'Miniature (trickplay)',
      thumbnailsOff:
        'Le miniature e le immagini dei capitoli sono disattivate in Impostazioni › Miniature (trickplay).',
      waiting: (count: number, length: number) => `${count} di ${length} in attesa`,
      working: 'Creazione delle immagini in corso',
      idle: 'Inattivo',
      pausedHosts: 'Origini in pausa',
      noPausedHosts: 'Nessuna origine in pausa.',
      until: (time: string) => `fino alle ${time}`,
      ratingsTitle: 'Voti da MDBList',
      ratingsHelp:
        'Le richieste che MDBList concede ogni giorno alla chiave del server per i voti, e quelle rimanenti fino all’azzeramento giornaliero. I titoli i cui voti non sono conservati vengono richiesti a gruppi, in background.',
      requestsLeft: 'Richieste rimanenti oggi',
      requestsLeftOf: (left: string, limit: string) => `${left} di ${limit}`,
      notToldYet: 'Noto alla prossima richiesta a MDBList',
      resets: 'Azzeramento giornaliero',
      ratingsHeld: 'Nuova richiesta',
      heldReasons: {
        limit: 'MDBList limita le richieste',
        refused: 'MDBList ha rifiutato la chiave',
        failed: 'MDBList non ha risposto',
      } as Record<string, string>,
      notHeld: 'Ora',
      titlesWaiting: 'Titoli in attesa',
      titlesKept: 'Titoli con voti conservati',
      backupTitle: 'Backup',
      backupOff: 'I backup sono disattivati. Attivali in Impostazioni › Backup.',
      backup: {
        result: 'Ultima esecuzione',
        none: 'Ancora nessun backup',
        succeeded: 'Riuscito',
        failed: 'Non riuscito',
        made: 'Ultimo backup eseguito',
        file: 'File',
        size: 'Dimensione',
        next: 'Prossimo backup',
        runHint:
          'Esegui «Esegui il backup del database» nella pagina Pianificazione per eseguire subito il backup.',
      },
      processTitle: 'Polyfin',
      version: 'Versione',
      uptime: 'In esecuzione da',
      memory: 'Memoria',
      heap: 'In uso',
      goroutines: 'Goroutine',
      goroutinesDownload: 'Scarica le goroutine',
      goroutinesHelp:
        'Quando una richiesta non riceve mai risposta, scarica ciò che il server sta facendo e invialo con la tua segnalazione. Non contiene indirizzi né segreti.',
      secretsTitle: 'Chiavi memorizzate',
      secretsHelp:
        'Le chiavi del server, i token e le chiavi di tracciamento degli utenti e le password delle condivisioni di rete, crittografati nel database con POLYFIN_SECRET_KEY (openssl rand -base64 32).',
      encryption: 'Crittografia',
      encrypted: 'Attiva',
      notEncrypted: 'Disattivata: POLYFIN_SECRET_KEY non è impostata',
      storedPlain: 'Memorizzate senza crittografia',
      unreadable: 'Impossibile decrittografare',
      noneUnreadable: 'Nessuna',
      proxiesTitle: 'Proxy noti',
      proxiesHelp:
        'I nomi host elencati in Proxy noti, risolti a ogni modifica delle impostazioni, ogni minuto e non appena un indirizzo sconosciuto inoltra richieste. Ogni indirizzo in cui si risolve un nome è considerato attendibile come proxy.',
      resolvesToNothing: 'Non si risolve in alcun indirizzo',
      secretNames: {
        publicMetaDbKey: 'Chiave PublicMetaDB',
        theIntroDbKey: 'Chiave TheIntroDB',
        mdblistRatingsKey: 'Chiave API MDBList per i voti',
        tmdbKey: 'Chiave API TMDB',
        traktClientSecret: 'Client secret di Trakt',
        lastFmSecret: 'Segreto condiviso di Last.fm',
        aniListClientSecret: 'Client secret di AniList',
        myAnimeListClientSecret: 'Client secret di MyAnimeList',
        smtpPassword: 'Password SMTP',
        torrentProxyPassword: 'Password del proxy SOCKS5 dei torrent',
        torrentWireGuard: 'Configurazione WireGuard dei torrent',
        metricsToken: 'Token delle metriche',
        updateToolToken: 'Token dello strumento di aggiornamento',
      },
      connectionOf: (service: string, user: string) => `connessione ${service} di ${user}`,
      targetOf: (target: string, user: string) =>
        `destinazione delle notifiche «${target}» di ${user}`,
      serverTargetOf: (target: string) => `destinazione delle notifiche del server «${target}»`,
      sharePasswordOf: (folder: string) => `password o chiave della condivisione «${folder}»`,
      destinationSecretOf: (name: string) =>
        `password o chiave della destinazione del backup «${name}»`,
      certificateKeys: 'chiavi del certificato Let’s Encrypt',
      twoStepOf: (user: string) => `chiave dell’accesso in due passaggi di ${user}`,
      noAddonsHint: 'Aggiungi un add-on Stremio in Contenuti › Origini.',
      noIptvHint: 'Aggiungi un account IPTV in Contenuti › Origini.',
      noGuides: 'Nessuna guida dei programmi.',
      noGuidesHint: 'Aggiungi una guida dei programmi a un catalogo TV in Contenuti › Diretta TV.',
      serverTitle: 'Server',
    },
    logs: {
      title: 'Log',
      description:
        'Le ultime righe del log del server, conservate in memoria. Degli indirizzi resta solo l’host, e password, token e chiavi sono nascosti.',
      follow: 'Segui',
      pause: 'Pausa',
      level: 'Livello',
      levels: {
        all: 'Tutti',
        info: 'Info e superiori',
        warn: 'Avvisi ed errori',
        error: 'Solo errori',
      },
      search: 'Cerca',
      download: 'Scarica',
      clear: 'Svuota la vista',
      empty: 'Ancora nessuna riga.',
      noMatch: 'Nessuna riga corrispondente.',
      shown: (shown: number, total: number) =>
        total === 1
          ? `${shown} di ${total} riga mostrata.`
          : `${shown} di ${total} righe mostrate.`,
      following: 'In ascolto: le nuove righe compaiono ogni 2 secondi.',
      paused: 'In pausa: le righe scritte nel frattempo compariranno quando riprendi a seguire.',
      detailedHint:
        'Le righe di debug compaiono quando «Log dettagliato» è attivo, in Impostazioni › Diagnostica.',
      region: 'Log del server',
      emptyHint: 'Le righe compaiono qui man mano che il server le scrive.',
      noMatchHint: 'Scegli un altro livello o modifica la ricerca.',
      filters: 'Filtra il log',
    },
  },
}

export default system
