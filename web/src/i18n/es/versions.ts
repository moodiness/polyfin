import type en from '../en/versions'

const versions: typeof en = {
  versions: {
    title: 'Versiones',
    description:
      'Vea qué pasa con cada transmisión que las extensiones listan para una película o un episodio, tal como se le ofrecen a un usuario, y fije o bloquee versiones.',
    asUser: 'Como usuario',
    asUserHelp:
      'Las versiones dependen del usuario: sus propias extensiones, los torrents, su grupo de calidad y su idioma de audio.',
    you: (name: string) => `${name} (usted)`,
    find: 'Buscar un título',
    search: 'Película o serie',
    searchHelp: 'Busca en las bibliotecas que ve este usuario. Una serie lista sus episodios.',
    searchButton: 'Buscar',
    searching: 'Buscando…',
    noTitles: 'No se encontró ninguna película ni serie.',
    titlesFound: (term: string) => `Títulos encontrados para «${term}»`,
    pickTitle: (name: string, details: string) => `${name}, ${details}`,
    episodesOf: (name: string) => `Episodios de ${name}`,
    noEpisodes: 'Ningún episodio.',
    back: 'Volver',
    everywhere: 'Bloqueados para todos los títulos',
    everywhereHelp:
      'No se lista ninguna versión de estos archivos para ningún título. El bloqueo de un solo título aparece en su página.',
    noBlocks: 'Ningún archivo está bloqueado para todos los títulos.',
    backToSearch: 'Versiones',
    askAgain: 'Volver a preguntar a las extensiones',
    askedAgain: 'Se vuelve a preguntar a las extensiones.',
    askedLately:
      'Se preguntó a las extensiones hace un momento: vuelva a intentarlo en unos segundos.',
    pending: (count: number) =>
      count === 1
        ? 'Todavía se pregunta a 1 extensión…'
        : `Todavía se pregunta a ${count} extensiones…`,
    order: 'Cómo se ordenan las versiones',
    orderNone: 'El orden de las extensiones: Ordenar las versiones por no tiene ningún criterio.',
    orderBy: (criteria: string) => `Ordenar las versiones por: ${criteria}.`,
    group: (height: number) => `Grupo de calidad: ${height}p.`,
    keptAll:
      'Las reglas de ocultación ocultarían todas las versiones, así que no se oculta ninguna.',
    closest: 'Ninguna versión cabe en el grupo de calidad: las alturas más cercanas van primero.',
    pin: 'Versión fijada',
    noPin:
      'No hay ninguna versión fijada. Una versión fijada va primero para todos los usuarios, en las páginas de los títulos y en la reproducción, mientras una extensión la liste; una versión elegida en el menú de versiones se sigue reproduciendo.',
    pinnedBy: (name: string) => (name === '' ? 'Fijada' : `Fijada por ${name}`),
    pinMissing: 'Ninguna extensión lista este archivo ahora: se aplica el orden habitual.',
    unpin: 'Quitar fijación',
    pinAction: 'Fijar',
    pinned: 'Fijada',
    pinDone: 'Versión fijada.',
    unpinDone: 'Versión ya no fijada.',
    sources: 'Transmisiones por fuente',
    sourceStates: {
      known: 'Respondió',
      stale: 'Caducada, se vuelve a preguntar',
      asked: 'Preguntando',
    },
    embedded: 'En la descripción del título',
    noStreams: 'No listó ninguna transmisión.',
    noSources: 'Ninguna extensión lista transmisiones para este título.',
    steps: {
      offered: (place: number) => `Versión ${place}`,
      nothing: 'Nada que reproducir',
      torrents: 'Torrents desactivados para este usuario',
      blocked: 'Bloqueado',
      duplicate: 'El mismo archivo listado antes',
      mirror: 'Espejo',
      failed: 'Falló hace poco',
      hidden: 'Oculta por una regla',
      group: 'Por encima del grupo de calidad',
    },
    stepHelp: {
      nothing: 'Una página web, un vídeo de otro sitio o un aviso: nada que Polyfin reproduzca.',
      torrents:
        'Reproducir transmisiones torrent está desactivado, o el usuario no puede reproducir transmisiones torrent (Ajustes › Torrents).',
      duplicate: 'El mismo nombre de archivo y el mismo tamaño, listados antes: una sola versión.',
      mirror:
        'El mismo archivo de otra extensión, guardado como espejo oculto: una reproducción sigue en él cuando la fuente de la versión falla.',
      group:
        'El grupo de calidad del usuario deja fuera las versiones más altas mientras alguna quepa.',
    },
    rules: {
      elsewhere: (marker: string) => `Nombra otro título (${marker})`,
      larger: (gigabytes: string) => `Ocultar las versiones de más de ${gigabytes} GB`,
      codec: (codec: string) => `Ocultar estos códecs de vídeo: ${codec}`,
      release: (kind: string) => `Ocultar publicaciones de baja calidad: ${kind}`,
      words: (word: string) => `Ocultar las versiones cuyas etiquetas contienen «${word}»`,
      label: (text: string) =>
        `Reglas de etiquetas: ocultar las versiones que coinciden con «${text}»`,
      other: (rule: string, detail: string) => (detail === '' ? rule : `${rule}: ${detail}`),
      none: 'Oculta por una regla cambiada desde entonces',
    },
    unstarted: 'No arrancó hace poco: las reproducciones que no eligen versión la pasan por alto',
    labelsAsRead: 'Etiquetas:',
    facts: 'Lo que saben las reglas:',
    analyzed: 'Analizada',
    multi: 'MULTI',
    tiers: { disc: 'Copia de disco', bluray: 'Blu-ray', web: 'Web', dvd: 'DVD', tv: 'TV' },
    lossless: 'Sin pérdida',
    objectAudio: 'Atmos o DTS:X',
    perMinute: (megabytes: number) => `${megabytes} MB/min`,
    labelRules: (rules: string[], score: number) =>
      `Reglas de etiquetas que coinciden: ${rules.map((rule) => `«${rule}»`).join(', ')}; puntuación ${score > 0 ? '+' : ''}${score}`,
    block: 'Bloquear',
    blockTitle: 'Bloquear este archivo',
    blockScope: 'Bloquearlo para',
    scopeTitle: 'Este título',
    scopeEverywhere: 'Todos los títulos',
    scopeHelp:
      'Todos los títulos bloquea este nombre de archivo y este tamaño dondequiera que una extensión los liste. Ninguna versión de un archivo bloqueado se lista ni se reproduce; las reglas de ocultación nunca ocultan todas las versiones, un bloqueo sí.',
    reason: 'Motivo',
    reasonHelp: (max: number) =>
      `Por qué, para el siguiente administrador. Hasta ${max} caracteres.`,
    blocked: 'Bloqueado',
    blockedBy: (name: string, reason: string) =>
      [name === '' ? 'Bloqueado' : `Bloqueado por ${name}`, reason].filter(Boolean).join(': '),
    blockedEverywhere: 'Para todos los títulos',
    unblock: 'Desbloquear',
    blockDone: 'Archivo bloqueado.',
    unblockDone: 'Archivo desbloqueado.',
    titleBlocks: 'Bloqueados para este título',
    noTitleBlocks: 'Ningún archivo está bloqueado para este título.',
    analyze: 'Analizar ahora',
    analyzeDone: 'Versión analizada.',
    analysisFailed: (why: string) => `No se pudo analizar: ${why}`,
    decisions: 'Últimas reproducciones',
    decisionsHelp:
      'Lo que intentaron las últimas reproducciones de este título, para cualquier usuario. Se guardan hasta un reinicio.',
    noDecisions: 'Ninguna reproducción desde que se inició el servidor.',
    decisionBy: (user: string, device: string, app: string) =>
      [user, device, app].filter(Boolean).join(' · '),
    picked: 'Elegida en el menú de versiones',
    noneChosen: 'No se reprodujo ninguna versión',
    deliveries: {
      directPlay: 'reproducción directa',
      directStream: 'transmisión directa',
      converted: 'convertida',
    },
    outcomes: {
      plays: 'se reproduce',
      unreadable: 'no se pudo leer',
      unstarted: 'el torrent no arrancó',
      torrentCap: 'había tantos torrents como se permiten',
      conversionCap: 'había tantas conversiones como se permiten',
      unplayable: 'necesita una conversión que el usuario no tiene, o supera sus límites',
      notAnalyzed: 'no se analizó antes, así que no se intentó',
      passed: 'todavía se estaba leyendo cuando la reproducción siguió',
    },
    notFound: 'Este título no se encuentra para este usuario.',
  },
}

export default versions
