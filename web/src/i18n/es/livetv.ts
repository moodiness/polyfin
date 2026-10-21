import type en from '../en/livetv'

const livetv: typeof en = {
  livetv: {
    title: 'Televisión en vivo',
    description:
      'Los catálogos de TV proceden de sus fuentes. Abra las guías de un catálogo para dar programación a sus canales.',
    recordings: 'Ajustes de grabación',
    settings: 'Ajustes de Televisión en vivo',
    catalogsTitle: 'Catálogos de TV',
    catalogsHelp:
      'Un catálogo de TV mostrado como biblioteca coloca sus canales en Televisión en vivo en las aplicaciones de Jellyfin. Sus guías dan la programación a los canales.',
    catalogsLabel: 'Catálogos de TV',
    catalogsLoading: 'Cargando catálogos de TV…',
    catalogsEmptyTitle: 'Aún no hay ningún catálogo de TV',
    catalogsEmpty:
      'Instale una extensión que tenga un catálogo de TV, o añada una fuente IPTV, en Fuentes: sus canales aparecerán aquí.',
    addSource: 'Añadir una fuente',
    kinds: { stremio: 'Extensión de Stremio', iptv: 'Fuente IPTV' },
    guides: (count: number) => (count === 1 ? '1 guía' : `${count} guías`),
    guideNone: 'Sin guía de programación',
    failing: (count: number) =>
      count === 1
        ? 'La última descarga de 1 guía ha fallado.'
        : `La última descarga de ${count} guías ha fallado.`,
    mapped: (mapped: string, channels: string) => `${mapped} de ${channels} canales con guía`,
    guideLine: (position: number, message: string) => `Guía ${position}: ${message}`,
    manage: 'Guías y asignación',
    lineup: 'Parrilla de canales y guías',
    lineupLabel: (name: string) => `Parrilla de canales y guías de ${name}`,
    manageLabel: (name: string) => `Guías y asignación de ${name}`,
    notShownHelp:
      'Muéstrelo como biblioteca para colocar sus canales en Televisión en vivo y asignarle guías.',
    openLibraries: 'Abrir Bibliotecas',
    states: {
      notShown: 'No está en Televisión en vivo',
      addonOff: 'Extensión desactivada',
      missing: 'Ya no disponible',
      failed: 'Error al descargar la guía',
      noGuide: 'Sin guía',
      notFetched: 'Aún no descargada',
      incomplete: 'Guía incompleta',
      complete: 'Guía completa',
    },
    iptvTitle: 'Canales IPTV',
    iptvHelp:
      'Los canales en directo de cada fuente IPTV, tal como los conserva su parrilla de canales.',
    iptvLoading: 'Cargando fuentes IPTV…',
    iptvEmptyTitle: 'Aún no hay ninguna fuente IPTV',
    iptvEmpty:
      'Añada una lista de reproducción M3U o una cuenta de Xtream Codes en Fuentes: sus canales se contabilizarán aquí.',
    kindM3u: 'Lista de reproducción M3U',
    kindXtream: 'Cuenta de Xtream Codes',
    kindTuner: 'Sintonizador HDHomeRun',
    turnedOff: 'Desactivada',
    lastDownload: 'Última descarga',
    nextDownload: 'próxima',
    never: 'nunca',
    figuresLabel: (name: string) => `Canales de ${name}`,
    figures: {
      entries: 'Entradas en directo',
      enabled: 'Canales activados',
      shown: 'Visibles en las aplicaciones',
      mapped: 'Con guía',
      unmapped: 'Sin guía',
    },
    noLiveTv: 'Esta fuente no importa ningún canal en directo.',
    importOptions: 'Opciones de importación',
    channels: 'Canales',
    channelsLabel: (name: string) => `Canales de ${name}`,
    openMapping: 'Abrir la asignación de guías',
    recordingsTitle: 'Grabaciones',
    recordingsLoading: 'Cargando grabaciones…',
    recordingsOffTitle: 'La grabación está desactivada',
    recordingsOff:
      'Actívela, y elija los márgenes y cuánto tiempo se conservan las grabaciones, en Ajustes › Grabaciones.',
    noTimersTitle: 'No hay ninguna grabación programada',
    noTimers:
      'Las grabaciones se programan desde la guía de programación de las aplicaciones de Jellyfin. Aparecen aquí hasta que terminan.',
    recordingNow: 'Grabando',
    scheduled: 'Programada',
    failed: 'Fallida',
    series: 'Serie',
    scheduledBy: (name: string) => `Programada por ${name}`,
    unknownChannel: 'Canal no encontrado',
    tunersShort: 'Sintonizadores insuficientes',
    tunersShortHelp:
      'Hay más grabaciones simultáneas en los canales de este sintonizador que sintonizadores disponibles: una de ellas puede fallar.',
    guidesOf: (name: string) => `Guías de ${name}`,
    spaceSaved: (count: number, saved: string) =>
      count === 1
        ? `1 grabación convertida a HEVC: ${saved} ahorrados.`
        : `${count} grabaciones convertidas a HEVC: ${saved} ahorrados.`,
  },
}

export default livetv
