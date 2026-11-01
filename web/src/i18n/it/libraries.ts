import type en from '../en/libraries'

const libraries: typeof en = {
  libraries: {
    title: 'Librerie',
    description:
      'Scegli quali cataloghi degli add-on del server compaiono come librerie nelle applicazioni Jellyfin, per tutti gli utenti.',
    shownTitle: 'Mostrate nelle applicazioni Jellyfin',
    shownHelp:
      'Le librerie compaiono in questo ordine. Lascia un nome vuoto per usare il nome del catalogo. Alle librerie con lo stesso nome viene aggiunto il tipo nelle applicazioni, per esempio «Popolari (Film)».',
    shownHelpMine:
      'Le librerie compaiono in questo ordine, dopo le librerie del server se le usi. Lascia un nome vuoto per usare il nome del catalogo. Alle librerie con lo stesso nome viene aggiunto il tipo nelle applicazioni, per esempio «Popolari (Film)».',
    defaultHelp:
      'Quando installi un add-on, i suoi cataloghi di collezioni diventano librerie: ognuna mostra le sue collezioni (per esempio generi o decenni), che si aprono sui rispettivi film e serie TV. Un add-on senza collezioni ottiene come librerie i suoi cataloghi di film, serie TV e TV, fino a 20. È solo un punto di partenza: abilita tutti i cataloghi che vuoi. Ogni libreria aggiunge una riga alla schermata iniziale delle applicazioni Jellyfin, tranne i cataloghi TV, i cui canali compaiono in Diretta TV.',
    manyWarning:
      'Più di 20 librerie possono appesantire la schermata iniziale delle applicazioni Jellyfin.',
    noAddons: 'Aggiungi prima un’origine: i suoi cataloghi verranno elencati qui.',
    shownEmpty: 'Nessuna libreria: le applicazioni Jellyfin non mostrano nulla di questi add-on.',
    name: 'Nome nelle applicazioni',
    appName: (name: string) => `Mostrata nelle applicazioni come «${name}».`,
    liveTv: 'I suoi canali compaiono in Diretta TV nelle applicazioni Jellyfin.',
    iptvVod: 'Da un’origine IPTV.',
    iptvVodLink: 'Le sue opzioni di importazione',
    addonOff: 'Add-on disattivato',
    missing: 'Non più disponibile',
    hideInMenus: 'Nascondi dalla barra superiore del player web',
    hideInMenusHelp:
      'Mantiene la sua riga nella schermata iniziale. Anche il menu laterale del player web la esclude; le altre applicazioni Jellyfin la elencano comunque. Si applica al prossimo caricamento del player web.',
    hiddenInMenus: 'Nascosta dalla barra superiore',
    skipLocal: 'Escludi i titoli presenti nelle cartelle locali',
    skipLocalHelp:
      'Un film o una serie che la libreria di una cartella locale elenca, collegato allo stesso identificativo, compare solo lì. Le sue versioni e il suo stato di visione restano gli stessi, e la ricerca lo trova una volta. Un utente che non vede la libreria della cartella lo trova ancora qui.',
    skippedLocal: 'Senza i titoli delle cartelle locali',
    showAsTitles: 'Mostra come film e serie',
    showAsTitlesHelp:
      'Per le applicazioni Jellyfin che elencano solo film e serie: un titolo con più video, o con uno numerato come episodio, diventa una serie, i suoi video i suoi episodi; un titolo senza video, o con un solo file, un film. Gli stati di visione, i punti di ripresa e i preferiti li seguono al salvataggio.',
    shownAsTitles: 'Mostrata come film e serie',
    remove: 'Rimuovi',
    removeLabel: (name: string) => `Rimuovi ${name}`,
    moreActions: (name: string) => `Altre azioni per ${name}`,
    userOrders: {
      help: 'Un utente che ha ordinato le librerie nelle Impostazioni › Home del player web mantiene quell’ordine, fino a «Applica questo ordine a tutti gli utenti» o al proprio «Ripristina le mie preferenze».',
      apply: 'Applica questo ordine a tutti gli utenti',
      saveFirst: 'Salva prima l’elenco: viene applicato l’ordine salvato.',
      title: 'Applicare questo ordine a tutti gli utenti?',
      counting: 'Conteggio degli utenti con un ordine proprio…',
      none: 'Tutti gli utenti seguono già questo ordine: nessuno ha ordinato le librerie nelle Impostazioni › Home del player web.',
      some: (count: number) =>
        `${count === 1 ? '1 utente ha ordinato' : `${count} utenti hanno ordinato`} le librerie nelle Impostazioni › Home del player web. Il loro ordine viene dimenticato e le loro applicazioni elencano le librerie in questo ordine, quello salvato; il player web entro un minuto. Le altre impostazioni restano.`,
      applied: (count: number) =>
        count === 0
          ? 'Tutti gli utenti seguono questo ordine.'
          : count === 1
            ? 'Questo ordine ora vale per tutti: è stato dimenticato l’ordine proprio di 1 utente.'
            : `Questo ordine ora vale per tutti: è stato dimenticato l’ordine proprio di ${count} utenti.`,
    },
    refresh: {
      start: 'Aggiorna le descrizioni',
      stop: 'Interrompi l’aggiornamento',
      queued: 'In attesa di aggiornare le descrizioni',
      running: (done: string, titles: string) =>
        `Aggiornamento delle descrizioni: ${done} di ${titles}`,
      done: (done: string, failed: number, figure: string) =>
        failed === 0
          ? `Descrizioni aggiornate: ${done}`
          : `Descrizioni aggiornate: ${done}, ${figure} non descritte`,
      stopped: (done: string, titles: string) => `Aggiornamento interrotto a ${done} di ${titles}`,
      failed: 'Non è stato possibile aggiornare le descrizioni',
      started: (name: string) => `${name}: le descrizioni vengono rilette, 30 al minuto.`,
      progress: (name: string) => `Aggiornamento di ${name}`,
    },
    removedLive: (name: string) => `${name} rimosso dalle librerie.`,
    dragHandle: (name: string) => `Trascina ${name}`,
    availableTitle: 'Cataloghi disponibili',
    availableHelp:
      'Aggiungendo un catalogo, questo viene mostrato come libreria alla fine dell’elenco qui sopra.',
    filter: 'Filtra',
    filterPlaceholder: 'Nome del catalogo o dell’add-on',
    type: 'Tipo',
    allTypes: 'Tutti i tipi',
    add: 'Aggiungi',
    addLabel: (name: string) => `Aggiungi ${name}`,
    addedLive: (name: string, position: number) => `${name} aggiunto come libreria ${position}.`,
    notBrowsable: 'Non disponibile',
    notBrowsableHelp: 'Richiede una ricerca o un altro valore, quindi non può essere una libreria.',
    availableEmpty: 'Tutti i cataloghi sono già mostrati.',
    noMatch: 'Nessun catalogo corrisponde a questo filtro.',
    reset: 'Ripristina',
    saved: 'Librerie salvate.',
    guideAfterSave: 'Salva le librerie per aggiungere una guida dei programmi a questo catalogo.',
    guideFetched: 'Ultimo recupero',
    guideNever: 'Non ancora recuperata.',
    loading: 'Caricamento librerie…',
    listLabel: 'Librerie, nell’ordine in cui le mostrano le applicazioni',
    columnCatalog: 'Catalogo',
    shownEmptyTitle: 'Ancora nessuna libreria',
    shownEmptyHow: 'Aggiungi cataloghi dall’elenco qui sotto: ognuno diventa una libreria.',
    noAddonsTitle: 'Ancora nessun catalogo',
    addSource: 'Aggiungi un’origine',
    defaultTitle: 'Come vengono scelte le prime librerie',
    availableLabel: (addon: string) => `Cataloghi di ${addon}`,
    music: {
      library: {
        music: 'Libreria musicale',
        audiobook: 'Libreria di libri',
        podcast: 'Libreria musicale',
      } as Record<string, string>,
      help: {
        music: 'I suoi elementi compaiono nella sezione musica delle applicazioni Jellyfin.',
        audiobook:
          'I suoi audiolibri compaiono in una libreria di libri delle applicazioni Jellyfin.',
        podcast: 'I suoi episodi compaiono nella sezione musica delle applicazioni Jellyfin.',
      } as Record<string, string>,
    },
    image: {
      label: 'Immagine nelle applicazioni',
      edit: (name: string, state: string) => `Immagine di ${name}: ${state}`,
      choices: {
        none: 'Nessuna',
        automatic: 'Automatica',
        generated: 'Generata',
        custom: 'Personalizzata',
      },
      help: {
        none: 'Le applicazioni mostrano questa libreria senza immagine, come prima.',
        automatic:
          'Polyfin la prende dal catalogo: lo sfondo di uno dei primi titoli, altrimenti una locandina.',
        generated:
          'Polyfin la compone con le locandine dei primi titoli del catalogo, o delle prime collezioni di una libreria di collezioni, insieme al nome della libreria, dopo averle lette.',
        custom:
          'Un’immagine che carichi tu, o che Polyfin scarica una sola volta da un indirizzo. Le immagini larghe in 16:9 si adattano meglio ai riquadri delle librerie. JPEG, PNG o WebP, massimo 10 MB.',
      },
      parental: 'Gli utenti sotto controllo parentale o con generi bloccati non la vedono.',
      notFound: {
        automatic:
          'Per ora non è stato trovato nulla in questo catalogo, quindi le applicazioni non mostrano alcuna immagine.',
        generated:
          'Nessuna locandina ancora letta da questo catalogo, quindi le applicazioni non mostrano alcuna immagine. Polyfin la compone quando la libreria viene aperta in un’applicazione.',
      },
      noImage: 'Nessuna immagine',
      upload: 'Carica un’immagine',
      uploadAnother: 'Carica un’altra immagine',
      address: 'Oppure l’indirizzo di un’immagine',
      addressHelp: 'Polyfin la scarica una sola volta e ne conserva una copia.',
      useAddress: 'Usa questo indirizzo',
      remove: 'Rimuovi l’immagine',
      removeTitle: (name: string) => `Rimuovere l’immagine di ${name}?`,
      removeToNone:
        'L’immagine caricata viene eliminata. Le applicazioni mostrano questa libreria senza immagine.',
      removeToAutomatic:
        'L’immagine caricata viene eliminata. Le applicazioni mostrano invece l’immagine trovata nel catalogo.',
      removeToGenerated:
        'L’immagine caricata viene eliminata. Le applicazioni mostrano invece l’immagine composta da Polyfin.',
      removeConfirm: 'Rimuovi',
      saved: 'Immagine salvata.',
      removed: 'Immagine rimossa.',
      atOnce: 'Le modifiche all’immagine vengono salvate subito.',
      afterSave: 'Salva prima le librerie, poi scegli l’immagine di questa libreria.',
      close: (name: string) => `Chiudi l’immagine di ${name}`,
    },
    narrow: {
      title: 'Genere e massimo',
      edit: (name: string) => `Genere e massimo di ${name}`,
      close: (name: string) => `Chiudi genere e massimo di ${name}`,
      staged: 'Salvati con il pulsante Salva, come i nomi.',
      genre: 'Genere',
      allGenres: 'Tutti i generi',
      genreHelp: 'Mostra solo i titoli di questo genere.',
      maxItems: 'Numero massimo di titoli',
      noMax: 'Nessun massimo',
      maxHelp: (max: string, where: string) =>
        `Mostra al massimo questo numero di titoli nella libreria, e altrettanti in ciascuna delle sue collezioni, al posto del limite dei cataloghi in ${where}. Da 1 a ${max}. Vuoto: si applica quel limite.`,
      maxHelpMine: (max: string) =>
        `Mostra al massimo questo numero di titoli nella libreria, e altrettanti in ciascuna delle sue collezioni, al posto del limite del server. Da 1 a ${max}. Vuoto: si applica il limite del server.`,
      maxRange: (max: string) => `Inserisci un numero intero da 1 a ${max}, oppure lascia vuoto.`,
      maxBadge: (count: string) => `max ${count}`,
    },
  },
  smartLists: {
    title: 'Liste intelligenti',
    help: 'Liste create da regole sui titoli dei cataloghi del server, come «Film francesi degli anni ’90 non ancora visti». Tutti gli utenti le vedono, entro le librerie che possono vedere.',
    helpMine:
      'Liste create da regole sui titoli dei tuoi cataloghi, e di quelli del server mentre li usi. Solo tu le vedi.',
    add: 'Aggiungi una lista intelligente',
    emptyTitle: 'Ancora nessuna lista intelligente',
    emptyHow:
      'Una lista intelligente mantiene i titoli dei cataloghi scelti che corrispondono alle sue regole, e segue quei cataloghi quando cambiano.',
    loading: 'Caricamento delle liste intelligenti…',
    shownAsLibrary: 'Libreria',
    shownAsCollection: 'Collezione',
    catalogCount: (count: number) => (count === 1 ? '1 catalogo' : `${count} cataloghi`),
    ruleCount: (count: number) => (count === 1 ? '1 regola' : `${count} regole`),
    maxBadge: (count: string) => `max ${count}`,
    newTitle: 'Nuova lista intelligente',
    back: 'Librerie',
    backMine: 'Le mie origini',
    lede: 'Scegli i cataloghi da cui attinge e le regole a cui corrispondono i suoi titoli. L’anteprima mostra i suoi titoli come sono adesso, prima di salvare.',
    notFound: 'Questa lista intelligente non esiste.',
    name: 'Nome',
    nameHelp: 'Il nome mostrato dalle applicazioni Jellyfin.',
    shownAs: 'Mostrata come',
    shownAsChoices: { library: 'Una libreria', collection: 'Una collezione' },
    shownAsHelp: {
      library:
        'Una libreria a sé nelle applicazioni Jellyfin, con la sua riga nella schermata iniziale: una libreria di film o di serie TV quando tutti i suoi cataloghi sono dello stesso tipo.',
      collection:
        'Una collezione nella vista Collezioni, accanto a quelle create dagli utenti. Le applicazioni non possono aggiungere o rimuovere i suoi titoli.',
    },
    catalogs: 'Cataloghi',
    catalogsHelp:
      'I suoi titoli provengono da questi cataloghi, ciascuno letto fino al suo massimo: quello della libreria quando il catalogo è una delle tue librerie, altrimenti Titoli letti per catalogo di film e serie TV.',
    catalogFilter: 'Filtra i cataloghi',
    catalogFilterPlaceholder: 'Nome del catalogo o dell’add-on',
    serverBadge: 'Server',
    noCatalogs: 'Ancora nessun catalogo da cui attingere: aggiungi prima un’origine.',
    noCatalogMatch: 'Nessun catalogo corrisponde a questo filtro.',
    rules: 'Regole',
    rulesHelp: 'Senza regole, la lista mantiene tutti i titoli dei suoi cataloghi.',
    exclude: 'Esclusioni',
    excludeHelp:
      'I titoli che corrispondono a queste regole vengono esclusi, qualunque cosa dicano le regole sopra. Senza esclusioni, nessuno lo è.',
    excludeMatch: 'Escludi i titoli che corrispondono a',
    match: 'I titoli corrispondono a',
    matchChoices: { all: 'Tutte le regole', any: 'Almeno una regola' },
    groupMatch: 'Questo gruppo corrisponde a',
    group: 'Gruppo',
    addRule: 'Aggiungi una regola',
    addGroup: 'Aggiungi un gruppo',
    removeRule: 'Rimuovi questa regola',
    removeGroup: 'Rimuovi questo gruppo',
    field: 'Riguarda',
    fields: {
      genre: 'Genere',
      year: 'Anno',
      rating: 'Voto IMDb',
      language: 'Lingua',
      type: 'Film o serie TV',
      rank: 'Posizione nel suo catalogo',
      state: 'Stato di visione',
      released: 'Data di uscita',
      homeRelease: 'Uscita a casa',
      votes: 'Voti IMDb',
      runtime: 'Durata',
      contentRating: 'Classificazione per età',
      person: 'Attore o regista',
      studio: 'Studio o rete',
    } as Record<string, string>,
    negation: 'Corrisponde o no',
    is: 'è',
    isNot: 'non è',
    genre: 'Genere',
    from: 'Da',
    to: 'A',
    language: 'Lingua',
    rankMax: 'Tra i suoi primi',
    minutes: 'minuti',
    atMost: 'Al massimo',
    releasedWhen: 'Quando',
    releasedWithin: { last: 'Negli ultimi', next: 'Nei prossimi' } as Record<string, string>,
    homeReleaseWithin: {
      released: 'Uscito negli ultimi',
      upcoming: 'Esce nei prossimi',
      both: 'Entro',
    } as Record<string, string>,
    days: 'giorni',
    namePlaceholder: { person: 'Il nome', studio: 'Il nome' } as Record<string, string>,
    types: { movie: 'Un film', series: 'Una serie TV' } as Record<string, string>,
    states: {
      played: 'Visto',
      unplayed: 'Non visto',
      started: 'Iniziato, non finito',
      favorite: 'Preferito',
    } as Record<string, string>,
    stateHelp:
      'Le regole visto, iniziato e preferito seguono ciò che ha fatto ciascun utente: ognuno vede la propria lista.',
    describedHelp:
      'La lingua di un titolo, e la sua durata, la classificazione, le persone e la data di uscita quando il suo catalogo non le dà, arrivano con la sua descrizione, che Polyfin legge in background: un titolo non ancora descritto resta escluso finché non lo è. Un titolo a cui manca ciò che una regola guarda non corrisponde né alla regola né al suo contrario.',
    votesHelp:
      'I voti IMDb vengono da MDBList, con la chiave API MDBList per i voti in Impostazioni › Contenuti, cercati in background. Senza quella chiave, nessun titolo ha voti.',
    studioHelp:
      'Studi e reti vengono da TMDB, con la chiave API TMDB in Impostazioni › Contenuti, cercati in background e conservati: la lista cresce man mano che arrivano. Senza quella chiave, contano solo gli studi che un amministratore ha indicato nei metadati di un titolo.',
    homeReleaseHelp:
      'L’uscita a casa di un film è la sua prima uscita digitale o fisica, altrimenti 90 giorni dopo l’uscita al cinema, secondo TMDB, con la chiave API TMDB in Impostazioni › Contenuti, cercata in background e conservata: la lista cresce man mano che arrivano. Quella di una serie è la sua data di debutto. Con Attendi l’uscita digitale attivo, un film non ancora uscito compare solo nelle liste con una regola di titoli in arrivo.',
    sort: 'Ordina',
    sorts: {
      catalog: 'Come li elencano i suoi cataloghi, uno per ciascuno a turno',
      premiere: 'Data di uscita, dalla più recente',
      latestEpisode: 'Ultimo episodio trasmesso, dal più recente',
      seasonPremiere: 'Ultimo inizio di stagione, dal più recente',
      rating: 'Voto IMDb, dal più alto',
      name: 'Nome',
      random: 'Casuale, estratto di nuovo ogni giorno',
    } as Record<string, string>,
    episodeSortHelp:
      'Le serie seguono il loro ultimo episodio regolare trasmesso, o l’inizio della loro ultima stagione, come dice la loro descrizione, senza speciali né episodi in arrivo; i film la loro data di uscita. Una serie la cui descrizione non è ancora stata letta va in fondo finché non lo è.',
    maxItems: 'Numero massimo di titoli',
    maxHelp: (max: string) => `Da 1 a ${max}. Vuoto: nessun massimo.`,
    noMax: 'Nessun massimo',
    create: 'Crea la lista',
    save: 'Salva',
    created: 'Lista intelligente creata.',
    saved: 'Lista intelligente salvata.',
    delete: 'Elimina',
    deleteTitle: (name: string) => `Eliminare ${name}?`,
    deleteBody:
      'Le applicazioni Jellyfin non la mostrano più. I suoi titoli restano nei rispettivi cataloghi.',
    deleted: 'Lista intelligente eliminata.',
    preview: 'Anteprima',
    previewHelp:
      'I titoli che la lista ti mostra ora. Ogni utente la vede con i propri titoli visti.',
    refresh: 'Aggiorna',
    listed: (count: string) => (count === '1' ? '1 titolo' : `${count} titoli`),
    firstOf: (shown: string, listed: string) => `I primi ${shown} di ${listed}.`,
    unknown: (count: string) =>
      count === '1'
        ? '1 titolo aspetta ancora ciò che guardano le sue regole: la descrizione, i voti o gli studi. La lista cresce man mano che arrivano.'
        : `${count} titoli aspettano ancora ciò che guardano le loro regole: la descrizione, i voti o gli studi. La lista cresce man mano che arrivano.`,
    incomplete:
      'Alcuni cataloghi non hanno risposto in tempo: la lista mostra ciò che è stato letto finora e legge il resto.',
    previewEmpty: 'Per ora nessun titolo corrisponde a queste regole.',
    previewNeedsCatalog: 'Scegli almeno un catalogo per vedere la lista.',
    previewLabel: 'Titoli della lista',
    columns: {
      title: 'Titolo',
      type: 'Tipo',
      year: 'Anno',
      rating: 'IMDb',
      genres: 'Generi',
      languages: 'Lingua',
    },
  },
  /** Episode orders: the order a series shows its seasons and episodes in, for every user. */
  episodeOrders: {
    title: 'Ordini degli episodi',
    help: 'Una serie mostra stagioni ed episodi come li numera la sua estensione, in ordine di messa in onda. Mostrane una in un altro ordine di TMDB, come DVD, assoluto o arco narrativo, per tutti gli utenti, come fa l’ordine di visualizzazione dell’editor dei metadati di jellyfin-web. I suoi episodi mantengono la propria stagione e il proprio numero per la riproduzione, la cronologia e il tracciamento.',
    loading: 'Caricamento degli ordini degli episodi…',
    emptyTitle: 'Tutte le serie sono in ordine di messa in onda',
    emptyHow: 'Cerca una serie qui sotto per mostrarla in un altro ordine.',
    search: 'Trova una serie',
    searchPlaceholder: 'Nome della serie',
    searchButton: 'Cerca',
    searching: 'Ricerca in corso…',
    results: 'Serie trovate',
    noSeries: 'Nessuna serie corrisponde a questo nome.',
    editTitle: (name: string) => `Ordine degli episodi di ${name}`,
    order: 'Ordine',
    orderHelp:
      'I gruppi di episodi che TMDB fornisce per questa serie. Le parti di un gruppo diventano le sue stagioni.',
    aired: 'Messa in onda',
    types: {
      originalAirDate: 'Prima messa in onda',
      absolute: 'Assoluto',
      dvd: 'DVD',
      digital: 'Digitale',
      storyArc: 'Arco narrativo',
      production: 'Produzione',
      tv: 'TV',
    },
    counts: (seasons: number, episodes: number) =>
      `${seasons === 1 ? '1 stagione' : `${seasons} stagioni`}, ${episodes === 1 ? '1 episodio' : `${episodes} episodi`}`,
    lookup: {
      no_key:
        'Aggiungi una chiave TMDB in Impostazioni › Contenuti per elencare gli ordini di questa serie.',
      unknown: 'TMDB non conosce questa serie: resta in ordine di messa in onda.',
      failed:
        'Impossibile raggiungere TMDB per elencare gli ordini di questa serie. Riprova tra un momento.',
    },
    specials: 'Speciali',
    specialsHelp:
      'Dove compare la stagione 0 tra le stagioni. Gli speciali nascosti si riproducono ancora da «Continua a guardare» o da un link.',
    specialsFirst: 'Per primi',
    specialsLast: 'Per ultimi',
    specialsHidden: 'Nascosti',
    specialsSummary: { last: 'Speciali per ultimi', hidden: 'Speciali nascosti' },
    saved: (name: string) => `${name}: ordine degli episodi salvato.`,
  },
}

export default libraries
