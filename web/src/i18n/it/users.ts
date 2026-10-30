import type en from '../en/users'

const users: typeof en = {
  users: {
    title: 'Utenti',
    description: 'Crea e gestisci gli account autorizzati a usare questo server.',
    listTitle: 'Account',
    empty: 'Ancora nessun utente.',
    administrator: 'Amministratore',
    hidden: 'Nascosto nella schermata di accesso',
    disabled: 'Disabilitato',
    you: 'Tu',
    createTitle: 'Crea un utente',
    name: 'Nome',
    password: 'Password',
    isAdministrator: 'Amministratore',
    isAdministratorHelp:
      'Gli amministratori possono gestire gli utenti e le impostazioni del server.',
    showOnSignIn: 'Mostra nella schermata di accesso',
    showOnSignInHelp:
      'Gli utenti visibili sono elencati nella schermata di accesso delle applicazioni Jellyfin. Gli utenti nascosti digitano il proprio nome, come avviene di default in Jellyfin.',
    create: 'Crea utente',
    creating: 'Creazione…',
    created: (name: string) => `${name} è stato creato.`,
    edit: 'Modifica',
    close: 'Chiudi',
    rename: 'Rinomina',
    renamed: 'Il nome è stato modificato.',
    resetPassword: 'Reimposta password',
    newPassword: 'Nuova password',
    resetPasswordHelp:
      'Cambiare la password disconnette tutti i dispositivi Jellyfin di questo utente.',
    passwordReset: 'La password è stata reimpostata.',
    accessTitle: 'Accesso',
    isDisabled: 'Disabilitato',
    isDisabledHelp:
      'Un utente disabilitato non può accedere e tutti i suoi dispositivi vengono disconnessi.',
    providerOnly: 'Solo provider di identità',
    providerOnlyHelp:
      'L’utente accede con il provider di identità configurato in Impostazioni › Sicurezza, mai con una password: la password viene rifiutata, e un account creato così non ne ha bisogno. Le applicazioni Jellyfin che non possono aprire il provider accedono con Quick Connect.',
    ended: 'Scaduto',
    endsOn: (date: string) => `Scade il ${date}`,
    endedOn: (date: string) => `Scaduto il ${date}`,
    endTitle: 'Scade il',
    endHelp:
      'A partire da questa data, nel fuso orario del server, l’account viene rifiutato come se fosse disabilitato: non può accedere e i suoi dispositivi vengono rifiutati. Tutto ciò che contiene viene conservato, e una data successiva, oppure «Mai», lo ripristina.',
    endAdministratorHelp:
      'L’account di un amministratore non scade mai: rendere un utente amministratore cancella la sua data di fine.',
    endDate: 'Data di fine',
    saveEnd: 'Salva data',
    endNever: 'Mai',
    canTranscode: 'Può usare la conversione (transcodifica)',
    canTranscodeHelp:
      'Consente al server di ricodificare video e audio quando l’applicazione di questo utente non riesce a riprodurre un file così com’è. Senza questa opzione, i file vengono riprodotti così come sono o semplicemente reimpacchettati.',
    canDownload: 'Può scaricare',
    canDownloadHelp:
      'Consente a questo utente di salvare titoli nelle applicazioni Jellyfin per guardarli offline.',
    canPlayTorrents: 'Può riprodurre stream torrent',
    canPlayTorrentsHelp:
      'Offre a questo utente l’elenco dei torrent degli add-on, mentre Impostazioni › Torrent li riproduce. Disattivato per tutti, amministratori inclusi, finché non viene concesso.',
    noTranscoding: 'Nessuna conversione',
    noDownloads: 'Nessun download',
    turnOffDownloads: 'Disattiva i download per tutti',
    turnOffDownloadsConfirm:
      'Tutti gli utenti e tutti i gruppi perdono il permesso di scaricare, anche nelle applicazioni Jellyfin con accesso già effettuato. Puoi restituirlo a un utente o a un gruppo dalla sua pagina, in Accesso.',
    turningOffDownloads: 'Disattivazione…',
    downloadsTurnedOff: (count: number) =>
      count === 0
        ? 'Nessuno poteva scaricare.'
        : count === 1
          ? '1 utente non può più scaricare.'
          : `${count} utenti non possono più scaricare.`,
    lastAdminHelp:
      'L’ultimo amministratore abilitato non può essere eliminato, declassato o disabilitato.',
    updated: 'Modifiche salvate.',
    metadataLanguageTitle: 'Lingua dei metadati',
    metadataLanguageHelp:
      'La lingua dei nomi, delle trame e delle immagini dei titoli nelle applicazioni di questo utente, quando uno degli add-on del server li descrive in quella lingua; i titoli che nessun add-on descrive in quella lingua mantengono la descrizione del server. Generi, classificazioni e persone restano gli stessi per tutti, così il controllo parentale e i generi bloccati valutano ogni utente allo stesso modo. I nomi creati da Polyfin stesso la seguono quando Polyfin la parla: inglese, francese, tedesco, spagnolo, italiano o portoghese.',
    metadataLanguageNone:
      'Nessun add-on del server ha ancora una lingua. Assegnane una a un add-on dalla sua pagina in Contenuti › Origini.',
    parentalTitle: 'Controllo parentale',
    maxRating: 'Classificazione massima',
    maxRatingHelp:
      'I titoli con una classificazione superiore a questo limite vengono nascosti a questo utente.',
    noLimit: 'Nessun limite',
    blockUnratedMovies: 'Blocca i film senza classificazione',
    blockUnratedShows: 'Blocca le serie TV senza classificazione',
    ratingLimit: (rating: string) => `Fino a ${rating}`,
    saveParental: 'Salva controllo parentale',
    devicesTitle: 'Dispositivi',
    delete: 'Elimina utente',
    deleteConfirm: (name: string) =>
      `Eliminare ${name}? I suoi dispositivi verranno disconnessi e l’operazione non può essere annullata.`,
    deleting: 'Eliminazione…',
    deleted: (name: string) => `${name} è stato eliminato.`,
    canAddAddons: 'Può aggiungere i propri add-on',
    canAddAddonsHelp:
      'Consente a questo utente di aggiungere add-on Stremio personali dalla sua pagina Le mie origini. Senza questa opzione, i suoi add-on vengono conservati ma non utilizzati.',
    noPersonalAddons: 'Nessun add-on personale',
    blockedUntil: (time: string) => `Bloccato fino alle ${time}`,
    blockedHelp:
      'Sono state inserite troppe password errate per questo account: non può accedere fino ad allora, nemmeno con la password corretta.',
    unblock: 'Sblocca',
    unblocking: 'Sblocco…',
    unblocked: 'L’account è sbloccato.',
    twoStepOn: 'L’accesso in due passaggi è attivo.',
    twoStepHelp:
      'Se questo utente ha perso il telefono con l’applicazione di autenticazione e non ha più codici di recupero, disattivalo: accederà con la sola password, poi potrà configurarlo di nuovo da Il mio account.',
    turnOffTwoStep: 'Disattiva l’accesso in due passaggi',
    turningOffTwoStep: 'Disattivazione…',
    turnOffTwoStepTitle: (name: string) => `Disattivare l’accesso in due passaggi di ${name}?`,
    turnOffTwoStepConfirm: (name: string) =>
      `${name} accederà quindi con la sola password. Il registro attività indica che l’hai disattivato.`,
    twoStepTurnedOff: 'L’accesso in due passaggi è disattivato per questo utente.',
    playbackAccessTitle: 'Riproduzione e accesso',
    maxPlaybacks: 'Riproduzioni simultanee (0 = nessun limite)',
    maxPlaybacksHelp:
      'Quanti dispositivi di questo utente possono riprodurre contemporaneamente. Può comunque accedere da altri dispositivi.',
    maxBitrate: 'Qualità massima',
    maxBitrateHelp:
      'Oltre questo valore, il video viene convertito a una qualità inferiore se questo utente può usare la conversione; altrimenti viene riprodotta una versione più leggera.',
    bitrateNoLimit: 'Nessun limite',
    bitrate4k: '40 Mbit/s (4K)',
    bitrate1080High: '20 Mbit/s (1080p, alta qualità)',
    bitrate1080: '10 Mbit/s (1080p)',
    bitrate720: '4 Mbit/s (720p)',
    bitrate480: '2 Mbit/s (480p)',
    bitrateOther: (mbits: string) => `${mbits} Mbit/s`,
    liveTv: 'Diretta TV',
    liveTvHelp: 'Mostra i canali della Diretta TV a questo utente.',
    syncPlay: 'Guarda insieme',
    syncPlayHelp:
      'Riproduci lo stesso titolo in sincronia con altri, nelle applicazioni Jellyfin che lo offrono.',
    syncPlayCreateAndJoin: 'Crea gruppi e partecipa',
    syncPlayJoin: 'Solo partecipazione',
    syncPlayNone: 'No',
    remoteControl: 'Può controllare le applicazioni di altri utenti',
    remoteControlHelp:
      'Consente a questo utente di avviare, mettere in pausa o inviare messaggi alle applicazioni Jellyfin di altri utenti. Ognuno può controllare le proprie applicazioni.',
    liveTvManagement: 'Può registrare la Diretta TV',
    liveTvManagementHelp: 'Pianifica ed elimina le registrazioni.',
    savePlaybackAccess: 'Salva riproduzione e accesso',
    visibleLibrariesTitle: 'Librerie visibili',
    visibleLibrariesHelp:
      'Le librerie del server mostrate dalle applicazioni di questo utente. Le librerie aggiunte in seguito vengono mostrate. Questa scelta riguarda solo le librerie mostrate: i titoli restano raggiungibili tramite altre librerie, la ricerca o i link. Per bloccare dei titoli, usa il controllo parentale o i generi bloccati.',
    noServerLibraries: 'Il server non ha ancora nessuna libreria.',
    saveVisibleLibraries: 'Salva librerie visibili',
    blockedGenresTitle: 'Generi bloccati',
    blockedGenresHelp:
      'I titoli di uno qualsiasi di questi generi sono nascosti ovunque a questo utente, come i titoli oltre il suo limite di controllo parentale.',
    genre: 'Genere',
    genreHint: 'Scegli un genere dalle librerie del server o digitane uno.',
    addGenre: 'Aggiungi',
    removeGenre: (genre: string) => `Rimuovi ${genre}`,
    noBlockedGenres: 'Nessun genere bloccato.',
    saveBlockedGenres: 'Salva generi bloccati',
    allowedHoursTitle: 'Orari consentiti',
    allowedHoursHelp:
      'Se sono impostati degli orari, questo utente può accedere e usare il server solo durante tali orari, nel fuso orario del server. Al di fuori, le sue applicazioni vengono rifiutate.',
    noAllowedHours:
      'Nessun orario impostato: questo utente può usare il server in qualsiasi momento.',
    day: 'Giorno',
    from: 'Dalle',
    to: 'Alle',
    addHours: 'Aggiungi orario',
    removeHours: 'Rimuovi',
    hoursOrder: 'Ogni orario di fine deve essere successivo al relativo orario di inizio.',
    saveAllowedHours: 'Salva orari consentiti',
    days: {
      Sunday: 'Domenica',
      Monday: 'Lunedì',
      Tuesday: 'Martedì',
      Wednesday: 'Mercoledì',
      Thursday: 'Giovedì',
      Friday: 'Venerdì',
      Saturday: 'Sabato',
      Everyday: 'Tutti i giorni',
      Weekday: 'Giorni feriali (da lunedì a venerdì)',
      Weekend: 'Fine settimana',
    },
    canManageCollections: 'Può gestire le collezioni',
    canManageCollectionsHelp:
      'Consente a questo utente di creare collezioni di titoli nelle applicazioni Jellyfin, aggiungervi o rimuoverne titoli ed eliminarle. Ogni utente vede le collezioni, ciascuna solo con i titoli che può vedere.',
    resetPinRequested: 'Reimpostazione password richiesta: PIN',
    resetPinValidUntil: (time: string) => `, valido fino alle ${time}`,
    resetPinHelp:
      'Comunica questo PIN all’utente: accederà con esso e diventerà la sua nuova password.',
    canManageSubtitles: 'Può gestire i sottotitoli',
    canManageSubtitlesHelp:
      'Consente a questo utente di aggiungere file di sottotitoli ai titoli dalle sue applicazioni Jellyfin e di cercarvi i sottotitoli degli add-on.',
    canShare: 'Può condividere titoli',
    canShareHelp:
      'Consente a questo utente di condividere un film o un episodio con qualcuno senza account, tramite un link con scadenza, da Il mio account o dalla pagina di un titolo nel lettore web, finché Impostazioni › Sicurezza › Consenti link di condivisione è attivo. Disattivandolo, i suoi link vengono revocati.',
    shareLinksTitle: 'Link condivisi',
    shareLinksHelp:
      'I link creati da questo utente per condividere un film o un episodio. «Revoca» ne interrompe uno subito.',
    qualityGroup: 'Gruppo di qualità',
    qualityGroupHelp:
      'La risoluzione massima offerta a questo utente. Le versioni superiori vengono escluse finché ce n’è una adatta; se nessuna è adatta, vengono convertite a una qualità inferiore, oppure rifiutate se questo utente non può usare la conversione. Anche la Diretta TV viene convertita a una qualità inferiore. Solo le versioni adatte vengono scaricate così come sono; con «Download convertiti» attivo in Impostazioni › Conversione (transcodifica), le applicazioni che lo richiedono scaricano le altre convertite a una qualità inferiore.',
    qualityGroupOriginal: 'Originale (nessun limite)',
    member: 'Membro',
    lastSignInLabel: 'Ultimo accesso:',
    active: 'Attivo',
    pinRequested: 'PIN richiesto',
    emptyHelp:
      'Crea un account per ogni persona che guarda qui, poi accedi con esso da un’applicazione Jellyfin.',
    devicesEmptyHelp: 'Le applicazioni compaiono qui non appena accedono con questo account.',
    notFound: 'Utente non trovato',
    notFoundHelp:
      'Questo utente non esiste più, oppure l’indirizzo è errato. Scegli un utente dall’elenco.',
    backToUsers: 'Torna agli utenti',
    sectionsLabel: 'Sezioni dell’utente',
    profileTitle: 'Nome e password',
    accessHelp: 'Ogni modifica viene salvata subito.',
    noServerLibrariesHelp:
      'Aggiungi una libreria in Contenuti › Librerie, poi scegli qui quali può vedere questo utente.',
    jellyfinImport: {
      open: 'Importa da un altro server',
      title: 'Importa da un altro server',
      description:
        'Trasferisci gli account e i relativi dati di visione da un server Jellyfin, Emby o Plex: nomi degli account e stato di amministratore, poi per ogni utente i film e gli episodi visti con le relative date, i punti di ripresa e i preferiti, abbinati tramite IMDb, TMDB o TVDB. I dati di Polyfin vengono solo integrati e sull’altro server non viene scritto nulla. I server non forniscono le password: ne imposti una per ogni nuovo account, oppure mantieni quella con cui l’utente accede.',
      serverKind: 'Server',
      servers: {
        jellyfin: 'Jellyfin',
        emby: 'Emby',
        plex: 'Plex',
      },
      serverKindHelp: {
        jellyfin: 'Jellyfin, o un server compatibile con la sua API.',
        emby: 'Emby, con le sue chiavi API e i suoi utenti.',
        plex: 'Plex Media Server, letto con il token del proprietario.',
      },
      connectTitle: (server: string) => `Connettiti a ${server}`,
      connectHelp:
        'L’indirizzo, la chiave e le password servono solo per questa importazione: Polyfin non li salva.',
      address: 'Indirizzo del server',
      addressHelp: (server: string) =>
        `L’indirizzo con cui apri ${server} in un browser. Va bene anche un indirizzo della rete locale.`,
      plexAddressHelp:
        'L’indirizzo di Plex Media Server stesso, di solito sulla porta 32400. Va bene anche un indirizzo della rete locale.',
      connectWith: 'Connettiti con',
      withApiKey: 'Una chiave API',
      withAccount: 'Un account utente',
      apiKey: 'Chiave API',
      apiKeyHelp:
        'Creane una nel pannello di controllo di Jellyfin, in Chiavi API: legge i dati di visione di tutti gli utenti.',
      embyApiKeyHelp:
        'Creane una nelle impostazioni di Emby, in Chiavi API: legge i dati di visione di tutti gli utenti. La chiave di un singolo utente non è accettata: Emby non indica a chi appartiene.',
      plexToken: 'Token Plex',
      plexTokenHelp:
        'Il token del proprietario del server, che Polyfin usa solo per leggere il server. Per trovarlo, apri Plex in un browser dopo aver effettuato l’accesso come proprietario. Apri il menu di un film qualsiasi, scegli Get Info, poi View XML: l’indirizzo della pagina che si apre termina con X-Plex-Token= seguito dal token. Copia quello che segue.',
      accountName: 'Nome utente',
      accountNameHelp: (server: string) =>
        `L’account di un amministratore elenca tutti gli utenti. Polyfin accede come fanno le applicazioni di ${server}, poi esce.`,
      accountPassword: 'Password',
      accountPasswordHelp: 'Lascia vuoto se l’account non ha una password.',
      connect: 'Connetti',
      connecting: 'Connessione…',
      chooseTitle: 'Scegli chi importare',
      chooseHelp: (server: string, product: string, version: string) =>
        `${server}, ${product} ${version}. Per ogni utente, scegli l’account Polyfin in cui importare, oppure creane uno.`,
      changeServer: 'Cambia server',
      noUsers: (product: string) => `Questo server ${product} non ha utenti.`,
      lastActivityLabel: 'Ultima attività:',
      importAs: 'Importa in',
      skip: 'Non importare',
      newUser: 'Nuovo utente',
      watchData: 'Importa i dati di visione',
      watchDataHelp: 'Titoli visti con le relative date, punti di ripresa e preferiti.',
      plexOwnerWatchDataHelp:
        'Titoli visti con le relative date e il numero di riproduzioni, e punti di ripresa.',
      plexWatchDataHelp:
        'I titoli che questo account ha riprodotto fino alla fine su questo server, dalla cronologia di riproduzione di Plex, con le relative date e il numero di riproduzioni. I titoli che ha segnato come visti o sincronizzato dal suo account Plex, e i suoi punti di ripresa, restano su Plex.',
      watchDataNeeded: 'Senza questi dati, per questo utente non viene importato nulla.',
      plexNotice:
        'Con il token del proprietario, Plex consente a Polyfin di leggere i titoli visti e i punti di ripresa del proprietario. Degli altri account fornisce solo la cronologia di riproduzione su questo server: i titoli che vi hanno riprodotto fino alla fine. I titoli segnati come visti senza riprodurli, quelli sincronizzati dal loro account Plex e i loro punti di ripresa non vi compaiono e restano su Plex. Plex non ha preferiti da importare.',
      ownerNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Con l’accesso come ${owner ?? 'un utente'}, Polyfin legge solo i dati di visione di quell’utente.` : `Questa chiave appartiene a ${owner ?? 'un utente'}: legge solo i dati di visione di quell’utente.`} Per importare quelli di un altro utente, inserisci la password del suo account sul server: Polyfin accede come quell’utente per leggerli. Senza password, il suo account può comunque essere creato.`,
      ownerOnlyNotice: (owner: string | null, signedIn: boolean) =>
        `${signedIn ? `Accesso effettuato come ${owner ?? 'questo utente'}` : `Questa chiave appartiene a ${owner ?? 'un utente'}`}, che non è amministratore di questo server: Polyfin importa solo questo account. Per elencare gli altri utenti del server, connettiti con l’account di un amministratore o con una chiave API.`,
      serverPassword: (server: string) => `Password su ${server}`,
      serverPasswordHelp: (user: string) =>
        `Polyfin accede come ${user} per leggere i suoi dati di visione, poi esce. Lascia vuoto se l’account non ha una password.`,
      keepPassword: 'Mantieni questa password in Polyfin',
      keepPasswordHelp:
        'Il nuovo account accede a Polyfin con la stessa password, se ha almeno 8 caratteri.',
      nothingToImport: 'Scegli un utente da creare o dei dati di visione da importare.',
      start: 'Avvia importazione',
      starting: 'Avvio…',
      started: (created: number, importing: boolean) =>
        created === 0
          ? 'Importazione avviata.'
          : `${created === 1 ? '1 utente creato.' : `${created} utenti creati.`}${importing ? ' Importazione avviata.' : ''}`,
      currentTitle: 'Importazione in corso',
      lastTitle: 'Ultima importazione',
      startedLabel: 'Avviata',
      endedLabel: 'Terminata',
      startedBy: (name: string, own: boolean) =>
        own ? `Cronologia di visione di ${name}, da Il mio account` : `Avviata da ${name}`,
      runningHelp:
        'Puoi lasciare questa pagina: l’importazione continua. Se nel frattempo Polyfin si riavvia, l’importazione si interrompe: ciò che è stato importato resta, e importando di nuovo non viene aggiunto nulla due volte.',
      stop: 'Interrompi importazione',
      stopping: 'Interruzione…',
      states: {
        running: 'In corso',
        done: 'Completata',
        stopped: 'Interrotta',
        failed: 'Non riuscita',
      },
      problems: {
        jellyfin_unreachable: (server: string) =>
          `${server} non è stato raggiungibile durante l’importazione. Ciò che è stato importato viene mantenuto.`,
        jellyfin_key_refused: (server: string) =>
          `${server} ha rifiutato la chiave API durante l’importazione: potrebbe essere stata revocata. Ciò che è stato importato viene mantenuto.`,
        not_jellyfin: (server: string) =>
          `Durante l’importazione il server ha smesso di rispondere come ${server}. Ciò che è stato importato viene mantenuto.`,
        internal: () =>
          'Polyfin ha riscontrato un errore durante l’importazione: il log del server fornisce più dettagli. Ciò che è stato importato viene mantenuto.',
        jellyfin_user_forbidden: () =>
          'Il server non consente a questa chiave di leggere i dati di questo utente. Importalo con una chiave API del pannello di controllo del server o con la chiave dell’utente stesso.',
      },
      errors: {
        emby: {
          invalid_jellyfin_address:
            'Inserisci l’indirizzo del server Emby, ad esempio http://192.168.1.10:8096.',
          jellyfin_key_refused:
            'Emby ha rifiutato questa chiave API. Creane una nelle sue impostazioni, in API Keys, e incollala di nuovo.',
          jellyfin_sign_in_refused: 'Emby ha rifiutato questo nome o questa password.',
          jellyfin_sign_in_forbidden:
            'Emby non consente l’accesso a questo account: potrebbe essere disabilitato o fuori dagli orari consentiti.',
          jellyfin_password_refused:
            'Emby ha rifiutato questa password. Lasciala vuota se l’account non ne ha una.',
          jellyfin_unreachable:
            'Nessuna risposta a questo indirizzo. Verificalo e controlla che Emby sia in esecuzione.',
          not_jellyfin:
            'Un server ha risposto a questo indirizzo, ma non è Emby. Verifica l’indirizzo.',
        },
        plex: {
          invalid_jellyfin_address:
            'Inserisci l’indirizzo di Plex Media Server, ad esempio http://192.168.1.10:32400.',
          jellyfin_key_refused:
            'Plex ha rifiutato questo token. Copia di nuovo il token del proprietario.',
          jellyfin_key_limited:
            'Questo token non può elencare gli account del server: non è quello del proprietario del server. Usa il token del proprietario.',
          jellyfin_unreachable:
            'Nessuna risposta a questo indirizzo. Verificalo e controlla che Plex Media Server sia in esecuzione.',
          not_jellyfin:
            'Un server ha risposto a questo indirizzo, ma non è Plex Media Server. Verifica l’indirizzo.',
        },
      } as Record<'emby' | 'plex', Partial<Record<string, string>>>,
      userStates: {
        waiting: 'In attesa',
        reading: 'Lettura',
        saving: 'Salvataggio',
        done: 'Completato',
        failed: 'Non riuscito',
      },
      notImported: 'Non importato',
      userMapping: (jellyfinName: string, userName: string) => `${jellyfinName} → ${userName}`,
      read: (count: number) => (count === 1 ? '1 elemento letto' : `${count} elementi letti`),
      counts: (played: number, resumed: number, favorites: number) =>
        `${played} segnati come visti, ${resumed === 1 ? '1 punto di ripresa' : `${resumed} punti di ripresa`}, ${favorites === 1 ? '1 preferito' : `${favorites} preferiti`}`,
      upToDate: (count: number) => `${count} già in Polyfin`,
      ratings: (count: number) => (count === 1 ? '1 valutato' : `${count} valutati`),
      watchlist: (count: number) =>
        count === 1 ? '1 aggiunto a «Da guardare»' : `${count} aggiunti a «Da guardare»`,
      undated: (count: number) =>
        count === 1
          ? '1 punto di ripresa senza data, non aggiunto'
          : `${count} punti di ripresa senza data, non aggiunti`,
      nothingAdded: {
        up_to_date: () =>
          'Tutto ciò che è stato letto era già in Polyfin: non c’era nulla da aggiungere.',
        nothing_played: (server: string) =>
          `${server} non elenca nulla di visto, in corso o preferito per questo utente.`,
        no_sections: () =>
          'Questo server Plex non ha librerie di film o serie TV: non c’era nulla da leggere.',
        plex_owner_nothing_watched: () =>
          'Il proprietario non ha nulla di visto o in corso nelle librerie di film e serie TV di questo server.',
        plex_nothing_watched: () =>
          'Questo account non ha nulla di visto, in corso o valutato nelle librerie di film e serie TV di questo server.',
        plex_no_history: () =>
          'Plex non elenca alcuna riproduzione di questo account su questo server. La sua cronologia di riproduzione contiene solo i titoli riprodotti fino alla fine su questo server: i titoli segnati come visti senza riprodurli, o sincronizzati dall’account Plex, non vi compaiono. Per portare tutto, punti di ripresa compresi, la persona accede con Plex da Il mio account › Importa da un altro server.',
      },
      unmatched: (count: number) =>
        count === 1 ? '1 titolo non trovato' : `${count} titoli non trovati`,
      unmatchedTitle: (name: string) => `Titoli non trovati per ${name}`,
      unmatchedHelp: 'Questi titoli non sono stati importati: il motivo spiega perché.',
      titleColumn: 'Titolo',
      kindColumn: 'Tipo',
      yearColumn: 'Anno',
      reasonColumn: 'Motivo',
      kinds: {
        movie: 'Film',
        episode: 'Episodio',
        series: 'Serie TV',
      },
      reasons: {
        no_identifier: 'Nessun identificativo IMDb, TMDB o TVDB',
        not_found: 'Non presente in Polyfin',
        not_in_library: 'Non più nelle librerie di Plex',
        other_episode_order: 'Numerato secondo un altro ordine degli episodi di TheTVDB',
      },
      more: (count: number) => (count === 1 ? 'E 1 altro.' : `E altri ${count}.`),
    },
    appImport: {
      open: 'Importa da un’applicazione',
      title: 'Importa da un’applicazione',
      description:
        'Trasferisci i dati di visione di un utente dal suo account su un’applicazione basata su add-on: i film e gli episodi riprodotti con le relative date, i punti di ripresa e la libreria dell’applicazione come preferiti, abbinati tramite IMDb, TMDB o Kitsu. I dati di Polyfin vengono solo integrati e nulla viene scritto nell’applicazione. Ti servono le credenziali di accesso dell’utente all’applicazione; gli utenti possono anche importare da soli, in Il mio account.',
      user: 'Nell’account di',
      userHelp: 'L’utente Polyfin ai cui dati di visione e add-on si aggiunge l’importazione.',
    },
    letterboxdImport: {
      open: 'Importa da Letterboxd',
      title: 'Importa da Letterboxd',
      description:
        'Trasferisci i film di un utente dal suo file di esportazione di Letterboxd: i film visti con le relative date e il numero di riproduzioni, la sua watchlist, i suoi voti e, se lo scegli, i film a cui ha messo «mi piace» come preferiti. I dati di Polyfin vengono solo integrati. Gli utenti possono anche importare i propri, in Il mio account.',
      user: 'Nell’account di',
      userHelp: 'L’utente Polyfin ai cui dati di visione si aggiunge l’importazione.',
    },
  },
  invites: {
    title: 'Link di invito',
    create: 'Crea un link di invito',
    empty: 'Ancora nessun link di invito.',
    emptyHelp:
      'Un link di invito permette alle persone a cui lo invii di creare il proprio account, con le impostazioni che scegli.',
    maxUses: 'Account che può creare',
    maxUsesHelp: 'Da 1 a 100. 1 per impostazione predefinita.',
    expires: 'Scadenza',
    after: (days: number) => (days === 1 ? 'Dopo 1 giorno' : `Dopo ${days} giorni`),
    never: 'Mai',
    accountsEnd: 'Fine degli account',
    accountsEndHelp:
      'Ogni account creato dal link termina questo numero di giorni dopo la sua creazione.',
    accountsEndAfter: (days: number) =>
      days === 1
        ? 'Gli account terminano dopo 1 giorno'
        : `Gli account terminano dopo ${days} giorni`,
    model: 'Impostazioni di',
    modelHelp:
      'I nuovi account copiano i permessi, le librerie visibili, i generi bloccati, gli orari consentiti, il controllo parentale, i limiti di riproduzione e di accesso e il gruppo di qualità di questo utente. Non sono mai amministratori.',
    newUser: 'Un nuovo utente, come con Crea un utente',
    submit: 'Crea link',
    creating: 'Creazione…',
    createdTitle: 'Link di invito creato',
    createdNotice: 'Copia subito questo link: viene mostrato solo questa volta.',
    copy: 'Copia link',
    copied: 'Link copiato.',
    copyFailed: 'Impossibile copiare il link. Selezionalo e copialo manualmente.',
    done: 'Fine',
    uses: (uses: number, maxUses: number) =>
      maxUses === 1
        ? `${uses} account creato su 1`
        : `${uses} ${uses === 1 ? 'account creato' : 'account creati'} su ${maxUses}`,
    settingsOf: (name: string) => `Impostazioni di ${name}`,
    newUserSettings: 'Impostazioni di un nuovo utente',
    expiresLabel: 'Scade',
    expiredLabel: 'Scaduto',
    neverExpires: 'Non scade mai',
    createdBy: (name: string) => `Creato da ${name}`,
    createdByDeleted: 'Creato da un utente eliminato',
    states: {
      active: 'Attivo',
      used_up: 'Esaurito',
      expired: 'Scaduto',
      revoked: 'Revocato',
    },
    revoke: 'Revoca',
    revokeTitle: 'Revocare questo link di invito?',
    revokeConfirm:
      'Il link smette subito di funzionare. Gli account già creati tramite il link vengono mantenuti.',
    revoking: 'Revoca in corso…',
    revoked: 'Il link di invito è stato revocato.',
  },
  groups: {
    tabsLabel: 'Utenti e gruppi',
    title: 'Gruppi',
    description:
      'Un gruppo raccoglie i limiti della pagina di un utente. I suoi membri lo seguono: una modifica al gruppo li raggiunge tutti subito.',
    listTitle: 'Gruppi',
    empty: 'Ancora nessun gruppo.',
    emptyHelp:
      'Crea un gruppo per impostare in una volta sola permessi, limiti di riproduzione, controllo parentale, librerie, generi e orari di più account.',
    create: 'Crea un gruppo',
    name: 'Nome',
    nameTaken: 'Questo nome è già usato da un altro gruppo.',
    nameRule: 'Da 1 a 64 caratteri, diverso dai nomi degli altri gruppi.',
    copyFrom: 'Impostazioni di',
    copyFromHelp:
      'Il gruppo parte con i permessi, i limiti di riproduzione e di accesso, il gruppo di qualità, il controllo parentale, le librerie visibili, i generi bloccati e gli orari consentiti di questo utente.',
    submit: 'Crea gruppo',
    creating: 'Creazione…',
    created: (name: string) => `Il gruppo ${name} è stato creato.`,
    memberCount: (count: number) =>
      count === 0 ? 'Nessun membro' : count === 1 ? '1 membro' : `${count} membri`,
    notFound: 'Gruppo non trovato',
    notFoundHelp: 'Potrebbe essere stato eliminato.',
    backToGroups: 'Torna ai gruppi',
    sectionsLabel: 'Sezioni del gruppo',
    pageHelp:
      'Ogni sezione si salva separatamente e raggiunge subito i membri che seguono il gruppo.',
    rename: 'Rinomina',
    renamed: 'Il gruppo è stato rinominato.',
    updated: 'Il gruppo è stato aggiornato.',
    accessHelp:
      'I permessi che i membri ricevono dal gruppo. Il loro ruolo, la schermata di accesso, «Disabilitato» e la data di fine restano nella loro pagina.',
    delete: 'Elimina gruppo',
    deleteTitle: (name: string) => `Eliminare il gruppo ${name}?`,
    deleteConfirm:
      'I suoi membri mantengono i valori del gruppo come impostazioni proprie, che potranno poi essere modificate nelle loro pagine.',
    deleting: 'Eliminazione…',
    deleted: (name: string) => `Il gruppo ${name} è stato eliminato.`,
    membersTitle: 'Membri',
    membersHelp:
      'Un membro che non segue più il gruppo ne mantiene i valori come impostazioni proprie, contrassegnate da «Impostazioni proprie».',
    noMembers: 'Ancora nessun membro.',
    noMembersHelp:
      'Aggiungi i membri qui, oppure scegli il gruppo nella sezione Accesso della pagina di un utente.',
    ownSettings: 'Impostazioni proprie',
    followsHelp: 'Segue il gruppo',
    ownSettingsHelp: 'Mantiene le proprie impostazioni',
    addMembers: 'Aggiungi membri',
    addMembersTitle: (name: string) => `Aggiungi membri a ${name}`,
    addMembersHelp:
      'Gli utenti scelti seguono il gruppo: ne adottano i valori al posto dei propri. Gli amministratori non possono entrare in un gruppo.',
    noCandidates: 'Nessuno da aggiungere.',
    noCandidatesHelp:
      'Tutti gli utenti che non sono amministratori fanno già parte di questo gruppo.',
    movesFrom: (name: string) => `Lascia il gruppo ${name}`,
    add: 'Aggiungi',
    adding: 'Aggiunta…',
    membersAdded: (count: number) =>
      count === 1 ? '1 membro è stato aggiunto.' : `${count} membri sono stati aggiunti.`,
    group: 'Gruppo',
    noGroup: 'Nessun gruppo',
    groupHelp:
      'Un membro segue il gruppo: si applicano i suoi valori e questa pagina li mostra in sola lettura. Lasciarlo li mantiene come impostazioni proprie dell’utente.',
    administratorNoGroup:
      'Gli amministratori non possono entrare in un gruppo, perché i loro permessi sono diversi.',
    administratorLeaves: (name: string) =>
      `Rendere questo utente amministratore lo rimuove dal gruppo ${name}.`,
    newAccountGroupHelp: 'Gli account creati entrano in questo gruppo e lo seguono.',
    newUserGroupHelp: 'L’utente entra in questo gruppo e lo segue.',
    followGroup: 'Segui il gruppo',
    followGroupHelp:
      'Se disattivato, l’utente mantiene i valori attuali del gruppo come propri e questa pagina può modificarli.',
    setByGroup: (name: string) => `Impostato dal gruppo ${name}.`,
    openGroup: 'Apri il gruppo',
    joinTitle: 'Entrare nel gruppo?',
    joinConfirm: (user: string, group: string) =>
      `${user} entra in ${group} e lo segue: i valori del gruppo sostituiscono i suoi in Accesso, Riproduzione e accesso, Controllo parentale, Librerie visibili, Generi bloccati e Orari consentiti.`,
    join: 'Entra',
    followAgainTitle: 'Seguire di nuovo il gruppo?',
    followAgainConfirm: (user: string, group: string) =>
      `${user} riprende i valori di ${group}: le sue impostazioni proprie nelle sezioni del gruppo vengono sostituite.`,
    followAgain: 'Segui il gruppo',
    groupBadge: (name: string) => `Gruppo ${name}`,
    groupOwnBadge: (name: string) => `Gruppo ${name}, impostazioni proprie`,
    filter: 'Mostra gli utenti di',
    allGroups: 'Tutti i gruppi',
    noneShown: 'Nessun utente qui.',
  },
  resetDefaults: {
    button: 'Ripristina i valori predefiniti',
    title: (name: string) => `Ripristinare le impostazioni di ${name}?`,
    groupTitle: (name: string) => `Ripristinare le impostazioni del gruppo ${name}?`,
    userHelp: (name: string) =>
      `I permessi di Accesso, Riproduzione e accesso con il Gruppo di qualità, Controllo parentale, Librerie visibili, Generi bloccati e Orari consentiti tornano alle impostazioni di un nuovo utente, quelle date da «Crea un utente»; le app di ${name} già collegate le ricevono subito. Nome e password, Amministratore, Mostra nella schermata di accesso, Disabilitato, Scade il, il gruppo, i propri addon, le connessioni, gli account di tracciamento e tutto ciò che ha guardato restano.`,
    ownSettingsHelp: (group: string) =>
      `Resta nel gruppo ${group} con le proprie impostazioni, senza seguirlo.`,
    groupHelp:
      'I permessi di Accesso, Riproduzione e accesso con il Gruppo di qualità, Controllo parentale, Librerie visibili, Generi bloccati e Orari consentiti tornano alle impostazioni di un nuovo utente. Il nome e i membri del gruppo restano.',
    followers: (count: number) =>
      count === 0
        ? 'Nessun membro segue il gruppo: la modifica non raggiunge nessuno.'
        : count === 1
          ? '1 membro segue il gruppo e riceve subito queste impostazioni.'
          : `${count} membri seguono il gruppo e ricevono subito queste impostazioni.`,
    changesTitle: 'Cosa cambia',
    loading: 'Ricerca di cosa cambia…',
    noChange: 'Non cambia nulla: sono già le impostazioni di un nuovo utente.',
    confirm: 'Ripristina',
    resetting: 'Ripristino…',
    done: (name: string) => `Le impostazioni di ${name} sono state ripristinate.`,
    groupDone: (name: string) => `Le impostazioni del gruppo ${name} sono state ripristinate.`,
  },
}

export default users
