import type en from '../en/sources'

const sources: typeof en = {
  sources: {
    title: 'Origini',
    lede: 'Add-on Stremio, add-on Eclipse, origini IPTV e cartelle locali in un unico elenco. Le origini del server servono tutti gli utenti; quelle personali di un membro servono solo lui.',
    add: 'Aggiungi un’origine',
    listLabel: 'Origini',
    listNote: 'Le origini personali dei membri servono solo il loro proprietario.',
    loading: 'Caricamento delle origini…',
    kindFilter: 'Filtra per tipo',
    all: 'Tutte',
    kinds: { stremio: 'Stremio', eclipse: 'Eclipse', iptv: 'IPTV', local: 'Cartelle' },
    owner: 'Proprietario',
    everyOwner: 'Tutti i proprietari',
    server: 'Il server',
    search: 'Cerca nelle origini',
    searchPlaceholder: 'Cerca per nome',
    clearFilters: 'Cancella i filtri',
    emptyTitle: 'Ancora nessuna origine',
    emptyShared:
      'Aggiungi un add-on Stremio, un add-on Eclipse, un’origine IPTV o una cartella locale: servirà tutti gli utenti di questo server.',
    emptyMine:
      'Aggiungi un add-on Stremio, un add-on Eclipse o un’origine IPTV: servirà solo te, dopo quelle del server.',
    noMatchTitle: 'Nessuna origine corrispondente',
    noMatch: 'Cambia il tipo, il proprietario o la ricerca.',
    tag: {
      stremio: 'Stremio',
      eclipse: 'Eclipse',
      m3u: 'M3U',
      xtream: 'Xtream',
      hdhomerun: 'HDHomeRun',
      local: 'Cartella',
    },
    kindName: {
      stremio: 'Add-on Stremio',
      eclipse: 'Add-on Eclipse',
      m3u: 'Scaletta M3U',
      xtream: 'Account Xtream Codes',
      hdhomerun: 'Sintonizzatore HDHomeRun',
      local: 'Cartella locale',
    },
    ownedBy: (name: string) => `Origine di ${name}`,
    yours: 'La tua origine',
    active: 'Attiva',
    off: 'Disattivata',
    failed: 'Download non riuscito',
    channelsShown: (count: string) => `${count} canali mostrati`,
    notDownloaded: 'Non ancora scaricata',
    moreActions: (name: string) => `Altre azioni per ${name}`,
    openPage: 'Apri la sua pagina',
    enabled: 'Abilitata',
    enabledHelp:
      'Disattivare un’origine nasconde le sue librerie nelle applicazioni Jellyfin senza perderle.',
    order: 'Ordine',
    position: (position: number, count: number) => `${position} di ${count}`,
    moveUp: (name: string) => `Sposta su ${name}`,
    moveDown: (name: string) => `Sposta giù ${name}`,
    moved: (name: string, position: number, count: number) =>
      `${name} spostata alla posizione ${position} di ${count}.`,
    description: 'Descrizione',
    details: 'Dettagli',
    manifestUrl: 'Indirizzo del manifest',
    address: 'Indirizzo',
    version: 'Versione',
    provides: 'Fornisce',
    types: 'Tipi',
    catalogs: 'Cataloghi',
    catalogCount: (count: number) => (count === 1 ? '1 catalogo' : `${count} cataloghi`),
    lastRefresh: 'Ultimo aggiornamento',
    channels: 'Canali',
    notImported: 'Non importati',
    shownOf: (shown: string, total: string) => `${shown} mostrati su ${total}`,
    vod: 'Film e serie TV',
    vodShort: (movies: string, series: string) => `${movies} film e ${series} serie TV importati`,
    lastFetch: 'Ultimo download',
    nextFetch: 'Prossimo download',
    never: 'Mai',
    figures: {
      entries: 'Voci nell’origine',
      shown: 'Canali mostrati',
      mapped: 'Con guida',
      unmapped: 'Senza guida',
    },
    openSource: 'Apri l’origine',
    iptvHint:
      'Cosa importare, categorie, canali, numeri e guide si impostano nella pagina dell’origine.',
    refresh: 'Aggiorna',
    refreshLabel: (name: string) => `Aggiorna ${name}`,
    refreshed: (name: string) => `${name}: il manifest è stato scaricato di nuovo.`,
    refreshedList: (name: string) => `${name}: l’elenco è stato scaricato di nuovo.`,
    replace: 'Sostituisci l’indirizzo',
    newManifestUrl: 'Nuovo indirizzo del manifest',
    replaceHint:
      'Usalo dopo aver riconfigurato l’add-on nella sua pagina di configurazione. Le sue librerie vengono mantenute.',
    replaceSubmit: 'Sostituisci',
    replaced: 'L’indirizzo del manifest è stato sostituito.',
    language: 'Lingua delle descrizioni',
    languageHelp:
      'Lascia Nessuna sull’unico add-on di metadati del server, qualunque sia la lingua impostata nella sua pagina di configurazione: un add-on con una lingua descrive i titoli solo per gli utenti che hanno scelto quella lingua, a meno che nessun add-on lasciato su Nessuna li descriva. Per permettere agli utenti di leggere i titoli in altre lingue, aggiungi di nuovo lo stesso add-on di metadati una volta per ogni lingua, configurato in quella lingua nella sua pagina di configurazione, e indica qui la lingua solo su queste copie aggiuntive: gli utenti che scelgono quella lingua dei metadati ne vedono i nomi, le trame e le immagini. Un add-on con una lingua e senza una propria libreria si limita a descrivere i titoli.',
    languageNone: 'Nessuna',
    languageSaved: (name: string) => `${name}: lingua delle descrizioni salvata.`,
    edit: 'Modifica l’account',
    saved: 'Origine salvata.',
    settings: 'Impostazioni',
    remove: 'Rimuovi',
    removeTitle: (name: string) => `Rimuovere ${name}?`,
    removeShared:
      'Le sue librerie scompariranno dalle applicazioni Jellyfin di tutti gli utenti. L’operazione non può essere annullata.',
    removeMine:
      'Le sue librerie scompariranno dalle tue applicazioni Jellyfin. L’operazione non può essere annullata.',
    removed: (name: string) => `${name} è stata rimossa.`,
    readOnly: (name: string) =>
      `Questa è un’origine personale di ${name}. Solo il proprietario può modificarla, dalla sua pagina Le mie origini.`,
    back: 'Origini',
    backMine: 'Le mie origini',
    notFoundTitle: 'Origine non trovata',
    notFound: 'Questa origine non esiste o non è tra quelle che puoi gestire.',
  },
  sourceAdd: {
    title: 'Aggiungi un’origine',
    kind: 'Tipo di origine',
    kinds: {
      stremio: 'Add-on Stremio',
      music: 'Add-on Eclipse',
      iptv: 'Origine IPTV',
      tuner: 'Sintonizzatore HDHomeRun',
      local: 'Cartella',
    },
    stremioHelp:
      'Un add-on Stremio fornisce cataloghi, metadati e stream. Incolla il suo link di installazione.',
    musicHelp:
      'Un add-on musicale Eclipse fornisce musica, audiolibri o podcast. Incolla l’indirizzo del suo manifest.',
    manifestUrl: 'Indirizzo del manifest',
    manifestUrlPlaceholder: 'https://…/manifest.json',
    manifestUrlHint:
      'Copia il link di installazione dalla pagina di configurazione dell’add-on. Spesso contiene le tue impostazioni o chiavi: Polyfin lo mantiene privato e ne mostra sempre solo una forma abbreviata.',
    install: 'Installa',
    installed: (name: string) => `${name} è stato installato.`,
    installedNoLibraries: (name: string) =>
      `${name} è stato installato. Nessuno dei suoi cataloghi è ancora una libreria: attivali in Contenuti › Librerie.`,
    installedNoLibrariesMine: (name: string) =>
      `${name} è stato installato. Nessuno dei suoi cataloghi è ancora una libreria: attivali in Le mie origini.`,
    installedLibraryLimit: (name: string, limit: number) =>
      `${name} è stato installato. Il server ha già ${limit} librerie, il massimo che Polyfin crea automaticamente, quindi nessuno dei suoi cataloghi lo è diventato: attivali in Contenuti › Librerie.`,
    installedLibraryLimitMine: (name: string, limit: number) =>
      `${name} è stato installato. Hai già ${limit} librerie, il massimo che Polyfin crea automaticamente, quindi nessuno dei suoi cataloghi lo è diventato: attivali in Le mie origini.`,
    installedMyMusic: (name: string) =>
      `${name} è stato installato. Non ha cataloghi propri, quindi la sua libreria è La mia musica, che all’inizio è vuota: elenca ciò che ogni utente ascolta o segna come preferito. Trova la sua musica con la ricerca nelle app.`,
    iptvHelp:
      'Importa direttamente i canali di una scaletta M3U o di un account Xtream Codes. Compaiono in Diretta TV come un catalogo TV, che puoi ordinare in Librerie. L’elenco viene scaricato di nuovo all’intervallo impostato in Impostazioni › Diretta TV.',
    steps: 'Passaggi',
    stepAccount: 'Account',
    stepImport: 'Cosa importare',
    name: 'Nome',
    account: 'Account',
    m3u: 'Scaletta M3U',
    xtream: 'Account Xtream Codes',
    playlistUrl: 'Indirizzo della scaletta',
    playlistUrlHint:
      'L’indirizzo M3U fornito dal tuo provider. Può contenere i tuoi dati di accesso: non viene mai mostrato per intero.',
    server: 'Indirizzo del server',
    serverHint: 'Ad esempio https://server:8080, come te lo fornisce il tuo provider.',
    username: 'Nome utente',
    password: 'Password',
    keepHint: 'Lascia vuoto per mantenere quello attuale.',
    passwordKeepHint: 'Lascia vuoto per mantenere la password attuale.',
    userAgent: 'User agent (facoltativo)',
    userAgentHint:
      'Se lo lasci vuoto, i canali vengono richiesti come li richiede un browser web. Inserisci quello che il tuo provider chiede quando serve solo certi lettori: la lista e i canali vengono allora richiesti con esso. Uno user agent che la lista dà a un canale ha la precedenza.',
    providerGuide: 'Usa la guida dei programmi del provider',
    providerGuideHelp:
      'I server Xtream Codes pubblicano una guida per l’account: compila i programmi dei canali.',
    guideUrl: 'Indirizzo della guida dei programmi (XMLTV, facoltativo)',
    guideUrlHint:
      'File XMLTV semplice o compresso. Puoi anche aggiungerlo o cambiarlo in seguito in Librerie.',
    next: 'Avanti: scegli cosa importare',
    back: 'Torna all’account',
    import: 'Importa',
    added: (name: string, parts: string[]) => `${name} è stata aggiunta: ${parts.join(', ')}.`,
    channels: (count: string) => `${count} canali`,
    movies: (count: string) => `${count} film`,
    series: (count: string) => `${count} serie TV`,
    tunerHelp:
      'Un sintonizzatore HDHomeRun nella tua rete fornisce i canali che riceve via antenna o cavo. Compaiono in Diretta TV come quelli di un’origine IPTV, e il dispositivo riproduce contemporaneamente tanti canali quanti sono i suoi sintonizzatori.',
    findTuners: 'Cerca sintonizzatori',
    findTunersHelp:
      'Chiede alla rete locale quali sintonizzatori sono presenti; richiede alcuni secondi.',
    finding: 'Ricerca nella rete locale…',
    foundTitle: 'Sintonizzatori nella rete',
    tunerCount: (count: number, formatted: string) =>
      count === 1 ? '1 sintonizzatore' : `${formatted} sintonizzatori`,
    addTuner: 'Aggiungi',
    addTunerLabel: (name: string) => `Aggiungi ${name}`,
    tunerAdded: 'Aggiunto',
    openTuner: (name: string) => `Apri ${name}`,
    noTuners: 'Nessun sintonizzatore ha risposto.',
    noTunersHelp:
      'In Docker, Polyfin rileva i sintonizzatori solo con la rete host (network_mode: host). Puoi anche aggiungere un sintonizzatore tramite il suo indirizzo.',
    byAddress: 'Aggiungi tramite indirizzo',
    tunerAddress: 'Indirizzo del sintonizzatore',
    tunerAddressHint: 'Il suo indirizzo IP, ad esempio 192.168.1.20, o il suo indirizzo http://.',
    tunerAddressInvalid:
      'Inserisci l’indirizzo IP del sintonizzatore, ad esempio 192.168.1.20, o il suo indirizzo http://.',
    tunerName: 'Nome (facoltativo)',
    tunerNameHint: 'Se vuoto, viene usato il nome del sintonizzatore.',
    favoritesOnly: 'Solo i canali preferiti del sintonizzatore',
    favoritesOnlyHelp:
      'Mantiene i canali segnati come preferiti sul sintonizzatore, nella sua app o nella sua pagina web, ed esclude gli altri.',
  },
  localFolders: {
    help: {
      container:
        'Una cartella montata nel container di Polyfin, è sufficiente la sola lettura. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on; una cartella di musica diventa una libreria musicale. Polyfin viene eseguito come utente 65532, che deve poterla leggere.',
      computer:
        'Una cartella del computer su cui è in esecuzione Polyfin, o di un disco o di una condivisione che raggiunge, è sufficiente la sola lettura. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on; una cartella di musica diventa una libreria musicale. L’account con cui viene eseguito Polyfin deve poterla leggere.',
    },
    location: 'Posizione',
    locations: {
      local: 'Cartella locale',
      smb: 'Condivisione SMB',
      webdav: 'Cartella WebDAV',
      s3: 'Bucket S3',
      sftp: 'Cartella SFTP',
      rclone: 'Remote rclone',
    },
    shareHelp: {
      smb: 'Una cartella condivisa tramite SMB (condivisione file di Windows) da un NAS o da un altro computer, che Polyfin legge direttamente, senza montarla. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on; una cartella di musica diventa una libreria musicale.',
      webdav:
        'Una cartella che un server condivide tramite WebDAV, che Polyfin legge direttamente, senza montarla. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on; una cartella di musica diventa una libreria musicale.',
      s3: 'Un prefisso di un bucket di uno storage compatibile con S3, che Polyfin legge direttamente, senza montarlo. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on, letti tramite la cache dei file in lettura; una cartella di musica diventa una libreria musicale.',
      sftp: 'Una cartella di un server raggiunto tramite SSH, che Polyfin legge direttamente, senza montarla. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on, letti tramite la cache dei file in lettura; una cartella di musica diventa una libreria musicale.',
      rclone:
        'Una cartella di uno degli storage raggiunti da rclone, letta tramite un’istanza di rclone che esegue rclone rcd --rc-serve, ad esempio un container accanto a Polyfin. I suoi video vengono abbinati ai titoli in base al nome e si riproducono come versioni di quei titoli, accanto agli stream degli add-on, letti tramite la cache dei file in lettura; una cartella di musica diventa una libreria musicale.',
    },
    address: 'Indirizzo',
    addressHint: {
      smb: 'smb:// seguito dal server, dalla condivisione e, se presente, dalla cartella al suo interno. Aggiungi :porta dopo il server per una porta diversa da 445.',
      webdav:
        'L’indirizzo WebDAV della cartella. Preferisci https://, perché http:// invia la password non cifrata.',
      s3: 's3:// seguito dal bucket e, se presente, dal prefisso al suo interno. I suoi oggetti vengono elencati come cartelle in base a /.',
      sftp: 'sftp:// seguito dal server e dal percorso completo della cartella sul server. Aggiungi :porta dopo il server per una porta diversa da 22.',
      rclone: '',
    },
    addressPlaceholder: {
      smb: 'smb://nas.local/media/film',
      webdav: 'https://nas.local/dav/film',
      s3: 's3://media/film',
      sftp: 'sftp://nas.local/srv/media/film',
      rclone: '',
    },
    user: 'Utente',
    userHint: {
      smb: 'Vuoto per una condivisione aperta agli ospiti. DOMINIO\\utente per un account di dominio.',
      webdav: 'Vuoto per una cartella aperta a tutti.',
      s3: 'Entrambi vuoti per un bucket leggibile da chiunque.',
      sftp: 'L’account con cui Polyfin accede.',
      rclone: 'L’utente del controllo remoto di rclone (--rc-user), vuoto se non c’è.',
    },
    password: 'Password',
    passwordHint: 'Polyfin non la mostrerà più.',
    sftpPasswordHint:
      'Vuoto quando l’accesso avviene con la chiave privata. Polyfin non la mostrerà più.',
    accessKey: 'Chiave di accesso',
    secretKey: 'Chiave segreta',
    endpoint: 'Endpoint',
    endpointHint:
      'L’indirizzo del servizio S3 dello storage. Polyfin indica il bucket nel percorso, come accetta ogni storage compatibile con S3, tranne che negli endpoint di Amazon.',
    region: 'Regione',
    regionHint:
      'Vuoto per us-east-1, accettata dalla maggior parte degli storage compatibili con S3.',
    privateKey: 'Chiave privata',
    privateKeyHint:
      'Una chiave privata OpenSSH o PEM, al posto della password o insieme a essa. Polyfin non la mostrerà più.',
    privateKeySaved: 'È salvata una chiave. Incollane un’altra per sostituirla.',
    privateKeyNotSent:
      'La chiave salvata non viene inviata a un altro server: incollala di nuovo, altrimenti verrà rimossa al salvataggio.',
    secretNotSent:
      'Il valore salvato non viene inviato a un altro server: digitalo di nuovo, altrimenti verrà rimosso al salvataggio.',
    privateKeyRemove: 'Rimuovi la chiave salvata',
    passphrase: 'Passphrase',
    passphraseHint: 'Solo per una chiave privata che ne ha una.',
    hostKey: 'Chiave host',
    hostKeyHint:
      'Salvata alla prima connessione. Polyfin rifiuta il server quando ne presenta un’altra.',
    hostKeyNone: 'Salvata alla prima connessione',
    hostKeyOffered: (fingerprint: string) =>
      `Il server ora presenta un’altra chiave host, ${fingerprint}, e Polyfin la rifiuta. Accettala solo se il server è stato reinstallato o se la sua chiave è stata cambiata intenzionalmente.`,
    acceptHostKey: 'Accetta la nuova chiave',
    hostKeyAccepted:
      'La nuova chiave host è stata accettata. Polyfin scansiona di nuovo la cartella.',
    rcloneInstance: 'Indirizzo di rclone',
    rcloneInstanceHint:
      'Dove risponde rclone rcd --rc-serve, raggiunto tramite http://, ad esempio http://rclone:5572 per un container accanto a Polyfin.',
    rcloneRemote: 'Remote',
    rcloneRemoteHint: 'Uno dei remote della configurazione di rclone.',
    listRemotes: 'Elenca i remote',
    remotesListed: (count: number, figure: string) =>
      count === 1 ? 'rclone ha un remote.' : `rclone ha ${figure} remote.`,
    noRemotes: 'rclone non ha alcun remote nella sua configurazione.',
    remotesFailed: {
      unreachable: 'rclone non risponde a questo indirizzo.',
      refused: 'rclone rifiuta questo utente o questa password.',
    } as Record<string, string>,
    rcloneFolder: 'Percorso nel remote',
    rcloneFolderHint: 'La cartella del remote da leggere, vuoto per la sua radice.',
    pollMinutes: 'Cerca modifiche ogni (minuti, 0 = solo durante le scansioni)',
    pollMinutesHint: {
      s3: 'Le cartelle del bucket vengono elencate di nuovo con questa frequenza per trovare i file aggiunti e rimossi. I provider possono addebitare ogni elenco. 30 per impostazione predefinita.',
      sftp: 'La cartella e le sue sottocartelle vengono elencate di nuovo con questa frequenza per trovare i file aggiunti e rimossi. 2 per impostazione predefinita.',
      rclone:
        'Le cartelle del remote vengono elencate di nuovo con questa frequenza per trovare i file aggiunti e rimossi. 2 per impostazione predefinita.',
    },
    name: 'Nome',
    nameHint: 'Il nome della sua libreria nelle applicazioni Jellyfin.',
    path: { container: 'Percorso nel container', computer: 'Percorso' },
    pathHint: {
      container:
        'Dove la cartella è montata nel container di Polyfin, non il suo percorso sull’host.',
      linux: 'Il percorso completo della cartella sul computer su cui è in esecuzione Polyfin.',
      macos:
        'Il percorso completo della cartella sul Mac su cui è in esecuzione Polyfin; le cartelle di un disco esterno si trovano in /Volumes.',
      windows:
        'Il percorso completo della cartella sul PC su cui è in esecuzione Polyfin: su un’unità, come D:\\Film, o su una condivisione, come \\\\nas\\media\\Film.',
    },
    pathPlaceholder: {
      container: '/media/film',
      linux: '/srv/media/film',
      macos: '/Users/io/Film',
      windows: 'D:\\Film',
    },
    kind: 'Contiene',
    kinds: { movies: 'Film', shows: 'Serie TV', music: 'Musica', homevideos: 'Video personali' },
    kindHelp: {
      movies:
        'Un file per film, da solo o in una cartella propria, con il nome del film e il suo anno.',
      shows:
        'Una cartella per serie TV, con la stagione e l’episodio nel nome di ogni file, come S01E02.',
      music:
        'File audio letti dai loro tag, altrimenti dai loro nomi, come Artista/Album/01 Titolo.flac, raggruppati in album e artisti.',
      homevideos:
        'Ogni video è un titolo a sé, con il nome del suo file, o della sua cartella quando vi è da solo, e datato dal suo file NFO, dalla sua registrazione o dal file. Non viene interpellato nessun add-on di metadati.',
    },
    kindNouns: {
      movies: 'film',
      shows: 'serie TV',
      music: 'musica',
      homevideos: 'video personali',
    },
    changeKind: 'Cambia',
    changeKindTitle: (name: string) => `Cambiare il contenuto di ${name}?`,
    changeKindScan: (from: string, to: string) =>
      `Contiene ${from}; conterrà ${to}. La cartella e la sua libreria mantengono i loro identificativi, il suo indirizzo e le sue impostazioni, e Polyfin la scansiona di nuovo come ${to}, in background.`,
    changeKindFollows:
      'Ciò che gli utenti hanno fatto con ogni video, il suo stato di visione, il punto di ripresa e il preferito, lo segue nel suo nuovo titolo: gli stati di visto e i preferiti vengono copiati e il titolo precedente li mantiene; il punto di ripresa si sposta con il file e il titolo precedente lo perde.',
    changeKindNothing:
      'Non viene trasferito nulla: brani e video sono file diversi. Ciò che gli utenti hanno fatto con i suoi titoli resta associato a essi e non viene più mostrato.',
    changeKindBack:
      'I suoi video personali vengono ricordati per 30 giorni: se entro questo periodo torna a contenere video personali, li elenca come gli stessi titoli, con ciò che gli utenti hanno fatto con essi.',
    linksDropped: (count: number, figure: string) =>
      count === 1 ? 'l’associazione manuale' : `le ${figure} associazioni manuali`,
    episodesDropped: (count: number, figure: string) =>
      count === 1
        ? 'gli episodi assegnati al titolo di un disco'
        : `gli episodi assegnati a ${figure} titoli di dischi`,
    changeKindDrops: (parts: string[]) =>
      `Eliminati, perché creati per il contenuto attuale: ${parts.join(' e ')}.`,
    useArtwork: 'Usa le sue immagini',
    useArtworkHelp:
      'Locandine, sfondi e loghi accanto ai file, come poster.jpg o Film (2001)-fanart.jpg, vengono mostrati al posto di quelli degli add-on di metadati per i rispettivi titoli, e le immagini degli episodi al posto delle loro anteprime. Se disattivato, vengono mostrati quelli degli add-on.',
    readExtras: 'Leggi extra e contenuti a tema',
    readExtrasHelp:
      'Extra, trailer locali e musiche a tema accanto ai titoli, con i nomi usati da Jellyfin, come Extras/Making of.mkv, Film (2001)-trailer.mkv o theme.mp3, vengono mostrati tra i contenuti speciali dei rispettivi titoli, riprodotti dal pulsante Trailer o in sottofondo alle loro pagine. Se disattivato, la scansione successiva li esclude.',
    add: 'Aggiungi la cartella',
    added: (name: string) => `${name} è stata aggiunta. La sua prima scansione è iniziata.`,
    edit: 'Modifica la cartella',
    save: 'Salva',
    cancel: 'Annulla',
    saved: 'Cartella salvata.',
    movedHint:
      'Un nuovo percorso dimentica i file trovati in quello precedente, associazioni incluse.',
    movedShareHint:
      'Un nuovo indirizzo dimentica i file trovati in quello precedente, associazioni incluse.',
    scan: 'Scansiona ora',
    scanLabel: (name: string) => `Scansiona ${name} ora`,
    scanStarted: (name: string) => `${name}: la scansione è iniziata.`,
    scanning: 'Scansione in corso',
    unreadable: 'Illeggibile',
    figuresTitle: 'File',
    files: 'File video',
    matched: 'Associati a un titolo',
    unmatched: 'Non associati',
    extras: 'Extra',
    tracks: 'Brani',
    albums: 'Album',
    artists: 'Artisti',
    gained: 'Con guadagno',
    measured: 'Misurati',
    leftOut: 'Esclusi',
    videos: 'Video',
    datedBy: {
      nfo: 'Datati da un file NFO',
      created: 'Datati dalla registrazione',
      file: 'Datati dal file',
    },
    matchedBy: (parts: string[]) => `Abbinati: ${parts.join(', ')}.`,
    matchMethods: {
      name: (count: string) => `${count} tramite un identificativo nel nome`,
      nfo: (count: string) => `${count} tramite un file NFO`,
      search: (count: string) => `${count} tramite una ricerca`,
      year: (count: string) => `${count} tramite una ricerca, di un anno di differenza`,
      numerals: (count: string) =>
        `${count} tramite una ricerca, con i numeri letti allo stesso modo`,
      yearNumerals: (count: string) =>
        `${count} tramite una ricerca, di un anno di differenza e con i numeri letti allo stesso modo`,
      link: (count: string) => `${count} manualmente`,
    },
    holds: 'Contenuto',
    lastScan: 'Ultima scansione',
    lastAttempt: 'Ultimo tentativo',
    never: 'Non ancora',
    errorTitle: 'Polyfin non riesce a leggere questa cartella',
    errorTitleShare: 'Polyfin non riesce a leggere questa condivisione',
    errors: {
      missing:
        'Il percorso non esiste dove è in esecuzione Polyfin: verifica che la cartella, o il disco o la condivisione in cui si trova, sia presente e, in Docker, che sia montata nel container.',
      unreadable:
        'La cartella esiste ma Polyfin non riesce a elencarne il contenuto: concedi all’account con cui è in esecuzione (l’utente 65532 in Docker) il permesso di leggerla e di aprirne le cartelle.',
      not_folder: 'Questo percorso è un file, non una cartella.',
    } as Record<string, string>,
    shareErrors: {
      missing: 'La condivisione, o la cartella al suo interno, non esiste: verifica l’indirizzo.',
      unreadable:
        'L’utente della condivisione non può elencare questa cartella: verifica i suoi permessi sul server.',
      not_folder: 'Questo indirizzo è un file, non una cartella.',
      unreachable:
        'Polyfin non riesce a raggiungere la condivisione: verifica che il server sia acceso, e controlla il server e la porta dell’indirizzo.',
      refused: 'La condivisione rifiuta l’utente o la password: verificali e inseriscili di nuovo.',
      host_key_changed:
        'Il server SFTP presenta una chiave host diversa da quella salvata alla prima connessione, e Polyfin la rifiuta. Se la modifica è prevista, accetta la nuova chiave in Modifica la cartella.',
    } as Record<string, string>,
    keptNote:
      'I file trovati in precedenza restano nella libreria finché la cartella non potrà essere letta di nuovo.',
    unmatchedTitle: 'File non abbinati',
    unmatchedHelp:
      'Non è stato trovato alcun titolo per questi file. Rinominali con il titolo e l’anno, oppure aggiungi un file NFO con l’identificativo del titolo, quindi avvia una scansione; oppure collegali all’identificativo IMDb presente nell’indirizzo IMDb del titolo (tt seguito da cifre). Un collegamento resta valido anche dopo nuove scansioni; in una cartella di serie TV copre l’intera serie e, per un disco Blu-ray o un DVD, l’intero disco, oppure uno dei film di un disco che ne contiene diversi. I titoli del disco di una serie TV prendono l’episodio così come lo indica il nome di un file, ad esempio S01E05, oppure S01E05-E06 per due; gli episodi suggeriti per un disco seguono quelli dei dischi precedenti della stagione.',
    noUnmatched: 'Ogni file è abbinato a un titolo.',
    leftOutTitle: 'Tracce escluse',
    leftOutHelp:
      'Né i tag né i nomi di questi file indicano un titolo o un album. Aggiungi i tag, oppure spostali in una cartella Artista/Album, quindi avvia una scansione.',
    noLeftOut: 'Ogni traccia è in un album.',
    shownOf: (shown: string, total: string) => `${shown} mostrati su ${total}.`,
    reasons: {
      unreadable_name: 'Il nome non indica alcun titolo o numero di episodio',
      disc_title: 'Un altro film del disco: collegalo al suo titolo',
      disc_episode: 'Un titolo del disco di una serie TV: indica il suo episodio',
      lone_extra: 'Un extra senza il suo titolo accanto: collegalo per renderlo un titolo',
      not_found: 'Gli add-on dei metadati non conoscono alcun titolo con questo nome',
      no_search: 'Nessun add-on del server descrive e cerca titoli di questo tipo',
      ambiguous: 'Diversi titoli hanno questo nome: aggiungi l’anno oppure collegalo',
      other_year: 'I titoli con questo nome sono di altri anni',
      search_failed: 'Impossibile effettuare la ricerca negli add-on dei metadati',
      no_title: 'I tag e il nome non indicano alcun titolo',
      no_album: 'I tag e le cartelle non indicano alcun album',
    } as Record<string, string>,
    searchFailedBy: (addon: string, failure: string) =>
      failure === ''
        ? `La ricerca di ${addon} non è riuscita`
        : `La ricerca non è riuscita: ${addon} ${failure}`,
    searchFailures: {
      unreachable: 'non è raggiungibile',
      timeout: 'non ha risposto in tempo',
      private_network: 'si trova su una rete locale',
      rate_limited: 'ha chiesto di rallentare (troppe richieste)',
      server_error: 'ha risposto con un errore del server',
      http_error: 'ha rifiutato la richiesta',
      invalid_response: 'ha inviato una risposta che Polyfin non riesce a leggere',
    } as Record<string, string>,
    noSearchTitle: (count: number, figure: string) =>
      count === 1
        ? 'Nessun add-on può cercare il titolo di 1 file'
        : `Nessun add-on può cercare i titoli di ${figure} file`,
    noSearchHelp:
      'L’associazione richiede un add-on del server, in Contenuti › Origini, che descriva questi titoli e abbia un catalogo di ricerca. Gli add-on personali degli utenti, in Le mie origini, non contano. Una volta aggiunto, scansiona la cartella.',
    searchFailedTitle: (count: number, figure: string) =>
      count === 1
        ? 'Non è stato possibile cercare il titolo di 1 file'
        : `Non è stato possibile cercare i titoli di ${figure} file`,
    searchRetry: (at: string) =>
      `Polyfin cerca di nuovo da solo, la prossima volta il ${at}, poi meno spesso finché la ricerca non riesce, e a ogni scansione.`,
    searchHelp:
      'L’associazione richiede un add-on del server che descriva questi titoli e abbia un catalogo di ricerca. Se le sue ricerche continuano a non riuscire, controllalo in Sistema › Stato di salute, oppure associa i file a mano qui sotto.',
    read: (title: string) => `Letto come «${title}»`,
    showFolder: (unit: string) => `Cartella della serie TV: ${unit}`,
    discTitle: (disc: string, title: number) => `${disc} · titolo ${title}`,
    disc: {
      bluray: (unit: string) => `Disco Blu-ray: ${unit}`,
      dvd: (unit: string) => `DVD: ${unit}`,
      bluray_image: (unit: string) => `Immagine Blu-ray: ${unit}`,
      dvd_image: (unit: string) => `Immagine DVD: ${unit}`,
    },
    minutes: (minutes: number) => `${minutes} min`,
    episodeLabel: (title: string) => `Episodio di ${title}`,
    episodePlaceholder: 'S01E01',
    giveEpisode: 'Salva',
    episodesGiven: (count: number) =>
      count === 1 ? 'Il titolo è numerato.' : `${count} titoli sono numerati.`,
    suggestion: (disc: string, first: string, last: string) =>
      first === last
        ? `Suggerito per ${disc}: ${first}`
        : `Suggeriti per ${disc}: da ${first} a ${last}`,
    confirmSuggestion: 'Usa questi episodi',
    imdbId: 'Identificativo IMDb',
    imdbPlaceholder: 'tt0000000',
    link: 'Collega',
    linkLabel: (path: string) => `Identificativo IMDb per ${path}`,
    linked: (unit: string, id: string) => `${unit} è collegato a ${id}.`,
    findTitle: 'Trova il titolo',
    searchLabel: (path: string) => `Ricerca del titolo per ${path}`,
    searchQuery: 'Titolo',
    searchYear: 'Anno',
    search: 'Cerca',
    searching: 'Ricerca negli add-on dei metadati…',
    noTitles:
      'Gli add-on dei metadati non hanno trovato alcun titolo con questo nome e un identificativo IMDb. Modifica la ricerca oppure digita l’identificativo.',
    titlesFound: (query: string) => `Titoli trovati per «${query}»`,
    titleKinds: { movie: 'Film', show: 'Serie TV' },
    pickTitle: (name: string, details: string) => `Collega a ${name}, ${details}`,
    linksTitle: 'Collegati manualmente',
    unlink: 'Rimuovi il collegamento',
    unlinkLabel: (unit: string) => `Rimuovi il collegamento di ${unit}`,
    unlinked: (unit: string) =>
      `Collegamento di ${unit} rimosso: viene di nuovo abbinato in base al nome o al file NFO.`,
    removeEpisodeLabel: (title: string) => `Rimuovi l’episodio di ${title}`,
    episodeRemoved: (title: string) =>
      `Episodio di ${title} rimosso: viene numerato dal nome del disco oppure viene di nuovo suggerito.`,
    looserTitle: 'Abbinati con una regola più flessibile',
    looserHelp:
      'Nessun titolo ha esattamente il nome e l’anno di questi file, quindi Polyfin li ha abbinati all’unico titolo che una regola più flessibile lascia: uno dell’anno prima o dopo, o uno con lo stesso nome una volta letti allo stesso modo i numeri, come 2 per II o Part 1 per Part One. Controllali. Scollega quello sbagliato: da quel momento viene abbinato solo in base al nome e all’anno esatti, e resta non abbinato finché non lo colleghi al suo titolo.',
    looserRules: {
      year: 'Un titolo di un anno di differenza',
      numerals: 'Numeri letti allo stesso modo',
      year_numerals: 'Un titolo di un anno di differenza, numeri letti allo stesso modo',
    } as Record<string, string>,
    matchedTo: (title: string) => `Abbinato a «${title}»`,
    fileCount: (count: number, figure: string) => (count === 1 ? '1 file' : `${figure} file`),
    looserUnlinkLabel: (unit: string) => `Scollega ${unit} dal suo titolo`,
    looserUnlinked: (unit: string) =>
      `${unit} è stato scollegato: viene abbinato solo in base al nome e all’anno esatti.`,
    openPage: 'Apri la pagina della cartella',
  },
  mySources: {
    lede: 'Aggiungi le tue origini Stremio, Eclipse e IPTV e scegli quali dei loro cataloghi compaiono come librerie nelle tue applicazioni Jellyfin.',
    preferenceTitle: 'Le origini del server',
    useShared: 'Usa le origini del server',
    useSharedHelp:
      'Attivo: ricevi le origini e le librerie del server, seguite dalle tue. Disattivato: ricevi solo le tue origini e librerie.',
    preferenceSaved: 'Preferenza salvata.',
    parentalControl:
      'Il controllo parentale si applica al tuo account: le tue applicazioni Jellyfin mostrano solo le origini e le librerie del server, perché forniscono le classificazioni su cui si basa. Le tue origini vengono conservate ma non usate.',
    personalAddonsOff:
      'Un amministratore ha disattivato le tue origini: le tue applicazioni Jellyfin mostrano solo le origini e le librerie del server. Le tue origini vengono conservate ma non usate.',
    listTitle: 'Le tue origini',
  },
  music: {
    content: { music: 'Musica', audiobook: 'Audiolibri', podcast: 'Podcast' } as Record<
      string,
      string
    >,
    library: {
      music: 'Libreria musicale',
      audiobook: 'Libreria di libri',
      podcast: 'Libreria musicale',
    } as Record<string, string>,
    settings: 'Impostazioni',
    settingsTitle: 'Impostazioni dell’add-on',
    settingsHelp:
      'Dichiarate dall’add-on e inviate con ogni richiesta che Polyfin gli fa. Possono contenere i dati del tuo account: solo tu, o gli amministratori per gli add-on del server, potete vederle.',
    noSettings: 'Questo add-on non ha impostazioni.',
    defaultValue: (value: string) => `Predefinito: ${value}`,
    defaultEmpty: 'Predefinito: vuoto',
    on: 'Attivo',
    off: 'Disattivato',
    range: (min: string, max: string) => `Da ${min} a ${max}.`,
    atLeast: (min: string) => `Almeno ${min}.`,
    atMost: (max: string) => `Al massimo ${max}.`,
    stepOf: (step: string) => `A intervalli di ${step}.`,
    maxLength: (count: number) =>
      count === 1 ? `Al massimo ${count} carattere.` : `Al massimo ${count} caratteri.`,
    tooLong: (count: number) =>
      count === 1 ? `Usa al massimo ${count} carattere.` : `Usa al massimo ${count} caratteri.`,
    belowMin: (min: string) => `Inserisci ${min} o più.`,
    aboveMax: (max: string) => `Inserisci ${max} o meno.`,
    offStep: (step: string) => `Usa un multiplo di ${step}.`,
    resetDefaults: 'Usa i valori predefiniti',
    saved: 'Impostazioni salvate. L’add-on le riceve dalla sua prossima richiesta.',
  },
}

export default sources
