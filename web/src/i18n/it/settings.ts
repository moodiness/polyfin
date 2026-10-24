import type en from '../en/settings'
import type { RangeText } from '../en/settings'

const settings: typeof en = {
  settings: {
    tracking: {
      description:
        'Ogni utente può collegare i propri account Trakt, Simkl, MDBList, PublicMetaDB, AniList e MyAnimeList dalla pagina Il mio account, e Polyfin comunica a questi servizi cosa guarda; Last.fm e ListenBrainz ricevono i brani che ascolta. Trakt, Simkl, AniList, MyAnimeList e Last.fm richiedono prima un’applicazione di questo server, da configurare qui. MDBList, PublicMetaDB e ListenBrainz non richiedono nulla.',
      traktSetup:
        'Crea un’applicazione su trakt.tv, in Settings, Your API Apps, New application. Dalle un nome qualsiasi, poi copia qui il suo client ID e il suo client secret.',
      redirectUri: 'Redirect URI da inserire nell’applicazione',
      traktClientId: 'Client ID Trakt',
      traktClientIdHelp:
        'Lascia vuoto per disattivare Trakt. Gli utenti possono collegarsi una volta salvati sia il client ID sia il client secret.',
      traktClientSecret: 'Client secret Trakt',
      traktClientSecretHelp:
        'Mostrato nella pagina dell’applicazione su trakt.tv, sotto il client ID.',
      simklSetup:
        'Crea un’applicazione su simkl.com, in Settings, Developer, Create new app, di tipo «TV, devices & command line» (AUTH V2 di Simkl; non richiede un redirect URI, e i client ID delle vecchie applicazioni AUTH V1 vengono rifiutati). Dalle un nome qualsiasi, poi copia qui il suo client ID.',
      simklClientId: 'Client ID Simkl',
      simklClientIdHelp: 'Lascia vuoto per disattivare Simkl. Non serve alcun client secret.',
      lastFmSetup:
        'Crea un account API su last.fm, all’indirizzo last.fm/api/account/create. Dagli un nome qualsiasi e lascia vuoto il callback URL: gli utenti autorizzano Polyfin su Last.fm, e Polyfin chiede direttamente a Last.fm la loro sessione. Poi copia qui la sua chiave API e il suo shared secret.',
      lastFmApiKey: 'Chiave API Last.fm',
      lastFmApiKeyHelp:
        'Lascia vuoto per disattivare Last.fm. Gli utenti possono collegarsi una volta salvati sia la chiave API sia lo shared secret.',
      lastFmSecret: 'Shared secret Last.fm',
      lastFmSecretHelp:
        'Mostrato nella pagina del tuo account API su last.fm, sotto la chiave API.',
      aniListSetup:
        'Crea un’applicazione su anilist.co, in Settings, Developer, Create New Client. Dalle un nome qualsiasi e, come redirect URL, l’indirizzo qui sotto, dove AniList mostra agli utenti il codice da incollare in Polyfin. Poi copia qui il suo client ID e il suo client secret.',
      aniListClientId: 'Client ID AniList',
      aniListClientIdHelp:
        'Lascia vuoto per disattivare AniList. Gli utenti possono collegarsi una volta salvati sia il client ID sia il client secret.',
      aniListClientSecret: 'Client secret AniList',
      aniListClientSecretHelp:
        'Mostrato nella pagina dell’applicazione su anilist.co, sotto il client ID.',
      myAnimeListSetup:
        'Crea un’applicazione su myanimelist.net, in Account Settings, API, Create ID, di tipo «web». Dalle un nome qualsiasi e, come redirect URL, l’indirizzo qui sotto: MyAnimeList vi rimanda i browser degli utenti dopo che hanno autorizzato Polyfin. Basta che lo raggiungano i loro browser. Poi copia qui il suo client ID e il suo client secret.',
      myAnimeListRedirectHelp:
        'L’indirizzo pubblico di questo server in Impostazioni › Generali, altrimenti l’indirizzo che stai usando ora.',
      myAnimeListClientId: 'Client ID MyAnimeList',
      myAnimeListClientIdHelp:
        'Lascia vuoto per disattivare MyAnimeList. Gli utenti possono collegarsi una volta salvati sia il client ID sia il client secret.',
      myAnimeListClientSecret: 'Client secret MyAnimeList',
      myAnimeListClientSecretHelp:
        'Mostrato nella pagina dell’applicazione su myanimelist.net, sotto il client ID.',
    },
    title: 'Impostazioni',
    description: 'Opzioni che si applicano all’intero server.',
    serverName: 'Nome del server',
    serverNameHelp: (r: RangeText) =>
      `Da ${r.min} a ${r.max} caratteri, mostrato nelle applicazioni Jellyfin.`,
    quickConnect: 'Consenti Quick Connect',
    quickConnectHelp:
      'Permette alle applicazioni per TV e telefono di accedere con un codice di 6 cifre approvato qui.',
    language: 'Lingua dei nomi generati',
    languageHelp:
      'Polyfin assegna da sé alcuni nomi nelle applicazioni Jellyfin: le stagioni («Stagione 1», «Speciali»), gli episodi senza titolo e il tipo aggiunto alle librerie con lo stesso nome («Popolari (Film)»).',
    legacyAuthorization: 'Consenti l’autorizzazione legacy',
    legacyAuthorizationHelp:
      'Accetta le vecchie intestazioni X-Emby-*, il parametro api_key e lo schema Emby per le applicazioni che ne hanno ancora bisogno. Disattivato per impostazione predefinita, come in Jellyfin 12.3.',
    updateCheck: 'Controlla nuove versioni',
    updateCheckHelp:
      'Una volta al giorno, e poco dopo l’avvio, Polyfin chiede a GitHub la sua ultima release, inviando solo la propria versione. Una versione più recente compare nella Home e in Sistema › Stato di salute, e viene inviata alle notifiche Nuova versione. Se disattivato, Polyfin non contatta mai GitHub.',
    localDiscovery: 'Rispondi al rilevamento sulla rete locale',
    localDiscoveryHelp:
      'Le applicazioni Jellyfin ed Emby che cercano server sulla rete locale trovano Polyfin e lo elencano, così non serve digitarne l’indirizzo. Chiedono sulla porta UDP 7359, e Polyfin risponde solo agli indirizzi della rete locale. Si applica subito, senza riavvio. In Docker serve la rete host o la porta pubblicata 7359/udp: consulta la documentazione di installazione.',
    localAddress: 'Indirizzo sulla rete locale',
    localAddressHelp:
      'L’indirizzo con cui le applicazioni della rete locale raggiungono Polyfin, ad esempio http://192.168.1.10:8096. Le risposte al rilevamento lo forniscono, e le applicazioni Jellyfin lo leggono come indirizzo locale del server. Se vuoto, le risposte forniscono l’indirizzo dell’interfaccia di rete che raggiunge l’applicazione, che è corretto a meno che Polyfin non giri in una rete bridge di Docker: in tal caso, inserisci l’indirizzo dell’host e la porta di Polyfin.',
    torrents: {
      description:
        'Alcuni add-on elencano torrent anziché link. Polyfin può riprodurli: entra nello swarm di un torrent solo mentre una riproduzione lo legge, e instrada tutto il suo traffico su un unico percorso, un’interfaccia di rete come quella di una VPN, un proxy SOCKS5 o un tunnel WireGuard che Polyfin gestisce da sé. Nulla viene mai inviato per un’altra via. I pezzi vengono conservati entro lo Spazio su disco per i file in lettura, in Impostazioni › Riproduzione.',
      play: 'Riproduci stream torrent',
      playHelp:
        'Disattivato per impostazione predefinita. Riprodurre un torrent lo scarica da altri peer e condivide quanto scaricato finché il limite di upload lo consente: sei tu a rispondere di ciò che questo server scarica e condivide. Gli utenti devono anche avere Può riprodurre stream torrent, nella loro pagina in Utenti. Richiede un percorso.',
      route: 'Percorso',
      routeHelp:
        'Ogni socket dei torrent, per peer, tracker e DHT, passa solo da qui. Quando non è disponibile, le versioni torrent vengono saltate, anche durante una riproduzione, e Sistema › Stato di salute ne indica il motivo.',
      routeHelpNoInterface:
        'Ogni socket dei torrent, per peer, tracker e DHT, passa solo da qui. Questo sistema non vincola socket a un’interfaccia di rete, cosa che fa solo Linux, come in Polyfin Desktop per Windows e macOS: un proxy SOCKS5 o un tunnel WireGuard trasporta i torrent. Quando non è disponibile, le versioni torrent vengono saltate, anche durante una riproduzione, e Sistema › Stato di salute ne indica il motivo.',
      routes: {
        interface: 'Interfaccia di rete',
        proxy: 'Proxy SOCKS5',
        wireguard: 'Tunnel WireGuard',
      },
      interface: 'Interfaccia di rete per i torrent',
      interfaceHelp:
        'Il suo nome, ad esempio wg0. Ogni socket dei torrent viene vincolato a essa, e i nomi dei tracker vengono risolti attraverso di essa. Solo Linux. In Docker, il container deve vedere l’interfaccia: consulta la documentazione sui torrent.',
      proxy: 'Proxy SOCKS5 per i torrent',
      proxyHelp:
        'Il suo indirizzo e la sua porta, ad esempio 10.0.0.2:1080. Peer e tracker HTTP passano tramite il suo CONNECT. I tracker UDP, il DHT e uTP richiedono il suo UDP ASSOCIATE, e senza di esso restano disattivati. Attraverso un proxy, nessun peer può connettersi in ingresso.',
      proxyUser: 'Utente del proxy',
      proxyUserHelp: 'Vuoto se il proxy non lo richiede.',
      proxyPassword: 'Password del proxy',
      proxyPasswordHelp: 'Memorizzata cifrata quando POLYFIN_SECRET_KEY è impostata.',
      wireGuard: 'Configurazione WireGuard',
      wireGuardHelp:
        'Incolla il file .conf che il tuo provider VPN fornisce per WireGuard. Polyfin gestisce il tunnel da sé, solo per i torrent, senza nulla da configurare su questo sistema. I nomi dei tracker vengono risolti attraverso il tunnel dai suoi server DNS, mai in altro modo. Nessun peer può connettersi dall’esterno. Salvata cifrata quando POLYFIN_SECRET_KEY è impostata, e mai più mostrata.',
      wireGuardSaved: (endpoint: string, addresses: string) =>
        `Una configurazione è salvata: server ${endpoint}, indirizzi ${addresses}.`,
      wireGuardDns: (servers: string) =>
        `I nomi dei tracker vengono risolti da ${servers}, attraverso il tunnel.`,
      wireGuardNoDns:
        'Non indica alcun server DNS: i tracker indicati da un nome host non vengono raggiunti, quelli indicati da un indirizzo sì.',
      wireGuardReplace: 'Incolla un’altra configurazione per sostituire quella salvata.',
      wireGuardInvalid: (problem: { line: number; key: string; reason: string }) => {
        const where = problem.line > 0 ? `Riga ${problem.line}: ` : ''
        const expected: Record<string, string> = {
          PrivateKey: 'una chiave di 32 byte in base64',
          PublicKey: 'una chiave di 32 byte in base64',
          PresharedKey: 'una chiave di 32 byte in base64',
          Address: 'indirizzi IP, come 10.2.0.2/32, separati da virgole',
          DNS: 'indirizzi IP separati da virgole',
          MTU: 'un numero da 576 a 65535',
          ListenPort: 'una porta da 0 a 65535',
          Endpoint: 'un nome host o un indirizzo IP con una porta, come vpn.example.com:51820',
          AllowedIPs:
            'indirizzi IP con la lunghezza del prefisso, come 0.0.0.0/0, separati da virgole',
          PersistentKeepalive: 'off, oppure un numero di secondi da 0 a 65535',
        }
        switch (problem.reason) {
          case 'too_long':
            return 'La configurazione WireGuard deve essere un testo di al massimo 8192 byte.'
          case 'syntax':
            return `${where}ogni riga deve essere una [sezione], una Chiave = Valore o un commento #.`
          case 'outside':
            return `${where}${problem.key} viene prima di [Interface] o [Peer].`
          case 'unknown':
            return `${where}${problem.key} non fa parte di una configurazione WireGuard.`
          case 'repeated':
            return problem.key === 'Peer'
              ? `${where}è supportata una sola sezione [Peer].`
              : `${where}${problem.key} è indicato due volte.`
          case 'missing':
            return `${where}${problem.key} manca nella configurazione WireGuard.`
          case 'no_ipv4':
            return `${where}Address richiede un indirizzo IPv4, usato dai tracker UDP e dalla DHT.`
          case 'not_all':
            return `${where}AllowedIPs richiede 0.0.0.0/0: i peer di un torrent sono ovunque.`
          default:
            return `${where}${problem.key} deve essere ${expected[problem.key] ?? 'valido'}.`
        }
      },
      downloadLimit: 'Limite di download (Mbps, 0 = nessuno)',
      downloadLimitHelp: (r: RangeText) =>
        `Da 0 a ${r.max}; ${r.default} per impostazione predefinita.`,
      uploadLimit: 'Limite di upload (Mbps, 0 = nessuna condivisione)',
      uploadLimitHelp: (r: RangeText) =>
        `I peer favoriscono chi condivide a sua volta, quindi con 0 l’avvio può essere più lento. La condivisione si interrompe al termine della riproduzione. Da 0 a ${r.max}; ${r.default} per impostazione predefinita.`,
    },
    dlna: {
      title: 'DLNA',
      description:
        'TV, ricevitori, console e lettori della rete locale che non hanno un’applicazione Jellyfin possono sfogliare e riprodurre le librerie tramite DLNA. Un dispositivo non può accedere: ognuno mostra le librerie dell’utente scelto per esso, con i limiti di quell’utente.',
      share: 'Condividi le librerie tramite DLNA',
      shareHelp:
        'Annuncia Polyfin ai dispositivi della rete locale sulla porta UDP 1900 e risponde solo a loro. Si applica subito, senza riavvio. In Docker serve la rete dell’host o una rete che dia al container un proprio indirizzo: vedi la documentazione DLNA.',
      name: 'Nome sulla rete',
      nameHelp: 'Il nome con cui i dispositivi elencano Polyfin. Se vuoto, il nome del server.',
      newDevicesUser: 'Mostra ai nuovi dispositivi le librerie di',
      newDevicesUserHelp:
        'L’utente di cui un dispositivo visto per la prima volta mostra le librerie. Con «Nessuno», un nuovo dispositivo vede un server vuoto finché non scegli un utente per esso qui sotto.',
      noOne: 'Nessuno',
      markPlayed: 'Segna come visto ciò che riproducono i dispositivi DLNA',
      markPlayedHelp:
        'I dispositivi non comunicano la posizione: un titolo conta come visto quando il dispositivo ne ha letto più di «Segna come visto dopo (%)». I dispositivi leggono in anticipo, quindi un titolo può essere segnato troppo presto.',
      devices: 'Dispositivi',
      devicesEmpty: 'Nessun dispositivo ha ancora richiesto nulla.',
      device: 'Dispositivo',
      address: 'Indirizzo',
      model: 'Modello',
      lastSeen: 'Ultimo accesso',
      showsLibrariesOf: 'Mostra le librerie di',
      blocked: 'Bloccato',
      profile: 'Profilo',
      automaticProfile: (name: string) => `Automatico (${name})`,
      forget: 'Dimentica',
      profiles: 'Profili',
      profilesHelp:
        'I profili usano il formato XML del plugin DLNA di Jellyfin. Un dispositivo riceve il primo che corrisponde a ciò che invia, prima i profili aggiunti, altrimenti quello generico.',
      builtin: 'Integrato',
      addProfile: 'Aggiungi un profilo',
      replaceProfile: 'Sostituisci',
      removeProfile: 'Rimuovi',
      profileXml: 'XML del profilo',
      devicesHelp:
        'I dispositivi della rete locale che hanno richiesto qualcosa a Polyfin, dall’ultimo visto, ciascuno riconosciuto dal suo indirizzo. Le modifiche si applicano subito.',
      devicesLabel: 'Dispositivi DLNA',
      automatic: 'Automatico',
      deviceSaved: (name: string) => `${name} salvato.`,
      forgetTitle: (name: string) => `Dimenticare ${name}?`,
      forgetBody:
        'Polyfin dimentica l’utente, il blocco e il profilo scelti per esso. Se richiede di nuovo qualcosa, è un nuovo dispositivo e mostra le librerie dell’utente scelto per i nuovi dispositivi.',
      forgotten: (name: string) => `${name} dimenticato.`,
      profilesLabel: 'Profili DLNA',
      added: 'Aggiunto',
      viewXml: 'Visualizza XML',
      profileXmlHelp:
        'Incolla l’XML di un profilo creato per il plugin DLNA di Jellyfin, oppure apri il suo file.',
      openFile: 'Apri un file',
      add: 'Aggiungi',
      adding: 'Aggiunta…',
      replacing: 'Sostituzione…',
      profileAdded: (name: string) => `Profilo ${name} aggiunto.`,
      profileReplaced: (name: string) => `Profilo ${name} sostituito.`,
      removeTitle: (name: string) => `Rimuovere il profilo ${name}?`,
      removeBody:
        'I dispositivi a cui è imposto ricevono di nuovo un profilo scelto in base a ciò che inviano.',
      profileRemoved: (name: string) => `Profilo ${name} rimosso.`,
    },
    metrics: 'Esponi le metriche',
    metricsHelp:
      'Fornisce le metriche di Polyfin a Prometheus su /metrics: riproduzioni, conversioni, richieste agli add-on, importazioni, notifiche, richieste HTTP, scansioni, database, spazio su disco e processo. Non indicano alcun utente, titolo, indirizzo o dispositivo. Prometheus invia il token delle metriche con ogni richiesta; senza di esso, /metrics risponde 401, e finché questa opzione è disattivata, 404.',
    metricsToken: 'Token delle metriche',
    metricsTokenHelp:
      'Creato da Polyfin la prima volta che le metriche vengono attivate. Prometheus lo invia come token bearer: consulta la documentazione di amministrazione per una configurazione di raccolta.',
    metricsTokenPending: 'Polyfin crea il token delle metriche al salvataggio.',
    newMetricsToken: 'Crea un nuovo token',
    newMetricsTokenTitle: 'Creare un nuovo token delle metriche?',
    newMetricsTokenConfirm:
      'Il token attuale smette subito di funzionare: Prometheus ha bisogno di quello nuovo per leggere le metriche.',
    newMetricsTokenMade: 'Nuovo token delle metriche creato',
    legacyWarningTitle: 'Avviso di sicurezza',
    legacyWarning:
      'I metodi legacy possono inviare le credenziali negli URL, che finiscono nei log, nella cronologia del browser e nei proxy. Attiva questa opzione solo se un’applicazione che usi non riesce ad accedere in altro modo.',
    prepareAhead: 'Prepara la riproduzione in anticipo',
    prepareAheadHelp:
      'Polyfin legge il file appena si apre la pagina di un titolo e prepara l’episodio successivo verso la fine di quello in corso, così la riproduzione parte subito. Elenca inoltre in anticipo le versioni dei titoli in «Continua a guardare» e «Prossimo», così le loro pagine le mostrano subito. Questo invia qualche richiesta in più alle tue origini, anche per titoli aperti ma non riprodotti.',
    transcoding: 'Conversione (transcodifica)',
    transcodingHelp:
      'Ricodifica video e audio per le applicazioni che non riescono a riprodurre un file così com’è. Se disattivata, le applicazioni riproducono i file così come sono o semplicemente reimpacchettati senza ricodifica, e un titolo che un’applicazione non può riprodurre in questo modo non partirà su quell’applicazione. Questo vale anche per le registrazioni convertite in HEVC, sulle applicazioni che non riproducono HEVC.',
    analysisTimeout: 'Tempo massimo per analizzare una versione',
    analysisTimeoutHelp: (r: RangeText) =>
      `Prima di una prima riproduzione, Polyfin analizza il file o il canale per sapere come riprodurlo. Se l’origine non risponde entro questo numero di secondi, Polyfin rinuncia a quella versione e passa alla successiva. Un numero più basso passa oltre prima, ma può rinunciare a origini lente che avrebbero funzionato. Un canale viene analizzato per 8 secondi al massimo. Da ${r.min} a ${r.max} secondi; ${r.default} per impostazione predefinita.`,
    versionAttempts: 'Versioni provate quando una non funziona',
    versionAttemptsHelp: (r: RangeText) =>
      `Quando un’applicazione riproduce un titolo senza scegliere una versione, Polyfin prova le versioni in ordine finché una funziona, ma ne analizza al massimo questo numero. Vengono provate anche le versioni già analizzate, perché non costano nulla. Un numero più alto trova più spesso una versione funzionante, ma un titolo che non si riproduce impiega più tempo a segnalarlo. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    preferDirectPlay: 'Preferisci le versioni che l’applicazione riproduce senza conversione',
    preferDirectPlayHelp:
      'Quando un’applicazione riproduce un titolo senza scegliere una versione, Polyfin sceglie la prima versione che l’applicazione riproduce così com’è o semplicemente reimpacchettata, anziché la prima che si riproduce comunque. La primissima riproduzione di un titolo può essere un po’ più lenta, perché potrebbero essere analizzate più versioni; non cambia nulla una volta che sono note.',
    nameHowVersionsPlay: 'Indica nei nomi delle versioni come vengono riprodotte',
    nameHowVersionsPlayHelp:
      'I nomi delle versioni terminano con il modo in cui l’applicazione riproduce ciascuna: «(riproduzione diretta)», «(stream diretto)» quando viene copiata senza conversione, «(convertita: motivo)» con HDR, il limite di qualità, i sottotitoli, il video o l’audio come motivo, oppure «(non riproducibile qui)». Polyfin lo stabilisce in base al profilo del dispositivo inviato dall’applicazione alla sua ultima riproduzione, quindi i nomi non indicano nulla prima della prima riproduzione di un dispositivo o dopo un riavvio, né per una versione non ancora analizzata o descritta da RemuxDB. I nomi sono nella lingua dei nomi generati. Per questo non viene letto nulla.',
    versionOrderGroup: 'Ordine delle versioni',
    versionOrder: {
      label: 'Ordina le versioni per',
      help: 'Ogni criterio raggruppa le versioni di un film o di un episodio, e quello successivo le ordina all’interno di ciascun gruppo; le versioni a pari merito mantengono l’ordine degli add-on. La prima versione è quella avviata da Riproduci, e le versioni provate quando non funziona vengono subito dopo. Polyfin si basa su ciò che sa di una versione senza leggere il file: la sua analisi dopo la riproduzione, altrimenti la descrizione di RemuxDB, altrimenti le parole delle sue etichette. Una versione di cui un criterio non sa nulla viene dopo le altre. La lingua audio preferita è quella di ciascun utente, impostata nella sua applicazione: prima le versioni note in quella lingua, poi quelle etichettate MULTI o DUAL, poi quelle di lingua sconosciuta. Finché la pagina di un titolo è aperta, le versioni mostrate mantengono la loro posizione.',
      none: 'Nessuno: le versioni mantengono l’ordine degli add-on.',
      add: 'Aggiungi un criterio',
      remove: (name: string) => `Rimuovi «${name}»`,
      criterion: (position: number) => `Criterio ${position}`,
      criteria: {
        language: 'Lingua audio preferita',
        'resolution-high': 'Risoluzione, prima la più alta',
        'resolution-low': 'Risoluzione, prima la più bassa',
        'size-large': 'Dimensione del file, prima la più grande',
        'size-small': 'Dimensione del file, prima la più piccola',
        codec: 'Codec video, nell’ordine sottostante',
        'hdr-first': 'Prima HDR e Dolby Vision',
        'hdr-last': 'HDR e Dolby Vision per ultimi',
        source: 'Origine, nell’ordine sottostante',
      },
    },
    versionCodecOrder: {
      label: 'Ordine dei codec video',
      help: 'L’ordine in cui il criterio «Codec video» dispone le versioni.',
    },
    versionSourceOrder: {
      label: 'Ordine delle origini',
      help: 'L’ordine in cui il criterio «Origine» dispone le versioni: gli add-on e le cartelle del server che forniscono versioni di film ed episodi. Un’origine aggiunta in seguito va in fondo finché non la sposti, e gli add-on personali degli utenti vengono dopo le origini del server. Cambia solo l’ordine delle versioni: descrizioni, sottotitoli e risultati di ricerca seguono l’ordine di Contenuti › Origini.',
      none: 'Nessuna origine del server fornisce ancora versioni di film o episodi.',
      dragHandle: (name: string) => `Trascina ${name}`,
    },
    versionCodecs: {
      av1: 'AV1',
      hevc: 'HEVC (H.265)',
      vp9: 'VP9',
      h264: 'H.264 (AVC)',
      vc1: 'VC-1',
      mpeg4: 'MPEG-4 Parte 2 (Xvid, DivX)',
      mpeg2video: 'MPEG-2',
    },
    hideVersionsLarger: 'Nascondi le versioni più grandi di (GB, 0 = nessun limite)',
    hideVersionsLargerHelp: (r: RangeText) =>
      `Le versioni che risultano più grandi non vengono elencate da nessuna parte: né nelle pagine dei titoli, né durante la riproduzione, né nei download. Se questa regola e quelle sottostanti nascondessero tutte le versioni di un titolo, nessuna viene nascosta. Fino a ${r.max} GB; ${r.default} per impostazione predefinita.`,
    hiddenVersionCodecs: 'Nascondi questi codec video',
    hiddenVersionCodecsHelp:
      'Le versioni che risultano in questi codec non vengono elencate da nessuna parte.',
    hiddenVersionWords: 'Nascondi le versioni le cui etichette contengono',
    hiddenVersionWordsHelp: (r: RangeText) =>
      `Una parola o frase per riga, cercata senza distinzione tra maiuscole e minuscole nel nome, nella descrizione e nel nome del file di una versione: per gli add-on che indicano nelle etichette che un file non è ancora pronto per la riproduzione. Fino a ${r.max} righe; nessuna per impostazione predefinita.`,
    syncAddonSubtitles: 'Sincronizza i sottotitoli degli add-on',
    syncAddonSubtitlesHelp:
      'Allinea i sottotitoli degli add-on alla versione riprodotta, quando sono in anticipo, in ritardo, o più veloci o più lenti. Alla prima riproduzione di una versione, Polyfin individua in background i momenti con parlato: da un sottotitolo testuale nel file, altrimenti da tre tratti del suo audio, leggendo al massimo 300 MB del file di un add-on. Le versioni torrent sono escluse, perché andrebbero scaricate dal loro swarm. Da quel momento, un sottotitolo che si adatta chiaramente meglio se spostato o accelerato viene fornito sincronizzato, con il nome che termina con «(sincronizzato)».',
    remuxDb: 'Descrivi le versioni con RemuxDB',
    remuxDbHelp:
      'Mostra le tracce audio, dei sottotitoli e video di una versione prima della sua prima riproduzione, come RemuxDB le ha trovate nello stesso file. Quando si aprono i dettagli di un titolo, Polyfin interroga RemuxDB, un database della community, tramite l’identificativo IMDb del titolo, e individua le versioni dai nomi dei file. La riproduzione analizza comunque ogni versione.',
    remuxDbUrl: 'Indirizzo di RemuxDB',
    remuxDbUrlHelp: 'Il server RemuxDB interrogato da Polyfin.',
    playbackHistoryGroup: 'Cronologia di riproduzione',
    playbackHistory: 'Mantieni una cronologia di riproduzione',
    playbackHistoryHelp:
      'Conserva ogni video riprodotto, con chi l’ha riprodotto, su quale applicazione e come, per Sistema › Statistiche e per le statistiche di ciascun utente in Il mio account. Se disattivato, non viene conservato più nulla; ciò che è già conservato resta finché non è troppo vecchio.',
    playbackHistoryDays: 'Giorni di conservazione della cronologia di riproduzione',
    playbackHistoryDaysHelp: (r: RangeText) =>
      `Le riproduzioni più vecchie vengono eliminate. Il controllo avviene ogni giorno. Da ${r.min} a ${r.max}, ${r.default} per impostazione predefinita.`,
    publicAddress: 'Indirizzo pubblico',
    publicAddressHelp:
      'L’indirizzo da cui si apre Polyfin, ad esempio https://media.example.org. I link nelle notifiche iniziano con questo indirizzo; se vuoto, i messaggi non contengono link. Quando inizia con https://, «Reindirizza HTTP a HTTPS», in Impostazioni › Sicurezza, punta alla sua porta, 443 se non ne indica una.',
    email: 'Email',
    emailHelp:
      'Le destinazioni email passano da questo server SMTP. Si possono aggiungere dopo aver salvato un server e un indirizzo del mittente.',
    smtpHost: 'Server SMTP',
    smtpHostHelp:
      'Il suo nome host, ad esempio smtp.example.org. Se vuoto, non è possibile aggiungere destinazioni email.',
    smtpPort: 'Porta SMTP',
    smtpPortHelp: 'Di solito 587 con STARTTLS e 465 con TLS dall’inizio.',
    smtpSecurity: 'Crittografia SMTP',
    smtpSecurityHelp:
      'Come viene crittografata la connessione. Con STARTTLS, non viene inviato nulla se il server non lo supporta. Senza crittografia, la password viene inviata solo a questa macchina.',
    smtpSecurities: { starttls: 'STARTTLS', tls: 'TLS dall’inizio', none: 'Nessuna' },
    smtpUser: 'Utente SMTP',
    smtpUserHelp: 'Lascia vuoto per inviare senza accedere.',
    smtpPassword: 'Password SMTP',
    smtpPasswordHelp: 'Viene conservata crittografata e non viene più mostrata.',
    smtpFrom: 'Indirizzo del mittente',
    smtpFromHelp: 'L’indirizzo da cui provengono i messaggi, ad esempio polyfin@example.org.',
    smtpFromName: 'Nome del mittente',
    smtpFromNameHelp: 'Mostrato con l’indirizzo. Se vuoto, viene mostrato il nome del server.',
    weeklySummary: 'Riepilogo settimanale',
    weeklySummaryHelp:
      'Le destinazioni che hanno scelto «Riepilogo settimanale» nei loro Eventi lo ricevono una volta a settimana: le destinazioni del server e degli amministratori secondo la settimana del server, le altre secondo la propria. Non viene inviato nulla per una settimana senza novità. Sistema › Pianificazione lo invia subito.',
    weeklySummaryDay: 'Invia il riepilogo settimanale il',
    weeklySummaryDayHelp: (day: string) => `${day} per impostazione predefinita.`,
    weeklySummaryHour: 'Invia il riepilogo settimanale alle',
    weeklySummaryHourHelp: (hour: string) =>
      `Nel fuso orario del server. Anche il mese in sintesi e l’anno in sintesi vengono inviati a quest’ora, il giorno 1. ${hour} per impostazione predefinita.`,
    maxConversions: 'Conversioni video contemporanee (0 = nessun limite)',
    maxConversionsHelp: (r: RangeText) =>
      `Convertire il video è il lavoro più pesante che svolge il server. Quando questo numero di riproduzioni ha il video convertito (inclusi i sottotitoli impressi nell’immagine), una nuova riproduzione riceve una versione che non richiede conversione, oppure non parte. Le riproduzioni già in corso non vengono mai interrotte. Anche la Diretta TV mantiene i propri limiti: 4 canali per utente e 16 per il server. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    maxConversionHeight: 'Qualità massima del video convertito',
    maxConversionHeightHelp:
      'Il video convertito viene ridotto al massimo a questa altezza, mantenendone le proporzioni, così da essere riprodotto bene anche su una connessione più lenta. I file riprodotti così come sono o semplicemente reimpacchettati mantengono la loro qualità. Una scheda grafica converte fino al 4K; il processore si ferma a 1080p e, quando converte da HDR a SDR, a «Qualità massima dell’HDR convertito dal processore», in Conversione (transcodifica) › HDR.',
    conversionHeightOriginal: 'Originale',
    conversionHeight: (height: number) => `${height}p`,
    convertedDownloads: 'Download convertiti',
    convertedDownloadsHelp:
      'Le applicazioni che lo richiedono, come Streamyfin, salvano un titolo nella qualità scelta dall’utente: Polyfin invia una copia più leggera, creata mentre l’applicazione la scarica e mai conservata sul server. Altrimenti, salvano il file così com’è. Ogni download conta in «Conversioni video contemporanee» mentre viene convertito, e come una delle «Riproduzioni contemporanee» dell’utente. L’utente deve avere «Può scaricare» e, quando il video o l’audio vengono convertiti, «Può usare la conversione (transcodifica)». Un download interrotto ricomincia da capo. Disattivato per impostazione predefinita.',
    conversion: {
      description:
        'Quando un’applicazione non riesce a riprodurre un file così com’è, Polyfin lo converte con FFmpeg. Queste impostazioni stabiliscono come, per film, serie TV e Diretta TV. I valori predefiniti vanno bene per la maggior parte dei server.',
      groups: {
        general: 'Generale',
        subtitles: 'Sottotitoli in immagini',
        gpu: 'Scheda grafica',
        video: 'Video',
        hdr: 'HDR',
        interlaced: 'Video interlacciato',
        audio: 'Audio',
        performance: 'Prestazioni',
      },
      subtitleOcr: 'Converti in testo i sottotitoli in immagini',
      subtitleOcrHelp:
        'I sottotitoli PGS e DVD (VobSub) sono immagini, e molte applicazioni accettano solo testo: per queste, Polyfin imprime la traccia nel video, il che richiede una conversione. Con questa opzione attiva, Tesseract legge una volta ogni traccia in immagini, in background, creandone una copia in testo, elencata come traccia aggiuntiva contrassegnata «(OCR)», che si riproduce senza conversione. Finché la copia non è pronta, la traccia viene impressa come prima. La traccia in immagini resta: corsivi, colori e posizioni vanno persi, e i caratteri stilizzati possono essere letti male. «Durante la riproduzione» legge le tracce di una versione la prima volta che un’applicazione che non sa visualizzarle la riproduce; «Anche dopo le scansioni delle librerie» legge anche le tracce dei nuovi file locali. Disattivato per impostazione predefinita.',
      subtitleOcrChoices: {
        off: 'Disattivato',
        played: 'Durante la riproduzione',
        scans: 'Anche dopo le scansioni delle librerie',
      },
      subtitleOcrTracks: 'Tracce di sottotitoli in immagini lette contemporaneamente',
      subtitleOcrTracksHelp: (r: RangeText) =>
        `Ogni traccia viene letta da un processo Tesseract che usa un core del processore. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
      hardwareAcceleration: 'Scheda grafica usata per la conversione',
      hardwareAccelerationHelp:
        'Una scheda grafica (GPU) converte i video molto più velocemente del processore. Ogni sistema offre le proprie: NVIDIA e VAAPI su Linux, VideoToolbox su macOS, NVIDIA, Intel e AMD su Windows. Senza una scheda funzionante, converte il processore. Una modifica si applica appena salvata, senza riavvio.',
      vaapiDevice: 'Scheda grafica per VAAPI',
      vaapiDeviceHelp:
        'Il nodo di rendering su cui VAAPI converte i video, quando più schede grafiche potrebbero farlo. Per impostazione predefinita, ognuna a turno: la prima che funziona.',
      vaapiDeviceEach: 'Ognuna a turno',
      vaapiDeviceMissing: (path: string) => `${path} (non trovato)`,
      hardware: {
        auto: 'Automatico: la prima che funziona, nell’ordine seguente',
        nvenc: 'NVIDIA (NVENC)',
        vaapi: 'AMD o Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
        none: 'Nessuna: solo il processore',
      },
      detected: 'Rilevato su questo server',
      noGpu: 'Non viene usata alcuna scheda grafica: i video vengono convertiti dal processore.',
      gpu: 'Scheda grafica',
      gpuNames: {
        cuda: 'NVIDIA',
        vaapi: 'AMD o Intel (VAAPI)',
        videotoolbox: 'VideoToolbox',
        qsv: 'Intel (QSV)',
        amf: 'AMD (AMF)',
      } as Record<string, string>,
      encoders: 'Codificatori sulla scheda',
      gpuToneMapping: 'Da HDR a SDR sulla scheda',
      processorToneMappingHint:
        'L’HDR viene quindi convertito dal processore, fino a «Qualità massima dell’HDR convertito dal processore».',
      qualityFactor: 'Accetta un valore di qualità',
      processor: 'Codificatori sul processore',
      processorToneMapping: 'Da HDR a SDR sul processore',
      yes: 'Sì',
      no: 'No',
      none: 'Nessuno',
      hardwareDecoding: 'Leggi questi formati sulla scheda grafica',
      hardwareDecodingHelp:
        'La scheda legge (decodifica) da sé questi formati, lasciando libero il processore. Deselezionane uno se i suoi video non si convertono o appaiono errati: lo leggerà il processore. Gli altri formati vengono tentati sulla scheda. HEVC 10 bit richiede HEVC.',
      hardwareDecodingNoGpu:
        'Non viene usata alcuna scheda grafica, quindi il processore legge tutti i formati.',
      codecs: {
        h264: 'H.264',
        hevc: 'HEVC',
        hevc_10bit: 'HEVC 10 bit',
        vp9: 'VP9',
        av1: 'AV1',
        mpeg2video: 'MPEG-2',
        vc1: 'VC-1',
      },
      encoderPreset: 'Velocità di codifica',
      encoderPresetHelp:
        'Più lenta dà un’immagine migliore a parità di dimensioni, ma richiede più potenza. Se i video convertiti scattano, scegline una più veloce. Automatico mantiene la scelta di Polyfin: molto veloce sul processore, media sulle schede NVIDIA, quella della scheda sulle altre.',
      presets: {
        auto: 'Automatico',
        veryslow: 'Molto lenta (immagine migliore)',
        slower: 'Più lenta',
        slow: 'Lenta',
        medium: 'Media',
        fast: 'Veloce',
        faster: 'Più veloce',
        veryfast: 'Molto veloce',
        superfast: 'Super veloce',
        ultrafast: 'Ultra veloce (carico minimo)',
      },
      h264Quality: 'Qualità H.264 (0 = in base al bitrate)',
      hevcQuality: 'Qualità HEVC (0 = in base al bitrate)',
      qualityHelp: (r: RangeText) =>
        `Un numero più basso offre un’immagine migliore e più dati; il bitrate resta comunque il massimo. 0 punta solo al bitrate. Jellyfin usa 23 per H.264 e 28 per HEVC. 0, oppure da ${r.min} a ${r.max}; ${r.default} come predefinito.`,
      qualityIgnored:
        'Questa scheda grafica non accetta un numero di qualità: continua a puntare al bitrate.',
      allowHevcEncoding: 'Consenti la conversione in HEVC',
      allowHevcEncodingHelp:
        'HEVC richiede meno dati di H.264 a parità di immagine, ma più potenza per crearlo. Se attivo, le applicazioni che indicano HEVC per primo lo ricevono. Se disattivato, ricevono HEVC solo le applicazioni che non riescono a riprodurre H.264.',
      noHevcEncoder: 'FFmpeg non può creare HEVC su questo server.',
      adaptiveQuality: 'Più qualità durante la conversione',
      adaptiveQualityHelp:
        'I video convertiti offrono anche fino a due qualità inferiori, ad esempio 720p e 480p sotto 1080p. Le applicazioni in grado di cambiare qualità passano a queste quando la connessione rallenta, invece di fermarsi a caricare. Una qualità abbandonata dall’applicazione continua a essere convertita per 10 secondi, così che tornarci non riparta da zero, e tutte contano come una sola conversione. I video copiati, la Diretta TV e la musica mantengono una sola qualità. «Rete locale» ha lo stesso significato che per i PIN di reimpostazione della password, attraverso i proxy noti. Disattivato come predefinito.',
      adaptiveQualities: {
        off: 'Disattivato',
        remote: 'Fuori dalla rete locale',
        always: 'Sempre',
      },
      toneMapping: 'Converti HDR in SDR (tone mapping)',
      toneMappingHelp:
        'Mantiene corretti i colori e la luminosità dei video HDR sugli schermi che mostrano solo SDR. Se disattivato, i video HDR convertiti appaiono sbiaditi e i video Dolby Vision senza livello HDR10 non vengono convertiti.',
      toneMappingUnavailable:
        'Né la scheda grafica né FFmpeg possono farlo su questo server: i video HDR vengono convertiti solo se questa opzione è disattivata.',
      toneMappingAlgorithm: 'Metodo di tone mapping',
      toneMappingAlgorithmHelp:
        'Come le parti luminose vengono ricondotte a SDR. Automatico usa BT.2390 sulle schede NVIDIA e Hable sul processore. Le schede Intel e VideoToolbox usano un proprio metodo e ignorano questa scelta.',
      algorithms: {
        auto: 'Automatico',
        bt2390: 'BT.2390 (solo schede NVIDIA)',
        hable: 'Hable',
        reinhard: 'Reinhard',
        mobius: 'Möbius',
        clip: 'Taglio',
        linear: 'Lineare',
      },
      gpuToneMappingSetting: 'Esegui il tone mapping HDR sulla scheda grafica',
      gpuToneMappingSettingHelp:
        'Le schede NVIDIA, le schede Intel (tramite VAAPI su Linux, QSV su Windows) e VideoToolbox su macOS convertono da sole HDR in SDR, fino a 4K. Disattiva questa opzione se i loro colori sembrano sbagliati: se ne occuperà il processore, con una qualità inferiore. Le schede Intel accettano solo video HDR10 che includono le informazioni del display di mastering, e VideoToolbox HDR10 e HLG; il processore converte gli altri, e il Dolby Vision senza livello HDR10 viene convertito solo sulle schede NVIDIA. Le schede AMD non lo fanno mai: il loro driver Linux ha fallito o si è bloccato in ogni modo tentato da Polyfin, e il filtro AMF di FFmpeg su Windows non esegue il tone mapping.',
      gpuToneMappingUnavailable:
        'La scheda grafica di questo server non può farlo: è il processore a convertire HDR in SDR.',
      processorToneMappingHeight: 'Qualità massima dell’HDR convertito dal processore',
      processorToneMappingHeightHelp:
        'Convertire HDR in SDR è un lavoro pesante per il processore: circa quattro volte la codifica stessa a 1080p. Automatico misura questo server all’avvio: 1080p se converte almeno una volta e mezza più velocemente della riproduzione del video, altrimenti 720p, usato anche finché la misurazione non termina. Valori sopra 1080p contano solo quando è una scheda grafica a codificare il video: da solo, il processore si ferma a 1080p.',
      toneMappingHeightAuto: 'Automatico (misurato all’avvio)',
      height4k: '4K',
      measuredHeight: (height: number) => `Misurato: ${height}p`,
      upToHeight: (height: number) => `fino a ${height}p`,
      toneMappingPeak: 'Luminosità di picco in nit (0 = dal video)',
      toneMappingDesat: 'Desaturazione delle alte luci (0 = disattivata)',
      toneMappingPeakHelp: (peak: RangeText, desat: RangeText) =>
        `Solo quando è il processore a convertire HDR in SDR: una scheda grafica che lo fa li ignora. Il picco sostituisce il livello più luminoso che il video dichiara di raggiungere: 0, oppure da ${peak.min} a ${peak.max}; ${peak.default} come predefinito. La desaturazione attenua il colore delle parti molto luminose: da ${desat.min} a ${desat.max}; ${desat.default} come predefinito.`,
      processorCannotToneMap:
        'FFmpeg non può eseguire il tone mapping sul processore di questo server.',
      deinterlaceMethod: 'Metodo di deinterlacciamento',
      deinterlaceMethodHelp:
        'Le trasmissioni TV e i DVD sono spesso interlacciati, il che mostra righe a pettine una volta convertiti. Yadif è veloce; Bwdif è un po’ più nitido.',
      noBwdif: 'FFmpeg su questo server non dispone di Bwdif.',
      deinterlacers: { yadif: 'Yadif', bwdif: 'Bwdif' },
      deinterlaceDoubleRate: 'Raddoppia la frequenza dei fotogrammi',
      deinterlaceDoubleRateHelp:
        'Crea un’immagine da ogni semiquadro, per movimenti più fluidi negli sport e nei programmi TV. Solo per video fino a 30 fotogrammi al secondo.',
      downmixAlgorithm: 'Missaggio in stereo',
      downmixAlgorithmHelp:
        'Come l’audio surround viene ridotto a due altoparlanti. Dave750 e Modalità notte mantengono chiare le voci; RFC 7845 e AC-4 seguono degli standard.',
      downmixes: {
        None: 'Missaggio di FFmpeg',
        Dave750: 'Dave750',
        NightmodeDialogue: 'Modalità notte (voci più chiare)',
        Rfc7845: 'RFC 7845',
        Ac4: 'AC-4',
      },
      downmixBoost: 'Volume nel missaggio in stereo',
      downmixBoostHelp: (r: RangeText) =>
        `I missaggi stereo spesso suonano più bassi: il volume viene moltiplicato per questo numero. Da ${r.min} a ${r.max}; ${r.default} (nessuna modifica) come predefinito. Jellyfin usa 2.`,
      maxAudioChannels: 'Numero massimo di canali audio',
      maxAudioChannelsHelp:
        'L’audio convertito mantiene al massimo questo numero di canali, anche se l’applicazione ne accetta di più.',
      audioChannels: (channels: number): string =>
        channels === 0
          ? 'Quanti ne accetta l’applicazione'
          : channels === 1
            ? 'Mono'
            : channels === 2
              ? 'Stereo'
              : '5.1',
      audioBitratePerChannel: 'Bitrate audio per canale in kb/s (0 = automatico)',
      audioBitratePerChannelHelp: (r: RangeText) =>
        `Automatico assegna i bitrate di Jellyfin: 128 kb/s in mono, 384 kb/s in stereo e 640 kb/s in 5.1, meno con un limite di bitrate basso. 0, oppure da ${r.min} a ${r.max}; ${r.default} come predefinito.`,
      encodingThreads: 'Thread del processore per conversione (0 = automatico)',
      encodingThreadsHelp: (r: RangeText) =>
        `Limita quanta parte del processore usa una conversione, per lasciare spazio ad altre attività. Da ${r.min} a ${r.max}; 0 lascia scegliere a FFmpeg.`,
      aheadSeconds: 'Secondi preparati in anticipo',
      aheadSecondsHelp: (r: RangeText) =>
        `Polyfin prepara un video al massimo per questo numero di secondi oltre la parte richiesta dall’applicazione, poi attende. Un valore più alto aiuta con le origini lente e mantiene attiva la loro connessione, ma usa più potenza e spazio su disco quando si smette di guardare presto. Da ${r.min} a ${r.max}; ${r.default} come predefinito.`,
      keepConvertedVideo: 'Mantieni il video convertito quando cambia la traccia audio',
      keepConvertedVideoHelp:
        'Converte immagine e audio separatamente, così scegliere un’altra traccia audio nel lettore prepara solo il nuovo audio: l’immagine già convertita continua e la nuova traccia parte prima. L’immagine viene mantenuta per 30 secondi dopo che un lettore la interrompe e nel frattempo conta come una conversione.',
    },
    catalogsTitle: 'Cataloghi',
    catalogLimit: 'Titoli letti per catalogo di film e serie TV',
    catalogLimitHelp: (r: RangeText) =>
      `Alcuni cataloghi sono quasi infiniti, quindi Polyfin smette di leggere un catalogo dopo questo numero di titoli. Un numero più alto mostra più titoli, ma le liste si caricano più lentamente e l’add-on riceve più richieste. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    channelLimit: 'Canali letti per catalogo di Diretta TV',
    channelLimitHelp: (r: RangeText) =>
      `Polyfin smette di leggere un catalogo di Diretta TV dopo questo numero di canali, e legge al massimo questo numero di programmi al giorno per la guida. Un numero più alto mostra più contenuti, ma il caricamento è più lento e l’add-on riceve più richieste. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    skipButtons: 'Pulsanti «Salta intro» e «Salta titoli di coda»',
    skipButtonsHelp:
      'Le applicazioni propongono di saltare intro, riassunti e titoli di coda, individuati in database della community. Se disattivato, le applicazioni non mostrano pulsanti per saltare e i database non vengono interpellati.',
    publicMetaDbKey: 'Chiave PublicMetaDB',
    publicMetaDbKeyHelp:
      'Facoltativa. Una chiave API PublicMetaDB aggiunge una terza origine di marcatori per saltare.',
    theIntroDbKey: 'Chiave TheIntroDB (facoltativa)',
    theIntroDbKeyHelp:
      'Aumenta il limite giornaliero di TheIntroDB e include i tuoi contributi. La lettura funziona anche senza.',
    segmentSources: {
      label: 'Origini dei marcatori per saltare',
      help: 'Per ogni tipo di passaggio (intro, riassunto, titoli di coda, anteprima), vince la prima origine attiva in questo elenco che lo conosce; le successive colmano le lacune. Le origini disattivate non vengono mai interpellate.',
      turnOn: (name: string) => `Interpella ${name}`,
      needsKey: 'Richiede una chiave',
    },
    audioIntros: 'Trova intro e titoli di coda nell’audio degli episodi locali',
    audioIntrosHelp:
      'Quando un episodio di una cartella locale viene riprodotto senza intro o titoli di coda forniti dai suoi capitoli o dalle origini qui sopra, Polyfin confronta in background l’audio degli episodi della sua stagione, e assegna a ciascuno il tratto che hanno in comune vicino all’inizio come intro, e quello che hanno in comune vicino alla fine come titoli di coda. Gli altri episodi della stagione li hanno subito, e quello riprodotto li avrà dalla riproduzione successiva. Richiede i pulsanti per saltare attivi.',
    similarTitles: 'Titoli simili',
    similarTitlesHelp:
      'La pagina di un titolo elenca titoli affini, trovati nei cataloghi degli add-on. Se disattivato, l’elenco è vuoto e gli add-on ricevono meno richieste.',
    generateImages: 'Genera immagini per le collezioni che non ne hanno',
    generateImagesHelp:
      'Una collezione di un add-on senza locandina, e una lista intelligente mostrata come collezione, mostrano le locandine dei loro primi titoli con il loro nome, disegnate da Polyfin dopo averne letto i titoli. Gli utenti con controllo parentale o generi bloccati non le vedono. Se disattivato, non mostrano alcuna immagine. Le librerie impostate su un’immagine generata la mantengono.',
    tmdbKey: 'Chiave API TMDB',
    tmdbKeyHelp:
      'Facoltativa. Con una chiave API o un token di accesso in lettura di un account TMDB, «Modifica immagini» di jellyfin-web cerca anche su TMDB locandine, sfondi e loghi di un titolo, fotogrammi di un episodio, locandine di una stagione e foto di una persona, che Polyfin inoltra. Senza chiave, «Modifica immagini» elenca solo le immagini degli add-on e TMDB non viene mai interpellato.',
    mdblistRatingsKey: 'Chiave API MDBList per i voti',
    mdblistRatingsKeyHelp:
      'Facoltativa. Con una chiave di un account MDBList, film e serie TV mostrano il voto IMDb, il voto della critica e il voto del pubblico da MDBList, recuperati in background e conservati per tutti gli utenti. Le chiavi MDBList personali degli utenti per il tracciamento non vengono mai usate a questo scopo. Senza chiave, MDBList non viene mai interpellato.',
    criticRatingShows: 'Il voto della critica mostra',
    criticRatingShowsHelp:
      'Le applicazioni Jellyfin hanno un solo campo per il voto della critica, da 0 a 100: mostra il voto della critica o quello del pubblico. Nella pagina di un titolo, il lettore web li mostra entrambi, con il voto IMDb.',
    criticRatingChoices: { critics: 'Voto della critica', audience: 'Voto del pubblico' },
    lyrics: 'Testi dei brani da LRCLIB',
    lyricsHelp:
      'Le applicazioni mostrano i testi dei brani, sincronizzati riga per riga quando LRCLIB, un database gratuito di testi, li ha. Polyfin cerca un brano una sola volta, per artista, titolo, album e durata, quando inizia la riproduzione o se ne aprono i dettagli o il testo. Se disattivato, i brani non hanno testi e LRCLIB non viene mai interpellato.',
    evenVolume: 'Uniforma il volume tra i brani',
    evenVolumeHelp:
      'Le applicazioni alzano o abbassano ogni brano, o ogni album, allo stesso volume percepito, in base ai tag ReplayGain dei brani locali o al loro volume misurato. I brani stessi non vengono modificati. Se disattivato, le applicazioni non ricevono alcun guadagno e non viene misurato nulla.',
    measureLoudness: 'Misura il volume dei brani senza guadagno',
    measureLoudnessHelp:
      'FFmpeg misura il volume dei brani privi di tag ReplayGain, due alla volta con la priorità più bassa, una sola volta ciascuno: i brani locali dopo una scansione, quelli degli add-on dopo la prima riproduzione. I brani più lunghi di 15 minuti e gli audiolibri non vengono mai misurati.',
    measureLoudnessChoices: {
      off: 'Disattivato',
      local: 'Brani locali',
      all: 'Brani locali e degli add-on',
    },
    playedPercent: 'Segnato come visto dopo (%)',
    playedPercentHelp: (r: RangeText) =>
      `Un titolo viene segnato come visto quando la riproduzione supera questa percentuale della sua durata. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita, come in Jellyfin.`,
    resumePercent: 'Punto di ripresa conservato dopo (%)',
    resumePercentHelp: (r: RangeText) =>
      `Il punto in cui si è interrotta la riproduzione viene conservato, per riprendere da lì, una volta superata questa percentuale della durata del titolo. Deve essere inferiore alla soglia di visto. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita, come in Jellyfin. Un titolo più breve di 5 minuti viene segnato come visto appena supera questo punto.`,
    versionListMinutes: 'Conserva gli elenchi delle versioni per (minuti)',
    versionListMinutesHelp: (r: RangeText) =>
      `Per quanto tempo Polyfin usa le versioni e i sottotitoli che gli add-on elencano per un titolo prima di interpellarli di nuovo. Un elenco più vecchio viene comunque mostrato subito quando il titolo viene riaperto, mentre Polyfin aggiorna. Conservare gli elenchi più a lungo invia meno richieste all’add-on degli stream, il che aiuta con i provider che rifiutano troppe richieste, ma le nuove versioni compaiono più tardi. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    catalogRefreshMinutes: 'Aggiorna i cataloghi dopo (minuti)',
    catalogRefreshMinutesHelp: (r: RangeText) =>
      `Quanto può invecchiare una pagina di catalogo letta da un add-on, guida della Diretta TV inclusa, prima che Polyfin la rilegga. Le applicazioni non aspettano mai: una pagina più vecchia viene comunque mostrata subito mentre Polyfin la rilegge in background, e viene conservata un giorno in più. Un tempo più lungo invia meno richieste agli add-on, ma i nuovi titoli compaiono più tardi. Da ${r.min} a ${r.max} (un giorno); ${r.default} per impostazione predefinita.`,
    collectionReadHour: 'Leggi tutte le collezioni ogni giorno',
    collectionReadHourNever: 'Mai',
    collectionReadHourHelp:
      'Legge a quest’ora tutte le collezioni delle librerie di collezioni del server, una dopo l’altra, poi i cataloghi delle liste intelligenti del server, così che si aprano subito. Richiede molte pagine agli add-on: lascialo disattivato per un add-on che condividi con altri.',
    collectionLibraryApps: 'Applicazioni che mostrano le librerie di collezioni per collezione',
    collectionLibraryAppsHelp:
      'Le librerie di collezioni vengono presentate alle applicazioni come librerie di contenuti misti, così che jellyfin-web e Streamyfin diano loro una riga nella schermata iniziale. Alle applicazioni selezionate viene comunicato che contengono collezioni, come fa la vista Collezioni di Jellyfin: le applicazioni che elencano una libreria in base al tipo mostrano allora le sue collezioni, e i titoli di una collezione una volta aperta, anziché i titoli di tutte le sue collezioni insieme. jellyfin-web e Streamyfin escludono tali librerie dalle righe della schermata iniziale, quindi non selezionarle. L’elenco mostra le applicazioni che hanno effettuato l’accesso a questo server.',
    collectionLibraryAppsNone:
      'Nessuna applicazione ha ancora effettuato l’accesso a questo server.',
    localScanHours: 'Scansiona le cartelle locali ogni (ore, 0 = mai)',
    localScanHoursHelp: (r: RangeText) =>
      `Ogni quanto ciascuna cartella locale viene scansionata di nuovo per individuare file aggiunti, modificati o rimossi. Le cartelle vengono scansionate anche all’avvio, su richiesta e quando quelle monitorate cambiano. 0 disattiva la pianificazione. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    watchLocalFolders: 'Monitora le modifiche di cartelle e condivisioni',
    watchLocalFoldersHelp: {
      container:
        'Scansiona di nuovo una cartella del container pochi secondi dopo che i suoi file smettono di cambiare, così che i file aggiunti, rinominati o rimossi compaiano entro un minuto.',
      computer:
        'Scansiona di nuovo una cartella del computer su cui gira Polyfin pochi secondi dopo che i suoi file smettono di cambiare, così che i file aggiunti, rinominati o rimossi compaiano entro un minuto.',
    },
    watchSharesHelp:
      'Una condivisione SMB viene scansionata di nuovo pochi secondi dopo che il suo server segnala una modifica. Le cartelle WebDAV vengono rielencate ogni 2 minuti, i bucket S3, le cartelle SFTP e i remote rclone con la frequenza richiesta da ciascuna cartella, una condivisione di molte cartelle in più passaggi, e vengono scansionate di nuovo quando un elenco è cambiato. Sistema › Stato di salute indica come viene monitorata ogni condivisione, quali cartelle o condivisioni non possono esserlo e perché.',
    personalAddons: 'Consenti gli add-on personali degli utenti',
    personalAddonsHelp:
      'Permette agli utenti di aggiungere add-on Stremio propri, oltre a quelli del server. Se disattivato, i loro add-on vengono conservati ma non usati, e le loro applicazioni Jellyfin mostrano solo gli add-on del server.',
    serverImports:
      'Gli utenti possono importare la cronologia di visione da un altro server o applicazione',
    serverImportsHelp:
      'In Il mio account, ogni utente può importare i film e gli episodi visti, i punti di ripresa e i preferiti da un server Jellyfin o Emby, o dal proprio account Stremio o Nuvio, accedendovi con le proprie credenziali. Se disattivato, queste sezioni sono nascoste. Un amministratore può comunque importare per conto di un utente, in Utenti.',
    importEveryMinutes: 'Importa gli account collegati ogni',
    importEveryMinutesHelp:
      'Ogni quanto Polyfin importa di nuovo gli account Stremio e Nuvio a cui gli utenti restano collegati e le cronologie di visione che importano dai servizi di tracciamento. Una modifica si applica subito: ogni importazione successiva avviene a questa distanza dall’ultima. Ogni importazione rilegge l’intero account, quindi un intervallo breve invia più richieste a quei servizi e, per Stremio, i cui termini vietano l’estrazione automatizzata, rende più marcata quella zona grigia. 6 ore per impostazione predefinita.',
    shareLinks: 'Consenti i link di condivisione',
    shareLinksHelp:
      'Gli utenti con «Può condividere titoli», nella loro pagina in Utenti, possono dare un film o un episodio a chi non ha un account, tramite un link che scade dopo un periodo da 1 ora a 30 giorni. L’ospite guarda in un browser, al massimo in 1080p ed entro il gruppo di qualità dell’utente, e conta subito tra le riproduzioni dell’utente. Se disattivato, non è possibile creare link e ogni link viene rifiutato, senza essere eliminato. Disattivato per impostazione predefinita.',
    importEveryChoice: (minutes: number) => (minutes < 60 ? `${minutes} min` : `${minutes / 60} h`),
    loginAttempts: 'Blocca un account dopo questo numero di password errate (0 = mai)',
    loginAttemptsHelp: (r: RangeText) =>
      `Dopo questo numero di password errate consecutive, l’account non può accedere per 15 minuti, nemmeno con la password corretta. Un amministratore può sbloccarlo prima dalla pagina Utenti. 0, oppure da ${r.min} a ${r.max}.`,
    twoStepAdministrators: 'Richiedi l’accesso in due passaggi per gli amministratori',
    twoStepAdministratorsHelp:
      'Gli amministratori accedono a questa applicazione di amministrazione con un codice di un’applicazione di autenticazione dopo la password. Chi non l’ha ancora configurato lo fa subito dopo la password, prima di ottenere una sessione. Da quel momento la sola password non consente l’accesso a nessuna applicazione Jellyfin: si usa Quick Connect, approvato qui. Le sessioni e i dispositivi già connessi restano connessi. Disattivato per impostazione predefinita.',
    inactiveDeviceDays: 'Disconnetti i dispositivi inutilizzati da (giorni, 0 = mai)',
    inactiveDeviceDaysHelp: (r: RangeText) =>
      `Le applicazioni Jellyfin non usate per questo numero di giorni vengono disconnesse e devono accedere di nuovo. Il controllo avviene ogni ora. L’accesso a questa pagina di amministrazione non è interessato. Da ${r.min} a ${r.max}.`,
    knownProxies: 'Proxy conosciuti',
    knownProxiesHelp: (r: RangeText) =>
      `I reverse proxy davanti a Polyfin, uno per riga: un indirizzo IP, una rete come 172.18.0.0/16 o un nome host come il nome di un container su una rete Docker (traefik), fino a ${r.max}. Un nome viene risolto ogni minuto e ogni indirizzo a cui corrisponde è considerato attendibile, quindi elenca solo nomi risolti dalla tua rete. Una richiesta da uno di essi proviene dal client indicato dalla sua intestazione X-Forwarded-For o X-Real-IP: è quell’indirizzo che limita gli accessi non riusciti, compare nei dispositivi e determina la rete locale. Le intestazioni inoltrate da qualsiasi altro indirizzo vengono ignorate. Lascia vuoto se davanti a Polyfin non c’è nulla.`,
    https: {
      title: 'HTTPS',
      intro:
        'Polyfin può servire HTTPS da solo, su una porta dedicata, senza un reverse proxy davanti. HTTP continua a funzionare sulla sua porta qualunque cosa accada a HTTPS: una porta che non si può aprire, o un certificato che non si può leggere o rinnovare, compare in Sistema › Stato di salute. Le modifiche si applicano subito, senza riavvio.',
      mode: 'Servi HTTPS',
      modeHelp:
        'File del certificato: un certificato ottenuto da un altro strumento, riletto quando cambia. Let’s Encrypt: un certificato che Polyfin ottiene e rinnova da solo, per nomi host pubblici che puntano a questo server. Disattivato per impostazione predefinita.',
      modes: { off: 'Disattivato', files: 'File del certificato', letsencrypt: 'Let’s Encrypt' },
      port: 'Porta HTTPS',
      portHelp: (r: RangeText) =>
        `La porta TCP su cui HTTPS è in ascolto nel container: pubblicala, come la 8096 per HTTP. L’utente del container non può restare in ascolto sotto la 1024: per raggiungere Polyfin sulla 443, inoltra la 443 a questa porta sul router o nelle porte del container e imposta l’Indirizzo pubblico, in Impostazioni › Notifiche, su https:// e il nome host, senza porta. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita, come Jellyfin.`,
      redirect: 'Reindirizza HTTP a HTTPS',
      redirectHelp:
        'Le richieste da internet che arrivano in HTTP ricevono un reindirizzamento permanente (308) allo stesso indirizzo in HTTPS, sulla porta da cui internet raggiunge HTTPS: la porta dell’Indirizzo pubblico, in Impostazioni › Notifiche, quando inizia con https://, la 443 quando non ne indica nessuna, altrimenti la porta HTTPS. La rete locale, i controlli di Let’s Encrypt e le richieste ricevute in HTTPS da un proxy conosciuto restano in HTTP. Si applica solo mentre HTTPS è servito. Disattivato per impostazione predefinita.',
      certificate: 'Certificato',
      certificateHelp:
        'Il percorso nel container del file PEM del certificato con la sua catena, come /certs/fullchain.pem. Al salvataggio viene verificato che sia leggibile. Polyfin lo rilegge entro un minuto da una modifica e conserva l’ultimo certificato valido finché una modifica non è leggibile.',
      privateKey: 'Chiave privata',
      privateKeyHelp:
        'Il percorso nel container della chiave privata RSA o ECDSA del certificato, come file PEM, ad esempio /certs/privkey.pem. Al salvataggio viene verificato che corrisponda al certificato.',
      hosts: 'Nomi host',
      hostsHelp: (r: RangeText) =>
        `I nomi pubblici che puntano a questo server, uno per riga, come media.example.org, fino a ${r.max}. Un unico certificato li contiene tutti. I nomi risolti solo sulla rete locale, e i caratteri jolly, richiedono la verifica DNS-01, a cui Polyfin non risponde: per questi usa file del certificato ottenuti da un altro strumento.`,
      email: 'Email',
      emailHelp:
        'Facoltativo. Let’s Encrypt potrebbe scrivere a questo indirizzo riguardo all’account.',
      terms: 'Accetto i termini di servizio di Let’s Encrypt',
      termsHelp:
        'Let’s Encrypt rilascia certificati solo dopo che i suoi termini di servizio, su letsencrypt.org/repository, sono stati accettati.',
      challenge: 'Verifica',
      challengeHelp:
        'Il modo in cui Let’s Encrypt verifica che i nomi puntino a questo server. HTTP-01: Polyfin risponde sulla sua porta HTTP, che la porta 80 da internet deve raggiungere. TLS-ALPN-01: Polyfin risponde sulla sua porta HTTPS, che la porta 443 da internet deve raggiungere. Il certificato, la sua chiave e la chiave dell’account sono conservati nel database, le chiavi cifrate con POLYFIN_SECRET_KEY, e il certificato viene rinnovato quando gli resta un terzo della sua validità.',
      challenges: {
        'http-01': 'HTTP-01 (porta 80 verso la porta HTTP)',
        'tls-alpn-01': 'TLS-ALPN-01 (porta 443 verso la porta HTTPS)',
      },
      useAddress: (address: string) => `Usa ${address}`,
      offeredAddress:
        'Gli indirizzi su cui Let’s Encrypt serve HTTPS, in Impostazioni › Sicurezza: senza porta una volta che il router inoltra la 443 alla porta HTTPS. «Reindirizza HTTP a HTTPS» porta alla porta dell’indirizzo scelto.',
    },
    detailedLog: 'Registro dettagliato (per diagnosticare un problema)',
    detailedLogHelp:
      'Polyfin scrive molto di più nel suo registro, subito e senza riavvio. Disattivalo una volta trovato il problema.',
    saved: 'Impostazioni salvate.',
    thumbnailsHelp:
      'Immagini ricavate dai titoli stessi, dopo che sono stati guardati. Per crearle, Polyfin legge piccole parti del file dall’origine, con calma, quando nessuno sta riproducendo da quell’origine: al massimo 60 richieste per titolo, una ogni 3 secondi, e 120 all’ora per origine, mai l’intero file. Un’origine che chiede a Polyfin di rallentare non riceve richieste di immagini per 2 ore. Disattivato per impostazione predefinita.',
    trickplay: 'Miniature durante lo scorrimento di un titolo',
    trickplayHelp:
      'Le applicazioni mostrano una piccola immagine del punto raggiunto nella barra di riproduzione. Polyfin le crea dopo che un titolo è stato guardato, e per l’episodio successivo quando la riproduzione viene preparata in anticipo. I film lunghi hanno miniature più distanziate: a qualche minuto anziché a qualche secondo l’una dall’altra.',
    trickplayInterval: 'Una miniatura ogni (secondi, almeno)',
    trickplayIntervalHelp: (r: RangeText) =>
      `Ogni quanto la barra di riproduzione cambia immagine. Polyfin legge al massimo 60 immagini per titolo, quindi un titolo più lungo di circa 10 minuti ha un’immagine per intervallo, con gli intervalli distribuiti uniformemente sulla durata: circa 1 minuto per un episodio di 1 ora, 2 minuti per un film di 2 ore. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita, come Jellyfin.`,
    trickplayWidth: 'Larghezza delle miniature',
    trickplayWidthHelp:
      'Le miniature più larghe sono più nitide sugli schermi grandi, ma occupano più spazio. Modificandola, nuove miniature vengono create alla successiva riproduzione di un titolo.',
    pixels: (width: number) => `${width} pixel`,
    chapterImages: 'Immagini dei capitoli',
    chapterImagesHelp:
      'Le applicazioni mostrano un’immagine per ogni capitolo di un titolo, nell’elenco delle scene. Polyfin usa l’immagine letta più vicina all’inizio di ogni capitolo, dalle stesse letture delle miniature, dopo che il titolo è stato guardato.',
    thumbnailStorage: 'Spazio per le immagini (GB)',
    thumbnailStorageHelp: (r: RangeText) =>
      `Le miniature e le immagini dei capitoli sono conservate nel database. Oltre questa dimensione, vengono rimosse le immagini dei titoli guardati meno di recente; vengono ricreate se il titolo viene riprodotto di nuovo. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    cacheSize: 'Spazio su disco per i file in lettura (GB)',
    cacheSizeHelp: (r: RangeText) =>
      `Spazio che Polyfin può usare per conservare le parti dei file in riproduzione, così che spostamenti e riprese non le scarichino di nuovo. Le parti lette negli ultimi 30 secondi vengono conservate anche oltre questo limite. Da ${r.min} a ${r.max} GB; ${r.default} per impostazione predefinita.`,
    recordingsTitle: 'Registrazioni',
    recordingsFolder: (folder: string) => `Le registrazioni vengono salvate in ${folder}.`,
    recording: 'Registra la Diretta TV',
    recordingHelp:
      'Consente agli utenti di pianificare registrazioni di programmi e serie TV della Diretta TV, salvate nella cartella qui sotto.',
    recordingsFolderLabel: 'Cartella delle registrazioni',
    recordingsFolderHelp: {
      container:
        'Una cartella del container di Polyfin, quella mostrata se il campo è vuoto, nel suo volume dati. Per conservare le registrazioni su un altro disco, montalo lì, oppure altrove e inserisci qui il suo percorso. Polyfin deve poterci scrivere. Modificarla non sposta le registrazioni già effettuate.',
      computer:
        'Una cartella del computer su cui gira Polyfin, quella mostrata se il campo è vuoto, nella sua cartella dati. Per conservare le registrazioni su un altro disco, inserisci qui una cartella di quel disco. Polyfin deve poterci scrivere. Modificarla non sposta le registrazioni già effettuate.',
    },
    recordingPrePadding: 'Inizia le registrazioni in anticipo (minuti)',
    recordingPrePaddingHelp: (r: RangeText) =>
      `Quanti minuti prima del programma inizia una registrazione. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    recordingPostPadding: 'Continua a registrare dopo la fine (minuti)',
    recordingPostPaddingHelp: (r: RangeText) =>
      `Per quanti minuti dopo il programma prosegue una registrazione. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    recordingRetentionDays: 'Conserva le registrazioni per (giorni, 0 = per sempre)',
    recordingRetentionDaysHelp: (r: RangeText) =>
      `Le registrazioni più vecchie vengono eliminate. Il controllo avviene ogni giorno. Da ${r.min} a ${r.max}.`,
    recordingCommercials: 'Trova la pubblicità nelle registrazioni',
    recordingCommercialsHelp:
      'Al termine di una registrazione, Polyfin la analizza in background, una registrazione alla volta, cercando i momenti al tempo stesso neri e silenziosi, e considera i brevi tratti tra di essi come interruzioni pubblicitarie. Le applicazioni propongono quindi di saltarle, secondo quanto scelto da ogni utente nelle impostazioni di riproduzione della propria applicazione, quando i pulsanti per saltare intro e titoli di coda sono attivi. Le registrazioni più lunghe di 4 ore non vengono analizzate.',
    convertRecordings: 'Converti in HEVC le registrazioni completate',
    convertRecordingsHelp:
      'Le registrazioni completate vengono convertite in HEVC, una alla volta, negli orari indicati sotto, così da occupare meno spazio. L’immagine mantiene le sue dimensioni; audio e sottotitoli vengono copiati così come sono. La conversione usa la «Scheda grafica usata per convertire», con la «Qualità HEVC» e il «Metodo di deinterlacciamento» di Conversione (transcodifica), e conta come una delle «Conversioni video simultanee»: attende finché le riproduzioni le occupano tutte. Inizia solo se c’è spazio libero per una copia grande quanto la registrazione. La copia sostituisce la registrazione solo se ha la stessa durata, contiene le stesse tracce, si decodifica fino alla fine ed è più piccola; altrimenti viene eliminata, e la registrazione resta com’è e non viene ritentata. Una registrazione in riproduzione viene sostituita quando la riproduzione si interrompe; lo stato di visione, i punti di ripresa e la pubblicità restano. Le applicazioni che non riproducono HEVC ricevono la registrazione convertita durante la riproduzione: con «Conversione (transcodifica)» disattivata, non possono riprodurla. Disattivato per impostazione predefinita.',
    convertRecordingsBetween: 'Converti tra le',
    convertRecordingsAnd: 'e le',
    convertRecordingsBetweenHelp: (from: string, until: string) =>
      `Nel fuso orario del server. Una conversione ancora in corso alla fine si interrompe e riparte da capo la notte successiva. Dalle ${from} alle ${until} per impostazione predefinita.`,
    convertRecordingsFrom: 'Inizio delle conversioni',
    convertRecordingsUntil: 'Fine delle conversioni',
    convertRecordingsOnProcessor: 'Usa il processore se non c’è una scheda grafica',
    convertRecordingsOnProcessorHelp:
      'Senza una scheda grafica in grado di codificare HEVC, le registrazioni vengono convertite dal processore con x265, alla priorità più bassa. È lento e tiene occupato il processore per tutti gli orari indicati sopra. Senza scheda grafica e con questa opzione disattivata, non viene convertito nulla. Disattivato per impostazione predefinita.',
    liveTvRefreshHours: 'Aggiorna elenchi e guide della Diretta TV ogni (ore)',
    liveTvRefreshHoursHelp: (r: RangeText) =>
      `Ogni quanto vengono scaricati di nuovo gli elenchi canali IPTV e le guide dei programmi XMLTV. Un download non riuscito viene ritentato prima: dopo 5 minuti, 15 minuti, poi ogni ora. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    backupsFolder: (folder: string) => `I backup del database vengono salvati in ${folder}.`,
    backups: 'Esegui il backup del database ogni giorno',
    backupsHelp:
      'Scrive ogni giorno una copia del database nella cartella indicata sotto, all’ora scelta, ed elimina le più vecchie oltre il numero da conservare.',
    backupFolderLabel: 'Cartella dei backup',
    backupFolderHelp: {
      container:
        'Una cartella del container di Polyfin, quella mostrata se il campo è vuoto, nel suo volume dati. I backup sono più sicuri su un disco diverso da quello del database: montane uno lì, oppure altrove e inserisci qui il suo percorso. Polyfin deve poterci scrivere.',
      computer:
        'Una cartella del computer su cui gira Polyfin, quella mostrata se il campo è vuoto, nella sua cartella dati. I backup sono più sicuri su un disco diverso da quello del database: inserisci qui una cartella di un disco simile. Polyfin deve poterci scrivere.',
    },
    backupHour: 'Esegui il backup ogni giorno alle',
    backupHourHelp: (hour: string) =>
      `Nel fuso orario del server. ${hour} per impostazione predefinita.`,
    backupsKept: 'Backup da conservare',
    backupsKeptHelp: (r: RangeText) =>
      `Dopo ogni backup, Polyfin elimina i suoi backup più vecchi oltre questo numero. Gli altri file nella cartella non vengono mai toccati. Da ${r.min} a ${r.max}; ${r.default} per impostazione predefinita.`,
    lastBackup: 'Ultimo backup',
    backupDestinations: 'Destinazioni',
    backupDestinationsHelp:
      'Ogni backup viene inviato anche a queste destinazioni, con lo stesso nome file, dopo essere stato scritto nella cartella: archivi compatibili S3, server SFTP o WebDAV. Un caricamento non riuscito viene ritentato al backup successivo.',
    webPlayerHelp:
      'Ciò che il lettore web (jellyfin-web, su /web/) mostra oltre alle proprie pagine. Lo script si applica al successivo caricamento di una pagina del lettore web; il lettore web conserva il CSS e il messaggio di accesso per un massimo di un minuto.',
    openWebPlayer: 'Apri il lettore web',
    webTheme: 'Tema',
    webThemeHelp:
      'L’aspetto del lettore web. LumaaGlaass è il tema incluso in Polyfin. Nessuno mantiene l’aspetto di jellyfin-web, il cui tema di visualizzazione ogni utente sceglie in Impostazioni › Visualizzazione. Personalizzato usa il tuo CSS e il tuo JavaScript qui sotto, conservati mentre è scelto un altro tema.',
    webThemeChoices: { lumaaglaass: 'LumaaGlaass', none: 'Nessuno', custom: 'Personalizzato' },
    webThemeReload:
      'Il lettore web conserva il CSS personalizzato letto fino a un minuto: dopo una modifica, ricarica le sue pagine aperte un minuto dopo.',
    webThemeTexts: {
      lumaaglaass:
        'Ciò che Polyfin serve per LumaaGlaass, mostrato qui e non modificabile. Scegli Personalizzato, o «Modifica come personalizzato», per scrivere il tuo.',
      none: 'Nessuno non serve CSS né JavaScript personalizzati. Scegli Personalizzato per scrivere il tuo.',
    },
    editAsCustom: 'Modifica come personalizzato',
    editAsCustomTitle: 'Sostituire i tuoi testi personalizzati?',
    editAsCustomConfirm:
      'Il tuo CSS e il tuo JavaScript personalizzati vengono sostituiti da quelli di LumaaGlaass, e il tema diventa Personalizzato. Salvare la pagina conserva la modifica.',
    themeCopy: 'Conserva una copia in Polyfin',
    themeCopyHelp: (depth: number, file: string, total: string, files: number) =>
      `Salva i fogli di stile, gli script, i caratteri e le immagini che i testi personalizzati caricano da altrove, come un tema su un CDN, nella cartella dei dati di Polyfin, e fa caricare i file da Polyfin: il tema funziona allora senza raggiungere il suo CDN. Segue @import, url() e gli indirizzi che gli script nominano, per ${depth} livelli, solo da indirizzi su internet, fino a ${file} per file, e ${total} e ${files} file in tutto.`,
    themeCopySaved: (files: number, size: string, date: string) =>
      `Una copia di ${files === 1 ? 'un file' : `${files} file`}, ${size}, salvata il ${date}.`,
    themeCopyFiles: 'File salvati',
    themeCopyUnsaved: (count: number) =>
      count === 1
        ? 'Non è stato possibile salvare un indirizzo; viene ancora caricato da altrove:'
        : `Non è stato possibile salvare ${count} indirizzi; vengono ancora caricati da altrove:`,
    themeCopyMissing: (count: number) =>
      count === 1
        ? 'Un file della copia manca nella cartella dei dati di Polyfin, come dopo il ripristino di un backup su un altro sistema. Aggiorna la copia per scaricarlo di nuovo.'
        : `${count} file della copia mancano nella cartella dei dati di Polyfin, come dopo il ripristino di un backup su un altro sistema. Aggiorna la copia per scaricarli di nuovo.`,
    themeCopyOutside: (count: number) =>
      count === 1
        ? 'I testi personalizzati caricano un indirizzo da altrove:'
        : `I testi personalizzati caricano ${count} indirizzi da altrove:`,
    themeCopyStillOutside: (count: number) =>
      count === 1
        ? 'Un indirizzo aggiunto dopo la copia è caricato da altrove; aggiorna la copia per salvare anche quello:'
        : `${count} indirizzi aggiunti dopo la copia sono caricati da altrove; aggiorna la copia per salvare anche quelli:`,
    themeCopyNothing: 'I testi personalizzati non caricano nulla da altrove.',
    themeCopyKeep: 'Conserva una copia in Polyfin',
    themeCopyUpdate: 'Aggiorna la copia',
    themeCopyRemove: 'Rimuovi la copia',
    themeCopyRemoveTitle: 'Rimuovere la copia?',
    themeCopyRemoveConfirm:
      'I suoi file vengono eliminati, e i testi personalizzati li caricano di nuovo dai loro indirizzi originali.',
    themeCopyKept: 'Copia salvata. Ricarica le pagine aperte del lettore web tra un minuto.',
    themeCopyRemoved: 'Copia rimossa. Ricarica le pagine aperte del lettore web tra un minuto.',
    themeCopyFailed:
      'Non è stato possibile salvare alcun file, quindi nessuna copia è stata fatta:',
    themeCopySaveFirst: 'Salva prima le modifiche: la copia viene fatta dai testi salvati.',
    themeCopyKinds: {
      stylesheet: 'foglio di stile',
      script: 'script',
      font: 'carattere',
      image: 'immagine',
    },
    themeCopyReasons: {
      private_address: 'su una rete locale, o reindirizzato lì',
      not_http: 'reindirizzato a un indirizzo né http né https',
      unavailable: 'non disponibile',
      timeout: 'ha impiegato troppo tempo',
      wrong_type: 'non è un file del tipo con cui è caricato',
      too_large: 'più grande del limite per un file',
      total_too_large: 'oltre il limite dell’intera copia',
      too_many_files: 'oltre il numero di file consentito',
      too_deep: 'nominato troppi livelli più in basso',
      built_at_run_time:
        'completato da uno script quando viene eseguito, cosa che nessuna copia può seguire',
    },
    customCss: 'CSS personalizzato',
    customCssHelp:
      'Applicato a ogni pagina del lettore web, per ogni utente, come CSS personalizzato di Jellyfin. Gli utenti possono disattivarlo in Impostazioni › Visualizzazione.',
    customJs: 'JavaScript personalizzato',
    customJsHelp:
      'Caricato da ogni pagina del lettore web, dopo i suoi script. Se vuoto, non viene caricato nulla.',
    customJsWarningTitle: 'Incolla solo codice di cui ti fidi',
    customJsWarning:
      'Questo script viene eseguito nel browser di ogni utente che apre il lettore web su questo server, con il suo account. Può leggere ciò che vede e agire al suo posto.',
    loginDisclaimer: 'Messaggio di accesso',
    loginDisclaimerHelp:
      'Mostrato sotto il modulo di accesso del lettore web. Testo semplice, Markdown o HTML; il lettore web rimuove l’HTML non sicuro.',
    addVersionsToOpenPage: 'Aggiungi le versioni alla pagina aperta di un titolo',
    addVersionsToOpenPageHelp:
      'Aggiunge le versioni trovate dagli add-on al menu delle versioni della pagina di un film o di un episodio mentre è aperta, mantenendo la versione scelta. Se disattivato, una pagina aperta prima che venisse trovata una versione elenca comunque le prime man mano che arrivano, e i suoi pulsanti di riproduzione attendono comunque una versione; le altre compaiono alla successiva apertura della pagina.',
    homeRows: 'Righe della Home',
    homeRowsHelp:
      'Righe che il lettore web aggiunge alla sua pagina Home, nella loro posizione, per tutti gli utenti o per utenti scelti. Ogni utente può disattivare una riga in Il mio account › Righe della Home; le sezioni proprie del lettore web restano nelle sue Impostazioni › Home. Ogni riga esclude ciò che l’utente non può vedere: i titoli oltre il suo controllo parentale, i generi bloccati, i titoli nascosti e le librerie a lui nascoste.',
    homeRow: {
      loading: 'Caricamento delle righe della Home…',
      empty: 'Nessuna riga della Home',
      emptyHint: 'La pagina Home del lettore web mostra solo le proprie sezioni.',
      add: 'Aggiungi una riga',
      addTitle: 'Aggiungi una riga della Home',
      editTitle: (name: string) => `Modifica ${name}`,
      edit: 'Modifica',
      delete: 'Elimina',
      deleteTitle: (name: string) => `Eliminare ${name}?`,
      deleteBody:
        'La riga scompare dalla pagina Home di ogni utente e la sua libreria scompare dalle altre applicazioni.',
      deleted: (name: string) => `${name} eliminata.`,
      saved: (name: string) => `${name} salvata.`,
      create: 'Aggiungi',
      creating: 'Aggiunta…',
      save: 'Salva',
      saving: 'Salvataggio…',
      name: 'Nome',
      nameHelp: 'Il titolo della riga nella pagina Home.',
      becauseNameHelp:
        'Ogni riga prende questo nome, seguito dal titolo a cui si riferisce: «Perché hai guardato Film di esempio».',
      kind: 'Tipo',
      kinds: {
        catalog: 'Un catalogo',
        collection: 'Una collezione',
        because: 'Perché hai guardato',
        new: 'Novità nelle tue librerie',
      },
      kindHelp: {
        catalog:
          'La prima pagina di un catalogo degli add-on del server, che sia una libreria o no, ad esempio un catalogo di tendenze. Gli utenti che non usano gli add-on del server, o a cui la sua libreria è nascosta, non la vedono.',
        collection:
          'I titoli di una collezione: una creata dagli utenti, una lista intelligente mostrata come collezione o una collezione di un add-on.',
        because:
          'Una riga per ciascuno degli ultimi film e serie TV riprodotti dall’utente, con i titoli simili, come li propongono i consigli sui film. Nessuna finché «Titoli simili» è disattivato.',
        new: 'I titoli arrivati nei cataloghi delle librerie dell’utente negli ultimi 7 giorni, dal più recente. Polyfin li conta dal momento in cui ha visto per la prima volta ogni titolo in ogni catalogo.',
      },
      defaultNames: {
        because: 'Perché hai guardato',
        new: 'Novità nelle tue librerie',
      },
      catalog: 'Catalogo',
      noCatalog: 'Gli add-on del server non hanno cataloghi da mostrare.',
      collection: 'Collezione',
      noCollection: 'Non ci sono ancora collezioni.',
      collectionSources: {
        users: 'Create dagli utenti',
        smart: 'Lista intelligente',
        addon: 'Add-on',
      },
      choose: 'Scegli…',
      unknownCatalog: 'Un catalogo non più disponibile',
      unknownCollection: 'Una collezione non più disponibile',
      count: 'Righe',
      countHelp: 'Quanti degli ultimi film e serie TV riprodotti ottengono una riga, da 1 a 10.',
      place: 'Posizione',
      places: {
        top: 'In alto',
        resume: 'Dopo «Continua a guardare»',
        nextup: 'Dopo «Prossimo»',
        bottom: 'In basso',
      },
      placeHelp:
        'Dove la mostra il lettore web. Se la pagina Home di un utente non mostra quella sezione, la riga prende la posizione precedente.',
      shownTo: 'Mostrata a',
      everyone: 'Tutti gli utenti',
      chosenUsers: 'Utenti scelti',
      users: 'Utenti',
      noUser: 'Scegli almeno un utente.',
      shownToUsers: (count: number) => (count === 1 ? '1 utente' : `${count} utenti`),
      rowsCount: (count: number) => (count === 1 ? '1 riga' : `${count} righe`),
      library: 'Anche una libreria nelle altre applicazioni',
      libraryHelp:
        'Alle altre applicazioni non si può dare un nuovo tipo di sezione della Home: così la riga diventa anche una libreria, senza tipo, che elencano tra le librerie dell’utente, con una riga dei suoi ultimi titoli nella loro Home.',
      libraryBadge: 'Libreria',
      moveLabel: 'Sposta',
    },
    codeSize: (used: number, max: number) =>
      `${used} KB di ${max >= 1024 ? `${max / 1024} MB` : `${max} KB`}`,
    codeKeys: 'Tab inserisce spazi; premi Escape, poi Tab, per uscire dal campo.',
  },
  settingsPage: {
    searchHint: 'Per nome o descrizione.',
    noMatch: 'Nessuna impostazione corrisponde a questa ricerca.',
    sections: {
      variables: 'Variabili d’ambiente',
    },
    variablesHelp:
      'Sola lettura. Sono impostate sul container (ambiente Docker, file compose o modello Unraid) e si applicano all’avvio di Polyfin: modificale lì, poi riavvia il container. Le variabili che prima impostavano altre opzioni vengono copiate una sola volta nelle impostazioni, al primo avvio di questa versione, e poi non vengono più lette: modificale nelle impostazioni. I segreti sono nascosti e l’indirizzo del database mostra solo l’host e il nome del database.',
    value: 'Valore in uso',
    defaultValue: 'Predefinito',
    setValue: 'Impostata',
    hidden: 'Nascosta',
    empty: 'Vuota',
    notRead: 'Non letta da Polyfin',
    searchPlaceholder: 'Cerca un’impostazione',
    results: (count: number) =>
      count === 1 ? '1 impostazione trovata' : `${count} impostazioni trovate`,
    noMatchHelp: 'Prova con un’altra parola, ad esempio il nome di una variabile o di un servizio.',
    variableHint: 'Variabile d’ambiente',
    ledes: {
      general:
        'Il nome del server, la sua lingua, come le applicazioni accedono e lo trovano, e il controllo di nuove versioni.',
      playback: 'Come Polyfin sceglie e prepara le versioni che riproduce.',
      content:
        'Pulsanti per saltare l’intro, titoli simili, immagini generate, voti, testi delle canzoni, volume uniforme tra i brani e soglie di riproduzione.',
      catalogs: 'Quanto Polyfin legge dai cataloghi degli add-on, e con quale frequenza.',
      security:
        'Add-on e importazioni degli utenti, account bloccati, dispositivi inutilizzati e reverse proxy.',
      liveTv: 'Ogni quanto vengono riscaricati gli elenchi canali e le guide IPTV.',
      diagnostics: 'Il registro dettagliato e le variabili d’ambiente in uso.',
      notifications:
        'Dove Polyfin segnala, per ogni utente, nuovi episodi, registrazioni, riproduzioni e connessioni che si interrompono, oltre ai problemi dello stato di salute, ai nuovi utenti, alle nuove versioni e agli account bloccati, e invia il riepilogo settimanale, il mese in sintesi e l’anno in sintesi.',
    },
    groups: {
      skip: 'Salta intro e titoli di coda',
      titlePages: 'Pagine dei titoli',
      images: 'Immagini',
      ratings: 'Voti',
      music: 'Musica',
      thresholds: 'Soglie di riproduzione',
    },
    leaveTitle: 'Uscire senza salvare?',
    leaveBody:
      'Le modifiche apportate in questa sezione non sono ancora salvate. Andranno perse se esci.',
    leave: 'Esci senza salvare',
    stay: 'Resta',
    variablesEmpty: 'Nessuna variabile da mostrare',
    variablesEmptyHelp: 'Imposta le variabili POLYFIN_ sul container, poi riavvialo.',
  },
}

export default settings
