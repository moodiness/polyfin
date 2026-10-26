import type en from '../en/account'

const account: typeof en = {
  account: {
    title: 'Il mio account',
    signedInAs: 'Accesso effettuato come ',
    sectionsLabel: 'Sezioni dell’account',
    sections: {
      statistics: 'Statistiche',
      tracking: 'Monitoraggio',
      resume: 'Continua a guardare',
      hidden: 'Titoli nascosti',
      homeRows: 'Righe della Home',
      metadataLanguage: 'Lingua dei metadati',
      sharedLinks: 'Link condivisi',
      reminders: 'Promemoria',
      import: 'Importa da un altro server',
      appImport: 'Importa da un’applicazione',
      notifications: 'Notifiche',
      devices: 'Dispositivi',
      password: 'Password',
      twoStep: 'Accesso in due passaggi',
    },
    metadataLanguage: {
      help: 'La lingua di nomi, trame e immagini dei titoli nelle tue applicazioni, quando uno degli add-on del server li descrive in quella lingua; i titoli che nessun add-on descrive in quella lingua mantengono la descrizione del server. Generi, classificazioni e persone restano uguali per tutti. I nomi che Polyfin crea da sé, come le stagioni, la seguono quando Polyfin la parla: inglese, francese, tedesco, spagnolo, italiano o portoghese.',
      label: 'Lingua dei metadati',
      server: 'Quella del server',
      noLanguage:
        'Nessun add-on del server ha ancora una lingua, quindi i titoli sono descritti in quella del server. Un amministratore assegna una lingua agli add-on in Contenuti › Origini.',
      saved: 'Lingua dei metadati salvata.',
    },
    homeRows: {
      help: 'Le righe che l’amministratore aggiunge per te alla pagina Home del player web. Disattivane una per escluderla dalla tua pagina Home e, quando lo è anche lì, dalle tue librerie nelle altre applicazioni. Le sezioni proprie del player web restano nelle sue Impostazioni › Home.',
      empty: 'Nessuna riga della Home',
      emptyHelp:
        'L’amministratore non ha aggiunto alcuna riga per te alla pagina Home del player web.',
      shown: (name: string) => `Mostra ${name} nella tua pagina Home`,
      on: (name: string) => `${name} è di nuovo nella tua pagina Home.`,
      off: (name: string) => `${name} è stata rimossa dalla tua pagina Home.`,
    },
    watching: {
      resumeHelp:
        'I titoli in corso nel tuo «Continua a guardare», per le applicazioni che non permettono di rimuoverne uno. Rimuovi toglie un titolo senza cambiarne lo stato di visto. Un’importazione da un servizio di tracciamento o da un’applicazione lo riporta solo con un punto di ripresa più recente.',
      hiddenHelp:
        'I titoli che hai nascosto da librerie, righe della Home, ricerche e titoli simili. Si aprono comunque da un link. Nel player web, il menu ⋮ di un titolo lo nasconde.',
      remove: 'Rimuovi',
      removeLabel: (name: string) => `Rimuovi ${name} da «Continua a guardare»`,
      removed: (name: string) => `${name} è stato rimosso da «Continua a guardare».`,
      resumeEmpty: 'Niente in corso',
      resumeEmptyHelp: 'I film e gli episodi che inizi compaiono qui finché non li finisci.',
      progress: (position: string, runtime: string) => `${position} di ${runtime}`,
      showAgain: 'Mostra di nuovo',
      showLabel: (name: string) => `Mostra di nuovo ${name}`,
      shown: (name: string) => `${name} è di nuovo nelle tue liste.`,
      hiddenEmpty: 'Nessun titolo nascosto',
      hiddenEmptyHelp: 'Nascondi un titolo dal suo menu ⋮ nel player web.',
      hiddenOn: 'Nascosto',
      kinds: {
        movie: 'Film',
        series: 'Serie TV',
        episode: 'Episodio',
        video: 'Video personale',
        replay: 'Replay',
      } as Record<string, string>,
    },
    reminders: {
      help: 'I programmi della Diretta TV per cui hai chiesto un promemoria dalla loro pagina nel player web. Ogni promemoria arriva sulle tue destinazioni che hanno scelto «Inizio di un programma», con l’anticipo impostato in Notifiche.',
      empty: 'Nessun promemoria',
      emptyHelp:
        'Nella pagina di un programma in arrivo nel player web, «Ricordamelo» ne imposta uno.',
      everyAiring: (time: string) => `Ogni messa in onda verso le ${time}`,
      next: 'Prossima ',
      noAiring: 'Nessuna messa in onda nella guida',
      delete: 'Elimina',
      deleteLabel: (name: string) => `Elimina il promemoria di ${name}`,
      deleted: (name: string) => `Promemoria di ${name} eliminato.`,
      settingsTitle: 'Promemoria',
      before: 'Avvisami prima',
      beforeHelp: 'Quanto tempo prima dell’inizio di un programma arriva il suo promemoria.',
      minutes: (minutes: number) =>
        minutes === 1 ? '1 minuto' : minutes === 60 ? '1 ora' : `${minutes} minuti`,
      onApps: 'Mostra i promemoria anche nelle mie applicazioni',
      onAppsHelp:
        'Ogni promemoria compare anche nelle tue applicazioni Jellyfin che accettano messaggi, come il player web mentre è aperto, come fa «Invia un messaggio».',
      noTarget:
        'Nessuna delle tue destinazioni ha scelto «Inizio di un programma» e le tue applicazioni sono disattivate: i tuoi promemoria non ti raggiungerebbero da nessuna parte.',
      saved: 'Impostazioni dei promemoria salvate.',
    },
    importEvery: (minutes: number) =>
      minutes < 60
        ? `ogni ${minutes} minuti`
        : minutes === 60
          ? 'ogni ora'
          : `ogni ${minutes / 60} ore`,
    serverImport: {
      description:
        'Porta qui i film e gli episodi che hai visto, i punti di ripresa e i preferiti da un server Jellyfin o Emby che usavi prima, oppure i tuoi film dall’esportazione di Letterboxd. Vanno solo nel tuo account e si aggiungono soltanto a ciò che Polyfin ha già. Sull’altro server non viene scritto nulla.',
      serverKind: 'Server',
      address: 'Indirizzo del server',
      addressHelp:
        'L’indirizzo a cui il server si apre in un browser, ad esempio http://192.168.1.10:8096.',
      name: 'Il tuo nome utente su quel server',
      password: 'La tua password su quel server',
      passwordHelp:
        'Usata solo per questa importazione e mai conservata. Polyfin accede con il tuo account, come fanno le applicazioni del server, poi esce. Lascia vuoto se il tuo account non ha una password.',
      start: 'Importa la mia cronologia di visione',
      starting: 'Avvio…',
      started: 'L’importazione è iniziata. Puoi lasciare questa pagina: prosegue comunque.',
      lastTitle: 'La tua ultima importazione',
      from: (server: string) => `Da ${server}`,
    },
    letterboxd: {
      option: 'Letterboxd (file di esportazione)',
      help: 'In Letterboxd, apri Settings › Import & Export e scegli Export your data: Letterboxd fornisce un file ZIP. Sceglilo qui così com’è. Polyfin lo legge in memoria e non lo conserva mai.',
      watchedHelp:
        'I film che hai visto vengono segnati come visti, alla data più recente indicata dal tuo diario, tante volte quante vi sono registrati. Ogni film viene cercato per titolo e anno negli add-on dei metadati del server, e viene tenuto quando questi conoscono esattamente un film con quel titolo e quell’anno.',
      noSearch:
        'Nessun add-on dei metadati di questo server cerca film: nessun film dell’esportazione può essere trovato. Chiedi a un amministratore di aggiungerne uno.',
      file: 'File di esportazione',
      fileHelp: 'Il file ZIP fornito da Letterboxd, di massimo 20 MB.',
      choose: 'Scegli il file',
      chooseAnother: 'Scegli un altro file',
      tooLarge: 'Questo file supera i 20 MB: scegli il file ZIP fornito da Letterboxd.',
      watchlist: 'Watchlist nella mia lista «Da guardare»',
      watchlistHelp:
        'I film della tua watchlist di Letterboxd vanno nella tua lista «Da guardare» nelle applicazioni Jellyfin, finché non li avvii.',
      ratings: 'Voti',
      ratingsHelp:
        'Le tue stelle diventano il tuo voto su 10: 3,5 stelle corrispondono a 7. Un voto che hai già in Polyfin resta invariato.',
      likes: 'Film con «Mi piace» come preferiti',
      likesHelp: 'I film a cui hai messo «Mi piace» su Letterboxd diventano i tuoi preferiti.',
      forUser: {
        watchedHelp:
          'I film che ha visto vengono segnati come visti, alla data più recente indicata dal suo diario, tante volte quante vi sono registrati. Ogni film viene cercato per titolo e anno negli add-on dei metadati del server, e viene tenuto quando questi conoscono esattamente un film con quel titolo e quell’anno.',
        watchlist: 'Watchlist nella sua lista «Da guardare»',
        watchlistHelp:
          'I film della sua watchlist di Letterboxd vanno nella sua lista «Da guardare» nelle applicazioni Jellyfin, finché non li avvia.',
        ratingsHelp:
          'Le sue stelle diventano il suo voto su 10: 3,5 stelle corrispondono a 7. Un voto che ha già in Polyfin resta invariato.',
        likesHelp: 'I film a cui ha messo «Mi piace» su Letterboxd diventano i suoi preferiti.',
      },
      start: 'Importa da Letterboxd',
      starting: 'Invio…',
      started: 'L’importazione è iniziata. Puoi lasciare questa pagina: prosegue comunque.',
      currentTitle: 'Importazione in corso',
      lastTitle: 'Ultima importazione',
      ownLastTitle: 'La tua ultima importazione',
      from: 'Da un’esportazione di Letterboxd',
      startedLabel: 'Iniziata',
      endedLabel: 'Terminata',
      startedBy: (name: string) => `Avviata da ${name}`,
      states: { running: 'In corso', done: 'Completata', failed: 'Non riuscita' },
      problem:
        'Polyfin ha riscontrato un errore durante l’importazione: il log del server fornisce più dettagli. Ciò che è stato importato viene conservato.',
      runningHelp:
        'Puoi lasciare questa pagina: l’importazione prosegue. Se nel frattempo Polyfin si riavvia, l’importazione si interrompe: ciò che è stato importato resta, e importare di nuovo non aggiunge nulla due volte.',
      searching: (searched: number, movies: number) =>
        `Ricerca dei film: ${searched} di ${movies}.`,
      searchingLabel: 'Film cercati',
      summary: (movies: number, added: string) => `${movies} film: ${added}.`,
      played: (count: number) =>
        count === 1 ? `${count} segnato come visto` : `${count} segnati come visti`,
      addedToWatchlist: (count: number) =>
        count === 1 ? `${count} aggiunto a «Da guardare»` : `${count} aggiunti a «Da guardare»`,
      rated: (count: number) => (count === 1 ? `${count} votato` : `${count} votati`),
      favorites: (count: number) =>
        count === 1 ? `${count} aggiunto ai preferiti` : `${count} aggiunti ai preferiti`,
      unmatched: (count: number) =>
        count === 1 ? '1 film non trovato' : `${count} film non trovati`,
      unmatchedTitle: 'Film non trovati',
      unmatchedHelp:
        'Questi film non sono stati importati. Ognuno viene cercato per titolo e anno, e viene tenuto solo quando gli add-on dei metadati del server conoscono esattamente un film con quel titolo e quell’anno. Aprili su Letterboxd per verificarli.',
      movieColumn: 'Film',
      reasonColumn: 'Motivo',
      onLetterboxd: 'Su Letterboxd',
      reasons: {
        not_found: 'Non trovato',
        ambiguous: 'Più film con quel titolo e quell’anno',
        search_failed: 'Ricerca non riuscita',
      },
    },
    appImport: {
      description:
        'Porta qui ciò che hai guardato in un’applicazione basata su add-on: i film e gli episodi riprodotti con le relative date, i tuoi punti di ripresa e la libreria dell’applicazione, che finisce nella tua lista «Da guardare» nelle applicazioni Jellyfin, abbinati tramite IMDb, TMDB o Kitsu. Si aggiungono soltanto a ciò che Polyfin ha già, e nulla viene scritto nell’applicazione.',
      app: 'Applicazione',
      apps: { nuvio: 'Nuvio', stremio: 'Stremio' },
      backend: 'Backend Nuvio',
      backendHelp:
        'Lascia vuoto per usare quello di Nuvio. Funziona anche un backend self-hosted, al suo indirizzo, anche sulla rete locale.',
      email: 'Email',
      password: 'Password',
      passwordHelp:
        'Usata solo per leggere l’account, e mai conservata. Polyfin accede come fanno le applicazioni Nuvio, legge l’account e poi esce, a meno che tu non resti connesso.',
      signIn: 'Accedi a Nuvio',
      signingIn: 'Accesso in corso…',
      chooseTitle: 'Scegli cosa importare',
      chooseHelp: {
        nuvio:
          'Nuvio conserva una cronologia per ogni profilo: scegli quello da importare. Polyfin conserva ciò che ha letto solo per pochi minuti.',
        stremio:
          'Stremio conserva una cronologia per ogni profilo, o un’unica cronologia per l’intero account senza profili: scegli quella da importare. Polyfin conserva ciò che ha letto solo per pochi minuti.',
      },
      stayConnected: 'Resta connesso',
      stayConnectedHelp: {
        nuvio: (every: string) =>
          `Polyfin conserva la sessione, cifrata, mai la tua password, e importa di nuovo il profilo che scegli ${every}. Si limita a leggere l’account. Puoi disconnetterti in qualsiasi momento.`,
        stremio: (every: string) =>
          `Polyfin conserva la chiave di autenticazione, cifrata, e per un profilo con PIN la sessione di quel profilo, mai il PIN, e importa di nuovo il profilo che scegli ${every}. Si limita a leggere l’account. Puoi disconnetterti in qualsiasi momento. I termini di Stremio vietano di estrarne i dati con mezzi automatizzati: leggere il tuo account ${every} rende questa zona grigia più marcata rispetto a un’importazione singola.`,
      },
      staysConnected: (every: string) =>
        `Resti connesso: questo profilo viene importato di nuovo ${every}, allo stesso modo. Gli add-on vengono installati solo ora.`,
      wholeAccount: 'L’intero account',
      profile: 'Profilo',
      profileNumber: (id: string) => `Profilo ${id}`,
      profileCounts: (played: number, resumes: number, library: number) =>
        `${played} visti, ${resumes === 1 ? '1 punto di ripresa' : `${resumes} punti di ripresa`}, ${library} nella libreria.`,
      lockedProfile: (name: string) => `${name} (PIN)`,
      pin: 'PIN',
      pinHelp:
        'Questo profilo chiede il suo PIN. Polyfin lo invia solo a Stremio, per aprire il profilo, e non lo conserva mai.',
      unlock: 'Apri questo profilo',
      unlocking: 'Apertura…',
      addons: 'Add-on',
      addonsHelp:
        'Ogni add-on selezionato diventa uno degli add-on propri dell’account, in Le mie origini. Il suo indirizzo può contenere impostazioni e chiavi: Polyfin non lo mostra mai per intero.',
      addonsOff:
        'Gli add-on personali sono disattivati per questo account: gli add-on dell’applicazione non vengono proposti.',
      noAddons: 'Questo profilo non ha add-on.',
      back: 'Accedi di nuovo',
      start: 'Importa questo profilo',
      starting: 'Avvio…',
      started: 'L’importazione è iniziata. Puoi lasciare questa pagina: prosegue comunque.',
      currentTitle: 'Importazione in corso',
      lastTitle: 'Ultima importazione',
      ownLastTitle: 'La tua ultima importazione',
      from: (app: string, profile: string) =>
        profile === '' ? `Da ${app}` : `Da ${app}, profilo ${profile}`,
      startedLabel: 'Avviata',
      endedLabel: 'Terminata',
      startedBy: (name: string) => `Avviata da ${name}`,
      states: { running: 'In corso', done: 'Completata', failed: 'Non riuscita' },
      problem:
        'Polyfin ha riscontrato un errore durante l’importazione: il registro del server fornisce maggiori dettagli. Ciò che è stato importato viene conservato.',
      runningHelp:
        'Puoi lasciare questa pagina: l’importazione prosegue. Se nel frattempo Polyfin si riavvia, l’importazione si interrompe: ciò che è stato importato resta, e importare di nuovo non aggiunge nulla due volte.',
      read: (count: number) => (count === 1 ? '1 titolo letto.' : `${count} titoli letti.`),
      counts: (read: number, played: number, resumed: number, watchlist: number) =>
        `${read === 1 ? '1 titolo letto' : `${read} titoli letti`}: ${played === 1 ? '1 segnato come visto' : `${played} segnati come visti`}, ${resumed === 1 ? '1 punto di ripresa' : `${resumed} punti di ripresa`}, ${watchlist === 1 ? '1 aggiunto' : `${watchlist} aggiunti`} a «Da guardare».`,
      unmatched: (count: number) =>
        count === 1 ? '1 titolo non trovato' : `${count} titoli non trovati`,
      unmatchedTitle: 'Titoli non trovati',
      unmatchedHelp:
        'Questi titoli non sono stati importati: l’applicazione li identifica con un identificatore che Polyfin non riconosce, oppure nessun titolo di Polyfin lo possiede. Per una serie TV, l’applicazione può anche indicare gli episodi visti in base alla loro posizione in un elenco di episodi che gli add-on di Polyfin ordinano diversamente.',
      titleColumn: 'Titolo',
      kindColumn: 'Tipo',
      reasonColumn: 'Motivo',
      reasons: {
        other_identifier: 'Non è un identificatore IMDb, TMDB o Kitsu',
        not_found: 'Non presente in Polyfin',
        episode_list:
          'I suoi episodi non corrispondono all’elenco di Polyfin: nessuno è stato segnato come visto',
      },
      addonInstalled: 'aggiunto',
      addonWaiting: 'in attesa',
      addonProblems: {
        not_allowed: 'non aggiunto: gli add-on personali erano disattivati',
        exists: 'già installato',
        invalid_address: 'non aggiunto: non è l’indirizzo di un add-on',
        private_network:
          'non aggiunto: si trova su una rete locale, raggiungibile solo dall’importazione di un amministratore',
        unreachable: 'non aggiunto: l’add-on non ha risposto',
        invalid_manifest: 'non aggiunto: non è un add-on',
        internal: 'non aggiunto: Polyfin ha riscontrato un errore',
      },
      stremio: {
        way: 'Accedi con',
        ways: {
          link: 'Un codice di collegamento',
          password: 'Email e password',
          key: 'Una chiave di autenticazione',
        },
        linkHelp:
          'Polyfin chiede un codice a Stremio, come fanno le app TV di Stremio. Lo approvi in un browser in cui hai eseguito l’accesso a Stremio: funziona qualunque sia il modo in cui accedi al tuo account, Facebook e Apple inclusi, e Polyfin non vede mai la tua password.',
        getCode: 'Ottieni un codice di collegamento',
        gettingCode: 'Richiesta a Stremio…',
        codeLabel: 'Codice di collegamento',
        approve:
          'Apri il link in un browser in cui hai eseguito l’accesso a Stremio, verifica che mostri questo codice e approvalo.',
        openLink: 'Apri il link',
        waiting:
          'In attesa della tua approvazione. Il codice dura 5 minuti; una volta approvato, Polyfin legge l’account, poi esce, a meno che tu non resti connesso.',
        cancel: 'Annulla',
        email: 'Email',
        password: 'Password',
        passwordHelp:
          'Usata solo per leggere l’account e mai conservata. Polyfin accede come fanno le app di Stremio, legge l’account, poi esce, a meno che tu non resti connesso. Un account creato con Facebook o Apple non ha una password: usa un codice di collegamento.',
        signIn: 'Accedi a Stremio',
        signingIn: 'Accesso in corso…',
        key: 'Chiave di autenticazione',
        keyHelp:
          'La chiave che l’app web di Stremio conserva dopo l’accesso: nella console del browser su quell’app, JSON.parse(localStorage.getItem("profile")).auth.key la restituisce. Usata per leggere l’account e conservata solo se resti connesso. Polyfin non la disconnette mai, perché ciò disconnetterebbe il tuo browser.',
        readKey: 'Leggi l’account',
        reading: 'Lettura…',
      },
      connections: {
        title: 'App connesse',
        help: (every: string) =>
          `Importate ${every}, come alla prima importazione: lo stesso profilo, senza i loro add-on. Polyfin si limita a leggerle.`,
        connectedAs: 'Connesso come ',
        connected: 'Connesso',
        connectedOn: (date: string) => ` dal ${date}`,
        connectedBy: (name: string) => ` da ${name}`,
        status: {
          connected: 'Connesso',
          importing: 'Importazione',
          paused: 'In pausa',
          stopped: 'Accedi di nuovo',
        },
        never: 'Non ancora importato.',
        lastImport: 'Ultima importazione ',
        colon: ': ',
        nextImport: 'Prossima importazione ',
        paused:
          'Il server non consente più agli utenti di importare la cronologia di visione: questa connessione è in pausa finché non lo consentirà di nuovo.',
        stopped: (app: string) =>
          `${app} ha rifiutato l’accesso conservato da Polyfin: non viene importato nulla finché non accedi di nuovo qui sotto, con «Resta connesso» selezionato. La disconnessione lo elimina.`,
        problems: {
          unreachable: (app: string) =>
            `${app} non ha risposto durante l’ultima importazione. La prossima riproverà.`,
          rate_limited: (app: string) =>
            `${app} ha chiesto a Polyfin di attendere durante l’ultima importazione. La prossima riproverà.`,
          unexpected: (app: string) =>
            `${app} ha risposto all’ultima importazione in un modo che questa versione di Polyfin non sa leggere. La prossima riproverà.`,
          profile_missing: (app: string) =>
            `Il profilo importato non è più presente nell’account ${app}. Accedi di nuovo per sceglierne un altro.`,
          profile_locked: (app: string) =>
            `Il profilo importato ora chiede un PIN, o uno diverso, in ${app}. Accedi di nuovo per sceglierlo con il suo PIN.`,
          sign_in_refused: (app: string) => `${app} ha rifiutato l’accesso conservato da Polyfin.`,
          internal: () =>
            'Polyfin ha riscontrato un errore durante l’ultima importazione: il log del server fornisce maggiori dettagli. La prossima riproverà.',
        },
        importNow: 'Importa ora',
        importing: 'Importazione…',
        disconnect: 'Disconnetti',
        disconnectTitle: (app: string) => `Disconnettere ${app}?`,
        disconnectBody: (app: string) =>
          `Polyfin smette di importare questo account ${app} ed elimina ciò che conservava per leggerlo, dopo aver chiuso la sessione che aveva aperto. Una chiave di autenticazione che hai incollato viene solo eliminata. Ciò che è stato importato resta.`,
        disconnected: (app: string) => `${app} disconnesso.`,
      },
    },
    sharedLinksHelp:
      'Condividi un film o un episodio con qualcuno che non ha un account: lo guarda nel suo browser tramite un link che scade, e conta subito tra le tue riproduzioni.',
    statisticsHelp:
      'Cosa hai guardato su questo server: ore, titoli, applicazioni e dispositivi, e le tue riproduzioni recenti.',
    notificationsHelp:
      'Dove Polyfin ti avvisa dei nuovi episodi delle serie TV che segui, delle tue registrazioni e riproduzioni, delle tue connessioni che si interrompono, dei blocchi del tuo account e dei suoi accessi da nuovi dispositivi, ti ricorda i programmi che hai scelto e ti invia il tuo riepilogo settimanale, il mese in sintesi e l’anno in sintesi.',
    devicesHelp:
      'Le applicazioni che hanno effettuato l’accesso con il tuo account. Un dispositivo che disconnetti ti chiederà la password alla prossima apertura.',
    passwordHelp: 'Cambiare la password disconnette tutti i tuoi dispositivi Jellyfin.',
    currentPassword: 'Password attuale',
    newPassword: 'Nuova password',
    confirmPassword: 'Conferma la nuova password',
    changePassword: 'Cambia password',
    changingPassword: 'Modifica in corso…',
    passwordChanged: 'La tua password è stata modificata.',
    twoStep: {
      help: 'Dopo la password, questa applicazione di amministrazione chiede un codice da un’applicazione di autenticazione sul tuo telefono. Le applicazioni Jellyfin non possono chiederlo: una volta attivato, accedono con Quick Connect, che approvi qui, e la sola password non basta più per accedervi. Le applicazioni e i browser che hanno già effettuato l’accesso restano connessi.',
      off: 'Disattivato',
      on: 'Attivo',
      offText: 'Accedi con la sola password.',
      onText: 'Accedi con la password, poi con un codice dalla tua applicazione di autenticazione.',
      required:
        'Gli amministratori devono usare l’accesso in due passaggi su questo server. Attivalo ora, altrimenti ti verrà richiesto al prossimo accesso.',
      requiredOn:
        'Gli amministratori devono usare l’accesso in due passaggi su questo server: resta attivo.',
      unreadable:
        'POLYFIN_SECRET_KEY non riesce più a leggere la chiave della tua applicazione di autenticazione, come dopo un ripristino con un’altra chiave: puoi accedere solo con i codici di recupero. Disattiva l’accesso in due passaggi con la tua password, poi riattivalo.',
      turnOn: 'Attiva',
      starting: 'Preparazione…',
      scan: 'Scansiona questo codice QR con la tua applicazione di autenticazione, oppure inserisci la chiave a mano. Polyfin ha generato il codice da sé: la chiave non va da nessun’altra parte.',
      qrLabel: 'Codice QR della chiave, per la tua applicazione di autenticazione',
      key: 'Chiave',
      openInApp: 'Apri in un’applicazione di autenticazione su questo dispositivo',
      code: 'Codice dall’applicazione',
      codeHelp:
        'Il codice a 6 cifre che l’applicazione mostra per Polyfin. Cambia ogni 30 secondi.',
      confirm: 'Attiva',
      confirming: 'Verifica…',
      turnedOn: 'Accesso in due passaggi attivato.',
      recoveryTitle: 'I tuoi codici di recupero',
      recoveryHelp:
        'Ogni codice ti permette di accedere una volta, al posto di un codice dall’applicazione, nel caso perdessi il telefono. Conservali in un luogo sicuro, come un gestore di password: vengono mostrati solo ora.',
      copy: 'Copia i codici',
      copied: 'Codici copiati.',
      copyFailed: 'Impossibile copiare i codici: selezionali e copiali a mano.',
      saved: 'Li ho salvati',
      codesLeft: (n: number) =>
        n === 1 ? '1 codice di recupero rimasto' : `${n} codici di recupero rimasti`,
      newCodes: 'Nuovi codici di recupero',
      newCodesTitle: 'Generare nuovi codici di recupero?',
      newCodesText: 'I nuovi codici annullano quelli precedenti.',
      making: 'Generazione…',
      turnOff: 'Disattiva',
      turnOffTitle: 'Disattivare l’accesso in due passaggi?',
      turnOffText: 'Accederai quindi con la sola password, qui e nelle applicazioni Jellyfin.',
      turningOff: 'Disattivazione…',
      turnedOff: 'Accesso in due passaggi disattivato.',
      confirmWith: 'Conferma con',
      withCode: 'Un codice',
      withPassword: 'La mia password',
      codeOrRecovery: 'Codice dall’applicazione o codice di recupero',
      password: 'Password',
    },
    tracking: {
      title: 'Monitoraggio',
      description:
        'Collega i servizi che tengono traccia di ciò che guardi e ascolti. Polyfin comunica a ogni servizio collegato i film e gli episodi che guardi, o i brani che riproduci, nelle tue applicazioni Jellyfin.',
      loading: 'Caricamento dei servizi di monitoraggio…',
      status: {
        connected: 'Collegato',
        notConnected: 'Non collegato',
        waiting: 'In attesa del codice',
        waitingSignIn: 'In attesa di te',
        reconnect: 'Da ricollegare',
        unreachable: 'Nuovo tentativo in corso',
        unavailable: 'Non configurato',
        appRefused: 'Applicazione rifiutata',
      },
      unavailable: (name: string) =>
        `${name} non è ancora configurato su questo server: un amministratore deve prima aggiungere l’applicazione ${name} nelle impostazioni.`,
      setUpApp: (name: string) => `Configura l’applicazione ${name}`,
      codeIntro: (name: string) =>
        `Collega il tuo account ${name}: Polyfin ti fornisce un codice da inserire sul sito di ${name}.`,
      keyIntro: (name: string) => `Collega il tuo account ${name} con la tua chiave API ${name}.`,
      signInIntro: (name: string) =>
        `Collega il tuo account ${name}: accedi sul sito di ${name} e consenti a Polyfin di inviare i brani che riproduci.`,
      tokenIntro: (name: string) =>
        `Collega il tuo account ${name} con il tuo token utente ${name}, per inviare i brani che riproduci.`,
      keyLabel: (name: string) => `Chiave API ${name}`,
      tokenLabel: (name: string) => `Token utente ${name}`,
      savedKeyHelp:
        'Solo tu puoi visualizzarla. Per usare un’altra chiave, scollega e poi collega di nuovo.',
      keyHelp: {
        mdblist: 'La trovi su mdblist.com, in Preferences, sotto API key.',
        publicmetadb: 'La trovi nel tuo account PublicMetaDB.',
        listenbrainz: 'Lo trovi su listenbrainz.org, nelle tue impostazioni, sotto User token.',
      },
      connect: 'Collega',
      connecting: 'Collegamento…',
      checking: 'Verifica della chiave…',
      reconnect: 'Collega di nuovo',
      newCode: 'Ottieni un nuovo codice',
      signInAgain: 'Ricomincia',
      enterCode: (site: string) => `Vai su ${site}, accedi e inserisci questo codice:`,
      codeLabel: 'Codice da inserire',
      openSite: (site: string) => `Apri ${site}`,
      waiting: 'Questa pagina si aggiorna da sola dopo che hai inserito il codice.',
      expires: (when: string) => `Il codice scade ${when}.`,
      codeEnded:
        'Il codice è scaduto o è stato rifiutato prima di essere inserito. Ottieni un nuovo codice per riprovare.',
      signInStep: (site: string) =>
        `Vai su ${site}, accedi se richiesto e consenti a Polyfin di usare il tuo account.`,
      signInWaiting: 'Questa pagina si aggiorna da sola dopo che hai dato l’autorizzazione.',
      signInExpires: (when: string) => `Questa richiesta scade ${when}.`,
      signInEnded:
        'Polyfin non è stato autorizzato in tempo, oppure è stato rifiutato. Ricomincia per riprovare.',
      pinIntro: (name: string) =>
        `Collega le tue liste ${name}: autorizza Polyfin sul sito di ${name}, poi incolla qui il codice che ti mostra.`,
      redirectIntro: (name: string) =>
        `Collega la tua lista ${name}: autorizza Polyfin sul sito di ${name}, che ti riporterà qui.`,
      pinStep: (site: string) =>
        `Vai su ${site}, accedi se richiesto, autorizza Polyfin, poi copia qui il codice che ti mostra.`,
      redirectStep: (site: string) =>
        `Vai su ${site}, accedi se richiesto e autorizza Polyfin: al termine verrai riportato qui.`,
      codeField: (name: string) => `Codice ${name}`,
      codeFieldHelp: (name: string) =>
        `Il codice che ${name} mostra dopo che hai autorizzato Polyfin.`,
      addressField: 'Indirizzo o codice',
      addressFieldHelp: (name: string) =>
        `Se il browser non è riuscito ad aprire la pagina a cui ${name} l’ha inviato, copia qui l’indirizzo di quella pagina dalla barra degli indirizzi.`,
      pasteEnded:
        'Questa richiesta è scaduta prima che Polyfin venisse autorizzato. Ricomincia per riprovare.',
      invalidCode: (name: string) =>
        `${name} non ha accettato questo codice, oppure appartiene a una richiesta precedente. Ricomincia per ottenerne uno nuovo.`,
      tokenExpires: (date: string) => `, fino al ${date}`,
      returning: (name: string) => `Collegamento del tuo account ${name}…`,
      connectedAs: 'Collegato come ',
      connectedWithKey: 'Collegato con la tua chiave API',
      connected: 'Collegato',
      connectedOn: (date: string) => ` il ${date}`,
      lastSent: ', ultimo invio ',
      nothingSent: ', ancora nulla inviato',
      connectedToast: (name: string) => `${name} collegato.`,
      disconnectedToast: (name: string) => `${name} scollegato.`,
      problemReconnect: (name: string) =>
        `${name} non accetta più questo collegamento, quindi non gli viene inviato nulla. Collega di nuovo per riprendere a inviare ciò che guardi.`,
      problemUnreachable: (name: string) =>
        `Ultimamente non è stato possibile raggiungere ${name}. Polyfin continua a riprovare e invierà ciò che hai guardato appena risponde.`,
      disconnect: 'Scollega',
      disconnectTitle: (name: string) => `Scollegare ${name}?`,
      disconnectBody: (name: string) =>
        `Polyfin smetterà di comunicargli ciò che guardi. Ciò che ha già ricevuto resta su ${name}.`,
      invalidKey: (name: string) =>
        `${name} non ha accettato questa chiave. Copiala di nuovo dal tuo account ${name}.`,
      invalidToken: (name: string) =>
        `${name} non ha accettato questo token. Copialo di nuovo dalle tue impostazioni di ${name}.`,
      serviceUnreachable: (name: string) =>
        `Impossibile raggiungere ${name}. Riprova tra qualche minuto.`,
      notAvailable: (name: string) =>
        `${name} non è configurato su questo server: un amministratore deve prima aggiungere l’applicazione ${name}.`,
      appRefused: (name: string) =>
        `${name} ha rifiutato l’applicazione di questo server. Un amministratore deve verificarne le credenziali in Impostazioni › Monitoraggio.`,
      history: {
        toggle: (name: string) => `Importa la mia cronologia di ${name}`,
        toggleHelp: (name: string, every: string) =>
          `Segna come visto in Polyfin ciò che hai guardato su ${name} e riprende da dove ti eri fermato, così Continua a guardare e Prossimo corrispondono. Importazione ${every}; nulla viene inviato a ${name}.`,
        listsToggle: (name: string) => `Importa le mie liste di ${name}`,
        listsToggleHelp: (name: string, every: string) =>
          `Segna come visti in Polyfin gli episodi che le tue liste di ${name} contano come visti: fino all’avanzamento di ogni voce per Watching, On hold e Dropped, tutti per Completed. Importazione ${every}; aggiunge soltanto.`,
        listCounts: (played: number, unmapped: number) =>
          `${played === 1 ? '1 titolo segnato come visto' : `${played} titoli segnati come visti`}, ${unmapped === 1 ? '1 titolo non trovato' : `${unmapped} titoli non trovati`} in Polyfin.`,
        importNow: 'Importa ora',
        importing: 'Importazione…',
        importingStatus: 'Importazione della tua cronologia…',
        never: 'Non ancora importato.',
        lastImport: 'Ultima importazione',
        colon: ': ',
        nextImport: 'Prossima importazione',
        counts: (played: number, resumed: number, unmapped: number) =>
          `${played === 1 ? '1 titolo segnato come visto' : `${played} titoli segnati come visti`}, ${resumed === 1 ? '1 punto di ripresa' : `${resumed} punti di ripresa`}, ${unmapped === 1 ? '1 titolo non trovato' : `${unmapped} titoli non trovati`} in Polyfin.`,
        problem: {
          reconnect: (name: string) =>
            `${name} ha rifiutato la connessione durante l’ultima importazione. Connettiti di nuovo per importare la tua cronologia.`,
          unreachable: (name: string) =>
            `Non è stato possibile leggere ${name} per intero durante l’ultima importazione. Ciò che è stato letto è stato importato; la prossima importazione riproverà.`,
          rate_limited: (name: string) =>
            `${name} ha chiesto a Polyfin di attendere durante l’ultima importazione. Ciò che è stato letto è stato importato; la prossima importazione riproverà.`,
        },
        watchlist: {
          lists: {
            trakt: 'watchlist',
            simkl: 'lista Plan to Watch',
            anilist: 'lista Plan to Watch',
            myanimelist: 'lista Plan to Watch',
          },
          toggle: (name: string, list: string) => `Importa la mia ${list} di ${name}`,
          toggleHelp: (name: string, list: string) =>
            `Inserisce i film e le serie TV della tua ${list} di ${name} nella vista «Da guardare» delle tue applicazioni Jellyfin, a ogni importazione. Un titolo ne esce non appena lo inizi o quando ${name} non lo elenca più. Nulla viene inviato a ${name}.`,
          counts: (added: number, unmapped: number) =>
            `${added === 1 ? '1 aggiunto' : `${added} aggiunti`} a «Da guardare», ${unmapped === 1 ? '1 titolo non trovato' : `${unmapped} titoli non trovati`} in Polyfin.`,
          added: (added: number) =>
            `${added === 1 ? '1 aggiunto' : `${added} aggiunti`} a «Da guardare».`,
        },
        ratings: {
          toggle: 'Sincronizza i miei voti',
          toggleHelp: (name: string) =>
            `Invia a ${name} i voti che dai a film e serie TV come tuoi punteggi e, a ogni importazione, assegna ai tuoi titoli senza voto i tuoi punteggi di ${name}. Una serie TV che ${name} suddivide in più voci viene saltata.`,
          counts: (rated: number, skipped: number) =>
            `${rated === 1 ? '1 titolo votato' : `${rated} titoli votati`}, ${skipped === 1 ? '1 voto saltato' : `${skipped} voti saltati`}.`,
        },
      },
    },
  },
  shareLinks: {
    share: 'Condividi un titolo',
    off: 'Al momento non puoi creare link di condivisione: un amministratore li ha disattivati per questo server o per il tuo account. Nel frattempo, i link ancora attivi non vengono riprodotti.',
    empty: 'Nessun link condiviso',
    emptyHelp:
      '«Condividi un titolo» rende disponibile un film o un episodio tramite un link. Nel lettore web, anche la pagina di un film o di un episodio offre «Condividi».',
    userEmpty: 'Questo utente non ha alcun link condiviso.',
    expiresLabel: 'Scade',
    expiredLabel: 'Scaduto',
    revokedLabel: 'Revocato',
    plays: (count: number) => (count === 1 ? '1 riproduzione' : `${count} riproduzioni`),
    viewers: (count: number) =>
      count === 1 ? '1 spettatore alla volta' : `${count} spettatori alla volta`,
    states: { active: 'Attivo', expired: 'Scaduto', revoked: 'Revocato' },
    revoke: 'Revoca',
    revokeLabel: (title: string) => `Revoca il link a ${title}`,
    revokeTitle: 'Revocare questo link condiviso?',
    revokeConfirm:
      'Il link smette subito di funzionare e chi sta guardando tramite il link viene interrotto entro pochi secondi.',
    revoking: 'Revoca in corso…',
    revoked: 'Link condiviso revocato.',
    search: 'Cerca nelle tue librerie',
    searchHelp: 'Un film, oppure una serie TV per sceglierne uno degli episodi.',
    searchButton: 'Cerca',
    searching: 'Ricerca in corso…',
    noTitles: 'Nessun film o serie TV trovato.',
    titlesFound: (term: string) => `Titoli trovati per ${term}`,
    pickTitle: (name: string, details: string) => `Condividi ${name}, ${details}`,
    episodesOf: (series: string) => `Episodi di ${series}`,
    noEpisodes: 'Questa serie TV non ha episodi che puoi condividere.',
    back: 'Torna ai risultati',
    chosen: 'Titolo',
    change: 'Cambia',
    expires: 'Scade dopo',
    expiresHelp: 'Dopo questo periodo nessuno potrà aprire il link. Puoi revocarlo prima.',
    after: (hours: number) =>
      hours === 1 ? '1 ora' : hours === 24 ? '1 giorno' : `${hours / 24} giorni`,
    viewersLabel: 'Spettatori alla volta',
    viewersHelp:
      'Quante persone possono guardare tramite il link contemporaneamente. Ognuna conta come una delle tue riproduzioni simultanee.',
    submit: 'Crea link',
    creating: 'Creazione in corso…',
    createdTitle: 'Link creato',
    createdNotice: (title: string) =>
      `Chiunque abbia questo link può guardare ${title} in un browser, senza account, fino alla scadenza. Copialo ora: viene mostrato solo questa volta.`,
    copy: 'Copia il link',
    copied: 'Link copiato.',
    copyFailed: 'Impossibile copiare il link. Selezionalo e copialo manualmente.',
    done: 'Fatto',
  },
  resetPreferences: {
    button: 'Ripristina le mie preferenze',
    title: 'Ripristinare le tue preferenze?',
    help: 'Ciò che hai scelto tu torna ai valori predefiniti del server:',
    metadataLanguage: 'Lingua dei metadati: Quella del server',
    homeRows: 'Righe della Home: tutte le righe tornano visibili',
    appPreferences:
      'Le preferenze che le app Jellyfin hanno salvato nel tuo account: lingue e modalità di audio e sottotitoli, preferenze della schermata Home e di visualizzazione',
    kept: 'Ciò che hai guardato, i tuoi preferiti, i punti di ripresa, le connessioni e le importazioni restano.',
    confirm: 'Ripristina',
    resetting: 'Ripristino…',
    done: 'Le tue preferenze sono state ripristinate.',
  },
}

export default account
