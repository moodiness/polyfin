import type en from '../en/livetv'

const livetv: typeof en = {
  livetv: {
    title: 'Live-TV',
    description:
      'TV-Kataloge stammen aus deinen Quellen. Öffne die Fernsehprogramme eines Katalogs, um seinen Kanälen ein Programm zu geben.',
    recordings: 'Aufnahmeeinstellungen',
    settings: 'Live-TV-Einstellungen',
    catalogsTitle: 'TV-Kataloge',
    catalogsHelp:
      'Ein als Bibliothek angezeigter TV-Katalog bringt seine Kanäle in das Live-TV der Jellyfin-Apps. Seine Fernsehprogramme liefern den Kanälen ihr Programm.',
    catalogsLabel: 'TV-Kataloge',
    catalogsLoading: 'TV-Kataloge werden geladen…',
    catalogsEmptyTitle: 'Noch kein TV-Katalog',
    catalogsEmpty:
      'Installiere unter „Quellen“ ein Add-on mit TV-Katalog oder füge eine IPTV-Quelle hinzu: Ihre Kanäle werden hier aufgeführt.',
    addSource: 'Quelle hinzufügen',
    kinds: { stremio: 'Stremio-Add-on', iptv: 'IPTV-Quelle' },
    guides: (count: number) => (count === 1 ? '1 Fernsehprogramm' : `${count} Fernsehprogramme`),
    guideNone: 'Kein Fernsehprogramm',
    failing: (count: number) =>
      count === 1
        ? 'Der letzte Download von 1 Fernsehprogramm ist fehlgeschlagen.'
        : `Der letzte Download von ${count} Fernsehprogrammen ist fehlgeschlagen.`,
    mapped: (mapped: string, channels: string) =>
      `${mapped} von ${channels} Kanälen mit Fernsehprogramm`,
    guideLine: (position: number, message: string) => `Fernsehprogramm ${position}: ${message}`,
    manage: 'Fernsehprogramme und Zuordnung',
    lineup: 'Senderliste und Fernsehprogramme',
    lineupLabel: (name: string) => `Senderliste und Fernsehprogramme von ${name}`,
    manageLabel: (name: string) => `Fernsehprogramme und Zuordnung von ${name}`,
    notShownHelp:
      'Zeige ihn als Bibliothek an, um seine Kanäle ins Live-TV zu bringen und ihm Fernsehprogramme zu geben.',
    openLibraries: 'Bibliotheken öffnen',
    states: {
      notShown: 'Nicht im Live-TV',
      addonOff: 'Add-on deaktiviert',
      missing: 'Nicht mehr verfügbar',
      failed: 'Download des Fernsehprogramms fehlgeschlagen',
      noGuide: 'Kein Fernsehprogramm',
      notFetched: 'Noch nicht abgerufen',
      incomplete: 'Fernsehprogramm unvollständig',
      complete: 'Fernsehprogramm vollständig',
    },
    iptvTitle: 'IPTV-Kanäle',
    iptvHelp: 'Die Live-Kanäle jeder IPTV-Quelle, so wie ihre Senderliste sie führt.',
    iptvLoading: 'IPTV-Quellen werden geladen…',
    iptvEmptyTitle: 'Noch keine IPTV-Quelle',
    iptvEmpty:
      'Füge unter „Quellen“ eine M3U-Wiedergabeliste oder ein Xtream-Codes-Konto hinzu: Ihre Kanäle werden hier gezählt.',
    kindM3u: 'M3U-Wiedergabeliste',
    kindXtream: 'Xtream-Codes-Konto',
    kindTuner: 'HDHomeRun-Tuner',
    turnedOff: 'Deaktiviert',
    lastDownload: 'Letzter Download',
    nextDownload: 'nächster',
    never: 'nie',
    figuresLabel: (name: string) => `Kanäle von ${name}`,
    figures: {
      entries: 'Live-Einträge',
      enabled: 'Aktive Kanäle',
      shown: 'In Apps angezeigt',
      mapped: 'Mit Fernsehprogramm',
      unmapped: 'Ohne Fernsehprogramm',
    },
    noLiveTv: 'Diese Quelle importiert keine Live-Kanäle.',
    importOptions: 'Importoptionen',
    channels: 'Kanäle',
    channelsLabel: (name: string) => `Kanäle von ${name}`,
    openMapping: 'Programmzuordnung öffnen',
    recordingsTitle: 'Aufnahmen',
    recordingsLoading: 'Aufnahmen werden geladen…',
    recordingsOffTitle: 'Aufnahme ist deaktiviert',
    recordingsOff:
      'Aktiviere sie und lege Vor- und Nachlaufzeit sowie die Aufbewahrungsdauer der Aufnahmen unter Einstellungen › Aufnahmen fest.',
    noTimersTitle: 'Keine Aufnahme geplant',
    noTimers:
      'Aufnahmen werden über das Fernsehprogramm in den Jellyfin-Apps geplant. Sie werden hier aufgeführt, bis sie enden.',
    recordingNow: 'Wird aufgenommen',
    scheduled: 'Geplant',
    failed: 'Fehlgeschlagen',
    series: 'Serie',
    scheduledBy: (name: string) => `Geplant von ${name}`,
    unknownChannel: 'Kanal nicht gefunden',
    tunersShort: 'Nicht genug Tuner',
    tunersShortHelp:
      'Auf den Kanälen dieses Geräts laufen gleichzeitig mehr Aufnahmen, als es Tuner hat: Eine davon kann fehlschlagen.',
    guidesOf: (name: string) => `Fernsehprogramme von ${name}`,
    spaceSaved: (count: number, saved: string) =>
      count === 1
        ? `1 Aufnahme in HEVC konvertiert: ${saved} gespart.`
        : `${count} Aufnahmen in HEVC konvertiert: ${saved} gespart.`,
  },
}

export default livetv
