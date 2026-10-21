import type en from '../en/livetv'

const livetv: typeof en = {
  livetv: {
    title: 'Diretta TV',
    description:
      'I cataloghi TV provengono dalle tue origini. Apri le guide di un catalogo per assegnare una programmazione ai suoi canali.',
    recordings: 'Impostazioni di registrazione',
    settings: 'Impostazioni Diretta TV',
    catalogsTitle: 'Cataloghi TV',
    catalogsHelp:
      'Un catalogo TV mostrato come libreria inserisce i suoi canali in Diretta TV nelle applicazioni Jellyfin. Le sue guide forniscono la programmazione ai canali.',
    catalogsLabel: 'Cataloghi TV',
    catalogsLoading: 'Caricamento dei cataloghi TV…',
    catalogsEmptyTitle: 'Ancora nessun catalogo TV',
    catalogsEmpty:
      'Installa un add-on con un catalogo TV, oppure aggiungi un’origine IPTV, in Origini: i suoi canali vengono elencati qui.',
    addSource: 'Aggiungi un’origine',
    kinds: { stremio: 'Add-on Stremio', iptv: 'Origine IPTV' },
    guides: (count: number) => (count === 1 ? '1 guida' : `${count} guide`),
    guideNone: 'Nessuna guida dei programmi',
    failing: (count: number) =>
      count === 1
        ? 'L’ultimo download di 1 guida non è riuscito.'
        : `L’ultimo download di ${count} guide non è riuscito.`,
    mapped: (mapped: string, channels: string) => `${mapped} canali su ${channels} con una guida`,
    guideLine: (position: number, message: string) => `Guida ${position}: ${message}`,
    manage: 'Guide e abbinamento',
    lineup: 'Elenco dei canali e guide',
    lineupLabel: (name: string) => `Elenco dei canali e guide di ${name}`,
    manageLabel: (name: string) => `Guide e abbinamento di ${name}`,
    notShownHelp:
      'Mostralo come libreria per inserire i suoi canali in Diretta TV e assegnargli delle guide.',
    openLibraries: 'Apri Librerie',
    states: {
      notShown: 'Non in Diretta TV',
      addonOff: 'Add-on disattivato',
      missing: 'Non più disponibile',
      failed: 'Download della guida non riuscito',
      noGuide: 'Nessuna guida',
      notFetched: 'Non ancora scaricato',
      incomplete: 'Guida incompleta',
      complete: 'Guida completa',
    },
    iptvTitle: 'Canali IPTV',
    iptvHelp:
      'I canali in diretta di ogni origine IPTV, così come li conserva il suo elenco dei canali.',
    iptvLoading: 'Caricamento delle origini IPTV…',
    iptvEmptyTitle: 'Ancora nessuna origine IPTV',
    iptvEmpty:
      'Aggiungi una scaletta M3U o un account Xtream Codes in Origini: i suoi canali vengono conteggiati qui.',
    kindM3u: 'Scaletta M3U',
    kindXtream: 'Account Xtream Codes',
    kindTuner: 'Sintonizzatore HDHomeRun',
    turnedOff: 'Disattivata',
    lastDownload: 'Ultimo download',
    nextDownload: 'prossimo',
    never: 'mai',
    figuresLabel: (name: string) => `Canali di ${name}`,
    figures: {
      entries: 'Voci in diretta',
      enabled: 'Canali attivi',
      shown: 'Mostrati nelle app',
      mapped: 'Con una guida',
      unmapped: 'Senza guida',
    },
    noLiveTv: 'Questa origine non importa canali in diretta.',
    importOptions: 'Opzioni di importazione',
    channels: 'Canali',
    channelsLabel: (name: string) => `Canali di ${name}`,
    openMapping: 'Apri l’abbinamento delle guide',
    recordingsTitle: 'Registrazioni',
    recordingsLoading: 'Caricamento delle registrazioni…',
    recordingsOffTitle: 'La registrazione è disattivata',
    recordingsOff:
      'Attivala, e scegli i margini e per quanto tempo conservare le registrazioni, in Impostazioni › Registrazioni.',
    noTimersTitle: 'Nessuna registrazione pianificata',
    noTimers:
      'Le registrazioni si pianificano dalla guida dei programmi nelle applicazioni Jellyfin. Vengono elencate qui fino al loro termine.',
    recordingNow: 'In registrazione',
    scheduled: 'Pianificata',
    failed: 'Non riuscita',
    series: 'Serie TV',
    scheduledBy: (name: string) => `Pianificata da ${name}`,
    unknownChannel: 'Canale non trovato',
    tunersShort: 'Sintonizzatori insufficienti',
    tunersShortHelp:
      'Sui canali di questo dispositivo sono in corso contemporaneamente più registrazioni dei sintonizzatori disponibili: una di esse potrebbe non riuscire.',
    guidesOf: (name: string) => `Guide di ${name}`,
    spaceSaved: (count: number, saved: string) =>
      count === 1
        ? `1 registrazione convertita in HEVC: ${saved} risparmiati.`
        : `${count} registrazioni convertite in HEVC: ${saved} risparmiati.`,
  },
}

export default livetv
