import type en from '../en/statistics'

const statistics: typeof en = {
  statistics: {
    title: 'Statistiche',
    description:
      'Come viene usato il server: ore guardate, titoli, applicazioni e dispositivi, e come i video arrivano alle applicazioni. Brani e audiolibri non vengono conteggiati.',
    period: 'Periodo',
    periods: { '7d': '7 giorni', '30d': '30 giorni', year: 'Anno', all: 'Tutto' },
    user: 'Utente',
    allUsers: 'Tutti gli utenti',
    exportCsv: 'Esporta CSV',
    historyOff: {
      server:
        'La cronologia di riproduzione è disattivata: le nuove riproduzioni non vengono conteggiate.',
      own: 'La cronologia di riproduzione è disattivata su questo server: le nuove riproduzioni non vengono conteggiate.',
    },
    openSettings: 'Apri Impostazioni › Riproduzione',
    empty: 'Nessuna riproduzione in questo periodo',
    emptyHint:
      'I video riprodotti per almeno 10 secondi in un’applicazione Jellyfin compaiono qui una volta interrotti.',
    figures: {
      played: 'Ore guardate',
      plays: 'Riproduzioni',
      users: 'Utenti',
      sent: 'Dati inviati',
    },
    perDay: 'Ore guardate al giorno',
    perMonth: 'Ore guardate al mese',
    perUser: 'Ore guardate per utente',
    movies: 'Film più riprodotti',
    series: 'Serie TV più riprodotte',
    channels: 'Canali più riprodotti',
    apps: 'Applicazioni',
    devices: 'Dispositivi',
    methods: 'Come vengono riprodotti i video',
    methodNames: {
      direct_play: 'Riproduzione diretta',
      direct_stream: 'Remux',
      conversion: 'Conversione',
      '': 'Sconosciuto',
    },
    hours: 'Orari di maggiore attività',
    hoursHelp: 'Ore guardate in ogni ora della settimana, nel tuo fuso orario.',
    hoursCell: (day: string, hour: string, time: string) => `${day} ${hour}: ${time}`,
    plays: (count: number) => (count === 1 ? '1 riproduzione' : `${count} riproduzioni`),
    users: (count: number) => (count === 1 ? '1 utente' : `${count} utenti`),
    share: (percent: string) => `${percent} delle riproduzioni`,
    none: 'Nessuno in questo periodo.',
    unknownApp: 'Applicazione sconosciuta',
    unknownDevice: 'Dispositivo sconosciuto',
    bar: (label: string, time: string) => `${label}: ${time}`,
    history: 'Riproduzioni recenti',
    historyEmpty: 'Nessuna riproduzione in questo periodo.',
    columns: {
      started: 'Inizio',
      user: 'Utente',
      title: 'Titolo',
      device: 'Applicazione e dispositivo',
      played: 'Durata riprodotta',
      sent: 'Dati inviati',
      method: 'Modalità di riproduzione',
    },
    appOn: (app: string, device: string) => (app && device ? `${app} su ${device}` : app || device),
    shared: 'Condiviso',
    sharedHelp: 'Riprodotto da un ospite tramite un link di condivisione',
    sharedLink: 'Link condiviso',
    webBrowser: 'Browser web',
    showMore: 'Mostra altro',
    loadingMore: 'Caricamento…',
  },
}

export default statistics
