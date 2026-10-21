import type en from '../en/statistics'

const statistics: typeof en = {
  statistics: {
    title: 'Statistiken',
    description:
      'Wie der Server genutzt wird: angesehene Stunden, Titel, Apps und Geräte sowie die Art, wie Videos zu den Apps gelangen. Songs und Hörbücher werden nicht gezählt.',
    period: 'Zeitraum',
    periods: { '7d': '7 Tage', '30d': '30 Tage', year: 'Jahr', all: 'Gesamt' },
    user: 'Benutzer',
    allUsers: 'Alle Benutzer',
    exportCsv: 'Als CSV exportieren',
    historyOff: {
      server: 'Der Wiedergabeverlauf ist deaktiviert: Neue Wiedergaben werden nicht gezählt.',
      own: 'Der Wiedergabeverlauf ist auf diesem Server deaktiviert: Neue Wiedergaben werden nicht gezählt.',
    },
    openSettings: 'Einstellungen › Wiedergabe öffnen',
    empty: 'Keine Wiedergabe in diesem Zeitraum',
    emptyHint:
      'Videos, die mindestens 10 Sekunden lang in einer Jellyfin-App abgespielt wurden, erscheinen hier, sobald sie beendet werden.',
    figures: { played: 'Angesehene Stunden', plays: 'Wiedergaben', users: 'Benutzer' },
    perDay: 'Angesehene Stunden pro Tag',
    perMonth: 'Angesehene Stunden pro Monat',
    perUser: 'Angesehene Stunden pro Benutzer',
    movies: 'Meistgesehene Filme',
    series: 'Meistgesehene Serien',
    channels: 'Meistgesehene Kanäle',
    apps: 'Apps',
    devices: 'Geräte',
    methods: 'Wie Videos abgespielt werden',
    methodNames: {
      direct_play: 'Direktes Abspielen',
      direct_stream: 'Remux',
      conversion: 'Konvertierung',
      '': 'Unbekannt',
    },
    hours: 'Stoßzeiten',
    hoursHelp: 'Angesehene Stunden zu jeder Stunde der Woche, in deiner Zeitzone.',
    hoursCell: (day: string, hour: string, time: string) => `${day} ${hour}: ${time}`,
    plays: (count: number) => (count === 1 ? '1 Wiedergabe' : `${count} Wiedergaben`),
    users: (count: number) => (count === 1 ? '1 Benutzer' : `${count} Benutzer`),
    share: (percent: string) => `${percent} der Wiedergaben`,
    none: 'Keine in diesem Zeitraum.',
    unknownApp: 'Unbekannte App',
    unknownDevice: 'Unbekanntes Gerät',
    bar: (label: string, time: string) => `${label}: ${time}`,
    history: 'Letzte Wiedergaben',
    historyEmpty: 'Keine Wiedergabe in diesem Zeitraum.',
    columns: {
      started: 'Gestartet',
      user: 'Benutzer',
      title: 'Titel',
      device: 'App und Gerät',
      played: 'Abgespielt',
      method: 'Wiedergabeart',
    },
    appOn: (app: string, device: string) =>
      app && device ? `${app} auf ${device}` : app || device,
    shared: 'Geteilt',
    sharedHelp: 'Von einem Gast über einen Freigabelink abgespielt',
    sharedLink: 'Freigabelink',
    webBrowser: 'Webbrowser',
    showMore: 'Mehr anzeigen',
    loadingMore: 'Wird geladen…',
  },
}

export default statistics
